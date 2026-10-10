import {
  AdminApiError,
  createIdempotencyKey,
  type AdminAccount,
} from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";
import type {
  RuleDefinition,
  RuleTicketSelection,
} from "../../shared/src/rules";

const BASE = "/api/v1/admin";
const MAX_INT64 = 9223372036854775807n;

export interface BrandBetPolicyConfig {
  min_bet_points: string;
  max_bet_points: string | null;
  max_period_points: string | null;
  max_user_period_points: string | null;
  user_cancel_allowed: boolean;
}
export interface LimitOverride {
  mode: "inherit" | "value" | "unlimited";
  points: string | null;
}
export interface GameBetPolicyConfig {
  min_bet_points: LimitOverride;
  max_bet_points: LimitOverride;
  max_period_points: LimitOverride;
  max_user_period_points: LimitOverride;
  user_cancel_allowed: boolean | null;
}
export interface BrandBetPolicy {
  brand_id: string;
  version: number;
  config: BrandBetPolicyConfig;
  updated_at: string;
}
export interface GameBetPolicy extends Omit<BrandBetPolicy, "config"> {
  game_id: string;
  config: GameBetPolicyConfig;
}
export interface AdminBetOrder {
  id: string;
  brand_id: string;
  global_user_id: string;
  brand_member_id: string;
  account_id: string;
  game_id: string;
  period_id: string;
  play_id: string;
  rule_version_id: string;
  definition_hash: string;
  definition_snapshot: RuleDefinition;
  status: string;
  version: number;
  selection_raw: RuleTicketSelection;
  selection_normalized: RuleTicketSelection;
  expanded_bets: RuleTicketSelection[];
  unit_points: string;
  combination_count: number;
  multiplier: string;
  total_points: string;
  deduction_allocation: { source: string; state: string; points: string }[];
  policy_snapshot: {
    min_bet_points: string;
    max_bet_points: string | null;
    max_period_points: string | null;
    max_user_period_points: string | null;
    user_cancel_allowed: boolean;
  };
  policy_versions: { brand: number; game: number };
  debit_entry_id: string;
  refund_entry_id?: string;
  client_key: string;
  placed_at: string;
  cancelled_at?: string | null;
  cancel_reason?: string;
  settlement_calculation_id?: string | null;
  payout_entry_id?: string | null;
  prize_points?: string;
  settled_at?: string | null;
}
export interface BetException {
  id: string;
  brand_id: string;
  order_id: string;
  order_version: number;
  marked_by: string;
  source?: "manual" | "system";
  job_id?: string | null;
  error_code?: string | null;
  reason: string;
  created_at: string;
}
export interface Judgment {
  id: string;
  brand_id: string;
  game_id: string;
  period_id: string;
  order_id: string;
  order_version: number;
  cause: "no_result" | "invalid_result";
  draw_result_id: string;
  judged_by: string;
  reason: string;
  created_at: string;
  refund_entry_id: string;
}
export interface ActionBody {
  version: number;
  reason: string;
}
export interface JudgeCancelBody extends ActionBody {
  cause: "no_result" | "invalid_result";
}
export interface BetManagementPermissions {
  policyView: boolean;
  policyWrite: boolean;
  ordersView: boolean;
  cancel: boolean;
  judgeCancel: boolean;
  markAbnormal: boolean;
  gameView: boolean;
}

export function betManagementPermissions(
  account: AdminAccount,
  brandId: string,
): BetManagementPermissions {
  const brand = brandPermissionSet(account, brandId);
  const view = (name: string) => Boolean(brandId) && brand.has(`${name}.view.brand`);
  const write = (grant: string) =>
    Boolean(brandId) && !account.super_admin && brand.has(grant);
  return {
    policyView: view("bet_policy"),
    policyWrite: write("bet_policy.write.brand"),
    ordersView: view("bet"),
    cancel: write("bet.cancel.brand"),
    judgeCancel: write("bet.judge_cancel.brand"),
    markAbnormal: write("bet.mark_abnormal.brand"),
    gameView: view("game"),
  };
}

export function isPositivePoints(text: string): boolean {
  if (!/^[1-9][0-9]*$/.test(text)) return false;
  try {
    return BigInt(text) <= MAX_INT64;
  } catch {
    return false;
  }
}

function stableJson(value: unknown): string {
  const sort = (input: unknown): unknown =>
    Array.isArray(input)
      ? input.map(sort)
      : input && typeof input === "object"
        ? Object.fromEntries(
            Object.entries(input)
              .sort(([a], [b]) => a.localeCompare(b))
              .map(([k, v]) => [k, sort(v)]),
          )
        : input;
  return JSON.stringify(sort(value));
}

/** Reuses keys across retries for an identical brand/entity/operation/body tuple. */
export function createBetMutationKeyTracker() {
  const entries = new Map<string, { body: string; key: string }>();
  return (scope: {
    brandId: string;
    entityId: string;
    operation: string;
    body: unknown;
  }): string => {
    const keyScope = JSON.stringify([
      scope.brandId,
      scope.entityId,
      scope.operation,
    ]);
    const body = stableJson(scope.body);
    const previous = entries.get(keyScope);
    if (previous?.body === body) return previous.key;
    const key = createIdempotencyKey();
    entries.set(keyScope, { body, key });
    return key;
  };
}

type Envelope<T> = {
  success?: boolean;
  data?: T;
  error?: { code?: string; message?: string } | null;
};
function invalidResponse(): never {
  throw new AdminApiError("投注管理响应格式无效。", 502, "INVALID_RESPONSE");
}
function isRecord(v: unknown): v is Record<string, unknown> {
  return Boolean(v && typeof v === "object" && !Array.isArray(v));
}
function nonempty(v: unknown): v is string {
  return typeof v === "string" && v.length > 0;
}
function validPolicy(v: unknown, game = false): boolean {
  if (
    !isRecord(v) ||
    !nonempty(v.brand_id) ||
    !Number.isSafeInteger(v.version) ||
    Number(v.version) < 1 ||
    !nonempty(v.updated_at) ||
    !isRecord(v.config)
  )
    return false;
  if (game && !nonempty(v.game_id)) return false;
  const c = v.config;
  if (game) {
    const override = (x: unknown, minimum = false) => {
      if (!isRecord(x)) return false;
      if (x.mode === "value")
        return typeof x.points === "string" && isPositivePoints(x.points);
      return (
        (x.mode === "inherit" || (!minimum && x.mode === "unlimited")) &&
        x.points === null
      );
    };
    if (
      !override(c.min_bet_points, true) ||
      ![c.max_bet_points, c.max_period_points, c.max_user_period_points].every(
        (x) => override(x),
      )
    )
      return false;
    return (
      c.user_cancel_allowed === null ||
      typeof c.user_cancel_allowed === "boolean"
    );
  }
  return (
    typeof c.min_bet_points === "string" &&
    isPositivePoints(c.min_bet_points) &&
    [c.max_bet_points, c.max_period_points, c.max_user_period_points].every(
      (x) => x === null || (typeof x === "string" && isPositivePoints(x)),
    ) &&
    typeof c.user_cancel_allowed === "boolean"
  );
}
function validOrder(v: unknown): v is AdminBetOrder {
  if (!isRecord(v)) return false;
  const names = [
    "id",
    "brand_id",
    "global_user_id",
    "brand_member_id",
    "account_id",
    "game_id",
    "period_id",
    "play_id",
    "rule_version_id",
    "definition_hash",
    "status",
    "debit_entry_id",
    "client_key",
    "placed_at",
  ];
  return (
    names.every((k) => nonempty(v[k])) &&
    Number.isSafeInteger(v.version) &&
    Number(v.version) > 0 &&
    ["unit_points", "multiplier", "total_points"].every(
      (k) => typeof v[k] === "string",
    ) &&
    Number.isSafeInteger(v.combination_count) &&
    Array.isArray(v.expanded_bets) &&
    Array.isArray(v.deduction_allocation) &&
    isRecord(v.policy_versions) &&
    isRecord(v.policy_snapshot) &&
    isRecord(v.definition_snapshot) &&
    isRecord(v.selection_raw) &&
    isRecord(v.selection_normalized)
  );
}

export function createBetManagementApi(fetcher: typeof fetch = fetch) {
  async function request<T>(
    brandId: string,
    path: string,
    options: {
      method?: "GET" | "POST" | "PUT";
      body?: unknown;
      key?: string;
    } = {},
  ): Promise<T> {
    if (!brandId.trim())
      throw new AdminApiError("请先选择品牌。", 0, "INVALID_INPUT");
    const headers = new Headers({
      Accept: "application/json",
      "X-Brand-ID": brandId,
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
      throw new AdminApiError(
        response.ok
          ? "Invalid server response"
          : `Request failed (${response.status})`,
        response.ok ? 502 : response.status,
        response.ok ? "INVALID_RESPONSE" : undefined,
      );
    }
    if (
      !response.ok ||
      envelope.success !== true ||
      envelope.data === undefined
    ) {
      const error =
        typeof envelope.error === "object" && envelope.error
          ? envelope.error
          : undefined;
      if (typeof envelope.error === "string") throw new AdminApiError("Invalid server response", 502, "INVALID_RESPONSE");
      throw new AdminApiError(
        error?.message || `Request failed (${response.status})`,
        response.ok ? 502 : response.status,
        error?.code,
      );
    }
    return envelope.data;
  }
  const readOrder = async (
    brandId: string,
    path: string,
    options: { method?: "GET" | "POST"; body?: unknown; key?: string } = {},
  ) => {
    const value = await request<unknown>(brandId, path, options);
    const order = isRecord(value) && "order" in value ? value.order : value;
    if (!validOrder(order)) invalidResponse();
    return order;
  };
  return {
    async getBrandPolicy(brandId: string) {
      const v = await request<unknown>(brandId, `${BASE}/bet-policy`);
      if (!validPolicy(v)) invalidResponse();
      return v as BrandBetPolicy;
    },
    async updateBrandPolicy(
      brandId: string,
      body: { version: number; config: BrandBetPolicyConfig; reason: string },
      key: string,
    ) {
      const v = await request<unknown>(brandId, `${BASE}/bet-policy`, {
        method: "PUT",
        body,
        key,
      });
      if (!validPolicy(v)) invalidResponse();
      return v as BrandBetPolicy;
    },
    async getGamePolicy(brandId: string, gameId: string) {
      const v = await request<unknown>(
        brandId,
        `${BASE}/games/${encodeURIComponent(gameId)}/bet-policy`,
      );
      if (!validPolicy(v, true)) invalidResponse();
      return v as GameBetPolicy;
    },
    async updateGamePolicy(
      brandId: string,
      gameId: string,
      body: { version: number; config: GameBetPolicyConfig; reason: string },
      key: string,
    ) {
      const v = await request<unknown>(
        brandId,
        `${BASE}/games/${encodeURIComponent(gameId)}/bet-policy`,
        { method: "PUT", body, key },
      );
      if (!validPolicy(v, true)) invalidResponse();
      return v as GameBetPolicy;
    },
    async getOrders(brandId: string, memberId = "", limit = 50, offset = 0) {
      const query = new URLSearchParams({
        limit: String(limit),
        offset: String(offset),
      });
      if (memberId) query.set("member_id", memberId);
      const v = await request<unknown>(brandId, `${BASE}/bet-orders?${query}`);
      if (!isRecord(v) || !Array.isArray(v.items) || !v.items.every(validOrder))
        invalidResponse();
      return v as { items: AdminBetOrder[] };
    },
    getOrder(brandId: string, id: string) {
      return readOrder(brandId, `${BASE}/bet-orders/${encodeURIComponent(id)}`);
    },
    cancelOrder(brandId: string, id: string, body: ActionBody, key: string) {
      return readOrder(
        brandId,
        `${BASE}/bet-orders/${encodeURIComponent(id)}/cancel`,
        { method: "POST", body, key },
      );
    },
    markAbnormal(brandId: string, id: string, body: ActionBody, key: string) {
      return readOrder(
        brandId,
        `${BASE}/bet-orders/${encodeURIComponent(id)}/abnormal`,
        { method: "POST", body, key },
      );
    },
    async judgeCancelOrder(
      brandId: string,
      id: string,
      body: JudgeCancelBody,
      key: string,
    ) {
      const result = await readOrder(
        brandId,
        `${BASE}/bet-orders/${encodeURIComponent(id)}/judge-cancel`,
        { method: "POST", body, key },
      );
      if (result.id !== id || result.brand_id !== brandId) invalidResponse();
      return result;
    },
    async getJudgment(brandId: string, id: string) {
      const v = await request<unknown>(
        brandId,
        `${BASE}/bet-orders/${encodeURIComponent(id)}/judgment`,
      );
      if (!isRecord(v)) invalidResponse();
      if (v.judgment === null) return v as { judgment: null };
      const j = v.judgment;
      if (
        !isRecord(j) ||
        !nonempty(j.id) ||
        j.brand_id !== brandId ||
        !nonempty(j.game_id) ||
        !nonempty(j.period_id) ||
        j.order_id !== id ||
        !Number.isSafeInteger(j.order_version) ||
        Number(j.order_version) < 2 ||
        (j.cause !== "no_result" && j.cause !== "invalid_result") ||
        typeof j.draw_result_id !== "string" ||
        !nonempty(j.judged_by) ||
        typeof j.reason !== "string" ||
        !j.reason.trim() ||
        new TextEncoder().encode(j.reason).length > 500 ||
        !nonempty(j.created_at) ||
        !nonempty(j.refund_entry_id)
      )
        invalidResponse();
      return v as { judgment: Judgment | null };
    },
    async getException(brandId: string, id: string) {
      const v = await request<unknown>(
        brandId,
        `${BASE}/bet-orders/${encodeURIComponent(id)}/exception`,
      );
      if (
        !isRecord(v) ||
        !(
          v.exception === null ||
          (isRecord(v.exception) &&
            nonempty(v.exception.id) &&
            nonempty(v.exception.brand_id) &&
            nonempty(v.exception.order_id) &&
            Number.isSafeInteger(v.exception.order_version) &&
            (nonempty(v.exception.marked_by) || (v.exception.marked_by === "" && v.exception.source === "system" && nonempty(v.exception.job_id) && nonempty(v.exception.error_code))) &&
            typeof v.exception.reason === "string" &&
            nonempty(v.exception.created_at))
        )
      )
        invalidResponse();
      return v as { exception: BetException | null };
    },
  };
}
export type BetManagementApi = ReturnType<typeof createBetManagementApi>;
