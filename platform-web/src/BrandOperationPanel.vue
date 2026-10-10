<script setup lang="ts">
import { ref } from 'vue'
import { PlatformApiError } from './platform-api'
import { createPlatformBrandOperationApi, type BrandOperation, type BrandOperationUpdate } from './brand-operation-api'

const props = defineProps<{ locale: 'en' | 'zh-CN'; allowed: boolean }>()
const emit = defineEmits<{ updated: [record: BrandOperation]; failure: [cause: unknown] }>()
const api = createPlatformBrandOperationApi()
const open = ref(false)
const loading = ref(false)
const saving = ref(false)
const unknownPending = ref(false)
const error = ref('')
const record = ref<BrandOperation | null>(null)
const reason = ref('')
const pending = ref<{ brandId: string; body: Readonly<BrandOperationUpdate>; key: string } | null>(null)
const en = {
  title: 'Brand operating status', read: 'Reading current status…', active: 'Active', paused: 'Paused',
  action: 'will change to', reason: 'Reason', cancel: 'Close', confirm: 'Confirm status change', retry: 'Retry the same request',
  unknown: 'The result is unconfirmed. Retry only with the original status, version, reason and key.',
}
const zh = {
  title: '品牌运行状态', read: '正在读取最新状态…', active: '运行中', paused: '已暂停',
  action: '将变更为', reason: '变更原因', cancel: '关闭', confirm: '确认变更状态', retry: '使用原请求重试',
  unknown: '结果尚未确认。只能使用原状态、版本、原因和请求键重试。',
}
const copy = () => props.locale === 'en' ? en : zh
const isUnknown = (cause: unknown) => cause instanceof PlatformApiError && (cause.network || cause.status >= 500 || cause.code === 'INVALID_RESPONSE')

async function openForBrand(brandId: string) {
  if (!props.allowed || loading.value || saving.value) return
  open.value = true
  error.value = ''
  if (pending.value) return
  record.value = null
  reason.value = ''
  loading.value = true
  try {
    record.value = await api.read(brandId)
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Request failed'
    if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  } finally {
    loading.value = false
  }
}

async function submit() {
  if (!props.allowed || saving.value || loading.value) return
  if (!pending.value) {
    if (!record.value || !reason.value.trim()) return
    const body = Object.freeze({
      version: record.value.version,
      status: record.value.status === 'active' ? 'paused' : 'active',
      reason: reason.value.trim(),
    })
    pending.value = { brandId: record.value.brand_id, body, key: crypto.randomUUID() }
  }
  const intent = pending.value
  unknownPending.value = false
  saving.value = true
  error.value = ''
  try {
    const updated = await api.update(intent.brandId, intent.body, intent.key)
    pending.value = null
    unknownPending.value = false
    record.value = updated
    open.value = false
    emit('updated', updated)
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Request failed'
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

defineExpose({ openForBrand })
</script>

<template>
  <div v-if="open" class="modal-scrim">
    <section class="modal-card" role="dialog" aria-modal="true" :aria-label="copy().title">
      <div class="modal-heading"><h2>{{ copy().title }}</h2><button class="icon-button" type="button" :disabled="saving" :aria-label="copy().cancel" @click="open = false">×</button></div>
      <p v-if="loading" role="status">{{ copy().read }}</p>
      <template v-if="record">
        <dl class="confirm-list">
          <div><dt>{{ locale === 'en' ? 'Brand' : '品牌' }}</dt><dd>{{ record.name }} · {{ record.brand_id }}</dd></div>
          <div><dt>{{ pending ? (locale === 'en' ? 'Request snapshot' : '请求快照') : (locale === 'en' ? 'Current status' : '当前状态') }}</dt><dd>{{ record.status === 'active' ? copy().active : copy().paused }} · v{{ record.version }}</dd></div>
          <div><dt>{{ copy().action }}</dt><dd>{{ pending ? (pending.body.status === 'active' ? copy().active : copy().paused) : record.status === 'active' ? copy().paused : copy().active }}</dd></div>
          <div v-if="pending"><dt>{{ copy().reason }}</dt><dd>{{ pending.body.reason }}</dd></div>
        </dl>
        <p v-if="pending && unknownPending" class="message" role="status">{{ copy().unknown }}</p>
        <label v-else-if="!pending" class="brand-form">{{ copy().reason }}<textarea v-model="reason" required rows="3" /></label>
      </template>
      <p v-if="error" class="message error" role="alert">{{ error }}</p>
      <div class="form-actions">
        <button class="secondary" type="button" :disabled="saving" @click="open = false">{{ copy().cancel }}</button>
        <button v-if="record" class="primary" type="button" :disabled="saving || loading || (!pending && !reason.trim())" @click="submit">{{ saving ? (locale === 'en' ? 'Saving…' : '提交中…') : pending ? copy().retry : copy().confirm }}</button>
      </div>
    </section>
  </div>
</template>
