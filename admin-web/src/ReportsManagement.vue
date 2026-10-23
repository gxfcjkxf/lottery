<script setup lang="ts">
import FilterAssociationSelect from "@lottery/shared/FilterAssociationSelect.vue";
import { filterDateBoundary } from "@lottery/shared";
import { associationGroupLabel } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import PlatformTemporalInput from "@lottery/shared/PlatformTemporalInput.vue";
import { vGlobalLoading } from "@lottery/shared";
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { createReportsApi, reportsPermissions } from "./reports-api";
import { createReportExportApi, reportExportPermissions } from "./report-export-api";
import WithdrawalReport from "./WithdrawalReport.vue";
import CommissionReport from "./CommissionReport.vue";
import CommissionAnalysisReport from "./CommissionAnalysisReport.vue";
import RewardReports from "./RewardReports.vue";
import AttributionReport from "./AttributionReportManagement.vue";
import { adminKey, useAdminI18n } from "./i18n";
import { dateRangePreset, type DateRangePreset } from "@lottery/shared";

const { t, locale, display } = useAdminI18n();
const ui = (key: string) => t(adminKey(key));

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
const bettingError = ref<string | import("@lottery/shared").LocalizedMessage>("");
const ledgerError = ref<string | import("@lottery/shared").LocalizedMessage>("");
const bettingBusy = ref(false);
const ledgerBusy = ref(false);
const bettingExportBusy = ref(false);
const ledgerExportBusy = ref(false);
const bettingExportError = ref<string | import("@lottery/shared").LocalizedMessage>("");
const ledgerExportError = ref<string | import("@lottery/shared").LocalizedMessage>("");
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
  const start = filterDateBoundary(from, false, 'local').toISOString();
  const end = filterDateBoundary(to, true, 'local').toISOString();
  if (!start || !end) throw new Error("请选择有效的报表开始时间和结束时间。");
  if (Date.parse(end) <= Date.parse(start)) throw new Error("报表结束时间必须晚于开始时间。");
  if (Date.parse(end) - Date.parse(start) > 93 * 24 * 60 * 60 * 1000) throw new Error("查询时间范围不能超过 93 天。");
  const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
  if (gameId && !uuid.test(gameId)) throw new Error("彩种筛选 UUID 格式无效。");
  const member = memberUsername ? { id: memberUsername } : null;
  return { from: start, to: end, memberId: member?.id ?? null, gameId: gameId || null };
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
    if (permissions.value.betting) bettingError.value = errorText(cause);
    if (permissions.value.ledger) ledgerError.value = errorText(cause);
  }
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
function formatDate(value: string): string{ return formatDisplayDateTime(value, displayLocale.value); }
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
    correction_open_count: "admin.copy.c4cdafbd", entry_count: "admin.copy.e0c59808", net_points: "admin.copy.3195be70", recharge_points: "admin.copy.cbf177ce",
    prize_credit_points: "admin.copy.2b9638d8", prize_reversal_points: "admin.copy.4b205d41", available_points: "admin.copy.5b7ceca3", frozen_points: "admin.copy.7c11c5d7",
    account_count: "admin.copy.64df0e2b", withdrawal_points: "admin.copy.4a6bf99f", total_points: "admin.copy.cb6f8f43",
  };
  const label = labels[key];
  if (!label) throw new Error(`Unsupported report metric: ${key}`);
  return ui(label);
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
  setPreset("today");
  setErrorForPermission();
});
onBeforeUnmount(() => { alive = false; exportGeneration++; exportTickets.betting++; exportTickets.ledger++; requestTicket++; });
</script>

<template>
  <section class="reports-management" aria-labelledby="reports-title">
    <header class="reports-heading">
      <div><h2 id="reports-title">{{ t('admin.copy.76514f50') }}</h2></div>

    </header>

    <nav class="reports-tabs" :aria-label="t('admin.copy.9dd4dae0')">
      <button type="button" :aria-pressed="activeReport === 'betting'" @click="activeReport = 'betting'">{{ t('admin.copy.b30b1af4') }}</button>
      <button type="button" :aria-pressed="activeReport === 'ledger'" @click="activeReport = 'ledger'">{{ t('admin.copy.5a74ef63') }}</button>
    </nav>

    <section class="reports-panel reports-filters" :aria-label="t('admin.copy.5cfbfc91')">
      <div class="reports-presets" :aria-label="t('admin.copy.56549499')">
        <button class="reports-button button-secondary" type="button" @click="setPreset('today')">{{ t('admin.report.today') }}</button>
        <button class="reports-button button-secondary" type="button" @click="setPreset('yesterday')">{{ t('admin.report.yesterday') }}</button>
        <button class="reports-button button-secondary" type="button" @click="setPreset('last-week')">{{ t('admin.report.lastWeek') }}</button>
        <button class="reports-button button-secondary" type="button" @click="setPreset('this-month')">{{ t('admin.report.thisMonth') }}</button>
        <button class="reports-button button-secondary" type="button" @click="setPreset('last-month')">{{ t('admin.report.lastMonth') }}</button>
      </div>
      <div class="reports-filter-grid">
        <label>{{ t('filter.fromDate') }}<PlatformTemporalInput v-model="fromDraft" type="date" :locale="displayLocale" /></label>
        <label>{{ t('filter.toDate') }}<PlatformTemporalInput v-model="toDraft" type="date" :locale="displayLocale" /></label>
        <label>{{ t('admin.copy.b43d1eaf') }}<FilterAssociationSelect v-model="memberDraft" :brand-id="brandId" surface="admin" :enabled="activeReport === 'betting' ? permissions.betting : permissions.ledger" :resource="`report_${activeReport}`" kind="member" :label="t('filter.member')" :locale="displayLocale" :scope-key="JSON.stringify(account)" /></label>
        <label v-if="activeReport === 'betting'">{{ t('admin.report.game') }}<FilterAssociationSelect v-model="gameDraft" :brand-id="brandId" surface="admin" :enabled="permissions.betting" resource="report_betting" kind="game" :label="t('filter.game')" :locale="displayLocale" :scope-key="JSON.stringify(account)" /></label>
        <label v-if="activeReport === 'betting'">{{ t('admin.report.dimension') }}<select v-model="bettingGroup" :aria-label="t('admin.report.dimension')"><option value="day">{{ t('admin.report.byDate') }}</option><option value="game">{{ t('admin.copy.ef04a723') }}</option><option value="member">{{ t('admin.copy.93a783ef') }}</option></select></label>
        <label v-else>{{ t('admin.copy.c9058d43') }}<select v-model="ledgerGroup" :aria-label="t('admin.copy.c9058d43')"><option value="day">{{ t('admin.copy.f91541ed') }}</option><option value="entry_type">{{ t('admin.copy.5ea104fa') }}</option></select></label>
        <button class="reports-button button button-primary" type="button" :disabled="bettingBusy || ledgerBusy" @click="withGlobalLoading(queryReports)">{{ t('admin.copy.6db5d760') }}</button>
      </div>
    </section>

    <p v-if="props.account.super_admin" class="reports-note">{{ t('admin.copy.6a77d8e0') }}</p>
    <p class="reports-pending" role="note">{{ t('admin.copy.0a7586a4') }}</p>

    <section v-show="activeReport === 'betting'" class="reports-panel" aria-labelledby="betting-title">
      <div class="reports-section-heading"><div><h3 id="betting-title">{{ t('admin.copy.b30b1af4') }}</h3></div><div class="reports-export-actions"><span v-global-loading v-if="bettingBusy" class="reports-state">{{ t('admin.copy.27342bf3') }}</span><button class="reports-button button-secondary" type="button" :disabled="!exportPermissions.betting || !committedQuery || bettingExportBusy" @click="withGlobalLoading(exportReport, 'betting')">{{ bettingExportBusy ? t('admin.copy.1c800a70') : t('admin.copy.800c75fc') }}</button></div></div>
      <p v-if="bettingExportError" class="reports-error" role="alert">{{ display(bettingExportError) }}</p>
      <p v-if="bettingError" class="reports-error" role="alert">{{ display(bettingError) }}</p>
      <template v-if="bettingResult">
        <div class="reports-context"><span>{{ t('admin.copy.f676f47d') }} <AssociationName :record="bettingResult" kind="brand" :id="bettingResult.brand_id" :locale="associationLocale" /></span><span>{{ t('admin.copy.82d4e9bd') }} {{ formatDate(bettingResult.snapshot_at) }}</span><span>{{ t('admin.copy.f257539c') }} {{ bettingResult.timezone }}</span><span>{{ t('admin.copy.8b5d46a2') }}{{ formatDate(bettingResult.query.from) }}, {{ formatDate(bettingResult.query.to) }})</span><span>{{ t('admin.copy.e4bcc7b2') }} {{ bettingResult.query.group_by }}</span><span>{{ t('admin.copy.56b2d19b') }} <template v-if="bettingResult.query.member_id"><AssociationName :record="bettingResult.query" kind="member" :id="bettingResult.query.member_id" :locale="associationLocale" /></template><span v-else>{{ t('admin.copy.ef22c1d2') }}</span></span><span>{{ t('admin.copy.7492bbfb') }} <template v-if="bettingResult.query.game_id"><AssociationName :record="bettingResult.query" kind="game" :id="bettingResult.query.game_id" :locale="associationLocale" /></template><span v-else>{{ t('admin.copy.ef22c1d2') }}</span></span></div>
        <p class="reports-definition">{{ t('admin.copy.172616ca') }}</p>
        <div class="reports-summary"><div v-for="metric in bettingMetrics(bettingResult.summary)" :key="metric[0]"><span>{{ labelFor(metric[0]) }}</span><strong>{{ formatInteger(metric[1]) }}</strong></div></div>
        <div v-if="bettingResult.items.length" class="reports-table-wrap"><table class="reports-table"><thead><tr><th>{{ t('admin.copy.b96f43c6') }}</th><th v-for="metric in bettingMetrics(bettingResult.items[0].totals)" :key="metric[0]">{{ labelFor(metric[0]) }}</th></tr></thead><tbody><tr v-for="item in bettingResult.items" :key="item.key"><td>{{ associationGroupLabel(item, bettingResult.query.group_by, item.key, item.label, associationLocale) }}</td><td v-for="metric in bettingMetrics(item.totals)" :key="metric[0]">{{ formatInteger(metric[1]) }}</td></tr></tbody></table></div>
        <div v-if="bettingResult.items.length" class="reports-cards"><article v-for="item in bettingResult.items" :key="item.key"><h4>{{ associationGroupLabel(item, bettingResult.query.group_by, item.key, item.label, associationLocale) }}</h4><dl><div v-for="[key, value] in rowValues(item.totals)" :key="key"><dt>{{ labelFor(key) }}</dt><dd>{{ formatInteger(value) }}</dd></div></dl></article></div>
        <p v-if="!bettingResult.items.length" class="reports-note empty-state">{{ t('admin.copy.ae99ca74') }}</p>
        <div class="reports-pagination"><span>{{ t('admin.copy.d116a312') }} {{ bettingOffset + 1 }}–{{ bettingOffset + bettingResult.items.length }} {{ t('admin.copy.32904ee4') }} {{ formatInteger(bettingResult.total_groups) }} {{ t('admin.copy.e2daf069') }}</span><button class="reports-button button-secondary" type="button" :disabled="bettingOffset === 0 || bettingBusy || ledgerBusy" @click="withGlobalLoading(pageReport, 'betting', -1)">{{ t('admin.copy.dbe7a5d7') }}</button><button class="reports-button button-secondary" type="button" :disabled="!hasNext(bettingResult.total_groups, bettingOffset, bettingResult.items.length) || bettingBusy || ledgerBusy" @click="withGlobalLoading(pageReport, 'betting', 1)">{{ t('admin.copy.ee97c955') }}</button></div>
      </template>
      <p v-else-if="!bettingBusy && !bettingError && permissions.betting" class="reports-note">{{ t('admin.copy.ebf1a4dd') }}</p>
    </section>

    <section v-show="activeReport === 'ledger'" class="reports-panel" aria-labelledby="ledger-title">
      <div class="reports-section-heading"><div><h3 id="ledger-title">{{ t('admin.copy.5a74ef63') }}</h3></div><div class="reports-export-actions"><span v-global-loading v-if="ledgerBusy" class="reports-state">{{ t('admin.copy.27342bf3') }}</span><button class="reports-button button-secondary" type="button" :disabled="!exportPermissions.ledger || !committedQuery || ledgerExportBusy" @click="withGlobalLoading(exportReport, 'ledger')">{{ ledgerExportBusy ? t('admin.copy.1c800a70') : t('admin.copy.800c75fc') }}</button></div></div>
      <p v-if="ledgerExportError" class="reports-error" role="alert">{{ display(ledgerExportError) }}</p>
      <p v-if="ledgerError" class="reports-error" role="alert">{{ display(ledgerError) }}</p>
      <template v-if="ledgerResult">
        <div class="reports-context"><span>{{ t('admin.copy.f676f47d') }} <AssociationName :record="ledgerResult" kind="brand" :id="ledgerResult.brand_id" :locale="associationLocale" /></span><span>{{ t('admin.copy.82d4e9bd') }} {{ formatDate(ledgerResult.snapshot_at) }}</span><span>{{ t('admin.copy.f257539c') }} {{ ledgerResult.timezone }}</span><span>{{ t('admin.copy.8b5d46a2') }}{{ formatDate(ledgerResult.query.from) }}, {{ formatDate(ledgerResult.query.to) }})</span><span>{{ t('admin.copy.e4bcc7b2') }} {{ ledgerResult.query.group_by }}</span><span>{{ t('admin.copy.56b2d19b') }} <template v-if="ledgerResult.query.member_id"><AssociationName :record="ledgerResult.query" kind="member" :id="ledgerResult.query.member_id" :locale="associationLocale" /></template><span v-else>{{ t('admin.copy.ef22c1d2') }}</span></span></div>
        <p class="reports-definition">{{ t('admin.copy.c330f5e7') }}</p>
        <div class="reports-summary"><div v-for="metric in ledgerMetrics(ledgerResult.summary)" :key="metric[0]"><span>{{ labelFor(metric[0]) }}</span><strong>{{ formatInteger(metric[1]) }}</strong></div></div>
        <div class="reports-balance"><h4>{{ t('admin.copy.fda22cd7') }}</h4><div class="reports-summary"> <div v-for="metric in balanceMetrics(ledgerResult)" :key="metric[0]"><span>{{ labelFor(metric[0]) }}</span><strong>{{ formatInteger(metric[1]) }}</strong></div></div></div>
        <div v-if="ledgerResult.items.length" class="reports-table-wrap"><table class="reports-table"><thead><tr><th>{{ t('admin.copy.b96f43c6') }}</th><th v-for="metric in ledgerMetrics(ledgerResult.items[0].totals)" :key="metric[0]">{{ labelFor(metric[0]) }}</th></tr></thead><tbody><tr v-for="item in ledgerResult.items" :key="item.key"><td>{{ associationGroupLabel(item, ledgerResult.query.group_by, item.key, item.label, associationLocale) }}</td><td v-for="metric in ledgerMetrics(item.totals)" :key="metric[0]">{{ formatInteger(metric[1]) }}</td></tr></tbody></table></div>
        <div v-if="ledgerResult.items.length" class="reports-cards"><article v-for="item in ledgerResult.items" :key="item.key"><h4>{{ associationGroupLabel(item, ledgerResult.query.group_by, item.key, item.label, associationLocale) }}</h4><dl><div v-for="[key, value] in rowValues(item.totals)" :key="key"><dt>{{ labelFor(key) }}</dt><dd>{{ formatInteger(value) }}</dd></div></dl></article></div>
        <p v-if="!ledgerResult.items.length" class="reports-note empty-state">{{ t('admin.copy.ae99ca74') }}</p>
        <div class="reports-pagination"><span>{{ t('admin.copy.d116a312') }} {{ ledgerOffset + 1 }}–{{ ledgerOffset + ledgerResult.items.length }} {{ t('admin.copy.32904ee4') }} {{ formatInteger(ledgerResult.total_groups) }} {{ t('admin.copy.e2daf069') }}</span><button class="reports-button button-secondary" type="button" :disabled="ledgerOffset === 0 || bettingBusy || ledgerBusy" @click="withGlobalLoading(pageReport, 'ledger', -1)">{{ t('admin.copy.dbe7a5d7') }}</button><button class="reports-button button-secondary" type="button" :disabled="!hasNext(ledgerResult.total_groups, ledgerOffset, ledgerResult.items.length) || bettingBusy || ledgerBusy" @click="withGlobalLoading(pageReport, 'ledger', 1)">{{ t('admin.copy.ee97c955') }}</button></div>
      </template>
      <p v-else-if="!ledgerBusy && !ledgerError && permissions.ledger" class="reports-note">{{ t('admin.copy.6552b4c8') }}</p>
    </section>
    <WithdrawalReport :account="props.account" :brand-id="props.brandId" @session-invalid="emit('session-invalid')" />
    <CommissionReport :key="`commission-report:${props.account.id}:${props.brandId}`" :account="props.account" :brand-id="props.brandId" @session-invalid="emit('session-invalid')" />
    <CommissionAnalysisReport :key="`commission-analysis:${props.account.id}:${props.brandId}`" :account="props.account" :brand-id="props.brandId" @session-invalid="emit('session-invalid')" />
    <RewardReports :key="`reward-report:${props.account.id}:${props.brandId}`" :account="props.account" :brand-id="props.brandId" @session-invalid="emit('session-invalid')" />
    <AttributionReport :key="`attribution-report:${props.account.id}:${props.brandId}`" :account="props.account" :brand-id="props.brandId" @session-invalid="emit('session-invalid')" />
  </section>
</template>

<style scoped>
.reports-management{display:grid;gap:16px;color:var(--ink);min-width:0}.reports-heading,.reports-section-heading,.reports-actions,.reports-pagination{display:flex;align-items:center;justify-content:space-between;gap:12px}.reports-heading h2{margin:2px 0 0;font-size:1.25rem}.reports-eyebrow{margin:0;color:#68758a;font-size:.78rem}.reports-tabs,.reports-presets,.reports-actions{display:flex;gap:8px;flex-wrap:wrap}.reports-tabs button{border:1px solid var(--line);border-radius:8px;background:var(--surface);padding:9px 14px;color:inherit;font:inherit;cursor:pointer}.reports-tabs button[aria-pressed=true]{background:#254a83;border-color:#254a83;color:var(--surface)}.reports-panel{display:grid;gap:14px;min-width:0;border:1px solid var(--line);border-radius:12px;padding:16px;background:var(--surface)}.reports-filters{background:var(--canvas)}.reports-filter-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px}.reports-filter-grid label{display:grid;gap:6px;min-width:0;font-size:.88rem}.reports-filter-grid input,.reports-filter-grid select{box-sizing:border-box;width:100%;min-width:0;border:1px solid var(--line);border-radius:8px;padding:9px 10px;background:var(--surface);font:inherit}.reports-button{min-height:40px;border:1px solid #c5ceda;border-radius:8px;padding:8px 12px;font:inherit;cursor:pointer;background:var(--surface);color:inherit}.reports-button.primary{background:#254a83;border-color:#254a83;color:var(--surface)}.reports-button:disabled{opacity:.5;cursor:not-allowed}.reports-section-heading h3{margin:0}.reports-section-heading p,.reports-definition,.reports-note,.reports-pending,.reports-state{margin:4px 0 0;color:var(--muted);font-size:.88rem}.reports-definition{line-height:1.55}.reports-context{display:flex;flex-wrap:wrap;gap:6px 14px;color:#475467;font-size:.82rem;overflow-wrap:anywhere}.reports-summary{display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:8px}.reports-summary>div{display:grid;gap:5px;min-width:0;border:1px solid #e5eaf0;border-radius:9px;padding:10px}.reports-summary span{color:var(--muted);font-size:.8rem}.reports-summary strong{overflow-wrap:anywhere;font-variant-numeric:tabular-nums}.reports-balance{display:grid;gap:8px;border-top:1px solid #e8ecf2;padding-top:12px}.reports-balance h4{margin:0}.reports-table-wrap{width:100%;overflow-x:auto;border:1px solid #e2e7ef;border-radius:9px}.reports-table{width:100%;min-width:730px;border-collapse:collapse;font-variant-numeric:tabular-nums}.reports-table th,.reports-table td{height:46px;box-sizing:border-box;padding:8px 10px;border-bottom:1px solid var(--line-subtle);text-align:left;white-space:nowrap}.reports-table th{position:sticky;top:0;background:var(--canvas);color:#475467;font-size:.82rem}.reports-table tr:last-child td{border-bottom:0}.reports-pagination{justify-content:flex-end;flex-wrap:wrap}.reports-pagination span{margin-right:auto;color:var(--muted);font-size:.86rem}.reports-error{margin:0;border-radius:8px;padding:10px 12px;background:#fff0ee;color:#a32e1a}.reports-note,.reports-pending{margin:0}.reports-pending{border-left:3px solid #d8a62c;padding:8px 10px;background:#fff9eb}.reports-cards{display:none}.reports-key{overflow-wrap:anywhere;color:var(--muted);font-size:.8rem}.reports-cards article{border:1px solid #e2e7ef;border-radius:9px;padding:12px}.reports-cards h4{margin:0}.reports-cards dl{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px;margin:10px 0 0}.reports-cards dl div{min-width:0}.reports-cards dt{color:var(--muted);font-size:.78rem}.reports-cards dd{margin:3px 0 0;font-weight:550;overflow-wrap:anywhere;font-variant-numeric:tabular-nums}
.reports-export-actions{display:flex;align-items:center;gap:10px;flex-wrap:wrap}
@media(max-width:700px){.reports-management{gap:12px}.reports-heading{align-items:flex-start}.reports-panel{padding:12px}.reports-filter-grid{grid-template-columns:minmax(0,1fr)}.reports-actions .reports-button{width:100%}.reports-table-wrap{display:none}.reports-cards{display:grid;gap:9px}.reports-pagination{justify-content:center}.reports-pagination span{flex-basis:100%;text-align:center}.reports-pagination .reports-button{flex:1;min-width:0}.reports-summary{grid-template-columns:repeat(2,minmax(0,1fr))}.reports-section-heading{align-items:flex-start}.reports-context{display:grid;gap:4px}}
</style>
