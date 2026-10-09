export type JoinCodeKind = "agent" | "referral";

export interface JoinCode {
  id: string;
  brand_id: string;
  kind: JoinCodeKind;
  code: string;
  owner_member_id: string;
  agent_id: string | null;
  status: "active" | "disabled";
  starts_at: string | null;
  expires_at: string | null;
  version: number;
  usable: boolean;
  created_at: string;
  updated_at: string;
  audit_log_id?: string;
}

export interface JoinCodePage {
  brand_id: string;
  member_id: string;
  items: JoinCode[];
  limit: number;
  offset: number;
  total_count: string;
}

export interface MemberAttribution {
  brand_id: string;
  member_id: string;
  join_method: "domain" | "agent_code" | "referral_code" | "operator";
  joined_at: string;
  code_id: string | null;
  source_code: string | null;
}

export interface JoinCodeContext {
  brand_id: string;
  member_id: string;
}

interface Envelope<T> {
  success?: boolean;
  data?: T;
  error?: string | { code?: string; message?: string } | null;
}

export class JoinCodesApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code?: string,
  ) {
    super(message);
    this.name = "JoinCodesApiError";
  }
}

const UUID =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const JOIN_CODE = /^[0-9A-F]{24}$/;
const CANONICAL_COUNT = /^(?:0|[1-9]\d*)$/;
const ISO_DATE_TIME =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,6}))?(Z|[+-](\d{2}):(\d{2}))$/;
const MAX_INT64 = "9223372036854775807";

function malformed(message: string): never {
  throw new JoinCodesApiError(
    `Malformed join codes API response: ${message}`,
    502,
    "invalid_response",
  );
}

function object(value: unknown, label: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return malformed(`${label} must be an object`);
  }
  return value as Record<string, unknown>;
}

function exactKeys(
  value: Record<string, unknown>,
  label: string,
  allowed: string[],
): void {
  const unexpected = Object.keys(value).find((key) => !allowed.includes(key));
  if (unexpected) malformed(`${label}.${unexpected} is not part of the public DTO`);
}

function uuid(value: unknown, label: string): string {
  if (typeof value !== "string" || !UUID.test(value)) {
    return malformed(`${label} must be a UUID`);
  }
  return value;
}

function dateTime(value: unknown, label: string): string {
  if (typeof value !== "string") {
    return malformed(`${label} must be an ISO-8601 date-time`);
  }
  const parts = ISO_DATE_TIME.exec(value);
  if (!parts || !Number.isFinite(Date.parse(value))) {
    return malformed(`${label} must be an ISO-8601 date-time`);
  }
  const [, year, month, day, hour, minute, second, , zone, offsetHour, offsetMinute] = parts;
  const y = Number(year);
  const m = Number(month);
  const d = Number(day);
  const daysInMonth =
    m === 2
      ? y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0)
        ? 29
        : 28
      : [4, 6, 9, 11].includes(m)
        ? 30
        : 31;
  if (
    m < 1 ||
    m > 12 ||
    d < 1 ||
    d > daysInMonth ||
    Number(hour) > 23 ||
    Number(minute) > 59 ||
    Number(second) > 59 ||
    (zone !== "Z" && (Number(offsetHour) > 23 || Number(offsetMinute) > 59))
  ) {
    return malformed(`${label} must be a valid ISO-8601 date-time`);
  }
  return value;
}

function parseJoinCode(value: unknown, context: JoinCodeContext): JoinCode {
  const data = object(value, "join code");
  exactKeys(data, "join code", [
    "id", "brand_id", "kind", "code", "owner_member_id", "agent_id", "status",
    "starts_at", "expires_at", "version", "usable", "created_at", "updated_at",
    "audit_log_id",
  ]);
  const id = uuid(data.id, "join code.id");
  const brandId = uuid(data.brand_id, "join code.brand_id");
  const ownerMemberId = uuid(data.owner_member_id, "join code.owner_member_id");
  if (brandId !== context.brand_id || ownerMemberId !== context.member_id) {
    return malformed("join code must belong to the authenticated member and brand");
  }
  if (data.kind !== "agent" && data.kind !== "referral") {
    return malformed("join code.kind is unsupported");
  }
  if (typeof data.code !== "string" || !JOIN_CODE.test(data.code)) {
    return malformed("join code.code must be 24 uppercase hexadecimal characters");
  }
  const agentId =
    data.agent_id === null ? null : uuid(data.agent_id, "join code.agent_id");
  if ((data.kind === "agent") !== (agentId !== null)) {
    return malformed("join code.agent_id must match its kind");
  }
  if (data.status !== "active" && data.status !== "disabled") {
    return malformed("join code.status is unsupported");
  }
  const startsAt =
    data.starts_at === null ? null : dateTime(data.starts_at, "join code.starts_at");
  const expiresAt =
    data.expires_at === null ? null : dateTime(data.expires_at, "join code.expires_at");
  if (
    startsAt !== null &&
    expiresAt !== null &&
    Date.parse(startsAt) >= Date.parse(expiresAt)
  ) {
    return malformed("join code.starts_at must be before expires_at");
  }
  if (!Number.isSafeInteger(data.version) || Number(data.version) < 1) {
    return malformed("join code.version must be a positive safe integer");
  }
  const version = data.version as number;
  if (typeof data.usable !== "boolean") {
    return malformed("join code.usable must be a boolean");
  }
  const createdAt = dateTime(data.created_at, "join code.created_at");
  const result: JoinCode = {
    id,
    brand_id: brandId,
    kind: data.kind,
    code: data.code,
    owner_member_id: ownerMemberId,
    agent_id: agentId,
    status: data.status,
    starts_at: startsAt,
    expires_at: expiresAt,
    version,
    usable: data.usable,
    created_at: createdAt,
    updated_at: dateTime(data.updated_at, "join code.updated_at"),
  };
  if (data.audit_log_id !== undefined) {
    result.audit_log_id = uuid(data.audit_log_id, "join code.audit_log_id");
  }
  return result;
}

function parsePage(
  value: unknown,
  context: JoinCodeContext,
  limit: number,
  offset: number,
): JoinCodePage {
  const data = object(value, "data");
  exactKeys(data, "join code page", ["brand_id", "member_id", "items", "limit", "offset", "total_count"]);
  const brandId = uuid(data.brand_id, "brand_id");
  const memberId = uuid(data.member_id, "member_id");
  if (brandId !== context.brand_id || memberId !== context.member_id) {
    return malformed("page scope does not match the authenticated member and brand");
  }
  if (!Array.isArray(data.items) || data.items.length > limit) {
    return malformed("items must be an array within the requested page size");
  }
  const items = data.items.map((item) => parseJoinCode(item, context));
  if (new Set(items.map((item) => item.id)).size !== items.length) {
    return malformed("join code ids must be unique");
  }
  if (data.limit !== limit || data.offset !== offset) {
    return malformed("limit and offset must match the request");
  }
  if (
    typeof data.total_count !== "string" ||
    !CANONICAL_COUNT.test(data.total_count) ||
    data.total_count.length > MAX_INT64.length ||
    (data.total_count.length === MAX_INT64.length && data.total_count > MAX_INT64)
  ) {
    return malformed("total_count must be a canonical nonnegative int64 string");
  }
  if (BigInt(data.total_count) < BigInt(offset + items.length)) {
    return malformed("total_count cannot be less than the page end");
  }
  return { brand_id: brandId, member_id: memberId, items, limit, offset, total_count: data.total_count };
}

function parseAttribution(
  value: unknown,
  context: JoinCodeContext,
): MemberAttribution {
  const data = object(value, "attribution");
  exactKeys(data, "attribution", ["brand_id", "member_id", "join_method", "joined_at", "code_id", "source_code"]);
  const brandId = uuid(data.brand_id, "attribution.brand_id");
  const memberId = uuid(data.member_id, "attribution.member_id");
  if (brandId !== context.brand_id || memberId !== context.member_id) {
    return malformed("attribution scope does not match the authenticated member and brand");
  }
  const methods = ["domain", "agent_code", "referral_code", "operator"] as const;
  if (typeof data.join_method !== "string" || !methods.includes(data.join_method as (typeof methods)[number])) {
    return malformed("attribution.join_method is unsupported");
  }
  const codeId = data.code_id === null ? null : uuid(data.code_id, "attribution.code_id");
  let sourceCode: string | null;
  if (data.source_code === null) sourceCode = null;
  else if (typeof data.source_code === "string" && JOIN_CODE.test(data.source_code)) sourceCode = data.source_code;
  else return malformed("attribution.source_code must be a join code or null");
  if ((codeId === null) !== (sourceCode === null)) {
    return malformed("attribution code_id and source_code must both be null or present");
  }
  return {
    brand_id: brandId,
    member_id: memberId,
    join_method: data.join_method as MemberAttribution["join_method"],
    joined_at: dateTime(data.joined_at, "attribution.joined_at"),
    code_id: codeId,
    source_code: sourceCode,
  };
}

export function normalizeJoinCode(value: string): string {
  const normalized = value.trim().toUpperCase();
  if (normalized && !JOIN_CODE.test(normalized)) {
    throw new JoinCodesApiError(
      "Join code must contain exactly 24 hexadecimal characters",
      0,
      "invalid_parameter",
    );
  }
  return normalized;
}

export function createJoinCodesApi(
  options: { brandCode?: string; fetch?: typeof fetch } = {},
) {
  const brandCode = options.brandCode ?? import.meta.env.VITE_BRAND_CODE ?? undefined;
  const base = brandCode
    ? `/api/v1/b/${encodeURIComponent(brandCode)}`
    : "/api/v1";
  const fetcher = options.fetch ?? fetch;

  async function request(path: string): Promise<unknown> {
    let response: Response;
    try {
      response = await fetcher(`${base}${path}`, {
        method: "GET",
        credentials: "include",
        headers: { Accept: "application/json" },
      });
    } catch (cause) {
      throw new JoinCodesApiError(
        cause instanceof Error ? cause.message : "Network request failed",
        0,
        "network_error",
      );
    }
    let envelope: Envelope<unknown>;
    try {
      envelope = (await response.json()) as Envelope<unknown>;
    } catch (cause) {
      if (!response.ok) throw new JoinCodesApiError(`Request failed (${response.status})`, response.status);
      throw new JoinCodesApiError(
        cause instanceof Error ? cause.message : "Invalid response",
        502,
        "invalid_response",
      );
    }
    if (!response.ok) {
      const error = typeof envelope?.error === "object" && envelope.error ? envelope.error : undefined;
      throw new JoinCodesApiError(
        typeof envelope?.error === "string" ? envelope.error : error?.message ?? `Request failed (${response.status})`,
        response.status,
        error?.code,
      );
    }
    if (!envelope || envelope.success !== true || envelope.data === undefined) {
      return malformed("successful response must contain a success envelope and data");
    }
    return envelope.data;
  }

  return {
    async list(context: JoinCodeContext, limit = 20, offset = 0): Promise<JoinCodePage> {
      validateContext(context);
      if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000) {
        throw new JoinCodesApiError("limit must be 1 to 100 and offset 0 to 1000000", 0, "invalid_parameter");
      }
      const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
      return parsePage(await request(`/me/join-codes?${query}`), context, limit, offset);
    },
    async attribution(context: JoinCodeContext): Promise<MemberAttribution> {
      validateContext(context);
      return parseAttribution(await request("/me/attribution"), context);
    },
  };
}

function validateContext(context: JoinCodeContext): void {
  if (!context || typeof context !== "object" || !UUID.test(context.brand_id) || !UUID.test(context.member_id)) {
    throw new JoinCodesApiError("brand_id and member_id must be UUIDs", 0, "invalid_parameter");
  }
}

export type JoinCodesApi = ReturnType<typeof createJoinCodesApi>;
