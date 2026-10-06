import type { AdminBetOrder, Judgment } from "./bet-management-api";
export const BET_ORDER_PAGE_SIZE = 50;
export const BET_COMBINATION_PREVIEW_LIMIT = 20;
export const BET_REASON_MAX_UTF8_BYTES = 500;

export function isUuid(value: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
    value,
  );
}

export function reasonByteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}

export function validBetReason(value: string): boolean {
  return (
    Boolean(value.trim()) &&
    reasonByteLength(value) <= BET_REASON_MAX_UTF8_BYTES
  );
}

export function combinationPreview<T>(items: T[]) {
  return {
    items: items.slice(0, BET_COMBINATION_PREVIEW_LIMIT),
    total: items.length,
    remaining: Math.max(0, items.length - BET_COMBINATION_PREVIEW_LIMIT),
  };
}

/** An uncertain write retains its submitted body and key until explicitly resolved. */
export interface FrozenBetMutation<TBody> {
  readonly orderId: string;
  readonly brandId: string;
  readonly action: "cancel" | "mark-abnormal" | "judge-cancel";
  readonly body: Readonly<TBody>;
  readonly idempotencyKey: string;
}

export function hasMatchingJudgmentWitness(
  order: Pick<
    AdminBetOrder,
    "id" | "brand_id" | "version" | "status" | "refund_entry_id"
  >,
  operation: FrozenBetMutation<{
    version: number;
    reason: string;
    cause?: string;
  }>,
  evidence: Judgment | null,
  actorId: string,
): boolean {
  return (
    operation.action === "judge-cancel" &&
    order.status === "judged_cancelled" &&
    order.id === operation.orderId &&
    order.brand_id === operation.brandId &&
    order.version === operation.body.version + 1 &&
    Boolean(order.refund_entry_id) &&
    evidence !== null &&
    evidence.brand_id === order.brand_id &&
    evidence.order_id === order.id &&
    evidence.order_version === order.version &&
    evidence.judged_by === actorId &&
    evidence.refund_entry_id === order.refund_entry_id &&
    evidence.reason === operation.body.reason &&
    evidence.cause === operation.body.cause
  );
}

export type BetMutationFailure = "unknown" | "conflict" | "rejected";

export function betMutationFailure(
  status: number | undefined,
): BetMutationFailure {
  if (status === undefined || status === 0 || status >= 500) return "unknown";
  if (status === 409) return "conflict";
  if (status >= 400 && status < 500) return "rejected";
  return "unknown";
}

export function settleBetMutationFailure(
  status: number | undefined,
  forgetFrozen: () => void,
): BetMutationFailure {
  const failure = betMutationFailure(status);
  if (failure !== "unknown") forgetFrozen();
  return failure;
}

export function createFrozenBetMutationStore<TBody extends object>() {
  const entries = new Map<string, FrozenBetMutation<TBody>>();
  const key = (
    actorId: string,
    brandId: string,
    orderId: string,
    action: FrozenBetMutation<TBody>["action"],
  ) => JSON.stringify([actorId, brandId, orderId, action]);
  return {
    remember(actorId: string, operation: FrozenBetMutation<TBody>) {
      entries.set(
        key(actorId, operation.brandId, operation.orderId, operation.action),
        operation,
      );
    },
    find(actorId: string, brandId: string, orderId: string) {
      for (const action of [
        "cancel",
        "mark-abnormal",
        "judge-cancel",
      ] as const) {
        const entry = entries.get(key(actorId, brandId, orderId, action));
        if (entry) return entry;
      }
      return null;
    },
    forget(actorId: string, operation: FrozenBetMutation<TBody>) {
      entries.delete(
        key(actorId, operation.brandId, operation.orderId, operation.action),
      );
    },
  };
}

export function freezeBetMutation<TBody extends object>(input: {
  orderId: string;
  brandId: string;
  action: FrozenBetMutation<TBody>["action"];
  body: TBody;
  idempotencyKey: string;
}): FrozenBetMutation<TBody> {
  return Object.freeze({
    ...input,
    body: Object.freeze({ ...input.body }),
  });
}
