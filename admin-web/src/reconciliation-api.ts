import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import type { FinanceAccount, SourceBuckets, WalletSource, WalletState } from "./finance-api";
import type { RepairPreview } from "./repair-api";
import { normalizeSourceBuckets, WALLET_SOURCES } from "@lottery/shared";

const BASE = "/api/v1/admin/reconciliations";
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const SAFE_CODE_RE = /^[A-Z][A-Z0-9_]{0,63}$/;
const MAX_PAGE_SIZE = 100;
const MAX_OFFSET = 1_000_000;
const SOURCES: readonly WalletSource[] = WALLET_SOURCES;
const STATES: readonly WalletState[] = ["available", "manual_frozen", "system_frozen", "withdrawal"];

export type ReconciliationJobState = "pending" | "running" | "completed" | "failed";
export type ReconciliationTargetState = "pending" | "checked" | "failed";
export type ReconciliationOutcome = "pending" | "failed" | "consistent" | "repairable" | "corrupt";
export type ReconciliationCheckScope = "wallet" | "wallet_and_business";
export const BUSINESS_FAMILIES = ["bet", "commission", "commission_adjustment", "commission_correction", "manual", "prize", "prize_reversal", "recharge", "refund", "reward", "unknown", "withdrawal"] as const;
export type BusinessFamily = typeof BUSINESS_FAMILIES[number];
export interface BusinessCoverage {
  family: BusinessFamily; ledger_entry_count: string; business_reference_count: string; issue_count: string;
}
export interface BusinessIssue {
  code: "UNSUPPORTED_LEDGER_TYPE" | "MISSING_BUSINESS_RECORD" | "INVALID_BUSINESS_BINDING" | "MISSING_LEDGER_ENTRY" | "DUPLICATE_BUSINESS_BINDING" | "INVALID_MANUAL_AUDIT";
  entry_type: string | null; ledger_entry_id: string | null; resource_type: BusinessFamily | "ledger"; resource_id: string | null;
}
export interface BusinessPreview {
  account_id: string; member_id: string; account_version: number; ledger_entry_count: string;
  business_reference_count: string; issue_count: string; issues_truncated: boolean; consistent: boolean;
  fingerprint: string; issues: BusinessIssue[]; coverage: BusinessCoverage[];
}
export interface ReconciliationJob {
  id: string; brand_id: string; state: ReconciliationJobState; version: number;
  target_count: string; checked_count: string; consistent_count: string; repairable_count: string;
  corrupt_count: string; failed_count: string; pending_count: string;
  created_by: string; reason: string; created_at: string; started_at: string | null;
  completed_at: string | null; last_error_code: "CHECK_FAILED" | null; can_retry: boolean;
  creation_audit_log_id: string; check_scope: ReconciliationCheckScope;
}
export interface ReconciliationJobPage {
  brand_id: string; items: ReconciliationJob[]; total_count: string; limit: number; offset: number;
}
export interface ReconciliationTarget {
  id: string; brand_id: string; job_id: string; account_id: string; member_id: string;
  state: ReconciliationTargetState; outcome: Exclude<ReconciliationOutcome, "pending" | "failed"> | null;
  preview: RepairPreview | null; attempt_count: number; error_code: "CHECK_FAILED" | null;
  checked_at: string | null; audit_log_id: string | null; check_scope: ReconciliationCheckScope;
  business_preview: BusinessPreview | null;
}
export interface ReconciliationTargetPage {
  brand_id: string; job_id: string; items: ReconciliationTarget[]; total_count: string;
  limit: number; offset: number; outcome: ReconciliationOutcome | null;
}
export interface ReconciliationCreateBody { reason: string; check_scope: ReconciliationCheckScope }
export interface ReconciliationRetryBody { version: number; reason: string }
export interface ReconciliationApi {
  list(brandId: string, limit?: number, offset?: number): Promise<ReconciliationJobPage>;
  read(brandId: string, jobId: string): Promise<ReconciliationJob>;
  targets(brandId: string, jobId: string, outcome?: ReconciliationOutcome | null, limit?: number, offset?: number, checkScope?: ReconciliationCheckScope): Promise<ReconciliationTargetPage>;
  create(brandId: string, body: ReconciliationCreateBody, key?: string, expectedAccountId?: string): Promise<ReconciliationJob>;
  retry(brandId: string, jobId: string, body: ReconciliationRetryBody, key: string, previous?: ReconciliationJob): Promise<ReconciliationJob>;
}

export function reconciliationPermissions(account: AdminAccount | FinanceAccount, brandId: string): { view: boolean; run: boolean; retry: boolean } {
  const scoped = validUuid(account.id) && validUuid(brandId);
  const member = scoped && (account.brand_ids ?? []).some((id) => sameUuid(id, brandId));
  const brand = new Set(account.permissions_by_brand === undefined ? account.permissions ?? [] : account.permissions_by_brand[brandId] ?? []);
  const platform = new Set(account.platform_permissions ?? account.permissions ?? []);
  const view = scoped && (platform.has("wallet.view.platform") || (member && brand.has("wallet.view.brand")));
  const write = member && !account.super_admin && brand.has("wallet.reconcile.brand") && view;
  return { view, run: write, retry: write };
}

function isRecord(value: unknown): value is Record<string, unknown> { return Boolean(value && typeof value === "object" && !Array.isArray(value)); }
function exact(value: Record<string, unknown>, fields: readonly string[]): boolean {
  const got = Object.keys(value).sort(), want = [...fields].sort();
  return got.length === want.length && got.every((key, index) => key === want[index]);
}
function validUuid(value: unknown): value is string { return typeof value === "string" && UUID_RE.test(value); }
function sameUuid(left: unknown, right: string): boolean { return typeof left === "string" && left.toLowerCase() === right.toLowerCase(); }
function positiveInt(value: unknown): value is number { return Number.isSafeInteger(value) && Number(value) > 0; }
function nonnegativeInt(value: unknown): value is number { return Number.isSafeInteger(value) && Number(value) >= 0; }
function decimal(value: unknown): value is string { return typeof value === "string" && /^(0|[1-9][0-9]*)$/.test(value); }
function scopeValue(value: unknown): value is ReconciliationCheckScope { return value === "wallet" || value === "wallet_and_business"; }
function int64(value: unknown): value is string {
  if (typeof value !== "string" || !/^(0|-?[1-9][0-9]*)$/.test(value)) return false;
  try { const n = BigInt(value); return n >= -9223372036854775808n && n <= 9223372036854775807n; } catch { return false; }
}
function validDate(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|([+-])(\d{2}):(\d{2}))$/.exec(value);
  if (!m) return false;
  const [, y, mo, d, h, mi, s, , , , oh, om] = m;
  const year = Number(y), month = Number(mo), day = Number(d);
  const days = month === 2 ? (year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28) : [4, 6, 9, 11].includes(month) ? 30 : 31;
  return year > 0 && month >= 1 && month <= 12 && day >= 1 && day <= days && Number(h) <= 23 && Number(mi) <= 59 && Number(s) <= 59 &&
    (oh === undefined || Number(oh) < 14 || (Number(oh) === 14 && Number(om) === 0)) && (oh === undefined || Number(om) <= 59) && Number.isFinite(Date.parse(value));
}
function validStoredReason(value: unknown): value is string {
  return typeof value === "string" && value.trim() === value && value.length > 0 && new TextEncoder().encode(value).length <= 500 &&
    !/[\u0000\r\n]/.test(value) && !hasLoneSurrogate(value);
}
function safeReason(value: unknown): value is string {
  return validStoredReason(value) && !/[\u0000-\u001f\u007f-\u009f]/u.test(value);
}
function hasLoneSurrogate(value: string): boolean {
  for (let i = 0; i < value.length; i++) {
    const code = value.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff) {
      const next = value.charCodeAt(i + 1);
      if (!(next >= 0xdc00 && next <= 0xdfff)) return true;
      i++;
    } else if (code >= 0xdc00 && code <= 0xdfff) return true;
  }
  return false;
}
function normalizedBuckets(value: unknown, sparse: boolean, allowNegative = sparse): Record<string, Record<string, string>> | null {
  const complete = normalizeSourceBuckets(value, allowNegative);
  if (complete) return complete;
  if (!sparse || !isRecord(value) || Object.keys(value).some((key) => !(SOURCES as readonly string[]).includes(key))) return null;
  if (!Object.entries(value).every(([source, buckets]) => (SOURCES as readonly string[]).includes(source) && isRecord(buckets) &&
    Object.keys(buckets).every((key) => (STATES as readonly string[]).includes(key)) &&
    Object.values(buckets).every((amount) => int64(amount) && (allowNegative || !amount.startsWith("-"))))) return null;
  return value as Record<string, Record<string, string>>;
}
function validPreview(value: unknown, target: { account_id: string; member_id: string }): value is RepairPreview {
  if (!isRecord(value) || !exact(value, ["account_id", "member_id", "version", "ledger_version", "actual", "expected", "repairable", "consistent", "issues", "token"]) ||
    !sameUuid(value.account_id, target.account_id) || !sameUuid(value.member_id, target.member_id) || !nonnegativeInt(value.version) || !nonnegativeInt(value.ledger_version) ||
    typeof value.repairable !== "boolean" || typeof value.consistent !== "boolean" ||
    !Array.isArray(value.issues) || !value.issues.every((issue) => typeof issue === "string") ||
    typeof value.token !== "string" || !/^[0-9a-f]{64}$/.test(value.token) || (value.repairable && value.consistent)) return false;
  const actual = normalizedBuckets(value.actual, true, true);
  const expected = normalizedBuckets(value.expected, false, false);
  if (!actual || !expected) return false;
  value.actual = actual;
  value.expected = expected;
  if(value.consistent){
    const completeActual = normalizeSourceBuckets(actual, false);
    const completeExpected = normalizeSourceBuckets(expected, false);
    if(value.issues.length!==0||value.version!==value.ledger_version||!completeActual||!completeExpected)return false;
    if(!SOURCES.every(source=>STATES.every(state=>completeActual[source][state]===completeExpected[source][state])))return false;
  }else if(value.issues.length===0)return false;
  return true;
}
const JOB_FIELDS = ["id", "brand_id", "state", "version", "target_count", "checked_count", "consistent_count", "repairable_count", "corrupt_count", "failed_count", "pending_count", "created_by", "reason", "created_at", "started_at", "completed_at", "last_error_code", "can_retry", "creation_audit_log_id", "check_scope"] as const;
function validJob(value: unknown, brand: string): value is ReconciliationJob {
  if (!isRecord(value) || !exact(value, JOB_FIELDS) || !scopeValue(value.check_scope) || !validUuid(value.id) || !sameUuid(value.brand_id, brand) ||
    !["pending", "running", "completed", "failed"].includes(String(value.state)) || !positiveInt(value.version) ||
    !["target_count", "checked_count", "consistent_count", "repairable_count", "corrupt_count", "failed_count", "pending_count"].every((key) => decimal(value[key])) ||
    !validUuid(value.created_by) || !validStoredReason(value.reason) || !validDate(value.created_at) || !(value.started_at === null || validDate(value.started_at)) ||
    !(value.completed_at === null || validDate(value.completed_at)) || !(value.last_error_code === null || value.last_error_code === "CHECK_FAILED") ||
    typeof value.can_retry !== "boolean" || !validUuid(value.creation_audit_log_id)) return false;
  const target = BigInt(value.target_count as string), checked = BigInt(value.checked_count as string), consistent = BigInt(value.consistent_count as string);
  const repairable = BigInt(value.repairable_count as string), corrupt = BigInt(value.corrupt_count as string), failed = BigInt(value.failed_count as string), pending = BigInt(value.pending_count as string);
  if (target > 100_000n || checked + failed + pending !== target || consistent + repairable + corrupt !== checked || failed > target ||
      (value.can_retry && value.state !== "failed") ||
      (value.state === "running" && value.started_at === null) ||
      (value.state === "completed" && (checked !== target || pending !== 0n || failed !== 0n || value.started_at===null || value.completed_at===null || value.last_error_code !== null || value.can_retry)) ||
      (value.state === "failed" && (failed===0n||value.last_error_code!=="CHECK_FAILED"||!value.can_retry||value.completed_at!==null)) ||
      (value.state !== "failed" && (value.last_error_code !== null||failed!==0n)) ||
      (value.state !== "completed" && value.completed_at!==null)) return false;
  if(value.started_at!==null&&Date.parse(value.started_at as string)<Date.parse(value.created_at as string))return false;
  if(value.completed_at!==null&&Date.parse(value.completed_at as string)<Date.parse(value.started_at as string))return false;
  return true;
}
const TARGET_FIELDS = ["id", "brand_id", "job_id", "account_id", "member_id", "state", "outcome", "preview", "attempt_count", "error_code", "checked_at", "audit_log_id", "check_scope", "business_preview"] as const;
const BUSINESS_ISSUE_CODES = ["UNSUPPORTED_LEDGER_TYPE", "MISSING_BUSINESS_RECORD", "INVALID_BUSINESS_BINDING", "MISSING_LEDGER_ENTRY", "DUPLICATE_BUSINESS_BINDING", "INVALID_MANUAL_AUDIT"] as const;
function validBusinessPreview(value: unknown, target: { account_id: string; member_id: string; preview: RepairPreview }): value is BusinessPreview {
  if (!isRecord(value) || !exact(value, ["account_id", "member_id", "account_version", "ledger_entry_count", "business_reference_count", "issue_count", "issues_truncated", "consistent", "fingerprint", "issues", "coverage"]) ||
    !sameUuid(value.account_id, target.account_id) || !sameUuid(value.member_id, target.member_id) || !nonnegativeInt(value.account_version) ||
    value.account_version !== target.preview.version || !decimal(value.ledger_entry_count) || value.ledger_entry_count !== String(target.preview.ledger_version) ||
    !decimal(value.business_reference_count) || !decimal(value.issue_count) || typeof value.issues_truncated !== "boolean" || typeof value.consistent !== "boolean" ||
    typeof value.fingerprint !== "string" || !/^[0-9a-f]{64}$/.test(value.fingerprint) || !Array.isArray(value.issues) || value.issues.length > 100 ||
    !Array.isArray(value.coverage) || value.coverage.length !== BUSINESS_FAMILIES.length) return false;
  let ledgerTotal = 0n, referenceTotal = 0n, issueTotal = 0n;
  for (let index = 0; index < BUSINESS_FAMILIES.length; index++) {
    const row = value.coverage[index];
    if (!isRecord(row) || !exact(row, ["family", "ledger_entry_count", "business_reference_count", "issue_count"]) || row.family !== BUSINESS_FAMILIES[index] ||
      !decimal(row.ledger_entry_count) || !decimal(row.business_reference_count) || !decimal(row.issue_count)) return false;
    ledgerTotal += BigInt(row.ledger_entry_count as string);
    referenceTotal += BigInt(row.business_reference_count as string);
    issueTotal += BigInt(row.issue_count as string);
  }
  if (ledgerTotal !== BigInt(value.ledger_entry_count) || referenceTotal !== BigInt(value.business_reference_count) || issueTotal !== BigInt(value.issue_count)) return false;
  const issueCount = BigInt(value.issue_count);
  if (value.issues.length !== Number(issueCount < 100n ? issueCount : 100n) || value.issues_truncated !== (issueCount > 100n) || value.consistent !== (issueCount === 0n)) return false;
  for (const issue of value.issues) {
    if (!isRecord(issue) || !exact(issue, ["code", "entry_type", "ledger_entry_id", "resource_type", "resource_id"]) ||
      !(BUSINESS_ISSUE_CODES as readonly unknown[]).includes(issue.code) ||
      !(issue.entry_type === null || typeof issue.entry_type === "string" && /^[A-Za-z0-9_.:-]{1,200}$/.test(issue.entry_type)) ||
      !(issue.ledger_entry_id === null || validUuid(issue.ledger_entry_id)) ||
      !(issue.resource_type === "ledger" || (BUSINESS_FAMILIES as readonly unknown[]).includes(issue.resource_type)) ||
      !(issue.resource_id === null || validUuid(issue.resource_id))) return false;
  }
  return true;
}
function validTarget(value: unknown, brand: string, job: string): value is ReconciliationTarget {
  if (!isRecord(value) || !exact(value, TARGET_FIELDS) || !scopeValue(value.check_scope) || !validUuid(value.id) || !sameUuid(value.brand_id, brand) || !sameUuid(value.job_id, job) ||
    !validUuid(value.account_id) || !validUuid(value.member_id) || !["pending", "checked", "failed"].includes(String(value.state)) ||
    !(value.outcome === null || ["consistent", "repairable", "corrupt"].includes(String(value.outcome))) ||
    !(value.preview === null || validPreview(value.preview, { account_id: value.account_id as string, member_id: value.member_id as string })) ||
    !nonnegativeInt(value.attempt_count) || !(value.error_code === null || value.error_code === "CHECK_FAILED") ||
    !(value.checked_at === null || validDate(value.checked_at)) || !(value.audit_log_id === null || validUuid(value.audit_log_id))) return false;
  const fullScope = value.check_scope === "wallet_and_business";
  if (value.state === "pending") return value.outcome === null && value.preview === null && value.checked_at === null && value.audit_log_id === null && value.error_code === null && value.business_preview === null;
  if (value.state === "failed") return value.outcome === null && value.preview === null && value.checked_at === null && value.audit_log_id === null &&
    value.error_code === "CHECK_FAILED" && value.attempt_count >= 1 && value.business_preview === null;
  if (value.attempt_count < 1 || value.error_code !== null || value.checked_at === null || value.audit_log_id === null || value.preview === null) return false;
  if (!fullScope) return value.business_preview === null && value.outcome === (value.preview.consistent ? "consistent" : value.preview.repairable ? "repairable" : "corrupt");
  if (!validBusinessPreview(value.business_preview, { account_id: value.account_id as string, member_id: value.member_id as string, preview: value.preview })) return false;
  const walletOutcome = value.preview.consistent ? "consistent" : value.preview.repairable ? "repairable" : "corrupt";
  return value.outcome === (value.business_preview.consistent ? walletOutcome : "corrupt");
}
function invalidInput(): never { throw new AdminApiError("Reconciliation request parameters are invalid.", 0, "INVALID_INPUT"); }
function invalidResponse(uncertain = false): never { throw new AdminApiError("Reconciliation response is invalid.", uncertain ? 0 : 502, "INVALID_RESPONSE"); }
function pageArgs(limit: number, offset: number): void { if (!Number.isSafeInteger(limit) || limit < 1 || limit > MAX_PAGE_SIZE || !Number.isSafeInteger(offset) || offset < 0 || offset > MAX_OFFSET) invalidInput(); }
type Envelope = { success?: unknown; data?: unknown; error?: unknown };

export function createReconciliationApi(fetchImpl: typeof fetch = fetch): ReconciliationApi {
  async function request(path: string, brand: string, options: { method?: "GET" | "POST"; body?: unknown; key?: string; write?: boolean } = {}): Promise<unknown> {
    if (!validUuid(brand)) invalidInput();
    const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brand });
    if (options.body !== undefined) headers.set("Content-Type", "application/json");
    if (options.key !== undefined) headers.set("Idempotency-Key", options.key);
    let response: Response;
    try { response = await fetchImpl(path, { method: options.method ?? "GET", credentials: "same-origin", headers, ...(options.body === undefined ? {} : { body: JSON.stringify(options.body) }) }); }
    catch (cause) { throw new AdminApiError(cause instanceof Error ? cause.message : "Network request failed", 0, "NETWORK_ERROR"); }
    let envelope: Envelope;
    try { envelope = await response.json() as Envelope; }
    catch { if (!response.ok) throw new AdminApiError(`Request failed (${response.status})`, response.status); invalidResponse(Boolean(options.write)); }
    if (!response.ok || !isRecord(envelope) || envelope.success !== true || envelope.data === undefined) {
      const detail = isRecord(envelope?.error) ? envelope.error : undefined;
      const message = typeof envelope?.error === "string" ? envelope.error : typeof detail?.message === "string" ? detail.message : `Request failed (${response.status})`;
      if (!response.ok) throw new AdminApiError(message, response.status, typeof detail?.code === "string" ? detail.code : undefined);
      invalidResponse(Boolean(options.write));
    }
    return envelope.data;
  }
  return {
    async list(brandId, limit = 20, offset = 0) {
      pageArgs(limit, offset);
      const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
      const value = await request(`${BASE}?${query}`, brandId);
      if (!isRecord(value) || !exact(value, ["brand_id", "items", "total_count", "limit", "offset"]) || !sameUuid(value.brand_id, brandId) ||
        !Array.isArray(value.items) || value.items.length > limit || !value.items.every((item) => validJob(item, brandId)) || !decimal(value.total_count) || BigInt(value.total_count) < BigInt(value.items.length) ||
        !nonnegativeInt(value.limit) || value.limit !== limit || !nonnegativeInt(value.offset) || value.offset !== offset) invalidResponse();
      return value as unknown as ReconciliationJobPage;
    },
    async read(brandId, jobId) {
      if (!validUuid(jobId)) invalidInput();
      const value = await request(`${BASE}/${encodeURIComponent(jobId)}`, brandId);
      if (!validJob(value, brandId) || !sameUuid(value.id, jobId)) invalidResponse();
      return value;
    },
    async targets(brandId, jobId, outcome = null, limit = 20, offset = 0, checkScope) {
      if (!validUuid(jobId) || !(outcome === null || ["pending", "failed", "consistent", "repairable", "corrupt"].includes(outcome)) ||
        (checkScope !== undefined && !scopeValue(checkScope))) invalidInput();
      pageArgs(limit, offset);
      const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
      if (outcome) query.set("outcome", outcome);
      const value = await request(`${BASE}/${encodeURIComponent(jobId)}/targets?${query}`, brandId);
      if (!isRecord(value) || !exact(value, ["brand_id", "job_id", "items", "total_count", "limit", "offset", "outcome"]) || !sameUuid(value.brand_id, brandId) ||
        !sameUuid(value.job_id, jobId) || !Array.isArray(value.items) || value.items.length > limit || !value.items.every((item) => validTarget(item, brandId, jobId) && (checkScope === undefined || item.check_scope === checkScope)) ||
        !decimal(value.total_count) || BigInt(value.total_count) < BigInt(value.items.length) || !nonnegativeInt(value.limit) || value.limit !== limit || !nonnegativeInt(value.offset) || value.offset !== offset ||
        value.outcome !== outcome ||
        (outcome !== null && value.items.some((item) => {
          const target = item as ReconciliationTarget;
          return outcome === "pending" || outcome === "failed" ? target.state !== outcome : target.outcome !== outcome;
        }))) invalidResponse();
      return value as unknown as ReconciliationTargetPage;
    },
    async create(brandId, body, key = createIdempotencyKey(), expectedAccountId) {
      if (!isRecord(body) || !exact(body, ["reason", "check_scope"]) || !safeReason(body.reason) || !scopeValue(body.check_scope) || !/^[A-Za-z0-9_:.-]{8,128}$/.test(key) ||
        (expectedAccountId !== undefined && !validUuid(expectedAccountId))) invalidInput();
      const value = await request(BASE, brandId, { method: "POST", body, key, write: true });
      if (!validJob(value, brandId) || value.version !== 1 || value.state !== "pending" || value.created_by.toLowerCase() !== expectedAccountId?.toLowerCase() && expectedAccountId !== undefined ||
        value.reason !== body.reason || value.check_scope !== body.check_scope || value.started_at !== null || value.completed_at !== null || value.last_error_code !== null || value.can_retry ||
        value.checked_count !== "0" || value.consistent_count !== "0" || value.repairable_count !== "0" || value.corrupt_count !== "0" || value.failed_count !== "0" || value.pending_count !== value.target_count) invalidResponse(true);
      return value;
    },
    async retry(brandId, jobId, body, key, previous) {
      if (!validUuid(jobId) || !isRecord(body) || !exact(body, ["version", "reason"]) || !positiveInt(body.version) || body.version >= Number.MAX_SAFE_INTEGER ||
        !safeReason(body.reason) || !/^[A-Za-z0-9_:.-]{8,128}$/.test(key) || (previous !== undefined && (!validJob(previous, brandId) || !sameUuid(previous.id, jobId) ||
          previous.state !== "failed" || !previous.can_retry || previous.version !== body.version))) invalidInput();
      const value = await request(`${BASE}/${encodeURIComponent(jobId)}/retry`, brandId, { method: "POST", body, key, write: true });
      if (!validJob(value, brandId) || !sameUuid(value.id, jobId) || value.version !== body.version + 1) invalidResponse(true);
      if (value.state !== "pending" || value.completed_at !== null || value.last_error_code !== null || value.can_retry || value.failed_count !== "0" ||
        BigInt(value.pending_count) !== BigInt(value.target_count) - BigInt(value.checked_count)) invalidResponse(true);
      if (previous && (previous.state !== "failed" || !previous.can_retry || previous.version !== body.version || value.target_count !== previous.target_count ||
        value.checked_count !== previous.checked_count || value.consistent_count !== previous.consistent_count || value.repairable_count !== previous.repairable_count ||
        value.corrupt_count !== previous.corrupt_count || value.started_at !== previous.started_at || !sameUuid(value.created_by, previous.created_by) || value.created_at !== previous.created_at ||
        !sameUuid(value.creation_audit_log_id, previous.creation_audit_log_id) || value.check_scope !== previous.check_scope)) invalidResponse(true);
      return value;
    },
  };
}
