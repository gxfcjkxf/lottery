<script setup lang="ts">
import ManagementList from '@lottery/shared/ManagementList.vue'
import ManagementTable from '@lottery/shared/ManagementTable.vue'
import MemberActionsMenu from '@lottery/shared/MemberActionsMenu.vue'
import TablePagination from '@lottery/shared/TablePagination.vue'
import { useOperationToast, vGlobalLoading } from "@lottery/shared";
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
import { withGlobalLoading } from '@lottery/shared';
import DetailModal from '@lottery/shared/DetailModal.vue';
import { vInlineValidation } from '@lottery/shared'
import { computed, onUnmounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { brandDomainsPermissions, createBrandDomainsApi, isCanonicalBrandDomain, type BrandDomain, type BrandDomainsRecord } from "./brand-domains-api";
import { brandDomainsSessionGeneration, classifyBrandDomainsFailure, clearAllPendingBrandDomainsWrites, clearBrandDomainsDraft, clearPendingBrandDomainsWrite, createBrandDomainsRequestGuard, getBrandDomainsDraft, getPendingBrandDomainsWrite, retainPendingBrandDomainsWrite, setBrandDomainsDraft, setPendingBrandDomainsWrite, type PendingBrandDomainsWrite } from "./brand-domains-state";
import { useAdminI18n } from "./i18n";
const pageSize = ref(50)


const props = defineProps<{ account: AdminAccount; brandId: string; brandStatus?: string }>();
const { t, message, display, locale } = useAdminI18n();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createBrandDomainsApi();
const readGuard = createBrandDomainsRequestGuard(), writeGuard = createBrandDomainsRequestGuard();
const permissions = computed(() => brandDomainsPermissions(props.account, props.brandId));
const scope = () => ({ accountId: props.account.id, brandId: props.brandId });
const record = ref<BrandDomainsRecord | null>(null);
const domainPage = ref(1)
const domainRows = computed(() => record.value?.domains.slice((domainPage.value - 1) * pageSize.value, domainPage.value * pageSize.value) ?? [])
watch(() => props.brandId, () => { domainPage.value = 1 })
watch(() => record.value?.domains.length, count => { domainPage.value = Math.min(domainPage.value, Math.max(1, Math.ceil((count ?? 0) / pageSize.value))) })
const pending = ref<PendingBrandDomainsWrite | null>(null);
const operation = ref<"add" | "edit">("add"), domainId = ref<string | undefined>();
const formOpen = ref(false);
const domain = ref(""), enabled = ref(false), reason = ref("");
const loading = ref(false), busy = ref(false), readFailed = ref(false), confirmationOpen = ref(false);
const error = ref<string | import("@lottery/shared").LocalizedMessage>(""), notice = ref<string | import("@lottery/shared").LocalizedMessage>(""), conflictReloaded = ref(false);
const readOnly = computed(() => props.brandStatus === "disabled" || record.value?.status === "disabled");
const conflict = computed(() => pending.value?.phase === "conflict");
const canEdit = computed(() => permissions.value.write && !readOnly.value && !pending.value && !busy.value && !loading.value && !readFailed.value && Boolean(record.value));
const domainCharacters = computed(() => Array.from(domain.value).length), reasonCharacters = computed(() => Array.from(reason.value).length);
const isNewDomainValid = computed(() => operation.value === "edit" || isCanonicalBrandDomain(domain.value));


function restoreDraft(): void {
  const stored = getBrandDomainsDraft(scope()), intent = getPendingBrandDomainsWrite(scope());
  pending.value = intent; confirmationOpen.value = Boolean(intent); conflictReloaded.value = false;
  if (intent) {
    if(intent.operation==='delete'){formOpen.value=false;return}
    formOpen.value = true;
    operation.value = intent.operation; domainId.value = intent.domainId;
    domain.value = intent.operation === "add" ? String((intent.body as { domain: string }).domain) : record.value?.domains.find((d) => d.id === intent.domainId)?.domain ?? "";
    if ('enabled' in intent.body) {enabled.value = intent.body.enabled; reason.value = intent.body.reason;}
  } else if (stored) {
    formOpen.value = true;
    operation.value = stored.operation; domainId.value = stored.domainId; domain.value = stored.domain; enabled.value = stored.enabled; reason.value = stored.reason;
  } else resetForm(false);
}
function persistDraft(): void {
  if (!formOpen.value || pending.value || getPendingBrandDomainsWrite(scope()) || !permissions.value.write || readOnly.value) return;
  setBrandDomainsDraft({ ...scope(), operation: operation.value, ...(domainId.value ? { domainId: domainId.value } : {}), domain: domain.value, enabled: enabled.value, reason: reason.value });
}
function resetForm(clear = true): void {
  formOpen.value = false;
  operation.value = "add"; domainId.value = undefined; domain.value = ""; enabled.value = false; reason.value = "";
  if (clear) clearBrandDomainsDraft(scope());
}
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
    record.value = next; readFailed.value = false;
    if (pending.value?.operation === "edit") domain.value = next.domains.find((item) => item.id === pending.value?.domainId)?.domain ?? "";
    if (conflict.value) conflictReloaded.value = true; return true;
  } catch (cause) {
    if (isCurrent(readGuard, token, accountId, brandId, epoch)) { readFailed.value = true; error.value = cause instanceof Error ? publicErrorMessage(cause, "read") : message("admin.copy.9e490389"); sessionError(cause, readGuard, token, accountId, brandId, epoch); }
    return false;
  } finally { if (isCurrent(readGuard, token, accountId, brandId, epoch)) loading.value = false; }
}

function editDomain(item: BrandDomain): void {
  if (!canEdit.value) return;
  formOpen.value = true; operation.value = "edit"; domainId.value = item.id; domain.value = item.domain; enabled.value = item.enabled; reason.value = ""; persistDraft();
}
function startAdd(): void { if (!canEdit.value || (record.value?.domains.length ?? 100) >= 100) return; resetForm(false); formOpen.value = true; persistDraft(); }
function reviewChange(): void {
  error.value = ""; notice.value = "";
  const existing = getPendingBrandDomainsWrite(scope());
  if (existing) { pending.value = existing; return; }
  if (!record.value || !canEdit.value || reasonCharacters.value > 500 || (operation.value === "add" && (!isCanonicalBrandDomain(domain.value) || domainCharacters.value > 253))) {
    error.value = message('admin.copy.89cceb00'); return;
  }
  if (operation.value === "edit" && !record.value.domains.some((item) => item.id === domainId.value)) { error.value = message('admin.copy.4219d6c9'); return; }
  const body = operation.value === "add"
    ? { version: record.value.version, domain: domain.value, enabled: enabled.value, reason: reason.value }
    : { version: record.value.version, enabled: enabled.value, reason: reason.value };
  const intent: PendingBrandDomainsWrite = { ...scope(), operation: operation.value, ...(domainId.value ? { domainId: domainId.value } : {}), body, key: createIdempotencyKey(), phase: "review" };
  setPendingBrandDomainsWrite(scope(), intent); pending.value = getPendingBrandDomainsWrite(scope()); conflictReloaded.value = false; confirmationOpen.value = true;
}
function reviewDelete(item:BrandDomain):void {
  if(!canEdit.value||!record.value)return;
  error.value='';notice.value='';
  const intent:PendingBrandDomainsWrite={...scope(),operation:'delete',domainId:item.id,domainName:item.domain,body:{version:record.value.version},key:createIdempotencyKey(),phase:'review'};
  setPendingBrandDomainsWrite(scope(),intent);pending.value=getPendingBrandDomainsWrite(scope());confirmationOpen.value=true;conflictReloaded.value=false;
}
function abandon(): void {
  const intent = pending.value;
  if (busy.value || !intent || intent.phase === "unknown" || (intent.phase === "conflict" && !conflictReloaded.value)) return;
  if (getPendingBrandDomainsWrite(scope())?.key !== intent.key) { pending.value = getPendingBrandDomainsWrite(scope()); return; }
  clearPendingBrandDomainsWrite(scope(), intent.key); pending.value = getPendingBrandDomainsWrite(scope()); confirmationOpen.value = false;
  if (intent.phase === 'conflict') resetForm(); else persistDraft();
  notice.value = "";
}
function closeConfirmation(): void { if (busy.value) return; if (pending.value?.phase === 'review') abandon(); else confirmationOpen.value = false; }
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
    else if(intent.operation==='delete') await api.remove(intent.brandId,intent.domainId!,intent.body,intent.key);
    else await api.patch(intent.brandId, intent.domainId!, intent.body as import("./brand-domains-api").BrandDomainPatchBody, intent.key);
    if (epoch !== brandDomainsSessionGeneration()) return;
    if (getPendingBrandDomainsWrite(ownScope)?.key === intent.key) {
      clearPendingBrandDomainsWrite(ownScope, intent.key);
      clearBrandDomainsDraft(ownScope);
    }
    if (!isCurrent(writeGuard, token, intent.accountId, intent.brandId, epoch)) return;
    pending.value = null; confirmationOpen.value = false; resetForm(); notice.value = message("admin.settings.saved");
    const refreshed = await loadRecord();
    if (isCurrent(writeGuard, token, intent.accountId, intent.brandId, epoch)) notice.value = refreshed ? message("admin.settings.saved") : message("admin.settings.savedReadFailed");
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
    if (kind === "unknown") error.value = message('admin.copy.c8825e5c');
    else if (kind === "conflict") { error.value = message('admin.copy.796db30c'); conflictReloaded.value = false; await loadRecord(); }
    else { confirmationOpen.value=false;error.value = publicErrorMessage(cause); if (pending.value === null) persistDraft(); }
    sessionError(cause, writeGuard, token, intent.accountId, intent.brandId, epoch);
  } finally { if (isCurrent(writeGuard, token, intent.accountId, intent.brandId, epoch)) busy.value = false; }
}
watch(() => [props.account.id, props.brandId], () => {
  readGuard.invalidate(); writeGuard.invalidate(); record.value = null; readFailed.value = false;
  loading.value = false; busy.value = false; error.value = ""; notice.value = ""; restoreDraft();
  if (permissions.value.view) { void withGlobalLoading(loadRecord); }
}, { immediate: true });
watch([operation, domainId, domain, enabled, reason], persistDraft);
onUnmounted(() => { readGuard.invalidate(); writeGuard.invalidate(); });

useOperationToast(() => error.value, value => display(value), 'error');
useOperationToast(() => notice.value, value => display(value), 'success');
</script>

<template>
  <ManagementList class="brand-domains" :busy="loading" :aria-label="t('admin.copy.802d25a8')">
    <template #actions><button type="button" class="button button-secondary" :disabled="!canEdit || (record?.domains.length ?? 100) >= 100" @click="startAdd">{{ t('admin.copy.29c3e74b') }}</button></template>
    <p v-if="!permissions.view" class="callout">{{ t('admin.copy.39e5f659') }}</p>
    <template v-else>
      <p v-if="readOnly" class="callout">{{ t('admin.copy.14b0c20f') }}</p>

      <button v-if="readFailed || (conflict&&!conflictReloaded)" class="button button-secondary" type="button" :disabled="loading" @click="withGlobalLoading(loadRecord)">{{t('admin.settings.readAgain')}}</button>
      <p v-if="loading&&!record" v-global-loading role="status">{{t('admin.copy.27342bf3')}}</p>
      <ManagementTable :entries="domainRows" :columns="[{key:'domain',label:t('admin.copy.f79c7aea')},{key:'status',label:t('admin.copy.a765b3ca')},{key:'actions',label:t('admin.copy.d21fcbef')}]" :empty="Boolean(record) && !loading && !readFailed && !record!.domains.length" :empty-label="t('admin.copy.f21c7235')">
        <template #domain="{entry:item}">{{ item.domain }}</template>
        <template #status="{entry:item}"><span class="status-pill" :class="item.enabled ? 'active' : 'disabled'">{{ t(item.enabled ? 'admin.copy.5eb01613' : 'admin.copy.5e3326db') }}</span></template>
        <template #actions="{entry:item}"><MemberActionsMenu :id="'domain-actions-'+item.id" :label="t('admin.copy.d21fcbef')" :disabled="!canEdit"><button type="button" role="menuitem" @click="editDomain(item)">{{t('admin.copy.24dfbf6c')}}</button><button type="button" role="menuitem" class="danger-text" @click="reviewDelete(item)">{{t('admin.domains.delete')}}</button></MemberActionsMenu></template>
      </ManagementTable>
      <button v-if="pending" class="button button-secondary" type="button" @click="confirmationOpen=true">{{t('admin.settings.reviewPending')}}</button>
      <DetailModal v-if="pending&&confirmationOpen" class="brand-settings-dialog" :title="t('admin.settings.confirmSave')" :close-label="t('association.close')" :close-disabled="busy" @close="closeConfirmation">
      <div class="review-panel">

        <p v-if="pending.operation==='delete'">{{t('admin.domains.confirmDelete',{domain:pending.domainName!})}}</p>
        <p v-if="pending.operation==='delete'" class="warning">{{t('admin.domains.deleteEffect')}}</p>
        <p v-if="'enabled' in pending.body"><strong>{{ pending.operation === 'add' ? (pending.body as any).domain : record?.domains.find(d => d.id === pending?.domainId)?.domain }}</strong> {{ pending.body.enabled ? t('admin.copy.8f5a84b3') : t('admin.copy.844522fb') }}</p>
        <p v-if="'reason' in pending.body&&pending.body.reason">{{ t('admin.copy.2809a1e6') }}{{ pending.body.reason }}</p>
        <p v-if="pending.operation === 'edit' && 'enabled' in pending.body && !pending.body.enabled" class="warning">{{ t('admin.copy.1e634e9a') }}</p>
        <div class="actions ui-form-actions"><button v-write-guard="busy" v-if="pending.phase === 'review'" type="button" class="button button-primary" :disabled="busy || !permissions.write || readOnly" @click="withGlobalLoading(send)">{{ busy ? t('admin.copy.46511dc0') : pending.operation==='delete'?t('admin.domains.confirmAction'):t('admin.copy.3135aeca') }}</button><button v-write-guard="busy" v-if="pending.phase === 'unknown'" type="button" class="button button-primary" :disabled="busy || !permissions.write || readOnly" @click="withGlobalLoading(send, true)">{{ t('admin.settings.retryOperation') }}</button><button v-if="pending.phase === 'conflict'" type="button" class="button button-secondary" :disabled="!conflictReloaded || busy" @click="abandon">{{ t('admin.settings.startAgain') }}</button></div>
        <button v-if="pending.phase==='conflict'&&!conflictReloaded" class="button button-secondary" type="button" :disabled="loading||busy" @click="withGlobalLoading(loadRecord)">{{t('admin.settings.readAgain')}}</button>
        <small v-if="pending.phase === 'conflict' && !conflictReloaded">{{ t('admin.copy.7bf458e5') }}</small>
      </div>
      </DetailModal>
      <DetailModal v-if="!pending && permissions.write && !readOnly && formOpen" class="brand-settings-dialog" :title="operation === 'add' ? t('admin.copy.56cdfefa') : t('admin.copy.23ee7918')" :close-label="t('association.close')" @close="resetForm()">

      <form novalidate v-inline-validation class="domain-form ui-form" @submit.prevent="withGlobalLoading(reviewChange)">
        <label v-if="operation === 'add'" class="field">{{ t('admin.copy.f79c7aea') }}<input v-model="domain" :aria-label="t('admin.copy.f79c7aea')" autocomplete="off" autocapitalize="none" spellcheck="false" maxlength="253" placeholder="play.example.com" :aria-invalid="domain.length > 0 && !isNewDomainValid" /></label>
        <label v-else class="field">{{ t('admin.copy.f79c7aea') }}<input :value="domain" :aria-label="t('admin.copy.f79c7aea')" readonly /></label>
        <label class="check"><input v-model="enabled" type="checkbox" @change="persistDraft()" /> {{ t('admin.copy.5eb01613') }}</label>
        <p v-if="operation === 'edit' && !enabled" class="warning">{{ t('admin.copy.1e634e9a') }}</p>

        <p v-if="(record?.domains.length ?? 0) >= 100 && operation === 'add'" class="warning">{{ t('admin.copy.32ce80e9') }}</p>
        <div class="actions ui-form-actions"><button type="submit" class="button button-primary" :disabled="busy || !permissions.write || readOnly || (operation === 'add' && (!isNewDomainValid || domainCharacters > 253))  || reasonCharacters > 500 || (operation === 'add' && (record?.domains.length ?? 0) >= 100)">{{ t('admin.copy.d3410bba') }}</button></div>
      </form>
      </DetailModal>
    </template>
    <template v-if="record && permissions.view" #pagination><TablePagination :total="String(record.domains.length)" :count="domainRows.length" :page="domainPage" :page-size="pageSize" :locale="locale" :previous-disabled="loading || busy || Boolean(pending) || domainPage===1" :next-disabled="loading || busy || Boolean(pending) || domainPage*pageSize>=record.domains.length" :go-to="async target => { domainPage=target }" :change-page-size="async (size: number) => { pageSize = size; domainPage = 1; await loadRecord() }" :busy="loading || busy" /></template>
  </ManagementList>
</template>

<style scoped>
.domains-header,.domain-row,.domain-actions,.actions { display:flex;align-items:center;gap:12px; }
.domains-header { justify-content:flex-end; }
.domain-list { min-width:0; }
.domain-row { justify-content:space-between;padding:10px 0;border-bottom:1px solid var(--line-subtle); }
.domain-row:last-child { border-bottom:0; }
.domain-actions { flex-wrap:wrap;justify-content:flex-end; }
.domain-main { display:grid;gap:6px;min-width:0;overflow-wrap:anywhere; }
.badges { display:flex;gap:6px;flex-wrap:wrap; }
.badge { width:max-content;padding:2px 8px;border-radius:999px;background:var(--canvas);color:var(--ink-secondary); }
.enabled-badge { background:var(--success-bg);color:var(--success-text); }
.disabled-badge { background:var(--line-subtle);color:var(--muted); }
.domain-form,.review-panel { display:grid;gap:12px;min-width:0; }
.domain-form h3,.review-panel h3,.review-panel p { margin:0;overflow-wrap:anywhere; }
.field { display:grid;gap:6px;min-width:0;height:auto;padding:0;border:0;background:transparent; }
.field input { box-sizing:border-box;width:100%;min-width:0; }
.field small { color:var(--muted); }
.check { display:flex;align-items:center;gap:8px;min-height:44px;padding:0;border:0;background:transparent;cursor:pointer; }
.check input { width:18px;height:18px;flex:0 0 18px;accent-color:var(--primary); }
.warning,.callout { padding:10px 12px;background:var(--canvas);border-radius:var(--radius-sm); }
.warning { background:var(--warning-bg);color:var(--warning-text); }
.message.error { background:var(--danger-bg);color:var(--danger-text);padding:12px;border-radius:var(--radius-sm); }
.message.notice { color:var(--primary-dark); }
.actions { justify-content:flex-start;flex-wrap:wrap; }
</style>
