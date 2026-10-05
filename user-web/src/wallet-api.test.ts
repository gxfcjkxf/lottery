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

describe("user wallet API", () => {
  it("uses an optional brand prefix and same-origin cookie credentials without bearer headers", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockImplementation(async () => ok({ items: [] }));
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
      .mockImplementation(async () => ok({ items: [] }));
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
});
