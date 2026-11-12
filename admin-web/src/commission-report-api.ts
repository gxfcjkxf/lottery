import { isPointTotal } from '@lottery/shared'
import { readReportDownload } from "@lottery/shared";
import { readAdminEnvelope } from './admin-api'
import { copyAssociationNames } from "@lottery/shared"
import { AdminApiError, readAdminErrorResponse, type AdminAccount } from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";

const BASE = "/api/v1/admin/reports/commission";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const UNSIGNED = /^(0|[1-9]\d*)$/;
const MAX_WINDOW_NS = 93n * 24n * 60n * 60n * 1_000_000_000n;
const FIELDS = ["entry_count", "paid_entry_count", "paid_points", "adjustment_entry_count", "adjustment_credit_points", "adjustment_debit_points", "correction_entry_count", "correction_credit_points", "correction_debit_points", "net_points"] as const;
const GROUPS = ["day", "agent", "cycle"] as const;
export type CommissionGroup = typeof GROUPS[number];
export interface CommissionReportQuery { from: string; to: string; group_by: CommissionGroup; limit?: number; offset?: number; agent_id?: string | null; member_id?: string | null; cycle_id?: string | null }
export interface CommissionTotals { entry_count: string; paid_entry_count: string; paid_points: string; adjustment_entry_count: string; adjustment_credit_points: string; adjustment_debit_points: string; correction_entry_count: string; correction_credit_points: string; correction_debit_points: string; net_points: string }
export interface CommissionReport {
  brand_id: string; snapshot_at: string; timezone: string;
  query: { from: string; to: string; group_by: CommissionGroup; limit: number; offset: number; agent_id: string | null; member_id: string | null; cycle_id: string | null };
  summary: CommissionTotals; items: { key: string; label: string; totals: CommissionTotals }[]; total_groups: string;
}
type FetchLike = typeof fetch;
type CanonicalQuery = CommissionReport["query"];
const isRecord = (v: unknown): v is Record<string, unknown> => !!v && typeof v === "object" && !Array.isArray(v);
const exactKeys = (v: Record<string, unknown>, keys: readonly string[]) => Object.keys(v).length === keys.length && Object.keys(v).every((k) => keys.includes(k));
function fail(message = "佣金报表响应格式无效。", status = 502, code = "INVALID_RESPONSE"): never { throw new AdminApiError(message, status, code); }
function bad(message = "佣金报表筛选无效。"): never { return fail(message, 0, "INVALID_INPUT"); }

function epochNs(value: string): bigint {
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|([+-])(\d{2}):(\d{2}))$/.exec(value);
  if (!m) return bad("from 和 to 必须是有效 RFC3339 日期时间。");
  const [, ys, mos, ds, hs, mis, ss, fraction = "", zone, , zh = "0", zm = "0"] = m;
  const y = Number(ys), mo = Number(mos), d = Number(ds), h = Number(hs), mi = Number(mis), sec = Number(ss);
  const days = mo === 2 ? (y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0) ? 29 : 28) : ([4, 6, 9, 11].includes(mo) ? 30 : 31);
  if (y < 1 || mo < 1 || mo > 12 || d < 1 || d > days || h > 23 || mi > 59 || sec > 59 || (zone !== "Z" && (Number(zh) > 14 || Number(zm) > 59 || Number(zh) === 14 && Number(zm) !== 0))) return bad("from 和 to 必须是有效 RFC3339 日期时间。");
  const millis = Date.parse(value.replace(/\.\d+(?=Z|[+-]\d{2}:\d{2}$)/, ""));
  if (!Number.isFinite(millis)) return bad("日期超出支持范围。");
  return BigInt(millis) * 1_000_000n + BigInt(fraction.padEnd(9, "0") || "0");
}
function utc(ns: bigint): string {
  const seconds = ns >= 0n ? ns / 1_000_000_000n : (ns - 999_999_999n) / 1_000_000_000n;
  const nanos = ns - seconds * 1_000_000_000n;
  const base = new Date(Number(seconds * 1_000n)).toISOString().slice(0, 19);
  const fraction = nanos.toString().padStart(9, "0").replace(/0+$/, "");
  return `${base}${fraction ? `.${fraction}` : ""}Z`;
}
function canonical(q: CommissionReportQuery): CanonicalQuery {
  if (!q || Object.keys(q).some((k) => !["from", "to", "group_by", "limit", "offset", "agent_id", "member_id", "cycle_id"].includes(k))) return bad("佣金报表包含不支持的筛选参数。");
  const start = epochNs(q.from), end = epochNs(q.to);
  if (end <= start || end - start > MAX_WINDOW_NS) return bad("时间范围必须大于零且不超过 93 天。");
  if (!GROUPS.includes(q.group_by)) return bad("佣金报表分组无效。");
  for (const id of [q.agent_id, q.member_id, q.cycle_id]) if (id != null && !UUID.test(id)) return bad("筛选 UUID 格式无效。");
  const limit = q.limit ?? 20, offset = q.offset ?? 0;
  if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000) return bad("分页参数无效。");
  return { from: utc(start), to: utc(end), group_by: q.group_by, limit, offset, agent_id: q.agent_id?.toLowerCase() ?? null, member_id: q.member_id?.toLowerCase() ?? null, cycle_id: q.cycle_id?.toLowerCase() ?? null };
}
function totals(value: unknown): CommissionTotals {
  if (!isRecord(value) || !exactKeys(value, FIELDS) || FIELDS.some((field) => typeof value[field] !== "string" || !(field.endsWith("_points") ? isPointTotal(value[field],field === "net_points") : UNSIGNED.test(value[field] as string)))) return fail();
  const v = value as unknown as CommissionTotals;
  return v;
}
function validTimezone(v: string): boolean { try { return !!v && v.length <= 100 && new Intl.DateTimeFormat("en", { timeZone: v }).format(0) !== ""; } catch { return false; } }
function validSnapshot(v: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(v)) return false;
  try { return utc(epochNs(v)) === v; } catch { return false; }
}
function validDay(v: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(v)) return false;
  const [y, m, d] = v.split("-").map(Number); const date = new Date(0);
  date.setUTCHours(0, 0, 0, 0); date.setUTCFullYear(y!, m! - 1, d!);
  return date.getUTCFullYear() === y && date.getUTCMonth() === m! - 1 && date.getUTCDate() === d;
}

export function commissionReportPermissions(account: AdminAccount, brand: string) {
  const valid = UUID.test(account.id) && UUID.test(brand);
  const grants = brandPermissionSet(account, brand.toLowerCase());
  const view = valid && grants.has("report_commission.view.brand");
  const exportAllowed = view && grants.has("report_commission.export.brand");
  return { view, export: exportAllowed };
}

export function createCommissionReportApi(fetcher: FetchLike = fetch) {
  async function report(brand: string, input: CommissionReportQuery): Promise<CommissionReport> {
    if (!UUID.test(brand)) return bad("品牌 UUID 无效。");
    const brandId = brand.toLowerCase();
    const q = canonical(input), params = new URLSearchParams();
    for (const [key, value] of Object.entries(q)) if (value !== null) params.set(key, String(value));
    const response = await fetcher(`${BASE}?${params}`, { method: "GET", credentials: "same-origin", headers: { "X-Brand-ID": brandId, Accept: "application/json" } });
    const body = await readAdminEnvelope(response);
    if (!isRecord(body) || Object.keys(body).some(key => !["success", "data", "request_id", "associations", "association_error"].includes(key)) || body.success !== true || typeof body.request_id !== "string" || !/^[a-zA-Z0-9_.:-]{1,80}$/.test(body.request_id) || !isRecord(body.data)) return fail();
    const d = body.data;
    if (!exactKeys(d, ["brand_id", "snapshot_at", "timezone", "query", "summary", "items", "total_groups"]) || typeof d.brand_id !== "string" || d.brand_id.toLowerCase() !== brandId || typeof d.snapshot_at !== "string" || !validSnapshot(d.snapshot_at) || typeof d.timezone !== "string" || !validTimezone(d.timezone) || !isRecord(d.query) || !Array.isArray(d.items) || typeof d.total_groups !== "string" || !UNSIGNED.test(d.total_groups)) return fail();
    const echo = d.query;
    if (!exactKeys(echo, ["from", "to", "group_by", "limit", "offset", "agent_id", "member_id", "cycle_id"]) || echo.from !== q.from || echo.to !== q.to || echo.group_by !== q.group_by || echo.limit !== q.limit || echo.offset !== q.offset || echo.agent_id !== q.agent_id || echo.member_id !== q.member_id || echo.cycle_id !== q.cycle_id) return fail("佣金报表回显范围与请求不一致。");
    const summary = totals(d.summary);
    const items = d.items.map((raw: unknown) => {
      if (!isRecord(raw) || !exactKeys(raw, ["key", "label", "totals"]) || typeof raw.key !== "string" || !raw.key || raw.label !== raw.key || typeof raw.label !== "string" || /[\u0000-\u001f\u007f]/.test(raw.label)) return fail();
      if (q.group_by === "day" && !validDay(raw.key) || q.group_by === "agent" && !UUID.test(raw.key) || q.group_by === "cycle" && !UUID.test(raw.key)) return fail();
      return { key: raw.key, label: raw.label, totals: totals(raw.totals) };
    });
    if (items.length > q.limit || BigInt(d.total_groups) < BigInt(items.length) || items.length > 0 && BigInt(q.offset) + BigInt(items.length) > BigInt(d.total_groups)) return fail();
    for (let i = 1; i < items.length; i++) if (items[i - 1]!.key >= items[i]!.key) return fail();
    const checked = { brand_id: brandId, snapshot_at: d.snapshot_at as string, timezone: d.timezone, query: echo as CommissionReport["query"], summary, items, total_groups: d.total_groups };
    copyAssociationNames(d, checked);
    return checked;
  }
  async function exportCsv(brand: string, input: CommissionReportQuery) {
    if (!UUID.test(brand)) return bad("品牌 UUID 无效。");
    const brandId = brand.toLowerCase();
    if (input.limit !== undefined || input.offset !== undefined) return bad("CSV 导出不接受分页参数。");
    const q = canonical(input), params = new URLSearchParams();
    for (const key of ["from", "to", "group_by", "agent_id", "member_id", "cycle_id"] as const) { const value = q[key]; if (value !== null) params.set(key, String(value)); }
    const response = await fetcher(`${BASE}.csv?${params}`, { method: "GET", credentials: "same-origin", headers: { "X-Brand-ID": brandId, Accept: "text/csv" } });
    if (!response.ok) throw await readAdminErrorResponse(response);
    return readReportDownload(response, brandId, 'commission', () => fail());
  }
  return { report, exportCsv };
}
