import { AdminApiError, type AdminAccount } from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";

const POLICY = "/api/v1/admin/commission-payment-policy";
const PAYMENTS = "/api/v1/admin/commission-payments";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const DATE_TIME = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/;
const DECIMAL = /^(?:0|[1-9][0-9]*)$/;

export type CommissionPaymentState = "awaiting_approval" | "paying" | "paid" | "failed" | "stale" | "blocked";
export type CommissionPayoutMode = "manual" | "automatic" | "mixed" | "none";
export interface CommissionPaymentPolicy { brand_id: string; version: number; enabled: boolean; audit_log_id: string; updated_at: string }
export interface CommissionPayment {
  id: string; brand_id: string; cycle_id: string; run_id: string; state: CommissionPaymentState;
  payout_mode: CommissionPayoutMode; version: number; evidence_epoch: string; total_points: string;
  paid_points: string; target_count: string; paid_count: string; creation_audit_log_id: string;
  last_error_code: string | null; created_at: string; updated_at: string;
}
export interface CommissionPaymentPage { brand_id: string; items: CommissionPayment[]; total_count: string; limit: number; offset: number }
export interface CommissionPaymentPolicyBody { version: number; enabled: boolean; reason: string }
export interface CommissionPaymentActionBody { version: number; reason: string }

type RecordValue = Record<string, unknown>;
function isRecord(value: unknown): value is RecordValue { return Boolean(value && typeof value === "object" && !Array.isArray(value)); }
function hasKeys(value: RecordValue, keys: readonly string[]): boolean { return Object.keys(value).length === keys.length && Object.keys(value).every((key) => keys.includes(key)); }
function validUuid(value: unknown): value is string { return typeof value === "string" && UUID.test(value); }
function validVersion(value: unknown): value is number { return Number.isSafeInteger(value) && Number(value) >= 1; }
function validDate(value: unknown): value is string {
  if (typeof value !== "string" || !DATE_TIME.test(value) || !Number.isFinite(Date.parse(value))) return false;
  const [date, rawTime] = value.split("T"), [year, month, day] = date.split("-").map(Number);
  const match = /^(\d{2}):(\d{2}):(\d{2})(?:\.\d{1,9})?(Z|([+-])(\d{2}):(\d{2}))$/.exec(rawTime);
  if (!match) return false;
  const hour = Number(match[1]), minute = Number(match[2]), second = Number(match[3]);
  const last = new Date(0); last.setUTCFullYear(year, month, 0);
  const offsetHour = Number(match[6]), offsetMinute = Number(match[7]);
  return year > 0 && month >= 1 && month <= 12 && day >= 1 && day <= last.getUTCDate() && hour <= 23 && minute <= 59 && second <= 59 &&
    (match[4] === "Z" || (offsetHour < 14 || (offsetHour === 14 && offsetMinute === 0)) && offsetMinute <= 59);
}
function validDecimal(value: unknown): value is string { return typeof value === "string" && DECIMAL.test(value); }
function validReason(value: unknown): value is string {
  if (typeof value !== "string" || !value || value.trim() !== value || new TextEncoder().encode(value).length > 500 || /[\u0000-\u001f\u007f-\u009f]/u.test(value)) return false;
  for (let i = 0; i < value.length; i++) {
    const code = value.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff) { const next = value.charCodeAt(++i); if (!(next >= 0xdc00 && next <= 0xdfff)) return false; }
    else if (code >= 0xdc00 && code <= 0xdfff) return false;
  }
  return true;
}
function invalidResponse(): never { throw new AdminApiError("佣金派发响应格式无效。", 502, "INVALID_RESPONSE"); }
function invalidInput(): never { throw new AdminApiError("佣金派发输入无效。", 0, "INVALID_INPUT"); }
function validPolicy(value: unknown, brand: string): value is CommissionPaymentPolicy {
  return isRecord(value) && hasKeys(value, ["brand_id", "version", "enabled", "audit_log_id", "updated_at"]) &&
    value.brand_id === brand && validUuid(value.brand_id) && validVersion(value.version) && typeof value.enabled === "boolean" &&
    (value.audit_log_id === "" || validUuid(value.audit_log_id)) && validDate(value.updated_at);
}
function validPayment(value: unknown, brand: string): value is CommissionPayment {
  if (!isRecord(value) || !hasKeys(value, ["id", "brand_id", "cycle_id", "run_id", "state", "payout_mode", "version", "evidence_epoch", "total_points", "paid_points", "target_count", "paid_count", "creation_audit_log_id", "last_error_code", "created_at", "updated_at"])) return false;
  if (typeof value.state !== "string" || typeof value.payout_mode !== "string") return false;
  if (!validUuid(value.id) || value.brand_id !== brand || !validUuid(value.brand_id) || !validUuid(value.cycle_id) || !validUuid(value.run_id) ||
    !["awaiting_approval", "paying", "paid", "failed", "stale", "blocked"].includes(value.state) ||
    !["manual", "automatic", "mixed", "none"].includes(value.payout_mode) || !validVersion(value.version) ||
    !validDecimal(value.evidence_epoch) || !validDecimal(value.total_points) || !validDecimal(value.paid_points) ||
    !validDecimal(value.target_count) || !validDecimal(value.paid_count) || BigInt(value.paid_count) > BigInt(value.target_count) ||
    BigInt(value.paid_points) > BigInt(value.total_points) || !validUuid(value.creation_audit_log_id) ||
    !(value.last_error_code === null || (typeof value.last_error_code === "string" && value.last_error_code.length > 0 && value.last_error_code.length <= 200)) ||
    !validDate(value.created_at) || !validDate(value.updated_at) || Date.parse(value.updated_at) < Date.parse(value.created_at)) return false;
  if ((value.state === "failed" || value.state === "blocked") !== (typeof value.last_error_code === "string")) return false;
  if (value.state === "paid" && (value.paid_count !== value.target_count || value.paid_points !== value.total_points)) return false;
  if (value.state === "awaiting_approval" && (!(["manual", "mixed"] as unknown[]).includes(value.payout_mode) || value.paid_count !== "0" || value.paid_points !== "0")) return false;
  return true;
}
function validPage(value: unknown, brand: string, limit: number, offset: number): value is CommissionPaymentPage {
  if (!isRecord(value) || !hasKeys(value, ["brand_id", "items", "total_count", "limit", "offset"]) || value.brand_id !== brand || !validUuid(value.brand_id) ||
    !Array.isArray(value.items) || value.items.length > limit || !value.items.every((item) => validPayment(item, brand)) || !validDecimal(value.total_count) || value.limit !== limit || value.offset !== offset) return false;
  const ids = value.items.map((item) => (item as CommissionPayment).id);
  return new Set(ids).size === ids.length && (ids.length === 0 || BigInt(value.total_count) >= BigInt(offset) + BigInt(ids.length));
}
function validBrand(value: string): void { if (!validUuid(value)) invalidInput(); }
function validKey(value: string): void { if (typeof value !== "string" || !/^[A-Za-z0-9._:-]{8,128}$/.test(value)) invalidInput(); }
function validPolicyBody(body: CommissionPaymentPolicyBody): void {
  if (!isRecord(body) || !hasKeys(body, ["version", "enabled", "reason"]) || !validVersion(body.version) || body.version >= Number.MAX_SAFE_INTEGER || typeof body.enabled !== "boolean" || !validReason(body.reason)) invalidInput();
}
function validActionBody(body: CommissionPaymentActionBody): void {
  if (!isRecord(body) || !hasKeys(body, ["version", "reason"]) || !validVersion(body.version) || body.version >= Number.MAX_SAFE_INTEGER || !validReason(body.reason)) invalidInput();
}

export function commissionPaymentPermissions(account: AdminAccount, brand: string) {
  const member = account.brand_ids.includes(brand);
  const brandPermissions = brandPermissionSet(account, brand);
  const view = validUuid(account.id) && validUuid(brand) && member && brandPermissions.has("commission.view.brand");
  return {
    view,
    policyWrite: Boolean(view && member && !account.super_admin && brandPermissions.has("commission_payment_policy.write.brand")),
    approve: Boolean(view && member && !account.super_admin && brandPermissions.has("commission_payment.approve.brand")),
    retry: Boolean(view && member && !account.super_admin && brandPermissions.has("commission_payment.retry.brand")),
  };
}

type Envelope = { success?: unknown; data?: unknown; error?: string | { code?: string; message?: string } | null };
// Actor is captured when the account-scoped client is created, not reread from
// a mutable cookie/session at retry time. Reads may use an unbound client.
export function createCommissionPaymentsApi(fetcher: typeof fetch = fetch, actorId?: string) {
  async function request(brand: string, path: string, options: { method?: "GET" | "PUT" | "POST"; body?: unknown; key?: string } = {}): Promise<unknown> {
    const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brand });
    if (options.body !== undefined) headers.set("Content-Type", "application/json");
    if (options.key !== undefined) headers.set("Idempotency-Key", options.key);
    if (options.method === "PUT" || options.method === "POST") {
      if (!validUuid(actorId)) invalidInput();
      headers.set("X-Commission-Payment-Actor-ID", actorId);
    }
    let response: Response;
    try { response = await fetcher(path, { method: options.method ?? "GET", credentials: "same-origin", headers, ...(options.body === undefined ? {} : { body: JSON.stringify(options.body) }) }); }
    catch (cause) { throw new AdminApiError(cause instanceof Error ? cause.message : "Network request failed", 0, "NETWORK_ERROR"); }
    let envelope: Envelope;
    try { envelope = await response.json() as Envelope; }
    catch { throw new AdminApiError(response.ok ? "Invalid server response" : `Request failed (${response.status})`, response.ok ? 502 : response.status, response.ok ? "INVALID_RESPONSE" : undefined); }
    if (!response.ok || !isRecord(envelope) || envelope.success !== true || envelope.data === undefined) {
      const detail = isRecord(envelope?.error) ? envelope.error : undefined;
      throw new AdminApiError(typeof envelope?.error === "string" ? envelope.error : (typeof detail?.message === "string" && detail.message) || `Request failed (${response.status})`, response.ok ? 502 : response.status, typeof detail?.code === "string" ? detail.code : undefined);
    }
    return envelope.data;
  }
  return {
    async getPolicy(brand: string): Promise<CommissionPaymentPolicy> { validBrand(brand); const data = await request(brand, POLICY); if (!validPolicy(data, brand)) invalidResponse(); return data; },
    async updatePolicy(brand: string, body: Readonly<CommissionPaymentPolicyBody>, key: string): Promise<CommissionPaymentPolicy> {
      validBrand(brand); validPolicyBody(body as CommissionPaymentPolicyBody); validKey(key);
      const data = await request(brand, POLICY, { method: "PUT", body, key });
      if (!validPolicy(data, brand) || data.version !== body.version + 1 || data.enabled !== body.enabled || !validUuid(data.audit_log_id)) invalidResponse();
      return data;
    },
    async list(brand: string, limit = 20, offset = 0): Promise<CommissionPaymentPage> {
      validBrand(brand); if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000) invalidInput();
      const query = new URLSearchParams({ limit: String(limit), offset: String(offset) }); const data = await request(brand, `${PAYMENTS}?${query}`);
      if (!validPage(data, brand, limit, offset)) invalidResponse(); return data;
    },
    async read(brand: string, id: string): Promise<CommissionPayment> { validBrand(brand); if (!validUuid(id)) invalidInput(); const data = await request(brand, `${PAYMENTS}/${encodeURIComponent(id)}`); if (!validPayment(data, brand) || data.id !== id) invalidResponse(); return data; },
    async approve(brand: string, id: string, body: Readonly<CommissionPaymentActionBody>, key: string): Promise<CommissionPayment> { return action("approve", brand, id, body, key); },
    async retry(brand: string, id: string, body: Readonly<CommissionPaymentActionBody>, key: string): Promise<CommissionPayment> { return action("retry", brand, id, body, key); },
  };
  async function action(name: "approve" | "retry", brand: string, id: string, body: Readonly<CommissionPaymentActionBody>, key: string): Promise<CommissionPayment> {
    validBrand(brand); if (!validUuid(id)) invalidInput(); validActionBody(body as CommissionPaymentActionBody); validKey(key);
    const data = await request(brand, `${PAYMENTS}/${encodeURIComponent(id)}/${name}`, { method: "POST", body, key });
    if (!validPayment(data, brand) || data.id !== id || data.version !== body.version + 1 || data.state !== "paying" || data.last_error_code !== null || (name === "approve" && !(["manual", "mixed"] as CommissionPayoutMode[]).includes(data.payout_mode))) invalidResponse(); return data;
  }
}
