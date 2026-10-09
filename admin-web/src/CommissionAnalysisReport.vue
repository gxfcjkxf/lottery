<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { useAdminI18n } from "./i18n";
import { COMMISSION_ANALYSIS_COVERAGE_FIELDS, COMMISSION_ANALYSIS_FIELDS, commissionAnalysisPermissions, createCommissionAnalysisApi, type CommissionAnalysisGroup, type CommissionAnalysisQuery, type CommissionAnalysisReport } from "./commission-analysis-api";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createCommissionAnalysisApi();
const { t } = useAdminI18n();
const permissions = computed(() => commissionAnalysisPermissions(props.account, props.brandId));
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const localInput = (date: Date) => { const p = (n: number) => String(n).padStart(2, "0"); return `${date.getFullYear()}-${p(date.getMonth() + 1)}-${p(date.getDate())}T${p(date.getHours())}:${p(date.getMinutes())}:${p(date.getSeconds())}`; };
const endDefault = new Date(Math.ceil(Date.now() / 1000) * 1000), startDefault = new Date(endDefault); startDefault.setDate(startDefault.getDate() - 7);
const fromDraft = ref(localInput(startDefault)), toDraft = ref(localInput(endDefault));
const groupDraft = ref<CommissionAnalysisGroup>("cycle"), agentDraft = ref(""), memberDraft = ref(""), cycleDraft = ref("");
const result = ref<CommissionAnalysisReport | null>(null), committed = ref<CommissionAnalysisQuery | null>(null);
const error = ref(""), exportError = ref(""), busy = ref(false), exportBusy = ref(false), offset = ref(0);
let requestGeneration = 0, exportGeneration = 0, alive = true, requestController: AbortController | null = null, exportController: AbortController | null = null;
const scopeKey = computed(() => JSON.stringify({ brand: props.brandId, actor: props.account.id, superAdmin: props.account.super_admin, brands: [...(props.account.brand_ids ?? [])].sort(), permissions: [...(props.account.permissions ?? [])].sort(), platform: [...(props.account.platform_permissions ?? [])].sort(), byBrand: [...(props.account.permissions_by_brand?.[props.brandId] ?? [])].sort(), allowed: permissions.value }));
const draftKey = computed(() => JSON.stringify([fromDraft.value, toDraft.value, groupDraft.value, agentDraft.value, memberDraft.value, cycleDraft.value]));
function clearCurrent() {
  requestGeneration++; exportGeneration++; requestController?.abort(); exportController?.abort(); requestController = null; exportController = null;
  result.value = null; committed.value = null; busy.value = false; exportBusy.value = false; offset.value = 0; exportError.value = "";
}
function strictLocal(value: string): Date {
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/.exec(value);
  if (!m) throw new Error(t("请输入有效的本地日期和时间。", "Enter a valid local date and time."));
  const [, ys, ms, ds, hs, mins, ss = "0"] = m, y = Number(ys), mo = Number(ms), d = Number(ds), h = Number(hs), mi = Number(mins), sec = Number(ss);
  const days = mo === 2 ? (y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0) ? 29 : 28) : ([4, 6, 9, 11].includes(mo) ? 30 : 31);
  if (mo < 1 || mo > 12 || d < 1 || d > days || h > 23 || mi > 59 || sec > 59) throw new Error(t("请输入有效的本地日期和时间。", "Enter a valid local date and time."));
  const date = new Date(value);
  if (!Number.isFinite(date.getTime()) || date.getFullYear() !== y || date.getMonth() + 1 !== mo || date.getDate() !== d || date.getHours() !== h || date.getMinutes() !== mi || date.getSeconds() !== sec) throw new Error(t("请输入有效的本地日期和时间。", "Enter a valid local date and time."));
  return date;
}
function queryFromDraft(): CommissionAnalysisQuery {
  const start = strictLocal(fromDraft.value), end = strictLocal(toDraft.value);
  if (end <= start || end.getTime() - start.getTime() > 93 * 86_400_000) throw new Error(t("时间范围必须大于零且不超过 93 天。", "Choose a valid range no longer than 93 days."));
  const ids = [agentDraft.value.trim(), memberDraft.value.trim(), cycleDraft.value.trim()];
  if (ids.some((id) => id && !UUID.test(id))) throw new Error(t("筛选 UUID 格式无效。", "Enter valid UUIDs for optional filters."));
  return { from: start.toISOString(), to: end.toISOString(), group_by: groupDraft.value, limit: 20, offset: 0, ...(ids[0] ? { agent_id: ids[0] } : {}), ...(ids[1] ? { member_id: ids[1] } : {}), ...(ids[2] ? { cycle_id: ids[2] } : {}) };
}
function isCurrent(id: number, scope: string) { return alive && id === requestGeneration && scope === scopeKey.value; }
async function run(nextOffset = 0, base?: CommissionAnalysisQuery) {
  requestController?.abort(); exportController?.abort();
  const scope = scopeKey.value, id = ++requestGeneration; exportGeneration++;
  result.value = null; error.value = ""; exportError.value = ""; exportBusy.value = false; offset.value = nextOffset;
  if (!permissions.value.view) { committed.value = null; busy.value = false; error.value = t("需要 commission.view 与 report_commission.view 的品牌级或平台级授权。", "Requires both commission.view and report_commission.view grants in brand or platform scope."); return; }
  let query: CommissionAnalysisQuery;
  try { query = base ? { ...base, limit: 20, offset: nextOffset } : queryFromDraft(); }
  catch (cause) { committed.value = null; busy.value = false; error.value = cause instanceof Error ? cause.message : t("筛选无效。", "Invalid filters."); return; }
  committed.value = { ...query }; busy.value = true; requestController = new AbortController(); const controller = requestController;
  try {
    const data = await api.report(props.brandId, query, controller.signal);
    if (isCurrent(id, scope) && committed.value && data.brand_id.toLowerCase() === props.brandId.toLowerCase()) result.value = data;
  } catch (cause) {
    if (!isCurrent(id, scope)) return;
    committed.value = null; result.value = null; error.value = cause instanceof Error ? cause.message : t("读取失败。", "Unable to load report.");
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
    exportError.value = cause instanceof Error ? cause.message : t("导出失败。", "Unable to export report.");
    if (cause instanceof AdminApiError && cause.status === 401) { clearCurrent(); error.value = t("登录状态已失效。", "Your admin session has expired."); emit("session-invalid"); }
  } finally { if (alive && id === exportGeneration) { exportBusy.value = false; exportController = null; } }
}
function exact(value: string | null | boolean) {
  if (value === null) return t("未知", "Unknown");
  if (typeof value === "boolean") return value ? t("完整", "Complete") : t("不完整", "Incomplete");
  try { const n = BigInt(value), neg = n < 0n, digits = (neg ? -n : n).toString().replace(/\B(?=(\d{3})+(?!\d))/g, ","); return `${neg ? "−" : ""}${digits}`; } catch { return value; }
}
const metricLabels: Record<typeof COMMISSION_ANALYSIS_FIELDS[number], [string, string]> = {
  observed_calculated_points: ["已知核算积分", "Observed calculated points"], calculated_points: ["完整核算积分", "Calculated points"],
  paid_entry_count: ["原派发笔数", "Original paid entries"], paid_points: ["原派发积分", "Original paid points"],
  adjustment_entry_count: ["人工修正笔数", "Manual adjustment entries"], adjustment_credit_points: ["人工修正增加", "Adjustment credits"], adjustment_debit_points: ["人工修正扣减", "Adjustment debits"],
  correction_entry_count: ["已执行更正笔数", "Executed correction entries"], correction_credit_points: ["更正补发积分", "Correction credits"], correction_debit_points: ["更正追回积分", "Correction debits"],
  posting_entry_count: ["实际入账总笔数", "Total posting entries"], actual_net_points: ["实际净入账", "Actual net posted"], manual_adjustment_net_points: ["人工修正净额", "Manual adjustment net"],
  effective_target_points: ["有效目标积分", "Effective target points"], calculation_minus_actual_points: ["核算减实际", "Calculation minus actual"], effective_minus_actual_points: ["有效目标减实际", "Effective target minus actual"],
  calculation_complete: ["核算完整", "Calculation complete"], effective_target_complete: ["有效目标完整", "Effective target complete"],
};
function metricLabel(field: typeof COMMISSION_ANALYSIS_FIELDS[number]) { const pair = metricLabels[field]; return t(pair[0], pair[1]); }
function coverageLabel(field: typeof COMMISSION_ANALYSIS_COVERAGE_FIELDS[number]) { const labels: Record<typeof COMMISSION_ANALYSIS_COVERAGE_FIELDS[number], [string, string]> = { selected_cycle_count: ["选中周期", "Selected cycles"], ready_cycle_count: ["就绪周期", "Ready cycles"], unready_cycle_count: ["未就绪周期", "Unready cycles"] }; const pair = labels[field]; return t(pair[0], pair[1]); }
function hasNext(report: CommissionAnalysisReport) { return BigInt(report.query.offset) + BigInt(report.items.length) < BigInt(report.total_groups) && report.query.offset < 1_000_000; }
function rangeTitle(group: CommissionAnalysisGroup) { return group === "cycle" ? t("按完整周期", "By complete cycle") : t("按受益代理", "By beneficiary agent"); }
watch(draftKey, () => { clearCurrent(); error.value = ""; }, { flush: "sync" });
watch(scopeKey, () => { clearCurrent(); if (!permissions.value.view) error.value = t("需要 commission.view 与 report_commission.view 的品牌级或平台级授权。", "Requires both commission.view and report_commission.view grants in brand or platform scope."); }, { flush: "sync" });
onBeforeUnmount(() => { alive = false; clearCurrent(); });
</script>

<template>
  <section class="commission-analysis" aria-labelledby="commission-analysis-title">
    <header><div><h3 id="commission-analysis-title">{{ t("佣金周期核算与实际入账分析", "Commission cycle calculation and actual posting analysis") }}</h3><p>{{ t("只读分析 · 独立周期分析报表", "Read-only analysis · separate cycle analysis report") }}</p></div><span v-if="busy" aria-live="polite">{{ t("读取中…", "Loading…") }}</span></header>
    <p class="explanation">{{ t("按保存周期的 window_to 选择完整周期（from 含、to 不含，最多 93 天），不会按天拆分。周期实际入账会纳入窗口外稍后发生的历史补发、追回和人工修正。完整核算与有效目标基于周期保存证据，不使用当前比例、钱包余额或未来预测。差额只是数学比较，不授予付款、补发或追回权限；此页面不会改变资金或审批状态。", "Select complete saved cycles by window_to (from inclusive, to exclusive, up to 93 days); commission is not split by day. Actual postings include later historical credits, debits and adjustments outside the selected window. Calculation and effective targets use saved cycle evidence, not today's rates, wallet balances or forecasts. Differences are mathematical comparisons and grant no payment, credit or recovery authority. This page does not change funds or approval state.") }}</p>
    <p class="explanation">{{ t("“未知”表示证据未就绪或不完整，不代表零。实际净额是佣金实际入账净额，不是钱包余额，也不表示付款授权或结清。代理分组仅含已保存的受益代理；未就绪周期仍计入覆盖范围。", "“Unknown” means evidence is incomplete or not ready; it does not mean zero. Actual net is historical commission postings, not a wallet balance, payment authorization or settlement. Agent grouping includes only saved beneficiaries; unready cycles remain in coverage.") }}</p>
    <p v-if="props.account.super_admin" class="explanation">{{ t("超级管理员标记不会授予权限；查看必须同时有 commission.view 与 report_commission.view，导出还需要 report_commission.export。", "The super-admin flag grants no access. Viewing requires both commission.view and report_commission.view; export also requires report_commission.export.") }}</p>
    <div class="filters" data-testid="commission-analysis-filters">
      <label>{{ t("周期结束时间下界（本地，含）", "Cycle-end lower bound (local, inclusive)") }}<input v-model="fromDraft" type="datetime-local" step="1"></label>
      <label>{{ t("周期结束时间上界（本地，不含）", "Cycle-end upper bound (local, exclusive)") }}<input v-model="toDraft" type="datetime-local" step="1"></label>
      <label>{{ t("分组", "Group by") }}<select v-model="groupDraft"><option value="cycle">{{ t("按完整周期", "By complete cycle") }}</option><option value="agent">{{ t("按受益代理", "By beneficiary agent") }}</option></select></label>
      <label>{{ t("代理 UUID（可选）", "Agent UUID (optional)") }}<input v-model.trim="agentDraft" autocomplete="off"></label>
      <label>{{ t("会员 UUID（可选）", "Member UUID (optional)") }}<input v-model.trim="memberDraft" autocomplete="off"></label>
      <label>{{ t("周期 UUID（可选）", "Cycle UUID (optional)") }}<input v-model.trim="cycleDraft" autocomplete="off"></label>
    </div>
    <p class="explanation">{{ t("日期时间按本设备时区输入，请明确点击查询；修改任一筛选会清除已提交结果和导出资格。导出始终绑定当前完整、已提交筛选，不采用分页或未提交条件。", "Times use this device's timezone. Click Query to submit; changing any filter clears the prior result and export eligibility. Export uses the complete committed filter, never a page or draft values.") }}</p>
    <div class="actions"><button type="button" data-testid="commission-analysis-query" :disabled="busy || !permissions.view" @click="run()">{{ t("查询周期分析", "Query cycle analysis") }}</button><span v-if="!permissions.view">{{ t("需要 commission.view + report_commission.view", "Requires commission.view + report_commission.view") }}</span></div>
    <p v-if="error" class="error" role="alert">{{ error }}</p><p v-if="exportError" class="error" role="alert">{{ exportError }}</p>
    <template v-if="result">
      <div class="context"><span>{{ t("品牌", "Brand") }} {{ result.brand_id }}</span><span>{{ t("快照", "Snapshot") }} {{ result.snapshot_at }}</span><span>{{ t("品牌时区", "Brand timezone") }} {{ result.timezone }}</span><span>{{ t("分组", "Grouping") }} {{ rangeTitle(result.query.group_by) }}</span><span>[{{ result.query.from }}, {{ result.query.to }})</span><span>{{ t("完整分组数", "Total groups") }} {{ exact(result.total_groups) }}</span></div>
      <section class="coverage" aria-labelledby="commission-analysis-coverage-title"><h4 id="commission-analysis-coverage-title">{{ t("周期覆盖", "Cycle coverage") }}</h4><dl><div v-for="field in COMMISSION_ANALYSIS_COVERAGE_FIELDS" :key="field"><dt>{{ coverageLabel(field) }}</dt><dd>{{ exact(result.coverage[field]) }}</dd></div></dl></section>
      <div class="actions"><button type="button" data-testid="commission-analysis-export" :disabled="!permissions.export || exportBusy || busy" @click="exportAll">{{ exportBusy ? t("导出中…", "Exporting…") : t("导出完整筛选 CSV", "Export full-filter CSV") }}</button><span v-if="!permissions.export">{{ t("需要独立 report_commission.export 授权", "Separate report_commission.export grant required") }}</span></div>
      <section class="totals" data-testid="commission-analysis-summary" aria-labelledby="commission-analysis-summary-title"><h4 id="commission-analysis-summary-title">{{ t("完整筛选汇总（不是当前页合计）", "Full-filter summary (not this page)") }}</h4><dl><div v-for="field in COMMISSION_ANALYSIS_FIELDS" :key="field"><dt>{{ metricLabel(field) }}</dt><dd>{{ exact(result.summary[field]) }}</dd></div></dl></section>
      <div v-if="result.items.length" class="table-wrap"><table><caption>{{ t("当前页分组明细", "Current page groups") }}</caption><thead><tr><th scope="col">{{ t("分组键 / 标签", "Group key / label") }}</th><th v-for="field in COMMISSION_ANALYSIS_FIELDS" :key="field" scope="col">{{ metricLabel(field) }}</th></tr></thead><tbody><tr v-for="item in result.items" :key="item.key"><th scope="row"><span>{{ item.label }}</span><small>{{ item.key }}</small></th><td v-for="field in COMMISSION_ANALYSIS_FIELDS" :key="field">{{ exact(item.totals[field]) }}</td></tr></tbody></table></div>
      <section v-if="result.items.length" class="mobile-groups" :aria-label="t('当前页分组明细', 'Current page groups')"><article v-for="item in result.items" :key="item.key"><h4>{{ item.label }}</h4><code>{{ item.key }}</code><details><summary>{{ t("查看全部 18 项指标", "View all 18 metrics") }}</summary><dl><div v-for="field in COMMISSION_ANALYSIS_FIELDS" :key="field"><dt>{{ metricLabel(field) }}</dt><dd>{{ exact(item.totals[field]) }}</dd></div></dl></details></article></section>
      <p v-else class="explanation">{{ t("本页没有分组记录；上方汇总仍覆盖完整筛选范围。", "No groups on this page; the summary above still covers the complete filter.") }}</p>
      <nav class="pages" :aria-label="t('佣金周期分析分页', 'Commission cycle analysis pages')"><span>{{ result.query.offset + 1 }}–{{ result.query.offset + result.items.length }} / {{ exact(result.total_groups) }}</span><button type="button" :disabled="busy || result.query.offset === 0" @click="run(Math.max(0, result.query.offset - 20), result.query)">{{ t("上一页", "Previous") }}</button><button type="button" :disabled="busy || !hasNext(result)" @click="run(result.query.offset + 20, result.query)">{{ t("下一页", "Next") }}</button></nav>
    </template>
  </section>
</template>

<style scoped>
.commission-analysis{display:grid;gap:12px;min-width:0;border:1px solid #d9e0ea;border-radius:12px;padding:16px;background:#fff;color:#172033}.commission-analysis>header,.actions,.pages{display:flex;align-items:center;justify-content:space-between;gap:10px;flex-wrap:wrap}.commission-analysis h3,.commission-analysis header p{margin:0}.commission-analysis header p,.explanation,.context,.actions span{color:#667085;font-size:.86rem;line-height:1.55}.explanation{margin:0}.filters{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px}.filters label{display:grid;gap:5px;min-width:0;font-size:.86rem}.filters input,.filters select{box-sizing:border-box;width:100%;min-width:0;padding:9px;border:1px solid #cbd3df;border-radius:8px;font:inherit}.commission-analysis button{min-height:40px;padding:8px 12px;border:1px solid #c5ceda;border-radius:8px;background:#fff;color:inherit;font:inherit;cursor:pointer}.commission-analysis button:disabled{opacity:.5;cursor:not-allowed}.context{display:flex;flex-wrap:wrap;gap:4px 14px;overflow-wrap:anywhere}.coverage,.totals{display:grid;gap:8px}.coverage h4,.totals h4{margin:0}.coverage dl,.totals dl,.mobile-groups dl{display:grid;grid-template-columns:repeat(auto-fit,minmax(145px,1fr));gap:8px;margin:0}.coverage dl>div,.totals dl>div,.mobile-groups dl>div{display:grid;gap:5px;min-width:0;border:1px solid #e5eaf0;border-radius:8px;padding:10px}.coverage dt,.totals dt,.mobile-groups dt{color:#667085;font-size:.8rem}.coverage dd,.totals dd,.mobile-groups dd{margin:0;overflow-wrap:anywhere;font-variant-numeric:tabular-nums}.table-wrap{overflow:auto;border:1px solid #e2e7ef;border-radius:8px}.table-wrap table{border-collapse:collapse;min-width:1450px;width:100%;font-variant-numeric:tabular-nums}.table-wrap caption{text-align:left;padding:8px;font-weight:600}.table-wrap th,.table-wrap td{padding:8px;border-bottom:1px solid #edf0f4;text-align:left;white-space:nowrap}.table-wrap th{position:sticky;top:0;background:#f7f9fc}.table-wrap tbody th{max-width:220px;white-space:normal;overflow-wrap:anywhere}.table-wrap small{display:block;color:#667085}.mobile-groups{display:none}.mobile-groups article{display:grid;gap:6px;border:1px solid #e2e7ef;border-radius:9px;padding:12px;min-width:0}.mobile-groups h4{margin:0}.mobile-groups code{overflow-wrap:anywhere;color:#667085}.mobile-groups details summary{cursor:pointer;padding:8px 0}.pages{justify-content:flex-end}.pages span{margin-right:auto;color:#667085}.pages button{min-width:92px}.error{margin:0;padding:10px;border-radius:8px;background:#fff0ee;color:#a32e1a}@media(max-width:700px){.commission-analysis{padding:12px}.filters{grid-template-columns:minmax(0,1fr)}.table-wrap{display:none}.mobile-groups{display:grid;gap:9px}.coverage dl,.totals dl{grid-template-columns:repeat(2,minmax(0,1fr))}.actions button{width:100%}.pages{justify-content:center}.pages span{flex-basis:100%;text-align:center}.pages button{flex:1}}
</style>
