<script setup lang="ts">
import ConsoleTabs from "@lottery/shared/ConsoleTabs.vue"
import AgentRatioField from "@lottery/shared/AgentRatioField.vue"
import AccountCreateFields from "@lottery/shared/AccountCreateFields.vue"
import DirectorySearch from "@lottery/shared/DirectorySearch.vue"
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName, associationLabel } from "@lottery/shared";

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
import { joinCodePermissions } from "./join-codes-api";
import JoinCodeManagement from "./JoinCodeManagement.vue";
import MemberActionsMenu from '@lottery/shared/MemberActionsMenu.vue'
import TablePagination from '@lottery/shared/TablePagination.vue'
import DetailModal from "@lottery/shared/DetailModal.vue"
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { agentsPermissions, createAgentsApi, type ModeChangeBody, type ModeChangeReceipt, type AgentDirectoryNode, type AgentNode, type AgentPolicy, type CreateAgentBody, type NodeConfig, type PolicyConfig, type Revision, type SaveAgentPolicyBody, type UpdateAgentBody } from "./agents-api";
import { allowedChildModes, classifyAgentFailure, clearAllPendingAgentWrites, clearPendingAgentWrite, freezeAgentBody, getPendingAgentWrite, parentModeVerified, setPendingAgentWrite, type AgentWriteOperation, type AgentWriteBody, type PendingAgentWrite } from "./agents-state";
import { adminKey, useAdminI18n } from "./i18n";

import { agentPercentageToRatio, agentRatioToPercentage, validAgentPercentage } from "./agent-percentage";

type Mode = "loss" | "turnover";
const props = withDefaults(defineProps<{ account: AdminAccount; brandId: string; mode?: 'directory' | 'settings' }>(), { mode: 'directory' });
const viewMode = ref<'list' | 'hierarchy'>('list');
const { t, display } = useAdminI18n();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createAgentsApi();
const agentPassword = ref("");
const permissionSet = computed(() => agentsPermissions(props.account, props.brandId));
const canViewPolicy = computed(() => permissionSet.value.policyView);
const canViewAgents = computed(() => permissionSet.value.view);
const canWritePolicy = computed(() => permissionSet.value.policyWrite);
const canWriteAgents = computed(() => permissionSet.value.write);
const canView = computed(() => props.mode === 'settings' ? canViewPolicy.value : canViewAgents.value);
const canWrite = computed(() => canWritePolicy.value || canWriteAgents.value);
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
async function searchAgents() { query.value = search.value.trim(); onlineQuery.value = onlineFilter.value; offset.value = 0; await loadTree(); }
async function clearSearch() { search.value = ''; onlineFilter.value = ''; await searchAgents(); }
const hasNext = computed(() => BigInt(offset.value) + BigInt(nodes.value.length) < BigInt(total.value));
const total = ref("0");
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
const history = ref<Revision[]>([]);
const showNodeHistory = ref(false);
const loading = ref(false);
const writing = ref(false);
const error = ref<string | import("@lottery/shared").LocalizedMessage>("");
const notice = ref("");
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
  error.value = cause instanceof Error ? publicErrorMessage(cause) : "请求失败";
}
async function loadPolicy(g = ++generation, brand = props.brandId, account = props.account.id) {
  if (!canViewPolicy.value || !brand) { policy.value = null; return; }
  try {
    const result = await api.policy(brand);
    if (!sameScope(g, brand, account)) return;
    if (result.brand_id !== brand) throw new Error("策略响应与当前品牌不匹配。");
    policy.value = result; policyDraft.value = { ...result.config }; policyLossPercentage.value = agentRatioToPercentage(result.config.loss_ratio_cap); policyTurnoverPercentage.value = agentRatioToPercentage(result.config.turnover_ratio_cap);
  } catch (cause) { if (sameScope(g, brand, account)) handleError(cause); }
}
async function loadTree(g = ++generation, brand = props.brandId, account = props.account.id) {
  if (!canViewAgents.value || !brand) { nodes.value = []; return; }
  loading.value = true; error.value = ""; nodes.value = []; directoryNodes.value = []; total.value = "0"; parentNode.value = null;
  try {
    const [result, parent] = await Promise.all([viewMode.value === 'list' ? api.list(brand, pageSize, offset.value, { query: query.value, online: onlineQuery.value }) : api.tree(brand, parentId.value, pageSize, offset.value), parentId.value ? api.node(brand, parentId.value) : Promise.resolve(null)]);
    if (!sameScope(g, brand, account)) return;
    if (result.brand_id !== brand || (viewMode.value === 'hierarchy' && (!('parent_id' in result) || result.parent_id !== parentId.value))) throw new Error("代理目录响应与当前查询范围不匹配。");
    nodes.value = result.items; directoryNodes.value = !('parent_id' in result) ? result.items : []; total.value = result.total_count; parentNode.value = parent; restoreIntent();
  } catch (cause) { if (sameScope(g, brand, account)) handleError(cause); }
  finally { if (sameScope(g, brand, account)) loading.value = false; }
}
async function loadAll() { const g = ++generation; const b = props.brandId; const a = props.account.id; await Promise.all([loadPolicy(g, b, a), ...(props.mode === 'settings' ? [] : [loadTree(g, b, a)])]); }
async function selectNode(n: AgentNode) {
  const b = props.brandId, a = props.account.id, g = ++generation;
  error.value = "";
  try {
    const detail = await api.node(b, n.id);
    if (!sameScope(g, b, a)) return;
    if (detail.id !== n.id || detail.brand_id !== b) throw new Error("代理详情响应与当前代理或品牌不匹配。");
    selected.value = detail; detailOpen.value = true;
    form.value = { agent_account_id: detail.agent_account_id, loss_ratio: detail.effective_mode === "loss" ? agentRatioToPercentage(detail.config.ratio) : "", turnover_ratio: detail.effective_mode === "turnover" ? agentRatioToPercentage(detail.config.ratio) : "", mode: detail.parent_id ? detail.config.mode ?? "" : detail.effective_mode, status: detail.config.status, can_create_children: detail.config.can_create_children };
    selectedParentReady.value = !detail.parent_id; selectedParentMode.value = null;
    history.value = []; showNodeHistory.value = false; restoreIntent();
    if (detail.parent_id) {
      try {
        const parent = await api.node(b, detail.parent_id);
        if (!sameScope(g, b, a) || selected.value?.id !== detail.id) return;
        if (parent.brand_id !== b || parent.id !== detail.parent_id) throw new Error("父级代理响应与当前品牌或关系不匹配。");
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
  viewMode.value = mode; parentId.value = null; parentNode.value = null; offset.value = 0;
  search.value = ''; query.value = ''; onlineFilter.value = ''; onlineQuery.value = '';
  closeDetail(); await loadAll();
}
async function openChildren(n: AgentNode) { viewMode.value = 'hierarchy'; parentId.value = n.id; offset.value = 0; closeDetail(); await loadTree(); }
async function goBack() {
  if (!parentNode.value) { parentId.value = null; offset.value = 0; }
  else { parentId.value = parentNode.value.parent_id; offset.value = 0; }
  closeDetail(); await loadTree();
}
async function loadNodeHistory() {
  if (!selected.value) return;
  const b = props.brandId, a = props.account.id, id = selected.value.id, g = ++generation;
  try { const r = await api.nodeHistory(b, id, pageSize, 0); if (!sameScope(g, b, a) || selected.value?.id !== id) return; if (r.brand_id !== b || r.agent_id !== id) throw new Error("代理历史响应范围不匹配。"); history.value = r.items; showNodeHistory.value = true; }
  catch (cause) { if (sameScope(g, b, a)) handleError(cause); }
}
function freeze(operation: AgentWriteOperation, resourceId: string | null, body: AgentWriteBody): boolean {
  const scope = { accountId: props.account.id, brandId: props.brandId };
  const old = getPendingAgentWrite(scope);
  if (old && (old.operation !== operation || old.resourceId !== resourceId || JSON.stringify(old.body) !== JSON.stringify(body))) {
    pending.value = old;
    error.value = "此账号和品牌已有未确认结果的冻结请求；请先核对并按原键重试。";
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
  if (!parentModeVerified(parentIdForWrite, selected.value ? selectedParentReady.value : Boolean(parentNode.value) || !parentId.value)) { error.value = "父级生效模式尚未验证，当前不能创建或提交此代理变更。请重新读取父级或代理详情。"; return; }
  if (form.value.mode && !allowedModes.value.includes(form.value.mode)) { error.value = "代理模式必须沿用父级生效模式或与其保持一致；请先读取父级模式。"; return; }
  const config: NodeConfig = { ratio: agentPercentageToRatio(draftRatio.value), mode: form.value.mode || null, status: form.value.status, can_create_children: form.value.can_create_children };
  if (selected.value) {
    const n = selected.value;
    const pVersion = n.parent_id ? (parentNode.value?.id === n.parent_id ? parentNode.value.version : n.parent_version) : null;
    const body: UpdateAgentBody = { version: n.version, policy_version: policy.value.version, parent_version: pVersion, config };
    if (freeze("update", n.id, body)) await confirmWrite();
  } else {
    if (parentId.value && !parentNode.value) { error.value = "父级代理尚未读取，无法确认其版本。"; return; }
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
    notice.value = i.operation === "create" ? "代理账号已创建。" : "已收到与冻结请求匹配的成功回执；正在独立读取当前状态。";
    if (i.operation === "create") { agentPassword.value = ""; createOpen.value = false; }
    if (i.operation === "policy") { policyEditorOpen.value = false; await loadPolicy(); }
    else if (i.operation === "create") await loadTree();
    else if (i.operation === 'mode-change') { const root = await api.node(i.brandId,i.resourceId!); await selectNode(root); if (!isCurrentWrite()) return; await loadTree(); }
    else { await selectNode(receipt as AgentNode); if (!isCurrentWrite()) return; await loadTree(); }
    if (isCurrentWrite()) notice.value = i.operation === "create" ? "代理账号已创建。" : "写入已确认；页面显示的是随后读取的当前状态。";
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
      notice.value = i.operation === "create" ? "暂未确认创建结果，请重试。系统会避免重复创建。" : "写入结果未知。冻结请求和幂等键已保留，只能按原请求重试。";
      error.value = cause instanceof Error ? publicErrorMessage(cause) : "请求结果未知。";
    } else {
      clearPendingAgentWrite(sharedScope, i.key);
      pending.value = getPendingAgentWrite(sharedScope);
      error.value = cause instanceof Error ? publicErrorMessage(cause) : "服务器拒绝了请求。";
    }
  } finally { if (isCurrentWrite()) writing.value = false; }
}
async function retryFrozen() { await confirmWrite(); }
function configText(c: PolicyConfig | NodeConfig) { return c; }
function modeLabel(value: string | null | undefined) { return value === "loss" ? t('admin.copy.a13e79ef') : value === "turnover" ? t('admin.copy.89fd862d') : t('admin.copy.b8bd227c'); }
function localizedAgentMessage(value: string | import("@lottery/shared").LocalizedMessage): string {
  if (typeof value !== "string") return displayPublicError(value);
  const translations: Record<string, string> = {
    "代理账号已创建。": "admin.agents.created",
    "暂未确认创建结果，请重试。系统会避免重复创建。": "admin.agents.createUnknown",
    "此账号和品牌已有未确认结果的冻结请求；请先核对并按原键重试。": 'admin.copy.48a6af2c',
    "请核对冻结请求内容后确认提交。": 'admin.copy.0784fa6e',
    "父级生效模式尚未验证，当前不能创建或提交此代理变更。请重新读取父级或代理详情。": 'admin.copy.1e15f14f',
    "代理模式必须沿用父级生效模式或与其保持一致；请先读取父级模式。": 'admin.copy.a596a9b7',
    "父级代理尚未读取，无法确认其版本。": 'admin.copy.421b0a6c',
    "已收到与冻结请求匹配的成功回执；正在独立读取当前状态。": 'admin.copy.6a656cc2',
    "写入已确认；页面显示的是随后读取的当前状态。": 'admin.copy.06415e47',
    "写入结果未知。冻结请求和幂等键已保留，只能按原请求重试。": 'admin.copy.bad148c4',
  };
  const pair = translations[value];
  return pair ? t(adminKey(pair)) : value;
}
async function changePage(d: number) { offset.value = Math.max(0, offset.value + d * pageSize); await loadTree(); }
onMounted(() => { restoreIntent(); void loadAll(); });
onBeforeUnmount(() => { disposed = true; generation++; writeGeneration++; modeScopeGeneration++; });
watch(() => [props.brandId, props.account.id, permissionSignature.value, props.mode] as const, () => {
  if (activeAccountId !== props.account.id) {
    clearAllPendingAgentWrites();
    activeAccountId = props.account.id;
  }
  modeScopeGeneration++; modeScopeLoading.value = false; modeLine.value = []; modeRatios.value = {};
  viewMode.value = 'list';
  search.value = ''; query.value = ''; onlineFilter.value = ''; onlineQuery.value = ''; directoryNodes.value = [];
  generation++; writeGeneration++; policy.value = null; policyEditorOpen.value = false; nodes.value = []; createOpen.value = false; selected.value = null; detailOpen.value = false; parentId.value = null; parentNode.value = null; offset.value = 0; error.value = ""; notice.value = ""; loading.value = false; writing.value = false; history.value = []; restoreIntent(); if (canView.value) void loadAll();
}, { flush: "sync" });
</script>

<template>
  <section class="agent-management">
    <header v-if="mode !== 'settings' || !policy" class="am-head"><div><h2>{{ t(mode === 'settings' ? 'admin.agents.rules' : 'admin.nav.agentList') }}</h2></div></header>
    <div v-if="error" class="am-message is-error" role="alert">{{ localizedAgentMessage(error) }}</div><p v-if="notice" class="am-message is-ok" role="status">{{ localizedAgentMessage(notice) }}</p>
    <p v-if="!canView" class="am-note">{{ t('admin.copy.07a2166b') }}</p>
    <template v-else>
      <section v-if="mode === 'settings' && policy" class="panel"><div class="am-card-head"><div><h2>{{ t('admin.agents.rules') }}</h2></div><button v-refresh-button="t('admin.copy.1935ecf7')" type="button" class="am-secondary button button-secondary" @click="withGlobalLoading(loadAll)">{{ t('admin.copy.1935ecf7') }}</button></div>
        <dl class="am-facts"><div><dt>{{ t('admin.copy.eae52028') }}</dt><dd>{{ policy.config.max_depth }}</dd></div><div><dt>{{ t('admin.agents.lossRatioCap') }}</dt><dd>{{ agentRatioToPercentage(policy.config.loss_ratio_cap) }}%</dd></div><div><dt>{{ t('admin.agents.turnoverRatioCap') }}</dt><dd>{{ agentRatioToPercentage(policy.config.turnover_ratio_cap) }}%</dd></div><div><dt>{{ t('admin.copy.60850269') }}</dt><dd>{{ policy.config.mode === "loss" ? t('admin.copy.a13e79ef') : t('admin.copy.89fd862d') }}</dd></div></dl>
        <div class="am-actions"><button v-if="mode === 'settings' && canWritePolicy && policy" class="button button-secondary" type="button" :disabled="writing || !!pending" @click="editPolicy">{{ t('admin.copy.24dfbf6c') }}</button></div>
      </section>
      <DetailModal class="agent-dialog" v-if="mode === 'settings' && canWritePolicy && policy && policyEditorOpen" :open="!pending" :title="t('admin.nav.agentSettings')" :close-label="t('association.close')" @close="!writing && (policyEditorOpen = false)">
        <p v-if="error" class="am-message is-error" role="alert">{{ localizedAgentMessage(error) }}</p>
      <section v-if="canWritePolicy && policy" class="am-card"><form v-write-guard="writing" novalidate v-inline-validation class="am-form" @submit.prevent="withGlobalLoading(preparePolicy)">
        <label>{{ t('admin.copy.96b469dc') }}<input v-model.number="policyDraft.max_depth" type="number" min="1" max="32" step="1" :aria-label="t('admin.copy.96b469dc')"></label>
        <label>{{ t('admin.agents.lossRatioCap') }}<span class="am-ratio-input"><input v-model.trim="policyLossPercentage" inputmode="decimal" autocomplete="off" :aria-label="t('admin.agents.lossRatioCap')"><span aria-hidden="true">%</span></span><small>{{ t('admin.copy.90be9d11') }}</small></label>
        <label>{{ t('admin.agents.turnoverRatioCap') }}<span class="am-ratio-input"><input v-model.trim="policyTurnoverPercentage" inputmode="decimal" autocomplete="off" :aria-label="t('admin.agents.turnoverRatioCap')"><span aria-hidden="true">%</span></span><small>{{ t('admin.copy.90be9d11') }}</small></label>
        <label>{{ t('admin.copy.60850269') }}<select v-model="policyDraft.mode" :aria-label="t('admin.copy.60850269')"><option value="loss">{{ t('admin.copy.a13e79ef') }}</option><option value="turnover">{{ t('admin.copy.89fd862d') }}</option></select></label>
        <p v-if="!policyRatioValid" class="am-field-error">{{ t('admin.copy.379b3171') }}</p><button class="am-primary button button-primary" type="submit" :disabled="writing || !policyRatioValid">{{ t('admin.copy.ea2d7203') }}</button>
      </form></section>
      </DetailModal>
      <ConsoleTabs v-if="mode!=='settings'&&canViewAgents" class="am-view-switch" :items="[{id:'list',label:t('admin.agents.allLevels')},{id:'hierarchy',label:t('admin.agents.byHierarchy')}]" :value="viewMode" :disabled="writing" :select="switchView" />
      <section v-if="mode !== 'settings' && canViewAgents" class="am-card"><div class="am-card-head"><div><p>{{ viewMode === 'list' ? t('admin.agents.allLevels') : parentId ? t('admin.copy.5ad5e615') + " " + associationLabel(parentNode, 'agent', parentId, associationLocale) : t('admin.copy.8645d7e9') }}</p></div><div class="am-actions"><button v-if="viewMode === 'hierarchy' && parentId" type="button" class="am-secondary button button-secondary" :disabled="loading" @click="withGlobalLoading(goBack)">{{ t('admin.copy.2fbcdb10') }}</button><button v-if="canWriteAgents && policy" type="button" class="am-secondary button button-secondary" :disabled="writing || !!pending || Boolean(parentNode?.pending_change)" @click="startCreate">{{ t('admin.copy.b8713ea7') }}</button></div></div>
        <DirectorySearch v-if="viewMode==='list'" v-model="search" :label="t('admin.members.search')" :query-label="t('admin.members.query')" :clear-label="t('admin.copy.b089734d')" :busy="loading" :active="Boolean(search||onlineFilter)" :search="searchAgents" :clear="clearSearch"><select v-model="onlineFilter" :aria-label="t('admin.members.onlineStatus')"><option value="">{{t('admin.members.allOnline')}}</option><option value="true">{{t('admin.members.online')}}</option><option value="false">{{t('admin.members.offline')}}</option></select></DirectorySearch>
        <div v-if="viewMode === 'list'" class="table-wrap"><table class="agent-directory-table console-table console-responsive-table"><thead><tr><th>{{ t('admin.copy.851c6121') }}</th><th>{{ t('admin.members.onlineStatus') }}</th><th>{{ t('admin.members.points') }}</th><th>{{ t('admin.copy.ac22afab') }}</th><th>{{ t('admin.copy.4f39e60b') }}</th><th>{{ t('admin.copy.602b68c2') }}</th><th>{{ t('admin.copy.cf6d9c6d') }}</th><th>{{ t('admin.copy.d21fcbef') }}</th></tr></thead><tbody><tr v-for="node in directoryNodes" :key="node.id"><td :data-label="t('admin.copy.851c6121')"><button class="text-button member-account" type="button" @click="withGlobalLoading(selectNode, node)">{{ node.username || t('association.unnamed') }}</button></td><td :data-label="t('admin.members.onlineStatus')"><span class="member-presence" :class="{ 'is-online': node.online }"><i aria-hidden="true" />{{ t(node.online ? 'admin.members.online' : 'admin.members.offline') }}</span></td><td class="member-points" :data-label="t('admin.members.points')">{{ node.display_points === null ? t('admin.members.pointsRestricted') : node.display_points }}</td><td :data-label="t('admin.copy.ac22afab')">{{ node.depth }}</td><td :data-label="t('admin.copy.4f39e60b')">{{ agentRatioToPercentage(node.config.ratio) }}%</td><td :data-label="t('admin.copy.602b68c2')">{{ modeLabel(node.effective_mode) }}</td><td :data-label="t('admin.copy.cf6d9c6d')">{{ t(node.config.status === 'active' ? 'admin.copy.5eb01613' : 'admin.copy.5e3326db') }}</td><td :data-label="t('admin.copy.d21fcbef')"><MemberActionsMenu :id="`agent-actions-${node.id}`" :label="t('admin.agents.actions', { account: node.username })"><button role="menuitem" type="button" @click="withGlobalLoading(selectNode, node)">{{ t('admin.copy.01db40da') }}</button><button role="menuitem" type="button" @click="withGlobalLoading(openChildren, node)">{{ t('admin.copy.2e035a42') }}</button></MemberActionsMenu></td></tr></tbody></table><p v-if="!loading && !directoryNodes.length" class="empty-state">{{ t('admin.copy.a59f8381') }}</p></div>
        <template v-else><div v-global-loading v-if="loading" class="am-note" role="status">{{ t('admin.copy.fed96ce5') }}</div><div class="am-node-list">
          <article v-for="node in nodes" :key="node.id" class="am-node"><div class="am-node-main"><strong><AssociationName :record="node" kind="agent" :id="node.id" :locale="associationLocale" /></strong><span>{{ t('admin.copy.ac22afab') }} {{ node.depth }} · {{ t('admin.copy.4f39e60b') }} {{ agentRatioToPercentage(node.config.ratio) }}% · {{ node.config.status === "active" ? t('admin.copy.5eb01613') : t('admin.copy.5e3326db') }}</span><small>{{ t('admin.copy.602b68c2') }}{{ node.effective_mode === "loss" ? t('admin.copy.a13e79ef') : t('admin.copy.89fd862d') }}</small></div><div class="am-actions"><button type="button" class="am-secondary button button-secondary" @click="withGlobalLoading(selectNode, node)">{{ t('admin.copy.01db40da') }}</button><button v-if="viewMode === 'hierarchy'" type="button" class="am-secondary button button-secondary" @click="withGlobalLoading(openChildren, node)">{{ t('admin.copy.2e035a42') }}</button></div></article>
          <p v-if="!loading && nodes.length === 0" class="am-note empty-state">{{ t('admin.copy.a59f8381') }}</p></div></template><TablePagination :summary="t('admin.agents.page', { page: String(Math.floor(offset / pageSize) + 1), count: String(nodes.length), total })"><template #actions><button type="button" class="button button-secondary" :disabled="loading || offset === 0" @click="withGlobalLoading(changePage, -1)">{{ t('admin.copy.dbe7a5d7') }}</button><button type="button" class="button button-secondary" :disabled="loading || !hasNext" @click="withGlobalLoading(changePage, 1)">{{ t('admin.copy.2e9c9e7a') }}</button></template></TablePagination>
      </section><p v-else-if="mode !== 'settings'" class="am-note">{{ t('admin.copy.5d517450') }}</p>
      <DetailModal class="agent-dialog" v-if="canWriteAgents && policy && canViewAgents && !selected && createOpen" :title="parentId ? t('admin.copy.512d8003') : t('admin.copy.fd71461b')" :close-label="t('association.close')" @close="!writing && (createOpen = false)">
        <p v-if="error" class="am-message is-error" role="alert">{{ localizedAgentMessage(error) }}</p>
      <section v-if="canWriteAgents && policy && canViewAgents && !selected && createOpen" class="am-card"><p v-if="parentNode" class="am-note">{{ t('admin.copy.ceac0bce') }} <AssociationName :record="parentNode" kind="agent" :id="parentNode.id" :locale="associationLocale" /> {{ t('admin.copy.ac22afab') }} {{ parentNode.depth }} · {{ t('admin.copy.ac8583ba') }}</p><p v-else-if="parentId" class="am-note">{{ t('admin.copy.87e0d696') }}</p><form v-write-guard="writing" novalidate v-inline-validation class="am-form" @submit.prevent="withGlobalLoading(prepareNode)"><fieldset class="am-form am-edit-fields" :disabled="writing || !!pending">
        <AccountCreateFields v-model:account="agentUsername" v-model:password="agentPassword" :account-label="t('admin.copy.61c7d845')" :password-label="t('admin.agents.password')" :account-required="true" :account-placeholder="t('admin.agents.accountPlaceholder')" :password-minlength="6" :password-maxlength="30" />
        <label>{{ t('admin.copy.94fca494') }}<select v-model="form.mode" :aria-label="t('admin.copy.94fca494')" :required="isFirstLevel"><option value="" :disabled="isFirstLevel">{{ isFirstLevel ? t('admin.agents.selectMode') : t('admin.copy.b8bd227c') }}</option><option v-for="mode in allowedModes" :key="mode" :value="mode">{{ mode === "loss" ? t('admin.copy.a13e79ef') : t('admin.copy.89fd862d') }}</option></select></label><AgentRatioField v-if="draftMode==='loss'" v-model="form.loss_ratio" :label="t('admin.agents.lossRatio')" :help="t('admin.copy.5f569342')" />
        <AgentRatioField v-if="draftMode==='turnover'" v-model="form.turnover_ratio" :label="t('admin.agents.turnoverRatio')" :help="t('admin.copy.5f569342')" /><p v-if="draftMode && createRatioLimit !== null" class="am-note">{{ t(parentId ? 'admin.agents.parentRatioLimit' : draftMode === 'loss' ? 'admin.agents.lossRatioLimit' : 'admin.agents.turnoverRatioLimit', { ratio: createRatioLimit }) }}</p><p v-if="createRatioExceeded" class="am-field-error" role="alert">{{ t('admin.agents.ratioExceeded', { ratio: createRatioLimit! }) }}</p><label>{{ t('admin.copy.054393be') }}<select v-model="form.status" :disabled="changingMode" :aria-label="t('admin.copy.054393be')"><option value="active">{{ t('admin.copy.5eb01613') }}</option><option value="disabled">{{ t('admin.copy.5e3326db') }}</option></select></label><label class="am-check"><input v-model="form.can_create_children" :disabled="changingMode" type="checkbox" :aria-label="t('admin.copy.a78c6c73')">{{ t('admin.copy.a78c6c73') }}</label>
        <p v-if="draftMode && draftRatio && !nodeRatioValid" class="am-field-error">{{ t('admin.copy.379b3171') }}</p></fieldset><p v-if="pending?.operation === 'create' && !writing" class="am-note" role="status">{{ t('admin.agents.createUnknown') }}</p><button v-write-guard="writing" v-if="pending?.operation === 'create'" class="button button-primary" type="button" :disabled="writing" @click="withGlobalLoading(retryFrozen)">{{ t('admin.agents.retryCreate') }}</button><button v-else class="am-primary button button-primary" type="submit" :disabled="writing || !nodeRatioValid || createRatioExceeded || !agentUsername.trim() || (Boolean(parentId) && !parentNode)">{{ t('admin.agents.create') }}</button></form></section>
      </DetailModal>
      <DetailModal class="agent-dialog" v-if="canViewAgents && !!selected && detailOpen" :title="t('admin.copy.01db40da')" :close-label="t('admin.copy.2bb2cb1d')" @close="closeDetail()">
<p v-if="error" class="am-message is-error" role="alert">{{ localizedAgentMessage(error) }}</p>
<section v-if="canViewAgents && selected" class="am-card"><div class="am-card-head"><div><h3>{{ t('admin.copy.3af6ab5e') }}</h3><p><code><AssociationName :record="selected" kind="agent" :id="selected.id" :locale="associationLocale" /></code> {{ t('admin.copy.86d0a04d') }} <AssociationName :record="selected" kind="agent" :id="selected.id" :locale="associationLocale" /></p></div><button type="button" class="am-secondary button button-secondary" @click="closeDetail">{{ t('admin.copy.61fb1fee') }}</button></div>
        <p class="am-note">{{ t('admin.copy.176c3806') }}<AssociationName v-if="selected.parent_id" :record="selected" kind="agent" :id="selected.parent_id" :locale="associationLocale" /><span v-else>{{ t('admin.copy.b0760966') }}</span>。{{ t('admin.copy.9174910c') }}{{ selected.effective_mode === "loss" ? t('admin.copy.a13e79ef') : t('admin.copy.89fd862d') }}{{ selected.mode_source_agent_id ? " · " + t('admin.copy.d2089b75') + " " + associationLabel(selected, 'agent', selected.mode_source_agent_id, associationLocale) : " · " + t('admin.copy.a70308e4') }}。</p>
        <p v-if="selected.parent_id && !selectedParentReady" class="am-message is-error" role="status">{{ t('admin.copy.c202b758') }}</p>
        <dl v-if="!canWriteAgents" class="am-facts"><div><dt>{{ t('admin.copy.94fca494') }}</dt><dd>{{ modeLabel(selected.parent_id ? selected.config.mode : selected.effective_mode) }}</dd></div><div><dt>{{ t(selected.effective_mode === 'loss' ? 'admin.agents.lossRatio' : 'admin.agents.turnoverRatio') }}</dt><dd>{{ agentRatioToPercentage(selected.config.ratio) }}%</dd></div><div><dt>{{ t('admin.copy.054393be') }}</dt><dd>{{ selected.config.status === "active" ? t('admin.copy.98d24810') : t('admin.copy.5e3326db') }}</dd></div><div><dt>{{ t('admin.copy.59e9ba82') }}</dt><dd>{{ selected.config.can_create_children ? t('admin.copy.1a264f45') : t('admin.copy.b92bb6e0') }}</dd></div><div><dt>{{ t('admin.copy.ac22afab') }}</dt><dd>{{ selected.depth }}</dd></div></dl>
        <p v-if="selected.pending_change" class="am-note" role="status">{{ t('admin.agents.scheduled', { mode: modeLabel(selected.pending_change.mode), ratio: agentRatioToPercentage(selected.pending_change.ratio) + "%", time: displayDate(selected.pending_change.effective_at) }) }}</p>
        <form v-write-guard="writing" novalidate v-inline-validation v-if="canWriteAgents" class="am-form" @submit.prevent="withGlobalLoading(prepareNode)"><fieldset :disabled="writing || !!pending || Boolean(selected.pending_change)" class="am-form am-edit-fields">
          <label>{{ t('admin.copy.61c7d845') }}<input :value="associationLabel(selected, 'agent', selected.id, associationLocale)" readonly :aria-label="t('admin.copy.61c7d845')"></label><label>{{ t('admin.copy.94fca494') }}<select v-model="form.mode" :aria-label="t('admin.copy.94fca494')" :required="isFirstLevel"><option value="" :disabled="isFirstLevel">{{ isFirstLevel ? t('admin.agents.selectMode') : t('admin.copy.b8bd227c') }}</option><option v-for="mode in allowedModes" :key="mode" :value="mode">{{ mode === "loss" ? t('admin.copy.a13e79ef') : t('admin.copy.89fd862d') }}</option><option v-if="form.mode && !allowedModes.includes(form.mode)" :value="form.mode" disabled>{{ t('admin.copy.7a1e57b8') }}{{ form.mode === "loss" ? t('admin.copy.a13e79ef') : t('admin.copy.89fd862d') }}</option></select><small v-if="!isFirstLevel">{{ t('admin.copy.42f30b77') }}</small></label><AgentRatioField v-if="draftMode==='loss'" v-model="form.loss_ratio" :label="t('admin.agents.lossRatio')" :help="t('admin.copy.5f569342')" />
        <AgentRatioField v-if="draftMode==='turnover'" v-model="form.turnover_ratio" :label="t('admin.agents.turnoverRatio')" :help="t('admin.copy.5f569342')" /><label>{{ t('admin.copy.054393be') }}<select v-model="form.status" :disabled="changingMode" :aria-label="t('admin.copy.054393be')"><option value="active">{{ t('admin.copy.5eb01613') }}</option><option value="disabled">{{ t('admin.copy.5e3326db') }}</option></select></label><label class="am-check"><input v-model="form.can_create_children" :disabled="changingMode" type="checkbox" :aria-label="t('admin.copy.a78c6c73')">{{ t('admin.copy.a78c6c73') }}</label>
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
          <p v-if="draftMode && draftRatio && !nodeRatioValid" class="am-field-error">{{ t('admin.copy.379b3171') }}</p><div class="am-actions"><button v-if="pending && (pending.operation === 'update' || pending.operation === 'mode-change')" v-write-guard="writing" class="button button-primary" type="button" :disabled="writing" @click="withGlobalLoading(retryFrozen)">{{ t('admin.settings.retryOperation') }}</button><button v-else class="am-primary button button-primary" type="submit" :disabled="writing || !!pending || Boolean(selected.pending_change) || (changingMode && !modeLineReady) || !nodeRatioValid || (Boolean(selected.parent_id) && !selectedParentReady)">{{ t('admin.copy.ea2d7203') }}</button><button type="button" class="am-secondary button button-secondary" @click="withGlobalLoading(loadNodeHistory)">{{ t('admin.copy.2460ea6a') }}</button></div></form>
        <button v-if="!canWriteAgents" type="button" class="am-secondary button button-secondary" @click="withGlobalLoading(loadNodeHistory)">{{ t('admin.copy.2460ea6a') }}</button>
        <div v-if="showNodeHistory" class="am-history"><h4>{{ t('admin.copy.489686f7') }}</h4><article v-for="item in history" :key="item.version + ':' + item.created_at" class="am-revision"><span>{{ displayDate(item.created_at) }} · {{ item.actor_type === "system" ? t('admin.copy.532e2e04') : item.actor_type === "user" ? t('admin.copy.689b5703') : t('admin.copy.37b47e95') }}</span><BusinessDetails :value="configText(item.config)" :locale="associationLocale" /><span v-if="item.reason">{{ t('admin.copy.2809a1e6') }}{{ item.reason }}</span></article></div>
      </section>
        <JoinCodeManagement v-if="selected && joinCodePermissions(account, brandId).view" :key="selected.id" :account="account" :brand-id="brandId" :member-id="selected.agent_account_id" :agent-id="selected.id" kind="agent" :owner-record="selected" @session-invalid="emit('session-invalid')" />
</DetailModal>
      <div v-if="pending?.operation === 'create' && !createOpen" class="am-actions"><button v-write-guard="writing" class="button button-primary" type="button" :disabled="writing || !canWriteAgents" @click="withGlobalLoading(retryFrozen)">{{ t('admin.agents.retryCreate') }}</button></div>
      <div v-if="pending?.operation === 'policy'" class="am-actions"><button v-write-guard="writing" class="button button-primary" type="button" :disabled="writing || !canWritePolicy" @click="withGlobalLoading(retryFrozen)">{{ t('admin.settings.retryOperation') }}</button></div>
      <div v-if="pending && (pending.operation === 'update' || pending.operation === 'mode-change') && !detailOpen" class="am-actions"><button v-write-guard="writing" class="button button-primary" type="button" :disabled="writing || !canWriteAgents" @click="withGlobalLoading(retryFrozen)">{{ t('admin.settings.retryOperation') }}</button></div>
    </template>
  </section>
</template>

<style scoped>
.agent-management{width:100%;min-width:0;color:var(--ink)}.am-head,.am-card-head{display:flex;justify-content:space-between;align-items:flex-start;gap:14px}.am-head{margin:4px 0 16px}.am-head h2{font-size:20px;margin:3px 0 5px}.am-head p,.am-card-head p{margin:0;color:var(--muted);font-size:13px;line-height:1.55;overflow-wrap:anywhere}.am-eyebrow{font-size:12px!important;letter-spacing:1.1px;color:var(--muted-light)!important;font-weight:700}.am-brand,.am-pending{border-radius:20px;background:var(--canvas);padding:8px 12px;color:var(--muted);font-size:13px;white-space:nowrap}.am-note,.am-message{border-radius:8px;background:var(--canvas);color:var(--muted);padding:11px 13px;font-size:13px;line-height:1.6;overflow-wrap:anywhere;margin:12px 0}.am-message.is-error{background:var(--danger-bg);color:var(--danger)}.am-message.is-ok{background:var(--primary-light);color:var(--primary-dark)}.am-card{background:var(--surface);border:1px solid var(--line);border-radius:11px;padding:16px;min-width:0;box-shadow:var(--shadow-sm);margin:13px 0}.am-card h3{font-size:16px;margin:0 0 12px}.am-card-head{margin-bottom:14px}.am-card-head h3{margin:0 0 4px}.am-card-head h2{margin:0 0 4px}.am-facts{display:grid;grid-template-columns:repeat(auto-fit,minmax(145px,1fr));gap:10px;margin:0}.am-facts div{border:1px solid var(--line);border-radius:8px;padding:12px;min-width:0}.am-facts dt{font-size:13px;color:var(--muted)}.am-facts + .am-actions{margin-top:12px}.am-ratio-input{display:flex;align-items:center;gap:8px;min-width:0}.am-ratio-input input{flex:1 1 0;min-width:0}.am-ratio-input > span{flex:none;color:var(--muted)}.am-facts dd{font-size:16px;font-weight:700;margin:6px 0 0;overflow-wrap:anywhere;font-variant-numeric:tabular-nums}.am-meta{font-size:13px;color:var(--muted);overflow-wrap:anywhere}.am-form{display:grid;grid-template-columns:minmax(0,1fr);gap:13px}.am-form label{display:grid;gap:6px;font-size:13px;font-weight:600;min-width:0}.am-form input:not([type=checkbox]),.am-form select,.am-form textarea{box-sizing:border-box;min-width:0}.am-form textarea{width:100%}.am-form small{font-size:12px;font-weight:400;color:var(--muted);line-height:1.5}.am-form .am-check,.am-check{display:flex;align-items:center;gap:9px;min-height:44px;font-size:14px}.am-check input{width:18px;height:18px;flex:none}.am-form textarea{resize:vertical}.am-field-error{grid-column:1/-1;color:var(--danger);font-size:13px;margin:0}.am-actions{display:flex;gap:8px;flex-wrap:wrap;align-items:center}.am-node-list{display:grid;gap:9px}.am-node{display:flex;justify-content:space-between;gap:12px;border:1px solid var(--line);border-radius:9px;padding:12px;min-width:0}.am-node-main{display:grid;gap:4px;min-width:0}.am-node-main strong{font-size:15px;overflow-wrap:anywhere}.am-node-main span,.am-node-main small{font-size:13px;color:var(--muted);overflow-wrap:anywhere}.am-node code,.am-card code,.am-meta code{overflow-wrap:anywhere;word-break:break-word}.am-pager{display:flex;justify-content:center;align-items:center;gap:12px;margin-top:13px;flex-wrap:wrap}.am-pager span{font-size:13px;color:var(--muted)}.am-history{display:grid;gap:9px;margin-top:16px}.am-history h4{margin:0;font-size:15px}.am-revision{display:grid;gap:6px;background:var(--canvas);border:1px solid var(--line);border-radius:8px;padding:11px;min-width:0;font-size:13px;overflow-wrap:anywhere}.am-revision span{color:var(--muted)}.am-revision code{white-space:pre-wrap;overflow-wrap:anywhere;word-break:break-word}.am-review{border-color:var(--primary)}.am-json{white-space:pre-wrap;overflow-wrap:anywhere;word-break:break-word;max-height:320px;overflow:auto;background:var(--canvas);border-radius:8px;padding:12px;font-size:13px;line-height:1.5}.am-confirm{margin:12px 0;font-size:14px}
@media(max-width:640px){.am-head,.am-card-head,.am-node{flex-direction:column}.am-form{grid-template-columns:minmax(0,1fr)}.am-brand{white-space:normal}.am-actions{width:100%}.am-actions button{flex:1}.am-card{padding:14px}.am-facts{grid-template-columns:repeat(2,minmax(0,1fr))}}

.agent-management .agent-dialog { width:min(520px,calc(100vw - 24px)); }
.am-edit-fields{border:0;padding:0;margin:0;min-width:0}.am-mode-line{display:grid;gap:12px;min-width:0}
.agent-dialog .am-form { grid-template-columns:minmax(0,1fr); }
.agent-dialog .am-form > label { justify-items:start; }
.agent-dialog .am-facts { grid-template-columns:minmax(0,1fr); }
.agent-dialog .am-card { margin:0; padding:0; border:0; box-shadow:none; }
@media(max-width:900px),(pointer:coarse){.am-form input:not([type=checkbox]){width:100%}}
</style>
