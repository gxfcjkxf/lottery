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
import FilterAssociationSelect from '@lottery/shared/FilterAssociationSelect.vue';
import PlatformTemporalInput from '@lottery/shared/PlatformTemporalInput.vue';
import {vGlobalLoading} from '@lottery/shared';
import {commissionDateRange,type CommissionListQuery} from './commission-list-query';
import { useOperationToast } from "@lottery/shared";
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
  type CorrectionExecution, type CorrectionExecutionTarget, type CorrectionCase, type CorrectionCasePage,
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
const executionPageSize = ref(100)
const planPageSize = ref(100)


const props = defineProps<{ account: AdminAccount; brandId: string; brandStatus?: string; cycleId?: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void; (event: "writing-change", value: boolean): void }>();
const { t, locale } = useAdminI18n();
const operationLabel = (operation: CommissionCorrectionOperation) => t({ policy: 'admin.copy.3a0deb6f', plan_retry: 'admin.copy.18acbe2a', approve: 'admin.copy.028518f0', continue: 'admin.copy.cdb10f42', execute_retry: 'admin.copy.50dae2bb' }[operation] as Parameters<typeof t>[0]);
const api = computed(() => createCommissionCorrectionsApi(fetch, props.account.id));
const paymentsApi = computed(() => createCommissionPaymentsApi(fetch, props.account.id));
const PAGE_SIZE = ref(20);

const casePage=ref<CorrectionCasePage|null>(null),caseOffset=ref(0),caseLoading=ref(false);
const queue=ref('pending'),fromDraft=ref(''),toDraft=ref(''),cycleDraft=ref(''),agentDraft=ref('');
const caseFilter=ref<CommissionListQuery>({state:'pending'}), searched=ref(true),showPolicy=ref(false),detailOpen=ref(false);
const detailTab=ref<'calculationChange'|'planDetails'|'execution'>('calculationChange');
let caseTicket=0,caseOpenTicket=0;
const caseColumns=computed(()=>[{key:'cycle',label:t('admin.commission.ui.cycle')},{key:'state',label:t('admin.commission.ui.state')},
 {key:'before',label:t('admin.commission.ui.originalPoints'),cellClass:'numeric'}, {key:'after',label:t('admin.commission.ui.newPoints'),cellClass:'numeric'},
 {key:'credit',label:t('admin.commission.ui.creditPoints'),cellClass:'numeric'},{key:'debit',label:t('admin.commission.ui.debitPoints'),cellClass:'numeric'},
 {key:'updated',label:t('admin.commission.ui.updated')},{key:'actions',label:t('admin.commission.ui.actions')}]);
const planColumns=computed(()=>[{key:'agent',label:t('admin.commission.ui.agent')},{key:'before',label:t('admin.commission.ui.originalPoints')},{key:'after',label:t('admin.commission.ui.newPoints')},{key:'delta',label:t('admin.commission.ui.deltaPoints')}]);
const executionColumns=computed(()=>[...planColumns.value,{key:'state',label:t('admin.commission.ui.state')}]);
function caseState(row:CorrectionCase){return row.plan.state==='stale'?planState(row.plan.state):row.execution_state?executionState(row.execution_state):row.plan.state==='ready'?t('admin.commission.ui.readyWaiting'):planState(row.plan.state)}
function hasCompletePlan(row:CorrectionCase){return row.plan.state==='ready'||row.execution_id!==null}
async function loadCases(offset=caseOffset.value,preserve=false){
 if(!rights.value.view)return;if(!preserve)invalidateConflictReview();invalidateActionReview();
 const ticket=++caseTicket,captured=scopeStamp.value,session=commissionCorrectionSessionGeneration();caseLoading.value=true;delete errors.value.cases;
 try{const result=await api.value.listCases(props.brandId,PAGE_SIZE.value,offset,caseFilter.value);if(!alive||ticket!==caseTicket||captured!==scopeStamp.value||session!==commissionCorrectionSessionGeneration()||!rights.value.view)return;casePage.value=result;caseOffset.value=offset}
 catch(cause){if(alive&&ticket===caseTicket&&captured===scopeStamp.value&&session===commissionCorrectionSessionGeneration())reportReadError('cases',cause,captured,session)}
 finally{if(ticket===caseTicket)caseLoading.value=false}
}
async function applyCaseFilters(){
  if (agentDraftSearchPending.value) return;
 try{caseFilter.value={...commissionDateRange(fromDraft.value,toDraft.value),...(queue.value?{state:queue.value}:{}),...((props.cycleId || cycleDraft.value)?{cycle_id:props.cycleId || cycleDraft.value}:{}),...(agentDraft.value?{agent_id:agentDraft.value}:{})};searched.value=true;closeCase();await loadCases(0)}
 catch{errors.value={...errors.value,cases:t('admin.commission.ui.dateError')}}
}
async function chooseQueue(value:string){queue.value=value;await applyCaseFilters()}

function dirtyCases(){caseTicket++;casePage.value=null;caseLoading.value=false;searched.value=false}
function closeCase(){caseOpenTicket++;detailTicket++;targetsTicket++;detailOpen.value=false;selectedPlanId.value=selectedExecutionId.value='';plan.value=null;execution.value=null;planTargets.value=[];executionTargets.value=[];loadingTargets.value=false;invalidateActionReview()}
async function openCase(row:CorrectionCase){
 const ticket=++caseOpenTicket,captured=scopeStamp.value,session=commissionCorrectionSessionGeneration();
 detailOpen.value=true;detailTab.value='calculationChange';selectedExecutionId.value='';execution.value=null;executionTargets.value=[];
 await selectPlan(row.id);
 if(!alive||ticket!==caseOpenTicket||captured!==scopeStamp.value||session!==commissionCorrectionSessionGeneration()||!plan.value||plan.value.id!==row.id)return;
 if(row.execution_id)await selectExecution(row.execution_id);
}

const rights = computed(() => commissionCorrectionPermissions(props.account, props.brandId));
const scopeStamp = computed(() => JSON.stringify([props.account.id, props.account.super_admin, props.account.brand_ids,
  props.account.permissions, props.account.permissions_by_brand, props.account.platform_permissions, props.brandId, props.brandStatus, rights.value, props.cycleId]));
const policy = ref<CommissionCorrectionPolicy | null>(null), planPage = ref<Awaited<ReturnType<ReturnType<typeof createCommissionCorrectionsApi>["listPlans"]>> | null>(null);
const executionPage = ref<Awaited<ReturnType<ReturnType<typeof createCommissionCorrectionsApi>["listExecutions"]>> | null>(null);
const plan = ref<CorrectionPlan | null>(null), execution = ref<CorrectionExecution | null>(null);
const paymentGate = ref<boolean | null>(null);
const planTargets = ref<CorrectionPlanTarget[]>([]), executionTargets = ref<CorrectionExecutionTarget[]>([]);
const planTargetsTotal = ref("0"), executionTargetsTotal = ref("0"), planTargetsOffset = ref(0), executionTargetsOffset = ref(0);
const failedPlanOffset = ref<number | null>(null), failedExecutionOffset = ref<number | null>(null);
const selectedPlanId = ref(""), selectedExecutionId = ref("");
const receipt = ref<{ operation: CommissionCorrectionOperation; value: unknown } | null>(null);
const loading = ref(false), loadingPolicy = ref(false), loadingPaymentGate = ref(false), loadingPlans = ref(false), loadingExecutions = ref(false), loadingTargets = ref(false), writing = ref(false);
const errors = ref<Record<string, string | import("@lottery/shared").LocalizedMessage>>({}), writeError = ref<string | import("@lottery/shared").LocalizedMessage>("");
const notice = ref<"confirmed" | "discarded" | "">("");
const correctionNoticeText = (value: "confirmed" | "discarded" | "") => value === "confirmed"
  ? t('admin.copy.064adf92')
  : value === "discarded"
    ? t('admin.copy.da960a7d') : "";
const reason = ref(""), policyEnabled = ref(false), confirmed = ref(false), dialog = ref<PendingCommissionCorrectionIntent | null>(null);
const unknownIntent = ref<PendingCommissionCorrectionIntent | null>(null), conflictIntent = ref<PendingCommissionCorrectionIntent | null>(null);
const conflict = ref(false), conflictReloading = ref(false), conflictRefreshed = ref(false), discardReviewed = ref(false);
const planOffset = ref(0), executionOffset = ref(0);
let alive = true, policyTicket = 0, paymentGateTicket = 0, plansTicket = 0, executionsTicket = 0, detailTicket = 0, targetsTicket = 0, writeTicket = 0, conflictTicket = 0;

const busyRead = computed(() => loading.value || loadingPolicy.value || loadingPaymentGate.value || loadingPlans.value || loadingExecutions.value || loadingTargets.value || caseLoading.value);
const canWrite = computed(() => rights.value.view && props.brandStatus !== "disabled" && !props.account.super_admin && !writing.value && !unknownIntent.value && !conflict.value);
const canPolicy = computed(() => canWrite.value && !busyRead.value && !loadingTargets.value && !errors.value.policy && rights.value.policyWrite && policy.value && policy.value.version < Number.MAX_SAFE_INTEGER && validReason(reason.value));
const canPlanRetry = computed(() => canWrite.value && !busyRead.value && !loadingTargets.value && !errors.value.plan && !errors.value.plans && !errors.value.planTargets && rights.value.planRetry && paymentGate.value === true && !errors.value.paymentGate && plan.value?.state === "failed" && plan.value.version < Number.MAX_SAFE_INTEGER && validReason(reason.value));
const executionMatchesPlan = computed(() => Boolean(plan.value && execution.value && plan.value.state==='ready' && execution.value.plan_id===plan.value.id && execution.value.plan_version===plan.value.version && execution.value.evidence_epoch===plan.value.evidence_epoch));
const canApprove = computed(() => executionMatchesPlan.value && policy.value?.enabled === true && paymentGate.value === true && !errors.value.policy && !errors.value.paymentGate && canWrite.value && !busyRead.value && !loadingTargets.value && !errors.value.execution && !errors.value.executions && !errors.value.planTargets && !errors.value.executionTargets && rights.value.approve && execution.value?.state === "awaiting_approval" && ["manual", "mixed"].includes(execution.value.payout_mode) && execution.value.version < Number.MAX_SAFE_INTEGER && validReason(reason.value));
const canContinue = computed(() => executionMatchesPlan.value && policy.value?.enabled === true && paymentGate.value === true && !errors.value.policy && !errors.value.paymentGate && canWrite.value && !busyRead.value && !loadingTargets.value && !errors.value.execution && !errors.value.executions && !errors.value.planTargets && !errors.value.executionTargets && rights.value.continue && execution.value?.state === "paused" && Boolean(execution.value.approval_actor_type) && Boolean(execution.value.approval_audit_log_id) && execution.value.cycle_hold_active === true && execution.value.version < Number.MAX_SAFE_INTEGER && validReason(reason.value));
const canRetry = computed(() => executionMatchesPlan.value && policy.value?.enabled === true && paymentGate.value === true && !errors.value.policy && !errors.value.paymentGate && canWrite.value && !busyRead.value && !loadingTargets.value && !errors.value.execution && !errors.value.executions && !errors.value.planTargets && !errors.value.executionTargets && rights.value.retry && execution.value?.state === "failed" && execution.value.version < Number.MAX_SAFE_INTEGER && validReason(reason.value));
const canResolveUnknown = computed(() => Boolean(unknownIntent.value && rights.value.view && props.brandStatus !== "disabled" &&
  (unknownIntent.value.operation === "policy" ? rights.value.policyWrite : unknownIntent.value.operation === "plan_retry" ? rights.value.planRetry : unknownIntent.value.operation === "approve" ? rights.value.approve : unknownIntent.value.operation === "continue" ? rights.value.continue : rights.value.retry)));

function validReason(value: string): boolean {
  if (value.trim() !== value || Array.from(value).length > 500 || /[\u0000-\u001f\u007f-\u009f]/u.test(value)) return false;
  for (let i = 0; i < value.length; i++) { const code = value.charCodeAt(i); if (code >= 0xd800 && code <= 0xdbff) { const next = value.charCodeAt(++i); if (!(next >= 0xdc00 && next <= 0xdfff)) return false; } else if (code >= 0xdc00 && code <= 0xdfff) return false; }
  return true;
}
function current(ticket: number, lane: string, captured: string, session: number): boolean {
  const laneTicket = lane === "policy" ? policyTicket : lane === "plans" ? plansTicket : lane === "executions" ? executionsTicket : lane === "detail" ? detailTicket : targetsTicket;
  return alive && ticket === laneTicket && captured === scopeStamp.value && session === commissionCorrectionSessionGeneration() && rights.value.view;
}
function currentWrite(ticket: number, captured: string, session: number): boolean { return alive && ticket === writeTicket && captured === scopeStamp.value && session === commissionCorrectionSessionGeneration(); }
function clearVisible() {
  caseTicket++;caseOpenTicket++;casePage.value=null;caseOffset.value=0;caseLoading.value=false;detailOpen.value=false;showPolicy.value=false;
  policy.value = null; planPage.value = null; executionPage.value = null; plan.value = null; execution.value = null;
  planTargets.value = []; executionTargets.value = []; planTargetsTotal.value = executionTargetsTotal.value = "0"; planTargetsOffset.value = executionTargetsOffset.value = 0; failedPlanOffset.value = failedExecutionOffset.value = null; selectedPlanId.value = ""; selectedExecutionId.value = ""; paymentGate.value = null;
  receipt.value = null; reason.value = ""; dialog.value = null; confirmed.value = false;
  unknownIntent.value = null; conflictIntent.value = null; conflict.value = false; conflictRefreshed.value = false; discardReviewed.value = false;
  errors.value = {}; writeError.value = ""; notice.value = ""; loading.value = loadingPolicy.value = loadingPaymentGate.value = loadingPlans.value = loadingExecutions.value = loadingTargets.value = writing.value = false;
}
function resetScope(_next?: string, previous?: string) {
  if (previous) { try { const old = JSON.parse(previous) as unknown[]; hideCommissionCorrectionIntentsForScope(String(old[0]), String(old[6])); } catch { /* Old scope cannot reveal an intent. */ } }
  policyTicket++; paymentGateTicket++; plansTicket++; executionsTicket++; detailTicket++; targetsTicket++; writeTicket++; conflictTicket++;
  clearVisible();
  const pending = listPendingCommissionCorrectionIntents(props.account.id, props.brandId);
  const conflictPending = pending.find(isCommissionCorrectionIntentConflict);
  conflictIntent.value = conflictPending ?? null; conflict.value = Boolean(conflictPending);
  unknownIntent.value = conflictPending ? null : pending[0] ?? null;
  if (unknownIntent.value) writeError.value = t('admin.copy.edf12fc2');
  fromDraft.value=toDraft.value=cycleDraft.value=agentDraft.value='';queue.value=props.cycleId?'':'pending';caseFilter.value=props.cycleId?{cycle_id:props.cycleId}:{state:'pending'};searched.value=true;
  if (rights.value.view) void reloadAll();
}
function sessionExpired(cause: unknown, captured: string, session: number): boolean {
  if (cause instanceof AdminApiError && cause.status === 401 && alive && captured === scopeStamp.value && session === commissionCorrectionSessionGeneration()) {
    policyTicket++; paymentGateTicket++; plansTicket++; executionsTicket++; detailTicket++; targetsTicket++; writeTicket++; clearAllCommissionCorrectionIntents(); clearVisible(); emit("session-invalid"); return true;
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
async function loadPaymentGate() {
  if (!rights.value.view) return;
  const ticket = ++paymentGateTicket, captured = scopeStamp.value, session = commissionCorrectionSessionGeneration();
  loadingPaymentGate.value = true; paymentGate.value = null;
  try {
    const value = await paymentsApi.value.getPolicy(props.brandId);
    if (alive && ticket === paymentGateTicket && captured === scopeStamp.value && session === commissionCorrectionSessionGeneration() && rights.value.view) { paymentGate.value = value.enabled; delete errors.value.paymentGate; }
  } catch (cause) {
    if (alive && ticket === paymentGateTicket && captured === scopeStamp.value && session === commissionCorrectionSessionGeneration() && rights.value.view) reportReadError('paymentGate', cause, captured, session);
  } finally { if (ticket === paymentGateTicket) loadingPaymentGate.value = false; }
}
async function loadPlans(offset = planOffset.value, preserve = false) {
  if (!rights.value.view) return; if (!preserve) invalidateConflictReview(); invalidateActionReview();
  const ticket = ++plansTicket, captured = scopeStamp.value, session = commissionCorrectionSessionGeneration();
  loadingPlans.value = true;
  try { const value = await api.value.listPlans(props.brandId, PAGE_SIZE.value, offset); if (!current(ticket, "plans", captured, session)) return; planPage.value = value; planOffset.value = offset; delete errors.value.plans;
    if (selectedPlanId.value && !value.items.some((item) => item.id === selectedPlanId.value)) { selectedPlanId.value = ""; plan.value = null; planTargets.value = []; }
  } catch (cause) { if (current(ticket, "plans", captured, session)) reportReadError("plans", cause, captured, session); }
  finally { if (ticket === plansTicket) loadingPlans.value = false; }
}
async function loadExecutions(offset = executionOffset.value, preserve = false) {
  if (!rights.value.view) return; if (!preserve) invalidateConflictReview(); invalidateActionReview();
  const ticket = ++executionsTicket, captured = scopeStamp.value, session = commissionCorrectionSessionGeneration();
  loadingExecutions.value = true;
  try { const value = await api.value.listExecutions(props.brandId, PAGE_SIZE.value, offset); if (!current(ticket, "executions", captured, session)) return; executionPage.value = value; executionOffset.value = offset; delete errors.value.executions;
    if (selectedExecutionId.value && !value.items.some((item) => item.id === selectedExecutionId.value)) { selectedExecutionId.value = ""; execution.value = null; executionTargets.value = []; }
  } catch (cause) { if (current(ticket, "executions", captured, session)) reportReadError("executions", cause, captured, session); }
  finally { if (ticket === executionsTicket) loadingExecutions.value = false; }
}
async function selectPlan(id: string, preserve = false) {
  if (!rights.value.view) return; if (!preserve) invalidateConflictReview(); invalidateActionReview();
  delete errors.value.planTargets;
  selectedPlanId.value = id; plan.value = null; planTargets.value = []; planTargetsTotal.value = "0"; planTargetsOffset.value = 0; failedPlanOffset.value = null; const ticket = ++detailTicket, targetTicket = ++targetsTicket;
  const captured = scopeStamp.value, session = commissionCorrectionSessionGeneration(); loadingTargets.value = true;
  try {
    const [fresh, targetPage] = await Promise.all([api.value.readPlan(props.brandId, id), api.value.listPlanTargets(props.brandId, id, planPageSize.value, 0)]);
    if (!current(ticket, "detail", captured, session) || targetTicket !== targetsTicket || id !== selectedPlanId.value) return;
    if (props.cycleId && fresh.cycle_id !== props.cycleId) throw new AdminApiError("Unexpected commission cycle", 502, "INVALID_RESPONSE");
    plan.value = fresh; planTargets.value = targetPage.items; planTargetsTotal.value = targetPage.total_count; delete errors.value.plan;
  } catch (cause) { if (current(ticket, "detail", captured, session) && targetTicket === targetsTicket) reportReadError("plan", cause, captured, session); }
  finally { if (alive && targetTicket === targetsTicket && captured === scopeStamp.value) loadingTargets.value = false; }
}
async function selectExecution(id: string, preserve = false) {
  if (!rights.value.view) return; if (!preserve) invalidateConflictReview(); invalidateActionReview();
  delete errors.value.executionTargets;
  selectedExecutionId.value = id; execution.value = null; executionTargets.value = []; executionTargetsTotal.value = "0"; executionTargetsOffset.value = 0; failedExecutionOffset.value = null; const ticket = ++detailTicket, targetTicket = ++targetsTicket;
  const captured = scopeStamp.value, session = commissionCorrectionSessionGeneration(); loadingTargets.value = true;
  try {
    const [fresh, targetPage] = await Promise.all([api.value.readExecution(props.brandId, id), api.value.listExecutionTargets(props.brandId, id, executionPageSize.value, 0)]);
    if (!current(ticket, "detail", captured, session) || targetTicket !== targetsTicket || id !== selectedExecutionId.value) return;
    if (props.cycleId && fresh.cycle_id !== props.cycleId) throw new AdminApiError("Unexpected commission cycle", 502, "INVALID_RESPONSE");
    execution.value = fresh; executionTargets.value = targetPage.items; executionTargetsTotal.value = targetPage.total_count; delete errors.value.execution;
  } catch (cause) { if (current(ticket, "detail", captured, session) && targetTicket === targetsTicket) reportReadError("execution", cause, captured, session); }
  finally { if (alive && targetTicket === targetsTicket && captured === scopeStamp.value) loadingTargets.value = false; }
}
async function loadPlanTargetPage(offset: number) {
  if (!plan.value || !rights.value.view) return; invalidateConflictReview(); invalidateActionReview();
  const id = plan.value.id, ticket = ++targetsTicket, captured = scopeStamp.value, session = commissionCorrectionSessionGeneration(); loadingTargets.value = true;
  try { const page = await api.value.listPlanTargets(props.brandId, id, planPageSize.value, offset); if (!current(ticket, "targets", captured, session) || selectedPlanId.value !== id) return; planTargets.value = page.items; planTargetsTotal.value = page.total_count; planTargetsOffset.value = offset; failedPlanOffset.value = null; delete errors.value.planTargets; }
  catch (cause) { if (current(ticket, "targets", captured, session)) { failedPlanOffset.value = offset; reportReadError("planTargets", cause, captured, session); } }
  finally { if (alive && ticket === targetsTicket && captured === scopeStamp.value) loadingTargets.value = false; }
}
async function loadExecutionTargetPage(offset: number) {
  if (!execution.value || !rights.value.view) return; invalidateConflictReview(); invalidateActionReview();
  const id = execution.value.id, ticket = ++targetsTicket, captured = scopeStamp.value, session = commissionCorrectionSessionGeneration(); loadingTargets.value = true;
  try { const page = await api.value.listExecutionTargets(props.brandId, id, executionPageSize.value, offset); if (!current(ticket, "targets", captured, session) || selectedExecutionId.value !== id) return; executionTargets.value = page.items; executionTargetsTotal.value = page.total_count; executionTargetsOffset.value = offset; failedExecutionOffset.value = null; delete errors.value.executionTargets; }
  catch (cause) { if (current(ticket, "targets", captured, session)) { failedExecutionOffset.value = offset; reportReadError("executionTargets", cause, captured, session); } }
  finally { if (alive && ticket === targetsTicket && captured === scopeStamp.value) loadingTargets.value = false; }
}
async function reloadAll(preserve = false) {
  if (!rights.value.view) return; loading.value = true;
  await Promise.all([loadPolicy(preserve), loadPaymentGate(), loadCases(caseOffset.value, preserve)]);
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
async function retryUnknown() {
  const intent = unknownIntent.value;
  if (!intent || !canResolveUnknown.value) return;
  if (props.cycleId && intent.operation !== "policy") {
    if (intent.operation === "plan_retry") await selectPlan(intent.targetId!, true);
    else await selectExecution(intent.targetId!, true);
    const target = intent.operation === "plan_retry" ? plan.value : execution.value;
    if (unknownIntent.value !== intent || !target || target.id !== intent.targetId || target.cycle_id !== props.cycleId) return;
  }
  dialog.value = intent; confirmed.value = false;
}
function validIntentScope(intent: PendingCommissionCorrectionIntent): boolean { return intent.actorId === props.account.id && intent.brandId === props.brandId && (intent.operation === "policy" ? intent.targetId === undefined : Boolean(intent.targetId)); }
async function submit(intent: PendingCommissionCorrectionIntent) {
  const recovery = Boolean(unknownIntent.value && unknownIntent.value.key === intent.key);
  const cycleTarget = intent.operation === "plan_retry" ? plan.value : execution.value;
  if (props.cycleId && intent.operation !== "policy" && (!cycleTarget || cycleTarget.id !== intent.targetId || cycleTarget.cycle_id !== props.cycleId)) return;
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
        policyTicket++; paymentGateTicket++; plansTicket++; executionsTicket++; detailTicket++; targetsTicket++; writeTicket++; clearVisible(); emit("session-invalid");
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
  const results = await Promise.allSettled([loadPolicy(true), loadPaymentGate(), loadPlans(planOffset.value, true), loadExecutions(executionOffset.value, true)]);
  if (planToRefresh) await selectPlan(planToRefresh, true);
  if (executionToRefresh) await selectExecution(executionToRefresh, true);
  const planRequired = Boolean(planToRefresh), executionRequired = Boolean(executionToRefresh);
  if (alive && ticket === conflictTicket && captured === scopeStamp.value && rights.value.view && results.every((result) => result.status === "fulfilled") && Object.keys(errors.value).length === 0 && (!planRequired || plan.value?.id === planToRefresh) && (!executionRequired || execution.value?.id === executionToRefresh) && Boolean(policy.value && paymentGate.value !== null && planPage.value && executionPage.value)) {
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
function receiptText(): string { return receipt.value ? operationLabel(receipt.value.operation) : ""; }

watch([fromDraft,toDraft,cycleDraft,agentDraft],dirtyCases,{flush:'sync'});
watch(writing, value => emit("writing-change", value), { flush: "sync" });
watch(scopeStamp, resetScope, { immediate: true });
onUnmounted(() => { alive = false; caseTicket++;caseOpenTicket++; policyTicket++; paymentGateTicket++; plansTicket++; executionsTicket++; detailTicket++; targetsTicket++; writeTicket++; conflictTicket++; });

useOperationToast(() => writeError.value, value => displayPublicError(value), 'error');
useOperationToast(() => notice.value, value => correctionNoticeText(value), 'success');
useOperationToast(() => errors.value.policy, value => displayPublicError(value), 'error');
useOperationToast(() => errors.value.paymentGate, value => displayPublicError(value), 'error');
useOperationToast(() => errors.value.planTargets, value => displayPublicError(value), 'error');
useOperationToast(() => errors.value.executionTargets, value => displayPublicError(value), 'error');
useOperationToast(() => errors.value.cases, value => displayPublicError(value), 'error');
useOperationToast(() => errors.value.plans, value => displayPublicError(value), 'error');
useOperationToast(() => errors.value.executions, value => displayPublicError(value), 'error');
</script>

<template>
 <article class="commission-workspace correction-admin">
  <header class="console-page-heading commission-heading"><h1 v-if="!cycleId" class="console-page-title">{{ t('admin.commission.ui.corrections') }}</h1></header>
  <p v-if="!rights.view" class="empty-state">{{ t('admin.copy.cdaf5831') }}</p>
  <template v-else>
   <p v-if="brandStatus==='disabled'" class="commission-warning">{{ t('admin.commission.ui.disabled') }}</p>
    <ManagementList><template #tabs><nav class="commission-tabs" role="tablist"><button v-for="tab in [{value:'pending',label:'correctionPending'},{value:'processing',label:'correctionProcessing'},{value:'history',label:'correctionHistory'},{value:'',label:'all'}]" :key="tab.value" role="tab" class="button" :aria-selected="queue===tab.value" :disabled="caseLoading" @click="withGlobalLoading(chooseQueue,tab.value)">{{ t(adminKey('admin.commission.ui.'+tab.label)) }}</button></nav></template><template #actions><button v-if="rights.view" class="button" @click="showPolicy=true">{{ t('admin.commission.ui.settings') }}</button></template><template #filters><form v-inline-validation novalidate class="commission-toolbar ui-filter-bar" @submit.prevent="withGlobalLoading(applyCaseFilters)"><label><PlatformTemporalInput :label="t('admin.commission.ui.from')" :placeholder="t('admin.commission.ui.from')" v-model="fromDraft" type="date" :locale="locale" /></label><label><PlatformTemporalInput :label="t('admin.commission.ui.to')" :placeholder="t('admin.commission.ui.to')" v-model="toDraft" type="date" :locale="locale" /></label><label v-if="!cycleId"><FilterAssociationSelect v-model="cycleDraft" :brand-id="brandId" surface="admin" resource="commission" kind="cycle" :label="t('admin.commission.ui.cycle')" :locale="locale" :scope-key="scopeStamp" :enabled="rights.view" /></label><label><FilterAssociationSearch v-model="agentDraft" v-model:pending="agentDraftSearchPending" :reset-key="accountSearchReset" :brand-id="brandId" surface="admin" resource="commission" kind="agent" :label="t('admin.commission.ui.agent')" :locale="locale" :scope-key="scopeStamp" :enabled="rights.view" /></label><div class="commission-actions ui-form-actions"><button class="button button-primary" type="submit" :disabled="agentDraftSearchPending || caseLoading">{{ t('admin.commission.ui.query') }}</button></div></form></template>
    <p v-if="!searched" class="commission-note">{{ t('admin.commission.ui.filtersChanged') }}</p>
    <p v-if="errors.cases" class="commission-warning" role="alert">{{ displayPublicError(errors.cases) }} <button :disabled="agentDraftSearchPending" v-refresh-button="t('admin.commission.ui.retryRead')" class="button" @click="withGlobalLoading(applyCaseFilters)">{{ t('admin.commission.ui.retryRead') }}</button></p>
    <p v-if="caseLoading" v-global-loading>{{ t('admin.copy.ab7239f3') }}</p>
    <ManagementTable :entries="casePage?.items??[]" :columns="caseColumns" :empty="!!casePage && !casePage.items.length" :empty-label="t('admin.commission.ui.empty')">
     <template #cycle="{entry}"><button class="text-button commission-link" :data-case-id="entry.id" @click="withGlobalLoading(openCase,entry)"><AssociationName :record="entry.plan" kind="cycle" :id="entry.plan.cycle_id" :locale="locale" /></button></template>
     <template #state="{entry}"><span class="commission-status" :class="entry.plan.state==='stale'?entry.plan.state:entry.execution_state??entry.plan.state">{{ caseState(entry) }}</span></template>
     <template #before="{entry}">{{ hasCompletePlan(entry)?entry.plan.before_points:t('admin.commission.ui.incomplete') }}</template><template #after="{entry}">{{ hasCompletePlan(entry)?entry.plan.calculated_points:t('admin.commission.ui.incomplete') }}</template><template #credit="{entry}">{{ hasCompletePlan(entry)?entry.plan.credit_points:'—' }}</template><template #debit="{entry}">{{ hasCompletePlan(entry)?entry.plan.debit_points:'—' }}</template>
     <template #updated="{entry}">{{ formatDate(entry.plan.updated_at) }}</template><template #actions="{entry}"><MemberActionsMenu :id="'CommissionCorrectionsManagement-'+entry.id" :label="t('admin.copy.d21fcbef')"><button type="button" role="menuitem" class="button" @click="withGlobalLoading(openCase,entry)">{{ t('admin.commission.ui.view') }}</button></MemberActionsMenu></template>
    </ManagementTable>
    <template #pagination><TablePagination v-if="casePage" class="commission-pager" :total="casePage.total_count" :page="Math.floor(casePage.offset / PAGE_SIZE + 1)" :count="casePage.items.length" :locale="locale" :page-size="PAGE_SIZE" :previous-disabled="caseLoading||caseOffset===0" :next-disabled="caseLoading||BigInt(caseOffset+PAGE_SIZE)>=BigInt(casePage.total_count)" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.2e9c9e7a')" :go-to="(target: number) => loadCases((target - 1) * PAGE_SIZE)" :change-page-size="async (size: number) => { PAGE_SIZE = size; await loadCases(0) }" :busy="caseLoading" /></template></ManagementList>
   <DetailModal data-testid="correction-detail-dialog" v-if="detailOpen" :open="!dialog && !showPolicy" :title="t('admin.commission.ui.corrections')" :close-label="t('association.close')" :close-disabled="writing" @close="closeCase">
   <section class="commission-detail">
    <p v-if="loadingTargets" v-global-loading>{{ t('admin.copy.ab7239f3') }}</p>
    <p v-if="errors.plan || errors.execution" class="commission-warning" role="alert">{{ displayPublicError(errors.plan ?? errors.execution) }}</p>
    <template v-if="plan">
     <div class="commission-heading"><b><AssociationName :record="plan" kind="cycle" :id="plan.cycle_id" :locale="locale" /></b><span class="commission-status" :class="plan.state==='stale'?plan.state:execution?.state??plan.state">{{ plan.state==='stale'?planState(plan.state):execution?executionState(execution.state):plan.state==='ready'?t('admin.commission.ui.readyWaiting'):planState(plan.state) }}</span></div>
     <p v-if="plan.state==='failed'" class="commission-warning">{{ t('admin.commission.ui.planFailed') }} {{ plan.last_error_code?displayPublicError(publicErrorMessage({code:plan.last_error_code})):'' }}</p>
     <p v-if="execution?.state==='paused'" class="commission-warning">{{ t('admin.commission.ui.executionPaused') }}</p><p v-if="execution?.state==='failed'" class="commission-warning">{{ t('admin.commission.ui.executionFailed') }} {{ execution.last_error_code?displayPublicError(publicErrorMessage({code:execution.last_error_code})):'' }}</p><p v-if="plan.state==='stale'||execution?.state==='stale'" class="commission-warning">{{ t('admin.commission.ui.correctionStale') }}</p>
     <p v-if="paymentGate === false" class="commission-warning">{{ t('admin.commission.ui.policyOff') }}</p>
     <p v-if="policy && !policy.enabled" class="commission-warning">{{ t('admin.copy.f81cb486') }}</p>
     <p v-if="errors.policy || errors.paymentGate" class="commission-warning" role="alert">{{ displayPublicError(errors.policy ?? errors.paymentGate) }} <button v-refresh-button="t('admin.commission.ui.retryRead')" class="button" @click="withGlobalLoading(reloadAll)">{{ t('admin.commission.ui.retryRead') }}</button></p>
     <div class="commission-actions ui-form-actions"><button v-if="plan.state==='failed' && rights.planRetry" class="button button-primary" data-testid="cc-plan-retry" :disabled="!canPlanRetry" @click="openReview('plan_retry')">{{ t('admin.copy.18acbe2a') }}</button><button v-if="execution?.state==='awaiting_approval' && rights.approve" class="button button-primary" data-testid="cc-approve" :disabled="!canApprove" @click="openReview('approve')">{{ t('admin.copy.028518f0') }}</button><button v-if="execution?.state==='paused' && rights.continue" class="button button-primary" data-testid="cc-continue" :disabled="!canContinue" @click="openReview('continue')">{{ t('admin.copy.cdb10f42') }}</button><button v-if="execution?.state==='failed' && rights.retry" class="button button-primary" data-testid="cc-execute-retry" :disabled="!canRetry" @click="openReview('execute_retry')">{{ t('admin.copy.50dae2bb') }}</button></div>
     <nav class="commission-tabs" role="tablist"><button v-for="tab in (['calculationChange','planDetails','execution'] as const)" :key="tab" class="button" role="tab" :aria-selected="detailTab===tab" @click="detailTab=tab">{{ t(adminKey('admin.commission.ui.'+tab)) }}</button></nav>
     <section v-if="detailTab==='calculationChange'" class="commission-detail" data-testid="cc-plan-detail">
      <dl class="commission-metrics"><div><dt>{{ t('admin.commission.ui.originalPoints') }}</dt><dd>{{ plan.state==='ready'||execution?plan.before_points:t('admin.commission.ui.incomplete') }}</dd></div><div><dt>{{ t('admin.commission.ui.newPoints') }}</dt><dd>{{ plan.state==='ready'||execution?plan.calculated_points:t('admin.commission.ui.incomplete') }}</dd></div><div><dt>{{ t('admin.commission.ui.creditPoints') }}</dt><dd>{{ plan.state==='ready'||execution?plan.credit_points:'—' }}</dd></div><div><dt>{{ t('admin.commission.ui.debitPoints') }}</dt><dd>{{ plan.state==='ready'||execution?plan.debit_points:'—' }}</dd></div></dl>
      <p class="commission-note">{{ t('admin.commission.ui.amountDifference') }} {{ t('admin.copy.abf96c9b') }}</p><p v-if="!execution" class="commission-note">{{ t('admin.commission.ui.noExecution') }}</p>
      <dl v-else class="commission-facts"><dt>{{ t('admin.commission.ui.progress') }}</dt><dd>{{ execution.applied_count }} / {{ execution.target_count }}</dd><dt>{{ t('admin.commission.ui.appliedPoints') }}</dt><dd>{{ execution.applied_credit_points }} / {{ execution.applied_debit_points }}</dd></dl>
     </section>
     <ManagementList embedded v-if="detailTab==='planDetails'" class="commission-detail"><p v-if="errors.planTargets" class="commission-warning" role="alert">{{ displayPublicError(errors.planTargets) }} <button v-refresh-button="t('admin.commission.ui.retryRead')" class="button" @click="withGlobalLoading(loadPlanTargetPage,failedPlanOffset ?? planTargetsOffset)">{{ t('admin.commission.ui.retryRead') }}</button></p><ManagementTable :entries="planTargets" :columns="planColumns" :empty="!loadingTargets&&!planTargets.length" :empty-label="t('admin.commission.ui.empty')"><template #agent="{entry}"><AssociationName :record="entry" kind="agent" :id="entry.agent_id" :locale="locale" /></template><template #before="{entry}">{{ entry.points_before }}</template><template #after="{entry}">{{ entry.points_after }}</template><template #delta="{entry}">{{ entry.delta_points }}</template></ManagementTable><template #pagination><TablePagination class="commission-pager" :total="planTargetsTotal" :page="Math.floor(planTargetsOffset / planPageSize + 1)" :count="planTargets.length" :locale="locale" :page-size="planPageSize" :previous-disabled="loadingTargets||planTargetsOffset===0" :next-disabled="loadingTargets||BigInt(planTargetsOffset+planPageSize)>=BigInt(planTargetsTotal)" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.2e9c9e7a')" :go-to="(target: number) => loadPlanTargetPage((target - 1) * planPageSize)" :change-page-size="async (size: number) => { planPageSize = size; await loadPlanTargetPage(0) }" /></template></ManagementList>
     <ManagementList embedded v-if="detailTab==='execution'" class="commission-detail" data-testid="cc-execution-detail"><p v-if="!execution" class="commission-note">{{ t('admin.commission.ui.noExecution') }}</p><template v-else><p v-if="errors.executionTargets" class="commission-warning" role="alert">{{ displayPublicError(errors.executionTargets) }} <button v-refresh-button="t('admin.commission.ui.retryRead')" class="button" @click="withGlobalLoading(loadExecutionTargetPage,failedExecutionOffset ?? executionTargetsOffset)">{{ t('admin.commission.ui.retryRead') }}</button></p><dl class="commission-metrics"><div><dt>{{ t('admin.commission.ui.progress') }}</dt><dd>{{ execution.applied_count }} / {{ execution.target_count }}</dd></div><div><dt>{{ t('admin.commission.ui.creditPoints') }}</dt><dd>{{ execution.applied_credit_points }} / {{ execution.credit_points }}</dd></div><div><dt>{{ t('admin.commission.ui.debitPoints') }}</dt><dd>{{ execution.applied_debit_points }} / {{ execution.debit_points }}</dd></div></dl><ManagementTable :entries="executionTargets" :columns="executionColumns" :empty="!loadingTargets&&!executionTargets.length" :empty-label="t('admin.commission.ui.empty')"><template #agent="{entry}"><AssociationName :record="entry" kind="agent" :id="entry.agent_id" :locale="locale" /></template><template #before="{entry}">{{ entry.points_before }}</template><template #after="{entry}">{{ entry.points_after }}</template><template #delta="{entry}">{{ entry.delta_points }}</template><template #state="{entry}">{{ entry.state==='applied' && entry.delta_points==='0'?t('admin.copy.a1cb3520'):entry.state==='applied'?t('admin.commission.targetApplied'):t('admin.commission.targetPending') }}</template></ManagementTable></template><template #pagination><TablePagination class="commission-pager" :total="executionTargetsTotal" :page="Math.floor(executionTargetsOffset / executionPageSize + 1)" :count="executionTargets.length" :locale="locale" :page-size="executionPageSize" :previous-disabled="loadingTargets||executionTargetsOffset===0" :next-disabled="loadingTargets||BigInt(executionTargetsOffset+executionPageSize)>=BigInt(executionTargetsTotal)" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.2e9c9e7a')" :go-to="(target: number) => loadExecutionTargetPage((target - 1) * executionPageSize)" :change-page-size="async (size: number) => { executionPageSize = size; await loadExecutionTargetPage(0) }" /></template></ManagementList>

    </template>
   </section>
   </DetailModal>
   <DetailModal v-if="showPolicy" :open="!dialog" :title="t('admin.commission.ui.settings')" :close-label="t('association.close')" @close="showPolicy=false"><div class="commission-detail" data-testid="cc-policy"><p v-if="errors.policy" class="commission-warning" role="alert">{{ displayPublicError(errors.policy) }} <button v-refresh-button="t('admin.commission.ui.retryRead')" class="button" @click="withGlobalLoading(loadPolicy)">{{ t('admin.commission.ui.retryRead') }}</button></p><label v-if="policy"><input data-testid="cc-policy-toggle" type="checkbox" v-model="policyEnabled" :disabled="!rights.policyWrite || account.super_admin || brandStatus==='disabled' || !policy || busyRead || writing || Boolean(unknownIntent) || conflict" /> {{ policy?.enabled?t('admin.copy.e1cc5fdc'):t('admin.copy.f81cb486') }}</label><div class="ui-form-actions"><button class="button button-primary" data-testid="cc-policy-review" :disabled="!canPolicy" @click="openReview('policy')">{{ t('admin.copy.3a0deb6f') }}</button></div></div></DetailModal>
      <div v-if="unknownIntent" class="cc-error" data-testid="cc-unknown"><span>{{ t('admin.copy.48cb0926') }} · {{ operationLabel(unknownIntent.operation) }} · </span><button v-write-guard="writing" class="button" type="button" :disabled="writing || !canResolveUnknown" @click="withGlobalLoading(retryUnknown)">{{ t('admin.copy.3ef3c644') }}</button></div>
      <div v-if="conflict" class="cc-review-card" data-testid="cc-conflict"><p>{{ t('admin.copy.af8f1d76') }}</p><button v-refresh-button="t('admin.copy.9fd90523')" class="button" type="button" :disabled="conflictReloading" @click="withGlobalLoading(refreshConflict)">{{ t('admin.copy.9fd90523') }}</button><label class="cc-confirm"><input type="checkbox" :disabled="!conflictRefreshed || conflictReloading" :checked="discardReviewed" @change="discardReviewed=($event.target as HTMLInputElement).checked" />{{ t('admin.copy.8d9b3d75') }}</label><button class="button" type="button" :disabled="!conflictRefreshed || !discardReviewed || conflictReloading" @click="discardConflict">{{ t('admin.copy.5079f563') }}</button></div>
      <section v-if="receipt" class="cc-receipt" data-testid="cc-receipt"><strong>{{ t('admin.copy.ff2e9c8f') }}</strong><code>{{ receiptText() }}</code><p>{{ t('admin.copy.6fa8b32f') }}</p><div v-if="Object.keys(errors).length" role="alert" class="cc-error">{{ Object.values(errors).map(displayPublicError).join(" · ") }}</div></section>
      <DetailModal v-if="dialog" class="cc-review-card" :title="t('admin.copy.1803550d')" :close-label="t('association.close')" :close-disabled="writing" @close="if (!writing) {dialog=null;confirmed=false}" data-testid="cc-review"><h2>{{ operationLabel(dialog.operation) }}</h2><dl class="cc-facts"><dt>{{ t('admin.copy.e5cf0427') }}</dt><dd><AssociationName :record="plan ?? policy" kind="brand" :id="dialog.brandId" :locale="associationLocale" /> {{ operationLabel(dialog.operation) }}</dd><template v-if="dialog.targetId && ((dialog.operation==='plan_retry' && plan?.id===dialog.targetId && plan.version===dialog.body.version) || (dialog.operation!=='plan_retry' && execution?.id===dialog.targetId && execution.version===dialog.body.version))"><dt>{{ t('admin.commission.ui.cycle') }}</dt><dd><AssociationName :record="plan ?? execution" kind="cycle" :id="plan?.cycle_id ?? execution?.cycle_id ?? ''" :locale="locale" /></dd><dt>{{ t('admin.commission.ui.creditPoints') }}</dt><dd>{{ execution?.credit_points ?? plan?.credit_points ?? '—' }}</dd><dt>{{ t('admin.commission.ui.debitPoints') }}</dt><dd>{{ execution?.debit_points ?? plan?.debit_points ?? '—' }}</dd></template><dt v-if="dialog.body.reason">{{ t('admin.copy.5ec7c8ba') }}</dt><dd v-if="dialog.body.reason">{{ dialog.body.reason }}</dd></dl><label class="cc-confirm"><input data-testid="cc-confirm-check" type="checkbox" :checked="confirmed" :disabled="busyRead" @change="confirmed=($event.target as HTMLInputElement).checked" />{{ t('admin.copy.41a04f7e') }}</label><div class="cc-actions ui-form-actions"><button v-write-guard="writing" class="button button-primary" data-testid="cc-submit" type="button" :disabled="writing || busyRead || !confirmed" @click="withGlobalLoading(submit, dialog)">{{ writing ? t('admin.copy.46511dc0') : unknownIntent ? t('admin.copy.59c708d8') : t('admin.copy.3135aeca') }}</button></div></DetailModal>
  </template>
 </article>
</template>
<style scoped>
.cc-review-card dl {display:grid;grid-template-columns:auto minmax(0,1fr);gap:6px 12px}.cc-review-card dd {margin:0;overflow-wrap:anywhere}.cc-confirm {display:flex;align-items:center;gap:8px;min-height:44px}.cc-actions {display:flex;gap:8px;flex-wrap:wrap}.cc-error,.cc-receipt {padding:12px;border:1px solid var(--line);border-radius:10px;overflow-wrap:anywhere}.cc-receipt {border-color:var(--success)}
</style>
