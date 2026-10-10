import { createIdempotencyKey } from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";
import { isWalletDTO, normalizeLedgerSnapshots, normalizeSourceBuckets, type WalletDTO, type WalletSource as SharedWalletSource, type WalletState as SharedWalletState, type SourceBuckets as SharedSourceBuckets } from "@lottery/shared";

const BASE = "/api/v1/admin";

export type WalletSource = SharedWalletSource;
export type WalletState = SharedWalletState;
export type SourceBuckets = SharedSourceBuckets;
export type Wallet = WalletDTO;

export interface Recharge {
  id: string;
  brand_id: string;
  member_id: string;
  account_id: string;
  points: string;
  state: "pending" | "confirmed" | "cancelled";
  proof_reference: string;
  remark: string;
  version: number;
  created_by: string;
  confirmed_by?: string;
  created_at: string;
  confirmed_at?: string;
  ledger_entry_id?: string;
  audit_log_id?: string;
}

export interface SourceAllocation {
  source: WalletSource;
  state: WalletState;
  points: string;
}

export interface LedgerEntry {
  id: string;
  brand_id: string;
  account_id: string;
  member_id: string;
  entry_type: string;
  reference_type: string;
  reference_id: string;
  operation_key: string;
  before_snapshot: SourceBuckets;
  delta_snapshot: SourceBuckets;
  after_snapshot: SourceBuckets;
  source_allocation: SourceAllocation[];
  reason: string;
  actor_type: string;
  actor_id: string;
  request_id: string;
  reversal_of?: string;
  version: number;
  created_at: string;
}

export interface Reconciliation {
  consistent: boolean;
  account_id: string;
  member_id: string;
  version: number;
  entry_count: number;
  expected: SourceBuckets;
  actual: SourceBuckets;
  issues: string[];
}

export interface FinanceAccount {
  id: string;
  super_admin: boolean;
  brand_ids: string[];
  permissions: string[];
  permissions_by_brand?: Record<string, string[]>;
  platform_permissions?: string[];
}

type Envelope<T> = {
  success?: boolean;
  data?: T;
  error?: { code: string; message: string } | null;
};

export class FinanceApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code?: string,
  ) {
    super(message);
    this.name = "FinanceApiError";
  }
}

export function brandPermissions(
  account: FinanceAccount,
  brandId: string,
): Set<string> {
  return brandPermissionSet(account, brandId);
}

export function canViewWallet(
  account: FinanceAccount,
  brandId: string,
): boolean {
  return (
    brandPermissions(account, brandId).has("wallet.view.brand")
  );
}

export function canViewRecharges(
  account: FinanceAccount,
  brandId: string,
): boolean {
  return (
    brandPermissions(account, brandId).has("recharge.view.brand")
  );
}

export function canWriteFinance(
  account: FinanceAccount,
  brandId: string,
  permission:
    "recharge.write.brand" | "wallet.freeze.brand" | "wallet.adjust.brand",
): boolean {
  return (
    !account.super_admin && brandPermissions(account, brandId).has(permission)
  );
}

export function isUnsignedAmount(value: string): boolean {
  return /^(0|[1-9]\d*)$/.test(value);
}

export function isPositiveAmount(value: string): boolean {
  return isUnsignedAmount(value) && BigInt(value) > 0n;
}

export function isSignedAmount(value: string): boolean {
  return /^-?(0|[1-9]\d*)$/.test(value) && BigInt(value) !== 0n;
}

/** Groups arbitrary-size integer point strings without converting through Number. */
export function formatIntegerAmount(value: string): string {
  if (!/^-?\d+$/.test(value)) return value;
  const negative = value.startsWith("-");
  const digits = negative ? value.slice(1) : value;
  const grouped = digits.replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  return `${negative ? "−" : ""}${grouped}`;
}

export function createBodyKeyTracker() {
  let previousBody: string | undefined;
  let previousKey = "";
  const next = ((body: unknown): string => {
    const serialized = JSON.stringify(body);
    if (serialized !== previousBody) {
      previousBody = serialized;
      previousKey = createIdempotencyKey();
    }
    return previousKey;
  }) as ((body: unknown) => string) & { clear: () => void };
  next.clear = () => {
    previousBody = undefined;
    previousKey = "";
  };
  return next;
}

type FetchLike = typeof fetch;

export function createFinanceApi(fetcher: FetchLike = fetch) {
  async function request<T>(
    brandId: string,
    path: string,
    options: {
      method?: "GET" | "POST";
      body?: unknown;
      idempotencyKey?: string;
    } = {},
  ): Promise<T> {
    const headers = new Headers({
      Accept: "application/json",
      "X-Brand-ID": brandId,
    });
    if (options.body !== undefined)
      headers.set("Content-Type", "application/json");
    if (options.idempotencyKey)
      headers.set("Idempotency-Key", options.idempotencyKey);
    const response = await fetcher(`${BASE}${path}`, {
      method: options.method ?? "GET",
      credentials: "same-origin",
      headers,
      ...(options.body === undefined
        ? {}
        : { body: JSON.stringify(options.body) }),
    });
    let envelope: Envelope<T>;
    try {
      envelope = (await response.json()) as Envelope<T>;
    } catch {
      throw new FinanceApiError("Invalid server response; operation result is unconfirmed", 502, "INVALID_RESPONSE");
    }
    if (!envelope || typeof envelope !== "object" || Array.isArray(envelope)) {
      throw new FinanceApiError("Invalid server response; operation result is unconfirmed", 502, "INVALID_RESPONSE");
    }
    const detail = envelope.error;
    if (!response.ok && envelope.success === false && detail && typeof detail === "object" && !Array.isArray(detail)
      && typeof detail.code === "string" && detail.code.trim()
      && typeof detail.message === "string" && detail.message.trim()) {
      throw new FinanceApiError(detail.message, response.status, detail.code);
    }
    if (
      !response.ok ||
      envelope.success !== true ||
      envelope.data == null || typeof envelope.data !== "object" || Array.isArray(envelope.data)
    ) {
      throw new FinanceApiError("Invalid server response; operation result is unconfirmed", 502, "INVALID_RESPONSE");
    }
    const data = envelope.data;
    if (data && typeof data === "object" && !Array.isArray(data) &&
      ["before_snapshot", "delta_snapshot", "after_snapshot"].every((key) => Object.hasOwn(data, key))) {
      const normalized = normalizeLedgerSnapshots(data);
      if (!normalized) throw new FinanceApiError("The server returned an invalid wallet receipt snapshot.", 502, "INVALID_RESPONSE");
      return normalized as T;
    }
    return data;
  }

  return {
    async wallet(brandId: string, memberId: string) {
      const value = await request<unknown>(
        brandId,
        `/wallets/${encodeURIComponent(memberId)}`,
      );
      if (!isWalletDTO(value)) throw new FinanceApiError("The server returned an invalid wallet response.", 502, "INVALID_RESPONSE");
      return value;
    },
    async ledger(brandId: string, memberId: string, limit = 50, offset = 0) {
      const value = await request<unknown>(
        brandId,
        `/wallets/${encodeURIComponent(memberId)}/ledger?limit=${limit}&offset=${offset}`,
      );
      if (!value || typeof value !== "object" || Array.isArray(value) || !Array.isArray((value as { items?: unknown }).items))
        throw new FinanceApiError("The server returned an invalid wallet ledger response.", 502, "INVALID_RESPONSE");
      const items = (value as { items: unknown[] }).items.map((entry) => normalizeLedgerSnapshots(entry) as LedgerEntry | null);
      if (items.some((entry) => entry === null)) throw new FinanceApiError("The server returned an invalid wallet ledger snapshot.", 502, "INVALID_RESPONSE");
      return { items: items as LedgerEntry[] };
    },
    reconciliation(brandId: string, memberId: string) {
      return request<Reconciliation>(
        brandId,
        `/wallets/${encodeURIComponent(memberId)}/reconciliation`,
      );
    },
    recharges(brandId: string, memberId?: string, limit = 50, offset = 0) {
      const query = new URLSearchParams({
        limit: String(limit),
        offset: String(offset),
      });
      if (memberId) query.set("member_id", memberId);
      return request<{ items: Recharge[] }>(
        brandId,
        `/recharges?${query.toString()}`,
      );
    },
    createRecharge(
      brandId: string,
      body: {
        member_id: string;
        points: string;
        proof_reference?: string;
        remark?: string;
        reason: string;
      },
      idempotencyKey: string,
    ) {
      return request<unknown>(brandId, "/recharges", {
        method: "POST",
        body,
        idempotencyKey,
      });
    },
    confirmRecharge(
      brandId: string,
      rechargeId: string,
      body: { version: number; reason: string },
      idempotencyKey: string,
    ) {
      return request<unknown>(
        brandId,
        `/recharges/${encodeURIComponent(rechargeId)}/confirm`,
        { method: "POST", body, idempotencyKey },
      );
    },
    cancelRecharge(
      brandId: string,
      rechargeId: string,
      body: { version: number; reason: string },
      idempotencyKey: string,
    ) {
      return request<unknown>(
        brandId,
        `/recharges/${encodeURIComponent(rechargeId)}/cancel`,
        { method: "POST", body, idempotencyKey },
      );
    },
    freezeWallet(
      brandId: string,
      memberId: string,
      body: { points: string; reason: string },
      idempotencyKey: string,
    ) {
      return request<unknown>(
        brandId,
        `/wallets/${encodeURIComponent(memberId)}/freeze`,
        { method: "POST", body, idempotencyKey },
      );
    },
    unfreezeWallet(
      brandId: string,
      memberId: string,
      body: { entry_id: string; reason: string },
      idempotencyKey: string,
    ) {
      return request<unknown>(
        brandId,
        `/wallets/${encodeURIComponent(memberId)}/unfreeze`,
        { method: "POST", body, idempotencyKey },
      );
    },
    adjustWallet(
      brandId: string,
      memberId: string,
      body: {
        source: WalletSource;
        delta: string;
        reason: string;
      },
      idempotencyKey: string,
    ) {
      return request<unknown>(
        brandId,
        `/wallets/${encodeURIComponent(memberId)}/adjust`,
        { method: "POST", body, idempotencyKey },
      );
    },
  };
}

export type FinanceApi = ReturnType<typeof createFinanceApi>;
