const ref = (name) => ({ $ref: `#/components/schemas/${name}` });
const nullable = (schema) => ({ anyOf: [schema, { type: "null" }] });
const str = (extra = {}) => ({ type: "string", ...extra });
const int = (extra = {}) => ({ type: "integer", ...extra });
const bool = { type: "boolean" };
const array = (items, extra = {}) => ({ type: "array", items, ...extra });
const aggregateInteger = str({ pattern: "^(0|[1-9][0-9]*)$", description: "Exact nonnegative decimal aggregate string; totals use SQL numeric-to-text conversion and may exceed int64." });
const aggregateSignedInteger = str({ pattern: "^-?(0|[1-9][0-9]*)$", description: "Exact signed decimal aggregate string; totals use SQL numeric-to-text conversion and may exceed int64." });
const reconciliationCount = str({ pattern: "^(0|[1-9][0-9]{0,4}|100000)$", description: "Exact nonnegative decimal count bounded by the reconciliation target cap of 100000." });
const object = (properties, required = Object.keys(properties), extra = {}) => ({
  type: "object",
  properties,
  required,
  additionalProperties: false,
  ...extra,
});

const pageParameters = [
  { name: "limit", in: "query", required: false, schema: int({ default: 50, minimum: 1, maximum: 100 }) },
  { name: "offset", in: "query", required: false, schema: int({ default: 0, minimum: 0, maximum: 1000000 }) },
];
const joinPageParameters = [
  { name: "limit", in: "query", required: false, schema: int({ default: 20, minimum: 1, maximum: 100 }) },
  { name: "offset", in: "query", required: false, schema: int({ default: 0, minimum: 0, maximum: 1000000 }) },
];
const reportPageParameters = [
  { name: "limit", in: "query", required: false, schema: int({ default: 20, minimum: 1, maximum: 100 }) },
  { name: "offset", in: "query", required: false, schema: int({ default: 0, minimum: 0, maximum: 1000000 }) },
];
const adminReadPermissions = (permission) => [permission];
const adminWritePermissions = (permission) => [permission];

export const schemas = {
  FinanceBalance: object({
    recharge: object({ available: ref("NonnegativeInt64String"), manual_frozen: ref("NonnegativeInt64String"), system_frozen: ref("NonnegativeInt64String"), withdrawal: ref("NonnegativeInt64String") }),
    winning: object({ available: ref("NonnegativeInt64String"), manual_frozen: ref("NonnegativeInt64String"), system_frozen: ref("NonnegativeInt64String"), withdrawal: ref("NonnegativeInt64String") }),
    gift: object({ available: ref("NonnegativeInt64String"), manual_frozen: ref("NonnegativeInt64String"), system_frozen: ref("NonnegativeInt64String"), withdrawal: ref("NonnegativeInt64String") }),
  }),
  FinanceDeltaBalance: object({
    recharge: object({ available: ref("Int64String"), manual_frozen: ref("Int64String"), system_frozen: ref("Int64String"), withdrawal: ref("Int64String") }),
    winning: object({ available: ref("Int64String"), manual_frozen: ref("Int64String"), system_frozen: ref("Int64String"), withdrawal: ref("Int64String") }),
    gift: object({ available: ref("Int64String"), manual_frozen: ref("Int64String"), system_frozen: ref("Int64String"), withdrawal: ref("Int64String") }),
  }),
  FinanceAllocation: object({ source: str({ enum: ["recharge", "winning", "gift"] }), state: str({ enum: ["available", "manual_frozen", "system_frozen", "withdrawal"] }), points: ref("PositiveInt64String") }),
  FinanceWallet: object({
    account_id: ref("UUID"), brand_id: ref("UUID"), member_id: ref("UUID"), version: int(),
    display_points: ref("NonnegativeInt64String"), available_points: ref("NonnegativeInt64String"), frozen_points: ref("NonnegativeInt64String"), withdrawal_points: ref("NonnegativeInt64String"),
    recharge_points: ref("NonnegativeInt64String"), winning_points: ref("NonnegativeInt64String"), gift_points: ref("NonnegativeInt64String"), manual_frozen_points: ref("NonnegativeInt64String"), system_frozen_points: ref("NonnegativeInt64String"), by_source: ref("FinanceBalance"),
  }),
  FinanceEntry: object({
    id: ref("UUID"), brand_id: ref("UUID"), account_id: ref("UUID"), member_id: ref("UUID"), entry_type: str({ pattern: "^[a-zA-Z0-9_.:-]{1,200}$" }), reference_type: str({ pattern: "^[a-zA-Z0-9_.:-]{1,200}$" }), reference_id: { oneOf: [ref("UUID"), { const: "" }] }, operation_key: str({ pattern: "^[a-zA-Z0-9_.:-]{1,200}$" }), reason: ref("Reason"), actor_type: str({ enum: ["user", "admin", "system"] }), actor_id: { oneOf: [ref("UUID"), { const: "" }] }, request_id: str({ minLength: 1, maxLength: 80 }),
    reversal_of: ref("UUID"), version: int(), before_snapshot: ref("FinanceBalance"), delta_snapshot: ref("FinanceDeltaBalance"), after_snapshot: ref("FinanceBalance"), source_allocation: array(ref("FinanceAllocation"), { minItems: 1, maxItems: 3 }), created_at: ref("DateTime"),
  }, ["id", "brand_id", "account_id", "member_id", "entry_type", "reference_type", "reference_id", "operation_key", "reason", "actor_type", "actor_id", "request_id", "version", "before_snapshot", "delta_snapshot", "after_snapshot", "source_allocation", "created_at"]),
  FinanceWalletLedgerPage: object({ items: array(ref("FinanceEntry")) }),
  FinanceRecharge: object({
    id: ref("UUID"), brand_id: ref("UUID"), member_id: ref("UUID"), account_id: ref("UUID"), points: ref("PositiveInt64String"), state: str({ enum: ["pending", "confirmed", "cancelled"] }), proof_reference: str(), remark: str(), created_by: ref("UUID"),
    confirmed_by: ref("UUID"), version: int(), created_at: ref("DateTime"), confirmed_at: ref("DateTime"), ledger_entry_id: ref("UUID"), audit_log_id: ref("UUID"),
  }, ["id", "brand_id", "member_id", "account_id", "points", "state", "proof_reference", "remark", "created_by", "version", "created_at"]),
  FinanceRechargeList: object({ items: array(ref("FinanceRecharge")) }),
  FinanceRechargeCreate: object({ member_id: ref("UUID"), points: ref("PositiveInt64String"), proof_reference: str({ maxLength: 500 }), remark: str({ maxLength: 2000 }), reason: ref("Reason") }, ["member_id", "points", "reason"]),
  FinanceVersionReason: object({ version: int({ minimum: 1 }), reason: ref("Reason") }),
  FinanceFreeze: object({ points: ref("PositiveInt64String"), reason: ref("Reason") }),
  FinanceUnfreeze: object({ entry_id: ref("UUID"), reason: ref("Reason") }),
  FinanceAdjust: object({ source: str({ enum: ["recharge", "winning", "gift"] }), delta: { allOf: [ref("Int64String")], not: { enum: ["0", "-9223372036854775808"] } }, reason: ref("Reason") }),
  FinanceAdjustmentPolicy: object({
    brand_id: ref("UUID"), version: int(), max_balance_points: nullable(ref("PositiveInt64String")), max_recharge_points: nullable(ref("PositiveInt64String")), max_adjustment_points: nullable(ref("PositiveInt64String")), audit_log_id: ref("UUID"),
  }, ["brand_id", "version", "max_balance_points", "max_recharge_points", "max_adjustment_points"]),
  FinanceAdjustmentPolicyInput: object({ version: int({ minimum: 1 }), max_balance_points: nullable(ref("PositiveInt64String")), max_recharge_points: nullable(ref("PositiveInt64String")), max_adjustment_points: nullable(ref("PositiveInt64String")), reason: ref("Reason") }),
  FinanceReconciliation: object({ consistent: bool, account_id: ref("UUID"), member_id: ref("UUID"), version: int(), entry_count: int(), expected: ref("FinanceBalance"), actual: ref("FinanceBalance"), issues: array(str()) }),
  FinanceRepairActual: object({
    recharge: object({ available: ref("NonnegativeInt64String"), manual_frozen: ref("NonnegativeInt64String"), system_frozen: ref("NonnegativeInt64String"), withdrawal: ref("NonnegativeInt64String") }, [], { additionalProperties: ref("NonnegativeInt64String") }),
    winning: object({ available: ref("NonnegativeInt64String"), manual_frozen: ref("NonnegativeInt64String"), system_frozen: ref("NonnegativeInt64String"), withdrawal: ref("NonnegativeInt64String") }, [], { additionalProperties: ref("NonnegativeInt64String") }),
    gift: object({ available: ref("NonnegativeInt64String"), manual_frozen: ref("NonnegativeInt64String"), system_frozen: ref("NonnegativeInt64String"), withdrawal: ref("NonnegativeInt64String") }, [], { additionalProperties: ref("NonnegativeInt64String") }),
  }, [], { additionalProperties: false }),
  FinanceRepairPreview: object({ account_id: ref("UUID"), member_id: ref("UUID"), version: int(), ledger_version: int(), actual: ref("FinanceRepairActual"), expected: ref("FinanceBalance"), repairable: bool, consistent: bool, issues: array(str()), token: str() }),
  FinanceRepairRecord: object({ id: ref("UUID"), preview: ref("FinanceRepairPreview"), audit_log_id: ref("UUID") }),
  FinanceRepairSnapshot: object({ version: int(), buckets: { anyOf: [ref("FinanceBalance"), ref("FinanceRepairActual")] } }),
  FinanceRepairHistory: object({ id: ref("UUID"), version: int(), before_snapshot: ref("FinanceRepairSnapshot"), after_snapshot: ref("FinanceRepairSnapshot"), reason: str(), actor_id: ref("UUID"), request_id: str(), created_at: ref("DateTime") }),
  FinanceRepairList: object({ items: array(ref("FinanceRepairHistory")) }),
  FinanceRepairInput: object({ version: int({ minimum: 1 }), token: str(), reason: ref("Reason") }),

  FinanceReconciliationReason: str({ minLength: 1, maxLength: 500, pattern: "^(?!\\s)(?![\\s\\S]*\\s$)(?![\\s\\S]*[\\r\\n\\x00])[\\s\\S]+$", description: "Nonempty valid UTF-8, at most 500 bytes, already trimmed, and contains no LF, CR, or NUL. The handler rejects rather than normalizes invalid reasons." }),
  FinanceReconciliationJob: object({
    id: ref("UUID"), brand_id: ref("UUID"), state: str({ enum: ["pending", "running", "completed", "failed"] }), version: int({ minimum: 1, maximum: Number.MAX_SAFE_INTEGER }),
    target_count: reconciliationCount, checked_count: reconciliationCount, consistent_count: reconciliationCount, repairable_count: reconciliationCount, corrupt_count: reconciliationCount, failed_count: reconciliationCount, pending_count: reconciliationCount,
    created_by: ref("UUID"), reason: ref("FinanceReconciliationReason"), created_at: ref("DateTime"), started_at: nullable(ref("DateTime")), completed_at: nullable(ref("DateTime")), last_error_code: nullable(str({ const: "CHECK_FAILED" })), can_retry: bool, creation_audit_log_id: ref("UUID"),
  }),
  FinanceReconciliationJobPage: object({ brand_id: ref("UUID"), items: array(ref("FinanceReconciliationJob")), total_count: aggregateInteger, limit: int({ minimum: 1, maximum: 100 }), offset: int({ minimum: 0, maximum: 1000000 }) }),
  FinanceReconciliationTarget: object({
    id: ref("UUID"), brand_id: ref("UUID"), job_id: ref("UUID"), account_id: ref("UUID"), member_id: ref("UUID"), state: str({ enum: ["pending", "checked", "failed"] }), outcome: nullable(str({ enum: ["consistent", "repairable", "corrupt"] })), preview: nullable(ref("FinanceRepairPreview")), attempt_count: int({ minimum: 0 }), error_code: nullable(str({ const: "CHECK_FAILED" })), checked_at: nullable(ref("DateTime")), audit_log_id: nullable(ref("UUID")),
  }),
  FinanceReconciliationTargetPage: object({ brand_id: ref("UUID"), job_id: ref("UUID"), items: array(ref("FinanceReconciliationTarget")), total_count: reconciliationCount, limit: int({ minimum: 1, maximum: 100 }), offset: int({ minimum: 0, maximum: 1000000 }), outcome: nullable(str({ enum: ["pending", "failed", "consistent", "repairable", "corrupt"] })) }),
  FinanceReconciliationCreateInput: object({ reason: ref("FinanceReconciliationReason") }),
  FinanceReconciliationRetryInput: object({ version: int({ minimum: 1, maximum: Number.MAX_SAFE_INTEGER }), reason: ref("FinanceReconciliationReason") }),

  FinanceWithdrawalBrandConfig: object({ enabled: bool, min_points: ref("PositiveInt64String"), max_points: nullable(ref("PositiveInt64String")), allowed_sources: array(str({ enum: ["recharge", "winning", "gift"] })), review_mode: str({ enum: ["manual", "automatic"] }), turnover_multiple: str({ pattern: "^(0|[1-9][0-9]{0,6})([.][0-9]{0,5}[1-9])?$" }) }),
  FinanceWithdrawalGameConfig: object({ turnover_multiple: nullable(str({ pattern: "^(0|[1-9][0-9]{0,6})([.][0-9]{0,5}[1-9])?$" })) }),
  FinanceWithdrawalPositiveMultiple: str({ pattern: "^(?!0$)(?:1000000|(?:0|[1-9][0-9]{0,5})(?:[.][0-9]{0,5}[1-9])?)$", description: "New policy writes require 0 < N <= 1000000, in canonical decimal notation with at most six fractional digits. Legacy zero-valued saved policies and immutable revisions remain readable without normalization." }),
  FinanceWithdrawalBrandWriteConfig: object({ enabled: bool, min_points: ref("PositiveInt64String"), max_points: nullable(ref("PositiveInt64String")), allowed_sources: array(str({ enum: ["recharge", "winning", "gift"] })), review_mode: str({ enum: ["manual", "automatic"] }), turnover_multiple: ref("FinanceWithdrawalPositiveMultiple") }),
  FinanceWithdrawalGameWriteConfig: object({ turnover_multiple: nullable(ref("FinanceWithdrawalPositiveMultiple")) }),
  FinanceWithdrawalBrandPolicy: object({ brand_id: ref("UUID"), version: int(), config: ref("FinanceWithdrawalBrandConfig"), updated_at: ref("DateTime"), audit_log_id: ref("UUID") }, ["brand_id", "version", "config", "updated_at"]),
  FinanceWithdrawalEffectiveMultiple: object({ turnover_multiple: str(), source: str({ enum: ["brand", "game"] }), brand_version: int(), game_version: int() }),
  FinanceWithdrawalGamePolicy: object({ brand_id: ref("UUID"), game_id: ref("UUID"), version: int(), config: ref("FinanceWithdrawalGameConfig"), effective: ref("FinanceWithdrawalEffectiveMultiple"), updated_at: ref("DateTime"), audit_log_id: ref("UUID") }, ["brand_id", "game_id", "version", "config", "effective", "updated_at"]),
  FinanceWithdrawalBrandInput: object({ version: int({ minimum: 1 }), config: ref("FinanceWithdrawalBrandWriteConfig"), reason: ref("Reason") }),
  FinanceWithdrawalGameInput: object({ version: int({ minimum: 1 }), config: ref("FinanceWithdrawalGameWriteConfig"), reason: ref("Reason") }),
  FinanceWithdrawalRevision: object({ id: ref("UUID"), brand_id: ref("UUID"), game_id: str(), version: int(), config: { oneOf: [ref("FinanceWithdrawalBrandConfig"), ref("FinanceWithdrawalGameConfig")] }, changed_by: str(), reason: str(), created_at: ref("DateTime") }),
  FinanceWithdrawalHistory: object({ items: array(ref("FinanceWithdrawalRevision")), limit: int(), offset: int() }),

  FinanceBettingTotals: object({ order_count: aggregateInteger, stake_points: aggregateInteger, placed_count: aggregateInteger, won_count: aggregateInteger, lost_count: aggregateInteger, abnormal_count: aggregateInteger, cancelled_count: aggregateInteger, refund_points: aggregateInteger, settled_stake_points: aggregateInteger, unfinalized_stake_points: aggregateInteger, abnormal_stake_points: aggregateInteger, current_prize_points: aggregateInteger, correction_open_count: aggregateInteger }),
  FinanceLedgerTotals: object({ entry_count: aggregateInteger, net_points: aggregateSignedInteger, recharge_points: aggregateInteger, prize_credit_points: aggregateInteger, prize_reversal_points: aggregateInteger, refund_points: aggregateInteger }),
  FinanceReportBalances: object({ account_count: aggregateInteger, available_points: aggregateInteger, frozen_points: aggregateInteger, withdrawal_points: aggregateInteger, total_points: aggregateInteger }),
  FinanceReportQuery: object({ from: ref("DateTime"), to: ref("DateTime"), group_by: str({ enum: ["day", "game", "member", "entry_type"] }), limit: int(), offset: int(), game_id: nullable(ref("UUID")), member_id: nullable(ref("UUID")) }),
  FinanceBettingGroup: object({ key: str(), label: str(), totals: ref("FinanceBettingTotals") }),
  FinanceLedgerGroup: object({ key: str(), label: str(), totals: ref("FinanceLedgerTotals") }),
  FinanceBettingReport: object({ brand_id: ref("UUID"), snapshot_at: ref("DateTime"), timezone: str(), query: ref("FinanceReportQuery"), summary: ref("FinanceBettingTotals"), items: array(ref("FinanceBettingGroup")), total_groups: aggregateInteger }),
  FinanceLedgerReport: object({ brand_id: ref("UUID"), snapshot_at: ref("DateTime"), timezone: str(), query: ref("FinanceReportQuery"), summary: ref("FinanceLedgerTotals"), items: array(ref("FinanceLedgerGroup")), total_groups: aggregateInteger, balances: ref("FinanceReportBalances") }),

  FinanceAgentPolicyConfig: object({ enabled: bool, max_depth: int({ minimum: 1, maximum: 32 }), ratio_cap: str({ pattern: "^(0|1|0\\.[0-9]{0,5}[1-9])$" }), mode: str({ enum: ["loss", "turnover"] }), cycle: str({ enum: ["weekly", "monthly"] }) }),
  FinanceAgentNodeConfig: object({ ratio: str({ pattern: "^(0|1|0\\.[0-9]{0,5}[1-9])$" }), mode: nullable(str({ enum: ["loss", "turnover"] })), status: str({ enum: ["active", "disabled"] }), can_create_children: bool }),
  FinanceAgentPolicy: object({ brand_id: ref("UUID"), version: int(), config: ref("FinanceAgentPolicyConfig"), updated_at: ref("DateTime"), audit_log_id: ref("UUID") }, ["brand_id", "version", "config", "updated_at"]),
  FinanceAgentNode: object({ id: ref("UUID"), brand_id: ref("UUID"), member_id: ref("UUID"), parent_id: nullable(ref("UUID")), depth: int(), path: array(ref("UUID")), version: int(), config: ref("FinanceAgentNodeConfig"), effective_mode: str({ enum: ["loss", "turnover"] }), mode_source_agent_id: nullable(ref("UUID")), policy_version: int(), parent_version: nullable(int()), created_by: ref("UUID"), created_at: ref("DateTime"), updated_at: ref("DateTime"), audit_log_id: ref("UUID") }, ["id", "brand_id", "member_id", "parent_id", "depth", "path", "version", "config", "effective_mode", "mode_source_agent_id", "policy_version", "parent_version", "created_by", "created_at", "updated_at"]),
  FinanceAgentPolicyInput: object({ version: int({ minimum: 1 }), config: ref("FinanceAgentPolicyConfig"), reason: ref("Reason") }),
  FinanceAgentCreateInput: object({ policy_version: int({ minimum: 1 }), member_id: ref("UUID"), parent_id: nullable(ref("UUID")), parent_version: nullable(int({ minimum: 1 })), config: ref("FinanceAgentNodeConfig"), reason: ref("Reason") }),
  FinanceAgentUpdateInput: object({ version: int({ minimum: 1 }), policy_version: int({ minimum: 1 }), parent_version: nullable(int({ minimum: 1 })), config: ref("FinanceAgentNodeConfig"), reason: ref("Reason") }),
  FinanceAgentChildUpdateInput: object({ version: int({ minimum: 1 }), policy_version: int({ minimum: 1 }), parent_version: int({ minimum: 1 }), ratio: str({ pattern: "^(0|1|0\\.[0-9]{0,5}[1-9])$" }), mode: nullable(str({ enum: ["loss", "turnover"] })), reason: ref("Reason") }),
  FinanceAgentTree: object({ brand_id: ref("UUID"), parent_id: nullable(ref("UUID")), items: array(ref("FinanceAgentNode")), limit: int(), offset: int(), total_count: str() }),
  FinanceAgentRevision: object({ id: ref("UUID"), brand_id: ref("UUID"), agent_id: nullable(ref("UUID")), version: int(), config: { oneOf: [ref("FinanceAgentPolicyConfig"), ref("FinanceAgentNodeConfig")] }, actor_type: str(), actor_id: nullable(ref("UUID")), reason: str(), created_at: ref("DateTime"), audit_log_id: nullable(ref("UUID")) }),
  FinanceAgentHistory: object({ brand_id: ref("UUID"), agent_id: nullable(ref("UUID")), items: array(ref("FinanceAgentRevision")), limit: int(), offset: int(), total_count: str() }),

  FinanceJoinCode: object({ id: ref("UUID"), brand_id: ref("UUID"), kind: str({ enum: ["agent", "referral"] }), code: str({ pattern: "^[A-F0-9]{24}$" }), owner_member_id: ref("UUID"), agent_id: nullable(ref("UUID")), status: str({ enum: ["active", "disabled"] }), starts_at: nullable(ref("DateTime")), expires_at: nullable(ref("DateTime")), version: int(), usable: bool, created_at: ref("DateTime"), updated_at: ref("DateTime"), audit_log_id: ref("UUID") }, ["id", "brand_id", "kind", "code", "owner_member_id", "agent_id", "status", "starts_at", "expires_at", "version", "usable", "created_at", "updated_at"]),
  FinanceJoinCodeCreateInput: object({ kind: str({ enum: ["agent", "referral"] }), owner_member_id: ref("UUID"), agent_id: nullable(ref("UUID")), starts_at: nullable(ref("DateTime")), expires_at: nullable(ref("DateTime")), reason: ref("Reason") }),
  FinanceJoinCodeUpdateInput: object({ version: int({ minimum: 1 }), status: str({ enum: ["active", "disabled"] }), starts_at: nullable(ref("DateTime")), expires_at: nullable(ref("DateTime")), reason: ref("Reason") }),
  FinanceJoinCodePage: object({ brand_id: ref("UUID"), kind: nullable(str({ enum: ["agent", "referral"] })), owner_member_id: nullable(ref("UUID")), items: array(ref("FinanceJoinCode")), limit: int(), offset: int(), total_count: str() }),
  FinanceJoinCodeSelfPage: object({ brand_id: ref("UUID"), member_id: ref("UUID"), items: array(ref("FinanceJoinCode")), limit: int(), offset: int(), total_count: str() }),
  FinanceJoinCodeRevision: object({ id: ref("UUID"), brand_id: ref("UUID"), code_id: ref("UUID"), version: int(), status: str({ enum: ["active", "disabled"] }), starts_at: nullable(ref("DateTime")), expires_at: nullable(ref("DateTime")), actor_id: ref("UUID"), reason: str(), audit_log_id: ref("UUID"), created_at: ref("DateTime") }),
  FinanceJoinCodeHistory: object({ brand_id: ref("UUID"), code_id: ref("UUID"), items: array(ref("FinanceJoinCodeRevision")), limit: int(), offset: int(), total_count: str() }),
  FinanceAttribution: object({ brand_id: ref("UUID"), member_id: ref("UUID"), join_method: str(), joined_at: ref("DateTime"), code_id: nullable(ref("UUID")), source_code: nullable(str()), legacy: bool }),
};

const admin = (method, path, operationId, summary, tag, data, permission, extra = {}) => ({
  method, path: `/api/v1/admin${path}`, operationId, summary, tag, auth: "admin", brandHeader: true,
  permissions: adminReadPermissions(permission), ...(data === undefined ? {} : { data }), ...extra,
});
const write = (method, path, operationId, summary, tag, body, data, permission, extra = {}) => ({
  ...admin(method, path, operationId, summary, tag, data, permission, extra), requestBody: ref(body), idempotency: true,
  permissions: extra.permissions ?? adminWritePermissions(permission),
});
const user = (method, path, operationId, summary, tag, data, extra = {}) => ({ method, path: `/api/v1${path}`, operationId, summary, tag, auth: "user", data, ...extra });
const query = (name, schema, required = false, description) => ({ name, in: "query", required, schema, ...(description ? { description } : {}) });

export const operations = [
  user("GET", "/wallet", "getMyFinanceWallet", "Get the authenticated member's wallet", "finance", ref("FinanceWallet")),
  user("GET", "/wallet/ledger", "listMyFinanceLedger", "List the authenticated member's ledger entries", "finance", ref("FinanceWalletLedgerPage"), { parameters: pageParameters }),
  admin("GET", "/wallets/{memberID}", "getMemberWallet", "Get a member wallet", "finance", ref("FinanceWallet"), "wallet.view.brand", { description: "Super-admin accounts are blocked from finance writes; this read requires wallet.view for the selected brand." }),
  admin("GET", "/wallets/{memberID}/ledger", "listMemberWalletLedger", "List a member's immutable ledger entries", "finance", ref("FinanceWalletLedgerPage"), "wallet.view.brand", { parameters: pageParameters }),
  admin("GET", "/wallets/{memberID}/reconciliation", "reconcileMemberWallet", "Compare materialized buckets with the immutable ledger", "finance", ref("FinanceReconciliation"), "wallet.view.brand", { description: "Read-only reconciliation. It reports mismatches and does not repair balances." }),
  admin("GET", "/recharges", "listRecharges", "List recharge records", "finance", ref("FinanceRechargeList"), "recharge.view.brand", { parameters: [...pageParameters, query("member_id", ref("UUID"))] }),
  write("POST", "/recharges", "createRecharge", "Create a pending manual recharge record", "finance", "FinanceRechargeCreate", ref("FinanceRecharge"), "recharge.write.brand", { successStatus: 201, description: "Creates a pending record; points are not credited until confirmation. A confirmed recharge is posted to the immutable points ledger. Super-admin accounts cannot perform finance writes." }),
  write("POST", "/recharges/{id}/confirm", "confirmRecharge", "Confirm a pending recharge", "finance", "FinanceVersionReason", ref("FinanceRecharge"), "recharge.write.brand", { description: "Requires the current version and a reason. Confirmation posts the recharge points once. Super-admin accounts cannot perform finance writes." }),
  write("POST", "/recharges/{id}/cancel", "cancelRecharge", "Cancel a pending recharge", "finance", "FinanceVersionReason", ref("FinanceRecharge"), "recharge.write.brand", { description: "Only a pending recharge at the supplied version can be cancelled; cancellation does not post points. Super-admin accounts cannot perform finance writes." }),
  write("POST", "/wallets/{memberID}/freeze", "freezeMemberPoints", "Move available points into manual frozen state", "finance", "FinanceFreeze", ref("FinanceEntry"), "wallet.freeze.brand", { description: "Allocates only available points across the three source buckets. This does not freeze system or withdrawal-state points. Super-admin accounts cannot perform finance writes." }),
  write("POST", "/wallets/{memberID}/unfreeze", "unfreezeMemberPoints", "Reverse an eligible manual freeze entry", "finance", "FinanceUnfreeze", ref("FinanceEntry"), "wallet.freeze.brand", { description: "The referenced entry must be a manual freeze; it is reversed using its original allocation. Super-admin accounts cannot perform finance writes." }),
  write("POST", "/wallets/{memberID}/adjust", "adjustMemberPoints", "Apply a signed manual point adjustment", "finance", "FinanceAdjust", ref("FinanceEntry"), "wallet.adjust.brand", { description: "Delta is a nonzero signed decimal int64 string and is applied only to the selected source's available bucket. Current point policy limits are enforced. Super-admin accounts cannot perform finance writes." }),
  admin("GET", "/point-policy", "getPointPolicy", "Get brand point adjustment caps", "finance", ref("FinanceAdjustmentPolicy"), "point_policy.view.brand"),
  write("PUT", "/point-policy", "updatePointPolicy", "Replace brand point adjustment caps", "finance", "FinanceAdjustmentPolicyInput", ref("FinanceAdjustmentPolicy"), "point_policy.write.brand", { description: "Complete replacement: all three cap fields are required; null clears a cap. Version must match. Super-admin accounts cannot perform finance writes." }),
  admin("GET", "/wallets/{memberID}/repair-preview", "previewWalletRepair", "Preview a materialized wallet balance repair", "finance", ref("FinanceRepairPreview"), "wallet.view.brand", { description: "Computes expected balances from the immutable ledger. The returned token and versions bind a later repair to this preview." }),
  write("POST", "/wallets/{memberID}/repair", "repairWalletBalance", "Rebuild materialized wallet buckets from an intact ledger", "finance", "FinanceRepairInput", ref("FinanceRepairRecord"), "wallet.repair.brand", { description: "Requires the current preview version and token, a nonempty reason, and an intact ledger. Rebuilds buckets; it does not create an economic credit or rewrite ledger entries. Super-admin accounts cannot perform finance writes." }),
  admin("GET", "/wallets/{memberID}/repairs", "listWalletRepairs", "List wallet repair history", "finance", ref("FinanceRepairList"), "wallet.view.brand", { parameters: pageParameters }),

  admin("GET", "/reconciliations", "listWalletReconciliations", "List durable wallet reconciliation jobs", "finance", ref("FinanceReconciliationJobPage"), "wallet.view.brand", { permissions: ["wallet.view.brand", "wallet.view.platform"], parameters: [{ name: "limit", in: "query", required: false, schema: int({ default: 20, minimum: 1, maximum: 100 }) }, { name: "offset", in: "query", required: false, schema: int({ default: 0, minimum: 0, maximum: 1000000 }) }], description: "Requires wallet.view.brand with selected-brand membership, or explicit wallet.view.platform independently of membership. Super-admin status alone grants no access. Reads use the primary database, revalidate authorization, and commit an audit record before responding. Counters and total_count are exact decimal strings for this job only; there are no global wallet segments." }),
  write("POST", "/reconciliations", "createWalletReconciliation", "Create a pending wallet reconciliation job", "finance", "FinanceReconciliationCreateInput", ref("FinanceReconciliationJob"), "wallet.reconcile.brand", { permissions: ["wallet.view.brand", "wallet.view.platform", "wallet.reconcile.brand"], successStatus: 201, additionalErrorStatuses: [413], description: "Requires wallet.reconcile.brand and either wallet.view.brand or wallet.view.platform. Brand-scoped grants must match explicit brand membership; platform view does not bypass brand membership. Super-admins are read-only. The only request field is reason. Captures the selected account/member targets transactionally and creates a frozen original pending v1 receipt. An active job returns 409; more than 100000 targets returns 413 atomically without creating a job. Reason must already be trimmed and contain no LF, CR, or NUL. Workers record per-account, nonfinancial bucket-versus-ledger observations; the service does not repair balances or expose ledger segments. Domain errors use RECONCILIATION_INPUT_INVALID, RECONCILIATION_STATE_CONFLICT, and RECONCILIATION_TOO_LARGE." }),
  admin("GET", "/reconciliations/{id}", "getWalletReconciliation", "Get current wallet reconciliation job state", "finance", ref("FinanceReconciliationJob"), "wallet.view.brand", { permissions: ["wallet.view.brand", "wallet.view.platform"], description: "Requires wallet.view.brand with selected-brand membership, or explicit wallet.view.platform independently of membership. Super-admin status alone grants no access. Reads current state from the primary database and commits an audit record. The POST creation receipt remains frozen at pending version 1; this endpoint returns current counters and state. Counts are job-local diagnostic observations, not global wallet segments." }),
  admin("GET", "/reconciliations/{id}/targets", "listWalletReconciliationTargets", "List diagnostic observations for a wallet reconciliation job", "finance", ref("FinanceReconciliationTargetPage"), "wallet.view.brand", { permissions: ["wallet.view.brand", "wallet.view.platform"], parameters: [{ name: "limit", in: "query", required: false, schema: int({ default: 20, minimum: 1, maximum: 100 }) }, { name: "offset", in: "query", required: false, schema: int({ default: 0, minimum: 0, maximum: 1000000 }) }, query("outcome", str({ enum: ["pending", "failed", "consistent", "repairable", "corrupt"] }))], description: "Requires wallet.view.brand with selected-brand membership, or explicit wallet.view.platform independently of membership. Super-admin status alone grants no access. Reads the primary database and commits an audit record. Pagination defaults to limit 20/offset 0, maximum limit 100 and offset 1000000. outcome filters pending/failed target states or the checked observation outcome. Totals are decimal strings scoped to this job; observations do not expose global wallet segments or repair balances." }),
  write("POST", "/reconciliations/{id}/retry", "retryWalletReconciliation", "Retry failed checks in a wallet reconciliation job", "finance", "FinanceReconciliationRetryInput", ref("FinanceReconciliationJob"), "wallet.reconcile.brand", { permissions: ["wallet.view.brand", "wallet.view.platform", "wallet.reconcile.brand"], description: "Requires wallet.reconcile.brand and either wallet.view.brand or wallet.view.platform; brand-scoped grants require explicit brand membership. Super-admins are read-only. Requires the current version and a reason. Returns the new pending version while preserving the earliest started_at (including null) and leaving completed checks untouched. Requests use checked idempotency; an idempotent replay returns its original receipt. Domain errors use RECONCILIATION_INPUT_INVALID, RECONCILIATION_NOT_FOUND, RECONCILIATION_STATE_CONFLICT, and RECONCILIATION_VERSION_CONFLICT." }),

  admin("GET", "/withdrawal-policy", "getBrandWithdrawalPolicy", "Get brand withdrawal policy configuration", "finance", ref("FinanceWithdrawalBrandPolicy"), "withdrawal_policy.view.brand", { description: "Policy configuration only; this API does not calculate withdrawal eligibility, create withdrawal orders, or transfer points." }),
  admin("GET", "/withdrawal-policy/history", "listBrandWithdrawalPolicyHistory", "List brand withdrawal policy revisions", "finance", ref("FinanceWithdrawalHistory"), "withdrawal_policy.view.brand", { parameters: pageParameters, description: "Policy revision history only; no withdrawal orders or eligibility data are exposed." }),
  write("PUT", "/withdrawal-policy", "updateBrandWithdrawalPolicy", "Replace brand withdrawal policy", "finance", "FinanceWithdrawalBrandInput", ref("FinanceWithdrawalBrandPolicy"), "withdrawal_policy.write.brand", { description: "Versioned full replacement of policy configuration. This module stores rules only; it does not evaluate eligibility, create orders, or transfer points. Super-admin accounts are forbidden from this write." }),
  admin("GET", "/games/{id}/withdrawal-policy", "getGameWithdrawalPolicy", "Get game withdrawal policy configuration", "finance", ref("FinanceWithdrawalGamePolicy"), "withdrawal_policy.view.brand", { description: "Returns the game override and its effective turnover multiple. Policy configuration only; no eligibility, order, or transfer operations." }),
  admin("GET", "/games/{id}/withdrawal-policy/history", "listGameWithdrawalPolicyHistory", "List game withdrawal policy revisions", "finance", ref("FinanceWithdrawalHistory"), "withdrawal_policy.view.brand", { parameters: pageParameters, description: "Policy revision history only; no withdrawal orders or eligibility data are exposed." }),
  write("PUT", "/games/{id}/withdrawal-policy", "updateGameWithdrawalPolicy", "Replace game withdrawal policy override", "finance", "FinanceWithdrawalGameInput", ref("FinanceWithdrawalGamePolicy"), "withdrawal_policy.write.brand", { description: "Versioned replacement of the game override. turnover_multiple may be null to inherit the brand value. Rules only; no eligibility, orders, or point transfers. Super-admin accounts are forbidden from this write." }),

  admin("GET", "/reports/betting", "getBettingReport", "Read a snapshot-consistent betting report", "reporting", ref("FinanceBettingReport"), "report_betting.view.brand", { parameters: [query("from", ref("DateTime"), true), query("to", ref("DateTime"), true), query("group_by", str({ enum: ["day", "game", "member"] }), true), ...reportPageParameters, query("game_id", ref("UUID")), query("member_id", ref("UUID"))], description: "Read-only report. from/to are RFC3339 timestamps defining a half-open range of at most 93 days. Unknown, repeated, or inapplicable filters are rejected. The report is audited; it does not expose member names, recharge proof, or ledger payloads." }),
  admin("GET", "/reports/ledger", "getLedgerReport", "Read a snapshot-consistent ledger report", "reporting", ref("FinanceLedgerReport"), "report_ledger.view.brand", { parameters: [query("from", ref("DateTime"), true), query("to", ref("DateTime"), true), query("group_by", str({ enum: ["day", "entry_type"] }), true), ...reportPageParameters, query("member_id", ref("UUID"))], description: "Read-only report. from/to are RFC3339 timestamps defining a half-open range of at most 93 days. game_id is not accepted; unknown, repeated, or inapplicable filters are rejected. The report is audited and excludes ledger reasons, proof references, and source allocations." }),

  ...["betting", "ledger"].map((kind) => {
    const queryParameters = [
      query("from", ref("DateTime"), true, "RFC3339 start of the half-open report interval."),
      query("to", ref("DateTime"), true, "RFC3339 end of the half-open report interval; no more than 93 days after from."),
      query("group_by", str({ enum: kind === "betting" ? ["day", "game", "member"] : ["day", "entry_type"] }), true),
      ...(kind === "betting" ? [query("game_id", ref("UUID"))] : []),
      query("member_id", ref("UUID")),
    ];
    const headers = {
      "Content-Type": { description: "UTF-8 CSV response.", schema: { type: "string", const: "text/csv; charset=utf-8" } },
      "Content-Disposition": { description: "Attachment filename includes the report kind, brand ID, and UTC snapshot time.", schema: { type: "string", pattern: '^attachment; filename="lottery-(betting|ledger)-[0-9a-f-]{36}-[0-9]{8}T[0-9]{6}Z[.]csv"$' } },
      "Content-Length": { description: "Exact response body length in bytes.", schema: { type: "string", pattern: "^(0|[1-9][0-9]*)$" } },
      "X-Content-Type-Options": { description: "Prevents MIME sniffing.", schema: { type: "string", const: "nosniff" } },
      "X-Report-Brand-ID": { description: "UUID of the selected brand.", schema: ref("UUID") },
      "X-Report-Kind": { description: "Exported report kind.", schema: { type: "string", const: kind } },
      "X-Report-Snapshot-At": { description: "UTC timestamp for the single SQL statement snapshot.", schema: ref("DateTime") },
      "X-Report-Group-Count": { description: "Exact number of groups included; exports above 10000 groups fail without a truncated file.", schema: { type: "string", pattern: "^(0|[1-9][0-9]*)$" } },
      "X-Report-SHA256": { description: "Lowercase SHA-256 digest of the complete CSV response body.", schema: { type: "string", pattern: "^[0-9a-f]{64}$" } },
      "X-Report-Format-Version": { description: "CSV format version.", schema: { type: "string", const: "1" } },
      "X-Report-Audit-ID": { description: "Committed audit record for this export.", schema: ref("UUID") },
    };
    return admin("GET", `/reports/${kind}/export`, `export${kind[0].toUpperCase()}${kind.slice(1)}Report`, `Export a complete ${kind} report as CSV`, "reporting", undefined, `report_${kind}.view.brand`, {
      parameters: queryParameters,
      permissions: [`report_${kind}.view.brand`, `report_${kind}.export.brand`, `report_${kind}.view.platform`, `report_${kind}.export.platform`],
      additionalErrorStatuses: [413],
      successContent: { "text/csv": { schema: { type: "string", format: "binary", description: "Complete UTF-8 CSV byte stream with a BOM and LF record endings. Exact integer cells are preserved as decimal text; spreadsheet applications must import them as TEXT to avoid rounding." } } },
      successDescription: "Complete CSV file. The report data is read from one SQL statement snapshot. The response is capped at 10000 groups and 4 MiB; exceeding either bound returns JSON 413 and never a truncated file. The audit record is committed before any response bytes are written.",
      successHeaders: headers,
      description: `Requires (report_${kind}.view.brand OR report_${kind}.view.platform) AND (report_${kind}.export.brand OR report_${kind}.export.platform). Each brand-scoped grant must match the selected brand membership; platform-scoped grants are evaluated at platform scope. Authorization is revalidated during the export. Accepts only from, to, group_by${kind === "betting" ? ", game_id" : ""}, and member_id; limit and offset pagination are not accepted. from/to define a half-open RFC3339 interval of at most 93 days. The CSV is audited and contains no truncation.`,
    });
  }),

  admin("GET", "/agent-policy", "getAgentPolicy", "Get brand agent policy", "agency", ref("FinanceAgentPolicy"), "agent_policy.view.brand", { description: "Agent policies configure hierarchy and ratio settings only. The agency service does not calculate, accrue, or pay commissions, or change wallets." }),
  write("PUT", "/agent-policy", "updateAgentPolicy", "Replace brand agent policy", "agency", "FinanceAgentPolicyInput", ref("FinanceAgentPolicy"), "agent_policy.write.brand", { description: "Versioned policy replacement with reason. Agent configuration only; it does not calculate or pay commissions. Super-admin accounts are forbidden from this write." }),
  admin("GET", "/agent-policy/history", "listAgentPolicyHistory", "List brand agent policy revisions", "agency", ref("FinanceAgentHistory"), "agent_policy.view.brand", { parameters: pageParameters }),
  admin("GET", "/agents/{id}/history", "listAgentHistory", "List agent configuration revisions", "agency", ref("FinanceAgentHistory"), "agent.view.brand", { parameters: pageParameters }),
  admin("GET", "/agents/tree", "getAgentTree", "List agents under an optional parent", "agency", ref("FinanceAgentTree"), "agent.view.brand", { parameters: [...pageParameters, query("parent_id", ref("UUID"))] }),
  admin("GET", "/agents/{id}", "getAgent", "Get an agent node", "agency", ref("FinanceAgentNode"), "agent.view.brand"),
  write("POST", "/agents", "createAgent", "Create an agent node", "agency", "FinanceAgentCreateInput", ref("FinanceAgentNode"), "agent.write.brand", { successStatus: 201, description: "Requires the current policy version and, when parent_id is supplied, parent_version. Ratio, depth, parent permissions, and policy constraints are checked. Configuration only; no commission accrual or payment. Super-admin accounts are forbidden from this write." }),
  write("PUT", "/agents/{id}", "updateAgent", "Replace agent node configuration", "agency", "FinanceAgentUpdateInput", ref("FinanceAgentNode"), "agent.write.brand", { description: "Requires current node, policy, and applicable parent versions. Agent configuration only; no commission accrual or payment. Super-admin accounts are forbidden from this write." }),
  user("GET", "/agent/me", "getMyAgentNode", "Get the authenticated member's agent node", "agency", ref("FinanceAgentNode")),
  user("GET", "/agent/children", "listMyDirectAgentChildren", "List the authenticated agent's direct children", "agency", ref("FinanceAgentTree"), { parameters: pageParameters }),
  user("PUT", "/agent/children/{id}/config", "updateMyDirectAgentChildConfig", "Update configuration for a direct child agent", "agency", ref("FinanceAgentNode"), { requestBody: ref("FinanceAgentChildUpdateInput"), idempotency: true, description: "Authenticated agent may update only an authorized direct child. The request is version-checked and does not calculate or pay commissions or change wallets." }),

  admin("GET", "/join-codes", "listJoinCodes", "List brand join codes", "agency", ref("FinanceJoinCodePage"), "join_code.view.brand", { parameters: [...joinPageParameters, query("kind", str({ enum: ["agent", "referral"] })), query("owner_member_id", ref("UUID"))] }),
  write("POST", "/join-codes", "createJoinCode", "Create a join code", "agency", "FinanceJoinCodeCreateInput", ref("FinanceJoinCode"), "join_code.write.brand", { successStatus: 201, description: "A random join code is generated by the service. Optional agent_id must identify the owner's agent node. Reason is stored in audited history; no credential or secret is accepted." }),
  admin("GET", "/join-codes/{id}", "getJoinCode", "Get a join code", "agency", ref("FinanceJoinCode"), "join_code.view.brand"),
  write("PUT", "/join-codes/{id}", "updateJoinCode", "Update join code status and active window", "agency", "FinanceJoinCodeUpdateInput", ref("FinanceJoinCode"), "join_code.write.brand"),
  admin("GET", "/join-codes/{id}/history", "listJoinCodeHistory", "List join code revisions", "agency", ref("FinanceJoinCodeHistory"), "join_code.view.brand", { parameters: joinPageParameters }),
  user("GET", "/me/join-codes", "listMyJoinCodes", "List join codes owned by the authenticated member", "agency", ref("FinanceJoinCodeSelfPage"), { parameters: joinPageParameters }),
  user("GET", "/me/attribution", "getMyAttribution", "Get the authenticated member's recorded attribution", "agency", ref("FinanceAttribution")),
];
