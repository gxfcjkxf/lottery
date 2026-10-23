<script setup lang="ts">
const ratio=defineModel<string>({required:true})
defineProps<{label:string;help:string}>()
</script>
<template><label>{{label}}<span class="am-ratio-input"><input v-model.trim="ratio" :aria-label="label" inputmode="decimal" autocomplete="off" required /><span aria-hidden="true">%</span></span><small>{{help}}</small></label></template>
