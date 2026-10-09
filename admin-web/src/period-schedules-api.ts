import {
  AdminApiError,
  createIdempotencyKey,
  type AdminAccount,
} from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";

const BASE = "/api/v1/admin";

export interface PeriodScheduleAccount extends AdminAccount {}

export interface Game {
  id: string;
  brand_id: string;
  code: string;
  name: string;
  timezone: string;
  version: number;
  status?: string;
}

export interface BusyWindow {
  start: string;
  end: string;
  interval_seconds: number;
}

export interface ScheduleSpec {
  timezone: string;
  mode: "daily" | "interval";
  daily_draw_times: string[];
  interval_seconds: number;
  busy_windows: BusyWindow[];
  bet_open_before_seconds: number;
  bet_close_before_seconds: number;
  pause_dates: string[];
  weekdays: number[];
  holiday_dates: string[];
  holiday_policy: "skip" | "normal";
}

export interface ScheduleRecord {
  id: string;
  brand_id: string;
  game_id: string;
  revision: number;
  spec: ScheduleSpec;
  game_version: number;
  created_at: string;
}

export interface Period {
  id: string;
  brand_id: string;
  game_id: string;
  period_no: string;
  sequence: number;
  bet_start_at: string;
  bet_end_at: string;
  draw_at: string;
  status: string;
  version: number;
  schedule_id?: string;
  state_reason?: string;
}

export interface UpdateScheduleBody {
  version: number;
  spec: ScheduleSpec;
  reason: string;
}

export interface GeneratePeriodsBody {
  from: string;
  to: string;
  reason: string;
}

export interface GeneratePeriodsResult {
  created: number;
  existing: number;
  periods: Period[];
}

export interface PeriodSchedulePermissions {
  gameCatalogView: boolean;
  scheduleView: boolean;
  scheduleWrite: boolean;
  periodView: boolean;
  periodGenerate: boolean;
}

export function periodSchedulePermissions(
  account: PeriodScheduleAccount,
  brandId: string,
): PeriodSchedulePermissions {
  if (!brandId.trim())
    return {
      gameCatalogView: false,
      scheduleView: false,
      scheduleWrite: false,
      periodView: false,
      periodGenerate: false,
    };
  const brand = brandPermissionSet(account, brandId);
  return {
    gameCatalogView: brand.has("game.view.brand"),
    scheduleView: brand.has("schedule.view.brand"),
    scheduleWrite:
      !account.super_admin && brand.has("schedule.write.brand"),
    periodView: brand.has("period.view.brand"),
    periodGenerate:
      !account.super_admin && brand.has("period.generate.brand"),
  };
}

/** Exact serialized request bodies retain their retry key within an operation scope. */
export function createPeriodScheduleKeyTracker() {
  const entries = new Map<string, { body: string; key: string }>();
  return (brandId: string, operation: string, targetId: string, body: unknown) => {
    const scope = JSON.stringify([brandId, operation, targetId]);
    const serialized = stableJson(body);
    const previous = entries.get(scope);
    if (previous?.body === serialized) return previous.key;
    const key = createIdempotencyKey();
    entries.set(scope, { body: serialized, key });
    return key;
  };
}

/** Tickets discard superseded responses and responses crossing a brand or permission change. */
export function createPeriodScheduleRequestGuard() {
  const generations = new Map<string, number>();
  type Ticket = { lane: string; generation: number; scope: string };
  return {
    capture(lane: string, scope: string): Ticket {
      const generation = (generations.get(lane) ?? 0) + 1;
      generations.set(lane, generation);
      return { lane, generation, scope };
    },
    invalidate(lane?: string) {
      if (lane) generations.set(lane, (generations.get(lane) ?? 0) + 1);
      else
        for (const name of generations.keys())
          generations.set(name, generations.get(name)! + 1);
    },
    isCurrent(ticket: Ticket, scope: string, permitted: boolean) {
      return (
        permitted &&
        ticket.scope === scope &&
        generations.get(ticket.lane) === ticket.generation
      );
    },
  };
}

const timePattern = /^(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d$/;
const datePattern = /^\d{4}-\d{2}-\d{2}$/;

function fail(message: string): never {
  throw new AdminApiError(message, 0, "INVALID_INPUT");
}

function validDate(value: string): boolean {
  if (!datePattern.test(value)) return false;
  const date = new Date(`${value}T00:00:00.000Z`);
  return Number.isFinite(date.valueOf()) && date.toISOString().slice(0, 10) === value;
}

function canonicalInteger(value: number, minimum = 0): boolean {
  return Number.isSafeInteger(value) && value >= minimum;
}

function isIanaTimezone(value: unknown): value is string {
  if (
    typeof value !== "string" ||
    !value ||
    value.length > 100 ||
    value === "Local" ||
    value.trim() !== value
  )
    return false;
  try {
    new Intl.DateTimeFormat("en", { timeZone: value });
    return true;
  } catch {
    return false;
  }
}

export function validateScheduleSpec(value: ScheduleSpec): ScheduleSpec {
  if (!value || typeof value !== "object" || Array.isArray(value))
    fail("排期内容格式无效。");
  if (!isIanaTimezone(value.timezone))
    fail("请填写有效的 IANA 时区，例如 Asia/Shanghai。");
  if (value.mode !== "daily" && value.mode !== "interval")
    fail("排期模式无效。");
  if (
    !Array.isArray(value.daily_draw_times) ||
    value.daily_draw_times.length > 2880 ||
    value.daily_draw_times.some((time) => typeof time !== "string" || !timePattern.test(time)) ||
    new Set(value.daily_draw_times).size !== value.daily_draw_times.length
  )
    fail("每日开奖时间须使用不重复的 HH:MM:SS。");
  if (!Number.isSafeInteger(value.interval_seconds))
    fail("基准间隔须为规范整数秒。");
  if (
    !Array.isArray(value.busy_windows) ||
    value.busy_windows.some(
      (window) =>
        !window ||
        typeof window !== "object" ||
        !timePattern.test(window.start) ||
        !timePattern.test(window.end) ||
        window.start >= window.end ||
        !canonicalInteger(window.interval_seconds, 30) ||
        window.interval_seconds > 86400,
    ) ||
    value.busy_windows.length > 128
  )
    fail("繁忙时段须填写有效的开始、结束时间和正整数秒间隔。");
  const sortedWindows = [...value.busy_windows].sort((a, b) => a.start.localeCompare(b.start));
  for (let index = 1; index < sortedWindows.length; index += 1)
    if (sortedWindows[index].start < sortedWindows[index - 1].end)
      fail("繁忙时段不能重叠。");
  if (
    !canonicalInteger(value.bet_open_before_seconds, 1) ||
    value.bet_open_before_seconds > 86400 ||
    !canonicalInteger(value.bet_close_before_seconds) ||
    value.bet_close_before_seconds >= value.bet_open_before_seconds
  )
    fail("开放提前秒数须为 1 至 86400；截止提前秒数须为非负且小于开放提前秒数。");
  for (const [label, dates] of [
    ["暂停日期", value.pause_dates],
    ["节假日", value.holiday_dates],
  ] as const) {
    if (
      !Array.isArray(dates) ||
      dates.length > 10000 ||
      dates.some((date) => typeof date !== "string" || !validDate(date)) ||
      new Set(dates).size !== dates.length
    )
      fail(`${label}须为不重复的 YYYY-MM-DD 日期。`);
  }
  if (
    !Array.isArray(value.weekdays) ||
    value.weekdays.length === 0 ||
    value.weekdays.length > 7 ||
    value.weekdays.some((day) => !Number.isInteger(day) || day < 0 || day > 6) ||
    new Set(value.weekdays).size !== value.weekdays.length
  )
    fail("开奖星期须为不重复的 0 至 6。" );
  if (value.holiday_policy !== "skip" && value.holiday_policy !== "normal")
    fail("节假日策略无效。");
  if (value.mode === "daily") {
    if (value.daily_draw_times.length === 0)
      fail("每日排期至少需要一个开奖时间。");
    if (value.interval_seconds !== 0 || value.busy_windows.length !== 0)
      fail("每日模式的 interval_seconds 必须为 0，且不能设置繁忙时段。");
  } else {
    if (value.daily_draw_times.length !== 0)
      fail("间隔模式不能设置每日开奖时间。");
    if (!canonicalInteger(value.interval_seconds, 30) || value.interval_seconds > 86400)
      fail("间隔模式的基准间隔须为 30 至 86400 秒。");
  }
  return {
    timezone: value.timezone.trim(),
    mode: value.mode,
    daily_draw_times: [...value.daily_draw_times],
    interval_seconds: value.interval_seconds,
    busy_windows: value.busy_windows.map((window) => ({ ...window })),
    bet_open_before_seconds: value.bet_open_before_seconds,
    bet_close_before_seconds: value.bet_close_before_seconds,
    pause_dates: [...value.pause_dates],
    weekdays: [...value.weekdays].sort((a, b) => a - b),
    holiday_dates: [...value.holiday_dates],
    holiday_policy: value.holiday_policy,
  };
}

export function buildUpdateScheduleBody(
  version: number,
  spec: ScheduleSpec,
  reason: string,
): UpdateScheduleBody {
  if (!canonicalInteger(version, 1)) fail("彩种版本无效，请重新读取。");
  if (!reason.trim()) fail("保存排期前必须填写原因。");
  return {
    version,
    spec: validateScheduleSpec(spec),
    reason: reason.trim(),
  };
}

function parseLocalDateTime(value: string): number[] | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/.exec(value);
  if (!match) return null;
  return match.slice(1).map((part, index) => Number(part ?? (index === 5 ? "0" : "0")));
}

/** Convert a local wall-clock value to UTC in the selected IANA zone, rejecting DST gaps. */
export function zonedDateTimeToUtc(value: string, timezone: string): string {
  const parts = parseLocalDateTime(value);
  if (!parts) fail("请选择有效的本地日期和时间。");
  if (!isIanaTimezone(timezone)) fail("请填写有效的 IANA 时区，例如 Asia/Shanghai。");
  const [year, month, day, hour, minute, second] = parts;
  const target = Date.UTC(year, month - 1, day, hour, minute, second);
  const checkDate = new Date(Date.UTC(year, month - 1, day));
  if (
    checkDate.getUTCFullYear() !== year ||
    checkDate.getUTCMonth() + 1 !== month ||
    checkDate.getUTCDate() !== day ||
    hour > 23 ||
    minute > 59 ||
    second > 59
  )
    fail("请选择有效的本地日期和时间。");
  const formatter = new Intl.DateTimeFormat("en-CA", {
    timeZone: timezone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hourCycle: "h23",
  });
  const matches: number[] = [];
  const sampleRange = 48 * 60 * 60 * 1000;
  const sampleStep = 30 * 60 * 1000;
  for (let delta = -sampleRange; delta <= sampleRange; delta += sampleStep) {
    const sample = target + delta;
    const partsAtSample = Object.fromEntries(
      formatter
        .formatToParts(new Date(sample))
        .map((part) => [part.type, part.value]),
    );
    const represented = Date.UTC(
      Number(partsAtSample.year),
      Number(partsAtSample.month) - 1,
      Number(partsAtSample.day),
      Number(partsAtSample.hour),
      Number(partsAtSample.minute),
      Number(partsAtSample.second),
    );
    const candidate = target - (represented - sample);
    const partsAtCandidate = Object.fromEntries(
      formatter
        .formatToParts(new Date(candidate))
        .map((part) => [part.type, part.value]),
    );
    if (
      Number(partsAtCandidate.year) === year &&
      Number(partsAtCandidate.month) === month &&
      Number(partsAtCandidate.day) === day &&
      Number(partsAtCandidate.hour) === hour &&
      Number(partsAtCandidate.minute) === minute &&
      Number(partsAtCandidate.second) === second
    )
      matches.push(candidate);
  }
  if (matches.length === 0)
    fail("该本地时间因夏令时切换不存在，请选择其他时间。");
  return new Date(Math.min(...matches)).toISOString();
}

export function buildGeneratePeriodsBody(
  fromLocal: string,
  toLocal: string,
  timezone: string,
  reason: string,
): GeneratePeriodsBody {
  if (!reason.trim()) fail("生成期数前必须填写原因。");
  const from = zonedDateTimeToUtc(fromLocal, timezone);
  const to = zonedDateTimeToUtc(toLocal, timezone);
  const span = Date.parse(to) - Date.parse(from);
  if (span <= 0) fail("结束时间必须晚于开始时间。");
  if (span > 7 * 24 * 60 * 60 * 1000) fail("单次生成范围不能超过 7 天。");
  return { from, to, reason: reason.trim() };
}

function validateUtcGenerationBody(body: GeneratePeriodsBody): GeneratePeriodsBody {
  if (!body || typeof body !== "object") fail("生成范围格式无效。");
  if (!body.reason?.trim()) fail("生成期数前必须填写原因。");
  for (const value of [body.from, body.to]) {
    if (
      typeof value !== "string" ||
      !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/.test(value) ||
      !Number.isFinite(Date.parse(value)) ||
      new Date(value).toISOString() !== value
    )
      fail("生成范围必须使用规范 UTC ISO 时间。");
  }
  const span = Date.parse(body.to) - Date.parse(body.from);
  if (span <= 0) fail("结束时间必须晚于开始时间。");
  if (span > 7 * 24 * 60 * 60 * 1000) fail("单次生成范围不能超过 7 天。");
  return { from: body.from, to: body.to, reason: body.reason.trim() };
}

export function defaultScheduleSpec(timezone: string): ScheduleSpec {
  return {
    timezone,
    mode: "daily",
    daily_draw_times: ["19:00:00"],
    interval_seconds: 0,
    busy_windows: [],
    bet_open_before_seconds: 86400,
    bet_close_before_seconds: 300,
    pause_dates: [],
    weekdays: [0, 1, 2, 3, 4, 5, 6],
    holiday_dates: [],
    holiday_policy: "skip",
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}
function stringField(value: unknown, name: string): value is string {
  return typeof value === "string" && (name === "state_reason" || value.length > 0);
}
function numberField(value: unknown, minimum = 0): value is number {
  return Number.isSafeInteger(value) && Number(value) >= minimum;
}

export function parseGame(value: unknown): Game {
  if (
    !isRecord(value) ||
    !stringField(value.id, "id") ||
    !stringField(value.brand_id, "brand_id") ||
    !stringField(value.code, "code") ||
    !stringField(value.name, "name") ||
    !isIanaTimezone(value.timezone) ||
    !numberField(value.version, 1)
  )
    throw new AdminApiError("彩种响应格式无效。", 502, "INVALID_RESPONSE");
  return value as unknown as Game;
}

export function parseScheduleSpec(value: unknown): ScheduleSpec {
  if (!isRecord(value))
    throw new AdminApiError("排期响应格式无效。", 502, "INVALID_RESPONSE");
  try {
    return validateScheduleSpec(value as unknown as ScheduleSpec);
  } catch (cause) {
    throw new AdminApiError(
      cause instanceof Error ? cause.message : "排期响应格式无效。",
      502,
      "INVALID_RESPONSE",
    );
  }
}

export function parseScheduleRecord(value: unknown): ScheduleRecord {
  if (
    !isRecord(value) ||
    !stringField(value.id, "id") ||
    !stringField(value.brand_id, "brand_id") ||
    !stringField(value.game_id, "game_id") ||
    !numberField(value.revision, 1) ||
    !numberField(value.game_version, 1) ||
    !stringField(value.created_at, "created_at") ||
    !Number.isFinite(Date.parse(value.created_at))
  )
    throw new AdminApiError("排期记录响应格式无效。", 502, "INVALID_RESPONSE");
  return {
    id: value.id,
    brand_id: value.brand_id,
    game_id: value.game_id,
    revision: value.revision,
    spec: parseScheduleSpec(value.spec),
    game_version: value.game_version,
    created_at: value.created_at,
  };
}

export function parsePeriod(value: unknown): Period {
  if (
    !isRecord(value) ||
    !stringField(value.id, "id") ||
    !stringField(value.brand_id, "brand_id") ||
    !stringField(value.game_id, "game_id") ||
    !stringField(value.period_no, "period_no") ||
    !numberField(value.sequence, 0) ||
    !stringField(value.bet_start_at, "bet_start_at") ||
    !stringField(value.bet_end_at, "bet_end_at") ||
    !stringField(value.draw_at, "draw_at") ||
    !stringField(value.status, "status") ||
    !numberField(value.version, 1) ||
    (value.schedule_id !== undefined && !stringField(value.schedule_id, "schedule_id")) ||
    (value.state_reason !== undefined && typeof value.state_reason !== "string")
  )
    throw new AdminApiError("期数响应格式无效。", 502, "INVALID_RESPONSE");
  for (const key of ["bet_start_at", "bet_end_at", "draw_at"] as const)
    if (!Number.isFinite(Date.parse(value[key] as string)))
      throw new AdminApiError("期数时间响应格式无效。", 502, "INVALID_RESPONSE");
  return value as unknown as Period;
}

function stableJson(value: unknown): string {
  function sorted(input: unknown): unknown {
    if (Array.isArray(input)) return input.map(sorted);
    if (input && typeof input === "object")
      return Object.fromEntries(
        Object.entries(input)
          .sort(([a], [b]) => a.localeCompare(b))
          .map(([key, item]) => [key, sorted(item)]),
      );
    return input;
  }
  return JSON.stringify(sorted(value));
}

type FetchLike = typeof fetch;
interface Envelope<T> {
  success?: boolean;
  data?: T;
  error?: string | { code?: string; message?: string } | null;
}

export function createPeriodSchedulesApi(fetcher: FetchLike = fetch) {
  async function request<T>(
    brandId: string,
    path: string,
    options: { method?: "GET" | "PUT" | "POST"; body?: unknown; key?: string } = {},
    parse: (value: unknown) => T,
  ): Promise<T> {
    if (!brandId.trim()) throw new AdminApiError("请先选择品牌。", 0);
    const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brandId });
    if (options.body !== undefined) {
      headers.set("Content-Type", "application/json");
      headers.set("Idempotency-Key", options.key ?? createIdempotencyKey());
    }
    const response = await fetcher(`${BASE}${path}`, {
      method: options.method ?? "GET",
      credentials: "same-origin",
      headers,
      ...(options.body === undefined ? {} : { body: JSON.stringify(options.body) }),
    });
    if (response.status === 404 && options.method === undefined)
      throw new AdminApiError("资源不存在。", 404);
    let envelope: Envelope<unknown> | null;
    try {
      envelope = await response.json();
    } catch {
      throw new AdminApiError("服务器未返回有效 JSON。", response.status);
    }
    if (
      !response.ok ||
      !isRecord(envelope) ||
      envelope.success !== true ||
      !("data" in envelope)
    ) {
      const detail = isRecord(envelope?.error) ? envelope.error : undefined;
      throw new AdminApiError(
        typeof envelope?.error === "string"
          ? envelope.error
          : (typeof detail?.message === "string"
              ? detail.message
              : `请求失败（${response.status}）`),
        response.status,
        typeof detail?.code === "string" ? detail.code : undefined,
      );
    }
    return parse(envelope.data);
  }
  const id = encodeURIComponent;
  const parseList = <T>(name: string, parse: (value: unknown) => T) => (value: unknown): T[] => {
    if (!isRecord(value) || !Array.isArray(value[name]))
      throw new AdminApiError("列表响应格式无效。", 502, "INVALID_RESPONSE");
    return (value[name] as unknown[]).map(parse);
  };
  return {
    getGames(brandId: string, limit = 25, offset = 0) {
      if (!canonicalInteger(limit, 1) || limit > 100 || !canonicalInteger(offset))
        fail("彩种分页参数无效。");
      return request(brandId, `/games?limit=${limit}&offset=${offset}`, {}, (value) => ({
        games: (() => {
          const games = parseList("games", parseGame)(value);
          if (games.length > limit)
            throw new AdminApiError("彩种分页响应超过请求上限。", 502, "INVALID_RESPONSE");
          return games;
        })(),
      }));
    },
    async getSchedule(brandId: string, gameId: string): Promise<ScheduleRecord | null> {
      try {
        return await request(brandId, `/games/${id(gameId)}/schedule`, {}, parseScheduleRecord);
      } catch (cause) {
        if (cause instanceof AdminApiError && cause.status === 404) return null;
        throw cause;
      }
    },
    updateSchedule(
      brandId: string,
      gameId: string,
      body: UpdateScheduleBody,
      key: string,
    ) {
      const valid = buildUpdateScheduleBody(body.version, body.spec, body.reason);
      return request(brandId, `/games/${id(gameId)}/schedule`, {
        method: "PUT", body: valid, key,
      }, parseScheduleRecord);
    },
    generatePeriods(
      brandId: string,
      gameId: string,
      body: GeneratePeriodsBody,
      timezone: string,
      key: string,
    ) {
      const valid = validateUtcGenerationBody(body);
      // Validate the selected zone even though the body is already normalized to UTC.
      if (!isIanaTimezone(timezone))
        fail("彩种时区无效，请重新读取彩种。");
      return request(brandId, `/games/${id(gameId)}/periods/generate`, {
        method: "POST", body: valid, key,
      }, (value) => {
        if (
          !isRecord(value) ||
          !numberField(value.created) ||
          !numberField(value.existing) ||
          !Array.isArray(value.periods) ||
          value.periods.length > 100
        )
          throw new AdminApiError("生成结果响应格式无效。", 502, "INVALID_RESPONSE");
        return { created: value.created, existing: value.existing, periods: value.periods.map(parsePeriod) };
      });
    },
    getPeriods(brandId: string, gameId: string, limit = 25, offset = 0) {
      if (!canonicalInteger(limit, 1) || limit > 100 || !canonicalInteger(offset))
        fail("期数分页参数无效。");
      return request(
        brandId,
        `/games/${id(gameId)}/periods?limit=${limit}&offset=${offset}`,
        {},
        (value) => {
          if (!isRecord(value) || !Array.isArray(value.periods) || value.periods.length > limit || value.limit !== limit || value.offset !== offset)
            throw new AdminApiError("期数分页响应格式无效。", 502, "INVALID_RESPONSE");
          return { periods: value.periods.map(parsePeriod), limit, offset };
        },
      );
    },
  };
}
