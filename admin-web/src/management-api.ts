import { AdminApiError, createIdempotencyKey } from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";

const BASE = "/api/v1/admin";

export interface ManagementAccount {
  id: string;
  super_admin: boolean;
  brand_ids: string[];
  permissions: string[];
  permissions_by_brand?: Record<string, string[]>;
}

export interface RoleRecord {
  id: string;
  brand_id: string;
  code: string;
  name: string;
  status: "active" | "disabled";
  version: number;
  is_bootstrap: boolean;
  permissions: string[];
  audit_log_id?: string;
}

export interface AdminRecord {
  id: string;
  username: string;
  status: string;
  version: number;
  super_admin: boolean;
  brand_ids: string[];
  role_ids: string[];
  role_codes: string[];
}

export interface AuthSettings {
  version: number;
  captcha_enabled: boolean;
  telegram_enabled: boolean;
  telegram_client_id: string;
  privacy_policy_version: string;
  service_terms_version: string;
  audit_log_id?: string;
}

export interface CreatedMember {
  user_id: string;
  member_id: string;
  brand_id: string;
  terms_accepted: false;
  audit_log_id: string;
}

type FetchLike = typeof fetch;

export function effectivePermissions(
  account: ManagementAccount,
  brandId: string,
): Set<string> {
  return brandPermissionSet(account, brandId);
}

export function canView(
  account: ManagementAccount,
  brandId: string,
  permission: string,
): boolean {
  return effectivePermissions(account, brandId).has(`${permission}.view.brand`);
}

export function canWrite(
  account: ManagementAccount,
  brandId: string,
  permission: string,
): boolean {
  return effectivePermissions(account, brandId).has(`${permission}.write.brand`);
}

/** Kept as the management section's read gate; writes are checked separately. */
export const canManage = canView;

export function rolePermissionChoices(
  account: ManagementAccount,
  brandId: string,
  registeredPermissions: string[] | null,
): string[] {
  if (!registeredPermissions) return [];
  const grants = effectivePermissions(account, brandId);
  return [
    ...new Set(
      registeredPermissions.filter(
        (permission) => grants.has(permission) && permission.endsWith(".brand"),
      ),
    ),
  ].sort();
}

export function assignableRoles(
  account: ManagementAccount,
  brandId: string,
  roles: RoleRecord[],
): RoleRecord[] {
  const grants = effectivePermissions(account, brandId);
  return roles.filter(
    (role) =>
      role.status === "active" &&
      role.brand_id === brandId &&
      role.permissions.every((permission) => grants.has(permission)),
  );
}

export function canEditAccountScope(
  account: ManagementAccount,
  brandId: string,
  target: AdminRecord,
): boolean {
  if (account.super_admin || !account.brand_ids.includes(brandId)) return false;
  if (target.super_admin || target.id === account.id) return false;
  return target.brand_ids.length === 1 && target.brand_ids[0] === brandId;
}

export function rolePermissionUnion(
  roles: Pick<RoleRecord, "permissions">[],
): string[] {
  return [...new Set(roles.flatMap((role) => role.permissions))].sort();
}

/** Preserve a retry key for an identical serialized request; any edited body gets a fresh key. */
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

export function createManagementApi(fetcher: FetchLike = fetch) {
  async function request<T>(
    path: string,
    options: {
      method?: "GET" | "POST" | "PATCH";
      brandId: string;
      body?: unknown;
      idempotencyKey?: string;
    },
  ): Promise<T> {
    const headers = new Headers({
      Accept: "application/json",
      "X-Brand-ID": options.brandId,
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
    let envelope: unknown;
    function invalid(): never { throw new AdminApiError("Invalid server response; operation result is unconfirmed", 502, "INVALID_RESPONSE"); }
    const object = (value: unknown): value is Record<string, unknown> => Boolean(value && typeof value === "object" && !Array.isArray(value));
    try {
      envelope = await response.json();
    } catch {
      invalid();
    }
    if (!object(envelope)) invalid();
    const detail = envelope.error;
    if (!response.ok && envelope.success === false && object(detail) &&
      typeof detail.code === "string" && detail.code.trim() && typeof detail.message === "string" && detail.message.trim()) {
      throw new AdminApiError(detail.message, response.status, detail.code);
    }
    if (!response.ok || envelope.success !== true || !object(envelope.data)) invalid();
    return envelope.data as T;
  }

  return {
    permissions(brandId: string) {
      return request<{ items: string[] }>("/permissions", { brandId });
    },
    roles(brandId: string, limit = 50, offset = 0) {
      return request<{ items: RoleRecord[] }>(
        `/roles?limit=${limit}&offset=${offset}`,
        { brandId },
      );
    },
    createRole(
      brandId: string,
      body: {
        code: string;
        name: string;
        permissions: string[];
        reason: string;
      },
      key: string,
    ) {
      return request<RoleRecord>("/roles", {
        method: "POST",
        brandId,
        body,
        idempotencyKey: key,
      });
    },
    updateRole(
      brandId: string,
      id: string,
      body: {
        version: number;
        name: string;
        status: "active" | "disabled";
        permissions: string[];
        reason: string;
      },
      key: string,
    ) {
      return request<RoleRecord>(`/roles/${encodeURIComponent(id)}`, {
        method: "PATCH",
        brandId,
        body,
        idempotencyKey: key,
      });
    },
    accounts(brandId: string, limit = 50, offset = 0) {
      return request<{ items: AdminRecord[] }>(
        `/accounts?limit=${limit}&offset=${offset}`,
        { brandId },
      );
    },
    createAccount(
      brandId: string,
      body: {
        username: string;
        password: string;
        role_ids: string[];
        reason: string;
      },
      key: string,
    ) {
      return request<AdminRecord>("/accounts", {
        method: "POST",
        brandId,
        body,
        idempotencyKey: key,
      });
    },
    updateAccount(
      brandId: string,
      id: string,
      body: {
        version: number;
        role_ids: string[];
        status: string;
        reason: string;
      },
      key: string,
    ) {
      return request<AdminRecord>(`/accounts/${encodeURIComponent(id)}`, {
        method: "PATCH",
        brandId,
        body,
        idempotencyKey: key,
      });
    },
    resetAccountPassword(
      brandId: string,
      id: string,
      body: { version: number; password: string; reason: string },
      key: string,
    ) {
      return request<{ audit_log_id?: string }>(
        `/accounts/${encodeURIComponent(id)}/reset-password`,
        { method: "POST", brandId, body, idempotencyKey: key },
      );
    },
    createMember(
      brandId: string,
      body: {
        username?: string;
        phone?: string;
        password: string;
        display_name?: string;
        notes?: string;
        reason: string;
      },
      key: string,
    ) {
      return request<CreatedMember>("/users", {
        method: "POST",
        brandId,
        body,
        idempotencyKey: key,
      });
    },
    authSettings(brandId: string) {
      return request<AuthSettings>("/auth-settings", { brandId });
    },
    updateAuthSettings(
      brandId: string,
      body: {
        version: number;
        captcha_enabled: boolean;
        telegram_enabled: boolean;
        telegram_client_id: string;
        reason: string;
      },
      key: string,
    ) {
      return request<AuthSettings>("/auth-settings", {
        method: "PATCH",
        brandId,
        body,
        idempotencyKey: key,
      });
    },
  };
}

export type ManagementApi = ReturnType<typeof createManagementApi>;
