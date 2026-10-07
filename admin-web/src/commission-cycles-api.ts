import { AdminApiError, type AdminAccount } from "./admin-api";

const CYCLES = "/api/v1/admin/commission-cycles";
const DISCOVERIES = "/api/v1/admin/commission-discovery";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const DATE_TIME = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/;
const MAX_INT64 = 9_223_372_036_854_775_807n;
const MAX_SAFE_VERSION = BigInt(Number.MAX_SAFE_INTEGER);

export interface CommissionCycleCalendar {
  timezone: string;
  cycle: "weekly" | "monthly";
  boundary_time: string;
  weekday: number | null;
  month_day: number | null;
  short_month: "" | "last_day" | "skip";
}

export interface CommissionCycleRecord {
  id: string;
  brand_id: string;
  window_from: string;
  window_to: string;
  anchor_order_id: string;
  calendar: CommissionCycleCalendar;
  state: "enumerating" | "waiting" | "calculating" | "summarizing" | "ready" | "failed";
  version: number;
  target_count: string;
  scan_complete: boolean;
  current_run_id: string | null;
  current_generation: string | null;
  evidence_epoch: string | null;
  evidence_current: boolean;
  calculated_count: string;
  earning_count: string;
  total_points: string;
  created_by: string | null;
  creation_actor_type: "admin" | "system";
  reason: string;
  created_at: string;
  updated_at: string;
  last_error_code: string | null;
  creation_audit_log_id: string;
}

export interface CommissionCyclePage {
  brand_id: string;
  items: CommissionCycleRecord[];
  total_count: string;
  limit: number;
  offset: number;
}

export interface CommissionEarning {
  id: string;
  brand_id: string;
  cycle_id: string;
  run_id: string;
  agent_id: string;
  member_id: string;
  exact_amount: { numerator: string; denominator: string };
  points: string;
  created_at: string;
}

export interface CommissionEarningPage {
  brand_id: string;
  cycle_id: string;
  items: CommissionEarning[];
  total_count: string;
  limit: number;
  offset: number;
}

export interface CommissionRun {
  id: string;
  brand_id: string;
  cycle_id: string;
  generation: string;
  evidence_epoch: string;
  state: "calculating" | "summarizing" | "ready" | "abandoned";
  calculated_count: string;
  earning_count: string;
  total_points: string;
  created_at: string;
}

export interface CommissionRunPage {
  brand_id: string;
  cycle_id: string;
  items: CommissionRun[];
  total_count: string;
  limit: number;
  offset: number;
}

export interface CommissionCalculation {
  id: string;
  brand_id: string;
  cycle_id: string;
  run_id: string;
  order_id: string;
  member_id: string;
  reason: "eligible" | "policy_disabled" | "unattributed" | "other_cycle" | "cancelled" | "abnormal";
  status: "won" | "lost" | "abnormal" | "bet_cancelled" | "judged_cancelled";
  stake_points: string;
  prize_points: string;
  base_points: string;
  job_id: string | null;
  calculation_id: string | null;
  generation: string | null;
  audit_log_id: string;
  created_at: string;
}

export interface CommissionCalculationPage {
  brand_id: string;
  cycle_id: string;
  run_id: string;
  items: CommissionCalculation[];
  total_count: string;
  limit: number;
  offset: number;
}

export interface CommissionDiscovery {
  id: string;
  brand_id: string;
  state: "pending" | "registered" | "failed";
  version: number;
  cycle_id: string | null;
  window_from: string | null;
  window_to: string | null;
  next_check_at: string;
  last_error_code: string | null;
  last_audit_log_id: string | null;
  created_at: string;
  updated_at: string;
}

export interface CommissionDiscoveryPage {
  brand_id: string;
  items: CommissionDiscovery[];
  total_count: string;
  limit: number;
  offset: number;
}

export interface CommissionCreateBody {
  anchor_order_id: string;
  reason: string;
}

export interface CommissionRetryBody {
  version: number;
  reason: string;
}

export interface CommissionCyclesApi {
  list(brand: string, limit?: number, offset?: number): Promise<CommissionCyclePage>;
  read(brand: string, id: string): Promise<CommissionCycleRecord>;
  earnings(brand: string, id: string, limit?: number, offset?: number): Promise<CommissionEarningPage>;
  runs(brand: string, id: string, limit?: number, offset?: number): Promise<CommissionRunPage>;
  calculations(brand: string, id: string, runId: string, limit?: number, offset?: number): Promise<CommissionCalculationPage>;
  discoveries(brand: string, limit?: number, offset?: number): Promise<CommissionDiscoveryPage>;
  create(brand: string, body: Readonly<CommissionCreateBody>, key: string, expectedAccountId?: string): Promise<CommissionCycleRecord>;
  retryCycle(brand: string, id: string, body: Readonly<CommissionRetryBody>, key: string): Promise<CommissionCycleRecord>;
  retryDiscovery(brand: string, id: string, body: Readonly<CommissionRetryBody>, key: string): Promise<CommissionDiscovery>;
}

type JsonRecord = Record<string, unknown>;

function isRecord(value: unknown): value is JsonRecord {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function hasOnlyKeys(value: JsonRecord, keys: readonly string[]): boolean {
  return Object.keys(value).every((key) => keys.includes(key));
}

function exactKeys(value: unknown, keys: readonly string[]): value is JsonRecord {
  return isRecord(value) && Object.keys(value).length === keys.length && hasOnlyKeys(value, keys);
}

function hasExactKeySet(value: JsonRecord, keys: readonly string[]): boolean {
  return Object.keys(value).length === keys.length && hasOnlyKeys(value, keys);
}

function isCanonicalUuid(value: unknown): value is string {
  return typeof value === "string" && UUID.test(value);
}

function isVersion(value: unknown): value is number {
  return Number.isSafeInteger(value) && Number(value) >= 1;
}

function isDecimal(value: unknown, positive = false, max = MAX_INT64): value is string {
  if (typeof value !== "string" || !(positive ? /^[1-9][0-9]*$/ : /^(0|[1-9][0-9]*)$/).test(value)) return false;
  try {
    const parsed = BigInt(value);
    return parsed <= max && (!positive || parsed > 0n);
  } catch {
    return false;
  }
}

function isNonnegativeDecimal(value: unknown): value is string {
  if (typeof value !== "string" || !/^(0|[1-9][0-9]*)$/.test(value)) return false;
  try { return BigInt(value) <= MAX_INT64 * MAX_INT64; } catch { return false; }
}

function isReason(value: unknown): value is string {
  if (typeof value !== "string" || value.length === 0 || value.trim() !== value || value.includes("\0")) return false;
  for (let i = 0; i < value.length; i++) {
    const code = value.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff) {
      const next = value.charCodeAt(i + 1);
      if (!(next >= 0xdc00 && next <= 0xdfff)) return false;
      i++;
    } else if (code >= 0xdc00 && code <= 0xdfff) return false;
  }
  return new TextEncoder().encode(value).length <= 500;
}

function isDateTime(value: unknown): value is string {
  if (typeof value !== "string" || !DATE_TIME.test(value) || !Number.isFinite(Date.parse(value))) return false;
  const [date, rawTime] = value.split("T");
  const [year, month, day] = date.split("-").map(Number);
  const time = rawTime.split(/[.+Z-]/)[0];
  const [hour, minute, second] = time.split(":").map(Number);
  const monthEnd = new Date(0);
  monthEnd.setUTCFullYear(year, month, 0);
  const offset = /([+-])(\d{2}):(\d{2})$/.exec(value);
  return year >= 1 && month >= 1 && month <= 12 && day >= 1 && day <= monthEnd.getUTCDate() &&
    hour <= 23 && minute <= 59 && second <= 59 &&
    (!offset || (Number(offset[2]) < 14 || (Number(offset[2]) === 14 && Number(offset[3]) === 0)) && Number(offset[3]) <= 59);
}

function isNullableUuid(value: unknown): value is string | null {
  return value === null || isCanonicalUuid(value);
}

function isNullableDateTime(value: unknown): value is string | null {
  return value === null || isDateTime(value);
}

function isCalendar(value: unknown): value is CommissionCycleCalendar {
  if (!exactKeys(value, ["timezone", "cycle", "boundary_time", "weekday", "month_day", "short_month"])) return false;
  if (typeof value.timezone !== "string" || !value.timezone.trim() || value.timezone === "Local") return false;
  try {
    new Intl.DateTimeFormat("en", { timeZone: value.timezone });
  } catch {
    return false;
  }
  if (typeof value.boundary_time !== "string" || !/^([01]\d|2[0-3]):[0-5]\d:[0-5]\d$/.test(value.boundary_time)) return false;
  if (value.cycle === "weekly") return Number.isInteger(value.weekday) && Number(value.weekday) >= 0 && Number(value.weekday) <= 6 && value.month_day === null && value.short_month === "";
  if (value.cycle === "monthly") return value.weekday === null && Number.isInteger(value.month_day) && Number(value.month_day) >= 1 && Number(value.month_day) <= 31 && (value.short_month === "last_day" || value.short_month === "skip");
  return false;
}

const CYCLE_KEYS = ["id", "brand_id", "window_from", "window_to", "anchor_order_id", "calendar", "state", "version", "target_count", "scan_complete", "current_run_id", "current_generation", "evidence_epoch", "evidence_current", "calculated_count", "earning_count", "total_points", "created_by", "creation_actor_type", "reason", "created_at", "updated_at", "last_error_code", "creation_audit_log_id"] as const;
const PAGE_KEYS = ["brand_id", "items", "total_count", "limit", "offset"] as const;

function validCycle(value: unknown, brand: string, allowLegacyActor = false): value is CommissionCycleRecord {
  if (!isRecord(value)) return false;
  const hasActorType = hasExactKeySet(value, CYCLE_KEYS);
  const hasLegacyActor = allowLegacyActor && hasExactKeySet(value, CYCLE_KEYS.filter((key) => key !== "creation_actor_type")) && isCanonicalUuid(value.created_by);
  if (!hasActorType && !hasLegacyActor) return false;
  const actorType = value.creation_actor_type === undefined ? "admin" : value.creation_actor_type;
  if (!isCanonicalUuid(value.id) || value.brand_id !== brand || !isCanonicalUuid(value.brand_id) ||
    !isDateTime(value.window_from) || !isDateTime(value.window_to) || Date.parse(value.window_from) >= Date.parse(value.window_to) ||
    !isCanonicalUuid(value.anchor_order_id) || !isCalendar(value.calendar) ||
    !["enumerating", "waiting", "calculating", "summarizing", "ready", "failed"].includes(String(value.state)) ||
    !isVersion(value.version) || BigInt(value.version) > MAX_SAFE_VERSION || !isDecimal(value.target_count) || typeof value.scan_complete !== "boolean" ||
    !isNullableUuid(value.current_run_id) || !(value.current_generation === null || isDecimal(value.current_generation, true)) ||
    !(value.evidence_epoch === null || isDecimal(value.evidence_epoch)) || typeof value.evidence_current !== "boolean" ||
    !isDecimal(value.calculated_count) || !isDecimal(value.earning_count) || !isNonnegativeDecimal(value.total_points) ||
    !isNullableUuid(value.created_by) || (actorType !== "admin" && actorType !== "system") ||
    (actorType === "system") !== (value.created_by === null) || !isReason(value.reason) ||
    !isDateTime(value.created_at) || !isDateTime(value.updated_at) || Date.parse(value.updated_at) < Date.parse(value.created_at) ||
    !(value.last_error_code === null || typeof value.last_error_code === "string") || !isCanonicalUuid(value.creation_audit_log_id)) return false;
  if (value.current_run_id === null ? value.current_generation !== null || value.evidence_epoch !== null : value.current_generation === null || value.evidence_epoch === null) return false;
  if ((value.state === "failed") !== (value.last_error_code !== null) ||
    (["waiting", "calculating", "summarizing", "ready"].includes(String(value.state)) && !value.scan_complete) ||
    (["calculating", "summarizing", "ready"].includes(String(value.state)) && value.current_run_id === null) ||
    (value.current_run_id === null && (value.calculated_count !== "0" || value.earning_count !== "0" || value.total_points !== "0" || value.evidence_current))) return false;
  value.creation_actor_type = actorType;
  return true;
}

function validPage(value: unknown, brand: string, limit: number, offset: number, row: (item: unknown) => boolean, scope: JsonRecord = {}): value is JsonRecord {
  if (!exactKeys(value, [...Object.keys(scope), ...PAGE_KEYS]) || !Object.entries(scope).every(([key, expected]) => value[key] === expected) ||
    value.brand_id !== brand || !isCanonicalUuid(value.brand_id) || !Array.isArray(value.items) || value.items.length > limit ||
    !value.items.every(row) || new Set(value.items.map((item) => (item as JsonRecord).id)).size !== value.items.length ||
    !isDecimal(value.total_count) || value.limit !== limit || value.offset !== offset) return false;
  const total = BigInt(value.total_count);
  return total >= BigInt(value.items.length) && (value.items.length === 0 || total >= BigInt(offset + value.items.length));
}

function validCyclePage(value: unknown, brand: string, limit: number, offset: number): value is CommissionCyclePage {
  return validPage(value, brand, limit, offset, (item) => validCycle(item, brand));
}

function validEarning(value: unknown, brand: string, cycle: string): value is CommissionEarning {
  if (!exactKeys(value, ["id", "brand_id", "cycle_id", "run_id", "agent_id", "member_id", "exact_amount", "points", "created_at"])) return false;
  const amount = value.exact_amount;
  return isCanonicalUuid(value.id) && value.brand_id === brand && isCanonicalUuid(value.brand_id) && value.cycle_id === cycle && isCanonicalUuid(value.cycle_id) &&
    isCanonicalUuid(value.run_id) && isCanonicalUuid(value.agent_id) && isCanonicalUuid(value.member_id) &&
    exactKeys(amount, ["numerator", "denominator"]) && isDecimal(amount.numerator, false, 10n ** 100n - 1n) && typeof amount.denominator === "string" && /^[1-9][0-9]*$/.test(amount.denominator) && amount.denominator.length <= 7 && BigInt(amount.denominator) <= 1_000_000n && 1_000_000n % BigInt(amount.denominator) === 0n &&
    greatestCommonDivisor(BigInt(amount.numerator), BigInt(amount.denominator)) === 1n && isDecimal(value.points) && roundedHalfUp(BigInt(amount.numerator), BigInt(amount.denominator)) === BigInt(value.points) && isDateTime(value.created_at);
}

function greatestCommonDivisor(left: bigint, right: bigint): bigint {
  while (right !== 0n) [left, right] = [right, left % right];
  return left;
}

function roundedHalfUp(numerator: bigint, denominator: bigint): bigint {
  return (2n * numerator + denominator) / (2n * denominator);
}

function validEarningPage(value: unknown, brand: string, cycle: string, limit: number, offset: number): value is CommissionEarningPage {
  if (!validPage(value, brand, limit, offset, (item) => validEarning(item, brand, cycle), { cycle_id: cycle })) return false;
  const items = value.items as unknown[];
  return items.length === 0 || items.every((item) => isRecord(item) && item.run_id === (items[0] as JsonRecord).run_id);
}

function validRun(value: unknown, brand: string, cycle: string): value is CommissionRun {
  return exactKeys(value, ["id", "brand_id", "cycle_id", "generation", "evidence_epoch", "state", "calculated_count", "earning_count", "total_points", "created_at"]) &&
    isCanonicalUuid(value.id) && value.brand_id === brand && isCanonicalUuid(value.brand_id) && value.cycle_id === cycle && isCanonicalUuid(value.cycle_id) &&
    isDecimal(value.generation, true) && isDecimal(value.evidence_epoch) && ["calculating", "summarizing", "ready", "abandoned"].includes(String(value.state)) &&
    isDecimal(value.calculated_count) && isDecimal(value.earning_count) && isNonnegativeDecimal(value.total_points) && isDateTime(value.created_at);
}

function validCalculation(value: unknown, brand: string, cycle: string, run: string): value is CommissionCalculation {
  if (!exactKeys(value, ["id", "brand_id", "cycle_id", "run_id", "order_id", "member_id", "reason", "status", "stake_points", "prize_points", "base_points", "job_id", "calculation_id", "generation", "audit_log_id", "created_at"]) ||
    !(isCanonicalUuid(value.id) && value.brand_id === brand && isCanonicalUuid(value.brand_id) && value.cycle_id === cycle && isCanonicalUuid(value.cycle_id) && value.run_id === run && isCanonicalUuid(value.run_id) &&
    isCanonicalUuid(value.order_id) && isCanonicalUuid(value.member_id) && ["eligible", "policy_disabled", "unattributed", "other_cycle", "cancelled", "abnormal"].includes(String(value.reason)) &&
    ["won", "lost", "abnormal", "bet_cancelled", "judged_cancelled"].includes(String(value.status)) && isDecimal(value.stake_points, true) && isDecimal(value.prize_points) && isDecimal(value.base_points) &&
    isNullableUuid(value.job_id) && isNullableUuid(value.calculation_id) && (value.generation === null || isDecimal(value.generation, true)) && isCanonicalUuid(value.audit_log_id) && isDateTime(value.created_at))) return false;
  return (value.job_id === null) === (value.calculation_id === null) && (value.job_id === null) === (value.generation === null);
}

function validDiscovery(value: unknown, brand: string): value is CommissionDiscovery {
  if (!exactKeys(value, ["id", "brand_id", "state", "version", "cycle_id", "window_from", "window_to", "next_check_at", "last_error_code", "last_audit_log_id", "created_at", "updated_at"])) return false;
  if (!isCanonicalUuid(value.id) || value.brand_id !== brand || !isCanonicalUuid(value.brand_id) || !["pending", "registered", "failed"].includes(String(value.state)) ||
    !isVersion(value.version) || BigInt(value.version) > MAX_SAFE_VERSION || !isNullableUuid(value.cycle_id) || !isNullableDateTime(value.window_from) || !isNullableDateTime(value.window_to) ||
    !isDateTime(value.next_check_at) || !(value.last_error_code === null || typeof value.last_error_code === "string") || !(value.last_audit_log_id === null || isCanonicalUuid(value.last_audit_log_id)) ||
    !isDateTime(value.created_at) || !isDateTime(value.updated_at) || Date.parse(value.updated_at) < Date.parse(value.created_at)) return false;
  if ((value.window_from === null) !== (value.window_to === null) || (value.window_from !== null && Date.parse(value.window_from as string) >= Date.parse(value.window_to as string))) return false;
  if ((value.state === "failed") !== (value.last_error_code !== null)) return false;
  if (value.state === "registered") return value.version > 1 && isCanonicalUuid(value.cycle_id) && value.window_from !== null && isCanonicalUuid(value.last_audit_log_id);
  if (value.state === "pending") return value.cycle_id === null && (value.version === 1 ? value.last_audit_log_id === null : isCanonicalUuid(value.last_audit_log_id));
  return value.version > 1 && isCanonicalUuid(value.last_audit_log_id);
}

function invalidInput(message: string): never {
  throw new AdminApiError(message, 0, "INVALID_INPUT");
}

function invalidReadResponse(): never {
  throw new AdminApiError("佣金周期响应格式无效。", 502, "INVALID_RESPONSE");
}

function unknownWriteStatus(message = "写入结果未知；请保留原幂等键并使用相同请求重试。"): never {
  throw new AdminApiError(message, 0, "UNKNOWN_WRITE_STATUS");
}

function validateBrand(brand: string): void {
  if (!isCanonicalUuid(brand)) invalidInput("品牌编号必须为规范 UUID。");
}

function validatePage(limit: number, offset: number): void {
  if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000) invalidInput("分页参数无效。");
}

function validateReason(reason: unknown): void {
  if (!isReason(reason)) invalidInput("原因必须是有效 UTF-8 文本且不超过 500 字节。");
}

function validateRetry(body: Readonly<CommissionRetryBody>): void {
  if (!isRecord(body) || !exactKeys(body, ["version", "reason"]) || !isVersion(body.version) || BigInt(body.version) >= MAX_SAFE_VERSION) invalidInput("重试版本无效。");
  validateReason(body.reason);
}

function validateKey(key: string): void {
  if (typeof key !== "string" || key.length < 8 || key.length > 128 || !/^[A-Za-z0-9._:-]+$/.test(key)) invalidInput("幂等键格式无效。");
}

export function commissionCyclePermissions(account: AdminAccount, brand: string): { view: boolean; run: boolean; retry: boolean } {
  const validBrand = isCanonicalUuid(brand);
  const memberOfBrand = account.brand_ids.includes(brand);
  const brandPermissions = new Set(account.permissions_by_brand === undefined
    ? memberOfBrand ? account.permissions ?? [] : []
    : account.permissions_by_brand[brand] ?? []);
  const platformPermissions = new Set(account.platform_permissions ?? account.permissions ?? []);
  const view = validBrand && isCanonicalUuid(account.id) && (platformPermissions.has("commission.view.platform") || (memberOfBrand && brandPermissions.has("commission.view.brand")));
  return {
    view,
    run: Boolean(view && memberOfBrand && !account.super_admin && brandPermissions.has("commission.run.brand")),
    retry: Boolean(view && memberOfBrand && !account.super_admin && brandPermissions.has("commission.retry.brand")),
  };
}

interface RequestOptions {
  method?: "GET" | "POST";
  body?: unknown;
  key?: string;
  write?: boolean;
  successStatus?: number;
}

export function createCommissionCyclesApi(fetcher: typeof fetch = fetch): CommissionCyclesApi {
  async function request<T>(brand: string, path: string, options: RequestOptions = {}, validate: (data: unknown) => data is T): Promise<T> {
    const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brand });
    if (options.body !== undefined) headers.set("Content-Type", "application/json");
    if (options.key !== undefined) headers.set("Idempotency-Key", options.key);
    let response: Response;
    try {
      response = await fetcher(path, {
        method: options.method ?? "GET",
        credentials: "same-origin",
        headers,
        ...(options.body === undefined ? {} : { body: JSON.stringify(options.body) }),
      });
    } catch (cause) {
      if (options.write) unknownWriteStatus(cause instanceof Error ? cause.message : undefined);
      throw new AdminApiError(cause instanceof Error ? cause.message : "Network request failed", 0, "NETWORK_ERROR");
    }

    if (!response.ok) {
      if (options.write && (response.status < 400 || response.status >= 500)) unknownWriteStatus();
      let error: unknown;
      try { error = await response.json(); } catch { error = undefined; }
      const detail = isRecord(error) && isRecord(error.error) ? error.error : undefined;
      const message = isRecord(error) && typeof error.error === "string" ? error.error :
        (typeof detail?.message === "string" && detail.message) || `Request failed (${response.status})`;
      throw new AdminApiError(message, response.status, typeof detail?.code === "string" ? detail.code : undefined);
    }

    let envelope: unknown;
    try { envelope = await response.json(); } catch {
      if (options.write) unknownWriteStatus();
      invalidReadResponse();
    }
    if (!isRecord(envelope) || envelope.success !== true || envelope.data === undefined ||
      (options.successStatus !== undefined && response.status !== options.successStatus) || !validate(envelope.data)) {
      if (options.write) unknownWriteStatus();
      invalidReadResponse();
    }
    return envelope.data;
  }

  function pageQuery(limit: number, offset: number): string {
    validatePage(limit, offset);
    return new URLSearchParams({ limit: String(limit), offset: String(offset) }).toString();
  }

  return {
    async list(brand, limit = 20, offset = 0) {
      validateBrand(brand);
      const query = pageQuery(limit, offset);
      return request(brand, `${CYCLES}?${query}`, {}, (value): value is CommissionCyclePage => validCyclePage(value, brand, limit, offset));
    },
    async read(brand, id) {
      validateBrand(brand);
      if (!isCanonicalUuid(id)) invalidInput("周期编号必须为规范 UUID。");
      return request(brand, `${CYCLES}/${id}`, {}, (value): value is CommissionCycleRecord => validCycle(value, brand) && value.id === id);
    },
    async earnings(brand, id, limit = 20, offset = 0) {
      validateBrand(brand);
      if (!isCanonicalUuid(id)) invalidInput("周期编号必须为规范 UUID。");
      const query = pageQuery(limit, offset);
      return request(brand, `${CYCLES}/${id}/earnings?${query}`, {}, (value): value is CommissionEarningPage => validEarningPage(value, brand, id, limit, offset));
    },
    async runs(brand, id, limit = 20, offset = 0) {
      validateBrand(brand);
      if (!isCanonicalUuid(id)) invalidInput("周期编号必须为规范 UUID。");
      const query = pageQuery(limit, offset);
      return request(brand, `${CYCLES}/${id}/runs?${query}`, {}, (value): value is CommissionRunPage =>
        validPage(value, brand, limit, offset, (item) => validRun(item, brand, id), { cycle_id: id }));
    },
    async calculations(brand, id, runId, limit = 20, offset = 0) {
      validateBrand(brand);
      if (!isCanonicalUuid(id) || !isCanonicalUuid(runId)) invalidInput("周期或运行编号必须为规范 UUID。");
      const query = pageQuery(limit, offset);
      return request(brand, `${CYCLES}/${id}/runs/${runId}/calculations?${query}`, {}, (value): value is CommissionCalculationPage =>
        validPage(value, brand, limit, offset, (item) => validCalculation(item, brand, id, runId), { cycle_id: id, run_id: runId }));
    },
    async discoveries(brand, limit = 20, offset = 0) {
      validateBrand(brand);
      const query = pageQuery(limit, offset);
      return request(brand, `${DISCOVERIES}?${query}`, {}, (value): value is CommissionDiscoveryPage => validPage(value, brand, limit, offset, (item) => validDiscovery(item, brand)));
    },
    async create(brand, body, key, expectedAccountId) {
      validateBrand(brand);
      if (!isRecord(body) || !exactKeys(body, ["anchor_order_id", "reason"]) || !isCanonicalUuid(body.anchor_order_id)) invalidInput("创建周期输入无效。");
      validateReason(body.reason);
      validateKey(key);
      if (expectedAccountId !== undefined && !isCanonicalUuid(expectedAccountId)) invalidInput("管理员编号必须为规范 UUID。");
      const requestBody: CommissionCreateBody = { anchor_order_id: body.anchor_order_id, reason: body.reason };
      return request(brand, CYCLES, { method: "POST", body: requestBody, key, write: true, successStatus: 201 }, (value): value is CommissionCycleRecord =>
        validCycle(value, brand, true) && value.version === 1 && value.state === "enumerating" && value.anchor_order_id === body.anchor_order_id &&
        value.reason === body.reason && value.creation_actor_type === "admin" && value.created_by !== null &&
        (expectedAccountId === undefined || value.created_by === expectedAccountId));
    },
    async retryCycle(brand, id, body, key) {
      validateBrand(brand);
      if (!isCanonicalUuid(id)) invalidInput("周期编号必须为规范 UUID。");
      validateRetry(body);
      validateKey(key);
      const requestBody: CommissionRetryBody = { version: body.version, reason: body.reason };
      return request(brand, `${CYCLES}/${id}/retry`, { method: "POST", body: requestBody, key, write: true, successStatus: 200 }, (value): value is CommissionCycleRecord =>
        validCycle(value, brand) && value.id === id && value.version === body.version + 1 &&
        (value.state === "waiting" || value.state === "enumerating") && value.current_run_id === null && value.current_generation === null &&
        value.evidence_epoch === null && value.last_error_code === null);
    },
    async retryDiscovery(brand, id, body, key) {
      validateBrand(brand);
      if (!isCanonicalUuid(id)) invalidInput("发现记录编号必须为规范 UUID。");
      validateRetry(body);
      validateKey(key);
      const requestBody: CommissionRetryBody = { version: body.version, reason: body.reason };
      return request(brand, `${DISCOVERIES}/${id}/retry`, { method: "POST", body: requestBody, key, write: true, successStatus: 200 }, (value): value is CommissionDiscovery =>
        validDiscovery(value, brand) && value.id === id && value.version === body.version + 1 && value.state === "pending" &&
        value.cycle_id === null && value.last_error_code === null && isCanonicalUuid(value.last_audit_log_id));
    },
  };
}
