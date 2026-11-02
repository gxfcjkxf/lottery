<script setup lang="ts">
import {provide,ref} from 'vue'
import {setPlatformDisplaySettings,setInlineValidationLocale} from '../../shared/src/index'
import {createAdminI18n,adminI18nKey} from '../../admin-web/src/i18n'
import CommissionCorrectionsManagement from '../../admin-web/src/CommissionCorrectionsManagement.vue'
import ReportsManagement from '../../admin-web/src/ReportsManagement.vue'
import OperationToastHost from '../../admin-web/src/OperationToastHost.vue'
import LoadingOverlay from '../../shared/src/LoadingOverlay.vue'
const props=defineProps<{locale:'en'|'zh-CN'}>()
const i18n=createAdminI18n();i18n.setLocale(props.locale);provide(adminI18nKey,i18n);setInlineValidationLocale(props.locale)
setPlatformDisplaySettings({timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'})
const brand='aaaaaaaa-aaaa-4aaa-8aaa-000000000001',actor='aaaaaaaa-aaaa-4aaa-8aaa-000000000002',cycle='bbbbbbbb-bbbb-4bbb-8bbb-000000000001'
const account={id:actor,username:'reviewer',super_admin:false,brand_ids:[brand],permissions:[],permissions_by_brand:{[brand]:['commission.view.brand','commission_correction_policy.write.brand','commission_correction.retry.brand','commission_correction.approve.brand','commission_correction.continue.brand','commission_correction.execute_retry.brand','report_commission.view.brand','report_commission.export.brand']}}
const section=ref('corrections')
</script>
<template><main class="management-console" style="display:block;min-height:100vh"><section class="page-content" style="max-width:1500px;margin:auto;padding:12px"><nav><button @click="section='corrections'">Corrections</button><button @click="section='reports'">Reports</button></nav><CommissionCorrectionsManagement v-if="section==='corrections'" :account="account" :brand-id="brand" brand-status="active"/><ReportsManagement v-else :account="account" :brand-id="brand"/></section><OperationToastHost/><LoadingOverlay :label="i18n.t('admin.copy.ab7239f3')"/></main></template>
