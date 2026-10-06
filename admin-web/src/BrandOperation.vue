<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { useAdminI18n } from "./i18n";
import type { LocalizedMessage } from "@lottery/shared";
import {
  brandOperationPermissions,
  createBrandOperationApi,
  type BrandOperation,
  type BrandOperationHistoryItem,
  type BrandOperationTargetStatus,
} from "./brand-operation-api";
import {
  brandOperationSessionGeneration,
  classifyBrandOperationFailure,
  clearPendingBrandOperationWrite,
  createBrandOperationRequestGuard,
  getPendingBrandOperationWrite,
  setPendingBrandOperationWrite,
  updatePendingBrandOperationPhase,
  type PendingBrandOperationWrite,
} from "./brand-operation-state";

const props = defineProps<{ account: AdminAccount; brandId: string; brandStatus?: string }>();
const emit = defineEmits<{
  (event: "session-invalid"): void;
  (event: "changed", value: { accountId: string; brandId: string; status: BrandOperationTargetStatus; version: number }): void;
}>();

const api = createBrandOperationApi();
const { t, message } = useAdminI18n();
const recordGuard = createBrandOperationRequestGuard();
const historyGuard = createBrandOperationRequestGuard();
const writeGuard = createBrandOperationRequestGuard();
const permissions = computed(() => brandOperationPermissions(props.account, props.brandId));
const record = ref<BrandOperation | null>(null);
const rows = ref<BrandOperationHistoryItem[]>([]);
const offset = ref(0);
const hasMore = ref(false);
const targetStatus = ref<BrandOperationTargetStatus>("paused");
const reason = ref("");
const pending = ref<PendingBrandOperationWrite | null>(null);
const busy = ref(false);
const loading = ref(false);
const historyLoading = ref(false);
const error = ref<string | LocalizedMessage>("");
const notice = ref<string | LocalizedMessage>("");
const conflictReloaded = ref(false);

const readonlyBrand = computed(() => props.brandStatus === "disabled" || record.value?.status === "disabled");
const unknownWrite = computed(() => pending.value?.phase === "unknown");
const conflictedWrite = computed(() => pending.value?.phase === "conflict");
const reviewing = computed(() => pending.value?.phase === "review");
const canWrite = computed(() => permissions.value.write && !readonlyBrand.value && !unknownWrite.value && !conflictedWrite.value);
const reasonBytes = computed(() => new TextEncoder().encode(reason.value).length);
const statusText = (value: string) => value === "active" ? t("运行中", "Active") : value === "paused" ? t("已暂停", "Paused") : value === "disabled" ? t("已停用", "Disabled") : value;
const statusStyle = (value: string) => value === "active" ? "is-active" : value === "paused" ? "is-paused" : "is-disabled";
const timeText = (value: string) => {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
};
const syncPending = () => {
  pending.value = getPendingBrandOperationWrite({ accountId: props.account.id, brandId: props.brandId });
  conflictReloaded.value = false;
};

async function reloadCurrent(): Promise<boolean> {
  const accountId = props.account.id;
  const brandId = props.brandId;
  const epoch = brandOperationSessionGeneration();
  const token = recordGuard.begin();
  loading.value = true;
  error.value = "";
  try {
    const next = await api.get(brandId);
    if (!recordGuard.isCurrent(token) || epoch !== brandOperationSessionGeneration() || props.account.id !== accountId || props.brandId !== brandId) return false;
    record.value = next;
    if (pending.value?.phase === "conflict") conflictReloaded.value = true;
    return true;
  } catch (cause) {
    if (recordGuard.isCurrent(token) && epoch === brandOperationSessionGeneration() && props.account.id === accountId && props.brandId === brandId) {
      error.value = errorText(cause, message("读取品牌状态失败。", "Failed to load brand status."));
      handleSessionError(cause, accountId, brandId, epoch, recordGuard, token);
    }
    return false;
  } finally {
    if (recordGuard.isCurrent(token) && epoch === brandOperationSessionGeneration()) loading.value = false;
  }
}

async function reloadHistory(nextOffset = offset.value): Promise<void> {
  const accountId = props.account.id;
  const brandId = props.brandId;
  const epoch = brandOperationSessionGeneration();
  const token = historyGuard.begin();
  historyLoading.value = true;
  try {
    const page = await api.history(brandId, 20, nextOffset);
    if (!historyGuard.isCurrent(token) || epoch !== brandOperationSessionGeneration() || props.account.id !== accountId || props.brandId !== brandId) return;
    rows.value = page.items;
    offset.value = page.offset;
    hasMore.value = page.items.length === page.limit;
  } catch (cause) {
    if (historyGuard.isCurrent(token) && epoch === brandOperationSessionGeneration() && props.account.id === accountId && props.brandId === brandId) {
      error.value = errorText(cause, message("读取操作记录失败。", "Failed to load operation history."));
      handleSessionError(cause, accountId, brandId, epoch, historyGuard, token);
    }
  } finally {
    if (historyGuard.isCurrent(token) && epoch === brandOperationSessionGeneration()) historyLoading.value = false;
  }
}

function errorText(cause: unknown, fallback: LocalizedMessage): string | LocalizedMessage {
  return cause instanceof Error ? cause.message : fallback;
}

function handleSessionError(cause: unknown, accountId: string, brandId: string, epoch: number, requestGuard: ReturnType<typeof createBrandOperationRequestGuard>, token: number): void {
  if (cause instanceof AdminApiError && cause.status === 401 && requestGuard.isCurrent(token) &&
    epoch === brandOperationSessionGeneration() && props.account.id === accountId && props.brandId === brandId) {
    emit("session-invalid");
  }
}

function reviewChange(): void {
  error.value = "";
  notice.value = "";
  if (!record.value || !canWrite.value) return;
  if (targetStatus.value === record.value.status) { error.value = message("目标状态与当前状态相同。", "The target status is the same as the current status."); return; }
  if (!reason.value.trim() || reasonBytes.value > 500) { error.value = message("请填写操作原因（最多 500 字节）。", "Enter a reason for the change (up to 500 bytes)."); return; }
  const intent: PendingBrandOperationWrite = {
    accountId: props.account.id,
    brandId: props.brandId,
    body: { version: record.value.version, status: targetStatus.value, reason: reason.value },
    key: createIdempotencyKey(),
    phase: "review",
  };
  setPendingBrandOperationWrite({ accountId: intent.accountId, brandId: intent.brandId }, intent);
  pending.value = getPendingBrandOperationWrite({ accountId: intent.accountId, brandId: intent.brandId });
}

function cancelReview(): void {
  if (!pending.value || (pending.value.phase !== "review" && !(pending.value.phase === "conflict" && conflictReloaded.value))) return;
  clearPendingBrandOperationWrite({ accountId: pending.value.accountId, brandId: pending.value.brandId }, pending.value.key);
  pending.value = null;
  conflictReloaded.value = false;
  notice.value = message("已放弃旧请求；请基于最新版本重新核对。", "The previous request was discarded. Review the change against the latest version.");
}

async function sendFrozenIntent(retry: boolean): Promise<void> {
  const intent = pending.value;
  if (!intent || busy.value || (retry ? intent.phase !== "unknown" : intent.phase !== "review")) return;
  const epoch = brandOperationSessionGeneration();
  const token = writeGuard.begin();
  busy.value = true;
  error.value = "";
  notice.value = "";
  updatePendingBrandOperationPhase({ accountId: intent.accountId, brandId: intent.brandId }, intent.key, "unknown");
  pending.value = getPendingBrandOperationWrite({ accountId: intent.accountId, brandId: intent.brandId });
  try {
    const receipt = await api.patch(intent.brandId, intent.body, intent.key);
    if (epoch !== brandOperationSessionGeneration()) return;
    clearPendingBrandOperationWrite({ accountId: intent.accountId, brandId: intent.brandId }, intent.key);
    if (props.account.id === intent.accountId && props.brandId === intent.brandId) pending.value = null;
    emit("changed", { accountId: intent.accountId, brandId: intent.brandId, status: receipt.status as BrandOperationTargetStatus, version: receipt.version });
    if (writeGuard.isCurrent(token) && props.account.id === intent.accountId && props.brandId === intent.brandId) {
      notice.value = message("变更回执已核验，正在独立读取最新状态…", "The change receipt was verified. Reading the latest status independently…");
      const liveStateRead = await reloadCurrent();
      await reloadHistory(0);
      if (writeGuard.isCurrent(token)) notice.value = liveStateRead
        ? message("变更回执已核验；当前状态已独立重新读取。", "The change receipt was verified and the current status was reloaded independently.")
        : message("变更回执已核验，但读取最新状态失败。当前显示可能不是最新状态，请重新读取。", "The change receipt was verified, but the latest status could not be loaded. The displayed status may be outdated; reload it.");
    }
  } catch (cause) {
    if (epoch !== brandOperationSessionGeneration()) return;
    const status = cause instanceof AdminApiError ? cause.status : undefined;
    const classification = classifyBrandOperationFailure(status);
    if (classification === "unknown") {
      updatePendingBrandOperationPhase({ accountId: intent.accountId, brandId: intent.brandId }, intent.key, "unknown");
      if (props.account.id === intent.accountId && props.brandId === intent.brandId) pending.value = getPendingBrandOperationWrite({ accountId: intent.accountId, brandId: intent.brandId });
      if (writeGuard.isCurrent(token)) error.value = message("提交结果未知。表单与原键已冻结；请使用原键重试，不要创建新请求。", "The submission result is unknown. The form and original key are frozen; retry with the original key and do not create a new request.");
    } else if (classification === "conflict") {
      updatePendingBrandOperationPhase({ accountId: intent.accountId, brandId: intent.brandId }, intent.key, "conflict");
      if (props.account.id === intent.accountId && props.brandId === intent.brandId) pending.value = getPendingBrandOperationWrite({ accountId: intent.accountId, brandId: intent.brandId });
      conflictReloaded.value = false;
      if (writeGuard.isCurrent(token)) error.value = message("版本或状态冲突。已冻结旧请求，正在重新读取；不得自动换键重试。", "Version or status conflict. The previous request is frozen and the latest state is being reloaded; it cannot be retried with a new key automatically.");
      if (props.account.id === intent.accountId && props.brandId === intent.brandId) await reloadCurrent();
    } else {
      clearPendingBrandOperationWrite({ accountId: intent.accountId, brandId: intent.brandId }, intent.key);
      if (props.account.id === intent.accountId && props.brandId === intent.brandId) pending.value = null;
      if (writeGuard.isCurrent(token)) error.value = errorText(cause, message("变更被服务器拒绝；读取最新状态后可重新核对。", "The server rejected the change. Reload the latest status before reviewing it again."));
      handleSessionError(cause, intent.accountId, intent.brandId, epoch, writeGuard, token);
    }
  } finally {
    if (epoch === brandOperationSessionGeneration() && props.account.id === intent.accountId && props.brandId === intent.brandId) busy.value = false;
  }
}

watch(() => [props.account.id, props.brandId] as const, () => {
  recordGuard.invalidate();
  historyGuard.invalidate();
  writeGuard.invalidate();
  record.value = null;
  rows.value = [];
  offset.value = 0;
  hasMore.value = false;
  error.value = "";
  notice.value = "";
  busy.value = false;
  syncPending();
  if (permissions.value.view) {
    void reloadCurrent();
    void reloadHistory(0);
  }
}, { immediate: true });

onUnmounted(() => {
  recordGuard.invalidate();
  historyGuard.invalidate();
  writeGuard.invalidate();
});
</script>

<template>
  <section class="brand-operation" aria-labelledby="brand-operation-heading">
    <header class="brand-operation__header">
      <div>
        <div class="brand-operation__eyebrow">{{ t("BRAND / OPERATION", "BRAND / OPERATION") }}</div>
        <h2 id="brand-operation-heading">{{ t("品牌运行状态", "Brand operation status") }}</h2>
        <p>{{ t("暂停仅阻止新投注；登录仍可用，已有订单、退款、开奖和结算继续。提现功能尚未完整实现。", "Pausing only blocks new bets. Sign-in remains available, and existing orders, refunds, draws, and settlement continue. Withdrawals are not fully implemented yet.") }}</p>
      </div>
      <button class="brand-operation__secondary" type="button" :disabled="loading" @click="reloadCurrent">{{ t("重新读取状态", "Reload status") }}</button>
    </header>

    <p v-if="!permissions.view" class="brand-operation__notice" role="status">{{ t("当前账号没有此品牌运行状态的查看权限。", "This account cannot view operation status for this brand.") }}</p>
    <template v-else>
      <div class="brand-operation__current" aria-live="polite">
        <div><span>{{ t("当前状态", "Current status") }}</span><strong class="brand-operation__status" :class="statusStyle(record?.status ?? '')">{{ record ? statusText(record.status) : loading ? t("读取中", "Loading") : t("不可用", "Unavailable") }}</strong></div>
        <div><span>{{ t("当前版本", "Current version") }}</span><strong>{{ record?.version ?? "—" }}</strong></div>
        <div class="brand-operation__current-name"><span>{{ t("品牌", "Brand") }}</span><strong>{{ record?.name ?? "—" }}</strong></div>
      </div>

      <p class="brand-operation__muted">{{ t("运行状态与认证设置共享配置版本；其他配置修改后，请重新读取并核对。", "Operation status shares a configuration version with authentication settings. After changing other settings, reload and review before proceeding.") }}</p>
      <p v-if="readonlyBrand" class="brand-operation__warning" role="status">{{ t("该品牌已停用，运行状态变更为只读。", "This brand is disabled, so operation status is read-only.") }}</p>
      <p v-if="unknownWrite" class="brand-operation__warning" role="alert">{{ t("提交结果未知：请求内容及幂等键已冻结。刷新状态或记录不会解除冻结。", "The submission result is unknown. The request and idempotency key are frozen; reloading status or history will not unfreeze them.") }}</p>
      <p v-else-if="conflictedWrite" class="brand-operation__warning" role="alert">{{ t("旧请求已因冲突冻结。先读取最新版本，再明确放弃旧请求；系统不会自动生成新键。", "The previous request is frozen due to a conflict. Load the latest version, then explicitly discard the old request; a new key will not be generated automatically.") }}</p>

      <form class="brand-operation__form" @submit.prevent="reviewChange">
        <label class="brand-operation__field" for="brand-operation-target">
          <span>{{ t("目标状态", "Target status") }}</span>
          <select id="brand-operation-target" v-model="targetStatus" :aria-label="t('目标状态', 'Target status')" :disabled="!canWrite || busy || reviewing">
            <option value="active">{{ t("运行中", "Active") }}</option>
            <option value="paused">{{ t("已暂停", "Paused") }}</option>
          </select>
        </label>
        <label class="brand-operation__field" for="brand-operation-reason">
          <span>{{ t("操作原因", "Reason for operation") }}</span>
          <textarea id="brand-operation-reason" v-model="reason" rows="3" maxlength="500" :aria-label="t('操作原因', 'Reason for operation')" :disabled="!canWrite || busy || reviewing" aria-describedby="brand-operation-reason-count" />
          <small id="brand-operation-reason-count">{{ reasonBytes }} / 500 {{ t("字节（UTF-8）", "bytes (UTF-8)") }}</small>
        </label>
        <button class="brand-operation__primary" type="submit" :disabled="!canWrite || !record || busy || reviewing || !reason.trim() || reasonBytes > 500">{{ t("核对状态变更", "Review status change") }}</button>
      </form>

      <div v-if="reviewing && pending" class="brand-operation__review" role="region" :aria-label="t('确认状态变更', 'Confirm status change')">
        <p>{{ t("将从版本 {version} 变更为“{status}”。原因：{reason}", "Change from version {version} to ‘{status}’. Reason: {reason}", { version: pending.body.version, status: statusText(pending.body.status), reason: pending.body.reason }) }}</p>
        <div class="brand-operation__actions">
          <button class="brand-operation__primary" type="button" :disabled="busy" @click="sendFrozenIntent(false)">{{ t("确认提交", "Confirm submission") }}</button>
          <button class="brand-operation__secondary" type="button" :disabled="busy" @click="cancelReview">{{ t("取消确认", "Cancel confirmation") }}</button>
        </div>
      </div>

      <div v-if="unknownWrite && pending" class="brand-operation__review">
        <p>{{ t("原请求：版本 {version} → {status}；{reason}", "Original request: version {version} → {status}; {reason}", { version: pending.body.version, status: statusText(pending.body.status), reason: pending.body.reason }) }}</p>
        <button class="brand-operation__primary" type="button" :disabled="busy" @click="sendFrozenIntent(true)">{{ t("使用原键重试", "Retry with original key") }}</button>
      </div>
      <div v-if="conflictedWrite" class="brand-operation__actions">
        <button class="brand-operation__secondary" type="button" :disabled="loading" @click="reloadCurrent">{{ t("重新读取最新状态", "Reload latest status") }}</button>
        <button v-if="conflictReloaded" class="brand-operation__secondary" type="button" @click="cancelReview">{{ t("放弃旧请求并返回编辑", "Discard old request and return to editing") }}</button>
      </div>
      <p v-if="error" class="brand-operation__error" role="alert">{{ t(error) }}</p>
      <p v-if="notice" class="brand-operation__notice" role="status">{{ t(notice) }}</p>

      <section class="brand-operation__history" aria-labelledby="brand-operation-history-heading">
        <div class="brand-operation__history-head">
          <h3 id="brand-operation-history-heading">{{ t("操作记录", "Operation history") }}</h3>
          <button class="brand-operation__secondary" type="button" :disabled="historyLoading" @click="reloadHistory(0)">{{ t("刷新记录", "Refresh history") }}</button>
        </div>
        <p v-if="historyLoading && !rows.length" class="brand-operation__muted">{{ t("正在读取记录…", "Loading history…") }}</p>
        <p v-else-if="!rows.length" class="brand-operation__muted">{{ t("暂无运行状态变更记录。", "No operation status changes yet.") }}</p>
        <ol v-else class="brand-operation__history-list">
          <li v-for="item in rows" :key="item.id">
            <div class="brand-operation__history-title"><strong>v{{ item.version }} · {{ statusText(item.previous_status) }} → {{ statusText(item.status) }}</strong><time :datetime="item.created_at">{{ timeText(item.created_at) }}</time></div>
            <p>{{ item.reason }}</p>
            <small>{{ t("操作人", "Changed by") }} {{ item.changed_by }} · {{ t("审计", "Audit") }} {{ item.audit_log_id }}</small>
          </li>
        </ol>
        <div class="brand-operation__pagination">
          <button class="brand-operation__secondary" type="button" :disabled="offset === 0 || historyLoading" @click="reloadHistory(Math.max(0, offset - 20))">{{ t("较新记录", "Newer") }}</button>
          <span>{{ offset + 1 }}–{{ offset + rows.length }}</span>
          <button class="brand-operation__secondary" type="button" :disabled="!hasMore || historyLoading" @click="reloadHistory(offset + 20)">{{ t("较旧记录", "Older") }}</button>
        </div>
      </section>
    </template>
  </section>
</template>

<style scoped>
.brand-operation { min-width: 0; padding: 20px; border: 1px solid #e3e8ee; border-radius: 14px; background: #fff; color: #233044; }
.brand-operation__header, .brand-operation__history-head, .brand-operation__history-title, .brand-operation__actions, .brand-operation__pagination { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.brand-operation__header { align-items: flex-start; }
.brand-operation__eyebrow { color: #77859a; font-size: 11px; letter-spacing: .12em; font-weight: 700; }
.brand-operation h2, .brand-operation h3 { margin: 5px 0 8px; }
.brand-operation__header p, .brand-operation__review p { margin: 0; color: #64748b; line-height: 1.5; }
.brand-operation__current { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; margin: 20px 0; padding: 14px; border-radius: 10px; background: #f7f9fc; }
.brand-operation__current > div { display: flex; flex-direction: column; gap: 5px; min-width: 0; }
.brand-operation__current span, .brand-operation__muted { color: #738197; font-size: 13px; }
.brand-operation__current strong, .brand-operation__history small { overflow-wrap: anywhere; }
.brand-operation__status { width: fit-content; border-radius: 999px; padding: 3px 9px; font-size: 13px; }
.is-active { background: #e4f6ed; color: #177245; }.is-paused { background: #fff3d9; color: #8c5b00; }.is-disabled { background: #eceff3; color: #5e6875; }
.brand-operation__warning, .brand-operation__notice, .brand-operation__error { padding: 10px 12px; border-radius: 8px; line-height: 1.45; overflow-wrap: anywhere; }
.brand-operation__warning { background: #fff5df; color: #805600; }.brand-operation__notice { background: #eef5ff; color: #315780; }.brand-operation__error { background: #fff0f0; color: #9a2f2f; }
.brand-operation__form { display: grid; grid-template-columns: minmax(150px, .7fr) minmax(220px, 1.3fr) auto; align-items: end; gap: 14px; margin: 18px 0; }
.brand-operation__field { display: flex; min-width: 0; flex-direction: column; gap: 6px; font-weight: 600; }
.brand-operation__field select, .brand-operation__field textarea { box-sizing: border-box; width: 100%; min-width: 0; border: 1px solid #cbd4df; border-radius: 8px; padding: 9px 10px; background: #fff; color: inherit; font: inherit; }
.brand-operation__field textarea { resize: vertical; }.brand-operation__field small { color: #718096; font-weight: 400; }
.brand-operation__primary, .brand-operation__secondary { max-width: 100%; min-height: 38px; border-radius: 8px; padding: 8px 13px; font: inherit; cursor: pointer; }
.brand-operation__primary { border: 1px solid #245fce; background: #245fce; color: #fff; }.brand-operation__secondary { border: 1px solid #cbd4df; background: #fff; color: #34445a; }
.brand-operation button:disabled, .brand-operation select:disabled, .brand-operation textarea:disabled { cursor: not-allowed; opacity: .58; }
.brand-operation__review { display: grid; gap: 12px; margin: 16px 0; padding: 14px; border: 1px solid #d8e1ec; border-radius: 10px; background: #fafbfd; }
.brand-operation__history { margin-top: 25px; border-top: 1px solid #e7ebf0; padding-top: 15px; }.brand-operation__history-head h3 { margin: 0; }
.brand-operation__history-list { margin: 12px 0; padding: 0; list-style: none; }.brand-operation__history-list li { min-width: 0; padding: 12px 0; border-bottom: 1px solid #edf0f4; }
.brand-operation__history-title { align-items: flex-start; flex-wrap: wrap; }.brand-operation__history-title time, .brand-operation__history-list small { color: #718096; font-size: 12px; }
.brand-operation__history-list p { margin: 7px 0; overflow-wrap: anywhere; }.brand-operation__pagination { justify-content: flex-end; margin-top: 12px; }.brand-operation__pagination span { color: #718096; font-size: 13px; }
@media (max-width: 700px) { .brand-operation { padding: 15px; } .brand-operation__header { flex-direction: column; } .brand-operation__current { grid-template-columns: repeat(2, minmax(0, 1fr)); } .brand-operation__current-name { grid-column: 1 / -1; } .brand-operation__form { grid-template-columns: minmax(0, 1fr); } .brand-operation__form > button { width: 100%; } .brand-operation__actions { flex-wrap: wrap; justify-content: flex-start; } }
</style>
