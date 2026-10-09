import { AdminApiError, type AdminAccount } from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";

const PAYMENTS = "/api/v1/admin/commission-payments";
const TARGETS = "/api/v1/admin/commission-payment-targets";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const UINT = /^(?:0|[1-9][0-9]*)$/;
const SINT = /^(?:[1-9][0-9]*|-[1-9][0-9]*|0)$/;
const DATE = /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?(?:Z|[+-]\d\d:\d\d)$/;
const I64_MAX = 9223372036854775807n;
const I64_MIN = -9223372036854775808n;

export interface CommissionAdjustmentTarget {
  id: string; brand_id: string; payment_id: string; earning_id: string; agent_id: string; member_id: string;
  original_points: string; state: "pending" | "paid"; ledger_entry_id: string | null; paid_at: string | null;
  adjustment_version: number | null; adjusted_points: string | null; last_adjustment_id: string | null;
}
export interface CommissionAdjustment {
  id: string; brand_id: string; target_id: string; payment_id: string; version: number;
  points_before: string; points_after: string; delta_points: string; ledger_entry_id: string; audit_log_id: string;
  created_by: string; reason: string; point_policy_version: string; created_at: string;
}
export interface CommissionAdjustmentTargetPage { brand_id: string; payment_id: string; items: CommissionAdjustmentTarget[]; total_count: string; limit: number; offset: number }
export interface CommissionAdjustmentHistoryPage { brand_id: string; target_id: string; items: CommissionAdjustment[]; total_count: string; limit: number; offset: number }
export interface CommissionAdjustmentBody { version: number; points: string; reason: string }
type Obj = Record<string, unknown>;
function obj(v: unknown): v is Obj { return !!v && typeof v === "object" && !Array.isArray(v); }
function keys(v: Obj, expected: string[]) { return Object.keys(v).length === expected.length && expected.every((k) => Object.hasOwn(v, k)); }
function uuid(v: unknown): v is string { return typeof v === "string" && UUID.test(v); }
function uint(v: unknown): v is string { return typeof v === "string" && UINT.test(v) && BigInt(v) <= I64_MAX; }
function sint(v: unknown): v is string { return typeof v === "string" && SINT.test(v) && BigInt(v) >= I64_MIN && BigInt(v) <= I64_MAX; }
function safeVersion(v: unknown): v is number { return Number.isSafeInteger(v) && Number(v) >= 1; }
function date(v: unknown): v is string { return typeof v === "string" && DATE.test(v) && Number.isFinite(Date.parse(v)); }
export function validCommissionAdjustmentReason(v: unknown): v is string {
  if (typeof v !== "string" || !v || v.trim() !== v || new TextEncoder().encode(v).length > 500 || v.includes("\u0000")) return false;
  for (let i = 0; i < v.length; i++) { const c = v.charCodeAt(i); if (c >= 0xd800 && c <= 0xdbff) { const n = v.charCodeAt(++i); if (!(n >= 0xdc00 && n <= 0xdfff)) return false; } else if (c >= 0xdc00 && c <= 0xdfff) return false; }
  return true;
}
export function validCommissionAdjustmentPoints(v: unknown): v is string { return uint(v) && BigInt(v) <= I64_MAX; }
export function validCommissionAdjustmentDelta(before: string, after: string): boolean {
  try { const delta = BigInt(after) - BigInt(before); return delta !== 0n && delta >= I64_MIN && delta <= I64_MAX; } catch { return false; }
}
function invalid(): never { throw new AdminApiError("佣金更正输入无效。", 0, "INVALID_INPUT"); }
function malformed(): never { throw new AdminApiError("佣金更正响应格式无效。", 502, "INVALID_RESPONSE"); }
function target(v: unknown, brand: string, paymentId: string): v is CommissionAdjustmentTarget {
  const fields = ["id","brand_id","payment_id","earning_id","agent_id","member_id","original_points","state","ledger_entry_id","paid_at","adjustment_version","adjusted_points","last_adjustment_id"];
  if (!obj(v) || !keys(v, fields) || !uuid(v.id) || v.brand_id !== brand || v.payment_id !== paymentId || !uuid(v.earning_id) || !uuid(v.agent_id) || !uuid(v.member_id) || !uint(v.original_points) || !["pending","paid"].includes(String(v.state))) return false;
  if (!(v.ledger_entry_id === null || uuid(v.ledger_entry_id)) || !(v.paid_at === null || date(v.paid_at)) || !(v.adjustment_version === null || safeVersion(v.adjustment_version)) || !(v.adjusted_points === null || uint(v.adjusted_points)) || !(v.last_adjustment_id === null || uuid(v.last_adjustment_id))) return false;
  return v.state === "pending" ? v.ledger_entry_id === null && v.paid_at === null && v.adjustment_version === null && v.adjusted_points === null && v.last_adjustment_id === null
    : safeVersion(v.adjustment_version) && v.adjusted_points !== null && v.adjustment_version >= 1 && (v.adjustment_version !== 1 || v.adjusted_points === v.original_points && v.last_adjustment_id === null) && (v.adjustment_version === 1 || uuid(v.last_adjustment_id)) && date(v.paid_at) && (v.original_points !== "0" ? uuid(v.ledger_entry_id) : v.ledger_entry_id === null);
}
function adjustment(v: unknown, brand: string, targetId: string, paymentId: string): v is CommissionAdjustment {
  const fields = ["id","brand_id","target_id","payment_id","version","points_before","points_after","delta_points","ledger_entry_id","audit_log_id","created_by","reason","point_policy_version","created_at"];
  if (!obj(v) || !keys(v, fields) || !uuid(v.id) || v.brand_id !== brand || v.target_id !== targetId || v.payment_id !== paymentId || !safeVersion(v.version) || v.version < 2 || !uint(v.points_before) || !uint(v.points_after) || !sint(v.delta_points) || !uuid(v.ledger_entry_id) || !uuid(v.audit_log_id) || !uuid(v.created_by) || !validCommissionAdjustmentReason(v.reason) || !uint(v.point_policy_version) || v.point_policy_version === "0" || BigInt(v.point_policy_version) > I64_MAX || !date(v.created_at)) return false;
  return BigInt(v.points_after) - BigInt(v.points_before) === BigInt(v.delta_points) && v.delta_points !== "0";
}
function validPage<T>(v: unknown, brand: string, parentField: "payment_id" | "target_id", parentId: string, limit: number, offset: number, validate: (x: unknown) => x is T): boolean {
  const shape = ["brand_id",parentField,"items","total_count","limit","offset"];
  if (!obj(v) || !keys(v,shape) || v.brand_id !== brand || v[parentField] !== parentId || !Array.isArray(v.items) || v.items.length > limit || !v.items.every(validate) || !validDecimal(v.total_count) || v.limit !== limit || v.offset !== offset) return false;
  return v.items.length === 0 || BigInt(v.total_count) >= BigInt(offset) + BigInt(v.items.length);
}
function validDecimal(v: unknown): v is string { return typeof v === "string" && UINT.test(v); }
export function commissionAdjustmentPermissions(account: AdminAccount, brand: string) {
  const member = account.brand_ids.includes(brand);
  const brandPermissions = brandPermissionSet(account, brand);
  const view = uuid(account.id) && uuid(brand) && member && brandPermissions.has("commission.view.brand");
  return { view, write: Boolean(view && !account.super_admin && member && brandPermissions.has("commission_adjustment.write.brand") && brandPermissions.has("commission.view.brand")) };
}
type Envelope = { success?: unknown; data?: unknown; error?: unknown };
export function createCommissionAdjustmentsApi(fetcher: typeof fetch = fetch) {
  async function request(brand: string, path: string, method: "GET" | "POST" = "GET", body?: CommissionAdjustmentBody, actor?: string, key?: string): Promise<unknown> {
    const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brand });
    if (body) { headers.set("Content-Type", "application/json"); headers.set("X-Commission-Payment-Actor-ID", actor ?? ""); headers.set("Idempotency-Key", key ?? ""); }
    let response: Response;
    try { response = await fetcher(path, { method, credentials: "same-origin", headers, ...(body ? { body: JSON.stringify(body) } : {}) }); }
    catch (e) { throw new AdminApiError(e instanceof Error ? e.message : "Network request failed", 0, "NETWORK_ERROR"); }
    let envelope: Envelope;
    try { envelope = await response.json() as Envelope; } catch { throw new AdminApiError("Invalid server response", response.ok ? 502 : response.status, response.ok ? "INVALID_RESPONSE" : undefined); }
    if (!response.ok || !obj(envelope) || envelope.success !== true || envelope.data === undefined) {
      const error = obj(envelope?.error) ? envelope.error : {};
      throw new AdminApiError(typeof error.message === "string" ? error.message : `Request failed (${response.status})`, response.ok ? 502 : response.status, typeof error.code === "string" ? error.code : undefined);
    }
    const expectedStatus = method === "POST" ? 201 : 200;
    if (response.status !== expectedStatus) throw new AdminApiError("Unexpected server response status", 502, "INVALID_RESPONSE");
    return envelope.data;
  }
  function pageBounds(limit: number, offset: number) { if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000) invalid(); }
  return {
    async listTargets(brand: string, paymentId: string, limit = 20, offset = 0): Promise<CommissionAdjustmentTargetPage> {
      if (!uuid(brand) || !uuid(paymentId)) invalid(); pageBounds(limit, offset);
      const query = new URLSearchParams({limit:String(limit),offset:String(offset)}); const data = await request(brand, `${PAYMENTS}/${encodeURIComponent(paymentId)}/targets?${query}`);
      if (!validPage(data, brand, "payment_id", paymentId, limit, offset, (x): x is CommissionAdjustmentTarget => target(x, brand, paymentId))) malformed(); return data as CommissionAdjustmentTargetPage;
    },
    async listAdjustments(brand: string, targetId: string, limit = 20, offset = 0): Promise<CommissionAdjustmentHistoryPage> {
      if (!uuid(brand) || !uuid(targetId)) invalid(); pageBounds(limit, offset);
      const query = new URLSearchParams({limit:String(limit),offset:String(offset)}); const data = await request(brand, `${TARGETS}/${encodeURIComponent(targetId)}/adjustments?${query}`);
      if (!validPage(data, brand, "target_id", targetId, limit, offset, (x): x is CommissionAdjustment => obj(x) && adjustment(x, brand, targetId, String(x.payment_id)))) malformed(); return data as CommissionAdjustmentHistoryPage;
    },
    async createAdjustment(brand: string, targetId: string, paymentId: string, body: Readonly<CommissionAdjustmentBody>, actorId: string, key: string): Promise<CommissionAdjustment> {
      if (!uuid(brand) || !uuid(targetId) || !uuid(paymentId) || !uuid(actorId) || !/^[A-Za-z0-9._:-]{8,128}$/.test(key) || !obj(body) || !keys(body,["version","points","reason"]) || !safeVersion(body.version) || body.version >= Number.MAX_SAFE_INTEGER || !validCommissionAdjustmentPoints(body.points) || !validCommissionAdjustmentReason(body.reason)) invalid();
      const data = await request(brand, `${TARGETS}/${encodeURIComponent(targetId)}/adjustments`, "POST", body, actorId, key);
      if (!adjustment(data, brand, targetId, paymentId) || data.version !== body.version + 1 || data.points_after !== body.points || data.created_by !== actorId) malformed(); return data;
    },
  };
}
