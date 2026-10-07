import { afterEach, describe, expect, it } from "vitest";
import {
  clearAllCommissionPaymentIntents, classifyCommissionPaymentWriteFailure,
  commissionPaymentSessionGeneration,
  createCommissionPaymentIntent, freezeCommissionPaymentBody,
  getPendingCommissionPaymentIntent, hideCommissionPaymentIntentsForScope,
  isCommissionPaymentIntentConflict, listPendingCommissionPaymentIntents,
  markCommissionPaymentIntentConflict, setPendingCommissionPaymentIntent,
} from "./commissionPayments-state";

const actor = "33333333-3333-4333-8333-333333333333";
const brand = "11111111-1111-4111-8111-111111111111";
const otherBrand = "22222222-2222-4222-8222-222222222222";
const paymentId = "44444444-4444-4444-8444-444444444444";
const key = "intent-key-0001";
const scope = { accountId: actor, brandId: brand, operation: "retry" as const, targetId: paymentId };

afterEach(() => clearAllCommissionPaymentIntents());

describe("commission payment intent state", () => {
  it("freezes canonical bodies and retains emoji in UTF-8 byte validation", () => {
    const body = freezeCommissionPaymentBody("policy", { version: 4, enabled: true, reason: "Reviewed ✅" });
    expect(body).toEqual({ version: 4, enabled: true, reason: "Reviewed ✅" });
    expect(Object.isFrozen(body)).toBe(true);
    expect(Object.keys(body)).toEqual(["version", "enabled", "reason"]);
  });

  it("stores an unknown request only in volatile in-memory scope and returns the exact frozen key/body", () => {
    const intent = createCommissionPaymentIntent(scope, { version: 2, reason: "Retry approved failure" }, key)!;
    setPendingCommissionPaymentIntent(scope, intent);
    expect(getPendingCommissionPaymentIntent(scope)).toEqual(intent);
    expect(getPendingCommissionPaymentIntent({ ...scope, brandId: otherBrand })).toBeNull();
    expect(listPendingCommissionPaymentIntents(actor, brand)).toEqual([intent]);
  });

  it("invalidates old response authority but retains the exact scoped intent for manual recovery", () => {
    const intent = createCommissionPaymentIntent(scope, { version: 2, reason: "Retry approved failure" }, key)!;
    setPendingCommissionPaymentIntent(scope, intent);
    const generation = commissionPaymentSessionGeneration();
    hideCommissionPaymentIntentsForScope(actor, brand);
    expect(commissionPaymentSessionGeneration()).toBeGreaterThan(generation);
    expect(getPendingCommissionPaymentIntent(scope)).toEqual(intent);
    expect(listPendingCommissionPaymentIntents(actor, brand)).toEqual([intent]);
    expect(listPendingCommissionPaymentIntents(actor, otherBrand)).toEqual([]);
  });

  it("retains 409 intents as conflicted until the user explicitly clears them", () => {
    const intent = createCommissionPaymentIntent(scope, { version: 2, reason: "Review conflict" }, key)!;
    setPendingCommissionPaymentIntent(scope, intent);
    markCommissionPaymentIntentConflict(intent);
    expect(isCommissionPaymentIntentConflict(intent)).toBe(true);
    expect(getPendingCommissionPaymentIntent(scope)).toEqual(intent);
  });

  it("classifies network and server failures as unknown, 409 as review required, and 4xx as definitive", () => {
    expect(classifyCommissionPaymentWriteFailure(0)).toBe("unknown");
    expect(classifyCommissionPaymentWriteFailure(503)).toBe("unknown");
    expect(classifyCommissionPaymentWriteFailure(409)).toBe("conflict");
    expect(classifyCommissionPaymentWriteFailure(403)).toBe("definitive");
  });
});
