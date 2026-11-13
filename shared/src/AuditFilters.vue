<script setup lang="ts">
import { ref, watch } from 'vue'
import { withGlobalLoading } from './global-loading'
import { vInlineValidation } from './form-validation'
import ClearableInput from './ClearableInput.vue'
import SelectControl from './SelectControl.vue'
import PlatformTemporalInput from './PlatformTemporalInput.vue'
import { createCatalogI18n } from './localization'
import { filterDateBoundary } from './filter-dates'
const props = defineProps<{ locale: 'en' | 'zh-CN'; busy: boolean }>()
const emit = defineEmits<{ apply: [filters: Record<string, string>] }>()
const i18n = createCatalogI18n({
  en: { actor: 'Operator', type: 'Type', all: 'All types', view: 'View', edit: 'Edit', create: 'Create', delete: 'Delete', queryType: 'Query', from: 'Start date', to: 'End date', query: 'Search', clear: 'Clear', dates: 'Select both dates within a range of 31 days.' },
  'zh-CN': { actor: '操作人', type: '类型', all: '全部类型', view: '查看', edit: '编辑', create: '创建', delete: '删除', queryType: '查询', from: '开始日期', to: '结束日期', query: '查询', clear: '清空', dates: '请选择起止日期，范围不超过31天。' },
}, props.locale)
watch(() => props.locale, value => i18n.setLocale(value))
const t = i18n.t
const actor = ref(''), category = ref(''), from = ref(''), to = ref(''), error = ref(false)
function submit() {
  const filters: Record<string, string> = {}
  error.value = false
  if (from.value || to.value) {
    try {
      const start = filterDateBoundary(from.value, false, 'utc'), end = filterDateBoundary(to.value, true, 'utc')
      if (end.getTime() <= start.getTime() || end.getTime() - start.getTime() > 31 * 86400000) throw new Error()
      filters.from = start.toISOString(); filters.to = end.toISOString()
    } catch { error.value = true; return }
  }
  if (actor.value.trim()) filters.actor = actor.value.trim()
  if (category.value) filters.action_type = category.value
  emit('apply', filters)
}
function clear() { actor.value = category.value = from.value = to.value = ''; submit() }
</script>
<template>
  <form class="ui-filter-bar" novalidate v-inline-validation @submit.prevent="withGlobalLoading(submit)">
    <ClearableInput v-model="actor" :locale="locale" :aria-label="t('actor')" :placeholder="t('actor')" :maxlength="128" :disabled="busy" />
    <SelectControl v-model="category" :label="t('type')" :disabled="busy" :options="['', 'create', 'view', 'query', 'edit', 'delete'].map(value => ({ value, label: t(value === 'query' ? 'queryType' : value || 'all') }))" />
    <PlatformTemporalInput v-model="from" type="date" :locale="locale" :label="t('from')" :placeholder="t('from')" :disabled="busy" />
    <PlatformTemporalInput v-model="to" type="date" :locale="locale" :label="t('to')" :placeholder="t('to')" :disabled="busy" />
    <button class="button button-primary" type="submit" :disabled="busy">{{ t('query') }}</button>
    <button class="button button-secondary" type="button" :disabled="busy" @click="withGlobalLoading(clear)">{{ t('clear') }}</button>
    <span v-if="error" role="alert">{{ t('dates') }}</span>
  </form>
</template>
