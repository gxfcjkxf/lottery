<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
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
const error = ref("");
const notice = ref("");
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
function handleError(problem: unknown) {
  if (problem instanceof AdminApiError && problem.status === 401) {
    invalidateSession();
    return;
  }
  error.value = problem instanceof Error ? problem.message : "请求失败，请重试。";
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
    if (isCurrent(policyLane, ticket, capturedScope, rights.value.policyView)) handleError(problem);
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
      notice.value = "当前期次没有有效的结算配置，禁止启动；配置仅影响之后新启动的任务。";
    else if (!nextContext.can_start)
      notice.value = "服务器当前不允许启动该期次的结算任务。";
    else notice.value = "已读取服务器期次上下文和任务状态。";
    return true;
  } catch (problem) {
    if (isCurrent(contextLane, contextTicket, capturedScope) && isCurrent(jobLane, jobTicket, capturedScope)) {
      context.value = null;
      job.value = null;
      clearTargets();
      handleError(problem);
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
    if (isCurrent(jobLane, ticket, capturedScope)) handleError(problem);
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
      handleError(problem);
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
  if (pendingWrite.value) notice.value = "存在结果未知的写入；普通读取不能证明写入是否成功。原请求仍保留，请使用原请求重试。";
}

function review(operation: "policy" | "start" | "approve" | "retry") {
  if (pendingWrite.value || writing.value || !validSettlementReason(reason.value)) return;
  const cleanReason = reason.value.trim();
  if (operation === "policy") {
    if (!rights.value.policyWrite || !canWrite.value || !policy.value) return;
    reviewBody.value = { version: policy.value.version, mode: policyMode.value || null, reason: cleanReason };
  } else if (operation === "start") {
    if (!canStart.value || !context.value || !policy.value || !context.value.draw_result_id) {
      error.value = "期次上下文、已配置的品牌模式和服务器 can_start 均为启动必需条件。";
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

function beginWrite() {
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
  void submitPending();
}

type PolicyReceipt = Policy & { audit_log_id: string };
function checkPolicyReceipt(result: PolicyReceipt, intent: WriteIntent) {
  checkPolicy(result);
  const body = intent.body as PolicyBody;
  if (!result.audit_log_id || result.version <= body.version || result.mode !== body.mode)
    throw new Error("品牌配置响应无法与已提交请求核对；将保留原请求供原样重试。");
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
  })) throw new Error("启动响应与原账号、品牌、期次、配置版本或请求内容不匹配。");
}
function checkJobMutationReceipt(result: Job, intent: WriteIntent) {
  checkJob(result);
  if (result.id !== intent.resourceId || result.version <= (intent.body as JobBody).version)
    throw new Error("任务写入响应没有匹配原任务和递增后的服务器任务版本。");
  if (intent.operation === "approve" && result.approved_by !== intent.accountId)
    throw new Error("批准回执没有匹配本次操作者。");
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
  notice.value = unknownWrite.value ? "正在使用原请求体和幂等键重试…" : "正在提交已核对的结算操作…";
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
      notice.value = "写入结果未知。原请求体和幂等键已保留；读取到相同状态不能证明本次写入成功，只能原样重试。";
    } else {
      frozenSettlementJobWrites.forget(intent);
      pendingWrite.value = null;
      unknownWrite.value = false;
      reviewBody.value = null;
      reviewMode.value = "";
      notice.value = failure === "conflict"
        ? "服务器报告版本冲突。已清除旧请求；请重新读取并重新核对后再提交。"
        : "服务器明确拒绝了请求。请检查权限及原因后重新读取。";
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
    notice.value = "已收到响应但无法确认它匹配原操作。原请求仍被保留，仅可原样重试。";
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
      if (!policyReadOk) throw new Error("结算配置已保存，但读取当前品牌配置失败。");
      const readOk = periodId.value ? await readPeriodState(true, true) : true;
      if (!readOk) throw new Error("结算配置已保存，但读取期次上下文失败。");
    } else {
      const directJob = receipt as Job;
      job.value = directJob;
      clearTargets();
      const policyReadOk = rights.value.policyView ? await readPolicy(true) : true;
      if (!policyReadOk) throw new Error("结算操作已确认，但读取品牌配置失败。");
      const periodReadOk = await readPeriodState(true, true);
      if (!periodReadOk) throw new Error("结算操作已确认，但读取期次上下文失败。");
      if (directJob.id && !(await readJob(directJob.id, capturedScope, true)))
        throw new Error("结算操作已确认，但读取服务器任务失败。");
    }
    if (isCurrent(writeLane, ticket, capturedScope, true)) {
      reason.value = "";
      notice.value = "服务器已确认写入。任务状态以重新读取的服务器任务版本为准；worker 可能继续异步处理。";
    }
  } catch (problem) {
    if (isCurrent(writeLane, ticket, capturedScope, true)) {
      if (problem instanceof AdminApiError && problem.status === 401) {
        invalidateSession();
        return;
      }
      clearReadData();
      error.value = problem instanceof Error ? problem.message : "重新读取失败。";
      notice.value = "服务器已确认写入，但后续读取失败。此操作不再视为未知，也不会自动重放；请手动重新读取。";
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
  return ({ processing: "处理中", awaiting_approval: "待运营批准", paying: "逐单入账中", failed: "失败", completed: "完成" })[state];
}
function targetStateLabel(state: string) {
  return ({ pending: "待处理", ready: "已核算待入账", paid: "已入账", excluded: "已排除", failed: "失败" } as Record<string, string>)[state] ?? state;
}
function outcomeText(won: boolean | null) { return won === null ? "—" : won ? "命中" : "未命中"; }

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
      <div><p class="sm-eyebrow">品牌结算 · 真实积分</p><h2 id="settlement-management-title">结算配置与任务</h2></div>
      <button v-if="rights.view || rights.policyView" class="sm-secondary" type="button" :disabled="policyBusy || contextBusy || jobBusy || writing" @click="refreshAll">重新读取</button>
    </header>
    <p class="sm-warning" role="note"><strong>这是真实积分结算，仅测试环境；不得把计算完成当派奖完成。</strong>自动模式由系统逐单积分入账；手动模式需品牌运营明确批准后才入账。初期不进行真实法币或币支付。</p>
    <p v-if="!rights.view && !rights.policyView" class="sm-note">当前账号没有查看结算配置或任务的权限。</p>
    <template v-else>
      <div class="sm-period-select">
        <label>期次 ID
          <input v-model.trim="periodId" type="text" placeholder="输入期次 UUID；可由期次页传入" :disabled="writing || Boolean(pendingWrite)" />
        </label>
      </div>
      <p v-if="error" class="sm-message error" role="alert">{{ error }}</p>
      <p v-if="notice" class="sm-message" role="status">{{ notice }}</p>
      <div v-if="pendingWrite && (rights.view || rights.policyView)" class="sm-pending">
        <strong>{{ unknownWrite ? "写入结果未知" : "写入待确认" }} · {{ pendingWrite.operation }}</strong>
        <p>已保留原请求体和幂等键。不得通过读取当前状态推断该操作成功；请使用同一操作、请求体和幂等键重试。</p>
        <button class="sm-primary" type="button" :disabled="writing || !canWrite" @click="submitPending">{{ writing ? "按原请求提交中…" : "使用原请求和幂等键重试" }}</button>
      </div>

      <div v-if="rights.policyView" class="sm-panel">
        <div class="sm-section-title"><h3>品牌结算模式</h3><span v-if="policy" class="sm-badge" :class="policyConfigured ? 'ready' : 'blocked'">{{ policy.mode ?? "未配置" }} · v{{ policy.version }}</span></div>
        <p class="sm-note">未配置（mode 为 null）时禁止启动任务。修改模式只影响之后新启动的任务，不会启动已有期次的旧开奖任务。</p>
        <p v-if="policyBusy" class="sm-note">正在读取品牌配置…</p>
        <dl v-if="policy" class="sm-facts">
          <div><dt>品牌</dt><dd class="sm-break">{{ policy.brand_id }}</dd></div>
          <div><dt>当前模式</dt><dd>{{ policy.mode ?? "未配置" }}</dd></div>
          <div><dt>配置版本 / 更新时间</dt><dd>v{{ policy.version }} · {{ policy.updated_at }}</dd></div>
        </dl>
        <p v-if="!rights.policyWrite" class="sm-note">当前账号可查看配置，没有修改权限。</p>
        <div v-else-if="policy && canWrite" class="sm-form">
          <label>新模式
            <select v-model="policyMode" aria-label="新模式" :disabled="writing || Boolean(pendingWrite)">
              <option value="">未配置（禁止新启动）</option><option value="automatic">自动逐单入账</option><option value="manual">手动批准后入账</option>
            </select>
          </label>
          <label>操作原因（UTF-8 不超过 500 字节）<textarea v-model="reason" rows="2" maxlength="500" :disabled="writing || Boolean(pendingWrite)" placeholder="说明品牌结算模式变更原因" /></label>
          <small>{{ reasonBytes }} / 500 字节</small>
          <button class="sm-primary" type="button" :disabled="!policy || writing || Boolean(pendingWrite) || !validSettlementReason(reason)" @click="review('policy')">核对配置变更</button>
        </div>
      </div>

      <div v-if="rights.view && periodId" class="sm-panel">
        <div class="sm-section-title"><h3>期次结算上下文</h3><span v-if="context" class="sm-badge" :class="canStart ? 'ready' : 'blocked'">{{ context.can_start ? "服务器允许启动" : "暂不可启动" }}</span></div>
        <p v-if="contextBusy" class="sm-note">正在读取期次上下文…</p>
        <dl v-if="context" class="sm-facts">
          <div><dt>品牌 / 彩种 / 期次</dt><dd class="sm-break">{{ context.brand_id }} · {{ context.game_id }} · {{ context.period_id }}</dd></div>
          <div><dt>期次版本 / 状态</dt><dd>v{{ context.period_version }} · {{ context.period_status }}</dd></div>
          <div><dt>开奖记录 ID</dt><dd class="sm-break">{{ context.draw_result_id ?? "—" }}</dd></div>
          <div><dt>应用的策略版本 / 模式</dt><dd>v{{ context.policy_version }} · {{ context.mode ?? "未配置" }}</dd></div>
        </dl>
        <p v-if="context && (!context.mode || context.policy_version === 0)" class="sm-warning compact">品牌结算模式尚未配置；禁止启动。修改配置不会自动启动任何期次。</p>
        <p v-else-if="context && policy && (context.policy_version !== policy.version || context.mode !== policy.mode)" class="sm-warning compact">期次上下文策略与当前品牌配置版本不同，请重新读取服务器上下文。</p>
        <p v-if="rights.view && !rights.run" class="sm-note">当前账号仅可查看期次上下文和任务，没有启动权限。</p>
        <div v-if="rights.run && canWrite && context" class="sm-form">
          <label>启动原因（UTF-8 不超过 500 字节）<textarea v-model="reason" rows="2" maxlength="500" :disabled="writing || Boolean(pendingWrite)" placeholder="说明启动本期结算的依据" /></label>
          <small>{{ reasonBytes }} / 500 字节</small>
          <button class="sm-primary" type="button" :disabled="!canStart || !validSettlementReason(reason)" @click="review('start')">核对并启动新结算任务</button>
        </div>
      </div>

      <div v-if="rights.view && selectedJob" class="sm-panel">
        <div class="sm-section-title"><h3>结算任务</h3><span class="sm-badge" :class="selectedJob.state === 'completed' ? 'ready' : selectedJob.state === 'failed' ? 'blocked' : 'pending'">{{ jobStateLabel(selectedJob.state) }}</span></div>
        <p v-if="jobBusy" class="sm-note">正在重新读取任务…</p>
        <p v-if="!selectedJob.current" class="sm-warning compact">这是历史代次或已被更正流程冻结的代次。显示其原始状态与历史入账金额，不代表当前资金或当前结算完成；此代次不可继续计算、批准或重试。</p>
        <p v-if="selectedJob.current && selectedJob.mode === 'manual' && selectedJob.state === 'awaiting_approval'" class="sm-warning compact">全部可结算目标已完成核算，仍未入账。品牌运营必须明确批准后 worker 才会逐单入账。</p>
        <p v-if="selectedJob.current && selectedJob.state === 'completed' && !currentJobIsTerminal" class="sm-warning compact">服务器任务标记为完成，但 paid + excluded 与目标总数不符；不显示为结算完成。</p>
        <p v-if="selectedJob.state === 'failed'" class="sm-warning compact">任务失败不会自动重试。异常或已取消目标为 excluded，不会自动重试；只有服务器 can_retry 为 true 时才能人工重试。</p>
        <dl class="sm-facts">
          <div><dt>结算代次 / 是否当前</dt><dd>第 {{ selectedJob.generation }} 代 · {{ selectedJob.current ? '当前' : '历史 / 更正冻结' }}</dd></div>
          <div><dt>上一代任务 / 更正任务</dt><dd class="sm-break">{{ selectedJob.previous_job_id ?? '—' }} / {{ selectedJob.correction_id ?? '—' }}</dd></div>
          <div><dt>任务 ID</dt><dd class="sm-break">{{ selectedJob.id }}</dd></div>
          <div><dt>品牌 / 彩种 / 期次</dt><dd class="sm-break">{{ selectedJob.brand_id }} · {{ selectedJob.game_id }} · {{ selectedJob.period_id }}</dd></div>
          <div><dt>服务器任务版本 / 模式</dt><dd>v{{ selectedJob.version }} · {{ selectedJob.mode }}</dd></div>
          <div><dt>期次版本 / 配置版本 / 开奖记录</dt><dd class="sm-break">期次 v{{ selectedJob.period_version }} · 配置 v{{ selectedJob.policy_version }} · {{ selectedJob.draw_result_id }}</dd></div>
          <div><dt>目标总数</dt><dd>{{ selectedJob.target_count }}</dd></div>
          <div><dt>待处理 / 待入账 / 已入账</dt><dd>{{ selectedJob.pending_count }} / {{ selectedJob.ready_count }} / {{ selectedJob.paid_count }}</dd></div>
          <div><dt>已排除 / 失败</dt><dd>{{ selectedJob.excluded_count }} / {{ selectedJob.failed_count }}</dd></div>
          <div><dt>计算积分总额（不是已入账）</dt><dd>{{ formatPoints(selectedJob.prize_points) }}</dd></div>
          <div><dt>已入账积分</dt><dd>{{ formatPoints(selectedJob.paid_points) }}</dd></div>
          <div><dt>创建人 / 批准人</dt><dd class="sm-break">{{ selectedJob.created_by }} / {{ selectedJob.approved_by ?? "—" }}</dd></div>
          <div><dt>创建时间 / 完成时间</dt><dd class="sm-break">{{ selectedJob.created_at }} / {{ selectedJob.completed_at ?? "—" }}</dd></div>
          <div><dt>原因 / 最后错误码</dt><dd class="sm-break">{{ selectedJob.reason }} / {{ selectedJob.last_error_code ?? "—" }}</dd></div>
        </dl>
        <p v-if="selectedJob.state === 'completed' && currentJobIsTerminal" class="sm-success">服务器任务已完成，且全部目标均已入账或排除。这里只代表积分结算任务完成。</p>
        <div v-if="canWrite && selectedJob.current && ((selectedJob.state === 'awaiting_approval' && rights.approve) || (selectedJob.state === 'failed' && selectedJob.can_retry && rights.retry))" class="sm-form">
          <label>{{ selectedJob.state === 'awaiting_approval' ? '运营批准原因' : '人工重试原因' }}（UTF-8 不超过 500 字节）<textarea v-model="reason" rows="2" maxlength="500" :disabled="writing || Boolean(pendingWrite)" placeholder="记录本次人工操作依据" /></label>
          <small>{{ reasonBytes }} / 500 字节</small>
          <button v-if="selectedJob.state === 'awaiting_approval' && rights.approve" class="sm-primary" type="button" :disabled="writing || Boolean(pendingWrite) || !validSettlementReason(reason)" @click="review('approve')">核对并批准，允许 worker 入账</button>
          <button v-if="selectedJob.state === 'failed' && selectedJob.can_retry && rights.retry" class="sm-primary" type="button" :disabled="writing || Boolean(pendingWrite) || !validSettlementReason(reason)" @click="review('retry')">核对并人工重试失败目标</button>
        </div>

        <div class="sm-target-heading"><h4>逐单目标</h4><span>每页 {{ SETTLEMENT_JOB_PAGE_SIZE }} 条</span></div>
        <p v-if="targetsBusy" class="sm-note">正在读取结算目标…</p>
        <div v-if="targetPage?.items.length" class="sm-targets">
          <article v-for="target in targetPage.items" :key="target.order_id" class="sm-target">
            <div class="sm-target-title"><strong>{{ targetStateLabel(target.state) }}</strong><span>{{ outcomeText(target.won) }}</span></div>
            <dl class="sm-facts compact-facts">
              <div><dt>注单 / 会员</dt><dd class="sm-break">{{ target.order_id }} / {{ target.member_id }}</dd></div>
              <div><dt>目标版本 / 注单版本 / 状态</dt><dd>v{{ target.version }} / v{{ target.order_version }} / {{ target.order_status }}</dd></div>
              <div><dt>核算 ID / 积分</dt><dd class="sm-break">{{ target.calculation_id ?? "—" }} / {{ formatPoints(target.prize_points) }}</dd></div>
              <div><dt>入账记录 / 错误码</dt><dd class="sm-break">{{ target.payout_entry_id ?? "—" }} / {{ target.error_code ?? "—" }}</dd></div>
            </dl>
          </article>
        </div>
        <p v-else-if="targetPage && !targetsBusy" class="sm-note">此页没有结算目标。</p>
        <div v-if="targetPage" class="sm-pagination">
          <button class="sm-secondary" type="button" :disabled="targetOffset <= 0 || targetsBusy || writing || Boolean(pendingWrite)" @click="readTargets(selectedJob.id, Math.max(0, targetOffset - SETTLEMENT_JOB_PAGE_SIZE))">上一页</button>
          <span>偏移 {{ targetPage.offset }} · {{ targetPage.items.length }} 条</span>
          <button class="sm-secondary" type="button" :disabled="!targetPage.has_more || targetsBusy || writing || Boolean(pendingWrite)" @click="readTargets(selectedJob.id, targetOffset + SETTLEMENT_JOB_PAGE_SIZE)">下一页</button>
        </div>
      </div>

      <div v-if="reviewMode && reviewBody && !pendingWrite" class="sm-confirm">
        <strong>请确认结算操作</strong>
        <p v-if="reviewMode === 'policy'">将把品牌模式设为 {{ (reviewBody as PolicyBody).mode ?? '未配置' }}，配置版本基于 v{{ reviewBody.version }}。不会自动启动任何已有期次。</p>
        <p v-else-if="reviewMode === 'start'">将使用服务器期次版本 v{{ (reviewBody as StartBody).version }}、策略版本 v{{ (reviewBody as StartBody).policy_version }} 和开奖记录 {{ (reviewBody as StartBody).draw_result_id }} 创建任务。</p>
        <p v-else>任务 {{ selectedJob?.id }}，服务器任务版本 v{{ reviewBody.version }}；操作：{{ reviewMode === 'approve' ? '批准手动任务入账' : '人工重试' }}。</p>
        <p>原因：{{ reviewBody.reason }}</p>
        <label class="sm-check"><input v-model="reviewConfirmed" type="checkbox" />我已核对品牌、期次、服务器版本、操作模式和原因，确认提交</label>
        <div class="sm-review-actions"><button class="sm-secondary" type="button" :disabled="writing" @click="cancelReview">返回修改</button><button class="sm-primary" type="button" :disabled="!reviewConfirmed || writing || !canWrite" @click="beginWrite">{{ writing ? "提交中…" : "确认提交结算操作" }}</button></div>
      </div>
    </template>
  </section>
</template>

<style scoped>
.settlement-management{display:grid;gap:16px;color:#172033}.sm-heading,.sm-section-title,.sm-target-heading,.sm-target-title,.sm-pagination,.sm-review-actions{display:flex;align-items:center;justify-content:space-between;gap:12px}.sm-heading h2{margin:2px 0 0;font-size:1.25rem}.sm-eyebrow{margin:0;color:#68758a;font-size:.78rem}.sm-warning,.sm-pending,.sm-confirm,.sm-panel{border:1px solid #d9e0ea;border-radius:12px;padding:16px;background:#fff}.sm-warning{background:#fff8e8;border-color:#e9c66a;color:#694d08}.sm-warning.compact{padding:10px;margin:12px 0}.sm-pending{background:#fff1ee;border-color:#df9385}.sm-panel{display:grid;gap:12px}.sm-section-title h3,.sm-target-heading h4{margin:0}.sm-target-heading{margin-top:8px}.sm-target-heading span,.sm-note,.sm-message,.sm-target-title span,.sm-pagination span{color:#667085;font-size:.88rem}.sm-facts{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px 18px;margin:0}.sm-facts>div{min-width:0;padding:8px 0;border-bottom:1px solid #eef1f5}.sm-facts dt{color:#697586;font-size:.78rem}.sm-facts dd{margin:4px 0 0;font-weight:550}.sm-break{overflow-wrap:anywhere;word-break:break-word}.sm-badge{display:inline-flex;border-radius:99px;padding:4px 10px;font-size:.8rem;background:#edf0f5}.sm-badge.ready{background:#e6f6eb;color:#176b37}.sm-badge.blocked{background:#fff0ee;color:#a32e1a}.sm-badge.pending{background:#fff5dd;color:#725708}.sm-form{display:grid;gap:8px;max-width:720px}.sm-form label,.sm-period-select label{display:grid;gap:6px;font-size:.9rem}.sm-form textarea,.sm-form select,.sm-period-select input{width:100%;box-sizing:border-box;border:1px solid #cbd3df;border-radius:8px;padding:9px 10px;font:inherit;background:white}.sm-form small{color:#667085}.sm-primary,.sm-secondary{border:1px solid #c5ceda;border-radius:8px;padding:8px 12px;font:inherit;cursor:pointer;background:#fff}.sm-primary{background:#254a83;border-color:#254a83;color:#fff}.sm-primary:disabled,.sm-secondary:disabled{opacity:.52;cursor:not-allowed}.sm-message{margin:0;padding:10px 12px;border-radius:8px;background:#f0f4fa}.sm-message.error{background:#fff0ee;color:#a32e1a}.sm-success{margin:0;padding:10px 12px;border-radius:8px;background:#e6f6eb;color:#176b37}.sm-period-select{max-width:720px}.sm-targets{display:grid;gap:10px}.sm-target{border:1px solid #e2e7ef;border-radius:10px;padding:12px}.compact-facts{margin-top:8px}.compact-facts>div{padding:5px 0}.sm-check{display:flex;align-items:flex-start;gap:8px;margin:12px 0}.sm-review-actions{justify-content:flex-end}.sm-pagination{justify-content:center;flex-wrap:wrap}
@media(max-width:600px){.settlement-management{gap:12px}.sm-heading{align-items:flex-start}.sm-warning,.sm-panel,.sm-confirm,.sm-pending{padding:12px}.sm-facts{grid-template-columns:minmax(0,1fr);gap:2px}.sm-section-title{align-items:flex-start}.sm-pagination .sm-secondary{flex:1}.sm-review-actions{align-items:stretch;flex-direction:column-reverse}.sm-review-actions button{width:100%}}
</style>
