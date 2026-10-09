<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { attributionReportPermissions, createAttributionReportApi, type AttributionGroupBy, type AttributionJoinMethod, type AttributionReport, type AttributionReportQuery, type AttributionTotals } from "./attribution-report-api";
import { useAdminI18n } from "./i18n";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, locale } = useAdminI18n();
const englishUi: Record<string, string> = {
  "归因报表": "Attribution report", "基于注单放置时保存的归因快照，只读运营统计。": "Read-only operational metrics from attribution snapshots saved when bets were placed.",
  "历史归因快照不会随当前代理关系或加入方式变化；此报表不是佣金结算依据或财务授权。": "Historical attribution snapshots do not change with current agent relationships or join methods. This report does not authorize commissions or financial actions.",
  "时间按设备本地时区输入并转换为 UTC；按天分组使用品牌时区。": "Enter dates in the device timezone; requests use UTC, and day groups use the brand timezone.",
  "开始时间": "Start time", "结束时间": "End time", "彩种 UUID（可选）": "Game UUID (optional)", "会员 UUID（可选）": "Member UUID (optional)", "代理 UUID（可选）": "Agent UUID (optional)",
  "代理范围": "Agent scope", "直属代理": "Direct agent", "下级代理": "Downline", "加入方式": "Join method", "全部加入方式": "All join methods", "按天": "By day", "按彩种": "By game", "按会员": "By member", "按代理": "By agent", "按加入方式": "By join method", "分组": "Group",
  "按代理分组始终使用快照中的直属代理；下级筛选后的分组仍保持互斥。": "Agent groups always use the saved direct agent. Groups remain disjoint when filtering a downline.",
  "查询": "Query", "导出完整 CSV": "Export complete CSV", "查询中…": "Loading…", "导出中…": "Exporting…", "快照": "Snapshot", "时区": "Timezone", "品牌": "Brand", "总计": "Summary", "分组记录": "Groups", "标签": "Label", "全部": "All", "历史代理未记录": "Legacy agent not recorded", "历史归因未记录": "Legacy attribution not recorded", "无代理": "No agent", "无加入方式": "No join method", "暂无分组": "No groups", "第": "Items", "项，共": "of", "组": "groups", "上一页": "Previous", "下一页": "Next",
  "注单数": "Bet orders", "投注积分": "Stake points", "已放置": "Placed", "已中奖": "Won", "已未中奖": "Lost", "异常": "Abnormal", "已取消": "Cancelled", "退款积分": "Refund points", "已结算投注": "Settled stakes", "未结算投注": "Unsettled stakes", "异常投注": "Abnormal stakes", "当前最终代次奖金": "Current final-generation prizes", "未完成更正数": "Open corrections", "最终未中奖投注": "Final lost stakes", "历史归因未记录数": "Legacy attribution count",
  "读取失败，请重试。": "Read failed. Please try again.", "需要 report_attribution.view.brand 或 report_attribution.view.platform 查看权限。": "Requires report_attribution.view.brand or report_attribution.view.platform permission.", "需要 report_attribution.export.brand 或 report_attribution.export.platform 导出权限。": "Requires report_attribution.export.brand or report_attribution.export.platform permission.",
  "下级范围需要有效代理 UUID。": "Downline scope requires a valid agent UUID.", "请选择有效时间范围。": "Choose a valid time range.", "结束时间必须晚于开始时间。": "The end must be later than the start.", "时间范围不能超过 93 天。": "The time range cannot exceed 93 days.", "UUID 格式无效。": "Invalid UUID format.",
  domain: "Domain", operator: "Operator", agent_code: "Agent code", referral_code: "Referral code", legacy: "Legacy",
};
const ui = (value: string) => t(value, englishUi[value] ?? value);
const api = createAttributionReportApi();
const permissions = computed(() => attributionReportPermissions(props.account, props.brandId));
const groupBy = ref<AttributionGroupBy>("day");
const agentScope = ref<"direct" | "downline">("direct");
const joinMethod = ref<AttributionJoinMethod | "">("");
const fromDraft = ref(""); const toDraft = ref(""); const gameDraft = ref(""); const memberDraft = ref(""); const agentDraft = ref("");
const result = ref<AttributionReport | null>(null); const error = ref(""); const exportError = ref("");
const busy = ref(false); const exportBusy = ref(false); const offset = ref(0);
const committed = ref<{ query: AttributionReportQuery; offset: number } | null>(null);
let ticket = 0, exportTicket = 0, alive = true;
const scopeKey = computed(() => JSON.stringify({ brand: props.brandId, account: props.account.id, super: props.account.super_admin, brands: [...(props.account.brand_ids ?? [])].sort(), permissions: [...(props.account.permissions ?? [])].sort(), platform: [...(props.account.platform_permissions ?? [])].sort(), byBrand: props.account.permissions_by_brand?.[props.brandId] ? [...props.account.permissions_by_brand[props.brandId]!].sort() : null, allowed: permissions.value }));
let committedScope = scopeKey.value;

function localValue(date: Date) { const pad = (n: number) => String(n).padStart(2, "0"); return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`; }
function preset24h() { const to = new Date(); const from = new Date(to.getTime() - 24 * 60 * 60 * 1000); from.setMilliseconds(0); to.setMilliseconds(0); fromDraft.value = localValue(from); toDraft.value = localValue(to); }
function toUtc(input: string): string | null {
  if (!input) return null;
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/.exec(input);
  if (!match) return null;
  const [, year, month, day, hour, minute, second = "0"] = match;
  const y = Number(year), m = Number(month), d0 = Number(day), h = Number(hour), min = Number(minute), sec = Number(second);
  const days = m === 2 ? (y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0) ? 29 : 28) : ([4, 6, 9, 11].includes(m) ? 30 : 31);
  if (y < 1 || m < 1 || m > 12 || d0 < 1 || d0 > days || h > 23 || min > 59 || sec > 59) return null;
  const date = new Date(input);
  return Number.isFinite(date.getTime()) ? date.toISOString() : null;
}
function uuidOrEmpty(input: string) { const value = input.trim(); if (value && !/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value)) throw new Error("UUID 格式无效。"); return value || null; }
function makeQuery(): AttributionReportQuery {
  const from = toUtc(fromDraft.value), to = toUtc(toDraft.value);
  if (!from || !to) throw new Error("请选择有效时间范围。");
  const start = Date.parse(from), end = Date.parse(to);
  if (end <= start) throw new Error("结束时间必须晚于开始时间。");
  if (end - start > 93 * 24 * 60 * 60 * 1000) throw new Error("时间范围不能超过 93 天。");
  const agentId = uuidOrEmpty(agentDraft.value);
  if (agentScope.value === "downline" && !agentId) throw new Error("下级范围需要有效代理 UUID。");
  return { from, to, group_by: groupBy.value, limit: 20, offset: 0, game_id: uuidOrEmpty(gameDraft.value), member_id: uuidOrEmpty(memberDraft.value), agent_id: agentId, agent_scope: agentScope.value, join_method: joinMethod.value || null };
}
function current(id: number, scope: string) { return alive && id === ticket && scope === scopeKey.value && scope === committedScope; }
function clear() { result.value = null; error.value = ""; exportError.value = ""; offset.value = 0; }
function errorMessage(cause: unknown) { return cause instanceof AdminApiError || cause instanceof Error ? cause.message : "读取失败，请重试。"; }
async function load(query: AttributionReportQuery, pageOffset: number) {
  const scope = scopeKey.value, id = ++ticket; committedScope = scope; clear();
  committed.value = { query: { ...query, offset: pageOffset }, offset: pageOffset }; busy.value = true;
  try { const next = await api.report(props.brandId, { ...query, offset: pageOffset }); if (current(id, scope)) { result.value = next; offset.value = pageOffset; } }
  catch (cause) { if (current(id, scope)) { error.value = errorMessage(cause); if (cause instanceof AdminApiError && cause.status === 401) { ticket++; committed.value = null; busy.value = false; emit("session-invalid"); } } }
  finally { if (current(id, scope)) busy.value = false; }
}
async function queryReport() { try { await load(makeQuery(), 0); } catch (cause) { ticket++; committed.value = null; clear(); error.value = errorMessage(cause); busy.value = false; } }
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
function formatDate(value: string) { const d = new Date(value); return Number.isFinite(d.getTime()) ? d.toLocaleString(locale.value === "en" ? "en-US" : "zh-CN") : value; }
function metrics(totals: AttributionTotals) { return [["注单数", totals.order_count], ["投注积分", totals.stake_points], ["已放置", totals.placed_count], ["已中奖", totals.won_count], ["已未中奖", totals.lost_count], ["异常", totals.abnormal_count], ["已取消", totals.cancelled_count], ["退款积分", totals.refund_points], ["已结算投注", totals.settled_stake_points], ["未结算投注", totals.unfinalized_stake_points], ["异常投注", totals.abnormal_stake_points], ["当前最终代次奖金", totals.current_prize_points], ["未完成更正数", totals.correction_open_count], ["最终未中奖投注", totals.final_lost_stake_points], ["历史归因未记录数", totals.legacy_attribution_count]] as const; }
function hasNext() { return !!result.value && offset.value + 20 <= 1_000_000 && BigInt(offset.value) + BigInt(result.value.items.length) < BigInt(result.value.total_groups); }
function displayGroup(item: AttributionReport["items"][number]) {
  if (result.value?.query.group_by === "agent") {
    if (item.key === "none") return ui("无代理");
    if (item.key === "legacy") return ui("历史代理未记录");
  }
  if (result.value?.query.group_by === "join_method" && item.key === "legacy") return ui("历史归因未记录");
  return item.label;
}
watch(scopeKey, (next) => { ticket++; exportTicket++; committedScope = next; committed.value = null; clear(); busy.value = false; exportBusy.value = false; }, { flush: "sync" });
onBeforeUnmount(() => { alive = false; ticket++; exportTicket++; });
// Prefill a useful range without making a request; results only appear after an explicit query.
preset24h();
</script>

<template>
  <section v-if="permissions.view" class="attribution-report" aria-labelledby="attribution-report-title">
    <header><div><h3 id="attribution-report-title">{{ ui("归因报表") }}</h3><p>{{ ui("基于注单放置时保存的归因快照，只读运营统计。") }}</p></div><button class="ar-button" type="button" :disabled="busy || exportBusy" data-testid="attribution-query" @click="queryReport">{{ ui("查询") }}</button></header>
    <p class="ar-definition">{{ ui("历史归因快照不会随当前代理关系或加入方式变化；此报表不是佣金结算依据或财务授权。") }}</p>
    <p class="ar-note">{{ ui("时间按设备本地时区输入并转换为 UTC；按天分组使用品牌时区。") }}</p>
    <div class="ar-filters">
      <label>{{ ui("开始时间") }}<input v-model="fromDraft" type="datetime-local" step="1"></label>
      <label>{{ ui("结束时间") }}<input v-model="toDraft" type="datetime-local" step="1"></label>
      <label>{{ ui("彩种 UUID（可选）") }}<input v-model.trim="gameDraft" autocomplete="off"></label>
      <label>{{ ui("会员 UUID（可选）") }}<input v-model.trim="memberDraft" autocomplete="off"></label>
      <label>{{ ui("代理 UUID（可选）") }}<input v-model.trim="agentDraft" autocomplete="off"></label>
      <label>{{ ui("代理范围") }}<select v-model="agentScope" :aria-label="ui('代理范围')" data-testid="attribution-agent-scope"><option value="direct">{{ ui("直属代理") }}</option><option value="downline">{{ ui("下级代理") }}</option></select></label>
      <label>{{ ui("分组") }}<select v-model="groupBy" :aria-label="ui('分组')" data-testid="attribution-group"><option value="day">{{ ui("按天") }}</option><option value="game">{{ ui("按彩种") }}</option><option value="member">{{ ui("按会员") }}</option><option value="agent">{{ ui("按代理") }}</option><option value="join_method">{{ ui("按加入方式") }}</option></select></label>
      <label>{{ ui("加入方式") }}<select v-model="joinMethod" :aria-label="ui('加入方式')" data-testid="attribution-join-method"><option value="">{{ ui("全部加入方式") }}</option><option value="domain">{{ ui("domain") }}</option><option value="operator">{{ ui("operator") }}</option><option value="agent_code">{{ ui("agent_code") }}</option><option value="referral_code">{{ ui("referral_code") }}</option><option value="legacy">{{ ui("legacy") }}</option></select></label>
    </div>
    <p class="ar-note">{{ ui("按代理分组始终使用快照中的直属代理；下级筛选后的分组仍保持互斥。") }}</p>
    <p v-if="props.account.super_admin" class="ar-note">{{ ui("需要 report_attribution.view.brand 或 report_attribution.view.platform 查看权限。") }}</p>
    <p v-if="error" class="ar-error" role="alert">{{ ui(error) }}</p>
    <div v-if="result" class="ar-result" data-testid="attribution-result">
      <div class="ar-heading"><div class="ar-context"><span>{{ ui("品牌") }} {{ result.brand_id }}</span><span>{{ ui("快照") }} {{ formatDate(result.snapshot_at) }}</span><span>{{ ui("时区") }} {{ result.timezone }}</span><span>[{{ formatDate(result.query.from) }}, {{ formatDate(result.query.to) }})</span><span>{{ ui("分组") }} {{ ui(result.query.group_by === "day" ? "按天" : result.query.group_by === "game" ? "按彩种" : result.query.group_by === "member" ? "按会员" : result.query.group_by === "agent" ? "按代理" : "按加入方式") }}</span></div>
        <button class="ar-button" type="button" data-testid="attribution-export" :disabled="!permissions.export || exportBusy" @click="exportReport">{{ ui(exportBusy ? "导出中…" : "导出完整 CSV") }}</button></div>
      <p v-if="!permissions.export" class="ar-note">{{ ui("需要 report_attribution.export.brand 或 report_attribution.export.platform 导出权限。") }}</p>
      <p v-if="exportError" class="ar-error" role="alert">{{ ui(exportError) }}</p>
      <h4>{{ ui("总计") }}</h4><div class="ar-metrics" data-testid="attribution-summary"><div v-for="metric in metrics(result.summary)" :key="metric[0]"><span>{{ ui(metric[0]) }}</span><strong>{{ formatInteger(metric[1]) }}</strong></div></div>
      <h4>{{ ui("分组记录") }}</h4>
      <div v-if="result.items.length" class="ar-table-wrap"><table><thead><tr><th>{{ ui("分组") }}</th><th>{{ ui("标签") }}</th><th v-for="metric in metrics(result.items[0]!.totals)" :key="metric[0]">{{ ui(metric[0]) }}</th></tr></thead><tbody><tr v-for="item in result.items" :key="item.key"><td>{{ item.key }}</td><td>{{ displayGroup(item) }}</td><td v-for="metric in metrics(item.totals)" :key="metric[0]">{{ formatInteger(metric[1]) }}</td></tr></tbody></table></div>
      <div v-if="result.items.length" class="ar-cards"><article v-for="item in result.items" :key="item.key"><h5>{{ displayGroup(item) }}</h5><p>{{ item.key }}</p><dl><div v-for="metric in metrics(item.totals)" :key="metric[0]"><dt>{{ ui(metric[0]) }}</dt><dd>{{ formatInteger(metric[1]) }}</dd></div></dl></article></div>
      <p v-if="!result.items.length" class="ar-note">{{ ui("暂无分组") }}</p>
      <nav class="ar-pagination"><span>{{ ui("第") }} {{ result.items.length ? offset + 1 : 0 }}–{{ result.items.length ? offset + result.items.length : 0 }} {{ ui("项，共") }} {{ formatInteger(result.total_groups) }} {{ ui("组") }}</span><button class="ar-button" type="button" :disabled="offset === 0 || busy" @click="page(-1)">{{ ui("上一页") }}</button><button class="ar-button" type="button" :disabled="!hasNext() || busy" @click="page(1)">{{ ui("下一页") }}</button></nav>
    </div>
    <p v-else-if="busy" class="ar-note" role="status">{{ ui("查询中…") }}</p>
  </section>
</template>

<style scoped>
.attribution-report{display:grid;gap:12px;min-width:0;border:1px solid #d9e0ea;border-radius:12px;padding:16px;background:#fff;color:#172033}.attribution-report header,.ar-heading,.ar-pagination{display:flex;align-items:center;justify-content:space-between;gap:12px}.attribution-report h3{margin:0}.attribution-report header p,.ar-note,.ar-definition{margin:4px 0 0;color:#667085;font-size:.88rem}.ar-definition{line-height:1.55}.ar-filters{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:10px}.ar-filters label{display:grid;gap:5px;min-width:0;font-size:.84rem}.ar-filters input,.ar-filters select{box-sizing:border-box;width:100%;min-width:0;border:1px solid #cbd3df;border-radius:8px;padding:9px;background:#fff;font:inherit}.ar-button{min-height:38px;border:1px solid #c5ceda;border-radius:8px;padding:7px 11px;background:#fff;color:inherit;font:inherit;cursor:pointer}.ar-button:disabled{opacity:.5;cursor:not-allowed}.ar-result{display:grid;gap:10px}.ar-result h4{margin:4px 0 0}.ar-context{display:flex;flex-wrap:wrap;gap:5px 13px;color:#475467;font-size:.82rem;overflow-wrap:anywhere}.ar-metrics{display:grid;grid-template-columns:repeat(auto-fit,minmax(130px,1fr));gap:7px}.ar-metrics>div{display:grid;gap:4px;min-width:0;border:1px solid #e5eaf0;border-radius:8px;padding:9px}.ar-metrics span{color:#667085;font-size:.78rem}.ar-metrics strong{overflow-wrap:anywhere;font-variant-numeric:tabular-nums}.ar-table-wrap{overflow:auto;border:1px solid #e2e7ef;border-radius:8px}.ar-table-wrap table{width:100%;min-width:1050px;border-collapse:collapse;font-variant-numeric:tabular-nums}.ar-table-wrap th,.ar-table-wrap td{padding:8px;border-bottom:1px solid #edf0f4;text-align:left;white-space:nowrap}.ar-table-wrap th{background:#f7f9fc;color:#475467;font-size:.8rem}.ar-cards{display:none}.ar-cards article{border:1px solid #e2e7ef;border-radius:8px;padding:10px}.ar-cards h5{margin:0}.ar-cards p{color:#667085;overflow-wrap:anywhere;font-size:.8rem}.ar-cards dl{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:7px;margin:8px 0}.ar-cards dl div{min-width:0}.ar-cards dt{color:#667085;font-size:.75rem}.ar-cards dd{margin:3px 0 0;overflow-wrap:anywhere;font-variant-numeric:tabular-nums}.ar-pagination{justify-content:flex-end;flex-wrap:wrap}.ar-pagination span{margin-right:auto;color:#667085;font-size:.84rem}.ar-error{margin:0;padding:9px 11px;border-radius:8px;background:#fff0ee;color:#a32e1a}
@media(max-width:760px){.attribution-report{padding:12px}.attribution-report header,.ar-heading{align-items:flex-start}.ar-filters{grid-template-columns:minmax(0,1fr)}.ar-table-wrap{display:none}.ar-cards{display:grid;gap:8px}.ar-pagination{justify-content:center}.ar-pagination span{flex-basis:100%;text-align:center}.ar-pagination .ar-button{flex:1}.ar-context{display:grid;gap:4px}.ar-metrics{grid-template-columns:repeat(2,minmax(0,1fr))}}
</style>
