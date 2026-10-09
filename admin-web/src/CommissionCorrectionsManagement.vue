<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { useAdminI18n } from "./i18n";
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
const errors = ref<Record<string, string>>({}), writeError = ref("");
const notice = ref<"confirmed" | "discarded" | "">("");
const noticeText = computed(() => notice.value === "confirmed"
  ? t("服务器已确认操作回执。回执已保留；当前状态查询单独刷新。", "The server confirmed the operation receipt. It is retained separately while current state is refreshed.")
  : notice.value === "discarded"
    ? t("已人工确认丢弃旧请求。请按当前版本重新审核，新的提交会生成新幂等键。", "The old request was explicitly discarded. Review the current version; a new submission will use a new idempotency key.") : "");
const reason = ref(""), policyEnabled = ref(false), confirmed = ref(false), dialog = ref<PendingCommissionCorrectionIntent | null>(null);
const unknownIntent = ref<PendingCommissionCorrectionIntent | null>(null), conflictIntent = ref<PendingCommissionCorrectionIntent | null>(null);
const conflict = ref(false), conflictReloading = ref(false), conflictRefreshed = ref(false), discardReviewed = ref(false);
const planOffset = ref(0), executionOffset = ref(0);
let alive = true, policyTicket = 0, plansTicket = 0, executionsTicket = 0, detailTicket = 0, targetsTicket = 0, writeTicket = 0, conflictTicket = 0;

const busyRead = computed(() => loading.value || loadingPolicy.value || loadingPlans.value || loadingExecutions.value || loadingTargets.value);
const planTotal = computed(() => planPage.value?.total_count ?? "0"), executionTotal = computed(() => executionPage.value?.total_count ?? "0");
const canWrite = computed(() => rights.value.view && props.brandStatus !== "disabled" && !props.account.super_admin && !writing.value && !unknownIntent.value && !conflict.value);
const canPolicy = computed(() => canWrite.value && !busyRead.value && !loadingTargets.value && !errors.value.policy && rights.value.policyWrite && policy.value && policy.value.version < Number.MAX_SAFE_INTEGER && validReason(reason.value));
const historicalOpen117Retry = computed(() => plan.value?.state === "blocked" && plan.value.last_error_code === "COMMISSION_CORRECTION_MANUAL_POLICY_UNRESOLVED" && plan.value.planned_count === "0" && plan.value.credit_points === null && plan.value.debit_points === null && plan.value.net_points === null);
const canPlanRetry = computed(() => canWrite.value && !busyRead.value && !loadingTargets.value && !errors.value.plan && !errors.value.plans && rights.value.planRetry && (plan.value?.state === "failed" || historicalOpen117Retry.value) && plan.value!.version < Number.MAX_SAFE_INTEGER && validReason(reason.value));
const canApprove = computed(() => canWrite.value && !busyRead.value && !loadingTargets.value && !errors.value.execution && !errors.value.executions && rights.value.approve && execution.value?.state === "awaiting_approval" && ["manual", "mixed"].includes(execution.value.payout_mode) && execution.value.version < Number.MAX_SAFE_INTEGER && validReason(reason.value));
const canContinue = computed(() => canWrite.value && !busyRead.value && !loadingTargets.value && !errors.value.execution && !errors.value.executions && rights.value.continue && execution.value?.state === "paused" && Boolean(execution.value.approval_actor_type) && Boolean(execution.value.approval_audit_log_id) && execution.value.cycle_hold_active === true && execution.value.version < Number.MAX_SAFE_INTEGER && validReason(reason.value));
const canRetry = computed(() => canWrite.value && !busyRead.value && !loadingTargets.value && !errors.value.execution && !errors.value.executions && rights.value.retry && execution.value?.state === "failed" && execution.value.version < Number.MAX_SAFE_INTEGER && validReason(reason.value));
const canResolveUnknown = computed(() => Boolean(unknownIntent.value && rights.value.view && props.brandStatus !== "disabled" &&
  (unknownIntent.value.operation === "policy" ? rights.value.policyWrite : unknownIntent.value.operation === "plan_retry" ? rights.value.planRetry : unknownIntent.value.operation === "approve" ? rights.value.approve : unknownIntent.value.operation === "continue" ? rights.value.continue : rights.value.retry)));

function validReason(value: string): boolean {
  if (!value || value.trim() !== value || new TextEncoder().encode(value).length > 500 || /[\u0000-\u001f\u007f-\u009f]/u.test(value)) return false;
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
  if (unknownIntent.value) writeError.value = t("存在同会话未确认请求。所有新资金操作已锁定；仅可人工复核并使用原请求重试。", "An unresolved request exists in this session. All new financial writes are locked; only manual review and replay of the original request are allowed.");
  if (rights.value.view) void reloadAll();
}
function sessionExpired(cause: unknown, captured: string, session: number): boolean {
  if (cause instanceof AdminApiError && cause.status === 401 && alive && captured === scopeStamp.value && session === commissionCorrectionSessionGeneration()) {
    policyTicket++; plansTicket++; executionsTicket++; detailTicket++; targetsTicket++; writeTicket++; clearAllCommissionCorrectionIntents(); clearVisible(); emit("session-invalid"); return true;
  }
  return false;
}
function message(cause: unknown): string { return cause instanceof AdminApiError && cause.status === 0 ? t("网络或响应不可用，当前数据未能读取。", "The network or response is unavailable; current data could not be loaded.") : cause instanceof Error ? cause.message : t("读取失败。", "Unable to load data."); }
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
      writeError.value = t("服务器返回 409。旧请求已保留且不可重放；刷新策略、计划、执行和关联目标，核对后明确丢弃旧请求。", "The server returned 409. The old request is retained and cannot be replayed; refresh policy, plans, executions and related targets, then explicitly discard it after review.");
      return;
    }
    if (!currentWrite(ticket, captured, session)) { if (outcome !== "unknown") clearPendingCommissionCorrectionIntent(scopeFor(intent.operation, intent.targetId), intent.key); return; }
    if (sessionExpired(cause, captured, session)) { clearPendingCommissionCorrectionIntent(scopeFor(intent.operation, intent.targetId), intent.key); return; }
    if (outcome === "unknown") {
      setPendingCommissionCorrectionIntent(scopeFor(intent.operation, intent.targetId), intent); unknownIntent.value = intent; dialog.value = null;
      writeError.value = t("提交结果未知。原请求正文、actor、品牌和幂等键均已冻结；只能人工复核后重放同一请求，不会自动重试。", "The outcome is unknown. The original body, actor, brand and idempotency key are frozen; replay requires manual review and is never automatic.");
    } else {
      clearPendingCommissionCorrectionIntent(scopeFor(intent.operation, intent.targetId), intent.key); dialog.value = null;
      writeError.value = cause instanceof Error ? cause.message : t("提交失败。", "Submission failed.");
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
    writeError.value = t("相关当前数据均已成功刷新。请核对后勾选确认，才可丢弃 409 原请求。", "All related current data refreshed successfully. Review it and check the confirmation before discarding the original 409 request.");
  }
  if (ticket === conflictTicket) conflictReloading.value = false;
}
function discardConflict() {
  const intent = conflictIntent.value; if (!intent || !conflictRefreshed.value || !discardReviewed.value || conflictReloading.value) return;
  clearPendingCommissionCorrectionIntent(scopeFor(intent.operation, intent.targetId), intent.key); conflictIntent.value = null; conflict.value = false;
  conflictRefreshed.value = false; discardReviewed.value = false; writeError.value = "";
  notice.value = "discarded";
}
function formatDate(value: string): string {
  const date = new Date(value); return Number.isFinite(date.getTime()) ? `${new Intl.DateTimeFormat(locale.value === "en" ? "en" : "zh-CN", { dateStyle: "medium", timeStyle: "medium", timeZone: "UTC" }).format(date)} UTC` : value;
}
function planState(state: string): string { const names: Record<string, [string,string]> = { planning:["准备中","Planning"],ready:["计划就绪","Plan ready"],blocked:["阻止","Blocked"],failed:["失败","Failed"],stale:["已过期","Stale"] }; return t(...(names[state] ?? [state,state])); }
function executionState(state: string): string { const names: Record<string, [string,string]> = { awaiting_approval:["待新批准","Awaiting approval"],applying:["执行中","Applying"],completed:["已完成","Completed"],paused:["暂停","Paused"],failed:["失败","Failed"],stale:["已过期","Stale"] }; return t(...(names[state] ?? [state,state])); }
function modeLabel(mode: string): string { const names: Record<string,[string,string]> = { manual:["人工","Manual"],automatic:["自动","Automatic"],mixed:["混合","Mixed"],none:["无","None"] }; return t(...(names[mode] ?? [mode,mode])); }
function receiptText(): string { return receipt.value ? `${receipt.value.operation} · ${JSON.stringify(receipt.value.value)}` : ""; }

watch(scopeStamp, resetScope, { immediate: true });
onUnmounted(() => { alive = false; policyTicket++; plansTicket++; executionsTicket++; detailTicket++; targetsTicket++; writeTicket++; conflictTicket++; });
</script>

<template>
  <article class="panel correction-admin">
    <header class="cc-heading"><div><p class="cc-kicker">COMMISSION / CORRECTION CONTROL</p><h1>{{ t("佣金更正差额", "Commission correction") }}</h1><p>{{ t("计划准备与真实资金执行分离。计划就绪只证明差额已核对，不代表批准或入账。", "Plan preparation is separate from real financial execution. Plan ready confirms a reviewed difference, not approval or posting.") }}</p></div><button class="button" type="button" data-testid="cc-refresh" :disabled="busyRead || !rights.view" @click="reloadAll()">{{ t("刷新全部", "Refresh all") }}</button></header>
    <div class="cc-callout">{{ t("更正资金开关默认关闭，原支付和人工修正历史保留。OPEN-117 已确认：新核算覆盖旧人工修正目标：实际12、新核算8，追回4，最终8。旧政策阻塞计划不会自动恢复，需有权限人员填写原因并显式重试；准备完成不代表已批准或入账。", "The correction gate is off by default. Under OPEN-117, retain the adjustment and audit history and original payment. Recalculation supersedes the prior manual target: from actual 12 to recalculated 8, claw back 4 and finish at 8. Historical policy blocks require an authorized, reasoned, explicit retry; there is no automatic recovery. Plan ready is not approval or posting.") }}</div>
    <p v-if="!rights.view" class="cc-muted">{{ t("当前账号没有此品牌的佣金查看权限。", "This account cannot view commissions for this brand.") }}</p>
    <template v-else>
      <div v-if="writeError" class="cc-error" role="alert">{{ writeError }}</div><div v-if="notice" class="cc-success" role="status">{{ noticeText }}</div>
      <section class="cc-card" data-testid="cc-policy"><div><h2>{{ t("资金更正开关", "Correction execution gate") }}</h2><p>{{ policy ? `${t("政策版本", "Policy version")} ${policy.version}` : (errors.policy || t("读取中…", "Loading…")) }}</p></div>
        <label class="cc-toggle"><input data-testid="cc-policy-toggle" type="checkbox" v-model="policyEnabled" :disabled="!rights.policyWrite || props.account.super_admin || props.brandStatus === 'disabled' || !policy || busyRead || writing || Boolean(unknownIntent) || conflict" />{{ policy?.enabled ? t("真实资金执行已开启", "Live correction execution is enabled") : t("真实资金执行已关闭", "Live correction execution is disabled") }}</label>
        <label class="cc-reason">{{ t("操作原因", "Reason") }}<textarea data-testid="cc-reason" v-model="reason" maxlength="500" /></label>
        <button class="button" data-testid="cc-policy-review" type="button" :disabled="!canPolicy" @click="openReview('policy')">{{ t("审核开关更改", "Review gate change") }}</button>
        <small v-if="props.brandStatus === 'paused'" class="cc-muted">{{ t("品牌已暂停：允许处理现存财务计划；执行开关和状态限制仍分别生效。", "Brand is paused: existing financial workflows may be handled, subject to the gate and each action's state rules.") }}</small>
      </section>

      <section class="cc-section"><div class="cc-section-heading"><div><h2>{{ t("更正计划", "Correction plans") }}</h2><p>{{ t("列表和明细是冻结的前后差额证据。", "Lists and details show frozen before and after difference evidence.") }} · {{ planTotal }}</p></div><div class="cc-actions"><button class="button" type="button" :disabled="busyRead" @click="loadPlans(planOffset)">{{ t("刷新计划", "Refresh plans") }}</button></div></div>
        <div v-if="errors.plans" class="cc-error" role="alert">{{ errors.plans }}</div>
        <div class="cc-table-wrap"><table><thead><tr><th>{{ t("计划 / 周期", "Plan / cycle") }}</th><th>{{ t("状态 / 模式", "State / mode") }}</th><th>{{ t("版本 / 原支付", "Version / original payment") }}</th><th>{{ t("冻结金额：之前 / 新核算 / 入账 / 追回 / 净额", "Frozen points: before / calculated / credit / debit / net") }}</th><th>{{ t("目标", "Targets") }}</th><th>{{ t("错误", "Error") }}</th></tr></thead><tbody>
          <tr v-for="row in planPage?.items ?? []" :key="row.id" :class="{selected:selectedPlanId===row.id}"><td><button class="cc-link" type="button" @click="selectPlan(row.id)">{{ row.id }}</button><small>{{ t("cycle", "cycle") }} {{ row.cycle_id }} · {{ t("run", "run") }} {{ row.run_id }}</small></td><td><span class="cc-status">{{ planState(row.state) }}</span><small>{{ modeLabel(row.payout_mode) }}</small></td><td>v{{ row.version }}<small>{{ row.payment_id }}</small></td><td>{{ row.before_points }} / {{ row.calculated_points }} / {{ row.credit_points ?? "—" }} / {{ row.debit_points ?? "—" }} / {{ row.net_points ?? "—" }}</td><td>{{ row.planned_count }} / {{ row.target_count }}</td><td>{{ row.last_error_code ?? "—" }}</td></tr>
          <tr v-if="!planPage?.items.length"><td colspan="6" class="cc-muted">{{ t("暂无计划", "No plans") }}</td></tr>
        </tbody></table></div><div class="cc-pagination"><button class="button" type="button" :disabled="planOffset===0 || busyRead" @click="loadPlans(Math.max(0,planOffset-PAGE_SIZE))">{{ t("上一页", "Previous") }}</button><span>{{ planOffset + 1 }}–{{ planOffset + (planPage?.items.length ?? 0) }} / {{ planTotal }}</span><button class="button" type="button" :disabled="!planPage || BigInt(planOffset+PAGE_SIZE)>=BigInt(planTotal) || busyRead" @click="loadPlans(planOffset+PAGE_SIZE)">{{ t("下一页", "Next") }}</button></div>
        <div v-if="plan" class="cc-card cc-detail" data-testid="cc-plan-detail"><div><h3>{{ t("计划明细", "Plan detail") }} · {{ plan.id }}</h3><span class="cc-status">{{ planState(plan.state) }}</span></div><dl class="cc-facts"><dt>{{ t("原支付 ID / 原支付 run", "Original payment ID / original payment run") }}</dt><dd>{{ plan.payment_id }} · {{ originalPaymentRun ?? errors.originalPayment ?? t("读取中…", "Loading…") }}</dd><dt>{{ t("新更正 run / 证据代次", "Corrected run / evidence epoch") }}</dt><dd>{{ plan.run_id }} · {{ plan.evidence_epoch }}</dd><dt>{{ t("冻结之前 / 新核算", "Frozen before / calculated") }}</dt><dd>{{ plan.before_points }} / {{ plan.calculated_points }}</dd><dt>{{ t("冻结入账 / 追回 / 净额", "Frozen credit / debit / net") }}</dt><dd>{{ plan.credit_points ?? "—" }} / {{ plan.debit_points ?? "—" }} / {{ plan.net_points ?? "—" }}</dd><dt>{{ t("审计来源 / 最近审计", "Creation / latest audit") }}</dt><dd>{{ plan.creation_audit_log_id }} / {{ plan.last_audit_log_id }}</dd><dt>{{ t("OPEN-117 / 错误原因", "OPEN-117 / error reason") }}</dt><dd>{{ plan.last_error_code ?? "—" }} <span v-if="plan.last_error_code==='COMMISSION_CORRECTION_MANUAL_POLICY_UNRESOLVED'">{{ t("这是保留的旧阻止记录，不会自动改动。符合 state=blocked、此错误码、planned_count=0 且入账/追回/净额均为空时，可在填写原因并人工确认后显式重试计划准备。", "This is a retained historical blocked row and will not change automatically. If state=blocked, this error code, planned_count=0 and all credit/debit/net amounts are null, an authorized user can enter a reason and explicitly confirm a plan preparation retry.") }}</span></dd></dl>
          <h4>{{ t("计划目标（每页最多 100）", "Plan targets (up to 100 per page)") }}</h4><div class="cc-table-wrap"><table><thead><tr><th>Agent / member</th><th>{{ t("之前 → 之后 / 差额", "Before → after / delta") }}</th><th>{{ t("原支付目标", "Original payment target") }}</th><th>{{ t("前次更正目标 / 财务版本", "Previous correction target / financial version") }}</th><th>{{ t("旧人工修正版本", "Manual adjustment version") }}</th></tr></thead><tbody><tr v-for="target in planTargets" :key="target.id"><td>{{ target.agent_id }}<small>{{ target.member_id }}</small></td><td>{{ target.points_before }} → {{ target.points_after }} / {{ target.delta_points }}<small v-if="target.delta_points==='0'">{{ t("零差额见证，不会产生零积分流水。", "Zero-difference witness; no zero-point ledger entry is created.") }}</small></td><td>{{ target.original_target_id ?? "—" }}</td><td>{{ target.previous_correction_target_id ?? "—" }} / {{ target.financial_version ?? "—" }}</td><td>{{ target.adjustment_version ?? "—" }}</td></tr><tr v-if="!planTargets.length"><td colspan="5" class="cc-muted">{{ t("暂无目标；检查准备状态或历史阻塞原因。", "No targets; inspect preparation status or historical block.") }}</td></tr></tbody></table></div>
          <div class="cc-pagination"><button class="button" type="button" :disabled="planTargetsOffset===0 || loadingTargets" @click="loadPlanTargetPage(Math.max(0,planTargetsOffset-TARGET_PAGE_SIZE))">{{ t("上一页目标", "Previous targets") }}</button><span>{{ planTargetsOffset + 1 }}–{{ planTargetsOffset + planTargets.length }} / {{ planTargetsTotal }}</span><button class="button" type="button" :disabled="!planTargets.length || BigInt(planTargetsOffset+TARGET_PAGE_SIZE)>=BigInt(planTargetsTotal) || loadingTargets" @click="loadPlanTargetPage(planTargetsOffset+TARGET_PAGE_SIZE)">{{ t("下一页目标", "Next targets") }}</button></div>
          <label class="cc-reason">{{ t("审批/重试原因", "Approval/retry reason") }}<textarea data-testid="cc-plan-reason" v-model="reason" maxlength="500" /></label><button class="button" data-testid="cc-plan-retry" type="button" :disabled="!canPlanRetry" @click="openReview('plan_retry')">{{ historicalOpen117Retry ? t("人工确认并重试旧 OPEN-117 计划", "Review and retry historical OPEN-117 plan") : t("明确重试计划准备", "Explicitly retry plan preparation") }}</button>
        </div>
      </section>

      <section class="cc-section"><div class="cc-section-heading"><div><h2>{{ t("真实资金执行", "Financial executions") }}</h2><p>{{ t("显示冻结计划金额与真实已入账金额；审批来源只属于当前更正。", "Frozen plan totals are shown beside actual applied totals; approval provenance belongs to this correction only.") }} · {{ executionTotal }}</p></div><button class="button" type="button" :disabled="busyRead" @click="loadExecutions(executionOffset)">{{ t("刷新执行", "Refresh executions") }}</button></div>
        <div v-if="errors.executions" class="cc-error" role="alert">{{ errors.executions }}</div>
        <div class="cc-table-wrap"><table><thead><tr><th>{{ t("执行 / 周期", "Execution / cycle") }}</th><th>{{ t("状态 / 模式 / 版本", "State / mode / version") }}</th><th>{{ t("冻结计划：入账 / 追回 / 净额", "Frozen plan: credit / debit / net") }}</th><th>{{ t("目标数 / 实际已应用数", "Targets / applied") }}</th><th>{{ t("实际入账：补发 / 追回", "Actual applied: credit / debit") }}</th><th>{{ t("暂停 / 错误", "Hold / error") }}</th></tr></thead><tbody>
          <tr v-for="row in executionPage?.items ?? []" :key="row.id" :class="{selected:selectedExecutionId===row.id}"><td><button class="cc-link" type="button" @click="selectExecution(row.id)">{{ row.id }}</button><small>{{ t("cycle", "cycle") }} {{ row.cycle_id }} · plan {{ row.plan_id }} · run {{ row.run_id }}</small></td><td><span class="cc-status">{{ executionState(row.state) }}</span><small>{{ modeLabel(row.payout_mode) }} · v{{ row.version }} / plan v{{ row.plan_version }}</small></td><td>{{ row.credit_points }} / {{ row.debit_points }} / {{ row.net_points }}</td><td>{{ row.applied_count }} / {{ row.target_count }}</td><td>{{ row.applied_credit_points }} / {{ row.applied_debit_points }}</td><td><span v-if="row.cycle_hold_active" class="cc-hold">{{ t("当前 cycle_hold_active：跨核算代次暂停", "cycle_hold_active now: hold persists across run generations") }}</span><span v-else>—</span><small>{{ row.last_error_code ?? "—" }}</small></td></tr><tr v-if="!executionPage?.items.length"><td colspan="6" class="cc-muted">{{ t("暂无执行记录", "No executions") }}</td></tr>
        </tbody></table></div><div class="cc-pagination"><button class="button" type="button" :disabled="executionOffset===0 || busyRead" @click="loadExecutions(Math.max(0,executionOffset-PAGE_SIZE))">{{ t("上一页", "Previous") }}</button><span>{{ executionOffset + 1 }}–{{ executionOffset + (executionPage?.items.length ?? 0) }} / {{ executionTotal }}</span><button class="button" type="button" :disabled="!executionPage || BigInt(executionOffset+PAGE_SIZE)>=BigInt(executionTotal) || busyRead" @click="loadExecutions(executionOffset+PAGE_SIZE)">{{ t("下一页", "Next") }}</button></div>
        <div v-if="execution" class="cc-card cc-detail" data-testid="cc-execution-detail"><div><h3>{{ t("执行明细", "Execution detail") }} · {{ execution.id }}</h3><span class="cc-status">{{ executionState(execution.state) }}</span></div><dl class="cc-facts"><dt>{{ t("计划 / run / 计划版本 / 证据代次", "Plan / run / plan version / evidence epoch") }}</dt><dd>{{ execution.plan_id }} / {{ execution.run_id }} / {{ execution.plan_version }} / {{ execution.evidence_epoch }}</dd><dt>{{ t("冻结计划总额：补发 / 追回 / 净额", "Frozen plan totals: credit / debit / net") }}</dt><dd>{{ execution.credit_points }} / {{ execution.debit_points }} / {{ execution.net_points }}</dd><dt>{{ t("目标数 / 实际已应用数", "Targets / actually applied") }}</dt><dd>{{ execution.target_count }} / {{ execution.applied_count }}</dd><dt>{{ t("真实已应用：补发 / 追回", "Actually applied: credit / debit") }}</dt><dd>{{ execution.applied_credit_points }} / {{ execution.applied_debit_points }}</dd><dt>{{ t("本次批准来源", "Approval provenance") }}</dt><dd>{{ execution.approved_by ?? "—" }} · {{ execution.approval_actor_type ?? "—" }} · {{ execution.approval_audit_log_id ?? "—" }}</dd><dt>{{ t("当前周期暂停", "Current cycle hold") }}</dt><dd>{{ execution.cycle_hold_active ? t("当前有效且跨run代次继承；这不是任务创建时的历史快照。", "Currently active and inherited across run generations; this is not a historical snapshot from task creation.") : t("当前无暂停", "No active hold now") }}<span v-if="execution.paused_plan_target_id"> · {{ execution.paused_plan_target_id }}</span></dd><dt>{{ t("创建 / 最近审计 / 错误", "Creation / latest audit / error") }}</dt><dd>{{ execution.creation_audit_log_id }} / {{ execution.last_audit_log_id }} / {{ execution.last_error_code ?? "—" }}</dd></dl>
          <h4>{{ t("执行目标", "Execution targets") }}</h4><div class="cc-table-wrap"><table><thead><tr><th>Agent / member</th><th>{{ t("之前 → 之后 / 差额", "Before → after / delta") }}</th><th>{{ t("应用状态 / 财务版本", "Apply state / financial version") }}</th><th>{{ t("旧账本 / 目标审计", "Ledger / target audit") }}</th></tr></thead><tbody><tr v-for="target in executionTargets" :key="target.id"><td>{{ target.agent_id }}<small>{{ target.member_id }}</small></td><td>{{ target.points_before }} → {{ target.points_after }} / {{ target.delta_points }}</td><td>{{ target.state }} / {{ target.financial_version ?? "—" }}<small v-if="target.delta_points==='0'">{{ t("零差额已见证，无零额流水。", "Zero difference witnessed; no zero-value ledger entry.") }}</small></td><td>{{ target.ledger_entry_id ?? "—" }}<small>{{ target.audit_log_id ?? "—" }}</small></td></tr><tr v-if="!executionTargets.length"><td colspan="4" class="cc-muted">{{ t("暂无目标", "No targets") }}</td></tr></tbody></table></div>
          <div class="cc-pagination"><button class="button" type="button" :disabled="executionTargetsOffset===0 || loadingTargets" @click="loadExecutionTargetPage(Math.max(0,executionTargetsOffset-TARGET_PAGE_SIZE))">{{ t("上一页目标", "Previous targets") }}</button><span>{{ executionTargetsOffset + 1 }}–{{ executionTargetsOffset + executionTargets.length }} / {{ executionTargetsTotal }}</span><button class="button" type="button" :disabled="!executionTargets.length || BigInt(executionTargetsOffset+TARGET_PAGE_SIZE)>=BigInt(executionTargetsTotal) || loadingTargets" @click="loadExecutionTargetPage(executionTargetsOffset+TARGET_PAGE_SIZE)">{{ t("下一页目标", "Next targets") }}</button></div>
          <label class="cc-reason">{{ t("本次操作原因", "Reason for this operation") }}<textarea data-testid="cc-execution-reason" v-model="reason" maxlength="500" /></label><div class="cc-actions"><button class="button" data-testid="cc-approve" type="button" :disabled="!canApprove" @click="openReview('approve')">{{ t("批准此更正计划", "Approve this correction plan") }}</button><button class="button" data-testid="cc-continue" type="button" :disabled="!canContinue" @click="openReview('continue')">{{ t("显式继续暂停周期", "Explicitly continue held cycle") }}</button><button class="button" data-testid="cc-execute-retry" type="button" :disabled="!canRetry" @click="openReview('execute_retry')">{{ t("重试技术失败", "Retry technical failure") }}</button></div>
        </div>
      </section>
      <div v-if="unknownIntent" class="cc-error" data-testid="cc-unknown"><span>{{ t("待人工复核的原请求", "Original request awaiting manual review") }} · {{ unknownIntent.operation }} · v{{ unknownIntent.body.version }} · {{ unknownIntent.key }}</span><button class="button" type="button" :disabled="writing || !canResolveUnknown" @click="retryUnknown">{{ t("查看原请求并重试", "Review original request and retry") }}</button></div>
      <div v-if="conflict" class="cc-review-card" data-testid="cc-conflict"><p>{{ t("409 原请求保留中；所有写入锁定。", "The original 409 request is retained; all writes are locked.") }}</p><button class="button" type="button" :disabled="conflictReloading" @click="refreshConflict">{{ t("刷新所有相关状态", "Refresh all related state") }}</button><label class="cc-confirm"><input type="checkbox" :disabled="!conflictRefreshed || conflictReloading" :checked="discardReviewed" @change="discardReviewed=($event.target as HTMLInputElement).checked" />{{ t("我已核对刷新后的策略、列表、详情及目标，并明确丢弃旧请求", "I reviewed the refreshed policy, lists, details and targets, and explicitly discard the old request") }}</label><button class="button" type="button" :disabled="!conflictRefreshed || !discardReviewed || conflictReloading" @click="discardConflict">{{ t("确认丢弃 409 原请求", "Confirm discard of original 409 request") }}</button></div>
      <section v-if="receipt" class="cc-receipt" data-testid="cc-receipt"><strong>{{ t("操作回执（服务器 ACK）", "Operation receipt (server ACK)") }}</strong><code>{{ receiptText() }}</code><p>{{ t("这是原操作回执，不是当前 GET 状态。若刷新失败仍保留此回执，不会视为未知或自动重发。", "This is the original operation receipt, separate from current GET state. A failed refresh keeps the receipt and never triggers an unknown state or resend.") }}</p><div v-if="Object.keys(errors).length" role="alert" class="cc-error">{{ Object.values(errors).join(" · ") }}</div></section>
      <section v-if="dialog" class="cc-review-card" data-testid="cc-review"><h2>{{ t("确认管理操作", "Confirm administrative action") }} · {{ dialog.operation }}</h2><dl class="cc-facts"><dt>{{ t("品牌 / 操作 / 目标", "Brand / operation / target") }}</dt><dd>{{ dialog.brandId }} / {{ dialog.operation }} / {{ dialog.targetId ?? "—" }}</dd><dt>{{ t("actor / 版本", "Actor / version") }}</dt><dd>{{ dialog.actorId }} / {{ dialog.body.version }}</dd><dt>{{ t("原因 / 幂等键", "Reason / idempotency key") }}</dt><dd>{{ dialog.body.reason }}<small>{{ dialog.key }}</small></dd><dt>{{ t("请求正文", "Request body") }}</dt><dd><code>{{ JSON.stringify(dialog.body) }}</code></dd></dl><label class="cc-confirm"><input data-testid="cc-confirm-check" type="checkbox" :checked="confirmed" :disabled="busyRead" @change="confirmed=($event.target as HTMLInputElement).checked" />{{ t("我确认以上品牌、版本、原因和请求正文；现在才提交此操作。", "I confirm the brand, version, reason and request body above; submit only now.") }}</label><div class="cc-actions"><button class="button button-primary" data-testid="cc-submit" type="button" :disabled="writing || busyRead || !confirmed" @click="submit(dialog)">{{ writing ? t("提交中…", "Submitting…") : unknownIntent ? t("用原正文和幂等键重试", "Retry original body and key") : t("确认并提交", "Confirm and submit") }}</button><button class="button" type="button" :disabled="writing" @click="dialog=null;confirmed=false">{{ t("取消", "Cancel") }}</button></div></section>
    </template>
  </article>
</template>

<style scoped>
/* Global admin table metadata uses nowrap. Reset it for stacked mobile rows:
   overflow widens the mobile visual viewport and offsets physical hit targets. */
.cc-table-wrap td small{white-space:normal;min-width:0}.cc-detail h3,.cc-review-card h2{overflow-wrap:anywhere}
.correction-admin{display:grid;gap:18px;min-width:0}.cc-heading,.cc-section-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:16px}.cc-heading h1,.cc-section h2,.cc-card h2,.cc-detail h3{margin:0}.cc-heading p,.cc-section-heading p,.cc-card p{margin:6px 0 0;color:var(--muted,#64748b);font-size:13px;line-height:1.55}.cc-kicker{font-size:10px!important;letter-spacing:.14em;color:var(--accent,#2563eb)!important;font-weight:800}.cc-callout,.cc-error,.cc-success,.cc-muted,.cc-receipt,.cc-review-card{padding:12px 14px;border-radius:10px;line-height:1.55}.cc-callout{border:1px solid var(--warning,#d97706);background:color-mix(in srgb,var(--warning,#d97706) 8%,transparent)}.cc-error{color:var(--danger,#b42318);background:color-mix(in srgb,var(--danger,#b42318) 8%,transparent);overflow-wrap:anywhere}.cc-success{color:var(--success,#15803d);background:color-mix(in srgb,var(--success,#15803d) 8%,transparent)}.cc-muted{color:var(--muted,#64748b)}.cc-card,.cc-review-card{display:grid;gap:14px;padding:18px;border:1px solid var(--border,#dbe2ea);border-radius:14px;background:var(--surface,#fff);min-width:0}.cc-section{display:grid;gap:12px;min-width:0}.cc-toggle,.cc-confirm{display:flex;align-items:center;gap:10px;font-weight:700}.cc-toggle input,.cc-confirm input{width:18px;height:18px;accent-color:var(--accent,#2563eb);flex:none}.cc-reason{display:grid;gap:6px;font-weight:650}.cc-reason textarea{resize:vertical;min-height:60px;padding:10px;border:1px solid var(--border,#cbd5e1);border-radius:8px;font:inherit}.cc-actions,.cc-pagination{display:flex;align-items:center;gap:8px;flex-wrap:wrap}.cc-pagination{justify-content:space-between}.cc-table-wrap{width:100%;overflow:auto;border:1px solid var(--border,#dbe2ea);border-radius:12px}.cc-table-wrap table{width:100%;border-collapse:collapse;min-width:850px}.cc-table-wrap th,.cc-table-wrap td{text-align:left;padding:10px 12px;border-bottom:1px solid var(--border,#e2e8f0);vertical-align:top}.cc-table-wrap th{font-size:11px;color:var(--muted,#64748b);background:var(--surface-muted,#f8fafc)}.cc-table-wrap tr:last-child td{border-bottom:0}.cc-table-wrap tr.selected{background:color-mix(in srgb,var(--accent,#2563eb) 7%,transparent)}.cc-table-wrap td small,.cc-facts dd small,.cc-review-card dd small{display:block;margin-top:4px;color:var(--muted,#64748b);overflow-wrap:anywhere}.cc-link{padding:0;border:0;background:transparent;color:var(--accent,#2563eb);font:inherit;text-align:left;overflow-wrap:anywhere;cursor:pointer}.cc-status,.cc-hold{display:inline-flex;padding:4px 8px;border-radius:999px;background:#eef2f7;font-size:12px;font-weight:700}.cc-hold{color:#9a3412;background:#fff7ed}.cc-facts{display:grid;grid-template-columns:minmax(170px,.35fr) minmax(0,1fr);gap:8px 14px;margin:0}.cc-facts dt{color:var(--muted,#64748b)}.cc-facts dd{margin:0;overflow-wrap:anywhere}.cc-detail h4{margin:4px 0}.cc-receipt{display:grid;gap:8px;border:1px solid var(--success,#15803d);background:color-mix(in srgb,var(--success,#15803d) 7%,transparent);min-width:0}.cc-receipt code,.cc-review-card code{overflow-wrap:anywhere;white-space:pre-wrap}.cc-review-card{max-width:780px;width:100%;margin-inline:auto;box-shadow:0 20px 60px #0f172a30}.cc-review-card h2,.cc-review-card p{margin:0}.cc-review-card p{color:var(--muted,#64748b)}
@media(max-width:700px){.cc-heading,.cc-section-heading{align-items:stretch;flex-direction:column}.cc-heading>.button,.cc-section-heading>.button{align-self:flex-start}.cc-card,.cc-review-card{padding:14px}.cc-facts{grid-template-columns:1fr;gap:4px}.cc-facts dd{margin-bottom:8px}.cc-actions>.button{flex:1 1 100%}.cc-pagination{align-items:flex-start;flex-direction:column}.cc-table-wrap{overflow:visible;border:0}.cc-table-wrap table,.cc-table-wrap tbody,.cc-table-wrap tr,.cc-table-wrap td{display:block;width:100%;min-width:0}.cc-table-wrap thead{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0,0,0,0)}.cc-table-wrap tbody{display:grid;gap:10px}.cc-table-wrap tbody tr{border:1px solid var(--border,#dbe2ea);border-radius:10px;padding:8px;background:var(--surface,#fff)}.cc-table-wrap td{border:0;padding:6px 4px;overflow-wrap:anywhere}.cc-table-wrap td+td{border-top:1px dashed var(--border,#dbe2ea)}.cc-error{display:grid;gap:10px}}
</style>
