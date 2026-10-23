<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from './i18n'
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from '@lottery/shared'
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import DetailModal from "@lottery/shared/DetailModal.vue"
import { computed, onUnmounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { adminKey, useAdminI18n } from "./i18n";
import CommissionAdjustmentsManagement from "./CommissionAdjustmentsManagement.vue";
import {
  commissionPaymentPermissions, createCommissionPaymentsApi,
  type CommissionPayment, type CommissionPaymentPage, type CommissionPaymentPolicy,
} from "./commissionPayments-api";
import {
  classifyCommissionPaymentWriteFailure, clearAllCommissionPaymentIntents,
  clearPendingCommissionPaymentIntent, commissionPaymentSessionGeneration,
  createCommissionPaymentIntent, freezeCommissionPaymentBody, isCommissionPaymentIntentConflict,
  getPendingCommissionPaymentIntent, hideCommissionPaymentIntentsForScope, listPendingCommissionPaymentIntents, markCommissionPaymentIntentConflict, setPendingCommissionPaymentIntent,
  type CommissionPaymentIntentScope, type PendingCommissionPaymentIntent,
} from "./commissionPayments-state";

const props = defineProps<{ account: AdminAccount; brandId: string; brandStatus?: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, locale } = useAdminI18n();
const operationLabel = (operation: PendingCommissionPaymentIntent['operation']) => t({ policy: 'admin.copy.2288349a', approve: 'admin.copy.c176586a', retry: 'admin.copy.ce7fd4f3' }[operation] as Parameters<typeof t>[0]);
const api = createCommissionPaymentsApi(fetch, props.account.id);
const PAGE_SIZE = 20;
const rights = computed(() => commissionPaymentPermissions(props.account, props.brandId));
const scopeStamp = computed(() => JSON.stringify([props.account.id, props.account.super_admin, props.account.brand_ids,
  props.account.permissions, props.account.permissions_by_brand, props.account.platform_permissions, props.brandId, props.brandStatus, rights.value]));
const policy = ref<CommissionPaymentPolicy | null>(null), page = ref<CommissionPaymentPage | null>(null);
const selected = ref<CommissionPayment | null>(null), detail = ref<CommissionPayment | null>(null);
const receipt = ref<{ operation: string; payment?: CommissionPayment; policy?: CommissionPaymentPolicy } | null>(null);
const loadingPolicy = ref(false), loadingList = ref(false), loadingDetail = ref(false), writing = ref(false);
const policyError = ref<string | import("@lottery/shared").LocalizedMessage>(""), listError = ref<string | import("@lottery/shared").LocalizedMessage>(""), detailError = ref<string | import("@lottery/shared").LocalizedMessage>(""), writeError = ref<string | import("@lottery/shared").LocalizedMessage>(""), notice = ref("");
const reason = ref(""), dialog = ref<PendingCommissionPaymentIntent | null>(null), confirmed = ref(false);
const unknownIntent = ref<PendingCommissionPaymentIntent | null>(null), conflictIntent = ref<PendingCommissionPaymentIntent | null>(null);
const conflict = ref(false), conflictReloading = ref(false), conflictRefreshed = ref(false), discardReviewed = ref(false);
let alive = true, generation = 0, policyTicket = 0, listTicket = 0, detailTicket = 0, paymentReadTicket = 0, writeTicket = 0;
let conflictTicket = 0;

const offset = computed(() => page.value?.offset ?? 0);
const totalCount = computed(() => page.value?.total_count ?? "0");
const hasPrevious = computed(() => offset.value > 0);
const hasNext = computed(() => page.value !== null && BigInt(page.value.offset + page.value.limit) < BigInt(page.value.total_count));
const policyCanSave = computed(() => rights.value.policyWrite && props.brandStatus !== "disabled" && !writing.value && !loadingPolicy.value && !policyError.value && !unknownIntent.value && !conflict.value && Boolean(policy.value) && policy.value!.version < Number.MAX_SAFE_INTEGER && validReason(reason.value));
const policyEnabled = ref(false);
const canApprove = computed(() => Boolean(rights.value.approve && props.brandStatus !== "disabled" && !props.account.super_admin && !writing.value && !loadingDetail.value && !detailError.value && !unknownIntent.value && !conflict.value && detail.value && detail.value.state === "awaiting_approval" && (detail.value.payout_mode === "manual" || detail.value.payout_mode === "mixed") && detail.value.version < Number.MAX_SAFE_INTEGER && validReason(reason.value)));
const canRetry = computed(() => Boolean(rights.value.retry && props.brandStatus !== "disabled" && !props.account.super_admin && !writing.value && !loadingDetail.value && !detailError.value && !unknownIntent.value && !conflict.value && detail.value?.state === "failed" && detail.value.version < Number.MAX_SAFE_INTEGER && validReason(reason.value)));

function validReason(value: string): boolean {
  if (value.trim() !== value || new TextEncoder().encode(value).length > 500 || /[\u0000-\u001f\u007f-\u009f]/u.test(value)) return false;
  for (let i = 0; i < value.length; i++) { const code = value.charCodeAt(i); if (code >= 0xd800 && code <= 0xdbff) { const next = value.charCodeAt(++i); if (!(next >= 0xdc00 && next <= 0xdfff)) return false; } else if (code >= 0xdc00 && code <= 0xdfff) return false; }
  return true;
}
function paymentScope(operation: "approve" | "retry", id: string): CommissionPaymentIntentScope { return { accountId: props.account.id, brandId: props.brandId, operation, targetId: id }; }
function current(ticket: number, lane: "policy" | "list" | "detail", captured: string, session: number, commonReadTicket?: number): boolean {
  const laneTicket = lane === "policy" ? policyTicket : lane === "list" ? listTicket : detailTicket;
  return alive && ticket === laneTicket && (lane === "policy" || commonReadTicket === paymentReadTicket) && captured === scopeStamp.value && session === commissionPaymentSessionGeneration() && rights.value.view;
}
function laneCurrent(ticket: number, lane: "policy" | "list" | "detail", captured: string, session: number): boolean {
  const laneTicket = lane === "policy" ? policyTicket : lane === "list" ? listTicket : detailTicket;
  return alive && ticket === laneTicket && captured === scopeStamp.value && session === commissionPaymentSessionGeneration() && rights.value.view;
}
function currentWrite(ticket: number, captured: string, session: number): boolean { return alive && ticket === writeTicket && captured === scopeStamp.value && session === commissionPaymentSessionGeneration(); }
function clearVisible() {
  policy.value = null; page.value = null; selected.value = null; detail.value = null; receipt.value = null;
  reason.value = ""; dialog.value = null; confirmed.value = false; unknownIntent.value = null; conflictIntent.value = null;
  conflict.value = false; conflictRefreshed.value = false; discardReviewed.value = false;
  policyError.value = listError.value = detailError.value = writeError.value = notice.value = "";
  loadingPolicy.value = loadingList.value = loadingDetail.value = writing.value = false;
}
function resetScope(_next?: string, previous?: string) {
  if (previous) {
    try { const previousScope = JSON.parse(previous) as unknown[]; hideCommissionPaymentIntentsForScope(String(previousScope[0]), String(previousScope[6])); }
    catch { /* Invalid old scope cannot authorize or expose a pending intent. */ }
  }
  generation++; policyTicket++; listTicket++; detailTicket++; paymentReadTicket++; writeTicket++; conflictTicket++;
  clearVisible();
  const pending = listPendingCommissionPaymentIntents(props.account.id, props.brandId);
  const conflictedIntent = pending.find(isCommissionPaymentIntentConflict);
  conflictIntent.value = conflictedIntent ?? null; conflict.value = Boolean(conflictedIntent);
  unknownIntent.value = conflictedIntent ? null : pending[0] ?? null;
  if (unknownIntent.value) writeError.value = t('admin.copy.7b647cb6');
  if (rights.value.view) {
    const captured = scopeStamp.value, session = commissionPaymentSessionGeneration(), target = unknownIntent.value?.targetId;
    const initial = Promise.all([loadPolicy(), loadList(0)]), initialRead = paymentReadTicket;
    void initial.then(() => { if (alive && rights.value.view && captured === scopeStamp.value && session === commissionPaymentSessionGeneration() && initialRead === paymentReadTicket && target) void loadDetail(target); });
  }
}
function sessionExpired(cause: unknown, captured: string, session: number): boolean {
  if (cause instanceof AdminApiError && cause.status === 401 && alive && captured === scopeStamp.value && session === commissionPaymentSessionGeneration()) {
    generation++; policyTicket++; listTicket++; detailTicket++; paymentReadTicket++; writeTicket++; clearAllCommissionPaymentIntents(); clearVisible(); emit("session-invalid"); return true;
  }
  return false;
}
function readFailure(cause: unknown): string | import("@lottery/shared").LocalizedMessage { return cause instanceof AdminApiError && cause.status === 0 ? t('admin.copy.ce90986e') : cause instanceof Error ? publicErrorMessage(cause) : t('admin.copy.466e55a6'); }
function isCurrent(payment: CommissionPayment | null): boolean { return Boolean(payment && detail.value?.id === payment.id && detail.value.version === payment.version); }

function cancelConflictReview() {
  if (!conflict.value) return;
  conflictTicket++; conflictReloading.value = false; conflictRefreshed.value = false; discardReviewed.value = false;
}
async function loadPolicy(preserveConflictReview = false) {
  if (!preserveConflictReview) cancelConflictReview();
  if (!rights.value.view) return;
  const ticket = ++policyTicket, captured = scopeStamp.value, session = commissionPaymentSessionGeneration();
  loadingPolicy.value = true; policyError.value = "";
  try {
    const fresh = await api.getPolicy(props.brandId);
    if (!current(ticket, "policy", captured, session)) return;
    policy.value = fresh; policyEnabled.value = fresh.enabled;
  } catch (cause) { if (current(ticket, "policy", captured, session)) { sessionExpired(cause, captured, session); policyError.value = readFailure(cause); } }
  finally { if (current(ticket, "policy", captured, session)) loadingPolicy.value = false; }
}
async function loadList(nextOffset = offset.value, preserveConflictReview = false) {
  if (!preserveConflictReview) cancelConflictReview();
  if (!rights.value.view) return;
  const readTicket = ++paymentReadTicket;
  const ticket = ++listTicket, captured = scopeStamp.value, session = commissionPaymentSessionGeneration();
  loadingList.value = true; listError.value = "";
  try {
    const fresh = await api.list(props.brandId, PAGE_SIZE, nextOffset);
    if (!current(ticket, "list", captured, session, readTicket)) return;
    page.value = fresh;
    if (selected.value) {
      const listed = fresh.items.find((item) => item.id === selected.value?.id);
      if (!listed) { selected.value = null; detail.value = null; detailTicket++; loadingDetail.value = false; }
      else if (listed.version !== detail.value?.version || listed.state !== detail.value?.state || listed.evidence_epoch !== detail.value?.evidence_epoch || listed.paid_count !== detail.value?.paid_count || listed.paid_points !== detail.value?.paid_points) void loadDetail(listed.id, true);
      else { selected.value = listed; detail.value = listed; }
    }
  } catch (cause) { if (current(ticket, "list", captured, session, readTicket)) { sessionExpired(cause, captured, session); listError.value = readFailure(cause); } }
  finally { if (laneCurrent(ticket, "list", captured, session)) loadingList.value = false; }
}
async function loadDetail(id: string, preserveConflictReview = false) {
  if (!preserveConflictReview) cancelConflictReview();
  const readTicket = ++paymentReadTicket;
  const ticket = ++detailTicket, captured = scopeStamp.value, session = commissionPaymentSessionGeneration();
  loadingDetail.value = true; detailError.value = "";
  selected.value = page.value?.items.find((item) => item.id === id) ?? selected.value;
  if (detail.value?.id !== id) { reason.value = ""; detail.value = null; }
  try {
    const fresh = await api.read(props.brandId, id);
    if (!current(ticket, "detail", captured, session, readTicket) || fresh.id !== id) return;
    selected.value = fresh; detail.value = fresh;
  } catch (cause) { if (current(ticket, "detail", captured, session, readTicket)) { sessionExpired(cause, captured, session); detailError.value = readFailure(cause); } }
  finally { if (laneCurrent(ticket, "detail", captured, session)) loadingDetail.value = false; }
}
async function reloadAll(preserveConflictReview = false) {
  await Promise.all([loadPolicy(preserveConflictReview), loadList(offset.value, preserveConflictReview)]);
  if (selected.value) await loadDetail(selected.value.id, preserveConflictReview);
}

function intentFor(operation: CommissionPaymentIntentScope["operation"], targetId?: string): PendingCommissionPaymentIntent | null {
  const scope: CommissionPaymentIntentScope = { accountId: props.account.id, brandId: props.brandId, operation, ...(operation !== "policy" && targetId ? { targetId } : {}) };
  const old = getPendingCommissionPaymentIntent(scope); if (old) return old;
  let body;
  if (operation === "policy") {
    if (!policyCanSave.value || !policy.value) return null;
    body = freezeCommissionPaymentBody("policy", { version: policy.value.version, enabled: policyEnabled.value, reason: reason.value });
  } else {
    const row = detail.value;
    if (!row || row.id !== targetId || !validReason(reason.value) || row.version >= Number.MAX_SAFE_INTEGER || writing.value || !rights.value.view || props.account.super_admin || loadingDetail.value) return null;
    if (operation === "approve" && !canApprove.value) return null;
    if (operation === "retry" && !canRetry.value) return null;
    body = freezeCommissionPaymentBody(operation, { version: row.version, reason: reason.value });
  }
  return createCommissionPaymentIntent(scope, body, createIdempotencyKey());
}
function openReview(operation: CommissionPaymentIntentScope["operation"]) {
  if (conflict.value || unknownIntent.value) return;
  const intent = intentFor(operation, detail.value?.id);
  if (!intent) return;
  dialog.value = intent; confirmed.value = false; writeError.value = "";
}
function pendingIntent(): PendingCommissionPaymentIntent | null {
  if (unknownIntent.value) return unknownIntent.value;
  return dialog.value;
}
async function submit(intent: PendingCommissionPaymentIntent, recovery = Boolean(unknownIntent.value && unknownIntent.value.key === intent.key)) {
  if (writing.value || !confirmed.value || !rights.value.view || props.brandStatus === "disabled" || !validIntentScope(intent)) return;
  if ((intent.operation === "policy" && !rights.value.policyWrite) || (intent.operation === "approve" && !rights.value.approve) || (intent.operation === "retry" && !rights.value.retry)) return;
  if (!recovery && intent.operation !== "policy" && (!detail.value || detail.value.id !== intent.targetId || detail.value.version !== intent.body.version || loadingDetail.value)) return;
  const captured = scopeStamp.value, session = commissionPaymentSessionGeneration(), ticket = ++writeTicket;
  writing.value = true; writeError.value = ""; notice.value = ""; setPendingCommissionPaymentIntent(intent, intent);
  try {
    let result: CommissionPayment | CommissionPaymentPolicy;
    if (intent.operation === "policy") result = await api.updatePolicy(intent.brandId, intent.body as Readonly<{ version: number; enabled: boolean; reason: string }>, intent.key);
    else if (intent.operation === "approve") result = await api.approve(intent.brandId, intent.targetId!, intent.body as Readonly<{ version: number; reason: string }>, intent.key);
    else result = await api.retry(intent.brandId, intent.targetId!, intent.body as Readonly<{ version: number; reason: string }>, intent.key);
    clearPendingCommissionPaymentIntent(intent, intent.key);
    if (!currentWrite(ticket, captured, session)) return;
    receipt.value = intent.operation === "policy" ? { operation: intent.operation, policy: result as CommissionPaymentPolicy } : { operation: intent.operation, payment: result as CommissionPayment };
    dialog.value = null; unknownIntent.value = null; confirmed.value = false;
    notice.value = t('admin.copy.1c78ca16');
    if (intent.operation === "policy") await loadPolicy();
    else { await loadList(offset.value); if (currentWrite(ticket, captured, session)) await loadDetail(intent.targetId!); }
  } catch (cause) {
    const status = cause instanceof AdminApiError ? cause.status : 0, outcome = classifyCommissionPaymentWriteFailure(status);
    if (!currentWrite(ticket, captured, session)) { if (outcome !== "unknown") clearPendingCommissionPaymentIntent(intent, intent.key); return; }
    if (sessionExpired(cause, captured, session)) { clearPendingCommissionPaymentIntent(intent, intent.key); return; }
    if (outcome === "unknown") {
      setPendingCommissionPaymentIntent(intent, intent); unknownIntent.value = intent; dialog.value = null;
      writeError.value = t('admin.copy.05b72762');
    } else if (outcome === "conflict") {
      markCommissionPaymentIntentConflict(intent); unknownIntent.value = null; conflictIntent.value = intent; dialog.value = null; conflict.value = true; conflictRefreshed.value = false; discardReviewed.value = false;
      writeError.value = t('admin.copy.0a97ca33');
    } else {
      clearPendingCommissionPaymentIntent(intent, intent.key); unknownIntent.value = null; dialog.value = null;
      writeError.value = cause instanceof Error ? publicErrorMessage(cause) : t('admin.copy.7565cd98');
    }
  } finally { if (currentWrite(ticket, captured, session)) writing.value = false; }
}
function validIntentScope(intent: PendingCommissionPaymentIntent): boolean { return intent.accountId === props.account.id && intent.brandId === props.brandId && (intent.operation === "policy" ? intent.targetId === undefined : typeof intent.targetId === "string"); }
async function reloadConflict() {
  if (!conflict.value || conflictReloading.value) return;
  const intent = conflictIntent.value;
  if (!intent) return;
  const ticket = ++conflictTicket, captured = scopeStamp.value;
  conflictReloading.value = true;
  policyError.value = listError.value = detailError.value = "";
  await Promise.all([loadPolicy(true), loadList(offset.value, true)]);
  if (intent.targetId) await loadDetail(intent.targetId, true);
  if (alive && ticket === conflictTicket && captured === scopeStamp.value && rights.value.view && policy.value && page.value && !policyError.value && !listError.value && (!intent.targetId || (detail.value?.id === intent.targetId && !detailError.value))) {
    conflictRefreshed.value = true; discardReviewed.value = false;
    writeError.value = t('admin.copy.18df46b5');
  }
  if (ticket === conflictTicket) conflictReloading.value = false;
}
function discardConflictIntent() {
  const intent = conflictIntent.value;
  if (!intent || !conflictRefreshed.value || !discardReviewed.value || conflictReloading.value) return;
  clearPendingCommissionPaymentIntent(intent, intent.key); conflictIntent.value = null; conflict.value = false;
  conflictRefreshed.value = false; discardReviewed.value = false; writeError.value = "";
  notice.value = t('admin.copy.93a931e1');
}
function retryUnknown() { const intent = unknownIntent.value; if (!intent || writing.value || !rights.value.view || props.brandStatus === "disabled" || !validIntentScope(intent)) return; dialog.value = intent; confirmed.value = false; }
function formatDate(value: string): string{ return formatDisplayDateTime(value, displayLocale.value); }
function stateLabel(state: CommissionPayment["state"]): string {
  const names: Record<CommissionPayment["state"], string> = { awaiting_approval: 'admin.copy.df25051f', paying: 'admin.copy.b4ab6cf7', paid: 'admin.copy.f2dc031c', failed: 'admin.copy.1feabec0', stale: 'admin.copy.39859fe0', blocked: 'admin.copy.271c1f70' };
 return t(adminKey(names[state]!));
}
function modeLabel(mode: CommissionPayment["payout_mode"]): string {
  const names: Record<CommissionPayment["payout_mode"], string> = { manual: 'admin.copy.6644f5f6', automatic: 'admin.copy.4b7603bc', mixed: 'admin.copy.ccdc17cc', none: 'admin.copy.c2b41b33' };
 return t(adminKey(names[mode]!));
}
function displayReceipt(): string { return receipt.value?.policy ? t('admin.versionDisplay.policySaved') : receipt.value?.payment ? stateLabel(receipt.value.payment.state) : ""; }

watch(scopeStamp, resetScope, { immediate: true });
onUnmounted(() => { alive = false; generation++; policyTicket++; listTicket++; detailTicket++; writeTicket++; conflictTicket++; });
function closeDetails() {
  detailTicket++; selected.value = null; detail.value = null; loadingDetail.value = false; detailError.value = '';
}
</script>

<template>
  <article class="panel commission-payments">
    <header class="cp-heading"><div><h1>{{ t('admin.copy.91082687') }}</h1></div><button v-refresh-button="t('admin.copy.894e2a7d')" class="button" type="button" :disabled="loadingPolicy || loadingList || !rights.view" @click="withGlobalLoading(reloadAll)">{{ t('admin.copy.894e2a7d') }}</button></header>

    <div class="cp-callout" role="note">{{ t('admin.copy.bed87261') }}</div>
    <p v-if="!rights.view" class="cp-state">{{ t('admin.copy.cdaf5831') }}</p>
    <template v-else>
      <div v-if="policyError" class="cp-error" role="alert">{{ displayPublicError(policyError) }}</div>
      <section class="cp-policy" aria-labelledby="cp-policy-title" data-testid="commission-payment-policy">
        <div><h2 id="cp-policy-title">{{ t('admin.copy.5d866b76') }}</h2></div>
        <label class="cp-toggle"><input data-testid="commission-payment-policy-enabled" v-model="policyEnabled" type="checkbox" :disabled="!rights.policyWrite || !policy || writing || Boolean(unknownIntent) || conflict" /><span>{{ policyEnabled ? t('admin.copy.29b7acd3') : t('admin.copy.6c682517') }}</span></label>
        <p v-if="!rights.policyWrite" class="cp-muted">{{ t('admin.copy.d49af3ea') }}</p>
        <p v-if="brandStatus === 'paused'" class="cp-muted">{{ t('admin.copy.042f6041') }}</p>
        <p class="cp-muted">{{ policyEnabled ? t('admin.copy.393c2eed') : t('admin.copy.70699ba7') }}</p>

        <div class="cp-actions"><button data-testid="commission-payment-policy-review" class="button button-primary" type="button" :disabled="!policyCanSave || policyEnabled === policy?.enabled" @click="openReview('policy')">{{ t('admin.copy.2288349a') }}</button><button v-refresh-button="conflictReloading ? t('admin.copy.948fc498') : t('admin.copy.6b2dcb02')" v-if="conflict" class="button" type="button" :disabled="conflictReloading" @click="withGlobalLoading(reloadConflict)">{{ conflictReloading ? t('admin.copy.948fc498') : t('admin.copy.6b2dcb02') }}</button></div>
      </section>

      <div class="cp-section-heading"><div><h2>{{ t('admin.copy.bf8bb3ee') }}</h2></div><button v-refresh-button="loadingList ? t('admin.copy.27342bf3') : t('admin.copy.855b9442')" data-testid="commission-payments-refresh" class="button" type="button" :disabled="loadingList" @click="withGlobalLoading(loadList, 0)">{{ loadingList ? t('admin.copy.27342bf3') : t('admin.copy.855b9442') }}</button></div>
      <div v-if="listError" class="cp-error" role="alert">{{ displayPublicError(listError) }}</div>
      <p v-global-loading v-else-if="loadingList && !page" class="cp-state" role="status">{{ t('admin.copy.dbac5664') }}</p>
      <p v-else-if="page?.items.length === 0" class="cp-state empty-state">{{ t('admin.copy.f64126f9') }}</p>
      <div v-if="page?.items.length" class="cp-table-wrap" data-testid="commission-payments-jobs"><table><thead><tr><th>{{ t('admin.copy.966b8f09') }}</th><th>{{ t('admin.copy.cf6d9c6d') }}</th><th>{{ t('admin.copy.e682c364') }}</th><th>{{ t('admin.copy.ec082fa7') }}</th><th>{{ t('admin.copy.bf8f85e1') }}</th><th>{{ t('admin.copy.19bf47ac') }}</th></tr></thead><tbody><tr v-for="item in page.items" :key="item.id" :data-payment-id="item.id" :class="{ selected: selected?.id === item.id }"><td><button class="cp-link" type="button" @click="withGlobalLoading(loadDetail, item.id)">{{ item.id }}</button><small>Cycle <AssociationName :record="item" kind="cycle" :id="item.cycle_id" :locale="associationLocale" /></small></td><td><span class="cp-status" :class="`state-${item.state}`">{{ stateLabel(item.state) }}</span><small v-if="item.last_error_code">{{ item.last_error_code ? displayPublicError(publicErrorMessage({ code: item.last_error_code })) : "—" }}</small></td><td>{{ modeLabel(item.payout_mode) }}</td><td>{{ item.paid_points }} / {{ item.total_points }}</td><td>{{ item.paid_count }} / {{ item.target_count }}</td><td>{{ formatDate(item.updated_at) }}</td></tr></tbody></table></div>
      <div v-if="page" class="cp-pagination"><span>{{ page.items.length ? page.offset + 1 : 0 }}–{{ page.offset + page.items.length }} / {{ page.total_count }}</span><div><button class="button" type="button" :disabled="!hasPrevious || loadingList" @click="withGlobalLoading(loadList, Math.max(0, offset - PAGE_SIZE))">{{ t('admin.copy.dbe7a5d7') }}</button><button class="button" type="button" :disabled="!hasNext || loadingList" @click="withGlobalLoading(loadList, offset + PAGE_SIZE)">{{ t('admin.copy.2e9c9e7a') }}</button></div></div>

      <DetailModal v-if="selected || detail || loadingDetail" :open="!dialog" :title="t('admin.copy.01db40da')" :close-label="t('admin.copy.2bb2cb1d')" @close="closeDetails">
<section v-global-loading v-if="loadingDetail" class="cp-detail cp-state" role="status">{{ t('admin.copy.b6d0df6c') }}</section>
      <section v-else-if="detail" class="cp-detail" aria-labelledby="cp-detail-title">
        <div class="cp-section-heading"><div><h2 id="cp-detail-title">{{ t('admin.copy.f5167363') }}</h2><p>{{ stateLabel(detail.state) }} · {{ modeLabel(detail.payout_mode) }}</p></div><button v-refresh-button="t('admin.copy.2afd7606')" class="button" type="button" :disabled="loadingDetail" @click="withGlobalLoading(loadDetail, detail.id)">{{ t('admin.copy.2afd7606') }}</button></div>
        <div v-if="detailError" class="cp-error" role="alert">{{ displayPublicError(detailError) }}</div>
        <dl class="cp-facts"><dt>ID</dt><dd>{{ detail.id }}</dd><dt>{{ t('admin.copy.5c8048b9') }}</dt><dd><AssociationName :record="detail" kind="cycle" :id="detail.cycle_id" :locale="associationLocale" /><br />{{ detail.run_id }}</dd><dt>{{ t('admin.copy.5261a883') }}</dt><dd>{{ detail.paid_count }} / {{ detail.target_count }}</dd><dt>{{ t('admin.copy.b28eed98') }}</dt><dd>{{ detail.paid_points }} / {{ detail.total_points }}</dd><dt>{{ t('admin.copy.bbf1c1e7') }}</dt><dd>{{ detail.last_error_code ? displayPublicError(publicErrorMessage({ code: detail.last_error_code })) : t('admin.copy.c2b41b33') }}</dd><dt>{{ t('admin.copy.59669acd') }}</dt><dd>{{ formatDate(detail.created_at) }}<br />{{ formatDate(detail.updated_at) }}</dd></dl>
        <div v-if="detail.payout_mode === 'mixed' && detail.state === 'awaiting_approval'" class="cp-callout" role="note">{{ t('admin.copy.f52ae3e2') }}</div>
        <div v-else-if="detail.state === 'blocked'" class="cp-callout" role="note">{{ t('admin.copy.df5f714e') }}</div>
        <div v-if="detail.paid_count !== '0' && detail.state === 'failed'" class="cp-callout" role="note">{{ t('admin.copy.a28d321d') }}</div>

        <div class="cp-actions"><button v-if="detail.state === 'awaiting_approval'" data-testid="commission-payment-approve" class="button button-primary" type="button" :disabled="!canApprove" @click="openReview('approve')">{{ t('admin.copy.c176586a') }}</button><button v-if="detail.state === 'failed'" data-testid="commission-payment-retry" class="button button-primary" type="button" :disabled="!canRetry" @click="openReview('retry')">{{ t('admin.copy.ce7fd4f3') }}</button><button v-refresh-button="t('admin.copy.d31400af')" class="button" type="button" :disabled="loadingList || loadingDetail" @click="withGlobalLoading(reloadAll)">{{ t('admin.copy.d31400af') }}</button></div>
        <CommissionAdjustmentsManagement v-if="!detailError"
          :key="`commission-adjustments:${account.id}:${brandId}:${detail.id}`"
          :account="account" :brand-id="brandId" :payment-id="detail.id"
          :payment-state="detail.state" :brand-status="brandStatus"
          @session-invalid="emit('session-invalid')" />
      </section>
</DetailModal>
      <div v-if="notice" class="cp-success" role="status">{{ notice }}</div>
      <div v-if="writeError" class="cp-error" role="alert">{{ displayPublicError(writeError) }}<button v-write-guard="writing" v-if="unknownIntent && rights.view" class="button" type="button" :disabled="writing || (unknownIntent.operation === 'policy' ? !rights.policyWrite : unknownIntent.operation === 'approve' ? !rights.approve : !rights.retry)" @click="retryUnknown">{{ writing ? t('admin.copy.a3df712a') : t('admin.copy.c6a6aa0c') }}</button><button v-refresh-button="t('admin.copy.6b2dcb02')" v-if="conflict" class="button" type="button" :disabled="conflictReloading" @click="withGlobalLoading(reloadConflict)">{{ t('admin.copy.6b2dcb02') }}</button></div>
      <section v-if="conflict && conflictIntent" class="cp-conflict" data-testid="commission-payment-conflict"><p>{{ conflictRefreshed ? t('admin.copy.281d8e9a') : t('admin.copy.c10e2006') }}</p><label class="cp-confirm"><input v-model="discardReviewed" type="checkbox" :disabled="!conflictRefreshed || conflictReloading" />{{ t('admin.copy.d52d7181') }}</label><div class="cp-actions"><button v-refresh-button="conflictReloading ? t('admin.copy.948fc498') : t('admin.copy.16a3182a')" class="button" type="button" :disabled="conflictReloading" @click="withGlobalLoading(reloadConflict)">{{ conflictReloading ? t('admin.copy.948fc498') : t('admin.copy.16a3182a') }}</button><button class="button button-primary" type="button" :disabled="!conflictRefreshed || !discardReviewed || conflictReloading" @click="discardConflictIntent">{{ t('admin.copy.7107e7f5') }}</button></div></section>
      <section v-if="receipt" class="cp-receipt" role="status" aria-live="polite" data-testid="commission-payment-receipt"><b>{{ t('admin.copy.d544aeb2') }}</b><span>{{ displayReceipt() }}</span><code v-if="receipt.payment">{{ receipt.payment.id }}</code></section>
      <section v-if="dialog" class="cp-dialog" role="dialog" aria-modal="true" :aria-label="t('admin.copy.08cf02f3')" data-testid="commission-payment-review">
        <h2>{{ t('admin.copy.08cf02f3') }}</h2>
        <p v-if="dialog.operation === 'policy' && 'enabled' in dialog.body && dialog.body.enabled" class="cp-callout" data-testid="commission-payment-enable-warning">{{ t('admin.copy.91b7e7bd') }}</p>
        <dl>
          <dt>{{ t('admin.copy.9c32661d') }}</dt><dd><AssociationName kind="brand" :id="dialog.brandId" :locale="associationLocale" /></dd>
          <dt>{{ t('admin.copy.087a786d') }}</dt><dd>{{ operationLabel(dialog.operation) }}<template v-if="dialog.targetId"> {{ dialog.targetId }}</template></dd>

          <dt v-if="'enabled' in dialog.body">{{ t('admin.copy.41210f20') }}</dt><dd v-if="'enabled' in dialog.body">{{ dialog.body.enabled ? t('admin.copy.de0e628f') : t('admin.copy.3565cde9') }}</dd>
          <dt v-if="dialog.body.reason">{{ t('admin.copy.5ec7c8ba') }}</dt><dd v-if="dialog.body.reason">{{ dialog.body.reason }}</dd>

        </dl>
        <label class="cp-confirm"><input data-testid="commission-payment-review-confirmed" v-model="confirmed" type="checkbox" />{{ t('admin.copy.2028b20d') }}</label>
        <div class="cp-actions"><button v-write-guard="writing" data-testid="commission-payment-confirm-submit" class="button button-primary" type="button" :disabled="writing || !confirmed" @click="withGlobalLoading(submit, dialog)">{{ writing ? t('admin.copy.46511dc0') : t('admin.copy.02c1e356') }}</button><button class="button" type="button" :disabled="writing" @click="dialog = null; confirmed = false">{{ t('admin.copy.e2f391e7') }}</button></div>
      </section>
    </template>
  </article>
</template>

<style scoped>
.commission-payments{display:grid;gap:18px;min-width:0}.cp-heading,.cp-section-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:16px}.cp-heading h1,.cp-section-heading h2,.cp-policy h2{margin:0}.cp-heading p,.cp-section-heading p,.cp-policy p{margin:6px 0 0;color:var(--muted);font-size:13px;line-height:1.55}.cp-kicker{font-size:11px!important;letter-spacing:.14em;color:var(--accent)!important;font-weight:800}.cp-callout,.cp-error,.cp-success,.cp-state,.cp-receipt{padding:12px 14px;border-radius:10px;line-height:1.55}.cp-callout{border:1px solid var(--warning);background:color-mix(in srgb,var(--warning) 8%,transparent)}.cp-error{display:flex;align-items:center;justify-content:space-between;gap:12px;color:var(--danger);background:color-mix(in srgb,var(--danger) 8%,transparent)}.cp-success{color:var(--success);background:color-mix(in srgb,var(--success) 8%,transparent)}.cp-state,.cp-muted{color:var(--muted)}.cp-policy,.cp-detail,.cp-dialog{display:grid;gap:14px;padding:18px;border:1px solid var(--border,var(--line));border-radius:14px;background:var(--surface)}.cp-toggle{display:flex;align-items:center;gap:10px;font-weight:750}.cp-toggle input,.cp-confirm input{width:18px;height:18px;accent-color:var(--accent)}.cp-reason{display:grid;gap:6px;font-weight:650}.cp-reason textarea{resize:vertical;min-height:60px;padding:10px;border:1px solid var(--border,#cbd5e1);border-radius:8px;font:inherit}.cp-actions,.cp-pagination,.cp-pagination>div{display:flex;align-items:center;gap:8px;flex-wrap:wrap}.cp-pagination{justify-content:space-between}.cp-table-wrap{width:100%;overflow:auto;border:1px solid var(--border,var(--line));border-radius:12px}.cp-table-wrap table{width:100%;border-collapse:collapse;min-width:790px}.cp-table-wrap th,.cp-table-wrap td{text-align:left;padding:11px 12px;border-bottom:1px solid var(--border,var(--line));vertical-align:top}.cp-table-wrap th{font-size:11px;color:var(--muted);background:var(--surface-muted,var(--canvas))}.cp-table-wrap tr:last-child td{border-bottom:0}.cp-table-wrap tr.selected{background:color-mix(in srgb,var(--accent) 7%,transparent)}.cp-table-wrap td small{display:block;margin-top:4px;color:var(--muted);overflow-wrap:anywhere}.cp-link{padding:0;border:0;background:transparent;color:var(--accent);font:inherit;text-align:left;overflow-wrap:anywhere;cursor:pointer}.cp-status{display:inline-flex;padding:4px 8px;border-radius:999px;background:#eef2f7;font-size:12px;font-weight:700}.state-paid{color:#087443;background:#e6f6ee}.state-failed,.state-blocked{color:#a12a22;background:#fff0ed}.state-stale{color:#8a5800;background:#fff6dd}.cp-facts,.cp-dialog dl{display:grid;grid-template-columns:minmax(140px,.35fr) minmax(0,1fr);gap:9px 14px;margin:0}.cp-facts dt,.cp-dialog dt{color:var(--muted)}.cp-facts dd,.cp-dialog dd{margin:0;overflow-wrap:anywhere}.cp-receipt{display:grid;gap:5px;border:1px solid var(--success);background:color-mix(in srgb,var(--success) 7%,transparent)}.cp-receipt code{overflow-wrap:anywhere}.cp-dialog{max-width:720px;width:100%;margin-inline:auto;box-shadow:0 20px 60px #0f172a30}.cp-dialog h2{margin:0}.cp-dialog>p{color:var(--muted)}.cp-confirm{display:flex;gap:9px;align-items:center}.cp-dialog dd:last-child{font-family:monospace;font-size:12px}
@media(max-width:700px){.cp-heading,.cp-section-heading{align-items:stretch;flex-direction:column}.cp-heading>.button,.cp-section-heading>.button{align-self:flex-start}.cp-policy,.cp-detail{padding:14px}.cp-facts,.cp-dialog dl{grid-template-columns:1fr;gap:4px}.cp-facts dd,.cp-dialog dd{margin-bottom:8px}.cp-actions>.button{flex:1 1 100%}.cp-pagination{align-items:flex-start;flex-direction:column}.cp-error{align-items:flex-start;flex-direction:column}}
</style>
