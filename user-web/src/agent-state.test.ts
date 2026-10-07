import { afterEach, describe, expect, it } from "vitest";
import type { AgentNode } from "./agent-api";
import {
  allowedChildModes,
  cacheAgentUpdateReceipt,
  clearAgentUpdateScope,
  clearAllAgentUpdates,
  clearPendingAgentUpdate,
  getAgentUpdateReceipt,
  getPendingAgentUpdate,
  rememberAgentUpdate,
} from "./agent-state";

const intent = (overrides: Partial<Parameters<typeof rememberAgentUpdate>[0]> = {}) => ({
  brandId: "11111111-1111-4111-8111-111111111111",
  memberId: "22222222-2222-4222-8222-222222222222",
  childId: "33333333-3333-4333-8333-333333333333",
  body: { version: 2, policy_version: 4, parent_version: 3, ratio: "0.1", mode: null, reason: "reviewed" },
  idempotencyKey: "stable-agent-update-1",
  ...overrides,
});

afterEach(clearAllAgentUpdates);

describe("in-memory agent update state", () => {
  it("allows a child only inheritance or its parent’s effective mode", () => {
    expect(allowedChildModes("loss")).toEqual(["loss"]);
    expect(allowedChildModes("turnover")).toEqual(["turnover"]);
    expect(allowedChildModes(null)).toEqual([]);
    expect(allowedChildModes(undefined)).toEqual([]);
  });
  it("retains the first exact body and key for an identity across remounts", () => {
    const first = rememberAgentUpdate(intent());
    const remounted = getPendingAgentUpdate(first.brandId, first.memberId, first.childId);
    expect(remounted).toBe(first);
    expect(rememberAgentUpdate(intent({ body: { ...first.body, ratio: "0.2" }, idempotencyKey: "new-operation-key" }))).toBe(first);
    expect(remounted?.body.ratio).toBe("0.1");
    expect(remounted?.idempotencyKey).toBe("stable-agent-update-1");
    expect(Object.isFrozen(remounted?.body)).toBe(true);
  });

  it("does not let a late completion clear another pending intent", () => {
    const old = rememberAgentUpdate(intent());
    clearPendingAgentUpdate(old);
    const next = rememberAgentUpdate(intent({ idempotencyKey: "stable-agent-update-2" }));
    expect(clearPendingAgentUpdate(old)).toBe(false);
    expect(getPendingAgentUpdate(next.brandId, next.memberId, next.childId)).toBe(next);
  });

  it("clears only the requested account scope", () => {
    const first = rememberAgentUpdate(intent());
    const otherBrand = rememberAgentUpdate(intent({ brandId: "44444444-4444-4444-8444-444444444444" }));
    const otherMember = rememberAgentUpdate(intent({ memberId: "55555555-5555-4555-8555-555555555555" }));
    clearAgentUpdateScope(first.brandId, first.memberId);
    expect(getPendingAgentUpdate(first.brandId, first.memberId, first.childId)).toBeNull();
    expect(getPendingAgentUpdate(otherBrand.brandId, otherBrand.memberId, otherBrand.childId)).toBe(otherBrand);
    expect(getPendingAgentUpdate(otherMember.brandId, otherMember.memberId, otherMember.childId)).toBe(otherMember);
  });

  it("keeps the first actual receipt per idempotency key as historical evidence", () => {
    const first = rememberAgentUpdate(intent());
    const receipt = { brand_id: first.brandId, member_id: first.memberId, id: first.childId, path: [first.childId], config: { ratio: "0.1", mode: null, status: "active", can_create_children: false }, updated_at: "first" } as AgentNode;
    const later = { ...receipt, updated_at: "later" } as AgentNode;
    const cached = cacheAgentUpdateReceipt(first, receipt);
    expect(cached).toMatchObject({ updated_at: "first" });
    expect(cacheAgentUpdateReceipt(first, later)).toBe(cached);
    expect(getAgentUpdateReceipt(first.brandId, first.memberId, first.childId, first.idempotencyKey)).toBe(cached);
    const next = rememberAgentUpdate(intent({ idempotencyKey: "stable-agent-update-2" }));
    const nextReceipt = cacheAgentUpdateReceipt(next, later);
    expect(getAgentUpdateReceipt(next.brandId, next.memberId, next.childId, next.idempotencyKey)).toBe(nextReceipt);
    clearAgentUpdateScope(first.brandId, first.memberId);
    expect(getAgentUpdateReceipt(first.brandId, first.memberId, first.childId, first.idempotencyKey)).toBeNull();
  });
});
