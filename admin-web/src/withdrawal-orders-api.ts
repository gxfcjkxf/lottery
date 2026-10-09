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

function isRecord(value: unknown): value is Record<string, unknown> { return Boolean(value && typeof value === "object" && !Array.isArray(value)); }
function unwrap<T>(raw: unknown): T {
  if (!isRecord(raw) || raw.success !== true || !Object.hasOwn(raw, "data")) throw new Error("Invalid server response");
  return raw.data as T;
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
    let raw: unknown;
    try { raw = await response.json(); } catch {
      if (options.method === "POST") throw new WithdrawalAdminUnknownIntentError("The server returned an invalid action receipt.");
      throw new AdminApiError(response.ok ? "Invalid server response" : `Request failed (${response.status})`, response.status);
    }
    if (response.ok && options.method === "POST" && (response.status !== 200 || !isRecord(raw) || raw.success !== true || !Object.hasOwn(raw, "data"))) {
      throw new WithdrawalAdminUnknownIntentError("The server returned an unrecognized withdrawal action response.");
    }
    if (!response.ok || !isRecord(raw) || raw.success !== true || !Object.hasOwn(raw, "data")) {
      const error = isRecord(raw) && isRecord(raw.error) ? raw.error : undefined;
      const code = typeof error?.code === "string" ? error.code : undefined;
      const message = typeof error?.message === "string" ? error.message : `Request failed (${response.status})`;
      if (options.method === "POST" && (!isRecord(raw) || raw.success !== false || !error || typeof error.code !== "string" || typeof error.message !== "string")) throw new WithdrawalAdminUnknownIntentError(message);
      if (options.method === "POST" && (response.status === 0 || response.status >= 500 || response.status === 408 || response.status === 429)) throw new WithdrawalAdminUnknownIntentError(message);
      throw new AdminApiError(message, response.status, code);
    }
    try { return unwrap<T>(raw); } catch (cause) {
      if (options.method === "POST") throw new WithdrawalAdminUnknownIntentError(cause instanceof Error ? cause.message : undefined);
      throw new AdminApiError("Invalid server response", response.status);
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
      if (!Number.isSafeInteger(body.version) || body.version < 1 || !body.reason.trim() || !key.trim()) throw new TypeError("A current version, reason, and idempotency key are required.");
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
