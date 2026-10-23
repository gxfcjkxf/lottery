<script setup lang="ts">
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { selectAdminAssociation } from './member-lookup';
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import DetailModal from "@lottery/shared/DetailModal.vue"
import { computed, onUnmounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { adminKey, useAdminI18n } from "./i18n";
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
const listError = ref<string | import("@lottery/shared").LocalizedMessage>(""), detailError = ref<string | import("@lottery/shared").LocalizedMessage>(""), historyError = ref<string | import("@lottery/shared").LocalizedMessage>(""), writeError = ref<string | import("@lottery/shared").LocalizedMessage>(""), notice = ref("");
const memberUsername = ref("");
const grantMember = ref<{id: string; username: string} | null>(null);
const selectingMember = ref(false);
const memberID = ref(""), points = ref(""), grantReason = ref(""), actionReason = ref("");
const dialog = ref<PendingRewardIntent | null>(null), confirmed = ref(false);
const unknownIntent = ref<PendingRewardIntent | null>(null), conflictIntent = ref<PendingRewardIntent | null>(null);
const hasUnresolvedIntent = ref(false);
const conflict = ref(false), conflictReloading = ref(false), conflictRefreshed = ref(false), discardReviewed = ref(false);
let alive = true, listTicket = 0, detailTicket = 0, historyTicket = 0, writeTicket = 0, conflictTicket = 0;

const offset = computed(() => page.value?.offset ?? 0);
const hasPrevious = computed(() => offset.value > 0);
const hasNext = computed(() => page.value !== null && BigInt(page.value.offset + page.value.limit) < BigInt(page.value.total_count));
const validGrant = computed(() => Boolean(memberUsername.value.trim()) && validPoints(points.value) && validReason(grantReason.value));
const canGrant = computed(() => rights.value.grant && props.brandStatus !== "disabled" && !props.account.super_admin && !writing.value && !selectingMember.value && !hasUnresolvedIntent.value && !conflict.value && validGrant.value);
const canRevoke = computed(() => Boolean(rights.value.revoke && props.brandStatus !== "disabled" && !props.account.super_admin && !writing.value && !hasUnresolvedIntent.value && !conflict.value && detail.value?.state === "granted" && detail.value.version < Number.MAX_SAFE_INTEGER && validReason(actionReason.value)));
const canRetry = computed(() => Boolean(rights.value.retry && props.brandStatus !== "disabled" && !props.account.super_admin && !writing.value && !hasUnresolvedIntent.value && !conflict.value && detail.value?.state === "revocation_pending" && detail.value.version < Number.MAX_SAFE_INTEGER && validReason(actionReason.value)));
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const MAX_INT64 = 9_223_372_036_854_775_807n;

function validPoints(value: string): boolean {
  if (!/^[1-9][0-9]*$/.test(value)) return false;
  try { return BigInt(value) <= MAX_INT64; } catch { return false; }
}
function validReason(value: string): boolean {
  if (value.trim() !== value || new TextEncoder().encode(value).length > 500 || /[\u0000-\u001f\u007f-\u009f]/u.test(value)) return false;
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
  memberUsername.value = ""; grantMember.value = null; selectingMember.value = false;
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
  if (unknownIntent.value) writeError.value = t('admin.copy.400038e2');
  if (rights.value.view) void loadList(0);
}
function sessionExpired(cause: unknown, captured: string, session: number): boolean {
  if (cause instanceof AdminApiError && cause.status === 401 && alive && captured === scopeStamp.value && session === rewardSessionGeneration()) {
    listTicket++; detailTicket++; historyTicket++; writeTicket++; clearAllRewardIntents(); clearVisible(); emit("session-invalid"); return true;
  }
  return false;
}
function readFailure(cause: unknown): string | import("@lottery/shared").LocalizedMessage {
  return cause instanceof AdminApiError && cause.status === 0
    ? t('admin.copy.ce90986e')
    : cause instanceof Error ? publicErrorMessage(cause, "read") : t('admin.copy.466e55a6');
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
async function openReview(operation: RewardOperation) {
  if (operation === "grant") {
    if (!canGrant.value) return;
    const captured = scopeStamp.value, username = memberUsername.value;
    selectingMember.value = true; writeError.value = "";
    try {
      const member = await selectAdminAssociation(props.brandId, username, 'reward', 'member', 'grant');
      if (!alive || captured !== scopeStamp.value || username !== memberUsername.value) return;
      memberID.value = member.id; grantMember.value = {id: member.id, username: member.username};
    } catch (cause) {
      if (alive && captured === scopeStamp.value) writeError.value = publicErrorMessage(cause, "read");
      return;
    } finally { if (captured === scopeStamp.value) selectingMember.value = false; }
  }
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
    notice.value = t('admin.copy.10801157');
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
      writeError.value = t('admin.copy.c21709d2');
    } else if (outcome === "conflict") {
      markRewardIntentConflict(intent); unknownIntent.value = null; conflictIntent.value = intent; dialog.value = null; confirmed.value = false;
      conflict.value = true; conflictRefreshed.value = false; discardReviewed.value = false;
      writeError.value = t('admin.copy.f4b7d21d');
    } else {
      clearPendingRewardIntent(intent, intent.key); hasUnresolvedIntent.value = false; unknownIntent.value = null; dialog.value = null; confirmed.value = false;
      writeError.value = cause instanceof Error ? publicErrorMessage(cause) : t('admin.copy.7565cd98');
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
    writeError.value = t('admin.copy.aa2d8ea3');
  }
  if (ticket === conflictTicket) conflictReloading.value = false;
}
function discardConflictIntent() {
  const intent = conflictIntent.value;
  if (!intent || !conflictRefreshed.value || !discardReviewed.value || conflictReloading.value) return;
  clearPendingRewardIntent(intent, intent.key); conflictIntent.value = null; conflict.value = false;
  hasUnresolvedIntent.value = false;
  conflictRefreshed.value = false; discardReviewed.value = false; writeError.value = "";
  notice.value = t('admin.copy.e65fbf40');
}
function stateLabel(state: RewardOrder["state"]): string {
  const labels: Record<RewardOrder["state"], string> = { granted: 'admin.copy.86b01b91', revocation_pending: 'admin.copy.2b729ea1', revoked: 'admin.copy.176004df' };
  return t(adminKey(labels[state]));
}
function operationLabel(operation: RewardOperation): string {
  const labels: Record<RewardOperation, string> = { grant: 'admin.copy.cbf460c0', revoke: 'admin.copy.ce100631', retry: 'admin.copy.4c4b23d9' };
  return t(adminKey(labels[operation]));
}
function formatDate(value: string): string{ return formatDisplayDateTime(value, displayLocale.value); }
function nullable(value: string | null): string { return value ?? t('admin.copy.c2b41b33'); }
function intentScopeLabel(intent: PendingRewardIntent): string { return intent.targetId ? `${operationLabel(intent.operation)} · ${intent.targetId}` : operationLabel(intent.operation); }

watch(scopeStamp, resetScope, { immediate: true });
  onUnmounted(() => { alive = false; listTicket++; detailTicket++; historyTicket++; writeTicket++; conflictTicket++; });
function closeReview() {
  if (writing.value) return;
  dialog.value = null; confirmed.value = false;
}
function closeDetails() {
  detailTicket++; historyTicket++; selected.value = null; detail.value = null; history.value = null; loadingDetail.value = false; loadingHistory.value = false; detailError.value = '';
}
</script>

<template>
  <article class="panel rewards-management">
    <header class="rw-heading"><div><h1>{{ t('admin.copy.23480b24') }}</h1></div></header>
    <template v-if="rights.view">
      <p v-if="brandStatus === 'disabled'" class="rw-callout" role="status">{{ t('admin.copy.1d27eb27') }}</p>
      <p v-else-if="brandStatus === 'paused'" class="rw-callout" role="status">{{ t('admin.copy.5606cd3a') }}</p>

      <section class="rw-grant">
        <div><h2>{{ t('admin.copy.b90c3979') }}</h2></div>
        <label>{{ t('admin.reward.memberUsername') }}<input data-testid="rewards-grant-member" v-model="memberUsername" autocomplete="off" :disabled="!rights.grant || writing || selectingMember || Boolean(unknownIntent) || conflict" /></label>
        <label>{{ t('admin.copy.ec082fa7') }}<input data-testid="rewards-grant-points" v-model="points" inputmode="numeric" autocomplete="off" :disabled="!rights.grant || writing || selectingMember || Boolean(unknownIntent) || conflict" /></label>

        <button data-testid="rewards-grant-review" class="button button-primary" type="button" :disabled="!canGrant" @click="withGlobalLoading(openReview, 'grant')">{{ t('admin.copy.da40d64c') }}</button>
      </section>

      <div class="rw-section-heading"><div><h2>{{ t('admin.copy.98f3f625') }}</h2></div><button v-refresh-button="loadingList ? t('admin.copy.27342bf3') : t('admin.copy.855b9442')" data-testid="rewards-list-refresh" class="button" type="button" :disabled="loadingList" @click="withGlobalLoading(loadList, 0)">{{ loadingList ? t('admin.copy.27342bf3') : t('admin.copy.855b9442') }}</button></div>
      <div v-if="listError" class="rw-error" role="alert">{{ displayPublicError(listError) }}</div>
      <p v-global-loading v-else-if="loadingList && !page" class="rw-state" role="status">{{ t('admin.copy.643f4147') }}</p>
      <p v-else-if="page?.items.length === 0" class="rw-state empty-state">{{ t('admin.copy.7a433aee') }}</p>
      <div v-if="page?.items.length" class="rw-table-wrap"><table><thead><tr><th>{{ t('admin.copy.09cc2b66') }}</th><th>{{ t('admin.copy.cf6d9c6d') }}</th><th>{{ t('admin.copy.ec082fa7') }}</th><th>{{ t('admin.copy.5ec7c8ba') }}</th><th>{{ t('admin.copy.19bf47ac') }}</th></tr></thead><tbody><tr v-for="item in page.items" :key="item.id" :class="{ selected: detail?.id === item.id }"><td><button class="rw-link" type="button" @click="withGlobalLoading(selectOrder, item.id)">{{ item.id }}</button><small><AssociationName :record="item" kind="member" :id="item.member_id" :locale="associationLocale" /></small></td><td>{{ stateLabel(item.state) }}</td><td>{{ item.points }}</td><td>{{ item.reason }}</td><td>{{ formatDate(item.updated_at) }}</td></tr></tbody></table></div>
      <div v-if="page" class="rw-pagination"><span>{{ page.items.length ? page.offset + 1 : 0 }}–{{ page.offset + page.items.length }} / {{ page.total_count }}</span><div><button class="button" type="button" :disabled="!hasPrevious || loadingList" @click="withGlobalLoading(loadList, Math.max(0, offset - PAGE_SIZE))">{{ t('admin.copy.dbe7a5d7') }}</button><button class="button" type="button" :disabled="!hasNext || loadingList" @click="withGlobalLoading(loadList, offset + PAGE_SIZE)">{{ t('admin.copy.2e9c9e7a') }}</button></div></div>

      <DetailModal v-if="selected || detail || loadingDetail" :open="!dialog" :title="t('admin.copy.01db40da')" :close-label="t('admin.copy.2bb2cb1d')" @close="closeDetails">
<section v-global-loading v-if="loadingDetail" class="rw-detail rw-state" role="status">{{ t('admin.copy.c10840f3') }}</section>
      <section v-else-if="detail" class="rw-detail">
        <div class="rw-section-heading"><div><h2>{{ t('admin.copy.5fea6596') }}</h2><p>{{ stateLabel(detail.state) }}</p></div><button v-refresh-button="t('admin.copy.2afd7606')" data-testid="rewards-detail-refresh" class="button" type="button" :disabled="loadingDetail" @click="withGlobalLoading(loadDetail, detail.id)">{{ t('admin.copy.2afd7606') }}</button></div>
        <div v-if="detailError" class="rw-error" role="alert">{{ displayPublicError(detailError) }}</div>
        <dl class="rw-facts"><dt>ID</dt><dd>{{ detail.id }}</dd><dt>{{ t('admin.copy.2bb1f2b5') }}</dt><dd><AssociationName :record="detail" kind="brand" :id="detail.brand_id" :locale="associationLocale" /><br /><AssociationName :record="detail" kind="member" :id="detail.member_id" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.b993a315') }}</dt><dd>{{ detail.points }} · {{ stateLabel(detail.state) }}</dd><dt>{{ t('admin.copy.a69844be') }}</dt><dd>{{ detail.grant_ledger_entry_id }}<br />{{ nullable(detail.revoke_ledger_entry_id) }}</dd><dt>{{ t('admin.copy.c0a94db8') }}</dt><dd>{{ detail.last_error_code ? displayPublicError(publicErrorMessage({ code: detail.last_error_code })) : t('admin.copy.c2b41b33') }}</dd><dt>{{ t('admin.copy.373c5abf') }}</dt><dd><AssociationName :record="detail" kind="admin" :id="detail.created_by" :locale="associationLocale" /><br />{{ detail.reason }}</dd><dt>{{ t('admin.copy.9efac6c5') }}</dt><dd>{{ formatDate(detail.created_at) }}<br />{{ formatDate(detail.updated_at) }}<br />{{ detail.revoked_at ? formatDate(detail.revoked_at) : t('admin.copy.c2b41b33') }}</dd></dl>
        <p v-if="detail.state === 'revocation_pending'" class="rw-callout" role="note">{{ t('admin.copy.5c474057') }}</p>

        <div class="rw-actions"><button v-if="detail.state === 'granted'" data-testid="rewards-revoke" class="button button-primary" type="button" :disabled="!canRevoke" @click="withGlobalLoading(openReview, 'revoke')">{{ t('admin.copy.50cba90d') }}</button><button v-if="detail.state === 'revocation_pending'" data-testid="rewards-retry" class="button button-primary" type="button" :disabled="!canRetry" @click="withGlobalLoading(openReview, 'retry')">{{ t('admin.copy.9042cec1') }}</button></div>

        <section class="rw-history"><div class="rw-section-heading"><div><h3>{{ t('admin.copy.07e3ef40') }}</h3></div><button v-refresh-button="loadingHistory ? t('admin.copy.27342bf3') : t('admin.copy.4f5df8c1')" data-testid="rewards-history-refresh" class="button" type="button" :disabled="loadingHistory" @click="withGlobalLoading(loadHistory, detail.id, 0)">{{ loadingHistory ? t('admin.copy.27342bf3') : t('admin.copy.4f5df8c1') }}</button></div>
          <div v-if="historyError" class="rw-error" role="alert">{{ displayPublicError(historyError) }}</div><p v-global-loading v-else-if="loadingHistory && !history" class="rw-state" role="status">{{ t('admin.copy.90608350') }}</p><p v-else-if="history?.items.length === 0" class="rw-state empty-state">{{ t('admin.copy.a6f853c8') }}</p>
          <div v-if="history?.items.length" class="rw-table-wrap"><table><thead><tr><th>{{ t('admin.copy.41b787d3') }}</th><th>{{ t('admin.copy.0e735405') }}</th><th>{{ t('admin.copy.ade69be3') }}</th><th>{{ t('admin.copy.96593f4b') }}</th><th>{{ t('admin.copy.0b62f6c4') }}</th></tr></thead><tbody><tr v-for="action in history.items" :key="action.id"><td>{{ operationLabel(action.operation) }}<small>{{ action.id }}</small></td><td>{{ action.state_before ? stateLabel(action.state_before) : t('admin.copy.c2b41b33') }} → {{ stateLabel(action.state_after) }}</td><td><AssociationName :record="action" kind="actor" :id="action.actor_id" :locale="associationLocale" /><small>{{ action.reason }}</small></td><td><small>{{ nullable(action.ledger_entry_id) }}</small></td><td>{{ formatDate(action.created_at) }}</td></tr></tbody></table></div>
          <div v-if="history" class="rw-pagination"><span>{{ history.items.length ? history.offset + 1 : 0 }}–{{ history.offset + history.items.length }} / {{ history.total_count }}</span><div><button class="button" type="button" :disabled="history.offset === 0 || loadingHistory" @click="withGlobalLoading(loadHistory, detail.id, Math.max(0, history.offset - PAGE_SIZE))">{{ t('admin.copy.dbe7a5d7') }}</button><button class="button" type="button" :disabled="BigInt(history.offset + history.limit) >= BigInt(history.total_count) || loadingHistory" @click="withGlobalLoading(loadHistory, detail.id, history.offset + PAGE_SIZE)">{{ t('admin.copy.2e9c9e7a') }}</button></div></div>
        </section>
      </section>
</DetailModal>

      <p v-if="unknownIntent && !writeError" class="rw-error">{{ t('admin.copy.3e878895') }} <button v-write-guard="writing" type="button" class="button" :disabled="writing || !permissionFor(unknownIntent.operation) || brandStatus === 'disabled'" @click="retryUnknown">{{ t('admin.copy.c6a6aa0c') }}</button></p>
      <div v-if="notice" class="rw-success" role="status">{{ notice }}</div>
      <div v-if="writeError" class="rw-error" role="alert">{{ displayPublicError(writeError) }}<button v-write-guard="writing" v-if="unknownIntent && rights.view" type="button" class="button" :disabled="writing || !permissionFor(unknownIntent.operation) || brandStatus === 'disabled'" @click="retryUnknown">{{ writing ? t('admin.copy.a3df712a') : t('admin.copy.c6a6aa0c') }}</button><button v-refresh-button="conflictReloading ? t('admin.copy.948fc498') : t('admin.copy.49614a57')" v-if="conflict" type="button" class="button" :disabled="conflictReloading" @click="withGlobalLoading(reloadConflict)">{{ conflictReloading ? t('admin.copy.948fc498') : t('admin.copy.49614a57') }}</button></div>
      <section v-if="conflict && conflictIntent" class="rw-conflict"><p>{{ conflictRefreshed ? t('admin.copy.6351eb6f') : t('admin.copy.3aefe357') }}</p><label class="rw-confirm"><input data-testid="rewards-discard-reviewed" v-model="discardReviewed" type="checkbox" :disabled="!conflictRefreshed || conflictReloading" />{{ t('admin.copy.3523267a') }}</label><div class="rw-actions"><button v-refresh-button="t('admin.copy.16a3182a')" data-testid="rewards-conflict-refresh" class="button" type="button" :disabled="conflictReloading" @click="withGlobalLoading(reloadConflict)">{{ t('admin.copy.16a3182a') }}</button><button data-testid="rewards-conflict-discard" class="button button-primary" type="button" :disabled="!conflictRefreshed || !discardReviewed || conflictReloading" @click="discardConflictIntent">{{ t('admin.copy.7107e7f5') }}</button></div></section>
      <section v-if="receipt" class="rw-receipt" role="status" aria-live="polite" data-testid="rewards-receipt"><b>{{ t('admin.copy.f8caefd7') }}</b><span>{{ operationLabel(receipt.operation) }} · {{ stateLabel(receipt.order.state) }}</span><code>{{ receipt.order.id }}</code><span>{{ t('admin.copy.a77616f0') }}</span></section>
      <DetailModal v-if="dialog" :title="t('admin.copy.bb763c46')" :close-label="t('admin.copy.2bb2cb1d')" @close="closeReview">
      <section class="rw-dialog" data-testid="rewards-review">
        <dl><dt>{{ t('admin.copy.cef73d0f') }}</dt><dd><AssociationName kind="admin" :id="dialog.accountId" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.9c32661d') }}</dt><dd><AssociationName kind="brand" :id="dialog.brandId" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.087a786d') }}</dt><dd>{{ intentScopeLabel(dialog) }}</dd><template v-if="dialog.operation === 'grant'"><dt>{{ t('admin.reward.memberUsername') }}</dt><dd v-if="(dialog.body as RewardGrantBody).member_id === grantMember?.id">{{ grantMember.username }}</dd><dd v-else><AssociationName :record="dialog.body" kind="member" :id="(dialog.body as RewardGrantBody).member_id" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.ec082fa7') }}</dt><dd>{{ (dialog.body as RewardGrantBody).points }}</dd></template><dt v-if="dialog.body.reason">{{ t('admin.copy.5ec7c8ba') }}</dt><dd v-if="dialog.body.reason">{{ dialog.body.reason }}</dd></dl>
        <details><summary>{{ t('admin.copy.518063e0') }}</summary><BusinessDetails :value="dialog.body" :locale="associationLocale" /></details>
        <label class="rw-confirm"><input data-testid="rewards-review-confirmed" v-model="confirmed" type="checkbox" />{{ t('admin.copy.56ec219c') }}</label>
        <div class="rw-actions"><button v-write-guard="writing" data-testid="rewards-confirm-submit" class="button button-primary" type="button" :disabled="writing || !confirmed" @click="withGlobalLoading(submit, dialog)">{{ writing ? t('admin.copy.46511dc0') : t('admin.copy.02c1e356') }}</button><button class="button" type="button" :disabled="writing" @click="closeReview">{{ t('admin.copy.e2f391e7') }}</button></div>
      </section>
      </DetailModal>
    </template>
    <p v-else class="rw-state" role="status">{{ t('admin.copy.2ea8d38e') }}</p>
  </article>
</template>

<style scoped>
.rw-dialog details{min-width:0}.rw-dialog pre{max-width:100%;white-space:pre-wrap;overflow-wrap:anywhere;word-break:break-word}
.rewards-management{display:grid;gap:18px;min-width:0}.rw-heading,.rw-section-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:16px}.rw-heading h1,.rw-section-heading h2,.rw-grant h2{margin:0}.rw-heading p,.rw-section-heading p,.rw-grant p{margin:6px 0 0;color:var(--muted);font-size:13px;line-height:1.55}.rw-kicker{font-size:11px!important;letter-spacing:.14em;color:var(--accent)!important;font-weight:800}.rw-callout,.rw-error,.rw-success,.rw-state,.rw-receipt,.rw-conflict{padding:12px 14px;border-radius:10px;line-height:1.55}.rw-callout,.rw-conflict{border:1px solid var(--warning);background:color-mix(in srgb,var(--warning) 8%,transparent)}.rw-error{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;color:var(--danger);background:color-mix(in srgb,var(--danger) 8%,transparent)}.rw-success{color:var(--success);background:color-mix(in srgb,var(--success) 8%,transparent)}.rw-state{color:var(--muted)}.rw-grant,.rw-detail,.rw-dialog{display:grid;gap:14px;padding:18px;border:1px solid var(--border,var(--line));border-radius:14px;background:var(--surface)}.rw-grant label,.rw-reason{display:grid;gap:6px;font-weight:650}.rw-grant input,.rw-grant textarea,.rw-reason textarea{width:100%;padding:10px;border:1px solid var(--border,#cbd5e1);border-radius:8px;font:inherit}.rw-grant textarea,.rw-reason textarea{resize:vertical;min-height:60px}.rw-actions,.rw-pagination,.rw-pagination>div{display:flex;align-items:center;gap:8px;flex-wrap:wrap}.rw-pagination{justify-content:space-between}.rw-table-wrap{width:100%;overflow:auto;border:1px solid var(--border,var(--line));border-radius:12px}.rw-table-wrap table{width:100%;border-collapse:collapse;min-width:850px}.rw-table-wrap th,.rw-table-wrap td{text-align:left;padding:11px 12px;border-bottom:1px solid var(--border,var(--line));vertical-align:top}.rw-table-wrap th{font-size:11px;color:var(--muted);background:var(--surface-muted,var(--canvas))}.rw-table-wrap tr:last-child td{border-bottom:0}.rw-table-wrap tr.selected{background:color-mix(in srgb,var(--accent) 7%,transparent)}.rw-table-wrap td small{display:block;margin-top:4px;color:var(--muted);overflow-wrap:anywhere}.rw-link{padding:0;border:0;background:transparent;color:var(--accent);font:inherit;text-align:left;overflow-wrap:anywhere;cursor:pointer}.rw-facts,.rw-dialog dl{display:grid;grid-template-columns:minmax(160px,.35fr) minmax(0,1fr);gap:9px 14px;margin:0}.rw-facts dt,.rw-dialog dt{color:var(--muted)}.rw-facts dd,.rw-dialog dd{margin:0;overflow-wrap:anywhere;word-break:break-word}.rw-history{display:grid;gap:12px;padding-top:16px;border-top:1px solid var(--border,var(--line))}.rw-history h3{margin:0}.rw-receipt{display:grid;gap:5px;border:1px solid var(--success);background:color-mix(in srgb,var(--success) 7%,transparent)}.rw-receipt code{overflow-wrap:anywhere}.rw-dialog{width:100%;border:0;padding:0;box-shadow:none}.rw-dialog h2{margin:0}.rw-dialog>p{color:var(--muted)}.rw-confirm{display:flex;gap:9px;align-items:flex-start}.rw-confirm input{width:18px;height:18px;flex:0 0 auto;accent-color:var(--accent)}
@media(max-width:700px){.rw-heading,.rw-section-heading{align-items:stretch;flex-direction:column}.rw-heading>.button,.rw-section-heading>.button{align-self:flex-start}.rw-grant,.rw-detail{padding:14px}.rw-facts,.rw-dialog dl{grid-template-columns:1fr;gap:4px}.rw-facts dd,.rw-dialog dd{margin-bottom:8px}.rw-actions>.button{flex:1 1 100%}.rw-pagination{align-items:flex-start;flex-direction:column}.rw-error{align-items:flex-start;flex-direction:column}}
</style>
