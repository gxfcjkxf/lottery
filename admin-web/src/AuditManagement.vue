<script setup lang="ts">
import { businessActionLabel, businessResourceLabel } from '@lottery/shared'
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import FilterAssociationSelect from "@lottery/shared/FilterAssociationSelect.vue";
import { filterDateBoundary } from "@lottery/shared";
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import PlatformTemporalInput from "@lottery/shared/PlatformTemporalInput.vue";
import { vGlobalLoading } from "@lottery/shared";
const displayDate = (value: string | null | undefined) => formatDisplayDateTime(value, displayLocale.value);
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import { vInlineValidation } from '@lottery/shared'
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { auditPermissions, createAuditApi, type AdminAuditRecord, type AuditFilters } from "./audit-api";
import { adminKey, useAdminI18n } from "./i18n";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, locale } = useAdminI18n();
const tr = (key: string) => t(adminKey(key));
const api = createAuditApi();
const PAGE_SIZE = 100;
const permissions = computed(() => auditPermissions(props.account, props.brandId));
const fromDraft = ref("");
const toDraft = ref("");
const actionDraft = ref("");
const actorDraft = ref("");
const resourceTypeDraft = ref("");
const resourceIdDraft = ref("");
const requestIdDraft = ref("");
const records = ref<AdminAuditRecord[]>([]);
const offset = ref(0);
const filters = ref<AuditFilters | null>(null);
const loading = ref(false);
const exporting = ref(false);
const error = ref<string | import("@lottery/shared").LocalizedMessage>("");
const receipt = ref<{ exportId: string; snapshotAt: string; rowCount: number; sha256: string } | null>(null);
let readGeneration = 0;
let exportGeneration = 0;
let alive = true;

const scopeKey = computed(() => JSON.stringify([
  props.account.id, props.account.version, props.account.super_admin, props.account.brand_ids,
  props.account.permissions, props.account.permissions_by_brand, props.account.platform_permissions,
  props.brandId, permissions.value, locale.value,
]));
watch(scopeKey, () => {
  readGeneration++;
  exportGeneration++;
  records.value = [];
  filters.value = null;
  offset.value = 0;
  loading.value = exporting.value = false;
  error.value = "";
  receipt.value = null;
});
onBeforeUnmount(() => { alive = false; readGeneration++; exportGeneration++; });
function currentRead(ticket: number, scope: string): boolean { return alive && ticket === readGeneration && scope === scopeKey.value; }
function currentExport(ticket: number, scope: string): boolean { return alive && ticket === exportGeneration && scope === scopeKey.value; }
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
  for (const [key, raw] of [["action", actionDraft.value], ["actor_id", actorDraft.value], ["resource_type", resourceTypeDraft.value], ["resource_id", resourceIdDraft.value], ["request_id", requestIdDraft.value]] as const) {
    if (!raw) continue;
    if (raw.trim() !== raw || /\p{Cc}/u.test(raw) || (["action", "resource_type", "request_id"].includes(key) && new TextEncoder().encode(raw).length > 128)) {
      error.value = tr('admin.copy.c25c18ad');
      return null;
    }
    if ((key === "actor_id" || key === "resource_id") && !/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(raw)) {
      error.value = tr(key === "actor_id" ? "admin.copy.ebd149a7" : "admin.copy.34a73fc7");
      return null;
    }
    next[key] = raw;
  }
  return next;
}
async function readPage(nextOffset: number, newFilters = false) {
  if (!permissions.value.view) { error.value = tr('admin.copy.6bf6ac7d'); return; }
  if (nextOffset < 0 || nextOffset > 100_000) return;
  const chosen = newFilters ? submitFilters() : filters.value;
  if (!chosen) return;
  const ticket = ++readGeneration, scope = scopeKey.value;
  exportGeneration++;
  if (newFilters) { filters.value = chosen; offset.value = 0; }
  else offset.value = nextOffset;
  loading.value = true;
  error.value = "";
  receipt.value = null;
  records.value = [];
  try {
    const result = await api.list(props.brandId, { ...chosen, limit: PAGE_SIZE, offset: newFilters ? 0 : nextOffset });
    if (!currentRead(ticket, scope)) return;
    records.value = result.items;
  } catch (cause) {
    if (!currentRead(ticket, scope)) return;
    error.value = reportError(cause);
    if (cause instanceof AdminApiError && cause.status === 401) emit("session-invalid");
  } finally { if (currentRead(ticket, scope)) loading.value = false; }
}
async function exportCsv() {
  if (!permissions.value.export) { error.value = tr('admin.copy.af86866a'); return; }
  const chosen = filters.value ?? submitFilters();
  if (!chosen) return;
  filters.value = chosen;
  const ticket = ++exportGeneration, scope = scopeKey.value;
  exporting.value = true;
  error.value = "";
  receipt.value = null;
  try {
    const result = await api.exportCsv(props.brandId, chosen);
    if (!currentExport(ticket, scope)) return;
    receipt.value = { exportId: result.exportId, snapshotAt: result.snapshotAt, rowCount: result.rowCount, sha256: result.sha256 };
    const url = URL.createObjectURL(new Blob([result.bytes], { type: "text/csv;charset=utf-8" }));
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = result.filename;
    anchor.click();
    setTimeout(() => URL.revokeObjectURL(url), 0);
  } catch (cause) {
    if (!currentExport(ticket, scope)) return;
    receipt.value = null;
    error.value = reportError(cause);
    if (cause instanceof AdminApiError && cause.status === 401) emit("session-invalid");
  } finally { if (currentExport(ticket, scope)) exporting.value = false; }
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
</script>

<template>
  <section class="audit-page" aria-labelledby="audit-title">
    <header class="audit-heading">
      <div><div class="audit-eyebrow">{{ tr('admin.copy.f1d91506') }}</div><h1 id="audit-title">{{ tr('admin.copy.315a9366') }}</h1>
      </div>
      <span class="audit-brand">{{ tr('admin.copy.9c32661d') }} · <code><AssociationName kind="brand" :id="brandId" :locale="associationLocale" /></code></span>
    </header>
    <p v-if="!permissions.view" class="audit-notice" role="status">{{ tr('admin.copy.792805a5') }}</p>
    <template v-else>
      <form novalidate v-inline-validation class="audit-filters" @submit.prevent="withGlobalLoading(readPage, 0, true)">
        <label><span>{{ t('filter.fromDateUTC') }}</span><PlatformTemporalInput v-model="fromDraft" type="date" required :locale="displayLocale" /></label>
        <label><span>{{ t('filter.toDateUTC') }}</span><PlatformTemporalInput v-model="toDraft" type="date" required :locale="displayLocale" /></label>

        <label><span>{{ t('filter.actor') }}</span><FilterAssociationSelect v-model="actorDraft" :brand-id="props.brandId" surface="admin" :enabled="permissions.view" resource="audit" kind="actor" :label="t('filter.actor')" :locale="displayLocale" :scope-key="JSON.stringify(props.account)" /></label>

        <label><span>{{ t('filter.audit_resource') }}</span><FilterAssociationSelect v-model="resourceIdDraft" :brand-id="props.brandId" surface="admin" :enabled="permissions.view" resource="audit" kind="audit_resource" :label="t('filter.audit_resource')" :locale="displayLocale" :scope-key="JSON.stringify(props.account)" /></label>

        <div class="audit-actions">
          <button type="submit" :disabled="loading">{{ loading ? tr('admin.copy.dea7c355') : tr('admin.copy.2d2b9232') }}</button>
          <button type="button" class="button button-secondary" :disabled="!filters || exporting || !permissions.export" @click="withGlobalLoading(exportCsv)">{{ exporting ? tr('admin.copy.da3845f1') : tr('admin.copy.800c75fc') }}</button>
        </div>
      </form>
      <p class="audit-help">{{ tr('admin.copy.9d061090') }}</p>
      <p v-if="!permissions.export" class="audit-notice" role="status">{{ tr('admin.copy.418a8d77') }}</p>
      <p v-if="error" class="audit-error" role="alert">{{ displayPublicError(error) }}</p>
      <p v-global-loading v-if="loading" class="audit-state" role="status">{{ tr('admin.copy.9b68f2d5') }}</p>
      <div v-else-if="filters" class="audit-results">
        <div v-if="!records.length" class="audit-state empty-state">{{ tr('admin.copy.07179c69') }}</div>
        <article v-for="entry in records" :key="entry.id" class="audit-record">
          <div class="record-title"><strong>{{ businessActionLabel(entry.action, associationLocale) }}</strong><time :datetime="entry.created_at">{{ timestamp(entry.created_at) }}</time></div>
          <dl>
            <div><dt>{{ tr('admin.copy.cef73d0f') }}</dt><dd>{{ actorLabel(entry) }}</dd></div>
            <div><dt>{{ tr('admin.copy.0cf060d7') }}</dt><dd>{{ businessResourceLabel(entry.resource_type, associationLocale) }}</dd></div>
            <div><dt>{{ tr('admin.copy.5ec7c8ba') }}</dt><dd>{{ entry.reason || tr('admin.copy.c2b41b33') }}</dd></div>
          </dl>
          <details><summary>{{ tr('admin.copy.2d5163d8') }}</summary>
            <div class="audit-snapshots"><section><h3>{{ tr('admin.copy.f85d9f7f') }}</h3><BusinessDetails :value="redacted(entry.before_json)" :locale="associationLocale" /></section><section><h3>{{ tr('admin.copy.6db1d337') }}</h3><BusinessDetails :value="redacted(entry.after_json)" :locale="associationLocale" /></section></div>
          </details>
        </article>
        <nav v-if="records.length || offset" class="audit-pagination" :aria-label="tr('admin.copy.e2f05ca9')">
          <button type="button" :disabled="loading || offset === 0" @click="withGlobalLoading(readPage, Math.max(0, offset - PAGE_SIZE))">{{ tr('admin.copy.dbe7a5d7') }}</button>
          <span>{{ tr('admin.copy.4ffb01e2') }} {{ offset }} · {{ tr('admin.copy.0bffd049') }} {{ records.length }}</span>
          <button type="button" :disabled="loading || records.length < PAGE_SIZE || offset + PAGE_SIZE > 100_000" @click="withGlobalLoading(readPage, offset + PAGE_SIZE)">{{ tr('admin.copy.2e9c9e7a') }}</button>
        </nav>
      </div>
      <aside v-if="receipt" class="audit-receipt" role="status" aria-live="polite">
        <strong>{{ tr('admin.copy.2014d47e') }}</strong>
        <span>{{ tr('admin.copy.a39460be') }}: {{ receipt.rowCount }} · {{ tr('admin.copy.82d4e9bd') }}: {{ displayDate(receipt.snapshotAt) }}</span>
        <code>{{ receipt.sha256 }}</code><small>{{ tr('admin.copy.71a032cb') }}: {{ receipt.exportId }}</small>
      </aside>
    </template>
  </section>
</template>

<style scoped>
.audit-page{display:grid;gap:16px;min-width:0}.audit-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:16px}.audit-heading h1{margin:4px 0;font-size:24px}.audit-heading p,.audit-help{margin:0;color:var(--text-muted,#687386);line-height:1.5}.audit-eyebrow{font-size:11px;letter-spacing:.12em;color:var(--text-muted,#687386)}.audit-brand{max-width:100%;overflow-wrap:anywhere;color:var(--text-muted,#687386)}.audit-brand code{color:inherit}.audit-filters{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px;padding:16px;border:1px solid var(--border,#dce1e8);border-radius:12px;background:var(--surface)}.audit-filters label{display:grid;gap:6px;min-width:0;font-size:12px;font-weight:600}.audit-filters input{box-sizing:border-box;width:100%;min-height:40px;padding:8px;border:1px solid var(--border,#cbd2dc);border-radius:7px;background:var(--surface);color:inherit;font:inherit}.audit-actions{display:flex;align-items:end;gap:8px;grid-column:span 4}.audit-actions button,.audit-pagination button{min-height:40px;padding:8px 13px;border:1px solid var(--accent);border-radius:7px;background:var(--accent);color:var(--surface);font:inherit;font-weight:600;cursor:pointer}.audit-actions button.secondary,.audit-pagination button{background:var(--surface);color:var(--accent)}button:disabled{opacity:.55;cursor:not-allowed}.audit-notice,.audit-state,.audit-error{padding:12px 14px;border-radius:8px;background:var(--surface-muted,#f2f4f7);line-height:1.5}.audit-error{color:var(--danger);background:#fff0ef}.audit-record{min-width:0;padding:16px;border:1px solid var(--border,#dce1e8);border-radius:10px;background:var(--surface)}.record-title{display:flex;justify-content:space-between;gap:12px;align-items:start}.record-title time{color:var(--text-muted,#687386);font-size:12px}.audit-record dl{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px 18px;margin:14px 0}.audit-record dl div{min-width:0}.audit-record dt{font-size:11px;color:var(--text-muted,#687386)}.audit-record dd{margin:3px 0 0;overflow-wrap:anywhere}.audit-record details{border-top:1px solid var(--border,#e1e5eb);padding-top:10px}.audit-record summary{cursor:pointer;font-weight:600}.audit-snapshots{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.audit-snapshots h3{font-size:13px}.audit-snapshots pre{max-height:320px;overflow:auto;white-space:pre-wrap;overflow-wrap:anywhere;padding:10px;border-radius:7px;background:var(--surface-muted,#f2f4f7)}.audit-pagination{display:flex;align-items:center;justify-content:center;gap:14px;flex-wrap:wrap}.audit-pagination span{color:var(--text-muted,#687386)}.audit-receipt{display:grid;gap:6px;padding:14px;border:1px solid var(--success);border-radius:9px;background:var(--surface);overflow-wrap:anywhere}.audit-receipt code{word-break:break-all}
@media(max-width:760px){.audit-heading{display:grid}.audit-filters{grid-template-columns:repeat(2,minmax(0,1fr));padding:12px}.audit-actions{grid-column:span 2;flex-wrap:wrap}.audit-actions button{flex:1 1 160px}.audit-record dl{grid-template-columns:1fr}.audit-snapshots{grid-template-columns:1fr}.record-title{display:grid}}
@media(max-width:420px){.audit-filters{grid-template-columns:1fr}.audit-actions{grid-column:1}.audit-actions button{width:100%}.audit-brand code{font-size:11px}}
</style>
