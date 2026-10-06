import {
  BettingApiError,
  bettingBasePath,
  type CatalogGame,
  type Period,
} from "./betting-api";

export interface PublicDrawResult {
  id: string;
  game: CatalogGame;
  period: Period;
  result: {
    regular: number[];
    special: number[];
    digits: number[];
  };
  drawn_at: string;
  origin: "manual" | "external";
}

export interface PublicPeriod {
  period: Period;
  draw: PublicDrawResult | null;
}

export interface ResultPage {
  items: PublicDrawResult[];
  limit: number;
  offset: number;
  has_more: boolean;
  server_time: string;
  brand_status: "active" | "paused";
}

export interface ResultDetail {
  item: PublicDrawResult;
  server_time: string;
  brand_status: "active" | "paused";
}

export interface PeriodPage {
  game: CatalogGame;
  items: PublicPeriod[];
  limit: number;
  offset: number;
  has_more: boolean;
  server_time: string;
  brand_status: "active" | "paused";
}

export interface ListResultsOptions {
  gameId?: string;
  periodNo?: string;
  limit?: number;
  offset?: number;
}

export interface ListPeriodsOptions {
  periodNo?: string;
  limit?: number;
  offset?: number;
}

const UUID =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const ISO_DATE_TIME =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(Z|[+-](\d{2}):(\d{2}))$/;

function malformed(message: string): never {
  throw new BettingApiError(
    `Malformed draws API response: ${message}`,
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

function string(value: unknown, label: string, nonempty = true): string {
  if (typeof value !== "string" || (nonempty && value.length === 0)) {
    return malformed(
      `${label} must be ${nonempty ? "a nonempty " : "a "}string`,
    );
  }
  return value;
}

function uuid(value: unknown, label: string): string {
  const parsed = string(value, label);
  if (!UUID.test(parsed)) return malformed(`${label} must be a UUID`);
  return parsed.toLowerCase();
}

function validateUuidParameter(value: string, label: string): void {
  if (typeof value !== "string" || !UUID.test(value)) {
    throw new BettingApiError(
      `${label} must be a UUID`,
      0,
      "invalid_parameter",
    );
  }
}

function dateTime(value: unknown, label: string): string {
  const parsed = string(value, label);
  const parts = ISO_DATE_TIME.exec(parsed);
  if (!parts || !Number.isFinite(Date.parse(parsed))) {
    return malformed(`${label} must be an ISO date-time`);
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
    return malformed(`${label} must be a valid ISO date-time`);
  }
  return parsed;
}

function parseGame(value: unknown): CatalogGame {
  const game = object(value, "game");
  const model = object(game.model, "game.model");
  const parsePool = (value: unknown, label: string) => {
    const pool = object(value, label);
    if (typeof pool.allow_repeat !== "boolean")
      return malformed(`${label}.allow_repeat must be a boolean`);
    for (const key of ["min", "max"] as const) {
      if (pool[key] !== undefined && !Number.isSafeInteger(pool[key])) {
        return malformed(`${label}.${key} must be an integer`);
      }
    }
    if (
      pool.values !== undefined &&
      pool.values !== null &&
      (!Array.isArray(pool.values) ||
        !pool.values.every((item) => Number.isSafeInteger(item) && item >= 0))
    ) {
      return malformed(
        `${label}.values must be an array of nonnegative integers or null`,
      );
    }
    return pool;
  };
  for (const key of [
    "regular_count",
    "special_count",
    "pool_size",
    "total_count",
    "length",
  ] as const) {
    if (!Number.isSafeInteger(model[key]) || (model[key] as number) < 0) {
      return malformed(`game.model.${key} must be a nonnegative integer`);
    }
  }
  for (const key of ["allow_repeat", "ordered"] as const) {
    if (typeof model[key] !== "boolean")
      return malformed(`game.model.${key} must be a boolean`);
  }
  const parsedModel: CatalogGame["model"] = {
    model: string(model.model, "game.model.model"),
    regular_pool: parsePool(
      model.regular_pool,
      "game.model.regular_pool",
    ) as unknown as CatalogGame["model"]["regular_pool"],
    special_pool: parsePool(
      model.special_pool,
      "game.model.special_pool",
    ) as unknown as CatalogGame["model"]["special_pool"],
    regular_count: model.regular_count as number,
    special_count: model.special_count as number,
    pool_size: model.pool_size as number,
    total_count: model.total_count as number,
    length: model.length as number,
    allow_repeat: model.allow_repeat as boolean,
    ordered: model.ordered as boolean,
  };
  if (!["DIGITS_0_9", "X_PLUS_Y", "M_SELECT_N"].includes(parsedModel.model))
    return malformed("unsupported game model");
  if (game.status !== "active" && game.status !== "paused")
    return malformed("game must be publicly readable");
  try {
    new Intl.DateTimeFormat("en", {
      timeZone: string(game.timezone, "game.timezone"),
    });
  } catch {
    return malformed("game.timezone must be a valid timezone");
  }
  return {
    id: uuid(game.id, "game.id"),
    code: string(game.code, "game.code"),
    name: string(game.name, "game.name"),
    model: parsedModel,
    timezone: string(game.timezone, "game.timezone"),
    status: string(game.status, "game.status"),
  };
}

function parsePeriod(value: unknown): Period {
  const period = object(value, "period");
  if (
    ![
      "pending",
      "betting",
      "closed",
      "waiting_draw",
      "drawn",
      "settling",
      "settled",
      "bet_cancelled",
      "judged_cancelled",
    ].includes(String(period.status))
  )
    return malformed("unknown period status");
  const drawResultId = period.draw_result_id;
  if (drawResultId !== undefined && drawResultId !== null) {
    uuid(drawResultId, "period.draw_result_id");
  }
  const parsed = {
    id: uuid(period.id, "period.id"),
    game_id: uuid(period.game_id, "period.game_id"),
    period_no: string(period.period_no, "period.period_no"),
    status: string(period.status, "period.status"),
    bet_start_at: dateTime(period.bet_start_at, "period.bet_start_at"),
    bet_end_at: dateTime(period.bet_end_at, "period.bet_end_at"),
    draw_at: dateTime(period.draw_at, "period.draw_at"),
    ...(drawResultId === undefined || drawResultId === null
      ? {}
      : { draw_result_id: drawResultId as string }),
  };
  if (
    Date.parse(parsed.bet_start_at) >= Date.parse(parsed.bet_end_at) ||
    Date.parse(parsed.bet_end_at) > Date.parse(parsed.draw_at)
  )
    return malformed("invalid period time window");
  return parsed;
}

function parseNumberArray(value: unknown, label: string): number[] {
  if (
    !Array.isArray(value) ||
    !value.every((item) => Number.isSafeInteger(item) && item >= 0)
  ) {
    return malformed(`${label} must be an array of nonnegative integers`);
  }
  return value as number[];
}

function parseDraw(value: unknown): PublicDrawResult {
  const draw = object(value, "draw result");
  const game = parseGame(draw.game);
  const period = parsePeriod(draw.period);
  if (period.game_id !== game.id) {
    return malformed("period.game_id must match game.id");
  }
  const id = uuid(draw.id, "draw result.id");
  if (period.draw_result_id !== id) {
    return malformed("period.draw_result_id must match result.id");
  }
  const result = object(draw.result, "result.result");
  const origin = string(draw.origin, "draw result.origin");
  if (origin !== "manual" && origin !== "external") {
    return malformed("draw result.origin must be manual or external");
  }
  if (
    ![
      "drawn",
      "settling",
      "settled",
      "bet_cancelled",
      "judged_cancelled",
    ].includes(period.status)
  )
    return malformed("result period has not been drawn");
  const parsed: PublicDrawResult = {
    id,
    game,
    period,
    result: {
      regular: parseNumberArray(result.regular, "result.regular"),
      special: parseNumberArray(result.special, "result.special"),
      digits: parseNumberArray(result.digits, "result.digits"),
    },
    drawn_at: dateTime(draw.drawn_at, "draw result.drawn_at"),
    origin,
  };
  const model = game.model;
  const values = parsed.result;
  if (model.model === "DIGITS_0_9") {
    if (
      model.length < 1 ||
      model.length > 10 ||
      !model.ordered ||
      values.digits.length !== model.length ||
      values.regular.length ||
      values.special.length ||
      values.digits.some((value) => value > 9) ||
      (!model.allow_repeat &&
        new Set(values.digits).size !== values.digits.length)
    )
      return malformed("digits do not match the published model");
  } else {
    const inPool = (
      values: number[],
      pool: CatalogGame["model"]["regular_pool"],
    ) =>
      values.every((value) =>
        pool.values?.length
          ? pool.values.includes(value)
          : value >= (pool.min ?? 0) && value <= (pool.max ?? 0),
      ) &&
      (pool.allow_repeat || new Set(values).size === values.length);
    if (
      model.regular_count > 10 ||
      model.special_count > 10 ||
      values.regular.length !== model.regular_count ||
      values.special.length !== model.special_count ||
      values.digits.length ||
      !inPool(values.regular, model.regular_pool) ||
      !inPool(values.special, model.special_pool) ||
      (model.model === "M_SELECT_N" &&
        values.regular.some((value) => values.special.includes(value)))
    )
      return malformed("numbers do not match the published model");
  }
  if (Date.parse(parsed.drawn_at) < Date.parse(period.draw_at))
    return malformed("actual draw precedes its planned time");
  return parsed;
}

function parseBrandStatus(value: unknown): "active" | "paused" {
  if (value !== "active" && value !== "paused") {
    return malformed("brand_status must be active or paused");
  }
  return value;
}

function parsePageMeta(
  value: Record<string, unknown>,
  limit: number,
  offset: number,
) {
  if (value.limit !== limit || value.offset !== offset) {
    return malformed("response pagination does not match the request");
  }
  if (typeof value.has_more !== "boolean") {
    return malformed("has_more must be a boolean");
  }
  return {
    limit,
    offset,
    has_more: value.has_more,
    server_time: dateTime(value.server_time, "server_time"),
    brand_status: parseBrandStatus(value.brand_status),
  };
}

function validatePageOptions(options: { limit?: number; offset?: number }) {
  const limit = options.limit ?? 50;
  const offset = options.offset ?? 0;
  if (!Number.isInteger(limit) || limit < 1 || limit > 100) {
    throw new BettingApiError(
      "limit must be an integer from 1 to 100",
      0,
      "invalid_parameter",
    );
  }
  if (!Number.isInteger(offset) || offset < 0 || offset > 1_000_000) {
    throw new BettingApiError(
      "offset must be an integer from 0 to 1000000",
      0,
      "invalid_parameter",
    );
  }
  return { limit, offset };
}

function validatePeriodNo(periodNo: string | undefined): void {
  if (periodNo === undefined) return;
  if (
    typeof periodNo !== "string" ||
    periodNo.trim().length === 0 ||
    new TextEncoder().encode(periodNo).length > 80
  ) {
    throw new BettingApiError(
      "periodNo must be a nonempty string of at most 80 UTF-8 bytes",
      0,
      "invalid_parameter",
    );
  }
}

function queryString(
  options: ListResultsOptions | ListPeriodsOptions,
  paging: { limit: number; offset: number },
  gameId?: string,
): string {
  const params = new URLSearchParams();
  if (gameId !== undefined) params.set("game_id", gameId);
  if (options.periodNo !== undefined) params.set("period_no", options.periodNo);
  params.set("limit", String(paging.limit));
  params.set("offset", String(paging.offset));
  return params.toString();
}

function unwrap(value: unknown): Record<string, unknown> {
  const root = object(value, "response");
  if ("success" in root || "data" in root) {
    if (root.success !== true) {
      const error =
        typeof root.error === "object" && root.error
          ? (root.error as Record<string, unknown>)
          : undefined;
      throw new BettingApiError(
        typeof root.error === "string"
          ? root.error
          : typeof error?.message === "string"
            ? error.message
            : "Request failed",
        200,
        typeof error?.code === "string" ? error.code : undefined,
      );
    }
    return object(root.data, "response.data");
  }
  return root;
}

export function createDrawsApi(
  brandCode?: string,
  fetcher: typeof fetch = fetch,
) {
  const configuredBrand =
    brandCode ?? import.meta.env.VITE_BRAND_CODE ?? undefined;
  const base = bettingBasePath(configuredBrand);

  async function request(path: string): Promise<Record<string, unknown>> {
    let response: Response;
    try {
      response = await fetcher(`${base}${path}`, {
        method: "GET",
        credentials: "include",
        headers: { Accept: "application/json" },
      });
    } catch (cause) {
      throw new BettingApiError(
        cause instanceof Error ? cause.message : "Network request failed",
        0,
      );
    }

    let payload: unknown;
    try {
      payload = JSON.parse(await response.text());
    } catch {
      throw new BettingApiError(
        response.ok
          ? "Server returned malformed JSON"
          : `Request failed (${response.status})`,
        response.ok ? 502 : response.status,
        response.ok ? "invalid_response" : undefined,
      );
    }
    if (!response.ok) {
      const envelope =
        payload && typeof payload === "object"
          ? (payload as Record<string, unknown>)
          : undefined;
      const error =
        typeof envelope?.error === "object" && envelope.error
          ? (envelope.error as Record<string, unknown>)
          : undefined;
      throw new BettingApiError(
        typeof envelope?.error === "string"
          ? envelope.error
          : typeof error?.message === "string"
            ? error.message
            : `Request failed (${response.status})`,
        response.status,
        typeof error?.code === "string" ? error.code : undefined,
      );
    }
    return unwrap(payload);
  }

  return {
    async listResults(options: ListResultsOptions = {}): Promise<ResultPage> {
      if (options.gameId !== undefined)
        validateUuidParameter(options.gameId, "gameId");
      validatePeriodNo(options.periodNo);
      const paging = validatePageOptions(options);
      const data = await request(
        `/draw-results?${queryString(options, paging, options.gameId)}`,
      );
      if (!Array.isArray(data.items))
        return malformed("items must be an array");
      const items = data.items.map(parseDraw);
      if (
        items.length > paging.limit ||
        new Set(items.map((item) => item.id)).size !== items.length ||
        (data.has_more === true && items.length !== paging.limit)
      )
        return malformed("invalid result page size or duplicate results");
      if (
        options.gameId !== undefined &&
        items.some((item) => item.game.id !== options.gameId!.toLowerCase())
      ) {
        return malformed("result game does not match game_id filter");
      }
      if (
        options.periodNo !== undefined &&
        items.some((item) => item.period.period_no !== options.periodNo)
      ) {
        return malformed("result period does not match period_no filter");
      }
      return { items, ...parsePageMeta(data, paging.limit, paging.offset) };
    },

    async getResult(id: string): Promise<ResultDetail> {
      validateUuidParameter(id, "result id");
      const data = await request(`/draw-results/${encodeURIComponent(id)}`);
      const item = parseDraw(data.item);
      if (item.id !== id.toLowerCase())
        return malformed("result id does not match requested id");
      return {
        item,
        server_time: dateTime(data.server_time, "server_time"),
        brand_status: parseBrandStatus(data.brand_status),
      };
    },

    async listPeriods(
      gameId: string,
      options: ListPeriodsOptions = {},
    ): Promise<PeriodPage> {
      validateUuidParameter(gameId, "gameId");
      validatePeriodNo(options.periodNo);
      const paging = validatePageOptions(options);
      const data = await request(
        `/games/${encodeURIComponent(gameId)}/periods?${queryString(options, paging)}`,
      );
      const game = parseGame(data.game);
      if (game.id !== gameId.toLowerCase())
        return malformed("response game does not match gameId");
      if (!Array.isArray(data.items))
        return malformed("items must be an array");
      const items = data.items.map((raw): PublicPeriod => {
        const entry = object(raw, "period entry");
        const period = parsePeriod(entry.period);
        if (period.game_id !== game.id)
          return malformed("period.game_id must match game.id");
        if (
          options.periodNo !== undefined &&
          period.period_no !== options.periodNo
        ) {
          return malformed("period does not match period_no filter");
        }
        if (entry.draw === null) {
          if (period.draw_result_id !== undefined)
            return malformed("period with draw_result_id must include draw");
          return { period, draw: null };
        }
        const draw = parseDraw(entry.draw);
        if (
          period.draw_result_id !== draw.id ||
          draw.period.draw_result_id !== draw.id
        ) {
          return malformed("period.draw_result_id must match result.id");
        }
        if (draw.period.id !== period.id || draw.game.id !== game.id) {
          return malformed("draw must belong to the listed period and game");
        }
        for (const key of [
          "game_id",
          "period_no",
          "status",
          "bet_start_at",
          "bet_end_at",
          "draw_at",
          "draw_result_id",
        ] as const)
          if (draw.period[key] !== period[key])
            return malformed("nested draw period differs from history entry");
        return { period, draw };
      });
      if (
        items.length > paging.limit ||
        new Set(items.map((item) => item.period.id)).size !== items.length ||
        (data.has_more === true && items.length !== paging.limit)
      )
        return malformed("invalid history page size or duplicate periods");
      return {
        game,
        items,
        ...parsePageMeta(data, paging.limit, paging.offset),
      };
    },
  };
}
