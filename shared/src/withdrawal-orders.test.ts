import { describe, expect, it } from "vitest";
import { isWithdrawalAvailability, isWithdrawalHistory, isWithdrawalOrder, validateWithdrawalBody, type WithdrawalOrder } from "./withdrawal-orders";
import { hasPendingWithdrawalIntent, retainWithdrawalIntent } from "./withdrawal-intents";

const ids = {
  brand: "11111111-1111-4111-8111-111111111111",
  member: "22222222-2222-4222-8222-222222222222",
  account: "33333333-3333-4333-8333-333333333333",
  order: "44444444-4444-4444-8444-444444444444",
  reserve: "55555555-5555-4555-8555-555555555555",
  release: "66666666-6666-4666-8666-666666666666",
  paid: "77777777-7777-4777-8777-777777777777",
  audit: "88888888-8888-4888-8888-888888888888",
  transition: "99999999-9999-4999-8999-999999999999",
};
const at = "2026-10-07T12:00:00Z";

function order(state: WithdrawalOrder["state"] = "reviewing", version = 1): WithdrawalOrder {
  const reviewed = state === "reviewing" ? null : at;
  const completed = ["paid", "rejected", "failed", "cancelled"].includes(state) ? at : null;
  const release_entry_id = ["rejected", "failed", "cancelled"].includes(state) ? ids.release : null;
  const paid_entry_id = state === "paid" ? ids.paid : null;
  return {
    id: ids.order, brand_id: ids.brand, member_id: ids.member, account_id: ids.account,
    points: "9007199254740993", state, version, source_allocation: [{ source: "recharge", state: "available", points: "9007199254740993" }],
    reserve_entry_id: ids.reserve, release_entry_id, paid_entry_id, cycle_from_at: null,
    cycle_from_version: "0", reserve_version: "1", created_at: at, updated_at: at,
    reviewed_at: reviewed, completed_at: completed, decision_reason: "", audit_log_id: ids.audit,
  };
}

describe("withdrawal order SDK DTO validation", () => {
  it("accepts real initial and progressed order versions with BigInt point strings", () => {
    expect(isWithdrawalOrder(order())).toBe(true);
    expect(isWithdrawalOrder({ ...order("processing", 2), reviewed_at: at })).toBe(true);
    expect(isWithdrawalOrder({ ...order("paid", 3), reviewed_at: at })).toBe(true);
    expect(isWithdrawalOrder({ ...order("cancelled", 2), reviewed_at: null })).toBe(true);
  });

  it("rejects invalid lifecycle fields, cursor relationships, and private DTO additions", () => {
    expect(isWithdrawalOrder({ ...order(), policy_snapshot: {} })).toBe(false);
    expect(isWithdrawalOrder({ ...order(), reserve_version: "0" })).toBe(false);
    expect(isWithdrawalOrder({ ...order(), cycle_from_at: at })).toBe(false);
    expect(isWithdrawalOrder({ ...order(), state: "paid", version: 2, completed_at: at, reviewed_at: at, paid_entry_id: ids.paid })).toBe(false);
    expect(isWithdrawalOrder({ ...order("processing", 2), release_entry_id: ids.release })).toBe(false);
  });

  it("requires positive int64 body allocations, unique canonical source order, and exact BigInt sum", () => {
    const body = { points: "9007199254740993", source_allocation: [{ source: "recharge" as const, state: "available" as const, points: "9007199254740993" }] };
    expect(validateWithdrawalBody(body)).toBe(true);
    expect(validateWithdrawalBody({ points: "2", source_allocation: [{ source: "winning", state: "available", points: "1" }, { source: "recharge", state: "available", points: "1" }] })).toBe(false);
    expect(validateWithdrawalBody({ points: "2", source_allocation: [{ source: "recharge", state: "available", points: "1" }, { source: "recharge", state: "available", points: "1" }] })).toBe(false);
    expect(validateWithdrawalBody({ points: "3", source_allocation: [{ source: "recharge", state: "available", points: "2" }] })).toBe(false);
    expect(validateWithdrawalBody({ points: "9223372036854775808", source_allocation: [{ source: "recharge", state: "available", points: "9223372036854775808" }] })).toBe(false);
    expect(validateWithdrawalBody({ points: "10", source_allocation: [
      { source: "recharge", state: "available", points: "1" },
      { source: "winning", state: "available", points: "2" },
      { source: "gift", state: "available", points: "3" },
      { source: "commission", state: "available", points: "4" },
    ] })).toBe(true);
    expect(validateWithdrawalBody({ points: "10", source_allocation: [
      { source: "commission", state: "available", points: "4" },
      { source: "gift", state: "available", points: "3" },
      { source: "winning", state: "available", points: "2" },
      { source: "recharge", state: "available", points: "1" },
    ] })).toBe(false);
  });

  it("accepts unordered policy source lists but enforces consistent admission flags and actor context", () => {
    const value = { actor_context: "a".repeat(64), brand_id: ids.brand, member_id: ids.member, policy_enabled: true, eligibility_configured: true, can_apply: true, reason_code: "AVAILABLE", min_points: "1", max_points: null, allowed_sources: ["gift", "recharge"], real_payments: false };
    expect(isWithdrawalAvailability(value)).toBe(true);
    expect(isWithdrawalAvailability({ ...value, can_apply: false })).toBe(false);
    expect(isWithdrawalAvailability({ ...value, actor_context: undefined })).toBe(false);
    expect(isWithdrawalAvailability({ ...value, reason_code: "WITHDRAWAL_DISABLED", policy_enabled: true, can_apply: false })).toBe(false);
    expect(isWithdrawalAvailability({ ...value, reason_code: "WITHDRAWAL_ELIGIBILITY_NOT_CONFIGURED", eligibility_configured: false, can_apply: false })).toBe(true);
    expect(isWithdrawalAvailability({ ...value, allowed_sources: ["recharge", "winning", "gift", "commission"] })).toBe(true);
  });

  it("keeps an unknown frozen request body byte-for-byte reusable when commission is added", () => {
    const original = Object.freeze({ points: "7", source_allocation: Object.freeze([
      Object.freeze({ source: "gift" as const, state: "available" as const, points: "7" }),
    ]) });
    const replayBody = original;
    expect(replayBody).toBe(original);
    expect(JSON.stringify(replayBody)).toBe('{"points":"7","source_allocation":[{"source":"gift","state":"available","points":"7"}]}');
  });

  it("validates actual history version chains, including the initial empty state and sanitized actor types", () => {
    const history = { brand_id: ids.brand, order_id: ids.order, items: [
      { id: ids.transition, version: 1, from_state: "", to_state: "reviewing", reason: "", actor_type: "user", created_at: at, audit_log_id: ids.audit },
      { id: ids.paid, version: 2, from_state: "reviewing", to_state: "processing", reason: "", actor_type: "admin", created_at: at, audit_log_id: ids.audit },
      { id: ids.release, version: 3, from_state: "processing", to_state: "paid", reason: "internal", actor_type: "system", created_at: at, audit_log_id: ids.audit },
    ] };
    expect(isWithdrawalHistory(history)).toBe(true);
    expect(isWithdrawalHistory({ ...history, items: [{ ...history.items[0], from_state: null }] })).toBe(false);
    expect(isWithdrawalHistory({ ...history, items: [{ ...history.items[0], actor_id: ids.account }] })).toBe(false);
  });
});

describe("frozen withdrawal intent scope", () => {
  it("retains on read refresh, blocks a different intent, and clears on scope change", () => {
    const intent = Object.freeze({ scope: "admin-a:brand-a:grants-a", key: "same-key", body: Object.freeze({ version: 1, reason: "review" }) });
    expect(retainWithdrawalIntent(intent, intent.scope)).toBe(intent);
    expect(hasPendingWithdrawalIntent(retainWithdrawalIntent(intent, intent.scope))).toBe(true);
    expect(retainWithdrawalIntent(intent, "admin-b:brand-a:grants-a")).toBeNull();
    expect(hasPendingWithdrawalIntent(null)).toBe(false);
  });
});
