import type { Correction, CorrectionMode, DrawArrays } from "./correction-api";

export const CORRECTION_PAGE_SIZE = 20;
export const CORRECTION_REASON_MAX_BYTES = 500;

export type CorrectionOperation = "correct" | "retry";

export interface FrozenCorrectionIntent<TBody extends object = Record<string, unknown>> {
  readonly accountId: string;
  readonly brandId: string;
  readonly periodId: string;
  readonly operation: CorrectionOperation;
  readonly resourceId: string | null;
  readonly expectedDrawResultId?: string;
  readonly expectedMode?: CorrectionMode;
  readonly body: Readonly<TBody>;
  readonly key: string;
}

/** Recursively copy enumerable JSON data; structuredClone rejects Vue proxies. */
export function copyCorrectionJson<T>(value: T): T {
  if (Array.isArray(value)) return value.map(copyCorrectionJson) as T;
  if (value && typeof value === "object") {
    return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, copyCorrectionJson(item)])) as T;
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

export function freezeCorrectionIntent<TBody extends object>(input: {
  accountId: string;
  brandId: string;
  periodId: string;
  operation: CorrectionOperation;
  resourceId?: string | null;
  expectedDrawResultId?: string;
  expectedMode?: CorrectionMode;
  body: TBody;
  key: string;
}): FrozenCorrectionIntent<TBody> {
  return deepFreeze({
    ...input,
    resourceId: input.resourceId ?? null,
    body: deepFreeze(copyCorrectionJson(input.body)),
  });
}

export function createFrozenCorrectionStore<TBody extends object>() {
  const entries = new Map<string, FrozenCorrectionIntent<TBody>>();
  const key = (accountId: string, brandId: string, periodId: string) => JSON.stringify([accountId, brandId, periodId]);
  return {
    remember(intent: FrozenCorrectionIntent<TBody>) {
      entries.set(key(intent.accountId, intent.brandId, intent.periodId), intent);
    },
    find(accountId: string, brandId: string, periodId: string) {
      return entries.get(key(accountId, brandId, periodId)) ?? null;
    },
    forget(intent: FrozenCorrectionIntent<TBody>) {
      const scope = key(intent.accountId, intent.brandId, intent.periodId);
      if (entries.get(scope) === intent) entries.delete(scope);
    },
    clear() { entries.clear(); },
  };
}

/** Uncertain write intents are memory-only and isolated by account, brand, and period. */
export const frozenCorrectionWrites = createFrozenCorrectionStore<Record<string, unknown>>();

export type CorrectionWriteFailure = "unknown" | "conflict" | "rejected";
export function correctionWriteFailure(status: number | undefined): CorrectionWriteFailure {
  if (status === undefined || status === 0 || status >= 500) return "unknown";
  if (status === 409) return "conflict";
  if (status >= 400 && status < 500) return "rejected";
  return "unknown";
}

export function createCorrectionRequestLane() {
  let generation = 0;
  return {
    begin() { generation += 1; return generation; },
    invalidate() { generation += 1; },
    isCurrent(ticket: number) { return generation === ticket; },
  };
}

export function sameCorrectionOperation(
  intent: Pick<FrozenCorrectionIntent, "accountId" | "brandId" | "periodId" | "operation" | "resourceId">,
  expected: { accountId: string; brandId: string; periodId: string; operation: CorrectionOperation; resourceId?: string | null },
): boolean {
  return intent.accountId === expected.accountId && intent.brandId === expected.brandId &&
    intent.periodId === expected.periodId && intent.operation === expected.operation &&
    intent.resourceId === (expected.resourceId ?? null);
}

export type CanonicalDrawResult = DrawArrays;

/** Lottery CSV accepts unsigned decimal integers; zero and input order are retained. */
export function parseCorrectionNumberCsv(value: string): number[] | null {
  if (!value.trim()) return [];
  const pieces = value.split(",");
  if (pieces.some(part => !/^\s*[0-9]+\s*$/.test(part))) return null;
  const numbers = pieces.map(part => Number(part.trim()));
  return numbers.every(Number.isSafeInteger) ? numbers : null;
}

export function canonicalCorrectionResult(result: CanonicalDrawResult, ordered: boolean): CanonicalDrawResult {
  const copy = copyCorrectionJson(result);
  if (!ordered) {
    copy.regular.sort((a, b) => a - b);
    copy.special.sort((a, b) => a - b);
  }
  return copy;
}

export function sameCorrectionResult(left: CanonicalDrawResult, right: CanonicalDrawResult): boolean {
  return left.regular.length === right.regular.length && left.regular.every((v, i) => v === right.regular[i]) &&
    left.special.length === right.special.length && left.special.every((v, i) => v === right.special[i]) &&
    left.digits.length === right.digits.length && left.digits.every((v, i) => v === right.digits[i]);
}

export function formatCorrectionPoints(value: string | null | undefined): string {
  if (value == null) return "—";
  if (!/^(0|[1-9][0-9]*)$/.test(value)) return value;
  return value.replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}

export function correctionReasonByteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}
export function validCorrectionReason(value: string): boolean {
  return Boolean(value.trim()) && correctionReasonByteLength(value) <= CORRECTION_REASON_MAX_BYTES;
}

export function correctionCountsMatch(correction: {
  target_count: number; pending_count: number; reversed_count: number; unchanged_count: number;
  excluded_count: number; failed_count: number;
}): boolean {
  return correction.pending_count + correction.reversed_count + correction.unchanged_count +
    correction.excluded_count + correction.failed_count === correction.target_count;
}

export function matchesCorrectionReceipt(
  receipt: Pick<Correction, "id" | "brand_id" | "period_id" | "draw_result_id" | "previous_draw_result_id" |
    "period_version" | "policy_version" | "mode" | "state" | "version" | "result" | "created_by" | "reason">,
  expected: {
    brandId: string; periodId: string; drawResultId?: string; previousDrawResultId: string; periodVersion: number;
    policyVersion: number | null; mode: CorrectionMode; result: DrawArrays;
    accountId: string; reason: string; financial: boolean;
  },
): boolean {
  return Boolean(receipt.id) && receipt.brand_id === expected.brandId && receipt.period_id === expected.periodId &&
    (!expected.drawResultId || receipt.draw_result_id === expected.drawResultId) && receipt.previous_draw_result_id === expected.previousDrawResultId &&
    receipt.period_version === expected.periodVersion + 1 && receipt.policy_version === expected.policyVersion &&
    receipt.mode === expected.mode && receipt.version === 1 &&
    receipt.state === (expected.financial ? "reversing" : "completed") &&
    receipt.created_by === expected.accountId && receipt.reason === expected.reason &&
    sameCorrectionResult(receipt.result, expected.result);
}

export function matchesCorrectionRetryReceipt(
  receipt: Pick<Correction, "id" | "version" | "state">,
  expected: { id: string; version: number },
): boolean {
  return receipt.id === expected.id && receipt.version === expected.version + 1 && receipt.state === "reversing";
}
