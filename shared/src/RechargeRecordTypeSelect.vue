<script setup lang="ts">
import { watch } from 'vue'
import SelectControl from './SelectControl.vue'
import { createCatalogI18n } from './localization'
import { cashCatalog } from './cash-catalog'
export type RechargeRecordType = 'deposit' | 'manual' | 'to_points' | 'from_points'
const props = defineProps<{ modelValue: RechargeRecordType; locale: 'en' | 'zh-CN'; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: RechargeRecordType] }>()
const i18n = createCatalogI18n(cashCatalog, props.locale)
const t = i18n.t
watch(() => props.locale, value => i18n.setLocale(value))
</script>

<template>
  <SelectControl data-testid="recharge-record-type" :model-value="modelValue" :label="t('cash.kind')" :disabled="disabled"
    :options="[
      { value: 'deposit', label: t('cash.fundsDeposit') },
      { value: 'manual', label: t('cash.manualPointsRecharge') },
      { value: 'to_points', label: t('cash.to_points') },
      { value: 'from_points', label: t('cash.from_points') },
    ]"
    @change="emit('update:modelValue', $event as RechargeRecordType)" />
</template>
