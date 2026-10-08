<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { useAdminI18n } from "./i18n";
import { commissionReportPermissions, createCommissionReportApi, type CommissionGroup, type CommissionReport, type CommissionReportQuery, type CommissionTotals } from "./commission-report-api";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createCommissionReportApi();
const { t } = useAdminI18n();
const permissions = computed(() => commissionReportPermissions(props.account, props.brandId));
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const localDateTime = (value: Date) => { const p = (n: number) => String(n).padStart(2, "0"); return `${value.getFullYear()}-${p(value.getMonth() + 1)}-${p(value.getDate())}T${p(value.getHours())}:${p(value.getMinutes())}:${p(value.getSeconds())}`; };
const endDefault = new Date(Math.ceil(Date.now() / 1000) * 1000), startDefault = new Date(endDefault); startDefault.setDate(startDefault.getDate() - 7);
const fromDraft = ref(localDateTime(startDefault)), toDraft = ref(localDateTime(endDefault));
const agentDraft = ref(""), memberDraft = ref(""), cycleDraft = ref(""), groupDraft = ref<CommissionGroup>("day");
const committed = ref<CommissionReportQuery | null>(null), result = ref<CommissionReport | null>(null);
const error = ref(""), exportError = ref(""), busy = ref(false), exportBusy = ref(false), offset = ref(0);
let requestGeneration = 0, exportGeneration = 0, alive = true;
const scopeKey = computed(() => JSON.stringify({ brand: props.brandId, actor: props.account.id, super: props.account.super_admin, brands: [...(props.account.brand_ids ?? [])].sort(), permissions: [...(props.account.permissions ?? [])].sort(), platform: [...(props.account.platform_permissions ?? [])].sort(), byBrand: [...(props.account.permissions_by_brand?.[props.brandId] ?? [])].sort(), allowed: permissions.value }));
function invalidate() { requestGeneration++; exportGeneration++; result.value = null; committed.value = null; busy.value = false; exportBusy.value = false; offset.value = 0; error.value = ""; exportError.value = ""; }
function queryFromDraft(): CommissionReportQuery {
  const from = strictLocalDate(fromDraft.value), to = strictLocalDate(toDraft.value);
  if (to <= from || to.getTime() - from.getTime() > 93 * 86400000) throw new Error(t("时间范围必须大于零且不超过 93 天。", "Choose a valid range no longer than 93 days."));
  const ids = [agentDraft.value.trim(), memberDraft.value.trim(), cycleDraft.value.trim()];
  if (ids.some((id) => id && !uuid.test(id))) throw new Error(t("筛选 UUID 格式无效。", "Enter valid UUIDs for the optional filters."));
  return { from: from.toISOString(), to: to.toISOString(), group_by: groupDraft.value, limit: 20, offset: 0, ...(ids[0] ? { agent_id: ids[0] } : {}), ...(ids[1] ? { member_id: ids[1] } : {}), ...(ids[2] ? { cycle_id: ids[2] } : {}) };
}
function isCurrent(id: number, scope: string) { return alive && id === requestGeneration && scope === scopeKey.value; }
async function run(nextOffset = 0, base?: CommissionReportQuery) {
  const scope = scopeKey.value, id = ++requestGeneration;
  exportGeneration++; exportBusy.value = false;
  result.value = null; error.value = ""; exportError.value = ""; offset.value = nextOffset;
  if (!permissions.value.view) { error.value = t("缺少佣金报表查看权限。", "Missing report_commission.view.brand or .platform grant."); return; }
  let query: CommissionReportQuery;
  try { query = base ? { ...base, limit: 20, offset: nextOffset } : queryFromDraft(); }
  catch (cause) { error.value = cause instanceof Error ? cause.message : t("筛选无效。", "Invalid filters."); return; }
  committed.value = { ...query }; busy.value = true;
  try {
    const data = await api.report(props.brandId, query);
    if (isCurrent(id, scope) && committed.value && data.brand_id.toLowerCase() === props.brandId.toLowerCase()) result.value = data;
  } catch (cause) {
    if (!isCurrent(id, scope)) return;
    result.value = null; error.value = cause instanceof Error ? cause.message : t("读取失败。", "Unable to load report.");
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
    exportError.value = cause instanceof Error ? cause.message : t("导出失败。", "Export failed.");
    if (cause instanceof AdminApiError && cause.status === 401) { invalidate(); emit("session-invalid"); }
  } finally { if (id === exportGeneration) exportBusy.value = false; }
}
const fields: readonly (keyof CommissionTotals)[] = ["entry_count", "paid_entry_count", "paid_points", "adjustment_entry_count", "adjustment_credit_points", "adjustment_debit_points", "correction_entry_count", "correction_credit_points", "correction_debit_points", "net_points"];
const metricLabels: Record<keyof CommissionTotals, [string, string]> = {
  entry_count: ["入账条目", "Ledger entries"], paid_entry_count: ["支付条目", "Paid entries"], paid_points: ["支付入账积分", "Paid ledger credits"], adjustment_entry_count: ["修正条目", "Adjustment entries"], adjustment_credit_points: ["修正增加", "Adjustment credits"], adjustment_debit_points: ["修正扣减", "Adjustment debits"], correction_entry_count: ["更正笔数", "Correction entries"], correction_credit_points: ["更正补发积分", "Correction credits"], correction_debit_points: ["更正追回积分", "Correction debits"], net_points: ["净积分变化", "Net points"],
};
function label(field: keyof CommissionTotals) { const pair = metricLabels[field]; return t(pair[0], pair[1]); }
function exact(value: string) { try { const n = BigInt(value), negative = n < 0n, digits = (negative ? -n : n).toString().replace(/\B(?=(\d{3})+(?!\d))/g, ","); return `${negative ? "−" : ""}${digits}`; } catch { return value; } }
function hasNext(value: CommissionReport) { try { return BigInt(value.query.offset) + 20n <= 1_000_000n && BigInt(value.query.offset) + BigInt(value.items.length) < BigInt(value.total_groups); } catch { return false; } }
function groupTitle(value: CommissionGroup) { return value === "day" ? t("按天", "Day") : value === "agent" ? t("按代理", "Agent") : t("按结算周期", "Cycle"); }
function strictLocalDate(value: string): Date {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2})(?:\.(\d{1,3}))?)?$/.exec(value);
  if (!match) throw new Error(t("请输入有效的本地日期和时间。", "Enter a valid local date and time."));
  const [, ys, mos, ds, hs, mis, ss = "0"] = match;
  const year = Number(ys), month = Number(mos), day = Number(ds), hour = Number(hs), minute = Number(mis), second = Number(ss);
  const days = month === 2 ? (year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28) : ([4, 6, 9, 11].includes(month) ? 30 : 31);
  if (year < 1 || month < 1 || month > 12 || day < 1 || day > days || hour > 23 || minute > 59 || second > 59) throw new Error(t("请输入有效的本地日期和时间。", "Enter a valid local date and time."));
  const date = new Date(value);
  if (!Number.isFinite(date.getTime()) || date.getFullYear() !== year || date.getMonth() + 1 !== month || date.getDate() !== day || date.getHours() !== hour || date.getMinutes() !== minute || date.getSeconds() !== second) throw new Error(t("请输入有效的本地日期和时间。", "Enter a valid local date and time."));
  return date;
}
watch(scopeKey, () => { invalidate(); if (!permissions.value.view) error.value = t("缺少佣金报表查看权限。", "Missing report_commission.view.brand or .platform grant."); }, { flush: "sync" });
onBeforeUnmount(() => { alive = false; invalidate(); });
</script>

<template>
  <section class="commission-report" aria-labelledby="commission-report-title">
    <header><div><h3 id="commission-report-title">{{ t("佣金账本报表", "Commission ledger report") }}</h3><p>{{ t("只读业务账本", "Read-only business ledger") }}</p></div><button type="button" data-testid="commission-report-query" :disabled="busy || !permissions.view" @click="run()">{{ t("查询", "Query") }}</button></header>
    <p class="definition">{{ t("按佣金账本实际入账时间 [from, to) 统计已支付目标、修正及更正账本。更正笔数、补发与追回积分反映历史实际入账，不是计划，也不是当前钱包余额。它不是投注或周期归属报表，也不代表预测或已核实的当前收入。周期被阻止或之后发生更正，不会抹去先前的账本入账。", "Counts paid target, adjustment, and correction ledger postings by their actual posting time in [from, to). Correction entry counts, credits, and debits are historical postings, not a plan or current wallet balance. This is not a bet or cycle cohort report, forecast, or verified current earnings. A later blocked cycle or correction does not erase earlier ledger postings.") }}</p>
    <div class="filters" data-testid="commission-report-filters">
      <label>{{ t("开始时间", "From") }}<input v-model="fromDraft" type="datetime-local" step="1"></label>
      <label>{{ t("结束时间", "To") }}<input v-model="toDraft" type="datetime-local" step="1"></label>
      <label>{{ t("代理 UUID", "Agent UUID") }}<input v-model.trim="agentDraft" autocomplete="off"></label>
      <label>{{ t("会员 UUID", "Member UUID") }}<input v-model.trim="memberDraft" autocomplete="off"></label>
      <label>{{ t("周期 UUID", "Cycle UUID") }}<input v-model.trim="cycleDraft" autocomplete="off"></label>
      <label>{{ t("分组", "Group by") }}<select v-model="groupDraft" data-testid="commission-report-group"><option value="day">{{ t("按天", "Day") }}</option><option value="agent">{{ t("按代理", "Agent") }}</option><option value="cycle">{{ t("按结算周期", "Cycle") }}</option></select></label>
    </div>
    <p class="definition">{{ t("日期时间输入使用本设备时区；按天分组使用品牌时区；请求及区间回显使用 UTC。", "Date/time inputs use this device's timezone; day groups use the brand timezone; requests and echoed ranges use UTC.") }}</p>
    <p v-if="props.account.super_admin" class="definition">{{ t("超级管理员也必须显式获得平台级或所选品牌级授权；查看和导出权限分别检查。", "Super admins also need explicit platform or selected-brand grants; view and export are checked separately.") }}</p>
    <p v-if="error" class="error" role="alert">{{ error }}</p><p v-if="exportError" class="error" role="alert">{{ exportError }}</p>
    <template v-if="result">
      <div class="context"><span>{{ t("品牌", "Brand") }} {{ result.brand_id }}</span><span>{{ t("快照", "Snapshot") }} {{ new Date(result.snapshot_at).toLocaleString() }}</span><span>{{ t("品牌时区", "Brand timezone") }} {{ result.timezone }}</span><span>{{ t("分组", "Grouped by") }} {{ groupTitle(result.query.group_by) }}</span><span>{{ t("区间", "Range") }} [{{ result.query.from }}, {{ result.query.to }})</span><span>{{ t("筛选", "Filters") }} {{ t("代理", "Agent") }}: {{ result.query.agent_id ?? t("全部", "All") }}, {{ t("会员", "Member") }}: {{ result.query.member_id ?? t("全部", "All") }}, {{ t("周期", "Cycle") }}: {{ result.query.cycle_id ?? t("全部", "All") }}</span><span>{{ t("总分组", "Groups") }} {{ exact(result.total_groups) }}</span></div>
      <div class="actions"><button type="button" data-testid="commission-report-export" :disabled="!permissions.export || exportBusy" @click="exportAll">{{ exportBusy ? t("导出中…", "Exporting…") : t("导出完整 CSV", "Export full CSV") }}</button><span v-if="!permissions.export">{{ t("需要独立导出授权", "Separate export grant required") }}</span></div>
      <div class="metrics" data-testid="commission-report-summary"><div v-for="field in fields" :key="field"><span>{{ label(field) }}</span><strong>{{ exact(result.summary[field]) }}</strong></div></div>
      <div v-if="result.items.length" class="table-wrap"><table><thead><tr><th>{{ t("分组键", "Key") }}</th><th>{{ t("标签", "Label") }}</th><th v-for="field in fields" :key="field">{{ label(field) }}</th></tr></thead><tbody><tr v-for="item in result.items" :key="item.key"><td>{{ item.key }}</td><td>{{ item.label }}</td><td v-for="field in fields" :key="field">{{ exact(item.totals[field]) }}</td></tr></tbody></table></div>
      <p v-else class="definition">{{ t("本页没有分组；上方汇总仍表示完整筛选范围的总计，可能非零。", "No groups on this page. The summary above still covers the full filter range and may be nonzero.") }}</p>
      <nav class="pages" aria-label="Commission report pages"><span>{{ result.query.offset + 1 }}–{{ result.query.offset + result.items.length }} / {{ exact(result.total_groups) }}</span><button type="button" :disabled="busy || result.query.offset === 0" @click="run(Math.max(0, result.query.offset - 20), result.query)">{{ t("上一页", "Previous") }}</button><button type="button" :disabled="busy || !hasNext(result)" @click="run(result.query.offset + 20, result.query)">{{ t("下一页", "Next") }}</button></nav>
    </template>
    <p v-else-if="busy" class="definition" aria-live="polite">{{ t("读取中…", "Loading…") }}</p>
  </section>
</template>

<style scoped>
.commission-report{display:grid;gap:12px;min-width:0;border:1px solid #d9e0ea;border-radius:12px;padding:16px;background:#fff;color:#172033}.commission-report header,.actions,.pages{display:flex;align-items:center;justify-content:space-between;gap:10px;flex-wrap:wrap}.commission-report h3,.commission-report header p{margin:0}.commission-report header p,.definition,.context,.actions span{color:#667085;font-size:.86rem;line-height:1.55}.filters{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px}.filters label{display:grid;gap:5px;min-width:0;font-size:.86rem}.filters input,.filters select{box-sizing:border-box;width:100%;min-width:0;padding:9px;border:1px solid #cbd3df;border-radius:8px;font:inherit}.commission-report button{min-height:40px;padding:8px 12px;border:1px solid #c5ceda;border-radius:8px;background:#fff;color:inherit;font:inherit}.commission-report button:disabled{opacity:.5}.context{display:flex;flex-wrap:wrap;gap:4px 14px;overflow-wrap:anywhere}.metrics{display:grid;grid-template-columns:repeat(auto-fit,minmax(145px,1fr));gap:8px}.metrics div{display:grid;gap:5px;min-width:0;border:1px solid #e5eaf0;border-radius:8px;padding:10px}.metrics span{color:#667085;font-size:.8rem}.metrics strong{overflow-wrap:anywhere;font-variant-numeric:tabular-nums}.table-wrap{overflow:auto;border:1px solid #e2e7ef;border-radius:8px}table{border-collapse:collapse;min-width:900px;width:100%;font-variant-numeric:tabular-nums}th,td{padding:8px;border-bottom:1px solid #edf0f4;text-align:left;white-space:nowrap}th{position:sticky;top:0;background:#f7f9fc}.error{margin:0;padding:10px;border-radius:8px;background:#fff0ee;color:#a32e1a}@media(max-width:700px){.commission-report{padding:12px}.filters{grid-template-columns:minmax(0,1fr)}.metrics{grid-template-columns:repeat(2,minmax(0,1fr))}.table-wrap{max-height:60vh}.actions button{width:100%}}
</style>
