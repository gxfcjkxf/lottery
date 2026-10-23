<script setup lang="ts">
import { computed } from 'vue'
import { businessDetailsCatalog } from './business-details-catalog'
import { formatDisplayDateTime } from './display-settings'
const props = defineProps<{ value: unknown; locale: 'en' | 'zh-CN'; field?: string }>()
const messages = computed(() => businessDetailsCatalog[props.locale])
const t = (key: string) => (messages.value as Readonly<Record<string, string>>)[key]
const object = computed(() => props.value !== null && typeof props.value === 'object' && !Array.isArray(props.value))
// Only business fields have a display label. Internal identifiers, credentials and traces are never rendered.
const entries = computed(() => object.value ? Object.entries(props.value as Record<string, unknown>).filter(([key, value]) => value !== undefined && t('field.' + key)) : [])
const list = computed(() => Array.isArray(props.value) ? props.value : [])
function text(value: unknown): string {
  if (value === null || value === undefined || value === '') return '—'
  if (typeof value === 'boolean') return t(value ? 'yes' : 'no')
  if (typeof value === 'number' || typeof value === 'bigint') return String(value)
  if (typeof value !== 'string') return t('unknown')
  if (['ratio', 'loss_ratio', 'turnover_ratio', 'loss_ratio_cap', 'turnover_ratio_cap'].includes(props.field ?? '') && /^\d+(?:\.\d+)?$/.test(value)) return new Intl.NumberFormat(props.locale, { maximumFractionDigits: 6 }).format(Number(value) * 100) + '%'
  if (t('value.' + value)) return t('value.' + value)
  if (/^\d{4}-\d{2}-\d{2}T/.test(value)) return formatDisplayDateTime(value, props.locale)
  if (/^[0-9a-f]{8}-[0-9a-f-]{27,}$/i.test(value) || /^[0-9a-f]{32,}$/i.test(value) || /[{}]|^[A-Z][A-Z_]{3,}$/.test(value) || value === '[REDACTED]') return t('unknown')
  return value
}
</script>
<template>
  <dl v-if="object && entries.length" class="business-details">
    <div v-for="[key, child] in entries" :key="key" class="business-details-row">
      <dt>{{ t('field.' + key) }}</dt><dd><BusinessDetails :value="child" :locale="locale" :field="key" /></dd>
    </div>
  </dl>
  <p v-else-if="object || (Array.isArray(value) && !list.length)" class="business-details-empty empty-state">{{ t('empty') }}</p>
  <ol v-else-if="Array.isArray(value)" class="business-details-list"><li v-for="(child, index) in list" :key="index"><BusinessDetails :value="child" :locale="locale" /></li></ol>
  <span v-else>{{ text(value) }}</span>
</template>
<style scoped>
.business-details{display:grid;gap:8px;margin:0;min-width:0;font-size:13px;line-height:1.6}.business-details-row{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,2fr);gap:12px;border-bottom:1px solid var(--line);padding:8px 0;min-width:0}.business-details dt{color:var(--muted);overflow-wrap:anywhere}.business-details dd{margin:0;min-width:0;overflow-wrap:anywhere}.business-details-list{margin:0;padding-inline-start:22px;min-width:0;font-size:13px;line-height:1.6}.business-details-list>li+li{margin-top:8px}.business-details-empty{margin:0;color:var(--muted);font-size:13px;line-height:1.6}@media(max-width:480px){.business-details-row{grid-template-columns:minmax(0,1fr);gap:4px}}
</style>
