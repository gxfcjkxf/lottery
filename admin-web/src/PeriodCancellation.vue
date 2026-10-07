<script setup lang="ts">
import type { LocalizedMessage } from "@lottery/shared";
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { useAdminI18n } from "./i18n";
import {
  AdminApiError,
  createIdempotencyKey,
  type AdminAccount,
} from "./admin-api";
import {
  createPeriodCancellationApi,
  periodCancellationPermissions,
  type Cancellation,
} from "./period-cancellation-api";
import { createPeriodSchedulesApi, type Period } from "./period-schedules-api";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, message: localized } = useAdminI18n();

const api = createPeriodCancellationApi();
const periodsApi = createPeriodSchedulesApi();
const generations = new Map<string, number>();
let alive = true;
const guard = {
  capture(lane: string, requestScope: string) {
    const generation = (generations.get(lane) ?? 0) + 1;
    generations.set(lane, generation);
    return { lane, generation, scope: requestScope };
  },
  invalidate(lane?: string) {
    const lanes = lane ? [lane] : [...generations.keys()];
    for (const name of lanes)
      generations.set(name, (generations.get(name) ?? 0) + 1);
  },
  isCurrent(
    ticket: { lane: string; generation: number; scope: string },
    requestScope: string,
    permitted: boolean,
  ) {
    return (
      alive &&
      permitted &&
      ticket.scope === requestScope &&
      generations.get(ticket.lane) === ticket.generation
    );
  },
};
const rights = computed(() =>
  periodCancellationPermissions(props.account, props.brandId),
);

const gameId = ref("");
const periods = ref<Period[]>([]);
const directPeriod = ref<Period | null>(null);
const loadingPeriod = ref(false);
const cancellation = ref<Cancellation | null>(null);
const summaryLoaded = ref(false);
const loadingPeriods = ref(false);
const loadingSummary = ref(false);
const writing = ref(false);
const error = ref<string | LocalizedMessage>("");
const notice = ref<string | LocalizedMessage>("");
type CancelMode = Cancellation["mode"];
type CancelCause = Cancellation["cause"];
type PeriodCancellationBody = {
  version: number;
  mode: CancelMode;
  cause: CancelCause;
  reason: string;
};
type PeriodCancellationRetryBody = { version: number; reason: string };
const mode = ref<CancelMode>("bet_cancelled");
const cause = ref<CancelCause>("operator_cancel");
const reason = ref("");
const retryReason = ref("");
const retryVersionInput = ref("");
const confirmStage = ref<"idle" | "review" | "retry-review">("idle");

type PendingWrite = {
  accountId: string;
  brandId: string;
  periodId: string;
  operation: "cancel" | "retry";
  body: PeriodCancellationBody | PeriodCancellationRetryBody;
  key: string;
};
function pendingStorageKey(
  accountId = props.account.id,
  brandId = props.brandId,
) {
  return `lottery.admin.period-cancellation.pending.v1:${accountId}:${brandId}`;
}
function restorePendingWrite(): PendingWrite | null {
  try {
    if (typeof sessionStorage === "undefined") return null;
    const value = JSON.parse(
      sessionStorage.getItem(pendingStorageKey()) ?? "null",
    ) as Partial<PendingWrite> | null;
    if (
      !value ||
      typeof value !== "object" ||
      typeof value.accountId !== "string" ||
      typeof value.brandId !== "string" ||
      typeof value.periodId !== "string" ||
      !validUuid(value.periodId) ||
      typeof value.key !== "string" ||
      !value.key
    )
      return null;
    if (value.accountId !== props.account.id || value.brandId !== props.brandId)
      return null;
    if (value.operation === "cancel") {
      const body = value.body as Partial<PeriodCancellationBody> | undefined;
      if (
        !body ||
        !Number.isSafeInteger(body.version) ||
        Number(body.version) <= 0 ||
        !["bet_cancelled", "judged_cancelled"].includes(String(body.mode)) ||
        !["operator_cancel", "no_result", "invalid_result"].includes(
          String(body.cause),
        ) ||
        typeof body.reason !== "string"
      )
        return null;
      return Object.freeze({
        accountId: value.accountId,
        brandId: value.brandId,
        periodId: value.periodId,
        operation: "cancel",
        body: Object.freeze(body as PeriodCancellationBody),
        key: value.key,
      });
    }
    if (value.operation === "retry") {
      const body = value.body as
        | Partial<PeriodCancellationRetryBody>
        | undefined;
      if (
        !body ||
        !Number.isSafeInteger(body.version) ||
        Number(body.version) <= 0 ||
        typeof body.reason !== "string"
      )
        return null;
      return Object.freeze({
        accountId: value.accountId,
        brandId: value.brandId,
        periodId: value.periodId,
        operation: "retry",
        body: Object.freeze(body as PeriodCancellationRetryBody),
        key: value.key,
      });
    }
  } catch {
    return null;
  }
  return null;
}
const pendingWrite = ref<PendingWrite | null>(restorePendingWrite());
const reviewedWrite = ref<{
  operation: PendingWrite["operation"];
  body: PendingWrite["body"];
} | null>(null);
const periodId = ref(pendingWrite.value?.periodId ?? "");
const periodVersion = ref(
  pendingWrite.value?.operation === "cancel"
    ? String(pendingWrite.value.body.version)
    : "",
);

const permissionKey = computed(() =>
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
function scope(lane: string) {
  return JSON.stringify([
    permissionKey.value,
    lane,
    periodId.value,
    gameId.value,
  ]);
}
function current(ticket: ReturnType<typeof guard.capture>, permitted: boolean) {
  return guard.isCurrent(ticket, scope(ticket.lane), permitted);
}
function invalidateReads() {
  guard.invalidate("periods");
  guard.invalidate("summary");
  loadingPeriods.value = false;
  loadingSummary.value = false;
}
function clearSelection() {
  invalidateReads();
  periods.value = [];
  cancellation.value = null;
  summaryLoaded.value = false;
  confirmStage.value = "idle";
  reason.value = "";
  retryReason.value = "";
  notice.value = "";
  error.value = "";
}
function reportError(problem: unknown) {
  if (problem instanceof AdminApiError && problem.status === 401)
    emit("session-invalid");
  error.value =
    problem instanceof Error ? problem.message : localized("请求失败，请重试。", "Request failed. Please try again.");
}
function checkRecord(record: Cancellation) {
  if (record.brand_id !== props.brandId || record.period_id !== periodId.value)
    throw new Error(t("取消任务响应与当前品牌或期次不匹配，请重新读取。", "The cancellation response does not match the current brand or period. Reload and try again."));
  return record;
}
function modeLabel(value: CancelMode) {
  return value === "bet_cancelled" ? t("投注取消", "Betting cancelled") : t("判定取消", "Judgment cancelled");
}
function causeLabel(value: CancelCause) {
  return value === "operator_cancel"
    ? t("运营取消", "Operator cancellation")
    : value === "no_result"
      ? t("无开奖结果", "No draw result")
      : t("开奖结果无效", "Invalid draw result");
}
function periodStatusMessage(value: string): LocalizedMessage {
  const labels: Record<string, LocalizedMessage> = {
    pending: localized("待开始", "Pending"), betting: localized("投注中", "Betting open"),
    closed: localized("已截止", "Closed"), waiting_draw: localized("待开奖", "Awaiting draw"),
    drawn: localized("已开奖", "Drawn"), settling: localized("结算中", "Settling"),
    settled: localized("已结算", "Settled"), bet_cancelled: localized("投注取消", "Betting cancelled"),
    judged_cancelled: localized("判定取消", "Judgment cancelled"),
  };
  return labels[value] ?? localized(value, value);
}
function periodStatusLabel(value: string) {
  return t(periodStatusMessage(value));
}
function validUuid(value: string) {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
    value.trim(),
  );
}
function positiveVersion(value: string) {
  if (!/^[1-9][0-9]*$/.test(value)) return null;
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : null;
}
const formReady = computed(
  () =>
    validUuid(periodId.value) && positiveVersion(periodVersion.value) !== null,
);
const canRead = computed(
  () => rights.value.view && validUuid(periodId.value) && !loadingSummary.value,
);
const canCancel = computed(
  () =>
    rights.value.cancel &&
    formReady.value &&
    (!rights.value.view || (summaryLoaded.value && !cancellation.value)) &&
    !writing.value &&
    !pendingWrite.value,
);
const canRetry = computed(
  () =>
    rights.value.retry &&
    validUuid(periodId.value) &&
    (rights.value.view
      ? cancellation.value?.state === "failed"
      : positiveVersion(retryVersionInput.value) !== null) &&
    !writing.value &&
    !pendingWrite.value,
);
const selectedPeriod = computed(() =>
  directPeriod.value?.id === periodId.value
    ? directPeriod.value
    : (periods.value.find(
        (item) =>
          item.id === periodId.value && item.game_id === gameId.value.trim(),
      ) ?? null),
);
const reasonLength = computed(
  () => new TextEncoder().encode(reason.value).length,
);
const retryReasonLength = computed(
  () => new TextEncoder().encode(retryReason.value).length,
);
const canUseCancelForm = computed(() => {
  if (mode.value === "bet_cancelled")
    return (
      (!selectedPeriod.value || selectedPeriod.value.status === "betting") &&
      cause.value === "operator_cancel"
    );
  return cause.value === "no_result" || cause.value === "invalid_result";
});

let pollTimer: ReturnType<typeof setTimeout> | undefined;
let pollCount = 0;
function stopPolling() {
  if (pollTimer) clearTimeout(pollTimer);
  pollTimer = undefined;
  pollCount = 0;
}
function schedulePoll() {
  if (
    pollTimer ||
    pollCount >= 6 ||
    !rights.value.view ||
    cancellation.value?.state !== "processing"
  )
    return;
  pollTimer = setTimeout(() => void pollOnce(), 10_000);
}
async function pollOnce() {
  pollTimer = undefined;
  if (
    pollCount >= 6 ||
    !rights.value.view ||
    cancellation.value?.state !== "processing"
  )
    return;
  pollCount += 1;
  await loadSummary(true);
}

async function readPeriods() {
  if (!rights.value.view || !validUuid(gameId.value) || loadingPeriods.value)
    return;
  const brandId = props.brandId;
  const selectedGameId = gameId.value.trim();
  guard.invalidate("periods");
  const ticket = guard.capture("periods", scope("periods"));
  loadingPeriods.value = true;
  error.value = "";
  try {
    const result = await periodsApi.getPeriods(brandId, selectedGameId, 100, 0);
    if (!current(ticket, rights.value.view)) return;
    for (const item of result.periods) {
      if (item.brand_id !== brandId || item.game_id !== selectedGameId)
        throw new Error(t("期次列表包含其他品牌或彩种的数据。", "The period list contains records from another brand or game."));
    }
    periods.value = result.periods;
    if (!result.periods.some((item) => item.id === periodId.value)) {
      periodId.value = "";
      periodVersion.value = "";
    }
    notice.value = localized("已读取 {count} 个期次。", "Loaded {count} periods.", { count: result.periods.length });
  } catch (problem) {
    if (current(ticket, rights.value.view)) reportError(problem);
  } finally {
    if (current(ticket, rights.value.view)) loadingPeriods.value = false;
  }
}

async function readPeriod() {
  if (!rights.value.view || !validUuid(periodId.value) || pendingWrite.value)
    return;
  const id = periodId.value.trim().toLowerCase(),
    brandId = props.brandId;
  periodId.value = id;
  const ticket = guard.capture("period", scope("period"));
  loadingPeriod.value = true;
  error.value = "";
  directPeriod.value = null;
  try {
    const item = await api.getPeriod(brandId, id);
    if (!current(ticket, rights.value.view)) return;
    directPeriod.value = item;
    periodVersion.value = String(item.version);
    const status = periodStatusMessage(item.status);
    notice.value = localized(
      "已读取期次 {periodNo}, 状态 {statusZh}, 版本 {version}.",
      "Loaded period {periodNo}, status {statusEn}, version {version}.",
      { periodNo: item.period_no, statusZh: status.zh, statusEn: status.en, version: item.version },
    );
  } catch (problem) {
    if (current(ticket, rights.value.view)) reportError(problem);
  } finally {
    if (current(ticket, true)) loadingPeriod.value = false;
  }
}

async function loadSummary(quiet = false) {
  if (!rights.value.view || !validUuid(periodId.value) || loadingSummary.value)
    return;
  const brandId = props.brandId;
  const id = periodId.value.trim();
  guard.invalidate("summary");
  const ticket = guard.capture("summary", scope("summary"));
  loadingSummary.value = true;
  if (!quiet) error.value = "";
  try {
    const response = await api.getCancellation(brandId, id);
    if (!current(ticket, rights.value.view)) return;
    const result = response.cancellation;
    if (result && (result.brand_id !== brandId || result.period_id !== id))
      throw new Error(t("取消摘要与当前品牌或期次不匹配。", "The cancellation summary does not match the current brand or period."));
    cancellation.value = result;
    summaryLoaded.value = true;
    if (!quiet)
      notice.value = result ? localized("已读取取消任务摘要。", "Cancellation summary loaded.") : localized("该期次没有取消任务。", "This period has no cancellation task.");
    schedulePoll();
  } catch (problem) {
    if (current(ticket, rights.value.view)) {
      summaryLoaded.value = false;
      reportError(problem);
    }
  } finally {
    if (current(ticket, rights.value.view)) loadingSummary.value = false;
  }
}

function validateCancel(): PeriodCancellationBody {
  const version = positiveVersion(periodVersion.value);
  const cleanReason = reason.value.trim();
  if (!validUuid(periodId.value)) throw new Error(t("请输入有效的期次 UUID。", "Enter a valid period UUID."));
  if (version === null) throw new Error(t("期次 version 必须是正整数。", "Period version must be a positive integer."));
  if (!cleanReason || reasonLength.value > 500)
    throw new Error(t("原因不能为空，且 UTF-8 长度不能超过 500 字节。", "Reason is required and must not exceed 500 UTF-8 bytes."));
  if (!canUseCancelForm.value)
    throw new Error(t("当前期次状态与取消模式或原因不匹配，请核对期次状态。", "The period status does not match the selected cancellation mode or cause. Review the period status."));
  return { version, mode: mode.value, cause: cause.value, reason: cleanReason };
}
function reviewCancel() {
  try {
    const body = validateCancel();
    if (rights.value.view && !summaryLoaded.value)
      throw new Error(t("请先读取取消摘要。", "Read the cancellation summary first."));
    confirmStage.value = "review";
    reviewedWrite.value = {
      operation: "cancel",
      body: Object.freeze(structuredClone(body)),
    };
    error.value = "";
  } catch (problem) {
    reportError(problem);
  }
}
function reviewRetry() {
  if (!validUuid(periodId.value)) {
    error.value = localized("请输入有效的期次 UUID，以指定要重试的取消任务。", "Enter a valid period UUID to identify the cancellation task to retry.");
    return;
  }
  const cleanReason = retryReason.value.trim();
  if (!cleanReason || retryReasonLength.value > 500) {
    error.value = localized("重试原因不能为空，且 UTF-8 长度不能超过 500 字节。", "Retry reason is required and must not exceed 500 UTF-8 bytes.");
    return;
  }
  if (
    rights.value.view &&
    (!cancellation.value || cancellation.value.state !== "failed")
  )
    return;
  if (!rights.value.view && positiveVersion(retryVersionInput.value) === null) {
    error.value = localized("请输入有效的取消任务 version。", "Enter a valid cancellation task version.");
    return;
  }
  confirmStage.value = "retry-review";
  const version =
    cancellation.value?.version ?? positiveVersion(retryVersionInput.value);
  if (version === null) return;
  reviewedWrite.value = {
    operation: "retry",
    body: Object.freeze({ version, reason: cleanReason }),
  };
  error.value = "";
}

function beginWrite(
  operation: PendingWrite["operation"],
  body: PendingWrite["body"],
) {
  const id = periodId.value.trim();
  const brandId = props.brandId;
  pendingWrite.value = Object.freeze({
    accountId: props.account.id,
    brandId,
    periodId: id,
    operation,
    body: Object.freeze(structuredClone(body)),
    key: createIdempotencyKey(),
  });
  void submitPending();
}
async function submitPending() {
  const attempt = pendingWrite.value;
  if (
    !attempt ||
    writing.value ||
    attempt.accountId !== props.account.id ||
    attempt.brandId !== props.brandId ||
    attempt.periodId !== periodId.value.trim()
  )
    return;
  writing.value = true;
  error.value = "";
  notice.value = localized("正在提交取消任务…", "Submitting cancellation task…");
  const lane = `write-${attempt.operation}`;
  const ticket = guard.capture(lane, scope(lane));
  try {
    const result =
      attempt.operation === "cancel"
        ? await api.cancelPeriod(
            attempt.brandId,
            attempt.periodId,
            attempt.body as PeriodCancellationBody,
            attempt.key,
          )
        : await api.retryCancellation(
            attempt.brandId,
            attempt.periodId,
            attempt.body as PeriodCancellationRetryBody,
            attempt.key,
          );
    if (
      !current(
        ticket,
        attempt.operation === "cancel"
          ? rights.value.cancel
          : rights.value.retry,
      )
    )
      return;
    checkRecord(result);
    cancellation.value = result;
    summaryLoaded.value = true;
    pendingWrite.value = null;
    confirmStage.value = "idle";
    reason.value = "";
    retryReason.value = "";
    notice.value =
      result.state === "processing"
        ? localized("取消任务已受理。期次已停止；原来源退款将异步逐笔处理，完成前不会显示退款完成。", "Cancellation accepted. The period has stopped; refunds will be processed asynchronously against their original sources and will not show as complete until processing finishes.")
        : result.state === "completed"
          ? localized("取消任务已完成。", "Cancellation task completed.")
          : localized("取消任务失败，请核对错误码后人工重试。", "Cancellation task failed. Review the error code before retrying manually.");
    stopPolling();
    schedulePoll();
  } catch (problem) {
    if (
      !current(
        ticket,
        attempt.operation === "cancel"
          ? rights.value.cancel
          : rights.value.retry,
      )
    )
      return;
    if (problem instanceof AdminApiError && problem.status === 409) {
      pendingWrite.value = null;
      confirmStage.value = "idle";
      notice.value =
        attempt.operation === "cancel"
          ? localized("期次版本冲突。已读取最新取消摘要；请重新核对并手动填写最新期次 version，再次确认。", "Period version conflict. The latest cancellation summary has been loaded; review it, enter the latest period version, and confirm again.")
          : localized("取消任务版本冲突。已读取最新摘要，请核对当前任务 version 后重新确认重试。", "Cancellation task version conflict. The latest summary has been loaded; review the task version and confirm the retry again.");
      reportError(problem);
      await loadSummary(true);
    } else if (
      problem instanceof AdminApiError &&
      problem.status > 0 &&
      problem.status < 500
    ) {
      pendingWrite.value = null;
      confirmStage.value = "idle";
      reportError(problem);
    } else {
      // Network failures and 5xx leave the immutable body/key available for exact retry.
      reportError(problem);
      notice.value = localized("写入结果未知。原请求已冻结；只能用相同内容和幂等键重试，勿另建请求。", "Write outcome unknown. The original request is frozen; retry only with the same body and idempotency key. Do not create another request.");
    }
  } finally {
    if (current(ticket, true)) writing.value = false;
  }
}
function confirmWrite() {
  const intent = reviewedWrite.value;
  if (
    !intent ||
    writing.value ||
    pendingWrite.value ||
    confirmStage.value === "idle"
  )
    return;
  beginWrite(intent.operation, intent.body);
}

watch(
  () => [props.brandId, permissionKey.value] as const,
  () => {
    guard.invalidate();
    clearSelection();
    writing.value = false;
    stopPolling();
    pendingWrite.value = restorePendingWrite();
    periodId.value = pendingWrite.value?.periodId ?? "";
    periodVersion.value =
      pendingWrite.value?.operation === "cancel"
        ? String(pendingWrite.value.body.version)
        : "";
    gameId.value = "";
    if (
      pendingWrite.value &&
      (pendingWrite.value.brandId !== props.brandId ||
        !rights.value[
          pendingWrite.value.operation === "cancel" ? "cancel" : "retry"
        ])
    ) {
      // Keep the frozen attempt in memory; a later authorized matching scope can retry it.
      writing.value = false;
    }
  },
  { flush: "sync" },
);
watch(
  pendingWrite,
  (attempt) => {
    try {
      if (typeof sessionStorage === "undefined") return;
      if (attempt)
        sessionStorage.setItem(
          pendingStorageKey(attempt.accountId, attempt.brandId),
          JSON.stringify(attempt),
        );
      else sessionStorage.removeItem(pendingStorageKey());
    } catch {
      // The in-memory frozen request remains available if browser storage is unavailable.
    }
  },
  { flush: "sync" },
);
watch(
  () => [
    periodVersion.value,
    reason.value,
    retryReason.value,
    retryVersionInput.value,
  ],
  () => {
    confirmStage.value = "idle";
    reviewedWrite.value = null;
  },
  { flush: "sync" },
);
watch(
  confirmStage,
  (stage) => {
    if (stage === "idle") reviewedWrite.value = null;
  },
  { flush: "sync" },
);
watch(
  () => [periodId.value, gameId.value] as const,
  () => {
    guard.invalidate("periods");
    loadingPeriods.value = false;
    guard.invalidate("period");
    loadingPeriod.value = false;
    directPeriod.value = null;
    guard.invalidate("summary");
    cancellation.value = null;
    summaryLoaded.value = false;
    confirmStage.value = "idle";
    stopPolling();
  },
  { flush: "sync" },
);
watch(
  () => [rights.value.view, rights.value.cancel, rights.value.retry] as const,
  () => {
    if (!rights.value.view) {
      guard.invalidate();
      clearSelection();
      stopPolling();
    }
  },
  { flush: "sync" },
);
onBeforeUnmount(() => {
  alive = false;
  guard.invalidate();
  stopPolling();
});
</script>

<template>
  <section
    class="period-cancellation"
    aria-labelledby="period-cancel-title"
    data-testid="period-cancellation"
  >
    <header class="heading">
      <div>
        <p class="eyebrow">{{ t("期次与开奖 · 高风险操作", "Periods and draws · High impact action") }}</p>
        <h2 id="period-cancel-title">{{ t("期次取消与退款", "Period cancellation and refunds") }}</h2>
        <p>
          {{ t("取消会立即停止期次。原来源退款逐笔异步处理，任务完成后才代表退款完成。", "Cancellation stops the period immediately. Refunds are processed asynchronously against their original sources; refunds are complete only when the task completes.") }}
        </p>
      </div>
      <span class="brand-tag">{{ t("品牌", "Brand") }} {{ brandId || t("未选择", "Not selected") }}</span>
    </header>

    <p v-if="!rights.view" class="callout">
      {{ t("当前账号没有读取取消摘要的权限。读取权限与写入权限相互独立。", "This account cannot read cancellation summaries. Read and write permissions are independent.") }}
    </p>
    <div v-if="pendingWrite" class="panel pending">
      <b>{{ t("有一项写入结果待确认，原请求内容和幂等键已冻结。", "A write outcome is unresolved. The original request body and idempotency key are frozen.") }}</b>
      <p>
        {{ t("请切回原账号、品牌和期次后按原请求重试，以确认服务端结果。不要另建请求。", "Return to the original account, brand, and period, then retry the original request to resolve the server outcome. Do not create a new request.") }}
      </p>
      <button
        class="primary"
        type="button"
        :disabled="
          writing ||
          pendingWrite.accountId !== account.id ||
          pendingWrite.brandId !== brandId ||
          pendingWrite.periodId !== periodId ||
          !(pendingWrite.operation === 'cancel' ? rights.cancel : rights.retry)
        "
        @click="submitPending"
      >
        {{ writing ? t("正在按原请求重试…", "Retrying the original request…") : t("使用原请求重试", "Retry original request") }}
      </button>
    </div>
    <template v-if="rights.view">
      <div class="panel">
        <div class="section-head">
          <h3>{{ t("期次", "Period") }}</h3>
          <span class="muted">{{ t("支持直接输入 UUID 和期次 version", "You can enter a UUID and period version directly") }}</span>
        </div>
        <div class="field-grid">
          <label
            >{{ t("彩种 UUID（仅用于读取列表）", "Game UUID (used only to read the list)") }}
            <input
              v-model="gameId"
              inputmode="text"
              autocomplete="off"
              placeholder="game UUID"
              :disabled="Boolean(pendingWrite)"
            />
          </label>
          <button
            class="secondary"
            type="button"
            :disabled="
              !validUuid(gameId) || loadingPeriods || Boolean(pendingWrite)
            "
            @click="readPeriods"
          >
            {{ loadingPeriods ? t("读取中…", "Loading…") : t("读取期次列表", "Read period list") }}
          </button>
          <label v-if="periods.length" class="wide"
            >{{ t("选择期次", "Select period") }}
            <select
              :value="periodId"
              :disabled="Boolean(pendingWrite)"
              @change="
                (event) => {
                  const p = periods.find(
                    (item) =>
                      item.id === (event.target as HTMLSelectElement).value,
                  );
                  periodId = p?.id ?? '';
                  periodVersion = p ? String(p.version) : '';
                }
              "
            >
              <option value="">{{ t("选择读取到的期次", "Select a loaded period") }}</option>
              <option v-for="item in periods" :key="item.id" :value="item.id">
                {{ item.period_no }} · {{ periodStatusLabel(item.status) }} · v{{ item.version }}
              </option>
            </select>
          </label>
          <label
            >{{ t("期次 UUID", "Period UUID") }}
            <input
              v-model="periodId"
              autocomplete="off"
              spellcheck="false"
              placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
              :disabled="Boolean(pendingWrite)"
            />
          </label>
          <label
            >{{ t("期次 version", "Period version") }}
            <input
              v-model="periodVersion"
              inputmode="numeric"
              autocomplete="off"
              :placeholder="t('正整数', 'Positive integer')"
              :disabled="Boolean(pendingWrite)"
            />
          </label>
          <button
            class="secondary"
            type="button"
            :disabled="
              !validUuid(periodId) || loadingPeriod || Boolean(pendingWrite)
            "
            @click="readPeriod"
          >
            {{ loadingPeriod ? t("读取中…", "Loading…") : t("读取期次详情", "Read period details") }}
          </button>
          <button
            class="primary"
            type="button"
            :disabled="!canRead"
            @click="loadSummary()"
          >
            {{ loadingSummary ? t("读取中…", "Loading…") : t("读取取消摘要", "Read cancellation summary") }}
          </button>
        </div>
      </div>

      <p v-if="error" class="message error" role="alert">{{ t(error) }}</p>
      <p v-if="notice" class="message notice" role="status">{{ t(notice) }}</p>

      <div v-if="summaryLoaded" class="panel">
        <div class="section-head">
          <h3>{{ t("取消任务摘要", "Cancellation summary") }}</h3>
          <span
            v-if="cancellation"
            class="state"
            :class="`state-${cancellation.state}`"
            >{{
              cancellation.state === "processing"
                ? t("处理中", "Processing")
                : cancellation.state === "failed"
                  ? t("失败", "Failed")
                  : t("已完成", "Completed")
            }}</span
          ><span v-else class="state state-empty">{{ t("无取消任务", "No cancellation task") }}</span>
        </div>
          <p v-if="!cancellation" class="muted">{{ t("当前期次没有取消任务记录。", "There is no cancellation task for this period.") }}</p>
        <template v-else>
          <dl class="details">
            <div>
              <dt>{{ t("任务 ID", "Task ID") }}</dt>
              <dd>{{ cancellation.id }}</dd>
            </div>
            <div>
              <dt>{{ t("取消后期次 version", "Period version after cancellation") }}</dt>
              <dd>{{ cancellation.period_version }}</dd>
            </div>
            <div>
              <dt>{{ t("取消任务 version", "Cancellation task version") }}</dt>
              <dd>{{ cancellation.version }}</dd>
            </div>
            <div>
              <dt>{{ t("模式 / 原因", "Mode / cause") }}</dt>
              <dd>
                {{ modeLabel(cancellation.mode) }} /
                {{ causeLabel(cancellation.cause) }}
              </dd>
            </div>
            <div>
              <dt>{{ t("创建原因", "Request reason") }}</dt>
              <dd>{{ cancellation.reason }}</dd>
            </div>
            <div>
              <dt>{{ t("操作人", "Operator") }}</dt>
              <dd>{{ cancellation.created_by }}</dd>
            </div>
            <div>
              <dt>{{ t("创建时间", "Created at") }}</dt>
              <dd>{{ cancellation.created_at }}</dd>
            </div>
            <div>
              <dt>{{ t("完成时间", "Completed at") }}</dt>
              <dd>{{ cancellation.completed_at || "—" }}</dd>
            </div>
          </dl>
          <div class="counts" :aria-label="t('取消与退款数量', 'Cancellation and refund counts')">
            <div>
              <span>{{ t("总数", "Total") }}</span><b>{{ cancellation.total_count }}</b>
            </div>
            <div>
              <span>{{ t("待处理", "Pending") }}</span><b>{{ cancellation.pending_count }}</b>
            </div>
            <div>
              <span>{{ t("已退款", "Refunded") }}</span><b>{{ cancellation.refunded_count }}</b>
            </div>
            <div>
              <span>{{ t("原已退款", "Already refunded") }}</span
              ><b>{{ cancellation.already_refunded_count }}</b>
            </div>
            <div>
              <span>{{ t("失败", "Failed") }}</span><b>{{ cancellation.failed_count }}</b>
            </div>
          </div>
          <p v-if="cancellation.last_error_code" class="callout error-code">
            {{ t("最后错误码：", "Last error code: ") }}<code>{{ cancellation.last_error_code }}</code
            >{{ t("。请运营核对后人工重试。", ". Have an operator review it before retrying manually.") }}
          </p>
          <p v-if="cancellation.state === 'processing'" class="muted">
            {{ t("退款逐笔异步处理；处理中不代表退款已完成，页面每 10 秒读取一次，最多 6 次。", "Refunds are processed asynchronously, one by one. Processing does not mean refunds are complete. This page polls every 10 seconds, up to 6 times.") }}
          </p>
          <p v-if="cancellation.state === 'completed'" class="muted">
            {{ t("任务已完成，服务端已确认本任务退款处理完成。", "The task is complete; the server has confirmed refund processing for this task.") }}
          </p>
        </template>
      </div>

      <div v-if="rights.cancel && summaryLoaded && !cancellation" class="panel">
        <div class="section-head"><h3>{{ t("发起期次取消", "Start period cancellation") }}</h3></div>
        <div class="field-grid">
          <label
            >{{ t("取消模式", "Cancellation mode") }}
            <select
              v-model="mode"
              :aria-label="t('取消模式', 'Cancellation mode')"
              :disabled="Boolean(pendingWrite)"
              @change="
                cause =
                  mode === 'bet_cancelled' ? 'operator_cancel' : 'no_result';
                confirmStage = 'idle';
              "
            >
              <option value="bet_cancelled">{{ t("投注取消", "Betting cancelled") }}</option>
              <option value="judged_cancelled">
                {{ t("未结算期次取消（保留开奖历史，不撤销结算）", "Cancel unsettled period (preserve draw history; do not reverse settlement)") }}
              </option>
            </select>
          </label>
          <label
            >{{ t("取消原因", "Cancellation cause") }}
            <select
              v-model="cause"
              :aria-label="t('取消原因', 'Cancellation cause')"
              :disabled="Boolean(pendingWrite)"
              @change="confirmStage = 'idle'"
            >
              <option v-if="mode === 'bet_cancelled'" value="operator_cancel">
                {{ t("运营取消", "Operator cancellation") }}
              </option>
              <template v-else>
                <option value="no_result">{{ t("无开奖结果", "No draw result") }}</option>
                <option value="invalid_result">{{ t("开奖结果无效", "Invalid draw result") }}</option>
              </template>
            </select>
          </label>
          <p v-if="mode === 'bet_cancelled'" class="hint wide">
            {{ t("投注取消仅适用于状态为 betting 的期次，且原因必须为运营取消。", "Betting cancellation applies only to periods in betting status and requires the operator_cancel cause.") }}
          </p>
          <p v-else class="hint wide">
            {{ t("仅用于未结算期次。已有开奖结果历史会保留，取消不会撤销结算。", "For unsettled periods only. Existing draw history is preserved, and cancellation does not reverse settlement.") }}
          </p>
          <label class="wide"
            >{{ t("操作原因（UTF-8 不超过 500 字节）", "Reason (up to 500 UTF-8 bytes)") }}
            <textarea
              :aria-label="t('操作原因（UTF-8 不超过 500 字节）', 'Reason (up to 500 UTF-8 bytes)')"
              v-model="reason"
              rows="3"
              maxlength="500"
              :disabled="Boolean(pendingWrite)"
              @input="confirmStage = 'idle'"
            />
            <small>{{ reasonLength }} / {{ t("500 字节", "500 bytes") }}</small>
          </label>
        </div>
        <div class="actions">
          <button
            v-if="confirmStage !== 'review'"
            class="secondary"
            type="button"
            :disabled="!canCancel || !canUseCancelForm || Boolean(pendingWrite)"
            @click="reviewCancel"
          >
            {{ t("检查并确认影响", "Review and confirm impact") }}
          </button>
          <template v-else>
            <p class="confirm-note">
              {{ t("即将停止期次并建立异步退款任务。请核对请求使用的期次 version、模式、原因和操作说明。取消后期次 version 将递增，任务 version 独立。", "This will stop the period and create an asynchronous refund task. Review the period version, mode, cause, and reason. Cancellation increments the period version; the task has its own version.") }}
            </p>
            <button
              class="primary danger"
              type="button"
              :disabled="!canCancel || !canUseCancelForm"
              @click="confirmWrite"
            >
              {{ writing ? t("提交中…", "Submitting…") : t("确认取消期次", "Confirm period cancellation") }}
            </button>
          </template>
        </div>
      </div>

      <div
        v-if="rights.retry && cancellation?.state === 'failed'"
        class="panel"
      >
        <div class="section-head"><h3>{{ t("人工重试失败任务", "Retry failed task manually") }}</h3></div>
        <p class="hint">
          {{ t("重试使用当前取消任务 version（", "Retry uses the current cancellation task version (") }}{{
            cancellation.version
          }}{{ t("）；这与发起取消所需的期次 version 不同。", "); this differs from the period version required to start cancellation.") }}
        </p>
        <label
          >{{ t("重试原因（UTF-8 不超过 500 字节）", "Retry reason (up to 500 UTF-8 bytes)") }}
          <textarea
            v-model="retryReason"
            rows="2"
            maxlength="500"
            :disabled="Boolean(pendingWrite)"
            @input="confirmStage = 'idle'"
          />
          <small>{{ retryReasonLength }} / {{ t("500 字节", "500 bytes") }}</small>
        </label>
        <div class="actions">
          <button
            v-if="confirmStage !== 'retry-review'"
            class="secondary"
            type="button"
            :disabled="!canRetry || Boolean(pendingWrite)"
            @click="reviewRetry"
          >
            {{ t("检查并确认重试", "Review and confirm retry") }}
          </button>
          <template v-else>
            <p class="confirm-note">
              {{ t("将以取消任务 version", "Refund processing will retry with cancellation task version") }} {{ cancellation.version }}。
            </p>
            <button
              class="primary"
              type="button"
              :disabled="!canRetry"
              @click="confirmWrite"
            >
              {{ writing ? t("提交中…", "Submitting…") : t("确认重试任务", "Confirm task retry") }}
            </button>
          </template>
        </div>
      </div>
    </template>

    <template v-if="!rights.view && (rights.cancel || rights.retry)">
      <div class="panel">
        <div class="section-head"><h3>{{ t("手动指定期次", "Specify period manually") }}</h3></div>
        <p class="hint">
          {{ t("当前账号没有取消摘要读取权。本页不会调用 GET；请核对期次及版本后再提交。", "This account cannot read cancellation summaries. This page will not make a GET request; verify the period and version before submitting.") }}
        </p>
        <div class="field-grid">
          <label
            >{{ t("期次 UUID", "Period UUID") }}<input
              v-model="periodId"
              autocomplete="off"
              spellcheck="false"
              :disabled="Boolean(pendingWrite)"
          /></label>
          <label
            >{{ t("期次 version", "Period version") }}<input
              v-model="periodVersion"
              inputmode="numeric"
              autocomplete="off"
              :disabled="Boolean(pendingWrite)"
          /></label>
        </div>
      </div>
      <p v-if="error" class="message error" role="alert">{{ t(error) }}</p>
      <p v-if="notice" class="message notice" role="status">{{ t(notice) }}</p>
      <div v-if="rights.cancel" class="panel">
        <div class="section-head"><h3>{{ t("发起期次取消", "Start period cancellation") }}</h3></div>
        <p class="hint">
          {{ t("投注取消仅适用于 betting 状态并使用“运营取消”；未结算期次取消会保留开奖结果历史，且不撤销结算。", "Betting cancellation applies only in betting status and uses the operator_cancel cause. Unsettled period cancellation preserves draw history and does not reverse settlement.") }}
        </p>
        <div class="field-grid">
          <label
            >{{ t("取消模式", "Cancellation mode") }}<select
              v-model="mode"
              :disabled="Boolean(pendingWrite)"
              @change="
                cause =
                  mode === 'bet_cancelled' ? 'operator_cancel' : 'no_result';
                confirmStage = 'idle';
              "
            >
              <option value="bet_cancelled">{{ t("投注取消", "Betting cancelled") }}</option>
              <option value="judged_cancelled">{{ t("未结算期次取消", "Unsettled period cancellation") }}</option>
            </select></label
          >
          <label
            >{{ t("取消原因", "Cancellation cause") }}<select
              v-model="cause"
              :disabled="Boolean(pendingWrite)"
              @change="confirmStage = 'idle'"
            >
              <option v-if="mode === 'bet_cancelled'" value="operator_cancel">
                {{ t("运营取消", "Operator cancellation") }}
              </option>
              <template v-else
                ><option value="no_result">{{ t("无开奖结果", "No draw result") }}</option>
                <option value="invalid_result">{{ t("开奖结果无效", "Invalid draw result") }}</option></template
              >
            </select></label
          >
          <label class="wide"
            >{{ t("操作原因（UTF-8 不超过 500 字节）", "Reason (up to 500 UTF-8 bytes)") }}<textarea
              :aria-label="t('操作原因（UTF-8 不超过 500 字节）', 'Reason (up to 500 UTF-8 bytes)')"
              v-model="reason"
              rows="3"
              maxlength="500"
              :disabled="Boolean(pendingWrite)"
              @input="confirmStage = 'idle'"
            /><small>{{ reasonLength }} / {{ t("500 字节", "500 bytes") }}</small></label
          >
        </div>
        <div class="actions">
          <button
            v-if="confirmStage !== 'review'"
            class="secondary"
            type="button"
            :disabled="!canCancel || !canUseCancelForm"
            @click="reviewCancel"
          >
            {{ t("检查并确认影响", "Review and confirm impact") }}
          </button>
          <template v-else
            ><p class="confirm-note">
              {{ t("请核对期次 UUID、期次 version、模式和原因。提交将立即停止期次并启动逐笔退款任务。", "Review the period UUID, period version, mode, and reason. Submission immediately stops the period and starts the per-order refund task.") }}
            </p>
            <button
              class="primary danger"
              type="button"
              :disabled="!canCancel || !canUseCancelForm"
              @click="confirmWrite"
            >
              {{ t("确认取消期次", "Confirm period cancellation") }}
            </button></template
          >
        </div>
      </div>
      <div v-if="rights.retry" class="panel">
        <div class="section-head"><h3>{{ t("人工重试失败任务", "Retry failed task manually") }}</h3></div>
        <p class="hint">
          {{ t("请先确认任务处于失败状态，并从摘要取得取消任务 version；它不同于期次 version。", "Confirm the task is failed and obtain the cancellation task version from its summary. This is separate from the period version.") }}
        </p>
        <div class="field-grid">
          <label
            >{{ t("取消任务 version", "Cancellation task version") }}<input
              v-model="retryVersionInput"
              inputmode="numeric"
              autocomplete="off"
              :disabled="Boolean(pendingWrite)"
          /></label>
          <label class="wide"
            >{{ t("重试原因（UTF-8 不超过 500 字节）", "Retry reason (up to 500 UTF-8 bytes)") }}<textarea
              v-model="retryReason"
              rows="2"
              maxlength="500"
              :disabled="Boolean(pendingWrite)"
              @input="confirmStage = 'idle'"
            /><small>{{ retryReasonLength }} / {{ t("500 字节", "500 bytes") }}</small></label
          >
        </div>
        <div class="actions">
          <button
            v-if="confirmStage !== 'retry-review'"
            class="secondary"
            type="button"
            :disabled="!canRetry"
            @click="reviewRetry"
          >
            {{ t("检查并确认重试", "Review and confirm retry") }}
          </button>
          <template v-else
            ><p class="confirm-note">
              {{ t("将以取消任务 version", "Retry with cancellation task version") }} {{ retryVersionInput }}{{ t("重试，请确认该任务当前为失败状态。", "; confirm that the task is currently failed.") }}
            </p>
            <button
              class="primary"
              type="button"
              :disabled="!canRetry"
              @click="confirmWrite"
            >
              {{ t("确认重试任务", "Confirm task retry") }}
            </button></template
          >
        </div>
      </div>
    </template>
  </section>
</template>

<style scoped>
.period-cancellation {
  display: grid;
  gap: 14px;
  min-width: 0;
  color: #252a36;
}
.heading,
.section-head,
.actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-width: 0;
}
.heading {
  align-items: flex-start;
}
.heading h2,
.section-head h3 {
  margin: 3px 0;
}
.heading p:last-child,
.muted,
.hint {
  color: #737b8c;
  font-size: 12px;
  line-height: 1.6;
  margin: 5px 0 0;
  overflow-wrap: anywhere;
}
.eyebrow {
  color: #5969df;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 1px;
  margin: 0;
}
.brand-tag,
.state {
  border: 1px solid #e3e6ee;
  border-radius: 14px;
  padding: 5px 9px;
  color: #596274;
  font-size: 11px;
  white-space: nowrap;
}
.panel {
  min-width: 0;
  padding: 16px;
  border: 1px solid #e9ebf0;
  border-radius: 9px;
  background: #fff;
  box-shadow: 0 2px 8px #1e2a8008;
}
.field-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 12px;
  align-items: end;
}
label {
  display: grid;
  gap: 6px;
  min-width: 0;
  color: #555e6e;
  font-size: 12px;
}
input,
select,
textarea {
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  border: 1px solid #dfe2e9;
  border-radius: 6px;
  padding: 9px;
  background: #fff;
  color: #252a36;
  font: inherit;
}
textarea {
  resize: vertical;
}
.wide {
  grid-column: 1 / -1;
}
button {
  border: 0;
  border-radius: 7px;
  padding: 10px 13px;
  font: inherit;
  font-size: 12px;
  font-weight: 650;
  cursor: pointer;
}
.primary {
  color: #fff;
  background: #405bd6;
}
.secondary {
  color: #455064;
  background: #eef1f7;
}
.danger {
  background: #b42332;
}
button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.message,
.callout {
  min-width: 0;
  padding: 10px 12px;
  border: 1px solid #e4e7ee;
  border-radius: 7px;
  background: #fff;
  font-size: 12px;
  line-height: 1.6;
  overflow-wrap: anywhere;
}
.error,
.error-code {
  color: #a5222f;
  border-color: #f0c9ce;
  background: #fff8f8;
}
.notice {
  color: #285c48;
  border-color: #cce5d8;
  background: #f5fbf7;
}
.details {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  margin: 12px 0;
}
.details div {
  min-width: 0;
  padding: 9px;
  border-radius: 6px;
  background: #f7f8fb;
}
.details dt {
  color: #737b8c;
  font-size: 10px;
}
.details dd {
  margin: 4px 0 0;
  font-size: 12px;
  overflow-wrap: anywhere;
}
.counts {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 8px;
}
.counts div {
  display: grid;
  gap: 5px;
  min-width: 0;
  padding: 10px 8px;
  border: 1px solid #e9ebf0;
  border-radius: 7px;
  text-align: center;
}
.counts span {
  color: #737b8c;
  font-size: 10px;
  overflow-wrap: anywhere;
}
.counts b {
  font-size: 18px;
  font-variant-numeric: tabular-nums;
}
.state-processing {
  color: #805900;
  background: #fff8e5;
}
.state-failed {
  color: #a5222f;
  background: #fff1f1;
}
.state-completed {
  color: #23734d;
  background: #eef9f2;
}
.state-empty {
  color: #596274;
  background: #f4f5f8;
}
.actions {
  align-items: flex-start;
  margin-top: 14px;
  flex-wrap: wrap;
}
.confirm-note {
  flex: 1 1 240px;
  margin: 0;
  color: #805900;
  font-size: 12px;
  line-height: 1.6;
}
.pending {
  border-color: #e6d29a;
  background: #fffcf2;
}
.pending p {
  margin: 5px 0 12px;
  font-size: 12px;
  line-height: 1.6;
}
small {
  color: #737b8c;
  text-align: right;
}
@media (max-width: 520px) {
  .heading,
  .section-head {
    align-items: flex-start;
    flex-direction: column;
  }
  .field-grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .wide {
    grid-column: auto;
  }
  .details {
    grid-template-columns: minmax(0, 1fr);
  }
  .counts {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .counts div:last-child {
    grid-column: 1 / -1;
  }
  .actions {
    align-items: stretch;
    flex-direction: column;
  }
  .actions button {
    width: 100%;
  }
  .brand-tag {
    white-space: normal;
    overflow-wrap: anywhere;
  }
}
</style>
