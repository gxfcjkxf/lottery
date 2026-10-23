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
import { attributionReportPermissions, createAttributionReportApi, type AttributionGroupBy, type AttributionJoinMethod, type AttributionReport, type AttributionReportQuery, type AttributionTotals } from "./attribution-report-api";
import { adminKey, useAdminI18n } from "./i18n";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, display, locale } = useAdminI18n();
const ui = (key: string) => t(adminKey(key));
const api = createAttributionReportApi();
const permissions = computed(() => attributionReportPermissions(props.account, props.brandId));
const groupBy = ref<AttributionGroupBy>("day");
const agentScope = ref<"direct" | "downline">("direct");
const joinMethod = ref<AttributionJoinMethod | "">("");
const fromDraft = ref(""); const toDraft = ref(""); const gameDraft = ref(""); const memberDraft = ref(""); const agentDraft = ref("");
const result = ref<AttributionReport | null>(null); const error = ref<string | import("@lottery/shared").LocalizedMessage>(""); const exportError = ref<string | import("@lottery/shared").LocalizedMessage>("");
const busy = ref(false); const exportBusy = ref(false); const offset = ref(0);
const committed = ref<{ query: AttributionReportQuery; offset: number } | null>(null);
let ticket = 0, exportTicket = 0, alive = true;
const scopeKey = computed(() => JSON.stringify({ brand: props.brandId, account: props.account.id, super: props.account.super_admin, brands: [...(props.account.brand_ids ?? [])].sort(), permissions: [...(props.account.permissions ?? [])].sort(), platform: [...(props.account.platform_permissions ?? [])].sort(), byBrand: props.account.permissions_by_brand?.[props.brandId] ? [...props.account.permissions_by_brand[props.brandId]!].sort() : null, allowed: permissions.value }));
let committedScope = scopeKey.value;

function localValue(date: Date) { const pad = (n: number) => String(n).padStart(2, "0"); return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`; }
function presetToday() { const to = new Date(); const from = new Date(to.getTime()); from.setMilliseconds(0); to.setMilliseconds(0); fromDraft.value = localValue(from).slice(0,10); toDraft.value = localValue(to).slice(0,10); }
function uuidOrEmpty(input: string) { const value = input.trim(); if (value && !/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value)) throw new Error("UUID 格式无效。"); return value || null; }
async function makeQuery(): Promise<AttributionReportQuery> {
  const from = filterDateBoundary(fromDraft.value, false, 'local').toISOString(), to = filterDateBoundary(toDraft.value, true, 'local').toISOString();
  if (!from || !to) throw new Error("请选择有效时间范围。");
  const start = Date.parse(from), end = Date.parse(to);
  if (end <= start) throw new Error("结束时间必须晚于开始时间。");
  if (end - start > 93 * 24 * 60 * 60 * 1000) throw new Error("时间范围不能超过 93 天。");
  const agentId = uuidOrEmpty(agentDraft.value);
  if (agentScope.value === "downline" && !agentId) throw new Error("下级范围需要有效代理 UUID。");
  const username = memberDraft.value.trim();
  const member = username ? { id: username } : null;
  return { from, to, group_by: groupBy.value, limit: 20, offset: 0, game_id: uuidOrEmpty(gameDraft.value), member_id: member?.id ?? null, agent_id: agentId, agent_scope: agentScope.value, join_method: joinMethod.value || null };
}
function current(id: number, scope: string) { return alive && id === ticket && scope === scopeKey.value && scope === committedScope; }
function clear() { result.value = null; error.value = ""; exportError.value = ""; offset.value = 0; }
function errorMessage(cause: unknown) { return cause instanceof AdminApiError || cause instanceof Error ? publicErrorMessage(cause, 'read') : "读取失败，请重试。"; }
async function load(query: AttributionReportQuery, pageOffset: number) {
  const scope = scopeKey.value, id = ++ticket; committedScope = scope; clear();
  committed.value = { query: { ...query, offset: pageOffset }, offset: pageOffset }; busy.value = true;
  try { const next = await api.report(props.brandId, { ...query, offset: pageOffset }); if (current(id, scope)) { result.value = next; offset.value = pageOffset; } }
  catch (cause) { if (current(id, scope)) { error.value = errorMessage(cause); if (cause instanceof AdminApiError && cause.status === 401) { ticket++; committed.value = null; busy.value = false; emit("session-invalid"); } } }
  finally { if (current(id, scope)) busy.value = false; }
}
async function queryReport() { try { await load(await makeQuery(), 0); } catch (cause) { ticket++; committed.value = null; clear(); error.value = errorMessage(cause); busy.value = false; } }
async function page(delta: -1 | 1) {
  if (!committed.value) return;
  const next = offset.value + delta * 20;
  if (!Number.isSafeInteger(next) || next < 0 || next > 1_000_000) return;
  await load(committed.value.query, next);
}
async function exportReport() {
  const saved = committed.value; if (!saved || !permissions.value.export || !alive) return;
  const scope = scopeKey.value, id = ++exportTicket; exportError.value = ""; exportBusy.value = true;
  try {
    const query = saved.query;
    const file = await api.exportCsv(props.brandId, { from: query.from, to: query.to, group_by: query.group_by, game_id: query.game_id, member_id: query.member_id, agent_id: query.agent_id, agent_scope: query.agent_scope, join_method: query.join_method }); if (!alive || id !== exportTicket || scope !== scopeKey.value || scope !== committedScope || committed.value !== saved) return;
    const url = URL.createObjectURL(new Blob([file.bytes], { type: "text/csv;charset=utf-8" })), anchor = document.createElement("a"); anchor.href = url; anchor.download = file.filename; anchor.click(); setTimeout(() => URL.revokeObjectURL(url), 0);
  } catch (cause) {
    if (!alive || id !== exportTicket || scope !== scopeKey.value || committed.value !== saved) return;
    if (cause instanceof AdminApiError && cause.status === 401) { ticket++; committed.value = null; clear(); emit("session-invalid"); }
    exportError.value = errorMessage(cause);
  } finally { if (id === exportTicket) exportBusy.value = false; }
}
function formatInteger(value: string) { try { const n = BigInt(value); return n.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ","); } catch { return value; } }
function formatDate(value: string){ return formatDisplayDateTime(value, displayLocale.value); }
function metrics(totals: AttributionTotals) { return [["注单数", totals.order_count], ["投注积分", totals.stake_points], ["已放置", totals.placed_count], ["已中奖", totals.won_count], ["已未中奖", totals.lost_count], ["异常", totals.abnormal_count], ["已取消", totals.cancelled_count], ["退款积分", totals.refund_points], ["已结算投注", totals.settled_stake_points], ["未结算投注", totals.unfinalized_stake_points], ["异常投注", totals.abnormal_stake_points], ["当前最终代次奖金", totals.current_prize_points], ["未完成更正数", totals.correction_open_count], ["最终未中奖投注", totals.final_lost_stake_points]] as const; }
const joinMethodLabels: Record<string, string> = { domain: 'admin.attribution.join.domain', operator: 'admin.attribution.join.operator', agent_code: 'admin.attribution.join.agentCode', referral_code: 'admin.attribution.join.referralCode' };
const groupLabels: Record<string, string> = { day: 'admin.copy.f91541ed', game: 'admin.copy.ef04a723', member: 'admin.copy.93a783ef', agent: 'admin.copy.835c8ca7', join_method: 'admin.copy.9a96afd1' };
const metricLabels: Record<string, string> = { '注单数': 'admin.copy.57a81d6e', '投注积分': 'admin.copy.33ae8fea', '已放置': 'admin.copy.96f50da6', '已中奖': 'admin.copy.6de62356', '已未中奖': 'admin.copy.1a44472c', '异常': 'admin.copy.242b6072', '已取消': 'admin.copy.c320e9ec', '退款积分': 'admin.copy.32dff627', '已结算投注': 'admin.copy.fa05625e', '未结算投注': 'admin.copy.4a69124d', '异常投注': 'admin.copy.9c70a235', '当前最终代次奖金': 'admin.copy.006c120b', '未完成更正数': 'admin.copy.c4cdafbd', '最终未中奖投注': 'admin.copy.9129589a' };
const metricLabel = (value: string) => t(adminKey(metricLabels[value] ?? value));
function hasNext() { return !!result.value && offset.value + 20 <= 1_000_000 && BigInt(offset.value) + BigInt(result.value.items.length) < BigInt(result.value.total_groups); }
function displayGroup(item: AttributionReport["items"][number]) {
  if (result.value?.query.group_by === "agent") {
    if (item.key === "none") return t('admin.copy.a4faa770');
  }
  return associationGroupLabel(item, result.value!.query.group_by, item.key, item.label, associationLocale.value);
}
watch(scopeKey, (next) => { ticket++; exportTicket++; committedScope = next; committed.value = null; clear(); busy.value = false; exportBusy.value = false; }, { flush: "sync" });
onBeforeUnmount(() => { alive = false; ticket++; exportTicket++; });
// Prefill a useful range without making a request; results only appear after an explicit query.
presetToday();
</script>

<template>
  <section v-if="permissions.view" class="attribution-report" aria-labelledby="attribution-report-title">
    <header><div><h3 id="attribution-report-title">{{ t('admin.copy.254d6bbb') }}</h3></div></header>
    <p class="ar-definition">{{ t('admin.copy.547cd806') }}</p>
    <div class="ar-filters">
      <label>{{ t('filter.fromDate') }}<PlatformTemporalInput v-model="fromDraft" type="date" :locale="displayLocale" /></label>
      <label>{{ t('filter.toDate') }}<PlatformTemporalInput v-model="toDraft" type="date" :locale="displayLocale" /></label>
      <label>{{ t('filter.game') }}<FilterAssociationSelect v-model="gameDraft" :brand-id="props.brandId" surface="admin" :enabled="permissions.view" resource="report_attribution" kind="game" :label="t('filter.game')" :locale="displayLocale" :scope-key="JSON.stringify(props.account)" /></label>
      <label>{{ t('filter.member') }}<FilterAssociationSelect v-model="memberDraft" :brand-id="props.brandId" surface="admin" :enabled="permissions.view" resource="report_attribution" kind="member" :label="t('filter.member')" :locale="displayLocale" :scope-key="JSON.stringify(props.account)" /></label>
      <label>{{ t('filter.agent') }}<FilterAssociationSelect v-model="agentDraft" :brand-id="props.brandId" surface="admin" :enabled="permissions.view" resource="report_attribution" kind="agent" :label="t('filter.agent')" :locale="displayLocale" :scope-key="JSON.stringify(props.account)" /></label>
      <label>{{ t('admin.copy.bc34386a') }}<select v-model="agentScope" :aria-label="t('admin.copy.bc34386a')" data-testid="attribution-agent-scope"><option value="direct">{{ t('admin.copy.36d246ea') }}</option><option value="downline">{{ t('admin.copy.0a3260d0') }}</option></select></label>
      <label>{{ t('admin.copy.60ecfb6c') }}<select v-model="groupBy" :aria-label="t('admin.copy.60ecfb6c')" data-testid="attribution-group"><option value="day">{{ t('admin.copy.f91541ed') }}</option><option value="game">{{ t('admin.copy.ef04a723') }}</option><option value="member">{{ t('admin.copy.93a783ef') }}</option><option value="agent">{{ t('admin.copy.835c8ca7') }}</option><option value="join_method">{{ t('admin.copy.9a96afd1') }}</option></select></label>
      <label>{{ t('admin.copy.93fbd29f') }}<select v-model="joinMethod" :aria-label="t('admin.copy.93fbd29f')" data-testid="attribution-join-method"><option value="">{{ t('admin.copy.1155b6f5') }}</option><option v-for="(key, value) in joinMethodLabels" :key="value" :value="value">{{ t(adminKey(key)) }}</option></select></label>
    <button class="ar-button button button-primary" type="button" :disabled="busy || exportBusy" data-testid="attribution-query" @click="withGlobalLoading(queryReport)">{{ t('admin.copy.62a97c9a') }}</button>
    </div>
    <p class="ar-note">{{ t('admin.copy.6e01de88') }}</p>
    <p v-if="props.account.super_admin" class="ar-note">{{ t('admin.copy.cbe29255') }}</p>
    <p v-if="error" class="ar-error" role="alert">{{ display(error) }}</p>
    <div v-if="result" class="ar-result" data-testid="attribution-result">
      <div class="ar-heading"><div class="ar-context"><span>{{ t('admin.copy.f676f47d') }} <AssociationName :record="result" kind="brand" :id="result.brand_id" :locale="associationLocale" /></span><span>{{ t('admin.copy.82d4e9bd') }} {{ formatDate(result.snapshot_at) }}</span><span>{{ t('admin.copy.f257539c') }} {{ result.timezone }}</span><span>[{{ formatDate(result.query.from) }}, {{ formatDate(result.query.to) }})</span><span>{{ t('admin.copy.60ecfb6c') }} {{ t(adminKey(groupLabels[result.query.group_by]!)) }}</span></div>
        <button class="ar-button" type="button" data-testid="attribution-export" :disabled="!permissions.export || exportBusy" @click="withGlobalLoading(exportReport)">{{ exportBusy ? t('admin.copy.1c800a70') : t('admin.copy.3aa35886') }}</button></div>
      <p v-if="!permissions.export" class="ar-note">{{ t('admin.copy.472a26d9') }}</p>
      <p v-if="exportError" class="ar-error" role="alert">{{ display(exportError) }}</p>
      <h4>{{ t('admin.copy.315a448d') }}</h4><div class="ar-metrics" data-testid="attribution-summary"><div v-for="metric in metrics(result.summary)" :key="metric[0]"><span>{{ metricLabel(metric[0]) }}</span><strong>{{ formatInteger(metric[1]) }}</strong></div></div>
      <h4>{{ t('admin.copy.1b73b990') }}</h4>
      <div v-if="result.items.length" class="ar-table-wrap"><table><thead><tr><th>{{ t('admin.copy.b96f43c6') }}</th><th v-for="metric in metrics(result.items[0]!.totals)" :key="metric[0]">{{ metricLabel(metric[0]) }}</th></tr></thead><tbody><tr v-for="item in result.items" :key="item.key"><td>{{ displayGroup(item) }}</td><td v-for="metric in metrics(item.totals)" :key="metric[0]">{{ formatInteger(metric[1]) }}</td></tr></tbody></table></div>
      <div v-if="result.items.length" class="ar-cards"><article v-for="item in result.items" :key="item.key"><h5>{{ displayGroup(item) }}</h5><dl><div v-for="metric in metrics(item.totals)" :key="metric[0]"><dt>{{ metricLabel(metric[0]) }}</dt><dd>{{ formatInteger(metric[1]) }}</dd></div></dl></article></div>
      <p v-if="!result.items.length" class="ar-note empty-state">{{ t('admin.copy.8e0ad65c') }}</p>
      <nav class="ar-pagination"><span>{{ t('admin.copy.d116a312') }} {{ result.items.length ? offset + 1 : 0 }}–{{ result.items.length ? offset + result.items.length : 0 }} {{ t('admin.copy.32904ee4') }} {{ formatInteger(result.total_groups) }} {{ t('admin.copy.e2daf069') }}</span><button class="ar-button" type="button" :disabled="offset === 0 || busy" @click="withGlobalLoading(page, -1)">{{ t('admin.copy.dbe7a5d7') }}</button><button class="ar-button" type="button" :disabled="!hasNext() || busy" @click="withGlobalLoading(page, 1)">{{ t('admin.copy.ee97c955') }}</button></nav>
    </div>
    <p v-global-loading v-else-if="busy" class="ar-note" role="status">{{ t('admin.copy.dea7c355') }}</p>
  </section>
</template>

<style scoped>
.attribution-report{display:grid;gap:12px;min-width:0;border:1px solid var(--line);border-radius:12px;padding:16px;background:var(--surface);color:var(--ink)}.attribution-report header,.ar-heading,.ar-pagination{display:flex;align-items:center;justify-content:space-between;gap:12px}.attribution-report h3{margin:0}.attribution-report header p,.ar-note,.ar-definition{margin:4px 0 0;color:var(--muted);font-size:.88rem}.ar-definition{line-height:1.55}.ar-filters{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:10px}.ar-filters label{display:grid;gap:5px;min-width:0;font-size:.84rem}.ar-filters input,.ar-filters select{box-sizing:border-box;width:100%;min-width:0;border:1px solid var(--line);border-radius:8px;padding:9px;background:var(--surface);font:inherit}.ar-button{min-height:38px;border:1px solid #c5ceda;border-radius:8px;padding:7px 11px;background:var(--surface);color:inherit;font:inherit;cursor:pointer}.ar-button:disabled{opacity:.5;cursor:not-allowed}.ar-result{display:grid;gap:10px}.ar-result h4{margin:4px 0 0}.ar-context{display:flex;flex-wrap:wrap;gap:5px 13px;color:#475467;font-size:.82rem;overflow-wrap:anywhere}.ar-metrics{display:grid;grid-template-columns:repeat(auto-fit,minmax(130px,1fr));gap:7px}.ar-metrics>div{display:grid;gap:4px;min-width:0;border:1px solid #e5eaf0;border-radius:8px;padding:9px}.ar-metrics span{color:var(--muted);font-size:.78rem}.ar-metrics strong{overflow-wrap:anywhere;font-variant-numeric:tabular-nums}.ar-table-wrap{overflow:auto;border:1px solid #e2e7ef;border-radius:8px}.ar-table-wrap table{width:100%;min-width:1050px;border-collapse:collapse;font-variant-numeric:tabular-nums}.ar-table-wrap th,.ar-table-wrap td{padding:8px;border-bottom:1px solid var(--line-subtle);text-align:left;white-space:nowrap}.ar-table-wrap th{background:var(--canvas);color:#475467;font-size:.8rem}.ar-cards{display:none}.ar-cards article{border:1px solid #e2e7ef;border-radius:8px;padding:10px}.ar-cards h5{margin:0}.ar-cards p{color:var(--muted);overflow-wrap:anywhere;font-size:.8rem}.ar-cards dl{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:7px;margin:8px 0}.ar-cards dl div{min-width:0}.ar-cards dt{color:var(--muted);font-size:.75rem}.ar-cards dd{margin:3px 0 0;overflow-wrap:anywhere;font-variant-numeric:tabular-nums}.ar-pagination{justify-content:flex-end;flex-wrap:wrap}.ar-pagination span{margin-right:auto;color:var(--muted);font-size:.84rem}.ar-error{margin:0;padding:9px 11px;border-radius:8px;background:#fff0ee;color:#a32e1a}
@media(max-width:760px){.attribution-report{padding:12px}.attribution-report header,.ar-heading{align-items:flex-start}.ar-filters{grid-template-columns:minmax(0,1fr)}.ar-table-wrap{display:none}.ar-cards{display:grid;gap:8px}.ar-pagination{justify-content:center}.ar-pagination span{flex-basis:100%;text-align:center}.ar-pagination .ar-button{flex:1}.ar-context{display:grid;gap:4px}.ar-metrics{grid-template-columns:repeat(2,minmax(0,1fr))}}
</style>
