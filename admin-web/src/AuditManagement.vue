<script setup lang="ts">
const filterReset = ref(0)

import ManagementList from "@lottery/shared/ManagementList.vue"
import { readPaginationTotal } from '@lottery/shared'
const recordTotal = ref('0')

import DetailModal from "@lottery/shared/DetailModal.vue"
import TablePagination from "@lottery/shared/TablePagination.vue"
const actorDraftSearchPending = ref(false);
import FilterAssociationSearch from "@lottery/shared/FilterAssociationSearch.vue";
import { useOperationToast } from "@lottery/shared";
import { auditReasonLabel, businessActionLabel, businessResourceLabel, hasAuditDetails, isAuditRead } from '@lottery/shared'
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { filterDateBoundary } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import PlatformTemporalInput from "@lottery/shared/PlatformTemporalInput.vue";
import { vGlobalLoading } from "@lottery/shared";
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import { vInlineValidation } from '@lottery/shared'
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { auditPermissions, createAuditApi, type AdminAuditRecord, type AuditFilters } from "./audit-api";
import { adminKey, useAdminI18n } from "./i18n";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, locale } = useAdminI18n();
const tr = (key: string) => t(adminKey(key));
const api = createAuditApi();
const PAGE_SIZE = ref(100);
const permissions = computed(() => auditPermissions(props.account, props.brandId));
const fromDraft = ref("");
const toDraft = ref("");
const actionDraft = ref("");
const actorDraft = ref("");
const resourceTypeDraft = ref("");
const requestIdDraft = ref("");
const records = ref<AdminAuditRecord[]>([]);
const expandedId = ref<string | null>(null);
const expandedRecord = computed(() => records.value.find(entry => entry.id === expandedId.value) ?? null);
const offset = ref(0);
const filters = ref<AuditFilters | null>(null);
const loading = ref(false);
const error = ref<string | import("@lottery/shared").LocalizedMessage>("");
let readGeneration = 0;
let alive = true;

const scopeKey = computed(() => JSON.stringify([
  props.account.id, props.account.version, props.account.super_admin, props.account.brand_ids,
  props.account.permissions, props.account.permissions_by_brand, props.account.platform_permissions,
  props.brandId, permissions.value, locale.value,
]));
watch(scopeKey, () => {
  readGeneration++;
  [records.value,recordTotal.value]=[[],'0'];
  filters.value = null;
  offset.value = 0;
  loading.value = false;
  error.value = "";
});
onMounted(() => { fromDraft.value = new Date().toISOString().slice(0, 10); toDraft.value = fromDraft.value; if (permissions.value.view) void withGlobalLoading(readPage, 0, true); });
onBeforeUnmount(() => { alive = false; readGeneration++; });
function currentRead(ticket: number, scope: string): boolean { return alive && ticket === readGeneration && scope === scopeKey.value; }
function reportError(cause: unknown): string | import("@lottery/shared").LocalizedMessage {
  if (cause instanceof AdminApiError) return publicErrorMessage(cause);
  return cause instanceof Error ? publicErrorMessage(cause) : tr('admin.copy.6fef6157');
}

function submitFilters(): AuditFilters | null {
  let from: string, to: string;
  try { from = filterDateBoundary(fromDraft.value, false, 'utc').toISOString(); to = filterDateBoundary(toDraft.value, true, 'utc').toISOString() } catch { error.value = tr('admin.copy.3e7f7074'); return null }
  if (!from || !to || new Date(to).getTime() <= new Date(from).getTime() || new Date(to).getTime() - new Date(from).getTime() > 31 * 24 * 60 * 60 * 1000) {
    error.value = tr('admin.copy.3e7f7074');
    return null;
  }
  const next: AuditFilters = { from, to };
  for (const [key, raw] of [["action", actionDraft.value], ["actor_id", actorDraft.value], ["resource_type", resourceTypeDraft.value], ["request_id", requestIdDraft.value]] as const) {
    if (!raw) continue;
    if (raw.trim() !== raw || /\p{Cc}/u.test(raw) || (["action", "resource_type", "request_id"].includes(key) && Array.from(raw).length > 128)) {
      error.value = tr('admin.copy.c25c18ad');
      return null;
    }
    if (key === "actor_id" && !/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(raw)) {
      error.value = tr(key === "actor_id" ? "admin.copy.ebd149a7" : "admin.copy.34a73fc7");
      return null;
    }
    next[key] = raw;
  }
  return next;
}
async function readPage(nextOffset: number, newFilters = false) {
  if (actorDraftSearchPending.value) return;
  if (!permissions.value.view) { error.value = tr('admin.copy.6bf6ac7d'); return; }
  if (nextOffset < 0 || nextOffset > 100_000) return;
  const chosen = newFilters ? submitFilters() : filters.value;
  if (!chosen) return;
  const ticket = ++readGeneration, scope = scopeKey.value;
  if (newFilters) { filters.value = chosen; offset.value = 0; }
  loading.value = true;
  error.value = "";
  try {
    const result = await api.list(props.brandId, { ...chosen, limit: PAGE_SIZE.value, offset: newFilters ? 0 : nextOffset });
    if (!currentRead(ticket, scope)) return;
    offset.value = newFilters ? 0 : nextOffset;
    [records.value, recordTotal.value] = [result.items, readPaginationTotal(result)];
    expandedId.value = null;
  } catch (cause) {
    if (!currentRead(ticket, scope)) return;
    error.value = reportError(cause);
    if (cause instanceof AdminApiError && cause.status === 401) emit("session-invalid");
  } finally { if (currentRead(ticket, scope)) loading.value = false; }
}
function hasChanges(entry: AdminAuditRecord): boolean {
  return !isAuditRead(entry.action, entry.resource_type) && (hasAuditDetails(entry.before_json) || hasAuditDetails(entry.after_json));
}
function redacted(value: unknown): unknown {
  let parsed = value;
  if (typeof value === "string") { try { parsed = JSON.parse(value); } catch { return null; } }
  const sensitive = /password|secret|token|authorization|cookie|credential|private|api[_-]?key|salt|email|phone|mobile|address|birth|identity|ssn/i;
  const redact = (item: unknown): unknown => {
    if (Array.isArray(item)) return item.map(redact);
    if (item && typeof item === "object") return Object.fromEntries(Object.entries(item).map(([key, child]) => [key, sensitive.test(key) ? "[REDACTED]" : redact(child)]));
    return item;
  };
  return redact(parsed);
}
function timestamp(value: string): string{ return formatDisplayDateTime(value, displayLocale.value); }
function actorLabel(entry: AdminAuditRecord): string {
  if (entry.actor_username) return entry.actor_username;
  if (entry.actor_type === "system") return tr('admin.copy.532e2e04');
  if (entry.actor_type === "user") return tr('admin.copy.689b5703');
  if (entry.actor_type === "anonymous") return tr('admin.copy.3499c2d9');
  return tr('admin.copy.6155e68e');
}

useOperationToast(() => error.value, value => displayPublicError(value), 'error');
</script>

<template>
  <ManagementList class="audit-page" :aria-label="tr('admin.copy.315a9366')">
    <template v-if="permissions.view" #filters>
      <form novalidate v-inline-validation class="audit-filters ui-filter-bar" @submit.prevent="withGlobalLoading(readPage, 0, true)">
        <label><PlatformTemporalInput :label="t('filter.fromDateUTC')" :placeholder="t('filter.fromDateUTC')" v-model="fromDraft" type="date" required :locale="displayLocale" /></label>
        <label><PlatformTemporalInput :label="t('filter.toDateUTC')" :placeholder="t('filter.toDateUTC')" v-model="toDraft" type="date" required :locale="displayLocale" /></label>

        <label><FilterAssociationSearch :reset-key="filterReset" v-model="actorDraft" v-model:pending="actorDraftSearchPending" :brand-id="props.brandId" surface="admin" :enabled="permissions.view" resource="audit" kind="actor" :label="t('filter.actor')" :locale="displayLocale" :scope-key="JSON.stringify(props.account)" /></label>


        <div class="audit-actions">
          <button class="button button-primary" type="submit" :disabled="loading || actorDraftSearchPending">{{ loading ? tr('admin.copy.dea7c355') : tr('admin.copy.2d2b9232') }}</button>

        </div>
      </form>
    </template>
    <p v-if="!permissions.view" class="audit-notice" role="status">{{ tr('admin.copy.792805a5') }}</p>
    <template v-else>


      <p v-global-loading v-if="loading" class="audit-state" role="status">{{ tr('admin.copy.9b68f2d5') }}</p>
      <div v-if="filters" class="audit-results">

        <div class="audit-table-wrap">
          <table class="console-table audit-table console-responsive-table">
            <thead><tr>
              <th scope="col">{{ t('admin.audit.time') }}</th>
              <th scope="col">{{ tr('admin.copy.cef73d0f') }}</th>
              <th scope="col">{{ t('admin.audit.action') }}</th>
              <th scope="col">{{ tr('admin.copy.0cf060d7') }}</th>
              <th scope="col">{{ tr('admin.copy.5ec7c8ba') }}</th>
              <th scope="col">{{ t('admin.audit.changes') }}</th>
            </tr></thead>
            <tbody>
              <template v-for="entry in records" :key="entry.id">
                <tr>
                  <td :data-label="t('admin.audit.time')"><time :datetime="entry.created_at">{{ timestamp(entry.created_at) }}</time></td>
                  <td :data-label="tr('admin.copy.cef73d0f')">{{ actorLabel(entry) }}</td>
                  <td :data-label="t('admin.audit.action')">{{ businessActionLabel(entry.action, associationLocale) }}</td>
                  <td :data-label="tr('admin.copy.0cf060d7')">{{ businessResourceLabel(entry.resource_type, associationLocale, entry.action) }}</td>
                  <td :data-label="tr('admin.copy.5ec7c8ba')" :title="auditReasonLabel(entry.reason, locale)">{{ auditReasonLabel(entry.reason, locale) }}</td>
                  <td :data-label="t('admin.audit.changes')"><button v-if="hasChanges(entry)" class="button button-secondary" type="button" :aria-expanded="expandedId === entry.id" aria-haspopup="dialog" :aria-controls="`audit-change-${entry.id}`" @click="expandedId = expandedId === entry.id ? null : entry.id">{{ t('admin.audit.changes') }}</button><span v-else>{{ t(isAuditRead(entry.action, entry.resource_type) ? 'admin.audit.noChange' : entry.before_json === null && entry.after_json === null ? 'admin.audit.missingSnapshot' : 'admin.audit.hiddenDetails') }}</span></td>
                </tr>
              </template>
              <tr v-if="!loading && !error && !records.length"><td colspan="6" class="empty-state">{{ tr('admin.copy.07179c69') }}</td></tr>
            </tbody>
          </table>
        </div>

      </div>
      <DetailModal v-if="expandedRecord" :title="t('admin.audit.changes')" :close-label="tr('admin.copy.2bb2cb1d')" @close="expandedId = null">
        <div :id="`audit-change-${expandedRecord.id}`" class="audit-snapshots"><section v-if="hasAuditDetails(expandedRecord.before_json)"><h3>{{ tr('admin.copy.f85d9f7f') }}</h3><BusinessDetails :value="redacted(expandedRecord.before_json)" :locale="associationLocale" /></section><section v-if="hasAuditDetails(expandedRecord.after_json)"><h3>{{ tr('admin.copy.6db1d337') }}</h3><BusinessDetails :value="redacted(expandedRecord.after_json)" :locale="associationLocale" /></section></div>
      </DetailModal>
    </template>
  <template #pagination v-if="(filters)"><TablePagination class="audit-pagination" :aria-label="tr('admin.copy.e2f05ca9')" :total="recordTotal" :page="Math.floor(offset / PAGE_SIZE + 1)" :count="records.length" :locale="locale" :page-size="PAGE_SIZE" :reachable-page="Math.floor(100_000 / PAGE_SIZE) + 1" :previous-disabled="actorDraftSearchPending || loading || offset === 0" :next-disabled="loading || records.length < PAGE_SIZE || offset + PAGE_SIZE > 100_000" :previous-label="tr('admin.copy.dbe7a5d7')" :next-label="tr('admin.copy.2e9c9e7a')" :go-to="(target: number) => readPage((target - 1) * PAGE_SIZE)" :change-page-size="async (size: number) => { PAGE_SIZE = size; await readPage(0) }" :busy="actorDraftSearchPending || loading" /></template></ManagementList>
</template>

<style scoped>
.audit-page{display:grid;grid-template-columns:minmax(0,1fr);gap:12px;min-width:0}.audit-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:16px}.audit-heading h1{margin:4px 0;font-size:24px}.audit-heading p,.audit-help{margin:0;color:var(--text-muted,var(--muted));line-height:1.5}.audit-eyebrow{font-size:var(--body-font-size);letter-spacing:.12em;color:var(--text-muted,var(--muted))}.audit-brand{max-width:100%;overflow-wrap:anywhere;color:var(--text-muted,var(--muted))}.audit-brand code{color:inherit}.audit-filters{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px;padding:16px;border:1px solid var(--border,var(--line));border-radius:12px;background:var(--surface)}.audit-filters label{display:grid;gap:6px;min-width:0;font-size:var(--body-font-size);font-weight:600}.audit-filters input{box-sizing:border-box;width:100%;min-height:40px;padding:8px;border:1px solid var(--border,var(--line));border-radius:7px;background:var(--surface);color:inherit;font:inherit}.audit-actions{display:flex;align-items:end;gap:8px;grid-column:span 4}.audit-actions button.secondary{background:var(--surface);color:var(--accent)}button:disabled{opacity:.55;cursor:not-allowed}.audit-notice,.audit-state,.audit-error{padding:12px 14px;border-radius:8px;background:var(--surface-muted,var(--canvas));line-height:1.5}.audit-error{color:var(--danger);background:var(--danger-bg)}.audit-table-wrap{min-width:0;border:1px solid var(--line);border-radius:var(--radius-sm);background:var(--surface);overflow:hidden}.audit-table{min-width:0;white-space:normal;width:100%;border-collapse:collapse;text-align:left;font-size:var(--body-font-size);line-height:1.5;table-layout:fixed}.audit-table th,.audit-table td{padding:10px 12px;border-bottom:1px solid var(--line);vertical-align:middle;overflow-wrap:anywhere}.audit-table th{background:var(--table-header-bg);color:var(--ink-secondary);font-weight:650}.audit-table tbody tr:last-child td{border-bottom:0}.audit-table button{max-width:100%;white-space:normal;height:auto;min-height:var(--control-height)}.audit-table time{font-variant-numeric:tabular-nums}.audit-detail-row td{background:var(--canvas)}.audit-snapshots section{min-width:0}.audit-table :is(th,td):is(:first-child,:last-child){position:static;text-align:left}.audit-snapshots{width:100%;display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.audit-snapshots>section:only-child{grid-column:1/-1}.audit-snapshots h3{font-size:var(--body-font-size)}.audit-snapshots pre{max-height:320px;overflow:auto;white-space:pre-wrap;overflow-wrap:anywhere;padding:10px;border-radius:7px;background:var(--surface-muted,var(--canvas))}.audit-receipt{display:grid;gap:6px;padding:14px;border:1px solid var(--success);border-radius:9px;background:var(--surface);overflow-wrap:anywhere}.audit-receipt code{word-break:break-all}
@media(max-width:760px){.audit-heading{display:grid}.audit-filters{grid-template-columns:repeat(2,minmax(0,1fr));padding:12px}.audit-actions{grid-column:span 2;flex-wrap:wrap}.audit-actions button{flex:1 1 160px}.audit-table-wrap{border:0;overflow:visible;background:transparent}.audit-table,.audit-table tbody{display:block}.audit-table thead{display:none}.audit-table tbody>tr{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));border:1px solid var(--line);border-radius:var(--radius-sm);padding:8px;margin-bottom:10px;background:var(--surface)}.audit-table td{height:auto;position:static;text-align:left;border:0;padding:6px}.audit-table td::before{content:attr(data-label);color:var(--muted)}.audit-table td:first-child,.audit-table td:nth-child(5),.audit-table td:last-child{grid-column:1/-1}.audit-table .audit-detail-row{grid-template-columns:minmax(0,1fr);background:var(--canvas)}.audit-detail-row td::before{content:none}.audit-snapshots{grid-template-columns:1fr}.record-title{display:grid}}
@media(max-width:420px){.audit-filters{grid-template-columns:1fr}.audit-actions{grid-column:1}.audit-actions button{width:100%}.audit-brand code{font-size:var(--body-font-size)}}
</style>
