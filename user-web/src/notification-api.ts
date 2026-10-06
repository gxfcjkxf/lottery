import { walletBasePath } from "./wallet-api";

export type NotificationEventType =
  | "member.joined"
  | "recharge.confirmed"
  | "bet.order.placed"
  | "bet.order.cancelled"
  | "bet.order.judged_cancelled"
  | "bet.order.abnormal";

export interface NotificationItem {
  id: string;
  brand_id: string;
  member_id: string;
  event_type: NotificationEventType;
  template_key: NotificationEventType;
  template_version: 1;
  payload: { resource_id: string; points: string | null };
  created_at: string;
  read_at: string | null;
}

export interface NotificationPage {
  brand_id: string;
  member_id: string;
  items: NotificationItem[];
  unread_count: string;
  limit: number;
  offset: number;
}

export interface NotificationReadReceipt {
  brand_id: string;
  member_id: string;
  ids: string[];
  changed: number;
  unread_count: string;
}

interface Envelope<T> {
  success?: boolean;
  data?: T;
  error?: string | { code?: string; message?: string } | null;
}

export class NotificationApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code?: string,
  ) {
    super(message);
    this.name = "NotificationApiError";
  }
}

const UUID =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const ISO_DATE_TIME =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(Z|[+-](\d{2}):(\d{2}))$/;
const POSITIVE_INT64 = /^(?:[1-9]\d*)$/;
const NONNEGATIVE_INT64 = /^(?:0|[1-9]\d*)$/;
const MAX_INT64 = "9223372036854775807";
const IDEMPOTENCY_KEY = /^[A-Za-z0-9_:.-]{8,128}$/;
const EVENT_TYPES = new Set<NotificationEventType>([
  "member.joined",
  "recharge.confirmed",
  "bet.order.placed",
  "bet.order.cancelled",
  "bet.order.judged_cancelled",
  "bet.order.abnormal",
]);

function malformed(message: string): never {
  throw new NotificationApiError(
    `Malformed notifications API response: ${message}`,
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

function uuid(value: unknown, label: string): string {
  if (typeof value !== "string" || !UUID.test(value)) {
    return malformed(`${label} must be a UUID`);
  }
  return value;
}

function int64(value: unknown, label: string, positive = false): string {
  const pattern = positive ? POSITIVE_INT64 : NONNEGATIVE_INT64;
  if (
    typeof value !== "string" ||
    !pattern.test(value) ||
    value.length > MAX_INT64.length ||
    (value.length === MAX_INT64.length && value > MAX_INT64)
  ) {
    return malformed(`${label} must be a canonical ${positive ? "positive " : "nonnegative "}int64 string`);
  }
  return value;
}

function dateTime(value: unknown, label: string): string {
  if (typeof value !== "string") return malformed(`${label} must be an ISO-8601 string`);
  const parts = ISO_DATE_TIME.exec(value);
  if (!parts || !Number.isFinite(Date.parse(value))) {
    return malformed(`${label} must be an ISO-8601 date-time`);
  }
  const [, year, month, day, hour, minute, second, , offsetHour, offsetMinute] =
    parts;
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
    Number(offsetHour ?? 0) > 23 ||
    Number(offsetMinute ?? 0) > 59
  ) {
    return malformed(`${label} must be a valid ISO-8601 date-time`);
  }
  return value;
}

function inputUuid(value: unknown, label: string): string {
  if (typeof value !== "string" || !UUID.test(value)) {
    throw new NotificationApiError(`${label} must be a UUID`, 0, "invalid_parameter");
  }
  return value;
}

function parseNotification(value: unknown): NotificationItem {
  const item = object(value, "notification");
  const id = uuid(item.id, "notification.id");
  const brandId = uuid(item.brand_id, "notification.brand_id");
  const memberId = uuid(item.member_id, "notification.member_id");
  if (typeof item.event_type !== "string" || !EVENT_TYPES.has(item.event_type as NotificationEventType)) {
    return malformed("notification.event_type is unsupported");
  }
  const eventType = item.event_type as NotificationEventType;
  if (item.template_key !== eventType) {
    return malformed("notification.template_key must match event_type");
  }
  if (item.template_version !== 1) {
    return malformed("notification.template_version must be 1");
  }
  const payload = object(item.payload, "notification.payload");
  const resourceId = uuid(payload.resource_id, "notification.payload.resource_id");
  let points: string | null;
  if (eventType === "member.joined") {
    if (payload.points !== null || resourceId !== memberId) {
      return malformed("member.joined must reference its member and have null points");
    }
    points = null;
  } else {
    points = int64(payload.points, "notification.payload.points", true);
  }
  return {
    id,
    brand_id: brandId,
    member_id: memberId,
    event_type: eventType,
    template_key: eventType,
    template_version: 1,
    payload: { resource_id: resourceId, points },
    created_at: dateTime(item.created_at, "notification.created_at"),
    read_at:
      item.read_at === null
        ? null
        : dateTime(item.read_at, "notification.read_at"),
  };
}

function parsePage(
  value: unknown,
  requestedLimit: number,
  requestedOffset: number,
): NotificationPage {
  const data = object(value, "data");
  const brandId = uuid(data.brand_id, "brand_id");
  const memberId = uuid(data.member_id, "member_id");
  if (!Array.isArray(data.items)) return malformed("items must be an array");
  if (data.items.length > requestedLimit) {
    return malformed("items must not exceed the requested limit");
  }
  const items = data.items.map(parseNotification);
  const ids = new Set<string>();
  for (const item of items) {
    if (item.brand_id !== brandId || item.member_id !== memberId) {
      return malformed("notification scope must match page scope");
    }
    if (ids.has(item.id)) return malformed("notification ids must be unique");
    ids.add(item.id);
  }
  const unreadCount = int64(data.unread_count, "unread_count");
  const pageUnreadCount = items.reduce(
    (count, item) => count + Number(item.read_at === null),
    0,
  );
  const minimumUnreadCount = String(pageUnreadCount);
  if (
    unreadCount.length < minimumUnreadCount.length ||
    (unreadCount.length === minimumUnreadCount.length &&
      unreadCount < minimumUnreadCount)
  ) {
    return malformed("unread_count cannot be less than unread items in this page");
  }
  if (typeof data.limit !== "number" || data.limit !== requestedLimit) {
    return malformed("limit must match the requested limit");
  }
  if (typeof data.offset !== "number" || data.offset !== requestedOffset) {
    return malformed("offset must match the requested offset");
  }
  return {
    brand_id: brandId,
    member_id: memberId,
    items,
    unread_count: unreadCount,
    limit: requestedLimit,
    offset: requestedOffset,
  };
}

function parseReceipt(
  value: unknown,
  ids: string[],
  context: { brand_id: string; member_id: string },
): NotificationReadReceipt {
  const data = object(value, "data");
  const brandId = uuid(data.brand_id, "brand_id");
  const memberId = uuid(data.member_id, "member_id");
  if (brandId !== context.brand_id || memberId !== context.member_id) {
    return malformed("read receipt scope does not match the requested context");
  }
  if (!Array.isArray(data.ids) || data.ids.some((id) => typeof id !== "string")) {
    return malformed("ids must be a UUID array");
  }
  const returnedIds = data.ids.map((id, index) => uuid(id, `ids[${index}]`));
  if (
    returnedIds.length !== ids.length ||
    returnedIds.some((id, index) => id !== ids[index])
  ) {
    return malformed("read receipt ids must match the request in order");
  }
  if (
    typeof data.changed !== "number" ||
    !Number.isSafeInteger(data.changed) ||
    data.changed < 0 ||
    data.changed > ids.length
  ) {
    return malformed("changed must be an integer within the requested id count");
  }
  return {
    brand_id: brandId,
    member_id: memberId,
    ids: returnedIds,
    changed: data.changed,
    unread_count: int64(data.unread_count, "unread_count"),
  };
}

export function createNotificationApi(
  options: { brandCode?: string; fetch?: typeof fetch } = {},
) {
  const brandCode =
    options.brandCode ?? import.meta.env.VITE_BRAND_CODE ?? undefined;
  const fetcher = options.fetch ?? fetch;
  const base = walletBasePath(brandCode);
  let listedContext: { brand_id: string; member_id: string } | undefined;

  async function request<T>(
    path: string,
    method: "GET" | "POST",
    body?: unknown,
    idempotencyKey?: string,
  ): Promise<T> {
    const headers = new Headers({ Accept: "application/json" });
    if (body !== undefined) headers.set("Content-Type", "application/json");
    if (idempotencyKey !== undefined) headers.set("Idempotency-Key", idempotencyKey);
    let response: Response;
    try {
      response = await fetcher(`${base}${path}`, {
        method,
        credentials: "include",
        headers,
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      });
    } catch (cause) {
      throw new NotificationApiError(
        cause instanceof Error ? cause.message : "Network request failed",
        0,
      );
    }
    let envelope: Envelope<T>;
    try {
      envelope = (await response.json()) as Envelope<T>;
    } catch (cause) {
      if (cause instanceof TypeError || (cause instanceof DOMException && cause.name === "AbortError")) {
        throw new NotificationApiError(
          cause instanceof Error ? cause.message : "Network response was interrupted",
          0,
          "network_error",
        );
      }
      if (response.ok) return malformed("server returned malformed JSON");
      throw new NotificationApiError(`Request failed (${response.status})`, response.status);
    }
    if (!response.ok) {
      const error =
        typeof envelope?.error === "object" && envelope.error
          ? envelope.error
          : undefined;
      throw new NotificationApiError(
        typeof envelope?.error === "string"
          ? envelope.error
          : (error?.message ?? `Request failed (${response.status})`),
        response.status,
        error?.code,
      );
    }
    if (
      !envelope ||
      typeof envelope !== "object" ||
      Array.isArray(envelope) ||
      envelope.success !== true ||
      envelope.data === undefined
    ) {
      return malformed("successful HTTP response must contain a valid success envelope");
    }
    return envelope.data;
  }

  return {
    async list(limit = 20, offset = 0): Promise<NotificationPage> {
      if (
        !Number.isSafeInteger(limit) ||
        limit < 1 ||
        limit > 100 ||
        !Number.isSafeInteger(offset) ||
        offset < 0 ||
        offset > 1_000_000
      ) {
        throw new NotificationApiError(
          "limit must be 1 to 100 and offset 0 to 1000000",
          0,
          "invalid_parameter",
        );
      }
      const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
      const page = parsePage(
        await request<unknown>(`/notifications?${query}`, "GET"),
        limit,
        offset,
      );
      listedContext = { brand_id: page.brand_id, member_id: page.member_id };
      return page;
    },

    async markRead(
      ids: string[],
      context: { brand_id: string; member_id: string },
      idempotencyKey: string,
    ): Promise<NotificationReadReceipt> {
      if (!Array.isArray(ids) || ids.length < 1 || ids.length > 100) {
        throw new NotificationApiError("ids must contain 1 to 100 UUIDs", 0, "invalid_parameter");
      }
      const validIds = ids.map((id, index) => inputUuid(id, `ids[${index}]`));
      if (new Set(validIds).size !== validIds.length) {
        throw new NotificationApiError("ids must be distinct", 0, "invalid_parameter");
      }
      const validContext = {
        brand_id: inputUuid(context?.brand_id, "context.brand_id"),
        member_id: inputUuid(context?.member_id, "context.member_id"),
      };
      if (
        listedContext &&
        (listedContext.brand_id !== validContext.brand_id ||
          listedContext.member_id !== validContext.member_id)
      ) {
        throw new NotificationApiError(
          "Read context does not match the most recently listed notification scope",
          0,
          "context_mismatch",
        );
      }
      if (typeof idempotencyKey !== "string" || !IDEMPOTENCY_KEY.test(idempotencyKey)) {
        throw new NotificationApiError(
          "idempotencyKey must contain 8 to 128 ASCII letters, digits, underscore, colon, dot or hyphen",
          0,
          "invalid_parameter",
        );
      }
      return parseReceipt(
        await request<unknown>(
          "/notifications/read",
          "POST",
          { ids: validIds },
          idempotencyKey,
        ),
        validIds,
        validContext,
      );
    },
  };
}

export type NotificationApi = ReturnType<typeof createNotificationApi>;
