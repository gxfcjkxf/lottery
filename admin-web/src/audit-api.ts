import { AdminApiError, type AdminAccount } from "./admin-api";

const BASE = "/api/v1/admin/audit";
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const MAX_WINDOW_NS = 31n * 24n * 60n * 60n * 1_000_000_000n;
const MAX_EXPORT_ROWS = 10_000;
const MAX_EXPORT_BYTES = 4 * 1024 * 1024;
const CSV_COLUMNS = ["id", "brand_id", "action", "actor_type", "actor_id", "resource_type", "resource_id", "reason", "request_id", "created_at", "ip_address", "before_json", "after_json"] as const;

export interface AuditFilters {
  from: string;
  to: string;
  action?: string;
  actor_id?: string;
  resource_type?: string;
  resource_id?: string;
  request_id?: string;
}

export interface AuditQuery extends Omit<Partial<AuditFilters>, "from" | "to"> {
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
}

export interface AdminAuditRecord {
  id: string;
  brand_id?: string | null;
  action: string;
  actor_type: string;
  actor_id: string;
  resource_type: string;
  resource_id: string;
  reason: string;
  request_id: string;
  created_at: string;
  ip_address: string;
  before_json: unknown;
  after_json: unknown;
}

export interface AuditExport {
  brandId: string;
  exportId: string;
  filename: string;
  snapshotAt: string;
  rowCount: number;
  sha256: string;
  bytes: Uint8Array;
}

function fail(message: string, status = 502, code = "INVALID_RESPONSE"): never {
  throw new AdminApiError(message, status, code);
}
function badInput(message: string): never { return fail(message, 0, "INVALID_INPUT"); }
function isRecord(value: unknown): value is Record<string, unknown> { return value !== null && typeof value === "object" && !Array.isArray(value); }
function validUuid(value: unknown): value is string { return typeof value === "string" && UUID_RE.test(value); }

function validUtcDateTime(value: unknown): value is string {
  if (typeof value !== "string" || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value)) return false;
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?Z$/.exec(value)!;
  const [, y, mo, d, h, mi, s] = match;
  const year = Number(y), month = Number(mo), day = Number(d);
  const days = month === 2 ? (year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28) : ([4, 6, 9, 11].includes(month) ? 30 : 31);
  if (year < 1 || month < 1 || month > 12 || day < 1 || day > days || Number(h) > 23 || Number(mi) > 59 || Number(s) > 59) return false;
  return Number.isFinite(Date.parse(value));
}
function epochNs(value: string): bigint {
  const fraction = /\.(\d{1,9})Z$/.exec(value)?.[1] ?? "";
  const millis = Date.parse(value.replace(/\.\d+Z$/, "Z"));
  return BigInt(millis) * 1_000_000n + BigInt(fraction.padEnd(9, "0") || "0");
}
function normalizeQuery(input: AuditQuery, exportMode = false) {
  if (!isRecord(input)) badInput("审计查询无效。 / Invalid audit query.");
  const allowed = new Set(["from", "to", "action", "actor_id", "resource_type", "resource_id", "request_id", ...(exportMode ? [] : ["limit", "offset"])]);
  if (Object.keys(input).some((key) => !allowed.has(key))) badInput("审计查询包含不支持的参数。 / Unsupported audit query parameter.");
  const hasFrom = input.from !== undefined, hasTo = input.to !== undefined;
  if (hasFrom !== hasTo || exportMode && !hasFrom) badInput("开始和结束时间必须同时提供。 / Both start and end are required together.");
  const normalized: Record<string, string | number> = {};
  if (hasFrom) {
    if (!validUtcDateTime(input.from) || !validUtcDateTime(input.to)) badInput("开始和结束时间必须是 UTC ISO 日期。 / Start and end must be UTC ISO dates.");
    const from = epochNs(input.from!), to = epochNs(input.to!);
    if (to <= from || to - from > MAX_WINDOW_NS) badInput("日期范围必须大于零且不超过 31 天。 / Date range must be positive and no longer than 31 days.");
    normalized.from = input.from!;
    normalized.to = input.to!;
  }
  for (const key of ["action", "actor_id", "resource_type", "resource_id", "request_id"] as const) {
    const value = input[key];
    if (value === undefined) continue;
    if (typeof value !== "string" || value.length === 0 || value.trim() !== value || /\p{Cc}/u.test(value) || (["action", "resource_type", "request_id"].includes(key) && new TextEncoder().encode(value).length > 128)) badInput(`${key} 不能为空且长度或格式无效。 / ${key} must be non-empty and within its byte limit.`);
    if ((key === "actor_id" || key === "resource_id") && !validUuid(value)) badInput(`${key} 必须是 UUID。 / ${key} must be a UUID.`);
    normalized[key] = value;
  }
  if (!exportMode) {
    const limit = input.limit ?? 100, offset = input.offset ?? 0;
    if (!Number.isInteger(limit) || Number(limit) < 1 || Number(limit) > 200 || !Number.isInteger(offset) || Number(offset) < 0 || Number(offset) > 100_000) badInput("分页参数无效。 / Invalid pagination.");
    normalized.limit = Number(limit);
    normalized.offset = Number(offset);
  }
  return normalized;
}

function readEnvelope<T>(raw: unknown, status: number): T {
  if (!isRecord(raw) || raw.success !== true || raw.data === undefined) {
    const error = isRecord(raw) && isRecord(raw.error) ? raw.error : null;
    throw new AdminApiError(typeof error?.message === "string" ? error.message : `Request failed (${status})`, status, typeof error?.code === "string" ? error.code : undefined);
  }
  return raw.data as T;
}
async function responseError(response: Response): Promise<never> {
  try {
    const raw: unknown = await response.json();
    if (isRecord(raw) && isRecord(raw.error)) throw new AdminApiError(typeof raw.error.message === "string" ? raw.error.message : `Request failed (${response.status})`, response.status, typeof raw.error.code === "string" ? raw.error.code : undefined);
    if (isRecord(raw) && typeof raw.error === "string") throw new AdminApiError(raw.error, response.status);
  } catch (cause) { if (cause instanceof AdminApiError) throw cause; }
  throw new AdminApiError(`Request failed (${response.status})`, response.status);
}
function validateItems(value: unknown, brandId: string, limit: number): AdminAuditRecord[] {
  if (!isRecord(value) || !Array.isArray(value.items) || value.items.length > limit) fail("审计响应格式无效。 / Invalid audit response.");
  const fields = ["id", "action", "actor_type", "actor_id", "resource_type", "resource_id", "reason", "request_id", "created_at", "ip_address"] as const;
  for (const item of value.items) {
    if (!isRecord(item) || fields.some((key) => typeof item[key] !== "string") || !validUuid(item.id) || typeof item.brand_id === "string" && item.brand_id.toLowerCase() !== brandId.toLowerCase() || item.brand_id !== undefined && item.brand_id !== null && typeof item.brand_id !== "string" || item.before_json === undefined || item.after_json === undefined || !validUtcDateTime(item.created_at)) fail("审计记录格式或品牌范围无效。 / Invalid audit record or brand scope.");
  }
  return value.items as AdminAuditRecord[];
}

function parseCsv(text: string): string[][] {
  const rows: string[][] = [];
  let row: string[] = [], field = "", quoted = false, afterQuote = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i]!;
    if (quoted) {
      if (c === '"') {
        if (text[i + 1] === '"') { field += '"'; i++; }
        else { quoted = false; afterQuote = true; }
      } else if (c === "\r" && text[i + 1] === "\n") { field += "\r\n"; i++; }
      else if (c === "\r" || c === "\n") fail("审计 CSV 行结束符无效。 / Invalid audit CSV line ending.");
      else field += c;
      continue;
    }
    if (afterQuote && c !== "," && c !== "\r") fail("审计 CSV 格式无效。 / Invalid audit CSV.");
    if (c === '"') { if (field || afterQuote) fail("审计 CSV 格式无效。 / Invalid audit CSV."); quoted = true; }
    else if (c === ",") { row.push(field); field = ""; afterQuote = false; }
    else if (c === "\r") { if (text[i + 1] !== "\n") fail("审计 CSV 格式无效。 / Invalid audit CSV."); row.push(field); rows.push(row); row = []; field = ""; afterQuote = false; i++; }
    else if (c === "\n") fail("审计 CSV 格式无效。 / Invalid audit CSV.");
    else { if (c.charCodeAt(0) < 0x20 && c !== "\t") fail("审计 CSV 格式无效。 / Invalid audit CSV."); field += c; }
  }
  if (quoted || row.length || field || !text.endsWith("\n")) fail("审计 CSV 格式无效。 / Invalid audit CSV.");
  return rows;
}
function formulaSafe(value: string): boolean {
  if (value.startsWith("'")) return true;
  return !/[\t\r\n]/.test(value) && !/^[\s\uFEFF]*[=+\-@]/.test(value);
}
function compareUuid(a: string, b: string): number { return a.toLowerCase() < b.toLowerCase() ? -1 : a.toLowerCase() > b.toLowerCase() ? 1 : 0; }
function validateCsv(text: string, brandId: string, query: Record<string, string | number>, rowCount: number): void {
  const rows = parseCsv(text);
  if (rows.length !== rowCount + 1 || rows[0]?.length !== CSV_COLUMNS.length || rows[0]?.some((column, i) => column !== CSV_COLUMNS[i])) fail("审计 CSV 行数或列格式无效。 / Invalid audit CSV columns or row count.");
  let previous: string[] | null = null;
  for (const row of rows.slice(1)) {
    if (row.length !== CSV_COLUMNS.length || row.some((value) => !formulaSafe(value))) fail("审计 CSV 包含无效或不安全字段。 / Audit CSV contains an invalid or unsafe field.");
    if (!validUuid(row[0]) || !validUuid(row[1]) || row[1]!.toLowerCase() !== brandId.toLowerCase() || !validUtcDateTime(row[9])) fail("审计 CSV 记录范围或格式无效。 / Invalid audit CSV record scope or format.");
    const at = epochNs(row[9]);
    if (query.from !== undefined && query.to !== undefined && (at < epochNs(String(query.from)) || at >= epochNs(String(query.to)))) fail("审计 CSV 记录超出请求时间范围。 / Audit CSV row is outside the requested interval.");
    if (previous) {
      const previousAt = epochNs(previous[9]!);
      if (at > previousAt || at === previousAt && compareUuid(row[0]!, previous[0]!) > 0) fail("审计 CSV 排序无效。 / Invalid audit CSV order.");
    }
    previous = row;
  }
}

export function auditPermissions(account: AdminAccount, brandId: string): { view: boolean; export: boolean } {
  const scoped = validUuid(account.id) && validUuid(brandId);
  const brandAllowed = scoped && (account.brand_ids ?? []).some((id) => id.toLowerCase() === brandId.toLowerCase());
  const brandGrants = new Set(account.permissions_by_brand?.[brandId] ?? []);
  const platformGrants = new Set(account.platform_permissions ?? []);
  const viewBrand = brandAllowed && brandGrants.has("audit.view.brand");
  const exportBrand = brandAllowed && brandGrants.has("audit.export.brand");
  const viewPlatform = platformGrants.has("audit.view.platform");
  const exportPlatform = platformGrants.has("audit.export.platform");
  const view = scoped && (viewBrand || viewPlatform);
  return { view, export: view && scoped && (exportBrand || exportPlatform) };
}

export function createAuditApi(fetchImpl: typeof fetch = fetch) {
  async function list(brandId: string, input: AuditQuery) {
    if (!validUuid(brandId)) badInput("品牌必须是 UUID。 / Brand must be a UUID.");
    const query = normalizeQuery(input);
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) params.set(key, String(value));
    const response = await fetchImpl(`${BASE}?${params.toString()}`, { method: "GET", credentials: "same-origin", headers: { Accept: "application/json", "X-Brand-ID": brandId } });
    if (!response.ok) return responseError(response);
    let raw: unknown;
    try { raw = await response.json(); } catch { fail("审计响应格式无效。 / Invalid audit response."); }
    return { items: validateItems(readEnvelope(raw, response.status), brandId, Number(query.limit)) };
  }
  async function exportCsv(brandId: string, input: AuditFilters): Promise<AuditExport> {
    if (!validUuid(brandId)) badInput("品牌必须是 UUID。 / Brand must be a UUID.");
    const query = normalizeQuery(input, true);
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) params.set(key, String(value));
    const response = await fetchImpl(`${BASE}/export?${params.toString()}`, { method: "GET", credentials: "same-origin", headers: { Accept: "text/csv", "X-Brand-ID": brandId } });
    if (!response.ok) return responseError(response);
    const header = (name: string) => response.headers.get(name);
    const exportId = header("X-Audit-Export-ID"), snapshotAt = header("X-Audit-Snapshot-At"), countText = header("X-Audit-Row-Count"), digestHeader = header("X-Audit-SHA256");
    const disposition = `attachment; filename="audit-${brandId}-v1.csv"`;
    if (!validUuid(exportId) || !validUtcDateTime(snapshotAt) || (header("Content-Type") ?? "").split(";", 1)[0]!.trim().toLowerCase() !== "text/csv" || header("X-Audit-Brand-ID")?.toLowerCase() !== brandId.toLowerCase() || header("X-Audit-Format-Version") !== "1" || header("Content-Disposition") !== disposition || !countText || !/^(0|[1-9]\d*)$/.test(countText) || Number(countText) > MAX_EXPORT_ROWS || !digestHeader || !/^[a-f0-9]{64}$/.test(digestHeader) || !/\bno-store\b/i.test(header("Cache-Control") ?? "") || header("X-Content-Type-Options")?.toLowerCase() !== "nosniff") fail("审计导出元数据无效。 / Invalid audit export metadata.");
    const lengthHeader = header("Content-Length");
    if (!lengthHeader || !/^(0|[1-9]\d*)$/.test(lengthHeader) || Number(lengthHeader) > MAX_EXPORT_BYTES) fail("审计导出长度无效。 / Invalid audit export length.");
    const bytes = new Uint8Array(await response.arrayBuffer());
    if (bytes.length !== Number(lengthHeader) || bytes.length > MAX_EXPORT_BYTES) fail("审计导出长度不匹配。 / Audit export length mismatch.");
    const digest = [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))].map((byte) => byte.toString(16).padStart(2, "0")).join("");
    if (digest !== digestHeader) fail("审计导出摘要校验失败。 / Audit export digest mismatch.");
    if (bytes[0] !== 0xef || bytes[1] !== 0xbb || bytes[2] !== 0xbf) fail("审计 CSV 缺少 UTF-8 BOM。 / Audit CSV is missing its UTF-8 BOM.");
    let csv: string;
    try { csv = new TextDecoder("utf-8", { fatal: true }).decode(bytes.subarray(3)); } catch { fail("审计 CSV 不是有效 UTF-8。 / Audit CSV is not valid UTF-8."); }
    validateCsv(csv, brandId, query, Number(countText));
    return { brandId, exportId, filename: `audit-${brandId}-v1.csv`, snapshotAt, rowCount: Number(countText), sha256: digest, bytes };
  }
  return { list, exportCsv };
}
