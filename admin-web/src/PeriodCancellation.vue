<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
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
const error = ref("");
const notice = ref("");
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
    problem instanceof Error ? problem.message : "请求失败，请重试。";
}
function checkRecord(record: Cancellation) {
  if (record.brand_id !== props.brandId || record.period_id !== periodId.value)
    throw new Error("取消任务响应与当前品牌或期次不匹配，请重新读取。");
  return record;
}
function modeLabel(value: CancelMode) {
  return value === "bet_cancelled" ? "投注取消" : "判定取消";
}
function causeLabel(value: CancelCause) {
  return value === "operator_cancel"
    ? "运营取消"
    : value === "no_result"
      ? "无开奖结果"
      : "开奖结果无效";
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
        throw new Error("期次列表包含其他品牌或彩种的数据。");
    }
    periods.value = result.periods;
    if (!result.periods.some((item) => item.id === periodId.value)) {
      periodId.value = "";
      periodVersion.value = "";
    }
    notice.value = `已读取 ${result.periods.length} 个期次。`;
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
    notice.value = `已读取期次 ${item.period_no}，状态 ${item.status}，版本 ${item.version}。`;
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
      throw new Error("取消摘要与当前品牌或期次不匹配。");
    cancellation.value = result;
    summaryLoaded.value = true;
    if (!quiet)
      notice.value = result ? "已读取取消任务摘要。" : "该期次没有取消任务。";
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
  if (!validUuid(periodId.value)) throw new Error("请输入有效的期次 UUID。");
  if (version === null) throw new Error("期次 version 必须是正整数。");
  if (!cleanReason || reasonLength.value > 500)
    throw new Error("原因不能为空，且 UTF-8 长度不能超过 500 字节。");
  if (!canUseCancelForm.value)
    throw new Error("当前期次状态与取消模式或原因不匹配，请核对期次状态。");
  return { version, mode: mode.value, cause: cause.value, reason: cleanReason };
}
function reviewCancel() {
  try {
    const body = validateCancel();
    if (rights.value.view && !summaryLoaded.value)
      throw new Error("请先读取取消摘要。");
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
    error.value = "请输入有效的期次 UUID，以指定要重试的取消任务。";
    return;
  }
  const cleanReason = retryReason.value.trim();
  if (!cleanReason || retryReasonLength.value > 500) {
    error.value = "重试原因不能为空，且 UTF-8 长度不能超过 500 字节。";
    return;
  }
  if (
    rights.value.view &&
    (!cancellation.value || cancellation.value.state !== "failed")
  )
    return;
  if (!rights.value.view && positiveVersion(retryVersionInput.value) === null) {
    error.value = "请输入有效的取消任务 version。";
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
  notice.value = "正在提交取消任务…";
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
        ? "取消任务已受理。期次已停止；原来源退款将异步逐笔处理，完成前不会显示退款完成。"
        : result.state === "completed"
          ? "取消任务已完成。"
          : "取消任务失败，请核对错误码后人工重试。";
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
          ? "期次版本冲突。已读取最新取消摘要；请重新核对并手动填写最新期次 version，再次确认。"
          : "取消任务版本冲突。已读取最新摘要，请核对当前任务 version 后重新确认重试。";
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
      notice.value =
        "写入结果未知。原请求已冻结；只能用相同内容和幂等键重试，勿另建请求。";
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
        <p class="eyebrow">期次与开奖 · 高风险操作</p>
        <h2 id="period-cancel-title">期次取消与退款</h2>
        <p>
          取消会立即停止期次。原来源退款逐笔异步处理，任务完成后才代表退款完成。
        </p>
      </div>
      <span class="brand-tag">品牌 {{ brandId || "未选择" }}</span>
    </header>

    <p v-if="!rights.view" class="callout">
      当前账号没有读取取消摘要的权限。读取权限与写入权限相互独立。
    </p>
    <div v-if="pendingWrite" class="panel pending">
      <b>有一项写入结果待确认，原请求内容和幂等键已冻结。</b>
      <p>
        请切回原账号、品牌和期次后按原请求重试，以确认服务端结果。不要另建请求。
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
        {{ writing ? "正在按原请求重试…" : "使用原请求重试" }}
      </button>
    </div>
    <template v-if="rights.view">
      <div class="panel">
        <div class="section-head">
          <h3>期次</h3>
          <span class="muted">支持直接输入 UUID 和期次 version</span>
        </div>
        <div class="field-grid">
          <label
            >彩种 UUID（仅用于读取列表）
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
            {{ loadingPeriods ? "读取中…" : "读取期次列表" }}
          </button>
          <label v-if="periods.length" class="wide"
            >选择期次
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
              <option value="">选择读取到的期次</option>
              <option v-for="item in periods" :key="item.id" :value="item.id">
                {{ item.period_no }} · {{ item.status }} · v{{ item.version }}
              </option>
            </select>
          </label>
          <label
            >期次 UUID
            <input
              v-model="periodId"
              autocomplete="off"
              spellcheck="false"
              placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
              :disabled="Boolean(pendingWrite)"
            />
          </label>
          <label
            >期次 version
            <input
              v-model="periodVersion"
              inputmode="numeric"
              autocomplete="off"
              placeholder="正整数"
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
            {{ loadingPeriod ? "读取中…" : "读取期次详情" }}
          </button>
          <button
            class="primary"
            type="button"
            :disabled="!canRead"
            @click="loadSummary()"
          >
            {{ loadingSummary ? "读取中…" : "读取取消摘要" }}
          </button>
        </div>
      </div>

      <p v-if="error" class="message error" role="alert">{{ error }}</p>
      <p v-if="notice" class="message notice" role="status">{{ notice }}</p>

      <div v-if="summaryLoaded" class="panel">
        <div class="section-head">
          <h3>取消任务摘要</h3>
          <span
            v-if="cancellation"
            class="state"
            :class="`state-${cancellation.state}`"
            >{{
              cancellation.state === "processing"
                ? "处理中"
                : cancellation.state === "failed"
                  ? "失败"
                  : "已完成"
            }}</span
          ><span v-else class="state state-empty">无取消任务</span>
        </div>
        <p v-if="!cancellation" class="muted">当前期次没有取消任务记录。</p>
        <template v-else>
          <dl class="details">
            <div>
              <dt>任务 ID</dt>
              <dd>{{ cancellation.id }}</dd>
            </div>
            <div>
              <dt>取消后期次 version</dt>
              <dd>{{ cancellation.period_version }}</dd>
            </div>
            <div>
              <dt>取消任务 version</dt>
              <dd>{{ cancellation.version }}</dd>
            </div>
            <div>
              <dt>模式 / 原因</dt>
              <dd>
                {{ modeLabel(cancellation.mode) }} /
                {{ causeLabel(cancellation.cause) }}
              </dd>
            </div>
            <div>
              <dt>创建原因</dt>
              <dd>{{ cancellation.reason }}</dd>
            </div>
            <div>
              <dt>操作人</dt>
              <dd>{{ cancellation.created_by }}</dd>
            </div>
            <div>
              <dt>创建时间</dt>
              <dd>{{ cancellation.created_at }}</dd>
            </div>
            <div>
              <dt>完成时间</dt>
              <dd>{{ cancellation.completed_at || "—" }}</dd>
            </div>
          </dl>
          <div class="counts" aria-label="取消与退款数量">
            <div>
              <span>总数</span><b>{{ cancellation.total_count }}</b>
            </div>
            <div>
              <span>待处理</span><b>{{ cancellation.pending_count }}</b>
            </div>
            <div>
              <span>已退款</span><b>{{ cancellation.refunded_count }}</b>
            </div>
            <div>
              <span>原已退款</span
              ><b>{{ cancellation.already_refunded_count }}</b>
            </div>
            <div>
              <span>失败</span><b>{{ cancellation.failed_count }}</b>
            </div>
          </div>
          <p v-if="cancellation.last_error_code" class="callout error-code">
            最后错误码：<code>{{ cancellation.last_error_code }}</code
            >。请运营核对后人工重试。
          </p>
          <p v-if="cancellation.state === 'processing'" class="muted">
            退款逐笔异步处理；处理中不代表退款已完成，页面每 10 秒读取一次，最多
            6 次。
          </p>
          <p v-if="cancellation.state === 'completed'" class="muted">
            任务已完成，服务端已确认本任务退款处理完成。
          </p>
        </template>
      </div>

      <div v-if="rights.cancel && summaryLoaded && !cancellation" class="panel">
        <div class="section-head"><h3>发起期次取消</h3></div>
        <div class="field-grid">
          <label
            >取消模式
            <select
              v-model="mode"
              aria-label="取消模式"
              :disabled="Boolean(pendingWrite)"
              @change="
                cause =
                  mode === 'bet_cancelled' ? 'operator_cancel' : 'no_result';
                confirmStage = 'idle';
              "
            >
              <option value="bet_cancelled">投注取消</option>
              <option value="judged_cancelled">
                未结算期次取消（保留开奖历史，不撤销结算）
              </option>
            </select>
          </label>
          <label
            >取消原因
            <select
              v-model="cause"
              aria-label="取消原因"
              :disabled="Boolean(pendingWrite)"
              @change="confirmStage = 'idle'"
            >
              <option v-if="mode === 'bet_cancelled'" value="operator_cancel">
                运营取消
              </option>
              <template v-else>
                <option value="no_result">无开奖结果</option>
                <option value="invalid_result">开奖结果无效</option>
              </template>
            </select>
          </label>
          <p v-if="mode === 'bet_cancelled'" class="hint wide">
            投注取消仅适用于状态为 betting 的期次，且原因必须为运营取消。
          </p>
          <p v-else class="hint wide">
            仅用于未结算期次。已有开奖结果历史会保留，取消不会撤销结算。
          </p>
          <label class="wide"
            >操作原因（UTF-8 不超过 500 字节）
            <textarea
              aria-label="操作原因（UTF-8 不超过 500 字节）"
              v-model="reason"
              rows="3"
              maxlength="500"
              :disabled="Boolean(pendingWrite)"
              @input="confirmStage = 'idle'"
            />
            <small>{{ reasonLength }} / 500 字节</small>
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
            检查并确认影响
          </button>
          <template v-else>
            <p class="confirm-note">
              即将停止期次并建立异步退款任务。请核对请求使用的期次
              version、模式、原因和操作说明。取消后期次 version 将递增，任务
              version 独立。
            </p>
            <button
              class="primary danger"
              type="button"
              :disabled="!canCancel || !canUseCancelForm"
              @click="confirmWrite"
            >
              {{ writing ? "提交中…" : "确认取消期次" }}
            </button>
          </template>
        </div>
      </div>

      <div
        v-if="rights.retry && cancellation?.state === 'failed'"
        class="panel"
      >
        <div class="section-head"><h3>人工重试失败任务</h3></div>
        <p class="hint">
          重试使用当前取消任务 version（{{
            cancellation.version
          }}）；这与发起取消所需的期次 version 不同。
        </p>
        <label
          >重试原因（UTF-8 不超过 500 字节）
          <textarea
            v-model="retryReason"
            rows="2"
            maxlength="500"
            :disabled="Boolean(pendingWrite)"
            @input="confirmStage = 'idle'"
          />
          <small>{{ retryReasonLength }} / 500 字节</small>
        </label>
        <div class="actions">
          <button
            v-if="confirmStage !== 'retry-review'"
            class="secondary"
            type="button"
            :disabled="!canRetry || Boolean(pendingWrite)"
            @click="reviewRetry"
          >
            检查并确认重试
          </button>
          <template v-else>
            <p class="confirm-note">
              将以取消任务 version {{ cancellation.version }} 重试退款处理。
            </p>
            <button
              class="primary"
              type="button"
              :disabled="!canRetry"
              @click="confirmWrite"
            >
              {{ writing ? "提交中…" : "确认重试任务" }}
            </button>
          </template>
        </div>
      </div>
    </template>

    <template v-if="!rights.view && (rights.cancel || rights.retry)">
      <div class="panel">
        <div class="section-head"><h3>手动指定期次</h3></div>
        <p class="hint">
          当前账号没有取消摘要读取权。本页不会调用
          GET；请核对期次及版本后再提交。
        </p>
        <div class="field-grid">
          <label
            >期次 UUID<input
              v-model="periodId"
              autocomplete="off"
              spellcheck="false"
              :disabled="Boolean(pendingWrite)"
          /></label>
          <label
            >期次 version<input
              v-model="periodVersion"
              inputmode="numeric"
              autocomplete="off"
              :disabled="Boolean(pendingWrite)"
          /></label>
        </div>
      </div>
      <p v-if="error" class="message error" role="alert">{{ error }}</p>
      <p v-if="notice" class="message notice" role="status">{{ notice }}</p>
      <div v-if="rights.cancel" class="panel">
        <div class="section-head"><h3>发起期次取消</h3></div>
        <p class="hint">
          投注取消仅适用于 betting
          状态并使用“运营取消”；未结算期次取消会保留开奖结果历史，且不撤销结算。
        </p>
        <div class="field-grid">
          <label
            >取消模式<select
              v-model="mode"
              :disabled="Boolean(pendingWrite)"
              @change="
                cause =
                  mode === 'bet_cancelled' ? 'operator_cancel' : 'no_result';
                confirmStage = 'idle';
              "
            >
              <option value="bet_cancelled">投注取消</option>
              <option value="judged_cancelled">未结算期次取消</option>
            </select></label
          >
          <label
            >取消原因<select
              v-model="cause"
              :disabled="Boolean(pendingWrite)"
              @change="confirmStage = 'idle'"
            >
              <option v-if="mode === 'bet_cancelled'" value="operator_cancel">
                运营取消
              </option>
              <template v-else
                ><option value="no_result">无开奖结果</option>
                <option value="invalid_result">开奖结果无效</option></template
              >
            </select></label
          >
          <label class="wide"
            >操作原因（UTF-8 不超过 500 字节）<textarea
              aria-label="操作原因（UTF-8 不超过 500 字节）"
              v-model="reason"
              rows="3"
              maxlength="500"
              :disabled="Boolean(pendingWrite)"
              @input="confirmStage = 'idle'"
            /><small>{{ reasonLength }} / 500 字节</small></label
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
            检查并确认影响
          </button>
          <template v-else
            ><p class="confirm-note">
              请核对期次 UUID、期次
              version、模式和原因。提交将立即停止期次并启动逐笔退款任务。
            </p>
            <button
              class="primary danger"
              type="button"
              :disabled="!canCancel || !canUseCancelForm"
              @click="confirmWrite"
            >
              确认取消期次
            </button></template
          >
        </div>
      </div>
      <div v-if="rights.retry" class="panel">
        <div class="section-head"><h3>人工重试失败任务</h3></div>
        <p class="hint">
          请先确认任务处于失败状态，并从摘要取得取消任务 version；它不同于期次
          version。
        </p>
        <div class="field-grid">
          <label
            >取消任务 version<input
              v-model="retryVersionInput"
              inputmode="numeric"
              autocomplete="off"
              :disabled="Boolean(pendingWrite)"
          /></label>
          <label class="wide"
            >重试原因（UTF-8 不超过 500 字节）<textarea
              v-model="retryReason"
              rows="2"
              maxlength="500"
              :disabled="Boolean(pendingWrite)"
              @input="confirmStage = 'idle'"
            /><small>{{ retryReasonLength }} / 500 字节</small></label
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
            检查并确认重试
          </button>
          <template v-else
            ><p class="confirm-note">
              将以取消任务 version
              {{ retryVersionInput }} 重试，请确认该任务当前为失败状态。
            </p>
            <button
              class="primary"
              type="button"
              :disabled="!canRetry"
              @click="confirmWrite"
            >
              确认重试任务
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
