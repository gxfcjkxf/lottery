<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { hasPendingWithdrawalIntent, isWithdrawalAllocation, retainWithdrawalIntent, type CreateWithdrawalBody, type WithdrawalAllocation, type WithdrawalAvailability, type WithdrawalHistory, type WithdrawalOrder, type WithdrawalSource } from "@lottery/shared";
import { createWithdrawalOrdersApi, WithdrawalUnknownIntentError } from "./withdrawal-orders-api";
import { formatIntegerAmount } from "./wallet-api";

const props = defineProps<{
  locale: "en" | "zh";
  brandCode?: string;
  member: { id: string; brand_id: string } | null;
}>();
const emit = defineEmits<{ (event: "auth-expired"): void; (event: "context-changed"): void }>();

const api = computed(() => createWithdrawalOrdersApi({ brandCode: props.brandCode }));
const availability = ref<WithdrawalAvailability | null>(null);
const orders = ref<WithdrawalOrder[]>([]);
const history = ref<WithdrawalHistory | null>(null);
const allocations = ref<Record<WithdrawalSource, string>>({ recharge: "", winning: "", gift: "" });
const loading = ref(false);
const submitting = ref(false);
const error = ref("");
const receipt = ref<WithdrawalOrder | null>(null);
const uncertain = ref<{
  scope: string;
  body: Readonly<CreateWithdrawalBody>;
  key: string;
  actorContext: string;
} | null>(null);
const scopeKey = computed(() => props.member ? `${props.brandCode ?? ""}:${props.member.brand_id}:${props.member.id}` : "");
let generation = 0;
let lastScope = "";
let historyGeneration = 0;
let controller: AbortController | null = null;
let historyController: AbortController | null = null;
let disposed = false;
const sources: WithdrawalSource[] = ["recharge", "winning", "gift"];
const labels = computed(() => props.locale === "en"
  ? { title: "Withdrawal requests", intro: "Submit a points withdrawal request and review its status.", refresh: "Refresh", loading: "Loading…", signIn: "Sign in to view withdrawal requests.", min: "Minimum", max: "Maximum", amount: "Points to withdraw", available: "Available source allocation", submit: "Submit request", unavailable: "Withdrawal requests are currently unavailable.", noEligibility: "Withdrawal eligibility has not been configured for this account.", disabled: "Withdrawals are disabled for this brand.", restricted: "This account cannot submit a withdrawal request.", source: { recharge: "Recharge", winning: "Winnings", gift: "Gift" }, orders: "Your requests", empty: "No withdrawal requests yet.", history: "History", close: "Close history", date: "Created", reason: "Reason", status: "Status", receipt: "Request submitted", refreshAfter: "The receipt shows the original submitted state. Refresh separately to see later changes.", replay: "Replay the same request", unknown: "The outcome is unknown. You can explicitly replay the exact same request for this account.", accountChanged: "The account confirmation changed. Refresh availability before starting a new request.", sourceSum: "Source allocations must add up exactly to the requested points.", invalidAmount: "Enter a positive whole number within the server limits.", realPayments: "No external payment is made by this request.", states: { reviewing: "Reviewing", processing: "Processing", paid: "Paid", rejected: "Rejected", failed: "Failed", cancelled: "Cancelled" } }
  : { title: "提现申请", intro: "提交积分提现申请并查看处理状态。", refresh: "刷新", loading: "正在加载…", signIn: "请登录以查看提现申请。", min: "最低金额", max: "最高金额", amount: "提现积分", available: "可用来源分配", submit: "提交申请", unavailable: "当前无法提交提现申请。", noEligibility: "此账户尚未配置提现资格。", disabled: "此品牌已停用提现。", restricted: "此账户无法提交提现申请。", source: { recharge: "充值", winning: "中奖", gift: "赠送" }, orders: "我的申请", empty: "暂无提现申请。", history: "历史记录", close: "关闭记录", date: "创建时间", reason: "原因", status: "状态", receipt: "申请已提交", refreshAfter: "此回执显示提交时的原始状态。请单独刷新以查看后续变化。", replay: "使用相同申请重试", unknown: "提交结果未知。你可以明确选择为此账户重放完全相同的申请。", accountChanged: "账户确认信息已变化。请先刷新资格，再开始新申请。", sourceSum: "各来源分配总额必须与申请积分完全一致。", invalidAmount: "请输入服务器金额限制内的正整数。", realPayments: "此申请不会触发外部付款。", states: { reviewing: "审核中", processing: "提现中", paid: "已提现", rejected: "已驳回", failed: "失败", cancelled: "已取消" } });
const total = computed(() => {
  try { return sources.reduce((sum, source) => sum + BigInt(allocations.value[source].trim() || "0"), 0n); } catch { return -1n; }
});
const validSubmission = computed(() => {
  const current = availability.value;
  if (!current?.can_apply || total.value <= 0n || total.value > 9223372036854775807n || uncertain.value) return false;
  const amount = total.value;
  if (amount < BigInt(current.min_points) || (current.max_points !== null && amount > BigInt(current.max_points))) return false;
  return sources.every((source) => {
    const raw = allocations.value[source].trim();
    if (!raw) return true;
    try { const amount = BigInt(raw); return amount === 0n || current.allowed_sources.includes(source) && isWithdrawalAllocation({ source, state: "available", points: amount.toString() }); } catch { return false; }
  });
});
function sourceName(source: WithdrawalSource) { return labels.value.source[source]; }
function stateName(order: WithdrawalOrder) { return labels.value.states[order.state]; }
function time(value: string) { return new Intl.DateTimeFormat(props.locale === "en" ? "en" : "zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)); }
function makeKey() { return globalThis.crypto?.randomUUID?.() ?? `${Date.now().toString(16)}${Array.from(globalThis.crypto.getRandomValues(new Uint8Array(16)), (n) => n.toString(16).padStart(2, "0")).join("")}`; }

async function load() {
  const requestGeneration = ++generation;
  controller?.abort();
  controller = new AbortController();
  const requestScope = scopeKey.value;
  const changedScope = lastScope !== requestScope;
  uncertain.value = retainWithdrawalIntent(uncertain.value, requestScope);
  if (changedScope) {
    uncertain.value = null;
    availability.value = null;
    allocations.value = { recharge: "", winning: "", gift: "" };
    submitting.value = false;
    receipt.value = null;
  }
  lastScope = requestScope;
  if (changedScope) availability.value = null;
  orders.value = [];
  historyController?.abort();
  historyGeneration++;
  history.value = null;
  error.value = "";
  if (!requestScope || !props.member) { loading.value = false; return; }
  loading.value = true;
  try {
    const [snapshot, page] = await Promise.all([
      api.value.availability(controller.signal), api.value.list(controller.signal),
    ]);
    if (disposed || requestGeneration !== generation || requestScope !== scopeKey.value) return;
    if (snapshot.member_id !== props.member.id || snapshot.brand_id !== props.member.brand_id || page.brand_id !== props.member.brand_id || page.items.some((item) => item.member_id !== props.member!.id || item.brand_id !== props.member!.brand_id)) throw new Error("The server returned withdrawal data for a different account.");
    availability.value = snapshot;
    orders.value = page.items;
  } catch (cause) {
    if (disposed || requestGeneration !== generation || requestScope !== scopeKey.value || (cause instanceof DOMException && cause.name === "AbortError")) return;
    availability.value = null;
    if (cause && typeof cause === "object" && "status" in cause && Number((cause as { status?: number }).status) === 401) emit("auth-expired");
    error.value = cause instanceof Error ? cause.message : "Request failed";
  } finally {
    if (!disposed && requestGeneration === generation && requestScope === scopeKey.value) loading.value = false;
  }
}

function buildBody(): CreateWithdrawalBody | null {
  try {
    const source_allocation = sources.filter((source) => allocations.value[source].trim()).map((source) => ({ source, state: "available" as const, points: BigInt(allocations.value[source].trim()).toString() })).filter((entry) => BigInt(entry.points) > 0n);
    const points = source_allocation.reduce((sum, item) => sum + BigInt(item.points), 0n).toString();
    if (!source_allocation.length || source_allocation.some((item) => !isWithdrawalAllocation(item))) return null;
    return Object.freeze({ points, source_allocation: Object.freeze(source_allocation.map((item) => Object.freeze(item))) }) as CreateWithdrawalBody;
  } catch { return null; }
}

async function submit(replay = false) {
  const currentScope = scopeKey.value;
  if (submitting.value || !currentScope) return;
  let intent = uncertain.value;
  if (replay) {
    if (!intent || intent.scope !== currentScope) return;
  } else {
    if (hasPendingWithdrawalIntent(intent) || !availability.value?.can_apply) return;
    if (!validSubmission.value) { error.value = labels.value.invalidAmount; return; }
    const body = buildBody();
    if (!body) { error.value = labels.value.sourceSum; return; }
    intent = Object.freeze({ scope: currentScope, body, key: makeKey(), actorContext: availability.value.actor_context });
  }
  if (!intent || intent.scope !== currentScope) return;
  submitting.value = true;
  error.value = "";
  try {
    const order = await api.value.create(intent.body, intent.key, intent.actorContext);
    if (disposed || currentScope !== scopeKey.value) return;
    if (order.member_id !== props.member?.id || order.brand_id !== props.member?.brand_id) throw new WithdrawalUnknownIntentError("The server returned a receipt for a different account.");
    uncertain.value = null;
    receipt.value = order;
    allocations.value = { recharge: "", winning: "", gift: "" };
  } catch (cause) {
    if (disposed || currentScope !== scopeKey.value) return;
    if (cause instanceof WithdrawalUnknownIntentError || (cause && typeof cause === "object" && "status" in cause && [0, 408, 429].includes(Number((cause as { status?: number }).status)) || cause instanceof Error && !("status" in cause))) {
      uncertain.value = intent;
      error.value = labels.value.unknown;
    } else if (cause && typeof cause === "object" && "code" in cause && (cause as { code?: string }).code === "WITHDRAWAL_CONFIRMATION_ACCOUNT_CHANGED") {
      uncertain.value = intent;
      error.value = labels.value.accountChanged;
      emit("context-changed");
    } else error.value = cause instanceof Error ? cause.message : "Request failed";
    if (cause && typeof cause === "object" && "status" in cause && Number((cause as { status?: number }).status) === 401) emit("auth-expired");
  } finally {
    if (currentScope === scopeKey.value) submitting.value = false;
  }
}

async function loadHistory(order: WithdrawalOrder) {
  historyController?.abort();
  historyController = new AbortController();
  const requestGeneration = ++historyGeneration;
  try {
    const result = await api.value.history(order.id, historyController.signal);
    if (!disposed && requestGeneration === historyGeneration && result.order_id === order.id && result.brand_id === order.brand_id) history.value = result;
  } catch (cause) {
    if (!disposed && requestGeneration === historyGeneration && !(cause instanceof DOMException && cause.name === "AbortError")) {
      if (cause && typeof cause === "object" && "status" in cause && Number((cause as { status?: number }).status) === 401) emit("auth-expired");
      error.value = cause instanceof Error ? cause.message : "Request failed";
    }
  }
}

watch(() => [scopeKey.value, props.member] as const, () => void load(), { immediate: true });
onUnmounted(() => { disposed = true; generation++; historyGeneration++; controller?.abort(); historyController?.abort(); });
</script>

<template>
  <section class="page-section narrow-page withdrawal-page">
    <div class="page-heading"><div><div class="eyebrow">POINTS</div><h1>{{ labels.title }}</h1><p>{{ labels.intro }}</p></div><button class="button button-secondary" :disabled="loading" @click="load">{{ loading ? labels.loading : labels.refresh }}</button></div>
    <p v-if="loading" class="detail-panel" role="status">{{ labels.loading }}</p>
    <div v-else-if="!member" class="detail-panel" role="status">{{ labels.signIn }}</div>
    <template v-else>
      <p v-if="error" class="auth-error" role="alert">{{ error }}</p>
      <div v-if="receipt" class="detail-panel success-note" role="status"><strong>{{ labels.receipt }} · {{ receipt.id }}</strong><p>{{ stateName(receipt) }}</p><p>{{ labels.refreshAfter }}</p><button class="button button-secondary" @click="receipt = null">{{ labels.close }}</button></div>
      <div v-if="uncertain && uncertain.scope === scopeKey" class="detail-panel demo-callout" role="alert"><p>{{ labels.unknown }}</p><button class="button button-secondary" :disabled="submitting" @click="submit(true)">{{ labels.replay }}</button></div>
      <div v-if="availability" class="detail-panel">
        <div class="eligibility"><div><span class="tiny-label">{{ labels.min }}</span><strong>{{ formatIntegerAmount(availability.min_points) }} pts</strong></div><div v-if="availability.max_points !== null"><span class="tiny-label">{{ labels.max }}</span><strong>{{ formatIntegerAmount(availability.max_points) }} pts</strong></div></div>
        <p v-if="availability.reason_code === 'WITHDRAWAL_ELIGIBILITY_NOT_CONFIGURED'" class="muted" role="status">{{ labels.noEligibility }}</p>
        <p v-else-if="availability.reason_code === 'WITHDRAWAL_DISABLED'" class="muted" role="status">{{ labels.disabled }}</p>
        <p v-else-if="availability.reason_code === 'WITHDRAWAL_ACCOUNT_RESTRICTED'" class="muted" role="status">{{ labels.restricted }}</p>
        <p class="muted">{{ labels.realPayments }}</p>
        <form @submit.prevent="submit()">
          <fieldset class="withdrawal-allocation" :disabled="!availability.can_apply || submitting || Boolean(uncertain)"><legend>{{ labels.available }}</legend>
            <label v-for="source in sources.filter((entry) => availability!.allowed_sources.includes(entry))" :key="source" class="field-label">{{ sourceName(source) }}<input v-model="allocations[source]" type="text" inputmode="numeric" pattern="[0-9]*" autocomplete="off" placeholder="0" :disabled="Boolean(uncertain) || submitting" /></label>
          </fieldset>
          <p class="muted">{{ labels.amount }}: {{ formatIntegerAmount(total.toString()) }} pts</p>
          <button class="button button-primary full-button" type="submit" :disabled="!validSubmission || submitting || Boolean(uncertain)">{{ submitting ? labels.loading : labels.submit }}</button>
        </form>
      </div>
      <div class="detail-panel"><h2>{{ labels.orders }}</h2><p v-if="!orders.length" class="muted">{{ labels.empty }}</p><article v-for="order in orders" :key="order.id" class="withdrawal-order"><div><strong>{{ formatIntegerAmount(order.points) }} pts</strong><span class="status-chip">{{ stateName(order) }}</span></div><small>{{ labels.date }} · {{ time(order.created_at) }}</small><button class="text-button" @click="loadHistory(order)">{{ labels.history }}</button><div v-if="history?.order_id === order.id" class="withdrawal-history"><button class="text-button" @click="history = null">{{ labels.close }}</button><div v-for="item in history.items" :key="item.id"><strong>{{ item.from_state ? labels.states[item.from_state] + ' → ' : '' }}{{ labels.states[item.to_state] }}</strong><small>{{ time(item.created_at) }} · {{ item.reason }}</small></div></div></article></div>
    </template>
  </section>
</template>

<style scoped>
.withdrawal-allocation{border:1px solid var(--line,#d9dedc);border-radius:12px;padding:16px;display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,180px),1fr));gap:12px;margin:16px 0}.withdrawal-allocation legend{padding:0 6px;font-weight:650}.withdrawal-allocation input{display:block;width:100%;margin-top:6px}.withdrawal-order{display:grid;grid-template-columns:1fr auto;gap:8px;padding:14px 0;border-bottom:1px solid var(--line,#d9dedc)}.withdrawal-order>div:first-child{display:flex;justify-content:space-between;gap:12px}.withdrawal-order small,.withdrawal-history small{color:var(--muted,#66716d)}.withdrawal-order>.text-button{justify-self:start}.withdrawal-history{grid-column:1/-1;padding:10px;background:var(--surface-soft,#f5f7f6);border-radius:8px}.withdrawal-history>div{display:grid;padding:8px 0}.withdrawal-page .auth-error{margin:10px 0}
</style>
