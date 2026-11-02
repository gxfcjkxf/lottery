<script setup lang="ts">
import {provide} from 'vue'
import WithdrawalManagement from '../../admin-web/src/WithdrawalManagement.vue'
import RewardsManagement from '../../admin-web/src/RewardsManagement.vue'
import WithdrawalsPanel from '../../platform-web/src/WithdrawalsPanel.vue'
import FinanceManagement from '../../admin-web/src/FinanceManagement.vue'
import RechargesPanel from '../../platform-web/src/RechargesPanel.vue'
import RewardsPanel from '../../platform-web/src/RewardsPanel.vue'
import {createAdminI18n,adminI18nKey} from '../../admin-web/src/i18n'
import {createPlatformI18n,platformI18nKey} from '../../platform-web/src/i18n'
import {setPlatformDisplaySettings} from '../../shared/src/display-settings'
const props=defineProps<{locale:'en'|'zh-CN';view:string}>()
const admin=createAdminI18n();admin.setLocale(props.locale);provide(adminI18nKey,admin)
const platform=createPlatformI18n();platform.setLocale(props.locale);provide(platformI18nKey,platform)
const settings={timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'} as const
setPlatformDisplaySettings(settings)
const brand='11111111-1111-4111-8111-111111111111'
const account={id:'22222222-2222-4222-8222-222222222222',super_admin:false,brand_ids:[brand],permissions:[],permissions_by_brand:{[brand]:['recharge.view.brand','withdrawal.view.brand','withdrawal.approve.brand','reward.view.brand','reward.grant.brand']}}
</script>
<template><main class="management-console" style="padding:12px;min-width:0">
<WithdrawalManagement v-if="view==='brand-withdrawals'" :account="account" :brand-id="brand" active />
<RewardsManagement v-else-if="view==='brand-rewards'" :account="account" :brand-id="brand" brand-status="active" />
<WithdrawalsPanel v-else-if="view==='platform-withdrawals'" :brand-id="brand" :locale="locale" :display-settings="settings" />
<FinanceManagement v-else-if="view==='brand-recharges'" :account="account" :brand-id="brand" view="recharges" />
<RechargesPanel v-else-if="view==='platform-recharges'" :brand-id="brand" member-id="" :locale="locale" :display-settings="settings" />
<RewardsPanel v-else :brand-id="brand" brand-name="Aurora" :locale="locale" :display-settings="settings" />
</main></template>
