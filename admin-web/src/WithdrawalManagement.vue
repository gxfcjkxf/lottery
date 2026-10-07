<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { hasPendingWithdrawalIntent, retainWithdrawalIntent, type WithdrawalAction, type WithdrawalActionBody, type WithdrawalHistory, type WithdrawalOrder, type WithdrawalState } from "@lottery/shared";
import type { AdminAccount } from "./admin-api";
import { createIdempotencyKey } from "./admin-api";
import { createWithdrawalOrdersAdminApi, MAX_WITHDRAWAL_PAGE, withdrawalPermissions, WithdrawalAdminUnknownIntentError } from "./withdrawal-orders-api";
import { useAdminI18n } from "./i18n";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { locale } = useAdminI18n();
const api = createWithdrawalOrdersAdminApi();
const page = ref<WithdrawalOrder[]>([]);
const stateFilter = ref<WithdrawalState | "">("");
const memberFilter = ref("");
const offset = ref(0);
const hasMore = ref(false);
const loading = ref(false);
const busy = ref(false);
const error = ref("");
const selected = ref<WithdrawalOrder | null>(null);
const history = ref<WithdrawalHistory | null>(null);
const reason = ref("");
const activeAction = ref<WithdrawalAction | null>(null);
const receipt = ref<WithdrawalOrder | null>(null);
const pending = ref<{ scope: string; orderId: string; action: WithdrawalAction; body: Readonly<WithdrawalActionBody>; key: string } | null>(null);
let generation = 0;
let historyGeneration = 0;
let controller: AbortController | null = null;
let historyController: AbortController | null = null;
let disposed = false;
const grants = computed(() => withdrawalPermissions(props.account, props.brandId));
const scope = computed(() => `${props.account.id}:${props.brandId}:${Number(grants.value.view)}:${Object.values(grants.value.actions).map(Number).join("")}`);
const en = computed(() => locale.value === "en");
const words = computed(() => en.value ? {
  title: "Withdrawal requests", intro: "Review live withdrawal orders and their audit history.", refresh: "Refresh", loading: "Loading…", noView: "This account does not have withdrawal view permission for this brand.", noBrand: "Choose a brand to view withdrawals.", noAccount: "Sign in to the admin account first.", empty: "No withdrawal orders found.", member: "Member ID", state: "State", filter: "Filter", all: "All states", order: "Order", points: "Points", created: "Created", history: "History", close: "Close", reason: "Reason", reasonPlaceholder: "Enter the decision reason", confirm: "Confirm", cancel: "Cancel", approve: "Approve", reject: "Reject", fail: "Mark failed", paid: "Mark paid (internal)", internal: "Internal ledger confirmation only; no external transfer is made.", reviewing: "Reviewing", processing: "Withdrawal in progress", paidState: "Withdrawn", rejected: "Rejected", failed: "Failed", cancelled: "Cancelled", receipt: "Action accepted", receiptNote: "The row retains its original state until you refresh the list.", previous: "Previous", next: "Next", unknown: "Action outcome is unknown. Replay only with this exact frozen action, reason, version, and key.", replay: "Replay same action", acted: "Action recorded", statuses: { reviewing: "Reviewing", processing: "Withdrawal in progress", paid: "Withdrawn", rejected: "Rejected", failed: "Failed", cancelled: "Cancelled" }, actors: "Actor type"
} : {
  title: "提现申请", intro: "审核真实提现订单及其审计记录。", refresh: "刷新", loading: "正在加载…", noView: "此账号没有当前品牌的提现查看权限。", noBrand: "请选择品牌以查看提现申请。", noAccount: "请先登录后台账号。", empty: "没有符合条件的提现订单。", member: "成员 ID", state: "状态", filter: "筛选", all: "全部状态", order: "订单", points: "积分", created: "创建时间", history: "历史记录", close: "关闭", reason: "原因", reasonPlaceholder: "填写处理原因", confirm: "确认", cancel: "取消", approve: "通过", reject: "驳回", fail: "标记失败", paid: "内部确认已提现", internal: "仅为内部账本操作，不会进行外部转账。", reviewing: "审核中", processing: "提现中", paidState: "已提现", rejected: "已驳回", failed: "失败", cancelled: "已取消", receipt: "操作已接受", receiptNote: "刷新列表前，订单行仍显示操作前的状态。", previous: "上一页", next: "下一页", unknown: "操作结果未知。只能按冻结的原操作、原因、版本和密钥重放。", replay: "重放相同操作", acted: "操作已记录", statuses: { reviewing: "审核中", processing: "提现中", paid: "已提现", rejected: "已驳回", failed: "失败", cancelled: "已取消" }, actors: "操作者类型"
});
const states: WithdrawalState[] = ["reviewing", "processing", "paid", "rejected", "failed", "cancelled"];
const stateName = (state: WithdrawalState) => words.value.statuses[state];
const actionsFor = (order: WithdrawalOrder): WithdrawalAction[] => order.state === "reviewing" ? ["approve", "reject", "cancel"] : order.state === "processing" ? ["fail", "mark-paid", "cancel"] : [];
const actionName = (action: WithdrawalAction) => ({ approve: words.value.approve, reject: words.value.reject, cancel: words.value.cancel, fail: words.value.fail, "mark-paid": words.value.paid })[action];
function date(value: string) { return new Intl.DateTimeFormat(en.value ? "en" : "zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)); }

async function load(nextOffset = 0) {
  const requestGeneration = ++generation;
  controller?.abort();
  controller = new AbortController();
  const requestScope = scope.value;
  pending.value = retainWithdrawalIntent(pending.value, requestScope);
  page.value = [];
  receipt.value = null;
  error.value = "";
  offset.value = nextOffset;
  if (!grants.value.view || !props.brandId) { loading.value = false; return; }
  loading.value = true;
  try {
    const result = await api.list(props.brandId, { state: stateFilter.value || undefined, memberId: memberFilter.value, limit: MAX_WITHDRAWAL_PAGE, offset: nextOffset }, controller.signal);
    if (disposed || requestGeneration !== generation || requestScope !== scope.value) return;
    if (result.brand_id !== props.brandId || result.items.some((item) => item.brand_id !== props.brandId)) throw new Error("The server returned withdrawal orders for another brand.");
    page.value = result.items;
    hasMore.value = result.has_more;
  } catch (cause) {
    if (disposed || requestGeneration !== generation || requestScope !== scope.value || cause instanceof DOMException && cause.name === "AbortError") return;
    if (cause && typeof cause === "object" && "status" in cause && Number((cause as {status?:number}).status) === 401) emit("session-invalid");
    error.value = cause instanceof Error ? cause.message : "Request failed";
  } finally { if (!disposed && requestGeneration === generation && requestScope === scope.value) loading.value = false; }
}

async function showHistory(order: WithdrawalOrder) {
  historyController?.abort();
  historyController = new AbortController();
  const requestGeneration = ++historyGeneration;
  selected.value = order;
  history.value = null;
  try {
    const result = await api.history(props.brandId, order.id, historyController.signal);
    if (!disposed && requestGeneration === historyGeneration && selected.value?.id === order.id && result.order_id === order.id && result.brand_id === props.brandId) history.value = result;
  } catch (cause) { if (!disposed && requestGeneration === historyGeneration && selected.value?.id === order.id && !(cause instanceof DOMException && cause.name === "AbortError")) error.value = cause instanceof Error ? cause.message : "Request failed"; }
}

function begin(order: WithdrawalOrder, action: WithdrawalAction) {
  if (hasPendingWithdrawalIntent(pending.value) || !grants.value.actions[action] || !actionsFor(order).includes(action)) return;
  selected.value = order;
  reason.value = "";
  activeAction.value = action;
  pending.value = null;
}

async function perform(action: WithdrawalAction, replay = false) {
  const order = selected.value;
  if (busy.value) return;
  const requestScope = scope.value;
  let intent = pending.value;
  if (replay) { if (!intent || intent.scope !== requestScope || intent.action !== action || !grants.value.actions[action]) return; }
  else {
    if (!order || hasPendingWithdrawalIntent(intent) || !grants.value.actions[action] || !actionsFor(order).includes(action)) return;
    if (!reason.value.trim()) { error.value = words.value.reasonPlaceholder; return; }
    const body = Object.freeze({ version: order.version, reason: reason.value.trim() });
    intent = Object.freeze({ scope: requestScope, orderId: order.id, action, body, key: createIdempotencyKey() });
  }
  busy.value = true;
  error.value = "";
  try {
    const updated = await api.action(props.brandId, intent.orderId, action, intent.body, intent.key);
    if (disposed || scope.value !== requestScope) return;
    if (updated.id !== intent.orderId || updated.brand_id !== props.brandId) throw new WithdrawalAdminUnknownIntentError("The server returned an action receipt for another order.");
    pending.value = null;
    receipt.value = updated;
    selected.value = null;
    activeAction.value = null;
    reason.value = "";
  } catch (cause) {
    if (disposed || scope.value !== requestScope) return;
    if (cause instanceof WithdrawalAdminUnknownIntentError) pending.value = intent;
    else if ((cause && typeof cause === "object" && "status" in cause && [0, 408, 429].includes(Number((cause as {status?:number}).status))) || (cause instanceof Error && !("status" in cause))) pending.value = intent;
    if (cause && typeof cause === "object" && "status" in cause && Number((cause as {status?:number}).status) === 401) emit("session-invalid");
    error.value = cause instanceof Error ? cause.message : "Request failed";
  } finally { if (scope.value === requestScope) busy.value = false; }
}

watch(scope, () => {
  pending.value = null;
  selected.value = null;
  history.value = null;
  activeAction.value = null;
  busy.value = false;
  controller?.abort();
  historyController?.abort();
  void load();
}, { immediate: true });
onUnmounted(() => { disposed = true; generation++; historyGeneration++; controller?.abort(); historyController?.abort(); });
</script>

<template>
  <section class="withdrawal-management page-content">
    <header class="panel withdrawal-heading"><div><div class="eyebrow">{{ en ? "FUNDS" : "资金" }}</div><h1>{{ words.title }}</h1><p>{{ words.intro }}</p></div><button class="button button-secondary" :disabled="loading || !grants.view" @click="load(offset)">{{ loading ? words.loading : words.refresh }}</button></header>
    <div v-if="!props.brandId" class="panel directory-state"><p>{{ words.noBrand }}</p></div>
    <div v-else-if="!grants.view" class="panel directory-state"><p>{{ words.noView }}</p></div>
    <div v-else>
      <p v-if="error" class="withdrawal-error" role="alert">{{ error }}</p>
      <p v-if="receipt" class="withdrawal-receipt" role="status">{{ words.receipt }} · {{ receipt.id }} · {{ stateName(receipt.state) }}. {{ words.receiptNote }}</p>
      <div v-if="pending && pending.scope === scope" class="withdrawal-confirm" role="alert"><strong>{{ words.unknown }} · {{ pending.orderId }}</strong><p>{{ words.reason }} · {{ pending.body.reason }}</p><button class="button button-secondary" :disabled="busy" @click="perform(pending.action, true)">{{ words.replay }}</button></div>
      <div class="panel withdrawal-filters"><label>{{ words.state }}<select v-model="stateFilter"><option value="">{{ words.all }}</option><option v-for="state in states" :key="state" :value="state">{{ stateName(state) }}</option></select></label><label>{{ words.member }}<input v-model="memberFilter" type="text" autocomplete="off" /></label><button class="button button-primary" :disabled="loading" @click="load(0)">{{ words.filter }}</button></div>
      <div class="panel withdrawal-table-wrap"><table class="withdrawal-table"><thead><tr><th>{{ words.order }}</th><th>{{ words.member }}</th><th>{{ words.points }}</th><th>{{ words.state }}</th><th>{{ words.created }}</th><th></th></tr></thead><tbody><tr v-for="order in page" :key="order.id"><td class="mono">{{ order.id }}</td><td class="mono">{{ order.member_id }}</td><td>{{ order.points }}</td><td><span class="badge" :class="order.state === 'paid' ? 'badge-success' : ['failed','rejected','cancelled'].includes(order.state) ? 'badge-neutral' : 'badge-warn'">{{ stateName(order.state) }}</span></td><td>{{ date(order.created_at) }}</td><td><button class="text-button" @click="showHistory(order)">{{ words.history }}</button></td></tr></tbody></table><p v-if="!page.length && !loading" class="muted">{{ words.empty }}</p></div>
      <nav class="withdrawal-pagination"><button class="button button-secondary" :disabled="offset === 0 || loading" @click="load(Math.max(0, offset - MAX_WITHDRAWAL_PAGE))">{{ words.previous }}</button><span>{{ offset + 1 }}–{{ offset + page.length }}</span><button class="button button-secondary" :disabled="!hasMore || loading" @click="load(offset + MAX_WITHDRAWAL_PAGE)">{{ words.next }}</button></nav>
      <div v-if="selected" class="panel withdrawal-detail"><div class="panel-header"><div><h2>{{ selected.id }}</h2><p>{{ words.state }} · {{ stateName(selected.state) }}</p></div><button class="text-button" @click="selected = null; history = null">{{ words.close }}</button></div><div v-if="history"><div v-for="item in history.items" :key="item.id" class="withdrawal-history-row"><span>{{ item.from_state ? stateName(item.from_state) + ' → ' : '' }}{{ stateName(item.to_state) }}</span><small>{{ date(item.created_at) }} · {{ words.actors }}: {{ item.actor_type }} · {{ item.reason }}</small></div></div><p v-else class="muted">{{ words.loading }}</p>
        <template v-if="actionsFor(selected).some((action) => grants.actions[action])"><div class="withdrawal-actions"><button v-for="action in actionsFor(selected).filter((entry) => grants.actions[entry])" :key="action" class="button button-secondary" :disabled="Boolean(pending)" :title="action === 'mark-paid' ? words.internal : undefined" @click="begin(selected!, action)">{{ actionName(action) }}</button></div><form v-if="activeAction && !pending" class="withdrawal-confirm" @submit.prevent="perform(activeAction)"><p v-if="activeAction === 'mark-paid'" class="withdrawal-internal-note" role="note">{{ words.internal }}</p><label>{{ words.reason }}<textarea v-model="reason" :placeholder="words.reasonPlaceholder" required maxlength="1000" /></label><div class="withdrawal-actions"><button class="button button-secondary" type="button" @click="reason = ''; activeAction = null">{{ words.close }}</button><button class="button button-primary" type="submit" :disabled="busy || !activeAction">{{ activeAction === 'mark-paid' ? words.confirm : `${words.confirm} ${actionName(activeAction)}` }}</button></div></form></template>
      </div>
    </div>
  </section>
</template>

<style scoped>
.withdrawal-management{max-width:1440px;margin:0 auto}.withdrawal-heading{display:flex;justify-content:space-between;align-items:center;gap:18px;padding:22px;margin-bottom:16px}.withdrawal-heading h1{margin:4px 0}.withdrawal-heading p{margin:0;color:var(--muted)}.withdrawal-filters{display:flex;align-items:end;gap:12px;padding:16px;margin-bottom:14px}.withdrawal-filters label{display:grid;gap:6px;min-width:min(100%,220px)}.withdrawal-table-wrap{overflow:auto;padding:8px 16px}.withdrawal-table{width:100%;border-collapse:collapse;min-width:680px}.withdrawal-table th,.withdrawal-table td{text-align:left;padding:12px 10px;border-bottom:1px solid var(--line,#dfe4e2)}.withdrawal-table th{color:var(--muted);font-size:12px}.withdrawal-pagination{display:flex;justify-content:center;align-items:center;gap:14px;padding:14px}.withdrawal-detail{padding:20px;margin-top:14px}.withdrawal-history-row{display:grid;gap:5px;padding:10px 0;border-bottom:1px solid var(--line,#dfe4e2)}.withdrawal-history-row small{color:var(--muted)}.withdrawal-actions{display:flex;flex-wrap:wrap;gap:8px;margin-top:14px}.withdrawal-confirm{display:grid;gap:10px;margin-top:14px;padding:14px;border-radius:10px;background:var(--surface-soft,#f5f7f6)}.withdrawal-confirm label{display:grid;gap:6px}.withdrawal-confirm textarea{min-height:80px;resize:vertical}.withdrawal-error{color:var(--danger,#a3372b)}.withdrawal-receipt{padding:12px;background:var(--surface-soft,#f5f7f6);border-radius:8px}@media(max-width:600px){.withdrawal-heading,.withdrawal-filters{align-items:stretch;flex-direction:column}.withdrawal-filters label{min-width:0}.withdrawal-management{padding-inline:0}}
</style>
