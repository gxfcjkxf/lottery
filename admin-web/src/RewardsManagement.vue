<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { useAdminI18n } from "./i18n";
import {
  createRewardsApi, type RewardActionBody, type RewardActionPage, type RewardGrantBody,
  type RewardOrder, type RewardOrderPage, type RewardOperation,
} from "./rewards-api";
import {
  classifyRewardWriteFailure, clearAllRewardIntents, clearPendingRewardIntent,
  createRewardIntent, freezeRewardBody, getPendingRewardIntent, hideRewardIntentsForScope,
  isRewardIntentConflict, listPendingRewardIntents, markRewardIntentConflict,
  rewardPermissions, rewardSessionGeneration, setPendingRewardIntent,
  type PendingRewardIntent, type RewardIntentScope,
} from "./rewards-state";

const props = defineProps<{ account: AdminAccount; brandId: string; brandStatus?: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, locale } = useAdminI18n();
const api = computed(() => createRewardsApi(fetch, props.account.id));
const PAGE_SIZE = 20;
const rights = computed(() => rewardPermissions(props.account, props.brandId));
const scopeStamp = computed(() => JSON.stringify([props.account.id, props.account.super_admin, props.account.brand_ids,
  props.account.permissions, props.account.permissions_by_brand, props.account.platform_permissions, props.brandId, props.brandStatus, rights.value]));
const page = ref<RewardOrderPage | null>(null), selected = ref<RewardOrder | null>(null), detail = ref<RewardOrder | null>(null);
const history = ref<RewardActionPage | null>(null);
const receipt = ref<{ operation: RewardOperation; order: RewardOrder } | null>(null);
const loadingList = ref(false), loadingDetail = ref(false), loadingHistory = ref(false), writing = ref(false);
const listError = ref(""), detailError = ref(""), historyError = ref(""), writeError = ref(""), notice = ref("");
const memberID = ref(""), points = ref(""), grantReason = ref(""), actionReason = ref("");
const dialog = ref<PendingRewardIntent | null>(null), confirmed = ref(false);
const unknownIntent = ref<PendingRewardIntent | null>(null), conflictIntent = ref<PendingRewardIntent | null>(null);
const hasUnresolvedIntent = ref(false);
const conflict = ref(false), conflictReloading = ref(false), conflictRefreshed = ref(false), discardReviewed = ref(false);
let alive = true, listTicket = 0, detailTicket = 0, historyTicket = 0, writeTicket = 0, conflictTicket = 0;

const offset = computed(() => page.value?.offset ?? 0);
const hasPrevious = computed(() => offset.value > 0);
const hasNext = computed(() => page.value !== null && BigInt(page.value.offset + page.value.limit) < BigInt(page.value.total_count));
const validGrant = computed(() => UUID.test(memberID.value) && validPoints(points.value) && validReason(grantReason.value));
const canGrant = computed(() => rights.value.grant && props.brandStatus !== "disabled" && !props.account.super_admin && !writing.value && !hasUnresolvedIntent.value && !conflict.value && validGrant.value);
const canRevoke = computed(() => Boolean(rights.value.revoke && props.brandStatus !== "disabled" && !props.account.super_admin && !writing.value && !hasUnresolvedIntent.value && !conflict.value && detail.value?.state === "granted" && detail.value.version < Number.MAX_SAFE_INTEGER && validReason(actionReason.value)));
const canRetry = computed(() => Boolean(rights.value.retry && props.brandStatus !== "disabled" && !props.account.super_admin && !writing.value && !hasUnresolvedIntent.value && !conflict.value && detail.value?.state === "revocation_pending" && detail.value.version < Number.MAX_SAFE_INTEGER && validReason(actionReason.value)));
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const MAX_INT64 = 9_223_372_036_854_775_807n;

function validPoints(value: string): boolean {
  if (!/^[1-9][0-9]*$/.test(value)) return false;
  try { return BigInt(value) <= MAX_INT64; } catch { return false; }
}
function validReason(value: string): boolean {
  if (!value || value.trim() !== value || new TextEncoder().encode(value).length > 500 || /[\u0000-\u001f\u007f-\u009f]/u.test(value)) return false;
  for (let i = 0; i < value.length; i++) { const code = value.charCodeAt(i); if (code >= 0xd800 && code <= 0xdbff) { const next = value.charCodeAt(++i); if (!(next >= 0xdc00 && next <= 0xdfff)) return false; } else if (code >= 0xdc00 && code <= 0xdfff) return false; }
  return true;
}
function rewardScope(operation: RewardOperation, targetId?: string): RewardIntentScope {
  return { accountId: props.account.id, brandId: props.brandId, operation, ...(operation === "grant" ? {} : { targetId }) };
}
function current(ticket: number, lane: "list" | "detail" | "history", captured: string, session: number): boolean {
  const laneTicket = lane === "list" ? listTicket : lane === "detail" ? detailTicket : historyTicket;
  return alive && ticket === laneTicket && captured === scopeStamp.value && session === rewardSessionGeneration() && rights.value.view;
}
function currentWrite(ticket: number, captured: string, session: number): boolean {
  return alive && ticket === writeTicket && captured === scopeStamp.value && session === rewardSessionGeneration();
}
function clearVisible() {
  page.value = null; selected.value = null; detail.value = null; history.value = null; receipt.value = null;
  memberID.value = points.value = grantReason.value = actionReason.value = "";
  dialog.value = null; confirmed.value = false; unknownIntent.value = null; conflictIntent.value = null;
  hasUnresolvedIntent.value = false;
  conflict.value = false; conflictRefreshed.value = false; discardReviewed.value = false;
  listError.value = detailError.value = historyError.value = writeError.value = notice.value = "";
  loadingList.value = loadingDetail.value = loadingHistory.value = writing.value = false;
}
function resetScope(_next?: string, previous?: string) {
  let previousAccount = "", previousBrand = "";
  if (previous) {
    try { const old = JSON.parse(previous) as unknown[]; previousAccount = String(old[0]); previousBrand = String(old[6]); } catch { /* Invalid old scope cannot reveal an intent. */ }
  }
  if (previousAccount && previousAccount !== props.account.id) clearAllRewardIntents();
  else if (previousAccount && previousBrand) hideRewardIntentsForScope(previousAccount, previousBrand);
  listTicket++; detailTicket++; historyTicket++; writeTicket++; conflictTicket++;
  clearVisible();
  if (!rights.value.view) return;
  const pending = listPendingRewardIntents(props.account.id, props.brandId);
  hasUnresolvedIntent.value = pending.length > 0;
  const visible = pending.find((intent) => permissionFor(intent.operation));
  const conflicted = visible && isRewardIntentConflict(visible) ? visible : pending.find((intent) => permissionFor(intent.operation) && isRewardIntentConflict(intent));
  conflictIntent.value = conflicted ?? null; conflict.value = Boolean(conflicted);
  unknownIntent.value = conflicted ? null : visible && !isRewardIntentConflict(visible) ? visible : pending.find((intent) => permissionFor(intent.operation) && !isRewardIntentConflict(intent)) ?? null;
  if (unknownIntent.value) writeError.value = t("有一个同会话待确认请求。请先核对原意图，只能使用原正文和幂等键恢复。", "A same-session request needs resolution. Review the original intent and recover only with its original body and key.");
  if (rights.value.view) void loadList(0);
}
function sessionExpired(cause: unknown, captured: string, session: number): boolean {
  if (cause instanceof AdminApiError && cause.status === 401 && alive && captured === scopeStamp.value && session === rewardSessionGeneration()) {
    listTicket++; detailTicket++; historyTicket++; writeTicket++; clearAllRewardIntents(); clearVisible(); emit("session-invalid"); return true;
  }
  return false;
}
function readFailure(cause: unknown): string {
  return cause instanceof AdminApiError && cause.status === 0
    ? t("网络或响应不可用，当前数据尚未读取。", "The network or response is unavailable; current data was not loaded.")
    : cause instanceof Error ? cause.message : t("读取失败。", "Unable to load data.");
}
function cancelConflictReview() {
  if (!conflict.value) return;
  conflictTicket++; conflictReloading.value = false; conflictRefreshed.value = false; discardReviewed.value = false;
}
async function loadList(nextOffset = offset.value, preserveConflictReview = false) {
  if (!preserveConflictReview) cancelConflictReview();
  if (!rights.value.view) return false;
  const ticket = ++listTicket, captured = scopeStamp.value, session = rewardSessionGeneration();
  loadingList.value = true; listError.value = "";
  try {
    const fresh = await api.value.list(props.brandId, PAGE_SIZE, nextOffset);
    if (!current(ticket, "list", captured, session)) return false;
    page.value = fresh;
    return true;
  } catch (cause) {
    if (current(ticket, "list", captured, session)) { sessionExpired(cause, captured, session); listError.value = readFailure(cause); }
    return false;
  } finally { if (alive && ticket === listTicket && captured === scopeStamp.value && session === rewardSessionGeneration()) loadingList.value = false; }
}
async function loadDetail(id: string, preserveConflictReview = false) {
  if (!preserveConflictReview) cancelConflictReview();
  if (!rights.value.view || !UUID.test(id)) return false;
  const ticket = ++detailTicket, captured = scopeStamp.value, session = rewardSessionGeneration();
  loadingDetail.value = true; detailError.value = "";
  if (detail.value?.id !== id) { detail.value = null; selected.value = page.value?.items.find((item) => item.id === id) ?? null; historyTicket++; history.value = null; actionReason.value = ""; }
  try {
    const fresh = await api.value.read(props.brandId, id);
    if (!current(ticket, "detail", captured, session) || fresh.id !== id) return false;
    selected.value = fresh; detail.value = fresh;
    return true;
  } catch (cause) {
    if (current(ticket, "detail", captured, session)) { sessionExpired(cause, captured, session); detailError.value = readFailure(cause); }
    return false;
  } finally { if (alive && ticket === detailTicket && captured === scopeStamp.value && session === rewardSessionGeneration()) loadingDetail.value = false; }
}
async function loadHistory(id: string, nextOffset = history.value?.offset ?? 0, preserveConflictReview = false) {
  if (!preserveConflictReview) cancelConflictReview();
  if (!rights.value.view || !UUID.test(id)) return false;
  const ticket = ++historyTicket, captured = scopeStamp.value, session = rewardSessionGeneration();
  loadingHistory.value = true; historyError.value = "";
  try {
    const fresh = await api.value.actions(props.brandId, id, PAGE_SIZE, nextOffset);
    if (!current(ticket, "history", captured, session) || fresh.order_id !== id) return false;
    history.value = fresh;
    return true;
  } catch (cause) {
    if (current(ticket, "history", captured, session)) { sessionExpired(cause, captured, session); historyError.value = readFailure(cause); }
    return false;
  } finally { if (alive && ticket === historyTicket && captured === scopeStamp.value && session === rewardSessionGeneration()) loadingHistory.value = false; }
}
async function selectOrder(id: string) {
  if (!UUID.test(id)) return;
  history.value = null;
  await Promise.all([loadDetail(id), loadHistory(id, 0)]);
}
function newIntent(operation: RewardOperation): PendingRewardIntent | null {
  if (hasUnresolvedIntent.value || unknownIntent.value || conflict.value || writing.value) return null;
  if (operation === "grant") {
    if (!canGrant.value) return null;
    const scope = rewardScope("grant"), body = freezeRewardBody("grant", { member_id: memberID.value, points: points.value, reason: grantReason.value });
    return createRewardIntent(scope, body, createIdempotencyKey());
  }
  const row = detail.value;
  if (!row || row.id !== selected.value?.id || row.version >= Number.MAX_SAFE_INTEGER || !validReason(actionReason.value) || loadingDetail.value || !rights.value.view || props.account.super_admin) return null;
  if (operation === "revoke" && !canRevoke.value || operation === "retry" && !canRetry.value) return null;
  const scope = rewardScope(operation, row.id), body = freezeRewardBody(operation, { version: row.version, reason: actionReason.value });
  return createRewardIntent(scope, body, createIdempotencyKey());
}
function openReview(operation: RewardOperation) {
  const intent = newIntent(operation);
  if (!intent) return;
  dialog.value = intent; confirmed.value = false; writeError.value = ""; notice.value = "";
}
function permissionFor(operation: RewardOperation): boolean {
  return operation === "grant" ? rights.value.grant : operation === "revoke" ? rights.value.revoke : rights.value.retry;
}
async function submit(intent: PendingRewardIntent) {
  if (!confirmed.value || writing.value || !rights.value.view || !permissionFor(intent.operation) || props.brandStatus === "disabled" ||
    intent.accountId !== props.account.id || intent.brandId !== props.brandId || (intent.operation !== "grant" && unknownIntent.value?.key !== intent.key && (!detail.value || detail.value.id !== intent.targetId || detail.value.version !== (intent.body as RewardActionBody).version))) return;
  const captured = scopeStamp.value, session = rewardSessionGeneration(), ticket = ++writeTicket;
  writing.value = true; writeError.value = ""; notice.value = ""; setPendingRewardIntent(intent, intent);
  hasUnresolvedIntent.value = true;
  try {
    let result: RewardOrder;
    if (intent.operation === "grant") result = await api.value.create(intent.brandId, intent.body as Readonly<RewardGrantBody>, intent.key);
    else if (intent.operation === "revoke") result = await api.value.revoke(intent.brandId, intent.targetId!, intent.body as Readonly<RewardActionBody>, intent.key);
    else result = await api.value.retryRevocation(intent.brandId, intent.targetId!, intent.body as Readonly<RewardActionBody>, intent.key);
    clearPendingRewardIntent(intent, intent.key);
    if (!currentWrite(ticket, captured, session)) return;
    hasUnresolvedIntent.value = false;
    receipt.value = { operation: intent.operation, order: result };
    dialog.value = null; unknownIntent.value = null; confirmed.value = false;
    // An acknowledged write invalidates the old action version immediately.
    // Failed subsequent GETs must not leave the old order actionable.
    detailTicket++; historyTicket++; detail.value = null; history.value = null;
    notice.value = t("服务器已确认操作回执；请以当前详情和历史查询核对最新状态。", "The server confirmed the operation receipt. Check the current detail and history for the latest state.");
    await loadList(offset.value);
    if (currentWrite(ticket, captured, session)) {
      const target = intent.operation === "grant" ? result.id : intent.targetId!;
      await Promise.all([loadDetail(target), loadHistory(target, 0)]);
    }
  } catch (cause) {
    const status = cause instanceof AdminApiError ? cause.status : undefined, outcome = classifyRewardWriteFailure(status);
    if (!currentWrite(ticket, captured, session)) {
      if (getPendingRewardIntent(intent)?.key === intent.key) {
        if (outcome === "conflict") markRewardIntentConflict(intent);
        else if (outcome === "definitive") clearPendingRewardIntent(intent, intent.key);
      }
      return;
    }
    if (sessionExpired(cause, captured, session)) return;
    if (outcome === "unknown") {
      setPendingRewardIntent(intent, intent); unknownIntent.value = intent; dialog.value = null; confirmed.value = false;
      writeError.value = t("提交结果未知。原请求已冻结；请先核对原意图，再使用原正文和幂等键恢复。", "The outcome is unknown. The original request is frozen; review the original intent, then recover with the same body and key.");
    } else if (outcome === "conflict") {
      markRewardIntentConflict(intent); unknownIntent.value = null; conflictIntent.value = intent; dialog.value = null; confirmed.value = false;
      conflict.value = true; conflictRefreshed.value = false; discardReviewed.value = false;
      writeError.value = t("服务器返回 409。原意图仍被冻结；请成功刷新相关读取并人工核对，再勾选确认丢弃。", "The server returned 409. The original intent remains frozen; successfully refresh the relevant reads, review them, then confirm its discard.");
    } else {
      clearPendingRewardIntent(intent, intent.key); hasUnresolvedIntent.value = false; unknownIntent.value = null; dialog.value = null; confirmed.value = false;
      writeError.value = cause instanceof Error ? cause.message : t("提交失败。", "Submission failed.");
    }
  } finally { if (currentWrite(ticket, captured, session)) writing.value = false; }
}
function retryUnknown() {
  const intent = unknownIntent.value;
  if (!intent || writing.value || !rights.value.view || !permissionFor(intent.operation) || props.brandStatus === "disabled" || intent.accountId !== props.account.id || intent.brandId !== props.brandId) return;
  dialog.value = intent; confirmed.value = false;
}
async function reloadConflict() {
  if (!conflict.value || conflictReloading.value || !conflictIntent.value) return;
  const intent = conflictIntent.value, ticket = ++conflictTicket, captured = scopeStamp.value;
  conflictReloading.value = true; conflictRefreshed.value = false; discardReviewed.value = false;
  listError.value = detailError.value = historyError.value = "";
  const reads: Promise<boolean>[] = [loadList(offset.value, true)];
  if (intent.operation !== "grant" && intent.targetId) reads.push(loadDetail(intent.targetId, true), loadHistory(intent.targetId, 0, true));
  const results = await Promise.all(reads);
  if (alive && ticket === conflictTicket && captured === scopeStamp.value && rights.value.view && results.every(Boolean)) {
    conflictRefreshed.value = true;
    writeError.value = t("相关数据已成功刷新。请检查列表、详情与历史，再明确勾选丢弃原请求。", "Relevant data was refreshed successfully. Review the list, detail, and history, then explicitly confirm discarding the original request.");
  }
  if (ticket === conflictTicket) conflictReloading.value = false;
}
function discardConflictIntent() {
  const intent = conflictIntent.value;
  if (!intent || !conflictRefreshed.value || !discardReviewed.value || conflictReloading.value) return;
  clearPendingRewardIntent(intent, intent.key); conflictIntent.value = null; conflict.value = false;
  hasUnresolvedIntent.value = false;
  conflictRefreshed.value = false; discardReviewed.value = false; writeError.value = "";
  notice.value = t("已确认丢弃旧请求。请按当前状态和版本重新审核；新操作将使用新幂等键。", "The old request was explicitly discarded. Review the current state and version; a new operation will use a new idempotency key.");
}
function stateLabel(state: RewardOrder["state"]): string {
  const labels: Record<RewardOrder["state"], [string, string]> = { granted: ["已发放", "Granted"], revocation_pending: ["撤销待处理", "Revocation pending"], revoked: ["已撤销", "Revoked"] };
  return t(...labels[state]);
}
function operationLabel(operation: RewardOperation): string {
  const labels: Record<RewardOperation, [string, string]> = { grant: ["发放", "Grant"], revoke: ["撤销", "Revoke"], retry: ["继续撤销", "Retry revocation"] };
  return t(...labels[operation]);
}
function formatDate(value: string): string {
  const date = new Date(value);
  return Number.isFinite(date.getTime()) ? `${new Intl.DateTimeFormat(locale.value === "en" ? "en" : "zh-CN", { dateStyle: "medium", timeStyle: "medium", timeZone: "UTC" }).format(date)} UTC` : value;
}
function nullable(value: string | null): string { return value ?? t("无", "None"); }
function intentScopeLabel(intent: PendingRewardIntent): string { return `${operationLabel(intent.operation)} · ${intent.targetId ?? intent.brandId}`; }
function reviewSnapshot(intent: PendingRewardIntent): string {
  return JSON.stringify({ actor_id: intent.accountId, brand_id: intent.brandId, operation: intent.operation,
    ...(intent.targetId ? { target_id: intent.targetId } : {}), body: intent.body, idempotency_key: intent.key }, null, 2);
}

watch(scopeStamp, resetScope, { immediate: true });
  onUnmounted(() => { alive = false; listTicket++; detailTicket++; historyTicket++; writeTicket++; conflictTicket++; });
</script>

<template>
  <article class="panel rewards-management">
    <header class="rw-heading"><div><p class="rw-kicker">{{ t("积分运营", "POINTS OPERATIONS") }}</p><h1>{{ t("人工奖励管理", "Manual rewards") }}</h1><p>{{ t("人工发放与全额撤销；每次资金操作均须核对并明确确认。", "Manual grants and full reversals; review and explicitly confirm every funds operation.") }}</p></div></header>
    <template v-if="rights.view">
      <p v-if="brandStatus === 'disabled'" class="rw-callout" role="status">{{ t("品牌已禁用，只能查看奖励记录。", "The brand is disabled; reward records are read-only.") }}</p>
      <p v-else-if="brandStatus === 'paused'" class="rw-callout" role="status">{{ t("品牌已暂停；有对应权限时仍可处理人工奖励。", "The brand is paused; manual reward operations remain available with the required permission.") }}</p>

      <section class="rw-grant">
        <div><h2>{{ t("人工发放", "Manual grant") }}</h2><p>{{ t("只向指定会员的gift.available增加正整数积分；不会触发其他资金操作。", "Adds positive integer points only to the selected member’s gift.available balance; no other funds operation is triggered.") }}</p></div>
        <label>{{ t("会员UUID", "Member UUID") }}<input data-testid="rewards-grant-member" v-model="memberID" autocomplete="off" :disabled="!rights.grant || writing || Boolean(unknownIntent) || conflict" /></label>
        <label>{{ t("积分", "Points") }}<input data-testid="rewards-grant-points" v-model="points" inputmode="numeric" autocomplete="off" :disabled="!rights.grant || writing || Boolean(unknownIntent) || conflict" /></label>
        <label>{{ t("发放原因", "Grant reason") }}<textarea data-testid="rewards-grant-reason" v-model="grantReason" rows="2" maxlength="500" :disabled="!rights.grant || writing || Boolean(unknownIntent) || conflict" /></label>
        <button data-testid="rewards-grant-review" class="button button-primary" type="button" :disabled="!canGrant" @click="openReview('grant')">{{ t("核对并审核发放", "Review grant") }}</button>
      </section>

      <div class="rw-section-heading"><div><h2>{{ t("奖励订单", "Reward orders") }}</h2><p>{{ t("订单列表按创建时间及编号排序。", "Orders are sorted by creation time and ID.") }}</p></div><button data-testid="rewards-list-refresh" class="button" type="button" :disabled="loadingList" @click="loadList(0)">{{ loadingList ? t("读取中…", "Loading…") : t("刷新列表", "Refresh list") }}</button></div>
      <div v-if="listError" class="rw-error" role="alert">{{ listError }}</div>
      <p v-else-if="loadingList && !page" class="rw-state" role="status">{{ t("正在读取奖励订单…", "Loading reward orders…") }}</p>
      <p v-else-if="page?.items.length === 0" class="rw-state">{{ t("没有可显示的奖励订单。", "No reward orders to display.") }}</p>
      <div v-if="page?.items.length" class="rw-table-wrap"><table><thead><tr><th>{{ t("订单 / 会员", "Order / member") }}</th><th>{{ t("状态", "State") }}</th><th>{{ t("积分", "Points") }}</th><th>{{ t("原因", "Reason") }}</th><th>{{ t("更新时间", "Updated") }}</th></tr></thead><tbody><tr v-for="item in page.items" :key="item.id" :class="{ selected: detail?.id === item.id }"><td><button class="rw-link" type="button" @click="selectOrder(item.id)">{{ item.id }}</button><small>{{ item.member_id }}</small></td><td>{{ stateLabel(item.state) }} · v{{ item.version }}</td><td>{{ item.points }}</td><td>{{ item.reason }}</td><td>{{ formatDate(item.updated_at) }}</td></tr></tbody></table></div>
      <div v-if="page" class="rw-pagination"><span>{{ page.items.length ? page.offset + 1 : 0 }}–{{ page.offset + page.items.length }} / {{ page.total_count }}</span><div><button class="button" type="button" :disabled="!hasPrevious || loadingList" @click="loadList(Math.max(0, offset - PAGE_SIZE))">{{ t("上一页", "Previous") }}</button><button class="button" type="button" :disabled="!hasNext || loadingList" @click="loadList(offset + PAGE_SIZE)">{{ t("下一页", "Next") }}</button></div></div>

      <section v-if="loadingDetail" class="rw-detail rw-state" role="status">{{ t("正在读取当前奖励详情…", "Loading current reward details…") }}</section>
      <section v-else-if="detail" class="rw-detail">
        <div class="rw-section-heading"><div><h2>{{ t("当前订单详情", "Current order details") }}</h2><p>{{ stateLabel(detail.state) }} · v{{ detail.version }}</p></div><button data-testid="rewards-detail-refresh" class="button" type="button" :disabled="loadingDetail" @click="loadDetail(detail.id)">{{ t("刷新详情", "Refresh details") }}</button></div>
        <div v-if="detailError" class="rw-error" role="alert">{{ detailError }}</div>
        <dl class="rw-facts"><dt>ID</dt><dd>{{ detail.id }}</dd><dt>{{ t("品牌 / 会员", "Brand / member") }}</dt><dd>{{ detail.brand_id }}<br />{{ detail.member_id }}</dd><dt>{{ t("积分 / 状态版本", "Points / state version") }}</dt><dd>{{ detail.points }} · {{ stateLabel(detail.state) }} · v{{ detail.version }}</dd><dt>{{ t("发放账本 / 撤销账本", "Grant ledger / reversal ledger") }}</dt><dd>{{ detail.grant_ledger_entry_id }}<br />{{ nullable(detail.revoke_ledger_entry_id) }}</dd><dt>{{ t("创建审计 / 最后审计", "Creation audit / last audit") }}</dt><dd>{{ detail.creation_audit_log_id }}<br />{{ detail.last_audit_log_id }}</dd><dt>{{ t("最近错误", "Last error") }}</dt><dd>{{ nullable(detail.last_error_code) }}</dd><dt>{{ t("创建人 / 原因", "Created by / original reason") }}</dt><dd>{{ detail.created_by }}<br />{{ detail.reason }}</dd><dt>{{ t("积分政策版本", "Point policy version") }}</dt><dd>{{ detail.point_policy_version }}</dd><dt>{{ t("创建 / 更新 / 撤销时间", "Created / updated / revoked") }}</dt><dd>{{ formatDate(detail.created_at) }}<br />{{ formatDate(detail.updated_at) }}<br />{{ detail.revoked_at ? formatDate(detail.revoked_at) : t("无", "None") }}</dd></dl>
        <p v-if="detail.state === 'revocation_pending'" class="rw-callout" role="note">{{ t("撤销待处理是已记录的业务结果，不会自动重试或解冻；请重新核对当前版本、赠送可用积分和原因后，显式执行继续。", "Revocation pending is a recorded business result. It will not retry or unfreeze automatically; recheck the current version, gift available balance, and reason before explicitly retrying.") }}</p>
        <label class="rw-reason">{{ t("撤销 / 继续原因", "Revoke / retry reason") }}<textarea data-testid="rewards-action-reason" v-model="actionReason" rows="2" maxlength="500" :disabled="writing || Boolean(unknownIntent) || conflict" /></label>
        <div class="rw-actions"><button v-if="detail.state === 'granted'" data-testid="rewards-revoke" class="button button-primary" type="button" :disabled="!canRevoke" @click="openReview('revoke')">{{ t("核对并撤销", "Review and revoke") }}</button><button v-if="detail.state === 'revocation_pending'" data-testid="rewards-retry" class="button button-primary" type="button" :disabled="!canRetry" @click="openReview('retry')">{{ t("核对并继续撤销", "Review and retry revocation") }}</button></div>

        <section class="rw-history"><div class="rw-section-heading"><div><h3>{{ t("不可变操作历史", "Immutable action history") }}</h3><p>{{ t("历史动作记录不代表当前余额。", "Historical actions do not prove the current balance.") }}</p></div><button data-testid="rewards-history-refresh" class="button" type="button" :disabled="loadingHistory" @click="loadHistory(detail.id, 0)">{{ loadingHistory ? t("读取中…", "Loading…") : t("刷新历史", "Refresh history") }}</button></div>
          <div v-if="historyError" class="rw-error" role="alert">{{ historyError }}</div><p v-else-if="loadingHistory && !history" class="rw-state" role="status">{{ t("正在读取操作历史…", "Loading action history…") }}</p><p v-else-if="history?.items.length === 0" class="rw-state">{{ t("没有可显示的操作历史。", "No actions to display.") }}</p>
          <div v-if="history?.items.length" class="rw-table-wrap"><table><thead><tr><th>{{ t("动作 / 版本", "Action / version") }}</th><th>{{ t("前态 → 后态", "Before → after") }}</th><th>{{ t("操作者 / 原因", "Actor / reason") }}</th><th>{{ t("审计 / 账本", "Audit / ledger") }}</th><th>{{ t("时间", "Time") }}</th></tr></thead><tbody><tr v-for="action in history.items" :key="action.id"><td>{{ operationLabel(action.operation) }} · v{{ action.version }}<small>{{ action.id }}</small></td><td>{{ action.state_before ? stateLabel(action.state_before) : t("无", "None") }} → {{ stateLabel(action.state_after) }}</td><td>{{ action.actor_id }}<small>{{ action.reason }}</small></td><td>{{ action.audit_log_id }}<small>{{ nullable(action.ledger_entry_id) }}</small></td><td>{{ formatDate(action.created_at) }}</td></tr></tbody></table></div>
          <div v-if="history" class="rw-pagination"><span>{{ history.items.length ? history.offset + 1 : 0 }}–{{ history.offset + history.items.length }} / {{ history.total_count }}</span><div><button class="button" type="button" :disabled="history.offset === 0 || loadingHistory" @click="loadHistory(detail.id, Math.max(0, history.offset - PAGE_SIZE))">{{ t("上一页", "Previous") }}</button><button class="button" type="button" :disabled="BigInt(history.offset + history.limit) >= BigInt(history.total_count) || loadingHistory" @click="loadHistory(detail.id, history.offset + PAGE_SIZE)">{{ t("下一页", "Next") }}</button></div></div>
        </section>
      </section>

      <div v-if="notice" class="rw-success" role="status">{{ notice }}</div>
      <div v-if="writeError" class="rw-error" role="alert">{{ writeError }}<button v-if="unknownIntent && rights.view" type="button" class="button" :disabled="writing || !permissionFor(unknownIntent.operation) || brandStatus === 'disabled'" @click="retryUnknown">{{ writing ? t("正在重试…", "Retrying…") : t("使用原请求重试", "Retry original request") }}</button><button v-if="conflict" type="button" class="button" :disabled="conflictReloading" @click="reloadConflict">{{ conflictReloading ? t("正在刷新…", "Refreshing…") : t("刷新并核对", "Refresh and review") }}</button></div>
      <section v-if="conflict && conflictIntent" class="rw-conflict"><p>{{ conflictRefreshed ? t("读取已成功完成。请人工核对当前状态，并明确确认丢弃原409请求后，才可创建新请求。", "Fresh reads succeeded. Review current state and explicitly confirm discarding the original 409 request before creating a new request.") : t("409原请求仍被冻结。请刷新相关读取；读到较新状态本身不会丢弃原请求。", "The original 409 request remains frozen. Refresh relevant reads; observing a newer state does not itself discard the request.") }}</p><label class="rw-confirm"><input data-testid="rewards-discard-reviewed" v-model="discardReviewed" type="checkbox" :disabled="!conflictRefreshed || conflictReloading" />{{ t("我已核对刷新后的状态，并确认丢弃原请求。", "I reviewed the refreshed state and confirm discarding the original request.") }}</label><div class="rw-actions"><button data-testid="rewards-conflict-refresh" class="button" type="button" :disabled="conflictReloading" @click="reloadConflict">{{ t("手动刷新当前状态", "Manually refresh current state") }}</button><button data-testid="rewards-conflict-discard" class="button button-primary" type="button" :disabled="!conflictRefreshed || !discardReviewed || conflictReloading" @click="discardConflictIntent">{{ t("确认丢弃并允许重新审核", "Confirm discard and allow new review") }}</button></div></section>
      <section v-if="receipt" class="rw-receipt" role="status" aria-live="polite" data-testid="rewards-receipt"><b>{{ t("操作回执（与当前查询分开）", "Operation receipt (separate from current queries)") }}</b><span>{{ operationLabel(receipt.operation) }} · {{ stateLabel(receipt.order.state) }} · v{{ receipt.order.version }}</span><code>{{ receipt.order.id }}</code><span>{{ t("回执原样保留；详情与历史来自后续读取。", "The original receipt is preserved; detail and history come from subsequent reads.") }}</span></section>
      <section v-if="dialog" class="rw-dialog" role="dialog" aria-modal="true" :aria-label="t('审核奖励操作', 'Review reward operation')" data-testid="rewards-review">
        <h2>{{ t("审核奖励操作", "Review reward operation") }}</h2><p>{{ t("请核对冻结的普通JSON请求快照。只有勾选确认后才会提交。", "Review the frozen ordinary JSON request snapshot. It is submitted only after explicit confirmation.") }}</p>
        <dl><dt>{{ t("品牌", "Brand") }}</dt><dd>{{ dialog.brandId }}</dd><dt>{{ t("操作 / 目标", "Operation / target") }}</dt><dd>{{ intentScopeLabel(dialog) }}</dd><template v-if="dialog.operation === 'grant'"><dt>{{ t("会员UUID", "Member UUID") }}</dt><dd>{{ (dialog.body as RewardGrantBody).member_id }}</dd><dt>{{ t("积分", "Points") }}</dt><dd>{{ (dialog.body as RewardGrantBody).points }}</dd></template><template v-else><dt>{{ t("版本", "Version") }}</dt><dd>v{{ (dialog.body as RewardActionBody).version }}</dd></template><dt>{{ t("原因", "Reason") }}</dt><dd>{{ dialog.body.reason }}</dd><dt>{{ t("幂等键", "Idempotency key") }}</dt><dd>{{ dialog.key }}</dd></dl>
        <details><summary>{{ t("查看冻结的普通JSON请求快照", "View frozen ordinary JSON request snapshot") }}</summary><pre>{{ reviewSnapshot(dialog) }}</pre></details>
        <label class="rw-confirm"><input data-testid="rewards-review-confirmed" v-model="confirmed" type="checkbox" />{{ t("我已核对以上请求并明确授权提交。", "I reviewed the request above and explicitly authorize submission.") }}</label>
        <div class="rw-actions"><button data-testid="rewards-confirm-submit" class="button button-primary" type="button" :disabled="writing || !confirmed" @click="submit(dialog)">{{ writing ? t("提交中…", "Submitting…") : t("确认提交", "Confirm and submit") }}</button><button class="button" type="button" :disabled="writing" @click="dialog = null; confirmed = false">{{ t("取消", "Cancel") }}</button></div>
      </section>
    </template>
    <p v-else class="rw-state" role="status">{{ t("没有奖励查看权限。", "Missing reward view permission.") }}</p>
  </article>
</template>

<style scoped>
.rw-dialog details{min-width:0}.rw-dialog pre{max-width:100%;white-space:pre-wrap;overflow-wrap:anywhere;word-break:break-word}
.rewards-management{display:grid;gap:18px;min-width:0}.rw-heading,.rw-section-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:16px}.rw-heading h1,.rw-section-heading h2,.rw-grant h2{margin:0}.rw-heading p,.rw-section-heading p,.rw-grant p{margin:6px 0 0;color:var(--muted,#64748b);font-size:13px;line-height:1.55}.rw-kicker{font-size:10px!important;letter-spacing:.14em;color:var(--accent,#2563eb)!important;font-weight:800}.rw-callout,.rw-error,.rw-success,.rw-state,.rw-receipt,.rw-conflict{padding:12px 14px;border-radius:10px;line-height:1.55}.rw-callout,.rw-conflict{border:1px solid var(--warning,#d97706);background:color-mix(in srgb,var(--warning,#d97706) 8%,transparent)}.rw-error{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;color:var(--danger,#b42318);background:color-mix(in srgb,var(--danger,#b42318) 8%,transparent)}.rw-success{color:var(--success,#15803d);background:color-mix(in srgb,var(--success,#15803d) 8%,transparent)}.rw-state{color:var(--muted,#64748b)}.rw-grant,.rw-detail,.rw-dialog{display:grid;gap:14px;padding:18px;border:1px solid var(--border,#dbe2ea);border-radius:14px;background:var(--surface,#fff)}.rw-grant label,.rw-reason{display:grid;gap:6px;font-weight:650}.rw-grant input,.rw-grant textarea,.rw-reason textarea{width:100%;padding:10px;border:1px solid var(--border,#cbd5e1);border-radius:8px;font:inherit}.rw-grant textarea,.rw-reason textarea{resize:vertical;min-height:60px}.rw-actions,.rw-pagination,.rw-pagination>div{display:flex;align-items:center;gap:8px;flex-wrap:wrap}.rw-pagination{justify-content:space-between}.rw-table-wrap{width:100%;overflow:auto;border:1px solid var(--border,#dbe2ea);border-radius:12px}.rw-table-wrap table{width:100%;border-collapse:collapse;min-width:850px}.rw-table-wrap th,.rw-table-wrap td{text-align:left;padding:11px 12px;border-bottom:1px solid var(--border,#e2e8f0);vertical-align:top}.rw-table-wrap th{font-size:11px;color:var(--muted,#64748b);background:var(--surface-muted,#f8fafc)}.rw-table-wrap tr:last-child td{border-bottom:0}.rw-table-wrap tr.selected{background:color-mix(in srgb,var(--accent,#2563eb) 7%,transparent)}.rw-table-wrap td small{display:block;margin-top:4px;color:var(--muted,#64748b);overflow-wrap:anywhere}.rw-link{padding:0;border:0;background:transparent;color:var(--accent,#2563eb);font:inherit;text-align:left;overflow-wrap:anywhere;cursor:pointer}.rw-facts,.rw-dialog dl{display:grid;grid-template-columns:minmax(160px,.35fr) minmax(0,1fr);gap:9px 14px;margin:0}.rw-facts dt,.rw-dialog dt{color:var(--muted,#64748b)}.rw-facts dd,.rw-dialog dd{margin:0;overflow-wrap:anywhere;word-break:break-word}.rw-history{display:grid;gap:12px;padding-top:16px;border-top:1px solid var(--border,#dbe2ea)}.rw-history h3{margin:0}.rw-receipt{display:grid;gap:5px;border:1px solid var(--success,#15803d);background:color-mix(in srgb,var(--success,#15803d) 7%,transparent)}.rw-receipt code{overflow-wrap:anywhere}.rw-dialog{max-width:720px;width:100%;margin-inline:auto;box-shadow:0 20px 60px #0f172a30}.rw-dialog h2{margin:0}.rw-dialog>p{color:var(--muted,#64748b)}.rw-confirm{display:flex;gap:9px;align-items:flex-start}.rw-confirm input{width:18px;height:18px;flex:0 0 auto;accent-color:var(--accent,#2563eb)}
@media(max-width:700px){.rw-heading,.rw-section-heading{align-items:stretch;flex-direction:column}.rw-heading>.button,.rw-section-heading>.button{align-self:flex-start}.rw-grant,.rw-detail{padding:14px}.rw-facts,.rw-dialog dl{grid-template-columns:1fr;gap:4px}.rw-facts dd,.rw-dialog dd{margin-bottom:8px}.rw-actions>.button{flex:1 1 100%}.rw-pagination{align-items:flex-start;flex-direction:column}.rw-error{align-items:flex-start;flex-direction:column}}
</style>
