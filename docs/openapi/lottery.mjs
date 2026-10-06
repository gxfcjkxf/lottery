const ref = (name) => ({ $ref: `#/components/schemas/${name}` });
const arr = (items) => ({ type: "array", items });
const obj = (properties, required = Object.keys(properties), extra = false) => ({
  type: "object", properties, required, additionalProperties: extra,
});
const nullable = (schema) => ({ anyOf: [schema, { type: "null" }] });
const str = { type: "string" };
const int = { type: "integer" };
const bool = { type: "boolean" };
const uuid = ref("UUID");
const dateTime = ref("DateTime");
const reason = ref("Reason");
const amount = ref("NonnegativeInt64String");
const positiveAmount = ref("PositiveInt64String");

const pageQuery = [
  { name: "limit", in: "query", schema: { type: "integer", minimum: 1, maximum: 100 } },
  { name: "offset", in: "query", schema: { type: "integer", minimum: 0 } },
];
const adminPerm = (key, method, description) => {
  const scopes = method === "GET" || key === "rule.simulate" ? ["brand", "platform"] : ["brand"];
  const grants = scopes.map((scope) => `${key}.${scope}`);
  return {
    permissions: grants,
    description: description ?? (scopes.length > 1
      ? `Read access accepts either ${grants.join(" or ")}; the actual handler applies its own brand and platform scope rules.`
      : `${grants[0]} is required. SUPER_ADMIN cannot use brand-scoped write actions.`),
  };
};
const op = (method, path, operationId, summary, tag, auth, data, extra = {}) => ({
  method, path, operationId, summary, tag, auth, data, ...extra,
});
const admin = (method, path, operationId, summary, data, permission, extra = {}) => op(
  method, `/api/v1/admin${path}`, operationId, summary, "games", "admin", data,
  { brandHeader: true, ...adminPerm(permission, method, extra.description), ...extra },
);
const mutation = (body, status = 200) => ({ requestBody: body, idempotency: true, successStatus: status });

const RuleCondition = obj({
  op: str, field: str, target: str, position: nullable(int), attribute_group: str,
  attribute_value: str, selection_key: str, value: nullable(int), values: nullable(arr(int)),
  min: nullable(int), max: nullable(int), children: nullable(arr(ref("LotteryRuleCondition"))),
}, ["op"]);
const RulePool = obj({ min: int, max: int, values: arr(int), allow_repeat: bool }, ["allow_repeat"]);
const RulePoolInput = obj({ min: int, max: int, values: nullable(arr(int)), allow_repeat: bool }, []);
const RuleModel = obj({
  model: str, regular_pool: ref("LotteryRulePool"), special_pool: ref("LotteryRulePool"),
  regular_count: int, special_count: int, pool_size: int, total_count: int,
  length: int, allow_repeat: bool, ordered: bool,
});
const RuleModelInput = obj({
  model: str, regular_pool: nullable(ref("LotteryRulePoolInput")), special_pool: nullable(ref("LotteryRulePoolInput")),
  regular_count: int, special_count: int, pool_size: int, total_count: int,
  length: int, allow_repeat: bool, ordered: bool,
}, ["model"]);
const RuleSelection = obj({
  regular: nullable(arr(int)), special: nullable(arr(int)), digits: nullable(arr(arr(int))), exclude: nullable(arr(int)),
  attributes: nullable({ type: "object", additionalProperties: nullable(arr(str)) }),
  features: nullable({ type: "object", additionalProperties: nullable(arr(int)) }),
});
const RuleSelectionInput = obj({
  regular: nullable(arr(int)), special: nullable(arr(int)), digits: nullable(arr(arr(int))), exclude: nullable(arr(int)),
  attributes: nullable({ type: "object", additionalProperties: nullable(arr(str)) }),
  features: nullable({ type: "object", additionalProperties: nullable(arr(int)) }),
}, []);
const RuleDraw = obj({ regular: nullable(arr(int)), special: nullable(arr(int)), digits: nullable(arr(int)) });
const RuleDrawInput = obj({ regular: nullable(arr(int)), special: nullable(arr(int)), digits: nullable(arr(int)) }, []);
const RuleTier = obj({
  code: str, condition: ref("LotteryRuleCondition"), odds: str, exclusive: bool,
  cap_points: nullable(positiveAmount),
});
const RuleTierInput = obj({ code: str, condition: ref("LotteryRuleCondition"), odds: str, exclusive: bool, cap_points: nullable(positiveAmount) }, ["code", "condition", "odds"]);
const RuleSelectionRuleInput = obj({ mode: str, regular_count: int, special_count: int, exclude_count: int,
  attribute_groups: nullable(arr(str)), feature_choices: nullable({ type: "object", additionalProperties: nullable(arr(int)) }) }, ["mode"]);
const RuleLimitsInput = obj({ max_combinations: int, max_multiplier: positiveAmount, max_bet_points: nullable(positiveAmount) }, ["max_combinations", "max_multiplier"]);
const RuleDefinition = obj({
  schema_version: int, model: ref("LotteryRuleModel"),
  selection: obj({ mode: str, regular_count: int, special_count: int, exclude_count: int,
    attribute_groups: nullable(arr(str)), feature_choices: nullable({ type: "object", additionalProperties: nullable(arr(int)) }) }),
  number_attributes: nullable({ type: "object", additionalProperties: { type: "object", additionalProperties: nullable(arr(int)) } }),
  unit_points: positiveAmount, prize_tiers: arr(ref("LotteryRuleTier")), mixed_tier_policy: str,
  cap_points: nullable(positiveAmount), rounding: str, rounding_scope: str,
  limits: obj({ max_combinations: int, max_multiplier: positiveAmount, max_bet_points: nullable(positiveAmount) }),
});
const RuleDefinitionInput = obj({
  schema_version: { type: "integer", const: 1 }, model: ref("LotteryRuleModelInput"),
  selection: ref("LotteryRuleSelectionRuleInput"), number_attributes: nullable({ type: "object", additionalProperties: { type: "object", additionalProperties: nullable(arr(int)) } }),
  unit_points: positiveAmount, prize_tiers: arr(ref("LotteryRuleTierInput")), mixed_tier_policy: str,
  cap_points: nullable(positiveAmount), rounding: { type: "string", const: "half_up" },
  rounding_scope: { type: "string", enum: ["order", "line", "tier"] }, limits: ref("LotteryRuleLimitsInput"),
}, ["schema_version", "model", "selection", "unit_points", "prize_tiers", "rounding", "rounding_scope", "limits"]);
const SimulationInput = obj({ definition: ref("LotteryRuleDefinitionInput"), selection: ref("LotteryRuleSelectionInput"), draw: ref("LotteryRuleDrawInput"), multiplier: positiveAmount });
const RuleTrace = obj({ op: str, field: str, actual: int, matched: bool, children: arr(ref("LotteryRuleTrace")) }, ["op", "actual", "matched"]);
const RuleTierHit = obj({ code: str, exclusive: bool, matched: bool, selected: bool, raw_points: str, capped_points: str, points: str, trace: ref("LotteryRuleTrace") });
const RuleLine = obj({ selection: ref("LotteryRuleSelection"), hits: arr(ref("LotteryRuleTierHit")), points: str });
const SimulationResult = obj({ won: bool, normalized: ref("LotteryRuleSelection"), combination_count: int, multiplier: amount, bet_points: amount, prize_points: amount, raw_prize_points: str, capped_prize_points: str, lines: arr(ref("LotteryRuleLine")), warnings: arr(str) });
const ValidationCase = obj({ name: str, selection: ref("LotteryRuleSelection"), draw: ref("LotteryRuleDraw"), multiplier: positiveAmount, expected_bet_points: nullable(amount), expected_prize_points: nullable(amount), expected_won: nullable(bool) }, ["name", "selection", "draw", "multiplier", "expected_bet_points", "expected_prize_points", "expected_won"]);
const ValidationCaseInput = obj({ name: str, selection: ref("LotteryRuleSelectionInput"), draw: ref("LotteryRuleDrawInput"), multiplier: positiveAmount, expected_bet_points: nullable(amount), expected_prize_points: nullable(amount), expected_won: nullable(bool) }, ["name", "selection", "draw", "multiplier", "expected_bet_points", "expected_prize_points", "expected_won"]);
const ValidationCaseReport = obj({ name: str, input: ref("LotteryValidationCase"), expected_bet_points: amount, actual_bet_points: amount, expected_prize_points: amount, actual_prize_points: amount, expected_won: bool, actual_won: bool, matched: bool, simulation: ref("LotterySimulationResult") });
const ValidationReport = obj({
  definition_hash: str, passed: bool, cases: arr(ref("LotteryValidationCaseReport")), warnings: arr(str),
  findings: arr(obj({ code: str, message: str, blocking: bool }, ["code", "message", "blocking"])),
});

const Game = obj({ id: uuid, brand_id: uuid, code: str, name: str, model: ref("LotteryRuleModel"), timezone: str, status: str, version: int, started_sequence: int });
const Play = obj({ id: uuid, brand_id: uuid, game_id: uuid, code: str, name: str, status: str, active_version_id: str, version: int });
const RuleVersion = obj({
  id: uuid, brand_id: uuid, game_id: uuid, play_id: uuid, version_no: int, version: int,
  definition: ref("LotteryRuleDefinition"), definition_hash: str, status: str, effect_mode: str,
  created_by: uuid, reviewed_by: str, review_comment: str, created_at: dateTime, updated_at: dateTime,
  effective_at: dateTime, effective_period_id: uuid, effective_sequence: int,
  source_version_id: uuid, validation: ref("LotteryValidationReport"), audit_log_id: uuid,
}, ["id", "brand_id", "game_id", "play_id", "version_no", "version", "definition", "definition_hash", "status", "effect_mode", "created_by", "reviewed_by", "review_comment", "created_at", "updated_at"]);
const Period = obj({ id: uuid, brand_id: uuid, game_id: uuid, period_no: str, sequence: int, bet_start_at: dateTime, bet_end_at: dateTime, draw_at: dateTime, status: str, version: int, schedule_id: str, state_reason: str }, ["id", "brand_id", "game_id", "period_no", "sequence", "bet_start_at", "bet_end_at", "draw_at", "status", "version", "state_reason"]);
const BusyWindow = obj({ start: str, end: str, interval_seconds: int });
const ScheduleSpec = obj({ timezone: str, mode: str, daily_draw_times: arr(str), interval_seconds: int, busy_windows: arr(ref("LotteryBusyWindow")), bet_open_before_seconds: int, bet_close_before_seconds: int, pause_dates: arr(str), weekdays: arr(int), holiday_dates: arr(str), holiday_policy: str });
const ScheduleRecord = obj({ id: uuid, brand_id: uuid, game_id: uuid, revision: int, spec: ref("LotteryScheduleSpec"), game_version: int, created_at: dateTime });
const Generation = obj({ created: int, existing: int, periods: arr(ref("LotteryPeriod")) });
const DrawSource = obj({ id: uuid, name: str, type: { type: "string", enum: ["api", "dom"] }, priority: int, enabled: bool, endpoint: str, selector: str, credential_ref: str });
const DrawSources = obj({ id: uuid, brand_id: uuid, game_id: uuid, revision: int, game_version: int, created_at: dateTime, sources: arr(ref("LotteryDrawSource")) });
const DrawResult = obj({ id: uuid, brand_id: uuid, game_id: uuid, period_id: uuid, source_id: str, kind: str, result: ref("LotteryRuleDraw"), result_hash: str, drawn_at: dateTime, created_at: dateTime, created_by: str, corrected_from_id: str }, ["id", "brand_id", "game_id", "period_id", "source_id", "kind", "result", "result_hash", "drawn_at", "created_at"]);
const DrawAttempt = obj({ source_id: str, status: str, code: str });
const DrawAttemptBatch = obj({ id: uuid, brand_id: uuid, game_id: uuid, period_id: uuid, source_set_id: uuid, observed_period_version: int, status: str, attempts: arr(ref("LotteryDrawAttempt")), created_at: dateTime });
const DrawHistory = obj({ current: nullable(ref("LotteryDrawResult")), history: arr(ref("LotteryDrawResult")), attempts: arr(ref("LotteryDrawAttemptBatch")), limit: int, offset: int });
const BrandPolicy = obj({ min_bet_points: positiveAmount, max_bet_points: nullable(positiveAmount), max_period_points: nullable(positiveAmount), max_user_period_points: nullable(positiveAmount), user_cancel_allowed: bool });
const LimitOverride = obj({ mode: { type: "string", enum: ["inherit", "unlimited", "value"] }, points: nullable(positiveAmount) });
const GamePolicy = obj({ min_bet_points: ref("LotteryLimitOverride"), max_bet_points: ref("LotteryLimitOverride"), max_period_points: ref("LotteryLimitOverride"), max_user_period_points: ref("LotteryLimitOverride"), user_cancel_allowed: nullable(bool) });
const EffectivePolicy = obj({ min_bet_points: amount, max_bet_points: nullable(amount), max_period_points: nullable(amount), max_user_period_points: nullable(amount), user_cancel_allowed: bool });
const PolicyVersions = obj({ brand: int, game: int });
const CatalogGame = obj({ id: uuid, code: str, name: str, model: ref("LotteryRuleModel"), timezone: str, status: str });
const CatalogPlay = obj({ id: uuid, game_id: uuid, code: str, name: str, rule_version_id: uuid, definition_hash: str, definition: ref("LotteryRuleDefinition") });
const BetPeriod = obj({ id: uuid, game_id: uuid, period_no: str, status: str, bet_start_at: dateTime, bet_end_at: dateTime, draw_at: dateTime, draw_result_id: str }, ["id", "game_id", "period_no", "status", "bet_start_at", "bet_end_at", "draw_at"]);
const Catalog = obj({ game: ref("LotteryCatalogGame"), plays: arr(ref("LotteryCatalogPlay")), period: nullable(ref("LotteryBetPeriod")), server_time: dateTime, brand_status: str, policy: ref("LotteryEffectivePolicy"), policy_versions: ref("LotteryPolicyVersions") });
const BetInput = obj({ period_id: uuid, play_id: uuid, rule_version_id: uuid, selection: ref("LotteryRuleSelectionInput"), multiplier: positiveAmount, policy_versions: nullable(ref("LotteryPolicyVersions")), actor_context: str }, ["period_id", "play_id", "rule_version_id", "selection", "multiplier"]);
const BetQuote = obj({ normalized: ref("LotteryRuleSelection"), expanded_bets: arr(ref("LotteryRuleSelection")), combination_count: int, unit_points: amount, multiplier: amount, bet_points: amount, period_id: uuid, play_id: uuid, rule_version_id: uuid, definition_hash: str, policy: ref("LotteryEffectivePolicy"), policy_versions: ref("LotteryPolicyVersions"), period: ref("LotteryBetPeriod") });
const Allocation = obj({ source: str, state: str, points: amount });
const BetOrder = obj({
  id: uuid, brand_id: uuid, global_user_id: uuid, brand_member_id: uuid, account_id: uuid,
  game_id: uuid, period_id: uuid, play_id: uuid, rule_version_id: uuid, definition_hash: str,
  definition_snapshot: ref("LotteryRuleDefinition"), status: str, version: int, selection_raw: ref("LotteryRuleSelection"),
  selection_normalized: ref("LotteryRuleSelection"), expanded_bets: arr(ref("LotteryRuleSelection")), unit_points: amount,
  combination_count: int, multiplier: amount, total_points: amount, deduction_allocation: arr(ref("LotteryPointAllocation")),
  policy_snapshot: ref("LotteryEffectivePolicy"), policy_versions: ref("LotteryPolicyVersions"), debit_entry_id: uuid,
  refund_entry_id: str, client_key: str, placed_at: dateTime, cancelled_at: dateTime, cancel_reason: str,
  settlement_calculation_id: nullable(uuid), payout_entry_id: nullable(uuid), prize_points: amount, settled_at: dateTime,
}, ["id", "brand_id", "global_user_id", "brand_member_id", "account_id", "game_id", "period_id", "play_id", "rule_version_id", "definition_hash", "definition_snapshot", "status", "version", "selection_raw", "selection_normalized", "expanded_bets", "unit_points", "combination_count", "multiplier", "total_points", "deduction_allocation", "policy_snapshot", "policy_versions", "debit_entry_id", "client_key", "placed_at", "settlement_calculation_id", "payout_entry_id", "prize_points", "settled_at"]);
const PublicDraw = obj({ id: uuid, game: ref("LotteryCatalogGame"), period: ref("LotteryBetPeriod"), result: ref("LotteryRuleDraw"), drawn_at: dateTime, origin: str });
const PublicDrawPage = obj({ items: arr(ref("LotteryPublicDraw")), limit: int, offset: int, has_more: bool, server_time: dateTime, brand_status: str });
const PublicPeriod = obj({ period: ref("LotteryBetPeriod"), draw: nullable(ref("LotteryPublicDraw")) });
const PublicPeriodPage = obj({ game: ref("LotteryCatalogGame"), items: arr(ref("LotteryPublicPeriod")), limit: int, offset: int, has_more: bool, server_time: dateTime, brand_status: str });
const SettlementPolicy = obj({ brand_id: uuid, version: int, mode: nullable(str), updated_at: dateTime, audit_log_id: str }, ["brand_id", "version", "mode", "updated_at"]);
const BetException = obj({ id: uuid, brand_id: uuid, order_id: uuid, order_version: int, marked_by: uuid, source: str, job_id: nullable(uuid), error_code: nullable(str), reason, created_at: dateTime });
const BetJudgment = obj({ id: uuid, brand_id: uuid, game_id: uuid, period_id: uuid, order_id: uuid, order_version: int, cause: str, draw_result_id: str, judged_by: uuid, reason, created_at: dateTime, refund_entry_id: str });
const SettlementContext = obj({ brand_id: uuid, game_id: uuid, period_id: uuid, order_id: uuid, order_version: int, order_status: str, definition_hash: str, period_version: int, period_status: str, draw_result_id: nullable(uuid), draw_hash: nullable(str), draw: nullable(ref("LotteryRuleDraw")), can_preview: bool });
const SettlementTarget = obj({ order_id: uuid, member_id: uuid, state: str, version: int, calculation_id: nullable(uuid), order_version: int, order_status: str, won: nullable(bool), prize_points: nullable(amount), payout_entry_id: nullable(uuid), error_code: nullable(str) });
const CorrectionTarget = obj({ order_id: uuid, member_id: uuid, state: str, version: int, old_order_version: int, old_order_status: str, old_calculation_id: nullable(uuid), old_payout_entry_id: nullable(uuid), old_prize_points: amount, reversal_entry_id: nullable(uuid), reset_order_version: nullable(int), error_code: nullable(str) });
const NotificationDelivery = obj({ event_id: uuid, brand_id: uuid, status: str, attempt_count: int, last_error: nullable(str), next_attempt_at: dateTime, sent_at: nullable(dateTime) });
const SettlementJob = obj({ id: uuid, brand_id: uuid, game_id: uuid, period_id: uuid, draw_result_id: uuid, period_version: int, policy_version: int, mode: str, state: str, version: int, target_count: int, created_by: uuid, approved_by: nullable(uuid), reason, created_at: dateTime, completed_at: nullable(dateTime), last_error_code: nullable(str), pending_count: int, ready_count: int, paid_count: int, excluded_count: int, failed_count: int, prize_points: amount, paid_points: amount, can_retry: bool, generation: int, previous_job_id: nullable(uuid), correction_id: nullable(uuid), current: bool });
const SettlementPreview = obj({ id: uuid, brand_id: uuid, game_id: uuid, period_id: uuid, order_id: uuid, order_version: int, order_status: str, period_version: int, period_status: str, draw_result_id: uuid, definition_hash: str, draw_hash: str, draw: ref("LotteryRuleDraw"), outcome: str, error_code: nullable(str), calculation: nullable(obj({ won: bool, combination_count: int, multiplier: amount, bet_points: amount, prize_points: amount, raw_prize_points: str, capped_prize_points: str })), created_by: uuid, created_at: dateTime, reason, audit_log_id: uuid, current: bool, applied: bool });
const Correction = obj({ id: uuid, brand_id: uuid, game_id: uuid, period_id: uuid, previous_draw_result_id: uuid, draw_result_id: uuid, result: ref("LotteryRuleDraw"), period_version: int, previous_job_id: nullable(uuid), new_job_id: nullable(uuid), policy_version: nullable(int), mode: nullable(str), state: str, version: int, target_count: int, created_by: uuid, reason, created_at: dateTime, completed_at: nullable(dateTime), last_error_code: nullable(str), pending_count: int, reversed_count: int, unchanged_count: int, excluded_count: int, failed_count: int, reverse_points: amount, reversed_points: amount, can_retry: bool, new_job_state: nullable(str), new_job_version: nullable(int), new_job_error_code: nullable(str) });
const CancelJob = obj({ id: uuid, brand_id: uuid, game_id: uuid, period_id: uuid, period_version: int, mode: str, cause: str, state: str, version: int, reason, created_by: uuid, created_at: dateTime, completed_at: nullable(dateTime), last_error_code: str, total_count: int, pending_count: int, refunded_count: int, already_refunded_count: int, failed_count: int });
const Notification = obj({ id: uuid, brand_id: uuid, member_id: uuid, event_type: str, template_key: str, template_version: { type: "integer", enum: [1] }, payload: obj({ resource_id: uuid, points: nullable(amount) }), created_at: dateTime, read_at: nullable(dateTime) });

export const schemas = {
  LotteryRuleCondition: RuleCondition, LotteryRulePool: RulePool, LotteryRuleModel: RuleModel,
  LotteryRuleSelection: RuleSelection, LotteryRuleDraw: RuleDraw, LotteryRuleTier: RuleTier,
  LotteryRuleDefinition: RuleDefinition, LotteryRuleDefinitionInput: RuleDefinitionInput, LotterySimulationInput: SimulationInput,
  LotteryRulePoolInput: RulePoolInput, LotteryRuleModelInput: RuleModelInput, LotteryRuleSelectionInput: RuleSelectionInput,
  LotteryRuleDrawInput: RuleDrawInput, LotteryRuleTierInput: RuleTierInput, LotteryRuleSelectionRuleInput: RuleSelectionRuleInput,
  LotteryRuleLimitsInput: RuleLimitsInput,
  LotterySimulationResult: SimulationResult, LotteryRuleTrace: RuleTrace, LotteryRuleTierHit: RuleTierHit,
  LotteryRuleLine: RuleLine, LotteryValidationCase: ValidationCase, LotteryValidationCaseInput: ValidationCaseInput, LotteryValidationCaseReport: ValidationCaseReport, LotteryValidationReport: ValidationReport,
  LotteryGame: Game, LotteryPlay: Play, LotteryRuleVersion: RuleVersion, LotteryPeriod: Period,
  LotteryBusyWindow: BusyWindow, LotteryScheduleSpec: ScheduleSpec, LotteryScheduleRecord: ScheduleRecord, LotteryPeriodGeneration: Generation,
  LotteryDrawSource: DrawSource, LotteryDrawSources: DrawSources, LotteryDrawResult: DrawResult,
  LotteryDrawAttempt: DrawAttempt, LotteryDrawAttemptBatch: DrawAttemptBatch, LotteryDrawHistory: DrawHistory,
  LotteryBrandPolicyConfig: BrandPolicy, LotteryLimitOverride: LimitOverride,
  LotteryGamePolicyConfig: GamePolicy, LotteryEffectivePolicy: EffectivePolicy, LotteryPolicyVersions: PolicyVersions,
  LotteryCatalogGame: CatalogGame, LotteryCatalogPlay: CatalogPlay, LotteryBetPeriod: BetPeriod,
  LotteryGameCatalog: Catalog, LotteryBetInput: BetInput, LotteryBetQuote: BetQuote, LotteryBetOrder: BetOrder,
  LotteryPublicDraw: PublicDraw, LotteryPublicDrawPage: PublicDrawPage, LotteryPublicPeriod: PublicPeriod,
  LotteryPublicPeriodPage: PublicPeriodPage, LotterySettlementPolicy: SettlementPolicy,
  LotteryBetException: BetException, LotteryBetJudgment: BetJudgment,
  LotterySettlementContext: SettlementContext, LotterySettlementTarget: SettlementTarget,
  LotteryCorrectionTarget: CorrectionTarget, LotteryNotificationDelivery: NotificationDelivery,
  LotteryPointAllocation: Allocation,
  LotterySettlementJob: SettlementJob, LotterySettlementPreview: SettlementPreview,
  LotteryCorrection: Correction, LotteryPeriodCancellation: CancelJob, LotteryNotification: Notification,
};

export const operations = [
  admin("GET", "/games", "adminListGames", "List brand games", ref("LotteryAdminGamesPage"), "game.view", { parameters: pageQuery }),
  admin("POST", "/games", "adminCreateGame", "Create a game", ref("LotteryGame"), "game.write", mutation(obj({ code: str, name: str, model: ref("LotteryRuleModelInput"), timezone: str, reason }, ["code", "name", "model", "timezone", "reason"]), 201)),
  admin("GET", "/games/{id}/plays", "adminListGamePlays", "List plays for a game", ref("LotteryAdminPlaysPage"), "game.view", { parameters: pageQuery }),
  admin("POST", "/games/{id}/plays", "adminCreateGamePlay", "Create a play", ref("LotteryPlay"), "game.write", mutation(obj({ code: str, name: str, reason }, ["code", "name", "reason"]), 201)),
  admin("GET", "/plays/{id}/rule-versions", "adminListPlayRuleVersions", "List rule versions for a play", ref("LotteryAdminRuleVersionsPage"), "rule.view", { parameters: pageQuery }),
  admin("GET", "/rule-versions/{id}", "adminGetRuleVersion", "Get a rule version", ref("LotteryRuleVersion"), "rule.view"),
  admin("POST", "/rule-versions", "adminCreateRuleVersion", "Create a draft rule version", ref("LotteryRuleVersion"), "rule.write", mutation(obj({ play_id: uuid, definition: ref("LotteryRuleDefinitionInput"), effect_mode: str, reason }, ["play_id", "definition", "effect_mode", "reason"]), 201)),
  admin("PUT", "/rule-versions/{id}", "adminUpdateRuleVersion", "Update a draft rule version", ref("LotteryRuleVersion"), "rule.write", mutation(obj({ version: int, definition: ref("LotteryRuleDefinitionInput"), effect_mode: str, reason }, ["version", "definition", "effect_mode", "reason"]))),
  admin("POST", "/rule-versions/{id}/validate", "adminValidateRuleVersion", "Validate a draft using supplied cases", ref("LotteryRuleVersion"), "rule.validate", { ...mutation(obj({ version: int, cases: arr(ref("LotteryValidationCaseInput")), reason }, ["version", "cases", "reason"])), description: "Validation executes the bounded deterministic rule engine and records a validation report; it does not place bets or settle prizes." }),
  admin("POST", "/rule-versions/{id}/submit-review", "adminSubmitRuleReview", "Submit a validated version for review", ref("LotteryRuleVersion"), "rule.submit", mutation(obj({ version: int, reason }, ["version", "reason"]))),
  ...["approve", "reject"].map((decision) => admin("POST", `/rule-versions/{id}/${decision}`, `admin${decision[0].toUpperCase()}${decision.slice(1)}RuleVersion`, `${decision} a rule version`, ref("LotteryRuleVersion"), "rule.review", { ...mutation(obj({ version: int, reason, warnings_acknowledged: bool }, ["version", "reason", "warnings_acknowledged"])), description: "Review requires a different authorized reviewer from the creator/editor; SUPER_ADMIN is read-only for this brand workflow." })),
  admin("POST", "/rule-versions/{id}/clone", "adminCloneRuleVersion", "Clone a version into a draft", ref("LotteryRuleVersion"), "rule.write", mutation(obj({ effect_mode: str, reason }, ["effect_mode", "reason"]), 201)),
  admin("POST", "/rule-simulations", "adminSimulateRule", "Simulate a rule without financial effects", ref("LotterySimulationResult"), "rule.simulate", { ...mutation(ref("LotterySimulationInput")), idempotency: false, description: "Runs the deterministic rule engine for the supplied definition, selection, draw and multiplier; it does not create a bet, payout, or settlement. SUPER_ADMIN is read-only for brand mutations; this simulation path is a non-financial calculation." }),
  admin("GET", "/games/{id}/schedule", "adminGetGameSchedule", "Get a game's schedule", ref("LotteryScheduleRecord"), "schedule.view"),
  admin("PUT", "/games/{id}/schedule", "adminWriteGameSchedule", "Replace a game's schedule", ref("LotteryScheduleRecord"), "schedule.write", mutation(obj({ version: int, spec: ref("LotteryScheduleSpec"), reason }, ["version", "spec", "reason"]))),
  admin("GET", "/games/{id}/periods", "adminListGamePeriods", "List periods for a game", ref("LotteryAdminPeriodsPage"), "period.view", { parameters: pageQuery }),
  admin("GET", "/periods/{id}", "adminGetPeriod", "Get a period", ref("LotteryPeriod"), "period.view"),
  admin("POST", "/games/{id}/periods/generate", "adminGeneratePeriods", "Generate periods within the next seven days", ref("LotteryPeriodGeneration"), "period.generate", mutation(obj({ from: dateTime, to: dateTime, reason }, ["from", "to", "reason"]))),
  admin("GET", "/games/{id}/draw-sources", "adminGetDrawSources", "Get configured draw sources", ref("LotteryDrawSources"), "draw_source.view", { description: "Returns source configuration only; adapters are bounded API/DOM integrations and this endpoint does not expose fetched response data or credentials." }),
  admin("PUT", "/games/{id}/draw-sources", "adminWriteDrawSources", "Replace configured draw sources", ref("LotteryDrawSources"), "draw_source.write", { ...mutation(obj({ version: int, sources: arr(ref("LotteryDrawSource")), reason }, ["version", "sources", "reason"])), description: "Source types are api or dom. credential_ref is a reference to separately stored secret material, never a credential value. The endpoint only saves configuration; it does not make network requests." }),
  admin("GET", "/periods/{id}/draw", "adminGetPeriodDrawHistory", "Get draw result history and adapter attempts", ref("LotteryDrawHistory"), "draw.view", { parameters: pageQuery, description: "Shows persisted adapter attempt metadata for this period. No live API or DOM fetch is performed by this read." }),
  admin("POST", "/periods/{id}/manual-draw", "adminCreateManualDraw", "Record a manual draw result", ref("LotteryDrawResult"), "draw.manual_create", mutation(obj({ version: int, period_no: str, result: ref("LotteryRuleDrawInput"), drawn_at: dateTime, reason }, ["version", "period_no", "result", "drawn_at", "reason"]), 201)),
  admin("GET", "/periods/{id}/cancellation", "adminGetPeriodCancellation", "Get period cancellation and refund progress", ref("LotteryCancellationEnvelope"), "period.view"),
  admin("POST", "/periods/{id}/cancel", "adminCancelPeriod", "Cancel a period and enqueue refunds", ref("LotteryPeriodCancellation"), "period.cancel", { ...mutation(obj({ version: int, mode: { type: "string", enum: ["bet_cancelled", "judged_cancelled"] }, cause: { type: "string", enum: ["operator_cancel", "no_result", "invalid_result"] }, reason }, ["version", "mode", "cause", "reason"])), description: "Durably closes the period and enqueues bounded refunds; response may be 202 while refund processing continues. SUPER_ADMIN cannot perform brand write operations." }),
  admin("POST", "/periods/{id}/cancellation/retry", "adminRetryPeriodCancellation", "Retry failed period refunds", ref("LotteryPeriodCancellation"), "period.cancel_retry", { ...mutation(obj({ version: int, reason }, ["version", "reason"])), successStatus: 202 }),
  admin("GET", "/bet-policy", "adminGetBrandBetPolicy", "Get brand betting policy", ref("LotteryBrandPolicyRecord"), "bet_policy.view"),
  admin("PUT", "/bet-policy", "adminWriteBrandBetPolicy", "Update brand betting policy", ref("LotteryBrandPolicyRecord"), "bet_policy.write", mutation(obj({ version: int, config: ref("LotteryBrandPolicyConfig"), reason }, ["version", "config", "reason"]))),
  admin("GET", "/games/{id}/bet-policy", "adminGetGameBetPolicy", "Get game betting policy overrides", ref("LotteryGamePolicyRecord"), "bet_policy.view"),
  admin("PUT", "/games/{id}/bet-policy", "adminWriteGameBetPolicy", "Update game betting policy overrides", ref("LotteryGamePolicyRecord"), "bet_policy.write", mutation(obj({ version: int, config: ref("LotteryGamePolicyConfig"), reason }, ["version", "config", "reason"]))),
  admin("GET", "/bet-orders", "adminListBetOrders", "List brand bet orders", ref("LotteryBetOrdersPage"), "bet.view", { parameters: [...pageQuery, { name: "member_id", in: "query", schema: uuid }] }),
  admin("GET", "/bet-orders/{id}", "adminGetBetOrder", "Get a bet order", ref("LotteryBetOrder"), "bet.view"),
  admin("POST", "/bet-orders/{id}/cancel", "adminCancelBetOrder", "Cancel a brand bet order", ref("LotteryBetOrder"), "bet.cancel", mutation(obj({ version: int, reason }, ["version", "reason"]))),
  admin("GET", "/bet-orders/{id}/exception", "adminGetBetException", "Get bet order exception details", ref("LotteryExceptionEnvelope"), "bet.view"),
  admin("POST", "/bet-orders/{id}/abnormal", "adminMarkBetAbnormal", "Mark a bet order abnormal", ref("LotteryBetOrder"), "bet.mark_abnormal", mutation(obj({ version: int, reason }, ["version", "reason"]))),
  admin("GET", "/bet-orders/{id}/judgment", "adminGetBetJudgment", "Get cancellation judgment", ref("LotteryJudgmentEnvelope"), "bet.view"),
  admin("POST", "/bet-orders/{id}/judge-cancel", "adminJudgeBetCancellation", "Record an operator cancellation judgment", ref("LotteryBetOrder"), "bet.judge_cancel", mutation(obj({ version: int, cause: { type: "string", enum: ["no_result", "invalid_result"] }, reason }, ["version", "cause", "reason"]))),
  admin("GET", "/settlement-policy", "adminGetSettlementPolicy", "Get brand settlement policy", ref("LotterySettlementPolicy"), "settlement_policy.view"),
  admin("PUT", "/settlement-policy", "adminWriteSettlementPolicy", "Update brand settlement policy", ref("LotterySettlementPolicy"), "settlement_policy.write", mutation(obj({ version: int, mode: nullable(str), reason }, ["version", "mode", "reason"]))),
  admin("GET", "/periods/{id}/settlement-context", "adminGetPeriodSettlementContext", "Get settlement eligibility context", ref("LotteryPeriodSettlementContext"), "settlement.view"),
  admin("GET", "/periods/{id}/settlement", "adminGetPeriodSettlement", "Get current settlement job for a period", ref("LotterySettlementEnvelope"), "settlement.view"),
  admin("POST", "/periods/{id}/settle", "adminStartSettlement", "Start settlement for a drawn period", ref("LotterySettlementJob"), "settlement.run", { ...mutation(obj({ version: int, policy_version: int, draw_result_id: uuid, reason }, ["version", "policy_version", "draw_result_id", "reason"]), 201), description: "Creates a durable settlement job; payout processing is performed by the settlement worker. SUPER_ADMIN cannot perform brand write operations." }),
  admin("GET", "/settlement-jobs/{id}", "adminGetSettlementJob", "Get a settlement job", ref("LotterySettlementJob"), "settlement.view"),
  admin("GET", "/settlement-jobs/{id}/targets", "adminListSettlementTargets", "List settlement targets", ref("LotterySettlementTargets"), "settlement.view", { parameters: pageQuery }),
  ...["approve", "retry"].map((action) => admin("POST", `/settlement-jobs/{id}/${action}`, `admin${action[0].toUpperCase()}${action.slice(1)}Settlement`, `${action} a settlement job`, ref("LotterySettlementJob"), `settlement.${action}`, { ...mutation(obj({ version: int, reason }, ["version", "reason"])), description: "Settlement actions are brand-scoped and unavailable to SUPER_ADMIN; work is handled by a durable asynchronous settlement job." })),
  admin("GET", "/bet-orders/{id}/settlement-context", "adminGetBetSettlementContext", "Get settlement preview eligibility", ref("LotterySettlementContext"), "settlement.view"),
  admin("GET", "/bet-orders/{id}/settlement-previews", "adminListSettlementPreviews", "List previews for a bet order", ref("LotterySettlementPreviewPage"), "settlement.view", { parameters: pageQuery }),
  admin("POST", "/bet-orders/{id}/settlement-previews", "adminCreateSettlementPreview", "Calculate a non-financial settlement preview", ref("LotterySettlementPreview"), "settlement.preview", { ...mutation(obj({ version: int, period_version: int, draw_result_id: uuid, reason }, ["version", "period_version", "draw_result_id", "reason"]), 201), description: "Stores immutable calculation evidence only; it does not apply a payout or mutate wallet balances. SUPER_ADMIN cannot perform brand writes." }),
  admin("GET", "/settlement-previews/{id}", "adminGetSettlementPreview", "Get a settlement preview", ref("LotterySettlementPreview"), "settlement.view"),
  admin("GET", "/settlement-previews/{id}/lines", "adminGetSettlementPreviewLines", "Get settlement preview line calculations", ref("LotterySettlementLinesPage"), "settlement.view", { parameters: pageQuery }),
  admin("GET", "/periods/{id}/correction-context", "adminGetDrawCorrectionContext", "Get draw correction eligibility and policy context", ref("LotteryCorrectionContext"), "draw.view"),
  admin("POST", "/draw-results/{id}/correct", "adminCorrectDrawResult", "Correct a draw result and queue required resettlement", ref("LotteryCorrection"), "draw.correct", { ...mutation(obj({ version: int, policy_version: nullable(int), result: ref("LotteryRuleDrawInput"), reason }, ["version", "result", "reason"]), 201), description: "Correction can reverse earlier settlement effects and may start resettlement. Requires draw.correct and, when policy_version is supplied, settlement.run; SUPER_ADMIN cannot perform brand writes." }),
  admin("GET", "/periods/{id}/corrections", "adminListDrawCorrections", "List corrections for a period", ref("LotteryCorrectionsPage"), "draw.view", { parameters: pageQuery }),
  admin("GET", "/corrections/{id}", "adminGetDrawCorrection", "Get a correction", ref("LotteryCorrection"), "draw.view"),
  admin("GET", "/corrections/{id}/targets", "adminListDrawCorrectionTargets", "List correction targets", ref("LotteryCorrectionTargetsPage"), "draw.view", { parameters: pageQuery }),
  admin("POST", "/corrections/{id}/retry", "adminRetryDrawCorrection", "Retry failed correction work", ref("LotteryCorrection"), "draw.correction_retry", { ...mutation(obj({ version: int, reason }, ["version", "reason"])), description: "Retry also requires settlement.run; SUPER_ADMIN cannot perform brand writes." }),
  admin("GET", "/notification-deliveries", "adminListNotificationDeliveries", "List in-app notification delivery records", ref("LotteryNotificationDeliveriesPage"), "notification.view", { tag: "notification", parameters: pageQuery }),
  admin("POST", "/notification-deliveries/{id}/retry", "adminRetryNotificationDelivery", "Retry in-app notification materialization", ref("LotteryNotificationDelivery"), "notification.retry", { ...mutation(obj({ attempt_count: int, reason }, ["attempt_count", "reason"])), tag: "notification", description: "Retries the in-app notification delivery only; templates are fixed and versioned by the service. Request data cannot edit templates or payload content." }),

  op("GET", "/api/v1/games", "listBetCatalogGames", "List games available for betting", "betting", "public", ref("LotteryCatalogGamesPage"), { parameters: pageQuery }),
  op("GET", "/api/v1/games/{id}", "getBetGameCatalog", "Get game catalog and current betting context", "betting", "public", ref("LotteryGameCatalog")),
  op("GET", "/api/v1/games/{id}/plays", "listBetCatalogPlays", "List published game plays", "betting", "public", ref("LotteryCatalogPlaysPage")),
  op("GET", "/api/v1/games/{id}/periods/current", "getCurrentBettingPeriod", "Get current period and server time", "betting", "public", ref("LotteryCurrentPeriod")),
  op("GET", "/api/v1/draw-results", "listPublicDrawResults", "List public draw results", "games", "public", ref("LotteryPublicDrawPage"), { parameters: [...pageQuery, { name: "game_id", in: "query", schema: uuid }, { name: "period_no", in: "query", schema: { type: "string", maxLength: 80 } }] }),
  op("GET", "/api/v1/draw-results/{id}", "getPublicDrawResult", "Get a public draw result", "games", "public", ref("LotteryPublicDrawDetail")),
  op("GET", "/api/v1/games/{id}/periods", "listPublicGamePeriods", "List public game periods and draws", "games", "public", ref("LotteryPublicPeriodPage"), { parameters: [...pageQuery, { name: "game_id", in: "query", schema: uuid }, { name: "period_no", in: "query", schema: { type: "string", maxLength: 80 } }] }),
  op("POST", "/api/v1/bet-previews", "previewBetOrder", "Preview a bet without reserving points", "betting", "user", ref("LotteryBetPreviewWithActor"), { requestBody: ref("LotteryBetInput"), idempotency: false, description: "Computes a quote only; it does not reserve points or place a bet." }),
  op("POST", "/api/v1/bet-orders", "placeBetOrder", "Place a bet order", "betting", "user", ref("LotteryBetOrder"), { ...mutation(ref("LotteryBetInput"), 201), description: "Places a bet using the current session and idempotency engine; placement revalidates the preview context." }),
  op("GET", "/api/v1/bet-orders", "listMyBetOrders", "List the current member's bet orders", "betting", "user", ref("LotteryBetOrderItems"), { parameters: pageQuery }),
  op("GET", "/api/v1/bet-orders/{id}", "getMyBetOrder", "Get one of the current member's bet orders", "betting", "user", ref("LotteryBetOrder")),
  op("POST", "/api/v1/bet-orders/{id}/cancel", "cancelMyBetOrder", "Request cancellation of a bet order", "betting", "user", ref("LotteryBetOrder"), { ...mutation(obj({ version: int, reason }, ["version", "reason"])), description: "Cancellation is subject to period state and the effective policy; idempotency is enforced by the mutation engine." }),
  op("GET", "/api/v1/notifications", "listMyNotifications", "List current member's in-app notifications", "notification", "user", ref("LotteryNotificationPage"), { parameters: pageQuery, description: "Notifications use fixed template_key/template_version 1 and the service-owned payload shape. Templates and payload content are not editable through this API." }),
  op("POST", "/api/v1/notifications/read", "markMyNotificationsRead", "Mark notifications as read", "notification", "user", ref("LotteryNotificationReadReceipt"), mutation(obj({ ids: { type: "array", minItems: 1, maxItems: 100, uniqueItems: true, items: uuid } }, ["ids"]))),
];

// Response schemas used by operation data refs. The builder supplies the common
// success/error envelope and shared headers around each operation.
Object.assign(schemas, {
  LotteryAdminGamesPage: obj({ games: arr(ref("LotteryGame")), limit: int, offset: int }),
  LotteryAdminPlaysPage: obj({ plays: arr(ref("LotteryPlay")), limit: int, offset: int }),
  LotteryAdminRuleVersionsPage: obj({ versions: arr(ref("LotteryRuleVersion")), limit: int, offset: int }),
  LotteryAdminPeriodsPage: obj({ periods: arr(ref("LotteryPeriod")), limit: int, offset: int }),
  LotteryPeriodGeneration: Generation,
  LotteryCancellationEnvelope: obj({ cancellation: ref("LotteryPeriodCancellation") }),
  LotteryBrandPolicyRecord: obj({ brand_id: uuid, version: int, config: ref("LotteryBrandPolicyConfig"), updated_at: dateTime }),
  LotteryGamePolicyRecord: obj({ brand_id: uuid, game_id: uuid, version: int, config: ref("LotteryGamePolicyConfig"), updated_at: dateTime }),
  LotteryBetOrdersPage: obj({ items: arr(ref("LotteryBetOrder")) }),
  LotteryExceptionEnvelope: obj({ exception: nullable(ref("LotteryBetException")) }),
  LotteryJudgmentEnvelope: obj({ judgment: nullable(ref("LotteryBetJudgment")) }),
  LotteryPeriodSettlementContext: obj({ brand_id: uuid, game_id: uuid, period_id: uuid, period_version: int, period_status: str, draw_result_id: nullable(uuid), policy_version: int, mode: nullable(str), can_start: bool }),
  LotterySettlementEnvelope: obj({ settlement: nullable(ref("LotterySettlementJob")) }),
  LotterySettlementTargets: obj({ brand_id: uuid, job_id: uuid, items: arr(ref("LotterySettlementTarget")), limit: int, offset: int, has_more: bool }),
  LotterySettlementContext: obj({ brand_id: uuid, game_id: uuid, period_id: uuid, order_id: uuid, order_version: int, order_status: str, definition_hash: str, period_version: int, period_status: str, draw_result_id: nullable(uuid), draw_hash: nullable(str), draw: nullable(ref("LotteryRuleDraw")), can_preview: bool }),
  LotterySettlementPreviewPage: obj({ brand_id: uuid, order_id: uuid, items: arr(ref("LotterySettlementPreview")), limit: int, offset: int, has_more: bool }),
  LotterySettlementLinesPage: obj({ brand_id: uuid, preview_id: uuid, order_id: uuid, items: arr(ref("LotteryRuleLine")), limit: int, offset: int, has_more: bool, total: int }),
  LotteryCorrectionContext: obj({ brand_id: uuid, game_id: uuid, period_id: uuid, period_version: int, period_status: str, draw_result_id: nullable(uuid), draw: nullable(ref("LotteryRuleDraw")), model: ref("LotteryRuleModel"), current_job_id: nullable(uuid), current_job_version: nullable(int), policy_version: int, mode: nullable(str), requires_resettlement: bool, can_correct: bool }),
  LotteryCorrectionsPage: obj({ brand_id: uuid, period_id: uuid, items: arr(ref("LotteryCorrection")), limit: int, offset: int, has_more: bool }),
  LotteryCorrectionTargetsPage: obj({ brand_id: uuid, correction_id: uuid, items: arr(ref("LotteryCorrectionTarget")), limit: int, offset: int, has_more: bool }),
  LotteryNotificationDelivery: NotificationDelivery,
  LotteryNotificationDeliveriesPage: obj({ items: arr(ref("LotteryNotificationDelivery")) }),
  LotteryCatalogGamesPage: obj({ items: arr(ref("LotteryCatalogGame")), limit: int, offset: int }),
  LotteryCatalogPlaysPage: obj({ items: arr(ref("LotteryCatalogPlay")) }),
  LotteryCurrentPeriod: obj({ period: nullable(ref("LotteryBetPeriod")), server_time: dateTime, brand_status: str }),
  LotteryPublicDrawDetail: obj({ item: ref("LotteryPublicDraw"), server_time: dateTime, brand_status: str }),
  LotteryBetPreviewWithActor: obj({ normalized: ref("LotteryRuleSelection"), expanded_bets: arr(ref("LotteryRuleSelection")), combination_count: int, unit_points: amount, multiplier: amount, bet_points: amount, period_id: uuid, play_id: uuid, rule_version_id: uuid, definition_hash: str, policy: ref("LotteryEffectivePolicy"), policy_versions: ref("LotteryPolicyVersions"), period: ref("LotteryBetPeriod"), actor_context: str }),
  LotteryBetOrderItems: obj({ items: arr(ref("LotteryBetOrder")) }),
  LotteryNotificationPage: obj({ brand_id: uuid, member_id: uuid, items: arr(ref("LotteryNotification")), unread_count: amount, limit: int, offset: int }),
  LotteryNotificationReadReceipt: obj({ brand_id: uuid, member_id: uuid, ids: arr(uuid), changed: int, unread_count: amount }),
});
