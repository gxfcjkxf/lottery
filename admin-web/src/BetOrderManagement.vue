<script setup lang="ts">
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import SiteIcon from "@lottery/shared/SiteIcon";
import { vGlobalLoading } from "@lottery/shared";
const displayDate = (value: string | null | undefined) => formatDisplayDateTime(value, displayLocale.value);
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import { vInlineValidation } from '@lottery/shared'
import DetailModal from "@lottery/shared/DetailModal.vue"
import type { LocalizedMessage } from "@lottery/shared";
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { adminKey, useAdminI18n } from "./i18n";
import { AdminApiError, type AdminAccount } from "./admin-api";
import SettlementPreview from "./SettlementPreview.vue";
import { lookupAdminMember } from "./member-lookup";
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
const { t, message: localized, display } = useAdminI18n();
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
    ["已结算中奖积分", order.settled_at ? (order.prize_points ?? "0") : t('admin.copy.15dadd6f')],
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
      placed: t('admin.copy.aab1777a'),
      abnormal: t('admin.copy.7bcf4b54'),
      bet_cancelled: t('admin.copy.a1834cc2'),
      judged_cancelled: t('admin.copy.46a60c2c'),
      won: t('admin.copy.6de62356'),
      lost: t('admin.copy.e2f3b9bc'),
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
  const keys: Record<string, string> = {
    "单注积分": "admin.betOrder.amount.unit", "组合数": "admin.betOrder.amount.combinations", "倍数": "admin.betOrder.amount.multiplier",
    "总扣款": "admin.betOrder.amount.total", "已结算中奖积分": "admin.betOrder.amount.settledPrize",
    "结算核算 ID": "admin.betOrder.amount.calculationId", "派奖账本 ID": "admin.betOrder.amount.ledgerId",
  };
  return keys[label] ? t(adminKey(keys[label]!)) : display(label);
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
    throw new Error(t('admin.copy.63869504'));
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
    detailError.value = localized('admin.copy.567187cc');
    return;
  }
  const message = cause instanceof Error ? publicErrorMessage(cause, "read") : localized('admin.copy.4fb9cc4e');
  if (lane === "list") listError.value = message;
  else detailError.value = message;
}

async function loadOrders(nextOffset = 0) {
  const username = memberQuery.value.trim();
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
    const memberId = username ? (await lookupAdminMember(brandId, username)).id : "";
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
    listError.value = localized('admin.copy.46944ded');
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
    const username = memberQuery.value.trim();
    const memberId = username ? (await lookupAdminMember(brandId, username)).id : undefined;
    checkOrder(order, brandId, id, memberId);
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
        throw new Error(t('admin.copy.cbcddc9e'));
      exception.value = evidence.exception;
    }
    if (judgmentEvidence.judgment) {
      const record = judgmentEvidence.judgment;
      if (record.brand_id !== brandId || record.order_id !== id)
        throw new Error(t('admin.copy.bea22326'));
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
        ? localized('admin.copy.5dde3a9f')
        : operation.action === "mark-abnormal"
          ? localized('admin.copy.e947e7fa')
          : localized('admin.copy.bf4b6380');
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
        t('admin.copy.f6c509fc'),
        502,
        "INVALID_RESPONSE",
      );
    }
    if (!isExpectedMutation(response, operation)) {
      pending.value = operation;
      rememberFrozen(operation);
      pendingUnknown.value = true;
      throw new Error(
        t('admin.copy.a148ba17'),
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
        ? localized('admin.copy.d8d5e8d8')
        : operation.action === "mark-abnormal"
          ? localized('admin.copy.63be1c31')
          : localized('admin.copy.f67d7173');
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
        localized('admin.copy.3c19afa0');
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
      detailError.value = localized('admin.copy.567187cc');
      return;
    }
    if (failure === "conflict") {
      await loadSelectedDetail();
      if (contextCurrent(snapshot, generation, "write")) {
        conflictNotice.value =
          selectedOrder.value && !detailError.value
            ? localized('admin.copy.d83fd039')
            : localized('admin.copy.61c8032c');
      }
      return;
    }
    writeError.value =
      cause instanceof Error ? publicErrorMessage(cause) : localized('admin.copy.b3332ee3');
  } finally {
    if (contextCurrent(snapshot, generation, "write")) writeBusy.value = false;
  }
}
async function submitConfirmed() {
  if (!confirmed.value || !pending.value || writeBusy.value) return;
  await runMutation(pending.value);
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
async function retryFrozen() {
  if (!pending.value || !pendingUnknown.value || writeBusy.value) return;
  await runMutation(pending.value);
}
async function reconcileFrozen() {
  if (!selectedOrderId.value || !rights.value.ordersView) return;
  await loadSelectedDetail();
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
        <div class="eyebrow">{{ t('admin.ui.ordersReview') }}</div>
        <h1>{{ t('admin.copy.f83e2039') }}</h1>
        <p>{{ t('admin.copy.8540f788') }}</p>
      </div>

    </header>

    <p v-if="!rights.ordersView" class="notice">
      {{ t('admin.copy.275b2488') }}
    </p>
    <template v-else>
      <form novalidate v-inline-validation class="search panel" @submit.prevent="withGlobalLoading(lookupOrder)">
        <label class="query-field">
          <span class="field-label">{{ t('admin.copy.08f6109f') }}</span>
          <div class="input-with-icon">
            <span class="input-icon"> <SiteIcon name="user" /> </span>
            <input
              v-model="memberQuery"
              autocomplete="off"
              :placeholder="t('admin.copy.2a7cdda3')"
            />
          </div>
        </label>
        <label class="query-field">
          <span class="field-label">{{ t('admin.copy.b5b3fc36') }}</span>
          <div class="input-with-icon">
            <span class="input-icon"> <SiteIcon name="ticket" /> </span>
            <input
              v-model="orderQuery"
              autocomplete="off"
              :placeholder="t('admin.console.betOrderPlaceholder')"
            />
          </div>
        </label>
        <button class="button button-primary" type="submit" :disabled="listBusy">
          {{ listBusy ? t('admin.copy.27342bf3') : t('admin.copy.62a97c9a') }}
        </button>
      </form>
        <p v-if="listError" class="error" role="alert">{{ display(listError) }}</p>

      <section class="panel list-panel" :aria-label="t('admin.copy.bd1b17b6')">
        <div class="section-heading">
          <div>
            <h2>{{ t('admin.copy.15883cbb') }}</h2>
          </div>
          <span class="muted">{{
            directLookup ? t('admin.copy.a87ca2e0') : `${t('admin.copy.4ffb01e2')} ${offset}`
          }}</span>
        </div>
        <div v-global-loading v-if="listBusy && !orders.length" class="empty">{{ t('admin.copy.db6d189c') }}</div>
        <div v-else-if="!orders.length" class="empty empty-state">{{ t('admin.copy.3d2ab929') }}</div>
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
              ><small>{{ t('admin.copy.86d0a04d') }} <AssociationName :record="order" kind="member" :id="order.brand_member_id" :locale="associationLocale" /></small></span
            >
            <span class="row-period"
              ><AssociationName :record="order" kind="game" :id="order.game_id" :locale="associationLocale" /><small><AssociationName :record="order" kind="period" :id="order.period_id" :locale="associationLocale" /></small></span
            >
            <span class="row-amount">{{ order.total_points }} {{ t('admin.copy.2f31ccd0') }}</span>
            <span class="badge" :class="statusClass(order.status)">{{
              statusName(order.status)
            }}</span>
            <time>{{ displayDate(order.placed_at) }}</time>
          </button>
        </div>
        <footer class="pager" v-if="!directLookup">
          <button
            class="button button-secondary"
            :disabled="listBusy || offset === 0"
            @click="withGlobalLoading(loadOrders, Math.max(0, offset - BET_ORDER_PAGE_SIZE))"
          >
            {{ t('admin.copy.dbe7a5d7') }}
          </button>
          <span>{{ Math.floor(offset / BET_ORDER_PAGE_SIZE) + 1 }} {{ t('admin.copy.a8168a5b') }}</span>
          <button
            class="button button-secondary"
            :disabled="listBusy || !hasNext"
            @click="withGlobalLoading(loadOrders, offset + BET_ORDER_PAGE_SIZE)"
          >
            {{ t('admin.copy.2e9c9e7a') }}
          </button>
        </footer>
      </section>

      <DetailModal v-if="!!selectedOrderId" :title="t('admin.copy.01db40da')" :close-label="t('admin.copy.2bb2cb1d')" :open="!intent || pendingUnknown" @close="selectOrder('')">
<section
        v-if="selectedOrderId"
        class="panel detail-panel"
        :aria-label="t('admin.copy.ea6ab3ba')"
      >
        <div class="section-heading">
          <div>
            <h2>{{ t('admin.copy.ea6ab3ba') }}</h2>
            <p class="mono">{{ selectedOrderId }}</p>
          </div>
          <button class="text-button" @click="selectOrder('')">{{ t('admin.copy.2bb2cb1d') }}</button>
        </div>
        <p v-global-loading v-if="detailBusy" class="empty">{{ t('admin.copy.19bfd4ec') }}</p>
        <p v-if="detailError" class="error" role="alert">{{ display(detailError) }}</p>
        <template v-if="selectedOrder">
          <div class="detail-top">
            <span class="badge" :class="statusClass(selectedOrder.status)">{{
              statusName(selectedOrder.status)
            }}</span>

            <time>{{ displayDate(selectedOrder.placed_at) }}</time>
          </div>
          <div class="facts">
            <div>
              <small>{{ t('admin.copy.9c32661d') }}</small><b><AssociationName :record="selectedOrder" kind="brand" :id="selectedOrder.brand_id" :locale="associationLocale" /></b>
            </div>
            <div>
              <small>{{ t('admin.copy.ac20b7ad') }}</small><b><AssociationName :record="selectedOrder" kind="member" :id="selectedOrder.brand_member_id" :locale="associationLocale" /></b>
            </div>
            <div>
              <small>{{ t('admin.copy.62839920') }}</small
              ><b><AssociationName :record="selectedOrder" kind="user" :id="selectedOrder.global_user_id" :locale="associationLocale" /></b>
            </div>
            <div>
              <small>{{ t('admin.copy.4ee99d60') }}</small
              ><b
                ><AssociationName :record="selectedOrder" kind="game" :id="selectedOrder.game_id" :locale="associationLocale" /> / <AssociationName :record="selectedOrder" kind="period" :id="selectedOrder.period_id" :locale="associationLocale" /></b
              >
            </div>
            <div>
              <small>{{ t('admin.copy.b6b073d7') }}</small><b><AssociationName :record="selectedOrder" kind="play" :id="selectedOrder.play_id" :locale="associationLocale" /></b>
            </div>
            <div>
              <small>{{ t('admin.copy.14e68878') }}</small
              ><b>{{ selectedOrder.rule_version_id }}</b>
            </div>
            <div>
              <small>{{ t('admin.copy.ed5261b2') }}</small
              ><b></b>
            </div>
          </div>
          <div class="amounts">
            <div v-for="row in amountRows" :key="row[0]">
              <small>{{ amountLabel(row[0]) }}</small
              ><b
                >{{ row[1]
                }}<template v-if="row[0] === '单注积分' || row[0] === '总扣款'">
                  {{ ' ' + t('admin.copy.2f31ccd0') }}</template
                ><template v-else-if="row[0] === '倍数'">{{ ' ' + t('admin.copy.b103c550') }}</template></b
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
            <h3>{{ t('admin.copy.45b3e677') }}</h3>
            <div
              class="tiers"
              v-if="selectedOrder.definition_snapshot.prize_tiers?.length"
            >
              <div
                v-for="tier in selectedOrder.definition_snapshot.prize_tiers"
                :key="tier.code"
              >
                <b>{{ tier.code }}</b
                ><span>{{ t('admin.copy.5e931966') }} {{ tier.odds }}</span
                ><span>{{ tier.exclusive ? t('admin.copy.8c316d58') : t('admin.copy.fc92d21e') }}</span
                ><span v-if="tier.cap_points"
                  >{{ t('admin.copy.6c53084a') }} {{ tier.cap_points }} {{ t('admin.copy.2f31ccd0') }}</span
                >
              </div>
            </div>
            <p v-else class="muted">{{ t('admin.copy.aa76ca22') }}</p>
            <details>
              <summary>{{ t('admin.copy.78d66c92') }}</summary>
              <BusinessDetails :value="selectedOrder.definition_snapshot" :locale="associationLocale" />
            </details>
          </section>

          <section class="subsection">
            <h3>{{ t('admin.copy.ec1dc383') }}</h3>
            <div class="selection-grid">
              <div>
                <h4>{{ t('admin.copy.c2129638') }}</h4>
                <BusinessDetails :value="selectedOrder.selection_raw" :locale="associationLocale" />
              </div>
              <div>
                <h4>{{ t('admin.copy.1f8d01c9') }}</h4>
                <BusinessDetails :value="selectedOrder.selection_normalized" :locale="associationLocale" />
              </div>
            </div>
          </section>
          <section class="subsection">
            <h3>{{ t('admin.copy.a1ab0eed') }} {{ BET_COMBINATION_PREVIEW_LIMIT }} {{ t('admin.copy.6a5f2e4c') }}</h3>
            <BusinessDetails :value="preview.items" :locale="associationLocale" />
            <p class="muted">
              {{ t('admin.copy.acbd5616') }} {{ preview.total }} {{ t('admin.copy.4cc490fd') }}<template v-if="preview.remaining"
                >，{{ t('admin.copy.dfef43ef') }} {{ preview.remaining }} {{ t('admin.copy.95856f76') }}</template
              >
            </p>
          </section>

          <section class="subsection">
            <h3>{{ t('admin.copy.56a04dab') }}</h3>
            <div class="facts">
              <div>
                <small>{{ t('admin.copy.d2f7f45c') }}</small
                ><b>{{ selectedOrder.debit_entry_id }}</b>
              </div>
              <div>
                <small>{{ t('admin.copy.f713fe99') }}</small
                ><b>{{ selectedOrder.refund_entry_id || t('admin.copy.c2b41b33') }}</b>
              </div>
              <div>
                <small>{{ t('admin.copy.67dbba5b') }}</small
                ><b>{{ displayDate(selectedOrder.cancelled_at) }}</b>
              </div>
              <div>
                <small>{{ t('admin.copy.cd19a931') }}</small
                ><b>{{ selectedOrder.cancel_reason || t('admin.copy.c2b41b33') }}</b>
              </div>
            </div>
            <div class="allocation">
              <b>{{ t('admin.copy.d1d97505') }}</b>
              <p
                v-for="(entry, index) in selectedOrder.deduction_allocation"
                :key="`${entry.source}-${entry.state}-${index}`"
              >
                {{ entry.source }} · <BusinessDetails :value="entry.state" :locale="associationLocale" />: {{ entry.points }} {{ t('admin.copy.2f31ccd0') }}
              </p>
            </div>
          </section>
          <section class="subsection">
            <h3>{{ t('admin.copy.9cd7b70b') }}</h3>
            <div class="facts">
              <div>
                <small>{{ t('admin.copy.8d2e4cc1') }}</small
                ><b>{{
                  selectedOrder.policy_snapshot.user_cancel_allowed
                    ? t('admin.copy.1a264f45')
                    : t('admin.copy.b92bb6e0')
                }}</b>
              </div>
            </div>
            <details>
              <summary>{{ t('admin.copy.2b707235') }}</summary>
              <BusinessDetails :value="selectedOrder.policy_snapshot" :locale="associationLocale" />
            </details>
          </section>

          <section class="subsection evidence">
            <h3>{{ t('admin.copy.51c55ad6') }}</h3>
            <p v-global-loading v-if="detailBusy" class="muted" aria-live="polite">{{ t('admin.copy.143f6e85') }}</p>
            <template v-else-if="exception"
              ><div class="facts">
                <div>
                  <small>{{ t('admin.copy.69271cf5') }}</small><b>{{ exception.id }}</b>
                </div>
                <div>
                  <small>{{ t('admin.copy.5aa58ea7') }}</small><b v-if="exception.source === 'system'">{{ t('admin.copy.9d8ab686') }} · {{ exception.error_code ? displayPublicError(publicErrorMessage({ code: exception.error_code })) : "—" }}</b><b v-else><AssociationName :record="exception" kind="admin" :id="exception.marked_by" :locale="associationLocale" /></b>
                </div>
                <div>
                  <small>{{ t('admin.copy.1f233b75') }}</small><b>{{ displayDate(exception.created_at) }}</b>
                </div>
              </div>
              <p class="reason-box">{{ exception.reason }}</p></template
            >
            <p v-else class="muted empty-state">{{ t('admin.copy.dd3601b3') }}</p>
          </section>

          <section class="subsection judgment-evidence">
            <h3>{{ t('admin.copy.8d79e526') }}</h3>
            <p v-global-loading v-if="detailBusy" class="muted" aria-live="polite">{{ t('admin.copy.8abd8e4b') }}</p>
            <template v-else-if="judgment">
              <div class="facts">
                <div>
                  <small>{{ t('admin.copy.b07bab35') }}</small><b>{{ judgment.id }}</b>
                </div>
                <div>
                  <small>{{ t('admin.copy.33086111') }}</small
                  ><b>{{
                    judgment.cause === "no_result" ? t('admin.copy.afd3d8be') : t('admin.copy.584eb6d6')
                  }}</b>
                </div>
                <div>
                  <small>{{ t('admin.copy.7a147774') }}</small><b><AssociationName :record="judgment" kind="admin" :id="judgment.judged_by" :locale="associationLocale" /></b>
                </div>
                <div>
                  <small>{{ t('admin.copy.eef2718f') }}</small><b>{{ displayDate(judgment.created_at) }}</b>
                </div>
                <div>
                  <small>{{ t('admin.copy.8507c9b3') }}</small
                  ><b>{{ judgment.draw_result_id || t('admin.copy.c2b41b33') }}</b>
                </div>
                <div>
                  <small>{{ t('admin.copy.59614e66') }}</small
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
              {{ t('admin.copy.d5063118') }}
            </p>
            <p v-else-if="judgmentRead" class="muted empty-state">
              {{ t('admin.copy.a36f78b3') }}
            </p>
            <p v-else class="muted">{{ t('admin.copy.ae3cc499') }}</p>
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
              class="button button-secondary"
              @click="beginIntent('mark-abnormal')"
            >
              {{ t('admin.copy.cceca499') }}
            </button>
            <button
              v-if="canCancelSelected"
              class="danger-button"
              @click="beginIntent('cancel')"
            >
              {{ t('admin.copy.42a5589b') }}
            </button>
            <button
              v-if="canJudgeCancelSelected"
              class="danger-button"
              @click="beginIntent('judge-cancel')"
            >
              {{ t('admin.copy.8b724b7d') }}
            </button>
            <span
              v-if="selectedOrder.status === 'abnormal' && rights.markAbnormal"
              class="muted"
              >{{ t('admin.copy.6cddb1b9') }}</span
            >
            <span
              v-if="!['placed', 'abnormal'].includes(selectedOrder.status)"
              class="muted"
              >{{ t('admin.copy.ce06895d') }}</span
            >
          </div>
          <p v-if="notice" class="success-note" role="status">{{ display(notice) }}</p>
          <p v-if="conflictNotice" class="conflict-note" role="status">
            {{ display(conflictNotice) }}
          </p>
          <p v-if="writeError" class="error" role="alert">{{ display(writeError) }}</p>
          <div
            v-if="
              pendingUnknown &&
              pending?.brandId === brandId &&
              pending?.orderId === selectedOrder.id &&
              rights.ordersView
            "
            class="reconcile-box"
          >
            <p>{{ t('admin.copy.074f6ff2') }}</p>
            <button
              class="button button-secondary"
              :disabled="detailBusy"
              @click="withGlobalLoading(reconcileFrozen)"
            >
              {{ t('admin.copy.376a0429') }}
            </button>
            <button
              v-write-guard="writeBusy"
              v-if="
                pending.action === 'cancel'
                  ? rights.cancel
                  : pending.action === 'mark-abnormal'
                    ? rights.markAbnormal
                    : rights.judgeCancel
              "
              class="danger-button"
              :disabled="writeBusy"
              @click="withGlobalLoading(retryFrozen)"
            >
              {{ writeBusy ? t('admin.copy.3cd36df0') : t('admin.copy.7d7a2bcb') }}
            </button>
          </div>
        </template>
      </section>
</DetailModal>
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
            <h2 id="bet-action-title">
              {{
                intent.action === "cancel"
                  ? t('admin.copy.e78ae248')
                  : intent.action === "mark-abnormal"
                    ? t('admin.copy.a6803e05')
                    : t('admin.copy.e5b3c548')
              }}
            </h2>
          </div>
          <button class="text-button" @click="closeIntent">{{ t('admin.copy.2bb2cb1d') }}</button>
        </div>
        <p v-if="intent.action === 'cancel'" class="warning">
          {{ t('admin.copy.1d790408') }}
        </p>
        <p v-else-if="intent.action === 'mark-abnormal'" class="warning">
          {{ t('admin.copy.ea7b786f') }}
        </p>
        <p v-else class="warning">
          {{ t('admin.copy.95ffd71d') }}
        </p>
        <div v-if="selectedOrder" class="confirm-facts">
          <p>
            {{ t('admin.copy.c083fb3e') }}<b>{{ selectedOrder.id }}</b>
          </p>
          <p>
            {{ t('admin.copy.546942e1') }}<b>{{ statusName(selectedOrder.status) }}</b>
          </p>
        </div>
        <label v-if="intent.action === 'judge-cancel'" class="reason-label">
          {{ t('admin.copy.33086111') }}
          <select
            v-model="judgmentCause"
            :aria-label="t('admin.copy.2d7f5472')"
            :disabled="confirmed || writeBusy"
          >
            <option value="no_result">{{ t('admin.copy.f04ceb54') }}</option>
            <option value="invalid_result">{{ t('admin.copy.9aec2fe0') }}</option>
          </select>
        </label>
        <p v-if="intent.action === 'judge-cancel'" class="muted">
          {{ t('admin.copy.12c256f5') }}
        </p>

        <p v-if="reason" class="byte-count" :class="{ over: reasonBytes > 500 }">
          {{ reasonBytes }} / {{ t('admin.copy.c8116369') }}
        </p>
        <template v-if="confirmed"
          ><div class="confirm-facts">
            <p>{{ t('admin.copy.c0c16887') }}<BusinessDetails :value="pending?.body" :locale="associationLocale" /></p>
            <p>
              {{ t('admin.copy.0aea2415') }}<b>{{ pending?.idempotencyKey }}</b>
            </p>
          </div>
          <label class="confirm-check"
            ><input v-model="confirmed" type="checkbox" />
            {{ t('admin.copy.4e9006ad') }}{{
              intent.action === "cancel"
                ? t('admin.copy.19b140e9')
                : intent.action === "mark-abnormal"
                  ? t('admin.copy.2d955a5e')
                  : t('admin.copy.9368b98b')
            }}。</label
          ></template
        >
          <p v-if="writeError" class="error" role="alert">{{ display(writeError) }}</p>
        <div class="dialog-actions">
          <button
            class="button button-secondary"
            :disabled="writeBusy || pendingUnknown"
            @click="closeIntent"
          >
            {{ t('admin.copy.b1d257a2') }}</button
          ><button
            v-if="!confirmed"
            class="button button-primary"
            :disabled="!validBetReason(reason) || writeBusy"
            @click="freezeIntent"
          >
            {{ t('admin.copy.bdba59e3') }}</button
          ><button
            v-write-guard="writeBusy"
            v-else
            class="danger-button"
            :disabled="!confirmed || writeBusy || !pending"
            @click="withGlobalLoading(submitConfirmed)"
          >
            {{ writeBusy ? t('admin.copy.46511dc0') : t('admin.copy.3135aeca') }}
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
  color: var(--muted);
  margin: 5px 0 0;
}
.eyebrow {
  color: #858da0;
  font-size: 11px;
  letter-spacing: 1.2px;
}
.panel {
  background: var(--surface);
  border: 1px solid var(--line);
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
  background: var(--surface);
  min-width: 0;
}
button {
  border: 0;
  border-radius: 7px;
  padding: 9px 13px;
  font-weight: 600;
}
.button-primary {
  color: var(--surface);
  background: var(--primary);
}
.button-secondary {
  color: #48536b;
  background: #f1f3f7;
  border: 1px solid #e3e6ed;
}
.danger-button {
  color: var(--surface);
  background: #bd3e4c;
}
.text-button {
  color: var(--primary);
  background: transparent;
  padding: 5px;
}
.notice {
  color: var(--muted);
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
  background: var(--surface);
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
  color: var(--muted);
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
  color: var(--primary);
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
  background: var(--surface);
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
  color: var(--muted);
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
@media (max-width: 1100px) {
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
