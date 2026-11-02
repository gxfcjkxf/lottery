<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import LoginSettingsSection from './LoginSettingsSection.vue'
import type { PlatformAccount } from './platform-api'
defineProps<{ account: PlatformAccount }>()
const emit = defineEmits<{ failure: [cause: unknown]; locked: [value: boolean] }>()
const authLocked = ref(false), complianceLocked = ref(false)
const locked = computed(() => authLocked.value || complianceLocked.value)
watch(locked, value => emit('locked', value), { immediate: true })
</script>

<template>
  <div class="security-settings console-stack">
    <LoginSettingsSection kind="auth" :account="account" :blocked="complianceLocked" @locked="authLocked = $event" @failure="emit('failure', $event)" />
    <LoginSettingsSection kind="compliance" :account="account" :blocked="authLocked" @locked="complianceLocked = $event" @failure="emit('failure', $event)" />
  </div>
</template>
