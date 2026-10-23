<script setup lang="ts">
const account=defineModel<string>('account',{required:true}),password=defineModel<string>('password',{required:true})
defineProps<{accountLabel:string;passwordLabel:string;accountRequired:boolean;accountPattern?:string;accountPlaceholder?:string;passwordMinlength?:number;passwordMaxlength?:number}>()
</script>
<template>
  <label>{{accountLabel}}<input v-model.trim="account" :aria-label="accountLabel" autocomplete="off" maxlength="32" :required="accountRequired" :pattern="accountPattern" :placeholder="accountPlaceholder" /></label>
  <label>{{passwordLabel}}<input v-model="password" :aria-label="passwordLabel" type="password" autocomplete="new-password" required :minlength="passwordMinlength" :maxlength="passwordMaxlength" /><small v-if="$slots['password-help']"><slot name="password-help" /></small></label>
</template>
