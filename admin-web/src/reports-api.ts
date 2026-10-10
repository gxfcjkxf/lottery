import { AdminApiError, type AdminAccount } from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";

const BASE = "/api/v1/admin/reports";
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const DAY_RE = /^\d{4}-\d{2}-\d{2}$/;
const INTEGER_RE = /^(0|[1-9]\d{0,127})$/;
const SIGNED_INTEGER_RE = /^(?:0|-?[1-9]\d{0,127})$/;
const ENTRY_TYPE_RE = /^[a-zA-Z0-9_.:-]{1,200}$/;
const MAX_LIMIT = 100;
const MAX_OFFSET = 1_000_000;
const MAX_WINDOW_MS = 93 * 24 * 60 * 60 * 1000;

export type ReportGroupBy = "day" | "game" | "member" | "entry_type";

export interface ReportQuery {
  from: string;
  to: string;
  group_by: ReportGroupBy;
  limit?: number;
  offset?: number;
  game_id?: string;
  member_id?: string;
}

export interface BettingTotals {
  order_count: string;
  stake_points: string;
  placed_count: string;
  won_count: string;
  lost_count: string;
  abnormal_count: string;
  cancelled_count: string;
  refund_points: string;
  settled_stake_points: string;
  unfinalized_stake_points: string;
  abnormal_stake_points: string;
  current_prize_points: string;
  correction_open_count: string;
}

export interface LedgerTotals {
  entry_count: string;
  net_points: string;
  recharge_points: string;
  prize_credit_points: string;
  prize_reversal_points: string;
  refund_points: string;
}

export interface ReportItem<Totals> {
  key: string;
  label: string;
  totals: Totals;
}

interface ReportBase<Totals> {
  brand_id: string;
  snapshot_at: string;
  timezone: string;
  query: Required<Pick<ReportQuery, "from" | "to" | "group_by" | "limit" | "offset">> & {
    game_id: string | null;
    member_id: string | null;
  };
  summary: Totals;
  items: ReportItem<Totals>[];
  total_groups: string;
}

export type BettingReport = ReportBase<BettingTotals>;

export type LedgerReport = ReportBase<LedgerTotals> & {
  balances: {
    account_count: string;
    available_points: string;
    frozen_points: string;
    withdrawal_points: string;
    total_points: string;
  };
};

type Envelope<T> = {
  success?: boolean;
  data?: T;
  error?: { code?: string; message?: string } | null;
};

type FetchLike = typeof fetch;

const BETTING_TOTAL_FIELDS: (keyof BettingTotals)[] = [
  "order_count", "stake_points", "placed_count", "won_count", "lost_count",
  "abnormal_count", "cancelled_count", "refund_points", "settled_stake_points",
  "unfinalized_stake_points", "abnormal_stake_points", "current_prize_points",
  "correction_open_count",
];
const LEDGER_TOTAL_FIELDS: (keyof LedgerTotals)[] = [
  "entry_count", "net_points", "recharge_points", "prize_credit_points",
  "prize_reversal_points", "refund_points",
];
function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function validUuid(value: unknown): value is string {
  return typeof value === "string" && UUID_RE.test(value);
}

function badInput(message: string): never {
  throw new AdminApiError(message, 0, "INVALID_INPUT");
}

function invalidResponse(message = "报告响应格式无效。"): never {
  throw new AdminApiError(message, 502, "INVALID_RESPONSE");
}

function validDateTime(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|([+-])(\d{2}):(\d{2}))$/.exec(value);
  if (!match) return false;
  const [, y, mo, d, h, mi, s, , zone, , zh, zm] = match;
  const year = Number(y), month = Number(mo), day = Number(d);
  const hour = Number(h), minute = Number(mi), second = Number(s);
  const days = month === 2
    ? (year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28)
    : ([4, 6, 9, 11].includes(month) ? 30 : 31);
  if (year < 1 || month < 1 || month > 12 || day < 1 || day > days ||
      hour > 23 || minute > 59 || second > 59) return false;
  if (zone !== "Z" && (Number(zh) > 14 || Number(zm) > 59 ||
      (Number(zh) === 14 && Number(zm) !== 0))) return false;
  return true;
}

function epoch(value: string): bigint {
  const fraction = /\.(\d{1,9})(?=Z|[+-]\d{2}:\d{2}$)/.exec(value)?.[1] ?? "";
  const millis = Date.parse(value.replace(/\.\d+(?=Z|[+-]\d{2}:\d{2}$)/,""));
  if (!Number.isFinite(millis)) badInput("报告日期无效。");
  return BigInt(millis)*1000000n + BigInt(fraction.padEnd(9,"0") || "0");
}

function normalizeQuery(query: ReportQuery, kind: "betting" | "ledger") {
  if (!query || !validDateTime(query.from) || !validDateTime(query.to))
    badInput("from 和 to 必须是有效 RFC3339 日期时间。");
  const from = epoch(query.from), to = epoch(query.to);
  if (to <= from || to - from > BigInt(MAX_WINDOW_MS)*1000000n)
    badInput("报告时间范围必须大于零且不超过 93 天。");
  const allowedGroups = kind === "betting" ? ["day", "game", "member"] : ["day", "entry_type"];
  if (!allowedGroups.includes(query.group_by)) badInput("group_by 不适用于此报告。");
  const limit = query.limit ?? 20;
  const offset = query.offset ?? 0;
  if (!Number.isSafeInteger(limit) || limit < 1 || limit > MAX_LIMIT ||
      !Number.isSafeInteger(offset) || offset < 0 || offset > MAX_OFFSET)
    badInput("分页参数无效。");
  if (query.game_id !== undefined && !validUuid(query.game_id)) badInput("game_id 必须是 UUID。");
  if (query.member_id !== undefined && !validUuid(query.member_id)) badInput("member_id 必须是 UUID。");
  if (kind === "ledger" && query.game_id !== undefined) badInput("账本报告不支持 game_id。");
  return {
    from: query.from,
    to: query.to,
    group_by: query.group_by,
    limit,
    offset,
    game_id: query.game_id ?? null,
    member_id: query.member_id ?? null,
  };
}

function validateTotals<T extends object>(
  value: unknown,
  fields: readonly (keyof T)[],
  signedFields: readonly (keyof T)[] = [],
): T {
  if (!isRecord(value)) invalidResponse();
  const record = value as Record<string, unknown>;
  const expected = new Set(fields as readonly string[]);
  if (Object.keys(value).length !== expected.size || Object.keys(value).some((key) => !expected.has(key)))
    invalidResponse();
  for (const field of fields) {
    const raw = record[field as string];
    const valid = signedFields.includes(field)
      ? typeof raw === "string" && SIGNED_INTEGER_RE.test(raw)
      : typeof raw === "string" && INTEGER_RE.test(raw);
    if (!valid) invalidResponse();
  }
  return value as T;
}

function validateBettingTotals(value: unknown): BettingTotals {
  const totals = validateTotals<BettingTotals>(value, BETTING_TOTAL_FIELDS);
  const statusCount = [totals.placed_count, totals.won_count, totals.lost_count,
    totals.abnormal_count, totals.cancelled_count].reduce((sum, current) => sum + BigInt(current), 0n);
  if (statusCount !== BigInt(totals.order_count)) invalidResponse();
  const partition = BigInt(totals.settled_stake_points) + BigInt(totals.unfinalized_stake_points) +
    BigInt(totals.abnormal_stake_points) + BigInt(totals.refund_points);
  if (partition !== BigInt(totals.stake_points)) invalidResponse();
  if (BigInt(totals.stake_points)<BigInt(totals.order_count) || BigInt(totals.correction_open_count)>BigInt(totals.order_count) ||
    (totals.cancelled_count==="0") !== (totals.refund_points==="0") ||
    (totals.abnormal_count==="0") !== (totals.abnormal_stake_points==="0") ||
    BigInt(totals.refund_points)<BigInt(totals.cancelled_count) || BigInt(totals.abnormal_stake_points)<BigInt(totals.abnormal_count) ||
    (totals.current_prize_points!=="0" && totals.won_count==="0") ||
    (totals.settled_stake_points!=="0" && BigInt(totals.won_count)+BigInt(totals.lost_count)===0n)) invalidResponse();
  return totals;
}

function validateLedgerTotals(value: unknown): LedgerTotals {
  const totals = validateTotals<LedgerTotals>(value, LEDGER_TOTAL_FIELDS, ["net_points"]);
  return totals;
}

function safeLabel(value: unknown): value is string {
  return typeof value === "string" && value.length > 0 && value.length <= 200 &&
    !/[\u0000-\u001f\u007f]/.test(value);
}

function validDay(value: string): boolean {
  if (!DAY_RE.test(value)) return false;
  const [year, month, day] = value.split("-").map(Number);
  const date = new Date(Date.UTC(year, month - 1, day));
  return date.getUTCFullYear() === year && date.getUTCMonth() === month - 1 && date.getUTCDate() === day;
}

function validateReport<T>(
  value: unknown,
  brandId: string,
  query: ReturnType<typeof normalizeQuery>,
  kind: "betting" | "ledger",
): ReportBase<T> & { balances?: LedgerReport["balances"] } {
  if (!isRecord(value) || value.brand_id !== brandId || !validDateTime(value.snapshot_at) ||
      typeof value.timezone !== "string" || !value.timezone || value.timezone.length > 100 ||
      !isRecord(value.query) || !Array.isArray(value.items) || value.items.length > query.limit ||
      typeof value.total_groups !== "string" || !INTEGER_RE.test(value.total_groups)) invalidResponse();
  const topLevelKeys = kind === "ledger"
    ? ["brand_id", "snapshot_at", "timezone", "query", "summary", "items", "total_groups", "balances"]
    : ["brand_id", "snapshot_at", "timezone", "query", "summary", "items", "total_groups"];
  if (Object.keys(value).length !== topLevelKeys.length || Object.keys(value).some((key) => !topLevelKeys.includes(key))) invalidResponse();
  try { new Intl.DateTimeFormat("en", { timeZone: value.timezone }); } catch { invalidResponse(); }
  const echoed = value.query;
  for (const key of ["from", "to", "group_by", "limit", "offset", "game_id", "member_id"] as const) {
    if (key === "from" || key === "to") {
      if (!validDateTime(echoed[key]) || epoch(echoed[key] as string)!==epoch(query[key])) invalidResponse("报告响应范围与请求不匹配。");
    } else if (echoed[key] !== query[key]) invalidResponse("报告响应范围与请求不匹配。");
  }
  if (Object.keys(echoed).length !== 7) invalidResponse();
  const summary = (kind === "betting" ? validateBettingTotals : validateLedgerTotals)(value.summary) as T;
  let balances: LedgerReport["balances"] | undefined;
  if (kind === "ledger") {
    const rawBalances = value.balances;
    if (!isRecord(rawBalances)) invalidResponse();
    const balanceFields = ["account_count", "available_points", "frozen_points", "withdrawal_points", "total_points"] as const;
    if (Object.keys(rawBalances).length !== balanceFields.length ||
        balanceFields.some((field) => typeof rawBalances[field] !== "string" || !INTEGER_RE.test(rawBalances[field] as string))) invalidResponse();
    const total = BigInt(rawBalances.total_points as string);
    if (BigInt(rawBalances.available_points as string) + BigInt(rawBalances.frozen_points as string) +
        BigInt(rawBalances.withdrawal_points as string) !== total) invalidResponse();
    balances = rawBalances as LedgerReport["balances"];
  } else if ("balances" in value) invalidResponse();
  const seen = new Set<string>();
  const items = value.items.map((raw): ReportItem<T> => {
    if (!isRecord(raw) || Object.keys(raw).length !== 3 ||
        Object.keys(raw).some((key) => !["key", "label", "totals"].includes(key)) ||
        typeof raw.key !== "string" || !safeLabel(raw.label)) invalidResponse();
    const key = raw.key;
    if (seen.has(key)) invalidResponse();
    seen.add(key);
    if (query.group_by === "day" && (!validDay(key) || raw.label !== key)) invalidResponse();
    if ((query.group_by === "game" || query.group_by === "member") && !validUuid(key)) invalidResponse();
    if ((query.group_by === "game" && query.game_id !== null && key.toLowerCase() !== query.game_id.toLowerCase()) ||
        (query.group_by === "member" && query.member_id !== null && key.toLowerCase() !== query.member_id.toLowerCase())) invalidResponse();
    if (query.group_by === "member" && raw.label !== key) invalidResponse();
    if (query.group_by === "game" && (!safeGameLabel(raw.label))) invalidResponse();
    if (query.group_by === "entry_type" && (!ENTRY_TYPE_RE.test(key) || raw.label !== key)) invalidResponse();
    const totals = (kind === "betting" ? validateBettingTotals : validateLedgerTotals)(raw.totals) as T;
    return { key, label: raw.label, totals };
  });
  if (BigInt(value.total_groups) < BigInt(items.length)) invalidResponse();
  return { brand_id: brandId, snapshot_at: value.snapshot_at, timezone: value.timezone,
    query: echoed as unknown as ReportBase<T>["query"], summary, items, total_groups: value.total_groups,
    ...(balances ? { balances } : {}) };
}

function safeGameLabel(value: string): boolean {
  return value.length <= 120 && !/[\u0000-\u001f\u007f]/.test(value);
}

export function reportsPermissions(
  account: AdminAccount,
  brandId: string,
): { betting: boolean; ledger: boolean } {
  const scoped = UUID_RE.test(account.id) && UUID_RE.test(brandId);
  const brand = brandPermissionSet(account, brandId.toLowerCase());
  return {
    betting: scoped && brand.has("report_betting.view.brand"),
    ledger: scoped && brand.has("report_ledger.view.brand"),
  };
}

export function createReportsApi(fetchImpl: FetchLike = fetch) {
  async function request<T>(brandId: string, report: "betting" | "ledger", query: ReportQuery): Promise<T> {
    if (!validUuid(brandId)) badInput("brand 必须是 UUID。");
    const normalized = normalizeQuery(query, report);
    const params = new URLSearchParams({
      from: normalized.from,
      to: normalized.to,
      group_by: normalized.group_by,
      limit: String(normalized.limit),
      offset: String(normalized.offset),
    });
    if (normalized.game_id !== null) params.set("game_id", normalized.game_id);
    if (normalized.member_id !== null) params.set("member_id", normalized.member_id);
    const response = await fetchImpl(`${BASE}/${report}?${params.toString()}`, {
      method: "GET",
      credentials: "same-origin",
      headers: { Accept: "application/json", "X-Brand-ID": brandId },
    });
    let envelope: Envelope<unknown>;
    try { envelope = await response.json() as Envelope<unknown>; }
    catch { throw new AdminApiError(response.ok ? "Invalid server response" : `Request failed (${response.status})`, response.status); }
    if (!response.ok || envelope.success !== true || envelope.data === undefined) {
      const error = typeof envelope.error === "object" && envelope.error !== null ? envelope.error : undefined;
      if (typeof envelope.error === "string") throw new AdminApiError("Invalid server response", 502, "INVALID_RESPONSE");
      throw new AdminApiError(error?.message || `Request failed (${response.status})`, response.status, error?.code);
    }
    return validateReport<T>(envelope.data, brandId, normalized, report) as T;
  }
  return {
    betting(brandId: string, query: ReportQuery): Promise<BettingReport> {
      return request<BettingReport>(brandId, "betting", query);
    },
    ledger(brandId: string, query: ReportQuery): Promise<LedgerReport> {
      return request<LedgerReport>(brandId, "ledger", query);
    },
  };
}
