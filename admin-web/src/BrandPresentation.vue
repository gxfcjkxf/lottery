<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useOperationToast, vWriteGuard, vRefreshButton, vGlobalLoading, vInlineValidation, withGlobalLoading, type LocalizedMessage } from '@lottery/shared'
import { AdminApiError, createIdempotencyKey, type AdminAccount } from './admin-api'
import { useAdminI18n } from './i18n'
import PresentationFields from './PresentationFields.vue'
import { brandPresentationPermissions, createBrandPresentationApi, validBrandPresentationConfig, type BrandPresentationEffective, type BrandPresentationConfig, type BrandPresentationRecord } from './brand-presentation-api'
const props = defineProps<{ account: AdminAccount; brandId: string }>()
const emit = defineEmits<{ 'session-invalid': []; loaded: [value: { accountId: string; record: BrandPresentationRecord }] }>()
const { t, message, display, errorMessage } = useAdminI18n()
const api = createBrandPresentationApi()
const rights = computed(() => brandPresentationPermissions(props.account, props.brandId))
const record = ref<BrandPresentationRecord | null>(null), draft = ref<BrandPresentationConfig | null>(null)
const defaults = ref<BrandPresentationEffective | null>(null)
const loading = ref(false), busy = ref(false), uploadBusy = ref(false), readFailed = ref(false), needsRead = ref(false)
const error = ref<string | LocalizedMessage>(''), notice = ref<string | LocalizedMessage>('')
const clone = <T,>(value: T): T => JSON.parse(JSON.stringify(value)) as T
const editable = computed(() => rights.value.write && record.value?.status !== 'disabled' && !uploadBusy.value && !busy.value && !loading.value && !readFailed.value && !needsRead.value)
let generation = 0, readTicket = 0, live = true
const current = (token: number) => live && token === generation
async function readLatest(preserveDraft = false) {
 const token = generation, ticket = ++readTicket, brandId = props.brandId, accountId = props.account.id
 loading.value = true; error.value = ''
 try {
  const [next, base] = await Promise.all([api.get(brandId), api.defaults(brandId)])
  if (!current(token) || ticket !== readTicket) return false
  record.value = next; defaults.value = base; readFailed.value = needsRead.value = false
  if (!preserveDraft || !draft.value) draft.value = clone(next.config)
  emit('loaded', { accountId, record: next }); return true
 } catch (cause) {
  if (current(token) && ticket === readTicket) { readFailed.value = true; error.value = errorMessage(cause, 'read'); if (cause instanceof AdminApiError && cause.status === 401) emit('session-invalid') }
  return false
 } finally { if (current(token) && ticket === readTicket) loading.value = false }
}
async function save() {
 if (!editable.value || !record.value || !draft.value) return
 if (!validBrandPresentationConfig(draft.value)) { error.value = message('admin.copy.e8d079a3'); return }
 const token = generation, brandId = props.brandId
 const body = { version: record.value.version, config: clone(draft.value), reason: '' }
 busy.value = true; error.value = ''; notice.value = ''
 try {
  const result = await api.put(brandId, body, createIdempotencyKey())
  if (!current(token)) return
  record.value = result; draft.value = clone(result.config)
  emit('loaded', { accountId: props.account.id, record: result })
  notice.value = message('admin.settings.saved')
 } catch (cause) {
  if (!current(token)) return
  error.value = errorMessage(cause)
  if (!(cause instanceof AdminApiError) || cause.status >= 500 || cause.status === 0 || cause.code === 'INVALID_RESPONSE' || cause.status === 409) needsRead.value = true
  if (cause instanceof AdminApiError && cause.status === 401) emit('session-invalid')
 } finally { if (current(token)) busy.value = false }
}
watch(() => [props.account.id, props.brandId, rights.value.view, rights.value.write], () => {
 generation++; record.value = null; defaults.value = null; draft.value = null
 loading.value = busy.value = readFailed.value = needsRead.value = uploadBusy.value = false; error.value = notice.value = ''
 if (rights.value.view) void readLatest()
}, { immediate: true })
onUnmounted(() => { live = false; generation++ })
useOperationToast(error, value => display(value), 'error')
useOperationToast(notice, value => display(value), 'success')
</script>

<template>
 <section class="brand-presentation brand-settings-panel" :aria-label="t('admin.copy.5f829ca8')">
  <p v-if="!rights.view" role="status">{{t('admin.copy.d8a49c2f')}}</p>
  <template v-else>
   <p v-global-loading v-if="loading&&!record" role="status">{{t('admin.copy.efd6ec13')}}</p>
   <p v-if="record?.status==='disabled'" role="status">{{t('admin.copy.81b3d5fa')}}</p>
   <form class="ui-form" v-write-guard="busy" novalidate v-inline-validation v-if="record&&draft&&defaults" @submit.prevent="withGlobalLoading(save)">
    <PresentationFields v-model:config="draft" :defaults="defaults" :brand-id="brandId" @session-invalid="emit('session-invalid')" @uploading="uploadBusy=$event" :disabled="!editable">
     <template #actions><button class="button button-primary" type="submit" :disabled="!editable">{{t('presentation.save')}}</button></template>
    </PresentationFields>
   </form>
   <p v-if="needsRead" role="status">{{t('settings.reloadBeforeSave')}}</p>
   <button v-refresh-button="t('admin.settings.readAgain')" v-if="readFailed || needsRead" class="button button-secondary" type="button" :disabled="loading || busy" @click="withGlobalLoading(readLatest, true)">{{t('admin.settings.readAgain')}}</button>
  </template>
 </section>
</template>
<style scoped>
.brand-presentation form { display:grid;gap:12px;min-width:0; }
</style>
