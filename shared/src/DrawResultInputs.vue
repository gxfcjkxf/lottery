<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { createCatalogI18n } from './localization'
import { platformLotteryCatalog, type PlatformPeriod } from './platform-lottery'
import type { RuleModel, RulePool } from './rules'
const props = defineProps<{ model: RuleModel; locale: 'en' | 'zh-CN' }>()
const i18n = createCatalogI18n(platformLotteryCatalog, props.locale)
watch(() => props.locale, value => i18n.setLocale(value))
const t = i18n.t
const groups = computed(() => {
 const m = props.model
 if (m.model === 'DIGITS_0_9') return [{ key: 'digits' as const, count: m.length, pool: { min: 0, max: 9, allow_repeat: m.allow_repeat } }]
 if (!['X_PLUS_Y', 'M_SELECT_N'].includes(m.model)) throw new Error('Unsupported draw number model')
 return [{ key: 'regular' as const, count: m.regular_count, pool: m.regular_pool }, { key: 'special' as const, count: m.special_count, pool: m.special_pool }].filter(group => group.count > 0)
})
const values = ref<Record<string, string[]>>({})
watch(groups, value => { values.value = Object.fromEntries(value.map(group => [group.key, Array(group.count).fill('')])) }, { immediate: true })
const root = ref<HTMLElement | null>(null), touched = ref<Set<string>>(new Set())
function error(key: string, index: number, pool: RulePool) {
 const text = values.value[key]![index]!
 if (!/^\d{1,2}$/.test(text)) return t('numberRequired')
 const number = Number(text), min = pool.values?.length ? Math.min(...pool.values) : pool.min ?? 0, max = pool.values?.length ? Math.max(...pool.values) : pool.max ?? 0
 if (number < min || number > max) return t('numberRange', { min, max })
 if (pool.values?.length && !pool.values.includes(number)) return t('numberOutsidePool')
 const duplicates = (other: string, position: number) => other !== '' && Number(other) === number && position !== index
 if (!pool.allow_repeat && values.value[key]!.some(duplicates)) return t('numberDuplicate')
 const m = props.model
 const markSix = m.model === 'X_PLUS_Y' && m.regular_count === 6 && m.special_count === 1 && m.regular_pool.min === 1 && m.regular_pool.max === 49 && m.special_pool.min === 1 && m.special_pool.max === 49
 if ((m.model === 'M_SELECT_N' || markSix) && key !== 'digits' && values.value[key === 'regular' ? 'special' : 'regular']?.some(other => other !== '' && Number(other) === number)) return t('numberDuplicate')
 return ''
}
function check(key: string, index: number) { touched.value.add(`${key}:${index}`) }
async function input(event: Event, key: string, index: number, pool: RulePool) {
 const element = event.target as HTMLInputElement
 values.value[key]![index] = element.value
 if (element.value.length === (key === 'digits' ? 1 : 2)) {
  check(key, index)
  if (!error(key, index, pool)) {
   await nextTick()
   const inputs = [...root.value!.querySelectorAll('input')]
   inputs[inputs.indexOf(element) + 1]?.focus()
  }
 }
}
function result(): NonNullable<PlatformPeriod['result']> | null {
 for (const group of groups.value) for (let index = 0; index < group.count; index++) {
  check(group.key, index)
  if (error(group.key, index, group.pool)) {
   root.value?.querySelector<HTMLInputElement>(`input[data-number="${group.key}:${index}"]`)?.focus()
   return null
  }
 }
 return { regular: (values.value.regular ?? []).map(Number), special: (values.value.special ?? []).map(Number), digits: (values.value.digits ?? []).map(Number) }
}
defineExpose({ result })
</script>
<template>
 <div ref="root" class="draw-result-inputs" role="group" :aria-label="t('drawNumbers')">
  <span class="draw-result-label">{{ t('drawNumbers') }}</span>
  <div class="draw-result-cells">
   <template v-for="group in groups" :key="group.key">
    <span v-if="group.key === 'special'" class="draw-result-plus" aria-hidden="true">+</span>
    <input v-for="(_, index) in values[group.key]" :key="`${group.key}:${index}`" :value="values[group.key]![index]" :data-number="`${group.key}:${index}`" :aria-label="t('numberLabel', {group: t(group.key), index: index + 1})" :aria-invalid="touched.has(`${group.key}:${index}`) && !!error(group.key,index,group.pool)" :aria-describedby="touched.has(`${group.key}:${index}`) && error(group.key,index,group.pool) ? `${group.key}-error-${index}` : undefined" type="text" inputmode="numeric" autocomplete="off" :maxlength="group.key === 'digits' ? 1 : 2" required @input="input($event,group.key,index,group.pool)" @blur="check(group.key,index)" @focus="($event.target as HTMLInputElement).select()" />
   </template>
  </div>
  <div v-for="group in groups" :key="group.key" class="draw-result-errors">
   <template v-for="(_, index) in values[group.key]" :key="index"><p v-if="touched.has(`${group.key}:${index}`) && error(group.key,index,group.pool)" :id="`${group.key}-error-${index}`" class="message error" role="alert">{{ t('numberLabel', {group:t(group.key),index:index + 1}) }}: {{ error(group.key,index,group.pool) }}</p></template>
  </div>
 </div>
</template>
<style scoped>
.draw-result-inputs{display:flex;flex-wrap:wrap;align-items:center;gap:8px;grid-column:1/-1;min-width:0}
.draw-result-label{white-space:nowrap}
.draw-result-cells{display:flex;flex-wrap:wrap;align-items:center;gap:4px;min-width:0}
#app .draw-result-inputs .draw-result-cells input{box-sizing:border-box;flex:0 0 auto;width:calc(2ch + 10px);min-width:0;max-width:none;min-height:var(--control-height);padding:4px;text-align:center;font:inherit;font-variant-numeric:tabular-nums;border:1px solid var(--line);border-radius:var(--radius-sm);background:var(--surface);color:var(--ink)}
.draw-result-cells input:focus-visible{outline:2px solid var(--primary);outline-offset:2px}
#app .draw-result-inputs .draw-result-cells input[aria-invalid=true]{border-color:var(--danger)}
.draw-result-plus{color:var(--muted);line-height:1}
.draw-result-errors{flex-basis:100%;min-width:0}.draw-result-errors:empty{display:none}
.draw-result-errors .message{margin:0;font-size:var(--body-font-size)}
</style>
