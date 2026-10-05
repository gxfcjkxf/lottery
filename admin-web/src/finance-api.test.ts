import { describe, expect, it, vi } from "vitest";
import {
  brandPermissions,
  canViewRecharges,
  canViewWallet,
  canWriteFinance,
  createBodyKeyTracker,
  createFinanceApi,
  formatIntegerAmount,
  isPositiveAmount,
  isSignedAmount,
  type FinanceAccount,
} from "./finance-api";

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

describe("finance API client", () => {
  it("sends admin wallet reads with same-origin cookies and mandatory brand scope", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockImplementation(async () => ok({}));
    const api = createFinanceApi(fetcher);
    await api.wallet("brand-1", "member/1");
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe("/api/v1/admin/wallets/member%2F1");
    expect(init?.credentials).toBe("same-origin");
    expect((init?.headers as Headers).get("X-Brand-ID")).toBe("brand-1");
    expect((init?.headers as Headers).get("Authorization")).toBeNull();
  });

  it("uses the specified routes, query fields, and request bodies", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockImplementation(async () => ok({ items: [] }));
    const api = createFinanceApi(fetcher);
    await api.ledger("b", "m", 50, 100);
    await api.reconciliation("b", "m");
    await api.recharges("b", "m", 50, 0);
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
      "/api/v1/admin/wallets/m/ledger?limit=50&offset=100",
      "/api/v1/admin/wallets/m/reconciliation",
      "/api/v1/admin/recharges?limit=50&offset=0&member_id=m",
    ]);
  });

  it("posts finance operations with brand, JSON and idempotency headers", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockImplementation(async () => ok({}));
    const api = createFinanceApi(fetcher);
    await api.createRecharge(
      "b",
      { member_id: "m", points: "90071992547409931234", reason: "receipt" },
      "recharge-key",
    );
    await api.confirmRecharge(
      "b",
      "r/1",
      { version: 4, reason: "verified" },
      "confirm-key",
    );
    await api.cancelRecharge(
      "b",
      "r/1",
      { version: 4, reason: "duplicate request" },
      "cancel-key",
    );
    await api.freezeWallet(
      "b",
      "m",
      { points: "25", reason: "review" },
      "freeze-key",
    );
    await api.unfreezeWallet(
      "b",
      "m",
      { entry_id: "freeze-entry", reason: "release" },
      "unfreeze-key",
    );
    await api.adjustWallet(
      "b",
      "m",
      { source: "winning", delta: "-5", reason: "correction" },
      "adjust-key",
    );
    const calls = fetcher.mock.calls;
    expect(calls.map(([url]) => url)).toEqual([
      "/api/v1/admin/recharges",
      "/api/v1/admin/recharges/r%2F1/confirm",
      "/api/v1/admin/recharges/r%2F1/cancel",
      "/api/v1/admin/wallets/m/freeze",
      "/api/v1/admin/wallets/m/unfreeze",
      "/api/v1/admin/wallets/m/adjust",
    ]);
    expect(JSON.parse(String(calls[0][1]?.body))).toEqual({
      member_id: "m",
      points: "90071992547409931234",
      reason: "receipt",
    });
    expect(JSON.parse(String(calls[5][1]?.body))).toEqual({
      source: "winning",
      delta: "-5",
      reason: "correction",
    });
    expect(JSON.parse(String(calls[2][1]?.body))).toEqual({
      version: 4,
      reason: "duplicate request",
    });
    for (const [, init] of calls) {
      expect(init?.method).toBe("POST");
      expect(init?.credentials).toBe("same-origin");
      expect((init?.headers as Headers).get("X-Brand-ID")).toBe("b");
      expect((init?.headers as Headers).get("Content-Type")).toBe(
        "application/json",
      );
      expect((init?.headers as Headers).get("Idempotency-Key")).toBeTruthy();
    }
    expect((calls[0][1]?.headers as Headers).get("Idempotency-Key")).toBe(
      "recharge-key",
    );
    expect((calls[2][1]?.headers as Headers).get("Idempotency-Key")).toBe(
      "cancel-key",
    );
  });

  it("reuses identical-body retry keys and changes them when the body changes", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockImplementation(async () => ok({}));
    const api = createFinanceApi(fetcher);
    const keyFor = createBodyKeyTracker();
    const first = {
      brand_id: "b",
      member_id: "m",
      points: "100",
      reason: "deposit",
    };
    const requestBody = ({ member_id, points, reason }: typeof first) => ({
      member_id,
      points,
      reason,
    });
    await api.createRecharge("b", requestBody(first), keyFor(first));
    await api.createRecharge("b", requestBody(first), keyFor({ ...first }));
    const otherScope = { ...first, member_id: "another-member" };
    await api.createRecharge("b", requestBody(otherScope), keyFor(otherScope));
    const otherBrand = { ...first, brand_id: "another-brand" };
    await api.createRecharge(
      "another-brand",
      requestBody(otherBrand),
      keyFor(otherBrand),
    );
    const changed = { ...first, reason: "corrected" };
    await api.createRecharge("b", requestBody(changed), keyFor(changed));
    const keys = fetcher.mock.calls.map(([, init]) =>
      (init?.headers as Headers).get("Idempotency-Key"),
    );
    expect(keys[0]).toBe(keys[1]);
    expect(new Set(keys).size).toBe(4);
    expect(
      JSON.parse(String(fetcher.mock.calls[2][1]?.body)),
    ).not.toHaveProperty("brand_id");
  });

  it("preserves 401, 403, and 409 statuses and codes", async () => {
    for (const status of [401, 403, 409]) {
      const api = createFinanceApi(
        vi.fn<typeof fetch>().mockResolvedValue(fail(status)),
      );
      await expect(api.recharges("b")).rejects.toMatchObject({
        status,
        code: `error_${status}`,
      });
    }
  });

  it("keeps arbitrary-size point amounts as strings and validates with BigInt", () => {
    expect(formatIntegerAmount("9007199254740993123456789")).toBe(
      "9,007,199,254,740,993,123,456,789",
    );
    expect(formatIntegerAmount("-9007199254740993123456789")).toBe(
      "−9,007,199,254,740,993,123,456,789",
    );
    expect(isPositiveAmount("9007199254740993123456789")).toBe(true);
    expect(isPositiveAmount("0")).toBe(false);
    expect(isPositiveAmount("1.5")).toBe(false);
    expect(isSignedAmount("-9007199254740993123456789")).toBe(true);
    expect(isSignedAmount("0")).toBe(false);
    expect(isSignedAmount("+1")).toBe(false);
  });

  it("applies brand-scoped and platform view grants, while super admins never receive writes", () => {
    const operator: FinanceAccount = {
      id: "a",
      super_admin: false,
      brand_ids: [],
      permissions: [
        "wallet.view.brand",
        "recharge.view.brand",
        "wallet.freeze.brand",
      ],
      permissions_by_brand: {
        a: ["wallet.view.brand", "recharge.view.brand", "wallet.freeze.brand"],
      },
    };
    expect(canViewWallet(operator, "a")).toBe(true);
    expect(canViewRecharges(operator, "a")).toBe(true);
    expect(canViewWallet(operator, "b")).toBe(false);
    expect(brandPermissions(operator, "b").size).toBe(0);
    expect(canWriteFinance(operator, "a", "wallet.freeze.brand")).toBe(true);
    expect(
      canWriteFinance(
        { ...operator, super_admin: true },
        "a",
        "wallet.freeze.brand",
      ),
    ).toBe(false);
    const platformReader = {
      ...operator,
      permissions: [],
      permissions_by_brand: {},
      platform_permissions: ["wallet.view.platform", "recharge.view.platform"],
    };
    expect(canViewWallet(platformReader, "b")).toBe(true);
    expect(canViewRecharges(platformReader, "b")).toBe(true);
    expect(
      canWriteFinance(
        { ...platformReader, platform_permissions: ["wallet.freeze.platform"] },
        "b",
        "wallet.freeze.brand",
      ),
    ).toBe(false);
  });

  it("treats an explicit empty platform grant list as authoritative", () => {
    const account: FinanceAccount = {
      id: "a",
      super_admin: false,
      brand_ids: [],
      permissions: ["wallet.view.platform"],
      platform_permissions: [],
    };
    expect(canViewWallet(account, "b")).toBe(false);
    expect(
      canViewWallet({ ...account, platform_permissions: undefined }, "b"),
    ).toBe(true);
  });
});
