<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from './i18n'
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from '@lottery/shared'
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
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
import { adminKey, useAdminI18n } from "./i18n";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const { t, display } = useAdminI18n();
const emit = defineEmits<{ (event: "session-invalid"): void }>();

const PAGE_SIZE = 20;
const api = createNotificationDeliveryApi();
const rights = computed(() => deliveryPermissions(props.account, props.brandId));
const items = ref<Delivery[]>([]);
const offset = ref(0);
const loading = ref(false);
const writing = ref(false);
const error = ref<string | import("@lottery/shared").LocalizedMessage>("");
const notice = ref("");
const retryReason = ref("");
const review = ref<PendingDeliveryRetry | null>(null);
const confirmed = ref(false);
const reviewHeading = ref<HTMLElement | null>(null);
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
  !writing.value && !loading.value && reasonBytes.value <= 500,
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
  error.value = problem instanceof Error ? publicErrorMessage(problem) : "请求失败，请重试。";
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
  void nextTick(() => {
    reviewHeading.value?.focus({ preventScroll: true });
    reviewHeading.value?.scrollIntoView({ block: "start" });
  });
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
        error.value = problem instanceof Error ? publicErrorMessage(problem) : "重试回执无效。";
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
async function confirmReview() {
  const intent = review.value;
  if (!intent || !confirmed.value || !canReview.value) return;
  await submitRetry(intent);
}
async function retryPending() {
  const intent = pending.value;
  if (!intent || writing.value || !rights.value.retry || props.account.super_admin) return;
  await submitRetry(intent);
}
async function changePage(nextOffset: number) {
  if (loading.value || writing.value || nextOffset < 0) return;
  notice.value = "";
  await readPage(nextOffset);
}
function statusLabel(status: string) {
  const labels: Record<string, string> = { pending: t('admin.copy.99f247fd'), sent: t('admin.copy.a657cfe0'), failed: t('admin.copy.1feabec0') };
  return labels[status] ?? status;
}
function localizedNotice(value: string): string {
  const accepted = /^retryAccepted:(pending|sent|failed)$/.exec(value);
  if (accepted) {
    const statusCopy: Record<string, string> = {
      pending: 'admin.copy.99f247fd', sent: 'admin.copy.a657cfe0', failed: 'admin.copy.1feabec0',
    };
    const status = t(adminKey(statusCopy[accepted[1]!]!));
    return t('admin.notification.retryAccepted', { status });
  }
  const translations: Record<string, string> = {
    "正在使用原请求体和幂等键重试…": 'admin.copy.9bf6f293',
    "正在提交已核对的投递重试…": 'admin.copy.662c6b9a',
    "回执无法核对，写入结果未知。原请求体和幂等键仍保留。": 'admin.copy.40126e29',
    "重试已由服务器确认，但后续状态读取失败。写入保持已确认，请使用只读刷新查看进度。": 'admin.copy.7ae22682',
    "重试已由服务器接受，并已读取最新投递状态。": 'admin.copy.c3b71397',
    "重试结果未知。原请求体和幂等键仍保留；只读刷新不能确认或清除此操作。": 'admin.copy.d6a9fcde',
    "服务器明确拒绝了这次重试。请读取最新状态并核实原因后，再创建新的重试请求。": 'admin.copy.c1df93a0',
  };
  const pair = translations[value];
  return pair ? t(adminKey(pair)) : value;
}
function formatTime(value: string | null){ return formatDisplayDateTime(value, displayLocale.value); }

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
      <div><h2 id="nd-title">{{ t('admin.copy.c678a3d9') }}</h2></div>
      <button v-refresh-button="t('admin.copy.f08af547')" v-if="rights.view" class="nd-button button-secondary" type="button" :aria-label="t('admin.copy.f08af547')" :disabled="loading || writing" @click="withGlobalLoading(readPage)">{{ loading ? t('admin.copy.27342bf3') : t('admin.copy.894e2a7d') }}</button>
    </header>

    <p v-if="!rights.view" class="nd-message" role="note">{{ t('admin.copy.35a80dde') }}</p>
    <template v-else>
      <p v-if="error" class="nd-message error" role="alert">{{ displayPublicError(error) }}</p>
      <p v-if="notice" class="nd-message" role="status">{{ localizedNotice(notice) }}</p>

      <aside v-if="pending" class="nd-pending" aria-labelledby="nd-pending-title">
        <strong id="nd-pending-title">{{ t('admin.copy.d6d50119') }}</strong>
        <p>{{ t('admin.copy.4515713e') }}</p>
        <dl class="nd-facts">
          <div><dt>{{ t('admin.copy.362d374e') }}</dt><dd class="nd-break"><AssociationName kind="brand" :id="pending.brandId" :locale="associationLocale" /> / {{ pending.eventId }}</dd></div>
          <div><dt>{{ t('admin.copy.2dbbd92c') }}</dt><dd>{{ pending.body.attempt_count }}</dd></div>
          <div v-if="pending.body.reason"><dt>{{ t('admin.copy.8107ea06') }}</dt><dd class="nd-break">{{ pending.body.reason }}</dd></div>

        </dl>
        <button v-write-guard="writing" v-if="rights.retry && !props.account.super_admin" class="nd-button button-primary" type="button" :disabled="writing" @click="withGlobalLoading(retryPending)">{{ writing ? t('admin.copy.20e088a1') : t('admin.copy.db0c4d5a') }}</button>
        <p v-else class="nd-note">{{ t('admin.copy.0056b123') }}</p>
      </aside>

      <div class="nd-table-wrap" role="region" :aria-label="t('admin.copy.e41e4275')" tabindex="0">
        <table class="nd-table">
          <thead><tr><th scope="col">{{ t('admin.copy.a765b3ca') }}</th><th scope="col">{{ t('admin.copy.6842b00e') }}</th><th scope="col">{{ t('admin.copy.0ca65814') }}</th><th scope="col">{{ t('admin.copy.bbf1c1e7') }}</th><th scope="col">{{ t('admin.copy.40bfc04b') }}</th><th scope="col">{{ t('admin.copy.231efd41') }}</th><th v-if="rights.retry && !props.account.super_admin" scope="col">{{ t('admin.copy.d21fcbef') }}</th></tr></thead>
          <tbody>
            <tr v-for="delivery in items" :key="delivery.event_id">
              <td :data-label="t('admin.copy.a765b3ca')"><span class="nd-status" :class="`status-${delivery.status}`">{{ statusLabel(delivery.status) }}</span></td>
              <td :data-label="t('admin.copy.6842b00e')" class="nd-break nd-id">{{ delivery.event_id }}</td>
              <td :data-label="t('admin.copy.0ca65814')">{{ delivery.attempt_count }}</td>
              <td :data-label="t('admin.copy.bbf1c1e7')" class="nd-break">{{ delivery.last_error ? displayPublicError(publicErrorMessage({ code: delivery.last_error })) : "—" }}</td>
              <td :data-label="t('admin.copy.40bfc04b')">{{ formatTime(delivery.next_attempt_at) }}</td>
              <td :data-label="t('admin.copy.231efd41')">{{ formatTime(delivery.sent_at) }}</td>
              <td v-if="rights.retry && !props.account.super_admin" :data-label="t('admin.copy.d21fcbef')">
                <button v-if="delivery.status === 'failed'" class="nd-button button-secondary" type="button" :aria-label="t('admin.notification.reviewRetry', { event: delivery.event_id })" :disabled="!canReview || Boolean(pending)" @click="startReview(delivery)">{{ t('admin.copy.57cc92e6') }}</button>
                <span v-else class="nd-note">{{ t('admin.copy.2181bc73') }}</span>
              </td>
            </tr>
            <tr v-if="!items.length && !loading"><td class="nd-empty empty-state" :colspan="rights.retry && !props.account.super_admin ? 7 : 6">{{ t('admin.copy.19a33759') }}</td></tr>
            <tr v-global-loading v-if="loading && !items.length"><td class="nd-empty empty-state" :colspan="rights.retry && !props.account.super_admin ? 7 : 6">{{ t('admin.copy.d5234310') }}</td></tr>
          </tbody>
        </table>
      </div>

      <footer class="nd-pagination" :aria-label="t('admin.copy.c52388a4')">
        <button class="nd-button button-secondary" type="button" :disabled="offset === 0 || loading || writing" @click="withGlobalLoading(changePage, Math.max(0, offset - PAGE_SIZE))">{{ t('admin.copy.dbe7a5d7') }}</button>
        <span>{{ t('admin.copy.d116a312') }} {{ Math.floor(offset / PAGE_SIZE) + 1 }} {{ t('admin.copy.75a35d58') }} {{ items.length }} {{ t('admin.copy.44d96226') }}</span>
        <button class="nd-button button-secondary" type="button" :disabled="!hasNext || loading || writing" @click="withGlobalLoading(changePage, offset + PAGE_SIZE)">{{ t('admin.copy.2e9c9e7a') }}</button>
      </footer>

      <section v-if="review && !pending" class="nd-confirm" aria-labelledby="nd-confirm-title">
        <strong id="nd-confirm-title" ref="reviewHeading" tabindex="-1">{{ t('admin.copy.57cc92e6') }}</strong>
        <p>{{ t('admin.copy.234a29a7') }}</p>
        <dl class="nd-facts">
          <div><dt>{{ t('admin.copy.4b2d7fdd') }}</dt><dd class="nd-break"><AssociationName kind="brand" :id="review.brandId" :locale="associationLocale" /></dd></div>
          <div><dt>{{ t('admin.copy.6842b00e') }}</dt><dd class="nd-break">{{ review.eventId }}</dd></div>
          <div><dt>{{ t('admin.copy.0072ddae') }}</dt><dd>{{ review.body.attempt_count }}</dd></div>
          <div v-if="review.body.reason"><dt>{{ t('admin.copy.d43a1026') }}</dt><dd class="nd-break">{{ review.body.reason }}</dd></div>

        </dl>
        <label class="nd-check"><input v-model="confirmed" type="checkbox" :aria-label="t('admin.copy.73d7b1b2')" />{{ t('admin.copy.fce6c713') }}</label>
        <div class="nd-actions">
          <button class="nd-button button-secondary" type="button" :disabled="writing" @click="clearReview">{{ t('admin.copy.bf7400b6') }}</button>
          <button v-write-guard="writing" class="nd-button button-primary" type="button" :disabled="!confirmed || writing || !rights.retry || props.account.super_admin" @click="withGlobalLoading(confirmReview)">{{ writing ? t('admin.copy.46511dc0') : t('admin.copy.9dcca427') }}</button>
        </div>
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
#nd-confirm-title{scroll-margin-block-start:24px}.notification-deliveries{display:grid;gap:16px;min-width:0;color:var(--ink)}.nd-heading,.nd-pagination,.nd-actions{display:flex;align-items:center;justify-content:space-between;gap:12px}.nd-heading h2{margin:2px 0 0;font-size:1.25rem}.nd-eyebrow{margin:0;color:#68758a;font-size:.78rem}.nd-message,.nd-pending,.nd-confirm{border:1px solid var(--line);border-radius:12px;padding:14px;background:var(--surface)}.nd-message{margin:0;color:#344054}.nd-message.error{background:#fff0ee;border-color:#df9385;color:#a32e1a}.nd-pending{display:grid;gap:10px;background:#fff1ee;border-color:#df9385}.nd-pending p,.nd-confirm p{margin:0}.nd-facts{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px 18px;margin:0}.nd-facts>div{min-width:0;padding:7px 0;border-bottom:1px solid #eef1f5}.nd-facts dt{color:#697586;font-size:.78rem}.nd-facts dd{margin:4px 0 0;font-weight:550}.nd-break{overflow-wrap:anywhere;word-break:break-word}.nd-table-wrap{max-width:100%;overflow-x:auto;border:1px solid var(--line);border-radius:10px;background:var(--surface)}.nd-table{width:100%;border-collapse:collapse;text-align:left;font-size:.9rem}.nd-table th,.nd-table td{padding:10px 12px;border-bottom:1px solid #eef1f5;vertical-align:top}.nd-table th{background:#f6f8fb;color:#536176;font-size:.8rem;white-space:nowrap}.nd-table tbody tr:last-child td{border-bottom:0}.nd-id{min-width:190px}.nd-status{display:inline-flex;border-radius:99px;padding:4px 9px;background:#edf0f5;white-space:nowrap}.status-sent{background:#e6f6eb;color:#176b37}.status-failed{background:#fff0ee;color:#a32e1a}.status-pending{background:#fff5dd;color:#725708}.nd-button{border:1px solid #c5ceda;border-radius:8px;padding:8px 12px;font:inherit;cursor:pointer;background:var(--surface);white-space:nowrap}.nd-button.primary{background:var(--primary);border-color:var(--primary);color:var(--surface)}.nd-button:disabled{opacity:.52;cursor:not-allowed}.nd-note,.nd-retry-form small,.nd-pagination span{color:var(--muted);font-size:.86rem}.nd-pagination{justify-content:center;flex-wrap:wrap}.nd-confirm{display:grid;gap:12px}.nd-confirm .wide{grid-column:1/-1}.nd-check{display:flex;align-items:flex-start;gap:9px}.nd-check input{margin-top:.25em}.nd-actions{justify-content:flex-end}.nd-retry-form{display:grid;gap:8px;max-width:720px}.nd-retry-form label{font-size:.9rem}.nd-retry-form textarea{box-sizing:border-box;width:100%;border:1px solid var(--line);border-radius:8px;padding:9px 10px;font:inherit;resize:vertical}
@media(max-width:700px){.notification-deliveries{gap:12px}.nd-heading{align-items:flex-start}.nd-heading>.nd-button{white-space:normal}.nd-pending,.nd-confirm,.nd-message{padding:12px}.nd-facts{grid-template-columns:minmax(0,1fr);gap:2px}.nd-table-wrap{overflow:visible;border:0;background:transparent}.nd-table,.nd-table tbody{display:block;width:100%}.nd-table thead{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0}.nd-table tr{display:grid;gap:0;margin-bottom:10px;padding:8px 12px;border:1px solid var(--line);border-radius:10px;background:var(--surface)}.nd-table td{display:grid;grid-template-columns:minmax(105px,34%) minmax(0,1fr);gap:10px;padding:8px 0;border-bottom:1px solid #eef1f5}.nd-table td::before{content:attr(data-label);color:#697586;font-size:.8rem}.nd-table td:last-child{border-bottom:0}.nd-table td.nd-empty{display:block}.nd-table td.nd-empty::before{content:none}.nd-actions{align-items:stretch;flex-direction:column-reverse}.nd-actions .nd-button{width:100%}.nd-pagination .nd-button{flex:1}.nd-confirm .wide{grid-column:auto}}
</style>
