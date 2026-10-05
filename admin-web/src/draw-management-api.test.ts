import { describe, expect, it, vi } from "vitest";
import type { AdminAccount } from "./admin-api";
import {
  buildDrawResult,
  buildManualDrawBody,
  buildSourceSetBody,
  createDrawManagementApi,
  createDrawManagementKeyTracker,
  createDrawManagementRequestGuard,
  drawManagementPermissions,
  parseDrawGame,
  type DrawGame,
  type SourceConfig,
} from "./draw-management-api";

const brand = "brand-1";
const gameId = "game/one";
const game: DrawGame = {
  id: "game-1",
  brand_id: brand,
  code: "G1",
  name: "Game One",
  timezone: "Asia/Singapore",
  version: 3,
  status: "active",
  model: {
    model: "X_PLUS_Y",
    regular_pool: {
      min: 1,
      max: 5,
      values: [1, 2, 3, 4, 5],
      allow_repeat: false,
    },
    special_pool: { min: 6, max: 8, values: [6, 7, 8], allow_repeat: false },
    regular_count: 2,
    special_count: 1,
    pool_size: 8,
    total_count: 3,
    length: 3,
    allow_repeat: false,
    ordered: false,
  },
  regular_count: 2,
  special_count: 1,
  length: 3,
};
const goGamePayload = {
  id: game.id,
  brand_id: brand,
  code: game.code,
  name: game.name,
  timezone: game.timezone,
  status: "active",
  version: 3,
  started_sequence: 0,
  model: {
    model: "X_PLUS_Y",
    regular_pool: { min: 1, max: 5, allow_repeat: false },
    special_pool: { min: 6, max: 8, allow_repeat: false },
    regular_count: 2,
    special_count: 1,
    pool_size: 0,
    total_count: 0,
    length: 0,
    allow_repeat: false,
    ordered: false,
  },
};
const period = {
  id: "period-1",
  brand_id: brand,
  game_id: game.id,
  period_no: "20261006001",
  sequence: 1,
  bet_start_at: "2026-10-06T00:00:00.000Z",
  bet_end_at: "2026-10-06T01:00:00.000Z",
  draw_at: "2026-10-06T01:00:00.000Z",
  status: "waiting_draw",
  version: 2,
};
const source: SourceConfig = {
  id: "018f47a1-7b2c-7abc-8def-0123456789ab",
  name: "Primary",
  type: "api",
  priority: 1,
  enabled: true,
  endpoint: "https://feeds.example.com/results",
  credential_ref: "draw/source1",
};
const iso = "2026-10-06T01:00:00.000Z";
const ok = (data: unknown, status = 200) =>
  new Response(JSON.stringify({ success: true, data }), {
    status,
    headers: { "Content-Type": "application/json" },
  });
const failure = (status: number, code: string) =>
  new Response(
    JSON.stringify({
      success: false,
      error: { code, message: "request failed" },
    }),
    { status },
  );

describe("draw management permissions", () => {
  it("uses exact brand/platform view grants and never grants writes to super admins", () => {
    const account: AdminAccount = {
      id: "admin",
      super_admin: true,
      brand_ids: [brand],
      permissions: [],
      permissions_by_brand: {
        [brand]: [
          "draw_source.view.brand",
          "draw_source.write.brand",
          "draw.manual_create.brand",
        ],
      },
      platform_permissions: [
        "game.view.platform",
        "period.view.platform",
        "draw.view.platform",
      ],
    };
    expect(drawManagementPermissions(account, brand)).toEqual({
      gamesView: true,
      periodsView: true,
      sourceView: true,
      sourceWrite: false,
      drawView: true,
      manualCreate: false,
    });
    expect(drawManagementPermissions(account, "")).toEqual({
      gamesView: false,
      periodsView: false,
      sourceView: false,
      sourceWrite: false,
      drawView: false,
      manualCreate: false,
    });
  });
});

describe("model-aware manual draw builders", () => {
  it("builds exact-count typed number draws and rejects overflow, out-of-pool and duplicates", () => {
    expect(buildDrawResult(game.model, "1, 5", "7", "")).toEqual({
      regular: [1, 5],
      special: [7],
      digits: [],
    });
    expect(() =>
      buildDrawResult(game.model, "1,9007199254740993", "7", ""),
    ).toThrow(/范围/);
    expect(() => buildDrawResult(game.model, "1, 1", "7", "")).toThrow(/重复/);
    expect(() => buildDrawResult(game.model, "1", "7", "")).toThrow(/恰好/);
    expect(() =>
      buildDrawResult(game.model, '{"regular":[1]}', "7", ""),
    ).toThrow();
  });

  it("requires distinct M_SELECT_N regular numbers and exact digit positions", () => {
    const mSelect = {
      ...game.model,
      model: "M_SELECT_N" as const,
      regular_count: 2,
      special_count: 0,
      regular_pool: { min: 1, max: 3, values: [1, 2, 3], allow_repeat: true },
      special_pool: { min: 0, max: 0, values: [], allow_repeat: false },
    };
    expect(() => buildDrawResult(mSelect, "2,2", "", "")).toThrow(/重复/);
    const digits = {
      ...game.model,
      model: "DIGITS_0_9" as const,
      regular_count: 0,
      special_count: 0,
      length: 3,
      regular_pool: { min: 0, max: 0, values: [], allow_repeat: false },
      special_pool: { min: 0, max: 0, values: [], allow_repeat: false },
      pool_size: 0,
      total_count: 0,
      allow_repeat: true,
      ordered: true,
    };
    expect(buildDrawResult(digits, "", "", "0,9,2")).toEqual({
      regular: [],
      special: [],
      digits: [0, 9, 2],
    });
    expect(() => buildDrawResult(digits, "", "", "0,10,2")).toThrow(/范围/);
    expect(() => buildDrawResult(digits, "1", "", "0,9,2")).toThrow();
  });

  it("requires explicit confirmation, a reason and strict nonfuture UTC ISO time", () => {
    const args = {
      version: 2,
      period,
      model: game.model,
      regularInput: "1,2",
      specialInput: "6",
      digitsInput: "",
      drawnAt: iso,
      reason: "external verified result",
      confirmed: true,
      serverNow: Date.parse(iso),
    };
    expect(buildManualDrawBody(args).result).toEqual({
      regular: [1, 2],
      special: [6],
      digits: [],
    });
    expect(() => buildManualDrawBody({ ...args, confirmed: false })).toThrow(
      /确认/,
    );
    expect(() => buildManualDrawBody({ ...args, reason: " " })).toThrow(/原因/);
    expect(
      buildManualDrawBody({ ...args, drawnAt: "2026-10-06T01:00:00Z" })
        .drawn_at,
    ).toBe("2026-10-06T01:00:00Z");
    expect(() =>
      buildManualDrawBody({ ...args, drawnAt: "2026-10-06T01:00:00.001Z" }),
    ).toThrow(/不能晚于/);
  });
});

describe("source configuration validation", () => {
  it("accepts safe full-set sources and rejects duplicate priorities and unsafe endpoints/credentials", () => {
    expect(buildSourceSetBody(3, [source], "add trusted source")).toEqual({
      version: 3,
      sources: [source],
      reason: "add trusted source",
    });
    expect(() =>
      buildSourceSetBody(
        3,
        [source, { ...source, id: "22222222-2222-4222-8222-222222222222" }],
        "duplicate priority",
      ),
    ).toThrow(/优先级/);
    for (const endpoint of [
      "http://feeds.example.com/path",
      "https://user:pass@feeds.example.com/path",
      "https://feeds.example.com/path?q=x",
      "https://feeds.example.com/path#top",
      "https://feeds.example.com:8443/path",
      "https://127.0.0.1/path",
      "https://[::1]/path",
      "https://feeds.test/path",
    ]) {
      expect(() =>
        buildSourceSetBody(3, [{ ...source, endpoint }], "bad endpoint"),
      ).toThrow(/HTTPS URL/);
    }
    expect(() =>
      buildSourceSetBody(
        3,
        [{ ...source, credential_ref: "contains=secret" }],
        "no secrets",
      ),
    ).toThrow(/标识格式/);
    expect(() =>
      buildSourceSetBody(
        3,
        [{ ...source, type: "dom", selector: "" }],
        "dom selector",
      ),
    ).toThrow(/选择器/);
    expect(() =>
      buildSourceSetBody(
        3,
        [{ ...source, name: "名".repeat(41) }],
        "oversized UTF-8 name",
      ),
    ).toThrow(/UTF-8/);
    expect(() =>
      buildSourceSetBody(3, [{ ...source, priority: 10001 }], "priority limit"),
    ).toThrow(/10000/);
    expect(() => buildSourceSetBody(3, [source], "原因".repeat(168))).toThrow(
      /UTF-8/,
    );
  });
});

describe("draw management API contract", () => {
  it("sends paged brand-scoped reads and strictly parses complete Game models", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValue(ok({ games: [goGamePayload] }));
    const api = createDrawManagementApi(fetcher);
    const result = await api.getGames(brand, 25, 50);
    expect(result.games[0]).toMatchObject({
      model: {
        model: "X_PLUS_Y",
        regular_count: 2,
        special_count: 1,
        length: 0,
      },
    });
    expect(fetcher).toHaveBeenCalledWith(
      "/api/v1/admin/games?limit=25&offset=50",
      expect.objectContaining({
        method: "GET",
        credentials: "same-origin",
        headers: expect.any(Headers),
      }),
    );
    const headers = new Headers(fetcher.mock.calls[0][1]?.headers);
    expect(headers.get("X-Brand-ID")).toBe(brand);
    expect(headers.get("Idempotency-Key")).toBeNull();
    expect(() =>
      parseDrawGame({
        ...goGamePayload,
        model: { ...goGamePayload.model, regular_count: -1 },
      }),
    ).toThrow(/计数/);
    const digits = parseDrawGame({
      ...goGamePayload,
      model: {
        model: "DIGITS_0_9",
        regular_pool: { allow_repeat: false },
        special_pool: { allow_repeat: false },
        length: 3,
        allow_repeat: true,
        ordered: true,
      },
    });
    expect(digits.model).toMatchObject({
      model: "DIGITS_0_9",
      regular_count: 0,
      special_count: 0,
      length: 3,
      regular_pool: { values: [], min: 0, max: 0 },
      special_pool: { values: [], min: 0, max: 0 },
    });
  });

  it("treats only source-set 404 as no configuration and validates its target scope", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValue(failure(404, "NOT_FOUND"));
    await expect(
      createDrawManagementApi(fetcher).getSourceSet(brand, gameId),
    ).resolves.toBeNull();
    expect(fetcher).toHaveBeenCalledWith(
      `/api/v1/admin/games/${encodeURIComponent(gameId)}/draw-sources`,
      expect.any(Object),
    );
    const malformed = createDrawManagementApi(
      vi.fn<typeof fetch>().mockResolvedValue(ok({ nope: true })),
    );
    await expect(malformed.getSourceSet(brand, game.id)).rejects.toMatchObject({
      status: 502,
      code: "INVALID_RESPONSE",
    });
  });

  it("puts only the versioned full source set and sends the reusable idempotency key", async () => {
    const saved = {
      id: "018f47a1-7b2c-7abc-8def-1123456789ab",
      brand_id: brand,
      game_id: game.id,
      revision: 4,
      game_version: 4,
      created_at: "2026-10-06T01:00:00.123456Z",
      sources: [source],
    };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(saved));
    await createDrawManagementApi(fetcher).updateSourceSet(
      brand,
      game.id,
      { version: 3, sources: [source], reason: "add primary" },
      "stable-key",
    );
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe(`/api/v1/admin/games/${game.id}/draw-sources`);
    expect(init?.method).toBe("PUT");
    expect(JSON.parse(String(init?.body))).toEqual({
      version: 3,
      sources: [source],
      reason: "add primary",
    });
    const headers = new Headers(init?.headers);
    expect(headers.get("X-Brand-ID")).toBe(brand);
    expect(headers.get("Idempotency-Key")).toBe("stable-key");
  });

  it("parses paged periods, draw history, attempts and posts the exact manual body", async () => {
    const draw = {
      id: "draw-1",
      brand_id: brand,
      game_id: game.id,
      period_id: period.id,
      source_id: "018f47a1-7b2c-7abc-8def-2123456789ab",
      kind: "manual",
      result: { regular: [1, 2], special: [6], digits: [] },
      result_hash: "abc",
      drawn_at: "2026-10-06T01:00:00.123456Z",
      created_at: "2026-10-06T01:00:00.123456789Z",
    };
    const batch = {
      id: "batch-1",
      brand_id: brand,
      game_id: game.id,
      period_id: period.id,
      source_set_id: "018f47a1-7b2c-7abc-8def-3123456789ab",
      observed_period_version: 2,
      status: "accepted",
      attempts: [{ source_id: source.id, status: "accepted", code: "OK" }],
      created_at: "2026-10-06T01:00:00.123456Z",
    };
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(ok({ periods: [period] }))
      .mockResolvedValueOnce(
        ok({
          current: draw,
          history: [draw],
          attempts: [batch],
          limit: 50,
          offset: 0,
        }),
      )
      .mockResolvedValueOnce(ok(draw));
    const api = createDrawManagementApi(fetcher);
    await expect(api.getPeriods(brand, game.id, 25, 25)).resolves.toMatchObject(
      { periods: [period], limit: 25, offset: 25 },
    );
    await expect(
      api.getDrawHistory(brand, game, period.id),
    ).resolves.toMatchObject({
      current: { kind: "manual" },
      history: [draw],
      attempts: [batch],
      limit: 50,
    });
    const manualPeriod = {
      ...period,
      draw_at: new Date(Date.now() - 5000).toISOString(),
    };
    const manualBody = {
      version: 2,
      period_no: period.period_no,
      result: draw.result,
      drawn_at: new Date(Date.now() - 1000)
        .toISOString()
        .replace(/\.\d{3}Z$/, ".123456Z"),
      reason: "verified",
    };
    await api.createManualDraw(
      brand,
      manualPeriod,
      manualBody,
      "manual-key",
      game,
    );
    expect(fetcher.mock.calls[2][0]).toBe(
      `/api/v1/admin/periods/${period.id}/manual-draw`,
    );
    expect(JSON.parse(String(fetcher.mock.calls[2][1]?.body))).toEqual(
      manualBody,
    );
    expect(
      new Headers(fetcher.mock.calls[2][1]?.headers).get("Idempotency-Key"),
    ).toBe("manual-key");
  });

  it("surfaces backend status/code and rejects unknown envelopes and wrong IDs", async () => {
    const api = createDrawManagementApi(
      vi.fn<typeof fetch>().mockResolvedValue(failure(409, "VERSION_CONFLICT")),
    );
    await expect(api.getGames(brand)).rejects.toMatchObject({
      status: 409,
      code: "VERSION_CONFLICT",
    });
    const bad = createDrawManagementApi(
      vi
        .fn<typeof fetch>()
        .mockResolvedValue(
          ok({
            games: [
              {
                ...goGamePayload,
                model: { ...goGamePayload.model, model: "future" },
              },
            ],
          }),
        ),
    );
    await expect(bad.getGames(brand)).rejects.toMatchObject({
      status: 502,
      code: "INVALID_RESPONSE",
    });
  });

  it("scopes idempotency keys to brand, operation, target and canonical body", () => {
    const tracker = createDrawManagementKeyTracker();
    const first = tracker(brand, "manual", "period-1", {
      version: 1,
      result: [1, 2],
    });
    expect(
      tracker(brand, "manual", "period-1", { result: [1, 2], version: 1 }),
    ).toBe(first);
    expect(
      tracker(brand, "manual", "period-2", { version: 1, result: [1, 2] }),
    ).not.toBe(first);
    expect(
      tracker("brand-2", "manual", "period-1", { version: 1, result: [1, 2] }),
    ).not.toBe(first);
    expect(
      tracker(brand, "manual", "period-1", { version: 2, result: [1, 2] }),
    ).not.toBe(first);
  });

  it("discards stale read tickets after scope changes, lane reloads, permission loss and invalidation", () => {
    const guard = createDrawManagementRequestGuard();
    const old = guard.capture("history", "brand/game/period-1");
    expect(guard.isCurrent(old, "brand/game/period-1", true)).toBe(true);
    expect(guard.isCurrent(old, "brand/game/period-2", true)).toBe(false);
    expect(guard.isCurrent(old, "brand/game/period-1", false)).toBe(false);
    const next = guard.capture("history", "brand/game/period-1");
    expect(guard.isCurrent(old, "brand/game/period-1", true)).toBe(false);
    expect(guard.isCurrent(next, "brand/game/period-1", true)).toBe(true);
    guard.invalidate("history");
    expect(guard.isCurrent(next, "brand/game/period-1", true)).toBe(false);
  });
});
