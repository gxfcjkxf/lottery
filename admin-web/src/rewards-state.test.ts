import { afterEach, describe, expect, it } from "vitest";
import {
  clearAllRewardIntents, clearPendingRewardIntent, classifyRewardWriteFailure,
  createRewardIntent, freezeRewardBody, getPendingRewardIntent, hideRewardIntentsForScope,
  isRewardIntentConflict, listPendingRewardIntents, markRewardIntentConflict,
  rewardPermissions, rewardSessionGeneration, setPendingRewardIntent,
  type RewardIntentScope,
} from "./rewards-state";

const actor = "33333333-3333-4333-8333-333333333333";
const brand = "11111111-1111-4111-8111-111111111111";
const otherBrand = "22222222-2222-4222-8222-222222222222";
const order = "44444444-4444-4444-8444-444444444444";
const member = "55555555-5555-4555-8555-555555555555";
const otherActor = "66666666-6666-4666-8666-666666666666";
const key = "reward-intent-key-01";
const scope: RewardIntentScope = { accountId: actor, brandId: brand, operation: "retry", targetId: order };

afterEach(() => clearAllRewardIntents());

describe("reward intent state", () => {
  it("freezes exact grant and action bodies and validates decimal int64 as strings", () => {
    const grant = freezeRewardBody("grant", { member_id: member, points: "9223372036854775807", reason: "Reviewed ✅" });
    const action = freezeRewardBody("revoke", { version: 2, reason: "Customer requested reversal" });
    expect(grant).toEqual({ member_id: member, points: "9223372036854775807", reason: "Reviewed ✅" });
    expect(action).toEqual({ version: 2, reason: "Customer requested reversal" });
    expect(Object.isFrozen(grant)).toBe(true);
    expect(Object.isFrozen(action)).toBe(true);
    expect(Object.keys(grant)).toEqual(["member_id", "points", "reason"]);
    expect(Object.keys(action)).toEqual(["version", "reason"]);
    for (const points of ["0", "01", "9223372036854775808", 1, null, undefined]) {
      expect(createRewardIntent({ accountId: actor, brandId: brand, operation: "grant" }, { member_id: member, points, reason: "Bad points" } as never, key)).toBeNull();
    }
  });

  it("copies Proxy input into an immutable flat JSON body containing only the approved fields", () => {
    const source = { version: 3, reason: "Original reason" };
    const proxied = new Proxy(source, { get: (target, property, receiver) => Reflect.get(target, property, receiver) });
    const body = freezeRewardBody("retry", proxied);
    source.version = 9;
    source.reason = "Changed after freezing";
    expect(body).toEqual({ version: 3, reason: "Original reason" });
    expect(Object.isFrozen(body)).toBe(true);
    expect(body).toEqual({ version: 3, reason: "Original reason" });
    expect(createRewardIntent(scope, { version: 3, reason: "Unknown field", extra: true } as never, key)?.body).toEqual({ version: 3, reason: "Unknown field" });
    expect(createRewardIntent(scope, { version: 1, reason: "No functions", run() {} } as never, key)?.body).toEqual({ version: 1, reason: "No functions" });
    const cyclic: Record<string, unknown> = { version: 1, reason: "Cycle" };
    cyclic.self = cyclic;
    expect(createRewardIntent(scope, cyclic as never, key)?.body).toEqual({ version: 1, reason: "Cycle" });
    expect(createRewardIntent(scope, { version: 1, reason: "BigInt", amount: 1n } as never, key)?.body).toEqual({ version: 1, reason: "BigInt" });
  });

  it("isolates intent by complete account, brand, operation and optional target scope", () => {
    const intent = createRewardIntent(scope, { version: 4, reason: "Retry approved order" }, key);
    expect(intent).not.toBeNull();
    setPendingRewardIntent(scope, intent!);
    expect(getPendingRewardIntent(scope)).toEqual(intent);
    expect(listPendingRewardIntents(actor, brand)).toEqual([intent]);
    expect(getPendingRewardIntent({ ...scope, brandId: otherBrand })).toBeNull();
    expect(getPendingRewardIntent({ ...scope, accountId: otherActor })).toBeNull();
    expect(getPendingRewardIntent({ ...scope, operation: "revoke" })).toBeNull();
    expect(getPendingRewardIntent({ ...scope, targetId: member })).toBeNull();
    expect(listPendingRewardIntents(actor, otherBrand)).toEqual([]);
  });

  it("requires grant member as the target and requires an order target for actions", () => {
    const grantScope: RewardIntentScope = { accountId: actor, brandId: brand, operation: "grant" };
    const grant = createRewardIntent(grantScope, { member_id: member, points: "5", reason: "Grant points" }, key);
    expect(grant?.targetId).toBeUndefined();
    expect(grant?.body).toEqual({ member_id: member, points: "5", reason: "Grant points" });
    expect(createRewardIntent({ ...grantScope, targetId: order }, { member_id: member, points: "5", reason: "Grant points" }, key)).toBeNull();
    expect(createRewardIntent({ accountId: actor, brandId: brand, operation: "revoke" }, { version: 1, reason: "Missing order" }, key)).toBeNull();
    expect(createRewardIntent({ ...scope, targetId: "bad-id" }, { version: 1, reason: "Bad order" }, key)).toBeNull();
    for (const invalidKey of ["short", "invalid/key", "", "x".repeat(129)]) {
      expect(createRewardIntent(scope, { version: 1, reason: "Retry" }, invalidKey)).toBeNull();
    }
    for (const invalidReason of ["", " padded ", "line\nbreak", "\ud800", "界".repeat(167)]) {
      expect(createRewardIntent(scope, { version: 1, reason: invalidReason }, key)).toBeNull();
    }
  });

  it("uses only brand-scoped reward permissions and never grants mutations to super admins", () => {
    const all = { id: actor, super_admin: false, brand_ids: [brand], permissions: [], platform_permissions: [], permissions_by_brand: { [brand]: ["reward.view.brand", "reward.grant.brand", "reward.revoke.brand", "reward.retry.brand"] } };
    expect(rewardPermissions(all, brand)).toEqual({ view: true, grant: true, revoke: true, retry: true });
    expect(rewardPermissions({ ...all, permissions_by_brand: { [brand]: ["reward.grant.brand"] }, permissions: ["reward.view.platform", "reward.revoke.brand"], platform_permissions: [] }, brand))
      .toEqual({ view: false, grant: false, revoke: false, retry: false });
    expect(rewardPermissions({ ...all, super_admin: true, platform_permissions: ["reward.view.platform"], permissions_by_brand: { [brand]: ["reward.view.brand", "reward.grant.brand"] } }, brand))
      .toEqual({ view: true, grant: false, revoke: false, retry: false });
    expect(rewardPermissions({ ...all, brand_ids: [], permissions_by_brand: { [brand]: ["reward.view.brand", "reward.grant.brand"] } }, brand))
      .toEqual({ view: false, grant: false, revoke: false, retry: false });
  });

  it("prevents stale expected keys from clearing a replacement request", () => {
    const first = createRewardIntent(scope, { version: 1, reason: "First request" }, key)!;
    const second = createRewardIntent(scope, { version: 2, reason: "Newer request" }, "reward-intent-key-02")!;
    setPendingRewardIntent(scope, first);
    setPendingRewardIntent(scope, second);
    clearPendingRewardIntent(scope, first.key);
    expect(getPendingRewardIntent(scope)).toEqual(second);
    clearPendingRewardIntent(scope, second.key);
    expect(getPendingRewardIntent(scope)).toBeNull();
  });

  it("retains an in-memory intent across scope hiding and exposes conflicts until explicitly cleared", () => {
    const intent = createRewardIntent(scope, { version: 2, reason: "Resolve a 409" }, key)!;
    setPendingRewardIntent(scope, intent);
    const generation = rewardSessionGeneration();
    hideRewardIntentsForScope(actor, brand);
    expect(rewardSessionGeneration()).toBeGreaterThan(generation);
    expect(getPendingRewardIntent(scope)).toEqual(intent);
    markRewardIntentConflict(intent);
    expect(isRewardIntentConflict(intent)).toBe(true);
    clearPendingRewardIntent(scope, intent.key);
    expect(isRewardIntentConflict(intent)).toBe(false);
    expect(getPendingRewardIntent(scope)).toBeNull();
  });

  it("classifies transport uncertainty, conflict and definitive client rejection", () => {
    expect(classifyRewardWriteFailure(undefined)).toBe("unknown");
    expect(classifyRewardWriteFailure(0)).toBe("unknown");
    expect(classifyRewardWriteFailure(503)).toBe("unknown");
    expect(classifyRewardWriteFailure(409)).toBe("conflict");
    expect(classifyRewardWriteFailure(400)).toBe("definitive");
    expect(classifyRewardWriteFailure(403)).toBe("definitive");
  });
});
