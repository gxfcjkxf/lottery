<script setup lang="ts">
import {provide,ref} from 'vue'
import {setPlatformDisplaySettings,setInlineValidationLocale} from '../../shared/src/index'
import {createAdminI18n,adminI18nKey} from '../../admin-web/src/i18n'
import CommissionManagement from '../../admin-web/src/CommissionManagement.vue'
import OperationToastHost from '../../admin-web/src/OperationToastHost.vue'
import LoadingOverlay from '../../shared/src/LoadingOverlay.vue'
const props=defineProps<{locale:'en'|'zh-CN'}>()
const i18n=createAdminI18n();i18n.setLocale(props.locale);provide(adminI18nKey,i18n);setInlineValidationLocale(props.locale)
setPlatformDisplaySettings({timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'})
const brand='aaaaaaaa-aaaa-4aaa-8aaa-000000000001',actor='aaaaaaaa-aaaa-4aaa-8aaa-000000000002'
const account={id:actor,username:'commission_reviewer',super_admin:false,brand_ids:[brand],permissions:[],permissions_by_brand:{[brand]:['commission.view.brand','commission.run.brand','commission.retry.brand','commission_payment.approve.brand','commission_payment.retry.brand','commission_adjustment.write.brand','commission_correction.approve.brand','commission_correction.retry.brand','commission_correction.continue.brand','commission_correction.execute_retry.brand']}}
</script>
<template>
 <main class="management-console" style="display:block;min-height:100vh"><section class="page-content" style="max-width:1500px;margin:auto;padding:12px">
  <CommissionManagement :account="account" :brand-id="brand" brand-status="active" />
 </section><OperationToastHost /><LoadingOverlay :label="i18n.t('admin.copy.ab7239f3')" /></main>
</template>
