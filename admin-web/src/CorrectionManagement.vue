<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
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
const error = ref("");
const notice = ref("");
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
function handleError(problem: unknown) {
  if (problem instanceof AdminApiError && problem.status === 401) { invalidateSession(); return; }
  error.value = problem instanceof Error ? problem.message : "请求失败，请重试。";
}
function checkContext(value: Context) {
  if (value.brand_id !== props.brandId || value.period_id !== periodId.value)
    throw new Error("更正上下文与当前品牌或期次不匹配，请重新读取。");
}
function checkCorrection(value: Correction) {
  if (value.brand_id !== props.brandId || value.period_id !== periodId.value)
    throw new Error("更正记录与当前品牌或期次不匹配。");
  if (!correctionCountsMatch(value)) throw new Error("服务器更正目标计数不一致，已停止显示该记录。");
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
    if (!result.can_correct) notice.value = "服务器当前不允许更正此期次。";
    else if (result.current_job_id && (!result.mode || result.policy_version < 1)) notice.value = "当前存在结算任务，但结算模式或策略版本无效；禁止财务更正。";
    else notice.value = "已读取服务器开奖、更正权限和当前结算任务。";
    return true;
  } catch (problem) {
    if (problem instanceof AdminApiError && problem.status === 401) { invalidateSession(); return false; }
    if (isCurrent(contextLane, ticket, capturedScope)) { context.value = null; handleError(problem); }
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
    if (isCurrent(historyLane, ticket, capturedScope)) { history.value = null; handleError(problem); }
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
    if (isCurrent(detailLane, ticket, capturedScope)) { selected.value = null; clearTargets(); handleError(problem); }
    return false;
  } finally { if (isCurrent(detailLane, ticket, capturedScope, true)) detailBusy.value = false; }
}
async function readTargets(id = selected.value?.id, offset = targetOffset.value, capturedScope = scope.value, allowDuringWrite = false) {
  if (!rights.value.view || !id || (writing.value && !allowDuringWrite)) return false;
  const ticket = targetsLane.begin(); targetsBusy.value = true;
  try {
    const page = await api.targets(props.brandId, id, CORRECTION_PAGE_SIZE, offset);
    if (!isCurrent(targetsLane, ticket, capturedScope) || selected.value?.id !== id) return false;
    if (page.brand_id !== props.brandId || page.correction_id !== id) throw new Error("目标列表与当前更正不匹配。");
    targetPage.value = page; targetOffset.value = page.offset; return true;
  } catch (problem) {
    if (problem instanceof AdminApiError && problem.status === 401) { invalidateSession(); return false; }
    if (isCurrent(targetsLane, ticket, capturedScope)) { targetPage.value = null; handleError(problem); }
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
  if (pendingWrite.value) notice.value = "存在结果未知的写入。只读刷新不会确认或清除它；只能使用原请求体和幂等键重试。";
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
function beginWrite() {
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
  void submitPending();
}
function validateReceipt(value: Correction, intent: WriteIntent) {
  checkCorrection(value);
  if (intent.operation === "correct") {
    const body = intent.body as CorrectBody;
    if (!intent.expectedDrawResultId || intent.expectedMode === undefined || !matchesCorrectionReceipt(value, {
      brandId: intent.brandId, periodId: intent.periodId, previousDrawResultId: intent.expectedDrawResultId,
      periodVersion: body.version, policyVersion: body.policy_version, mode: intent.expectedMode,
      accountId: intent.accountId, reason: body.reason, result: body.result, financial: body.policy_version !== null,
    })) throw new Error("更正回执与原账号、品牌、期次、开奖记录、递增版本、模式、原因或规范结果不匹配。");
  } else {
    const body = intent.body as RetryBody;
    if (!intent.resourceId || !matchesCorrectionRetryReceipt(value, { id: intent.resourceId, version: body.version }))
      throw new Error("重试回执未匹配原更正、冲正状态或递增后的版本。");
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
  notice.value = unknownWrite.value ? "正在使用原请求体和幂等键重试…" : "正在提交已核对的更正操作…";
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
      notice.value = "写入结果未知。原请求体和幂等键仍保留；只读状态不能证明操作已成功。";
    } else {
      frozenCorrectionWrites.forget(intent); pendingWrite.value = null; unknownWrite.value = false; cancelReview();
      notice.value = failure === "conflict" ? "服务器报告版本冲突；请重新读取并重新核对。" : "服务器明确拒绝请求；请检查权限和操作原因。";
    }
    handleError(problem); writing.value = false; return;
  }
  if (!isCurrent(writeLane, ticket, capturedScope, true)) return;
  try { validateReceipt(receipt, intent); }
  catch (problem) {
    frozenCorrectionWrites.remember(intent); pendingWrite.value = intent; unknownWrite.value = true;
    notice.value = "收到的回执与原请求不匹配。原请求仍保留，只能使用相同请求体和幂等键重试。";
    handleError(problem); writing.value = false; return;
  }

  // A validated receipt acknowledges the write before follow-up GETs. A GET failure never re-freezes it.
  frozenCorrectionWrites.forget(intent); pendingWrite.value = null; unknownWrite.value = false; cancelReview();
  notice.value = "服务器已确认写入；正在读取最新服务器状态。";
  const correctionId = intent.operation === "correct" ? receipt.id : intent.resourceId!;
  try {
    const detailOk = await readCorrection(correctionId, capturedScope, true);
    if (!isCurrent(writeLane, ticket, capturedScope, true)) return;
    const historyOk = await readHistory(0, capturedScope, true);
    if (!isCurrent(writeLane, ticket, capturedScope, true)) return;
    if (!detailOk || !historyOk) notice.value = "写入已确认；后续读取失败。写入保持已确认，不会要求重复提交。请只读刷新。";
    else notice.value = "更正已确认，并已读取最新服务器状态。";
  } finally { if (isCurrent(writeLane, ticket, capturedScope, true)) writing.value = false; }
}

function stateLabel(state: string) {
  const labels: Record<string, string> = { reversing: "正在撤销旧账", resettling: "正在重算并结算", failed: "更正失败", completed: "更正完成" };
  return labels[state] ?? state;
}
function targetStateLabel(state: string) {
  const labels: Record<string, string> = { pending: "待处理", reversed: "已撤销", unchanged: "无需撤销", excluded: "已排除", failed: "失败" };
  return labels[state] ?? state;
}
function modelDescription(model: NonNullable<Context["model"]>) {
  return `${model.model}${model.ordered ? " · 顺序有意义" : " · 普通号码按升序规范化"}`;
}
function selectCorrection(id: string) { if (id !== selected.value?.id) { cancelReview(); selected.value = null; clearTargets(); void readCorrection(id); } }

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
      <div><p class="cm-eyebrow">开奖更正 · 账务冲正</p><h2 id="correction-management-title">开奖结果更正</h2></div>
      <button v-if="rights.view" class="cm-secondary" type="button" :disabled="contextBusy || historyBusy || writing" @click="refreshAll">只读刷新</button>
    </header>
    <p class="cm-warning" role="note"><strong>开奖结果更正不可撤回。</strong>有结算任务时，先按原账本全额冲正，再按新开奖结果重算。若可用中奖积分不足，冲正会安全停止：不会扣其他来源积分，也不会产生负债。</p>
    <p v-if="!rights.view" class="cm-note">当前账号没有 draw.view.brand / draw.view.platform 查看权限。</p>
    <template v-else>
      <div class="cm-period"><label for="correction-period-id">期次 UUID</label><input id="correction-period-id" v-model.trim="periodId" type="text" placeholder="输入期次 UUID" :disabled="writing || Boolean(pendingWrite)" /></div>
      <p v-if="error" class="cm-message error" role="alert">{{ error }}</p>
      <p v-if="notice" class="cm-message" role="status">{{ notice }}</p>
      <div v-if="pendingWrite" class="cm-pending">
        <strong>写入结果未知 · {{ pendingWrite.operation === 'correct' ? '开奖更正' : '重试冲正' }}</strong>
        <p>原请求、账号、品牌、期次、目标及幂等键仅保存在本页内存。只读刷新不会确认或清除写入。</p>
        <p class="cm-break">冻结的期次 {{ pendingWrite.periodId }} · 开奖记录 {{ pendingWrite.expectedDrawResultId ?? '—' }} · 模式 {{ pendingWrite.expectedMode ?? '无财务结算' }} · 请求版本 v{{ pendingWrite.body.version }}</p>
        <button class="cm-primary" type="button" :disabled="writing || (pendingWrite.operation === 'correct' ? !rights.correct || ((pendingWrite.body as CorrectBody).policy_version !== null && !rights.settleRun) : !rights.retry || !rights.settleRun)" @click="submitPending">{{ writing ? '按原请求提交中…' : '原样重试同一请求' }}</button>
      </div>

      <section v-if="context" class="cm-panel" aria-labelledby="cm-context-title">
        <div class="cm-section-title"><h3 id="cm-context-title">期次上下文</h3><span class="cm-badge" :class="context.can_correct ? 'ready' : 'blocked'">{{ context.can_correct ? '服务器允许更正' : '暂不可更正' }}</span></div>
        <p v-if="contextBusy" class="cm-note">正在读取期次上下文…</p>
        <dl class="cm-facts">
          <div><dt>品牌 / 彩种 / 期次</dt><dd class="cm-break">{{ context.brand_id }} · {{ context.game_id }} · {{ context.period_id }}</dd></div>
          <div><dt>期次版本 / 状态</dt><dd>v{{ context.period_version }} · {{ context.period_status }}</dd></div>
          <div><dt>当前开奖记录</dt><dd class="cm-break">{{ context.draw_result_id ?? '—' }}</dd></div>
          <div><dt>号码模型</dt><dd>{{ modelDescription(context.model) }}</dd></div>
          <div><dt>当前结算任务</dt><dd class="cm-break">{{ context.current_job_id ?? '无' }}<template v-if="context.current_job_id"> · v{{ context.current_job_version }}</template></dd></div>
          <div><dt>结算策略 / 模式</dt><dd>v{{ context.policy_version }} · {{ context.mode ?? '未配置' }}</dd></div>
        </dl>
        <p v-if="context.current_job_id" class="cm-warning compact">此更正涉及财务：必须有有效结算模式、draw.correct.brand 和 settlement.run.brand。旧代次会冻结，旧账逐单冲正后才创建新结算任务。</p>
        <p v-else class="cm-note">当前无结算任务；提交后只更正未结算结果，不会启动或创建结算任务。</p>
        <p v-if="rights.correct && !rights.settleRun && context.current_job_id" class="cm-note">缺少 settlement.run.brand，不能提交财务更正。</p>
        <p v-if="props.account.super_admin" class="cm-note">超级管理员可查看，但更正操作不提供写入。</p>
      </section>

      <section v-if="context && rights.correct && !props.account.super_admin" class="cm-panel" aria-labelledby="cm-edit-title">
        <div class="cm-section-title"><h3 id="cm-edit-title">更正结果</h3><span class="cm-badge" :class="financial ? 'pending' : 'ready'">{{ financial ? '更正并重新结算' : '只更正未结算结果' }}</span></div>
        <dl class="cm-facts cm-results">
          <div><dt>更正前</dt><dd class="cm-break">普通：{{ context.draw?.regular.join(', ') ?? '—' }}；特别：{{ context.draw?.special.join(', ') ?? '—' }}；数字：{{ context.draw?.digits.join(', ') ?? '—' }}</dd></div>
          <div><dt>提交候选（规范化后）</dt><dd class="cm-break">普通：{{ parsedResult?.regular.join(', ') ?? '格式无效' }}；特别：{{ parsedResult?.special.join(', ') ?? '格式无效' }}；数字：{{ parsedResult?.digits.join(', ') ?? '格式无效' }}</dd></div>
        </dl>
        <div class="cm-form">
          <label for="correction-regular">普通号码（逗号分隔）</label><input id="correction-regular" v-model="resultInput.regular" aria-label="普通号码（逗号分隔）" inputmode="numeric" autocomplete="off" placeholder="例如 3, 12, 28" :disabled="writing || Boolean(pendingWrite)" />
          <label for="correction-special">特别号码（逗号分隔）</label><input id="correction-special" v-model="resultInput.special" aria-label="特别号码（逗号分隔）" inputmode="numeric" autocomplete="off" placeholder="例如 7" :disabled="writing || Boolean(pendingWrite)" />
          <label for="correction-digits">数字结果（逗号分隔）</label><input id="correction-digits" v-model="resultInput.digits" aria-label="数字结果（逗号分隔）" inputmode="numeric" autocomplete="off" placeholder="例如 1, 2, 3, 4" :disabled="writing || Boolean(pendingWrite)" />
          <p class="cm-note">普通及特别号码按模型规则处理；数字结果保持输入顺序。相同旧结果不能再次提交。</p>
          <label for="correction-reason">操作原因（UTF-8 不超过 500 字节）</label><textarea id="correction-reason" v-model="reason" rows="2" maxlength="500" :disabled="writing || Boolean(pendingWrite)" placeholder="说明更正依据" />
          <small>{{ reasonBytes }} / 500 字节</small>
          <button class="cm-primary" type="button" :disabled="!canCorrect" @click="reviewCorrect">{{ financial ? '核对并更正、重新结算' : '核对并只更正未结算结果' }}</button>
          <p v-if="sameAsPrevious" class="cm-note">候选结果与当前开奖结果相同，无需更正。</p>
        </div>
      </section>

      <section v-if="rights.view && periodId" class="cm-panel" aria-labelledby="cm-history-title">
        <div class="cm-section-title"><h3 id="cm-history-title">更正历史</h3><span>每页 {{ CORRECTION_PAGE_SIZE }} 条</span></div>
        <p v-if="historyBusy" class="cm-note">正在读取更正历史…</p>
        <div v-if="history?.items.length" class="cm-history-list">
          <button v-for="item in history.items" :key="item.id" class="cm-history-item" :class="{ active: selected?.id === item.id }" type="button" :disabled="writing || Boolean(pendingWrite)" @click="selectCorrection(item.id)">
            <span><strong>{{ stateLabel(item.state) }}</strong><small>{{ item.created_at }}</small></span><span class="cm-break">{{ item.id }}</span><span>v{{ item.version }} · {{ item.target_count }} 个目标</span>
          </button>
        </div>
        <p v-else-if="history && !historyBusy" class="cm-note">此期次还没有更正记录。</p>
        <div v-if="history" class="cm-pagination">
          <button class="cm-secondary" type="button" :disabled="historyOffset <= 0 || historyBusy || writing" @click="readHistory(Math.max(0, historyOffset - CORRECTION_PAGE_SIZE))">上一页</button>
          <span>偏移 {{ history.offset }} · {{ history.items.length }} 条</span>
          <button class="cm-secondary" type="button" :disabled="!history.has_more || historyBusy || writing" @click="readHistory(historyOffset + CORRECTION_PAGE_SIZE)">下一页</button>
        </div>
      </section>

      <section v-if="selected" class="cm-panel" aria-labelledby="cm-detail-title">
        <div class="cm-section-title"><h3 id="cm-detail-title">更正详情</h3><span class="cm-badge" :class="selected.state === 'completed' ? 'ready' : selected.state === 'failed' ? 'blocked' : 'pending'">{{ stateLabel(selected.state) }}</span></div>
        <p v-if="detailBusy" class="cm-note">正在读取更正详情…</p>
        <dl class="cm-facts">
          <div><dt>更正 / 期次版本</dt><dd class="cm-break">{{ selected.id }} · v{{ selected.period_version }} (更正时版本)</dd></div>
          <div><dt>旧 / 新开奖记录</dt><dd class="cm-break">{{ selected.previous_draw_result_id }} → {{ selected.draw_result_id }}</dd></div>
          <div><dt>该更正保存的规范结果</dt><dd class="cm-break">普通：{{ selected.result.regular.join(', ') }}；特别：{{ selected.result.special.join(', ') }}；数字：{{ selected.result.digits.join(', ') }}</dd></div>
          <div><dt>旧 / 新结算任务</dt><dd class="cm-break">{{ selected.previous_job_id ?? '—' }} → {{ selected.new_job_id ?? '—' }}</dd></div>
          <div><dt>策略 / 模式</dt><dd>{{ selected.policy_version ?? '—' }} · {{ selected.mode ?? '无财务结算' }}</dd></div>
          <div><dt>目标总数 / 状态计数</dt><dd>{{ selected.target_count }} · 待 {{ selected.pending_count }} / 已冲正 {{ selected.reversed_count }} / 无需冲正 {{ selected.unchanged_count }} / 排除 {{ selected.excluded_count }} / 失败 {{ selected.failed_count }}</dd></div>
          <div><dt>原应冲正积分 / 实际冲正积分</dt><dd>{{ formatCorrectionPoints(selected.reverse_points) }} / {{ formatCorrectionPoints(selected.reversed_points) }}</dd></div>
          <div><dt>操作者 / 创建时间 / 完成时间</dt><dd class="cm-break">{{ selected.created_by }} · {{ selected.created_at }} · {{ selected.completed_at ?? '—' }}</dd></div>
          <div><dt>原因 / 最后错误码</dt><dd class="cm-break">{{ selected.reason }} · {{ selected.last_error_code ?? '—' }}</dd></div>
          <div><dt>新任务状态 / 版本 / 错误码</dt><dd>{{ selected.new_job_state ?? '—' }} · {{ selected.new_job_version ?? '—' }} · {{ selected.new_job_error_code ?? '—' }}</dd></div>
        </dl>
        <p v-if="selected.failed_count > 0" class="cm-warning compact">冲正失败会停止，不会自动重试。可用中奖积分不足时不会从其他积分来源扣款，也不会形成负债；原中奖账本只会有一次全额冲正。</p>
        <div v-if="canRetry" class="cm-form">
          <label for="correction-retry-reason">冲正失败重试原因（UTF-8 不超过 500 字节）</label><textarea id="correction-retry-reason" v-model="reason" rows="2" maxlength="500" :disabled="writing || Boolean(pendingWrite)" placeholder="说明重试依据" />
          <small>{{ reasonBytes }} / 500 字节</small>
          <button class="cm-primary" type="button" :disabled="!canStartRetry" @click="reviewRetry">核对并重试失败冲正</button>
        </div>
        <button v-if="selected.new_job_id" class="cm-secondary" type="button" @click="emit('open-settlement', selected.period_id)">打开新结算任务</button>

        <div class="cm-target-heading"><h4>逐单冲正目标</h4><span>每页 {{ CORRECTION_PAGE_SIZE }} 条</span></div>
        <p v-if="targetsBusy" class="cm-note">正在读取冲正目标…</p>
        <div v-if="targetPage?.items.length" class="cm-targets">
          <article v-for="target in targetPage.items" :key="target.order_id" class="cm-target">
            <div class="cm-target-title"><strong>{{ targetStateLabel(target.state) }}</strong><span>{{ target.error_code ?? '—' }}</span></div>
            <dl class="cm-facts compact-facts">
              <div><dt>注单 / 会员</dt><dd class="cm-break">{{ target.order_id }} / {{ target.member_id }}</dd></div>
              <div><dt>旧注单版本 / 状态</dt><dd>v{{ target.old_order_version }} / {{ target.old_order_status }}</dd></div>
              <div><dt>旧核算 / 原派奖账本</dt><dd class="cm-break">{{ target.old_calculation_id ?? '—' }} / {{ target.old_payout_entry_id ?? '—' }}</dd></div>
              <div><dt>原中奖积分 / 冲正账本 / 注单重置版本</dt><dd class="cm-break">{{ formatCorrectionPoints(target.old_prize_points) }} / {{ target.reversal_entry_id ?? '—' }} / {{ target.reset_order_version ?? '—' }}</dd></div>
            </dl>
          </article>
        </div>
        <p v-else-if="targetPage && !targetsBusy" class="cm-note">此页没有冲正目标。</p>
        <div v-if="targetPage" class="cm-pagination">
          <button class="cm-secondary" type="button" :disabled="targetOffset <= 0 || targetsBusy || writing" @click="readTargets(selected.id, Math.max(0, targetOffset - CORRECTION_PAGE_SIZE))">上一页</button>
          <span>偏移 {{ targetPage.offset }} · {{ targetPage.items.length }} 条</span>
          <button class="cm-secondary" type="button" :disabled="!targetPage.has_more || targetsBusy || writing" @click="readTargets(selected.id, targetOffset + CORRECTION_PAGE_SIZE)">下一页</button>
        </div>
      </section>

      <div v-if="reviewMode && reviewBody && !pendingWrite" class="cm-confirm">
        <strong>再次确认不可撤回操作</strong>
        <p v-if="reviewMode === 'correct'">{{ (reviewBody as CorrectBody).policy_version !== null ? '更正并重新结算' : '只更正未结算结果' }}。期次 v{{ reviewBody.version }}；策略 {{ (reviewBody as CorrectBody).policy_version ?? '无' }}；模式 {{ reviewExpectedMode ?? '无财务结算' }}。确认结果：普通 {{ (reviewBody as CorrectBody).result.regular.join(', ') }}；特别 {{ (reviewBody as CorrectBody).result.special.join(', ') }}；数字 {{ (reviewBody as CorrectBody).result.digits.join(', ') }}。</p>
        <p v-else>重试更正 {{ selected?.id }}，使用冲正版本 v{{ reviewBody.version }}。</p>
        <p>原因：{{ reviewBody.reason }}</p>
        <label for="correction-confirmed" class="cm-check"><input id="correction-confirmed" v-model="reviewConfirmed" type="checkbox" />我已核对品牌、期次、版本、结果和原因，确认提交</label>
        <div class="cm-review-actions"><button class="cm-secondary" type="button" :disabled="writing" @click="cancelReview">返回修改</button><button class="cm-primary" type="button" :disabled="!reviewConfirmed || writing || (reviewMode === 'correct' ? !rights.correct : !rights.retry || !rights.settleRun)" @click="beginWrite">{{ writing ? '提交中…' : '确认提交' }}</button></div>
      </div>
    </template>
  </section>
</template>

<style scoped>
.correction-management{display:grid;gap:16px;color:#172033}.cm-heading,.cm-section-title,.cm-target-heading,.cm-target-title,.cm-pagination,.cm-review-actions{display:flex;align-items:center;justify-content:space-between;gap:12px}.cm-heading h2{margin:2px 0 0;font-size:1.25rem}.cm-eyebrow{margin:0;color:#68758a;font-size:.78rem}.cm-warning,.cm-pending,.cm-confirm,.cm-panel{border:1px solid #d9e0ea;border-radius:12px;padding:16px;background:#fff}.cm-warning{background:#fff8e8;border-color:#e9c66a;color:#694d08}.cm-warning.compact{padding:10px;margin:8px 0}.cm-pending{background:#fff1ee;border-color:#df9385}.cm-panel{display:grid;gap:12px}.cm-section-title h3,.cm-target-heading h4{margin:0}.cm-target-heading{margin-top:8px}.cm-target-heading span,.cm-note,.cm-message,.cm-target-title span,.cm-pagination span,.cm-history-item small{color:#667085;font-size:.88rem}.cm-facts{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px 18px;margin:0}.cm-facts>div{min-width:0;padding:8px 0;border-bottom:1px solid #eef1f5}.cm-facts dt{color:#697586;font-size:.78rem}.cm-facts dd{margin:4px 0 0;font-weight:550}.cm-break{overflow-wrap:anywhere;word-break:break-word}.cm-badge{display:inline-flex;border-radius:99px;padding:4px 10px;font-size:.8rem;background:#edf0f5}.cm-badge.ready{background:#e6f6eb;color:#176b37}.cm-badge.blocked{background:#fff0ee;color:#a32e1a}.cm-badge.pending{background:#fff5dd;color:#725708}.cm-form{display:grid;gap:8px;max-width:720px}.cm-form label,.cm-period label{display:grid;gap:6px;font-size:.9rem}.cm-form textarea,.cm-form input,.cm-period input{width:100%;box-sizing:border-box;border:1px solid #cbd3df;border-radius:8px;padding:9px 10px;font:inherit;background:white}.cm-form small{color:#667085}.cm-primary,.cm-secondary{border:1px solid #c5ceda;border-radius:8px;padding:8px 12px;font:inherit;cursor:pointer;background:#fff}.cm-primary{background:#254a83;border-color:#254a83;color:#fff}.cm-primary:disabled,.cm-secondary:disabled{opacity:.52;cursor:not-allowed}.cm-message{margin:0;padding:10px 12px;border-radius:8px;background:#f0f4fa}.cm-message.error{background:#fff0ee;color:#a32e1a}.cm-note{margin:0}.cm-period{display:grid;gap:6px;max-width:720px}.cm-history-list,.cm-targets{display:grid;gap:10px}.cm-history-item{display:grid;grid-template-columns:1fr minmax(120px,1fr) auto;align-items:center;gap:12px;text-align:left;border:1px solid #e2e7ef;border-radius:10px;padding:12px;background:white;color:inherit;font:inherit;cursor:pointer}.cm-history-item span:first-child{display:grid;gap:4px}.cm-history-item.active{border-color:#254a83;background:#f1f5fb}.cm-target{border:1px solid #e2e7ef;border-radius:10px;padding:12px}.compact-facts{margin-top:8px}.compact-facts>div{padding:5px 0}.cm-check{display:flex;align-items:flex-start;gap:8px;margin:12px 0}.cm-review-actions{justify-content:flex-end}.cm-pagination{justify-content:center;flex-wrap:wrap}
@media(max-width:600px){.correction-management{gap:12px}.cm-heading{align-items:flex-start}.cm-warning,.cm-panel,.cm-confirm,.cm-pending{padding:12px}.cm-facts{grid-template-columns:minmax(0,1fr);gap:2px}.cm-section-title{align-items:flex-start}.cm-pagination .cm-secondary{flex:1}.cm-review-actions{align-items:stretch;flex-direction:column-reverse}.cm-review-actions button{width:100%}.cm-history-item{grid-template-columns:1fr;gap:6px}}
</style>
