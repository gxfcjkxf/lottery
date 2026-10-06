import { describe, expect, it } from "vitest";
import {
  betMutationFailure,
  createFrozenBetMutationStore,
  freezeBetMutation,
  settleBetMutationFailure,
} from "./bet-order-state";

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
    }>();
    const operation = freezeBetMutation({
      orderId: "order-1",
      brandId: "brand-1",
      action: "cancel",
      body: { version: 7, reason: "duplicate entry" },
      idempotencyKey: "same-key",
    });
    store.remember("actor-1", operation);

    expect(store.find("actor-1", "brand-1", "order-1")).toBe(operation);
    expect(store.find("actor-2", "brand-1", "order-1")).toBeNull();
    expect(store.find("actor-1", "brand-2", "order-1")).toBeNull();
    expect(store.find("actor-1", "brand-1", "order-2")).toBeNull();
    expect(Object.isFrozen(operation.body)).toBe(true);
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

  it.each([400, 403, 409])(
    "forgets a frozen key after known HTTP %s",
    (status) => {
      const store = createFrozenBetMutationStore<{
        version: number;
        reason: string;
      }>();
      const operation = freezeBetMutation({
        orderId: "order-1",
        brandId: "brand-1",
        action: "cancel",
        body: { version: 3, reason: "review" },
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
      }>();
      const operation = freezeBetMutation({
        orderId: "order-1",
        brandId: "brand-1",
        action: "cancel",
        body: { version: 3, reason: "review" },
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
    },
  );
});
