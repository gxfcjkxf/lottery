import { AdminApiError, type AdminAccount } from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";

const BASE = "/api/v1/admin";
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const MAX_PAGE_SIZE = 100;
const MAX_OFFSET = 1_000_000;

export type SettlementMode = "automatic" | "manual";
export type SettlementPolicyMode = SettlementMode | null;
export type SettlementJobState = "processing" | "awaiting_approval" | "paying" | "failed" | "completed";
export type SettlementTargetState = "pending" | "ready" | "paid" | "excluded" | "failed";

export interface SettlementPolicy {
  brand_id: string;
  version: number;
  mode: SettlementPolicyMode;
  updated_at: string;
}

export interface SettlementPolicyBody {
  version: number;
  mode: SettlementPolicyMode;
  reason: string;
}

export interface SettlementPolicyReceipt extends SettlementPolicy {
  audit_log_id: string;
}

export interface SettlementJobContext {
  brand_id: string;
  game_id: string;
  period_id: string;
  period_version: number;
  period_status: string;
  draw_result_id: string | null;
  policy_version: number;
  mode: SettlementPolicyMode;
  can_start: boolean;
}

export interface SettlementStartBody {
  version: number;
  policy_version: number;
  draw_result_id: string;
  reason: string;
}

export interface SettlementJobActionBody {
  version: number;
  reason: string;
}

export interface SettlementJob {
  generation: number;
  previous_job_id: string | null;
  correction_id: string | null;
  current: boolean;
  id: string;
  brand_id: string;
  game_id: string;
  period_id: string;
  draw_result_id: string;
  period_version: number;
  policy_version: number;
  mode: SettlementMode;
  state: SettlementJobState;
  version: number;
  target_count: number;
  created_by: string;
  approved_by: string | null;
  reason: string;
  created_at: string;
  completed_at: string | null;
  last_error_code: string | null;
  pending_count: number;
  ready_count: number;
  paid_count: number;
  excluded_count: number;
  failed_count: number;
  prize_points: string;
  paid_points: string;
  can_retry: boolean;
}

export interface SettlementTarget {
  order_id: string;
  member_id: string;
  state: SettlementTargetState;
  version: number;
  calculation_id: string | null;
  order_version: number;
  order_status: string;
  won: boolean | null;
  prize_points: string | null;
  payout_entry_id: string | null;
  error_code: string | null;
}

export interface SettlementJobApi {
  policy(brandId: string): Promise<SettlementPolicy>;
  savePolicy(brandId: string, body: SettlementPolicyBody, key: string): Promise<SettlementPolicyReceipt>;
  context(brandId: string, periodId: string): Promise<SettlementJobContext>;
  periodJob(brandId: string, periodId: string): Promise<{ settlement: SettlementJob | null }>;
  job(brandId: string, jobId: string): Promise<SettlementJob>;
  start(brandId: string, periodId: string, body: SettlementStartBody, key: string, expectedAccountId?: string): Promise<SettlementJob>;
  approve(brandId: string, jobId: string, body: SettlementJobActionBody, key: string): Promise<SettlementJob>;
  retry(brandId: string, jobId: string, body: SettlementJobActionBody, key: string): Promise<SettlementJob>;
  targets(brandId: string, jobId: string, limit?: number, offset?: number): Promise<{
    brand_id: string;
    job_id: string;
    items: SettlementTarget[];
    limit: number;
    offset: number;
    has_more: boolean;
  }>;
}

export function settlementJobPermissions(
  account: AdminAccount,
  brandId: string,
): { view: boolean; run: boolean; approve: boolean; retry: boolean; policyView: boolean; policyWrite: boolean } {
  const brand = brandPermissionSet(account, brandId);
  const member = UUID_RE.test(account.id) && UUID_RE.test(brandId);
  return {
    view: member && brand.has("settlement.view.brand"),
    run: member && brand.has("settlement.run.brand"),
    approve: member && brand.has("settlement.approve.brand"),
    retry: member && brand.has("settlement.retry.brand"),
    policyView: member && brand.has("settlement_policy.view.brand"),
    policyWrite: member && brand.has("settlement_policy.write.brand"),
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}
function nonempty(value: unknown): value is string {
  return typeof value === "string" && value.trim().length > 0;
}
function validUuid(value: unknown): value is string {
  return typeof value === "string" && UUID_RE.test(value);
}
function sameUuid(left: unknown, right: string): boolean {
  return typeof left === "string" && left.toLowerCase() === right.toLowerCase();
}
function positiveVersion(value: unknown): value is number {
  return Number.isSafeInteger(value) && Number(value) > 0;
}
function nonnegativeInt(value: unknown): value is number {
  return Number.isSafeInteger(value) && Number(value) >= 0;
}
function validMode(value: unknown): value is SettlementPolicyMode {
  return value === null || value === "automatic" || value === "manual";
}
function validIsoDateTime(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|([+-])(\d{2}):(\d{2}))$/.exec(value);
  if (!match) return false;
  const [, y, mo, d, h, mi, s, , , , oh, om] = match;
  const year = Number(y), month = Number(mo), day = Number(d);
  const hour = Number(h), minute = Number(mi), second = Number(s);
  const days = month === 2 ? (year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28)
    : [4, 6, 9, 11].includes(month) ? 30 : 31;
  if (year < 1 || month < 1 || month > 12 || day < 1 || day > days || hour > 23 || minute > 59 || second > 59) return false;
  if (oh !== undefined && (Number(oh) > 14 || Number(om) > 59 || (Number(oh) === 14 && Number(om) !== 0))) return false;
  return Number.isFinite(Date.parse(value));
}
function validAggregate(value: unknown): value is string {
  return typeof value === "string" && /^(0|[1-9][0-9]*)$/.test(value) && (() => {
    try { return BigInt(value) >= 0n; } catch { return false; }
  })();
}
function validInt64(value: unknown): value is string {
  if (typeof value !== "string" || !/^(0|[1-9][0-9]*)$/.test(value)) return false;
  try { return BigInt(value) <= 9223372036854775807n; } catch { return false; }
}
function invalidResponse(): never {
  throw new AdminApiError("结算任务响应格式无效。", 502, "INVALID_RESPONSE");
}
function invalidInput(message: string): never {
  throw new AdminApiError(message, 0, "INVALID_INPUT");
}
function validatePage(limit: number, offset: number): void {
  if (!Number.isSafeInteger(limit) || limit < 1 || limit > MAX_PAGE_SIZE ||
    !Number.isSafeInteger(offset) || offset < 0 || offset > MAX_OFFSET) invalidInput("分页参数超出允许范围。");
}
function validPolicy(value: unknown, brandId: string): value is SettlementPolicy {
  return isRecord(value) && sameUuid(value.brand_id, brandId) && positiveVersion(value.version) &&
    validMode(value.mode) && validIsoDateTime(value.updated_at);
}
function validContext(value: unknown, brandId: string, periodId: string): value is SettlementJobContext {
  if (!isRecord(value) || !sameUuid(value.brand_id, brandId) || !validUuid(value.game_id) ||
    !sameUuid(value.period_id, periodId) || !positiveVersion(value.period_version) ||
    !nonempty(value.period_status) || !(value.draw_result_id === null || validUuid(value.draw_result_id)) ||
    !positiveVersion(value.policy_version) || !validMode(value.mode) || typeof value.can_start !== "boolean") return false;
  return value.can_start === (value.period_status === "drawn" && value.draw_result_id !== null && value.mode !== null);
}
function validJob(value: unknown, brandId: string, expected?: { jobId?: string; periodId?: string; drawId?: string; policyVersion?: number; mode?: SettlementMode; accountId?: string }): value is SettlementJob {
  if (!isRecord(value) || !validUuid(value.id) || (expected?.jobId !== undefined && !sameUuid(value.id, expected.jobId)) ||
    !sameUuid(value.brand_id, brandId) || !validUuid(value.game_id) || !validUuid(value.period_id) ||
    (expected?.periodId !== undefined && !sameUuid(value.period_id, expected.periodId)) ||
    !validUuid(value.draw_result_id) || (expected?.drawId !== undefined && !sameUuid(value.draw_result_id, expected.drawId)) ||
    !positiveVersion(value.period_version) || !positiveVersion(value.policy_version) ||
    (expected?.policyVersion !== undefined && value.policy_version !== expected.policyVersion) ||
    (expected?.mode !== undefined && value.mode !== expected.mode) ||
    !(value.mode === "automatic" || value.mode === "manual") ||
    !["processing", "awaiting_approval", "paying", "failed", "completed"].includes(String(value.state)) ||
    !positiveVersion(value.version) || !positiveVersion(value.generation) || typeof value.current !== "boolean" ||
    !(value.previous_job_id === null || validUuid(value.previous_job_id)) || !(value.correction_id === null || validUuid(value.correction_id)) ||
    (value.generation === 1 ? value.previous_job_id !== null || value.correction_id !== null : value.previous_job_id === null || value.correction_id === null) ||
    !nonnegativeInt(value.target_count) || !validUuid(value.created_by) ||
    (expected?.accountId !== undefined && !sameUuid(value.created_by, expected.accountId)) ||
    !(value.approved_by === null || validUuid(value.approved_by)) || !nonempty(value.reason) ||
    !validIsoDateTime(value.created_at) || !(value.completed_at === null || validIsoDateTime(value.completed_at)) ||
    !(value.last_error_code === null || nonempty(value.last_error_code)) ||
    !nonnegativeInt(value.pending_count) || !nonnegativeInt(value.ready_count) || !nonnegativeInt(value.paid_count) ||
    !nonnegativeInt(value.excluded_count) || !nonnegativeInt(value.failed_count) ||
    !validAggregate(value.prize_points) || !validAggregate(value.paid_points) || typeof value.can_retry !== "boolean") return false;
  if (value.pending_count + value.ready_count + value.paid_count + value.excluded_count + value.failed_count !== value.target_count ||
      value.can_retry !== (value.state === "failed" && value.current) || (value.last_error_code !== null) !== (value.state === "failed") ||
      (value.completed_at !== null) !== (value.state === "completed") || BigInt(value.paid_points) > BigInt(value.prize_points)) return false;
  if (["awaiting_approval", "paying", "completed"].includes(String(value.state)) && (value.pending_count !== 0 || value.failed_count !== 0)) return false;
  if (value.state === "completed" && value.ready_count !== 0) return false;
  if (value.state === "awaiting_approval" && (value.mode !== "manual" || value.approved_by !== null)) return false;
  if (value.state === "paying" && value.mode === "manual" && value.approved_by === null) return false;
  return value.mode !== "automatic" || value.approved_by === null;
}
function validTarget(value: unknown): value is SettlementTarget {
  return isRecord(value) && validUuid(value.order_id) && validUuid(value.member_id) &&
    ["pending", "ready", "paid", "excluded", "failed"].includes(String(value.state)) &&
    positiveVersion(value.version) && (value.calculation_id === null || validUuid(value.calculation_id)) &&
    positiveVersion(value.order_version) && nonempty(value.order_status) &&
    (value.won === null || typeof value.won === "boolean") &&
    (value.prize_points === null || validInt64(value.prize_points)) &&
    (value.payout_entry_id === null || validUuid(value.payout_entry_id)) &&
    (value.error_code === null || nonempty(value.error_code));
}

type Envelope = { success?: unknown; data?: unknown; error?: { code?: string; message?: string } | null };

export function createSettlementJobApi({ fetch: fetcher = fetch }: { fetch?: typeof fetch } = {}): SettlementJobApi {
  async function request(path: string, brandId: string, options: { method?: "GET" | "POST" | "PUT"; body?: unknown; key?: string; successStatus?: number } = {}): Promise<unknown> {
    if (!validUuid(brandId)) invalidInput("品牌编号必须是 UUID。");
    const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brandId });
    if (options.body !== undefined) headers.set("Content-Type", "application/json");
    if (options.key !== undefined) headers.set("Idempotency-Key", options.key);
    let response: Response;
    try {
      response = await fetcher(path, {
        method: options.method ?? "GET", credentials: "include", headers,
        ...(options.body === undefined ? {} : { body: JSON.stringify(options.body) }),
      });
    } catch (cause) {
      throw new AdminApiError(cause instanceof Error ? cause.message : "Network request failed", 0, "NETWORK_ERROR");
    }
    if (response.ok && response.status !== (options.successStatus ?? 200)) invalidResponse();
    let envelope: Envelope;
    try { envelope = await response.json() as Envelope; }
    catch {
      throw new AdminApiError(response.ok ? "Invalid server response" : `Request failed (${response.status})`, response.ok ? 502 : response.status, response.ok ? "INVALID_RESPONSE" : undefined);
    }
    if (!response.ok || envelope.success !== true || envelope.data === undefined) {
      const error = typeof envelope.error === "object" && envelope.error ? envelope.error : undefined;
      if (typeof envelope.error === "string") throw new AdminApiError("Invalid server response", 502, "INVALID_RESPONSE");
      throw new AdminApiError(error?.message || `Request failed (${response.status})`, response.ok ? 502 : response.status, response.ok ? "INVALID_RESPONSE" : error?.code);
    }
    return envelope.data;
  }
  function requireId(value: string, label: string): void {
    if (!validUuid(value)) invalidInput(`${label}必须是 UUID。`);
  }

  return {
    async policy(brandId) {
      const value = await request(`${BASE}/settlement-policy`, brandId);
      if (!validPolicy(value, brandId)) invalidResponse();
      return value;
    },
    async savePolicy(brandId, body, key) {
      if (!positiveVersion(body.version) || body.version >= Number.MAX_SAFE_INTEGER || !validMode(body.mode) || !nonempty(body.reason) || !nonempty(key)) invalidInput("结算策略请求参数无效。");
      const value = await request(`${BASE}/settlement-policy`, brandId, { method: "PUT", body, key });
      if (!validPolicy(value, brandId) || value.version !== body.version + 1 || value.mode !== body.mode ||
        !isRecord(value) || !validUuid(value.audit_log_id)) invalidResponse();
      return value as unknown as SettlementPolicyReceipt;
    },
    async context(brandId, periodId) {
      requireId(periodId, "期次编号");
      const value = await request(`${BASE}/periods/${encodeURIComponent(periodId)}/settlement-context`, brandId);
      if (!validContext(value, brandId, periodId)) invalidResponse();
      return value;
    },
    async periodJob(brandId, periodId) {
      requireId(periodId, "期次编号");
      const value = await request(`${BASE}/periods/${encodeURIComponent(periodId)}/settlement`, brandId);
      if (!isRecord(value) || !(value.settlement === null || validJob(value.settlement, brandId, { periodId }))) invalidResponse();
      return value as { settlement: SettlementJob | null };
    },
    async job(brandId, jobId) {
      requireId(jobId, "结算任务编号");
      const value = await request(`${BASE}/settlement-jobs/${encodeURIComponent(jobId)}`, brandId);
      if (!validJob(value, brandId, { jobId })) invalidResponse();
      return value;
    },
    async start(brandId, periodId, body, key, expectedAccountId) {
      requireId(periodId, "期次编号");
      if (!positiveVersion(body.version) || !positiveVersion(body.policy_version) || !validUuid(body.draw_result_id) || !nonempty(body.reason) || !nonempty(key) || (expectedAccountId !== undefined && !validUuid(expectedAccountId))) invalidInput("启动结算请求参数无效。");
      const value = await request(`${BASE}/periods/${encodeURIComponent(periodId)}/settle`, brandId, { method: "POST", body, key, successStatus: 201 });
      if (!validJob(value, brandId, { periodId, drawId: body.draw_result_id, policyVersion: body.policy_version, accountId: expectedAccountId }) || value.period_version !== body.version + 1 || value.state !== "processing" || value.version !== 1) invalidResponse();
      return value;
    },
    async approve(brandId, jobId, body, key) {
      requireId(jobId, "结算任务编号");
      if (!positiveVersion(body.version) || !nonempty(body.reason) || !nonempty(key)) invalidInput("审批请求参数无效。");
      const value = await request(`${BASE}/settlement-jobs/${encodeURIComponent(jobId)}/approve`, brandId, { method: "POST", body, key });
      if (!validJob(value, brandId, { jobId }) || value.version !== body.version + 1 || value.state !== "paying" || value.mode !== "manual") invalidResponse();
      return value;
    },
    async retry(brandId, jobId, body, key) {
      requireId(jobId, "结算任务编号");
      if (!positiveVersion(body.version) || !nonempty(body.reason) || !nonempty(key)) invalidInput("重试请求参数无效。");
      const value = await request(`${BASE}/settlement-jobs/${encodeURIComponent(jobId)}/retry`, brandId, { method: "POST", body, key });
      if (!validJob(value, brandId, { jobId }) || value.version !== body.version + 1 || !["processing", "paying"].includes(value.state)) invalidResponse();
      return value;
    },
    async targets(brandId, jobId, limit = 20, offset = 0) {
      validatePage(limit, offset);
      requireId(jobId, "结算任务编号");
      const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
      const value = await request(`${BASE}/settlement-jobs/${encodeURIComponent(jobId)}/targets?${query}`, brandId);
      if (!isRecord(value) || !sameUuid(value.brand_id, brandId) || !sameUuid(value.job_id, jobId) ||
        !Array.isArray(value.items) || value.items.length > limit || value.limit !== limit || value.offset !== offset ||
        typeof value.has_more !== "boolean" || (value.has_more && value.items.length !== limit) ||
        !value.items.every(validTarget)) invalidResponse();
      return value as unknown as Awaited<ReturnType<SettlementJobApi["targets"]>>;
    },
  };
}
