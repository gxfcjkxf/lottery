<script setup lang="ts">
import SelectControl from '@lottery/shared/SelectControl.vue'
import { useOperationToast } from "@lottery/shared";
import DetailModal from "@lottery/shared/DetailModal.vue"
import { vWriteGuard } from "@lottery/shared";
import { withGlobalLoading } from '@lottery/shared';
import { usePlatformI18n } from "./i18n"
const ui = usePlatformI18n().t
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import PlatformTemporalInput from '@lottery/shared/PlatformTemporalInput.vue'
import { lotteryLocalTimeToISO, vInlineValidation, formatPlatformDateTime, type PlatformDisplaySettings } from '@lottery/shared'
import { platformErrorMessage } from './platform-error-message'
import { PlatformApiError } from './platform-api'
import { createPlatformBrandOperationApi, type BrandOperation, type BrandOperationUpdate } from './brand-operation-api'

const props = defineProps<{ locale: 'en' | 'zh-CN'; allowed: boolean; displaySettings: PlatformDisplaySettings }>()
const emit = defineEmits<{ updated: [record: BrandOperation]; failure: [cause: unknown] }>()
const api = createPlatformBrandOperationApi()
let readGeneration = 0, alive = true
const open = ref(false)
const loading = ref(false)
const saving = ref(false)
const unknownPending = ref(false)
const error = ref('')
const target = ref<BrandOperationUpdate['status']>('paused')
const reason = ref('')
const timed = ref(false)
const startsAt = ref('')
const endsAt = ref('')
const reasonLength = computed(() => Array.from(reason.value).length)
const reasonValid = computed(() => reasonLength.value <= 500)
const canSubmit = computed(() => !!pending.value || (reasonValid.value && (!timed.value || target.value !== 'paused' || (!!startsAt.value && !!endsAt.value))))
const statusText = (status: string) => status === 'active' ? copy.value.active : status === 'paused' ? copy.value.paused : ui('brandOperation.stopped')
const dateText = (value: string) => formatPlatformDateTime(value, props.displaySettings, props.locale)
const record = ref<BrandOperation | null>(null)
const pending = ref<{ brandId: string; body: Readonly<BrandOperationUpdate>; key: string } | null>(null)
const copy = computed(() => ({
  title: ui('platform.copy.76f0b47f'), brand: ui('platform.copy.d5b68c0f'), requestSnapshot: ui('platform.copy.e8288885'), currentStatus: ui('platform.copy.f00e558d'), saving: ui('platform.copy.999d870e'), active: ui('platform.copy.a727eaa4'), paused: ui('platform.copy.3d294087'),
  action: ui('platform.copy.7cec5974'), cancel: ui('platform.copy.27b2fd29'), confirm: ui('platform.copy.227ed5ec'), retry: ui('platform.copy.0420fc32'),
  unknown: ui('platform.copy.0cdc5954'),
}))
const isUnknown = (cause: unknown) => cause instanceof PlatformApiError && (cause.network || cause.status >= 500 || cause.code === 'INVALID_RESPONSE')

async function openForBrand(brandId: string) {
  if (!props.allowed || loading.value || saving.value) return
  const ticket = ++readGeneration
  open.value = true
  error.value = ''
  if (pending.value) return
  record.value = null
  loading.value = true
  try {
    const result = await api.read(brandId)
    if (ticket !== readGeneration) return
    record.value = result
    target.value = record.value.status === 'active' ? 'paused' : 'active'
    reason.value = ''
    timed.value = false
    startsAt.value = ''
    endsAt.value = ''
  } catch (cause) {
    if (ticket !== readGeneration) return
    error.value = platformErrorMessage(cause, props.locale)
    if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  } finally {
    if (ticket === readGeneration) loading.value = false
  }
}

async function submit() {
  if (!props.allowed || saving.value || loading.value) return
  if (!pending.value) {
    if (!record.value) return
    if (!canSubmit.value) return
    let window: Pick<BrandOperationUpdate, 'pause_starts_at' | 'pause_ends_at'> = {}
    if (target.value === 'paused' && timed.value) {
      try {
        const start = lotteryLocalTimeToISO(startsAt.value, props.displaySettings.timezone)
        const end = lotteryLocalTimeToISO(endsAt.value, props.displaySettings.timezone)
        if (Date.parse(end) <= Math.max(Date.parse(start), Date.now())) throw new Error()
        window = { pause_starts_at: start, pause_ends_at: end }
      } catch { error.value = ui('brandOperation.invalidWindow'); return }
    }
    const body = Object.freeze({
      version: record.value.version,
      status: target.value,
      reason: reason.value.trim(),
      ...window,
    })
    pending.value = { brandId: record.value.brand_id, body, key: crypto.randomUUID() }
  }
  const intent = pending.value
  unknownPending.value = false
  saving.value = true
  error.value = ''
  try {
    const updated = await api.update(intent.brandId, intent.body, intent.key)
    if (!alive) return
    pending.value = null
    unknownPending.value = false
    record.value = updated
    open.value = false
    emit('updated', updated)
  } catch (cause) {
    if (!alive) return
    error.value = platformErrorMessage(cause, props.locale)
    if (isUnknown(cause)) unknownPending.value = true
    else {
      pending.value = null
      unknownPending.value = false
    }
    if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  } finally {
    saving.value = false
  }
}

function close() {
  if (saving.value) return
  readGeneration++; loading.value = false; open.value = false
}
watch(() => props.allowed, () => {
  readGeneration++; loading.value = false
  if (!pending.value && !saving.value) { record.value = null; open.value = false }
}, { flush: 'sync' })
onBeforeUnmount(() => { alive = false; readGeneration++ })

defineExpose({ openForBrand })

useOperationToast(error, value => value, 'error');
</script>

<template>
  <DetailModal v-if="open" class="confirmation-dialog brand-operation-modal" :title="copy.title" :close-label="ui('platform.copy.27b2fd29')" :close-disabled="saving" @close="close">
      <template v-if="record">
        <dl class="confirm-list">
          <div><dt>{{ copy.brand }}</dt><dd>{{ record.name }}</dd></div>
          <div><dt>{{ pending ? copy.requestSnapshot : copy.currentStatus }}</dt><dd>{{ statusText(record.status) }}</dd></div>
          <div v-if="record.pause_starts_at && record.pause_ends_at"><dt>{{ ui('brandOperation.currentWindow') }}</dt><dd>{{ dateText(record.pause_starts_at) }} → {{ dateText(record.pause_ends_at) }}</dd></div>
        </dl>
        <form v-write-guard="saving" novalidate v-inline-validation v-if="!pending" id="brand-operation-form" class="brand-form brand-operation-form ui-form" @submit.prevent="withGlobalLoading(submit)">
          <div class="brand-operation-options">
          <label>{{ copy.action }}<SelectControl v-model="target" :disabled="saving" :label="String(copy.action)" :options="[...((record.status !== 'active' || record.pause_starts_at) ? [{ value: 'active', label: (String(copy.active)).trim() }] : []), ...((record.status !== 'disabled') ? [{ value: 'paused', label: (String(copy.paused)).trim() }] : []), ...((record.status !== 'disabled') ? [{ value: 'disabled', label: (String(ui('brandOperation.stopped'))).trim() }] : [])]" /></label>
          <label v-if="target === 'paused'">{{ ui('brandOperation.pauseMode') }}<SelectControl v-model="timed" :disabled="saving" :label="String(ui('brandOperation.pauseMode'))" :options="[{ value: false, label: (String(ui('brandOperation.manual'))).trim() }, { value: true, label: (String(ui('brandOperation.timed'))).trim() }]" /></label>
          </div>
          <template v-if="target === 'paused' && timed">
            <p class="brand-operation-hint">{{ ui('brandOperation.scheduleHint') }}</p>
            <label>{{ ui('brandOperation.startsAt') }}<PlatformTemporalInput v-model="startsAt" type="datetime-local" :locale="locale" required :disabled="saving" /></label>
            <label>{{ ui('brandOperation.endsAt') }}<PlatformTemporalInput v-model="endsAt" type="datetime-local" :locale="locale" required :disabled="saving" /></label>
          </template>
          <p v-if="target === 'disabled'" class="brand-operation-hint">{{ ui('brandOperation.stopHint') }}</p>

          <p v-if="reason && !reasonValid" class="message error" role="alert">{{ ui('brandOperation.invalidReason') }}</p>
        </form>
        <dl v-else class="confirm-list">
          <div><dt>{{ copy.action }}</dt><dd>{{ statusText(pending.body.status) }}</dd></div>
          <div v-if="pending.body.pause_starts_at && pending.body.pause_ends_at"><dt>{{ ui('brandOperation.currentWindow') }}</dt><dd>{{ dateText(pending.body.pause_starts_at) }} → {{ dateText(pending.body.pause_ends_at) }}</dd></div>
          <div><dt v-if="pending.body.reason">{{ ui('brandOperation.reason') }}</dt><dd v-if="pending.body.reason">{{ pending.body.reason }}</dd></div>
        </dl>
        <p v-if="pending && unknownPending" class="message" role="status">{{ copy.unknown }}</p>
      </template>

      <div class="form-actions ui-form-actions">

        <button v-write-guard="saving" v-if="record" class="button button-primary" type="button" :disabled="saving || loading || !allowed || !canSubmit" @click="withGlobalLoading(submit)">{{ saving ? copy.saving : pending ? copy.retry : copy.confirm }}</button>
      </div>
  </DetailModal>
</template>

<style scoped>
.brand-operation-form { display: grid; gap: 12px; margin: 16px 0; }
.brand-operation-options { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.brand-operation-form label { display: grid; gap: 8px; min-width: 0; font-size: 14px; }
.brand-operation-form textarea { width: 100%; min-width: 0; min-height: 44px; font-size: 14px; box-sizing: border-box; }
.brand-operation-form textarea { display: block; min-height: 112px; resize: vertical; padding-bottom: 30px; }
.brand-operation-reason { display: block; position: relative; min-width: 0; }
.reason-count { position: absolute; right: 16px; bottom: 8px; color: var(--muted); font-size: var(--body-font-size); line-height: 1.5; pointer-events: none; }
.reason-over-limit { color: var(--danger); }
.brand-operation-hint, .confirm-list small { margin: 0; color: var(--muted); font-size: var(--body-font-size); line-height: 1.6; }
.confirm-list { margin: 12px 0; }
.confirm-list small { display: block; font-weight: 400; }
.confirm-list dd { min-width: 0; overflow-wrap: anywhere; white-space: pre-wrap; }
.form-actions { flex-wrap: wrap; }
@media (max-width: 480px) { .brand-operation-options { grid-template-columns: minmax(0, 1fr); } }
</style>
