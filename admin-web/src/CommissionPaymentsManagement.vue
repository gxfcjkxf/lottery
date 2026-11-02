<script setup lang="ts">
import MemberActionsMenu from "@lottery/shared/MemberActionsMenu.vue"
import ManagementList from "@lottery/shared/ManagementList.vue"
import TablePagination from "@lottery/shared/TablePagination.vue"
const agentDraftSearchPending = ref(false);
const accountSearchReset = ref(0);
import FilterAssociationSearch from "@lottery/shared/FilterAssociationSearch.vue";
import {vInlineValidation} from "@lottery/shared";
import './commission-management.css';
import ManagementTable from '@lottery/shared/ManagementTable.vue';
import PlatformTemporalInput from '@lottery/shared/PlatformTemporalInput.vue';
import FilterAssociationSelect from '@lottery/shared/FilterAssociationSelect.vue';
import { commissionDateRange, type CommissionListQuery } from './commission-list-query';
import CommissionPaymentTargets from './CommissionPaymentTargets.vue';
import { useOperationToast } from "@lottery/shared";
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
  type CommissionPayment, type CommissionPaymentPage, type CommissionPaymentPolicy, type CommissionPaymentHistoryPage,
} from "./commissionPayments-api";
import {
  classifyCommissionPaymentWriteFailure, clearAllCommissionPaymentIntents,
  clearPendingCommissionPaymentIntent, commissionPaymentSessionGeneration,
  createCommissionPaymentIntent, freezeCommissionPaymentBody, isCommissionPaymentIntentConflict,
  getPendingCommissionPaymentIntent, hideCommissionPaymentIntentsForScope, listPendingCommissionPaymentIntents, markCommissionPaymentIntentConflict, setPendingCommissionPaymentIntent,
  type CommissionPaymentIntentScope, type PendingCommissionPaymentIntent,
} from "./commissionPayments-state";

const props = defineProps<{ account: AdminAccount; brandId: string; brandStatus?: string; initialCycleId?: string; cycleId?: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void; (event: "writing-change", value: boolean): void; (event: "open-cycle", cycleId: string): void }>();
const { t, message, locale } = useAdminI18n();
const operationLabel = (operation: PendingCommissionPaymentIntent['operation']) => t({ policy: 'admin.copy.2288349a', approve: 'admin.copy.c176586a', retry: 'admin.copy.ce7fd4f3' }[operation] as Parameters<typeof t>[0]);
const api = createCommissionPaymentsApi(fetch, props.account.id);
const PAGE_SIZE = 20;
const fromDraft=ref(''), toDraft=ref(''), agentDraft=ref(''), cycleDraft=ref(''), queue=ref('');
const committedFilter=ref<CommissionListQuery>({});
const searched=ref(true), detailTab=ref<'summary'|'targets'|'adjustments'|'paymentHistory'>('summary');
const historyPage=ref<CommissionPaymentHistoryPage|null>(null), historyBusy=ref(false), historyError=ref<string|import('@lottery/shared').LocalizedMessage>('');
let historyTicket=0;
const historyColumns=computed(()=>[{key:'operation',label:t('admin.commission.ui.actions')},{key:'actor',label:t('admin.commission.ui.actor')},{key:'state',label:t('admin.commission.ui.state')},{key:'time',label:t('admin.commission.ui.updated')}]);
async function loadHistory(offset=0){
 const row=detail.value;if(!row||!rights.value.view)return;
 const ticket=++historyTicket,captured=scopeStamp.value,session=commissionPaymentSessionGeneration();historyBusy.value=true;historyError.value='';
 try{const result=await api.history(props.brandId,row.id,20,offset);if(alive&&ticket===historyTicket&&captured===scopeStamp.value&&detail.value?.id===row.id&&session===commissionPaymentSessionGeneration())historyPage.value=result}
 catch(cause){if(alive&&ticket===historyTicket&&captured===scopeStamp.value&&detail.value?.id===row.id){sessionExpired(cause,captured,session);historyError.value=readFailure(cause)}}
 finally{if(ticket===historyTicket)historyBusy.value=false}
}
async function openDetailTab(tab:typeof detailTab.value){detailTab.value=tab;if(tab==='paymentHistory')await loadHistory()}
const reviewSummary=ref<{cycleId:string;points:string;count:string}|null>(null);
const columns=computed(()=>[
 {key:'cycle',label:t('admin.commission.ui.cycle')}, {key:'state',label:t('admin.commission.ui.state')},
 {key:'mode',label:t('admin.commission.ui.mode')}, {key:'planned',label:t('admin.commission.ui.planned'),cellClass:'numeric'},
 {key:'paid',label:t('admin.commission.ui.paid'),cellClass:'numeric'}, {key:'progress',label:t('admin.commission.ui.progress')},
 {key:'updated',label:t('admin.commission.ui.updated')}, {key:'actions',label:t('admin.commission.ui.actions')},
]);
async function applyFilters() {
  if (agentDraftSearchPending.value) return;
 try {
  committedFilter.value={...commissionDateRange(fromDraft.value,toDraft.value), ...(queue.value?{state:queue.value}:{}), ...(agentDraft.value?{agent_id:agentDraft.value}:{}), ...((props.cycleId || cycleDraft.value)?{cycle_id:props.cycleId || cycleDraft.value}:{})};
  searched.value=true; closeDetails(); await loadList(0);
 } catch { listError.value=t('admin.commission.ui.dateError'); }
}
async function resetFilters(){agentDraftSearchPending.value=false;accountSearchReset.value++;fromDraft.value=toDraft.value=agentDraft.value=cycleDraft.value='';queue.value=props.cycleId?'':rights.value.approve?'awaiting_approval':'';await applyFilters()}
async function chooseQueue(value:string){queue.value=value;await applyFilters()}
function dirtyFilters(){listTicket++;paymentReadTicket++;page.value=null;loadingList.value=false;searched.value=false}

const rights = computed(() => commissionPaymentPermissions(props.account, props.brandId));
const scopeStamp = computed(() => JSON.stringify([props.account.id, props.account.super_admin, props.account.brand_ids,
  props.account.permissions, props.account.permissions_by_brand, props.account.platform_permissions, props.brandId, props.brandStatus, rights.value, props.cycleId]));
const policy = ref<CommissionPaymentPolicy | null>(null), page = ref<CommissionPaymentPage | null>(null);
const selected = ref<CommissionPayment | null>(null), detail = ref<CommissionPayment | null>(null);
const receipt = ref<{ operation: string; payment?: CommissionPayment; policy?: CommissionPaymentPolicy } | null>(null);
const loadingPolicy = ref(false), loadingList = ref(false), loadingDetail = ref(false), writing = ref(false);
const policyError = ref<string | import("@lottery/shared").LocalizedMessage>(""), listError = ref<string | import("@lottery/shared").LocalizedMessage>(""), detailError = ref<string | import("@lottery/shared").LocalizedMessage>(""), writeError = ref<string | import("@lottery/shared").LocalizedMessage>(""), notice = ref<string | import("@lottery/shared").LocalizedMessage>("");
const reason = ref(""), dialog = ref<PendingCommissionPaymentIntent | null>(null), confirmed = ref(false);
const unknownIntent = ref<PendingCommissionPaymentIntent | null>(null), conflictIntent = ref<PendingCommissionPaymentIntent | null>(null);
const conflict = ref(false), conflictReloading = ref(false), conflictRefreshed = ref(false), discardReviewed = ref(false);
let alive = true, generation = 0, policyTicket = 0, listTicket = 0, detailTicket = 0, paymentReadTicket = 0, writeTicket = 0;
let conflictTicket = 0;

const offset = computed(() => page.value?.offset ?? 0);
const hasPrevious = computed(() => offset.value > 0);
const hasNext = computed(() => page.value !== null && BigInt(page.value.offset + page.value.limit) < BigInt(page.value.total_count));
const policyCanSave = computed(() => rights.value.policyWrite && props.brandStatus !== "disabled" && !writing.value && !loadingPolicy.value && !policyError.value && !unknownIntent.value && !conflict.value && Boolean(policy.value) && policy.value!.version < Number.MAX_SAFE_INTEGER && validReason(reason.value));
const policyEnabled = ref(false);
const canApprove = computed(() => Boolean(rights.value.approve && policy.value?.enabled === true && !loadingPolicy.value && !policyError.value && props.brandStatus !== "disabled" && !props.account.super_admin && !writing.value && !loadingDetail.value && !detailError.value && !unknownIntent.value && !conflict.value && detail.value && detail.value.state === "awaiting_approval" && (detail.value.payout_mode === "manual" || detail.value.payout_mode === "mixed") && detail.value.version < Number.MAX_SAFE_INTEGER && validReason(reason.value)));
const canRetry = computed(() => Boolean(rights.value.retry && policy.value?.enabled === true && !loadingPolicy.value && !policyError.value && props.brandStatus !== "disabled" && !props.account.super_admin && !writing.value && !loadingDetail.value && !detailError.value && !unknownIntent.value && !conflict.value && detail.value?.state === "failed" && detail.value.version < Number.MAX_SAFE_INTEGER && validReason(reason.value)));

function validReason(value: string): boolean {
  if (value.trim() !== value || Array.from(value).length > 500 || /[\u0000-\u001f\u007f-\u009f]/u.test(value)) return false;
  for (let i = 0; i < value.length; i++) { const code = value.charCodeAt(i); if (code >= 0xd800 && code <= 0xdbff) { const next = value.charCodeAt(++i); if (!(next >= 0xdc00 && next <= 0xdfff)) return false; } else if (code >= 0xdc00 && code <= 0xdfff) return false; }
  return true;
}
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
  historyTicket++;historyPage.value=null;historyBusy.value=false;historyError.value='';
  policy.value = null; page.value = null; selected.value = null; detail.value = null; receipt.value = null;
  reason.value = ""; detailTab.value="summary"; reviewSummary.value=null; dialog.value = null; confirmed.value = false; unknownIntent.value = null; conflictIntent.value = null;
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
  fromDraft.value=toDraft.value=agentDraft.value=''; cycleDraft.value=props.cycleId ?? props.initialCycleId ?? '';
  queue.value=(props.cycleId || props.initialCycleId)?'':rights.value.approve?'awaiting_approval':'';
  committedFilter.value={...(queue.value?{state:queue.value}:{}),...((props.cycleId || cycleDraft.value)?{cycle_id:props.cycleId || cycleDraft.value}:{})};searched.value=true;
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
    const fresh = await api.list(props.brandId, PAGE_SIZE, nextOffset, committedFilter.value);
    if (!current(ticket, "list", captured, session, readTicket)) return;
    if (props.cycleId && fresh.items.some(item => item.cycle_id !== props.cycleId)) throw new AdminApiError("Unexpected commission cycle", 502, "INVALID_RESPONSE");
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
  if (detail.value?.id !== id) { reason.value = ""; detail.value = null; detailTab.value="summary"; }
  try {
    const fresh = await api.read(props.brandId, id);
    if (!current(ticket, "detail", captured, session, readTicket) || fresh.id !== id) return;
    if (props.cycleId && fresh.cycle_id !== props.cycleId) throw new AdminApiError("Unexpected commission cycle", 502, "INVALID_RESPONSE");
    selected.value = fresh; detail.value = fresh;
  } catch (cause) { if (current(ticket, "detail", captured, session, readTicket)) { sessionExpired(cause, captured, session); detailError.value = readFailure(cause); } }
  finally { if (laneCurrent(ticket, "detail", captured, session)) loadingDetail.value = false; }
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
  reviewSummary.value=detail.value && detail.value.id===intent.targetId && detail.value.version===intent.body.version ? {cycleId:detail.value.cycle_id,points:detail.value.total_points,count:detail.value.target_count}:null;
  dialog.value = intent; confirmed.value = false; writeError.value = "";
}
async function submit(intent: PendingCommissionPaymentIntent, recovery = Boolean(unknownIntent.value && unknownIntent.value.key === intent.key)) {
  if (writing.value || !confirmed.value || !rights.value.view || props.brandStatus === "disabled" || !validIntentScope(intent)) return;
  if ((intent.operation === "policy" && !rights.value.policyWrite) || (intent.operation === "approve" && !rights.value.approve) || (intent.operation === "retry" && !rights.value.retry)) return;
  if (!recovery && intent.operation !== "policy" && (!detail.value || detail.value.id !== intent.targetId || detail.value.version !== intent.body.version || loadingDetail.value)) return;
  if (props.cycleId && intent.operation !== "policy" && (!detail.value || detail.value.id !== intent.targetId || detail.value.cycle_id !== props.cycleId)) return;
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
    notice.value = message('admin.copy.1c78ca16');
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
  notice.value = message('admin.copy.93a931e1');
}
async function retryUnknown() {
  reviewSummary.value=null;
  const intent = unknownIntent.value;
  if (!intent || writing.value || !rights.value.view || props.brandStatus === "disabled" || !validIntentScope(intent)) return;
  if (props.cycleId && intent.operation !== "policy") {
    await loadDetail(intent.targetId!, true);
    if (unknownIntent.value !== intent || !detail.value || detail.value.id !== intent.targetId || detail.value.cycle_id !== props.cycleId || detailError.value) return;
  }
  dialog.value = intent; confirmed.value = false;
}
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

watch([fromDraft,toDraft,agentDraft,cycleDraft],dirtyFilters,{flush:'sync'});
watch(writing, value => emit("writing-change", value), { flush: "sync" });
watch(scopeStamp, resetScope, { immediate: true });
onUnmounted(() => { alive = false; generation++; policyTicket++; listTicket++; detailTicket++; writeTicket++; conflictTicket++; });
function closeDetails() {
  detailTicket++; paymentReadTicket++; historyTicket++;historyPage.value=null;historyBusy.value=false;historyError.value=''; selected.value = null; detail.value = null; loadingDetail.value = false; detailError.value = '';
}

useOperationToast(() => historyError.value,value=>displayPublicError(value),'error');
useOperationToast(() => policyError.value, value => displayPublicError(value), 'error');
useOperationToast(() => listError.value, value => displayPublicError(value), 'error');
useOperationToast(() => detailError.value, value => displayPublicError(value), 'error');
useOperationToast(() => notice.value, value => displayPublicError(value), 'info');
useOperationToast(() => writeError.value, value => displayPublicError(value), 'error');
</script>

<template>
 <article class="commission-workspace commission-payments">
  <header v-if="!selected && !detail && !loadingDetail && !detailError" class="console-page-heading commission-heading"><h1 v-if="!cycleId" class="console-page-title">{{ t('admin.copy.61a05916') }}</h1><div class="commission-actions console-page-actions"><span v-if="policy" class="commission-status" :class="policyEnabled?'ready':'stale'" data-testid="commission-payment-policy">{{ policyEnabled?t('admin.commission.ui.policyOn'):t('admin.commission.ui.policyOff') }}</span></div></header>
  <p v-if="!rights.view" class="empty-state">{{ t('admin.copy.cdaf5831') }}</p>
  <template v-else>
   <p v-if="brandStatus==='disabled'" class="commission-warning">{{ t('admin.commission.ui.disabled') }}</p>
   <template v-if="!selected && !detail && !loadingDetail && !detailError">
    <nav class="commission-tabs" role="tablist"><button v-for="tab in [{value:'awaiting_approval',label:'pending'},{value:'paying',label:'processing'},{value:'needs_attention',label:'attention'},{value:'',label:'all'}]" :key="tab.value" class="button" role="tab" :aria-selected="queue===tab.value" :disabled="loadingList" @click="withGlobalLoading(chooseQueue,tab.value)">{{ t(adminKey('admin.commission.ui.'+tab.label)) }}</button></nav>
    <ManagementList><template #filters><form v-inline-validation novalidate class="commission-toolbar ui-filter-bar" @submit.prevent="withGlobalLoading(applyFilters)">
     <label><PlatformTemporalInput :label="t('admin.commission.ui.from')" :placeholder="t('admin.commission.ui.from')" v-model="fromDraft" type="date" :locale="locale" /></label><label><PlatformTemporalInput :label="t('admin.commission.ui.to')" :placeholder="t('admin.commission.ui.to')" v-model="toDraft" type="date" :locale="locale" /></label>
     <label v-if="!cycleId"><FilterAssociationSelect v-model="cycleDraft" :brand-id="brandId" surface="admin" resource="commission" kind="cycle" :label="t('admin.commission.ui.cycle')" :locale="locale" :scope-key="scopeStamp" :enabled="rights.view" /></label>
     <label><FilterAssociationSearch v-model="agentDraft" v-model:pending="agentDraftSearchPending" :reset-key="accountSearchReset" :brand-id="brandId" surface="admin" resource="commission" kind="agent" :label="t('admin.commission.ui.agent')" :locale="locale" :scope-key="scopeStamp" :enabled="rights.view" /></label>
     <div class="commission-actions ui-form-actions"><button class="button button-primary" type="submit" :disabled="agentDraftSearchPending || loadingList">{{ t('admin.commission.ui.query') }}</button><button class="button" type="button" @click="withGlobalLoading(resetFilters)">{{ t('admin.commission.ui.reset') }}</button></div>
    </form></template>
    <p v-if="!searched" class="commission-note">{{ t('admin.commission.ui.filtersChanged') }}</p>
    <p v-if="listError" class="commission-warning" role="alert">{{ t('admin.commission.ui.readFailed') }} <button :disabled="agentDraftSearchPending" v-refresh-button="t('admin.commission.ui.retryRead')" class="button" @click="withGlobalLoading(applyFilters)">{{ t('admin.commission.ui.retryRead') }}</button></p>
    <p v-if="loadingList" v-global-loading>{{ t('admin.copy.ab7239f3') }}</p>
    <ManagementTable :entries="page?.items??[]" :columns="columns" :empty="!!page && !page.items.length" :empty-label="t('admin.commission.ui.empty')" data-testid="commission-payments-jobs">
     <template #cycle="{entry}"><button class="text-button commission-link" :data-payment-id="entry.id" @click="withGlobalLoading(loadDetail,entry.id)"><AssociationName :record="entry" kind="cycle" :id="entry.cycle_id" :locale="locale" /></button></template>
     <template #state="{entry}"><span class="commission-status" :class="entry.state">{{ stateLabel(entry.state) }}</span></template>
     <template #mode="{entry}">{{ modeLabel(entry.payout_mode) }}</template><template #planned="{entry}">{{ entry.total_points }}</template><template #paid="{entry}">{{ entry.paid_points }}</template><template #progress="{entry}">{{ entry.paid_count }} / {{ entry.target_count }}</template><template #updated="{entry}">{{ formatDate(entry.updated_at) }}</template>
     <template #actions="{entry}"><MemberActionsMenu :id="'CommissionPaymentsManagement-'+entry.id" :label="t('admin.copy.d21fcbef')"><button type="button" role="menuitem" class="button" @click="withGlobalLoading(loadDetail,entry.id)">{{ t('admin.commission.ui.view') }}</button></MemberActionsMenu></template>
    </ManagementTable>
    <template #pagination><TablePagination :show-empty="Boolean(page)" v-if="page" class="commission-pager" :total="page.total_count" :page="Math.floor(page.offset / PAGE_SIZE + 1)" :count="page.items.length" :locale="associationLocale" :page-size="PAGE_SIZE" :previous-disabled="loadingList || !hasPrevious" :next-disabled="loadingList || !hasNext" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.2e9c9e7a')" :go-to="(target: number) => loadList((target - 1) * PAGE_SIZE)" /></template></ManagementList>
   </template>
   <section v-if="selected || detail || loadingDetail || detailError" class="commission-detail">
    <header class="console-page-heading commission-heading"><div class="commission-actions"><button class="button" @click="closeDetails">{{ t('admin.commission.ui.back') }}</button><h1 v-if="!cycleId" class="console-page-title">{{ t('admin.copy.61a05916') }}</h1></div><button v-if="detail" class="button" @click="emit('open-cycle',detail.cycle_id)">{{ t('admin.commission.ui.openCycle') }}</button></header>
    <p v-if="loadingDetail" v-global-loading>{{ t('admin.copy.b6d0df6c') }}</p>
    <p v-if="detailError" class="commission-warning" role="alert">{{ displayPublicError(detailError) }}</p>
    <template v-if="detail && !loadingDetail && !detailError">
     <div class="commission-heading"><b><AssociationName :record="detail" kind="cycle" :id="detail.cycle_id" :locale="locale" /></b><span class="commission-status" :class="detail.state">{{ stateLabel(detail.state) }}</span></div>
     <p v-if="detail.last_error_code" class="commission-warning">{{ displayPublicError(publicErrorMessage({code:detail.last_error_code})) }}</p>
     <p v-if="policy && !policy.enabled" class="commission-warning">{{ t('admin.commission.ui.policyOff') }}</p>
     <p v-if="policyError" class="commission-warning" role="alert">{{ displayPublicError(policyError) }} <button v-refresh-button="t('admin.commission.ui.retryRead')" class="button" @click="withGlobalLoading(loadPolicy)">{{ t('admin.commission.ui.retryRead') }}</button></p>
     <header class="console-page-heading"><div class="commission-actions console-page-actions"><button v-if="detail.state==='awaiting_approval' && rights.approve" data-testid="commission-payment-approve" class="button button-primary" :disabled="!canApprove" @click="openReview('approve')">{{ t('admin.copy.c176586a') }}</button><button v-if="detail.state==='failed' && rights.retry" data-testid="commission-payment-retry" class="button button-primary" :disabled="!canRetry" @click="openReview('retry')">{{ t('admin.copy.ce7fd4f3') }}</button><button v-refresh-button="t('admin.copy.894e2a7d')" class="button" :disabled="loadingDetail" @click="withGlobalLoading(loadDetail,detail.id)">{{ t('admin.copy.894e2a7d') }}</button></div></header>
     <nav class="commission-tabs" role="tablist"><button v-for="tab in (['summary','targets','adjustments','paymentHistory'] as const)" :key="tab" class="button" role="tab" :aria-selected="detailTab===tab" @click="withGlobalLoading(openDetailTab,tab)">{{ t(adminKey('admin.commission.ui.'+tab)) }}</button></nav>
     <section v-if="detailTab==='summary'" class="commission-detail" role="tabpanel">
      <dl class="commission-metrics"><div><dt>{{ t('admin.commission.ui.planned') }}</dt><dd>{{ detail.total_points }}</dd></div><div><dt>{{ t('admin.commission.ui.paid') }}</dt><dd>{{ detail.paid_points }}</dd></div><div><dt>{{ t('admin.commission.ui.progress') }}</dt><dd>{{ detail.paid_count }} / {{ detail.target_count }}</dd></div></dl>
      <dl class="commission-facts"><dt>{{ t('admin.commission.ui.mode') }}</dt><dd>{{ modeLabel(detail.payout_mode) }}</dd><dt>{{ t('admin.commission.ui.updated') }}</dt><dd>{{ formatDate(detail.updated_at) }}</dd></dl>
      <p v-if="detail.payout_mode==='mixed' && detail.state==='awaiting_approval'" class="commission-warning">{{ t('admin.copy.f52ae3e2') }}</p><p v-if="detail.state==='blocked'" class="commission-warning">{{ t('admin.copy.df5f714e') }}</p><p v-if="detail.state==='failed' && detail.paid_count!=='0'" class="commission-warning">{{ t('admin.copy.a28d321d') }}</p>
     </section>
     <CommissionPaymentTargets v-if="detailTab==='targets'" :key="detail.id+':'+detail.version+':'+detail.paid_count" :account="account" :brand-id="brandId" :payment-id="detail.id" @session-invalid="emit('session-invalid')" />
     <template v-if="detailTab==='adjustments'"><p v-if="detail.state!=='paid'" class="commission-note">{{ t('admin.commission.ui.noAdjustment') }}</p><CommissionAdjustmentsManagement :key="`commission-adjustments:${account.id}:${brandId}:${detail.id}`" :account="account" :brand-id="brandId" :payment-id="detail.id" :payment-state="detail.state" :brand-status="brandStatus" @session-invalid="emit('session-invalid')" /></template>
     <ManagementList embedded v-if="detailTab==='paymentHistory'" class="commission-detail"><p v-if="historyError" class="commission-warning">{{ displayPublicError(historyError) }} <button v-refresh-button="t('admin.commission.ui.retryRead')" class="button" @click="withGlobalLoading(loadHistory)">{{ t('admin.commission.ui.retryRead') }}</button></p>
      <ManagementTable :entries="historyPage?.items??[]" :columns="historyColumns" :empty="!!historyPage&&!historyPage.items.length" :empty-label="t('admin.commission.ui.empty')"><template #operation="{entry}">{{ t(adminKey('admin.commission.ui.'+entry.operation)) }}</template><template #actor="{entry}"><AssociationName v-if="entry.actor_type==='admin'&&entry.actor_id" :record="entry" kind="actor" :id="entry.actor_id" :locale="locale" /><span v-else>{{ t('admin.commission.ui.system') }}</span></template><template #state="{entry}">{{ entry.state ? stateLabel(entry.state) : '—' }}</template><template #time="{entry}">{{ formatDate(entry.created_at) }}</template></ManagementTable>

     <template #pagination><TablePagination :show-empty="Boolean(historyPage)" v-if="historyPage" class="commission-pager" :total="historyPage.total_count" :page="Math.floor(historyPage.offset / 20 + 1)" :count="historyPage.items.length" :locale="associationLocale" :page-size="20" :previous-disabled="historyBusy||historyPage.offset===0" :next-disabled="historyBusy||BigInt(historyPage.offset+20)>=BigInt(historyPage.total_count)" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.2e9c9e7a')" :go-to="(target: number) => loadHistory((target - 1) * 20)" /></template></ManagementList>

    </template>
   </section>
      <div v-if="writeError" class="operation-recovery-actions ui-form-actions"><button v-write-guard="writing" v-if="unknownIntent && rights.view" class="button" type="button" :disabled="writing || (unknownIntent.operation === 'policy' ? !rights.policyWrite : unknownIntent.operation === 'approve' ? !rights.approve : !rights.retry)" @click="withGlobalLoading(retryUnknown)">{{ writing ? t('admin.copy.a3df712a') : t('admin.copy.c6a6aa0c') }}</button><button v-refresh-button="t('admin.copy.6b2dcb02')" v-if="conflict" class="button" type="button" :disabled="conflictReloading" @click="withGlobalLoading(reloadConflict)">{{ t('admin.copy.6b2dcb02') }}</button></div>
      <section v-if="conflict && conflictIntent" class="cp-conflict" data-testid="commission-payment-conflict"><p>{{ conflictRefreshed ? t('admin.copy.281d8e9a') : t('admin.copy.c10e2006') }}</p><label class="cp-confirm"><input v-model="discardReviewed" type="checkbox" :disabled="!conflictRefreshed || conflictReloading" />{{ t('admin.copy.d52d7181') }}</label><div class="cp-actions ui-form-actions"><button v-refresh-button="conflictReloading ? t('admin.copy.948fc498') : t('admin.copy.16a3182a')" class="button" type="button" :disabled="conflictReloading" @click="withGlobalLoading(reloadConflict)">{{ conflictReloading ? t('admin.copy.948fc498') : t('admin.copy.16a3182a') }}</button><button class="button button-primary" type="button" :disabled="!conflictRefreshed || !discardReviewed || conflictReloading" @click="discardConflictIntent">{{ t('admin.copy.7107e7f5') }}</button></div></section>
      <section v-if="receipt" class="cp-receipt" role="status" aria-live="polite" data-testid="commission-payment-receipt"><b>{{ t('admin.copy.d544aeb2') }}</b><span>{{ displayReceipt() }}</span><code v-if="receipt.payment">{{ receipt.payment.id }}</code></section>
      <DetailModal v-if="dialog" class="confirmation-dialog cp-dialog" :title="t('admin.copy.08cf02f3')"  :close-label="t('association.close')" :close-disabled="writing" @close="if (!(writing)) { dialog = null; confirmed = false; }" data-testid="commission-payment-review">

        <p v-if="dialog.operation === 'policy' && 'enabled' in dialog.body && dialog.body.enabled" class="cp-callout" data-testid="commission-payment-enable-warning">{{ t('admin.copy.91b7e7bd') }}</p>
        <p v-if="unknownIntent" class="commission-warning">{{ t('admin.commission.ui.requestUnknown') }}</p>
        <dl v-if="reviewSummary" class="commission-facts"><dt>{{ t('admin.commission.ui.cycle') }}</dt><dd><AssociationName :record="detail" kind="cycle" :id="reviewSummary.cycleId" :locale="locale" /></dd><dt>{{ t('admin.commission.ui.planned') }}</dt><dd>{{ reviewSummary.points }}</dd><dt>{{ t('admin.commission.ui.agents') }}</dt><dd>{{ reviewSummary.count }}</dd></dl>
        <dl>
          <dt>{{ t('admin.copy.9c32661d') }}</dt><dd><AssociationName :record="detail" kind="brand" :id="dialog.brandId" :locale="associationLocale" /></dd>
          <dt>{{ t('admin.copy.087a786d') }}</dt><dd>{{ operationLabel(dialog.operation) }}</dd>

          <dt v-if="'enabled' in dialog.body">{{ t('admin.copy.41210f20') }}</dt><dd v-if="'enabled' in dialog.body">{{ dialog.body.enabled ? t('admin.copy.de0e628f') : t('admin.copy.3565cde9') }}</dd>
          <dt v-if="dialog.body.reason">{{ t('admin.copy.5ec7c8ba') }}</dt><dd v-if="dialog.body.reason">{{ dialog.body.reason }}</dd>

        </dl>

        <label class="cp-confirm"><input data-testid="commission-payment-review-confirmed" v-model="confirmed" type="checkbox" />{{ t('admin.copy.2028b20d') }}</label>
        <div class="cp-actions ui-form-actions"><button v-write-guard="writing" data-testid="commission-payment-confirm-submit" class="button button-primary" type="button" :disabled="writing || !confirmed" @click="withGlobalLoading(submit, dialog)">{{ writing ? t('admin.copy.46511dc0') : t('admin.copy.02c1e356') }}</button></div>
      </DetailModal>
  </template>
 </article>
</template>
<style scoped>
.cp-conflict,.cp-receipt { padding:12px;border:1px solid var(--line);border-radius:10px;overflow-wrap:anywhere; }
.cp-confirm { display:flex;align-items:center;gap:8px;min-height:44px; }
.cp-dialog dl { display:grid;grid-template-columns:auto minmax(0,1fr);gap:6px 12px; }
.cp-dialog dd { margin:0;overflow-wrap:anywhere; }
.cp-receipt { display:grid;gap:6px;border-color:var(--success); }
</style>
