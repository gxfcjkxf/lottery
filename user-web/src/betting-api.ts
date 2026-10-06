import type {
  RuleDefinition,
  RuleTicketSelection,
} from "../../shared/src/rules";

export interface CatalogGame {
  id: string;
  code: string;
  name: string;
  model: RuleDefinition["model"];
  timezone: string;
  status: string;
}
export interface CatalogPlay {
  id: string;
  game_id: string;
  code: string;
  name: string;
  rule_version_id: string;
  definition_hash: string;
  definition: RuleDefinition;
}
export interface Period {
  id: string;
  game_id: string;
  period_no: string;
  status: string;
  bet_start_at: string;
  bet_end_at: string;
  draw_at: string;
  draw_result_id?: string;
}
export interface PolicyVersions {
  brand: number;
  game: number;
}
export interface EffectivePolicy {
  min_bet_points: string;
  max_bet_points: string | null;
  max_period_points: string | null;
  max_user_period_points: string | null;
  user_cancel_allowed: boolean;
}
export interface GameCatalog {
  game: CatalogGame;
  plays: CatalogPlay[];
  period: Period | null;
  server_time: string;
  brand_status: string;
  policy: EffectivePolicy;
  policy_versions: PolicyVersions;
}
export interface BetInput {
  actor_context?: string;
  period_id: string;
  play_id: string;
  rule_version_id: string;
  selection: RuleTicketSelection;
  multiplier: string;
  policy_versions?: PolicyVersions;
}
export interface BetQuote {
  actor_context: string;
  normalized: RuleTicketSelection;
  expanded_bets: RuleTicketSelection[];
  combination_count: number;
  unit_points: string;
  multiplier: string;
  bet_points: string;
  period_id: string;
  play_id: string;
  rule_version_id: string;
  definition_hash: string;
  policy: EffectivePolicy;
  policy_versions: PolicyVersions;
  period: Period;
}
export interface DeductionAllocation {
  source: string;
  state: string;
  points: string;
}
export interface BetOrder {
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
  deduction_allocation: DeductionAllocation[];
  policy_snapshot: EffectivePolicy;
  policy_versions: PolicyVersions;
  debit_entry_id: string;
  refund_entry_id?: string;
  client_key: string;
  placed_at: string;
  cancelled_at?: string;
  cancel_reason?: string;
  settlement_calculation_id?: string | null;
  payout_entry_id?: string | null;
  prize_points?: string;
  settled_at?: string | null;
}
export interface BettingApiErrorEnvelope {
  code?: string;
  message?: string;
}
interface Envelope<T> {
  success?: boolean;
  data?: T;
  error?: string | BettingApiErrorEnvelope | null;
}

export class BettingApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code?: string,
  ) {
    super(message);
    this.name = "BettingApiError";
  }
}

export function bettingBasePath(brandCode?: string): string {
  return brandCode ? `/api/v1/b/${encodeURIComponent(brandCode)}` : "/api/v1";
}

const canonicalPositiveInt64 = /^(?:[1-9]\d*)$/;
const maxInt64 = "9223372036854775807";
function validateMultiplier(value: string): void {
  if (
    !canonicalPositiveInt64.test(value) ||
    value.length > maxInt64.length ||
    (value.length === maxInt64.length && value > maxInt64)
  ) {
    throw new BettingApiError(
      "Multiplier must be a canonical positive int64",
      0,
      "invalid_multiplier",
    );
  }
}

function canonicalClone<T>(value: T): T {
  if (Array.isArray(value)) return value.map(canonicalClone) as T;
  if (value && typeof value === "object") {
    const result: Record<string, unknown> = {};
    for (const key of Object.keys(value as Record<string, unknown>).sort()) {
      const item = (value as Record<string, unknown>)[key];
      if (item !== undefined) result[key] = canonicalClone(item);
    }
    return result as T;
  }
  return value;
}

export function createBetKey(prefix = "bet"): string {
  const uuid = globalThis.crypto?.randomUUID?.();
  if (!uuid)
    throw new Error("crypto.randomUUID is required to create a bet key");
  return `${prefix}:${uuid}`;
}

export interface BetIntentSnapshot {
  body: BetInput;
  key: string;
}
export function snapshotBetIntent(
  input: BetInput,
  quote: BetQuote,
): BetIntentSnapshot {
  if (
    !quote.policy_versions ||
    input.period_id !== quote.period_id ||
    input.play_id !== quote.play_id ||
    input.rule_version_id !== quote.rule_version_id ||
    input.multiplier !== quote.multiplier
  ) {
    throw new BettingApiError(
      "Quote does not match the bet intent",
      0,
      "quote_mismatch",
    );
  }
  validateMultiplier(input.multiplier);
  return {
    body: canonicalClone({
      ...input,
      policy_versions: quote.policy_versions,
      actor_context: quote.actor_context,
    }),
    key: createBetKey(),
  };
}

export function createBettingApi(
  options: { brandCode?: string; fetcher?: typeof fetch } = {},
) {
  const brandCode =
    options.brandCode ?? import.meta.env.VITE_BRAND_CODE ?? undefined;
  const fetcher = options.fetcher ?? fetch;
  const base = bettingBasePath(brandCode);

  async function request<T>(
    path: string,
    method = "GET",
    body?: unknown,
    key?: string,
  ): Promise<T> {
    let response: Response;
    try {
      const headers = new Headers({ Accept: "application/json" });
      if (body !== undefined) headers.set("Content-Type", "application/json");
      if (key) headers.set("Idempotency-Key", key);
      response = await fetcher(`${base}${path}`, {
        method,
        credentials: "same-origin",
        headers,
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      });
    } catch (cause) {
      throw new BettingApiError(
        cause instanceof Error ? cause.message : "Network request failed",
        0,
      );
    }
    let envelope: Envelope<T>;
    try {
      envelope = (await response.json()) as Envelope<T>;
    } catch {
      throw new BettingApiError(
        response.ok
          ? "Server returned malformed JSON"
          : `Request failed (${response.status})`,
        response.status,
      );
    }
    if (
      !envelope ||
      typeof envelope !== "object" ||
      !response.ok ||
      envelope.success !== true ||
      envelope.data === undefined
    ) {
      const error =
        typeof envelope?.error === "object" && envelope.error
          ? envelope.error
          : undefined;
      throw new BettingApiError(
        typeof envelope?.error === "string"
          ? envelope.error
          : (error?.message ?? `Request failed (${response.status})`),
        response.status,
        error?.code,
      );
    }
    return envelope.data;
  }
  const pageQuery = (limit: number, offset: number) =>
    new URLSearchParams({ limit: String(limit), offset: String(offset) });
  return {
    games(limit = 50, offset = 0) {
      return request<{ items: CatalogGame[]; limit: number; offset: number }>(
        `/games?${pageQuery(limit, offset)}`,
      );
    },
    game(id: string) {
      return request<GameCatalog>(`/games/${encodeURIComponent(id)}`);
    },
    preview(input: BetInput) {
      validateMultiplier(input.multiplier);
      return request<BetQuote>("/bet-previews", "POST", canonicalClone(input));
    },
    place(input: BetInput, key: string) {
      validateMultiplier(input.multiplier);
      return request<BetOrder>(
        "/bet-orders",
        "POST",
        canonicalClone(input),
        key,
      );
    },
    orders(limit = 50, offset = 0) {
      return request<{ items: BetOrder[] }>(
        `/bet-orders?${pageQuery(limit, offset)}`,
      );
    },
    order(id: string) {
      return request<BetOrder>(`/bet-orders/${encodeURIComponent(id)}`);
    },
    cancel(id: string, body: { version: number; reason: string }, key: string) {
      return request<BetOrder>(
        `/bet-orders/${encodeURIComponent(id)}/cancel`,
        "POST",
        body,
        key,
      );
    },
  };
}
export type BettingApi = ReturnType<typeof createBettingApi>;
