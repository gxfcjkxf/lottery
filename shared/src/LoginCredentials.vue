<script setup lang="ts">
const identifier = defineModel<string>('identifier', { required: true })
const password = defineModel<string>('password', { required: true })
defineProps<{
  identifierLabel: string
  passwordLabel: string
  readonly?: boolean
  passwordMinlength?: number
  passwordMaxlength?: number
}>()
</script>
<template>
  <label class="login-entry__field">{{identifierLabel}}<input v-model="identifier" class="field" autocomplete="username" required :readonly="readonly" /></label>
  <label class="login-entry__field">{{passwordLabel}}<input v-model="password" class="field" type="password" autocomplete="current-password" required :readonly="readonly" :minlength="passwordMinlength" :maxlength="passwordMaxlength" /></label>
</template>
