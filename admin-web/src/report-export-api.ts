import { AdminApiError, type AdminAccount } from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";
import type { ReportGroupBy, ReportQuery } from "./reports-api";

const BASE = "/api/v1/admin/reports";
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const INTEGER_RE = /^(0|[1-9]\d*)$/;
const SIGNED_INTEGER_RE = /^(?:0|-?[1-9]\d*)$/;
const MAX_WINDOW_NS = 93n * 24n * 60n * 60n * 1_000_000_000n;
const COMMON_COLUMNS = ["record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "game_id", "member_id", "key", "label"] as const;
const BETTING_FIELDS = ["order_count", "stake_points", "placed_count", "won_count", "lost_count", "abnormal_count", "cancelled_count", "refund_points", "settled_stake_points", "unfinalized_stake_points", "abnormal_stake_points", "current_prize_points", "correction_open_count"] as const;
const LEDGER_FIELDS = ["entry_count", "net_points", "recharge_points", "prize_credit_points", "prize_reversal_points", "refund_points"] as const;
const BALANCE_FIELDS = ["account_count", "available_points", "frozen_points", "withdrawal_points", "total_points"] as const;
const MAX_GROUPS = 10_000;
const MAX_BYTES = 4 * 1024 * 1024;

export type ExportKind = "betting" | "ledger";
export type ReportExport = { filename: string; bytes: Uint8Array; groupCount: string; snapshotAt: string; auditLogId: string };
export type ExportQuery = Pick<ReportQuery, "from" | "to" | "group_by" | "game_id" | "member_id">;

function fail(message = "报表导出响应格式无效。", status = 502, code = "INVALID_RESPONSE"): never {
  throw new AdminApiError(message, status, code);
}
function badInput(message: string): never { return fail(message, 0, "INVALID_INPUT"); }
function isRecord(value: unknown): value is Record<string, unknown> { return value !== null && typeof value === "object" && !Array.isArray(value); }
function validDateTime(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|([+-])(\d{2}):(\d{2}))$/.exec(value);
  if (!match) return false;
  const [, y, mo, d, h, mi, s, , zone, , zh, zm] = match;
  const year = Number(y), month = Number(mo), day = Number(d);
  const days = month === 2 ? (year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28) : ([4, 6, 9, 11].includes(month) ? 30 : 31);
  if (year < 1 || month < 1 || month > 12 || day < 1 || day > days || Number(h) > 23 || Number(mi) > 59 || Number(s) > 59) return false;
  return zone === "Z" || (Number(zh) <= 14 && Number(zm) <= 59 && !(Number(zh) === 14 && Number(zm) !== 0));
}
function validCanonicalUtcDateTime(value: string): boolean {
  if (!validDateTime(value) || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value)) return false;
  const fraction = /\.(\d+)Z$/.exec(value)?.[1];
  return fraction === undefined || !fraction.endsWith("0");
}
function validDay(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const [year, month, day] = value.split("-").map(Number);
  const date = new Date(Date.UTC(year!, month! - 1, day!));
  return date.getUTCFullYear() === year && date.getUTCMonth() === month! - 1 && date.getUTCDate() === day;
}
function epochNs(value: string): bigint {
  const fraction = /\.(\d{1,9})(?=Z|[+-]\d{2}:\d{2}$)/.exec(value)?.[1] ?? "";
  const milliseconds = Date.parse(value.replace(/\.\d+(?=Z|[+-]\d{2}:\d{2}$)/, ""));
  if (!Number.isFinite(milliseconds)) badInput("报表日期无效。");
  return BigInt(milliseconds) * 1_000_000n + BigInt(fraction.padEnd(9, "0") || "0");
}
function validateQuery(kind: ExportKind, query: ExportQuery): ExportQuery {
  if (!query || !validDateTime(query.from) || !validDateTime(query.to)) badInput("from 和 to 必须是有效 RFC3339 日期时间。");
  if (Object.keys(query).some((key) => !["from", "to", "group_by", "game_id", "member_id"].includes(key))) badInput("导出查询包含不支持的参数。");
  const start = epochNs(query.from), end = epochNs(query.to);
  if (end <= start || end - start > MAX_WINDOW_NS) badInput("报告时间范围必须大于零且不超过 93 天。");
  const groups: readonly ReportGroupBy[] = kind === "betting" ? ["day", "game", "member"] : ["day", "entry_type"];
  if (!groups.includes(query.group_by)) badInput("group_by 不适用于此报告。");
  if (query.game_id !== undefined && (kind !== "betting" || !UUID_RE.test(query.game_id))) badInput("game_id 不适用于此报告或 UUID 格式无效。");
  if (query.member_id !== undefined && !UUID_RE.test(query.member_id)) badInput("member_id UUID 格式无效。");
  return { from: query.from, to: query.to, group_by: query.group_by, ...(query.game_id ? { game_id: query.game_id } : {}), ...(query.member_id ? { member_id: query.member_id } : {}) };
}
function expectedColumns(kind: ExportKind): string[] { return [...COMMON_COLUMNS, ...(kind === "betting" ? BETTING_FIELDS : [...LEDGER_FIELDS, ...BALANCE_FIELDS])]; }
function parseCsv(text: string): string[][] {
  const rows: string[][] = [];
  let row: string[] = [], field = "", quoted = false, afterQuote = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i]!;
    if (quoted) {
      if (c === '"') {
        if (text[i + 1] === '"') { field += '"'; i++; }
        else { quoted = false; afterQuote = true; }
      } else field += c;
      continue;
    }
    if (afterQuote && c !== "," && c !== "\n") fail();
    if (c === '"') { if (field || afterQuote) fail(); quoted = true; }
    else if (c === ",") { row.push(field); field = ""; afterQuote = false; }
    else if (c === "\n") { row.push(field); rows.push(row); row = []; field = ""; afterQuote = false; }
    else { if (c === "\r" || c.charCodeAt(0) < 0x20 && c !== "\t") fail(); field += c; }
  }
  if (quoted || text.length === 0 || text[text.length - 1] !== "\n" || row.length !== 0 || field !== "" || afterQuote) fail();
  return rows;
}
function safeCell(value: string): boolean {
  return !/^[\s\u0000-\u001f]*[=+\-@]/.test(value) || value.startsWith("'");
}
function unescapeFormulaCell(value: string): string {
  const unprefixed = value.startsWith("'") ? value.slice(1) : value;
  return /^[\s\u0000-\u001f\u007f\uFEFF]*[=+\-@]/.test(unprefixed) ? unprefixed : value;
}
function sameInstant(a: string, b: string): boolean { try { return epochNs(a) === epochNs(b); } catch { return false; } }
function validTimezone(value: string): boolean {
  if (!value || value.length > 100) return false;
  try { new Intl.DateTimeFormat("en", { timeZone: value }); return true; } catch { return false; }
}
function filenameDate(snapshotAt: string): string {
  const date = new Date(snapshotAt);
  if (!Number.isFinite(date.getTime())) fail();
  return `${String(date.getUTCFullYear()).padStart(4, "0")}${String(date.getUTCMonth() + 1).padStart(2, "0")}${String(date.getUTCDate()).padStart(2, "0")}T${String(date.getUTCHours()).padStart(2, "0")}${String(date.getUTCMinutes()).padStart(2, "0")}${String(date.getUTCSeconds()).padStart(2, "0")}Z`;
}
async function parseError(response: Response): Promise<never> {
  let message = `Request failed (${response.status})`, code: string | undefined;
  let body: unknown;
  try {
    body = await response.json();
  } catch { /* Non-JSON error bodies are intentionally not downloaded. */ }
  if (isRecord(body) && typeof body.error === "string")
    throw new AdminApiError("Invalid server response", 502, "INVALID_RESPONSE");
  if (isRecord(body) && isRecord(body.error)) {
    if (typeof body.error.message === "string") message = body.error.message;
    if (typeof body.error.code === "string") code = body.error.code;
  }
  throw new AdminApiError(message, response.status, code);
}

function validateCsv(csvText: string, kind: ExportKind, brandId: string, query: ExportQuery, headers: Headers): { groupCount: string; snapshotAt: string } {
  const rows = parseCsv(csvText);
  const columns = expectedColumns(kind);
  if (rows.length < 2 || rows[0]!.length !== columns.length || rows[0]!.some((value, i) => value !== columns[i])) fail();
  const header = (name: string) => headers.get(name) ?? "";
  const snapshotAt = header("X-Report-Snapshot-At");
  const groupCount = header("X-Report-Group-Count");
  const auditId = header("X-Report-Audit-ID");
  const digest = header("X-Report-SHA256");
  if (header("X-Report-Brand-ID").toLowerCase() !== brandId.toLowerCase() || header("X-Report-Kind") !== kind ||
      !validDateTime(snapshotAt) || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(snapshotAt) ||
      !INTEGER_RE.test(groupCount) || BigInt(groupCount) > BigInt(MAX_GROUPS) ||
      !UUID_RE.test(auditId) || !/^[a-f0-9]{64}$/.test(digest) || header("X-Report-Format-Version") !== "1" ||
      !/^(0|[1-9]\d*)$/.test(header("Content-Length")) || Number(header("Content-Length")) > MAX_BYTES ||
      !/^text\/csv\s*;\s*charset=utf-8$/i.test(header("Content-Type")) || header("Cache-Control").toLowerCase() !== "no-store") fail();
  const filename = `lottery-${kind}-${brandId}-${filenameDate(snapshotAt)}.csv`;
  if (header("Content-Disposition") !== `attachment; filename="${filename}"`) fail();
  const metadata = rows.slice(1).map((row) => {
    if (row.length !== columns.length) fail();
    const record: Record<string, string> = {};
    columns.forEach((column, i) => { record[column] = row[i]!; });
    if (record.brand_id.toLowerCase() !== brandId.toLowerCase() || record.snapshot_at !== snapshotAt ||
        !validTimezone(record.timezone) || !validCanonicalUtcDateTime(record.from) || !sameInstant(record.from, query.from) ||
        !validCanonicalUtcDateTime(record.to) || !sameInstant(record.to, query.to) || record.group_by !== query.group_by ||
        record.game_id !== (query.game_id ?? "") || record.member_id !== (query.member_id ?? "")) fail("导出内容与请求范围不匹配。");
    return record;
  });
  if (metadata.length < 1 || metadata[0]!.record_type !== "summary" || metadata[0]!.key !== "" || metadata[0]!.label !== "") fail();
  let groupStart = 1;
  if (kind === "ledger") {
    if (metadata[1]?.record_type !== "balances") fail();
    if (metadata[1]!.key !== "" || metadata[1]!.label !== "" || LEDGER_FIELDS.some((field) => metadata[1]![field] !== "") ||
        BALANCE_FIELDS.some((field) => !INTEGER_RE.test(metadata[1]![field]!))) fail();
    const b = metadata[1]!;
    if (BigInt(b.available_points) + BigInt(b.frozen_points) + BigInt(b.withdrawal_points) !== BigInt(b.total_points)) fail();
    groupStart++;
  }
  const summary = metadata[0]!;
  const totalFields = kind === "betting" ? BETTING_FIELDS : LEDGER_FIELDS;
  for (const field of totalFields) if (!(field === "net_points" ? SIGNED_INTEGER_RE : INTEGER_RE).test(summary[field]!)) fail();
  if (kind === "ledger" && BALANCE_FIELDS.some((field) => summary[field] !== "")) fail();
  const stableTimezone = metadata[0]!.timezone;
  const aggregate = new Map<string, bigint>(totalFields.map((field) => [field, 0n]));
  const groupRows = metadata.slice(groupStart);
  let priorRawKey: string | null = null;
  for (const record of groupRows) {
    if (record.record_type !== "group" || record.key === "" || !safeCell(record.key) || !safeCell(record.label) ||
        record.timezone !== stableTimezone) fail();
    const rawKey = query.group_by === "entry_type" ? unescapeFormulaCell(record.key) : record.key;
    if ((priorRawKey !== null && priorRawKey >= rawKey) ||
        (query.group_by === "day" && (!validDay(rawKey) || record.label !== rawKey)) ||
        ((query.group_by === "game" || query.group_by === "member") && !UUID_RE.test(rawKey)) ||
        (query.group_by === "member" && record.label !== rawKey) ||
        (query.group_by === "game" && query.game_id !== undefined && rawKey.toLowerCase() !== query.game_id.toLowerCase()) ||
        (query.group_by === "member" && query.member_id !== undefined && rawKey.toLowerCase() !== query.member_id.toLowerCase()) ||
        (query.group_by === "entry_type" && (!/^[a-zA-Z0-9_.:-]{1,200}$/.test(rawKey) || record.label !== record.key))) fail();
    priorRawKey = rawKey;
    for (const field of totalFields) {
      if (!(field === "net_points" ? SIGNED_INTEGER_RE : INTEGER_RE).test(record[field]!)) fail();
      aggregate.set(field, aggregate.get(field)! + BigInt(record[field]!));
    }
    if (kind === "ledger" && BALANCE_FIELDS.some((field) => record[field] !== "")) fail();
  }
  if (metadata.some((record) => record.timezone !== stableTimezone)) fail();
  if (totalFields.some((field) => aggregate.get(field) !== BigInt(summary[field]!))) fail("导出分组汇总与总计不一致。");
  if (String(groupRows.length) !== groupCount) fail("导出分组数量与响应头不匹配。");
  if (metadata.slice(1, groupStart).some((r) => r.record_type !== "balances")) fail();
  return { groupCount, snapshotAt };
}

export function reportExportPermissions(account: AdminAccount, brandId: string): { betting: boolean; ledger: boolean } {
  const scoped = UUID_RE.test(account.id) && UUID_RE.test(brandId);
  const brandGrants = brandPermissionSet(account, brandId.toLowerCase());
  const allowed = (kind: ExportKind) => {
    const base = `report_${kind}`;
    return scoped && brandGrants.has(`${base}.view.brand`) && brandGrants.has(`${base}.export.brand`);
  };
  return { betting: allowed("betting"), ledger: allowed("ledger") };
}

export function createReportExportApi(fetchImpl: typeof fetch = fetch) {
  async function request(kind: ExportKind, brandId: string, input: ExportQuery): Promise<ReportExport> {
    if (!UUID_RE.test(brandId)) badInput("brand 必须是 UUID。");
    const query = validateQuery(kind, input);
    const params = new URLSearchParams({ from: query.from, to: query.to, group_by: query.group_by });
    if (query.game_id) params.set("game_id", query.game_id);
    if (query.member_id) params.set("member_id", query.member_id);
    const response = await fetchImpl(`${BASE}/${kind}/export?${params}`, { method: "GET", credentials: "same-origin", headers: { Accept: "text/csv", "X-Brand-ID": brandId } });
    if (!response.ok) return parseError(response);
    const contentLength = response.headers.get("Content-Length");
    if (!contentLength || !/^(0|[1-9]\d*)$/.test(contentLength) || Number(contentLength) > MAX_BYTES) fail("导出文件大小无效。");
    const bytes = new Uint8Array(await response.arrayBuffer());
    if (bytes.length !== Number(contentLength) || bytes.length > MAX_BYTES) fail("导出文件长度不匹配。");
    const digest = [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))].map((n) => n.toString(16).padStart(2, "0")).join("");
    if (digest !== response.headers.get("X-Report-SHA256")) fail("导出文件校验失败。");
    if (bytes[0] !== 0xef || bytes[1] !== 0xbb || bytes[2] !== 0xbf) fail("导出 CSV 缺少 UTF-8 BOM。");
    let decoded: string;
    try { decoded = new TextDecoder("utf-8", { fatal: true }).decode(bytes.subarray(3)); } catch { return fail("导出文件不是有效 UTF-8。"); }
    const validated = validateCsv(decoded, kind, brandId, query, response.headers);
    const filename = `lottery-${kind}-${brandId}-${filenameDate(validated.snapshotAt)}.csv`;
    return { filename, bytes, groupCount: validated.groupCount, snapshotAt: validated.snapshotAt, auditLogId: response.headers.get("X-Report-Audit-ID")! };
  }
  return {
    betting(brandId: string, query: ExportQuery) { return request("betting", brandId, query); },
    ledger(brandId: string, query: ExportQuery) { return request("ledger", brandId, query); },
  };
}
