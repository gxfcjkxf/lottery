<script setup lang="ts">
import type { NumericTone } from "./report-tones"

defineProps<{
  fields: Array<{
    label: string
    value: string | number | null | undefined
    tone?: NumericTone
  }>
}>()
</script>

<template>
  <dl class="console-facts">
    <div v-for="field in fields" :key="field.label">
      <dt>{{ field.label }}</dt>
      <dd>
        <span v-if="field.tone" :data-numeric-tone="field.tone">{{ field.value }}</span>
        <template v-else>{{ field.value }}</template>
      </dd>
    </div>
  </dl>
</template>
