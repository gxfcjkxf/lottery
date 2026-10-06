import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import {
  betManagementPermissions,
  createBetManagementApi,
  createBetMutationKeyTracker,
  isPositivePoints,
  type AdminBetOrder,
  type BrandBetPolicy,
} from "./bet-management-api";

const brand = "brand-a";
const config = {
  min_bet_points: "1",
  max_bet_points: null,
  max_period_points: null,
  max_user_period_points: null,
  user_cancel_allowed: false,
};
const policy: BrandBetPolicy = {
  brand_id: brand,
  version: 1,
  config,
  updated_at: "2026-10-06T00:00:00Z",
};
const order = {
  id: "order-1",
  brand_id: brand,
  global_user_id: "user-1",
  brand_member_id: "member-1",
  account_id: "account-1",
  game_id: "game-1",
  period_id: "period-1",
  play_id: "play-1",
  rule_version_id: "rule-1",
  definition_hash: "hash",
  definition_snapshot: {},
  status: "placed",
  version: 2,
  selection_raw: {},
  selection_normalized: {},
  expanded_bets: [],
  unit_points: "1",
  combination_count: 1,
  multiplier: "1",
  total_points: "1",
  deduction_allocation: [],
  policy_snapshot: config,
  policy_versions: { brand: 1, game: 1 },
  debit_entry_id: "debit-1",
  client_key: "client-1",
  placed_at: "2026-10-06T00:00:00Z",
} as unknown as AdminBetOrder;
const ok = (data: unknown) =>
  new Response(JSON.stringify({ success: true, data }), { status: 200 });

describe("bet management API", () => {
  it("rejects numeric money and invalid overrides without treating malformed successful writes as definitive", async () => {
    await expect(
      createBetManagementApi(
        vi
          .fn<typeof fetch>()
          .mockResolvedValue(
            ok({ ...policy, config: { ...config, min_bet_points: 1 } }),
          ),
      ).getBrandPolicy(brand),
    ).rejects.toMatchObject({ status: 502 });
    const inherit = { mode: "inherit", points: null };
    const gamePolicy = {
      ...policy,
      game_id: "game-1",
      config: {
        min_bet_points: inherit,
        max_bet_points: inherit,
        max_period_points: inherit,
        max_user_period_points: inherit,
        user_cancel_allowed: null,
      },
    };
    for (const invalid of [
      { mode: "unlimited", points: null },
      { mode: "value", points: "01" },
      { mode: "value", points: "0" },
    ]) {
      await expect(
        createBetManagementApi(
          vi
            .fn<typeof fetch>()
            .mockResolvedValue(
              ok({
                ...gamePolicy,
                config: { ...gamePolicy.config, min_bet_points: invalid },
              }),
            ),
        ).getGamePolicy(brand, "game-1"),
      ).rejects.toMatchObject({ status: 502 });
    }
    const brokenJSON = new Response("not-json", { status: 200 });
    await expect(
      createBetManagementApi(
        vi.fn<typeof fetch>().mockResolvedValue(brokenJSON),
      ).updateBrandPolicy(
        brand,
        { version: 1, config, reason: "test" },
        "original-key",
      ),
    ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });
  it("uses scoped admin endpoints, same-origin cookies and explicit brand headers", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(ok(policy))
      .mockResolvedValueOnce(ok({ items: [order] }))
      .mockResolvedValueOnce(ok(order));
    const api = createBetManagementApi(fetcher);
    expect(await api.getBrandPolicy(brand)).toEqual(policy);
    expect(await api.getOrders(brand, "member / one")).toEqual({
      items: [order],
    });
    expect(await api.getOrder(brand, "order/one")).toEqual(order);
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
      "/api/v1/admin/bet-policy",
      "/api/v1/admin/bet-orders?limit=50&offset=0&member_id=member+%2F+one",
      "/api/v1/admin/bet-orders/order%2Fone",
    ]);
    for (const [, init] of fetcher.mock.calls) {
      expect(init?.credentials).toBe("same-origin");
      expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    }
  });

  it("sends caller-supplied write keys and exact versioned bodies", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(ok(policy))
      .mockResolvedValueOnce(ok(order));
    const api = createBetManagementApi(fetcher);
    await api.updateBrandPolicy(
      brand,
      { version: 1, config, reason: "policy" },
      "retry-key",
    );
    await api.cancelOrder(
      brand,
      "order-1",
      { version: 2, reason: "cancel" },
      "cancel-key",
    );
    expect(
      fetcher.mock.calls.map(([url, init]) => [
        url,
        init?.method,
        JSON.parse(String(init?.body)),
        new Headers(init?.headers).get("Idempotency-Key"),
      ]),
    ).toEqual([
      [
        "/api/v1/admin/bet-policy",
        "PUT",
        { version: 1, config, reason: "policy" },
        "retry-key",
      ],
      [
        "/api/v1/admin/bet-orders/order-1/cancel",
        "POST",
        { version: 2, reason: "cancel" },
        "cancel-key",
      ],
    ]);
  });

  it("marks abnormal orders and reads the exception envelope", async () => {
    const exception = {
      id: "exception-1",
      brand_id: brand,
      order_id: order.id,
      order_version: 2,
      marked_by: "admin-1",
      reason: "reviewed",
      created_at: "2026-10-06T00:00:00Z",
    };
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(ok(order))
      .mockResolvedValueOnce(ok({ exception }));
    const api = createBetManagementApi(fetcher);
    expect(
      await api.markAbnormal(
        brand,
        order.id,
        { version: 2, reason: "reviewed" },
        "abnormal-key",
      ),
    ).toEqual(order);
    expect(await api.getException(brand, order.id)).toEqual({ exception });
    expect(
      fetcher.mock.calls.map(([url, init]) => [
        url,
        init?.method,
        new Headers(init?.headers).get("Idempotency-Key"),
      ]),
    ).toEqual([
      [`/api/v1/admin/bet-orders/${order.id}/abnormal`, "POST", "abnormal-key"],
      [`/api/v1/admin/bet-orders/${order.id}/exception`, "GET", null],
    ]);
  });

  it("retains arbitrarily large point strings and validates signed-int64 canonical input", () => {
    expect(isPositivePoints("9223372036854775807")).toBe(true);
    expect(isPositivePoints("9223372036854775808")).toBe(false);
    expect(isPositivePoints("9007199254740993")).toBe(true);
    for (const input of ["0", "01", "+1", "-1", " 1", "1.0", ""])
      expect(isPositivePoints(input)).toBe(false);
  });

  it("tracks idempotency keys by canonical body, brand, entity and operation", () => {
    const tracker = createBetMutationKeyTracker();
    const scope = {
      brandId: brand,
      entityId: "order-1",
      operation: "cancel",
      body: { version: 1, reason: "same", nested: { b: 2, a: 1 } },
    };
    const first = tracker(scope);
    expect(
      tracker({
        ...scope,
        body: { nested: { a: 1, b: 2 }, reason: "same", version: 1 },
      }),
    ).toBe(first);
    expect(tracker({ ...scope, body: { ...scope.body, version: 2 } })).not.toBe(
      first,
    );
    expect(tracker({ ...scope, brandId: "brand-b" })).not.toBe(first);
    expect(tracker({ ...scope, operation: "abnormal" })).not.toBe(first);
  });

  it("preserves structured server errors and turns malformed success envelopes into AdminApiError", async () => {
    for (const status of [400, 403, 409]) {
      const failure = new Response(
        JSON.stringify({
          success: false,
          error: {
            code: `ERROR_${status}`,
            message: `failure-${status}`,
            details: { field: "version" },
          },
        }),
        { status },
      );
      await expect(
        createBetManagementApi(
          vi.fn<typeof fetch>().mockResolvedValue(failure),
        ).getBrandPolicy(brand),
      ).rejects.toMatchObject({
        status,
        code: `ERROR_${status}`,
        message: `failure-${status}`,
      });
    }
    await expect(
      createBetManagementApi(
        vi.fn<typeof fetch>().mockResolvedValue(ok({})),
      ).getBrandPolicy(brand),
    ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(
      createBetManagementApi(
        vi.fn<typeof fetch>().mockRejectedValue(new TypeError("offline")),
      ).getBrandPolicy(brand),
    ).rejects.toMatchObject({ status: 0, code: "NETWORK_ERROR" });
    await expect(
      createBetManagementApi(
        vi
          .fn<typeof fetch>()
          .mockResolvedValue(ok({ success: true } as unknown)),
      ).getBrandPolicy(brand),
    ).rejects.toBeInstanceOf(AdminApiError);
  });

  it("computes per-brand read and write rights without write-implies-read", () => {
    const account: AdminAccount = {
      id: "a",
      super_admin: false,
      brand_ids: [brand],
      permissions: [],
      platform_permissions: ["bet.view.platform", "game.view.platform"],
      permissions_by_brand: {
        [brand]: [
          "bet.cancel.brand",
          "bet.mark_abnormal.brand",
          "bet_policy.write.brand",
        ],
      },
    };
    expect(betManagementPermissions(account, brand)).toEqual({
      policyView: false,
      policyWrite: true,
      ordersView: true,
      cancel: true,
      markAbnormal: true,
      gameView: true,
    });
    expect(
      betManagementPermissions({ ...account, super_admin: true }, brand),
    ).toMatchObject({ policyWrite: false, cancel: false, markAbnormal: false });
    expect(
      betManagementPermissions(
        {
          ...account,
          permissions_by_brand: { other: ["bet.view.brand"] },
          permissions: ["bet_policy.view.brand"],
          platform_permissions: ["game.view.platform"],
        },
        brand,
      ),
    ).toMatchObject({ policyView: false, ordersView: false });
  });
});
