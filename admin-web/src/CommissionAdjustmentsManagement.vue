<script setup lang="ts">
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

const props = defineProps<{ account: AdminAccount; brandId: string; paymentId: string; paymentState: string; brandStatus?: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, display } = useAdminI18n();
const api = createCommissionAdjustmentsApi(fetch);
const PAGE = 20;
const SCAN_PAGE = 100;
const rights = computed(() => commissionAdjustmentPermissions(props.account, props.brandId));
const stamp = computed(() => JSON.stringify([props.account.id, props.account.super_admin, props.account.brand_ids, props.account.permissions, props.account.permissions_by_brand, props.account.platform_permissions, props.brandId, props.paymentId, props.paymentState, props.brandStatus, rights.value]));
const targets = ref<CommissionAdjustmentTarget[]>([]), targetTotal = ref("0"), targetOffset = ref(0), targetLimit = ref(PAGE), selectedId = ref("");
const history = ref<CommissionAdjustment[]>([]), historyTotal = ref("0"), historyOffset = ref(0);
const loading = ref(false), historyLoading = ref(false), writing = ref(false);
const error = ref<string | import("@lottery/shared").LocalizedMessage>(""), historyError = ref<string | import("@lottery/shared").LocalizedMessage>(""), writeError = ref<string | import("@lottery/shared").LocalizedMessage>(""), notice = ref("");
const points = ref(""), reason = ref(""), dialog = ref<PendingCommissionAdjustmentIntent | null>(null), confirmed = ref(false);
const unknown = ref<PendingCommissionAdjustmentIntent | null>(null), conflict = ref<PendingCommissionAdjustmentIntent | null>(null);
const conflictRefreshed = ref(false), conflictTargetMissing = ref(false), discardChecked = ref(false), conflictRefreshBusy = ref(false);
const receipt = ref<CommissionAdjustment | null>(null);
let alive = true, generation = 0, targetTicket = 0, historyTicket = 0, writeTicket = 0, readLane = 0, conflictTicket = 0;
const selected = computed(() => targets.value.find((row) => row.id === selectedId.value) ?? null);
const available = computed(() => Boolean(rights.value.write && props.brandStatus !== "disabled" && props.paymentState === "paid" && selected.value?.state === "paid" && selected.value.adjustment_version !== null && selected.value.adjustment_version < Number.MAX_SAFE_INTEGER && !writing.value && !unknown.value && !conflict.value && !loading.value && !historyLoading.value && !error.value && !historyError.value));
const reasonValid = computed(() => validCommissionAdjustmentReason(reason.value.trim()));
const deltaPreview = computed(() => { if (!selected.value || !/^(?:0|[1-9][0-9]*)$/.test(points.value)) return ""; try { const delta = BigInt(points.value) - BigInt(selected.value.adjusted_points ?? "0"); return delta.toString(); } catch { return ""; } });
const canPrepare = computed(() => available.value && Boolean(selected.value && !getPendingCommissionAdjustmentIntent(scopeFor(selected.value.id))) && validCommissionAdjustmentPoints(points.value) && Boolean(selected.value && validCommissionAdjustmentDelta(selected.value.adjusted_points ?? "0",points.value)) && reasonValid.value);
const hasTargetNext = computed(() => BigInt(targetOffset.value + targets.value.length) < BigInt(targetTotal.value));
const hasHistoryNext = computed(() => BigInt(historyOffset.value + history.value.length) < BigInt(historyTotal.value));
const hasTargetPages = computed(() => BigInt(targetTotal.value) > BigInt(targetLimit.value));
const hasHistoryPages = computed(() => BigInt(historyTotal.value) > BigInt(PAGE));
const scopeFor = (targetId: string): CommissionAdjustmentIntentScope => ({ accountId: props.account.id, brandId: props.brandId, paymentId: props.paymentId, targetId });
function active(_ticket: number, captured: string, session: number) { return alive && captured === stamp.value && session === commissionAdjustmentSessionGeneration() && rights.value.view; }
function readFailure(cause: unknown) { return cause instanceof Error ? publicErrorMessage(cause) : t('admin.copy.466e55a6'); }
function reset() {
  generation++; readLane++; targetTicket++; historyTicket++; writeTicket++; conflictTicket++;
  targets.value = []; history.value = []; targetTotal.value = historyTotal.value = "0"; targetOffset.value = historyOffset.value = 0; targetLimit.value = PAGE; selectedId.value = ""; points.value = ""; reason.value = ""; dialog.value = null; confirmed.value = false; error.value = historyError.value = writeError.value = notice.value = ""; loading.value = historyLoading.value = writing.value = false; conflictRefreshed.value = conflictTargetMissing.value = discardChecked.value = conflictRefreshBusy.value = false; receipt.value = null;
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
    let page = await api.listTargets(props.brandId, props.paymentId, PAGE, offset);
    if (!active(ticket, captured, session) || lane !== readLane) return;
    if (locateTargetId && !page.items.some((row) => row.id === locateTargetId)) {
      let scanOffset = 0, located = false;
      while (true) {
        const scan = await api.listTargets(props.brandId,props.paymentId,SCAN_PAGE,scanOffset);
        if (!active(ticket,captured,session) || lane !== readLane) return;
        const index = scan.items.findIndex((row) => row.id === locateTargetId);
        if (index >= 0) {
          const desiredOffset = scan.offset + Math.floor(index / PAGE) * PAGE;
          page = await api.listTargets(props.brandId,props.paymentId,PAGE,desiredOffset);
          if (!active(ticket,captured,session) || lane !== readLane) return;
          located = page.items.some((row) => row.id === locateTargetId);
          break;
        }
        scanOffset += scan.limit;
        if (!scan.items.length || BigInt(scanOffset) >= BigInt(scan.total_count)) break;
      }
      if (!located) notice.value = t('admin.copy.b18927d9');
    }
    targets.value = page.items; targetTotal.value = page.total_count; targetOffset.value = page.offset; targetLimit.value = page.limit;
    if (locateTargetId && page.items.some((row) => row.id === locateTargetId)) selectedId.value = locateTargetId;
    else if (!page.items.some((row) => row.id === selectedId.value)) selectedId.value = page.items[0]?.id ?? "";
    if (selectedId.value) void loadHistory(selectedId.value);
  } catch (cause) { if (active(ticket, captured, session) && lane === readLane) { sessionExpired(cause,captured,session); error.value = readFailure(cause); } }
  finally { if (active(ticket,captured,session) && ticket === targetTicket) loading.value = false; }
}
async function loadHistory(id: string, offset = historyOffset.value) {
  if (!rights.value.view || !id) return;
  conflictTicket++; conflictRefreshBusy.value = false; conflictRefreshed.value = false; discardChecked.value = false;
  const ticket = ++historyTicket, captured = stamp.value, session = commissionAdjustmentSessionGeneration(), lane = readLane; historyLoading.value = true; historyError.value = "";
  try {
    const page = await api.listAdjustments(props.brandId,id,PAGE,offset);
    if (!active(generation,captured,session) || ticket !== historyTicket || lane !== readLane || selectedId.value !== id || page.items.some((item) => item.target_id !== id || item.payment_id !== props.paymentId)) throw new AdminApiError("佣金修正历史响应范围无效。", 502, "INVALID_RESPONSE");
    history.value = page.items; historyTotal.value = page.total_count; historyOffset.value = page.offset;
  } catch (cause) { if (active(generation,captured,session) && ticket === historyTicket && lane === readLane) { sessionExpired(cause,captured,session); historyError.value = readFailure(cause); } }
  finally { if (ticket === historyTicket && active(generation,captured,session)) historyLoading.value = false; }
}
async function chooseTarget(id: string) {
  selectedId.value = id; points.value = ""; reason.value = ""; historyOffset.value = 0; dialog.value = null; receipt.value = null;
  const pending = getPendingCommissionAdjustmentIntent(scopeFor(id));
  conflict.value = pending && isCommissionAdjustmentConflict(pending) ? pending : null;
  unknown.value = pending && !conflict.value ? pending : null;
  await loadHistory(id,0);
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
    receipt.value = acknowledged; dialog.value = null; unknown.value = null; confirmed.value = false; notice.value = t('admin.copy.61bac1f8');
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
      if (targetIndex >= 0) { targetPage = page; targetPageOffset = page.offset + Math.floor(targetIndex / PAGE) * PAGE; found = true; break; }
      offset += page.limit;
      if (!page.items.length || BigInt(offset) >= BigInt(page.total_count)) break;
    } while (true);
    if (!found || !targetPage) { conflictTargetMissing.value = true; return; }
    targetPage = await api.listTargets(intent.brandId,intent.paymentId,PAGE,targetPageOffset);
    if (marker !== conflictTicket || captured !== stamp.value || lane !== readLane || session !== commissionAdjustmentSessionGeneration() || !rights.value.view || !targetPage.items.some((item) => item.id === intent.targetId)) { conflictTargetMissing.value = true; return; }
    const historyPage = await api.listAdjustments(intent.brandId,intent.targetId,PAGE,0);
    if (marker !== conflictTicket || captured !== stamp.value || lane !== readLane || session !== commissionAdjustmentSessionGeneration() || !rights.value.view || historyPage.items.some((item) => item.target_id !== intent.targetId || item.payment_id !== intent.paymentId)) return;
    targets.value = targetPage.items; targetTotal.value = targetPage.total_count; targetOffset.value = targetPage.offset; targetLimit.value = targetPage.limit;
    history.value = historyPage.items; historyTotal.value = historyPage.total_count; historyOffset.value = 0; selectedId.value = intent.targetId; conflictRefreshed.value = true;
  } catch (cause) { if (marker === conflictTicket && captured === stamp.value) { sessionExpired(cause,captured,session); historyError.value = readFailure(cause); } }
  finally { if (marker === conflictTicket) conflictRefreshBusy.value = false; }
}
function discardConflict() {
  const intent = conflict.value; if (!intent || !conflictRefreshed.value || !discardChecked.value || conflictRefreshBusy.value) return;
  clearPendingCommissionAdjustmentIntent(scopeFor(intent.targetId),intent.key); conflict.value = null; conflictRefreshed.value = false; discardChecked.value = false; writeError.value = ""; notice.value = t('admin.copy.702b35bf');
}
function cancelDialog() { dialog.value = null; confirmed.value = false; }
watch(stamp, reset, { immediate: true });
onUnmounted(() => { alive = false; generation++; targetTicket++; historyTicket++; writeTicket++; readLane++; });
</script>

<template>
  <section class="commission-adjustments" :data-testid="'commission-adjustments-management'">
    <header><div><h2>{{ t('admin.copy.fa127cf3') }}</h2><p>{{ t('admin.copy.7ac7064d') }}</p></div><button v-refresh-button="t('admin.copy.894e2a7d')" type="button" data-testid="adjustments-refresh" :disabled="loading" @click="withGlobalLoading(loadTargets, 0)">{{ t('admin.copy.894e2a7d') }}</button></header>
    <p v-if="!rights.view" role="status">{{ t('admin.copy.847f145c') }}</p>
    <template v-else>
      <p v-if="!rights.write" role="status">{{ t('admin.copy.71fcb5dc') }}</p>
      <p v-if="props.brandStatus === 'disabled'" role="status">{{ t('admin.copy.d21d148a') }}</p>
      <p v-if="error" role="alert">{{ displayPublicError(error) }}</p><p v-if="writeError" role="alert">{{ displayPublicError(writeError) }}</p><p v-if="notice" role="status">{{ notice }}</p>

      <p v-if="unknown" class="pending-intent">{{ t('admin.copy.3e878895') }} · {{ unknown.body.points }} <button type="button" :disabled="!rights.write || writing || props.brandStatus === 'disabled'" @click="recover">{{ t('admin.copy.589bf172') }}</button></p>
      <div class="target-list" aria-label="Commission targets">
        <button v-for="target in targets" :key="target.id" type="button" class="target-card" :aria-pressed="selectedId === target.id" @click="withGlobalLoading(chooseTarget, target.id)">
          <strong>{{ t('admin.copy.559998d7') }} {{ target.id }}</strong><span>{{ t('admin.copy.451e940a') }}: {{ target.original_points }}</span><span>{{ t('admin.copy.cb6eed10') }}: {{ target.adjusted_points ?? "—" }}</span>
        </button>
      </div>
      <nav v-if="hasTargetPages" class="pagination" aria-label="Target pages"><button type="button" :disabled="targetOffset === 0 || loading" @click="withGlobalLoading(pageTargets, Math.max(0,targetOffset-targetLimit))">{{ t('admin.copy.dbe7a5d7') }}</button><span>{{ targetOffset + 1 }}–{{ targetOffset + targets.length }} / {{ targetTotal }}</span><button type="button" :disabled="!hasTargetNext || loading" @click="withGlobalLoading(pageTargets, targetOffset+targetLimit)">{{ t('admin.copy.2e9c9e7a') }}</button></nav>
      <p v-if="!loading && targets.length === 0" class="empty-state">{{ t('admin.copy.43fa95e6') }}</p>
      <article v-if="selected" class="target-detail">
        <h3>{{ t('admin.copy.97dd5cea') }}</h3>
        <dl><dt>{{ t('admin.copy.5dbf3829') }}</dt><dd>{{ selected.original_points }}</dd><dt>{{ t('admin.copy.0eca5c00') }}</dt><dd>{{ selected.adjusted_points ?? "—" }}</dd><dt>{{ t('admin.copy.ad19102e') }}</dt><dd>{{ selected.ledger_entry_id ?? "—" }}</dd><dt>{{ t('admin.copy.386b3ddb') }}</dt><dd>{{ selected.last_adjustment_id ?? "—" }}</dd></dl>
        <form novalidate v-inline-validation @submit.prevent="withGlobalLoading(prepare)"><label>{{ t('admin.copy.2f024587') }}<input v-model="points" inputmode="numeric" autocomplete="off" data-testid="adjustment-points" :disabled="!available"></label><p>{{ t('admin.copy.451e940a') }} {{ selected.adjusted_points ?? "—" }} → {{ points || "—" }} · Δ {{ deltaPreview || "—" }}</p><button type="submit" data-testid="adjustment-review" :disabled="!canPrepare">{{ t('admin.copy.da7028ac') }}</button></form>
      </article>
      <section class="adjustment-history"><h3>{{ t('admin.copy.d55ecbbf') }}</h3><p v-if="historyError" role="alert">{{ displayPublicError(historyError) }}</p><p v-global-loading v-if="historyLoading">{{ t('admin.copy.27342bf3') }}</p><p v-else-if="history.length === 0" class="empty-state">{{ t('admin.copy.d7301a2b') }}</p><article v-for="item in history" :key="item.id" class="history-card"><strong> {{ item.delta_points }}</strong><dl><dt>{{ t('admin.copy.a592c0e8') }}</dt><dd>{{ item.points_before }}</dd><dt>{{ t('admin.copy.00c00ace') }}</dt><dd>{{ item.points_after }}</dd><dt>{{ t('admin.copy.cef73d0f') }}</dt><dd><AssociationName :record="item" kind="admin" :id="item.created_by" :locale="associationLocale" /></dd><dt v-if="item.reason">{{ t('admin.copy.5ec7c8ba') }}</dt><dd v-if="item.reason">{{ item.reason }}</dd><dt>Ledger ID</dt><dd>{{ item.ledger_entry_id }}</dd></dl></article><nav v-if="hasHistoryPages" class="pagination" aria-label="Adjustment history pages"><button type="button" :disabled="historyOffset === 0 || historyLoading" @click="withGlobalLoading(pageHistory, Math.max(0,historyOffset-PAGE))">{{ t('admin.copy.dbe7a5d7') }}</button><span>{{ historyOffset + 1 }}–{{ historyOffset + history.length }} / {{ historyTotal }}</span><button type="button" :disabled="!hasHistoryNext || historyLoading" @click="withGlobalLoading(pageHistory, historyOffset+PAGE)">{{ t('admin.copy.2e9c9e7a') }}</button></nav></section>
      <article v-if="receipt" class="receipt" data-testid="adjustment-receipt"><h3>{{ t('admin.copy.e671d5a1') }}</h3><p>{{ receipt.id }} · {{ receipt.points_before }} → {{ receipt.points_after }} · Δ {{ receipt.delta_points }}</p></article>
      <article v-if="conflict" data-testid="adjustment-conflict" role="alert"><p v-if="conflictRefreshed">{{ t('admin.copy.f96322ad') }}</p><p v-if="conflictTargetMissing">{{ t('admin.copy.93f14581') }}</p><button v-refresh-button="t('admin.copy.0ded9cd5')" type="button" :disabled="conflictRefreshBusy" @click="withGlobalLoading(refreshConflict)">{{ t('admin.copy.0ded9cd5') }}</button><label><input type="checkbox" v-model="discardChecked" :disabled="!conflictRefreshed || conflictRefreshBusy">{{ t('admin.copy.d42166af') }}</label><button type="button" :disabled="!conflictRefreshed || !discardChecked || conflictRefreshBusy" @click="discardConflict">{{ t('admin.copy.1abc7148') }}</button></article>
    </template>
    <div v-if="dialog" class="review-backdrop" role="presentation"><section class="review-dialog" role="dialog" aria-modal="true" data-testid="adjustment-review-dialog"><h3>{{ t('admin.copy.7a6afe50') }}</h3><dl><dt>{{ t('admin.copy.cef73d0f') }}</dt><dd><AssociationName kind="admin" :id="dialog.actorId" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.9c32661d') }}</dt><dd><AssociationName kind="brand" :id="dialog.brandId" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.559998d7') }}</dt><dd>{{ dialog.targetId }}</dd><dt>{{ t('admin.copy.7fcec890') }}</dt><dd>{{ dialog.pointsBefore }}</dd><dt>{{ t('admin.copy.583fb1c2') }}</dt><dd>{{ dialog.body.points }}</dd><dt>Δ</dt><dd>{{ (BigInt(dialog.body.points) - BigInt(dialog.pointsBefore)).toString() }}</dd><dt v-if="dialog.body.reason">{{ t('admin.copy.5ec7c8ba') }}</dt><dd v-if="dialog.body.reason">{{ dialog.body.reason }}</dd></dl><label><input type="checkbox" v-model="confirmed" data-testid="adjustment-confirmed">{{ t('admin.copy.23502432') }}</label><button v-write-guard="writing" type="button" data-testid="adjustment-confirm-submit" :disabled="!confirmed || writing" @click="withGlobalLoading(submit)">{{ t('admin.copy.40d04e41') }}</button><button type="button" @click="cancelDialog">{{ t('admin.copy.e2f391e7') }}</button></section></div>
    <small>{{ targetTotal }} · {{ historyTotal }}</small>
  </section>
</template>

<style scoped>
.commission-adjustments button{min-height:40px;border:1px solid #c5ceda;border-radius:8px;padding:8px 12px;font:inherit;color:inherit;background:var(--surface);cursor:pointer;overflow-wrap:anywhere}
.commission-adjustments button:disabled{opacity:.55;cursor:not-allowed}
.commission-adjustments input:not([type=checkbox]),.commission-adjustments textarea{box-sizing:border-box;width:100%;min-height:40px;border:1px solid #c5ceda;border-radius:8px;padding:9px 10px;font:inherit;background:var(--surface);color:inherit}
.commission-adjustments textarea{resize:vertical;min-height:88px}
.commission-adjustments h2,.commission-adjustments h3{margin:0 0 .6rem}.commission-adjustments header p{color:var(--muted);line-height:1.55;margin:.35rem 0 0}
.commission-adjustments .target-card,.commission-adjustments .target-detail,.commission-adjustments .history-card,.commission-adjustments .receipt{border-color:var(--line);border-radius:12px;background:var(--surface)}
.commission-adjustments .target-card strong{overflow-wrap:anywhere}.commission-adjustments .target-card[aria-pressed=true]{background:#f4f8ff}
.commission-adjustments dt{color:var(--muted);font-size:.86rem}.commission-adjustments dd{line-height:1.5}
.commission-adjustments .adjustment-history{display:grid;gap:12px;grid-template-columns:repeat(auto-fit,minmax(min(100%,18rem),1fr))}
.commission-adjustments .adjustment-history>h3,.commission-adjustments .adjustment-history>p,.commission-adjustments .adjustment-history>.pagination{grid-column:1/-1}
.commission-adjustments .receipt{overflow-wrap:anywhere}
.commission-adjustments{display:grid;gap:1rem;color:inherit}.commission-adjustments header{display:flex;align-items:flex-start;justify-content:space-between;gap:1rem}.target-list{display:grid;grid-template-columns:repeat(auto-fit,minmax(14rem,1fr));gap:.75rem}.target-card{display:grid;text-align:left;gap:.35rem;padding:.8rem;border:1px solid var(--line);border-radius:.5rem;background:transparent;color:inherit}.target-card[aria-pressed=true]{outline:2px solid var(--primary)}.target-detail,.history-card,.receipt,.review-dialog,.adjustment-conflict{border:1px solid var(--line);border-radius:.5rem;padding:1rem}.target-detail dl,.history-card dl,.review-dialog dl{display:grid;grid-template-columns:minmax(9rem,auto) minmax(0,1fr);gap:.35rem .8rem}.target-detail dd,.history-card dd,.review-dialog dd{margin:0;overflow-wrap:anywhere}.target-detail form{display:grid;gap:.8rem;max-width:38rem}.target-detail label{display:grid;gap:.3rem}.pagination{display:flex;align-items:center;justify-content:center;gap:1rem}.review-backdrop{position:fixed;inset:0;z-index:50;background:#0008;display:grid;place-items:center;padding:1rem}.review-dialog{width:min(42rem,100%);max-height:90vh;overflow:auto;background:Canvas;color:CanvasText;display:grid;gap:.75rem}.adjustment-conflict{display:grid;gap:.7rem}.pending-intent{overflow-wrap:anywhere}@media(max-width:600px){.commission-adjustments header{display:grid}.target-detail dl,.history-card dl,.review-dialog dl{grid-template-columns:1fr}.target-detail dd,.history-card dd,.review-dialog dd{margin-bottom:.35rem}}
</style>
