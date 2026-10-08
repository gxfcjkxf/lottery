import { AdminApiError, type AdminAccount } from "./admin-api";

const ROOT = "/api/v1/admin";
const POLICY = `${ROOT}/commission-correction-policy`;
const PLANS = `${ROOT}/commission-correction-plans`;
const EXECUTIONS = `${ROOT}/commission-correction-executions`;
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const UINT = /^(?:0|[1-9][0-9]*)$/;
const SINT = /^(?:0|-?[1-9][0-9]*)$/;
const DATE_TIME = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/;
const MAX_INT64 = 9_223_372_036_854_775_807n;
const MIN_INT64 = -9_223_372_036_854_775_808n;
const MAX_TARGETS = 100_000n;

export type CorrectionPlanState = "planning" | "ready" | "blocked" | "failed" | "stale";
export type CorrectionPayoutMode = "manual" | "automatic" | "mixed" | "none";
export type CorrectionExecutionState = "awaiting_approval" | "applying" | "completed" | "paused" | "failed" | "stale";
export type CorrectionExecutionTargetState = "pending" | "applied";
export type CommissionCorrectionErrorCode =
  | "COMMISSION_CORRECTION_BUSY" | "COMMISSION_CORRECTION_INPUT_INVALID" | "COMMISSION_CORRECTION_NOT_FOUND"
  | "COMMISSION_CORRECTION_STATE_CONFLICT" | "COMMISSION_CORRECTION_VERSION_CONFLICT"
  | "COMMISSION_CORRECTION_EVIDENCE_CONFLICT" | "COMMISSION_CORRECTION_LIMIT";

export interface CommissionCorrectionPolicy {
  brand_id: string; version: number; enabled: boolean; audit_log_id: string; updated_at: string;
}
export type GoExecutionPolicy = CommissionCorrectionPolicy;
export interface CommissionCorrectionPolicyBody { version: number; enabled: boolean; reason: string }
export interface CommissionCorrectionActionBody { version: number; reason: string }

export interface CorrectionPlan {
  id: string; brand_id: string; cycle_id: string; payment_id: string; run_id: string; state: CorrectionPlanState;
  payout_mode: CorrectionPayoutMode; version: number; evidence_epoch: string; before_points: string; calculated_points: string;
  credit_points: string | null; debit_points: string | null; net_points: string | null; target_count: string; planned_count: string;
  creation_audit_log_id: string; last_audit_log_id: string; last_error_code: string | null; created_at: string; updated_at: string;
}
export interface CorrectionPlanTarget {
  id: string; brand_id: string; plan_id: string; agent_id: string; member_id: string; original_target_id: string | null;
  earning_id: string | null; adjustment_version: number | null; previous_correction_target_id: string | null;
  financial_version: number | null; points_before: string; points_after: string; delta_points: string;
  creation_audit_log_id: string; created_at: string;
}
export interface CorrectionPlanPage { brand_id: string; items: CorrectionPlan[]; total_count: string; limit: number; offset: number }
export interface CorrectionPlanTargetPage { brand_id: string; plan_id: string; items: CorrectionPlanTarget[]; total_count: string; limit: number; offset: number }

export interface CorrectionExecution {
  id: string; brand_id: string; cycle_id: string; plan_id: string; run_id: string; payout_mode: CorrectionPayoutMode;
  state: CorrectionExecutionState; version: number; plan_version: number; evidence_epoch: string; credit_points: string;
  debit_points: string; net_points: string; applied_credit_points: string; applied_debit_points: string; target_count: string;
  applied_count: string; paused_plan_target_id: string | null; last_error_code: string | null; creation_audit_log_id: string;
  last_audit_log_id: string; approved_by: string | null; approval_actor_type: string | null; approval_audit_log_id: string | null;
  cycle_hold_active: boolean; created_at: string; updated_at: string;
}
export interface CorrectionExecutionTarget {
  id: string; brand_id: string; execution_id: string; plan_target_id: string; agent_id: string; member_id: string;
  points_before: string; points_after: string; delta_points: string; state: CorrectionExecutionTargetState;
  ledger_entry_id: string | null; audit_log_id: string | null; financial_version: number | null; created_at: string; applied_at: string | null;
}
export interface CorrectionExecutionPage { brand_id: string; items: CorrectionExecution[]; total_count: string; limit: number; offset: number }
export interface CorrectionExecutionTargetPage { brand_id: string; execution_id: string; items: CorrectionExecutionTarget[]; total_count: string; limit: number; offset: number }

type Obj = Record<string, unknown>;
type Envelope = Obj & { success?: unknown; data?: unknown; error?: unknown };
const PLAN_KEYS = ["id", "brand_id", "cycle_id", "payment_id", "run_id", "state", "payout_mode", "version", "evidence_epoch", "before_points", "calculated_points", "credit_points", "debit_points", "net_points", "target_count", "planned_count", "creation_audit_log_id", "last_audit_log_id", "last_error_code", "created_at", "updated_at"] as const;
const PLAN_TARGET_KEYS = ["id", "brand_id", "plan_id", "agent_id", "member_id", "original_target_id", "earning_id", "adjustment_version", "previous_correction_target_id", "financial_version", "points_before", "points_after", "delta_points", "creation_audit_log_id", "created_at"] as const;
const EXECUTION_KEYS = ["id", "brand_id", "cycle_id", "plan_id", "run_id", "payout_mode", "state", "version", "plan_version", "evidence_epoch", "credit_points", "debit_points", "net_points", "applied_credit_points", "applied_debit_points", "target_count", "applied_count", "paused_plan_target_id", "last_error_code", "creation_audit_log_id", "last_audit_log_id", "approved_by", "approval_actor_type", "approval_audit_log_id", "cycle_hold_active", "created_at", "updated_at"] as const;
const EXECUTION_TARGET_KEYS = ["id", "brand_id", "execution_id", "plan_target_id", "agent_id", "member_id", "points_before", "points_after", "delta_points", "state", "ledger_entry_id", "audit_log_id", "financial_version", "created_at", "applied_at"] as const;

function isObj(value: unknown): value is Obj { return value !== null && typeof value === "object" && !Array.isArray(value); }
function exact(value: Obj, keys: readonly string[]): boolean { const actual = Object.keys(value); return actual.length === keys.length && actual.every((key) => keys.includes(key)); }
function uuid(value: unknown): value is string { return typeof value === "string" && UUID.test(value); }
function version(value: unknown): value is number { return typeof value === "number" && Number.isSafeInteger(value) && value >= 1; }
function int64uint(value: unknown): value is string { if (typeof value !== "string" || !UINT.test(value)) return false; try { return BigInt(value) <= MAX_INT64; } catch { return false; } }
function int64sint(value: unknown): value is string { if (typeof value !== "string" || !SINT.test(value)) return false; try { const n = BigInt(value); return n >= MIN_INT64 && n <= MAX_INT64; } catch { return false; } }
function integerString(value: unknown): value is string { return typeof value === "string" && UINT.test(value); }
function signedIntegerString(value: unknown): value is string { return typeof value === "string" && SINT.test(value); }
function nullableUUID(value: unknown): boolean { return value === null || uuid(value); }
function nullableCode(value: unknown): boolean { return value === null || (typeof value === "string" && value.length > 0 && value.length <= 200); }
function dateTime(value: unknown): value is string {
  if (typeof value !== "string" || !DATE_TIME.test(value) || !Number.isFinite(Date.parse(value))) return false;
  const [date, rawTime] = value.split("T"), [year, month, day] = date.split("-").map(Number);
  const m = /^(\d{2}):(\d{2}):(\d{2})(?:\.\d{1,9})?(Z|([+-])(\d{2}):(\d{2}))$/.exec(rawTime);
  if (!m) return false;
  const last = new Date(0); last.setUTCFullYear(year, month, 0);
  const oh = Number(m[5] ?? 0), om = Number(m[6] ?? 0);
  return year >= 1 && month >= 1 && month <= 12 && day >= 1 && day <= last.getUTCDate() && Number(m[1]) <= 23 && Number(m[2]) <= 59 && Number(m[3]) <= 59 && om <= 59 && (m[4] === "Z" || oh < 14 || (oh === 14 && om === 0));
}
function instantNs(value: string): bigint {
  const m = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})$/.exec(value)!;
  return BigInt(Math.floor(Date.parse(`${m[1]}${m[3]}`) / 1000)) * 1_000_000_000n + BigInt((m[2] ?? "").padEnd(9, "0"));
}
function validReason(value: unknown): value is string {
  if (typeof value !== "string" || !value || value.trim() !== value || /[\u0000-\u001f\u007f-\u009f]/u.test(value)) return false;
  for (let i = 0; i < value.length; i++) { const c = value.charCodeAt(i); if (c >= 0xd800 && c <= 0xdbff) { const n = value.charCodeAt(++i); if (!(n >= 0xdc00 && n <= 0xdfff)) return false; } else if (c >= 0xdc00 && c <= 0xdfff) return false; }
  return new TextEncoder().encode(value).length <= 500;
}
function validKey(value: string): boolean { return typeof value === "string" && /^[A-Za-z0-9._:-]{8,128}$/.test(value); }
function validPageArgs(limit: number, offset: number): boolean { return Number.isSafeInteger(limit) && limit >= 1 && limit <= 100 && Number.isSafeInteger(offset) && offset >= 0 && offset <= 1_000_000; }
function invalidInput(): never { throw new AdminApiError("佣金更正请求参数无效。", 0, "INVALID_INPUT"); }
function invalidResponse(): never { throw new AdminApiError("佣金更正服务响应格式无效。", 502, "INVALID_RESPONSE"); }
function reasonBody(value: unknown, policy: boolean): boolean {
  return isObj(value) && exact(value, policy ? ["version", "enabled", "reason"] : ["version", "reason"]) && version(value.version) && value.version < Number.MAX_SAFE_INTEGER &&
    (!policy || typeof value.enabled === "boolean") && validReason(value.reason);
}
function validPolicy(value: unknown, brand: string): value is CommissionCorrectionPolicy {
  if (!isObj(value) || !exact(value, ["brand_id", "version", "enabled", "audit_log_id", "updated_at"]) || value.brand_id !== brand || !uuid(value.brand_id) || !version(value.version) || typeof value.enabled !== "boolean" || !dateTime(value.updated_at)) return false;
  return value.version === 1 ? value.enabled === false && value.audit_log_id === "" : uuid(value.audit_log_id);
}
function nullableAggregate(value: unknown): value is string | null { return value === null || integerString(value); }
function validPlan(value: unknown, brand: string): value is CorrectionPlan {
  if (!isObj(value) || !exact(value, PLAN_KEYS) || value.brand_id !== brand || !uuid(value.id) || !uuid(value.brand_id) || !uuid(value.cycle_id) || !uuid(value.payment_id) || !uuid(value.run_id) || !["planning", "ready", "blocked", "failed", "stale"].includes(String(value.state)) || !["manual", "automatic", "mixed", "none"].includes(String(value.payout_mode)) || !version(value.version) || !integerString(value.evidence_epoch) || BigInt(value.evidence_epoch) > MAX_INT64 || !integerString(value.before_points) || !integerString(value.calculated_points) || !nullableAggregate(value.credit_points) || !nullableAggregate(value.debit_points) || !(value.net_points === null || signedIntegerString(value.net_points)) || !integerString(value.target_count) || !integerString(value.planned_count) || BigInt(value.target_count) > MAX_TARGETS || BigInt(value.planned_count) > BigInt(value.target_count) || !uuid(value.creation_audit_log_id) || !uuid(value.last_audit_log_id) || !nullableCode(value.last_error_code) || !dateTime(value.created_at) || !dateTime(value.updated_at) || instantNsSafe(value.updated_at) < instantNsSafe(value.created_at)) return false;
  const resolved = value.credit_points !== null && value.debit_points !== null && value.net_points !== null;
  if ([value.credit_points, value.debit_points, value.net_points].some((v) => v === null) && ![value.credit_points, value.debit_points, value.net_points].every((v) => v === null)) return false;
  if (resolved) {
    const delta = BigInt(value.calculated_points) - BigInt(value.before_points);
    if (BigInt(value.net_points as string) !== delta || BigInt(value.net_points as string) !== BigInt(value.credit_points as string) - BigInt(value.debit_points as string)) return false;
  }
  if (value.state === "ready" && (value.last_error_code !== null || BigInt(value.planned_count) !== BigInt(value.target_count) || !resolved)) return false;
  if (value.state === "planning" && value.last_error_code !== null) return false;
  if ((value.state === "failed" || value.state === "blocked") !== (value.last_error_code !== null)) return false;
  if (value.state === "blocked" && (resolved || value.planned_count !== "0" || value.last_error_code !== "COMMISSION_CORRECTION_MANUAL_POLICY_UNRESOLVED")) return false;
  if (!resolved && (value.state !== "blocked" && value.state !== "stale" || value.planned_count !== "0")) return false;
  return true;
}
function instantNsSafe(value: string): bigint { return instantNs(value); }
function nullableSafeVersion(value: unknown): boolean { return value === null || version(value); }
function validPlanTarget(value: unknown, brand: string, planID: string): value is CorrectionPlanTarget {
  if (!isObj(value) || !exact(value, PLAN_TARGET_KEYS) || value.brand_id !== brand || value.plan_id !== planID || !uuid(value.id) || !uuid(value.brand_id) || !uuid(value.plan_id) || !uuid(value.agent_id) || !uuid(value.member_id) || !nullableUUID(value.original_target_id) || !nullableUUID(value.earning_id) || !nullableSafeVersion(value.adjustment_version) || !nullableUUID(value.previous_correction_target_id) || !nullableSafeVersion(value.financial_version) || !int64uint(value.points_before) || !int64uint(value.points_after) || !int64sint(value.delta_points) || BigInt(value.delta_points) !== BigInt(value.points_after) - BigInt(value.points_before) || !uuid(value.creation_audit_log_id) || !dateTime(value.created_at)) return false;
  const hasOriginal = value.original_target_id !== null, hasPrevious = value.previous_correction_target_id !== null;
  if (hasOriginal !== (value.adjustment_version !== null) || hasPrevious !== (value.financial_version !== null) || !(hasOriginal || value.earning_id !== null || hasPrevious)) return false;
  if (BigInt(value.points_before) > 0n && !hasOriginal && !hasPrevious) return false;
  return true;
}
function validExecution(value: unknown, brand: string): value is CorrectionExecution {
  if (!isObj(value) || !exact(value, EXECUTION_KEYS) || value.brand_id !== brand || !uuid(value.id) || !uuid(value.brand_id) || !uuid(value.cycle_id) || !uuid(value.plan_id) || !uuid(value.run_id) || !["manual", "automatic", "mixed", "none"].includes(String(value.payout_mode)) || !["awaiting_approval", "applying", "completed", "paused", "failed", "stale"].includes(String(value.state)) || !version(value.version) || !version(value.plan_version) || !integerString(value.evidence_epoch) || BigInt(value.evidence_epoch) > MAX_INT64 || !integerString(value.credit_points) || !integerString(value.debit_points) || !signedIntegerString(value.net_points) || !integerString(value.applied_credit_points) || !integerString(value.applied_debit_points) || !integerString(value.target_count) || !integerString(value.applied_count) || BigInt(value.target_count) > MAX_TARGETS || BigInt(value.applied_count) > BigInt(value.target_count) || !nullableUUID(value.paused_plan_target_id) || !nullableCode(value.last_error_code) || !uuid(value.creation_audit_log_id) || !uuid(value.last_audit_log_id) || !nullableUUID(value.approved_by) || !(value.approval_actor_type === null || ["admin", "system"].includes(String(value.approval_actor_type))) || !nullableUUID(value.approval_audit_log_id) || typeof value.cycle_hold_active !== "boolean" || !dateTime(value.created_at) || !dateTime(value.updated_at) || instantNs(value.updated_at) < instantNs(value.created_at)) return false;
  if (BigInt(value.net_points) !== BigInt(value.credit_points) - BigInt(value.debit_points) || BigInt(value.applied_credit_points) > BigInt(value.credit_points) || BigInt(value.applied_debit_points) > BigInt(value.debit_points)) return false;
  const noApproval = value.approved_by === null && value.approval_actor_type === null && value.approval_audit_log_id === null;
  if (value.state === "awaiting_approval") return ["manual", "mixed"].includes(String(value.payout_mode)) && noApproval && value.last_error_code === null && value.paused_plan_target_id === null && value.applied_count === "0" && value.applied_credit_points === "0" && value.applied_debit_points === "0";
  if (!noApproval && (value.approval_actor_type === null || value.approval_audit_log_id === null || (value.approval_actor_type === "admin" ? value.approved_by === null : value.approved_by !== null))) return false;
  if (noApproval && (value.state !== "stale" || !["manual", "mixed"].includes(String(value.payout_mode)) || value.applied_count !== "0" || value.applied_credit_points !== "0" || value.applied_debit_points !== "0")) return false;
  if (value.state === "paused") return value.cycle_hold_active &&
    (value.last_error_code === "COMMISSION_CORRECTION_AVAILABLE_INSUFFICIENT" && uuid(value.paused_plan_target_id) && BigInt(value.applied_count) < BigInt(value.target_count) ||
      value.last_error_code === "COMMISSION_CORRECTION_CYCLE_HELD" && value.paused_plan_target_id === null);
  if (value.paused_plan_target_id !== null) return false;
  if (value.state === "failed") return value.last_error_code !== null;
  if (value.state === "applying" || value.state === "completed" || value.state === "stale") {
    if (value.last_error_code !== null) return false;
  }
  if (value.state === "completed") return value.applied_count === value.target_count && value.applied_credit_points === value.credit_points && value.applied_debit_points === value.debit_points;
  return true;
}
function validExecutionTarget(value: unknown, brand: string, executionID: string): value is CorrectionExecutionTarget {
  if (!isObj(value) || !exact(value, EXECUTION_TARGET_KEYS) || value.brand_id !== brand || value.execution_id !== executionID || !uuid(value.id) || !uuid(value.brand_id) || !uuid(value.execution_id) || !uuid(value.plan_target_id) || !uuid(value.agent_id) || !uuid(value.member_id) || !int64uint(value.points_before) || !int64uint(value.points_after) || !int64sint(value.delta_points) || BigInt(value.delta_points) !== BigInt(value.points_after) - BigInt(value.points_before) || !["pending", "applied"].includes(String(value.state)) || !nullableUUID(value.ledger_entry_id) || !nullableUUID(value.audit_log_id) || !nullableSafeVersion(value.financial_version) || !dateTime(value.created_at) || !(value.applied_at === null || dateTime(value.applied_at))) return false;
  if (value.state === "pending") return value.ledger_entry_id === null && value.audit_log_id === null && value.financial_version === null && value.applied_at === null;
  return value.audit_log_id !== null && typeof value.financial_version === "number" && value.financial_version >= 1 && value.applied_at !== null && instantNs(value.applied_at) >= instantNs(value.created_at) && (BigInt(value.delta_points) === 0n ? value.ledger_entry_id === null : value.ledger_entry_id !== null);
}
function validPage<T>(value: unknown, brand: string, limit: number, offset: number, expectedKeys: readonly string[], parentID: string | null, validate: (item: unknown) => item is T, compare?: (left: T, right: T) => number): value is Obj & { items: T[]; total_count: string; limit: number; offset: number } {
  if (!isObj(value) || !exact(value, expectedKeys) || value.brand_id !== brand || !uuid(value.brand_id) || (parentID !== null && (value.plan_id ?? value.execution_id) !== parentID) || !Array.isArray(value.items) || value.items.length > limit || !integerString(value.total_count) || value.limit !== limit || value.offset !== offset || !value.items.every(validate)) return false;
  const items = value.items as Obj[];
  const total = BigInt(value.total_count as string), remaining = total > BigInt(offset) ? total - BigInt(offset) : 0n;
  const expectedLength = Number(remaining < BigInt(limit) ? remaining : BigInt(limit));
  const unique = new Set(items.map((item) => item.id)).size === items.length && items.length === expectedLength;
  const ordered = !compare || (value.items as T[]).every((item, index, all) => index === 0 || compare(all[index - 1], item) <= 0);
  return unique && ordered && (items.length === 0 || BigInt(value.total_count as string) >= BigInt(offset) + BigInt(items.length));
}
function compareNewest<T extends { created_at: string; id: string }>(left: T, right: T): number {
  const time = instantNs(right.created_at) - instantNs(left.created_at);
  if (time !== 0n) return time < 0n ? -1 : 1;
  return left.id === right.id ? 0 : left.id > right.id ? -1 : 1;
}
function compareTarget<T extends { agent_id: string; id: string }>(left: T, right: T): number {
  return left.agent_id === right.agent_id ? left.id < right.id ? -1 : left.id > right.id ? 1 : 0 : left.agent_id < right.agent_id ? -1 : 1;
}
function validBrand(brand: string): void { if (!uuid(brand)) invalidInput(); }
function safeErrorCode(value: unknown): string | undefined {
  if (typeof value !== "string") return undefined;
  if (["COMMISSION_CORRECTION_BUSY", "COMMISSION_CORRECTION_INPUT_INVALID", "COMMISSION_CORRECTION_NOT_FOUND", "COMMISSION_CORRECTION_STATE_CONFLICT", "COMMISSION_CORRECTION_VERSION_CONFLICT", "COMMISSION_CORRECTION_EVIDENCE_CONFLICT", "COMMISSION_CORRECTION_LIMIT"].includes(value)) return value;
  return /^(?:AUTH_[A-Z0-9_]{1,80}|PERMISSION_DENIED|CSRF_REJECTED|CONTENT_TYPE_INVALID|REQUEST_INVALID)$/.test(value) ? value : undefined;
}

export function commissionCorrectionPermissions(account: AdminAccount, brand: string) {
  const member = Array.isArray(account.brand_ids) && account.brand_ids.includes(brand);
  const brandGrants = new Set(account.permissions_by_brand === undefined ? (member ? account.permissions ?? [] : []) : account.permissions_by_brand[brand] ?? []);
  const platformGrants = new Set(account.platform_permissions ?? account.permissions ?? []);
  const view = uuid(account.id) && uuid(brand) && (account.super_admin ? platformGrants.has("commission.view.platform") : (member && brandGrants.has("commission.view.brand")) || platformGrants.has("commission.view.platform"));
  const writesAllowed = Boolean(view && member && !account.super_admin);
  return {
    view,
    policyWrite: Boolean(writesAllowed && brandGrants.has("commission_correction_policy.write.brand")),
    planRetry: Boolean(writesAllowed && brandGrants.has("commission_correction.retry.brand")),
    approve: Boolean(writesAllowed && brandGrants.has("commission_correction.approve.brand")),
    continue: Boolean(writesAllowed && brandGrants.has("commission_correction.continue.brand")),
    retry: Boolean(writesAllowed && brandGrants.has("commission_correction.execute_retry.brand")),
  };
}

export function createCommissionCorrectionsApi(fetcher: typeof fetch = fetch, actorId?: string) {
  const capturedActor = actorId;
  async function request(brand: string, path: string, method: "GET" | "PUT" | "POST" = "GET", body?: unknown, key?: string): Promise<unknown> {
    const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brand });
    if (method !== "GET") {
      if (!uuid(capturedActor)) invalidInput();
      if (!validKey(key ?? "")) invalidInput();
      headers.set("Content-Type", "application/json"); headers.set("Idempotency-Key", key!);
      headers.set("X-Commission-Correction-Actor-ID", capturedActor);
    }
    let response: Response;
    try { response = await fetcher(path, { method, credentials: "same-origin", headers, ...(body === undefined ? {} : { body: JSON.stringify(body) }) }); }
    catch (cause) { throw new AdminApiError(cause instanceof Error ? cause.message : "Network request failed", 0, "NETWORK_ERROR"); }
    let decoded: unknown;
    try { decoded = await response.json(); } catch { throw new AdminApiError(response.ok ? "Invalid server response" : `Request failed (${response.status})`, response.ok ? 502 : response.status, response.ok ? "INVALID_RESPONSE" : undefined); }
    if (!response.ok || !isObj(decoded) || decoded.success !== true || decoded.data === undefined) {
      const detail = isObj((decoded as Envelope | null)?.error) ? (decoded as Envelope).error as Obj : undefined;
      if (!response.ok && detail && safeErrorCode(detail.code) === undefined) throw new AdminApiError(`Request failed (${response.status})`, response.status, "API_ERROR");
      throw new AdminApiError(typeof (decoded as Envelope | null)?.error === "string" ? (decoded as Envelope).error as string : (typeof detail?.message === "string" ? detail.message : `Request failed (${response.status})`), response.ok ? 502 : response.status, safeErrorCode(detail?.code));
    }
    return decoded.data;
  }
  function query(limit: number, offset: number): string {
    if (!validPageArgs(limit, offset)) invalidInput();
    return new URLSearchParams({ limit: String(limit), offset: String(offset) }).toString();
  }
  return {
    async getPolicy(brand: string): Promise<CommissionCorrectionPolicy> { validBrand(brand); const data = await request(brand, POLICY); if (!validPolicy(data, brand)) invalidResponse(); return data; },
    async updatePolicy(brand: string, body: Readonly<CommissionCorrectionPolicyBody>, key: string): Promise<CommissionCorrectionPolicy> {
      validBrand(brand); if (!reasonBody(body, true) || !validKey(key)) invalidInput(); const snapshot = { version: body.version, enabled: body.enabled, reason: body.reason };
      const data = await request(brand, POLICY, "PUT", snapshot, key);
      if (!validPolicy(data, brand) || data.version !== snapshot.version + 1 || data.enabled !== snapshot.enabled || !uuid(data.audit_log_id)) invalidResponse(); return data;
    },
    async listPlans(brand: string, limit = 20, offset = 0): Promise<CorrectionPlanPage> {
      validBrand(brand); const q = query(limit, offset), data = await request(brand, `${PLANS}?${q}`);
      if (!validPage(data, brand, limit, offset, ["brand_id", "items", "total_count", "limit", "offset"], null, (v): v is CorrectionPlan => validPlan(v, brand), compareNewest)) invalidResponse(); return data as unknown as CorrectionPlanPage;
    },
    async readPlan(brand: string, id: string): Promise<CorrectionPlan> { validBrand(brand); if (!uuid(id)) invalidInput(); const data = await request(brand, `${PLANS}/${encodeURIComponent(id)}`); if (!validPlan(data, brand) || data.id !== id) invalidResponse(); return data; },
    async listPlanTargets(brand: string, id: string, limit = 20, offset = 0): Promise<CorrectionPlanTargetPage> {
      validBrand(brand); if (!uuid(id)) invalidInput(); const q = query(limit, offset), data = await request(brand, `${PLANS}/${encodeURIComponent(id)}/targets?${q}`);
      if (!validPage(data, brand, limit, offset, ["brand_id", "plan_id", "items", "total_count", "limit", "offset"], id, (v): v is CorrectionPlanTarget => validPlanTarget(v, brand, id), compareTarget)) invalidResponse(); return data as unknown as CorrectionPlanTargetPage;
    },
    async retryPlan(brand: string, id: string, body: Readonly<CommissionCorrectionActionBody>, key: string): Promise<CorrectionPlan> {
      validBrand(brand); if (!uuid(id) || !reasonBody(body, false) || !validKey(key)) invalidInput(); const snapshot = { version: body.version, reason: body.reason };
      const data = await request(brand, `${PLANS}/${encodeURIComponent(id)}/retry`, "POST", snapshot, key);
      if (!validPlan(data, brand) || data.id !== id || data.version !== snapshot.version + 1 || data.state !== "planning" || data.last_error_code !== null) invalidResponse(); return data;
    },
    async listExecutions(brand: string, limit = 20, offset = 0): Promise<CorrectionExecutionPage> {
      validBrand(brand); const q = query(limit, offset), data = await request(brand, `${EXECUTIONS}?${q}`);
      if (!validPage(data, brand, limit, offset, ["brand_id", "items", "total_count", "limit", "offset"], null, (v): v is CorrectionExecution => validExecution(v, brand), compareNewest)) invalidResponse(); return data as unknown as CorrectionExecutionPage;
    },
    async readExecution(brand: string, id: string): Promise<CorrectionExecution> { validBrand(brand); if (!uuid(id)) invalidInput(); const data = await request(brand, `${EXECUTIONS}/${encodeURIComponent(id)}`); if (!validExecution(data, brand) || data.id !== id) invalidResponse(); return data; },
    async listExecutionTargets(brand: string, id: string, limit = 20, offset = 0): Promise<CorrectionExecutionTargetPage> {
      validBrand(brand); if (!uuid(id)) invalidInput(); const q = query(limit, offset), data = await request(brand, `${EXECUTIONS}/${encodeURIComponent(id)}/targets?${q}`);
      if (!validPage(data, brand, limit, offset, ["brand_id", "execution_id", "items", "total_count", "limit", "offset"], id, (v): v is CorrectionExecutionTarget => validExecutionTarget(v, brand, id), compareTarget)) invalidResponse(); return data as unknown as CorrectionExecutionTargetPage;
    },
    async approve(brand: string, id: string, body: Readonly<CommissionCorrectionActionBody>, key: string): Promise<CorrectionExecution> { return action("approve", brand, id, body, key); },
    async continue(brand: string, id: string, body: Readonly<CommissionCorrectionActionBody>, key: string): Promise<CorrectionExecution> { return action("continue", brand, id, body, key); },
    async retry(brand: string, id: string, body: Readonly<CommissionCorrectionActionBody>, key: string): Promise<CorrectionExecution> { return action("retry", brand, id, body, key); },
  };
  async function action(name: "approve" | "continue" | "retry", brand: string, id: string, body: Readonly<CommissionCorrectionActionBody>, key: string): Promise<CorrectionExecution> {
    validBrand(brand); if (!uuid(id) || !reasonBody(body, false) || !validKey(key)) invalidInput(); const snapshot = { version: body.version, reason: body.reason };
    const data = await request(brand, `${EXECUTIONS}/${encodeURIComponent(id)}/${name}`, "POST", snapshot, key);
    if (!validExecution(data, brand) || data.id !== id || data.version !== snapshot.version + 1 ||
      (name === "approve" ? data.state !== "applying" && !(data.state === "paused" && data.cycle_hold_active) : data.state !== "applying")) invalidResponse();
    if (name === "approve" && (!(["manual", "mixed"] as string[]).includes(data.payout_mode) || data.approved_by !== capturedActor || data.approval_actor_type !== "admin")) invalidResponse();
    return data;
  }
}
