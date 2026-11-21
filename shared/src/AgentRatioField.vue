<script setup lang="ts">
import { useId } from 'vue'
const ratio=defineModel<string>({required:true})
defineProps<{label:string;maximumLabel?:string}>()
const maximumId=useId()
</script>
<template><label>{{label}}<span class="am-ratio-input"><span class="am-ratio-value"><input v-model.trim="ratio" :aria-label="label" :aria-describedby="maximumLabel ? maximumId : undefined" inputmode="decimal" autocomplete="off" required /><span aria-hidden="true">%</span></span><span v-if="maximumLabel" :id="maximumId" class="am-ratio-maximum">{{maximumLabel}}</span></span></label></template>
<style scoped>
.am-ratio-input { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 8px; min-width: 0; }
.am-ratio-input > .am-ratio-value { display: inline-flex; flex: 0 1 auto; align-items: center; gap: 8px; min-width: 0; max-width: 100%; }
.am-ratio-value input { min-width: 0; max-width: calc(100% - 24px); }
.am-ratio-maximum { color: var(--ink-secondary); font-size: var(--body-font-size); font-weight: 400; }
</style>
