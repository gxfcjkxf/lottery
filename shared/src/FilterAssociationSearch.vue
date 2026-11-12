<script setup lang="ts">
import {withGlobalLoading} from './global-loading'
import SelectControl from './SelectControl.vue'
import { onBeforeUnmount, ref, watch } from 'vue'
import { readFilterOptions, type FilterOption } from './filter-options'
import { createCatalogI18n } from './localization'
const props = withDefaults(defineProps<{ modelValue: string; brandId: string; surface: 'admin' | 'platform'; resource: string; label: string; locale: 'en' | 'zh-CN'; enabled?: boolean; disabled?: boolean; scopeKey?: string; kind?: 'member' | 'agent' | 'actor'; resetKey?: number; action?: 'view' | 'grant'; inputTestId?: string; selectedName?: string }>(), { enabled: true, kind: 'member' })
const emit = defineEmits<{ 'update:modelValue': [value: string]; 'update:pending': [value: boolean]; select: [item: FilterOption] }>()
const i18n = createCatalogI18n({ en: { retry: 'Retry', failed: 'Unable to search. Please retry.', more: 'More results', choose: 'Select an account from the results' }, 'zh-CN': { retry: '重试', failed: '搜索失败，请重试。', more: '更多结果', choose: '请从搜索结果选择账号' } }, props.locale)
const t = i18n.t
watch(() => props.locale, value => i18n.setLocale(value))
const query = ref(props.modelValue ? props.selectedName ?? '' : ''), items = ref<FilterOption[]>([]), busy = ref(false), failed = ref(false), more = ref(false), selected = ref(Boolean(props.modelValue))
let generation = 0, controller: AbortController | undefined
let searchTimer: ReturnType<typeof setTimeout> | undefined
function reset() {
  clearTimeout(searchTimer)
  generation++; controller?.abort(); query.value = ''; items.value = []; busy.value = failed.value = more.value = selected.value = false
  emit('update:modelValue', ''); emit('update:pending', false)
}
function edit(value: string, event: Event) {
  query.value = value
  clearTimeout(searchTimer)
  generation++; controller?.abort(); items.value = []; busy.value = failed.value = more.value = selected.value = false
  const composing = event instanceof InputEvent && event.isComposing
  emit('update:modelValue', ''); emit('update:pending', composing || Boolean(query.value.trim()))
  if (!composing && Array.from(query.value.trim()).length >= 2) searchTimer = setTimeout(() => { void search() }, 300)
}
async function search(offset = 0) {
  clearTimeout(searchTimer)
  if (Array.from(query.value.trim()).length < 2 || props.disabled || props.enabled === false) return
  controller?.abort(); controller = new AbortController(); const ticket = ++generation
  busy.value = true; failed.value = false
  try {
    const page = await readFilterOptions({ ...props, kind: props.kind, search: query.value.trim(), signal: controller.signal }, offset)
    if (ticket !== generation) return
    items.value = offset ? [...items.value, ...page.items] : page.items; more.value = page.items.length === 100
  } catch { if (ticket === generation) failed.value = true }
  finally { if (ticket === generation) busy.value = false }
}
function choose(item: FilterOption) {
  clearTimeout(searchTimer)
  generation++; controller?.abort(); busy.value = failed.value = false
  query.value = item.name; selected.value = true; items.value = [item]; more.value = false
  emit('select', item); emit('update:modelValue', item.id); emit('update:pending', false)
}
watch(() => [props.brandId, props.surface, props.resource, props.kind, props.scopeKey, props.enabled, props.action], reset)
watch(() => props.resetKey, reset)
watch(() => props.modelValue, value => { if (!value && selected.value) reset() })
onBeforeUnmount(() => { clearTimeout(searchTimer); generation++; controller?.abort() })
</script>
<template>
  <span class="association-search" :aria-busy="busy">
    <SelectControl :data-testid="inputTestId" :model-value="modelValue" :search="{value:query,locale}" :label="label" :placeholder="label" :disabled="disabled || enabled === false || !brandId" :options="items.map(item => ({value:item.id,label:item.name}))" @search="edit" @change="value => choose(items.find(item => item.id === value)!)">
      <template #footer>
        <span v-if="failed" role="alert">{{ t('failed') }} <button type="button" class="button button-secondary" :disabled="busy || disabled || enabled === false" @click="withGlobalLoading(search)">{{ t('retry') }}</button></span>
        <button v-if="more" type="button" class="button button-secondary" :disabled="busy" @click="withGlobalLoading(search,items.length)">{{ t('more') }}</button>
      </template>
    </SelectControl>
  </span>
</template>
<style scoped>
.association-search{display:inline-flex;min-width:0;max-width:100%;--input-width:168px}
</style>
