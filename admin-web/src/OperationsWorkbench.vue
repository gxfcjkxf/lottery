<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { useAdminI18n } from "./i18n";
import { reportsPermissions } from "./reports-api";
import { createWorkbenchApi, workbenchPermissions, type SectionStatus, type WorkbenchSnapshot } from "./workbench-api";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void; (event: "navigate", destination: string): void }>();
const { t, message } = useAdminI18n();
const api = createWorkbenchApi();
const snapshot = ref<WorkbenchSnapshot | null>(null);
const loading = ref(false);
const error = ref<string | ReturnType<typeof message> | null>(null);
const controller = ref<AbortController | null>(null);
let generation = 0;

const permissionKey = computed(() => JSON.stringify({ id: props.account.id, brand_ids: props.account.brand_ids,
  permissions: props.account.permissions, permissions_by_brand: props.account.permissions_by_brand,
  platform_permissions: props.account.platform_permissions }));
const permissions = computed(() => workbenchPermissions(props.account, props.brandId));
const reportAccess = computed(() => reportsPermissions(props.account, props.brandId));
const scopeAvailable = computed(() => /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(props.brandId));

const sections = computed(() => [
  { key: "brand", title: t("品牌", "Brand"), fields: [["name", "品牌名称", "Brand name"], ["code", "品牌代码", "Brand code"], ["state", "状态", "State"]] },
  { key: "periods", title: t("期次", "Periods"), fields: [["pending", "待处理", "Pending"], ["betting", "投注中", "Betting"], ["closed", "已截止", "Closed"], ["waiting_draw", "待开奖", "Awaiting draw"], ["drawn", "已开奖", "Drawn"], ["settling", "结算中", "Settling"], ["refund_pending", "待退款", "Refund pending"], ["refund_failed", "退款失败", "Refund failed"]] },
  { key: "orders", title: t("注单", "Orders"), fields: [["placed", "已提交", "Placed"], ["abnormal", "异常", "Abnormal"]] },
  { key: "today_bets", title: t("今日投注", "Today's bets"), fields: [["order_count", "提交注单数", "Submitted orders"], ["stake_points", "提交投注额（积分）", "Submitted stake (points)"], ["cancelled_count", "已取消", "Cancelled"], ["abnormal_count", "异常", "Abnormal"]], note: t("按提交时间统计，包含取消及异常注单；不代表有效投注额。", "Counted by submission time, including cancelled and abnormal orders; not valid turnover.") },
  { key: "settlement", title: t("结算", "Settlement"), fields: [["processing", "处理中", "Processing"], ["awaiting_approval", "待审批", "Awaiting approval"], ["paying", "派彩中", "Paying"], ["failed", "失败", "Failed"]] },
  { key: "recharges", title: t("充值", "Recharges"), fields: [["pending_count", "待处理笔数", "Pending count"], ["pending_points", "待处理积分", "Pending points"]] },
  { key: "ledger", title: t("账本", "Ledger"), fields: [["entry_count", "流水笔数", "Entries"], ["net_points", "净额（积分）", "Net points"], ["recharge_points", "充值积分", "Recharge points"], ["prize_credit_points", "派奖积分", "Prize credits"], ["prize_reversal_points", "撤销派奖积分", "Prize reversals"], ["refund_points", "退款积分", "Refund points"]] },
  { key: "balances", title: t("余额", "Balances"), fields: [["account_count", "账户数", "Accounts"], ["available_points", "可用积分", "Available points"], ["frozen_points", "冻结积分", "Frozen points"], ["withdrawal_points", "提现中积分", "Withdrawal points"], ["total_points", "总积分", "Total points"]] },
  { key: "reconciliation", title: t("账本对账", "Ledger reconciliation"), fields: [] },
  { key: "sources", title: t("开奖来源", "Draw sources"), fields: [["adapter_state", "适配器状态", "Adapter state"], ["configured_games", "已配置彩种", "Configured games"], ["enabled_api_sources", "启用 API 来源", "Enabled API sources"], ["enabled_dom_sources", "启用 DOM 来源", "Enabled DOM sources"], ["attempts_today", "今日尝试", "Attempts today"], ["failed_today", "今日失败", "Failed today"], ["no_data_today", "今日无数据", "No data today"], ["last_attempt_at", "最近尝试", "Last attempt"]] },
  { key: "withdrawals", title: t("提现", "Withdrawals"), fields: [["reviewing_count", "待审核笔数", "Awaiting review"], ["reviewing_points", "待审核积分", "Awaiting review (points)"], ["processing_count", "提现中笔数", "Processing count"], ["processing_points", "提现中积分", "Processing points"]], note: t("仅统计当前待审核和提现中订单及其订单积分；不代表资格审核、实际支付或利润。", "Counts only current reviewing and processing orders and their order points; this does not indicate eligibility, actual payment, or profit.") },
  { key: "commissions", title: t("佣金", "Commissions"), fields: [] },
  { key: "rewards", title: t("奖励", "Rewards"), fields: [] },
] as const);

function clearScope(): void {
  generation += 1;
  controller.value?.abort();
  controller.value = null;
  snapshot.value = null;
  error.value = null;
  loading.value = false;
}

async function refresh(): Promise<void> {
  clearScope();
  if (!scopeAvailable.value || !props.brandId) return;
  const ticket = generation;
  const accountId = props.account.id;
  const brandId = props.brandId;
  const permissionSnapshot = permissionKey.value;
  const abort = new AbortController();
  controller.value = abort;
  loading.value = true;
  try {
    const result = await api.get(brandId, abort.signal);
    if (generation !== ticket || accountId !== props.account.id || brandId !== props.brandId || permissionSnapshot !== permissionKey.value) return;
    snapshot.value = result;
  } catch (cause) {
    if (generation !== ticket || abort.signal.aborted || accountId !== props.account.id || brandId !== props.brandId || permissionSnapshot !== permissionKey.value) return;
    if (cause instanceof AdminApiError && cause.status === 401) emit("session-invalid");
    error.value = cause instanceof AdminApiError && cause.status === 503
      ? message("工作台暂时不可用，请稍后手动刷新。", "The workbench is temporarily unavailable. Refresh manually later.")
      : cause instanceof AdminApiError && cause.code === "INVALID_RESPONSE"
        ? message("工作台返回了无效数据。", "The workbench returned invalid data.")
        : message("读取工作台失败，请检查网络后手动重试。", "Could not load the workbench. Check the connection and retry manually.");
  } finally {
    if (generation === ticket) { loading.value = false; controller.value = null; }
  }
}

watch(() => [props.account.id, props.brandId, permissionKey.value], () => { clearScope(); void refresh(); }, { immediate: true });
onBeforeUnmount(clearScope);

const accessBySection = (key: string) => permissions.value[key as keyof typeof permissions.value] ?? false;
const section = (key: string) => snapshot.value?.[key as keyof WorkbenchSnapshot] as { status: SectionStatus; data: Record<string, unknown> | null } | undefined;
const displayValue = (value: unknown) => {
  if (value === null) return t("无记录", "No record");
  if (typeof value === "string" && /^\d{4}-\d\d-\d\dT/.test(value)) {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short", timeZone: snapshot.value?.timezone }).format(date);
  }
  return String(value);
};
function jobSummary(): string {
  const job = section("reconciliation")?.data?.latest_job as Record<string, unknown> | null | undefined;
  if (!job) return t("没有对账任务记录", "No reconciliation job recorded");
  const labels: Record<string, [string, string]> = { pending: ["待处理", "Pending"], running: ["运行中", "Running"], completed: ["已完成", "Completed"], failed: ["失败", "Failed"] };
  const [zh, en] = labels[String(job.state)] ?? [String(job.state), String(job.state)];
  return `${t(zh, en)} · ${job.checked_count} / ${job.target_count}`;
}
function jobField(field: string): unknown {
  const data = section("reconciliation")?.data;
  const job = data && "latest_job" in data ? data.latest_job : null;
  return job && typeof job === "object" && field in job ? (job as Record<string, unknown>)[field] : "";
}
function destination(key: string): string | null {
  const routes: Record<string, string> = { periods: "期次和开奖", orders: "注单和异常", today_bets: "报表和对账", settlement: "期次和开奖", recharges: "资金与账本", ledger: "资金与账本", balances: "资金与账本", withdrawals: "资金与账本", reconciliation: "批量对账", sources: "期次和开奖" };
  return routes[key] ?? null;
}
function canNavigate(key: string): boolean {
  if (!accessBySection(key)) return false;
  const route = destination(key);
  if (route === "期次和开奖") return permissions.value.periods;
  if (route === "注单和异常") return permissions.value.orders;
  if (route === "批量对账") return permissions.value.reconciliation;
  if (route === "报表和对账") return reportAccess.value.betting;
  return true;
}
function canOpenReport(key: string): boolean {
  if (key === "orders") return reportAccess.value.betting;
  if (key === "ledger") return reportAccess.value.ledger;
  return false;
}
const snapshotTime = computed(() => snapshot.value ? displayValue(snapshot.value.snapshot_at) : "");
</script>

<template>
  <main class="workbench">
    <header class="workbench-head">
      <div><div class="eyebrow">{{ t("实时运营快照", "Live operations snapshot") }}</div><h1>{{ t("运营工作台", "Operations workbench") }}</h1>
        <p v-if="snapshot">{{ t("品牌", "Brand") }} · {{ snapshot.brand_id }} <span class="dot">·</span> {{ t("快照时间", "Snapshot") }} {{ snapshotTime }} ({{ snapshot.timezone }})</p>
        <p v-else>{{ t("跨期数据按品牌当地日界统计；今日投注按提交时间汇总。", "Daily figures follow the brand's local day; today's bets are grouped by submission time.") }}</p>
      </div>
      <button class="refresh" type="button" :disabled="loading || !scopeAvailable" @click="refresh">{{ loading ? t("读取中…", "Loading…") : t("刷新", "Refresh") }}</button>
    </header>

    <div v-if="!scopeAvailable" class="notice">{{ t("请先登录并选择品牌以查看运营工作台。", "Sign in and select a brand to view the operations workbench.") }}</div>
    <div v-else-if="error" class="notice is-error" role="alert">{{ t(error) }}</div>
    <div v-else-if="loading && !snapshot" class="notice" role="status">{{ t("正在读取当前品牌快照…", "Loading the current brand snapshot…") }}</div>
    <div v-else-if="!snapshot" class="notice">{{ t("请选择一个有权限访问的品牌。", "Choose a brand you are authorized to view.") }}</div>

    <template v-if="snapshot">
      <div class="scope-note">{{ t("时间范围", "Time window") }}: [{{ displayValue(snapshot.day_from) }}, {{ snapshotTime }}) · {{ snapshot.timezone }}
        <span>{{ t("未授权或未实现的数据保持不可用，不以零代替。", "Unauthorized or unimplemented data remains unavailable; it is never shown as zero.") }}</span>
      </div>
      <div class="cards">
        <article v-for="item in sections" :key="item.key" class="card">
          <header class="card-head"><h2>{{ item.title }}</h2>
            <span class="badge" :class="`status-${section(item.key)?.status}`">{{ section(item.key)?.status === 'ready' ? t("可用", "Ready") : section(item.key)?.status === 'forbidden' ? t("无权限", "Unavailable") : t("未实现", "Not implemented") }}</span>
          </header>
          <template v-if="section(item.key)?.status === 'ready' && section(item.key)?.data">
            <template v-if="item.key === 'reconciliation'">
              <div class="job-state">{{ jobSummary() }}</div>
              <dl v-if="section(item.key)?.data?.latest_job" class="metrics">
                <template v-for="field in [['repairable_count','可修复','Repairable'],['corrupt_count','损坏','Corrupt'],['failed_count','失败','Failed']]" :key="field[0]">
                  <div><dt>{{ t(field[1], field[2]) }}</dt><dd>{{ jobField(field[0]) }}</dd></div>
                </template>
              </dl>
            </template>
            <dl v-else class="metrics">
              <div v-for="field in item.fields" :key="field[0]"><dt>{{ t(field[1], field[2]) }}</dt><dd>{{ displayValue(section(item.key)?.data?.[field[0]]) }}</dd></div>
            </dl>
            <p v-if="item.key === 'sources'" class="footnote">{{ t("适配器为 stub；此状态不表示上游来源健康。", "Adapter state is stub; this does not indicate upstream health.") }}</p>
            <p v-if="'note' in item" class="footnote">{{ item.note }}</p>
            <button v-if="destination(item.key)" class="action-link" type="button" :disabled="!canNavigate(item.key)" @click="emit('navigate', destination(item.key)!)">{{ t("打开管理页面", "Open management page") }} <span aria-hidden="true">→</span></button>
            <button v-if="canOpenReport(item.key)" class="action-link report-link" type="button" @click="emit('navigate', '报表和对账')">{{ t("查看报表", "Open reports") }} <span aria-hidden="true">→</span></button>
          </template>
          <p v-else class="unavailable">{{ section(item.key)?.status === 'not_implemented' ? t("此数据模块尚未接入。", "This data section has not been implemented.") : t("当前账号没有此模块的查看权限。", "Your account cannot view this section.") }}</p>
        </article>
      </div>
    </template>
  </main>
</template>

<style scoped>
.workbench{max-width:1440px;margin:0 auto;padding:28px clamp(16px,3vw,42px) 48px;color:#252a36}.workbench-head{display:flex;justify-content:space-between;align-items:flex-start;gap:24px;margin-bottom:22px}.eyebrow{color:#5969df;font-size:11px;font-weight:750;letter-spacing:.1em;text-transform:uppercase}.workbench h1{font-size:clamp(22px,2.2vw,30px);line-height:1.2;letter-spacing:-.035em;margin:7px 0 8px}.workbench-head p{color:#737b8c;margin:0;line-height:1.55}.dot{padding:0 5px;color:#b1b7c2}.refresh{border:0;border-radius:9px;background:#5969df;color:#fff;font-weight:700;padding:11px 17px;white-space:nowrap}.scope-note,.notice{border:1px solid #e9ebf0;background:#fff;border-radius:10px;padding:13px 16px;color:#626b7b;line-height:1.55;margin-bottom:16px}.scope-note span{display:block;color:#9097a4;font-size:12px;margin-top:4px}.notice{margin-top:20px}.is-error{border-color:#f0c8c8;background:#fffafa;color:#9e3f43}.cards{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:14px}.card{min-width:0;background:#fff;border:1px solid #e9ebf0;border-radius:12px;padding:17px;box-shadow:0 3px 12px #1e2a5008}.card-head{display:flex;justify-content:space-between;align-items:center;gap:10px;margin-bottom:14px}.card h2{font-size:15px;margin:0;letter-spacing:-.01em}.badge{font-size:11px;font-weight:700;border-radius:99px;padding:5px 8px;white-space:nowrap}.status-ready{color:#237354;background:#eaf6f0}.status-forbidden{color:#767e8d;background:#f1f2f5}.status-not_implemented{color:#8b6b34;background:#fbf4e5}.metrics{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:13px 10px;margin:0}.metrics div{min-width:0}.metrics dt{font-size:11px;color:#858d9b;line-height:1.4}.metrics dd{font-size:18px;font-variant-numeric:tabular-nums;overflow-wrap:anywhere;font-weight:700;margin:4px 0 0}.job-state{font-size:15px;font-weight:700;margin:2px 0 15px}.unavailable{font-size:13px;color:#8b929f;line-height:1.55;min-height:44px;margin:0}.footnote{font-size:11px;color:#858d9b;line-height:1.5;margin:13px 0 0}.action-link{margin-top:15px;padding:0;border:0;background:transparent;color:#5363d4;font-size:12px;font-weight:700}.action-link:disabled{color:#9298a5;background:none;opacity:.6}.report-link{margin-left:12px}
@media(max-width:1000px){.cards{grid-template-columns:repeat(2,minmax(0,1fr))}}
@media(max-width:560px){.workbench{padding:20px 14px 32px}.workbench-head{gap:12px}.workbench-head p{font-size:12px}.refresh{padding:10px 12px}.cards{grid-template-columns:1fr;gap:10px}.card{padding:15px}.metrics dd{font-size:17px}}
</style>
