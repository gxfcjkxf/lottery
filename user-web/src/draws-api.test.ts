import { describe, expect, it, vi } from "vitest";
import { BettingApiError } from "./betting-api";
import { createDrawsApi, type PublicDrawResult } from "./draws-api";

const gameId = "11111111-1111-4111-8111-111111111111";
const periodId = "22222222-2222-4222-8222-222222222222";
const resultId = "33333333-3333-4333-8333-333333333333";
const timestamp = "2026-10-06T12:00:00Z";

const draw: PublicDrawResult = {
  id: resultId,
  game: {
    id: gameId,
    code: "daily",
    name: "Daily",
    model: {
      model: "DIGITS_0_9",
      regular_pool: { allow_repeat: false },
      special_pool: { allow_repeat: false },
      regular_count: 0,
      special_count: 0,
      pool_size: 0,
      total_count: 0,
      length: 3,
      allow_repeat: true,
      ordered: true,
    },
    timezone: "Asia/Singapore",
    status: "active",
  },
  period: {
    id: periodId,
    game_id: gameId,
    period_no: "20261006001",
    status: "judged_cancelled",
    bet_start_at: "2026-10-06T10:00:00+08:00",
    bet_end_at: "2026-10-06T11:00:00+08:00",
    draw_at: "2026-10-06T12:00:00+08:00",
    draw_result_id: resultId,
  },
  result: { regular: [], special: [], digits: [0, 0, 7] },
  drawn_at: timestamp,
  origin: "manual",
};

const page = (overrides: Record<string, unknown> = {}) => ({
  items: [draw],
  limit: 50,
  offset: 0,
  has_more: false,
  server_time: timestamp,
  brand_status: "active",
  ...overrides,
});
const ok = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200 });

describe("public draws API", () => {
  it("reads a valid result, preserves zero-leading digits, and uses included cookie credentials", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(page()));
    const api = createDrawsApi("north star", fetcher);
    const response = await api.listResults({
      gameId,
      periodNo: draw.period.period_no,
    });

    expect(response.items[0].result.digits).toEqual([0, 0, 7]);
    expect(response.items[0].period.status).toBe("judged_cancelled");
    expect(fetcher.mock.calls[0][0]).toBe(
      `/api/v1/b/north%20star/draw-results?game_id=${gameId}&period_no=20261006001&limit=50&offset=0`,
    );
    const [, init] = fetcher.mock.calls[0];
    expect(init?.method).toBe("GET");
    expect(init?.credentials).toBe("include");
    const headers = new Headers(init?.headers);
    expect(headers.get("Idempotency-Key")).toBeNull();
    expect(headers.get("X-Brand-Code")).toBeNull();
    expect(headers.get("Authorization")).toBeNull();
  });

  it("accepts an envelope and paused brands, and permits empty pages", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        ok({ success: true, data: page({ brand_status: "paused" }) }),
      )
      .mockResolvedValueOnce(ok(page({ items: [], brand_status: "paused" })));
    const api = createDrawsApi(undefined, fetcher);
    await expect(api.listResults()).resolves.toMatchObject({
      items: [draw],
      brand_status: "paused",
    });
    await expect(api.listResults()).resolves.toMatchObject({
      items: [],
      brand_status: "paused",
    });
  });

  it("parses details and period entries with their draw relationship", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        ok({ item: draw, server_time: timestamp, brand_status: "active" }),
      )
      .mockResolvedValueOnce(
        ok({
          game: draw.game,
          items: [{ period: draw.period, draw }],
          limit: 50,
          offset: 0,
          has_more: false,
          server_time: timestamp,
          brand_status: "active",
        }),
      );
    const api = createDrawsApi("brand", fetcher);
    await expect(api.getResult(resultId)).resolves.toMatchObject({
      item: draw,
    });
    await expect(api.listPeriods(gameId)).resolves.toMatchObject({
      game: draw.game,
      items: [{ period: draw.period, draw }],
    });
    expect(fetcher.mock.calls[1][0]).toBe(
      `/api/v1/b/brand/games/${gameId}/periods?limit=50&offset=0`,
    );
  });

  it("surfaces HTTP errors and rejects malformed successful responses", async () => {
    const api = createDrawsApi(
      undefined,
      vi
        .fn<typeof fetch>()
        .mockResolvedValue(
          new Response(
            JSON.stringify({
              error: { code: "not_found", message: "Not found" },
            }),
            { status: 404 },
          ),
        ),
    );
    await expect(api.getResult(resultId)).rejects.toMatchObject({
      status: 404,
      code: "not_found",
    });

    const malformedApi = createDrawsApi(
      undefined,
      vi.fn<typeof fetch>().mockResolvedValue(
        ok({
          ...page(),
          items: [{ ...draw, result: { ...draw.result, digits: null } }],
        }),
      ),
    );
    await expect(malformedApi.listResults()).rejects.toBeInstanceOf(
      BettingApiError,
    );
    await expect(malformedApi.listResults()).rejects.toMatchObject({
      code: "invalid_response",
    });
  });

  it("rejects invalid IDs, periods, limits, and offsets before fetching", async () => {
    const fetcher = vi.fn<typeof fetch>();
    const api = createDrawsApi(undefined, fetcher);
    await expect(api.getResult("../../games")).rejects.toMatchObject({
      code: "invalid_parameter",
    });
    await expect(
      api.listResults({ gameId: "not-a-uuid" }),
    ).rejects.toMatchObject({ code: "invalid_parameter" });
    await expect(api.listResults({ periodNo: "" })).rejects.toMatchObject({
      code: "invalid_parameter",
    });
    await expect(
      api.listResults({ periodNo: "界".repeat(27) }),
    ).rejects.toMatchObject({ code: "invalid_parameter" });
    await expect(api.listResults({ limit: 0 })).rejects.toMatchObject({
      code: "invalid_parameter",
    });
    await expect(api.listResults({ limit: 101 })).rejects.toMatchObject({
      code: "invalid_parameter",
    });
    await expect(api.listPeriods("bad-id")).rejects.toMatchObject({
      code: "invalid_parameter",
    });
    await expect(
      api.listPeriods(gameId, { offset: 1_000_001 }),
    ).rejects.toMatchObject({ code: "invalid_parameter" });
    expect(fetcher).not.toHaveBeenCalled();
  });

  it("encodes exact period filters and keeps explicit zero offsets", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValue(ok(page({ items: [], limit: 1 })));
    const api = createDrawsApi("a/b", fetcher);
    await api.listResults({ periodNo: "期 01", limit: 1, offset: 0 });
    expect(fetcher.mock.calls[0][0]).toBe(
      "/api/v1/b/a%2Fb/draw-results?period_no=%E6%9C%9F+01&limit=1&offset=0",
    );
  });

  it("rejects cross-game/filter mismatches and invalid period draw links", async () => {
    const otherGame = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
    const listApi = createDrawsApi(
      undefined,
      vi.fn<typeof fetch>().mockResolvedValue(
        ok(
          page({
            items: [{ ...draw, game: { ...draw.game, id: otherGame } }],
          }),
        ),
      ),
    );
    await expect(listApi.listResults({ gameId })).rejects.toMatchObject({
      code: "invalid_response",
    });

    const periodsApi = createDrawsApi(
      undefined,
      vi.fn<typeof fetch>().mockResolvedValue(
        ok({
          game: draw.game,
          items: [
            { period: { ...draw.period, draw_result_id: undefined }, draw },
          ],
          limit: 50,
          offset: 0,
          has_more: false,
          server_time: timestamp,
          brand_status: "active",
        }),
      ),
    );
    await expect(periodsApi.listPeriods(gameId)).rejects.toMatchObject({
      code: "invalid_response",
    });
  });

  it("rejects malformed model numbers, unknown states, duplicate pages, and wrong detail ids", async () => {
    for (const entry of [
      { ...draw, result: { ...draw.result, digits: [0, 10, 7] } },
      { ...draw, result: { ...draw.result, digits: [0, 7] } },
      {
        ...draw,
        game: {
          ...draw.game,
          model: { ...draw.game.model, model: "unsupported" },
        },
      },
      { ...draw, period: { ...draw.period, status: "cancelled" } },
      { ...draw, period: { ...draw.period, status: "betting" } },
      { ...draw, game: { ...draw.game, timezone: "Not/AZone" } },
    ]) {
      await expect(
        createDrawsApi(
          undefined,
          vi.fn<typeof fetch>().mockResolvedValue(ok(page({ items: [entry] }))),
        ).listResults(),
      ).rejects.toMatchObject({ status: 502, code: "invalid_response" });
    }
    await expect(
      createDrawsApi(
        undefined,
        vi
          .fn<typeof fetch>()
          .mockResolvedValue(ok(page({ items: [draw, draw] }))),
      ).listResults(),
    ).rejects.toMatchObject({ status: 502, code: "invalid_response" });
    await expect(
      createDrawsApi(
        undefined,
        vi
          .fn<typeof fetch>()
          .mockResolvedValue(
            ok({ item: draw, server_time: timestamp, brand_status: "active" }),
          ),
      ).getResult("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
    ).rejects.toMatchObject({ status: 502, code: "invalid_response" });
  });

  it("reads legitimate X+Y and M-select-N numbers without erasing repeated or ordered groups", async () => {
    const xy = {
      ...draw,
      game: {
        ...draw.game,
        model: {
          ...draw.game.model,
          model: "X_PLUS_Y",
          regular_count: 3,
          special_count: 1,
          length: 0,
          allow_repeat: false,
          ordered: true,
          regular_pool: { min: 1, max: 49, allow_repeat: true },
          special_pool: { min: 1, max: 49, allow_repeat: true },
        },
      },
      result: { regular: [8, 1, 8], special: [7], digits: [] },
    };
    const fetched = await createDrawsApi(
      undefined,
      vi.fn<typeof fetch>().mockResolvedValue(ok(page({ items: [xy] }))),
    ).listResults();
    expect(fetched.items[0].result.regular).toEqual([8, 1, 8]);
    const mn = {
      ...xy,
      game: {
        ...xy.game,
        model: {
          ...xy.game.model,
          model: "M_SELECT_N",
          total_count: 4,
          pool_size: 49,
          regular_pool: { min: 1, max: 49, allow_repeat: false },
          special_pool: { min: 1, max: 49, allow_repeat: false },
        },
      },
      result: { regular: [1, 2, 3], special: [7], digits: [] },
    };
    await expect(
      createDrawsApi(
        undefined,
        vi.fn<typeof fetch>().mockResolvedValue(ok(page({ items: [mn] }))),
      ).listResults(),
    ).resolves.toMatchObject({ items: [{ result: mn.result }] });
    const overlapping = { ...mn, result: { ...mn.result, special: [3] } };
    await expect(
      createDrawsApi(
        undefined,
        vi
          .fn<typeof fetch>()
          .mockResolvedValue(ok(page({ items: [overlapping] }))),
      ).listResults(),
    ).rejects.toMatchObject({ status: 502, code: "invalid_response" });
  });
});
