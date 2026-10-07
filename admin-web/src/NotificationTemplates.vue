<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import {
  createNotificationTemplatesApi, notificationTemplateKeys, notificationTemplatePermissions,
  validNotificationTemplateContent, type NotificationTemplate, type NotificationTemplateContent,
  type NotificationTemplateKey, type NotificationTemplateRevision,
} from "./notification-templates-api";
import {
  classifyNotificationTemplateFailure, clearPendingNotificationTemplateWrite, freezeNotificationTemplateBody,
  getPendingNotificationTemplateWrite, notificationTemplateScopeKey, notificationTemplateSessionGeneration,
  setPendingNotificationTemplateWrite, type PendingNotificationTemplateWrite,
} from "./notification-templates-state";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const PAGE_SIZE = 20;
const SAMPLE_RESOURCE_ID = "00000000-0000-4000-8000-000000000099";
const SAMPLE_POINTS = "12345";
const api = createNotificationTemplatesApi();
const rights = computed(() => notificationTemplatePermissions(props.account, props.brandId));
const templates = ref<NotificationTemplate[]>([]);
const record = ref<NotificationTemplate | null>(null);
const receipt = ref<NotificationTemplate | null>(null);
const history = ref<NotificationTemplateRevision[]>([]);
const historyOffset = ref(0);
const historyHasNext = computed(() => history.value.length === PAGE_SIZE);
const selectedKey = ref<NotificationTemplateKey | "">("");
const draft = ref<NotificationTemplateContent | null>(null);
const reason = ref("");
const review = ref<PendingNotificationTemplateWrite | null>(null);
const confirmed = ref(false);
const pending = ref<PendingNotificationTemplateWrite | null>(null);
const loading = ref(false), historyLoading = ref(false), writing = ref(false);
const error = ref(""), notice = ref("");
let alive = true, listTicket = 0, historyTicket = 0, writeTicket = 0;

const permissionScope = computed(() => JSON.stringify([
  props.account.id, props.account.super_admin, props.account.brand_ids, props.account.permissions,
  props.account.permissions_by_brand, props.account.platform_permissions, props.brandId, rights.value,
]));
const scope = computed(() => selectedKey.value ? notificationTemplateScopeKey(props.account.id, props.brandId, selectedKey.value) : "");
const reasonValid = computed(() => reason.value.trim() === reason.value && reason.value.trim().length > 0 &&
  new TextEncoder().encode(reason.value).length <= 500);
const reasonByteLength = computed(() => new TextEncoder().encode(reason.value).length);
const contentValid = computed(() => Boolean(draft.value && selectedKey.value && validNotificationTemplateContent(draft.value, selectedKey.value)));
const commissionFacts = computed(() => {
  if (selectedKey.value === "commission.paid") return [
    "Historical record: commission points were credited. Check the current wallet; this is not a new income forecast or an external payment.",
    "历史记录：佣金积分曾入账。请查看当前钱包；本记录不是新收入预测或外部付款承诺。",
  ] as const;
  if (selectedKey.value === "commission.adjusted") return [
    "Historical record: a signed commission adjustment was recorded. Check the current wallet; this is not a new income forecast or an external payment.",
    "历史记录：曾记录一笔带正负方向的佣金调整。请查看当前钱包；本记录不是新收入预测或外部付款承诺。",
  ] as const;
  return null;
});
const canEdit = computed(() => rights.value.view && rights.value.write && !props.account.super_admin && Boolean(record.value) &&
  Boolean(draft.value) && !pending.value && !writing.value && !loading.value);
const canReview = computed(() => canEdit.value && contentValid.value && reasonValid.value && !historyLoading.value);
const unknown = computed(() => Boolean(pending.value));

function current(ticket: number, lane: "list" | "history", capturedPermission: string, generation: number): boolean {
  return alive && ticket === (lane === "list" ? listTicket : historyTicket) &&
    capturedPermission === permissionScope.value && generation === notificationTemplateSessionGeneration() && rights.value.view;
}
function currentWrite(ticket: number, capturedScope: string, capturedPermission: string, generation: number): boolean {
  return alive && ticket === writeTicket && capturedScope === scope.value && capturedPermission === permissionScope.value &&
    generation === notificationTemplateSessionGeneration();
}
function refreshPending() {
  pending.value = selectedKey.value ? getPendingNotificationTemplateWrite({ accountId: props.account.id, brandId: props.brandId, templateKey: selectedKey.value }) : null;
}
function cloneContent(value: NotificationTemplateContent): NotificationTemplateContent {
  return { en: { ...value.en }, "zh-CN": { ...value["zh-CN"] } };
}
function clearReview() { review.value = null; confirmed.value = false; }
function resetScope() {
  listTicket++; historyTicket++; writeTicket++;
  loading.value = false; historyLoading.value = false; writing.value = false;
  templates.value = []; record.value = null; receipt.value = null; history.value = []; historyOffset.value = 0;
  selectedKey.value = ""; draft.value = null; reason.value = ""; error.value = ""; notice.value = "";
  clearReview(); refreshPending();
}
function invalidateSession(capturedPermission: string, generation: number) {
  if (!alive || capturedPermission !== permissionScope.value || generation !== notificationTemplateSessionGeneration()) return;
  emit("session-invalid");
}
function handleReadError(cause: unknown, capturedPermission: string, generation: number) {
  if (cause instanceof AdminApiError && cause.status === 401) invalidateSession(capturedPermission, generation);
  else if (capturedPermission === permissionScope.value && generation === notificationTemplateSessionGeneration())
    error.value = cause instanceof Error ? cause.message : "读取通知模板失败。";
}
function chooseRecord(key: NotificationTemplateKey, value: NotificationTemplate, isReceipt = false) {
  if (selectedKey.value !== key) return;
  record.value = value;
  // A remount can select its unresolved event before the list finishes.
  // Recover the frozen draft even when no prior editor record exists.
  draft.value = cloneContent(pending.value?.body.content ?? value.content);
  if (isReceipt) receipt.value = value;
}
async function readTemplates() {
  if (!rights.value.view) return false;
  const ticket = ++listTicket, capturedPermission = permissionScope.value, generation = notificationTemplateSessionGeneration();
  loading.value = true; error.value = "";
  try {
    const result = await api.list(props.brandId);
    if (!current(ticket, "list", capturedPermission, generation)) return false;
    templates.value = result;
    if (!selectedKey.value && result.length) selectedKey.value = result[0].key;
    const selected = result.find((item) => item.key === selectedKey.value);
    if (selected && !receipt.value) chooseRecord(selected.key, selected);
    return true;
  } catch (cause) {
    if (current(ticket, "list", capturedPermission, generation)) handleReadError(cause, capturedPermission, generation);
    return false;
  } finally { if (current(ticket, "list", capturedPermission, generation)) loading.value = false; }
}
async function readHistory(offset = historyOffset.value) {
  if (!rights.value.view || !selectedKey.value) return false;
  const ticket = ++historyTicket, capturedPermission = permissionScope.value, generation = notificationTemplateSessionGeneration();
  const key = selectedKey.value;
  historyLoading.value = true;
  try {
    const result = await api.history(key, props.brandId, PAGE_SIZE, offset);
    if (!current(ticket, "history", capturedPermission, generation) || key !== selectedKey.value) return false;
    history.value = result; historyOffset.value = offset; return true;
  } catch (cause) {
    if (current(ticket, "history", capturedPermission, generation) && key === selectedKey.value) handleReadError(cause, capturedPermission, generation);
    return false;
  } finally { if (current(ticket, "history", capturedPermission, generation)) historyLoading.value = false; }
}
function startReview() {
  if (!canReview.value || !record.value || !draft.value || !selectedKey.value) return;
  const body = freezeNotificationTemplateBody({ version: record.value.version, content: draft.value, reason: reason.value });
  review.value = Object.freeze({ accountId: props.account.id, brandId: props.brandId, templateKey: selectedKey.value,
    body, key: createIdempotencyKey() });
  confirmed.value = false; error.value = ""; notice.value = "核对这份冻结的中英文内容、当前版本和原因后确认保存。";
}
async function submit(intent: PendingNotificationTemplateWrite) {
  if (writing.value || !rights.value.view || !rights.value.write || props.account.super_admin ||
    intent.accountId !== props.account.id || intent.brandId !== props.brandId || intent.templateKey !== selectedKey.value ||
    (intent !== review.value && intent !== pending.value)) return;
  const capturedScope = scope.value, capturedPermission = permissionScope.value, generation = notificationTemplateSessionGeneration();
  const ticket = ++writeTicket;
  writing.value = true; error.value = "";
  setPendingNotificationTemplateWrite({ accountId: intent.accountId, brandId: intent.brandId, templateKey: intent.templateKey }, intent);
  pending.value = intent; clearReview();
  notice.value = "正在提交已核对的模板版本…";
  try {
    const accepted = await api.put(intent.templateKey, intent.brandId, intent.body, intent.key);
    clearPendingNotificationTemplateWrite({ accountId: intent.accountId, brandId: intent.brandId, templateKey: intent.templateKey }, intent.key);
    if (!currentWrite(ticket, capturedScope, capturedPermission, generation)) return;
    pending.value = null;
    // Keep the exact immutable receipt visible; a later list/history read cannot replace it.
    receipt.value = accepted; record.value = accepted; draft.value = cloneContent(accepted.content);
    templates.value = templates.value.map((item) => item.key === accepted.key ? accepted : item);
    notice.value = `已保存服务器确认的第 ${accepted.version} 版。正在独立刷新修订历史。`;
    void readHistory(0);
  } catch (cause) {
    if (cause instanceof AdminApiError && cause.status === 401) {
      if (currentWrite(ticket, capturedScope, capturedPermission, generation)) invalidateSession(capturedPermission, generation);
      return;
    }
    const status = cause instanceof AdminApiError ? cause.status : 0;
    if (classifyNotificationTemplateFailure(status) === "unknown") {
      if (currentWrite(ticket, capturedScope, capturedPermission, generation)) {
        pending.value = getPendingNotificationTemplateWrite({ accountId: intent.accountId, brandId: intent.brandId, templateKey: intent.templateKey });
        notice.value = "写入结果未知。原请求正文和幂等键已保留；只读刷新不会确认或清除此请求。请显式按原请求重试。";
        error.value = cause instanceof Error ? cause.message : "写入结果未知。";
      }
    } else {
      clearPendingNotificationTemplateWrite({ accountId: intent.accountId, brandId: intent.brandId, templateKey: intent.templateKey }, intent.key);
      if (currentWrite(ticket, capturedScope, capturedPermission, generation)) {
        pending.value = null;
        notice.value = status === 409 ? "服务器拒绝了此版本；读取最新模板后重新核对并创建新请求。" : "服务器明确拒绝了本次保存。";
        error.value = cause instanceof Error ? cause.message : "保存失败。";
        if (status === 409) { receipt.value = null; void readTemplates(); }
      }
    }
  } finally { if (currentWrite(ticket, capturedScope, capturedPermission, generation)) writing.value = false; }
}
function confirmSave() { if (review.value && confirmed.value && canReview.value) void submit(review.value); }
function retryUnknown() { if (pending.value && !writing.value) void submit(pending.value); }
function selectKey(value: string) {
  if (!notificationTemplateKeys.includes(value as NotificationTemplateKey)) return;
  selectedKey.value = value as NotificationTemplateKey;
  const next = templates.value.find((item) => item.key === selectedKey.value);
  record.value = next ?? null; receipt.value = null; draft.value = next ? cloneContent(next.content) : null;
  history.value = []; historyOffset.value = 0; error.value = ""; notice.value = ""; reason.value = "";
  clearReview(); refreshPending();
  if (pending.value) draft.value = cloneContent(pending.value.body.content);
}
function interpolate(text: string): string {
  return text.replaceAll("{points}", SAMPLE_POINTS).replaceAll("{resource_id}", SAMPLE_RESOURCE_ID);
}
function time(value: string): string {
  const date = new Date(value); return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}
function formatKey(key: string): string { return key; }

watch(permissionScope, () => { resetScope(); if (rights.value.view) void readTemplates(); }, { flush: "sync" });
watch(selectedKey, (next, previous) => {
  if (next === previous || !next) return;
  clearReview(); refreshPending(); void readHistory(0);
});
watch(reason, () => { if (review.value) clearReview(); });
onMounted(() => { refreshPending(); if (rights.value.view) void readTemplates(); });
onBeforeUnmount(() => { alive = false; listTicket++; historyTicket++; writeTicket++; });
</script>

<template>
  <section class="nt-page" aria-labelledby="nt-title">
    <header class="nt-heading">
      <div><p class="nt-eyebrow">通知服务 · 模板与修订记录</p><h2 id="nt-title">通知模板</h2></div>
      <button v-if="rights.view" class="nt-button" type="button" aria-label="只读刷新通知模板" :disabled="loading || writing" @click="readTemplates()">{{ loading ? "读取中…" : "刷新模板" }}</button>
    </header>

    <p v-if="!rights.view" class="nt-message">当前账号没有此品牌的通知模板查看权限。查看权限和品牌成员范围分别核验。</p>
    <template v-else>
      <p v-if="error" class="nt-message nt-error" role="alert">{{ error }}</p>
      <p v-if="notice" class="nt-message" role="status">{{ notice }}</p>
      <div class="nt-layout">
        <nav class="nt-keys" aria-label="通知事件模板">
          <label for="nt-key">通知事件</label>
          <select id="nt-key" :value="selectedKey" :disabled="writing" @change="selectKey(($event.target as HTMLSelectElement).value)">
            <option value="" disabled>选择通知事件</option>
            <option v-for="key in notificationTemplateKeys" :key="key" :value="key">{{ formatKey(key) }}</option>
          </select>
          <p class="nt-hint">支持16种通知事件：会员、充值、注单、奖项、佣金及提现状态。事件事实由系统生成；奖项发放、冲正、佣金和提现事件的双语历史说明保持只读。提现通知仅记录内部积分状态，不代表银行或虚拟币转账。</p>
        </nav>

        <div v-if="(record && draft && selectedKey) || pending" class="nt-workspace">
          <section v-if="record && draft" class="nt-card" aria-labelledby="nt-editor-title">
            <div class="nt-card-heading"><div><h3 id="nt-editor-title">模板内容</h3><p>版本 {{ record.version }} · 更新于 {{ time(record.updated_at) }}</p></div>
              <span v-if="receipt" class="nt-receipt">已确认保存 v{{ receipt.version }}</span>
            </div>
            <dl class="nt-audit">
              <div><dt>品牌</dt><dd>{{ record.brand_id }}</dd></div>
              <div><dt>当前版本审计 ID</dt><dd>{{ record.audit_log_id ?? "初始版本暂无审计 ID" }}</dd></div>
            </dl>
            <div class="nt-languages">
              <fieldset v-for="locale in (['en', 'zh-CN'] as const)" :key="locale" class="nt-locale" :disabled="!canEdit">
                <legend>{{ locale === "en" ? "English" : "简体中文" }}</legend>
                <label :for="`nt-title-${locale}`">标题（最多 120 UTF-8 字节）</label>
                <input :id="`nt-title-${locale}`" v-model="draft[locale].title" maxlength="120" autocomplete="off" />
                <label :for="`nt-body-${locale}`">正文（最多 1200 UTF-8 字节）</label>
                <textarea :id="`nt-body-${locale}`" v-model="draft[locale].body" rows="5" maxlength="1200" />
                <small>{{ selectedKey === 'member.joined' ? '欢迎通知只能使用 {resource_id}，不能引用积分。' : '正文须包含 {points}，可使用 {resource_id}。' }} 预览使用固定示例，不是真实通知。</small>
                <div class="nt-preview" aria-label="固定示例预览"><strong>固定示例预览 · 不是实际用户通知</strong>
                  <p>{{ interpolate(draft[locale].title) }}</p><div>{{ interpolate(draft[locale].body) }}</div>
                  <small>示例 points={{ SAMPLE_POINTS }} · resource_id={{ SAMPLE_RESOURCE_ID }}</small>
                </div>
              </fieldset>
            </div>
            <div v-if="commissionFacts" class="nt-facts-note" data-testid="commission-facts-note"><strong>不可编辑的佣金历史事实 / Fixed commission history</strong>
              <p v-for="fact in commissionFacts" :key="fact">{{ fact }}</p></div>
            <div v-else class="nt-facts-note"><strong>系统事实说明</strong><p>中奖金额、奖项发放和冲正事实不在模板中编辑；只可使用允许的通知占位符。</p></div>
            <p v-if="draft && !contentValid" class="nt-message nt-error" role="alert">标题或正文格式无效：请检查字节上限、首尾空白、占位符；不接受 HTML、外部地址或控制字符。</p>
            <div v-if="rights.write && !props.account.super_admin" class="nt-reason">
              <label for="nt-reason">修改原因（最多 500 UTF-8 字节）</label>
              <textarea id="nt-reason" v-model="reason" rows="2" :disabled="writing || Boolean(pending)" placeholder="说明此次修改的依据" />
              <small>{{ reasonByteLength }} / 500 字节；首尾空白不会自动移除。</small>
              <button class="nt-button nt-primary" type="button" :disabled="!canReview" @click="startReview">核对并继续</button>
            </div>
            <p v-else class="nt-readonly">当前账号可查看此模板，但没有品牌模板修改权限。</p>
          </section>

          <aside v-if="pending" class="nt-pending" aria-labelledby="nt-pending-title">
            <h3 id="nt-pending-title">{{ writing ? '正在提交模板' : '保存结果未知' }}</h3>
            <p>只读刷新不会确认或清除此请求。只有显式重试会再次提交完全相同的正文和幂等键。</p>
            <dl class="nt-audit"><div><dt>事件 / 品牌</dt><dd>{{ pending.templateKey }} · {{ pending.brandId }}</dd></div>
              <div><dt>原版本</dt><dd>{{ pending.body.version }}</dd></div><div><dt>原原因</dt><dd>{{ pending.body.reason }}</dd></div>
              <div><dt>幂等键</dt><dd>{{ pending.key }}</dd></div></dl>
            <button v-if="rights.write && !props.account.super_admin" class="nt-button nt-primary" type="button" :disabled="writing" @click="retryUnknown">{{ writing ? "按原请求提交中…" : "按原请求重试" }}</button>
            <p v-else>当前账号不能重试；请求仍保留在本页会话内存中。</p>
          </aside>

          <section v-if="review && !pending" class="nt-confirm" aria-labelledby="nt-confirm-title">
            <h3 id="nt-confirm-title">核对保存内容</h3>
            <p>将为 {{ review.templateKey }} 提交版本 {{ review.body.version }} 的内容和原因。确认后会使用新幂等键发送一次。</p>
            <div class="nt-review-locale" v-for="locale in (['en', 'zh-CN'] as const)" :key="locale">
              <strong>{{ locale === "en" ? "English" : "简体中文" }}</strong>
              <p>{{ review.body.content[locale].title }}</p><div>{{ review.body.content[locale].body }}</div>
            </div>
            <p><strong>原因：</strong>{{ review.body.reason }}</p><p class="nt-break"><strong>幂等键：</strong>{{ review.key }}</p>
            <label class="nt-check"><input v-model="confirmed" type="checkbox" />我已核对品牌、事件、版本、中英文内容和原因，并确认保存</label>
            <div class="nt-actions"><button class="nt-button" type="button" :disabled="writing" @click="clearReview">返回修改</button>
              <button class="nt-button nt-primary" type="button" :disabled="!confirmed || writing || !rights.write" @click="confirmSave">{{ writing ? "保存中…" : "确认保存" }}</button></div>
          </section>

          <section class="nt-card nt-history" aria-labelledby="nt-history-title">
            <div class="nt-card-heading"><div><h3 id="nt-history-title">修订历史</h3><p>每页 {{ PAGE_SIZE }} 条 · 历史只读</p></div>
              <button class="nt-button" type="button" :disabled="historyLoading || writing" @click="readHistory()">{{ historyLoading ? "读取中…" : "刷新历史" }}</button></div>
            <ol class="nt-revisions"><li v-for="revision in history" :key="revision.id">
              <div class="nt-card-heading"><strong>版本 {{ revision.version }}</strong><time>{{ time(revision.created_at) }}</time></div>
              <p>修改者：{{ revision.changed_by ?? "系统初始化" }} · 审计 ID：{{ revision.audit_log_id ?? "无" }}</p>
              <p>原因：{{ revision.reason }}</p>
              <details><summary>查看保存内容</summary><div v-for="locale in (['en', 'zh-CN'] as const)" :key="locale" class="nt-review-locale">
                <strong>{{ locale }}</strong><p>{{ revision.content[locale].title }}</p><div>{{ revision.content[locale].body }}</div></div></details>
            </li><li v-if="!history.length && !historyLoading" class="nt-empty">暂无修订历史。</li>
              <li v-if="historyLoading && !history.length" class="nt-empty">正在读取修订历史…</li></ol>
            <footer class="nt-pagination"><button class="nt-button" type="button" :disabled="historyOffset === 0 || historyLoading || writing" @click="readHistory(Math.max(0, historyOffset - PAGE_SIZE))">上一页</button>
              <span>偏移 {{ historyOffset }} · {{ history.length }} 条</span>
              <button class="nt-button" type="button" :disabled="!historyHasNext || historyLoading || writing" @click="readHistory(historyOffset + PAGE_SIZE)">下一页</button></footer>
          </section>
        </div>
        <p v-else class="nt-message">{{ loading ? "正在读取通知模板…" : "所选事件暂无模板记录。" }}</p>
      </div>
    </template>
  </section>
</template>

<style scoped>
.nt-page{display:grid;gap:16px;min-width:0;color:#172033}.nt-heading,.nt-card-heading,.nt-pagination,.nt-actions{display:flex;align-items:center;justify-content:space-between;gap:12px}.nt-heading{align-items:flex-start}.nt-heading h2{margin:2px 0 0;font-size:1.25rem}.nt-eyebrow{margin:0;color:#68758a;font-size:.78rem}.nt-layout{display:grid;grid-template-columns:minmax(190px,240px) minmax(0,1fr);align-items:start;gap:16px}.nt-keys,.nt-card,.nt-confirm,.nt-pending,.nt-message{border:1px solid #d9e0ea;border-radius:12px;padding:16px;background:#fff}.nt-keys{display:grid;gap:9px;position:sticky;top:12px}.nt-keys label,.nt-locale label,.nt-reason label{font-weight:600;font-size:.9rem}.nt-keys select,.nt-locale input,.nt-locale textarea,.nt-reason textarea{box-sizing:border-box;width:100%;border:1px solid #c5ceda;border-radius:8px;padding:9px 10px;font:inherit;background:#fff}.nt-workspace{display:grid;gap:16px;min-width:0}.nt-card h3,.nt-confirm h3,.nt-pending h3{margin:0}.nt-card-heading p,.nt-card-heading h3{margin:0}.nt-card-heading p,.nt-hint,.nt-locale small,.nt-reason small,.nt-readonly{color:#667085;font-size:.86rem}.nt-hint{margin:0;line-height:1.45}.nt-audit{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px 18px;margin:14px 0}.nt-audit>div{min-width:0;padding:7px 0;border-bottom:1px solid #eef1f5}.nt-audit dt{color:#697586;font-size:.78rem}.nt-audit dd{margin:4px 0 0;overflow-wrap:anywhere;word-break:break-word;font-size:.88rem}.nt-languages{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.nt-locale{display:grid;align-content:start;gap:9px;min-width:0;border:1px solid #d9e0ea;border-radius:10px;padding:12px}.nt-locale legend{padding:0 5px;font-weight:700}.nt-locale textarea,.nt-reason textarea{resize:vertical}.nt-locale small{line-height:1.4}.nt-preview,.nt-review-locale{display:grid;gap:5px;padding:10px;border-radius:8px;background:#f5f7fa;overflow-wrap:anywhere;white-space:pre-wrap}.nt-preview p,.nt-review-locale p,.nt-review-locale div{margin:0;white-space:pre-wrap;overflow-wrap:anywhere}.nt-preview>strong,.nt-preview small{font-size:.78rem;color:#536176}.nt-facts-note{margin-top:14px;padding:11px 12px;border-left:3px solid #8b9bb3;background:#f7f8fa}.nt-facts-note p{margin:4px 0 0;color:#536176;font-size:.88rem}.nt-reason{display:grid;gap:8px;margin-top:16px;max-width:760px}.nt-button{border:1px solid #c5ceda;border-radius:8px;padding:8px 12px;font:inherit;cursor:pointer;background:#fff}.nt-button.nt-primary{background:#254a83;border-color:#254a83;color:#fff}.nt-button:disabled{opacity:.52;cursor:not-allowed}.nt-reason>.nt-button{justify-self:start}.nt-message{margin:0;color:#344054}.nt-error{background:#fff0ee;border-color:#df9385;color:#a32e1a}.nt-pending{display:grid;gap:8px;background:#fff1ee;border-color:#df9385}.nt-pending p,.nt-confirm>p{margin:0;overflow-wrap:anywhere}.nt-confirm{display:grid;gap:12px}.nt-review-locale{border:1px solid #e0e5ec}.nt-check{display:flex;align-items:flex-start;gap:9px}.nt-check input{margin-top:.25em}.nt-actions{justify-content:flex-end}.nt-history{display:grid;gap:12px}.nt-revisions{display:grid;gap:10px;margin:0;padding:0;list-style:none}.nt-revisions>li{display:grid;gap:8px;padding:12px;border:1px solid #e1e6ed;border-radius:9px;min-width:0}.nt-revisions>li>p{margin:0;font-size:.9rem;overflow-wrap:anywhere}.nt-revisions time{color:#667085;font-size:.82rem}.nt-revisions details summary{cursor:pointer}.nt-revisions details{display:grid;gap:8px}.nt-empty{display:block!important;text-align:center;color:#667085;padding:18px!important}.nt-pagination{justify-content:center;flex-wrap:wrap}.nt-break{overflow-wrap:anywhere;word-break:break-word}.nt-receipt{border-radius:99px;padding:5px 9px;background:#e6f6eb;color:#176b37;font-size:.82rem;white-space:nowrap}
@media(max-width:850px){.nt-layout{grid-template-columns:minmax(0,1fr)}.nt-keys{position:static}.nt-languages{grid-template-columns:minmax(0,1fr)}}
@media(max-width:560px){.nt-card,.nt-confirm,.nt-pending,.nt-keys,.nt-message{padding:12px}.nt-heading{align-items:stretch;flex-direction:column}.nt-heading .nt-button{align-self:flex-start}.nt-card-heading{align-items:flex-start;flex-wrap:wrap}.nt-audit{grid-template-columns:minmax(0,1fr);gap:2px}.nt-actions{align-items:stretch;flex-direction:column-reverse}.nt-actions .nt-button{width:100%}.nt-pagination .nt-button{flex:1}}
</style>
