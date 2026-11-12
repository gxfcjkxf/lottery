<script setup lang="ts">
import { ref } from 'vue'
import SiteIcon from './SiteIcon'
const passwordVisible = ref(false)
const identifier = defineModel<string>('identifier', { required: true })
const password = defineModel<string>('password', { required: true })
defineProps<{
  identifierLabel: string
  passwordLabel: string
  readonly?: boolean
  passwordMinlength?: number
  passwordMaxlength?: number
  passwordCharacters?: boolean
  passwordVisibilityLabels?: { show: string; hide: string }
}>()
</script>
<template>
  <label class="login-entry__field"><span class="login-entry__label">{{identifierLabel}}</span><span class="login-entry__input"><input v-model="identifier" class="field" autocomplete="username" required :readonly="readonly" /></span></label>
  <label class="login-entry__field"><span class="login-entry__label">{{passwordLabel}}</span><span class="login-entry__input" :class="{ 'login-entry__input--password': passwordVisibilityLabels }"><input v-model="password" class="field" :type="passwordVisible ? 'text' : 'password'" autocomplete="current-password" required :readonly="readonly" :minlength="passwordMinlength" :maxlength="passwordMaxlength" :data-min-characters="passwordCharacters ? 6 : undefined" :data-max-characters="passwordCharacters ? 30 : undefined" /><button v-if="passwordVisibilityLabels" class="login-entry__password-toggle" type="button" :aria-label="passwordVisible ? passwordVisibilityLabels.hide : passwordVisibilityLabels.show" :aria-pressed="passwordVisible" @click="passwordVisible = !passwordVisible"><SiteIcon :name="passwordVisible ? 'eyeOff' : 'eye'" /></button></span></label>
</template>
