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
import { vGlobalLoading } from "@lottery/shared";
const displayDate = (value: string | null | undefined) => formatDisplayDateTime(value, displayLocale.value);
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import type { LocalizedMessage } from "@lottery/shared";
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useAdminI18n } from "./i18n";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { createSettlementJobApi, settlementJobPermissions } from "./settlement-job-api";
import {
  createSettlementJobRequestLane,
  formatSettlementPoints,
  freezeSettlementJobIntent,
  frozenSettlementJobWrites,
  isSettlementJobTerminal,
  matchesSettlementJobReceipt,
  sameSettlementJobOperation,
  settlementJobWriteFailure,
  SETTLEMENT_JOB_PAGE_SIZE,
  validSettlementReason,
} from "./settlement-job-state";

const props = defineProps<{
  account: AdminAccount;
  brandId: string;
  initialPeriodId?: string;
}>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, message: localized, display } = useAdminI18n();

const api = createSettlementJobApi();
const rights = computed(() => settlementJobPermissions(props.account, props.brandId));
type Policy = Awaited<ReturnType<typeof api.policy>>;
type Context = Awaited<ReturnType<typeof api.context>>;
type Job = NonNullable<Awaited<ReturnType<typeof api.periodJob>>["settlement"]>;
type TargetPage = Awaited<ReturnType<typeof api.targets>>;
type PolicyBody = { version: number; mode: "automatic" | "manual" | null; reason: string };
type StartBody = { version: number; policy_version: number; draw_result_id: string; reason: string };
type JobBody = { version: number; reason: string };
type WriteIntent = ReturnType<typeof freezeSettlementJobIntent<PolicyBody | StartBody | JobBody>>;

const periodId = ref(props.initialPeriodId ?? "");
const policy = ref<Policy | null>(null);
const context = ref<Context | null>(null);
const job = ref<Job | null>(null);
const targetPage = ref<TargetPage | null>(null);
const targetOffset = ref(0);
const policyBusy = ref(false);
const contextBusy = ref(false);
const jobBusy = ref(false);
const targetsBusy = ref(false);
const writing = ref(false);
const error = ref<string | LocalizedMessage>("");
const notice = ref<string | LocalizedMessage>("");
const reason = ref("");
const policyMode = ref<"automatic" | "manual" | "">("");
const reviewMode = ref<"policy" | "start" | "approve" | "retry" | "">("");
const reviewBody = ref<PolicyBody | StartBody | JobBody | null>(null);
const reviewConfirmed = ref(false);
const pendingWrite = ref<WriteIntent | null>(
  frozenSettlementJobWrites.find(props.account.id, props.brandId, props.initialPeriodId ?? "") as WriteIntent | null,
);
const unknownWrite = ref(Boolean(pendingWrite.value));
let alive = true;
const policyLane = createSettlementJobRequestLane();
const contextLane = createSettlementJobRequestLane();
const jobLane = createSettlementJobRequestLane();
const targetsLane = createSettlementJobRequestLane();
const writeLane = createSettlementJobRequestLane();

const scope = computed(() => JSON.stringify([
  props.account.id, props.account.super_admin, props.account.brand_ids,
  props.account.permissions, props.account.permissions_by_brand,
  props.account.platform_permissions, props.brandId, periodId.value, rights.value,
]));
const canWrite = computed(() => !props.account.super_admin);
const selectedJob = computed(() => job.value);
const reasonBytes = computed(() => new TextEncoder().encode(reason.value).length);
const policyConfigured = computed(() => policy.value?.mode === "automatic" || policy.value?.mode === "manual");
const canStart = computed(() => Boolean(
  rights.value.view && rights.value.run && canWrite.value && context.value?.can_start &&
  context.value.mode && context.value.mode === policy.value?.mode &&
  context.value.policy_version === policy.value?.version && context.value.draw_result_id &&
  !pendingWrite.value && !writing.value && !contextBusy.value,
));
const currentJobIsTerminal = computed(() => Boolean(job.value?.current && isSettlementJobTerminal(job.value)));

function isCurrent(lane: ReturnType<typeof createSettlementJobRequestLane>, ticket: number, capturedScope: string, permitted = rights.value.view) {
  return alive && lane.isCurrent(ticket) && scope.value === capturedScope && permitted;
}
function invalidateReads() {
  policyLane.invalidate();
  contextLane.invalidate();
  jobLane.invalidate();
  targetsLane.invalidate();
  policyBusy.value = false;
  contextBusy.value = false;
  jobBusy.value = false;
  targetsBusy.value = false;
}
function clearReadData() {
  invalidateReads();
  policy.value = null;
  context.value = null;
  job.value = null;
  targetPage.value = null;
  targetOffset.value = 0;
}
function clearScopeData() {
  clearReadData();
  writeLane.invalidate();
  error.value = "";
  notice.value = "";
  reason.value = "";
  policyMode.value = "";
  reviewMode.value = "";
  reviewBody.value = null;
  reviewConfirmed.value = false;
  writing.value = false;
  pendingWrite.value = frozenSettlementJobWrites.find(props.account.id, props.brandId, periodId.value) as WriteIntent | null;
  unknownWrite.value = Boolean(pendingWrite.value);
}
function invalidateSession() {
  clearScopeData();
  frozenSettlementJobWrites.clear();
  pendingWrite.value = null;
  emit("session-invalid");
}
function handleError(problem: unknown, operation: "read" | "write" = "write") {
  if (problem instanceof AdminApiError && problem.status === 401) {
    invalidateSession();
    return;
  }
  error.value = problem instanceof Error ? publicErrorMessage(problem, operation) : localized('admin.copy.4fb9cc4e');
}
function checkPolicy(value: Policy) {
  if (value.brand_id !== props.brandId) throw new Error("品牌结算配置与当前品牌不匹配。");
}
function checkContext(value: Context) {
  if (value.brand_id !== props.brandId || value.period_id !== periodId.value)
    throw new Error("结算上下文与当前品牌或期次不匹配，请重新读取。");
  if (policy.value && (value.policy_version !== policy.value.version || value.mode !== policy.value.mode))
    throw new Error("结算上下文中的配置版本与品牌当前配置不一致，请重新读取。");
}
function checkJob(value: Job) {
  if (value.brand_id !== props.brandId || value.period_id !== periodId.value)
    throw new Error("结算任务与当前品牌或期次不匹配。");
}
function clearTargets() {
  targetsLane.invalidate();
  targetsBusy.value = false;
  targetPage.value = null;
  targetOffset.value = 0;
}

async function readPolicy(allowDuringWrite = false): Promise<boolean> {
  if (!rights.value.policyView || !props.brandId || (writing.value && !allowDuringWrite)) return false;
  const ticket = policyLane.begin();
  const capturedScope = scope.value;
  policyBusy.value = true;
  try {
    const result = await api.policy(props.brandId);
    if (!isCurrent(policyLane, ticket, capturedScope, rights.value.policyView)) return false;
    checkPolicy(result);
    policy.value = result;
    policyMode.value = result.mode ?? "";
    return true;
  } catch (problem) {
    if (isCurrent(policyLane, ticket, capturedScope, rights.value.policyView)) handleError(problem, "read");
    return false;
  } finally {
    if (isCurrent(policyLane, ticket, capturedScope, rights.value.policyView)) policyBusy.value = false;
  }
}

async function readPeriodState(keepJob = true, allowDuringWrite = false): Promise<boolean> {
  if (!rights.value.view || !periodId.value || (writing.value && !allowDuringWrite)) return false;
  const capturedScope = scope.value;
  const contextTicket = contextLane.begin();
  const jobTicket = jobLane.begin();
  contextBusy.value = true;
  jobBusy.value = true;
  error.value = "";
  try {
    const [nextContext, periodResult] = await Promise.all([
      api.context(props.brandId, periodId.value),
      api.periodJob(props.brandId, periodId.value),
    ]);
    if (!isCurrent(contextLane, contextTicket, capturedScope) || !isCurrent(jobLane, jobTicket, capturedScope)) return false;
    checkContext(nextContext);
    const nextJob = periodResult.settlement;
    if (nextJob) checkJob(nextJob);
    context.value = nextContext;
    if (keepJob || nextJob?.id !== job.value?.id) {
      job.value = nextJob;
      clearTargets();
      if (nextJob) void readJob(nextJob.id, capturedScope, allowDuringWrite);
    }
    if (nextContext.mode === null || nextContext.policy_version === 0)
      notice.value = localized('admin.copy.e9f5593b');
    else if (!nextContext.can_start)
      notice.value = localized('admin.copy.e0ce5326');
    else notice.value = localized('admin.copy.0dacc403');
    return true;
  } catch (problem) {
    if (isCurrent(contextLane, contextTicket, capturedScope) && isCurrent(jobLane, jobTicket, capturedScope)) {
      context.value = null;
      job.value = null;
      clearTargets();
      handleError(problem, "read");
    }
    return false;
  } finally {
    if (isCurrent(contextLane, contextTicket, capturedScope, true)) contextBusy.value = false;
    if (isCurrent(jobLane, jobTicket, capturedScope, true)) jobBusy.value = false;
  }
}

async function readJob(id: string, capturedScope = scope.value, allowDuringWrite = false): Promise<boolean> {
  if (!rights.value.view || (writing.value && !allowDuringWrite)) return false;
  const ticket = jobLane.begin();
  jobBusy.value = true;
  try {
    const result = await api.job(props.brandId, id);
    if (!isCurrent(jobLane, ticket, capturedScope)) return false;
    checkJob(result);
    // This is a fresh server read; worker progress outranks the earlier write receipt.
    job.value = result;
    if (job.value.id === id) void readTargets(id, 0, capturedScope, allowDuringWrite);
    return true;
  } catch (problem) {
    if (isCurrent(jobLane, ticket, capturedScope)) handleError(problem, "read");
    return false;
  } finally {
    if (isCurrent(jobLane, ticket, capturedScope, true)) jobBusy.value = false;
  }
}

async function readTargets(id = job.value?.id, offset = targetOffset.value, capturedScope = scope.value, allowDuringWrite = false): Promise<boolean> {
  if (!rights.value.view || !id || (writing.value && !allowDuringWrite)) return false;
  const ticket = targetsLane.begin();
  targetsBusy.value = true;
  try {
    const result = await api.targets(props.brandId, id, SETTLEMENT_JOB_PAGE_SIZE, offset);
    if (!isCurrent(targetsLane, ticket, capturedScope) || job.value?.id !== id) return false;
    if (result.brand_id !== props.brandId || result.job_id !== id)
      throw new Error("结算目标列表与当前品牌或任务不匹配。");
    targetPage.value = result;
    targetOffset.value = result.offset;
    return true;
  } catch (problem) {
    if (isCurrent(targetsLane, ticket, capturedScope)) {
      targetPage.value = null;
      handleError(problem, "read");
    }
    return false;
  } finally {
    if (isCurrent(targetsLane, ticket, capturedScope, true)) targetsBusy.value = false;
  }
}

async function refreshAll() {
  if (writing.value) return;
  if (rights.value.policyView && !(await readPolicy())) return;
  if (periodId.value) await readPeriodState();
  if (pendingWrite.value) notice.value = localized('admin.copy.44e0e121');
}

function review(operation: "policy" | "start" | "approve" | "retry") {
  if (pendingWrite.value || writing.value || !validSettlementReason(reason.value)) return;
  const cleanReason = reason.value.trim();
  if (operation === "policy") {
    if (!rights.value.policyWrite || !canWrite.value || !policy.value) return;
    reviewBody.value = { version: policy.value.version, mode: policyMode.value || null, reason: cleanReason };
  } else if (operation === "start") {
    if (!canStart.value || !context.value || !policy.value || !context.value.draw_result_id) {
      error.value = localized('admin.copy.f898703a');
      return;
    }
    reviewBody.value = {
      version: context.value.period_version,
      policy_version: context.value.policy_version,
      draw_result_id: context.value.draw_result_id,
      reason: cleanReason,
    };
  } else {
    if (!job.value || job.value.version < 1) return;
    if (operation === "approve" && !(rights.value.approve && canWrite.value && job.value.current && job.value.state === "awaiting_approval")) return;
    if (operation === "retry" && !(rights.value.retry && canWrite.value && job.value.state === "failed" && job.value.can_retry)) return;
    reviewBody.value = { version: job.value.version, reason: cleanReason };
  }
  reviewMode.value = operation;
  reviewConfirmed.value = false;
  error.value = "";
}

async function beginWrite() {
  const operation = reviewMode.value;
  const body = reviewBody.value;
  if (!operation || !body || pendingWrite.value || !(operation === "policy" ? rights.value.policyView : rights.value.view) || !canWrite.value) return;
  const intent = freezeSettlementJobIntent({
    accountId: props.account.id,
    brandId: props.brandId,
    periodId: periodId.value,
      operation,
      resourceId: operation === "approve" || operation === "retry" ? job.value?.id ?? null : null,
      ...(operation === "start" && context.value?.mode ? { expectedMode: context.value.mode } : {}),
      body,
    key: createIdempotencyKey(),
  }) as WriteIntent;
  pendingWrite.value = intent;
  frozenSettlementJobWrites.remember(intent);
  unknownWrite.value = false;
  await submitPending();
}

type PolicyReceipt = Policy & { audit_log_id: string };
function checkPolicyReceipt(result: PolicyReceipt, intent: WriteIntent) {
  checkPolicy(result);
  const body = intent.body as PolicyBody;
  if (!result.audit_log_id || result.version <= body.version || result.mode !== body.mode)
    throw new Error(t('admin.copy.cf3cf2c6'));
}
function checkStartReceipt(result: Job, intent: WriteIntent) {
  const body = intent.body as StartBody;
  checkJob(result);
  const expectedMode = intent.expectedMode;
  if (!expectedMode || !matchesSettlementJobReceipt(result, {
    brandId: intent.brandId, periodId: intent.periodId,
    periodVersion: body.version + 1, policyVersion: body.policy_version,
    drawResultId: body.draw_result_id, mode: expectedMode,
    reason: body.reason, accountId: intent.accountId,
  })) throw new Error(t('admin.copy.2856351d'));
}
function checkJobMutationReceipt(result: Job, intent: WriteIntent) {
  checkJob(result);
  if (result.id !== intent.resourceId || result.version <= (intent.body as JobBody).version)
    throw new Error(t('admin.copy.76be3992'));
  if (intent.operation === "approve" && result.approved_by !== intent.accountId)
    throw new Error(t('admin.copy.0b233374'));
}

async function submitPending() {
  const intent = pendingWrite.value;
  if (!intent || writing.value || !(intent.operation === "policy" ? rights.value.policyView : rights.value.view) || !canWrite.value) return;
  if (!sameSettlementJobOperation(intent, {
    accountId: props.account.id, brandId: props.brandId, periodId: periodId.value,
    operation: intent.operation, resourceId: intent.resourceId,
  })) return;
  const allowed = intent.operation === "policy" ? rights.value.policyWrite
    : intent.operation === "start" ? rights.value.run
      : intent.operation === "approve" ? rights.value.approve : rights.value.retry;
  if (!allowed) return;
  writing.value = true;
  error.value = "";
  notice.value = unknownWrite.value ? localized('admin.copy.9bf6f293') : localized('admin.copy.ea558cd7');
  invalidateReads();
  const ticket = writeLane.begin();
  const capturedScope = scope.value;
  let receipt: unknown;
  try {
    switch (intent.operation) {
      case "policy": receipt = await api.savePolicy(intent.brandId, intent.body as PolicyBody, intent.key); break;
      case "start": receipt = await api.start(intent.brandId, intent.periodId, intent.body as StartBody, intent.key); break;
      case "approve": receipt = await api.approve(intent.brandId, intent.resourceId!, intent.body as JobBody, intent.key); break;
      case "retry": receipt = await api.retry(intent.brandId, intent.resourceId!, intent.body as JobBody, intent.key); break;
    }
  } catch (problem) {
    if (!isCurrent(writeLane, ticket, capturedScope, true)) return;
    const status = problem instanceof AdminApiError ? problem.status : undefined;
    const failure = settlementJobWriteFailure(status);
    if (status === 401) {
      frozenSettlementJobWrites.forget(intent);
      invalidateSession();
      return;
    }
    if (failure === "unknown") {
      frozenSettlementJobWrites.remember(intent);
      pendingWrite.value = intent;
      unknownWrite.value = true;
      notice.value = localized('admin.copy.8910f331');
    } else {
      frozenSettlementJobWrites.forget(intent);
      pendingWrite.value = null;
      unknownWrite.value = false;
      reviewBody.value = null;
      reviewMode.value = "";
      notice.value = failure === "conflict"
        ? localized('admin.copy.4894b28b')
        : localized('admin.copy.f23858b8');
    }
    handleError(problem);
    writing.value = false;
    return;
  }
  if (!isCurrent(writeLane, ticket, capturedScope, true)) return;

  try {
    if (intent.operation === "policy") checkPolicyReceipt(receipt as PolicyReceipt, intent);
    else if (intent.operation === "start") checkStartReceipt(receipt as Job, intent);
    else checkJobMutationReceipt(receipt as Job, intent);
  } catch (problem) {
    frozenSettlementJobWrites.remember(intent);
    pendingWrite.value = intent;
    unknownWrite.value = true;
      notice.value = localized('admin.copy.df33af6a');
    handleError(problem);
    writing.value = false;
    return;
  }

  // A qualified write receipt confirms success before any follow-up reads.
  frozenSettlementJobWrites.forget(intent);
  pendingWrite.value = null;
  unknownWrite.value = false;
  reviewMode.value = "";
  reviewBody.value = null;
  reviewConfirmed.value = false;
  try {
    if (intent.operation === "policy") {
      const confirmed = receipt as PolicyReceipt;
      policy.value = confirmed;
      policyMode.value = confirmed.mode ?? "";
      // Updating policy never starts or retries an existing period job.
      const policyReadOk = await readPolicy(true);
      if (!policyReadOk) throw new Error(t('admin.copy.2fa99dcc'));
      const readOk = periodId.value ? await readPeriodState(true, true) : true;
      if (!readOk) throw new Error(t('admin.copy.352daf6d'));
    } else {
      const directJob = receipt as Job;
      job.value = directJob;
      clearTargets();
      const policyReadOk = rights.value.policyView ? await readPolicy(true) : true;
      if (!policyReadOk) throw new Error(t('admin.copy.ef054401'));
      const periodReadOk = await readPeriodState(true, true);
      if (!periodReadOk) throw new Error(t('admin.copy.5f2b6f28'));
      if (directJob.id && !(await readJob(directJob.id, capturedScope, true)))
        throw new Error(t('admin.copy.ebc62d07'));
    }
    if (isCurrent(writeLane, ticket, capturedScope, true)) {
      reason.value = "";
      notice.value = localized('admin.copy.92e3d61d');
    }
  } catch (problem) {
    if (isCurrent(writeLane, ticket, capturedScope, true)) {
      if (problem instanceof AdminApiError && problem.status === 401) {
        invalidateSession();
        return;
      }
      clearReadData();
      error.value = problem instanceof Error ? publicErrorMessage(problem, "read") : localized('admin.copy.06d5fbe1');
      notice.value = localized('admin.copy.00d8d7f9');
    }
  } finally {
    if (isCurrent(writeLane, ticket, capturedScope, true)) writing.value = false;
  }
}

function cancelReview() {
  if (pendingWrite.value || writing.value) return;
  reviewMode.value = "";
  reviewBody.value = null;
  reviewConfirmed.value = false;
}
function formatPoints(value: string | null | undefined) { return formatSettlementPoints(value); }
function jobStateLabel(state: Job["state"]) {
  return ({ processing: t('admin.copy.395f62f9'), awaiting_approval: t('admin.copy.662f2855'), paying: t('admin.copy.4faa8659'), failed: t('admin.copy.1feabec0'), completed: t('admin.copy.a5aa4b5c') })[state];
}
function targetStateLabel(state: string) {
  return ({ pending: t('admin.copy.99f247fd'), ready: t('admin.copy.175dd418'), paid: t('admin.copy.feca7e49'), excluded: t('admin.copy.c43717fb'), failed: t('admin.copy.1feabec0') } as Record<string, string>)[state] ?? state;
}
function outcomeText(won: boolean | null) { return won === null ? "—" : won ? t('admin.copy.bd8fef07') : t('admin.copy.35341bcb'); }
function settlementModeLabel(mode: string | null) {
  return mode === "automatic" ? t('admin.copy.4b7603bc') : mode === "manual" ? t('admin.copy.81397eb4') : t('admin.copy.c18d7481');
}
function periodStatusLabel(state: string) {
  const labels: Record<string, string> = { pending: t('admin.copy.997b3d22'), betting: t('admin.copy.3a20b720'), closed: t('admin.copy.3198ad9b'), waiting_draw: t('admin.copy.aab1777a'), drawn: t('admin.copy.a16e8013'), settling: t('admin.copy.01f90196'), settled: t('admin.copy.037164c2'), bet_cancelled: t('admin.copy.a1834cc2'), judged_cancelled: t('admin.copy.46a60c2c') };
  if (!labels[state]) throw new Error("Unsupported period status");
  return labels[state];
}
function orderStateLabel(state: string) {
  const labels: Record<string, string> = { placed: t('admin.copy.aab1777a'), abnormal: t('admin.copy.7bcf4b54'), bet_cancelled: t('admin.copy.a1834cc2'), judged_cancelled: t('admin.copy.46a60c2c'), won: t('admin.copy.6de62356'), lost: t('admin.copy.e2f3b9bc') };
  return labels[state] ?? state;
}

watch(() => props.initialPeriodId, (value) => {
  if (value !== undefined && value !== periodId.value) periodId.value = value;
});
watch(scope, () => {
  clearScopeData();
  if (alive && (rights.value.view || rights.value.policyView)) void refreshAll();
}, { flush: "sync" });
watch(() => [rights.value.view, rights.value.policyView, rights.value.policyWrite, rights.value.run, rights.value.approve, rights.value.retry] as const,
  ([view, policyView]) => { if (!view && !policyView) clearScopeData(); }, { flush: "sync" });
watch(reason, () => { if (!pendingWrite.value) cancelReview(); });
onMounted(() => {
  if (rights.value.view || rights.value.policyView) void refreshAll();
});
onBeforeUnmount(() => {
  alive = false;
  invalidateReads();
  writeLane.invalidate();
});
</script>

<template>
  <section class="settlement-management" aria-labelledby="settlement-management-title">
    <header class="sm-heading">
      <div><h2 id="settlement-management-title">{{ t('admin.copy.a215c7cc') }}</h2></div>
      <button v-refresh-button="t('admin.copy.deca021e')" v-if="rights.view || rights.policyView" class="sm-secondary button button-secondary" type="button" :disabled="policyBusy || contextBusy || jobBusy || writing" @click="withGlobalLoading(refreshAll)">{{ t('admin.copy.deca021e') }}</button>
    </header>
    <p class="sm-warning" role="note">{{ t('admin.copy.1a630990') }}</p>
    <p v-if="!rights.view && !rights.policyView" class="sm-note">{{ t('admin.copy.9dc3650d') }}</p>
    <template v-else>
      <p class="muted">{{ t('admin.period.selectionGuide') }}</p>
      <details :open="!initialPeriodId">
        <summary>{{ t('admin.period.lookupById') }}</summary>
      <div class="sm-period-select">
        <label>{{ t('admin.copy.3ba6d390') }}
          <input v-model.trim="periodId" type="text" :placeholder="t('admin.copy.52d4c62d')" :disabled="writing || Boolean(pendingWrite)" />
        </label>
      </div>
      </details>
      <p v-if="error" class="sm-message error" role="alert">{{ display(error) }}</p>
      <p v-if="notice" class="sm-message" role="status">{{ display(notice) }}</p>
      <div v-if="pendingWrite && (rights.view || rights.policyView)" class="sm-pending">
        <strong>{{ unknownWrite ? t('admin.copy.20bbd934') : t('admin.copy.3044c278') }} · {{ pendingWrite.operation === 'policy' ? t('admin.copy.99dfd35f') : pendingWrite.operation === 'start' ? t('admin.copy.e590db5b') : pendingWrite.operation === 'approve' ? t('admin.copy.0ee74403') : t('admin.copy.248d391e') }}</strong>
        <p>{{ t('admin.copy.d3ced462') }}</p>
        <button v-write-guard="writing" class="sm-primary button button-primary" type="button" :disabled="writing || !canWrite" @click="withGlobalLoading(submitPending)">{{ writing ? t('admin.copy.fb8562cf') : t('admin.copy.767252c3') }}</button>
      </div>

      <div v-if="rights.policyView" class="sm-panel">
        <div class="sm-section-title"><h3>{{ t('admin.copy.08715b59') }}</h3><span v-if="policy" class="sm-badge" :class="policyConfigured ? 'ready' : 'blocked'">{{ settlementModeLabel(policy.mode) }}</span></div>
        <p class="sm-note">{{ t('admin.copy.eba167ea') }}</p>
        <p v-global-loading v-if="policyBusy" class="sm-note">{{ t('admin.copy.efdc340c') }}</p>
        <dl v-if="policy" class="sm-facts">
          <div><dt>{{ t('admin.copy.9c32661d') }}</dt><dd class="sm-break"><AssociationName :record="policy" kind="brand" :id="policy.brand_id" :locale="associationLocale" /></dd></div>
          <div><dt>{{ t('admin.copy.ced345f7') }}</dt><dd>{{ settlementModeLabel(policy.mode) }}</dd></div>
          <div><dt>{{ t('admin.copy.f30af82e') }}</dt><dd> {{ displayDate(policy.updated_at) }}</dd></div>
        </dl>
        <p v-if="!rights.policyWrite" class="sm-note">{{ t('admin.copy.b04fdba8') }}</p>
        <div v-else-if="policy && canWrite" class="sm-form">
          <label>{{ t('admin.copy.09e2fa24') }}
            <select v-model="policyMode" :aria-label="t('admin.copy.09e2fa24')" :disabled="writing || Boolean(pendingWrite)">
              <option value="">{{ t('admin.copy.699e8b76') }}</option><option value="automatic">{{ t('admin.copy.9b247acc') }}</option><option value="manual">{{ t('admin.copy.1a45ea80') }}</option>
            </select>
          </label>

          <small v-if="reason">{{ reasonBytes }} / {{ t('admin.copy.c8116369') }}</small>
          <button class="sm-primary button button-primary" type="button" :disabled="!policy || writing || Boolean(pendingWrite) || !validSettlementReason(reason)" @click="review('policy')">{{ t('admin.copy.6d70941d') }}</button>
        </div>
      </div>

      <div v-if="rights.view && periodId" class="sm-panel">
        <div class="sm-section-title"><h3>{{ t('admin.copy.625b254a') }}</h3><span v-if="context" class="sm-badge" :class="canStart ? 'ready' : 'blocked'">{{ context.can_start ? t('admin.copy.8e6cc0ec') : t('admin.copy.2fab7bbb') }}</span></div>
        <p v-global-loading v-if="contextBusy" class="sm-note">{{ t('admin.copy.aa4dcd75') }}</p>
        <dl v-if="context" class="sm-facts">
          <div><dt>{{ t('admin.copy.11f487e4') }}</dt><dd class="sm-break"><AssociationName :record="context" kind="brand" :id="context.brand_id" :locale="associationLocale" /> · <AssociationName :record="context" kind="game" :id="context.game_id" :locale="associationLocale" /> · <AssociationName :record="context" kind="period" :id="context.period_id" :locale="associationLocale" /></dd></div>
          <div><dt>{{ t('admin.copy.4ac4f8df') }}</dt><dd> {{ periodStatusLabel(context.period_status) }}</dd></div>
          <div><dt>{{ t('admin.copy.59b4215b') }}</dt><dd class="sm-break">{{ context.draw_result_id ?? "—" }}</dd></div>
          <div><dt>{{ t('admin.copy.b50b0f52') }}</dt><dd> {{ settlementModeLabel(context.mode) }}</dd></div>
        </dl>
        <p v-if="context && (!context.mode || context.policy_version === 0)" class="sm-warning compact">{{ t('admin.copy.b52bef61') }}</p>
        <p v-else-if="context && policy && (context.policy_version !== policy.version || context.mode !== policy.mode)" class="sm-warning compact">{{ t('admin.copy.9b6d565a') }}</p>
        <p v-if="rights.view && !rights.run" class="sm-note">{{ t('admin.copy.5291770d') }}</p>
        <div v-if="rights.run && canWrite && context" class="sm-form">

          <small v-if="reason">{{ reasonBytes }} / {{ t('admin.copy.c8116369') }}</small>
          <button class="sm-primary button button-primary" type="button" :disabled="!canStart || !validSettlementReason(reason)" @click="review('start')">{{ t('admin.copy.d776ab5a') }}</button>
        </div>
      </div>

      <div v-if="rights.view && selectedJob" class="sm-panel">
        <div class="sm-section-title"><h3>{{ t('admin.copy.56e68471') }}</h3><span class="sm-badge" :class="selectedJob.state === 'completed' ? 'ready' : selectedJob.state === 'failed' ? 'blocked' : 'pending'">{{ jobStateLabel(selectedJob.state) }}</span></div>
        <p v-global-loading v-if="jobBusy" class="sm-note">{{ t('admin.copy.83428ba2') }}</p>
        <p v-if="!selectedJob.current" class="sm-warning compact">{{ t('admin.copy.0e0d3ac6') }}</p>
        <p v-if="selectedJob.current && selectedJob.mode === 'manual' && selectedJob.state === 'awaiting_approval'" class="sm-warning compact">{{ t('admin.copy.6d6bcc5d') }}</p>
        <p v-if="selectedJob.current && selectedJob.state === 'completed' && !currentJobIsTerminal" class="sm-warning compact">{{ t('admin.copy.062a6684') }}</p>
        <p v-if="selectedJob.state === 'failed'" class="sm-warning compact">{{ t('admin.copy.438494a3') }}</p>
        <dl class="sm-facts">
          <div><dt>{{ t('admin.copy.714ad808') }}</dt><dd>{{ t('admin.settlement.generation', { generation: selectedJob.generation }) }} · {{ selectedJob.current ? t('admin.copy.a83d9ef8') : t('admin.copy.5af7459d') }}</dd></div>
          <div><dt>{{ t('admin.copy.de7aca53') }}</dt><dd class="sm-break">{{ selectedJob.previous_job_id ?? '—' }} / {{ selectedJob.correction_id ?? '—' }}</dd></div>
          <div><dt>{{ t('admin.copy.d1f86c4a') }}</dt><dd class="sm-break">{{ selectedJob.id }}</dd></div>
          <div><dt>{{ t('admin.copy.11f487e4') }}</dt><dd class="sm-break"><AssociationName :record="selectedJob" kind="brand" :id="selectedJob.brand_id" :locale="associationLocale" /> · <AssociationName :record="selectedJob" kind="game" :id="selectedJob.game_id" :locale="associationLocale" /> · <AssociationName :record="selectedJob" kind="period" :id="selectedJob.period_id" :locale="associationLocale" /></dd></div>
          <div><dt>{{ t('admin.copy.eea32b3c') }}</dt><dd> {{ settlementModeLabel(selectedJob.mode) }}</dd></div>
          <div><dt>{{ t('admin.copy.dde15187') }}</dt><dd class="sm-break"> {{ selectedJob.draw_result_id }}</dd></div>
          <div><dt>{{ t('admin.copy.17c8ec6d') }}</dt><dd>{{ selectedJob.target_count }}</dd></div>
          <div><dt>{{ t('admin.copy.d7e52eb3') }}</dt><dd>{{ selectedJob.pending_count }} / {{ selectedJob.ready_count }} / {{ selectedJob.paid_count }}</dd></div>
          <div><dt>{{ t('admin.copy.4fbdc908') }}</dt><dd>{{ selectedJob.excluded_count }} / {{ selectedJob.failed_count }}</dd></div>
          <div><dt>{{ t('admin.copy.6880653a') }}</dt><dd>{{ formatPoints(selectedJob.prize_points) }}</dd></div>
          <div><dt>{{ t('admin.copy.eac73db9') }}</dt><dd>{{ formatPoints(selectedJob.paid_points) }}</dd></div>
          <div><dt>{{ t('admin.copy.0bb56300') }}</dt><dd class="sm-break"><AssociationName :record="selectedJob" kind="admin" :id="selectedJob.created_by" :locale="associationLocale" /> / <template v-if="selectedJob.approved_by"><AssociationName :record="selectedJob" kind="admin" :id="selectedJob.approved_by" :locale="associationLocale" /></template><span v-else>{{ "—" }}</span></dd></div>
          <div><dt>{{ t('admin.copy.ce57e054') }}</dt><dd class="sm-break">{{ displayDate(selectedJob.created_at) }} / {{ displayDate(selectedJob.completed_at) }}</dd></div>
          <div><dt>{{ t('admin.copy.da9cc09c') }}</dt><dd class="sm-break">{{ selectedJob.reason }} / {{ selectedJob.last_error_code ? displayPublicError(publicErrorMessage({ code: selectedJob.last_error_code })) : "—" }}</dd></div>
        </dl>
        <p v-if="selectedJob.state === 'completed' && currentJobIsTerminal" class="sm-success">{{ t('admin.copy.567ff18d') }}</p>
        <div v-if="canWrite && selectedJob.current && ((selectedJob.state === 'awaiting_approval' && rights.approve) || (selectedJob.state === 'failed' && selectedJob.can_retry && rights.retry))" class="sm-form">

          <small v-if="reason">{{ reasonBytes }} / {{ t('admin.copy.c8116369') }}</small>
          <button v-if="selectedJob.state === 'awaiting_approval' && rights.approve" class="sm-primary button button-primary" type="button" :disabled="writing || Boolean(pendingWrite) || !validSettlementReason(reason)" @click="review('approve')">{{ t('admin.copy.33918fe4') }}</button>
          <button v-if="selectedJob.state === 'failed' && selectedJob.can_retry && rights.retry" class="sm-primary button button-primary" type="button" :disabled="writing || Boolean(pendingWrite) || !validSettlementReason(reason)" @click="review('retry')">{{ t('admin.copy.fcb16b59') }}</button>
        </div>

        <div class="sm-target-heading"><h4>{{ t('admin.copy.1b2929b5') }}</h4><span>{{ t('admin.copy.5cb7f04d') }} {{ SETTLEMENT_JOB_PAGE_SIZE }} {{ t('admin.copy.067fed38') }}</span></div>
        <p v-global-loading v-if="targetsBusy" class="sm-note">{{ t('admin.copy.461d61b9') }}</p>
        <div v-if="targetPage?.items.length" class="sm-targets">
          <article v-for="target in targetPage.items" :key="target.order_id" class="sm-target">
            <div class="sm-target-title"><strong>{{ targetStateLabel(target.state) }}</strong><span>{{ outcomeText(target.won) }}</span></div>
            <dl class="sm-facts compact-facts">
              <div><dt>{{ t('admin.copy.8a34d31c') }}</dt><dd class="sm-break">{{ target.order_id }} / <AssociationName :record="target" kind="member" :id="target.member_id" :locale="associationLocale" /></dd></div>
              <div><dt>{{ t('admin.copy.14d0d2c9') }}</dt><dd> {{ orderStateLabel(target.order_status) }}</dd></div>
              <div><dt>{{ t('admin.copy.d2a4e247') }}</dt><dd class="sm-break">{{ target.calculation_id ?? "—" }} / {{ formatPoints(target.prize_points) }}</dd></div>
              <div><dt>{{ t('admin.copy.74549735') }}</dt><dd class="sm-break">{{ target.payout_entry_id ?? "—" }} / {{ target.error_code ? displayPublicError(publicErrorMessage({ code: target.error_code })) : "—" }}</dd></div>
            </dl>
          </article>
        </div>
        <p v-else-if="targetPage && !targetsBusy" class="sm-note empty-state">{{ t('admin.copy.197d41dd') }}</p>
        <div v-if="targetPage" class="sm-pagination">
          <button class="sm-secondary button button-secondary" type="button" :disabled="targetOffset <= 0 || targetsBusy || writing || Boolean(pendingWrite)" @click="withGlobalLoading(readTargets, selectedJob.id, Math.max(0, targetOffset - SETTLEMENT_JOB_PAGE_SIZE))">{{ t('admin.copy.dbe7a5d7') }}</button>
          <span>{{ t('admin.copy.4ffb01e2') }} {{ targetPage.offset }} · {{ targetPage.items.length }} {{ t('admin.copy.067fed38') }}</span>
          <button class="sm-secondary button button-secondary" type="button" :disabled="!targetPage.has_more || targetsBusy || writing || Boolean(pendingWrite)" @click="withGlobalLoading(readTargets, selectedJob.id, targetOffset + SETTLEMENT_JOB_PAGE_SIZE)">{{ t('admin.copy.2e9c9e7a') }}</button>
        </div>
      </div>

      <div v-if="reviewMode && reviewBody && !pendingWrite" class="sm-confirm">
        <strong>{{ t('admin.copy.4047ee4c') }}</strong>
        <p v-if="reviewMode === 'policy'">{{ t('admin.copy.c9509bb1') }} {{ settlementModeLabel((reviewBody as PolicyBody).mode) }}{{ t('admin.copy.73d5fbcf') }}</p>
        <p v-else-if="reviewMode === 'start'"> {{ t('admin.copy.0336a9c7') }}  {{ (reviewBody as StartBody).draw_result_id }}{{ t('admin.copy.79ba3c08') }}</p>
        <p v-else>{{ t('admin.copy.da4f443a') }} {{ selectedJob?.id }}{{ t('admin.copy.e0209a0a') }}{{ reviewMode === 'approve' ? t('admin.copy.d14f2dd8') : t('admin.copy.248d391e') }}。</p>
        <p v-if="reviewBody.reason">{{ t('admin.copy.2809a1e6') }}{{ reviewBody.reason }}</p>
        <label class="sm-check"><input v-model="reviewConfirmed" type="checkbox" />{{ t('admin.copy.08022d44') }}</label>
        <div class="sm-review-actions"><button class="sm-secondary button button-secondary" type="button" :disabled="writing" @click="cancelReview">{{ t('admin.copy.bf7400b6') }}</button><button v-write-guard="writing" class="sm-primary button button-primary" type="button" :disabled="!reviewConfirmed || writing || !canWrite" @click="withGlobalLoading(beginWrite)">{{ writing ? t('admin.copy.46511dc0') : t('admin.copy.d562331f') }}</button></div>
      </div>
    </template>
  </section>
</template>

<style scoped>
details > summary { min-height:44px; padding:10px 0; box-sizing:border-box; cursor:pointer; }
.settlement-management{display:grid;gap:16px;color:var(--ink)}.sm-heading,.sm-section-title,.sm-target-heading,.sm-target-title,.sm-pagination,.sm-review-actions{display:flex;align-items:center;justify-content:space-between;gap:12px}.sm-heading h2{margin:2px 0 0;font-size:1.25rem}.sm-eyebrow{margin:0;color:#68758a;font-size:.78rem}.sm-warning,.sm-pending,.sm-confirm,.sm-panel{border:1px solid var(--line);border-radius:12px;padding:16px;background:var(--surface)}.sm-warning{background:#fff8e8;border-color:#e9c66a;color:#694d08}.sm-warning.compact{padding:10px;margin:12px 0}.sm-pending{background:#fff1ee;border-color:#df9385}.sm-panel{display:grid;gap:12px}.sm-section-title h3,.sm-target-heading h4{margin:0}.sm-target-heading{margin-top:8px}.sm-target-heading span,.sm-note,.sm-message,.sm-target-title span,.sm-pagination span{color:var(--muted);font-size:.88rem}.sm-facts{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px 18px;margin:0}.sm-facts>div{min-width:0;padding:8px 0;border-bottom:1px solid #eef1f5}.sm-facts dt{color:#697586;font-size:.78rem}.sm-facts dd{margin:4px 0 0;font-weight:550}.sm-break{overflow-wrap:anywhere;word-break:break-word}.sm-badge{display:inline-flex;border-radius:99px;padding:4px 10px;font-size:.8rem;background:#edf0f5}.sm-badge.ready{background:#e6f6eb;color:#176b37}.sm-badge.blocked{background:#fff0ee;color:#a32e1a}.sm-badge.pending{background:#fff5dd;color:#725708}.sm-form{display:grid;gap:8px;max-width:720px}.sm-form label,.sm-period-select label{display:grid;gap:6px;font-size:.9rem}.sm-form textarea,.sm-form select,.sm-period-select input{width:100%;box-sizing:border-box;border:1px solid var(--line);border-radius:8px;padding:9px 10px;font:inherit;background:white}.sm-form small{color:var(--muted)}.sm-primary,.sm-secondary{border:1px solid #c5ceda;border-radius:8px;padding:8px 12px;font:inherit;cursor:pointer;background:var(--surface)}.sm-primary{background:var(--primary);border-color:var(--primary);color:var(--surface)}.sm-primary:disabled,.sm-secondary:disabled{opacity:.52;cursor:not-allowed}.sm-message{margin:0;padding:10px 12px;border-radius:8px;background:#f0f4fa}.sm-message.error{background:#fff0ee;color:#a32e1a}.sm-success{margin:0;padding:10px 12px;border-radius:8px;background:#e6f6eb;color:#176b37}.sm-period-select{max-width:720px}.sm-targets{display:grid;gap:10px}.sm-target{border:1px solid #e2e7ef;border-radius:10px;padding:12px}.compact-facts{margin-top:8px}.compact-facts>div{padding:5px 0}.sm-check{display:flex;align-items:flex-start;gap:8px;margin:12px 0}.sm-review-actions{justify-content:flex-end}.sm-pagination{justify-content:center;flex-wrap:wrap}
@media(max-width:600px){.settlement-management{gap:12px}.sm-heading{align-items:flex-start}.sm-warning,.sm-panel,.sm-confirm,.sm-pending{padding:12px}.sm-facts{grid-template-columns:minmax(0,1fr);gap:2px}.sm-section-title{align-items:flex-start}.sm-pagination .sm-secondary{flex:1}.sm-review-actions{align-items:stretch;flex-direction:column-reverse}.sm-review-actions button{width:100%}}
</style>
