<script setup lang="ts">
import { formValidationText } from './form-validation'
const account=defineModel<string>('account',{required:true}),password=defineModel<string>('password',{required:true})
defineProps<{accountLabel:string;passwordLabel:string;accountRequired:boolean;accountPattern?:string;accountPlaceholder?:string;passwordMinlength?:number;passwordMaxlength?:number;passwordCharacters?:boolean}>()
</script>
<template>
  <label>{{accountLabel}}<input v-model.trim="account" :aria-label="accountLabel" autocomplete="off" maxlength="32" :required="accountRequired" :pattern="accountPattern" :data-validation-format="accountPattern ? formValidationText('accountFormat') : undefined" :placeholder="accountPlaceholder" /><small v-if="accountPattern">{{formValidationText('accountFormat')}}</small></label>
  <label>{{passwordLabel}}<input v-model="password" :aria-label="passwordLabel" type="password" autocomplete="new-password" required :minlength="passwordMinlength" :maxlength="passwordMaxlength" :data-min-characters="passwordCharacters ? 6 : undefined" :data-max-characters="passwordCharacters ? 30 : undefined" /><small v-if="passwordCharacters">{{formValidationText('passwordHelp')}}</small><small v-else-if="$slots['password-help']"><slot name="password-help" /></small></label>
</template>
