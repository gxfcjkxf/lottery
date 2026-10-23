<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import FilterAssociationSelect from "@lottery/shared/FilterAssociationSelect.vue";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
const displayDate = (value: string | null | undefined) => formatDisplayDateTime(value, displayLocale.value);
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
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

const props = defineProps<{ account: AdminAccount; brandId: string; initialPeriodId?: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, message: localized, display } = useAdminI18n();

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
let summaryPolling: object | null = null;
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
const periodId = ref(pendingWrite.value?.periodId ?? props.initialPeriodId ?? "");
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
  summaryPolling = null;
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
function reportError(problem: unknown, operation: "read" | "write" = "write") {
  if (problem instanceof AdminApiError && problem.status === 401)
    emit("session-invalid");
  error.value =
    problem instanceof Error ? publicErrorMessage(problem, operation) : localized('admin.copy.4fb9cc4e');
}
function checkRecord(record: Cancellation) {
  if (record.brand_id !== props.brandId || record.period_id !== periodId.value)
    throw new Error(t('admin.copy.3d0cf675'));
  return record;
}
function modeLabel(value: CancelMode) {
  return value === "bet_cancelled" ? t('admin.copy.a1834cc2') : t('admin.copy.46a60c2c');
}
function causeLabel(value: CancelCause) {
  return value === "operator_cancel"
    ? t('admin.copy.644b3753')
    : value === "no_result"
      ? t('admin.copy.ced2d172')
      : t('admin.copy.9aec2fe0');
}
function periodStatusMessage(value: string): LocalizedMessage {
  const labels: Record<string, LocalizedMessage> = {
    pending: localized('admin.copy.997b3d22'), betting: localized('admin.copy.3a20b720'),
    closed: localized('admin.copy.3198ad9b'), waiting_draw: localized('admin.copy.aab1777a'),
    drawn: localized('admin.copy.a16e8013'), settling: localized('admin.copy.01f90196'),
    settled: localized('admin.copy.037164c2'), bet_cancelled: localized('admin.copy.a1834cc2'),
    judged_cancelled: localized('admin.copy.46a60c2c'),
  };
  return labels[value] ?? display(value);
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
    rights.value.view && summaryLoaded.value && !cancellation.value &&
    !writing.value &&
    !pendingWrite.value,
);
const canRetry = computed(
  () =>
    rights.value.retry &&
    validUuid(periodId.value) &&
    rights.value.view && cancellation.value?.state === "failed" &&
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
        throw new Error(t('admin.copy.4a571ca7'));
    }
    periods.value = result.periods;
    if (!result.periods.some((item) => item.id === periodId.value)) {
      periodId.value = "";
      periodVersion.value = "";
    }
    notice.value = localized('admin.copy.231a79ff', { count: result.periods.length });
  } catch (problem) {
    if (current(ticket, rights.value.view)) reportError(problem, "read");
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
    notice.value = localized('admin.copy.53980742',
      { periodNo: item.period_no, statusZh: status.zh, statusEn: status.en, version: item.version },
    );
  } catch (problem) {
    if (current(ticket, rights.value.view)) reportError(problem, "read");
  } finally {
    if (current(ticket, true)) loadingPeriod.value = false;
  }
}

async function loadSummary(quiet = false) {
  if (!rights.value.view || !validUuid(periodId.value) || loadingSummary.value || (quiet && summaryPolling))
    return;
  const brandId = props.brandId;
  const id = periodId.value.trim();
  guard.invalidate("summary");
  const ticket = guard.capture("summary", scope("summary"));
  if (quiet) summaryPolling = ticket;
  else loadingSummary.value = true;
  if (!quiet) error.value = "";
  try {
    const response = await api.getCancellation(brandId, id);
    if (!current(ticket, rights.value.view)) return;
    const result = response.cancellation;
    if (result && (result.brand_id !== brandId || result.period_id !== id))
      throw new Error(t('admin.copy.d8b99df2'));
    cancellation.value = result;
    summaryLoaded.value = true;
    if (!quiet)
      notice.value = result ? localized('admin.copy.f67ff579') : localized('admin.copy.b0f26080');
    schedulePoll();
  } catch (problem) {
    if (current(ticket, rights.value.view)) {
      summaryLoaded.value = false;
      reportError(problem, "read");
    }
  } finally {
    if (quiet) {
      if (summaryPolling === ticket) summaryPolling = null;
    } else if (current(ticket, rights.value.view)) loadingSummary.value = false;
  }
}

function validateCancel(): PeriodCancellationBody {
  const version = positiveVersion(periodVersion.value);
  const cleanReason = reason.value.trim();
  if (!validUuid(periodId.value)) throw new Error(t('admin.copy.ec713d85'));
  if (version === null) throw new Error(t('admin.copy.6776f3a2'));
  if (reasonLength.value > 500)
    throw new Error(t('admin.copy.5d042c77'));
  if (!canUseCancelForm.value)
    throw new Error(t('admin.copy.f7292a6a'));
  return { version, mode: mode.value, cause: cause.value, reason: cleanReason };
}
function reviewCancel() {
  try {
    const body = validateCancel();
    if (rights.value.view && !summaryLoaded.value)
      throw new Error(t('admin.copy.af900b56'));
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
    error.value = localized('admin.copy.ceeffa3b');
    return;
  }
  const cleanReason = retryReason.value.trim();
  if (retryReasonLength.value > 500) {
    error.value = localized('admin.copy.068b3baf');
    return;
  }
  if (!rights.value.view || !cancellation.value || cancellation.value.state !== "failed") return;
  confirmStage.value = "retry-review";
  const version = cancellation.value.version;
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
  notice.value = localized('admin.copy.26e61dfc');
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
        ? localized('admin.copy.c76a306f')
        : result.state === "completed"
          ? localized('admin.copy.78edb334')
          : localized('admin.copy.e5d08c9a');
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
          ? localized('admin.copy.6de1b196')
          : localized('admin.copy.7792806f');
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
      notice.value = localized('admin.copy.f9ed225d');
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
watch(() => props.initialPeriodId, async (id) => {
  if (!id || pendingWrite.value) return;
  periodId.value = id;
  await readPeriod();
  if (directPeriod.value?.id === periodId.value) await loadSummary();
}, { immediate: true });
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
        <h2 id="period-cancel-title">{{ t('admin.copy.7ed0de73') }}</h2>
      </div>
      <span class="brand-tag">{{ t('admin.copy.9c32661d') }} <AssociationName kind="brand" :id="brandId" :locale="associationLocale" /></span>
    </header>

    <p v-if="!rights.view" class="callout">
      {{ t('admin.copy.60800d78') }}
    </p>
    <div v-if="pendingWrite" class="panel pending">
      <b>{{ t('admin.copy.adce7dcd') }}</b>
      <p>
        {{ t('admin.copy.14eebfb4') }}
      </p>
      <button
        v-write-guard="writing"
        class="button button-primary"
        type="button"
        :disabled="
          writing ||
          pendingWrite.accountId !== account.id ||
          pendingWrite.brandId !== brandId ||
          pendingWrite.periodId !== periodId ||
          !(pendingWrite.operation === 'cancel' ? rights.cancel : rights.retry)
        "
        @click="withGlobalLoading(submitPending)"
      >
        {{ writing ? t('admin.copy.45d732b2') : t('admin.copy.c6a6aa0c') }}
      </button>
    </div>
    <template v-if="rights.view">
      <div class="panel">
        <div class="section-head">
          <h3>{{ t('admin.copy.30efb4b6') }}</h3>
          <span class="muted">{{ t('admin.period.selectionGuide') }}</span>
        </div>

        <details :open="!initialPeriodId">
          <summary>{{ t('filter.selectPeriod') }}</summary>
        <div class="field-grid">
          <label
            >{{ t('admin.copy.b50f74d5') }}
            <FilterAssociationSelect v-model="gameId" :brand-id="brandId" surface="admin" resource="period" kind="game" :label="t('filter.game')" :locale="displayLocale" :scope-key="JSON.stringify(account)" :disabled="Boolean(pendingWrite)" />
          </label>
          <button
            class="button button-secondary"
            type="button"
            :disabled="
              !validUuid(gameId) || loadingPeriods || Boolean(pendingWrite)
            "
            @click="withGlobalLoading(readPeriods)"
          >
            {{ loadingPeriods ? t('admin.copy.27342bf3') : t('admin.copy.73a00766') }}
          </button>
          <label v-if="periods.length" class="wide"
            >{{ t('admin.copy.72743752') }}
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
              <option value="">{{ t('admin.copy.14ddb4bf') }}</option>
              <option v-for="item in periods" :key="item.id" :value="item.id">
                {{ item.period_no }} · {{ periodStatusLabel(item.status) }}
              </option>
            </select>
          </label>
          <label
            >{{ t('admin.copy.c506752c') }}
            <input
              v-model="periodId"
              autocomplete="off"
              spellcheck="false"
              placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
              :disabled="Boolean(pendingWrite)"
            />
          </label>
          <button
            class="button button-secondary"
            type="button"
            :disabled="
              !validUuid(periodId) || loadingPeriod || Boolean(pendingWrite)
            "
            @click="withGlobalLoading(readPeriod)"
          >
            {{ loadingPeriod ? t('admin.copy.27342bf3') : t('admin.copy.59a78f76') }}
          </button>
          <button
            class="button button-primary"
            type="button"
            :disabled="!canRead"
            @click="withGlobalLoading(loadSummary)"
          >
            {{ loadingSummary ? t('admin.copy.27342bf3') : t('admin.copy.35a3005a') }}
          </button>
        </div>
        </details>
      </div>

      <p v-if="error" class="message error" role="alert">{{ display(error) }}</p>
      <p v-if="notice" class="message notice" role="status">{{ display(notice) }}</p>

      <div v-if="summaryLoaded" class="panel">
        <div class="section-head">
          <h3>{{ t('admin.copy.418dd66f') }}</h3>
          <span
            v-if="cancellation"
            class="state"
            :class="`state-${cancellation.state}`"
            >{{
              cancellation.state === "processing"
                ? t('admin.copy.395f62f9')
                : cancellation.state === "failed"
                  ? t('admin.copy.1feabec0')
                  : t('admin.copy.293e79cc')
            }}</span
          ><span v-else class="state state-empty">{{ t('admin.copy.ebb87c5c') }}</span>
        </div>
          <p v-if="!cancellation" class="muted empty-state">{{ t('admin.copy.082d6a87') }}</p>
        <template v-else>
          <dl class="details">
            <div>
              <dt>{{ t('admin.copy.5e0d3d70') }}</dt>
              <dd>{{ cancellation.id }}</dd>
            </div>

            <div>
              <dt>{{ t('admin.copy.0474d950') }}</dt>
              <dd>
                {{ modeLabel(cancellation.mode) }} /
                {{ causeLabel(cancellation.cause) }}
              </dd>
            </div>
            <div v-if="cancellation.reason">
              <dt>{{ t('admin.copy.ead40fc2') }}</dt>
              <dd>{{ cancellation.reason }}</dd>
            </div>
            <div>
              <dt>{{ t('admin.copy.262c7356') }}</dt>
              <dd><AssociationName :record="cancellation" kind="admin" :id="cancellation.created_by" :locale="associationLocale" /></dd>
            </div>
            <div>
              <dt>{{ t('admin.copy.165ebd73') }}</dt>
              <dd>{{ displayDate(cancellation.created_at) }}</dd>
            </div>
            <div>
              <dt>{{ t('admin.copy.670c5f97') }}</dt>
              <dd>{{ displayDate(cancellation.completed_at) }}</dd>
            </div>
          </dl>
          <div class="counts" :aria-label="t('admin.copy.35c12d6c')">
            <div>
              <span>{{ t('admin.copy.aeb697d4') }}</span><b>{{ cancellation.total_count }}</b>
            </div>
            <div>
              <span>{{ t('admin.copy.99f247fd') }}</span><b>{{ cancellation.pending_count }}</b>
            </div>
            <div>
              <span>{{ t('admin.copy.f5e49aba') }}</span><b>{{ cancellation.refunded_count }}</b>
            </div>
            <div>
              <span>{{ t('admin.copy.5d511341') }}</span
              ><b>{{ cancellation.already_refunded_count }}</b>
            </div>
            <div>
              <span>{{ t('admin.copy.1feabec0') }}</span><b>{{ cancellation.failed_count }}</b>
            </div>
          </div>
          <p v-if="cancellation.last_error_code" class="callout error-code">
            {{ t('admin.copy.326cb409') }}<code>{{ cancellation.last_error_code ? displayPublicError(publicErrorMessage({ code: cancellation.last_error_code })) : "—" }}</code
            >{{ t('admin.copy.1ce44f67') }}
          </p>
          <p v-if="cancellation.state === 'processing'" class="muted">
            {{ t('admin.copy.5de9b627') }}
          </p>
          <p v-if="cancellation.state === 'completed'" class="muted">
            {{ t('admin.copy.f92acd02') }}
          </p>
        </template>
      </div>

      <div v-if="rights.cancel && summaryLoaded && !cancellation" class="panel">
        <div class="section-head"><h3>{{ t('admin.copy.5d24429f') }}</h3></div>
        <div class="field-grid">
          <label
            >{{ t('admin.copy.328ec45d') }}
            <select
              v-model="mode"
              :aria-label="t('admin.copy.328ec45d')"
              :disabled="Boolean(pendingWrite)"
              @change="
                cause =
                  mode === 'bet_cancelled' ? 'operator_cancel' : 'no_result';
                confirmStage = 'idle';
              "
            >
              <option value="bet_cancelled">{{ t('admin.copy.a1834cc2') }}</option>
              <option value="judged_cancelled">
                {{ t('admin.copy.6782f607') }}
              </option>
            </select>
          </label>
          <label
            >{{ t('admin.copy.5670d7e8') }}
            <select
              v-model="cause"
              :aria-label="t('admin.copy.5670d7e8')"
              :disabled="Boolean(pendingWrite)"
              @change="confirmStage = 'idle'"
            >
              <option v-if="mode === 'bet_cancelled'" value="operator_cancel">
                {{ t('admin.copy.644b3753') }}
              </option>
              <template v-else>
                <option value="no_result">{{ t('admin.copy.ced2d172') }}</option>
                <option value="invalid_result">{{ t('admin.copy.9aec2fe0') }}</option>
              </template>
            </select>
          </label>
          <p v-if="mode === 'bet_cancelled'" class="hint wide">
            {{ t('admin.copy.bc05fb72') }}
          </p>
          <p v-else class="hint wide">
            {{ t('admin.copy.c7d0879a') }}
          </p>

        </div>
        <div class="actions">
          <button
            v-if="confirmStage !== 'review'"
            class="button button-secondary"
            type="button"
            :disabled="!canCancel || !canUseCancelForm || Boolean(pendingWrite)"
            @click="reviewCancel"
          >
            {{ t('admin.copy.ac1bb6b5') }}
          </button>
          <template v-else>
            <p class="confirm-note">
              {{ t('admin.copy.6bf8c111') }}
            </p>
            <button
              v-write-guard="writing"
              class="button button-primary button-danger"
              type="button"
              :disabled="!canCancel || !canUseCancelForm"
              @click="confirmWrite"
            >
              {{ writing ? t('admin.copy.46511dc0') : t('admin.copy.cace7851') }}
            </button>
          </template>
        </div>
      </div>

      <div
        v-if="rights.retry && cancellation?.state === 'failed'"
        class="panel"
      >
        <div class="section-head"><h3>{{ t('admin.copy.b757e351') }}</h3></div>
        <p class="hint">
          {{ t('admin.versionDisplay.confirmRetry') }}
        </p>

        <div class="actions">
          <button
            v-if="confirmStage !== 'retry-review'"
            class="button button-secondary"
            type="button"
            :disabled="!canRetry || Boolean(pendingWrite)"
            @click="reviewRetry"
          >
            {{ t('admin.copy.a6f031a3') }}
          </button>
          <template v-else>
            <p class="confirm-note">{{ t('admin.versionDisplay.confirmRetry') }}</p>
            <button
              v-write-guard="writing"
              class="button button-primary"
              type="button"
              :disabled="!canRetry"
              @click="confirmWrite"
            >
              {{ writing ? t('admin.copy.46511dc0') : t('admin.copy.0927cf24') }}
            </button>
          </template>
        </div>
      </div>
    </template>

    <p v-if="!rights.view && (rights.cancel || rights.retry)" class="hint">{{ t('admin.versionDisplay.readRequired') }}</p>
  </section>
</template>

<style scoped>
details > summary { min-height:44px; padding:10px 0; box-sizing:border-box; cursor:pointer; }
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
  color: var(--muted);
  font-size: 12px;
  line-height: 1.6;
  margin: 5px 0 0;
  overflow-wrap: anywhere;
}
.eyebrow {
  color: var(--primary);
  font-size: 11px;
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
  border: 1px solid var(--line);
  border-radius: 9px;
  background: var(--surface);
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
  background: var(--surface);
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
.button-primary {
  color: var(--surface);
  background: #405bd6;
}
.button-secondary {
  color: #455064;
  background: #eef1f7;
}
.button-danger {
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
  background: var(--surface);
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
  background: var(--canvas);
}
.details dt {
  color: var(--muted);
  font-size: 11px;
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
  border: 1px solid var(--line);
  border-radius: 7px;
  text-align: center;
}
.counts span {
  color: var(--muted);
  font-size: 11px;
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
  color: var(--muted);
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
