<script setup lang="ts">
import {provide} from 'vue'
import BrandPresentation from '../../admin-web/src/BrandPresentation.vue'
import WithdrawalPolicySettings from '../../admin-web/src/WithdrawalPolicySettings.vue'
import DataRetentionPanel from '../../platform-web/src/DataRetentionPanel.vue'
import AutomationSettings from '../../shared/src/AutomationSettings.vue'
import BettingPreferencesSettings from '../../user-web/src/BettingPreferencesSettings.vue'
import {createAdminI18n,adminI18nKey} from '../../admin-web/src/i18n'
import {createPlatformI18n,platformI18nKey} from '../../platform-web/src/i18n'
import {setPlatformDisplaySettings,setInlineValidationLocale} from '../../shared/src/index'
const props=defineProps<{view:'presentation'|'withdrawal'|'archive'|'preferences'|'automation';locale:'en'|'zh-CN'}>()
const admin=createAdminI18n();admin.setLocale(props.locale);provide(adminI18nKey,admin)
const platform=createPlatformI18n();platform.setLocale(props.locale);provide(platformI18nKey,platform)
setInlineValidationLocale(props.locale)
setPlatformDisplaySettings({timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'})
const brand='aaaaaaaa-aaaa-4aaa-8aaa-000000000001',actor='bbbbbbbb-bbbb-4bbb-8bbb-000000000001'
const permissions=['brand_presentation.view.brand','brand_presentation.write.brand','withdrawal_policy.view.brand','withdrawal_policy.write.brand','game.view.brand']
const account={id:actor,super_admin:false,brand_ids:[brand],permissions:[],permissions_by_brand:{[brand]:permissions}}
</script>
<template>
 <main :class="view==='preferences'?'page-actions-user':'management-console'" style="padding:12px;min-width:0">
  <BrandPresentation v-if="view==='presentation'" :account="account" :brand-id="brand" />
  <WithdrawalPolicySettings v-else-if="view==='withdrawal'" :account="account" :brand-id="brand" />
  <DataRetentionPanel v-else-if="view==='archive'" :account-id="actor" :brand-id="brand" can-view can-manage :locale="locale" :display-settings="{timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}" />
  <AutomationSettings v-else-if="view==='automation'" surface="platform" :game-id="brand" brand-id="" :locale="locale" can-write />
  <BettingPreferencesSettings v-else :brand-id="brand" :member-id="actor" :locale="locale" />
 </main>
</template>
