import { AdminApiError, type AdminAccount } from "./admin-api";

const BASE = "/api/v1/admin/notification-templates";
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const MAX_SAFE_VERSION = Number.MAX_SAFE_INTEGER;
const KEYS = [
  "member.joined", "recharge.confirmed", "bet.order.placed", "bet.order.cancelled",
  "bet.order.judged_cancelled", "bet.order.abnormal", "bet.order.won", "bet.order.prize_reversed",
  "reward.order.granted", "reward.order.revocation_pending", "reward.order.revoked",
  "commission.adjusted", "commission.paid",
  "withdrawal.order.reviewing", "withdrawal.order.processing", "withdrawal.order.paid",
  "withdrawal.order.rejected", "withdrawal.order.failed", "withdrawal.order.cancelled",
] as const;
const LOCALES = ["en", "zh-CN"] as const;
const PLACEHOLDERS = new Set(["points", "resource_id"]);

export type NotificationTemplateKey = typeof KEYS[number];
export type NotificationLocale = typeof LOCALES[number];
export interface NotificationTemplateText { title: string; body: string }
export interface NotificationTemplateContent {
  en: NotificationTemplateText;
  "zh-CN": NotificationTemplateText;
}
export interface NotificationTemplate {
  brand_id: string;
  key: NotificationTemplateKey;
  version: number;
  content: NotificationTemplateContent;
  updated_at: string;
  audit_log_id: string | null;
}
export interface NotificationTemplateRevision {
  id: string;
  brand_id: string;
  key: NotificationTemplateKey;
  version: number;
  content: NotificationTemplateContent;
  changed_by: string | null;
  reason: string;
  audit_log_id: string | null;
  created_at: string;
}
export interface NotificationTemplatePutBody {
  version: number;
  content: NotificationTemplateContent;
  reason: string;
}
export interface NotificationTemplatesApi {
  list(brandId: string): Promise<NotificationTemplate[]>;
  history(key: NotificationTemplateKey, brandId: string, limit?: number, offset?: number): Promise<NotificationTemplateRevision[]>;
  put(key: NotificationTemplateKey, brandId: string, body: NotificationTemplatePutBody, idempotencyKey: string): Promise<NotificationTemplate>;
}

export const notificationTemplateKeys = KEYS;

export function notificationTemplatePermissions(account: AdminAccount, brandId: string): { view: boolean; write: boolean } {
  const scoped = UUID_RE.test(account.id) && UUID_RE.test(brandId);
  const member = scoped && (account.brand_ids ?? []).includes(brandId);
  const brandPermissions = new Set(account.permissions_by_brand?.[brandId] ?? []);
  const platformPermissions = new Set(account.platform_permissions ?? []);
  return {
    view: scoped && (member && brandPermissions.has("notification_template.view.brand") ||
      platformPermissions.has("notification_template.view.platform")),
    write: member && !account.super_admin && brandPermissions.has("notification_template.write.brand"),
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}
function exactKeys(value: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(value).sort(), expected = [...keys].sort();
  return actual.length === expected.length && actual.every((key, index) => key === expected[index]);
}
function validUuid(value: unknown): value is string { return typeof value === "string" && UUID_RE.test(value); }
function validVersion(value: unknown): value is number { return Number.isSafeInteger(value) && Number(value) > 0 && Number(value) <= MAX_SAFE_VERSION; }
function validDateTime(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?Z$/.exec(value);
  if (!match) return false;
  const [, y, mo, d, h, mi, s] = match;
  const year = Number(y), month = Number(mo), day = Number(d);
  const days = month === 2 ? (year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28) : [4, 6, 9, 11].includes(month) ? 30 : 31;
  return year > 0 && month >= 1 && month <= 12 && day >= 1 && day <= days && Number(h) <= 23 && Number(mi) <= 59 && Number(s) <= 59 && Number.isFinite(Date.parse(value));
}
function bytes(value: string): number { return new TextEncoder().encode(value).length; }
function safeText(value: unknown, maxBytes: number, title: boolean): value is string {
  return typeof value === "string" && value.trim() === value && value.trim().length > 0 && bytes(value) <= maxBytes &&
    !/[<>\uD800-\uDFFF]/u.test(value) && !/(?:https?:|javascript:|data:|www\.)/i.test(value) &&
    !(title ? /[\u0000-\u001f\u007f-\u009f]/ : /[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f-\u009f]/).test(value);
}
function placeholdersValid(text: string, key: NotificationTemplateKey, body: boolean): boolean {
  const tokens = text.match(/\{[^{}]*\}/g) ?? [];
  const residue = text.replace(/\{[^{}]*\}/g, "");
  if (/[{}]/.test(residue) || tokens.some((token) => !PLACEHOLDERS.has(token.slice(1, -1)))) return false;
  const hasPoints = tokens.some((token) => token === "{points}");
  return key === "member.joined" ? !hasPoints : !body || hasPoints;
}
export function validNotificationTemplateContent(value: unknown, key: NotificationTemplateKey): value is NotificationTemplateContent {
  if (!validKey(key) || !isRecord(value) || !exactKeys(value, LOCALES)) return false;
  for (const locale of LOCALES) {
    const entry = value[locale];
    if (!isRecord(entry) || !exactKeys(entry, ["title", "body"]) || !safeText(entry.title, 120, true) ||
      !safeText(entry.body, 1200, false) || !placeholdersValid(entry.title, key, false) || !placeholdersValid(entry.body, key, true)) return false;
  }
  return true;
}
function validKey(value: unknown): value is NotificationTemplateKey { return typeof value === "string" && (KEYS as readonly string[]).includes(value); }
function sameContent(left: unknown, right: NotificationTemplateContent): boolean {
  return isRecord(left) && LOCALES.every((locale) => isRecord(left[locale]) &&
    left[locale].title === right[locale].title && left[locale].body === right[locale].body);
}
function validTemplate(value: unknown, brand: string, key?: NotificationTemplateKey): value is NotificationTemplate {
  return isRecord(value) && validUuid(value.brand_id) && value.brand_id === brand && validKey(value.key) &&
    (key === undefined || value.key === key) && validVersion(value.version) &&
    validNotificationTemplateContent(value.content, value.key) && validDateTime(value.updated_at) &&
    (value.version === 1 ? value.audit_log_id === null : validUuid(value.audit_log_id));
}
function validRevision(value: unknown, brand: string, key: NotificationTemplateKey): value is NotificationTemplateRevision {
  if (!isRecord(value) || !validUuid(value.id) || value.brand_id !== brand || !validKey(value.key) || value.key !== key ||
    !validVersion(value.version) || !validNotificationTemplateContent(value.content, key) ||
    !(value.changed_by === null || validUuid(value.changed_by)) || typeof value.reason !== "string" ||
    (value.audit_log_id !== null && !validUuid(value.audit_log_id)) || !validDateTime(value.created_at)) return false;
  return value.version === 1 ? value.changed_by === null && value.audit_log_id === null
    : value.changed_by !== null && value.audit_log_id !== null;
}
function invalidInput(): never { throw new AdminApiError("通知模板参数无效。", 400, "INVALID_INPUT"); }
function invalidResponse(write = false): never { throw new AdminApiError("通知模板响应格式无效。", write ? 0 : 502, "INVALID_RESPONSE"); }
type Envelope = { success?: unknown; data?: unknown; error?: unknown };

export function createNotificationTemplatesApi(fetchImpl: typeof fetch = fetch): NotificationTemplatesApi {
  async function request(path: string, brandId: string, options: { method?: "GET" | "PUT"; body?: unknown; key?: string; write?: boolean } = {}): Promise<unknown> {
    if (!validUuid(brandId)) invalidInput();
    const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brandId });
    if (options.body !== undefined) headers.set("Content-Type", "application/json");
    if (options.key !== undefined) headers.set("Idempotency-Key", options.key);
    let response: Response;
    try {
      response = await fetchImpl(path, { method: options.method ?? "GET", credentials: "same-origin", headers,
        ...(options.body === undefined ? {} : { body: JSON.stringify(options.body) }) });
    } catch (cause) {
      throw new AdminApiError(cause instanceof Error ? cause.message : "Network request failed", 0, "NETWORK_ERROR");
    }
    let envelope: Envelope;
    try { envelope = await response.json() as Envelope; }
    catch {
      if (!response.ok) throw new AdminApiError(`Request failed (${response.status})`, response.status);
      invalidResponse(Boolean(options.write));
    }
    if (!response.ok || !isRecord(envelope) || envelope.success !== true || envelope.data === undefined) {
      const error = isRecord(envelope?.error) ? envelope.error : undefined;
      const message = typeof envelope?.error === "string" ? envelope.error : typeof error?.message === "string" ? error.message : `Request failed (${response.status})`;
      if (!response.ok) throw new AdminApiError(message, response.status, typeof error?.code === "string" ? error.code : undefined);
      invalidResponse(Boolean(options.write));
    }
    return envelope.data;
  }
  return {
    async list(brandId) {
      const value = await request(BASE, brandId);
      if (!isRecord(value) || !exactKeys(value, ["items"]) || !Array.isArray(value.items) || value.items.length > KEYS.length ||
        !value.items.every((item) => validTemplate(item, brandId)) || new Set(value.items.map((item) => (item as NotificationTemplate).key)).size !== value.items.length) invalidResponse();
      return value.items as NotificationTemplate[];
    },
    async history(key, brandId, limit = 20, offset = 0) {
      if (!validKey(key) || !Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000) invalidInput();
      const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
      const value = await request(`${BASE}/${encodeURIComponent(key)}/history?${query}`, brandId);
      if (!isRecord(value) || !exactKeys(value, ["items"]) || !Array.isArray(value.items) || value.items.length > limit ||
        !value.items.every((item) => validRevision(item, brandId, key))) invalidResponse();
      return value.items as NotificationTemplateRevision[];
    },
    async put(key, brandId, body, idempotencyKey) {
      if (!validKey(key) || !isRecord(body) || !exactKeys(body, ["version", "content", "reason"]) || !validVersion(body.version) ||
        body.version >= MAX_SAFE_VERSION || !validNotificationTemplateContent(body.content, key) ||
        typeof body.reason !== "string" || body.reason.trim() !== body.reason || !body.reason.trim() || bytes(body.reason) > 500 ||
        /[\u0000-\u001f\u007f-\u009f\uD800-\uDFFF]/u.test(body.reason) || typeof idempotencyKey !== "string" || !/^[A-Za-z0-9_:.-]{8,128}$/.test(idempotencyKey)) invalidInput();
      const value = await request(`${BASE}/${encodeURIComponent(key)}`, brandId, { method: "PUT", body, key: idempotencyKey, write: true });
      if (!validTemplate(value, brandId, key) || value.version !== body.version + 1 || !sameContent(value.content, body.content)) invalidResponse(true);
      return value;
    },
  };
}
