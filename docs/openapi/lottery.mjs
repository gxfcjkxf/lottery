import { addHistoryReadHeaders } from "./history-reads.mjs";
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
const notificationTemplateKeys = [
  "bet.order.abnormal", "bet.order.cancelled", "bet.order.judged_cancelled", "bet.order.placed",
  "bet.order.prize_reversed", "bet.order.won", "commission.adjusted", "commission.corrected", "commission.paid",
  "draw.result.corrected", "draw.result.published", "member.joined", "recharge.confirmed",
  "reward.order.granted", "reward.order.revoked",
  "withdrawal.order.cancelled", "withdrawal.order.failed", "withdrawal.order.paid",
  "withdrawal.order.processing", "withdrawal.order.rejected", "withdrawal.order.reviewing",
];
const notificationTemplateKey = { type: "string", enum: notificationTemplateKeys };
const notificationTemplateVersion = {
  type: "integer", minimum: 1, maximum: 9007199254740991,
  description: "Positive version no greater than 9007199254740991, so it is exactly representable as a JSON/JavaScript integer.",
};
const notificationTemplateCopy = obj({
  title: { type: "string", minLength: 1, maxLength: 120, pattern: String.raw`^(?:(?![{}])[^\u0000-\u001F<>]|\{(?:points|resource_id)\})+$`, description: "Schema limits this to 1–120 Unicode characters and rejects angle brackets, C0 controls, unmatched braces, and unsupported placeholders. The handler additionally requires trimmed valid UTF-8, limits the value to 120 characters, and rejects case-insensitive https?:, javascript:, data:, and www. substrings." },
  body: { type: "string", minLength: 1, maxLength: 1200, pattern: String.raw`^(?:(?![{}])[^\u0000-\u0008\u000B\u000C\u000E-\u001F<>]|\{(?:points|resource_id)\})+$`, description: "Schema limits this to 1–1200 Unicode characters and rejects angle brackets, disallowed C0 controls, unmatched braces, and unsupported placeholders; LF and tab are allowed. The handler additionally requires trimmed valid UTF-8, limits the value to 1200 characters, and rejects case-insensitive https?:, javascript:, data:, and www. substrings." },
});
const notificationCustomContent = obj({
  en: ref("LotteryNotificationTemplateCopy"),
  "zh-CN": ref("LotteryNotificationTemplateCopy"),
}, ["en", "zh-CN"]);
notificationCustomContent.description = "Exactly English and Simplified Chinese copies. Only {points} and {resource_id} placeholders are supported; nineteen event templates require {points} in each language body, while member.joined forbids {points} in either body or title and draw result templates require {resource_id} in both bodies and forbid {points}. Placeholder presence and Unicode character limits are enforced by the handler. Draw result notices identify historical results and are not guarantees of a win or prize payment; the actual result is available in the user app and users cannot edit the notice. Withdrawal events describe historical internal points states, never proof of an external transfer.";
const notificationI18nReference = obj({ i18n_key: notificationTemplateKey, i18n_version: { type: "integer", const: 1 } });
const notificationTemplateContent = { oneOf: [ref("LotteryNotificationI18nReference"), ref("LotteryNotificationCustomContent")], description: "System templates store only a shared i18n key and catalog version. Brand-authored templates store their explicit bilingual copy. Event parameters remain in payload; UI clients resolve system copy through their shared i18n catalog." };
const notificationTemplatePairRules = notificationTemplateKeys.map((key) => ({
  properties: { event_type: { const: key }, template_key: { const: key } },
}));

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
const platformTemplate = (method, path, operationId, summary, data, permission, extra = {}) => op(method, `/api/v1/platform${path}`, operationId.replace('admin', 'platform'), summary, 'notification', 'admin', data, { permissions: [`${permission}.platform`], ...extra });
const mutation = (body, status = 200) => ({ requestBody: body, idempotency: true, successStatus: status });

const RuleCondition = obj({
  op: str, field: str, target: str, position: nullable(int), attribute_group: str,
  attribute_value: str, selection_key: str, value: nullable(int), values: nullable(arr(int)),
  min: nullable(int), max: nullable(int), children: nullable(arr(ref("LotteryRuleCondition"))),
}, ["op"]);
const RulePool = obj({ min: { ...int, default: 0 }, max: { ...int, default: 0 }, values: arr(int), allow_repeat: bool }, ["allow_repeat"]);
const RulePoolInput = obj({ min: { ...int, default: 0 }, max: { ...int, default: 0 }, values: nullable(arr(int)), allow_repeat: bool }, []);
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
  game_id: uuid, period_id: uuid, period_no: { type: 'string', description: 'Human-readable period number included in order list responses.' }, play_id: uuid, rule_version_id: uuid, definition_hash: str,
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
const withdrawalNotificationKeys = notificationTemplateKeys.filter(key => key.startsWith("withdrawal.order."));
const rewardNotificationKeys = notificationTemplateKeys.filter(key => key.startsWith("reward.order."));
const drawNotificationKeys = ["draw.result.published", "draw.result.corrected"];
const positiveNotificationKeys = notificationTemplateKeys.filter(key => key !== "member.joined" && key !== "commission.adjusted" && key !== "commission.corrected" && !drawNotificationKeys.includes(key));
const int64Upper = "9223372036854775807";
const int64Alternatives = (limit) => {
  const alternatives = [`[1-9][0-9]{0,${limit.length - 2}}`, limit];
  for (let i = 0; i < limit.length; i++) {
    const floor = i === 0 ? 1 : 0, ceiling = Number(limit[i]) - 1;
    if (ceiling < floor) continue;
    const digit = ceiling === floor ? String(floor) : `[${floor}-${ceiling}]`;
    alternatives.push(limit.slice(0, i) + digit + `[0-9]{${limit.length - i - 1}}`);
  }
  return alternatives;
};
const positiveInt64Alternatives = int64Alternatives(int64Upper);
const negativeInt64Alternatives = int64Alternatives("9223372036854775808");
const withdrawalNotificationPoints = { type:"string", pattern:`^(?:${positiveInt64Alternatives.join("|")})$`, description:"Canonical positive int64 points; at most 9223372036854775807." };
const adjustedCommissionPoints = { type:"string", pattern:`^(?:${positiveInt64Alternatives.join("|")}|-(?:${negativeInt64Alternatives.join("|")}))$`, description:"Canonical signed nonzero int64 points; at most 9223372036854775807 or as low as -9223372036854775808." };
const drawNumber = { type:"integer", minimum:0, maximum:1_000_000 };
const drawNumbers = { type:"array", maxItems:10, items:drawNumber };
const drawDigits = { type:"array", maxItems:10, items:{ type:"integer", minimum:0, maximum:9 } };
const drawResult = {
  ...obj({ regular:drawNumbers, special:drawNumbers, digits:drawDigits }),
  allOf:[
    { anyOf:[{ properties:{ regular:{ minItems:1 } } },{ properties:{ special:{ minItems:1 } } },{ properties:{ digits:{ minItems:1 } } }] },
    { if:{ properties:{ digits:{ minItems:1 } } }, then:{ properties:{ regular:{ maxItems:0 }, special:{ maxItems:0 } } } },
  ],
};
const drawNoticePayload = previousDrawID => obj({
  resource_id:uuid,
  points:{ type:"null" },
  draw:obj({
    game_id:uuid, period_id:uuid, period_no:{ type:"string", minLength:1, maxLength:80, description:"Nonempty period identifier, limited to 80 characters." }, result:drawResult, drawn_at:dateTime,
    previous_draw_id:previousDrawID,
  }),
});
const publishedDrawPayload = drawNoticePayload({ type:"null" });
const correctedDrawPayload = drawNoticePayload(uuid);
const SettlementJob = obj({ id: uuid, brand_id: uuid, game_id: uuid, period_id: uuid, draw_result_id: uuid, period_version: int, policy_version: int, mode: str, state: str, version: int, target_count: int, created_by: uuid, approved_by: nullable(uuid), reason, created_at: dateTime, completed_at: nullable(dateTime), last_error_code: nullable(str), pending_count: int, ready_count: int, paid_count: int, excluded_count: int, failed_count: int, prize_points: amount, paid_points: amount, can_retry: bool, generation: int, previous_job_id: nullable(uuid), correction_id: nullable(uuid), current: bool });
const SettlementPreview = obj({ context: ref("LotterySettlementContext"), outcome: str, error_code: nullable(str), calculation: nullable(obj({ won: bool, combination_count: int, multiplier: amount, bet_points: amount, prize_points: amount, raw_prize_points: str, capped_prize_points: str })), lines: arr(ref("LotteryRuleLine")) });
const Correction = obj({ id: uuid, brand_id: uuid, game_id: uuid, period_id: uuid, previous_draw_result_id: uuid, draw_result_id: uuid, result: ref("LotteryRuleDraw"), period_version: int, previous_job_id: nullable(uuid), new_job_id: nullable(uuid), policy_version: nullable(int), mode: nullable(str), state: str, version: int, target_count: int, created_by: uuid, reason, created_at: dateTime, completed_at: nullable(dateTime), last_error_code: nullable(str), pending_count: int, reversed_count: int, unchanged_count: int, excluded_count: int, failed_count: int, reverse_points: amount, reversed_points: amount, can_retry: bool, new_job_state: nullable(str), new_job_version: nullable(int), new_job_error_code: nullable(str) });
const CancelJob = obj({ id: uuid, brand_id: uuid, game_id: uuid, period_id: uuid, period_version: int, mode: str, cause: str, state: str, version: int, reason, created_by: uuid, created_at: dateTime, completed_at: nullable(dateTime), last_error_code: str, total_count: int, pending_count: int, refunded_count: int, already_refunded_count: int, failed_count: int });
const Notification = {
  ...obj({
    id: uuid, brand_id: uuid, member_id: uuid,
    event_type: { type: "string", enum: notificationTemplateKeys },
    template_key: notificationTemplateKey,
    template_version: notificationTemplateVersion,
    content: ref("LotteryNotificationTemplateContent"),
    payload: obj({ resource_id: uuid, points: nullable(str), draw:{ type:"object" } }, ["resource_id", "points"]), created_at: dateTime, read_at: nullable(dateTime),
  }),
  allOf: [
    { oneOf: notificationTemplatePairRules },
    { required:["content"] },
    { if:{properties:{event_type:{enum:withdrawalNotificationKeys}}}, then:{properties:{payload:obj({resource_id:uuid,points:withdrawalNotificationPoints})}} },
    { if:{properties:{event_type:{enum:positiveNotificationKeys}}}, then:{properties:{payload:obj({resource_id:uuid,points:withdrawalNotificationPoints})}} },
    { if:{properties:{event_type:{enum:["commission.adjusted","commission.corrected"]}}}, then:{properties:{payload:obj({resource_id:uuid,points:adjustedCommissionPoints})}} },
    { if:{not:{properties:{event_type:{enum:drawNotificationKeys}},required:["event_type"]}}, then:{properties:{payload:{not:{required:["draw"]}}}} },
    { if:{properties:{event_type:{const:"draw.result.published"}}}, then:{properties:{payload:publishedDrawPayload}} },
    { if:{properties:{event_type:{const:"draw.result.corrected"}}}, then:{properties:{payload:correctedDrawPayload}} },
  ],
  description: "Notification content freezes the selected template identity: system templates store an i18n key/version reference and custom templates store their explicit bilingual copy. Parameters are kept separately in payload. Withdrawal, reward and commission.paid points are positive int64; commission.adjusted and commission.corrected points are signed nonzero int64. Draw result payloads are closed historical facts; published events have no previous result and corrected events identify the prior result. Reward events retain a state-specific historical and nonfinancial disclaimer that template editing cannot remove. Updating a template never rewrites existing notifications.",
};

export const schemas = {
  LotteryPlayerDrawArchive: obj({mark_six:bool,game_id:uuid,game_name:str,timezone:str,daily:bool,choices:arr(str),selected:str,items:arr(obj({period_no:str,result:RuleDraw,regular_order:{anyOf:[{type:"array",items:{type:"integer"}},{type:"null"}],description:"Saved regular-ball draw sequence; null when no verified sequence is available. The special ball is separate."},drawn_at:nullable(dateTime),draw_date:{anyOf:[{type:"string",format:"date"},{type:"null"}],description:"Confirmed historical calendar date; null when unknown. Historical records do not invent an intraday timestamp."},attributes:{type:"object",additionalProperties:obj({wave:str,zodiac:str,element:str})}})),has_more:bool}),
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
  LotteryNotificationTemplateCopy: notificationTemplateCopy,
  LotteryNotificationTemplateContent: notificationTemplateContent,
  LotteryNotificationI18nReference: notificationI18nReference,
  LotteryNotificationCustomContent: notificationCustomContent,
  LotteryNotificationTemplate: {
    ...obj({
    key: notificationTemplateKey, version: notificationTemplateVersion,
    content: ref("LotteryNotificationTemplateContent"), updated_at: dateTime,
    audit_log_id: nullable(uuid),
    }),
    allOf: [{ oneOf: [
      { properties: { version: { const: 1 }, audit_log_id: { type: "null" } } },
      { properties: { version: { minimum: 2 }, audit_log_id: uuid } },
    ] }],
    description: "Version 1 has no audit log ID; versions 2 and later have a non-null audit log ID.",
  },
  LotteryNotificationTemplateRevision: {
    ...obj({
    id: uuid, key: notificationTemplateKey, version: notificationTemplateVersion,
    content: ref("LotteryNotificationTemplateContent"), changed_by: nullable(uuid), reason: { type: 'string', maxLength: 500 },
    audit_log_id: nullable(uuid), created_at: dateTime,
    }),
    allOf: [{ oneOf: [
      { properties: { version: { const: 1 }, changed_by: { type: "null" }, audit_log_id: { type: "null" } } },
      { properties: { version: { minimum: 2 }, changed_by: uuid, audit_log_id: uuid } },
    ] }],
    description: "Version 1 has null changed_by and audit_log_id; versions 2 and later have both IDs non-null.",
  },
  LotteryNotificationTemplateUpdateInput: obj({
    version: notificationTemplateVersion, content: ref("LotteryNotificationTemplateContent"), reason: { type: 'string', maxLength: 500 },
  }, ["version", "content"]),
  LotteryNotificationTemplatesPage: obj({ items: arr(ref("LotteryNotificationTemplate")) }),
  LotteryNotificationTemplateHistoryPage: obj({ items: arr(ref("LotteryNotificationTemplateRevision")) }),
  LotteryPointAllocation: Allocation,
  LotterySettlementJob: SettlementJob, LotterySettlementPreview: SettlementPreview,
  LotteryCorrection: Correction, LotteryPeriodCancellation: CancelJob, LotteryNotification: Notification,
  LotteryNotificationDrawPublished: {
    allOf: [ref("LotteryNotification"), { properties: { event_type: { const: "draw.result.published" } } }],
  },
  LotteryNotificationDrawCorrected: {
    allOf: [ref("LotteryNotification"), { properties: { event_type: { const: "draw.result.corrected" } } }],
  },
};

export const operations = [
  admin("GET", "/notification-deliveries", "adminListNotificationDeliveries", "List in-app notification delivery records", ref("LotteryNotificationDeliveriesPage"), "notification.view", { tag: "notification", parameters: pageQuery }),
  admin("POST", "/notification-deliveries/{id}/retry", "adminRetryNotificationDelivery", "Retry in-app notification materialization", ref("LotteryNotificationDelivery"), "notification.retry", { ...mutation(obj({ attempt_count: int, reason }, ["attempt_count"])), tag: "notification", description: "Retries in-app materialization only. A not-yet-created message snapshots the current brand template at materialization; existing messages are unchanged. Retry input cannot edit templates or event facts." }),
  platformTemplate("GET", "/notification-templates", "adminListNotificationTemplates", "List shared platform notification templates", ref("LotteryNotificationTemplatesPage"), "notification_template.view", { tag: "notification", description: "Requires explicit notification_template.view.platform. Applies to all brands; no brand header is required. Brand entry routes are unavailable. Template versions, immutable history and audit are global per event. The list accepts no query parameters. History pagination uses limit 1–100 and offset 0–1000000." }),
  platformTemplate("GET", "/notification-templates/{key}/history", "adminListNotificationTemplateHistory", "List revisions of a notification template", ref("LotteryNotificationTemplateHistoryPage"), "notification_template.view", { tag: "notification", parameters: [{ name: "key", in: "path", required: true, schema: notificationTemplateKey }, { name: "limit", in: "query", schema: { type: "integer", minimum: 1, maximum: 100, default: 20 } }, { name: "offset", in: "query", schema: { type: "integer", minimum: 0, maximum: 1000000, default: 0 } }], description: "Requires explicit notification_template.view.platform. Applies to all brands; no brand header is required. Brand entry routes are unavailable. Template versions, immutable history and audit are global per event. The list accepts no query parameters. History pagination uses limit 1–100 and offset 0–1000000." }),
  platformTemplate("PUT", "/notification-templates/{key}", "adminUpdateNotificationTemplate", "Replace a notification template", ref("LotteryNotificationTemplate"), "notification_template.write", { ...mutation(ref("LotteryNotificationTemplateUpdateInput")), tag: "notification", parameters: [{ name: "key", in: "path", required: true, schema: notificationTemplateKey }], description: "Requires explicit notification_template.write.platform. Applies to all brands; no brand header is required. Brand entry routes are unavailable. Template versions, immutable history and audit are global per event. Writes check the expected version and use checked idempotency; retries must reuse the original key and body. Changes affect newly materialized notifications, never existing snapshots." }),

  op("GET", "/api/v1/draw-archive", "getPlayerDrawArchive", "Read one game's yearly or daily result archive", "games", "public", ref("LotteryPlayerDrawArchive"), {parameters:[{name:"game_id",in:"query",required:true,schema:uuid},{name:"kind",in:"query",required:true,schema:{type:"string",enum:["configured"]}},{name:"selected",in:"query",schema:{type:"string",description:"Platform-timezone year YYYY or date YYYY-MM-DD; defaults to the current year or day."}},{name:"offset",in:"query",schema:{type:"integer",minimum:0,maximum:1000000}}]}),
  op("GET", "/api/v1/notifications", "listMyNotifications", "List current member's in-app notifications", "notification", "user", ref("LotteryNotificationPage"), { parameters: pageQuery, description: "Notifications expose immutable content snapshots and their template versions. Templates and payload content are not editable through this API." }),
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

addHistoryReadHeaders(operations);
