<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { createReportsApi, reportsPermissions } from "./reports-api";
import { createReportExportApi, reportExportPermissions } from "./report-export-api";
import WithdrawalReport from "./WithdrawalReport.vue";
import CommissionReport from "./CommissionReport.vue";
import CommissionAnalysisReport from "./CommissionAnalysisReport.vue";
import RewardReports from "./RewardReports.vue";
import AttributionReport from "./AttributionReportManagement.vue";
import { useAdminI18n } from "./i18n";

const { t, locale } = useAdminI18n();
const englishUi: Record<string, string> = {
  "只读统计 · 按品牌隔离": "Read-only statistics · Brand scoped",
  "运营报表": "Operational reports",
  "只读刷新报表": "Refresh reports (read only)",
  "报表类型": "Report type",
  "投注报表": "Betting report",
  "账本报表": "Ledger report",
  "报表筛选": "Report filters",
  "时间范围快捷选项": "Quick date ranges",
  "今天（设备时区）": "Today (device timezone)",
  "近 7 天": "Last 7 days",
  "近 30 天": "Last 30 days",
  "报表开始时间": "Report start time",
  "报表结束时间": "Report end time",
  "会员筛选 UUID": "Member UUID filter",
  "彩种筛选 UUID": "Game UUID filter",
  "可留空": "Optional",
  "投注分组": "Betting grouping",
  "账本分组": "Ledger grouping",
  "按天": "By day",
  "按彩种": "By game",
  "按会员": "By member",
  "按账本类型": "By ledger entry type",
  "时间按本机时区输入，并以显式 UTC RFC3339 发送；修改筛选草稿不会改变当前已显示结果，点击查询后才会提交。": "Enter times in the device timezone; requests use explicit UTC RFC3339. Editing draft filters does not change displayed results until you query.",
  "查询报表": "Query reports",
  "超级管理员仍须分别获得对应品牌级或平台级查看与导出授权。": "Super administrators still need separate brand or platform view and export permissions.",
  "报表汇总是当前投影结果，不代表财务关账。用 Excel 查看 CSV 时，请将数值列作为文本导入，以免超过 15 位的整数被舍入。": "Report totals are current projections, not closed financial accounts. Import CSV numeric columns as text in Excel to avoid rounding integers longer than 15 digits.",
  "投注按注单提交时间落入半开区间 [from, to)。": "Bets are selected by submission time in the half-open interval [from, to).",
  "账本按实际创建及入账时间落入半开区间 [from, to)。": "Ledger entries are selected by actual creation and posting time in the half-open interval [from, to).",
  "读取中…": "Loading…",
  "导出中…": "Exporting…",
  "导出全部分组 CSV": "Export all groups as CSV",
  "品牌": "Brand",
  "快照": "Snapshot",
  "时区": "Timezone",
  "区间 [": "Interval [",
  "分组": "Group",
  "会员": "Member",
  "彩种": "Game",
  "全部": "All",
  "标签": "Label",
  "已结算仅计入期间已完成且无待处理更正的最终结算；未结算含已放置、部分派奖及待处理更正，不含异常和取消。当前最终代次奖金仅计入当前最终代次，不是历史累计派奖。历史更正会重算旧注单 cohort。": "Settled amounts include final settlements without pending corrections. Unsettled includes placed bets, partial payouts and pending corrections, excluding abnormal and cancelled bets. Prizes reflect the current final generation, not historical cumulative payouts. Corrections recompute the original bet cohort.",
  "账本统计按实际入账时间，不按注单创建时间。派奖入账、派奖冲正和净变动分开显示；全部来源状态的账本 delta 求和为净变动（统一16分项），冻结与解冻转移的净变动为零。下方余额为快照时品牌/会员当前余额，不受查询时间范围限制；不代表全账本已完成对账。": "Ledger statistics use posting time, not bet creation time. Prize credits, reversals and net changes are separate. Net change sums all source/state deltas (16 current buckets); freeze/unfreeze transfers have zero net change. Balances below are current brand/member balances at the snapshot, independent of the date range, not proof of full reconciliation.",
  "当前余额快照（范围外）": "Current balance snapshot (outside date range)",
  "此页没有分组记录。": "No groups on this page.",
  "第": "Items",
  "项，共": "of",
  "组": "groups",
  "上一页": "Previous",
  "下一页": "Next",
  "设置筛选条件后查询投注报表。": "Set filters and query the betting report.",
  "设置筛选条件后查询账本报表。": "Set filters and query the ledger report.",
  "注单数": "Bet orders",
  "投注积分": "Stake points",
  "已放置": "Placed",
  "已中奖": "Won",
  "已未中奖": "Lost",
  "异常": "Abnormal",
  "已取消": "Cancelled",
  "退款积分": "Refund points",
  "已结算投注": "Settled stakes",
  "未结算投注": "Unsettled stakes",
  "异常投注": "Abnormal stakes",
  "当前最终代次奖金": "Current final-generation prizes",
  "未完成更正数": "Open corrections",
  "账本笔数": "Ledger entries",
  "净变动": "Net change",
  "充值入账": "Recharge credits",
  "派奖入账": "Prize credits",
  "派奖冲正": "Prize reversals",
  "退款": "Refunds",
  "账户数": "Accounts",
  "可用积分": "Available points",
  "冻结积分": "Frozen points",
  "提现中积分": "Withdrawal-pending points",
  "当前积分合计": "Current total points",
  "读取失败，请重试。": "Read failed. Please try again.",
  "请选择有效的报表开始时间和结束时间。": "Choose valid report start and end times.",
  "报表结束时间必须晚于开始时间。": "The report end must be later than the start.",
  "查询时间范围不能超过 93 天。": "The report window cannot exceed 93 days.",
  "会员筛选 UUID 格式无效。": "The member filter UUID is invalid.",
  "彩种筛选 UUID 格式无效。": "The game filter UUID is invalid.",
  "缺少 report_betting.view.brand 或 report_betting.view.platform 查看权限。": "Requires report_betting.view.brand or report_betting.view.platform permission.",
  "缺少 report_ledger.view.brand 或 report_ledger.view.platform 查看权限。": "Requires report_ledger.view.brand or report_ledger.view.platform permission.",
};
function ui(value: string) { return t(value, englishUi[value] ?? value); }

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();

const PAGE_SIZE = 20;
const api = createReportsApi();
const exportApi = createReportExportApi();
const permissions = computed(() => reportsPermissions(props.account, props.brandId));
const exportPermissions = computed(() => reportExportPermissions(props.account, props.brandId));
const activeReport = ref<"betting" | "ledger">("betting");
const bettingGroup = ref<"day" | "game" | "member">("day");
const ledgerGroup = ref<"day" | "entry_type">("day");
const memberDraft = ref("");
const gameDraft = ref("");
const fromDraft = ref("");
const toDraft = ref("");
const bettingResult = ref<Awaited<ReturnType<typeof api.betting>> | null>(null);
const ledgerResult = ref<Awaited<ReturnType<typeof api.ledger>> | null>(null);
const bettingError = ref("");
const ledgerError = ref("");
const bettingBusy = ref(false);
const ledgerBusy = ref(false);
const bettingExportBusy = ref(false);
const ledgerExportBusy = ref(false);
const bettingExportError = ref("");
const ledgerExportError = ref("");
const bettingOffset = ref(0);
const ledgerOffset = ref(0);
let requestTicket = 0;
let exportGeneration = 0;
const exportTickets = { betting: 0, ledger: 0 };
let alive = true;
type ReportFilters = { from: string; to: string; memberId: string | null; gameId: string | null };
type ReportGroups = { betting: "day" | "game" | "member"; ledger: "day" | "entry_type" };
const committedQuery = ref<{ filters: ReportFilters; groups: ReportGroups } | null>(null);

const scopeKey = computed(() => JSON.stringify({
  brand: props.brandId,
  account: props.account.id,
  superAdmin: props.account.super_admin,
  brands: [...(props.account.brand_ids ?? [])].sort(),
  permissions: [...(props.account.permissions ?? [])].sort(),
  platform: [...(props.account.platform_permissions ?? [])].sort(),
  byBrand: props.account.permissions_by_brand?.[props.brandId] ? [...props.account.permissions_by_brand[props.brandId]].sort() : null,
  allowed: permissions.value,
}));
let committedScope = scopeKey.value;

function localDateTime(date: Date): string {
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}
function setPreset(days: 0 | 7 | 30) {
  const end = new Date();
  end.setMilliseconds(0);
  const start = new Date(end);
  if (days === 0) start.setHours(0, 0, 0, 0);
  else start.setDate(start.getDate() - days);
  fromDraft.value = localDateTime(start);
  toDraft.value = localDateTime(end);
}
function toUtc(value: string): string | null {
  if (!value) return null;
  const date = new Date(value);
  return Number.isFinite(date.getTime()) ? date.toISOString() : null;
}
function clearResults() {
  bettingResult.value = null;
  ledgerResult.value = null;
  bettingError.value = "";
  ledgerError.value = "";
  bettingOffset.value = 0;
  ledgerOffset.value = 0;
}
function current(ticket: number, scope: string): boolean {
  return alive && requestTicket === ticket && scopeKey.value === scope && committedScope === scope;
}
function errorText(cause: unknown): string {
  if (cause instanceof AdminApiError) return cause.message;
  return cause instanceof Error ? cause.message : "读取失败，请重试。";
}
function handle401(cause: unknown, ticket: number, scope: string) {
  if (cause instanceof AdminApiError && cause.status === 401 && current(ticket, scope)) {
    requestTicket++;
    clearResults();
    bettingBusy.value = false;
    ledgerBusy.value = false;
    setErrorForPermission();
    emit("session-invalid");
  }
}

function queryValues(from: string, to: string, memberId: string, gameId: string) {
  const start = toUtc(from);
  const end = toUtc(to);
  if (!start || !end) throw new Error("请选择有效的报表开始时间和结束时间。");
  if (Date.parse(end) <= Date.parse(start)) throw new Error("报表结束时间必须晚于开始时间。");
  if (Date.parse(end) - Date.parse(start) > 93 * 24 * 60 * 60 * 1000) throw new Error("查询时间范围不能超过 93 天。");
  const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
  if (memberId && !uuid.test(memberId)) throw new Error("会员筛选 UUID 格式无效。");
  if (gameId && !uuid.test(gameId)) throw new Error("彩种筛选 UUID 格式无效。");
  return { from: start, to: end, memberId: memberId || null, gameId: gameId || null };
}

async function loadReports(offsets: { betting: number; ledger: number }, filters: ReportFilters, groups: ReportGroups) {
  const scope = scopeKey.value;
  const ticket = ++requestTicket;
  committedScope = scope;
  committedQuery.value = { filters: { ...filters }, groups: { ...groups } };
  clearResults();
  setErrorForPermission();
  if (permissions.value.betting) bettingBusy.value = true;
  if (permissions.value.ledger) ledgerBusy.value = true;
  const jobs: Promise<void>[] = [];

  if (permissions.value.betting) {
    const query = { from: filters.from, to: filters.to, group_by: groups.betting, limit: PAGE_SIZE, offset: offsets.betting, ...(filters.gameId ? { game_id: filters.gameId } : {}), ...(filters.memberId ? { member_id: filters.memberId } : {}) };
    jobs.push(api.betting(props.brandId, query).then((result: Awaited<ReturnType<typeof api.betting>>) => {
      if (!current(ticket, scope)) return;
      bettingResult.value = result;
      bettingOffset.value = offsets.betting;
    }).catch((cause: unknown) => {
      if (!current(ticket, scope)) return;
      handle401(cause, ticket, scope);
      if (current(ticket, scope)) bettingError.value = errorText(cause);
    }).finally(() => { if (current(ticket, scope)) bettingBusy.value = false; }));
  }
  if (permissions.value.ledger) {
    const query = { from: filters.from, to: filters.to, group_by: groups.ledger, limit: PAGE_SIZE, offset: offsets.ledger, ...(filters.memberId ? { member_id: filters.memberId } : {}) };
    jobs.push(api.ledger(props.brandId, query).then((result: Awaited<ReturnType<typeof api.ledger>>) => {
      if (!current(ticket, scope)) return;
      ledgerResult.value = result;
      ledgerOffset.value = offsets.ledger;
    }).catch((cause: unknown) => {
      if (!current(ticket, scope)) return;
      handle401(cause, ticket, scope);
      if (current(ticket, scope)) ledgerError.value = errorText(cause);
    }).finally(() => { if (current(ticket, scope)) ledgerBusy.value = false; }));
  }
  await Promise.all(jobs);
}

async function queryReports() {
  try {
    const filters = queryValues(fromDraft.value, toDraft.value, memberDraft.value.trim(), activeReport.value === "betting" ? gameDraft.value.trim() : "");
    const groups = { betting: bettingGroup.value, ledger: ledgerGroup.value };
    await loadReports({ betting: 0, ledger: 0 }, filters, groups);
  } catch (cause) {
    requestTicket++;
    clearResults();
    committedQuery.value = null;
    bettingBusy.value = false;
    ledgerBusy.value = false;
    setErrorForPermission();
    if (permissions.value.betting) bettingError.value = errorText(cause);
    if (permissions.value.ledger) ledgerError.value = errorText(cause);
  }
}

async function refreshReports() {
  if (!committedQuery.value) return queryReports();
  await loadReports({ betting: bettingOffset.value, ledger: ledgerOffset.value }, committedQuery.value.filters, committedQuery.value.groups);
}

async function exportReport(kind: "betting" | "ledger") {
  const committed = committedQuery.value;
  const allowed = kind === "betting" ? exportPermissions.value.betting : exportPermissions.value.ledger;
  if (!allowed || !committed || !alive) return;
  const scope = scopeKey.value;
  const generation = exportGeneration;
  const exportTicket = ++exportTickets[kind];
  const exportKind = activeReport.value;
  const busy = kind === "betting" ? bettingExportBusy : ledgerExportBusy;
  const error = kind === "betting" ? bettingExportError : ledgerExportError;
  busy.value = true;
  error.value = "";
  try {
    const filters = committed.filters;
    const query = {
      from: filters.from, to: filters.to,
      group_by: kind === "betting" ? committed.groups.betting : committed.groups.ledger,
      ...(kind === "betting" && filters.gameId ? { game_id: filters.gameId } : {}),
      ...(filters.memberId ? { member_id: filters.memberId } : {}),
    };
    const result = kind === "betting"
      ? await exportApi.betting(props.brandId, query)
      : await exportApi.ledger(props.brandId, query);
    if (!alive || exportGeneration !== generation || scopeKey.value !== scope || committedQuery.value !== committed || activeReport.value !== exportKind || exportKind !== kind) return;
    const blob = new Blob([result.bytes], { type: "text/csv;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = result.filename;
    anchor.click();
    setTimeout(() => URL.revokeObjectURL(url), 0);
  } catch (cause) {
    if (!alive || exportGeneration !== generation || scopeKey.value !== scope || committedQuery.value !== committed || activeReport.value !== exportKind) return;
    if (cause instanceof AdminApiError && cause.status === 401) {
      requestTicket++;
      clearResults();
      committedQuery.value = null;
      setErrorForPermission();
      emit("session-invalid");
    }
    error.value = errorText(cause);
  } finally {
    if (exportTickets[kind] === exportTicket) busy.value = false;
  }
}

function committedFilters(result: NonNullable<typeof bettingResult.value> | NonNullable<typeof ledgerResult.value>) {
  return { from: result.query.from, to: result.query.to, memberId: result.query.member_id, gameId: result.query.game_id };
}
async function pageReport(kind: "betting" | "ledger", delta: -1 | 1) {
  const result = kind === "betting" ? bettingResult.value : ledgerResult.value;
  if (!result) return;
  const offset = Math.max(0, (kind === "betting" ? bettingOffset.value : ledgerOffset.value) + delta * PAGE_SIZE);
  const filters = committedFilters(result);
  const bettingGroupBy = bettingResult.value?.query.group_by;
  const ledgerGroupBy = ledgerResult.value?.query.group_by;
  const groups: ReportGroups = {
    betting: bettingGroupBy === "game" || bettingGroupBy === "member" ? bettingGroupBy : bettingGroupBy === "day" ? "day" : bettingGroup.value,
    ledger: ledgerGroupBy === "entry_type" ? "entry_type" : ledgerGroupBy === "day" ? "day" : ledgerGroup.value,
  };
  const offsets = { betting: bettingOffset.value, ledger: ledgerOffset.value };
  offsets[kind] = offset;
  await loadReports(offsets, filters, groups);
}

function formatInteger(value: string): string {
  try {
    const parsed = BigInt(value);
    const negative = parsed < 0n;
    const digits = (negative ? -parsed : parsed).toString().replace(/\B(?=(\d{3})+(?!\d))/g, ",");
    return `${negative ? "−" : ""}${digits}`;
  } catch { return value; }
}
function formatDate(value: string): string {
  const date = new Date(value);
  return Number.isFinite(date.getTime()) ? date.toLocaleString(locale.value === "en" ? "en-US" : "zh-CN") : value;
}
function hasNext(total: string, offset: number, count: number): boolean {
  try { return BigInt(offset) + BigInt(count) < BigInt(total); } catch { return false; }
}
function bettingMetrics(totals: NonNullable<typeof bettingResult.value>["summary"]) {
  return [
    ["注单数", totals.order_count], ["投注积分", totals.stake_points], ["已放置", totals.placed_count],
    ["已中奖", totals.won_count], ["已未中奖", totals.lost_count], ["异常", totals.abnormal_count],
    ["已取消", totals.cancelled_count], ["退款积分", totals.refund_points], ["已结算投注", totals.settled_stake_points],
    ["未结算投注", totals.unfinalized_stake_points], ["异常投注", totals.abnormal_stake_points],
    ["当前最终代次奖金", totals.current_prize_points], ["未完成更正数", totals.correction_open_count],
  ] as const;
}
function ledgerMetrics(totals: NonNullable<typeof ledgerResult.value>["summary"]) {
  return [
    ["账本笔数", totals.entry_count], ["净变动", totals.net_points], ["充值入账", totals.recharge_points],
    ["派奖入账", totals.prize_credit_points], ["派奖冲正", totals.prize_reversal_points], ["退款", totals.refund_points],
  ] as const;
}
function balanceMetrics(result: NonNullable<typeof ledgerResult.value>) {
  return [
    ["账户数", result.balances.account_count], ["可用积分", result.balances.available_points],
    ["冻结积分", result.balances.frozen_points], ["提现中积分", result.balances.withdrawal_points],
    ["当前积分合计", result.balances.total_points],
  ] as const;
}
function rowValues(totals: object): [string, string][] {
  return Object.entries(totals) as [string, string][];
}
function labelFor(key: string): string {
  const labels: Record<string, string> = {
    order_count: "注单数", stake_points: "投注积分", placed_count: "已放置", won_count: "已中奖", lost_count: "已未中奖",
    abnormal_count: "异常", cancelled_count: "已取消", refund_points: "退款积分", settled_stake_points: "已结算投注",
    unfinalized_stake_points: "未结算投注", abnormal_stake_points: "异常投注", current_prize_points: "当前最终代次奖金",
    correction_open_count: "未完成更正数", entry_count: "账本笔数", net_points: "净变动", recharge_points: "充值入账",
    prize_credit_points: "派奖入账", prize_reversal_points: "派奖冲正", available_points: "可用积分", frozen_points: "冻结积分",
    withdrawal_points: "提现中积分", total_points: "当前积分合计",
  };
  return ui(labels[key] ?? key);
}
function setErrorForPermission() {
  if (!permissions.value.betting) bettingError.value = "缺少 report_betting.view.brand 或 report_betting.view.platform 查看权限。";
  if (!permissions.value.ledger) ledgerError.value = "缺少 report_ledger.view.brand 或 report_ledger.view.platform 查看权限。";
}

watch(scopeKey, (next) => {
  exportGeneration++;
  exportTickets.betting++;
  exportTickets.ledger++;
  requestTicket++;
  committedScope = next;
  committedQuery.value = null;
  clearResults();
  bettingBusy.value = false;
  ledgerBusy.value = false;
  bettingExportBusy.value = false;
  ledgerExportBusy.value = false;
  bettingExportError.value = "";
  ledgerExportError.value = "";
  setErrorForPermission();
}, { flush: "sync" });
watch(activeReport, () => {
  exportGeneration++;
  exportTickets.betting++;
  exportTickets.ledger++;
  bettingExportBusy.value = false;
  ledgerExportBusy.value = false;
  bettingExportError.value = "";
  ledgerExportError.value = "";
}, { flush: "sync" });
onMounted(() => {
  setPreset(7);
  setErrorForPermission();
});
onBeforeUnmount(() => { alive = false; exportGeneration++; exportTickets.betting++; exportTickets.ledger++; requestTicket++; });
</script>

<template>
  <section class="reports-management" aria-labelledby="reports-title">
    <header class="reports-heading">
      <div><p class="reports-eyebrow">{{ ui("只读统计 · 按品牌隔离") }}</p><h2 id="reports-title">{{ ui("运营报表") }}</h2></div>
      <button class="reports-button secondary" type="button" :disabled="bettingBusy || ledgerBusy" @click="refreshReports">{{ ui("只读刷新报表") }}</button>
    </header>

    <nav class="reports-tabs" :aria-label="ui('报表类型')">
      <button type="button" :aria-pressed="activeReport === 'betting'" @click="activeReport = 'betting'">{{ ui("投注报表") }}</button>
      <button type="button" :aria-pressed="activeReport === 'ledger'" @click="activeReport = 'ledger'">{{ ui("账本报表") }}</button>
    </nav>

    <section class="reports-panel reports-filters" :aria-label="ui('报表筛选')">
      <div class="reports-presets" :aria-label="ui('时间范围快捷选项')">
        <button class="reports-button secondary" type="button" @click="setPreset(0)">{{ ui("今天（设备时区）") }}</button>
        <button class="reports-button secondary" type="button" @click="setPreset(7)">{{ ui("近 7 天") }}</button>
        <button class="reports-button secondary" type="button" @click="setPreset(30)">{{ ui("近 30 天") }}</button>
      </div>
      <div class="reports-filter-grid">
        <label>{{ ui("报表开始时间") }}<input v-model="fromDraft" type="datetime-local" step="1" /></label>
        <label>{{ ui("报表结束时间") }}<input v-model="toDraft" type="datetime-local" step="1" /></label>
        <label>{{ ui("会员筛选 UUID") }}<input v-model.trim="memberDraft" type="text" inputmode="text" autocomplete="off" :placeholder="ui('可留空')" /></label>
        <label v-if="activeReport === 'betting'">{{ ui("彩种筛选 UUID") }}<input v-model.trim="gameDraft" type="text" inputmode="text" autocomplete="off" :placeholder="ui('可留空')" /></label>
        <label v-if="activeReport === 'betting'">{{ ui("投注分组") }}<select v-model="bettingGroup" :aria-label="ui('投注分组')"><option value="day">{{ ui("按天") }}</option><option value="game">{{ ui("按彩种") }}</option><option value="member">{{ ui("按会员") }}</option></select></label>
        <label v-else>{{ ui("账本分组") }}<select v-model="ledgerGroup" :aria-label="ui('账本分组')"><option value="day">{{ ui("按天") }}</option><option value="entry_type">{{ ui("按账本类型") }}</option></select></label>
      </div>
      <p class="reports-note">{{ ui("时间按本机时区输入，并以显式 UTC RFC3339 发送；修改筛选草稿不会改变当前已显示结果，点击查询后才会提交。") }}</p>
      <div class="reports-actions"><button class="reports-button primary" type="button" :disabled="bettingBusy || ledgerBusy" @click="queryReports">{{ ui("查询报表") }}</button></div>
    </section>

    <p v-if="props.account.super_admin" class="reports-note">{{ ui("超级管理员仍须分别获得对应品牌级或平台级查看与导出授权。") }}</p>
    <p class="reports-pending" role="note">{{ ui("报表汇总是当前投影结果，不代表财务关账。用 Excel 查看 CSV 时，请将数值列作为文本导入，以免超过 15 位的整数被舍入。") }}</p>

    <section v-show="activeReport === 'betting'" class="reports-panel" aria-labelledby="betting-title">
      <div class="reports-section-heading"><div><h3 id="betting-title">{{ ui("投注报表") }}</h3><p>{{ ui("投注按注单提交时间落入半开区间 [from, to)。") }}</p></div><div class="reports-export-actions"><span v-if="bettingBusy" class="reports-state">{{ ui("读取中…") }}</span><button class="reports-button secondary" type="button" :disabled="!exportPermissions.betting || !committedQuery || bettingExportBusy" @click="exportReport('betting')">{{ ui(bettingExportBusy ? '导出中…' : '导出全部分组 CSV') }}</button></div></div>
      <p v-if="bettingExportError" class="reports-error" role="alert">{{ ui(bettingExportError) }}</p>
      <p v-if="bettingError" class="reports-error" role="alert">{{ ui(bettingError) }}</p>
      <template v-if="bettingResult">
        <div class="reports-context"><span>{{ ui("品牌") }} {{ bettingResult.brand_id }}</span><span>{{ ui("快照") }} {{ formatDate(bettingResult.snapshot_at) }}</span><span>{{ ui("时区") }} {{ bettingResult.timezone }}</span><span>{{ ui("区间 [") }}{{ formatDate(bettingResult.query.from) }}, {{ formatDate(bettingResult.query.to) }})</span><span>{{ ui("分组") }} {{ bettingResult.query.group_by }}</span><span>{{ ui("会员") }} {{ bettingResult.query.member_id ?? ui('全部') }}</span><span>{{ ui("彩种") }} {{ bettingResult.query.game_id ?? ui('全部') }}</span></div>
        <p class="reports-definition">{{ ui("已结算仅计入期间已完成且无待处理更正的最终结算；未结算含已放置、部分派奖及待处理更正，不含异常和取消。当前最终代次奖金仅计入当前最终代次，不是历史累计派奖。历史更正会重算旧注单 cohort。") }}</p>
        <div class="reports-summary"><div v-for="metric in bettingMetrics(bettingResult.summary)" :key="metric[0]"><span>{{ ui(metric[0]) }}</span><strong>{{ formatInteger(metric[1]) }}</strong></div></div>
        <div v-if="bettingResult.items.length" class="reports-table-wrap"><table class="reports-table"><thead><tr><th>{{ ui("分组") }}</th><th>{{ ui("标签") }}</th><th v-for="metric in bettingMetrics(bettingResult.items[0].totals)" :key="metric[0]">{{ ui(metric[0]) }}</th></tr></thead><tbody><tr v-for="item in bettingResult.items" :key="item.key"><td>{{ item.key }}</td><td>{{ item.label }}</td><td v-for="metric in bettingMetrics(item.totals)" :key="metric[0]">{{ formatInteger(metric[1]) }}</td></tr></tbody></table></div>
        <div v-if="bettingResult.items.length" class="reports-cards"><article v-for="item in bettingResult.items" :key="item.key"><h4>{{ item.label }}</h4><p class="reports-key">{{ item.key }}</p><dl><div v-for="[key, value] in rowValues(item.totals)" :key="key"><dt>{{ labelFor(key) }}</dt><dd>{{ formatInteger(value) }}</dd></div></dl></article></div>
        <p v-if="!bettingResult.items.length" class="reports-note">{{ ui("此页没有分组记录。") }}</p>
        <div class="reports-pagination"><span>{{ ui("第") }} {{ bettingOffset + 1 }}–{{ bettingOffset + bettingResult.items.length }} {{ ui("项，共") }} {{ formatInteger(bettingResult.total_groups) }} {{ ui("组") }}</span><button class="reports-button secondary" type="button" :disabled="bettingOffset === 0 || bettingBusy || ledgerBusy" @click="pageReport('betting', -1)">{{ ui("上一页") }}</button><button class="reports-button secondary" type="button" :disabled="!hasNext(bettingResult.total_groups, bettingOffset, bettingResult.items.length) || bettingBusy || ledgerBusy" @click="pageReport('betting', 1)">{{ ui("下一页") }}</button></div>
      </template>
      <p v-else-if="!bettingBusy && !bettingError && permissions.betting" class="reports-note">{{ ui("设置筛选条件后查询投注报表。") }}</p>
    </section>

    <section v-show="activeReport === 'ledger'" class="reports-panel" aria-labelledby="ledger-title">
      <div class="reports-section-heading"><div><h3 id="ledger-title">{{ ui("账本报表") }}</h3><p>{{ ui("账本按实际创建及入账时间落入半开区间 [from, to)。") }}</p></div><div class="reports-export-actions"><span v-if="ledgerBusy" class="reports-state">{{ ui("读取中…") }}</span><button class="reports-button secondary" type="button" :disabled="!exportPermissions.ledger || !committedQuery || ledgerExportBusy" @click="exportReport('ledger')">{{ ui(ledgerExportBusy ? '导出中…' : '导出全部分组 CSV') }}</button></div></div>
      <p v-if="ledgerExportError" class="reports-error" role="alert">{{ ui(ledgerExportError) }}</p>
      <p v-if="ledgerError" class="reports-error" role="alert">{{ ui(ledgerError) }}</p>
      <template v-if="ledgerResult">
        <div class="reports-context"><span>{{ ui("品牌") }} {{ ledgerResult.brand_id }}</span><span>{{ ui("快照") }} {{ formatDate(ledgerResult.snapshot_at) }}</span><span>{{ ui("时区") }} {{ ledgerResult.timezone }}</span><span>{{ ui("区间 [") }}{{ formatDate(ledgerResult.query.from) }}, {{ formatDate(ledgerResult.query.to) }})</span><span>{{ ui("分组") }} {{ ledgerResult.query.group_by }}</span><span>{{ ui("会员") }} {{ ledgerResult.query.member_id ?? ui('全部') }}</span></div>
        <p class="reports-definition">{{ ui("账本统计按实际入账时间，不按注单创建时间。派奖入账、派奖冲正和净变动分开显示；全部来源状态的账本 delta 求和为净变动（统一16分项），冻结与解冻转移的净变动为零。下方余额为快照时品牌/会员当前余额，不受查询时间范围限制；不代表全账本已完成对账。") }}</p>
        <div class="reports-summary"><div v-for="metric in ledgerMetrics(ledgerResult.summary)" :key="metric[0]"><span>{{ ui(metric[0]) }}</span><strong>{{ formatInteger(metric[1]) }}</strong></div></div>
        <div class="reports-balance"><h4>{{ ui("当前余额快照（范围外）") }}</h4><div class="reports-summary"> <div v-for="metric in balanceMetrics(ledgerResult)" :key="metric[0]"><span>{{ ui(metric[0]) }}</span><strong>{{ formatInteger(metric[1]) }}</strong></div></div></div>
        <div v-if="ledgerResult.items.length" class="reports-table-wrap"><table class="reports-table"><thead><tr><th>{{ ui("分组") }}</th><th>{{ ui("标签") }}</th><th v-for="metric in ledgerMetrics(ledgerResult.items[0].totals)" :key="metric[0]">{{ ui(metric[0]) }}</th></tr></thead><tbody><tr v-for="item in ledgerResult.items" :key="item.key"><td>{{ item.key }}</td><td>{{ item.label }}</td><td v-for="metric in ledgerMetrics(item.totals)" :key="metric[0]">{{ formatInteger(metric[1]) }}</td></tr></tbody></table></div>
        <div v-if="ledgerResult.items.length" class="reports-cards"><article v-for="item in ledgerResult.items" :key="item.key"><h4>{{ item.label }}</h4><p class="reports-key">{{ item.key }}</p><dl><div v-for="[key, value] in rowValues(item.totals)" :key="key"><dt>{{ labelFor(key) }}</dt><dd>{{ formatInteger(value) }}</dd></div></dl></article></div>
        <p v-if="!ledgerResult.items.length" class="reports-note">{{ ui("此页没有分组记录。") }}</p>
        <div class="reports-pagination"><span>{{ ui("第") }} {{ ledgerOffset + 1 }}–{{ ledgerOffset + ledgerResult.items.length }} {{ ui("项，共") }} {{ formatInteger(ledgerResult.total_groups) }} {{ ui("组") }}</span><button class="reports-button secondary" type="button" :disabled="ledgerOffset === 0 || bettingBusy || ledgerBusy" @click="pageReport('ledger', -1)">{{ ui("上一页") }}</button><button class="reports-button secondary" type="button" :disabled="!hasNext(ledgerResult.total_groups, ledgerOffset, ledgerResult.items.length) || bettingBusy || ledgerBusy" @click="pageReport('ledger', 1)">{{ ui("下一页") }}</button></div>
      </template>
      <p v-else-if="!ledgerBusy && !ledgerError && permissions.ledger" class="reports-note">{{ ui("设置筛选条件后查询账本报表。") }}</p>
    </section>
    <WithdrawalReport :account="props.account" :brand-id="props.brandId" @session-invalid="emit('session-invalid')" />
    <CommissionReport :key="`commission-report:${props.account.id}:${props.brandId}`" :account="props.account" :brand-id="props.brandId" @session-invalid="emit('session-invalid')" />
    <CommissionAnalysisReport :key="`commission-analysis:${props.account.id}:${props.brandId}`" :account="props.account" :brand-id="props.brandId" @session-invalid="emit('session-invalid')" />
    <RewardReports :key="`reward-report:${props.account.id}:${props.brandId}`" :account="props.account" :brand-id="props.brandId" @session-invalid="emit('session-invalid')" />
    <AttributionReport :key="`attribution-report:${props.account.id}:${props.brandId}`" :account="props.account" :brand-id="props.brandId" @session-invalid="emit('session-invalid')" />
  </section>
</template>

<style scoped>
.reports-management{display:grid;gap:16px;color:#172033;min-width:0}.reports-heading,.reports-section-heading,.reports-actions,.reports-pagination{display:flex;align-items:center;justify-content:space-between;gap:12px}.reports-heading h2{margin:2px 0 0;font-size:1.25rem}.reports-eyebrow{margin:0;color:#68758a;font-size:.78rem}.reports-tabs,.reports-presets,.reports-actions{display:flex;gap:8px;flex-wrap:wrap}.reports-tabs button{border:1px solid #cbd3df;border-radius:8px;background:#fff;padding:9px 14px;color:inherit;font:inherit;cursor:pointer}.reports-tabs button[aria-pressed=true]{background:#254a83;border-color:#254a83;color:#fff}.reports-panel{display:grid;gap:14px;min-width:0;border:1px solid #d9e0ea;border-radius:12px;padding:16px;background:#fff}.reports-filters{background:#f8fafc}.reports-filter-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px}.reports-filter-grid label{display:grid;gap:6px;min-width:0;font-size:.88rem}.reports-filter-grid input,.reports-filter-grid select{box-sizing:border-box;width:100%;min-width:0;border:1px solid #cbd3df;border-radius:8px;padding:9px 10px;background:#fff;font:inherit}.reports-button{min-height:40px;border:1px solid #c5ceda;border-radius:8px;padding:8px 12px;font:inherit;cursor:pointer;background:#fff;color:inherit}.reports-button.primary{background:#254a83;border-color:#254a83;color:#fff}.reports-button:disabled{opacity:.5;cursor:not-allowed}.reports-section-heading h3{margin:0}.reports-section-heading p,.reports-definition,.reports-note,.reports-pending,.reports-state{margin:4px 0 0;color:#667085;font-size:.88rem}.reports-definition{line-height:1.55}.reports-context{display:flex;flex-wrap:wrap;gap:6px 14px;color:#475467;font-size:.82rem;overflow-wrap:anywhere}.reports-summary{display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:8px}.reports-summary>div{display:grid;gap:5px;min-width:0;border:1px solid #e5eaf0;border-radius:9px;padding:10px}.reports-summary span{color:#667085;font-size:.8rem}.reports-summary strong{overflow-wrap:anywhere;font-variant-numeric:tabular-nums}.reports-balance{display:grid;gap:8px;border-top:1px solid #e8ecf2;padding-top:12px}.reports-balance h4{margin:0}.reports-table-wrap{width:100%;overflow-x:auto;border:1px solid #e2e7ef;border-radius:9px}.reports-table{width:100%;min-width:730px;border-collapse:collapse;font-variant-numeric:tabular-nums}.reports-table th,.reports-table td{height:46px;box-sizing:border-box;padding:8px 10px;border-bottom:1px solid #edf0f4;text-align:left;white-space:nowrap}.reports-table th{position:sticky;top:0;background:#f7f9fc;color:#475467;font-size:.82rem}.reports-table tr:last-child td{border-bottom:0}.reports-pagination{justify-content:flex-end;flex-wrap:wrap}.reports-pagination span{margin-right:auto;color:#667085;font-size:.86rem}.reports-error{margin:0;border-radius:8px;padding:10px 12px;background:#fff0ee;color:#a32e1a}.reports-note,.reports-pending{margin:0}.reports-pending{border-left:3px solid #d8a62c;padding:8px 10px;background:#fff9eb}.reports-cards{display:none}.reports-key{overflow-wrap:anywhere;color:#667085;font-size:.8rem}.reports-cards article{border:1px solid #e2e7ef;border-radius:9px;padding:12px}.reports-cards h4{margin:0}.reports-cards dl{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px;margin:10px 0 0}.reports-cards dl div{min-width:0}.reports-cards dt{color:#667085;font-size:.78rem}.reports-cards dd{margin:3px 0 0;font-weight:550;overflow-wrap:anywhere;font-variant-numeric:tabular-nums}
.reports-export-actions{display:flex;align-items:center;gap:10px;flex-wrap:wrap}
@media(max-width:700px){.reports-management{gap:12px}.reports-heading{align-items:flex-start}.reports-panel{padding:12px}.reports-filter-grid{grid-template-columns:minmax(0,1fr)}.reports-actions .reports-button{width:100%}.reports-table-wrap{display:none}.reports-cards{display:grid;gap:9px}.reports-pagination{justify-content:center}.reports-pagination span{flex-basis:100%;text-align:center}.reports-pagination .reports-button{flex:1;min-width:0}.reports-summary{grid-template-columns:repeat(2,minmax(0,1fr))}.reports-section-heading{align-items:flex-start}.reports-context{display:grid;gap:4px}}
</style>
