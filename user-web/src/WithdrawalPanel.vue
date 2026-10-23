<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import { useUserI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
import { formatDisplayDateTime } from "@lottery/shared";
import { vInlineValidation } from '@lottery/shared'
import DetailModal from "@lottery/shared/DetailModal.vue"
import { computed, onUnmounted, ref, watch } from "vue";
import { hasPendingWithdrawalIntent, isWithdrawalAllocation, retainWithdrawalIntent, type CreateWithdrawalBody, type WithdrawalAllocation, type WithdrawalAvailability, type WithdrawalHistory, type WithdrawalOrder, type WithdrawalQualification, type WithdrawalSource } from "@lottery/shared";
import { createWithdrawalOrdersApi, WithdrawalUnknownIntentError } from "./withdrawal-orders-api";
import { formatIntegerAmount } from "./wallet-api";
import { useUserI18n } from './i18n';

const props = defineProps<{
  locale: "en" | "zh";
  brandCode?: string;
  member: { id: string; brand_id: string } | null;
}>();
const emit = defineEmits<{ (event: "auth-expired"): void; (event: "context-changed"): void }>();

const api = computed(() => createWithdrawalOrdersApi({ brandCode: props.brandCode }));
const availability = ref<WithdrawalAvailability | null>(null);
const qualification = ref<WithdrawalQualification | null>(null);
const qualificationLoading = ref(false);
const qualificationError = ref<string | import("@lottery/shared").LocalizedMessage>("");
const orders = ref<WithdrawalOrder[]>([]);
const history = ref<WithdrawalHistory | null>(null);
const allocations = ref<Record<WithdrawalSource, string>>({ recharge: "", winning: "", gift: "", commission: "" });
const loading = ref(false);
const submitting = ref(false);
const error = ref<string | import("@lottery/shared").LocalizedMessage>("");
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
const sources: WithdrawalSource[] = ["recharge", "winning", "gift", "commission"];
const ui = useUserI18n().t;
const labels = computed(() => ({
  title: ui('user.withdrawal.title'), intro: ui('user.withdrawal.intro'), refresh: ui('user.withdrawal.refresh'), loading: ui('user.withdrawal.loading'), signIn: ui('user.withdrawal.signIn'), min: ui('user.withdrawal.min'), max: ui('user.withdrawal.max'), amount: ui('user.withdrawal.amount'), available: ui('user.withdrawal.available'), submit: ui('user.withdrawal.submit'), unavailable: ui('user.withdrawal.unavailable'), noEligibility: ui('user.withdrawal.noEligibility'), disabled: ui('user.withdrawal.disabled'), restricted: ui('user.withdrawal.restricted'), qualification: ui('user.withdrawal.qualification'), basePoints: ui('user.withdrawal.basePoints'), validPoints: ui('user.withdrawal.validPoints'), validOrders: ui('user.withdrawal.validOrders'), credit: ui('user.withdrawal.credit'), meetsTurnover: ui('user.withdrawal.meetsTurnover'), missesTurnover: ui('user.withdrawal.missesTurnover'), qualificationUnavailable: ui('user.withdrawal.qualificationUnavailable'), qualificationNote: ui('user.withdrawal.qualificationNote'), orders: ui('user.withdrawal.orders'), empty: ui('user.withdrawal.empty'), history: ui('user.withdrawal.history'), close: ui('user.withdrawal.close'), date: ui('user.withdrawal.date'), reason: ui('user.withdrawal.reason'), status: ui('user.withdrawal.status'), receipt: ui('user.withdrawal.receipt'), refreshAfter: ui('user.withdrawal.refreshAfter'), replay: ui('user.withdrawal.replay'), unknown: ui('user.withdrawal.unknown'), accountChanged: ui('user.withdrawal.accountChanged'), sourceSum: ui('user.withdrawal.sourceSum'), invalidAmount: ui('user.withdrawal.invalidAmount'), realPayments: ui('user.withdrawal.realPayments'), source: { recharge: ui('user.withdrawal.source.recharge'), winning: ui('user.withdrawal.source.winning'), gift: ui('user.withdrawal.source.gift'), commission: ui('user.withdrawal.source.commission') }, ordersLabel: ui('user.withdrawal.orders'), states: { reviewing: ui('user.withdrawal.state.reviewing'), processing: ui('user.withdrawal.state.processing'), paid: ui('user.withdrawal.state.paid'), rejected: ui('user.withdrawal.state.rejected'), failed: ui('user.withdrawal.state.failed'), cancelled: ui('user.withdrawal.state.cancelled') },
}));
const total = computed(() => {
  try { return sources.reduce((sum, source) => sum + BigInt(allocations.value[source].trim() || "0"), 0n); } catch { return -1n; }
});
const validSubmission = computed(() => {
  const current = availability.value;
  if (!current?.can_apply || !qualification.value?.meets_turnover || qualificationLoading.value || total.value <= 0n || total.value > 9223372036854775807n || uncertain.value) return false;
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
function time(value: string) { return formatDisplayDateTime(value, props.locale ?? "en"); }

async function load() {
  const requestGeneration = ++generation;
  controller?.abort();
  controller = new AbortController();
  const requestScope = scopeKey.value;
  const changedScope = lastScope !== requestScope;
  qualification.value = null;
  qualificationLoading.value = false;
  qualificationError.value = "";
  uncertain.value = retainWithdrawalIntent(uncertain.value, requestScope);
  if (changedScope) {
    uncertain.value = null;
    availability.value = null;
    orders.value = [];
    allocations.value = { recharge: "", winning: "", gift: "", commission: "" };
    submitting.value = false;
    receipt.value = null;
  }
  lastScope = requestScope;
  if (changedScope) availability.value = null;
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
    loading.value = false;
    if (snapshot.can_apply) {
      qualificationLoading.value = true;
      try {
        const preview = await api.value.qualification(controller.signal);
        if (disposed || requestGeneration !== generation || requestScope !== scopeKey.value) return;
        if (preview.member_id !== props.member.id || preview.brand_id !== props.member.brand_id) {
          availability.value = null;
          orders.value = [];
          qualification.value = null;
          qualificationError.value = "";
          emit("context-changed");
          return;
        }
        qualification.value = preview;
      } catch (cause) {
        if (disposed || requestGeneration !== generation || requestScope !== scopeKey.value || (cause instanceof DOMException && cause.name === "AbortError")) return;
        qualification.value = null;
        qualificationError.value = cause instanceof Error ? publicErrorMessage(cause) : labels.value.qualificationUnavailable;
        if (cause && typeof cause === "object" && "status" in cause && Number((cause as { status?: number }).status) === 401) {
          qualification.value = null;
          emit("auth-expired");
        }
      } finally {
        if (!disposed && requestGeneration === generation && requestScope === scopeKey.value) qualificationLoading.value = false;
      }
    }
  } catch (cause) {
    if (disposed || requestGeneration !== generation || requestScope !== scopeKey.value || (cause instanceof DOMException && cause.name === "AbortError")) return;
    availability.value = null;
    qualification.value = null;
    qualificationLoading.value = false;
    if (cause && typeof cause === "object" && "status" in cause && Number((cause as { status?: number }).status) === 401) {
      qualification.value = null;
      emit("auth-expired");
    }
    error.value = cause instanceof Error ? publicErrorMessage(cause) : ui('user.common.requestFailed');
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
    intent = Object.freeze({ scope: currentScope, body, key: crypto.randomUUID(), actorContext: availability.value.actor_context });
  }
  if (!intent || intent.scope !== currentScope) return;
  receipt.value = null;
  submitting.value = true;
  error.value = "";
  try {
    const order = await api.value.create(intent.body, intent.key, intent.actorContext);
    if (disposed || currentScope !== scopeKey.value) return;
    if (order.member_id !== props.member?.id || order.brand_id !== props.member?.brand_id) throw new WithdrawalUnknownIntentError("The server returned a receipt for a different account.");
    uncertain.value = null;
    qualification.value = null;
    receipt.value = order;
    allocations.value = { recharge: "", winning: "", gift: "", commission: "" };
  } catch (cause) {
    if (disposed || currentScope !== scopeKey.value) return;
    if (cause instanceof WithdrawalUnknownIntentError || (cause && typeof cause === "object" && "status" in cause && [0, 408, 429].includes(Number((cause as { status?: number }).status)) || cause instanceof Error && !("status" in cause))) {
      uncertain.value = intent;
      error.value = labels.value.unknown;
    } else if (cause && typeof cause === "object" && "code" in cause && (cause as { code?: string }).code === "WITHDRAWAL_CONFIRMATION_ACCOUNT_CHANGED") {
      uncertain.value = intent;
      error.value = labels.value.accountChanged;
      emit("context-changed");
    } else {
      const code = cause && typeof cause === "object" && "code" in cause ? String((cause as { code?: string }).code ?? "") : "";
      const status = cause && typeof cause === "object" && "status" in cause ? Number((cause as { status?: number }).status) : 0;
      if (status === 409 && /INELIGIBLE|TURNOVER.*EVIDENCE|EVIDENCE.*TURNOVER/i.test(code)) qualification.value = null;
      error.value = cause instanceof Error ? publicErrorMessage(cause) : ui('user.common.requestFailed');
    }
    if (cause && typeof cause === "object" && "status" in cause && Number((cause as { status?: number }).status) === 401) {
      qualification.value = null;
      emit("auth-expired");
    }
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
      if (cause && typeof cause === "object" && "status" in cause && Number((cause as { status?: number }).status) === 401) {
        qualification.value = null;
        emit("auth-expired");
      }
    error.value = cause instanceof Error ? publicErrorMessage(cause) : ui('user.common.requestFailed');
    }
  }
}

watch(() => [scopeKey.value, props.member] as const, () => void load(), { immediate: true });
onUnmounted(() => { disposed = true; generation++; historyGeneration++; controller?.abort(); historyController?.abort(); });
</script>

<template>
  <section class="page-section narrow-page withdrawal-page">
    <div class="page-heading"><div><div class="eyebrow">{{ ui('user.ui.points') }}</div><h1>{{ labels.title }}</h1></div><Teleport defer to="#user-page-refresh"><button v-refresh-button="loading ? labels.loading : labels.refresh" class="button button-secondary" :disabled="loading" @click="withGlobalLoading(load)">{{ loading ? labels.loading : labels.refresh }}</button></Teleport></div>
    <p v-global-loading v-if="loading" class="detail-panel" role="status">{{ labels.loading }}</p>
    <div v-else-if="!member" class="detail-panel" role="status">{{ labels.signIn }}</div>
    <template v-else>
      <p v-if="error" class="auth-error" role="alert">{{ displayPublicError(error) }}</p>
      <div v-if="receipt" class="detail-panel success-note" role="status"><strong>{{ labels.receipt }}</strong><p>{{ stateName(receipt) }}</p><p>{{ labels.refreshAfter }}</p><button class="button button-secondary" @click="receipt = null">{{ labels.close }}</button></div>
      <div v-if="uncertain && uncertain.scope === scopeKey" class="detail-panel demo-callout" role="alert"><p>{{ labels.unknown }}</p><button v-write-guard="submitting" class="button button-secondary" :disabled="submitting" @click="withGlobalLoading(submit, true)">{{ labels.replay }}</button></div>
      <div v-if="availability" class="detail-panel">
        <div class="eligibility"><div><span class="tiny-label">{{ labels.min }}</span><strong>{{ formatIntegerAmount(availability.min_points) }} pts</strong></div><div v-if="availability.max_points !== null"><span class="tiny-label">{{ labels.max }}</span><strong>{{ formatIntegerAmount(availability.max_points) }} pts</strong></div></div>
        <p v-if="availability.reason_code === 'WITHDRAWAL_ELIGIBILITY_NOT_CONFIGURED'" class="muted" role="status">{{ labels.noEligibility }}</p>
        <p v-else-if="availability.reason_code === 'WITHDRAWAL_DISABLED'" class="muted" role="status">{{ labels.disabled }}</p>
        <p v-else-if="availability.reason_code === 'WITHDRAWAL_ACCOUNT_RESTRICTED'" class="muted" role="status">{{ labels.restricted }}</p>
        <p class="muted">{{ labels.realPayments }}</p>
        <div v-if="availability.can_apply" class="qualification-snapshot" aria-live="polite">
          <h2>{{ labels.qualification }}</h2>
          <p v-global-loading v-if="qualificationLoading" class="muted" role="status">{{ labels.loading }}</p>
          <p v-else-if="qualificationError" class="muted" role="status">{{ labels.qualificationUnavailable }}</p>
          <template v-else-if="qualification">
            <dl>
              <div><dt>{{ labels.basePoints }}</dt><dd>{{ qualification.base_points }} pts</dd></div>
              <div><dt>{{ labels.validPoints }}</dt><dd>{{ qualification.valid_points }} pts</dd></div>
              <div><dt>{{ labels.validOrders }}</dt><dd>{{ qualification.valid_order_count }}</dd></div>
              <div><dt>{{ labels.credit }}</dt><dd>{{ qualification.credit_numerator }} / {{ qualification.credit_denominator }} pts</dd></div>
            </dl>
            <p class="muted" role="status">{{ qualification.meets_turnover ? labels.meetsTurnover : labels.missesTurnover }}</p>
            <p class="muted">{{ labels.qualificationNote }}</p>
          </template>
        </div>
        <form v-write-guard="submitting" novalidate v-inline-validation @submit.prevent="withGlobalLoading(submit)">
          <fieldset class="withdrawal-allocation" :disabled="!availability.can_apply || submitting || Boolean(uncertain)"><legend>{{ labels.available }}</legend>
            <label v-for="source in sources.filter((entry) => availability!.allowed_sources.includes(entry))" :key="source" class="field-label">{{ sourceName(source) }}<input v-model="allocations[source]" type="text" inputmode="numeric" pattern="[0-9]*" autocomplete="off" placeholder="0" :disabled="Boolean(uncertain) || submitting" /></label>
          </fieldset>
          <p class="muted">{{ labels.amount }}: {{ formatIntegerAmount(total.toString()) }} pts</p>
          <button class="button button-primary full-button" type="submit" :disabled="!validSubmission || submitting || Boolean(uncertain)">{{ submitting ? labels.loading : labels.submit }}</button>
        </form>
      </div>
      <div class="detail-panel">
        <h2>{{ labels.orders }}</h2>
        <p v-if="!orders.length" class="muted empty-state">{{ labels.empty }}</p>
        <article v-for="order in orders" :key="order.id" class="withdrawal-order">
          <div><strong>{{ formatIntegerAmount(order.points) }} pts</strong><span class="status-chip">{{ stateName(order) }}</span></div>
          <small>{{ labels.date }} · {{ time(order.created_at) }}</small>
          <button class="text-button" :aria-expanded="history?.order_id === order.id" :aria-controls="'withdrawal-history-' + order.id" @click="withGlobalLoading(loadHistory, order)">{{ labels.history }}</button>
          <DetailModal v-if="history?.order_id === order.id" :title="labels.history" :close-label="labels.close" @close="history = null">
<div v-if="history?.order_id === order.id" :id="'withdrawal-history-' + order.id" class="withdrawal-history" role="region" :aria-label="labels.history + ' · ' + order.id">
            <button class="text-button" @click="history = null">{{ labels.close }}</button>
            <div v-for="item in history.items" :key="item.id"><strong>{{ item.from_state ? labels.states[item.from_state] + ' → ' : '' }}{{ labels.states[item.to_state] }}</strong><small>{{ time(item.created_at) }} · {{ item.reason }}</small></div>
          </div>
</DetailModal>
        </article>
      </div>
    </template>
  </section>
</template>

<style scoped>
.withdrawal-page .text-button{min-height:44px;padding:8px 0;border:0;background:transparent;color:var(--brand-primary);font:inherit;font-size:14px;font-weight:650;text-align:left;cursor:pointer;justify-self:start}
.withdrawal-page .text-button:focus-visible{outline:2px solid var(--brand-primary);outline-offset:3px;border-radius:4px}
.qualification-snapshot{border-top:1px solid var(--line);margin-top:14px;padding-top:12px}.qualification-snapshot h2{font-size:1rem}.qualification-snapshot dl{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,180px),1fr));gap:8px 16px;margin:0}.qualification-snapshot dl>div{min-width:0}.qualification-snapshot dt{color:var(--muted);font-size:.85rem}.qualification-snapshot dd{margin:3px 0 0;overflow-wrap:anywhere;font-variant-numeric:tabular-nums}
.withdrawal-allocation{border:1px solid var(--line);border-radius:12px;padding:16px;display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,180px),1fr));gap:12px;margin:16px 0}.withdrawal-allocation legend{padding:0 6px;font-weight:650}.withdrawal-allocation input{display:block;width:100%;margin-top:6px}.withdrawal-order{display:grid;grid-template-columns:1fr auto;gap:8px;padding:14px 0;border-bottom:1px solid var(--line)}.withdrawal-order>div:first-child{display:flex;justify-content:space-between;gap:12px}.withdrawal-order small,.withdrawal-history small{color:var(--muted)}.withdrawal-order>.text-button{justify-self:start}.withdrawal-history{grid-column:1/-1;padding:10px;background:var(--surface-soft,#f5f7f6);border-radius:8px}.withdrawal-history>div{display:grid;padding:8px 0}.withdrawal-page .auth-error{margin:10px 0}
</style>
