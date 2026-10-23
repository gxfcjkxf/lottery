import { readAdminEnvelope } from './admin-api'

import { isWithdrawalHistory, isWithdrawalOrder, isWithdrawalPage, type WithdrawalAction, type WithdrawalActionBody, type WithdrawalHistory, type WithdrawalOrder, type WithdrawalPage, type WithdrawalState } from "@lottery/shared";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";

export { AdminApiError as WithdrawalAdminApiError };
export const MAX_WITHDRAWAL_PAGE = 100;
const BRAND_UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function withdrawalPermissions(account: AdminAccount, brandId: string) {
  const validBrand = BRAND_UUID.test(brandId);
  const brandGrants = brandPermissionSet(account, brandId);
  return {
    view: validBrand && brandGrants.has("withdrawal.view.brand"),
    actions: {
      approve: validBrand && brandGrants.has("withdrawal.approve.brand"),
      reject: validBrand && brandGrants.has("withdrawal.reject.brand"),
      cancel: validBrand && brandGrants.has("withdrawal.cancel.brand"),
      fail: validBrand && brandGrants.has("withdrawal.fail.brand"),
      "mark-paid": validBrand && brandGrants.has("withdrawal.mark_paid.brand"),
    } satisfies Record<WithdrawalAction, boolean>,
  };
}

export class WithdrawalAdminUnknownIntentError extends Error {
  constructor(message = "The server response did not confirm the withdrawal action.") {
    super(message);
    this.name = "WithdrawalAdminUnknownIntentError";
  }
}

function encodeQuery(values: Record<string, string | number | undefined>) {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(values)) if (value !== undefined && value !== "") query.set(key, String(value));
  return query.toString();
}

export function createWithdrawalOrdersAdminApi(fetcher: typeof fetch = fetch) {
  async function request<T>(path: string, brandId: string, options: { method?: "GET" | "POST"; body?: unknown; key?: string; signal?: AbortSignal } = {}): Promise<T> {
    const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brandId });
    if (options.body !== undefined) headers.set("Content-Type", "application/json");
    if (options.key) headers.set("Idempotency-Key", options.key);
    let response: Response;
    try {
      response = await fetcher(path, { method: options.method ?? "GET", credentials: "same-origin", headers, ...(options.body === undefined ? {} : { body: JSON.stringify(options.body) }), signal: options.signal });
    } catch (cause) {
      if (options.method === "POST") throw new WithdrawalAdminUnknownIntentError(cause instanceof Error ? cause.message : undefined);
      throw new AdminApiError(cause instanceof Error ? cause.message : "Network unavailable", 0);
    }
    try {
      const raw = await readAdminEnvelope<T>(response);
      if (options.method === "POST" && response.status !== 200) throw new WithdrawalAdminUnknownIntentError("The server returned an unrecognized withdrawal action response.");
      return raw.data;
    } catch (cause) {
      if (options.method === "POST" && cause instanceof AdminApiError && (cause.status === 0 || cause.status >= 500 || cause.status === 408 || cause.status === 429)) throw new WithdrawalAdminUnknownIntentError(cause.message);
      throw cause;
    }

  }
  return {
    async list(brandId: string, filters: { state?: WithdrawalState; memberId?: string; limit: number; offset: number }, signal?: AbortSignal): Promise<WithdrawalPage> {
      const query = encodeQuery({ state: filters.state, member_id: filters.memberId?.trim(), limit: filters.limit, offset: filters.offset });
      const value = await request<unknown>(`/api/v1/admin/withdrawals?${query}`, brandId, { signal });
      if (!isWithdrawalPage(value)) throw new AdminApiError("Invalid withdrawal page", 200);
      return value;
    },
    async get(brandId: string, id: string, signal?: AbortSignal): Promise<WithdrawalOrder> {
      const value = await request<unknown>(`/api/v1/admin/withdrawals/${encodeURIComponent(id)}`, brandId, { signal });
      if (!isWithdrawalOrder(value)) throw new AdminApiError("Invalid withdrawal order", 200);
      return value;
    },
    async history(brandId: string, id: string, signal?: AbortSignal): Promise<WithdrawalHistory> {
      const value = await request<unknown>(`/api/v1/admin/withdrawals/${encodeURIComponent(id)}/history`, brandId, { signal });
      if (!isWithdrawalHistory(value)) throw new AdminApiError("Invalid withdrawal history", 200);
      return value;
    },
    async action(brandId: string, id: string, action: WithdrawalAction, body: WithdrawalActionBody, key: string): Promise<WithdrawalOrder> {
      if (!Number.isSafeInteger(body.version) || body.version < 1  || !key.trim()) throw new TypeError("A current version, reason, and idempotency key are required.");
      let value: unknown;
      try { value = await request<unknown>(`/api/v1/admin/withdrawals/${encodeURIComponent(id)}/${action}`, brandId, { method: "POST", body, key }); }
      catch (cause) { throw cause; }
      const expectedState: Record<WithdrawalAction, WithdrawalState> = {
        approve: "processing",
        reject: "rejected",
        cancel: "cancelled",
        fail: "failed",
        "mark-paid": "paid",
      };
      if (!isWithdrawalOrder(value) || value.brand_id !== brandId || value.id !== id || value.version !== body.version + 1 || value.state !== expectedState[action]) throw new WithdrawalAdminUnknownIntentError("The server returned an action receipt that does not match the requested order transition.");
      return value;
    },
  };
}
