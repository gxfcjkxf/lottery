import { beforeEach, describe, expect, it } from "vitest";
import {
  classifyCommissionCorrectionWriteFailure, clearAllCommissionCorrectionIntents,
  clearPendingCommissionCorrectionIntent, commissionCorrectionContextKey,
  createCommissionCorrectionIntent, getPendingCommissionCorrectionIntent,
  isCommissionCorrectionIntentConflict, listPendingCommissionCorrectionIntents,
  markCommissionCorrectionIntentConflict, setPendingCommissionCorrectionIntent,
  type CommissionCorrectionIntentScope,
} from "./commission-corrections-state";

const actor = "33333333-3333-4333-8333-333333333333", otherActor = "88888888-8888-4888-8888-888888888888";
const brand = "11111111-1111-4111-8111-111111111111", target = "44444444-4444-4444-8444-444444444444";
const scope: CommissionCorrectionIntentScope = { actorId: actor, brandId: brand, operation: "approve", targetId: target };
beforeEach(clearAllCommissionCorrectionIntents);

describe("commission correction request state", () => {
  it("freezes a canonical copy and scopes pending requests by actor, brand, operation and target", () => {
    const source = { version: 4, reason: "Reviewed correction" };
    const intent = createCommissionCorrectionIntent(scope, source, "idem-key-0001")!;
    source.reason = "mutated";
    expect(Object.isFrozen(intent)).toBe(true);
    expect(Object.isFrozen(intent.body)).toBe(true);
    expect(intent.body.reason).toBe("Reviewed correction");
    setPendingCommissionCorrectionIntent(scope, intent);
    expect(getPendingCommissionCorrectionIntent(scope)?.body).toEqual({ version: 4, reason: "Reviewed correction" });
    expect(listPendingCommissionCorrectionIntents(actor, brand)).toHaveLength(1);
    expect(listPendingCommissionCorrectionIntents(otherActor, brand)).toHaveLength(0);
    expect(getPendingCommissionCorrectionIntent({ ...scope, operation: "continue" })).toBeNull();
    expect(commissionCorrectionContextKey(actor, brand)).not.toBe(commissionCorrectionContextKey(otherActor, brand));
  });

  it("isolates operation idempotency slots and clears only the expected original key", () => {
    const first = createCommissionCorrectionIntent(scope, { version: 2, reason: "First request" }, "idem-key-0001")!;
    const second = createCommissionCorrectionIntent({ ...scope, operation: "execute_retry" }, { version: 2, reason: "Other operation" }, "idem-key-0002")!;
    setPendingCommissionCorrectionIntent(scope, first);
    setPendingCommissionCorrectionIntent({ ...scope, operation: "execute_retry" }, second);
    expect(clearPendingCommissionCorrectionIntent(scope, "wrong-key-0000")).toBe(false);
    expect(getPendingCommissionCorrectionIntent(scope)?.key).toBe(first.key);
    expect(getPendingCommissionCorrectionIntent({ ...scope, operation: "execute_retry" })?.key).toBe(second.key);
    expect(clearPendingCommissionCorrectionIntent(scope, first.key)).toBe(true);
  });

  it("retains conflicts until explicit discard and clears every intent for logout", () => {
    const intent = createCommissionCorrectionIntent(scope, { version: 2, reason: "Version conflict" }, "idem-key-0001")!;
    setPendingCommissionCorrectionIntent(scope, intent);
    markCommissionCorrectionIntentConflict(intent);
    expect(isCommissionCorrectionIntentConflict(intent)).toBe(true);
    expect(getPendingCommissionCorrectionIntent(scope)?.key).toBe(intent.key);
    clearPendingCommissionCorrectionIntent(scope, intent.key);
    expect(getPendingCommissionCorrectionIntent(scope)).toBeNull();
    setPendingCommissionCorrectionIntent(scope, intent);
    clearAllCommissionCorrectionIntents();
    expect(listPendingCommissionCorrectionIntents(actor, brand)).toEqual([]);
  });

  it("classifies unknown outcomes, 409 conflicts, and definitive client failures", () => {
    expect(classifyCommissionCorrectionWriteFailure(undefined)).toBe("unknown");
    expect(classifyCommissionCorrectionWriteFailure(0)).toBe("unknown");
    expect(classifyCommissionCorrectionWriteFailure(503)).toBe("unknown");
    expect(classifyCommissionCorrectionWriteFailure(409)).toBe("conflict");
    expect(classifyCommissionCorrectionWriteFailure(400)).toBe("definitive");
    expect(classifyCommissionCorrectionWriteFailure(403)).toBe("definitive");
  });
});
