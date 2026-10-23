<script setup lang="ts" generic="T extends { id: string }">
defineProps<{ entries: T[]; columns: Array<{ key: string; label: string; cellClass?: string }>; emptyLabel: string; empty: boolean; tableClass?: string }>()
</script>
<template><div class="table-wrap"><table class="console-table console-responsive-table" :class="tableClass"><thead><tr><th v-for="column in columns" :key="column.key">{{ column.label }}</th></tr></thead><tbody><tr v-for="entry in entries" :key="entry.id"><td v-for="column in columns" :key="column.key" :data-label="column.label" :class="column.cellClass"><slot :name="column.key" :entry="entry" /></td></tr><tr v-if="empty"><td :colspan="columns.length" class="empty-state">{{ emptyLabel }}</td></tr></tbody></table></div></template>
