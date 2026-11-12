<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import SelectControl from '@lottery/shared/SelectControl.vue'
import SiteIcon from '@lottery/shared/SiteIcon'
import { useOperationToast, vGlobalLoading, vInlineValidation, vWriteGuard, withGlobalLoading } from '@lottery/shared'
import { contactKinds, isBrandContactConfig, type BrandContact, type BrandContactConfig } from '../../shared/src/brand-contacts'
import { brandPresentationPermissions } from './brand-presentation-api'
import { createBrandContactsApi, type BrandContactsBody, type BrandContactsRecord } from './brand-contacts-api'
import { AdminApiError, createIdempotencyKey, type AdminAccount } from './admin-api'
import { useAdminI18n } from './i18n'
const props = defineProps<{ account: AdminAccount; brandId: string }>()
const emit = defineEmits<{ 'session-invalid': [] }>()
const { t, errorMessage, display, message } = useAdminI18n()
const api = createBrandContactsApi()
const rights = computed(() => brandPresentationPermissions(props.account, props.brandId))
const record = ref<BrandContactsRecord | null>(null), draft = ref<BrandContactConfig | null>(null)
const loading = ref(false), busy = ref(false), uploading = ref(false), needsRead = ref(false)
const error = ref<ReturnType<typeof errorMessage> | ''>(''), notice = ref<ReturnType<typeof message> | ''>('')
const pending = ref<{ body: BrandContactsBody; key: string } | null>(null)
const locked = computed(() => !rights.value.write || record.value?.status === 'disabled' || loading.value || busy.value || uploading.value || needsRead.value || !!pending.value)
let generation = 0
const clone = <T,>(value: T): T => JSON.parse(JSON.stringify(value))
async function read() {
  const token = generation
  loading.value = true; error.value = ''
  try {
    const next = await api.get(props.brandId)
    if (token !== generation) return
    record.value = next; draft.value = clone(next.config); needsRead.value = false
  } catch (cause) { if (token === generation) { error.value = errorMessage(cause, 'read'); needsRead.value = true; if (cause instanceof AdminApiError && cause.status === 401) emit('session-invalid') } }
  finally { if (token === generation) loading.value = false }
}
function add() {
  if (locked.value || !draft.value) return
  const id = crypto.randomUUID()
  draft.value.items.push({ id, kind: 'wechat', value: '', image_url: null })
  if (draft.value.items.length === 1) draft.value.default_id = id
}
function remove(item: BrandContact) {
  if (locked.value || !draft.value) return
  draft.value.items = draft.value.items.filter(entry => entry.id !== item.id)
  if (draft.value.default_id === item.id) draft.value.default_id = null
}
async function upload(item: BrandContact, event: Event) {
  const input = event.target as HTMLInputElement, file = input.files?.[0]
  if (!file || locked.value) return
  error.value = ''
  if (!['image/png', 'image/jpeg'].includes(file.type) || file.size > 1024 * 1024) { error.value = message('presentation.uploadInvalid'); input.value = ''; return }
  const token = generation
  uploading.value = true
  try { const url = await api.upload(props.brandId, file); if (token === generation) item.image_url = url }
  catch (cause) { if (token === generation) { error.value = errorMessage(cause); if (cause instanceof AdminApiError && cause.status === 401) emit('session-invalid') } }
  finally { if (token === generation) uploading.value = false; input.value = '' }
}
async function save() {
  if (busy.value || uploading.value || !rights.value.write || needsRead.value || !record.value || !draft.value || record.value.status === 'disabled') return
  if (!pending.value) {
    const config = clone(draft.value)
    config.items.forEach(item => item.value = item.value.trim())
    if (!isBrandContactConfig(config)) { error.value = message('contacts.invalid'); return }
    pending.value = { body: { version: record.value.version, config }, key: createIdempotencyKey() }
  }
  const token = generation, frozen = pending.value
  busy.value = true; error.value = ''; notice.value = ''
  try {
    const next = await api.put(props.brandId, frozen.body, frozen.key)
    if (token !== generation) return
    record.value = next; draft.value = clone(next.config); pending.value = null; notice.value = message('admin.settings.saved')
  } catch (cause) {
    if (token !== generation) return
    error.value = errorMessage(cause)
    if (cause instanceof AdminApiError && cause.status > 0 && cause.status < 500 && cause.code !== 'INVALID_RESPONSE') { pending.value = null; if (cause.status === 409) needsRead.value = true }
    if (cause instanceof AdminApiError && cause.status === 401) emit('session-invalid')
  } finally { if (token === generation) busy.value = false }
}
watch(() => [props.account.id, props.brandId, rights.value.view, rights.value.write], () => {
  generation++; record.value = draft.value = null; pending.value = null
  loading.value = busy.value = uploading.value = needsRead.value = false; error.value = notice.value = ''
  if (rights.value.view) void read()
}, { immediate: true })
onUnmounted(() => generation++)
useOperationToast(error, value => display(value), 'error')
useOperationToast(notice, value => display(value), 'success')
</script>
<template>
  <section class="brand-contacts brand-settings-panel" :aria-label="t('contacts.title')">
    <p v-if="!rights.view" role="status">{{ t('admin.copy.d8a49c2f') }}</p>
    <p v-if="loading" v-global-loading role="status">{{ t('admin.copy.efd6ec13') }}</p>
    <form v-if="draft" class="ui-form" novalidate v-inline-validation v-write-guard="busy" @submit.prevent="withGlobalLoading(save)">
      <div class="contact-toolbar"><button v-if="rights.write" type="button" class="button button-secondary" :disabled="locked" @click="add"><SiteIcon name="plus" />{{ t('contacts.add') }}</button></div>
      <p v-if="!draft.items.length" class="empty-state">{{ t('contacts.empty') }}</p>
      <fieldset v-for="(item, index) in draft.items" :key="item.id" class="contact-editor" :disabled="locked" :aria-label="t('contacts.entry', { number: index + 1 })">
        <div class="contact-fields">
          <label>{{ t('contacts.kind') }}<SelectControl v-model="item.kind" :options="contactKinds.map(kind => ({ value: kind, label: t(`contacts.${kind}`) }))" :aria-label="t('contacts.kind')" :disabled="locked" /></label>
          <label>{{ t('contacts.value') }}<input v-model="item.value" maxlength="120" :disabled="locked" /></label>
        </div>
        <div class="contact-image-editor">
          <img v-if="item.image_url" class="contact-image-preview" :src="item.image_url" :alt="t('contacts.image', { kind: t(`contacts.${item.kind}`) })" />
          <div class="contact-image-actions"><label class="button button-secondary contact-upload"><span>{{ t('contacts.upload') }}</span><input v-write-guard="uploading" type="file" accept="image/png,image/jpeg" :aria-label="t('contacts.upload')" :disabled="locked" @change="withGlobalLoading(upload, item, $event)" /></label><button v-if="item.image_url" type="button" class="button button-secondary" :disabled="locked" @click="item.image_url = null">{{ t('contacts.clearImage') }}</button></div>
        </div>
        <div class="contact-row-actions"><label class="contact-default"><input v-model="draft.default_id" type="radio" name="default-contact" :value="item.id" :disabled="locked" />{{ t('contacts.default') }}</label><button type="button" class="button button-secondary" :disabled="locked" @click="remove(item)">{{ t('contacts.remove') }}</button></div>
      </fieldset>
      <p v-if="pending && !busy" role="status">{{ t('contacts.unknown') }}</p>
      <div v-if="rights.write" class="ui-form-actions"><button type="submit" class="button button-primary" :disabled="busy || uploading || loading || needsRead || record?.status === 'disabled'">{{ pending && !busy ? t('contacts.retry') : t('presentation.save') }}</button></div>
    </form>
    <button v-if="needsRead" type="button" class="button button-secondary" :disabled="busy || loading" @click="withGlobalLoading(read)">{{ t('admin.settings.readAgain') }}</button>
  </section>
</template>
<style scoped>
.brand-contacts { min-width:0; }
.brand-contacts > .ui-form { max-width:860px; }
.contact-toolbar,.contact-image-actions,.contact-row-actions { display:flex;align-items:center;gap:8px;flex-wrap:wrap; }
.contact-fields { display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px; }
#app .ui-form > .contact-editor { padding:12px;border:1px solid var(--line);border-radius:var(--radius-sm); }
.contact-image-editor { display:flex;align-items:flex-start;gap:12px;flex-wrap:wrap;min-width:0; }
.contact-image-preview { display:block;max-width:100%;width:80px;height:80px;object-fit:contain;border:1px solid var(--line);border-radius:var(--radius-sm);background:var(--surface); }
.contact-upload { position:relative;overflow:hidden; }
.contact-upload input { position:absolute;inset:0;width:100%;height:100%;opacity:0;cursor:pointer; }
.contact-upload:focus-within { outline:2px solid var(--primary);outline-offset:2px; }
.contact-row-actions { justify-content:space-between; }
.contact-default { display:flex;align-items:center;gap:8px;min-height:44px; }
@media(max-width:640px) { .contact-fields { grid-template-columns:minmax(0,1fr);gap:8px; } }
@media(max-width:900px), (pointer:coarse) { #app .ui-form > .contact-editor { padding:8px;row-gap:8px; } .contact-image-editor { gap:8px; } }
</style>
