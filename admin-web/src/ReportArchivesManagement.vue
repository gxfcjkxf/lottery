<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from './i18n'
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from '@lottery/shared'
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
import ArchiveReport from "@lottery/shared/ArchiveReport.vue"
import { archiveReportCatalog } from '@lottery/shared'
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { adminKey, useAdminI18n } from "./i18n";
import type { LocalizedMessage } from "@lottery/shared";
import { createReportArchivesApi, reportArchivesPermissions, type ReportArchiveRecord } from "./report-archives-api";
import { createArchiveIntent, getArchiveIntent, setArchiveIntent, clearArchiveIntent, markArchiveConflict, isArchiveConflict, clearAllArchiveIntents, archiveSessionGeneration, archiveIntentChanges, classifyArchiveWriteFailure, type ArchiveIntent } from "./report-archives-state";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, message, display, locale } = useAdminI18n();
const api = createReportArchivesApi(), pageSize = 20;
const rights = computed(() => reportArchivesPermissions(props.account, props.brandId));
const scope = computed(() => JSON.stringify([props.account.id, props.brandId, props.account.super_admin, props.account.brand_ids, props.account.permissions_by_brand, props.account.platform_permissions, rights.value]));
const items = ref<ReportArchiveRecord[]>([]), total = ref("0"), offset = ref(0);
const selected = ref<ReportArchiveRecord | null>(null), receipt = ref<ReportArchiveRecord | null>(null);
const detailView = ref(false);
const backLabel = computed(() => archiveReportCatalog[locale.value].back);
const reportLabels = computed(() => ({ betting: t('admin.copy.25d0ea82'), ledger: t('admin.copy.59b53ce0'), wallet: t('admin.copy.42249bcd'), withdrawals: t('admin.copy.d9c32442'), commissions: t('admin.copy.a74c0b47'), rewards: t('admin.copy.15791d7e'), reward_orders: t('admin.copy.edec9473') }));
function reportTime(value: string){ return formatDisplayDateTime(value, displayLocale.value); }
function backToList() { readTicket++; downloadTicket++; reading.value = downloading.value = false; selected.value = null; detailView.value = false; recordError.value = ''; }
const kind = ref<"daily" | "monthly">("daily"), periodKey = ref(""), expectedRevision = ref(0);
const review = ref<ArchiveIntent | null>(null), confirmed = ref(false), pending = ref<ArchiveIntent | null>(null);
const conflict = ref(false), conflictRefreshed = ref(false), discardChecked = ref(false);
const loading = ref(false), reading = ref(false), writing = ref(false), downloading = ref(false);
const error = ref<string | LocalizedMessage>(""), notice = ref<string | LocalizedMessage>("");
const listError = ref<string | LocalizedMessage>(""), recordError = ref<string | LocalizedMessage>("");
let alive = true, listTicket = 0, readTicket = 0, writeTicket = 0, downloadTicket = 0;
const input = () => ({ kind: kind.value, period_key: periodKey.value, expected_revision: expectedRevision.value });
const formValid = computed(() => createArchiveIntent(props.account.id, props.brandId, input(), "archive-validation-key") !== null);
const canCreate = computed(() => rights.value.create && !writing.value && !pending.value && formValid.value);
const nextPage = computed(() => BigInt(offset.value) + BigInt(pageSize) < BigInt(total.value) && offset.value + pageSize <= 1_000_000);
const metricLabels: Record<string, string> = {
  order_count: 'admin.copy.c838e9e3', stake_points: 'admin.copy.33ae8fea',
  placed_count: 'admin.copy.ec28c69c', won_count: 'admin.copy.e84691cc', lost_count: 'admin.copy.80609afc',
  abnormal_count: 'admin.copy.cffd6905', cancelled_count: 'admin.copy.f06fe965', refund_points: 'admin.copy.32dff627',
  settled_stake_points: 'admin.copy.f6e65eb6', unfinalized_stake_points: 'admin.copy.ff0c9a21',
  abnormal_stake_points: 'admin.copy.8d8d3946', current_prize_points: 'admin.copy.0b48b66c', correction_open_count: 'admin.copy.5cedcbcb',
  entry_count: 'admin.copy.dea8d793', net_points: 'admin.copy.34ba10a2', recharge_points: 'admin.copy.9a685341',
  prize_credit_points: 'admin.copy.b65dc089', prize_reversal_points: 'admin.copy.9d267717',
  account_count: 'admin.copy.64df0e2b', available_points: 'admin.copy.5b7ceca3', frozen_points: 'admin.copy.7c11c5d7',
  withdrawal_points: 'admin.copy.d061b361', total_points: 'admin.copy.89bf2b95',
  requested_points: 'admin.copy.0aaf1409', reviewing_count: 'admin.copy.5a7b4a84', reviewing_points: 'admin.copy.b08bae0f',
  processing_count: 'admin.copy.08868c76', processing_points: 'admin.copy.e5b1c95d',
  paid_count: 'admin.copy.351cab6f', paid_points: 'admin.copy.60c1097c', rejected_count: 'admin.copy.70a01cf5', rejected_points: 'admin.copy.eaad2b14',
  failed_count: 'admin.copy.589a1333', failed_points: 'admin.copy.3292260a', cancelled_points: 'admin.copy.a977e4cc',
  paid_entry_count: 'admin.copy.035d281a', adjustment_entry_count: 'admin.copy.fdfa9915',
  adjustment_credit_points: 'admin.copy.1c22dde0', adjustment_debit_points: 'admin.copy.1cee0316',
  correction_entry_count: 'admin.copy.d241d62f', correction_credit_points: 'admin.copy.735ad96e', correction_debit_points: 'admin.copy.45f5e324',
  grant_entry_count: 'admin.copy.928c9412', grant_points: 'admin.copy.b83d5c43', reversal_entry_count: 'admin.copy.d2b5d567', reversal_points: 'admin.copy.88ca0b25',
  original_points: 'admin.copy.99619b43', granted_count: 'admin.copy.dff54f6f', granted_points: 'admin.copy.43c77853',
  pending_count: 'admin.copy.e6645f3b', pending_points: 'admin.copy.ee9a1603', revoked_count: 'admin.copy.85866367', revoked_points: 'admin.copy.6b110769',
};
 function metricLabel(field: string) { const key = metricLabels[field]; if (!key) throw new Error(`Unsupported archive metric: ${field}`); return t(adminKey(key)); }

function refreshPending() {
  pending.value = getArchiveIntent(props.account.id, props.brandId);
  conflict.value = pending.value !== null && isArchiveConflict(pending.value);
}
function current(ticket: number, lane: "list" | "read" | "write" | "download", captured: string, generation: number) {
  const actual = lane === "list" ? listTicket : lane === "read" ? readTicket : lane === "write" ? writeTicket : downloadTicket;
  return alive && actual === ticket && captured === scope.value && generation === archiveSessionGeneration();
}
function sessionInvalid() { clearAllArchiveIntents(); emit("session-invalid"); }
function readFailure(problem: unknown) {
  if (problem instanceof AdminApiError && problem.status === 401) { sessionInvalid(); return; }
  return problem instanceof Error ? publicErrorMessage(problem, 'read') : message('admin.copy.ba61331b');
}
async function loadList(next = offset.value) {
  if (!rights.value.view) return false;
  const ticket = ++listTicket, captured = scope.value, generation = archiveSessionGeneration();
  loading.value = true; listError.value = "";
  try {
    const page = await api.list(props.brandId, pageSize, next);
    if (!current(ticket, "list", captured, generation)) return false;
    items.value = page.items; total.value = page.total_count; offset.value = page.offset; return true;
  } catch (problem) { if (current(ticket, "list", captured, generation)) listError.value = readFailure(problem) ?? ""; return false; }
  finally { if (current(ticket, "list", captured, generation)) loading.value = false; }
}
async function loadRecord(id: string) {
  if (!rights.value.view) return false;
  const ticket = ++readTicket, captured = scope.value, generation = archiveSessionGeneration();
  detailView.value = true;
  downloadTicket++; downloading.value = false; selected.value = null; reading.value = true; recordError.value = "";
  try {
    const record = await api.read(props.brandId, id);
    if (!current(ticket, "read", captured, generation)) return false;
    selected.value = record; return true;
  } catch (problem) { if (current(ticket, "read", captured, generation)) recordError.value = readFailure(problem) ?? ""; return false; }
  finally { if (current(ticket, "read", captured, generation)) reading.value = false; }
}
function beginReview(replay = false) {
  if (!rights.value.create || writing.value || conflict.value) return;
  const intent = replay ? getArchiveIntent(props.account.id, props.brandId) : canCreate.value ? createArchiveIntent(props.account.id, props.brandId, input(), createIdempotencyKey()) : null;
  if (!intent) return;
  review.value = intent; confirmed.value = false; error.value = "";
}
function appendVersion() {
  if (!selected.value || pending.value || writing.value || !rights.value.create) return;
  kind.value = selected.value.window.kind; periodKey.value = selected.value.window.period_key; expectedRevision.value = selected.value.revision;
  backToList();
}
async function submit() {
  const intent = review.value;
  if (!intent || !confirmed.value || !rights.value.create || writing.value || conflict.value || !setArchiveIntent(intent)) return;
  const ticket = ++writeTicket, captured = scope.value, generation = archiveSessionGeneration();
  refreshPending(); writing.value = true; review.value = null; confirmed.value = false; error.value = ""; notice.value = "";
  try {
    const record = await api.create(intent.brandId, intent.body, intent.key, intent.actorId);
    if (!current(ticket, "write", captured, generation)) return;
    clearArchiveIntent(intent.actorId, intent.brandId, intent.key); refreshPending(); receipt.value = record;
    notice.value = message('admin.copy.e9891939');
    await Promise.all([loadList(offset.value), loadRecord(record.id)]);
  } catch (problem) {
    // A late result must never clear or mark an intent from a later login.
    if (generation !== archiveSessionGeneration()) return;
    const status = problem instanceof AdminApiError ? problem.status : undefined;
    if (status === 401) { if (current(ticket, "write", captured, generation)) sessionInvalid(); return; }
    const classification = classifyArchiveWriteFailure(status);
    if (classification === "conflict") markArchiveConflict(intent);
    else if (classification === "definitive") clearArchiveIntent(intent.actorId, intent.brandId, intent.key);
    if (!current(ticket, "write", captured, generation)) return;
    refreshPending(); conflictRefreshed.value = false; discardChecked.value = false;
    error.value = classification === "unknown"
      ? message('admin.copy.5dc21ff1')
      : classification === "conflict" ? message('admin.copy.4dd3ecc1')
      : problem instanceof Error ? publicErrorMessage(problem) : message('admin.copy.788d8888');
  } finally { if (current(ticket, "write", captured, generation)) writing.value = false; }
}
async function refreshConflict() {
  if (!pending.value || !conflict.value || loading.value || reading.value) return;
  const captured = scope.value, generation = archiveSessionGeneration(), key = pending.value.key, selectedID = selected.value?.id;
  conflictRefreshed.value = false; discardChecked.value = false;
  const results = await Promise.all([loadList(offset.value), selectedID ? loadRecord(selectedID) : Promise.resolve(true)]);
  if (alive && captured === scope.value && generation === archiveSessionGeneration() && pending.value?.key === key) conflictRefreshed.value = results.every(Boolean);
}
function discardConflict() {
  if (!pending.value || !conflict.value || !conflictRefreshed.value || !discardChecked.value || writing.value) return;
  clearArchiveIntent(pending.value.actorId, pending.value.brandId, pending.value.key); refreshPending(); error.value = ""; conflictRefreshed.value = discardChecked.value = false;
}
async function download() {
  if (!selected.value || !rights.value.download || downloading.value) return;
  // Vue wraps selected records in a Proxy, which structuredClone cannot copy.
  // The closed DTO contains strings and safe revision numbers, never BigInt.
  const record = JSON.parse(JSON.stringify(selected.value)) as ReportArchiveRecord, ticket = ++downloadTicket, captured = scope.value, generation = archiveSessionGeneration();
  downloading.value = true; error.value = "";
  try {
    const file = await api.download(props.brandId, record.id, record);
    if (!current(ticket, "download", captured, generation) || selected.value?.id !== record.id || !rights.value.download) return;
    const url = URL.createObjectURL(new Blob([file.bytes.slice().buffer], { type: "application/json; charset=utf-8" }));
    const link = document.createElement("a"); link.href = url; link.download = file.metadata.filename; link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    notice.value = message('admin.copy.c882036a');
  } catch (problem) { if (current(ticket, "download", captured, generation)) error.value = readFailure(problem) ?? ""; }
  finally { if (current(ticket, "download", captured, generation)) downloading.value = false; }
}
function reset() {
  listTicket++; readTicket++; writeTicket++; downloadTicket++;
  items.value = []; total.value = "0"; offset.value = 0; selected.value = receipt.value = null; detailView.value = false;
  loading.value = reading.value = writing.value = downloading.value = false;
  kind.value = "daily"; periodKey.value = ""; expectedRevision.value = 0;
  review.value = null; confirmed.value = conflictRefreshed.value = discardChecked.value = false; error.value = notice.value = listError.value = recordError.value = ""; refreshPending();
}
watch(scope, () => { reset(); if (rights.value.view) void loadList(0); }, { immediate: true });
let observedGeneration = archiveSessionGeneration();
watch(archiveIntentChanges, () => {
  if (observedGeneration !== archiveSessionGeneration()) { observedGeneration = archiveSessionGeneration(); reset(); }
  else refreshPending();
});
onBeforeUnmount(() => { alive = false; listTicket++; readTicket++; writeTicket++; downloadTicket++; });
</script>

<template>
  <section class="archives panel">
    <header><div><h2>{{ t('admin.copy.d8f53155') }}</h2></div></header>
    <p v-if="!rights.view" role="alert">{{ t('admin.copy.da2b5fad') }}</p>
    <template v-else>
      <p v-if="error" role="alert">{{ display(error) }}</p><p v-if="notice" role="status">{{ display(notice) }}</p>
        <p v-if="listError" role="alert" data-testid="archive-list-error">{{ t('admin.copy.100064db') }} {{ display(listError) }}</p>
      <p v-if="recordError" role="alert" data-testid="archive-record-error">{{ t('admin.copy.4a0c74ec') }} {{ display(recordError) }}</p>
      <div v-if="pending" data-testid="archive-unknown" class="archive-warning">
        <h3>{{ conflict ? t('admin.copy.e9f8a856') : t('admin.copy.135c82ba') }}</h3>
        <p>{{ t('admin.copy.9e01d9a3') }}</p>
        <dl><dt>{{ t('admin.copy.6a5e9429') }}</dt><dd>{{ pending.body.kind }} / {{ pending.body.period_key }}</dd></dl>
        <button v-if="!conflict" class="button button-secondary" data-testid="archive-replay" :disabled="!rights.create || writing" @click="beginReview(true)">{{ t('admin.copy.7434d6ed') }}</button>
        <div v-else data-testid="archive-conflict"><button v-refresh-button="t('admin.copy.6a657880')" class="button button-secondary" data-testid="archive-conflict-refresh" :disabled="loading || reading" @click="withGlobalLoading(refreshConflict)">{{ t('admin.copy.6a657880') }}</button><label><input v-model="discardChecked" data-testid="archive-discard-check" type="checkbox" :disabled="!conflictRefreshed" />{{ t('admin.copy.d3399b8c') }}</label><button class="button button-secondary" data-testid="archive-discard" :disabled="!conflictRefreshed || !discardChecked || writing" @click="discardConflict">{{ t('admin.copy.169500cc') }}</button></div>
      </div>
      <div v-if="!detailView" class="archive-form">
        <h3>{{ t('admin.copy.5c4adf0e') }}</h3>
        <p>{{ t('admin.copy.20f18f37') }}</p>
        <label>{{ t('admin.copy.9ac774fa') }}<select v-model="kind" @change="expectedRevision = 0" data-testid="archive-kind" :aria-label="t('admin.copy.9ac774fa')" :disabled="!!pending || writing || !!review"><option value="daily">{{ t('admin.copy.3c770216') }}</option><option value="monthly">{{ t('admin.copy.5505e809') }}</option></select></label>
        <label>{{ t('admin.copy.43b6066a') }}<input v-model="periodKey" @input="expectedRevision = 0" data-testid="archive-period-key" :aria-label="t('admin.copy.43b6066a')" :placeholder="kind === 'daily' ? 'YYYY-MM-DD' : 'YYYY-MM'" :disabled="!!pending || writing || !!review" /></label>
        <button class="button" data-testid="archive-review-button" :disabled="!canCreate" @click="beginReview()">{{ t('admin.copy.5b3af0db') }}</button>
        <p v-if="!rights.create">{{ t('admin.copy.b2dbbdb7') }}</p>
      </div>
      <div v-if="receipt" :hidden="detailView" data-testid="archive-receipt" class="archive-receipt"><h3>{{ t('admin.copy.3a7c2bd8') }}</h3><p>{{ receipt.id }}</p></div>
      <div v-if="!detailView" class="archive-list" data-testid="archive-list"><h3>{{ t('admin.copy.b8bf3584') }}</h3><p>{{ t('admin.copy.0392849b') }}: {{ total }} · {{ t('admin.copy.4ffb01e2') }}: {{ offset }}</p><p v-global-loading v-if="loading" role="status">{{ t('admin.copy.ab7239f3') }}</p><p v-else-if="items.length === 0" class="empty-state">{{ t('admin.copy.abbcb5e7') }}</p><ul><li v-for="item in items" :key="item.id"><button class="button button-secondary" :disabled="reading" @click="withGlobalLoading(loadRecord, item.id)">{{ item.window.period_key }} · {{ item.window.kind }}<small>{{ item.id }}</small></button><small>{{ displayDate(item.snapshot_at) }}</small></li></ul><div class="archive-actions"><button class="button button-secondary" :disabled="loading || offset === 0" @click="withGlobalLoading(loadList, Math.max(0, offset - pageSize))">{{ t('admin.copy.70598166') }}</button><button class="button button-secondary" :disabled="loading || !nextPage" @click="withGlobalLoading(loadList, offset + pageSize)">{{ t('admin.copy.ee97c955') }}</button><button v-refresh-button="t('admin.copy.2d775c49')" class="button button-secondary" data-testid="archive-refresh" :disabled="!rights.view || loading" @click="withGlobalLoading(loadList)">{{ t('admin.copy.2d775c49') }}</button></div></div>
      <p v-global-loading v-if="reading" role="status">{{ t('admin.copy.7c2c8a32') }}</p>
      <div v-if="detailView" data-testid="archive-detail" class="archive-detail">
        <ArchiveReport v-if="selected" :record="selected" :locale="locale" :metric-label="metricLabel" :group-labels="reportLabels" :format-time="reportTime">
          <template #actions><button class="button button-secondary" data-testid="archive-back" @click="backToList">{{ backLabel }}</button><button class="button button-secondary" :disabled="!rights.create || !!pending || writing" @click="appendVersion">{{ t('admin.copy.829881ed') }}</button><button class="button button-primary" data-testid="archive-download" :disabled="!rights.download || downloading" @click="withGlobalLoading(download)">{{ t('admin.copy.7ac58498') }}</button></template>
        </ArchiveReport>
        <button v-else class="button button-secondary" @click="backToList">{{ backLabel }}</button>
      </div>
    </template>
    <div v-if="review" class="archive-modal" role="dialog" aria-modal="true" :aria-label="t('admin.copy.713d0f00')" data-testid="archive-review"><div><h3>{{ t('admin.copy.713d0f00') }}</h3><dl><dt>{{ t('admin.copy.9c32661d') }}</dt><dd><AssociationName kind="brand" :id="review.brandId" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.7c92cc47') }}</dt><dd><AssociationName kind="admin" :id="review.actorId" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.6a5e9429') }}</dt><dd>{{ review.body.kind }} / {{ review.body.period_key }}</dd></dl><label><input v-model="confirmed" type="checkbox" data-testid="archive-confirm" />{{ t('admin.copy.db19e6a4') }}</label><div class="archive-actions"><button class="button button-secondary" @click="review = null">{{ t('admin.copy.e2f391e7') }}</button><button v-write-guard="writing" class="button" data-testid="archive-submit" :disabled="!confirmed || writing || !rights.create" @click="withGlobalLoading(submit)">{{ t('admin.copy.3135aeca') }}</button></div></div></div>
  </section>
</template>

<style scoped>
.archives{min-width:0;overflow-wrap:anywhere;padding:1.25rem}.archives header{display:flex;justify-content:space-between;align-items:flex-start;gap:1rem}.archives p{line-height:1.6;color:var(--muted)}.archives label{display:grid;gap:.5rem}.archives input,.archives select,.archives textarea{width:100%;min-width:0;box-sizing:border-box;padding:.65rem;border:1px solid var(--border,#d5dce2);border-radius:.5rem;background:var(--surface);color:inherit}.archives input[type=checkbox]{width:auto;justify-self:start}.archives textarea{min-height:5rem}.archive-form{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:1rem;margin:1.5rem 0}.archive-form h3,.archive-form>p,.archive-wide{grid-column:1/-1}.archives dl{display:grid;grid-template-columns:minmax(100px,1fr) minmax(0,2fr);gap:.5rem 1rem}.archives dt{color:var(--muted)}.archives dd{margin:0;min-width:0;white-space:pre-wrap;overflow-wrap:anywhere}.archive-warning,.archive-receipt{padding:1rem;border:1px solid #d5b05b;border-radius:.75rem;background:#fbf7ed;margin:1rem 0}.archive-list ul{list-style:none;padding:0;display:grid;gap:.75rem}.archive-list li{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:.5rem;padding:.75rem;border:1px solid var(--border,#d5dce2);border-radius:.65rem}.archive-list small{white-space:normal;overflow-wrap:anywhere}.archive-list li small:last-child{grid-column:1/-1}.archive-actions{display:flex;gap:.75rem;flex-wrap:wrap;align-items:center}.archives button{white-space:normal;max-width:100%}.archive-blocks{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:1rem;margin-top:1rem}.archive-blocks section{border:1px solid var(--border,#d5dce2);border-radius:.65rem;padding:1rem;min-width:0}.archive-blocks dl{grid-template-columns:minmax(0,1.5fr) minmax(0,1fr)}.archive-modal{position:fixed;inset:0;background:#17253699;z-index:90;display:flex;align-items:center;justify-content:center;padding:1rem}.archive-modal>div{background:var(--surface);border-radius:1rem;padding:1.25rem;width:36rem;max-width:100%;max-height:85dvh;overflow:auto}.archive-modal .archive-actions{margin-top:1rem}@media(max-width:700px){.archives{padding:1rem}.archives header{display:block}.archive-form,.archive-blocks{grid-template-columns:minmax(0,1fr)}.archive-list li{grid-template-columns:minmax(0,1fr)}.archives dl{grid-template-columns:minmax(0,1fr)}.archives dt{font-size:.85rem}.archive-blocks dl{grid-template-columns:minmax(0,1.5fr) minmax(0,1fr)}.archive-modal{padding:.5rem}.archive-modal>div{padding:1rem}}
</style>
