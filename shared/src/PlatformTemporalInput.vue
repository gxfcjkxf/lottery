<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { createCatalogI18n } from './localization'
import PlatformDateInput from './PlatformDateInput.vue'
import PlatformTimeInput from './PlatformTimeInput.vue'
import { platformDisplaySettings, displaySettingsCatalog } from './display-settings'

const props = defineProps<{ modelValue: string; type: 'datetime-local' | 'date' | 'time'; locale: 'en' | 'zh-CN'; required?: boolean; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const control = ref<HTMLElement>()
function focusControl() { control.value!.querySelector<HTMLElement>('button, select')!.focus() }
const i18n = createCatalogI18n(displaySettingsCatalog, props.locale)
watch(() => props.locale, value => i18n.setLocale(value))
const t = i18n.t
const label = computed(() => t(props.type))
// Keep the canonical wall-clock model and existing business interpretation unchanged.
const date = computed(() => props.type === 'date' ? props.modelValue : props.modelValue.split('T')[0])
const time = computed(() => props.type === 'time' ? props.modelValue : props.modelValue.split('T')[1] ?? '')
function updateDate(value: string) { emit('update:modelValue', props.type === 'date' ? value : `${value}T${time.value}`) }
function updateTime(value: string) { emit('update:modelValue', props.type === 'time' ? value : `${date.value}T${value}`) }
const pattern = computed(() => props.type === 'date' ? '\\d{4}-\\d{2}-\\d{2}' : props.type === 'time' ? '\\d{2}:\\d{2}(:\\d{2})?' : '\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}(:\\d{2})?')
</script>
<template>
  <span v-if="platformDisplaySettings" ref="control" class="platform-temporal-input" role="group" :aria-label="label">
    <input class="platform-temporal-value" type="text" :value="modelValue" :pattern="pattern" :required="required" :disabled="disabled" :aria-label="label" aria-hidden="true" tabindex="-1" @focus="focusControl" />
    <PlatformDateInput v-if="type !== 'time'" :model-value="date" :display-settings="platformDisplaySettings" :locale="locale" :label="t('date')" :disabled="disabled" @update:model-value="updateDate" />
    <PlatformTimeInput v-if="type !== 'date'" :model-value="time" :display-settings="platformDisplaySettings" :locale="locale" :label="t('time')" :required="required" :disabled="disabled" @update:model-value="updateTime" />
  </span>
  <span v-else role="alert">{{ displaySettingsCatalog[locale].unavailable }}</span>
</template>
<style scoped>
.platform-temporal-input { display: flex; flex-wrap: wrap; align-items: start; gap: 8px; min-width: 0; position: relative; }
.platform-temporal-input > :not(.platform-temporal-value) { flex: 1 1 220px; min-width: 0; }
.platform-temporal-value { position: absolute; width: 1px; height: 1px; padding: 0; border: 0; opacity: 0; pointer-events: none; }
</style>
