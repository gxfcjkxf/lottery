<script setup lang="ts">
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { vWriteGuard } from "@lottery/shared";
import DetailModal from "@lottery/shared/DetailModal.vue";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName, associationLabel, copyAssociationNames } from "@lottery/shared";
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
import { vInlineValidation } from '@lottery/shared'
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { createJoinCodesApi, joinCodePermissions, validJoinCodeDate, validJoinCodeDateRange, type CreateJoinCodeBody, type JoinCode, type JoinCodeKind, type JoinCodeStatus, type UpdateJoinCodeBody } from "./join-codes-api";
import { classifyJoinCodeWriteFailure, clearAllPendingJoinCodeWrites, clearPendingJoinCodeWrite, freezeJoinCodeBody, getPendingJoinCodeWrite, setPendingJoinCodeWrite, type JoinCodeWriteOperation, type JoinCodeWriteBody, type PendingJoinCodeWrite } from "./join-codes-state";
import { adminKey, useAdminI18n } from "./i18n";

const props = defineProps<{ account: AdminAccount; brandId: string; memberId: string; kind: JoinCodeKind; agentId?: string; ownerRecord: object }>();
const { t, display } = useAdminI18n();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createJoinCodesApi();
const permissions = computed(() => joinCodePermissions(props.account, props.brandId));
const canView = computed(() => permissions.value.view);
const canWrite = computed(() => permissions.value.write);
const permissionSignature = computed(() => JSON.stringify({ accountId: props.account.id, brandId: props.brandId,
  superAdmin: props.account.super_admin, brandIds: [...props.account.brand_ids].sort(),
  brandPermissions: [...(props.account.permissions_by_brand?.[props.brandId] ?? [])].sort(),
  platformPermissions: [...(props.account.platform_permissions ?? [])].sort() }));
const pageSize = 20;
const offset = ref(0);
const items = ref<JoinCode[]>([]);
const total = ref("0");
const selected = ref<JoinCode | null>(null);
const createOpen = ref(false);
const historyItems = ref<Awaited<ReturnType<typeof api.history>>["items"]>([]);
const historyOffset = ref(0);
const historyTotal = ref("0");
const historyOpen = ref(false);
const form = ref({ kind: props.kind, owner_member_id: props.memberId, agent_id: props.agentId ?? "", status: "active" as JoinCodeStatus,
  starts_at: "", expires_at: "" });
const loading = ref(false);
const writing = ref(false);
const error = ref<string | import("@lottery/shared").LocalizedMessage>("");
const notice = ref("");
const pending = ref<PendingJoinCodeWrite | null>(null);
const review = ref(false);
const pendingOpen = ref(false);
watch(pending, value => { pendingOpen.value = Boolean(value); });
let scopeGeneration = 0;
let listGeneration = 0;
let detailGeneration = 0;
let historyGeneration = 0;
let writeGeneration = 0;
let disposed = false;
let activeAccountId = props.account.id;
const activeQuery = ref(0);
function scope() { return { accountId: props.account.id, brandId: props.brandId }; }
function currentScope(ticket: number, accountId: string, brandId: string) {
  return !disposed && ticket === scopeGeneration && props.account.id === accountId && props.brandId === brandId;
}
function currentWrite(ticket: number, accountId: string, brandId: string, grant: string) {
  return !disposed && ticket === writeGeneration && props.account.id === accountId && props.brandId === brandId &&
    permissionSignature.value === grant && canWrite.value;
}
function restoreIntent() { pending.value = getPendingJoinCodeWrite(scope()); review.value = false; }
function handleError(cause: unknown) {
  if (cause instanceof AdminApiError && cause.status === 401) {
    clearAllPendingJoinCodeWrites(); pending.value = null; emit("session-invalid");
  }
  error.value = cause instanceof Error ? publicErrorMessage(cause) : "请求失败";
}
function filters() {
  return { kind: props.kind, owner_member_id: props.memberId };
}
async function loadList(): Promise<boolean> {
  if (!canView.value || !props.brandId) { items.value = []; total.value = "0"; restoreIntent(); return false; }
  const ticket = ++listGeneration, scopeTicket = scopeGeneration, accountId = props.account.id, brandId = props.brandId;
  const query = activeQuery.value;
  const requestedFilters = Object.freeze({ ...filters() });
  const requestedOffset = offset.value;
  const requestedLimit = pageSize;
  review.value = false;
  items.value = []; total.value = "0";
  loading.value = true; error.value = "";
  try {
    const result = await api.list(brandId, requestedFilters, requestedLimit, requestedOffset);
    if (ticket !== listGeneration || !currentScope(scopeTicket, accountId, brandId) || query !== activeQuery.value) return false;
    if (result.brand_id !== brandId || result.kind !== (requestedFilters.kind ?? null) || result.owner_member_id !== (requestedFilters.owner_member_id ?? null) ||
        result.limit !== requestedLimit || result.offset !== requestedOffset)
      throw new Error("加入码列表响应与当前筛选范围不匹配。");
    items.value = result.items; total.value = result.total_count; restoreIntent(); return true;
  } catch (cause) { if (ticket === listGeneration && currentScope(scopeTicket, accountId, brandId) && query === activeQuery.value) handleError(cause); return false; }
  finally { if (ticket === listGeneration && currentScope(scopeTicket, accountId, brandId)) loading.value = false; }
}
async function search() { offset.value = 0; activeQuery.value++; selected.value = null; historyItems.value = []; historyOpen.value = false; await loadList(); }
async function openCode(item: JoinCode) {
  if (!canView.value) return;
  const ticket = ++detailGeneration, scopeTicket = scopeGeneration, brandId = props.brandId, accountId = props.account.id;
  error.value = "";
  try {
    const detail = await api.get(brandId, item.id);
    if (ticket !== detailGeneration || !currentScope(scopeTicket, accountId, brandId)) return;
    if (detail.id !== item.id || detail.brand_id !== brandId || detail.kind !== item.kind || detail.code !== item.code ||
        detail.owner_member_id !== item.owner_member_id || detail.agent_id !== item.agent_id)
      throw new Error("加入码详情与所选列表记录不匹配。");
    createOpen.value = false; selected.value = detail; form.value = { kind: detail.kind, owner_member_id: detail.owner_member_id, agent_id: detail.agent_id ?? "",
      status: detail.status, starts_at: detail.starts_at ?? "", expires_at: detail.expires_at ?? "" };
    historyItems.value = []; historyOffset.value = 0; historyOpen.value = false; restoreIntent();
  } catch (cause) { if (ticket === detailGeneration && currentScope(scopeTicket, accountId, brandId)) handleError(cause); }
}
function startCreate() {
  createOpen.value = true;
  selected.value = null; historyItems.value = []; historyOpen.value = false;
  form.value = { kind: props.kind, owner_member_id: props.memberId, agent_id: props.agentId ?? "", status: "active", starts_at: "", expires_at: "" };
}
function dateInput(value: string): string | null { return value.trim() === "" ? null : value.trim(); }
async function prepareWrite(operation: JoinCodeWriteOperation) {
  if (!canWrite.value || !props.brandId) return;
  let body: JoinCodeWriteBody;
  let identity: PendingJoinCodeWrite["identity"];
  let resourceId: string | null;
  if (operation === "create") {
    if (props.kind !== 'agent' || !props.agentId) return;
    const create: CreateJoinCodeBody = { kind: props.kind, owner_member_id: props.memberId,
      agent_id: props.agentId, starts_at: dateInput(form.value.starts_at), expires_at: dateInput(form.value.expires_at) };
    body = create; resourceId = null;
    identity = { kind: create.kind, code: null, owner_member_id: create.owner_member_id, agent_id: create.agent_id };
    copyAssociationNames(props.ownerRecord, create); copyAssociationNames(props.ownerRecord, identity);
  } else {
    if (!selected.value) return;
    const update: UpdateJoinCodeBody = { version: selected.value.version, status: form.value.status,
      starts_at: dateInput(form.value.starts_at), expires_at: dateInput(form.value.expires_at) };
    body = update; resourceId = selected.value.id;
    identity = { kind: selected.value.kind, code: selected.value.code, owner_member_id: selected.value.owner_member_id, agent_id: selected.value.agent_id };
    copyAssociationNames(selected.value,identity);
  }
  const sharedScope = scope(), old = getPendingJoinCodeWrite(sharedScope);
  if (old && (old.operation !== operation || old.resourceId !== resourceId || JSON.stringify(old.body) !== JSON.stringify(body))) {
    pending.value = old; review.value = false;
    error.value = "此账号和品牌已有未确认请求，请先按原内容和幂等键核对或重试。"; return;
  }
  const intent: PendingJoinCodeWrite = old ?? { ...sharedScope, operation, resourceId, identity, body: freezeJoinCodeBody(body), key: createIdempotencyKey() };
  setPendingJoinCodeWrite(sharedScope, intent); pending.value = getPendingJoinCodeWrite(sharedScope); review.value = false;
  error.value = ""; notice.value = "请检查冻结的请求内容与归属后确认。";
}
async function confirmWrite() {
  const intent = pending.value;
  if (!intent || loading.value || writing.value || !review.value || !canWrite.value || intent.accountId !== props.account.id || intent.brandId !== props.brandId) return;
  const ticket = ++writeGeneration, grant = permissionSignature.value, accountId = intent.accountId, brandId = intent.brandId;
  const isCurrent = () => currentWrite(ticket, accountId, brandId, grant);
  const sharedScope = { accountId, brandId };
  writing.value = true; error.value = ""; notice.value = "";
  let acknowledged = false;
  try {
    if (intent.operation === "create") await api.create(brandId, intent.body as CreateJoinCodeBody, intent.key);
    else await api.update(brandId, intent.resourceId!, intent.identity, intent.body as UpdateJoinCodeBody, intent.key);
    acknowledged = true;
    createOpen.value = false;
    // A matching SDK receipt confirms only this request. Clear before the independent GET.
    clearPendingJoinCodeWrite(sharedScope, intent.key);
    if (!isCurrent()) return;
    pending.value = getPendingJoinCodeWrite(sharedScope);
    notice.value = "已收到与冻结请求匹配的回执，正在独立读取当前状态。";
    if (intent.operation === "update" && selected.value?.id === intent.resourceId) {
      const detail = await api.get(brandId, intent.resourceId!);
      if (!isCurrent()) return;
      selected.value = detail;
    }
    if (!isCurrent()) return;
    offset.value = intent.operation === "create" ? 0 : offset.value;
    activeQuery.value++;
    const refreshed = await loadList();
    if (isCurrent()) notice.value = refreshed ? "写入已确认；页面显示的是随后读取的当前状态。" : "写入已确认，但刷新当前目录失败；可稍后重新查询。";
  } catch (cause) {
    if (!isCurrent()) return;
    const status = cause instanceof AdminApiError ? cause.status : undefined;
    if (status === 401) { writing.value = false; clearAllPendingJoinCodeWrites(); pending.value = null; emit("session-invalid"); return; }
    if (acknowledged) {
      notice.value = "写入已确认，但读取当前状态失败；原请求已确认，不需要再次提交。";
      error.value = cause instanceof Error ? publicErrorMessage(cause) : "当前状态读取失败。";
      return;
    }
    if (classifyJoinCodeWriteFailure(status) === "uncertain") {
      pending.value = getPendingJoinCodeWrite(sharedScope); review.value = false;
      notice.value = "写入结果未知。冻结的请求正文和幂等键已保留；可按原请求重试。";
      error.value = cause instanceof Error ? publicErrorMessage(cause) : "请求结果未知。";
    } else {
      clearPendingJoinCodeWrite(sharedScope, intent.key); pending.value = getPendingJoinCodeWrite(sharedScope);
      error.value = publicErrorMessage(cause);
    }
  } finally { if (isCurrent()) writing.value = false; }
}
async function retryFrozen() { await confirmWrite(); }
async function loadHistory() {
  if (!selected.value || !canView.value) return;
  const ticket = ++historyGeneration, scopeTicket = scopeGeneration, accountId = props.account.id, brandId = props.brandId, id = selected.value.id;
  try {
    const result = await api.history(brandId, id, pageSize, historyOffset.value);
    if (ticket !== historyGeneration || !currentScope(scopeTicket, accountId, brandId) || selected.value?.id !== id) return;
    if (result.brand_id !== brandId || result.code_id !== id) throw new Error("加入码历史响应范围不匹配。");
    historyItems.value = result.items; historyTotal.value = result.total_count; historyOpen.value = true;
  } catch (cause) { if (ticket === historyGeneration && currentScope(scopeTicket, accountId, brandId)) handleError(cause); }
}
async function changePage(direction: -1 | 1) { offset.value = Math.max(0, offset.value + direction * pageSize); await loadList(); }
function validTime(value: string) {
  if (!value.trim()) return true;
  return validJoinCodeDate(value.trim());
}
const formValid = computed(() => {
  const start = dateInput(form.value.starts_at), end = dateInput(form.value.expires_at);
  return Boolean((selected.value || props.kind === "agent" && props.memberId && props.agentId) &&
    validTime(form.value.starts_at) && validTime(form.value.expires_at) &&
    validJoinCodeDateRange(start, end));
});
function pendingTitle(intent: PendingJoinCodeWrite) { return intent.operation === "create" ? t(intent.identity.kind === 'agent' ? 'admin.join.createAgent' : 'admin.join.createReferral') : `${t('admin.copy.ddfd358b')} ${intent.identity.code}`; }
function historyRange(item: { starts_at: string | null; expires_at: string | null }) { return `${item.starts_at ?? t('admin.copy.399ab167')} → ${item.expires_at ?? t('admin.copy.f647d099')}`; }
function localizedMessage(value: string | import("@lottery/shared").LocalizedMessage): string {
  if (typeof value !== "string") return displayPublicError(value);
  const translations: Record<string, string> = {
    "此账号和品牌已有未确认请求，请先按原内容和幂等键核对或重试。": 'admin.copy.67e735db',
    "请检查冻结的请求内容与归属后确认。": 'admin.copy.21e7a50c',
    "已收到与冻结请求匹配的回执，正在独立读取当前状态。": 'admin.copy.f85d4003',
    "写入已确认；页面显示的是随后读取的当前状态。": 'admin.copy.06415e47',
    "写入已确认，但刷新当前目录失败；可稍后重新查询。": 'admin.copy.08822376',
    "写入已确认，但读取当前状态失败；原请求已确认，不需要再次提交。": 'admin.copy.c5aa0bfc',
    "写入结果未知。冻结的请求正文和幂等键已保留；可按原请求重试。": 'admin.copy.e8f1d2b7',
  };
  const pair = translations[value];
  return pair ? t(adminKey(pair)) : value;
}
onMounted(() => { restoreIntent(); void loadList(); });
onBeforeUnmount(() => { disposed = true; scopeGeneration++; listGeneration++; detailGeneration++; historyGeneration++; writeGeneration++; });
watch(() => [props.brandId, props.account.id, permissionSignature.value] as const, () => {
  if (activeAccountId !== props.account.id) { clearAllPendingJoinCodeWrites(); activeAccountId = props.account.id; }
  scopeGeneration++; listGeneration++; detailGeneration++; historyGeneration++; writeGeneration++; items.value = []; total.value = "0"; selected.value = null; historyItems.value = []; historyOpen.value = false;
  offset.value = 0; error.value = ""; notice.value = ""; loading.value = false; writing.value = false; restoreIntent();
  if (canView.value) void loadList();
}, { flush: "sync" });
</script>

<template>
  <section class="join-codes">

    <div v-if="error && !selected && !createOpen && !pendingOpen" class="jc-message is-error" role="alert">{{ localizedMessage(error) }}</div><p v-if="notice" class="jc-message is-ok" role="status">{{ localizedMessage(notice) }}</p>
    <p v-if="!canView" class="jc-note">{{ t('admin.copy.9901b15b') }}</p>
    <template v-else>
      <section class="jc-card"><div class="jc-card-head"><div><h3>{{ t(kind === 'agent' ? 'admin.copy.ae9fa0bf' : 'admin.copy.d245e2d6') }}</h3><p>{{ t('admin.copy.acbd5616') }} {{ total }} {{ t('admin.copy.d71e8d3c') }}</p></div><button v-if="canWrite && kind === 'agent'" type="button" class="jc-secondary button button-secondary" @click="startCreate">{{ t('admin.join.newAgent') }}</button></div>
        <p v-global-loading v-if="loading" class="jc-note" role="status">{{ t('admin.copy.439fda03') }}</p><div class="jc-list">
          <article v-for="item in items" :key="item.id" class="jc-row"><div class="jc-row-main"><strong><code>{{ item.code }}</code></strong><span>{{ item.status === "active" ? t('admin.copy.98d24810') : t('admin.copy.5e3326db') }}</span><small>{{ historyRange(item) }} · {{ item.usable ? t('admin.copy.36437c04') : t('admin.copy.36fcc900') }}</small></div><div class="jc-actions"><button type="button" class="jc-secondary button button-secondary" @click="withGlobalLoading(openCode, item)">{{ t('admin.copy.6532e77d') }}</button></div></article>
          <p v-if="!loading && items.length === 0" class="jc-note empty-state">{{ t('admin.copy.704b95cd') }}</p>
        </div><div class="jc-pager"><button type="button" class="jc-secondary button button-secondary" :disabled="loading || offset === 0" @click="withGlobalLoading(changePage, -1)">{{ t('admin.copy.dbe7a5d7') }}</button><span>{{ total === "0" ? 0 : offset + 1 }}–{{ Math.min(offset + pageSize, Number(total)) }} / {{ total }}</span><button type="button" class="jc-secondary button button-secondary" :disabled="loading || offset + pageSize >= Number(total)" @click="withGlobalLoading(changePage, 1)">{{ t('admin.copy.2e9c9e7a') }}</button></div>
      </section>
    <DetailModal v-if="selected || createOpen && kind === 'agent'" :open="!pendingOpen" :title="selected ? t(selected.kind === 'agent' ? 'admin.join.editAgent' : 'admin.join.editReferral') : t(form.kind === 'agent' ? 'admin.join.createAgent' : 'admin.join.createReferral')" :close-label="t('association.close')" @close="selected = null; createOpen = false">
      <p v-if="error" class="jc-message is-error" role="alert">{{ localizedMessage(error) }}</p>
    <section v-if="canWrite && (selected || createOpen && kind === 'agent')" class="jc-card"><p v-if="!selected">{{ t('admin.copy.b8377c86') }}</p>

        <form novalidate v-inline-validation class="jc-form" @submit.prevent="withGlobalLoading(prepareWrite, selected ? 'update' : 'create')">
          <template v-if="selected"><label>{{ t('admin.copy.25eb928d') }}<input :value="selected.code" readonly></label><label>{{ t('admin.copy.a765b3ca') }}<select v-model="form.status"><option value="active">{{ t('admin.copy.98d24810') }}</option><option value="disabled">{{ t('admin.copy.5e3326db') }}</option></select></label></template>
          <label>{{ t('admin.copy.56078f47') }}<input v-model.trim="form.starts_at" autocomplete="off" placeholder="2026-10-06T04:00:00.123456Z"></label><label>{{ t('admin.copy.5511b602') }}<input v-model.trim="form.expires_at" autocomplete="off" placeholder="2026-11-06T04:00:00Z"></label>
        <p v-if="!validTime(form.starts_at) || !validTime(form.expires_at)" class="jc-field-error">{{ t('admin.copy.6c349f0f') }}</p><p v-else-if="dateInput(form.starts_at) && dateInput(form.expires_at) && !validJoinCodeDateRange(dateInput(form.starts_at), dateInput(form.expires_at))" class="jc-field-error">{{ t('admin.copy.eedcdbfe') }}</p>
          <button class="jc-primary button button-primary" type="submit" :disabled="writing || !formValid">{{ selected ? t('admin.copy.ea2d7203') : t('admin.copy.26bad1f6') }}</button>
        </form>
      </section>
      <section v-if="selected && canView" class="jc-card"><div class="jc-card-head"><div><h3>{{ t('admin.copy.da7055cb') }}</h3></div><button v-if="historyOpen" v-refresh-button="t('admin.copy.4f5df8c1')" class="jc-secondary button button-secondary" type="button" @click="withGlobalLoading(loadHistory)">{{ t('admin.copy.4f5df8c1') }}</button><button v-else class="jc-secondary button button-secondary" type="button" @click="withGlobalLoading(loadHistory)">{{ t('admin.copy.e8ac3e26') }}</button></div><div v-if="historyOpen" class="jc-history"><article v-for="item in historyItems" :key="item.id" class="jc-history-row"><strong> {{ item.status === "active" ? t('admin.copy.98d24810') : t('admin.copy.5e3326db') }}</strong><span>{{ displayDate(item.created_at) }} · {{ t('admin.copy.aa3420fe') }} <AssociationName v-if="item.actor_id" :record="item" kind="actor" :id="item.actor_id" :locale="associationLocale" /><span v-else>{{ t('admin.joinCode.systemActor') }}</span></span><span>{{ historyRange(item) }}</span><span v-if="item.actor_id === null || item.reason">{{ t('admin.copy.2809a1e6') }}{{ item.actor_id === null ? t('admin.joinCode.automaticAllocation') : item.reason }}</span><span>{{ t('admin.copy.55378394') }} </span></article><p v-if="!historyItems.length" class="jc-note empty-state">{{ t('admin.copy.2876be3f') }}</p><div class="jc-pager"><button class="jc-secondary button button-secondary" type="button" :disabled="historyOffset === 0" @click="withGlobalLoading(async () => { historyOffset = Math.max(0, historyOffset - pageSize); await loadHistory() })">{{ t('admin.copy.dbe7a5d7') }}</button><span>{{ historyTotal }} {{ t('admin.copy.44d96226') }}</span><button class="jc-secondary button button-secondary" type="button" :disabled="historyOffset + pageSize >= Number(historyTotal)" @click="withGlobalLoading(async () => { historyOffset += pageSize; await loadHistory() })">{{ t('admin.copy.2e9c9e7a') }}</button></div></div></section>
    </DetailModal>
    </template>
    <button v-if="pending" class="jc-secondary button button-secondary" type="button" @click="pendingOpen = true">{{ t('admin.settings.reviewPending') }}</button>
    <DetailModal v-if="pending && pendingOpen" :title="t('admin.copy.3c44046e')" :close-label="t('association.close')" @close="!writing && (pendingOpen = false)">
      <p v-if="error" class="jc-message is-error" role="alert">{{ localizedMessage(error) }}</p>
    <section v-if="pending && pending.accountId === account.id && pending.brandId === brandId" class="jc-card jc-review"><div class="jc-card-head"><div><h3>{{ t('admin.copy.3c44046e') }}</h3><p>{{ pendingTitle(pending) }}</p></div><span class="jc-pending">{{ t('admin.copy.e3708260') }}</span></div><p class="jc-note">{{ t('admin.copy.851c6121') }} {{ account.username }} · {{ t('admin.copy.9c32661d') }} <AssociationName kind="brand" :id="pending.brandId" :locale="associationLocale" />。{{ t('admin.copy.d710df42') }}</p><dl v-if="pending.operation === 'create'" class="jc-identity"><div><dt>{{ t('admin.copy.214be851') }}</dt><dd>{{ pending.identity.kind === "agent" ? t('admin.copy.ae9fa0bf') : t('admin.copy.d245e2d6') }}</dd></div><div><dt>{{ t('admin.copy.f28a26fa') }}</dt><dd><AssociationName :record="pending.identity" :kind="pending.identity.kind === 'agent' ? 'agent' : 'member'" :id="pending.identity.agent_id ?? pending.identity.owner_member_id" :locale="associationLocale" /></dd></div><div><dt>{{ t('admin.copy.b863c1ed') }}</dt><dd><template v-if="pending.identity.agent_id"><AssociationName :record="pending.identity" kind="agent" :id="pending.identity.agent_id" :locale="associationLocale" /></template><span v-else>{{ t('admin.copy.c2b41b33') }}</span></dd></div></dl><dl v-else class="jc-identity"><div><dt>{{ t('admin.copy.2b9c4937') }}</dt><dd>{{ pending.identity.code }}</dd></div><div><dt>{{ t('admin.copy.dcb3af52') }}</dt><dd><AssociationName :record="pending.identity" :kind="pending.identity.kind === 'agent' ? 'agent' : 'member'" :id="pending.identity.agent_id ?? pending.identity.owner_member_id" :locale="associationLocale" /></dd></div><div><dt>{{ t('admin.copy.0992c6f7') }}</dt><dd><template v-if="pending.identity.agent_id"><AssociationName :record="pending.identity" kind="agent" :id="pending.identity.agent_id" :locale="associationLocale" /></template><span v-else>{{ t('admin.copy.c2b41b33') }}</span></dd></div><div><dt>{{ t('admin.copy.214be851') }}</dt><dd>{{ t(pending.identity.kind === 'agent' ? 'admin.copy.ae9fa0bf' : 'admin.copy.d245e2d6') }}</dd></div></dl><BusinessDetails class="jc-json" :value="{ status: 'status' in pending.body ? pending.body.status : undefined, starts_at: pending.body.starts_at, expires_at: pending.body.expires_at, reason: pending.body.reason }" :locale="associationLocale" /><label class="jc-check"><input v-model="review" type="checkbox" data-testid="join-code-frozen-confirmation" :disabled="!canWrite || loading || writing">{{ t('admin.copy.9fac2a31') }}</label><div class="jc-actions"><button v-write-guard="writing" class="jc-primary button button-primary" type="button" :disabled="!canWrite || !review || loading || writing" @click="withGlobalLoading(confirmWrite)">{{ writing ? t('admin.copy.46511dc0') : t('admin.copy.a1a8013d') }}</button><button v-write-guard="writing" v-if="!writing" class="jc-secondary button button-secondary" type="button" data-testid="join-code-retry" :disabled="!canWrite || !review || loading" @click="withGlobalLoading(retryFrozen)">{{ t('admin.copy.3f6d9e01') }}</button><button class="jc-secondary button button-secondary" type="button" :disabled="writing || loading" @click="review = false; pendingOpen = false">{{ t('admin.copy.92358b01') }}</button></div></section>
    </DetailModal>
  </section>
</template>

<style scoped>
.join-codes{width:100%;min-width:0;color:var(--text)}.jc-head,.jc-card-head{display:flex;justify-content:space-between;align-items:flex-start;gap:14px}.jc-head{margin:4px 0 16px}.jc-head h2{font-size:20px;margin:3px 0 5px}.jc-head p,.jc-card-head p{margin:0;color:var(--muted);font-size:13px;line-height:1.55;overflow-wrap:anywhere}.jc-eyebrow{font-size:12px!important;letter-spacing:1.1px;color:#8991a2!important;font-weight:700}.jc-brand,.jc-pending{border-radius:20px;background:#f1f3f8;padding:8px 12px;color:var(--muted);font-size:13px;white-space:nowrap}.jc-note,.jc-message{border-radius:8px;background:#f4f6fc;color:var(--muted);padding:11px 13px;font-size:13px;line-height:1.6;overflow-wrap:anywhere;margin:12px 0}.jc-message.is-error{background:#fff1f0;color:#a12d25}.jc-message.is-ok{background:#eef8f3;color:#287554}.jc-card{background:var(--surface);border:1px solid var(--line);border-radius:11px;padding:16px;min-width:0;box-shadow:0 2px 8px #1e2a5008;margin:13px 0}.jc-card h3{font-size:16px;margin:0 0 12px}.jc-card-head{margin-bottom:14px}.jc-card-head h3{margin:0 0 4px}.jc-filters,.jc-form{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:13px}.jc-filters{grid-template-columns:minmax(150px,.7fr) minmax(240px,1.3fr) auto;align-items:end;margin:12px 0 16px}.jc-filters label,.jc-form label{display:grid;gap:6px;font-size:13px;font-weight:600;min-width:0}.jc-filters input,.jc-filters select,.jc-form input:not([type=checkbox]),.jc-form select,.jc-form textarea{box-sizing:border-box;width:100%;min-width:0;border:1px solid var(--line);border-radius:7px;background:var(--surface);padding:10px 11px;color:var(--text);font:inherit}.jc-form textarea{resize:vertical}.jc-wide,.jc-field-error{grid-column:1/-1}.jc-list{display:grid;gap:9px}.jc-row{display:flex;justify-content:space-between;gap:12px;border:1px solid var(--line);border-radius:9px;padding:12px;min-width:0}.jc-row-main{display:grid;gap:5px;min-width:0}.jc-row-main strong,.jc-row-main small,.jc-row-main span{overflow-wrap:anywhere}.jc-row-main span,.jc-row-main small{font-size:13px;color:var(--muted)}.jc-row code,.jc-card code{overflow-wrap:anywhere;word-break:break-word}.jc-pager{display:flex;justify-content:center;align-items:center;gap:12px;margin-top:13px;flex-wrap:wrap}.jc-pager span{font-size:13px;color:var(--muted);font-variant-numeric:tabular-nums}.jc-primary,.jc-secondary{border-radius:7px;padding:9px 13px;font-size:14px;min-height:44px}.jc-primary{border:1px solid var(--primary);background:var(--primary);color:var(--surface)}.jc-secondary{border:1px solid var(--line);background:var(--surface);color:#41495a}.jc-actions{display:flex;gap:8px;flex-wrap:wrap;align-items:center}.jc-field-error{color:#a12d25;font-size:13px;margin:0}.jc-history{display:grid;gap:9px}.jc-history-row{display:grid;gap:6px;background:#f9fafc;border:1px solid var(--line);border-radius:8px;padding:11px;min-width:0;font-size:13px;overflow-wrap:anywhere}.jc-history-row span{color:var(--muted)}.jc-identity{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:9px;margin:12px 0}.jc-identity div{border:1px solid var(--line);border-radius:8px;padding:10px;min-width:0}.jc-identity dt{font-size:12px;color:var(--muted)}.jc-identity dd{font-size:13px;font-weight:600;overflow-wrap:anywhere;margin:5px 0 0}.jc-review{border-color:#cdd3ff}.jc-json{white-space:pre-wrap;overflow-wrap:anywhere;word-break:break-word;max-height:320px;overflow:auto;background:var(--canvas);border-radius:8px;padding:12px;font-size:13px;line-height:1.5}.jc-meta{font-size:13px;color:var(--muted);overflow-wrap:anywhere}.jc-check{display:flex;align-items:center;gap:9px;min-height:44px;font-size:14px;margin:12px 0}.jc-check input{width:18px;height:18px;flex:none}
@media(max-width:640px){.jc-head,.jc-card-head,.jc-row{flex-direction:column}.jc-form,.jc-filters{grid-template-columns:minmax(0,1fr)}.jc-brand{white-space:normal}.jc-actions{width:100%}.jc-actions button{flex:1}.jc-card{padding:14px}}
</style>
