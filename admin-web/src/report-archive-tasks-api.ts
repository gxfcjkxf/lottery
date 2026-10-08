import { AdminApiError, type AdminAccount } from "./admin-api";

const BASE = "/api/v1/admin/report-archive-tasks";
const POLICY = "/api/v1/admin/report-archive-policy";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const MAX_SAFE = BigInt(Number.MAX_SAFE_INTEGER);

export interface ReportArchivePolicy {
  brand_id: string; version: number; daily_enabled: boolean; monthly_enabled: boolean;
  daily_start_period: string | null; monthly_start_period: string | null; timezone: string;
  audit_log_id: string | null; updated_at: string;
}
export interface ReportArchiveTaskWindow { kind: "daily" | "monthly"; period_key: string; timezone: string; from: string; to: string }
export interface ReportArchiveTask {
  id: string; brand_id: string; policy_version: number; window: ReportArchiveTaskWindow;
  state: "pending" | "completed" | "skipped" | "failed"; version: number; attempt_count: number;
  archive_id: string | null; last_error_code: string | null; creation_audit_log_id: string;
  last_audit_log_id: string; created_at: string; updated_at: string;
}
export interface ReportArchiveTaskPage { brand_id: string; items: ReportArchiveTask[]; total_count: string; limit: number; offset: number }
export interface ReportArchiveTaskRetryInput { version: number; reason: string }

function record(v: unknown): v is Record<string, unknown> { return v !== null && typeof v === "object" && !Array.isArray(v); }
function exact(v: Record<string, unknown>, fields: readonly string[]): boolean {
  const got = Object.keys(v).sort(), expected = [...fields].sort();
  return got.length === expected.length && got.every((key, i) => key === expected[i]);
}
function uuid(v: unknown): v is string { return typeof v === "string" && UUID.test(v); }
function version(v: unknown, allowZero = false): v is number {
  return typeof v === "number" && Number.isSafeInteger(v) && v >= (allowZero ? 0 : 1);
}
function invalidInput(): never { throw new AdminApiError("Report archive task request is invalid.", 0, "INVALID_INPUT"); }
function invalidResponse(): never { throw new AdminApiError("Report archive task response is invalid.", 502, "INVALID_RESPONSE"); }
function unknownWrite(): never { throw new AdminApiError("Report archive task write result is unknown; retry with the same idempotency key.", 0, "UNKNOWN_WRITE_STATUS"); }
function sameUuid(a: unknown, b: string): boolean { return typeof a === "string" && a.toLowerCase() === b.toLowerCase(); }

// Parse RFC3339 to nanoseconds without Date's millisecond truncation.
function instant(value: unknown): bigint | null {
  if (typeof value !== "string") return null;
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|([+-])(\d{2}):(\d{2}))$/.exec(value);
  if (!m) return null;
  const [, ys, mos, ds, hs, mis, ss, fraction = "", zone, sign, zhs, zms] = m;
  const y = Number(ys), mo = Number(mos), d = Number(ds), h = Number(hs), mi = Number(mis), s = Number(ss);
  const leap = y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0);
  const daysInMonth = mo === 2 ? leap ? 29 : 28 : [4, 6, 9, 11].includes(mo) ? 30 : 31;
  if (y < 1 || mo < 1 || mo > 12 || d < 1 || d > daysInMonth || h > 23 || mi > 59 || s > 59) return null;
  let offset = 0;
  if (zone !== "Z") {
    const oh = Number(zhs), om = Number(zms);
    if (oh > 14 || om > 59 || oh === 14 && om !== 0) return null;
    offset = (oh * 60 + om) * (sign === "+" ? 1 : -1);
  }
  const yy = BigInt(y - (mo <= 2 ? 1 : 0)), era = yy / 400n, yoe = yy - era * 400n;
  const m2 = BigInt(mo + (mo > 2 ? -3 : 9));
  const doy = (153n * m2 + 2n) / 5n + BigInt(d - 1);
  const days = era * 146097n + yoe * 365n + yoe / 4n - yoe / 100n + doy - 719468n;
  const seconds = ((days * 24n + BigInt(h)) * 60n + BigInt(mi - offset)) * 60n + BigInt(s);
  const ns = seconds * 1_000_000_000n + BigInt(fraction.padEnd(9, "0") || "0");
  return ns < -62_135_596_800_000_000_000n || ns >= 253_402_300_800_000_000_000n ? null : ns;
}
function validDate(v: unknown): v is string { return instant(v) !== null; }
function validTimezone(v: unknown): v is string {
  if (typeof v !== "string" || !v || v === "Local" || v.length > 100) return false;
  try { new Intl.DateTimeFormat("en", { timeZone: v }); return true; } catch { return false; }
}
function canonicalPeriod(v: unknown, kind: "daily" | "monthly"): v is string {
  if (typeof v !== "string") return false;
  const m = (kind === "daily" ? /^(\d{4})-(\d{2})-(\d{2})$/ : /^(\d{4})-(\d{2})$/).exec(v);
  if (!m) return false;
  const y = Number(m[1]), month = Number(m[2]), day = kind === "daily" ? Number(m[3]) : 1;
  const leap = y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0);
  return y >= 1 && y <= 9998 && month >= 1 && month <= 12 && day >= 1 && day <= (month === 2 ? leap ? 29 : 28 : [4, 6, 9, 11].includes(month) ? 30 : 31);
}
function validWindow(v: unknown): v is ReportArchiveTaskWindow {
  if (!record(v) || !exact(v, ["kind", "period_key", "timezone", "from", "to"]) || (v.kind !== "daily" && v.kind !== "monthly") || !validTimezone(v.timezone) || !validDate(v.from) || !validDate(v.to)) return false;
  if (typeof v.period_key !== "string") return false;
  const match = v.kind === "daily" ? /^(\d{4})-(\d{2})-(\d{2})$/.exec(v.period_key) : /^(\d{4})-(\d{2})$/.exec(v.period_key);
  if (!match) return false;
  const y = Number(match[1]), m = Number(match[2]), d = v.kind === "daily" ? Number(match[3]) : 1;
  if (y < 1 || y > 9998 || m < 1 || m > 12 || d < 1 || d > (m === 2 ? (y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0) ? 29 : 28) : [4, 6, 9, 11].includes(m) ? 30 : 31)) return false;
  // Keep the period key canonical while treating its persisted timestamps as frozen instants.
  const expectedKey = `${String(y).padStart(4, "0")}-${String(m).padStart(2, "0")}${v.kind === "daily" ? `-${String(d).padStart(2, "0")}` : ""}`;
  if (v.period_key !== expectedKey) return false;
  const from = instant(v.from), to = instant(v.to);
  return from !== null && to !== null && from < to;
}
const policyFields = ["brand_id", "version", "daily_enabled", "monthly_enabled", "daily_start_period", "monthly_start_period", "timezone", "audit_log_id", "updated_at"] as const;
function validPolicy(v: unknown, brand: string): v is ReportArchivePolicy {
  return record(v) && exact(v, policyFields) && sameUuid(v.brand_id, brand) && version(v.version) && typeof v.daily_enabled === "boolean" && typeof v.monthly_enabled === "boolean" &&
    (v.daily_start_period === null || canonicalPeriod(v.daily_start_period, "daily")) &&
    (v.monthly_start_period === null || canonicalPeriod(v.monthly_start_period, "monthly")) &&
    (!v.daily_enabled || v.daily_start_period !== null) && (!v.monthly_enabled || v.monthly_start_period !== null) && validTimezone(v.timezone) &&
    (v.audit_log_id === null || uuid(v.audit_log_id)) && validDate(v.updated_at) &&
    (v.version === 1
      ? v.audit_log_id === null && !v.daily_enabled && !v.monthly_enabled && v.daily_start_period === null && v.monthly_start_period === null
      : v.audit_log_id !== null);
}
const taskFields = ["id", "brand_id", "policy_version", "window", "state", "version", "attempt_count", "archive_id", "last_error_code", "creation_audit_log_id", "last_audit_log_id", "created_at", "updated_at"] as const;
function validTask(v: unknown, brand: string): v is ReportArchiveTask {
  if (!record(v) || !exact(v, taskFields) || !uuid(v.id) || !sameUuid(v.brand_id, brand) || !version(v.policy_version) || !validWindow(v.window) ||
    !version(v.version) || !version(v.attempt_count, true) || !uuid(v.creation_audit_log_id) || !uuid(v.last_audit_log_id) || !validDate(v.created_at) || !validDate(v.updated_at) ||
    instant(v.updated_at)! < instant(v.created_at)!) return false;
  if (instant((v.window as ReportArchiveTaskWindow).to)! > instant(v.created_at)!) return false;
  const { state, attempt_count: attempts, archive_id: archive, last_error_code: error } = v;
  const attemptBig = BigInt(attempts), versionBig = BigInt(v.version);
  if (state === "pending") return archive === null && error === null && (attempts === 0 ? versionBig === 1n : versionBig === attemptBig * 2n + 1n);
  if (state === "completed") return attempts >= 1 && versionBig === attemptBig * 2n && uuid(archive) && error === null;
  if (state === "skipped") return attempts >= 1 && versionBig === attemptBig * 2n && uuid(archive) && error === null;
  if (state === "failed") return attempts >= 1 && versionBig === attemptBig * 2n && archive === null && error === "ARCHIVE_FAILED";
  return false;
}
function validPage(v: unknown, brand: string, limit: number, offset: number): v is ReportArchiveTaskPage {
  if (!record(v) || !exact(v, ["brand_id", "items", "total_count", "limit", "offset"]) || !sameUuid(v.brand_id, brand) || !Array.isArray(v.items) ||
    typeof v.total_count !== "string" || !/^(0|[1-9][0-9]*)$/.test(v.total_count) || !Number.isSafeInteger(v.limit) || v.limit !== limit || !Number.isSafeInteger(v.offset) || v.offset !== offset || v.items.length > limit) return false;
  const remaining = BigInt(v.total_count) - BigInt(offset);
  const expectedItems = remaining <= 0n ? 0n : remaining < BigInt(limit) ? remaining : BigInt(limit);
  if (BigInt(v.items.length) !== expectedItems) return false;
  if (!v.items.every(item => validTask(item, brand))) return false;
  const ids = new Set<string>();
  for (const item of v.items) {
    const normalizedId = item.id.toLowerCase();
    if (ids.has(normalizedId)) return false;
    ids.add(normalizedId);
  }
  for (let i = 1; i < v.items.length; i++) {
    const a = v.items[i - 1]!, b = v.items[i]!;
    if (instant(a.created_at)! < instant(b.created_at)! || instant(a.created_at) === instant(b.created_at) && a.id.toLowerCase() <= b.id.toLowerCase()) return false;
  }
  return true;
}
function validateBrand(brand: string): void { if (!uuid(brand)) invalidInput(); }
function validatePage(limit: number, offset: number): void { if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000) invalidInput(); }
function validateRetry(input: ReportArchiveTaskRetryInput): void {
  if (!record(input) || !exact(input, ["version", "reason"]) || !version(input.version) || BigInt(input.version) >= MAX_SAFE || typeof input.reason !== "string") invalidInput();
  const reason = input.reason;
  if (reason.length === 0 || reason.trim() !== reason || /[\u0000-\u001f\u007f-\u009f]/u.test(reason) || /[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/u.test(reason) || new TextEncoder().encode(reason).byteLength > 500) invalidInput();
}
function validateKey(key: string): void { if (typeof key !== "string" || !/^[A-Za-z0-9_:.-]{8,128}$/.test(key)) invalidInput(); }

export function reportArchiveTasksPermissions(account: AdminAccount, brand: string): { view: boolean; retry: boolean } {
  const member = account.brand_ids.includes(brand);
  const brandPermissions = new Set(account.permissions_by_brand === undefined ? member ? account.permissions ?? [] : [] : account.permissions_by_brand[brand] ?? []);
  const platformPermissions = new Set(account.platform_permissions ?? account.permissions ?? []);
  const view = uuid(brand) && (platformPermissions.has("report_archive.view.platform") || member && brandPermissions.has("report_archive.view.brand"));
  return { view, retry: Boolean(uuid(brand) && member && !account.super_admin && brandPermissions.has("report_archive.view.brand") && brandPermissions.has("report_archive_task.retry.brand")) };
}

export function createReportArchiveTasksApi(fetcher: typeof fetch = fetch) {
  async function request<T>(brand: string, path: string, valid: (data: unknown) => data is T, write = false, headers?: HeadersInit, body?: unknown): Promise<T> {
    const requestHeaders = new Headers({ Accept: "application/json", "X-Brand-ID": brand });
    new Headers(headers).forEach((value, name) => requestHeaders.set(name, value));
    if (body !== undefined) requestHeaders.set("Content-Type", "application/json");
    let response: Response;
    try { response = await fetcher(path, { method: body === undefined ? "GET" : "POST", credentials: "same-origin", headers: requestHeaders, ...(body === undefined ? {} : { body: JSON.stringify(body) }) }); }
    catch { if (write) unknownWrite(); throw new AdminApiError("Network request failed.", 0, "NETWORK_ERROR"); }
    if (write && (response.status < 200 || response.status >= 300) && (response.status < 400 || response.status >= 500)) unknownWrite();
    let envelope: unknown;
    try { envelope = await response.json(); } catch { if (write) unknownWrite(); invalidResponse(); }
    if (!response.ok) {
      const detail = record(envelope) && record(envelope.error) ? envelope.error : undefined;
      throw new AdminApiError(typeof detail?.message === "string" ? detail.message : `Request failed (${response.status})`, response.status, typeof detail?.code === "string" ? detail.code : undefined);
    }
    if (response.status !== 200 || !record(envelope) || envelope.success !== true || envelope.data === undefined || !valid(envelope.data)) { if (write) unknownWrite(); invalidResponse(); }
    return envelope.data;
  }
  return {
    policy(brand: string) { validateBrand(brand); return request(brand, POLICY, (data): data is ReportArchivePolicy => validPolicy(data, brand)); },
    list(brand: string, limit = 20, offset = 0) {
      validateBrand(brand); validatePage(limit, offset);
      return request(brand, `${BASE}?${new URLSearchParams({ limit: String(limit), offset: String(offset) })}`, (data): data is ReportArchiveTaskPage => validPage(data, brand, limit, offset));
    },
    read(brand: string, id: string) {
      validateBrand(brand); if (!uuid(id)) invalidInput();
      return request(brand, `${BASE}/${id}`, (data): data is ReportArchiveTask => validTask(data, brand) && data.id.toLowerCase() === id.toLowerCase());
    },
    retry(brand: string, id: string, input: ReportArchiveTaskRetryInput, key: string, expectedActor: string) {
      validateBrand(brand); if (!uuid(id) || !uuid(expectedActor)) invalidInput(); validateRetry(input); validateKey(key);
      const headers = new Headers({ "Idempotency-Key": key, "X-Report-Archive-Actor-ID": expectedActor });
      return request(brand, `${BASE}/${id}/retry`, (data): data is ReportArchiveTask => validTask(data, brand) && data.id.toLowerCase() === id.toLowerCase() && data.version === input.version + 1 && data.state === "pending" && data.archive_id === null && data.last_error_code === null, true, headers, input);
    },
  };
}
