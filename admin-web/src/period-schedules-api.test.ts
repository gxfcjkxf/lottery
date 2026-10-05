import { describe, expect, it, vi } from "vitest";
import {
  buildGeneratePeriodsBody,
  buildUpdateScheduleBody,
  createPeriodScheduleKeyTracker,
  createPeriodScheduleRequestGuard,
  createPeriodSchedulesApi,
  defaultScheduleSpec,
  parseGame,
  parsePeriod,
  parseScheduleRecord,
  periodSchedulePermissions,
  validateScheduleSpec,
  zonedDateTimeToUtc,
  type Game,
  type Period,
  type ScheduleRecord,
} from "./period-schedules-api";

const brand = "brand-a";
const game: Game = {
  id: "game-1",
  brand_id: brand,
  code: "daily",
  name: "每日彩",
  timezone: "Asia/Shanghai",
  version: 4,
  status: "active",
};
const spec = defaultScheduleSpec(game.timezone);
const schedule: ScheduleRecord = {
  id: "schedule-1",
  brand_id: brand,
  game_id: game.id,
  revision: 3,
  spec,
  game_version: 4,
  created_at: "2026-10-06T01:02:03Z",
};
const period: Period = {
  id: "period-1",
  brand_id: brand,
  game_id: game.id,
  period_no: "20261006001",
  sequence: 1,
  bet_start_at: "2026-10-05T11:00:00Z",
  bet_end_at: "2026-10-06T10:55:00Z",
  draw_at: "2026-10-06T11:00:00Z",
  status: "scheduled",
  version: 1,
  schedule_id: schedule.id,
  state_reason: "按排期生成",
};
const account = (permissions: string[], extra: Record<string, unknown> = {}) => ({
  id: "operator",
  super_admin: false,
  brand_ids: [brand],
  permissions: [],
  permissions_by_brand: { [brand]: permissions },
  platform_permissions: [],
  ...extra,
});
const ok = (data: unknown) =>
  new Response(JSON.stringify({ success: true, data }), { status: 200 });
const fail = (status: number) =>
  new Response(
    JSON.stringify({ success: false, error: { code: `E${status}`, message: "拒绝" } }),
    { status },
  );

describe("period schedule API", () => {
  it("sends brand-scoped same-origin reads and exact paths, and treats only schedule 404 as no saved schedule", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(ok({ games: [game] }))
      .mockResolvedValueOnce(new Response("", { status: 404 }))
      .mockResolvedValueOnce(ok({ periods: [period], limit: 25, offset: 0 }));
    const api = createPeriodSchedulesApi(fetcher);
    expect((await api.getGames(brand)).games[0]).toEqual(game);
    expect(await api.getSchedule(brand, game.id)).toBeNull();
    expect((await api.getPeriods(brand, game.id)).periods).toEqual([period]);
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
      "/api/v1/admin/games?limit=25&offset=0",
      "/api/v1/admin/games/game-1/schedule",
      "/api/v1/admin/games/game-1/periods?limit=25&offset=0",
    ]);
    for (const [, init] of fetcher.mock.calls) {
      const headers = new Headers(init?.headers);
      expect(init?.credentials).toBe("same-origin");
      expect(headers.get("X-Brand-ID")).toBe(brand);
      expect(headers.get("Authorization")).toBeNull();
      expect(headers.get("Idempotency-Key")).toBeNull();
    }
  });

  it("uses exact PUT/POST bodies, encoded game IDs, JSON headers and retry keys", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(ok(schedule))
      .mockResolvedValueOnce(ok({ created: 1, existing: 2, periods: [period] }));
    const api = createPeriodSchedulesApi(fetcher);
    const updateBody = buildUpdateScheduleBody(4, spec, "运营排期");
    await api.updateSchedule(brand, "game/a", updateBody, "schedule-key");
    const generationBody = {
      from: "2026-10-05T16:00:00.000Z",
      to: "2026-10-06T16:00:00.000Z",
      reason: "补齐期数",
    };
    const generated = await api.generatePeriods(
      brand,
      "game/a",
      generationBody,
      "Asia/Shanghai",
      "generate-key",
    );
    expect(generated).toEqual({ created: 1, existing: 2, periods: [period] });
    const expectations = [
      ["/api/v1/admin/games/game%2Fa/schedule", "PUT", updateBody, "schedule-key"],
      ["/api/v1/admin/games/game%2Fa/periods/generate", "POST", generationBody, "generate-key"],
    ] as const;
    fetcher.mock.calls.forEach(([url, init], index) => {
      const [path, method, body, key] = expectations[index];
      const headers = new Headers(init?.headers);
      expect(url).toBe(path);
      expect(init?.method).toBe(method);
      expect(init?.credentials).toBe("same-origin");
      expect(headers.get("Content-Type")).toBe("application/json");
      expect(headers.get("X-Brand-ID")).toBe(brand);
      expect(headers.get("Idempotency-Key")).toBe(key);
      expect(JSON.parse(String(init?.body))).toEqual(body);
    });
  });

  it("validates schedule fields, canonical integer inputs, dates, weekdays and required reasons", () => {
    expect(validateScheduleSpec(spec)).toEqual(spec);
    expect(() => validateScheduleSpec({ ...spec, mode: "cron" } as never)).toThrow();
    expect(() => validateScheduleSpec({ ...spec, timezone: "Not/AZone" })).toThrow();
    expect(() => validateScheduleSpec({ ...spec, timezone: "Local" })).toThrow();
    expect(() => validateScheduleSpec({ ...spec, timezone: "A".repeat(101) })).toThrow();
    expect(validateScheduleSpec({ ...spec, timezone: "UTC" }).timezone).toBe("UTC");
    expect(() => validateScheduleSpec({ ...spec, daily_draw_times: ["25:00:00"] })).toThrow();
    expect(() => validateScheduleSpec({ ...spec, interval_seconds: 3600 })).toThrow();
    expect(() => validateScheduleSpec({ ...spec, weekdays: [7] })).toThrow();
    expect(() => validateScheduleSpec({ ...spec, weekdays: [] })).toThrow();
    expect(() => validateScheduleSpec({ ...spec, bet_open_before_seconds: 0 })).toThrow();
    expect(() => validateScheduleSpec({ ...spec, bet_close_before_seconds: 86400 })).toThrow();
    expect(() => validateScheduleSpec({ ...spec, pause_dates: ["2026-02-30"] })).toThrow();
    const interval = {
      ...spec,
      mode: "interval" as const,
      daily_draw_times: [],
      interval_seconds: 3600,
      busy_windows: [
        { start: "08:00:00", end: "09:00:00", interval_seconds: 1800 },
        { start: "08:30:00", end: "10:00:00", interval_seconds: 1200 },
      ],
    };
    expect(() => validateScheduleSpec(interval)).toThrow("不能重叠");
    expect(() => validateScheduleSpec({ ...interval, busy_windows: [{ ...interval.busy_windows[0], start: "10:00:00", end: "09:00:00" }] })).toThrow();
    expect(() => validateScheduleSpec({ ...interval, interval_seconds: 29, busy_windows: [] })).toThrow();
    expect(() => validateScheduleSpec({ ...interval, daily_draw_times: ["12:00:00"], busy_windows: [] })).toThrow();
    expect(() => validateScheduleSpec({ ...interval, busy_windows: [{ ...interval.busy_windows[0], interval_seconds: 29 }] })).toThrow();
    expect(() => validateScheduleSpec({ ...interval, interval_seconds: 3600, busy_windows: Array.from({ length: 129 }, () => ({ start: "08:00:00", end: "09:00:00", interval_seconds: 3600 })) })).toThrow();
    expect(validateScheduleSpec({ ...interval, busy_windows: [] }).interval_seconds).toBe(3600);
    expect(() => buildUpdateScheduleBody(0, spec, "reason")).toThrow();
    expect(() => buildUpdateScheduleBody(4, spec, "  ")).toThrow();
    expect(buildUpdateScheduleBody(4, spec, "  调整  ").reason).toBe("调整");
  });

  it("converts local wall time using the selected IANA zone and rejects invalid ranges and DST gaps", () => {
    expect(zonedDateTimeToUtc("2026-10-06T19:00", "Asia/Shanghai")).toBe(
      "2026-10-06T11:00:00.000Z",
    );
    expect(
      buildGeneratePeriodsBody(
        "2026-10-06T19:00",
        "2026-10-07T19:00",
        "Asia/Shanghai",
        " backfill ",
      ),
    ).toEqual({
      from: "2026-10-06T11:00:00.000Z",
      to: "2026-10-07T11:00:00.000Z",
      reason: "backfill",
    });
    expect(() =>
      buildGeneratePeriodsBody("2026-10-07T19:00", "2026-10-06T19:00", "Asia/Shanghai", "x"),
    ).toThrow();
    expect(() =>
      buildGeneratePeriodsBody("2026-10-01T00:00", "2026-10-09T00:00", "UTC", "x"),
    ).toThrow("单次生成范围不能超过 7 天");
    expect(() => zonedDateTimeToUtc("2026-03-08T02:30", "America/New_York")).toThrow();
    expect(zonedDateTimeToUtc("2026-10-25T02:30", "Europe/Berlin")).toBe(
      "2026-10-25T00:30:00.000Z",
    );
    expect(zonedDateTimeToUtc("2026-11-01T01:30", "America/New_York")).toBe(
      "2026-11-01T05:30:00.000Z",
    );
  });

  it("scopes idempotency keys to brand, operation, target and stable body", () => {
    const keyFor = createPeriodScheduleKeyTracker();
    const body = { version: 4, spec, reason: "update" };
    const first = keyFor(brand, "schedule.update", game.id, body);
    expect(keyFor(brand, "schedule.update", game.id, { ...body })).toBe(first);
    expect(keyFor(brand, "schedule.update", game.id, { ...body, reason: "retry edit" })).not.toBe(first);
    expect(keyFor("brand-b", "schedule.update", game.id, body)).not.toBe(first);
    expect(keyFor(brand, "period.generate", game.id, body)).not.toBe(first);
    expect(keyFor(brand, "schedule.update", "game-2", body)).not.toBe(first);
  });

  it("allows independent view grants and brand-only writes; Super Admin never gains schedule writes", () => {
    const all = periodSchedulePermissions(
      account(["schedule.view.brand", "schedule.write.brand", "period.view.brand", "period.generate.brand"]),
      brand,
    );
    expect(all).toEqual({ gameCatalogView: false, scheduleView: true, scheduleWrite: true, periodView: true, periodGenerate: true });
    const platformView = periodSchedulePermissions(
      account([], { platform_permissions: ["schedule.view.platform", "period.view.platform", "schedule.write.platform", "period.generate.platform"] }),
      brand,
    );
    expect(platformView).toEqual({ gameCatalogView: false, scheduleView: true, scheduleWrite: false, periodView: true, periodGenerate: false });
    const superAdmin = periodSchedulePermissions(
      account(["schedule.write.brand", "period.generate.brand"], { super_admin: true }),
      brand,
    );
    expect(superAdmin.scheduleWrite).toBe(false);
    expect(superAdmin.periodGenerate).toBe(false);
    expect(periodSchedulePermissions(account(["schedule.view.brand"]), "").scheduleView).toBe(false);
    expect(
      periodSchedulePermissions(account(["game.view.brand", "schedule.write.brand"]), brand),
    ).toMatchObject({ gameCatalogView: true, scheduleView: false, scheduleWrite: true });
  });

  it("discards stale, cross-scope and revoked-permission responses", () => {
    const guard = createPeriodScheduleRequestGuard();
    const old = guard.capture("schedule", "brand-a/game-1");
    const latest = guard.capture("schedule", "brand-a/game-1");
    expect(guard.isCurrent(old, "brand-a/game-1", true)).toBe(false);
    expect(guard.isCurrent(latest, "brand-a/game-2", true)).toBe(false);
    expect(guard.isCurrent(latest, "brand-a/game-1", false)).toBe(false);
    expect(guard.isCurrent(latest, "brand-a/game-1", true)).toBe(true);
    guard.invalidate("schedule");
    expect(guard.isCurrent(latest, "brand-a/game-1", true)).toBe(false);
  });

  it("rejects malformed or cross-shape API responses and preserves auth/conflict statuses", async () => {
    expect(() => parseGame({ ...game, version: "4" })).toThrow();
    expect(() => parseScheduleRecord({ ...schedule, revision: 0 })).toThrow();
    expect(() => parsePeriod({ ...period, draw_at: "yesterday" })).toThrow();
    const malformed = createPeriodSchedulesApi(
      vi.fn<typeof fetch>().mockResolvedValue(ok({ games: [{}] })),
    );
    await expect(malformed.getGames(brand)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    for (const status of [401, 403, 409]) {
      const api = createPeriodSchedulesApi(vi.fn<typeof fetch>().mockResolvedValue(fail(status)));
      await expect(api.getGames(brand)).rejects.toMatchObject({ status, code: `E${status}` });
    }
  });

  it("rejects requests that violate backend limits before issuing a write", async () => {
    const fetcher = vi.fn<typeof fetch>();
    const api = createPeriodSchedulesApi(fetcher);
    const badDaily = { ...spec, interval_seconds: 3600 };
    expect(() =>
      api.updateSchedule(brand, game.id, { version: game.version, spec: badDaily, reason: "bad" }, "key"),
    ).toThrow("interval_seconds");
    expect(() =>
      api.generatePeriods(
        brand,
        game.id,
        {
          from: "2026-10-05T16:00:00.000Z",
          to: "2026-10-13T16:00:00.000Z",
          reason: "bad batch",
        },
        game.timezone,
        "key",
      ),
    ).toThrow("单次生成范围不能超过 7 天");
    expect(fetcher).not.toHaveBeenCalled();
  });

  it("accepts aggregate generation counts independently of the capped result list and rejects more than 100 returned rows", async () => {
    const result = {
      created: 120,
      existing: 80,
      periods: Array.from({ length: 100 }, (_, index) => ({
        ...period,
        id: `period-${index}`,
      })),
    };
    const api = createPeriodSchedulesApi(vi.fn<typeof fetch>().mockResolvedValue(ok(result)));
    const generated = await api.generatePeriods(
      brand,
      game.id,
      {
        from: "2026-10-05T16:00:00.000Z",
        to: "2026-10-06T16:00:00.000Z",
        reason: "page aggregate",
      },
      game.timezone,
      "key",
    );
    expect(generated.created).toBe(120);
    expect(generated.existing).toBe(80);
    expect(generated.periods).toHaveLength(100);

    const tooMany = createPeriodSchedulesApi(
      vi.fn<typeof fetch>().mockResolvedValue(
        ok({ created: 101, existing: 0, periods: Array.from({ length: 101 }, () => period) }),
      ),
    );
    await expect(
      tooMany.generatePeriods(
        brand,
        game.id,
        {
          from: "2026-10-05T16:00:00.000Z",
          to: "2026-10-06T16:00:00.000Z",
          reason: "oversized response",
        },
        game.timezone,
        "key",
      ),
    ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });
});
