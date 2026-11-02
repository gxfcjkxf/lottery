<script setup lang="ts">
import SelectControl from './SelectControl.vue'
import { computed } from 'vue'
import { timeZoneCatalog, timeZoneOptions, timezoneOffset } from './time-zones'
const props = defineProps<{ modelValue: string; locale: 'en' | 'zh-CN'; disabled?: boolean; required?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const copy = computed(() => timeZoneCatalog[props.locale])
const options = computed(() => timeZoneOptions.map(([value, name]) => ({
  value, label: `${value}${props.locale === 'zh-CN' ? '（' : ' ('}${copy.value[name]}${props.locale === 'zh-CN' ? '，' : ', '}${timezoneOffset(value)}${props.locale === 'zh-CN' ? '）' : ')'}`,
})))
</script>

<template>
  <SelectControl :model-value="modelValue" :disabled="disabled" :required="required" @change="emit('update:modelValue', $event)" :label="copy.choose" :options="[{ value: '', label: (String(copy.choose)).trim(), disabled: true }, ...((options).map((zone) => ({ value: zone.value, label: (String(zone.label)).trim() })))]" />
</template>
