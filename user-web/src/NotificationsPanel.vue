<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import type { Language } from "../../shared/src/brand";
import {
  createNotificationApi,
  type NotificationItem,
} from "./notification-api";
import { renderNotification } from "./notification-presentation";

const props = defineProps<{ brandCode?: string; locale: Language }>();
const emit = defineEmits<{
  "unread-count": [count: string | null];
  "auth-expired": [];
}>();

type Context = { brand_id: string; member_id: string };
type PendingIntent = {
  ids: string[];
  context: Context;
  idempotencyKey: string;
};

const items = ref<NotificationItem[]>([]);
const context = ref<Context | null>(null);
const limit = 20;
const offset = ref(0);
const total = ref<number | null>(null);
const loading = ref(false);
const writing = ref(false);
const error = ref("");
const pending = ref<PendingIntent | null>(null);
const scopeGeneration = ref(0);
const listGeneration = ref(0);
const disposed = ref(false);
const api = ref(createNotificationApi({ brandCode: props.brandCode }));
const isZh = computed(() => props.locale === "zh");
const pageNumber = computed(() => Math.floor(offset.value / limit) + 1);
const hasPrevious = computed(() => offset.value > 0);
const hasNext = computed(
  () => total.value !== null && offset.value + limit < total.value,
);
const unreadOnPage = computed(() => items.value.filter((item) => !item.read_at));

function text(en: string, zh: string): string {
  return isZh.value ? zh : en;
}

function isCurrent(generation: number): boolean {
  return !disposed.value && generation === scopeGeneration.value;
}

function sameContext(a: Context | null, b: Context): boolean {
  return a?.brand_id === b.brand_id && a.member_id === b.member_id;
}

function clearAccountState() {
  items.value = [];
  context.value = null;
  pending.value = null;
  offset.value = 0;
  total.value = null;
  emit("unread-count", null);
}

function handleAuthExpired() {
  clearAccountState();
  error.value = text("Your session expired. Please sign in again.", "登录已过期，请重新登录。");
  emit("auth-expired");
}

function errorMessage(cause: unknown): string {
  const err = cause as { status?: number; message?: string };
  if (err?.status === 401) return text("Your session expired. Please sign in again.", "登录已过期，请重新登录。");
  if (err?.status === 0 || (err?.status !== undefined && err.status >= 500)) {
    return text("The request result is unknown. Retry the same action; this page will not assume it succeeded.", "请求结果暂时无法确认。可重试同一操作；页面不会自行推断操作已成功。");
  }
  return text("Notifications could not be loaded. Try again.", "暂时无法加载通知，请重试。");
}

function presentation(item: NotificationItem) {
  try {
    return renderNotification(item, props.locale);
  } catch {
    return {
      title: text("Notification unavailable", "通知内容暂不可用"),
      body: text("This notification cannot be displayed.", "暂时无法显示此通知。"),
      createdAt: item.created_at,
      points: null,
      reference: null,
      protectedNote: null,
    };
  }
}

async function loadPage(options: { resetError?: boolean; afterWrite?: boolean } = {}) {
  if (writing.value && !options.afterWrite) return;
  const generation = scopeGeneration.value;
  const request = ++listGeneration.value;
  if (options.resetError !== false && !pending.value) error.value = "";
  loading.value = true;
  try {
    const result = await api.value.list(limit, offset.value);
    if (!isCurrent(generation) || request !== listGeneration.value) return;
    const nextContext = { brand_id: result.brand_id, member_id: result.member_id };
    if (context.value && !sameContext(context.value, nextContext)) {
      // An account change invalidates any unresolved write identity.
      pending.value = null;
      items.value = [];
      offset.value = 0;
      context.value = nextContext;
      emit("unread-count", result.unread_count);
      if (result.offset !== 0) {
        await loadPage({ resetError: false });
        return;
      }
    } else {
      context.value = nextContext;
      emit("unread-count", result.unread_count);
    }
    items.value = result.items;
    total.value = result.offset + result.items.length + (result.items.length === limit ? 1 : 0);
    if (!pending.value) error.value = "";
  } catch (cause) {
    if (!isCurrent(generation) || request !== listGeneration.value) return;
    const err = cause as { status?: number };
    if (err?.status === 401) handleAuthExpired();
    else error.value = errorMessage(cause);
  } finally {
    if (isCurrent(generation) && request === listGeneration.value) loading.value = false;
  }
}

function createKey(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) return crypto.randomUUID();
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

async function submitIntent(intent: PendingIntent) {
  if (writing.value || loading.value || !isCurrent(scopeGeneration.value)) return;
  // Ignore any list response started before this write; it must not overwrite its receipt.
  listGeneration.value++;
  writing.value = true;
  error.value = "";
  const generation = scopeGeneration.value;
  try {
    const result = await api.value.markRead(
      [...intent.ids],
      { ...intent.context },
      intent.idempotencyKey,
    );
    if (!isCurrent(generation)) return;
    if (!sameContext(intent.context, { brand_id: result.brand_id, member_id: result.member_id })) {
      clearAccountState();
      error.value = text("Your account changed. Reload notifications.", "账号已切换，请重新加载通知。");
      return;
    }
    pending.value = null;
    emit("unread-count", result.unread_count);
    // A successful write refreshes through the current scope and current page.
    await loadPage({ resetError: false, afterWrite: true });
  } catch (cause) {
    if (!isCurrent(generation)) return;
    const err = cause as { status?: number };
    if (err?.status === 401) {
      pending.value = null;
      handleAuthExpired();
    } else {
      // Preserve the exact ids/context/key for an explicit same-request retry.
      if (err?.status !== undefined && err.status > 0 && err.status < 500) {
        pending.value = null;
        error.value = text("The request was not accepted. Reload and try again.", "请求未被接受。请重新加载后再试。");
      } else {
        pending.value = intent;
        error.value = errorMessage(cause);
      }
    }
  } finally {
    if (isCurrent(generation)) writing.value = false;
  }
}

function markRead(ids: string[]) {
  if (loading.value || writing.value || pending.value || ids.length === 0 || !context.value) return;
  const intent: PendingIntent = {
    ids: [...ids],
    context: { ...context.value },
    idempotencyKey: createKey(),
  };
  void submitIntent(intent);
}

function retryPending() {
  if (pending.value && !writing.value && !loading.value) void submitIntent(pending.value);
}

function changePage(nextOffset: number) {
  if (pending.value || writing.value || loading.value) return;
  offset.value = Math.max(0, nextOffset);
  void loadPage();
}

watch(
  () => props.brandCode,
  (brandCode) => {
    scopeGeneration.value++;
    listGeneration.value++;
    loading.value = false;
    writing.value = false;
    api.value = createNotificationApi({ brandCode });
    clearAccountState();
    error.value = "";
    void loadPage();
  },
);

onMounted(() => void loadPage());
onBeforeUnmount(() => {
  emit("unread-count", null);
  disposed.value = true;
  scopeGeneration.value++;
  listGeneration.value++;
});
</script>

<template>
  <section class="notifications-panel">
    <div class="page-heading notifications-heading">
      <div>
        <div class="eyebrow">{{ text("YOUR INBOX", "站内信") }}</div>
        <h1>{{ text("Updates", "消息") }}</h1>
        <p>
          {{ text("Messages for your current brand. External notifications are not connected.", "当前品牌的站内消息。外部通知暂未接入。") }}
        </p>
      </div>
    </div>

    <div class="notifications-toolbar">
      <button
        class="button button-secondary"
        type="button"
        :disabled="loading || writing"
        @click="loadPage()"
      >
        {{ text("Reload", "重新加载") }}
      </button>
      <button
        v-if="unreadOnPage.length"
        class="button button-secondary"
        type="button"
        :disabled="loading || writing || !!pending"
        @click="markRead(unreadOnPage.map((item) => item.id))"
      >
        {{ text("Mark this page read", "本页标为已读") }}
      </button>
    </div>

    <p v-if="error && !pending" class="notifications-message notifications-error" role="alert">
      {{ error }}
    </p>
    <p v-if="pending" class="notifications-message notifications-error" role="alert">
      {{ text("The mark-read result is unknown. Reloading does not confirm whether the earlier request succeeded.", "标为已读的结果尚未确认。重新加载无法确认此前请求是否成功。") }}
      <button
        type="button"
        class="notifications-inline-button"
        :disabled="writing || loading"
        @click="retryPending"
      >
        {{ text("Retry the same request", "重试同一请求") }}
      </button>
    </p>
    <p v-if="loading && !items.length" class="notifications-message" role="status">
      {{ text("Loading notifications…", "正在加载通知…") }}
    </p>
    <p v-else-if="loading" class="notifications-message" role="status">
      {{ text("Refreshing notifications…", "正在刷新通知…") }}
    </p>
    <div v-else-if="!items.length && !error" class="notifications-empty">
      {{ text("You have no notifications yet.", "暂无通知。") }}
    </div>

    <div v-if="items.length" class="notification-list">
      <article
        v-for="item in items"
        :key="item.id"
        class="notification-card"
        :class="{ 'read-card': !!item.read_at }"
      >
        <span class="notification-marker" aria-hidden="true"></span>
        <div class="notifications-content">
          <div class="notification-meta">
            {{ presentation(item).createdAt }}
            <span v-if="!item.read_at">{{ text("NEW", "未读") }}</span>
          </div>
          <h3>{{ presentation(item).title }}</h3>
          <p>{{ presentation(item).body }}</p>
          <p v-if="presentation(item).protectedNote" class="notification-protected-note">
            {{ presentation(item).protectedNote }}
          </p>
          <p v-if="presentation(item).reference" class="notification-reference">
            {{ text("Reference", "业务编号") }}: {{ presentation(item).reference }}
          </p>
          <button
            v-if="!item.read_at"
            class="notifications-inline-button"
            type="button"
            :disabled="loading || writing || !!pending"
            @click="markRead([item.id])"
          >
            {{ text("Mark as read", "标为已读") }}
          </button>
        </div>
      </article>
    </div>

    <div v-if="items.length || total !== null" class="notifications-pagination">
      <button
        class="button button-secondary"
        type="button"
        :disabled="!hasPrevious || loading || writing || !!pending"
        @click="changePage(offset - limit)"
      >
        {{ text("Previous", "上一页") }}
      </button>
      <span>{{ text(`Page ${pageNumber}`, `第 ${pageNumber} 页`) }}</span>
      <button
        class="button button-secondary"
        type="button"
        :disabled="!hasNext || loading || writing || !!pending"
        @click="changePage(offset + limit)"
      >
        {{ text("Next", "下一页") }}
      </button>
    </div>
  </section>
</template>

<style scoped>
.notifications-panel {
  min-width: 0;
}
.notifications-heading {
  padding-bottom: 16px;
}
.notifications-heading h1 {
  overflow-wrap: anywhere;
}
.notifications-toolbar,
.notifications-pagination {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 10px;
  padding: 0 17px 12px;
}
.notifications-pagination {
  justify-content: center;
  padding: 14px 17px;
  color: #87938a;
  font-size: 12px;
}
.notifications-message,
.notifications-empty {
  margin: 0;
  padding: 18px;
  color: #87938a;
  font-size: 13px;
  line-height: 1.6;
  overflow-wrap: anywhere;
}
.notifications-error {
  color: #9b4b3f;
}
.notifications-content {
  white-space: pre-wrap;
  min-width: 0;
  overflow-wrap: anywhere;
}
.notification-card .notification-reference {
  margin-top: 2px;
  color: #98a29a;
  font-size: 11px;
  overflow-wrap: anywhere;
}
.notifications-inline-button {
  display: inline-block;
  padding: 4px 0;
  border: 0;
  background: transparent;
  color: var(--brand-primary, #56745e);
  font: inherit;
  font-size: 12px;
  font-weight: 700;
  text-align: left;
  cursor: pointer;
}
.notifications-inline-button:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
.notifications-pagination .button:disabled,
.notifications-toolbar .button:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
@media (max-width: 620px) {
  .notifications-toolbar {
    padding-right: 12px;
    padding-left: 12px;
  }
  .notification-card {
    gap: 9px;
    padding: 16px 2px;
  }
  .notifications-pagination {
    gap: 7px;
    padding-right: 12px;
    padding-left: 12px;
  }
  .notifications-pagination .button {
    min-width: 0;
    padding-right: 10px;
    padding-left: 10px;
  }
}
</style>
