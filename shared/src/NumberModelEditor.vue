<script setup lang="ts">
import SelectControl from './SelectControl.vue'
import { formValidationText } from './form-validation'
import { computed, ref, watch } from 'vue'
import type { RuleModel, RulePool } from './rules'
import { createCatalogI18n } from './localization'
import { platformLotteryCatalog } from './platform-lottery'
const props = defineProps<{ modelValue: RuleModel; locale: 'en' | 'zh-CN'; disabled?: boolean; readOnly?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [model: RuleModel] }>()
const i18n = createCatalogI18n(platformLotteryCatalog, props.locale)
watch(() => props.locale, locale => i18n.setLocale(locale))
const t = i18n.t
const drafts = ref<Record<string, string>>({})
const pools = computed(() => props.modelValue.model === 'M_SELECT_N' ? ['regular_pool'] as const : ['regular_pool', 'special_pool'] as const)
const modelFields = computed(() => {
  const model = props.modelValue
  const fields = [{ label: t('type'), value: t(model.model === 'DIGITS_0_9' ? 'modelDigits' : model.model === 'X_PLUS_Y' ? 'modelXY' : 'modelMN') }]
  if (model.model === 'DIGITS_0_9') {
    fields.push({ label: t('digits'), value: String(model.length) }, { label: t('repeat'), value: t(model.allow_repeat ? 'modelYes' : 'modelNo') })
    return fields
  }
  fields.push({ label: t('regularCount'), value: String(model.regular_count) }, { label: t('specialCount'), value: String(model.special_count) })
  for (const pool of pools.value) {
    const label = t(model.model === 'M_SELECT_N' ? 'numberPool' : pool === 'regular_pool' ? 'regularPool' : 'specialPool')
    fields.push({ label, value: model[pool].values ? model[pool].values.join(', ') : `${model[pool].min}–${model[pool].max}` })
    if (model.model === 'X_PLUS_Y') fields.push({ label: `${label} · ${t('repeat')}`, value: t(model[pool].allow_repeat ? 'modelYes' : 'modelNo') })
  }
  fields.push({ label: t('ordered'), value: t(model.ordered ? 'modelYes' : 'modelNo') })
  return fields
})
function update(change: (model: RuleModel) => void) {
  if (props.disabled) return
  const next = JSON.parse(JSON.stringify(props.modelValue)) as RuleModel
  change(next)
  if (next.model === 'M_SELECT_N') {
    next.special_pool = { ...next.regular_pool, values: next.regular_pool.values?.slice(), allow_repeat: false }
    next.pool_size = next.regular_pool.values?.length || (next.regular_pool.max! - (next.regular_pool.min ?? 0) + 1)
    next.total_count = next.regular_count + next.special_count
  }
  emit('update:modelValue', next)
}
function changeType(type: string) {
  drafts.value = {}
  update(model => Object.assign(model, {
    model: type, regular_pool: type === 'DIGITS_0_9' ? { allow_repeat: false } : { min: 1, max: 49, allow_repeat: false },
    special_pool: type === 'DIGITS_0_9' ? { allow_repeat: false } : { min: 1, max: 49, allow_repeat: false },
    regular_count: type === 'DIGITS_0_9' ? 0 : 6, special_count: type === 'DIGITS_0_9' ? 0 : 1,
    length: type === 'DIGITS_0_9' ? 3 : 0, pool_size: 0, total_count: 0, allow_repeat: type === 'DIGITS_0_9', ordered: true,
  }))
}
function number(field: 'length' | 'regular_count' | 'special_count', event: Event) { update(model => { model[field] = Number((event.target as HTMLInputElement).value) }) }
function poolNumber(pool: 'regular_pool' | 'special_pool', field: 'min' | 'max', event: Event) { update(model => { model[pool][field] = Number((event.target as HTMLInputElement).value) }) }
function mode(pool: 'regular_pool' | 'special_pool', kind: string) { delete drafts.value[pool]; update(model => { model[pool] = kind === 'list' ? { values: [1], allow_repeat: false } : { min: 1, max: 49, allow_repeat: false } }) }
function values(pool: 'regular_pool' | 'special_pool', event: Event) { const raw = (event.target as HTMLInputElement).value; drafts.value[pool] = raw; if (/^[0-9]+(\s*,\s*[0-9]+)*$/.test(raw)) update(model => { model[pool].values = raw.split(',').map(value => Number(value.trim())) }) }
const listMode = (pool: RulePool) => !!pool.values?.length
</script>

<template>
  <div v-if="readOnly" class="table-wrap number-model-table-wrap">
    <table class="console-table console-responsive-table number-model-table">
      <thead><tr><th v-for="field in modelFields" :key="field.label" scope="col">{{ field.label }}</th></tr></thead>
      <tbody><tr><td v-for="field in modelFields" :key="field.label" :data-label="String(field.label)">{{ field.value }}</td></tr></tbody>
    </table>
  </div>
  <fieldset v-else class="number-model-editor" :disabled="disabled">
    <label>{{ t('type') }}<SelectControl :model-value="modelValue.model" @change="changeType($event)" :label="String(t('type'))" :options="[{ value: 'DIGITS_0_9', label: (String(t('modelDigits'))).trim() }, { value: 'X_PLUS_Y', label: (String(t('modelXY'))).trim() }, { value: 'M_SELECT_N', label: (String(t('modelMN'))).trim() }]" /></label>
    <template v-if="modelValue.model === 'DIGITS_0_9'">
      <label>{{ t('digits') }}<input type="number" min="1" max="10" :value="modelValue.length" required @input="number('length', $event)" /></label>
      <label class="model-checkbox"><input type="checkbox" :checked="modelValue.allow_repeat" @change="update(model => { model.allow_repeat = ($event.target as HTMLInputElement).checked })" />{{ t('repeat') }}</label>
    </template>
    <template v-else>
      <label>{{ t('regularCount') }}<input type="number" min="0" max="100" :value="modelValue.regular_count" required @input="number('regular_count', $event)" /></label>
      <label>{{ t('specialCount') }}<input type="number" min="0" max="100" :value="modelValue.special_count" required @input="number('special_count', $event)" /></label>
      <section v-for="pool in pools" :key="pool" class="model-pool">
        <h3>{{ modelValue.model === 'M_SELECT_N' ? t('numberPool') : pool === 'regular_pool' ? t('regularPool') : t('specialPool') }}</h3>
        <label>{{ t('poolMode') }}<SelectControl :model-value="listMode(modelValue[pool]) ? 'list' : 'range'" @change="mode(pool, $event)" :label="String(t('poolMode'))" :options="[{ value: 'range', label: (String(t('range'))).trim() }, { value: 'list', label: (String(t('numberList'))).trim() }]" /></label>
        <label v-if="listMode(modelValue[pool])">{{ t('numberList') }}<input class="model-number-list" :value="drafts[pool] ?? modelValue[pool].values!.join(', ')" pattern="[0-9]+(\s*,\s*[0-9]+)*" :data-validation-format="formValidationText('numberList')" required @input="values(pool, $event)" /></label>
        <template v-else><label>{{ t('minimum') }}<input type="number" min="0" max="1000000" :value="modelValue[pool].min ?? 0" required @input="poolNumber(pool, 'min', $event)" /></label><label>{{ t('maximum') }}<input type="number" min="0" max="1000000" :value="modelValue[pool].max ?? 0" required @input="poolNumber(pool, 'max', $event)" /></label></template>
        <label v-if="modelValue.model === 'X_PLUS_Y'" class="model-checkbox"><input type="checkbox" :checked="modelValue[pool].allow_repeat" @change="update(model => { model[pool].allow_repeat = ($event.target as HTMLInputElement).checked })" />{{ t('repeat') }}</label>
      </section>
    </template>
    <label v-if="modelValue.model !== 'DIGITS_0_9'" class="model-checkbox"><input type="checkbox" :checked="modelValue.ordered" @change="update(model => { model.ordered = ($event.target as HTMLInputElement).checked })" />{{ t('ordered') }}</label>
  </fieldset>
</template>

<style scoped>
.number-model-table-wrap{min-width:0;margin-top:12px}
.number-model-table{width:100%}
.management-console table.number-model-table{white-space:normal}
.number-model-table :is(th,td){white-space:normal;overflow-wrap:anywhere}

.number-model-editor { border: 0; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 240px), 1fr)); gap: 12px 20px; min-width: 0; }
.number-model-editor label { display: grid; grid-template-columns: minmax(0, 1fr); gap: 6px; font-size: var(--body-font-size); min-width: 0; }
.number-model-editor input { min-width: 0; max-width: 100%; box-sizing: border-box; padding: 8px; font: inherit; border: 1px solid var(--line); border-radius: var(--radius-sm); color: var(--ink); background: var(--surface); }
.number-model-editor input { width: 100px; }
.number-model-editor .model-number-list { width: 400px; }
.number-model-editor :disabled { opacity: 1; color: var(--ink); cursor: default; }
.number-model-editor .model-checkbox { display: flex; flex-direction: row; justify-content: flex-start; align-items: center; gap: 8px; }
.model-checkbox input { width: auto; }
.model-pool { grid-column: 1 / -1; display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 220px), 1fr)); gap: 12px; border-top: 1px solid var(--line); padding-top: 12px; }
.model-pool h3 { grid-column: 1 / -1; font-size: 16px; margin: 0; }
</style>
