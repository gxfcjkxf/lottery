<script setup lang="ts">
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
  <select :value="modelValue" :disabled="disabled" :required="required" @change="emit('update:modelValue', ($event.target as HTMLSelectElement).value)">
    <option value="" disabled>{{ copy.choose }}</option>
    <option v-for="zone in options" :key="zone.value" :value="zone.value">{{ zone.label }}</option>
  </select>
</template>
