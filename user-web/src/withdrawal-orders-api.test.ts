import { describe, expect, it, vi } from "vitest";
import { createWithdrawalOrdersApi, WithdrawalUnknownIntentError, withdrawalBasePath } from "./withdrawal-orders-api";

const id = "11111111-1111-4111-8111-111111111111";
const at = "2026-10-07T12:00:00Z";
const body = { points: "9007199254740993", source_allocation: [{ source: "recharge" as const, state: "available" as const, points: "9007199254740993" }] };
const receipt = {
  id, brand_id: "22222222-2222-4222-8222-222222222222", member_id: "33333333-3333-4333-8333-333333333333", account_id: "44444444-4444-4444-8444-444444444444",
  points: body.points, state: "reviewing", version: 1, source_allocation: body.source_allocation, reserve_entry_id: "55555555-5555-4555-8555-555555555555",
  release_entry_id: null, paid_entry_id: null, cycle_from_at: null, cycle_from_version: "0", reserve_version: "1", created_at: at, updated_at: at,
  reviewed_at: null, completed_at: null, decision_reason: "", audit_log_id: "66666666-6666-4666-8666-666666666666",
};
const response = (data: unknown, status = 201) => new Response(JSON.stringify({ success: true, data }), { status });

describe("user withdrawal orders SDK", () => {
  it("uses the brand alias path and posts only the exact allocation body with both required headers", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response(receipt));
    const api = createWithdrawalOrdersApi({ brandCode: "north star", fetcher });
    const result = await api.create(body, "idempotency-key", "a".repeat(64));
    expect(result).toEqual(receipt);
    const [url, init] = fetcher.mock.calls[0]!;
    expect(url).toBe("/api/v1/b/north%20star/withdrawals");
    expect(withdrawalBasePath("north star")).toBe("/api/v1/b/north%20star");
    expect(init?.credentials).toBe("same-origin");
    expect(new Headers(init?.headers).get("Idempotency-Key")).toBe("idempotency-key");
    expect(new Headers(init?.headers).get("X-Withdrawal-Actor-Context")).toBe("a".repeat(64));
    expect(new Headers(init?.headers).get("Authorization")).toBeNull();
    expect(JSON.parse(String(init?.body))).toEqual(body);
    expect(JSON.stringify(init?.body)).not.toMatch(/member_id|client_key|qualification|eligibility/i);
  });

  it("treats malformed, wrong-status, or cross-scope 201 receipts as unknown intent", async () => {
    for (const [raw, status] of [[{ success: true, data: { ...receipt, points: "1" } }, 201], [{ success: true, data: { ...receipt, state: "paid", version: 3 } }, 201], [{ success: true, data: receipt }, 200]] as const) {
      const api = createWithdrawalOrdersApi({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify(raw), { status })) });
      await expect(api.create(body, "frozen-key", "b".repeat(64))).rejects.toBeInstanceOf(WithdrawalUnknownIntentError);
    }
  });

  it("never converts a server-side actor-context rejection to a success", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: false, error: { code: "WITHDRAWAL_CONFIRMATION_ACCOUNT_CHANGED", message: "context changed" } }), { status: 403 }));
    await expect(createWithdrawalOrdersApi({ fetcher }).create(body, "frozen-key", "c".repeat(64))).rejects.toMatchObject({ code: "WITHDRAWAL_CONFIRMATION_ACCOUNT_CHANGED", status: 403 });
  });

  it("keeps non-JSON gateway failures and unrecognized rejections unknown", async () => {
    for(const result of [new Response("gateway response",{status:503}),new Response("proxy rejection",{status:400}),new Response(JSON.stringify(receipt),{status:201})]) {
      const api=createWithdrawalOrdersApi({fetcher:vi.fn<typeof fetch>().mockResolvedValue(result)});
      await expect(api.create(body,"frozen-gateway-key","a".repeat(64))).rejects.toBeInstanceOf(WithdrawalUnknownIntentError);
    }
  });
});
