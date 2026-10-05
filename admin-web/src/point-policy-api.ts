import { AdminApiError, createIdempotencyKey } from "./admin-api";

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
  permissions: string[];
  permissions_by_brand?: Record<string, string[]>;
  platform_permissions?: string[];
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
  error?: string | { code?: string; message?: string } | null;
};

export function effectiveBrandPermissions(
  account: PointPolicyAccount,
  brandId: string,
): Set<string> {
  return new Set(
    account.permissions_by_brand === undefined
      ? (account.permissions ?? [])
      : (account.permissions_by_brand[brandId] ?? []),
  );
}

function hasPlatformPermission(
  account: PointPolicyAccount,
  permission: string,
): boolean {
  return (account.platform_permissions ?? account.permissions ?? []).includes(
    permission,
  );
}

export function canViewPointPolicy(
  account: PointPolicyAccount,
  brandId: string,
): boolean {
  return (
    effectiveBrandPermissions(account, brandId).has(
      "point_policy.view.brand",
    ) || hasPlatformPermission(account, "point_policy.view.platform")
  );
}

export function canWritePointPolicy(
  account: PointPolicyAccount,
  brandId: string,
): boolean {
  return (
    !account.super_admin &&
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
      throw new AdminApiError(
        response.ok
          ? "Invalid server response"
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
      throw new AdminApiError(
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
