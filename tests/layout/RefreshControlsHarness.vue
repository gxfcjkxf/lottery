<script setup lang="ts">
import {provide} from 'vue'
import CommissionCycles from '../../admin-web/src/CommissionCyclesManagement.vue'
import {createAdminI18n,adminI18nKey} from '../../admin-web/src/i18n'
import {setPlatformDisplaySettings} from '../../shared/src/display-settings'
import AccessPanel from '../../platform-web/src/AccessPanel.vue'
import ArchivesPanel from '../../platform-web/src/ArchivesPanel.vue'
import CommissionPanel from '../../platform-web/src/CommissionPanel.vue'
import NotificationsPanel from '../../platform-web/src/NotificationsPanel.vue'
import NotificationTemplatesPanel from '../../platform-web/src/NotificationTemplatesPanel.vue'
import RewardsPanel from '../../platform-web/src/RewardsPanel.vue'
import AgentCommission from '../../agent-web/src/pages/CommissionPage.vue'
import AgentChildren from '../../agent-web/src/pages/ChildrenPage.vue'
import AgentMembers from '../../agent-web/src/pages/MembersPage.vue'
import {createPlatformI18n,platformI18nKey} from '../../platform-web/src/i18n'
import {i18n as agentI18n} from '../../agent-web/src/i18n'
const props=defineProps<{component:string;locale:'en'|'zh-CN';componentProps:Record<string,unknown>}>()
const components={CommissionCycles,AccessPanel,ArchivesPanel,CommissionPanel,NotificationsPanel,NotificationTemplatesPanel,RewardsPanel,AgentCommission,AgentChildren,AgentMembers}
const platformI18n=createPlatformI18n();platformI18n.setLocale(props.locale);provide(platformI18nKey,platformI18n)
agentI18n.setLocale(props.locale)
const adminI18n=createAdminI18n();adminI18n.setLocale(props.locale);provide(adminI18nKey,adminI18n)
setPlatformDisplaySettings({timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'})
function failed(cause:unknown){document.documentElement.dataset.refreshFailure=String(cause)}
</script>
<template><main class="management-console" style="padding:12px;min-width:0"><component :is="components[component as keyof typeof components]" v-bind="componentProps" @failure="failed" /></main></template>
