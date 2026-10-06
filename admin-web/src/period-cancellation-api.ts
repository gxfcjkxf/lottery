import { AdminApiError, type AdminAccount } from "./admin-api";
import { createBetMutationKeyTracker } from "./bet-management-api";
import type { Period } from "./period-schedules-api";

const BASE = "/api/v1/admin";

export interface Cancellation {
  id: string;
  brand_id: string;
  game_id: string;
  period_id: string;
  period_version: number;
  mode: "bet_cancelled" | "judged_cancelled";
  cause: "operator_cancel" | "no_result" | "invalid_result";
  state: "processing" | "failed" | "completed";
  version: number;
  reason: string;
  created_by: string;
  created_at: string;
  completed_at: string | null;
  last_error_code: string;
  total_count: number;
  pending_count: number;
  refunded_count: number;
  already_refunded_count: number;
  failed_count: number;
}

export interface PeriodCancellationPermissions {
  view: boolean;
  cancel: boolean;
  retry: boolean;
}

export function periodCancellationPermissions(
  account: AdminAccount,
  brandId: string,
): PeriodCancellationPermissions {
  const brand = new Set(
    account.permissions_by_brand === undefined
      ? (account.permissions ?? [])
      : (account.permissions_by_brand[brandId] ?? []),
  );
  const platform = new Set(
    account.platform_permissions ?? account.permissions ?? [],
  );
  return {
    view:
      Boolean(brandId) &&
      (brand.has("period.view.brand") || platform.has("period.view.platform")),
    cancel:
      Boolean(brandId) &&
      !account.super_admin &&
      brand.has("period.cancel.brand"),
    retry:
      Boolean(brandId) &&
      !account.super_admin &&
      brand.has("period.cancel_retry.brand"),
  };
}

export { createBetMutationKeyTracker };

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}
function nonempty(value: unknown): value is string {
  return typeof value === "string" && value.length > 0;
}
function validCancellation(value: unknown): value is Cancellation {
  if (!isRecord(value)) return false;
  const counts = [
    "total_count",
    "pending_count",
    "refunded_count",
    "already_refunded_count",
    "failed_count",
  ];
  if (
    !counts.every(
      (key) => Number.isSafeInteger(value[key]) && Number(value[key]) >= 0,
    )
  )
    return false;
  if (
    BigInt(Number(value.total_count)) !==
    [
      value.pending_count,
      value.refunded_count,
      value.already_refunded_count,
      value.failed_count,
    ].reduce<bigint>((sum, x) => sum + BigInt(Number(x)), 0n)
  )
    return false;
  if (
    (value.state === "completed") !==
      (typeof value.completed_at === "string") ||
    (value.state === "failed") !== (value.last_error_code !== "")
  )
    return false;
  if (
    value.state === "completed" &&
    (value.pending_count !== 0 || value.failed_count !== 0)
  )
    return false;
  return (
    [
      "id",
      "brand_id",
      "game_id",
      "period_id",
      "reason",
      "created_by",
      "created_at",
    ].every((key) => nonempty(value[key])) &&
    Number.isSafeInteger(value.period_version) &&
    Number(value.period_version) >= 1 &&
    Number.isSafeInteger(value.version) &&
    Number(value.version) >= 1 &&
    ["bet_cancelled", "judged_cancelled"].includes(String(value.mode)) &&
    ["operator_cancel", "no_result", "invalid_result"].includes(
      String(value.cause),
    ) &&
    ["processing", "failed", "completed"].includes(String(value.state)) &&
    (value.completed_at === null || nonempty(value.completed_at)) &&
    typeof value.last_error_code === "string" &&
    [
      "total_count",
      "pending_count",
      "refunded_count",
      "already_refunded_count",
      "failed_count",
    ].every(
      (key) => Number.isSafeInteger(value[key]) && Number(value[key]) >= 0,
    )
  );
}
function invalidResponse(): never {
  throw new AdminApiError("期次撤销响应格式无效。", 502, "INVALID_RESPONSE");
}
type Envelope = {
  success?: boolean;
  data?: unknown;
  error?: string | { code?: string; message?: string } | null;
};

export function createPeriodCancellationApi(fetcher: typeof fetch = fetch) {
  async function request(
    brandId: string,
    path: string,
    body?: unknown,
    key?: string,
  ) {
    if (!brandId.trim())
      throw new AdminApiError("请先选择品牌。", 0, "INVALID_INPUT");
    const headers = new Headers({
      Accept: "application/json",
      "X-Brand-ID": brandId,
    });
    if (body !== undefined) headers.set("Content-Type", "application/json");
    if (key !== undefined) headers.set("Idempotency-Key", key);
    let response: Response;
    try {
      response = await fetcher(path, {
        method: body === undefined ? "GET" : "POST",
        credentials: "same-origin",
        headers,
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      });
    } catch (cause) {
      throw new AdminApiError(
        cause instanceof Error ? cause.message : "Network request failed",
        0,
        "NETWORK_ERROR",
      );
    }
    let envelope: Envelope;
    try {
      envelope = (await response.json()) as Envelope;
    } catch {
      throw new AdminApiError(
        response.ok
          ? "Invalid server response"
          : `Request failed (${response.status})`,
        response.ok ? 502 : response.status,
        response.ok ? "INVALID_RESPONSE" : undefined,
      );
    }
    if (
      !response.ok ||
      envelope.success !== true ||
      envelope.data === undefined
    ) {
      const error =
        typeof envelope.error === "object" && envelope.error
          ? envelope.error
          : undefined;
      throw new AdminApiError(
        typeof envelope.error === "string"
          ? envelope.error
          : error?.message || `Request failed (${response.status})`,
        response.ok ? 502 : response.status,
        error?.code,
      );
    }
    return envelope.data;
  }
  function checked(value: unknown, brandId: string): Cancellation {
    if (!validCancellation(value) || value.brand_id !== brandId)
      invalidResponse();
    return value;
  }
  return {
    async getPeriod(brandId: string, periodId: string): Promise<Period> {
      const value = await request(
        brandId,
        `${BASE}/periods/${encodeURIComponent(periodId)}`,
      );
      if (
        !isRecord(value) ||
        value.id !== periodId ||
        value.brand_id !== brandId ||
        !nonempty(value.game_id) ||
        !nonempty(value.period_no) ||
        !nonempty(value.status) ||
        !Number.isSafeInteger(value.version) ||
        Number(value.version) < 1 ||
        !Number.isSafeInteger(value.sequence) ||
        Number(value.sequence) < 1 ||
        ![value.bet_start_at, value.bet_end_at, value.draw_at].every(nonempty)
      )
        invalidResponse();
      return value as unknown as Period;
    },
    async getCancellation(
      brandId: string,
      periodId: string,
    ): Promise<{ cancellation: Cancellation | null }> {
      const value = await request(
        brandId,
        `${BASE}/periods/${encodeURIComponent(periodId)}/cancellation`,
      );
      if (
        !isRecord(value) ||
        !(value.cancellation === null || validCancellation(value.cancellation))
      )
        invalidResponse();
      if (
        value.cancellation !== null &&
        value.cancellation.brand_id !== brandId
      )
        invalidResponse();
      return value as { cancellation: Cancellation | null };
    },
    async cancelPeriod(
      brandId: string,
      periodId: string,
      body: {
        version: number;
        mode: Cancellation["mode"];
        cause: Cancellation["cause"];
        reason: string;
      },
      key: string,
    ): Promise<Cancellation> {
      return checked(
        await request(
          brandId,
          `${BASE}/periods/${encodeURIComponent(periodId)}/cancel`,
          body,
          key,
        ),
        brandId,
      );
    },
    async retryCancellation(
      brandId: string,
      periodId: string,
      body: { version: number; reason: string },
      key: string,
    ): Promise<Cancellation> {
      return checked(
        await request(
          brandId,
          `${BASE}/periods/${encodeURIComponent(periodId)}/cancellation/retry`,
          body,
          key,
        ),
        brandId,
      );
    },
  };
}

export type PeriodCancellationApi = ReturnType<
  typeof createPeriodCancellationApi
>;
