export type RechargeState = "pending" | "confirmed" | "cancelled";

export interface MemberRecharge {
  id: string;
  brand_id: string;
  member_id: string;
  points: string;
  state: RechargeState;
  version: string;
  created_at: string;
  confirmed_at: string | null;
  ledger_entry_id: string | null;
}

export interface MemberRechargePage {
  brand_id: string;
  member_id: string;
  snapshot_at: string;
  state: RechargeState | null;
  items: MemberRecharge[];
  limit: number;
  offset: number;
  total_count: string;
}

export interface RechargeScope {
  brand_id: string;
  member_id: string;
}

export interface RechargeListOptions {
  limit?: number;
  offset?: number;
  state?: RechargeState | null;
}

export class RechargeApiError extends Error {
  constructor(message: string, readonly status: number, readonly code?: string) {
    super(message);
    this.name = "RechargeApiError";
  }
}

const states = new Set<RechargeState>(["pending", "confirmed", "cancelled"]);
const maxInt64 = 9223372036854775807n;
const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const timestampPattern = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(\.\d{1,9})?Z$/;
const monthDaysBefore: Record<string, bigint> = { "01": 0n, "02": 31n, "03": 59n, "04": 90n, "05": 120n, "06": 151n, "07": 181n, "08": 212n, "09": 243n, "10": 273n, "11": 304n, "12": 334n };

export function rechargeBasePath(brandCode?: string): string {
  return brandCode ? `/api/v1/b/${encodeURIComponent(brandCode)}` : "/api/v1";
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function hasExactKeys(value: Record<string, unknown>, expected: readonly string[]): boolean {
  const keys = Object.keys(value);
  return keys.length === expected.length && expected.every((key) => Object.hasOwn(value, key));
}

function isUUID(value: unknown): value is string {
  return typeof value === "string" && uuidPattern.test(value);
}

function isTimestamp(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const match = timestampPattern.exec(value);
  if (!match) return false;
  const [, year, month, day, hour, minute, second] = match;
  if (year === "0000") return false;
  const date = new Date(value);
  return Number.isFinite(date.getTime()) && date.toISOString().slice(0, 19) === `${year}-${month}-${day}T${hour}:${minute}:${second}`;
}

function timestampParts(value: string): { seconds: bigint; nanos: bigint } {
  const match = timestampPattern.exec(value)!;
  const year = BigInt(match[1]!);
  const month = BigInt(match[2]!);
  const day = BigInt(match[3]!);
  const hour = BigInt(match[4]!);
  const minute = BigInt(match[5]!);
  const second = BigInt(match[6]!);
  const leap = year % 4n === 0n && (year % 100n !== 0n || year % 400n === 0n);
  const daysBeforeYear = 365n * (year - 1n) + (year - 1n) / 4n - (year - 1n) / 100n + (year - 1n) / 400n;
  const daysBeforeMonth = monthDaysBefore[match[2]!]! + (leap && month > 2n ? 1n : 0n);
  const days = daysBeforeYear + daysBeforeMonth + day - 1n;
  return {
    seconds: days * 86_400n + hour * 3_600n + minute * 60n + second,
    nanos: BigInt((match[7]?.slice(1) ?? "").padEnd(9, "0") || "0"),
  };
}

function orderedUniqueItems(items: MemberRecharge[]): boolean {
  const ids = new Set<string>();
  for (let index = 0; index < items.length; index++) {
    const item = items[index]!;
    const normalizedId = item.id.toLowerCase();
    if (ids.has(normalizedId)) return false;
    ids.add(normalizedId);
    if (index === 0) continue;
    const previous = items[index - 1]!;
    const before = timestampParts(previous.created_at);
    const current = timestampParts(item.created_at);
    if (before.seconds < current.seconds || (before.seconds === current.seconds && before.nanos < current.nanos)) return false;
    if (before.seconds === current.seconds && before.nanos === current.nanos && previous.id.toLowerCase() <= normalizedId) return false;
  }
  return true;
}

function isPositiveInt64(value: unknown): value is string {
  if (typeof value !== "string" || !/^[1-9]\d*$/.test(value)) return false;
  try { return BigInt(value) <= maxInt64; } catch { return false; }
}

function isNonNegativeInt64(value: unknown): value is string {
  if (value === "0") return true;
  return isPositiveInt64(value);
}

export function isMemberRecharge(value: unknown): value is MemberRecharge {
  if (!isRecord(value) || !hasExactKeys(value, ["id", "brand_id", "member_id", "points", "state", "version", "created_at", "confirmed_at", "ledger_entry_id"])) return false;
  if (!isUUID(value.id) || !isUUID(value.brand_id) || !isUUID(value.member_id) || !isPositiveInt64(value.points) || !isPositiveInt64(value.version) || !isTimestamp(value.created_at) || !states.has(value.state as RechargeState)) return false;
  if (value.state === "confirmed") return isTimestamp(value.confirmed_at) && isUUID(value.ledger_entry_id);
  return value.confirmed_at === null && value.ledger_entry_id === null;
}

export function isMemberRechargePage(value: unknown): value is MemberRechargePage {
  return isRecord(value)
    && hasExactKeys(value, ["brand_id", "member_id", "snapshot_at", "state", "items", "limit", "offset", "total_count"])
    && isUUID(value.brand_id)
    && isUUID(value.member_id)
    && isTimestamp(value.snapshot_at)
    && (value.state === null || states.has(value.state as RechargeState))
    && Array.isArray(value.items)
    && value.items.every(isMemberRecharge)
    && Number.isInteger(value.limit) && Number(value.limit) >= 1 && Number(value.limit) <= 100
    && Number.isInteger(value.offset) && Number(value.offset) >= 0 && Number(value.offset) <= 1_000_000
    && isNonNegativeInt64(value.total_count);
}

export function createRechargeApi(options: { brandCode?: string; fetcher?: typeof fetch } = {}) {
  const basePath = rechargeBasePath(options.brandCode ?? import.meta.env.VITE_BRAND_CODE ?? undefined);
  const fetcher = options.fetcher ?? fetch;

  async function request(path: string, signal?: AbortSignal): Promise<unknown> {
    let response: Response;
    try {
      response = await fetcher(`${basePath}${path}`, {
        method: "GET",
        credentials: "same-origin",
        headers: { Accept: "application/json" },
        signal,
      });
    } catch (cause) {
      throw new RechargeApiError(cause instanceof Error ? cause.message : "Request failed", 0);
    }
    let body: unknown;
    try {
      const text = await response.text();
      if (!text) throw new Error("Empty response");
      body = JSON.parse(text) as unknown;
    } catch {
      throw new RechargeApiError("The server returned an unreadable response.", response.status || 502, "INVALID_RESPONSE");
    }
    if (!response.ok) {
      const envelope = isRecord(body) && isRecord(body.error) ? body.error : null;
      throw new RechargeApiError(typeof envelope?.message === "string" ? envelope.message : `Request failed (${response.status})`, response.status, typeof envelope?.code === "string" ? envelope.code : undefined);
    }
    if (response.status !== 200) throw new RechargeApiError("The server returned an unexpected status.", 502, "INVALID_RESPONSE");
    if (!isRecord(body) || body.success !== true || !Object.hasOwn(body, "data")) throw new RechargeApiError("The server returned an invalid response.", 502, "INVALID_RESPONSE");
    return body.data;
  }

  function expectedScope(scope: RechargeScope): void {
    if (!isUUID(scope.brand_id) || !isUUID(scope.member_id)) throw new TypeError("A valid recharge member scope is required.");
  }

  return {
    async list(scope: RechargeScope, options: RechargeListOptions = {}, signal?: AbortSignal): Promise<MemberRechargePage> {
      expectedScope(scope);
      const limit = options.limit ?? 20;
      const offset = options.offset ?? 0;
      const state = options.state ?? null;
      if (!Number.isInteger(limit) || limit < 1 || limit > 100 || !Number.isInteger(offset) || offset < 0 || offset > 1_000_000 || (state !== null && !states.has(state))) throw new TypeError("Invalid recharge list options.");
      const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
      if (state) query.set("state", state);
      const value = await request(`/recharges?${query}`, signal);
      if (!isMemberRechargePage(value) || value.limit !== limit || value.offset !== offset || value.state !== state || value.items.length > limit || (value.items.length > 0 && BigInt(value.total_count) < BigInt(offset + value.items.length)) || !orderedUniqueItems(value.items) || (state !== null && value.items.some((item) => item.state !== state))) {
        throw new RechargeApiError("The server returned an invalid recharge page.", 502, "INVALID_RESPONSE");
      }
      if (value.brand_id !== scope.brand_id || value.member_id !== scope.member_id || value.items.some((item) => item.brand_id !== scope.brand_id || item.member_id !== scope.member_id)) throw new RechargeApiError("The response belongs to a different signed-in account.", 401, "SCOPE_MISMATCH");
      return value;
    },
    async get(scope: RechargeScope, id: string, signal?: AbortSignal): Promise<MemberRecharge> {
      expectedScope(scope);
      if (!isUUID(id)) throw new TypeError("A valid recharge ID is required.");
      const value = await request(`/recharges/${encodeURIComponent(id)}`, signal);
      if (!isMemberRecharge(value) || value.id !== id) throw new RechargeApiError("The server returned an invalid recharge record.", 502, "INVALID_RESPONSE");
      if (value.brand_id !== scope.brand_id || value.member_id !== scope.member_id) throw new RechargeApiError("The response belongs to a different signed-in account.", 401, "SCOPE_MISMATCH");
      return value;
    },
  };
}

export type RechargeApi = ReturnType<typeof createRechargeApi>;
