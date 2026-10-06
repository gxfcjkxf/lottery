import { AdminApiError, type AdminAccount } from "./admin-api";
import type { RuleTicketSelection } from "../../shared/src/rules";

const BASE = "/api/v1/admin";
const MAX_INT64 = 9223372036854775807n;
const MAX_TRACE_NODES = 200_000;
const MAX_TRACE_DEPTH = 32;
const MAX_COMBINATIONS = 10_000;
const MAX_PAGE_SIZE = 100;
const MAX_OFFSET = 1_000_000;
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

export interface SettlementDraw {
  regular: number[];
  special: number[];
  digits: number[];
}

export interface SettlementContext {
  brand_id: string;
  game_id: string;
  period_id: string;
  order_id: string;
  order_version: number;
  order_status: string;
  definition_hash: string;
  period_version: number;
  period_status: string;
  draw_result_id: string | null;
  draw_hash: string | null;
  draw: SettlementDraw | null;
  can_preview: boolean;
}

export interface SettlementSummary {
  won: boolean;
  combination_count: number;
  multiplier: string;
  bet_points: string;
  prize_points: string;
  raw_prize_points: string;
  capped_prize_points: string;
}

export interface SettlementPreview {
  id: string;
  brand_id: string;
  game_id: string;
  period_id: string;
  order_id: string;
  order_version: number;
  order_status: string;
  period_version: number;
  period_status: string;
  draw_result_id: string;
  definition_hash: string;
  draw_hash: string;
  draw: SettlementDraw;
  outcome: "won" | "lost" | "abnormal" | "excluded";
  error_code: string | null;
  calculation: SettlementSummary | null;
  created_by: string;
  created_at: string;
  reason: string;
  audit_log_id: string;
  /** True only when the stored source snapshots still match; never means settled or paid. */
  current: boolean;
  applied: false;
}

export interface SettlementPreviewBody {
  version: number;
  period_version: number;
  draw_result_id: string;
  reason: string;
}

export interface SettlementTrace {
  op: string;
  field?: string;
  actual: number;
  matched: boolean;
  children?: SettlementTrace[];
}

export interface SettlementLineHit {
  code: string;
  exclusive: boolean;
  matched: boolean;
  selected: boolean;
  raw_points: string;
  capped_points: string;
  points: string;
  trace: SettlementTrace;
}

export interface SettlementPreviewLine {
  selection: RuleTicketSelection;
  hits: SettlementLineHit[];
  points: string;
}

export interface SettlementPreviewApi {
  context(brandId: string, orderId: string): Promise<SettlementContext>;
  history(
    brandId: string,
    orderId: string,
    limit?: number,
    offset?: number,
  ): Promise<{
    brand_id: string;
    order_id: string;
    items: SettlementPreview[];
    limit: number;
    offset: number;
    has_more: boolean;
  }>;
  create(
    brandId: string,
    orderId: string,
    body: SettlementPreviewBody,
    key: string,
    expectedAccountId?: string,
  ): Promise<SettlementPreview>;
  preview(brandId: string, previewId: string): Promise<SettlementPreview>;
  lines(
    brandId: string,
    previewId: string,
    limit?: number,
    offset?: number,
  ): Promise<{
    brand_id: string;
    preview_id: string;
    order_id: string;
    items: SettlementPreviewLine[];
    limit: number;
    offset: number;
    has_more: boolean;
    total: number;
  }>;
}

export function settlementPreviewPermissions(
  account: AdminAccount,
  brandId: string,
): { view: boolean; preview: boolean } {
  const member =
    UUID_RE.test(account.id) &&
    UUID_RE.test(brandId) &&
    account.brand_ids.some((id) => id.toLowerCase() === brandId.toLowerCase());
  const brand = new Set(
    account.permissions_by_brand === undefined
      ? account.permissions ?? []
      : account.permissions_by_brand[brandId] ?? [],
  );
  const platform = new Set(account.platform_permissions ?? []);
  return {
    view:
      member &&
      (brand.has("settlement.view.brand") ||
        (account.super_admin && platform.has("settlement.view.platform"))),
    preview:
      member &&
      !account.super_admin &&
      brand.has("settlement.preview.brand"),
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}
function nonempty(value: unknown): value is string {
  return typeof value === "string" && value.length > 0;
}
function validUuid(value: unknown): value is string {
  return typeof value === "string" && UUID_RE.test(value);
}
function positiveVersion(value: unknown): value is number {
  return Number.isSafeInteger(value) && Number(value) > 0;
}
function safeInt(value: unknown): value is number {
  return Number.isSafeInteger(value);
}
function nonnegativeInt(value: unknown): value is number {
  return safeInt(value) && Number(value) >= 0;
}
function validHash(value: unknown): value is string {
  return typeof value === "string" && /^[a-f0-9]{64}$/.test(value);
}
function validIsoDateTime(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|([+-])(\d{2}):(\d{2}))$/.exec(value);
  if (!match) return false;
  const [, yearText, monthText, dayText, hourText, minuteText, secondText, , , , offsetHourText, offsetMinuteText] = match;
  const year = Number(yearText);
  const month = Number(monthText);
  const day = Number(dayText);
  const hour = Number(hourText);
  const minute = Number(minuteText);
  const second = Number(secondText);
  const daysInMonth = month === 2
    ? year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28
    : [4, 6, 9, 11].includes(month) ? 30 : 31;
  if (
    year < 1 || month < 1 || month > 12 || day < 1 || day > daysInMonth ||
    hour > 23 || minute > 59 || second > 59
  ) return false;
  if (offsetHourText !== undefined) {
    const offsetHour = Number(offsetHourText);
    const offsetMinute = Number(offsetMinuteText);
    if (offsetHour > 14 || offsetMinute > 59 || (offsetHour === 14 && offsetMinute !== 0))
      return false;
  }
  return Number.isFinite(Date.parse(value));
}
function validInt64(value: unknown, positive: boolean): value is string {
  if (
    typeof value !== "string" ||
    !(positive ? /^[1-9][0-9]*$/ : /^(0|[1-9][0-9]*)$/).test(value)
  )
    return false;
  try {
    return BigInt(value) <= MAX_INT64;
  } catch {
    return false;
  }
}
function validIntegerString(value: unknown): value is string {
  return typeof value === "string" && /^(0|[1-9][0-9]*)$/.test(value);
}
function parseRational(value: unknown): { numerator: bigint; denominator: bigint } | null {
  if (typeof value !== "string") return null;
  const match = /^(0|[1-9][0-9]*)(?:\/([1-9][0-9]*))?$/.exec(value);
  if (!match) return null;
  try {
    const numerator = BigInt(match[1]);
    const denominator = BigInt(match[2] ?? "1");
    let a = numerator;
    let b = denominator;
    while (b !== 0n) [a, b] = [b, a % b];
    if (a !== 1n || (match[2] !== undefined && denominator === 1n)) return null;
    return { numerator, denominator };
  } catch {
    return null;
  }
}
function validRational(value: unknown): value is string {
  return parseRational(value) !== null;
}
function rationalAtMost(left: string, right: string): boolean {
  const a = parseRational(left);
  const b = parseRational(right);
  return Boolean(a && b && a.numerator * b.denominator <= b.numerator * a.denominator);
}
function roundedHalfUp(value: string): bigint | null {
  const rational = parseRational(value);
  if (!rational) return null;
  return (2n * rational.numerator + rational.denominator) / (2n * rational.denominator);
}
function validNumberArray(value: unknown, maxLength: number, maxValue: number): value is number[] {
  return Array.isArray(value) && value.length <= maxLength &&
    value.every((item) => nonnegativeInt(item) && item <= maxValue);
}
function validDraw(value: unknown): value is SettlementDraw {
  return (
    isRecord(value) &&
    validNumberArray(value.regular, 100, 1_000_000) &&
    validNumberArray(value.special, 100, 1_000_000) &&
    validNumberArray(value.digits, 10, 9)
  );
}
function validContext(
  value: unknown,
  brandId: string,
  orderId: string,
): value is SettlementContext {
  if (
    !isRecord(value) ||
    value.brand_id !== brandId ||
    value.order_id !== orderId ||
    !validUuid(value.game_id) ||
    !validUuid(value.period_id) ||
    !validUuid(value.order_id) ||
    !positiveVersion(value.order_version) ||
    !nonempty(value.order_status) ||
    !validHash(value.definition_hash) ||
    !positiveVersion(value.period_version) ||
    !nonempty(value.period_status) ||
    typeof value.can_preview !== "boolean"
  )
    return false;
  if (value.draw_result_id === null) {
    if (value.draw_hash !== null || value.draw !== null) return false;
  } else if (
    !validUuid(value.draw_result_id) ||
    !validHash(value.draw_hash) ||
    !validDraw(value.draw)
  ) {
    return false;
  }
  return (
    !value.can_preview ||
    (value.period_status === "drawn" && value.draw_result_id !== null)
  );
}
function validSummary(value: unknown): value is SettlementSummary {
  if (
    isRecord(value) &&
    typeof value.won === "boolean" &&
    Number.isSafeInteger(value.combination_count) &&
    Number(value.combination_count) > 0 && Number(value.combination_count) <= MAX_COMBINATIONS &&
    validInt64(value.multiplier, true) &&
    validInt64(value.bet_points, true) &&
    validInt64(value.prize_points, false) &&
    validRational(value.raw_prize_points) &&
    validRational(value.capped_prize_points) &&
    rationalAtMost(value.capped_prize_points, value.raw_prize_points)
  ) {
    const rounded = roundedHalfUp(value.capped_prize_points as string);
    return rounded !== null && BigInt(value.prize_points as string) === rounded;
  }
  return false;
}
function validPreview(
  value: unknown,
  brandId: string,
  expected?: { orderId?: string; previewId?: string; body?: SettlementPreviewBody; accountId?: string },
): value is SettlementPreview {
  if (
    !isRecord(value) ||
    !validUuid(value.id) ||
    (expected?.previewId !== undefined && value.id !== expected.previewId) ||
    value.brand_id !== brandId ||
    !validUuid(value.game_id) ||
    !validUuid(value.period_id) ||
    !validUuid(value.order_id) ||
    (expected?.orderId !== undefined && value.order_id !== expected.orderId) ||
    !positiveVersion(value.order_version) ||
    !nonempty(value.order_status) ||
    !positiveVersion(value.period_version) ||
    !nonempty(value.period_status) ||
    !validUuid(value.draw_result_id) ||
    !validHash(value.definition_hash) ||
    !validHash(value.draw_hash) ||
    !validDraw(value.draw) ||
    !["won", "lost", "abnormal", "excluded"].includes(String(value.outcome)) ||
    !validUuid(value.created_by) ||
    (expected?.accountId !== undefined && value.created_by !== expected.accountId) ||
    !validIsoDateTime(value.created_at) ||
    !nonempty(value.reason) ||
    !validUuid(value.audit_log_id) ||
    typeof value.current !== "boolean" ||
    value.applied !== false
  )
    return false;
  if (expected?.body) {
    if (
      value.order_version !== expected.body.version ||
      value.period_version !== expected.body.period_version ||
      value.draw_result_id !== expected.body.draw_result_id ||
      value.reason !== expected.body.reason
    )
      return false;
  }
  if (value.outcome === "won" || value.outcome === "lost") {
    return (
      value.order_status === "placed" &&
      value.period_status === "drawn" &&
      value.error_code === null &&
      validSummary(value.calculation) &&
      value.calculation.won === (value.outcome === "won") &&
      (value.outcome !== "lost" || value.calculation.prize_points === "0")
    );
  }
  if (value.calculation !== null || !nonempty(value.error_code)) return false;
  return value.outcome === "abnormal"
    ? value.order_status === "placed"
    : value.order_status !== "placed";
}
function validSelection(value: unknown): value is RuleTicketSelection {
  if (!isRecord(value)) return false;
  const selectionNumbers = (item: unknown) =>
    item === null || validNumberArray(item, MAX_COMBINATIONS, 1_000_000);
  const digits = (item: unknown) =>
    item === null ||
    (Array.isArray(item) && item.length <= 10 && item.every((entry) => validNumberArray(entry, 10, 9)));
  const stringMap = (item: unknown) =>
    item === null ||
    (isRecord(item) && Object.values(item).every((entry) =>
      Array.isArray(entry) && entry.every((x) => typeof x === "string"),
    ));
  const numberMap = (item: unknown) =>
    item === null ||
    (isRecord(item) && Object.values(item).every((entry) => selectionNumbers(entry)));
  return (
    selectionNumbers(value.regular) &&
    selectionNumbers(value.special) &&
    digits(value.digits) &&
    selectionNumbers(value.exclude) &&
    stringMap(value.attributes) &&
    numberMap(value.features)
  );
}
function validTrace(
  value: unknown,
  budget: { count: number },
  depth = 1,
): value is SettlementTrace {
  budget.count += 1;
  if (
    budget.count > MAX_TRACE_NODES ||
    depth > MAX_TRACE_DEPTH ||
    !isRecord(value) ||
    !nonempty(value.op) ||
    (value.field !== undefined && typeof value.field !== "string") ||
    !safeInt(value.actual) ||
    typeof value.matched !== "boolean"
  )
    return false;
  if (value.children === undefined) return true;
  return (
    Array.isArray(value.children) &&
    value.children.every((child) => validTrace(child, budget, depth + 1))
  );
}
function validLine(value: unknown, budget: { count: number }): value is SettlementPreviewLine {
  if (
    !isRecord(value) ||
    !validSelection(value.selection) ||
    !Array.isArray(value.hits) ||
    !validIntegerString(value.points)
  )
    return false;
  for (const hit of value.hits) {
    budget.count += 1;
    if (
      budget.count > MAX_TRACE_NODES ||
      !isRecord(hit) ||
      !nonempty(hit.code) ||
      typeof hit.exclusive !== "boolean" ||
      typeof hit.matched !== "boolean" ||
      typeof hit.selected !== "boolean" ||
      (hit.selected && !hit.matched) ||
      !validRational(hit.raw_points) ||
      !validRational(hit.capped_points) ||
      !rationalAtMost(hit.capped_points as string, hit.raw_points as string) ||
      !validIntegerString(hit.points) ||
      !validTrace(hit.trace, budget)
    )
      return false;
  }
  return true;
}
function invalidResponse(): never {
  throw new AdminApiError("核算预览响应格式无效。", 502, "INVALID_RESPONSE");
}
function invalidInput(message: string): never {
  throw new AdminApiError(message, 0, "INVALID_INPUT");
}
function validatePage(limit: number, offset: number): void {
  if (
    !Number.isSafeInteger(limit) ||
    limit < 1 ||
    limit > MAX_PAGE_SIZE ||
    !Number.isSafeInteger(offset) ||
    offset < 0 ||
    offset > MAX_OFFSET
  )
    invalidInput("分页参数超出允许范围。");
}

type Envelope = {
  success?: unknown;
  data?: unknown;
  error?: string | { code?: string; message?: string } | null;
};

export function createSettlementPreviewApi({ fetch: fetcher = fetch }: { fetch?: typeof fetch } = {}): SettlementPreviewApi {
  async function request(
    brandId: string,
    path: string,
    options: {
      method?: "GET" | "POST";
      body?: unknown;
      key?: string;
      successStatus?: number;
    } = {},
  ): Promise<unknown> {
    if (!validUuid(brandId)) invalidInput("品牌编号必须是 UUID。");
    const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brandId });
    if (options.body !== undefined) headers.set("Content-Type", "application/json");
    if (options.key !== undefined) headers.set("Idempotency-Key", options.key);
    let response: Response;
    try {
      response = await fetcher(path, {
        method: options.method ?? "GET",
        credentials: "same-origin",
        headers,
        ...(options.body === undefined ? {} : { body: JSON.stringify(options.body) }),
      });
    } catch (cause) {
      throw new AdminApiError(
        cause instanceof Error ? cause.message : "Network request failed",
        0,
        "NETWORK_ERROR",
      );
    }
    if (response.ok && response.status !== (options.successStatus ?? 200))
      invalidResponse();
    let envelope: Envelope;
    try {
      envelope = (await response.json()) as Envelope;
    } catch {
      throw new AdminApiError(
        response.ok ? "Invalid server response" : `Request failed (${response.status})`,
        response.ok ? 502 : response.status,
        response.ok ? "INVALID_RESPONSE" : undefined,
      );
    }
    if (!response.ok || envelope.success !== true || envelope.data === undefined) {
      const error = typeof envelope.error === "object" && envelope.error ? envelope.error : undefined;
      throw new AdminApiError(
        typeof envelope.error === "string" ? envelope.error : error?.message || `Request failed (${response.status})`,
        response.ok ? 502 : response.status,
        response.ok ? "INVALID_RESPONSE" : error?.code,
      );
    }
    return envelope.data;
  }

  return {
    async context(brandId, orderId) {
      if (!validUuid(orderId)) invalidInput("投注单编号必须是 UUID。");
      const value = await request(brandId, `${BASE}/bet-orders/${encodeURIComponent(orderId)}/settlement-context`);
      if (!validContext(value, brandId, orderId)) invalidResponse();
      return value;
    },
    async history(brandId, orderId, limit = 20, offset = 0) {
      validatePage(limit, offset);
      if (!validUuid(orderId)) invalidInput("投注单编号必须是 UUID。");
      const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
      const value = await request(brandId, `${BASE}/bet-orders/${encodeURIComponent(orderId)}/settlement-previews?${query}`);
      if (
        !isRecord(value) || value.brand_id !== brandId || value.order_id !== orderId ||
        !Array.isArray(value.items) || value.items.length > limit || value.limit !== limit ||
        value.offset !== offset || typeof value.has_more !== "boolean" ||
        (value.has_more && value.items.length !== limit) ||
        new Set(value.items.flatMap((item) => isRecord(item) && validUuid(item.id) ? [item.id] : [])).size !== value.items.length ||
        !value.items.every((item) => validPreview(item, brandId, { orderId }))
      ) invalidResponse();
      return value as unknown as Awaited<ReturnType<SettlementPreviewApi["history"]>>;
    },
    async create(brandId, orderId, body, key, expectedAccountId) {
      if (!validUuid(orderId) || !nonempty(key) || !nonempty(body.reason) ||
        !positiveVersion(body.version) || !positiveVersion(body.period_version) || !validUuid(body.draw_result_id) ||
        (expectedAccountId !== undefined && !validUuid(expectedAccountId)))
        invalidInput("核算预览请求参数无效。");
      const value = await request(brandId, `${BASE}/bet-orders/${encodeURIComponent(orderId)}/settlement-previews`, {
        method: "POST", body, key, successStatus: 201,
      });
      if (!validPreview(value, brandId, { orderId, body, accountId: expectedAccountId })) invalidResponse();
      return value;
    },
    async preview(brandId, previewId) {
      if (!validUuid(previewId)) invalidInput("核算预览编号必须是 UUID。");
      const value = await request(brandId, `${BASE}/settlement-previews/${encodeURIComponent(previewId)}`);
      if (!validPreview(value, brandId, { previewId })) invalidResponse();
      return value;
    },
    async lines(brandId, previewId, limit = 20, offset = 0) {
      validatePage(limit, offset);
      if (!validUuid(previewId)) invalidInput("核算预览编号必须是 UUID。");
      const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
      const value = await request(brandId, `${BASE}/settlement-previews/${encodeURIComponent(previewId)}/lines?${query}`);
      const budget = { count: 0 };
      if (
        !isRecord(value) || value.brand_id !== brandId || value.preview_id !== previewId ||
        !validUuid(value.order_id) || !Array.isArray(value.items) || value.items.length > limit ||
        value.limit !== limit || value.offset !== offset || typeof value.has_more !== "boolean" ||
        !nonnegativeInt(value.total) || value.total > MAX_COMBINATIONS || value.items.length > value.total ||
        value.has_more !== (offset + value.items.length < value.total) ||
        !value.items.every((line) => validLine(line, budget))
      ) invalidResponse();
      return value as unknown as Awaited<ReturnType<SettlementPreviewApi["lines"]>>;
    },
  };
}
