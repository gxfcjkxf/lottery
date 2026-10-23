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
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
const displayDate = (value: string | null | undefined) => formatDisplayDateTime(value, displayLocale.value);
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import type { LocalizedMessage } from "@lottery/shared";
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useAdminI18n } from "./i18n";
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
const { t, message: localized, display } = useAdminI18n();
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
const error = ref<string | LocalizedMessage>("");
const notice = ref<string | LocalizedMessage>("");
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
  error.value = problem instanceof Error ? publicErrorMessage(problem) : localized('admin.copy.4fb9cc4e');
}
function checkContext(value: Context) {
  if (!samePreviewContext(value, {
    brandId: props.brandId,
    orderId: props.order.id,
    orderVersion: value.order_version,
  })) throw new Error(t('admin.copy.b560a501'));
  if (value.game_id !== props.order.game_id || value.period_id !== props.order.period_id)
    throw new Error(t('admin.copy.63e3a238'));
  if (props.order.brand_id !== props.brandId)
    throw new Error(t('admin.copy.9bd2a6e4'));
}
function checkPreview(value: Preview) {
  if (
    value.brand_id !== props.brandId || value.order_id !== props.order.id ||
    value.game_id !== props.order.game_id || value.period_id !== props.order.period_id
  ) throw new Error(t('admin.copy.454b13da'));
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
  })) throw new Error(t('admin.copy.0cf12503'));
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
      throw new Error(t('admin.copy.3765f23c'));
    for (const item of nextHistory.items) checkPreview(item);
    context.value = nextContext;
    history.value = nextHistory.items;
    historyHasMore.value = nextHistory.has_more;
    historyOffset.value = nextHistory.offset;
    const previewId = keepId || currentRecord.value?.id || "";
    selectedPreviewId.value = previewId;
    if (previewId) void loadPreview(previewId, capturedScope);
    if (nextContext.order_version !== props.order.version)
      notice.value = localized('admin.copy.e8754dab');
    else if (nextContext.definition_hash !== props.order.definition_hash)
      notice.value = localized('admin.copy.771e7b51');
    else if (nextContext.order_status !== props.order.status)
      notice.value = localized('admin.copy.a9280bdd');
    else notice.value = localized('admin.copy.2efdb65c');
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
      throw new Error(t('admin.copy.3765f23c'));
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
      throw new Error(t('admin.copy.f7afa7ae'));
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

async function selectHistory(item: Preview) {
  if (writing.value) return;
  selectedPreviewId.value = item.id;
  selectedPreview.value = null;
  previewLane.invalidate();
  linesLane.invalidate();
  linesPage.value = null;
  linesOffset.value = 0;
  linesBusy.value = false;
  error.value = "";
  await loadPreview(item.id, scope.value);
}
function reviewPreview() {
  const currentContext = context.value;
  const cleanReason = reason.value.trim();
  if (!currentContext || !rights.value.preview) return;
  if (currentContext.order_version !== props.order.version) {
    error.value = localized('admin.copy.3f3f45af');
    return;
  }
  if (currentContext.definition_hash !== props.order.definition_hash) {
    error.value = localized('admin.copy.d893d669');
    return;
  }
  if (currentContext.order_status !== props.order.status) {
    error.value = localized('admin.copy.73ea49a4');
    return;
  }
  if (!currentContext.can_preview || !currentContext.draw_result_id) {
    error.value = localized('admin.copy.c2157afa');
    return;
  }
  if (!validPreviewReason(cleanReason)) {
    error.value = localized('admin.copy.5d042c77');
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
    throw new Error(t('admin.copy.0fbac57f'));
  if (nextHistory.brand_id !== props.brandId || nextHistory.order_id !== props.order.id)
    throw new Error(t('admin.copy.3765f23c'));
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
  notice.value = unknownWrite.value ? localized('admin.copy.9c2c6c7f') : localized('admin.copy.7d52672d');
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
      notice.value = localized('admin.copy.0e49024f');
    } else {
      frozenWrites.forget(intent);
      pendingWrite.value = null;
      unknownWrite.value = false;
      reviewBody.value = null;
      reviewConfirmed.value = false;
      notice.value = failure === "conflict"
        ? localized('admin.copy.cac962c1')
        : localized('admin.copy.9756bb62');
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
      notice.value = localized('admin.copy.0879aa9b');
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
        ? localized('admin.copy.76127eaf')
        : localized('admin.copy.dd83e7a6');
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
      error.value = problem instanceof Error ? publicErrorMessage(problem) : localized('admin.copy.06d5fbe1');
      notice.value = localized('admin.copy.244e68ff');
    }
  } finally {
    if (isCurrent(writeLane, ticket, capturedScope, true)) writing.value = false;
  }
}
async function confirmPreview() {
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
  await submitPending();
}
function formatPoints(value: string | null | undefined) {
  return value == null ? "—" : formatIntegerPoints(value);
}
function resultLabel(outcome: Preview["outcome"]) {
  return ({ won: t('admin.copy.0beda3ec'), lost: t('admin.copy.3333ba80'), abnormal: t('admin.copy.ac8f977e'), excluded: t('admin.copy.f10ff6f0') })[outcome];
}
function orderStatusLabel(status: string) {
  const labels: Record<string, string> = { placed: t('admin.copy.aab1777a'), abnormal: t('admin.copy.7bcf4b54'), bet_cancelled: t('admin.copy.a1834cc2'), judged_cancelled: t('admin.copy.46a60c2c'), won: t('admin.copy.6de62356'), lost: t('admin.copy.e2f3b9bc') };
  return labels[status] ?? status;
}
function periodStatusLabel(status: string) {
  const labels: Record<string, string> = { pending: t('admin.copy.997b3d22'), betting: t('admin.copy.3a20b720'), closed: t('admin.copy.3198ad9b'), waiting_draw: t('admin.copy.aab1777a'), drawn: t('admin.copy.a16e8013'), settling: t('admin.copy.01f90196'), settled: t('admin.copy.037164c2'), bet_cancelled: t('admin.copy.a1834cc2'), judged_cancelled: t('admin.copy.46a60c2c') };
  return labels[status] ?? status;
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
        <h3 :id="`settlement-preview-title-${order.id}`">{{ t('admin.copy.7c7088c4') }}</h3>
      </div>
      <button v-refresh-button="contextBusy || historyBusy ? t('admin.copy.27342bf3') : t('admin.copy.deca021e')" v-if="rights.view" class="sp-secondary button button-secondary" type="button" :disabled="contextBusy || historyBusy || writing" @click="withGlobalLoading(readContextAndHistory, true)">
        {{ contextBusy || historyBusy ? t('admin.copy.27342bf3') : t('admin.copy.deca021e') }}
      </button>
    </header>
    <p v-if="!rights.view" class="sp-note">{{ t('admin.copy.63f33d99') }}</p>
    <template v-else>
      <div class="sp-actions">
        <button class="sp-secondary button button-secondary" type="button" :disabled="contextBusy || historyBusy || writing" @click="withGlobalLoading(readContextAndHistory)">{{ contextBusy || historyBusy ? t('admin.copy.27342bf3') : t('admin.copy.225391a1') }}</button>
      </div>
        <p v-if="error" class="sp-message error" role="alert">{{ display(error) }}</p>
        <p v-if="notice" class="sp-message" role="status">{{ display(notice) }}</p>
      <p v-if="!rights.preview" class="sp-note">{{ t('admin.copy.d92d9167') }}</p>
      <div v-if="rights.preview && pendingWrite" class="sp-pending">
        <strong>{{ unknownWrite ? t('admin.copy.880cebfc') : t('admin.copy.67156c50') }}</strong>
        <p>{{ unknownWrite ? t('admin.copy.29f93441') : t('admin.copy.1dd98e35') }}</p>
        <button v-write-guard="writing" class="sp-primary button button-primary" type="button" :disabled="writing || pendingWrite.accountId !== account.id || pendingWrite.brandId !== brandId || pendingWrite.orderId !== order.id" @click="withGlobalLoading(submitPending)">{{ writing ? t('admin.copy.fb8562cf') : t('admin.copy.c5cb7c41') }}</button>
      </div>
      <div v-if="context" class="sp-panel">
        <div class="sp-section-title"><h4>{{ t('admin.copy.16d19e0f') }}</h4><span class="sp-badge" :class="context.can_preview && !staleOrder && !ruleHashMismatch && !orderStatusMismatch ? 'ready' : 'blocked'">{{ context.can_preview && !staleOrder && !ruleHashMismatch && !orderStatusMismatch ? t('admin.copy.22e014b4') : t('admin.copy.b5abb68b') }}</span></div>
        <dl class="sp-facts">
          <div><dt>{{ t('admin.copy.e45c6d5f') }}</dt><dd>{{ orderStatusLabel(context.order_status) }}</dd></div>
          <div><dt>{{ t('admin.copy.1f232918') }}</dt><dd>{{ periodStatusLabel(context.period_status) }}</dd></div>
          <div><dt>{{ t('admin.copy.225941c3') }}</dt><dd class="sp-break">{{ context.draw_result_id || "—" }}</dd></div>
          <div><dt>{{ t('admin.copy.b4f4b2b5') }}</dt><dd class="sp-break"><BusinessDetails :value="context.draw" :locale="displayLocale" /></dd></div>



        </dl>
        <p v-if="context.order_version !== order.version" class="sp-warning compact">{{ t('admin.versionDisplay.staleOrder') }}</p>
        <p v-if="ruleHashMismatch" class="sp-warning compact">{{ t('admin.copy.e2e031a3') }}</p>
        <p v-if="orderStatusMismatch" class="sp-warning compact">{{ t('admin.copy.197f0f39') }}</p>
        <template v-if="rights.preview">
          <div v-if="!pendingWrite" class="sp-form">

            <small v-if="reason">{{ reasonBytes }} / {{ t('admin.copy.c8116369') }}</small>
            <button class="sp-primary button button-primary" type="button" :disabled="!canStartPreview || !validPreviewReason(reason)" @click="reviewPreview">{{ t('admin.copy.3f3c073d') }}</button>
            <div v-if="reviewBody" class="sp-confirm">
              <strong>{{ t('admin.copy.5c14763f') }}</strong>
              <p> {{ t('admin.copy.0336a9c7') }} {{ reviewBody.draw_result_id }}</p>
              <p>{{ t('admin.copy.2809a1e6') }}{{ reviewBody.reason }}</p>
              <label class="sp-check"><input v-model="reviewConfirmed" type="checkbox" />{{ t('admin.copy.fe072a12') }}</label>
              <button v-write-guard="writing" class="sp-primary button button-primary" type="button" :disabled="!reviewConfirmed || writing" @click="withGlobalLoading(confirmPreview)">{{ writing ? t('admin.copy.ee73ed7b') : t('admin.copy.6474905c') }}</button>
            </div>
          </div>
        </template>
      </div>

      <div v-if="history.length || currentRecord" class="sp-panel">
        <div class="sp-section-title"><h4>{{ t('admin.copy.95130f18') }}</h4><span class="sp-note-inline">{{ t('admin.copy.5cb7f04d') }} {{ SETTLEMENT_PREVIEW_PAGE_SIZE }} {{ t('admin.copy.067fed38') }}</span></div>
        <div class="sp-history">
          <button v-for="item in history" :key="item.id" class="sp-history-item" :class="{ selected: selectedPreviewId === item.id }" type="button" :disabled="writing" @click="withGlobalLoading(selectHistory, item)">
            <span>{{ displayDate(item.created_at) }}</span><b>{{ resultLabel(item.outcome) }}</b><small>{{ item.current ? t('admin.copy.99a7cf58') : t('admin.copy.170fb916') }}</small>
          </button>
        </div>
        <div class="sp-pagination"><button class="sp-secondary button button-secondary" type="button" :disabled="historyOffset <= 0 || historyBusy || writing" @click="withGlobalLoading(readHistoryPage, Math.max(0, historyOffset - SETTLEMENT_PREVIEW_PAGE_SIZE))">{{ t('admin.copy.dbe7a5d7') }}</button><button class="sp-secondary button button-secondary" type="button" :disabled="!historyHasMore || historyBusy || writing" @click="withGlobalLoading(readHistoryPage, historyOffset + SETTLEMENT_PREVIEW_PAGE_SIZE)">{{ t('admin.copy.2e9c9e7a') }}</button></div>
      </div>

      <div v-if="selectedPreview" class="sp-panel">
        <div class="sp-section-title"><h4>{{ t('admin.copy.0d8d2143') }}</h4><span class="sp-badge" :class="resultClass(selectedPreview.outcome)">{{ resultLabel(selectedPreview.outcome) }}</span></div>
        <p v-if="!selectedPreview.current" class="sp-warning compact">{{ t('admin.copy.34ae4fb1') }}</p>
        <p v-if="selectedPreview.outcome === 'abnormal' || selectedPreview.outcome === 'excluded'" class="sp-warning compact">{{ t('admin.copy.7a5fd891') }}</p>
        <dl class="sp-facts">
          <div><dt>{{ t('admin.copy.d670976f') }}</dt><dd class="sp-break">{{ selectedPreview.id }}</dd></div>
          <div><dt>{{ t('admin.copy.57855772') }}</dt><dd>{{ selectedPreview.current ? t('admin.copy.a83d9ef8') : t('admin.copy.fbd68efe') }}</dd></div>
          <div><dt>{{ t('admin.copy.2ed716bf') }}</dt><dd>{{ selectedPreview.error_code ? displayPublicError(publicErrorMessage({ code: selectedPreview.error_code })) : "—" }}</dd></div>
          <div><dt>{{ t('admin.copy.1bdec815') }}</dt><dd>{{ selectedPreview.calculation ? `${selectedPreview.calculation.won ? t('admin.copy.1a264f45') : t('admin.copy.b92bb6e0')} / ${selectedPreview.calculation.combination_count}` : "—" }}</dd></div>
          <div><dt>{{ t('admin.copy.a062ae4f') }}</dt><dd>{{ selectedPreview.calculation?.multiplier ?? "—" }}</dd></div>
          <div><dt>{{ t('admin.copy.86f4cbba') }}</dt><dd>{{ formatPoints(selectedPreview.calculation?.bet_points) }}</dd></div>
          <div><dt>{{ t('admin.copy.c1894407') }}</dt><dd>{{ formatPoints(selectedPreview.calculation?.raw_prize_points) }}</dd></div>
          <div><dt>{{ t('admin.copy.cf044177') }}</dt><dd>{{ formatPoints(selectedPreview.calculation?.capped_prize_points) }}</dd></div>
          <div><dt>{{ t('admin.copy.958800e1') }}</dt><dd>{{ formatPoints(selectedPreview.calculation?.prize_points) }}</dd></div>

          <div><dt>{{ t('admin.copy.4a7743c8') }}</dt><dd class="sp-break"><AssociationName :record="selectedPreview" kind="admin" :id="selectedPreview.created_by" :locale="associationLocale" /> {{ displayDate(selectedPreview.created_at) }}</dd></div>

        </dl>
        <div class="sp-section-title sp-lines-heading"><h4>{{ t('admin.copy.78ce40ee') }}</h4><span class="sp-note-inline">{{ t('admin.copy.5cb7f04d') }} {{ SETTLEMENT_PREVIEW_PAGE_SIZE }} {{ t('admin.copy.ce1ae1a7') }}</span></div>
        <p v-global-loading v-if="linesBusy" class="sp-note">{{ t('admin.copy.be0d0f79') }}</p>
        <p v-else-if="linesPage" class="sp-note">{{ t('admin.copy.acbd5616') }} {{ linesPage.total }} {{ t('admin.copy.8dc49f98') }} {{ linesPage.items.length }} {{ t('admin.copy.3128c12e') }}</p>
        <div v-if="linesPage" class="sp-lines">
          <article v-for="(line, index) in linesPage.items" :key="`${linesOffset}-${index}`" class="sp-line">
            <div class="sp-line-head"><b>{{ t('admin.copy.681138ab') }} {{ linesOffset + index + 1 }}</b><strong>{{ formatPoints(line.points) }} {{ t('admin.copy.feacf287') }}</strong></div>
            <details><summary>{{ t('admin.copy.d7f07368') }}{{ line.hits.length }} {{ t('admin.copy.8de26184') }}</summary><BusinessDetails :value="line.selection" :locale="associationLocale" /><div v-for="(hit, hitIndex) in line.hits" :key="`${hit.code}-${hitIndex}`" class="sp-hit"><b>{{ hit.code }} · {{ hit.exclusive ? t('admin.copy.f36934e9') : t('admin.copy.1932d335') }} · {{ hit.matched ? t('admin.copy.b74bcd82') : t('admin.copy.a56e9a5f') }}</b><span>{{ t('admin.copy.ff1d7325') }} {{ hit.selected }} · {{ t('admin.copy.bcecdbf2') }} <BusinessDetails :value="hit.trace.actual" :locale="associationLocale" /> · {{ t('admin.copy.1eae5f29') }} {{ formatPoints(hit.raw_points) }} · {{ t('admin.copy.2cd9734f') }} {{ formatPoints(hit.capped_points) }} · {{ t('admin.copy.e4b26b37') }} {{ formatPoints(hit.points) }}</span><details><summary>{{ t('admin.copy.e0715b8a') }}</summary><BusinessDetails :value="hit.trace" :locale="associationLocale" /></details></div></details>
          </article>
        </div>
        <div v-if="linesPage" class="sp-pagination"><button class="sp-secondary button button-secondary" type="button" :disabled="linesOffset <= 0 || linesBusy || writing" @click="withGlobalLoading(loadLines, Math.max(0, linesOffset - SETTLEMENT_PREVIEW_PAGE_SIZE))">{{ t('admin.copy.cd3b31b1') }}</button><button class="sp-secondary button button-secondary" type="button" :disabled="linesOffset + linesPage.items.length >= linesPage.total || linesBusy || writing" @click="withGlobalLoading(loadLines, linesOffset + SETTLEMENT_PREVIEW_PAGE_SIZE)">{{ t('admin.copy.4fe9cc14') }}</button></div>
      </div>
    </template>
  </section>
</template>

<style scoped>
.settlement-preview { min-width: 0; display: grid; gap: 12px; color: #252a36; }
.sp-heading, .sp-section-title, .sp-pagination, .sp-line-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; min-width: 0; }
.sp-heading h3, .sp-section-title h4 { margin: 0; font-size: 14px; }
.sp-eyebrow { margin: 0 0 3px; color: #8991a0; font-size: 11px; }
.sp-panel { min-width: 0; padding: 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--surface); }
.sp-warning, .sp-note, .sp-pending, .sp-confirm { margin: 0; padding: 10px; border-radius: 7px; background: #fff7e8; color: #704d18; line-height: 1.55; overflow-wrap: anywhere; }
.sp-warning.compact { margin-top: 10px; padding: 8px; }
.sp-note { background: #f5f6f9; color: #687083; }
.sp-actions { display: flex; flex-wrap: wrap; gap: 8px; }
.sp-primary, .sp-secondary { min-height: 34px; max-width: 100%; padding: 7px 11px; border: 1px solid #dfe2e9; border-radius: 6px; background: var(--surface); color: #384154; font: inherit; cursor: pointer; }
.sp-primary { border-color: var(--primary); background: var(--primary); color: white; }
.sp-primary:disabled, .sp-secondary:disabled { opacity: .5; cursor: not-allowed; }
.sp-message { margin: 0; padding: 9px; border-radius: 6px; background: #eef1ff; color: #4758b9; overflow-wrap: anywhere; }
.sp-message.error { background: #fff0ef; color: #a3332d; }
.sp-badge { flex: 0 0 auto; padding: 4px 7px; border-radius: 999px; background: #edf0f5; color: #687083; font-size: 11px; }
.sp-badge.ready, .sp-badge.good { background: #e8f5ef; color: #25734e; }
.sp-badge.blocked, .sp-badge.alert { background: #fff0ef; color: #a3332d; }
.sp-badge.quiet { background: #f0f2f5; color: #596174; }
.sp-facts { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; margin: 12px 0 0; }
.sp-facts div { min-width: 0; padding: 8px; border-radius: 6px; background: #f8f9fb; }
.sp-facts dt { margin-bottom: 4px; color: #788092; font-size: 11px; }
.sp-facts dd { margin: 0; font-size: 12px; overflow-wrap: anywhere; word-break: break-word; }
.sp-break { word-break: break-all; }
.sp-form { display: grid; gap: 8px; margin-top: 12px; }
.sp-form label { display: grid; gap: 5px; font-size: 11px; }
.sp-form textarea { width: 100%; min-width: 0; resize: vertical; padding: 8px; border: 1px solid #dfe2e9; border-radius: 6px; }
.sp-form small, .sp-note-inline { color: #8790a0; font-size: 11px; }
.sp-confirm, .sp-pending { display: grid; gap: 8px; background: #f5f6ff; color: #3f4c94; }
.sp-confirm p, .sp-pending p { margin: 0; overflow-wrap: anywhere; }
.sp-check { display: flex !important; align-items: flex-start; }
.sp-check input { flex: 0 0 auto; margin: 2px 0 0; }
.sp-history { display: grid; gap: 6px; margin-top: 10px; }
.sp-history-item { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 4px 8px; min-width: 0; padding: 8px; border: 1px solid var(--line); border-radius: 6px; background: var(--surface); text-align: left; overflow-wrap: anywhere; }
.sp-history-item.selected { border-color: #8390ed; background: #f7f8ff; }
.sp-history-item span { color: #788092; font-size: 11px; }
.sp-history-item b { font-size: 11px; }
.sp-history-item small { grid-column: 1 / -1; color: #8790a0; }
.sp-pagination { justify-content: flex-start; flex-wrap: wrap; margin-top: 9px; }
.sp-lines-heading { margin-top: 16px; }
.sp-lines { display: grid; gap: 7px; margin-top: 8px; }
.sp-line { min-width: 0; padding: 9px; border: 1px solid var(--line); border-radius: 6px; }
.sp-line-head { margin-bottom: 7px; font-size: 11px; }
.sp-hit { display: grid; gap: 4px; padding: 8px 0; border-top: 1px solid #eceef2; overflow-wrap: anywhere; }
.sp-hit span { color: #697184; font-size: 11px; }
details { min-width: 0; }
summary { color: #4e5fc9; cursor: pointer; font-size: 11px; }
pre { max-width: 100%; margin: 7px 0; padding: 8px; border-radius: 5px; background: #f5f6f8; white-space: pre-wrap; overflow-wrap: anywhere; word-break: break-word; font: 11px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace; }
@media (max-width: 480px) {
  .sp-panel { padding: 9px; }
  .sp-facts { grid-template-columns: minmax(0, 1fr); }
  .sp-history-item { grid-template-columns: minmax(0, 1fr); }
  .sp-history-item small { grid-column: auto; }
}
</style>
