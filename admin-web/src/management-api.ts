import { AdminApiError, createIdempotencyKey } from "./admin-api";

const BASE = "/api/v1/admin";

export interface ManagementAccount {
  id: string;
  super_admin: boolean;
  brand_ids: string[];
  permissions: string[];
  platform_permissions?: string[];
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

export const KNOWN_BRAND_PERMISSION_KEYS = [
  "admin.view.brand",
  "admin.write.brand",
  "auth_config.view.brand",
  "auth_config.write.brand",
  "role.view.brand",
  "role.write.brand",
  "user.create.brand",
].sort();

type Envelope<T> = {
  success?: boolean;
  data?: T;
  error?: string | { code?: string; message?: string } | null;
};
type FetchLike = typeof fetch;

export function effectivePermissions(
  account: ManagementAccount,
  brandId: string,
): Set<string> {
  const byBrand = account.permissions_by_brand;
  return new Set(
    byBrand ? (byBrand[brandId] ?? []) : (account.permissions ?? []),
  );
}

function hasPlatformGrant(
  account: ManagementAccount,
  permission: string,
  action: "view" | "write",
): boolean {
  const grant = `${permission}.${action}.platform`;
  const platformPermissions =
    account.platform_permissions ?? account.permissions ?? [];
  return platformPermissions.includes(grant);
}

export function hasPlatformWrite(
  account: ManagementAccount,
  permission: string,
): boolean {
  return hasPlatformGrant(account, permission, "write");
}

export function canView(
  account: ManagementAccount,
  brandId: string,
  permission: string,
): boolean {
  const grants = effectivePermissions(account, brandId);
  return Boolean(
    grants.has(`${permission}.view.brand`) ||
    hasPlatformGrant(account, permission, "view"),
  );
}

export function canWrite(
  account: ManagementAccount,
  brandId: string,
  permission: string,
): boolean {
  return Boolean(
    effectivePermissions(account, brandId).has(`${permission}.write.brand`) ||
    hasPlatformGrant(account, permission, "write"),
  );
}

/** Kept as the management section's read gate; writes are checked separately. */
export const canManage = canView;

export function rolePermissionChoices(
  account: ManagementAccount,
  brandId: string,
  registeredPermissions: string[] | null,
): string[] {
  const source = hasPlatformGrant(account, "role", "write")
    ? (registeredPermissions ?? KNOWN_BRAND_PERMISSION_KEYS)
    : [...effectivePermissions(account, brandId)];
  return [
    ...new Set(source.filter((permission) => permission.endsWith(".brand"))),
  ].sort();
}

export function assignableRoles(
  account: ManagementAccount,
  brandId: string,
  roles: RoleRecord[],
): RoleRecord[] {
  if (hasPlatformGrant(account, "admin", "write")) {
    return roles.filter(
      (role) => role.status === "active" && role.brand_id === brandId,
    );
  }
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
  target: AdminRecord,
): boolean {
  if (target.super_admin || target.id === account.id) return false;
  if (hasPlatformGrant(account, "admin", "write")) return true;
  return target.brand_ids.length <= 1;
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
          : detail?.message || `Request failed (${response.status})`,
        response.status,
        detail?.code,
      );
    }
    return envelope.data;
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
