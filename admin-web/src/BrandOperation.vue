<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
import DetailModal from '@lottery/shared/DetailModal.vue'
import { computed, onUnmounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { useAdminI18n } from "./i18n";
import type { LocalizedMessage } from "@lottery/shared";
import {
  brandOperationPermissions,
  createBrandOperationApi,
  type BrandOperation,
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
const { t, message, display } = useAdminI18n();
const recordGuard = createBrandOperationRequestGuard();
const writeGuard = createBrandOperationRequestGuard();
const permissions = computed(() => brandOperationPermissions(props.account, props.brandId));
const record = ref<BrandOperation | null>(null);
const pending = ref<PendingBrandOperationWrite | null>(null);
const busy = ref(false);
const loading = ref(false);
const confirmationOpen = ref(false);
const readFailed = ref(false);
const error = ref<string | LocalizedMessage>("");
const notice = ref<string | LocalizedMessage>("");
const conflictReloaded = ref(false);

const readonlyBrand = computed(() => props.brandStatus === "disabled" || record.value?.status === "disabled");
const unknownWrite = computed(() => pending.value?.phase === "unknown");
const conflictedWrite = computed(() => pending.value?.phase === "conflict");
const reviewing = computed(() => pending.value?.phase === "review");
const nextStatus = computed<BrandOperationTargetStatus | null>(() => record.value?.status === 'active' ? 'paused' : record.value?.status === 'paused' ? 'active' : null);
const actionLabel = computed(() => nextStatus.value === 'paused' ? t('brandOperation.pause') : t('brandOperation.enable'));
const canWrite = computed(() => permissions.value.write && !readonlyBrand.value && !pending.value && !busy.value && !loading.value && !readFailed.value);
const statusText = (value: string) => value === "active" ? t('admin.copy.e410d276') : value === "paused" ? t('admin.copy.a32d8f3f') : value === "disabled" ? t('admin.copy.1b7158cb') : value;
const statusStyle = (value: string) => value === "active" ? "is-active" : value === "paused" ? "is-paused" : "is-disabled";
const syncPending = () => {
  pending.value = getPendingBrandOperationWrite({ accountId: props.account.id, brandId: props.brandId });
  conflictReloaded.value = false;
  confirmationOpen.value = Boolean(pending.value);
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
    readFailed.value = false;
    if (pending.value?.phase === "conflict") conflictReloaded.value = true;
    return true;
  } catch (cause) {
    if (recordGuard.isCurrent(token) && epoch === brandOperationSessionGeneration() && props.account.id === accountId && props.brandId === brandId) {
      readFailed.value = true;
      error.value = cause instanceof Error ? publicErrorMessage(cause, 'read') : message('admin.copy.a69016b7');
      handleSessionError(cause, accountId, brandId, epoch, recordGuard, token);
    }
    return false;
  } finally {
    if (recordGuard.isCurrent(token) && epoch === brandOperationSessionGeneration()) loading.value = false;
  }
}

function errorText(cause: unknown, fallback: LocalizedMessage): string | LocalizedMessage {
  return cause instanceof Error ? publicErrorMessage(cause) : fallback;
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
  if (!record.value || !canWrite.value || !nextStatus.value) return;
  const intent: PendingBrandOperationWrite = {
    accountId: props.account.id,
    brandId: props.brandId,
    body: { version: record.value.version, status: nextStatus.value, reason: '' },
    key: createIdempotencyKey(),
    phase: "review",
  };
  setPendingBrandOperationWrite({ accountId: intent.accountId, brandId: intent.brandId }, intent);
  pending.value = getPendingBrandOperationWrite({ accountId: intent.accountId, brandId: intent.brandId });
  confirmationOpen.value = true;
}

function cancelReview(): void {
  if (busy.value || !pending.value || (pending.value.phase !== "review" && !(pending.value.phase === "conflict" && conflictReloaded.value))) return;
  clearPendingBrandOperationWrite({ accountId: pending.value.accountId, brandId: pending.value.brandId }, pending.value.key);
  pending.value = null;
  conflictReloaded.value = false;
  confirmationOpen.value = false;
}

function closeConfirmation(): void {
  if (busy.value) return;
  if (reviewing.value) cancelReview();
  else confirmationOpen.value = false;
}

async function sendFrozenIntent(retry: boolean): Promise<void> {
  const intent = pending.value;
  if (!intent || busy.value || !permissions.value.write || readonlyBrand.value || (retry ? intent.phase !== "unknown" : intent.phase !== "review")) return;
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
    if (props.account.id === intent.accountId && props.brandId === intent.brandId) { pending.value = null; confirmationOpen.value = false; }
    emit("changed", { accountId: intent.accountId, brandId: intent.brandId, status: receipt.status as BrandOperationTargetStatus, version: receipt.version });
    if (writeGuard.isCurrent(token) && props.account.id === intent.accountId && props.brandId === intent.brandId) {
      notice.value = message('brandOperation.updated');
      const liveStateRead = await reloadCurrent();
      if (writeGuard.isCurrent(token)) notice.value = liveStateRead
        ? message('brandOperation.updated')
        : message('brandOperation.updatedReadFailed');
    }
  } catch (cause) {
    if (epoch !== brandOperationSessionGeneration()) return;
    const status = cause instanceof AdminApiError ? cause.status : undefined;
    const classification = classifyBrandOperationFailure(status);
    if (classification === "unknown") {
      updatePendingBrandOperationPhase({ accountId: intent.accountId, brandId: intent.brandId }, intent.key, "unknown");
      if (props.account.id === intent.accountId && props.brandId === intent.brandId) pending.value = getPendingBrandOperationWrite({ accountId: intent.accountId, brandId: intent.brandId });
      if (writeGuard.isCurrent(token)) error.value = message('brandOperation.unknown');
    } else if (classification === "conflict") {
      updatePendingBrandOperationPhase({ accountId: intent.accountId, brandId: intent.brandId }, intent.key, "conflict");
      if (props.account.id === intent.accountId && props.brandId === intent.brandId) pending.value = getPendingBrandOperationWrite({ accountId: intent.accountId, brandId: intent.brandId });
      conflictReloaded.value = false;
      if (writeGuard.isCurrent(token)) error.value = message('brandOperation.conflict');
      if (props.account.id === intent.accountId && props.brandId === intent.brandId) await reloadCurrent();
    } else {
      clearPendingBrandOperationWrite({ accountId: intent.accountId, brandId: intent.brandId }, intent.key);
      if (props.account.id === intent.accountId && props.brandId === intent.brandId) { pending.value = null; confirmationOpen.value = false; }
      if (writeGuard.isCurrent(token)) error.value = errorText(cause, message('admin.copy.189085fb'));
      handleSessionError(cause, intent.accountId, intent.brandId, epoch, writeGuard, token);
    }
  } finally {
    if (epoch === brandOperationSessionGeneration() && props.account.id === intent.accountId && props.brandId === intent.brandId) busy.value = false;
  }
}

watch(() => [props.account.id, props.brandId] as const, () => {
  recordGuard.invalidate();
  writeGuard.invalidate();
  record.value = null;
  readFailed.value = false;
  error.value = "";
  notice.value = "";
  busy.value = false;
  syncPending();
  if (permissions.value.view) {
    void reloadCurrent();
  }
}, { immediate: true });

onUnmounted(() => {
  recordGuard.invalidate();
  writeGuard.invalidate();
});
</script>

<template>
  <section class="brand-operation" :aria-label="t('admin.copy.febb1b95')">
    <p v-if="!permissions.view" class="message" role="status">{{ t('admin.copy.23b19e91') }}</p>
    <template v-else>
      <div class="brand-operation__summary">
        <div class="brand-operation__identity">
          <strong v-if="record">{{ record.name }}</strong>
          <span>{{ t('admin.copy.a38ce499') }}</span>
          <span v-global-loading="loading && !record" class="status-chip" :class="record ? statusStyle(record.status) : ''">{{ record ? statusText(record.status) : loading ? t('admin.copy.ab7239f3') : t('admin.copy.5dc10169') }}</span>
        </div>
        <button v-if="pending" class="button button-secondary" type="button" :disabled="busy" @click="confirmationOpen = true">{{ t('brandOperation.pendingAction') }}</button>
        <button v-else-if="nextStatus && permissions.write && !readonlyBrand" class="button" :class="nextStatus === 'paused' ? 'button-secondary' : 'button-primary'" type="button" :disabled="!canWrite" @click="reviewChange">{{ actionLabel }}</button>
      </div>
      <p v-if="readonlyBrand" class="message" role="status">{{ t('admin.copy.57ab1456') }}</p>
      <p v-if="error && !confirmationOpen" class="message error" role="alert">{{ display(error) }}</p>
      <p v-if="notice" class="message" role="status">{{ display(notice) }}</p>
      <button v-if="readFailed && !confirmationOpen" class="button button-secondary" type="button" :disabled="loading" @click="withGlobalLoading(reloadCurrent)">{{ t('brandOperation.readLatest') }}</button>
    </template>

    <DetailModal v-if="pending && confirmationOpen" class="brand-operation-dialog" :title="t('admin.copy.5ce8c984')" :close-label="t('association.close')" @close="closeConfirmation">
      <div class="brand-operation__confirmation">
        <p v-if="record">{{ t('brandOperation.confirm', { brand: record.name, status: statusText(pending.body.status) }) }}</p>
        <p v-else>{{ t('admin.copy.4e32f6b1', { status: statusText(pending.body.status) }) }}</p>
        <p v-if="reviewing">{{ pending.body.status === 'paused' ? t('brandOperation.pauseEffect') : t('brandOperation.enableEffect') }}</p>
        <p v-if="pending.body.reason">{{ pending.body.reason }}</p>
        <p v-if="error && !unknownWrite && !conflictedWrite" class="message error" role="alert">{{ display(error) }}</p>
        <p v-if="unknownWrite && !busy" class="message" role="alert">{{ t('brandOperation.unknown') }}</p>
        <p v-else-if="conflictedWrite" class="message" role="alert">{{ t('brandOperation.conflict') }}</p>
      </div>
      <template #footer>
        <button v-if="reviewing" class="button button-secondary" type="button" :disabled="busy" @click="cancelReview">{{ t('admin.copy.96f851ea') }}</button>
        <button v-write-guard="busy" v-if="reviewing" class="button button-primary" type="button" :disabled="busy || !permissions.write || readonlyBrand" @click="withGlobalLoading(sendFrozenIntent, false)">{{ t('admin.copy.a1a8013d') }}</button>
        <button v-write-guard="busy" v-if="unknownWrite" class="button button-primary" type="button" :disabled="busy || !permissions.write || readonlyBrand" @click="withGlobalLoading(sendFrozenIntent, true)">{{ t('brandOperation.retry') }}</button>
        <template v-if="conflictedWrite">
          <button v-if="!conflictReloaded" class="button button-secondary" type="button" :disabled="loading || busy" @click="withGlobalLoading(reloadCurrent)">{{ t('brandOperation.readLatest') }}</button>
          <button v-else class="button button-secondary" type="button" :disabled="busy" @click="cancelReview">{{ t('brandOperation.startAgain') }}</button>
        </template>
      </template>
    </DetailModal>
  </section>
</template>

<style scoped>
.brand-operation { min-width: 0; padding: 16px; border: 1px solid var(--line); border-radius: var(--radius-sm); background: var(--surface); }
.brand-operation__summary, .brand-operation__identity { display: flex; align-items: center; flex-wrap: wrap; gap: 12px; min-width: 0; }
.brand-operation__summary { justify-content: space-between; }
.brand-operation__identity strong { overflow-wrap: anywhere; }
.brand-operation__identity > span:not(.status-chip) { color: var(--muted); }
.brand-operation .message { margin: 12px 0 0; }
.status-chip.is-active { background: var(--success-bg); color: var(--success-text); }
.status-chip.is-paused { background: var(--warning-bg); color: var(--warning-text); }
.status-chip.is-disabled { background: var(--line-subtle); color: var(--ink-secondary); }
.brand-operation-dialog { width: min(480px, calc(100vw - 24px)); }
.brand-operation__confirmation { display: grid; gap: 12px; }
.brand-operation__confirmation p { margin: 0; overflow-wrap: anywhere; }
</style>
