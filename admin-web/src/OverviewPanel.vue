<script setup lang="ts">
import { computed } from 'vue'
import ManagementOverview from '@lottery/shared/ManagementOverview.vue'
import { platformDisplaySettings, type MessageParameters } from '@lottery/shared'
import { useAdminI18n, adminKey } from './i18n'
import { AdminApiError, type AdminAccount } from './admin-api'
import { brandPermissionSet } from './brand-permissions'
import { createBrandOverviewApi } from './overview-api'

const props = defineProps<{ account: AdminAccount; brandId: string }>()
const emit = defineEmits<{ 'session-invalid': [] }>()
const { t, locale, errorMessage, display } = useAdminI18n()
const api = createBrandOverviewApi()
const canView = computed(() => brandPermissionSet(props.account, props.brandId).has('overview.view.brand'))
const settings = computed(() => platformDisplaySettings.value!)
const translate = (key: string, parameters?: MessageParameters) => t(adminKey(key), parameters)
const read = (signal: AbortSignal) => api.read(props.brandId, signal)
function failure(cause: unknown) {
  if (cause instanceof AdminApiError && cause.status === 401) emit('session-invalid')
}
</script>
<template>
  <ManagementOverview :key="account.id + ':' + brandId + ':' + canView" scope="brand" :can-view="canView"
    :locale="locale" :display-settings="settings" :read="read" :t="translate"
    :error-message="cause => errorMessage(cause, 'read')" :display-error="display"
    @failure="failure" />
</template>
