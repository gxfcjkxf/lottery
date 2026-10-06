import { describe, expect, it } from "vitest";
import {
  betMutationFailure,
  createFrozenBetMutationStore,
  freezeBetMutation,
  hasMatchingJudgmentWitness,
  settleBetMutationFailure,
} from "./bet-order-state";
import type { Judgment } from "./bet-management-api";

describe("judgment confirmation evidence", () => {
  const operation = freezeBetMutation({
    orderId: "o",
    brandId: "b",
    action: "judge-cancel",
    body: { version: 7, reason: "no result decision", cause: "no_result" },
    idempotencyKey: "original-key",
  });
  const order = {
    id: "o",
    brand_id: "b",
    version: 8,
    status: "judged_cancelled",
    refund_entry_id: "refund-1",
  };
  const evidence: Judgment = {
    id: "j",
    brand_id: "b",
    game_id: "g",
    period_id: "p",
    order_id: "o",
    order_version: 8,
    cause: "no_result",
    draw_result_id: "",
    judged_by: "actor-1",
    reason: "no result decision",
    created_at: "2026-10-06T00:00:00Z",
    refund_entry_id: "refund-1",
  };
  it("matches the resulting version, original actor, cause and exact refund", () => {
    expect(
      hasMatchingJudgmentWitness(order, operation, evidence, "actor-1"),
    ).toBe(true);
  });
  it("does not mistake whole-period cancellation or other evidence for this request", () => {
    expect(hasMatchingJudgmentWitness(order, operation, null, "actor-1")).toBe(
      false,
    );
    for (const patch of [
      { order_version: 7 },
      { judged_by: "actor-2" },
      { cause: "invalid_result" as const },
      { refund_entry_id: "other-refund" },
      { order_id: "other-order" },
      { brand_id: "other-brand" },
      { reason: "other decision" },
    ]) {
      expect(
        hasMatchingJudgmentWitness(
          order,
          operation,
          { ...evidence, ...patch },
          "actor-1",
        ),
      ).toBe(false);
    }
  });
});

describe("bet mutation failure classification", () => {
  it.each([0, 500, 502, undefined])("keeps status %s uncertain", (status) => {
    expect(betMutationFailure(status)).toBe("unknown");
  });

  it("distinguishes a conflict from other known client rejections", () => {
    expect(betMutationFailure(409)).toBe("conflict");
    expect(betMutationFailure(400)).toBe("rejected");
    expect(betMutationFailure(403)).toBe("rejected");
  });
});

describe("frozen bet mutations", () => {
  it("retains the exact body and key for same-actor retry and isolates other scopes", () => {
    const store = createFrozenBetMutationStore<{
      version: number;
      reason: string;
      cause?: "no_result" | "invalid_result";
    }>();
    const operation = freezeBetMutation({
      orderId: "order-1",
      brandId: "brand-1",
      action: "judge-cancel",
      body: {
        version: 7,
        reason: "draw result failed external validation",
        cause: "invalid_result" as const,
      },
      idempotencyKey: "same-key",
    });
    store.remember("actor-1", operation);

    expect(store.find("actor-1", "brand-1", "order-1")).toBe(operation);
    expect(store.find("actor-2", "brand-1", "order-1")).toBeNull();
    expect(store.find("actor-1", "brand-2", "order-1")).toBeNull();
    expect(store.find("actor-1", "brand-1", "order-2")).toBeNull();
    expect(Object.isFrozen(operation.body)).toBe(true);
    expect(store.find("actor-1", "brand-1", "order-1")?.body).toEqual({
      version: 7,
      reason: "draw result failed external validation",
      cause: "invalid_result",
    });
  });

  it("removes an operation after a known response", () => {
    const store = createFrozenBetMutationStore<{
      version: number;
      reason: string;
    }>();
    const operation = freezeBetMutation({
      orderId: "order-1",
      brandId: "brand-1",
      action: "mark-abnormal",
      body: { version: 2, reason: "review" },
      idempotencyKey: "old-key",
    });
    store.remember("actor-1", operation);
    store.forget("actor-1", operation);
    expect(store.find("actor-1", "brand-1", "order-1")).toBeNull();
  });

  it("forgets an old frozen judgment key after a known failure", () => {
    const store = createFrozenBetMutationStore<{
      version: number;
      reason: string;
      cause: "no_result" | "invalid_result";
    }>();
    const operation = freezeBetMutation({
      orderId: "order-1",
      brandId: "brand-1",
      action: "judge-cancel",
      body: {
        version: 3,
        reason: "no result was published",
        cause: "no_result" as const,
      },
      idempotencyKey: "old-key",
    });
    store.remember("actor-1", operation);
    const failure = settleBetMutationFailure(403, () =>
      store.forget("actor-1", operation),
    );
    expect(failure).toBe("rejected");
    expect(store.find("actor-1", "brand-1", "order-1")).toBeNull();
  });

  it.each([400, 403, 409])(
    "forgets a frozen key after known HTTP %s",
    (status) => {
      const store = createFrozenBetMutationStore<{
        version: number;
        reason: string;
        cause?: "no_result" | "invalid_result";
      }>();
      const operation = freezeBetMutation({
        orderId: "order-1",
        brandId: "brand-1",
        action: "judge-cancel",
        body: { version: 3, reason: "review", cause: "no_result" as const },
        idempotencyKey: "rejected-key",
      });
      store.remember("actor-1", operation);
      const failure = settleBetMutationFailure(status, () =>
        store.forget("actor-1", operation),
      );
      expect(failure).toBe(status === 409 ? "conflict" : "rejected");
      expect(store.find("actor-1", "brand-1", "order-1")).toBeNull();
    },
  );

  it.each([0, 503])(
    "preserves the frozen key after unknown HTTP %s",
    (status) => {
      const store = createFrozenBetMutationStore<{
        version: number;
        reason: string;
        cause: "no_result" | "invalid_result";
      }>();
      const operation = freezeBetMutation({
        orderId: "order-1",
        brandId: "brand-1",
        action: "judge-cancel",
        body: {
          version: 3,
          reason: "review",
          cause: "invalid_result" as const,
        },
        idempotencyKey: "retry-this-key",
      });
      store.remember("actor-1", operation);
      const failure = settleBetMutationFailure(status, () =>
        store.forget("actor-1", operation),
      );
      expect(failure).toBe("unknown");
      expect(store.find("actor-1", "brand-1", "order-1")).toBe(operation);
      expect(store.find("actor-1", "brand-1", "order-1")?.idempotencyKey).toBe(
        "retry-this-key",
      );
      expect(store.find("actor-1", "brand-1", "order-1")?.body.cause).toBe(
        "invalid_result",
      );
    },
  );
});
