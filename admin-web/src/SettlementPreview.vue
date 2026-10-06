<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import {
  AdminApiError,
  createIdempotencyKey,
  type AdminAccount,
} from "./admin-api";
import { createSettlementPreviewApi, settlementPreviewPermissions } from "./settlement-preview-api";
import type { AdminBetOrder } from "./bet-management-api";
import {
  createPreviewRequestLane,
  frozenSettlementPreviewWrites,
  formatIntegerPoints,
  freezePreviewIntent,
  matchesPreviewReceipt,
  previewWriteFailure,
  reasonByteLength,
  samePreviewContext,
  SETTLEMENT_PREVIEW_PAGE_SIZE,
  type SettlementPreviewRequestBody,
  validPreviewReason,
} from "./settlement-preview-state";

const props = defineProps<{
  account: AdminAccount;
  brandId: string;
  order: AdminBetOrder;
}>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createSettlementPreviewApi();
const rights = computed(() => settlementPreviewPermissions(props.account, props.brandId));
type Context = Awaited<ReturnType<typeof api.context>>;
type Preview = Awaited<ReturnType<typeof api.preview>>;
type LinesPage = Awaited<ReturnType<typeof api.lines>>;
type HistoryPage = Awaited<ReturnType<typeof api.history>>;
type CreateBody = SettlementPreviewRequestBody;
const frozenWrites = frozenSettlementPreviewWrites;

const permissionsScope = computed(() => JSON.stringify([
  props.account.id,
  props.account.super_admin,
  props.account.brand_ids,
  props.account.permissions,
  props.account.permissions_by_brand,
  props.account.platform_permissions,
  props.brandId,
  rights.value,
]));
const scope = computed(() => JSON.stringify([
  permissionsScope.value, props.order.id, props.order.brand_id, props.order.version,
  props.order.game_id, props.order.period_id, props.order.status, props.order.definition_hash,
]));
const context = ref<Context | null>(null);
const history = ref<Preview[]>([]);
const historyHasMore = ref(false);
const selectedPreview = ref<Preview | null>(null);
const selectedPreviewId = ref("");
const linesPage = ref<LinesPage | null>(null);
const historyOffset = ref(0);
const linesOffset = ref(0);
const historyBusy = ref(false);
const contextBusy = ref(false);
const linesBusy = ref(false);
const writing = ref(false);
const error = ref("");
const notice = ref("");
const reason = ref("");
const reviewBody = ref<Readonly<CreateBody> | null>(null);
const reviewConfirmed = ref(false);
const initialPendingWrite = frozenWrites.find(props.account.id, props.brandId, props.order.id);
const pendingWrite = ref(initialPendingWrite);
const unknownWrite = ref(Boolean(initialPendingWrite));
let alive = true;
const contextLane = createPreviewRequestLane();
const historyLane = createPreviewRequestLane();
const previewLane = createPreviewRequestLane();
const linesLane = createPreviewRequestLane();
const writeLane = createPreviewRequestLane();

const reasonBytes = computed(() => reasonByteLength(reason.value));
const staleOrder = computed(() => Boolean(
  context.value && context.value.order_version !== props.order.version,
));
const ruleHashMismatch = computed(() => Boolean(
  context.value && context.value.definition_hash !== props.order.definition_hash,
));
const orderStatusMismatch = computed(() => Boolean(
  context.value && context.value.order_status !== props.order.status,
));
const canStartPreview = computed(() => Boolean(
  rights.value.preview && rights.value.view && context.value?.can_preview &&
  context.value.draw_result_id && !staleOrder.value && !ruleHashMismatch.value &&
  !orderStatusMismatch.value && !writing.value &&
  !pendingWrite.value && !contextBusy.value,
));
const currentRecord = computed(() => history.value.find((item) => item.current) ?? null);

function isCurrent(
  lane: ReturnType<typeof createPreviewRequestLane>,
  ticket: number,
  capturedScope: string,
  permitted = rights.value.view,
) {
  return alive && lane.isCurrent(ticket) && scope.value === capturedScope && permitted;
}
function cancelReadLanes() {
  contextLane.invalidate();
  historyLane.invalidate();
  previewLane.invalidate();
  linesLane.invalidate();
  contextBusy.value = false;
  historyBusy.value = false;
  linesBusy.value = false;
}
function clearReadData() {
  cancelReadLanes();
  context.value = null;
  history.value = [];
  historyHasMore.value = false;
  selectedPreview.value = null;
  selectedPreviewId.value = "";
  linesPage.value = null;
  historyOffset.value = 0;
  linesOffset.value = 0;
}
function clearVisibleData() {
  clearReadData();
  writeLane.invalidate();
  error.value = "";
  notice.value = "";
  reason.value = "";
  reviewBody.value = null;
  reviewConfirmed.value = false;
  writing.value = false;
  pendingWrite.value = frozenWrites.find(props.account.id, props.brandId, props.order.id);
  unknownWrite.value = Boolean(pendingWrite.value);
}
function invalidateSession() {
  clearVisibleData();
  frozenWrites.clear();
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
function checkContext(value: Context) {
  if (!samePreviewContext(value, {
    brandId: props.brandId,
    orderId: props.order.id,
    orderVersion: value.order_version,
  })) throw new Error("核算上下文与当前品牌或注单不匹配，请重新读取。");
  if (value.game_id !== props.order.game_id || value.period_id !== props.order.period_id)
    throw new Error("核算上下文的彩种或期次与注单不匹配，请刷新注单详情。");
  if (props.order.brand_id !== props.brandId)
    throw new Error("当前注单所属品牌与页面品牌不匹配，请刷新注单详情。");
}
function checkPreview(value: Preview) {
  if (
    value.brand_id !== props.brandId || value.order_id !== props.order.id ||
    value.game_id !== props.order.game_id || value.period_id !== props.order.period_id
  ) throw new Error("预览记录与当前品牌、彩种、期次或注单不匹配。");
}
function checkCreatedPreview(value: Preview, body: Readonly<CreateBody>) {
  checkPreview(value);
  if (!matchesPreviewReceipt(value, {
    brandId: props.brandId,
    orderId: props.order.id,
    gameId: props.order.game_id,
    periodId: props.order.period_id,
    orderVersion: body.version,
    periodVersion: body.period_version,
    drawResultId: body.draw_result_id,
    definitionHash: props.order.definition_hash,
    accountId: props.account.id,
    reason: body.reason,
  })) throw new Error("预览写入响应与原始账号、注单上下文或请求内容不一致；请保留原请求并重试确认。");
}

async function readContextAndHistory(keepSelection = false) {
  if (!rights.value.view || writing.value || !props.brandId || !props.order.id) return;
  const keepId = keepSelection ? selectedPreviewId.value : "";
  clearReadData();
  const contextTicket = contextLane.begin();
  const historyTicket = historyLane.begin();
  const capturedScope = scope.value;
  contextBusy.value = true;
  historyBusy.value = true;
  error.value = "";
  notice.value = "";
  try {
    const [nextContext, nextHistory] = await Promise.all([
      api.context(props.brandId, props.order.id),
      api.history(props.brandId, props.order.id, SETTLEMENT_PREVIEW_PAGE_SIZE, 0),
    ]);
    if (!isCurrent(contextLane, contextTicket, capturedScope) ||
        !isCurrent(historyLane, historyTicket, capturedScope)) return;
    checkContext(nextContext);
    if (nextHistory.brand_id !== props.brandId || nextHistory.order_id !== props.order.id)
      throw new Error("预览记录列表与当前品牌或注单不匹配。");
    for (const item of nextHistory.items) checkPreview(item);
    context.value = nextContext;
    history.value = nextHistory.items;
    historyHasMore.value = nextHistory.has_more;
    historyOffset.value = nextHistory.offset;
    const previewId = keepId || currentRecord.value?.id || "";
    selectedPreviewId.value = previewId;
    if (previewId) void loadPreview(previewId, capturedScope);
    if (nextContext.order_version !== props.order.version)
      notice.value = "订单版本已变化。请刷新注单详情；本组件不会更新父级注单数据，也不会提交核算预览。";
    else if (nextContext.definition_hash !== props.order.definition_hash)
      notice.value = "服务端核算规则 hash 与注单保存的规则 hash 不一致；已阻止创建预览，请刷新注单详情并核对。";
    else if (nextContext.order_status !== props.order.status)
      notice.value = "服务端注单状态与页面注单状态不一致；已阻止创建预览，请刷新注单详情。";
    else notice.value = "已读取服务端核算上下文与历史预览记录。";
  } catch (problem) {
    if (isCurrent(contextLane, contextTicket, capturedScope) &&
        isCurrent(historyLane, historyTicket, capturedScope)) {
      clearReadData();
      handleError(problem);
    }
  } finally {
    if (isCurrent(contextLane, contextTicket, capturedScope, true)) contextBusy.value = false;
    if (isCurrent(historyLane, historyTicket, capturedScope, true)) historyBusy.value = false;
  }
}

async function loadPreview(id: string, capturedScope = scope.value) {
  if (!rights.value.view || writing.value || selectedPreviewId.value !== id) return;
  const ticket = previewLane.begin();
  linesLane.invalidate();
  linesBusy.value = false;
  linesPage.value = null;
  linesOffset.value = 0;
  try {
    const result = await api.preview(props.brandId, id);
    if (!isCurrent(previewLane, ticket, capturedScope) || selectedPreviewId.value !== id) return;
    checkPreview(result);
    selectedPreview.value = result;
    await loadLines(0, id, capturedScope);
  } catch (problem) {
    if (isCurrent(previewLane, ticket, capturedScope) && selectedPreviewId.value === id) {
      selectedPreview.value = null;
      linesPage.value = null;
      handleError(problem);
    }
  }
}

async function readHistoryPage(offset: number) {
  if (!rights.value.view || historyBusy.value || writing.value) return;
  const ticket = historyLane.begin();
  const capturedScope = scope.value;
  historyBusy.value = true;
  try {
    const result: HistoryPage = await api.history(
      props.brandId,
      props.order.id,
      SETTLEMENT_PREVIEW_PAGE_SIZE,
      offset,
    );
    if (!isCurrent(historyLane, ticket, capturedScope)) return;
    if (result.brand_id !== props.brandId || result.order_id !== props.order.id)
      throw new Error("预览记录列表与当前品牌或注单不匹配。");
    for (const item of result.items) checkPreview(item);
    history.value = result.items;
    historyOffset.value = result.offset;
    historyHasMore.value = result.has_more;
  } catch (problem) {
    if (isCurrent(historyLane, ticket, capturedScope)) {
      clearReadData();
      handleError(problem);
    }
  } finally {
    if (isCurrent(historyLane, ticket, capturedScope, true)) historyBusy.value = false;
  }
}

async function loadLines(
  offset = 0,
  previewId = selectedPreview.value?.id,
  capturedScope = scope.value,
  allowDuringWrite = false,
) {
  if (!rights.value.view || !previewId || (writing.value && !allowDuringWrite)) return;
  const ticket = linesLane.begin();
  linesBusy.value = true;
  try {
    const result = await api.lines(props.brandId, previewId, SETTLEMENT_PREVIEW_PAGE_SIZE, offset);
    if (!isCurrent(linesLane, ticket, capturedScope) || selectedPreviewId.value !== previewId) return;
    if (result.brand_id !== props.brandId || result.preview_id !== previewId || result.order_id !== props.order.id)
      throw new Error("核算明细与当前品牌、注单或预览记录不匹配。");
    linesPage.value = result;
    linesOffset.value = result.offset;
  } catch (problem) {
    if (isCurrent(linesLane, ticket, capturedScope) && selectedPreviewId.value === previewId) {
      linesPage.value = null;
      handleError(problem);
    }
  } finally {
    if (isCurrent(linesLane, ticket, capturedScope, true)) linesBusy.value = false;
  }
}

function selectHistory(item: Preview) {
  if (writing.value) return;
  selectedPreviewId.value = item.id;
  selectedPreview.value = null;
  previewLane.invalidate();
  linesLane.invalidate();
  linesPage.value = null;
  linesOffset.value = 0;
  linesBusy.value = false;
  error.value = "";
  void loadPreview(item.id, scope.value);
}
function reviewPreview() {
  const currentContext = context.value;
  const cleanReason = reason.value.trim();
  if (!currentContext || !rights.value.preview) return;
  if (currentContext.order_version !== props.order.version) {
    error.value = "订单版本与服务端核算上下文不一致。请刷新注单详情后再试。";
    return;
  }
  if (currentContext.definition_hash !== props.order.definition_hash) {
    error.value = "服务端核算规则 hash 与注单保存的规则 hash 不一致。请刷新注单详情并核对。";
    return;
  }
  if (currentContext.order_status !== props.order.status) {
    error.value = "服务端注单状态与页面注单状态不一致。请刷新注单详情。";
    return;
  }
  if (!currentContext.can_preview || !currentContext.draw_result_id) {
    error.value = "当前开奖或期次状态不允许预览，请重新读取核算上下文。";
    return;
  }
  if (!validPreviewReason(cleanReason)) {
    error.value = "原因不能为空，且 UTF-8 长度不能超过 500 字节。";
    return;
  }
  reviewBody.value = Object.freeze({
    version: currentContext.order_version,
    period_version: currentContext.period_version,
    draw_result_id: currentContext.draw_result_id,
    reason: cleanReason,
  });
  reviewConfirmed.value = false;
  error.value = "";
}
async function refreshAfterWrite(
  receipt: Preview,
  ticket: number,
  capturedScope: string,
  body: Readonly<CreateBody>,
): Promise<"current" | "outdated" | "stale"> {
  const [nextContext, nextHistory, freshPreview] = await Promise.all([
    api.context(props.brandId, props.order.id),
    api.history(props.brandId, props.order.id, SETTLEMENT_PREVIEW_PAGE_SIZE, 0),
    api.preview(props.brandId, receipt.id),
  ]);
  if (!isCurrent(writeLane, ticket, capturedScope, rights.value.preview)) return "stale";
  checkContext(nextContext);
  checkCreatedPreview(freshPreview, body);
  if (freshPreview.id !== receipt.id)
    throw new Error("重新读取的预览 ID 与已确认回执不一致。");
  if (nextHistory.brand_id !== props.brandId || nextHistory.order_id !== props.order.id)
    throw new Error("预览记录列表与当前品牌或注单不匹配。");
  for (const item of nextHistory.items) checkPreview(item);
  context.value = nextContext;
  history.value = nextHistory.items;
  historyHasMore.value = nextHistory.has_more;
  historyOffset.value = nextHistory.offset;
  selectedPreview.value = freshPreview;
  selectedPreviewId.value = freshPreview.id;
  linesPage.value = null;
  linesOffset.value = 0;
  reviewBody.value = null;
  reviewConfirmed.value = false;
  reason.value = "";
  await loadLines(0, freshPreview.id, capturedScope, true);
  return freshPreview.current ? "current" : "outdated";
}
async function submitPending() {
  const intent = pendingWrite.value;
  if (
    !intent || writing.value || intent.accountId !== props.account.id ||
    intent.brandId !== props.brandId || intent.orderId !== props.order.id ||
    !rights.value.preview || !rights.value.view
  ) return;
  writing.value = true;
  error.value = "";
  notice.value = unknownWrite.value ? "正在用原请求内容和幂等键重试…" : "正在保存只读核算预览…";
  cancelReadLanes();
  selectedPreview.value = null;
  selectedPreviewId.value = "";
  linesPage.value = null;
  linesOffset.value = 0;
  const ticket = writeLane.begin();
  const capturedScope = scope.value;
  const expectedBody = intent.body;
  let result: Preview;
  try {
    result = await api.create(intent.brandId, intent.orderId, intent.body, intent.key, intent.accountId);
  } catch (problem) {
    if (!isCurrent(writeLane, ticket, capturedScope, rights.value.preview)) return;
    const status = problem instanceof AdminApiError ? problem.status : undefined;
    const failure = previewWriteFailure(status);
    if (status === 401) {
      frozenWrites.forget(intent);
      invalidateSession();
      return;
    }
    if (failure === "unknown") {
      frozenWrites.remember(intent);
      pendingWrite.value = intent;
      unknownWrite.value = true;
      notice.value = "请求结果未知。原请求体和幂等键已保留；只能原样重试。读取历史中出现同一记录不能证明本次请求已成功。";
    } else {
      frozenWrites.forget(intent);
      pendingWrite.value = null;
      unknownWrite.value = false;
      reviewBody.value = null;
      reviewConfirmed.value = false;
      notice.value = failure === "conflict"
        ? "版本冲突。已清除旧请求；请重新读取上下文并人工核对后再创建预览。"
        : "服务端已明确拒绝请求。请核对原因、权限及上下文后重新读取。";
    }
    handleError(problem);
    writing.value = false;
    return;
  }
  if (!isCurrent(writeLane, ticket, capturedScope, rights.value.preview)) return;
  try {
    checkCreatedPreview(result, expectedBody);
  } catch (problem) {
    frozenWrites.remember(intent);
    pendingWrite.value = intent;
    unknownWrite.value = true;
    notice.value = "已收到但无法核实匹配关系的响应。原请求体和幂等键已保留，请原样重试；不会把此响应当作已确认预览。";
    handleError(problem);
    writing.value = false;
    return;
  }

  // A qualified create receipt is definitive. Remove the frozen intent before any follow-up GET.
  frozenWrites.forget(intent);
  pendingWrite.value = null;
  unknownWrite.value = false;
  try {
    const refreshed = await refreshAfterWrite(result, ticket, capturedScope, expectedBody);
    if (refreshed !== "stale" && isCurrent(writeLane, ticket, capturedScope, true)) {
      notice.value = refreshed === "current"
        ? "预览已保存并重新读取为当前记录。它只用于核对，不会派奖或改变注单、期次状态。"
        : "预览已保存；重新读取后该记录已过时，不可用于新结算。它不会派奖或改变注单、期次状态。";
    }
  } catch (problem) {
    if (isCurrent(writeLane, ticket, capturedScope, rights.value.preview)) {
      if (problem instanceof AdminApiError && problem.status === 401) {
        // The write receipt is already acknowledged; clear the expired session
        // without recreating an unknown intent for that completed write.
        invalidateSession();
        return;
      }
      clearReadData();
      error.value = problem instanceof Error ? problem.message : "重新读取失败。";
      notice.value = "预览已由服务端确认保存，但后续读取失败。请求不会再次冻结为未知；请手动重新读取。";
    }
  } finally {
    if (isCurrent(writeLane, ticket, capturedScope, true)) writing.value = false;
  }
}
function confirmPreview() {
  if (!reviewBody.value || !reviewConfirmed.value || pendingWrite.value || writing.value) return;
  const intent = freezePreviewIntent({
    accountId: props.account.id,
    brandId: props.brandId,
    orderId: props.order.id,
    body: reviewBody.value as CreateBody,
    key: createIdempotencyKey(),
  });
  pendingWrite.value = intent;
  frozenWrites.remember(intent);
  unknownWrite.value = false;
  void submitPending();
}
function formatPoints(value: string | null | undefined) {
  return value == null ? "—" : formatIntegerPoints(value);
}
function resultLabel(outcome: Preview["outcome"]) {
  return ({ won: "命中（计算结果）", lost: "未命中（计算结果）", abnormal: "异常：不进行普通结算", excluded: "排除：不进行普通结算" })[outcome];
}
function resultClass(outcome: Preview["outcome"]) {
  return outcome === "won" ? "good" : outcome === "lost" ? "quiet" : "alert";
}
function drawText(draw: Context["draw"]) {
  if (!draw) return "—";
  return JSON.stringify(draw);
}
function stringify(value: unknown) {
  return JSON.stringify(value ?? null, null, 2);
}

watch(scope, () => clearVisibleData(), { flush: "sync" });
watch(
  () => [rights.value.view, rights.value.preview] as const,
  ([canView]) => {
    if (!canView) clearVisibleData();
  },
  { flush: "sync" },
);
watch(reason, () => {
  if (!pendingWrite.value) {
    reviewBody.value = null;
    reviewConfirmed.value = false;
  }
});
onMounted(() => {
  if (rights.value.view) void readContextAndHistory();
});
onBeforeUnmount(() => {
  alive = false;
  contextLane.invalidate();
  historyLane.invalidate();
  previewLane.invalidate();
  linesLane.invalidate();
  writeLane.invalidate();
});
</script>

<template>
  <section class="settlement-preview" :aria-labelledby="`settlement-preview-title-${order.id}`">
    <header class="sp-heading">
      <div>
        <p class="sp-eyebrow">注单详情 · 只读核算</p>
        <h3 :id="`settlement-preview-title-${order.id}`">结算核算预览</h3>
      </div>
      <button v-if="rights.view" class="sp-secondary" type="button" :disabled="contextBusy || historyBusy || writing" @click="readContextAndHistory(true)">
        {{ contextBusy || historyBusy ? "读取中…" : "重新读取" }}
      </button>
    </header>
    <p class="sp-warning" role="note"><strong>不派奖、不改变注单或期次状态，不代表已结算。</strong>本组件没有派奖或真实结算操作；预览结果金额不会计入任何账户。</p>
    <p v-if="!rights.view" class="sp-note">当前账号没有查看核算预览的权限。</p>
    <template v-else>
      <div class="sp-actions">
        <button class="sp-secondary" type="button" :disabled="contextBusy || historyBusy || writing" @click="readContextAndHistory()">{{ contextBusy || historyBusy ? "读取中…" : "读取核算上下文与记录" }}</button>
      </div>
      <p v-if="error" class="sp-message error" role="alert">{{ error }}</p>
      <p v-if="notice" class="sp-message" role="status">{{ notice }}</p>
      <p v-if="!rights.preview" class="sp-note">当前账号仅可查看核算上下文和预览记录，没有创建预览的权限。</p>
      <div v-if="rights.preview && pendingWrite" class="sp-pending">
        <strong>{{ unknownWrite ? "预览写入结果未知" : "预览请求待提交" }}</strong>
        <p>{{ unknownWrite ? "原请求体和幂等键仍在本标签页内存中。请用下方按钮重试原请求；查看到相同历史记录不算作请求成功。" : "已冻结待确认请求。核对内容后再继续。" }}</p>
        <button class="sp-primary" type="button" :disabled="writing || pendingWrite.accountId !== account.id || pendingWrite.brandId !== brandId || pendingWrite.orderId !== order.id" @click="submitPending">{{ writing ? "按原请求提交中…" : "使用同一请求与幂等键重试" }}</button>
      </div>
      <div v-if="context" class="sp-panel">
        <div class="sp-section-title"><h4>服务端核算依据</h4><span class="sp-badge" :class="context.can_preview && !staleOrder && !ruleHashMismatch && !orderStatusMismatch ? 'ready' : 'blocked'">{{ context.can_preview && !staleOrder && !ruleHashMismatch && !orderStatusMismatch ? "允许预览" : "暂不可预览" }}</span></div>
        <dl class="sp-facts">
          <div><dt>注单状态 / 服务端版本</dt><dd>{{ context.order_status }} / v{{ context.order_version }}</dd></div>
          <div><dt>期次状态 / 实际期次版本</dt><dd>{{ context.period_status }} / v{{ context.period_version }}</dd></div>
          <div><dt>开奖记录 ID</dt><dd class="sp-break">{{ context.draw_result_id || "—" }}</dd></div>
          <div><dt>实际开奖</dt><dd class="sp-break">{{ drawText(context.draw) }}</dd></div>
          <div><dt>开奖内容 hash</dt><dd class="sp-break">{{ context.draw_hash || "—" }}</dd></div>
          <div><dt>服务端核算规则 hash</dt><dd class="sp-break">{{ context.definition_hash }}</dd></div>
          <div><dt>注单保存规则 hash</dt><dd class="sp-break">{{ order.definition_hash }}</dd></div>
        </dl>
        <p v-if="context.order_version !== order.version" class="sp-warning compact">当前页面注单为 v{{ order.version }}，服务端上下文为 v{{ context.order_version }}。请刷新注单详情；预览已阻止。</p>
        <p v-if="ruleHashMismatch" class="sp-warning compact">服务端核算规则 hash 与注单保存的规则 hash 不一致；预览已阻止，请刷新注单详情并核对。</p>
        <p v-if="orderStatusMismatch" class="sp-warning compact">服务端注单状态与页面注单状态不一致；预览已阻止，请刷新注单详情。</p>
        <template v-if="rights.preview">
          <div v-if="!pendingWrite" class="sp-form">
            <label>核对原因（UTF-8 不超过 500 字节）
              <textarea v-model="reason" rows="2" maxlength="500" :disabled="!canStartPreview" placeholder="说明本次只读核算的核对目的" />
            </label>
            <small>{{ reasonBytes }} / 500 字节</small>
            <button class="sp-primary" type="button" :disabled="!canStartPreview || !validPreviewReason(reason)" @click="reviewPreview">核对本次预览</button>
            <div v-if="reviewBody" class="sp-confirm">
              <strong>请确认将仅保存一条核算预览</strong>
              <p>订单 v{{ reviewBody.version }} · 期次 v{{ reviewBody.period_version }} · 开奖记录 {{ reviewBody.draw_result_id }}</p>
              <p>原因：{{ reviewBody.reason }}</p>
              <label class="sp-check"><input v-model="reviewConfirmed" type="checkbox" />我已核对上方开奖、版本、规则 hash 与原因，确认只创建预览记录</label>
              <button class="sp-primary" type="button" :disabled="!reviewConfirmed || writing" @click="confirmPreview">{{ writing ? "正在保存预览…" : "确认保存只读预览" }}</button>
            </div>
          </div>
        </template>
      </div>

      <div v-if="history.length || currentRecord" class="sp-panel">
        <div class="sp-section-title"><h4>预览记录</h4><span class="sp-note-inline">每页 {{ SETTLEMENT_PREVIEW_PAGE_SIZE }} 条</span></div>
        <div class="sp-history">
          <button v-for="item in history" :key="item.id" class="sp-history-item" :class="{ selected: selectedPreviewId === item.id }" type="button" :disabled="writing" @click="selectHistory(item)">
            <span>{{ item.created_at }}</span><b>{{ resultLabel(item.outcome) }}</b><small>{{ item.current ? "当前记录" : "过时记录 · 不可用于新结算" }}</small>
          </button>
        </div>
        <div class="sp-pagination"><button class="sp-secondary" type="button" :disabled="historyOffset <= 0 || historyBusy || writing" @click="readHistoryPage(Math.max(0, historyOffset - SETTLEMENT_PREVIEW_PAGE_SIZE))">上一页</button><button class="sp-secondary" type="button" :disabled="!historyHasMore || historyBusy || writing" @click="readHistoryPage(historyOffset + SETTLEMENT_PREVIEW_PAGE_SIZE)">下一页</button></div>
      </div>

      <div v-if="selectedPreview" class="sp-panel">
        <div class="sp-section-title"><h4>核算结果</h4><span class="sp-badge" :class="resultClass(selectedPreview.outcome)">{{ resultLabel(selectedPreview.outcome) }}</span></div>
        <p v-if="!selectedPreview.current" class="sp-warning compact">这是过时预览记录，不可用于新结算。</p>
        <p v-if="selectedPreview.outcome === 'abnormal' || selectedPreview.outcome === 'excluded'" class="sp-warning compact">此记录不进行普通结算；其中没有可派奖金额。</p>
        <dl class="sp-facts">
          <div><dt>预览 ID</dt><dd class="sp-break">{{ selectedPreview.id }}</dd></div>
          <div><dt>记录状态</dt><dd>{{ selectedPreview.current ? "当前" : "过时 · 不可用于新结算" }}</dd></div>
          <div><dt>错误码</dt><dd>{{ selectedPreview.error_code || "—" }}</dd></div>
          <div><dt>计算是否命中 / 组合数</dt><dd>{{ selectedPreview.calculation ? `${selectedPreview.calculation.won ? "是" : "否"} / ${selectedPreview.calculation.combination_count}` : "—" }}</dd></div>
          <div><dt>倍数</dt><dd>{{ selectedPreview.calculation?.multiplier ?? "—" }}</dd></div>
          <div><dt>投注积分（仅核算数据）</dt><dd>{{ formatPoints(selectedPreview.calculation?.bet_points) }}</dd></div>
          <div><dt>原始计算积分（不入账）</dt><dd>{{ formatPoints(selectedPreview.calculation?.raw_prize_points) }}</dd></div>
          <div><dt>封顶计算积分（不入账）</dt><dd>{{ formatPoints(selectedPreview.calculation?.capped_prize_points) }}</dd></div>
          <div><dt>结果积分（不入账）</dt><dd>{{ formatPoints(selectedPreview.calculation?.prize_points) }}</dd></div>
          <div><dt>规则 hash / 开奖 hash</dt><dd class="sp-break">{{ selectedPreview.definition_hash }} / {{ selectedPreview.draw_hash || "—" }}</dd></div>
          <div><dt>操作人与时间</dt><dd class="sp-break">{{ selectedPreview.created_by }} · {{ selectedPreview.created_at }}</dd></div>
          <div><dt>原因 / 审计记录</dt><dd class="sp-break">{{ selectedPreview.reason }} · {{ selectedPreview.audit_log_id }}</dd></div>
        </dl>
        <div class="sp-section-title sp-lines-heading"><h4>规则逐注明细</h4><span class="sp-note-inline">每页 {{ SETTLEMENT_PREVIEW_PAGE_SIZE }} 条；命中 trace 按需展开</span></div>
        <p v-if="linesBusy" class="sp-note">正在读取明细…</p>
        <p v-else-if="linesPage" class="sp-note">共 {{ linesPage.total }} 注，当前显示 {{ linesPage.items.length }} 注。</p>
        <div v-if="linesPage" class="sp-lines">
          <article v-for="(line, index) in linesPage.items" :key="`${linesOffset}-${index}`" class="sp-line">
            <div class="sp-line-head"><b>第 {{ linesOffset + index + 1 }} 注</b><strong>{{ formatPoints(line.points) }} 积分</strong></div>
            <details><summary>查看选择与规则命中（{{ line.hits.length }} 项）</summary><pre>{{ stringify(line.selection) }}</pre><div v-for="(hit, hitIndex) in line.hits" :key="`${hit.code}-${hitIndex}`" class="sp-hit"><b>{{ hit.code }} · {{ hit.exclusive ? "排他" : "普通" }} · {{ hit.matched ? "命中" : "未命中" }}</b><span>选择 {{ hit.selected }} · 实际 {{ stringify(hit.trace.actual) }} · 原始 {{ formatPoints(hit.raw_points) }} · 封顶 {{ formatPoints(hit.capped_points) }} · 采用 {{ formatPoints(hit.points) }}</span><details><summary>规则 trace</summary><pre>{{ stringify(hit.trace) }}</pre></details></div></details>
          </article>
        </div>
        <div v-if="linesPage" class="sp-pagination"><button class="sp-secondary" type="button" :disabled="linesOffset <= 0 || linesBusy || writing" @click="loadLines(Math.max(0, linesOffset - SETTLEMENT_PREVIEW_PAGE_SIZE))">上一页明细</button><button class="sp-secondary" type="button" :disabled="linesOffset + linesPage.items.length >= linesPage.total || linesBusy || writing" @click="loadLines(linesOffset + SETTLEMENT_PREVIEW_PAGE_SIZE)">下一页明细</button></div>
      </div>
    </template>
  </section>
</template>

<style scoped>
.settlement-preview { min-width: 0; display: grid; gap: 12px; color: #252a36; }
.sp-heading, .sp-section-title, .sp-pagination, .sp-line-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; min-width: 0; }
.sp-heading h3, .sp-section-title h4 { margin: 0; font-size: 14px; }
.sp-eyebrow { margin: 0 0 3px; color: #8991a0; font-size: 10px; }
.sp-panel { min-width: 0; padding: 12px; border: 1px solid #e9ebf0; border-radius: 8px; background: #fff; }
.sp-warning, .sp-note, .sp-pending, .sp-confirm { margin: 0; padding: 10px; border-radius: 7px; background: #fff7e8; color: #704d18; line-height: 1.55; overflow-wrap: anywhere; }
.sp-warning.compact { margin-top: 10px; padding: 8px; }
.sp-note { background: #f5f6f9; color: #687083; }
.sp-actions { display: flex; flex-wrap: wrap; gap: 8px; }
.sp-primary, .sp-secondary { min-height: 34px; max-width: 100%; padding: 7px 11px; border: 1px solid #dfe2e9; border-radius: 6px; background: #fff; color: #384154; font: inherit; cursor: pointer; }
.sp-primary { border-color: #5969df; background: #5969df; color: white; }
.sp-primary:disabled, .sp-secondary:disabled { opacity: .5; cursor: not-allowed; }
.sp-message { margin: 0; padding: 9px; border-radius: 6px; background: #eef1ff; color: #4758b9; overflow-wrap: anywhere; }
.sp-message.error { background: #fff0ef; color: #a3332d; }
.sp-badge { flex: 0 0 auto; padding: 4px 7px; border-radius: 999px; background: #edf0f5; color: #687083; font-size: 10px; }
.sp-badge.ready, .sp-badge.good { background: #e8f5ef; color: #25734e; }
.sp-badge.blocked, .sp-badge.alert { background: #fff0ef; color: #a3332d; }
.sp-badge.quiet { background: #f0f2f5; color: #596174; }
.sp-facts { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; margin: 12px 0 0; }
.sp-facts div { min-width: 0; padding: 8px; border-radius: 6px; background: #f8f9fb; }
.sp-facts dt { margin-bottom: 4px; color: #788092; font-size: 10px; }
.sp-facts dd { margin: 0; font-size: 12px; overflow-wrap: anywhere; word-break: break-word; }
.sp-break { word-break: break-all; }
.sp-form { display: grid; gap: 8px; margin-top: 12px; }
.sp-form label { display: grid; gap: 5px; font-size: 11px; }
.sp-form textarea { width: 100%; min-width: 0; resize: vertical; padding: 8px; border: 1px solid #dfe2e9; border-radius: 6px; }
.sp-form small, .sp-note-inline { color: #8790a0; font-size: 10px; }
.sp-confirm, .sp-pending { display: grid; gap: 8px; background: #f5f6ff; color: #3f4c94; }
.sp-confirm p, .sp-pending p { margin: 0; overflow-wrap: anywhere; }
.sp-check { display: flex !important; align-items: flex-start; }
.sp-check input { flex: 0 0 auto; margin: 2px 0 0; }
.sp-history { display: grid; gap: 6px; margin-top: 10px; }
.sp-history-item { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 4px 8px; min-width: 0; padding: 8px; border: 1px solid #e9ebf0; border-radius: 6px; background: #fff; text-align: left; overflow-wrap: anywhere; }
.sp-history-item.selected { border-color: #8390ed; background: #f7f8ff; }
.sp-history-item span { color: #788092; font-size: 10px; }
.sp-history-item b { font-size: 11px; }
.sp-history-item small { grid-column: 1 / -1; color: #8790a0; }
.sp-pagination { justify-content: flex-start; flex-wrap: wrap; margin-top: 9px; }
.sp-lines-heading { margin-top: 16px; }
.sp-lines { display: grid; gap: 7px; margin-top: 8px; }
.sp-line { min-width: 0; padding: 9px; border: 1px solid #e9ebf0; border-radius: 6px; }
.sp-line-head { margin-bottom: 7px; font-size: 11px; }
.sp-hit { display: grid; gap: 4px; padding: 8px 0; border-top: 1px solid #eceef2; overflow-wrap: anywhere; }
.sp-hit span { color: #697184; font-size: 11px; }
details { min-width: 0; }
summary { color: #4e5fc9; cursor: pointer; font-size: 11px; }
pre { max-width: 100%; margin: 7px 0; padding: 8px; border-radius: 5px; background: #f5f6f8; white-space: pre-wrap; overflow-wrap: anywhere; word-break: break-word; font: 10px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace; }
@media (max-width: 480px) {
  .sp-panel { padding: 9px; }
  .sp-facts { grid-template-columns: minmax(0, 1fr); }
  .sp-history-item { grid-template-columns: minmax(0, 1fr); }
  .sp-history-item small { grid-column: auto; }
}
</style>
