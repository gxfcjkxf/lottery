export const WALLET_SOURCES = ["recharge", "winning", "gift", "commission"] as const;
export type WalletSource = (typeof WALLET_SOURCES)[number];

export const WALLET_STATES = [
  "available",
  "manual_frozen",
  "system_frozen",
  "withdrawal",
] as const;
export type WalletState = (typeof WALLET_STATES)[number];
export type SourceBuckets = Record<WalletSource, Record<WalletState, string>>;

export interface WalletDTO {
  account_id: string;
  brand_id: string;
  member_id: string;
  version: number;
  display_points: string;
  available_points: string;
  frozen_points: string;
  withdrawal_points: string;
  recharge_points: string;
  winning_points: string;
  gift_points: string;
  commission_points: string;
  manual_frozen_points: string;
  system_frozen_points: string;
  by_source: SourceBuckets;
}

const LEGACY_SOURCES = ["recharge", "winning", "gift"] as const;
const WALLET_KEYS = [
  "account_id", "brand_id", "member_id", "version", "display_points",
  "available_points", "frozen_points", "withdrawal_points", "recharge_points",
  "winning_points", "gift_points", "commission_points", "manual_frozen_points",
  "system_frozen_points", "by_source",
] as const;
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const MAX_INT64 = 9223372036854775807n;

function record(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function exact(value: Record<string, unknown>, keys: readonly string[]): boolean {
  return Object.keys(value).length === keys.length && keys.every((key) => Object.hasOwn(value, key));
}

function amount(value: unknown, signed: boolean): value is string {
  if (typeof value !== "string" || !(signed ? /^(0|-?[1-9]\d*)$/ : /^(0|[1-9]\d*)$/).test(value)) return false;
  try {
    const parsed = BigInt(value);
    return parsed >= (signed ? -9223372036854775808n : 0n) && parsed <= MAX_INT64;
  } catch {
    return false;
  }
}

/** Current wallet responses are closed four-source DTOs. */
export function isWalletDTO(value: unknown): value is WalletDTO {
  if (!record(value) || !exact(value, WALLET_KEYS) ||
    !UUID.test(String(value.account_id)) || !UUID.test(String(value.brand_id)) || !UUID.test(String(value.member_id)) ||
    !Number.isSafeInteger(value.version) || Number(value.version) < 0 ||
    !["display_points", "available_points", "frozen_points", "withdrawal_points", "recharge_points", "winning_points", "gift_points", "commission_points", "manual_frozen_points", "system_frozen_points"].every((key) => amount(value[key], false)) ||
    !record(value.by_source) || !exact(value.by_source, WALLET_SOURCES)) return false;
  const buckets = normalizeSourceBuckets(value.by_source);
  if (!buckets) return false;
  let aggregate = 0n;
  for (const source of WALLET_SOURCES) {
    for (const state of WALLET_STATES) {
      aggregate += BigInt(buckets[source][state]);
      if (aggregate > MAX_INT64) return false;
    }
  }
  const total = (state: WalletState) => WALLET_SOURCES.reduce((sum, source) => sum + BigInt(buckets[source][state]), 0n);
  const available = total("available");
  const manual = total("manual_frozen");
  const system = total("system_frozen");
  return BigInt(value.available_points as string) === available &&
    BigInt(value.frozen_points as string) === manual + system &&
    BigInt(value.withdrawal_points as string) === total("withdrawal") &&
    BigInt(value.manual_frozen_points as string) === manual &&
    BigInt(value.system_frozen_points as string) === system &&
    BigInt(value.display_points as string) === available + manual + system &&
    BigInt(value.recharge_points as string) === BigInt(buckets.recharge.available) &&
    BigInt(value.winning_points as string) === BigInt(buckets.winning.available) &&
    BigInt(value.gift_points as string) === BigInt(buckets.gift.available) &&
    BigInt(value.commission_points as string) === BigInt(buckets.commission.available);
}

/**
 * Read immutable legacy 12-bucket snapshots without modifying persisted history.
 * Four-source snapshots stay strict; the returned value is always four-source.
 */
export function normalizeSourceBuckets(value: unknown, signed = false): SourceBuckets | null {
  if (!record(value)) return null;
  const legacy = exact(value, LEGACY_SOURCES);
  if (!legacy && !exact(value, WALLET_SOURCES)) return null;
  const normalized = {} as SourceBuckets;
  for (const source of WALLET_SOURCES) {
    const buckets = source === "commission" && legacy ? undefined : value[source];
    if (source === "commission" && legacy) {
      normalized.commission = { available: "0", manual_frozen: "0", system_frozen: "0", withdrawal: "0" };
      continue;
    }
    if (!record(buckets) || !exact(buckets, WALLET_STATES) || !WALLET_STATES.every((state) => amount(buckets[state], signed))) return null;
    normalized[source] = {
      available: buckets.available as string,
      manual_frozen: buckets.manual_frozen as string,
      system_frozen: buckets.system_frozen as string,
      withdrawal: buckets.withdrawal as string,
    };
  }
  if (!signed) {
    let aggregate = 0n;
    for (const source of WALLET_SOURCES) {
      for (const state of WALLET_STATES) {
        aggregate += BigInt(normalized[source][state]);
        if (aggregate > MAX_INT64) return null;
      }
    }
  }
  return normalized;
}

/** Normalize immutable ledger entries, including old idempotent write receipts, in memory. */
export function normalizeLedgerSnapshots<T>(value: T): T | (T & {
  before_snapshot: SourceBuckets;
  delta_snapshot: SourceBuckets;
  after_snapshot: SourceBuckets;
}) | null {
  if (!record(value)) return null;
  const before = normalizeSourceBuckets(value.before_snapshot);
  const delta = normalizeSourceBuckets(value.delta_snapshot, true);
  const after = normalizeSourceBuckets(value.after_snapshot);
  if (!before || !delta || !after) return null;
  return { ...value, before_snapshot: before, delta_snapshot: delta, after_snapshot: after } as T & {
    before_snapshot: SourceBuckets;
    delta_snapshot: SourceBuckets;
    after_snapshot: SourceBuckets;
  };
}
