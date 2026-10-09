import { AdminApiError, type AdminAccount } from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";

const BASE = "/api/v1/admin/join-codes";
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const CODE_RE = /^[0-9A-F]{24}$/;
const MAX_PAGE_SIZE = 100;
const MAX_OFFSET = 1_000_000;

export type JoinCodeKind = "agent" | "referral";
export type JoinCodeStatus = "active" | "disabled";
export interface JoinCode {
  id: string;
  brand_id: string;
  kind: JoinCodeKind;
  code: string;
  owner_member_id: string;
  agent_id: string | null;
  status: JoinCodeStatus;
  starts_at: string | null;
  expires_at: string | null;
  version: number;
  usable: boolean;
  created_at: string;
  updated_at: string;
  audit_log_id?: string | null;
}
export interface CreateJoinCodeBody {
  kind: JoinCodeKind;
  owner_member_id: string;
  agent_id: string | null;
  starts_at: string | null;
  expires_at: string | null;
  reason: string;
}
export interface UpdateJoinCodeBody {
  version: number;
  status: JoinCodeStatus;
  starts_at: string | null;
  expires_at: string | null;
  reason: string;
}
export interface JoinCodePage {
  brand_id: string;
  kind: JoinCodeKind | null;
  owner_member_id: string | null;
  items: JoinCode[];
  limit: number;
  offset: number;
  total_count: string;
}
export interface JoinCodeHistoryItem {
  id: string;
  brand_id: string;
  code_id: string;
  version: number;
  status: JoinCodeStatus;
  starts_at: string | null;
  expires_at: string | null;
  actor_id: string;
  reason: string;
  audit_log_id: string;
  created_at: string;
}
export interface JoinCodeHistoryPage {
  brand_id: string;
  code_id: string;
  items: JoinCodeHistoryItem[];
  limit: number;
  offset: number;
  total_count: string;
}
type Envelope = { success?: unknown; data?: unknown; error?: string | { code?: string; message?: string } | null };

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}
function exactKeys(value: Record<string, unknown>, keys: string[]): boolean {
  return Object.keys(value).length === keys.length && keys.every((key) => Object.prototype.hasOwnProperty.call(value, key));
}
function validUuid(value: unknown): value is string {
  return typeof value === "string" && UUID_RE.test(value);
}
function positiveInt(value: unknown): value is number {
  return Number.isSafeInteger(value) && Number(value) >= 1;
}
function validKind(value: unknown): value is JoinCodeKind {
  return value === "agent" || value === "referral";
}
function validStatus(value: unknown): value is JoinCodeStatus {
  return value === "active" || value === "disabled";
}
export function validJoinCodeDate(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,6}))?(Z|([+-])(\d{2}):(\d{2}))$/.exec(value);
  if (!m) return false;
  const [, ys, mos, ds, hs, mis, ss, , , , oh, om] = m;
  const y = Number(ys), mo = Number(mos), d = Number(ds);
  const days = mo === 2 ? (y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0) ? 29 : 28) : [4, 6, 9, 11].includes(mo) ? 30 : 31;
  if (y < 1 || mo < 1 || mo > 12 || d < 1 || d > days || Number(hs) > 23 || Number(mis) > 59 || Number(ss) > 59) return false;
  if (oh !== undefined && (Number(oh) > 14 || Number(om) > 59 || (Number(oh) === 14 && Number(om) !== 0))) return false;
  return Number.isFinite(Date.parse(value));
}
export function joinCodeInstantMicros(value: string): bigint {
  const fraction = /\.(\d{1,6})(?:Z|[+-]\d{2}:\d{2})$/.exec(value)?.[1] ?? "";
  const millisecondBase = BigInt(Date.parse(value)) * 1000n;
  return millisecondBase + BigInt(fraction.padEnd(6, "0").slice(3) || "0");
}
export function validJoinCodeDateRange(start: string | null, end: string | null): boolean {
  if (start === null || end === null) return true;
  return joinCodeInstantMicros(start) < joinCodeInstantMicros(end);
}
function sameNullableInstant(actual: unknown, expected: string | null): boolean {
  if (expected === null) return actual === null;
  return typeof actual === "string" && validJoinCodeDate(actual) &&
    joinCodeInstantMicros(actual) === joinCodeInstantMicros(expected);
}
function validReason(value: unknown): value is string {
  return typeof value === "string" && value.trim().length > 0 && new TextEncoder().encode(value).length <= 500;
}
function invalidInput(message: string): never {
  throw new AdminApiError(message, 0, "JOIN_CODE_INPUT_INVALID");
}
function invalidResponse(write: boolean): never {
  throw new AdminApiError("Invalid join code API response", write ? 0 : 502, "INVALID_RESPONSE");
}
function pageArgs(limit: number, offset: number): void {
  if (!Number.isSafeInteger(limit) || limit < 1 || limit > MAX_PAGE_SIZE || !Number.isSafeInteger(offset) || offset < 0 || offset > MAX_OFFSET)
    invalidInput("Pagination parameters are out of range");
}
function validCode(value: unknown, brand: string, expected: Partial<JoinCode> = {}, writeReceipt = false): value is JoinCode {
  return isRecord(value) && validUuid(value.id) && value.brand_id === brand && validKind(value.kind) &&
    CODE_RE.test(String(value.code)) && validUuid(value.owner_member_id) && (value.agent_id === null || validUuid(value.agent_id)) &&
    (value.kind === "agent" ? value.agent_id !== null : value.agent_id === null) && validStatus(value.status) &&
    (value.starts_at === null || validJoinCodeDate(value.starts_at)) && (value.expires_at === null || validJoinCodeDate(value.expires_at)) &&
    validJoinCodeDateRange(value.starts_at as string | null, value.expires_at as string | null) && positiveInt(value.version) && typeof value.usable === "boolean" &&
    validJoinCodeDate(value.created_at) && validJoinCodeDate(value.updated_at) &&
    (writeReceipt ? validUuid(value.audit_log_id) : value.audit_log_id === undefined || value.audit_log_id === null || validUuid(value.audit_log_id)) &&
    Object.entries(expected).every(([key, expectedValue]) => key === "starts_at" || key === "expires_at"
      ? sameNullableInstant(value[key], expectedValue as string | null)
      : value[key] === expectedValue);
}
function validPage(value: unknown, brand: string, kind: JoinCodeKind | null, owner: string | null, limit: number, offset: number): value is JoinCodePage {
  return isRecord(value) && value.brand_id === brand && value.kind === kind && value.owner_member_id === owner &&
    Array.isArray(value.items) && value.items.length <= limit && value.items.every((item) => validCode(item, brand) &&
      (kind === null || item.kind === kind) && (owner === null || item.owner_member_id === owner)) &&
    value.limit === limit && value.offset === offset && typeof value.total_count === "string" && /^(0|[1-9]\d*)$/.test(value.total_count);
}
function validHistoryItem(value: unknown, brand: string, codeId: string): value is JoinCodeHistoryItem {
  return isRecord(value) && validUuid(value.id) && value.brand_id === brand && value.code_id === codeId && positiveInt(value.version) &&
    validStatus(value.status) && (value.starts_at === null || validJoinCodeDate(value.starts_at)) && (value.expires_at === null || validJoinCodeDate(value.expires_at)) &&
    validJoinCodeDateRange(value.starts_at as string | null, value.expires_at as string | null) && validUuid(value.actor_id) && validReason(value.reason) &&
    validUuid(value.audit_log_id) && validJoinCodeDate(value.created_at);
}

export function joinCodePermissions(account: AdminAccount, brand: string): { view: boolean; write: boolean } {
  const validScope = validUuid(account.id) && validUuid(brand);
  const brandScoped = validScope && account.brand_ids.includes(brand);
  const permissions = brandPermissionSet(account, brand);
  return {
    view: brandScoped && permissions.has("join_code.view.brand"),
    write: brandScoped && !account.super_admin && permissions.has("join_code.write.brand"),
  };
}

export function standardJoinCodeApiError(error: unknown): AdminApiError {
  if (error instanceof AdminApiError) return error;
  return new AdminApiError(error instanceof Error && error.message ? error.message : "Join code request failed", 0, "NETWORK_ERROR");
}

export function createJoinCodesApi(fetchImpl: typeof fetch = fetch) {
  async function request(path: string, brand: string, options: { method?: "GET" | "POST" | "PUT"; body?: unknown; key?: string } = {}): Promise<unknown> {
    if (!validUuid(brand)) invalidInput("Brand ID must be a UUID");
    const write = options.method !== undefined && options.method !== "GET";
    if (write && (!options.key || !options.key.trim())) invalidInput("Idempotency-Key is required for writes");
    const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brand });
    if (options.body !== undefined) headers.set("Content-Type", "application/json");
    if (options.key) headers.set("Idempotency-Key", options.key);
    let response: Response;
    try {
      response = await fetchImpl(path, { method: options.method ?? "GET", credentials: "same-origin", headers,
        ...(options.body === undefined ? {} : { body: JSON.stringify(options.body) }) });
    } catch (error) { throw standardJoinCodeApiError(error); }
    let envelope: Envelope;
    try { envelope = await response.json() as Envelope; }
    catch { throw new AdminApiError(response.ok ? "Invalid server response" : `Request failed (${response.status})`, response.ok && write ? 0 : response.ok ? 502 : response.status, response.ok ? "INVALID_RESPONSE" : undefined); }
    if (!response.ok || envelope.success !== true || envelope.data === undefined) {
      const detail = typeof envelope.error === "object" && envelope.error ? envelope.error : undefined;
      throw new AdminApiError(typeof envelope.error === "string" ? envelope.error : detail?.message ?? `Request failed (${response.status})`,
        response.ok && write ? 0 : response.ok ? 502 : response.status, response.ok ? "INVALID_RESPONSE" : detail?.code);
    }
    return envelope.data;
  }
  return {
    async list(brand: string, filters: { kind?: JoinCodeKind; owner_member_id?: string } = {}, limit = 20, offset = 0): Promise<JoinCodePage> {
      pageArgs(limit, offset);
      if (filters.kind !== undefined && !validKind(filters.kind)) invalidInput("Invalid join code kind");
      if (filters.owner_member_id !== undefined && !validUuid(filters.owner_member_id)) invalidInput("Owner member ID must be a UUID");
      const query = new URLSearchParams();
      if (filters.kind !== undefined) query.set("kind", filters.kind);
      if (filters.owner_member_id !== undefined) query.set("owner_member_id", filters.owner_member_id);
      query.set("limit", String(limit)); query.set("offset", String(offset));
      const kind = filters.kind ?? null, owner = filters.owner_member_id ?? null;
      const value = await request(`${BASE}?${query}`, brand);
      if (!validPage(value, brand, kind, owner, limit, offset)) invalidResponse(false);
      return value;
    },
    async get(brand: string, id: string): Promise<JoinCode> {
      if (!validUuid(id)) invalidInput("Join code ID must be a UUID");
      const value = await request(`${BASE}/${encodeURIComponent(id)}`, brand);
      if (!validCode(value, brand, { id })) invalidResponse(false);
      return value;
    },
    async history(brand: string, id: string, limit = 20, offset = 0): Promise<JoinCodeHistoryPage> {
      if (!validUuid(id)) invalidInput("Join code ID must be a UUID");
      pageArgs(limit, offset);
      const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
      const value = await request(`${BASE}/${encodeURIComponent(id)}/history?${query}`, brand);
      if (!isRecord(value) || value.brand_id !== brand || value.code_id !== id || !Array.isArray(value.items) || value.items.length > limit ||
          !value.items.every((item) => validHistoryItem(item, brand, id)) || value.limit !== limit || value.offset !== offset ||
          typeof value.total_count !== "string" || !/^(0|[1-9]\d*)$/.test(value.total_count)) invalidResponse(false);
      return value as unknown as JoinCodeHistoryPage;
    },
    async create(brand: string, body: CreateJoinCodeBody, key: string): Promise<JoinCode> {
      if (!isRecord(body) || !exactKeys(body, ["kind", "owner_member_id", "agent_id", "starts_at", "expires_at", "reason"]) ||
          !validKind(body.kind) || !validUuid(body.owner_member_id) ||
          !(body.agent_id === null || validUuid(body.agent_id)) || (body.kind === "agent" ? !validUuid(body.agent_id) : body.agent_id !== null) ||
          !(body.starts_at === null || validJoinCodeDate(body.starts_at)) || !(body.expires_at === null || validJoinCodeDate(body.expires_at)) ||
          !validJoinCodeDateRange(body.starts_at, body.expires_at) || !validReason(body.reason)) invalidInput("Join code creation request is invalid");
      const value = await request(BASE, brand, { method: "POST", body, key });
      if (!validCode(value, brand, { kind: body.kind, owner_member_id: body.owner_member_id, agent_id: body.agent_id,
        status: "active", starts_at: body.starts_at, expires_at: body.expires_at, version: 1 }, true)) invalidResponse(true);
      return value;
    },
    async update(brand: string, id: string, identity: { kind: JoinCodeKind; code: string | null; owner_member_id: string; agent_id: string | null }, body: UpdateJoinCodeBody, key: string): Promise<JoinCode> {
      if (!validUuid(id) || !validKind(identity.kind) || typeof identity.code !== "string" || !CODE_RE.test(identity.code) || !validUuid(identity.owner_member_id) ||
          !(identity.agent_id === null || validUuid(identity.agent_id))) invalidInput("Join code identity is invalid");
      if (!isRecord(body) || !exactKeys(body, ["version", "status", "starts_at", "expires_at", "reason"]) ||
          !positiveInt(body.version) || body.version >= Number.MAX_SAFE_INTEGER || !validStatus(body.status) ||
          !(body.starts_at === null || validJoinCodeDate(body.starts_at)) || !(body.expires_at === null || validJoinCodeDate(body.expires_at)) ||
          !validJoinCodeDateRange(body.starts_at, body.expires_at) || !validReason(body.reason)) invalidInput("Join code update request is invalid");
      const value = await request(`${BASE}/${encodeURIComponent(id)}`, brand, { method: "PUT", body, key });
      if (!validCode(value, brand, { id, kind: identity.kind, code: identity.code, owner_member_id: identity.owner_member_id,
        agent_id: identity.agent_id, status: body.status, starts_at: body.starts_at, expires_at: body.expires_at, version: body.version + 1 }, true)) invalidResponse(true);
      return value;
    },
  };
}

export type JoinCodesApi = ReturnType<typeof createJoinCodesApi>;
