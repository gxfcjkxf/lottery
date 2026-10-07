import { beforeEach, describe, expect, it } from "vitest";
import {
  clearAllPendingCommissionCycleWrites, clearPendingCommissionCycleWrite, commissionCycleSessionGeneration,
  commissionCycleWriteScopeKey, classifyCommissionCycleWriteFailure, freezeCommissionCycleBody,
  getPendingCommissionCycleWrite, listPendingCommissionCycleWrites, setPendingCommissionCycleWrite,
  type CommissionWriteScope, type PendingCommissionCycleWrite,
} from "./commission-cycles-state";

const actor = "11111111-1111-4111-8111-111111111111", brand = "22222222-2222-4222-8222-222222222222";
const id = "33333333-3333-4333-8333-333333333333", other = "44444444-4444-4444-8444-444444444444";
const key = "55555555-5555-4555-8555-555555555555";
const scope: CommissionWriteScope = { accountId: actor, brandId: brand, operation: "create" };
const intent = (): PendingCommissionCycleWrite => ({ ...scope, key, body: { anchor_order_id: id, reason: "owned cycle registration" } });

describe("commission page-session frozen intents", () => {
  beforeEach(clearAllPendingCommissionCycleWrites);
  it("copies and freezes body in stable backend fingerprint order", () => {
    const value = intent();
    setPendingCommissionCycleWrite(scope, value);
    (value.body as { reason: string }).reason = "mutated form";
    expect(getPendingCommissionCycleWrite(scope)?.body.reason).toBe("owned cycle registration");
    expect(Object.isFrozen(getPendingCommissionCycleWrite(scope))).toBe(true);
    expect(Object.isFrozen(getPendingCommissionCycleWrite(scope)?.body)).toBe(true);
    expect(JSON.stringify(freezeCommissionCycleBody({ reason: "x", anchor_order_id: id }))).toBe(`{"anchor_order_id":"${id}","reason":"x"}`);
  });
  it("isolates account, brand, operation and target; navigation alone keeps intent", () => {
    setPendingCommissionCycleWrite(scope, intent());
    expect(getPendingCommissionCycleWrite({ ...scope, accountId: other })).toBeNull();
    expect(getPendingCommissionCycleWrite({ ...scope, brandId: other })).toBeNull();
    expect(getPendingCommissionCycleWrite({ ...scope, operation: "retry_cycle", targetId: id })).toBeNull();
    expect(listPendingCommissionCycleWrites(actor, brand)).toHaveLength(1);
    expect(listPendingCommissionCycleWrites(other, brand)).toHaveLength(0);
    expect(commissionCycleWriteScopeKey({ ...scope, operation: "retry_cycle", targetId: id })).not.toBe(commissionCycleWriteScopeKey({ ...scope, operation: "retry_discovery", targetId: id }));
  });
  it("old completion cannot clear a newer request in the same scope", () => {
    setPendingCommissionCycleWrite(scope, intent());
    setPendingCommissionCycleWrite(scope, { ...intent(), key: other });
    clearPendingCommissionCycleWrite(scope, key);
    expect(getPendingCommissionCycleWrite(scope)?.key).toBe(other);
    clearPendingCommissionCycleWrite(scope, other);
    expect(getPendingCommissionCycleWrite(scope)).toBeNull();
  });
  it("session clear changes generation and removes every scope", () => {
    setPendingCommissionCycleWrite(scope, intent());
    const generation = commissionCycleSessionGeneration();
    clearAllPendingCommissionCycleWrites();
    expect(commissionCycleSessionGeneration()).toBe(generation + 1);
    expect(listPendingCommissionCycleWrites(actor, brand)).toEqual([]);
  });
  it("rejects mismatched intent shapes and unsafe versions/reasons", () => {
    for (const bad of [
      { ...intent(), key: "tiny" }, { ...intent(), brandId: other },
      { ...intent(), body: { anchor_order_id: id, reason: " padded " } },
      { ...intent(), body: { anchor_order_id: id, reason: "\ud800" } },
      { ...intent(), body: { anchor_order_id: id, reason: "a".repeat(501) } },
      { ...intent(), body: { anchor_order_id: id, reason: "ok", version: 1 } },
    ]) { setPendingCommissionCycleWrite(scope, bad); expect(getPendingCommissionCycleWrite(scope)).toBeNull(); }
    const retry: CommissionWriteScope = { ...scope, operation: "retry_cycle", targetId: id };
    for (const version of [0, -1, 1.1, Number.MAX_SAFE_INTEGER]) {
      setPendingCommissionCycleWrite(retry, { ...retry, key, body: { version, reason: "retry" } });
      expect(getPendingCommissionCycleWrite(retry)).toBeNull();
    }
    setPendingCommissionCycleWrite(retry, { ...retry, key, body: { version: 2, reason: "retry" } });
    expect(getPendingCommissionCycleWrite(retry)?.body).toEqual({ version: 2, reason: "retry" });
  });
  it("separates conflicts from unknown writes and definitive refusals", () => {
    for (const status of [undefined, 0, 500, 502, 503]) expect(classifyCommissionCycleWriteFailure(status)).toBe("unknown");
    expect(classifyCommissionCycleWriteFailure(409)).toBe("conflict");
    for (const status of [400, 401, 403, 404, 415, 429]) expect(classifyCommissionCycleWriteFailure(status)).toBe("definitive");
  });
});
