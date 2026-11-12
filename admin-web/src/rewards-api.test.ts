import { associationLabel } from "@lottery/shared";
import { describe, expect, it, vi } from "vitest";
import {
  createRewardsApi,
  type RewardAction,
  type RewardActionPage,
  type RewardOrder,
  type RewardOrderPage,
} from "./rewards-api";

const brand = "0199a000-0000-7000-8000-000000000001";
const otherBrand = "0199a000-0000-7000-8000-000000000002";
const actor = "0199a000-0000-7000-8000-000000000003";
const member = "0199a000-0000-7000-8000-000000000004";
const orderID = "0199a000-0000-7000-8000-000000000005";
const grantLedger = "0199a000-0000-7000-8000-000000000006";
const revokeLedger = "0199a000-0000-7000-8000-000000000007";
const audit = "0199a000-0000-7000-8000-000000000008";
const actionID = "0199a000-0000-7000-8000-000000000009";
const now = "2026-10-08T10:00:00.123456Z";
const requestID = "reward-sdk-test-request-001";

function order(overrides: Partial<RewardOrder> = {}): RewardOrder {
  return {
    id: orderID,
    brand_id: brand,
    member_id: member,
    points: "25",
    state: "granted",
    version: 1,
    grant_ledger_entry_id: grantLedger,
    revoke_ledger_entry_id: null,
    creation_audit_log_id: audit,
    last_audit_log_id: audit,

    created_by: actor,
    reason: "approved customer reward",
    point_policy_version: "1",
    created_at: now,
    updated_at: now,
    revoked_at: null,
    ...overrides,
  };
}



function revoked(overrides: Partial<RewardOrder> = {}): RewardOrder {
  return order({ state: "revoked", version: 2, revoke_ledger_entry_id: revokeLedger, revoked_at: "2026-10-08T10:01:00Z", updated_at: "2026-10-08T10:01:00Z", ...overrides });
}

function action(overrides: Partial<RewardAction> = {}): RewardAction {
  return {
    id: actionID,
    brand_id: brand,
    order_id: orderID,
    version: 1,
    operation: "grant",
    state_before: null,
    state_after: "granted",
    actor_id: actor,
    reason: "approved customer reward",
    audit_log_id: audit,
    ledger_entry_id: grantLedger,
    created_at: now,
    ...overrides,
  };
}

function orderPage(items: RewardOrder[], overrides: Partial<RewardOrderPage> = {}): RewardOrderPage {
  return { brand_id: brand, items, total_count: String(items.length), limit: 20, offset: 0, ...overrides };
}

function actionPage(items: RewardAction[], overrides: Partial<RewardActionPage> = {}): RewardActionPage {
  return { brand_id: brand, order_id: orderID, items, total_count: String(items.length), limit: 20, offset: 0, ...overrides };
}

function ok(data: unknown, status = 200): Response {
  return new Response(JSON.stringify({ success: true, data, request_id: requestID, associations: { ["member:" + member]: { name: "reward_user" } } }), { status });
}

function fail(code: string, status = 409): Response {
  return new Response(JSON.stringify({ success: false, error: { code, message: "safe server message" }, request_id: requestID }), { status });
}

describe("rewards API", () => {

  it("checks write receipts against actor, brand, target, submitted amount and exact next version", async () => {
    const grantBody = { member_id: member, points: "45", reason: "approved grant" };
    for (const receipt of [
      order({ created_by: otherBrand }), order({ brand_id: otherBrand }), order({ member_id: otherBrand }),
      order({ points: "44" }), order({ state: "revoked", version: 1, revoke_ledger_entry_id: revokeLedger, revoked_at: now }),
    ]) {
      await expect(createRewardsApi(async () => ok(receipt, 201), actor).create(brand, grantBody, "reward-grant-key-0020"))
        .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
    await expect(createRewardsApi(async () => ok(order({ id: otherBrand }))).read(brand, orderID))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(createRewardsApi(async () => ok(actionPage([action()], { order_id: otherBrand }))).actions(brand, orderID))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    for (const receipt of [
      revoked({ version: 3 }),
    ]) {
      await expect(createRewardsApi(async () => ok(receipt), actor).revoke(brand, orderID, { version: 1, reason: "request reversal" }, "reward-revoke-key-02"))
        .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
    await expect(createRewardsApi(async () => ok(order(), 200), actor).create(brand, grantBody, "reward-grant-key-0021"))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("requires page response scope and query echo, sorted unique items, and exact decimal counts", async () => {
    const newerWithLowerID = order({ id: "0199a000-0000-7000-8000-000000000004", created_at: "2026-10-08T10:00:00.123457Z", updated_at: "2026-10-08T10:00:00.123457Z" });
    const olderWithHigherID = order({ id: "0199a000-0000-7000-8000-000000000010", created_at: "2026-10-08T10:00:00.123456Z", updated_at: "2026-10-08T10:00:00.123456Z" });
    const valid = orderPage([newerWithLowerID, olderWithHigherID]);
    await expect(createRewardsApi(async () => ok(valid)).list(brand)).resolves.toMatchObject({ items: [{ id: newerWithLowerID.id }, { id: olderWithHigherID.id }] });
    const invalidPages = [
      orderPage([order()], { brand_id: otherBrand }),
      orderPage([order()], { limit: 10 }),
      orderPage([order()], { offset: 1 }),
      orderPage([order(), order()], { total_count: "2" }),
      orderPage([order({ id: "0199a000-0000-7000-8000-000000000010", created_at: "2026-10-07T10:00:00Z" }), order()]),
      orderPage([
        order({ id: "0199a000-0000-7000-8000-000000000004", created_at: "2026-10-08T10:00:00.123456Z" }),
        order({ id: "0199a000-0000-7000-8000-000000000010", created_at: "2026-10-08T12:00:00.123456+02:00" }),
      ]),
      orderPage([order()], { total_count: "0" }),
      orderPage([], { total_count: "9223372036854775808" }),
    ];
    for (const page of invalidPages) {
      await expect(createRewardsApi(async () => ok(page)).list(brand)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
    await expect(createRewardsApi(async () => ok(orderPage([], { total_count: "0", offset: 200 }))).list(brand, 20, 200))
      .resolves.toMatchObject({ items: [], offset: 200 });
    await expect(createRewardsApi(async () => ok(actionPage([], { order_id: otherBrand }))).actions(brand, orderID))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(createRewardsApi(async () => ok(actionPage([], { total_count: "0", offset: 200 }))).actions(brand, orderID, 20, 200))
      .resolves.toMatchObject({ items: [], offset: 200 });
  });

  it("preserves current structured server errors and rejects malformed envelopes", async () => {
    await expect(createRewardsApi(async () => fail("REWARD_VERSION_CONFLICT")).read(brand, orderID))
      .rejects.toMatchObject({ code: "REWARD_VERSION_CONFLICT", status: 409 });
    await expect(createRewardsApi(async () => fail("AUTH_SESSION_REVOKED", 401)).read(brand, orderID))
      .rejects.toMatchObject({ code: "AUTH_SESSION_REVOKED", status: 401 });
    await expect(createRewardsApi(async () => fail("AUTH_ACTOR_CONTEXT_CHANGED", 401)).read(brand, orderID))
      .rejects.toMatchObject({ code: "AUTH_ACTOR_CONTEXT_CHANGED", status: 401 });
    for (const malformed401 of [
      new Response("not json", { status: 401 }),
      new Response(JSON.stringify({ success: true, data: order(), request_id: requestID }), { status: 401 }),
      new Response(JSON.stringify({ success: false, error: { code: "bad code", message: "x" }, request_id: requestID }), { status: 401 }),
    ]) {
      await expect(createRewardsApi(async () => malformed401).read(brand, orderID))
        .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
    await expect(createRewardsApi(async () => fail("internal.stack.trace", 500)).read(brand, orderID))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE", status: 502 });
    await expect(createRewardsApi(async () => new Response(JSON.stringify({ success: true, data: order(), request_id: requestID, debug: "extra" })))
      .read(brand, orderID)).rejects.toMatchObject({ code: "INVALID_RESPONSE", status: 502 });
    await expect(createRewardsApi(async () => new Response("not json")).read(brand, orderID))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE", status: 502 });
    await expect(createRewardsApi(async () => { throw new Error("secret network detail"); }).read(brand, orderID))
      .rejects.toMatchObject({ code: "NETWORK_ERROR", status: 0 });
  });
});

it("retains authorized member names on validated reward reads", async () => {
  const api = createRewardsApi(vi.fn<typeof fetch>().mockResolvedValue(ok(orderPage([order({reason: ""})]))), actor);
  const result = await api.list(brand);
  expect(associationLabel(result.items[0], "member", member, "zh-CN")).toBe("reward_user");
  expect(Object.keys(result.items[0])).not.toContain("associations");
});

it('compares reward grant receipts by exact decimal value while preserving the request body', async () => {
 const body={member_id:member,points:'0.50',reason:''},data=order({points:'0.5',reason:''})
 const fetcher=vi.fn<typeof fetch>().mockResolvedValue(ok(data,201))
 await expect(createRewardsApi(fetcher,actor).create(brand,body,'reward-decimal-key')).resolves.toEqual(data)
 expect(JSON.parse(String(fetcher.mock.calls[0]![1]!.body))).toEqual(body)
 await expect(createRewardsApi(async()=>ok(order({points:'0.51'}),201),actor).create(brand,body,'reward-decimal-key')).rejects.toMatchObject({code:'INVALID_RESPONSE'})
})
