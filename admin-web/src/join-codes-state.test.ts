import { beforeEach, describe, expect, it } from "vitest";
import {
  classifyJoinCodeWriteFailure, clearAllPendingJoinCodeWrites, clearPendingJoinCodeWrite,
  freezeJoinCodeBody, getPendingJoinCodeWrite, scopeKey, setPendingJoinCodeWrite, type PendingJoinCodeWrite,
} from "./join-codes-state";

const accountId = "00000000-0000-4000-8000-000000000001";
const otherAccount = "00000000-0000-4000-8000-000000000002";
const brandId = "00000000-0000-4000-8000-000000000003";
const otherBrand = "00000000-0000-4000-8000-000000000004";
const owner = "00000000-0000-4000-8000-000000000005";
const agent = "00000000-0000-4000-8000-000000000006";
const codeId = "00000000-0000-4000-8000-000000000007";
const code = "0123456789ABCDEF01234567";
const at = "2026-10-06T04:00:00.123456Z";
const scope = { accountId, brandId };
function createIntent(overrides: Partial<PendingJoinCodeWrite> = {}): PendingJoinCodeWrite {
  return { ...scope, operation: "create", resourceId: null, identity: { kind: "agent", code: null, owner_member_id: owner, agent_id: agent },
    body: { kind: "agent", owner_member_id: owner, agent_id: agent, starts_at: at, expires_at: null, reason: "new campaign" }, key: "create-key", ...overrides };
}
function updateIntent(overrides: Partial<PendingJoinCodeWrite> = {}): PendingJoinCodeWrite {
  return { ...scope, operation: "update", resourceId: codeId, identity: { kind: "agent", code, owner_member_id: owner, agent_id: agent },
    body: { version: 3, status: "disabled", starts_at: null, expires_at: at, reason: "retire campaign" }, key: "update-key", ...overrides };
}
beforeEach(() => clearAllPendingJoinCodeWrites());

describe("join code write state", () => {
  it("freezes detached proxy-safe body and keeps create/update intents in account-brand memory", () => {
    const proxy = new Proxy({ nested: { value: "original" } }, {});
    const body = freezeJoinCodeBody(proxy);
    proxy.nested.value = "changed";
    expect(body).toEqual({ nested: { value: "original" } });
    expect(Object.isFrozen(body)).toBe(true); expect(Object.isFrozen(body.nested)).toBe(true);
    setPendingJoinCodeWrite(scope, createIntent()); setPendingJoinCodeWrite({ accountId, brandId: otherBrand }, updateIntent({ brandId: otherBrand }));
    expect(getPendingJoinCodeWrite(scope)).toEqual(createIntent());
    expect(getPendingJoinCodeWrite(scopeKey(accountId, brandId))).toEqual(createIntent());
    expect(getPendingJoinCodeWrite({ accountId: otherAccount, brandId })).toBeNull();
    expect(getPendingJoinCodeWrite({ accountId, brandId: otherBrand })).toMatchObject({ operation: "update" });
  });

  it("rejects malformed, foreign scope, or mismatched immutable identity intents", () => {
    setPendingJoinCodeWrite(scope, createIntent({ accountId: otherAccount }));
    expect(getPendingJoinCodeWrite(scope)).toBeNull();
    setPendingJoinCodeWrite(scope, createIntent({ body: { version: 3 } as PendingJoinCodeWrite["body"] }));
    expect(getPendingJoinCodeWrite(scope)).toBeNull();
    setPendingJoinCodeWrite(scope, createIntent({ identity: { kind: "referral", code: null, owner_member_id: owner, agent_id: null } }));
    expect(getPendingJoinCodeWrite(scope)).toBeNull();
    setPendingJoinCodeWrite(scope, updateIntent({ identity: { kind: "agent", code: "invalid", owner_member_id: owner, agent_id: agent } }));
    expect(getPendingJoinCodeWrite(scope)).toBeNull();
    setPendingJoinCodeWrite(scope, updateIntent({ resourceId: null }));
    expect(getPendingJoinCodeWrite(scope)).toBeNull();
    setPendingJoinCodeWrite(scope, updateIntent());
    const restored = getPendingJoinCodeWrite(scope);
    expect(restored).toMatchObject({ operation: "update", resourceId: codeId });
    expect(Object.isFrozen(restored)).toBe(true); expect(Object.isFrozen(restored?.body)).toBe(true);
  });

  it("retains frozen intent through read-only access and only clears matching acknowledgements", () => {
    setPendingJoinCodeWrite(scope, updateIntent());
    const original = getPendingJoinCodeWrite(scope)!;
    clearPendingJoinCodeWrite(scope, "other-key");
    expect(getPendingJoinCodeWrite(scope)).toEqual(original);
    clearPendingJoinCodeWrite(scope, original.key);
    expect(getPendingJoinCodeWrite(scope)).toBeNull();
  });

  it("isolates late acknowledgements after auth-scope reset and classifies unknown outcomes", () => {
    const first = createIntent(); const replacement = createIntent({ key: "replacement" });
    setPendingJoinCodeWrite(scope, first); clearAllPendingJoinCodeWrites(); setPendingJoinCodeWrite(scope, replacement);
    clearPendingJoinCodeWrite(scope, first.key);
    expect(getPendingJoinCodeWrite(scope)).toEqual(replacement);
    clearAllPendingJoinCodeWrites(); expect(getPendingJoinCodeWrite(scope)).toBeNull();
    expect(classifyJoinCodeWriteFailure(undefined)).toBe("uncertain");
    expect(classifyJoinCodeWriteFailure(0)).toBe("uncertain");
    expect(classifyJoinCodeWriteFailure(503)).toBe("uncertain");
    expect(classifyJoinCodeWriteFailure(409)).toBe("definitive");
    expect(classifyJoinCodeWriteFailure(403)).toBe("definitive");
  });
});
