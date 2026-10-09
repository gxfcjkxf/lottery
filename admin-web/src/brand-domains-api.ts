import { AdminApiError, type AdminAccount } from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";

const BASE = "/api/v1/admin/brand-domains";
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

export interface BrandDomain { id: string; domain: string; enabled: boolean; is_primary: boolean }
export interface BrandDomainsRecord { brand_id: string; version: number; status: "active" | "paused" | "disabled"; domains: BrandDomain[]; updated_at: string; audit_log_id?: string }
export interface BrandDomainsReceipt extends BrandDomainsRecord { audit_log_id: string }
export interface BrandDomainsHistoryItem { id: string; brand_id: string; version: number; changed_by: string; reason: string; audit_log_id: string; created_at: string; before_domains: BrandDomain[]; domains: BrandDomain[] }
export interface BrandDomainsHistoryPage { items: BrandDomainsHistoryItem[]; limit: number; offset: number }
export interface BrandDomainPostBody { version: number; domain: string; enabled: boolean; is_primary: boolean; reason: string }
export interface BrandDomainPatchBody { version: number; enabled: boolean; is_primary: boolean; reason: string }
export interface BrandDomainsApi {
  get(brandId: string): Promise<BrandDomainsRecord>;
  history(brandId: string, limit?: number, offset?: number): Promise<BrandDomainsHistoryPage>;
  post(brandId: string, body: BrandDomainPostBody, key: string): Promise<BrandDomainsReceipt>;
  patch(brandId: string, domainId: string, body: BrandDomainPatchBody, key: string): Promise<BrandDomainsReceipt>;
}

export function brandDomainsPermissions(account: AdminAccount, brandId: string): { view: boolean; write: boolean } {
  const scoped = brandPermissionSet(account, brandId);
  const member = UUID_RE.test(account.id) && UUID_RE.test(brandId) && (account.brand_ids ?? []).includes(brandId);
  return {
    view: member && scoped.has("brand_domains.view.brand"),
    write: member && scoped.has("brand_domains.write.brand"),
  };
}

function isRecord(v: unknown): v is Record<string, unknown> { return Boolean(v && typeof v === "object" && !Array.isArray(v)); }
function exact(v: Record<string, unknown>, keys: string[]): boolean { const a = Object.keys(v).sort(), b = [...keys].sort(); return a.length === b.length && a.every((k, i) => k === b[i]); }
function uuid(v: unknown): v is string { return typeof v === "string" && UUID_RE.test(v); }
function version(v: unknown): v is number { return Number.isSafeInteger(v) && Number(v) >= 1; }
function timestamp(v: unknown): v is string {
  if (typeof v !== "string") return false;
  const m = /^(\d{4})-(\d\d)-(\d\d)T(\d\d):(\d\d):(\d\d)(?:\.\d{1,9})?Z$/.exec(v);
  if (!m) return false;
  const [, y, mo, d, h, mi, s] = m, year = Number(y), month = Number(mo), day = Number(d);
  const days = month === 2 ? (year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28) : [4, 6, 9, 11].includes(month) ? 30 : 31;
  return year >= 1 && month >= 1 && month <= 12 && day >= 1 && day <= days && Number(h) <= 23 && Number(mi) <= 59 && Number(s) <= 59 && Number.isFinite(Date.parse(v));
}
const DOMAIN_RE = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$/;
export function isCanonicalBrandDomain(host: string): boolean {
  if (typeof host !== "string" || host.length > 253 || !/^[\x00-\x7f]+$/.test(host) || /[\u0000-\u0020\u007f]/.test(host) || host !== host.toLowerCase() || host.endsWith(".") || host.includes("*") || host.includes(":")) return false;
  if (!DOMAIN_RE.test(host) || host.split(".").some((label) => label.length > 63)) return false;
  if (/^(?:\d{1,3}\.){3}\d{1,3}$/.test(host) || host.split(".").every((label) => /^\d+$/.test(label)) || host === "localhost" || host.endsWith(".localhost") || host.endsWith(".local") || host.endsWith(".localdomain") || host.endsWith(".internal")) return false;
  const labels = host.split(".");
  const finalLabel = labels[labels.length - 1];
  if (!/[a-z]/.test(finalLabel) || /^\d+$/.test(finalLabel) || /^0x[0-9a-f]+$/.test(finalLabel)) return false;
  return true;
}
function domain(value: unknown): value is BrandDomain {
  return isRecord(value) && exact(value, ["id", "domain", "enabled", "is_primary"]) && uuid(value.id) &&
    typeof value.domain === "string" && value.domain.length > 0 && new TextEncoder().encode(value.domain).length <= 253 &&
    !/[\u0000-\u001f\u007f]/.test(value.domain) && typeof value.enabled === "boolean" && typeof value.is_primary === "boolean" && (!value.is_primary || value.enabled);
}
function domainList(v: unknown): v is BrandDomain[] {
  return Array.isArray(v) && v.length <= 100 && v.every(domain) && new Set(v.map((item) => item.id)).size === v.length && new Set(v.map((item) => item.domain)).size === v.length && v.filter((item) => item.is_primary).length <= 1;
}
function record(value: unknown, brandId: string): value is BrandDomainsRecord {
  return isRecord(value) && (exact(value, ["brand_id", "version", "status", "domains", "updated_at"]) || exact(value, ["brand_id", "version", "status", "domains", "updated_at", "audit_log_id"])) &&
    value.brand_id === brandId && version(value.version) && ["active", "paused", "disabled"].includes(String(value.status)) && domainList(value.domains) && timestamp(value.updated_at) && (value.audit_log_id === undefined || uuid(value.audit_log_id));
}
function invalidInput(): never { throw new AdminApiError("品牌域名参数无效。", 400, "BRAND_DOMAIN_INPUT_INVALID"); }
function invalidResponse(write: boolean): never { throw new AdminApiError("品牌域名响应格式无效。", write ? 0 : 502, "INVALID_RESPONSE"); }
type Envelope = { success?: unknown; data?: unknown; error?: unknown };

export function createBrandDomainsApi(fetchImpl: typeof fetch = fetch): BrandDomainsApi {
  async function request(path: string, brandId: string, method: "GET" | "POST" | "PATCH" = "GET", body?: unknown, key?: string): Promise<{ data: unknown; status: number }> {
    if (!uuid(brandId)) invalidInput();
    const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brandId });
    if (body !== undefined) headers.set("Content-Type", "application/json");
    if (key !== undefined) headers.set("Idempotency-Key", key);
    let response: Response;
    try { response = await fetchImpl(path, { method, credentials: "same-origin", headers, ...(body === undefined ? {} : { body: JSON.stringify(body) }) }); }
    catch (cause) { throw new AdminApiError(cause instanceof Error ? cause.message : "Network request failed", 0, "NETWORK_ERROR"); }
    let envelope: Envelope;
    try { envelope = await response.json() as Envelope; }
    catch { if (!response.ok) throw new AdminApiError(`Request failed (${response.status})`, response.status); invalidResponse(method !== "GET"); }
    const expectedStatus = method === "GET" ? 200 : method === "POST" ? 201 : 200;
    if (response.status !== expectedStatus) {
      const err = isRecord(envelope?.error) ? envelope.error : undefined;
      if (!response.ok) throw new AdminApiError(typeof err?.message === "string" ? err.message : typeof envelope?.error === "string" ? envelope.error : `Request failed (${response.status})`, response.status, typeof err?.code === "string" ? err.code : undefined);
      invalidResponse(method !== "GET");
    }
    if (!isRecord(envelope) || envelope.success !== true || envelope.data === undefined) {
      const err = isRecord(envelope?.error) ? envelope.error : undefined;
      if (!response.ok) throw new AdminApiError(typeof err?.message === "string" ? err.message : `Request failed (${response.status})`, response.status, typeof err?.code === "string" ? err.code : undefined);
      invalidResponse(method !== "GET");
    }
    return { data: envelope.data, status: response.status };
  }
  function validWrite(body: BrandDomainPostBody | BrandDomainPatchBody, key: string, creating: boolean): boolean {
    return isRecord(body) && exact(body, creating ? ["version", "domain", "enabled", "is_primary", "reason"] : ["version", "enabled", "is_primary", "reason"]) &&
      version(body.version) && typeof body.enabled === "boolean" && typeof body.is_primary === "boolean" && (!body.is_primary || body.enabled) &&
      typeof body.reason === "string" && body.reason.trim().length > 0 && new TextEncoder().encode(body.reason).length <= 500 && /^[A-Za-z0-9_:.-]{8,128}$/.test(key);
  }
  function validateReceipt(value: unknown, brandId: string, body: BrandDomainPostBody | BrandDomainPatchBody, targetId?: string): BrandDomainsReceipt {
    if (!record(value, brandId) || !exact(value as unknown as Record<string, unknown>, ["brand_id", "version", "status", "domains", "updated_at", "audit_log_id"]) || value.version !== body.version + 1 || !uuid(value.audit_log_id)) invalidResponse(true);
    const domains = value.domains;
    if ("domain" in body) {
      const created = domains.find((item) => item.domain === body.domain);
      if (!created || created.enabled !== body.enabled || created.is_primary !== body.is_primary) invalidResponse(true);
    } else {
      const updated = domains.find((item) => item.id === targetId);
      if (!updated || updated.enabled !== body.enabled || updated.is_primary !== body.is_primary) invalidResponse(true);
    }
    if (body.is_primary && domains.filter((item) => item.is_primary).length !== 1) invalidResponse(true);
    return value as BrandDomainsReceipt;
  }
  return {
    async get(brandId) { const { data } = await request(BASE, brandId); if (!record(data, brandId)) invalidResponse(false); return data; },
    async history(brandId, limit = 20, offset = 0) {
      if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000) invalidInput();
      const { data } = await request(`${BASE}/history?limit=${limit}&offset=${offset}`, brandId);
      const validItem = (v: unknown): v is BrandDomainsHistoryItem => isRecord(v) && exact(v, ["id", "brand_id", "version", "changed_by", "reason", "audit_log_id", "created_at", "before_domains", "domains"]) && uuid(v.id) && v.brand_id === brandId && version(v.version) && uuid(v.changed_by) && typeof v.reason === "string" && new TextEncoder().encode(v.reason).length <= 500 && uuid(v.audit_log_id) && timestamp(v.created_at) && domainList(v.before_domains) && domainList(v.domains);
      if (!isRecord(data) || !exact(data, ["items", "limit", "offset"]) || data.limit !== limit || data.offset !== offset || !Array.isArray(data.items) || data.items.length > limit || !data.items.every(validItem)) invalidResponse(false);
      return data as unknown as BrandDomainsHistoryPage;
    },
    async post(brandId, body, key) {
      if (!validWrite(body, key, true) || !isCanonicalBrandDomain(body.domain)) invalidInput();
      const { data } = await request(BASE, brandId, "POST", { version: body.version, domain: body.domain, enabled: body.enabled, is_primary: body.is_primary, reason: body.reason }, key);
      return validateReceipt(data, brandId, body);
    },
    async patch(brandId, domainId, body, key) {
      if (!uuid(domainId) || !validWrite(body, key, false)) invalidInput();
      const { data } = await request(`${BASE}/${encodeURIComponent(domainId)}`, brandId, "PATCH", { version: body.version, enabled: body.enabled, is_primary: body.is_primary, reason: body.reason }, key);
      return validateReceipt(data, brandId, body, domainId);
    },
  };
}
