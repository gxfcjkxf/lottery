import { AdminApiError, type AdminAccount } from "./admin-api";

const BASE = "/api/v1/admin/brands";
const CODE_RE = /^[a-z][a-z0-9_]{0,47}$/;
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const KEY_RE = /^[A-Za-z0-9_:.-]{8,128}$/;
const LOCALES = ["en", "zh-CN"] as const;
const CONTROL_OR_LINE_SEPARATOR_RE = /[\p{Cc}\u2028\u2029]/u;

export type BrandCreationLocale = typeof LOCALES[number];
export interface BrandCreationBody {
  code: string;
  name: string;
  default_locale: BrandCreationLocale;
  timezone: string;
  reason: string;
}
export interface CreatedBrandReceipt {
  id: string;
  code: string;
  name: string;
  status: "paused";
  default_locale: BrandCreationLocale;
  timezone: string;
  version: 1;
  created_at: string;
  audit_log_id: string;
}

export function brandCreationPermission(account: AdminAccount): boolean {
  return (account.platform_permissions ?? []).includes("brand.create.platform");
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}
function exactKeys(value: Record<string, unknown>, keys: string[]): boolean {
  const actual = Object.keys(value).sort(), expected = [...keys].sort();
  return actual.length === expected.length && actual.every((key, index) => key === expected[index]);
}
function byteLength(value: string): number { return new TextEncoder().encode(value).length; }
function validTimezone(value: string): boolean {
  if (!value || value === "Local" || byteLength(value) > 80 || (value !== "UTC" && !value.includes("/"))) return false;
  try { new Intl.DateTimeFormat("en", { timeZone: value }); return true; }
  catch { return false; }
}
function validDateTime(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|([+-])(\d{2}):(\d{2}))$/.exec(value);
  if (!match) return false;
  const [, y, mo, d, h, mi, s, , , , oh, om] = match;
  const year = Number(y), month = Number(mo), day = Number(d);
  const days = month === 2 ? (year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28) : [4, 6, 9, 11].includes(month) ? 30 : 31;
  return year >= 1 && month >= 1 && month <= 12 && day >= 1 && day <= days && Number(h) <= 23 && Number(mi) <= 59 && Number(s) <= 59 &&
    (oh === undefined || (Number(oh) <= 14 && Number(om) <= 59 && (Number(oh) !== 14 || Number(om) === 0))) && Number.isFinite(Date.parse(value));
}
function invalidInput(): never { throw new AdminApiError("品牌创建参数无效。", 400, "BRAND_CREATE_INPUT_INVALID"); }
function invalidReceipt(): never { throw new AdminApiError("品牌创建回执格式无效或与提交内容不匹配。", 0, "INVALID_RESPONSE"); }

export function validateBrandCreationBody(body: BrandCreationBody): void {
  if (!isRecord(body) || !exactKeys(body, ["code", "name", "default_locale", "timezone", "reason"]) ||
    typeof body.code !== "string" || !CODE_RE.test(body.code) || typeof body.name !== "string" || !body.name || body.name.trim() !== body.name || CONTROL_OR_LINE_SEPARATOR_RE.test(body.name) || byteLength(body.name) > 120 ||
    !(LOCALES as readonly unknown[]).includes(body.default_locale) || typeof body.timezone !== "string" || !validTimezone(body.timezone) ||
    typeof body.reason !== "string" || !body.reason || body.reason.trim() !== body.reason || CONTROL_OR_LINE_SEPARATOR_RE.test(body.reason) || byteLength(body.reason) > 500) invalidInput();
}

export function createBrandCreationApi(fetchImpl: typeof fetch = fetch) {
  return {
    async create(body: BrandCreationBody, idempotencyKey: string): Promise<CreatedBrandReceipt> {
      validateBrandCreationBody(body);
      if (typeof idempotencyKey !== "string" || !KEY_RE.test(idempotencyKey)) invalidInput();
      let response: Response;
      try {
        response = await fetchImpl(BASE, { method: "POST", credentials: "same-origin", headers: {
          Accept: "application/json", "Content-Type": "application/json", "Idempotency-Key": idempotencyKey,
        }, body: JSON.stringify({ code: body.code, name: body.name, default_locale: body.default_locale, timezone: body.timezone, reason: body.reason }) });
      } catch (cause) {
        throw new AdminApiError(cause instanceof Error ? cause.message : "Network request failed", 0, "NETWORK_ERROR");
      }
      let envelope: unknown;
      try { envelope = await response.json(); }
      catch {
        if (!response.ok) throw new AdminApiError(`Request failed (${response.status})`, response.status);
        invalidReceipt();
      }
      if (!response.ok || !isRecord(envelope) || envelope.success !== true || envelope.data === undefined) {
        const error = isRecord(envelope) && isRecord(envelope.error) ? envelope.error : undefined;
        const message = isRecord(envelope) && typeof envelope.error === "string" ? envelope.error :
          typeof error?.message === "string" ? error.message : `Request failed (${response.status})`;
        if (!response.ok) throw new AdminApiError(message, response.status, typeof error?.code === "string" ? error.code : undefined);
        invalidReceipt();
      }
      const data = envelope.data;
      if (response.status !== 201 || !isRecord(data) || !exactKeys(data, ["id", "code", "name", "status", "default_locale", "timezone", "version", "created_at", "audit_log_id"]) ||
        typeof data.id !== "string" || !UUID_RE.test(data.id) || data.code !== body.code || data.name !== body.name || data.status !== "paused" ||
        data.default_locale !== body.default_locale || data.timezone !== body.timezone || data.version !== 1 || !validDateTime(data.created_at) ||
        typeof data.audit_log_id !== "string" || !UUID_RE.test(data.audit_log_id)) invalidReceipt();
      return data as unknown as CreatedBrandReceipt;
    },
  };
}
