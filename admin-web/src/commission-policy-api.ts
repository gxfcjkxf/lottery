import { AdminApiError, type AdminAccount } from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";

const BASE = "/api/v1/admin/commission-policy";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const DATE_TIME = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/;

export type CommissionCycle = "weekly" | "monthly";
export type CommissionShortMonth = "" | "last_day" | "skip";
export type CommissionPayoutMode = "manual" | "automatic";

export interface CommissionCalendar {
  timezone: string;
  cycle: CommissionCycle;
  boundary_time: string;
  weekday: number | null;
  month_day: number | null;
  short_month: CommissionShortMonth;
}

export interface CommissionPolicyConfig {
  enabled: boolean;
  calendar: CommissionCalendar | null;
  payout_mode: CommissionPayoutMode;
}

export interface CommissionPolicy {
  brand_id: string;
  version: number;
  config: CommissionPolicyConfig;
  created_at: string;
  updated_at: string;
  revision_id: string;
  audit_log_id?: string;
}

export interface CommissionPolicyRevision {
  id: string;
  brand_id: string;
  version: number;
  config: CommissionPolicyConfig;
  changed_by: string | null;
  reason: string;
  audit_log_id: string | null;
  created_at: string;
}

export interface UpdateCommissionPolicyBody {
  version: number;
  config: CommissionPolicyConfig;
  reason: string;
}

export interface CommissionPolicyHistory {
  items: CommissionPolicyRevision[];
  limit: number;
  offset: number;
}

export function commissionPolicyPermissions(
  account: AdminAccount,
  brand: string,
): { view: boolean; write: boolean } {
  const brandPermissions = brandPermissionSet(account, brand);
  const view = Boolean(brand) && brandPermissions.has("commission_policy.view.brand");
  return {
    view,
    write:
      Boolean(brand) &&
      !account.super_admin && account.brand_ids.includes(brand) &&
      brandPermissions.has("commission_policy.write.brand"),
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function hasOnlyKeys(value: Record<string, unknown>, keys: readonly string[]): boolean {
  return Object.keys(value).every((key) => keys.includes(key));
}

export function isCanonicalUuid(value: unknown): value is string {
  return typeof value === "string" && UUID.test(value);
}

function isVersion(value: unknown): value is number {
  return Number.isSafeInteger(value) && Number(value) >= 1;
}

function isDateTime(value: unknown): value is string {
  if (typeof value !== "string" || !DATE_TIME.test(value) || !Number.isFinite(Date.parse(value))) return false;
  const [date, rawTime] = value.split("T");
  const [year, month, day] = date.split("-").map(Number);
  const time = rawTime.split(/[.+Z-]/)[0];
  const [hour, minute, second] = time.split(":").map(Number);
  const lastDay = new Date(0);
  lastDay.setUTCFullYear(year, month, 0);
  const dayCount = lastDay.getUTCDate();
  const offset = /([+-])(\d{2}):(\d{2})$/.exec(value);
  return year >= 1 && month >= 1 && month <= 12 && day >= 1 && day <= dayCount &&
    hour <= 23 && minute <= 59 && second <= 59 &&
    (!offset || (Number(offset[2]) < 14 || (Number(offset[2]) === 14 && Number(offset[3]) === 0)) && Number(offset[3]) <= 59);
}

function validCalendar(value: unknown): value is CommissionCalendar {
  if (!isRecord(value) || !hasOnlyKeys(value, ["timezone", "cycle", "boundary_time", "weekday", "month_day", "short_month"])) return false;
  if (typeof value.timezone !== "string" || !value.timezone.trim() || value.timezone === "Local") return false;
  try {
    new Intl.DateTimeFormat("en", { timeZone: value.timezone });
  } catch {
    return false;
  }
  if (typeof value.boundary_time !== "string" || !/^([01]\d|2[0-3]):[0-5]\d:[0-5]\d$/.test(value.boundary_time)) return false;
  if (value.cycle === "weekly") {
    return Number.isInteger(value.weekday) && Number(value.weekday) >= 0 && Number(value.weekday) <= 6 &&
      value.month_day === null && value.short_month === "";
  }
  if (value.cycle === "monthly") {
    return value.weekday === null && Number.isInteger(value.month_day) && Number(value.month_day) >= 1 && Number(value.month_day) <= 31 &&
      (value.short_month === "last_day" || value.short_month === "skip");
  }
  return false;
}

export function isValidCommissionPolicyConfig(value: unknown): value is CommissionPolicyConfig {
  return isRecord(value) && hasOnlyKeys(value, ["enabled", "calendar", "payout_mode"]) &&
    typeof value.enabled === "boolean" &&
    (value.calendar === null || validCalendar(value.calendar)) &&
    (!value.enabled || value.calendar !== null) &&
    (value.payout_mode === "manual" || value.payout_mode === "automatic");
}

function sameConfig(a: CommissionPolicyConfig, b: CommissionPolicyConfig): boolean {
  if (a.enabled !== b.enabled || a.payout_mode !== b.payout_mode || (a.calendar === null) !== (b.calendar === null)) return false;
  if (a.calendar === null || b.calendar === null) return a.calendar === b.calendar;
  return a.calendar.timezone === b.calendar.timezone && a.calendar.cycle === b.calendar.cycle &&
    a.calendar.boundary_time === b.calendar.boundary_time && a.calendar.weekday === b.calendar.weekday &&
    a.calendar.month_day === b.calendar.month_day && a.calendar.short_month === b.calendar.short_month;
}

function validPolicy(value: unknown, brand: string): value is CommissionPolicy {
  if (!isRecord(value) || !hasOnlyKeys(value, ["brand_id", "version", "config", "created_at", "updated_at", "revision_id", "audit_log_id"])) return false;
  return value.brand_id === brand && isCanonicalUuid(value.brand_id) && isVersion(value.version) &&
    isValidCommissionPolicyConfig(value.config) && isDateTime(value.created_at) && isDateTime(value.updated_at) &&
    isCanonicalUuid(value.revision_id) && Date.parse(value.updated_at) >= Date.parse(value.created_at) &&
    (value.audit_log_id === undefined
      ? value.version === 1
      : isCanonicalUuid(value.audit_log_id));
}

function validRevision(value: unknown, brand: string): value is CommissionPolicyRevision {
  return isRecord(value) && hasOnlyKeys(value, ["id", "brand_id", "version", "config", "changed_by", "reason", "audit_log_id", "created_at"]) &&
    isCanonicalUuid(value.id) && value.brand_id === brand && isCanonicalUuid(value.brand_id) && isVersion(value.version) &&
    isValidCommissionPolicyConfig(value.config) &&
    (value.version === 1
      ? value.changed_by === null && value.audit_log_id === null
      : isCanonicalUuid(value.changed_by) && isCanonicalUuid(value.audit_log_id)) &&
    typeof value.reason === "string" && value.reason.trim().length > 0 && new TextEncoder().encode(value.reason).length <= 500 &&
    isDateTime(value.created_at);
}

function invalidResponse(): never {
  throw new AdminApiError("佣金策略响应格式无效。", 502, "INVALID_RESPONSE");
}

function invalidInput(message: string): never {
  throw new AdminApiError(message, 0, "INVALID_INPUT");
}

function validateBrand(brand: string): void {
  if (!isCanonicalUuid(brand)) invalidInput("品牌编号必须为有效 UUID。");
}

function validateBody(body: UpdateCommissionPolicyBody): void {
  if (!isRecord(body) || !hasOnlyKeys(body, ["version", "config", "reason"]) || !isVersion(body.version) || body.version >= Number.MAX_SAFE_INTEGER || !isValidCommissionPolicyConfig(body.config) ||
    typeof body.reason !== "string" || !body.reason.trim() || new TextEncoder().encode(body.reason).length > 500 || body.reason.includes("\0")) {
    invalidInput("佣金策略输入无效。");
  }
}

type Envelope<T> = { success?: boolean; data?: T; error?: string | { code?: string; message?: string } | null };

export function createCommissionPolicyApi(fetcher: typeof fetch = fetch) {
  async function request<T>(brand: string, path: string, options: { method?: "GET" | "PUT"; body?: unknown; key?: string } = {}): Promise<T> {
    const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brand });
    if (options.body !== undefined) headers.set("Content-Type", "application/json");
    if (options.key !== undefined) headers.set("Idempotency-Key", options.key);
    let response: Response;
    try {
      response = await fetcher(path, {
        method: options.method ?? "GET",
        credentials: "same-origin",
        headers,
        ...(options.body === undefined ? {} : { body: JSON.stringify(options.body) }),
      });
    } catch (cause) {
      throw new AdminApiError(cause instanceof Error ? cause.message : "Network request failed", 0, "NETWORK_ERROR");
    }
    let envelope: Envelope<T>;
    try {
      envelope = await response.json() as Envelope<T>;
    } catch {
      throw new AdminApiError(response.ok ? "Invalid server response" : `Request failed (${response.status})`, response.ok ? 502 : response.status, response.ok ? "INVALID_RESPONSE" : undefined);
    }
    if (!response.ok || !isRecord(envelope) || envelope.success !== true || envelope.data === undefined) {
      const detail = isRecord(envelope?.error) ? envelope.error : undefined;
      throw new AdminApiError(
        typeof envelope?.error === "string" ? envelope.error :
          (typeof detail?.message === "string" && detail.message) || `Request failed (${response.status})`,
        response.ok ? 502 : response.status,
        typeof detail?.code === "string" ? detail.code : undefined,
      );
    }
    return envelope.data;
  }

  return {
    async getPolicy(brand: string): Promise<CommissionPolicy> {
      validateBrand(brand);
      const value = await request<unknown>(brand, BASE);
      if (!validPolicy(value, brand)) invalidResponse();
      return value;
    },
    async getHistory(brand: string, limit = 20, offset = 0): Promise<CommissionPolicyHistory> {
      validateBrand(brand);
      if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000) invalidInput("历史分页参数无效。");
      const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
      const value = await request<unknown>(brand, `${BASE}/history?${query}`);
      if (!isRecord(value) || !hasOnlyKeys(value, ["items", "limit", "offset"]) || value.limit !== limit || value.offset !== offset ||
        !Array.isArray(value.items) || value.items.length > limit || !value.items.every((item) => validRevision(item, brand)) ||
        new Set(value.items.map((item) => (item as CommissionPolicyRevision).id)).size !== value.items.length ||
        !value.items.every((item, index, items) => index === 0 || (item as CommissionPolicyRevision).version < (items[index - 1] as CommissionPolicyRevision).version)) invalidResponse();
      return value as unknown as CommissionPolicyHistory;
    },
    async updatePolicy(brand: string, body: Readonly<UpdateCommissionPolicyBody>, key: string): Promise<CommissionPolicy> {
      validateBrand(brand);
      validateBody(body as UpdateCommissionPolicyBody);
      if (!isCanonicalUuid(key)) invalidInput("幂等键必须为规范 UUID。");
      const value = await request<unknown>(brand, BASE, { method: "PUT", body, key });
      if (!validPolicy(value, brand) || value.version !== body.version + 1 || !sameConfig(value.config, body.config)) invalidResponse();
      return value;
    },
  };
}
