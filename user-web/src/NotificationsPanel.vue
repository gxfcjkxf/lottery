<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import { withGlobalLoading } from '@lottery/shared';
import SiteIcon from "@lottery/shared/SiteIcon";
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading, formatDisplayDateTime } from "@lottery/shared";
import { useUserI18n } from "./i18n";
import DetailModal from '@lottery/shared/DetailModal.vue';

const ui = useUserI18n().t;
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
const selected = ref<NotificationItem | null>(null);
const confirmedRead = ref(new Set<string>());
const isMobile = ref(false);
const loadSentinel = ref<HTMLElement>();
const loadMoreError = ref(false);
const loadedMore = ref(false);
let observer: IntersectionObserver;
let media: MediaQueryList;
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
const pageNumber = computed(() => Math.floor(offset.value / limit) + 1);
const hasPrevious = computed(() => offset.value > 0);
const hasNext = computed(
  () => total.value !== null && offset.value + limit < total.value,
);
function isRead(item: NotificationItem) { return !!item.read_at || confirmedRead.value.has(item.id); }
const unreadOnPage = computed(() => items.value.filter((item) => !isRead(item)));
const unreadBatch = computed(() => unreadOnPage.value.slice(0, 100));
const batchLabel = computed(() => isMobile.value
  ? ui('user.notifications.markLoaded', { count: unreadBatch.value.length }) : ui('user.copy.ba71505e'));


function isCurrent(generation: number): boolean {
  return !disposed.value && generation === scopeGeneration.value;
}

function sameContext(a: Context | null, b: Context): boolean {
  return a?.brand_id === b.brand_id && a.member_id === b.member_id;
}

function clearAccountState() {
  items.value = [];
  selected.value = null;
  confirmedRead.value = new Set();
  loadMoreError.value = false;
  loadedMore.value = false;
  context.value = null;
  pending.value = null;
  offset.value = 0;
  total.value = null;
  emit("unread-count", null);
}

function handleAuthExpired() {
  clearAccountState();
  error.value = ui('user.copy.6480027f');
  emit("auth-expired");
}

function errorMessage(cause: unknown): string {
  const err = cause as { status?: number; message?: string };
  if (err?.status === 401) return ui('user.copy.6480027f');
  if (err?.status === 0 || (err?.status !== undefined && err.status >= 500)) {
    return ui('user.copy.a9882e2b');
  }
  return ui('user.copy.9326acb2');
}

function presentation(item: NotificationItem) {
  try {
    return renderNotification(item, props.locale);
  } catch {
    return {
      title: ui('user.copy.b811f4e5'),
      body: ui('user.copy.51e43bf7'),
      createdAt: formatDisplayDateTime(item.created_at, props.locale),
      points: null,
      reference: null,
      protectedNote: null,
    };
  }
}

async function loadPage(options: { resetError?: boolean; afterWrite?: boolean; append?: boolean; offset?: number } = {}) {
  if (writing.value && !options.afterWrite) return;
  const generation = scopeGeneration.value;
  const request = ++listGeneration.value;
  if (options.resetError !== false && !pending.value) error.value = "";
  loading.value = true;
  loadMoreError.value = false;
  const requestOffset = options.offset ?? (options.append ? offset.value + limit : isMobile.value ? 0 : offset.value);
  try {
    const result = await api.value.list(limit, requestOffset);
    if (!isCurrent(generation) || request !== listGeneration.value) return;
    const nextContext = { brand_id: result.brand_id, member_id: result.member_id };
    if (context.value && !sameContext(context.value, nextContext)) {
      // An account change invalidates any unresolved write identity.
      pending.value = null;
      items.value = [];
      selected.value = null;
      confirmedRead.value = new Set();
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
    if (options.append && requestOffset > 0) {
      const existing = new Set(items.value.map(item => item.id));
      items.value = [...items.value, ...result.items.filter(item => !existing.has(item.id))];
    } else items.value = result.items;
    offset.value = result.offset;
    loadedMore.value = !!options.append && requestOffset > 0;
    total.value = result.offset + result.items.length + (result.items.length === limit ? 1 : 0);
    if (!pending.value) error.value = "";
  } catch (cause) {
    if (!isCurrent(generation) || request !== listGeneration.value) return;
    const err = cause as { status?: number };
    if (err?.status === 401) handleAuthExpired();
    else { error.value = errorMessage(cause); loadMoreError.value = !!options.append; }
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
      error.value = ui('user.copy.b6aff54e');
      return;
    }
    pending.value = null;
    emit("unread-count", result.unread_count);
    // A validated receipt confirms these IDs are read without inventing a timestamp.
    // Preserve the accumulated mobile list and scroll position.
    confirmedRead.value = new Set([...confirmedRead.value, ...result.ids]);
    if (!isMobile.value) await loadPage({ resetError: false, afterWrite: true });
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
        error.value = ui('user.copy.45032eb2');
      } else {
        pending.value = intent;
        error.value = errorMessage(cause);
      }
    }
  } finally {
    if (isCurrent(generation)) writing.value = false;
  }
}

async function markRead(ids: string[]) {
  if (loading.value || writing.value || pending.value || ids.length === 0 || !context.value) return;
  const intent: PendingIntent = {
    ids: [...ids],
    context: { ...context.value },
    idempotencyKey: createKey(),
  };
  await submitIntent(intent);
}

function openMessage(item: NotificationItem) {
  selected.value = item;
  if (!isRead(item)) markRead([item.id]);
}

async function retryPending() {
  if (pending.value && !writing.value && !loading.value) await submitIntent(pending.value);
}

async function changePage(nextOffset: number) {
  if (pending.value || writing.value || loading.value) return;
  await loadPage({ offset: Math.max(0, nextOffset) });
}

async function loadMore(retry = false) {
  if (!isMobile.value || !hasNext.value || loading.value || writing.value || pending.value || (error.value && !retry)) return;
  await loadPage({ append: true });
}
function changeViewport(event: MediaQueryListEvent) {
  isMobile.value = event.matches;
  if (!pending.value && !writing.value) void loadPage({ offset: 0 });
}
watch([loadSentinel, isMobile, hasNext, loading, writing, pending, error], () => {
  observer?.disconnect();
  if (isMobile.value && loadSentinel.value && hasNext.value && !loading.value && !writing.value && !pending.value && !error.value) observer?.observe(loadSentinel.value);
}, { flush: 'post' });

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

onMounted(() => {
  media = window.matchMedia('(max-width: 620px)');
  isMobile.value = media.matches;
  media.addEventListener('change', changeViewport);
  observer = new IntersectionObserver(entries => { if (entries.some(entry => entry.isIntersecting)) loadMore(); }, { rootMargin: '160px 0px' });
  void loadPage();
});
onBeforeUnmount(() => {
  observer?.disconnect();
  media?.removeEventListener('change', changeViewport);
  emit("unread-count", null);
  disposed.value = true;
  scopeGeneration.value++;
  listGeneration.value++;
});
</script>

<template>
  <section class="notifications-panel">
    <div class="notifications-toolbar">
      <Teleport defer to="#user-page-refresh"><button v-refresh-button="ui('user.copy.8a197e0a')"
        class="button button-secondary notification-tool"
        type="button"
        :aria-label="ui('user.copy.8a197e0a')"
        :title="ui('user.copy.8a197e0a')"
        :disabled="loading || writing"
        @click="withGlobalLoading(loadPage)"
      >
        {{ ui('user.copy.8a197e0a') }}
      </button></Teleport>
      <button
        v-write-guard="writing"
        v-if="unreadBatch.length"
        class="button button-secondary notification-tool"
        type="button"
        :aria-label="batchLabel"
        :title="batchLabel"
        :disabled="loading || writing || !!pending"
        @click="withGlobalLoading(markRead, unreadBatch.map((item) => item.id))"
      >
        <SiteIcon name="read" />
      </button>
    </div>

    <p v-if="error && !pending && !loadMoreError" class="notifications-message notifications-error" role="alert">
      {{ error }}
    </p>
    <p v-if="pending" class="notifications-message notifications-error" role="alert">
      {{ ui('user.copy.39040996') }}
      <button
        v-write-guard="writing"
        type="button"
        class="notifications-inline-button"
        :disabled="writing || loading"
        @click="withGlobalLoading(retryPending)"
      >
        {{ ui('user.copy.4da8cfa4') }}
      </button>
    </p>
    <p v-global-loading v-if="loading && !items.length" class="notifications-message" role="status">
      {{ ui('user.copy.c8ac0a56') }}
    </p>
    <p v-global-loading v-else-if="loading && !isMobile" class="notifications-message" role="status">
      {{ ui('user.copy.c36a8e93') }}
    </p>
    <div v-else-if="!items.length && !error" class="notifications-empty empty-state">
      {{ ui('user.notifications.empty') }}
    </div>

    <div v-if="items.length" class="notification-list">
      <article
        v-for="item in items"
        :key="item.id"
        class="notification-card"
        :class="{ 'read-card': isRead(item) }"
      >
        <button type="button" class="notification-summary" aria-haspopup="dialog" :disabled="loading || writing || !!pending" @click="openMessage(item)">
          <span class="notification-status" role="img" :aria-label="isRead(item) ? ui('user.notifications.read') : ui('user.copy.25fe7670')" :title="isRead(item) ? ui('user.notifications.read') : ui('user.copy.25fe7670')"><span class="notification-marker" aria-hidden="true"></span></span>
          <strong class="notification-title">{{ presentation(item).title }}</strong>
          <time :datetime="item.created_at">{{ presentation(item).createdAt }}</time>
        </button>
      </article>
    </div>

    <div v-if="isMobile && items.length && (hasNext || loadedMore || loadMoreError || loading)" ref="loadSentinel" class="notifications-load-more">
      <template v-if="loadMoreError">
        <p class="notifications-error" role="alert">{{ error }}</p>
        <button type="button" class="button button-secondary" :disabled="loading || writing || !!pending" @click="withGlobalLoading(loadMore, true)">{{ ui('user.notifications.retryMore') }}</button>
      </template>
      <span v-global-loading v-else-if="loading" role="status">{{ ui('user.notifications.loadingMore') }}</span>
      <span v-else-if="loadedMore && !hasNext">{{ ui('user.notifications.noMore') }}</span>
    </div>

    <div v-if="!isMobile && (items.length || total !== null)" class="notifications-pagination">
      <button
        class="button button-secondary"
        type="button"
        :disabled="!hasPrevious || loading || writing || !!pending"
        @click="withGlobalLoading(changePage, offset - limit)"
      >
        {{ ui('user.copy.d1c7b375') }}
      </button>
      <span>{{ ui("user.pagination.page", { page: pageNumber }) }}</span>
      <button
        class="button button-secondary"
        type="button"
        :disabled="!hasNext || loading || writing || !!pending"
        @click="withGlobalLoading(changePage, offset + limit)"
      >
        {{ ui('user.copy.2d3bed9a') }}
      </button>
    </div>
    <DetailModal v-if="selected" :title="presentation(selected).title" :close-label="ui('user.notifications.close')" @close="selected = null">
      <div class="notification-detail">
        <div class="notification-meta"><span>{{ isRead(selected) ? ui('user.notifications.read') : ui('user.copy.25fe7670') }}</span><time :datetime="selected.created_at">{{ presentation(selected).createdAt }}</time></div>
        <p class="notification-body">{{ presentation(selected).body }}</p>
        <p v-if="presentation(selected).protectedNote && presentation(selected).protectedNote !== presentation(selected).body">{{ presentation(selected).protectedNote }}</p>
        <p v-if="presentation(selected).reference" class="notification-reference">{{ ui('user.copy.c7f9efbc') }}: {{ presentation(selected).reference }}</p>
        <p v-if="error && !loadMoreError" class="notifications-error" role="alert">{{ error }}</p>
        <button v-write-guard="writing" v-if="pending" type="button" class="button button-secondary" :disabled="writing || loading" @click="withGlobalLoading(retryPending)">{{ ui('user.copy.4da8cfa4') }}</button>
      </div>
      <template #footer><button type="button" class="button button-primary" @click="selected = null">{{ ui('user.notifications.closeAction') }}</button></template>
    </DetailModal>
  </section>
</template>

<style scoped>
.notifications-panel {
  min-width: 0;
}
.notifications-toolbar:not(:has(> button)) { display: none; }
.notifications-toolbar,
.notifications-pagination {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 10px;
  padding: 0 17px 12px;
}
.notifications-pagination {
  justify-content: center;
  padding: 14px 17px;
  color: var(--muted);
  font-size: 12px;
}
.notifications-message {
  margin: 0;
  padding: 18px;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.6;
  overflow-wrap: anywhere;
}
.notifications-error {
  color: var(--danger);
}
.notifications-content {
  white-space: pre-wrap;
  min-width: 0;
  overflow-wrap: anywhere;
}
.notification-reference {
  margin-top: 2px;
  color: var(--muted);
  font-size: 11px;
  overflow-wrap: anywhere;
}
.notifications-inline-button {
  display: inline-block;
  padding: 4px 0;
  border: 0;
  background: transparent;
  color: var(--primary);
  font: inherit;
  font-size: 12px;
  font-weight: 700;
  text-align: left;
  cursor: pointer;
  min-height: 44px;
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
.notification-tool { width: 44px; height: 44px; min-width: 44px; padding: 10px; }

.notification-card { display: block; padding: 0; overflow: hidden; }
.notification-summary { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; gap: 14px; align-items: center; width: 100%; min-height: 64px; padding: 16px; border: 0; background: var(--surface); color: var(--ink); text-align: left; font: inherit; cursor: pointer; }
.notification-summary:hover { background: var(--canvas); }
.notification-summary:focus-visible { outline: 2px solid var(--primary); outline-offset: -2px; }
.notification-status { display: inline-flex; align-items: center; gap: 7px; font-size: 12px; color: var(--muted); white-space: nowrap; }
.notification-marker { width: 8px; height: 8px; margin: 0; background: var(--danger); box-shadow: none; flex-shrink: 0; }
.read-card .notification-marker { background: var(--success); }
.notification-title { min-width: 0; font-size: 15px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.notification-summary time { font-size: 12px; color: var(--muted); white-space: nowrap; }
.notification-detail { display: grid; gap: 16px; min-width: 0; overflow-wrap: anywhere; }
.notification-detail p { margin: 0; }
.notification-body { white-space: pre-wrap; line-height: 1.7; }
.notification-detail .notification-meta { display: flex; flex-wrap: wrap; gap: 12px; color: var(--muted); font-size: 12px; }
.notifications-load-more { min-height: 44px; padding: 16px; text-align: center; color: var(--muted); font-size: 13px; }
@media (max-width: 620px) {
  .notifications-toolbar {
    padding-right: 12px;
    padding-left: 12px;
  }
  .notification-card {
    padding: 0;
  }
  .notification-summary { gap: 10px; padding: 14px; }
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
