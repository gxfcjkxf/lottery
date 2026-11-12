<script setup lang="ts">
import { readPaginationTotal } from '@lottery/shared'
const betTotal = ref('0')

import ManagementList from "@lottery/shared/ManagementList.vue"
import { useOperationToast } from "@lottery/shared";
import SummaryFacts from "@lottery/shared/SummaryFacts.vue"
import {ref,onMounted,onBeforeUnmount,watch} from 'vue'
import {vRefreshButton,vGlobalLoading,withGlobalLoading} from '@lottery/shared'
import PaginationControls from '@lottery/shared/PaginationControls.vue'
import BettingHistoryTable from '@lottery/shared/BettingHistoryTable.vue'
import {request,AgentError} from '../api'
import {i18n} from '../i18n'
import type {Member,MemberScope,Bet,Wallet} from '../types'
const pageSize = ref(20)

const props=defineProps<{memberId:string|null;scope:MemberScope}>(),emit=defineEmits<{clear:[];'session-invalid':[]}>(),{t,locale}=i18n
const info=ref<{member:Member;wallet:Wallet;net_points:string;unsettled_points:string}|null>(null),bets=ref<Bet[]>([]),cursor=ref<string|null>(null),loading=ref(false),error=ref<unknown>(null);const pageIndex=ref(0),cursors=ref<Array<string|null>>([null]);let generation=0
async function load(index=0){if(!props.memberId)return;const ticket=++generation;loading.value=true;error.value=null;try{const path=`/members/${encodeURIComponent(props.memberId)}`;if(index===0){const summary=await request<NonNullable<typeof info.value>>(path+`/summary?scope=${props.scope}`);if(ticket!==generation)return;info.value=summary}const page=await request<{items:Bet[];total_count:string;next_cursor:string|null}>(path+`/bets?scope=${props.scope}&limit=${pageSize.value}${cursors.value[index]?'&cursor='+encodeURIComponent(cursors.value[index]!):''}`);if(ticket===generation){[bets.value,betTotal.value]=[page.items,readPaginationTotal(page)];pageIndex.value=index;cursor.value=page.next_cursor;cursors.value=cursors.value.slice(0,index+1);if(page.next_cursor)cursors.value.push(page.next_cursor)}}catch(e){if(ticket!==generation)return;if(e instanceof AgentError&&e.status===401){emit('session-invalid');return}error.value=e}finally{if(ticket===generation)loading.value=false}}
watch(()=>[props.memberId,props.scope],()=>{generation++;info.value=null;[bets.value,betTotal.value]=[[],'0'];cursor.value=null;pageIndex.value=0;cursors.value=[null];void load()});onMounted(()=>load());onBeforeUnmount(()=>generation++)

useOperationToast(error, value => i18n.errorText(value), 'error');
</script>
<template><ManagementList v-if="memberId" :busy="loading" class="member-bets"><template #heading><h2>{{info?.member.username}}</h2></template><template #actions><button type="button" class="button button-secondary" @click="emit('clear')">{{t('selectMember')}}</button><button v-refresh-button="t('retryRead')" v-if="error" type="button" class="button button-secondary" :disabled="loading" @click="withGlobalLoading(load, pageIndex)">{{t('retryRead')}}</button></template><p v-global-loading v-if="loading" role="status">{{t('loadingBets')}}</p><template v-if="info" #summary><SummaryFacts v-if="info" :fields="[{label:t('points'),value:info.wallet.available_points},{label:t('profit'),value:info.net_points},{label:t('pending'),value:info.unsettled_points}]" /></template><BettingHistoryTable v-if="bets.length" :items="bets" :locale="locale" show-game /><p v-if="!loading&&!error&&!bets.length" class="empty-state">{{t('emptyBets')}}</p><template #pagination><PaginationControls :total="betTotal" :page="Math.floor(pageIndex + 1)" :count="bets.length" :locale="locale" :previous-label="t('previous')" :next-label="t('next')" :busy="loading" :has-previous="pageIndex>0" :has-next="Boolean(cursor)"  :page-size="pageSize" :go-to="async target => { await load(target - 1) }" :reachable-page="pageIndex + (cursor ? 2 : 1)" :change-page-size="async (size: number) => { pageSize = size; cursors = [null]; cursor = null; await load(0) }" /></template></ManagementList></template>
