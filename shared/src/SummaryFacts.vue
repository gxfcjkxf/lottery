<script setup lang="ts">
defineProps<{fields:Array<{label:string;value:string|number|null|undefined}>}>()
</script>
<template><dl class="console-facts"><div v-for="field in fields" :key="field.label"><dt>{{field.label}}</dt><dd>{{field.value}}</dd></div></dl></template>
