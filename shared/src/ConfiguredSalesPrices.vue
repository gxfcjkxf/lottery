<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import DetailModal from './DetailModal.vue'
import SelectControl from './SelectControl.vue'
import { createCatalogI18n } from './localization'
import { configuredRuleCatalog } from './configured-rule-catalog'
import { configuredOptionPriceKeys, type ConfiguredSnapshot } from './configured-rules'
const props = defineProps<{ snapshot: ConfiguredSnapshot; modelValue: ConfiguredSnapshot['prices']; locale: 'en' | 'zh-CN'; disabled: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: ConfiguredSnapshot['prices']]; close: [] }>()
const i18n = createCatalogI18n(configuredRuleCatalog, props.locale)
watch(() => props.locale, value => i18n.setLocale(value))
const t = i18n.t
const draft = ref<ConfiguredSnapshot['prices']>(JSON.parse(JSON.stringify(props.modelValue)))
const batchOdds = ref('')
const submitButton = ref<HTMLButtonElement | null>(null)
onMounted(async () => { await nextTick(); submitButton.value?.focus({ preventScroll: true }) })
const plate = ref(props.snapshot.catalog.plates[0]!)
const playId = ref(props.snapshot.catalog.plays[0]!.id)
const rows = computed(() => {
  const play = props.snapshot.catalog.plays.find(item => item.id === playId.value)!
  const options = props.snapshot.catalog.domains[play.selection.domain]!.options
  const labels = new Map<string, string[]>()
  for (const option of options) for (const key of configuredOptionPriceKeys(props.snapshot, play, option)) {
    const names = labels.get(key) ?? []
    names.push(option.label[props.locale])
    labels.set(key, names)
  }
  const entries = [...labels].map(([key, names]) => ({ key, name: names.length === options.length && options.length > 1 ? play.label[props.locale] : [...new Set(names)].join(' / ') }))
  return entries.map((entry, index) => ({ key: entry.key, label: entries.filter(item => item.name === entry.name).length > 1 ? t('oddsNumber', { name: entry.name, index: entries.slice(0, index + 1).filter(item => item.name === entry.name).length }) : entry.name }))
})
function edit(key: string, value: string) {
  draft.value.plates[plate.value] = { ...draft.value.plates[plate.value], [key]: value }
}
function applyBatch() {
  for (const row of rows.value) edit(row.key, batchOdds.value)
}
function submit() {
  emit('update:modelValue', draft.value)
  emit('close')
}
watch([plate, playId], () => { batchOdds.value = '' })
</script>
<template>
  <DetailModal class="configured-odds-modal" :title="t('salesOdds')" :close-label="t('closeEditor')" @close="emit('close')">
    <div class="configured-odds-filters">
      <label>{{ t('plate') }}<SelectControl v-model="plate" :label="t('plate')" :options="snapshot.catalog.plates.map(value => ({ value, label: value }))" /></label>
      <label>{{ t('play') }}<SelectControl v-model="playId" :label="t('play')" :options="snapshot.catalog.plays.map(play => ({ value: play.id, label: play.label[locale] }))" /></label>
      <div v-if="!disabled" class="configured-odds-batch">
        <input v-model="batchOdds" inputmode="decimal" :aria-label="t('batchOdds')" />
        <button type="button" class="button button-secondary" :disabled="!batchOdds.trim()" @click="applyBatch">{{ t('batchModify') }}</button>
      </div>
    </div>
    <fieldset :disabled="disabled" class="configured-odds-fields">
      <label v-for="row in rows" :key="row.key"><span>{{ row.label }}</span><input inputmode="decimal" :value="draft.plates[plate]?.[row.key]" @input="edit(row.key, ($event.target as HTMLInputElement).value)" /></label>
    </fieldset>
    <template v-if="!disabled" #footer>
      <button ref="submitButton" type="button" class="button button-primary" @click="submit">{{ t('submitOdds') }}</button>
    </template>
  </DetailModal>
</template>
<style scoped>
.configured-odds-modal.detail-modal{width:min(760px,calc(100vw - 24px));max-width:calc(100vw - 24px)}
:global(#app .configured-odds-modal input[inputmode="decimal"]){--input-width:88px}
.configured-odds-filters{display:flex;flex-wrap:wrap;align-items:center;gap:12px;margin-bottom:16px}
.configured-odds-filters label{display:flex;flex-direction:row;align-items:center;gap:8px;max-width:100%}
.configured-odds-filters :deep(.select-control){min-width:0;max-width:100%}
.configured-odds-batch{display:flex;flex-direction:row;align-items:center;gap:8px;max-width:100%}
.configured-odds-batch input{width:88px;min-width:0;box-sizing:border-box}
.configured-odds-fields{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px;border:0;padding:0;margin:0;min-width:0}
.configured-odds-modal label{min-width:0;font-size:var(--body-font-size);overflow-wrap:anywhere}
.configured-odds-fields label{display:flex;flex-direction:row;align-items:center;justify-content:space-between;gap:8px}
.configured-odds-fields input{width:88px;max-width:55%;min-width:0;box-sizing:border-box;flex-shrink:0}
@media(max-width:620px){.configured-odds-fields{grid-template-columns:repeat(2,minmax(0,1fr))}}
@media(max-width:380px){.configured-odds-fields{grid-template-columns:minmax(0,1fr)}}
</style>
