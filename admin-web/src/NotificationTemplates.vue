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
  createNotificationTemplatesApi, notificationTemplateKeys, notificationTemplatePermissions,
  validNotificationTemplateContent, type NotificationTemplate, type NotificationTemplateContent,
  type NotificationTemplateKey, type NotificationTemplateRevision,
} from "./notification-templates-api";
import {
  classifyNotificationTemplateFailure, clearPendingNotificationTemplateWrite, freezeNotificationTemplateBody,
  getPendingNotificationTemplateWrite, notificationTemplateScopeKey, notificationTemplateSessionGeneration,
  setPendingNotificationTemplateWrite, type PendingNotificationTemplateWrite,
} from "./notification-templates-state";
import { adminKey, useAdminI18n } from "./i18n";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const { t, display } = useAdminI18n();
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
const reviewHeading = ref<HTMLHeadingElement | null>(null);
const pending = ref<PendingNotificationTemplateWrite | null>(null);
const loading = ref(false), historyLoading = ref(false), writing = ref(false);
const error = ref<string | import("@lottery/shared").LocalizedMessage>(""), notice = ref("");
let alive = true, listTicket = 0, historyTicket = 0, writeTicket = 0;

const permissionScope = computed(() => JSON.stringify([
  props.account.id, props.account.super_admin, props.account.brand_ids, props.account.permissions,
  props.account.permissions_by_brand, props.account.platform_permissions, props.brandId, rights.value,
]));
const scope = computed(() => selectedKey.value ? notificationTemplateScopeKey(props.account.id, props.brandId, selectedKey.value) : "");
const contentValid = computed(() => Boolean(draft.value && selectedKey.value && validNotificationTemplateContent(draft.value, selectedKey.value)));
const commissionFacts = computed(() => {
  if (selectedKey.value === "commission.paid") return [
    "Historical record: commission points were credited. Check the current wallet.",
    "历史记录：佣金积分曾入账。请查看当前钱包。",
  ] as const;
  if (selectedKey.value === "commission.adjusted") return [
    "Historical record: a signed commission adjustment was recorded. Check the current wallet.",
    "历史记录：曾记录一笔带正负方向的佣金调整。请查看当前钱包。",
  ] as const;
  if (selectedKey.value === "commission.corrected") return [
    "Historical record: a signed commission correction records a past additional credit or recovery. Check the current wallet. Editing template copy cannot change this state note.",
    "历史记录：带正负方向的佣金更正记录过去的补发或追回。请查看当前钱包。编辑模板文案无法改变此状态说明。",
  ] as const;
  return null;
});
const rewardFacts = computed(() => {
  if (selectedKey.value === "reward.order.granted") return [
    `Historical record: ${SAMPLE_POINTS} points were credited to the gift available balance in the past. Editing template copy cannot change this state note.`,
    `历史记录：${SAMPLE_POINTS} 积分曾记入赠送可用积分。编辑模板文案无法改变此状态说明。`,
  ] as const;
  if (selectedKey.value === "reward.order.revocation_pending") return [
    `Historical record: a full reversal of ${SAMPLE_POINTS} reward points was requested and is awaiting operator handling. No points or money moved in this attempt. It does not retry, unfreeze, or deduct automatically. Editing template copy cannot change this state note.`,
    `历史记录：曾申请全额撤销 ${SAMPLE_POINTS} 积分奖励，正在等待运营处理。本次没有积分或资金变动，不会自动重试、解冻或扣除。编辑模板文案无法改变此状态说明。`,
  ] as const;
  if (selectedKey.value === "reward.order.revoked") return [
    `Historical record: the full original reward of ${SAMPLE_POINTS} points was reversed from the gift available balance. Editing template copy cannot change this state note.`,
    `历史记录：原奖励全额 ${SAMPLE_POINTS} 积分曾从赠送可用积分撤销。编辑模板文案无法改变此状态说明。`,
  ] as const;
  return null;
});
const drawFacts = computed(() => {
  if (selectedKey.value !== "draw.result.published" && selectedKey.value !== "draw.result.corrected") return null;
  return [
    "Historical draw record · Period: 20261009001 · Regular: 03, 12, 28 · Special: 07 · Digits: none · Drawn at: 2026-10-09T12:00:00Z.",
    "历史开奖记录 · 期号：20261009001 · 普通：03、12、28 · 特别：07 · 数字：无 · 开奖时间：2026-10-09T12:00:00Z。",
  ] as const;
});
const canEdit = computed(() => rights.value.view && rights.value.write && !props.account.super_admin && Boolean(record.value) &&
  Boolean(draft.value) && !pending.value && !writing.value && !loading.value);
const canReview = computed(() => canEdit.value && contentValid.value && !historyLoading.value);
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
    error.value = cause instanceof Error ? publicErrorMessage(cause) : "读取通知模板失败。";
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
  confirmed.value = false; error.value = ""; notice.value = "核对这份冻结的中英文内容、当前版本后确认保存。";
  void nextTick(() => {
    reviewHeading.value?.focus({ preventScroll: true });
    reviewHeading.value?.scrollIntoView({ block: "start" });
  });
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
    notice.value = "template-saved";
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
        error.value = cause instanceof Error ? publicErrorMessage(cause) : "写入结果未知。";
      }
    } else {
      clearPendingNotificationTemplateWrite({ accountId: intent.accountId, brandId: intent.brandId, templateKey: intent.templateKey }, intent.key);
      if (currentWrite(ticket, capturedScope, capturedPermission, generation)) {
        pending.value = null;
        notice.value = status === 409 ? "服务器拒绝了此版本；读取最新模板后重新核对并创建新请求。" : "服务器明确拒绝了本次保存。";
        error.value = cause instanceof Error ? publicErrorMessage(cause) : "保存失败。";
        if (status === 409) { receipt.value = null; void readTemplates(); }
      }
    }
  } finally { if (currentWrite(ticket, capturedScope, capturedPermission, generation)) writing.value = false; }
}
async function confirmSave() { if (review.value && confirmed.value && canReview.value) await submit(review.value); }
async function retryUnknown() { if (pending.value && !writing.value) await submit(pending.value); }
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
function time(value: string): string{ return formatDisplayDateTime(value, displayLocale.value); }
function formatKey(key: NotificationTemplateKey): string {
  return t(adminKey(`admin.notification.event.${key}`));
}
function localizedNotice(value: string): string {
  const fixed: Record<string, string> = {
    "template-saved": 'admin.notification.savedRevision',
    "核对这份冻结的中英文内容、当前版本后确认保存。": 'admin.copy.235f78b8',
    "正在提交已核对的模板版本…": 'admin.copy.8979f8d5',
    "写入结果未知。原请求正文和幂等键已保留；只读刷新不会确认或清除此请求。请显式按原请求重试。": 'admin.copy.8a2b03fd',
    "服务器拒绝了此版本；读取最新模板后重新核对并创建新请求。": 'admin.copy.f86d6ac2',
    "服务器明确拒绝了本次保存。": 'admin.copy.2460ac1d',
  };
  const pair = fixed[value];
  if (pair) return t(adminKey(pair));
  return value;
}

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
      <div><h2 id="nt-title">{{ t('admin.copy.e4f1dc7c') }}</h2></div>
      <button v-refresh-button="t('admin.copy.f3fcdf0d')" v-if="rights.view" class="nt-button" type="button" :aria-label="t('admin.copy.f3fcdf0d')" :disabled="loading || writing" @click="withGlobalLoading(readTemplates)">{{ loading ? t('admin.copy.27342bf3') : t('admin.copy.755b67f2') }}</button>
    </header>

    <p v-if="!rights.view" class="nt-message">{{ t('admin.copy.1363a863') }}</p>
    <template v-else>
      <p v-if="error" class="nt-message nt-error" role="alert">{{ displayPublicError(error) }}</p>
      <p v-if="notice" class="nt-message" role="status">{{ localizedNotice(notice) }}</p>
      <div class="nt-layout">
        <nav class="nt-keys" :aria-label="t('admin.copy.4ba7cff7')">
          <label for="nt-key">{{ t('admin.copy.e6928e40') }}</label>
          <select id="nt-key" :value="selectedKey" :disabled="writing" @change="selectKey(($event.target as HTMLSelectElement).value)">
            <option value="" disabled>{{ t('admin.copy.c1ee6b40') }}</option>
            <option v-for="key in notificationTemplateKeys" :key="key" :value="key">{{ formatKey(key) }}</option>
          </select>
        </nav>

        <div v-if="(record && draft && selectedKey) || pending" class="nt-workspace">
          <section v-if="record && draft" class="nt-card" aria-labelledby="nt-editor-title">
            <div class="nt-card-heading"><div><h3 id="nt-editor-title">{{ t('admin.copy.c99e7d96') }}</h3><p> {{ t('admin.copy.c6f1ca9e') }} {{ time(record.updated_at) }}</p></div>

            </div>
            <div class="nt-languages">
              <fieldset v-for="locale in (['en', 'zh-CN'] as const)" :key="locale" class="nt-locale" :disabled="!canEdit">
                <legend>{{ locale === "en" ? t('admin.copy.61f7e041') : t('admin.copy.07a5470d') }}</legend>
                <label :for="`nt-title-${locale}`">{{ t('admin.copy.c4ce8854') }}</label>
                <input :id="`nt-title-${locale}`" v-model="draft[locale].title" maxlength="120" autocomplete="off" />
                <label :for="`nt-body-${locale}`">{{ t('admin.copy.6811dcad') }}</label>
                <textarea :id="`nt-body-${locale}`" v-model="draft[locale].body" rows="5" maxlength="1200" />
                <small>{{ selectedKey.startsWith('draw.result.') ? t('admin.copy.4622fdc7') : selectedKey === 'member.joined' ? t('admin.copy.30936cb1') : t('admin.copy.59670147') }} {{ t('admin.copy.d697ab14') }}</small>
                <div class="nt-preview" :aria-label="t('admin.copy.3eece8be')"><strong>{{ t('admin.copy.8326c0e9') }}</strong>
                  <p>{{ interpolate(draft[locale].title) }}</p><div>{{ interpolate(draft[locale].body) }}</div>
                  <small>{{ t('admin.copy.9a4b4624') }} <template v-if="!selectedKey.startsWith('draw.result.')">points={{ SAMPLE_POINTS }} · </template>resource_id={{ SAMPLE_RESOURCE_ID }}</small>
                </div>
              </fieldset>
            </div>
            <div v-if="drawFacts" class="nt-facts-note" data-testid="draw-facts-note"><strong>{{ t('admin.copy.b4f787ee') }}</strong>
              <p v-for="fact in drawFacts" :key="fact">{{ fact }}</p></div>
            <div v-else-if="commissionFacts" class="nt-facts-note" data-testid="commission-facts-note"><strong>{{ t('admin.copy.a80ceded') }}</strong>
              <p v-for="fact in commissionFacts" :key="fact">{{ fact }}</p></div>
            <div v-else-if="rewardFacts" class="nt-facts-note" data-testid="reward-facts-note"><strong>{{ t('admin.copy.e88286c1') }}</strong>
              <p v-for="fact in rewardFacts" :key="fact">{{ fact }}</p></div>
            <div v-else class="nt-facts-note"><strong>{{ t('admin.copy.c2395ad2') }}</strong><p>{{ t('admin.copy.1030b53c') }}</p></div>
            <p v-if="draft && !contentValid" class="nt-message nt-error" role="alert">{{ t('admin.copy.13f37d8a') }}</p>
            <div v-if="rights.write && !props.account.super_admin" class="nt-reason">
              <button class="nt-button nt-primary" type="button" :disabled="!canReview" @click="startReview">{{ t('admin.copy.54bf243a') }}</button>
            </div>
            <p v-else class="nt-readonly">{{ t('admin.copy.239326de') }}</p>
          </section>

          <aside v-if="pending" class="nt-pending" aria-labelledby="nt-pending-title">
            <h3 id="nt-pending-title">{{ writing ? t('admin.copy.5dd1c388') : t('admin.copy.1c9b6685') }}</h3>
            <p>{{ t('admin.copy.c6b19626') }}</p>
            <dl class="nt-audit"><div><dt>{{ t('admin.copy.f6964fee') }}</dt><dd>{{ formatKey(pending.templateKey) }} · <AssociationName kind="brand" :id="pending.brandId" :locale="associationLocale" /></dd></div>

              </dl>
            <button v-write-guard="writing" v-if="rights.write && !props.account.super_admin" class="nt-button nt-primary" type="button" :disabled="writing" @click="withGlobalLoading(retryUnknown)">{{ writing ? t('admin.copy.20e088a1') : t('admin.copy.db0c4d5a') }}</button>
            <p v-else>{{ t('admin.copy.7d5ffcee') }}</p>
          </aside>

          <section v-if="review && !pending" class="nt-confirm" aria-labelledby="nt-confirm-title">
            <h3 id="nt-confirm-title" ref="reviewHeading" tabindex="-1">{{ t('admin.copy.22341a5c') }}</h3>
            <p>{{ t('admin.versionDisplay.saveTemplate') }}</p>
            <div class="nt-review-locale" v-for="locale in (['en', 'zh-CN'] as const)" :key="locale">
              <strong>{{ locale === "en" ? t('admin.copy.61f7e041') : t('admin.copy.07a5470d') }}</strong>
              <p>{{ review.body.content[locale].title }}</p><div>{{ review.body.content[locale].body }}</div>
            </div>
            <p v-if="review.body.reason" class="nt-break">{{ t('admin.copy.2809a1e6') }}{{ review.body.reason }}</p>

            <label class="nt-check"><input v-model="confirmed" type="checkbox" />{{ t('admin.copy.760eac81') }}</label>
            <div class="nt-actions"><button class="nt-button" type="button" :disabled="writing" @click="clearReview">{{ t('admin.copy.bf7400b6') }}</button>
              <button v-write-guard="writing" class="nt-button nt-primary" type="button" :disabled="!confirmed || writing || !rights.write" @click="withGlobalLoading(confirmSave)">{{ writing ? t('admin.copy.2462134b') : t('admin.copy.98ec8b57') }}</button></div>
          </section>

          <section class="nt-card nt-history" aria-labelledby="nt-history-title">
            <div class="nt-card-heading"><div><h3 id="nt-history-title">{{ t('admin.copy.e28a7842') }}</h3><p>{{ t('admin.copy.5cb7f04d') }} {{ PAGE_SIZE }} {{ t('admin.copy.0eac00c9') }}</p></div>
              <button v-refresh-button="historyLoading ? t('admin.copy.27342bf3') : t('admin.copy.4f5df8c1')" class="nt-button" type="button" :disabled="historyLoading || writing" @click="withGlobalLoading(readHistory)">{{ historyLoading ? t('admin.copy.27342bf3') : t('admin.copy.4f5df8c1') }}</button></div>
            <ol class="nt-revisions"><li v-for="revision in history" :key="revision.id">
              <div class="nt-card-heading"><time>{{ time(revision.created_at) }}</time></div>
              <p v-if="revision.reason">{{ t('admin.copy.2809a1e6') }}{{ revision.reason }}</p>
              <details><summary>{{ t('admin.copy.7e83342d') }}</summary><div v-for="locale in (['en', 'zh-CN'] as const)" :key="locale" class="nt-review-locale">
                <strong>{{ locale }}</strong><p>{{ revision.content[locale].title }}</p><div>{{ revision.content[locale].body }}</div></div></details>
            </li><li v-if="!history.length && !historyLoading" class="nt-empty empty-state">{{ t('admin.copy.f197163b') }}</li>
              <li v-global-loading v-if="historyLoading && !history.length" class="nt-empty">{{ t('admin.copy.bb2c0dea') }}</li></ol>
            <footer class="nt-pagination"><button class="nt-button" type="button" :disabled="historyOffset === 0 || historyLoading || writing" @click="withGlobalLoading(readHistory, Math.max(0, historyOffset - PAGE_SIZE))">{{ t('admin.copy.dbe7a5d7') }}</button>
              <span>{{ t('admin.copy.4ffb01e2') }} {{ historyOffset }} · {{ history.length }} {{ t('admin.copy.44d96226') }}</span>
              <button class="nt-button" type="button" :disabled="!historyHasNext || historyLoading || writing" @click="withGlobalLoading(readHistory, historyOffset + PAGE_SIZE)">{{ t('admin.copy.2e9c9e7a') }}</button></footer>
          </section>
        </div>
        <p v-global-loading="loading" v-else class="nt-message empty-state">{{ loading ? t('admin.copy.2a4d8a82') : t('admin.copy.49e11e41') }}</p>
      </div>
    </template>
  </section>
</template>

<style scoped>
#nt-confirm-title{scroll-margin-block-start:24px}.nt-page{display:grid;gap:16px;min-width:0;color:var(--ink)}.nt-heading,.nt-card-heading,.nt-pagination,.nt-actions{display:flex;align-items:center;justify-content:space-between;gap:12px}.nt-heading{align-items:flex-start}.nt-heading h2{margin:2px 0 0;font-size:1.25rem}.nt-eyebrow{margin:0;color:#68758a;font-size:.78rem}.nt-layout{display:grid;grid-template-columns:minmax(190px,240px) minmax(0,1fr);align-items:start;gap:16px}.nt-keys,.nt-card,.nt-confirm,.nt-pending,.nt-message{border:1px solid var(--line);border-radius:12px;padding:16px;background:var(--surface)}.nt-keys{display:grid;gap:9px;position:sticky;top:12px}.nt-keys label,.nt-locale label,.nt-reason label{font-weight:600;font-size:.9rem}.nt-keys select,.nt-locale input,.nt-locale textarea,.nt-reason textarea{box-sizing:border-box;width:100%;border:1px solid #c5ceda;border-radius:8px;padding:9px 10px;font:inherit;background:var(--surface)}.nt-workspace{display:grid;gap:16px;min-width:0}.nt-card h3,.nt-confirm h3,.nt-pending h3{margin:0}.nt-card-heading p,.nt-card-heading h3{margin:0}.nt-card-heading p,.nt-hint,.nt-locale small,.nt-reason small,.nt-readonly{color:var(--muted);font-size:.86rem}.nt-hint{margin:0;line-height:1.45}.nt-audit{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px 18px;margin:14px 0}.nt-audit>div{min-width:0;padding:7px 0;border-bottom:1px solid #eef1f5}.nt-audit dt{color:#697586;font-size:.78rem}.nt-audit dd{margin:4px 0 0;overflow-wrap:anywhere;word-break:break-word;font-size:.88rem}.nt-languages{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.nt-locale{display:grid;align-content:start;gap:9px;min-width:0;border:1px solid var(--line);border-radius:10px;padding:12px}.nt-locale legend{padding:0 5px;font-weight:700}.nt-locale textarea,.nt-reason textarea{resize:vertical}.nt-locale small{line-height:1.4}.nt-preview,.nt-review-locale{display:grid;gap:5px;padding:10px;border-radius:8px;background:#f5f7fa;overflow-wrap:anywhere;white-space:pre-wrap}.nt-preview p,.nt-review-locale p,.nt-review-locale div{margin:0;white-space:pre-wrap;overflow-wrap:anywhere}.nt-preview>strong,.nt-preview small{font-size:.78rem;color:#536176}.nt-facts-note{margin-top:14px;padding:11px 12px;border-left:3px solid #8b9bb3;background:#f7f8fa}.nt-facts-note p{margin:4px 0 0;color:#536176;font-size:.88rem}.nt-reason{display:grid;gap:8px;margin-top:16px;max-width:760px}.nt-button{border:1px solid #c5ceda;border-radius:8px;padding:8px 12px;font:inherit;cursor:pointer;background:var(--surface)}.nt-button.nt-primary{background:var(--primary);border-color:var(--primary);color:var(--surface)}.nt-button:disabled{opacity:.52;cursor:not-allowed}.nt-reason>.nt-button{justify-self:start}.nt-message{margin:0;color:#344054}.nt-error{background:#fff0ee;border-color:#df9385;color:#a32e1a}.nt-pending{display:grid;gap:8px;background:#fff1ee;border-color:#df9385}.nt-pending p,.nt-confirm>p{margin:0;overflow-wrap:anywhere}.nt-confirm{display:grid;gap:12px}.nt-review-locale{border:1px solid #e0e5ec}.nt-check{display:flex;align-items:flex-start;gap:9px}.nt-check input{margin-top:.25em}.nt-actions{justify-content:flex-end}.nt-history{display:grid;gap:12px}.nt-revisions{display:grid;gap:10px;margin:0;padding:0;list-style:none}.nt-revisions>li{display:grid;gap:8px;padding:12px;border:1px solid #e1e6ed;border-radius:9px;min-width:0}.nt-revisions>li>p{margin:0;font-size:.9rem;overflow-wrap:anywhere}.nt-revisions time{color:var(--muted);font-size:.82rem}.nt-revisions details summary{cursor:pointer}.nt-revisions details{display:grid;gap:8px}.nt-pagination{justify-content:center;flex-wrap:wrap}.nt-break{overflow-wrap:anywhere;word-break:break-word}.nt-receipt{border-radius:99px;padding:5px 9px;background:#e6f6eb;color:#176b37;font-size:.82rem;white-space:nowrap}
@media(max-width:850px){.nt-layout{grid-template-columns:minmax(0,1fr)}.nt-keys{position:static}.nt-languages{grid-template-columns:minmax(0,1fr)}}
@media(max-width:560px){.nt-card,.nt-confirm,.nt-pending,.nt-keys,.nt-message{padding:12px}.nt-heading{align-items:stretch;flex-direction:column}.nt-heading .nt-button{align-self:flex-start}.nt-card-heading{align-items:flex-start;flex-wrap:wrap}.nt-audit{grid-template-columns:minmax(0,1fr);gap:2px}.nt-actions{align-items:stretch;flex-direction:column-reverse}.nt-actions .nt-button{width:100%}.nt-pagination .nt-button{flex:1}}
</style>
