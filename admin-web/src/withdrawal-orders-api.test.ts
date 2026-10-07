import { describe, expect, it, vi } from "vitest";
import type { WithdrawalAction, WithdrawalOrder, WithdrawalState } from "@lottery/shared";
import { createWithdrawalOrdersAdminApi, withdrawalPermissions, WithdrawalAdminUnknownIntentError } from "./withdrawal-orders-api";

const brand = "11111111-1111-4111-8111-111111111111";
const orderId = "22222222-2222-4222-8222-222222222222";
const at = "2026-10-07T12:00:00Z";
const states: Record<WithdrawalAction, WithdrawalState> = { approve: "processing", reject: "rejected", cancel: "cancelled", fail: "failed", "mark-paid": "paid" };
function receipt(state: WithdrawalState, version: number): WithdrawalOrder {
  const finished = ["paid", "rejected", "failed", "cancelled"].includes(state);
  return {
    id: orderId, brand_id: brand, member_id: "33333333-3333-4333-8333-333333333333", account_id: "44444444-4444-4444-8444-444444444444", points: "9007199254740993", state, version,
    source_allocation: [{ source: "recharge", state: "available", points: "9007199254740993" }], reserve_entry_id: "55555555-5555-4555-8555-555555555555",
    release_entry_id: ["rejected", "failed", "cancelled"].includes(state) ? "66666666-6666-4666-8666-666666666666" : null,
    paid_entry_id: state === "paid" ? "77777777-7777-4777-8777-777777777777" : null, cycle_from_at: null, cycle_from_version: "0", reserve_version: "1", created_at: at, updated_at: at,
    reviewed_at: state === "reviewing" || state === "cancelled" && version === 2 ? null : at, completed_at: finished ? at : null, decision_reason: "", audit_log_id: "88888888-8888-4888-8888-888888888888",
  };
}
const ok = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { status: 200 });
const account = { id: "admin", super_admin: false, brand_ids: [brand], permissions: [], permissions_by_brand: {} as Record<string, string[]>, platform_permissions: [] as string[] };

describe("admin withdrawal permissions", () => {
  it("keeps read and each state action as independent explicit grants", () => {
    const writeOnly = { ...account, permissions_by_brand: { [brand]: ["withdrawal.approve.brand", "withdrawal.mark_paid.brand"] } };
    expect(withdrawalPermissions(writeOnly, brand)).toMatchObject({ view: false, actions: { approve: true, reject: false, cancel: false, fail: false, "mark-paid": true } });
    const readOnly = { ...account, permissions_by_brand: { [brand]: ["withdrawal.view.brand"] } };
    expect(withdrawalPermissions(readOnly, brand).view).toBe(true);
    expect(Object.values(withdrawalPermissions(readOnly, brand).actions).every((granted) => !granted)).toBe(true);
  });

  it("rejects forged brand grants without membership and invalid brand identifiers", () => {
    const forged = { ...account, brand_ids: [], permissions_by_brand: { [brand]: ["withdrawal.view.brand", "withdrawal.approve.brand", "withdrawal.mark_paid.brand"] } };
    expect(withdrawalPermissions(forged, brand)).toMatchObject({ view: false, actions: { approve: false, "mark-paid": false } });
    expect(withdrawalPermissions({ ...account, permissions_by_brand: { [brand]: ["withdrawal.view.brand"] } }, "not-a-uuid").view).toBe(false);
  });

  it("allows explicit platform view without membership while super admins never gain writes", () => {
    const platform = { ...account, super_admin: true, brand_ids: [], platform_permissions: ["withdrawal.view.platform"] };
    expect(withdrawalPermissions(platform, brand).view).toBe(true);
    expect(Object.values(withdrawalPermissions(platform, brand).actions).every((granted) => !granted)).toBe(true);
  });
});

describe("admin withdrawal action SDK", () => {
  it("sends the brand UUID and exact frozen action body, then verifies each target state", async () => {
    for (const action of Object.keys(states) as WithdrawalAction[]) {
      const expected = receipt(states[action], action === "approve" || action === "reject" || action === "cancel" ? 2 : 3);
      const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(expected));
      const api = createWithdrawalOrdersAdminApi(fetcher);
      const body = { version: action === "fail" || action === "mark-paid" ? 2 : 1, reason: "confirmed internal review" };
      const result = await api.action(brand, orderId, action, body, "same-idempotency-key");
      expect(result).toEqual(expected);
      const [url, init] = fetcher.mock.calls[0]!;
      expect(url).toBe(`/api/v1/admin/withdrawals/${orderId}/${action}`);
      expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
      expect(new Headers(init?.headers).get("Idempotency-Key")).toBe("same-idempotency-key");
      expect(JSON.parse(String(init?.body))).toEqual(body);
    }
  });

  it("keeps malformed or mismatched 200 action receipts unknown instead of acknowledging them", async () => {
    const variants = [
      { ...receipt("processing", 2), id: "99999999-9999-4999-8999-999999999999" },
      { ...receipt("processing", 2), brand_id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" },
      { ...receipt("processing", 2), version: 3 },
      receipt("reviewing", 1),
      { success: true },
    ];
    for (const data of variants) {
      const api = createWithdrawalOrdersAdminApi(vi.fn<typeof fetch>().mockResolvedValue(ok(data)));
      await expect(api.action(brand, orderId, "approve", { version: 1, reason: "review" }, "frozen-key")).rejects.toBeInstanceOf(WithdrawalAdminUnknownIntentError);
    }
  });

  it("preserves the pending action for unreadable gateway errors or contradictory 200 failures", async () => {
    for(const result of [new Response("gateway response",{status:503}),new Response("proxy rejection",{status:400}),new Response(JSON.stringify({success:false,error:{code:"FAIL",message:"contradictory response"}}),{status:200})]) {
      const api=createWithdrawalOrdersAdminApi(vi.fn<typeof fetch>().mockResolvedValue(result));
      await expect(api.action(brand,orderId,"approve",{version:1,reason:"review"},"frozen-gateway-key")).rejects.toBeInstanceOf(WithdrawalAdminUnknownIntentError);
    }
  });
});
