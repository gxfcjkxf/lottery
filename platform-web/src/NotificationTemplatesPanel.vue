<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useOperationToast, vWriteGuard, vInlineValidation, vRefreshButton, withGlobalLoading } from '@lottery/shared'
import ManagementList from '@lottery/shared/ManagementList.vue'
import { createPlatformNotificationsApi, validTemplateContent, type Template, type TemplateContent, type TemplateWrite } from './notifications-api'
import { PlatformApiError } from './platform-api'
import { formatPlatformDateTime, type PlatformDisplaySettings } from './platform-format'
import { usePlatformI18n } from './i18n'
const props = defineProps<{ accountId: string; canManage: boolean; canView: boolean; locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings }>()
const pendingWrite = defineModel<TemplateWrite | null>('pendingWrite', { default: null })
const pending = computed(() => pendingWrite.value?.accountId === props.accountId ? pendingWrite.value : null)
const editorHeading = ref<HTMLHeadingElement | null>(null)
const emit = defineEmits<{ failure: [cause: unknown] }>()
const { t, errorMessage, displayError, message } = usePlatformI18n()
const api = createPlatformNotificationsApi()
const templates = ref<Template[]>([]), selected = ref<Template | null>(null)
const draft = ref<TemplateContent | null>(null)
const loading = ref(false), writing = ref(false), conflict = ref(false)
const error = ref<ReturnType<typeof errorMessage> | ''>(''), notice = ref<ReturnType<typeof message> | ''>('')
let alive = true
let generation = 0
let readTicket = 0
const label = (key: Template['key']) => t(`templates.name.${key}`)
const valid = computed(() => selected.value && draft.value && validTemplateContent(draft.value, selected.value.key))
const clone = (content: TemplateContent): TemplateContent => ({ en: { ...content.en }, 'zh-CN': { ...content['zh-CN'] } })
async function load() {
 const token = generation, ticket = ++readTicket
 if (!props.canView) {
  loading.value = false; templates.value = []
  error.value = errorMessage(new PlatformApiError('', 403, 'PERMISSION_DENIED'))
  return null
 }
 const current = () => alive && token === generation && ticket === readTicket
 loading.value = true; error.value = ''
 try {
  const rows = await api.templates()
  if (!current()) return null
  templates.value = rows
  const request = pending.value
  const item = rows.find(row => row.key === request?.event)
  if (!selected.value && request && item) {
   selected.value = item
   draft.value = clone(request.body.content)
  }
  return rows
 } catch (cause) {
  if (current()) { error.value = errorMessage(cause); emit('failure', cause) }
  return null
 } finally { if (current()) loading.value = false }
}
async function selectTemplate(item: Template) {
 if (loading.value || writing.value || pending.value) return
 if (selected.value?.key !== item.key) {
  selected.value = item
  draft.value = clone(item.content)
  conflict.value = false
 }
 const token = generation
 await nextTick()
 if (token !== generation || selected.value?.key !== item.key) return
 editorHeading.value?.focus({ preventScroll: true })
 editorHeading.value?.scrollIntoView({ block: 'start' })
}
async function save() {
 if (!selected.value || !draft.value || !props.canManage || !props.canView || writing.value || loading.value || conflict.value) return
 let request = pending.value
 if (!request) {
  if (!valid.value) return
  request = { accountId: props.accountId, event: selected.value.key, body: { version: selected.value.version, content: clone(draft.value), reason: '' }, key: crypto.randomUUID() }
  pendingWrite.value = request
 }
 const token = generation
 writing.value = true; error.value = ''; notice.value = ''
 try {
  const result = await api.update(request.event, request.body, request.key)
  if (!alive || token !== generation) return
  pendingWrite.value = null
  selected.value = result; draft.value = clone(result.content)
  templates.value = templates.value.map(item => item.key === result.key ? result : item)
  notice.value = message('templates.saved')
 } catch (cause) {
  if (alive && token === generation) {
   if (cause instanceof PlatformApiError && cause.status >= 400 && cause.status < 500 && cause.code !== 'INVALID_RESPONSE') {
    pendingWrite.value = null
    conflict.value = cause.status === 409
   }
   error.value = errorMessage(cause); emit('failure', cause)
  }
 } finally { if (alive && token === generation) writing.value = false }
}
async function reloadSelected() {
 const event = selected.value?.key
 const rows = await load()
 if (!rows || selected.value?.key !== event) return
 const item = rows.find(row => row.key === event)
 if (item) { selected.value = item; conflict.value = false }
}
watch(() => [props.accountId, props.canManage, props.canView], () => {
 generation++; templates.value = []; selected.value = null; draft.value = null
 writing.value = conflict.value = false; error.value = notice.value = ''
 void withGlobalLoading(load)
}, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { alive = false; generation++ })
useOperationToast(error, value => displayError(value), 'error')
useOperationToast(notice, value => displayError(value), 'success')
</script>
<template>
 <div class="console-stack">
  <section v-if="selected && draft" id="notification-template-editor" class="brand-settings-panel" data-testid="platform-template-detail" aria-labelledby="notification-template-heading">
   <h2 id="notification-template-heading" ref="editorHeading" tabindex="-1">{{ label(selected.key) }}</h2>
   <form v-write-guard="writing" v-inline-validation novalidate class="ui-form" @submit.prevent="withGlobalLoading(save)">
    <fieldset class="template-languages" :disabled="!canManage || writing || conflict || !!pending">
     <div v-for="language in (['zh-CN', 'en'] as const)" :key="language" class="console-stack template-language">
      <h3>{{ t(language === 'en' ? 'platform.copy.d2332795' : 'platform.copy.01f1aac2') }}</h3>
      <label>{{ t('platform.copy.1e908e34') }}<input v-model="draft[language].title" maxlength="240" required /></label>
      <label>{{ t('platform.copy.29314db9') }}<textarea v-model="draft[language].body" rows="4" maxlength="2400" required /></label>
     </div>
    </fieldset>
    <p v-if="canManage" class="template-scope">{{ t('templates.placeholders') }}</p>
    <p v-if="pending && !writing" class="template-scope" role="status">{{ t('templates.unknown') }}</p>
    <p v-if="conflict" class="template-scope" role="status">{{ t('settings.reloadBeforeSave') }}</p>
    <div v-if="canManage" class="ui-form-actions">
     <button v-if="conflict" type="button" class="button button-secondary" :disabled="loading || writing" @click="withGlobalLoading(reloadSelected)">{{ t('templates.reload') }}</button>
     <button v-else type="submit" class="button button-primary" :disabled="writing || loading || (!pending && !valid)">{{ t(writing ? 'common.saving' : pending ? 'templates.retry' : 'common.saveSettings') }}</button>
    </div>
   </form>
  </section>
  <ManagementList :busy="loading" data-testid="platform-template-list">
   <template v-if="error && !selected" #actions><button v-refresh-button="t('common.refresh')" class="button button-secondary" :disabled="loading" @click="withGlobalLoading(load)">{{ t('common.refresh') }}</button></template>
   <div class="table-wrap"><table class="console-table template-table console-responsive-table"><thead><tr><th>{{ t('platform.copy.c772d98c') }}</th><th>{{ t('platform.copy.4eca7621') }}</th></tr></thead><tbody>
    <tr v-for="item in templates" :key="item.key"><td :data-label="String(t('platform.copy.c772d98c'))"><button class="text-button row-action" type="button" :aria-pressed="selected?.key === item.key" :aria-controls="selected?.key === item.key ? 'notification-template-editor' : undefined" :disabled="loading || writing || !!pending" @click="selectTemplate(item)">{{ label(item.key) }}</button></td><td :data-label="String(t('platform.copy.4eca7621'))">{{ formatPlatformDateTime(item.updated_at, displaySettings, locale) }}</td></tr>
    <tr v-if="!loading && !error && !templates.length"><td colspan="2" class="empty-state">{{ t('platform.copy.bf688a4b') }}</td></tr>
   </tbody></table></div>
  </ManagementList>
 </div>
</template>
<style scoped>
#notification-template-heading { margin: 0; font-size: var(--body-font-size); scroll-margin-block: 64px; }
.template-scope { margin: 0; color: var(--muted); overflow-wrap: anywhere; }
.template-languages { grid-template-columns: repeat(2, minmax(0, 1fr)); }
.template-language { display: grid; gap: 12px; min-width: 0; }
.template-language h3 { margin: 0; font-size: var(--body-font-size); }
.template-language input, .template-language textarea { width: 100%; min-width: 0; }
.template-table { table-layout: fixed; min-width: 0; white-space: normal; }
.template-table th, .template-table td { white-space: normal; overflow-wrap: anywhere; }
.template-table th:first-child { width: 55%; }
.template-table .row-action { white-space: normal; text-align: left; }
.template-table .row-action[aria-pressed="true"] { color: var(--primary); font-weight: 700; }
@media (max-width: 700px) { .template-languages { grid-template-columns: minmax(0, 1fr); } }
</style>
