<script setup lang="ts">
import { useUserI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
import { formatDisplayDateTime } from "@lottery/shared";
import DetailModal from "@lottery/shared/DetailModal.vue"
import { computed, onUnmounted, ref, watch } from "vue";
import { formatIntegerAmount } from "./wallet-api";
import { useUserI18n } from './i18n';
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
const errorText = ref<string | import("@lottery/shared").LocalizedMessage>("");
const detail = ref<MemberRecharge | null>(null);
const detailLoading = ref(false);
let generation = 0;
let detailGeneration = 0;
let listController: AbortController | null = null;
let detailController: AbortController | null = null;
let disposed = false;

const ui = useUserI18n().t;
const labels = computed(() => ({
  title: ui('user.recharge.title'), intro: ui('user.recharge.intro'), refresh: ui('user.recharge.refresh'), loading: ui('user.recharge.loading'), signIn: ui('user.recharge.signIn'), signInAction: ui('user.recharge.signInAction'), empty: ui('user.recharge.empty'), status: ui('user.recharge.status'), all: ui('user.recharge.all'), pending: ui('user.recharge.pending'), confirmed: ui('user.recharge.confirmed'), cancelled: ui('user.recharge.cancelled'), created: ui('user.recharge.created'), confirmedAt: ui('user.recharge.confirmedAt'), points: ui('user.recharge.points'), pointsUnit: ui('user.recharge.pointsUnit'), details: ui('user.recharge.details'), close: ui('user.recharge.close'), previous: ui('user.recharge.previous'), next: ui('user.recharge.next'), page: ui('user.recharge.page'), total: ui('user.recharge.total'), recordId: ui('user.recharge.recordId'), version: ui('user.recharge.version'), ledgerEntry: ui('user.recharge.ledgerEntry'), snapshot: ui('user.recharge.snapshot'), utc: ui('user.recharge.utc'), walletGuide: ui('user.recharge.walletGuide'), manual: ui('user.recharge.manual'), unavailable: ui('user.recharge.unavailable'), mismatch: ui('user.recharge.mismatch'), invalid: ui('user.recharge.invalid'),
  pendingState: ui('user.recharge.pending'), confirmedState: ui('user.recharge.confirmed'), cancelledState: ui('user.recharge.cancelled'),
}));
const error = computed(() => errorKey.value ? labels.value[errorKey.value] : errorText.value);

function statusName(state: RechargeState) { return labels.value[state]; }
function formatted(value: string) { return formatIntegerAmount(value); }
function date(value: string) { return formatDisplayDateTime(value, props.locale ?? "en"); }
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
    else if (cause instanceof Error) errorText.value = publicErrorMessage(cause);
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
    else if (cause instanceof Error) errorText.value = publicErrorMessage(cause);
    else errorKey.value = "invalid";
  } finally {
    if (!disposed && requestGeneration === detailGeneration && requestScope === scopeKey.value) detailLoading.value = false;
  }
}

async function nextPage() {
  if (!page.value || BigInt(offset.value + page.value.items.length) >= BigInt(page.value.total_count) || offset.value >= 1_000_000) return;
  offset.value += 20;
  await load();
}
async function previousPage() {
  if (offset.value <= 0) return;
  offset.value = Math.max(0, offset.value - 20);
  await load();
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
      <div><div class="eyebrow">{{ ui('user.ui.points') }}</div><h1>{{ labels.title }}</h1></div>
      <Teleport defer to="#user-page-refresh"><button v-refresh-button="loading ? labels.loading : labels.refresh" class="button button-secondary" :disabled="loading || !member" @click="withGlobalLoading(load)">{{ loading ? labels.loading : labels.refresh }}</button></Teleport>
    </div>
    <p class="muted" role="note">{{ labels.manual }}</p>
    <div v-if="!member" class="detail-panel" role="status"><p>{{ labels.signIn }}</p><button class="button button-secondary" @click="emit('sign-in')">{{ labels.signInAction }}</button><slot name="sign-in" /></div>
    <template v-else>
      <label class="field-label recharge-filter">{{ labels.status }}
        <select :value="selectedState" :disabled="loading" @change="selectedState = ($event.target as HTMLSelectElement).value as RechargeState | ''">
          <option value="">{{ labels.all }}</option><option value="pending">{{ labels.pending }}</option><option value="confirmed">{{ labels.confirmed }}</option><option value="cancelled">{{ labels.cancelled }}</option>
        </select>
      </label>
      <p v-global-loading v-if="loading" class="detail-panel" role="status">{{ labels.loading }}</p>
      <p v-if="error" class="auth-error" role="alert">{{ displayPublicError(error) }}</p>
      <template v-if="page">
        <div class="detail-panel recharge-list">
          <p class="muted recharge-total">{{ labels.total }} · {{ formatted(page.total_count) }}</p>
          <p class="muted recharge-snapshot">{{ labels.snapshot }} · {{ date(page.snapshot_at) }}</p>
          <p v-if="!items.length" class="muted empty-state" role="status">{{ labels.empty }}</p>
          <article v-for="item in items" :key="item.id" class="recharge-record">
            <div><strong>{{ formatted(item.points) }} {{ labels.pointsUnit }}</strong><span class="status-chip">{{ statusName(item.state) }}</span></div>
            <small>{{ labels.recordId }} · {{ item.id }}</small>
            <small>{{ labels.created }} · {{ date(item.created_at) }}</small>
            <button class="text-button" @click="withGlobalLoading(openDetail, item)">{{ labels.details }}</button>
          </article>
          <nav class="recharge-pagination" :aria-label="labels.page">
            <button class="button button-secondary" :disabled="loading || offset === 0" @click="withGlobalLoading(previousPage)">{{ labels.previous }}</button>
            <span>{{ labels.page }} {{ Math.floor(offset / 20) + 1 }}</span>
            <button class="button button-secondary" :disabled="loading || BigInt(offset + items.length) >= BigInt(page.total_count) || offset >= 1_000_000" @click="withGlobalLoading(nextPage)">{{ labels.next }}</button>
          </nav>
        </div>
        <div v-global-loading v-if="detailLoading" class="detail-panel" role="status">{{ labels.loading }}</div>
        <DetailModal v-if="!!detail" :title="labels.details" :close-label="labels.close" @close="detail = null">
<section v-if="detail" class="detail-panel recharge-detail" aria-live="polite">
          <div class="recharge-detail-heading"><h2>{{ labels.details }}</h2><button class="text-button" @click="detail = null">{{ labels.close }}</button></div>
          <dl><div><dt>{{ labels.recordId }}</dt><dd>{{ detail.id }}</dd></div><div><dt>{{ labels.points }}</dt><dd>{{ formatted(detail.points) }} {{ labels.pointsUnit }}</dd></div><div><dt>{{ labels.status }}</dt><dd>{{ statusName(detail.state) }}</dd></div><div><dt>{{ labels.created }}</dt><dd>{{ date(detail.created_at) }}</dd></div><div v-if="detail.confirmed_at"><dt>{{ labels.confirmedAt }}</dt><dd>{{ date(detail.confirmed_at) }}</dd></div><div v-if="detail.ledger_entry_id"><dt>{{ labels.ledgerEntry }}</dt><dd>{{ detail.ledger_entry_id }}</dd></div></dl>
        </section>
</DetailModal>
      </template>
    </template>
  </section>
</template>

<style scoped>
.recharge-filter { max-width: 260px; margin: 18px 0; }
.recharge-filter select { width: 100%; min-height: 42px; padding: 0 12px; border: 1px solid #e3e9e2; border-radius: 9px; background: var(--surface); color: #35483b; font: inherit; }
.recharge-page .text-button { min-height: 44px; padding: 8px 0; border: 0; background: transparent; color: var(--brand-primary); font: inherit; font-size: 14px; font-weight: 650; text-align: left; cursor: pointer; justify-self: start; }
.recharge-page .text-button:focus-visible { outline: 2px solid var(--brand-primary); outline-offset: 3px; border-radius: 4px; }
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
