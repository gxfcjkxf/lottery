<script setup lang="ts">
import { computed, watch } from 'vue'
import SelectControl from './SelectControl.vue'
import { createCatalogI18n } from './localization'
import { cashCatalog } from './cash-catalog'
import type { RechargeRecordType } from './RechargeRecordTypeSelect.vue'
const props = defineProps<{ modelValue: string; kind: RechargeRecordType; locale: 'en' | 'zh-CN'; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: string]; change: [] }>()
const i18n = createCatalogI18n(cashCatalog, props.locale)
watch(() => props.locale, value => i18n.setLocale(value))
const states = computed(() => props.kind === 'manual' ? ['pending', 'confirmed', 'cancelled'] : props.kind === 'deposit' ? ['pending', 'submitted', 'confirmed', 'cancelled', 'expired'] : ['completed'])
const options = computed(() => [{ value: '', label: i18n.t('cash.all') }, ...states.value.map(value => ({ value, label: i18n.t(props.kind === 'manual' && value === 'pending' ? 'cash.pointsPending' : props.kind === 'manual' && value === 'confirmed' ? 'cash.pointsConfirmed' : `cash.${value}`) }))])
</script>
<template>
  <SelectControl data-testid="cash-state-filter" :model-value="modelValue" :label="i18n.t('cash.status')" :options="options" :disabled="disabled" @change="emit('update:modelValue', String($event)); emit('change')" />
</template>
