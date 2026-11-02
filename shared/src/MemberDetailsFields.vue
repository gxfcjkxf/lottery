<script setup lang="ts">
const phone=defineModel<string>('phone',{required:true})
defineProps<{phoneLabel:string;phoneMaxlength?:number}>()
</script>
<template><label>{{phoneLabel}}<input v-model.trim="phone" :maxlength="phoneMaxlength" inputmode="tel" autocomplete="off" /></label></template>
