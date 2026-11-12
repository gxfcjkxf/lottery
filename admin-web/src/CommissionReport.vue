<script setup lang="ts">
import { formatPoints } from '@lottery/shared'

const showSummary = ref(true), showDetails = ref(true)
import {vInlineValidation} from '@lottery/shared'
const filterReset=ref(0)
const metricDetail=ref<Record<string, string> | null>(null)
import DetailModal from "@lottery/shared/DetailModal.vue"
import ManagementList from "@lottery/shared/ManagementList.vue"
import TablePagination from "@lottery/shared/TablePagination.vue"
const agentDraftSearchPending = ref(false);
import FilterAssociationSearch from "@lottery/shared/FilterAssociationSearch.vue";
const memberSearchPending = ref(false);
import SelectControl from '@lottery/shared/SelectControl.vue'
import './commission-management.css';
import {commissionDateRange} from './commission-list-query';
import {platformDisplaySettings} from '@lottery/shared';
import { useOperationToast } from "@lottery/shared";
import FilterAssociationSelect from "@lottery/shared/FilterAssociationSelect.vue";
import { associationGroupLabel } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import PlatformTemporalInput from "@lottery/shared/PlatformTemporalInput.vue";
import { vGlobalLoading } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { adminKey, useAdminI18n } from "./i18n";
import { commissionReportPermissions, createCommissionReportApi, type CommissionGroup, type CommissionReport, type CommissionReportQuery, type CommissionTotals } from "./commission-report-api";
const pageSize = ref(20)


const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createCommissionReportApi();
const { t } = useAdminI18n();
const permissions = computed(() => commissionReportPermissions(props.account, props.brandId));
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const endDefault = new Date(Math.ceil(Date.now() / 1000) * 1000), startDefault = new Date(endDefault); startDefault.setDate(startDefault.getDate() - 6);
function platformDay(value:Date) {const zone=platformDisplaySettings.value?.timezone;if(!zone)return '';const parts=Object.fromEntries(new Intl.DateTimeFormat('en-CA',{timeZone:zone,year:'numeric',month:'2-digit',day:'2-digit'}).formatToParts(value).map(p=>[p.type,p.value]));return `${parts.year}-${parts.month}-${parts.day}`;}
const fromDraft = ref(platformDay(startDefault)), toDraft = ref(platformDay(endDefault));
const agentDraft = ref(""), memberDraft = ref(""), cycleDraft = ref(""), groupDraft = ref<CommissionGroup>("day");
const committed = ref<CommissionReportQuery | null>(null), result = ref<CommissionReport | null>(null);
const error = ref<string | import("@lottery/shared").LocalizedMessage>(""), busy = ref(false), offset = ref(0);
let requestGeneration = 0, alive = true;
const scopeKey = computed(() => JSON.stringify({ brand: props.brandId, actor: props.account.id, super: props.account.super_admin, brands: [...(props.account.brand_ids ?? [])].sort(), permissions: [...(props.account.permissions ?? [])].sort(), platform: [...(props.account.platform_permissions ?? [])].sort(), byBrand: [...(props.account.permissions_by_brand?.[props.brandId] ?? [])].sort(), allowed: permissions.value }));
function invalidate() { metricDetail.value=null; requestGeneration++; result.value = null; committed.value = null; busy.value = false; offset.value = 0; error.value = ""; }
function queryFromDraft(): CommissionReportQuery {
  let range:ReturnType<typeof commissionDateRange>;
  try{range=commissionDateRange(fromDraft.value,toDraft.value)}catch{throw new Error(t('admin.copy.bddf70ec'))}
  if(!range.from || !range.to || Date.parse(range.to)-Date.parse(range.from)>93*86400000) throw new Error(t('admin.copy.bddf70ec'));
  const agent = agentDraft.value.trim(), username = memberDraft.value.trim(), cycle = cycleDraft.value.trim();
  if ([agent, cycle].some((id) => id && !uuid.test(id))) throw new Error(t('admin.copy.e6b53084'));
  const memberId = username;
  return { from: range.from, to: range.to, group_by: groupDraft.value, limit: pageSize.value, offset: 0, ...(agent ? { agent_id: agent } : {}), ...(memberId ? { member_id: memberId } : {}), ...(cycle ? { cycle_id: cycle } : {}) };
}
function isCurrent(id: number, scope: string) { return alive && id === requestGeneration && scope === scopeKey.value; }
async function run(nextOffset = 0, base?: CommissionReportQuery) {
  if (agentDraftSearchPending.value || memberSearchPending.value) return;
  const scope = scopeKey.value, id = ++requestGeneration;
  error.value = ""; if (!permissions.value.view) { error.value = t('admin.copy.8019a922'); return; }
  let query: CommissionReportQuery;
  try { query = base ? { ...base, limit: pageSize.value, offset: nextOffset } : queryFromDraft(); }
  catch (cause) { error.value = cause instanceof Error ? publicErrorMessage(cause, 'read') : t('admin.copy.98ceb525'); return; }
  busy.value = true;
  try {
    const data = await api.report(props.brandId, query);
    if (isCurrent(id, scope) && data.brand_id.toLowerCase() === props.brandId.toLowerCase()) { result.value = data; committed.value = { ...query }; offset.value = nextOffset; }
  } catch (cause) {
    if (!isCurrent(id, scope)) return;
    error.value = cause instanceof Error ? publicErrorMessage(cause, 'read') : t('admin.copy.c3026898');
    if (cause instanceof AdminApiError && cause.status === 401) { invalidate(); emit("session-invalid"); }
  } finally { if (isCurrent(id, scope)) busy.value = false; }
}
const fields: readonly (keyof CommissionTotals)[] = ["entry_count", "paid_entry_count", "paid_points", "adjustment_entry_count", "adjustment_credit_points", "adjustment_debit_points", "correction_entry_count", "correction_credit_points", "correction_debit_points", "net_points"];
const headlineFields: readonly (keyof CommissionTotals)[] = ['paid_points','adjustment_credit_points','adjustment_debit_points','correction_credit_points','correction_debit_points','net_points'];
const extraFields = fields.filter(field=>!headlineFields.includes(field));
const metricLabels: Record<keyof CommissionTotals, string> = {
  entry_count: 'admin.copy.a96a323a', paid_entry_count: 'admin.copy.7c65b1b9', paid_points: 'admin.copy.ff3bfa25', adjustment_entry_count: 'admin.copy.e6662700', adjustment_credit_points: 'admin.copy.36dcbc81', adjustment_debit_points: 'admin.copy.bfbbbb56', correction_entry_count: 'admin.copy.f5a1fc64', correction_credit_points: 'admin.copy.a5c37447', correction_debit_points: 'admin.copy.7d47af21', net_points: 'admin.copy.a0bee852',
};
function label(field: keyof CommissionTotals) { const pair = metricLabels[field]; return t(adminKey(pair)); }
function exact(value: string) { return formatPoints(value, displayLocale.value).replace(/^-/, "−"); }
function hasNext(value: CommissionReport) { try { return BigInt(value.query.offset) + 20n <= 1_000_000n && BigInt(value.query.offset) + BigInt(value.items.length) < BigInt(value.total_groups); } catch { return false; } }
watch(scopeKey, () => { invalidate(); if (!permissions.value.view) error.value = t('admin.copy.8019a922'); }, { flush: "sync" });
onMounted(() => { if (permissions.value.view) void withGlobalLoading(run); });
onBeforeUnmount(() => { alive = false; invalidate(); });

useOperationToast(() => error.value, value => displayPublicError(value), 'error');

</script>

<template>
  <ManagementList class="commission-report report-view" :aria-label="t('admin.commission.ui.postingReport')">
    <template #filters><form v-inline-validation novalidate @submit.prevent="withGlobalLoading(run)" class="commission-toolbar filters report-filters ui-filter-bar" data-testid="commission-report-filters">
      <label><PlatformTemporalInput :label="t('filter.fromDate')" :placeholder="t('filter.fromDate')" v-model="fromDraft" type="date" :locale="displayLocale" /></label>
      <label><PlatformTemporalInput :label="t('filter.toDate')" :placeholder="t('filter.toDate')" v-model="toDraft" type="date" :locale="displayLocale" /></label>
      <label><FilterAssociationSearch :reset-key="filterReset" v-model="agentDraft" v-model:pending="agentDraftSearchPending" :brand-id="props.brandId" surface="admin" :enabled="permissions.view" resource="report_commission" kind="agent" :label="t('filter.agent')" :locale="displayLocale" :scope-key="JSON.stringify(props.account)" /></label>
      <label><FilterAssociationSearch :reset-key="filterReset" v-model="memberDraft" :brand-id="props.brandId" surface="admin" :enabled="permissions.view" resource="report_commission"  v-model:pending="memberSearchPending" :label="t('filter.member')" :locale="displayLocale" :scope-key="JSON.stringify(props.account)" /></label>
      <label><FilterAssociationSelect v-model="cycleDraft" :brand-id="props.brandId" surface="admin" :enabled="permissions.view" resource="report_commission" kind="cycle" :label="t('filter.cycle')" :locale="displayLocale" :scope-key="JSON.stringify(props.account)" /></label>
      <label><SelectControl :placeholder="t('admin.copy.6cc02c91')" v-model="groupDraft" data-testid="commission-report-group" :label="String(t('admin.copy.6cc02c91'))" :options="[{ value: 'day', label: (String(t('admin.copy.f91541ed'))).trim() }, { value: 'agent', label: (String(t('admin.copy.219d0268'))).trim() }, { value: 'cycle', label: (String(t('admin.copy.653c73b9'))).trim() }]" /></label>
    <label class="report-display-option"><input v-model="showSummary" type="checkbox" />{{ t('admin.reports.showSummary') }}</label><label class="report-display-option"><input v-model="showDetails" type="checkbox" />{{ t('admin.reports.showDetails') }}</label><button class="button button-primary" type="submit" data-testid="commission-report-query" :disabled="agentDraftSearchPending || memberSearchPending || busy || !permissions.view">{{ t('admin.copy.2bd50b3e') }}</button>
    </form></template>

    <p v-if="!permissions.view" class="commission-note">{{ t('admin.copy.8019a922') }}</p>
    <div v-if="showSummary || showDetails" class="report-results">

    <template v-if="result">
      <section v-if="showSummary" class="totals" data-testid="commission-report-summary"><dl class="commission-metrics report-summary-tags"><div v-for="field in headlineFields" :key="field"><dt>{{ label(field) }}</dt><dd>{{ exact(result.summary[field]) }}</dd></div></dl></section>
      <div v-if="showDetails" class="table-wrap"><table class="console-table console-responsive-table"><thead><tr><th>{{ t('admin.copy.b96f43c6') }}</th><th v-for="field in headlineFields" :key="field">{{ label(field) }}</th><th>{{ t('admin.commission.ui.reportDetails') }}</th></tr></thead><tbody><tr v-for="item in result.items" :key="item.key"><td :data-label="t('admin.copy.b96f43c6')">{{ associationGroupLabel(item,result.query.group_by,item.key,item.label,associationLocale) }}</td><td v-for="field in headlineFields" :key="field" :data-label="label(field)">{{ exact(item.totals[field]) }}</td><td :data-label="t('admin.commission.ui.reportDetails')"><button type="button" class="text-button" @click="metricDetail=item.totals">{{ t('admin.commission.ui.moreMetrics') }}</button></td></tr><tr v-if="!result.items.length"><td :colspan="headlineFields.length + 2" class="empty-state">{{ t('admin.copy.24e33023') }}</td></tr></tbody></table></div>

    </template>
    <p v-global-loading v-else-if="busy" class="definition" aria-live="polite">{{ t('admin.copy.27342bf3') }}</p>
    </div>
  <template #pagination v-if="showDetails && (result)"><TablePagination class="pages" :aria-label="t('admin.commission.ui.postingPages')" :total="result.total_groups" :page="Math.floor(result.query.offset / pageSize + 1)" :count="result.items.length" :locale="associationLocale" :page-size="pageSize" :previous-disabled="agentDraftSearchPending || memberSearchPending || busy || result.query.offset === 0" :next-disabled="agentDraftSearchPending || memberSearchPending || busy || !hasNext(result)" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.2e9c9e7a')" :go-to="(target: number) => run((target - 1) * pageSize, result!.query)" :change-page-size="async (size: number) => { pageSize = size; await run(0, result!.query) }" :busy="busy" /></template></ManagementList>
  <DetailModal v-if="metricDetail" :title="t('admin.commission.ui.reportDetails')" :close-label="t('association.close')" @close="metricDetail=null"><dl class="report-extra"><div v-for="field in extraFields" :key="field"><dt>{{ label(field) }}</dt><dd>{{ exact(metricDetail[field]) }}</dd></div></dl></DetailModal>
</template>

<style scoped>
.commission-report header,.actions{display:flex;align-items:center;justify-content:space-between;gap:10px;flex-wrap:wrap}.commission-report h3,.commission-report header p{margin:0}.commission-report header p,.definition,.actions span{color:var(--muted);font-size:var(--body-font-size);line-height:1.55}.filters{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px}.filters label{display:grid;gap:5px;min-width:0;font-size:var(--body-font-size)}.filters input{box-sizing:border-box;width:100%;min-width:0;padding:9px;border:1px solid var(--line);border-radius:8px;font:inherit}.commission-report button:disabled{opacity:.5}.metrics{display:grid;grid-template-columns:repeat(auto-fit,minmax(145px,1fr));gap:8px}.metrics div{display:grid;gap:5px;min-width:0;border:1px solid var(--line);border-radius:8px;padding:10px}.metrics span{color:var(--muted);font-size:var(--body-font-size)}.metrics strong{overflow-wrap:anywhere;font-variant-numeric:tabular-nums}.table-wrap{overflow:auto;border:1px solid var(--line);border-radius:8px}table{border-collapse:collapse;min-width:900px;width:100%;font-variant-numeric:tabular-nums}th,td{padding:8px;border-bottom:1px solid var(--line-subtle);text-align:left;white-space:nowrap}th{position:sticky;top:0;background:var(--canvas)}.error{margin:0;padding:10px;border-radius:8px;background:var(--danger-bg);color:var(--danger-text)}@media(max-width:700px){.commission-report{padding:12px}.filters{grid-template-columns:minmax(0,1fr)}.metrics{grid-template-columns:repeat(2,minmax(0,1fr))}.table-wrap{max-height:60vh}.actions button{width:100%}}

.commission-analysis, .commission-report {padding:0;border:0;background:transparent;}
.commission-toolbar label {grid-template-columns:minmax(0,1fr);}
.table-wrap {overflow:visible;border:0;min-width:0;}
.table-wrap table {min-width:0;}
.table-wrap th,.table-wrap td {white-space:normal;position:static;overflow-wrap:anywhere;}
.report-extra {display:grid;gap:6px;min-width:180px;margin:8px 0;}
.report-extra div {display:flex;justify-content:space-between;gap:12px;}
.report-extra dt {color:var(--muted);}.report-extra dd {margin:0;}
.commission-analysis .actions button,.commission-report .actions button {width:auto;}
@media(max-width:700px){.table-wrap{display:block;max-height:none}}
</style>
