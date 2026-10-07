import { isWalletDTO, normalizeLedgerSnapshots, type WalletDTO, type WalletSource, type WalletState, type SourceBuckets } from "@lottery/shared";

export type { WalletSource, WalletState, SourceBuckets };

export type Wallet = WalletDTO;

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

interface Envelope<T> {
  success?: boolean;
  data?: T;
  error?: string | { code?: string; message?: string } | null;
}

export class WalletApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code?: string,
  ) {
    super(message);
    this.name = "WalletApiError";
  }
}

export function walletBasePath(brandCode?: string): string {
  return brandCode ? `/api/v1/b/${encodeURIComponent(brandCode)}` : "/api/v1";
}

export function formatIntegerAmount(value: string): string {
  if (!/^-?\d+$/.test(value)) return value;
  const negative = value.startsWith("-");
  const digits = negative ? value.slice(1) : value;
  const grouped = digits.replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  return `${negative ? "−" : ""}${grouped}`;
}

export function createWalletApi(
  options: {
    brandCode?: string;
    fetcher?: typeof fetch;
  } = {},
) {
  const brandCode =
    options.brandCode ?? import.meta.env.VITE_BRAND_CODE ?? undefined;
  const fetcher = options.fetcher ?? fetch;
  const base = walletBasePath(brandCode);

  async function request<T>(path: string): Promise<T> {
    let response: Response;
    try {
      response = await fetcher(`${base}${path}`, {
        method: "GET",
        credentials: "same-origin",
        headers: { Accept: "application/json" },
      });
    } catch (cause) {
      throw new WalletApiError(
        cause instanceof Error ? cause.message : "网络暂不可用",
        0,
      );
    }
    let envelope: Envelope<T>;
    try {
      envelope = (await response.json()) as Envelope<T>;
    } catch {
      throw new WalletApiError(
        response.ok
          ? "服务器返回了无法读取的数据"
          : `Request failed (${response.status})`,
        response.status,
      );
    }
    if (
      !response.ok ||
      envelope.success !== true ||
      envelope.data === undefined
    ) {
      const detail =
        typeof envelope.error === "object" && envelope.error
          ? envelope.error
          : undefined;
      throw new WalletApiError(
        typeof envelope.error === "string"
          ? envelope.error
          : (detail?.message ?? `Request failed (${response.status})`),
        response.status,
        detail?.code,
      );
    }
    return envelope.data;
  }

  return {
    async wallet() {
      const value = await request<unknown>("/wallet");
      if (!isWalletDTO(value)) throw new WalletApiError("The server returned an invalid wallet response.", 502, "INVALID_RESPONSE");
      return value;
    },
    async ledger(limit = 50, offset = 0) {
      const query = new URLSearchParams({
        limit: String(limit),
        offset: String(offset),
      });
      const value = await request<unknown>(`/wallet/ledger?${query}`);
      if (!value || typeof value !== "object" || Array.isArray(value) || !Array.isArray((value as { items?: unknown }).items))
        throw new WalletApiError("The server returned an invalid wallet ledger response.", 502, "INVALID_RESPONSE");
      const items = (value as { items: unknown[] }).items.map((entry) => normalizeLedgerSnapshots(entry) as LedgerEntry | null);
      if (items.some((entry) => entry === null)) throw new WalletApiError("The server returned an invalid wallet ledger snapshot.", 502, "INVALID_RESPONSE");
      return { items: items as LedgerEntry[] };
    },
  };
}

export type WalletApi = ReturnType<typeof createWalletApi>;
