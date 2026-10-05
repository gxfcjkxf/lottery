import {
  AdminApiError,
  createIdempotencyKey,
  type AdminAccount,
} from "./admin-api";
import {
  buildRuleSimulationRequest,
  DEFAULT_RULE_SIMULATION_FORM,
  type RuleDefinition,
  type RuleDraw,
  type RuleModelName,
  type RuleModel,
  type RuleSimulation,
  type RuleSimulationForm,
  type RuleTemplate,
  type RuleTicketSelection,
} from "./rule-simulation-api";

const BASE = "/api/v1/admin";
export type RuleVersionAccount = AdminAccount;
export type RuleEffectMode = "immediate" | "next_period";
export type RuleVersionStatus =
  | "draft"
  | "pending_review"
  | "approved"
  | "active"
  | "expired"
  | "rejected"
  | "rolled_back";

export interface GameRecord {
  id: string;
  brand_id: string;
  code: string;
  name: string;
  model: RuleModel;
  timezone: string;
  version: number;
  status: string;
  started_sequence?: number;
}
export interface PlayRecord {
  id: string;
  brand_id: string;
  game_id: string;
  code: string;
  name: string;
  status: string;
  active_version_id: string | null;
  version: number;
}
export interface RuleValidationCase {
  name: string;
  selection: RuleTicketSelection;
  draw: RuleDraw;
  multiplier: string;
  expected_bet_points: string;
  expected_prize_points: string;
  expected_won: boolean;
}
export interface RuleValidationReport {
  passed: boolean;
  definition_hash: string;
  cases: RuleValidationCaseReport[];
  warnings: string[];
  findings: { code: string; message: string; blocking: boolean }[];
}
export interface RuleValidationCaseReport {
  name: string;
  input?: RuleValidationCase;
  expected_bet_points: string;
  actual_bet_points: string;
  expected_prize_points: string;
  actual_prize_points: string;
  expected_won: boolean;
  actual_won: boolean;
  matched: boolean;
  simulation: RuleSimulation;
}
export interface RuleVersion {
  id: string;
  brand_id: string;
  game_id: string;
  play_id: string;
  version_no: number;
  version: number;
  definition: RuleDefinition;
  definition_hash: string;
  status: RuleVersionStatus;
  effect_mode: RuleEffectMode;
  created_by: string;
  reviewed_by: string | null;
  review_comment: string;
  created_at: string;
  updated_at: string;
  effective_at?: string | null;
  effective_period_id?: string | null;
  effective_sequence?: number | null;
  source_version_id?: string | null;
  validation?: RuleValidationReport | null;
  audit_log_id?: string;
}
export interface CreateGameBody {
  code: string;
  name: string;
  model: RuleModel;
  timezone: string;
  reason: string;
}
export interface CreatePlayBody {
  code: string;
  name: string;
  reason: string;
}
export interface CreateRuleVersionBody {
  play_id: string;
  definition: RuleDefinition;
  effect_mode: RuleEffectMode;
  reason: string;
}
export interface UpdateRuleVersionBody {
  version: number;
  definition: RuleDefinition;
  effect_mode: RuleEffectMode;
  reason: string;
}
export interface VersionReasonBody {
  version: number;
  reason: string;
}
export interface ValidateRuleVersionBody extends VersionReasonBody {
  cases: RuleValidationCase[];
}
export interface ApproveRuleVersionBody extends VersionReasonBody {
  warnings_acknowledged: boolean;
}
export interface CloneRuleVersionBody {
  effect_mode: RuleEffectMode;
  reason: string;
}

export function ruleVersionPermissions(
  account: RuleVersionAccount,
  brandId: string,
) {
  const brand = new Set(
    account.permissions_by_brand === undefined
      ? (account.permissions ?? [])
      : (account.permissions_by_brand[brandId] ?? []),
  );
  const platform = new Set(
    account.platform_permissions ?? account.permissions ?? [],
  );
  const view = (resource: "game" | "rule") =>
    Boolean(brandId) &&
    (brand.has(`${resource}.view.brand`) ||
      platform.has(`${resource}.view.platform`));
  const write = (grant: string) =>
    Boolean(brandId) && !account.super_admin && brand.has(grant);
  return {
    gamesView: view("game"),
    gamesWrite: write("game.write.brand"),
    rulesView: view("rule"),
    rulesWrite: write("rule.write.brand"),
    validate: write("rule.validate.brand"),
    submit: write("rule.submit.brand"),
    review: write("rule.review.brand"),
  };
}
export function canReviewRuleVersion(
  account: RuleVersionAccount,
  brandId: string,
  record: RuleVersion,
) {
  return (
    ruleVersionPermissions(account, brandId).review &&
    record.brand_id === brandId &&
    record.status === "pending_review" &&
    Boolean(account.id) &&
    record.created_by !== account.id
  );
}
export function canCloneRuleVersion(
  account: RuleVersionAccount,
  brandId: string,
  record: RuleVersion,
) {
  return (
    ruleVersionPermissions(account, brandId).rulesWrite &&
    record.brand_id === brandId &&
    ["active", "expired", "rolled_back"].includes(record.status)
  );
}

export function ruleVersionDefinitionLocked(record: RuleVersion): boolean {
  return Boolean(record.source_version_id) || record.status !== "draft";
}

/** One retry key per operation, brand and target; edited content rotates that scope's key. */
export function createRuleVersionKeyTracker() {
  const previous = new Map<string, { body: string; key: string }>();
  return (
    brandId: string,
    operation: string,
    targetId: string,
    body: unknown,
  ) => {
    const scope = JSON.stringify([brandId, operation, targetId]);
    const serialized = stableJson(body);
    const entry = previous.get(scope);
    if (entry?.body === serialized) return entry.key;
    const key = createIdempotencyKey();
    previous.set(scope, { body: serialized, key });
    return key;
  };
}

/** Used by each read/write lane to discard superseded, cross-brand or revoked responses. */
export function createRuleVersionRequestGuard() {
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
        for (const key of generations.keys())
          generations.set(key, generations.get(key)! + 1);
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

export function defaultRuleVersionForm(
  template: RuleTemplate,
): RuleSimulationForm {
  const form = structuredClone(DEFAULT_RULE_SIMULATION_FORM);
  if (template === "m-select-n") form.specialNumbers = "7";
  return form;
}
export function ruleTemplateModel(template: RuleTemplate): RuleModelName {
  return buildRuleSimulationRequest(template, defaultRuleVersionForm(template))
    .definition.model.model;
}

export function buildRuleValidationCase(
  template: RuleTemplate,
  form: RuleSimulationForm,
  expected: { name: string; bet: string; prize: string; won: boolean },
): RuleValidationCase {
  if (!expected.name.trim()) throw new Error("请填写验证用例名称。");
  if (
    ![expected.bet, expected.prize].every((value) =>
      /^(0|[1-9]\d*)$/.test(value),
    )
  )
    throw new Error("预期投注积分和中奖积分必须是规范非负整数字符串。");
  const { selection, draw, multiplier } = buildRuleSimulationRequest(
    template,
    form,
  );
  return {
    name: expected.name.trim(),
    selection,
    draw,
    multiplier,
    expected_bet_points: expected.bet,
    expected_prize_points: expected.prize,
    expected_won: expected.won,
  };
}

function stableJson(value: unknown): string {
  function sorted(input: unknown): unknown {
    if (Array.isArray(input)) return input.map(sorted);
    if (input && typeof input === "object")
      return Object.fromEntries(
        Object.entries(input)
          .sort(([a], [b]) => a.localeCompare(b))
          .map(([key, item]) => [key, sorted(item)]),
      );
    return input;
  }
  return JSON.stringify(sorted(value));
}

/** Normalize only Go's omitted empty pool fields and empty collections, preserving other fields. */
export function ruleDefinitionSignature(definition: RuleDefinition): string {
  const normalizedPool = (value: RuleDefinition["model"]["regular_pool"]) => ({
    ...value,
    min: value.min ?? 0,
    max: value.max ?? 0,
    values: value.values ?? [],
  });
  return stableJson({
    ...definition,
    model: {
      ...definition.model,
      regular_pool: normalizedPool(definition.model.regular_pool),
      special_pool: normalizedPool(definition.model.special_pool),
    },
    selection: {
      ...definition.selection,
      attribute_groups: definition.selection.attribute_groups ?? [],
      feature_choices: definition.selection.feature_choices ?? {},
    },
    number_attributes: definition.number_attributes ?? {},
  });
}

/** Only offer editable controls if rebuilding the stored definition is lossless. */
export function restoreRuleVersionTemplate(
  definition: RuleDefinition,
): { template: RuleTemplate; form: RuleSimulationForm } | null {
  try {
    const templates: RuleTemplate[] = [
      "special",
      "digits",
      "features",
      "exclude",
      "attributes",
      "m-select-n",
    ];
    for (const template of templates) {
      try {
        const form = defaultRuleVersionForm(template);
        const tier = definition.prize_tiers[0];
        if (!tier) return null;
        form.unitPoints = definition.unit_points;
        form.odds = tier.odds;
        form.capPoints = definition.cap_points ?? "";
        form.maxBetPoints = definition.limits.max_bet_points ?? "";
        form.roundingScope = definition.rounding_scope;
        if (tier.condition.op === "all" || tier.condition.op === "any")
          form.conditionJoin = tier.condition.op;
        if (template === "attributes") {
          form.redNumbers = (
            definition.number_attributes?.color?.red ?? []
          ).join(",");
          form.blueNumbers = (
            definition.number_attributes?.color?.blue ?? []
          ).join(",");
        }
        if (template === "features") {
          for (const key of Object.keys(form.featureValues)) {
            const choices = definition.selection.feature_choices?.[key];
            form.featureValues[key] = choices?.length
              ? choices.includes(Number(form.featureValues[key]))
                ? form.featureValues[key]
                : String(choices[0])
              : "";
          }
        }
        if (template === "exclude") {
          const count = definition.selection.exclude_count;
          if (!Number.isInteger(count) || count < 1 || count > 49) continue;
          if (count !== 3)
            form.excludedNumbers = Array.from(
              { length: count },
              (_, index) => index + 1,
            ).join(",");
        }
        const rebuilt = buildRuleSimulationRequest(template, form).definition;
        if (
          ruleDefinitionSignature(rebuilt) ===
          ruleDefinitionSignature(definition)
        )
          return { template, form };
      } catch {
        continue;
      }
    }
  } catch {
    /* Unsupported definitions stay read-only; they are never rewritten from guesses. */
  }
  return null;
}

export function createRuleVersionsApi(fetcher: typeof fetch = fetch) {
  async function request<T>(
    brandId: string,
    path: string,
    method: "GET" | "POST" | "PUT" = "GET",
    body?: unknown,
    idempotencyKey?: string,
  ): Promise<T> {
    if (!brandId.trim()) throw new AdminApiError("请先选择品牌。", 0);
    const headers = new Headers({
      Accept: "application/json",
      "X-Brand-ID": brandId,
    });
    if (body !== undefined) {
      headers.set("Content-Type", "application/json");
      headers.set("Idempotency-Key", idempotencyKey ?? createIdempotencyKey());
    }
    const response = await fetcher(`${BASE}${path}`, {
      method,
      credentials: "same-origin",
      headers,
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    });
    let envelope: {
      success?: boolean;
      data?: T;
      error?: string | { code?: string; message?: string } | null;
    } | null;
    try {
      envelope = await response.json();
    } catch {
      throw new AdminApiError("服务器未返回有效 JSON。", response.status);
    }
    if (
      !response.ok ||
      !envelope ||
      typeof envelope !== "object" ||
      Array.isArray(envelope) ||
      envelope.success !== true ||
      envelope.data == null ||
      typeof envelope.data !== "object" ||
      Array.isArray(envelope.data)
    ) {
      const detail = envelope?.error;
      throw new AdminApiError(
        typeof detail === "string"
          ? detail
          : (detail?.message ?? `请求失败（${response.status}）`),
        response.status,
        typeof detail === "object" && detail ? detail.code : undefined,
      );
    }
    return envelope.data;
  }
  const id = encodeURIComponent;
  return {
    getGames: (brandId: string) =>
      request<{ games: GameRecord[] }>(brandId, "/games"),
    createGame: (brandId: string, body: CreateGameBody, key?: string) =>
      request<GameRecord>(brandId, "/games", "POST", body, key),
    getPlays: (brandId: string, gameId: string) =>
      request<{ plays: PlayRecord[] }>(brandId, `/games/${id(gameId)}/plays`),
    createPlay: (
      brandId: string,
      gameId: string,
      body: CreatePlayBody,
      key?: string,
    ) =>
      request<PlayRecord>(
        brandId,
        `/games/${id(gameId)}/plays`,
        "POST",
        body,
        key,
      ),
    getRuleVersions: (brandId: string, playId: string) =>
      request<{ versions: RuleVersion[] }>(
        brandId,
        `/plays/${id(playId)}/rule-versions`,
      ),
    createRuleVersion: (
      brandId: string,
      body: CreateRuleVersionBody,
      key?: string,
    ) => request<RuleVersion>(brandId, "/rule-versions", "POST", body, key),
    updateRuleVersion: (
      brandId: string,
      versionId: string,
      body: UpdateRuleVersionBody,
      key?: string,
    ) =>
      request<RuleVersion>(
        brandId,
        `/rule-versions/${id(versionId)}`,
        "PUT",
        body,
        key,
      ),
    validateRuleVersion: (
      brandId: string,
      versionId: string,
      body: ValidateRuleVersionBody,
      key?: string,
    ) =>
      request<RuleVersion>(
        brandId,
        `/rule-versions/${id(versionId)}/validate`,
        "POST",
        body,
        key,
      ),
    submitRuleVersion: (
      brandId: string,
      versionId: string,
      body: VersionReasonBody,
      key?: string,
    ) =>
      request<RuleVersion>(
        brandId,
        `/rule-versions/${id(versionId)}/submit-review`,
        "POST",
        body,
        key,
      ),
    approveRuleVersion: (
      brandId: string,
      versionId: string,
      body: ApproveRuleVersionBody,
      key?: string,
    ) =>
      request<RuleVersion>(
        brandId,
        `/rule-versions/${id(versionId)}/approve`,
        "POST",
        body,
        key,
      ),
    rejectRuleVersion: (
      brandId: string,
      versionId: string,
      body: VersionReasonBody,
      key?: string,
    ) =>
      request<RuleVersion>(
        brandId,
        `/rule-versions/${id(versionId)}/reject`,
        "POST",
        body,
        key,
      ),
    cloneRuleVersion: (
      brandId: string,
      versionId: string,
      body: CloneRuleVersionBody,
      key?: string,
    ) =>
      request<RuleVersion>(
        brandId,
        `/rule-versions/${id(versionId)}/clone`,
        "POST",
        body,
        key,
      ),
  };
}
