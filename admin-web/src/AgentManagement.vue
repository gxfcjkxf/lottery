<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { agentsPermissions, createAgentsApi, type AgentNode, type AgentPolicy, type CreateAgentBody, type NodeConfig, type PolicyConfig, type Revision, type SaveAgentPolicyBody, type UpdateAgentBody } from "./agents-api";
import { allowedChildModes, classifyAgentFailure, clearAllPendingAgentWrites, clearPendingAgentWrite, freezeAgentBody, getPendingAgentWrite, parentModeVerified, setPendingAgentWrite, type AgentWriteOperation, type AgentWriteBody, type PendingAgentWrite } from "./agents-state";
import { useAdminI18n } from "./i18n";

type Mode = "loss" | "turnover";
const props = defineProps<{ account: AdminAccount; brandId: string }>();
const { t } = useAdminI18n();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createAgentsApi();
const permissionSet = computed(() => agentsPermissions(props.account, props.brandId));
const canViewPolicy = computed(() => permissionSet.value.policyView);
const canViewAgents = computed(() => permissionSet.value.view);
const canWritePolicy = computed(() => permissionSet.value.policyWrite);
const canWriteAgents = computed(() => permissionSet.value.write);
const canView = computed(() => canViewPolicy.value || canViewAgents.value);
const canWrite = computed(() => canWritePolicy.value || canWriteAgents.value);
const permissionSignature = computed(() => JSON.stringify({
  accountId: props.account.id,
  brandId: props.brandId,
  superAdmin: props.account.super_admin,
  brandIds: [...props.account.brand_ids].sort(),
  brandPermissions: [...(props.account.permissions_by_brand?.[props.brandId] ?? props.account.permissions ?? [])].sort(),
  platformPermissions: [...(props.account.platform_permissions ?? [])].sort(),
}));
const policy = ref<AgentPolicy | null>(null);
const policyDraft = ref<PolicyConfig>({ enabled: false, max_depth: 5, ratio_cap: "0", mode: "loss", cycle: "monthly" });
const policyReason = ref("");
const nodes = ref<AgentNode[]>([]);
const total = ref("0");
const offset = ref(0);
const parentId = ref<string | null>(null);
const parentNode = ref<AgentNode | null>(null);
const selected = ref<AgentNode | null>(null);
const selectedParentMode = ref<Mode | null>(null);
const selectedParentReady = ref(false);
const form = ref({ member_id: "", ratio: "0", mode: "" as "" | Mode, status: "active" as "active" | "disabled", can_create_children: false, reason: "" });
const history = ref<Revision[]>([]);
const policyHistory = ref<Revision[]>([]);
const policyHistoryTotal = ref("0");
const policyHistoryOffset = ref(0);
const policyHistoryShown = ref(false);
const showNodeHistory = ref(false);
const loading = ref(false);
const writing = ref(false);
const error = ref("");
const notice = ref("");
const review = ref(false);
const pending = ref<PendingAgentWrite | null>(null);
let generation = 0;
let writeGeneration = 0;
let disposed = false;
let activeAccountId = props.account.id;
const pageSize = 20;
const ratioValid = (s: string) => /^(?:0|1|0\.(?=.{1,6}$)0*[1-9](?:[0-9]*[1-9])?)$/.test(s);
const policyRatioValid = computed(() => ratioValid(policyDraft.value.ratio_cap));
const nodeRatioValid = computed(() => ratioValid(form.value.ratio));
const nodeParentMode = computed<Mode | null | undefined>(() => selected.value ? (selected.value.parent_id ? (selectedParentReady.value ? selectedParentMode.value ?? undefined : undefined) : policy.value?.config.mode) : (parentNode.value ? parentNode.value.effective_mode : parentId.value ? undefined : null));
const allowedModes = computed(() => selected.value?.parent_id === null || (!selected.value && !parentId.value) ? ["loss", "turnover"] as Mode[] : allowedChildModes(nodeParentMode.value));
const sameScope = (g: number, brand: string, account: string) => g === generation && brand === props.brandId && account === props.account.id;
function currentWrite(ticket: number, accountId: string, brandId: string, permissions: string, operation: AgentWriteOperation) {
  return !disposed && ticket === writeGeneration && props.account.id === accountId && props.brandId === brandId &&
    permissionSignature.value === permissions && (operation === "policy" ? canWritePolicy.value : canWriteAgents.value);
}

function restoreIntent() { pending.value = getPendingAgentWrite({ accountId: props.account.id, brandId: props.brandId }); review.value = false; }
function handleError(cause: unknown) {
  if (cause instanceof AdminApiError && cause.status === 401) { clearAllPendingAgentWrites(); pending.value = null; emit("session-invalid"); }
  error.value = cause instanceof Error ? cause.message : "请求失败";
  if (cause instanceof AdminApiError && cause.code) error.value = cause.code + ": " + error.value;
}
async function loadPolicy(g = ++generation, brand = props.brandId, account = props.account.id) {
  if (!canViewPolicy.value || !brand) { policy.value = null; return; }
  try {
    const result = await api.policy(brand);
    if (!sameScope(g, brand, account)) return;
    if (result.brand_id !== brand) throw new Error("策略响应与当前品牌不匹配。");
    policy.value = result; policyDraft.value = { ...result.config }; policyReason.value = "";
  } catch (cause) { if (sameScope(g, brand, account)) handleError(cause); }
}
async function loadTree(g = ++generation, brand = props.brandId, account = props.account.id) {
  if (!canViewAgents.value || !brand) { nodes.value = []; return; }
  loading.value = true; error.value = ""; nodes.value = []; total.value = "0"; parentNode.value = null;
  try {
    const [result, parent] = await Promise.all([api.tree(brand, parentId.value, pageSize, offset.value), parentId.value ? api.node(brand, parentId.value) : Promise.resolve(null)]);
    if (!sameScope(g, brand, account)) return;
    if (result.brand_id !== brand || (result.parent_id ?? null) !== parentId.value) throw new Error("代理目录响应与当前查询范围不匹配。");
    nodes.value = result.items; total.value = result.total_count; parentNode.value = parent; restoreIntent();
  } catch (cause) { if (sameScope(g, brand, account)) handleError(cause); }
  finally { if (sameScope(g, brand, account)) loading.value = false; }
}
async function loadAll() { const g = ++generation; const b = props.brandId; const a = props.account.id; await Promise.all([loadPolicy(g, b, a), loadTree(g, b, a)]); }
async function selectNode(n: AgentNode) {
  const b = props.brandId, a = props.account.id, g = ++generation;
  error.value = "";
  try {
    const detail = await api.node(b, n.id);
    if (!sameScope(g, b, a)) return;
    if (detail.id !== n.id || detail.brand_id !== b) throw new Error("代理详情响应与当前代理或品牌不匹配。");
    selected.value = detail;
    form.value = { member_id: detail.member_id, ratio: detail.config.ratio, mode: detail.config.mode ?? "", status: detail.config.status, can_create_children: detail.config.can_create_children, reason: "" };
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
function closeDetail() { selected.value = null; selectedParentMode.value = null; selectedParentReady.value = false; form.value = { member_id: "", ratio: "0", mode: "", status: "active", can_create_children: false, reason: "" }; restoreIntent(); }
function startCreate() { closeDetail(); }
async function openChildren(n: AgentNode) { parentId.value = n.id; offset.value = 0; closeDetail(); await loadTree(); }
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
async function loadPolicyHistory() {
  const b = props.brandId, a = props.account.id, g = ++generation;
  if (!canViewPolicy.value) return;
  try { const r = await api.policyHistory(b, pageSize, policyHistoryOffset.value); if (!sameScope(g, b, a)) return; if (r.brand_id !== b || r.agent_id !== null) throw new Error("政策历史响应范围不匹配。"); policyHistory.value = r.items; policyHistoryTotal.value = r.total_count; policyHistoryShown.value = true; }
  catch (cause) { if (sameScope(g, b, a)) handleError(cause); }
}
function freeze(operation: AgentWriteOperation, resourceId: string | null, body: AgentWriteBody) {
  const scope = { accountId: props.account.id, brandId: props.brandId };
  const old = getPendingAgentWrite(scope);
  if (old && (old.operation !== operation || old.resourceId !== resourceId || JSON.stringify(old.body) !== JSON.stringify(body))) {
    pending.value = old;
    error.value = "此账号和品牌已有未确认结果的冻结请求；请先核对并按原键重试。";
    return;
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
  review.value = true;
  notice.value = "请核对冻结请求内容后确认提交。";
}
function preparePolicy() {
  if (!policy.value || !canWritePolicy.value || !policyRatioValid.value || !policyReason.value.trim() || !Number.isInteger(policyDraft.value.max_depth) || policyDraft.value.max_depth < 1 || policyDraft.value.max_depth > 32) return;
  const body: SaveAgentPolicyBody = { version: policy.value.version, config: { ...policyDraft.value }, reason: policyReason.value.trim() };
  freeze("policy", null, body);
}
function prepareNode() {
  if (!policy.value || !canWriteAgents.value || !nodeRatioValid.value || !form.value.reason.trim()) return;
  const parentIdForWrite = selected.value?.parent_id ?? parentNode.value?.id ?? null;
  if (!parentModeVerified(parentIdForWrite, selected.value ? selectedParentReady.value : Boolean(parentNode.value) || !parentId.value)) { error.value = "父级生效模式尚未验证，当前不能创建或提交此代理变更。请重新读取父级或代理详情。"; return; }
  if (form.value.mode && !allowedModes.value.includes(form.value.mode)) { error.value = "代理模式必须沿用父级生效模式或与其保持一致；请先读取父级模式。"; return; }
  const config: NodeConfig = { ratio: form.value.ratio, mode: form.value.mode || null, status: form.value.status, can_create_children: form.value.can_create_children };
  if (selected.value) {
    const n = selected.value;
    const pVersion = n.parent_id ? (parentNode.value?.id === n.parent_id ? parentNode.value.version : n.parent_version) : null;
    const body: UpdateAgentBody = { version: n.version, policy_version: policy.value.version, parent_version: pVersion, config, reason: form.value.reason.trim() };
    freeze("update", n.id, body);
  } else {
    if (parentId.value && !parentNode.value) { error.value = "父级代理尚未读取，无法确认其版本。"; return; }
    const memberId = form.value.member_id.trim();
    const body: CreateAgentBody = { policy_version: policy.value.version, member_id: memberId, parent_id: parentNode.value?.id ?? null, parent_version: parentNode.value?.version ?? null, config, reason: form.value.reason.trim() };
    freeze("create", null, body);
  }
}
async function confirmWrite() {
  const i = pending.value;
  if (!i || writing.value || !review.value || i.accountId !== props.account.id || i.brandId !== props.brandId || (i.operation === "policy" ? !canWritePolicy.value : !canWriteAgents.value)) return;
  const ticket = ++writeGeneration;
  const accountId = i.accountId;
  const brandId = i.brandId;
  const permissions = permissionSignature.value;
  const isCurrentWrite = () => currentWrite(ticket, accountId, brandId, permissions, i.operation);
  const sharedScope = { accountId, brandId };
  writing.value = true; error.value = ""; notice.value = "";
  try {
    let receipt: AgentPolicy | AgentNode;
    if (i.operation === "policy") receipt = await api.savePolicy(i.brandId, i.body as SaveAgentPolicyBody, i.key);
    else if (i.operation === "create") receipt = await api.createNode(i.brandId, i.body as CreateAgentBody, i.key);
    else receipt = await api.updateNode(i.brandId, i.resourceId!, i.body as UpdateAgentBody, i.key);
    // A validated SDK receipt acknowledges only this frozen key; it cannot clear a replacement intent.
    clearPendingAgentWrite(sharedScope, i.key);
    if (!isCurrentWrite()) return;
    pending.value = getPendingAgentWrite(sharedScope);
    notice.value = "已收到与冻结请求匹配的成功回执；正在独立读取当前状态。";
    if (i.operation === "policy") await loadPolicy();
    else { await selectNode(receipt as AgentNode); if (!isCurrentWrite()) return; await loadTree(); }
    if (isCurrentWrite()) notice.value = "写入已确认；页面显示的是随后读取的当前状态。";
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
      review.value = true;
      notice.value = "写入结果未知。冻结请求和幂等键已保留，只能按原请求重试。";
      error.value = cause instanceof Error ? cause.message : "请求结果未知。";
    } else {
      clearPendingAgentWrite(sharedScope, i.key);
      pending.value = getPendingAgentWrite(sharedScope);
      error.value = cause instanceof Error ? cause.message : "服务器拒绝了请求。";
      if (cause instanceof AdminApiError && cause.code) error.value = cause.code + ": " + error.value;
    }
  } finally { if (isCurrentWrite()) writing.value = false; }
}
function retryFrozen() { review.value = true; void confirmWrite(); }
function configText(c: PolicyConfig | NodeConfig) { return JSON.stringify(c, null, 2); }
function modeLabel(value: string | null | undefined) { return value === "loss" ? t("输赢", "Loss") : value === "turnover" ? t("流水", "Turnover") : t("继承", "Inherit"); }
function pendingSummary(i: PendingAgentWrite) {
  if (i.operation === "policy") return t("保存代理政策 · 品牌政策", "Save agent policy · brand policy");
  if (i.operation === "create") return t("创建代理 · 目标父级", "Create agent · target parent") + " " + ((i.body as CreateAgentBody).parent_id ?? t("根节点", "Root")) + " · " + t("会员", "Member") + " " + (i.body as CreateAgentBody).member_id;
  return t("保存代理变更 · 代理", "Save agent change · agent") + " " + i.resourceId;
}
function localizedAgentMessage(value: string): string {
  const translations: Record<string, [string, string]> = {
    "此账号和品牌已有未确认结果的冻结请求；请先核对并按原键重试。": ["此账号和品牌已有未确认结果的冻结请求；请先核对并按原键重试。", "An unresolved frozen request already exists for this account and brand. Review it and retry with its original key."],
    "请核对冻结请求内容后确认提交。": ["请核对冻结请求内容后确认提交。", "Review the frozen request content before confirming submission."],
    "父级生效模式尚未验证，当前不能创建或提交此代理变更。请重新读取父级或代理详情。": ["父级生效模式尚未验证，当前不能创建或提交此代理变更。请重新读取父级或代理详情。", "The parent's effective mode has not been verified. Reload the parent or agent details before creating or submitting this change."],
    "代理模式必须沿用父级生效模式或与其保持一致；请先读取父级模式。": ["代理模式必须沿用父级生效模式或与其保持一致；请先读取父级模式。", "The agent mode must inherit or match the parent's effective mode. Load the parent mode first."],
    "父级代理尚未读取，无法确认其版本。": ["父级代理尚未读取，无法确认其版本。", "The parent agent has not been loaded, so its version cannot be confirmed."],
    "已收到与冻结请求匹配的成功回执；正在独立读取当前状态。": ["已收到与冻结请求匹配的成功回执；正在独立读取当前状态。", "The receipt matches the frozen request. Reading current state independently."],
    "写入已确认；页面显示的是随后读取的当前状态。": ["写入已确认；页面显示的是随后读取的当前状态。", "The write was confirmed; the page shows the current state read afterward."],
    "写入结果未知。冻结请求和幂等键已保留，只能按原请求重试。": ["写入结果未知。冻结请求和幂等键已保留，只能按原请求重试。", "The write outcome is unknown. The frozen request and idempotency key are retained; retry only the original request."],
  };
  const pair = translations[value];
  return pair ? t(pair[0], pair[1]) : value;
}
function changePage(d: number) { offset.value = Math.max(0, offset.value + d * pageSize); void loadTree(); }
onMounted(() => { restoreIntent(); void loadAll(); });
onBeforeUnmount(() => { disposed = true; generation++; writeGeneration++; });
watch(() => [props.brandId, props.account.id, permissionSignature.value] as const, () => {
  if (activeAccountId !== props.account.id) {
    clearAllPendingAgentWrites();
    activeAccountId = props.account.id;
  }
  generation++; writeGeneration++; policy.value = null; nodes.value = []; selected.value = null; parentId.value = null; parentNode.value = null; offset.value = 0; error.value = ""; notice.value = ""; loading.value = false; writing.value = false; history.value = []; policyHistory.value = []; policyHistoryShown.value = false; policyHistoryOffset.value = 0; restoreIntent(); if (canView.value) void loadAll();
}, { flush: "sync" });
</script>

<template>
  <section class="agent-management">
    <header class="am-head"><div><p class="am-eyebrow">AGENT CONFIGURATION</p><h2>{{ t("代理管理", "Agent management") }}</h2><p>{{ t("配置代理关系和比例。此页面不计算或发放佣金，也不提供推荐码加入。", "Configure agent relationships and ratios. This page does not calculate or pay commissions and does not support referral-code enrollment.") }}</p></div><span class="am-brand">{{ t("品牌", "Brand") }} · {{ brandId || t("未选择", "None selected") }}</span></header>
    <p class="am-note">{{ t("比例按原始分数显示，例如 0.1 表示 10%。配置只定义上限、模式和层级关系；不代表收益、佣金计算或派发。", "Ratios are shown as decimals: 0.1 means 10%. Configuration defines caps, mode, and hierarchy only; it does not represent earnings, commission calculation, or payout.") }}</p>
    <div v-if="error" class="am-message is-error" role="alert">{{ localizedAgentMessage(error) }}</div><p v-if="notice" class="am-message is-ok" role="status">{{ localizedAgentMessage(notice) }}</p>
    <p v-if="!canView" class="am-note">{{ t("当前账号缺少此品牌的代理查看权限。", "This account does not have agent view access for this brand.") }}</p>
    <template v-else>
      <section v-if="policy" class="am-card"><div class="am-card-head"><div><h3>{{ t("品牌代理政策", "Brand agent policy") }}</h3><p>{{ t("版本", "Version") }} {{ policy.version }} · {{ t("更新于", "Updated") }} {{ policy.updated_at }}</p></div><button type="button" class="am-secondary" @click="loadAll">{{ t("查询", "Reload") }}</button></div>
        <dl class="am-facts"><div><dt>{{ t("代理配置", "Agent configuration") }}</dt><dd>{{ policy.config.enabled ? t("已启用", "Enabled") : t("已停用", "Disabled") }}</dd></div><div><dt>{{ t("最大层级", "Maximum depth") }}</dt><dd>{{ policy.config.max_depth }}</dd></div><div><dt>{{ t("品牌比例上限", "Brand ratio cap") }}</dt><dd>{{ policy.config.ratio_cap }}</dd></div><div><dt>{{ t("品牌佣金模式", "Brand commission mode") }}</dt><dd>{{ policy.config.mode === "loss" ? t("输赢", "Loss") : t("流水", "Turnover") }}</dd></div><div><dt>{{ t("周期", "Cycle") }}</dt><dd>{{ policy.config.cycle === "weekly" ? t("每周", "Weekly") : t("每月", "Monthly") }}</dd></div></dl>
        <p v-if="policy.audit_log_id" class="am-meta">{{ t("审计记录：", "Audit record: ") }}{{ policy.audit_log_id }}</p>
      </section>
      <section v-if="canWritePolicy && policy" class="am-card"><h3>{{ t("编辑品牌政策", "Edit brand policy") }}</h3><form class="am-form" @submit.prevent="preparePolicy">
        <label class="am-check"><input v-model="policyDraft.enabled" type="checkbox" :aria-label="t('代理管理启用', 'Enable agent management')">{{ t("代理管理启用", "Enable agent management") }}</label>
        <label>{{ t("代理最大层级", "Maximum agent depth") }}<input v-model.number="policyDraft.max_depth" type="number" min="1" max="32" step="1" :aria-label="t('代理最大层级', 'Maximum agent depth')"></label>
        <label>{{ t("品牌比例上限", "Brand ratio cap") }}<input v-model.trim="policyDraft.ratio_cap" inputmode="decimal" autocomplete="off" :aria-label="t('品牌比例上限', 'Brand ratio cap')"><small>{{ t("原始分数 0–1，最多 6 位小数；0.1 表示 10%。", "Decimal from 0 to 1, up to 6 decimal places; 0.1 means 10%.") }}</small></label>
        <label>{{ t("品牌佣金模式", "Brand commission mode") }}<select v-model="policyDraft.mode" :aria-label="t('品牌佣金模式', 'Brand commission mode')"><option value="loss">{{ t("输赢", "Loss") }}</option><option value="turnover">{{ t("流水", "Turnover") }}</option></select></label>
        <label>{{ t("佣金周期", "Commission cycle") }}<select v-model="policyDraft.cycle" :aria-label="t('佣金周期', 'Commission cycle')"><option value="weekly">{{ t("每周", "Weekly") }}</option><option value="monthly">{{ t("每月", "Monthly") }}</option></select></label>
        <label>{{ t("代理政策变更原因", "Agent policy change reason") }}<textarea v-model.trim="policyReason" rows="3" maxlength="500" required :aria-label="t('代理政策变更原因', 'Agent policy change reason')"></textarea></label>
        <p v-if="!policyRatioValid" class="am-field-error">{{ t("请输入规范比例：0、1，或不带末尾零的最多 6 位小数。", "Enter a canonical ratio: 0, 1, or up to 6 decimals without trailing zeros.") }}</p><button class="am-primary" type="submit" :disabled="writing || !policyRatioValid || !policyReason.trim()">{{ t("核对并保存", "Review and save") }}</button>
      </form></section><p v-else-if="policy && canViewPolicy" class="am-note">{{ t("当前账号仅有读取权限，或属于只读超级管理员；不能修改代理配置。", "This account is read-only or is a read-only super admin; agent configuration cannot be changed.") }}</p>
      <section v-if="canViewAgents" class="am-card"><div class="am-card-head"><div><h3>{{ t("代理树", "Agent tree") }}</h3><p>{{ parentId ? t("当前父级", "Current parent") + " " + parentId : t("当前范围：根代理", "Current scope: root agents") }} · {{ t("共", "Total") }} {{ total }}</p></div><div class="am-actions"><button v-if="parentId" type="button" class="am-secondary" :disabled="loading" @click="goBack">{{ t("返回上级", "Back to parent") }}</button><button v-if="canWriteAgents && policy" type="button" class="am-secondary" @click="startCreate">{{ t("新建代理", "Create agent") }}</button></div></div>
        <div v-if="loading" class="am-note" role="status">{{ t("正在读取代理目录…", "Loading agent directory…") }}</div><div class="am-node-list">
          <article v-for="node in nodes" :key="node.id" class="am-node"><div class="am-node-main"><strong>{{ node.member_id }}</strong><span>{{ t("代理 ID", "Agent ID") }} <code>{{ node.id }}</code></span><span>{{ t("层级", "Depth") }} {{ node.depth }} · {{ t("比例", "Ratio") }} {{ node.config.ratio }} · {{ node.config.status === "active" ? t("启用", "Enabled") : t("停用", "Disabled") }}</span><small>{{ t("生效模式：", "Effective mode: ") }}{{ node.effective_mode === "loss" ? t("输赢", "Loss") : t("流水", "Turnover") }} · {{ t("路径", "Path") }} {{ node.path.join(" › ") }}</small></div><div class="am-actions"><button type="button" class="am-secondary" @click="selectNode(node)">{{ t("详情", "Details") }}</button><button type="button" class="am-secondary" @click="openChildren(node)">{{ t("下级", "Children") }}</button></div></article>
          <p v-if="!loading && nodes.length === 0" class="am-note">{{ t("此范围暂无代理。", "No agents in this scope.") }}</p></div><div class="am-pager"><button type="button" class="am-secondary" :disabled="loading || offset === 0" @click="changePage(-1)">{{ t("上一页", "Previous") }}</button><span>{{ total === "0" ? 0 : offset + 1 }}–{{ Math.min(offset + pageSize, Number(total)) }} / {{ total }}</span><button type="button" class="am-secondary" :disabled="loading || offset + pageSize >= Number(total)" @click="changePage(1)">{{ t("下一页", "Next") }}</button></div>
      </section><p v-else class="am-note">{{ t("当前账号没有代理树查看权限。", "This account does not have agent tree view access.") }}</p>
      <section v-if="canWriteAgents && policy && canViewAgents && !selected" class="am-card"><h3>{{ parentId ? t("创建直属代理", "Create direct agent") : t("创建根代理", "Create root agent") }}</h3><p v-if="parentNode" class="am-note">{{ t("父级", "Parent") }} {{ parentNode.id }} · {{ t("版本", "Version") }} {{ parentNode.version }} · {{ t("层级", "Depth") }} {{ parentNode.depth }} · {{ t("上限由服务端校验。新代理须与父级生效模式一致。", "Limits are validated by the server. A new agent must use the parent's effective mode.") }}</p><p v-else-if="parentId" class="am-note">{{ t("父级模式尚未读取；目前只能选择继承。", "The parent mode has not been loaded; only inheritance is available.") }}</p><form class="am-form" @submit.prevent="prepareNode">
        <label>{{ t("代理会员 UUID", "Agent member UUID") }}<input v-model.trim="form.member_id" autocomplete="off" required :aria-label="t('代理会员 UUID', 'Agent member UUID')"></label><label>{{ t("代理比例", "Agent ratio") }}<input v-model.trim="form.ratio" inputmode="decimal" autocomplete="off" required :aria-label="t('代理比例', 'Agent ratio')"><small>{{ t("原始分数；服务器校验父级和品牌上限。", "Decimal value; the server validates parent and brand caps.") }}</small></label>
        <label>{{ t("代理模式", "Agent mode") }}<select v-model="form.mode" :aria-label="t('代理模式', 'Agent mode')"><option value="">{{ t("继承", "Inherit") }}</option><option v-for="mode in allowedModes" :key="mode" :value="mode">{{ mode === "loss" ? t("输赢", "Loss") : t("流水", "Turnover") }}</option></select></label><label>{{ t("代理状态", "Agent status") }}<select v-model="form.status" :aria-label="t('代理状态', 'Agent status')"><option value="active">{{ t("启用", "Enabled") }}</option><option value="disabled">{{ t("停用", "Disabled") }}</option></select></label><label class="am-check"><input v-model="form.can_create_children" type="checkbox" :aria-label="t('允许发展下级', 'Allow child agents')">{{ t("允许发展下级", "Allow child agents") }}</label><label>{{ t("代理变更原因", "Agent change reason") }}<textarea v-model.trim="form.reason" rows="3" maxlength="500" required :aria-label="t('代理变更原因', 'Agent change reason')"></textarea></label>
        <p v-if="!nodeRatioValid" class="am-field-error">{{ t("请输入规范比例：0、1，或不带末尾零的最多 6 位小数。", "Enter a canonical ratio: 0, 1, or up to 6 decimals without trailing zeros.") }}</p><button class="am-primary" type="submit" :disabled="writing || !nodeRatioValid || !form.member_id.trim() || !form.reason.trim() || (Boolean(parentId) && !parentNode)">{{ t("核对并创建", "Review and create") }}</button></form></section>
      <section v-if="canViewAgents && selected" class="am-card"><div class="am-card-head"><div><h3>{{ t("代理详情与配置", "Agent details and configuration") }}</h3><p><code>{{ selected.id }}</code> · {{ t("成员", "Member") }} {{ selected.member_id }} · {{ t("版本", "Version") }} {{ selected.version }}</p></div><button type="button" class="am-secondary" @click="closeDetail">{{ t("关闭详情", "Close details") }}</button></div>
        <p class="am-note">{{ t("父级关系不可更改。当前父级：", "Parent relationships cannot be changed. Current parent: ") }}{{ selected.parent_id ?? t("根代理", "Root agent") }}。{{ t("有效模式：", "Effective mode: ") }}{{ selected.effective_mode === "loss" ? t("输赢", "Loss") : t("流水", "Turnover") }}{{ selected.mode_source_agent_id ? " · " + t("来源", "Source") + " " + selected.mode_source_agent_id : " · " + t("品牌默认", "Brand default") }}。</p>
        <p v-if="selected.parent_id && !selectedParentReady" class="am-message is-error" role="status">{{ t("父级生效模式尚未验证，暂不能提交新的代理变更；请重新读取代理详情。", "The parent's effective mode has not been verified. Agent changes cannot be submitted yet; reload the agent details.") }}</p>
        <dl v-if="!canWriteAgents" class="am-facts"><div><dt>{{ t("代理比例", "Agent ratio") }}</dt><dd>{{ selected.config.ratio }}</dd></div><div><dt>{{ t("代理模式", "Agent mode") }}</dt><dd>{{ modeLabel(selected.config.mode) }}</dd></div><div><dt>{{ t("代理状态", "Agent status") }}</dt><dd>{{ selected.config.status === "active" ? t("启用", "Active") : t("停用", "Disabled") }}</dd></div><div><dt>{{ t("允许发展下级", "Can create child agents") }}</dt><dd>{{ selected.config.can_create_children ? t("是", "Yes") : t("否", "No") }}</dd></div><div><dt>{{ t("层级", "Depth") }}</dt><dd>{{ selected.depth }}</dd></div></dl>
        <form v-if="canWriteAgents" class="am-form" @submit.prevent="prepareNode">
          <label>{{ t("代理会员 UUID", "Agent member UUID") }}<input :value="form.member_id" readonly :aria-label="t('代理会员 UUID', 'Agent member UUID')"></label><label>{{ t("代理比例", "Agent ratio") }}<input v-model.trim="form.ratio" inputmode="decimal" autocomplete="off" required :aria-label="t('代理比例', 'Agent ratio')"></label><label>{{ t("代理模式", "Agent mode") }}<select v-model="form.mode" :aria-label="t('代理模式', 'Agent mode')"><option value="">{{ t("继承", "Inherit") }}</option><option v-for="mode in allowedModes" :key="mode" :value="mode">{{ mode === "loss" ? t("输赢", "Loss") : t("流水", "Turnover") }}</option><option v-if="form.mode && !allowedModes.includes(form.mode)" :value="form.mode" disabled>{{ t("历史模式（仅查看）：", "Historical mode (view only): ") }}{{ form.mode === "loss" ? t("输赢", "Loss") : t("流水", "Turnover") }}</option></select><small>{{ t("下级必须与父级生效模式一致；沿用即继承该模式。", "A child must use the parent's effective mode; inheriting keeps that mode.") }}</small></label><label>{{ t("代理状态", "Agent status") }}<select v-model="form.status" :aria-label="t('代理状态', 'Agent status')"><option value="active">{{ t("启用", "Enabled") }}</option><option value="disabled">{{ t("停用", "Disabled") }}</option></select></label><label class="am-check"><input v-model="form.can_create_children" type="checkbox" :aria-label="t('允许发展下级', 'Allow child agents')">{{ t("允许发展下级", "Allow child agents") }}</label><label>{{ t("代理变更原因", "Agent change reason") }}<textarea v-model.trim="form.reason" rows="3" maxlength="500" required :aria-label="t('代理变更原因', 'Agent change reason')"></textarea></label>
          <p v-if="!nodeRatioValid" class="am-field-error">{{ t("请输入规范比例：0、1，或不带末尾零的最多 6 位小数。", "Enter a canonical ratio: 0, 1, or up to 6 decimals without trailing zeros.") }}</p><div class="am-actions"><button class="am-primary" type="submit" :disabled="writing || !nodeRatioValid || !form.reason.trim() || (Boolean(selected.parent_id) && !selectedParentReady)">{{ t("核对并保存", "Review and save") }}</button><button type="button" class="am-secondary" @click="loadNodeHistory">{{ t("查看变更历史", "View change history") }}</button></div></form>
        <button v-if="!canWriteAgents" type="button" class="am-secondary" @click="loadNodeHistory">{{ t("查看变更历史", "View change history") }}</button>
        <div v-if="showNodeHistory" class="am-history"><h4>{{ t("代理变更历史", "Agent change history") }}</h4><article v-for="item in history" :key="item.version + ':' + item.created_at" class="am-revision"><strong>{{ t("版本", "Version") }} {{ item.version }} · {{ item.actor_type }}</strong><span>{{ item.created_at }} · {{ item.actor_id ?? t("系统", "System") }}</span><code>{{ configText(item.config) }}</code><span>{{ t("原因：", "Reason: ") }}{{ item.reason }}</span><span v-if="item.audit_log_id">{{ t("审计", "Audit") }} {{ item.audit_log_id }}</span></article></div>
      </section>
      <section v-if="canViewPolicy" class="am-card"><div class="am-card-head"><div><h3>{{ t("政策变更历史", "Policy change history") }}</h3><p>{{ t("不可变修订记录", "Immutable revision records") }}</p></div><button class="am-secondary" type="button" @click="loadPolicyHistory">{{ policyHistoryShown ? t("刷新历史", "Refresh history") : t("查询历史", "Load history") }}</button></div><div v-if="policyHistoryShown" class="am-history"><article v-for="item in policyHistory" :key="item.version + ':' + item.created_at" class="am-revision"><strong>{{ t("版本", "Version") }} {{ item.version }} · {{ item.actor_type }}</strong><span>{{ item.created_at }} · {{ item.actor_id ?? t("系统", "System") }}</span><code>{{ configText(item.config) }}</code><span>{{ t("原因：", "Reason: ") }}{{ item.reason }}</span><span v-if="item.audit_log_id">{{ t("审计", "Audit") }} {{ item.audit_log_id }}</span></article><p v-if="!policyHistory.length" class="am-note">{{ t("暂无政策历史。", "No policy history.") }}</p><div class="am-pager"><button class="am-secondary" type="button" :disabled="policyHistoryOffset === 0" @click="policyHistoryOffset = Math.max(0, policyHistoryOffset - pageSize); loadPolicyHistory()">{{ t("上一页", "Previous") }}</button><span>{{ policyHistoryTotal }} {{ t("条", "records") }}</span><button class="am-secondary" type="button" :disabled="policyHistoryOffset + pageSize >= Number(policyHistoryTotal)" @click="policyHistoryOffset += pageSize; loadPolicyHistory()">{{ t("下一页", "Next") }}</button></div></div></section>
      <section v-if="pending && pending.accountId === account.id && pending.brandId === brandId" class="am-card am-review"><div class="am-card-head"><div><h3>{{ t("核对并确认写入", "Review and confirm write") }}</h3><p>{{ pendingSummary(pending) }}</p></div><span class="am-pending">{{ t("冻结请求", "Frozen request") }}</span></div><p class="am-note">{{ t("范围：账号", "Scope: account") }} {{ pending.accountId }} · {{ t("品牌", "brand") }} {{ pending.brandId }}。{{ t("结果未知时，只能按相同请求体和幂等键重试；读取当前状态不能确认旧写入。", "If the outcome is unknown, retry only with the same request body and idempotency key; reading current state cannot confirm an earlier write.") }}</p><pre class="am-json">{{ JSON.stringify(pending.body, null, 2) }}</pre><p class="am-meta">Idempotency-Key: <code>{{ pending.key }}</code></p><label class="am-check am-confirm"><input v-model="review" type="checkbox">{{ t("我已核对以上冻结请求", "I reviewed the frozen request above") }}</label><div class="am-actions"><button class="am-primary" type="button" :disabled="!review || writing || (pending.operation === 'policy' ? !canWritePolicy : !canWriteAgents)" @click="confirmWrite">{{ writing ? t("提交中…", "Submitting…") : t("确认", "Confirm") }}</button><button v-if="!writing" class="am-secondary" type="button" @click="retryFrozen">{{ t("按原键重试", "Retry with original key") }}</button><button class="am-secondary" type="button" :disabled="writing" @click="review = false">{{ t("收起核对", "Hide review") }}</button></div></section>
    </template>
  </section>
</template>

<style scoped>
.agent-management{width:100%;min-width:0;color:#252a36}.am-head,.am-card-head{display:flex;justify-content:space-between;align-items:flex-start;gap:14px}.am-head{margin:4px 0 16px}.am-head h2{font-size:20px;margin:3px 0 5px}.am-head p,.am-card-head p{margin:0;color:#737b8c;font-size:13px;line-height:1.55;overflow-wrap:anywhere}.am-eyebrow{font-size:12px!important;letter-spacing:1.1px;color:#8991a2!important;font-weight:700}.am-brand,.am-pending{border-radius:20px;background:#f1f3f8;padding:8px 12px;color:#626b7c;font-size:13px;white-space:nowrap}.am-note,.am-message{border-radius:8px;background:#f4f6fc;color:#626b7c;padding:11px 13px;font-size:13px;line-height:1.6;overflow-wrap:anywhere;margin:12px 0}.am-message.is-error{background:#fff1f0;color:#a12d25}.am-message.is-ok{background:#eef8f3;color:#287554}.am-card{background:#fff;border:1px solid #e9ebf0;border-radius:11px;padding:16px;min-width:0;box-shadow:0 2px 8px #1e2a5008;margin:13px 0}.am-card h3{font-size:16px;margin:0 0 12px}.am-card-head{margin-bottom:14px}.am-card-head h3{margin:0 0 4px}.am-facts{display:grid;grid-template-columns:repeat(auto-fit,minmax(145px,1fr));gap:10px;margin:0}.am-facts div{border:1px solid #eceef3;border-radius:8px;padding:12px;min-width:0}.am-facts dt{font-size:13px;color:#737b8c}.am-facts dd{font-size:16px;font-weight:700;margin:6px 0 0;overflow-wrap:anywhere;font-variant-numeric:tabular-nums}.am-meta{font-size:13px;color:#737b8c;overflow-wrap:anywhere}.am-form{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:13px}.am-form label{display:grid;gap:6px;font-size:13px;font-weight:600;min-width:0}.am-form input:not([type=checkbox]),.am-form select,.am-form textarea{box-sizing:border-box;width:100%;min-width:0;border:1px solid #dfe3eb;border-radius:7px;background:#fff;padding:10px 11px;color:#252a36;font:inherit}.am-form small{font-size:12px;font-weight:400;color:#737b8c;line-height:1.5}.am-form .am-check,.am-check{display:flex;align-items:center;gap:9px;min-height:40px;font-size:14px}.am-check input{width:18px;height:18px;flex:none}.am-form textarea{resize:vertical}.am-field-error{grid-column:1/-1;color:#a12d25;font-size:13px;margin:0}.am-primary,.am-secondary{border-radius:7px;padding:9px 13px;font-size:14px;min-height:40px}.am-primary{border:1px solid #5969df;background:#5969df;color:white}.am-secondary{border:1px solid #dfe3eb;background:#fff;color:#41495a}.am-actions{display:flex;gap:8px;flex-wrap:wrap;align-items:center}.am-node-list{display:grid;gap:9px}.am-node{display:flex;justify-content:space-between;gap:12px;border:1px solid #eceef3;border-radius:9px;padding:12px;min-width:0}.am-node-main{display:grid;gap:4px;min-width:0}.am-node-main strong{font-size:15px;overflow-wrap:anywhere}.am-node-main span,.am-node-main small{font-size:13px;color:#626b7c;overflow-wrap:anywhere}.am-node code,.am-card code,.am-meta code{overflow-wrap:anywhere;word-break:break-word}.am-pager{display:flex;justify-content:center;align-items:center;gap:12px;margin-top:13px;flex-wrap:wrap}.am-pager span{font-size:13px;color:#626b7c}.am-history{display:grid;gap:9px;margin-top:16px}.am-history h4{margin:0;font-size:15px}.am-revision{display:grid;gap:6px;background:#f9fafc;border:1px solid #eceef3;border-radius:8px;padding:11px;min-width:0;font-size:13px;overflow-wrap:anywhere}.am-revision span{color:#626b7c}.am-revision code{white-space:pre-wrap;overflow-wrap:anywhere;word-break:break-word}.am-review{border-color:#cdd3ff}.am-json{white-space:pre-wrap;overflow-wrap:anywhere;word-break:break-word;max-height:320px;overflow:auto;background:#f7f8fb;border-radius:8px;padding:12px;font-size:13px;line-height:1.5}.am-confirm{margin:12px 0;font-size:14px}
@media(max-width:640px){.am-head,.am-card-head,.am-node{flex-direction:column}.am-form{grid-template-columns:minmax(0,1fr)}.am-brand{white-space:normal}.am-actions{width:100%}.am-actions button{flex:1}.am-card{padding:14px}.am-facts{grid-template-columns:repeat(2,minmax(0,1fr))}}
</style>
