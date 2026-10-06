import { AdminApiError, type AdminAccount } from "./admin-api";

const BASE = "/api/v1/admin";
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const MAX_PAGE_SIZE = 100;
const MAX_OFFSET = 1_000_000;
const SAFE_CODE_RE = /^[A-Z][A-Z0-9_]{0,63}$/;

export type DeliveryStatus = "pending" | "sent" | "failed";

export interface Delivery {
  event_id: string;
  brand_id: string;
  status: DeliveryStatus;
  attempt_count: number;
  last_error: string | null;
  next_attempt_at: string;
  sent_at: string | null;
}

export interface NotificationDeliveryRetryBody {
  attempt_count: number;
  reason: string;
}

export interface NotificationDeliveryApi {
  list(brand: string, limit?: number, offset?: number): Promise<Delivery[]>;
  retry(brand: string, eventID: string, body: NotificationDeliveryRetryBody, key: string): Promise<Delivery>;
}

export function deliveryPermissions(
  account: AdminAccount,
  brand: string,
): { view: boolean; retry: boolean } {
  const brandPermissions = new Set(
    account.permissions_by_brand === undefined
      ? account.permissions ?? []
      : account.permissions_by_brand[brand] ?? [],
  );
  const platformPermissions = new Set(account.platform_permissions ?? []);
  const isMember = UUID_RE.test(account.id) && UUID_RE.test(brand) &&
    account.brand_ids.includes(brand);
  return {
    view: isMember && (brandPermissions.has("notification.view.brand") ||
      (account.super_admin && platformPermissions.has("notification.view.platform"))),
    retry: isMember && !account.super_admin && brandPermissions.has("notification.retry.brand"),
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}
function validUuid(value: unknown): value is string {
  return typeof value === "string" && UUID_RE.test(value);
}
function sameUuid(left: unknown, right: string): boolean {
  return typeof left === "string" && left === right;
}
function nonnegativeInt(value: unknown): value is number {
  return Number.isSafeInteger(value) && Number(value) >= 0;
}
function validIsoDateTime(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|([+-])(\d{2}):(\d{2}))$/.exec(value);
  if (!match) return false;
  const [, y, mo, d, h, mi, s, , , , oh, om] = match;
  const year = Number(y), month = Number(mo), day = Number(d);
  const hour = Number(h), minute = Number(mi), second = Number(s);
  const days = month === 2
    ? (year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28)
    : [4, 6, 9, 11].includes(month) ? 30 : 31;
  if (year < 1 || month < 1 || month > 12 || day < 1 || day > days || hour > 23 || minute > 59 || second > 59) return false;
  if (oh !== undefined && (Number(oh) > 14 || Number(om) > 59 || (Number(oh) === 14 && Number(om) !== 0))) return false;
  return Number.isFinite(Date.parse(value));
}
function invalidInput(message: string): never {
  throw new AdminApiError(message, 0, "INVALID_INPUT");
}
function invalidResponse(uncertain = false): never {
  throw new AdminApiError("通知投递响应格式无效。", uncertain ? 0 : 502, "INVALID_RESPONSE");
}
function validateDelivery(value: unknown, brand: string, expected?: { eventID?: string; attemptCount?: number; retryReceipt?: boolean }): value is Delivery {
  if (!isRecord(value) || !validUuid(value.event_id) || (expected?.eventID !== undefined && !sameUuid(value.event_id, expected.eventID)) ||
    !sameUuid(value.brand_id, brand) || !["pending", "sent", "failed"].includes(String(value.status)) ||
    !nonnegativeInt(value.attempt_count) || (expected?.attemptCount !== undefined && value.attempt_count !== expected.attemptCount) ||
    !(value.last_error === null || (typeof value.last_error === "string" && SAFE_CODE_RE.test(value.last_error))) ||
    !validIsoDateTime(value.next_attempt_at) || !(value.sent_at === null || validIsoDateTime(value.sent_at))) return false;
  if ((value.status === "sent") !== (value.sent_at !== null)) return false;
  if (value.status === "failed" && (value.attempt_count < 1 || value.last_error === null)) return false;
  if (expected?.retryReceipt && value.status !== "pending") return false;
  return true;
}

type Envelope = { success?: unknown; data?: unknown; error?: string | { code?: string; message?: string } | null };

export function createNotificationDeliveryApi(fetchImpl: typeof fetch = fetch): NotificationDeliveryApi {
  async function request(path: string, brand: string, options: {
    method?: "GET" | "POST";
    body?: unknown;
    key?: string;
    write?: boolean;
  } = {}): Promise<unknown> {
    if (!validUuid(brand)) invalidInput("品牌编号必须是 UUID。");
    const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brand });
    if (options.body !== undefined) headers.set("Content-Type", "application/json");
    if (options.key !== undefined) headers.set("Idempotency-Key", options.key);
    let response: Response;
    try {
      response = await fetchImpl(path, {
        method: options.method ?? "GET",
        credentials: "include",
        headers,
        ...(options.body === undefined ? {} : { body: JSON.stringify(options.body) }),
      });
    } catch (cause) {
      throw new AdminApiError(cause instanceof Error ? cause.message : "Network request failed", 0, "NETWORK_ERROR");
    }
    let envelope: Envelope;
    try {
      envelope = await response.json() as Envelope;
    } catch {
      throw new AdminApiError(response.ok ? "Invalid server response" : `Request failed (${response.status})`,
        response.ok ? (options.write ? 0 : 502) : response.status, response.ok ? "INVALID_RESPONSE" : undefined);
    }
    if (!response.ok || envelope.success !== true || envelope.data === undefined) {
      const error = typeof envelope.error === "object" && envelope.error ? envelope.error : undefined;
      throw new AdminApiError(typeof envelope.error === "string" ? envelope.error : error?.message || `Request failed (${response.status})`,
        response.ok ? (options.write ? 0 : 502) : response.status,
        response.ok ? "INVALID_RESPONSE" : error?.code);
    }
    return envelope.data;
  }

  return {
    async list(brand, limit = 20, offset = 0) {
      if (!Number.isSafeInteger(limit) || limit < 1 || limit > MAX_PAGE_SIZE ||
        !Number.isSafeInteger(offset) || offset < 0 || offset > MAX_OFFSET) {
        invalidInput("分页参数超出允许范围。");
      }
      const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
      const value = await request(`${BASE}/notification-deliveries?${query}`, brand);
      if (!isRecord(value) || !Array.isArray(value.items) || value.items.length > limit ||
        !value.items.every((item) => validateDelivery(item, brand))) invalidResponse();
      return value.items as Delivery[];
    },
    async retry(brand, eventID, body, key) {
      if (!validUuid(eventID)) invalidInput("事件编号必须是 UUID。");
      if (!Number.isSafeInteger(body?.attempt_count) || body.attempt_count < 1 ||
        typeof body.reason !== "string" || body.reason.trim().length === 0 || new TextEncoder().encode(body.reason).length > 500 ||
        typeof key !== "string" || key.length === 0) invalidInput("重试请求参数无效。");
      const value = await request(`${BASE}/notification-deliveries/${encodeURIComponent(eventID)}/retry`, brand,
        { method: "POST", body, key, write: true });
      if (!validateDelivery(value, brand, { eventID, attemptCount: body.attempt_count, retryReceipt: true })) invalidResponse(true);
      return value;
    },
  };
}
