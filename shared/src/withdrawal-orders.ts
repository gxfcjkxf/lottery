export type WithdrawalSource = "recharge" | "winning" | "gift";
export type WithdrawalState =
  | "reviewing"
  | "processing"
  | "paid"
  | "rejected"
  | "failed"
  | "cancelled";

export interface WithdrawalAllocation {
  source: WithdrawalSource;
  state: "available";
  points: string;
}

export interface WithdrawalOrder {
  id: string;
  brand_id: string;
  member_id: string;
  account_id: string;
  points: string;
  state: WithdrawalState;
  version: number;
  source_allocation: WithdrawalAllocation[];
  reserve_entry_id: string;
  release_entry_id: string | null;
  paid_entry_id: string | null;
  cycle_from_at: string | null;
  cycle_from_version: string;
  reserve_version: string;
  created_at: string;
  updated_at: string;
  reviewed_at: string | null;
  completed_at: string | null;
  decision_reason: string;
  audit_log_id: string;
}

export interface WithdrawalPage {
  brand_id: string;
  items: WithdrawalOrder[];
  limit: number;
  offset: number;
  has_more: boolean;
}

export interface WithdrawalHistoryItem {
  id: string;
  version: number;
  from_state: WithdrawalState | "";
  to_state: WithdrawalState;
  reason: string;
  actor_type: string;
  created_at: string;
  audit_log_id: string;
}

export interface WithdrawalHistory {
  brand_id: string;
  order_id: string;
  items: WithdrawalHistoryItem[];
}

export type WithdrawalAvailabilityReason =
  | "AVAILABLE"
  | "WITHDRAWAL_DISABLED"
  | "WITHDRAWAL_ELIGIBILITY_NOT_CONFIGURED"
  | "WITHDRAWAL_ACCOUNT_RESTRICTED";

export interface WithdrawalAvailability {
  actor_context: string;
  brand_id: string;
  member_id: string;
  policy_enabled: boolean;
  eligibility_configured: boolean;
  can_apply: boolean;
  reason_code: WithdrawalAvailabilityReason;
  min_points: string;
  max_points: string | null;
  allowed_sources: WithdrawalSource[];
  real_payments: false;
}

export interface CreateWithdrawalBody {
  points: string;
  source_allocation: WithdrawalAllocation[];
}

export type WithdrawalAction =
  | "approve"
  | "reject"
  | "cancel"
  | "fail"
  | "mark-paid";

export interface WithdrawalActionBody {
  version: number;
  reason: string;
}

const MAX_INT64 = 9223372036854775807n;
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const STATES: readonly WithdrawalState[] = ["reviewing", "processing", "paid", "rejected", "failed", "cancelled"];
const SOURCES: readonly WithdrawalSource[] = ["recharge", "winning", "gift"];
const ACTORS = ["user", "admin", "system"] as const;
const ORDER_KEYS = ["id", "brand_id", "member_id", "account_id", "points", "state", "version", "source_allocation", "reserve_entry_id", "release_entry_id", "paid_entry_id", "cycle_from_at", "cycle_from_version", "reserve_version", "created_at", "updated_at", "reviewed_at", "completed_at", "decision_reason", "audit_log_id"] as const;

function record(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}
function uuid(value: unknown): value is string { return typeof value === "string" && UUID.test(value); }
function dateTime(value: unknown): value is string {
  return typeof value === "string" && /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?(?:Z|[+-]\d\d:\d\d)$/.test(value) && Number.isFinite(Date.parse(value));
}
function nullableDate(value: unknown): value is string | null { return value === null || dateTime(value); }
function decimal(value: unknown): value is string { return typeof value === "string" && /^(0|[1-9]\d*)$/.test(value) && BigInt(value) <= MAX_INT64; }
function exactKeys(value: Record<string, unknown>, keys: readonly string[]): boolean {
  return keys.length === Object.keys(value).length && keys.every((key) => Object.hasOwn(value, key));
}
function validAllocation(value: unknown): value is WithdrawalAllocation[] {
  if (!Array.isArray(value) || value.length < 1 || value.length > 3 || value.some((item) =>
    !record(item) || !exactKeys(item, ["source", "state", "points"]) ||
    !SOURCES.includes(item.source as WithdrawalSource) || item.state !== "available" || !decimal(item.points) || BigInt(item.points) === 0n)) return false;
  const indexes = value.map((item) => SOURCES.indexOf((item as WithdrawalAllocation).source));
  return new Set(indexes).size === indexes.length && indexes.every((index, position) => position === 0 || indexes[position - 1]! < index);
}

export function isWithdrawalOrder(value: unknown): value is WithdrawalOrder {
  if (!record(value) || !exactKeys(value, ORDER_KEYS)) return false;
  const allocation = value.source_allocation;
  if (!validAllocation(allocation)) return false;
  const common = uuid(value.id) && uuid(value.brand_id) && uuid(value.member_id) && uuid(value.account_id) &&
    decimal(value.points) && BigInt(value.points) > 0n && STATES.includes(value.state as WithdrawalState) &&
    Number.isSafeInteger(value.version) && Number(value.version) > 0 && uuid(value.reserve_entry_id) &&
    (value.release_entry_id === null || uuid(value.release_entry_id)) && (value.paid_entry_id === null || uuid(value.paid_entry_id)) &&
    nullableDate(value.cycle_from_at) && decimal(value.cycle_from_version) && decimal(value.reserve_version) &&
    dateTime(value.created_at) && dateTime(value.updated_at) && nullableDate(value.reviewed_at) && nullableDate(value.completed_at) &&
    typeof value.decision_reason === "string" && uuid(value.audit_log_id) &&
    allocation.reduce((sum, item) => sum + BigInt(item.points), 0n) === BigInt(value.points) &&
    BigInt(value.reserve_version) > 0n && BigInt(value.reserve_version) > BigInt(value.cycle_from_version) &&
    ((value.cycle_from_at === null) === (BigInt(value.cycle_from_version) === 0n));
  if (!common) return false;
  const reviewed = value.reviewed_at !== null;
  const completed = value.completed_at !== null;
  switch (value.state as WithdrawalState) {
    case "reviewing": return value.version === 1 && !reviewed && !completed && value.release_entry_id === null && value.paid_entry_id === null;
    case "processing": return value.version === 2 && reviewed && !completed && value.release_entry_id === null && value.paid_entry_id === null;
    case "paid": return value.version === 3 && reviewed && completed && value.release_entry_id === null && uuid(value.paid_entry_id);
    case "rejected": return value.version === 2 && reviewed && completed && uuid(value.release_entry_id) && value.paid_entry_id === null;
    case "failed": return value.version === 3 && reviewed && completed && uuid(value.release_entry_id) && value.paid_entry_id === null;
    case "cancelled": return (value.version === 2 || value.version === 3) && completed && (value.version === 2 || reviewed) && uuid(value.release_entry_id) && value.paid_entry_id === null;
  }
}

export function isWithdrawalAvailability(value: unknown): value is WithdrawalAvailability {
  if (!record(value) || !exactKeys(value, ["actor_context", "brand_id", "member_id", "policy_enabled", "eligibility_configured", "can_apply", "reason_code", "min_points", "max_points", "allowed_sources", "real_payments"])) return false;
  const validReasons: readonly string[] = ["AVAILABLE", "WITHDRAWAL_DISABLED", "WITHDRAWAL_ELIGIBILITY_NOT_CONFIGURED", "WITHDRAWAL_ACCOUNT_RESTRICTED"];
  const sources = value.allowed_sources;
  const reason = value.reason_code;
  const validReason = reason === "AVAILABLE"
    ? value.can_apply === true && value.policy_enabled === true && value.eligibility_configured === true
    : reason === "WITHDRAWAL_DISABLED"
      ? value.can_apply === false && value.policy_enabled === false
      : reason === "WITHDRAWAL_ELIGIBILITY_NOT_CONFIGURED"
        ? value.can_apply === false && value.policy_enabled === true && value.eligibility_configured === false
        : reason === "WITHDRAWAL_ACCOUNT_RESTRICTED" && value.can_apply === false;
  return typeof value.actor_context === "string" && /^[0-9a-f]{64}$/i.test(value.actor_context) && uuid(value.brand_id) && uuid(value.member_id) && typeof value.policy_enabled === "boolean" && typeof value.eligibility_configured === "boolean" && typeof value.can_apply === "boolean" && validReasons.includes(String(reason)) && decimal(value.min_points) && BigInt(value.min_points) > 0n && (value.max_points === null || decimal(value.max_points) && BigInt(value.max_points) >= BigInt(value.min_points)) && Array.isArray(sources) && sources.length >= 1 && sources.length <= 3 && sources.every((source) => SOURCES.includes(source as WithdrawalSource)) && new Set(sources).size === sources.length && value.real_payments === false && validReason;
}

export function isWithdrawalPage(value: unknown): value is WithdrawalPage {
  return record(value) && exactKeys(value, ["brand_id", "items", "limit", "offset", "has_more"]) && uuid(value.brand_id) && Array.isArray(value.items) && value.items.every(isWithdrawalOrder) && Number.isSafeInteger(value.limit) && Number(value.limit) > 0 && Number.isSafeInteger(value.offset) && Number(value.offset) >= 0 && typeof value.has_more === "boolean";
}

export function isWithdrawalHistory(value: unknown): value is WithdrawalHistory {
  if (!record(value) || !exactKeys(value, ["brand_id", "order_id", "items"]) || !uuid(value.brand_id) || !uuid(value.order_id) || !Array.isArray(value.items)) return false;
  let previous: WithdrawalState | null = null;
  return value.items.every((item, index) => {
    if (!record(item) || !exactKeys(item, ["id", "version", "from_state", "to_state", "reason", "actor_type", "created_at", "audit_log_id"]) || !uuid(item.id) || item.version !== index + 1 || (index === 0 ? item.from_state !== "" || item.to_state !== "reviewing" : item.from_state !== previous) || !STATES.includes(item.to_state as WithdrawalState) || typeof item.reason !== "string" || !ACTORS.includes(item.actor_type as typeof ACTORS[number]) || !dateTime(item.created_at) || !uuid(item.audit_log_id)) return false;
    const transition = `${String(item.from_state)}>${String(item.to_state)}`;
    if (![">reviewing", "reviewing>processing", "reviewing>rejected", "reviewing>cancelled", "processing>paid", "processing>failed", "processing>cancelled"].includes(transition)) return false;
    previous = item.to_state as WithdrawalState;
    return true;
  });
}

export function isWithdrawalAllocation(value: unknown): value is WithdrawalAllocation {
  return record(value) && exactKeys(value, ["source", "state", "points"]) && SOURCES.includes(value.source as WithdrawalSource) && value.state === "available" && decimal(value.points) && BigInt(value.points) > 0n;
}

export function validateWithdrawalBody(body: CreateWithdrawalBody): boolean {
  if (!decimal(body.points) || BigInt(body.points) === 0n || !validAllocation(body.source_allocation)) return false;
  return body.source_allocation.reduce((sum, entry) => sum + BigInt(entry.points), 0n) === BigInt(body.points);
}
