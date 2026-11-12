<script setup lang="ts">
import { readPaginationTotal } from '@lottery/shared'

import TablePagination from "@lottery/shared/TablePagination.vue"
import { vWriteGuard } from "@lottery/shared";
import { withGlobalLoading } from '@lottery/shared';
import SiteIcon from "@lottery/shared/SiteIcon";
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading, formatDisplayDateTime } from "@lottery/shared";
import { useUserI18n } from "./i18n";
import DetailModal from '@lottery/shared/DetailModal.vue';
import ConfiguredBetReviewTable from './ConfiguredBetReviewTable.vue';
import type { ConfiguredLiveOrder } from '@lottery/shared';
import { readNotificationOrder } from './notification-order';

const ui = useUserI18n().t;
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import type { Language } from "../../shared/src/brand";
import {
  createNotificationApi,
  type NotificationItem,
} from "./notification-api";
import { notificationAmount, renderNotification } from "./notification-presentation";

const props = defineProps<{ brandCode?: string; locale: Language }>();
const catalogLocale = computed(() => ({ en: 'en', zh: 'zh-CN' } as const)[props.locale]);
const emit = defineEmits<{
  "unread-count": [count: string | null];
  "auth-expired": [];
}>();

type Context = { brand_id: string; member_id: string };

const items = ref<NotificationItem[]>([]);
const selected = ref<NotificationItem | null>(null);
const selectedPresentation = computed(() => selected.value ? presentation(selected.value) : null);
const selectedAmount = computed(() => selected.value ? notificationAmount(selected.value, props.locale) : null);
const selectedDraw = computed(() => selected.value && 'draw' in selected.value.payload ? selected.value.payload.draw : null);
const betOrder = ref<ConfiguredLiveOrder | null>(null);
const betOrderError = ref<'unavailable' | 'failed' | null>(null);
let orderGeneration = 0;
function isBetNotification(item: NotificationItem) { return item.event_type.startsWith('bet.order.'); }
const confirmedRead = ref(new Set<string>());
const isMobile = ref(false);
let media: MediaQueryList;
const context = ref<Context | null>(null);
const limit = ref(20);
const offset = ref(0);
const total = ref<string | null>(null);
const loading = ref(false);
const writing = ref(false);
const error = ref("");
const scopeGeneration = ref(0);
const listGeneration = ref(0);
const disposed = ref(false);
const api = ref(createNotificationApi({ brandCode: props.brandCode }));
const pageNumber = computed(() => Math.floor(offset.value / limit.value) + 1);
const hasPrevious = computed(() => offset.value > 0);
const hasNext = computed(
  () => total.value !== null && BigInt(offset.value + limit.value) < BigInt(total.value),
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
  context.value = null;
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

async function loadPage(options: { resetError?: boolean; afterWrite?: boolean; offset?: number } = {}) {
  if (writing.value && !options.afterWrite) return;
  const generation = scopeGeneration.value;
  const request = ++listGeneration.value;
  if (options.resetError !== false) error.value = "";
  loading.value = true;
  const requestOffset = options.offset ?? offset.value;
  try {
    const result = await api.value.list(limit.value, requestOffset);
    if (!isCurrent(generation) || request !== listGeneration.value) return;
    const nextContext = { brand_id: result.brand_id, member_id: result.member_id };
    if (context.value && !sameContext(context.value, nextContext)) {
      // An account change invalidates any unresolved write identity.
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
    items.value = result.items;
    offset.value = result.offset;
    total.value = readPaginationTotal(result);
    error.value = "";
  } catch (cause) {
    if (!isCurrent(generation) || request !== listGeneration.value) return;
    const err = cause as { status?: number };
    if (err?.status === 401) handleAuthExpired();
    else error.value = errorMessage(cause);
  } finally {
    if (isCurrent(generation) && request === listGeneration.value) loading.value = false;
  }
}

async function markRead(ids: string[]) {
  if (loading.value || writing.value || ids.length === 0 || !context.value) return;
  const capturedContext = { ...context.value }, generation = scopeGeneration.value;
  listGeneration.value++;
  writing.value = true;
  error.value = "";
  try {
    const result = await api.value.markRead([...ids], capturedContext, crypto.randomUUID());
    if (!isCurrent(generation)) return;
    if (!sameContext(capturedContext, { brand_id: result.brand_id, member_id: result.member_id })) {
      clearAccountState();
      error.value = ui('user.copy.b6aff54e');
      return;
    }
    emit("unread-count", result.unread_count);
    confirmedRead.value = new Set([...confirmedRead.value, ...result.ids]);
    await loadPage({ resetError: false, afterWrite: true });
  } catch (cause) {
    if (!isCurrent(generation)) return;
    if ((cause as { status?: number })?.status === 401) handleAuthExpired();
    else error.value = errorMessage(cause);
  } finally {
    if (isCurrent(generation)) writing.value = false;
  }
}

function openMessage(item: NotificationItem) {
  selected.value = item;
  if (!isRead(item)) markRead([item.id]);
}

async function loadBetOrder() {
  const item = selected.value, capturedContext = context.value, generation = scopeGeneration.value;
  const request = ++orderGeneration;
  betOrder.value = null;
  betOrderError.value = null;
  if (!item || !isBetNotification(item) || !capturedContext) return;
  try {
    const order = await readNotificationOrder(props.brandCode, capturedContext, item.payload.resource_id);
    if (!isCurrent(generation) || request !== orderGeneration || selected.value?.id !== item.id) return;
    betOrder.value = order;
  } catch (cause) {
    if (!isCurrent(generation) || request !== orderGeneration || selected.value?.id !== item.id) return;
    const status = (cause as { status?: number }).status;
    if (status === 401) handleAuthExpired();
    else betOrderError.value = status === 404 ? 'unavailable' : 'failed';
  }
}
watch(selected, item => {
  if (item && isBetNotification(item)) void withGlobalLoading(loadBetOrder);
  else { orderGeneration++; betOrder.value = null; betOrderError.value = null; }
}, { flush: 'sync' });

async function changePage(nextOffset: number) {
  if (writing.value || loading.value) return;
  await loadPage({ offset: Math.max(0, nextOffset) });
}


function changeViewport(event: MediaQueryListEvent) {
  isMobile.value = event.matches;
  if (!writing.value) void loadPage({ offset: 0 });
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

onMounted(() => {
  media = window.matchMedia('(max-width: 620px)');
  isMobile.value = media.matches;
  media.addEventListener('change', changeViewport);
  void loadPage();
});
onBeforeUnmount(() => {
  media?.removeEventListener('change', changeViewport);
  disposed.value = true;
  scopeGeneration.value++;
  listGeneration.value++;
});
</script>

<template>
  <section class="notifications-panel user-record-list" :aria-busy="loading">
    <Teleport defer to="#user-page-refresh"><button v-if="error" v-refresh-button="ui('user.copy.8a197e0a')"
        class="icon-button"
        type="button"
        :aria-label="ui('user.copy.8a197e0a')"
        :title="ui('user.copy.8a197e0a')"
        :disabled="loading || writing"
        @click="withGlobalLoading(loadPage)"
      >
        {{ ui('user.copy.8a197e0a') }}
      </button></Teleport>
    <div v-if="unreadBatch.length" class="notifications-toolbar">
      <button
        v-write-guard="writing"
        v-if="unreadBatch.length"
        class="icon-button"
        type="button"
        :aria-label="batchLabel"
        :title="batchLabel"
        :disabled="loading || writing"
        @click="withGlobalLoading(markRead, unreadBatch.map((item) => item.id))"
      >
        <SiteIcon name="read" />
      </button>
    </div>

    <p v-if="error" class="notifications-message notifications-error" role="alert">
      {{ error }}
    </p>
    <p v-global-loading v-if="loading && !items.length" class="notifications-message" role="status">
      {{ ui('user.copy.c8ac0a56') }}
    </p>
    <p v-global-loading v-else-if="loading" class="notifications-message" role="status">
      {{ ui('user.copy.c36a8e93') }}
    </p>
    <div v-else-if="total !== null && !items.length && !error" class="notifications-empty empty-state">
      {{ ui('user.notifications.empty') }}
    </div>

    <div v-if="items.length" class="notification-list">
      <article
        v-for="item in items"
        :key="item.id"
        class="notification-card"
        :class="{ 'read-card': isRead(item) }"
      >
        <button type="button" class="notification-summary" aria-haspopup="dialog" :disabled="loading || writing" @click="openMessage(item)">
          <span class="notification-status" role="img" :aria-label="isRead(item) ? ui('user.notifications.read') : ui('user.copy.25fe7670')" :title="isRead(item) ? ui('user.notifications.read') : ui('user.copy.25fe7670')"><span class="notification-marker" aria-hidden="true"></span></span>
          <strong class="notification-title">{{ presentation(item).title }}</strong>
          <span v-if="notificationAmount(item, locale)" class="notification-summary-amount">{{ notificationAmount(item, locale)!.value }}</span>
        </button>
      </article>
    </div>


    <TablePagination v-if="total !== null" class="notifications-pagination" :total="total ?? '0'" :page="Math.floor(pageNumber)" :count="items.length" :locale="catalogLocale" :page-size="limit" :previous-disabled="!hasPrevious || loading || writing" :next-disabled="!hasNext || loading || writing" :previous-label="ui('user.copy.d1c7b375')" :next-label="ui('user.copy.2d3bed9a')" :go-to="(target: number) => changePage((target - 1) * limit)" :change-page-size="async (size: number) => { limit = size; await loadPage({ offset: 0 }) }" :busy="loading || writing" />
    <DetailModal v-if="selected && selectedPresentation" class="notification-dialog" :title="selectedPresentation.title" :close-label="ui('user.notifications.close')" @close="selected = null">
      <template #heading-extra><span class="notification-read-state">{{ isRead(selected) ? ui('user.notifications.read') : ui('user.copy.25fe7670') }}</span></template>
      <div class="notification-detail">
        <div v-if="selectedAmount" class="notification-amount" :class="selectedAmount.tone"><span>{{ selectedAmount.label }}</span><strong>{{ selectedAmount.value }}</strong></div>
        <p v-if="selected.event_type === 'member.joined' || selected.template_version > 1" class="notification-body">{{ selectedPresentation.body }}</p>
        <template v-if="betOrder">
          <dl class="notification-bet-summary">
            <div><dt>{{ ui('user.notifications.betGame') }}</dt><dd>{{ betOrder.game_name }}</dd></div>
            <div><dt>{{ ui('user.orders.column.period') }}</dt><dd>{{ betOrder.period_no }}</dd></div>
            <div><dt>{{ ui('user.orders.column.stake') }}</dt><dd>{{ betOrder.quote.bet_points }}</dd></div>
            <div><dt>{{ ui('user.orders.column.time') }}</dt><dd>{{ formatDisplayDateTime(betOrder.placed_at, locale) }}</dd></div>
          </dl>
          <ConfiguredBetReviewTable :snapshot="betOrder.snapshot" :lines="betOrder.quote.lines" :locale="catalogLocale" />
        </template>
        <div v-if="betOrderError" class="notification-bet-error">
          <p class="notifications-error" role="alert">{{ ui(betOrderError === 'unavailable' ? 'user.notifications.betUnavailable' : 'user.notifications.betLoadFailure') }}</p>
          <button v-if="betOrderError === 'failed'" type="button" class="button button-secondary" @click="withGlobalLoading(loadBetOrder)">{{ ui('user.notifications.betRetry') }}</button>
        </div>
        <dl v-if="selectedDraw" class="notification-bet-summary">
          <div><dt>{{ ui('user.orders.column.period') }}</dt><dd>{{ selectedDraw.period_no }}</dd></div>
          <div><dt>{{ ui('user.notification.drawTime') }}</dt><dd>{{ formatDisplayDateTime(selectedDraw.drawn_at, locale) }}</dd></div>
          <div v-if="selectedDraw.result.regular.length"><dt>{{ ui('user.draw.group.regular') }}</dt><dd>{{ selectedDraw.result.regular.join(', ') }}</dd></div>
          <div v-if="selectedDraw.result.special.length"><dt>{{ ui('user.draw.group.special') }}</dt><dd>{{ selectedDraw.result.special.join(', ') }}</dd></div>
          <div v-if="selectedDraw.result.digits.length"><dt>{{ ui('user.draw.group.digits') }}</dt><dd>{{ selectedDraw.result.digits.join('') }}</dd></div>
        </dl>
        <p v-if="error" class="notifications-error" role="alert">{{ error }}</p>
      </div>
      </DetailModal>
  </section>
</template>

<style scoped>
.notifications-panel {
  min-width: 0;
}
.notifications-toolbar:not(:has(> button)) { display: none; }.notifications-toolbar {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 10px;
  padding: 0 17px 12px;
}
.notifications-message {
  margin: 0;
  padding: 18px;
  color: var(--muted);
  font-size: var(--body-font-size);
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
.notifications-inline-button {
  display: inline-block;
  padding: 4px 0;
  border: 0;
  background: transparent;
  color: var(--primary);
  font: inherit;
  font-size: var(--body-font-size);
  font-weight: 700;
  text-align: left;
  cursor: pointer;
  min-height: var(--control-height);
  box-sizing: border-box;
  padding-block: 4px;
  line-height: 22px;
}
.notifications-inline-button:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}.notifications-toolbar .button:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}


.notification-card { display: block; padding: 0; overflow: hidden; }
.notification-summary { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; gap: 14px; align-items: center; width: 100%; min-height: 64px; padding: 16px; border: 0; background: var(--surface); color: var(--ink); text-align: left; font: inherit; cursor: pointer; }
.notification-summary:hover { background: var(--canvas); }
.notification-summary:focus-visible { outline: 2px solid var(--primary); outline-offset: -2px; }
.notification-status { display: inline-flex; align-items: center; gap: 7px; font-size: var(--body-font-size); color: var(--muted); white-space: nowrap; }
.notification-marker { width: 8px; height: 8px; margin: 0; background: var(--danger); box-shadow: none; flex-shrink: 0; }
.read-card .notification-marker { background: var(--success); }
.notification-summary-amount { font-variant-numeric:tabular-nums; overflow-wrap:anywhere; text-align:right; max-width:40vw; }
.notification-title { min-width: 0; font-size: 15px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.notification-detail { display: grid; gap: 16px; min-width: 0; overflow-wrap: anywhere; }
.notification-detail p { margin: 0; }
.notification-bet-summary { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px 16px; margin: 0; font-size: var(--body-font-size); }
.notification-bet-summary > div { display:flex; align-items:baseline; justify-content:space-between; gap:12px; min-width:0; }
.notification-bet-summary dt { color: var(--muted); flex-shrink:0; }
.notification-bet-summary dd { text-align:right; margin: 0; overflow-wrap: anywhere; font-variant-numeric: tabular-nums; }
.notification-bet-error { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
@media (max-width: 560px) { .notification-bet-summary { grid-template-columns: minmax(0, 1fr); } }
.notification-body { white-space: pre-wrap; line-height: 1.7; }

.notification-dialog { width:min(640px,calc(100vw - 24px)); max-width:calc(100vw - 24px); }
.notification-dialog :deep(.detail-modal-heading) { justify-content:flex-start; }
.notification-dialog :deep(.detail-modal-close) { margin-left:auto; }
.notification-read-state { color:var(--muted); font-size:var(--body-font-size); white-space:nowrap; }
.notification-amount { display:flex; align-items:baseline; justify-content:space-between; gap:16px; min-width:0; }
.notification-amount > span { color:var(--muted); flex-shrink:0; }
.notification-amount strong { font-size:28px; font-weight:650; line-height:1.3; font-variant-numeric:tabular-nums; overflow-wrap:anywhere; text-align:right; }
.notification-amount.positive strong { color:var(--success); }
.notification-amount.negative strong { color:var(--danger); }
@media(max-width:900px),(pointer:coarse) { .notification-detail { gap:8px; } }

@media (max-width: 620px) {
  .notifications-toolbar {
    padding-right: 12px;
    padding-left: 12px;
  }
  .notification-card {
    padding: 0;
  }
  .notification-summary { gap: 10px; padding: 14px; }
}
@media (min-width: 901px) and (pointer: fine) {
  .notification-summary { min-height: 48px; padding: 8px 12px; gap: 8px; }
}
</style>
