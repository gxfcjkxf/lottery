import { AdminApiError, createIdempotencyKey } from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";

const BASE = "/api/v1/admin";

export interface PointPolicy {
  brand_id: string;
  version: number;
  max_balance_points: string | null;
  max_recharge_points: string | null;
  max_adjustment_points: string | null;
  audit_log_id?: string;
}

export interface PointPolicyAccount {
  super_admin: boolean;
  brand_ids: string[];
  permissions_by_brand?: Record<string, string[]>;
}

export interface UpdatePointPolicyBody {
  version: number;
  max_balance_points: string | null;
  max_recharge_points: string | null;
  max_adjustment_points: string | null;
  reason: string;
}

type Envelope<T> = {
  success?: boolean;
  data?: T;
  error?: { code: string; message: string } | null;
};

export function effectiveBrandPermissions(
  account: PointPolicyAccount,
  brandId: string,
): Set<string> {
  return brandPermissionSet(account, brandId);
}

export function canViewPointPolicy(
  account: PointPolicyAccount,
  brandId: string,
): boolean {
  return (
    effectiveBrandPermissions(account, brandId).has("point_policy.view.brand")
  );
}

export function canWritePointPolicy(
  account: PointPolicyAccount,
  brandId: string,
): boolean {
  return (
    effectiveBrandPermissions(account, brandId).has("point_policy.write.brand")
  );
}

export function isCanonicalPositiveLimit(value: string): boolean {
  return /^[1-9]\d*$/.test(value);
}

export function createPointPolicyKeyTracker() {
  let previousInput: string | undefined;
  let previousKey = "";
  return (input: unknown) => {
    const serialized = JSON.stringify(input);
    if (serialized !== previousInput) {
      previousInput = serialized;
      previousKey = createIdempotencyKey();
    }
    return previousKey;
  };
}

export function createPointPolicyApi(fetcher: typeof fetch = fetch) {
  async function request<T>(
    brandId: string,
    options: {
      method?: "GET" | "PUT";
      body?: UpdatePointPolicyBody;
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
    const response = await fetcher(`${BASE}/point-policy`, {
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
      throw new AdminApiError("Invalid server response; operation result is unconfirmed", 502, "INVALID_RESPONSE");
    }
    if (!envelope || typeof envelope !== "object" || Array.isArray(envelope)) {
      throw new AdminApiError("Invalid server response; operation result is unconfirmed", 502, "INVALID_RESPONSE");
    }
    const detail = envelope.error;
    if (!response.ok && envelope.success === false && detail && typeof detail === "object" && !Array.isArray(detail)
      && typeof detail.code === "string" && detail.code.trim()
      && typeof detail.message === "string" && detail.message.trim()) {
      throw new AdminApiError(detail.message, response.status, detail.code);
    }
    if (
      !response.ok ||
      envelope.success !== true ||
      envelope.data == null || typeof envelope.data !== "object" || Array.isArray(envelope.data)
    ) {
      throw new AdminApiError("Invalid server response; operation result is unconfirmed", 502, "INVALID_RESPONSE");
    }
    return envelope.data;
  }

  return {
    getPointPolicy(brandId: string) {
      return request<PointPolicy>(brandId);
    },
    updatePointPolicy(
      brandId: string,
      body: UpdatePointPolicyBody,
      idempotencyKey = createIdempotencyKey(),
    ) {
      return request<PointPolicy>(brandId, {
        method: "PUT",
        body,
        idempotencyKey,
      });
    },
  };
}

export type PointPolicyApi = ReturnType<typeof createPointPolicyApi>;
