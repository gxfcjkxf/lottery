import { createApiClient, ApiError } from "./api";
export const currencies = ["CNY", "USD", "THB", "USDT"] as const;
export type Currency = (typeof currencies)[number];
export interface CashRule {
  currency: Currency;
  rate: string;
  deposit_min: string;
  deposit_max: string;
  withdrawal_min: string;
  withdrawal_max: string;
  fee: string;
  mode: "count" | "amount";
  threshold: string;
  enabled: boolean;
}
export interface CashConfig {
  version: number;
  rules: CashRule[];
}
export interface CashDestination {
  id: string;
  currency: Currency;
  channel: string;
  holder: string;
  institution: string;
  number: string;
  enabled: boolean;
  version: number;
}
export interface CashBalance {
  currency: Currency;
  deposited: string;
  withdrawable: string;
  reserved: string;
}
export interface AgentPointTransfer {id:string;member_id:string;member_account:string|null;amount:string;created_at:string}
export interface AgentPointTransferPage {items:AgentPointTransfer[];total_count:string}
export interface AgentWalletRecord {id:string;kind:CashOrder["kind"]|"transfer";created_at:string;order:CashOrder|null;transfer:AgentPointTransfer|null}
export interface CashOrder {
  automatic: boolean;
  credited_at:string;
  sender:string;
  id: string;
  brand_id: string;
  member_id: string;
  member_account: string;
  kind: "deposit" | "withdrawal" | "to_points" | "from_points";
  currency: Currency;
  amount: string;
  requested_amount: string;
  points: string;
  fee: string;
  rate: string;
  state: string;
  version: number;
  destination: CashDestination;
  proof: string;
  reference: string;
  created_at: string;
  expires_at: string;
}
export interface CashState {
  auto_usdt: boolean;
  actor_context: string;
  brand_id: string;
  member_id: string;
  balances: CashBalance[];
  destinations: CashDestination[];
  platform: CashConfig;
  brand: CashConfig;
}
const object = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object" && !Array.isArray(v);
export const cashDecimal = (v: unknown): v is string =>
  typeof v === "string" && /^(0|[1-9]\d*)(\.\d{1,6})?$/.test(v);
const currency = (v: unknown): v is Currency =>
  currencies.includes(v as Currency);
const version = (v: unknown) =>
  typeof v === "number" && Number.isSafeInteger(v) && v >= 0;
export function isCashConfig(v: unknown): v is CashConfig {
  return (
    object(v) &&
    version(v.version) &&
    Array.isArray(v.rules) &&
    v.rules.length === 4 &&
    new Set(v.rules.map((r) => r.currency)).size === 4 &&
    v.rules.every(
      (r) =>
        object(r) &&
        currency(r.currency) &&
        typeof r.enabled === "boolean" &&
        typeof r.rate === "string" &&
        (r.rate === "" || cashDecimal(r.rate)) &&
        [
          "deposit_min",
          "deposit_max",
          "withdrawal_min",
          "withdrawal_max",
          "fee",
          "threshold",
        ].every((k) => cashDecimal(r[k])) &&
        ["count", "amount"].includes(String(r.mode)),
    )
  );
}
const uuid = (v: unknown) =>
  typeof v === "string" &&
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(v);
const money = (v: unknown, c: Currency): v is string =>
  cashDecimal(v) &&
  (c === "USDT" || !v.includes(".") || v.split(".")[1].length <= 2) &&
  minor(v) <= 9223372036854775807n;
export function isCashDestination(v: unknown): v is CashDestination {
  return (
    object(v) &&
    uuid(v.id) &&
    currency(v.currency) &&
    version(v.version) &&
    Number(v.version) > 0 &&
    typeof v.enabled === "boolean" &&
    ["channel", "holder", "institution", "number"].every(
      (k) => typeof v[k] === "string",
    ) &&
    v.channel === (v.currency === "USDT" ? "TRON/TRC20" : "bank")
  );
}
export function isCashOrder(v: unknown): v is CashOrder {
  if (
    !object(v) ||
    typeof v.automatic !== "boolean" ||
    !["id", "brand_id", "member_id"].every((k) => uuid(v[k])) ||
    ![
      "member_account",
      "credited_at",
      "sender",
      "created_at",
      "expires_at",
      "proof",
      "reference",
      "rate",
    ].every((k) => typeof v[k] === "string") ||
    !Number.isFinite(Date.parse(String(v.created_at))) ||
    !["deposit", "withdrawal", "to_points", "from_points"].includes(
      String(v.kind),
    ) ||
    ![
      "pending",
      "submitted",
      "confirmed",
      "cancelled",
      "expired",
      "completed",
      "reviewing",
      "processing",
      "paid",
      "rejected",
      "failed",
    ].includes(String(v.state)) ||
    !currency(v.currency) ||
    !version(v.version) ||
    Number(v.version) < 1 ||
    !["amount", "requested_amount", "fee"].every((k) =>
      money(v[k], v.currency as Currency),
    ) ||
    !cashDecimal(v.points) ||
    (v.points.includes(".") && v.points.split(".")[1].length > 2) ||
    minor(String(v.amount)) <= 0n ||
    minor(String(v.fee)) >= minor(String(v.amount))
  )
    return false;
  return v.kind === "deposit" || v.kind === "withdrawal"
    ? isCashDestination(v.destination) && v.destination.currency === v.currency
    : object(v.destination) && v.destination.id === "";
}
const balance = (v: unknown): v is CashBalance =>
  object(v) &&
  currency(v.currency) &&
  ["deposited", "withdrawable", "reserved"].every((k) =>
    money(v[k], v.currency as Currency),
  );
export function isCashState(v: unknown): v is CashState {
  return (
    object(v) &&
    uuid(v.brand_id) &&
    typeof v.auto_usdt === "boolean" &&
    uuid(v.member_id) &&
    typeof v.actor_context === "string" &&
    /^[a-f0-9]{64}$/.test(v.actor_context) &&
    isCashConfig(v.platform) &&
    isCashConfig(v.brand) &&
    Array.isArray(v.destinations) &&
    v.destinations.every(isCashDestination) &&
    Array.isArray(v.balances) &&
    v.balances.length === 4 &&
    new Set(v.balances.map((b) => b.currency)).size === 4 &&
    v.balances.every(balance)
  );
}
export function minor(value: string): bigint {
  if (!cashDecimal(value)) throw new Error("Invalid currency amount");
  const [whole, fraction = ""] = value.split(".");
  return BigInt(whole) * 1000000n + BigInt(fraction.padEnd(6, "0"));
}
export function decimal(value: bigint): string {
  const negative = value < 0n,
    n = negative ? -value : value,
    w = n / 1000000n,
    f = (n % 1000000n).toString().padStart(6, "0").replace(/0+$/, "");
  return (negative ? "-" : "") + w.toString() + (f ? "." + f : "");
}
export function exchangeResult(
  amount: string,
  rate: string,
  toPoints: boolean,
  currency: Currency,
): string {
  const a = minor(amount),
    r = minor(rate);
  if (a <= 0n || r <= 0n) return "0";
  const n = toPoints
    ? ((a * r) / 10000000000n) * 10000n
    : ((a * 1000000n) / r / (currency === "USDT" ? 1n : 10000n)) *
      (currency === "USDT" ? 1n : 10000n);
  return decimal(n);
}
export function emptyDestination(currency: Currency): CashDestination {
  return {
    id: "",
    version: 0,
    currency,
    channel: currency === "USDT" ? "TRON/TRC20" : "bank",
    holder: "",
    institution: "",
    number: "",
    enabled: true,
  };
}

export function createCashApi(
  base: string,
  brandId?: string,
  actorContext?: string,
) {
  const client = createApiClient();
  const headers = (): Record<string, string> => ({
    ...(brandId ? { "X-Brand-ID": brandId } : {}),
    ...(actorContext ? { "X-Cash-Actor-Context": actorContext } : {}),
  });
  const invalid = () =>
    new ApiError("Invalid currency response", 502, "INVALID_RESPONSE");
  async function get<T>(
    path: string,
    guard: (v: unknown) => v is T,
  ): Promise<T> {
    const value = await client.request<unknown>(base + path, {
      headers: headers(),
    });
    if (!guard(value)) throw invalid();
    return value;
  }
  async function write<T>(
    path: string,
    body: unknown,
    key: string,
    guard: (v: unknown) => v is T,
    method = "POST",
  ): Promise<T> {
    const value = await client.request<unknown>(base + path, {
      method,
      headers: {
        ...headers(),
        "Content-Type": "application/json",
        "Idempotency-Key": key,
      },
      body: JSON.stringify(body),
    });
    if (!guard(value)) throw invalid();
    return value;
  }
  return {
    balances: (memberId: string) =>
      get(
        "/cash/balances/" + encodeURIComponent(memberId),
        (v): v is CashBalance[] =>
          Array.isArray(v) && v.length === 4 && v.every(balance),
      ),
    state: () => get("/cash", isCashState),
    config: () => get("/cash/config", isCashConfig),
    destinations: () =>
      get(
        "/cash/destinations",
        (v): v is CashDestination[] =>
          Array.isArray(v) && v.every(isCashDestination),
      ),
    agentRecords:(page:number,size:number)=>get(`/wallet/records?limit=${size}&offset=${(page-1)*size}`,(v):v is {items:AgentWalletRecord[];total_count:string}=>object(v)&&Array.isArray(v.items)&&v.items.every(item=>object(item)&&typeof item.id==='string'&&typeof item.created_at==='string'&&(item.kind==='transfer'?item.order===null&&object(item.transfer)&&typeof item.transfer.member_id==='string'&&(item.transfer.member_account===null||typeof item.transfer.member_account==='string')&&cashDecimal(item.transfer.amount):item.transfer===null&&isCashOrder(item.order)&&item.order.kind===item.kind))&&typeof v.total_count==='string'&&/^\d+$/.test(v.total_count)),
    agentTransfers: (page:number,size:number)=>get(`/transfers?limit=${size}&offset=${(page-1)*size}`,(v):v is AgentPointTransferPage=>object(v)&&Array.isArray(v.items)&&v.items.every(item=>object(item)&&typeof item.id==='string'&&typeof item.member_id==='string'&&(item.member_account===null||typeof item.member_account==='string')&&cashDecimal(item.amount)&&typeof item.created_at==='string'&&Number.isFinite(Date.parse(item.created_at)))&&typeof v.total_count==='string'&&/^\d+$/.test(v.total_count)),
    orders: (kind: string, page: number, size: number) =>
      get(
        `/cash/orders?kind=${encodeURIComponent(kind)}&limit=${size}&offset=${(page - 1) * size}`,
        (v): v is { items: CashOrder[]; total_count: string } =>
          object(v) &&
          Array.isArray(v.items) &&
          v.items.every(isCashOrder) &&
          typeof v.total_count === "string" &&
          /^\d+$/.test(v.total_count),
      ),
    writeOrder: async (path: string, body: unknown, key: string) => {
      const result = await write(path, body, key, isCashOrder);
      if (!object(body)) throw invalid();
      if (
        path === "/cash/deposits" &&
        (result.kind !== "deposit" ||
          result.currency !== body.currency ||
          minor(result.requested_amount) !== minor(String(body.amount)))
      )
        throw invalid();
      if (
        path === "/cash/withdrawals" &&
        (result.kind !== "withdrawal" ||
          result.currency !== body.currency ||
          minor(result.amount) !== minor(String(body.amount)) ||
          minor(result.fee) !== minor(String(body.fee)) ||
          result.destination.id !== body.destination_id ||
          result.destination.version !== body.destination_version)
      )
        throw invalid();
      if (path === "/cash/exchanges") {
        if (
          result.kind !== body.direction ||
          result.currency !== body.currency ||
          result.rate !== body.rate
        )
          throw invalid();
        const expected = exchangeResult(
          String(body.amount),
          String(body.rate),
          body.direction === "to_points",
          body.currency as Currency,
        );
        if (
          body.direction === "to_points"
            ? minor(result.amount) !== minor(String(body.amount)) ||
              minor(result.points) !== minor(expected)
            : minor(result.points) !== minor(String(body.amount)) ||
              minor(result.amount) !== minor(expected)
        )
          throw invalid();
      }
      if (path.includes("/orders/")) {
        const id = path.split("/").at(-2);
        if (result.id !== id || result.version !== Number(body.version) + 1)
          throw invalid();
      }
      return result;
    },
    saveDestination: (body: CashDestination, key: string) =>
      write("/cash/destinations", body, key, isCashDestination),
    saveConfig: (body: CashConfig, key: string) =>
      write("/cash/config", body, key, isCashConfig, "PUT"),
  };
}

// Preserve an unconfirmed financial intent across page remounts in this session.
export interface PendingCashWrite {
  path: string;
  body: unknown;
  key: string;
  destination: boolean;
  config: boolean;
  dialog: string;
  detail: CashOrder | null;
}
export const pendingCashWrites = new Map<string, PendingCashWrite>();
