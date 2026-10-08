const ref = name => ({ $ref: `#/components/schemas/${name}` });
const obj = (properties, required = Object.keys(properties)) => ({
  type: "object", properties, required, additionalProperties: false,
});
const amount = { type: "string", pattern: "^(0|[1-9][0-9]*)$", description: "Canonical nonnegative decimal integer string with no int64 aggregate bound." };
const count = amount;
const dateTime = ref("DateTime");
const uuid = ref("UUID");
const str = { type: "string" };
const nullable = schema => ({ anyOf: [schema, { type: "null" }] });

const section = (name, permission, data) => ({
  ...obj({
    status: { type: "string", enum: ["ready", "forbidden"] },
    data: { anyOf: [
      { allOf: [data, { type: "object" }], description: "Present only when status is ready." },
      { type: "null", description: "Required when status is forbidden or not_implemented." },
    ] },
  }),
  description: `${name} requires CanView resource "${permission}" for the exact selected brand or an explicit ${permission}.view.platform grant. It is populated only for ready status; forbidden and not_implemented return null data.`,
  allOf: [
    { if: { properties: { status: { const: "ready" } }, required: ["status"] }, then: { properties: { data } } },
    { if: { properties: { status: { const: "forbidden" } }, required: ["status"] }, then: { properties: { data: { type: "null" } } } },
  ],
});

const brand = obj({ name: str, code: str, state: { type: "string", enum: ["active", "paused", "disabled"] } });
const periods = obj({ pending: count, betting: count, closed: count, waiting_draw: count, drawn: count, settling: count, refund_pending: count, refund_failed: count });
const orders = obj({ placed: count, abnormal: count });
const todayBets = obj({ order_count: count, stake_points: amount, cancelled_count: count, abnormal_count: count });
const settlement = obj({ processing: count, awaiting_approval: count, paying: count, failed: count });
const recharges = obj({ pending_count: count, pending_points: amount });
const ledger = obj({ entry_count: count, net_points: { type: "string", pattern: "^(0|-?[1-9][0-9]*)$", description: "Canonical signed decimal integer string with no int64 aggregate bound." }, recharge_points: amount, prize_credit_points: amount, prize_reversal_points: amount, refund_points: amount });
const balances = obj({ account_count: count, available_points: amount, frozen_points: amount, withdrawal_points: amount, total_points: amount });
const reconciliationJob = obj({ id: uuid, state: { type: "string", enum: ["pending", "running", "completed", "failed"] }, created_at: dateTime, completed_at: nullable(dateTime), target_count: count, checked_count: count, repairable_count: count, corrupt_count: count, failed_count: count });
const reconciliation = obj({ latest_job: nullable(ref("AdminWorkbenchReconciliationJob")) });
const sources = obj({ adapter_state: { type: "string", const: "stub" }, configured_games: count, enabled_api_sources: count, enabled_dom_sources: count, attempts_today: count, failed_today: count, no_data_today: count, last_attempt_at: nullable(dateTime) });
const withdrawals = obj({ reviewing_count: count, reviewing_points: amount, processing_count: count, processing_points: amount });
const unavailable = status => ({
  type: "object", properties: { status: { const: status }, data: { type: "null" } },
  required: ["status", "data"], additionalProperties: false,
});

export const schemas = {
  AdminWorkbenchBrand: brand,
  AdminWorkbenchPeriods: periods,
  AdminWorkbenchOrders: orders,
  AdminWorkbenchTodayBets: todayBets,
  AdminWorkbenchSettlement: settlement,
  AdminWorkbenchRecharges: recharges,
  AdminWorkbenchLedger: ledger,
  AdminWorkbenchBalances: balances,
  AdminWorkbenchReconciliationJob: reconciliationJob,
  AdminWorkbenchReconciliation: reconciliation,
  AdminWorkbenchSources: sources,
  AdminWorkbenchWithdrawals: withdrawals,
  AdminWorkbenchRewards: obj({ granted_count: count, pending_count: count, revoked_count: count }),
  AdminWorkbenchSnapshot: obj({
    brand_id: uuid, snapshot_at: dateTime, timezone: str, day_from: dateTime,
    brand: ref("AdminWorkbenchBrandSection"), periods: ref("AdminWorkbenchPeriodsSection"),
    orders: ref("AdminWorkbenchOrdersSection"), today_bets: ref("AdminWorkbenchTodayBetsSection"),
    settlement: ref("AdminWorkbenchSettlementSection"), recharges: ref("AdminWorkbenchRechargesSection"),
    ledger: ref("AdminWorkbenchLedgerSection"), balances: ref("AdminWorkbenchBalancesSection"),
    reconciliation: ref("AdminWorkbenchReconciliationSection"), sources: ref("AdminWorkbenchSourcesSection"),
    withdrawals: ref("AdminWorkbenchWithdrawalsSection"), commissions: ref("AdminWorkbenchCommissionsSection"),
    rewards: ref("AdminWorkbenchRewardsSection"),
  }),
  AdminWorkbenchBrandSection: section("brand", "brand", ref("AdminWorkbenchBrand")),
  AdminWorkbenchPeriodsSection: section("periods", "period", ref("AdminWorkbenchPeriods")),
  AdminWorkbenchOrdersSection: section("orders", "bet", ref("AdminWorkbenchOrders")),
  AdminWorkbenchTodayBetsSection: section("today_bets", "report_betting", ref("AdminWorkbenchTodayBets")),
  AdminWorkbenchSettlementSection: section("settlement", "settlement", ref("AdminWorkbenchSettlement")),
  AdminWorkbenchRechargesSection: section("recharges", "recharge", ref("AdminWorkbenchRecharges")),
  AdminWorkbenchLedgerSection: section("ledger", "report_ledger", ref("AdminWorkbenchLedger")),
  AdminWorkbenchBalancesSection: section("balances", "report_ledger", ref("AdminWorkbenchBalances")),
  AdminWorkbenchReconciliationSection: section("reconciliation", "wallet", ref("AdminWorkbenchReconciliation")),
  AdminWorkbenchSourcesSection: section("sources", "draw_source", ref("AdminWorkbenchSources")),
  AdminWorkbenchWithdrawalsSection: section("withdrawals", "withdrawal", ref("AdminWorkbenchWithdrawals")),
  AdminWorkbenchCommissionsSection: unavailable("not_implemented"),
  AdminWorkbenchRewardsSection: section("rewards", "reward", ref("AdminWorkbenchRewards")),
};

const permissions = ["brand.view.brand", "period.view.brand", "bet.view.brand", "report_betting.view.brand", "settlement.view.brand", "recharge.view.brand", "report_ledger.view.brand", "wallet.view.brand", "draw_source.view.brand", "withdrawal.view.brand", "brand.view.platform", "period.view.platform", "bet.view.platform", "report_betting.view.platform", "settlement.view.platform", "recharge.view.platform", "report_ledger.view.platform", "wallet.view.platform", "draw_source.view.platform", "withdrawal.view.platform"];
export const operations = [{
  method: "GET", path: "/api/v1/admin/workbench", operationId: "adminGetWorkbench",
  summary: "Read the selected brand's operations workbench", tag: "workbench", auth: "admin",
  data: ref("AdminWorkbenchSnapshot"), brandHeader: true, permissions: [...permissions, "reward.view.brand", "reward.view.platform"],
  description: "Requires an authenticated administrator and X-Brand-ID. Each section independently requires its corresponding CanView resource grant for the exact selected brand or an explicit platform grant; platform identity or super-admin status alone grants no access. The brand must be valid and is never inferred from administrator identity. All sections are read from one primary-database snapshot; no query parameters or read-routing headers are supported. Withdrawals reports only current reviewing and processing order counts and order points; it does not infer eligibility, actual payment, or profit. Rewards counts all current reward orders in granted, revocation_pending and revoked states; not today's postings, attempt counts, wallet balance or reserved funds. Only commissions remains not_implemented. Fresh authorization is rechecked after audit waits before releasing data. Aggregate counts and amounts are exact decimal strings without an int64 bound; ledger.net_points is a canonical signed decimal string.",
}];
