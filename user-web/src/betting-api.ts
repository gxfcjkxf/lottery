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
  prize_points: string;
  settled_at?: string | null;
}
export interface BettingApiErrorEnvelope {
  code?: string;
  message?: string;
}
interface Envelope<T> {
  success?: boolean;
  data?: T;
  error?: BettingApiErrorEnvelope | null;
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

function invalidResponse(): never {
  throw new BettingApiError('Server returned an invalid current betting response', 502, 'invalid_response');
}
function checkedPage<T extends { items: unknown[] }>(value: T): T {
  if (!value || Array.isArray(value) || !Array.isArray(value.items)) invalidResponse();
  return value;
}
function checkedCatalogGame(value: CatalogGame): CatalogGame {
  if (!value || !value.model || typeof value.model !== 'object' ||
    !['X_PLUS_Y', 'M_SELECT_N', 'DIGITS_0_9'].includes(value.model.model)) invalidResponse();
  return value;
}
function checkedGameCatalog(value: GameCatalog): GameCatalog {
  if (!value || !Array.isArray(value.plays) || !Object.hasOwn(value, 'period') ||
    (value.period !== null && (!value.period || typeof value.period.period_no !== 'string')) || !value.policy ||
    typeof value.brand_status !== 'string' || typeof value.server_time !== 'string' ||
    !Number.isFinite(Date.parse(value.server_time)) || !value.policy_versions) invalidResponse();
  checkedCatalogGame(value.game);
  return value;
}
function checkedOrder(value: BetOrder): BetOrder {
  const fields = ['unit_points', 'multiplier', 'total_points', 'prize_points'] as const;
  if (!value || fields.some(field => {
    const amount = value[field];
    return typeof amount !== 'string' || !(field === 'prize_points' ? /^(?:0|[1-9]\d*)$/ : canonicalPositiveInt64).test(amount);
  })) invalidResponse();
  return value;
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
    } catch { invalidResponse(); }
    if (!envelope || typeof envelope !== 'object') invalidResponse();
    if (!response.ok) {
      if (envelope.success !== false || typeof envelope.error?.code !== 'string' ||
        typeof envelope.error?.message !== 'string') invalidResponse();
      throw new BettingApiError(envelope.error.message, response.status, envelope.error.code);
    }
    if (envelope.success !== true || envelope.data == null) invalidResponse();
    return envelope.data;
  }
  const pageQuery = (limit: number, offset: number) =>
    new URLSearchParams({ limit: String(limit), offset: String(offset) });
  return {
    games(limit = 50, offset = 0) {
      return request<{ items: CatalogGame[]; limit: number; offset: number }>(
        `/games?${pageQuery(limit, offset)}`,
      ).then(value => { checkedPage(value); value.items.forEach(checkedCatalogGame); return value; });
    },
    game(id: string) {
      return request<GameCatalog>(`/games/${encodeURIComponent(id)}`).then(checkedGameCatalog);
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
      ).then(checkedOrder);
    },
    orders(limit = 50, offset = 0) {
      return request<{ items: BetOrder[] }>(
        `/bet-orders?${pageQuery(limit, offset)}`,
      ).then(value => { checkedPage(value); value.items.forEach(checkedOrder); return value; });
    },
    order(id: string) {
      return request<BetOrder>(`/bet-orders/${encodeURIComponent(id)}`).then(checkedOrder);
    },
    cancel(id: string, body: { version: number; reason: string }, key: string) {
      return request<BetOrder>(
        `/bet-orders/${encodeURIComponent(id)}/cancel`,
        "POST",
        body,
        key,
      ).then(checkedOrder);
    },
  };
}
export type BettingApi = ReturnType<typeof createBettingApi>;
