<script setup lang="ts">
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
const { t, message: localized } = useAdminI18n();

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
function handleError(problem: unknown) {
  if (problem instanceof AdminApiError && problem.status === 401) { invalidateSession(); return; }
  error.value = problem instanceof Error ? problem.message : localized("请求失败，请重试。", "Request failed. Please try again.");
}
function checkContext(value: Context) {
  if (value.brand_id !== props.brandId || value.period_id !== periodId.value)
    throw new Error(t("更正上下文与当前品牌或期次不匹配，请重新读取。", "Correction context does not match the current brand or period. Reload and try again."));
}
function checkCorrection(value: Correction) {
  if (value.brand_id !== props.brandId || value.period_id !== periodId.value)
    throw new Error(t("更正记录与当前品牌或期次不匹配。", "Correction record does not match the current brand or period."));
  if (!correctionCountsMatch(value)) throw new Error(t("服务器更正目标计数不一致，已停止显示该记录。", "Server correction target counts do not match; this record is hidden."));
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
    if (!result.can_correct) notice.value = localized("服务器当前不允许更正此期次。", "The server currently does not allow correction of this period.");
    else if (result.current_job_id && (!result.mode || result.policy_version < 1)) notice.value = localized("当前存在结算任务，但结算模式或策略版本无效；禁止财务更正。", "A settlement job exists, but its mode or policy version is invalid. Financial correction is blocked.");
    else notice.value = localized("已读取服务器开奖、更正权限和当前结算任务。", "Draw result, correction eligibility, and current settlement job loaded from the server.");
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
    if (page.brand_id !== props.brandId || page.correction_id !== id) throw new Error(t("目标列表与当前更正不匹配。", "Target list does not match the current correction."));
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
  if (pendingWrite.value) notice.value = localized("存在结果未知的写入。只读刷新不会确认或清除它；只能使用原请求体和幂等键重试。", "A write outcome is unknown. Read-only refresh cannot confirm or clear it; retry only with the original request body and idempotency key.");
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
    })) throw new Error(t("更正回执与原账号、品牌、期次、开奖记录、递增版本、模式、原因或规范结果不匹配。", "Correction receipt does not match the original account, brand, period, draw record, incremented version, mode, reason, or canonical result."));
  } else {
    const body = intent.body as RetryBody;
    if (!intent.resourceId || !matchesCorrectionRetryReceipt(value, { id: intent.resourceId, version: body.version }))
      throw new Error(t("重试回执未匹配原更正、冲正状态或递增后的版本。", "Retry receipt does not match the original correction, reversal state, or incremented version."));
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
  notice.value = unknownWrite.value ? localized("正在使用原请求体和幂等键重试…", "Retrying with the original request body and idempotency key…") : localized("正在提交已核对的更正操作…", "Submitting the reviewed correction…");
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
      notice.value = localized("写入结果未知。原请求体和幂等键仍保留；只读状态不能证明操作已成功。", "The write outcome is unknown. The original request body and idempotency key remain available; read-only state cannot prove that the action succeeded.");
    } else {
      frozenCorrectionWrites.forget(intent); pendingWrite.value = null; unknownWrite.value = false; cancelReview();
      notice.value = failure === "conflict" ? localized("服务器报告版本冲突；请重新读取并重新核对。", "The server reported a version conflict. Reload and review again.") : localized("服务器明确拒绝请求；请检查权限和操作原因。", "The server rejected the request. Check permissions and the reason.");
    }
    handleError(problem); writing.value = false; return;
  }
  if (!isCurrent(writeLane, ticket, capturedScope, true)) return;
  try { validateReceipt(receipt, intent); }
  catch (problem) {
    frozenCorrectionWrites.remember(intent); pendingWrite.value = intent; unknownWrite.value = true;
    notice.value = localized("收到的回执与原请求不匹配。原请求仍保留，只能使用相同请求体和幂等键重试。", "The receipt does not match the original request. The original request is retained and can only be retried with the same body and idempotency key.");
    handleError(problem); writing.value = false; return;
  }

  // A validated receipt acknowledges the write before follow-up GETs. A GET failure never re-freezes it.
  frozenCorrectionWrites.forget(intent); pendingWrite.value = null; unknownWrite.value = false; cancelReview();
  notice.value = localized("服务器已确认写入；正在读取最新服务器状态。", "The server confirmed the write; loading the latest server state.");
  const correctionId = intent.operation === "correct" ? receipt.id : intent.resourceId!;
  try {
    const detailOk = await readCorrection(correctionId, capturedScope, true);
    if (!isCurrent(writeLane, ticket, capturedScope, true)) return;
    const historyOk = await readHistory(0, capturedScope, true);
    if (!isCurrent(writeLane, ticket, capturedScope, true)) return;
    if (!detailOk || !historyOk) notice.value = localized("写入已确认；后续读取失败。写入保持已确认，不会要求重复提交。请只读刷新。", "The write is confirmed, but a follow-up read failed. It remains confirmed and must not be resubmitted. Refresh status only.");
    else notice.value = localized("更正已确认，并已读取最新服务器状态。", "Correction confirmed; the latest server state has been loaded.");
  } finally { if (isCurrent(writeLane, ticket, capturedScope, true)) writing.value = false; }
}

function stateLabel(state: string) {
  const labels: Record<string, string> = { reversing: t("正在撤销旧账", "Reversing old entries"), resettling: t("正在重算并结算", "Recalculating and settling"), failed: t("更正失败", "Correction failed"), completed: t("更正完成", "Correction completed") };
  return labels[state] ?? state;
}
function targetStateLabel(state: string) {
  const labels: Record<string, string> = { pending: t("待处理", "Pending"), reversed: t("已撤销", "Reversed"), unchanged: t("无需撤销", "No reversal needed"), excluded: t("已排除", "Excluded"), failed: t("失败", "Failed") };
  return labels[state] ?? state;
}
function periodStatusLabel(state: string) {
  const labels: Record<string, string> = { pending: t("待开始", "Pending"), betting: t("投注中", "Betting open"), closed: t("已截止", "Closed"), waiting_draw: t("待开奖", "Awaiting draw"), drawn: t("已开奖", "Drawn"), settling: t("结算中", "Settling"), settled: t("已结算", "Settled"), bet_cancelled: t("投注取消", "Betting cancelled"), judged_cancelled: t("判定取消", "Judgment cancelled") };
  return labels[state] ?? state;
}
function orderStatusLabel(state: string) {
  const labels: Record<string, string> = { placed: t("待开奖", "Awaiting draw"), abnormal: t("异常注单", "Abnormal order"), bet_cancelled: t("投注取消", "Betting cancelled"), judged_cancelled: t("判定取消", "Judgment cancelled"), won: t("已中奖", "Won"), lost: t("未中奖", "Lost") };
  return labels[state] ?? state;
}
function jobStateLabel(state: string | null) {
  const labels: Record<string, string> = { processing: t("处理中", "Processing"), awaiting_approval: t("待运营批准", "Awaiting approval"), paying: t("逐单入账中", "Posting payouts"), failed: t("失败", "Failed"), completed: t("完成", "Completed") };
  return state ? labels[state] ?? state : "—";
}
function modelDescription(model: NonNullable<Context["model"]>) {
  return `${model.model}${model.ordered ? ` · ${t("顺序有意义", "order matters")}` : ` · ${t("普通号码按升序规范化", "regular numbers are normalized in ascending order")}`}`;
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
      <div><p class="cm-eyebrow">{{ t("开奖更正 · 账务冲正", "Draw correction · Financial reversal") }}</p><h2 id="correction-management-title">{{ t("开奖结果更正", "Draw result correction") }}</h2></div>
      <button v-if="rights.view" class="cm-secondary" type="button" :disabled="contextBusy || historyBusy || writing" @click="refreshAll">{{ t("只读刷新", "Refresh status") }}</button>
    </header>
    <p class="cm-warning" role="note"><strong>{{ t("开奖结果更正不可撤回。", "Draw corrections cannot be undone.") }}</strong>{{ t("有结算任务时，先按原账本全额冲正，再按新开奖结果重算。若可用中奖积分不足，冲正会安全停止：不会扣其他来源积分，也不会产生负债。", "When a settlement job exists, the original ledger entries are reversed in full before recalculation with the corrected result. Reversal stops safely if available winnings are insufficient; other point sources are not debited and no debt is created.") }}</p>
    <p v-if="!rights.view" class="cm-note">{{ t("当前账号没有 draw.view.brand / draw.view.platform 查看权限。", "This account lacks draw.view.brand / draw.view.platform permission.") }}</p>
    <template v-else>
      <div class="cm-period"><label for="correction-period-id">{{ t("期次 UUID", "Period UUID") }}</label><input id="correction-period-id" v-model.trim="periodId" type="text" :placeholder="t('输入期次 UUID', 'Enter period UUID')" :disabled="writing || Boolean(pendingWrite)" /></div>
      <p v-if="error" class="cm-message error" role="alert">{{ t(error) }}</p>
      <p v-if="notice" class="cm-message" role="status">{{ t(notice) }}</p>
      <div v-if="pendingWrite" class="cm-pending">
        <strong>{{ t("写入结果未知", "Write outcome unknown") }} · {{ pendingWrite.operation === 'correct' ? t('开奖更正', 'Draw correction') : t('重试冲正', 'Retry reversal') }}</strong>
        <p>{{ t("原请求、账号、品牌、期次、目标及幂等键仅保存在本页内存。只读刷新不会确认或清除写入。", "The original request, account, brand, period, targets, and idempotency key are held in this page's memory. A read-only refresh cannot confirm or clear the write.") }}</p>
        <p class="cm-break">{{ t("冻结的期次", "Frozen period") }} {{ pendingWrite.periodId }} · {{ t("开奖记录", "Draw record") }} {{ pendingWrite.expectedDrawResultId ?? '—' }} · {{ t("模式", "Mode") }} {{ pendingWrite.expectedMode ?? t('无财务结算', 'No financial settlement') }} · {{ t("请求版本", "Request version") }} v{{ pendingWrite.body.version }}</p>
        <button class="cm-primary" type="button" :disabled="writing || (pendingWrite.operation === 'correct' ? !rights.correct || ((pendingWrite.body as CorrectBody).policy_version !== null && !rights.settleRun) : !rights.retry || !rights.settleRun)" @click="submitPending">{{ writing ? t('按原请求提交中…', 'Retrying the original request…') : t('原样重试同一请求', 'Retry the exact same request') }}</button>
      </div>

      <section v-if="context" class="cm-panel" aria-labelledby="cm-context-title">
        <div class="cm-section-title"><h3 id="cm-context-title">{{ t("期次上下文", "Period context") }}</h3><span class="cm-badge" :class="context.can_correct ? 'ready' : 'blocked'">{{ context.can_correct ? t('服务器允许更正', 'Correction allowed by server') : t('暂不可更正', 'Correction unavailable') }}</span></div>
        <p v-if="contextBusy" class="cm-note">{{ t("正在读取期次上下文…", "Reading period context…") }}</p>
        <dl class="cm-facts">
          <div><dt>{{ t("品牌 / 彩种 / 期次", "Brand / game / period") }}</dt><dd class="cm-break">{{ context.brand_id }} · {{ context.game_id }} · {{ context.period_id }}</dd></div>
          <div><dt>{{ t("期次版本 / 状态", "Period version / status") }}</dt><dd>v{{ context.period_version }} · {{ periodStatusLabel(context.period_status) }}</dd></div>
          <div><dt>{{ t("当前开奖记录", "Current draw record") }}</dt><dd class="cm-break">{{ context.draw_result_id ?? '—' }}</dd></div>
          <div><dt>{{ t("号码模型", "Number model") }}</dt><dd>{{ modelDescription(context.model) }}</dd></div>
          <div><dt>{{ t("当前结算任务", "Current settlement job") }}</dt><dd class="cm-break">{{ context.current_job_id ?? t('无', 'None') }}<template v-if="context.current_job_id"> · v{{ context.current_job_version }}</template></dd></div>
          <div><dt>{{ t("结算策略 / 模式", "Settlement policy / mode") }}</dt><dd>v{{ context.policy_version }} · {{ context.mode ?? t('未配置', 'Not configured') }}</dd></div>
        </dl>
        <p v-if="context.current_job_id" class="cm-warning compact">{{ t("此更正涉及财务：必须有有效结算模式、draw.correct.brand 和 settlement.run.brand。旧代次会冻结，旧账逐单冲正后才创建新结算任务。", "This correction affects finances and requires a valid settlement mode, draw.correct.brand, and settlement.run.brand. The old generation is frozen; a new job starts only after every old ledger entry is reversed.") }}</p>
        <p v-else class="cm-note">{{ t("当前无结算任务；提交后只更正未结算结果，不会启动或创建结算任务。", "There is no settlement job. Submitting corrects only the unsettled result and will not start or create a settlement job.") }}</p>
        <p v-if="rights.correct && !rights.settleRun && context.current_job_id" class="cm-note">{{ t("缺少 settlement.run.brand，不能提交财务更正。", "Missing settlement.run.brand; financial correction cannot be submitted.") }}</p>
        <p v-if="props.account.super_admin" class="cm-note">{{ t("超级管理员可查看，但更正操作不提供写入。", "Super administrators may view this page, but cannot write corrections.") }}</p>
      </section>

      <section v-if="context && rights.correct && !props.account.super_admin" class="cm-panel" aria-labelledby="cm-edit-title">
        <div class="cm-section-title"><h3 id="cm-edit-title">{{ t("更正结果", "Correct result") }}</h3><span class="cm-badge" :class="financial ? 'pending' : 'ready'">{{ financial ? t('更正并重新结算', 'Correct and settle again') : t('只更正未结算结果', 'Correct unsettled result only') }}</span></div>
        <dl class="cm-facts cm-results">
          <div><dt>{{ t("更正前", "Before correction") }}</dt><dd class="cm-break">{{ t("普通：", "Regular: ") }}{{ context.draw?.regular.join(', ') ?? '—' }}；{{ t("特别：", "Special: ") }}{{ context.draw?.special.join(', ') ?? '—' }}；{{ t("数字：", "Digits: ") }}{{ context.draw?.digits.join(', ') ?? '—' }}</dd></div>
          <div><dt>{{ t("提交候选（规范化后）", "Submitted candidate (normalized)") }}</dt><dd class="cm-break">{{ t("普通：", "Regular: ") }}{{ parsedResult?.regular.join(', ') ?? t('格式无效', 'Invalid format') }}；{{ t("特别：", "Special: ") }}{{ parsedResult?.special.join(', ') ?? t('格式无效', 'Invalid format') }}；{{ t("数字：", "Digits: ") }}{{ parsedResult?.digits.join(', ') ?? t('格式无效', 'Invalid format') }}</dd></div>
        </dl>
        <div class="cm-form">
          <label for="correction-regular">{{ t("普通号码（逗号分隔）", "Regular numbers (comma-separated)") }}</label><input id="correction-regular" v-model="resultInput.regular" :aria-label="t('普通号码（逗号分隔）', 'Regular numbers (comma-separated)')" inputmode="numeric" autocomplete="off" :placeholder="t('例如 3, 12, 28', 'e.g. 3, 12, 28')" :disabled="writing || Boolean(pendingWrite)" />
          <label for="correction-special">{{ t("特别号码（逗号分隔）", "Special numbers (comma-separated)") }}</label><input id="correction-special" v-model="resultInput.special" :aria-label="t('特别号码（逗号分隔）', 'Special numbers (comma-separated)')" inputmode="numeric" autocomplete="off" :placeholder="t('例如 7', 'e.g. 7')" :disabled="writing || Boolean(pendingWrite)" />
          <label for="correction-digits">{{ t("数字结果（逗号分隔）", "Digits (comma-separated)") }}</label><input id="correction-digits" v-model="resultInput.digits" :aria-label="t('数字结果（逗号分隔）', 'Digits (comma-separated)')" inputmode="numeric" autocomplete="off" :placeholder="t('例如 1, 2, 3, 4', 'e.g. 1, 2, 3, 4')" :disabled="writing || Boolean(pendingWrite)" />
          <p class="cm-note">{{ t("普通及特别号码按模型规则处理；数字结果保持输入顺序。相同旧结果不能再次提交。", "Regular and special numbers follow the model rules; digit results retain input order. The current result cannot be submitted again as a correction.") }}</p>
          <label for="correction-reason">{{ t("操作原因（UTF-8 不超过 500 字节）", "Reason (up to 500 UTF-8 bytes)") }}</label><textarea id="correction-reason" v-model="reason" rows="2" maxlength="500" :disabled="writing || Boolean(pendingWrite)" :placeholder="t('说明更正依据', 'Describe the basis for this correction')" />
          <small>{{ reasonBytes }} / {{ t("500 字节", "500 bytes") }}</small>
          <button class="cm-primary" type="button" :disabled="!canCorrect" @click="reviewCorrect">{{ financial ? t('核对并更正、重新结算', 'Review, correct, and settle again') : t('核对并只更正未结算结果', 'Review and correct unsettled result only') }}</button>
          <p v-if="sameAsPrevious" class="cm-note">{{ t("候选结果与当前开奖结果相同，无需更正。", "The candidate matches the current result; no correction is needed.") }}</p>
        </div>
      </section>

      <section v-if="rights.view && periodId" class="cm-panel" aria-labelledby="cm-history-title">
        <div class="cm-section-title"><h3 id="cm-history-title">{{ t("更正历史", "Correction history") }}</h3><span>{{ t("每页", "Per page") }} {{ CORRECTION_PAGE_SIZE }}</span></div>
        <p v-if="historyBusy" class="cm-note">{{ t("正在读取更正历史…", "Reading correction history…") }}</p>
        <div v-if="history?.items.length" class="cm-history-list">
          <button v-for="item in history.items" :key="item.id" class="cm-history-item" :class="{ active: selected?.id === item.id }" type="button" :disabled="writing || Boolean(pendingWrite)" @click="selectCorrection(item.id)">
            <span><strong>{{ stateLabel(item.state) }}</strong><small>{{ item.created_at }}</small></span><span class="cm-break">{{ item.id }}</span><span>v{{ item.version }} · {{ item.target_count }} {{ t("个目标", "targets") }}</span>
          </button>
        </div>
        <p v-else-if="history && !historyBusy" class="cm-note">{{ t("此期次还没有更正记录。", "There are no correction records for this period.") }}</p>
        <div v-if="history" class="cm-pagination">
          <button class="cm-secondary" type="button" :disabled="historyOffset <= 0 || historyBusy || writing" @click="readHistory(Math.max(0, historyOffset - CORRECTION_PAGE_SIZE))">{{ t("上一页", "Previous") }}</button>
          <span>{{ t("偏移", "Offset") }} {{ history.offset }} · {{ history.items.length }} {{ t("条", "items") }}</span>
          <button class="cm-secondary" type="button" :disabled="!history.has_more || historyBusy || writing" @click="readHistory(historyOffset + CORRECTION_PAGE_SIZE)">{{ t("下一页", "Next") }}</button>
        </div>
      </section>

      <section v-if="selected" class="cm-panel" aria-labelledby="cm-detail-title">
        <div class="cm-section-title"><h3 id="cm-detail-title">{{ t("更正详情", "Correction details") }}</h3><span class="cm-badge" :class="selected.state === 'completed' ? 'ready' : selected.state === 'failed' ? 'blocked' : 'pending'">{{ stateLabel(selected.state) }}</span></div>
        <p v-if="detailBusy" class="cm-note">{{ t("正在读取更正详情…", "Reading correction details…") }}</p>
        <dl class="cm-facts">
          <div><dt>{{ t("更正 / 期次版本", "Correction / period version") }}</dt><dd class="cm-break">{{ selected.id }} · v{{ selected.period_version }} ({{ t("更正时版本", "version at correction") }})</dd></div>
          <div><dt>{{ t("旧 / 新开奖记录", "Previous / new draw record") }}</dt><dd class="cm-break">{{ selected.previous_draw_result_id }} → {{ selected.draw_result_id }}</dd></div>
          <div><dt>{{ t("该更正保存的规范结果", "Canonical result saved by this correction") }}</dt><dd class="cm-break">{{ t("普通：", "Regular: ") }}{{ selected.result.regular.join(', ') }}；{{ t("特别：", "Special: ") }}{{ selected.result.special.join(', ') }}；{{ t("数字：", "Digits: ") }}{{ selected.result.digits.join(', ') }}</dd></div>
          <div><dt>{{ t("旧 / 新结算任务", "Previous / new settlement job") }}</dt><dd class="cm-break">{{ selected.previous_job_id ?? '—' }} → {{ selected.new_job_id ?? '—' }}</dd></div>
          <div><dt>{{ t("策略 / 模式", "Policy / mode") }}</dt><dd>{{ selected.policy_version ?? '—' }} · {{ selected.mode ?? t('无财务结算', 'No financial settlement') }}</dd></div>
          <div><dt>{{ t("目标总数 / 状态计数", "Total targets / state counts") }}</dt><dd>{{ selected.target_count }} · {{ t("待", "Pending") }} {{ selected.pending_count }} / {{ t("已冲正", "Reversed") }} {{ selected.reversed_count }} / {{ t("无需冲正", "Unchanged") }} {{ selected.unchanged_count }} / {{ t("排除", "Excluded") }} {{ selected.excluded_count }} / {{ t("失败", "Failed") }} {{ selected.failed_count }}</dd></div>
          <div><dt>{{ t("原应冲正积分 / 实际冲正积分", "Points due for reversal / points reversed") }}</dt><dd>{{ formatCorrectionPoints(selected.reverse_points) }} / {{ formatCorrectionPoints(selected.reversed_points) }}</dd></div>
          <div><dt>{{ t("操作者 / 创建时间 / 完成时间", "Operator / created / completed") }}</dt><dd class="cm-break">{{ selected.created_by }} · {{ selected.created_at }} · {{ selected.completed_at ?? '—' }}</dd></div>
          <div><dt>{{ t("原因 / 最后错误码", "Reason / last error code") }}</dt><dd class="cm-break">{{ selected.reason }} · {{ selected.last_error_code ?? '—' }}</dd></div>
          <div><dt>{{ t("新任务状态 / 版本 / 错误码", "New job state / version / error code") }}</dt><dd>{{ jobStateLabel(selected.new_job_state) }} · {{ selected.new_job_version ?? '—' }} · {{ selected.new_job_error_code ?? '—' }}</dd></div>
        </dl>
        <p v-if="selected.failed_count > 0" class="cm-warning compact">{{ t("冲正失败会停止，不会自动重试。可用中奖积分不足时不会从其他积分来源扣款，也不会形成负债；原中奖账本只会有一次全额冲正。", "A failed reversal stops and is not retried automatically. If available winnings are insufficient, other point sources are not debited and no debt is created. The original winnings ledger is reversed in full only once.") }}</p>
        <div v-if="canRetry" class="cm-form">
          <label for="correction-retry-reason">{{ t("冲正失败重试原因（UTF-8 不超过 500 字节）", "Failed reversal retry reason (up to 500 UTF-8 bytes)") }}</label><textarea id="correction-retry-reason" v-model="reason" rows="2" maxlength="500" :disabled="writing || Boolean(pendingWrite)" :placeholder="t('说明重试依据', 'Describe the basis for retrying')" />
          <small>{{ reasonBytes }} / {{ t("500 字节", "500 bytes") }}</small>
          <button class="cm-primary" type="button" :disabled="!canStartRetry" @click="reviewRetry">{{ t("核对并重试失败冲正", "Review and retry failed reversals") }}</button>
        </div>
        <button v-if="selected.new_job_id" class="cm-secondary" type="button" @click="emit('open-settlement', selected.period_id)">{{ t("打开新结算任务", "Open new settlement job") }}</button>

        <div class="cm-target-heading"><h4>{{ t("逐单冲正目标", "Per-order reversal targets") }}</h4><span>{{ t("每页", "Per page") }} {{ CORRECTION_PAGE_SIZE }} {{ t("条", "items") }}</span></div>
        <p v-if="targetsBusy" class="cm-note">{{ t("正在读取冲正目标…", "Loading reversal targets…") }}</p>
        <div v-if="targetPage?.items.length" class="cm-targets">
          <article v-for="target in targetPage.items" :key="target.order_id" class="cm-target">
            <div class="cm-target-title"><strong>{{ targetStateLabel(target.state) }}</strong><span>{{ target.error_code ?? '—' }}</span></div>
            <dl class="cm-facts compact-facts">
              <div><dt>{{ t("注单 / 会员", "Order / member") }}</dt><dd class="cm-break">{{ target.order_id }} / {{ target.member_id }}</dd></div>
              <div><dt>{{ t("旧注单版本 / 状态", "Previous order version / status") }}</dt><dd>v{{ target.old_order_version }} / {{ orderStatusLabel(target.old_order_status) }}</dd></div>
              <div><dt>{{ t("旧核算 / 原派奖账本", "Previous calculation / original payout ledger") }}</dt><dd class="cm-break">{{ target.old_calculation_id ?? '—' }} / {{ target.old_payout_entry_id ?? '—' }}</dd></div>
              <div><dt>{{ t("原中奖积分 / 冲正账本 / 注单重置版本", "Original winnings / reversal ledger / reset order version") }}</dt><dd class="cm-break">{{ formatCorrectionPoints(target.old_prize_points) }} / {{ target.reversal_entry_id ?? '—' }} / {{ target.reset_order_version ?? '—' }}</dd></div>
            </dl>
          </article>
        </div>
        <p v-else-if="targetPage && !targetsBusy" class="cm-note">{{ t("此页没有冲正目标。", "No reversal targets on this page.") }}</p>
        <div v-if="targetPage" class="cm-pagination">
          <button class="cm-secondary" type="button" :disabled="targetOffset <= 0 || targetsBusy || writing" @click="readTargets(selected.id, Math.max(0, targetOffset - CORRECTION_PAGE_SIZE))">{{ t("上一页", "Previous") }}</button>
          <span>{{ t("偏移", "Offset") }} {{ targetPage.offset }} · {{ targetPage.items.length }} {{ t("条", "items") }}</span>
          <button class="cm-secondary" type="button" :disabled="!targetPage.has_more || targetsBusy || writing" @click="readTargets(selected.id, targetOffset + CORRECTION_PAGE_SIZE)">{{ t("下一页", "Next") }}</button>
        </div>
      </section>

      <div v-if="reviewMode && reviewBody && !pendingWrite" class="cm-confirm">
        <strong>{{ t("再次确认不可撤回操作", "Confirm this irreversible action") }}</strong>
        <p v-if="reviewMode === 'correct'">{{ (reviewBody as CorrectBody).policy_version !== null ? t('更正并重新结算', 'Correct and settle again') : t('只更正未结算结果', 'Correct unsettled result only') }}{{ t("。期次 v", ". Period v") }}{{ reviewBody.version }}{{ t("；策略", "; policy") }} {{ (reviewBody as CorrectBody).policy_version ?? '—' }}{{ t("；模式", "; mode") }} {{ reviewExpectedMode ?? t('无财务结算', 'No financial settlement') }}{{ t("。确认结果：普通", ". Confirm result: regular") }} {{ (reviewBody as CorrectBody).result.regular.join(', ') }}{{ t("；特别", "; special") }} {{ (reviewBody as CorrectBody).result.special.join(', ') }}{{ t("；数字", "; digits") }} {{ (reviewBody as CorrectBody).result.digits.join(', ') }}。</p>
        <p v-else>{{ t("重试更正", "Retry correction") }} {{ selected?.id }}{{ t("，使用冲正版本 v", " with reversal version v") }}{{ reviewBody.version }}。</p>
        <p>{{ t("原因：", "Reason: ") }}{{ reviewBody.reason }}</p>
        <label for="correction-confirmed" class="cm-check"><input id="correction-confirmed" v-model="reviewConfirmed" type="checkbox" />{{ t("我已核对品牌、期次、版本、结果和原因，确认提交", "I reviewed the brand, period, version, result, and reason, and confirm submission") }}</label>
        <div class="cm-review-actions"><button class="cm-secondary" type="button" :disabled="writing" @click="cancelReview">{{ t("返回修改", "Back to edit") }}</button><button class="cm-primary" type="button" :disabled="!reviewConfirmed || writing || (reviewMode === 'correct' ? !rights.correct : !rights.retry || !rights.settleRun)" @click="beginWrite">{{ writing ? t('提交中…', 'Submitting…') : t('确认提交', 'Confirm submission') }}</button></div>
      </div>
    </template>
  </section>
</template>

<style scoped>
.correction-management{display:grid;gap:16px;color:#172033}.cm-heading,.cm-section-title,.cm-target-heading,.cm-target-title,.cm-pagination,.cm-review-actions{display:flex;align-items:center;justify-content:space-between;gap:12px}.cm-heading h2{margin:2px 0 0;font-size:1.25rem}.cm-eyebrow{margin:0;color:#68758a;font-size:.78rem}.cm-warning,.cm-pending,.cm-confirm,.cm-panel{border:1px solid #d9e0ea;border-radius:12px;padding:16px;background:#fff}.cm-warning{background:#fff8e8;border-color:#e9c66a;color:#694d08}.cm-warning.compact{padding:10px;margin:8px 0}.cm-pending{background:#fff1ee;border-color:#df9385}.cm-panel{display:grid;gap:12px}.cm-section-title h3,.cm-target-heading h4{margin:0}.cm-target-heading{margin-top:8px}.cm-target-heading span,.cm-note,.cm-message,.cm-target-title span,.cm-pagination span,.cm-history-item small{color:#667085;font-size:.88rem}.cm-facts{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px 18px;margin:0}.cm-facts>div{min-width:0;padding:8px 0;border-bottom:1px solid #eef1f5}.cm-facts dt{color:#697586;font-size:.78rem}.cm-facts dd{margin:4px 0 0;font-weight:550}.cm-break{overflow-wrap:anywhere;word-break:break-word}.cm-badge{display:inline-flex;border-radius:99px;padding:4px 10px;font-size:.8rem;background:#edf0f5}.cm-badge.ready{background:#e6f6eb;color:#176b37}.cm-badge.blocked{background:#fff0ee;color:#a32e1a}.cm-badge.pending{background:#fff5dd;color:#725708}.cm-form{display:grid;gap:8px;max-width:720px}.cm-form label,.cm-period label{display:grid;gap:6px;font-size:.9rem}.cm-form textarea,.cm-form input,.cm-period input{width:100%;box-sizing:border-box;border:1px solid #cbd3df;border-radius:8px;padding:9px 10px;font:inherit;background:white}.cm-form small{color:#667085}.cm-primary,.cm-secondary{border:1px solid #c5ceda;border-radius:8px;padding:8px 12px;font:inherit;cursor:pointer;background:#fff}.cm-primary{background:#254a83;border-color:#254a83;color:#fff}.cm-primary:disabled,.cm-secondary:disabled{opacity:.52;cursor:not-allowed}.cm-message{margin:0;padding:10px 12px;border-radius:8px;background:#f0f4fa}.cm-message.error{background:#fff0ee;color:#a32e1a}.cm-note{margin:0}.cm-period{display:grid;gap:6px;max-width:720px}.cm-history-list,.cm-targets{display:grid;gap:10px}.cm-history-item{display:grid;grid-template-columns:1fr minmax(120px,1fr) auto;align-items:center;gap:12px;text-align:left;border:1px solid #e2e7ef;border-radius:10px;padding:12px;background:white;color:inherit;font:inherit;cursor:pointer}.cm-history-item span:first-child{display:grid;gap:4px}.cm-history-item.active{border-color:#254a83;background:#f1f5fb}.cm-target{border:1px solid #e2e7ef;border-radius:10px;padding:12px}.compact-facts{margin-top:8px}.compact-facts>div{padding:5px 0}.cm-check{display:flex;align-items:flex-start;gap:8px;margin:12px 0}.cm-review-actions{justify-content:flex-end}.cm-pagination{justify-content:center;flex-wrap:wrap}
@media(max-width:600px){.correction-management{gap:12px}.cm-heading{align-items:flex-start}.cm-warning,.cm-panel,.cm-confirm,.cm-pending{padding:12px}.cm-facts{grid-template-columns:minmax(0,1fr);gap:2px}.cm-section-title{align-items:flex-start}.cm-pagination .cm-secondary{flex:1}.cm-review-actions{align-items:stretch;flex-direction:column-reverse}.cm-review-actions button{width:100%}.cm-history-item{grid-template-columns:1fr;gap:6px}}
</style>
