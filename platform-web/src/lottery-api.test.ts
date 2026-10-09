import { describe, expect, it, vi } from "vitest";
import { createPlatformLotteryApi } from "./lottery-api";
import { PlatformApiError } from "./platform-api";

const brand = "11111111-1111-4111-8111-111111111111";
const gameId = "22222222-2222-4222-8222-222222222222";
const periodId = "33333333-3333-4333-8333-333333333333";
const sourceId = "44444444-4444-4444-8444-444444444444";
const timestamp = "2026-10-08T12:00:00Z";
const game = {
  id: gameId, brand_id: brand, code: "pick3", name: "Pick Three", timezone: "Asia/Singapore",
  status: "active", version: 2, started_sequence: 8,
  model: { model: "DIGITS_0_9", regular_pool: { allow_repeat: false }, special_pool: { allow_repeat: false }, regular_count: 0, special_count: 0, pool_size: 0, total_count: 0, length: 3, allow_repeat: true, ordered: true },
};
const period = {
  id: periodId, brand_id: brand, game_id: gameId, period_no: "20261008001", sequence: 9,
  bet_start_at: timestamp, bet_end_at: timestamp, draw_at: timestamp, status: "drawn", version: 1,
  state_reason: "", schedule_id: "55555555-5555-4555-8555-555555555555",
};
const result = {
  id: "66666666-6666-4666-8666-666666666666", brand_id: brand, game_id: gameId, period_id: periodId,
  source_id: sourceId, kind: "manual", result: { regular: [], special: [], digits: [0, 9, 2] },
  result_hash: "hash", drawn_at: timestamp, created_at: timestamp,
};
const ok = (data: unknown, status = 200) => new Response(JSON.stringify({ success: true, data }), { status });
const page = (data: unknown, limit = 51, offset = 0) => ({ ...data as object, limit, offset });

describe("platform lottery read API", () => {
  it("uses exact GET routes, selected brand headers, and parses current DTOs", async () => {
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(ok(page({ games: [game] })))
      .mockResolvedValueOnce(ok(page({ periods: [period] })))
      .mockResolvedValueOnce(ok({ current: result, history: [result], attempts: [], limit: 50, offset: 0 }, 200));
    const api = createPlatformLotteryApi(fetcher);
    const games = await api.games(brand);
    const periods = await api.periods(brand, gameId);
    const draws = await api.draw(brand, periodId);
    expect(games[0]).toMatchObject({ id: gameId, model: { model: "DIGITS_0_9", length: 3 } });
    expect(periods[0]).toMatchObject({ id: periodId, game_id: gameId });
    expect(draws.current?.result.digits).toEqual([0, 9, 2]);
    expect(fetcher.mock.calls.map(call => call[0])).toEqual([
      "/api/v1/platform/games?limit=51&offset=0",
      `/api/v1/platform/games/${gameId}/periods?limit=51&offset=0`,
      `/api/v1/platform/periods/${periodId}/draw?limit=50&offset=0`,
    ]);
    for (const [, init] of fetcher.mock.calls) {
      expect(init?.method).toBe("GET");
      expect(init?.credentials).toBe("same-origin");
      expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    }
  });

  it("accepts null current and validates returned scopes and matching pagination", async () => {
    const lookahead = vi.fn<typeof fetch>().mockResolvedValue(ok({ current: null, history: [], attempts: [], limit: 51, offset: 50 }));
    await expect(createPlatformLotteryApi(lookahead).draw(brand, periodId, 51, 50)).resolves.toMatchObject({ limit: 51, offset: 50 });
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok({ current: null, history: [], attempts: [], limit: 50, offset: 0 }));
    const response = await createPlatformLotteryApi(fetcher).draw(brand, periodId);
    expect(response.current).toBeNull();
    const mismatchedPage = vi.fn<typeof fetch>().mockResolvedValue(ok({ current: null, history: [], attempts: [], limit: 50, offset: 4 }));
    await expect(createPlatformLotteryApi(mismatchedPage).draw(brand, periodId)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(createPlatformLotteryApi(vi.fn<typeof fetch>().mockResolvedValue(ok({ current: null, history: [{ ...result, brand_id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" }], attempts: [], limit: 50, offset: 0 }))).draw(brand, periodId)).rejects.toBeInstanceOf(PlatformApiError);
    await expect(createPlatformLotteryApi(vi.fn<typeof fetch>().mockResolvedValue(ok(page({ periods: [{ ...period, game_id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" }] })))).periods(brand, gameId)).rejects.toBeInstanceOf(PlatformApiError);
  });

  it("rejects malformed DTOs, absent brand selection, and permission failures", async () => {
    await expect(createPlatformLotteryApi(vi.fn<typeof fetch>()).games(" ")).rejects.toMatchObject({ code: "BRAND_REQUIRED" });
    await expect(createPlatformLotteryApi(vi.fn<typeof fetch>().mockResolvedValue(ok(page({ games: [{ ...game, version: "2" }] })))).games(brand)).rejects.toBeInstanceOf(PlatformApiError);
    const denied = new Response(JSON.stringify({ success: false, error: { code: "PERMISSION_DENIED", message: "Denied" } }), { status: 403 });
    await expect(createPlatformLotteryApi(vi.fn<typeof fetch>().mockResolvedValue(denied)).games(brand)).rejects.toMatchObject({ status: 403, code: "PERMISSION_DENIED" });
  });
});
