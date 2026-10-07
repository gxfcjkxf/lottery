import { ApiError, createApiClient } from "@lottery/shared";
import {
  isWithdrawalAvailability,
  isWithdrawalHistory,
  isWithdrawalOrder,
  isWithdrawalPage,
  validateWithdrawalBody,
  type CreateWithdrawalBody,
  type WithdrawalAvailability,
  type WithdrawalHistory,
  type WithdrawalOrder,
  type WithdrawalPage,
} from "@lottery/shared";

export { ApiError as WithdrawalApiError };
export function withdrawalBasePath(brandCode?: string): string {
  return brandCode ? `/api/v1/b/${encodeURIComponent(brandCode)}` : "/api/v1";
}

export class WithdrawalUnknownIntentError extends Error {
  constructor(message = "The server response did not confirm the withdrawal outcome.") {
    super(message);
    this.name = "WithdrawalUnknownIntentError";
  }
}

export function createWithdrawalOrdersApi(options: {
  brandCode?: string;
  fetcher?: typeof fetch;
} = {}) {
  const basePath = withdrawalBasePath(options.brandCode ?? import.meta.env.VITE_BRAND_CODE ?? undefined);
  const api = createApiClient({ fetcher: options.fetcher, baseUrl: "" });
  const request = <T>(path: string, signal?: AbortSignal) => api.request<unknown>(`${basePath}${path}`, { signal }).then((value) => {
    if (!isEnvelopeData(value)) throw new Error("Invalid server response");
    return value;
  });
  return {
    availability(signal?: AbortSignal): Promise<WithdrawalAvailability> {
      return request("/withdrawal-availability", signal).then((data) => {
        if (!isWithdrawalAvailability(data)) throw new Error("Invalid server response");
        return data;
      });
    },
    list(signal?: AbortSignal): Promise<WithdrawalPage> {
      return request("/withdrawals", signal).then((data) => {
        if (!isWithdrawalPage(data)) throw new Error("Invalid server response");
        return data;
      });
    },
    get(id: string, signal?: AbortSignal): Promise<WithdrawalOrder> {
      return request(`/withdrawals/${encodeURIComponent(id)}`, signal).then((data) => {
        if (!isWithdrawalOrder(data)) throw new Error("Invalid server response");
        return data;
      });
    },
    history(id: string, signal?: AbortSignal): Promise<WithdrawalHistory> {
      return request(`/withdrawals/${encodeURIComponent(id)}/history`, signal).then((data) => {
        if (!isWithdrawalHistory(data)) throw new Error("Invalid server response");
        return data;
      });
    },
    async create(body: CreateWithdrawalBody, idempotencyKey: string, actorContext: string): Promise<WithdrawalOrder> {
      if (!validateWithdrawalBody(body)) throw new TypeError("Withdrawal allocation must equal a positive int64 points amount.");
      if (!idempotencyKey.trim()) throw new TypeError("Idempotency-Key is required.");
      if (!/^[0-9a-f]{64}$/i.test(actorContext)) throw new TypeError("A valid withdrawal actor context is required.");
      let response: WithdrawalOrder;
      try {
        const fetcher = options.fetcher ?? fetch;
        const fetched = await fetcher(`${basePath}/withdrawals`, {
          method: "POST",
          credentials: "same-origin",
          headers: { Accept: "application/json", "Content-Type": "application/json", "Idempotency-Key": idempotencyKey, "X-Withdrawal-Actor-Context": actorContext },
          body: JSON.stringify(body),
        });
        let raw: unknown;
        try { raw = await fetched.json(); } catch {
          throw new WithdrawalUnknownIntentError("The server returned an unreadable withdrawal receipt.");
        }
        if (!fetched.ok) {
          const envelope = isEnvelope(raw) ? raw : undefined;
          if (!envelope || envelope.success !== false || !envelope.error || typeof envelope.error.code !== "string" || typeof envelope.error.message !== "string") throw new WithdrawalUnknownIntentError("The server did not provide a confirmed rejection.");
          throw new ApiError(envelope?.error?.message ?? envelope?.message ?? `Request failed (${fetched.status})`, fetched.status, envelope?.error?.code ?? envelope?.code);
        }
        if (fetched.status !== 201) throw new WithdrawalUnknownIntentError("The server returned an unexpected creation status.");
        const data = isEnvelope(raw) && raw.success === true && Object.hasOwn(raw, "data") ? raw.data : undefined;
        if (!isWithdrawalOrder(data) || data.points !== body.points || data.source_allocation.length !== body.source_allocation.length || data.source_allocation.some((allocation, index) => allocation.source !== body.source_allocation[index]?.source || allocation.state !== body.source_allocation[index]?.state || allocation.points !== body.source_allocation[index]?.points) || !((data.state === "reviewing" && data.version === 1) || (data.state === "processing" && data.version === 2))) throw new WithdrawalUnknownIntentError("The server returned a receipt that does not match the submitted withdrawal intent.");
        response = data;
      } catch (cause) {
        if (cause instanceof WithdrawalUnknownIntentError) throw cause;
        if (cause instanceof ApiError && typeof cause.status === "number" && cause.status > 0 && cause.status < 500 && cause.status !== 408 && cause.status !== 429) throw cause;
        throw new WithdrawalUnknownIntentError(cause instanceof Error ? cause.message : undefined);
      }
      return response;
    },
  };
}

function isEnvelopeData(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function isEnvelope(value: unknown): value is { success?: boolean; data?: unknown; message?: string; code?: string; error?: { message?: string; code?: string } } {
  return Boolean(value && typeof value === "object" && !Array.isArray(value) && "success" in value);
}
