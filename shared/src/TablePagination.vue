<script setup lang="ts">
defineProps<{ summary: string; ariaLabel?: string }>()
</script>

<template>
  <nav class="table-pagination" :aria-label="ariaLabel">
    <span>{{ summary }}</span>
    <div v-if="$slots.actions" class="table-pagination-actions"><slot name="actions" /></div>
  </nav>
</template>

<style scoped>
.table-pagination { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 48px; padding: 8px 14px; border-top: 1px solid var(--line, #e2e8f0); color: var(--muted, #64748b); font-size: 12px; }
.table-pagination-actions { display: flex; align-items: center; gap: 8px; }
@media (max-width: 640px) { .table-pagination { align-items: stretch; flex-direction: column; } .table-pagination-actions { justify-content: flex-end; } }
</style>
