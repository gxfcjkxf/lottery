import { AdminApiError, type AdminAccount } from "./admin-api";

const BASE = "/api/v1/admin";
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const MAX_PAGE_SIZE = 100;
const MAX_OFFSET = 1_000_000;

export type CorrectionMode = "automatic" | "manual" | null;
export type CorrectionState = "reversing" | "resettling" | "failed" | "completed";
export type CorrectionTargetState = "pending" | "reversed" | "unchanged" | "excluded" | "failed";
export type DrawArrays = { regular: number[]; special: number[]; digits: number[] };
export interface CorrectionContext {
  brand_id: string; game_id: string; period_id: string; period_version: number;
  period_status: string; draw_result_id: string | null; draw: DrawArrays | null;
  model: { model: string; ordered: boolean; [key: string]: unknown };
  current_job_id: string | null; current_job_version: number | null;
  policy_version: number; mode: CorrectionMode; requires_resettlement: boolean; can_correct: boolean;
}
export interface Correction {
  id: string; brand_id: string; game_id: string; period_id: string;
  previous_draw_result_id: string; draw_result_id: string; result: DrawArrays;
  period_version: number; previous_job_id: string | null; new_job_id: string | null;
  policy_version: number | null; mode: CorrectionMode; state: CorrectionState;
  version: number; target_count: number; created_by: string; reason: string;
  created_at: string; completed_at: string | null; last_error_code: string | null;
  pending_count: number; reversed_count: number; unchanged_count: number;
  excluded_count: number; failed_count: number; reverse_points: string; reversed_points: string;
  can_retry: boolean; new_job_state: string | null; new_job_version: number | null;
  new_job_error_code: string | null;
}
export interface CorrectionTarget {
  order_id: string; member_id: string; state: CorrectionTargetState; version: number;
  old_order_version: number; old_order_status: string; old_calculation_id: string | null;
  old_payout_entry_id: string | null; old_prize_points: string;
  reversal_entry_id: string | null; reset_order_version: number | null; error_code: string | null;
}
export interface CorrectionBody { version: number; policy_version: number | null; result: DrawArrays; reason: string }
export interface CorrectionActionBody { version: number; reason: string }
export interface CorrectionExpected { periodId: string; accountId: string; mode: CorrectionMode }
export interface CorrectionPage<T> {
  brand_id: string; period_id?: string; correction_id?: string; items: T[];
  limit: number; offset: number; has_more: boolean;
}
export interface CorrectionPermissions { view: boolean; correct: boolean; retry: boolean; settleRun: boolean }

export function correctionPermissions(account: AdminAccount, brandId: string): CorrectionPermissions {
  const member = Boolean(brandId) && account.brand_ids.some(id => sameUuid(id, brandId));
  const brand = new Set(account.permissions_by_brand === undefined ? account.permissions ?? [] : account.permissions_by_brand[brandId] ?? []);
  const platform = new Set(account.platform_permissions ?? []);
  return {
    view: member && (brand.has("draw.view.brand") || platform.has("draw.view.platform")),
    correct: member && !account.super_admin && brand.has("draw.correct.brand"),
    retry: member && !account.super_admin && brand.has("draw.correction_retry.brand"),
    settleRun: member && !account.super_admin && brand.has("settlement.run.brand"),
  };
}

function isRecord(v: unknown): v is Record<string, unknown> { return Boolean(v && typeof v === "object" && !Array.isArray(v)); }
function nonempty(v: unknown): v is string { return typeof v === "string" && v.trim().length > 0; }
function validUuid(v: unknown): v is string { return typeof v === "string" && UUID_RE.test(v); }
function sameUuid(a: unknown, b: string): boolean { return typeof a === "string" && a.toLowerCase() === b.toLowerCase(); }
function positive(v: unknown): v is number { return Number.isSafeInteger(v) && Number(v) > 0; }
function count(v: unknown): v is number { return Number.isSafeInteger(v) && Number(v) >= 0; }
function validMode(v: unknown): v is CorrectionMode { return v === null || v === "automatic" || v === "manual"; }
function validDate(v: unknown): v is string {
  if (typeof v !== "string") return false;
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d{1,9})?(Z|[+-](\d{2}):(\d{2}))$/.exec(v);
  if (!match) return false;
  const [, y, mo, d, h, mi, s, , oh, om] = match;
  const year = Number(y), month = Number(mo), day = Number(d);
  const days = month === 2 ? (year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28) : [4, 6, 9, 11].includes(month) ? 30 : 31;
  return year > 0 && month >= 1 && month <= 12 && day >= 1 && day <= days && Number(h) < 24 && Number(mi) < 60 && Number(s) < 60 &&
    (oh === undefined || (Number(oh) <= 14 && Number(om) < 60 && (Number(oh) !== 14 || Number(om) === 0))) && Number.isFinite(Date.parse(v));
}
function validAmount(v: unknown): v is string { return typeof v === "string" && /^(0|[1-9][0-9]*)$/.test(v); }
function validInt64(v: unknown): v is string {
  if (typeof v !== "string" || !/^(0|-?[1-9][0-9]*)$/.test(v)) return false;
  try { const n = BigInt(v); return n >= -9223372036854775808n && n <= 9223372036854775807n; } catch { return false; }
}
function invalidInput(message: string): never { throw new AdminApiError(message, 0, "INVALID_INPUT"); }
function invalidResponse(): never { throw new AdminApiError("开奖结果更正响应格式无效。", 502, "INVALID_RESPONSE"); }
function validArrays(v: unknown): v is DrawArrays {
  return isRecord(v) && ["regular", "special", "digits"].every(k => Array.isArray(v[k]) && (v[k] as unknown[]).every(n => Number.isSafeInteger(n) && Number(n) >= 0));
}
function validContext(v: unknown, brand: string, period: string): v is CorrectionContext {
  if (!isRecord(v) || !sameUuid(v.brand_id, brand) || !validUuid(v.game_id) || !sameUuid(v.period_id, period) ||
    !positive(v.period_version) || !nonempty(v.period_status) || !(v.draw_result_id === null || validUuid(v.draw_result_id)) ||
    !(v.draw === null || validArrays(v.draw)) || !isRecord(v.model) || !nonempty(v.model.model) || typeof v.model.ordered !== "boolean" ||
    !(v.current_job_id === null || validUuid(v.current_job_id)) || !(v.current_job_version === null || positive(v.current_job_version)) ||
    !positive(v.policy_version) || !validMode(v.mode) || typeof v.requires_resettlement !== "boolean" || typeof v.can_correct !== "boolean") return false;
  return v.requires_resettlement === (v.current_job_id !== null) && (v.current_job_id === null) === (v.current_job_version === null) &&
    (v.draw_result_id === null) === (v.draw === null) &&
    (!v.can_correct || (v.draw_result_id !== null && (v.current_job_id === null ? v.period_status === "drawn" : ["settling", "settled"].includes(v.period_status))));
}
function validCorrection(v: unknown): v is Correction {
  if (!isRecord(v)) return false;
  const counts = ["target_count", "pending_count", "reversed_count", "unchanged_count", "excluded_count", "failed_count"];
  if (!counts.every(k => count(v[k]))) return false;
  const total = BigInt(v.pending_count as number) + BigInt(v.reversed_count as number) + BigInt(v.unchanged_count as number) + BigInt(v.excluded_count as number) + BigInt(v.failed_count as number);
  return validUuid(v.id) && validUuid(v.brand_id) && validUuid(v.game_id) && validUuid(v.period_id) && validUuid(v.previous_draw_result_id) && validUuid(v.draw_result_id) && validArrays(v.result) && positive(v.period_version) &&
    (v.previous_job_id === null || validUuid(v.previous_job_id)) && (v.new_job_id === null || validUuid(v.new_job_id)) && (v.policy_version === null || positive(v.policy_version)) && validMode(v.mode) &&
    ["reversing", "resettling", "failed", "completed"].includes(String(v.state)) && positive(v.version) && total === BigInt(v.target_count as number) && validUuid(v.created_by) && nonempty(v.reason) && validDate(v.created_at) &&
    (v.completed_at === null || validDate(v.completed_at)) && (v.last_error_code === null || typeof v.last_error_code === "string") && validAmount(v.reverse_points) && validAmount(v.reversed_points) && typeof v.can_retry === "boolean" &&
    (v.new_job_state === null || nonempty(v.new_job_state)) && (v.new_job_version === null || positive(v.new_job_version)) && (v.new_job_error_code === null || typeof v.new_job_error_code === "string");
}
function validTarget(v: unknown): v is CorrectionTarget {
  return isRecord(v) && validUuid(v.order_id) && validUuid(v.member_id) && ["pending", "reversed", "unchanged", "excluded", "failed"].includes(String(v.state)) && positive(v.version) && positive(v.old_order_version) && nonempty(v.old_order_status) &&
    (v.old_calculation_id === null || validUuid(v.old_calculation_id)) && (v.old_payout_entry_id === null || validUuid(v.old_payout_entry_id)) && validInt64(v.old_prize_points) && (v.reversal_entry_id === null || validUuid(v.reversal_entry_id)) &&
    (v.reset_order_version === null || positive(v.reset_order_version)) && (v.error_code === null || typeof v.error_code === "string");
}
function validatePage<T>(v: unknown, brand: string, key: "period_id" | "correction_id", id: string, check: (x: unknown) => x is T): CorrectionPage<T> {
  if (!isRecord(v) || !sameUuid(v.brand_id, brand) || !sameUuid(v[key], id) || !Array.isArray(v.items) || !v.items.every(check) || !count(v.limit) || !count(v.offset) || typeof v.has_more !== "boolean") invalidResponse();
  return v as unknown as CorrectionPage<T>;
}
function stableJson(value: unknown): string {
  if (Array.isArray(value)) return "[" + value.map(stableJson).join(",") + "]";
  if (isRecord(value)) return "{" + Object.keys(value).sort().map(k => JSON.stringify(k) + ":" + stableJson(value[k])).join(",") + "}";
  return JSON.stringify(value);
}
export function createCorrectionApi(fetcher: typeof fetch = fetch) {
  async function request<T>(brandId: string, path: string, body?: unknown, key?: string, successStatus = 200): Promise<T> {
    if (!validUuid(brandId)) invalidInput("品牌标识无效。");
    const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brandId });
    if (body !== undefined) headers.set("Content-Type", "application/json");
    if (key) headers.set("Idempotency-Key", key);
    let response: Response;
    try { response = await fetcher(BASE + path, { method: body === undefined ? "GET" : "POST", credentials: "include", headers, ...(body === undefined ? {} : { body: JSON.stringify(body) }) }); }
    catch (cause) { throw new AdminApiError(cause instanceof Error ? cause.message : "Network request failed", 0, "NETWORK_ERROR"); }
    let envelope: unknown;
    try { envelope = await response.json(); } catch {
      if (response.ok) invalidResponse();
      throw new AdminApiError("Invalid server response", response.status);
    }
    if (!response.ok) {
      const error = isRecord(envelope) && isRecord(envelope.error) ? envelope.error : null;
      throw new AdminApiError(typeof error?.message === "string" ? error.message : "Request failed (" + response.status + ")", response.status, typeof error?.code === "string" ? error.code : undefined);
    }
    if (response.status !== successStatus || !isRecord(envelope) || envelope.success !== true || !("data" in envelope)) invalidResponse();
    return envelope.data as T;
  }
  function pageArgs(limit: number, offset: number) { if (!Number.isSafeInteger(limit) || limit < 1 || limit > MAX_PAGE_SIZE || !Number.isSafeInteger(offset) || offset < 0 || offset > MAX_OFFSET) invalidInput("分页参数超出允许范围。"); }
  const api = {
    async context(brandId: string, periodId: string): Promise<CorrectionContext> {
      if (!validUuid(periodId)) invalidInput("期次标识无效。");
      const value = await request<unknown>(brandId, "/periods/" + encodeURIComponent(periodId) + "/correction-context");
      if (!validContext(value, brandId, periodId)) invalidResponse(); return value;
    },
    /** Return the cached start receipt. The caller owns frozen intent and subsequent reads. */
    async create(brandId: string, drawResultId: string, body: CorrectionBody, key: string, expected?: CorrectionExpected): Promise<Correction> {
      if (!validUuid(drawResultId) || !positive(body?.version) || body.version === Number.MAX_SAFE_INTEGER ||
        !(body.policy_version === null || positive(body.policy_version)) || !validArrays(body.result) || !nonempty(body.reason) || !nonempty(key) ||
        (expected !== undefined && (!validUuid(expected.periodId) || !validUuid(expected.accountId) || !validMode(expected.mode) ||
          (body.policy_version === null ? expected.mode !== null : expected.mode === null)))) invalidInput("更正请求参数无效。");
      // Snapshot plain fields before awaiting; input may be a Vue proxy or later edited by its caller.
      const payload: CorrectionBody = { version: body.version, policy_version: body.policy_version,
        result: { regular: [...body.result.regular], special: [...body.result.special], digits: [...body.result.digits] }, reason: body.reason };
      const scope = expected === undefined ? undefined : { ...expected };
      const receipt = await request<unknown>(brandId, "/draw-results/" + encodeURIComponent(drawResultId) + "/correct", payload, key, 201);
      if (!validCorrection(receipt) || !sameUuid(receipt.brand_id, brandId) || !sameUuid(receipt.previous_draw_result_id, drawResultId) ||
        receipt.period_version !== payload.version + 1 || receipt.version !== 1 || receipt.reason !== payload.reason ||
        stableJson(receipt.result) !== stableJson(payload.result) || receipt.policy_version !== payload.policy_version ||
        (scope !== undefined && (!sameUuid(receipt.period_id, scope.periodId) || !sameUuid(receipt.created_by, scope.accountId) || receipt.mode !== scope.mode))) invalidResponse();
      if (payload.policy_version === null) {
        if (receipt.state !== "completed" || receipt.previous_job_id !== null || receipt.mode !== null || receipt.target_count !== 0 || receipt.completed_at === null) invalidResponse();
      } else if (receipt.state !== "reversing" || receipt.previous_job_id === null || receipt.mode === null || receipt.completed_at !== null) invalidResponse();
      if (receipt.new_job_id !== null || receipt.new_job_state !== null || receipt.new_job_version !== null || receipt.new_job_error_code !== null) invalidResponse();
      return receipt;
    },
    async history(brandId: string, periodId: string, limit = 20, offset = 0): Promise<CorrectionPage<Correction>> {
      if (!validUuid(periodId)) invalidInput("期次标识无效。"); pageArgs(limit, offset);
      const page = validatePage(await request<unknown>(brandId, "/periods/" + encodeURIComponent(periodId) + "/corrections?limit=" + limit + "&offset=" + offset), brandId, "period_id", periodId, validCorrection);
      if (page.limit !== limit || page.offset !== offset || page.items.length > limit || page.items.some(c => !sameUuid(c.brand_id, brandId) || !sameUuid(c.period_id, periodId))) invalidResponse(); return page;
    },
    async detail(brandId: string, correctionId: string): Promise<Correction> {
      if (!validUuid(correctionId)) invalidInput("更正标识无效。");
      const value = await request<unknown>(brandId, "/corrections/" + encodeURIComponent(correctionId));
      if (!validCorrection(value) || !sameUuid(value.brand_id, brandId) || !sameUuid(value.id, correctionId)) invalidResponse(); return value;
    },
    async retry(brandId: string, correctionId: string, body: CorrectionActionBody, key: string): Promise<Correction> {
      if (!validUuid(correctionId) || !positive(body?.version) || body.version === Number.MAX_SAFE_INTEGER || !nonempty(body.reason) || !nonempty(key)) invalidInput("重试请求参数无效。");
      const payload = { version: body.version, reason: body.reason };
      const receipt = await request<unknown>(brandId, "/corrections/" + encodeURIComponent(correctionId) + "/retry", payload, key);
      // Retry actor/reason live in audit records; these fields remain those of the original correction.
      if (!validCorrection(receipt) || !sameUuid(receipt.id, correctionId) || !sameUuid(receipt.brand_id, brandId) ||
        receipt.version !== payload.version + 1 || receipt.state !== "reversing" || receipt.previous_job_id === null || receipt.policy_version === null || receipt.mode === null ||
        receipt.new_job_id !== null || receipt.completed_at !== null) invalidResponse();
      return receipt;
    },
    async targets(brandId: string, correctionId: string, limit = 20, offset = 0): Promise<CorrectionPage<CorrectionTarget>> {
      if (!validUuid(correctionId)) invalidInput("更正标识无效。"); pageArgs(limit, offset);
      const page = validatePage(await request<unknown>(brandId, "/corrections/" + encodeURIComponent(correctionId) + "/targets?limit=" + limit + "&offset=" + offset), brandId, "correction_id", correctionId, validTarget);
      if (page.limit !== limit || page.offset !== offset || page.items.length > limit) invalidResponse(); return page;
    },
  };
  return api;
}
