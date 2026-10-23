<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import FilterAssociationSelect from "@lottery/shared/FilterAssociationSelect.vue";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
const displayDate = (value: string | null | undefined) => formatDisplayDateTime(value, displayLocale.value);
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import type { LocalizedMessage } from "@lottery/shared";
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useAdminI18n } from "./i18n";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import {
  correctionPermissions, createCorrectionApi,
  type Correction, type CorrectionContext, type CorrectionBody, type CorrectionActionBody,
  type CorrectionMode, type DrawArrays,
} from "./correction-api";
import {
  canonicalCorrectionResult,
  CORRECTION_PAGE_SIZE,
  correctionCountsMatch,
  correctionReasonByteLength,
  correctionWriteFailure,
  createCorrectionRequestLane,
  formatCorrectionPoints,
  freezeCorrectionIntent,
  frozenCorrectionWrites,
  matchesCorrectionReceipt,
  matchesCorrectionRetryReceipt,
  parseCorrectionNumberCsv,
  sameCorrectionOperation,
  sameCorrectionResult,
  validCorrectionReason,
} from "./correction-state";

const props = defineProps<{ account: AdminAccount; brandId: string; initialPeriodId?: string }>();
const emit = defineEmits<{
  (event: "session-invalid"): void;
  (event: "open-settlement", periodId: string): void;
}>();
const { t, message: localized, display } = useAdminI18n();

const api = createCorrectionApi();
const rights = computed(() => correctionPermissions(props.account, props.brandId));

type Context = CorrectionContext;
type HistoryPage = Awaited<ReturnType<typeof api.history>>;
type TargetPage = Awaited<ReturnType<typeof api.targets>>;
type Result = DrawArrays;
type CorrectBody = CorrectionBody;
type RetryBody = CorrectionActionBody;
type WriteIntent = ReturnType<typeof freezeCorrectionIntent<CorrectBody | RetryBody>>;

const periodId = ref(props.initialPeriodId ?? "");
const context = ref<Context | null>(null);
const history = ref<HistoryPage | null>(null);
const selected = ref<Correction | null>(null);
const targetPage = ref<TargetPage | null>(null);
const historyOffset = ref(0);
const targetOffset = ref(0);
const resultInput = ref({ regular: "", special: "", digits: "" });
const reason = ref("");
const contextBusy = ref(false);
const historyBusy = ref(false);
const detailBusy = ref(false);
const targetsBusy = ref(false);
const writing = ref(false);
const error = ref<string | LocalizedMessage>("");
const notice = ref<string | LocalizedMessage>("");
const reviewMode = ref<"correct" | "retry" | "">("");
const reviewBody = ref<CorrectBody | RetryBody | null>(null);
const reviewExpectedMode = ref<CorrectionMode>(null);
const reviewDrawResultId = ref<string | null>(null);
const reviewResourceId = ref<string | null>(null);
const reviewConfirmed = ref(false);
const pendingWrite = ref<WriteIntent | null>(
  frozenCorrectionWrites.find(props.account.id, props.brandId, props.initialPeriodId ?? "") as WriteIntent | null,
);
const unknownWrite = ref(Boolean(pendingWrite.value));
let alive = true;
const contextLane = createCorrectionRequestLane();
const historyLane = createCorrectionRequestLane();
const detailLane = createCorrectionRequestLane();
const targetsLane = createCorrectionRequestLane();
const writeLane = createCorrectionRequestLane();

const scope = computed(() => JSON.stringify([
  props.account.id, props.account.super_admin, props.account.brand_ids, props.account.permissions,
  props.account.permissions_by_brand, props.account.platform_permissions, props.brandId, periodId.value, rights.value,
]));
const financial = computed(() => context.value?.current_job_id !== null && context.value?.current_job_id !== undefined);
const reasonBytes = computed(() => correctionReasonByteLength(reason.value));
const parsedResult = computed<Result | null>(() => {
  if (!context.value?.model) return null;
  const regular = parseCorrectionNumberCsv(resultInput.value.regular);
  const special = parseCorrectionNumberCsv(resultInput.value.special);
  const digits = parseCorrectionNumberCsv(resultInput.value.digits);
  if (!regular || !special || !digits) return null;
  return canonicalCorrectionResult({ regular, special, digits }, context.value.model.ordered);
});
const previousResult = computed(() => context.value?.draw ?? null);
const sameAsPrevious = computed(() => Boolean(parsedResult.value && previousResult.value && sameCorrectionResult(parsedResult.value, previousResult.value)));
const canCorrect = computed(() => Boolean(
  rights.value.view && rights.value.correct && !props.account.super_admin && context.value?.can_correct &&
  context.value.draw_result_id && parsedResult.value && !sameAsPrevious.value && validCorrectionReason(reason.value) &&
  (!financial.value || (rights.value.settleRun && context.value?.mode && context.value.policy_version > 0)) &&
  !pendingWrite.value && !writing.value && !contextBusy.value,
));
const canRetry = computed(() => Boolean(
  selected.value?.state === "failed" && selected.value.previous_job_id && selected.value.new_job_id === null &&
  selected.value.policy_version !== null &&
  selected.value.failed_count > 0 && selected.value.can_retry && rights.value.retry && rights.value.settleRun && !props.account.super_admin &&
  validCorrectionReason(reason.value) && !pendingWrite.value && !writing.value,
));
const canStartRetry = computed(() => Boolean(selected.value?.can_retry && canRetry.value));

function isCurrent(lane: ReturnType<typeof createCorrectionRequestLane>, ticket: number, capturedScope: string, permitted = rights.value.view) {
  return alive && lane.isCurrent(ticket) && scope.value === capturedScope && permitted;
}
function invalidateReads() {
  contextLane.invalidate(); historyLane.invalidate(); detailLane.invalidate(); targetsLane.invalidate();
  contextBusy.value = false; historyBusy.value = false; detailBusy.value = false; targetsBusy.value = false;
}
function clearReadData() {
  invalidateReads();
  context.value = null; history.value = null; selected.value = null; targetPage.value = null;
  historyOffset.value = 0; targetOffset.value = 0;
}
function cancelReview() {
  reviewMode.value = ""; reviewBody.value = null; reviewConfirmed.value = false;
  reviewExpectedMode.value = null; reviewDrawResultId.value = null; reviewResourceId.value = null;
}
function clearScopeData() {
  clearReadData(); writeLane.invalidate(); error.value = ""; notice.value = "";
  resultInput.value = { regular: "", special: "", digits: "" }; reason.value = ""; cancelReview();
  writing.value = false;
  pendingWrite.value = frozenCorrectionWrites.find(props.account.id, props.brandId, periodId.value) as WriteIntent | null;
  unknownWrite.value = Boolean(pendingWrite.value);
}
function invalidateSession() {
  clearScopeData(); frozenCorrectionWrites.clear(); pendingWrite.value = null; unknownWrite.value = false; emit("session-invalid");
}
function handleError(problem: unknown, operation: "read" | "write" = "write") {
  if (problem instanceof AdminApiError && problem.status === 401) { invalidateSession(); return; }
  error.value = problem instanceof Error ? publicErrorMessage(problem, operation) : localized('admin.copy.4fb9cc4e');
}
function checkContext(value: Context) {
  if (value.brand_id !== props.brandId || value.period_id !== periodId.value)
    throw new Error(t('admin.copy.29e03173'));
}
function checkCorrection(value: Correction) {
  if (value.brand_id !== props.brandId || value.period_id !== periodId.value)
    throw new Error(t('admin.copy.4e2c3a8e'));
  if (!correctionCountsMatch(value)) throw new Error(t('admin.copy.8e8a006c'));
}
function clearTargets() { targetsLane.invalidate(); targetsBusy.value = false; targetPage.value = null; targetOffset.value = 0; }

async function readContext() {
  if (!rights.value.view || !periodId.value || writing.value) return false;
  const ticket = contextLane.begin(); const capturedScope = scope.value; contextBusy.value = true; error.value = "";
  try {
    const result = await api.context(props.brandId, periodId.value);
    if (!isCurrent(contextLane, ticket, capturedScope)) return false;
    checkContext(result); context.value = result;
    if (!pendingWrite.value) {
      resultInput.value = {
        regular: result.draw?.regular.join(", ") ?? "",
        special: result.draw?.special.join(", ") ?? "",
        digits: result.draw?.digits.join(", ") ?? "",
      };
    }
    if (!result.can_correct) notice.value = localized('admin.copy.4e02872f');
    else if (result.current_job_id && (!result.mode || result.policy_version < 1)) notice.value = localized('admin.copy.43a77556');
    else notice.value = localized('admin.copy.fc163500');
    return true;
  } catch (problem) {
    if (problem instanceof AdminApiError && problem.status === 401) { invalidateSession(); return false; }
    if (isCurrent(contextLane, ticket, capturedScope)) { context.value = null; handleError(problem, "read"); }
    return false;
  } finally { if (isCurrent(contextLane, ticket, capturedScope, true)) contextBusy.value = false; }
}
async function readHistory(offset = historyOffset.value, capturedScope = scope.value, allowDuringWrite = false) {
  if (!rights.value.view || !periodId.value || (writing.value && !allowDuringWrite)) return false;
  const ticket = historyLane.begin(); historyBusy.value = true;
  try {
    const page = await api.history(props.brandId, periodId.value, CORRECTION_PAGE_SIZE, offset);
    if (!isCurrent(historyLane, ticket, capturedScope) || page.period_id !== periodId.value || page.brand_id !== props.brandId) return false;
    history.value = page; historyOffset.value = page.offset;
    if (selected.value && !page.items.some((item: Correction) => item.id === selected.value?.id)) {
      selected.value = null; clearTargets();
    }
    if (!selected.value && page.items[0]) void readCorrection(page.items[0].id, capturedScope, allowDuringWrite);
    return true;
  } catch (problem) {
    if (problem instanceof AdminApiError && problem.status === 401) { invalidateSession(); return false; }
    if (isCurrent(historyLane, ticket, capturedScope)) { history.value = null; handleError(problem, "read"); }
    return false;
  } finally { if (isCurrent(historyLane, ticket, capturedScope, true)) historyBusy.value = false; }
}
async function readCorrection(id: string, capturedScope = scope.value, allowDuringWrite = false) {
  if (!rights.value.view || (writing.value && !allowDuringWrite)) return false;
  const ticket = detailLane.begin(); detailBusy.value = true;
  try {
    const value = await api.detail(props.brandId, id);
    if (!isCurrent(detailLane, ticket, capturedScope)) return false;
    checkCorrection(value); selected.value = value; clearTargets();
    void readTargets(id, 0, capturedScope, allowDuringWrite);
    return true;
  } catch (problem) {
    if (problem instanceof AdminApiError && problem.status === 401) { invalidateSession(); return false; }
    if (isCurrent(detailLane, ticket, capturedScope)) { selected.value = null; clearTargets(); handleError(problem, "read"); }
    return false;
  } finally { if (isCurrent(detailLane, ticket, capturedScope, true)) detailBusy.value = false; }
}
async function readTargets(id = selected.value?.id, offset = targetOffset.value, capturedScope = scope.value, allowDuringWrite = false) {
  if (!rights.value.view || !id || (writing.value && !allowDuringWrite)) return false;
  const ticket = targetsLane.begin(); targetsBusy.value = true;
  try {
    const page = await api.targets(props.brandId, id, CORRECTION_PAGE_SIZE, offset);
    if (!isCurrent(targetsLane, ticket, capturedScope) || selected.value?.id !== id) return false;
    if (page.brand_id !== props.brandId || page.correction_id !== id) throw new Error(t('admin.copy.dd44f587'));
    targetPage.value = page; targetOffset.value = page.offset; return true;
  } catch (problem) {
    if (problem instanceof AdminApiError && problem.status === 401) { invalidateSession(); return false; }
    if (isCurrent(targetsLane, ticket, capturedScope)) { targetPage.value = null; handleError(problem, "read"); }
    return false;
  } finally { if (isCurrent(targetsLane, ticket, capturedScope, true)) targetsBusy.value = false; }
}
async function refreshAll() {
  if (writing.value || !rights.value.view) return;
  if (!pendingWrite.value) cancelReview();
  const ok = await readContext();
  if (!ok) return;
  await readHistory(0);
  if (selected.value) await readCorrection(selected.value.id);
  if (pendingWrite.value) notice.value = localized('admin.copy.d3750e0e');
}

function reviewCorrect() {
  if (!canCorrect.value || !context.value || !parsedResult.value || !context.value.draw_result_id) return;
  reviewBody.value = {
    version: context.value.period_version,
    policy_version: financial.value ? context.value.policy_version : null,
    result: canonicalCorrectionResult(parsedResult.value, context.value.model.ordered),
    reason: reason.value.trim(),
  };
  reviewExpectedMode.value = financial.value ? context.value.mode : null;
  reviewDrawResultId.value = context.value.draw_result_id;
  reviewMode.value = "correct"; reviewConfirmed.value = false; error.value = "";
}
function reviewRetry() {
  if (!canStartRetry.value || !selected.value) return;
  reviewBody.value = { version: selected.value.version, reason: reason.value.trim() };
  reviewResourceId.value = selected.value.id;
  reviewDrawResultId.value = selected.value.draw_result_id;
  reviewExpectedMode.value = selected.value.mode;
  reviewMode.value = "retry"; reviewConfirmed.value = false; error.value = "";
}
async function beginWrite() {
  const mode = reviewMode.value; const body = reviewBody.value;
  if (!mode || !body || !reviewConfirmed.value || pendingWrite.value || writing.value || props.account.super_admin) return;
  if (mode === "correct" && !canCorrect.value) return;
  if (mode === "retry" && !canStartRetry.value) return;
  const intent = freezeCorrectionIntent({
    accountId: props.account.id, brandId: props.brandId, periodId: periodId.value,
    operation: mode, resourceId: mode === "retry" ? reviewResourceId.value : null,
    ...(reviewDrawResultId.value ? {
      expectedDrawResultId: reviewDrawResultId.value, expectedMode: reviewExpectedMode.value,
    } : {}),
    body, key: createIdempotencyKey(),
  }) as WriteIntent;
  pendingWrite.value = intent; frozenCorrectionWrites.remember(intent); unknownWrite.value = false;
  await submitPending();
}
function validateReceipt(value: Correction, intent: WriteIntent) {
  checkCorrection(value);
  if (intent.operation === "correct") {
    const body = intent.body as CorrectBody;
    if (!intent.expectedDrawResultId || intent.expectedMode === undefined || !matchesCorrectionReceipt(value, {
      brandId: intent.brandId, periodId: intent.periodId, previousDrawResultId: intent.expectedDrawResultId,
      periodVersion: body.version, policyVersion: body.policy_version, mode: intent.expectedMode,
      accountId: intent.accountId, reason: body.reason, result: body.result, financial: body.policy_version !== null,
    })) throw new Error(t('admin.copy.cb9227b9'));
  } else {
    const body = intent.body as RetryBody;
    if (!intent.resourceId || !matchesCorrectionRetryReceipt(value, { id: intent.resourceId, version: body.version }))
      throw new Error(t('admin.copy.60489250'));
  }
}
async function submitPending() {
  const intent = pendingWrite.value;
  if (!intent || writing.value || (intent.operation === "correct" ? !rights.value.correct : !rights.value.retry) || props.account.super_admin ||
    !sameCorrectionOperation(intent, {
      accountId: props.account.id, brandId: props.brandId, periodId: periodId.value,
      operation: intent.operation, resourceId: intent.resourceId,
    })) return;
  if ((intent.operation === "retry" || (intent.body as CorrectBody).policy_version != null) && !rights.value.settleRun) return;
  if (intent.operation === "correct" && (!intent.expectedDrawResultId || intent.expectedMode === undefined)) return;
  writing.value = true; error.value = "";
  notice.value = unknownWrite.value ? localized('admin.copy.9bf6f293') : localized('admin.copy.5e070757');
  invalidateReads(); const ticket = writeLane.begin(); const capturedScope = scope.value;
  let receipt: Correction;
  try {
    receipt = intent.operation === "correct"
      ? await api.create(intent.brandId, intent.expectedDrawResultId!, intent.body as CorrectBody, intent.key, {
        periodId: intent.periodId, accountId: intent.accountId, mode: intent.expectedMode ?? null,
      })
      : await api.retry(intent.brandId, intent.resourceId!, intent.body as RetryBody, intent.key);
  } catch (problem) {
    if (problem instanceof AdminApiError && problem.status === 401) { invalidateSession(); return; }
    if (!isCurrent(writeLane, ticket, capturedScope, true)) return;
    const status = problem instanceof AdminApiError ? problem.status : undefined;
    const failure = correctionWriteFailure(status);
    if (failure === "unknown") {
      frozenCorrectionWrites.remember(intent); pendingWrite.value = intent; unknownWrite.value = true;
      notice.value = localized('admin.copy.80f12762');
    } else {
      frozenCorrectionWrites.forget(intent); pendingWrite.value = null; unknownWrite.value = false; cancelReview();
      notice.value = failure === "conflict" ? localized('admin.copy.650c3c5d') : localized('admin.copy.04bc6fd5');
    }
    handleError(problem); writing.value = false; return;
  }
  if (!isCurrent(writeLane, ticket, capturedScope, true)) return;
  try { validateReceipt(receipt, intent); }
  catch (problem) {
    frozenCorrectionWrites.remember(intent); pendingWrite.value = intent; unknownWrite.value = true;
    notice.value = localized('admin.copy.85a03112');
    handleError(problem); writing.value = false; return;
  }

  // A validated receipt acknowledges the write before follow-up GETs. A GET failure never re-freezes it.
  frozenCorrectionWrites.forget(intent); pendingWrite.value = null; unknownWrite.value = false; cancelReview();
  notice.value = localized('admin.copy.3b592217');
  const correctionId = intent.operation === "correct" ? receipt.id : intent.resourceId!;
  try {
    const detailOk = await readCorrection(correctionId, capturedScope, true);
    if (!isCurrent(writeLane, ticket, capturedScope, true)) return;
    const historyOk = await readHistory(0, capturedScope, true);
    if (!isCurrent(writeLane, ticket, capturedScope, true)) return;
    if (!detailOk || !historyOk) notice.value = localized('admin.copy.6d5599f8');
    else notice.value = localized('admin.copy.ed38bfa7');
  } finally { if (isCurrent(writeLane, ticket, capturedScope, true)) writing.value = false; }
}

function stateLabel(state: string) {
  const labels: Record<string, string> = { reversing: t('admin.copy.6967c4cf'), resettling: t('admin.copy.937f6429'), failed: t('admin.copy.25c4fdb7'), completed: t('admin.copy.c9cf4e4d') };
  return labels[state] ?? state;
}
function targetStateLabel(state: string) {
  const labels: Record<string, string> = { pending: t('admin.copy.99f247fd'), reversed: t('admin.copy.8d77a2f1'), unchanged: t('admin.copy.025bb177'), excluded: t('admin.copy.c43717fb'), failed: t('admin.copy.1feabec0') };
  return labels[state] ?? state;
}
function settlementModeLabel(mode: CorrectionMode) {
  return mode === "automatic" ? t('admin.copy.4b7603bc') : mode === "manual" ? t('admin.copy.81397eb4') : t('admin.copy.c18d7481');
}
function periodStatusLabel(state: string) {
  const labels: Record<string, string> = { pending: t('admin.copy.997b3d22'), betting: t('admin.copy.3a20b720'), closed: t('admin.copy.3198ad9b'), waiting_draw: t('admin.copy.aab1777a'), drawn: t('admin.copy.a16e8013'), settling: t('admin.copy.01f90196'), settled: t('admin.copy.037164c2'), bet_cancelled: t('admin.copy.a1834cc2'), judged_cancelled: t('admin.copy.46a60c2c') };
  return labels[state] ?? state;
}
function orderStatusLabel(state: string) {
  const labels: Record<string, string> = { placed: t('admin.copy.aab1777a'), abnormal: t('admin.copy.7bcf4b54'), bet_cancelled: t('admin.copy.a1834cc2'), judged_cancelled: t('admin.copy.46a60c2c'), won: t('admin.copy.6de62356'), lost: t('admin.copy.e2f3b9bc') };
  return labels[state] ?? state;
}
function jobStateLabel(state: string | null) {
  const labels: Record<string, string> = { processing: t('admin.copy.395f62f9'), awaiting_approval: t('admin.copy.662f2855'), paying: t('admin.copy.4faa8659'), failed: t('admin.copy.1feabec0'), completed: t('admin.copy.a5aa4b5c') };
  return state ? labels[state] ?? state : "—";
}
function modelDescription(model: NonNullable<Context["model"]>) {
  return `${model.model}${model.ordered ? ` · ${t('admin.copy.6aef61c3')}` : ` · ${t('admin.copy.98f4ddb5')}`}`;
}
async function selectCorrection(id: string) { if (id !== selected.value?.id) { cancelReview(); selected.value = null; clearTargets(); await readCorrection(id); } }

watch(() => props.initialPeriodId, (value) => { if (value !== undefined && value !== periodId.value) periodId.value = value; });
watch(scope, () => { clearScopeData(); if (alive && rights.value.view) void refreshAll(); }, { flush: "sync" });
watch(reason, () => { if (!pendingWrite.value) cancelReview(); });
watch(resultInput, () => { if (!pendingWrite.value) cancelReview(); }, { deep: true });
onMounted(() => { if (rights.value.view) void refreshAll(); });
onBeforeUnmount(() => { alive = false; invalidateReads(); writeLane.invalidate(); });
</script>

<template>
  <section class="correction-management" aria-labelledby="correction-management-title">
    <header class="cm-heading">
      <div><h2 id="correction-management-title">{{ t('admin.copy.fc54e9ea') }}</h2></div>
      <button v-refresh-button="t('admin.copy.a3439760')" v-if="rights.view" class="cm-secondary button button-secondary" type="button" :disabled="contextBusy || historyBusy || writing" @click="withGlobalLoading(refreshAll)">{{ t('admin.copy.a3439760') }}</button>
    </header>
    <p class="cm-warning" role="note"><strong>{{ t('admin.copy.4d163c15') }}</strong>{{ t('admin.copy.b2f62001') }}</p>
    <p v-if="!rights.view" class="cm-note">{{ t('admin.copy.6ab52e13') }}</p>
    <template v-else>
      <p class="muted">{{ t('admin.period.selectionGuide') }}</p>
      <details v-if="!initialPeriodId" open>
        <summary>{{ t('filter.selectPeriod') }}</summary>
      <div class="cm-period"><label for="correction-period-id">{{ t('admin.copy.c506752c') }}</label><FilterAssociationSelect v-model="periodId" :brand-id="brandId" surface="admin" resource="draw" kind="period" :label="t('filter.period')" :locale="displayLocale" :scope-key="JSON.stringify(account)" :disabled="writing || Boolean(pendingWrite)" /></div>
      </details>
      <p v-if="error" class="cm-message error" role="alert">{{ display(error) }}</p>
      <p v-if="notice" class="cm-message" role="status">{{ display(notice) }}</p>
      <div v-if="pendingWrite" class="cm-pending">
        <strong>{{ t('admin.copy.20bbd934') }} · {{ pendingWrite.operation === 'correct' ? t('admin.copy.a2850468') : t('admin.copy.56709e84') }}</strong>
        <p>{{ t('admin.copy.d1a0334e') }}</p>
        <p class="cm-break">{{ t('admin.copy.310f9b9a') }} {{ pendingWrite.periodId }} · {{ t('admin.copy.0336a9c7') }} {{ pendingWrite.expectedDrawResultId ?? '—' }} · {{ t('admin.copy.e682c364') }} {{ pendingWrite.expectedMode ?? t('admin.copy.6eee3edb') }}</p>
        <button v-write-guard="writing" class="cm-primary button button-primary" type="button" :disabled="writing || (pendingWrite.operation === 'correct' ? !rights.correct || ((pendingWrite.body as CorrectBody).policy_version !== null && !rights.settleRun) : !rights.retry || !rights.settleRun)" @click="withGlobalLoading(submitPending)">{{ writing ? t('admin.copy.4faf8610') : t('admin.copy.9e6100bd') }}</button>
      </div>

      <section v-if="context" class="cm-panel" aria-labelledby="cm-context-title">
        <div class="cm-section-title"><h3 id="cm-context-title">{{ t('admin.copy.05bd87cf') }}</h3><span class="cm-badge" :class="context.can_correct ? 'ready' : 'blocked'">{{ context.can_correct ? t('admin.copy.b9c85213') : t('admin.copy.38157e6a') }}</span></div>
        <p v-global-loading v-if="contextBusy" class="cm-note">{{ t('admin.copy.4de21371') }}</p>
        <dl class="cm-facts">
          <div><dt>{{ t('admin.copy.11f487e4') }}</dt><dd class="cm-break"><AssociationName :record="context" kind="brand" :id="context.brand_id" :locale="associationLocale" /> · <AssociationName :record="context" kind="game" :id="context.game_id" :locale="associationLocale" /> · <AssociationName :record="context" kind="period" :id="context.period_id" :locale="associationLocale" /></dd></div>
          <div><dt>{{ t('admin.copy.4ac4f8df') }}</dt><dd> {{ periodStatusLabel(context.period_status) }}</dd></div>
          <div><dt>{{ t('admin.copy.089bad6a') }}</dt><dd class="cm-break">{{ context.draw_result_id ?? '—' }}</dd></div>
          <div><dt>{{ t('admin.copy.570709e7') }}</dt><dd>{{ modelDescription(context.model) }}</dd></div>
          <div><dt>{{ t('admin.copy.0fc7375a') }}</dt><dd class="cm-break">{{ context.current_job_id ?? t('admin.copy.c2b41b33') }}</dd></div>
          <div><dt>{{ t('admin.copy.8c6d9d0a') }}</dt><dd> {{ settlementModeLabel(context.mode) }}</dd></div>
        </dl>
        <p v-if="context.current_job_id" class="cm-warning compact">{{ t('admin.copy.fd95657c') }}</p>
        <p v-else class="cm-note">{{ t('admin.copy.75c697b7') }}</p>
        <p v-if="rights.correct && !rights.settleRun && context.current_job_id" class="cm-note">{{ t('admin.copy.7f62f7e5') }}</p>
        <p v-if="props.account.super_admin" class="cm-note">{{ t('admin.copy.cfc13b33') }}</p>
      </section>

      <section v-if="context && rights.correct && !props.account.super_admin" class="cm-panel" aria-labelledby="cm-edit-title">
        <div class="cm-section-title"><h3 id="cm-edit-title">{{ t('admin.copy.fc9ac622') }}</h3><span class="cm-badge" :class="financial ? 'pending' : 'ready'">{{ financial ? t('admin.copy.dfb07aad') : t('admin.copy.3940538c') }}</span></div>
        <dl class="cm-facts cm-results">
          <div><dt>{{ t('admin.copy.e52beeae') }}</dt><dd class="cm-break">{{ t('admin.copy.9e0f0b07') }}{{ context.draw?.regular.join(', ') ?? '—' }}；{{ t('admin.copy.ab8809e4') }}{{ context.draw?.special.join(', ') ?? '—' }}；{{ t('admin.copy.7f03881e') }}{{ context.draw?.digits.join(', ') ?? '—' }}</dd></div>
          <div><dt>{{ t('admin.copy.0ecc7ccd') }}</dt><dd class="cm-break">{{ t('admin.copy.9e0f0b07') }}{{ parsedResult?.regular.join(', ') ?? t('admin.copy.37c876ca') }}；{{ t('admin.copy.ab8809e4') }}{{ parsedResult?.special.join(', ') ?? t('admin.copy.37c876ca') }}；{{ t('admin.copy.7f03881e') }}{{ parsedResult?.digits.join(', ') ?? t('admin.copy.37c876ca') }}</dd></div>
        </dl>
        <div class="cm-form">
          <label for="correction-regular">{{ t('admin.copy.a1f3c1fc') }}</label><input id="correction-regular" v-model="resultInput.regular" :aria-label="t('admin.copy.a1f3c1fc')" inputmode="numeric" autocomplete="off" :placeholder="t('admin.copy.f7aec45a')" :disabled="writing || Boolean(pendingWrite)" />
          <label for="correction-special">{{ t('admin.copy.caa54743') }}</label><input id="correction-special" v-model="resultInput.special" :aria-label="t('admin.copy.caa54743')" inputmode="numeric" autocomplete="off" :placeholder="t('admin.copy.7d133634')" :disabled="writing || Boolean(pendingWrite)" />
          <label for="correction-digits">{{ t('admin.copy.b9f04d9a') }}</label><input id="correction-digits" v-model="resultInput.digits" :aria-label="t('admin.copy.b9f04d9a')" inputmode="numeric" autocomplete="off" :placeholder="t('admin.copy.8571188a')" :disabled="writing || Boolean(pendingWrite)" />
          <p class="cm-note">{{ t('admin.copy.634c5bed') }}</p>

          <small v-if="reason">{{ reasonBytes }} / {{ t('admin.copy.c8116369') }}</small>
          <button class="cm-primary button button-primary" type="button" :disabled="!canCorrect" @click="reviewCorrect">{{ financial ? t('admin.copy.808714d5') : t('admin.copy.f19f4318') }}</button>
          <p v-if="sameAsPrevious" class="cm-note">{{ t('admin.copy.e8d91f39') }}</p>
        </div>
      </section>

      <section v-if="rights.view && periodId" class="cm-panel" aria-labelledby="cm-history-title">
        <div class="cm-section-title"><h3 id="cm-history-title">{{ t('admin.copy.29852268') }}</h3><span>{{ t('admin.copy.5cb7f04d') }} {{ CORRECTION_PAGE_SIZE }}</span></div>
        <p v-global-loading v-if="historyBusy" class="cm-note">{{ t('admin.copy.362f810a') }}</p>
        <div v-if="history?.items.length" class="cm-history-list">
          <button v-for="item in history.items" :key="item.id" class="cm-history-item" :class="{ active: selected?.id === item.id }" type="button" :disabled="writing || Boolean(pendingWrite)" @click="withGlobalLoading(selectCorrection, item.id)">
            <span><strong>{{ stateLabel(item.state) }}</strong><small>{{ displayDate(item.created_at) }}</small></span><span class="cm-break">{{ item.id }}</span><span> {{ item.target_count }} {{ t('admin.copy.23ac8df4') }}</span>
          </button>
        </div>
        <p v-else-if="history && !historyBusy" class="cm-note empty-state">{{ t('admin.copy.cdbaf4a2') }}</p>
        <div v-if="history" class="cm-pagination">
          <button class="cm-secondary button button-secondary" type="button" :disabled="historyOffset <= 0 || historyBusy || writing" @click="withGlobalLoading(readHistory, Math.max(0, historyOffset - CORRECTION_PAGE_SIZE))">{{ t('admin.copy.dbe7a5d7') }}</button>
          <span>{{ t('admin.copy.4ffb01e2') }} {{ history.offset }} · {{ history.items.length }} {{ t('admin.copy.067fed38') }}</span>
          <button class="cm-secondary button button-secondary" type="button" :disabled="!history.has_more || historyBusy || writing" @click="withGlobalLoading(readHistory, historyOffset + CORRECTION_PAGE_SIZE)">{{ t('admin.copy.2e9c9e7a') }}</button>
        </div>
      </section>

      <section v-if="selected" class="cm-panel" aria-labelledby="cm-detail-title">
        <div class="cm-section-title"><h3 id="cm-detail-title">{{ t('admin.copy.243a698d') }}</h3><span class="cm-badge" :class="selected.state === 'completed' ? 'ready' : selected.state === 'failed' ? 'blocked' : 'pending'">{{ stateLabel(selected.state) }}</span></div>
        <p v-global-loading v-if="detailBusy" class="cm-note">{{ t('admin.copy.9cc5a14b') }}</p>
        <dl class="cm-facts">
          <div><dt>{{ t('admin.copy.838e3a09') }}</dt><dd class="cm-break">{{ selected.id }}</dd></div>
          <div><dt>{{ t('admin.copy.fdc9ab8b') }}</dt><dd class="cm-break">{{ selected.previous_draw_result_id }} → {{ selected.draw_result_id }}</dd></div>
          <div><dt>{{ t('admin.copy.f1b9111b') }}</dt><dd class="cm-break">{{ t('admin.copy.9e0f0b07') }}{{ selected.result.regular.join(', ') }}；{{ t('admin.copy.ab8809e4') }}{{ selected.result.special.join(', ') }}；{{ t('admin.copy.7f03881e') }}{{ selected.result.digits.join(', ') }}</dd></div>
          <div><dt>{{ t('admin.copy.fa60586f') }}</dt><dd class="cm-break">{{ selected.previous_job_id ?? '—' }} → {{ selected.new_job_id ?? '—' }}</dd></div>
          <div><dt>{{ t('admin.copy.1d82d0ed') }}</dt><dd> {{ selected.mode === null ? t('admin.copy.6eee3edb') : settlementModeLabel(selected.mode) }}</dd></div>
          <div><dt>{{ t('admin.copy.f8e5c1cc') }}</dt><dd>{{ selected.target_count }} · {{ t('admin.copy.bc56e30b') }} {{ selected.pending_count }} / {{ t('admin.copy.8598a3e4') }} {{ selected.reversed_count }} / {{ t('admin.copy.b6ebf693') }} {{ selected.unchanged_count }} / {{ t('admin.copy.6a1919eb') }} {{ selected.excluded_count }} / {{ t('admin.copy.1feabec0') }} {{ selected.failed_count }}</dd></div>
          <div><dt>{{ t('admin.copy.fb40a1d6') }}</dt><dd>{{ formatCorrectionPoints(selected.reverse_points) }} / {{ formatCorrectionPoints(selected.reversed_points) }}</dd></div>
          <div><dt>{{ t('admin.copy.86543ed7') }}</dt><dd class="cm-break"><AssociationName :record="selected" kind="admin" :id="selected.created_by" :locale="associationLocale" /> {{ displayDate(selected.created_at) }} · {{ displayDate(selected.completed_at) }}</dd></div>
          <div><dt>{{ t('admin.copy.da9cc09c') }}</dt><dd class="cm-break">{{ selected.reason }} · {{ selected.last_error_code ? displayPublicError(publicErrorMessage({ code: selected.last_error_code })) : "—" }}</dd></div>
          <div><dt>{{ t('admin.copy.09d8990a') }}</dt><dd>{{ jobStateLabel(selected.new_job_state) }} · {{ selected.new_job_error_code ?? '—' }}</dd></div>
        </dl>
        <p v-if="selected.failed_count > 0" class="cm-warning compact">{{ t('admin.copy.a66c9a95') }}</p>
        <div v-if="canRetry" class="cm-form">

          <small v-if="reason">{{ reasonBytes }} / {{ t('admin.copy.c8116369') }}</small>
          <button class="cm-primary button button-primary" type="button" :disabled="!canStartRetry" @click="reviewRetry">{{ t('admin.copy.d6e10f74') }}</button>
        </div>
        <button v-if="selected.new_job_id" class="cm-secondary button button-secondary" type="button" @click="emit('open-settlement', selected.period_id)">{{ t('admin.copy.19b7dd12') }}</button>

        <div class="cm-target-heading"><h4>{{ t('admin.copy.15fdd724') }}</h4><span>{{ t('admin.copy.5cb7f04d') }} {{ CORRECTION_PAGE_SIZE }} {{ t('admin.copy.067fed38') }}</span></div>
        <p v-global-loading v-if="targetsBusy" class="cm-note">{{ t('admin.copy.a4a7f5f1') }}</p>
        <div v-if="targetPage?.items.length" class="cm-targets">
          <article v-for="target in targetPage.items" :key="target.order_id" class="cm-target">
            <div class="cm-target-title"><strong>{{ targetStateLabel(target.state) }}</strong><span>{{ target.error_code ? displayPublicError(publicErrorMessage({ code: target.error_code })) : "—" }}</span></div>
            <dl class="cm-facts compact-facts">
              <div><dt>{{ t('admin.copy.8a34d31c') }}</dt><dd class="cm-break">{{ target.order_id }} / <AssociationName :record="target" kind="member" :id="target.member_id" :locale="associationLocale" /></dd></div>
              <div><dt>{{ t('admin.copy.0b25637f') }}</dt><dd> {{ orderStatusLabel(target.old_order_status) }}</dd></div>
              <div><dt>{{ t('admin.copy.b3781bda') }}</dt><dd class="cm-break">{{ target.old_calculation_id ?? '—' }} / {{ target.old_payout_entry_id ?? '—' }}</dd></div>
              <div><dt>{{ t('admin.copy.dd8e73d1') }}</dt><dd class="cm-break">{{ formatCorrectionPoints(target.old_prize_points) }} / {{ target.reversal_entry_id ?? '—' }}</dd></div>
            </dl>
          </article>
        </div>
        <p v-else-if="targetPage && !targetsBusy" class="cm-note empty-state">{{ t('admin.copy.9e8399db') }}</p>
        <div v-if="targetPage" class="cm-pagination">
          <button class="cm-secondary button button-secondary" type="button" :disabled="targetOffset <= 0 || targetsBusy || writing" @click="withGlobalLoading(readTargets, selected.id, Math.max(0, targetOffset - CORRECTION_PAGE_SIZE))">{{ t('admin.copy.dbe7a5d7') }}</button>
          <span>{{ t('admin.copy.4ffb01e2') }} {{ targetPage.offset }} · {{ targetPage.items.length }} {{ t('admin.copy.067fed38') }}</span>
          <button class="cm-secondary button button-secondary" type="button" :disabled="!targetPage.has_more || targetsBusy || writing" @click="withGlobalLoading(readTargets, selected.id, targetOffset + CORRECTION_PAGE_SIZE)">{{ t('admin.copy.2e9c9e7a') }}</button>
        </div>
      </section>

      <div v-if="reviewMode && reviewBody && !pendingWrite" class="cm-confirm">
        <strong>{{ t('admin.copy.1cbc1c30') }}</strong>
        <p v-if="reviewMode === 'correct'">{{ t('admin.copy.e682c364') }}  {{ reviewExpectedMode === null ? t('admin.copy.6eee3edb') : settlementModeLabel(reviewExpectedMode) }}{{ t('admin.copy.adb13b01') }} {{ (reviewBody as CorrectBody).result.regular.join(', ') }}{{ t('admin.copy.bc420c94') }} {{ (reviewBody as CorrectBody).result.special.join(', ') }}{{ t('admin.copy.5a7a878c') }} {{ (reviewBody as CorrectBody).result.digits.join(', ') }}。</p>
        <p v-else>{{ t('admin.copy.94372458') }} {{ selected?.id }}。</p>
        <p v-if="reviewBody.reason">{{ t('admin.copy.2809a1e6') }}{{ reviewBody.reason }}</p>
        <label for="correction-confirmed" class="cm-check"><input id="correction-confirmed" v-model="reviewConfirmed" type="checkbox" />{{ t('admin.copy.a42ef8dd') }}</label>
        <div class="cm-review-actions"><button class="cm-secondary button button-secondary" type="button" :disabled="writing" @click="cancelReview">{{ t('admin.copy.bf7400b6') }}</button><button v-write-guard="writing" class="cm-primary button button-primary" type="button" :disabled="!reviewConfirmed || writing || (reviewMode === 'correct' ? !rights.correct : !rights.retry || !rights.settleRun)" @click="withGlobalLoading(beginWrite)">{{ writing ? t('admin.copy.46511dc0') : t('admin.copy.a1a8013d') }}</button></div>
      </div>
    </template>
  </section>
</template>

<style scoped>
details > summary { min-height:44px; padding:10px 0; box-sizing:border-box; cursor:pointer; }
.correction-management{display:grid;gap:16px;color:var(--ink)}.cm-heading,.cm-section-title,.cm-target-heading,.cm-target-title,.cm-pagination,.cm-review-actions{display:flex;align-items:center;justify-content:space-between;gap:12px}.cm-heading h2{margin:2px 0 0;font-size:1.25rem}.cm-eyebrow{margin:0;color:#68758a;font-size:.78rem}.cm-warning,.cm-pending,.cm-confirm,.cm-panel{border:1px solid var(--line);border-radius:12px;padding:16px;background:var(--surface)}.cm-warning{background:#fff8e8;border-color:#e9c66a;color:#694d08}.cm-warning.compact{padding:10px;margin:8px 0}.cm-pending{background:#fff1ee;border-color:#df9385}.cm-panel{display:grid;gap:12px}.cm-section-title h3,.cm-target-heading h4{margin:0}.cm-target-heading{margin-top:8px}.cm-target-heading span,.cm-note,.cm-message,.cm-target-title span,.cm-pagination span,.cm-history-item small{color:var(--muted);font-size:.88rem}.cm-facts{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px 18px;margin:0}.cm-facts>div{min-width:0;padding:8px 0;border-bottom:1px solid #eef1f5}.cm-facts dt{color:#697586;font-size:.78rem}.cm-facts dd{margin:4px 0 0;font-weight:550}.cm-break{overflow-wrap:anywhere;word-break:break-word}.cm-badge{display:inline-flex;border-radius:99px;padding:4px 10px;font-size:.8rem;background:#edf0f5}.cm-badge.ready{background:#e6f6eb;color:#176b37}.cm-badge.blocked{background:#fff0ee;color:#a32e1a}.cm-badge.pending{background:#fff5dd;color:#725708}.cm-form{display:grid;gap:8px;max-width:720px}.cm-form label,.cm-period label{display:grid;gap:6px;font-size:.9rem}.cm-form textarea,.cm-form input,.cm-period input{width:100%;box-sizing:border-box;border:1px solid var(--line);border-radius:8px;padding:9px 10px;font:inherit;background:white}.cm-form small{color:var(--muted)}.cm-primary,.cm-secondary{border:1px solid #c5ceda;border-radius:8px;padding:8px 12px;font:inherit;cursor:pointer;background:var(--surface)}.cm-primary{background:var(--primary);border-color:var(--primary);color:var(--surface)}.cm-primary:disabled,.cm-secondary:disabled{opacity:.52;cursor:not-allowed}.cm-message{margin:0;padding:10px 12px;border-radius:8px;background:#f0f4fa}.cm-message.error{background:#fff0ee;color:#a32e1a}.cm-note{margin:0}.cm-period{display:grid;gap:6px;max-width:720px}.cm-history-list,.cm-targets{display:grid;gap:10px}.cm-history-item{display:grid;grid-template-columns:1fr minmax(120px,1fr) auto;align-items:center;gap:12px;text-align:left;border:1px solid #e2e7ef;border-radius:10px;padding:12px;background:white;color:inherit;font:inherit;cursor:pointer}.cm-history-item span:first-child{display:grid;gap:4px}.cm-history-item.active{border-color:var(--primary);background:var(--primary-light)}.cm-target{border:1px solid #e2e7ef;border-radius:10px;padding:12px}.compact-facts{margin-top:8px}.compact-facts>div{padding:5px 0}.cm-check{display:flex;align-items:flex-start;gap:8px;margin:12px 0}.cm-review-actions{justify-content:flex-end}.cm-pagination{justify-content:center;flex-wrap:wrap}
@media(max-width:600px){.correction-management{gap:12px}.cm-heading{align-items:flex-start}.cm-warning,.cm-panel,.cm-confirm,.cm-pending{padding:12px}.cm-facts{grid-template-columns:minmax(0,1fr);gap:2px}.cm-section-title{align-items:flex-start}.cm-pagination .cm-secondary{flex:1}.cm-review-actions{align-items:stretch;flex-direction:column-reverse}.cm-review-actions button{width:100%}.cm-history-item{grid-template-columns:1fr;gap:6px}}
</style>
