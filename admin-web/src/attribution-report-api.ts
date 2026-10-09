import { AdminApiError, type AdminAccount } from "./admin-api";

const BASE = "/api/v1/admin/reports/attribution";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const UNSIGNED = /^(0|[1-9]\d*)$/;
const MAX_WINDOW_NS = 93n * 24n * 60n * 60n * 1_000_000_000n;
const MAX_GROUPS = 10_000;
const MAX_BYTES = 4 * 1024 * 1024;
const GROUPS = ["day", "game", "member", "agent", "join_method"] as const;
const JOIN_METHODS = ["domain", "operator", "agent_code", "referral_code", "legacy"] as const;
export type AttributionGroupBy = typeof GROUPS[number];
export type AttributionJoinMethod = typeof JOIN_METHODS[number];
export interface AttributionReportQuery {
  from: string; to: string; group_by: AttributionGroupBy; limit?: number; offset?: number;
  game_id?: string | null; member_id?: string | null; agent_id?: string | null;
  agent_scope?: "direct" | "downline"; join_method?: AttributionJoinMethod | null;
}
export interface AttributionTotals {
  order_count: string; stake_points: string; placed_count: string; won_count: string; lost_count: string;
  abnormal_count: string; cancelled_count: string; refund_points: string; settled_stake_points: string;
  unfinalized_stake_points: string; abnormal_stake_points: string; current_prize_points: string;
  correction_open_count: string; final_lost_stake_points: string; legacy_attribution_count: string;
}
export interface AttributionReport {
  brand_id: string; snapshot_at: string; timezone: string;
  query: { from: string; to: string; group_by: AttributionGroupBy; limit: number; offset: number; game_id: string | null; member_id: string | null; agent_id: string | null; agent_scope: "direct" | "downline"; join_method: AttributionJoinMethod | null };
  summary: AttributionTotals; items: { key: string; label: string; totals: AttributionTotals }[]; total_groups: string;
}
export type AttributionExport = { filename: string; bytes: Uint8Array; groupCount: string; snapshotAt: string; auditLogId: string };

const FIELDS = ["order_count", "stake_points", "placed_count", "won_count", "lost_count", "abnormal_count", "cancelled_count", "refund_points", "settled_stake_points", "unfinalized_stake_points", "abnormal_stake_points", "current_prize_points", "correction_open_count", "final_lost_stake_points", "legacy_attribution_count"] as const;
const COLUMNS = ["record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "game_id", "member_id", "agent_id", "agent_scope", "join_method", "key", "label", ...FIELDS] as const;
type CanonicalQuery = AttributionReport["query"];
type FetchLike = typeof fetch;
const isRecord = (v: unknown): v is Record<string, unknown> => !!v && typeof v === "object" && !Array.isArray(v);
const exactKeys = (v: Record<string, unknown>, keys: readonly string[]) => Object.keys(v).length === keys.length && Object.keys(v).every((k) => keys.includes(k));
function fail(message = "归因报表响应格式无效。", status = 502, code = "INVALID_RESPONSE"): never { throw new AdminApiError(message, status, code); }
function bad(message = "归因报表筛选无效。"): never { return fail(message, 0, "INVALID_INPUT"); }

function epochNs(value: unknown): bigint {
  if (typeof value !== "string") return bad("from 和 to 必须是有效 RFC3339 日期时间。");
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
function canonical(input: AttributionReportQuery): CanonicalQuery {
  if (!input || typeof input !== "object" || Object.keys(input).some((k) => !["from", "to", "group_by", "limit", "offset", "game_id", "member_id", "agent_id", "agent_scope", "join_method"].includes(k))) return bad("归因报表包含不支持的筛选参数。");
  const start = epochNs(input.from), end = epochNs(input.to);
  if (end <= start || end - start > MAX_WINDOW_NS) return bad("时间范围必须大于零且不超过 93 天。");
  if (!GROUPS.includes(input.group_by)) return bad("归因报表分组无效。");
  const agentScope = input.agent_scope ?? "direct";
  if (agentScope !== "direct" && agentScope !== "downline") return bad("代理范围无效。");
  const ids = [input.game_id, input.member_id, input.agent_id];
  if (ids.some((id) => id != null && (typeof id !== "string" || !UUID.test(id)))) return bad("筛选 UUID 格式无效。");
  if (input.join_method != null && !JOIN_METHODS.includes(input.join_method)) return bad("加入方式无效。");
  if (agentScope === "downline" && !input.agent_id) return bad("下级代理范围必须指定代理 UUID。");
  const limit = input.limit ?? 20, offset = input.offset ?? 0;
  if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000) return bad("分页参数无效。");
  return { from: utc(start), to: utc(end), group_by: input.group_by, limit, offset,
    game_id: input.game_id?.toLowerCase() ?? null, member_id: input.member_id?.toLowerCase() ?? null,
    agent_id: input.agent_id?.toLowerCase() ?? null, agent_scope: agentScope, join_method: input.join_method ?? null };
}
function totals(value: unknown): AttributionTotals {
  if (!isRecord(value) || !exactKeys(value, FIELDS) || FIELDS.some((field) => typeof value[field] !== "string" || !UNSIGNED.test(value[field] as string))) return fail();
  return value as unknown as AttributionTotals;
}
function validTimezone(v: string): boolean { try { return !!v && v.length <= 100 && new Intl.DateTimeFormat("en", { timeZone: v }).format(0) !== ""; } catch { return false; } }
function validSnapshot(v: string): boolean { if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(v)) return false; try { return utc(epochNs(v)) === v; } catch { return false; } }
function validDay(v: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(v)) return false;
  const [y, m, d] = v.split("-").map(Number); const date = new Date(0); date.setUTCHours(0, 0, 0, 0); date.setUTCFullYear(y!, m! - 1, d!);
  return date.getUTCFullYear() === y && date.getUTCMonth() === m! - 1 && date.getUTCDate() === d;
}
function validGroupKey(key: string, group: AttributionGroupBy): boolean {
  if (group === "day") return validDay(key);
  if (group === "game" || group === "member") return UUID.test(key);
  if (group === "agent") return key === "none" || key === "legacy" || UUID.test(key);
  return JOIN_METHODS.includes(key as AttributionJoinMethod);
}
function validGroupForQuery(key: string, q: CanonicalQuery): boolean {
  if (!validGroupKey(key, q.group_by)) return false;
  if (q.group_by === "game" && q.game_id !== null && key !== q.game_id) return false;
  if (q.group_by === "member" && q.member_id !== null && key !== q.member_id) return false;
  if (q.group_by === "join_method" && q.join_method !== null && key !== q.join_method) return false;
  if (q.group_by === "agent" && q.agent_scope === "direct" && q.agent_id !== null && key !== q.agent_id) return false;
  return true;
}
function validTotalsForQuery(value: AttributionTotals, q: CanonicalQuery): boolean {
  const orders = BigInt(value.order_count), legacy = BigInt(value.legacy_attribution_count);
  const statusOrders = BigInt(value.placed_count) + BigInt(value.won_count) + BigInt(value.lost_count) + BigInt(value.abnormal_count) + BigInt(value.cancelled_count);
  if (statusOrders !== orders || orders === 0n && FIELDS.some((field) => BigInt(value[field]) !== 0n)) return false;
  if (legacy > orders) return false;
  if (q.join_method === "legacy" && legacy !== orders) return false;
  if (q.join_method !== null && q.join_method !== "legacy" && legacy !== 0n) return false;
  if (q.agent_id !== null && legacy !== 0n) return false;
  if (q.agent_id !== null && q.join_method === "legacy" && FIELDS.some((field) => BigInt(value[field]) !== 0n)) return false;
  return true;
}
function sortedItems(raw: unknown, q: CanonicalQuery): AttributionReport["items"] {
  if (!Array.isArray(raw) || raw.length > q.limit) return fail();
  const items = raw.map((item): AttributionReport["items"][number] => {
    if (!isRecord(item) || !exactKeys(item, ["key", "label", "totals"]) || typeof item.key !== "string" || !item.key || typeof item.label !== "string" || !item.label || item.label.includes("\0") || !validGroupForQuery(item.key, q)) return fail();
    const parsed = totals(item.totals);
    if (!validTotalsForQuery(parsed, q)) return fail("归因分组与查询筛选不一致。");
    return { key: item.key, label: item.label, totals: parsed };
  });
  for (let i = 1; i < items.length; i++) if (items[i - 1]!.key >= items[i]!.key) return fail("归因报表分组顺序无效。");
  return items;
}
function sumMatches(items: AttributionReport["items"], summary: AttributionTotals): boolean {
  return FIELDS.every((field) => items.reduce((sum, item) => sum + BigInt(item.totals[field]), 0n) === BigInt(summary[field]));
}
function parseCsv(text: string): string[][] {
  const rows: string[][] = []; let row: string[] = [], field = "", quoted = false, afterQuote = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i]!;
    if (quoted) { if (c === '"') { if (text[i + 1] === '"') { field += '"'; i++; } else { quoted = false; afterQuote = true; } } else field += c; continue; }
    if (afterQuote && c !== "," && c !== "\n" && c !== "\r") return fail("归因 CSV 格式无效。");
    if (c === '"') { if (field || afterQuote) return fail("归因 CSV 格式无效。"); quoted = true; }
    else if (c === ",") { row.push(field); field = ""; afterQuote = false; }
    else if (c === "\n") { row.push(field); rows.push(row); row = []; field = ""; afterQuote = false; }
    else if (c === "\r") { if (text[i + 1] !== "\n") return fail("归因 CSV 格式无效。"); }
    else { if (c.charCodeAt(0) < 0x20) return fail("归因 CSV 格式无效。"); field += c; }
  }
  if (quoted || !text.endsWith("\n") || row.length || field || afterQuote) return fail("归因 CSV 格式无效。");
  return rows;
}
function filenameDate(snapshot: string): string { const d = new Date(snapshot); return `${String(d.getUTCFullYear()).padStart(4, "0")}${String(d.getUTCMonth() + 1).padStart(2, "0")}${String(d.getUTCDate()).padStart(2, "0")}T${String(d.getUTCHours()).padStart(2, "0")}${String(d.getUTCMinutes()).padStart(2, "0")}${String(d.getUTCSeconds()).padStart(2, "0")}Z`; }
function validateCsv(text: string, brand: string, q: CanonicalQuery, headers: Headers, byteLength: number): { filename: string; groupCount: string; snapshotAt: string; auditLogId: string } {
  const rows = parseCsv(text);
  if (rows.length < 2 || rows[0]!.length !== COLUMNS.length || rows[0]!.some((v, i) => v !== COLUMNS[i])) return fail("归因 CSV 列格式无效。");
  const header = (name: string) => headers.get(name) ?? "";
  const snapshot = header("X-Report-Snapshot-At"), groupCount = header("X-Report-Group-Count"), audit = header("X-Report-Audit-ID");
  const filename = `lottery-attribution-${brand}-${filenameDate(snapshot)}.csv`, timezone = header("X-Report-Timezone");
  if (header("X-Report-Brand-ID").toLowerCase() !== brand || header("X-Report-Kind") !== "attribution" || !validSnapshot(snapshot) || !UUID.test(audit) || !/^[a-f0-9]{64}$/.test(header("X-Report-SHA256")) || header("X-Report-Format-Version") !== "1" || !UNSIGNED.test(groupCount) || BigInt(groupCount) > BigInt(MAX_GROUPS) || header("Content-Length") !== String(byteLength) || header("X-Report-Byte-Count") !== String(byteLength) || byteLength > MAX_BYTES || !validTimezone(timezone) || !/^text\/csv\s*;\s*charset=utf-8$/i.test(header("Content-Type")) || header("Cache-Control").toLowerCase() !== "no-store" || header("Content-Disposition") !== `attachment; filename="${filename}"`) return fail("归因 CSV 元数据校验失败。");
  const data = rows.slice(1);
  if (data.length !== Number(groupCount) + 1 || data.some((r) => r.length !== COLUMNS.length)) return fail("归因 CSV 行数无效。");
  const queryCells = [q.from, q.to, q.group_by, q.game_id ?? "", q.member_id ?? "", q.agent_id ?? "", q.agent_scope, q.join_method ?? ""];
  for (const r of data) if (r[1]!.toLowerCase() !== brand || r[2] !== snapshot || r[3] !== timezone || queryCells.some((v, i) => r[i + 4] !== v)) return fail("归因 CSV 内容与请求范围不匹配。");
  const parseRowTotals = (r: string[]) => totals(Object.fromEntries(FIELDS.map((field, i) => [field, r[14 + i]])));
  const summary = data[0]!;
  if (summary[0] !== "summary" || summary[12] !== "" || summary[13] !== "") return fail();
  const summaryTotals = parseRowTotals(summary), sums = FIELDS.map(() => 0n); let prior = "";
  for (const r of data.slice(1)) {
    const key = r[12]!;
    if (r[0] !== "group" || !validGroupForQuery(key, q) || !r[13] || r[13]!.includes("\0") || /^[\s\u0000-\u001f\uFEFF]*[=+@-]/.test(r[13]!) || key <= prior) return fail("归因 CSV 分组格式无效。");
    prior = key; const item = parseRowTotals(r);
    if (!validTotalsForQuery(item, q)) return fail("归因 CSV 分组与查询筛选不一致。");
    FIELDS.forEach((field, i) => sums[i] += BigInt(item[field]));
  }
  if (!validTotalsForQuery(summaryTotals, q)) return fail("归因 CSV 汇总与查询筛选不一致。");
  if (FIELDS.some((field, i) => sums[i] !== BigInt(summaryTotals[field]))) return fail("归因 CSV 分组汇总与总计不一致。");
  return { filename, groupCount, snapshotAt: snapshot, auditLogId: audit };
}

export function attributionReportPermissions(account: AdminAccount, brand: string) {
  const brandId = brand.toLowerCase(), valid = UUID.test(account.id) && UUID.test(brandId), inBrand = (account.brand_ids ?? []).some((id) => id.toLowerCase() === brandId);
  const scoped = account.permissions_by_brand === undefined ? inBrand ? account.permissions ?? [] : [] : account.permissions_by_brand[brandId] ?? [];
  const grants = new Set(scoped), platform = new Set(account.platform_permissions ?? account.permissions ?? []);
  const view = valid && (platform.has("report_attribution.view.platform") || inBrand && grants.has("report_attribution.view.brand"));
  const exportAllowed = view && (platform.has("report_attribution.export.platform") || inBrand && grants.has("report_attribution.export.brand"));
  return { view, export: exportAllowed };
}

export function createAttributionReportApi(fetcher: FetchLike = fetch) {
  async function errorResponse(response: Response): Promise<never> {
    let message = `Request failed (${response.status})`, code: string | undefined;
    try { const body: unknown = await response.json(); if (isRecord(body) && isRecord(body.error)) { if (typeof body.error.message === "string") message = body.error.message; if (typeof body.error.code === "string") code = body.error.code; } } catch { /* status fallback */ }
    throw new AdminApiError(message, response.status, code);
  }
  async function request(brand: string, input: AttributionReportQuery, exportCsv: boolean): Promise<AttributionReport | AttributionExport> {
    if (!UUID.test(brand)) return bad("品牌 UUID 无效。");
    if (exportCsv && (input.limit !== undefined || input.offset !== undefined)) return bad("CSV 导出不接受分页参数。");
    const brandId = brand.toLowerCase(), q = canonical(input), params = new URLSearchParams();
    for (const [key, value] of Object.entries(q)) if (value !== null && (!exportCsv || key !== "limit" && key !== "offset")) params.set(key, String(value));
    const response = await fetcher(`${BASE}${exportCsv ? "/export" : ""}?${params}`, { method: "GET", credentials: "same-origin", headers: { "X-Brand-ID": brandId, Accept: exportCsv ? "text/csv" : "application/json" } });
    if (!response.ok) return errorResponse(response);
    if (exportCsv) {
      const length = response.headers.get("Content-Length");
      if (!length || !UNSIGNED.test(length) || Number(length) > MAX_BYTES) return fail("归因 CSV 文件大小无效。");
      const bytes = new Uint8Array(await response.arrayBuffer());
      if (bytes.length !== Number(length) || bytes.length > MAX_BYTES || bytes[0] !== 0xef || bytes[1] !== 0xbb || bytes[2] !== 0xbf) return fail("归因 CSV 长度或 BOM 校验失败。");
      const digest = [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))].map((n) => n.toString(16).padStart(2, "0")).join("");
      if (digest !== response.headers.get("X-Report-SHA256")) return fail("归因 CSV SHA-256 校验失败。");
      let decoded: string; try { decoded = new TextDecoder("utf-8", { fatal: true }).decode(bytes.subarray(3)); } catch { return fail("归因 CSV 不是有效 UTF-8。"); }
      return { ...validateCsv(decoded, brandId, q, response.headers, bytes.length), bytes };
    }
    let body: unknown; try { body = await response.json(); } catch { body = null; }
    if (!isRecord(body) || !exactKeys(body, ["success", "data", "request_id"]) || body.success !== true || typeof body.request_id !== "string" || !/^[a-zA-Z0-9_.:-]{1,80}$/.test(body.request_id) || !isRecord(body.data)) return fail();
    const d = body.data;
    if (!exactKeys(d, ["brand_id", "snapshot_at", "timezone", "query", "summary", "items", "total_groups"]) || typeof d.brand_id !== "string" || d.brand_id.toLowerCase() !== brandId || typeof d.snapshot_at !== "string" || !validSnapshot(d.snapshot_at) || typeof d.timezone !== "string" || !validTimezone(d.timezone) || !isRecord(d.query) || typeof d.total_groups !== "string" || !UNSIGNED.test(d.total_groups)) return fail();
    const echo = d.query;
    if (!exactKeys(echo, ["from", "to", "group_by", "limit", "offset", "game_id", "member_id", "agent_id", "agent_scope", "join_method"]) || echo.from !== q.from || echo.to !== q.to || echo.group_by !== q.group_by || echo.limit !== q.limit || echo.offset !== q.offset || echo.game_id !== q.game_id || echo.member_id !== q.member_id || echo.agent_id !== q.agent_id || echo.agent_scope !== q.agent_scope || echo.join_method !== q.join_method) return fail("归因报表回显范围与请求不一致。");
    const summary = totals(d.summary);
    if (!validTotalsForQuery(summary, q)) return fail("归因汇总与查询筛选不一致。");
    const items = sortedItems(d.items, q), total = BigInt(d.total_groups);
    if (total < BigInt(items.length) || items.length > 0 && BigInt(q.offset) + BigInt(items.length) > total) return fail();
    if (q.offset === 0 && BigInt(items.length) === total && !sumMatches(items, summary)) return fail("归因分组与汇总不一致。");
    return { brand_id: brandId, snapshot_at: d.snapshot_at, timezone: d.timezone, query: echo as AttributionReport["query"], summary, items, total_groups: d.total_groups };
  }
  return {
    report: async (brand: string, query: AttributionReportQuery) => await request(brand, query, false) as AttributionReport,
    exportCsv: async (brand: string, query: AttributionReportQuery) => await request(brand, query, true) as AttributionExport,
  };
}
