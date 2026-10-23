<script setup lang="ts">
import { createCatalogI18n } from './localization'
import { configuredRuleCatalog } from './configured-rule-catalog'
import type { RuleNode } from './configured-rules'
import { computed, ref, watch } from 'vue'
const props = defineProps<{ modelValue: RuleNode; locale: 'en' | 'zh-CN'; field?: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: RuleNode] }>()
const i18n = createCatalogI18n(configuredRuleCatalog, props.locale)
watch(() => props.locale, value => i18n.setLocale(value))
const t = i18n.t
const openKeys = ref(new Set<string>())
const label = (key: string) => key in configuredRuleCatalog.en ? t(key) : key // Operator-defined attribute and price identifiers are configuration data.
const choices: Record<string, string[]> = {
  op: ['constant','selected_count','selected_value','selected_contains','selected_tag','numbers','raw_value','match_count','covered_option_count','attribute_values','weighted_group_sum','sum','distinct_count','modulo','quotient','digit_sum','max_tie_count','selected_is_unique_max','all','any','not','equals','greater_than','at_least','at_most','in','between'],
  kind: ['numbers','attribute','all','transform','fixed','option','cases','fixed_return','net_profit_per_hit'],
  mode: ['singles','combinations','as_selected'], counting: ['occurrences','distinct_numbers'],
  target: ['all','regular','special'], transform: ['modulo','quotient'],
  stake_mode: ['per_line','unit_multiplier'], settlement: ['tiers','partition'],
  rounding: ['half_up'], rounding_scope: ['line','tier','order'],
  mixed_tier_policy: ['','max_all','max_exclusive_plus_additive'], funding: ['','reserve'],
}
const enumChoices = computed(() => props.field ? choices[props.field] : undefined)
function set(key: string | number, value: RuleNode) {
  const next = JSON.parse(JSON.stringify(props.modelValue)) as RuleNode
  if (Array.isArray(next)) next[Number(key)] = value
  else if (next && typeof next === 'object') next[String(key)] = value
  emit('update:modelValue', next)
}
function remove(index: number) { const next = [...props.modelValue as RuleNode[]]; next.splice(index, 1); emit('update:modelValue', next) }
function add() { const next = [...props.modelValue as RuleNode[]]; if (next.length) next.push(JSON.parse(JSON.stringify(next[next.length - 1]!)) as RuleNode); emit('update:modelValue', next) }
</script>
<template>
  <div class="configured-node">
    <template v-if="Array.isArray(modelValue)">
      <div v-for="(item, index) in modelValue" :key="index" class="configured-node-item">
        <ConfiguredNodeEditor :model-value="item" :locale="locale" :field="field" @update:model-value="set(index, $event)" />
        <button type="button" class="button button-secondary" @click="remove(index)">{{ t('remove') }}</button>
      </div>
      <button v-if="modelValue.length" type="button" class="button button-secondary" @click="add">{{ t('add') }}</button>
    </template>
    <template v-else-if="modelValue !== null && typeof modelValue === 'object'">
      <div v-for="(value, key) in modelValue" v-show="!['schema_version', 'protocol', 'revision', 'rule_id', 'version', 'snapshot_hash', 'actor_context'].includes(String(key))" :key="key">
        <details v-if="value !== null && typeof value === 'object'" @toggle="($event.target as HTMLDetailsElement).open ? openKeys.add(String(key)) : openKeys.delete(String(key))"><summary>{{ label(String(key)) }}</summary><ConfiguredNodeEditor v-if="openKeys.has(String(key))" :model-value="value" :locale="locale" :field="String(key)" @update:model-value="set(key, $event)" /></details>
        <label v-else>{{ label(String(key)) }}<ConfiguredNodeEditor :model-value="value" :locale="locale" :field="String(key)" @update:model-value="set(key, $event)" /></label>
      </div>
    </template>
    <select v-else-if="typeof modelValue === 'boolean'" :value="String(modelValue)" @change="emit('update:modelValue', ($event.target as HTMLSelectElement).value === 'true')"><option value="true">{{ t('true') }}</option><option value="false">{{ t('false') }}</option></select>
    <input v-else-if="typeof modelValue === 'number'" type="number" step="1" :value="field === 'positions' ? modelValue + 1 : modelValue" @input="emit('update:modelValue', ($event.target as HTMLInputElement).valueAsNumber - (field === 'positions' ? 1 : 0))" />
    <select v-else-if="typeof modelValue === 'string' && enumChoices" :value="modelValue" @change="emit('update:modelValue', ($event.target as HTMLSelectElement).value)"><option v-if="!enumChoices.includes(modelValue)" :value="modelValue" disabled>{{ t('undefinedChoice') }}</option><option v-for="value in enumChoices" :key="value" :value="value">{{ value ? t(value) : t('null') }}</option></select>
    <input v-else-if="typeof modelValue === 'string'" :value="modelValue" @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)" />
    <span v-else>{{ t('null') }}</span>
  </div>
</template>
<style scoped>
.configured-node{display:grid;gap:8px;min-width:0}.configured-node input,.configured-node select{width:100%;min-width:0;min-height:44px;box-sizing:border-box}.configured-node label{display:grid;gap:4px;font-size:14px}.configured-node details{border-left:2px solid var(--line);padding-left:12px;min-width:0}.configured-node summary{cursor:pointer;min-height:44px;overflow-wrap:anywhere;font-size:14px}.configured-node-item{padding:8px;border:1px solid var(--line);border-radius:var(--radius-sm);min-width:0}
</style>
