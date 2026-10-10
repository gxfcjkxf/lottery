import {
  AdminApiError,
  createIdempotencyKey,
  type AdminAccount,
} from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";
import { parsePeriod, type Period } from "./period-schedules-api";
import type { RuleDraw, RuleModel } from "./rule-simulation-api";

const BASE = "/api/v1/admin";

export interface DrawGame {
  id: string;
  brand_id: string;
  code: string;
  name: string;
  model: RuleModel;
  timezone: string;
  version: number;
  status: string;
  [key: string]: unknown;
}

export type DrawSourceType = "api" | "dom";
export interface SourceConfig {
  id: string;
  name: string;
  type: DrawSourceType;
  priority: number;
  enabled: boolean;
  endpoint: string;
  selector?: string;
  credential_ref?: string;
}
export interface SourceSet {
  id: string;
  brand_id: string;
  game_id: string;
  revision: number;
  game_version: number;
  created_at: string;
  sources: SourceConfig[];
}
export interface DrawResult {
  id: string;
  brand_id: string;
  game_id: string;
  period_id: string;
  source_id: string;
  kind: "api" | "dom" | "manual";
  result: RuleDraw;
  result_hash: string;
  drawn_at: string;
  created_at: string;
  created_by?: string;
  corrected_from_id?: string;
}
export interface DrawAttempt {
  source_id: string;
  status: string;
  code: string;
}
export interface AttemptBatch {
  id: string;
  brand_id: string;
  game_id: string;
  period_id: string;
  source_set_id: string;
  observed_period_version: number;
  status: "no_data" | "accepted" | "discarded" | "failed";
  attempts: DrawAttempt[];
  created_at: string;
}
export interface DrawHistoryPage {
  current: DrawResult | null;
  history: DrawResult[];
  attempts: AttemptBatch[];
  limit: number;
  offset: number;
}
export interface ManualDrawBody {
  version: number;
  period_no: string;
  result: RuleDraw;
  drawn_at: string;
  reason: string;
}

export interface DrawManagementPermissions {
  gamesView: boolean;
  periodsView: boolean;
  sourceView: boolean;
  sourceWrite: boolean;
  drawView: boolean;
  manualCreate: boolean;
}

export function drawManagementPermissions(
  account: AdminAccount,
  brandId: string,
): DrawManagementPermissions {
  const brand = brandPermissionSet(account, brandId);
  const view = (name: string) => Boolean(brandId) && brand.has(`${name}.view.brand`);
  return {
    gamesView: view("game"),
    periodsView: view("period"),
    sourceView: view("draw_source"),
    sourceWrite:
      Boolean(brandId) &&
      brand.has("draw_source.write.brand"),
    drawView: view("draw"),
    manualCreate:
      Boolean(brandId) &&
      brand.has("draw.manual_create.brand"),
  };
}

/** Each lane invalidates only its own older response; scope binds it to permissions and selection. */
export function createDrawManagementRequestGuard() {
  const generations = new Map<string, number>();
  type Ticket = { lane: string; generation: number; scope: string };
  return {
    capture(lane: string, scope: string): Ticket {
      const generation = (generations.get(lane) ?? 0) + 1;
      generations.set(lane, generation);
      return { lane, generation, scope };
    },
    invalidate(lane?: string) {
      if (lane) generations.set(lane, (generations.get(lane) ?? 0) + 1);
      else
        for (const name of generations.keys())
          generations.set(name, generations.get(name)! + 1);
    },
    isCurrent(ticket: Ticket, scope: string, permitted: boolean) {
      return (
        permitted &&
        ticket.scope === scope &&
        generations.get(ticket.lane) === ticket.generation
      );
    },
  };
}

/** Reuses a key only for an identical body and brand/operation/target. */
export function createDrawManagementKeyTracker() {
  const entries = new Map<string, { body: string; key: string }>();
  return (
    brandId: string,
    operation: string,
    targetId: string,
    body: unknown,
  ) => {
    const scope = JSON.stringify([brandId, operation, targetId]);
    const serialized = stableJson(body);
    const previous = entries.get(scope);
    if (previous?.body === serialized) return previous.key;
    const key = createIdempotencyKey();
    entries.set(scope, { body: serialized, key });
    return key;
  };
}

function fail(message: string): never {
  throw new AdminApiError(message, 0, "INVALID_INPUT");
}
function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}
function nonempty(value: unknown): value is string {
  return typeof value === "string" && value.length > 0;
}
function safeInt(value: unknown, minimum = 0): value is number {
  return Number.isSafeInteger(value) && Number(value) >= minimum;
}
function invalidResponse(message: string): never {
  throw new AdminApiError(message, 502, "INVALID_RESPONSE");
}
function isTimezone(value: unknown): value is string {
  if (typeof value !== "string" || !value || value.trim() !== value)
    return false;
  try {
    new Intl.DateTimeFormat("en", { timeZone: value });
    return true;
  } catch {
    return false;
  }
}
function parsePool(value: unknown): RuleModel["regular_pool"] | null {
  if (!isRecord(value) || typeof value.allow_repeat !== "boolean") return null;
  const values = value.values === undefined ? [] : value.values;
  if (!Array.isArray(values) || values.some((item) => !safeInt(item)))
    return null;
  const suppliedMin = value.min;
  const suppliedMax = value.max;
  if (suppliedMin !== undefined && !safeInt(suppliedMin)) return null;
  if (suppliedMax !== undefined && !safeInt(suppliedMax)) return null;
  const min =
    suppliedMin === undefined
      ? values.length
        ? Math.min(...values)
        : 0
      : (suppliedMin as number);
  const max =
    suppliedMax === undefined
      ? values.length
        ? Math.max(...values)
        : 0
      : (suppliedMax as number);
  if (
    max < min ||
    values.some((item) => item < min || item > max) ||
    new Set(values).size !== values.length
  )
    return null;
  return { min, max, values: [...values], allow_repeat: value.allow_repeat };
}
function parseRuleModel(value: unknown): RuleModel {
  if (
    !isRecord(value) ||
    !["X_PLUS_Y", "M_SELECT_N", "DIGITS_0_9"].includes(String(value.model)) ||
    typeof value.allow_repeat !== "boolean" ||
    typeof value.ordered !== "boolean"
  )
    invalidResponse("彩种规则模型响应格式无效。");
  const model = value.model as RuleModel["model"];
  const emptyPool = { min: 0, max: 0, values: [], allow_repeat: false };
  const regularPool = parsePool(
    value.regular_pool ?? (model === "DIGITS_0_9" ? emptyPool : null),
  );
  const specialPool = parsePool(
    value.special_pool ?? (model === "DIGITS_0_9" ? emptyPool : null),
  );
  if (!regularPool || !specialPool) invalidResponse("彩种号码池响应格式无效。");
  const readCount = (field: string, optional: boolean) => {
    const result = value[field];
    if (result === undefined && optional) return 0;
    if (!safeInt(result)) invalidResponse("彩种计数响应格式无效。");
    return result;
  };
  const regularCount = readCount("regular_count", model === "DIGITS_0_9");
  const specialCount = readCount("special_count", model === "DIGITS_0_9");
  const poolSize = readCount("pool_size", true);
  const totalCount = readCount("total_count", true);
  const length = readCount("length", model !== "DIGITS_0_9");
  const poolSizeOf = (pool: RuleModel["regular_pool"]) =>
    pool.values.length || pool.max - pool.min + 1;
  const regularSize = poolSizeOf(regularPool);
  const specialSize = poolSizeOf(specialPool);
  const malformed =
    model === "DIGITS_0_9"
      ? regularCount !== 0 ||
        specialCount !== 0 ||
        poolSize !== 0 ||
        totalCount !== 0 ||
        length < 1 ||
        length > 10 ||
        !value.ordered ||
        regularSize !== 1 ||
        specialSize !== 1 ||
        regularPool.min !== 0 ||
        regularPool.max !== 0 ||
        specialPool.min !== 0 ||
        specialPool.max !== 0
      : model === "X_PLUS_Y"
        ? regularCount + specialCount < 1 ||
          length !== 0 ||
          poolSize !== 0 ||
          totalCount !== 0 ||
          value.allow_repeat ||
          (regularCount > 0 && regularSize === 0) ||
          (specialCount > 0 && specialSize === 0) ||
          (!regularPool.allow_repeat && regularCount > regularSize) ||
          (!specialPool.allow_repeat && specialCount > specialSize)
        : poolSize < 1 ||
          poolSize > 10000 ||
          totalCount < 1 ||
          totalCount >= poolSize ||
          regularCount + specialCount !== totalCount ||
          length !== 0 ||
          value.allow_repeat ||
          regularPool.allow_repeat ||
          specialPool.allow_repeat ||
          regularSize !== poolSize ||
          specialSize !== poolSize ||
          !sameNumberSet(
            allowedPoolValues(regularPool),
            allowedPoolValues(specialPool),
          );
  if (malformed) invalidResponse("彩种模型计数与类型不匹配。");
  return {
    model,
    regular_pool: regularPool,
    special_pool: specialPool,
    regular_count: regularCount,
    special_count: specialCount,
    pool_size: poolSize,
    total_count: totalCount,
    length,
    allow_repeat: value.allow_repeat,
    ordered: value.ordered,
  };
}
function isUuid(value: unknown): value is string {
  return (
    typeof value === "string" &&
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
      value,
    )
  );
}
function sameNumberSet(left: number[], right: number[]) {
  const leftSet = new Set(left);
  const rightSet = new Set(right);
  return (
    left.length === right.length &&
    leftSet.size === rightSet.size &&
    [...leftSet].every((value) => rightSet.has(value))
  );
}
function allowedPoolValues(pool: RuleModel["regular_pool"]): number[] {
  if (pool.values.length) return pool.values;
  if (pool.max - pool.min > 10000) fail("彩种号码池超出输入上限。");
  return Array.from(
    { length: pool.max - pool.min + 1 },
    (_, index) => pool.min + index,
  );
}
export function parseDrawGame(value: unknown): DrawGame {
  if (
    !isRecord(value) ||
    !nonempty(value.id) ||
    !nonempty(value.brand_id) ||
    !nonempty(value.code) ||
    !nonempty(value.name) ||
    !isTimezone(value.timezone) ||
    !safeInt(value.version, 1) ||
    !nonempty(value.status)
  )
    invalidResponse("彩种响应格式无效。");
  const model = parseRuleModel(value.model);
  return { ...value, model } as DrawGame;
}
function parseIsoTimestamp(value: unknown, field: string): string {
  if (
    typeof value !== "string" ||
    !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value) ||
    !Number.isFinite(Date.parse(`${value.slice(0, 19)}.000Z`)) ||
    new Date(`${value.slice(0, 19)}.000Z`).toISOString().slice(0, 19) !==
      value.slice(0, 19)
  )
    invalidResponse(`响应中的 ${field} 时间格式无效。`);
  return value;
}
function isUtcIsoTimestamp(value: unknown): value is string {
  if (
    typeof value !== "string" ||
    !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value)
  )
    return false;
  const base = `${value.slice(0, 19)}.000Z`;
  return (
    Number.isFinite(Date.parse(base)) &&
    new Date(base).toISOString().slice(0, 19) === value.slice(0, 19)
  );
}
function utf8Length(value: string): number {
  return new TextEncoder().encode(value).length;
}
function parseSource(value: unknown): SourceConfig {
  if (
    !isRecord(value) ||
    !isUuid(value.id) ||
    !nonempty(value.name) ||
    (value.type !== "api" && value.type !== "dom") ||
    !safeInt(value.priority, 1) ||
    typeof value.enabled !== "boolean" ||
    !nonempty(value.endpoint) ||
    (value.selector !== undefined && typeof value.selector !== "string") ||
    (value.credential_ref !== undefined &&
      typeof value.credential_ref !== "string")
  )
    invalidResponse("开奖来源响应格式无效。");
  try {
    return validateSource(value as unknown as SourceConfig);
  } catch {
    return invalidResponse("开奖来源响应不符合来源配置限制。");
  }
}
function parseSourceSet(value: unknown): SourceSet {
  if (
    !isRecord(value) ||
    !isUuid(value.id) ||
    !nonempty(value.brand_id) ||
    !nonempty(value.game_id) ||
    !safeInt(value.revision, 1) ||
    !safeInt(value.game_version, 1) ||
    !Array.isArray(value.sources) ||
    value.sources.length > 16
  )
    invalidResponse("开奖来源集响应格式无效。");
  const parsedSources = value.sources.map(parseSource);
  if (
    new Set(parsedSources.map((source) => source.id)).size !==
      parsedSources.length ||
    new Set(parsedSources.map((source) => source.priority)).size !==
      parsedSources.length
  )
    invalidResponse("开奖来源集包含重复 ID 或优先级。");
  return {
    ...(value as unknown as SourceSet),
    created_at: parseIsoTimestamp(value.created_at, "created_at"),
    sources: parsedSources,
  };
}
function validateEndpoint(value: string) {
  if (
    !value ||
    utf8Length(value) > 2048 ||
    value.includes("?") ||
    value.includes("#")
  )
    return false;
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    return false;
  }
  const host = url.hostname.replace(/^\[|\]$/g, "");
  const dnsLabel = /^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$/;
  const hostLower = host.toLowerCase();
  const reservedSuffix = [
    ".localhost",
    ".local",
    ".internal",
    ".test",
    ".onion",
    ".invalid",
    ".example",
  ];
  return (
    url.protocol === "https:" &&
    !url.username &&
    !url.password &&
    !url.search &&
    !url.hash &&
    !url.port &&
    host.length <= 253 &&
    host.includes(".") &&
    !host.includes(":") &&
    !/^\d+(?:\.\d+){0,3}$/.test(host) &&
    host.split(".").every((label) => dnsLabel.test(label)) &&
    !reservedSuffix.some((suffix) => hostLower.endsWith(suffix)) &&
    value.trim() === value
  );
}
function validateSource(source: SourceConfig): SourceConfig {
  if (!source || typeof source !== "object" || !isRecord(source))
    fail("开奖来源格式无效。");
  if (!isUuid(source.id)) fail("来源 ID 必须为 UUID。");
  if (!source.name.trim() || source.name.includes("\0"))
    fail("来源名称必填且不能包含空字符。");
  if (utf8Length(source.name) > 120)
    fail("来源名称不能超过 120 个 UTF-8 字节。");
  if (source.type !== "api" && source.type !== "dom") fail("来源类型无效。");
  if (!safeInt(source.priority, 1) || source.priority > 10000)
    fail("来源优先级必须为 1 至 10000 的整数。");
  if (typeof source.enabled !== "boolean") fail("来源启用状态无效。");
  if (!validateEndpoint(source.endpoint))
    fail("来源地址必须为不含凭据、查询、片段或 IP 地址的 HTTPS URL。");
  if (
    source.selector !== undefined &&
    (source.selector.includes("\0") || utf8Length(source.selector) > 500)
  )
    fail("DOM 选择器不能超过 500 个 UTF-8 字节或包含空字符。");
  if (source.type === "dom" && !source.selector?.trim())
    fail("DOM 来源必须填写选择器。");
  if (
    source.type === "api" &&
    source.selector !== undefined &&
    source.selector !== ""
  )
    fail("API 来源不能填写 DOM 选择器。");
  if (
    source.credential_ref !== undefined &&
    (utf8Length(source.credential_ref) > 120 ||
      (source.credential_ref !== "" &&
        !/^[a-zA-Z][a-zA-Z0-9_.:/-]{0,119}$/.test(source.credential_ref)))
  )
    fail("凭据引用须符合 120 UTF-8 字节以内的标识格式，不能填写实际密钥。");
  return {
    id: source.id,
    name: source.name.trim(),
    type: source.type,
    priority: source.priority,
    enabled: source.enabled,
    endpoint: source.endpoint,
    ...(source.type === "dom" ? { selector: source.selector!.trim() } : {}),
    ...(source.credential_ref?.trim()
      ? { credential_ref: source.credential_ref.trim() }
      : {}),
  };
}
export function buildSourceSetBody(
  version: number,
  sources: SourceConfig[],
  reason: string,
) {
  if (!safeInt(version, 1)) fail("彩种版本无效，请重新读取。");
  if (!reason.trim() || utf8Length(reason.trim()) > 500)
    fail("保存来源前必须填写不超过 500 个 UTF-8 字节的原因。");
  if (!Array.isArray(sources) || sources.length > 16)
    fail("开奖来源最多可配置 16 个。");
  const clean = sources.map(validateSource);
  if (new Set(clean.map((source) => source.id)).size !== clean.length)
    fail("来源 ID 不能重复。");
  if (new Set(clean.map((source) => source.priority)).size !== clean.length)
    fail("来源优先级不能重复。");
  return { version, sources: clean, reason: reason.trim() };
}

function parseNumberList(
  input: string,
  label: string,
  count: number,
  allowed: number[],
  allowRepeat: boolean,
) {
  if (count === 0) {
    if (input.trim()) fail(`${label}在当前彩种模型中必须留空。`);
    return [];
  }
  const tokens = input.split(",").map((part) => part.trim());
  if (
    !input.trim() ||
    tokens.length !== count ||
    tokens.some((part) => !/^(?:0|[1-9]\d*)$/.test(part))
  )
    fail(`${label}必须恰好填写 ${count} 个逗号分隔的非负整数。`);
  const values = tokens.map((part) => {
    const value = Number(part);
    if (!Number.isSafeInteger(value) || !allowed.includes(value))
      fail(`${label}包含超出规则范围的数字。`);
    return value;
  });
  if (!allowRepeat && new Set(values).size !== values.length)
    fail(`${label}不能包含重复数字。`);
  return values;
}
export function buildDrawResult(
  model: RuleModel,
  regularInput: string,
  specialInput: string,
  digitsInput: string,
): RuleDraw {
  if (model.model === "DIGITS_0_9") {
    const digits = parseNumberList(
      digitsInput,
      "数字位置",
      model.length,
      [0, 1, 2, 3, 4, 5, 6, 7, 8, 9],
      model.allow_repeat,
    );
    if (regularInput.trim() || specialInput.trim())
      fail("数字型开奖只填写数字位置。");
    return { regular: [], special: [], digits };
  }
  if (digitsInput.trim()) fail("此彩种不接受数字位置输入。");
  const regular = parseNumberList(
    regularInput,
    "普通号码",
    model.regular_count,
    allowedPoolValues(model.regular_pool),
    model.regular_pool.allow_repeat,
  );
  const special = parseNumberList(
    specialInput,
    "特别号码",
    model.special_count,
    allowedPoolValues(model.special_pool),
    model.special_pool.allow_repeat,
  );
  if (model.model === "M_SELECT_N" && new Set(regular).size !== regular.length)
    fail("M_SELECT_N 普通号码不能重复。");
  if (
    model.model === "M_SELECT_N" &&
    regular.some((value) => special.includes(value))
  )
    fail("M_SELECT_N 普通号码与特别号码不能重叠。");
  return { regular, special, digits: [] };
}
export function buildManualDrawBody(args: {
  version: number;
  period: Period;
  model: RuleModel;
  regularInput: string;
  specialInput: string;
  digitsInput: string;
  drawnAt: string;
  reason: string;
  confirmed: boolean;
  serverNow?: number;
}): ManualDrawBody {
  if (!safeInt(args.version, 1) || args.version !== args.period.version)
    fail("期数版本无效，请重新读取。");
  if (!args.confirmed) fail("请先明确确认手动录入开奖结果。");
  if (!args.reason.trim() || utf8Length(args.reason.trim()) > 500)
    fail("手动录入必须填写不超过 500 个 UTF-8 字节的原因。");
  if (!isUtcIsoTimestamp(args.drawnAt))
    fail("开奖时间须为有效的 UTC RFC3339 时间。");
  if (Date.parse(args.drawnAt) < Date.parse(args.period.draw_at))
    fail("开奖时间不能早于期数的计划开奖时间。");
  if (Date.parse(args.drawnAt) > (args.serverNow ?? Date.now()))
    fail("开奖时间不能晚于当前时间。");
  return {
    version: args.version,
    period_no: args.period.period_no,
    result: buildDrawResult(
      args.model,
      args.regularInput,
      args.specialInput,
      args.digitsInput,
    ),
    drawn_at: args.drawnAt,
    reason: args.reason.trim(),
  };
}

function parseDrawResult(value: unknown, model: RuleModel): DrawResult {
  if (
    !isRecord(value) ||
    !nonempty(value.id) ||
    !nonempty(value.brand_id) ||
    !nonempty(value.game_id) ||
    !nonempty(value.period_id) ||
    !isUuid(value.source_id) ||
    !["api", "dom", "manual"].includes(String(value.kind)) ||
    !nonempty(value.result_hash) ||
    (value.created_by !== undefined && !nonempty(value.created_by)) ||
    (value.corrected_from_id !== undefined &&
      !nonempty(value.corrected_from_id))
  )
    invalidResponse("开奖结果响应格式无效。");
  if (
    !isRecord(value.result) ||
    !Array.isArray(value.result.regular) ||
    !Array.isArray(value.result.special) ||
    !Array.isArray(value.result.digits)
  )
    invalidResponse("开奖结果数字响应格式无效。");
  if (
    [
      ...value.result.regular,
      ...value.result.special,
      ...value.result.digits,
    ].some((item) => !safeInt(item))
  )
    invalidResponse("开奖结果数字响应格式无效。");
  let result: RuleDraw;
  try {
    result = buildDrawResult(
      model,
      value.result.regular.join(","),
      value.result.special.join(","),
      value.result.digits.join(","),
    );
  } catch {
    return invalidResponse("开奖结果与当前彩种规则不匹配。");
  }
  return {
    ...(value as unknown as DrawResult),
    result,
    drawn_at: parseIsoTimestamp(value.drawn_at, "drawn_at"),
    created_at: parseIsoTimestamp(value.created_at, "created_at"),
  };
}
function parseAttemptBatch(value: unknown): AttemptBatch {
  if (
    !isRecord(value) ||
    !nonempty(value.id) ||
    !nonempty(value.brand_id) ||
    !nonempty(value.game_id) ||
    !nonempty(value.period_id) ||
    !isUuid(value.source_set_id) ||
    !safeInt(value.observed_period_version, 1) ||
    !["no_data", "accepted", "discarded", "failed"].includes(
      String(value.status),
    ) ||
    !Array.isArray(value.attempts) ||
    value.attempts.some(
      (item) =>
        !isRecord(item) ||
        !isUuid(item.source_id) ||
        !nonempty(item.status) ||
        !nonempty(item.code),
    )
  )
    invalidResponse("开奖尝试响应格式无效。");
  return {
    ...(value as unknown as AttemptBatch),
    created_at: parseIsoTimestamp(value.created_at, "created_at"),
  };
}
function stableJson(value: unknown): string {
  const sorted = (input: unknown): unknown =>
    Array.isArray(input)
      ? input.map(sorted)
      : input && typeof input === "object"
        ? Object.fromEntries(
            Object.entries(input)
              .sort(([a], [b]) => a.localeCompare(b))
              .map(([key, item]) => [key, sorted(item)]),
          )
        : input;
  return JSON.stringify(sorted(value));
}
type Envelope<T> = {
  success?: boolean;
  data?: T;
  error?: { code?: string; message?: string } | null;
};

export function createDrawManagementApi(fetcher: typeof fetch = fetch) {
  async function request<T>(
    brandId: string,
    path: string,
    options: {
      method?: "GET" | "PUT" | "POST";
      body?: unknown;
      key?: string;
    } = {},
    parse: (value: unknown) => T,
  ): Promise<T> {
    if (!brandId.trim()) throw new AdminApiError("请先选择品牌。", 0);
    const headers = new Headers({
      Accept: "application/json",
      "X-Brand-ID": brandId,
    });
    if (options.body !== undefined) {
      headers.set("Content-Type", "application/json");
      headers.set("Idempotency-Key", options.key ?? createIdempotencyKey());
    }
    const response = await fetcher(`${BASE}${path}`, {
      method: options.method ?? "GET",
      credentials: "same-origin",
      headers,
      ...(options.body === undefined
        ? {}
        : { body: JSON.stringify(options.body) }),
    });
    let envelope: Envelope<unknown> | null;
    try {
      envelope = await response.json();
    } catch {
      throw new AdminApiError("服务器未返回有效 JSON。", response.status);
    }
    if (
      !response.ok ||
      !isRecord(envelope) ||
      envelope.success !== true ||
      !("data" in envelope)
    ) {
      const detail = isRecord(envelope?.error) ? envelope.error : undefined;
      if (typeof envelope?.error === "string") throw new AdminApiError("Invalid server response", 502, "INVALID_RESPONSE");
      throw new AdminApiError(
        typeof detail?.message === "string"
            ? detail.message
            : `请求失败（${response.status}）`,
        response.status,
        typeof detail?.code === "string" ? detail.code : undefined,
      );
    }
    return parse(envelope.data);
  }
  const id = encodeURIComponent;
  return {
    getGames(brandId: string, limit = 25, offset = 0) {
      if (!safeInt(limit, 1) || limit > 100 || !safeInt(offset))
        fail("彩种分页参数无效。");
      return request(
        brandId,
        `/games?limit=${limit}&offset=${offset}`,
        {},
        (value) => {
          if (
            !isRecord(value) ||
            !Array.isArray(value.games) ||
            value.games.length > limit
          )
            invalidResponse("彩种分页响应格式无效。");
          return { games: value.games.map(parseDrawGame) };
        },
      );
    },
    getSourceSet(brandId: string, gameId: string) {
      return request<SourceSet | null>(
        brandId,
        `/games/${id(gameId)}/draw-sources`,
        {},
        (value) => (value === null ? null : parseSourceSet(value)),
      ).catch((cause) => {
        if (cause instanceof AdminApiError && cause.status === 404) return null;
        throw cause;
      });
    },
    updateSourceSet(
      brandId: string,
      gameId: string,
      body: { version: number; sources: SourceConfig[]; reason: string },
      key: string,
    ) {
      const clean = buildSourceSetBody(body.version, body.sources, body.reason);
      return request(
        brandId,
        `/games/${id(gameId)}/draw-sources`,
        { method: "PUT", body: clean, key },
        parseSourceSet,
      );
    },
    getPeriods(brandId: string, gameId: string, limit = 25, offset = 0) {
      if (!safeInt(limit, 1) || limit > 100 || !safeInt(offset))
        fail("期数分页参数无效。");
      return request(
        brandId,
        `/games/${id(gameId)}/periods?limit=${limit}&offset=${offset}`,
        {},
        (value) => {
          if (
            !isRecord(value) ||
            !Array.isArray(value.periods) ||
            value.periods.length > limit
          )
            invalidResponse("期数分页响应格式无效。");
          return { periods: value.periods.map(parsePeriod), limit, offset };
        },
      );
    },
    getDrawHistory(
      brandId: string,
      game: DrawGame,
      periodId: string,
      limit = 50,
      offset = 0,
    ) {
      if (!safeInt(limit, 1) || limit > 50 || !safeInt(offset))
        fail("开奖历史分页参数无效。");
      return request(
        brandId,
        `/periods/${id(periodId)}/draw?limit=${limit}&offset=${offset}`,
        {},
        (value) => {
          if (
            !isRecord(value) ||
            !(value.current === null || isRecord(value.current)) ||
            !Array.isArray(value.history) ||
            !Array.isArray(value.attempts) ||
            value.history.length > limit ||
            value.attempts.length > limit ||
            value.limit !== limit ||
            value.offset !== offset
          )
            invalidResponse("开奖历史分页响应格式无效。");
          return {
            current:
              value.current === null
                ? null
                : parseDrawResult(value.current, game.model),
            history: value.history.map((item) =>
              parseDrawResult(item, game.model),
            ),
            attempts: value.attempts.map(parseAttemptBatch),
            limit,
            offset,
          } satisfies DrawHistoryPage;
        },
      );
    },
    createManualDraw(
      brandId: string,
      period: Period,
      body: ManualDrawBody,
      key: string,
      game: DrawGame,
    ) {
      if (
        !safeInt(body.version, 1) ||
        body.version !== period.version ||
        !nonempty(body.period_no) ||
        body.period_no !== period.period_no ||
        !body.reason.trim() ||
        utf8Length(body.reason.trim()) > 500 ||
        !isUtcIsoTimestamp(body.drawn_at) ||
        Date.parse(body.drawn_at) < Date.parse(period.draw_at) ||
        Date.parse(body.drawn_at) > Date.now()
      )
        fail("手动开奖结果请求格式无效。");
      if (
        !isRecord(body.result) ||
        !Array.isArray(body.result.regular) ||
        !Array.isArray(body.result.special) ||
        !Array.isArray(body.result.digits) ||
        [
          ...body.result.regular,
          ...body.result.special,
          ...body.result.digits,
        ].some((item) => !safeInt(item))
      )
        fail("手动开奖结果数字格式无效。");
      let result: RuleDraw;
      try {
        result = buildDrawResult(
          game.model,
          body.result.regular.join(","),
          body.result.special.join(","),
          body.result.digits.join(","),
        );
      } catch (cause) {
        fail(
          cause instanceof Error
            ? cause.message
            : "手动开奖结果不符合彩种模型。",
        );
      }
      const cleanBody = {
        ...body,
        period_no: body.period_no.trim(),
        result,
        reason: body.reason.trim(),
      };
      return request(
        brandId,
        `/periods/${id(period.id)}/manual-draw`,
        { method: "POST", body: cleanBody, key },
        (value) => parseDrawResult(value, game.model),
      );
    },
  };
}
