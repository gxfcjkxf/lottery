<script setup lang="ts">
import { provide, ref, watch } from 'vue'
import PointPolicy from '../../admin-web/src/PointPolicySettings.vue'
import { createAdminI18n, adminI18nKey } from '../../admin-web/src/i18n'
import OperationToastHost from '../../shared/src/OperationToastHost.vue'
import Withdrawals from '../../platform-web/src/WithdrawalsPanel.vue'
import Rewards from '../../platform-web/src/RewardsPanel.vue'
import LoadingOverlay from '../../shared/src/LoadingOverlay.vue'
import { setPlatformDisplaySettings } from '../../shared/src/display-settings'
import { createPlatformI18n, platformI18nKey } from '../../platform-web/src/i18n'
const props=defineProps<{locale:'en'|'zh-CN';kind:'withdrawals'|'rewards'|'policy'}>()
const selectedLocale=ref(props.locale)
const admin=createAdminI18n();admin.setLocale(props.locale);provide(adminI18nKey,admin)
const i18n=createPlatformI18n();i18n.setLocale(props.locale);provide(platformI18nKey,i18n)
watch(selectedLocale,value=>{admin.setLocale(value);i18n.setLocale(value)})
const account={id:'22222222-2222-4222-8222-222222222222',super_admin:false,brand_ids:['11111111-1111-4111-8111-111111111111'],permissions:[],permissions_by_brand:{'11111111-1111-4111-8111-111111111111':['point_policy.view.brand','point_policy.write.brand']}}
const displaySettings={timezone:'Asia/Singapore',date_format:'YYYY-MM-DD' as const,time_format:'HH:mm:ss' as const};setPlatformDisplaySettings(displaySettings)
const brandId=ref('11111111-1111-4111-8111-111111111111')
</script>
<template>
 <div class="management-console console-stack" style="padding:12px">
  <button data-testid="switch-fixture-brand" class="button" @click="brandId='99999999-9999-4999-8999-999999999999'">Switch fixture brand</button>
  <Withdrawals v-if="kind==='withdrawals'" :brand-id="brandId" :locale="locale" :display-settings="displaySettings" />
  <Rewards v-else-if="kind==='rewards'" :brand-id="brandId" brand-name="Trial" :locale="locale" :display-settings="displaySettings" />
  <template v-if="kind==='policy'"><button data-testid="switch-fixture-language" class="button" @click="selectedLocale=selectedLocale==='en'?'zh-CN':'en'">Switch fixture language</button><PointPolicy :account="account" :brand-id="brandId" /></template>
 </div>
 <OperationToastHost :locale="selectedLocale" />
 <LoadingOverlay label="Loading…" />
</template>
