<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { formatIntegerAmount } from "./wallet-api";
import { createRechargeApi, RechargeApiError, type MemberRecharge, type RechargeState } from "./recharge-api";

const props = defineProps<{
  brandCode?: string;
  locale?: "en" | "zh" | "zh-CN";
  member: { id: string; brand_id: string } | null;
}>();
const emit = defineEmits<{ (event: "auth-expired"): void; (event: "sign-in"): void }>();

const api = computed(() => createRechargeApi({ brandCode: props.brandCode }));
const scopeKey = computed(() => props.member ? `${props.brandCode ?? ""}:${props.member.brand_id}:${props.member.id}` : "");
const page = ref<Awaited<ReturnType<ReturnType<typeof createRechargeApi>["list"]>> | null>(null);
const items = computed(() => page.value?.items ?? []);
const selectedState = ref<RechargeState | "">("");
const offset = ref(0);
const loading = ref(false);
const errorKey = ref<"unavailable" | "mismatch" | "invalid" | "">("");
const errorText = ref("");
const detail = ref<MemberRecharge | null>(null);
const detailLoading = ref(false);
let generation = 0;
let detailGeneration = 0;
let listController: AbortController | null = null;
let detailController: AbortController | null = null;
let disposed = false;

const labels = computed(() => props.locale === "en" ? {
  title: "Recharge records", intro: "View recharge records handled by the service team. This page does not make or accept payments.", refresh: "Refresh", loading: "Loading…", signIn: "Sign in to view your recharge records.", signInAction: "Sign in", empty: "No recharge records yet.", status: "Status", all: "All statuses", pending: "Pending", confirmed: "Confirmed", cancelled: "Cancelled", created: "Created", confirmedAt: "Confirmed", points: "Points", pointsUnit: "pts", details: "Details", close: "Close details", previous: "Previous", next: "Next", page: "Page", total: "Total records", recordId: "Record ID", version: "Version", ledgerEntry: "Ledger entry", snapshot: "Queried at", utc: "UTC", walletGuide: "For current available points, check your points wallet. This list is not payment evidence.", manual: "Recharges are recorded manually by the service team. No real payment is initiated here. Records show current status only; they are not a history snapshot, payment receipt, or current balance.", unavailable: "Recharge records are temporarily unavailable.", mismatch: "Your sign-in may have changed. Sign in again to view the correct account.", invalid: "Recharge records could not be loaded.",
} : {
  title: "充值记录", intro: "查看由服务团队处理的充值记录。此页面不会发起或接收付款。", refresh: "刷新", loading: "正在加载…", signIn: "请登录以查看充值记录。", signInAction: "登录", empty: "暂无充值记录。", status: "状态", all: "全部状态", pending: "待处理", confirmed: "已确认", cancelled: "已取消", created: "创建时间", confirmedAt: "确认时间", points: "积分", pointsUnit: "积分", details: "详情", close: "关闭详情", previous: "上一页", next: "下一页", page: "页码", total: "记录总数", recordId: "记录编号", version: "版本", ledgerEntry: "账本条目", snapshot: "查询时间", utc: "UTC", walletGuide: "当前可用积分请查看积分钱包。本列表不是付款凭证。", manual: "充值由服务团队人工记录处理。本页面不会发起真实付款。记录仅显示当前状态，不是历史快照、付款回执或当前余额。", unavailable: "暂时无法读取充值记录。", mismatch: "登录身份可能已变化。请重新登录以查看正确账户。", invalid: "无法读取充值记录。",
});
const error = computed(() => errorKey.value ? labels.value[errorKey.value] : errorText.value);

function statusName(state: RechargeState) { return labels.value[state]; }
function formatted(value: string) { return formatIntegerAmount(value); }
function date(value: string) {
  return new Intl.DateTimeFormat(props.locale === "en" ? "en" : "zh-CN", { dateStyle: "medium", timeStyle: "short", timeZone: "UTC" }).format(new Date(value));
}
function isScopeMismatch(cause: unknown) { return cause instanceof RechargeApiError && cause.code === "SCOPE_MISMATCH"; }
function isUnauthorized(cause: unknown) { return cause instanceof RechargeApiError && cause.status === 401; }
function isInvalidResponse(cause: unknown) { return cause instanceof RechargeApiError && cause.code === "INVALID_RESPONSE"; }
function isUnavailable(cause: unknown) { return cause instanceof RechargeApiError && cause.status === 0; }
function clearVisible() {
  generation++;
  page.value = null;
  detail.value = null;
  detailLoading.value = false;
  loading.value = false;
  offset.value = 0;
  errorKey.value = "";
  errorText.value = "";
  listController?.abort();
  detailController?.abort();
  detailGeneration++;
}

async function load(resetOffset = false) {
  const requestGeneration = ++generation;
  listController?.abort();
  listController = new AbortController();
  const requestScope = scopeKey.value;
  const member = props.member ? { id: props.member.id, brand_id: props.member.brand_id } : null;
  if (resetOffset) offset.value = 0;
  detailController?.abort();
  detailGeneration++;
  detail.value = null;
  detailLoading.value = false;
  errorKey.value = "";
  errorText.value = "";
  if (!requestScope || !member) {
    clearVisible();
    loading.value = false;
    return;
  }
  loading.value = true;
  try {
    const result = await api.value.list({ brand_id: member.brand_id, member_id: member.id }, { limit: 20, offset: offset.value, state: selectedState.value || null }, listController.signal);
    if (disposed || requestGeneration !== generation || requestScope !== scopeKey.value) return;
    page.value = result;
  } catch (cause) {
    if (disposed || requestGeneration !== generation || requestScope !== scopeKey.value || (cause instanceof DOMException && cause.name === "AbortError")) return;
    page.value = null;
    if (isUnauthorized(cause) || isScopeMismatch(cause)) {
      emit("auth-expired");
      errorKey.value = "mismatch";
    } else if (isInvalidResponse(cause)) errorKey.value = "invalid";
    else if (isUnavailable(cause)) errorKey.value = "unavailable";
    else if (cause instanceof Error) errorText.value = cause.message;
    else errorKey.value = "unavailable";
  } finally {
    if (!disposed && requestGeneration === generation && requestScope === scopeKey.value) loading.value = false;
  }
}

async function openDetail(item: MemberRecharge) {
  const requestGeneration = ++detailGeneration;
  detailController?.abort();
  detailController = new AbortController();
  const requestScope = scopeKey.value;
  const expectedId = item.id;
  const member = props.member ? { id: props.member.id, brand_id: props.member.brand_id } : null;
  detail.value = null;
  detailLoading.value = false;
  if (!member || item.member_id !== member.id || item.brand_id !== member.brand_id) {
    clearVisible();
    emit("auth-expired");
    errorKey.value = "mismatch";
    return;
  }
  detailLoading.value = true;
  errorKey.value = "";
  errorText.value = "";
  try {
    const result = await api.value.get({ brand_id: member.brand_id, member_id: member.id }, expectedId, detailController.signal);
    if (disposed || requestGeneration !== detailGeneration || requestScope !== scopeKey.value) return;
    detail.value = result;
  } catch (cause) {
    if (disposed || requestGeneration !== detailGeneration || requestScope !== scopeKey.value || (cause instanceof DOMException && cause.name === "AbortError")) return;
    if (isUnauthorized(cause) || isScopeMismatch(cause)) {
      clearVisible();
      emit("auth-expired");
      errorKey.value = "mismatch";
    } else if (isInvalidResponse(cause)) errorKey.value = "invalid";
    else if (isUnavailable(cause)) errorKey.value = "unavailable";
    else if (cause instanceof Error) errorText.value = cause.message;
    else errorKey.value = "invalid";
  } finally {
    if (!disposed && requestGeneration === detailGeneration && requestScope === scopeKey.value) detailLoading.value = false;
  }
}

function nextPage() {
  if (!page.value || BigInt(offset.value + page.value.items.length) >= BigInt(page.value.total_count) || offset.value >= 1_000_000) return;
  offset.value += 20;
  void load();
}
function previousPage() {
  if (offset.value <= 0) return;
  offset.value = Math.max(0, offset.value - 20);
  void load();
}

watch(scopeKey, () => {
  generation++;
  clearVisible();
  loading.value = false;
  void load(true);
}, { immediate: true });
watch(selectedState, () => {
  if (scopeKey.value) void load(true);
});
onUnmounted(() => {
  disposed = true;
  generation++;
  detailGeneration++;
  listController?.abort();
  detailController?.abort();
});
</script>

<template>
  <section class="page-section narrow-page recharge-page">
    <div class="page-heading">
      <div><div class="eyebrow">POINTS</div><h1>{{ labels.title }}</h1><p>{{ labels.intro }}</p></div>
      <button class="button button-secondary" :disabled="loading || !member" @click="load()">{{ loading ? labels.loading : labels.refresh }}</button>
    </div>
    <p class="muted" role="note">{{ labels.manual }}</p>
    <div v-if="!member" class="detail-panel" role="status"><p>{{ labels.signIn }}</p><button class="button button-secondary" @click="emit('sign-in')">{{ labels.signInAction }}</button><slot name="sign-in" /></div>
    <template v-else>
      <label class="field-label recharge-filter">{{ labels.status }}
        <select :value="selectedState" :disabled="loading" @change="selectedState = ($event.target as HTMLSelectElement).value as RechargeState | ''">
          <option value="">{{ labels.all }}</option><option value="pending">{{ labels.pending }}</option><option value="confirmed">{{ labels.confirmed }}</option><option value="cancelled">{{ labels.cancelled }}</option>
        </select>
      </label>
      <p v-if="loading" class="detail-panel" role="status">{{ labels.loading }}</p>
      <p v-if="error" class="auth-error" role="alert">{{ error }}</p>
      <template v-if="page">
        <div class="detail-panel recharge-list">
          <p class="muted recharge-total">{{ labels.total }} · {{ formatted(page.total_count) }}</p>
          <p class="muted recharge-snapshot">{{ labels.snapshot }} · {{ date(page.snapshot_at) }} {{ labels.utc }}</p>
          <p class="muted recharge-wallet-guide">{{ labels.walletGuide }}</p>
          <p v-if="!items.length" class="muted" role="status">{{ labels.empty }}</p>
          <article v-for="item in items" :key="item.id" class="recharge-record">
            <div><strong>{{ formatted(item.points) }} {{ labels.pointsUnit }}</strong><span class="status-chip">{{ statusName(item.state) }}</span></div>
            <small>{{ labels.recordId }} · {{ item.id }}</small>
            <small>{{ labels.created }} · {{ date(item.created_at) }} {{ labels.utc }}</small>
            <button class="text-button" @click="openDetail(item)">{{ labels.details }}</button>
          </article>
          <nav class="recharge-pagination" :aria-label="labels.page">
            <button class="button button-secondary" :disabled="loading || offset === 0" @click="previousPage">{{ labels.previous }}</button>
            <span>{{ labels.page }} {{ Math.floor(offset / 20) + 1 }}</span>
            <button class="button button-secondary" :disabled="loading || BigInt(offset + items.length) >= BigInt(page.total_count) || offset >= 1_000_000" @click="nextPage">{{ labels.next }}</button>
          </nav>
        </div>
        <div v-if="detailLoading" class="detail-panel" role="status">{{ labels.loading }}</div>
        <section v-if="detail" class="detail-panel recharge-detail" aria-live="polite">
          <div class="recharge-detail-heading"><h2>{{ labels.details }}</h2><button class="text-button" @click="detail = null">{{ labels.close }}</button></div>
          <dl><div><dt>{{ labels.recordId }}</dt><dd>{{ detail.id }}</dd></div><div><dt>{{ labels.points }}</dt><dd>{{ formatted(detail.points) }} {{ labels.pointsUnit }}</dd></div><div><dt>{{ labels.status }}</dt><dd>{{ statusName(detail.state) }}</dd></div><div><dt>{{ labels.version }}</dt><dd>{{ detail.version }}</dd></div><div><dt>{{ labels.created }}</dt><dd>{{ date(detail.created_at) }} {{ labels.utc }}</dd></div><div v-if="detail.confirmed_at"><dt>{{ labels.confirmedAt }}</dt><dd>{{ date(detail.confirmed_at) }} {{ labels.utc }}</dd></div><div v-if="detail.ledger_entry_id"><dt>{{ labels.ledgerEntry }}</dt><dd>{{ detail.ledger_entry_id }}</dd></div></dl>
        </section>
      </template>
    </template>
  </section>
</template>

<style scoped>
.recharge-filter { max-width: 260px; margin: 18px 0; }
.recharge-filter select { width: 100%; min-height: 42px; padding: 0 12px; border: 1px solid #e3e9e2; border-radius: 9px; background: #fff; color: #35483b; font: inherit; }
.recharge-page .text-button { min-height: 44px; padding: 8px 0; border: 0; background: transparent; color: var(--brand-primary, #20594c); font: inherit; font-size: 14px; font-weight: 650; text-align: left; cursor: pointer; justify-self: start; }
.recharge-page .text-button:focus-visible { outline: 2px solid var(--brand-primary, #20594c); outline-offset: 3px; border-radius: 4px; }
.recharge-total { margin-top: 0; }
.recharge-record { display: grid; gap: 7px; padding: 15px 0; border-top: 1px solid #e9ede8; }
.recharge-record > div { display: flex; align-items: center; gap: 10px; }
.recharge-record small { color: #69766e; }
.recharge-pagination, .recharge-detail-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-top: 16px; }
.recharge-detail dl { display: grid; gap: 12px; }
.recharge-detail dl > div { display: flex; justify-content: space-between; gap: 16px; }
.recharge-detail dt { color: #69766e; }
.recharge-detail dd { margin: 0; text-align: right; }
</style>
