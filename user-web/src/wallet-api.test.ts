import { describe, expect, it, vi } from "vitest";
import {
  createWalletApi,
  formatIntegerAmount,
  WalletApiError,
  walletBasePath,
} from "./wallet-api";

const ok = (data: unknown) =>
  new Response(JSON.stringify({ success: true, data }), { status: 200 });
const fail = (status: number) =>
  new Response(
    JSON.stringify({
      success: false,
      error: { code: `error_${status}`, message: `status ${status}` },
    }),
    { status },
  );
const wallet = {
  account_id: "11111111-1111-4111-8111-111111111111",
  brand_id: "22222222-2222-4222-8222-222222222222",
  member_id: "33333333-3333-4333-8333-333333333333",
  version: 1, display_points: "0", available_points: "0", frozen_points: "0", withdrawal_points: "0",
  recharge_points: "0", winning_points: "0", gift_points: "0", commission_points: "0", manual_frozen_points: "0", system_frozen_points: "0",
  by_source: Object.fromEntries(["recharge", "winning", "gift", "commission"].map((source) => [source, { available: "0", manual_frozen: "0", system_frozen: "0", withdrawal: "0" }])),
};

describe("user wallet API", () => {
  it("uses an optional brand prefix and same-origin cookie credentials without bearer headers", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockImplementation(async (input) => ok(String(input).endsWith("/wallet") ? wallet : { items: [] }));
    const api = createWalletApi({ brandCode: "north star", fetcher });
    await api.wallet();
    const [url, init] = fetcher.mock.calls[0];
    expect(walletBasePath("north star")).toBe("/api/v1/b/north%20star");
    expect(url).toBe("/api/v1/b/north%20star/wallet");
    expect(init?.credentials).toBe("same-origin");
    expect(new Headers(init?.headers).get("Authorization")).toBeNull();
    expect(new Headers(init?.headers).get("Cookie")).toBeNull();
  });

  it("uses unprefixed wallet paths when no brand code is configured", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockImplementation(async (input) => ok(String(input).includes("/wallet/ledger?") ? { items: [] } : wallet));
    const api = createWalletApi({ brandCode: "", fetcher });
    await api.wallet();
    await api.ledger(50, 100);
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
      "/api/v1/wallet",
      "/api/v1/wallet/ledger?limit=50&offset=100",
    ]);
    for (const [, init] of fetcher.mock.calls) {
      expect(init?.credentials).toBe("same-origin");
      expect(new Headers(init?.headers).get("Authorization")).toBeNull();
    }
  });

  it("surfaces 401, 403 and 409 errors instead of returning sample wallet data", async () => {
    for (const status of [401, 403, 409]) {
      const api = createWalletApi({
        fetcher: vi.fn<typeof fetch>().mockResolvedValue(fail(status)),
      });
      await expect(api.wallet()).rejects.toMatchObject({
        status,
        code: `error_${status}`,
      });
    }
  });

  it("formats exact integer strings without Number precision loss", () => {
    expect(formatIntegerAmount("9007199254740993123456789")).toBe(
      "9,007,199,254,740,993,123,456,789",
    );
    expect(formatIntegerAmount("-9007199254740993123456789")).toBe(
      "−9,007,199,254,740,993,123,456,789",
    );
  });

  it("normalizes legacy ledger snapshots without altering their source object", async () => {
    const legacy = Object.fromEntries(["recharge", "winning", "gift"].map((source) => [source, { available: "1", manual_frozen: "0", system_frozen: "0", withdrawal: "0" }]));
    const entry = { before_snapshot: legacy, delta_snapshot: legacy, after_snapshot: legacy };
    const api = createWalletApi({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(ok({ items: [entry] })) });
    const result = await api.ledger();
    expect(result.items[0]?.before_snapshot.commission.available).toBe("0");
    expect(Object.keys(legacy)).toEqual(["recharge", "winning", "gift"]);
  });

  it("rejects current wallet payloads that omit commission", async () => {
    const { commission_points: _commissionPoints, ...legacy } = wallet;
    const api = createWalletApi({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(ok(legacy)) });
    await expect(api.wallet()).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });
});
