import { walletBasePath } from "./wallet-api";

export type NotificationEventType =
  | "member.joined"
  | "recharge.confirmed"
  | "bet.order.placed"
  | "bet.order.cancelled"
  | "bet.order.judged_cancelled"
  | "bet.order.abnormal"
  | "bet.order.won"
  | "bet.order.prize_reversed"
  | "reward.order.granted"
  | "reward.order.revocation_pending"
  | "reward.order.revoked"
  | "commission.paid"
  | "commission.adjusted"
  | "commission.corrected"
  | "draw.result.published"
  | "draw.result.corrected"
  | "withdrawal.order.reviewing"
  | "withdrawal.order.processing"
  | "withdrawal.order.paid"
  | "withdrawal.order.rejected"
  | "withdrawal.order.failed"
  | "withdrawal.order.cancelled";

export interface NotificationTemplateContent {
  en: { title: string; body: string };
  "zh-CN": { title: string; body: string };
}

export interface NotificationItem {
  id: string;
  brand_id: string;
  member_id: string;
  event_type: NotificationEventType;
  template_key: NotificationEventType;
  template_version: number;
  content: NotificationTemplateContent | null;
  payload: LegacyNotificationPayload | DrawNotificationPayload;
  created_at: string;
  read_at: string | null;
}

export interface LegacyNotificationPayload { resource_id: string; points: string | null }
export interface DrawNotificationPayload {
  resource_id: string;
  points: null;
  draw: {
    game_id: string;
    period_id: string;
    period_no: string;
    result: { regular: number[]; special: number[]; digits: number[] };
    drawn_at: string;
    previous_draw_id: string | null;
  };
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
const SIGNED_NONZERO_INT64 = /^(?:[1-9]\d*|-[1-9]\d*)$/;
const MAX_INT64 = "9223372036854775807";
const MAX_NEGATIVE_INT64 = "9223372036854775808";
const MAX_SAFE_INTEGER = 9007199254740991;
const IDEMPOTENCY_KEY = /^[A-Za-z0-9_:.-]{8,128}$/;
const TEMPLATE_PLACEHOLDERS = new Set(["points", "resource_id"]);
const EVENT_TYPES = new Set<NotificationEventType>([
  "member.joined",
  "recharge.confirmed",
  "bet.order.placed",
  "bet.order.cancelled",
  "bet.order.judged_cancelled",
  "bet.order.abnormal",
  "bet.order.won",
  "bet.order.prize_reversed",
  "reward.order.granted",
  "reward.order.revocation_pending",
  "reward.order.revoked",
  "commission.paid",
  "commission.adjusted",
  "commission.corrected",
  "draw.result.published",
  "draw.result.corrected",
  "withdrawal.order.reviewing",
  "withdrawal.order.processing",
  "withdrawal.order.paid",
  "withdrawal.order.rejected",
  "withdrawal.order.failed",
  "withdrawal.order.cancelled",
]);
const WITHDRAWAL_EVENT_TYPES = new Set<NotificationEventType>([
  "withdrawal.order.reviewing",
  "withdrawal.order.processing",
  "withdrawal.order.paid",
  "withdrawal.order.rejected",
  "withdrawal.order.failed",
  "withdrawal.order.cancelled",
]);
const DRAW_EVENT_TYPES = new Set<NotificationEventType>([
  "draw.result.published",
  "draw.result.corrected",
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

function exactKeys(value: Record<string, unknown>, keys: string[], label: string): void {
  const actual = Object.keys(value).sort();
  const expected = [...keys].sort();
  if (actual.length !== expected.length || actual.some((key, index) => key !== expected[index])) {
    malformed(`${label} must contain exactly ${keys.join(", ")}`);
  }
}

function safeTemplateText(
  value: unknown,
  label: string,
  maxLength: number,
  byteLength: boolean,
  allowLineBreaks: boolean,
): string {
  if (typeof value !== "string" || value.trim() !== value || value.length === 0) {
    return malformed(`${label} must be a nonempty trimmed string`);
  }
  const length = byteLength ? new TextEncoder().encode(value).length : [...value].length;
  if (length > maxLength) return malformed(`${label} is too long`);
  if (/[<>\uD800-\uDFFF]|https?:|javascript:|data:|www\./iu.test(value)) {
    return malformed(`${label} contains disallowed markup or a URL`);
  }
  const controls = allowLineBreaks ? /[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f-\u009f]/ : /[\u0000-\u001f\u007f-\u009f]/;
  if (controls.test(value)) return malformed(`${label} contains a control character`);
  // Reject lone UTF-16 surrogates instead of silently replacing them during UTF-8 sizing.
  if (/[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/.test(value)) {
    return malformed(`${label} contains invalid Unicode`);
  }
  const withoutPlaceholders = value.replace(/\{(points|resource_id)\}/g, "");
  if (/[{}]/.test(withoutPlaceholders)) return malformed(`${label} contains an unsupported placeholder`);
  for (const match of value.matchAll(/\{([^{}]*)\}/g)) {
    if (!TEMPLATE_PLACEHOLDERS.has(match[1])) return malformed(`${label} contains an unsupported placeholder`);
  }
  return value;
}

function parseTemplateContent(value: unknown, eventType: NotificationEventType): NotificationTemplateContent {
  const content = object(value, "notification.content");
  exactKeys(content, ["en", "zh-CN"], "notification.content");
  const parsed = {} as NotificationTemplateContent;
  for (const locale of ["en", "zh-CN"] as const) {
    const localized = object(content[locale], `notification.content.${locale}`);
    exactKeys(localized, ["title", "body"], `notification.content.${locale}`);
    const title = safeTemplateText(localized.title, `notification.content.${locale}.title`, 120, true, false);
    const body = safeTemplateText(localized.body, `notification.content.${locale}.body`, 1200, true, true);
    if (DRAW_EVENT_TYPES.has(eventType)) {
      if (title.includes("{points}") || body.includes("{points}")) {
        return malformed(`${eventType} content cannot use {points}`);
      }
      if (!body.includes("{resource_id}")) {
        return malformed(`${eventType} content body must use {resource_id}`);
      }
    } else if (eventType === "member.joined") {
      if (title.includes("{points}") || body.includes("{points}")) {
        return malformed("member.joined content cannot use {points}");
      }
    } else if (!body.includes("{points}")) {
      return malformed(`${eventType} content body must use {points}`);
    }
    parsed[locale] = { title, body };
  }
  return parsed;
}

function inputUuid(value: unknown, label: string): string {
  if (typeof value !== "string" || !UUID.test(value)) {
    throw new NotificationApiError(`${label} must be a UUID`, 0, "invalid_parameter");
  }
  return value;
}

function parseDrawPayload(value: Record<string, unknown>, eventType: NotificationEventType, resourceId: string): DrawNotificationPayload {
  exactKeys(value, ["resource_id", "points", "draw"], "notification.payload");
  if (value.points !== null) return malformed("draw notification points must be null");
  const draw = object(value.draw, "notification.payload.draw");
  exactKeys(draw, ["game_id", "period_id", "period_no", "result", "drawn_at", "previous_draw_id"], "notification.payload.draw");
  const gameId = uuid(draw.game_id, "notification.payload.draw.game_id");
  const periodId = uuid(draw.period_id, "notification.payload.draw.period_id");
  if (typeof draw.period_no !== "string" || draw.period_no.trim().length === 0 || new TextEncoder().encode(draw.period_no).length > 80) {
    return malformed("notification.payload.draw.period_no must be a nonempty string of at most 80 UTF-8 bytes");
  }
  const result = object(draw.result, "notification.payload.draw.result");
  exactKeys(result, ["regular", "special", "digits"], "notification.payload.draw.result");
  const parseNumbers = (input: unknown, label: string, maximum: number): number[] => {
    if (!Array.isArray(input) || input.length > 10 || !input.every((n) => Number.isSafeInteger(n) && n >= 0 && n <= maximum)) {
      return malformed(`${label} must contain at most 10 integers from 0 to ${maximum}`);
    }
    return input as number[];
  };
  const regular = parseNumbers(result.regular, "notification.payload.draw.result.regular", 1_000_000);
  const special = parseNumbers(result.special, "notification.payload.draw.result.special", 1_000_000);
  const digits = parseNumbers(result.digits, "notification.payload.draw.result.digits", 9);
  if (!(regular.length || special.length || digits.length) || (digits.length > 0 && (regular.length > 0 || special.length > 0))) {
    return malformed("notification.payload.draw.result must be nonempty and match one supported number shape");
  }
  const drawnAt = dateTime(draw.drawn_at, "notification.payload.draw.drawn_at");
  const previousDrawId = draw.previous_draw_id === null ? null : uuid(draw.previous_draw_id, "notification.payload.draw.previous_draw_id");
  if (eventType === "draw.result.published" ? previousDrawId !== null : previousDrawId === null || previousDrawId === resourceId) {
    return malformed(`${eventType} has an invalid previous_draw_id`);
  }
  return {
    resource_id: resourceId,
    points: null,
    draw: { game_id: gameId, period_id: periodId, period_no: draw.period_no, result: { regular, special, digits }, drawn_at: drawnAt, previous_draw_id: previousDrawId },
  };
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
  if (
    typeof item.template_version !== "number" ||
    !Number.isSafeInteger(item.template_version) ||
    item.template_version < 1 ||
    item.template_version > MAX_SAFE_INTEGER
  ) {
    return malformed("notification.template_version must be a positive safe integer");
  }
  const templateVersion = item.template_version;
  const content = item.content === undefined || item.content === null
    ? null
    : parseTemplateContent(item.content, eventType);
  if (DRAW_EVENT_TYPES.has(eventType) && content === null) {
    return malformed("draw notifications require an immutable content snapshot");
  }
  const rewardEvent = eventType.startsWith("reward.order.");
  if (content === null && (WITHDRAWAL_EVENT_TYPES.has(eventType) || eventType.startsWith("commission.") || rewardEvent)) {
    return malformed("withdrawal, commission, and reward notifications require an immutable content snapshot");
  }
  if (content === null && templateVersion !== 1) {
    return malformed("notification.content is required for template versions above 1");
  }
  const payload = object(item.payload, "notification.payload");
  const resourceId = uuid(payload.resource_id, "notification.payload.resource_id");
  if (DRAW_EVENT_TYPES.has(eventType)) {
    return {
      id, brand_id: brandId, member_id: memberId, event_type: eventType, template_key: eventType,
      template_version: templateVersion, content,
      payload: parseDrawPayload(payload, eventType, resourceId),
      created_at: dateTime(item.created_at, "notification.created_at"),
      read_at: item.read_at === null ? null : dateTime(item.read_at, "notification.read_at"),
    };
  }
  exactKeys(payload, ["resource_id", "points"], "notification.payload");
  let points: string | null;
  if (eventType === "member.joined") {
    if (payload.points !== null || resourceId !== memberId) {
      return malformed("member.joined must reference its member and have null points");
    }
    points = null;
  } else {
    if (eventType === "commission.adjusted" || eventType === "commission.corrected") {
      if (typeof payload.points !== "string" || !SIGNED_NONZERO_INT64.test(payload.points)) {
        return malformed(`${eventType} points must be a canonical signed nonzero int64 string`);
      }
      const digits = payload.points.startsWith("-") ? payload.points.slice(1) : payload.points;
      const limit = payload.points.startsWith("-") ? MAX_NEGATIVE_INT64 : MAX_INT64;
      if (digits.length > limit.length || (digits.length === limit.length && digits > limit)) {
        return malformed(`${eventType} points exceed int64 range`);
      }
      points = payload.points;
    } else {
      points = int64(payload.points, "notification.payload.points", true);
    }
  }
  return {
    id,
    brand_id: brandId,
    member_id: memberId,
    event_type: eventType,
    template_key: eventType,
    template_version: templateVersion,
    content,
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
