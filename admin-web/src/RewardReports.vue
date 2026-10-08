<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { useAdminI18n } from "./i18n";
import { createRewardReportApi, rewardReportPermissions, type RewardReport, type RewardReportKind, type RewardReportQuery } from "./reward-report-api";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t } = useAdminI18n(), api = createRewardReportApi();
const permissions = computed(() => rewardReportPermissions(props.account, props.brandId));
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const utcInput = (date: Date) => `${date.toISOString().slice(0, 19)}`;
const end = new Date(Math.ceil(Date.now() / 60_000) * 60_000), start = new Date(end.getTime() - 7 * 86_400_000);
const fromDraft = ref(utcInput(start)), toDraft = ref(utcInput(end)), memberDraft = ref(""), orderDraft = ref("");
const mode = ref<RewardReportKind>("rewards"), groupDraft = ref<RewardReportQuery["group_by"]>("day");
const committed = ref<{ kind: RewardReportKind; query: RewardReportQuery } | null>(null), result = ref<RewardReport | null>(null);
const error = ref(""), exportError = ref(""), busy = ref(false), exportBusy = ref(false);
let readGeneration = 0, exportGeneration = 0, alive = true, readController: AbortController | undefined, exportController: AbortController | undefined;
const scopeKey = computed(() => JSON.stringify({ brand: props.brandId, actor: props.account.id, super: props.account.super_admin, brands: [...(props.account.brand_ids ?? [])].sort(), permissions: [...(props.account.permissions ?? [])].sort(), platform: [...(props.account.platform_permissions ?? [])].sort(), byBrand: [...(props.account.permissions_by_brand?.[props.brandId] ?? [])].sort(), allowed: permissions.value }));
const groups = computed(() => mode.value === "rewards" ? ["day", "member", "order"] : ["day", "member", "state"]);
function abortRead() { readGeneration++; readController?.abort(); readController = undefined; busy.value = false; }
function abortExport() { exportGeneration++; exportController?.abort(); exportController = undefined; exportBusy.value = false; }
function clearScope() { abortRead(); abortExport(); committed.value = null; result.value = null; error.value = ""; exportError.value = ""; }
function validDate(value: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/.exec(value);
  if (!m) throw new Error(t("请输入有效的 UTC 日期和时间。", "Enter a valid UTC date and time."));
  const [, ys, ms, ds, hs, mins, secs = "0"] = m, y = Number(ys), mo = Number(ms), d = Number(ds), h = Number(hs), mi = Number(mins), s = Number(secs);
  const days = mo === 2 ? (y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0) ? 29 : 28) : ([4, 6, 9, 11].includes(mo) ? 30 : 31);
  if (y < 1 || mo < 1 || mo > 12 || d < 1 || d > days || h > 23 || mi > 59 || s > 59) throw new Error(t("请输入有效的 UTC 日期和时间。", "Enter a valid UTC date and time."));
  return `${ys}-${ms}-${ds}T${hs}:${mins}:${String(s).padStart(2, "0")}Z`;
}
function queryDraft(): RewardReportQuery {
  const from = validDate(fromDraft.value), to = validDate(toDraft.value), fromMs = Date.parse(from), toMs = Date.parse(to);
  if (toMs <= fromMs || toMs - fromMs > 93 * 86_400_000) throw new Error(t("时间范围必须大于零且不超过 93 天。", "Choose a valid range no longer than 93 days."));
  const member = memberDraft.value.trim(), order = orderDraft.value.trim();
  if (member && !uuid.test(member) || order && !uuid.test(order)) throw new Error(t("筛选 UUID 格式无效。", "Enter valid UUIDs for the optional filters."));
  return { from, to, group_by: groupDraft.value, limit: 20, offset: 0, ...(member ? { member_id: member } : {}), ...(order ? { order_id: order } : {}) };
}
function current(generation: number, scope: string) { return alive && generation === readGeneration && scope === scopeKey.value; }
async function run(saved?: { kind: RewardReportKind; query: RewardReportQuery }) {
  abortRead(); abortExport(); result.value = null; error.value = ""; exportError.value = "";
  const generation = readGeneration, scope = scopeKey.value;
  if (!permissions.value.view) { error.value = t("缺少奖励报表查看权限。", "Missing report_reward.view.brand or .platform grant."); return; }
  let request: { kind: RewardReportKind; query: RewardReportQuery };
  try { request = saved ? { kind: saved.kind, query: { ...saved.query } } : { kind: mode.value, query: queryDraft() }; }
  catch (cause) { error.value = cause instanceof Error ? cause.message : t("筛选无效。", "Invalid filters."); return; }
  committed.value = request; busy.value = true; readController = new AbortController();
  try {
    const report = await api.read(request.kind, props.brandId, request.query, readController.signal);
    if (current(generation, scope) && report.brand_id.toLowerCase() === props.brandId.toLowerCase() && JSON.stringify(committed.value) === JSON.stringify(request)) result.value = report;
  } catch (cause) {
    if (!current(generation, scope)) return;
    error.value = cause instanceof Error ? cause.message : t("读取失败。", "Unable to load report.");
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
    exportError.value = cause instanceof Error ? cause.message : t("导出失败。", "Export failed.");
    if (cause instanceof AdminApiError && cause.status === 401) { clearScope(); emit("session-invalid"); }
  } finally { if (generation === exportGeneration) { exportBusy.value = false; exportController = undefined; } }
}
const fields = computed(() => mode.value === "rewards" ? ["entry_count", "grant_entry_count", "grant_points", "reversal_entry_count", "reversal_points", "net_points"] : ["order_count", "original_points", "granted_count", "granted_points", "pending_count", "pending_points", "revoked_count", "revoked_points"]);
const names: Record<string, [string, string]> = {
  entry_count: ["入账条目", "Posting entries"], grant_entry_count: ["发放条目", "Grant entries"], grant_points: ["发放积分", "Granted points"], reversal_entry_count: ["撤销条目", "Reversal entries"], reversal_points: ["撤销积分", "Reversed points"], net_points: ["净积分变化", "Net points"],
  order_count: ["订单数", "Orders"], original_points: ["原始积分", "Original points"], granted_count: ["已发放订单", "Granted orders"], granted_points: ["已发放积分", "Granted points"], pending_count: ["待处理订单", "Pending orders"], pending_points: ["待处理积分", "Pending points"], revoked_count: ["已撤销订单", "Revoked orders"], revoked_points: ["已撤销积分", "Revoked points"],
};
function metric(field: string) { return t(...names[field]!); }
function exact(value: string) { try { const n = BigInt(value), neg = n < 0n, digits = (neg ? -n : n).toString().replace(/\B(?=(\d{3})+(?!\d))/g, ","); return `${neg ? "−" : ""}${digits}`; } catch { return value; } }
function count(totals: RewardReport["summary"], field: string) { return exact((totals as unknown as Record<string, string>)[field]!); }
function groupLabel(group: string) { return group === "day" ? t("按天", "Day") : group === "member" ? t("按会员", "Member") : group === "order" ? t("按订单", "Order") : t("按当前状态", "Current state"); }
function stateLabel(key: string) { return key === "granted" ? t("已发放", "Granted") : key === "revocation_pending" ? t("撤销待处理", "Revocation pending") : key === "revoked" ? t("已撤销", "Revoked") : key; }
function hasNext(report: RewardReport) { return BigInt(report.query.offset) + BigInt(report.items.length) < BigInt(report.total_groups) && report.query.offset + 20 <= 1_000_000; }
function page(offset: number) { if (committed.value) void run({ kind: committed.value.kind, query: { ...committed.value.query, limit: 20, offset } }); }
watch(mode, () => { groupDraft.value = "day"; clearScope(); }, { flush: "sync" });
watch(scopeKey, () => { clearScope(); if (!permissions.value.view) error.value = t("缺少奖励报表查看权限。", "Missing report_reward.view.brand or .platform grant."); }, { flush: "sync" });
onBeforeUnmount(() => { alive = false; clearScope(); });
</script>

<template>
  <section class="reward-reports" aria-labelledby="reward-reports-title">
    <header><div><h3 id="reward-reports-title">{{ t("奖励报表", "Reward reports") }}</h3><p>{{ t("只读报表", "Read-only reports") }}</p></div><button type="button" data-testid="reward-report-query" :disabled="busy || !permissions.view" @click="run()">{{ t("查询", "Query") }}</button></header>
    <div class="modes" role="tablist" :aria-label="t('报表类型', 'Report mode')">
      <button type="button" role="tab" data-testid="reward-report-mode-postings" :aria-selected="mode === 'rewards'" @click="mode = 'rewards'">{{ t("实际奖励入账", "Actual reward postings") }}</button>
      <button type="button" role="tab" data-testid="reward-report-mode-orders" :aria-selected="mode === 'reward_orders'" @click="mode = 'reward_orders'">{{ t("当前奖励订单", "Current reward orders") }}</button>
    </div>
    <p class="definition">{{ mode === "rewards" ? t("按奖励实际入账时间筛选，统计账本发放与撤销。", "Filters by actual reward posting time and counts ledger grants and reversals.") : t("按订单创建时间筛选 cohort，并展示当前状态投影；日期范围不代表状态变更发生时间。", "Filters an order cohort by creation time and shows its current state. The date range is not the time of a state change.") }}</p>
    <p class="definition">{{ mode === "reward_orders" ? t("待撤销只表示当前订单状态，不会预留或提取资金。两种报表均为只读，不会更改奖励或钱包。日期筛选使用 UTC。", "Pending revocation is only the current order state; it does not reserve or withdraw funds. Both reports are read-only and never change rewards or wallets. Date filters use UTC.") : t("按入账时间统计不可变的奖励账本分录，不代表当前钱包余额。报表只读，不会更改奖励或钱包。日期筛选使用 UTC。", "Counts immutable reward ledger postings by posting time and does not represent current wallet balance. This report is read-only and never changes rewards or wallets. Date filters use UTC.") }}</p>
    <div class="filters" data-testid="reward-report-filters">
      <label>{{ t("开始时间（UTC）", "From (UTC)") }}<input v-model="fromDraft" type="datetime-local" step="1"></label>
      <label>{{ t("结束时间（UTC）", "To (UTC)") }}<input v-model="toDraft" type="datetime-local" step="1"></label>
      <label>{{ t("会员 UUID（可选）", "Member UUID (optional)") }}<input v-model.trim="memberDraft" autocomplete="off"></label>
      <label>{{ t("订单 UUID（可选）", "Order UUID (optional)") }}<input v-model.trim="orderDraft" autocomplete="off"></label>
      <label>{{ t("分组", "Group by") }}<select v-model="groupDraft" data-testid="reward-report-group"><option v-for="group in groups" :key="group" :value="group">{{ groupLabel(group) }}</option></select></label>
    </div>
    <p v-if="props.account.super_admin" class="definition">{{ t("超级管理员也必须明确获得奖励报表查看和导出授权；导出还需单独的导出权限。", "Super admins also need explicit reward report view and export grants; export requires its own grant.") }}</p>
    <p v-if="error" class="error" role="alert">{{ error }}</p><p v-if="exportError" class="error" role="alert">{{ exportError }}</p>
    <template v-if="result && committed">
      <div class="context"><span>{{ t("报表", "Report") }} {{ committed.kind === 'rewards' ? t("实际奖励入账", "Actual reward postings") : t("当前奖励订单", "Current reward orders") }}</span><span>{{ t("品牌", "Brand") }} {{ result.brand_id }}</span><span>{{ t("快照", "Snapshot") }} {{ result.snapshot_at }}</span><span>{{ t("品牌时区", "Brand timezone") }} {{ result.timezone }}</span><span>{{ t("分组", "Grouped by") }} {{ groupLabel(result.query.group_by) }}</span><span>{{ t("区间 UTC", "UTC range") }} [{{ result.query.from }}, {{ result.query.to }})</span><span>{{ t("会员", "Member") }}: {{ result.query.member_id ?? t("全部", "All") }}</span><span>{{ t("订单", "Order") }}: {{ result.query.order_id ?? t("全部", "All") }}</span><span>{{ t("总分组", "Groups") }} {{ exact(result.total_groups) }}</span></div>
      <p class="definition">{{ t("导出使用最近一次已提交查询的筛选范围，不使用尚未查询的编辑内容。", "Export uses the last submitted query scope, not unsubmitted edits.") }}</p>
      <div class="actions"><button type="button" data-testid="reward-report-refresh" :disabled="busy" @click="run(committed!)">{{ t("刷新已提交范围", "Refresh committed scope") }}</button><button type="button" data-testid="reward-report-export" :disabled="!permissions.view || !permissions.export || exportBusy" @click="exportAll">{{ exportBusy ? t("导出中…", "Exporting…") : t("导出完整 CSV", "Export full CSV") }}</button><span v-if="!permissions.export">{{ t("需要独立奖励报表导出授权", "Separate reward report export grant required") }}</span></div>
      <div class="metrics" data-testid="reward-report-summary"><div v-for="field in fields" :key="field"><span>{{ metric(field) }}</span><strong>{{ count(result.summary, field) }}</strong></div></div>
      <div v-if="result.items.length" class="table-wrap"><table><thead><tr><th>{{ t("分组键", "Key") }}</th><th>{{ result.query.group_by === 'state' ? t("当前状态", "Current state") : t("标签", "Label") }}</th><th v-for="field in fields" :key="field">{{ metric(field) }}</th></tr></thead><tbody><tr v-for="item in result.items" :key="item.key"><td>{{ item.key }}</td><td>{{ result.query.group_by === 'state' ? stateLabel(item.label) : item.label }}</td><td v-for="field in fields" :key="field">{{ count(item.totals, field) }}</td></tr></tbody></table></div>
      <p v-else class="definition">{{ t("本页没有分组；汇总仍表示完整筛选范围。", "No groups on this page. The summary covers the full filter range.") }}</p>
      <nav class="pages" :aria-label="t('奖励报表分页', 'Reward report pages')"><span>{{ result.query.offset + 1 }}–{{ result.query.offset + result.items.length }} / {{ exact(result.total_groups) }}</span><button type="button" :disabled="busy || result.query.offset === 0" @click="page(Math.max(0, result.query.offset - 20))">{{ t("上一页", "Previous") }}</button><button type="button" :disabled="busy || !hasNext(result)" @click="page(result.query.offset + 20)">{{ t("下一页", "Next") }}</button></nav>
    </template>
    <p v-else-if="busy" class="definition" aria-live="polite">{{ t("读取中…", "Loading…") }}</p>
  </section>
</template>

<style scoped>
.reward-reports{display:grid;gap:12px;min-width:0;border:1px solid #d9e0ea;border-radius:12px;padding:16px;background:#fff;color:#172033}.reward-reports header,.actions,.pages{display:flex;align-items:center;justify-content:space-between;gap:10px;flex-wrap:wrap}.reward-reports h3,.reward-reports header p{margin:0}.reward-reports header p,.definition,.context,.actions span{color:#667085;font-size:.86rem;line-height:1.55}.modes{display:flex;gap:8px;flex-wrap:wrap}.modes button[aria-selected="true"]{border-color:#2563eb;background:#eff6ff;color:#1d4ed8}.filters{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px}.filters label{display:grid;gap:5px;min-width:0;font-size:.86rem}.filters input,.filters select{box-sizing:border-box;width:100%;min-width:0;padding:9px;border:1px solid #cbd3df;border-radius:8px;font:inherit}.reward-reports button{min-height:40px;padding:8px 12px;border:1px solid #c5ceda;border-radius:8px;background:#fff;color:inherit;font:inherit}.reward-reports button:disabled{opacity:.5}.context{display:flex;flex-wrap:wrap;gap:4px 14px;overflow-wrap:anywhere}.metrics{display:grid;grid-template-columns:repeat(auto-fit,minmax(145px,1fr));gap:8px}.metrics div{display:grid;gap:5px;min-width:0;border:1px solid #e5eaf0;border-radius:8px;padding:10px}.metrics span{color:#667085;font-size:.8rem}.metrics strong{overflow-wrap:anywhere;font-variant-numeric:tabular-nums}.table-wrap{overflow:auto;border:1px solid #e2e7ef;border-radius:8px}table{border-collapse:collapse;min-width:900px;width:100%;font-variant-numeric:tabular-nums}th,td{padding:8px;border-bottom:1px solid #edf0f4;text-align:left;white-space:nowrap}th{position:sticky;top:0;background:#f7f9fc}.error{margin:0;padding:10px;border-radius:8px;background:#fff0ee;color:#a32e1a}@media(max-width:700px){.reward-reports{padding:12px}.filters{grid-template-columns:minmax(0,1fr)}.metrics{grid-template-columns:repeat(2,minmax(0,1fr))}.table-wrap{max-height:60vh}.actions button{width:100%}}
</style>
