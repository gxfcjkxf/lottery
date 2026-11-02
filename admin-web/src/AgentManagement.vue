<script setup lang="ts">
import SelectControl from '@lottery/shared/SelectControl.vue'
import { useOperationToast } from "@lottery/shared";
import ManagementList from "@lottery/shared/ManagementList.vue"
import ConsoleTabs from "@lottery/shared/ConsoleTabs.vue"
import AgentRatioField from "@lottery/shared/AgentRatioField.vue"
import AccountCreateFields from "@lottery/shared/AccountCreateFields.vue"
import DirectorySearch from "@lottery/shared/DirectorySearch.vue"
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName, associationLabel } from "@lottery/shared";

import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
const displayDate = (value: string | null | undefined) => formatDisplayDateTime(value, displayLocale.value);
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import { vInlineValidation } from '@lottery/shared'
import { joinCodePermissions } from "./join-codes-api";
import JoinCodeManagement from "./JoinCodeManagement.vue";
import MemberActionsMenu from '@lottery/shared/MemberActionsMenu.vue'
import TablePagination from '@lottery/shared/TablePagination.vue'
import DetailModal from "@lottery/shared/DetailModal.vue"
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { agentsPermissions, createAgentsApi, type ModeChangeBody, type ModeChangeReceipt, type AgentDirectoryNode, type AgentNode, type AgentPolicy, type CreateAgentBody, type NodeConfig, type PolicyConfig, type SaveAgentPolicyBody, type UpdateAgentBody } from "./agents-api";
import { allowedChildModes, classifyAgentFailure, clearAllPendingAgentWrites, clearPendingAgentWrite, freezeAgentBody, getPendingAgentWrite, parentModeVerified, setPendingAgentWrite, type AgentWriteOperation, type AgentWriteBody, type PendingAgentWrite } from "./agents-state";
import { useAdminI18n } from "./i18n";

import { agentPercentageToRatio, agentRatioToPercentage, validAgentPercentage } from "./agent-percentage";

type Mode = "loss" | "turnover";
const props = withDefaults(defineProps<{ account: AdminAccount; brandId: string; mode?: 'directory' | 'settings' }>(), { mode: 'directory' });
const viewMode = ref<'list' | 'hierarchy'>('list');
const { t, message, display } = useAdminI18n();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createAgentsApi();
const agentPassword = ref("");
const permissionSet = computed(() => agentsPermissions(props.account, props.brandId));
const canViewPolicy = computed(() => permissionSet.value.policyView);
const canViewAgents = computed(() => permissionSet.value.view);
const canWritePolicy = computed(() => permissionSet.value.policyWrite);
const canWriteAgents = computed(() => permissionSet.value.write);
const canView = computed(() => props.mode === 'settings' ? canViewPolicy.value : canViewAgents.value);
const permissionSignature = computed(() => JSON.stringify({
  accountId: props.account.id,
  brandId: props.brandId,
  superAdmin: props.account.super_admin,
  brandIds: [...props.account.brand_ids].sort(),
  brandPermissions: [...(props.account.permissions_by_brand?.[props.brandId] ?? [])].sort(),
  platformPermissions: [...(props.account.platform_permissions ?? [])].sort(),
}));
const policy = ref<AgentPolicy | null>(null);
const policyDraft = ref<PolicyConfig>({ enabled: true, max_depth: 5, loss_ratio_cap: "0", turnover_ratio_cap: "0", mode: "loss", cycle: "monthly" });
const policyLossPercentage = ref("0"), policyTurnoverPercentage = ref("0");
const nodes = ref<AgentNode[]>([]);
const directoryNodes = ref<AgentDirectoryNode[]>([]);
const search = ref(''), query = ref('');
const onlineFilter = ref<'' | 'true' | 'false'>(''), onlineQuery = ref<'' | 'true' | 'false'>('');
async function searchAgents() { await loadTree(++generation, props.brandId, props.account.id, 0, search.value.trim(), onlineFilter.value); }
async function clearSearch() { search.value = ''; onlineFilter.value = ''; await searchAgents(); }
const hasNext = computed(() => BigInt(offset.value) + BigInt(nodes.value.length) < BigInt(total.value));
const total = ref("0"), listLoaded = ref(false);
const offset = ref(0);
const parentId = ref<string | null>(null);
const parentNode = ref<AgentNode | null>(null);
const selected = ref<AgentNode | null>(null);
const detailOpen = ref(false);
const modeLine = ref<AgentNode[]>([]);
const modeRatios = ref<Record<string,string>>({});
const modeScopeLoading = ref(false);
let modeScopeGeneration = 0;
const selectedParentMode = ref<Mode | null>(null);
const selectedParentReady = ref(false);
const form = ref({ agent_account_id: "", loss_ratio: "", turnover_ratio: "", mode: "" as "" | Mode, status: "active" as "active" | "disabled", can_create_children: false });
const agentUsername = ref('');
const loading = ref(false);
const writing = ref(false);
const error = ref<string | import("@lottery/shared").LocalizedMessage>("");
const notice = ref<string | import("@lottery/shared").LocalizedMessage>("");
const createOpen = ref(false), policyEditorOpen = ref(false);
function editPolicy() { if (!policy.value || !canWritePolicy.value) return; policyDraft.value = { ...policy.value.config }; policyLossPercentage.value = agentRatioToPercentage(policy.value.config.loss_ratio_cap); policyTurnoverPercentage.value = agentRatioToPercentage(policy.value.config.turnover_ratio_cap); policyEditorOpen.value = true; }
const pending = ref<PendingAgentWrite | null>(null);
let generation = 0;
let writeGeneration = 0;
let disposed = false;
let activeAccountId = props.account.id;
const pageSize = 20;
const policyRatioValid = computed(() => validAgentPercentage(policyLossPercentage.value) && validAgentPercentage(policyTurnoverPercentage.value));
const nodeRatioValid = computed(() => Boolean(draftMode.value) && validAgentPercentage(draftRatio.value));
const nodeParentMode = computed<Mode | null | undefined>(() => selected.value ? (selected.value.parent_id ? (selectedParentReady.value ? selectedParentMode.value ?? undefined : undefined) : policy.value?.config.mode) : (parentNode.value ? parentNode.value.effective_mode : parentId.value ? undefined : null));
const isFirstLevel = computed(() => selected.value ? selected.value.parent_id === null : !parentId.value);
const draftMode = computed(() => form.value.mode || (isFirstLevel.value ? null : nodeParentMode.value));
const draftRatio = computed(() => draftMode.value === 'loss' ? form.value.loss_ratio : draftMode.value === 'turnover' ? form.value.turnover_ratio : '');
const createRatioLimit = computed(() => parentId.value ? (parentNode.value ? agentRatioToPercentage(parentNode.value.config.ratio) : null) : (policy.value ? agentRatioToPercentage(draftMode.value === 'loss' ? policy.value.config.loss_ratio_cap : policy.value.config.turnover_ratio_cap) : null));
const createRatioExceeded = computed(() => validAgentPercentage(draftRatio.value) && createRatioLimit.value !== null && Number(draftRatio.value) > Number(createRatioLimit.value));
const allowedModes = computed(() => selected.value?.parent_id === null || (!selected.value && !parentId.value) ? ['loss', 'turnover'] as Mode[] : allowedChildModes(nodeParentMode.value));
const changingMode = computed(() => Boolean(selected.value && !selected.value.parent_id && form.value.mode && form.value.mode !== selected.value.effective_mode));
const modeLineReady = computed(() => !modeScopeLoading.value && modeLine.value.length > 0 && modeLine.value.every(node => node.id === selected.value?.id || validAgentPercentage(modeRatios.value[node.id] ?? '')));
watch(changingMode, async change => {
  const ticket = ++modeScopeGeneration;
  modeLine.value = []; modeRatios.value = {}; modeScopeLoading.value = false;
  if (!change || !selected.value) return;
  const id = selected.value.id, brand = props.brandId, accountId = props.account.id;
  modeScopeLoading.value = true;
  try {
    const scope = await api.modeScope(brand,id);
    if (ticket !== modeScopeGeneration || brand !== props.brandId || accountId !== props.account.id || selected.value?.id !== id) return;
    if (scope.policy_version !== policy.value?.version || scope.items.some(node => node.pending_change) || scope.items.find(node => node.id === id)?.version !== selected.value.version) throw new AdminApiError('Mode scope changed',409,'AGENT_VERSION_CONFLICT');
    modeLine.value = scope.items;
  } catch(cause) { if(ticket === modeScopeGeneration) handleError(cause); }
  finally { if(ticket === modeScopeGeneration) modeScopeLoading.value = false; }
});
const sameScope = (g: number, brand: string, account: string) => g === generation && brand === props.brandId && account === props.account.id;
function currentWrite(ticket: number, accountId: string, brandId: string, permissions: string, operation: AgentWriteOperation) {
  return !disposed && ticket === writeGeneration && props.account.id === accountId && props.brandId === brandId &&
    permissionSignature.value === permissions && (operation === "policy" ? canWritePolicy.value : canWriteAgents.value);
}

function restoreIntent() { pending.value = getPendingAgentWrite({ accountId: props.account.id, brandId: props.brandId }); }
function handleError(cause: unknown) {
  if (cause instanceof AdminApiError && cause.status === 401) { clearAllPendingAgentWrites(); pending.value = null; emit("session-invalid"); }
  error.value = publicErrorMessage(cause);
}
async function loadPolicy(g = ++generation, brand = props.brandId, account = props.account.id) {
  if (!canViewPolicy.value || !brand) { policy.value = null; return; }
  try {
    const result = await api.policy(brand);
    if (!sameScope(g, brand, account)) return;
    if (result.brand_id !== brand) throw new AdminApiError('', 502, 'INVALID_RESPONSE');
    policy.value = result; policyDraft.value = { ...result.config }; policyLossPercentage.value = agentRatioToPercentage(result.config.loss_ratio_cap); policyTurnoverPercentage.value = agentRatioToPercentage(result.config.turnover_ratio_cap);
  } catch (cause) { if (sameScope(g, brand, account)) handleError(cause); }
}
async function loadTree(g = ++generation, brand = props.brandId, account = props.account.id, requestedOffset = offset.value, requestedQuery = query.value, requestedOnline = onlineQuery.value, requestedView = viewMode.value, requestedParent = parentId.value) {
  if (!canViewAgents.value || !brand) { nodes.value = []; return; }
  loading.value = true; error.value = "";
  try {
    const [result, parent] = await Promise.all([requestedView === 'list' ? api.list(brand, pageSize, requestedOffset, { query: requestedQuery, online: requestedOnline }) : api.tree(brand, requestedParent, pageSize, requestedOffset), requestedParent ? api.node(brand, requestedParent) : Promise.resolve(null)]);
    if (!sameScope(g, brand, account)) return;
    if (result.brand_id !== brand || (requestedView === 'hierarchy' && (!('parent_id' in result) || result.parent_id !== requestedParent))) throw new AdminApiError('', 502, 'INVALID_RESPONSE');
    listLoaded.value = true; viewMode.value = requestedView; parentId.value = requestedParent; offset.value = requestedOffset; query.value = requestedQuery; onlineQuery.value = requestedOnline;
    nodes.value = result.items; directoryNodes.value = !('parent_id' in result) ? result.items : []; total.value = result.total_count; parentNode.value = parent; restoreIntent();
  } catch (cause) { if (sameScope(g, brand, account)) handleError(cause); }
  finally { if (sameScope(g, brand, account)) loading.value = false; }
}
async function loadAll() { const g = ++generation; const b = props.brandId; const a = props.account.id; await Promise.all([loadPolicy(g, b, a), ...(props.mode === 'settings' ? [] : [loadTree(g, b, a)])]); }
async function selectNode(n: AgentNode, openDialog = true) {
  const b = props.brandId, a = props.account.id, g = ++generation;
  error.value = "";
  try {
    const detail = await api.node(b, n.id);
    if (!sameScope(g, b, a)) return;
    if (detail.id !== n.id || detail.brand_id !== b) throw new AdminApiError('', 502, 'INVALID_RESPONSE');
    selected.value = detail; detailOpen.value = openDialog;
    form.value = { agent_account_id: detail.agent_account_id, loss_ratio: detail.effective_mode === "loss" ? agentRatioToPercentage(detail.config.ratio) : "", turnover_ratio: detail.effective_mode === "turnover" ? agentRatioToPercentage(detail.config.ratio) : "", mode: detail.parent_id ? detail.config.mode ?? "" : detail.effective_mode, status: detail.config.status, can_create_children: detail.config.can_create_children };
    selectedParentReady.value = !detail.parent_id; selectedParentMode.value = null;
    restoreIntent();
    if (detail.parent_id) {
      try {
        const parent = await api.node(b, detail.parent_id);
        if (!sameScope(g, b, a) || selected.value?.id !== detail.id) return;
        if (parent.brand_id !== b || parent.id !== detail.parent_id) throw new AdminApiError('', 502, 'INVALID_RESPONSE');
        selectedParentMode.value = parent.effective_mode; selectedParentReady.value = true;
      } catch (cause) {
        if (!sameScope(g, b, a) || selected.value?.id !== detail.id) return;
        selectedParentMode.value = null; selectedParentReady.value = false;
        handleError(cause);
      }
    }
  } catch (cause) { if (sameScope(g, b, a)) handleError(cause); }
}
function closeDetail() { modeScopeGeneration++; modeScopeLoading.value = false; modeLine.value = []; modeRatios.value = {}; createOpen.value = false; detailOpen.value = false; selected.value = null; selectedParentMode.value = null; selectedParentReady.value = false; form.value = { agent_account_id: "", loss_ratio: "", turnover_ratio: "", mode: "", status: "active", can_create_children: false }; restoreIntent(); }
function startCreate() { if (writing.value || pending.value) return; closeDetail(); agentUsername.value=""; agentPassword.value=""; error.value = ""; createOpen.value = true; }
async function switchView(mode: 'list' | 'hierarchy') {
  search.value = ''; onlineFilter.value = '';
  closeDetail(); await loadTree(++generation, props.brandId, props.account.id, 0, '', '', mode, null);
}
async function openChildren(n: AgentNode) { closeDetail(); await loadTree(++generation, props.brandId, props.account.id, 0, query.value, onlineQuery.value, 'hierarchy', n.id); }
async function goBack() {
  const parent = parentNode.value?.parent_id ?? null;
  closeDetail(); await loadTree(++generation, props.brandId, props.account.id, 0, query.value, onlineQuery.value, 'hierarchy', parent);
}
function freeze(operation: AgentWriteOperation, resourceId: string | null, body: AgentWriteBody): boolean {
  const scope = { accountId: props.account.id, brandId: props.brandId };
  const old = getPendingAgentWrite(scope);
  if (old && (old.operation !== operation || old.resourceId !== resourceId || JSON.stringify(old.body) !== JSON.stringify(body))) {
    pending.value = old;
    error.value = message('admin.copy.48a6af2c');
    return false;
  }
  const intent: PendingAgentWrite = old ?? {
    ...scope,
    operation,
    resourceId,
    body: freezeAgentBody(body),
    key: createIdempotencyKey(),
  };
  setPendingAgentWrite(scope, intent);
  pending.value = getPendingAgentWrite(scope);
  return true;
}
async function preparePolicy() {
  if (writing.value || pending.value) return;
  if (!policy.value || !canWritePolicy.value || !policyRatioValid.value || !Number.isInteger(policyDraft.value.max_depth) || policyDraft.value.max_depth < 1 || policyDraft.value.max_depth > 32) return;
  const body: SaveAgentPolicyBody = { version: policy.value.version, config: { ...policyDraft.value, enabled: true, loss_ratio_cap: agentPercentageToRatio(policyLossPercentage.value), turnover_ratio_cap: agentPercentageToRatio(policyTurnoverPercentage.value) } };
  if (freeze("policy", null, body)) await confirmWrite();
}
async function prepareNode() {
  if (writing.value || pending.value) return;
  if (!policy.value || !canWriteAgents.value || (!selected.value && createRatioExceeded.value) || !nodeRatioValid.value || (isFirstLevel.value && !form.value.mode) || selected.value?.pending_change) return;
  if (changingMode.value && selected.value && form.value.mode) {
    if (!modeLineReady.value) return;
    const body: ModeChangeBody = { policy_version: policy.value.version, mode: form.value.mode,
      items: modeLine.value.map(node => ({ agent_id: node.id, version: node.version, ratio: agentPercentageToRatio(node.id === selected.value!.id ? draftRatio.value : modeRatios.value[node.id]!) })) };
    if (freeze('mode-change', selected.value.id, body)) await confirmWrite();
    return;
  }
  const parentIdForWrite = selected.value?.parent_id ?? parentNode.value?.id ?? null;
  if (!parentModeVerified(parentIdForWrite, selected.value ? selectedParentReady.value : Boolean(parentNode.value) || !parentId.value)) { error.value = message('admin.copy.c202b758'); return; }
  if (form.value.mode && !allowedModes.value.includes(form.value.mode)) { error.value = message('admin.copy.42f30b77'); return; }
  const config: NodeConfig = { ratio: agentPercentageToRatio(draftRatio.value), mode: form.value.mode || null, status: form.value.status, can_create_children: form.value.can_create_children };
  if (selected.value) {
    const n = selected.value;
    const pVersion = n.parent_id ? (parentNode.value?.id === n.parent_id ? parentNode.value.version : n.parent_version) : null;
    const body: UpdateAgentBody = { version: n.version, policy_version: policy.value.version, parent_version: pVersion, config };
    if (freeze("update", n.id, body)) await confirmWrite();
  } else {
    if (parentId.value && !parentNode.value) { error.value = message('admin.copy.421b0a6c'); return; }
    const username = agentUsername.value.trim().toLowerCase();
    if (!/^[a-z][a-z0-9_]{2,31}$/.test(username) || agentPassword.value.length < 6 || agentPassword.value.length > 30) { error.value = publicErrorMessage({status:400}); return; }
    const body: CreateAgentBody = { policy_version: policy.value.version, username, password: agentPassword.value, parent_id: parentNode.value?.id ?? null, parent_version: parentNode.value?.version ?? null, config };
    error.value = "";
    if (freeze("create", null, body)) await confirmWrite();
  }
}
async function confirmWrite() {
  const i = pending.value;
  if (!i || writing.value || i.accountId !== props.account.id || i.brandId !== props.brandId || (i.operation === "policy" ? !canWritePolicy.value : !canWriteAgents.value)) return;
  const ticket = ++writeGeneration;
  const accountId = i.accountId;
  const brandId = i.brandId;
  const permissions = permissionSignature.value;
  const isCurrentWrite = () => currentWrite(ticket, accountId, brandId, permissions, i.operation);
  const sharedScope = { accountId, brandId };
  writing.value = true; error.value = ""; notice.value = "";
  try {
    let receipt: AgentPolicy | AgentNode | ModeChangeReceipt;
    if (i.operation === "policy") receipt = await api.savePolicy(i.brandId, i.body as SaveAgentPolicyBody, i.key);
    else if (i.operation === "create") receipt = await api.createNode(i.brandId, i.body as CreateAgentBody, i.key);
    else if (i.operation === 'mode-change') receipt = await api.scheduleMode(i.brandId,i.resourceId!,i.body as ModeChangeBody,i.key);
    else receipt = await api.updateNode(i.brandId, i.resourceId!, i.body as UpdateAgentBody, i.key);
    // A validated SDK receipt acknowledges only this frozen key; it cannot clear a replacement intent.
    clearPendingAgentWrite(sharedScope, i.key);
    if (!isCurrentWrite()) return;
    pending.value = getPendingAgentWrite(sharedScope);
    detailOpen.value = false;
    notice.value = i.operation === "create" ? message('admin.agents.created') : message('admin.copy.6a656cc2');
    if (i.operation === "create") { agentPassword.value = ""; createOpen.value = false; }
    if (i.operation === "policy") { policyEditorOpen.value = false; await loadPolicy(); }
    else if (i.operation === "create") await loadTree();
    else if (i.operation === 'mode-change') { const root = await api.node(i.brandId,i.resourceId!); await selectNode(root, false); if (!isCurrentWrite()) return; await loadTree(); }
    else { await selectNode(receipt as AgentNode, false); if (!isCurrentWrite()) return; await loadTree(); }
    if (isCurrentWrite()) notice.value = i.operation === "create" ? message('admin.agents.created') : message('admin.copy.06415e47');
  } catch (cause) {
    if (!isCurrentWrite()) return;
    const status = cause instanceof AdminApiError ? cause.status : undefined;
    if (status === 401) {
      writing.value = false;
      clearAllPendingAgentWrites(); pending.value = null; emit("session-invalid");
      return;
    }
    if (classifyAgentFailure(status) === "uncertain") {
      pending.value = getPendingAgentWrite(sharedScope);
      notice.value = i.operation === "create" ? message('admin.agents.createUnknown') : message('admin.copy.bad148c4');
      error.value = publicErrorMessage(cause);
    } else {
      clearPendingAgentWrite(sharedScope, i.key);
      pending.value = getPendingAgentWrite(sharedScope);
      error.value = publicErrorMessage(cause);
    }
  } finally { if (isCurrentWrite()) writing.value = false; }
}
async function retryFrozen() { await confirmWrite(); }
function modeLabel(value: string | null | undefined) { return value === "loss" ? t('admin.copy.a13e79ef') : value === "turnover" ? t('admin.copy.89fd862d') : t('admin.copy.b8bd227c'); }

async function changePage(d: number) { await loadTree(++generation, props.brandId, props.account.id, Math.max(0, offset.value + d * pageSize)); }
onMounted(() => { restoreIntent(); void loadAll(); });
onBeforeUnmount(() => { disposed = true; generation++; writeGeneration++; modeScopeGeneration++; });
watch(() => [props.brandId, props.account.id, permissionSignature.value, props.mode] as const, () => {
  if (activeAccountId !== props.account.id) {
    clearAllPendingAgentWrites();
    activeAccountId = props.account.id;
  }
  modeScopeGeneration++; modeScopeLoading.value = false; modeLine.value = []; modeRatios.value = {};
  viewMode.value = 'list';
  search.value = ''; query.value = ''; onlineFilter.value = ''; onlineQuery.value = ''; directoryNodes.value = []; listLoaded.value = false;
  generation++; writeGeneration++; policy.value = null; policyEditorOpen.value = false; nodes.value = []; createOpen.value = false; selected.value = null; detailOpen.value = false; parentId.value = null; parentNode.value = null; offset.value = 0; error.value = ""; notice.value = ""; loading.value = false; writing.value = false; restoreIntent(); if (canView.value) void loadAll();
}, { flush: "sync" });

useOperationToast(() => error.value, value => display(value), 'error');
useOperationToast(() => notice.value, value => display(value), 'info');
</script>

<template>
  <section class="agent-management" :class="{ 'brand-settings-panel': mode === 'settings' }">
    <header v-if="mode !== 'settings' || !policy" class="console-page-heading am-head"><div><h2 class="console-page-title">{{ t(mode === 'settings' ? 'admin.agents.rules' : 'admin.nav.agentList') }}</h2></div></header>

    <p v-if="!canView" class="am-note">{{ t('admin.copy.07a2166b') }}</p>
    <template v-else>
      <section v-if="mode === 'settings' && policy" class="agent-settings-policy"><div class="am-card-head"><div><h2>{{ t('admin.agents.rules') }}</h2></div><button v-if="error" v-refresh-button="t('admin.copy.1935ecf7')" type="button" class="am-secondary button button-secondary" @click="withGlobalLoading(loadAll)">{{ t('admin.copy.1935ecf7') }}</button></div>
        <dl class="am-facts"><div><dt>{{ t('admin.copy.eae52028') }}</dt><dd>{{ policy.config.max_depth }}</dd></div><div><dt>{{ t('admin.agents.lossRatioCap') }}</dt><dd>{{ agentRatioToPercentage(policy.config.loss_ratio_cap) }}%</dd></div><div><dt>{{ t('admin.agents.turnoverRatioCap') }}</dt><dd>{{ agentRatioToPercentage(policy.config.turnover_ratio_cap) }}%</dd></div><div><dt>{{ t('admin.copy.60850269') }}</dt><dd>{{ policy.config.mode === "loss" ? t('admin.copy.a13e79ef') : t('admin.copy.89fd862d') }}</dd></div></dl>
        <div class="am-actions ui-form-actions"><button v-if="mode === 'settings' && canWritePolicy && policy" class="button button-secondary" type="button" :disabled="writing || !!pending" @click="editPolicy">{{ t('admin.copy.24dfbf6c') }}</button></div>
      </section>
      <DetailModal class="agent-dialog ui-form-dialog" v-if="mode === 'settings' && canWritePolicy && policy && policyEditorOpen" :open="!pending" :title="t('admin.nav.agentSettings')" :close-label="t('association.close')" :close-disabled="writing" @close="!writing && (policyEditorOpen = false)">

      <section v-if="canWritePolicy && policy" class="am-card"><form v-write-guard="writing" novalidate v-inline-validation class="am-form ui-form" @submit.prevent="withGlobalLoading(preparePolicy)">
        <label>{{ t('admin.copy.96b469dc') }}<input v-model.number="policyDraft.max_depth" type="number" min="1" max="32" step="1" :aria-label="t('admin.copy.96b469dc')"></label>
        <label>{{ t('admin.agents.lossRatioCap') }}<span class="am-ratio-input"><input v-model.trim="policyLossPercentage" inputmode="decimal" autocomplete="off" :aria-label="t('admin.agents.lossRatioCap')"><span aria-hidden="true">%</span></span></label>
        <label>{{ t('admin.agents.turnoverRatioCap') }}<span class="am-ratio-input"><input v-model.trim="policyTurnoverPercentage" inputmode="decimal" autocomplete="off" :aria-label="t('admin.agents.turnoverRatioCap')"><span aria-hidden="true">%</span></span></label>
        <label>{{ t('admin.copy.60850269') }}<SelectControl v-model="policyDraft.mode" :label="t('admin.copy.60850269')" :options="[{ value: 'loss', label: (String(t('admin.copy.a13e79ef'))).trim() }, { value: 'turnover', label: (String(t('admin.copy.89fd862d'))).trim() }]" /></label>
        <p v-if="!policyRatioValid" class="am-field-error">{{ t('admin.copy.379b3171') }}</p><button class="am-primary button button-primary" type="submit" :disabled="writing || !policyRatioValid">{{ t('admin.copy.ea2d7203') }}</button>
      </form></section>
      </DetailModal>
      <ManagementList v-if="mode !== 'settings' && canViewAgents" :busy="loading"><template #tabs><ConsoleTabs  class="am-view-switch" :items="[{id:'list',label:t('admin.agents.allLevels')},{id:'hierarchy',label:t('admin.agents.byHierarchy')}]" :value="viewMode" :disabled="loading || writing" :select="switchView" /></template><template v-if="(canWriteAgents && policy) || (viewMode === 'hierarchy' && parentId)" #actions><button v-if="viewMode === 'hierarchy' && parentId" type="button" class="am-secondary button button-secondary" :disabled="loading" @click="withGlobalLoading(goBack)">{{ t('admin.copy.2fbcdb10') }}</button><button v-if="canWriteAgents && policy" type="button" class="am-secondary button button-secondary" :disabled="writing || !!pending || Boolean(parentNode?.pending_change)" @click="startCreate">{{ t('admin.copy.b8713ea7') }}</button></template><template v-if="viewMode === 'hierarchy' && parentId" #summary><p class="muted">{{ associationLabel(parentNode, 'agent', parentId, associationLocale) }}</p></template>
        <template #filters><DirectorySearch v-if="viewMode==='list'" v-model="search" :label="t('admin.members.search')" :query-label="t('admin.members.query')" :clear-label="t('admin.copy.b089734d')" :busy="loading" :active="Boolean(search||onlineFilter||query||onlineQuery)" :search="searchAgents" :clear="clearSearch"><SelectControl v-model="onlineFilter" :label="t('admin.members.onlineStatus')" :options="[{ value: '', label: (String(t('admin.members.allOnline'))).trim() }, { value: 'true', label: (String(t('admin.members.online'))).trim() }, { value: 'false', label: (String(t('admin.members.offline'))).trim() }]" /></DirectorySearch></template>
        <div v-if="viewMode === 'list'" class="table-wrap"><table class="agent-directory-table console-table console-responsive-table"><thead><tr><th>{{ t('admin.copy.851c6121') }}</th><th>{{ t('admin.members.onlineStatus') }}</th><th>{{ t('admin.members.points') }}</th><th>{{ t('admin.copy.ac22afab') }}</th><th>{{ t('admin.copy.4f39e60b') }}</th><th>{{ t('admin.copy.602b68c2') }}</th><th>{{ t('admin.copy.cf6d9c6d') }}</th><th>{{ t('admin.copy.d21fcbef') }}</th></tr></thead><tbody><tr v-for="node in directoryNodes" :key="node.id"><td :data-label="t('admin.copy.851c6121')"><button class="text-button member-account" type="button" @click="withGlobalLoading(selectNode, node)">{{ node.username || t('association.unnamed') }}</button></td><td :data-label="t('admin.members.onlineStatus')"><span class="member-presence" :class="{ 'is-online': node.online }"><i aria-hidden="true" />{{ t(node.online ? 'admin.members.online' : 'admin.members.offline') }}</span></td><td class="member-points" :data-label="t('admin.members.points')">{{ node.display_points === null ? t('admin.members.pointsRestricted') : node.display_points }}</td><td :data-label="t('admin.copy.ac22afab')">{{ node.depth }}</td><td :data-label="t('admin.copy.4f39e60b')">{{ agentRatioToPercentage(node.config.ratio) }}%</td><td :data-label="t('admin.copy.602b68c2')">{{ modeLabel(node.effective_mode) }}</td><td :data-label="t('admin.copy.cf6d9c6d')">{{ t(node.config.status === 'active' ? 'admin.copy.5eb01613' : 'admin.copy.5e3326db') }}</td><td :data-label="t('admin.copy.d21fcbef')"><MemberActionsMenu :id="`agent-actions-${node.id}`" :label="t('admin.agents.actions', { account: node.username })"><button role="menuitem" type="button" @click="withGlobalLoading(selectNode, node)">{{ t('admin.copy.01db40da') }}</button><button role="menuitem" type="button" @click="withGlobalLoading(openChildren, node)">{{ t('admin.copy.2e035a42') }}</button></MemberActionsMenu></td></tr><tr v-if="listLoaded && !loading && !error && !directoryNodes.length"><td colspan="8" class="empty-state">{{ t('admin.copy.a59f8381') }}</td></tr></tbody></table></div>
        <template v-else><div v-global-loading v-if="loading" class="am-note" role="status">{{ t('admin.copy.fed96ce5') }}</div><div class="am-node-list">
          <article v-for="node in nodes" :key="node.id" class="am-node"><div class="am-node-main"><strong><AssociationName :record="node" kind="agent" :id="node.id" :locale="associationLocale" /></strong><span>{{ t('admin.copy.ac22afab') }} {{ node.depth }} · {{ t('admin.copy.4f39e60b') }} {{ agentRatioToPercentage(node.config.ratio) }}% · {{ node.config.status === "active" ? t('admin.copy.5eb01613') : t('admin.copy.5e3326db') }}</span><small>{{ t('admin.copy.602b68c2') }}{{ node.effective_mode === "loss" ? t('admin.copy.a13e79ef') : t('admin.copy.89fd862d') }}</small></div><div class="am-actions ui-form-actions"><button type="button" class="am-secondary button button-secondary" @click="withGlobalLoading(selectNode, node)">{{ t('admin.copy.01db40da') }}</button><button v-if="viewMode === 'hierarchy'" type="button" class="am-secondary button button-secondary" @click="withGlobalLoading(openChildren, node)">{{ t('admin.copy.2e035a42') }}</button></div></article>
          <p v-if="!loading && !error && nodes.length === 0" class="am-note empty-state">{{ t('admin.copy.a59f8381') }}</p></div></template>
      <template #pagination><TablePagination :show-empty="listLoaded" :total="total" :page="Math.floor(offset / pageSize + 1)" :count="nodes.length" :locale="associationLocale" :page-size="pageSize" :previous-disabled="loading || offset === 0" :next-disabled="loading || !hasNext" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.2e9c9e7a')" :go-to="(target: number) => changePage(target - (Math.floor(offset / pageSize + 1)))" /></template></ManagementList><p v-else-if="mode !== 'settings'" class="am-note">{{ t('admin.copy.5d517450') }}</p>
      <DetailModal class="agent-dialog ui-form-dialog" v-if="canWriteAgents && policy && canViewAgents && !selected && createOpen" :title="parentId ? t('admin.copy.512d8003') : t('admin.copy.fd71461b')" :close-label="t('association.close')" :close-disabled="writing" @close="!writing && (createOpen = false)">

      <section v-if="canWriteAgents && policy && canViewAgents && !selected && createOpen" class="am-card"><p v-if="parentNode" class="am-note">{{ t('admin.copy.ceac0bce') }} <AssociationName :record="parentNode" kind="agent" :id="parentNode.id" :locale="associationLocale" /> {{ t('admin.copy.ac22afab') }} {{ parentNode.depth }}</p><p v-else-if="parentId" class="am-note">{{ t('admin.copy.87e0d696') }}</p><form v-write-guard="writing" novalidate v-inline-validation class="am-form ui-form" @submit.prevent="withGlobalLoading(prepareNode)"><fieldset class="am-form am-edit-fields" :disabled="writing || !!pending">
        <AccountCreateFields v-model:account="agentUsername" v-model:password="agentPassword" :account-label="t('admin.copy.61c7d845')" :password-label="t('admin.agents.password')" :account-required="true" :account-placeholder="t('admin.agents.accountPlaceholder')" :password-characters="true" />
        <label>{{ t('admin.copy.94fca494') }}<SelectControl v-model="form.mode" :label="t('admin.copy.94fca494')" :required="isFirstLevel" :options="[{ value: '', label: (String(isFirstLevel ? t('admin.agents.selectMode') : t('admin.copy.b8bd227c'))).trim(), disabled: isFirstLevel }, ...((allowedModes).map((mode) => ({ value: mode, label: (String(mode === 'loss' ? t('admin.copy.a13e79ef') : t('admin.copy.89fd862d'))).trim() })))]" /></label><AgentRatioField v-if="draftMode==='loss'" v-model="form.loss_ratio" :label="t('admin.agents.lossRatio')" />
        <AgentRatioField v-if="draftMode==='turnover'" v-model="form.turnover_ratio" :label="t('admin.agents.turnoverRatio')" /><p v-if="draftMode && createRatioLimit !== null" class="am-note">{{ t(parentId ? 'admin.agents.parentRatioLimit' : draftMode === 'loss' ? 'admin.agents.lossRatioLimit' : 'admin.agents.turnoverRatioLimit', { ratio: createRatioLimit }) }}</p><p v-if="createRatioExceeded" class="am-field-error" role="alert">{{ t('admin.agents.ratioExceeded', { ratio: createRatioLimit! }) }}</p><label>{{ t('admin.copy.054393be') }}<SelectControl v-model="form.status" :disabled="changingMode" :label="t('admin.copy.054393be')" :options="[{ value: 'active', label: (String(t('admin.copy.5eb01613'))).trim() }, { value: 'disabled', label: (String(t('admin.copy.5e3326db'))).trim() }]" /></label><label class="am-check"><input v-model="form.can_create_children" :disabled="changingMode" type="checkbox" :aria-label="t('admin.copy.a78c6c73')">{{ t('admin.copy.a78c6c73') }}</label>
        <p v-if="draftMode && draftRatio && !nodeRatioValid" class="am-field-error">{{ t('admin.copy.379b3171') }}</p></fieldset><p v-if="pending?.operation === 'create' && !writing" class="am-note" role="status">{{ t('admin.agents.createUnknown') }}</p><button v-write-guard="writing" v-if="pending?.operation === 'create'" class="button button-primary" type="button" :disabled="writing" @click="withGlobalLoading(retryFrozen)">{{ t('admin.agents.retryCreate') }}</button><button v-else class="am-primary button button-primary" type="submit" :disabled="writing || !nodeRatioValid || createRatioExceeded || !agentUsername.trim() || (Boolean(parentId) && !parentNode)">{{ t('admin.agents.create') }}</button></form></section>
      </DetailModal>
      <DetailModal class="agent-dialog ui-form-dialog" v-if="canViewAgents && !!selected && detailOpen" :title="t('admin.copy.01db40da')" :close-label="t('admin.copy.2bb2cb1d')" @close="closeDetail()">

<section v-if="canViewAgents && selected" class="am-card agent-detail">
        <dl class="agent-detail-facts">
          <div><dt>{{ t('admin.copy.61c7d845') }}</dt><dd><AssociationName :record="selected" kind="agent" :id="selected.id" :locale="associationLocale" /></dd></div>
          <div><dt>{{ t('admin.nav.agentHierarchy') }}</dt><dd>{{ t('admin.agents.levelNames').split(',')[selected.depth - 1] }}</dd></div>
        </dl>
        <JoinCodeManagement v-if="joinCodePermissions(account, brandId).view" :key="selected.id" :account="account" :brand-id="brandId" :member-id="selected.agent_account_id" kind="agent" @session-invalid="emit('session-invalid')" />
        <p v-if="selected.parent_id && !selectedParentReady" class="am-message is-error" role="status">{{ t('admin.copy.c202b758') }}</p>
        <dl v-if="!canWriteAgents" class="agent-detail-facts"><div><dt>{{ t('admin.copy.94fca494') }}</dt><dd>{{ modeLabel(selected.parent_id ? selected.config.mode : selected.effective_mode) }}</dd></div><div><dt>{{ t(selected.effective_mode === 'loss' ? 'admin.agents.lossRatio' : 'admin.agents.turnoverRatio') }}</dt><dd>{{ agentRatioToPercentage(selected.config.ratio) }}%</dd></div><div><dt>{{ t('admin.copy.054393be') }}</dt><dd>{{ selected.config.status === "active" ? t('admin.copy.98d24810') : t('admin.copy.5e3326db') }}</dd></div><div><dt>{{ t('admin.copy.59e9ba82') }}</dt><dd>{{ selected.config.can_create_children ? t('admin.copy.1a264f45') : t('admin.copy.b92bb6e0') }}</dd></div></dl>
        <p v-if="selected.pending_change" class="am-note" role="status">{{ t('admin.agents.scheduled', { mode: modeLabel(selected.pending_change.mode), ratio: agentRatioToPercentage(selected.pending_change.ratio) + "%", time: displayDate(selected.pending_change.effective_at) }) }}</p>
        <form v-write-guard="writing" novalidate v-inline-validation v-if="canWriteAgents" class="am-form ui-form agent-detail-form" @submit.prevent="withGlobalLoading(prepareNode)"><fieldset :disabled="writing || !!pending || Boolean(selected.pending_change)" class="am-form am-edit-fields agent-detail-fields">
          <label>{{ t('admin.copy.94fca494') }}<SelectControl v-model="form.mode" :label="t('admin.copy.94fca494')" :required="isFirstLevel" :options="[{ value: '', label: (String(isFirstLevel ? t('admin.agents.selectMode') : t('admin.copy.b8bd227c'))).trim(), disabled: isFirstLevel }, ...((allowedModes).map((mode) => ({ value: mode, label: (String(mode === 'loss' ? t('admin.copy.a13e79ef') : t('admin.copy.89fd862d'))).trim() }))), ...((form.mode && !allowedModes.includes(form.mode)) ? [{ value: form.mode, label: (String(t('admin.copy.7a1e57b8')) + String(form.mode === 'loss' ? t('admin.copy.a13e79ef') : t('admin.copy.89fd862d'))).trim(), disabled: true }] : [])]" /></label><AgentRatioField v-if="draftMode==='loss'" v-model="form.loss_ratio" :label="t('admin.agents.lossRatio')" />
        <AgentRatioField v-if="draftMode==='turnover'" v-model="form.turnover_ratio" :label="t('admin.agents.turnoverRatio')" /><label>{{ t('admin.copy.054393be') }}<SelectControl v-model="form.status" :disabled="changingMode" :label="t('admin.copy.054393be')" :options="[{ value: 'active', label: (String(t('admin.copy.5eb01613'))).trim() }, { value: 'disabled', label: (String(t('admin.copy.5e3326db'))).trim() }]" /></label><label class="am-check"><span>{{ t('admin.copy.a78c6c73') }}</span><input v-model="form.can_create_children" :disabled="changingMode" type="checkbox" :aria-label="t('admin.copy.a78c6c73')"></label>
          <section v-if="changingMode" class="am-mode-line">
            <p class="am-note">{{ t('admin.agents.changeHelp') }}</p>
            <p v-global-loading v-if="modeScopeLoading" role="status">{{ t('admin.copy.fed96ce5') }}</p>
            <label v-for="node in modeLine.filter(item => item.id !== selected!.id)" :key="node.id">
              <AssociationName :record="node" kind="agent" :id="node.id" :locale="associationLocale" />
              <span>{{ t(form.mode === 'loss' ? 'admin.agents.lossRatio' : 'admin.agents.turnoverRatio') }}</span>
              <span class="am-ratio-input"><input v-model.trim="modeRatios[node.id]" inputmode="decimal" autocomplete="off" required :aria-label="t('admin.agents.newRatioFor', { account: associationLabel(node, 'agent', node.id, associationLocale) })"><span aria-hidden="true">%</span></span>
            </label>
          </section>
          </fieldset>
          <p v-if="draftMode && draftRatio && !nodeRatioValid" class="am-field-error">{{ t('admin.copy.379b3171') }}</p><div class="am-actions ui-form-actions"><button v-if="pending && (pending.operation === 'update' || pending.operation === 'mode-change')" v-write-guard="writing" class="button button-primary" type="button" :disabled="writing" @click="withGlobalLoading(retryFrozen)">{{ t('admin.settings.retryOperation') }}</button><button v-else class="am-primary button button-primary" type="submit" :disabled="writing || !!pending || Boolean(selected.pending_change) || (changingMode && !modeLineReady) || !nodeRatioValid || (Boolean(selected.parent_id) && !selectedParentReady)">{{ t('admin.copy.ea2d7203') }}</button></div></form>
      </section>
</DetailModal>
      <div v-if="pending?.operation === 'create' && !createOpen" class="am-actions ui-form-actions"><button v-write-guard="writing" class="button button-primary" type="button" :disabled="writing || !canWriteAgents" @click="withGlobalLoading(retryFrozen)">{{ t('admin.agents.retryCreate') }}</button></div>
      <div v-if="pending?.operation === 'policy'" class="am-actions ui-form-actions"><button v-write-guard="writing" class="button button-primary" type="button" :disabled="writing || !canWritePolicy" @click="withGlobalLoading(retryFrozen)">{{ t('admin.settings.retryOperation') }}</button></div>
      <div v-if="pending && (pending.operation === 'update' || pending.operation === 'mode-change') && !detailOpen" class="am-actions ui-form-actions"><button v-write-guard="writing" class="button button-primary" type="button" :disabled="writing || !canWriteAgents" @click="withGlobalLoading(retryFrozen)">{{ t('admin.settings.retryOperation') }}</button></div>
    </template>
  </section>
</template>

<style scoped>
.agent-management{width:100%;min-width:0;color:var(--ink)}.am-head,.am-card-head{display:flex;justify-content:space-between;align-items:flex-start;gap:14px}.am-head{margin:4px 0 16px}.am-head h2{font-size:20px;margin:3px 0 5px}.am-head p,.am-card-head p{margin:0;color:var(--muted);font-size:var(--body-font-size);line-height:1.55;overflow-wrap:anywhere}.am-eyebrow{font-size:var(--body-font-size)!important;letter-spacing:1.1px;color:var(--muted-light)!important;font-weight:700}.am-brand,.am-pending{border-radius:20px;background:var(--canvas);padding:8px 12px;color:var(--muted);font-size:var(--body-font-size);white-space:nowrap}.am-note,.am-message{border-radius:8px;background:var(--canvas);color:var(--muted);padding:11px 13px;font-size:var(--body-font-size);line-height:1.6;overflow-wrap:anywhere;margin:12px 0}.am-message.is-error{background:var(--danger-bg);color:var(--danger)}.am-message.is-ok{background:var(--primary-light);color:var(--primary-dark)}.am-card{background:var(--surface);border:1px solid var(--line);border-radius:11px;padding:16px;min-width:0;box-shadow:var(--shadow-sm);margin:13px 0}.am-card h3{font-size:16px;margin:0 0 12px}.am-card-head{margin-bottom:14px}.am-card-head h3{margin:0 0 4px}.am-card-head h2{margin:0 0 4px}.am-facts{display:grid;grid-template-columns:repeat(auto-fit,minmax(145px,1fr));gap:10px;margin:0}.am-facts div{border:1px solid var(--line);border-radius:8px;padding:12px;min-width:0}.am-facts dt{font-size:var(--body-font-size);color:var(--muted)}.am-facts + .am-actions{margin-top:12px}.am-ratio-input{display:flex;align-items:center;gap:8px;min-width:0}.am-ratio-input input{flex:1 1 0;min-width:0}.am-ratio-input > span{flex:none;color:var(--muted)}.am-facts dd{font-size:16px;font-weight:700;margin:6px 0 0;overflow-wrap:anywhere;font-variant-numeric:tabular-nums}.am-meta{font-size:var(--body-font-size);color:var(--muted);overflow-wrap:anywhere}.am-form{display:grid;grid-template-columns:minmax(0,1fr);gap:13px}.am-form label{display:grid;gap:6px;font-size:var(--body-font-size);font-weight:600;min-width:0}.am-form input:not([type=checkbox]), .am-form textarea{box-sizing:border-box;min-width:0}.am-form textarea{width:100%}.am-form small{font-size:var(--body-font-size);font-weight:400;color:var(--muted);line-height:1.5}.am-form .am-check,.am-check{display:flex;align-items:center;gap:9px;min-height:44px;font-size:14px}.am-check input{width:18px;height:18px;flex:none}.am-form textarea{resize:vertical}.am-field-error{grid-column:1/-1;color:var(--danger);font-size:var(--body-font-size);margin:0}.am-actions{display:flex;gap:8px;flex-wrap:wrap;align-items:center}.am-node-list{display:grid;gap:9px}.am-node{display:flex;justify-content:space-between;gap:12px;border:1px solid var(--line);border-radius:9px;padding:12px;min-width:0}.am-node-main{display:grid;gap:4px;min-width:0}.am-node-main strong{font-size:15px;overflow-wrap:anywhere}.am-node-main span,.am-node-main small{font-size:var(--body-font-size);color:var(--muted);overflow-wrap:anywhere}.am-node code,.am-card code,.am-meta code{overflow-wrap:anywhere;word-break:break-word}.am-review{border-color:var(--primary)}.am-json{white-space:pre-wrap;overflow-wrap:anywhere;word-break:break-word;max-height:320px;overflow:auto;background:var(--canvas);border-radius:8px;padding:12px;font-size:var(--body-font-size);line-height:1.5}.am-confirm{margin:12px 0;font-size:14px}
@media(max-width:640px){.am-head,.am-card-head,.am-node{flex-direction:column}.am-form{grid-template-columns:minmax(0,1fr)}.am-brand{white-space:normal}.am-actions{width:100%}.am-actions button{flex:1}.am-card{padding:14px}.am-facts{grid-template-columns:repeat(2,minmax(0,1fr))}}

.agent-management .agent-dialog { width:min(520px,calc(100vw - 24px)); }
.am-edit-fields{border:0;padding:0;margin:0;min-width:0}.am-mode-line{display:grid;gap:12px;min-width:0}
.agent-dialog .am-form { grid-template-columns:minmax(0,1fr); }
.agent-dialog .am-form > label { justify-items:start; }
.agent-dialog .am-facts { grid-template-columns:minmax(0,1fr); }
.agent-dialog .am-card { margin:0; padding:0; border:0; box-shadow:none; }
@media(max-width:900px),(pointer:coarse){.am-form input:not([type=checkbox]){width:100%}}

.agent-detail{--agent-detail-label-width:112px;display:grid;gap:12px}
.agent-detail-facts{display:grid;gap:12px;margin:0}
#app .agent-detail :deep(dl > div:has(> dt + dd)){grid-template-columns:var(--agent-detail-label-width) minmax(0,1fr)!important;gap:12px!important;padding:0;border:0;background:transparent}
#app .agent-detail :deep(dt){color:var(--ink-secondary);font-weight:600}
#app .agent-detail :deep(dd){text-align:start;font-weight:400;font-size:var(--body-font-size)}
#app .agent-detail :deep(.owner-join-code){margin:0;padding:0;border:0}
#app .agent-detail .agent-detail-fields > label:not(.am-check){grid-template-columns:var(--agent-detail-label-width) minmax(0,1fr);align-items:center;gap:12px}
#app .agent-detail .agent-detail-fields > label :deep(:is(.select-control,.am-ratio-input)){width:100%;min-width:0}
#app .agent-detail .agent-detail-fields > label :deep(.am-ratio-input){max-width:160px}
#app .agent-detail .agent-detail-fields > .am-check{display:grid;grid-template-columns:var(--agent-detail-label-width) minmax(0,1fr);gap:12px!important;min-height:var(--control-height)}
#app .agent-detail .agent-detail-fields > .am-check input{margin:0}
#app .agent-detail .agent-detail-form{gap:12px}
#app .agent-detail .ui-form-actions{margin-top:0}
</style>
