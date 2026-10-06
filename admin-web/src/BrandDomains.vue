<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { brandDomainsPermissions, createBrandDomainsApi, isCanonicalBrandDomain, type BrandDomain, type BrandDomainsHistoryItem, type BrandDomainsRecord } from "./brand-domains-api";
import { brandDomainsSessionGeneration, classifyBrandDomainsFailure, clearAllPendingBrandDomainsWrites, clearBrandDomainsDraft, clearPendingBrandDomainsWrite, createBrandDomainsRequestGuard, getBrandDomainsDraft, getPendingBrandDomainsWrite, retainPendingBrandDomainsWrite, setBrandDomainsDraft, setPendingBrandDomainsWrite, type PendingBrandDomainsWrite } from "./brand-domains-state";

const props = defineProps<{ account: AdminAccount; brandId: string; brandStatus?: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createBrandDomainsApi();
const readGuard = createBrandDomainsRequestGuard(), historyGuard = createBrandDomainsRequestGuard(), writeGuard = createBrandDomainsRequestGuard();
const permissions = computed(() => brandDomainsPermissions(props.account, props.brandId));
const scope = () => ({ accountId: props.account.id, brandId: props.brandId });
const record = ref<BrandDomainsRecord | null>(null), rows = ref<BrandDomainsHistoryItem[]>([]);
const pending = ref<PendingBrandDomainsWrite | null>(null);
const operation = ref<"add" | "edit">("add"), domainId = ref<string | undefined>();
const formOpen = ref(false);
const domain = ref(""), enabled = ref(false), primary = ref(false), reason = ref("");
const offset = ref(0), hasMore = ref(false), loading = ref(false), historyLoading = ref(false), busy = ref(false);
const error = ref(""), notice = ref(""), conflictReloaded = ref(false);
const readOnly = computed(() => props.brandStatus === "disabled" || record.value?.status === "disabled");
const uncertain = computed(() => pending.value?.phase === "unknown"), conflict = computed(() => pending.value?.phase === "conflict");
const canEdit = computed(() => permissions.value.write && !readOnly.value && !pending.value && !busy.value);
const primaryWillDemote = computed(() => primary.value && Boolean(record.value?.domains.some((item) => item.is_primary && (operation.value === "add" || item.id !== domainId.value))));
const domainBytes = computed(() => new TextEncoder().encode(domain.value).length), reasonBytes = computed(() => new TextEncoder().encode(reason.value).length);
const isNewDomainValid = computed(() => operation.value === "edit" || isCanonicalBrandDomain(domain.value));
const statusLabel = (value: string) => value === "active" ? "Active" : value === "paused" ? "Paused" : "Disabled";
const timeLabel = (value: string) => { const date = new Date(value); return Number.isNaN(date.getTime()) ? value : date.toLocaleString(); };

function restoreDraft(): void {
  const stored = getBrandDomainsDraft(scope()), intent = getPendingBrandDomainsWrite(scope());
  pending.value = intent; conflictReloaded.value = false;
  if (intent) {
    formOpen.value = true;
    operation.value = intent.operation; domainId.value = intent.domainId;
    domain.value = intent.operation === "add" ? String((intent.body as { domain: string }).domain) : record.value?.domains.find((d) => d.id === intent.domainId)?.domain ?? "";
    enabled.value = intent.body.enabled; primary.value = intent.body.is_primary; reason.value = intent.body.reason;
  } else if (stored) {
    formOpen.value = true;
    operation.value = stored.operation; domainId.value = stored.domainId; domain.value = stored.domain; enabled.value = stored.enabled; primary.value = stored.is_primary; reason.value = stored.reason;
  } else resetForm(false);
}
function persistDraft(): void {
  if (pending.value || getPendingBrandDomainsWrite(scope()) || !permissions.value.write || readOnly.value) return;
  setBrandDomainsDraft({ ...scope(), operation: operation.value, ...(domainId.value ? { domainId: domainId.value } : {}), domain: domain.value, enabled: enabled.value, is_primary: primary.value, reason: reason.value });
}
function resetForm(clear = true): void {
  formOpen.value = false;
  operation.value = "add"; domainId.value = undefined; domain.value = ""; enabled.value = false; primary.value = false; reason.value = "";
  if (clear) clearBrandDomainsDraft(scope());
}
function errorText(cause: unknown, fallback: string): string { return cause instanceof Error ? cause.message : fallback; }
function isCurrent(guard: ReturnType<typeof createBrandDomainsRequestGuard>, token: number, accountId: string, brandId: string, epoch: number): boolean {
  return guard.isCurrent(token) && epoch === brandDomainsSessionGeneration() && props.account.id === accountId && props.brandId === brandId;
}
function sessionError(cause: unknown, guard: ReturnType<typeof createBrandDomainsRequestGuard>, token: number, accountId: string, brandId: string, epoch: number): void {
  if (cause instanceof AdminApiError && cause.status === 401 && isCurrent(guard, token, accountId, brandId, epoch)) { clearAllPendingBrandDomainsWrites(); emit("session-invalid"); }
}
async function loadRecord(): Promise<boolean> {
  const { accountId, brandId } = scope(), epoch = brandDomainsSessionGeneration(), token = readGuard.begin(); loading.value = true; error.value = "";
  try {
    const next = await api.get(brandId);
    if (!isCurrent(readGuard, token, accountId, brandId, epoch)) return false;
    record.value = next;
    if (pending.value?.operation === "edit") domain.value = next.domains.find((item) => item.id === pending.value?.domainId)?.domain ?? "";
    if (conflict.value) conflictReloaded.value = true; return true;
  } catch (cause) {
    if (isCurrent(readGuard, token, accountId, brandId, epoch)) { error.value = errorText(cause, "Could not load brand domains."); sessionError(cause, readGuard, token, accountId, brandId, epoch); }
    return false;
  } finally { if (isCurrent(readGuard, token, accountId, brandId, epoch)) loading.value = false; }
}
async function loadHistory(nextOffset = offset.value): Promise<void> {
  const { accountId, brandId } = scope(), epoch = brandDomainsSessionGeneration(), token = historyGuard.begin(); historyLoading.value = true;
  try {
    const page = await api.history(brandId, 20, nextOffset);
    if (!isCurrent(historyGuard, token, accountId, brandId, epoch)) return;
    rows.value = page.items; offset.value = page.offset; hasMore.value = page.items.length === page.limit;
  } catch (cause) { if (isCurrent(historyGuard, token, accountId, brandId, epoch)) { error.value = errorText(cause, "Could not load domain history."); sessionError(cause, historyGuard, token, accountId, brandId, epoch); } }
  finally { if (isCurrent(historyGuard, token, accountId, brandId, epoch)) historyLoading.value = false; }
}
function editDomain(item: BrandDomain): void {
  if (!canEdit.value) return;
  formOpen.value = true; operation.value = "edit"; domainId.value = item.id; domain.value = item.domain; enabled.value = item.enabled; primary.value = item.is_primary; reason.value = ""; persistDraft();
}
function startAdd(): void { if (!canEdit.value || (record.value?.domains.length ?? 100) >= 100) return; resetForm(false); formOpen.value = true; persistDraft(); }
function reviewChange(): void {
  error.value = ""; notice.value = "";
  const existing = getPendingBrandDomainsWrite(scope());
  if (existing) { pending.value = existing; return; }
  if (!record.value || !canEdit.value || reason.value.trim().length === 0 || reasonBytes.value > 500 || (operation.value === "add" && (!isCanonicalBrandDomain(domain.value) || domainBytes.value > 253))) {
    error.value = "Enter a valid domain and a reason of 1–500 UTF-8 bytes."; return;
  }
  if (operation.value === "edit" && !record.value.domains.some((item) => item.id === domainId.value)) { error.value = "That domain is no longer in this brand. Reload the list."; return; }
  const body = operation.value === "add"
    ? { version: record.value.version, domain: domain.value, enabled: enabled.value, is_primary: primary.value, reason: reason.value }
    : { version: record.value.version, enabled: enabled.value, is_primary: primary.value, reason: reason.value };
  const intent: PendingBrandDomainsWrite = { ...scope(), operation: operation.value, ...(domainId.value ? { domainId: domainId.value } : {}), body, key: createIdempotencyKey(), phase: "review" };
  setPendingBrandDomainsWrite(scope(), intent); pending.value = getPendingBrandDomainsWrite(scope()); conflictReloaded.value = false;
}
function abandon(): void {
  const intent = pending.value;
  if (busy.value || !intent || intent.phase === "unknown" || (intent.phase === "conflict" && !conflictReloaded.value)) return;
  if (getPendingBrandDomainsWrite(scope())?.key !== intent.key) { pending.value = getPendingBrandDomainsWrite(scope()); return; }
  clearPendingBrandDomainsWrite(scope(), intent.key); pending.value = getPendingBrandDomainsWrite(scope()); resetForm();
  notice.value = "The frozen request was abandoned. Review the current list before starting a new change.";
}
async function send(retry = false): Promise<void> {
  const intent = pending.value;
  if (!intent || busy.value || !permissions.value.write || readOnly.value || (retry ? intent.phase !== "unknown" : intent.phase !== "review")) return;
  const epoch = brandDomainsSessionGeneration(), token = writeGuard.begin();
  const ownScope = { accountId: intent.accountId, brandId: intent.brandId };
  if (!retainPendingBrandDomainsWrite(ownScope, intent, "unknown")) { pending.value = getPendingBrandDomainsWrite(scope()); return; }
  pending.value = getPendingBrandDomainsWrite(scope());
  if (pending.value?.key !== intent.key) return;
  busy.value = true; error.value = ""; notice.value = "";
  try {
    if (intent.operation === "add") await api.post(intent.brandId, intent.body as import("./brand-domains-api").BrandDomainPostBody, intent.key);
    else await api.patch(intent.brandId, intent.domainId!, intent.body as import("./brand-domains-api").BrandDomainPatchBody, intent.key);
    if (epoch !== brandDomainsSessionGeneration()) return;
    if (getPendingBrandDomainsWrite(ownScope)?.key === intent.key) {
      clearPendingBrandDomainsWrite(ownScope, intent.key);
      clearBrandDomainsDraft(ownScope);
    }
    if (!isCurrent(writeGuard, token, intent.accountId, intent.brandId, epoch)) return;
    pending.value = null; notice.value = "The receipt was verified. Reloading the current list and history.";
    const refreshed = await loadRecord(); await loadHistory(0);
    if (isCurrent(writeGuard, token, intent.accountId, intent.brandId, epoch)) notice.value = refreshed ? "The receipt was verified; the latest domain list was independently read." : "The receipt was verified, but reloading the latest list failed. Do not resubmit; reload before continuing.";
  } catch (cause) {
    if (epoch !== brandDomainsSessionGeneration()) return;
    const cached = getPendingBrandDomainsWrite(ownScope);
    if (cached && cached.key !== intent.key) return;
    const kind = classifyBrandDomainsFailure(cause instanceof AdminApiError ? cause.status : undefined);
    if (kind !== "definitive") {
      if (!retainPendingBrandDomainsWrite(ownScope, intent, kind)) return;
    } else {
      if (cached?.key !== intent.key) return;
      clearPendingBrandDomainsWrite(ownScope, intent.key);
    }
    if (!isCurrent(writeGuard, token, intent.accountId, intent.brandId, epoch)) return;
    pending.value = getPendingBrandDomainsWrite(scope());
    if (kind === "unknown") error.value = "The outcome is unknown. Retry only the exact frozen request with its existing key.";
    else if (kind === "conflict") { error.value = "The shared brand version changed. Read the latest list, then explicitly abandon this request before creating a new one."; conflictReloaded.value = false; await loadRecord(); }
    else { error.value = errorText(cause, "The domain change was rejected."); if (pending.value === null) persistDraft(); }
    sessionError(cause, writeGuard, token, intent.accountId, intent.brandId, epoch);
  } finally { if (isCurrent(writeGuard, token, intent.accountId, intent.brandId, epoch)) busy.value = false; }
}
function togglePrimary(value: boolean): void { primary.value = value; if (value) enabled.value = true; persistDraft(); }
watch(() => [props.account.id, props.brandId], () => {
  readGuard.invalidate(); historyGuard.invalidate(); writeGuard.invalidate(); record.value = null; rows.value = []; offset.value = 0; hasMore.value = false;
  loading.value = false; historyLoading.value = false; busy.value = false; error.value = ""; notice.value = ""; restoreDraft();
  if (permissions.value.view) { void loadRecord(); void loadHistory(0); }
}, { immediate: true });
watch([operation, domainId, domain, enabled, primary, reason], persistDraft);
onUnmounted(() => { readGuard.invalidate(); historyGuard.invalidate(); writeGuard.invalidate(); });
</script>

<template>
  <section class="brand-domains" aria-labelledby="domains-title">
    <header class="domains-header"><div><h2 id="domains-title">Brand domains</h2><p>Manage domain bindings for the selected brand. Domain changes do not configure DNS or TLS.</p></div><button type="button" class="button secondary" :disabled="!canEdit || (record?.domains.length ?? 100) >= 100" @click="startAdd">Add domain</button></header>
    <p v-if="!permissions.view" class="callout">You do not have brand domain view access for this brand.</p>
    <template v-else>
      <p v-if="readOnly" class="callout">This brand is disabled. Domain changes are read-only. Admin access is available only through another active configured entry.</p>
      <p v-else-if="record?.status === 'paused'" class="callout">This brand is paused. Domain changes remain available.</p>
      <p class="callout">DNS and TLS certificates are not checked here. Disabling the domain currently used for admin access can make management unreachable until another configured entry is used.</p>
      <div v-if="error" class="message error" role="alert">{{ error }}</div><div v-if="notice" class="message notice" role="status">{{ notice }}</div>
      <div class="domains-toolbar"><span v-if="record">Version {{ record.version }} · {{ statusLabel(record.status) }} · {{ record.domains.length }}/100 bindings</span><button type="button" class="button secondary" :disabled="loading" @click="loadRecord">{{ loading ? "Loading…" : "Reload list" }}</button></div>
      <div v-if="record" class="domain-list">
        <article v-for="item in record.domains" :key="item.id" class="domain-row">
          <div class="domain-main"><strong>{{ item.domain }}</strong><span class="badges"><span v-if="item.is_primary" class="badge primary-badge">Primary</span><span class="badge" :class="item.enabled ? 'enabled-badge' : 'disabled-badge'">{{ item.enabled ? "Enabled" : "Disabled" }}</span></span><small>Binding {{ item.id }}</small></div>
          <button type="button" class="button secondary" :disabled="!canEdit" @click="editDomain(item)">Edit</button>
        </article>
        <p v-if="record.domains.length === 0" class="empty">No domain bindings yet.</p>
      </div>
      <div v-if="pending" class="review-panel" aria-live="polite">
        <h3>{{ pending.phase === 'unknown' ? 'Request outcome unknown' : pending.phase === 'conflict' ? 'Version conflict' : 'Review domain change' }}</h3>
        <p>Frozen {{ pending.operation === 'add' ? 'add' : 'edit' }} request · shared version {{ pending.body.version }} · key {{ pending.key }}</p>
        <p><strong>{{ pending.operation === 'add' ? (pending.body as any).domain : record?.domains.find(d => d.id === pending?.domainId)?.domain }}</strong> · {{ pending.body.enabled ? 'enabled' : 'disabled' }}{{ pending.body.is_primary ? ' · primary' : '' }}</p>
        <p>Reason: {{ pending.body.reason }}</p>
        <p v-if="pending.operation === 'edit' && !pending.body.enabled" class="warning">Disabling this entry may make management unreachable if it is the admin host. Use another active configured entry to regain access.</p>
        <p v-if="pending.body.is_primary" class="warning">Setting this binding as primary will demote the current primary in the same transaction. Promotion does not redirect traffic or disable the old host.</p>
        <div class="actions"><button v-if="pending.phase === 'review'" type="button" class="button primary" :disabled="busy || !permissions.write || readOnly" @click="send()">{{ busy ? "Submitting…" : "Confirm and submit" }}</button><button v-if="pending.phase === 'unknown'" type="button" class="button primary" :disabled="busy || !permissions.write || readOnly" @click="send(true)">Retry exact request</button><button v-if="pending.phase === 'conflict'" type="button" class="button secondary" :disabled="!conflictReloaded || busy" @click="abandon">Abandon frozen request</button><button v-if="pending.phase === 'review' || pending.phase === 'conflict' && conflictReloaded" type="button" class="button secondary" :disabled="busy" @click="abandon">Cancel</button></div>
        <small v-if="pending.phase === 'conflict' && !conflictReloaded">Reload the latest state before abandoning this request.</small>
      </div>
      <form v-else-if="canEdit && formOpen" class="domain-form" @submit.prevent="reviewChange">
        <h3>{{ operation === 'add' ? 'Add a domain' : 'Edit binding' }}</h3>
        <label v-if="operation === 'add'" class="field">Domain hostname<input v-model="domain" aria-label="Domain hostname" autocomplete="off" autocapitalize="none" spellcheck="false" maxlength="253" placeholder="play.example.com" :aria-invalid="domain.length > 0 && !isNewDomainValid" /><small>Use a lowercase dotted DNS hostname. URLs, IP addresses, wildcards, local names, and ports are not accepted.</small><small>{{ domainBytes }}/253 UTF-8 bytes</small></label>
        <label v-else class="field">Domain hostname<input :value="domain" aria-label="Domain hostname" readonly /><small>Existing bindings cannot be renamed or reassigned.</small></label>
        <label class="check"><input v-model="enabled" type="checkbox" @change="!enabled && primary ? togglePrimary(false) : persistDraft()" /> Enabled</label>
        <p v-if="operation === 'edit' && !enabled" class="warning">Disabling this entry may make management unreachable if it is the admin host. Use another active configured entry to regain access.</p>
        <label class="check"><input :checked="primary" type="checkbox" :disabled="!enabled" @change="togglePrimary(($event.target as HTMLInputElement).checked)" /> Primary domain</label>
        <p v-if="primaryWillDemote" class="warning">The current primary domain will be demoted in the same transaction. Promotion does not redirect traffic or disable the old host.</p>
        <label class="field">Reason<textarea v-model="reason" aria-label="Reason" rows="3" maxlength="500" placeholder="Why is this domain binding changing?" /><small>{{ reasonBytes }}/500 UTF-8 bytes</small></label>
        <p v-if="(record?.domains.length ?? 0) >= 100 && operation === 'add'" class="warning">This brand has reached the 100 binding limit.</p>
        <div class="actions"><button type="submit" class="button primary" :disabled="busy || !permissions.write || readOnly || (operation === 'add' && (!isNewDomainValid || domainBytes > 253)) || !reason.trim() || reasonBytes > 500 || (operation === 'add' && (record?.domains.length ?? 0) >= 100)">Review change</button><button type="button" class="button secondary" @click="resetForm()">Clear form</button></div>
      </form>
      <section class="history" aria-labelledby="history-title"><div class="section-heading"><div><h3 id="history-title">Domain history</h3><p>Previous and resulting binding snapshots for each change.</p></div><button type="button" class="button secondary" :disabled="historyLoading" @click="loadHistory(0)">Refresh history</button></div>
        <article v-for="item in rows" :key="item.id" class="history-row"><div class="history-meta"><strong>Version {{ item.version }}</strong><time :datetime="item.created_at">{{ timeLabel(item.created_at) }}</time></div><p>{{ item.reason }}</p><small>Actor {{ item.changed_by }} · Audit {{ item.audit_log_id }}</small><details><summary>Compare domain snapshots</summary><div class="snapshot-grid"><div><h4>Before</h4><ul><li v-for="d in item.before_domains" :key="d.id">{{ d.domain }} — {{ d.enabled ? 'enabled' : 'disabled' }}{{ d.is_primary ? ', primary' : '' }}</li><li v-if="!item.before_domains.length">No bindings</li></ul></div><div><h4>After</h4><ul><li v-for="d in item.domains" :key="d.id">{{ d.domain }} — {{ d.enabled ? 'enabled' : 'disabled' }}{{ d.is_primary ? ', primary' : '' }}</li><li v-if="!item.domains.length">No bindings</li></ul></div></div></details></article>
        <p v-if="!historyLoading && rows.length === 0" class="empty">No domain changes recorded.</p><div class="pagination"><button type="button" class="button secondary" :disabled="historyLoading || offset === 0" @click="loadHistory(Math.max(0, offset - 20))">Previous</button><span>Offset {{ offset }}</span><button type="button" class="button secondary" :disabled="historyLoading || !hasMore" @click="loadHistory(offset + 20)">Next</button></div>
      </section>
    </template>
  </section>
</template>

<style scoped>
.brand-domains{color:#1f2937;display:grid;gap:16px;max-width:100%;min-width:0}.brand-domains *{box-sizing:border-box}.domains-header,.domains-toolbar,.section-heading,.domain-row,.history-meta,.actions,.pagination{display:flex;align-items:center;justify-content:space-between;gap:12px}.domains-header{align-items:flex-start}.brand-domains h2,.brand-domains h3,.brand-domains h4,.brand-domains p{margin:0}.brand-domains h2{font-size:1.25rem}.brand-domains h3{font-size:1.05rem}.domains-header p,.section-heading p{color:#667085;margin-top:4px}.button{min-height:44px;padding:9px 14px;border:1px solid #cbd5e1;border-radius:8px;background:#fff;color:#344054;font:inherit;font-weight:600;cursor:pointer}.button:disabled{opacity:.5;cursor:not-allowed}.button.primary{background:#155eef;color:#fff;border-color:#155eef}.domains-toolbar{justify-content:flex-end;color:#667085;font-size:.9rem}.domain-list{display:grid;border:1px solid #e4e7ec;border-radius:10px;overflow:hidden}.domain-row{padding:14px 16px;border-bottom:1px solid #eaecf0}.domain-row:last-child{border-bottom:0}.domain-main{display:grid;gap:5px;min-width:0;overflow-wrap:anywhere}.domain-main strong{font-size:1rem}.domain-main small,.history-row small,.field small,.review-panel small{color:#667085}.badges{display:flex;gap:6px}.badge{width:max-content;padding:2px 8px;border-radius:999px;font-size:.75rem;background:#f2f4f7}.primary-badge{background:#eff8ff;color:#175cd3}.enabled-badge{background:#ecfdf3;color:#027a48}.disabled-badge{background:#f2f4f7;color:#475467}.empty{padding:16px;color:#667085}.domain-form,.review-panel{display:grid;gap:14px;padding:18px;border:1px solid #d0d5dd;border-radius:10px;background:#fcfcfd}.field{display:grid;gap:6px;font-weight:600}.field input,.field textarea{width:100%;min-height:44px;padding:10px 12px;border:1px solid #98a2b3;border-radius:7px;background:#fff;color:#101828;font:inherit}.field textarea{min-height:88px;resize:vertical}.check{display:flex;align-items:center;gap:10px;min-height:44px}.check input{width:20px;height:20px}.warning{padding:10px 12px;border-radius:7px;background:#fffaeb;color:#93370d}.message,.callout{padding:12px 14px;border-radius:8px;background:#f2f4f7}.message.error{background:#fef3f2;color:#b42318}.message.notice{background:#ecfdf3;color:#027a48}.history{display:grid;gap:12px}.history-row{display:grid;gap:8px;padding:14px 16px;border:1px solid #e4e7ec;border-radius:9px;overflow-wrap:anywhere}.history-meta time{color:#667085;font-size:.88rem}.history-row details{margin-top:3px}.history-row summary{min-height:44px;display:flex;align-items:center;cursor:pointer;font-weight:600}.snapshot-grid{display:grid;grid-template-columns:1fr 1fr;gap:14px}.snapshot-grid ul{padding-left:20px;overflow-wrap:anywhere}.pagination{justify-content:flex-end}.pagination span{color:#667085;font-size:.9rem}.review-panel>p{overflow-wrap:anywhere}.actions{justify-content:flex-start;flex-wrap:wrap}@media(max-width:600px){.domains-header,.section-heading{align-items:flex-start;flex-direction:column}.domain-row{align-items:flex-start}.snapshot-grid{grid-template-columns:1fr}.history-meta{align-items:flex-start;flex-direction:column;gap:3px}.domains-toolbar{flex-wrap:wrap}.domain-form,.review-panel{padding:14px}}
.brand-domains .field{height:auto;padding:0;border:0;background:transparent}
.brand-domains .check{height:44px;min-height:44px;padding:0;border:0;background:transparent;cursor:pointer}
.brand-domains .check input{width:20px;height:20px;flex:0 0 20px}
</style>
