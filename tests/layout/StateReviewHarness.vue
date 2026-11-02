<script setup lang="ts">
import { provide, ref, watch } from 'vue'
import DetailModal from '../../shared/src/DetailModal.vue'
import LoadingOverlay from '../../shared/src/LoadingOverlay.vue'
import OperationToastHost from '../../shared/src/OperationToastHost.vue'
import {useOperationToast} from '../../shared/src/operation-toast'
import { createCatalogI18n, type LocalizedMessage } from '../../shared/src/localization'
import { withGlobalLoading } from '../../shared/src/global-loading'
import { setInlineValidationLocale } from '../../shared/src/form-validation'
import MembersPage from '../../agent-web/src/pages/MembersPage.vue'
import { i18n as agentI18n } from '../../agent-web/src/i18n'
import RechargePanel from '../../user-web/src/RechargePanel.vue'
import { createUserI18n, userI18nKey } from '../../user-web/src/i18n'
import AccessManagement from '../../admin-web/src/AccessManagement.vue'
import { createAdminI18n, adminI18nKey } from '../../admin-web/src/i18n'
import BetOrdersPanel from '../../platform-web/src/BetOrdersPanel.vue'
import { createPlatformI18n, platformI18nKey } from '../../platform-web/src/i18n'
const props = defineProps<{surface:'user'|'brand'|'platform'|'agent';mode:'read'|'shared'}>()
const id='aaaaaaaa-aaaa-4aaa-8aaa-000000000001'
const locale=ref<'en'|'zh-CN'>('en'), user=createUserI18n(),admin=createAdminI18n(),platform=createPlatformI18n()
provide(userI18nKey,user);provide(adminI18nKey,admin);provide(platformI18nKey,platform)
const i18n=createCatalogI18n({en:{title:'Confirm account settings',close:'Close confirmation',confirm:'Confirm and save these account settings',cancel:'Return to account settings',loading:'Saving account settings…',saved:'Settings for account “{account}” saved successfully.',notice:'Account settings saved'},'zh-CN':{title:'确认账号设置变更',close:'关闭确认弹窗',confirm:'确认并保存当前账号设置',cancel:'返回账号设置继续修改',loading:'正在保存账号设置…',saved:'账号「{account}」的设置已保存。',notice:'账号设置已保存'}},'en')
watch(locale,value=>{user.setLocale(value);admin.setLocale(value);platform.setLocale(value);agentI18n.setLocale(value);i18n.setLocale(value);setInlineValidationLocale(value)},{immediate:true})
const dialog=ref(false),busy=ref(false),notice=ref<LocalizedMessage|null>(null)
async function save(){busy.value=true;try{await withGlobalLoading(async()=>{const response=await fetch('/api/state-save',{method:'POST'});if(!response.ok)throw Error('Synthetic save failed');await response.json();notice.value=i18n.message('saved',{account:'review_account'});dialog.value=false})}finally{busy.value=false}}
useOperationToast(notice, value => i18n.displayError(value), 'success')
</script>
<template>
 <main class="state-review-harness" style="padding:16px;min-width:0">
  <div id="user-page-refresh" />
  <button id="state-language" type="button" class="button button-secondary" @click="locale=locale==='en'?'zh-CN':'en'">{{locale}}</button>
  <template v-if="mode==='read'">
   <MembersPage v-if="surface==='agent'" />
   <RechargePanel v-else-if="surface==='user'" :locale="locale" :member="{id,brand_id:id}" />
   <AccessManagement section="roles" v-else-if="surface==='brand'" :brand-id="id" :account="{id,username:'state_reviewer',super_admin:false,brand_ids:[id],permissions:[],permissions_by_brand:{[id]:['role.view.brand']}}" />
   <BetOrdersPanel v-else :brand-id="id" :game-id="id" :locale="locale" :display-settings="{timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}" />
  </template>
  <template v-else>
   <button id="state-open" type="button" class="button button-primary" @click="dialog=true">{{i18n.t('title')}}</button>
   <DetailModal v-if="dialog" :title="i18n.t('title')" :close-label="i18n.t('close')" :close-disabled="busy" @close="dialog=false">
    <label>Account<input value="review_account" readonly /></label>
    <dl><div><dt>Account</dt><dd>review_account</dd></div><div><dt>Change</dt><dd>{{'Long account setting description '.repeat(12)}}</dd></div></dl>
    <template #footer><button id="state-save" type="button" class="button button-primary" :disabled="busy" @click="save">{{i18n.t('confirm')}}</button></template>
   </DetailModal>
  </template>
  <OperationToastHost :locale="locale" />
  <LoadingOverlay :label="i18n.t('loading')" />
 </main>
</template>
