import { AdminApiError, type AdminAccount } from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";

const BASE = "/api/v1/admin/reports/commission-analysis";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const UNSIGNED = /^(0|[1-9]\d*)$/;
const SIGNED = /^(0|-?[1-9]\d*)$/;
const MAX_WINDOW_NS = 93n * 86_400_000_000_000n;
const MAX_GROUPS = 10_000;
const MAX_BYTES = 4 * 1024 * 1024;
export const COMMISSION_ANALYSIS_FIELDS = [
  "observed_calculated_points", "calculated_points", "paid_entry_count", "paid_points",
  "adjustment_entry_count", "adjustment_credit_points", "adjustment_debit_points",
  "correction_entry_count", "correction_credit_points", "correction_debit_points",
  "posting_entry_count", "actual_net_points", "manual_adjustment_net_points",
  "effective_target_points", "calculation_minus_actual_points", "effective_minus_actual_points",
  "calculation_complete", "effective_target_complete",
] as const;
export const COMMISSION_ANALYSIS_COVERAGE_FIELDS = ["selected_cycle_count", "ready_cycle_count", "unready_cycle_count"] as const;
const UUID_FIELDS = ["agent_id", "member_id", "cycle_id"] as const;
const CSV_COLUMNS = ["record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "agent_id", "member_id", "cycle_id", "key", "label", ...COMMISSION_ANALYSIS_COVERAGE_FIELDS, ...COMMISSION_ANALYSIS_FIELDS] as const;
type AnalysisField = typeof COMMISSION_ANALYSIS_FIELDS[number];
type CoverageField = typeof COMMISSION_ANALYSIS_COVERAGE_FIELDS[number];
export type CommissionAnalysisGroup = "cycle" | "agent";
export interface CommissionAnalysisQuery { from: string; to: string; group_by: CommissionAnalysisGroup; limit?: number; offset?: number; agent_id?: string | null; member_id?: string | null; cycle_id?: string | null }
export type CommissionAnalysisTotals = { [K in AnalysisField]: K extends "calculation_complete" | "effective_target_complete" ? boolean : string | null };
export type CommissionAnalysisCoverage = Record<CoverageField, string>;
export interface CommissionAnalysisReport {
  brand_id: string; snapshot_at: string; timezone: string;
  query: { from: string; to: string; group_by: CommissionAnalysisGroup; limit: number; offset: number; agent_id: string | null; member_id: string | null; cycle_id: string | null };
  coverage: CommissionAnalysisCoverage; summary: CommissionAnalysisTotals;
  items: { key: string; label: string; totals: CommissionAnalysisTotals }[]; total_groups: string;
}
export type CommissionAnalysisExport = { filename: string; bytes: Uint8Array; groupCount: string; snapshotAt: string; auditLogId: string };
type CanonicalQuery = CommissionAnalysisReport["query"];
type FetchLike = typeof fetch;
const nullableFields = new Set<AnalysisField>(["calculated_points", "effective_target_points", "calculation_minus_actual_points", "effective_minus_actual_points"]);
const signedFields = new Set<AnalysisField>(["actual_net_points", "manual_adjustment_net_points", "calculation_minus_actual_points", "effective_minus_actual_points"]);
const boolFields = new Set<AnalysisField>(["calculation_complete", "effective_target_complete"]);

const isRecord = (v: unknown): v is Record<string, unknown> => !!v && typeof v === "object" && !Array.isArray(v);
const exactKeys = (v: Record<string, unknown>, keys: readonly string[]) => Object.keys(v).length === keys.length && Object.keys(v).every((k) => keys.includes(k));
function fail(message = "佣金周期分析响应格式无效。", status = 502, code = "INVALID_RESPONSE"): never { throw new AdminApiError(message, status, code); }
function bad(message = "佣金周期分析筛选无效。"): never { return fail(message, 0, "INVALID_INPUT"); }
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
function canonical(q: CommissionAnalysisQuery): CanonicalQuery {
  if (!q || Object.keys(q).some((k) => !["from", "to", "group_by", "limit", "offset", ...UUID_FIELDS].includes(k))) return bad("报表包含不支持的筛选参数。");
  const start = epochNs(q.from), end = epochNs(q.to);
  if (end <= start || end - start > MAX_WINDOW_NS) return bad("时间范围必须大于零且不超过 93 天。");
  if (q.group_by !== "cycle" && q.group_by !== "agent") return bad("佣金周期分析分组无效。");
  for (const id of [q.agent_id, q.member_id, q.cycle_id]) if (id != null && (typeof id !== "string" || !UUID.test(id))) return bad("筛选 UUID 格式无效。");
  const limit = q.limit ?? 20, offset = q.offset ?? 0;
  if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000) return bad("分页参数无效。");
  return { from: utc(start), to: utc(end), group_by: q.group_by, limit, offset, agent_id: q.agent_id?.toLowerCase() ?? null, member_id: q.member_id?.toLowerCase() ?? null, cycle_id: q.cycle_id?.toLowerCase() ?? null };
}
function validSnapshot(value: unknown): value is string {
  if (typeof value !== "string" || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value)) return false;
  try { return utc(epochNs(value)) === value; } catch { return false; }
}
function validTimezone(value: unknown): value is string { try { return typeof value === "string" && !!value && value !== "Local" && value.length <= 100 && !!new Intl.DateTimeFormat("en", { timeZone: value }); } catch { return false; } }
function coverage(value: unknown): CommissionAnalysisCoverage {
  if (!isRecord(value) || !exactKeys(value, COMMISSION_ANALYSIS_COVERAGE_FIELDS) || COMMISSION_ANALYSIS_COVERAGE_FIELDS.some((f) => typeof value[f] !== "string" || !UNSIGNED.test(value[f] as string))) return fail("周期覆盖计数格式无效。");
  const v = value as unknown as CommissionAnalysisCoverage;
  if (BigInt(v.selected_cycle_count) !== BigInt(v.ready_cycle_count) + BigInt(v.unready_cycle_count)) return fail("周期覆盖计数不一致。");
  return v;
}
function totals(value: unknown): CommissionAnalysisTotals {
  if (!isRecord(value) || !exactKeys(value, COMMISSION_ANALYSIS_FIELDS)) return fail("分析金额字段不完整。");
  for (const field of COMMISSION_ANALYSIS_FIELDS) {
    const raw = value[field];
    if (boolFields.has(field)) { if (typeof raw !== "boolean") return fail(); }
    else if (raw === null) { if (!nullableFields.has(field)) return fail(); }
    else if (typeof raw !== "string" || !(signedFields.has(field) ? SIGNED : UNSIGNED).test(raw)) return fail("分析金额必须是规范十进制字符串。");
  }
  const v = value as unknown as CommissionAnalysisTotals;
  const add = (...fields: AnalysisField[]) => fields.map((f) => BigInt(v[f] as string));
  const [paidCount, adjustmentCount, correctionCount] = add("paid_entry_count", "adjustment_entry_count", "correction_entry_count");
  if (BigInt(v.posting_entry_count!) !== paidCount! + adjustmentCount! + correctionCount!) return fail("入账笔数分项与总计不一致。");
  const [paid, adjustmentCredit, adjustmentDebit, correctionCredit, correctionDebit] = add("paid_points", "adjustment_credit_points", "adjustment_debit_points", "correction_credit_points", "correction_debit_points");
  const actual = paid! + adjustmentCredit! - adjustmentDebit! + correctionCredit! - correctionDebit!;
  if (BigInt(v.actual_net_points!) !== actual || BigInt(v.manual_adjustment_net_points!) !== adjustmentCredit! - adjustmentDebit!) return fail("实际净额或人工修正净额不一致。");
  if (v.effective_target_complete && !v.calculation_complete) return fail("有效目标完整性不能超过核算完整性。");
  if (v.calculation_complete !== (v.calculated_points !== null) || v.effective_target_complete !== (v.effective_target_points !== null)) return fail("完整性标记与可空目标不一致。");
  if (v.calculation_complete && v.observed_calculated_points !== v.calculated_points) return fail("完整核算与已知核算积分不一致。");
  const calculatedDiff = v.calculated_points === null ? null : BigInt(v.calculated_points) - actual;
  const effectiveDiff = v.effective_target_points === null ? null : BigInt(v.effective_target_points) - actual;
  if ((calculatedDiff === null ? v.calculation_minus_actual_points !== null : v.calculation_minus_actual_points === null || BigInt(v.calculation_minus_actual_points) !== calculatedDiff) ||
      (effectiveDiff === null ? v.effective_minus_actual_points !== null : v.effective_minus_actual_points === null || BigInt(v.effective_minus_actual_points) !== effectiveDiff)) return fail("分析差额与目标、实际净额不一致。");
  return v;
}
function sameCoverage(a: CommissionAnalysisCoverage, b: CommissionAnalysisCoverage): boolean { return COMMISSION_ANALYSIS_COVERAGE_FIELDS.every((f) => a[f] === b[f]); }
function assertCoverageSummary(rows: CommissionAnalysisTotals[], summary: CommissionAnalysisTotals, cov: CommissionAnalysisCoverage, group: CommissionAnalysisGroup, count: string) {
  if (summary.calculation_complete !== (BigInt(cov.unready_cycle_count) === 0n)) return fail("核算完整性与周期覆盖不一致。");
  if (group === "cycle" && BigInt(count) !== BigInt(cov.selected_cycle_count)) return fail("周期分组数量与选中周期不一致。");
  if (group === "agent" && rows.some((row) => row.calculation_complete !== summary.calculation_complete)) return fail("代理核算完整性与完整范围不一致。");
}
function assertAggregate(rows: CommissionAnalysisTotals[], summary: CommissionAnalysisTotals, cov: CommissionAnalysisCoverage, group: CommissionAnalysisGroup) {
  const unknownEmptyAgent = rows.length === 0 && group === "agent" && BigInt(cov.unready_cycle_count) > 0n;
  const aggregateFields = COMMISSION_ANALYSIS_FIELDS.filter((f) => !boolFields.has(f));
  for (const field of aggregateFields) {
    const values = rows.map((r) => r[field]);
    const expected = nullableFields.has(field) && (unknownEmptyAgent || values.some((v) => v === null)) ? null : values.reduce((sum, v) => sum + BigInt(v as string), 0n).toString();
    if (summary[field] !== expected) fail("分组汇总与完整范围总计不一致。");
  }
  for (const flag of ["calculation_complete", "effective_target_complete"] as const) if (summary[flag] !== (!unknownEmptyAgent && rows.every((row) => row[flag]))) fail("完整性汇总与分组不一致。");
}
function parseCsv(text: string): string[][] {
  const rows: string[][] = []; let row: string[] = [], field = "", quoted = false, afterQuote = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i]!;
    if (quoted) { if (c === '"') { if (text[i + 1] === '"') { field += '"'; i++; } else { quoted = false; afterQuote = true; } } else field += c; continue; }
    if (afterQuote && c !== "," && c !== "\n") return fail("佣金分析 CSV 格式无效。");
    if (c === '"') { if (field || afterQuote) return fail(); quoted = true; }
    else if (c === ",") { row.push(field); field = ""; afterQuote = false; }
    else if (c === "\n") { row.push(field); rows.push(row); row = []; field = ""; afterQuote = false; }
    else { if (c === "\r" || c.charCodeAt(0) < 0x20) return fail(); field += c; }
  }
  if (quoted || !text.endsWith("\n") || row.length || field || afterQuote) return fail();
  return rows;
}
function filenameDate(snapshot: string): string { const d = new Date(snapshot); return `${String(d.getUTCFullYear()).padStart(4, "0")}${String(d.getUTCMonth() + 1).padStart(2, "0")}${String(d.getUTCDate()).padStart(2, "0")}T${String(d.getUTCHours()).padStart(2, "0")}${String(d.getUTCMinutes()).padStart(2, "0")}${String(d.getUTCSeconds()).padStart(2, "0")}Z`; }
function csvTotals(row: string[]): CommissionAnalysisTotals {
  const raw: Record<string, unknown> = {};
  COMMISSION_ANALYSIS_FIELDS.forEach((field, index) => {
    let cell: string | null = row[12 + COMMISSION_ANALYSIS_COVERAGE_FIELDS.length + index]!;
    if (boolFields.has(field)) { if (cell !== "true" && cell !== "false") return fail("CSV 布尔字段无效。"); raw[field] = cell === "true"; return; }
    if (nullableFields.has(field) && cell === "") { raw[field] = null; return; }
    if (signedFields.has(field) && cell.startsWith("'")) { cell = cell.slice(1); if (!/^-[1-9]\d*$/.test(cell)) return fail("CSV 负数前缀无效。"); }
    else if (signedFields.has(field) && cell.startsWith("-")) return fail("CSV 负数必须使用安全前缀。");
    raw[field] = cell;
  });
  return totals(raw);
}
function validateCsv(csv: string, brand: string, q: Omit<CanonicalQuery, "limit" | "offset">, headers: Headers, byteLength: number): { filename: string; groupCount: string } {
  const rows = parseCsv(csv);
  if (rows.length < 2 || rows[0]!.length !== CSV_COLUMNS.length || rows[0]!.some((v, i) => v !== CSV_COLUMNS[i])) return fail("佣金分析 CSV 列格式无效。");
  const h = (name: string) => headers.get(name) ?? "";
  const snapshot = h("X-Report-Snapshot-At"), count = h("X-Report-Group-Count"), audit = h("X-Report-Audit-ID");
  const filename = `lottery-commission-analysis-${brand}-${filenameDate(snapshot)}.csv`, zone = h("X-Report-Timezone");
  if (h("X-Report-Brand-ID").toLowerCase() !== brand || h("X-Report-Kind") !== "commission_analysis" || !validSnapshot(snapshot) || !UUID.test(audit) || !/^[a-f0-9]{64}$/.test(h("X-Report-SHA256")) || h("X-Report-Format-Version") !== "1" || !UNSIGNED.test(count) || BigInt(count) > BigInt(MAX_GROUPS) || h("Content-Length") !== String(byteLength) || h("X-Report-Byte-Count") !== String(byteLength) || byteLength > MAX_BYTES || !validTimezone(zone) || !/^text\/csv\s*;\s*charset=utf-8$/i.test(h("Content-Type")) || h("Cache-Control").toLowerCase() !== "no-store" || h("Content-Disposition") !== `attachment; filename="${filename}"`) return fail("佣金分析 CSV 元数据校验失败。");
  const records = rows.slice(1).map((r) => { if (r.length !== CSV_COLUMNS.length) return fail("佣金分析 CSV 列数无效。"); return r; });
  const matchesScope = (r: string[]) => r[1]!.toLowerCase() === brand && r[2] === snapshot && r[3] === zone && r[4] === q.from && r[5] === q.to && r[6] === q.group_by && r[7] === (q.agent_id ?? "") && r[8] === (q.member_id ?? "") && r[9] === (q.cycle_id ?? "");
  if (records.length !== Number(count) + 1 || records.some((r) => !matchesScope(r))) return fail("佣金分析 CSV 内容与请求范围不匹配。");
  const recordCoverage = (r: string[]) => coverage(Object.fromEntries(COMMISSION_ANALYSIS_COVERAGE_FIELDS.map((f, i) => [f, r[12 + i]])));
  const summary = records[0]!;
  if (summary[0] !== "summary" || summary[10] !== "" || summary[11] !== "") return fail();
  const summaryCoverage = recordCoverage(summary), summaryTotals = csvTotals(summary);
  const groups: CommissionAnalysisTotals[] = []; let prior = "";
  for (const r of records.slice(1)) {
    const key = r[10]!;
    if (r[0] !== "group" || !UUID.test(key) || key !== key.toLowerCase() || r[11] !== key || key <= prior || !sameCoverage(recordCoverage(r), summaryCoverage)) return fail("佣金分析 CSV 分组格式无效。");
    prior = key; groups.push(csvTotals(r));
  }
  assertCoverageSummary(groups, summaryTotals, summaryCoverage, q.group_by, count);
  assertAggregate(groups, summaryTotals, summaryCoverage, q.group_by);
  return { filename, groupCount: count };
}
async function parseError(response: Response): Promise<never> {
  let message = `Request failed (${response.status})`, code: string | undefined;
  try { const body: unknown = await response.json(); if (isRecord(body) && isRecord(body.error)) { if (typeof body.error.message === "string") message = body.error.message; if (typeof body.error.code === "string") code = body.error.code; } } catch { /* retain status fallback */ }
  throw new AdminApiError(message, response.status, code);
}
export function commissionAnalysisPermissions(account: AdminAccount, brand: string) {
  const valid = UUID.test(account.id) && UUID.test(brand);
  const grants = brandPermissionSet(account, brand.toLowerCase());
  const scoped = (permission: string) => grants.has(`${permission}.brand`);
  const view = valid && scoped("commission.view") && scoped("report_commission.view");
  const exportAllowed = view && scoped("report_commission.export");
  return { view, export: exportAllowed };
}
export function createCommissionAnalysisApi(fetcher: FetchLike = fetch) {
  async function report(brand: string, input: CommissionAnalysisQuery, signal?: AbortSignal): Promise<CommissionAnalysisReport> {
    if (!UUID.test(brand)) return bad("品牌 UUID 无效。");
    const brandId = brand.toLowerCase(), q = canonical(input), params = new URLSearchParams();
    for (const key of ["from", "to", "group_by", "limit", "offset", ...UUID_FIELDS] as const) { const value = q[key]; if (value !== null && value !== undefined) params.set(key, String(value)); }
    const response = await fetcher(`${BASE}?${params}`, { method: "GET", credentials: "same-origin", headers: { Accept: "application/json", "X-Brand-ID": brandId }, signal });
    if (!response.ok) return parseError(response);
    let body: unknown; try { body = await response.json(); } catch { return fail("佣金周期分析 JSON 无效。"); }
    if (!isRecord(body) || body.success !== true || !isRecord(body.data)) return fail("佣金周期分析 JSON 无效。");
    const d = body.data;
    if (!exactKeys(d, ["brand_id", "snapshot_at", "timezone", "query", "coverage", "summary", "items", "total_groups"]) || typeof d.brand_id !== "string" || d.brand_id.toLowerCase() !== brandId || !validSnapshot(d.snapshot_at) || !validTimezone(d.timezone) || !isRecord(d.query) || !exactKeys(d.query, ["from", "to", "group_by", "limit", "offset", ...UUID_FIELDS])) return fail();
    const echo = d.query;
    if (echo.from !== q.from || echo.to !== q.to || echo.group_by !== q.group_by || echo.limit !== q.limit || echo.offset !== q.offset || UUID_FIELDS.some((f) => echo[f] !== q[f])) return fail("响应筛选与请求不一致。");
    const cov = coverage(d.coverage), summary = totals(d.summary);
    if (typeof d.total_groups !== "string" || !UNSIGNED.test(d.total_groups) || !Array.isArray(d.items)) return fail();
    const items = d.items.map((raw) => {
      if (!isRecord(raw) || !exactKeys(raw, ["key", "label", "totals"]) || typeof raw.key !== "string" || raw.key !== raw.key.toLowerCase() || !UUID.test(raw.key) || raw.label !== raw.key) return fail("分组键必须是规范 UUID，标签必须与键一致。");
      return { key: raw.key, label: raw.label, totals: totals(raw.totals) };
    });
    if (items.length > q.limit || BigInt(d.total_groups) < BigInt(items.length) || items.length > 0 && BigInt(q.offset) + BigInt(items.length) > BigInt(d.total_groups)) return fail("分页响应范围无效。");
    if (items.some((item, i) => i > 0 && items[i - 1]!.key >= item.key)) return fail("分组顺序必须按 C 顺序严格递增。");
    const groupTotals = items.map((i) => i.totals);
    assertCoverageSummary(groupTotals, summary, cov, q.group_by, d.total_groups);
    if ((q.offset === 0 || d.total_groups === "0") && BigInt(items.length) === BigInt(d.total_groups)) assertAggregate(groupTotals, summary, cov, q.group_by);
    return { brand_id: brandId, snapshot_at: d.snapshot_at, timezone: d.timezone, query: echo as CanonicalQuery, coverage: cov, summary, items, total_groups: d.total_groups };
  }
  async function exportCsv(brand: string, input: CommissionAnalysisQuery, signal?: AbortSignal): Promise<CommissionAnalysisExport> {
    if (!UUID.test(brand)) return bad("品牌 UUID 无效。");
    if (input.limit !== undefined || input.offset !== undefined) return bad("CSV 导出不接受分页参数。");
    const brandId = brand.toLowerCase(), q = canonical(input), params = new URLSearchParams();
    for (const key of ["from", "to", "group_by", ...UUID_FIELDS] as const) { const value = q[key]; if (value !== null && value !== undefined) params.set(key, String(value)); }
    const response = await fetcher(`${BASE}.csv?${params}`, { method: "GET", credentials: "same-origin", headers: { Accept: "text/csv", "X-Brand-ID": brandId }, signal });
    if (!response.ok) return parseError(response);
    const length = response.headers.get("Content-Length");
    if (!length || !UNSIGNED.test(length) || Number(length) > MAX_BYTES) return fail("佣金分析 CSV 文件大小无效。");
    const bytes = new Uint8Array(await response.arrayBuffer());
    if (bytes.length !== Number(length) || bytes.length > MAX_BYTES || bytes[0] !== 0xef || bytes[1] !== 0xbb || bytes[2] !== 0xbf) return fail("佣金分析 CSV 长度或 BOM 校验失败。");
    const digest = [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))].map((v) => v.toString(16).padStart(2, "0")).join("");
    if (digest !== response.headers.get("X-Report-SHA256")) return fail("佣金分析 CSV SHA-256 校验失败。");
    let text: string; try { text = new TextDecoder("utf-8", { fatal: true }).decode(bytes.subarray(3)); } catch { return fail("佣金分析 CSV 不是有效 UTF-8。"); }
    const validated = validateCsv(text, brandId, { from: q.from, to: q.to, group_by: q.group_by, agent_id: q.agent_id, member_id: q.member_id, cycle_id: q.cycle_id }, response.headers, bytes.length);
    return { ...validated, bytes, snapshotAt: response.headers.get("X-Report-Snapshot-At")!, auditLogId: response.headers.get("X-Report-Audit-ID")! };
  }
  return { report, exportCsv };
}
