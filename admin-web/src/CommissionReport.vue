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
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { adminKey, useAdminI18n } from "./i18n";
import { commissionReportPermissions, createCommissionReportApi, type CommissionGroup, type CommissionReport, type CommissionReportQuery, type CommissionTotals } from "./commission-report-api";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createCommissionReportApi();
const { t, display } = useAdminI18n();
const permissions = computed(() => commissionReportPermissions(props.account, props.brandId));
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const localDateTime = (value: Date) => { const p = (n: number) => String(n).padStart(2, "0"); return `${value.getFullYear()}-${p(value.getMonth() + 1)}-${p(value.getDate())}T${p(value.getHours())}:${p(value.getMinutes())}:${p(value.getSeconds())}`; };
const endDefault = new Date(Math.ceil(Date.now() / 1000) * 1000), startDefault = new Date(endDefault); startDefault.setDate(startDefault.getDate() - 6);
const fromDraft = ref(localDateTime(startDefault).slice(0,10)), toDraft = ref(localDateTime(endDefault).slice(0,10));
const agentDraft = ref(""), memberDraft = ref(""), cycleDraft = ref(""), groupDraft = ref<CommissionGroup>("day");
const committed = ref<CommissionReportQuery | null>(null), result = ref<CommissionReport | null>(null);
const error = ref<string | import("@lottery/shared").LocalizedMessage>(""), exportError = ref<string | import("@lottery/shared").LocalizedMessage>(""), busy = ref(false), exportBusy = ref(false), offset = ref(0);
let requestGeneration = 0, exportGeneration = 0, alive = true;
const scopeKey = computed(() => JSON.stringify({ brand: props.brandId, actor: props.account.id, super: props.account.super_admin, brands: [...(props.account.brand_ids ?? [])].sort(), permissions: [...(props.account.permissions ?? [])].sort(), platform: [...(props.account.platform_permissions ?? [])].sort(), byBrand: [...(props.account.permissions_by_brand?.[props.brandId] ?? [])].sort(), allowed: permissions.value }));
function invalidate() { requestGeneration++; exportGeneration++; result.value = null; committed.value = null; busy.value = false; exportBusy.value = false; offset.value = 0; error.value = ""; exportError.value = ""; }
async function queryFromDraft(): Promise<CommissionReportQuery> {
  const from = filterDateBoundary(fromDraft.value, false, 'local'), to = filterDateBoundary(toDraft.value, true, 'local');
  if (to <= from || to.getTime() - from.getTime() > 93 * 86400000) throw new Error(t('admin.copy.bddf70ec'));
  const agent = agentDraft.value.trim(), username = memberDraft.value.trim(), cycle = cycleDraft.value.trim();
  if ([agent, cycle].some((id) => id && !uuid.test(id))) throw new Error(t('admin.copy.e6b53084'));
  const memberId = username;
  return { from: from.toISOString(), to: to.toISOString(), group_by: groupDraft.value, limit: 20, offset: 0, ...(agent ? { agent_id: agent } : {}), ...(memberId ? { member_id: memberId } : {}), ...(cycle ? { cycle_id: cycle } : {}) };
}
function isCurrent(id: number, scope: string) { return alive && id === requestGeneration && scope === scopeKey.value; }
async function run(nextOffset = 0, base?: CommissionReportQuery) {
  const scope = scopeKey.value, id = ++requestGeneration;
  exportGeneration++; exportBusy.value = false;
  result.value = null; error.value = ""; exportError.value = ""; offset.value = nextOffset;
  if (!permissions.value.view) { error.value = t('admin.copy.8019a922'); return; }
  let query: CommissionReportQuery;
  try { query = base ? { ...base, limit: 20, offset: nextOffset } : await queryFromDraft(); }
  catch (cause) { error.value = cause instanceof Error ? publicErrorMessage(cause, 'read') : t('admin.copy.98ceb525'); return; }
  committed.value = { ...query }; busy.value = true;
  try {
    const data = await api.report(props.brandId, query);
    if (isCurrent(id, scope) && committed.value && data.brand_id.toLowerCase() === props.brandId.toLowerCase()) result.value = data;
  } catch (cause) {
    if (!isCurrent(id, scope)) return;
    result.value = null; error.value = cause instanceof Error ? publicErrorMessage(cause, 'read') : t('admin.copy.c3026898');
    if (cause instanceof AdminApiError && cause.status === 401) { committed.value = null; emit("session-invalid"); }
  } finally { if (isCurrent(id, scope)) busy.value = false; }
}
async function exportAll() {
  const saved = committed.value;
  if (!permissions.value.export || !saved || !result.value || !alive) return;
  const scope = scopeKey.value, id = ++exportGeneration;
  exportBusy.value = true; exportError.value = "";
  try {
    const file = await api.exportCsv(props.brandId, { from: saved.from, to: saved.to, group_by: saved.group_by, ...(saved.agent_id ? { agent_id: saved.agent_id } : {}), ...(saved.member_id ? { member_id: saved.member_id } : {}), ...(saved.cycle_id ? { cycle_id: saved.cycle_id } : {}) });
    if (!alive || id !== exportGeneration || scope !== scopeKey.value || saved !== committed.value) return;
    const url = URL.createObjectURL(new Blob([file.bytes], { type: "text/csv;charset=utf-8" }));
    const anchor = document.createElement("a"); anchor.href = url; anchor.download = file.filename; anchor.click(); setTimeout(() => URL.revokeObjectURL(url), 0);
  } catch (cause) {
    if (!alive || id !== exportGeneration || scope !== scopeKey.value || saved !== committed.value) return;
    exportError.value = cause instanceof Error ? publicErrorMessage(cause, 'read') : t('admin.copy.8e66aa98');
    if (cause instanceof AdminApiError && cause.status === 401) { invalidate(); emit("session-invalid"); }
  } finally { if (id === exportGeneration) exportBusy.value = false; }
}
const fields: readonly (keyof CommissionTotals)[] = ["entry_count", "paid_entry_count", "paid_points", "adjustment_entry_count", "adjustment_credit_points", "adjustment_debit_points", "correction_entry_count", "correction_credit_points", "correction_debit_points", "net_points"];
const metricLabels: Record<keyof CommissionTotals, string> = {
  entry_count: 'admin.copy.a96a323a', paid_entry_count: 'admin.copy.7c65b1b9', paid_points: 'admin.copy.ff3bfa25', adjustment_entry_count: 'admin.copy.e6662700', adjustment_credit_points: 'admin.copy.36dcbc81', adjustment_debit_points: 'admin.copy.bfbbbb56', correction_entry_count: 'admin.copy.f5a1fc64', correction_credit_points: 'admin.copy.a5c37447', correction_debit_points: 'admin.copy.7d47af21', net_points: 'admin.copy.a0bee852',
};
function label(field: keyof CommissionTotals) { const pair = metricLabels[field]; return t(adminKey(pair)); }
function exact(value: string) { try { const n = BigInt(value), negative = n < 0n, digits = (negative ? -n : n).toString().replace(/\B(?=(\d{3})+(?!\d))/g, ","); return `${negative ? "−" : ""}${digits}`; } catch { return value; } }
function hasNext(value: CommissionReport) { try { return BigInt(value.query.offset) + 20n <= 1_000_000n && BigInt(value.query.offset) + BigInt(value.items.length) < BigInt(value.total_groups); } catch { return false; } }
function groupTitle(value: CommissionGroup) { return value === "day" ? t('admin.copy.f91541ed') : value === "agent" ? t('admin.copy.219d0268') : t('admin.copy.653c73b9'); }
watch(scopeKey, () => { invalidate(); if (!permissions.value.view) error.value = t('admin.copy.8019a922'); }, { flush: "sync" });
onBeforeUnmount(() => { alive = false; invalidate(); });
</script>

<template>
  <section class="commission-report" aria-labelledby="commission-report-title">
    <header><div><h3 id="commission-report-title">{{ t('admin.copy.c7622ab2') }}</h3></div></header>
    <p class="definition">{{ t('admin.copy.86580929') }}</p>
    <div class="filters" data-testid="commission-report-filters">
      <label>{{ t('filter.fromDate') }}<PlatformTemporalInput v-model="fromDraft" type="date" :locale="displayLocale" /></label>
      <label>{{ t('filter.toDate') }}<PlatformTemporalInput v-model="toDraft" type="date" :locale="displayLocale" /></label>
      <label>{{ t('filter.agent') }}<FilterAssociationSelect v-model="agentDraft" :brand-id="props.brandId" surface="admin" :enabled="permissions.view" resource="report_commission" kind="agent" :label="t('filter.agent')" :locale="displayLocale" :scope-key="JSON.stringify(props.account)" /></label>
      <label>{{ t('filter.member') }}<FilterAssociationSelect v-model="memberDraft" :brand-id="props.brandId" surface="admin" :enabled="permissions.view" resource="report_commission" kind="member" :label="t('filter.member')" :locale="displayLocale" :scope-key="JSON.stringify(props.account)" /></label>
      <label>{{ t('filter.cycle') }}<FilterAssociationSelect v-model="cycleDraft" :brand-id="props.brandId" surface="admin" :enabled="permissions.view" resource="report_commission" kind="cycle" :label="t('filter.cycle')" :locale="displayLocale" :scope-key="JSON.stringify(props.account)" /></label>
      <label>{{ t('admin.copy.6cc02c91') }}<select v-model="groupDraft" data-testid="commission-report-group"><option value="day">{{ t('admin.copy.f91541ed') }}</option><option value="agent">{{ t('admin.copy.219d0268') }}</option><option value="cycle">{{ t('admin.copy.653c73b9') }}</option></select></label>
    <button class="button button-primary" type="button" data-testid="commission-report-query" :disabled="busy || !permissions.view" @click="withGlobalLoading(run)">{{ t('admin.copy.2bd50b3e') }}</button>
    </div>
    <p v-if="props.account.super_admin" class="definition">{{ t('admin.copy.786dcf42') }}</p>
    <p v-if="error" class="error" role="alert">{{ displayPublicError(error) }}</p><p v-if="exportError" class="error" role="alert">{{ displayPublicError(exportError) }}</p>
    <template v-if="result">
      <div class="context"><span>{{ t('admin.copy.9c32661d') }} <AssociationName :record="result" kind="brand" :id="result.brand_id" :locale="associationLocale" /></span><span>{{ t('admin.copy.82d4e9bd') }} {{ formatDisplayDateTime(result.snapshot_at, displayLocale) }}</span><span>{{ t('admin.copy.c6bf9750') }} {{ result.timezone }}</span><span>{{ t('admin.copy.e4bcc7b2') }} {{ groupTitle(result.query.group_by) }}</span><span>{{ t('admin.copy.343763ae') }} [{{ result.query.from }}, {{ result.query.to }})</span><span>{{ t('admin.copy.9bf72772') }} {{ t('admin.copy.b863c1ed') }}: <template v-if="result.query.agent_id"><AssociationName :record="result.query" kind="agent" :id="result.query.agent_id" :locale="associationLocale" /></template><span v-else>{{ t('admin.copy.ef22c1d2') }}</span>, {{ t('admin.copy.56b2d19b') }}: <template v-if="result.query.member_id"><AssociationName :record="result.query" kind="member" :id="result.query.member_id" :locale="associationLocale" /></template><span v-else>{{ t('admin.copy.ef22c1d2') }}</span>, {{ t('admin.copy.11c28a36') }}: {{ result.query.cycle_id ?? t('admin.copy.ef22c1d2') }}</span><span>{{ t('admin.copy.a0c04ae0') }} {{ exact(result.total_groups) }}</span></div>
      <div class="actions"><button type="button" data-testid="commission-report-export" :disabled="!permissions.export || exportBusy" @click="withGlobalLoading(exportAll)">{{ exportBusy ? t('admin.copy.1c800a70') : t('admin.copy.800c75fc') }}</button><span v-if="!permissions.export">{{ t('admin.copy.fcd324d4') }}</span></div>
      <div class="metrics" data-testid="commission-report-summary"><div v-for="field in fields" :key="field"><span>{{ label(field) }}</span><strong>{{ exact(result.summary[field]) }}</strong></div></div>
      <div v-if="result.items.length" class="table-wrap"><table><thead><tr><th>{{ t('admin.copy.b96f43c6') }}</th><th v-for="field in fields" :key="field">{{ label(field) }}</th></tr></thead><tbody><tr v-for="item in result.items" :key="item.key"><td>{{ associationGroupLabel(item, result.query.group_by, item.key, item.label, associationLocale) }}</td><td v-for="field in fields" :key="field">{{ exact(item.totals[field]) }}</td></tr></tbody></table></div>
      <p v-else class="definition empty-state">{{ t('admin.copy.898225da') }}</p>
      <nav class="pages" aria-label="Commission report pages"><span>{{ result.query.offset + 1 }}–{{ result.query.offset + result.items.length }} / {{ exact(result.total_groups) }}</span><button type="button" :disabled="busy || result.query.offset === 0" @click="withGlobalLoading(run, Math.max(0, result.query.offset - 20), result.query)">{{ t('admin.copy.dbe7a5d7') }}</button><button type="button" :disabled="busy || !hasNext(result)" @click="withGlobalLoading(run, result.query.offset + 20, result.query)">{{ t('admin.copy.2e9c9e7a') }}</button></nav>
    </template>
    <p v-global-loading v-else-if="busy" class="definition" aria-live="polite">{{ t('admin.copy.27342bf3') }}</p>
  </section>
</template>

<style scoped>
.commission-report{display:grid;gap:12px;min-width:0;border:1px solid var(--line);border-radius:12px;padding:16px;background:var(--surface);color:var(--ink)}.commission-report header,.actions,.pages{display:flex;align-items:center;justify-content:space-between;gap:10px;flex-wrap:wrap}.commission-report h3,.commission-report header p{margin:0}.commission-report header p,.definition,.context,.actions span{color:var(--muted);font-size:.86rem;line-height:1.55}.filters{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px}.filters label{display:grid;gap:5px;min-width:0;font-size:.86rem}.filters input,.filters select{box-sizing:border-box;width:100%;min-width:0;padding:9px;border:1px solid var(--line);border-radius:8px;font:inherit}.commission-report button{min-height:40px;padding:8px 12px;border:1px solid #c5ceda;border-radius:8px;background:var(--surface);color:inherit;font:inherit}.commission-report button:disabled{opacity:.5}.context{display:flex;flex-wrap:wrap;gap:4px 14px;overflow-wrap:anywhere}.metrics{display:grid;grid-template-columns:repeat(auto-fit,minmax(145px,1fr));gap:8px}.metrics div{display:grid;gap:5px;min-width:0;border:1px solid #e5eaf0;border-radius:8px;padding:10px}.metrics span{color:var(--muted);font-size:.8rem}.metrics strong{overflow-wrap:anywhere;font-variant-numeric:tabular-nums}.table-wrap{overflow:auto;border:1px solid #e2e7ef;border-radius:8px}table{border-collapse:collapse;min-width:900px;width:100%;font-variant-numeric:tabular-nums}th,td{padding:8px;border-bottom:1px solid var(--line-subtle);text-align:left;white-space:nowrap}th{position:sticky;top:0;background:var(--canvas)}.error{margin:0;padding:10px;border-radius:8px;background:#fff0ee;color:#a32e1a}@media(max-width:700px){.commission-report{padding:12px}.filters{grid-template-columns:minmax(0,1fr)}.metrics{grid-template-columns:repeat(2,minmax(0,1fr))}.table-wrap{max-height:60vh}.actions button{width:100%}}
</style>
