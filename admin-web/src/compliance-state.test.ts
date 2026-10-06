import { afterEach, describe, expect, it } from "vitest";
import { clearAllPendingComplianceIntents, clearPendingComplianceIntent, classifyComplianceFailure, complianceSessionGeneration, createComplianceRequestGuard, getPendingComplianceIntent, setPendingComplianceIntent, updatePendingCompliancePhase, type PendingComplianceIntent } from "./compliance-state";

const scope = { accountId: "00000000-0000-4000-8000-000000000001", brandId: "00000000-0000-4000-8000-000000000002" };
const policy: PendingComplianceIntent = { ...scope, kind: "policy", key: "policy-key-001", phase: "unknown", body: { version: 8, config: { age_enabled: false, minimum_age: null, region_enabled: true, allowed_countries: ["CA", "US"], identity_enabled: false }, reason: "Review regions" } };
const check: PendingComplianceIntent = { ...scope, kind: "check", key: "check-key-001", phase: "review", body: { version: 8, operation: "withdrawal", reason: "Manual simulation" } };
afterEach(() => clearAllPendingComplianceIntents());

describe("compliance intent state", () => {
  it("stores frozen detached policy and check intents per actor and brand across remounts", () => {
    const mutable = structuredClone(policy) as Extract<PendingComplianceIntent, { kind: "policy" }>;
    setPendingComplianceIntent(mutable); setPendingComplianceIntent(check);
    mutable.body.config.allowed_countries.reverse();
    const saved = getPendingComplianceIntent(scope, "policy"), savedCheck = getPendingComplianceIntent(scope, "check");
    expect(saved).toEqual(policy); expect(Object.isFrozen(saved)).toBe(true); expect(Object.isFrozen(saved?.body)).toBe(true);
    expect(Object.isFrozen((saved as Extract<PendingComplianceIntent, { kind: "policy" }>).body.config)).toBe(true);
    expect(Object.isFrozen((saved as Extract<PendingComplianceIntent, { kind: "policy" }>).body.config.allowed_countries)).toBe(true);
    expect(savedCheck).toEqual(check);
    expect(getPendingComplianceIntent({ ...scope, accountId: "00000000-0000-4000-8000-000000000003" }, "policy")).toBeNull();
    expect(getPendingComplianceIntent({ ...scope, brandId: "00000000-0000-4000-8000-000000000004" }, "check")).toBeNull();
  });

  it("retains unknown bodies and keys until explicitly resolved", () => {
    setPendingComplianceIntent({ ...check, phase: "unknown" });
    updatePendingCompliancePhase(scope, "check", "wrong-key-001", "review");
    expect(getPendingComplianceIntent(scope, "check")).toMatchObject({ key: check.key, phase: "unknown", body: check.body });
    updatePendingCompliancePhase(scope, "check", check.key, "conflict");
    clearPendingComplianceIntent(scope, "check", "wrong-key-001");
    expect(getPendingComplianceIntent(scope, "check")?.phase).toBe("conflict");
    clearPendingComplianceIntent(scope, "check", check.key);
    expect(getPendingComplianceIntent(scope, "check")).toBeNull();
  });

  it("refuses replacing unresolved intents and rejects padded reasons", () => {
    expect(setPendingComplianceIntent(policy)).toBe(true);
    expect(setPendingComplianceIntent({ ...policy, phase: "review" })).toBe(true);
    expect(getPendingComplianceIntent(scope, "policy")?.phase).toBe("unknown");
    expect(setPendingComplianceIntent({ ...policy, key: "replacement-key" })).toBe(false);
    expect(setPendingComplianceIntent({ ...policy, body: { ...policy.body, reason: " padded " } })).toBe(false);
    expect(getPendingComplianceIntent(scope, "policy")).toMatchObject({ key: policy.key, phase: "unknown", body: policy.body });
    updatePendingCompliancePhase(scope, "policy", policy.key, "conflict");
    expect(setPendingComplianceIntent({ ...policy, key: "conflict-replacement" })).toBe(false);
    expect(getPendingComplianceIntent(scope, "policy")).toMatchObject({ key: policy.key, phase: "conflict", body: policy.body });
    expect(setPendingComplianceIntent({ ...check, body: { ...check.body, reason: " padded " } })).toBe(false);
  });

  it("updates only the origin scope after the selected brand has changed", () => {
    const nextScope = { accountId: scope.accountId, brandId: "00000000-0000-4000-8000-000000000009" };
    setPendingComplianceIntent(policy);
    setPendingComplianceIntent({ ...check, ...nextScope, key: "new-scope-key-01" });
    updatePendingCompliancePhase(scope, "policy", policy.key, "unknown");
    expect(getPendingComplianceIntent(scope, "policy")?.phase).toBe("unknown");
    expect(getPendingComplianceIntent(nextScope, "check")?.phase).toBe("review");
  });

  it("classifies ambiguous outcomes and invalidates callbacks at logout", () => {
    expect(classifyComplianceFailure(undefined)).toBe("unknown"); expect(classifyComplianceFailure(0)).toBe("unknown"); expect(classifyComplianceFailure(503)).toBe("unknown");
    expect(classifyComplianceFailure(409, "COMPLIANCE_VERSION_CONFLICT")).toBe("conflict"); expect(classifyComplianceFailure(409, "COMPLIANCE_STATE_CONFLICT")).toBe("conflict");
    expect(classifyComplianceFailure(400, "COMPLIANCE_INPUT_INVALID")).toBe("definitive"); expect(classifyComplianceFailure(404, "COMPLIANCE_NOT_FOUND")).toBe("definitive"); expect(classifyComplianceFailure(403, "PERMISSION_DENIED")).toBe("definitive");
    const first = createComplianceRequestGuard(), independent = createComplianceRequestGuard();
    const firstToken = first.begin(), independentToken = independent.begin();
    expect(first.isCurrent(firstToken)).toBe(true); expect(independent.isCurrent(independentToken)).toBe(true);
    const generation = complianceSessionGeneration(); clearAllPendingComplianceIntents();
    expect(complianceSessionGeneration()).toBe(generation + 1); expect(first.isCurrent(firstToken)).toBe(false); expect(independent.isCurrent(independentToken)).toBe(false);
  });
});
