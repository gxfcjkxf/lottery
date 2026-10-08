import { AdminApiError, type AdminAccount } from "./admin-api";

const BASE = "/api/v1/admin/reports/commission";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const UNSIGNED = /^(0|[1-9]\d*)$/;
const SIGNED = /^(0|-?[1-9]\d*)$/;
const MAX_WINDOW_NS = 93n * 24n * 60n * 60n * 1_000_000_000n;
const MAX_GROUPS = 10_000;
const MAX_BYTES = 4 * 1024 * 1024;
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
  if (!isRecord(value) || !exactKeys(value, FIELDS) || FIELDS.some((field) => typeof value[field] !== "string" || !(field === "net_points" ? SIGNED : UNSIGNED).test(value[field] as string))) return fail();
  const v = value as unknown as CommissionTotals;
  if (BigInt(v.entry_count) !== BigInt(v.paid_entry_count) + BigInt(v.adjustment_entry_count) + BigInt(v.correction_entry_count) || BigInt(v.net_points) !== BigInt(v.paid_points) + BigInt(v.adjustment_credit_points) - BigInt(v.adjustment_debit_points) + BigInt(v.correction_credit_points) - BigInt(v.correction_debit_points)) return fail("佣金分项与总计不一致。");
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

function parseCsv(text: string): string[][] {
  const rows: string[][] = []; let row: string[] = [], field = "", quoted = false, afterQuote = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i]!;
    if (quoted) { if (c === '"') { if (text[i + 1] === '"') { field += '"'; i++; } else { quoted = false; afterQuote = true; } } else field += c; continue; }
    if (afterQuote && c !== "," && c !== "\n" && c !== "\r") return fail("佣金 CSV 格式无效。");
    if (c === '"') { if (field || afterQuote) return fail("佣金 CSV 格式无效。"); quoted = true; }
    else if (c === ",") { row.push(field); field = ""; afterQuote = false; }
    else if (c === "\n") { row.push(field); rows.push(row); row = []; field = ""; afterQuote = false; }
    else if (c === "\r") { if (text[i + 1] !== "\n") return fail("佣金 CSV 格式无效。"); }
    else { if (c.charCodeAt(0) < 0x20) return fail("佣金 CSV 格式无效。"); field += c; }
  }
  if (quoted || !text.endsWith("\n") || row.length || field || afterQuote) return fail("佣金 CSV 格式无效。");
  return rows;
}
function filenameDate(snapshot: string): string {
  const d = new Date(snapshot); if (!Number.isFinite(d.getTime())) return fail();
  return `${String(d.getUTCFullYear()).padStart(4, "0")}${String(d.getUTCMonth() + 1).padStart(2, "0")}${String(d.getUTCDate()).padStart(2, "0")}T${String(d.getUTCHours()).padStart(2, "0")}${String(d.getUTCMinutes()).padStart(2, "0")}${String(d.getUTCSeconds()).padStart(2, "0")}Z`;
}
function validateCsv(csv: string, brand: string, q: Omit<CanonicalQuery, "limit" | "offset">, headers: Headers, byteLength: number): { filename: string; groupCount: string } {
  const columns = ["record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "agent_id", "member_id", "cycle_id", "key", "label", ...FIELDS];
  const rows = parseCsv(csv);
  if (rows.length < 2 || rows[0]!.length !== columns.length || rows[0]!.some((v, i) => v !== columns[i])) return fail("佣金 CSV 列格式无效。");
  const header = (name: string) => headers.get(name) ?? "";
  const snapshot = header("X-Report-Snapshot-At"), groupCount = header("X-Report-Group-Count"), audit = header("X-Report-Audit-ID");
  const digest = header("X-Report-SHA256"), filename = `lottery-commission-${brand}-${filenameDate(snapshot)}.csv`;
  const timezone = header("X-Report-Timezone");
  if (header("X-Report-Brand-ID").toLowerCase() !== brand.toLowerCase() || header("X-Report-Kind") !== "commission" || !validSnapshot(snapshot) || !UUID.test(audit) || !/^[a-f0-9]{64}$/.test(digest) || header("X-Report-Format-Version") !== "2" || !UNSIGNED.test(groupCount) || BigInt(groupCount) > BigInt(MAX_GROUPS) || header("Content-Length") !== String(byteLength) || header("X-Report-Byte-Count") !== String(byteLength) || byteLength > MAX_BYTES || !validTimezone(timezone) || !/^text\/csv\s*;\s*charset=utf-8$/i.test(header("Content-Type")) || header("Cache-Control").toLowerCase() !== "no-store" || header("Content-Disposition") !== `attachment; filename="${filename}"`) return fail("佣金 CSV 元数据校验失败。");
  const metadata = rows.slice(1).map((r) => { if (r.length !== columns.length) return fail("佣金 CSV 列数无效。"); return r; });
  const validateMeta = (r: string[]) => r[1]!.toLowerCase() === brand.toLowerCase() && r[2] === snapshot && r[3] === timezone && r[4] === q.from && r[5] === q.to && r[6] === q.group_by && r[7] === (q.agent_id ?? "") && r[8] === (q.member_id ?? "") && r[9] === (q.cycle_id ?? "");
  if (metadata.length !== Number(groupCount) + 1 || metadata.some((r) => !validateMeta(r))) return fail("佣金 CSV 内容与请求范围不匹配。");
  const summary = metadata[0]!;
  if (summary[0] !== "summary" || summary[10] !== "" || summary[11] !== "") return fail();
  const parseTotals = (r: string[]) => {
    const raw = Object.fromEntries(FIELDS.map((f, i) => {
      const cell = r[12 + i]!;
      if (f === "net_points" && cell.startsWith("'")) {
        if (!/^'-(?:[1-9]\d*)$/.test(cell)) return [f, "invalid"];
        return [f, cell.slice(1)];
      }
      if (f === "net_points" && cell.startsWith("-")) return [f, "invalid"];
      return [f, cell];
    }));
    return totals(raw);
  };
  const total = parseTotals(summary), sums = FIELDS.map(() => 0n); let prior = "";
  for (const row of metadata.slice(1)) {
    const key = row[10]!;
    if (row[0] !== "group" || !key || key !== row[11] || /^[\s\u0000-\u001f\uFEFF]*[=+@-]/.test(key) || key <= prior) return fail("佣金 CSV 分组格式无效。");
    if (q.group_by === "day" && !validDay(key) || q.group_by === "agent" && !UUID.test(key) || q.group_by === "cycle" && !UUID.test(key)) return fail();
    prior = key; const item = parseTotals(row); FIELDS.forEach((f, i) => sums[i] += BigInt(item[f]));
  }
  if (FIELDS.some((f, i) => sums[i] !== BigInt(total[f]))) return fail("佣金 CSV 分组汇总与总计不一致。");
  return { filename, groupCount };
}

export function commissionReportPermissions(account: AdminAccount, brand: string) {
  const valid = UUID.test(account.id) && UUID.test(brand);
  const inBrand = (account.brand_ids ?? []).some((id) => id.toLowerCase() === brand.toLowerCase());
  const grants = new Set(account.permissions_by_brand?.[brand] ?? []), platform = new Set(account.platform_permissions ?? []);
  const view = valid && (platform.has("report_commission.view.platform") || inBrand && grants.has("report_commission.view.brand"));
  const exportAllowed = view && (platform.has("report_commission.export.platform") || inBrand && grants.has("report_commission.export.brand"));
  return { view, export: exportAllowed };
}

export function createCommissionReportApi(fetcher: FetchLike = fetch) {
  async function errorResponse(response: Response): Promise<never> {
    let message = `Request failed (${response.status})`, code: string | undefined;
    try { const body: unknown = await response.json(); if (isRecord(body) && isRecord(body.error)) { if (typeof body.error.message === "string") message = body.error.message; if (typeof body.error.code === "string") code = body.error.code; } } catch { /* Keep the status fallback for non-JSON errors. */ }
    throw new AdminApiError(message, response.status, code);
  }
  async function report(brand: string, input: CommissionReportQuery): Promise<CommissionReport> {
    if (!UUID.test(brand)) return bad("品牌 UUID 无效。");
    const brandId = brand.toLowerCase();
    const q = canonical(input), params = new URLSearchParams();
    for (const [key, value] of Object.entries(q)) if (value !== null) params.set(key, String(value));
    const response = await fetcher(`${BASE}?${params}`, { method: "GET", credentials: "same-origin", headers: { "X-Brand-ID": brandId, Accept: "application/json" } });
    if (!response.ok) return errorResponse(response);
    let body: unknown; try { body = await response.json(); } catch { body = null; }
    if (!isRecord(body) || !exactKeys(body, ["success", "data", "request_id"]) || body.success !== true || typeof body.request_id !== "string" || !/^[a-zA-Z0-9_.:-]{1,80}$/.test(body.request_id) || !isRecord(body.data)) return fail();
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
    const sums = FIELDS.map(() => 0n);
    for (let i = 0; i < items.length; i++) { if (i && items[i - 1]!.key >= items[i]!.key) return fail(); FIELDS.forEach((f, j) => sums[j] += BigInt(items[i]!.totals[f])); }
    if (q.offset === 0 && BigInt(items.length) === BigInt(d.total_groups) && FIELDS.some((f, i) => sums[i] !== BigInt(summary[f]))) return fail("佣金分组与汇总不一致。");
    return { brand_id: brandId, snapshot_at: d.snapshot_at as string, timezone: d.timezone, query: echo as CommissionReport["query"], summary, items, total_groups: d.total_groups };
  }
  async function exportCsv(brand: string, input: CommissionReportQuery) {
    if (!UUID.test(brand)) return bad("品牌 UUID 无效。");
    const brandId = brand.toLowerCase();
    if (input.limit !== undefined || input.offset !== undefined) return bad("CSV 导出不接受分页参数。");
    const q = canonical(input), params = new URLSearchParams();
    for (const key of ["from", "to", "group_by", "agent_id", "member_id", "cycle_id"] as const) { const value = q[key]; if (value !== null) params.set(key, String(value)); }
    const response = await fetcher(`${BASE}.csv?${params}`, { method: "GET", credentials: "same-origin", headers: { "X-Brand-ID": brandId, Accept: "text/csv" } });
    if (!response.ok) return errorResponse(response);
    const length = response.headers.get("Content-Length");
    if (!length || !UNSIGNED.test(length) || Number(length) > MAX_BYTES) return fail("佣金 CSV 文件大小无效。");
    const bytes = new Uint8Array(await response.arrayBuffer());
    if (bytes.length !== Number(length) || bytes.length > MAX_BYTES || bytes[0] !== 0xef || bytes[1] !== 0xbb || bytes[2] !== 0xbf) return fail("佣金 CSV 长度或 BOM 校验失败。");
    const digest = [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))].map((n) => n.toString(16).padStart(2, "0")).join("");
    if (digest !== response.headers.get("X-Report-SHA256")) return fail("佣金 CSV SHA-256 校验失败。");
    let decoded: string; try { decoded = new TextDecoder("utf-8", { fatal: true }).decode(bytes.subarray(3)); } catch { return fail("佣金 CSV 不是有效 UTF-8。"); }
    const validated = validateCsv(decoded, brandId, { from: q.from, to: q.to, group_by: q.group_by, agent_id: q.agent_id, member_id: q.member_id, cycle_id: q.cycle_id }, response.headers, bytes.length);
    return { ...validated, bytes, snapshotAt: response.headers.get("X-Report-Snapshot-At")!, auditLogId: response.headers.get("X-Report-Audit-ID")! };
  }
  return { report, exportCsv };
}
