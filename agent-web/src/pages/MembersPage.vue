<script setup lang="ts">
import DirectorySearch from "@lottery/shared/DirectorySearch.vue"
import ConsoleTabs from "@lottery/shared/ConsoleTabs.vue"
import SummaryFacts from "@lottery/shared/SummaryFacts.vue"
import ManagementTable from "@lottery/shared/ManagementTable.vue"
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import PaginationControls from '@lottery/shared/PaginationControls.vue'
import {formatDisplayDateTime,vGlobalLoading,withGlobalLoading} from '@lottery/shared'
import {request,AgentError} from '../api'
import {i18n} from '../i18n'
import type {Member,MemberPage,MemberScope} from '../types'
defineProps<{pick?:boolean}>()
const emit=defineEmits<{bets:[member:Member,scope:MemberScope];'session-invalid':[]}>()
const {t,locale}=i18n
const scope=ref<MemberScope>('direct'),search=ref(''),offset=ref(0),data=ref<MemberPage|null>(null),loading=ref(false),error=ref<unknown>(null)
const errorText=computed(()=>i18n.errorText(error.value));let generation=0
async function load(){const ticket=++generation;loading.value=true;error.value=null;try{const result=await request<MemberPage>(`/members?scope=${scope.value}&limit=20&offset=${offset.value}&q=${encodeURIComponent(search.value)}`);if(ticket===generation)data.value=result}catch(e){if(ticket!==generation)return;error.value=e;if(e instanceof AgentError&&e.status===401)emit('session-invalid')}finally{if(ticket===generation)loading.value=false}}
async function setScope(value:MemberScope){scope.value=value;offset.value=0;data.value=null;await load()}
onMounted(load);onBeforeUnmount(()=>generation++)
</script>
<template>
 <section class="panel console-panel console-stack member-directory">
  <p v-if="pick" class="muted">{{t('selectMember')}}</p>
  <ConsoleTabs :items="(['direct','line'] as const).map(id=>({id,label:t(id)}))" :value="scope" :disabled="loading" :select="setScope" />
  <DirectorySearch v-model="search" :label="t('account')" :query-label="t('search')" :clear-label="t('clear')" :busy="loading" :active="Boolean(search)" :search="async()=>{offset=0;await load()}" :clear="async()=>{search='';offset=0;await load()}" />
  <SummaryFacts class="console-facts--cards" v-if="data&&!pick" :fields="(['member_count','available_points','bet_count','stake_points','unsettled_points','net_points'] as const).map(key=>({label:t(key),value:data!.summary[key]}))" />
  <p v-global-loading v-if="loading" role="status">{{t('loading')}}</p><p v-if="error" class="message error" role="alert">{{errorText}}</p>
  <ManagementTable table-class="member-directory-table" :entries="data?.items??[]" :columns="[{key:'account',label:t('account'),cellClass:'member-cell'},{key:'name',label:t('name')},{key:'owner',label:t('ownerAgent')},{key:'phone',label:t('phone')},{key:'points',label:t('points')},{key:'joined',label:t('joined')},{key:'actions',label:t('details')}]" :empty="!loading&&!error&&data?.items.length===0" :empty-label="t('empty')"><template #account="{entry}"><button type="button" class="member-account" @click="emit('bets',entry,scope)">{{entry.username}}</button></template><template #name="{entry}">{{entry.display_name}}</template><template #owner="{entry}">{{entry.agent_username}} · {{t(entry.is_direct?'direct':'descendant')}}</template><template #phone="{entry}">{{entry.phone}}</template><template #points="{entry}">{{entry.available_points}}</template><template #joined="{entry}">{{formatDisplayDateTime(entry.joined_at,locale)}}</template><template #actions="{entry}"><button type="button" class="text-button" @click="emit('bets',entry,scope)">{{t('viewBets')}}</button></template></ManagementTable>
  
  <PaginationControls :summary="t('totalRecords',{count:data?.total_count??'0'})" :previous-label="t('previous')" :next-label="t('next')" :busy="loading" :has-previous="offset>0" :has-next="Boolean(data)&&BigInt(offset+20)<BigInt(data!.total_count)" :previous="async()=>{offset-=20;await load()}" :next="async()=>{offset+=20;await load()}" />
 </section>
</template>
