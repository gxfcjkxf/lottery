<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

const list = ref<HTMLUListElement>()
let observer: ResizeObserver
onMounted(() => {
  const element = list.value!
  const rows = Array.from(element.children).slice(0, 10).map(branch => branch.querySelector<HTMLElement>(':scope > .agent-tree-row')!)
  observer = new ResizeObserver(() => {
    element.style.maxHeight = element.children.length > 10
      ? `${rows.reduce((height, row) => height + row.getBoundingClientRect().height, 0) + 9 * 8}px`
      : ''
  })
  rows.forEach(row => observer.observe(row))
})
onBeforeUnmount(() => observer.disconnect())
</script>

<template><ul ref="list" class="agent-tree-level"><slot /></ul></template>

<style scoped>
.agent-tree-level { display: grid; align-content: start; gap: 8px; min-width: 0; margin: 0; padding: 0; list-style: none; overflow-y: auto; }
</style>
