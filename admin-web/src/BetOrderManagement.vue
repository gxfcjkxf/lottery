<script setup lang="ts">
import type { LocalizedMessage } from "@lottery/shared";
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useAdminI18n } from "./i18n";
import { AdminApiError, type AdminAccount } from "./admin-api";
import SettlementPreview from "./SettlementPreview.vue";
import {
  betManagementPermissions,
  createBetManagementApi,
  createBetMutationKeyTracker,
  type ActionBody,
  type AdminBetOrder,
  type BetException,
  type JudgeCancelBody,
  type Judgment,
} from "./bet-management-api";
import {
  BET_COMBINATION_PREVIEW_LIMIT,
  BET_ORDER_PAGE_SIZE,
  combinationPreview,
  createFrozenBetMutationStore,
  freezeBetMutation,
  hasMatchingJudgmentWitness,
  isUuid,
  reasonByteLength,
  settleBetMutationFailure,
  validBetReason,
  type FrozenBetMutation,
} from "./bet-order-state";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, message: localized } = useAdminI18n();
const api = createBetManagementApi();
const keyFor = createBetMutationKeyTracker();
type BetOrderActionBody = ActionBody | JudgeCancelBody;
const frozenOperations = createFrozenBetMutationStore<BetOrderActionBody>();
const rights = computed(() =>
  betManagementPermissions(props.account, props.brandId),
);
const context = computed(() =>
  JSON.stringify([
    props.account.id,
    props.account.super_admin,
    props.account.permissions,
    props.account.permissions_by_brand,
    props.account.platform_permissions,
    props.brandId,
    rights.value,
  ]),
);

const memberQuery = ref("");
const orderQuery = ref("");
const orders = ref<AdminBetOrder[]>([]);
const offset = ref(0);
const hasNext = ref(false);
const selectedOrderId = ref("");
const selectedOrder = ref<AdminBetOrder | null>(null);
const exception = ref<BetException | null>(null);
const judgment = ref<Judgment | null>(null);
const judgmentRead = ref(false);
const listBusy = ref(false);
const detailBusy = ref(false);
const writeBusy = ref(false);
const listError = ref<string | LocalizedMessage>("");
const detailError = ref<string | LocalizedMessage>("");
const writeError = ref<string | LocalizedMessage>("");
const conflictNotice = ref<string | LocalizedMessage>("");
const notice = ref<string | LocalizedMessage>("");
const directLookup = ref(false);
const intent = ref<{
  action: "cancel" | "mark-abnormal" | "judge-cancel";
  body: BetOrderActionBody;
} | null>(null);
const confirmed = ref(false);
const reason = ref("");
const judgmentCause = ref<"no_result" | "invalid_result">("no_result");
const pending = ref<FrozenBetMutation<BetOrderActionBody> | null>(null);
const pendingUnknown = ref(false);
const readGeneration = ref(0);
const detailGeneration = ref(0);
const writeGeneration = ref(0);
let live = true;

const canCancelSelected = computed(
  () =>
    rights.value.cancel &&
    ["placed", "abnormal"].includes(selectedOrder.value?.status ?? ""),
);
const canJudgeCancelSelected = computed(
  () =>
    rights.value.judgeCancel &&
    ["placed", "abnormal"].includes(selectedOrder.value?.status ?? ""),
);
const canMarkSelected = computed(
  () => rights.value.markAbnormal && selectedOrder.value?.status === "placed",
);
const reasonBytes = computed(() => reasonByteLength(reason.value));
const amountRows = computed(() => {
  const order = selectedOrder.value;
  if (!order) return [];
  return [
    ["单注积分", order.unit_points],
    ["组合数", String(order.combination_count)],
    ["倍数", order.multiplier],
    ["总扣款", order.total_points],
    ["已结算中奖积分", order.settled_at ? (order.prize_points ?? "0") : t("未结算", "Not settled")],
    ["结算核算 ID", order.settlement_calculation_id ?? "—"],
    ["派奖账本 ID", order.payout_entry_id ?? "—"],
  ];
});
const preview = computed(() =>
  combinationPreview(selectedOrder.value?.expanded_bets ?? []),
);

function statusName(status: string) {
  return (
    {
      placed: t("待开奖", "Awaiting draw"),
      abnormal: t("异常注单", "Abnormal order"),
      bet_cancelled: t("投注取消", "Betting cancelled"),
      judged_cancelled: t("判定取消", "Judgment cancelled"),
      won: t("已中奖", "Won"),
      lost: t("未中奖", "Lost"),
    }[status] ?? status
  );
}
function statusClass(status: string) {
  if (status === "abnormal") return "danger";
  if (status === "won") return "success";
  if (status === "placed") return "waiting";
  return "neutral";
}
function amountLabel(label: string) {
  const english: Record<string, string> = {
    "单注积分": "Points per bet", "组合数": "Combination count", "倍数": "Multiplier",
    "总扣款": "Total debit", "已结算中奖积分": "Settled winnings",
    "结算核算 ID": "Settlement calculation ID", "派奖账本 ID": "Payout ledger ID",
  };
  return english[label] ? t(label, english[label]) : label;
}
function stringify(value: unknown) {
  return JSON.stringify(value ?? null, null, 2);
}
function clearVisibleData() {
  orders.value = [];
  selectedOrder.value = null;
  exception.value = null;
  judgment.value = null;
  judgmentRead.value = false;
  selectedOrderId.value = "";
  offset.value = 0;
  hasNext.value = false;
  directLookup.value = false;
  intent.value = null;
  confirmed.value = false;
  reason.value = "";
  listError.value = "";
  detailError.value = "";
  writeError.value = "";
  conflictNotice.value = "";
  notice.value = "";
  listBusy.value = false;
  detailBusy.value = false;
  writeBusy.value = false;
  pending.value = null;
  pendingUnknown.value = false;
}
function rememberFrozen(operation: FrozenBetMutation<BetOrderActionBody>) {
  frozenOperations.remember(props.account.id, operation);
}
function forgetFrozen(operation: FrozenBetMutation<BetOrderActionBody>) {
  frozenOperations.forget(props.account.id, operation);
}
function findFrozen(orderId: string) {
  return frozenOperations.find(props.account.id, props.brandId, orderId);
}
function resetSession() {
  readGeneration.value++;
  detailGeneration.value++;
  writeGeneration.value++;
  clearVisibleData();
  emit("session-invalid");
}
function contextCurrent(
  snapshot: string,
  generation: number,
  lane: "list" | "detail" | "write",
) {
  return (
    live &&
    context.value === snapshot &&
    (lane === "list"
      ? readGeneration.value
      : lane === "detail"
        ? detailGeneration.value
        : writeGeneration.value) === generation
  );
}
function checkOrder(
  order: AdminBetOrder,
  brandId: string,
  orderId?: string,
  memberId?: string,
) {
  if (
    order.brand_id !== brandId ||
    (orderId && order.id !== orderId) ||
    (memberId && order.brand_member_id !== memberId)
  )
    throw new Error(t("响应注单与当前品牌、成员或注单编号不匹配，请重新读取。", "The order response does not match the current brand, member, or order ID. Reload and try again."));
}
function syncVisibleOrder(order: AdminBetOrder) {
  orders.value = orders.value.map((item) =>
    item.id === order.id &&
    item.brand_id === order.brand_id &&
    item.version <= order.version
      ? order
      : item,
  );
}
function onReadError(cause: unknown, lane: "list" | "detail") {
  if (cause instanceof AdminApiError && cause.status === 401) {
    resetSession();
    return;
  }
  if (
    cause instanceof AdminApiError &&
    cause.status === 404 &&
    lane === "detail"
  ) {
    selectedOrder.value = null;
    exception.value = null;
    judgment.value = null;
    judgmentRead.value = false;
    detailError.value = localized("注单不存在或已不可见，请刷新列表。", "The order does not exist or is no longer visible. Refresh the list.");
    return;
  }
  const message = cause instanceof Error ? cause.message : localized("请求失败，请重试。", "Request failed. Please try again.");
  if (lane === "list") listError.value = message;
  else detailError.value = message;
}

async function loadOrders(nextOffset = 0) {
  const memberId = memberQuery.value.trim();
  if (memberId && !isUuid(memberId)) {
    listError.value = localized("成员编号必须是有效 UUID。", "Member ID must be a valid UUID.");
    return;
  }
  readGeneration.value++;
  const generation = readGeneration.value;
  const snapshot = context.value;
  const brandId = props.brandId;
  if (!rights.value.ordersView || !brandId) {
    clearVisibleData();
    return;
  }
  listBusy.value = true;
  listError.value = "";
  notice.value = "";
  try {
    const result = await api.getOrders(
      brandId,
      memberId,
      BET_ORDER_PAGE_SIZE,
      nextOffset,
    );
    if (
      !contextCurrent(snapshot, generation, "list") ||
      !rights.value.ordersView
    )
      return;
    result.items.forEach((item) =>
      checkOrder(item, brandId, undefined, memberId),
    );
    orders.value = result.items;
    offset.value = nextOffset;
    hasNext.value = result.items.length === BET_ORDER_PAGE_SIZE;
    directLookup.value = false;
    if (
      selectedOrderId.value &&
      !result.items.some((item) => item.id === selectedOrderId.value)
    ) {
      selectedOrderId.value = "";
      selectedOrder.value = null;
      exception.value = null;
    }
  } catch (cause) {
    if (contextCurrent(snapshot, generation, "list")) {
      orders.value = [];
      onReadError(cause, "list");
    }
  } finally {
    if (contextCurrent(snapshot, generation, "list")) listBusy.value = false;
  }
}

async function lookupOrder() {
  const id = orderQuery.value.trim();
  if (!id) {
    directLookup.value = false;
    await loadOrders(0);
    return;
  }
  if (!isUuid(id)) {
    listError.value = localized("注单编号必须是有效 UUID。", "Order ID must be a valid UUID.");
    return;
  }
  readGeneration.value++;
  const generation = readGeneration.value;
  const snapshot = context.value;
  const brandId = props.brandId;
  if (!rights.value.ordersView || !brandId) return;
  listBusy.value = true;
  listError.value = "";
  selectedOrderId.value = "";
  selectedOrder.value = null;
  exception.value = null;
  judgment.value = null;
  judgmentRead.value = false;
  try {
    const order = await api.getOrder(brandId, id);
    if (
      !contextCurrent(snapshot, generation, "list") ||
      !rights.value.ordersView
    )
      return;
    checkOrder(order, brandId, id, memberQuery.value.trim());
    orders.value = [order];
    offset.value = 0;
    hasNext.value = false;
    directLookup.value = true;
    selectedOrderId.value = order.id;
  } catch (cause) {
    if (contextCurrent(snapshot, generation, "list")) {
      orders.value = [];
      onReadError(cause, "list");
    }
  } finally {
    if (contextCurrent(snapshot, generation, "list")) listBusy.value = false;
  }
}

async function loadSelectedDetail() {
  const id = selectedOrderId.value;
  if (!id || !rights.value.ordersView || !props.brandId) return;
  // A receipt for the previously selected order must not label this order as
  // cancelled/refunded. Unknown intents are independently retained by scope.
  if (selectedOrder.value?.id !== id) notice.value = "";
  detailGeneration.value++;
  const generation = detailGeneration.value;
  const snapshot = context.value;
  const brandId = props.brandId;
  detailBusy.value = true;
  detailError.value = "";
  writeError.value = "";
  conflictNotice.value = "";
  intent.value = null;
  confirmed.value = false;
  reason.value = "";
  try {
    const order = await api.getOrder(brandId, id);
    if (
      !contextCurrent(snapshot, generation, "detail") ||
      selectedOrderId.value !== id ||
      !rights.value.ordersView
    )
      return;
    checkOrder(order, brandId, id);
    selectedOrder.value = order;
    syncVisibleOrder(order);
    const frozen = findFrozen(id);
    if (frozen) {
      pending.value = frozen;
      pendingUnknown.value = true;
      intent.value = { action: frozen.action, body: frozen.body };
      confirmed.value = true;
      reason.value = frozen.body.reason;
      if (frozen.action === "judge-cancel")
        judgmentCause.value = (frozen.body as JudgeCancelBody).cause;
    }
    exception.value = null;
    judgment.value = null;
    judgmentRead.value = false;
    // Exception evidence uses the same explicit bet.view authorization as order detail.
    const [evidence, judgmentEvidence] = await Promise.all([
      api.getException(brandId, id),
      api.getJudgment(brandId, id),
    ]);
    if (
      !contextCurrent(snapshot, generation, "detail") ||
      selectedOrderId.value !== id ||
      !rights.value.ordersView
    )
      return;
    if (evidence.exception) {
      if (
        evidence.exception.brand_id !== brandId ||
        evidence.exception.order_id !== id
      )
        throw new Error(t("异常记录与当前品牌或注单不匹配，请重新读取。", "The abnormal record does not match the current brand or order. Reload and try again."));
      exception.value = evidence.exception;
    }
    if (judgmentEvidence.judgment) {
      const record = judgmentEvidence.judgment;
      if (record.brand_id !== brandId || record.order_id !== id)
        throw new Error(t("判定记录与当前品牌或注单不匹配，请重新读取。", "The judgment record does not match the current brand or order. Reload and try again."));
      judgment.value = record;
    }
    judgmentRead.value = true;
    reconcilePending(order, evidence.exception, judgmentEvidence.judgment);
  } catch (cause) {
    if (
      contextCurrent(snapshot, generation, "detail") &&
      selectedOrderId.value === id
    ) {
      selectedOrder.value = null;
      exception.value = null;
      onReadError(cause, "detail");
    }
  } finally {
    if (contextCurrent(snapshot, generation, "detail"))
      detailBusy.value = false;
  }
}

function selectOrder(id: string) {
  // The watcher clears any old intent and errors before fetching the new selection.
  if (selectedOrderId.value !== id) notice.value = "";
  selectedOrderId.value = id;
}
watch(selectedOrderId, (id) => {
  // A write may finish after the operator opens a different order. Its frozen
  // intent remains scoped to the original order, but cannot replace this detail.
  writeGeneration.value++;
  writeBusy.value = false;
  selectedOrder.value = null;
  exception.value = null;
  judgment.value = null;
  judgmentRead.value = false;
  detailError.value = "";
  writeError.value = "";
  conflictNotice.value = "";
  intent.value = null;
  confirmed.value = false;
  reason.value = "";
  pending.value = null;
  pendingUnknown.value = false;
  if (id) void loadSelectedDetail();
});

function beginIntent(action: "cancel" | "mark-abnormal" | "judge-cancel") {
  const order = selectedOrder.value;
  if (!order || pendingUnknown.value) return;
  if (action === "cancel" && !canCancelSelected.value) return;
  if (action === "mark-abnormal" && !canMarkSelected.value) return;
  if (action === "judge-cancel" && !canJudgeCancelSelected.value) return;
  pendingUnknown.value = false;
  pending.value = null;
  reason.value = "";
  judgmentCause.value = "no_result";
  writeError.value = "";
  conflictNotice.value = "";
  confirmed.value = false;
  intent.value = {
    action,
    body:
      action === "judge-cancel"
        ? { version: order.version, reason: "", cause: judgmentCause.value }
        : { version: order.version, reason: "" },
  };
}
function freezeIntent() {
  if (!intent.value || !selectedOrder.value || !validBetReason(reason.value))
    return;
  intent.value = {
    ...intent.value,
    body:
      intent.value.action === "judge-cancel"
        ? {
            version: selectedOrder.value.version,
            reason: reason.value.trim(),
            cause: judgmentCause.value,
          }
        : { version: selectedOrder.value.version, reason: reason.value.trim() },
  };
  const scope = {
    brandId: props.brandId,
    entityId: selectedOrder.value.id,
    operation: `${intent.value.action}:${crypto.randomUUID()}`,
    body: intent.value.body,
  };
  const key = keyFor(scope);
  pending.value = freezeBetMutation({
    orderId: selectedOrder.value.id,
    brandId: props.brandId,
    action: intent.value.action,
    body: intent.value.body,
    idempotencyKey: key,
  });
  confirmed.value = true;
}
function isExpectedMutation(
  order: AdminBetOrder,
  operation: FrozenBetMutation<BetOrderActionBody>,
) {
  return operation.action === "cancel"
    ? order.status === "bet_cancelled" &&
        order.version > operation.body.version &&
        Boolean(order.refund_entry_id)
    : operation.action === "mark-abnormal"
      ? order.status === "abnormal" && order.version > operation.body.version
      : order.status === "judged_cancelled" &&
        order.version === operation.body.version + 1 &&
        Boolean(order.refund_entry_id);
}
function reconcilePending(
  order: AdminBetOrder,
  evidence: BetException | null,
  judgmentEvidence: Judgment | null,
) {
  const operation = pending.value;
  if (
    !operation ||
    operation.orderId !== order.id ||
    operation.brandId !== order.brand_id
  )
    return;
  const evidenceMatches =
    operation.action === "mark-abnormal"
      ? Boolean(
          evidence &&
            evidence.order_id === order.id &&
            evidence.brand_id === order.brand_id &&
            evidence.reason === operation.body.reason,
        )
      : operation.action !== "judge-cancel" ||
        hasMatchingJudgmentWitness(
          order,
          operation,
          judgmentEvidence,
          props.account.id,
        );
  if (isExpectedMutation(order, operation) && evidenceMatches) {
    pending.value = null;
    forgetFrozen(operation);
    pendingUnknown.value = false;
    intent.value = null;
    confirmed.value = false;
    reason.value = "";
    notice.value =
      operation.action === "cancel"
        ? localized("已从后台读取到取消及退款结果。", "Cancellation and refund result loaded from the server.")
        : operation.action === "mark-abnormal"
          ? localized("已从后台读取到异常标记记录。", "Abnormal marker record loaded from the server.")
          : localized("已从后台读取到判定取消结果。", "Judgment cancellation result loaded from the server.");
  }
}
async function runMutation(operation: FrozenBetMutation<BetOrderActionBody>) {
  if (
    operation.brandId !== props.brandId ||
    operation.orderId !== selectedOrderId.value
  )
    return;
  const permitted =
    operation.action === "cancel"
      ? rights.value.cancel
      : operation.action === "mark-abnormal"
        ? rights.value.markAbnormal
        : rights.value.judgeCancel;
  if (!rights.value.ordersView || !permitted) return;
  const generation = ++writeGeneration.value;
  const snapshot = context.value;
  writeBusy.value = true;
  rememberFrozen(operation);
  writeError.value = "";
  notice.value = "";
  try {
    const response =
      operation.action === "cancel"
        ? await api.cancelOrder(
            operation.brandId,
            operation.orderId,
            operation.body,
            operation.idempotencyKey,
          )
        : operation.action === "mark-abnormal"
          ? await api.markAbnormal(
              operation.brandId,
              operation.orderId,
              operation.body,
              operation.idempotencyKey,
            )
          : await api.judgeCancelOrder(
              operation.brandId,
              operation.orderId,
              operation.body as JudgeCancelBody,
              operation.idempotencyKey,
            );
    if (
      !contextCurrent(snapshot, generation, "write") ||
      !rights.value.ordersView
    )
      return;
    try {
      checkOrder(response, operation.brandId, operation.orderId);
    } catch {
      pending.value = operation;
      rememberFrozen(operation);
      pendingUnknown.value = true;
      throw new AdminApiError(
        t("后台响应与当前注单不匹配，操作结果尚未确定。", "The server response does not match the current order; the action outcome is unknown."),
        502,
        "INVALID_RESPONSE",
      );
    }
    if (!isExpectedMutation(response, operation)) {
      pending.value = operation;
      rememberFrozen(operation);
      pendingUnknown.value = true;
      throw new Error(
        t("后台返回的状态未确认此操作。请读取注单并使用相同请求重试。", "The returned state does not confirm this action. Read the order and retry using the same request."),
      );
    }
    selectedOrder.value = response;
    syncVisibleOrder(response);
    pending.value = null;
    forgetFrozen(operation);
    pendingUnknown.value = false;
    intent.value = null;
    confirmed.value = false;
    reason.value = "";
    notice.value =
      operation.action === "cancel"
        ? localized("后台已确认取消，退款记录已生成。", "The server confirmed cancellation and created a refund record.")
        : operation.action === "mark-abnormal"
          ? localized("后台已确认注单标记为异常。", "The server confirmed that the order was marked abnormal.")
          : localized("后台已确认判定取消；请查看判定记录或整期判定说明。", "The server confirmed judgment cancellation. Review the judgment record or period-wide judgment details.");
    await loadSelectedDetail();
    if (!directLookup.value) void loadOrders(offset.value);
  } catch (cause) {
    if (!contextCurrent(snapshot, generation, "write")) return;
    const failure = settleBetMutationFailure(
      cause instanceof AdminApiError ? cause.status : undefined,
      () => forgetFrozen(operation),
    );
    if (failure === "unknown") {
      pending.value = operation;
      rememberFrozen(operation);
      pendingUnknown.value = true;
      writeError.value =
        localized("请求结果尚未确定。原因和版本已锁定；读取后台状态后，可用同一请求编号重试。", "The request outcome is unknown. Reason and version are locked; after reading server state, retry with the same request ID.");
      return;
    }
    if (cause instanceof AdminApiError && cause.status === 401) {
      resetSession();
      return;
    }
    if (pending.value?.idempotencyKey === operation.idempotencyKey) {
      pending.value = null;
      pendingUnknown.value = false;
    }
    intent.value = null;
    confirmed.value = false;
    reason.value = "";
    writeError.value = "";
    if (cause instanceof AdminApiError && cause.status === 404) {
      selectedOrder.value = null;
      exception.value = null;
      detailError.value = localized("注单不存在或已不可见，请刷新列表。", "The order does not exist or is no longer visible. Refresh the list.");
      return;
    }
    if (failure === "conflict") {
      await loadSelectedDetail();
      if (contextCurrent(snapshot, generation, "write")) {
        conflictNotice.value =
          selectedOrder.value && !detailError.value
            ? localized("操作因版本、状态或判定依据冲突被拒绝，旧请求已丢弃。已重新读取当前注单；请核对后重新填写原因并确认。", "The action was rejected because of a version, state, or judgment conflict. The old request was discarded and the current order reloaded; review it, re-enter the reason, and confirm again.")
            : localized("注单版本冲突已确认，重新读取未成功。请刷新注单后再发起新确认。", "The order version conflict is confirmed, but reloading failed. Refresh the order before starting a new confirmation.");
      }
      return;
    }
    writeError.value =
      cause instanceof Error ? cause.message : localized("操作失败，请重试。", "Action failed. Please try again.");
  } finally {
    if (contextCurrent(snapshot, generation, "write")) writeBusy.value = false;
  }
}
function submitConfirmed() {
  if (!confirmed.value || !pending.value || writeBusy.value) return;
  void runMutation(pending.value);
}
function closeIntent() {
  if (pendingUnknown.value || writeBusy.value) return;
  if (pending.value) forgetFrozen(pending.value);
  intent.value = null;
  confirmed.value = false;
  pending.value = null;
  reason.value = "";
  writeError.value = "";
}
function retryFrozen() {
  if (!pending.value || !pendingUnknown.value || writeBusy.value) return;
  void runMutation(pending.value);
}
function reconcileFrozen() {
  if (!selectedOrderId.value || !rights.value.ordersView) return;
  void loadSelectedDetail();
}

watch(
  context,
  () => {
    readGeneration.value++;
    detailGeneration.value++;
    writeGeneration.value++;
    clearVisibleData();
    // Keep an uncertain write privately frozen; it can only be reconciled in its original brand scope.
    if (rights.value.ordersView && props.brandId) void loadOrders(0);
  },
  { flush: "sync" },
);

onMounted(() => {
  if (rights.value.ordersView && props.brandId) void loadOrders(0);
});
onUnmounted(() => {
  live = false;
  readGeneration.value++;
  detailGeneration.value++;
  writeGeneration.value++;
});
</script>

<template>
  <main class="bet-orders" data-testid="bet-order-management">
    <header class="heading">
      <div>
        <div class="eyebrow">ORDERS / REVIEW</div>
        <h1>{{ t("注单管理", "Bet order management") }}</h1>
        <p>{{ t("查看不可变投注快照；管理员取消会按原扣款分配退款。", "Review immutable bet snapshots. Admin cancellation refunds the original deduction allocation.") }}</p>
      </div>
      <button
        class="secondary"
        :disabled="listBusy || !rights.ordersView"
        @click="loadOrders(offset)"
      >
        {{ t("刷新", "Refresh") }}
      </button>
    </header>

    <p v-if="!rights.ordersView" class="notice">
      {{ t("当前账号没有此品牌的注单查看权限。", "This account cannot view bet orders for this brand.") }}
    </p>
    <template v-else>
      <form class="search panel" @submit.prevent="lookupOrder">
        <label
          >{{ t("成员 UUID（可选）", "Member UUID (optional)") }}
          <input
            v-model="memberQuery"
            autocomplete="off"
            :placeholder="t('按品牌成员筛选', 'Filter by brand member')"
          />
        </label>
        <label
          >{{ t("直接查询注单 UUID", "Look up bet order by UUID") }}
          <input
            v-model="orderQuery"
            autocomplete="off"
            placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
          />
        </label>
        <button class="primary" type="submit" :disabled="listBusy">
          {{ listBusy ? t("读取中…", "Loading…") : t("查询", "Search") }}
        </button>
      </form>
      <p v-if="listError" class="error" role="alert">{{ t(listError) }}</p>

      <section class="panel list-panel" :aria-label="t('注单列表', 'Bet order list')">
        <div class="section-heading">
          <div>
            <h2>{{ t("注单", "Orders") }}</h2>
            <p>{{ t("每页最多", "Up to") }} {{ BET_ORDER_PAGE_SIZE }} {{ t("条，按下单时间倒序。", "per page, newest orders first.") }}</p>
          </div>
          <span class="muted">{{
            directLookup ? t("直接查询结果", "Direct lookup result") : `${t("偏移", "Offset")} ${offset}`
          }}</span>
        </div>
        <div v-if="listBusy && !orders.length" class="empty">{{ t("正在读取注单…", "Loading bet orders…") }}</div>
        <div v-else-if="!orders.length" class="empty">{{ t("没有符合条件的注单。", "No matching bet orders.") }}</div>
        <div v-else class="order-list">
          <button
            v-for="order in orders"
            :key="order.id"
            class="order-row"
            :class="{ selected: selectedOrderId === order.id }"
            @click="selectOrder(order.id)"
          >
            <span class="row-id"
              ><b>{{ order.id }}</b
              ><small>{{ t("成员", "Member") }} {{ order.brand_member_id }}</small></span
            >
            <span class="row-period"
              >{{ order.game_id }}<small>{{ order.period_id }}</small></span
            >
            <span class="row-amount">{{ order.total_points }} {{ t("分", "points") }}</span>
            <span class="badge" :class="statusClass(order.status)">{{
              statusName(order.status)
            }}</span>
            <time>{{ order.placed_at }}</time>
          </button>
        </div>
        <footer class="pager" v-if="!directLookup">
          <button
            class="secondary"
            :disabled="listBusy || offset === 0"
            @click="loadOrders(Math.max(0, offset - BET_ORDER_PAGE_SIZE))"
          >
            {{ t("上一页", "Previous") }}
          </button>
          <span>{{ Math.floor(offset / BET_ORDER_PAGE_SIZE) + 1 }} {{ t("页", "page") }}</span>
          <button
            class="secondary"
            :disabled="listBusy || !hasNext"
            @click="loadOrders(offset + BET_ORDER_PAGE_SIZE)"
          >
            {{ t("下一页", "Next") }}
          </button>
        </footer>
      </section>

      <section
        v-if="selectedOrderId"
        class="panel detail-panel"
        :aria-label="t('注单详情', 'Bet order details')"
      >
        <div class="section-heading">
          <div>
            <h2>{{ t("注单详情", "Bet order details") }}</h2>
            <p class="mono">{{ selectedOrderId }}</p>
          </div>
          <button class="text-button" @click="selectOrder('')">{{ t("关闭", "Close") }}</button>
        </div>
        <p v-if="detailBusy" class="empty">{{ t("正在读取详情…", "Loading details…") }}</p>
        <p v-if="detailError" class="error" role="alert">{{ t(detailError) }}</p>
        <template v-if="selectedOrder">
          <div class="detail-top">
            <span class="badge" :class="statusClass(selectedOrder.status)">{{
              statusName(selectedOrder.status)
            }}</span>
            <span>{{ t("版本", "Version") }} {{ selectedOrder.version }}</span>
            <time>{{ selectedOrder.placed_at }}</time>
          </div>
          <div class="facts">
            <div>
              <small>{{ t("品牌", "Brand") }}</small><b>{{ selectedOrder.brand_id }}</b>
            </div>
            <div>
              <small>{{ t("成员 ID", "Member ID") }}</small><b>{{ selectedOrder.brand_member_id }}</b>
            </div>
            <div>
              <small>{{ t("全局用户 ID", "Global user ID") }}</small
              ><b>{{ selectedOrder.global_user_id }}</b>
            </div>
            <div>
              <small>{{ t("彩种 ID / 期次 ID", "Game ID / period ID") }}</small
              ><b
                >{{ selectedOrder.game_id }} / {{ selectedOrder.period_id }}</b
              >
            </div>
            <div>
              <small>{{ t("玩法 ID", "Play ID") }}</small><b>{{ selectedOrder.play_id }}</b>
            </div>
            <div>
              <small>{{ t("规则版本 ID", "Rule version ID") }}</small
              ><b>{{ selectedOrder.rule_version_id }}</b>
            </div>
            <div>
              <small>{{ t("规则快照哈希", "Rule snapshot hash") }}</small
              ><b>{{ selectedOrder.definition_hash }}</b>
            </div>
          </div>
          <div class="amounts">
            <div v-for="row in amountRows" :key="row[0]">
              <small>{{ amountLabel(row[0]) }}</small
              ><b
                >{{ row[1]
                }}<template v-if="row[0] === '单注积分' || row[0] === '总扣款'">
                  {{ ' ' + t("分", "points") }}</template
                ><template v-else-if="row[0] === '倍数'">{{ ' ' + t("倍", "x") }}</template></b
              >
            </div>
          </div>

          <SettlementPreview
            :key="`${brandId}:${selectedOrder.id}:${selectedOrder.version}`"
            :account="account"
            :brand-id="brandId"
            :order="selectedOrder"
            @session-invalid="resetSession"
          />

          <section class="subsection">
            <h3>{{ t("规则与奖级赔率快照", "Rule and prize tier odds snapshot") }}</h3>
            <div
              class="tiers"
              v-if="selectedOrder.definition_snapshot.prize_tiers?.length"
            >
              <div
                v-for="tier in selectedOrder.definition_snapshot.prize_tiers"
                :key="tier.code"
              >
                <b>{{ tier.code }}</b
                ><span>{{ t("赔率", "Odds") }} {{ tier.odds }}</span
                ><span>{{ tier.exclusive ? t("互斥", "Exclusive") : t("可叠加", "Stackable") }}</span
                ><span v-if="tier.cap_points"
                  >{{ t("封顶", "Cap") }} {{ tier.cap_points }} {{ t("分", "points") }}</span
                >
              </div>
            </div>
            <p v-else class="muted">{{ t("快照中没有奖级赔率。", "No prize tier odds in this snapshot.") }}</p>
            <details>
              <summary>{{ t("查看完整规则定义", "View full rule definition") }}</summary>
              <pre>{{ stringify(selectedOrder.definition_snapshot) }}</pre>
            </details>
          </section>

          <section class="subsection">
            <h3>{{ t("选号快照", "Selection snapshot") }}</h3>
            <div class="selection-grid">
              <div>
                <h4>{{ t("原始选号", "Original selection") }}</h4>
                <pre>{{ stringify(selectedOrder.selection_raw) }}</pre>
              </div>
              <div>
                <h4>{{ t("标准化选号", "Normalized selection") }}</h4>
                <pre>{{ stringify(selectedOrder.selection_normalized) }}</pre>
              </div>
            </div>
          </section>
          <section class="subsection">
            <h3>{{ t("展开组合（最多预览", "Expanded combinations (preview up to") }} {{ BET_COMBINATION_PREVIEW_LIMIT }} {{ t("条）", ")") }}</h3>
            <pre>{{ stringify(preview.items) }}</pre>
            <p class="muted">
              {{ t("共", "Total") }} {{ preview.total }} {{ t("组", "combinations") }}<template v-if="preview.remaining"
                >，{{ t("其余", "the remaining") }} {{ preview.remaining }} {{ t("组不在页面展开。", "combinations are not expanded here.") }}</template
              >
            </p>
          </section>

          <section class="subsection">
            <h3>{{ t("扣款与退款证据", "Debit and refund evidence") }}</h3>
            <div class="facts">
              <div>
                <small>{{ t("扣款账本条目", "Debit ledger entry") }}</small
                ><b>{{ selectedOrder.debit_entry_id }}</b>
              </div>
              <div>
                <small>{{ t("退款账本条目", "Refund ledger entry") }}</small
                ><b>{{ selectedOrder.refund_entry_id || t("无", "None") }}</b>
              </div>
              <div>
                <small>{{ t("取消时间", "Cancelled at") }}</small
                ><b>{{ selectedOrder.cancelled_at || t("无", "None") }}</b>
              </div>
              <div>
                <small>{{ t("取消原因", "Cancellation reason") }}</small
                ><b>{{ selectedOrder.cancel_reason || t("无", "None") }}</b>
              </div>
            </div>
            <div class="allocation">
              <b>{{ t("原始扣款来源分配", "Original debit allocation by source") }}</b>
              <p
                v-for="(entry, index) in selectedOrder.deduction_allocation"
                :key="`${entry.source}-${entry.state}-${index}`"
              >
                {{ entry.source }} · {{ entry.state }}: {{ entry.points }} {{ t("分", "points") }}
              </p>
            </div>
          </section>
          <section class="subsection">
            <h3>{{ t("策略版本", "Policy versions") }}</h3>
            <div class="facts">
              <div>
                <small>{{ t("品牌策略版本", "Brand policy version") }}</small
                ><b>{{ selectedOrder.policy_versions.brand }}</b>
              </div>
              <div>
                <small>{{ t("彩种策略版本", "Game policy version") }}</small
                ><b>{{ selectedOrder.policy_versions.game }}</b>
              </div>
              <div>
                <small>{{ t("用户可自行取消", "User may cancel") }}</small
                ><b>{{
                  selectedOrder.policy_snapshot.user_cancel_allowed
                    ? t("是", "Yes")
                    : t("否", "No")
                }}</b>
              </div>
            </div>
            <details>
              <summary>{{ t("查看策略快照", "View policy snapshot") }}</summary>
              <pre>{{ stringify(selectedOrder.policy_snapshot) }}</pre>
            </details>
          </section>

          <section class="subsection evidence">
            <h3>{{ t("异常标记证据", "Abnormal marker evidence") }}</h3>
            <p v-if="detailBusy" class="muted" aria-live="polite">{{ t("正在读取异常标记记录…", "Loading abnormal marker records…") }}</p>
            <template v-else-if="exception"
              ><div class="facts">
                <div>
                  <small>{{ t("异常记录 ID", "Abnormal record ID") }}</small><b>{{ exception.id }}</b>
                </div>
                <div>
                  <small>{{ t("记录时注单版本", "Order version when recorded") }}</small
                  ><b>{{ exception.order_version }}</b>
                </div>
                <div>
                  <small>{{ t("标记来源 / 标记者", "Marker source / actor") }}</small><b>{{ exception.source === 'system' ? `${t('系统核算', 'System validation')} · ${exception.error_code}` : exception.marked_by }}</b>
                </div>
                <div>
                  <small>{{ t("记录时间", "Recorded at") }}</small><b>{{ exception.created_at }}</b>
                </div>
              </div>
              <p class="reason-box">{{ exception.reason }}</p></template
            >
            <p v-else class="muted">{{ t("当前注单没有异常标记记录。", "No abnormal marker record for this order.") }}</p>
          </section>

          <section class="subsection judgment-evidence">
            <h3>{{ t("判定取消记录", "Judgment cancellation record") }}</h3>
            <p v-if="detailBusy" class="muted" aria-live="polite">{{ t("正在读取判定取消记录…", "Loading judgment cancellation records…") }}</p>
            <template v-else-if="judgment">
              <div class="facts">
                <div>
                  <small>{{ t("判定记录 ID", "Judgment record ID") }}</small><b>{{ judgment.id }}</b>
                </div>
                <div>
                  <small>{{ t("判定时注单版本", "Order version at judgment") }}</small
                  ><b>{{ judgment.order_version }}</b>
                </div>
                <div>
                  <small>{{ t("判定原因类型", "Judgment cause") }}</small
                  ><b>{{
                    judgment.cause === "no_result" ? t("未出结果", "No result") : t("结果无效", "Invalid result")
                  }}</b>
                </div>
                <div>
                  <small>{{ t("判定操作者 ID", "Judged by account ID") }}</small><b>{{ judgment.judged_by }}</b>
                </div>
                <div>
                  <small>{{ t("判定时间", "Judged at") }}</small><b>{{ judgment.created_at }}</b>
                </div>
                <div>
                  <small>{{ t("开奖结果引用", "Draw result reference") }}</small
                  ><b>{{ judgment.draw_result_id || t("无", "None") }}</b>
                </div>
                <div>
                  <small>{{ t("退款账本引用", "Refund ledger reference") }}</small
                  ><b>{{ judgment.refund_entry_id }}</b>
                </div>
              </div>
              <p class="reason-box">{{ judgment.reason }}</p>
            </template>
            <p
              v-else-if="
                judgmentRead && selectedOrder.status === 'judged_cancelled'
              "
              class="muted"
            >
              {{ t("没有单独判定记录；整期判定取消流程可能不会为每注生成此记录。", "No individual judgment record exists. Period-wide judgment cancellation may not create one for every order.") }}
            </p>
            <p v-else-if="judgmentRead" class="muted">
              {{ t("当前注单没有单独判定取消记录。", "No individual judgment cancellation record for this order.") }}
            </p>
            <p v-else class="muted">{{ t("正在读取判定记录…", "Loading judgment record…") }}</p>
          </section>

          <div
            v-if="
              (rights.cancel || rights.markAbnormal || rights.judgeCancel) &&
              !pendingUnknown
            "
            class="actions"
          >
            <button
              v-if="canMarkSelected"
              class="secondary"
              @click="beginIntent('mark-abnormal')"
            >
              {{ t("标记为异常", "Mark abnormal") }}
            </button>
            <button
              v-if="canCancelSelected"
              class="danger-button"
              @click="beginIntent('cancel')"
            >
              {{ t("管理员取消并退款", "Cancel and refund") }}
            </button>
            <button
              v-if="canJudgeCancelSelected"
              class="danger-button"
              @click="beginIntent('judge-cancel')"
            >
              {{ t("判定取消", "Judge cancellation") }}
            </button>
            <span
              v-if="selectedOrder.status === 'abnormal' && rights.markAbnormal"
              class="muted"
              >{{ t("已标记异常；取消仍可单独处理。", "Marked abnormal; cancellation can still be handled separately.") }}</span
            >
            <span
              v-if="!['placed', 'abnormal'].includes(selectedOrder.status)"
              class="muted"
              >{{ t("此状态只读，投注、规则、赔率与积分均不可编辑。", "This status is read-only; bets, rules, odds, and points cannot be edited.") }}</span
            >
          </div>
          <p v-if="notice" class="success-note" role="status">{{ t(notice) }}</p>
          <p v-if="conflictNotice" class="conflict-note" role="status">
            {{ t(conflictNotice) }}
          </p>
          <p v-if="writeError" class="error" role="alert">{{ t(writeError) }}</p>
          <div
            v-if="
              pendingUnknown &&
              pending?.brandId === brandId &&
              pending?.orderId === selectedOrder.id &&
              rights.ordersView
            "
            class="reconcile-box"
          >
            <p>{{ t("有一项结果未确定的操作。请求正文和编号保持锁定。", "An action has an unresolved outcome. Its request body and identifier remain locked.") }}</p>
            <button
              class="secondary"
              :disabled="detailBusy"
              @click="reconcileFrozen"
            >
              {{ t("读取后台状态", "Read server state") }}
            </button>
            <button
              v-if="
                pending.action === 'cancel'
                  ? rights.cancel
                  : pending.action === 'mark-abnormal'
                    ? rights.markAbnormal
                    : rights.judgeCancel
              "
              class="danger-button"
              :disabled="writeBusy"
              @click="retryFrozen"
            >
              {{ writeBusy ? t("重试中…", "Retrying…") : t("使用相同请求编号重试", "Retry with the same request ID") }}
            </button>
          </div>
        </template>
      </section>
    </template>

    <div
      v-if="intent && !pendingUnknown"
      class="backdrop"
      @click.self="closeIntent"
    >
      <section
        class="dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="bet-action-title"
      >
        <div class="section-heading">
          <div>
            <div class="eyebrow">
              {{
                intent.action === "cancel"
                  ? "REFUNDING CANCELLATION"
                  : intent.action === "mark-abnormal"
                    ? "ABNORMAL REVIEW"
                    : "JUDGMENT CANCELLATION"
              }}
            </div>
            <h2 id="bet-action-title">
              {{
                intent.action === "cancel"
                  ? t("确认管理员取消", "Confirm admin cancellation")
                  : intent.action === "mark-abnormal"
                    ? t("确认标记异常", "Confirm abnormal marking")
                    : t("确认判定取消", "Confirm judgment cancellation")
              }}
            </h2>
          </div>
          <button class="text-button" @click="closeIntent">{{ t("关闭", "Close") }}</button>
        </div>
        <p v-if="intent.action === 'cancel'" class="warning">
          {{ t("管理员取消会将原始扣款积分按来源分配全额退回。该操作适用于待开奖或异常注单，会生成退款账本记录。", "Admin cancellation refunds the full original deduction by source. It applies to awaiting-draw or abnormal orders and creates a refund ledger entry.") }}
        </p>
        <p v-else-if="intent.action === 'mark-abnormal'" class="warning">
          {{ t("标记异常只记录异常证据，不退款。注单状态必须仍为待开奖。", "Marking abnormal records evidence only and does not issue a refund. The order must still be awaiting draw.") }}
        </p>
        <p v-else class="warning">
          {{ t("判定取消会依据开奖结果情况取消注单并生成退款记录；这不是付款或重新结算。服务器会检查该注单及期次是否允许判定。", "Judgment cancellation cancels the order based on the draw result and creates a refund record. It is not a payout or resettlement. The server verifies that the order and period allow judgment.") }}
        </p>
        <div v-if="selectedOrder" class="confirm-facts">
          <p>
            {{ t("注单：", "Order: ") }}<b>{{ selectedOrder.id }}</b>
          </p>
          <p>
            {{ t("提交版本：", "Submission version: ") }}<b>{{ selectedOrder.version }}</b>
          </p>
          <p>
            {{ t("当前状态：", "Current status: ") }}<b>{{ statusName(selectedOrder.status) }}</b>
          </p>
        </div>
        <label v-if="intent.action === 'judge-cancel'" class="reason-label">
          {{ t("判定原因类型", "Judgment cause") }}
          <select
            v-model="judgmentCause"
            :aria-label="t('判定取消原因', 'Judgment cancellation cause')"
            :disabled="confirmed || writeBusy"
          >
            <option value="no_result">{{ t("未出开奖结果", "No draw result") }}</option>
            <option value="invalid_result">{{ t("开奖结果无效", "Invalid draw result") }}</option>
          </select>
        </label>
        <p v-if="intent.action === 'judge-cancel'" class="muted">
          {{ t("“未出开奖结果”要求当前没有锁定结果；若存在外部候选结果但经人工认定无效，请选择“开奖结果无效”。", "Use “No draw result” only when no result is locked. If an external candidate exists but has been confirmed invalid, choose “Invalid draw result”.") }}
        </p>
        <label class="reason-label"
          >{{ t("操作原因（必填，UTF-8 最多 500 字节）", "Reason (required, up to 500 UTF-8 bytes)") }}
          <textarea
            v-model="reason"
            :disabled="confirmed || writeBusy"
            rows="4"
            maxlength="5000"
            :placeholder="t('填写可审计的操作原因', 'Enter an auditable reason for this action')"
          ></textarea>
        </label>
        <p class="byte-count" :class="{ over: reasonBytes > 500 }">
          {{ reasonBytes }} / {{ t("500 字节", "500 bytes") }}
        </p>
        <template v-if="confirmed"
          ><div class="confirm-facts">
            <p>{{ t("固定请求正文：", "Frozen request body: ") }}{{ stringify(pending?.body) }}</p>
            <p>
              {{ t("请求编号：", "Request ID: ") }}<b>{{ pending?.idempotencyKey }}</b>
            </p>
          </div>
          <label class="confirm-check"
            ><input v-model="confirmed" type="checkbox" />
            {{ t("我已核对注单、版本、原因及", "I reviewed the order, version, reason, and ") }}{{
              intent.action === "cancel"
                ? t("退款影响", "refund impact")
                : intent.action === "mark-abnormal"
                  ? t("异常标记", "abnormal marker")
                  : t("判定依据和退款影响", "judgment basis and refund impact")
            }}。</label
          ></template
        >
        <p v-if="writeError" class="error" role="alert">{{ t(writeError) }}</p>
        <div class="dialog-actions">
          <button
            class="secondary"
            :disabled="writeBusy || pendingUnknown"
            @click="closeIntent"
          >
            {{ t("返回", "Back") }}</button
          ><button
            v-if="!confirmed"
            class="primary"
            :disabled="!validBetReason(reason) || writeBusy"
            @click="freezeIntent"
          >
            {{ t("检查并确认", "Review and confirm") }}</button
          ><button
            v-else
            class="danger-button"
            :disabled="!confirmed || writeBusy || !pending"
            @click="submitConfirmed"
          >
            {{ writeBusy ? t("提交中…", "Submitting…") : t("确认并提交", "Confirm and submit") }}
          </button>
        </div>
      </section>
    </div>
  </main>
</template>

<style scoped>
.bet-orders {
  color: #252a36;
  display: grid;
  gap: 16px;
  min-width: 0;
}
.heading,
.section-heading,
.detail-top,
.pager,
.actions,
.dialog-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.heading h1,
.section-heading h2,
.dialog h2 {
  margin: 3px 0;
}
.heading p,
.section-heading p {
  color: #737b8c;
  margin: 5px 0 0;
}
.eyebrow {
  color: #858da0;
  font-size: 10px;
  letter-spacing: 1.2px;
}
.panel {
  background: #fff;
  border: 1px solid #e9ebf0;
  border-radius: 10px;
  padding: 16px;
  box-shadow: 0 2px 8px #1e2a5008;
}
.search {
  display: grid;
  grid-template-columns: minmax(180px, 1fr) minmax(260px, 1.5fr) auto;
  align-items: end;
  gap: 12px;
}
label {
  display: grid;
  gap: 6px;
  color: #60697b;
  font-size: 12px;
}
input,
select,
textarea {
  width: 100%;
  border: 1px solid #dfe2e9;
  border-radius: 7px;
  padding: 10px 11px;
  color: #252a36;
  background: #fff;
  min-width: 0;
}
button {
  border: 0;
  border-radius: 7px;
  padding: 9px 13px;
  font-weight: 600;
}
.primary {
  color: #fff;
  background: #5969df;
}
.secondary {
  color: #48536b;
  background: #f1f3f7;
  border: 1px solid #e3e6ed;
}
.danger-button {
  color: #fff;
  background: #bd3e4c;
}
.text-button {
  color: #5969df;
  background: transparent;
  padding: 5px;
}
.empty,
.notice {
  color: #737b8c;
  text-align: center;
  padding: 20px 8px;
}
.error {
  color: #b32e40;
  background: #fff2f3;
  border-radius: 7px;
  padding: 10px 12px;
  margin: 0;
  overflow-wrap: anywhere;
}
.success-note {
  color: #26724f;
  background: #edfaf3;
  border-radius: 7px;
  padding: 10px 12px;
}
.conflict-note {
  color: #76571a;
  background: #fff8e7;
  border: 1px solid #edc974;
  border-radius: 7px;
  padding: 10px 12px;
}
.muted,
small {
  color: #81899a;
}
.order-list {
  display: grid;
}
.order-row {
  display: grid;
  grid-template-columns:
    minmax(190px, 1.4fr) minmax(140px, 1fr) minmax(100px, 0.6fr)
    auto minmax(145px, 0.9fr);
  gap: 12px;
  align-items: center;
  text-align: left;
  background: #fff;
  border-bottom: 1px solid #eef0f4;
  border-radius: 0;
  padding: 13px 4px;
}
.order-row:hover,
.order-row.selected {
  background: #f7f8ff;
}
.order-row span,
.order-row time {
  min-width: 0;
  overflow-wrap: anywhere;
}
.row-id b,
.row-id small,
.row-period small {
  display: block;
}
.row-id b,
.mono,
.facts b {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 11px;
  overflow-wrap: anywhere;
}
.row-id small,
.row-period small {
  margin-top: 4px;
}
.row-amount {
  font-variant-numeric: tabular-nums;
  font-weight: 700;
  white-space: nowrap;
}
.order-row time,
.detail-top time {
  font-size: 11px;
  color: #747c8d;
}
.badge {
  display: inline-flex;
  width: max-content;
  border-radius: 20px;
  padding: 4px 9px;
  font-size: 11px;
  background: #f0f1f4;
  color: #687184;
}
.badge.waiting {
  color: #8b6418;
  background: #fff5d9;
}
.badge.danger {
  color: #ad3341;
  background: #fff0f1;
}
.badge.success {
  color: #287550;
  background: #e9f8ef;
}
.pager {
  justify-content: center;
  padding-top: 15px;
  color: #737b8c;
}
.detail-panel {
  display: grid;
  gap: 16px;
}
.detail-top {
  justify-content: flex-start;
  flex-wrap: wrap;
  color: #626b7c;
  font-size: 12px;
}
.facts {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}
.facts > div,
.amounts > div {
  display: grid;
  gap: 5px;
  min-width: 0;
  background: #f8f9fb;
  border-radius: 7px;
  padding: 10px;
}
.facts b {
  color: #333a49;
  font-weight: 500;
}
.amounts {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 9px;
}
.amounts b {
  font-size: 15px;
  font-variant-numeric: tabular-nums;
}
.subsection {
  border-top: 1px solid #eceef2;
  padding-top: 14px;
  min-width: 0;
}
.subsection h3 {
  font-size: 14px;
  margin: 0 0 10px;
}
.subsection h4 {
  margin: 0 0 7px;
}
.tiers {
  display: grid;
  gap: 7px;
  margin-bottom: 12px;
}
.tiers > div {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  background: #f8f9fb;
  border-radius: 6px;
  padding: 9px;
}
.selection-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
pre {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  max-height: 340px;
  overflow: auto;
  background: #f7f8fa;
  border: 1px solid #eceef2;
  border-radius: 7px;
  padding: 11px;
  font:
    11px/1.55 ui-monospace,
    SFMono-Regular,
    Menlo,
    monospace;
}
summary {
  cursor: pointer;
  color: #5969df;
  margin: 10px 0;
}
.allocation {
  margin-top: 12px;
}
.allocation p {
  display: inline-block;
  margin: 7px 14px 0 0;
  color: #555e70;
}
.reason-box {
  padding: 10px;
  background: #fff8e8;
  border-radius: 7px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.actions {
  justify-content: flex-start;
  flex-wrap: wrap;
  border-top: 1px solid #eceef2;
  padding-top: 14px;
}
.reconcile-box {
  border: 1px solid #edc974;
  background: #fffaf0;
  border-radius: 8px;
  padding: 12px;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 9px;
}
.reconcile-box p {
  flex-basis: 100%;
  margin: 0;
  color: #76571a;
}
.backdrop {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: grid;
  place-items: center;
  padding: 16px;
  background: #171b2b77;
}
.dialog {
  width: min(600px, 100%);
  max-height: min(92vh, 850px);
  overflow: auto;
  background: #fff;
  border-radius: 12px;
  padding: 20px;
  display: grid;
  gap: 13px;
  box-shadow: 0 18px 70px #12182d4d;
}
.warning {
  margin: 0;
  border-left: 3px solid #c38b22;
  background: #fff8e7;
  color: #6f541c;
  padding: 11px 12px;
  line-height: 1.55;
}
.confirm-facts {
  background: #f7f8fa;
  border-radius: 7px;
  padding: 9px 12px;
  overflow-wrap: anywhere;
}
.confirm-facts p {
  margin: 5px 0;
}
.reason-label textarea {
  resize: vertical;
}
.byte-count {
  justify-self: end;
  margin: -8px 0 0;
  color: #737b8c;
  font-size: 11px;
}
.byte-count.over {
  color: #b32e40;
}
.confirm-check {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}
.confirm-check input {
  width: auto;
  margin: 2px 0;
}
.dialog-actions {
  justify-content: flex-end;
}
@media (max-width: 800px) {
  .order-row {
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 7px 12px;
  }
  .order-row .row-period,
  .order-row .row-amount,
  .order-row time {
    grid-column: 1;
  }
  .order-row .badge {
    grid-column: 2;
    grid-row: 1;
  }
  .facts {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (max-width: 520px) {
  .heading {
    align-items: flex-start;
  }
  .search {
    grid-template-columns: 1fr;
  }
  .search button {
    width: 100%;
  }
  .panel {
    padding: 12px;
  }
  .selection-grid {
    grid-template-columns: 1fr;
  }
  .amounts {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .facts {
    grid-template-columns: 1fr;
  }
  .detail-top {
    align-items: flex-start;
  }
  .reconcile-box > * {
    width: 100%;
  }
  .dialog {
    padding: 16px;
  }
}
</style>
