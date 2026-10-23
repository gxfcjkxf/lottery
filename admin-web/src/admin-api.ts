import { readApiEnvelope, readApiRejection } from '@lottery/shared'

import { notifySessionReplaced } from '@lottery/shared'
export type MemberStatus =
  | "normal"
  | "frozen"
  | "disabled"
  | "expired"
  | "cancelled";

export interface AdminAccount {
  id: string;
  username?: string;
  super_admin: boolean;
  brand_ids: string[];
  permissions: string[];
  permissions_by_brand?: Record<string, string[]>;
  platform_permissions?: string[];
  version?: number;
}

export interface AdminBrand {
  id: string;
  code: string;
  name: string;
  status: string;
}

export interface Member {
  id: string;
  global_user_id: string;
  username: string;
  phone: string;
  display_name: string;
  notes: string;
  status: MemberStatus;
  joined_at: string;
  brand_id: string;
  tags: string[];
  online: boolean;
  display_points: string | null;
}

export interface AdminAuditRecord {
  id: string;
  action: string;
  actor_type: string;
  actor_id: string;
  actor_username: string;
  resource_type: string;
  resource_id: string;
  reason: string;
  request_id: string;
  created_at: string;
  ip_address: string;
  before_json: unknown;
  after_json: unknown;
}

export class AdminApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code?: string,
  ) {
    super(message);
    this.name = "AdminApiError";
    notifySessionReplaced(code)
  }
}

type FetchLike = typeof fetch;

export function createIdempotencyKey(): string {
  const uuid = globalThis.crypto?.randomUUID?.();
  if (!uuid)
    throw new Error("crypto.randomUUID is required to create an idempotency key");
  return uuid;
}

export function readAdminEnvelope<T = unknown>(response: Response) {
  return readApiEnvelope<T>(response, (message, status, code) => new AdminApiError(message, status, code));
}
export async function readAdminErrorResponse(response: Response): Promise<AdminApiError> {
  return await readApiRejection(response, (message, status, code) => new AdminApiError(message, status, code)) as AdminApiError;
}

export function createAdminApi(fetcher: FetchLike = fetch) {
  async function request<T>(
    path: string,
    options: {
      method?: "GET" | "POST" | "PATCH";
      brandId?: string;
      body?: unknown;
      idempotencyKey?: string;
    } = {},
  ): Promise<T> {
    const headers = new Headers({ Accept: "application/json" });
    if (options.body !== undefined)
      headers.set("Content-Type", "application/json");
    if (options.brandId) headers.set("X-Brand-ID", options.brandId);
    if (options.idempotencyKey)
      headers.set("Idempotency-Key", options.idempotencyKey);
    const response = await fetcher(path, {
      method: options.method ?? "GET",
      credentials: "same-origin",
      headers,
      ...(options.body === undefined
        ? {}
        : { body: JSON.stringify(options.body) }),
    });
    const envelope = await readAdminEnvelope<T>(response);
    if (envelope.data == null || typeof envelope.data !== "object" || Array.isArray(envelope.data))
      throw new AdminApiError("Invalid server response; operation result is unconfirmed", 502, "INVALID_RESPONSE");
    return envelope.data;
  }

  return {
    login(
      identifier: string,
      password: string,
      idempotencyKey = createIdempotencyKey(),
    ) {
      // The response is intentionally discarded: the browser session is the HttpOnly cookie.
      return request<unknown>("/api/v1/admin/auth/login", {
        method: "POST",
        body: { identifier, password },
        idempotencyKey,
      });
    },
    logout(idempotencyKey = createIdempotencyKey()) {
      return request<unknown>("/api/v1/admin/auth/logout", {
        method: "POST",
        body: {},
        idempotencyKey,
      });
    },
    changePassword(currentPassword: string, newPassword: string, idempotencyKey = createIdempotencyKey()) {
      return request<unknown>('/api/v1/admin/me/password', {
        method: 'POST',
        body: { current_password: currentPassword, new_password: newPassword },
        idempotencyKey,
      });
    },
    me() {
      return request<{ account: AdminAccount }>("/api/v1/admin/me");
    },
    brands() {
      return request<{ items: AdminBrand[] }>("/api/v1/admin/brands");
    },
    async users(brandId: string, limit = 100, offset = 0, username = "", filters: { query?: string; online?: 'true' | 'false' | '' } = {}) {
      const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
      if (username.trim()) query.set("username", username.trim());
      if (filters.query?.trim()) query.set("q", filters.query.trim());
      if (filters.online) query.set("online", filters.online);
      const result = await request<{ items: Member[]; limit: number; offset: number; total_count: string }>(`/api/v1/admin/users?${query}`, { brandId });
      if (!Array.isArray(result.items) || result.limit !== limit || result.offset !== offset || typeof result.total_count !== 'string' || !/^(0|[1-9]\d*)$/.test(result.total_count) ||
          result.items.some(item => item.brand_id !== brandId || typeof item.online !== 'boolean' || !(item.display_points === null || typeof item.display_points === 'string' && /^(0|[1-9]\d*)$/.test(item.display_points)))) {
        throw new AdminApiError('Invalid member page', 502, 'INVALID_RESPONSE');
      }
      return result;
    },
    audit(brandId: string, limit = 100, offset = 0) {
      return request<{ items: AdminAuditRecord[] }>(
        `/api/v1/admin/audit?limit=${limit}&offset=${offset}`,
        { brandId },
      );
    },
    updateUser(
      brandId: string,
      memberId: string,
      body: { status: MemberStatus; notes: string; reason: string },
      idempotencyKey: string,
    ) {
      return request<unknown>(
        `/api/v1/admin/users/${encodeURIComponent(memberId)}`,
        { method: "PATCH", brandId, body, idempotencyKey },
      );
    },
    kickUser(
      brandId: string,
      memberId: string,
      reason: string,
      idempotencyKey: string,
    ) {
      return request<unknown>(
        `/api/v1/admin/users/${encodeURIComponent(memberId)}/kick`,
        { method: "POST", brandId, body: { reason }, idempotencyKey },
      );
    },
    resetPassword(
      brandId: string,
      memberId: string,
      password: string,
      reason: string,
      idempotencyKey: string,
    ) {
      return request<unknown>(
        `/api/v1/admin/users/${encodeURIComponent(memberId)}/reset-password`,
        { method: "POST", brandId, body: { password, reason }, idempotencyKey },
      );
    },
  };
}

export type AdminApi = ReturnType<typeof createAdminApi>;
