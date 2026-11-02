<script setup lang="ts">
import {provide} from 'vue'
import Domains from '../../admin-web/src/BrandDomains.vue'
import PlatformLottery from '../../shared/src/PlatformLottery.vue'
import Access from '../../platform-web/src/AccessPanel.vue'
import BrandAgents from '../../admin-web/src/AgentManagement.vue'
import BrandAccess from '../../admin-web/src/AccessManagement.vue'
import Members from '../../agent-web/src/pages/MembersPage.vue'
import Children from '../../agent-web/src/pages/ChildrenPage.vue'
import Commission from '../../agent-web/src/pages/CommissionPage.vue'
import {createPlatformI18n,platformI18nKey} from '../../platform-web/src/i18n'
import {createAdminI18n,adminI18nKey} from '../../admin-web/src/i18n'
import {i18n} from '../../agent-web/src/i18n'
import LoadingOverlay from '../../shared/src/LoadingOverlay.vue'
import {setPlatformDisplaySettings} from '../../shared/src/display-settings'
const props=defineProps<{kind:string;locale:'en'|'zh-CN';id:string;readOnly?:boolean}>()
const platform=createPlatformI18n();platform.setLocale(props.locale);provide(platformI18nKey,platform)
const admin=createAdminI18n();admin.setLocale(props.locale);provide(adminI18nKey,admin);i18n.setLocale(props.locale)
setPlatformDisplaySettings({timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'})
const grants=props.readOnly?['admin.view.platform','role.view.platform']:['admin.view.platform','admin.write.platform','admin.delete.platform','role.view.platform','role.write.platform','role.delete.platform']
const brandGrants=props.readOnly?['admin.view.brand','role.view.brand']:['admin.view.brand','admin.write.brand','role.view.brand','role.write.brand']
const agentAccount={id:props.id,super_admin:false,brand_ids:[props.id],permissions:[],permissions_by_brand:{[props.id]:['agent.view.brand','agent_policy.view.brand']}}
const domainAccount={id:props.id,username:'domain_reviewer',super_admin:true,platform_permissions:['brand_domains.view.platform',...(!props.readOnly?['brand_domains.write.platform']:[])],brand_ids:[]}
const account={id:props.id,username:'review_operator',super_admin:false,brand_ids:[props.id],permissions_by_brand:{[props.id]:brandGrants},permissions:brandGrants}
</script>
<template><main class="management-console" style="padding:12px">
 <PlatformLottery v-if="kind==='platform-catalog'||kind==='brand-catalog'" :surface="kind==='platform-catalog'?'platform':'admin'" :brand-id="id" :locale="locale" :display-settings="{timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}" :can-manage="!readOnly" :can-connect="!readOnly" :can-sell="!readOnly" />
 <Domains v-else-if="kind==='domains'" :account="domainAccount" :brand-id="id" />
 <Access v-else-if="kind==='platform-access'" :brand-id="id" :account-id="id" scope="platform" :grants="grants" :locale="locale" />
 <BrandAgents v-else-if="kind==='brand-agents'" :account="agentAccount" :brand-id="id" />
 <BrandAccess v-else-if="kind==='brand-access' || kind==='brand-roles'" :account="account" :brand-id="id" :section="kind==='brand-roles'?'roles':'accounts'" />
 <Members v-else-if="kind==='agent-members'" />
 <Children v-else-if="kind==='agent-children'" />
 <Commission v-else-if="kind==='agent-commission'" />
 <LoadingOverlay :label="locale==='en'?'Loading…':'加载中…'" />
</main></template>
