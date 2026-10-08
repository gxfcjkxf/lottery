import { AdminApiError } from "./admin-api";

const ORDERS = "/api/v1/admin/reward-orders";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const UINT = /^(?:0|[1-9][0-9]*)$/;
const DATE_TIME = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/;
const MAX_INT64 = 9_223_372_036_854_775_807n;
const MAX_SAFE_VERSION = Number.MAX_SAFE_INTEGER;

export type RewardState = "granted" | "revocation_pending" | "revoked";
export type RewardOperation = "grant" | "revoke" | "retry";

export interface RewardOrder {
  id: string;
  brand_id: string;
  member_id: string;
  points: string;
  state: RewardState;
  version: number;
  grant_ledger_entry_id: string;
  revoke_ledger_entry_id: string | null;
  creation_audit_log_id: string;
  last_audit_log_id: string;
  last_error_code: string | null;
  created_by: string;
  reason: string;
  point_policy_version: string;
  created_at: string;
  updated_at: string;
  revoked_at: string | null;
}

export interface RewardOrderPage {
  brand_id: string;
  items: RewardOrder[];
  total_count: string;
  limit: number;
  offset: number;
}

export interface RewardAction {
  id: string;
  brand_id: string;
  order_id: string;
  version: number;
  operation: RewardOperation;
  state_before: RewardState | null;
  state_after: RewardState;
  actor_id: string;
  reason: string;
  audit_log_id: string;
  ledger_entry_id: string | null;
  created_at: string;
}

export interface RewardActionPage {
  brand_id: string;
  order_id: string;
  items: RewardAction[];
  total_count: string;
  limit: number;
  offset: number;
}

export interface RewardGrantBody {
  member_id: string;
  points: string;
  reason: string;
}

export interface RewardActionBody {
  version: number;
  reason: string;
}

export class RewardsApiError extends AdminApiError {
  constructor(message: string, status: number, code?: string) {
    super(message, status, code);
    this.name = "RewardsApiError";
  }
}

type ObjectValue = Record<string, unknown>;
type Envelope = ObjectValue & { success?: unknown; data?: unknown; error?: unknown; request_id?: unknown };
const ORDER_KEYS = [
  "id", "brand_id", "member_id", "points", "state", "version", "grant_ledger_entry_id", "revoke_ledger_entry_id",
  "creation_audit_log_id", "last_audit_log_id", "last_error_code", "created_by", "reason", "point_policy_version",
  "created_at", "updated_at", "revoked_at",
] as const;
const ACTION_KEYS = [
  "id", "brand_id", "order_id", "version", "operation", "state_before", "state_after", "actor_id", "reason",
  "audit_log_id", "ledger_entry_id", "created_at",
] as const;
const ORDER_PAGE_KEYS = ["brand_id", "items", "total_count", "limit", "offset"] as const;
const ACTION_PAGE_KEYS = ["brand_id", "order_id", "items", "total_count", "limit", "offset"] as const;
const REWARD_STATES: readonly string[] = ["granted", "revocation_pending", "revoked"];

function isObject(value: unknown): value is ObjectValue {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function hasExactKeys(value: ObjectValue, keys: readonly string[]): boolean {
  const actual = Object.keys(value);
  return actual.length === keys.length && actual.every((key) => keys.includes(key));
}

function isUuid(value: unknown): value is string {
  return typeof value === "string" && UUID.test(value);
}

function isRequestID(value: unknown): value is string {
  return typeof value === "string" && /^[A-Za-z0-9_.:-]{1,80}$/.test(value);
}

function isSafeVersion(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 1 && value <= MAX_SAFE_VERSION;
}

function isUnsigned(value: unknown, positive = false): value is string {
  if (typeof value !== "string" || !UINT.test(value)) return false;
  try {
    const amount = BigInt(value);
    return amount <= MAX_INT64 && (!positive || amount > 0n);
  } catch {
    return false;
  }
}

function isDateTime(value: unknown): value is string {
  if (typeof value !== "string" || !DATE_TIME.test(value) || !Number.isFinite(Date.parse(value))) return false;
  const [date, rawTime] = value.split("T");
  const [year, month, day] = date.split("-").map(Number);
  const match = /^(\d{2}):(\d{2}):(\d{2})(?:\.\d{1,9})?(Z|([+-])(\d{2}):(\d{2}))$/.exec(rawTime);
  if (!match) return false;
  const hour = Number(match[1]);
  const minute = Number(match[2]);
  const second = Number(match[3]);
  const lastDay = new Date(0);
  lastDay.setUTCFullYear(year, month, 0);
  const offsetHour = Number(match[6] ?? "0");
  const offsetMinute = Number(match[7] ?? "0");
  return year >= 1 && month >= 1 && month <= 12 && day >= 1 && day <= lastDay.getUTCDate() &&
    hour <= 23 && minute <= 59 && second <= 59 && offsetMinute <= 59 &&
    (match[4] === "Z" || offsetHour < 14 || (offsetHour === 14 && offsetMinute === 0));
}

function instantNanoseconds(value: string): bigint {
  const match = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})$/.exec(value);
  if (!match) return 0n;
  const wholeSecondMillis = Date.parse(`${match[1]}${match[3]}`);
  const epochSeconds = Math.floor(wholeSecondMillis / 1_000);
  return BigInt(epochSeconds) * 1_000_000_000n + BigInt((match[2] ?? "").padEnd(9, "0"));
}

function isReason(value: unknown): value is string {
  if (typeof value !== "string" || value.length === 0 || value.trim() !== value || /[\u0000-\u001f\u007f-\u009f]/u.test(value)) return false;
  for (let i = 0; i < value.length; i++) {
    const code = value.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff) {
      const next = value.charCodeAt(++i);
      if (!(next >= 0xdc00 && next <= 0xdfff)) return false;
    } else if (code >= 0xdc00 && code <= 0xdfff) {
      return false;
    }
  }
  return new TextEncoder().encode(value).length <= 500;
}

function validOrder(value: unknown, brand: string): value is RewardOrder {
  if (!isObject(value) || !hasExactKeys(value, ORDER_KEYS) || value.brand_id !== brand ||
    !isUuid(value.id) || !isUuid(value.brand_id) || !isUuid(value.member_id) || !isUnsigned(value.points, true) ||
    !REWARD_STATES.includes(String(value.state)) || !isSafeVersion(value.version) ||
    !isUuid(value.grant_ledger_entry_id) || !isUuid(value.creation_audit_log_id) || !isUuid(value.last_audit_log_id) ||
    !isUuid(value.created_by) || !isReason(value.reason) || !isUnsigned(value.point_policy_version, true) ||
    !isDateTime(value.created_at) || !isDateTime(value.updated_at) || instantNanoseconds(value.updated_at) < instantNanoseconds(value.created_at)) return false;
  if (!(value.revoke_ledger_entry_id === null || isUuid(value.revoke_ledger_entry_id)) ||
    !(value.last_error_code === null || typeof value.last_error_code === "string") ||
    !(value.revoked_at === null || isDateTime(value.revoked_at))) return false;

  switch (value.state) {
    case "granted":
      return value.version === 1 && value.revoke_ledger_entry_id === null && value.revoked_at === null && value.last_error_code === null;
    case "revocation_pending":
      return value.version >= 2 && value.revoke_ledger_entry_id === null && value.revoked_at === null &&
        value.last_error_code === "REWARD_AVAILABLE_INSUFFICIENT";
    case "revoked":
      return value.version >= 2 && isUuid(value.revoke_ledger_entry_id) && value.revoked_at !== null &&
        instantNanoseconds(value.revoked_at) >= instantNanoseconds(value.created_at) && instantNanoseconds(value.revoked_at) <= instantNanoseconds(value.updated_at) &&
        value.last_error_code === null;
    default:
      return false;
  }
}

function validAction(value: unknown, brand: string, orderID: string): value is RewardAction {
  if (!isObject(value) || !hasExactKeys(value, ACTION_KEYS) || value.brand_id !== brand || value.order_id !== orderID ||
    !isUuid(value.id) || !isUuid(value.brand_id) || !isUuid(value.order_id) || !isSafeVersion(value.version) ||
    !["grant", "revoke", "retry"].includes(String(value.operation)) ||
    !(value.state_before === null || REWARD_STATES.includes(String(value.state_before))) ||
    !REWARD_STATES.includes(String(value.state_after)) || !isUuid(value.actor_id) || !isReason(value.reason) ||
    !isUuid(value.audit_log_id) || !(value.ledger_entry_id === null || isUuid(value.ledger_entry_id)) || !isDateTime(value.created_at)) return false;

  if (value.operation === "grant") {
    return value.version === 1 && value.state_before === null && value.state_after === "granted" && isUuid(value.ledger_entry_id);
  }
  if (value.operation === "revoke") {
    return value.version > 1 && value.state_before === "granted" &&
      (value.state_after === "revocation_pending" ? value.ledger_entry_id === null : value.state_after === "revoked" && isUuid(value.ledger_entry_id));
  }
  return value.version > 1 && value.state_before === "revocation_pending" &&
    (value.state_after === "revocation_pending" ? value.ledger_entry_id === null : value.state_after === "revoked" && isUuid(value.ledger_entry_id));
}

function validOrderPage(value: unknown, brand: string, limit: number, offset: number): value is RewardOrderPage {
  if (!isObject(value) || !hasExactKeys(value, ORDER_PAGE_KEYS) || value.brand_id !== brand || !isUuid(value.brand_id) ||
    !Array.isArray(value.items) || value.items.length > limit || !isUnsigned(value.total_count) ||
    value.limit !== limit || value.offset !== offset || !value.items.every((item) => validOrder(item, brand))) return false;
  const items = value.items as RewardOrder[];
  if (new Set(items.map((item) => item.id)).size !== items.length ||
    (items.length > 0 && BigInt(value.total_count) < BigInt(offset) + BigInt(items.length))) return false;
  return items.every((item, index) => index === 0 || compareOrder(items[index - 1], item) <= 0);
}

function compareOrder(left: RewardOrder, right: RewardOrder): number {
  const timeDifference = instantNanoseconds(right.created_at) - instantNanoseconds(left.created_at);
  if (timeDifference !== 0n) return timeDifference < 0n ? -1 : 1;
  return left.id === right.id ? 0 : left.id > right.id ? -1 : 1;
}

function validActionPage(value: unknown, brand: string, orderID: string, limit: number, offset: number): value is RewardActionPage {
  if (!isObject(value) || !hasExactKeys(value, ACTION_PAGE_KEYS) || value.brand_id !== brand || value.order_id !== orderID ||
    !isUuid(value.brand_id) || !isUuid(value.order_id) || !Array.isArray(value.items) || value.items.length > limit ||
    !isUnsigned(value.total_count) || value.limit !== limit || value.offset !== offset ||
    !value.items.every((item) => validAction(item, brand, orderID))) return false;
  const items = value.items as RewardAction[];
  if (new Set(items.map((item) => item.id)).size !== items.length || new Set(items.map((item) => item.version)).size !== items.length ||
    (items.length > 0 && BigInt(value.total_count) < BigInt(offset) + BigInt(items.length))) return false;
  return items.every((item, index) => index === 0 || items[index - 1].version > item.version);
}

function invalidInput(): never {
  throw new RewardsApiError("奖励请求参数不正确。", 0, "INVALID_INPUT");
}

function invalidResponse(): never {
  throw new RewardsApiError("奖励服务响应格式无效。", 502, "INVALID_RESPONSE");
}

function validateBrand(brand: string): void {
  if (!isUuid(brand)) invalidInput();
}

function validatePage(limit: number, offset: number): void {
  if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000) invalidInput();
}

function validateKey(key: string): void {
  if (typeof key !== "string" || !/^[A-Za-z0-9._:-]{8,128}$/.test(key)) invalidInput();
}

function validateGrantBody(body: Readonly<RewardGrantBody>): void {
  if (!isObject(body) || !hasExactKeys(body, ["member_id", "points", "reason"]) || !isUuid(body.member_id) ||
    !isUnsigned(body.points, true) || !isReason(body.reason)) invalidInput();
}

function validateActionBody(body: Readonly<RewardActionBody>): void {
  if (!isObject(body) || !hasExactKeys(body, ["version", "reason"]) || !isSafeVersion(body.version) ||
    body.version >= MAX_SAFE_VERSION || !isReason(body.reason)) invalidInput();
}

function snapshotBody<T extends object>(value: unknown, validate: (value: unknown) => value is T): Readonly<T> {
  let encoded: string | undefined;
  try {
    encoded = JSON.stringify(value);
  } catch {
    invalidInput();
  }
  if (encoded === undefined) invalidInput();
  let snapshot: unknown;
  try {
    snapshot = JSON.parse(encoded);
  } catch {
    invalidInput();
  }
  if (!validate(snapshot)) invalidInput();
  // Reward request DTOs are flat scalar-only objects, so a shallow freeze is
  // sufficient. The same clone is serialized and used to validate the receipt.
  return Object.freeze(snapshot);
}

function invalidUnauthorizedResponse(): never {
  throw new RewardsApiError("奖励请求失败（401）。", 401, "AUTH_UNAUTHENTICATED");
}

function safeAPIErrorCode(value: unknown): string | undefined {
  if (typeof value !== "string") return undefined;
  return /^(?:REWARD_[A-Z0-9_]{1,80}|AUTH_[A-Z0-9_]{1,80}|PERMISSION_DENIED|CSRF_REJECTED|CONTENT_TYPE_INVALID|REQUEST_INVALID)$/.test(value)
    ? value
    : undefined;
}

export function createRewardsApi(fetcher: typeof fetch = fetch, actorID = "") {
  // This closure intentionally captures the actor identity once. The API does
  // not infer role permissions from this header; the server remains authoritative.
  const capturedActorID = actorID;

  async function request<T>(
    brand: string,
    path: string,
    method: "GET" | "POST",
    expectedStatus: number,
    validate: (value: unknown) => value is T,
    body?: unknown,
    key?: string,
  ): Promise<T> {
    const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brand });
    if (method === "POST") {
      if (!isUuid(capturedActorID)) throw new RewardsApiError("需要固定的管理员身份才能执行奖励写入。", 0, "AUTH_ACTOR_REQUIRED");
      validateKey(key ?? "");
      headers.set("Content-Type", "application/json");
      headers.set("X-Reward-Actor-ID", capturedActorID);
      headers.set("Idempotency-Key", key!);
    }

    let response: Response;
    try {
      response = await fetcher(path, {
        method,
        credentials: "include",
        headers,
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      });
    } catch {
      throw new RewardsApiError("奖励请求暂时无法连接。", 0, "NETWORK_ERROR");
    }

    let envelope: Envelope;
    try {
      const decoded: unknown = await response.json();
      if (!isObject(decoded)) {
        if (response.status === 401) invalidUnauthorizedResponse();
        invalidResponse();
      }
      envelope = decoded;
    } catch (error) {
      if (error instanceof RewardsApiError) throw error;
      if (response.status === 401) invalidUnauthorizedResponse();
      if (response.ok) invalidResponse();
      throw new RewardsApiError(`奖励请求失败（${response.status}）。`, response.status, "API_ERROR");
    }

    if (!response.ok) {
      const error = isObject(envelope.error) ? envelope.error : undefined;
      const code = safeAPIErrorCode(error?.code) ?? "API_ERROR";
      if (!hasExactKeys(envelope, ["success", "error", "request_id"]) || !error || !hasExactKeys(error, ["code", "message"]) ||
        envelope.success !== false || !isReason(error.message) || !isRequestID(envelope.request_id) ||
        (response.status === 401 && !safeAPIErrorCode(error.code))) {
        if (response.status === 401) invalidUnauthorizedResponse();
        invalidResponse();
      }
      throw new RewardsApiError(`奖励请求失败：${code}`, response.status, code);
    }
    if (response.status !== expectedStatus || !hasExactKeys(envelope, ["success", "data", "request_id"]) ||
      envelope.success !== true || !isRequestID(envelope.request_id) || !validate(envelope.data)) invalidResponse();
    return envelope.data;
  }

  function pageQuery(limit: number, offset: number): string {
    validatePage(limit, offset);
    return new URLSearchParams({ limit: String(limit), offset: String(offset) }).toString();
  }

  return {
    async list(brand: string, limit = 20, offset = 0): Promise<RewardOrderPage> {
      validateBrand(brand);
      const data = await request(brand, `${ORDERS}?${pageQuery(limit, offset)}`, "GET", 200,
        (value): value is RewardOrderPage => validOrderPage(value, brand, limit, offset));
      return data;
    },

    async create(brand: string, body: Readonly<RewardGrantBody>, key: string): Promise<RewardOrder> {
      validateBrand(brand);
      const snapshot = snapshotBody(body, (value): value is RewardGrantBody => {
        validateGrantBody(value as Readonly<RewardGrantBody>);
        return true;
      });
      validateKey(key);
      const data = await request(brand, ORDERS, "POST", 201,
        (value): value is RewardOrder => validOrder(value, brand) && value.member_id === snapshot.member_id && value.points === snapshot.points &&
          value.state === "granted" && value.version === 1 && value.created_by === capturedActorID,
        snapshot, key);
      return data;
    },

    async read(brand: string, id: string): Promise<RewardOrder> {
      validateBrand(brand);
      if (!isUuid(id)) invalidInput();
      const data = await request(brand, `${ORDERS}/${encodeURIComponent(id)}`, "GET", 200,
        (value): value is RewardOrder => validOrder(value, brand) && value.id === id);
      return data;
    },

    async actions(brand: string, id: string, limit = 20, offset = 0): Promise<RewardActionPage> {
      validateBrand(brand);
      if (!isUuid(id)) invalidInput();
      const query = pageQuery(limit, offset);
      const data = await request(brand, `${ORDERS}/${encodeURIComponent(id)}/actions?${query}`, "GET", 200,
        (value): value is RewardActionPage => validActionPage(value, brand, id, limit, offset));
      return data;
    },

    async revoke(brand: string, id: string, body: Readonly<RewardActionBody>, key: string): Promise<RewardOrder> {
      return writeAction("revoke", brand, id, body, key);
    },

    async retryRevocation(brand: string, id: string, body: Readonly<RewardActionBody>, key: string): Promise<RewardOrder> {
      return writeAction("retry-revocation", brand, id, body, key);
    },
  };

  async function writeAction(pathAction: "revoke" | "retry-revocation", brand: string, id: string, body: Readonly<RewardActionBody>, key: string): Promise<RewardOrder> {
    validateBrand(brand);
    if (!isUuid(id)) invalidInput();
    const snapshot = snapshotBody(body, (value): value is RewardActionBody => {
      validateActionBody(value as Readonly<RewardActionBody>);
      return true;
    });
    validateKey(key);
    const data = await request(brand, `${ORDERS}/${encodeURIComponent(id)}/${pathAction}`, "POST", 200,
      (value): value is RewardOrder => validOrder(value, brand) && value.id === id && value.version === snapshot.version + 1 &&
        (value.state === "revocation_pending" || value.state === "revoked"),
      snapshot, key);
    return data;
  }
}
