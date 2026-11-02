<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useOperationToast, vWriteGuard, vInlineValidation, vRefreshButton, withGlobalLoading } from '@lottery/shared'
import ManagementList from '@lottery/shared/ManagementList.vue'
import DetailModal from '@lottery/shared/DetailModal.vue'
import { createPlatformNotificationsApi, validTemplateContent, type Template, type TemplateContent } from './notifications-api'
import { PlatformApiError } from './platform-api'
import { formatPlatformDateTime, type PlatformDisplaySettings } from './platform-format'
import { usePlatformI18n } from './i18n'
const props = defineProps<{ accountId: string; canManage: boolean; locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const { t, errorMessage, displayError, message } = usePlatformI18n()
const api = createPlatformNotificationsApi()
const templates = ref<Template[]>([]), selected = ref<Template | null>(null)
const draft = ref<TemplateContent | null>(null)
const loading = ref(false), writing = ref(false), conflict = ref(false)
const error = ref<ReturnType<typeof errorMessage> | ''>(''), notice = ref<ReturnType<typeof message> | ''>('')
let alive = true
let generation = 0
const label = (key: Template['key']) => t(`templates.name.${key}`)
const valid = computed(() => selected.value && draft.value && validTemplateContent(draft.value, selected.value.key))
const clone = (content: TemplateContent): TemplateContent => ({ en: { ...content.en }, 'zh-CN': { ...content['zh-CN'] } })
async function load() {
 loading.value = true; error.value = ''
 try { const rows = await api.templates(); if (alive) templates.value = rows }
 catch (cause) { if (alive) { error.value = errorMessage(cause); emit('failure', cause) } }
 finally { if (alive) loading.value = false }
}
function open(item: Template) {
 selected.value = item
 draft.value = clone(item.content)
 conflict.value = false
}
function close() { if (writing.value) return; selected.value = null }
async function save() {
 if (!selected.value || !draft.value || !props.canManage || writing.value || conflict.value || !valid.value) return
 const event = selected.value.key, token = generation
 const body = { version: selected.value.version, content: clone(draft.value), reason: '' }
 writing.value = true; error.value = ''; notice.value = ''
 try {
  const result = await api.update(event, body, crypto.randomUUID())
  if (!alive || token !== generation) return
  selected.value = result; draft.value = clone(result.content)
  templates.value = templates.value.map(item => item.key === event ? result : item)
  notice.value = message('templates.saved')
 } catch (cause) {
  if (alive && token === generation) {
   conflict.value = !(cause instanceof PlatformApiError) || cause.status === 0 || cause.status >= 500 || cause.status === 409 || cause.code === 'INVALID_RESPONSE'
   error.value = errorMessage(cause); emit('failure', cause)
  }
 } finally { if (alive && token === generation) writing.value = false }
}
async function reloadSelected() {
 const event = selected.value?.key
 await load()
 const item = templates.value.find(row => row.key === event)
 if (item && !error.value && selected.value?.key === event) { selected.value = item; conflict.value = false }
}
watch(() => [props.accountId, props.canManage], () => { generation++; selected.value = null; draft.value = null; writing.value = conflict.value = false; error.value = notice.value = '' })
onMounted(load)
onBeforeUnmount(() => { alive = false; generation++ })
useOperationToast(error, value => displayError(value), 'error')
useOperationToast(notice, value => displayError(value), 'success')
</script>
<template>
 <ManagementList :busy="loading" data-testid="platform-template-list">
  <template #actions><button v-refresh-button="t('common.refresh')" class="button button-secondary" :disabled="loading" @click="withGlobalLoading(load)">{{ t('common.refresh') }}</button></template>
  <div class="table-wrap"><table class="console-table template-table console-responsive-table"><thead><tr><th>{{ t('platform.copy.c772d98c') }}</th><th>{{ t('platform.copy.4eca7621') }}</th></tr></thead><tbody>
   <tr v-for="item in templates" :key="item.key"><td :data-label="String(t('platform.copy.c772d98c'))"><button class="text-button row-action" @click="open(item)">{{ label(item.key) }}</button></td><td :data-label="String(t('platform.copy.4eca7621'))">{{ formatPlatformDateTime(item.updated_at, displaySettings, locale) }}</td></tr>
   <tr v-if="!loading && !error && !templates.length"><td colspan="2" class="empty-state">{{ t('platform.copy.bf688a4b') }}</td></tr>
  </tbody></table></div>
 </ManagementList>
 <DetailModal v-if="selected && draft" :title="label(selected.key)" :close-label="t('platform.copy.27b2fd29')" :close-disabled="writing" @close="close">
  <div class="console-stack" data-testid="platform-template-detail">
   <p class="template-scope">{{ t('templates.scope') }}</p>
   <form v-write-guard="writing" v-inline-validation novalidate class="brand-form ui-form" @submit.prevent="withGlobalLoading(save)">
    <fieldset class="template-languages" :disabled="!canManage || writing || conflict">
     <div v-for="language in (['zh-CN', 'en'] as const)" :key="language" class="console-stack template-language">
      <h3>{{ t(language === 'en' ? 'platform.copy.d2332795' : 'platform.copy.01f1aac2') }}</h3>
      <label>{{ t('platform.copy.1e908e34') }}<input v-model="draft[language].title" maxlength="240" required /></label>
      <label>{{ t('platform.copy.29314db9') }}<textarea v-model="draft[language].body" rows="6" maxlength="2400" required /></label>
     </div>
    </fieldset>
    <p class="template-scope">{{ t('templates.placeholders') }}</p>
    <p v-if="conflict" role="status">{{t('settings.reloadBeforeSave')}}</p>
    <div class="form-actions ui-form-actions">
     <button v-if="canManage && !conflict" type="submit" class="button button-primary" :disabled="writing || !valid">{{ t('common.saveSettings') }}</button>
     <button v-if="conflict" v-refresh-button="t('templates.reload')" type="button" class="button button-secondary" :disabled="loading || writing" @click="withGlobalLoading(reloadSelected)">{{ t('templates.reload') }}</button>
    </div>
   </form>

  </div>
 </DetailModal>
</template>
<style scoped>
.template-scope{margin:0;color:var(--muted);overflow-wrap:anywhere}.template-languages{border:0;margin:0;padding:0;display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px;min-width:0}.template-language{min-width:0}.template-language h3{margin:0}.template-language input,.template-language textarea{width:100%;min-width:0;box-sizing:border-box;resize:vertical}.template-table{table-layout:fixed;min-width:0;white-space:normal}.template-table th,.template-table td{white-space:normal;overflow-wrap:anywhere}.template-table th:first-child{width:55%}.template-table .row-action{white-space:normal;text-align:left}@media(max-width:700px){.template-languages{grid-template-columns:minmax(0,1fr)}}
</style>
