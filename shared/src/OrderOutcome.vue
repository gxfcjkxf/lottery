<script setup lang="ts">
import { computed } from 'vue'
import { configuredRuleCatalog } from './configured-rule-catalog'
const props = defineProps<{outcome?: string; locale: 'en' | 'zh-CN'}>()
const label = computed(() => {
  const outcome = props.outcome
  if (outcome === undefined) return '—'
  if (outcome !== 'won' && outcome !== 'lost' && outcome !== 'push') throw new TypeError('Invalid betting outcome')
  return configuredRuleCatalog[props.locale][outcome]
})
</script>
<template><span class="configured-order-outcome" :class="outcome ? `configured-order-outcome--${outcome}` : undefined">{{ label }}</span></template>
<style scoped>
.configured-order-outcome{font-weight:600;white-space:nowrap}
.configured-order-outcome--won{color:var(--success-text)}
.configured-order-outcome--lost{color:var(--danger-text)}
.configured-order-outcome--push{color:var(--warning-text)}
</style>
