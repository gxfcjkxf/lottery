<script setup lang="ts">
import ManagementOverview from '@lottery/shared/ManagementOverview.vue'
import { PlatformApiError } from './platform-api'
import { createOverviewApi } from './overview-api'
import { usePlatformI18n } from './i18n'
import type { PlatformDisplaySettings } from './platform-format'

defineProps<{ canView: boolean; locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings }>()
const emit = defineEmits<{ failure: [cause: unknown]; brands: [] }>()
const { t, errorMessage, displayError } = usePlatformI18n()
const api = createOverviewApi()
function failure(cause: unknown) {
  if (cause instanceof PlatformApiError && cause.status === 401) emit('failure', cause)
}
</script>
<template>
  <ManagementOverview scope="platform" :can-view="canView" :locale="locale" :display-settings="displaySettings"
    :read="api.read" :t="t" :error-message="cause => errorMessage(cause, 'read')" :display-error="displayError"
    @failure="failure" @open="emit('brands')" />
</template>
