<script setup lang="ts">
import { isPoints, pointsMinor, pointsFromMinor } from '@lottery/shared'

import { auditReasonLabel } from "@lottery/shared";
import { useOperationToast } from "@lottery/shared";
import { formValidationText } from '@lottery/shared'
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
import TablePagination from "@lottery/shared/TablePagination.vue"
import DetailModal from "@lottery/shared/DetailModal.vue"
import { computed, onUnmounted, ref, watch } from "vue";
import { hasPendingWithdrawalIntent, retainWithdrawalIntent, type CreateWithdrawalBody, type WithdrawalAvailability, type WithdrawalHistory, type WithdrawalOrder, type WithdrawalPage, type WithdrawalQualification, type WithdrawalSource } from "@lottery/shared";
import { createWithdrawalOrdersApi, WithdrawalUnknownIntentError } from "./withdrawal-orders-api";
import { formatIntegerAmount } from "./wallet-api";
import { useUserI18n } from './i18n';
const pageSize = ref(20)


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
const ordersPage = ref<WithdrawalPage | null>(null), offset = ref(0);
const history = ref<WithdrawalHistory | null>(null);
const amount = ref("");
const loading = ref(false);
const submitting = ref(false);
const applicationOpen = ref(false);
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
  pointsUnit: ui('user.recharge.pointsUnit'),
  title: ui('user.withdrawal.title'), refresh: ui('user.withdrawal.refresh'), loading: ui('user.withdrawal.loading'),
  signIn: ui('user.withdrawal.signIn'), amount: ui('user.withdrawal.amount'), submit: ui('user.withdrawal.submit'),
  noEligibility: ui('user.withdrawal.noEligibility'), disabled: ui('user.withdrawal.disabled'), restricted: ui('user.withdrawal.restricted'),
  empty: ui('user.withdrawal.empty'), history: ui('user.withdrawal.history'), close: ui('user.withdrawal.close'), date: ui('user.withdrawal.date'),
  receipt: ui('user.withdrawal.receipt'), replay: ui('user.withdrawal.replay'), unknown: ui('user.withdrawal.unknown'), accountChanged: ui('user.withdrawal.accountChanged'),
  states: { reviewing: ui('user.withdrawal.state.reviewing'), processing: ui('user.withdrawal.state.processing'), paid: ui('user.withdrawal.state.paid'), rejected: ui('user.withdrawal.state.rejected'), failed: ui('user.withdrawal.state.failed'), cancelled: ui('user.withdrawal.state.cancelled') },
}));
const total = computed(() => isPoints(amount.value.trim()) ? pointsMinor(amount.value.trim()) : -1n);
const availablePoints = computed(() => {
  const current = availability.value;
  if (!current?.can_apply || !qualification.value?.meets_turnover) return 0n;
  const balance = pointsMinor(current.available_points);
  const capped = current.max_points !== null && balance > pointsMinor(current.max_points) ? pointsMinor(current.max_points) : balance;
  return capped / pointsMinor(current.points_step) * pointsMinor(current.points_step);
});
const amountError = computed(() => {
  const current = availability.value;
  if (!current || !amount.value.trim()) return '';
  if (total.value <= 0n) return ui('user.withdrawal.positiveInteger');
  if (total.value < pointsMinor(current.min_points)) return ui('user.withdrawal.amountMinimum', { min: current.min_points });
  if (total.value % pointsMinor(current.points_step) !== 0n) return ui('user.withdrawal.stepHint', { step: current.points_step });
  if (total.value > availablePoints.value) return ui('user.withdrawal.exceedsAvailable');
  return '';
});
const validSubmission = computed(() => Boolean(availability.value?.can_apply && qualification.value?.meets_turnover && !qualificationLoading.value && !uncertain.value && total.value > 0n && !amountError.value && total.value <= availablePoints.value));
function openApplication(event: MouseEvent) {
  (event.currentTarget as HTMLButtonElement).focus({ preventScroll: true });
  applicationOpen.value = true;
}
function stateName(order: WithdrawalOrder) { return labels.value.states[order.state]; }
function time(value: string) { return formatDisplayDateTime(value, props.locale ?? "en"); }

async function load(nextOffset = offset.value) {
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
    applicationOpen.value = false;
    availability.value = null;
    orders.value = []; ordersPage.value = null; offset.value = 0;
    amount.value = "";
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
      api.value.availability(controller.signal), api.value.list(changedScope ? 0 : nextOffset, controller.signal, pageSize.value),
    ]);
    if (disposed || requestGeneration !== generation || requestScope !== scopeKey.value) return;
    if (snapshot.member_id !== props.member.id || snapshot.brand_id !== props.member.brand_id || page.brand_id !== props.member.brand_id || page.items.some((item) => item.member_id !== props.member!.id || item.brand_id !== props.member!.brand_id)) throw new Error("The server returned withdrawal data for a different account.");
    availability.value = snapshot;
    orders.value = page.items; ordersPage.value = page; offset.value = page.offset;
    loading.value = false;
    if (snapshot.can_apply) {
      qualificationLoading.value = true;
      try {
        const preview = await api.value.qualification(controller.signal);
        if (disposed || requestGeneration !== generation || requestScope !== scopeKey.value) return;
        if (preview.member_id !== props.member.id || preview.brand_id !== props.member.brand_id) {
          availability.value = null;
          orders.value = []; ordersPage.value = null;
          qualification.value = null;
          qualificationError.value = "";
          emit("context-changed");
          return;
        }
        qualification.value = preview;
      } catch (cause) {
        if (disposed || requestGeneration !== generation || requestScope !== scopeKey.value || (cause instanceof DOMException && cause.name === "AbortError")) return;
        qualification.value = null;
        qualificationError.value = cause instanceof Error ? publicErrorMessage(cause, 'read') : ui('user.withdrawal.qualificationFailed');
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
    qualificationLoading.value = false;
    if (cause && typeof cause === "object" && "status" in cause && Number((cause as { status?: number }).status) === 401) {
      availability.value=null;qualification.value=null;orders.value=[];
      orders.value = []; ordersPage.value = null;
      emit("auth-expired");
    }
    error.value = cause instanceof Error ? publicErrorMessage(cause, 'read') : ui('user.common.requestFailed');
  } finally {
    if (!disposed && requestGeneration === generation && requestScope === scopeKey.value) loading.value = false;
  }
}

function buildBody(): CreateWithdrawalBody | null {
  const current = availability.value;
  if (!current || !validSubmission.value) return null;
  let remaining = total.value;
  const source_allocation: CreateWithdrawalBody['source_allocation'] = [];
  for (const source of sources) {
    const balance = current.source_balances.find(item => item.source === source);
    if (!balance || remaining === 0n) continue;
    const points = pointsMinor(balance.points) < remaining ? pointsMinor(balance.points) : remaining;
    source_allocation.push({ source, state: 'available', points: pointsFromMinor(points) });
    remaining -= points;
  }
  return remaining === 0n ? { points: pointsFromMinor(total.value), source_allocation } : null;
}

async function submit(replay = false) {
  const currentScope = scopeKey.value;
  if (submitting.value || !currentScope) return;
  let intent = uncertain.value;
  if (replay) {
    if (!intent || intent.scope !== currentScope) return;
  } else {
    if (hasPendingWithdrawalIntent(intent) || !availability.value?.can_apply) return;
    if (!validSubmission.value) { error.value = amountError.value || ui('user.withdrawal.unavailable'); return; }
    const body = buildBody();
    if (!body) { error.value = ui('user.withdrawal.exceedsAvailable'); return; }
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
    applicationOpen.value = false;
    amount.value = "";
    await load(0);
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
      availability.value=null;qualification.value=null;orders.value=[];
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
    error.value = cause instanceof Error ? publicErrorMessage(cause, 'read') : ui('user.common.requestFailed');
    }
  }
}

watch(() => [scopeKey.value, props.member] as const, () => void load(), { immediate: true });
onUnmounted(() => { disposed = true; generation++; historyGeneration++; controller?.abort(); historyController?.abort(); });

useOperationToast(error, value => displayPublicError(value), 'error');
useOperationToast(receipt, order => `${labels.value.receipt} · ${stateName(order)}`, 'success');
</script>

<template>
  <section class="page-section narrow-page withdrawal-page">
    <Teleport v-if="error || orders.some(order => order.state === 'reviewing' || order.state === 'processing')" defer to="#user-page-refresh"><button type="button" v-refresh-button="loading ? labels.loading : labels.refresh" class="button button-secondary" :disabled="loading" @click="withGlobalLoading(load)">{{ loading ? labels.loading : labels.refresh }}</button></Teleport>
    <p v-global-loading v-if="loading && !ordersPage" class="detail-panel" role="status">{{ labels.loading }}</p>
    <div v-else-if="!member" class="detail-panel" role="status">{{ labels.signIn }}</div>
    <template v-else>
      <div class="withdrawal-toolbar"><button type="button" class="button button-primary" :disabled="!availability || submitting" @click="openApplication">{{ ui('user.withdrawal.action') }}</button></div>
      <div v-if="!applicationOpen && uncertain && uncertain.scope === scopeKey" class="detail-panel demo-callout" role="alert"><p>{{ labels.unknown }}</p><button type="button" v-write-guard="submitting" class="button button-secondary" :disabled="submitting" @click="withGlobalLoading(submit, true)">{{ labels.replay }}</button></div>
      <DetailModal v-if="applicationOpen && availability" class="withdrawal-application ui-form-dialog" :title="labels.title" :close-label="labels.close" :close-disabled="submitting" @close="applicationOpen = false">
      <div class="withdrawal-application-body">
        <div v-if="uncertain && uncertain.scope === scopeKey" class="demo-callout" role="alert"><p>{{ labels.unknown }}</p><button type="button" v-write-guard="submitting" class="button button-secondary" :disabled="submitting" @click="withGlobalLoading(submit, true)">{{ labels.replay }}</button></div>
        <div class="withdrawal-summary">
          <div><span class="tiny-label">{{ ui('user.withdrawal.availablePoints') }}</span><strong>{{ formatIntegerAmount(pointsFromMinor(availablePoints)) }} {{ labels.pointsUnit }}</strong></div>
          <div><span class="tiny-label">{{ ui('user.withdrawal.minimumPoints') }}</span><strong>{{ formatIntegerAmount(availability.min_points) }} {{ labels.pointsUnit }}</strong></div>
        </div>
        <p v-if="availability.reason_code === 'WITHDRAWAL_ELIGIBILITY_NOT_CONFIGURED'" class="muted" role="status">{{ labels.noEligibility }}</p>
        <p v-else-if="availability.reason_code === 'WITHDRAWAL_DISABLED'" class="muted" role="status">{{ labels.disabled }}</p>
        <p v-else-if="availability.reason_code === 'WITHDRAWAL_ACCOUNT_RESTRICTED'" class="muted" role="status">{{ labels.restricted }}</p>
        <p v-else-if="qualificationLoading" v-global-loading class="muted" role="status">{{ labels.loading }}</p>
        <p v-else-if="qualificationError" class="muted" role="status">{{ ui('user.withdrawal.qualificationFailed') }}</p>
        <p v-else-if="qualification && !qualification.meets_turnover" class="muted" role="status">{{ ui('user.withdrawal.turnoverRequired') }}</p>
        <form v-write-guard="submitting" novalidate v-inline-validation class="withdrawal-form ui-form" @submit.prevent="withGlobalLoading(submit)">
          <label class="field-label ui-field-row" for="withdrawal-amount"><span>{{ labels.amount }}</span><input id="withdrawal-amount" :aria-describedby="pointsMinor(availability.points_step) > 100n ? 'withdrawal-step-hint' : undefined" v-model="amount" type="text" inputmode="decimal" pattern="(0|[1-9][0-9]*)([.][0-9]{1,2})?" :data-validation-format="formValidationText('positivePoints')" autocomplete="off" :placeholder="ui('user.withdrawal.pointsPlaceholder')" :disabled="!availability.can_apply || Boolean(uncertain) || submitting" /></label>
          <p v-if="pointsMinor(availability.points_step) > 100n" id="withdrawal-step-hint" class="muted">{{ ui('user.withdrawal.stepHint', { step: availability.points_step }) }}</p>
          <p v-if="amountError" class="form-error" role="alert">{{ amountError }}</p>
          <div class="ui-form-actions"><button class="button button-primary" type="submit" :disabled="!validSubmission || submitting">{{ submitting ? labels.loading : labels.submit }}</button></div>
        </form>
      </div>
      </DetailModal>
      <div class="withdrawal-records user-record-list" :aria-busy="loading">
        <p v-if="ordersPage && !orders.length" class="muted empty-state">{{ labels.empty }}</p>
        <article v-for="order in orders" :key="order.id" class="withdrawal-order">
          <div><strong>{{ formatIntegerAmount(order.points) }} {{ labels.pointsUnit }}</strong><span class="status-chip">{{ stateName(order) }}</span></div>
          <small>{{ labels.date }} · {{ time(order.created_at) }}</small>
          <button type="button" class="text-button" tabindex="0" aria-haspopup="dialog" :aria-expanded="history?.order_id === order.id" :aria-controls="'withdrawal-history-' + order.id" @click="withGlobalLoading(loadHistory, order)">{{ labels.history }}</button>
          <DetailModal v-if="history?.order_id === order.id" :title="labels.history" :close-label="labels.close" @close="history = null">
<div v-if="history?.order_id === order.id" :id="'withdrawal-history-' + order.id" class="withdrawal-history" role="region" :aria-label="labels.history + ' · ' + order.id">
            <div v-for="item in history.items" :key="item.id"><strong>{{ item.from_state ? labels.states[item.from_state] + ' → ' : '' }}{{ labels.states[item.to_state] }}</strong><small>{{ time(item.created_at) }} · {{ auditReasonLabel(item.reason, publicErrorI18n.locale.value) }}</small></div>
          </div>
</DetailModal>
        </article>
        <TablePagination v-if="ordersPage" :total="ordersPage.total_count" :page="Math.floor(offset / pageSize + 1)" :count="orders.length" :locale="publicErrorI18n.locale.value" :page-size="pageSize" :previous-disabled="loading || offset === 0" :next-disabled="loading || !ordersPage.has_more" :go-to="(target: number) => load((target - 1) * pageSize)" :change-page-size="async (size: number) => { pageSize = size; await load(0) }" :busy="loading" />
      </div>
    </template>
  </section>
</template>

<style scoped>


.withdrawal-order{display:grid;grid-template-columns:1fr auto;gap:8px;padding:14px 0;border-bottom:1px solid var(--line)}.withdrawal-order>div:first-child{display:flex;justify-content:space-between;gap:12px}.withdrawal-order small,.withdrawal-history small{color:var(--muted)}.withdrawal-order>.text-button{justify-self:start}.withdrawal-history{grid-column:1/-1;padding:10px;background:var(--canvas);border-radius:8px}.withdrawal-history>div{display:grid;padding:8px 0}.withdrawal-page .auth-error{margin:10px 0}

.withdrawal-page{display:grid;gap:12px}
.withdrawal-toolbar{display:flex;justify-content:flex-end}

.withdrawal-application-body{min-width:0}
.withdrawal-order{grid-template-columns:minmax(0,1fr) auto}
.withdrawal-order>div:first-child{flex-wrap:wrap}
.withdrawal-order small{overflow-wrap:anywhere}
@media(max-width:700px){.withdrawal-order{grid-template-columns:minmax(0,1fr)}}

.withdrawal-summary{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px;padding-bottom:16px;border-bottom:1px solid var(--line)}
.withdrawal-summary>div{display:grid;gap:6px;min-width:0}
.withdrawal-summary strong{font-size:20px;color:var(--ink);overflow-wrap:anywhere;font-variant-numeric:tabular-nums}
.withdrawal-summary>div:first-child strong{color:var(--primary-dark)}
.withdrawal-form{display:grid;gap:12px;margin-top:16px}
.withdrawal-form .field-label{display:grid;gap:6px}
.withdrawal-form input{width:100%;min-width:0;box-sizing:border-box}
.withdrawal-form p{margin:0}
.withdrawal-form button{justify-self:end}
@media (max-width:900px), (pointer:coarse) {
  .withdrawal-summary { grid-template-columns: minmax(0,1fr); gap: 4px; padding-bottom: 8px; }
  .withdrawal-summary > div { display:flex; align-items:baseline; justify-content:space-between; gap:8px; flex-wrap:wrap; }
  .withdrawal-summary strong { font-size:var(--body-font-size); }
  .withdrawal-form { margin-top:8px; }
}
</style>
