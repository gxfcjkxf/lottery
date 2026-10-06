import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import {
  createWithdrawalPolicyApi,
  isValidTurnoverMultiple,
  withdrawalPolicyPermissions,
  type BrandWithdrawalConfig,
  type BrandWithdrawalPolicy,
  type GameWithdrawalConfig,
  type GameWithdrawalPolicy,
  type PolicyRevision,
} from "./withdrawal-policy-api";

const brand = "11111111-1111-4111-8111-111111111111";
const otherBrand = "22222222-2222-4222-8222-222222222222";
const game = "33333333-3333-4333-8333-333333333333";
const otherGame = "44444444-4444-4444-8444-444444444444";
const auditId = "55555555-5555-4555-8555-555555555555";
const revisionId = "66666666-6666-4666-8666-666666666666";
const timestamp = "2026-10-06T00:00:00Z";

const brandConfig: BrandWithdrawalConfig = {
  enabled: false,
  min_points: "1",
  max_points: null,
  allowed_sources: ["recharge", "winning", "gift"],
  review_mode: "manual",
  turnover_multiple: "1",
};
const brandPolicy: BrandWithdrawalPolicy = {
  brand_id: brand,
  version: 1,
  config: brandConfig,
  updated_at: timestamp,
  audit_log_id: auditId,
};
const gameConfig: GameWithdrawalConfig = { turnover_multiple: null };
const gamePolicy: GameWithdrawalPolicy = {
  brand_id: brand,
  game_id: game,
  version: 3,
  config: gameConfig,
  effective: {
    turnover_multiple: "0",
    source: "brand",
    brand_version: 1,
    game_version: 3,
  },
  updated_at: timestamp,
};
const brandRevision: PolicyRevision = {
  id: revisionId,
  brand_id: brand,
  game_id: "",
  version: 1,
  config: brandConfig,
  changed_by: "",
  reason: "initial",
  created_at: timestamp,
};
const gameRevision: PolicyRevision = {
  ...brandRevision,
  game_id: game,
  config: gameConfig,
};
const ok = (data: unknown) =>
  new Response(JSON.stringify({ success: true, data }), { status: 200 });

describe("withdrawal policy API", () => {
  it("uses explicit, brand-scoped read and write permissions", () => {
    const account = {
      id: "admin",
      super_admin: false,
      brand_ids: [brand],
      permissions: ["withdrawal_policy.write.brand"],
    } satisfies AdminAccount;
    expect(withdrawalPolicyPermissions(account, brand)).toEqual({
      view: false,
      write: true,
      gameView: false,
    });
    expect(
      withdrawalPolicyPermissions(
        {
          ...account,
          super_admin: true,
          permissions: [
            "withdrawal_policy.view.brand",
            "withdrawal_policy.write.brand",
          ],
        },
        brand,
      ),
    ).toEqual({ view: true, write: false, gameView: false });
    expect(
      withdrawalPolicyPermissions(
        {
          ...account,
          permissions: ["withdrawal_policy.view.brand", "game.view.brand"],
          permissions_by_brand: {
            [otherBrand]: ["withdrawal_policy.view.brand"],
          },
          platform_permissions: ["game.view.platform"],
        },
        brand,
      ),
    ).toEqual({ view: false, write: false, gameView: true });
    expect(
      withdrawalPolicyPermissions(
        {
          ...account,
          permissions: [],
          platform_permissions: ["withdrawal_policy.view.platform"],
        },
        brand,
      ),
    ).toMatchObject({ view: true, write: false });
    expect(withdrawalPolicyPermissions(account, "")).toEqual({
      view: false,
      write: false,
      gameView: false,
    });
    expect(withdrawalPolicyPermissions(account, otherBrand)).toEqual({
      view: false,
      write: false,
      gameView: false,
    });
  });

  it("validates turnover decimals without floating point", () => {
    for (const valid of ["0", "0.000001", "1", "1.25", "1000000"]) {
      expect(isValidTurnoverMultiple(valid), valid).toBe(true);
    }
    for (const invalid of [
      "",
      "00",
      "01",
      "-1",
      "+1",
      "1e2",
      "1.0",
      "0.0000001",
      "1000000.000001",
      "1000001",
      " 1",
    ]) {
      expect(isValidTurnoverMultiple(invalid), invalid).toBe(false);
    }
  });

  it("fetches scoped policies and history with same-origin credentials", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(ok(brandPolicy))
      .mockResolvedValueOnce(ok(gamePolicy))
      .mockResolvedValueOnce(
        ok({ items: [brandRevision], limit: 50, offset: 0 }),
      )
      .mockResolvedValueOnce(
        ok({ items: [gameRevision], limit: 7, offset: 9 }),
      );
    const api = createWithdrawalPolicyApi(fetcher);
    expect(await api.getBrandPolicy(brand)).toEqual(brandPolicy);
    expect(await api.getGamePolicy(brand, game)).toEqual(gamePolicy);
    expect(await api.getBrandHistory(brand)).toEqual({
      items: [brandRevision],
      limit: 50,
      offset: 0,
    });
    expect(await api.getGameHistory(brand, game, 7, 9)).toEqual({
      items: [gameRevision],
      limit: 7,
      offset: 9,
    });
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
      "/api/v1/admin/withdrawal-policy",
      `/api/v1/admin/games/${game}/withdrawal-policy`,
      "/api/v1/admin/withdrawal-policy/history?limit=50&offset=0",
      `/api/v1/admin/games/${game}/withdrawal-policy/history?limit=7&offset=9`,
    ]);
    for (const [, init] of fetcher.mock.calls) {
      expect(init?.credentials).toBe("same-origin");
      expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    }
  });

  it("keeps the caller's exact body and idempotency key for both writes", async () => {
    const brandBody = { version: 1, config: brandConfig, reason: " reviewed " };
    const gameBody = {
      version: 3,
      config: { turnover_multiple: "0" },
      reason: "game override",
    };
    const gameOverride = {
      ...gamePolicy,
      version: 4,
      audit_log_id: auditId,
      config: { turnover_multiple: "0" },
      effective: {
        ...gamePolicy.effective,
        game_version: 4,
        turnover_multiple: "0",
        source: "game" as const,
      },
    };
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(ok({ ...brandPolicy, version: 2 }))
      .mockResolvedValueOnce(ok({ ...brandPolicy, version: 2 }))
      .mockResolvedValueOnce(ok(gameOverride));
    const api = createWithdrawalPolicyApi(fetcher);
    await api.updateBrandPolicy(brand, brandBody, "same-original-key");
    await api.updateBrandPolicy(brand, brandBody, "same-original-key");
    await api.updateGamePolicy(brand, game, gameBody, "same-original-key");
    expect(
      fetcher.mock.calls.map(([, init]) => [
        init?.method,
        JSON.parse(String(init?.body)),
        new Headers(init?.headers).get("Idempotency-Key"),
      ]),
    ).toEqual([
      ["PUT", brandBody, "same-original-key"],
      ["PUT", brandBody, "same-original-key"],
      ["PUT", gameBody, "same-original-key"],
    ]);
  });

  it("allows an explicit zero game override and validates inherited precedence", async () => {
    const zero = {
      ...gamePolicy,
      config: { turnover_multiple: "0" },
      effective: {
        ...gamePolicy.effective,
        turnover_multiple: "0",
        source: "game" as const,
      },
    };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok(zero));
    expect(
      await createWithdrawalPolicyApi(fetcher).getGamePolicy(brand, game),
    ).toEqual(zero);
    const badInheritance = {
      ...gamePolicy,
      effective: { ...gamePolicy.effective, source: "game" },
    };
    await expect(
      createWithdrawalPolicyApi(
        vi.fn<typeof fetch>().mockResolvedValue(ok(badInheritance)),
      ).getGamePolicy(brand, game),
    ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("rejects invalid and over-limit inputs before making a request", async () => {
    const fetcher = vi.fn<typeof fetch>();
    const api = createWithdrawalPolicyApi(fetcher);
    const invalidConfigs: unknown[] = [
      { ...brandConfig, min_points: "01" },
      { ...brandConfig, min_points: "9223372036854775808" },
      { ...brandConfig, min_points: "2", max_points: "1" },
      { ...brandConfig, max_points: 1 },
      { ...brandConfig, allowed_sources: [] },
      { ...brandConfig, allowed_sources: ["gift", "gift"] },
      { ...brandConfig, allowed_sources: ["cash"] },
      { ...brandConfig, review_mode: "automatic-ish" },
      { ...brandConfig, turnover_multiple: "1000001" },
      { ...brandConfig, turnover_multiple: "1.0000001" },
    ];
    for (const config of invalidConfigs) {
      await expect(
        api.updateBrandPolicy(
          brand,
          { version: 1, config, reason: "test" } as never,
          "key",
        ),
      ).rejects.toMatchObject({ status: 0, code: "INVALID_INPUT" });
    }
    await expect(api.getGamePolicy("not-a-uuid", game)).rejects.toMatchObject({
      status: 0,
      code: "INVALID_INPUT",
    });
    await expect(
      api.updateGamePolicy(
        brand,
        game,
        { version: 1, config: { turnover_multiple: "1.0" }, reason: "x" },
        "key",
      ),
    ).rejects.toMatchObject({ status: 0, code: "INVALID_INPUT" });
    await expect(
      api.updateBrandPolicy(
        brand,
        { version: 1, config: brandConfig, reason: "字".repeat(168) },
        "key",
      ),
    ).rejects.toMatchObject({ status: 0, code: "INVALID_INPUT" });
    expect(fetcher).not.toHaveBeenCalled();
  });

  it("rejects malformed success DTOs, bad history context and mismatched revisions", async () => {
    for (const data of [
      { ...brandPolicy, brand_id: otherBrand },
      { ...brandPolicy, config: { ...brandConfig, min_points: 1 } },
      { ...brandPolicy, updated_at: "yesterday" },
      { ...brandPolicy, updated_at: "2026-02-30T00:00:00Z" },
      { ...brandPolicy, audit_log_id: "not-uuid" },
    ]) {
      await expect(
        createWithdrawalPolicyApi(
          vi.fn<typeof fetch>().mockResolvedValue(ok(data)),
        ).getBrandPolicy(brand),
      ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
    for (const data of [
      { ...gamePolicy, game_id: otherGame },
      { ...gamePolicy, version: 2 },
      {
        ...gamePolicy,
        effective: { ...gamePolicy.effective, game_version: 2 },
      },
      { ...gamePolicy, config: { turnover_multiple: "0" } },
    ]) {
      await expect(
        createWithdrawalPolicyApi(
          vi.fn<typeof fetch>().mockResolvedValue(ok(data)),
        ).getGamePolicy(brand, game),
      ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
    const mismatchedBrandHistory = {
      items: [{ ...brandRevision, brand_id: otherBrand }],
      limit: 50,
      offset: 0,
    };
    await expect(
      createWithdrawalPolicyApi(
        vi.fn<typeof fetch>().mockResolvedValue(ok(mismatchedBrandHistory)),
      ).getBrandHistory(brand),
    ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    const wrongGameHistory = {
      items: [{ ...gameRevision, game_id: otherGame }],
      limit: 50,
      offset: 0,
    };
    await expect(
      createWithdrawalPolicyApi(
        vi.fn<typeof fetch>().mockResolvedValue(ok(wrongGameHistory)),
      ).getGameHistory(brand, game),
    ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("preserves API errors and maps malformed successful responses to 502", async () => {
    const conflict = new Response(
      JSON.stringify({
        success: false,
        error: {
          code: "WITHDRAWAL_POLICY_VERSION_CONFLICT",
          message: "conflict",
        },
      }),
      { status: 409 },
    );
    await expect(
      createWithdrawalPolicyApi(
        vi.fn<typeof fetch>().mockResolvedValue(conflict),
      ).getBrandPolicy(brand),
    ).rejects.toMatchObject({
      status: 409,
      code: "WITHDRAWAL_POLICY_VERSION_CONFLICT",
    });
    const badSuccess = new Response("not-json", { status: 200 });
    await expect(
      createWithdrawalPolicyApi(
        vi.fn<typeof fetch>().mockResolvedValue(badSuccess),
      ).getBrandPolicy(brand),
    ).rejects.toBeInstanceOf(AdminApiError);
    await expect(
      createWithdrawalPolicyApi(
        vi
          .fn<typeof fetch>()
          .mockResolvedValue(new Response("not-json", { status: 200 })),
      ).getBrandPolicy(brand),
    ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });
  it("requires a write receipt to match the frozen configuration and next version", async () => {
    const body = { version: 1, config: brandConfig, reason: "verified" };
    for (const bad of [
      brandPolicy,
      { ...brandPolicy, version: 2, audit_log_id: undefined },
      {
        ...brandPolicy,
        version: 2,
        config: { ...brandConfig, turnover_multiple: "2" },
      },
    ]) {
      await expect(
        createWithdrawalPolicyApi(
          vi.fn<typeof fetch>().mockResolvedValue(ok(bad)),
        ).updateBrandPolicy(brand, body, "receipt-test-key"),
      ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
    const rearranged = {
      ...brandPolicy,
      version: 2,
      config: {
        turnover_multiple: "1",
        review_mode: "manual",
        allowed_sources: brandConfig.allowed_sources,
        max_points: null,
        min_points: "1",
        enabled: false,
      },
    };
    await expect(
      createWithdrawalPolicyApi(
        vi.fn<typeof fetch>().mockResolvedValue(ok(rearranged)),
      ).updateBrandPolicy(brand, body, "receipt-test-key"),
    ).resolves.toMatchObject({ version: 2 });
    const fetcher = vi.fn<typeof fetch>();
    await expect(
      createWithdrawalPolicyApi(fetcher).getBrandHistory(brand, 101, 0),
    ).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(
      createWithdrawalPolicyApi(fetcher).getGameHistory(brand, ""),
    ).rejects.toMatchObject({ code: "INVALID_INPUT" });
    expect(fetcher).not.toHaveBeenCalled();
  });
});
