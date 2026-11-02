<script setup lang="ts">
import {provide} from 'vue'
import CommissionCyclesManagement from '../../admin-web/src/CommissionCyclesManagement.vue'
import {createAdminI18n,adminI18nKey} from '../../admin-web/src/i18n'
import {setPlatformDisplaySettings} from '../../shared/src/display-settings'
import LoadingOverlay from '../../shared/src/LoadingOverlay.vue'
const props=defineProps<{locale:'en'|'zh-CN'}>()
const i18n=createAdminI18n();i18n.setLocale(props.locale);provide(adminI18nKey,i18n)
setPlatformDisplaySettings({timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'})
const brand='aaaaaaaa-aaaa-4aaa-8aaa-000000000001'
const account={id:'aaaaaaaa-aaaa-4aaa-8aaa-000000000002',username:'modal_spacing_review',super_admin:false,brand_ids:[brand],permissions:[],permissions_by_brand:{[brand]:['commission.view.brand','commission.run.brand']}}
</script>
<template><main class="management-console" style="padding:12px;min-width:0"><CommissionCyclesManagement :account="account" :brand-id="brand" brand-status="active" /><LoadingOverlay :label="i18n.t('admin.copy.ab7239f3')" /></main></template>
