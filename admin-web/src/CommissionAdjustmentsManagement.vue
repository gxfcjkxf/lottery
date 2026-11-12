<script setup lang="ts">
import { isPoints } from "@lottery/shared";
import { pointsFromMinor } from '@lottery/shared'

import { pointsMinor } from '@lottery/shared'

const filterReset = ref(0)

import MemberActionsMenu from "@lottery/shared/MemberActionsMenu.vue"
import ManagementList from "@lottery/shared/ManagementList.vue"
import TablePagination from "@lottery/shared/TablePagination.vue"
const agentDraftSearchPending = ref(false);
import FilterAssociationSearch from "@lottery/shared/FilterAssociationSearch.vue";
import ManagementTable from '@lottery/shared/ManagementTable.vue';
import { useOperationToast } from "@lottery/shared";
import DetailModal from "@lottery/shared/DetailModal.vue"
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
import { vInlineValidation } from '@lottery/shared'
import { computed, onUnmounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { useAdminI18n } from "./i18n";
import { commissionAdjustmentPermissions, createCommissionAdjustmentsApi, validCommissionAdjustmentDelta, validCommissionAdjustmentPoints, validCommissionAdjustmentReason, type CommissionAdjustment, type CommissionAdjustmentTarget } from "./commission-adjustments-api";
import { classifyCommissionAdjustmentFailure, clearAllPendingCommissionAdjustments, clearPendingCommissionAdjustmentIntent, commissionAdjustmentSessionGeneration, createCommissionAdjustmentIntent, getPendingCommissionAdjustmentIntent, isCommissionAdjustmentConflict, listPendingCommissionAdjustmentIntents, markCommissionAdjustmentConflict, setPendingCommissionAdjustmentIntent, type CommissionAdjustmentIntentScope, type PendingCommissionAdjustmentIntent } from "./commission-adjustments-state";
const historyPageSize = ref(20)
const targetPageSize = ref(20)


const props = defineProps<{ account: AdminAccount; brandId: string; paymentId: string; paymentState: string; brandStatus?: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, message } = useAdminI18n();
const api = createCommissionAdjustmentsApi(fetch);

const SCAN_PAGE = 100;
const rights = computed(() => commissionAdjustmentPermissions(props.account, props.brandId));
const stamp = computed(() => JSON.stringify([props.account.id, props.account.super_admin, props.account.brand_ids, props.account.permissions, props.account.permissions_by_brand, props.account.platform_permissions, props.brandId, props.paymentId, props.paymentState, props.brandStatus, rights.value]));
const targetColumns=computed(()=>[
 {key:'agent',label:t('admin.commission.ui.agent')}, {key:'original',label:t('admin.commission.ui.original'),cellClass:'numeric'},
 {key:'adjusted',label:t('admin.commission.ui.adjusted'),cellClass:'numeric'}, {key:'actions',label:t('admin.commission.ui.actions')},
]);
const agentDraft = ref(''), agentFilter = ref('');
const targets = ref<CommissionAdjustmentTarget[]>([]), targetTotal = ref("0"), targetOffset = ref(0), targetLimit = ref(targetPageSize.value), selectedId = ref("");
const history = ref<CommissionAdjustment[]>([]), historyTotal = ref("0"), historyOffset = ref(0);
const loading = ref(false), historyLoading = ref(false), writing = ref(false);
const error = ref<string | import("@lottery/shared").LocalizedMessage>(""), historyError = ref<string | import("@lottery/shared").LocalizedMessage>(""), writeError = ref<string | import("@lottery/shared").LocalizedMessage>(""), notice = ref<string | import("@lottery/shared").LocalizedMessage>("");
const points = ref(""), reason = ref(""), dialog = ref<PendingCommissionAdjustmentIntent | null>(null), confirmed = ref(false);
const unknown = ref<PendingCommissionAdjustmentIntent | null>(null), conflict = ref<PendingCommissionAdjustmentIntent | null>(null);
const conflictRefreshed = ref(false), conflictTargetMissing = ref(false), discardChecked = ref(false), conflictRefreshBusy = ref(false);
const receipt = ref<CommissionAdjustment | null>(null);
let alive = true, generation = 0, targetTicket = 0, historyTicket = 0, writeTicket = 0, readLane = 0, conflictTicket = 0;
const selected = computed(() => targets.value.find((row) => row.id === selectedId.value) ?? null);
const available = computed(() => Boolean(rights.value.write && agentDraft.value === agentFilter.value && props.brandStatus !== "disabled" && props.paymentState === "paid" && selected.value?.state === "paid" && selected.value.adjustment_version !== null && selected.value.adjustment_version < Number.MAX_SAFE_INTEGER && !writing.value && !unknown.value && !conflict.value && !loading.value && !historyLoading.value && !error.value && !historyError.value));
const reasonValid = computed(() => validCommissionAdjustmentReason(reason.value.trim()));
const deltaPreview = computed(() => { if (!selected.value || !isPoints(points.value)) return "";
  try { const delta = pointsMinor(points.value) - pointsMinor(selected.value.adjusted_points ?? "0"); return pointsFromMinor(delta); } catch { return ""; } });
const canPrepare = computed(() => available.value && Boolean(selected.value && !getPendingCommissionAdjustmentIntent(scopeFor(selected.value.id))) && validCommissionAdjustmentPoints(points.value) && Boolean(selected.value && validCommissionAdjustmentDelta(selected.value.adjusted_points ?? "0",points.value)) && reasonValid.value);
const hasTargetNext = computed(() => BigInt(targetOffset.value + targets.value.length) < BigInt(targetTotal.value));
const hasHistoryNext = computed(() => BigInt(historyOffset.value + history.value.length) < BigInt(historyTotal.value));
const targetsReady = ref(false);
const historyReady = ref(false);
const scopeFor = (targetId: string): CommissionAdjustmentIntentScope => ({ accountId: props.account.id, brandId: props.brandId, paymentId: props.paymentId, targetId });
function active(_ticket: number, captured: string, session: number) { return alive && captured === stamp.value && session === commissionAdjustmentSessionGeneration() && rights.value.view; }
function readFailure(cause: unknown) { return cause instanceof Error ? publicErrorMessage(cause) : t('admin.copy.466e55a6'); }
function reset() {
  detailOpen.value=false;
  targetsReady.value=historyReady.value=false;
  generation++; readLane++; targetTicket++; historyTicket++; writeTicket++; conflictTicket++;
  agentDraft.value = agentFilter.value = ""; targets.value = []; history.value = []; targetTotal.value = historyTotal.value = "0"; targetOffset.value = historyOffset.value = 0; targetLimit.value = targetPageSize.value; selectedId.value = ""; points.value = ""; reason.value = ""; dialog.value = null; confirmed.value = false; error.value = historyError.value = writeError.value = notice.value = ""; loading.value = historyLoading.value = writing.value = false; conflictRefreshed.value = conflictTargetMissing.value = discardChecked.value = conflictRefreshBusy.value = false; receipt.value = null;
  const pending = listPendingCommissionAdjustmentIntents(props.account.id, props.brandId);
  const forPayment = pending.filter((item) => item.paymentId === props.paymentId);
  conflict.value = forPayment.find(isCommissionAdjustmentConflict) ?? null; unknown.value = conflict.value ? null : forPayment[0] ?? null;
  if (unknown.value) { selectedId.value = unknown.value.targetId; writeError.value = t('admin.copy.ecefab0c'); }
  if (rights.value.view) void loadTargets(0, unknown.value?.targetId);
}
function sessionExpired(cause: unknown, captured: string, session: number) {
  if (cause instanceof AdminApiError && cause.status === 401 && captured === stamp.value && session === commissionAdjustmentSessionGeneration()) {
    clearAllPendingCommissionAdjustments(); generation++; readLane++; targetTicket++; historyTicket++; writeTicket++; conflictTicket++;
    targets.value = []; history.value = []; targetTotal.value = historyTotal.value = "0"; selectedId.value = ""; unknown.value = null; conflict.value = null; dialog.value = null; receipt.value = null;
    loading.value = historyLoading.value = writing.value = false; emit("session-invalid"); return true;
  }
  return false;
}
async function loadTargets(offset = targetOffset.value, locateTargetId?: string) {
  if (!rights.value.view) return;
  conflictTicket++; conflictRefreshBusy.value = false; conflictRefreshed.value = false; discardChecked.value = false;
  const ticket = ++targetTicket, captured = stamp.value, session = commissionAdjustmentSessionGeneration(), lane = ++readLane; loading.value = true; error.value = "";
  try {
    if (locateTargetId) agentDraft.value = agentFilter.value = "";
    let page = await api.listTargets(props.brandId, props.paymentId, targetPageSize.value, offset, agentFilter.value ? {agent_id: agentFilter.value} : {});
    if (!active(ticket, captured, session) || lane !== readLane) return;
    if (locateTargetId && !page.items.some((row) => row.id === locateTargetId)) {
      let scanOffset = 0, located = false;
      while (true) {
        const scan = await api.listTargets(props.brandId,props.paymentId,SCAN_PAGE,scanOffset);
        if (!active(ticket,captured,session) || lane !== readLane) return;
        const index = scan.items.findIndex((row) => row.id === locateTargetId);
        if (index >= 0) {
          const desiredOffset = scan.offset + Math.floor(index / targetPageSize.value) * targetPageSize.value;
          page = await api.listTargets(props.brandId,props.paymentId,targetPageSize.value,desiredOffset);
          if (!active(ticket,captured,session) || lane !== readLane) return;
          located = page.items.some((row) => row.id === locateTargetId);
          break;
        }
        scanOffset += scan.limit;
        if (!scan.items.length || BigInt(scanOffset) >= BigInt(scan.total_count)) break;
      }
      if (!located) notice.value = message('admin.copy.b18927d9');
    }
    const previousId = selectedId.value;
    targetsReady.value=true; targets.value = page.items; targetTotal.value = page.total_count; targetOffset.value = page.offset; targetLimit.value = page.limit;
    if (locateTargetId && page.items.some((row) => row.id === locateTargetId)) selectedId.value = locateTargetId;
    else if (!page.items.some((row) => row.id === selectedId.value)) selectedId.value = page.items[0]?.id ?? "";
    if (selectedId.value !== previousId) { historyReady.value=false; historyTicket++; history.value = []; historyTotal.value = "0"; historyOffset.value = 0; historyError.value = ""; points.value = reason.value = ""; }
    if (selectedId.value) void loadHistory(selectedId.value);
  } catch (cause) { if (active(ticket, captured, session) && lane === readLane) { sessionExpired(cause,captured,session); error.value = readFailure(cause); } }
  finally { if (active(ticket,captured,session) && ticket === targetTicket) loading.value = false; }
}
async function loadHistory(id: string, offset = historyOffset.value) {
  if (!rights.value.view || !id) return;
  conflictTicket++; conflictRefreshBusy.value = false; conflictRefreshed.value = false; discardChecked.value = false;
  const ticket = ++historyTicket, captured = stamp.value, session = commissionAdjustmentSessionGeneration(), lane = readLane; historyLoading.value = true; historyError.value = "";
  try {
    const page = await api.listAdjustments(props.brandId,id,historyPageSize.value,offset);
    if (!active(generation,captured,session) || ticket !== historyTicket || lane !== readLane || selectedId.value !== id || page.items.some((item) => item.target_id !== id || item.payment_id !== props.paymentId)) throw new AdminApiError("佣金修正历史响应范围无效。", 502, "INVALID_RESPONSE");
    historyReady.value=true; history.value = page.items; historyTotal.value = page.total_count; historyOffset.value = page.offset;
  } catch (cause) { if (active(generation,captured,session) && ticket === historyTicket && lane === readLane) { sessionExpired(cause,captured,session); historyError.value = readFailure(cause); } }
  finally { if (ticket === historyTicket && active(generation,captured,session)) historyLoading.value = false; }
}
const detailOpen=ref(false)
async function chooseTarget(id: string) {
  detailOpen.value=true;
  historyReady.value=false; selectedId.value = id; points.value = ""; reason.value = ""; history.value = []; historyTotal.value = "0"; historyOffset.value = 0; dialog.value = null; receipt.value = null;
  const pending = getPendingCommissionAdjustmentIntent(scopeFor(id));
  conflict.value = pending && isCommissionAdjustmentConflict(pending) ? pending : null;
  unknown.value = pending && !conflict.value ? pending : null;
  await loadHistory(id,0);
}
async function searchTargets() {
  if (agentDraftSearchPending.value) return;
  agentFilter.value = agentDraft.value; selectedId.value = ""; points.value = ""; history.value = []; historyTotal.value = "0";
  await loadTargets(0);
}

async function pageTargets(next: number) { if (next >= 0 && BigInt(next) < BigInt(targetTotal.value)) await loadTargets(next); }
async function pageHistory(next: number) { if (next >= 0 && BigInt(next) < BigInt(historyTotal.value) && selectedId.value) await loadHistory(selectedId.value,next); }
function prepare() {
  if (!canPrepare.value || !selected.value) return;
  const intent = createCommissionAdjustmentIntent(scopeFor(selected.value.id), props.account.id, selected.value.adjusted_points!, { version: selected.value.adjustment_version!, points: points.value, reason: reason.value.trim() }, createIdempotencyKey());
  if (!intent) return; dialog.value = intent; confirmed.value = false; writeError.value = "";
}
function recover() { const intent = unknown.value; if (!intent || !rights.value.write || props.brandStatus === "disabled") return; dialog.value = intent; confirmed.value = false; }
function validScope(intent: PendingCommissionAdjustmentIntent) { return intent.accountId === props.account.id && intent.brandId === props.brandId && intent.paymentId === props.paymentId && intent.actorId === props.account.id; }
async function submit() {
  const intent = dialog.value; if (!intent || !confirmed.value || writing.value || !validScope(intent) || !rights.value.write || props.brandStatus === "disabled") return;
  const recovery = unknown.value?.key === intent.key;
  if (!recovery && (!selected.value || selected.value.id !== intent.targetId || selected.value.adjustment_version !== intent.body.version || !canPrepare.value)) return;
  const intentScope = scopeFor(intent.targetId), existing = getPendingCommissionAdjustmentIntent(intentScope);
  if (existing && existing.key !== intent.key) { writeError.value = t('admin.copy.6eaf20aa'); return; }
  if (!setPendingCommissionAdjustmentIntent(intentScope,intent)) return;
  const captured = stamp.value, session = commissionAdjustmentSessionGeneration(), ticket = ++writeTicket; writing.value = true; writeError.value = "";
  try {
    const acknowledged = await api.createAdjustment(intent.brandId,intent.targetId,intent.paymentId,intent.body,intent.actorId,intent.key);
    clearPendingCommissionAdjustmentIntent(scopeFor(intent.targetId),intent.key);
    if (!active(ticket,captured,session) || ticket !== writeTicket) return;
    receipt.value = acknowledged; dialog.value = null; unknown.value = null; confirmed.value = false; notice.value = message('admin.copy.61bac1f8');
    // Reload targets first; its own follow-up reads the selected history.
    // Concurrent target/history refreshes must not race the same read lane.
    await loadTargets(targetOffset.value, intent.targetId);
  } catch (cause) {
    const status = cause instanceof AdminApiError ? cause.status : undefined, result = classifyCommissionAdjustmentFailure(status);
    if (!active(ticket,captured,session) || ticket !== writeTicket) { if (result === "definitive") clearPendingCommissionAdjustmentIntent(scopeFor(intent.targetId),intent.key); return; }
    if (sessionExpired(cause,captured,session)) { clearPendingCommissionAdjustmentIntent(scopeFor(intent.targetId),intent.key); return; }
    if (result === "unknown") { setPendingCommissionAdjustmentIntent(scopeFor(intent.targetId),intent); unknown.value = intent; dialog.value = null; writeError.value = t('admin.copy.cf50fe97'); }
    else if (result === "conflict") { markCommissionAdjustmentConflict(intent); unknown.value = null; conflict.value = intent; dialog.value = null; conflictRefreshed.value = discardChecked.value = false; writeError.value = t('admin.copy.948e3901'); }
    else { clearPendingCommissionAdjustmentIntent(scopeFor(intent.targetId),intent.key); dialog.value = null; writeError.value = cause instanceof Error ? publicErrorMessage(cause) : t('admin.copy.7565cd98'); }
  } finally { if (active(ticket,captured,session)) writing.value = false; }
}
async function refreshConflict() {
  const intent = conflict.value; if (!intent || conflictRefreshBusy.value) return;
  targetTicket++; historyTicket++; loading.value = historyLoading.value = false;
  const marker = ++conflictTicket, captured = stamp.value, session = commissionAdjustmentSessionGeneration(), lane = ++readLane; conflictRefreshBusy.value = true; conflictRefreshed.value = false; conflictTargetMissing.value = false; discardChecked.value = false;
  try {
    let offset = 0, targetPage: Awaited<ReturnType<typeof api.listTargets>> | null = null, found = false, targetPageOffset = 0;
    do {
      const page = await api.listTargets(intent.brandId,intent.paymentId,SCAN_PAGE,offset);
      if (marker !== conflictTicket || captured !== stamp.value || lane !== readLane || session !== commissionAdjustmentSessionGeneration() || !rights.value.view) return;
      const targetIndex = page.items.findIndex((item) => item.id === intent.targetId);
      if (targetIndex >= 0) { targetPage = page; targetPageOffset = page.offset + Math.floor(targetIndex / targetPageSize.value) * targetPageSize.value; found = true; break; }
      offset += page.limit;
      if (!page.items.length || BigInt(offset) >= BigInt(page.total_count)) break;
    } while (true);
    if (!found || !targetPage) { conflictTargetMissing.value = true; return; }
    targetPage = await api.listTargets(intent.brandId,intent.paymentId,targetPageSize.value,targetPageOffset);
    if (marker !== conflictTicket || captured !== stamp.value || lane !== readLane || session !== commissionAdjustmentSessionGeneration() || !rights.value.view || !targetPage.items.some((item) => item.id === intent.targetId)) { conflictTargetMissing.value = true; return; }
    const historyPage = await api.listAdjustments(intent.brandId,intent.targetId,historyPageSize.value,0);
    if (marker !== conflictTicket || captured !== stamp.value || lane !== readLane || session !== commissionAdjustmentSessionGeneration() || !rights.value.view || historyPage.items.some((item) => item.target_id !== intent.targetId || item.payment_id !== intent.paymentId)) return;
    agentDraft.value = agentFilter.value = "";
    targetsReady.value=true; targets.value = targetPage.items; targetTotal.value = targetPage.total_count; targetOffset.value = targetPage.offset; targetLimit.value = targetPage.limit;
    historyReady.value=true; history.value = historyPage.items; historyTotal.value = historyPage.total_count; historyOffset.value = 0; selectedId.value = intent.targetId; conflictRefreshed.value = true;
  } catch (cause) { if (marker === conflictTicket && captured === stamp.value) { sessionExpired(cause,captured,session); historyError.value = readFailure(cause); } }
  finally { if (marker === conflictTicket) conflictRefreshBusy.value = false; }
}
function discardConflict() {
  const intent = conflict.value; if (!intent || !conflictRefreshed.value || !discardChecked.value || conflictRefreshBusy.value) return;
  clearPendingCommissionAdjustmentIntent(scopeFor(intent.targetId),intent.key); conflict.value = null; conflictRefreshed.value = false; discardChecked.value = false; writeError.value = ""; notice.value = message('admin.copy.702b35bf');
}
function cancelDialog() { dialog.value = null; confirmed.value = false; }
watch(stamp, reset, { immediate: true });
onUnmounted(() => { alive = false; generation++; targetTicket++; historyTicket++; writeTicket++; readLane++; });

useOperationToast(() => error.value, value => displayPublicError(value), 'error');
useOperationToast(() => writeError.value, value => displayPublicError(value), 'error');
useOperationToast(() => notice.value, value => displayPublicError(value), 'success');
useOperationToast(() => historyError.value, value => displayPublicError(value), 'error');
</script>

<template>
  <section class="commission-adjustments" :data-testid="'commission-adjustments-management'">

    <p v-if="!rights.view" role="status">{{ t('admin.copy.847f145c') }}</p>
    <template v-else>
      <p v-if="!rights.write" role="status">{{ t('admin.copy.71fcb5dc') }}</p>
      <p v-if="props.brandStatus === 'disabled'" role="status">{{ t('admin.copy.d21d148a') }}</p>

      <p v-if="unknown" class="pending-intent">{{ t('admin.copy.3e878895') }} · {{ unknown.body.points }} <button class="button button-secondary" type="button" :disabled="!rights.write || writing || props.brandStatus === 'disabled'" @click="recover">{{ t('admin.copy.589bf172') }}</button></p>
      <ManagementList><template #filters><form v-inline-validation novalidate class="commission-toolbar ui-filter-bar" @submit.prevent="withGlobalLoading(searchTargets)"><label><FilterAssociationSearch :reset-key="filterReset" v-model="agentDraft" v-model:pending="agentDraftSearchPending" :brand-id="brandId" surface="admin" resource="commission" kind="agent" :label="t('admin.commission.ui.agent')" :locale="associationLocale" :scope-key="stamp" :enabled="rights.view" /></label><button class="button button-primary" type="submit" data-testid="adjustment-search" :disabled="agentDraftSearchPending || loading || writing || Boolean(unknown) || Boolean(conflict)">{{ t('admin.commission.ui.query') }}</button></form></template>
      <p v-if="agentDraft!==agentFilter" class="commission-note">{{ t('admin.commission.ui.filtersChanged') }}</p>
      <ManagementTable :entries="targets" :columns="targetColumns" :empty="!loading && !error && targets.length===0" :empty-label="t('admin.commission.ui.empty')">
       <template #agent="{entry}"><button type="button" class="text-button" :data-target-id="entry.id" :aria-pressed="selectedId===entry.id" @click="withGlobalLoading(chooseTarget,entry.id)"><AssociationName :record="entry" kind="agent" :id="entry.agent_id" :locale="associationLocale" /></button></template>
       <template #original="{entry}">{{ entry.original_points }}</template><template #adjusted="{entry}">{{ entry.adjusted_points??'—' }}</template>
       <template #actions="{entry}"><MemberActionsMenu :id="'CommissionAdjustmentsManagement-'+entry.id" :label="t('admin.copy.d21fcbef')"><button type="button" role="menuitem" class="button target-card" :aria-pressed="selectedId===entry.id" @click="withGlobalLoading(chooseTarget,entry.id)">{{ t('admin.commission.ui.view') }}</button></MemberActionsMenu></template>
      </ManagementTable>
      <template #pagination><TablePagination v-if="targetsReady" class="commission-pager" :aria-label="t('admin.commission.ui.targets')" :total="targetTotal" :page="Math.floor(targetOffset / targetLimit + 1)" :count="targets.length" :locale="associationLocale" :page-size="targetPageSize" :previous-disabled="targetOffset === 0 || loading" :next-disabled="!hasTargetNext || loading" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.2e9c9e7a')" :go-to="(target: number) => pageTargets((target - 1) * targetLimit)" :change-page-size="async (size: number) => { targetPageSize = size; await loadTargets(0) }" :busy="loading" /></template></ManagementList>
      <DetailModal v-if="selected && detailOpen" :open="!dialog" :title="t('admin.copy.01db40da')" :close-label="t('association.close')" :close-disabled="writing" @close="detailOpen=false">
      <article v-if="selected" class="target-detail">
        <h3><AssociationName :record="selected" kind="agent" :id="selected.agent_id" :locale="associationLocale" /></h3>
        <dl><dt>{{ t('admin.copy.5dbf3829') }}</dt><dd>{{ selected.original_points }}</dd><dt>{{ t('admin.copy.0eca5c00') }}</dt><dd>{{ selected.adjusted_points ?? "—" }}</dd></dl>
        <form class="ui-form" novalidate v-inline-validation @submit.prevent="withGlobalLoading(prepare)"><label>{{ t('admin.copy.2f024587') }}<input v-model="points" inputmode="decimal" autocomplete="off" data-testid="adjustment-points" :disabled="!available"></label><p>{{ t('admin.copy.451e940a') }} {{ selected.adjusted_points ?? "—" }} → {{ points || "—" }} · Δ {{ deltaPreview || "—" }}</p><button class="button button-primary" type="submit" data-testid="adjustment-review" :disabled="!canPrepare">{{ t('admin.copy.da7028ac') }}</button></form>
      </article>
      <ManagementList embedded v-if="selected" class="adjustment-history"><template #heading><h3>{{ t('admin.copy.d55ecbbf') }}</h3></template><p v-global-loading v-if="historyLoading">{{ t('admin.copy.27342bf3') }}</p><p v-else-if="historyError" role="alert" class="commission-warning">{{ displayPublicError(historyError) }} <button class="button button-secondary" v-refresh-button="t('admin.commission.ui.retryRead')" type="button" @click="withGlobalLoading(loadHistory,selectedId,historyOffset)">{{ t('admin.commission.ui.retryRead') }}</button></p><p v-else-if="history.length === 0" class="empty-state">{{ t('admin.copy.d7301a2b') }}</p><article v-for="item in history" :key="item.id" class="history-card"><strong> {{ item.delta_points }}</strong><dl><dt>{{ t('admin.copy.a592c0e8') }}</dt><dd>{{ item.points_before }}</dd><dt>{{ t('admin.copy.00c00ace') }}</dt><dd>{{ item.points_after }}</dd><dt>{{ t('admin.copy.cef73d0f') }}</dt><dd><AssociationName :record="item" kind="admin" :id="item.created_by" :locale="associationLocale" /></dd><dt v-if="item.reason">{{ t('admin.copy.5ec7c8ba') }}</dt><dd v-if="item.reason">{{ item.reason }}</dd></dl></article><template #pagination><TablePagination v-if="historyReady" class="commission-pager" :aria-label="t('admin.copy.d55ecbbf')" :total="historyTotal" :page="Math.floor(historyOffset / historyPageSize + 1)" :count="history.length" :locale="associationLocale" :page-size="historyPageSize" :previous-disabled="historyOffset === 0 || historyLoading" :next-disabled="!hasHistoryNext || historyLoading" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.2e9c9e7a')" :go-to="(target: number) => pageHistory((target - 1) * historyPageSize)" :change-page-size="async (size: number) => { historyPageSize = size; await loadHistory(selectedId, 0) }" :busy="historyLoading" /></template></ManagementList>
      </DetailModal>
      <article v-if="receipt" class="receipt" data-testid="adjustment-receipt"><h3>{{ t('admin.copy.e671d5a1') }}</h3><p>{{ receipt.points_before }} → {{ receipt.points_after }} · Δ {{ receipt.delta_points }}</p></article>
      <article v-if="conflict" data-testid="adjustment-conflict" role="alert"><header class="console-page-heading"><div class="console-page-actions"><button class="button button-secondary" v-refresh-button="t('admin.copy.894e2a7d')" type="button" data-testid="adjustments-refresh" :disabled="loading" @click="withGlobalLoading(loadTargets, 0)">{{ t('admin.copy.894e2a7d') }}</button></div></header><p v-if="conflictRefreshed">{{ t('admin.copy.f96322ad') }}</p><p v-if="conflictTargetMissing">{{ t('admin.copy.93f14581') }}</p><button class="button button-secondary" v-refresh-button="t('admin.copy.0ded9cd5')" type="button" :disabled="conflictRefreshBusy" @click="withGlobalLoading(refreshConflict)">{{ t('admin.copy.0ded9cd5') }}</button><label><input type="checkbox" v-model="discardChecked" :disabled="!conflictRefreshed || conflictRefreshBusy">{{ t('admin.copy.d42166af') }}</label><button class="button button-secondary" type="button" :disabled="!conflictRefreshed || !discardChecked || conflictRefreshBusy" @click="discardConflict">{{ t('admin.copy.1abc7148') }}</button></article>
    </template>
    <DetailModal v-if="dialog" class="confirmation-dialog review-dialog" :title="t('admin.copy.7a6afe50')"  :close-label="t('association.close')" :close-disabled="writing" @close="if (!(writing)) { cancelDialog(); }" data-testid="adjustment-review-dialog"><dl><dt>{{ t('admin.copy.cef73d0f') }}</dt><dd><AssociationName :record="selected" kind="admin" :id="dialog.actorId" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.9c32661d') }}</dt><dd><AssociationName :record="selected" kind="brand" :id="dialog.brandId" :locale="associationLocale" /></dd><dt>{{ t('admin.commission.ui.agent') }}</dt><dd><AssociationName v-if="selected && selected.id===dialog.targetId" :record="selected" kind="agent" :id="selected.agent_id" :locale="associationLocale" /><span v-else>—</span></dd><dt>{{ t('admin.commission.ui.currentPoints') }}</dt><dd>{{ dialog.pointsBefore }}</dd><dt>{{ t('admin.commission.ui.adjustedPoints') }}</dt><dd>{{ dialog.body.points }}</dd><dt>{{ t('admin.commission.ui.deltaPoints') }}</dt><dd>{{ pointsFromMinor(pointsMinor(dialog.body.points) - pointsMinor(dialog.pointsBefore)) }}</dd><dt v-if="dialog.body.reason">{{ t('admin.copy.5ec7c8ba') }}</dt><dd v-if="dialog.body.reason">{{ dialog.body.reason }}</dd></dl><label><input type="checkbox" v-model="confirmed" data-testid="adjustment-confirmed">{{ t('admin.copy.23502432') }}</label><div class="ui-form-actions"><button class="button button-primary" v-write-guard="writing" type="button" data-testid="adjustment-confirm-submit" :disabled="!confirmed || writing" @click="withGlobalLoading(submit)">{{ t('admin.copy.40d04e41') }}</button></div></DetailModal>
  </section>
</template>

<style scoped>

.commission-adjustments button:disabled{opacity:.55;cursor:not-allowed}
.commission-adjustments input:not([type=checkbox]),.commission-adjustments textarea{box-sizing:border-box;width:100%;min-height:40px;border:1px solid var(--line);border-radius:8px;padding:9px 10px;font:inherit;background:var(--surface);color:inherit}
.commission-adjustments textarea{resize:vertical;min-height:88px}
.commission-adjustments h2,.commission-adjustments h3{margin:0 0 .6rem}.commission-adjustments header p{color:var(--muted);line-height:1.55;margin:.35rem 0 0}
.commission-adjustments .target-card,.commission-adjustments .target-detail,.commission-adjustments .history-card,.commission-adjustments .receipt{border-color:var(--line);border-radius:12px;background:var(--surface)}
.commission-adjustments .target-card strong{overflow-wrap:anywhere}.commission-adjustments .target-card[aria-pressed=true]{background:#ecfdf5}
.commission-adjustments dt{color:var(--muted);font-size:var(--body-font-size)}.commission-adjustments dd{line-height:1.5}
.commission-adjustments .adjustment-history{display:grid;gap:12px;grid-template-columns:repeat(auto-fit,minmax(min(100%,18rem),1fr))}
.commission-adjustments .adjustment-history>h3,.commission-adjustments .adjustment-history>p{grid-column:1/-1}
.commission-adjustments .receipt{overflow-wrap:anywhere}
.commission-adjustments{display:grid;gap:1rem;color:inherit}.commission-adjustments header{display:flex;align-items:flex-start;justify-content:space-between;gap:1rem}.target-list{display:grid;grid-template-columns:repeat(auto-fit,minmax(14rem,1fr));gap:.75rem}.target-card{display:grid;text-align:left;gap:.35rem;padding:.8rem;border:1px solid var(--line);border-radius:.5rem;background:transparent;color:inherit}.target-card[aria-pressed=true]{outline:2px solid var(--primary)}.target-detail,.history-card,.receipt,.review-dialog,.adjustment-conflict{border:1px solid var(--line);border-radius:.5rem;padding:1rem}.target-detail dl,.history-card dl,.review-dialog dl{display:grid;grid-template-columns:minmax(9rem,auto) minmax(0,1fr);gap:.35rem .8rem}.target-detail dd,.history-card dd,.review-dialog dd{margin:0;overflow-wrap:anywhere}.target-detail form{display:grid;gap:.8rem;max-width:38rem}.target-detail label{display:grid;gap:.3rem}.review-backdrop{position:fixed;inset:0;z-index:50;background:#0008;display:grid;place-items:center;padding:1rem}.adjustment-conflict{display:grid;gap:.7rem}.pending-intent{overflow-wrap:anywhere}@media(max-width:600px){.commission-adjustments header{display:grid}.target-detail dl,.history-card dl,.review-dialog dl{grid-template-columns:1fr}.target-detail dd,.history-card dd,.review-dialog dd{margin-bottom:.35rem}}
</style>
