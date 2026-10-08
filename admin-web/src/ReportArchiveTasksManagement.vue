<script setup lang="ts">
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
const { t, message } = useAdminI18n();
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
const isRetryEnabled = computed(() => rights.value.retry && Boolean(selected.value && selected.value.state === "failed") && !pending.value && !writing.value && reason.value.trim().length > 0);
const isPolicyEditorEnabled = computed(() => policyRights.value.write && Boolean(policy.value) && !policyLoading.value && !policyError.value && !policyIntent.value && !policyReview.value && !policyWriting.value);
const isPolicySaveEnabled = computed(() => isPolicyEditorEnabled.value && policyReason.value.trim().length > 0 &&
  (policyDaily.value !== policy.value?.daily_enabled || policyMonthly.value !== policy.value?.monthly_enabled) &&
  Boolean(policy.value && Number.isSafeInteger(policy.value.version) && policy.value.version > 0 && policy.value.version < Number.MAX_SAFE_INTEGER));

function copy(zh: string, en: string): LocalizedMessage { return message(zh, en); }
function windowKindLabel(kind: "daily" | "monthly") { return kind === "daily" ? t("日归档", "Daily") : t("月归档", "Monthly"); }
function refreshPending() { pending.value = getReportArchiveTaskRetryIntents(props.account.id, props.brandId)[0] ?? null; }
function refreshPolicyIntent() { policyIntent.value = getReportArchivePolicyIntent(props.account.id, props.brandId); }
function current(ticket: number, lane: "policy" | "list" | "read" | "write", captured: string, generation: number) {
  const actual = lane === "policy" ? policyTicket : lane === "list" ? listTicket : lane === "read" ? readTicket : writeTicket;
  return alive && actual === ticket && captured === scope.value && generation === reportArchiveTaskSessionGeneration();
}
function sessionInvalid() { clearAllReportArchivePolicyIntents(); clearAllReportArchiveTaskRetryIntents(); emit("session-invalid"); }
function readFailure(problem: unknown): string | LocalizedMessage {
  if (problem instanceof AdminApiError && problem.status === 401) { sessionInvalid(); return ""; }
  return problem instanceof Error ? problem.message : copy("任务读取失败，请重新读取。", "Task read failed. Reload the data.");
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
    notice.value = copy("原重试请求已获服务器确认；下面的列表和任务是独立的最新读取。", "The original retry request was acknowledged. The list and task below are independent latest reads.");
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
      ? copy("重试结果未知。原账号、品牌、任务、版本、原因和幂等键已冻结；只能按原请求重放。", "Retry outcome is unknown. The original account, brand, task, version, reason and idempotency key are frozen; only the exact request can be replayed.")
      : classification === "conflict"
        ? copy("任务版本冲突。重新读取列表和原任务并人工核对后，才能丢弃原请求。", "Task version conflict. Reload the list and original task, review them, then explicitly discard the original request.")
        : problem instanceof Error ? problem.message : copy("服务器拒绝了重试请求。", "The server rejected the retry request.");
  } finally { if (current(ticket, "write", captured, generation)) writing.value = false; }
}
function beginPolicySave() {
  if (!isPolicySaveEnabled.value || !policy.value) return;
  const intent = createReportArchivePolicyIntent(props.account.id, props.brandId, policy.value.version, policyDaily.value, policyMonthly.value, policyReason.value, createIdempotencyKey());
  if (!intent) { policyWriteError.value = copy("原因无效：请使用不超过 500 字节的普通文本。", "Invalid reason. Use plain text of at most 500 UTF-8 bytes."); return; }
  policyReview.value = intent; policyReviewCurrent.value = Object.freeze({ ...policy.value }); policyConfirmed.value = false; policyWriteError.value = "";
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
        policyWriteError.value = copy("策略版本冲突。已保存原请求；读取当前策略并人工确认后，才能丢弃。", "Policy version conflict. The original request is frozen; reload the current policy and explicitly confirm before discarding it.");
        await reloadPolicyForDiscard(intent);
      }
    } else if (classification === "definitive") {
      clearReportArchivePolicyIntent(intent.actorId, intent.brandId, intent.key);
      if (currentPolicy(ticket, captured, generation)) {
        refreshPolicyIntent(); policyWriteError.value = problem instanceof Error ? problem.message : copy("服务器拒绝了策略更新。", "The server rejected the policy update.");
      }
    } else if (currentPolicy(ticket, captured, generation)) {
      refreshPolicyIntent(); policyChecked.value = false; policyReloaded.value = false;
      policyWriteError.value = copy("更新结果未知。冻结的原账号、品牌、版本、开关、原因和幂等键只能按原请求重放。", "Update outcome is unknown. Only the exact frozen account, brand, version, flags, reason and idempotency key can be replayed.");
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
</script>

<template>
  <section class="archive-tasks panel">
    <header><div><h2>{{ t("自动归档任务", "Automatic archive tasks") }}</h2><p>{{ t("查看并管理自动归档策略与任务。首次启用从品牌时区的当天或当月开始，完整周期结束后归档；关闭后重开保留原起点，不回补历史。", "View and manage automatic archive policy and tasks. First activation starts in the current brand-local day or month and archives after that complete period; turning it off and on retains its start and does not backfill history.") }}</p></div><div class="actions"><button class="button button-secondary" data-testid="archive-tasks-refresh" :disabled="!rights.view || policyLoading || listLoading" @click="loadPolicy(); loadList()">{{ t("刷新", "Refresh") }}</button></div></header>
    <p v-if="!rights.view" role="alert">{{ t("没有此品牌的归档查看权限。", "No archive viewing permission for this brand.") }}</p>
    <template v-else>
      <p v-if="notice" role="status">{{ t(notice) }}</p><p v-if="retryError" role="alert">{{ t(retryError) }}</p>
      <section class="subpanel" aria-labelledby="archive-policy-heading"><h3 id="archive-policy-heading">{{ t("自动归档策略", "Automatic archive policy") }}</h3><p v-if="policyLoading" role="status">{{ t("读取中…", "Loading…") }}</p><p v-if="policyError" role="alert" data-testid="archive-tasks-policy-error">{{ t("策略读取失败：", "Policy read failed: ") }}{{ t(policyError) }}</p>
        <dl v-if="policy" data-testid="archive-tasks-policy"><dt>{{ t("品牌 ID", "Brand ID") }}</dt><dd>{{ policy.brand_id }}</dd><dt>{{ t("策略版本", "Policy version") }}</dt><dd>{{ policy.version }}</dd><dt>{{ t("日归档启用", "Daily enabled") }}</dt><dd>{{ policy.daily_enabled }}</dd><dt>{{ t("月归档启用", "Monthly enabled") }}</dt><dd>{{ policy.monthly_enabled }}</dd><dt>{{ t("日归档起点", "Daily start period") }}</dt><dd>{{ policy.daily_start_period ?? "—" }}</dd><dt>{{ t("月归档起点", "Monthly start period") }}</dt><dd>{{ policy.monthly_start_period ?? "—" }}</dd><dt>{{ t("时区", "Timezone") }}</dt><dd>{{ policy.timezone }}</dd><dt>{{ t("策略审计 ID", "Policy audit ID") }}</dt><dd>{{ policy.audit_log_id ?? "—" }}</dd><dt>{{ t("更新时间", "Updated at") }}</dt><dd>{{ policy.updated_at }}</dd></dl>
        <div class="policy-editor" data-testid="archive-policy-editor">
          <h4>{{ t("编辑归档开关", "Edit archive switches") }}</h4>
          <p>{{ t("首次启用从品牌时区的当前日或当前月开始，在该完整周期结束后归档。关闭后再开启会保留原起点，不会回补历史周期。", "First activation starts with the current day or month in the brand timezone and archives after that complete period ends. Turning a switch off and on retains its saved start and does not backfill earlier periods.") }}</p>
          <p v-if="!policyRights.write">{{ t("当前账号可查看策略，但没有此品牌的策略修改权限。", "This account can view the policy but cannot edit this brand's policy.") }}</p>
          <p v-if="policyIntent" role="status">{{ policyIntent.phase === "unknown" ? t("更新结果未知", "Update outcome unknown") : policyIntent.phase === "conflict" ? t("策略版本冲突", "Policy version conflict") : t("原策略更新已确认", "Original policy update acknowledged") }}</p>
          <p v-if="policyWriteError" role="alert">{{ t(policyWriteError) }}</p>
          <label class="policy-check"><input v-model="policyDaily" data-testid="archive-policy-daily" type="checkbox" :disabled="!isPolicyEditorEnabled" />{{ t("启用日归档", "Enable daily archives") }}</label>
          <label class="policy-check"><input v-model="policyMonthly" data-testid="archive-policy-monthly" type="checkbox" :disabled="!isPolicyEditorEnabled" />{{ t("启用月归档", "Enable monthly archives") }}</label>
          <label class="policy-reason">{{ t("操作原因", "Reason for change") }}<textarea v-model="policyReason" data-testid="archive-policy-reason" :disabled="!isPolicyEditorEnabled" maxlength="500" /></label>
          <button class="button" data-testid="archive-policy-save" :disabled="!isPolicySaveEnabled" @click="beginPolicySave">{{ t("核对并保存策略", "Review and save policy") }}</button>
        </div>
        <section v-if="policyIntent" class="frozen" data-testid="archive-policy-frozen">
          <h4>{{ policyIntent.phase === "unknown" ? t("策略更新结果未知", "Policy update outcome unknown") : policyIntent.phase === "conflict" ? t("策略更新冲突", "Policy update conflicted") : t("策略更新回执", "Policy update receipt") }}</h4>
          <p>{{ t("账号、品牌、版本、两个开关、原因和幂等键冻结在当前页面会话内；切换页面、读取策略或切换语言都不会自动重发。", "The account, brand, version, switches, reason and idempotency key are frozen in page-session memory. Navigation, policy reads and locale changes never replay them automatically.") }}</p>
          <dl><dt>{{ t("管理员", "Actor") }}</dt><dd>{{ policyIntent.actorId }}</dd><dt>{{ t("品牌", "Brand") }}</dt><dd>{{ policyIntent.brandId }}</dd><dt>{{ t("原策略版本", "Original policy version") }}</dt><dd>{{ policyIntent.version }}</dd><dt>{{ t("日归档", "Daily archives") }}</dt><dd>{{ policyIntent.daily_enabled }}</dd><dt>{{ t("月归档", "Monthly archives") }}</dt><dd>{{ policyIntent.monthly_enabled }}</dd><dt>{{ t("操作原因", "Reason") }}</dt><dd>{{ policyIntent.reason }}</dd><dt>{{ t("幂等键", "Idempotency key") }}</dt><dd>{{ policyIntent.key }}</dd></dl>
          <div v-if="policyIntent.phase === 'acknowledged' && policyIntent.receipt" class="policy-receipt" data-testid="archive-policy-receipt">
            <h4>{{ t("原更新请求成功回执（完整策略）", "Original update ACK (complete policy)") }}</h4>
            <dl><dt>{{ t("品牌 ID", "Brand ID") }}</dt><dd>{{ policyIntent.receipt.brand_id }}</dd><dt>{{ t("策略版本", "Policy version") }}</dt><dd>{{ policyIntent.receipt.version }}</dd><dt>{{ t("日归档启用", "Daily enabled") }}</dt><dd>{{ policyIntent.receipt.daily_enabled }}</dd><dt>{{ t("月归档启用", "Monthly enabled") }}</dt><dd>{{ policyIntent.receipt.monthly_enabled }}</dd><dt>{{ t("日归档起点", "Daily start period") }}</dt><dd>{{ policyIntent.receipt.daily_start_period ?? "—" }}</dd><dt>{{ t("月归档起点", "Monthly start period") }}</dt><dd>{{ policyIntent.receipt.monthly_start_period ?? "—" }}</dd><dt>{{ t("时区", "Timezone") }}</dt><dd>{{ policyIntent.receipt.timezone }}</dd><dt>{{ t("策略审计 ID", "Policy audit ID") }}</dt><dd>{{ policyIntent.receipt.audit_log_id ?? "—" }}</dd><dt>{{ t("更新时间", "Updated at") }}</dt><dd>{{ policyIntent.receipt.updated_at }}</dd></dl>
          </div>
          <template v-if="policyIntent.phase === 'unknown'"><label class="policy-check"><input v-model="policyChecked" data-testid="archive-policy-replay-check" type="checkbox" />{{ t("我已核对冻结的原请求，确认使用相同幂等键和全部原字段重放。", "I reviewed the frozen request and confirm replaying every original field with the same idempotency key.") }}</label><button class="button button-secondary" data-testid="archive-policy-replay" :disabled="!policyRights.write || policyWriting || !policyChecked" @click="replayOriginalPolicy">{{ t("按原请求重放", "Replay exact request") }}</button></template>
          <template v-else><button class="button button-secondary" data-testid="archive-policy-reload-frozen" :disabled="policyLoading || policyWriting" @click="reloadPolicyForDiscard()">{{ t("重新读取当前策略", "Reload current policy") }}</button><label class="policy-check"><input v-model="policyChecked" data-testid="archive-policy-discard-check" type="checkbox" :disabled="!policyReloaded" />{{ t("我已核对重新读取的当前策略，确认丢弃冻结请求。", "I reviewed the freshly loaded current policy and confirm discarding the frozen request.") }}</label><button class="button button-secondary" data-testid="archive-policy-discard" :disabled="!policyReloaded || !policyChecked || policyWriting" @click="discardPolicyIntent">{{ t("确认丢弃原请求", "Confirm discard") }}</button></template>
        </section>
      </section>
      <section class="subpanel" aria-labelledby="archive-tasks-list-heading"><h3 id="archive-tasks-list-heading">{{ t("任务列表", "Task list") }}</h3><p>{{ t("总任务数", "Total tasks") }}: {{ total }} · {{ t("当前页", "Current page") }}: {{ offset }}–{{ offset + items.length }}</p><p>{{ t("下方状态只描述当前页，不能据此推断其他页或全局状态。", "Statuses below describe this page only and do not imply the status of other pages or the overall system.") }}</p><p v-if="listLoading" role="status">{{ t("读取中…", "Loading…") }}</p><p v-if="listError" role="alert" data-testid="archive-tasks-list-error">{{ t("任务列表读取失败：", "Task list read failed: ") }}{{ t(listError) }}</p><p v-if="!listLoading && items.length === 0">{{ t("当前页没有任务。", "No tasks on this page.") }}</p><ul class="task-list" data-testid="archive-tasks-list" :aria-label="t('自动归档任务列表', 'Automatic archive task list')"><li v-for="item in items" :key="item.id"><button class="button button-secondary" :aria-label="`${windowKindLabel(item.window.kind)} ${item.window.kind} · ${item.window.period_key} · ${item.state} · v${item.version} · ${item.id}`" :disabled="detailLoading" @click="loadTask(item.id)">{{ windowKindLabel(item.window.kind) }} ({{ item.window.kind }}) · {{ item.window.period_key }} · {{ item.state }} · v{{ item.version }}<small>{{ item.id }}</small></button><small>{{ item.updated_at }}</small></li></ul><div class="actions"><button class="button button-secondary" :disabled="listLoading || offset === 0" @click="loadList(Math.max(0, offset - pageSize))">{{ t("上一页", "Previous page") }}</button><button class="button button-secondary" :disabled="listLoading || !nextPage" @click="loadList(offset + pageSize)">{{ t("下一页", "Next page") }}</button></div></section>
      <section v-if="selected || detailError" class="subpanel" data-testid="archive-tasks-detail"><h3>{{ t("任务详情", "Task detail") }}</h3><p v-if="detailLoading" role="status">{{ t("读取任务中…", "Loading task…") }}</p><p v-if="detailError" role="alert" data-testid="archive-tasks-detail-error">{{ t("任务详情读取失败：", "Task detail read failed: ") }}{{ t(detailError) }}</p><dl v-if="selected"><dt>ID</dt><dd>{{ selected.id }}</dd><dt>{{ t("品牌 ID", "Brand ID") }}</dt><dd>{{ selected.brand_id }}</dd><dt>{{ t("策略版本", "Policy version") }}</dt><dd>{{ selected.policy_version }}</dd><dt>{{ t("时间窗口类型", "Window kind") }}</dt><dd>{{ windowKindLabel(selected.window.kind) }} ({{ selected.window.kind }})</dd><dt>{{ t("周期", "Period") }}</dt><dd>{{ selected.window.period_key }}</dd><dt>{{ t("时区", "Timezone") }}</dt><dd>{{ selected.window.timezone }}</dd><dt>{{ t("起始时间", "From") }}</dt><dd>{{ selected.window.from }}</dd><dt>{{ t("结束时间", "To") }}</dt><dd>{{ selected.window.to }}</dd><dt>{{ t("状态", "State") }}</dt><dd>{{ selected.state }}</dd><dt>{{ t("任务版本", "Task version") }}</dt><dd>{{ selected.version }}</dd><dt>{{ t("尝试次数", "Attempt count") }}</dt><dd>{{ selected.attempt_count }}</dd><dt>{{ t("归档 ID", "Archive ID") }}</dt><dd>{{ selected.archive_id ?? "—" }}</dd><dt>{{ t("最后错误码", "Last error code") }}</dt><dd>{{ selected.last_error_code ?? "—" }}</dd><dt>{{ t("创建审计 ID", "Creation audit ID") }}</dt><dd>{{ selected.creation_audit_log_id }}</dd><dt>{{ t("最近审计 ID", "Last audit ID") }}</dt><dd>{{ selected.last_audit_log_id }}</dd><dt>{{ t("创建时间", "Created at") }}</dt><dd>{{ selected.created_at }}</dd><dt>{{ t("更新时间", "Updated at") }}</dt><dd>{{ selected.updated_at }}</dd></dl>
        <div v-if="selected?.state === 'failed'" class="retry-form"><h4>{{ t("人工重试失败任务", "Manually retry failed task") }}</h4><label>{{ t("操作原因", "Reason for operation") }}<textarea v-model="reason" data-testid="archive-tasks-reason" :disabled="!rights.retry || !!pending || writing" /></label><p v-if="!rights.retry">{{ t("当前账号只读，不能重试。", "This account is read-only and cannot retry tasks.") }}</p><p v-if="rights.retry && !props.account.super_admin">{{ t("重试将使用当前任务版本；提交前请核对品牌、任务、版本和原因。", "Retry uses the current task version. Review the brand, task, version and reason before submitting.") }}</p><button class="button" data-testid="archive-tasks-retry" :disabled="!isRetryEnabled" @click="beginRetry">{{ t("核对并重试", "Review and retry") }}</button></div>
      </section>
      <section v-if="pending" class="frozen" data-testid="archive-tasks-frozen"><h3>{{ pending.phase === "unknown" ? t("重试结果未知", "Retry outcome unknown") : pending.phase === "conflict" ? t("原重试请求冲突", "Original retry request conflicted") : t("原重试已确认", "Original retry acknowledged") }}</h3><p>{{ t("账号、品牌、任务、版本、原因和原幂等键仅保存在当前会话内。离页、刷新或语言切换不会清除它，也不会自动重发。", "The account, brand, task, version, reason and original idempotency key stay in current-session memory. Leaving, refreshing or changing language does not clear or automatically replay it.") }}</p><dl><dt>{{ t("管理员", "Actor") }}</dt><dd>{{ pending.actorId }}</dd><dt>{{ t("品牌", "Brand") }}</dt><dd>{{ pending.brandId }}</dd><dt>{{ t("任务", "Task") }}</dt><dd>{{ pending.taskId }}</dd><dt>{{ t("原任务版本", "Original task version") }}</dt><dd>{{ pending.version }}</dd><dt>{{ t("操作原因", "Reason") }}</dt><dd>{{ pending.reason }}</dd><dt>{{ t("原幂等键", "Original idempotency key") }}</dt><dd>{{ pending.key }}</dd></dl>
        <div v-if="pending.phase === 'acknowledged' && pending.receipt" class="ack-receipt" data-testid="archive-tasks-receipt"><div data-testid="archive-tasks-ack-receipt"><h4>{{ t("原重试请求回执", "Original retry ACK receipt") }}</h4><dl><dt>ID</dt><dd>{{ pending.receipt.id }}</dd><dt>{{ t("品牌 ID", "Brand ID") }}</dt><dd>{{ pending.receipt.brand_id }}</dd><dt>{{ t("策略版本", "Policy version") }}</dt><dd>{{ pending.receipt.policy_version }}</dd><dt>{{ t("窗口类型", "Window kind") }}</dt><dd>{{ windowKindLabel(pending.receipt.window.kind) }} ({{ pending.receipt.window.kind }})</dd><dt>{{ t("周期", "Period") }}</dt><dd>{{ pending.receipt.window.period_key }}</dd><dt>{{ t("时区", "Timezone") }}</dt><dd>{{ pending.receipt.window.timezone }}</dd><dt>{{ t("起始时间", "From") }}</dt><dd>{{ pending.receipt.window.from }}</dd><dt>{{ t("结束时间", "To") }}</dt><dd>{{ pending.receipt.window.to }}</dd><dt>{{ t("回执状态", "Receipt state") }}</dt><dd>{{ pending.receipt.state }}</dd><dt>{{ t("回执版本", "Receipt version") }}</dt><dd>{{ pending.receipt.version }}</dd><dt>{{ t("回执尝试次数", "Receipt attempt count") }}</dt><dd>{{ pending.receipt.attempt_count }}</dd><dt>{{ t("回执归档 ID", "Receipt archive ID") }}</dt><dd>{{ pending.receipt.archive_id ?? "—" }}</dd><dt>{{ t("回执错误码", "Receipt error code") }}</dt><dd>{{ pending.receipt.last_error_code ?? "—" }}</dd><dt>{{ t("创建审计 ID", "Creation audit ID") }}</dt><dd>{{ pending.receipt.creation_audit_log_id }}</dd><dt>{{ t("最近审计 ID", "Last audit ID") }}</dt><dd>{{ pending.receipt.last_audit_log_id }}</dd><dt>{{ t("创建时间", "Created at") }}</dt><dd>{{ pending.receipt.created_at }}</dd><dt>{{ t("更新时间", "Receipt updated at") }}</dt><dd>{{ pending.receipt.updated_at }}</dd></dl><p>{{ t("这是原 retry API 的成功回执；当前任务查询显示的是 worker 后续状态，两者分别保存。", "This is the successful receipt from the original retry API. The current task query shows a later worker state; they are stored separately.") }}</p></div></div>
        <template v-if="pending.phase === 'unknown'"><label class="replay-confirm"><input v-model="checked" data-testid="archive-tasks-replay-check" type="checkbox" />{{ t("我已核对下方冻结的原请求，确认使用同一账号、品牌、任务、版本、原因和幂等键重放。", "I reviewed the frozen original request below and confirm replaying it with the same account, brand, task, version, reason and idempotency key.") }}</label><button class="button button-secondary" data-testid="archive-tasks-replay" :disabled="!rights.retry || writing || !checked" @click="replayOriginal">{{ t("按原请求重放", "Replay exact request") }}</button></template>
        <template v-else><button class="button button-secondary" data-testid="archive-tasks-reload-frozen" :disabled="listLoading || detailLoading" @click="refreshFrozen()">{{ t("重新读取列表和原任务", "Reload list and original task") }}</button><label><input v-model="checked" data-testid="archive-tasks-discard-check" type="checkbox" :disabled="!reloaded" />{{ t("我已核对重新读取的列表和原任务，确认丢弃原请求。", "I reviewed the reloaded list and original task and confirm discarding the original request.") }}</label><button class="button button-secondary" data-testid="archive-tasks-discard" :disabled="!reloaded || !checked || writing" @click="discardFrozen">{{ t("確認丢弃原请求", "Confirm discard of original request") }}</button></template>
      </section>
      <p v-if="rights.view && !rights.retry">{{ t("当前账号可查看策略和任务；手动重试权限未授予。", "This account can view policy and tasks; manual retry permission is not granted.") }}</p>
    </template>
    <div v-if="policyReview" class="review" role="dialog" aria-modal="true" :aria-label="t('确认策略更新', 'Confirm policy update')" data-testid="archive-policy-review"><div><h3>{{ t("确认策略更新", "Confirm policy update") }}</h3><dl><dt>{{ t("管理员", "Actor") }}</dt><dd>{{ policyReview.actorId }}</dd><dt>{{ t("品牌", "Brand") }}</dt><dd>{{ policyReview.brandId }}</dd><dt>{{ t("原策略版本", "Policy version") }}</dt><dd>{{ policyReview.version }}</dd><dt>{{ t("日归档", "Daily archives") }}</dt><dd>{{ policyReview.daily_enabled }}</dd><dt>{{ t("月归档", "Monthly archives") }}</dt><dd>{{ policyReview.monthly_enabled }}</dd><dt>{{ t("操作原因", "Reason") }}</dt><dd>{{ policyReview.reason }}</dd><dt>{{ t("幂等键", "Idempotency key") }}</dt><dd>{{ policyReview.key }}</dd></dl><h4>{{ t("核对时的完整当前策略", "Full current policy at review") }}</h4><dl v-if="policyReviewCurrent"><dt>{{ t("品牌 ID", "Brand ID") }}</dt><dd>{{ policyReviewCurrent.brand_id }}</dd><dt>{{ t("策略版本", "Policy version") }}</dt><dd>{{ policyReviewCurrent.version }}</dd><dt>{{ t("日归档启用", "Daily enabled") }}</dt><dd>{{ policyReviewCurrent.daily_enabled }}</dd><dt>{{ t("月归档启用", "Monthly enabled") }}</dt><dd>{{ policyReviewCurrent.monthly_enabled }}</dd><dt>{{ t("日归档起点", "Daily start period") }}</dt><dd>{{ policyReviewCurrent.daily_start_period ?? "—" }}</dd><dt>{{ t("月归档起点", "Monthly start period") }}</dt><dd>{{ policyReviewCurrent.monthly_start_period ?? "—" }}</dd><dt>{{ t("时区", "Timezone") }}</dt><dd>{{ policyReviewCurrent.timezone }}</dd><dt>{{ t("策略审计 ID", "Policy audit ID") }}</dt><dd>{{ policyReviewCurrent.audit_log_id ?? "—" }}</dd><dt>{{ t("更新时间", "Updated at") }}</dt><dd>{{ policyReviewCurrent.updated_at }}</dd></dl><label><input v-model="policyConfirmed" data-testid="archive-policy-confirm" type="checkbox" />{{ t("我已核对管理员、品牌、版本、日/月开关和原因，确认提交此策略更新。", "I reviewed the administrator, brand, version, daily and monthly switches, and reason, and confirm this policy update.") }}</label><div class="actions"><button class="button button-secondary" @click="policyReview = null; policyReviewCurrent = null; policyConfirmed = false">{{ t("取消", "Cancel") }}</button><button class="button" data-testid="archive-policy-submit" :disabled="!policyConfirmed || !policyRights.write || policyWriting" @click="submitReviewedPolicy">{{ t("确认并保存", "Confirm and save") }}</button></div></div></div>
    <div v-if="review" class="review" role="dialog" aria-modal="true" :aria-label="t('确认重试请求', 'Confirm retry request')" data-testid="archive-tasks-review"><div><h3>{{ t("确认重试请求", "Confirm retry request") }}</h3><dl><dt>{{ t("管理员", "Actor") }}</dt><dd>{{ review.actorId }}</dd><dt>{{ t("品牌", "Brand") }}</dt><dd>{{ review.brandId }}</dd><dt>{{ t("任务", "Task") }}</dt><dd>{{ review.taskId }}</dd><dt>{{ t("原任务版本", "Task version") }}</dt><dd>{{ review.version }}</dd><dt>{{ t("操作原因", "Reason") }}</dt><dd>{{ review.reason }}</dd><dt>{{ t("幂等键", "Idempotency key") }}</dt><dd>{{ review.key }}</dd></dl><label><input v-model="confirmed" data-testid="archive-tasks-confirm" type="checkbox" />{{ t("我已核对管理员、品牌、失败任务、当前版本和原因，确认提交此重试。", "I reviewed the administrator, brand, failed task, current version and reason, and confirm this retry.") }}</label><div class="actions"><button class="button button-secondary" @click="review = null; confirmed = false">{{ t("取消", "Cancel") }}</button><button class="button" data-testid="archive-tasks-submit" :disabled="!confirmed || !rights.retry || writing" @click="submitReviewedRetry">{{ t("确认并重试", "Confirm and retry") }}</button></div></div></div>
  </section>
</template>

<style scoped>
.archive-tasks{min-width:0;overflow-wrap:anywhere;padding:1.25rem}.archive-tasks header,.actions{display:flex;justify-content:space-between;align-items:flex-start;gap:.75rem;flex-wrap:wrap}.archive-tasks p{line-height:1.55;color:var(--muted,#64748b)}.subpanel,.frozen{margin:1rem 0;padding:1rem;border:1px solid var(--border,#d5dce2);border-radius:.75rem;min-width:0}.frozen{background:#fbf7ed;border-color:#d5b05b}.archive-tasks dl{display:grid;grid-template-columns:minmax(140px,1fr) minmax(0,2fr);gap:.5rem 1rem}.archive-tasks dt{color:var(--muted,#64748b)}.archive-tasks dd{margin:0;overflow-wrap:anywhere;white-space:pre-wrap}.task-list{list-style:none;padding:0;display:grid;gap:.5rem}.task-list li{display:flex;justify-content:space-between;align-items:center;gap:.75rem;padding:.6rem;border:1px solid var(--border,#d5dce2);border-radius:.6rem}.task-list small{display:block;overflow-wrap:anywhere}.retry-form,.policy-editor{display:grid;gap:.75rem;margin-top:1rem}.retry-form label,.policy-reason{display:grid;gap:.5rem}.retry-form textarea,.policy-reason textarea{box-sizing:border-box;width:100%;min-height:5rem;padding:.65rem;border:1px solid var(--border,#d5dce2);border-radius:.5rem;background:var(--surface,#fff);color:inherit}.policy-check{display:flex;align-items:flex-start;gap:.6rem}.policy-check input,.frozen input[type=checkbox]{width:auto;flex:none}.policy-receipt{margin-top:1rem;padding:.75rem;border:1px solid var(--border,#d5dce2);border-radius:.6rem}.review{position:fixed;inset:0;z-index:90;display:flex;align-items:center;justify-content:center;padding:1rem;background:#17253699}.review>div{box-sizing:border-box;width:38rem;max-width:100%;max-height:85dvh;overflow:auto;padding:1.25rem;border-radius:1rem;background:var(--surface,#fff)}.review label{display:flex;align-items:flex-start;gap:.6rem;min-width:0}.review input{flex:none}.review dd{min-width:0;overflow-wrap:anywhere;word-break:break-word}@media(max-width:700px){.archive-tasks{padding:1rem}.archive-tasks dl{grid-template-columns:minmax(0,1fr)}.task-list li{align-items:flex-start;flex-direction:column}}@media(max-width:380px){.review{padding:.5rem}.review>div{width:100%;padding:.8rem}.review dl{grid-template-columns:minmax(0,1fr);gap:.2rem}.review dd{margin-bottom:.45rem}.review .actions{justify-content:flex-start}.policy-check{align-items:flex-start}.policy-editor>.button,.frozen>.button{max-width:100%;white-space:normal;overflow-wrap:anywhere}}
.task-list li{min-width:0}.task-list button{display:block;min-width:0;max-width:100%;width:100%;white-space:normal;overflow-wrap:anywhere;text-align:left}
</style>
