import { AdminApiError, type AdminAccount } from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";

const BASE = "/api/v1/admin/compliance-policy";
const CHECKS = "/api/v1/admin/compliance-checks";
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const COUNTRIES_RE = /^[A-Z]{2}$/;
const OPERATIONS = ["registration", "betting", "withdrawal"] as const;
const CHECKS_ORDER = ["age", "region", "identity", "account_risk", "betting_risk", "exclusion", "responsible_gambling"] as const;
const CHECK_DECISIONS = ["allow", "review", "deny", "freeze"] as const;
const CHECK_REASONS = ["CHECK_DISABLED", "ADAPTER_NOT_CONFIGURED"] as const;

export type ComplianceOperation = (typeof OPERATIONS)[number];
export interface ComplianceConfig { age_enabled: boolean; minimum_age: number | null; region_enabled: boolean; allowed_countries: string[]; identity_enabled: boolean; account_risk_enabled: boolean; betting_risk_enabled: boolean; exclusion_enabled: boolean; responsible_gambling_enabled: boolean }
export interface CompliancePolicy { brand_id: string; version: number; config: ComplianceConfig; updated_at: string; audit_log_id?: string }
export interface CompliancePolicyRevision { id: string; brand_id: string; version: number; config: ComplianceConfig; changed_by: string | null; reason: string; audit_log_id: string | null; created_at: string }
export interface ComplianceHistoryPage { brand_id: string; items: CompliancePolicyRevision[]; limit: number; offset: number; total_count: string }
export interface ComplianceCheck { check: (typeof CHECKS_ORDER)[number]; enabled: boolean; decision: (typeof CHECK_DECISIONS)[number]; reason_code: (typeof CHECK_REASONS)[number] }
export interface ComplianceDecision { id: string; brand_id: string; policy_version: number; config: ComplianceConfig; operation: ComplianceOperation; decision: (typeof CHECK_DECISIONS)[number]; checks: ComplianceCheck[]; adapter_mode: "stub"; created_by: string; reason: string; audit_log_id: string; created_at: string }
export interface ComplianceChecksPage { brand_id: string; operation: ComplianceOperation | null; items: ComplianceDecision[]; limit: number; offset: number; total_count: string }
export type ComplianceGateOperation = "registration" | "betting";
export type ComplianceGateAction = "register" | "join" | "operator_join" | "bet_preview" | "bet_place";
export interface ComplianceGateRecord {
  id: string; brand_id: string; policy_version: number; config: ComplianceConfig; operation: ComplianceGateOperation;
  action: ComplianceGateAction; decision: "review"; checks: ComplianceCheck[]; adapter_mode: "stub";
  actor_type: "anonymous" | "user" | "admin"; actor_id: string | null; member_id: string | null;
  request_id: string; audit_log_id: string; created_at: string;
}
export interface ComplianceGatesPage { brand_id: string; operation: ComplianceGateOperation | null; items: ComplianceGateRecord[]; limit: number; offset: number; total_count: string }
export interface CompliancePolicyPutBody { version: number; config: ComplianceConfig; reason: string }
export interface ComplianceCheckBody { version: number; operation: ComplianceOperation; reason: string }
export interface CompliancePolicyApi {
  get(brandId: string): Promise<CompliancePolicy>;
  history(brandId: string, limit?: number, offset?: number): Promise<ComplianceHistoryPage>;
  put(brandId: string, body: CompliancePolicyPutBody, key: string): Promise<CompliancePolicy>;
  run(brandId: string, body: ComplianceCheckBody, key: string): Promise<ComplianceDecision>;
  decisions(brandId: string, limit?: number, offset?: number, operation?: ComplianceOperation): Promise<ComplianceChecksPage>;
  gates(brandId: string, limit?: number, offset?: number, operation?: ComplianceGateOperation): Promise<ComplianceGatesPage>;
}

export function compliancePermissions(account: AdminAccount, brandId: string): { viewPolicy: boolean; writePolicy: boolean; viewChecks: boolean; runCheck: boolean } {
  const brandGrants = brandPermissionSet(account, brandId);
  const validScope = UUID_RE.test(account.id) && UUID_RE.test(brandId);
  const inBrand = validScope && (account.brand_ids ?? []).includes(brandId);
  return {
    viewPolicy: validScope && inBrand && brandGrants.has("compliance_policy.view.brand"),
    writePolicy: inBrand && !account.super_admin && brandGrants.has("compliance_policy.write.brand"),
    viewChecks: validScope && inBrand && brandGrants.has("compliance_check.view.brand"),
    runCheck: inBrand && !account.super_admin && brandGrants.has("compliance_check.run.brand"),
  };
}
function record(value: unknown): value is Record<string, unknown> { return Boolean(value && typeof value === "object" && !Array.isArray(value)); }
function exact(value: Record<string, unknown>, keys: readonly string[]): boolean { const a = Object.keys(value).sort(), b = [...keys].sort(); return a.length === b.length && a.every((k, i) => k === b[i]); }
function uuid(value: unknown): value is string { return typeof value === "string" && UUID_RE.test(value); }
function version(value: unknown): value is number { return Number.isSafeInteger(value) && Number(value) >= 1; }
function timestamp(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const match = /^(\d{4})-(\d\d)-(\d\d)T(\d\d):(\d\d):(\d\d)(?:\.\d+)?(Z|[+-](\d\d):(\d\d))$/.exec(value);
  if (!match) return false;
  const year = Number(match[1]), month = Number(match[2]), day = Number(match[3]);
  const days = month === 2 ? year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28 : [4, 6, 9, 11].includes(month) ? 30 : 31;
  const zoneHour = match[8] ? Number(match[8]) : 0, zoneMinute = match[9] ? Number(match[9]) : 0;
  return year >= 1 && month >= 1 && month <= 12 && day >= 1 && day <= days && Number(match[4]) <= 23 && Number(match[5]) <= 59 && Number(match[6]) <= 59 && zoneHour <= 23 && zoneMinute <= 59 && Number.isFinite(Date.parse(value));
}
export function validComplianceReason(value: unknown): value is string { return typeof value === "string" && value.length > 0 && value.trim() === value && !/\p{Cc}/u.test(value) && new TextEncoder().encode(value).length <= 500; }
export function validComplianceConfig(value: unknown): value is ComplianceConfig {
  if (!record(value) || !exact(value, ["age_enabled", "minimum_age", "region_enabled", "allowed_countries", "identity_enabled", "account_risk_enabled", "betting_risk_enabled", "exclusion_enabled", "responsible_gambling_enabled"])) return false;
  if (!["age_enabled", "region_enabled", "identity_enabled", "account_risk_enabled", "betting_risk_enabled", "exclusion_enabled", "responsible_gambling_enabled"].every((key) => typeof value[key] === "boolean")) return false;
  if (value.minimum_age !== null && (!Number.isInteger(value.minimum_age) || Number(value.minimum_age) < 18 || Number(value.minimum_age) > 120)) return false;
  if (value.age_enabled && value.minimum_age === null) return false;
  if (!Array.isArray(value.allowed_countries) || value.allowed_countries.length > 250 || !value.allowed_countries.every((country) => typeof country === "string" && COUNTRIES_RE.test(country))) return false;
  const countries = value.allowed_countries as string[];
  if (new Set(countries).size !== countries.length || countries.some((country, i) => i > 0 && countries[i - 1] >= country)) return false;
  return !value.region_enabled || countries.length > 0;
}
function validCount(value: unknown): value is string { return typeof value === "string" && /^(0|[1-9][0-9]*)$/.test(value); }
function equalConfig(a: unknown, b: unknown): boolean {
  if (!record(a) || !record(b) || !Array.isArray(a.allowed_countries) || !Array.isArray(b.allowed_countries)) return false;
  const aCountries = a.allowed_countries as unknown[], bCountries = b.allowed_countries as unknown[];
  return a.age_enabled === b.age_enabled && a.minimum_age === b.minimum_age && a.region_enabled === b.region_enabled && a.identity_enabled === b.identity_enabled && a.account_risk_enabled === b.account_risk_enabled && a.betting_risk_enabled === b.betting_risk_enabled && a.exclusion_enabled === b.exclusion_enabled && a.responsible_gambling_enabled === b.responsible_gambling_enabled && aCountries.length === bCountries.length && aCountries.every((country, index) => country === bCountries[index]);
}
function enabledChecks(config: ComplianceConfig): boolean[] { return [config.age_enabled, config.region_enabled, config.identity_enabled, config.account_risk_enabled, config.betting_risk_enabled, config.exclusion_enabled, config.responsible_gambling_enabled]; }
function validPolicy(value: unknown, brandId: string): value is CompliancePolicy {
  if (!record(value) || !(exact(value, ["brand_id", "version", "config", "updated_at"]) || exact(value, ["brand_id", "version", "config", "updated_at", "audit_log_id"]))) return false;
  return value.brand_id === brandId && version(value.version) && validComplianceConfig(value.config) && timestamp(value.updated_at) && (value.audit_log_id === undefined || uuid(value.audit_log_id));
}
function validRevision(value: unknown, brandId: string): value is CompliancePolicyRevision {
  if (!record(value) || !exact(value, ["id", "brand_id", "version", "config", "changed_by", "reason", "audit_log_id", "created_at"]) || !uuid(value.id) || value.brand_id !== brandId || !version(value.version) || !validComplianceConfig(value.config) || !validComplianceReason(value.reason) || !timestamp(value.created_at)) return false;
  return value.version === 1 ? value.changed_by === null && value.audit_log_id === null : uuid(value.changed_by) && uuid(value.audit_log_id);
}
function validDecision(value: unknown, brandId: string): value is ComplianceDecision {
  if (!record(value) || !exact(value, ["id", "brand_id", "policy_version", "config", "operation", "decision", "checks", "adapter_mode", "created_by", "reason", "audit_log_id", "created_at"])) return false;
  if (!uuid(value.id) || value.brand_id !== brandId || !version(value.policy_version) || !validComplianceConfig(value.config) || !OPERATIONS.includes(value.operation as ComplianceOperation) || !CHECK_DECISIONS.includes(value.decision as ComplianceDecision["decision"]) || value.adapter_mode !== "stub" || !uuid(value.created_by) || !validComplianceReason(value.reason) || !uuid(value.audit_log_id) || !timestamp(value.created_at) || !Array.isArray(value.checks) || value.checks.length !== CHECKS_ORDER.length) return false;
  const expectedEnabled = enabledChecks(value.config);
  const checksValid = value.checks.every((item, i) => record(item) && exact(item, ["check", "enabled", "decision", "reason_code"]) && item.check === CHECKS_ORDER[i] && item.enabled === expectedEnabled[i] && item.decision === (expectedEnabled[i] ? "review" : "allow") && item.reason_code === (expectedEnabled[i] ? "ADAPTER_NOT_CONFIGURED" : "CHECK_DISABLED"));
  return checksValid && value.decision === (expectedEnabled.some(Boolean) ? "review" : "allow");
}
function validGateRecord(value: unknown, brandId: string): value is ComplianceGateRecord {
  const fields = ["id", "brand_id", "policy_version", "config", "operation", "action", "decision", "checks", "adapter_mode", "actor_type", "actor_id", "member_id", "request_id", "audit_log_id", "created_at"];
  if (!record(value) || !exact(value, fields) || !uuid(value.id) || value.brand_id !== brandId || !version(value.policy_version) || !validComplianceConfig(value.config)) return false;
  if (!(value.operation === "registration" || value.operation === "betting") || value.decision !== "review" || value.adapter_mode !== "stub" || !uuid(value.audit_log_id) || !timestamp(value.created_at)) return false;
  if (typeof value.request_id !== "string" || value.request_id.length < 1 || value.request_id.length > 80 || /\p{Cc}/u.test(value.request_id)) return false;
  const config = value.config as ComplianceConfig;
  const enabled = enabledChecks(config);
  if (!enabled.some(Boolean) || !Array.isArray(value.checks) || value.checks.length !== CHECKS_ORDER.length) return false;
  const checksValid = value.checks.every((item, i) => record(item) && exact(item, ["check", "enabled", "decision", "reason_code"]) && item.check === CHECKS_ORDER[i] && item.enabled === enabled[i] && item.decision === (enabled[i] ? "review" : "allow") && item.reason_code === (enabled[i] ? "ADAPTER_NOT_CONFIGURED" : "CHECK_DISABLED"));
  if (!checksValid) return false;
  if (value.operation === "registration") {
    if (value.action === "register") return value.actor_type === "anonymous" && value.actor_id === null && value.member_id === null;
    if (value.action === "join") return value.actor_type === "user" && uuid(value.actor_id) && (value.member_id === null || uuid(value.member_id));
    if (value.action === "operator_join") return value.actor_type === "admin" && uuid(value.actor_id) && value.member_id === null;
    return false;
  }
  return (value.action === "bet_preview" || value.action === "bet_place") && value.actor_type === "user" && uuid(value.actor_id) && uuid(value.member_id);
}
function invalid(write: boolean): never { throw new AdminApiError("合规接口响应格式无效。", write ? 0 : 502, "INVALID_RESPONSE"); }
function input(): never { throw new AdminApiError("合规请求参数无效。", 400, "INVALID_INPUT"); }
type Envelope = { success?: unknown; data?: unknown; error?: unknown };

export function createCompliancePolicyApi(fetchImpl: typeof fetch = fetch): CompliancePolicyApi {
  async function request(path: string, brandId: string, method: "GET" | "PUT" | "POST" = "GET", body?: unknown, key?: string, expectedStatus = 200): Promise<unknown> {
    if (!uuid(brandId)) input();
    const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brandId });
    if (body !== undefined) headers.set("Content-Type", "application/json");
    if (key !== undefined) headers.set("Idempotency-Key", key);
    let response: Response;
    try { response = await fetchImpl(path, { method, credentials: "same-origin", headers, ...(body === undefined ? {} : { body: JSON.stringify(body) }) }); }
    catch (cause) { throw new AdminApiError(cause instanceof Error ? cause.message : "Network request failed", 0, "NETWORK_ERROR"); }
    let envelope: Envelope;
    try { envelope = await response.json() as Envelope; } catch { if (!response.ok) throw new AdminApiError(`Request failed (${response.status})`, response.status); invalid(method !== "GET"); }
    if (!response.ok || !record(envelope) || envelope.success !== true || envelope.data === undefined) {
      const error = record(envelope?.error) ? envelope.error : undefined;
      const message = typeof envelope?.error === "string" ? envelope.error : typeof error?.message === "string" ? error.message : `Request failed (${response.status})`;
      if (!response.ok) throw new AdminApiError(message, response.status, typeof error?.code === "string" ? error.code : undefined);
      invalid(method !== "GET");
    }
    if (response.status !== expectedStatus) invalid(method !== "GET");
    return envelope.data;
  }
  return {
    async get(brandId) { const data = await request(BASE, brandId); if (!validPolicy(data, brandId)) invalid(false); return data; },
    async history(brandId, limit = 20, offset = 0) {
      if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000) input();
      const data = await request(`${BASE}/history?limit=${limit}&offset=${offset}`, brandId);
      if (!record(data) || !exact(data, ["brand_id", "items", "limit", "offset", "total_count"]) || data.brand_id !== brandId || data.limit !== limit || data.offset !== offset || !validCount(data.total_count) || !Array.isArray(data.items) || data.items.length > limit || !data.items.every((item) => validRevision(item, brandId))) invalid(false);
      return data as unknown as ComplianceHistoryPage;
    },
    async put(brandId, body, key) {
      if (!uuid(brandId) || !record(body) || !exact(body, ["version", "config", "reason"]) || !version(body.version) || !validComplianceConfig(body.config) || !validComplianceReason(body.reason) || typeof key !== "string" || !/^[A-Za-z0-9_:.-]{8,128}$/.test(key)) input();
      const data = await request(BASE, brandId, "PUT", body, key);
      if (!validPolicy(data, brandId) || !exact(data as unknown as Record<string, unknown>, ["brand_id", "version", "config", "updated_at", "audit_log_id"]) || data.version !== body.version + 1 || !uuid(data.audit_log_id) || !equalConfig(data.config, body.config)) invalid(true);
      return data;
    },
    async run(brandId, body, key) {
      if (!uuid(brandId) || !record(body) || !exact(body, ["version", "operation", "reason"]) || !version(body.version) || !OPERATIONS.includes(body.operation as ComplianceOperation) || !validComplianceReason(body.reason) || typeof key !== "string" || !/^[A-Za-z0-9_:.-]{8,128}$/.test(key)) input();
      const data = await request(CHECKS, brandId, "POST", body, key, 201);
      if (!validDecision(data, brandId) || data.policy_version !== body.version || data.operation !== body.operation || data.reason !== body.reason) invalid(true);
      return data;
    },
    async decisions(brandId, limit = 20, offset = 0, operation) {
      if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000 || operation !== undefined && !OPERATIONS.includes(operation)) input();
      const query = new URLSearchParams({ limit: String(limit), offset: String(offset) }); if (operation) query.set("operation", operation);
      const data = await request(`${CHECKS}?${query}`, brandId);
      if (!record(data) || !exact(data, ["brand_id", "operation", "items", "limit", "offset", "total_count"]) || data.brand_id !== brandId || data.operation !== (operation ?? null) || data.limit !== limit || data.offset !== offset || !validCount(data.total_count) || !Array.isArray(data.items) || data.items.length > limit || !data.items.every((item) => validDecision(item, brandId))) invalid(false);
      return data as unknown as ComplianceChecksPage;
    },
    async gates(brandId, limit = 20, offset = 0, operation) {
      if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000 || operation !== undefined && operation !== "registration" && operation !== "betting") input();
      const query = new URLSearchParams({ limit: String(limit), offset: String(offset) }); if (operation) query.set("operation", operation);
      const data = await request(`/api/v1/admin/compliance-gates?${query}`, brandId);
      if (!record(data) || !exact(data, ["brand_id", "operation", "items", "limit", "offset", "total_count"]) || data.brand_id !== brandId || data.operation !== (operation ?? null) || data.limit !== limit || data.offset !== offset || !validCount(data.total_count) || !Array.isArray(data.items) || data.items.length > limit || !data.items.every((item) => validGateRecord(item, brandId))) invalid(false);
      return data as unknown as ComplianceGatesPage;
    },
  };
}
