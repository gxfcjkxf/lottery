<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import type { Language } from "../../shared/src/brand";
import { createAuthClient } from "../../shared/src/auth";
import { AgentApiError, createAgentApi, type AgentChildUpdate, type AgentMode, type AgentNode, type AgentNodePage } from "./agent-api";
import { allowedChildModes, cacheAgentUpdateReceipt, clearAgentUpdateReceipt, clearAgentUpdateScope, clearPendingAgentUpdate, getPendingAgentUpdate, getPendingAgentUpdatesForScope, rememberAgentUpdate, type AgentMode as ChildMode, type AgentUpdateIntent } from "./agent-state";

const props = defineProps<{ brandCode?: string; locale: Language }>();
const emit = defineEmits<{ "auth-expired": [] }>();
const pageSize = 20;
const api = ref(createAgentApi({ brandCode: props.brandCode }));
const auth = ref(createAuthClient({ brandCode: props.brandCode }));
const self = ref<AgentNode | null>(null);
const children = ref<AgentNode[]>([]);
const offset = ref(0);
const total = ref("0");
const loading = ref(false);
const writing = ref(false);
const error = ref("");
const unavailable = ref<"not-agent" | "not-eligible" | null>(null);
const intentRevision = ref(0);
const confirmed = ref<{ childId: string; ratio: string; mode: AgentMode | null; reason: string } | null>(null);
const drafts = ref<Record<string, { ratio: string; mode: AgentMode | null; reason: string }>>({});
const generation = ref(0);
const disposed = ref(false);
const contextKey = ref<string | null>(null);
const isZh = computed(() => props.locale === "zh");
const pageNumber = computed(() => Math.floor(offset.value / pageSize) + 1);
const hasPrevious = computed(() => offset.value > 0);
const hasNext = computed(() => BigInt(offset.value + children.value.length) < BigInt(total.value));

function text(en: string, zh: string): string { return isZh.value ? zh : en; }
function current(token: number): boolean { return !disposed.value && token === generation.value; }
function parseError(err: unknown): { status?: number; code?: string; message?: string } { return err as { status?: number; code?: string; message?: string }; }
function refreshIntentRevision() { intentRevision.value++; }
function pendingFor(child: AgentNode): AgentUpdateIntent | null {
  void intentRevision.value;
  const me = self.value;
  return me ? getPendingAgentUpdate(me.brand_id, me.member_id, child.id) : null;
}
function setDraft(child: AgentNode) {
  drafts.value[child.id] ??= { ratio: child.config.ratio, mode: child.config.mode, reason: "" };
  return drafts.value[child.id];
}
function modeLabel(mode: AgentMode): string { return mode === "loss" ? text("Loss", "输赢（仅有效输钱）") : text("Turnover", "流水"); }
function modeDisplay(mode: AgentMode | null): string { return mode === null ? text("Use inherited mode", "沿用上级模式") : modeLabel(mode); }
function allowedModes(child: AgentNode): ChildMode[] { return allowedChildModes(self.value?.effective_mode); }
function modeAllowed(child: AgentNode, mode: AgentMode | null): boolean { return mode === null || allowedModes(child).includes(mode); }

function clearView() {
  self.value = null;
  children.value = [];
  total.value = "0";
  offset.value = 0;
  confirmed.value = null;
  drafts.value = {};
}
function authExpired() {
  const [brandId, memberId] = contextKey.value?.split(":") ?? [];
  if (brandId && memberId) {
    for (const intent of getPendingAgentUpdatesForScope(brandId, memberId)) {
      clearPendingAgentUpdate(intent);
      clearAgentUpdateReceipt(intent);
    }
  }
  refreshIntentRevision();
  clearView();
  unavailable.value = null;
  error.value = text("Your session expired. Please sign in again.", "登录已过期，请重新登录。");
  emit("auth-expired");
}
function apiMessage(err: unknown): string {
  const e = parseError(err);
  if (e.status === 401) return text("Your session expired. Please sign in again.", "登录已过期，请重新登录。");
  if (e.status === 0 || (e.status !== undefined && e.status >= 500)) return text("The request result is unknown. Retry the same saved request; refreshing does not confirm it.", "请求结果尚未确认。请重试已保存的同一请求；刷新无法确认此前操作是否成功。");
  if (e.code === "AGENT_VERSION_CONFLICT" || e.code === "AGENT_LIMIT_CONFLICT" || e.code === "AGENT_STATE_CONFLICT") return text("The agent configuration changed. Reload the latest settings before starting a new update.", "代理设置已变化。请重新加载最新设置后再发起更新。");
  return text("Agent information could not be loaded. Please try again.", "暂时无法读取代理信息，请重试。");
}

async function load(options: { preserveError?: boolean } = {}) {
    const token = generation.value;
  if (writing.value) return;
  loading.value = true;
  if (!options.preserveError && !children.value.length) error.value = "";
  unavailable.value = null;
  try {
    const profile = await auth.value.me();
    if (!current(token)) return;
    if (profile.user.status !== "active" || profile.member.status !== "normal") {
      clearView();
      unavailable.value = "not-eligible";
      error.value = "";
      return;
    }
    const actorKey = `${profile.member.brand_id}:${profile.member.id}`;
    if (contextKey.value && contextKey.value !== actorKey) {
      const [oldBrand, oldMember] = contextKey.value.split(":");
      clearAgentUpdateScope(oldBrand, oldMember);
      refreshIntentRevision();
      clearView();
      offset.value = 0;
    }
    contextKey.value = actorKey;
    let me: AgentNode;
    try {
      me = await api.value.me();
    } catch (cause) {
      const e = parseError(cause);
      if (e.status === 404) {
        clearView();
        unavailable.value = "not-agent";
        error.value = "";
        return;
      }
      throw cause;
    }
    if (!current(token)) return;
    if (me.brand_id !== profile.member.brand_id || me.member_id !== profile.member.id) {
      clearAgentUpdateScope(profile.member.brand_id, profile.member.id);
      refreshIntentRevision();
      clearView();
      error.value = text("Your account or brand changed. Reload agent information.", "账号或品牌已切换，请重新加载代理信息。");
      return;
    }
    self.value = me;
    const page = await api.value.children(pageSize, offset.value);
    if (!current(token)) return;
    if (page.brand_id !== me.brand_id || page.parent_id !== me.id || page.items.some((item) => item.brand_id !== me.brand_id || item.parent_id !== me.id)) throw new AgentApiError("Child page scope does not match the current agent", 502, "invalid_response");
    children.value = page.items;
    total.value = page.total_count;
    for (const child of children.value) setDraft(child);
    refreshIntentRevision();
    error.value = options.preserveError && error.value ? error.value : "";
  } catch (cause) {
    if (!current(token)) return;
    const e = parseError(cause);
    if (e.status === 401) { authExpired(); return; }
    if (e.status === 404 && self.value === null) {
      clearView();
      unavailable.value = "not-agent";
      error.value = "";
    } else error.value = apiMessage(cause);
  } finally {
    if (current(token)) loading.value = false;
  }
}

function review(child: AgentNode) {
  if (loading.value || writing.value || pendingFor(child)) return;
  const draft = setDraft(child);
  if (!draft.reason.trim() || draft.ratio !== child.config.ratio && !/^(?:0|1|0\.[0-9]{0,5}[1-9])$/.test(draft.ratio) || !modeAllowed(child, draft.mode)) {
    error.value = text("Choose inherited mode or your effective mode, and enter a canonical ratio fraction and review reason.", "模式只能沿用本级生效模式或选择与本级相同的显式模式，并请输入规范比例小数和审核原因。");
    return;
  }
  confirmed.value = { childId: child.id, ratio: draft.ratio, mode: draft.mode, reason: draft.reason };
  error.value = "";
}

function newKey(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) return crypto.randomUUID();
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

async function submit(intent: AgentUpdateIntent) {
  if (writing.value || loading.value || !current(generation.value)) return;
  const me = self.value;
  if (!me || intent.brandId !== me.brand_id || intent.memberId !== me.member_id) return;
  const token = generation.value;
  writing.value = true;
  error.value = "";
  try {
    const receipt = await api.value.updateChild(intent.childId, intent.body as AgentChildUpdate, intent.idempotencyKey, { brandId: intent.brandId, memberId: intent.memberId, parentId: me.id, childMemberId: children.value.find((child) => child.id === intent.childId)?.member_id });
    if (!current(token)) return;
    cacheAgentUpdateReceipt(intent, receipt);
    clearPendingAgentUpdate(intent);
    refreshIntentRevision();
    confirmed.value = null;
    // The receipt acknowledges the write; this separate GET is the live state.
    writing.value = false;
    await load();
  } catch (cause) {
    if (!current(token)) return;
    const e = parseError(cause);
    if (e.status === 401) {
      clearPendingAgentUpdate(intent);
      authExpired();
    } else if (e.status === 0 || (e.status !== undefined && e.status >= 500)) {
      error.value = apiMessage(cause);
    } else {
      clearPendingAgentUpdate(intent);
      refreshIntentRevision();
      confirmed.value = null;
      error.value = apiMessage(cause);
    }
  } finally {
    if (current(token)) writing.value = false;
  }
}

function confirmUpdate() {
  const selection = confirmed.value;
  const me = self.value;
  const child = children.value.find((item) => item.id === selection?.childId);
  if (!selection || !me || !child || writing.value || loading.value || pendingFor(child) || !modeAllowed(child, selection.mode)) return;
  const body: AgentChildUpdate = {
    version: child.version,
    policy_version: child.policy_version,
    parent_version: me.version,
    ratio: selection.ratio,
    mode: selection.mode,
    reason: selection.reason,
  };
  const intent = rememberAgentUpdate({ brandId: me.brand_id, memberId: me.member_id, childId: child.id, body, idempotencyKey: newKey() });
  refreshIntentRevision();
  void submit(intent);
}

function retry(child: AgentNode) {
  const intent = pendingFor(child);
  if (intent && !writing.value && !loading.value) void submit(intent);
}
function changePage(next: number) {
  if (loading.value || writing.value || children.value.some((child) => pendingFor(child))) return;
  offset.value = Math.max(0, next);
  void load();
}

watch(() => props.brandCode, (brandCode) => {
  generation.value++;
  loading.value = false;
  writing.value = false;
  api.value = createAgentApi({ brandCode });
  auth.value = createAuthClient({ brandCode });
  contextKey.value = null;
  clearView();
  error.value = "";
  void load();
});
onMounted(() => void load());
onBeforeUnmount(() => {
  const previousContext = contextKey.value;
  const scopeParts = previousContext?.split(":") ?? [];
  const pendingAtUnmount = previousContext
    ? getPendingAgentUpdatesForScope(scopeParts[0], scopeParts[1])
    : [];
  disposed.value = true;
  generation.value++;
  // A brief profile check distinguishes logout/account switch from an ordinary remount.
  if (!previousContext) return;
  void auth.value.me().then((profile) => {
    if (`${profile.member.brand_id}:${profile.member.id}` !== previousContext) {
      for (const intent of pendingAtUnmount) {
        clearPendingAgentUpdate(intent);
        clearAgentUpdateReceipt(intent);
      }
    }
  }).catch((cause) => {
    if (parseError(cause).status === 401) {
      for (const intent of pendingAtUnmount) {
        clearPendingAgentUpdate(intent);
        clearAgentUpdateReceipt(intent);
      }
    }
  });
});
</script>

<template>
  <section class="agent-panel">
    <div class="page-heading agent-heading">
      <div>
        <div class="eyebrow">{{ text("AGENT SETTINGS", "代理设置") }}</div>
        <h1>{{ text("My agent", "我的代理") }}</h1>
        <p>{{ text("Your agent settings and direct children. Commission calculation and payout are not implemented.", "查看自己的代理设置和直属下级。佣金计算与发放尚未实现。") }}</p>
      </div>
      <button class="button button-secondary" type="button" :disabled="loading || writing" @click="load()">{{ text("Reload", "重新加载") }}</button>
    </div>

    <p v-if="error" class="agent-message agent-error" role="alert">{{ error }}</p>
    <p v-if="loading && !self" class="agent-message" role="status">{{ text("Loading agent settings…", "正在读取代理设置…") }}</p>
    <div v-else-if="unavailable === 'not-agent'" class="agent-empty">{{ text("This account is not provisioned as an agent.", "此账号尚未开通代理身份。") }}</div>
    <div v-else-if="unavailable === 'not-eligible'" class="agent-empty">{{ text("Agent information is available to active users with a normal brand membership.", "代理信息仅对状态正常的用户和品牌成员开放。") }}</div>

    <template v-if="self">
      <article class="agent-card agent-self">
        <div><span>{{ text("My ratio (raw fraction)", "我的比例（原始小数）") }}</span><strong>{{ self.config.ratio }}</strong></div>
        <div><span>{{ text("Effective mode", "生效模式") }}</span><strong>{{ modeLabel(self.effective_mode) }}</strong></div>
        <small>{{ text("A ratio is a configuration fraction, not income or a commission estimate.", "比例仅为配置小数，不代表收入或佣金预估。") }}</small>
      </article>

      <div class="agent-section-title">
        <div><h2>{{ text("Direct children", "直属下级") }}</h2><p>{{ text("Only each child's ratio and mode can be changed here.", "这里只能修改下级的比例和模式。") }}</p></div>
      </div>
      <div v-if="loading && !children.length" class="agent-message" role="status">{{ text("Loading direct children…", "正在读取直属下级…") }}</div>
      <div v-else-if="!children.length" class="agent-empty">{{ text("No direct children on this page.", "当前页没有直属下级。") }}</div>

      <article v-for="child in children" :key="child.id" class="agent-card agent-child">
        <div class="agent-child-summary">
          <div><span>{{ text("Member ID", "成员编号") }}</span><strong class="agent-id">{{ child.member_id }}</strong></div>
          <div><span>{{ text("Current ratio (raw fraction)", "当前比例（原始小数）") }}</span><strong>{{ child.config.ratio }}</strong></div>
          <div><span>{{ text("Effective mode", "生效模式") }}</span><strong>{{ modeLabel(child.effective_mode) }}</strong></div>
        </div>
        <p v-if="pendingFor(child)" class="agent-message agent-error" role="alert">
          {{ text("This update has an unknown result. Reloading cannot confirm it.", "此更新结果尚未确认。重新加载无法确认此前操作。") }}
          <button class="agent-inline-button" type="button" :disabled="loading || writing" @click="retry(child)">{{ text("Retry the same request", "重试同一请求") }}</button>
        </p>
        <template v-else>
          <div class="agent-editor">
            <label class="agent-field"><span>{{ text("Ratio fraction", "比例小数") }}</span><input v-model="setDraft(child).ratio" :aria-label="text('Ratio fraction', '比例小数')" inputmode="decimal" autocomplete="off" placeholder="0.1" :disabled="loading || writing || child.config.status !== 'active'"><small>{{ text("For example, 0.1 means 10%. Use 0, 1, or up to six decimals without trailing zeros.", "例如 0.1 表示 10%。可填 0、1，或最多六位且末尾不为零的小数。") }}</small></label>
            <label class="agent-field"><span>{{ text("Mode", "模式") }}</span><select v-model="setDraft(child).mode" :aria-label="text('Mode', '模式')" :disabled="loading || writing || child.config.status !== 'active'"><option :value="null">{{ text("Inherit", "沿用上级") }}</option><option v-for="mode in allowedModes(child)" :key="mode" :value="mode">{{ modeLabel(mode) }}</option></select><small>{{ text("A child must use this agent’s effective mode; inheritance follows it.", "下级必须与本级生效模式一致；选择沿用时将继承该模式。") }}</small></label>
            <label class="agent-field agent-reason"><span>{{ text("Review reason", "审核原因") }}</span><textarea v-model="setDraft(child).reason" :aria-label="text('Review reason', '审核原因')" rows="2" maxlength="500" :disabled="loading || writing || child.config.status !== 'active'" :placeholder="text('Why is this change being made?', '请说明本次调整原因')" /></label>
          </div>
          <p v-if="child.config.status !== 'active'" class="agent-muted">{{ text("This child is not active, so its settings cannot be changed.", "此下级当前未启用，不能修改设置。") }}</p>
          <button class="button button-secondary agent-review-button" type="button" :disabled="loading || writing || child.config.status !== 'active'" @click="review(child)">{{ text("Review change", "审核并确认") }}</button>
          <div v-if="confirmed?.childId === child.id" class="agent-confirm" role="group" :aria-label="text('Confirm agent update', '确认代理设置更新')">
            <strong>{{ text("Confirm this configuration", "请确认以下设置") }}</strong>
            <span>{{ text("Ratio", "比例") }}: {{ confirmed.ratio }} · {{ text("Mode", "模式") }}: {{ modeDisplay(confirmed.mode) }}</span>
            <span>{{ text("Reason", "原因") }}: {{ confirmed.reason }}</span>
            <div><button class="button button-primary" type="button" :disabled="loading || writing" @click="confirmUpdate">{{ writing ? text("Saving…", "保存中…") : text("Confirm update", "确认修改") }}</button><button class="button button-secondary" type="button" :disabled="writing" @click="confirmed = null">{{ text("Cancel", "取消") }}</button></div>
          </div>
        </template>
      </article>

      <div v-if="children.length || total !== '0'" class="agent-pagination">
        <button class="button button-secondary" type="button" :disabled="!hasPrevious || loading || writing" @click="changePage(offset - pageSize)">{{ text("Previous", "上一页") }}</button>
        <span>{{ text(`Page ${pageNumber} · ${total} total`, `第 ${pageNumber} 页 · 共 ${total} 位`) }}</span>
        <button class="button button-secondary" type="button" :disabled="!hasNext || loading || writing" @click="changePage(offset + pageSize)">{{ text("Next", "下一页") }}</button>
      </div>
    </template>
  </section>
</template>

<style scoped>
.agent-panel { min-width: 0; }
.agent-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; padding-bottom: 16px; }
.agent-heading h1 { overflow-wrap: anywhere; }
.agent-heading p, .agent-section-title p { color: #87938a; font-size: 13px; line-height: 1.6; }
.agent-card { min-width: 0; margin: 12px 17px; padding: 17px; border: 1px solid #e9ede8; border-radius: 13px; background: white; box-shadow: 0 2px 8px #1e2a5008; }
.agent-self { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 13px; }
.agent-self > div, .agent-child-summary > div { display: grid; gap: 6px; min-width: 0; }
.agent-self span, .agent-child-summary span { color: #87938a; font-size: 12px; }
.agent-self strong, .agent-child-summary strong { color: #28362d; font-size: 16px; overflow-wrap: anywhere; }
.agent-self small { grid-column: 1 / -1; color: #87938a; font-size: 12px; line-height: 1.55; }
.agent-section-title { margin: 20px 17px 8px; }
.agent-section-title h2 { margin: 0; color: #28362d; font-size: 17px; }
.agent-section-title p { margin: 5px 0 0; }
.agent-child-summary { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.agent-child-summary .agent-id { font-size: 12px; font-weight: 500; }
.agent-editor { display: grid; grid-template-columns: minmax(150px, 1fr) minmax(150px, 1fr); gap: 12px; margin-top: 17px; }
.agent-field { display: grid; gap: 6px; color: #546258; font-size: 12px; font-weight: 650; }
.agent-field input, .agent-field select, .agent-field textarea { width: 100%; min-width: 0; border: 1px solid #dfe6df; border-radius: 8px; background: #fff; padding: 10px 11px; color: #26352b; font: inherit; font-size: 13px; }
.agent-field textarea { resize: vertical; }
.agent-field small { color: #87938a; font-size: 12px; line-height: 1.5; font-weight: 400; }
.agent-reason { grid-column: 1 / -1; }
.agent-review-button { margin-top: 12px; }
.agent-confirm { display: grid; gap: 8px; margin-top: 14px; padding: 13px; border-radius: 9px; background: #f2f6f1; color: #4d5b51; font-size: 12px; line-height: 1.55; overflow-wrap: anywhere; }
.agent-confirm strong { color: #28362d; font-size: 13px; }
.agent-confirm > div { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 4px; }
.agent-muted, .agent-message, .agent-empty { margin: 0; padding: 15px 17px; color: #87938a; font-size: 13px; line-height: 1.6; overflow-wrap: anywhere; }
.agent-error { color: #9b4b3f; }
.agent-inline-button { display: inline; margin-left: 5px; padding: 0; border: 0; background: none; color: var(--brand-primary, #56745e); font: inherit; font-weight: 700; text-decoration: underline; cursor: pointer; }
.agent-inline-button:disabled, .agent-panel button:disabled { opacity: .55; cursor: not-allowed; }
.agent-pagination { display: flex; align-items: center; justify-content: center; flex-wrap: wrap; gap: 12px; padding: 14px 17px; color: #87938a; font-size: 12px; }
@media (max-width: 620px) {
  .agent-heading { align-items: flex-start; }
  .agent-heading .button { flex: 0 0 auto; }
  .agent-card { margin: 10px 12px; padding: 14px; }
  .agent-self, .agent-child-summary, .agent-editor { grid-template-columns: minmax(0, 1fr); }
  .agent-reason, .agent-self small { grid-column: auto; }
  .agent-section-title { margin-right: 12px; margin-left: 12px; }
  .agent-pagination { gap: 7px; padding-right: 12px; padding-left: 12px; }
}
</style>
