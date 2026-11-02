<script setup lang="ts">
import {provide} from 'vue'
import RewardsManagement from '../../admin-web/src/RewardsManagement.vue'
import {createAdminI18n,adminI18nKey} from '../../admin-web/src/i18n'
const props=defineProps<{locale:'en'|'zh-CN';readonly?:boolean}>()
const i18n=createAdminI18n();i18n.setLocale(props.locale);provide(adminI18nKey,i18n)
const brand='11111111-1111-4111-8111-111111111111'
const account={id:'22222222-2222-4222-8222-222222222222',super_admin:false,brand_ids:[brand],permissions:[],permissions_by_brand:{[brand]:props.readonly?['reward.view.brand']:['reward.view.brand','reward.grant.brand']}}
</script>
<template><main class="management-console" style="padding:12px;min-width:0"><RewardsManagement :account="account" :brand-id="brand" brand-status="active" /></main></template>
