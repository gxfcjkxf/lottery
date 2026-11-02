<script setup lang="ts">
import {provide} from 'vue'
import '../../admin-web/src/report-workspace.css'
import Audit from '../../admin-web/src/AuditManagement.vue'
import Reward from '../../admin-web/src/RewardReports.vue'
import Commission from '../../admin-web/src/CommissionReport.vue'
import Withdrawal from '../../admin-web/src/WithdrawalReport.vue'
import Finance from './FinanceConsistencyHarness.vue'
import {createAdminI18n,adminI18nKey} from '../../admin-web/src/i18n'
import {setPlatformDisplaySettings} from '../../shared/src/display-settings'
const props=defineProps<{locale:'en'|'zh-CN';view:string}>()
const i18n=createAdminI18n();i18n.setLocale(props.locale);provide(adminI18nKey,i18n)
setPlatformDisplaySettings({timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'})
const brand='11111111-1111-4111-8111-111111111111'
const account={id:'22222222-2222-4222-8222-222222222222',super_admin:false,brand_ids:[brand],permissions:[],permissions_by_brand:{[brand]:['audit.view.brand','audit.export.brand','report_reward.view.brand','report_reward.export.brand','report_commission.view.brand','report_commission.export.brand','report_withdrawal.view.brand','report_withdrawal.export.brand']}}
</script>
<template><main class="management-console" style="padding:12px;min-width:0"><Audit v-if="view==='audit'" :brand-id="brand" :account="account"/><div v-else-if="view.endsWith('report')" class="reports-management"><div class="report-panel"><Reward v-if="view==='reward-report'" :brand-id="brand" :account="account"/><Commission v-else-if="view==='commission-report'" :brand-id="brand" :account="account"/><Withdrawal v-else-if="view==='withdrawal-report'" :brand-id="brand" :account="account"/></div></div><Finance v-else :locale="locale" :view="view"/></main></template>
