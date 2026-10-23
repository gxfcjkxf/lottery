<script setup lang="ts">
import SummaryFacts from './SummaryFacts.vue'
import DetailModal from './DetailModal.vue'
defineProps<{ title: string; closeLabel: string; fields: Array<{ label: string; value: string | number }> }>()
defineEmits<{ close: [] }>()
</script>
<template><DetailModal class="console-account-dialog" :title="title" :close-label="closeLabel" @close="$emit('close')"><div class="console-stack"><SummaryFacts class="confirm-list" :fields="fields" /><slot /></div></DetailModal></template>
