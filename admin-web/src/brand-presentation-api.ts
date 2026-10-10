import { AdminApiError, type AdminAccount } from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";

const BASE = "/api/v1/admin/brand-presentation";
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const CONFIG_KEYS = ["display_name", "logo_text", "logo_url", "favicon_url", "primary_color", "accent_color", "success_color", "warning_color", "danger_color", "font_family", "font_scale", "radius", "shadow", "default_locale", "available_locales", "content"] as const;
const LOCALES = ["en", "zh-CN"] as const;
const COLOR_RE = /^#[0-9a-fA-F]{6}$/;

export type BrandPresentationLocale = (typeof LOCALES)[number];
export interface BrandPresentationLocaleContent { en: { tagline: string | null; announcement: string | null }; "zh-CN": { tagline: string | null; announcement: string | null } }
export interface BrandPresentationConfig {
  display_name: string | null;
  logo_text: string | null;
  logo_url: string | null;
  favicon_url: string | null;
  primary_color: string | null;
  accent_color: string | null;
  success_color: string | null;
  warning_color: string | null;
  danger_color: string | null;
  font_family: "system" | "serif" | "mono" | null;
  font_scale: "compact" | "standard" | "large" | null;
  radius: "square" | "soft" | "round" | null;
  shadow: "none" | "subtle" | "lifted" | null;
  default_locale: BrandPresentationLocale | null;
  available_locales: BrandPresentationLocale[] | null;
  content: BrandPresentationLocaleContent | null;
}
export interface EffectiveBrandPresentationConfig extends Omit<BrandPresentationConfig, keyof BrandPresentationConfig> {
  display_name: string;
  logo_text: string;
  logo_url: string | null;
  favicon_url: string | null;
  primary_color: string;
  accent_color: string;
  success_color: string;
  warning_color: string;
  danger_color: string;
  font_family: "system" | "serif" | "mono";
  font_scale: "compact" | "standard" | "large";
  radius: "square" | "soft" | "round";
  shadow: "none" | "subtle" | "lifted";
  default_locale: BrandPresentationLocale;
  available_locales: BrandPresentationLocale[];
  content: { en: { tagline: string; announcement: string }; "zh-CN": { tagline: string; announcement: string } };
}
export type BrandPresentationEffective = EffectiveBrandPresentationConfig;
export type BrandPresentationStatus = "active" | "paused" | "disabled";
export interface BrandPresentationRecord {
  brand_id: string; version: number; status: BrandPresentationStatus; base_name: string;
  config: BrandPresentationConfig; effective: EffectiveBrandPresentationConfig; updated_at: string; audit_log_id?: string;
}
export interface BrandPresentationReceipt extends BrandPresentationRecord { audit_log_id: string }
export interface BrandPresentationRevision {
  id: string; brand_id: string; version: number; config: BrandPresentationConfig; effective: EffectiveBrandPresentationConfig;
  changed_by: string; reason: string; audit_log_id: string; created_at: string;
}
export interface BrandPresentationHistoryPage { items: BrandPresentationRevision[]; limit: number; offset: number }
export interface BrandPresentationPutBody { version: number; config: BrandPresentationConfig; reason: string }
export interface BrandPresentationApi {
  get(brandId: string): Promise<BrandPresentationRecord>;
  history(brandId: string, limit?: number, offset?: number): Promise<BrandPresentationHistoryPage>;
  put(brandId: string, body: BrandPresentationPutBody, idempotencyKey: string): Promise<BrandPresentationReceipt>;
}

export function brandPresentationPermissions(account: AdminAccount, brandId: string): { view: boolean; write: boolean } {
  const brandPermissions = brandPermissionSet(account, brandId);
  const inBrand = UUID_RE.test(account.id) && UUID_RE.test(brandId) && (account.brand_ids ?? []).includes(brandId);
  return {
    view: inBrand && brandPermissions.has("brand_presentation.view.brand"),
    write: inBrand && brandPermissions.has("brand_presentation.write.brand"),
  };
}

function isRecord(value: unknown): value is Record<string, unknown> { return Boolean(value && typeof value === "object" && !Array.isArray(value)); }
function exactKeys(value: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(value).sort(), expected = [...keys].sort();
  return actual.length === expected.length && actual.every((key, i) => key === expected[i]);
}
function validUuid(value: unknown): value is string { return typeof value === "string" && UUID_RE.test(value); }
function validVersion(value: unknown): value is number { return Number.isSafeInteger(value) && Number(value) >= 1; }
function validDateTime(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?Z$/.exec(value);
  if (!m) return false;
  const [, y, mo, d, h, mi, s] = m;
  const year = Number(y), month = Number(mo), day = Number(d);
  const days = month === 2 ? (year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28) : [4, 6, 9, 11].includes(month) ? 30 : 31;
  return year >= 1 && month >= 1 && month <= 12 && day >= 1 && day <= days && Number(h) <= 23 && Number(mi) <= 59 && Number(s) <= 59 && Number.isFinite(Date.parse(value));
}
const HOST_RE = /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+$/;
const ASSET_EXT_RE = /\.(?:png|jpe?g|webp|ico)$/i;
function byteLength(value: string): number { return new TextEncoder().encode(value).length; }
function validText(value: unknown, maxBytes: number, oneLine: boolean, nonblank = false): value is string {
  return typeof value === "string" && byteLength(value) <= maxBytes && (!nonblank || value.trim().length > 0) &&
    (!oneLine || !/[\u0000-\u001f\u007f-\u009f]/.test(value));
}
function validAssetUri(value: unknown): value is string | null {
  if (value === null) return true;
  if (typeof value !== "string" || !value || byteLength(value) > 512 || value.trim() !== value || /[\\%\s\p{Cc}]/u.test(value) || value.includes("..") || /[?#]/.test(value)) return false;
  if (!ASSET_EXT_RE.test(value.slice(value.lastIndexOf("/") + 1))) return false;
  if (value.startsWith("/") && !value.startsWith("//")) return value.startsWith("/icons/") || value.startsWith("/brand-assets/");
  if (!value.startsWith("https://")) return false;
  try {
    const url = new URL(value);
    const authority = value.slice("https://".length).split(/[/?#]/, 1)[0];
    const host = url.hostname.toLowerCase();
    return url.protocol === "https:" && !url.username && !url.password && !url.search && !url.hash && !authority.includes(":") &&
      HOST_RE.test(host) && /[a-z]$/i.test(host) && !/^\d+(?:\.\d+){3}$/.test(host) && host !== "localhost" && !host.endsWith(".localhost") && !host.endsWith(".local") && !host.endsWith(".internal");
  } catch { return false; }
}
function validLocaleContent(value: unknown, effective = false): boolean {
  if (!isRecord(value) || !exactKeys(value, LOCALES)) return false;
  return LOCALES.every((locale) => {
    const entry = value[locale];
    return isRecord(entry) && exactKeys(entry, ["tagline", "announcement"]) &&
      (effective ? validText(entry.tagline, 160, false) && validText(entry.announcement, 2000, false) :
        (entry.tagline === null || validText(entry.tagline, 160, false)) && (entry.announcement === null || validText(entry.announcement, 2000, false)));
  });
}
function validConfig(value: unknown, effective = false): boolean {
  if (!isRecord(value) || !exactKeys(value, CONFIG_KEYS)) return false;
  const nullableName = (x: unknown, max: number) => x === null && !effective || validText(x, max, true, true);
  const name = (x: unknown, max: number) => validText(x, max, true, true);
  const string = (x: unknown): x is string => typeof x === "string";
  const enumValue = (x: unknown, allowed: readonly string[]) => x === null && !effective || typeof x === "string" && allowed.includes(x);
  const locales = value.available_locales;
  const localeListValid = Array.isArray(locales) && locales.length >= 1 && locales.length <= LOCALES.length && locales.every((x) => LOCALES.includes(x as BrandPresentationLocale)) && new Set(locales).size === locales.length;
  const localeNullable = !effective && locales === null;
  const defaultLocale = value.default_locale === null ? "en" : value.default_locale;
  return (effective ? name(value.display_name, 80) : nullableName(value.display_name, 80)) &&
    (effective ? name(value.logo_text, 32) : nullableName(value.logo_text, 32)) && validAssetUri(value.logo_url) && validAssetUri(value.favicon_url) &&
    (effective ? string(value.primary_color) && COLOR_RE.test(value.primary_color) : value.primary_color === null || typeof value.primary_color === "string" && COLOR_RE.test(value.primary_color)) &&
    (effective ? string(value.accent_color) && COLOR_RE.test(value.accent_color) : value.accent_color === null || typeof value.accent_color === "string" && COLOR_RE.test(value.accent_color)) &&
    (effective ? string(value.success_color) && COLOR_RE.test(value.success_color) : value.success_color === null || typeof value.success_color === "string" && COLOR_RE.test(value.success_color)) &&
    (effective ? string(value.warning_color) && COLOR_RE.test(value.warning_color) : value.warning_color === null || typeof value.warning_color === "string" && COLOR_RE.test(value.warning_color)) &&
    (effective ? string(value.danger_color) && COLOR_RE.test(value.danger_color) : value.danger_color === null || typeof value.danger_color === "string" && COLOR_RE.test(value.danger_color)) &&
    enumValue(value.font_family, ["system", "serif", "mono"]) && enumValue(value.font_scale, ["compact", "standard", "large"]) &&
    enumValue(value.radius, ["square", "soft", "round"]) && enumValue(value.shadow, ["none", "subtle", "lifted"]) &&
    (effective ? LOCALES.includes(value.default_locale as BrandPresentationLocale) : value.default_locale === null || LOCALES.includes(value.default_locale as BrandPresentationLocale)) &&
    (localeNullable || localeListValid) && (!effective || localeListValid) &&
    (locales === null || localeListValid && locales.includes(defaultLocale)) &&
    (effective ? validLocaleContent(value.content, true) : value.content === null || validLocaleContent(value.content));
}
export function validBrandPresentationConfig(config: unknown): config is BrandPresentationConfig { return validConfig(config); }
function validStatus(value: unknown): value is BrandPresentationStatus { return value === "active" || value === "paused" || value === "disabled"; }
function validRecord(value: unknown, brandId: string): value is BrandPresentationRecord {
  return isRecord(value) && (exactKeys(value, ["brand_id", "version", "status", "base_name", "config", "effective", "updated_at"]) || exactKeys(value, ["brand_id", "version", "status", "base_name", "config", "effective", "updated_at", "audit_log_id"])) &&
    value.brand_id === brandId && validVersion(value.version) && validStatus(value.status) && validText(value.base_name, 80, true, true) && validConfig(value.config) && validConfig(value.effective, true) && effectiveMatchesConfig(value.config, value.effective) && validDateTime(value.updated_at) &&
    (value.audit_log_id === undefined || validUuid(value.audit_log_id));
}
function validRevision(value: unknown, brandId: string): value is BrandPresentationRevision {
  return isRecord(value) && exactKeys(value, ["id", "brand_id", "version", "config", "effective", "changed_by", "reason", "audit_log_id", "created_at"]) && validUuid(value.id) && value.brand_id === brandId && validVersion(value.version) && validConfig(value.config) && validConfig(value.effective, true) && effectiveMatchesConfig(value.config, value.effective) && validUuid(value.changed_by) && validText(value.reason, 500, false, true) && validUuid(value.audit_log_id) && validDateTime(value.created_at);
}
function deepEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (Array.isArray(a) || Array.isArray(b)) return Array.isArray(a) && Array.isArray(b) && a.length === b.length && a.every((v, i) => deepEqual(v, b[i]));
  if (!isRecord(a) || !isRecord(b)) return false;
  const ak = Object.keys(a).sort(), bk = Object.keys(b).sort();
  return ak.length === bk.length && ak.every((k, i) => k === bk[i] && deepEqual(a[k], b[k]));
}
function effectiveMatchesConfig(config: unknown, effective: unknown): boolean {
  if (!isRecord(config) || !isRecord(effective)) return false;
  const scalarOverrides = ["display_name", "logo_text", "logo_url", "favicon_url", "primary_color", "accent_color", "success_color", "warning_color", "danger_color", "font_family", "font_scale", "radius", "shadow", "default_locale"];
  if (!scalarOverrides.every((key) => config[key] === null || config[key] === effective[key])) return false;
  if (config.available_locales !== null && !deepEqual(config.available_locales, effective.available_locales)) return false;
  if (config.content !== null) {
    if (!isRecord(config.content) || !isRecord(effective.content)) return false;
    for (const locale of LOCALES) {
      const override = config.content[locale];
      const resolved = effective.content[locale];
      if (!isRecord(override) || !isRecord(resolved)) return false;
      if (override.tagline !== null && override.tagline !== resolved.tagline) return false;
      if (override.announcement !== null && override.announcement !== resolved.announcement) return false;
    }
  }
  return Array.isArray(effective.available_locales) && effective.available_locales.includes(effective.default_locale as string);
}
function invalidInput(): never { throw new AdminApiError("品牌展示配置参数无效。", 400, "INVALID_INPUT"); }
function invalidResponse(write: boolean): never { throw new AdminApiError("品牌展示配置响应格式无效。", write ? 0 : 502, "INVALID_RESPONSE"); }
type Envelope = { success?: unknown; data?: unknown; error?: unknown };

export function createBrandPresentationApi(fetchImpl: typeof fetch = fetch): BrandPresentationApi {
  async function request(path: string, brandId: string, options: { method?: "GET" | "PUT"; body?: unknown; key?: string; write?: boolean } = {}): Promise<unknown> {
    if (!validUuid(brandId)) invalidInput();
    const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brandId });
    if (options.body !== undefined) headers.set("Content-Type", "application/json");
    if (options.key !== undefined) headers.set("Idempotency-Key", options.key);
    let response: Response;
    try { response = await fetchImpl(path, { method: options.method ?? "GET", credentials: "same-origin", headers, ...(options.body === undefined ? {} : { body: JSON.stringify(options.body) }) }); }
    catch (cause) { throw new AdminApiError(cause instanceof Error ? cause.message : "Network request failed", 0, "NETWORK_ERROR"); }
    let envelope: Envelope;
    try { envelope = await response.json() as Envelope; }
    catch { if (!response.ok) throw new AdminApiError(`Request failed (${response.status})`, response.status); invalidResponse(Boolean(options.write)); }
    if (response.status !== 200 && response.ok) invalidResponse(Boolean(options.write));
    if (!response.ok || !isRecord(envelope) || envelope.success !== true || envelope.data === undefined) {
      const error = isRecord(envelope?.error) ? envelope.error : undefined;
      if (typeof envelope?.error === "string") throw new AdminApiError("Invalid server response", 502, "INVALID_RESPONSE");
      const message = typeof error?.message === "string" ? error.message : `Request failed (${response.status})`;
      if (!response.ok) throw new AdminApiError(message, response.status, typeof error?.code === "string" ? error.code : undefined);
      invalidResponse(Boolean(options.write));
    }
    return envelope.data;
  }
  return {
    async get(brandId) { const data = await request(BASE, brandId); if (!validRecord(data, brandId)) invalidResponse(false); return data; },
    async history(brandId, limit = 20, offset = 0) {
      if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000) invalidInput();
      const data = await request(`${BASE}/history?limit=${limit}&offset=${offset}`, brandId);
      if (!isRecord(data) || !exactKeys(data, ["items", "limit", "offset"]) || data.limit !== limit || data.offset !== offset || !Array.isArray(data.items) || data.items.length > limit || !data.items.every((item) => validRevision(item, brandId))) invalidResponse(false);
      return data as unknown as BrandPresentationHistoryPage;
    },
    async put(brandId, body, idempotencyKey) {
      if (!validUuid(brandId) || !isRecord(body) || !exactKeys(body, ["version", "config", "reason"]) || !validVersion(body.version) || !validBrandPresentationConfig(body.config) || !validText(body.reason, 500, false) || !body.reason.trim() || typeof idempotencyKey !== "string" || !/^[A-Za-z0-9_:.-]{8,128}$/.test(idempotencyKey)) invalidInput();
      const data = await request(BASE, brandId, { method: "PUT", body, key: idempotencyKey, write: true });
      if (!isRecord(data) || !validRecord(data, brandId) || !exactKeys(data, ["brand_id", "version", "status", "base_name", "config", "effective", "updated_at", "audit_log_id"]) || data.version !== body.version + 1 || !validUuid(data.audit_log_id) || !deepEqual(data.config, body.config)) invalidResponse(true);
      return data as BrandPresentationReceipt;
    },
  };
}
