<script setup lang="ts">
const phone=defineModel<string>('phone',{required:true})
defineProps<{phoneLabel:string;phoneMaxlength?:number}>()
</script>
<template><label class="ui-field-row">{{phoneLabel}}<input v-model.trim="phone" :maxlength="phoneMaxlength" inputmode="tel" autocomplete="off" /></label></template>
