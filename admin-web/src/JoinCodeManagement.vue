<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { createJoinCodesApi, joinCodePermissions, validJoinCodeDate, validJoinCodeDateRange, type CreateJoinCodeBody, type JoinCode, type JoinCodeKind, type JoinCodeStatus, type UpdateJoinCodeBody } from "./join-codes-api";
import { classifyJoinCodeWriteFailure, clearAllPendingJoinCodeWrites, clearPendingJoinCodeWrite, freezeJoinCodeBody, getPendingJoinCodeWrite, setPendingJoinCodeWrite, type JoinCodeWriteOperation, type JoinCodeWriteBody, type PendingJoinCodeWrite } from "./join-codes-state";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createJoinCodesApi();
const permissions = computed(() => joinCodePermissions(props.account, props.brandId));
const canView = computed(() => permissions.value.view);
const canWrite = computed(() => permissions.value.write);
const permissionSignature = computed(() => JSON.stringify({ accountId: props.account.id, brandId: props.brandId,
  superAdmin: props.account.super_admin, brandIds: [...props.account.brand_ids].sort(),
  brandPermissions: [...(props.account.permissions_by_brand?.[props.brandId] ?? props.account.permissions ?? [])].sort(),
  platformPermissions: [...(props.account.platform_permissions ?? [])].sort() }));
const pageSize = 20;
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const kindFilter = ref<"" | JoinCodeKind>("");
const ownerFilter = ref("");
const offset = ref(0);
const items = ref<JoinCode[]>([]);
const total = ref("0");
const selected = ref<JoinCode | null>(null);
const historyItems = ref<Awaited<ReturnType<typeof api.history>>["items"]>([]);
const historyOffset = ref(0);
const historyTotal = ref("0");
const historyOpen = ref(false);
const form = ref({ kind: "referral" as JoinCodeKind, owner_member_id: "", agent_id: "", status: "active" as JoinCodeStatus,
  starts_at: "", expires_at: "", reason: "" });
const loading = ref(false);
const writing = ref(false);
const error = ref("");
const notice = ref("");
const pending = ref<PendingJoinCodeWrite | null>(null);
const review = ref(false);
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
  error.value = cause instanceof Error ? cause.message : "请求失败";
  if (cause instanceof AdminApiError && cause.code) error.value = `${cause.code}: ${error.value}`;
}
function filters() {
  return { ...(kindFilter.value ? { kind: kindFilter.value } : {}), ...(ownerFilter.value.trim() ? { owner_member_id: ownerFilter.value.trim() } : {}) };
}
async function loadList(): Promise<boolean> {
  if (!canView.value || !props.brandId) { items.value = []; total.value = "0"; restoreIntent(); return false; }
  const ticket = ++listGeneration, scopeTicket = scopeGeneration, accountId = props.account.id, brandId = props.brandId;
  const query = activeQuery.value;
  const requestedFilters = Object.freeze({ ...filters() });
  const requestedOffset = offset.value;
  const requestedLimit = pageSize;
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
function search() { offset.value = 0; activeQuery.value++; selected.value = null; historyItems.value = []; historyOpen.value = false; void loadList(); }
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
    selected.value = detail; form.value = { kind: detail.kind, owner_member_id: detail.owner_member_id, agent_id: detail.agent_id ?? "",
      status: detail.status, starts_at: detail.starts_at ?? "", expires_at: detail.expires_at ?? "", reason: "" };
    historyItems.value = []; historyOffset.value = 0; historyOpen.value = false; restoreIntent();
  } catch (cause) { if (ticket === detailGeneration && currentScope(scopeTicket, accountId, brandId)) handleError(cause); }
}
function startCreate() {
  selected.value = null; historyItems.value = []; historyOpen.value = false;
  form.value = { kind: "referral", owner_member_id: "", agent_id: "", status: "active", starts_at: "", expires_at: "", reason: "" };
}
function dateInput(value: string): string | null { return value.trim() === "" ? null : value.trim(); }
function prepareWrite(operation: JoinCodeWriteOperation) {
  if (!canWrite.value || !props.brandId || !form.value.reason.trim()) return;
  let body: JoinCodeWriteBody;
  let identity: PendingJoinCodeWrite["identity"];
  let resourceId: string | null;
  if (operation === "create") {
    const create: CreateJoinCodeBody = { kind: form.value.kind, owner_member_id: form.value.owner_member_id.trim(),
      agent_id: form.value.kind === "agent" ? form.value.agent_id.trim() : null,
      starts_at: dateInput(form.value.starts_at), expires_at: dateInput(form.value.expires_at), reason: form.value.reason.trim() };
    body = create; resourceId = null;
    identity = { kind: create.kind, code: null, owner_member_id: create.owner_member_id, agent_id: create.agent_id };
  } else {
    if (!selected.value) return;
    const update: UpdateJoinCodeBody = { version: selected.value.version, status: form.value.status,
      starts_at: dateInput(form.value.starts_at), expires_at: dateInput(form.value.expires_at), reason: form.value.reason.trim() };
    body = update; resourceId = selected.value.id;
    identity = { kind: selected.value.kind, code: selected.value.code, owner_member_id: selected.value.owner_member_id, agent_id: selected.value.agent_id };
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
  if (!intent || writing.value || !review.value || !canWrite.value || intent.accountId !== props.account.id || intent.brandId !== props.brandId) return;
  const ticket = ++writeGeneration, grant = permissionSignature.value, accountId = intent.accountId, brandId = intent.brandId;
  const isCurrent = () => currentWrite(ticket, accountId, brandId, grant);
  const sharedScope = { accountId, brandId };
  writing.value = true; error.value = ""; notice.value = "";
  let acknowledged = false;
  try {
    if (intent.operation === "create") await api.create(brandId, intent.body as CreateJoinCodeBody, intent.key);
    else await api.update(brandId, intent.resourceId!, intent.identity, intent.body as UpdateJoinCodeBody, intent.key);
    acknowledged = true;
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
      error.value = cause instanceof Error ? cause.message : "当前状态读取失败。";
      return;
    }
    if (classifyJoinCodeWriteFailure(status) === "uncertain") {
      pending.value = getPendingJoinCodeWrite(sharedScope); review.value = false;
      notice.value = "写入结果未知。冻结的请求正文和幂等键已保留；可按原请求重试。";
      error.value = cause instanceof Error ? cause.message : "请求结果未知。";
    } else {
      clearPendingJoinCodeWrite(sharedScope, intent.key); pending.value = getPendingJoinCodeWrite(sharedScope);
      error.value = cause instanceof Error ? cause.message : "服务器拒绝了请求。";
      if (cause instanceof AdminApiError && cause.code) error.value = `${cause.code}: ${error.value}`;
    }
  } finally { if (isCurrent()) writing.value = false; }
}
function retryFrozen() { void confirmWrite(); }
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
function changePage(direction: -1 | 1) { offset.value = Math.max(0, offset.value + direction * pageSize); void loadList(); }
function validTime(value: string) {
  if (!value.trim()) return true;
  return validJoinCodeDate(value.trim());
}
const formValid = computed(() => {
  const start = dateInput(form.value.starts_at), end = dateInput(form.value.expires_at);
  return Boolean(UUID_RE.test(form.value.owner_member_id.trim()) && (form.value.kind !== "agent" || UUID_RE.test(form.value.agent_id.trim())) &&
    form.value.reason.trim() && validTime(form.value.starts_at) && validTime(form.value.expires_at) &&
    validJoinCodeDateRange(start, end));
});
function pendingTitle(intent: PendingJoinCodeWrite) { return intent.operation === "create" ? "创建加入码" : `更新加入码 ${intent.resourceId}`; }
function historyRange(item: { starts_at: string | null; expires_at: string | null }) { return `${item.starts_at ?? "不限开始"} → ${item.expires_at ?? "不限到期"}`; }
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
    <header class="jc-head"><div><p class="jc-eyebrow">JOIN CODE MANAGEMENT</p><h2>加入码管理</h2><p>管理会员加入来源编码。创建时绑定的类型、会员和代理不可更改；停用后不影响已有归属。</p></div><span class="jc-brand">品牌 · {{ brandId }}</span></header>
    <div v-if="error" class="jc-message is-error" role="alert">{{ error }}</div><p v-if="notice" class="jc-message is-ok" role="status">{{ notice }}</p>
    <p v-if="!canView" class="jc-note">当前账号缺少此品牌的加入码查看权限。冻结请求仍保留在本页会话中。</p>
    <template v-else>
      <section class="jc-card"><div class="jc-card-head"><div><h3>加入码目录</h3><p>共 {{ total }} 条 · 不展示财务或推算统计</p></div><button v-if="canWrite" type="button" class="jc-secondary" @click="startCreate">新建加入码</button></div>
        <form class="jc-filters" @submit.prevent="search"><label>类型<select v-model="kindFilter"><option value="">全部</option><option value="agent">代理</option><option value="referral">推荐</option></select></label><label>归属会员 UUID<input v-model.trim="ownerFilter" autocomplete="off" placeholder="按会员筛选"></label><button class="jc-secondary" type="submit" :disabled="loading">查询</button></form>
        <p v-if="loading" class="jc-note" role="status">正在读取加入码…</p><div class="jc-list">
          <article v-for="item in items" :key="item.id" class="jc-row"><div class="jc-row-main"><strong><code>{{ item.code }}</code></strong><span>{{ item.kind === "agent" ? "代理码" : "推荐码" }} · {{ item.status === "active" ? "启用" : "停用" }} · 版本 {{ item.version }}</span><small>归属会员 {{ item.owner_member_id }}<template v-if="item.agent_id"> · 代理 {{ item.agent_id }}</template></small><small>{{ historyRange(item) }} · {{ item.usable ? "当前可用" : "当前不可用" }}</small></div><div class="jc-actions"><button type="button" class="jc-secondary" @click="openCode(item)">详情与历史</button></div></article>
          <p v-if="!loading && items.length === 0" class="jc-note">当前筛选没有加入码记录。</p>
        </div><div class="jc-pager"><button type="button" class="jc-secondary" :disabled="loading || offset === 0" @click="changePage(-1)">上一页</button><span>{{ total === "0" ? 0 : offset + 1 }}–{{ Math.min(offset + pageSize, Number(total)) }} / {{ total }}</span><button type="button" class="jc-secondary" :disabled="loading || offset + pageSize >= Number(total)" @click="changePage(1)">下一页</button></div>
      </section>
    <section v-if="canWrite" class="jc-card"><div class="jc-card-head"><div><h3>{{ selected ? "编辑加入码" : "创建加入码" }}</h3><p v-if="selected">编码与归属身份不可更改 · 版本 {{ selected.version }}</p><p v-else>编码由服务器生成，创建后不能换绑</p></div><button v-if="selected" type="button" class="jc-secondary" @click="selected = null; startCreate()">新建</button></div>
        <form class="jc-form" @submit.prevent="prepareWrite(selected ? 'update' : 'create')">
          <template v-if="!selected"><label>加入码类型<select v-model="form.kind"><option value="referral">推荐码</option><option value="agent">代理码</option></select></label><label>归属会员 UUID<input v-model.trim="form.owner_member_id" autocomplete="off" required></label><label v-if="form.kind === 'agent'">代理 UUID<input v-model.trim="form.agent_id" autocomplete="off" required></label></template>
          <template v-else><label>加入码类型<input :value="selected.kind === 'agent' ? '代理码' : '推荐码'" readonly></label><label>编码<input :value="selected.code" readonly></label><label>归属会员 UUID<input :value="selected.owner_member_id" readonly></label><label>代理 UUID<input :value="selected.agent_id ?? '无'" readonly></label><label>状态<select v-model="form.status"><option value="active">启用</option><option value="disabled">停用</option></select></label></template>
          <label>开始时间（RFC3339；留空表示不限）<input v-model.trim="form.starts_at" autocomplete="off" placeholder="2026-10-06T04:00:00.123456Z"></label><label>到期时间（RFC3339；留空表示不限）<input v-model.trim="form.expires_at" autocomplete="off" placeholder="2026-11-06T04:00:00Z"></label><label class="jc-wide">管理原因<textarea v-model.trim="form.reason" rows="3" maxlength="500" required></textarea></label>
        <p v-if="!validTime(form.starts_at) || !validTime(form.expires_at)" class="jc-field-error">时间必须包含时区，最多 6 位小数秒，例如 2026-10-06T04:00:00.123456Z。</p><p v-else-if="dateInput(form.starts_at) && dateInput(form.expires_at) && !validJoinCodeDateRange(dateInput(form.starts_at), dateInput(form.expires_at))" class="jc-field-error">开始时间必须早于到期时间。</p>
          <button class="jc-primary" type="submit" :disabled="writing || !formValid">{{ selected ? "核对并保存" : "核对并创建" }}</button>
        </form>
      </section>
      <section v-if="selected && canView" class="jc-card"><div class="jc-card-head"><div><h3>变更历史</h3><p>仅管理员可见原因和操作者 · 不可改写记录</p></div><button class="jc-secondary" type="button" @click="loadHistory">{{ historyOpen ? "刷新历史" : "查询历史" }}</button></div><div v-if="historyOpen" class="jc-history"><article v-for="item in historyItems" :key="item.id" class="jc-history-row"><strong>版本 {{ item.version }} · {{ item.status === "active" ? "启用" : "停用" }}</strong><span>{{ item.created_at }} · 操作者 {{ item.actor_id }}</span><span>{{ historyRange(item) }}</span><span>原因：{{ item.reason }}</span><span>审计记录 {{ item.audit_log_id }}</span></article><p v-if="!historyItems.length" class="jc-note">暂无历史记录。</p><div class="jc-pager"><button class="jc-secondary" type="button" :disabled="historyOffset === 0" @click="historyOffset = Math.max(0, historyOffset - pageSize); loadHistory()">上一页</button><span>{{ historyTotal }} 条</span><button class="jc-secondary" type="button" :disabled="historyOffset + pageSize >= Number(historyTotal)" @click="historyOffset += pageSize; loadHistory()">下一页</button></div></div></section>
    </template>
    <section v-if="pending && pending.accountId === account.id && pending.brandId === brandId" class="jc-card jc-review"><div class="jc-card-head"><div><h3>核对冻结请求</h3><p>{{ pendingTitle(pending) }}</p></div><span class="jc-pending">未确认结果</span></div><p class="jc-note">账号 {{ pending.accountId }} · 品牌 {{ pending.brandId }}。查询当前状态不能确认旧请求；结果未知时只能按原正文与幂等键重试。</p><dl v-if="pending.operation === 'create'" class="jc-identity"><div><dt>类型</dt><dd>{{ pending.identity.kind === "agent" ? "代理码" : "推荐码" }}</dd></div><div><dt>归属会员</dt><dd>{{ pending.identity.owner_member_id }}</dd></div><div><dt>代理</dt><dd>{{ pending.identity.agent_id ?? "无" }}</dd></div></dl><dl v-else class="jc-identity"><div><dt>不可变编码</dt><dd>{{ pending.identity.code }}</dd></div><div><dt>原归属会员</dt><dd>{{ pending.identity.owner_member_id }}</dd></div><div><dt>原代理</dt><dd>{{ pending.identity.agent_id ?? "无" }}</dd></div><div><dt>类型</dt><dd>{{ pending.identity.kind }}</dd></div></dl><pre class="jc-json">{{ JSON.stringify(pending.body, null, 2) }}</pre><p class="jc-meta">Idempotency-Key：<code>{{ pending.key }}</code></p><label class="jc-check"><input v-model="review" type="checkbox" :disabled="!canWrite">我已核对上方不可变归属与请求内容</label><div class="jc-actions"><button class="jc-primary" type="button" :disabled="!canWrite || !review || writing" @click="confirmWrite">{{ writing ? "提交中…" : "确认提交" }}</button><button v-if="!writing" class="jc-secondary" type="button" :disabled="!canWrite || !review" @click="retryFrozen">按原键重试</button><button class="jc-secondary" type="button" :disabled="writing" @click="review = false">收起核对</button></div></section>
  </section>
</template>

<style scoped>
.join-codes{width:100%;min-width:0;color:#252a36}.jc-head,.jc-card-head{display:flex;justify-content:space-between;align-items:flex-start;gap:14px}.jc-head{margin:4px 0 16px}.jc-head h2{font-size:20px;margin:3px 0 5px}.jc-head p,.jc-card-head p{margin:0;color:#737b8c;font-size:13px;line-height:1.55;overflow-wrap:anywhere}.jc-eyebrow{font-size:12px!important;letter-spacing:1.1px;color:#8991a2!important;font-weight:700}.jc-brand,.jc-pending{border-radius:20px;background:#f1f3f8;padding:8px 12px;color:#626b7c;font-size:13px;white-space:nowrap}.jc-note,.jc-message{border-radius:8px;background:#f4f6fc;color:#626b7c;padding:11px 13px;font-size:13px;line-height:1.6;overflow-wrap:anywhere;margin:12px 0}.jc-message.is-error{background:#fff1f0;color:#a12d25}.jc-message.is-ok{background:#eef8f3;color:#287554}.jc-card{background:#fff;border:1px solid #e9ebf0;border-radius:11px;padding:16px;min-width:0;box-shadow:0 2px 8px #1e2a5008;margin:13px 0}.jc-card h3{font-size:16px;margin:0 0 12px}.jc-card-head{margin-bottom:14px}.jc-card-head h3{margin:0 0 4px}.jc-filters,.jc-form{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:13px}.jc-filters{grid-template-columns:minmax(150px,.7fr) minmax(240px,1.3fr) auto;align-items:end;margin:12px 0 16px}.jc-filters label,.jc-form label{display:grid;gap:6px;font-size:13px;font-weight:600;min-width:0}.jc-filters input,.jc-filters select,.jc-form input:not([type=checkbox]),.jc-form select,.jc-form textarea{box-sizing:border-box;width:100%;min-width:0;border:1px solid #dfe3eb;border-radius:7px;background:#fff;padding:10px 11px;color:#252a36;font:inherit}.jc-form textarea{resize:vertical}.jc-wide,.jc-field-error{grid-column:1/-1}.jc-list{display:grid;gap:9px}.jc-row{display:flex;justify-content:space-between;gap:12px;border:1px solid #eceef3;border-radius:9px;padding:12px;min-width:0}.jc-row-main{display:grid;gap:5px;min-width:0}.jc-row-main strong,.jc-row-main small,.jc-row-main span{overflow-wrap:anywhere}.jc-row-main span,.jc-row-main small{font-size:13px;color:#626b7c}.jc-row code,.jc-card code{overflow-wrap:anywhere;word-break:break-word}.jc-pager{display:flex;justify-content:center;align-items:center;gap:12px;margin-top:13px;flex-wrap:wrap}.jc-pager span{font-size:13px;color:#626b7c;font-variant-numeric:tabular-nums}.jc-primary,.jc-secondary{border-radius:7px;padding:9px 13px;font-size:14px;min-height:40px}.jc-primary{border:1px solid #5969df;background:#5969df;color:#fff}.jc-secondary{border:1px solid #dfe3eb;background:#fff;color:#41495a}.jc-actions{display:flex;gap:8px;flex-wrap:wrap;align-items:center}.jc-field-error{color:#a12d25;font-size:13px;margin:0}.jc-history{display:grid;gap:9px}.jc-history-row{display:grid;gap:6px;background:#f9fafc;border:1px solid #eceef3;border-radius:8px;padding:11px;min-width:0;font-size:13px;overflow-wrap:anywhere}.jc-history-row span{color:#626b7c}.jc-identity{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:9px;margin:12px 0}.jc-identity div{border:1px solid #eceef3;border-radius:8px;padding:10px;min-width:0}.jc-identity dt{font-size:12px;color:#737b8c}.jc-identity dd{font-size:13px;font-weight:600;overflow-wrap:anywhere;margin:5px 0 0}.jc-review{border-color:#cdd3ff}.jc-json{white-space:pre-wrap;overflow-wrap:anywhere;word-break:break-word;max-height:320px;overflow:auto;background:#f7f8fb;border-radius:8px;padding:12px;font-size:13px;line-height:1.5}.jc-meta{font-size:13px;color:#737b8c;overflow-wrap:anywhere}.jc-check{display:flex;align-items:center;gap:9px;min-height:40px;font-size:14px;margin:12px 0}.jc-check input{width:18px;height:18px;flex:none}
@media(max-width:640px){.jc-head,.jc-card-head,.jc-row{flex-direction:column}.jc-form,.jc-filters{grid-template-columns:minmax(0,1fr)}.jc-brand{white-space:normal}.jc-actions{width:100%}.jc-actions button{flex:1}.jc-card{padding:14px}}
</style>
