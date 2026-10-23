<script setup lang="ts">
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { copyAssociationNames, AssociationName } from "@lottery/shared";
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
import DetailModal from "@lottery/shared/DetailModal.vue"
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { useAdminI18n } from "./i18n";
import type { LocalizedMessage } from "@lottery/shared";
import { createReportArchiveTasksApi, reportArchiveTasksPermissions, type ReportArchivePolicy, type ReportArchiveTask } from "./report-archive-tasks-api";
import { createReportArchivePolicyApi, reportArchivePolicyPermissions } from "./report-archive-policy-api";
import {
  clearAllReportArchivePolicyIntents,
  clearReportArchivePolicyIntent,
  classifyReportArchivePolicyFailure,
  createReportArchivePolicyIntent,
  getReportArchivePolicyIntent,
  markReportArchivePolicyAcknowledged,
  markReportArchivePolicyConflict,
  reportArchivePolicyIntentChanges,
  reportArchivePolicySessionGeneration,
  setReportArchivePolicyIntent,
  type ReportArchivePolicyIntent,
} from "./report-archive-policy-state";
import {
  clearAllReportArchiveTaskRetryIntents,
  clearReportArchiveTaskRetryIntent,
  classifyReportArchiveTaskRetryFailure,
  createReportArchiveTaskRetryIntent,
  getReportArchiveTaskRetryIntent,
  getReportArchiveTaskRetryIntents,
  markReportArchiveTaskRetryAcknowledged,
  markReportArchiveTaskRetryConflict,
  reportArchiveTaskIntentChanges,
  reportArchiveTaskSessionGeneration,
  setReportArchiveTaskRetryIntent,
  type ReportArchiveTaskRetryIntent,
} from "./report-archive-tasks-state";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, message, display } = useAdminI18n();
const api = createReportArchiveTasksApi(), policyApi = createReportArchivePolicyApi(), pageSize = 20;
const rights = computed(() => reportArchiveTasksPermissions(props.account, props.brandId));
const policyRights = computed(() => reportArchivePolicyPermissions(props.account, props.brandId));
const scope = computed(() => JSON.stringify([props.account.id, props.brandId, props.account.super_admin, props.account.brand_ids, props.account.permissions, props.account.permissions_by_brand, props.account.platform_permissions, rights.value, policyRights.value]));
const policy = ref<ReportArchivePolicy | null>(null), policyError = ref<string | LocalizedMessage>("");
const policyIntent = ref<ReportArchivePolicyIntent | null>(null), policyDaily = ref(false), policyMonthly = ref(false), policyReason = ref("");
const policyReview = ref<ReportArchivePolicyIntent | null>(null), policyReviewCurrent = ref<ReportArchivePolicy | null>(null);
const policyConfirmed = ref(false), policyChecked = ref(false), policyReloaded = ref(false);
const items = ref<ReportArchiveTask[]>([]), total = ref("0"), offset = ref(0), listError = ref<string | LocalizedMessage>("");
const selected = ref<ReportArchiveTask | null>(null), detailError = ref<string | LocalizedMessage>("");
const pending = ref<ReportArchiveTaskRetryIntent | null>(null), reason = ref(""), checked = ref(false), reloaded = ref(false);
const review = ref<ReportArchiveTaskRetryIntent | null>(null), confirmed = ref(false);
const policyLoading = ref(false), listLoading = ref(false), detailLoading = ref(false), writing = ref(false), policyWriting = ref(false);
const notice = ref<string | LocalizedMessage>(""), retryError = ref<string | LocalizedMessage>(""), policyWriteError = ref<string | LocalizedMessage>("");
let alive = true, policyTicket = 0, listTicket = 0, readTicket = 0, writeTicket = 0, policyWriteTicket = 0;
const nextPage = computed(() => BigInt(offset.value) + BigInt(pageSize) < BigInt(total.value) && offset.value + pageSize <= 1_000_000);
const isRetryEnabled = computed(() => rights.value.retry && Boolean(selected.value && selected.value.state === "failed") && !pending.value && !writing.value);
const isPolicyEditorEnabled = computed(() => policyRights.value.write && Boolean(policy.value) && !policyLoading.value && !policyError.value && !policyIntent.value && !policyReview.value && !policyWriting.value);
const isPolicySaveEnabled = computed(() => isPolicyEditorEnabled.value &&
  (policyDaily.value !== policy.value?.daily_enabled || policyMonthly.value !== policy.value?.monthly_enabled) &&
  Boolean(policy.value && Number.isSafeInteger(policy.value.version) && policy.value.version > 0 && policy.value.version < Number.MAX_SAFE_INTEGER));

function windowKindLabel(kind: "daily" | "monthly") { return kind === "daily" ? t('admin.copy.3c770216') : t('admin.copy.5505e809'); }
function refreshPending() { pending.value = getReportArchiveTaskRetryIntents(props.account.id, props.brandId)[0] ?? null; }
function refreshPolicyIntent() { policyIntent.value = getReportArchivePolicyIntent(props.account.id, props.brandId); }
function current(ticket: number, lane: "policy" | "list" | "read" | "write", captured: string, generation: number) {
  const actual = lane === "policy" ? policyTicket : lane === "list" ? listTicket : lane === "read" ? readTicket : writeTicket;
  return alive && actual === ticket && captured === scope.value && generation === reportArchiveTaskSessionGeneration();
}
function sessionInvalid() { clearAllReportArchivePolicyIntents(); clearAllReportArchiveTaskRetryIntents(); emit("session-invalid"); }
function readFailure(problem: unknown): string | LocalizedMessage {
  if (problem instanceof AdminApiError && problem.status === 401) { sessionInvalid(); return ""; }
  return problem instanceof Error ? publicErrorMessage(problem) : message('admin.archiveTask.readFailure');
}
async function refreshLists() {
  await Promise.all([loadPolicy(), loadList()]);
}
async function loadPolicy() {
  if (!rights.value.view) return false;
  const ticket = ++policyTicket, captured = scope.value, generation = reportArchiveTaskSessionGeneration(), intentGeneration = reportArchivePolicySessionGeneration();
  policyLoading.value = true; policyError.value = "";
  try {
    const result = await api.policy(props.brandId);
    if (!current(ticket, "policy", captured, generation) || intentGeneration !== reportArchivePolicySessionGeneration()) return false;
    policy.value = result;
    policyDaily.value = result.daily_enabled; policyMonthly.value = result.monthly_enabled;
    refreshPolicyIntent(); return true;
  } catch (problem) {
    if (current(ticket, "policy", captured, generation) && intentGeneration === reportArchivePolicySessionGeneration()) {
      policy.value = null; policyDaily.value = false; policyMonthly.value = false;
      policyError.value = readFailure(problem);
    }
    return false;
  } finally { if (current(ticket, "policy", captured, generation) && intentGeneration === reportArchivePolicySessionGeneration()) policyLoading.value = false; }
}
async function loadList(next = offset.value) {
  if (!rights.value.view) return false;
  const ticket = ++listTicket, captured = scope.value, generation = reportArchiveTaskSessionGeneration();
  listLoading.value = true; listError.value = "";
  try {
    const result = await api.list(props.brandId, pageSize, next);
    if (!current(ticket, "list", captured, generation)) return false;
    items.value = result.items; total.value = result.total_count; offset.value = result.offset; return true;
  } catch (problem) {
    if (current(ticket, "list", captured, generation)) listError.value = readFailure(problem);
    return false;
  } finally { if (current(ticket, "list", captured, generation)) listLoading.value = false; }
}
async function loadTask(id: string) {
  if (!rights.value.view) return false;
  const ticket = ++readTicket, captured = scope.value, generation = reportArchiveTaskSessionGeneration();
  detailLoading.value = true; detailError.value = ""; selected.value = null;
  try {
    const result = await api.read(props.brandId, id);
    if (!current(ticket, "read", captured, generation)) return false;
    selected.value = result; reason.value = ""; return true;
  } catch (problem) {
    if (current(ticket, "read", captured, generation)) detailError.value = readFailure(problem);
    return false;
  } finally { if (current(ticket, "read", captured, generation)) detailLoading.value = false; }
}
function beginRetry() {
  if (!isRetryEnabled.value || !selected.value) return;
  const intent = createReportArchiveTaskRetryIntent(props.account.id, props.brandId, selected.value.id, selected.value.version, reason.value, createIdempotencyKey());
  if (!intent || pending.value) { refreshPending(); return; }
  review.value = intent; confirmed.value = false; retryError.value = "";
}
async function submitReviewedRetry() {
  const intent = review.value;
  if (!intent || !confirmed.value || !rights.value.retry || writing.value || !setReportArchiveTaskRetryIntent(intent)) return;
  review.value = null; confirmed.value = false; refreshPending(); checked.value = false; reloaded.value = false;
  await sendRetry(intent);
}
async function sendRetry(intent: ReportArchiveTaskRetryIntent) {
  if (!rights.value.retry || writing.value || intent.actorId !== props.account.id || intent.brandId !== props.brandId) return;
  const ticket = ++writeTicket, captured = scope.value, generation = reportArchiveTaskSessionGeneration();
  writing.value = true;
  try {
    const receipt = await api.retry(intent.brandId, intent.taskId, { version: intent.version, reason: intent.reason }, intent.key, intent.actorId);
    if (!current(ticket, "write", captured, generation)) return;
    markReportArchiveTaskRetryAcknowledged(intent, receipt); refreshPending();
    notice.value = message('admin.archiveTask.retryAcknowledged');
    await refreshFrozen(intent);
  } catch (problem) {
    if (generation !== reportArchiveTaskSessionGeneration()) return;
    if (problem instanceof AdminApiError && problem.status === 401) { if (current(ticket, "write", captured, generation)) sessionInvalid(); return; }
    const classification = classifyReportArchiveTaskRetryFailure(problem instanceof AdminApiError ? problem.status : undefined);
    if (classification === "conflict") markReportArchiveTaskRetryConflict(intent);
    if (classification === "definitive") clearReportArchiveTaskRetryIntent(intent.actorId, intent.brandId, intent.taskId, intent.key);
    if (!current(ticket, "write", captured, generation)) return;
    refreshPending(); checked.value = false; reloaded.value = false;
    retryError.value = classification === "unknown"
      ? message('admin.archiveTask.retryUnknown')
      : classification === "conflict"
        ? message('admin.archiveTask.retryConflict')
        : problem instanceof Error ? publicErrorMessage(problem) : message('admin.archiveTask.retryRejected');
  } finally { if (current(ticket, "write", captured, generation)) writing.value = false; }
}
function beginPolicySave() {
  if (!isPolicySaveEnabled.value || !policy.value) return;
  const intent = createReportArchivePolicyIntent(props.account.id, props.brandId, policy.value.version, policyDaily.value, policyMonthly.value, policyReason.value, createIdempotencyKey());
  if (!intent) { policyWriteError.value = message('admin.archiveTask.reasonInvalid'); return; }
  policyReview.value = intent; const snapshot = { ...policy.value }; copyAssociationNames(policy.value, snapshot); policyReviewCurrent.value = Object.freeze(snapshot); policyConfirmed.value = false; policyWriteError.value = "";
}
async function submitReviewedPolicy() {
  const intent = policyReview.value;
  if (!intent || !policyConfirmed.value || !policyRights.value.write || policyWriting.value || !setReportArchivePolicyIntent(intent)) return;
  policyReview.value = null; policyReviewCurrent.value = null; policyConfirmed.value = false; policyChecked.value = false; policyReloaded.value = false;
  policyReason.value = ""; refreshPolicyIntent();
  await sendPolicyUpdate(intent);
}
async function sendPolicyUpdate(intent: ReportArchivePolicyIntent) {
  if (!policyRights.value.write || policyWriting.value || intent.actorId !== props.account.id || intent.brandId !== props.brandId) return;
  const ticket = ++policyWriteTicket, captured = scope.value, generation = reportArchivePolicySessionGeneration();
  policyWriting.value = true; policyWriteError.value = "";
  try {
    const receipt = await policyApi.update(intent.brandId, {
      version: intent.version, daily_enabled: intent.daily_enabled, monthly_enabled: intent.monthly_enabled, reason: intent.reason,
    }, intent.key, intent.actorId);
    // The intent transition is guarded by its generation and frozen key, even if the page scope changed in flight.
    markReportArchivePolicyAcknowledged(intent, receipt);
    if (!currentPolicy(ticket, captured, generation)) return;
    refreshPolicyIntent();
    await loadPolicy();
  } catch (problem) {
    if (generation !== reportArchivePolicySessionGeneration()) return;
    if (problem instanceof AdminApiError && problem.status === 401) {
      if (currentPolicy(ticket, captured, generation)) sessionInvalid();
      return;
    }
    const classification = classifyReportArchivePolicyFailure(problem instanceof AdminApiError ? problem.status : undefined);
    if (classification === "conflict") {
      markReportArchivePolicyConflict(intent);
      if (currentPolicy(ticket, captured, generation)) {
        refreshPolicyIntent(); policyChecked.value = false; policyReloaded.value = false;
        policyWriteError.value = message('admin.archiveTask.policyConflict');
        await reloadPolicyForDiscard(intent);
      }
    } else if (classification === "definitive") {
      clearReportArchivePolicyIntent(intent.actorId, intent.brandId, intent.key);
      if (currentPolicy(ticket, captured, generation)) {
        refreshPolicyIntent(); policyWriteError.value = problem instanceof Error ? publicErrorMessage(problem) : message('admin.archiveTask.policyRejected');
      }
    } else if (currentPolicy(ticket, captured, generation)) {
      refreshPolicyIntent(); policyChecked.value = false; policyReloaded.value = false;
      policyWriteError.value = message('admin.archiveTask.policyUnknown');
    }
  } finally { if (currentPolicy(ticket, captured, generation)) policyWriting.value = false; }
}
function currentPolicy(ticket: number, captured: string, generation: number) {
  return alive && policyWriteTicket === ticket && captured === scope.value && generation === reportArchivePolicySessionGeneration();
}
async function replayOriginalPolicy() {
  const intent = policyIntent.value;
  if (!intent || intent.phase !== "unknown" || !policyRights.value.write || policyWriting.value || !policyChecked.value) return;
  const saved = getReportArchivePolicyIntent(intent.actorId, intent.brandId);
  if (!saved || saved.phase !== "unknown" || saved.key !== intent.key || saved.version !== intent.version || saved.reason !== intent.reason ||
    saved.daily_enabled !== intent.daily_enabled || saved.monthly_enabled !== intent.monthly_enabled) return;
  policyChecked.value = false;
  await sendPolicyUpdate(saved);
}
async function reloadPolicyForDiscard(intent = policyIntent.value) {
  if (!intent || intent.phase === "unknown" || !policyRights.value.view || policyLoading.value) return;
  const captured = scope.value, generation = reportArchivePolicySessionGeneration(), key = intent.key;
  policyReloaded.value = false; policyChecked.value = false;
  const ok = await loadPolicy();
  if (ok && alive && captured === scope.value && generation === reportArchivePolicySessionGeneration() &&
    policyIntent.value?.key === key && policyIntent.value.phase !== "unknown") policyReloaded.value = true;
}
function discardPolicyIntent() {
  const intent = policyIntent.value;
  if (!intent || intent.phase === "unknown" || !policyReloaded.value || !policyChecked.value || policyWriting.value) return;
  clearReportArchivePolicyIntent(intent.actorId, intent.brandId, intent.key);
  refreshPolicyIntent(); policyDaily.value = policy.value?.daily_enabled ?? false; policyMonthly.value = policy.value?.monthly_enabled ?? false;
  policyReason.value = ""; policyChecked.value = false; policyReloaded.value = false; policyWriteError.value = "";
}
async function replayOriginal() {
  const intent = pending.value;
  if (!intent || intent.phase !== "unknown" || !rights.value.retry || writing.value || !checked.value) return;
  const saved = getReportArchiveTaskRetryIntent(intent.actorId, intent.brandId, intent.taskId);
  if (!saved || saved.phase !== "unknown" || saved.key !== intent.key || saved.version !== intent.version || saved.reason !== intent.reason) return;
  checked.value = false;
  await sendRetry(saved);
}
async function refreshFrozen(intent = pending.value) {
  if (!intent || !rights.value.view || listLoading.value || detailLoading.value) return;
  const captured = scope.value, generation = reportArchiveTaskSessionGeneration(), key = intent.key;
  reloaded.value = false; checked.value = false;
  const [listOk, taskOk] = await Promise.all([loadList(offset.value), loadTask(intent.taskId)]);
  if (alive && captured === scope.value && generation === reportArchiveTaskSessionGeneration() && pending.value?.key === key && pending.value.phase !== "unknown") {
    reloaded.value = listOk && taskOk;
  }
}
function discardFrozen() {
  const intent = pending.value;
  if (!intent || intent.phase === "unknown" || !reloaded.value || !checked.value || writing.value) return;
  clearReportArchiveTaskRetryIntent(intent.actorId, intent.brandId, intent.taskId, intent.key);
  refreshPending(); reason.value = ""; checked.value = false; reloaded.value = false; retryError.value = "";
}
function retryForSelected() { return selected.value ? getReportArchiveTaskRetryIntent(props.account.id, props.brandId, selected.value.id) : null; }
function reset() {
  policyTicket++; listTicket++; readTicket++; writeTicket++; policyWriteTicket++;
  policy.value = null; items.value = []; total.value = "0"; offset.value = 0; selected.value = null;
  policyLoading.value = listLoading.value = detailLoading.value = writing.value = policyWriting.value = false;
  policyError.value = listError.value = detailError.value = retryError.value = notice.value = policyWriteError.value = "";
  policyDaily.value = policyMonthly.value = false; policyReason.value = ""; policyReview.value = null; policyReviewCurrent.value = null; policyConfirmed.value = false;
  policyChecked.value = policyReloaded.value = false; refreshPolicyIntent();
  reason.value = ""; checked.value = reloaded.value = false; review.value = null; confirmed.value = false; refreshPending();
}
watch(scope, () => { reset(); if (rights.value.view) { void loadPolicy(); void loadList(0); } }, { immediate: true, flush: "sync" });
let observedGeneration = reportArchiveTaskSessionGeneration(), observedPolicyGeneration = reportArchivePolicySessionGeneration();
watch([reportArchiveTaskIntentChanges, reportArchivePolicyIntentChanges], () => {
  const taskGeneration = reportArchiveTaskSessionGeneration(), policyGeneration = reportArchivePolicySessionGeneration();
  if (observedGeneration !== taskGeneration || observedPolicyGeneration !== policyGeneration) {
    observedGeneration = taskGeneration; observedPolicyGeneration = policyGeneration; reset();
  } else { refreshPending(); refreshPolicyIntent(); }
});
onBeforeUnmount(() => { alive = false; policyTicket++; listTicket++; readTicket++; writeTicket++; policyWriteTicket++; });
function closeDetails() {
  readTicket++; selected.value = null; detailLoading.value = false; detailError.value = '';
}
</script>

<template>
  <section class="archive-tasks panel">
    <header><div><h2>{{ t('admin.copy.6477d963') }}</h2></div><div class="actions"><button v-refresh-button="t('admin.copy.894e2a7d')" class="button button-secondary" data-testid="archive-tasks-refresh" :disabled="!rights.view || policyLoading || listLoading" @click="withGlobalLoading(refreshLists)">{{ t('admin.copy.894e2a7d') }}</button></div></header>
    <p v-if="!rights.view" role="alert">{{ t('admin.copy.da2b5fad') }}</p>
    <template v-else>
      <p v-if="notice" role="status">{{ display(notice) }}</p><p v-if="retryError" role="alert">{{ display(retryError) }}</p>
      <section class="subpanel" aria-labelledby="archive-policy-heading"><h3 id="archive-policy-heading">{{ t('admin.copy.1da4ae4a') }}</h3><p v-global-loading v-if="policyLoading" role="status">{{ t('admin.copy.27342bf3') }}</p><p v-if="policyError" role="alert" data-testid="archive-tasks-policy-error">{{ t('admin.copy.76d921e0') }}{{ display(policyError) }}</p>
        <dl v-if="policy" data-testid="archive-tasks-policy"><dt>{{ t('admin.copy.4b2d7fdd') }}</dt><dd><AssociationName :record="policy" kind="brand" :id="policy.brand_id" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.4a2571f0') }}</dt><dd><BusinessDetails :value="policy.daily_enabled" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.029856d3') }}</dt><dd><BusinessDetails :value="policy.monthly_enabled" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.49ac5ccb') }}</dt><dd>{{ policy.daily_start_period ?? "—" }}</dd><dt>{{ t('admin.copy.758785b6') }}</dt><dd>{{ policy.monthly_start_period ?? "—" }}</dd><dt>{{ t('admin.copy.f257539c') }}</dt><dd>{{ policy.timezone }}</dd><dt>{{ t('admin.copy.2aed1b31') }}</dt><dd>{{ displayDate(policy.updated_at) }}</dd></dl>
        <div class="policy-editor" data-testid="archive-policy-editor">
          <h4>{{ t('admin.copy.8275a384') }}</h4>
          <p>{{ t('admin.copy.f3cd1797') }}</p>
          <p v-if="!policyRights.write">{{ t('admin.copy.926be966') }}</p>
          <p v-if="policyIntent" role="status">{{ policyIntent.phase === "unknown" ? t('admin.copy.c901f346') : policyIntent.phase === "conflict" ? t('admin.copy.766de75d') : t('admin.copy.2e6cc45d') }}</p>
          <p v-if="policyWriteError" role="alert">{{ display(policyWriteError) }}</p>
          <label class="policy-check"><input v-model="policyDaily" data-testid="archive-policy-daily" type="checkbox" :disabled="!isPolicyEditorEnabled" />{{ t('admin.copy.e322c0a3') }}</label>
          <label class="policy-check"><input v-model="policyMonthly" data-testid="archive-policy-monthly" type="checkbox" :disabled="!isPolicyEditorEnabled" />{{ t('admin.copy.82082484') }}</label>
          <button class="button" data-testid="archive-policy-save" :disabled="!isPolicySaveEnabled" @click="beginPolicySave">{{ t('admin.copy.946d4890') }}</button>
        </div>
        <section v-if="policyIntent" class="frozen" data-testid="archive-policy-frozen">
          <h4>{{ policyIntent.phase === "unknown" ? t('admin.copy.215225a7') : policyIntent.phase === "conflict" ? t('admin.copy.ffa507ed') : t('admin.copy.aeedd3b8') }}</h4>
          <p>{{ policyIntent.phase === 'acknowledged' ? t('admin.archiveTask.policyAcknowledgedNote') : policyIntent.phase === 'conflict' ? t('admin.archiveTask.policyConflict') : t('admin.copy.95f0e69e') }}</p>
          <dl><dt>{{ t('admin.copy.d48825ff') }}</dt><dd><AssociationName kind="admin" :id="policyIntent.actorId" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.9c32661d') }}</dt><dd><AssociationName kind="brand" :id="policyIntent.brandId" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.9aea13b1') }}</dt><dd><BusinessDetails :value="policyIntent.daily_enabled" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.ee98bce0') }}</dt><dd><BusinessDetails :value="policyIntent.monthly_enabled" :locale="associationLocale" /></dd></dl>
          <div v-if="policyIntent.phase === 'acknowledged' && policyIntent.receipt" class="policy-receipt" data-testid="archive-policy-receipt">
            <h4>{{ t('admin.copy.88332f7e') }}</h4>
            <dl><dt>{{ t('admin.copy.4b2d7fdd') }}</dt><dd><AssociationName :record="policyIntent.receipt" kind="brand" :id="policyIntent.receipt.brand_id" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.4a2571f0') }}</dt><dd><BusinessDetails :value="policyIntent.receipt.daily_enabled" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.029856d3') }}</dt><dd><BusinessDetails :value="policyIntent.receipt.monthly_enabled" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.49ac5ccb') }}</dt><dd>{{ policyIntent.receipt.daily_start_period ?? "—" }}</dd><dt>{{ t('admin.copy.758785b6') }}</dt><dd>{{ policyIntent.receipt.monthly_start_period ?? "—" }}</dd><dt>{{ t('admin.copy.f257539c') }}</dt><dd>{{ policyIntent.receipt.timezone }}</dd><dt>{{ t('admin.copy.2aed1b31') }}</dt><dd>{{ displayDate(policyIntent.receipt.updated_at) }}</dd></dl>
          </div>
          <template v-if="policyIntent.phase === 'unknown'"><label class="policy-check"><input v-model="policyChecked" data-testid="archive-policy-replay-check" type="checkbox" />{{ t('admin.copy.28e2e4ea') }}</label><button class="button button-secondary" data-testid="archive-policy-replay" :disabled="!policyRights.write || policyWriting || !policyChecked" @click="withGlobalLoading(replayOriginalPolicy)">{{ t('admin.copy.84dcf188') }}</button></template>
          <template v-else><button v-refresh-button="t('admin.copy.ecc0fdbe')" class="button button-secondary" data-testid="archive-policy-reload-frozen" :disabled="policyLoading || policyWriting" @click="withGlobalLoading(reloadPolicyForDiscard)">{{ t('admin.copy.ecc0fdbe') }}</button><label class="policy-check"><input v-model="policyChecked" data-testid="archive-policy-discard-check" type="checkbox" :disabled="!policyReloaded" />{{ t('admin.copy.0ae3eb5d') }}</label><button class="button button-secondary" data-testid="archive-policy-discard" :disabled="!policyReloaded || !policyChecked || policyWriting" @click="discardPolicyIntent">{{ t('admin.copy.fe5ebdd2') }}</button></template>
        </section>
      </section>
      <section class="subpanel" aria-labelledby="archive-tasks-list-heading"><h3 id="archive-tasks-list-heading">{{ t('admin.copy.7e743bd1') }}</h3><p>{{ t('admin.copy.15a79c1a') }}: {{ total }} · {{ t('admin.copy.c06359dc') }}: {{ offset }}–{{ offset + items.length }}</p><p v-global-loading v-if="listLoading" role="status">{{ t('admin.copy.27342bf3') }}</p><p v-if="listError" role="alert" data-testid="archive-tasks-list-error">{{ t('admin.copy.927f1e29') }}{{ display(listError) }}</p><p v-if="!listLoading && items.length === 0" class="empty-state">{{ t('admin.copy.199a53b5') }}</p><ul class="task-list" data-testid="archive-tasks-list" :aria-label="t('admin.copy.0bbf8d91')"><li v-for="item in items" :key="item.id"><button class="button button-secondary" :aria-label="`${windowKindLabel(item.window.kind)} ${item.window.kind} · ${item.window.period_key} · ${item.state} · ${item.id}`" :disabled="detailLoading" @click="withGlobalLoading(loadTask, item.id)">{{ windowKindLabel(item.window.kind) }} · {{ item.window.period_key }} · <BusinessDetails :value="item.state" :locale="associationLocale" /><small>{{ item.id }}</small></button><small>{{ displayDate(item.updated_at) }}</small></li></ul><div class="actions"><button class="button button-secondary" :disabled="listLoading || offset === 0" @click="withGlobalLoading(loadList, Math.max(0, offset - pageSize))">{{ t('admin.copy.70598166') }}</button><button class="button button-secondary" :disabled="listLoading || !nextPage" @click="withGlobalLoading(loadList, offset + pageSize)">{{ t('admin.copy.ee97c955') }}</button></div></section>
      <DetailModal v-if="!!selected || !!detailError" :title="t('admin.copy.01db40da')" :close-label="t('admin.copy.2bb2cb1d')" :open="!review" @close="closeDetails">
<section v-if="selected || detailError" class="subpanel" data-testid="archive-tasks-detail"><h3>{{ t('admin.copy.177e340e') }}</h3><p v-global-loading v-if="detailLoading" role="status">{{ t('admin.copy.984a463e') }}</p><p v-if="detailError" role="alert" data-testid="archive-tasks-detail-error">{{ t('admin.copy.59fda136') }}{{ display(detailError) }}</p><dl v-if="selected"><dt>ID</dt><dd>{{ selected.id }}</dd><dt>{{ t('admin.copy.4b2d7fdd') }}</dt><dd><AssociationName :record="selected" kind="brand" :id="selected.brand_id" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.632f7e0d') }}</dt><dd>{{ windowKindLabel(selected.window.kind) }}</dd><dt>{{ t('admin.copy.6a5e9429') }}</dt><dd>{{ selected.window.period_key }}</dd><dt>{{ t('admin.copy.f257539c') }}</dt><dd>{{ selected.window.timezone }}</dd><dt>{{ t('admin.copy.db2c3883') }}</dt><dd>{{ displayDate(selected.window.from) }}</dd><dt>{{ t('admin.copy.9a57baca') }}</dt><dd>{{ displayDate(selected.window.to) }}</dd><dt>{{ t('admin.copy.cf6d9c6d') }}</dt><dd><BusinessDetails :value="selected.state" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.19b46a5c') }}</dt><dd>{{ selected.attempt_count }}</dd><dt>{{ t('admin.copy.93b798e6') }}</dt><dd>{{ selected.archive_id ?? "—" }}</dd><dt>{{ t('admin.copy.9cf6d9e9') }}</dt><dd>{{ selected.last_error_code ? displayPublicError(publicErrorMessage({ code: selected.last_error_code })) : "—" }}</dd><dt>{{ t('admin.copy.165ebd73') }}</dt><dd>{{ displayDate(selected.created_at) }}</dd><dt>{{ t('admin.copy.2aed1b31') }}</dt><dd>{{ displayDate(selected.updated_at) }}</dd></dl>
        <div v-if="selected?.state === 'failed'" class="retry-form"><h4>{{ t('admin.copy.0783a071') }}</h4><p v-if="!rights.retry">{{ t('admin.copy.7e9c1f21') }}</p><p v-if="rights.retry && !props.account.super_admin">{{ t('admin.copy.58a2bef2') }}</p><button class="button" data-testid="archive-tasks-retry" :disabled="!isRetryEnabled" @click="beginRetry">{{ t('admin.copy.826aa0d5') }}</button></div>
      </section>
</DetailModal>
      <section v-if="pending" class="frozen" data-testid="archive-tasks-frozen"><h3>{{ pending.phase === "unknown" ? t('admin.copy.d6d50119') : pending.phase === "conflict" ? t('admin.copy.b4d7c207') : t('admin.copy.7e89ab2b') }}</h3><p>{{ pending.phase === 'acknowledged' ? t('admin.archiveTask.retryAcknowledgedNote') : pending.phase === 'conflict' ? t('admin.archiveTask.retryConflict') : t('admin.copy.15f0412c') }}</p><dl><dt>{{ t('admin.copy.d48825ff') }}</dt><dd><AssociationName kind="admin" :id="pending.actorId" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.9c32661d') }}</dt><dd><AssociationName kind="brand" :id="pending.brandId" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.cf731ef2') }}</dt><dd>{{ pending.taskId }}</dd></dl>
        <div v-if="pending.phase === 'acknowledged' && pending.receipt" class="ack-receipt" data-testid="archive-tasks-receipt"><div data-testid="archive-tasks-ack-receipt"><h4>{{ t('admin.copy.da2e8c3a') }}</h4><dl><dt>ID</dt><dd>{{ pending.receipt.id }}</dd><dt>{{ t('admin.copy.4b2d7fdd') }}</dt><dd><AssociationName :record="pending.receipt" kind="brand" :id="pending.receipt.brand_id" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.31d2d8ab') }}</dt><dd>{{ windowKindLabel(pending.receipt.window.kind) }}</dd><dt>{{ t('admin.copy.6a5e9429') }}</dt><dd>{{ pending.receipt.window.period_key }}</dd><dt>{{ t('admin.copy.f257539c') }}</dt><dd>{{ pending.receipt.window.timezone }}</dd><dt>{{ t('admin.copy.db2c3883') }}</dt><dd>{{ pending.receipt.window.from }}</dd><dt>{{ t('admin.copy.9a57baca') }}</dt><dd>{{ pending.receipt.window.to }}</dd><dt>{{ t('admin.copy.8d422e4a') }}</dt><dd><BusinessDetails :value="pending.receipt.state" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.31ad622b') }}</dt><dd>{{ pending.receipt.attempt_count }}</dd><dt>{{ t('admin.copy.25a07919') }}</dt><dd>{{ pending.receipt.archive_id ?? "—" }}</dd><dt>{{ t('admin.copy.50208fdc') }}</dt><dd>{{ pending.receipt.last_error_code ? displayPublicError(publicErrorMessage({ code: pending.receipt.last_error_code })) : "—" }}</dd><dt>{{ t('admin.copy.165ebd73') }}</dt><dd>{{ displayDate(pending.receipt.created_at) }}</dd><dt>{{ t('admin.copy.a76e654d') }}</dt><dd>{{ displayDate(pending.receipt.updated_at) }}</dd></dl><p>{{ t('admin.copy.4f2eda64') }}</p></div></div>
        <template v-if="pending.phase === 'unknown'"><label class="replay-confirm"><input v-model="checked" data-testid="archive-tasks-replay-check" type="checkbox" />{{ t('admin.copy.9c9c9be5') }}</label><button v-write-guard="writing" class="button button-secondary" data-testid="archive-tasks-replay" :disabled="!rights.retry || writing || !checked" @click="withGlobalLoading(replayOriginal)">{{ t('admin.copy.84dcf188') }}</button></template>
        <template v-else><button v-refresh-button="t('admin.copy.f08cd529')" class="button button-secondary" data-testid="archive-tasks-reload-frozen" :disabled="listLoading || detailLoading" @click="withGlobalLoading(refreshFrozen)">{{ t('admin.copy.f08cd529') }}</button><label><input v-model="checked" data-testid="archive-tasks-discard-check" type="checkbox" :disabled="!reloaded" />{{ t('admin.copy.87705adc') }}</label><button class="button button-secondary" data-testid="archive-tasks-discard" :disabled="!reloaded || !checked || writing" @click="discardFrozen">{{ t('admin.copy.18ba4f82') }}</button></template>
      </section>
      <p v-if="rights.view && !rights.retry">{{ t('admin.copy.7e2e976e') }}</p>
    </template>
    <div v-if="policyReview" class="review" role="dialog" aria-modal="true" :aria-label="t('admin.copy.ab891eb9')" data-testid="archive-policy-review"><div><h3>{{ t('admin.copy.ab891eb9') }}</h3><dl><dt>{{ t('admin.copy.d48825ff') }}</dt><dd><AssociationName kind="admin" :id="policyReview.actorId" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.9c32661d') }}</dt><dd><AssociationName kind="brand" :id="policyReview.brandId" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.9aea13b1') }}</dt><dd><BusinessDetails :value="policyReview.daily_enabled" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.ee98bce0') }}</dt><dd><BusinessDetails :value="policyReview.monthly_enabled" :locale="associationLocale" /></dd><dt v-if="policyReview.reason">{{ t('admin.copy.f7781afd') }}</dt><dd v-if="policyReview.reason">{{ policyReview.reason }}</dd></dl><h4>{{ t('admin.copy.8ae67f48') }}</h4><dl v-if="policyReviewCurrent"><dt>{{ t('admin.copy.4b2d7fdd') }}</dt><dd><AssociationName :record="policyReviewCurrent" kind="brand" :id="policyReviewCurrent.brand_id" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.4a2571f0') }}</dt><dd><BusinessDetails :value="policyReviewCurrent.daily_enabled" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.029856d3') }}</dt><dd><BusinessDetails :value="policyReviewCurrent.monthly_enabled" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.49ac5ccb') }}</dt><dd>{{ policyReviewCurrent.daily_start_period ?? "—" }}</dd><dt>{{ t('admin.copy.758785b6') }}</dt><dd>{{ policyReviewCurrent.monthly_start_period ?? "—" }}</dd><dt>{{ t('admin.copy.f257539c') }}</dt><dd>{{ policyReviewCurrent.timezone }}</dd><dt>{{ t('admin.copy.2aed1b31') }}</dt><dd>{{ displayDate(policyReviewCurrent.updated_at) }}</dd></dl><label><input v-model="policyConfirmed" data-testid="archive-policy-confirm" type="checkbox" />{{ t('admin.copy.40960348') }}</label><div class="actions"><button class="button button-secondary" @click="policyReview = null; policyReviewCurrent = null; policyConfirmed = false">{{ t('admin.copy.e2f391e7') }}</button><button v-write-guard="policyWriting" class="button" data-testid="archive-policy-submit" :disabled="!policyConfirmed || !policyRights.write || policyWriting" @click="withGlobalLoading(submitReviewedPolicy)">{{ t('admin.copy.30530eb0') }}</button></div></div></div>
    <div v-if="review" class="review" role="dialog" aria-modal="true" :aria-label="t('admin.copy.a1190dd3')" data-testid="archive-tasks-review"><div><h3>{{ t('admin.copy.a1190dd3') }}</h3><dl><dt>{{ t('admin.copy.d48825ff') }}</dt><dd><AssociationName kind="admin" :id="review.actorId" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.9c32661d') }}</dt><dd><AssociationName kind="brand" :id="review.brandId" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.cf731ef2') }}</dt><dd>{{ review.taskId }}</dd><dt v-if="review.reason">{{ t('admin.copy.f7781afd') }}</dt><dd v-if="review.reason">{{ review.reason }}</dd></dl><label><input v-model="confirmed" data-testid="archive-tasks-confirm" type="checkbox" />{{ t('admin.copy.62c4033f') }}</label><div class="actions"><button class="button button-secondary" @click="review = null; confirmed = false">{{ t('admin.copy.e2f391e7') }}</button><button v-write-guard="writing" class="button" data-testid="archive-tasks-submit" :disabled="!confirmed || !rights.retry || writing" @click="withGlobalLoading(submitReviewedRetry)">{{ t('admin.copy.fd6e1a4e') }}</button></div></div></div>
  </section>
</template>

<style scoped>
.archive-tasks{min-width:0;overflow-wrap:anywhere;padding:1.25rem}.archive-tasks header,.actions{display:flex;justify-content:space-between;align-items:flex-start;gap:.75rem;flex-wrap:wrap}.archive-tasks p{line-height:1.55;color:var(--muted)}.subpanel,.frozen{margin:1rem 0;padding:1rem;border:1px solid var(--border,#d5dce2);border-radius:.75rem;min-width:0}.frozen{background:#fbf7ed;border-color:#d5b05b}.archive-tasks dl{display:grid;grid-template-columns:minmax(140px,1fr) minmax(0,2fr);gap:.5rem 1rem}.archive-tasks dt{color:var(--muted)}.archive-tasks dd{margin:0;overflow-wrap:anywhere;white-space:pre-wrap}.task-list{list-style:none;padding:0;display:grid;gap:.5rem}.task-list li{display:flex;justify-content:space-between;align-items:center;gap:.75rem;padding:.6rem;border:1px solid var(--border,#d5dce2);border-radius:.6rem}.task-list small{display:block;overflow-wrap:anywhere}.retry-form,.policy-editor{display:grid;gap:.75rem;margin-top:1rem}.retry-form label,.policy-reason{display:grid;gap:.5rem}.retry-form textarea,.policy-reason textarea{box-sizing:border-box;width:100%;min-height:5rem;padding:.65rem;border:1px solid var(--border,#d5dce2);border-radius:.5rem;background:var(--surface);color:inherit}.policy-check{display:flex;align-items:flex-start;gap:.6rem}.policy-check input,.frozen input[type=checkbox]{width:auto;flex:none}.policy-receipt{margin-top:1rem;padding:.75rem;border:1px solid var(--border,#d5dce2);border-radius:.6rem}.review{position:fixed;inset:0;z-index:90;display:flex;align-items:center;justify-content:center;padding:1rem;background:#17253699}.review>div{box-sizing:border-box;width:38rem;max-width:100%;max-height:85dvh;overflow:auto;padding:1.25rem;border-radius:1rem;background:var(--surface)}.review label{display:flex;align-items:flex-start;gap:.6rem;min-width:0}.review input{flex:none}.review dd{min-width:0;overflow-wrap:anywhere;word-break:break-word}@media(max-width:700px){.archive-tasks{padding:1rem}.archive-tasks dl{grid-template-columns:minmax(0,1fr)}.task-list li{align-items:flex-start;flex-direction:column}}@media(max-width:380px){.review{padding:.5rem}.review>div{width:100%;padding:.8rem}.review dl{grid-template-columns:minmax(0,1fr);gap:.2rem}.review dd{margin-bottom:.45rem}.review .actions{justify-content:flex-start}.policy-check{align-items:flex-start}.policy-editor>.button,.frozen>.button{max-width:100%;white-space:normal;overflow-wrap:anywhere}}
.task-list li{min-width:0}.task-list button{display:block;min-width:0;max-width:100%;width:100%;white-space:normal;overflow-wrap:anywhere;text-align:left}
</style>
