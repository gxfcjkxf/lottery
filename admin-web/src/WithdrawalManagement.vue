<script setup lang="ts">
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import SiteIcon from "@lottery/shared/SiteIcon";
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import { vInlineValidation } from '@lottery/shared'
import TablePagination from "@lottery/shared/TablePagination.vue"
import DetailModal from "@lottery/shared/DetailModal.vue"
import { computed, onUnmounted, ref, watch } from "vue";
import { hasPendingWithdrawalIntent, retainWithdrawalIntent, type WithdrawalAction, type WithdrawalActionBody, type WithdrawalHistory, type WithdrawalOrder, type WithdrawalState } from "@lottery/shared";
import type { AdminAccount } from "./admin-api";
import { createIdempotencyKey } from "./admin-api";
import { createWithdrawalOrdersAdminApi, MAX_WITHDRAWAL_PAGE, withdrawalPermissions, WithdrawalAdminUnknownIntentError } from "./withdrawal-orders-api";
import { useAdminI18n } from "./i18n";
import { lookupAdminMember } from "./member-lookup";

const props = defineProps<{ account: AdminAccount; brandId: string; active: boolean }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { locale, t, display } = useAdminI18n();
const api = createWithdrawalOrdersAdminApi();
const page = ref<WithdrawalOrder[]>([]);
const stateFilter = ref<WithdrawalState | "">("");
const memberFilter = ref("");
const offset = ref(0);
const hasMore = ref(false);
const loading = ref(false);
const busy = ref(false);
const error = ref<string | import("@lottery/shared").LocalizedMessage>("");
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
const words = computed(() => ({
  title: t('admin.withdrawal.title'), intro: t('admin.withdrawal.intro'), refresh: t('admin.withdrawal.refresh'), loading: t('admin.withdrawal.loading'), noView: t('admin.withdrawal.noView'), noBrand: t('admin.withdrawal.noBrand'), empty: t('admin.withdrawal.empty'), member: t('admin.withdrawal.member'), state: t('admin.withdrawal.state'), filter: t('admin.withdrawal.filter'), all: t('admin.withdrawal.all'), order: t('admin.withdrawal.order'), points: t('admin.withdrawal.points'), created: t('admin.withdrawal.created'), history: t('admin.withdrawal.history'), close: t('admin.withdrawal.close'), reason: t('admin.withdrawal.reason'), reasonPlaceholder: t('admin.withdrawal.reasonPlaceholder'), confirm: t('admin.withdrawal.confirm'), cancel: t('admin.withdrawal.cancel'), approve: t('admin.withdrawal.approve'), reject: t('admin.withdrawal.reject'), fail: t('admin.withdrawal.fail'), paid: t('admin.withdrawal.paid'), internal: t('admin.withdrawal.internal'), receipt: t('admin.withdrawal.receipt'), receiptNote: t('admin.withdrawal.receiptNote'), unknown: t('admin.withdrawal.unknown'), replay: t('admin.withdrawal.replay'), acted: t('admin.withdrawal.acted'), actors: t('admin.withdrawal.actors'), previous: t('admin.withdrawal.previous'), next: t('admin.withdrawal.next'), statuses: { reviewing: t('admin.withdrawal.reviewing'), processing: t('admin.withdrawal.processing'), paid: t('admin.withdrawal.paidState'), rejected: t('admin.withdrawal.rejected'), failed: t('admin.withdrawal.failed'), cancelled: t('admin.withdrawal.cancelled') }, eyebrow: t('admin.withdrawal.eyebrow'), filterPlaceholder: t('admin.withdrawal.filterPlaceholder'), details: t('admin.withdrawal.details'), requestFailed: t('admin.withdrawal.requestFailed')
}));
const states: WithdrawalState[] = ["reviewing", "processing", "paid", "rejected", "failed", "cancelled"];
const stateName = (state: WithdrawalState) => words.value.statuses[state];
const actionsFor = (order: WithdrawalOrder): WithdrawalAction[] => order.state === "reviewing" ? ["approve", "reject", "cancel"] : order.state === "processing" ? ["fail", "mark-paid", "cancel"] : [];
const actionName = (action: WithdrawalAction) => ({ approve: words.value.approve, reject: words.value.reject, cancel: words.value.cancel, fail: words.value.fail, "mark-paid": words.value.paid })[action];
function date(value: string){ return formatDisplayDateTime(value, displayLocale.value); }

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
    const username = memberFilter.value.trim();
    const memberId = username ? (await lookupAdminMember(props.brandId, username)).id : undefined;
    const result = await api.list(props.brandId, { state: stateFilter.value || undefined, memberId, limit: MAX_WITHDRAWAL_PAGE, offset: nextOffset }, controller.signal);
    if (disposed || requestGeneration !== generation || requestScope !== scope.value) return;
    if (result.brand_id !== props.brandId || result.items.some((item) => item.brand_id !== props.brandId)) throw new Error("The server returned withdrawal orders for another brand.");
    page.value = result.items;
    hasMore.value = result.has_more;
  } catch (cause) {
    if (disposed || requestGeneration !== generation || requestScope !== scope.value || cause instanceof DOMException && cause.name === "AbortError") return;
    if (cause && typeof cause === "object" && "status" in cause && Number((cause as {status?:number}).status) === 401) emit("session-invalid");
    error.value = cause instanceof Error ? publicErrorMessage(cause) : words.value.requestFailed;
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
  } catch (cause) { if (!disposed && requestGeneration === historyGeneration && selected.value?.id === order.id && !(cause instanceof DOMException && cause.name === "AbortError")) error.value = cause instanceof Error ? publicErrorMessage(cause) : words.value.requestFailed; }
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
    selected.value = null;
    activeAction.value = null;
    reason.value = "";
    await load(offset.value);
    if (!disposed && scope.value === requestScope) receipt.value = updated;
  } catch (cause) {
    if (disposed || scope.value !== requestScope) return;
    if (cause instanceof WithdrawalAdminUnknownIntentError) pending.value = intent;
    else if ((cause && typeof cause === "object" && "status" in cause && [0, 408, 429].includes(Number((cause as {status?:number}).status))) || (cause instanceof Error && !("status" in cause))) pending.value = intent;
    if (cause && typeof cause === "object" && "status" in cause && Number((cause as {status?:number}).status) === 401) emit("session-invalid");
    error.value = cause instanceof Error ? publicErrorMessage(cause) : words.value.requestFailed;
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
  if (props.active) void withGlobalLoading(load);
}, { immediate: true });
watch(() => props.active, active => { if (active) void withGlobalLoading(load, offset.value); });
onUnmounted(() => { disposed = true; generation++; historyGeneration++; controller?.abort(); historyController?.abort(); });
</script>

<template>
  <section class="withdrawal-management page-content">
    <header class="panel withdrawal-heading"><div><div class="eyebrow">{{ words.eyebrow }}</div><h1>{{ words.title }}</h1></div></header>
    <div v-if="!props.brandId" class="panel directory-state"><p>{{ words.noBrand }}</p></div>
    <div v-else-if="!grants.view" class="panel directory-state"><p>{{ words.noView }}</p></div>
    <div v-else>
      <p v-if="error" class="withdrawal-error" role="alert">{{ display(error) }}</p>
      <p v-if="receipt" class="withdrawal-receipt" role="status">{{ words.receipt }} · {{ receipt.id }} · {{ stateName(receipt.state) }}. {{ words.receiptNote }}</p>
      <div v-if="pending && pending.scope === scope" class="withdrawal-confirm" role="alert"><strong>{{ words.unknown }} · {{ pending.orderId }}</strong><p v-if="pending.body.reason">{{ words.reason }} · {{ pending.body.reason }}</p><button v-write-guard="busy" class="button button-secondary" :disabled="busy" @click="withGlobalLoading(perform, pending.action, true)">{{ words.replay }}</button></div>
      <div class="panel withdrawal-filters">
        <label class="query-field query-field-status">
          <span class="field-label"><BusinessDetails :value="words.state" :locale="associationLocale" /></span>
          <select v-model="stateFilter">
            <option value="">{{ words.all }}</option>
            <option v-for="state in states" :key="state" :value="state">{{ stateName(state) }}</option>
          </select>
        </label>
        <label class="query-field">
          <span class="field-label">{{ words.member }}</span>
          <div class="input-with-icon">
            <span class="input-icon"> <SiteIcon name="user" /> </span>
            <input v-model="memberFilter" type="text" autocomplete="off" :placeholder="words.filterPlaceholder" />
          </div>
        </label>
        <button class="button button-primary" :disabled="loading" @click="withGlobalLoading(load, 0)">{{ words.filter }}</button>
      </div>
      <div class="panel withdrawal-table-wrap"><table v-if="page.length" class="withdrawal-table console-responsive-table"><thead><tr><th>{{ words.order }}</th><th>{{ words.member }}</th><th>{{ words.points }}</th><th><BusinessDetails :value="words.state" :locale="associationLocale" /></th><th>{{ words.created }}</th><th></th></tr></thead><tbody><tr v-for="order in page" :key="order.id"><td class="mono" :data-label="words.order">{{ order.id }}</td><td class="mono" :data-label="words.member"><AssociationName :record="order" kind="member" :id="order.member_id" :locale="associationLocale" /></td><td :data-label="words.points">{{ order.points }}</td><td :data-label="words.state"><span class="badge" :class="order.state === 'paid' ? 'badge-success' : ['failed','rejected','cancelled'].includes(order.state) ? 'badge-neutral' : 'badge-warn'">{{ stateName(order.state) }}</span></td><td :data-label="words.created">{{ date(order.created_at) }}</td><td :data-label="t('admin.console.actions')"><button class="text-button" @click="withGlobalLoading(showHistory, order)">{{ words.history }}</button></td></tr></tbody></table><p v-if="!page.length && !loading" class="muted empty-state">{{ words.empty }}</p></div>
      <TablePagination class="withdrawal-pagination" :summary="t('admin.console.recordPage', { page: String(offset / MAX_WITHDRAWAL_PAGE + 1), count: String(page.length) })">
        <template #actions><button class="button button-secondary" :disabled="offset === 0 || loading" @click="withGlobalLoading(load, Math.max(0, offset - MAX_WITHDRAWAL_PAGE))">{{ words.previous }}</button><button class="button button-secondary" :disabled="!hasMore || loading" @click="withGlobalLoading(load, offset + MAX_WITHDRAWAL_PAGE)">{{ words.next }}</button></template>
      </TablePagination>
      <DetailModal v-if="!!selected" :title="words.details" :close-label="words.close" @close="selected = null; history = null">
<div v-if="selected" class="panel withdrawal-detail"><div class="panel-header"><div><h2>{{ selected.id }}</h2><p><BusinessDetails :value="words.state" :locale="associationLocale" /> · {{ stateName(selected.state) }}</p></div></div><p>{{ words.member }} · <AssociationName :record="selected" kind="member" :id="selected.member_id" :locale="associationLocale" /> · {{ words.points }} · {{ selected.points }}</p><div v-if="history"><div v-for="item in history.items" :key="item.id" class="withdrawal-history-row"><span>{{ item.from_state ? stateName(item.from_state) + ' → ' : '' }}{{ stateName(item.to_state) }}</span><small>{{ date(item.created_at) }} · {{ words.actors }}: {{ item.actor_type === 'admin' ? t('admin.copy.37b47e95') : item.actor_type === 'user' ? t('admin.copy.689b5703') : t('admin.copy.532e2e04') }}<span v-if="item.reason"> · {{ item.reason }}</span></small></div></div><p v-else class="muted">{{ words.loading }}</p>
        <template v-if="actionsFor(selected).some((action) => grants.actions[action])"><div class="withdrawal-actions"><button v-for="action in actionsFor(selected).filter((entry) => grants.actions[entry])" :key="action" class="button button-secondary" :disabled="Boolean(pending)" :title="action === 'mark-paid' ? words.internal : undefined" @click="begin(selected!, action)">{{ actionName(action) }}</button></div><form v-write-guard="busy" novalidate v-inline-validation v-if="activeAction && !pending" class="withdrawal-confirm" @submit.prevent="withGlobalLoading(perform, activeAction)"><p v-if="activeAction === 'mark-paid'" class="withdrawal-internal-note" role="note">{{ words.internal }}</p><div class="withdrawal-actions"><button class="button button-secondary" type="button" @click="reason = ''; activeAction = null">{{ words.close }}</button><button class="button button-primary" type="submit" :disabled="busy || !activeAction">{{ activeAction === 'mark-paid' ? words.confirm : `${words.confirm} ${actionName(activeAction)}` }}</button></div></form></template>
      </div>
</DetailModal>
    </div>
  </section>
</template>

<style scoped>
.withdrawal-management{max-width:1440px;margin:0 auto}.withdrawal-heading{display:flex;justify-content:space-between;align-items:center;gap:18px;padding:22px;margin-bottom:16px}.withdrawal-heading h1{margin:4px 0}.withdrawal-heading p{margin:0;color:var(--muted)}.withdrawal-filters{display:flex;align-items:end;gap:12px;padding:16px;margin-bottom:14px}.withdrawal-filters label{display:grid;gap:6px;min-width:min(100%,220px)}.withdrawal-table-wrap{overflow:auto;padding:8px 16px}.withdrawal-table{width:100%;border-collapse:collapse;min-width:680px}.withdrawal-table th,.withdrawal-table td{text-align:left;padding:12px 10px;border-bottom:1px solid var(--line)}.withdrawal-table th{color:var(--muted);font-size:12px}.withdrawal-detail{padding:20px;margin-top:14px}.withdrawal-history-row{display:grid;gap:5px;padding:10px 0;border-bottom:1px solid var(--line)}.withdrawal-history-row small{color:var(--muted)}.withdrawal-actions{display:flex;flex-wrap:wrap;gap:8px;margin-top:14px}.withdrawal-confirm{display:grid;gap:10px;margin-top:14px;padding:14px;border-radius:10px;background:var(--surface-soft,#f5f7f6)}.withdrawal-confirm label{display:grid;gap:6px}.withdrawal-confirm textarea{min-height:80px;resize:vertical}.withdrawal-error{color:var(--danger)}.withdrawal-receipt{padding:12px;background:var(--surface-soft,#f5f7f6);border-radius:8px}@media(max-width:600px){.withdrawal-heading,.withdrawal-filters{align-items:stretch;flex-direction:column}.withdrawal-filters label{min-width:0}.withdrawal-management{padding-inline:0}}
@media (max-width: 1080px) { .withdrawal-table-wrap { overflow: visible; } .withdrawal-table { min-width: 0; } }
</style>
