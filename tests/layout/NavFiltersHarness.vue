<script setup lang="ts">
import { provide } from 'vue'
import Cycles from '../../admin-web/src/CommissionCyclesManagement.vue'
import Audit from '../../admin-web/src/AuditManagement.vue'
import Reports from '../../admin-web/src/ReportsManagement.vue'
import PlatformReports from '../../platform-web/src/ReportsPanel.vue'
import { createAdminI18n, adminI18nKey } from '../../admin-web/src/i18n'
import { createPlatformI18n, platformI18nKey } from '../../platform-web/src/i18n'
import { setPlatformDisplaySettings } from '../../shared/src/display-settings'
const props = defineProps<{locale:'en'|'zh-CN';view:'audit'|'reports'|'platform'|'cycles'}>()
const admin = createAdminI18n();admin.setLocale(props.locale);provide(adminI18nKey,admin)
const platform = createPlatformI18n();platform.setLocale(props.locale);provide(platformI18nKey,platform)
const settings={timezone:'Asia/Singapore',date_format:'YYYY-MM-DD' as const,time_format:'HH:mm:ss' as const}
setPlatformDisplaySettings(settings)
const brand='11111111-1111-4111-8111-111111111111'
const account={id:'22222222-2222-4222-8222-222222222222',super_admin:false,brand_ids:[brand],permissions:[],permissions_by_brand:{[brand]:['audit.view.brand','audit.export.brand','report_betting.view.brand','report_ledger.view.brand','commission.view.brand','report_commission.view.brand','report_attribution.view.brand']}}
</script>
<template>
<div class="app-shell management-console"><aside class="sidebar"></aside><div class="main-shell"><header class="topbar"><div class="breadcrumbs"><span>Aurora</span><span>/</span><b>{{view==='audit'?admin.t('admin.copy.315a9366'):admin.t('admin.nav.reports')}}</b></div></header><main class="page-content"><Audit v-if="view==='audit'" :account="account" :brand-id="brand"/><Reports v-else-if="view==='reports'" :account="account" :brand-id="brand"/><Cycles v-else-if="view==='cycles'" :account="account" :brand-id="brand"/><PlatformReports v-else :brand-id="brand" member-id="" :locale="locale" :display-settings="settings"/></main></div></div>
</template>
