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
import { createRewardReportApi, rewardReportPermissions, type RewardReport, type RewardReportKind, type RewardReportQuery } from "./reward-report-api";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, display } = useAdminI18n(), api = createRewardReportApi();
const permissions = computed(() => rewardReportPermissions(props.account, props.brandId));
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const utcInput = (date: Date) => `${date.toISOString().slice(0, 19)}`;
const end = new Date(Math.ceil(Date.now() / 60_000) * 60_000), start = new Date(end.getTime() - 6 * 86_400_000);
const fromDraft = ref(utcInput(start).slice(0,10)), toDraft = ref(utcInput(end).slice(0,10)), memberDraft = ref(""), orderDraft = ref("");
const mode = ref<RewardReportKind>("rewards"), groupDraft = ref<RewardReportQuery["group_by"]>("day");
const committed = ref<{ kind: RewardReportKind; query: RewardReportQuery } | null>(null), result = ref<RewardReport | null>(null);
const error = ref<string | import("@lottery/shared").LocalizedMessage>(""), exportError = ref<string | import("@lottery/shared").LocalizedMessage>(""), busy = ref(false), exportBusy = ref(false);
let readGeneration = 0, exportGeneration = 0, alive = true, readController: AbortController | undefined, exportController: AbortController | undefined;
const scopeKey = computed(() => JSON.stringify({ brand: props.brandId, actor: props.account.id, super: props.account.super_admin, brands: [...(props.account.brand_ids ?? [])].sort(), permissions: [...(props.account.permissions ?? [])].sort(), platform: [...(props.account.platform_permissions ?? [])].sort(), byBrand: [...(props.account.permissions_by_brand?.[props.brandId] ?? [])].sort(), allowed: permissions.value }));
const groups = computed(() => mode.value === "rewards" ? ["day", "member", "order"] : ["day", "member", "state"]);
function abortRead() { readGeneration++; readController?.abort(); readController = undefined; busy.value = false; }
function abortExport() { exportGeneration++; exportController?.abort(); exportController = undefined; exportBusy.value = false; }
function clearScope() { abortRead(); abortExport(); committed.value = null; result.value = null; error.value = ""; exportError.value = ""; }
async function queryDraft(): Promise<RewardReportQuery> {
  const from = filterDateBoundary(fromDraft.value, false, 'utc').toISOString(), to = filterDateBoundary(toDraft.value, true, 'utc').toISOString(), fromMs = Date.parse(from), toMs = Date.parse(to);
  if (toMs <= fromMs || toMs - fromMs > 93 * 86_400_000) throw new Error(t('admin.copy.bddf70ec'));
  const member = memberDraft.value.trim(), order = orderDraft.value.trim();
  if (order && !uuid.test(order)) throw new Error(t('admin.copy.864d220e'));
  const memberId = member;
  return { from, to, group_by: groupDraft.value, limit: 20, offset: 0, ...(memberId ? { member_id: memberId } : {}), ...(order ? { order_id: order } : {}) };
}
function current(generation: number, scope: string) { return alive && generation === readGeneration && scope === scopeKey.value; }
async function run(saved?: { kind: RewardReportKind; query: RewardReportQuery }) {
  abortRead(); abortExport(); result.value = null; error.value = ""; exportError.value = "";
  const generation = readGeneration, scope = scopeKey.value;
  if (!permissions.value.view) { error.value = t('admin.copy.56ef8abd'); return; }
  let request: { kind: RewardReportKind; query: RewardReportQuery };
  try { request = saved ? { kind: saved.kind, query: { ...saved.query } } : { kind: mode.value, query: await queryDraft() }; }
  catch (cause) { error.value = cause instanceof Error ? publicErrorMessage(cause, 'read') : t('admin.copy.98ceb525'); return; }
  committed.value = request; busy.value = true; readController = new AbortController();
  try {
    const report = await api.read(request.kind, props.brandId, request.query, readController.signal);
    if (current(generation, scope) && report.brand_id.toLowerCase() === props.brandId.toLowerCase() && JSON.stringify(committed.value) === JSON.stringify(request)) result.value = report;
  } catch (cause) {
    if (!current(generation, scope)) return;
    error.value = cause instanceof Error ? publicErrorMessage(cause, 'read') : t('admin.copy.c3026898');
    if (cause instanceof AdminApiError && cause.status === 401) { committed.value = null; emit("session-invalid"); }
  } finally { if (current(generation, scope)) { busy.value = false; readController = undefined; } }
}
async function exportAll() {
  const saved = committed.value;
  if (!permissions.value.view || !permissions.value.export || !saved || !result.value || !alive) return;
  abortExport(); const generation = exportGeneration, scope = scopeKey.value, snapshot = saved;
  exportBusy.value = true; exportError.value = ""; exportController = new AbortController();
  try {
    const file = await api.export(snapshot.kind, props.brandId, { from: snapshot.query.from, to: snapshot.query.to, group_by: snapshot.query.group_by, ...(snapshot.query.member_id ? { member_id: snapshot.query.member_id } : {}), ...(snapshot.query.order_id ? { order_id: snapshot.query.order_id } : {}) }, exportController.signal);
    if (!alive || generation !== exportGeneration || scope !== scopeKey.value || snapshot !== committed.value) return;
    const url = URL.createObjectURL(new Blob([file.bytes], { type: "text/csv;charset=utf-8" })), anchor = document.createElement("a");
    anchor.href = url; anchor.download = file.filename; anchor.click(); setTimeout(() => URL.revokeObjectURL(url), 0);
  } catch (cause) {
    if (!alive || generation !== exportGeneration || scope !== scopeKey.value || snapshot !== committed.value) return;
    exportError.value = cause instanceof Error ? publicErrorMessage(cause, 'read') : t('admin.copy.8e66aa98');
    if (cause instanceof AdminApiError && cause.status === 401) { clearScope(); emit("session-invalid"); }
  } finally { if (generation === exportGeneration) { exportBusy.value = false; exportController = undefined; } }
}
const fields = computed(() => mode.value === "rewards" ? ["entry_count", "grant_entry_count", "grant_points", "reversal_entry_count", "reversal_points", "net_points"] : ["order_count", "original_points", "granted_count", "granted_points", "pending_count", "pending_points", "revoked_count", "revoked_points"]);
const names: Record<string, string> = {
  entry_count: 'admin.copy.1d7e7e9b', grant_entry_count: 'admin.copy.a9b90197', grant_points: 'admin.copy.b83d5c43', reversal_entry_count: 'admin.copy.0bc0ad18', reversal_points: 'admin.copy.3089b321', net_points: 'admin.copy.a0bee852',
  order_count: 'admin.copy.c2179a71', original_points: 'admin.copy.dea61704', granted_count: 'admin.copy.facb8181', granted_points: 'admin.copy.43c77853', pending_count: 'admin.copy.458ea6d1', pending_points: 'admin.copy.a8473f43', revoked_count: 'admin.copy.44e2367f', revoked_points: 'admin.copy.6b110769',
};
 function metric(field: string) { return t(adminKey(names[field]!)); }
function exact(value: string) { try { const n = BigInt(value), neg = n < 0n, digits = (neg ? -n : n).toString().replace(/\B(?=(\d{3})+(?!\d))/g, ","); return `${neg ? "−" : ""}${digits}`; } catch { return value; } }
function count(totals: RewardReport["summary"], field: string) { return exact((totals as unknown as Record<string, string>)[field]!); }
function groupLabel(group: string) { return group === "day" ? t('admin.copy.f91541ed') : group === "member" ? t('admin.copy.5073d20c') : group === "order" ? t('admin.copy.2e709a31') : t('admin.copy.70a80209'); }
function stateLabel(key: string) { return key === "granted" ? t('admin.copy.86b01b91') : key === "revocation_pending" ? t('admin.copy.2b729ea1') : key === "revoked" ? t('admin.copy.176004df') : key; }
function hasNext(report: RewardReport) { return BigInt(report.query.offset) + BigInt(report.items.length) < BigInt(report.total_groups) && report.query.offset + 20 <= 1_000_000; }
async function page(offset: number) { if (committed.value) await run({ kind: committed.value.kind, query: { ...committed.value.query, limit: 20, offset } }); }
watch(mode, () => { groupDraft.value = "day"; clearScope(); }, { flush: "sync" });
watch(scopeKey, () => { clearScope(); if (!permissions.value.view) error.value = t('admin.copy.56ef8abd'); }, { flush: "sync" });
onBeforeUnmount(() => { alive = false; clearScope(); });
</script>

<template>
  <section class="reward-reports" aria-labelledby="reward-reports-title">
    <header><div><h3 id="reward-reports-title">{{ t('admin.copy.1d928afb') }}</h3></div></header>
    <div class="modes" role="tablist" :aria-label="t('admin.copy.397de89b')">
      <button type="button" role="tab" data-testid="reward-report-mode-postings" :aria-selected="mode === 'rewards'" @click="mode = 'rewards'">{{ t('admin.copy.15791d7e') }}</button>
      <button type="button" role="tab" data-testid="reward-report-mode-orders" :aria-selected="mode === 'reward_orders'" @click="mode = 'reward_orders'">{{ t('admin.copy.3bc61630') }}</button>
    </div>
    <p class="definition">{{ mode === "rewards" ? t('admin.copy.a5481a8b') : t('admin.copy.13c75e69') }}</p>
    <div class="filters" data-testid="reward-report-filters">
      <label>{{ t('filter.fromDateUTC') }}<PlatformTemporalInput v-model="fromDraft" type="date" :locale="displayLocale" /></label>
      <label>{{ t('filter.toDateUTC') }}<PlatformTemporalInput v-model="toDraft" type="date" :locale="displayLocale" /></label>
      <label>{{ t('filter.member') }}<FilterAssociationSelect v-model="memberDraft" :brand-id="props.brandId" surface="admin" :enabled="permissions.view" resource="report_rewards" kind="member" :label="t('filter.member')" :locale="displayLocale" :scope-key="JSON.stringify(props.account)" /></label>
      <label>{{ t('filter.reward_order') }}<FilterAssociationSelect v-model="orderDraft" :brand-id="props.brandId" surface="admin" :enabled="permissions.view" resource="report_rewards" kind="reward_order" :label="t('filter.reward_order')" :locale="displayLocale" :scope-key="JSON.stringify(props.account)" /></label>
      <label>{{ t('admin.copy.6cc02c91') }}<select v-model="groupDraft" data-testid="reward-report-group"><option v-for="group in groups" :key="group" :value="group">{{ groupLabel(group) }}</option></select></label>
    <button class="button button-primary" type="button" data-testid="reward-report-query" :disabled="busy || !permissions.view" @click="withGlobalLoading(run)">{{ t('admin.copy.2bd50b3e') }}</button>
    </div>

    <p v-if="error" class="error" role="alert">{{ displayPublicError(error) }}</p><p v-if="exportError" class="error" role="alert">{{ displayPublicError(exportError) }}</p>
    <template v-if="result && committed">
      <div class="context"><span>{{ t('admin.copy.c79f8452') }} {{ committed.kind === 'rewards' ? t('admin.copy.15791d7e') : t('admin.copy.3bc61630') }}</span><span>{{ t('admin.copy.9c32661d') }} <AssociationName :record="result" kind="brand" :id="result.brand_id" :locale="associationLocale" /></span><span>{{ t('admin.copy.82d4e9bd') }} {{ displayDate(result.snapshot_at) }}</span><span>{{ t('admin.copy.c6bf9750') }} {{ result.timezone }}</span><span>{{ t('admin.copy.e4bcc7b2') }} {{ groupLabel(result.query.group_by) }}</span><span>{{ t('admin.copy.4f2fb666') }} [{{ displayDate(result.query.from) }}, {{ displayDate(result.query.to) }})</span><span>{{ t('admin.copy.56b2d19b') }}: <template v-if="result.query.member_id"><AssociationName :record="result.query" kind="member" :id="result.query.member_id" :locale="associationLocale" /></template><span v-else>{{ t('admin.copy.ef22c1d2') }}</span></span><span>{{ t('admin.copy.69dfdfc8') }}: {{ result.query.order_id ?? t('admin.copy.ef22c1d2') }}</span><span>{{ t('admin.copy.a0c04ae0') }} {{ exact(result.total_groups) }}</span></div>
      <div class="actions"><button type="button" data-testid="reward-report-export" :disabled="!permissions.view || !permissions.export || exportBusy" @click="withGlobalLoading(exportAll)">{{ exportBusy ? t('admin.copy.1c800a70') : t('admin.copy.800c75fc') }}</button><span v-if="!permissions.export">{{ t('admin.copy.02995907') }}</span></div>
      <div class="metrics" data-testid="reward-report-summary"><div v-for="field in fields" :key="field"><span>{{ metric(field) }}</span><strong>{{ count(result.summary, field) }}</strong></div></div>
      <div v-if="result.items.length" class="table-wrap"><table><thead><tr><th>{{ result.query.group_by === 'state' ? t('admin.copy.5d42f3a0') : t('admin.copy.b96f43c6') }}</th><th v-for="field in fields" :key="field">{{ metric(field) }}</th></tr></thead><tbody><tr v-for="item in result.items" :key="item.key"><td>{{ result.query.group_by === 'state' ? stateLabel(item.label) : associationGroupLabel(item, result.query.group_by, item.key, item.label, associationLocale) }}</td><td v-for="field in fields" :key="field">{{ count(item.totals, field) }}</td></tr></tbody></table></div>
      <p v-else class="definition empty-state">{{ t('admin.copy.473e3420') }}</p>
      <nav class="pages" :aria-label="t('admin.copy.b81942bf')"><span>{{ result.query.offset + 1 }}–{{ result.query.offset + result.items.length }} / {{ exact(result.total_groups) }}</span><button type="button" :disabled="busy || result.query.offset === 0" @click="withGlobalLoading(page, Math.max(0, result.query.offset - 20))">{{ t('admin.copy.dbe7a5d7') }}</button><button type="button" :disabled="busy || !hasNext(result)" @click="withGlobalLoading(page, result.query.offset + 20)">{{ t('admin.copy.2e9c9e7a') }}</button></nav>
    </template>
    <p v-global-loading v-else-if="busy" class="definition" aria-live="polite">{{ t('admin.copy.27342bf3') }}</p>
  </section>
</template>

<style scoped>
.reward-reports{display:grid;gap:12px;min-width:0;border:1px solid var(--line);border-radius:12px;padding:16px;background:var(--surface);color:var(--ink)}.reward-reports header,.actions,.pages{display:flex;align-items:center;justify-content:space-between;gap:10px;flex-wrap:wrap}.reward-reports h3,.reward-reports header p{margin:0}.reward-reports header p,.definition,.context,.actions span{color:var(--muted);font-size:.86rem;line-height:1.55}.modes{display:flex;gap:8px;flex-wrap:wrap}.modes button[aria-selected="true"]{border-color:var(--primary);background:var(--primary-light);color:var(--primary-dark)}.filters{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px}.filters label{display:grid;gap:5px;min-width:0;font-size:.86rem}.filters input,.filters select{box-sizing:border-box;width:100%;min-width:0;padding:9px;border:1px solid var(--line);border-radius:8px;font:inherit}.reward-reports button{min-height:40px;padding:8px 12px;border:1px solid #c5ceda;border-radius:8px;background:var(--surface);color:inherit;font:inherit}.reward-reports button:disabled{opacity:.5}.context{display:flex;flex-wrap:wrap;gap:4px 14px;overflow-wrap:anywhere}.metrics{display:grid;grid-template-columns:repeat(auto-fit,minmax(145px,1fr));gap:8px}.metrics div{display:grid;gap:5px;min-width:0;border:1px solid #e5eaf0;border-radius:8px;padding:10px}.metrics span{color:var(--muted);font-size:.8rem}.metrics strong{overflow-wrap:anywhere;font-variant-numeric:tabular-nums}.table-wrap{overflow:auto;border:1px solid #e2e7ef;border-radius:8px}table{border-collapse:collapse;min-width:900px;width:100%;font-variant-numeric:tabular-nums}th,td{padding:8px;border-bottom:1px solid var(--line-subtle);text-align:left;white-space:nowrap}th{position:sticky;top:0;background:var(--canvas)}.error{margin:0;padding:10px;border-radius:8px;background:#fff0ee;color:#a32e1a}@media(max-width:700px){.reward-reports{padding:12px}.filters{grid-template-columns:minmax(0,1fr)}.metrics{grid-template-columns:repeat(2,minmax(0,1fr))}.table-wrap{max-height:60vh}.actions button{width:100%}}
</style>
