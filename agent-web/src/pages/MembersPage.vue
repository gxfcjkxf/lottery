<script setup lang="ts">
import MemberActionsMenu from "@lottery/shared/MemberActionsMenu.vue"
import ManagementList from "@lottery/shared/ManagementList.vue"
import { useOperationToast } from "@lottery/shared";
import DirectorySearch from "@lottery/shared/DirectorySearch.vue"
import ConsoleTabs from "@lottery/shared/ConsoleTabs.vue"
import SummaryFacts from "@lottery/shared/SummaryFacts.vue"
import ManagementTable from "@lottery/shared/ManagementTable.vue"
import { ref, onMounted, onBeforeUnmount } from 'vue'
import PaginationControls from '@lottery/shared/PaginationControls.vue'
import {formatDisplayDateTime,vRefreshButton,vGlobalLoading,withGlobalLoading} from '@lottery/shared'
import {request,AgentError} from '../api'
import {i18n} from '../i18n'
import type {Member,MemberPage,MemberScope} from '../types'
const emit=defineEmits<{bets:[member:Member,scope:MemberScope];'session-invalid':[]}>()
const {t,locale}=i18n
const scope=ref<MemberScope>('direct'),search=ref(''),appliedSearch=ref(''),offset=ref(0),data=ref<MemberPage|null>(null),loading=ref(false),error=ref<unknown>(null)
let generation=0
let lastRequest={offset:0,scope:'direct' as MemberScope,query:''}
async function load(requestedOffset=offset.value,requestedScope=scope.value,requestedSearch=appliedSearch.value){
 const ticket=++generation;lastRequest={offset:requestedOffset,scope:requestedScope,query:requestedSearch};loading.value=true;error.value=null
 try{const result=await request<MemberPage>(`/members?scope=${requestedScope}&limit=20&offset=${requestedOffset}&q=${encodeURIComponent(requestedSearch)}`);if(ticket===generation){data.value=result;offset.value=requestedOffset;scope.value=requestedScope;appliedSearch.value=requestedSearch}}
 catch(e){if(ticket!==generation)return;error.value=e;if(e instanceof AgentError&&e.status===401)emit('session-invalid')}
 finally{if(ticket===generation)loading.value=false}
}
async function setScope(value:MemberScope){await load(0,value)}
async function retryRead(){await load(lastRequest.offset,lastRequest.scope,lastRequest.query)}
onMounted(load);onBeforeUnmount(()=>generation++)

useOperationToast(error, value => i18n.errorText(value), 'error');
</script>
<template>
 <ManagementList :busy="loading" class="member-directory">
  <template v-if="$slots.actions" #actions><slot name="actions" /></template>
  <template #tabs><ConsoleTabs :items="(['direct','line'] as const).map(id=>({id,label:t(id)}))" :value="scope" :disabled="loading" :select="setScope" /></template>
  <template #filters><DirectorySearch v-model="search" :label="t('account')" :query-label="t('search')" :clear-label="t('clear')" :busy="loading" :active="Boolean(search || appliedSearch)" :search="async()=>{await load(0,scope,search)}" :clear="async()=>{search='';await load(0,scope,'')}" /></template>
  <template v-if="data" #summary><SummaryFacts v-if="data" :fields="(['member_count','available_points','bet_count','stake_points','unsettled_points','net_points'] as const).map(key=>({label:t(key),value:data!.summary[key]}))" /></template>
  <p v-global-loading v-if="loading" role="status">{{t('loadingMembers')}}</p><button v-refresh-button="t('retryRead')" v-if="error" type="button" class="button button-secondary" :disabled="loading" @click="withGlobalLoading(retryRead)">{{t('retryRead')}}</button>
  <ManagementTable table-class="member-directory-table" :entries="data?.items??[]" :columns="[{key:'account',label:t('account'),cellClass:'member-cell'},{key:'name',label:t('name')},{key:'owner',label:t('ownerAgent')},{key:'phone',label:t('phone')},{key:'points',label:t('points'),cellClass:'member-points'},{key:'joined',label:t('joined')},{key:'actions',label:t('actions')}]" :empty="!loading&&!error&&data?.items.length===0" :empty-label="t('emptyMembers')"><template #account="{entry}"><button type="button" class="member-account" @click="emit('bets',entry,scope)">{{entry.username}}</button></template><template #name="{entry}">{{entry.display_name || '—'}}</template><template #owner="{entry}">{{entry.agent_username}} · {{t(entry.is_direct?'direct':'descendant')}}</template><template #phone="{entry}">{{entry.phone || '—'}}</template><template #points="{entry}">{{entry.available_points}}</template><template #joined="{entry}">{{formatDisplayDateTime(entry.joined_at,locale)}}</template><template #actions="{entry}"><MemberActionsMenu :id="'agent-member-actions-'+entry.id" :label="t('actions')"><button type="button" role="menuitem" @click="emit('bets',entry,scope)">{{t('viewBets')}}</button></MemberActionsMenu></template></ManagementTable>

  
 <template #pagination><PaginationControls :total="data?.total_count ?? '0'" :page="Math.floor(offset / 20 + 1)" :count="data?.items.length ?? 0" :locale="locale" :previous-label="t('previous')" :next-label="t('next')" :busy="loading" :has-previous="offset>0" :has-next="Boolean(data)&&BigInt(offset+20)<BigInt(data!.total_count)"  :page-size="20" :go-to="async target => { await load((target - 1) * 20) }" /></template></ManagementList>
</template>
