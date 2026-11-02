<script setup lang="ts" generic="T extends { id: string }">
import { computed, ref, shallowRef, watch } from 'vue'
import DetailModal from './DetailModal.vue'
import { showOperationToast } from './operation-toast'
import { withGlobalLoading } from './global-loading'
import { vWriteGuard } from './write-guard'
import { createCatalogI18n } from './localization'
import { memberBatchDeleteCatalog } from './member-batch-delete-catalog'

const props = defineProps<{
  feedback?: 'toast'; subject: 'accounts' | 'brands' | 'roles'; entryLabel: (entry: T) => string; selectable?: (entry: T) => boolean; scopeKey: string; warning?: string; entries: T[]; enabled: boolean; locale: 'en' | 'zh-CN'; loading: boolean
  deleteEntry: (entry: T, key: string) => Promise<unknown>
  errorText: (cause: unknown) => string
  isUnknown: (cause: unknown) => boolean
}>()
const emit = defineEmits<{ deleted: [entry: T]; finished: []; locked: [value: boolean] }>()
const i18n = createCatalogI18n(memberBatchDeleteCatalog, props.locale), t = i18n.t
watch(() => props.locale, locale => i18n.setLocale(locale))
const selected = ref<string[]>([]), open = ref(false), busy = ref(false), attempted = ref(false)
type Item = { entry: T; key: string; state: 'pending' | 'deleted' | 'failed' | 'unknown'; cause?: unknown }
watch(open, value => emit('locked', value))
const items = shallowRef<Item[]>([])
const successCount = computed(() => items.value.filter(item => item.state === 'deleted').length)
const remainingCount = computed(() => items.value.length - successCount.value)
const unknown = computed(() => items.value.some(item => item.state === 'unknown'))
const eligible = computed(() => props.entries.filter(entry => !props.selectable || props.selectable(entry)))
const allSelected = computed(() => eligible.value.length > 0 && eligible.value.every(entry => selected.value.includes(entry.id)))
watch(() => props.scopeKey, () => { if (!open.value) selected.value = [] })
const selectLabel = (account: string) => t(props.subject === 'accounts' ? 'selectAccount' : 'selectItem', { account })
watch(() => props.entries, entries => {
  if (!open.value) { selected.value = []; return }
  selected.value = selected.value.filter(id => entries.some(entry => entry.id === id && (!props.selectable || props.selectable(entry))))
})
watch(() => props.enabled, enabled => { if (!enabled && !open.value) selected.value = [] })
function toggle(id: string) {
  if (!props.enabled || props.loading || open.value || !eligible.value.some(entry => entry.id === id)) return
  selected.value = selected.value.includes(id) ? selected.value.filter(value => value !== id) : [...selected.value, id]
}
function selectPage() {
  if (!props.enabled || props.loading || open.value) return
  selected.value = allSelected.value ? [] : eligible.value.map(entry => entry.id)
}
function begin() {
  if (!props.enabled || props.loading || !selected.value.length) return
  items.value = props.entries.filter(entry => selected.value.includes(entry.id)).map(entry => ({ entry: { ...entry }, key: crypto.randomUUID(), state: 'pending' }))
  attempted.value = false
  open.value = true
}
function close() {
  if (busy.value || unknown.value) return
  open.value = false
  if (attempted.value) emit('finished')
}
async function submit() {
  if (busy.value || !props.enabled) return
  busy.value = true
  attempted.value = true
  try {
    for (const item of items.value) {
      if (item.state === 'deleted') continue
      // Confirmed failures can be submitted again; uncertain results keep their key.
      if (item.state === 'failed') item.key = crypto.randomUUID()
      item.state = 'pending'
      item.cause = undefined
      items.value = [...items.value]
      try {
        await props.deleteEntry(item.entry, item.key)
        item.state = 'deleted'
        selected.value = selected.value.filter(id => id !== item.entry.id)
        emit('deleted', item.entry)
      } catch (cause) {
        item.cause = cause
        item.state = props.isUnknown(cause) ? 'unknown' : 'failed'
        if (props.feedback === 'toast') showOperationToast(() => props.errorText(cause), 'error')
      }
      items.value = [...items.value]
      if (item.state === 'unknown') break
    }
  } finally {
    busy.value = false
    if (props.feedback === 'toast') {
      const success = successCount.value, remaining = remainingCount.value
      showOperationToast(() => t('summary', { success, remaining }), remaining ? 'info' : 'success')
      if (remaining === 0) close()
    }
  }
}
</script>

<template>
  <div v-if="enabled && entries.length" class="member-batch-toolbar">
    <label class="member-select-control"><input type="checkbox" :checked="allSelected" :indeterminate="selected.length > 0 && !allSelected" :disabled="loading || open || !eligible.length" @change="selectPage" /><span>{{ t('selectPage') }}</span></label>
    <span role="status">{{ t('selected', { count: selected.length }) }}</span>
    <button type="button" class="button button-danger" :disabled="!selected.length || loading || open" @click="begin">{{ t('delete') }}</button>
  </div>
  <slot :selected="selected" :toggle="toggle" :select-label="selectLabel" :locked="loading || open" />
  <DetailModal v-if="open" class="confirmation-dialog member-batch-dialog" :title="t(subject === 'brands' ? 'titleBrands' : subject === 'roles' ? 'titleRoles' : 'title')" :close-label="t('close')" :close-disabled="busy || unknown" @close="close">
    <p v-if="warning">{{ warning }}</p>
    <p v-if="attempted && feedback !== 'toast'" role="status">{{ t('summary', { success: successCount, remaining: remainingCount }) }}</p>
    <p v-else-if="!attempted">{{ t('selected', { count: items.length }) }}</p>
    <ul class="member-batch-list">
      <li v-for="item in items" :key="item.entry.id"><div><strong>{{ entryLabel(item.entry) }}</strong><span>{{ t(item.state) }}</span></div><p v-if="feedback !== 'toast' && item.state === 'failed' && item.cause" class="message error" role="alert">{{ errorText(item.cause) }}</p></li>
    </ul>
    <p v-if="unknown" class="message" role="status">{{ t('uncertain') }}</p>
    <div v-if="remainingCount" class="form-actions ui-form-actions">
      <button v-write-guard="busy" type="button" class="button button-danger" :disabled="busy || !enabled" @click="withGlobalLoading(submit)">{{ busy ? t('busy') : attempted ? t('retry') : t(subject === 'brands' ? 'confirmBrands' : subject === 'roles' ? 'confirmRoles' : 'confirm', { count: items.length }) }}</button>
    </div>
  </DetailModal>
</template>
