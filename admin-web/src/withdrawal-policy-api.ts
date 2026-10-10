import { AdminApiError, type AdminAccount } from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";
import { WALLET_SOURCES, type WalletSource } from "@lottery/shared";

const BASE = "/api/v1/admin";
const MAX_INT64 = 9223372036854775807n;
const MAX_TURNOVER_MICROS = 1000000n * 1000000n;
const UUID =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const DATE_TIME =
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/;

export type WithdrawalSource = WalletSource;
export type WithdrawalReviewMode = "manual" | "automatic";

export interface BrandWithdrawalConfig {
  enabled: boolean;
  min_points: string;
  max_points: string | null;
  allowed_sources: WithdrawalSource[];
  review_mode: WithdrawalReviewMode;
  turnover_multiple: string;
}

export interface BrandWithdrawalPolicy {
  brand_id: string;
  version: number;
  config: BrandWithdrawalConfig;
  updated_at: string;
  audit_log_id?: string;
}

export interface GameWithdrawalConfig {
  turnover_multiple: string | null;
}

export interface GameWithdrawalPolicy {
  brand_id: string;
  game_id: string;
  version: number;
  config: GameWithdrawalConfig;
  effective: {
    turnover_multiple: string;
    source: "brand" | "game";
    brand_version: number;
    game_version: number;
  };
  updated_at: string;
  audit_log_id?: string;
}

export interface PolicyRevision {
  id: string;
  brand_id: string;
  game_id: string;
  version: number;
  config: BrandWithdrawalConfig | GameWithdrawalConfig;
  changed_by: string;
  reason: string;
  created_at: string;
}

export interface UpdateBrandWithdrawalPolicyBody {
  version: number;
  config: BrandWithdrawalConfig;
  reason: string;
}

export interface UpdateGameWithdrawalPolicyBody {
  version: number;
  config: GameWithdrawalConfig;
  reason: string;
}

export function withdrawalPolicyPermissions(
  account: AdminAccount,
  brand: string,
): { view: boolean; write: boolean; gameView: boolean } {
  const brandPermissions = brandPermissionSet(account, brand);
  const view = (name: string) => Boolean(brand) && brandPermissions.has(`${name}.view.brand`);
  return {
    view: view("withdrawal_policy"),
    write:
      Boolean(brand) &&
      brandPermissions.has("withdrawal_policy.write.brand"),
    gameView: view("game"),
  };
}

/** Canonical decimal string in (0, 1,000,000], at most six decimals. */
export function isValidTurnoverMultiple(value: string): boolean {
  if (value.length > 14) return false;
  if (!/^(?:0|[1-9]\d*)(?:\.\d{1,6})?$/.test(value)) return false;
  if (value.includes(".") && value.endsWith("0")) return false;
  const [whole, fraction = ""] = value.split(".");
  const micros =
    BigInt(whole) * 1000000n + BigInt(fraction.padEnd(6, "0") || "0");
  return micros > 0n && micros <= MAX_TURNOVER_MICROS;
}

type Envelope<T> = {
  success?: boolean;
  data?: T;
  error?: { code: string; message: string } | null;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function isUuid(value: unknown): value is string {
  return typeof value === "string" && UUID.test(value);
}

function isVersion(value: unknown): value is number {
  return Number.isSafeInteger(value) && Number(value) >= 1;
}

function isDateTime(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const match = DATE_TIME.exec(value);
  if (!match || !Number.isFinite(Date.parse(value))) return false;
  const [date, time] = value.split("T");
  const [year, month, day] = date.split("-").map(Number);
  const [hour, minute, second] = time
    .split(/[.+Z-]/)[0]
    .split(":")
    .map(Number);
  const lastDay = new Date(0);
  lastDay.setUTCFullYear(year, month, 0);
  const daysInMonth = lastDay.getUTCDate();
  return (
    month >= 1 &&
    month <= 12 &&
    day >= 1 &&
    day <= daysInMonth &&
    hour <= 23 &&
    minute <= 59 &&
    second <= 59
  );
}

function isPositiveInt64(value: unknown): value is string {
  if (typeof value !== "string" || !/^[1-9]\d*$/.test(value)) return false;
  try {
    return BigInt(value) <= MAX_INT64;
  } catch {
    return false;
  }
}

function validBrandConfig(value: unknown): value is BrandWithdrawalConfig {
  if (!isRecord(value)) return false;
  if (
    typeof value.enabled !== "boolean" ||
    !isPositiveInt64(value.min_points) ||
    !(value.max_points === null || isPositiveInt64(value.max_points)) ||
    !Array.isArray(value.allowed_sources) ||
    value.allowed_sources.length === 0 ||
    value.allowed_sources.length > WALLET_SOURCES.length ||
    typeof value.review_mode !== "string" ||
    !["manual", "automatic"].includes(value.review_mode) ||
    typeof value.turnover_multiple !== "string" ||
    !isValidTurnoverMultiple(value.turnover_multiple)
  )
    return false;
  if (
    value.max_points !== null &&
    BigInt(value.max_points) < BigInt(value.min_points)
  )
    return false;
  const sources = value.allowed_sources;
  return (
    sources.every(
      (source) =>
        typeof source === "string" &&
        (WALLET_SOURCES as readonly string[]).includes(source),
    ) && new Set(sources).size === sources.length
  );
}

function validGameConfig(value: unknown): value is GameWithdrawalConfig {
  return (
    isRecord(value) &&
    (value.turnover_multiple === null ||
      (typeof value.turnover_multiple === "string" &&
        isValidTurnoverMultiple(value.turnover_multiple)))
  );
}

function validBrandPolicy(
  value: unknown,
  brand: string,
): value is BrandWithdrawalPolicy {
  return (
    isRecord(value) &&
    value.brand_id === brand &&
    isUuid(value.brand_id) &&
    isVersion(value.version) &&
    validBrandConfig(value.config) &&
    isDateTime(value.updated_at) &&
    (value.audit_log_id === undefined || isUuid(value.audit_log_id))
  );
}
function sameBrandConfig(
  a: BrandWithdrawalConfig,
  b: BrandWithdrawalConfig,
): boolean {
  return (
    a.enabled === b.enabled &&
    a.min_points === b.min_points &&
    a.max_points === b.max_points &&
    a.review_mode === b.review_mode &&
    a.turnover_multiple === b.turnover_multiple &&
    a.allowed_sources.length === b.allowed_sources.length &&
    a.allowed_sources.every(
      (value, index) => value === b.allowed_sources[index],
    )
  );
}

function validGamePolicy(
  value: unknown,
  brand: string,
  game: string,
): value is GameWithdrawalPolicy {
  if (
    !isRecord(value) ||
    value.brand_id !== brand ||
    value.game_id !== game ||
    !isUuid(value.brand_id) ||
    !isUuid(value.game_id) ||
    !isVersion(value.version) ||
    !validGameConfig(value.config) ||
    !isRecord(value.effective) ||
    typeof value.effective.turnover_multiple !== "string" ||
    !isValidTurnoverMultiple(value.effective.turnover_multiple) ||
    !["brand", "game"].includes(String(value.effective.source)) ||
    !isVersion(value.effective.brand_version) ||
    !isVersion(value.effective.game_version) ||
    value.effective.game_version !== value.version ||
    !isDateTime(value.updated_at) ||
    (value.audit_log_id !== undefined && !isUuid(value.audit_log_id))
  )
    return false;
  const hasGameOverride = value.config.turnover_multiple !== null;
  if (value.effective.source !== (hasGameOverride ? "game" : "brand"))
    return false;
  if (
    hasGameOverride &&
    value.effective.turnover_multiple !== value.config.turnover_multiple
  )
    return false;
  return true;
}

function validRevision(
  value: unknown,
  brand: string,
  game: string,
): value is PolicyRevision {
  if (
    !isRecord(value) ||
    !isUuid(value.id) ||
    value.brand_id !== brand ||
    !isUuid(value.brand_id) ||
    value.game_id !== game ||
    !(game === "" || isUuid(value.game_id)) ||
    !isVersion(value.version) ||
    !(game === ""
      ? validBrandConfig(value.config)
      : validGameConfig(value.config)) ||
    typeof value.changed_by !== "string" ||
    typeof value.reason !== "string" ||
    value.reason.trim().length === 0 ||
    new TextEncoder().encode(value.reason).length > 500 ||
    !isDateTime(value.created_at)
  )
    return false;
  return value.version === 1
    ? value.changed_by === ""
    : isUuid(value.changed_by);
}

function invalidResponse(): never {
  throw new AdminApiError("提现策略响应格式无效。", 502, "INVALID_RESPONSE");
}

function invalidInput(message: string): never {
  throw new AdminApiError(message, 0, "INVALID_INPUT");
}

function validateContext(brand: string, game?: string): void {
  if (!isUuid(brand)) invalidInput("品牌编号必须为有效 UUID。");
  if (game !== undefined && !isUuid(game))
    invalidInput("彩种编号必须为有效 UUID。");
}

function validateUpdateBody(
  body: UpdateBrandWithdrawalPolicyBody | UpdateGameWithdrawalPolicyBody,
  game: boolean,
): void {
  if (
    !isRecord(body) ||
    !isVersion(body.version) ||
    typeof body.reason !== "string" ||
    body.reason.trim().length === 0 ||
    new TextEncoder().encode(body.reason).length > 500 ||
    !(game ? validGameConfig(body.config) : validBrandConfig(body.config))
  )
    invalidInput("提现策略输入无效。");
}

function validateHistoryArgs(limit: number, offset: number): void {
  if (
    !Number.isSafeInteger(limit) ||
    limit < 1 ||
    limit > 100 ||
    !Number.isSafeInteger(offset) ||
    offset < 0 ||
    offset > 1_000_000
  )
    invalidInput("历史分页参数无效。");
}

export function createWithdrawalPolicyApi(fetcher: typeof fetch = fetch) {
  async function request<T>(
    brand: string,
    path: string,
    options: { method?: "GET" | "PUT"; body?: unknown; key?: string } = {},
  ): Promise<T> {
    const headers = new Headers({
      Accept: "application/json",
      "X-Brand-ID": brand,
    });
    if (options.body !== undefined)
      headers.set("Content-Type", "application/json");
    if (options.key !== undefined) headers.set("Idempotency-Key", options.key);
    let response: Response;
    try {
      response = await fetcher(path, {
        method: options.method ?? "GET",
        credentials: "same-origin",
        headers,
        ...(options.body === undefined
          ? {}
          : { body: JSON.stringify(options.body) }),
      });
    } catch (cause) {
      throw new AdminApiError(
        cause instanceof Error ? cause.message : "Network request failed",
        0,
        "NETWORK_ERROR",
      );
    }
    let envelope: Envelope<T>;
    try {
      envelope = (await response.json()) as Envelope<T>;
    } catch {
      throw new AdminApiError("Invalid server response; operation result is unconfirmed", 502, "INVALID_RESPONSE");
    }
    if (!isRecord(envelope)) throw new AdminApiError("Invalid server response; operation result is unconfirmed", 502, "INVALID_RESPONSE");
    const error = envelope.error;
    if (!response.ok && envelope.success === false && isRecord(error)
      && typeof error.code === "string" && error.code.trim()
      && typeof error.message === "string" && error.message.trim()) {
      throw new AdminApiError(error.message, response.status, error.code);
    }
    if (
      !response.ok ||
      envelope.success !== true ||
      envelope.data == null || !isRecord(envelope.data)
    ) {
      throw new AdminApiError("Invalid server response; operation result is unconfirmed", 502, "INVALID_RESPONSE");
    }
    return envelope.data;
  }

  async function history(
    brand: string,
    game: string,
    limit = 50,
    offset = 0,
  ): Promise<{ items: PolicyRevision[]; limit: number; offset: number }> {
    validateContext(brand, game || undefined);
    validateHistoryArgs(limit, offset);
    const suffix = game ? `/games/${encodeURIComponent(game)}` : "";
    const value = await request<unknown>(
      brand,
      `${BASE}${suffix}/withdrawal-policy/history?${new URLSearchParams({ limit: String(limit), offset: String(offset) })}`,
    );
    if (
      !isRecord(value) ||
      value.limit !== limit ||
      value.offset !== offset ||
      !Array.isArray(value.items) ||
      value.items.length > limit ||
      !value.items.every((item) => validRevision(item, brand, game)) ||
      new Set(value.items.map((item) => item.id)).size !== value.items.length ||
      !value.items.every(
        (item, index, items) =>
          index === 0 || item.version < items[index - 1].version,
      )
    )
      invalidResponse();
    return value as { items: PolicyRevision[]; limit: number; offset: number };
  }

  return {
    async getBrandPolicy(brand: string): Promise<BrandWithdrawalPolicy> {
      validateContext(brand);
      const value = await request<unknown>(brand, `${BASE}/withdrawal-policy`);
      if (!validBrandPolicy(value, brand)) invalidResponse();
      return value;
    },
    async updateBrandPolicy(
      brand: string,
      body: UpdateBrandWithdrawalPolicyBody,
      key: string,
    ): Promise<BrandWithdrawalPolicy> {
      validateContext(brand);
      validateUpdateBody(body, false);
      if (typeof key !== "string" || key.length === 0)
        invalidInput("幂等键不能为空。");
      const value = await request<unknown>(brand, `${BASE}/withdrawal-policy`, {
        method: "PUT",
        body,
        key,
      });
      if (
        !validBrandPolicy(value, brand) ||
        value.version !== body.version + 1 ||
        !isUuid(value.audit_log_id) ||
        !sameBrandConfig(value.config, body.config)
      )
        invalidResponse();
      return value;
    },
    async getGamePolicy(
      brand: string,
      game: string,
    ): Promise<GameWithdrawalPolicy> {
      validateContext(brand, game);
      const value = await request<unknown>(
        brand,
        `${BASE}/games/${encodeURIComponent(game)}/withdrawal-policy`,
      );
      if (!validGamePolicy(value, brand, game)) invalidResponse();
      return value;
    },
    async updateGamePolicy(
      brand: string,
      game: string,
      body: UpdateGameWithdrawalPolicyBody,
      key: string,
    ): Promise<GameWithdrawalPolicy> {
      validateContext(brand, game);
      validateUpdateBody(body, true);
      if (typeof key !== "string" || key.length === 0)
        invalidInput("幂等键不能为空。");
      const value = await request<unknown>(
        brand,
        `${BASE}/games/${encodeURIComponent(game)}/withdrawal-policy`,
        { method: "PUT", body, key },
      );
      if (
        !validGamePolicy(value, brand, game) ||
        value.version !== body.version + 1 ||
        !isUuid(value.audit_log_id) ||
        value.config.turnover_multiple !== body.config.turnover_multiple
      )
        invalidResponse();
      return value;
    },
    getBrandHistory(brand: string, limit = 50, offset = 0) {
      return history(brand, "", limit, offset);
    },
    async getGameHistory(brand: string, game: string, limit = 50, offset = 0) {
      validateContext(brand, game);
      return history(brand, game, limit, offset);
    },
  };
}
