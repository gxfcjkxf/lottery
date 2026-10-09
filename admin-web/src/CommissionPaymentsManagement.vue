<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { useAdminI18n } from "./i18n";
import CommissionAdjustmentsManagement from "./CommissionAdjustmentsManagement.vue";
import {
  commissionPaymentPermissions, createCommissionPaymentsApi,
  type CommissionPayment, type CommissionPaymentPage, type CommissionPaymentPolicy,
} from "./commissionPayments-api";
import {
  classifyCommissionPaymentWriteFailure, clearAllCommissionPaymentIntents,
  clearPendingCommissionPaymentIntent, commissionPaymentSessionGeneration,
  createCommissionPaymentIntent, freezeCommissionPaymentBody, isCommissionPaymentIntentConflict,
  getPendingCommissionPaymentIntent, hideCommissionPaymentIntentsForScope, listPendingCommissionPaymentIntents, markCommissionPaymentIntentConflict, setPendingCommissionPaymentIntent,
  type CommissionPaymentIntentScope, type PendingCommissionPaymentIntent,
} from "./commissionPayments-state";

const props = defineProps<{ account: AdminAccount; brandId: string; brandStatus?: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, locale } = useAdminI18n();
const api = createCommissionPaymentsApi(fetch, props.account.id);
const PAGE_SIZE = 20;
const rights = computed(() => commissionPaymentPermissions(props.account, props.brandId));
const scopeStamp = computed(() => JSON.stringify([props.account.id, props.account.super_admin, props.account.brand_ids,
  props.account.permissions, props.account.permissions_by_brand, props.account.platform_permissions, props.brandId, props.brandStatus, rights.value]));
const policy = ref<CommissionPaymentPolicy | null>(null), page = ref<CommissionPaymentPage | null>(null);
const selected = ref<CommissionPayment | null>(null), detail = ref<CommissionPayment | null>(null);
const receipt = ref<{ operation: string; payment?: CommissionPayment; policy?: CommissionPaymentPolicy } | null>(null);
const loadingPolicy = ref(false), loadingList = ref(false), loadingDetail = ref(false), writing = ref(false);
const policyError = ref(""), listError = ref(""), detailError = ref(""), writeError = ref(""), notice = ref("");
const reason = ref(""), dialog = ref<PendingCommissionPaymentIntent | null>(null), confirmed = ref(false);
const unknownIntent = ref<PendingCommissionPaymentIntent | null>(null), conflictIntent = ref<PendingCommissionPaymentIntent | null>(null);
const conflict = ref(false), conflictReloading = ref(false), conflictRefreshed = ref(false), discardReviewed = ref(false);
let alive = true, generation = 0, policyTicket = 0, listTicket = 0, detailTicket = 0, paymentReadTicket = 0, writeTicket = 0;
let conflictTicket = 0;

const offset = computed(() => page.value?.offset ?? 0);
const totalCount = computed(() => page.value?.total_count ?? "0");
const hasPrevious = computed(() => offset.value > 0);
const hasNext = computed(() => page.value !== null && BigInt(page.value.offset + page.value.limit) < BigInt(page.value.total_count));
const policyCanSave = computed(() => rights.value.policyWrite && props.brandStatus !== "disabled" && !writing.value && !loadingPolicy.value && !policyError.value && !unknownIntent.value && !conflict.value && Boolean(policy.value) && policy.value!.version < Number.MAX_SAFE_INTEGER && validReason(reason.value));
const policyEnabled = ref(false);
const canApprove = computed(() => Boolean(rights.value.approve && props.brandStatus !== "disabled" && !props.account.super_admin && !writing.value && !loadingDetail.value && !detailError.value && !unknownIntent.value && !conflict.value && detail.value && detail.value.state === "awaiting_approval" && (detail.value.payout_mode === "manual" || detail.value.payout_mode === "mixed") && detail.value.version < Number.MAX_SAFE_INTEGER && validReason(reason.value)));
const canRetry = computed(() => Boolean(rights.value.retry && props.brandStatus !== "disabled" && !props.account.super_admin && !writing.value && !loadingDetail.value && !detailError.value && !unknownIntent.value && !conflict.value && detail.value?.state === "failed" && detail.value.version < Number.MAX_SAFE_INTEGER && validReason(reason.value)));

function validReason(value: string): boolean {
  if (!value || value.trim() !== value || new TextEncoder().encode(value).length > 500 || /[\u0000-\u001f\u007f-\u009f]/u.test(value)) return false;
  for (let i = 0; i < value.length; i++) { const code = value.charCodeAt(i); if (code >= 0xd800 && code <= 0xdbff) { const next = value.charCodeAt(++i); if (!(next >= 0xdc00 && next <= 0xdfff)) return false; } else if (code >= 0xdc00 && code <= 0xdfff) return false; }
  return true;
}
function paymentScope(operation: "approve" | "retry", id: string): CommissionPaymentIntentScope { return { accountId: props.account.id, brandId: props.brandId, operation, targetId: id }; }
function current(ticket: number, lane: "policy" | "list" | "detail", captured: string, session: number, commonReadTicket?: number): boolean {
  const laneTicket = lane === "policy" ? policyTicket : lane === "list" ? listTicket : detailTicket;
  return alive && ticket === laneTicket && (lane === "policy" || commonReadTicket === paymentReadTicket) && captured === scopeStamp.value && session === commissionPaymentSessionGeneration() && rights.value.view;
}
function laneCurrent(ticket: number, lane: "policy" | "list" | "detail", captured: string, session: number): boolean {
  const laneTicket = lane === "policy" ? policyTicket : lane === "list" ? listTicket : detailTicket;
  return alive && ticket === laneTicket && captured === scopeStamp.value && session === commissionPaymentSessionGeneration() && rights.value.view;
}
function currentWrite(ticket: number, captured: string, session: number): boolean { return alive && ticket === writeTicket && captured === scopeStamp.value && session === commissionPaymentSessionGeneration(); }
function clearVisible() {
  policy.value = null; page.value = null; selected.value = null; detail.value = null; receipt.value = null;
  reason.value = ""; dialog.value = null; confirmed.value = false; unknownIntent.value = null; conflictIntent.value = null;
  conflict.value = false; conflictRefreshed.value = false; discardReviewed.value = false;
  policyError.value = listError.value = detailError.value = writeError.value = notice.value = "";
  loadingPolicy.value = loadingList.value = loadingDetail.value = writing.value = false;
}
function resetScope(_next?: string, previous?: string) {
  if (previous) {
    try { const previousScope = JSON.parse(previous) as unknown[]; hideCommissionPaymentIntentsForScope(String(previousScope[0]), String(previousScope[6])); }
    catch { /* Invalid old scope cannot authorize or expose a pending intent. */ }
  }
  generation++; policyTicket++; listTicket++; detailTicket++; paymentReadTicket++; writeTicket++; conflictTicket++;
  clearVisible();
  const pending = listPendingCommissionPaymentIntents(props.account.id, props.brandId);
  const conflictedIntent = pending.find(isCommissionPaymentIntentConflict);
  conflictIntent.value = conflictedIntent ?? null; conflict.value = Boolean(conflictedIntent);
  unknownIntent.value = conflictedIntent ? null : pending[0] ?? null;
  if (unknownIntent.value) writeError.value = t("有一个同会话待确认请求。只能用原正文和原幂等键重试。", "A pending request from this session needs resolution. Retry only with its original body and key.");
  if (rights.value.view) {
    const captured = scopeStamp.value, session = commissionPaymentSessionGeneration(), target = unknownIntent.value?.targetId;
    const initial = Promise.all([loadPolicy(), loadList(0)]), initialRead = paymentReadTicket;
    void initial.then(() => { if (alive && rights.value.view && captured === scopeStamp.value && session === commissionPaymentSessionGeneration() && initialRead === paymentReadTicket && target) void loadDetail(target); });
  }
}
function sessionExpired(cause: unknown, captured: string, session: number): boolean {
  if (cause instanceof AdminApiError && cause.status === 401 && alive && captured === scopeStamp.value && session === commissionPaymentSessionGeneration()) {
    generation++; policyTicket++; listTicket++; detailTicket++; paymentReadTicket++; writeTicket++; clearAllCommissionPaymentIntents(); clearVisible(); emit("session-invalid"); return true;
  }
  return false;
}
function readFailure(cause: unknown): string { return cause instanceof AdminApiError && cause.status === 0 ? t("网络或响应不可用，当前数据尚未读取。", "The network or response is unavailable; current data was not loaded.") : cause instanceof Error ? cause.message : t("读取失败。", "Unable to load data."); }
function isCurrent(payment: CommissionPayment | null): boolean { return Boolean(payment && detail.value?.id === payment.id && detail.value.version === payment.version); }

function cancelConflictReview() {
  if (!conflict.value) return;
  conflictTicket++; conflictReloading.value = false; conflictRefreshed.value = false; discardReviewed.value = false;
}
async function loadPolicy(preserveConflictReview = false) {
  if (!preserveConflictReview) cancelConflictReview();
  if (!rights.value.view) return;
  const ticket = ++policyTicket, captured = scopeStamp.value, session = commissionPaymentSessionGeneration();
  loadingPolicy.value = true; policyError.value = "";
  try {
    const fresh = await api.getPolicy(props.brandId);
    if (!current(ticket, "policy", captured, session)) return;
    policy.value = fresh; policyEnabled.value = fresh.enabled;
  } catch (cause) { if (current(ticket, "policy", captured, session)) { sessionExpired(cause, captured, session); policyError.value = readFailure(cause); } }
  finally { if (current(ticket, "policy", captured, session)) loadingPolicy.value = false; }
}
async function loadList(nextOffset = offset.value, preserveConflictReview = false) {
  if (!preserveConflictReview) cancelConflictReview();
  if (!rights.value.view) return;
  const readTicket = ++paymentReadTicket;
  const ticket = ++listTicket, captured = scopeStamp.value, session = commissionPaymentSessionGeneration();
  loadingList.value = true; listError.value = "";
  try {
    const fresh = await api.list(props.brandId, PAGE_SIZE, nextOffset);
    if (!current(ticket, "list", captured, session, readTicket)) return;
    page.value = fresh;
    if (selected.value) {
      const listed = fresh.items.find((item) => item.id === selected.value?.id);
      if (!listed) { selected.value = null; detail.value = null; detailTicket++; loadingDetail.value = false; }
      else if (listed.version !== detail.value?.version || listed.state !== detail.value?.state || listed.evidence_epoch !== detail.value?.evidence_epoch || listed.paid_count !== detail.value?.paid_count || listed.paid_points !== detail.value?.paid_points) void loadDetail(listed.id, true);
      else { selected.value = listed; detail.value = listed; }
    }
  } catch (cause) { if (current(ticket, "list", captured, session, readTicket)) { sessionExpired(cause, captured, session); listError.value = readFailure(cause); } }
  finally { if (laneCurrent(ticket, "list", captured, session)) loadingList.value = false; }
}
async function loadDetail(id: string, preserveConflictReview = false) {
  if (!preserveConflictReview) cancelConflictReview();
  const readTicket = ++paymentReadTicket;
  const ticket = ++detailTicket, captured = scopeStamp.value, session = commissionPaymentSessionGeneration();
  loadingDetail.value = true; detailError.value = "";
  selected.value = page.value?.items.find((item) => item.id === id) ?? selected.value;
  if (detail.value?.id !== id) { reason.value = ""; detail.value = null; }
  try {
    const fresh = await api.read(props.brandId, id);
    if (!current(ticket, "detail", captured, session, readTicket) || fresh.id !== id) return;
    selected.value = fresh; detail.value = fresh;
  } catch (cause) { if (current(ticket, "detail", captured, session, readTicket)) { sessionExpired(cause, captured, session); detailError.value = readFailure(cause); } }
  finally { if (laneCurrent(ticket, "detail", captured, session)) loadingDetail.value = false; }
}
async function reloadAll(preserveConflictReview = false) {
  await Promise.all([loadPolicy(preserveConflictReview), loadList(offset.value, preserveConflictReview)]);
  if (selected.value) await loadDetail(selected.value.id, preserveConflictReview);
}

function intentFor(operation: CommissionPaymentIntentScope["operation"], targetId?: string): PendingCommissionPaymentIntent | null {
  const scope: CommissionPaymentIntentScope = { accountId: props.account.id, brandId: props.brandId, operation, ...(operation !== "policy" && targetId ? { targetId } : {}) };
  const old = getPendingCommissionPaymentIntent(scope); if (old) return old;
  let body;
  if (operation === "policy") {
    if (!policyCanSave.value || !policy.value) return null;
    body = freezeCommissionPaymentBody("policy", { version: policy.value.version, enabled: policyEnabled.value, reason: reason.value });
  } else {
    const row = detail.value;
    if (!row || row.id !== targetId || !validReason(reason.value) || row.version >= Number.MAX_SAFE_INTEGER || writing.value || !rights.value.view || props.account.super_admin || loadingDetail.value) return null;
    if (operation === "approve" && !canApprove.value) return null;
    if (operation === "retry" && !canRetry.value) return null;
    body = freezeCommissionPaymentBody(operation, { version: row.version, reason: reason.value });
  }
  return createCommissionPaymentIntent(scope, body, createIdempotencyKey());
}
function openReview(operation: CommissionPaymentIntentScope["operation"]) {
  if (conflict.value || unknownIntent.value) return;
  const intent = intentFor(operation, detail.value?.id);
  if (!intent) return;
  dialog.value = intent; confirmed.value = false; writeError.value = "";
}
function pendingIntent(): PendingCommissionPaymentIntent | null {
  if (unknownIntent.value) return unknownIntent.value;
  return dialog.value;
}
async function submit(intent: PendingCommissionPaymentIntent, recovery = Boolean(unknownIntent.value && unknownIntent.value.key === intent.key)) {
  if (writing.value || !confirmed.value || !rights.value.view || props.brandStatus === "disabled" || !validIntentScope(intent)) return;
  if ((intent.operation === "policy" && !rights.value.policyWrite) || (intent.operation === "approve" && !rights.value.approve) || (intent.operation === "retry" && !rights.value.retry)) return;
  if (!recovery && intent.operation !== "policy" && (!detail.value || detail.value.id !== intent.targetId || detail.value.version !== intent.body.version || loadingDetail.value)) return;
  const captured = scopeStamp.value, session = commissionPaymentSessionGeneration(), ticket = ++writeTicket;
  writing.value = true; writeError.value = ""; notice.value = ""; setPendingCommissionPaymentIntent(intent, intent);
  try {
    let result: CommissionPayment | CommissionPaymentPolicy;
    if (intent.operation === "policy") result = await api.updatePolicy(intent.brandId, intent.body as Readonly<{ version: number; enabled: boolean; reason: string }>, intent.key);
    else if (intent.operation === "approve") result = await api.approve(intent.brandId, intent.targetId!, intent.body as Readonly<{ version: number; reason: string }>, intent.key);
    else result = await api.retry(intent.brandId, intent.targetId!, intent.body as Readonly<{ version: number; reason: string }>, intent.key);
    clearPendingCommissionPaymentIntent(intent, intent.key);
    if (!currentWrite(ticket, captured, session)) return;
    receipt.value = intent.operation === "policy" ? { operation: intent.operation, policy: result as CommissionPaymentPolicy } : { operation: intent.operation, payment: result as CommissionPayment };
    dialog.value = null; unknownIntent.value = null; confirmed.value = false;
    notice.value = t("服务器已确认操作回执；回执不能替代当前状态查询。", "The server confirmed the operation receipt; it does not replace a current-state query.");
    if (intent.operation === "policy") await loadPolicy();
    else { await loadList(offset.value); if (currentWrite(ticket, captured, session)) await loadDetail(intent.targetId!); }
  } catch (cause) {
    const status = cause instanceof AdminApiError ? cause.status : 0, outcome = classifyCommissionPaymentWriteFailure(status);
    if (!currentWrite(ticket, captured, session)) { if (outcome !== "unknown") clearPendingCommissionPaymentIntent(intent, intent.key); return; }
    if (sessionExpired(cause, captured, session)) { clearPendingCommissionPaymentIntent(intent, intent.key); return; }
    if (outcome === "unknown") {
      setPendingCommissionPaymentIntent(intent, intent); unknownIntent.value = intent; dialog.value = null;
      writeError.value = t("提交结果未知。原正文和幂等键已冻结；请使用同一请求重试，不能编辑。", "The outcome is unknown. The original body and idempotency key are frozen; retry the same request without edits.");
    } else if (outcome === "conflict") {
      markCommissionPaymentIntentConflict(intent); unknownIntent.value = null; conflictIntent.value = intent; dialog.value = null; conflict.value = true; conflictRefreshed.value = false; discardReviewed.value = false;
      writeError.value = t("服务器返回 409。请刷新当前策略/记录并重新核对，再以新幂等键重新审核；原请求不会重放。", "The server returned 409. Refresh the policy/record and review it again with a new idempotency key. The old request will not be replayed.");
    } else {
      clearPendingCommissionPaymentIntent(intent, intent.key); unknownIntent.value = null; dialog.value = null;
      writeError.value = cause instanceof Error ? cause.message : t("提交失败。", "Submission failed.");
    }
  } finally { if (currentWrite(ticket, captured, session)) writing.value = false; }
}
function validIntentScope(intent: PendingCommissionPaymentIntent): boolean { return intent.accountId === props.account.id && intent.brandId === props.brandId && (intent.operation === "policy" ? intent.targetId === undefined : typeof intent.targetId === "string"); }
async function reloadConflict() {
  if (!conflict.value || conflictReloading.value) return;
  const intent = conflictIntent.value;
  if (!intent) return;
  const ticket = ++conflictTicket, captured = scopeStamp.value;
  conflictReloading.value = true;
  policyError.value = listError.value = detailError.value = "";
  await Promise.all([loadPolicy(true), loadList(offset.value, true)]);
  if (intent.targetId) await loadDetail(intent.targetId, true);
  if (alive && ticket === conflictTicket && captured === scopeStamp.value && rights.value.view && policy.value && page.value && !policyError.value && !listError.value && (!intent.targetId || (detail.value?.id === intent.targetId && !detailError.value))) {
    conflictRefreshed.value = true; discardReviewed.value = false;
    writeError.value = t("状态已刷新。请核对新状态；勾选下方确认后才会丢弃 409 原请求并允许创建新幂等键。", "Current state is refreshed. Review it, then explicitly confirm discarding the original 409 request before creating a new key.");
  }
  if (ticket === conflictTicket) conflictReloading.value = false;
}
function discardConflictIntent() {
  const intent = conflictIntent.value;
  if (!intent || !conflictRefreshed.value || !discardReviewed.value || conflictReloading.value) return;
  clearPendingCommissionPaymentIntent(intent, intent.key); conflictIntent.value = null; conflict.value = false;
  conflictRefreshed.value = false; discardReviewed.value = false; writeError.value = "";
  notice.value = t("已确认丢弃旧请求。请使用当前版本重新审核，新请求将使用新幂等键。", "The old request was explicitly discarded. Review the current version; a new request will use a new key.");
}
function retryUnknown() { const intent = unknownIntent.value; if (!intent || writing.value || !rights.value.view || props.brandStatus === "disabled" || !validIntentScope(intent)) return; dialog.value = intent; confirmed.value = false; }
function formatDate(value: string): string { const date = new Date(value); return Number.isFinite(date.getTime()) ? `${new Intl.DateTimeFormat(locale.value === "en" ? "en" : "zh-CN", { dateStyle: "medium", timeStyle: "medium", timeZone: "UTC" }).format(date)} UTC` : value; }
function stateLabel(state: CommissionPayment["state"]): string {
  const names: Record<CommissionPayment["state"], [string, string]> = { awaiting_approval: ["待审批", "Awaiting approval"], paying: ["派发中", "Paying"], paid: ["已入账", "Paid"], failed: ["失败", "Failed"], stale: ["证据已过期", "Stale evidence"], blocked: ["已阻止", "Blocked"] };
  return t(...names[state]);
}
function modeLabel(mode: CommissionPayment["payout_mode"]): string {
  const names: Record<CommissionPayment["payout_mode"], [string, string]> = { manual: ["人工", "Manual"], automatic: ["自动", "Automatic"], mixed: ["混合（整周期人工审核）", "Mixed (whole-cycle manual review)"], none: ["无", "None"] };
  return t(...names[mode]);
}
function displayReceipt(): string { return receipt.value?.policy ? `${t("策略版本", "Policy version")} ${receipt.value.policy.version}` : receipt.value?.payment ? `${stateLabel(receipt.value.payment.state)} · v${receipt.value.payment.version}` : ""; }

watch(scopeStamp, resetScope, { immediate: true });
onUnmounted(() => { alive = false; generation++; policyTicket++; listTicket++; detailTicket++; writeTicket++; conflictTicket++; });
</script>

<template>
  <article class="panel commission-payments">
    <header class="cp-heading"><div><p class="cp-kicker">COMMISSION / PAYOUT CONTROL</p><h1>{{ t("佣金派发管理", "Commission payouts") }}</h1><p>{{ t("真实派发后台。派发策略默认关闭；周期核算就绪不代表已派发或已查询派发状态。", "Live payout controls. Payout policy is disabled by default; a ready calculation does not mean paid or that payout state was queried.") }}</p></div><button class="button" type="button" :disabled="loadingPolicy || loadingList || !rights.view" @click="reloadAll()">{{ t("刷新", "Refresh") }}</button></header>

    <div class="cp-callout" role="note">{{ t("自动派发须先开启本品牌运行开关。周期内只要有任一投注快照为人工派发，整个混合周期都须品牌管理员审批；不会拆分自动部分。重试只续跑未完成目标，不重复已完成的入账。", "Automatic payout requires enabling this brand's runtime switch. If any bet snapshot in a cycle is manual, a brand administrator must approve the whole mixed cycle; the automatic portion is not split out. Retry resumes unfinished targets without repeating completed credits.") }}</div>
    <p v-if="!rights.view" class="cp-state">{{ t("当前账号没有此品牌的佣金查看权限。", "This account cannot view commissions for this brand.") }}</p>
    <template v-else>
      <div v-if="policyError" class="cp-error" role="alert">{{ policyError }}</div>
      <section class="cp-policy" aria-labelledby="cp-policy-title" data-testid="commission-payment-policy">
        <div><h2 id="cp-policy-title">{{ t("真实派发策略", "Live payout policy") }}</h2><p>{{ policy ? `${t("版本", "Version")} ${policy.version} · ${formatDate(policy.updated_at)}` : t("读取当前品牌策略…", "Loading brand policy…") }}</p></div>
        <label class="cp-toggle"><input data-testid="commission-payment-policy-enabled" v-model="policyEnabled" type="checkbox" :disabled="!rights.policyWrite || !policy || writing || Boolean(unknownIntent) || conflict" /><span>{{ policyEnabled ? t("真实派发已启用", "Live payout enabled") : t("真实派发已关闭", "Live payout disabled") }}</span></label>
        <p v-if="!rights.policyWrite" class="cp-muted">{{ t("只读：策略更改要求品牌佣金派发策略写入权限。", "Read only: changing policy requires the brand payout policy write permission.") }}</p>
        <p v-if="brandStatus === 'paused'" class="cp-muted">{{ t("品牌已暂停；财务派发策略仍可按权限管理。", "The brand is paused; finance payout policy remains manageable with the required permission.") }}</p>
        <p class="cp-muted">{{ policyEnabled ? t("开启后，后台可能处理历史 ready 且未派发周期；若投注时派发模式为自动，可能直接产生真实积分入账。", "Enabling may process historical ready cycles that have not been paid; automatic bet-time payout mode may create real points credits.") : t("关闭会暂停后续积分入账，不撤销已有审批，也不回滚已入账积分。", "Disabling pauses future credits. It does not revoke existing approvals or reverse credited points.") }}</p>
        <label class="cp-reason">{{ t("策略更改原因", "Policy change reason") }}<textarea data-testid="commission-payment-policy-reason" v-model="reason" rows="2" maxlength="500" :disabled="!rights.policyWrite || writing || Boolean(unknownIntent) || conflict" /></label>
        <div class="cp-actions"><button data-testid="commission-payment-policy-review" class="button button-primary" type="button" :disabled="!policyCanSave || policyEnabled === policy?.enabled" @click="openReview('policy')">{{ t("审核并保存策略", "Review and save policy") }}</button><button v-if="conflict" class="button" type="button" :disabled="conflictReloading" @click="reloadConflict">{{ conflictReloading ? t("正在刷新…", "Refreshing…") : t("刷新并重新审核", "Refresh and review again") }}</button></div>
      </section>

      <div class="cp-section-heading"><div><h2>{{ t("派发任务", "Payout jobs") }}</h2><p>{{ t("状态来自派发 API；核算页的 ready 状态不能替代此处的派发记录。", "These states come from the payout API; ready on the calculation page is not a payout record.") }}</p></div><button data-testid="commission-payments-refresh" class="button" type="button" :disabled="loadingList" @click="loadList(0)">{{ loadingList ? t("读取中…", "Loading…") : t("刷新列表", "Refresh list") }}</button></div>
      <div v-if="listError" class="cp-error" role="alert">{{ listError }}</div>
      <p v-else-if="loadingList && !page" class="cp-state" role="status">{{ t("正在读取派发任务…", "Loading payout jobs…") }}</p>
      <p v-else-if="page?.items.length === 0" class="cp-state">{{ t("没有可显示的派发任务。", "No payout jobs to display.") }}</p>
      <div v-if="page?.items.length" class="cp-table-wrap" data-testid="commission-payments-jobs"><table><thead><tr><th>{{ t("派发记录", "Payment") }}</th><th>{{ t("状态", "State") }}</th><th>{{ t("模式", "Mode") }}</th><th>{{ t("积分", "Points") }}</th><th>{{ t("目标 / 已入账", "Targets / paid") }}</th><th>{{ t("更新时间", "Updated") }}</th></tr></thead><tbody><tr v-for="item in page.items" :key="item.id" :data-payment-id="item.id" :class="{ selected: selected?.id === item.id }"><td><button class="cp-link" type="button" @click="loadDetail(item.id)">{{ item.id }}</button><small>Cycle {{ item.cycle_id }}</small></td><td><span class="cp-status" :class="`state-${item.state}`">{{ stateLabel(item.state) }}</span><small v-if="item.last_error_code">{{ item.last_error_code }}</small></td><td>{{ modeLabel(item.payout_mode) }}</td><td>{{ item.paid_points }} / {{ item.total_points }}</td><td>{{ item.paid_count }} / {{ item.target_count }}</td><td>{{ formatDate(item.updated_at) }}</td></tr></tbody></table></div>
      <div v-if="page" class="cp-pagination"><span>{{ page.offset + 1 }}–{{ page.offset + page.items.length }} / {{ page.total_count }}</span><div><button class="button" type="button" :disabled="!hasPrevious || loadingList" @click="loadList(Math.max(0, offset - PAGE_SIZE))">{{ t("上一页", "Previous") }}</button><button class="button" type="button" :disabled="!hasNext || loadingList" @click="loadList(offset + PAGE_SIZE)">{{ t("下一页", "Next") }}</button></div></div>

      <section v-if="loadingDetail" class="cp-detail cp-state" role="status">{{ t("读取当前派发记录…", "Loading current payout record…") }}</section>
      <section v-else-if="detail" class="cp-detail" aria-labelledby="cp-detail-title">
        <div class="cp-section-heading"><div><h2 id="cp-detail-title">{{ t("派发记录详情", "Payout record details") }}</h2><p>{{ stateLabel(detail.state) }} · v{{ detail.version }} · {{ modeLabel(detail.payout_mode) }}</p></div><button class="button" type="button" :disabled="loadingDetail" @click="loadDetail(detail.id)">{{ t("刷新详情", "Refresh details") }}</button></div>
        <div v-if="detailError" class="cp-error" role="alert">{{ detailError }}</div>
        <dl class="cp-facts"><dt>ID</dt><dd>{{ detail.id }}</dd><dt>{{ t("周期 / 核算代次", "Cycle / calculation run") }}</dt><dd>{{ detail.cycle_id }}<br />{{ detail.run_id }}</dd><dt>{{ t("证据代次", "Evidence epoch") }}</dt><dd>{{ detail.evidence_epoch }}</dd><dt>{{ t("已入账 / 目标", "Paid / targets") }}</dt><dd>{{ detail.paid_count }} / {{ detail.target_count }}</dd><dt>{{ t("积分已入账 / 总额", "Points paid / total") }}</dt><dd>{{ detail.paid_points }} / {{ detail.total_points }}</dd><dt>{{ t("创建审计记录", "Creation audit record") }}</dt><dd>{{ detail.creation_audit_log_id }}</dd><dt>{{ t("最后错误", "Last error") }}</dt><dd>{{ detail.last_error_code ?? t("无", "None") }}</dd><dt>{{ t("创建 / 更新", "Created / updated") }}</dt><dd>{{ formatDate(detail.created_at) }}<br />{{ formatDate(detail.updated_at) }}</dd></dl>
        <div v-if="detail.payout_mode === 'mixed' && detail.state === 'awaiting_approval'" class="cp-callout" role="note">{{ t("此混合周期含人工投注快照，须由品牌管理员审批整个周期；自动部分不会单独派发。", "This mixed cycle contains a manual bet snapshot. A brand administrator must approve the entire cycle; the automatic portion will not be paid separately.") }}</div>
        <div v-else-if="detail.state === 'blocked'" class="cp-callout" role="note">{{ t("此记录因更正、入账证据或其他阻止原因需要人工处理；此类阻止记录不能在此审批或重试。", "This record is blocked for a correction, ledger evidence, or another reason requiring manual handling. This blocked record cannot be approved or retried here.") }}</div>
        <div v-if="detail.paid_count !== '0' && detail.state === 'failed'" class="cp-callout" role="note">{{ t("已有部分目标入账。重试会由后端续跑未完成目标，已入账目标保留且不会重复入账。", "Some targets are already credited. The backend resumes only unfinished targets; credited targets remain accounted for and are not credited again.") }}</div>
        <label class="cp-reason">{{ t("审批 / 重试原因", "Approval / retry reason") }}<textarea data-testid="commission-payment-action-reason" v-model="reason" rows="2" maxlength="500" :disabled="writing || Boolean(unknownIntent) || conflict" /></label>
        <div class="cp-actions"><button v-if="detail.state === 'awaiting_approval'" data-testid="commission-payment-approve" class="button button-primary" type="button" :disabled="!canApprove" @click="openReview('approve')">{{ t("审核并批准派发", "Review and approve payout") }}</button><button v-if="detail.state === 'failed'" data-testid="commission-payment-retry" class="button button-primary" type="button" :disabled="!canRetry" @click="openReview('retry')">{{ t("审核并重试", "Review and retry") }}</button><button class="button" type="button" :disabled="loadingList || loadingDetail" @click="reloadAll()">{{ t("刷新策略及记录", "Refresh policy and record") }}</button></div>
        <CommissionAdjustmentsManagement v-if="!detailError"
          :key="`commission-adjustments:${account.id}:${brandId}:${detail.id}`"
          :account="account" :brand-id="brandId" :payment-id="detail.id"
          :payment-state="detail.state" :brand-status="brandStatus"
          @session-invalid="emit('session-invalid')" />
      </section>
      <div v-if="notice" class="cp-success" role="status">{{ notice }}</div>
      <div v-if="writeError" class="cp-error" role="alert">{{ writeError }}<button v-if="unknownIntent && rights.view" class="button" type="button" :disabled="writing || (unknownIntent.operation === 'policy' ? !rights.policyWrite : unknownIntent.operation === 'approve' ? !rights.approve : !rights.retry)" @click="retryUnknown">{{ writing ? t("正在重试…", "Retrying…") : t("使用原请求重试", "Retry original request") }}</button><button v-if="conflict" class="button" type="button" :disabled="conflictReloading" @click="reloadConflict">{{ t("刷新并重新审核", "Refresh and review again") }}</button></div>
      <section v-if="conflict && conflictIntent" class="cp-conflict" data-testid="commission-payment-conflict"><p>{{ conflictRefreshed ? t("已重新读取当前服务端状态。请人工核对后明确丢弃旧 409 请求；新操作才会生成新幂等键。", "Current server state has been reread. Review it manually and explicitly discard the old 409 request; only then can a new key be created.") : t("409 原请求已冻结且不会重放。请手动刷新策略和记录，再审核最新版本。", "The original 409 request is frozen and will not be replayed. Manually refresh the policy and record, then review the latest version.") }}</p><label class="cp-confirm"><input v-model="discardReviewed" type="checkbox" :disabled="!conflictRefreshed || conflictReloading" />{{ t("我已核对刷新后的当前状态，并确认丢弃原请求。", "I reviewed the refreshed current state and confirm discarding the original request.") }}</label><div class="cp-actions"><button class="button" type="button" :disabled="conflictReloading" @click="reloadConflict">{{ conflictReloading ? t("正在刷新…", "Refreshing…") : t("手动刷新当前状态", "Manually refresh current state") }}</button><button class="button button-primary" type="button" :disabled="!conflictRefreshed || !discardReviewed || conflictReloading" @click="discardConflictIntent">{{ t("确认丢弃并允许重新审核", "Confirm discard and allow new review") }}</button></div></section>
      <section v-if="receipt" class="cp-receipt" role="status" aria-live="polite" data-testid="commission-payment-receipt"><b>{{ t("操作回执（与当前状态分开）", "Operation receipt (separate from current state)") }}</b><span>{{ displayReceipt() }}</span><code v-if="receipt.payment">{{ receipt.payment.id }}</code></section>
      <section v-if="dialog" class="cp-dialog" role="dialog" aria-modal="true" :aria-label="t('审核派发操作', 'Review payout operation')" data-testid="commission-payment-review">
        <h2>{{ t("审核派发操作", "Review payout operation") }}</h2>
        <p>{{ t("提交前核对品牌、目标、版本和原因。确认后使用冻结的请求正文及幂等键。", "Review the brand, target, version, and reason. Confirmation submits the frozen body and idempotency key.") }}</p>
        <p v-if="dialog.operation === 'policy' && 'enabled' in dialog.body && dialog.body.enabled" class="cp-callout" data-testid="commission-payment-enable-warning">{{ t("开启此运行开关可能立即接入历史 ready 且未派发周期。若派发模式为自动，后台可能在无需逐任务审批的情况下实际给账户入账积分。关闭开关只暂停后续入账，不撤销已有审批或已入账积分。", "Enabling this runtime switch may immediately enqueue historical ready cycles. Automatic payout mode may credit real points without per-job approval. Disabling pauses future credits; it does not revoke approvals or reverse credits.") }}</p>
        <dl>
          <dt>{{ t("品牌", "Brand") }}</dt><dd>{{ dialog.brandId }}</dd>
          <dt>{{ t("操作 / 目标", "Operation / target") }}</dt><dd>{{ dialog.operation }} · {{ dialog.targetId ?? dialog.brandId }}</dd>
          <dt>{{ t("版本", "Version") }}</dt><dd>v{{ dialog.body.version }}</dd>
          <dt v-if="'enabled' in dialog.body">{{ t("运行开关", "Runtime switch") }}</dt><dd v-if="'enabled' in dialog.body">{{ dialog.body.enabled ? t("开启", "Enable") : t("关闭", "Disable") }}</dd>
          <dt>{{ t("原因", "Reason") }}</dt><dd>{{ dialog.body.reason }}</dd>
          <dt>{{ t("幂等键", "Idempotency key") }}</dt><dd data-testid="commission-payment-review-key">{{ dialog.key }}</dd>
        </dl>
        <label class="cp-confirm"><input data-testid="commission-payment-review-confirmed" v-model="confirmed" type="checkbox" />{{ t("我已理解本次影响，核对以上请求并授权提交。", "I understand the impact, reviewed the request, and authorize submission.") }}</label>
        <div class="cp-actions"><button data-testid="commission-payment-confirm-submit" class="button button-primary" type="button" :disabled="writing || !confirmed" @click="submit(dialog)">{{ writing ? t("提交中…", "Submitting…") : t("确认提交", "Confirm and submit") }}</button><button class="button" type="button" :disabled="writing" @click="dialog = null; confirmed = false">{{ t("取消", "Cancel") }}</button></div>
      </section>
    </template>
  </article>
</template>

<style scoped>
.commission-payments{display:grid;gap:18px;min-width:0}.cp-heading,.cp-section-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:16px}.cp-heading h1,.cp-section-heading h2,.cp-policy h2{margin:0}.cp-heading p,.cp-section-heading p,.cp-policy p{margin:6px 0 0;color:var(--muted,#64748b);font-size:13px;line-height:1.55}.cp-kicker{font-size:10px!important;letter-spacing:.14em;color:var(--accent,#2563eb)!important;font-weight:800}.cp-callout,.cp-error,.cp-success,.cp-state,.cp-receipt{padding:12px 14px;border-radius:10px;line-height:1.55}.cp-callout{border:1px solid var(--warning,#d97706);background:color-mix(in srgb,var(--warning,#d97706) 8%,transparent)}.cp-error{display:flex;align-items:center;justify-content:space-between;gap:12px;color:var(--danger,#b42318);background:color-mix(in srgb,var(--danger,#b42318) 8%,transparent)}.cp-success{color:var(--success,#15803d);background:color-mix(in srgb,var(--success,#15803d) 8%,transparent)}.cp-state,.cp-muted{color:var(--muted,#64748b)}.cp-policy,.cp-detail,.cp-dialog{display:grid;gap:14px;padding:18px;border:1px solid var(--border,#dbe2ea);border-radius:14px;background:var(--surface,#fff)}.cp-toggle{display:flex;align-items:center;gap:10px;font-weight:750}.cp-toggle input,.cp-confirm input{width:18px;height:18px;accent-color:var(--accent,#2563eb)}.cp-reason{display:grid;gap:6px;font-weight:650}.cp-reason textarea{resize:vertical;min-height:60px;padding:10px;border:1px solid var(--border,#cbd5e1);border-radius:8px;font:inherit}.cp-actions,.cp-pagination,.cp-pagination>div{display:flex;align-items:center;gap:8px;flex-wrap:wrap}.cp-pagination{justify-content:space-between}.cp-table-wrap{width:100%;overflow:auto;border:1px solid var(--border,#dbe2ea);border-radius:12px}.cp-table-wrap table{width:100%;border-collapse:collapse;min-width:790px}.cp-table-wrap th,.cp-table-wrap td{text-align:left;padding:11px 12px;border-bottom:1px solid var(--border,#e2e8f0);vertical-align:top}.cp-table-wrap th{font-size:11px;color:var(--muted,#64748b);background:var(--surface-muted,#f8fafc)}.cp-table-wrap tr:last-child td{border-bottom:0}.cp-table-wrap tr.selected{background:color-mix(in srgb,var(--accent,#2563eb) 7%,transparent)}.cp-table-wrap td small{display:block;margin-top:4px;color:var(--muted,#64748b);overflow-wrap:anywhere}.cp-link{padding:0;border:0;background:transparent;color:var(--accent,#2563eb);font:inherit;text-align:left;overflow-wrap:anywhere;cursor:pointer}.cp-status{display:inline-flex;padding:4px 8px;border-radius:999px;background:#eef2f7;font-size:12px;font-weight:700}.state-paid{color:#087443;background:#e6f6ee}.state-failed,.state-blocked{color:#a12a22;background:#fff0ed}.state-stale{color:#8a5800;background:#fff6dd}.cp-facts,.cp-dialog dl{display:grid;grid-template-columns:minmax(140px,.35fr) minmax(0,1fr);gap:9px 14px;margin:0}.cp-facts dt,.cp-dialog dt{color:var(--muted,#64748b)}.cp-facts dd,.cp-dialog dd{margin:0;overflow-wrap:anywhere}.cp-receipt{display:grid;gap:5px;border:1px solid var(--success,#15803d);background:color-mix(in srgb,var(--success,#15803d) 7%,transparent)}.cp-receipt code{overflow-wrap:anywhere}.cp-dialog{max-width:720px;width:100%;margin-inline:auto;box-shadow:0 20px 60px #0f172a30}.cp-dialog h2{margin:0}.cp-dialog>p{color:var(--muted,#64748b)}.cp-confirm{display:flex;gap:9px;align-items:center}.cp-dialog dd:last-child{font-family:monospace;font-size:12px}
@media(max-width:700px){.cp-heading,.cp-section-heading{align-items:stretch;flex-direction:column}.cp-heading>.button,.cp-section-heading>.button{align-self:flex-start}.cp-policy,.cp-detail{padding:14px}.cp-facts,.cp-dialog dl{grid-template-columns:1fr;gap:4px}.cp-facts dd,.cp-dialog dd{margin-bottom:8px}.cp-actions>.button{flex:1 1 100%}.cp-pagination{align-items:flex-start;flex-direction:column}.cp-error{align-items:flex-start;flex-direction:column}}
</style>
