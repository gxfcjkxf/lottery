import { describe, expect, it, vi } from "vitest";
import {
  BettingApiError,
  bettingBasePath,
  createBettingApi,
  snapshotBetIntent,
  type BetInput,
  type BetQuote,
} from "./betting-api";

const ok = (data: unknown) =>
  new Response(JSON.stringify({ success: true, data }), { status: 200 });
const failure = (status: number) =>
  new Response(
    JSON.stringify({
      success: false,
      error: { code: `code_${status}`, message: `error ${status}` },
    }),
    { status },
  );
const input: BetInput = {
  period_id: "period",
  play_id: "play",
  rule_version_id: "rule",
  selection: {
    regular: [9, 1],
    special: null,
    digits: null,
    exclude: null,
    attributes: null,
    features: null,
  },
  multiplier: "2",
};
const quote: BetQuote = {
  actor_context: "opaque-server-confirmation-context",
  normalized: input.selection,
  expanded_bets: [],
  combination_count: 0,
  unit_points: "1",
  multiplier: "2",
  bet_points: "2",
  period_id: "period",
  play_id: "play",
  rule_version_id: "rule",
  definition_hash: "hash",
  policy: {
    min_bet_points: "1",
    max_bet_points: null,
    max_period_points: null,
    max_user_period_points: null,
    user_cancel_allowed: true,
  },
  policy_versions: { brand: 4, game: 7 },
  period: {
    id: "period",
    game_id: "game",
    period_no: "1",
    status: "open",
    bet_start_at: "",
    bet_end_at: "",
    draw_at: "",
  },
};

describe("betting API", () => {
  it('accepts current empty pages and explicit zero prize strings without replacements', async () => {
    const pages = createBettingApi({ fetcher: vi.fn<typeof fetch>().mockImplementation(async () => ok({ items: [], limit: 50, offset: 0 })) });
    expect((await pages.games()).items).toEqual([]);
    expect((await pages.orders()).items).toEqual([]);
    const order = { unit_points: '1', multiplier: '2', total_points: '2', prize_points: '0' };
    const api = createBettingApi({ fetcher: vi.fn<typeof fetch>().mockImplementation(async () => ok(order)) });
    expect(await api.order('order-id')).toEqual(order);
  });
  it('rejects legacy model names and missing current catalog metadata', async () => {
    const catalog = { game: { model: { model: 'DIGITS_0_9' } }, plays: [], period: null, policy: quote.policy, policy_versions: quote.policy_versions, server_time: '2026-10-10T04:00:00Z', brand_status: 'active' };
    for (const body of [{ ...catalog, game: { model: 'DIGITS_0_9' } }, { ...catalog, plays: null }, { ...catalog, period: undefined }, { ...catalog, server_time: 'invalid' }]) {
      const api = createBettingApi({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(ok(body)) });
      await expect(api.game('game-id')).rejects.toMatchObject({ status: 502, code: 'invalid_response' });
    }
  });
  it('treats malformed write replies and string-only gateway errors as unknown responses', async () => {
    for (const reply of [new Response('broken', { status: 201 }), new Response(JSON.stringify({ success: false, error: 'gateway rejection' }), { status: 400 })]) {
      const api = createBettingApi({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(reply) });
      await expect(api.place(input, 'original-key')).rejects.toMatchObject({ status: 502, code: 'invalid_response' });
    }
  });
  it('rejects bare-array and missing-items list formats instead of treating them as empty', async () => {
    for (const body of [[], {}, { items: null }]) {
      const api = createBettingApi({ fetcher: vi.fn<typeof fetch>().mockImplementation(async () => ok(body)) });
      await expect(api.games()).rejects.toMatchObject({ status: 502, code: 'invalid_response' });
      await expect(api.orders()).rejects.toMatchObject({ status: 502, code: 'invalid_response' });
    }
  });
  it('rejects numeric, missing and noncanonical order amounts in reads and write receipts', async () => {
    const order = { unit_points: '1', multiplier: '2', total_points: '2', prize_points: '0' };
    for (const field of Object.keys(order)) {
      for (const value of [0, null, undefined, '01', '1.5']) {
        const invalid = { ...order, [field]: value };
        const api = createBettingApi({ fetcher: vi.fn<typeof fetch>().mockImplementation(async () => ok(invalid)) });
        await expect(api.order('order-id')).rejects.toMatchObject({ status: 502, code: 'invalid_response' });
        await expect(api.place(input, 'original-key')).rejects.toMatchObject({ status: 502, code: 'invalid_response' });
        await expect(api.cancel('order-id', { version: 1, reason: 'Cancel owned test order' }, 'cancel-key')).rejects.toMatchObject({ status: 502, code: 'invalid_response' });
        const list = createBettingApi({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(ok({ items: [invalid] })) });
        await expect(list.orders()).rejects.toMatchObject({ status: 502, code: 'invalid_response' });
      }
    }
  });
  it("uses brand encoded endpoints and cookie credentials without session tokens", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValue(ok({ items: [], limit: 50, offset: 0 }));
    await createBettingApi({ brandCode: "north star", fetcher }).games();
    expect(bettingBasePath("north star")).toBe("/api/v1/b/north%20star");
    expect(fetcher.mock.calls[0][0]).toBe(
      "/api/v1/b/north%20star/games?limit=50&offset=0",
    );
    expect(fetcher.mock.calls[0][1]?.credentials).toBe("same-origin");
    const headers = new Headers(fetcher.mock.calls[0][1]?.headers);
    expect(headers.get("Authorization")).toBeNull();
    expect(headers.get("Cookie")).toBeNull();
  });

  it("sends a reusable caller key and preserves arbitrary precision point strings", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockImplementation(async () => ok({ unit_points: '1', multiplier: '2', total_points: "9007199254740993", prize_points: '0' }));
    const api = createBettingApi({ fetcher });
    const placed = await api.place(input, "caller-stable-key");
    await api.place(input, "caller-stable-key");
    expect(placed.total_points).toBe("9007199254740993");
    expect(
      fetcher.mock.calls.map(([, init]) =>
        new Headers(init?.headers).get("Idempotency-Key"),
      ),
    ).toEqual(["caller-stable-key", "caller-stable-key"]);
    expect(JSON.parse(String(fetcher.mock.calls[0][1]?.body)).multiplier).toBe(
      "2",
    );
  });

  it("uses the public game detail route without an invented catalog suffix", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok({
      game: { model: { model: 'DIGITS_0_9' } }, plays: [], period: null,
      policy: quote.policy, policy_versions: quote.policy_versions,
      server_time: '2026-10-10T04:00:00Z', brand_status: 'active',
    }));
    await createBettingApi({ fetcher }).game("game-id");
    expect(fetcher.mock.calls[0][0]).toBe("/api/v1/games/game-id");
  });

  it("rejects noncanonical or overflowing multipliers before network access", async () => {
    const fetcher = vi.fn<typeof fetch>();
    const api = createBettingApi({ fetcher });
    for (const multiplier of ["0", "01", "+1", "1.0", "9223372036854775808"]) {
      expect(() => api.preview({ ...input, multiplier })).toThrow(
        BettingApiError,
      );
    }
    expect(fetcher).not.toHaveBeenCalled();
  });

  it("surfaces structured 401 and 409 errors, and malformed successful JSON", async () => {
    for (const status of [401, 409]) {
      const api = createBettingApi({
        fetcher: vi.fn<typeof fetch>().mockResolvedValue(failure(status)),
      });
      await expect(api.orders()).rejects.toMatchObject({
        status,
        code: `code_${status}`,
      });
    }
    const malformed = createBettingApi({
      fetcher: vi
        .fn<typeof fetch>()
        .mockResolvedValue(new Response("not json", { status: 200 })),
    });
    await expect(malformed.orders()).rejects.toMatchObject({ status: 502, code: 'invalid_response' });
  });

  it("snapshots a canonical detached intent and captures quote policy versions once", () => {
    const source: BetInput = structuredClone(input);
    const snapshot = snapshotBetIntent(source, quote);
    expect(snapshot.body.policy_versions).toEqual({ brand: 4, game: 7 });
    expect(snapshot.body.actor_context).toBe(quote.actor_context);
    expect(JSON.stringify(snapshot.body.selection)).toBe(
      '{"attributes":null,"digits":null,"exclude":null,"features":null,"regular":[9,1],"special":null}',
    );
    source.selection.regular?.push(3);
    expect(snapshot.body.selection.regular).toEqual([9, 1]);
    expect(snapshot.key).toMatch(/^bet:/);
    expect(() =>
      snapshotBetIntent(input, { ...quote, play_id: "another" }),
    ).toThrow(/Quote does not match/);
  });
});
