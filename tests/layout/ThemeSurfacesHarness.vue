<script setup lang="ts">
import {provide} from 'vue'
import AccessManagement from '../../admin-web/src/AccessManagement.vue'
import MemberProvision from '../../admin-web/src/MemberProvision.vue'
import AccessPanel from '../../platform-web/src/AccessPanel.vue'
import ManagementOverview from '../../shared/src/ManagementOverview.vue'
import {createAdminI18n,adminI18nKey} from '../../admin-web/src/i18n'
import {createPlatformI18n,platformI18nKey} from '../../platform-web/src/i18n'
import type {OverviewSummary} from '../../shared/src/overview'
const props=defineProps<{client:'brand'|'platform'}>()
const admin=createAdminI18n(),platform=createPlatformI18n()
admin.setLocale('zh-CN');platform.setLocale('zh-CN')
provide(adminI18nKey,admin);provide(platformI18nKey,platform)
const i18n=props.client==='brand'?admin:platform
const id='aaaaaaaa-aaaa-4aaa-8aaa-000000000001'
const grants=['admin.view','admin.write','role.view','role.write'].map(p=>`${p}.${props.client==='brand'?'brand':'platform'}`)
const account={id,username:'theme_reviewer',status:'active',super_admin:false,brand_ids:[id],permissions:[],permissions_by_brand:{[id]:[...grants,'user.create.brand']}}
const settings={theme:'jade' as const,timezone:'Asia/Singapore',date_format:'YYYY-MM-DD' as const,time_format:'HH:mm:ss' as const}
const summary:OverviewSummary={snapshot_at:'2026-11-12T00:00:00Z',timezone:'Asia/Singapore',today:'2026-11-12',total_members:'10',online_members:'2',unsettled_points:'20',available_points:'100',frozen_points:'10',withdrawal_points:'10',withdrawal_count:'1',yesterday_same_time_stake:'20',days:Array.from({length:7},(_,i)=>({day:`2026-11-${i+6}`,stake_points:'100',net_points:i===6?'-10':'20',order_count:'2',bettor_count:'2',refund_points:'0',new_members:'1'})),brands:[{brand_id:id,brand_name:'Theme review',stake_points:'100',net_points:'-10'}]}
</script>
<template>
<div class="management-console app-shell"><main class="content" style="margin:0;min-width:0;padding:12px">
<ManagementOverview :can-view="true" :locale="i18n.locale.value" :scope="client" :display-settings="settings" :read="async()=>summary" :t="i18n.t" :error-message="i18n.errorMessage" :display-error="i18n.display" />
<section v-if="client==='brand'" class="panel palette-legacy-surfaces">
 <span class="profile-avatar">R</span><span class="game-avatar mint">6</span>
 <div class="simulation-result"><span class="success-check">✓</span><div class="sim-stats"><b>100</b></div></div>
 <div class="agent-alert"><b>状态</b></div><div class="withdraw-note">100 USDT</div>
 <div class="save-status">已保存</div><div class="modal-hint">状态</div>
</section>
<MemberProvision v-if="client==='brand'" :account="account" :brand-id="id" />
<AccessManagement v-if="client==='brand'" :account="account" :brand-id="id" section="roles" />
<AccessPanel v-else :brand-id="''" locale="zh-CN" scope="platform" :account-id="id" :grants="grants" initial-tab="roles" />
</main></div>
</template>
