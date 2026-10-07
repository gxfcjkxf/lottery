import { beforeEach, describe, expect, it } from "vitest";
import {
  classifyCommissionWriteFailure,
  clearAllPendingCommissionWrites,
  clearPendingCommissionWrite,
  commissionPolicyBodyFingerprint,
  commissionPolicyContextKey,
  commissionPolicyScopeMatches,
  findPendingCommissionWrite,
  freezeCommissionPolicyBody,
  getPendingCommissionWrite,
  setPendingCommissionWrite,
} from "./commission-policy-state";

describe("commission policy write state", () => {
  beforeEach(clearAllPendingCommissionWrites);

  it("freezes a copied request and fingerprints object keys canonically", () => {
    const source = { version: 4, config: { enabled: false, calendar: null, payout_mode: "manual" }, reason: "审阅" };
    const frozen = freezeCommissionPolicyBody(source);
    expect(Object.isFrozen(frozen)).toBe(true);
    expect(Object.isFrozen(frozen.config)).toBe(true);
    expect(frozen).not.toBe(source);
    expect(commissionPolicyBodyFingerprint(frozen)).toBe(commissionPolicyBodyFingerprint({
      reason: "审阅",
      config: { payout_mode: "manual", calendar: null, enabled: false },
      version: 4,
    }));
    expect(source.config.enabled).toBe(false);
  });

  it("isolates page-session intents by actor and brand and supports logout clearing", () => {
    const intent = Object.freeze({ actorId: "actor-a", brandId: "brand-a", body: Object.freeze({ version: 1 }), key: "same-key" });
    const key = commissionPolicyContextKey(intent.actorId, intent.brandId);
    setPendingCommissionWrite(key, intent);
    expect(getPendingCommissionWrite(key)).toBe(intent);
    expect(getPendingCommissionWrite(commissionPolicyContextKey("actor-b", "brand-a"))).toBeNull();
    expect(findPendingCommissionWrite<typeof intent>((value) => value.key === "same-key")).toBe(intent);
    clearPendingCommissionWrite(key);
    expect(getPendingCommissionWrite(key)).toBeNull();
    setPendingCommissionWrite(key, intent);
    clearAllPendingCommissionWrites();
    expect(getPendingCommissionWrite(key)).toBeNull();
  });

  it("does not let an old success or definitive-failure callback clear a newer idempotency intent", () => {
    const key = commissionPolicyContextKey("actor-a", "brand-a");
    const oldIntent = Object.freeze({ actorId: "actor-a", brandId: "brand-a", key: "old-idempotency-key" });
    const newIntent = Object.freeze({ actorId: "actor-a", brandId: "brand-a", key: "new-idempotency-key" });

    for (const staleCallback of ["success", "definitive failure"]) {
      setPendingCommissionWrite(key, oldIntent);
      setPendingCommissionWrite(key, newIntent);
      expect(clearPendingCommissionWrite(key, oldIntent.key), staleCallback).toBe(false);
      expect(getPendingCommissionWrite(key)).toBe(newIntent);
    }
  });

  it("drops stale reads when the account, brand, permission, or component scope changes", () => {
    const request = { ticket: 8, currentTicket: 8, actorId: "actor-a", currentActorId: "actor-a", brandId: "brand-a", currentBrandId: "brand-a", canView: true, live: true };
    expect(commissionPolicyScopeMatches(request)).toBe(true);
    expect(commissionPolicyScopeMatches({ ...request, currentTicket: 9 })).toBe(false);
    expect(commissionPolicyScopeMatches({ ...request, currentBrandId: "brand-b" })).toBe(false);
    expect(commissionPolicyScopeMatches({ ...request, currentActorId: "actor-b" })).toBe(false);
    expect(commissionPolicyScopeMatches({ ...request, canView: false })).toBe(false);
    expect(commissionPolicyScopeMatches({ ...request, live: false })).toBe(false);
  });

  it("classifies conflicts, uncertain transport/server failures, and definitive client failures", () => {
    expect(classifyCommissionWriteFailure(0)).toBe("uncertain");
    expect(classifyCommissionWriteFailure(502)).toBe("uncertain");
    expect(classifyCommissionWriteFailure(503)).toBe("uncertain");
    expect(classifyCommissionWriteFailure(409)).toBe("conflict");
    expect(classifyCommissionWriteFailure(400)).toBe("definitive");
    expect(classifyCommissionWriteFailure(403)).toBe("definitive");
  });
});
