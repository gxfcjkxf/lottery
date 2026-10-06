export const SETTLEMENT_JOB_PAGE_SIZE = 20;
export const SETTLEMENT_JOB_REASON_MAX_BYTES = 500;

export type SettlementJobOperation = "policy" | "start" | "approve" | "retry";

export interface FrozenSettlementJobIntent<TBody extends object = Record<string, unknown>> {
  readonly accountId: string;
  readonly brandId: string;
  readonly periodId: string;
  readonly operation: SettlementJobOperation;
  readonly resourceId: string | null;
  readonly expectedMode?: "automatic" | "manual";
  readonly body: Readonly<TBody>;
  readonly key: string;
}

function cloneValue<T>(value: T): T {
  // Vue refs may expose Proxy objects, which structuredClone rejects. Intent
  // bodies are JSON values; recursive enumeration produces a plain snapshot.
  if (Array.isArray(value)) return value.map(cloneValue) as T;
  if (value && typeof value === "object") {
    return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, cloneValue(item)])) as T;
  }
  return value;
}

function deepFreeze<T>(value: T): T {
  if (value && typeof value === "object" && !Object.isFrozen(value)) {
    Object.freeze(value);
    Object.values(value).forEach(deepFreeze);
  }
  return value;
}

export function freezeSettlementJobIntent<TBody extends object>(input: {
  accountId: string;
  brandId: string;
  periodId: string;
  operation: SettlementJobOperation;
  resourceId?: string | null;
  expectedMode?: "automatic" | "manual";
  body: TBody;
  key: string;
}): FrozenSettlementJobIntent<TBody> {
  return deepFreeze({
    ...input,
    resourceId: input.resourceId ?? null,
    body: deepFreeze(cloneValue(input.body)),
  });
}

export function createFrozenSettlementJobStore<TBody extends object>() {
  const entries = new Map<string, FrozenSettlementJobIntent<TBody>>();
  const scopeKey = (accountId: string, brandId: string, periodId: string) =>
    JSON.stringify([accountId, brandId, periodId]);
  return {
    remember(intent: FrozenSettlementJobIntent<TBody>) {
      entries.set(scopeKey(intent.accountId, intent.brandId, intent.periodId), intent);
    },
    find(accountId: string, brandId: string, periodId: string) {
      return entries.get(scopeKey(accountId, brandId, periodId)) ?? null;
    },
    forget(intent: FrozenSettlementJobIntent<TBody>) {
      const key = scopeKey(intent.accountId, intent.brandId, intent.periodId);
      if (entries.get(key) === intent) entries.delete(key);
    },
    clear() {
      entries.clear();
    },
  };
}

/** Pending intents live only in memory and are scoped to account, brand, and period. */
export const frozenSettlementJobWrites =
  createFrozenSettlementJobStore<Record<string, unknown>>();

export type SettlementJobWriteFailure = "unknown" | "conflict" | "rejected";

export function settlementJobWriteFailure(status: number | undefined): SettlementJobWriteFailure {
  if (status === undefined || status === 0 || status >= 500) return "unknown";
  if (status === 409) return "conflict";
  if (status >= 400 && status < 500) return "rejected";
  return "unknown";
}

export function createSettlementJobRequestLane() {
  let generation = 0;
  return {
    begin() {
      generation += 1;
      return generation;
    },
    invalidate() {
      generation += 1;
    },
    isCurrent(ticket: number) {
      return generation === ticket;
    },
  };
}

export function sameSettlementJobOperation(
  intent: Pick<FrozenSettlementJobIntent, "accountId" | "brandId" | "periodId" | "operation" | "resourceId">,
  expected: { accountId: string; brandId: string; periodId: string; operation: SettlementJobOperation; resourceId?: string | null },
): boolean {
  return intent.accountId === expected.accountId &&
    intent.brandId === expected.brandId &&
    intent.periodId === expected.periodId &&
    intent.operation === expected.operation &&
    intent.resourceId === (expected.resourceId ?? null);
}

export function matchesSettlementJobReceipt(
  job: {
    id: string;
    brand_id: string;
    period_id: string;
    period_version: number;
    policy_version: number;
    draw_result_id: string;
    mode: "automatic" | "manual";
    reason: string;
    created_by: string;
  },
  expected: {
    id?: string;
    brandId: string;
    periodId: string;
    periodVersion: number;
    policyVersion: number;
    drawResultId: string;
    mode: "automatic" | "manual";
    reason: string;
    accountId: string;
  },
): boolean {
  return Boolean(job.id) && (!expected.id || job.id === expected.id) &&
    job.brand_id === expected.brandId && job.period_id === expected.periodId &&
    job.period_version === expected.periodVersion && job.policy_version === expected.policyVersion &&
    job.draw_result_id === expected.drawResultId && job.mode === expected.mode &&
    job.reason === expected.reason && job.created_by === expected.accountId;
}

/** Group arbitrary precision non-negative integer strings without Number conversion. */
export function formatSettlementPoints(value: string | null | undefined): string {
  if (value == null) return "—";
  if (!/^(0|[1-9][0-9]*)$/.test(value)) return value;
  return value.replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}

export function settlementReasonByteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}

export function validSettlementReason(value: string): boolean {
  return Boolean(value.trim()) && settlementReasonByteLength(value) <= SETTLEMENT_JOB_REASON_MAX_BYTES;
}

export function isSettlementJobTerminal(job: {
  state: string;
  paid_count: number;
  excluded_count: number;
  target_count: number;
}): boolean {
  return job.state === "completed" && job.paid_count + job.excluded_count === job.target_count;
}
