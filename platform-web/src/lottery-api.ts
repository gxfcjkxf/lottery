import type { RuleModel } from "../../shared/src/rules";
import { PlatformApiError } from "./platform-api";

const BASE = "/api/v1/platform";

export interface Game {
  id: string;
  brand_id: string;
  code: string;
  name: string;
  model: RuleModel;
  timezone: string;
  status: string;
  version: number;
  started_sequence: number;
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
  state_reason: string;
}

export interface DrawResult {
  id: string;
  brand_id: string;
  game_id: string;
  period_id: string;
  source_id: string;
  kind: string;
  result: { regular: number[]; special: number[]; digits: number[] };
  result_hash: string;
  drawn_at: string;
  created_at: string;
  created_by?: string;
  corrected_from_id?: string;
}

export interface DrawAttempt {
  source_id: string;
  status: string;
  code: string;
}

export interface AttemptBatch {
  id: string;
  brand_id: string;
  game_id: string;
  period_id: string;
  source_set_id: string;
  observed_period_version: number;
  status: string;
  attempts: DrawAttempt[];
  created_at: string;
}

export interface DrawHistoryPage {
  current: DrawResult | null;
  history: DrawResult[];
  attempts: AttemptBatch[];
  limit: number;
  offset: number;
}

type Envelope = { success?: unknown; data?: unknown; error?: unknown };
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const record = (v: unknown): v is Record<string, unknown> => !!v && typeof v === "object" && !Array.isArray(v);
const string = (v: unknown): v is string => typeof v === "string" && v.length > 0;
const integer = (v: unknown, min = 0): v is number => Number.isSafeInteger(v) && Number(v) >= min;
function invalid(message: string): never { throw new PlatformApiError(message, 0, "INVALID_RESPONSE"); }
function input(message: string, code = "REQUEST_INVALID"): never { throw new PlatformApiError(message, 400, code); }
function iso(value: unknown): value is string {
  return typeof value === "string" && /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(value) && Number.isFinite(Date.parse(value));
}
function scoped(value: unknown, brandId: string, gameId?: string): value is Record<string, unknown> {
  return record(value) && value.brand_id === brandId && (gameId === undefined || value.game_id === gameId);
}
function parseGame(value: unknown, brandId: string): Game {
  if (!scoped(value, brandId) || !UUID.test(String(value.id)) || !string(value.code) || !string(value.name) || !string(value.timezone) || !string(value.status) || !integer(value.version, 1) || !integer(value.started_sequence) || !record(value.model)) invalid("Invalid game response");
  const model = value.model;
  if (!string(model.model) || !record(model.regular_pool) || !record(model.special_pool) || !integer(model.regular_count) || !integer(model.special_count) || !integer(model.pool_size) || !integer(model.total_count) || !integer(model.length) || typeof model.allow_repeat !== "boolean" || typeof model.ordered !== "boolean") invalid("Invalid game model response");
  for (const pool of [model.regular_pool, model.special_pool]) {
    if ((pool.min !== undefined && !integer(pool.min)) || (pool.max !== undefined && !integer(pool.max)) || (pool.min !== undefined && pool.max !== undefined && Number(pool.max) < Number(pool.min)) || typeof pool.allow_repeat !== "boolean" || (pool.values !== undefined && pool.values !== null && (!Array.isArray(pool.values) || pool.values.some(n => !integer(n))))) invalid("Invalid game number pool response");
  }
  return value as unknown as Game;
}
function parsePeriod(value: unknown, brandId: string, gameId: string): Period {
  if (!scoped(value, brandId, gameId) || !UUID.test(String(value.id)) || !string(value.period_no) || !integer(value.sequence) || !iso(value.bet_start_at) || !iso(value.bet_end_at) || !iso(value.draw_at) || !string(value.status) || !integer(value.version, 1) || (value.schedule_id !== undefined && !UUID.test(String(value.schedule_id))) || typeof value.state_reason !== "string") invalid("Invalid period response");
  return value as unknown as Period;
}
function parseDraw(value: unknown, brandId: string, gameId: string | undefined, periodId: string): DrawResult {
  if (!scoped(value, brandId, gameId) || value.period_id !== periodId || !UUID.test(String(value.id)) || !UUID.test(String(value.game_id)) || !UUID.test(String(value.source_id)) || !["api", "dom", "manual"].includes(String(value.kind)) || !string(value.result_hash) || !iso(value.drawn_at) || !iso(value.created_at) || (value.created_by !== undefined && !UUID.test(String(value.created_by))) || (value.corrected_from_id !== undefined && !UUID.test(String(value.corrected_from_id))) || !record(value.result) || !Array.isArray(value.result.regular) || !Array.isArray(value.result.special) || !Array.isArray(value.result.digits) || [...value.result.regular, ...value.result.special, ...value.result.digits].some(n => !integer(n))) invalid("Invalid draw response");
  return value as unknown as DrawResult;
}
function parseAttempt(value: unknown, brandId: string, gameId: string, periodId: string): AttemptBatch {
  if (!scoped(value, brandId, gameId) || value.period_id !== periodId || !UUID.test(String(value.id)) || !UUID.test(String(value.source_set_id)) || !integer(value.observed_period_version, 1) || !["no_data", "accepted", "discarded", "failed"].includes(String(value.status)) || !Array.isArray(value.attempts) || !value.attempts.every(item => record(item) && UUID.test(String(item.source_id)) && string(item.status) && string(item.code)) || !iso(value.created_at)) invalid("Invalid draw attempt response");
  return value as unknown as AttemptBatch;
}

export function createPlatformLotteryApi(fetcher: typeof fetch = fetch) {
  async function request(brandId: string, path: string): Promise<unknown> {
    if (!brandId.trim()) input("A brand must be selected before loading lottery data", "BRAND_REQUIRED");
    let response: Response;
    try {
      response = await fetcher(`${BASE}${path}`, { method: "GET", credentials: "same-origin", headers: { Accept: "application/json", "X-Brand-ID": brandId } });
    } catch (cause) {
      throw new PlatformApiError(cause instanceof Error ? cause.message : "Network request failed", 0, "NETWORK_ERROR", true);
    }
    let envelope: Envelope;
    try { envelope = await response.json() as Envelope; }
    catch { throw new PlatformApiError(response.ok ? "Invalid server response" : `Request failed (${response.status})`, response.status, "INVALID_RESPONSE"); }
    if (!response.ok || !record(envelope) || envelope.success !== true || !("data" in envelope)) {
      const detail = record(envelope?.error) ? envelope.error : undefined;
      throw new PlatformApiError(typeof detail?.message === "string" ? detail.message : typeof envelope?.error === "string" ? envelope.error : `Request failed (${response.status})`, response.status, typeof detail?.code === "string" ? detail.code : undefined);
    }
    return envelope.data;
  }
  function page(limit: number, offset: number, max = 100) {
    if (!integer(limit, 1) || limit > max || !integer(offset)) input("Invalid lottery pagination");
  }
  return {
    async games(brandId: string, limit = 51, offset = 0): Promise<Game[]> {
      page(limit, offset);
      const value = await request(brandId, `/games?limit=${limit}&offset=${offset}`);
      if (!record(value) || !Array.isArray(value.games) || value.games.length > limit || value.limit !== limit || value.offset !== offset) invalid("Invalid games page");
      return value.games.map(item => parseGame(item, brandId));
    },
    async periods(brandId: string, gameId: string, limit = 51, offset = 0): Promise<Period[]> {
      if (!UUID.test(gameId)) input("A valid game ID is required");
      page(limit, offset);
      const value = await request(brandId, `/games/${encodeURIComponent(gameId)}/periods?limit=${limit}&offset=${offset}`);
      if (!record(value) || !Array.isArray(value.periods) || value.periods.length > limit || value.limit !== limit || value.offset !== offset) invalid("Invalid periods page");
      return value.periods.map(item => parsePeriod(item, brandId, gameId));
    },
    async draw(brandId: string, periodId: string, limit = 50, offset = 0): Promise<DrawHistoryPage> {
      if (!UUID.test(periodId)) input("A valid period ID is required");
      page(limit, offset);
      const value = await request(brandId, `/periods/${encodeURIComponent(periodId)}/draw?limit=${limit}&offset=${offset}`);
      if (!record(value) || !(value.current === null || record(value.current)) || !Array.isArray(value.history) || !Array.isArray(value.attempts) || value.history.length > limit || value.attempts.length > limit || value.limit !== limit || value.offset !== offset) invalid("Invalid draw history page");
      const candidates = [...(value.current === null ? [] : [value.current]), ...value.history, ...value.attempts];
      const gameIds = candidates.map(item => scoped(item, brandId) && string(item.game_id) ? item.game_id : "");
      if (gameIds.some(id => !UUID.test(id)) || new Set(gameIds).size > 1) invalid("Draw history contains inconsistent game scopes");
      const gameId = gameIds[0];
      return {
        current: value.current === null ? null : parseDraw(value.current, brandId, gameId, periodId),
        history: value.history.map(item => parseDraw(item, brandId, gameId, periodId)),
        attempts: value.attempts.map(item => parseAttempt(item, brandId, gameId!, periodId)),
        limit, offset,
      };
    },
  };
}
