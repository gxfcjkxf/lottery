<script setup lang="ts">
import { provide } from 'vue'
import AuditManagement from '../../admin-web/src/AuditManagement.vue'
import { createAdminI18n, adminI18nKey } from '../../admin-web/src/i18n'
import { setPlatformDisplaySettings } from '../../shared/src/display-settings'
const props = defineProps<{ locale: 'en' | 'zh-CN' }>()
const i18n = createAdminI18n(); i18n.setLocale(props.locale); provide(adminI18nKey, i18n)
setPlatformDisplaySettings({ timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss' })
const brand = '11111111-1111-4111-8111-111111111111'
const account = { id: '22222222-2222-4222-8222-222222222222', username: 'log_reviewer', super_admin: false, brand_ids: [brand], permissions: [], permissions_by_brand: { [brand]: ['audit.view.brand', 'audit.export.brand', 'notification.view.brand', 'notification.retry.brand'] } }
</script>
<template><main style="padding:16px;min-width:0"><AuditManagement :account="account" :brand-id="brand" /></main></template>
