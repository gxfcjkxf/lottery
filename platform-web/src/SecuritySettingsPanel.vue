<script setup lang="ts">
import { ref, watch } from 'vue'
import LoginSettingsSection from './LoginSettingsSection.vue'
import type { PlatformAccount } from './platform-api'
defineProps<{ account: PlatformAccount }>()
const emit = defineEmits<{ failure: [cause: unknown]; locked: [value: boolean] }>()
const authLocked = ref(false)
watch(authLocked, value => emit('locked', value), { immediate: true })
</script>

<template>
  <div class="security-settings console-stack">
    <LoginSettingsSection kind="auth" :account="account" :blocked="false" @locked="authLocked = $event" @failure="emit('failure', $event)" />
  </div>
</template>
