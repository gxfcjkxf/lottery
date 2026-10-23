<script setup lang="ts">
const name=defineModel<string>('name',{required:true}),phone=defineModel<string>('phone',{required:true})
defineProps<{nameLabel:string;phoneLabel:string;phoneMaxlength?:number}>()
</script>
<template><label>{{nameLabel}}<input v-model.trim="name" maxlength="120" /></label><label>{{phoneLabel}}<input v-model.trim="phone" :maxlength="phoneMaxlength" inputmode="tel" autocomplete="off" /></label></template>
