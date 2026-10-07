<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import {
  createNotificationDeliveryApi,
  deliveryPermissions,
  type Delivery,
} from "./notification-delivery-api";
import {
  classifyDeliveryRetryFailure,
  clearPendingDeliveryRetry,
  freezeDeliveryRetryBody,
  getPendingDeliveryRetry,
  scopeKey,
  setPendingDeliveryRetry,
  type PendingDeliveryRetry,
} from "./notification-delivery-state";
import { useAdminI18n } from "./i18n";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const { t } = useAdminI18n();
const emit = defineEmits<{ (event: "session-invalid"): void }>();

const PAGE_SIZE = 20;
const api = createNotificationDeliveryApi();
const rights = computed(() => deliveryPermissions(props.account, props.brandId));
const items = ref<Delivery[]>([]);
const offset = ref(0);
const loading = ref(false);
const writing = ref(false);
const error = ref("");
const notice = ref("");
const retryReason = ref("");
const review = ref<PendingDeliveryRetry | null>(null);
const confirmed = ref(false);
const pending = ref<PendingDeliveryRetry | null>(null);
let alive = true;
let readTicket = 0;
let writeTicket = 0;

const activeScope = computed(() => scopeKey(props.account.id, props.brandId));
const permissionScope = computed(() => JSON.stringify([
  props.account.id, props.account.super_admin, props.account.brand_ids,
  props.account.permissions, props.account.permissions_by_brand,
  props.account.platform_permissions, props.brandId, rights.value,
]));
const hasNext = computed(() => items.value.length === PAGE_SIZE);
const reasonBytes = computed(() => new TextEncoder().encode(retryReason.value.trim()).length);
const canReview = computed(() => Boolean(
  rights.value.view && rights.value.retry && !props.account.super_admin && !pending.value &&
  !writing.value && !loading.value && reasonBytes.value > 0 && reasonBytes.value <= 500,
));

function isCurrent(ticket: number, capturedScope: string, capturedPermissions: string, permitted = rights.value.view) {
  return alive && ticket === readTicket && capturedScope === activeScope.value &&
    capturedPermissions === permissionScope.value && permitted;
}
function isCurrentWrite(ticket: number, capturedScope: string, capturedPermissions: string) {
  return alive && ticket === writeTicket && capturedScope === activeScope.value &&
    capturedPermissions === permissionScope.value;
}
function refreshPending() {
  pending.value = getPendingDeliveryRetry(activeScope.value);
}
function clearReview() {
  review.value = null;
  confirmed.value = false;
}
function clearScopeView() {
  readTicket += 1;
  writeTicket += 1;
  loading.value = false;
  writing.value = false;
  items.value = [];
  offset.value = 0;
  error.value = "";
  notice.value = "";
  retryReason.value = "";
  clearReview();
  refreshPending();
}
function invalidateSession(capturedScope = activeScope.value, capturedPermissions = permissionScope.value) {
  if (!alive || capturedScope !== activeScope.value || capturedPermissions !== permissionScope.value) return;
  clearPendingDeliveryRetry(capturedScope);
  pending.value = null;
  clearReview();
  emit("session-invalid");
}
function showError(problem: unknown) {
  error.value = problem instanceof Error ? problem.message : "请求失败，请重试。";
}
function validatePage(page: Delivery[], brandId: string): Delivery[] {
  if (!Array.isArray(page)) throw new Error("投递列表响应格式无效，请重新读取。");
  for (const delivery of page) {
    if (!delivery || delivery.brand_id?.toLowerCase() !== brandId.toLowerCase() ||
      typeof delivery.event_id !== "string" || typeof delivery.status !== "string" ||
      !Number.isSafeInteger(delivery.attempt_count) || delivery.attempt_count < 0 ||
      !(delivery.last_error === null || typeof delivery.last_error === "string") ||
      typeof delivery.next_attempt_at !== "string" ||
      !(delivery.sent_at === null || typeof delivery.sent_at === "string"))
      throw new Error("投递列表包含无效或品牌不匹配的记录，已停止显示。");
  }
  return page;
}
function validateReceipt(value: Delivery, intent: PendingDeliveryRetry): Delivery {
  if (!value || typeof value !== "object" || value.brand_id?.toLowerCase() !== intent.brandId.toLowerCase() ||
    value.event_id?.toLowerCase() !== intent.eventId.toLowerCase() || value.status !== "pending" ||
    value.attempt_count !== intent.body.attempt_count ||
    !(value.last_error === null || typeof value.last_error === "string") ||
    typeof value.next_attempt_at !== "string" || value.sent_at !== null)
    throw new AdminApiError("重试回执与原投递不匹配，写入结果未知；请使用相同请求重试。", 502, "INVALID_RESPONSE");
  return value;
}
async function readPage(pageOffset = offset.value, capturedScope = activeScope.value, capturedPermissions = permissionScope.value) {
  if (!rights.value.view || !props.brandId) return false;
  const ticket = ++readTicket;
  const brandId = props.brandId;
  loading.value = true;
  error.value = "";
  try {
    const result = await api.list(brandId, PAGE_SIZE, pageOffset);
    if (!isCurrent(ticket, capturedScope, capturedPermissions)) return false;
    const valid = validatePage(result, brandId);
    items.value = valid;
    offset.value = pageOffset;
    return true;
  } catch (problem) {
    if (problem instanceof AdminApiError && problem.status === 401) {
      if (isCurrent(ticket, capturedScope, capturedPermissions, true)) invalidateSession(capturedScope, capturedPermissions);
      return false;
    }
    if (isCurrent(ticket, capturedScope, capturedPermissions)) showError(problem);
    return false;
  } finally {
    if (isCurrent(ticket, capturedScope, capturedPermissions, true)) loading.value = false;
  }
}

function startReview(delivery: Delivery) {
  if (!canReview.value || delivery.status !== "failed") return;
  const body = freezeDeliveryRetryBody({ attempt_count: delivery.attempt_count, reason: retryReason.value.trim() });
  review.value = Object.freeze({
    accountId: props.account.id,
    brandId: props.brandId,
    eventId: delivery.event_id,
    body,
    key: createIdempotencyKey(),
  });
  confirmed.value = false;
  error.value = "";
}
async function submitRetry(intent: PendingDeliveryRetry) {
  if (writing.value || !rights.value.retry || props.account.super_admin ||
    intent.accountId !== props.account.id || intent.brandId !== props.brandId ||
    intent !== pending.value && intent !== review.value) return;
  const capturedScope = activeScope.value;
  const capturedPermissions = permissionScope.value;
  const ticket = ++writeTicket;
  writing.value = true;
  error.value = "";
  notice.value = pending.value === intent
    ? "正在使用原请求体和幂等键重试…"
    : "正在提交已核对的投递重试…";
  // Record before sending so a remount or brand switch cannot lose an in-flight operation.
  setPendingDeliveryRetry(capturedScope, intent);
  pending.value = intent;
  clearReview();
  try {
    const receipt = await api.retry(intent.brandId, intent.eventId, intent.body, intent.key);
    let accepted: Delivery;
    try {
      accepted = validateReceipt(receipt, intent);
    } catch (problem) {
      if (isCurrentWrite(ticket, capturedScope, capturedPermissions)) {
        error.value = problem instanceof Error ? problem.message : "重试回执无效。";
        notice.value = "回执无法核对，写入结果未知。原请求体和幂等键仍保留。";
      }
      return;
    }

    // A valid accepted response acknowledges the write. Follow-up reads cannot turn it into unknown.
    clearPendingDeliveryRetry(capturedScope, intent.key);
    if (isCurrentWrite(ticket, capturedScope, capturedPermissions)) {
      pending.value = getPendingDeliveryRetry(capturedScope);
      // Store the enum value, not a locale-rendered label, so later locale
      // changes can re-render this notice without touching the frozen request.
      notice.value = `retryAccepted:${accepted.status}`;
      const readOk = await readPage(offset.value, capturedScope, capturedPermissions);
      if (!isCurrentWrite(ticket, capturedScope, capturedPermissions)) return;
      if (!readOk) notice.value = "重试已由服务器确认，但后续状态读取失败。写入保持已确认，请使用只读刷新查看进度。";
      else notice.value = "重试已由服务器接受，并已读取最新投递状态。";
    }
  } catch (problem) {
    if (problem instanceof AdminApiError && problem.status === 401) {
      if (isCurrentWrite(ticket, capturedScope, capturedPermissions)) invalidateSession(capturedScope, capturedPermissions);
      return;
    }
    const status = problem instanceof AdminApiError ? problem.status : 0;
    const failure = classifyDeliveryRetryFailure(status);
    if (failure === "uncertain") {
      if (isCurrentWrite(ticket, capturedScope, capturedPermissions)) {
        pending.value = intent;
        notice.value = "重试结果未知。原请求体和幂等键仍保留；只读刷新不能确认或清除此操作。";
        showError(problem);
      }
    } else {
      clearPendingDeliveryRetry(capturedScope, intent.key);
      if (isCurrentWrite(ticket, capturedScope, capturedPermissions)) {
        pending.value = getPendingDeliveryRetry(capturedScope);
        notice.value = "服务器明确拒绝了这次重试。请读取最新状态并核实原因后，再创建新的重试请求。";
        showError(problem);
      }
    }
  } finally {
    if (isCurrentWrite(ticket, capturedScope, capturedPermissions)) writing.value = false;
  }
}
function confirmReview() {
  const intent = review.value;
  if (!intent || !confirmed.value || !canReview.value) return;
  void submitRetry(intent);
}
function retryPending() {
  const intent = pending.value;
  if (!intent || writing.value || !rights.value.retry || props.account.super_admin) return;
  void submitRetry(intent);
}
function changePage(nextOffset: number) {
  if (loading.value || writing.value || nextOffset < 0) return;
  notice.value = "";
  void readPage(nextOffset);
}
function statusLabel(status: string) {
  const labels: Record<string, string> = { pending: t("待处理", "Pending"), sent: t("已投递", "Sent"), failed: t("失败", "Failed") };
  return labels[status] ?? status;
}
function localizedNotice(value: string): string {
  const accepted = /^retryAccepted:(pending|sent|failed)$/.exec(value);
  if (accepted) {
    const statusCopy: Record<string, [string, string]> = {
      pending: ["待处理", "Pending"], sent: ["已投递", "Sent"], failed: ["失败", "Failed"],
    };
    const [zhStatus, enStatus] = statusCopy[accepted[1]!]!;
    return t(`服务器已接受重试；回执状态为“${zhStatus}”。正在读取最新投递状态。`, `The server accepted the retry with receipt status “${enStatus}”. Loading the latest delivery status.`);
  }
  const translations: Record<string, [string, string]> = {
    "正在使用原请求体和幂等键重试…": ["正在使用原请求体和幂等键重试…", "Retrying with the original request body and idempotency key…"],
    "正在提交已核对的投递重试…": ["正在提交已核对的投递重试…", "Submitting the reviewed delivery retry…"],
    "回执无法核对，写入结果未知。原请求体和幂等键仍保留。": ["回执无法核对，写入结果未知。原请求体和幂等键仍保留。", "The receipt could not be verified, so the write outcome is unknown. The original request body and idempotency key are retained."],
    "重试已由服务器确认，但后续状态读取失败。写入保持已确认，请使用只读刷新查看进度。": ["重试已由服务器确认，但后续状态读取失败。写入保持已确认，请使用只读刷新查看进度。", "The server confirmed the retry, but the follow-up status read failed. The write remains confirmed; use read-only refresh to check progress."],
    "重试已由服务器接受，并已读取最新投递状态。": ["重试已由服务器接受，并已读取最新投递状态。", "The server accepted the retry and the latest delivery status was read."],
    "重试结果未知。原请求体和幂等键仍保留；只读刷新不能确认或清除此操作。": ["重试结果未知。原请求体和幂等键仍保留；只读刷新不能确认或清除此操作。", "The retry outcome is unknown. The original request body and idempotency key are retained; a read-only refresh cannot confirm or clear this operation."],
    "服务器明确拒绝了这次重试。请读取最新状态并核实原因后，再创建新的重试请求。": ["服务器明确拒绝了这次重试。请读取最新状态并核实原因后，再创建新的重试请求。", "The server definitively rejected this retry. Read the latest status and verify the reason before creating a new retry request."],
  };
  const pair = translations[value];
  return pair ? t(pair[0], pair[1]) : value;
}
function formatTime(value: string | null) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

watch(permissionScope, () => {
  clearScopeView();
  if (alive && rights.value.view) void readPage(0);
}, { flush: "sync" });
watch(retryReason, () => { if (review.value) clearReview(); });
onMounted(() => { refreshPending(); if (rights.value.view) void readPage(0); });
onBeforeUnmount(() => { alive = false; readTicket += 1; writeTicket += 1; });
</script>

<template>
  <section class="notification-deliveries" aria-labelledby="nd-title">
    <header class="nd-heading">
      <div><p class="nd-eyebrow">{{ t("通知服务 · 投递审计", "Notifications · Delivery audit") }}</p><h2 id="nd-title">{{ t("通知投递记录", "Notification deliveries") }}</h2></div>
      <button v-if="rights.view" class="nd-button secondary" type="button" :aria-label="t('只读刷新通知投递记录', 'Refresh notification deliveries (read only)')" :disabled="loading || writing" @click="readPage()">{{ loading ? t("读取中…", "Loading…") : t("刷新", "Refresh") }}</button>
    </header>

    <p v-if="!rights.view" class="nd-message" role="note">{{ t("当前账号没有此品牌的通知投递查看权限。超级管理员仅在具备平台查看权限时可读取，且不能执行重试。", "This account cannot view notification deliveries for this brand. Super admins can read them only with platform view access and cannot retry deliveries.") }}</p>
    <template v-else>
      <p v-if="error" class="nd-message error" role="alert">{{ error }}</p>
      <p v-if="notice" class="nd-message" role="status">{{ localizedNotice(notice) }}</p>

      <aside v-if="pending" class="nd-pending" aria-labelledby="nd-pending-title">
        <strong id="nd-pending-title">{{ t("重试结果未知", "Retry outcome unknown") }}</strong>
        <p>{{ t("服务器可能已接受此操作。只读刷新不会确认或清除此请求；只能使用下列原请求体和幂等键重试。", "The server may have accepted this operation. A read-only refresh will not confirm or clear it; retry only with the original request body and idempotency key below.") }}</p>
        <dl class="nd-facts">
          <div><dt>{{ t("品牌 / 事件", "Brand / event") }}</dt><dd class="nd-break">{{ pending.brandId }} / {{ pending.eventId }}</dd></div>
          <div><dt>{{ t("冻结尝试次数", "Frozen attempt count") }}</dt><dd>{{ pending.body.attempt_count }}</dd></div>
          <div><dt>{{ t("冻结原因", "Frozen reason") }}</dt><dd class="nd-break">{{ pending.body.reason }}</dd></div>
          <div><dt>{{ t("幂等键", "Idempotency key") }}</dt><dd class="nd-break">{{ pending.key }}</dd></div>
        </dl>
        <button v-if="rights.retry && !props.account.super_admin" class="nd-button primary" type="button" :disabled="writing" @click="retryPending">{{ writing ? t("按原请求提交中…", "Submitting original request…") : t("按原请求重试", "Retry original request") }}</button>
        <p v-else class="nd-note">{{ t("当前账号不能重试此操作；请求意图仍保留在本页会话内存中。", "This account cannot retry this operation; the request intent remains in this page's session memory.") }}</p>
      </aside>

      <div class="nd-table-wrap" role="region" :aria-label="t('通知投递记录表格', 'Notification delivery table')" tabindex="0">
        <table class="nd-table">
          <thead><tr><th scope="col">{{ t("状态", "Status") }}</th><th scope="col">{{ t("事件 ID", "Event ID") }}</th><th scope="col">{{ t("尝试次数", "Attempts") }}</th><th scope="col">{{ t("最后错误", "Last error") }}</th><th scope="col">{{ t("下次尝试时间", "Next attempt") }}</th><th scope="col">{{ t("投递时间", "Delivered at") }}</th><th v-if="rights.retry && !props.account.super_admin" scope="col">{{ t("操作", "Actions") }}</th></tr></thead>
          <tbody>
            <tr v-for="delivery in items" :key="delivery.event_id">
              <td :data-label="t('状态', 'Status')"><span class="nd-status" :class="`status-${delivery.status}`">{{ statusLabel(delivery.status) }}</span></td>
              <td :data-label="t('事件 ID', 'Event ID')" class="nd-break nd-id">{{ delivery.event_id }}</td>
              <td :data-label="t('尝试次数', 'Attempts')">{{ delivery.attempt_count }}</td>
              <td :data-label="t('最后错误', 'Last error')" class="nd-break">{{ delivery.last_error ?? "—" }}</td>
              <td :data-label="t('下次尝试时间', 'Next attempt')">{{ formatTime(delivery.next_attempt_at) }}</td>
              <td :data-label="t('投递时间', 'Delivered at')">{{ formatTime(delivery.sent_at) }}</td>
              <td v-if="rights.retry && !props.account.super_admin" :data-label="t('操作', 'Actions')">
                <button v-if="delivery.status === 'failed'" class="nd-button secondary" type="button" :aria-label="t(`核对重试事件 ${delivery.event_id}`, `Review retry for event ${delivery.event_id}`)" :disabled="!canReview || Boolean(pending)" @click="startReview(delivery)">{{ t("核对重试", "Review retry") }}</button>
                <span v-else class="nd-note">{{ t("仅失败记录可重试", "Only failed deliveries can be retried") }}</span>
              </td>
            </tr>
            <tr v-if="!items.length && !loading"><td class="nd-empty" :colspan="rights.retry && !props.account.super_admin ? 7 : 6">{{ t("此页没有投递记录。", "No delivery records on this page.") }}</td></tr>
            <tr v-if="loading && !items.length"><td class="nd-empty" :colspan="rights.retry && !props.account.super_admin ? 7 : 6">{{ t("正在读取投递记录…", "Loading notification deliveries…") }}</td></tr>
          </tbody>
        </table>
      </div>

      <footer class="nd-pagination" :aria-label="t('投递记录分页', 'Delivery pagination')">
        <button class="nd-button secondary" type="button" :disabled="offset === 0 || loading || writing" @click="changePage(Math.max(0, offset - PAGE_SIZE))">{{ t("上一页", "Previous") }}</button>
        <span>{{ t("第", "Page") }} {{ Math.floor(offset / PAGE_SIZE) + 1 }} {{ t("页 ·", "·") }} {{ items.length }} {{ t("条", "records") }}</span>
        <button class="nd-button secondary" type="button" :disabled="!hasNext || loading || writing" @click="changePage(offset + PAGE_SIZE)">{{ t("下一页", "Next") }}</button>
      </footer>

      <section v-if="review && !pending" class="nd-confirm" aria-labelledby="nd-confirm-title">
        <strong id="nd-confirm-title">{{ t("核对重试", "Review retry") }}</strong>
        <p>{{ t("即将使用服务器当前失败记录的尝试次数执行一次人工重试。该请求不会自动再次发送。", "One manual retry will use the attempt count on the server's current failed record. This request will not be sent again automatically.") }}</p>
        <dl class="nd-facts">
          <div><dt>{{ t("品牌 ID", "Brand ID") }}</dt><dd class="nd-break">{{ review.brandId }}</dd></div>
          <div><dt>{{ t("事件 ID", "Event ID") }}</dt><dd class="nd-break">{{ review.eventId }}</dd></div>
          <div><dt>{{ t("原尝试次数（冻结）", "Original attempt count (frozen)") }}</dt><dd>{{ review.body.attempt_count }}</dd></div>
          <div><dt>{{ t("请求原因（冻结）", "Request reason (frozen)") }}</dt><dd class="nd-break">{{ review.body.reason }}</dd></div>
          <div class="wide"><dt>{{ t("幂等键（冻结）", "Idempotency key (frozen)") }}</dt><dd class="nd-break">{{ review.key }}</dd></div>
        </dl>
        <label class="nd-check"><input v-model="confirmed" type="checkbox" :aria-label="t('我已核对品牌、事件、尝试次数、原因和幂等键，并确认重试', 'I reviewed the brand, event, attempt count, reason, and idempotency key, and confirm the retry')" />{{ t("我已核对品牌、事件、尝试次数、原因和幂等键，确认提交", "I reviewed the brand, event, attempt count, reason, and idempotency key, and confirm submission") }}</label>
        <div class="nd-actions">
          <button class="nd-button secondary" type="button" :disabled="writing" @click="clearReview">{{ t("返回修改", "Back to edit") }}</button>
          <button class="nd-button primary" type="button" :disabled="!confirmed || writing || !rights.retry || props.account.super_admin" @click="confirmReview">{{ writing ? t("提交中…", "Submitting…") : t("确认重试", "Confirm retry") }}</button>
        </div>
      </section>

      <section v-if="rights.retry && !props.account.super_admin && !pending" class="nd-retry-form" :aria-label="t('失败通知人工重试原因', 'Manual retry reason for failed notification')">
        <label for="nd-retry-reason">{{ t("重试原因（最多 500 UTF-8 字节）", "Retry reason (up to 500 UTF-8 bytes)") }}</label>
        <textarea id="nd-retry-reason" v-model="retryReason" rows="3" maxlength="500" :disabled="writing" :placeholder="t('说明重试依据', 'Explain the basis for retrying')" />
        <small>{{ reasonBytes }} {{ t("/ 500 字节。选择失败记录后，请核对冻结请求并再次确认。", "/ 500 bytes. Select a failed delivery, review the frozen request, and confirm again.") }}</small>
      </section>
    </template>
  </section>
</template>

<style scoped>
.nd-table { white-space:normal; min-width:0; }
.nd-table td { font-size:inherit; height:auto; }
@media(max-width:700px) {
  .notification-deliveries .nd-table, .notification-deliveries .nd-table tbody,
  .notification-deliveries .nd-table tr, .notification-deliveries .nd-table td {
    min-width:0; height:auto; white-space:normal; box-sizing:border-box;
  }
  .notification-deliveries .nd-id { min-width:0; }
}
.notification-deliveries{display:grid;gap:16px;min-width:0;color:#172033}.nd-heading,.nd-pagination,.nd-actions{display:flex;align-items:center;justify-content:space-between;gap:12px}.nd-heading h2{margin:2px 0 0;font-size:1.25rem}.nd-eyebrow{margin:0;color:#68758a;font-size:.78rem}.nd-message,.nd-pending,.nd-confirm{border:1px solid #d9e0ea;border-radius:12px;padding:14px;background:#fff}.nd-message{margin:0;color:#344054}.nd-message.error{background:#fff0ee;border-color:#df9385;color:#a32e1a}.nd-pending{display:grid;gap:10px;background:#fff1ee;border-color:#df9385}.nd-pending p,.nd-confirm p{margin:0}.nd-facts{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px 18px;margin:0}.nd-facts>div{min-width:0;padding:7px 0;border-bottom:1px solid #eef1f5}.nd-facts dt{color:#697586;font-size:.78rem}.nd-facts dd{margin:4px 0 0;font-weight:550}.nd-break{overflow-wrap:anywhere;word-break:break-word}.nd-table-wrap{max-width:100%;overflow-x:auto;border:1px solid #d9e0ea;border-radius:10px;background:#fff}.nd-table{width:100%;border-collapse:collapse;text-align:left;font-size:.9rem}.nd-table th,.nd-table td{padding:10px 12px;border-bottom:1px solid #eef1f5;vertical-align:top}.nd-table th{background:#f6f8fb;color:#536176;font-size:.8rem;white-space:nowrap}.nd-table tbody tr:last-child td{border-bottom:0}.nd-id{min-width:190px}.nd-status{display:inline-flex;border-radius:99px;padding:4px 9px;background:#edf0f5;white-space:nowrap}.status-sent{background:#e6f6eb;color:#176b37}.status-failed{background:#fff0ee;color:#a32e1a}.status-pending{background:#fff5dd;color:#725708}.nd-button{border:1px solid #c5ceda;border-radius:8px;padding:8px 12px;font:inherit;cursor:pointer;background:#fff;white-space:nowrap}.nd-button.primary{background:#254a83;border-color:#254a83;color:#fff}.nd-button:disabled{opacity:.52;cursor:not-allowed}.nd-note,.nd-retry-form small,.nd-pagination span{color:#667085;font-size:.86rem}.nd-empty{text-align:center;color:#667085;padding:22px!important}.nd-pagination{justify-content:center;flex-wrap:wrap}.nd-confirm{display:grid;gap:12px}.nd-confirm .wide{grid-column:1/-1}.nd-check{display:flex;align-items:flex-start;gap:9px}.nd-check input{margin-top:.25em}.nd-actions{justify-content:flex-end}.nd-retry-form{display:grid;gap:8px;max-width:720px}.nd-retry-form label{font-size:.9rem}.nd-retry-form textarea{box-sizing:border-box;width:100%;border:1px solid #cbd3df;border-radius:8px;padding:9px 10px;font:inherit;resize:vertical}
@media(max-width:700px){.notification-deliveries{gap:12px}.nd-heading{align-items:flex-start}.nd-heading>.nd-button{white-space:normal}.nd-pending,.nd-confirm,.nd-message{padding:12px}.nd-facts{grid-template-columns:minmax(0,1fr);gap:2px}.nd-table-wrap{overflow:visible;border:0;background:transparent}.nd-table,.nd-table tbody{display:block;width:100%}.nd-table thead{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0}.nd-table tr{display:grid;gap:0;margin-bottom:10px;padding:8px 12px;border:1px solid #d9e0ea;border-radius:10px;background:#fff}.nd-table td{display:grid;grid-template-columns:minmax(105px,34%) minmax(0,1fr);gap:10px;padding:8px 0;border-bottom:1px solid #eef1f5}.nd-table td::before{content:attr(data-label);color:#697586;font-size:.8rem}.nd-table td:last-child{border-bottom:0}.nd-table td.nd-empty{display:block}.nd-table td.nd-empty::before{content:none}.nd-actions{align-items:stretch;flex-direction:column-reverse}.nd-actions .nd-button{width:100%}.nd-pagination .nd-button{flex:1}.nd-confirm .wide{grid-column:auto}}
</style>
