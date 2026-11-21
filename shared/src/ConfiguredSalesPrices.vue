<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue'
import SelectControl from './SelectControl.vue'
import { configuredSelectionOptionLabel } from './configured-selection-groups'
import { createCatalogI18n } from './localization'
import { configuredRuleCatalog } from './configured-rule-catalog'
import { configuredAvailabilityDisabled, configuredGroupsForScope, configuredOptionsForScope, type ConfiguredAvailability } from './configured-business'
import { configuredOptionPriceKeys, type ConfiguredSnapshot } from './configured-rules'

const props = defineProps<{
  snapshot: ConfiguredSnapshot
  availability: ConfiguredAvailability
  modelValue: ConfiguredSnapshot['prices']
  locale: 'en' | 'zh-CN'
  canWrite: boolean
  locked: boolean
}>()
type Item = { play: string; option: string }
const emit = defineEmits<{ 'update:modelValue': [value: ConfiguredSnapshot['prices']]; 'set-items': [items: Item[], enabled: boolean] }>()
const groupId = defineModel<string>('groupId', { required: true })
const editorId = useId()
const i18n = createCatalogI18n(configuredRuleCatalog, props.locale)
watch(() => props.locale, value => i18n.setLocale(value))
const t = i18n.t
const batchOdds = ref('')
const plate = ref(props.snapshot.catalog.plates[0]!)
const groups = computed(() => configuredGroupsForScope(props.snapshot, props.availability))
const rows = computed(() => {
  const group = groups.value.find(item => item.id === groupId.value)
  if (!group) return []
  return group.plays.flatMap(play => configuredOptionsForScope(props.snapshot, play, props.availability).map(option => ({
    id: JSON.stringify([play.id, option.id]),
    play: play.id,
    option: option.id,
    label: configuredSelectionOptionLabel(play, option)[props.locale],
    disabled: configuredAvailabilityDisabled(props.availability, play.id, option.id),
    priceKeys: configuredOptionPriceKeys(props.snapshot, play, option),
  })))
})
const editableKeys = computed(() => [...new Set(rows.value.filter(row => !row.disabled).flatMap(row => row.priceKeys))])
const editDisabled = computed(() => !props.canWrite || props.locked)

function edit(keys: readonly string[], value: string) {
  if (editDisabled.value) return
  const prices = Object.fromEntries(keys.map(key => [key, value]))
  emit('update:modelValue', {
    ...props.modelValue,
    plates: { ...props.modelValue.plates, [plate.value]: { ...props.modelValue.plates[plate.value], ...prices } },
  })
}
function setItems(items: Item[], enabled: boolean) {
  if (editDisabled.value) return
  emit('set-items', items, enabled)
}
watch([plate, groupId], () => { batchOdds.value = '' })
</script>

<template>
  <fieldset class="configured-odds-editor" :disabled="locked">
    <p v-if="!groups.length" class="empty-state">{{ t('noAvailableOptions') }}</p>
    <template v-else>
      <div class="configured-odds-filters">
        <label>{{ t('plate') }}<SelectControl v-model="plate" :label="t('plate')" :options="snapshot.catalog.plates.map(value => ({ value, label: value }))" /></label>
        <label>{{ t('play') }}<SelectControl v-model="groupId" :label="t('play')" :options="groups.map(group => ({ value: group.id, label: group.label[locale] }))" /></label>
        <div v-if="canWrite" class="configured-odds-batch">
          <input v-model="batchOdds" inputmode="decimal" :aria-label="t('batchOdds')" :disabled="!editableKeys.length" />
          <button type="button" class="button button-secondary" :disabled="!batchOdds.trim() || !editableKeys.length" @click="edit(editableKeys, batchOdds)">{{ t('batchModify') }}</button>
        </div>
        <div v-if="canWrite" class="configured-odds-batch">
          <button type="button" class="button button-secondary" :disabled="!rows.some(row => row.disabled)" @click="setItems(rows, true)">{{ t('enablePlayItems') }}</button>
          <button type="button" class="button button-secondary" :disabled="!rows.some(row => !row.disabled)" @click="setItems(rows, false)">{{ t('disablePlayItems') }}</button>
        </div>
      </div>
      <div class="configured-odds-fields">
        <div v-for="(row, index) in rows" :key="row.id" class="configured-odds-item" :class="{ 'configured-odds-item-disabled': row.disabled, 'configured-odds-item-multiple': !row.disabled && row.priceKeys.length > 1 }">
          <span class="configured-odds-label" :id="`${editorId}-item-${index}`">{{ row.label }}</span>
          <div class="configured-odds-item-controls">
            <input v-if="row.disabled" inputmode="decimal" :aria-labelledby="`${editorId}-item-${index}`" :value="t('oddsUnavailable')" disabled />
            <template v-else>
              <input v-for="(key, priceIndex) in row.priceKeys" :key="key" inputmode="decimal"
                :aria-label="row.priceKeys.length === 1 ? row.label : t('oddsNumber', { name: row.label, index: priceIndex + 1 })"
                :disabled="editDisabled" :value="modelValue.plates[plate]?.[key]"
                @input="edit([key], ($event.target as HTMLInputElement).value)" />
            </template>
            <button v-if="canWrite" type="button" class="button button-secondary configured-odds-item-toggle"
              :aria-label="t(row.disabled ? 'enableOddsOption' : 'disableOddsOption', { name: row.label })"
              @click="setItems([row], row.disabled)">{{ t(row.disabled ? 'enableOddsShort' : 'disableOddsShort') }}</button>
          </div>
        </div>
      </div>
    </template>
  </fieldset>
</template>

<style scoped>
.configured-odds-editor{border:0;padding:0;margin:0;min-width:0}
:global(#app .configured-odds-editor input[inputmode="decimal"]){--input-width:calc(5ch + 14px);width:var(--input-width);box-sizing:border-box;padding-inline:6px}
.configured-odds-filters,.configured-odds-batch{display:flex;flex-wrap:wrap;align-items:center;gap:8px;max-width:100%}
.configured-odds-filters{gap:8px 12px;margin-bottom:12px}
.configured-odds-filters label{display:flex;flex-direction:row;align-items:center;gap:8px;max-width:100%}
.configured-odds-filters :deep(.select-control){min-width:0;max-width:100%}
.configured-odds-fields{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(196px,100%),1fr));gap:8px;min-width:0}
.configured-odds-fields .configured-odds-item{display:grid;grid-template-columns:minmax(0,1fr) auto;align-items:center;gap:6px;min-width:0;padding:6px 8px;border:1px solid color-mix(in srgb,var(--primary) 55%,var(--line));border-inline-start:3px solid var(--primary);border-radius:var(--radius-sm);background:color-mix(in srgb,var(--primary) 22%,var(--surface));color:var(--ink)}
.configured-odds-fields .configured-odds-item-disabled{border-color:var(--line);border-inline-start-color:var(--muted);background:var(--canvas);color:var(--muted)}
:global(#app .configured-odds-editor .configured-odds-item-disabled input){--input-width:calc(5ch + 22px);padding-inline:4px;font-size:12px!important;opacity:1;color:var(--muted)}
.configured-odds-fields .configured-odds-item-multiple{grid-template-columns:minmax(0,1fr)}
.configured-odds-label{font-weight:650;overflow-wrap:anywhere}
.configured-odds-item-controls{display:flex;justify-content:flex-end;align-items:center;flex-wrap:wrap;gap:4px;max-width:100%;min-width:0}
.configured-odds-item-controls>input{min-width:0}
.configured-odds-item-controls .configured-odds-item-toggle{--button-width:var(--control-height);min-width:var(--control-height);padding-inline:6px}
.configured-odds-fields .configured-odds-item:focus-within{border-color:var(--primary);box-shadow:0 0 0 2px var(--primary-ring)}
</style>
