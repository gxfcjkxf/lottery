export type MemberStatus =
  | "normal"
  | "frozen"
  | "disabled"
  | "expired"
  | "cancelled";

export interface AdminAccount {
  id: string;
  super_admin: boolean;
  brand_ids: string[];
  permissions: string[];
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
}

export interface AdminAuditRecord {
  id: string;
  action: string;
  actor_type: string;
  actor_id: string;
  resource_type: string;
  resource_id: string;
  reason: string;
  request_id: string;
  created_at: string;
  ip_address: string;
  before_json: unknown;
  after_json: unknown;
}

interface Envelope<T> {
  success: boolean;
  data?: T;
  error?:
    | { code?: string; message?: string; [key: string]: unknown }
    | string
    | null;
}

export class AdminApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code?: string,
  ) {
    super(message);
    this.name = "AdminApiError";
  }
}

type FetchLike = typeof fetch;

function errorMessage(
  error: Envelope<unknown>["error"],
  fallback: string,
): string {
  if (typeof error === "string") return error;
  return error?.message || fallback;
}

export function createIdempotencyKey(): string {
  if (globalThis.crypto?.randomUUID) return globalThis.crypto.randomUUID();
  const bytes = new Uint8Array(16);
  globalThis.crypto.getRandomValues(bytes);
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = [...bytes]
    .map((value) => value.toString(16).padStart(2, "0"))
    .join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
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
      const error =
        typeof envelope.error === "object" && envelope.error !== null
          ? envelope.error
          : undefined;
      throw new AdminApiError(
        errorMessage(envelope.error, `Request failed (${response.status})`),
        response.status,
        error?.code,
      );
    }
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
    me() {
      return request<{ account: AdminAccount }>("/api/v1/admin/me");
    },
    brands() {
      return request<{ items: AdminBrand[] }>("/api/v1/admin/brands");
    },
    users(brandId: string, limit = 100, offset = 0) {
      return request<{ items: Member[] }>(
        `/api/v1/admin/users?limit=${limit}&offset=${offset}`,
        { brandId },
      );
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
