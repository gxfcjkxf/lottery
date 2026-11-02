<script setup lang="ts">
// Business reads, permissions and writes stay with the owning page.
defineProps<{ busy?: boolean; embedded?: boolean }>()
</script>
<template>
  <section class="panel console-panel management-list" :class="{ 'management-list--embedded': embedded }" :aria-busy="busy || undefined">
    <header v-if="$slots.heading || $slots.tabs || $slots.filters || $slots.actions" class="management-list-toolbar console-page-heading">
      <div v-if="$slots.heading" class="management-list-heading"><slot name="heading" /></div>
      <div v-if="$slots.tabs" class="management-list-tabs"><slot name="tabs" /></div>
      <div v-if="$slots.filters" class="management-list-filters"><slot name="filters" /></div>
      <div v-if="$slots.actions" class="console-page-actions"><slot name="actions" /></div>
    </header>
    <div v-if="$slots.summary" class="management-list-summary"><slot name="summary" /></div>
    <slot />
    <slot name="pagination" />
  </section>
</template>
