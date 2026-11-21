<script setup lang="ts">
import SelectControl from './SelectControl.vue'
import PlatformTemporalInput from './PlatformTemporalInput.vue'
import { createCatalogI18n } from './localization'
import { computed, watch } from 'vue'
const props = defineProps<{ modelValue: string; recentSupported?: boolean; today: string; yesterday: string; locale: 'en' | 'zh-CN'; disabled?: boolean }>()
const emit = defineEmits<{ select: [date: string] }>()
const i18n = createCatalogI18n({en:{view:'View',byDate:'By date',recent50:'Latest 50 draws',recent100:'Latest 100 draws',date:'Date',today:'Today',yesterday:'Yesterday'},'zh-CN':{view:'查看方式',byDate:'按日期',recent50:'最近50期',recent100:'最近100期',date:'日期',today:'当天',yesterday:'昨天'}}, props.locale)
watch(() => props.locale, value => i18n.setLocale(value))
const t = i18n.t
const mode = computed(() => ['recent-50','recent-100'].includes(props.modelValue) ? props.modelValue : 'date')
</script>
<template>
 <div class="draw-history-date-filter">
  <div v-if="recentSupported" class="ui-filter-field"><SelectControl :model-value="mode" :label="t('view')" :disabled="disabled" :options="[{value:'date',label:t('byDate')},{value:'recent-50',label:t('recent50')},{value:'recent-100',label:t('recent100')}]" @change="value => emit('select', value==='date' ? today : value)" /></div>
  <div v-if="mode==='date'" class="ui-filter-field"><PlatformTemporalInput :model-value="modelValue" type="date" :locale="locale" :label="t('date')" :disabled="disabled" @update:model-value="emit('select', $event)" /></div>
  <div v-if="mode==='date'" class="draw-date-shortcuts"><button v-for="choice in [{date:today,label:t('today')},{date:yesterday,label:t('yesterday')}]" :key="choice.date" type="button" class="button" :class="modelValue===choice.date ? 'button-primary' : 'button-secondary'" :disabled="disabled" :aria-pressed="modelValue===choice.date" @click="emit('select', choice.date)">{{ choice.label }}</button></div>
 </div>
</template>
<style scoped>
.draw-history-date-filter{display:flex;align-items:flex-end;flex-wrap:wrap;gap:8px;min-width:0}
.draw-history-date-filter .ui-filter-field{margin:0;min-width:0}
.draw-date-shortcuts{display:flex;gap:8px}
</style>
