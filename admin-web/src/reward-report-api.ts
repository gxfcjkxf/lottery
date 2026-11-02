import { readReportDownload } from "@lottery/shared";
import { readAdminEnvelope } from './admin-api'
import { readAdminErrorResponse } from './admin-api'
import { copyAssociationNames } from "@lottery/shared"
import { AdminApiError, type AdminAccount } from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";

// Match the backend's public UUID shape contract; legacy IDs need not carry RFC version/variant bits.
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const UINT = /^(0|[1-9]\d*)$/;
const SINT = /^(0|-?[1-9]\d*)$/;
const MAX_WINDOW_NS = 93n * 86_400n * 1_000_000_000n;
const MIN_UTC_NS = BigInt(Date.parse("0001-01-01T00:00:00.000Z")) * 1_000_000n;
const MAX_UTC_NS = BigInt(Date.parse("+010000-01-01T00:00:00.000Z")) * 1_000_000n;
const STATES = ["granted", "revoked"] as const;
const POSTING_FIELDS = ["entry_count", "grant_entry_count", "grant_points", "reversal_entry_count", "reversal_points", "net_points"] as const;
const ORDER_FIELDS = ["order_count", "original_points", "granted_count", "granted_points", "revoked_count", "revoked_points"] as const;
const BASE = "/api/v1/admin/reports/rewards";

export type RewardReportKind = "rewards" | "reward_orders";
type RewardPostingGroup = "day" | "member" | "order";
type RewardOrderGroup = "day" | "member" | "state";
export interface RewardReportQuery { from: string; to: string; group_by: RewardPostingGroup | RewardOrderGroup; limit?: number; offset?: number; member_id?: string | null; order_id?: string | null }
interface RewardPostingTotals { entry_count: string; grant_entry_count: string; grant_points: string; reversal_entry_count: string; reversal_points: string; net_points: string }
interface RewardOrderTotals { order_count: string; original_points: string; granted_count: string; granted_points: string; revoked_count: string; revoked_points: string }
type RewardReportTotals = RewardPostingTotals | RewardOrderTotals;
export interface RewardReport { brand_id: string; snapshot_at: string; timezone: string; query: { from: string; to: string; group_by: RewardReportQuery["group_by"]; limit: number; offset: number; member_id: string | null; order_id: string | null }; summary: RewardReportTotals; items: { key: string; label: string; totals: RewardReportTotals }[]; total_groups: string }
type CanonicalQuery = Omit<RewardReport["query"], "limit" | "offset">;
type FetchLike = typeof fetch;
const record = (value: unknown): value is Record<string, unknown> => !!value && typeof value === "object" && !Array.isArray(value);
const exact = (value: Record<string, unknown>, keys: readonly string[]) => Object.keys(value).length === keys.length && Object.keys(value).every((key) => keys.includes(key));
function invalid(message = "奖励报表响应格式无效。", status = 502, code = "INVALID_RESPONSE"): never { throw new AdminApiError(message, status, code); }
function input(message = "奖励报表筛选无效。"): never { return invalid(message, 0, "INVALID_INPUT"); }
function isKind(value: unknown): value is RewardReportKind { return value === "rewards" || value === "reward_orders"; }

function instant(value: string): bigint {
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|([+-])(\d{2}):(\d{2}))$/.exec(value);
  if (!m) return input("from 和 to 必须是有效 RFC3339 日期时间。");
  const [, yy, mm, dd, hh, mi, ss, fraction = "", zone, , oh = "0", om = "0"] = m;
  const y = Number(yy), month = Number(mm), day = Number(dd), hour = Number(hh), minute = Number(mi), second = Number(ss);
  const days = month === 2 ? (y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0) ? 29 : 28) : ([4, 6, 9, 11].includes(month) ? 30 : 31);
  if (y < 1 || month < 1 || month > 12 || day < 1 || day > days || hour > 23 || minute > 59 || second > 59 || (zone !== "Z" && (Number(oh) > 14 || Number(om) > 59 || Number(oh) === 14 && Number(om) !== 0))) return input("日期时间无效。");
  const whole = value.replace(/\.\d+(?=Z|[+-]\d{2}:\d{2}$)/, "");
  const millis = Date.parse(whole);
  if (!Number.isFinite(millis)) return input("日期超出支持范围。");
  const result = BigInt(millis) * 1_000_000n + BigInt(fraction.padEnd(9, "0") || "0");
  if (result < MIN_UTC_NS || result >= MAX_UTC_NS) return input("日期超出支持范围。");
  return result;
}
function normalizeUtc(ns: bigint): string {
  if (ns < MIN_UTC_NS || ns >= MAX_UTC_NS) return input("日期超出支持范围。");
  const sec = ns >= 0n ? ns / 1_000_000_000n : (ns - 999_999_999n) / 1_000_000_000n;
  const nanos = ns - sec * 1_000_000_000n;
  const base = new Date(Number(sec * 1_000n)).toISOString().slice(0, 19);
  const fraction = nanos.toString().padStart(9, "0").replace(/0+$/, "");
  return `${base}${fraction ? `.${fraction}` : ""}Z`;
}
function validTimestamp(value: unknown): value is string {
  if (typeof value !== "string" || !/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?(?:Z|[+-]\d\d:\d\d)$/.test(value)) return false;
  try { instant(value); return true; } catch { return false; }
}
function canonical(inputQuery: RewardReportQuery, kind: RewardReportKind, exportMode = false): RewardReport["query"] {
  if (!record(inputQuery) || Object.keys(inputQuery).some((k) => !["from", "to", "group_by", "limit", "offset", "member_id", "order_id"].includes(k))) return input("包含不支持的筛选参数。");
  if (typeof inputQuery.from !== "string" || typeof inputQuery.to !== "string" || typeof inputQuery.group_by !== "string") return input("from、to 和 group_by 必须是字符串。");
  const fromNs = instant(inputQuery.from), toNs = instant(inputQuery.to);
  if (toNs <= fromNs || toNs - fromNs > MAX_WINDOW_NS) return input("时间范围必须大于零且不超过 93 天。");
  const groups = kind === "rewards" ? ["day", "member", "order"] : ["day", "member", "state"];
  if (!groups.includes(inputQuery.group_by)) return input("分组无效。");
  for (const id of [inputQuery.member_id, inputQuery.order_id]) if (id != null && (typeof id !== "string" || !UUID.test(id))) return input("筛选 UUID 格式无效。");
  const limit = inputQuery.limit ?? 20, offset = inputQuery.offset ?? 0;
  if (exportMode && (inputQuery.limit !== undefined || inputQuery.offset !== undefined)) return input("CSV 导出不接受分页参数。");
  if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000) return input("分页参数无效。");
  return { from: normalizeUtc(fromNs), to: normalizeUtc(toNs), group_by: inputQuery.group_by, limit, offset, member_id: inputQuery.member_id?.toLowerCase() ?? null, order_id: inputQuery.order_id?.toLowerCase() ?? null };
}
function fields(kind: RewardReportKind): readonly string[] { return kind === "rewards" ? POSTING_FIELDS : ORDER_FIELDS; }
function totals(value: unknown, kind: RewardReportKind): RewardReportTotals {
  const list = fields(kind);
  if (!record(value) || !exact(value, list) || list.some((f) => typeof value[f] !== "string" || !(f === "net_points" ? SINT : UINT).test(value[f] as string))) return invalid();
  const t = value as unknown as RewardReportTotals;
  return t;
}
function timezone(value: unknown): value is string { try { return typeof value === "string" && value.length > 0 && value.length <= 100 && !!new Intl.DateTimeFormat("en", { timeZone: value }).format(0); } catch { return false; } }
function day(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const [y, m, d] = value.split("-").map(Number), date = new Date(0); date.setUTCHours(0, 0, 0, 0); date.setUTCFullYear(y!, m! - 1, d!);
  return date.getUTCFullYear() === y && date.getUTCMonth() === m! - 1 && date.getUTCDate() === d;
}
function localDay(ns: bigint, zone: string): string {
  const seconds = ns >= 0n ? ns / 1_000_000_000n : (ns - 999_999_999n) / 1_000_000_000n;
  const p = new Intl.DateTimeFormat("en-CA", { timeZone: zone, year: "numeric", month: "2-digit", day: "2-digit" }).formatToParts(new Date(Number(seconds * 1000n)));
  const get = (type: string) => p.find((part) => part.type === type)!.value;
  return `${get("year")}-${get("month")}-${get("day")}`;
}
function validateKey(key: string, group: string, kind: RewardReportKind, q: CanonicalQuery, zone: string): boolean {
  if (group === "day") {
    if (!day(key)) return false;
    const first = localDay(instant(q.from), zone), last = localDay(instant(q.to), zone);
    return key >= first && (key < last || key === last && localDay(instant(q.to) - 1n, zone) === last);
  }
  if (group === "member" || group === "order") return UUID.test(key);
  return kind === "reward_orders" && (STATES as readonly string[]).includes(key);
}
function validatePage(data: unknown, brand: string, kind: RewardReportKind, q: RewardReport["query"]): RewardReport {
  if (!record(data) || !exact(data, ["brand_id", "snapshot_at", "timezone", "query", "summary", "items", "total_groups"]) || typeof data.brand_id !== "string" || data.brand_id.toLowerCase() !== brand || !validTimestamp(data.snapshot_at) || !timezone(data.timezone) || !record(data.query) || !Array.isArray(data.items) || typeof data.total_groups !== "string" || !UINT.test(data.total_groups)) return invalid();
  const echo = data.query;
  if (!exact(echo, ["from", "to", "group_by", "limit", "offset", "member_id", "order_id"]) || echo.from !== q.from || echo.to !== q.to || echo.group_by !== q.group_by || echo.limit !== q.limit || echo.offset !== q.offset || echo.member_id !== q.member_id || echo.order_id !== q.order_id) return invalid("奖励报表回显范围与请求不一致。");
  const summary = totals(data.summary, kind), items = data.items.map((raw: unknown) => {
    if (!record(raw) || !exact(raw, ["key", "label", "totals"]) || typeof raw.key !== "string" || raw.label !== raw.key || typeof raw.label !== "string" || !validateKey(raw.key, q.group_by, kind, q, data.timezone as string) || q.group_by === "member" && q.member_id !== null && raw.key !== q.member_id || q.group_by === "order" && q.order_id !== null && raw.key !== q.order_id) return invalid();
    const itemTotals = totals(raw.totals, kind);
    return { key: raw.key, label: raw.label, totals: itemTotals };
  });
  const total = BigInt(data.total_groups), offset = BigInt(q.offset), remaining = total > offset ? total - offset : 0n;
  const expectedItems = Number(remaining < BigInt(q.limit) ? remaining : BigInt(q.limit));
  if (items.length !== expectedItems || new Set(items.map((v) => v.key)).size !== items.length) return invalid("奖励报表分页边界无效。");
  for (let i = 1; i < items.length; i++) if (items[i - 1]!.key >= items[i]!.key) return invalid("奖励报表分组排序无效。");
  return { brand_id: brand, snapshot_at: data.snapshot_at, timezone: data.timezone, query: echo as RewardReport["query"], summary, items, total_groups: data.total_groups };
}
export function rewardReportPermissions(account: AdminAccount, brand: string) {
  const valid = UUID.test(account.id) && UUID.test(brand);
  const grants = brandPermissionSet(account, brand.toLowerCase());
  const view = valid && grants.has("report_reward.view.brand");
  return { view, export: Boolean(view && grants.has("report_reward.export.brand")) };
}

export function createRewardReportApi(fetcher: FetchLike = fetch) {
  async function read(kind: RewardReportKind, brand: string, query: RewardReportQuery, signal?: AbortSignal): Promise<RewardReport> {
    if (!isKind(kind)) return input("奖励报表类型无效。");
    if (!UUID.test(brand)) return input("品牌 UUID 无效。");
    const brandId = brand.toLowerCase(), q = canonical(query, kind), params = new URLSearchParams();
    for (const [key, value] of Object.entries(q)) if (value !== null) params.set(key, String(value));
    const path = kind === "rewards" ? BASE : `${BASE.replace(/rewards$/, "reward-orders")}`;
    const response = await fetcher(`${path}?${params}`, { method: "GET", credentials: "same-origin", headers: { "X-Brand-ID": brandId, Accept: "application/json" }, ...(signal ? { signal } : {}) });
    const body = await readAdminEnvelope(response);
    if (!record(body) || Object.keys(body).some(key => !["success", "data", "request_id", "associations", "association_error"].includes(key)) || body.success !== true || typeof body.request_id !== "string" || !/^[A-Za-z0-9_.:-]{1,80}$/.test(body.request_id)) return invalid();
    const checked = validatePage(body.data, brandId, kind, q);
    copyAssociationNames(body.data, checked);
    return checked;
  }
  async function exportCsv(kind: RewardReportKind, brand: string, query: RewardReportQuery, signal?: AbortSignal) {
    if (!isKind(kind)) return input("奖励报表类型无效。");
    if (!UUID.test(brand)) return input("品牌 UUID 无效。");
    const brandId = brand.toLowerCase(), q = canonical(query, kind, true), params = new URLSearchParams();
    for (const k of ["from", "to", "group_by", "member_id", "order_id"] as const) { const v = q[k]; if (v !== null) params.set(k, String(v)); }
    const path = `${kind === "rewards" ? BASE : BASE.replace(/rewards$/, "reward-orders")}.csv`;
    const response = await fetcher(`${path}?${params}`, { method: "GET", credentials: "same-origin", headers: { "X-Brand-ID": brandId, Accept: "text/csv" }, ...(signal ? { signal } : {}) });
    if (!response.ok) throw await readAdminErrorResponse(response);
    return readReportDownload(response, brandId, kind, () => invalid());
  }
  return { read, export: exportCsv };
}
