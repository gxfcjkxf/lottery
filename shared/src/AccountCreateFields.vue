<script setup lang="ts">
import { formValidationText } from './form-validation'
const account=defineModel<string>('account',{required:true}),password=defineModel<string>('password',{required:true})
defineProps<{accountLabel:string;passwordLabel:string;accountRequired:boolean;accountPattern?:string;accountPlaceholder?:string;passwordMinlength?:number;passwordMaxlength?:number;passwordCharacters?:boolean}>()
</script>
<template>
  <label>{{accountLabel}}<input v-model.trim="account" :aria-label="accountLabel" autocomplete="off" maxlength="32" :required="accountRequired" :pattern="accountPattern" :data-validation-format="accountPattern ? formValidationText('accountFormat') : undefined" :placeholder="accountPlaceholder" /></label>
  <label>{{passwordLabel}}<input v-model="password" :aria-label="passwordLabel" type="password" autocomplete="new-password" required :minlength="passwordMinlength" :maxlength="passwordMaxlength" :data-min-characters="passwordCharacters ? 6 : undefined" :data-max-characters="passwordCharacters ? 30 : undefined" /></label>
</template>
