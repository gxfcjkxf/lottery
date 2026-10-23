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
const displayDate = (value: string | null | undefined) => formatDisplayDateTime(value, displayLocale.value);
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { adminKey, useAdminI18n } from "./i18n";
import { COMMISSION_ANALYSIS_COVERAGE_FIELDS, COMMISSION_ANALYSIS_FIELDS, commissionAnalysisPermissions, createCommissionAnalysisApi, type CommissionAnalysisGroup, type CommissionAnalysisQuery, type CommissionAnalysisReport } from "./commission-analysis-api";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createCommissionAnalysisApi();
const { t, display } = useAdminI18n();
const permissions = computed(() => commissionAnalysisPermissions(props.account, props.brandId));
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const localInput = (date: Date) => { const p = (n: number) => String(n).padStart(2, "0"); return `${date.getFullYear()}-${p(date.getMonth() + 1)}-${p(date.getDate())}T${p(date.getHours())}:${p(date.getMinutes())}:${p(date.getSeconds())}`; };
const endDefault = new Date(Math.ceil(Date.now() / 1000) * 1000), startDefault = new Date(endDefault); startDefault.setDate(startDefault.getDate() - 6);
const fromDraft = ref(localInput(startDefault).slice(0,10)), toDraft = ref(localInput(endDefault).slice(0,10));
const groupDraft = ref<CommissionAnalysisGroup>("cycle"), agentDraft = ref(""), memberDraft = ref(""), cycleDraft = ref("");
const result = ref<CommissionAnalysisReport | null>(null), committed = ref<CommissionAnalysisQuery | null>(null);
const error = ref<string | import("@lottery/shared").LocalizedMessage>(""), exportError = ref<string | import("@lottery/shared").LocalizedMessage>(""), busy = ref(false), exportBusy = ref(false), offset = ref(0);
let requestGeneration = 0, exportGeneration = 0, alive = true, requestController: AbortController | null = null, exportController: AbortController | null = null;
const scopeKey = computed(() => JSON.stringify({ brand: props.brandId, actor: props.account.id, superAdmin: props.account.super_admin, brands: [...(props.account.brand_ids ?? [])].sort(), permissions: [...(props.account.permissions ?? [])].sort(), platform: [...(props.account.platform_permissions ?? [])].sort(), byBrand: [...(props.account.permissions_by_brand?.[props.brandId] ?? [])].sort(), allowed: permissions.value }));
const draftKey = computed(() => JSON.stringify([fromDraft.value, toDraft.value, groupDraft.value, agentDraft.value, memberDraft.value, cycleDraft.value]));
function clearCurrent() {
  requestGeneration++; exportGeneration++; requestController?.abort(); exportController?.abort(); requestController = null; exportController = null;
  result.value = null; committed.value = null; busy.value = false; exportBusy.value = false; offset.value = 0; exportError.value = "";
}
async function queryFromDraft(): Promise<CommissionAnalysisQuery> {
  const start = filterDateBoundary(fromDraft.value, false, 'local'), end = filterDateBoundary(toDraft.value, true, 'local');
  if (end <= start || end.getTime() - start.getTime() > 93 * 86_400_000) throw new Error(t('admin.copy.bddf70ec'));
  const agent = agentDraft.value.trim(), username = memberDraft.value.trim(), cycle = cycleDraft.value.trim();
  if ([agent, cycle].some((id) => id && !UUID.test(id))) throw new Error(t('admin.copy.e6b53084'));
  const memberId = username;
  return { from: start.toISOString(), to: end.toISOString(), group_by: groupDraft.value, limit: 20, offset: 0, ...(agent ? { agent_id: agent } : {}), ...(memberId ? { member_id: memberId } : {}), ...(cycle ? { cycle_id: cycle } : {}) };
}
function isCurrent(id: number, scope: string) { return alive && id === requestGeneration && scope === scopeKey.value; }
async function run(nextOffset = 0, base?: CommissionAnalysisQuery) {
  requestController?.abort(); exportController?.abort();
  const scope = scopeKey.value, id = ++requestGeneration; exportGeneration++;
  result.value = null; error.value = ""; exportError.value = ""; exportBusy.value = false; offset.value = nextOffset;
  if (!permissions.value.view) { committed.value = null; busy.value = false; error.value = t('admin.copy.e419de1e'); return; }
  let query: CommissionAnalysisQuery;
  try { query = base ? { ...base, limit: 20, offset: nextOffset } : await queryFromDraft(); }
  catch (cause) { committed.value = null; busy.value = false; error.value = cause instanceof Error ? publicErrorMessage(cause, 'read') : t('admin.copy.98ceb525'); return; }
  committed.value = { ...query }; busy.value = true; requestController = new AbortController(); const controller = requestController;
  try {
    const data = await api.report(props.brandId, query, controller.signal);
    if (isCurrent(id, scope) && committed.value && data.brand_id.toLowerCase() === props.brandId.toLowerCase()) result.value = data;
  } catch (cause) {
    if (!isCurrent(id, scope)) return;
    committed.value = null; result.value = null; error.value = cause instanceof Error ? publicErrorMessage(cause, 'read') : t('admin.copy.c3026898');
    if (cause instanceof AdminApiError && cause.status === 401) emit("session-invalid");
  } finally { if (isCurrent(id, scope)) { busy.value = false; requestController = null; } }
}
async function exportAll() {
  const saved = committed.value;
  if (!permissions.value.export || !saved || !result.value || !alive) return;
  const scope = scopeKey.value, requestId = requestGeneration, id = ++exportGeneration;
  exportController?.abort(); exportController = new AbortController(); const controller = exportController;
  exportBusy.value = true; exportError.value = "";
  try {
    const query = { from: saved.from, to: saved.to, group_by: saved.group_by, ...(saved.agent_id ? { agent_id: saved.agent_id } : {}), ...(saved.member_id ? { member_id: saved.member_id } : {}), ...(saved.cycle_id ? { cycle_id: saved.cycle_id } : {}) };
    const file = await api.exportCsv(props.brandId, query, controller.signal);
    if (!alive || id !== exportGeneration || requestId !== requestGeneration || scope !== scopeKey.value || committed.value !== saved) return;
    const url = URL.createObjectURL(new Blob([file.bytes], { type: "text/csv;charset=utf-8" })), anchor = document.createElement("a");
    anchor.href = url; anchor.download = file.filename; anchor.click(); setTimeout(() => URL.revokeObjectURL(url), 0);
  } catch (cause) {
    if (!alive || id !== exportGeneration || requestId !== requestGeneration || scope !== scopeKey.value || committed.value !== saved) return;
    exportError.value = cause instanceof Error ? publicErrorMessage(cause, 'read') : t('admin.copy.22f21e1d');
    if (cause instanceof AdminApiError && cause.status === 401) { clearCurrent(); error.value = t('admin.copy.503d09f2'); emit("session-invalid"); }
  } finally { if (alive && id === exportGeneration) { exportBusy.value = false; exportController = null; } }
}
function exact(value: string | null | boolean) {
  if (value === null) return t('admin.copy.a2688e12');
  if (typeof value === "boolean") return value ? t('admin.copy.dc02c354') : t('admin.copy.717c2384');
  try { const n = BigInt(value), neg = n < 0n, digits = (neg ? -n : n).toString().replace(/\B(?=(\d{3})+(?!\d))/g, ","); return `${neg ? "−" : ""}${digits}`; } catch { return value; }
}
const metricLabels: Record<typeof COMMISSION_ANALYSIS_FIELDS[number], string> = {
  observed_calculated_points: 'admin.copy.3958a017', calculated_points: 'admin.copy.b81ec9de',
  paid_entry_count: 'admin.copy.bcd9f804', paid_points: 'admin.copy.ecd7f664',
  adjustment_entry_count: 'admin.copy.71dee966', adjustment_credit_points: 'admin.copy.54b4374e', adjustment_debit_points: 'admin.copy.de4bbdcb',
  correction_entry_count: 'admin.copy.95f6b880', correction_credit_points: 'admin.copy.a5c37447', correction_debit_points: 'admin.copy.7d47af21',
  posting_entry_count: 'admin.copy.5ca2ea68', actual_net_points: 'admin.copy.132cd87b', manual_adjustment_net_points: 'admin.copy.5c426582',
  effective_target_points: 'admin.copy.4fe44a9b', calculation_minus_actual_points: 'admin.copy.f532f35d', effective_minus_actual_points: 'admin.copy.38671725',
  calculation_complete: 'admin.copy.4a441ef8', effective_target_complete: 'admin.copy.b53a8d76',
};
function metricLabel(field: typeof COMMISSION_ANALYSIS_FIELDS[number]) { const pair = metricLabels[field]; return t(adminKey(pair)); }
function coverageLabel(field: typeof COMMISSION_ANALYSIS_COVERAGE_FIELDS[number]) { const labels: Record<typeof COMMISSION_ANALYSIS_COVERAGE_FIELDS[number], string> = { selected_cycle_count: 'admin.copy.da74927e', ready_cycle_count: 'admin.copy.06da59e7', unready_cycle_count: 'admin.copy.23f1f524' }; const pair = labels[field]; return t(adminKey(pair)); }
function hasNext(report: CommissionAnalysisReport) { return BigInt(report.query.offset) + BigInt(report.items.length) < BigInt(report.total_groups) && report.query.offset < 1_000_000; }
function rangeTitle(group: CommissionAnalysisGroup) { return group === "cycle" ? t('admin.copy.cb0d09bb') : t('admin.copy.ff58c071'); }
watch(draftKey, () => { clearCurrent(); error.value = ""; }, { flush: "sync" });
watch(scopeKey, () => { clearCurrent(); if (!permissions.value.view) error.value = t('admin.copy.e419de1e'); }, { flush: "sync" });
onBeforeUnmount(() => { alive = false; clearCurrent(); });
</script>

<template>
  <section class="commission-analysis" aria-labelledby="commission-analysis-title">
    <header><div><h3 id="commission-analysis-title">{{ t('admin.copy.71f79d66') }}</h3></div><span v-global-loading v-if="busy" aria-live="polite">{{ t('admin.copy.27342bf3') }}</span></header>
    <p class="explanation">{{ t('admin.copy.d2024e47') }}</p>
    <p class="explanation">{{ t('admin.copy.0d081b49') }}</p>
    <p v-if="props.account.super_admin" class="explanation">{{ t('admin.copy.ff4afcd7') }}</p>
    <div class="filters" data-testid="commission-analysis-filters">
      <label>{{ t('filter.fromDate') }}<PlatformTemporalInput v-model="fromDraft" type="date" :locale="displayLocale" /></label>
      <label>{{ t('filter.toDate') }}<PlatformTemporalInput v-model="toDraft" type="date" :locale="displayLocale" /></label>
      <label>{{ t('admin.copy.6cc02c91') }}<select v-model="groupDraft"><option value="cycle">{{ t('admin.copy.cb0d09bb') }}</option><option value="agent">{{ t('admin.copy.ff58c071') }}</option></select></label>
      <label>{{ t('filter.agent') }}<FilterAssociationSelect v-model="agentDraft" :brand-id="props.brandId" surface="admin" :enabled="permissions.view" resource="report_commission" kind="agent" :label="t('filter.agent')" :locale="displayLocale" :scope-key="JSON.stringify(props.account)" /></label>
      <label>{{ t('filter.member') }}<FilterAssociationSelect v-model="memberDraft" :brand-id="props.brandId" surface="admin" :enabled="permissions.view" resource="report_commission" kind="member" :label="t('filter.member')" :locale="displayLocale" :scope-key="JSON.stringify(props.account)" /></label>
      <label>{{ t('filter.cycle') }}<FilterAssociationSelect v-model="cycleDraft" :brand-id="props.brandId" surface="admin" :enabled="permissions.view" resource="report_commission" kind="cycle" :label="t('filter.cycle')" :locale="displayLocale" :scope-key="JSON.stringify(props.account)" /></label>
    <button class="button button-primary" type="button" data-testid="commission-analysis-query" :disabled="busy || !permissions.view" @click="withGlobalLoading(run)">{{ t('admin.copy.a9b623ef') }}</button>
    </div>
    <div v-if="!permissions.view" class="actions"><span>{{ t('admin.copy.8bb94752') }}</span></div>
    <p v-if="error" class="error" role="alert">{{ displayPublicError(error) }}</p><p v-if="exportError" class="error" role="alert">{{ displayPublicError(exportError) }}</p>
    <template v-if="result">
      <div class="context"><span>{{ t('admin.copy.9c32661d') }} <AssociationName :record="result" kind="brand" :id="result.brand_id" :locale="associationLocale" /></span><span>{{ t('admin.copy.82d4e9bd') }} {{ displayDate(result.snapshot_at) }}</span><span>{{ t('admin.copy.c6bf9750') }} {{ result.timezone }}</span><span>{{ t('admin.copy.21fe3e8c') }} {{ rangeTitle(result.query.group_by) }}</span><span>[{{ displayDate(result.query.from) }}, {{ displayDate(result.query.to) }})</span><span>{{ t('admin.copy.8b96a575') }} {{ exact(result.total_groups) }}</span></div>
      <section class="coverage" aria-labelledby="commission-analysis-coverage-title"><h4 id="commission-analysis-coverage-title">{{ t('admin.copy.3150252c') }}</h4><dl><div v-for="field in COMMISSION_ANALYSIS_COVERAGE_FIELDS" :key="field"><dt>{{ coverageLabel(field) }}</dt><dd>{{ exact(result.coverage[field]) }}</dd></div></dl></section>
      <div class="actions"><button type="button" data-testid="commission-analysis-export" :disabled="!permissions.export || exportBusy || busy" @click="withGlobalLoading(exportAll)">{{ exportBusy ? t('admin.copy.1c800a70') : t('admin.copy.a53c96d1') }}</button><span v-if="!permissions.export">{{ t('admin.copy.97587160') }}</span></div>
      <section class="totals" data-testid="commission-analysis-summary" aria-labelledby="commission-analysis-summary-title"><h4 id="commission-analysis-summary-title">{{ t('admin.copy.17ea1be1') }}</h4><dl><div v-for="field in COMMISSION_ANALYSIS_FIELDS" :key="field"><dt>{{ metricLabel(field) }}</dt><dd>{{ exact(result.summary[field]) }}</dd></div></dl></section>
      <div v-if="result.items.length" class="table-wrap"><table><caption>{{ t('admin.copy.ec57878e') }}</caption><thead><tr><th scope="col">{{ t('admin.copy.34341794') }}</th><th v-for="field in COMMISSION_ANALYSIS_FIELDS" :key="field" scope="col">{{ metricLabel(field) }}</th></tr></thead><tbody><tr v-for="item in result.items" :key="item.key"><th scope="row"><span>{{ associationGroupLabel(item, result.query.group_by, item.key, item.label, associationLocale) }}</span></th><td v-for="field in COMMISSION_ANALYSIS_FIELDS" :key="field">{{ exact(item.totals[field]) }}</td></tr></tbody></table></div>
      <section v-if="result.items.length" class="mobile-groups" :aria-label="t('admin.copy.ec57878e')"><article v-for="item in result.items" :key="item.key"><h4>{{ associationGroupLabel(item, result.query.group_by, item.key, item.label, associationLocale) }}</h4><details><summary>{{ t('admin.copy.ec639451') }}</summary><dl><div v-for="field in COMMISSION_ANALYSIS_FIELDS" :key="field"><dt>{{ metricLabel(field) }}</dt><dd>{{ exact(item.totals[field]) }}</dd></div></dl></details></article></section>
      <p v-else class="explanation empty-state">{{ t('admin.copy.24e33023') }}</p>
      <nav class="pages" :aria-label="t('admin.copy.558ec4ec')"><span>{{ result.query.offset + 1 }}–{{ result.query.offset + result.items.length }} / {{ exact(result.total_groups) }}</span><button type="button" :disabled="busy || result.query.offset === 0" @click="withGlobalLoading(run, Math.max(0, result.query.offset - 20), result.query)">{{ t('admin.copy.dbe7a5d7') }}</button><button type="button" :disabled="busy || !hasNext(result)" @click="withGlobalLoading(run, result.query.offset + 20, result.query)">{{ t('admin.copy.2e9c9e7a') }}</button></nav>
    </template>
  </section>
</template>

<style scoped>
.commission-analysis{display:grid;gap:12px;min-width:0;border:1px solid var(--line);border-radius:12px;padding:16px;background:var(--surface);color:var(--ink)}.commission-analysis>header,.actions,.pages{display:flex;align-items:center;justify-content:space-between;gap:10px;flex-wrap:wrap}.commission-analysis h3,.commission-analysis header p{margin:0}.commission-analysis header p,.explanation,.context,.actions span{color:var(--muted);font-size:.86rem;line-height:1.55}.explanation{margin:0}.filters{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px}.filters label{display:grid;gap:5px;min-width:0;font-size:.86rem}.filters input,.filters select{box-sizing:border-box;width:100%;min-width:0;padding:9px;border:1px solid var(--line);border-radius:8px;font:inherit}.commission-analysis button{min-height:40px;padding:8px 12px;border:1px solid #c5ceda;border-radius:8px;background:var(--surface);color:inherit;font:inherit;cursor:pointer}.commission-analysis button:disabled{opacity:.5;cursor:not-allowed}.context{display:flex;flex-wrap:wrap;gap:4px 14px;overflow-wrap:anywhere}.coverage,.totals{display:grid;gap:8px}.coverage h4,.totals h4{margin:0}.coverage dl,.totals dl,.mobile-groups dl{display:grid;grid-template-columns:repeat(auto-fit,minmax(145px,1fr));gap:8px;margin:0}.coverage dl>div,.totals dl>div,.mobile-groups dl>div{display:grid;gap:5px;min-width:0;border:1px solid #e5eaf0;border-radius:8px;padding:10px}.coverage dt,.totals dt,.mobile-groups dt{color:var(--muted);font-size:.8rem}.coverage dd,.totals dd,.mobile-groups dd{margin:0;overflow-wrap:anywhere;font-variant-numeric:tabular-nums}.table-wrap{overflow:auto;border:1px solid #e2e7ef;border-radius:8px}.table-wrap table{border-collapse:collapse;min-width:1450px;width:100%;font-variant-numeric:tabular-nums}.table-wrap caption{text-align:left;padding:8px;font-weight:600}.table-wrap th,.table-wrap td{padding:8px;border-bottom:1px solid var(--line-subtle);text-align:left;white-space:nowrap}.table-wrap th{position:sticky;top:0;background:var(--canvas)}.table-wrap tbody th{max-width:220px;white-space:normal;overflow-wrap:anywhere}.table-wrap small{display:block;color:var(--muted)}.mobile-groups{display:none}.mobile-groups article{display:grid;gap:6px;border:1px solid #e2e7ef;border-radius:9px;padding:12px;min-width:0}.mobile-groups h4{margin:0}.mobile-groups code{overflow-wrap:anywhere;color:var(--muted)}.mobile-groups details summary{cursor:pointer;padding:8px 0}.pages{justify-content:flex-end}.pages span{margin-right:auto;color:var(--muted)}.pages button{min-width:92px}.error{margin:0;padding:10px;border-radius:8px;background:#fff0ee;color:#a32e1a}@media(max-width:700px){.commission-analysis{padding:12px}.filters{grid-template-columns:minmax(0,1fr)}.table-wrap{display:none}.mobile-groups{display:grid;gap:9px}.coverage dl,.totals dl{grid-template-columns:repeat(2,minmax(0,1fr))}.actions button{width:100%}.pages{justify-content:center}.pages span{flex-basis:100%;text-align:center}.pages button{flex:1}}
</style>
