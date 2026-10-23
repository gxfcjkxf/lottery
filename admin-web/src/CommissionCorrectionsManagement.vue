<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import DetailModal from "@lottery/shared/DetailModal.vue"
import { computed, onUnmounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { adminKey, useAdminI18n } from "./i18n";
import { createCommissionPaymentsApi } from "./commissionPayments-api";
import {
  commissionCorrectionPermissions, createCommissionCorrectionsApi,
  type CommissionCorrectionActionBody, type CommissionCorrectionPolicy, type CommissionCorrectionPolicyBody, type CorrectionPlan, type CorrectionPlanTarget,
  type CorrectionExecution, type CorrectionExecutionTarget,
} from "./commission-corrections-api";
import {
  classifyCommissionCorrectionWriteFailure, clearAllCommissionCorrectionIntents,
  clearPendingCommissionCorrectionIntent, commissionCorrectionSessionGeneration,
  createCommissionCorrectionIntent, freezeCommissionCorrectionBody,
  hideCommissionCorrectionIntentsForScope, isCommissionCorrectionIntentConflict,
  listPendingCommissionCorrectionIntents, markCommissionCorrectionIntentConflict,
  setPendingCommissionCorrectionIntent,
  type CommissionCorrectionOperation, type PendingCommissionCorrectionIntent,
} from "./commission-corrections-state";

const props = defineProps<{ account: AdminAccount; brandId: string; brandStatus?: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, locale } = useAdminI18n();
const operationLabel = (operation: CommissionCorrectionOperation) => t({ policy: 'admin.copy.3a0deb6f', plan_retry: 'admin.copy.18acbe2a', approve: 'admin.copy.028518f0', continue: 'admin.copy.cdb10f42', execute_retry: 'admin.copy.50dae2bb' }[operation] as Parameters<typeof t>[0]);
const api = computed(() => createCommissionCorrectionsApi(fetch, props.account.id));
const paymentsApi = computed(() => createCommissionPaymentsApi(fetch, props.account.id));
const PAGE_SIZE = 20, TARGET_PAGE_SIZE = 100;
const rights = computed(() => commissionCorrectionPermissions(props.account, props.brandId));
const scopeStamp = computed(() => JSON.stringify([props.account.id, props.account.super_admin, props.account.brand_ids,
  props.account.permissions, props.account.permissions_by_brand, props.account.platform_permissions, props.brandId, props.brandStatus, rights.value]));
const policy = ref<CommissionCorrectionPolicy | null>(null), planPage = ref<Awaited<ReturnType<ReturnType<typeof createCommissionCorrectionsApi>["listPlans"]>> | null>(null);
const executionPage = ref<Awaited<ReturnType<ReturnType<typeof createCommissionCorrectionsApi>["listExecutions"]>> | null>(null);
const plan = ref<CorrectionPlan | null>(null), execution = ref<CorrectionExecution | null>(null);
const originalPaymentRun = ref<string | null>(null);
const planTargets = ref<CorrectionPlanTarget[]>([]), executionTargets = ref<CorrectionExecutionTarget[]>([]);
const planTargetsTotal = ref("0"), executionTargetsTotal = ref("0"), planTargetsOffset = ref(0), executionTargetsOffset = ref(0);
const selectedPlanId = ref(""), selectedExecutionId = ref("");
const receipt = ref<{ operation: CommissionCorrectionOperation; value: unknown } | null>(null);
const loading = ref(false), loadingPolicy = ref(false), loadingPlans = ref(false), loadingExecutions = ref(false), loadingTargets = ref(false), writing = ref(false);
const errors = ref<Record<string, string | import("@lottery/shared").LocalizedMessage>>({}), writeError = ref<string | import("@lottery/shared").LocalizedMessage>("");
const notice = ref<"confirmed" | "discarded" | "">("");
const noticeText = computed(() => notice.value === "confirmed"
  ? t('admin.copy.064adf92')
  : notice.value === "discarded"
    ? t('admin.copy.da960a7d') : "");
const reason = ref(""), policyEnabled = ref(false), confirmed = ref(false), dialog = ref<PendingCommissionCorrectionIntent | null>(null);
const unknownIntent = ref<PendingCommissionCorrectionIntent | null>(null), conflictIntent = ref<PendingCommissionCorrectionIntent | null>(null);
const conflict = ref(false), conflictReloading = ref(false), conflictRefreshed = ref(false), discardReviewed = ref(false);
const planOffset = ref(0), executionOffset = ref(0);
let alive = true, policyTicket = 0, plansTicket = 0, executionsTicket = 0, detailTicket = 0, targetsTicket = 0, writeTicket = 0, conflictTicket = 0;

const busyRead = computed(() => loading.value || loadingPolicy.value || loadingPlans.value || loadingExecutions.value || loadingTargets.value);
const planTotal = computed(() => planPage.value?.total_count ?? "0"), executionTotal = computed(() => executionPage.value?.total_count ?? "0");
const canWrite = computed(() => rights.value.view && props.brandStatus !== "disabled" && !props.account.super_admin && !writing.value && !unknownIntent.value && !conflict.value);
const canPolicy = computed(() => canWrite.value && !busyRead.value && !loadingTargets.value && !errors.value.policy && rights.value.policyWrite && policy.value && policy.value.version < Number.MAX_SAFE_INTEGER && validReason(reason.value));
const canPlanRetry = computed(() => canWrite.value && !busyRead.value && !loadingTargets.value && !errors.value.plan && !errors.value.plans && rights.value.planRetry && plan.value?.state === "failed" && plan.value.version < Number.MAX_SAFE_INTEGER && validReason(reason.value));
const canApprove = computed(() => canWrite.value && !busyRead.value && !loadingTargets.value && !errors.value.execution && !errors.value.executions && rights.value.approve && execution.value?.state === "awaiting_approval" && ["manual", "mixed"].includes(execution.value.payout_mode) && execution.value.version < Number.MAX_SAFE_INTEGER && validReason(reason.value));
const canContinue = computed(() => canWrite.value && !busyRead.value && !loadingTargets.value && !errors.value.execution && !errors.value.executions && rights.value.continue && execution.value?.state === "paused" && Boolean(execution.value.approval_actor_type) && Boolean(execution.value.approval_audit_log_id) && execution.value.cycle_hold_active === true && execution.value.version < Number.MAX_SAFE_INTEGER && validReason(reason.value));
const canRetry = computed(() => canWrite.value && !busyRead.value && !loadingTargets.value && !errors.value.execution && !errors.value.executions && rights.value.retry && execution.value?.state === "failed" && execution.value.version < Number.MAX_SAFE_INTEGER && validReason(reason.value));
const canResolveUnknown = computed(() => Boolean(unknownIntent.value && rights.value.view && props.brandStatus !== "disabled" &&
  (unknownIntent.value.operation === "policy" ? rights.value.policyWrite : unknownIntent.value.operation === "plan_retry" ? rights.value.planRetry : unknownIntent.value.operation === "approve" ? rights.value.approve : unknownIntent.value.operation === "continue" ? rights.value.continue : rights.value.retry)));

function validReason(value: string): boolean {
  if (value.trim() !== value || new TextEncoder().encode(value).length > 500 || /[\u0000-\u001f\u007f-\u009f]/u.test(value)) return false;
  for (let i = 0; i < value.length; i++) { const code = value.charCodeAt(i); if (code >= 0xd800 && code <= 0xdbff) { const next = value.charCodeAt(++i); if (!(next >= 0xdc00 && next <= 0xdfff)) return false; } else if (code >= 0xdc00 && code <= 0xdfff) return false; }
  return true;
}
function current(ticket: number, lane: string, captured: string, session: number): boolean {
  const laneTicket = lane === "policy" ? policyTicket : lane === "plans" ? plansTicket : lane === "executions" ? executionsTicket : lane === "detail" ? detailTicket : targetsTicket;
  return alive && ticket === laneTicket && captured === scopeStamp.value && session === commissionCorrectionSessionGeneration() && rights.value.view;
}
function currentWrite(ticket: number, captured: string, session: number): boolean { return alive && ticket === writeTicket && captured === scopeStamp.value && session === commissionCorrectionSessionGeneration(); }
function clearVisible() {
  policy.value = null; planPage.value = null; executionPage.value = null; plan.value = null; execution.value = null;
  planTargets.value = []; executionTargets.value = []; planTargetsTotal.value = executionTargetsTotal.value = "0"; planTargetsOffset.value = executionTargetsOffset.value = 0; selectedPlanId.value = ""; selectedExecutionId.value = ""; originalPaymentRun.value = null;
  receipt.value = null; reason.value = ""; dialog.value = null; confirmed.value = false;
  unknownIntent.value = null; conflictIntent.value = null; conflict.value = false; conflictRefreshed.value = false; discardReviewed.value = false;
  errors.value = {}; writeError.value = ""; notice.value = ""; loading.value = loadingPolicy.value = loadingPlans.value = loadingExecutions.value = loadingTargets.value = writing.value = false;
}
function resetScope(_next?: string, previous?: string) {
  if (previous) { try { const old = JSON.parse(previous) as unknown[]; hideCommissionCorrectionIntentsForScope(String(old[0]), String(old[6])); } catch { /* Old scope cannot reveal an intent. */ } }
  policyTicket++; plansTicket++; executionsTicket++; detailTicket++; targetsTicket++; writeTicket++; conflictTicket++;
  clearVisible();
  const pending = listPendingCommissionCorrectionIntents(props.account.id, props.brandId);
  const conflictPending = pending.find(isCommissionCorrectionIntentConflict);
  conflictIntent.value = conflictPending ?? null; conflict.value = Boolean(conflictPending);
  unknownIntent.value = conflictPending ? null : pending[0] ?? null;
  if (unknownIntent.value) writeError.value = t('admin.copy.edf12fc2');
  if (rights.value.view) void reloadAll();
}
function sessionExpired(cause: unknown, captured: string, session: number): boolean {
  if (cause instanceof AdminApiError && cause.status === 401 && alive && captured === scopeStamp.value && session === commissionCorrectionSessionGeneration()) {
    policyTicket++; plansTicket++; executionsTicket++; detailTicket++; targetsTicket++; writeTicket++; clearAllCommissionCorrectionIntents(); clearVisible(); emit("session-invalid"); return true;
  }
  return false;
}
function message(cause: unknown): string | import("@lottery/shared").LocalizedMessage { return cause instanceof AdminApiError && cause.status === 0 ? t('admin.copy.a230479a') : cause instanceof Error ? publicErrorMessage(cause, 'read') : t('admin.copy.466e55a6'); }
function reportReadError(lane: string, cause: unknown, captured: string, session: number) { sessionExpired(cause, captured, session); errors.value = { ...errors.value, [lane]: message(cause) }; }
async function loadPolicy(preserve = false) {
  if (!rights.value.view) return; if (!preserve) invalidateConflictReview(); invalidateActionReview();
  const ticket = ++policyTicket, captured = scopeStamp.value, session = commissionCorrectionSessionGeneration();
  loadingPolicy.value = true;
  try { const value = await api.value.getPolicy(props.brandId); if (!current(ticket, "policy", captured, session)) return; policy.value = value; policyEnabled.value = value.enabled; delete errors.value.policy; }
  catch (cause) { if (current(ticket, "policy", captured, session)) reportReadError("policy", cause, captured, session); }
  finally { if (ticket === policyTicket) loadingPolicy.value = false; }
}
async function loadPlans(offset = planOffset.value, preserve = false) {
  if (!rights.value.view) return; if (!preserve) invalidateConflictReview(); invalidateActionReview();
  const ticket = ++plansTicket, captured = scopeStamp.value, session = commissionCorrectionSessionGeneration();
  loadingPlans.value = true;
  try { const value = await api.value.listPlans(props.brandId, PAGE_SIZE, offset); if (!current(ticket, "plans", captured, session)) return; planPage.value = value; planOffset.value = offset; delete errors.value.plans;
    if (selectedPlanId.value && !value.items.some((item) => item.id === selectedPlanId.value)) { selectedPlanId.value = ""; plan.value = null; planTargets.value = []; }
  } catch (cause) { if (current(ticket, "plans", captured, session)) reportReadError("plans", cause, captured, session); }
  finally { if (ticket === plansTicket) loadingPlans.value = false; }
}
async function loadExecutions(offset = executionOffset.value, preserve = false) {
  if (!rights.value.view) return; if (!preserve) invalidateConflictReview(); invalidateActionReview();
  const ticket = ++executionsTicket, captured = scopeStamp.value, session = commissionCorrectionSessionGeneration();
  loadingExecutions.value = true;
  try { const value = await api.value.listExecutions(props.brandId, PAGE_SIZE, offset); if (!current(ticket, "executions", captured, session)) return; executionPage.value = value; executionOffset.value = offset; delete errors.value.executions;
    if (selectedExecutionId.value && !value.items.some((item) => item.id === selectedExecutionId.value)) { selectedExecutionId.value = ""; execution.value = null; executionTargets.value = []; }
  } catch (cause) { if (current(ticket, "executions", captured, session)) reportReadError("executions", cause, captured, session); }
  finally { if (ticket === executionsTicket) loadingExecutions.value = false; }
}
async function selectPlan(id: string, preserve = false) {
  if (!rights.value.view) return; if (!preserve) invalidateConflictReview(); invalidateActionReview();
  selectedPlanId.value = id; plan.value = null; originalPaymentRun.value = null; planTargets.value = []; planTargetsTotal.value = "0"; planTargetsOffset.value = 0; const ticket = ++detailTicket, targetTicket = ++targetsTicket;
  const captured = scopeStamp.value, session = commissionCorrectionSessionGeneration(); loadingTargets.value = true;
  try {
    const [fresh, targetPage] = await Promise.all([api.value.readPlan(props.brandId, id), api.value.listPlanTargets(props.brandId, id, TARGET_PAGE_SIZE, 0)]);
    if (!current(ticket, "detail", captured, session) || targetTicket !== targetsTicket || id !== selectedPlanId.value) return;
    plan.value = fresh; planTargets.value = targetPage.items; planTargetsTotal.value = targetPage.total_count; delete errors.value.plan;
    try { const originalPayment = await paymentsApi.value.read(props.brandId, fresh.payment_id); if (current(ticket, "detail", captured, session) && id === selectedPlanId.value) originalPaymentRun.value = originalPayment.run_id; }
    catch (cause) { if (current(ticket, "detail", captured, session) && id === selectedPlanId.value) errors.value = { ...errors.value, originalPayment: message(cause) }; }
  } catch (cause) { if (current(ticket, "detail", captured, session) && targetTicket === targetsTicket) reportReadError("plan", cause, captured, session); }
  finally { if (alive && targetTicket === targetsTicket && captured === scopeStamp.value) loadingTargets.value = false; }
}
async function selectExecution(id: string, preserve = false) {
  if (!rights.value.view) return; if (!preserve) invalidateConflictReview(); invalidateActionReview();
  selectedExecutionId.value = id; execution.value = null; executionTargets.value = []; executionTargetsTotal.value = "0"; executionTargetsOffset.value = 0; const ticket = ++detailTicket, targetTicket = ++targetsTicket;
  const captured = scopeStamp.value, session = commissionCorrectionSessionGeneration(); loadingTargets.value = true;
  try {
    const [fresh, targetPage] = await Promise.all([api.value.readExecution(props.brandId, id), api.value.listExecutionTargets(props.brandId, id, TARGET_PAGE_SIZE, 0)]);
    if (!current(ticket, "detail", captured, session) || targetTicket !== targetsTicket || id !== selectedExecutionId.value) return;
    execution.value = fresh; executionTargets.value = targetPage.items; executionTargetsTotal.value = targetPage.total_count; delete errors.value.execution;
  } catch (cause) { if (current(ticket, "detail", captured, session) && targetTicket === targetsTicket) reportReadError("execution", cause, captured, session); }
  finally { if (alive && targetTicket === targetsTicket && captured === scopeStamp.value) loadingTargets.value = false; }
}
async function loadPlanTargetPage(offset: number) {
  if (!plan.value || !rights.value.view) return; invalidateConflictReview(); invalidateActionReview();
  const id = plan.value.id, ticket = ++targetsTicket, captured = scopeStamp.value, session = commissionCorrectionSessionGeneration(); loadingTargets.value = true;
  try { const page = await api.value.listPlanTargets(props.brandId, id, TARGET_PAGE_SIZE, offset); if (!current(ticket, "targets", captured, session) || selectedPlanId.value !== id) return; planTargets.value = page.items; planTargetsTotal.value = page.total_count; planTargetsOffset.value = offset; delete errors.value.planTargets; }
  catch (cause) { if (current(ticket, "targets", captured, session)) reportReadError("planTargets", cause, captured, session); }
  finally { if (alive && ticket === targetsTicket && captured === scopeStamp.value) loadingTargets.value = false; }
}
async function loadExecutionTargetPage(offset: number) {
  if (!execution.value || !rights.value.view) return; invalidateConflictReview(); invalidateActionReview();
  const id = execution.value.id, ticket = ++targetsTicket, captured = scopeStamp.value, session = commissionCorrectionSessionGeneration(); loadingTargets.value = true;
  try { const page = await api.value.listExecutionTargets(props.brandId, id, TARGET_PAGE_SIZE, offset); if (!current(ticket, "targets", captured, session) || selectedExecutionId.value !== id) return; executionTargets.value = page.items; executionTargetsTotal.value = page.total_count; executionTargetsOffset.value = offset; delete errors.value.executionTargets; }
  catch (cause) { if (current(ticket, "targets", captured, session)) reportReadError("executionTargets", cause, captured, session); }
  finally { if (alive && ticket === targetsTicket && captured === scopeStamp.value) loadingTargets.value = false; }
}
async function reloadAll(preserve = false) {
  if (!rights.value.view) return; loading.value = true;
  await Promise.all([loadPolicy(preserve), loadPlans(0, preserve), loadExecutions(0, preserve)]);
  if (selectedPlanId.value) await selectPlan(selectedPlanId.value, preserve);
  if (selectedExecutionId.value) await selectExecution(selectedExecutionId.value, preserve);
  loading.value = false;
}
function invalidateConflictReview() { if (!conflict.value) return; conflictTicket++; conflictReloading.value = false; conflictRefreshed.value = false; discardReviewed.value = false; }
function invalidateActionReview() { if (!dialog.value || unknownIntent.value) return; dialog.value = null; confirmed.value = false; }
function scopeFor(operation: CommissionCorrectionOperation, targetId?: string) { return { actorId: props.account.id, brandId: props.brandId, operation, ...(targetId ? { targetId } : {}) }; }
function buildIntent(operation: CommissionCorrectionOperation): PendingCommissionCorrectionIntent | null {
  const target = operation === "policy" ? undefined : operation === "plan_retry" ? plan.value : execution.value;
  if (operation === "policy") {
    if (!canPolicy.value) return null;
    return createCommissionCorrectionIntent(scopeFor(operation), freezeCommissionCorrectionBody(scopeFor(operation), { version: policy.value!.version, enabled: policyEnabled.value, reason: reason.value }), createIdempotencyKey());
  }
  if (!target || target.version >= Number.MAX_SAFE_INTEGER || !validReason(reason.value)) return null;
  const allowed = operation === "plan_retry" ? canPlanRetry.value : operation === "approve" ? canApprove.value : operation === "continue" ? canContinue.value : canRetry.value;
  if (!allowed) return null;
  const scope = scopeFor(operation, target.id);
  return createCommissionCorrectionIntent(scope, freezeCommissionCorrectionBody(scope, { version: target.version, reason: reason.value }), createIdempotencyKey());
}
function openReview(operation: CommissionCorrectionOperation) {
  if (unknownIntent.value || conflict.value) return;
  const intent = buildIntent(operation); if (!intent) return;
  dialog.value = intent; confirmed.value = false; writeError.value = "";
}
function retryUnknown() { if (!unknownIntent.value || !canResolveUnknown.value) return; dialog.value = unknownIntent.value; confirmed.value = false; }
function validIntentScope(intent: PendingCommissionCorrectionIntent): boolean { return intent.actorId === props.account.id && intent.brandId === props.brandId && (intent.operation === "policy" ? intent.targetId === undefined : Boolean(intent.targetId)); }
async function submit(intent: PendingCommissionCorrectionIntent) {
  const recovery = Boolean(unknownIntent.value && unknownIntent.value.key === intent.key);
  if (writing.value || busyRead.value || loadingTargets.value || !confirmed.value || !rights.value.view || props.brandStatus === "disabled" || !validIntentScope(intent)) return;
  if (!recovery && (unknownIntent.value || conflict.value)) return;
  if ((intent.operation === "policy" && !rights.value.policyWrite) || (intent.operation === "plan_retry" && !rights.value.planRetry) || (intent.operation === "approve" && !rights.value.approve) || (intent.operation === "continue" && !rights.value.continue) || (intent.operation === "execute_retry" && !rights.value.retry)) return;
  if (!recovery && intent.operation !== "policy") {
    const selectedRow = intent.operation === "plan_retry" ? plan.value : execution.value;
    if (!selectedRow || selectedRow.id !== intent.targetId || selectedRow.version !== intent.body.version || loadingTargets.value) return;
  }
  const captured = scopeStamp.value, session = commissionCorrectionSessionGeneration(), ticket = ++writeTicket;
  writing.value = true; writeError.value = ""; notice.value = ""; setPendingCommissionCorrectionIntent(scopeFor(intent.operation, intent.targetId), intent);
  try {
    let value: unknown;
    if (intent.operation === "policy") value = await api.value.updatePolicy(intent.brandId, intent.body as Readonly<CommissionCorrectionPolicyBody>, intent.key);
    else if (intent.operation === "plan_retry") value = await api.value.retryPlan(intent.brandId, intent.targetId!, intent.body as Readonly<CommissionCorrectionActionBody>, intent.key);
    else if (intent.operation === "approve") value = await api.value.approve(intent.brandId, intent.targetId!, intent.body as Readonly<CommissionCorrectionActionBody>, intent.key);
    else if (intent.operation === "continue") value = await api.value.continue(intent.brandId, intent.targetId!, intent.body as Readonly<CommissionCorrectionActionBody>, intent.key);
    else value = await api.value.retry(intent.brandId, intent.targetId!, intent.body as Readonly<CommissionCorrectionActionBody>, intent.key);
    // An exact server ACK consumes only the intent from the same in-memory auth session.
    // Expected-key matching prevents an old ACK from deleting a replacement slot.
    if (session === commissionCorrectionSessionGeneration()) clearPendingCommissionCorrectionIntent(scopeFor(intent.operation, intent.targetId), intent.key);
    if (!currentWrite(ticket, captured, session)) return;
    receipt.value = { operation: intent.operation, value }; dialog.value = null; unknownIntent.value = null; confirmed.value = false;
    notice.value = "confirmed";
    plan.value = null; execution.value = null; planTargets.value = []; executionTargets.value = [];
    await reloadAll();
  } catch (cause) {
    const status = cause instanceof AdminApiError ? cause.status : 0, outcome = classifyCommissionCorrectionWriteFailure(status);
    if (status === 401) {
      const sameOriginalSession = session === commissionCorrectionSessionGeneration() && props.account.id === intent.actorId;
      if (sameOriginalSession) {
        clearAllCommissionCorrectionIntents();
        policyTicket++; plansTicket++; executionsTicket++; detailTicket++; targetsTicket++; writeTicket++; clearVisible(); emit("session-invalid");
      }
      return;
    }
    if (outcome === "conflict") {
      // Keep and mark only the matching intent in the same auth session. A late
      // 409 may update hidden memory state, but never reopens UI in another scope.
      if (session === commissionCorrectionSessionGeneration()) markCommissionCorrectionIntentConflict(intent);
      if (!currentWrite(ticket, captured, session)) return;
      unknownIntent.value = null; conflictIntent.value = intent; dialog.value = null; conflict.value = true; conflictRefreshed.value = false; discardReviewed.value = false;
      writeError.value = t('admin.copy.b85d5b60');
      return;
    }
    if (!currentWrite(ticket, captured, session)) { if (outcome !== "unknown") clearPendingCommissionCorrectionIntent(scopeFor(intent.operation, intent.targetId), intent.key); return; }
    if (sessionExpired(cause, captured, session)) { clearPendingCommissionCorrectionIntent(scopeFor(intent.operation, intent.targetId), intent.key); return; }
    if (outcome === "unknown") {
      setPendingCommissionCorrectionIntent(scopeFor(intent.operation, intent.targetId), intent); unknownIntent.value = intent; dialog.value = null;
      writeError.value = t('admin.copy.f05cbc0c');
    } else {
      clearPendingCommissionCorrectionIntent(scopeFor(intent.operation, intent.targetId), intent.key); dialog.value = null;
      writeError.value = cause instanceof Error ? publicErrorMessage(cause) : t('admin.copy.7565cd98');
    }
  } finally { if (currentWrite(ticket, captured, session)) writing.value = false; }
}
async function refreshConflict() {
  const intent = conflictIntent.value; if (!conflict.value || !intent || conflictReloading.value) return;
  const ticket = ++conflictTicket, captured = scopeStamp.value;
  const planToRefresh = selectedPlanId.value || (intent.operation === "plan_retry" ? intent.targetId ?? "" : "");
  const executionToRefresh = selectedExecutionId.value || (intent.operation !== "policy" && intent.operation !== "plan_retry" ? intent.targetId ?? "" : "");
  conflictReloading.value = true; errors.value = {};
  const results = await Promise.allSettled([loadPolicy(true), loadPlans(planOffset.value, true), loadExecutions(executionOffset.value, true)]);
  if (planToRefresh) await selectPlan(planToRefresh, true);
  if (executionToRefresh) await selectExecution(executionToRefresh, true);
  const planRequired = Boolean(planToRefresh), executionRequired = Boolean(executionToRefresh);
  if (alive && ticket === conflictTicket && captured === scopeStamp.value && rights.value.view && results.every((result) => result.status === "fulfilled") && Object.keys(errors.value).length === 0 && (!planRequired || plan.value?.id === planToRefresh) && (!executionRequired || execution.value?.id === executionToRefresh) && Boolean(policy.value && planPage.value && executionPage.value)) {
    conflictRefreshed.value = true; discardReviewed.value = false;
    writeError.value = t('admin.copy.e58d7d85');
  }
  if (ticket === conflictTicket) conflictReloading.value = false;
}
function discardConflict() {
  const intent = conflictIntent.value; if (!intent || !conflictRefreshed.value || !discardReviewed.value || conflictReloading.value) return;
  clearPendingCommissionCorrectionIntent(scopeFor(intent.operation, intent.targetId), intent.key); conflictIntent.value = null; conflict.value = false;
  conflictRefreshed.value = false; discardReviewed.value = false; writeError.value = "";
  notice.value = "discarded";
}
function formatDate(value: string): string{ return formatDisplayDateTime(value, displayLocale.value); }
 function planState(state: string): string { const names: Record<string, string> = { planning:'admin.copy.30fa9fae',ready:'admin.copy.6b41f253',blocked:'admin.copy.4565cb00',failed:'admin.copy.1feabec0',stale:'admin.copy.38135e1a' }; return names[state] ? t(adminKey(names[state]!)) : state; }
 function executionState(state: string): string { const names: Record<string, string> = { awaiting_approval:'admin.copy.564c9922',applying:'admin.copy.4eb76ff3',completed:'admin.copy.293e79cc',paused:'admin.copy.40ecc18f',failed:'admin.copy.1feabec0',stale:'admin.copy.38135e1a' }; return names[state] ? t(adminKey(names[state]!)) : state; }
 function modeLabel(mode: string): string { const names: Record<string, string> = { manual:'admin.copy.6644f5f6',automatic:'admin.copy.4b7603bc',mixed:'admin.copy.2e8f9df9',none:'admin.copy.c2b41b33' }; return names[mode] ? t(adminKey(names[mode]!)) : mode; }
function receiptText(): string { return receipt.value ? operationLabel(receipt.value.operation) : ""; }

watch(scopeStamp, resetScope, { immediate: true });
onUnmounted(() => { alive = false; policyTicket++; plansTicket++; executionsTicket++; detailTicket++; targetsTicket++; writeTicket++; conflictTicket++; });
</script>

<template>
  <article class="panel correction-admin">
    <header class="cc-heading"><div><h1>{{ t('admin.copy.f8cf27de') }}</h1></div><button v-refresh-button="t('admin.copy.56b8a362')" class="button" type="button" data-testid="cc-refresh" :disabled="busyRead || !rights.view" @click="withGlobalLoading(reloadAll)">{{ t('admin.copy.56b8a362') }}</button></header>
    <div class="cc-callout">{{ t('admin.copy.abf96c9b') }}</div>
    <p v-if="!rights.view" class="cc-muted">{{ t('admin.copy.cdaf5831') }}</p>
    <template v-else>
      <div v-if="writeError" class="cc-error" role="alert">{{ displayPublicError(writeError) }}</div><div v-if="notice" class="cc-success" role="status">{{ noticeText }}</div>
      <section class="cc-card" data-testid="cc-policy"><div><h2>{{ t('admin.copy.02ee068b') }}</h2></div>
        <label class="cc-toggle"><input data-testid="cc-policy-toggle" type="checkbox" v-model="policyEnabled" :disabled="!rights.policyWrite || props.account.super_admin || props.brandStatus === 'disabled' || !policy || busyRead || writing || Boolean(unknownIntent) || conflict" />{{ policy?.enabled ? t('admin.copy.e1cc5fdc') : t('admin.copy.f81cb486') }}</label>

        <button class="button" data-testid="cc-policy-review" type="button" :disabled="!canPolicy" @click="openReview('policy')">{{ t('admin.copy.3a0deb6f') }}</button>
        <small v-if="props.brandStatus === 'paused'" class="cc-muted">{{ t('admin.copy.8144c980') }}</small>
      </section>

      <section class="cc-section"><div class="cc-section-heading"><div><h2>{{ t('admin.copy.fac20e49') }}</h2><p>{{ t('admin.copy.a79f8665') }} · {{ planTotal }}</p></div><div class="cc-actions"><button v-refresh-button="t('admin.copy.ffd7684e')" class="button" type="button" :disabled="busyRead" @click="withGlobalLoading(loadPlans, planOffset)">{{ t('admin.copy.ffd7684e') }}</button></div></div>
        <div v-if="errors.plans" class="cc-error" role="alert">{{ displayPublicError(errors.plans) }}</div>
        <div class="cc-table-wrap"><table><thead><tr><th>{{ t('admin.copy.2931716c') }}</th><th>{{ t('admin.copy.e6dee72a') }}</th><th>{{ t('admin.copy.cbe1a7b4') }}</th><th>{{ t('admin.copy.f30f55ed') }}</th><th>{{ t('admin.copy.64cb4a2c') }}</th><th>{{ t('admin.copy.28f2247b') }}</th></tr></thead><tbody>
          <tr v-for="row in planPage?.items ?? []" :key="row.id" :class="{selected:selectedPlanId===row.id}"><td><button class="cc-link" type="button" @click="withGlobalLoading(selectPlan, row.id)">{{ row.id }}</button><small>{{ t('admin.copy.030e0beb') }} <AssociationName :record="row" kind="cycle" :id="row.cycle_id" :locale="associationLocale" /> {{ t('admin.copy.9d344757') }} {{ row.run_id }}</small></td><td><span class="cc-status">{{ planState(row.state) }}</span><small>{{ modeLabel(row.payout_mode) }}</small></td><td><small>{{ row.payment_id }}</small></td><td>{{ row.before_points }} / {{ row.calculated_points }} / {{ row.credit_points ?? "—" }} / {{ row.debit_points ?? "—" }} / {{ row.net_points ?? "—" }}</td><td>{{ row.planned_count }} / {{ row.target_count }}</td><td>{{ row.last_error_code ? displayPublicError(publicErrorMessage({ code: row.last_error_code })) : "—" }}</td></tr>
          <tr v-if="!planPage?.items.length"><td colspan="6" class="cc-muted empty-state">{{ t('admin.copy.c57345d3') }}</td></tr>
        </tbody></table></div><div class="cc-pagination"><button class="button" type="button" :disabled="planOffset===0 || busyRead" @click="withGlobalLoading(loadPlans, Math.max(0,planOffset-PAGE_SIZE))">{{ t('admin.copy.dbe7a5d7') }}</button><span>{{ planPage?.items.length ? planOffset + 1 : 0 }}–{{ planOffset + (planPage?.items.length ?? 0) }} / {{ planTotal }}</span><button class="button" type="button" :disabled="!planPage || BigInt(planOffset+PAGE_SIZE)>=BigInt(planTotal) || busyRead" @click="withGlobalLoading(loadPlans, planOffset+PAGE_SIZE)">{{ t('admin.copy.2e9c9e7a') }}</button></div>
        <DetailModal v-if="!!plan" :title="t('admin.copy.01db40da')" :close-label="t('admin.copy.2bb2cb1d')" :open="!dialog" @close="plan = null; selectedPlanId = ''">
<div v-if="plan" class="cc-card cc-detail" data-testid="cc-plan-detail"><div><h3>{{ t('admin.copy.89016b32') }} · {{ plan.id }}</h3><span class="cc-status">{{ planState(plan.state) }}</span></div><dl class="cc-facts"><dt>{{ t('admin.copy.3f938f0d') }}</dt><dd>{{ plan.payment_id }} · {{ originalPaymentRun ?? errors.originalPayment ?? t('admin.copy.27342bf3') }}</dd><dt>{{ t('admin.copy.ad5f2d15') }}</dt><dd>{{ plan.before_points }} / {{ plan.calculated_points }}</dd><dt>{{ t('admin.copy.4d58a26a') }}</dt><dd>{{ plan.credit_points ?? "—" }} / {{ plan.debit_points ?? "—" }} / {{ plan.net_points ?? "—" }}</dd><dt>{{ t('admin.copy.0e2843d8') }}</dt><dd>{{ plan.last_error_code ? displayPublicError(publicErrorMessage({ code: plan.last_error_code })) : "—" }}</dd></dl>
          <h4>{{ t('admin.copy.2ce503d6') }}</h4><div class="cc-table-wrap"><table><thead><tr><th>{{ t('admin.ui.agentMember') }}</th><th>{{ t('admin.copy.82b6cfd2') }}</th><th>{{ t('admin.copy.ee390b58') }}</th><th>{{ t('admin.copy.a69f2089') }}</th></tr></thead><tbody><tr v-for="target in planTargets" :key="target.id"><td><AssociationName :record="target" kind="agent" :id="target.agent_id" :locale="associationLocale" /><small><AssociationName :record="target" kind="member" :id="target.member_id" :locale="associationLocale" /></small></td><td>{{ target.points_before }} → {{ target.points_after }} / {{ target.delta_points }}<small v-if="target.delta_points==='0'">{{ t('admin.copy.7d577b92') }}</small></td><td>{{ target.original_target_id ?? "—" }}</td><td>{{ target.previous_correction_target_id ?? "—" }}</td></tr><tr v-if="!planTargets.length"><td colspan="4" class="cc-muted empty-state">{{ t('admin.copy.cf8e4243') }}</td></tr></tbody></table></div>
          <div class="cc-pagination"><button class="button" type="button" :disabled="planTargetsOffset===0 || loadingTargets" @click="withGlobalLoading(loadPlanTargetPage, Math.max(0,planTargetsOffset-TARGET_PAGE_SIZE))">{{ t('admin.copy.d2c000b4') }}</button><span>{{ planTargets.length ? planTargetsOffset + 1 : 0 }}–{{ planTargetsOffset + planTargets.length }} / {{ planTargetsTotal }}</span><button class="button" type="button" :disabled="!planTargets.length || BigInt(planTargetsOffset+TARGET_PAGE_SIZE)>=BigInt(planTargetsTotal) || loadingTargets" @click="withGlobalLoading(loadPlanTargetPage, planTargetsOffset+TARGET_PAGE_SIZE)">{{ t('admin.copy.2d1da8f1') }}</button></div>
          <button class="button" data-testid="cc-plan-retry" type="button" :disabled="!canPlanRetry" @click="openReview('plan_retry')">{{ t('admin.copy.18acbe2a') }}</button>
        </div>
</DetailModal>
      </section>

      <section class="cc-section"><div class="cc-section-heading"><div><h2>{{ t('admin.copy.4ef2ee2e') }}</h2><p>{{ t('admin.copy.6a6d8948') }} · {{ executionTotal }}</p></div><button v-refresh-button="t('admin.copy.78035961')" class="button" type="button" :disabled="busyRead" @click="withGlobalLoading(loadExecutions, executionOffset)">{{ t('admin.copy.78035961') }}</button></div>
        <div v-if="errors.executions" class="cc-error" role="alert">{{ displayPublicError(errors.executions) }}</div>
        <div class="cc-table-wrap"><table><thead><tr><th>{{ t('admin.copy.ecc139f1') }}</th><th>{{ t('admin.copy.cbcba430') }}</th><th>{{ t('admin.copy.f17936f2') }}</th><th>{{ t('admin.copy.4079135c') }}</th><th>{{ t('admin.copy.16f272ca') }}</th><th>{{ t('admin.copy.3a65b310') }}</th></tr></thead><tbody>
          <tr v-for="row in executionPage?.items ?? []" :key="row.id" :class="{selected:selectedExecutionId===row.id}"><td><button class="cc-link" type="button" @click="withGlobalLoading(selectExecution, row.id)">{{ row.id }}</button><small>{{ t('admin.copy.030e0beb') }} <AssociationName :record="row" kind="cycle" :id="row.cycle_id" :locale="associationLocale" /> · plan {{ row.plan_id }} · run {{ row.run_id }}</small></td><td><span class="cc-status">{{ executionState(row.state) }}</span><small>{{ modeLabel(row.payout_mode) }} </small></td><td>{{ row.credit_points }} / {{ row.debit_points }} / {{ row.net_points }}</td><td>{{ row.applied_count }} / {{ row.target_count }}</td><td>{{ row.applied_credit_points }} / {{ row.applied_debit_points }}</td><td><span v-if="row.cycle_hold_active" class="cc-hold">{{ t('admin.copy.06630857') }}</span><span v-else>—</span><small>{{ row.last_error_code ? displayPublicError(publicErrorMessage({ code: row.last_error_code })) : "—" }}</small></td></tr><tr v-if="!executionPage?.items.length"><td colspan="6" class="cc-muted empty-state">{{ t('admin.copy.c2e38121') }}</td></tr>
        </tbody></table></div><div class="cc-pagination"><button class="button" type="button" :disabled="executionOffset===0 || busyRead" @click="withGlobalLoading(loadExecutions, Math.max(0,executionOffset-PAGE_SIZE))">{{ t('admin.copy.dbe7a5d7') }}</button><span>{{ executionPage?.items.length ? executionOffset + 1 : 0 }}–{{ executionOffset + (executionPage?.items.length ?? 0) }} / {{ executionTotal }}</span><button class="button" type="button" :disabled="!executionPage || BigInt(executionOffset+PAGE_SIZE)>=BigInt(executionTotal) || busyRead" @click="withGlobalLoading(loadExecutions, executionOffset+PAGE_SIZE)">{{ t('admin.copy.2e9c9e7a') }}</button></div>
        <DetailModal v-if="!!execution" :title="t('admin.copy.01db40da')" :close-label="t('admin.copy.2bb2cb1d')" :open="!dialog" @close="execution = null; selectedExecutionId = ''">
<div v-if="execution" class="cc-card cc-detail" data-testid="cc-execution-detail"><div><h3>{{ t('admin.copy.a94b1595') }} · {{ execution.id }}</h3><span class="cc-status">{{ executionState(execution.state) }}</span></div><dl class="cc-facts"><dt>{{ t('admin.copy.02b926ce') }}</dt><dd>{{ execution.credit_points }} / {{ execution.debit_points }} / {{ execution.net_points }}</dd><dt>{{ t('admin.copy.7e784a27') }}</dt><dd>{{ execution.target_count }} / {{ execution.applied_count }}</dd><dt>{{ t('admin.copy.29e743e2') }}</dt><dd>{{ execution.applied_credit_points }} / {{ execution.applied_debit_points }}</dd><dt>{{ t('admin.copy.a7e6daf6') }}</dt><dd>{{ execution.cycle_hold_active ? t('admin.copy.da09988d') : t('admin.copy.bae5218f') }}<span v-if="execution.paused_plan_target_id"> {{ execution.paused_plan_target_id }}</span></dd></dl>
          <h4>{{ t('admin.copy.0c186621') }}</h4><div class="cc-table-wrap"><table><thead><tr><th>{{ t('admin.ui.agentMember') }}</th><th>{{ t('admin.copy.82b6cfd2') }}</th><th>{{ t('admin.copy.86d53071') }}</th><th>{{ t('admin.copy.3cee6980') }}</th></tr></thead><tbody><tr v-for="target in executionTargets" :key="target.id"><td><AssociationName :record="target" kind="agent" :id="target.agent_id" :locale="associationLocale" /><small><AssociationName :record="target" kind="member" :id="target.member_id" :locale="associationLocale" /></small></td><td>{{ target.points_before }} → {{ target.points_after }} / {{ target.delta_points }}</td><td>{{ target.state === 'applied' ? t('admin.commission.targetApplied') : t('admin.commission.targetPending') }}<small v-if="target.delta_points==='0'">{{ t('admin.copy.a1cb3520') }}</small></td><td>{{ target.ledger_entry_id ?? "—" }}</td></tr><tr v-if="!executionTargets.length"><td colspan="4" class="cc-muted empty-state">{{ t('admin.copy.2b4a1bed') }}</td></tr></tbody></table></div>
          <div class="cc-pagination"><button class="button" type="button" :disabled="executionTargetsOffset===0 || loadingTargets" @click="withGlobalLoading(loadExecutionTargetPage, Math.max(0,executionTargetsOffset-TARGET_PAGE_SIZE))">{{ t('admin.copy.d2c000b4') }}</button><span>{{ executionTargets.length ? executionTargetsOffset + 1 : 0 }}–{{ executionTargetsOffset + executionTargets.length }} / {{ executionTargetsTotal }}</span><button class="button" type="button" :disabled="!executionTargets.length || BigInt(executionTargetsOffset+TARGET_PAGE_SIZE)>=BigInt(executionTargetsTotal) || loadingTargets" @click="withGlobalLoading(loadExecutionTargetPage, executionTargetsOffset+TARGET_PAGE_SIZE)">{{ t('admin.copy.2d1da8f1') }}</button></div>
          <div class="cc-actions"><button class="button" data-testid="cc-approve" type="button" :disabled="!canApprove" @click="openReview('approve')">{{ t('admin.copy.028518f0') }}</button><button class="button" data-testid="cc-continue" type="button" :disabled="!canContinue" @click="openReview('continue')">{{ t('admin.copy.cdb10f42') }}</button><button class="button" data-testid="cc-execute-retry" type="button" :disabled="!canRetry" @click="openReview('execute_retry')">{{ t('admin.copy.50dae2bb') }}</button></div>
        </div>
</DetailModal>
      </section>
      <div v-if="unknownIntent" class="cc-error" data-testid="cc-unknown"><span>{{ t('admin.copy.48cb0926') }} · {{ operationLabel(unknownIntent.operation) }} · </span><button v-write-guard="writing" class="button" type="button" :disabled="writing || !canResolveUnknown" @click="retryUnknown">{{ t('admin.copy.3ef3c644') }}</button></div>
      <div v-if="conflict" class="cc-review-card" data-testid="cc-conflict"><p>{{ t('admin.copy.af8f1d76') }}</p><button v-refresh-button="t('admin.copy.9fd90523')" class="button" type="button" :disabled="conflictReloading" @click="withGlobalLoading(refreshConflict)">{{ t('admin.copy.9fd90523') }}</button><label class="cc-confirm"><input type="checkbox" :disabled="!conflictRefreshed || conflictReloading" :checked="discardReviewed" @change="discardReviewed=($event.target as HTMLInputElement).checked" />{{ t('admin.copy.8d9b3d75') }}</label><button class="button" type="button" :disabled="!conflictRefreshed || !discardReviewed || conflictReloading" @click="discardConflict">{{ t('admin.copy.5079f563') }}</button></div>
      <section v-if="receipt" class="cc-receipt" data-testid="cc-receipt"><strong>{{ t('admin.copy.ff2e9c8f') }}</strong><code>{{ receiptText() }}</code><p>{{ t('admin.copy.6fa8b32f') }}</p><div v-if="Object.keys(errors).length" role="alert" class="cc-error">{{ Object.values(errors).map(displayPublicError).join(" · ") }}</div></section>
      <section v-if="dialog" class="cc-review-card" data-testid="cc-review"><h2>{{ t('admin.copy.1803550d') }} · {{ operationLabel(dialog.operation) }}</h2><dl class="cc-facts"><dt>{{ t('admin.copy.e5cf0427') }}</dt><dd><AssociationName kind="brand" :id="dialog.brandId" :locale="associationLocale" /> {{ operationLabel(dialog.operation) }} / {{ dialog.targetId ?? "—" }}</dd><dt v-if="dialog.body.reason">{{ t('admin.copy.5ec7c8ba') }}</dt><dd v-if="dialog.body.reason">{{ dialog.body.reason }}</dd></dl><label class="cc-confirm"><input data-testid="cc-confirm-check" type="checkbox" :checked="confirmed" :disabled="busyRead" @change="confirmed=($event.target as HTMLInputElement).checked" />{{ t('admin.copy.41a04f7e') }}</label><div class="cc-actions"><button v-write-guard="writing" class="button button-primary" data-testid="cc-submit" type="button" :disabled="writing || busyRead || !confirmed" @click="withGlobalLoading(submit, dialog)">{{ writing ? t('admin.copy.46511dc0') : unknownIntent ? t('admin.copy.59c708d8') : t('admin.copy.3135aeca') }}</button><button class="button" type="button" :disabled="writing" @click="dialog=null;confirmed=false">{{ t('admin.copy.e2f391e7') }}</button></div></section>
    </template>
  </article>
</template>

<style scoped>
/* Global admin table metadata uses nowrap. Reset it for stacked mobile rows:
   overflow widens the mobile visual viewport and offsets physical hit targets. */
.cc-table-wrap td small{white-space:normal;min-width:0}.cc-detail h3,.cc-review-card h2{overflow-wrap:anywhere}
.correction-admin{display:grid;gap:18px;min-width:0}.cc-heading,.cc-section-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:16px}.cc-heading h1,.cc-section h2,.cc-card h2,.cc-detail h3{margin:0}.cc-heading p,.cc-section-heading p,.cc-card p{margin:6px 0 0;color:var(--muted);font-size:13px;line-height:1.55}.cc-kicker{font-size:11px!important;letter-spacing:.14em;color:var(--accent)!important;font-weight:800}.cc-callout,.cc-error,.cc-success,.cc-muted,.cc-receipt,.cc-review-card{padding:12px 14px;border-radius:10px;line-height:1.55}.cc-callout{border:1px solid var(--warning);background:color-mix(in srgb,var(--warning) 8%,transparent)}.cc-error{color:var(--danger);background:color-mix(in srgb,var(--danger) 8%,transparent);overflow-wrap:anywhere}.cc-success{color:var(--success);background:color-mix(in srgb,var(--success) 8%,transparent)}.cc-muted{color:var(--muted)}.cc-card,.cc-review-card{display:grid;gap:14px;padding:18px;border:1px solid var(--border,var(--line));border-radius:14px;background:var(--surface);min-width:0}.cc-section{display:grid;gap:12px;min-width:0}.cc-toggle,.cc-confirm{display:flex;align-items:center;gap:10px;font-weight:700}.cc-toggle input,.cc-confirm input{width:18px;height:18px;accent-color:var(--accent);flex:none}.cc-reason{display:grid;gap:6px;font-weight:650}.cc-reason textarea{resize:vertical;min-height:60px;padding:10px;border:1px solid var(--border,#cbd5e1);border-radius:8px;font:inherit}.cc-actions,.cc-pagination{display:flex;align-items:center;gap:8px;flex-wrap:wrap}.cc-pagination{justify-content:space-between}.cc-table-wrap{width:100%;overflow:auto;border:1px solid var(--border,var(--line));border-radius:12px}.cc-table-wrap table{width:100%;border-collapse:collapse;min-width:850px}.cc-table-wrap th,.cc-table-wrap td{text-align:left;padding:10px 12px;border-bottom:1px solid var(--border,var(--line));vertical-align:top}.cc-table-wrap th{font-size:11px;color:var(--muted);background:var(--surface-muted,var(--canvas))}.cc-table-wrap tr:last-child td{border-bottom:0}.cc-table-wrap tr.selected{background:color-mix(in srgb,var(--accent) 7%,transparent)}.cc-table-wrap td small,.cc-facts dd small,.cc-review-card dd small{display:block;margin-top:4px;color:var(--muted);overflow-wrap:anywhere}.cc-link{padding:0;border:0;background:transparent;color:var(--accent);font:inherit;text-align:left;overflow-wrap:anywhere;cursor:pointer}.cc-status,.cc-hold{display:inline-flex;padding:4px 8px;border-radius:999px;background:#eef2f7;font-size:12px;font-weight:700}.cc-hold{color:#9a3412;background:#fff7ed}.cc-facts{display:grid;grid-template-columns:minmax(170px,.35fr) minmax(0,1fr);gap:8px 14px;margin:0}.cc-facts dt{color:var(--muted)}.cc-facts dd{margin:0;overflow-wrap:anywhere}.cc-detail h4{margin:4px 0}.cc-receipt{display:grid;gap:8px;border:1px solid var(--success);background:color-mix(in srgb,var(--success) 7%,transparent);min-width:0}.cc-receipt code,.cc-review-card code{overflow-wrap:anywhere;white-space:pre-wrap}.cc-review-card{max-width:780px;width:100%;margin-inline:auto;box-shadow:0 20px 60px #0f172a30}.cc-review-card h2,.cc-review-card p{margin:0}.cc-review-card p{color:var(--muted)}
@media(max-width:700px){.cc-heading,.cc-section-heading{align-items:stretch;flex-direction:column}.cc-heading>.button,.cc-section-heading>.button{align-self:flex-start}.cc-card,.cc-review-card{padding:14px}.cc-facts{grid-template-columns:1fr;gap:4px}.cc-facts dd{margin-bottom:8px}.cc-actions>.button{flex:1 1 100%}.cc-pagination{align-items:flex-start;flex-direction:column}.cc-table-wrap{overflow:visible;border:0}.cc-table-wrap table,.cc-table-wrap tbody,.cc-table-wrap tr,.cc-table-wrap td{display:block;width:100%;min-width:0}.cc-table-wrap thead{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0,0,0,0)}.cc-table-wrap tbody{display:grid;gap:10px}.cc-table-wrap tbody tr{border:1px solid var(--border,var(--line));border-radius:10px;padding:8px;background:var(--surface)}.cc-table-wrap td{border:0;padding:6px 4px;white-space:normal;overflow-wrap:anywhere}.cc-table-wrap td+td{border-top:1px dashed var(--border,var(--line))}.cc-error{display:grid;gap:10px}}
</style>
