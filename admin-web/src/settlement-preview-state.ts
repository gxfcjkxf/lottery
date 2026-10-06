export const SETTLEMENT_PREVIEW_PAGE_SIZE = 20;
export const SETTLEMENT_PREVIEW_REASON_MAX_BYTES = 500;
export interface SettlementPreviewRequestBody {
  version: number;
  period_version: number;
  draw_result_id: string;
  reason: string;
}

export interface PreviewReceiptFacts {
  id: string;
  brand_id: string;
  order_id: string;
  game_id: string;
  period_id: string;
  order_version: number;
  period_version: number;
  draw_result_id: string;
  definition_hash: string;
  created_by: string;
  reason: string;
  applied: boolean;
}

export function matchesPreviewReceipt(
  receipt: PreviewReceiptFacts,
  expected: {
    brandId: string;
    orderId: string;
    gameId: string;
    periodId: string;
    orderVersion: number;
    periodVersion: number;
    drawResultId: string;
    definitionHash: string;
    accountId: string;
    reason: string;
  },
): boolean {
  return (
    Boolean(receipt.id) &&
    receipt.brand_id === expected.brandId &&
    receipt.order_id === expected.orderId &&
    receipt.game_id === expected.gameId &&
    receipt.period_id === expected.periodId &&
    receipt.order_version === expected.orderVersion &&
    receipt.period_version === expected.periodVersion &&
    receipt.draw_result_id === expected.drawResultId &&
    receipt.definition_hash === expected.definitionHash &&
    receipt.created_by === expected.accountId &&
    receipt.reason === expected.reason &&
    receipt.applied === false
  );
}

export function createPreviewRequestLane() {
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

export interface FrozenPreviewIntent<TBody extends object = Record<string, unknown>> {
  readonly accountId: string;
  readonly brandId: string;
  readonly orderId: string;
  readonly body: Readonly<TBody>;
  readonly key: string;
}

function cloneValue<T>(value: T): T {
  if (typeof structuredClone === "function") return structuredClone(value);
  if (Array.isArray(value)) return value.map(cloneValue) as T;
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value).map(([key, item]) => [key, cloneValue(item)]),
    ) as T;
  }
  return value;
}

function deepFreeze<T>(value: T): T {
  if (value && typeof value === "object" && !Object.isFrozen(value)) {
    Object.freeze(value);
    for (const item of Object.values(value)) deepFreeze(item);
  }
  return value;
}

export function freezePreviewIntent<TBody extends object>(input: {
  accountId: string;
  brandId: string;
  orderId: string;
  body: TBody;
  key: string;
}): FrozenPreviewIntent<TBody> {
  return deepFreeze({
    ...input,
    body: deepFreeze(cloneValue(input.body)),
  });
}

/** In-memory only: unknown preview writes survive a component remount in this tab. */
export function createFrozenPreviewStore<TBody extends object>() {
  const entries = new Map<string, FrozenPreviewIntent<TBody>>();
  const scopeKey = (accountId: string, brandId: string, orderId: string) =>
    JSON.stringify([accountId, brandId, orderId]);
  return {
    remember(intent: FrozenPreviewIntent<TBody>) {
      entries.set(scopeKey(intent.accountId, intent.brandId, intent.orderId), intent);
    },
    find(accountId: string, brandId: string, orderId: string) {
      return entries.get(scopeKey(accountId, brandId, orderId)) ?? null;
    },
    forget(intent: FrozenPreviewIntent<TBody>) {
      const key = scopeKey(intent.accountId, intent.brandId, intent.orderId);
      if (entries.get(key) === intent) entries.delete(key);
    },
    clear() {
      entries.clear();
    },
  };
}

/** Shared only by mounted code in this page session; it never uses browser storage. */
export const frozenSettlementPreviewWrites =
  createFrozenPreviewStore<SettlementPreviewRequestBody>();

export type PreviewWriteFailure = "unknown" | "conflict" | "rejected";

export function previewWriteFailure(status: number | undefined): PreviewWriteFailure {
  if (status === undefined || status === 0 || status >= 500) return "unknown";
  if (status === 409) return "conflict";
  if (status >= 400 && status < 500) return "rejected";
  return "unknown";
}

export function samePreviewContext(
  context: {
    brand_id: string;
    order_id: string;
    order_version: number;
  },
  expected: { brandId: string; orderId: string; orderVersion: number },
): boolean {
  return (
    context.brand_id === expected.brandId &&
    context.order_id === expected.orderId &&
    context.order_version === expected.orderVersion
  );
}

/** Group a non-negative integer decimal string without converting it to Number. */
export function formatIntegerPoints(value: string): string {
  const match = /^(-?)(\d+)(\.\d+)?$/.exec(value);
  if (!match) return value;
  return `${match[1]}${match[2].replace(/\B(?=(\d{3})+(?!\d))/g, ",")}${match[3] ?? ""}`;
}

export function reasonByteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}

export function validPreviewReason(value: string): boolean {
  return (
    Boolean(value.trim()) &&
    reasonByteLength(value) <= SETTLEMENT_PREVIEW_REASON_MAX_BYTES
  );
}
