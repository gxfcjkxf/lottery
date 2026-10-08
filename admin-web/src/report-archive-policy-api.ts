import { AdminApiError, type AdminAccount } from "./admin-api";
import { reportArchiveTasksPermissions, type ReportArchivePolicy } from "./report-archive-tasks-api";

const PATH = "/api/v1/admin/report-archive-policy";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const MAX_SAFE = Number.MAX_SAFE_INTEGER;

export interface UpdateReportArchivePolicyInput {
  version: number;
  daily_enabled: boolean;
  monthly_enabled: boolean;
  reason: string;
}

function record(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function exact(value: Record<string, unknown>, fields: readonly string[]): boolean {
  const keys = Object.keys(value).sort();
  const expected = [...fields].sort();
  return keys.length === expected.length && keys.every((key, index) => key === expected[index]);
}

function invalidInput(): never {
  throw new AdminApiError("Report archive policy input is invalid.", 0, "INVALID_INPUT");
}

function unknownWrite(): never {
  throw new AdminApiError("Report archive policy write result is unknown; retry with the same body and idempotency key.", 0, "UNKNOWN_WRITE_STATUS");
}

function validateBrand(brand: string): void {
  if (typeof brand !== "string" || !UUID.test(brand)) invalidInput();
}

function validateInput(input: UpdateReportArchivePolicyInput): void {
  if (!record(input) || !exact(input, ["version", "daily_enabled", "monthly_enabled", "reason"]) ||
    !Number.isSafeInteger(input.version) || Number(input.version) < 1 || Number(input.version) >= MAX_SAFE ||
    typeof input.daily_enabled !== "boolean" || typeof input.monthly_enabled !== "boolean" || typeof input.reason !== "string") invalidInput();
  const reason = input.reason;
  if (!reason || reason.trim() !== reason || /[\u0000-\u001f\u007f-\u009f]/u.test(reason) ||
    /[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/u.test(reason) ||
    new TextEncoder().encode(reason).byteLength > 500) invalidInput();
}

function validateKey(key: string): void {
  if (typeof key !== "string" || !/^[A-Za-z0-9_:.-]{8,128}$/.test(key)) invalidInput();
}

function validDate(value: unknown): value is string {
  if (typeof value !== "string" || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.test(value) || !Number.isFinite(Date.parse(value))) return false;
  const [date, rawTime] = value.split("T");
  const [year, month, day] = date!.split("-").map(Number);
  const [hour, minute, second] = rawTime!.split(/[.+Z-]/)[0]!.split(":").map(Number);
  const lastDay = new Date(0);
  lastDay.setUTCFullYear(year!, month!, 0);
  const offset = /([+-])(\d{2}):(\d{2})$/.exec(value);
  return year! >= 1 && month! >= 1 && month! <= 12 && day! >= 1 && day! <= lastDay.getUTCDate() &&
    hour! <= 23 && minute! <= 59 && second! <= 59 &&
    (!offset || (Number(offset[2]) < 14 || (Number(offset[2]) === 14 && Number(offset[3]) === 0)) && Number(offset[3]) <= 59);
}

function validPeriod(value: unknown, monthly: boolean): value is string {
  if (typeof value !== "string") return false;
  const match = (monthly ? /^(\d{4})-(\d{2})$/ : /^(\d{4})-(\d{2})-(\d{2})$/).exec(value);
  if (!match) return false;
  const year = Number(match[1]), month = Number(match[2]), day = monthly ? 1 : Number(match[3]);
  const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const days = month === 2 ? leap ? 29 : 28 : [4, 6, 9, 11].includes(month) ? 30 : 31;
  return year >= 1 && year <= 9998 && month >= 1 && month <= 12 && day >= 1 && day <= days;
}

function validPolicy(value: unknown, brand: string, input: UpdateReportArchivePolicyInput): value is ReportArchivePolicy {
  if (!record(value) || !exact(value, ["brand_id", "version", "daily_enabled", "monthly_enabled", "daily_start_period", "monthly_start_period", "timezone", "audit_log_id", "updated_at"])) return false;
  if (typeof value.brand_id !== "string" || value.brand_id.toLowerCase() !== brand.toLowerCase() || !UUID.test(value.brand_id) ||
    value.version !== input.version + 1 || !Number.isSafeInteger(value.version) ||
    value.daily_enabled !== input.daily_enabled || value.monthly_enabled !== input.monthly_enabled ||
    (value.daily_start_period !== null && !validPeriod(value.daily_start_period, false)) ||
    (value.monthly_start_period !== null && !validPeriod(value.monthly_start_period, true)) ||
    (value.daily_enabled && value.daily_start_period === null) || (value.monthly_enabled && value.monthly_start_period === null) ||
    typeof value.timezone !== "string" || !value.timezone || value.timezone === "Local" || value.timezone.length > 100 ||
    (value.audit_log_id !== null && (typeof value.audit_log_id !== "string" || !UUID.test(value.audit_log_id))) ||
    value.audit_log_id === null || !validDate(value.updated_at)) return false;
  if (input.version === 1 && (!input.daily_enabled && value.daily_start_period !== null || !input.monthly_enabled && value.monthly_start_period !== null)) return false;
  try { new Intl.DateTimeFormat("en", { timeZone: value.timezone }); } catch { return false; }
  return true;
}

export function reportArchivePolicyPermissions(account: AdminAccount, brand: string): { view: boolean; write: boolean } {
  const member = account.brand_ids.includes(brand);
  const permissions = new Set(account.permissions_by_brand === undefined
    ? member ? account.permissions ?? [] : []
    : account.permissions_by_brand[brand] ?? []);
  return {
    view: reportArchiveTasksPermissions(account, brand).view,
    write: Boolean(UUID.test(brand) && member && !account.super_admin && permissions.has("report_archive.view.brand") && permissions.has("report_archive_policy.write.brand")),
  };
}

export function createReportArchivePolicyApi(fetcher: typeof fetch = fetch) {
  return {
    async update(brand: string, input: UpdateReportArchivePolicyInput, key: string, actorID: string): Promise<ReportArchivePolicy> {
      validateBrand(brand);
      validateInput(input);
      validateKey(key);
      if (typeof actorID !== "string" || !UUID.test(actorID)) invalidInput();
      const snapshot: UpdateReportArchivePolicyInput = Object.freeze({
        version: input.version,
        daily_enabled: input.daily_enabled,
        monthly_enabled: input.monthly_enabled,
        reason: input.reason,
      });
      const headers = new Headers({
        Accept: "application/json",
        "Content-Type": "application/json",
        "X-Brand-ID": brand,
        "Idempotency-Key": key,
        "X-Report-Archive-Actor-ID": actorID,
      });
      let response: Response;
      try {
        response = await fetcher(PATH, { method: "PUT", credentials: "same-origin", headers, body: JSON.stringify(snapshot) });
      } catch {
        unknownWrite();
      }
      if ((response.status < 200 || response.status >= 300) && (response.status < 400 || response.status >= 500)) unknownWrite();
      let envelope: unknown;
      try { envelope = await response.json(); } catch { unknownWrite(); }
      if (!response.ok) {
        const detail = record(envelope) && record(envelope.error) ? envelope.error : undefined;
        throw new AdminApiError(typeof detail?.message === "string" ? detail.message : `Request failed (${response.status})`, response.status, typeof detail?.code === "string" ? detail.code : undefined);
      }
      if (response.status !== 200 || !record(envelope) || envelope.success !== true || envelope.data === undefined || !validPolicy(envelope.data, brand, snapshot)) unknownWrite();
      return envelope.data;
    },
  };
}
