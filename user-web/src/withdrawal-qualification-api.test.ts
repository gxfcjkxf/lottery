import { describe, expect, it, vi } from "vitest";
import { createWithdrawalOrdersApi } from "./withdrawal-orders-api";

const snapshot = {
  brand_id: "11111111-1111-4111-8111-111111111111", member_id: "22222222-2222-4222-8222-222222222222", account_id: "33333333-3333-4333-8333-333333333333",
  base_points: "9007199254740993", valid_points: "9007199254740994", valid_order_count: "2", credit_numerator: "18014398509481987", credit_denominator: "2",
  meets_turnover: true, cycle_from_at: null, cycle_from_version: "0", cutoff_at: "2026-10-07T12:00:00Z", cutoff_version: "8",
};

describe("withdrawal qualification API", () => {
  it("uses an exact read-only GET path with no query or body and forwards cancellation", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: true, data: snapshot })));
    const controller = new AbortController();
    const result = await createWithdrawalOrdersApi({ brandCode: "north star", fetcher }).qualification(controller.signal);
    expect(result).toEqual(snapshot);
    const [url, init] = fetcher.mock.calls[0]!;
    expect(url).toBe("/api/v1/b/north%20star/withdrawal-qualification");
    expect(init?.method).toBeUndefined();
    expect(init?.body).toBeUndefined();
    expect(new URL(String(url), "https://local.test").search).toBe("");
    expect(init?.signal).toBe(controller.signal);
    expect(init?.credentials).toBe("same-origin");
  });

  it("validates strict snapshot shape before returning it", async () => {
    const api = createWithdrawalOrdersApi({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: true, data: { ...snapshot, internal_reason: "private" } }))) });
    await expect(api.qualification()).rejects.toThrow("Invalid server response");
  });
});
