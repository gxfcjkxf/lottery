import { AdminApiError, type AdminAccount } from "./admin-api";

const BASE = "/api/v1/admin/workbench";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const UINT = /^(0|[1-9][0-9]*)$/;
const SINT = /^(0|-?[1-9][0-9]*)$/;
const STATUSES = ["ready", "forbidden", "not_implemented"] as const;

export type SectionStatus = (typeof STATUSES)[number];
export interface Section<T> { status: SectionStatus; data: T | null }
export interface WorkbenchBrand { name: string; code: string; state: string }
export interface WorkbenchPeriods {
  pending: string; betting: string; closed: string; waiting_draw: string; drawn: string; settling: string;
  refund_pending: string; refund_failed: string;
}
export interface WorkbenchOrders { placed: string; abnormal: string }
export interface WorkbenchTodayBets { order_count: string; stake_points: string; cancelled_count: string; abnormal_count: string }
export interface WorkbenchSettlement { processing: string; awaiting_approval: string; paying: string; failed: string }
export interface WorkbenchRecharges { pending_count: string; pending_points: string }
export interface WorkbenchLedger {
  entry_count: string; net_points: string; recharge_points: string; prize_credit_points: string;
  prize_reversal_points: string; refund_points: string;
}
export interface WorkbenchBalances {
  account_count: string; available_points: string; frozen_points: string; withdrawal_points: string; total_points: string;
}
export interface WorkbenchReconciliationJob {
  id: string; state: "pending" | "running" | "completed" | "failed";
  created_at: string; completed_at: string | null; target_count: string; checked_count: string;
  repairable_count: string; corrupt_count: string; failed_count: string;
}
export interface WorkbenchReconciliation { latest_job: WorkbenchReconciliationJob | null }
export interface WorkbenchSources {
  adapter_state: "stub"; configured_games: string; enabled_api_sources: string; enabled_dom_sources: string;
  attempts_today: string; failed_today: string; no_data_today: string; last_attempt_at: string | null;
}
export interface WorkbenchWithdrawals { reviewing_count: string; reviewing_points: string; processing_count: string; processing_points: string }
export interface WorkbenchCommissions {
  discovery_pending_count: string; discovery_failed_count: string;
  cycle_processing_count: string; cycle_waiting_count: string; cycle_ready_count: string; cycle_stale_count: string; cycle_failed_count: string;
  payment_awaiting_approval_count: string; payment_processing_count: string; payment_blocked_count: string; payment_failed_count: string;
  plan_processing_count: string; plan_ready_count: string; plan_blocked_count: string; plan_failed_count: string;
  execution_awaiting_approval_count: string; execution_processing_count: string; execution_paused_count: string; execution_failed_count: string;
}
export interface WorkbenchRewards { granted_count: string; pending_count: string; revoked_count: string }
export interface WorkbenchSnapshot {
  brand_id: string; snapshot_at: string; timezone: string; day_from: string;
  brand: Section<WorkbenchBrand>; periods: Section<WorkbenchPeriods>; orders: Section<WorkbenchOrders>;
  today_bets: Section<WorkbenchTodayBets>; settlement: Section<WorkbenchSettlement>; recharges: Section<WorkbenchRecharges>;
  ledger: Section<WorkbenchLedger>; balances: Section<WorkbenchBalances>; reconciliation: Section<WorkbenchReconciliation>;
  sources: Section<WorkbenchSources>; withdrawals: Section<WorkbenchWithdrawals>;
  commissions: Section<WorkbenchCommissions>; rewards: Section<WorkbenchRewards>;
}

type SectionName = keyof Omit<WorkbenchSnapshot, "brand_id" | "snapshot_at" | "timezone" | "day_from">;
const SECTION_KEYS: Record<SectionName, string[]> = {
  brand: ["name", "code", "state"],
  periods: ["pending", "betting", "closed", "waiting_draw", "drawn", "settling", "refund_pending", "refund_failed"],
  orders: ["placed", "abnormal"],
  today_bets: ["order_count", "stake_points", "cancelled_count", "abnormal_count"],
  settlement: ["processing", "awaiting_approval", "paying", "failed"],
  recharges: ["pending_count", "pending_points"],
  ledger: ["entry_count", "net_points", "recharge_points", "prize_credit_points", "prize_reversal_points", "refund_points"],
  balances: ["account_count", "available_points", "frozen_points", "withdrawal_points", "total_points"],
  reconciliation: ["latest_job"],
  sources: ["adapter_state", "configured_games", "enabled_api_sources", "enabled_dom_sources", "attempts_today", "failed_today", "no_data_today", "last_attempt_at"],
  withdrawals: ["reviewing_count", "reviewing_points", "processing_count", "processing_points"],
  commissions: ["discovery_pending_count", "discovery_failed_count", "cycle_processing_count", "cycle_waiting_count", "cycle_ready_count", "cycle_stale_count", "cycle_failed_count",
    "payment_awaiting_approval_count", "payment_processing_count", "payment_blocked_count", "payment_failed_count", "plan_processing_count", "plan_ready_count",
    "plan_blocked_count", "plan_failed_count", "execution_awaiting_approval_count", "execution_processing_count", "execution_paused_count", "execution_failed_count"],
  rewards: ["granted_count", "pending_count", "revoked_count"],
};
const COUNTS: Partial<Record<SectionName, string[]>> = {
  periods: SECTION_KEYS.periods, orders: SECTION_KEYS.orders,
  today_bets: ["order_count", "cancelled_count", "abnormal_count"],
  settlement: SECTION_KEYS.settlement, recharges: ["pending_count"], ledger: ["entry_count"],
  balances: ["account_count"], sources: ["configured_games", "enabled_api_sources", "enabled_dom_sources", "attempts_today", "failed_today", "no_data_today"],
  withdrawals: ["reviewing_count", "processing_count"],
  commissions: SECTION_KEYS.commissions, rewards: SECTION_KEYS.rewards,
};
const POINTS: Partial<Record<SectionName, string[]>> = {
  today_bets: ["stake_points"], recharges: ["pending_points"],
  ledger: ["recharge_points", "prize_credit_points", "prize_reversal_points", "refund_points"],
  balances: ["available_points", "frozen_points", "withdrawal_points", "total_points"],
  withdrawals: ["reviewing_points", "processing_points"],
};
const SECTION_ORDER = Object.keys(SECTION_KEYS) as SectionName[];

function record(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}
function exactKeys(value: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(value);
  return actual.length === keys.length && keys.every((key) => Object.hasOwn(value, key));
}
function timestamp(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const match = /^(\d{4})-(\d\d)-(\d\d)T(\d\d):(\d\d):(\d\d)(?:\.\d{1,9})?(Z|[+-](\d\d):(\d\d))$/.exec(value);
  if (!match) return false;
  const [, year, month, day, hour, minute, second, , offsetHour, offsetMinute] = match;
  const date = new Date(Date.UTC(Number(year), Number(month) - 1, Number(day)));
  if (date.getUTCFullYear() !== Number(year) || date.getUTCMonth() + 1 !== Number(month) || date.getUTCDate() !== Number(day) ||
    Number(hour) > 23 || Number(minute) > 59 || Number(second) > 59 ||
    (offsetHour !== undefined && (Number(offsetHour) > 23 || Number(offsetMinute) > 59))) return false;
  return Number.isFinite(Date.parse(value));
}
function uint(value: unknown): value is string { return typeof value === "string" && UINT.test(value); }
function sint(value: unknown): value is string { return typeof value === "string" && SINT.test(value); }
function timeAfterOrEqual(a: string, b: string): boolean { return Date.parse(a) >= Date.parse(b); }

function validJob(value: unknown, snapshotAt: string): value is WorkbenchReconciliationJob {
  if (!record(value) || !exactKeys(value, ["id", "state", "created_at", "completed_at", "target_count", "checked_count", "repairable_count", "corrupt_count", "failed_count"])) return false;
  if (typeof value.id !== "string" || !UUID.test(value.id) || !["pending", "running", "completed", "failed"].includes(String(value.state)) ||
    !timestamp(value.created_at) || !timeAfterOrEqual(snapshotAt, value.created_at) ||
    !(value.completed_at === null || timestamp(value.completed_at)) ||
    !["target_count", "checked_count", "repairable_count", "corrupt_count", "failed_count"].every((key) => uint(value[key]))) return false;
  const target = BigInt(value.target_count as string), checked = BigInt(value.checked_count as string);
  const failed = BigInt(value.failed_count as string);
  const outcomes = BigInt(value.repairable_count as string) + BigInt(value.corrupt_count as string);
  if (checked + failed > target || outcomes > checked || target > 100000n) return false;
  if (value.state !== "completed") return value.completed_at === null;
  return timestamp(value.completed_at) && timeAfterOrEqual(value.completed_at, value.created_at as string) && timeAfterOrEqual(snapshotAt, value.completed_at) &&
    checked === target;
}

export function validWorkbenchSnapshot(value: unknown, brandId: string): value is WorkbenchSnapshot {
  if (!record(value) || !exactKeys(value, ["brand_id", "snapshot_at", "timezone", "day_from", ...SECTION_ORDER])) return false;
  if (value.brand_id !== brandId || !timestamp(value.snapshot_at) || !timestamp(value.day_from) || !timeAfterOrEqual(value.snapshot_at, value.day_from) ||
    Date.parse(value.snapshot_at) - Date.parse(value.day_from) > 26 * 60 * 60 * 1000 ||
    typeof value.timezone !== "string" || value.timezone.length > 100) return false;
  try {
    const formatter = new Intl.DateTimeFormat("en", { timeZone: value.timezone as string, hourCycle: "h23" });
    const localDay = formatter.format(new Date(value.snapshot_at as string));
    if (formatter.format(new Date(value.day_from as string)) !== localDay) return false;
  } catch { return false; }

  for (const name of SECTION_ORDER) {
    const section = value[name];
    if (!record(section) || !exactKeys(section, ["status", "data"]) || !STATUSES.includes(section.status as SectionStatus)) return false;
    if (name !== "commissions" && section.status === "not_implemented") return false;
    if (section.status !== "ready") {
      if (section.data !== null) return false;
      continue;
    }
    if (!record(section.data) || !exactKeys(section.data, SECTION_KEYS[name])) return false;
    const data = section.data;
    if (name === "brand") {
      if (typeof data.name !== "string" || typeof data.code !== "string" || typeof data.state !== "string" ||
        !data.name.trim() || !data.code.trim() || !["active", "paused", "disabled"].includes(data.state)) return false;
    } else if (name === "reconciliation") {
      if (!(data.latest_job === null || validJob(data.latest_job, value.snapshot_at as string))) return false;
    } else if (name === "sources") {
      if (data.adapter_state !== "stub" || !(data.last_attempt_at === null || timestamp(data.last_attempt_at)) ||
        (data.last_attempt_at !== null && !timeAfterOrEqual(value.snapshot_at as string, data.last_attempt_at as string))) return false;
    }
    for (const key of COUNTS[name] ?? []) if (!uint(data[key])) return false;
    for (const key of POINTS[name] ?? []) if (!uint(data[key])) return false;
    if (name === "ledger" && !sint(data.net_points)) return false;
  }
  const ready = (name: SectionName) => (value[name] as Section<unknown>).status === "ready" ? (value[name] as Section<Record<string, string>>).data : null;
  const bets = ready("today_bets"), sources = ready("sources");
  if (bets && (BigInt(bets.cancelled_count) > BigInt(bets.order_count) || BigInt(bets.abnormal_count) > BigInt(bets.order_count))) return false;
  if (sources && BigInt(sources.failed_today) + BigInt(sources.no_data_today) > BigInt(sources.attempts_today)) return false;
  const balances = ready("balances");
  if (balances && BigInt(balances.available_points) + BigInt(balances.frozen_points) + BigInt(balances.withdrawal_points) !== BigInt(balances.total_points)) return false;
  return true;
}

export function workbenchPermissions(account: AdminAccount, brandId: string): Record<SectionName, boolean> {
  const inBrand = (account.brand_ids ?? []).some((id) => id.toLowerCase() === brandId.toLowerCase());
  const brand = new Set(account.permissions_by_brand === undefined ? account.permissions ?? [] : account.permissions_by_brand[brandId] ?? []);
  const platform = new Set(account.platform_permissions ?? account.permissions ?? []);
  const allowed = (resource: string) => platform.has(`${resource}.view.platform`) || inBrand && brand.has(`${resource}.view.brand`);
  return {
    brand: allowed("brand"), periods: allowed("period"), orders: allowed("bet"),
    today_bets: allowed("report_betting"), settlement: allowed("settlement"), recharges: allowed("recharge"),
    ledger: allowed("report_ledger"), balances: allowed("report_ledger"), reconciliation: allowed("wallet"), sources: allowed("draw_source"),
    withdrawals: allowed("withdrawal"), commissions: allowed("commission"), rewards: allowed("reward"),
  };
}

export function createWorkbenchApi(fetchImpl: typeof fetch = fetch) {
  return {
    async get(brandId: string, signal?: AbortSignal): Promise<WorkbenchSnapshot> {
      if (!UUID.test(brandId)) throw new AdminApiError("品牌编号必须是 UUID。", 0, "INVALID_INPUT");
      let response: Response;
      try {
        response = await fetchImpl(BASE, { method: "GET", credentials: "same-origin", signal,
          headers: { Accept: "application/json", "X-Brand-ID": brandId } });
      } catch (cause) {
        if (cause instanceof DOMException && cause.name === "AbortError") throw cause;
        throw new AdminApiError(cause instanceof Error ? cause.message : "Network request failed", 0, "NETWORK_ERROR");
      }
      let envelope: unknown;
      try { envelope = await response.json(); }
      catch {
        if (!response.ok) throw new AdminApiError(`Request failed (${response.status})`, response.status);
        throw new AdminApiError("运营工作台响应格式无效。", 502, "INVALID_RESPONSE");
      }
      if (!response.ok) {
        const error = record(envelope) && record(envelope.error) ? envelope.error : null;
        throw new AdminApiError(typeof error?.message === "string" ? error.message : `Request failed (${response.status})`, response.status,
          typeof error?.code === "string" ? error.code : undefined);
      }
      if (!record(envelope) || !Object.keys(envelope).every(key => ["success", "data", "request_id"].includes(key)) ||
        envelope.success !== true || !(envelope.request_id === undefined || typeof envelope.request_id === "string") || !validWorkbenchSnapshot(envelope.data, brandId)) {
        throw new AdminApiError("运营工作台响应格式无效。", 502, "INVALID_RESPONSE");
      }
      return envelope.data;
    },
  };
}
