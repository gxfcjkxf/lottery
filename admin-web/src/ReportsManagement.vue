<script setup lang="ts">
const showSummary = ref(true), showDetails = ref(true)
import {vInlineValidation} from '@lottery/shared'
const filterReset=ref(0)
import ManagementList from "@lottery/shared/ManagementList.vue"
import TablePagination from "@lottery/shared/TablePagination.vue"
import FilterAssociationSearch from "@lottery/shared/FilterAssociationSearch.vue";
const memberSearchPending = ref(false);
import SelectControl from '@lottery/shared/SelectControl.vue'
import { useOperationToast } from "@lottery/shared";
import FilterAssociationSelect from "@lottery/shared/FilterAssociationSelect.vue";
import { platformDateBoundary, platformDisplaySettings } from "@lottery/shared";
import { associationGroupLabel } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
import { withGlobalLoading } from '@lottery/shared';
import PlatformTemporalInput from "@lottery/shared/PlatformTemporalInput.vue";
const displayLocale = useAdminI18n().locale;
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { createReportsApi, reportsPermissions } from "./reports-api";
import { createAttributionReportApi, attributionReportPermissions, type AttributionReport, type AttributionGroupBy, type AttributionJoinMethod } from "./attribution-report-api";
import "./report-workspace.css";
import WithdrawalReport from "./WithdrawalReport.vue";
import CommissionReport from "./CommissionReport.vue";
import RewardReports from "./RewardReports.vue";
import { adminKey, useAdminI18n } from "./i18n";
import { brandPermissionSet } from "./brand-permissions";
import { dateRangePreset, type DateRangePreset } from "@lottery/shared";

const { t, locale, display } = useAdminI18n();
const ui = (key: string) => t(adminKey(key));

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();

const pageSizes = ref({ betting: 20, ledger: 20 });
const api = createReportsApi();
const attributionApi = createAttributionReportApi();
const attributionRights = computed(() => attributionReportPermissions(props.account, props.brandId));
const canViewBetting = computed(() => permissions.value.betting || attributionRights.value.view);
const agentSearchPending = ref(false);
const agentDraft = ref("");
const agentScope = ref<"direct" | "downline">("direct");
const joinMethod = ref<AttributionJoinMethod | "">("");
const joinMethodLabels = { domain: 'admin.attribution.join.domain', operator: 'admin.attribution.join.operator', agent_code: 'admin.attribution.join.agentCode', referral_code: 'admin.attribution.join.referralCode' } as const;
const permissions = computed(() => reportsPermissions(props.account, props.brandId));
const reportTabs = [
  { id: 'betting', permissions: ['report_betting.view.brand'], label: 'admin.copy.b30b1af4' },
  { id: 'ledger', permissions: ['report_ledger.view.brand'], label: 'admin.copy.5a74ef63' },
  { id: 'withdrawal', permissions: ['report_withdrawal.view.brand'], label: 'admin.copy.90a6f04b' },
  { id: 'reward', permissions: ['report_reward.view.brand'], label: 'admin.copy.1d928afb' },
  { id: 'posting', permissions: ['report_commission.view.brand'], label: 'admin.commission.ui.postingReport' },
] as const;
const reportGrants = brandPermissionSet(props.account, props.brandId.toLowerCase());
const initialReport = reportTabs.find(tab => tab.id === 'betting' ? canViewBetting.value : tab.permissions.every(permission => reportGrants.has(permission)));
const activeReport = ref<(typeof reportTabs)[number]['id']>(initialReport?.id ?? 'betting');
function focusReportTab(event: KeyboardEvent, index: number) {
  let target: number;
  if (event.key === 'ArrowRight') target = (index + 1) % reportTabs.length;
  else if (event.key === 'ArrowLeft') target = (index + reportTabs.length - 1) % reportTabs.length;
  else if (event.key === 'Home') target = 0;
  else if (event.key === 'End') target = reportTabs.length - 1;
  else return;
  event.preventDefault();
  const nav = (event.currentTarget as HTMLElement).parentElement!;
  nav.querySelectorAll<HTMLButtonElement>('[role="tab"]')[target]!.focus();
}
const bettingGroup = ref<AttributionGroupBy>("day");
const ledgerGroup = ref<"day" | "entry_type">("day");
const memberDraft = ref("");
const gameDraft = ref("");
const fromDraft = ref("");
const toDraft = ref("");
const bettingResult = ref<Awaited<ReturnType<typeof api.betting>> | AttributionReport | null>(null);
const ledgerResult = ref<Awaited<ReturnType<typeof api.ledger>> | null>(null);
const bettingError = ref<string | import("@lottery/shared").LocalizedMessage>("");
const ledgerError = ref<string | import("@lottery/shared").LocalizedMessage>("");
const bettingBusy = ref(false);
const ledgerBusy = ref(false);
const bettingOffset = ref(0);
const ledgerOffset = ref(0);
let requestTicket = 0;
let alive = true;
type ReportFilters = { from: string; to: string; memberId: string | null; gameId: string | null; agentId: string | null; agentScope: "direct" | "downline"; joinMethod: AttributionJoinMethod | null };
type ReportGroups = { betting: AttributionGroupBy; ledger: "day" | "entry_type" };
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
  attribution: attributionRights.value,
}));
let committedScope = scopeKey.value;

function localDateTime(date: Date): string {
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}
function setPreset(preset: DateRangePreset) {
  const range = dateRangePreset(preset);
  fromDraft.value = localDateTime(range.from).slice(0,10);
  const inclusiveEnd = new Date(range.to); if (['yesterday', 'last-week', 'last-month'].includes(preset)) inclusiveEnd.setDate(inclusiveEnd.getDate()-1);
  toDraft.value = localDateTime(inclusiveEnd).slice(0,10);
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
function errorText(cause: unknown): string | import("@lottery/shared").LocalizedMessage {
  if (cause instanceof AdminApiError) return publicErrorMessage(cause, 'read');
  return cause instanceof Error ? publicErrorMessage(cause) : "读取失败，请重试。";
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

async function queryValues(from: string, to: string, memberUsername: string, gameId: string) {
  const start = platformDateBoundary(from,false,platformDisplaySettings.value!.timezone).toISOString();
  const end = platformDateBoundary(to,true,platformDisplaySettings.value!.timezone).toISOString();
  if (!start || !end) throw new Error("请选择有效的报表开始时间和结束时间。");
  if (Date.parse(end) <= Date.parse(start)) throw new Error("报表结束时间必须晚于开始时间。");
  if (Date.parse(end) - Date.parse(start) > 93 * 24 * 60 * 60 * 1000) throw new Error("查询时间范围不能超过 93 天。");
  const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
  if (gameId && !uuid.test(gameId)) throw new Error("彩种筛选 UUID 格式无效。");
  const member = memberUsername ? { id: memberUsername } : null;
  return { from: start, to: end, memberId: member?.id ?? null, gameId: gameId || null,
    agentId: activeReport.value === 'betting' ? agentDraft.value.trim() || null : null,
    agentScope: activeReport.value === 'betting' ? agentScope.value : 'direct' as const,
    joinMethod: activeReport.value === 'betting' ? joinMethod.value || null : null,
  };
}

function usesAttribution(filters: ReportFilters, group: AttributionGroupBy) {
  return !permissions.value.betting || !!filters.agentId || filters.agentScope === 'downline' || !!filters.joinMethod || group === 'agent' || group === 'join_method';
}
function bettingQuery(filters: ReportFilters, group: AttributionGroupBy) {
  if (group === 'agent' || group === 'join_method') throw new AdminApiError('Unsupported betting grouping', 400, 'INVALID_INPUT');
  return { from: filters.from, to: filters.to, group_by: group, ...(filters.gameId ? { game_id: filters.gameId } : {}), ...(filters.memberId ? { member_id: filters.memberId } : {}) };
}
function attributionQuery(filters: ReportFilters, group: AttributionGroupBy) {
  if (!attributionRights.value.view) throw new AdminApiError('Forbidden', 403, 'FORBIDDEN');
  return { from: filters.from, to: filters.to, group_by: group, game_id: filters.gameId, member_id: filters.memberId, agent_id: filters.agentId, agent_scope: filters.agentScope, join_method: filters.joinMethod };
}
async function readBetting(filters: ReportFilters, group: AttributionGroupBy, offset: number) {
  return usesAttribution(filters, group)
    ? attributionApi.report(props.brandId, { ...attributionQuery(filters, group), limit: pageSizes.value.betting, offset })
    : api.betting(props.brandId, { ...bettingQuery(filters, group), limit: pageSizes.value.betting, offset });
}

async function loadReports(offsets: { betting: number; ledger: number }, filters: ReportFilters, groups: ReportGroups) {
  const scope = scopeKey.value;
  const ticket = ++requestTicket;
  committedScope = scope;
  bettingResult.value = null;
  ledgerResult.value = null;
  committedQuery.value = { filters: { ...filters }, groups: { ...groups } };
  bettingError.value = ""; ledgerError.value = "";
  setErrorForPermission();
  if (canViewBetting.value) bettingBusy.value = true;
  if (permissions.value.ledger) ledgerBusy.value = true;
  const jobs: Promise<void>[] = [];

  if (canViewBetting.value) {
    jobs.push(readBetting(filters, groups.betting, offsets.betting).then((result) => {
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
    const query = { from: filters.from, to: filters.to, group_by: groups.ledger, limit: pageSizes.value.ledger, offset: offsets.ledger, ...(filters.memberId ? { member_id: filters.memberId } : {}) };
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
  if (memberSearchPending.value || activeReport.value === 'betting' && agentSearchPending.value) return
  try {
    const filters = await queryValues(fromDraft.value, toDraft.value, memberDraft.value.trim(), activeReport.value === "betting" ? gameDraft.value.trim() : "");
    const groups = { betting: bettingGroup.value, ledger: ledgerGroup.value };
    await loadReports({ betting: 0, ledger: 0 }, filters, groups);
  } catch (cause) {
    requestTicket++;
    clearResults();
    committedQuery.value = null;
    bettingBusy.value = false;
    ledgerBusy.value = false;
    setErrorForPermission();
    if (canViewBetting.value) bettingError.value = errorText(cause);
    if (permissions.value.ledger) ledgerError.value = errorText(cause);
  }
}

async function pageReport(kind: "betting" | "ledger", delta: number) {
  const committed = committedQuery.value;
  if (!committed) return;
  const offsets = { betting: bettingOffset.value, ledger: ledgerOffset.value };
  offsets[kind] = Math.max(0, offsets[kind] + delta * pageSizes.value[kind]);
  await loadReports(offsets, committed.filters, committed.groups);
}

function formatInteger(value: string): string {
  try {
    const parsed = BigInt(value);
    const negative = parsed < 0n;
    const digits = (negative ? -parsed : parsed).toString().replace(/\B(?=(\d{3})+(?!\d))/g, ",");
    return `${negative ? "−" : ""}${digits}`;
  } catch { return value; }
}
function hasNext(total: string, offset: number, count: number): boolean {
  try { return BigInt(offset) + BigInt(count) < BigInt(total); } catch { return false; }
}
function bettingMetrics(totals: NonNullable<typeof bettingResult.value>["summary"]) {
  return [
    ["order_count", totals.order_count], ["stake_points", totals.stake_points], ["placed_count", totals.placed_count],
    ["won_count", totals.won_count], ["lost_count", totals.lost_count], ["abnormal_count", totals.abnormal_count],
    ["cancelled_count", totals.cancelled_count], ["refund_points", totals.refund_points], ["settled_stake_points", totals.settled_stake_points],
    ["unfinalized_stake_points", totals.unfinalized_stake_points], ["abnormal_stake_points", totals.abnormal_stake_points],
    ["current_prize_points", totals.current_prize_points], ["correction_open_count", totals.correction_open_count],
    ...("final_lost_stake_points" in totals ? [["final_lost_stake_points", totals.final_lost_stake_points] as const] : []),
  ] as const;
}
function ledgerMetrics(totals: NonNullable<typeof ledgerResult.value>["summary"]) {
  return [
    ["entry_count", totals.entry_count], ["net_points", totals.net_points], ["recharge_points", totals.recharge_points],
    ["prize_credit_points", totals.prize_credit_points], ["prize_reversal_points", totals.prize_reversal_points], ["refund_points", totals.refund_points],
  ] as const;
}
function balanceMetrics(result: NonNullable<typeof ledgerResult.value>) {
  return [
    ["account_count", result.balances.account_count], ["available_points", result.balances.available_points],
    ["frozen_points", result.balances.frozen_points], ["withdrawal_points", result.balances.withdrawal_points],
    ["total_points", result.balances.total_points],
  ] as const;
}
function rowValues(totals: object): [string, string][] {
  return Object.entries(totals) as [string, string][];
}
function labelFor(key: string): string {
  const labels: Record<string, string> = {
    order_count: "admin.copy.57a81d6e", stake_points: "admin.copy.33ae8fea", placed_count: "admin.copy.96f50da6", won_count: "admin.copy.6de62356", lost_count: "admin.copy.1a44472c",
    abnormal_count: "admin.copy.242b6072", cancelled_count: "admin.copy.c320e9ec", refund_points: "admin.copy.32dff627", settled_stake_points: "admin.copy.fa05625e",
    unfinalized_stake_points: "admin.copy.4a69124d", abnormal_stake_points: "admin.copy.9c70a235", current_prize_points: "admin.copy.006c120b",
    final_lost_stake_points: "admin.copy.9129589a", correction_open_count: "admin.copy.c4cdafbd", entry_count: "admin.copy.e0c59808", net_points: "admin.copy.3195be70", recharge_points: "admin.copy.cbf177ce",
    prize_credit_points: "admin.copy.2b9638d8", prize_reversal_points: "admin.copy.4b205d41", available_points: "admin.copy.5b7ceca3", frozen_points: "admin.copy.7c11c5d7",
    account_count: "admin.copy.64df0e2b", withdrawal_points: "admin.copy.4a6bf99f", total_points: "admin.copy.cb6f8f43",
  };
  const label = labels[key];
  if (!label) throw new Error(`Unsupported report metric: ${key}`);
  return ui(label);
}
function setErrorForPermission() {
  if (activeReport.value === "betting" && !canViewBetting.value) bettingError.value = "缺少 report_betting.view.brand 或 report_betting.view.platform 查看权限。";
  if (activeReport.value === "ledger" && !permissions.value.ledger) ledgerError.value = "缺少 report_ledger.view.brand 或 report_ledger.view.platform 查看权限。";
}

watch(scopeKey, (next) => {
  requestTicket++;
  committedScope = next;
  committedQuery.value = null;
  clearResults();
  bettingBusy.value = false;
  ledgerBusy.value = false;
  setErrorForPermission();
  if (canViewBetting.value || permissions.value.ledger) void withGlobalLoading(queryReports);
}, { flush: "sync" });
watch(activeReport, () => {
  }, { flush: "sync" });
onMounted(() => {
  setPreset("today");
  setErrorForPermission();
  if (canViewBetting.value || permissions.value.ledger) void withGlobalLoading(queryReports);
});
onBeforeUnmount(() => { alive = false; requestTicket++; });

useOperationToast(() => bettingError.value, value => display(value), 'error');
useOperationToast(() => ledgerError.value, value => display(value), 'error');

</script>

<template>
  <section class="reports-management" aria-labelledby="reports-title">
    <header class="console-page-heading reports-heading">
      <div><h2 id="reports-title" class="console-page-title">{{ t('admin.nav.reports') }}</h2></div>

    </header>

    <nav class="reports-tabs console-tabs" role="tablist" :aria-label="t('admin.copy.9dd4dae0')">
      <button v-for="(tab, index) in reportTabs" :id="`report-tab-${tab.id}`" :key="tab.id" type="button" role="tab" :aria-selected="activeReport === tab.id" :aria-controls="`report-panel-${tab.id}`" :tabindex="activeReport === tab.id ? 0 : -1" @keydown="focusReportTab($event, index)" @click="activeReport = tab.id">{{ t(tab.label) }}</button>
    </nav>

    <div :id="`report-panel-${activeReport}`" role="tabpanel" :aria-labelledby="`report-tab-${activeReport}`" :tabindex="0" class="report-panel">
    <ManagementList v-show="activeReport === 'betting' || activeReport === 'ledger'" :busy="bettingBusy || ledgerBusy" class="report-panel">
<template #filters>
      <div class="reports-presets" :aria-label="t('admin.copy.56549499')">
        <button class="reports-button button button-secondary" type="button" @click="setPreset('today')">{{ t('admin.report.today') }}</button>
        <button class="reports-button button button-secondary" type="button" @click="setPreset('yesterday')">{{ t('admin.report.yesterday') }}</button>
        <button class="reports-button button button-secondary" type="button" @click="setPreset('last-week')">{{ t('admin.report.lastWeek') }}</button>
        <button class="reports-button button button-secondary" type="button" @click="setPreset('this-month')">{{ t('admin.report.thisMonth') }}</button>
        <button class="reports-button button button-secondary" type="button" @click="setPreset('last-month')">{{ t('admin.report.lastMonth') }}</button>
      </div>
      <form v-inline-validation novalidate class="reports-filter-grid ui-filter-bar" @submit.prevent="withGlobalLoading(queryReports)">
        <label><PlatformTemporalInput :label="t('filter.fromDate')" :placeholder="t('filter.fromDate')" v-model="fromDraft" type="date" :locale="displayLocale" /></label>
        <label><PlatformTemporalInput :label="t('filter.toDate')" :placeholder="t('filter.toDate')" v-model="toDraft" type="date" :locale="displayLocale" /></label>
        <label><FilterAssociationSearch :reset-key="filterReset" v-model="memberDraft" v-model:pending="memberSearchPending" :brand-id="brandId" surface="admin" :enabled="activeReport === 'betting' ? canViewBetting : permissions.ledger" :resource="activeReport === 'betting' && !permissions.betting ? 'report_attribution' : `report_${activeReport}`" :label="t('filter.member')" :locale="displayLocale" :scope-key="JSON.stringify(account)" /></label>
        <label v-if="activeReport === 'betting'"><FilterAssociationSelect v-model="gameDraft" :brand-id="brandId" surface="admin" :enabled="canViewBetting" :resource="permissions.betting ? 'report_betting' : 'report_attribution'" kind="game" :label="t('filter.game')" :locale="displayLocale" :scope-key="JSON.stringify(account)" /></label>
        <template v-if="attributionRights.view">
          <label v-show="activeReport === 'betting'"><FilterAssociationSearch :reset-key="filterReset" v-model="agentDraft" v-model:pending="agentSearchPending" :brand-id="brandId" surface="admin" :enabled="attributionRights.view" resource="report_attribution" kind="agent" :label="t('filter.agent')" :locale="displayLocale" :scope-key="JSON.stringify(account)" /></label>
          <label v-show="activeReport === 'betting'"><SelectControl :placeholder="t('admin.copy.bc34386a')" data-testid="attribution-agent-scope" v-model="agentScope" :label="t('admin.copy.bc34386a')" :options="[{ value: 'direct', label: t('admin.copy.36d246ea') }, { value: 'downline', label: t('admin.copy.0a3260d0') }]" /></label>
          <label v-show="activeReport === 'betting'"><SelectControl :placeholder="t('admin.copy.93fbd29f')" data-testid="attribution-join-method" v-model="joinMethod" :label="t('admin.copy.93fbd29f')" :options="[{ value: '', label: t('admin.copy.1155b6f5') }, ...Object.entries(joinMethodLabels).map(([value, key]) => ({ value, label: t(key) }))]" /></label>
        </template>
        <label v-if="activeReport === 'betting'"><SelectControl :placeholder="t('admin.copy.6cc02c91')" data-testid="attribution-group" v-model="bettingGroup" :label="t('admin.copy.6cc02c91')" :options="[{ value: 'day', label: (String(t('admin.report.byDate'))).trim() }, { value: 'game', label: (String(t('admin.copy.ef04a723'))).trim() }, { value: 'member', label: (String(t('admin.copy.93a783ef'))).trim() }, ...(attributionRights.view ? [{ value: 'agent', label: t('filter.agent') }, { value: 'join_method', label: t('admin.copy.9a96afd1') }] : [])]" /></label>
        <label v-else><SelectControl :placeholder="t('admin.copy.6cc02c91')" v-model="ledgerGroup" :label="t('admin.copy.6cc02c91')" :options="[{ value: 'day', label: (String(t('admin.copy.f91541ed'))).trim() }, { value: 'entry_type', label: (String(t('admin.copy.5ea104fa'))).trim() }]" /></label>
        <label class="report-display-option"><input v-model="showSummary" type="checkbox" />{{ t('admin.reports.showSummary') }}</label><label class="report-display-option"><input v-model="showDetails" type="checkbox" />{{ t('admin.reports.showDetails') }}</label>
        <button data-testid="attribution-query" class="reports-button button button-primary" type="submit" :disabled="bettingBusy || ledgerBusy || memberSearchPending || activeReport === 'betting' && agentSearchPending">{{ t('admin.copy.2bd50b3e') }}</button>
      </form>
    </template>


    <div v-show="activeReport === 'betting' && (showSummary || showDetails)" id="betting-results" data-testid="attribution-result" class="reports-panel report-results" :aria-label="t('admin.copy.b30b1af4')">


      <template v-if="bettingResult">
        <dl data-testid="attribution-summary" v-if="showSummary" class="reports-summary report-metrics report-summary-tags"><div v-for="metric in bettingMetrics(bettingResult.summary)" :key="metric[0]"><dt><span>{{ labelFor(metric[0]) }}</span></dt><dd><strong>{{ formatInteger(metric[1]) }}</strong></dd></div></dl>
        <div v-if="showDetails" class="table-wrap reports-table-wrap"><table class="console-table reports-table" :class="{'console-responsive-table':!bettingResult.items.length}"><thead><tr><th>{{ t('admin.copy.b96f43c6') }}</th><th v-for="metric in bettingMetrics(bettingResult.summary)" :key="metric[0]">{{ labelFor(metric[0]) }}</th></tr></thead><tbody><tr v-for="item in bettingResult.items" :key="item.key"><td :data-label="String(t('admin.copy.b96f43c6'))">{{ bettingResult.query.group_by === 'agent' && item.key === 'none' ? t('admin.copy.a4faa770') : associationGroupLabel(item, bettingResult.query.group_by, item.key, item.label, associationLocale) }}</td><td :data-label="String(labelFor(metric[0]))" v-for="metric in bettingMetrics(item.totals)" :key="metric[0]">{{ formatInteger(metric[1]) }}</td></tr><tr v-if="!bettingResult.items.length"><td :colspan="bettingMetrics(bettingResult.summary).length + 1" class="empty-state">{{ t('admin.copy.24e33023') }}</td></tr></tbody></table></div>
        <div v-if="showDetails && bettingResult.items.length" class="reports-cards"><article v-for="item in bettingResult.items" :key="item.key"><h4>{{ bettingResult.query.group_by === 'agent' && item.key === 'none' ? t('admin.copy.a4faa770') : associationGroupLabel(item, bettingResult.query.group_by, item.key, item.label, associationLocale) }}</h4><dl><div v-for="[key, value] in rowValues(item.totals)" :key="key"><dt>{{ labelFor(key) }}</dt><dd>{{ formatInteger(value) }}</dd></div></dl></article></div>


      </template>

    </div>

    <div v-show="activeReport === 'ledger' && (showSummary || showDetails)" id="ledger-results" class="reports-panel report-results" :aria-label="t('admin.copy.5a74ef63')">


      <template v-if="ledgerResult">
        <dl v-if="showSummary" class="reports-summary report-metrics report-summary-tags"><div v-for="metric in ledgerMetrics(ledgerResult.summary)" :key="metric[0]"><dt><span>{{ labelFor(metric[0]) }}</span></dt><dd><strong>{{ formatInteger(metric[1]) }}</strong></dd></div></dl>
        <div v-if="showSummary" class="reports-balance"><h4>{{ t('admin.copy.fda22cd7') }}</h4><dl v-if="showSummary" class="reports-summary report-metrics report-summary-tags"> <div v-for="metric in balanceMetrics(ledgerResult)" :key="metric[0]"><dt><span>{{ labelFor(metric[0]) }}</span></dt><dd><strong>{{ formatInteger(metric[1]) }}</strong></dd></div></dl></div>
        <div v-if="showDetails" class="table-wrap reports-table-wrap"><table class="console-table reports-table" :class="{'console-responsive-table':!ledgerResult.items.length}"><thead><tr><th>{{ t('admin.copy.b96f43c6') }}</th><th v-for="metric in ledgerMetrics(ledgerResult.summary)" :key="metric[0]">{{ labelFor(metric[0]) }}</th></tr></thead><tbody><tr v-for="item in ledgerResult.items" :key="item.key"><td :data-label="String(t('admin.copy.b96f43c6'))">{{ associationGroupLabel(item, ledgerResult.query.group_by, item.key, item.label, associationLocale) }}</td><td :data-label="String(labelFor(metric[0]))" v-for="metric in ledgerMetrics(item.totals)" :key="metric[0]">{{ formatInteger(metric[1]) }}</td></tr><tr v-if="!ledgerResult.items.length"><td :colspan="ledgerMetrics(ledgerResult.summary).length + 1" class="empty-state">{{ t('admin.copy.24e33023') }}</td></tr></tbody></table></div>
        <div v-if="showDetails && ledgerResult.items.length" class="reports-cards"><article v-for="item in ledgerResult.items" :key="item.key"><h4>{{ associationGroupLabel(item, ledgerResult.query.group_by, item.key, item.label, associationLocale) }}</h4><dl><div v-for="[key, value] in rowValues(item.totals)" :key="key"><dt>{{ labelFor(key) }}</dt><dd>{{ formatInteger(value) }}</dd></div></dl></article></div>


      </template>

    </div>
<template v-if="showDetails" #pagination><template v-if="activeReport==='betting' && bettingResult"><TablePagination class="reports-pagination" :total="bettingResult.total_groups" :page="Math.floor(bettingOffset / pageSizes.betting + 1)" :count="bettingResult.items.length" :locale="locale" :page-size="pageSizes.betting" :previous-disabled="bettingOffset === 0 || bettingBusy || ledgerBusy" :next-disabled="!hasNext(bettingResult.total_groups, bettingOffset, bettingResult.items.length) || bettingBusy || ledgerBusy" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.ee97c955')" :go-to="(target: number) => pageReport('betting', target - (Math.floor(bettingOffset / pageSizes.betting + 1)))" :change-page-size="async (size: number) => { pageSizes.betting = size; await loadReports({ betting: 0, ledger: ledgerOffset }, committedQuery!.filters, committedQuery!.groups) }" :busy="bettingBusy || ledgerBusy" /></template><template v-else-if="activeReport==='ledger' && ledgerResult"><TablePagination class="reports-pagination" :total="ledgerResult.total_groups" :page="Math.floor(ledgerOffset / pageSizes.ledger + 1)" :count="ledgerResult.items.length" :locale="locale" :page-size="pageSizes.ledger" :previous-disabled="ledgerOffset === 0 || bettingBusy || ledgerBusy" :next-disabled="!hasNext(ledgerResult.total_groups, ledgerOffset, ledgerResult.items.length) || bettingBusy || ledgerBusy" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.ee97c955')" :go-to="(target: number) => pageReport('ledger', target - (Math.floor(ledgerOffset / pageSizes.ledger + 1)))" :change-page-size="async (size: number) => { pageSizes.ledger = size; await loadReports({ betting: bettingOffset, ledger: 0 }, committedQuery!.filters, committedQuery!.groups) }" :busy="bettingBusy || ledgerBusy" /></template></template>
    </ManagementList>
    <WithdrawalReport v-if="activeReport === 'withdrawal'" :account="props.account" :brand-id="props.brandId" @session-invalid="emit('session-invalid')" />
    <RewardReports v-if="activeReport === 'reward'" :account="props.account" :brand-id="props.brandId" @session-invalid="emit('session-invalid')" />
    <CommissionReport v-if="activeReport === 'posting'" :account="props.account" :brand-id="props.brandId" @session-invalid="emit('session-invalid')" />
    </div>
  </section>
</template>

<style scoped>
.reports-management{display:grid;gap:16px;color:var(--ink);min-width:0}.reports-heading,.reports-section-heading,.reports-actions{display:flex;align-items:center;justify-content:space-between;gap:12px}.reports-heading h2{margin:2px 0 0;font-size:1.25rem}.reports-eyebrow{margin:0;color:#68758a;font-size:var(--body-font-size)}.reports-tabs,.reports-presets,.reports-actions{display:flex;gap:8px;flex-wrap:wrap}.report-panel{display:grid;gap:14px;min-width:0}.reports-tabs button{border:1px solid var(--line);border-radius:8px;background:var(--surface);min-height:var(--control-height);padding:4px 12px;line-height:22px;max-width:100%;white-space:normal;color:inherit;font:inherit;cursor:pointer}.reports-tabs button[aria-selected=true]{background:var(--primary-light);border-color:var(--primary);color:var(--primary-dark)}.reports-filters{background:var(--canvas)}.reports-filter-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px}.reports-filter-grid label{display:grid;gap:6px;min-width:0;font-size:.88rem}.reports-filter-grid input{box-sizing:border-box;width:100%;min-width:0;border:1px solid var(--line);border-radius:8px;padding:9px 10px;background:var(--surface);font:inherit}.reports-button{min-height:var(--control-height);border:1px solid var(--line);border-radius:8px;padding:8px 12px;font:inherit;cursor:pointer;background:var(--surface);color:inherit;box-sizing:border-box;padding-block:4px;line-height:22px}.reports-button.primary{background:var(--primary);border-color:var(--primary);color:var(--surface)}.reports-button:disabled{opacity:.5;cursor:not-allowed}.reports-section-heading h3{margin:0}.reports-section-heading p,.reports-note,.reports-state{margin:4px 0 0;color:var(--muted);font-size:.88rem}.reports-context{display:flex;flex-wrap:wrap;gap:6px 14px;color:var(--ink-secondary);font-size:var(--body-font-size);overflow-wrap:anywhere}.reports-summary{display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:8px}.reports-summary>div{display:grid;gap:5px;min-width:0;border:1px solid var(--line);border-radius:9px;padding:10px}.reports-summary span{color:var(--muted);font-size:var(--body-font-size)}.reports-summary strong{overflow-wrap:anywhere;font-variant-numeric:tabular-nums}.reports-balance{display:grid;gap:8px;border-top:1px solid #e8ecf2;padding-top:12px}.reports-balance h4{margin:0}.reports-table-wrap{width:100%;overflow-x:auto;border:1px solid var(--line);border-radius:9px}.reports-table{width:100%;min-width:730px;border-collapse:collapse;font-variant-numeric:tabular-nums}.reports-table th,.reports-table td{height:46px;box-sizing:border-box;padding:8px 10px;border-bottom:1px solid var(--line-subtle);text-align:left;white-space:nowrap}.reports-table th{position:sticky;top:0;background:var(--canvas);color:var(--ink-secondary);font-size:var(--body-font-size)}.reports-table tr:last-child td{border-bottom:0}.reports-error{margin:0;border-radius:8px;padding:10px 12px;background:var(--danger-bg);color:var(--danger-text)}.reports-note{margin:0}.reports-cards{display:none}.reports-key{overflow-wrap:anywhere;color:var(--muted);font-size:var(--body-font-size)}.reports-cards article{border:1px solid var(--line);border-radius:9px;padding:12px}.reports-cards h4{margin:0}.reports-cards dl{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px;margin:10px 0 0}.reports-cards dl div{min-width:0}.reports-cards dt{color:var(--muted);font-size:var(--body-font-size)}.reports-cards dd{margin:3px 0 0;font-weight:550;overflow-wrap:anywhere;font-variant-numeric:tabular-nums}
.reports-export-actions{display:flex;align-items:center;gap:10px;flex-wrap:wrap}
@media(max-width:700px){.reports-management{gap:12px}.reports-heading{align-items:flex-start}.reports-panel{padding:12px}.reports-filter-grid{grid-template-columns:minmax(0,1fr)}.reports-actions .reports-button{width:100%}.reports-table-wrap{display:none}.reports-cards{display:grid;gap:9px}.reports-summary{grid-template-columns:repeat(2,minmax(0,1fr))}.reports-section-heading{align-items:flex-start}.reports-context{display:grid;gap:4px}}
.reports-section-heading{flex-wrap:wrap;min-width:0}.reports-section-heading>div{min-width:0}.reports-section-heading button{max-width:100%;white-space:normal}
</style>
