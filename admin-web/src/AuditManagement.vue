<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { auditPermissions, createAuditApi, type AdminAuditRecord, type AuditFilters } from "./audit-api";
import { useAdminI18n } from "./i18n";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, locale } = useAdminI18n();
const tr = (zh: string, en: string) => t(zh, en);
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
const error = ref("");
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

function localToIso(value: string): string | null {
  if (!value) return null;
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2})(?:\.(\d{1,3}))?)?$/.exec(value);
  if (!match) return null;
  const [, yearText, monthText, dayText, hourText, minuteText, secondText = "00", millisecondText = ""] = match;
  const year = Number(yearText), month = Number(monthText), day = Number(dayText);
  const hour = Number(hourText), minute = Number(minuteText), second = Number(secondText);
  const days = month === 2 ? (year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28) : ([4, 6, 9, 11].includes(month) ? 30 : 31);
  if (year < 1 || year > 9999 || month < 1 || month > 12 || day < 1 || day > days || hour > 23 || minute > 59 || second > 59) return null;
  const date = new Date(0);
  date.setUTCFullYear(year, month - 1, day);
  date.setUTCHours(hour, minute, second, Number(millisecondText.padEnd(3, "0") || "0"));
  return Number.isFinite(date.getTime()) ? date.toISOString() : null;
}
function currentRead(ticket: number, scope: string): boolean { return alive && ticket === readGeneration && scope === scopeKey.value; }
function currentExport(ticket: number, scope: string): boolean { return alive && ticket === exportGeneration && scope === scopeKey.value; }
function reportError(cause: unknown): string {
  if (cause instanceof AdminApiError) return cause.message;
  return cause instanceof Error ? cause.message : tr("读取审计日志失败。", "Could not load audit records.");
}
function submitFilters(): AuditFilters | null {
  const from = localToIso(fromDraft.value), to = localToIso(toDraft.value);
  if (!from || !to || new Date(to).getTime() <= new Date(from).getTime() || new Date(to).getTime() - new Date(from).getTime() > 31 * 24 * 60 * 60 * 1000) {
    error.value = tr("请选择有效且不超过 31 天的开始与结束时间。", "Choose a valid start and end within 31 days.");
    return null;
  }
  const next: AuditFilters = { from, to };
  for (const [key, raw] of [["action", actionDraft.value], ["actor_id", actorDraft.value], ["resource_type", resourceTypeDraft.value], ["resource_id", resourceIdDraft.value], ["request_id", requestIdDraft.value]] as const) {
    if (!raw) continue;
    if (raw.trim() !== raw || /\p{Cc}/u.test(raw) || (["action", "resource_type", "request_id"].includes(key) && new TextEncoder().encode(raw).length > 128)) {
      error.value = tr("筛选值格式无效；请移除空白或控制字符。", "A filter is invalid. Remove surrounding whitespace or control characters.");
      return null;
    }
    if ((key === "actor_id" || key === "resource_id") && !/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(raw)) {
      error.value = tr(key === "actor_id" ? "操作人 ID 必须是 UUID。" : "资源 ID 必须是 UUID。", key === "actor_id" ? "Actor ID must be a UUID." : "Resource ID must be a UUID.");
      return null;
    }
    next[key] = raw;
  }
  return next;
}
async function readPage(nextOffset: number, newFilters = false) {
  if (!permissions.value.view) { error.value = tr("需要 audit.view.brand 或 audit.view.platform 查看权限。", "Requires audit.view.brand or audit.view.platform permission."); return; }
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
  if (!permissions.value.export) { error.value = tr("导出需要分别授予 audit.view.brand/platform 与 audit.export.brand/platform，并作用于当前品牌。", "Export requires separate audit.view.brand/platform and audit.export.brand/platform grants for this selected brand."); return; }
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
function redacted(value: unknown): string {
  let parsed = value;
  if (typeof value === "string") { try { parsed = JSON.parse(value); } catch { return "[unstructured snapshot hidden]"; } }
  const sensitive = /password|secret|token|authorization|cookie|credential|private|api[_-]?key|salt|email|phone|mobile|address|birth|identity|ssn/i;
  const redact = (item: unknown): unknown => {
    if (Array.isArray(item)) return item.map(redact);
    if (item && typeof item === "object") return Object.fromEntries(Object.entries(item).map(([key, child]) => [key, sensitive.test(key) ? "[REDACTED]" : redact(child)]));
    return item;
  };
  return JSON.stringify(redact(parsed), null, 2) ?? "null";
}
function timestamp(value: string): string { return new Date(value).toLocaleString(locale.value === "en" ? "en" : "zh-CN", { timeZone: "UTC", timeZoneName: "short" }); }
</script>

<template>
  <section class="audit-page" aria-labelledby="audit-title">
    <header class="audit-heading">
      <div><div class="audit-eyebrow">SECURITY / AUDIT TRAIL</div><h1 id="audit-title">{{ tr("审计日志", "Audit log") }}</h1>
        <p>{{ tr("按明确品牌查询审计记录；导出仅包含已提交筛选条件的完整时间范围。", "Query audit records for the explicit brand. Exports cover the full selected interval.") }}</p>
      </div>
      <span class="audit-brand">{{ tr("品牌", "Brand") }} · <code>{{ brandId }}</code></span>
    </header>
    <p v-if="!permissions.view" class="audit-notice" role="status">{{ tr("当前账号没有该品牌的审计查看权限。", "This account cannot view audit records for this brand.") }}</p>
    <template v-else>
      <form class="audit-filters" @submit.prevent="readPage(0, true)">
        <label><span>{{ tr("开始时间（UTC）", "From (UTC)") }}</span><input v-model="fromDraft" type="datetime-local" step="1" required /></label>
        <label><span>{{ tr("结束时间（UTC）", "To (UTC)") }}</span><input v-model="toDraft" type="datetime-local" step="1" required /></label>
        <label><span>{{ tr("操作", "Action") }}</span><input v-model="actionDraft" autocomplete="off" /></label>
        <label><span>{{ tr("操作人 UUID", "Actor UUID") }}</span><input v-model="actorDraft" autocomplete="off" /></label>
        <label><span>{{ tr("资源类型", "Resource type") }}</span><input v-model="resourceTypeDraft" autocomplete="off" /></label>
        <label><span>{{ tr("资源 UUID", "Resource UUID") }}</span><input v-model="resourceIdDraft" autocomplete="off" /></label>
        <label><span>Request ID</span><input v-model="requestIdDraft" autocomplete="off" /></label>
        <div class="audit-actions">
          <button type="submit" :disabled="loading">{{ loading ? tr("查询中…", "Loading…") : tr("查询审计记录", "Query audit records") }}</button>
          <button type="button" class="secondary" :disabled="!filters || exporting || !permissions.export" @click="exportCsv">{{ exporting ? tr("验证并导出…", "Validating export…") : tr("导出完整 CSV", "Export full CSV") }}</button>
        </div>
      </form>
      <p class="audit-help">{{ tr("时间以 UTC 输入并按半开区间 [from, to) 查询。每页最多 100 条；使用分页读取后续记录。", "Enter UTC times; the interval is [from, to). Each page has up to 100 records; use pagination to continue.") }}</p>
      <p v-if="!permissions.export" class="audit-notice" role="status">{{ tr("导出未授权：查看与导出权限须分别授予（audit.view.brand/platform 与 audit.export.brand/platform），并适用于当前品牌。超级管理员身份不会替代这些权限。", "Export is not authorized. View and export are checked separately (audit.view.brand/platform and audit.export.brand/platform) for this selected brand. Super-admin status does not replace these grants.") }}</p>
      <p v-if="error" class="audit-error" role="alert">{{ error }}</p>
      <p v-if="loading" class="audit-state" role="status">{{ tr("正在读取审计记录…", "Loading audit records…") }}</p>
      <div v-else-if="filters" class="audit-results">
        <div v-if="!records.length" class="audit-state">{{ tr("此页没有审计记录。", "No audit records on this page.") }}</div>
        <article v-for="entry in records" :key="entry.id" class="audit-record">
          <div class="record-title"><strong>{{ entry.action }}</strong><time :datetime="entry.created_at">{{ timestamp(entry.created_at) }}</time></div>
          <dl>
            <div><dt>{{ tr("操作人", "Actor") }}</dt><dd>{{ entry.actor_type }} · {{ entry.actor_id }}</dd></div>
            <div><dt>{{ tr("资源", "Resource") }}</dt><dd>{{ entry.resource_type }}<template v-if="entry.resource_id"> · {{ entry.resource_id }}</template></dd></div>
            <div><dt>{{ tr("原因", "Reason") }}</dt><dd>{{ entry.reason || tr("无", "None") }}</dd></div>
            <div><dt>Request ID</dt><dd>{{ entry.request_id }}</dd></div>
            <div><dt>IP</dt><dd>{{ entry.ip_address || tr("无", "None") }}</dd></div>
          </dl>
          <details><summary>{{ tr("查看脱敏前后快照", "View redacted before and after snapshots") }}</summary>
            <div class="audit-snapshots"><section><h3>{{ tr("变更前", "Before") }}</h3><pre>{{ redacted(entry.before_json) }}</pre></section><section><h3>{{ tr("变更后", "After") }}</h3><pre>{{ redacted(entry.after_json) }}</pre></section></div>
          </details>
        </article>
        <nav v-if="records.length || offset" class="audit-pagination" :aria-label="tr('审计记录分页', 'Audit record pagination')">
          <button type="button" :disabled="loading || offset === 0" @click="readPage(Math.max(0, offset - PAGE_SIZE))">{{ tr("上一页", "Previous") }}</button>
          <span>{{ tr("偏移", "Offset") }} {{ offset }} · {{ tr("本页", "This page") }} {{ records.length }}</span>
          <button type="button" :disabled="loading || records.length < PAGE_SIZE || offset + PAGE_SIZE > 100_000" @click="readPage(offset + PAGE_SIZE)">{{ tr("下一页", "Next") }}</button>
        </nav>
      </div>
      <aside v-if="receipt" class="audit-receipt" role="status" aria-live="polite">
        <strong>{{ tr("导出已验证", "Export verified") }}</strong>
        <span>{{ tr("行数", "Rows") }}: {{ receipt.rowCount }} · {{ tr("快照", "Snapshot") }}: {{ receipt.snapshotAt }}</span>
        <code>{{ receipt.sha256 }}</code><small>{{ tr("导出 ID", "Export ID") }}: {{ receipt.exportId }}</small>
      </aside>
    </template>
  </section>
</template>

<style scoped>
.audit-page{display:grid;gap:16px;min-width:0}.audit-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:16px}.audit-heading h1{margin:4px 0;font-size:24px}.audit-heading p,.audit-help{margin:0;color:var(--text-muted,#687386);line-height:1.5}.audit-eyebrow{font-size:11px;letter-spacing:.12em;color:var(--text-muted,#687386)}.audit-brand{max-width:100%;overflow-wrap:anywhere;color:var(--text-muted,#687386)}.audit-brand code{color:inherit}.audit-filters{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px;padding:16px;border:1px solid var(--border,#dce1e8);border-radius:12px;background:var(--surface,#fff)}.audit-filters label{display:grid;gap:6px;min-width:0;font-size:12px;font-weight:600}.audit-filters input{box-sizing:border-box;width:100%;min-height:40px;padding:8px;border:1px solid var(--border,#cbd2dc);border-radius:7px;background:var(--surface,#fff);color:inherit;font:inherit}.audit-actions{display:flex;align-items:end;gap:8px;grid-column:span 4}.audit-actions button,.audit-pagination button{min-height:40px;padding:8px 13px;border:1px solid var(--accent,#315bd6);border-radius:7px;background:var(--accent,#315bd6);color:#fff;font:inherit;font-weight:600;cursor:pointer}.audit-actions button.secondary,.audit-pagination button{background:var(--surface,#fff);color:var(--accent,#315bd6)}button:disabled{opacity:.55;cursor:not-allowed}.audit-notice,.audit-state,.audit-error{padding:12px 14px;border-radius:8px;background:var(--surface-muted,#f2f4f7);line-height:1.5}.audit-error{color:var(--danger,#a12622);background:#fff0ef}.audit-record{min-width:0;padding:16px;border:1px solid var(--border,#dce1e8);border-radius:10px;background:var(--surface,#fff)}.record-title{display:flex;justify-content:space-between;gap:12px;align-items:start}.record-title time{color:var(--text-muted,#687386);font-size:12px}.audit-record dl{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px 18px;margin:14px 0}.audit-record dl div{min-width:0}.audit-record dt{font-size:11px;color:var(--text-muted,#687386)}.audit-record dd{margin:3px 0 0;overflow-wrap:anywhere}.audit-record details{border-top:1px solid var(--border,#e1e5eb);padding-top:10px}.audit-record summary{cursor:pointer;font-weight:600}.audit-snapshots{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.audit-snapshots h3{font-size:13px}.audit-snapshots pre{max-height:320px;overflow:auto;white-space:pre-wrap;overflow-wrap:anywhere;padding:10px;border-radius:7px;background:var(--surface-muted,#f2f4f7)}.audit-pagination{display:flex;align-items:center;justify-content:center;gap:14px;flex-wrap:wrap}.audit-pagination span{color:var(--text-muted,#687386)}.audit-receipt{display:grid;gap:6px;padding:14px;border:1px solid var(--success,#32805b);border-radius:9px;background:var(--surface,#fff);overflow-wrap:anywhere}.audit-receipt code{word-break:break-all}
@media(max-width:760px){.audit-heading{display:grid}.audit-filters{grid-template-columns:repeat(2,minmax(0,1fr));padding:12px}.audit-actions{grid-column:span 2;flex-wrap:wrap}.audit-actions button{flex:1 1 160px}.audit-record dl{grid-template-columns:1fr}.audit-snapshots{grid-template-columns:1fr}.record-title{display:grid}}
@media(max-width:420px){.audit-filters{grid-template-columns:1fr}.audit-actions{grid-column:1}.audit-actions button{width:100%}.audit-brand code{font-size:11px}}
</style>
