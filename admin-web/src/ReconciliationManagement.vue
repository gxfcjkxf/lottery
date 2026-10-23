<script setup lang="ts">
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
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
const displayDate = (value: string | null | undefined) => formatDisplayDateTime(value, displayLocale.value);
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { adminKey, useAdminI18n } from "./i18n";
import type { LocalizedMessage } from "@lottery/shared";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { default as BrandBusinessInventory } from "./BrandBusinessInventory.vue";
import {
  createReconciliationApi, reconciliationPermissions,
  type ReconciliationCheckScope, type ReconciliationCreateBody, type ReconciliationJob, type ReconciliationOutcome, type ReconciliationTarget,
} from "./reconciliation-api";
import {
  classifyReconciliationWriteFailure, clearPendingReconciliationWrite, freezeReconciliationBody,
  getPendingReconciliationWrite, listPendingReconciliationWrites, reconciliationSessionGeneration,
  setPendingReconciliationWrite, type PendingReconciliationWrite, type ReconciliationWriteScope,
} from "./reconciliation-state";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const PAGE_SIZE = 20;
const api = createReconciliationApi();
const { t, message, display } = useAdminI18n();
const rights = computed(() => reconciliationPermissions(props.account, props.brandId));
const jobs = ref<ReconciliationJob[]>([]), jobTotal = ref("0"), offset = ref(0);
const job = ref<ReconciliationJob | null>(null), receipt = ref<ReconciliationJob | null>(null);
const targets = ref<ReconciliationTarget[]>([]), targetTotal = ref("0"), targetOffset = ref(0);
const outcomeFilter = ref<ReconciliationOutcome | "">("");
const reason = ref(""), retryReason = ref("");
const checkScope = ref<ReconciliationCheckScope>("wallet");
const review = ref<PendingReconciliationWrite | null>(null), confirmed = ref(false);
const pendingCreate = ref<PendingReconciliationWrite | null>(null), pendingRetries = ref<PendingReconciliationWrite[]>([]);
const loading = ref(false), detailLoading = ref(false), targetLoading = ref(false), writing = ref(false);
const listError = ref<string | LocalizedMessage>(""), detailError = ref<string | LocalizedMessage>(""), targetError = ref<string | LocalizedMessage>(""), writeError = ref<string | LocalizedMessage>(""), notice = ref<string | LocalizedMessage>("");
let alive = true, listTicket = 0, detailTicket = 0, targetTicket = 0, writeTicket = 0;

const permissionScope = computed(() => JSON.stringify([props.account.id, props.account.super_admin, props.account.brand_ids,
  props.account.permissions, props.account.permissions_by_brand, props.account.platform_permissions, props.brandId, rights.value]));
const createScope = computed<ReconciliationWriteScope>(() => ({ accountId: props.account.id, brandId: props.brandId, operation: "create" }));
const retryScope = computed<ReconciliationWriteScope | null>(() => job.value ? ({ accountId: props.account.id, brandId: props.brandId, operation: "retry", jobId: job.value.id }) : null);
const reasonValid = computed(() => validReasonText(reason.value));
const retryReasonValid = computed(() => validReasonText(retryReason.value));
const reasonByteLength = computed(() => new TextEncoder().encode(reason.value).length);
const hasPendingWrites = computed(() => pendingCreate.value !== null || pendingRetries.value.length > 0);
const canCreate = computed(() => rights.value.run && !props.account.super_admin && !writing.value && !pendingCreate.value && reasonValid.value);
const canRetry = computed(() => Boolean(rights.value.retry && !props.account.super_admin && job.value?.state === "failed" && job.value.can_retry &&
  retryScope.value && !getPendingReconciliationWrite(retryScope.value) && retryReasonValid.value && !writing.value));
const totalJobPages = computed(() => Math.max(1, Math.ceil(Number(jobTotal.value) / PAGE_SIZE)));
const totalTargetPages = computed(() => Math.max(1, Math.ceil(Number(targetTotal.value) / PAGE_SIZE)));

function current(ticket: number, lane: "list" | "detail" | "targets", scope: string, generation: number): boolean {
  const laneTicket = lane === "list" ? listTicket : lane === "detail" ? detailTicket : targetTicket;
  return alive && ticket === laneTicket && scope === permissionScope.value && generation === reconciliationSessionGeneration() && rights.value.view;
}
function currentWrite(ticket: number, scope: string, generation: number): boolean {
  return alive && ticket === writeTicket && scope === permissionScope.value && generation === reconciliationSessionGeneration();
}
function refreshPending() {
  const intents = listPendingReconciliationWrites(props.account.id, props.brandId);
  pendingCreate.value = intents.find((item) => item.operation === "create") ?? null;
  pendingRetries.value = intents.filter((item) => item.operation === "retry");
}
function resetView() {
  listTicket++; detailTicket++; targetTicket++; writeTicket++;
  loading.value = detailLoading.value = targetLoading.value = writing.value = false;
  jobs.value = []; jobTotal.value = "0"; job.value = null; receipt.value = null;
  targets.value = []; targetTotal.value = "0"; offset.value = targetOffset.value = 0;
  outcomeFilter.value = ""; listError.value = detailError.value = targetError.value = writeError.value = notice.value = "";
  review.value = null; confirmed.value = false; refreshPending();
}
async function loadList(next = offset.value) {
  if (!rights.value.view) { resetView(); return false; }
  const ticket = ++listTicket, captured = permissionScope.value, generation = reconciliationSessionGeneration();
  loading.value = true; listError.value = "";
  try {
    const page = await api.list(props.brandId, PAGE_SIZE, next);
    if (!current(ticket, "list", captured, generation)) return false;
    jobs.value = page.items; jobTotal.value = page.total_count; offset.value = page.offset;
    if (job.value && !page.items.some((item) => item.id === job.value?.id)) void loadJob(job.value.id);
    return true;
  } catch (problem) {
    if (current(ticket, "list", captured, generation)) listError.value = readFailure(problem);
    return false;
  } finally { if (current(ticket, "list", captured, generation)) loading.value = false; }
}
async function loadTargets(next = 0) {
  if (!job.value || !rights.value.view) return false;
  const id = job.value.id, ticket = ++targetTicket, captured = permissionScope.value, generation = reconciliationSessionGeneration();
  targetLoading.value = true; targetError.value = "";
  targets.value = []; targetTotal.value = "0";
  try {
    const page = await api.targets(props.brandId, id, outcomeFilter.value || null, PAGE_SIZE, next, job.value.check_scope);
    if (!current(ticket, "targets", captured, generation) || !job.value || job.value.id !== id) return false;
    targets.value = page.items; targetTotal.value = page.total_count; targetOffset.value = page.offset; return true;
  } catch (problem) {
    if (current(ticket, "targets", captured, generation) && job.value?.id === id) targetError.value = readFailure(problem);
    return false;
  } finally { if (current(ticket, "targets", captured, generation)) targetLoading.value = false; }
}
async function loadJob(id: string) {
  const ticket = ++detailTicket, captured = permissionScope.value, generation = reconciliationSessionGeneration();
  detailLoading.value = true; detailError.value = ""; job.value = null; refreshPending(); targetTicket++; targets.value = []; targetTotal.value = "0";
  try {
    const value = await api.read(props.brandId, id);
    if (!current(ticket, "detail", captured, generation)) return false;
    job.value = value; refreshPending(); await loadTargets(0); return true;
  } catch (problem) {
    if (current(ticket, "detail", captured, generation)) detailError.value = readFailure(problem);
    return false;
  } finally { if (current(ticket, "detail", captured, generation)) detailLoading.value = false; }
}
function readFailure(problem: unknown): string | LocalizedMessage {
  if (problem instanceof AdminApiError && problem.status === 0) return message('admin.copy.92a4c079');
  return problem instanceof Error ? publicErrorMessage(problem) : message('admin.copy.eb96ab64');
}
function validReasonText(value: string): boolean {
  if (value.trim() !== value || new TextEncoder().encode(value).length > 500 || /[\u0000-\u001f\u007f-\u009f]/u.test(value)) return false;
  for (let i = 0; i < value.length; i++) {
    const code = value.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff) {
      const next = value.charCodeAt(i + 1);
      if (!(next >= 0xdc00 && next <= 0xdfff)) return false;
      i++;
    } else if (code >= 0xdc00 && code <= 0xdfff) return false;
  }
  return true;
}
function selectedForCreate(): PendingReconciliationWrite | null {
  const existing = getPendingReconciliationWrite(createScope.value);
  if (existing) return existing;
  if (!reasonValid.value) return null;
  const body = freezeReconciliationBody({ reason: reason.value, check_scope: checkScope.value });
  return Object.freeze({ ...createScope.value, body, key: createIdempotencyKey() });
}
function selectedForRetry(): PendingReconciliationWrite | null {
  const scope = retryScope.value;
  if (!scope) return null;
  const existing = getPendingReconciliationWrite(scope);
  if (existing) return existing;
  if (!job.value || !retryReasonValid.value) return null;
  const body = freezeReconciliationBody({ version: job.value.version, reason: retryReason.value });
  return Object.freeze({ ...scope, body, key: createIdempotencyKey() });
}
function beginReview(intent: PendingReconciliationWrite | null) {
  if (!intent || writing.value) return;
  review.value = intent; confirmed.value = false; writeError.value = "";
}
async function submit(intent: PendingReconciliationWrite) {
  const capturedPermission = permissionScope.value, generation = reconciliationSessionGeneration();
  const ticket = ++writeTicket;
  const knownJob = job.value;
  const previous = knownJob && knownJob.id === intent.jobId && knownJob.state === "failed" &&
    knownJob.version === (intent.body as { version: number }).version ? knownJob : undefined;
  if (!getPendingReconciliationWrite(intent)) setPendingReconciliationWrite(intent, intent);
  refreshPending(); writing.value = true; review.value = null; confirmed.value = false; writeError.value = ""; notice.value = "";
  try {
    const result = intent.operation === "create"
      ? await api.create(intent.brandId, intent.body as ReconciliationCreateBody, intent.key, intent.accountId)
      : await api.retry(intent.brandId, intent.jobId!, intent.body as { version: number; reason: string }, intent.key, previous);
    if (!currentWrite(ticket, capturedPermission, generation)) return;
    clearPendingReconciliationWrite(intent, intent.key);
    receipt.value = result; notice.value = message('admin.copy.999daa90'); refreshPending();
    // Receipt is immutable evidence of this request; current state is a separate read.
    job.value = result;
    await Promise.all([loadJob(result.id), loadList(offset.value)]);
  } catch (problem) {
    if (problem instanceof AdminApiError && problem.status === 401) {
      if (currentWrite(ticket, capturedPermission, generation)) emit("session-invalid");
      return;
    }
    const failure = classifyReconciliationWriteFailure(problem instanceof AdminApiError ? problem.status : undefined);
    if (failure === "unknown") {
      if (currentWrite(ticket, capturedPermission, generation)) {
        setPendingReconciliationWrite(intent, intent); refreshPending(); writeError.value = message('admin.copy.95ee06b3');
      }
    } else {
      if (currentWrite(ticket, capturedPermission, generation)) {
        clearPendingReconciliationWrite(intent, intent.key); refreshPending();
        writeError.value = publicErrorMessage(problem);
      }
    }
  } finally { if (currentWrite(ticket, capturedPermission, generation)) writing.value = false; }
}
async function confirmWrite() {
  if (!review.value || !confirmed.value || writing.value) return;
  await submit(review.value);
}
function confirmRecovery(intent: PendingReconciliationWrite) { beginReview(intent); }
async function selectJob(id: string) { receipt.value = null; await loadJob(id); }
function setOutcome(value: string) { outcomeFilter.value = value as ReconciliationOutcome | ""; void loadTargets(0); }
async function pageList(delta: number) { if (!loading.value) await loadList(Math.max(0, offset.value + delta * PAGE_SIZE)); }
async function pageTargets(delta: number) { if (!targetLoading.value) await loadTargets(Math.max(0, targetOffset.value + delta * PAGE_SIZE)); }
function statusLabel(value: string) {
  const labels: Record<string, string> = { pending: 'admin.copy.99f247fd', running: 'admin.copy.4dd8120a', completed: 'admin.copy.293e79cc', failed: 'admin.copy.1feabec0', checked: 'admin.copy.135efbbb', consistent: 'admin.copy.f97b639f', repairable: 'admin.copy.d591240c', corrupt: 'admin.copy.242b6072' };
 return Object.prototype.hasOwnProperty.call(labels, value) ? t(adminKey(labels[value]!)) : value;
}
function short(value: string) { return `${value.slice(0, 8)}…${value.slice(-4)}`; }
function json(value: unknown) { return JSON.stringify(value, null, 2); }
function onOutcomeChange(event: Event) { setOutcome((event.target as HTMLSelectElement).value); }
function checkScopeLabel(scope: ReconciliationCheckScope) {
  return scope === "wallet_and_business" ? t('admin.copy.91f0f456') : t('admin.copy.ed353707');
}

watch(permissionScope, () => { resetView(); if (rights.value.view) void loadList(0); }, { immediate: true });
watch(() => job.value?.id, () => { refreshPending(); retryReason.value = ""; });
onBeforeUnmount(() => { alive = false; listTicket++; detailTicket++; targetTicket++; writeTicket++; });
</script>

<template>
  <main class="recon" aria-labelledby="recon-title">
    <header class="recon-head">
      <div><h1 id="recon-title">{{ t('admin.copy.50b60c4a') }}</h1><p class="sub">{{ t('admin.copy.df0539dd') }}</p></div>
      <button v-refresh-button="t('admin.copy.7d512bdb')" class="button button-quiet" :disabled="loading || !rights.view" @click="withGlobalLoading(loadList, offset)">{{ t('admin.copy.7d512bdb') }}</button>
    </header>
    <p v-if="notice" class="notice" role="status">{{ display(notice) }}</p>
    <p v-if="writeError" class="error write-error" role="alert">{{ display(writeError) }}</p>

    <BrandBusinessInventory :account="props.account" :brand-id="props.brandId" @session-invalid="emit('session-invalid')" />

    <section v-if="!rights.view" class="panel muted-panel">{{ t('admin.copy.3b8298f2') }}</section>

    <template v-else>
      <section v-if="receipt" class="panel receipt-panel" role="status">
        <div class="section-title"><div><h2>{{ t('admin.copy.3cb6da95') }}</h2></div></div>
        <p>{{ t('admin.copy.da4f443a') }} {{ receipt.id }} · {{ statusLabel(receipt.state) }} · {{ checkScopeLabel(receipt.check_scope) }} {{ t('admin.copy.3065a8cc') }} {{ receipt.target_count }} {{ t('admin.copy.f40ff44c') }} {{ receipt.pending_count }}</p>
      </section>
      <section class="panel create-panel">
        <div class="section-title"><div><h2>{{ t('admin.copy.9da20c6c') }}</h2><p>{{ t('admin.copy.fddf81a6') }}</p></div><span class="tag">{{ t('admin.copy.abf308d8') }}</span></div>
        <label class="field">{{ t('admin.copy.c97f0e15') }}<select v-model="checkScope" :aria-label="t('admin.copy.c97f0e15')" :disabled="hasPendingWrites || writing"><option value="wallet">{{ t('admin.copy.ed353707') }}</option><option value="wallet_and_business">{{ t('admin.copy.91f0f456') }}</option></select><small>{{ checkScope === "wallet_and_business" ? t('admin.copy.cabc1af4') : t('admin.copy.635c1e67') }}</small></label>

        <div class="actions"><button class="button button-primary" :disabled="!canCreate" @click="beginReview(selectedForCreate())">{{ t('admin.copy.55c13341') }}</button><span v-if="!rights.run" class="sub">{{ t('admin.copy.9df01ba5') }}</span></div>
      </section>

      <section v-if="pendingCreate || pendingRetries.length" class="panel pending-panel" aria-live="polite">
        <div class="section-title"><div><h2>{{ t('admin.copy.13ae36af') }}</h2></div><span class="tag warning">{{ writing ? t('admin.copy.bef5a2c2') : t('admin.copy.31e554b9') }}</span></div>
        <article v-if="pendingCreate" class="pending-item"><b>{{ t('admin.copy.bc0d513d') }} · {{ checkScopeLabel((pendingCreate.body as ReconciliationCreateBody).check_scope) }}</b><code></code><p>{{ t('admin.copy.12fe1e62') }}{{ pendingCreate.body.reason }}</p><button class="button button-primary" :disabled="writing" @click="confirmRecovery(pendingCreate)">{{ t('admin.copy.bbd107af') }}</button></article>
        <article v-for="intent in pendingRetries" :key="intent.key" class="pending-item"><b>{{ t('admin.copy.947e611a') }} {{ short(intent.jobId!) }}</b><code></code><p> {{ intent.body.reason }}</p><button class="button button-primary" :disabled="writing" @click="confirmRecovery(intent)">{{ t('admin.copy.fd712c1b') }}</button></article>
      </section>

      <div class="columns">
        <section class="panel jobs-panel">
          <div class="section-title"><div><h2>{{ t('admin.copy.41f3b7fb') }}</h2></div><span v-global-loading v-if="loading" class="tag">{{ t('admin.copy.ab7239f3') }}</span></div>
          <p v-if="listError" class="error" role="alert">{{ display(listError) }}</p>
          <div v-if="!loading && !listError && jobs.length === 0" class="empty empty-state">{{ t('admin.copy.293a3374') }}</div>
          <div v-for="item in jobs" :key="item.id" class="job-row" :class="{ selected: job?.id === item.id }">
            <button class="job-select" :aria-label="t('admin.copy.35d626eb', { id: item.id })" @click="withGlobalLoading(selectJob, item.id)"><span class="job-title">{{ short(item.id) }} <i :class="['state', item.state]">{{ statusLabel(item.state) }}</i></span><span class="sub">{{ displayDate(item.created_at) }}</span><span class="sub">{{ item.checked_count }} / {{ item.target_count }} {{ t('admin.copy.3fb2b6ef') }} {{ item.pending_count }} {{ t('admin.copy.99f247fd') }}</span></button>
          </div>
          <div class="pager"><span>{{ jobTotal }} {{ t('admin.copy.d9009591') }}</span><button class="button button-quiet" :disabled="loading || offset <= 0" @click="withGlobalLoading(pageList, -1)">{{ t('admin.copy.dbe7a5d7') }}</button><span>{{ Math.floor(offset / PAGE_SIZE) + 1 }} / {{ totalJobPages }}</span><button class="button button-quiet" :disabled="loading || offset + PAGE_SIZE >= Number(jobTotal)" @click="withGlobalLoading(pageList, 1)">{{ t('admin.copy.2e9c9e7a') }}</button></div>
        </section>

        <section class="panel detail-panel">
          <template v-if="job || detailLoading || detailError">
            <div class="section-title"><div><h2>{{ t('admin.copy.30b9c431') }}</h2><p v-if="job">{{ job.id }}</p></div><span v-global-loading v-if="detailLoading" class="tag">{{ t('admin.copy.ab7239f3') }}</span></div>
          <p v-if="detailError" class="error" role="alert">{{ t('admin.copy.a121cf61') }}{{ display(detailError) }}</p>
            <template v-if="job">
              <div class="stats"><div><b>{{ job.target_count }}</b><span>{{ t('admin.copy.0b2fa5d6') }}</span></div><div><b>{{ job.checked_count }}</b><span>{{ t('admin.copy.135efbbb') }}</span></div><div><b>{{ job.pending_count }}</b><span>{{ t('admin.copy.99f247fd') }}</span></div><div><b>{{ job.failed_count }}</b><span>{{ t('admin.copy.b130c8a5') }}</span></div></div>
              <div class="metrics"><span>{{ t('admin.copy.f97b639f') }} <b>{{ job.consistent_count }}</b></span><span>{{ t('admin.copy.d591240c') }} <b>{{ job.repairable_count }}</b></span><span>{{ t('admin.copy.242b6072') }} <b>{{ job.corrupt_count }}</b></span></div>
              <dl class="facts"><dt>{{ t('admin.copy.803706ab') }}</dt><dd>{{ statusLabel(job.state) }}</dd><dt>{{ t('admin.copy.c97f0e15') }}</dt><dd>{{ checkScopeLabel(job.check_scope) }}</dd><dt>{{ t('admin.copy.bc2ac9f1') }}</dt><dd><AssociationName :record="job" kind="admin" :id="job.created_by" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.165ebd73') }}</dt><dd>{{ displayDate(job.created_at) }}</dd><dt>{{ t('admin.copy.81c4dee4') }}</dt><dd>{{ displayDate(job.started_at) }}</dd><dt>{{ t('admin.copy.670c5f97') }}</dt><dd>{{ displayDate(job.completed_at) }}</dd><dt v-if="job.reason">{{ t('admin.copy.5ec7c8ba') }}</dt><dd v-if="job.reason">{{ job.reason }}</dd><dt>{{ t('admin.copy.c0a94db8') }}</dt><dd>{{ job.last_error_code ? displayPublicError(publicErrorMessage({ code: job.last_error_code })) : t('admin.copy.c2b41b33') }}</dd></dl>
              <div v-if="job.state === 'failed' && job.can_retry" class="retry-box"><h3>{{ t('admin.copy.5603ef73') }}</h3><p>。</p><button class="button button-primary" :disabled="!canRetry" @click="beginReview(selectedForRetry())">{{ t('admin.copy.a6f031a3') }}</button></div>
              <div class="targets-head"><div><h3>{{ t('admin.copy.cea82aeb') }}</h3><p>{{ targetTotal }} {{ t('admin.copy.44d96226') }}{{ outcomeFilter ? ` · ${statusLabel(outcomeFilter)}` : "" }}</p></div><label>{{ t('admin.copy.1bcf8e12') }}<select :aria-label="t('admin.copy.1bcf8e12')" :value="outcomeFilter" @change="onOutcomeChange"><option value="">{{ t('admin.copy.ef22c1d2') }}</option><option value="pending">{{ t('admin.copy.99f247fd') }}</option><option value="failed">{{ t('admin.copy.b130c8a5') }}</option><option value="consistent">{{ t('admin.copy.f97b639f') }}</option><option value="repairable">{{ t('admin.copy.d591240c') }}</option><option value="corrupt">{{ t('admin.copy.242b6072') }}</option></select></label></div>
              <p v-if="targetError" class="error" role="alert">{{ display(targetError) }}</p><p v-global-loading v-if="targetLoading" class="sub">{{ t('admin.copy.d3547df0') }}</p>
              <div v-if="!targetLoading && !targetError && targets.length === 0" class="empty empty-state">{{ t('admin.copy.8a4e90e4') }}</div>
              <article v-for="item in targets" :key="item.id" class="target-card">
                <div class="target-title"><span><b><AssociationName :record="item" kind="member" :id="item.member_id" :locale="associationLocale" /></b><small>{{ t('admin.copy.c0ec3fe1') }} {{ item.account_id }}</small></span><i :class="['state', item.outcome ?? item.state]">{{ statusLabel(item.outcome ?? item.state) }}</i></div>
                <p class="sub">{{ t('admin.copy.76d23127') }} {{ item.attempt_count }} {{ t('admin.copy.b9c63db4') }} {{ displayDate(item.checked_at) }} · {{ item.error_code ? displayPublicError(publicErrorMessage({ code: item.error_code })) : t('admin.copy.85b71678') }}</p>
                <details v-if="item.preview"><summary>{{ t('admin.copy.d411ddf8') }}</summary><p class="sub">{{ t('admin.copy.48810643') }}</p><div class="preview-grid"><div><h4>{{ t('admin.copy.a3ba38f0') }}</h4><BusinessDetails :value="item.preview.actual" :locale="associationLocale" /></div><div><h4>{{ t('admin.copy.71c78245') }}</h4><BusinessDetails :value="item.preview.expected" :locale="associationLocale" /></div></div><p>  {{ item.preview.consistent ? t('admin.copy.f97b639f') : item.preview.repairable ? t('admin.copy.d591240c') : t('admin.copy.74c3eb6a') }}</p><ul v-if="item.preview.issues.length"><li v-for="issue in item.preview.issues" :key="issue">{{ displayPublicError(publicErrorMessage({ code: issue })) }}</li></ul></details>
                <details v-if="item.business_preview"><summary>{{ t('admin.copy.3fd07c57') }}</summary><p>{{ t('admin.copy.c86823e3') }} {{ item.business_preview.ledger_entry_count }} · {{ t('admin.copy.746287bd') }} {{ item.business_preview.business_reference_count }} · {{ t('admin.copy.b9534a45') }} {{ item.business_preview.issue_count }} · {{ item.business_preview.consistent ? t('admin.copy.f97b639f') : t('admin.copy.034d4ff5') }}</p><ul class="coverage-list"><li v-for="coverage in item.business_preview.coverage" :key="coverage.family"><b>{{ coverage.family }}</b>: {{ t('admin.copy.c4a1cfc9') }} {{ coverage.ledger_entry_count }}, {{ t('admin.copy.1f2b44d5') }} {{ coverage.business_reference_count }}, {{ t('admin.copy.70c56725') }} {{ coverage.issue_count }}</li></ul><p v-if="item.business_preview.issues_truncated" class="sub">{{ t('admin.copy.615c8b97') }}</p><ul v-if="item.business_preview.issues.length"><li v-for="(issue, index) in item.business_preview.issues" :key="`${issue.code}-${issue.ledger_entry_id}-${index}`"><b>{{ issue.code }}</b> {{ issue.resource_type }} {{ issue.resource_id ?? "—" }} · {{ issue.entry_type ?? "—" }} · {{ t('admin.copy.edc72063') }} {{ issue.ledger_entry_id ?? "—" }}</li></ul></details>
              </article>
              <div class="pager"><button class="button button-quiet" :disabled="targetLoading || targetOffset <= 0" @click="withGlobalLoading(pageTargets, -1)">{{ t('admin.copy.dbe7a5d7') }}</button><span>{{ Math.floor(targetOffset / PAGE_SIZE) + 1 }} / {{ totalTargetPages }}</span><button class="button button-quiet" :disabled="targetLoading || targetOffset + PAGE_SIZE >= Number(targetTotal)" @click="withGlobalLoading(pageTargets, 1)">{{ t('admin.copy.2e9c9e7a') }}</button></div>
            </template>
          </template>
          <div v-else class="empty detail-empty empty-state">{{ t('admin.copy.a34b3751') }}</div>
        </section>
      </div>
    </template>

    <div v-if="review" class="scrim" role="presentation" @click.self="review = null; confirmed = false">
      <section class="confirm panel" role="dialog" aria-modal="true" aria-labelledby="confirm-title">
        <h2 id="confirm-title">{{ t('admin.copy.c4412948') }}</h2><p>{{ t('admin.copy.e078c0d2') }}</p>
        <dl class="facts"><dt>{{ t('admin.copy.015371dd') }}</dt><dd>{{ review.operation === "create" ? t('admin.copy.9febc38d') : t('admin.copy.2d6c4512', { id: review.jobId! }) }}</dd><dt v-if="review.operation === 'create'">{{ t('admin.copy.c97f0e15') }}</dt><dd v-if="review.operation === 'create'">{{ checkScopeLabel((review.body as ReconciliationCreateBody).check_scope) }}</dd><dt v-if="review.body.reason">{{ t('admin.copy.5ec7c8ba') }}</dt><dd v-if="review.body.reason">{{ review.body.reason }}</dd></dl>
        <label class="confirm-check"><input v-model="confirmed" type="checkbox">{{ t('admin.copy.a3a3fb1a') }}</label>
        <div class="actions"><button class="button button-quiet" @click="review = null; confirmed = false">{{ t('admin.copy.b1d257a2') }}</button><button v-write-guard="writing" class="button button-primary" :disabled="!confirmed || writing" @click="withGlobalLoading(confirmWrite)">{{ t('admin.copy.3135aeca') }}</button></div>
      </section>
    </div>
  </main>
</template>

<style scoped>
.recon{width:100%;max-width:1500px;min-width:0;margin:0 auto;padding:clamp(14px,2.4vw,30px);color:#252a36;overflow-wrap:anywhere}.recon *{min-width:0}.recon-head,.section-title,.actions,.pager,.targets-head,.target-title{display:flex;align-items:center;justify-content:space-between;gap:12px}.recon-head{margin-bottom:20px}.recon h1{font-size:clamp(22px,3vw,30px);margin:0 0 7px}.recon h2{font-size:17px;margin:0 0 5px}.recon h3{font-size:14px;margin:0 0 8px}.recon h4{font-size:12px;margin:8px 0}.recon p{margin:4px 0 10px}.eyebrow{font-size:11px;letter-spacing:1.2px;color:#6674da;font-weight:700}.sub,.recon small{color:var(--muted);font-size:11px}.panel{background:var(--surface);border:1px solid var(--line);border-radius:12px;padding:18px;box-shadow:0 2px 8px #1e2a5008;margin-bottom:16px}.muted-panel{color:var(--muted)}.section-title{align-items:flex-start;margin-bottom:12px}.tag,.state{display:inline-flex;border-radius:999px;padding:4px 9px;background:#f0f2f7;color:#687083;font-size:11px;font-style:normal;white-space:nowrap}.tag.warning{background:#fff2dc;color:#945b00}.field{display:flex;flex-direction:column;gap:5px;max-width:720px;margin:12px 0}.field textarea,.targets-head select{border:1px solid #dfe2ea;border-radius:8px;padding:10px;background:var(--surface);color:inherit;resize:vertical}.button-quiet,.button-primary{border:1px solid #dfe2ea;border-radius:8px;padding:9px 13px;background:var(--surface)}.button-primary{background:var(--primary);color:var(--surface);border-color:var(--primary);font-weight:650}.actions{justify-content:flex-start;flex-wrap:wrap}.columns{display:grid;grid-template-columns:minmax(260px,.72fr) minmax(0,1.5fr);gap:16px;align-items:start}.jobs-panel,.detail-panel{min-width:0}.job-row{border-top:1px solid #eef0f4}.job-row.selected{background:#f5f6ff}.job-select{display:flex;flex-direction:column;align-items:flex-start;gap:5px;width:100%;padding:12px 8px;border:0;background:transparent;text-align:left;overflow-wrap:anywhere}.job-title{display:flex;gap:8px;align-items:center;font-weight:700}.state.completed,.state.consistent{background:#e6f5ed;color:#1e7954}.state.failed,.state.corrupt{background:#fff0ed;color:#ab4236}.state.repairable{background:#fff4df;color:#936000}.state.running{background:#eaf0ff;color:#4259c8}.pager{margin-top:14px;font-size:11px;color:var(--muted)}.stats{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:8px}.stats>div{background:var(--canvas);border-radius:9px;padding:11px}.stats b,.stats span{display:block}.stats b{font-size:19px}.stats span{font-size:11px;color:var(--muted);margin-top:3px}.metrics{display:flex;gap:18px;flex-wrap:wrap;padding:13px 0;border-bottom:1px solid #eef0f4}.facts{display:grid;grid-template-columns:minmax(110px,.45fr) minmax(0,1fr);gap:9px 14px;font-size:11px}.facts dt{color:var(--muted)}.facts dd{margin:0;overflow-wrap:anywhere}.retry-box{background:#fffaf0;border:1px solid #f2e5c8;border-radius:10px;padding:14px;margin:16px 0}.targets-head{align-items:flex-end;margin-top:20px;padding-top:14px;border-top:1px solid var(--line)}.targets-head label{display:flex;align-items:center;gap:7px;font-size:11px}.target-card{border:1px solid var(--line);border-radius:9px;padding:12px;margin-top:9px}.target-title small{display:block;margin-top:4px}.target-card details{border-top:1px solid #eef0f4;padding-top:9px;margin-top:9px}.target-card summary{cursor:pointer;font-weight:650;font-size:11px}.preview-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px}.preview-grid pre{max-height:260px;overflow:auto;background:var(--canvas);border-radius:7px;padding:10px;font-size:11px;white-space:pre-wrap;overflow-wrap:anywhere}.error{color:#a63732;background:#fff2f0;border-radius:8px;padding:10px}.notice{position:fixed;right:18px;bottom:18px;max-width:min(520px,calc(100vw - 36px));background:#e8f5ee;color:#236548;padding:12px 15px;border-radius:9px;box-shadow:0 8px 30px #0002;z-index:20}.write-error{position:fixed;right:18px;bottom:18px;max-width:min(520px,calc(100vw - 36px));z-index:21}.pending-panel{border-color:#f0d8a9;background:#fffdf8}.pending-item{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:8px 12px;padding:12px 0;border-top:1px solid #f2e8d5}.pending-item code{overflow-wrap:anywhere}.pending-item p{grid-column:1/-1;margin:0}.pending-item button{grid-column:1/-1;justify-self:start}.scrim{position:fixed;inset:0;background:#17203388;z-index:30;display:grid;place-items:center;padding:16px}.confirm{width:min(560px,100%);max-height:90vh;overflow:auto;margin:0}.confirm-check{display:flex;gap:9px;align-items:flex-start;font-size:12px;margin:16px 0}.confirm-check input{margin-top:2px}
@media(max-width:760px){.columns{grid-template-columns:minmax(0,1fr)}.recon-head{align-items:flex-start}.stats{grid-template-columns:repeat(2,minmax(0,1fr))}.preview-grid{grid-template-columns:minmax(0,1fr)}.panel{padding:14px}.targets-head{align-items:flex-start;flex-direction:column}.targets-head label{width:100%;justify-content:space-between}.targets-head select{max-width:65%}}
@media(max-width:420px){.recon{padding:12px}.recon-head{gap:6px}.recon-head .button-quiet{padding:8px;font-size:11px}.pending-item{grid-template-columns:minmax(0,1fr)}.pending-item code,.pending-item p,.pending-item button{grid-column:1}.facts{grid-template-columns:minmax(88px,.4fr) minmax(0,1fr);gap:8px}.pager{gap:6px}.pager button{padding:8px}}
.notice,.write-error{position:static;max-width:100%;z-index:auto;right:auto;bottom:auto;margin-bottom:16px}
.recon .field{height:auto;border:0;background:transparent;padding:0;font-size:12px}
.recon .state,.recon .tag{flex-shrink:0}
.target-title>span{flex:1;min-width:0}
@media(max-width:760px){.section-title{flex-wrap:wrap}}
</style>
