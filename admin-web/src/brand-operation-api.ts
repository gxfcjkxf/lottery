import { AdminApiError, type AdminAccount } from "./admin-api";

const BASE = "/api/v1/admin/brand-operation";
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const STATUSES = ["active", "paused", "disabled"] as const;

export type BrandOperationStatus = "active" | "paused" | "disabled";
export type BrandOperationTargetStatus = Exclude<BrandOperationStatus, "disabled">;

export interface BrandOperation {
  brand_id: string;
  version: number;
  name: string;
  status: BrandOperationStatus;
  updated_at: string;
  audit_log_id?: string;
}

export interface BrandOperationReceipt extends BrandOperation {
  audit_log_id: string;
}

export interface BrandOperationHistoryItem {
  id: string;
  brand_id: string;
  version: number;
  previous_status: BrandOperationStatus;
  status: BrandOperationStatus;
  changed_by: string;
  reason: string;
  audit_log_id: string;
  created_at: string;
}

export interface BrandOperationHistoryPage {
  items: BrandOperationHistoryItem[];
  limit: number;
  offset: number;
}

export interface BrandOperationPatchBody {
  version: number;
  status: BrandOperationTargetStatus;
  reason: string;
}

export interface BrandOperationApi {
  get(brandId: string): Promise<BrandOperation>;
  history(brandId: string, limit?: number, offset?: number): Promise<BrandOperationHistoryPage>;
  patch(brandId: string, body: BrandOperationPatchBody, idempotencyKey: string): Promise<BrandOperationReceipt>;
}

export function brandOperationPermissions(account: AdminAccount, brandId: string): { view: boolean; write: boolean } {
  const brandPermissions = new Set(account.permissions_by_brand === undefined
    ? account.permissions ?? []
    : account.permissions_by_brand[brandId] ?? []);
  const platformPermissions = new Set(account.platform_permissions ?? []);
  const inBrand = UUID_RE.test(account.id) && UUID_RE.test(brandId) && (account.brand_ids ?? []).includes(brandId);
  return {
    view: (inBrand && brandPermissions.has("brand_operation.view.brand")) || platformPermissions.has("brand_operation.view.platform"),
    write: (inBrand && brandPermissions.has("brand_operation.write.brand")) || platformPermissions.has("brand_operation.write.platform"),
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function exactKeys(value: Record<string, unknown>, keys: string[]): boolean {
  const actual = Object.keys(value).sort();
  const expected = [...keys].sort();
  return actual.length === expected.length && actual.every((key, index) => key === expected[index]);
}

function validUuid(value: unknown): value is string {
  return typeof value === "string" && UUID_RE.test(value);
}

function validDateTime(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|([+-])(\d{2}):(\d{2}))$/.exec(value);
  if (!m) return false;
  const [, y, mo, d, h, mi, s, , , , oh, om] = m;
  const year = Number(y), month = Number(mo), day = Number(d);
  const days = month === 2 ? (year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28) : [4, 6, 9, 11].includes(month) ? 30 : 31;
  if (year < 1 || month < 1 || month > 12 || day < 1 || day > days || Number(h) > 23 || Number(mi) > 59 || Number(s) > 59) return false;
  if (oh !== undefined && (Number(oh) > 14 || Number(om) > 59 || (Number(oh) === 14 && Number(om) !== 0))) return false;
  return Number.isFinite(Date.parse(value));
}

function validStatus(value: unknown): value is BrandOperationStatus {
  return typeof value === "string" && STATUSES.includes(value as BrandOperationStatus);
}

function validVersion(value: unknown): value is number {
  return Number.isSafeInteger(value) && Number(value) >= 1;
}

function validBrandOperation(value: unknown, brandId: string): value is BrandOperation {
  return isRecord(value) && (exactKeys(value, ["brand_id", "version", "name", "status", "updated_at"]) ||
      exactKeys(value, ["brand_id", "version", "name", "status", "updated_at", "audit_log_id"])) &&
    value.brand_id === brandId && validVersion(value.version) && typeof value.name === "string" &&
    validStatus(value.status) && validDateTime(value.updated_at) &&
    (value.audit_log_id === undefined || validUuid(value.audit_log_id));
}

function validHistoryItem(value: unknown, brandId: string): value is BrandOperationHistoryItem {
  return isRecord(value) && exactKeys(value, ["id", "brand_id", "version", "previous_status", "status", "changed_by", "reason", "audit_log_id", "created_at"]) &&
    validUuid(value.id) && value.brand_id === brandId && validVersion(value.version) &&
    (value.previous_status === "active" || value.previous_status === "paused") &&
    (value.status === "active" || value.status === "paused") && validUuid(value.changed_by) &&
    typeof value.reason === "string" && new TextEncoder().encode(value.reason).length <= 500 &&
    validUuid(value.audit_log_id) && validDateTime(value.created_at);
}

function invalidInput(message: string): never {
  throw new AdminApiError(message, 0, "INVALID_INPUT");
}

function invalidResponse(write: boolean): never {
  throw new AdminApiError("品牌运行状态响应格式无效。", write ? 0 : 502, "INVALID_RESPONSE");
}

type Envelope = { success?: unknown; data?: unknown; error?: unknown };

export function createBrandOperationApi(fetchImpl: typeof fetch = fetch): BrandOperationApi {
  async function request(path: string, brandId: string, options: { method?: "GET" | "PATCH"; body?: unknown; key?: string; write?: boolean } = {}): Promise<unknown> {
    if (!validUuid(brandId)) invalidInput("品牌编号必须是 UUID。");
    const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brandId });
    if (options.body !== undefined) headers.set("Content-Type", "application/json");
    if (options.key !== undefined) headers.set("Idempotency-Key", options.key);
    let response: Response;
    try {
      response = await fetchImpl(path, { method: options.method ?? "GET", credentials: "same-origin", headers,
        ...(options.body === undefined ? {} : { body: JSON.stringify(options.body) }) });
    } catch (cause) {
      throw new AdminApiError(cause instanceof Error ? cause.message : "Network request failed", 0, "NETWORK_ERROR");
    }
    let envelope: Envelope;
    try {
      envelope = await response.json() as Envelope;
    } catch {
      if (!response.ok) throw new AdminApiError(`Request failed (${response.status})`, response.status);
      invalidResponse(Boolean(options.write));
    }
    if (!response.ok || !isRecord(envelope) || envelope.success !== true || envelope.data === undefined) {
      const error = typeof envelope?.error === "object" && envelope.error ? envelope.error as Record<string, unknown> : undefined;
      const message = typeof envelope?.error === "string" ? envelope.error : typeof error?.message === "string" ? error.message : `Request failed (${response.status})`;
      if (!response.ok) throw new AdminApiError(message, response.status, typeof error?.code === "string" ? error.code : undefined);
      invalidResponse(Boolean(options.write));
    }
    return envelope.data;
  }

  return {
    async get(brandId) {
      const data = await request(BASE, brandId);
      if (!validBrandOperation(data, brandId)) invalidResponse(false);
      return data;
    },
    async history(brandId, limit = 20, offset = 0) {
      if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000)
        invalidInput("分页参数超出允许范围。");
      const data = await request(`${BASE}/history?limit=${limit}&offset=${offset}`, brandId);
      if (!isRecord(data) || !exactKeys(data, ["items", "limit", "offset"]) || data.limit !== limit || data.offset !== offset ||
        !Array.isArray(data.items) || data.items.length > limit || !data.items.every((item) => validHistoryItem(item, brandId))) invalidResponse(false);
      return data as unknown as BrandOperationHistoryPage;
    },
    async patch(brandId, body, idempotencyKey) {
      if (!validUuid(brandId) || !isRecord(body) || !validVersion(body.version) ||
        (body.status !== "active" && body.status !== "paused") || typeof body.reason !== "string" ||
        body.reason.trim().length === 0 || new TextEncoder().encode(body.reason).length > 500 ||
        typeof idempotencyKey !== "string" || !/^[\x21-\x7e]{8,128}$/.test(idempotencyKey)) invalidInput("品牌运行状态操作参数无效。");
      const data = await request(BASE, brandId, { method: "PATCH", body: { version: body.version, status: body.status, reason: body.reason }, key: idempotencyKey, write: true });
      if (!isRecord(data) || !exactKeys(data, ["brand_id", "version", "name", "status", "updated_at", "audit_log_id"]) ||
        !validBrandOperation({ brand_id: data.brand_id, version: data.version, name: data.name, status: data.status, updated_at: data.updated_at }, brandId) ||
        data.status !== body.status || data.version !== body.version + 1 || !validUuid(data.audit_log_id)) invalidResponse(true);
      return data as unknown as BrandOperationReceipt;
    },
  };
}
