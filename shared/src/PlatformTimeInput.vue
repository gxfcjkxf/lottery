<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { PlatformDisplaySettings } from './date-time'
import { createCatalogI18n } from './localization'
import { platformLotteryCatalog } from './platform-lottery'

const props = withDefaults(defineProps<{ modelValue: string; displaySettings: PlatformDisplaySettings; label: string; locale: 'en' | 'zh-CN'; required?: boolean; disabled?: boolean }>(), { required: true })
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const i18n = createCatalogI18n(platformLotteryCatalog, props.locale)
watch(() => props.locale, value => i18n.setLocale(value))
const t = i18n.t
const twelveHour = computed(() => props.displaySettings.time_format === 'hh:mm:ss A')
const hour = ref(''), minute = ref(''), second = ref('00'), meridiem = ref('AM')
const pad = (value: number) => String(value).padStart(2, '0')
const hours = computed(() => Array.from({ length: twelveHour.value ? 12 : 24 }, (_, index) => pad(index + (twelveHour.value ? 1 : 0))))
const minutes = Array.from({ length: 60 }, (_, index) => pad(index))
watch(() => [props.modelValue, props.displaySettings.time_format], () => {
  if (!props.modelValue) { hour.value = ''; minute.value = ''; second.value = '00'; meridiem.value = 'AM'; return }
  const [h, m, s] = props.modelValue.split(':')
  hour.value = pad(twelveHour.value ? Number(h) % 12 || 12 : Number(h))
  minute.value = m; second.value = s ?? '00'; meridiem.value = Number(h) >= 12 ? 'PM' : 'AM'
}, { immediate: true })
function update() {
  if (!hour.value || !minute.value || !second.value) return
  const h = twelveHour.value ? Number(hour.value) % 12 + (meridiem.value === 'PM' ? 12 : 0) : Number(hour.value)
  emit('update:modelValue', `${pad(h)}:${minute.value}:${second.value}`)
}
</script>

<template>
  <span class="platform-time-input" :lang="locale" role="group" :aria-label="label">
    <select v-model="hour" :aria-label="`${label} · ${t('timeHour')}`" :required="required" :disabled="disabled" @change="update"><option value="" disabled>{{ t('timeHour') }}</option><option v-for="value in hours" :key="value" :value="value">{{ value }}</option></select>
    <select v-model="minute" :aria-label="`${label} · ${t('timeMinute')}`" :required="required" :disabled="disabled" @change="update"><option value="" disabled>{{ t('timeMinute') }}</option><option v-for="value in minutes" :key="value" :value="value">{{ value }}</option></select>
    <select v-model="second" :aria-label="`${label} · ${t('timeSecond')}`" :required="required" :disabled="disabled" @change="update"><option v-for="value in minutes" :key="value" :value="value">{{ value }}</option></select>
    <select v-if="twelveHour" v-model="meridiem" :aria-label="`${label} · ${t('timeMeridiem')}`" :disabled="disabled" @change="update"><option value="AM">{{ t('timeAM') }}</option><option value="PM">{{ t('timePM') }}</option></select>
  </span>
</template>

<style scoped>
.platform-time-input { display: flex; align-items: flex-start; flex-wrap: wrap; gap: 6px; min-width: 0; }
.platform-time-input select { flex: 1 1 72px; width: 0; min-width: 72px; min-height: 40px; box-sizing: border-box; max-width: 100%; padding: 8px 20px 8px 6px; font: inherit; border: 1px solid var(--line); border-radius: var(--radius-sm); background: var(--surface); color: var(--ink); }
@media (max-width: 640px) { .platform-time-input select { min-height: 44px; } }
</style>
