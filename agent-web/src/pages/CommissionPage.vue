<script setup lang="ts">
import { pointsMinor } from '@lottery/shared'

import ManagementList from "@lottery/shared/ManagementList.vue"
import { useOperationToast } from "@lottery/shared";
import SummaryFacts from "@lottery/shared/SummaryFacts.vue"
import ManagementTable from "@lottery/shared/ManagementTable.vue"
import {ref,onMounted,onBeforeUnmount,computed} from 'vue'
import PaginationControls from '@lottery/shared/PaginationControls.vue'
import {vGlobalLoading,vRefreshButton,withGlobalLoading,formatDisplayDateTime} from '@lottery/shared'
import {request,AgentError} from '../api'
import {i18n} from '../i18n'
import type {Page,Wallet} from '../types'
const pageSize = ref(20)

interface Posting{id:string;created_at:string;entry_type:string;points:string}
interface Commission extends Page<Posting>{credited_points:string;debited_points:string}
const {t,locale}=i18n,emit=defineEmits<{'session-invalid':[]}>()
const data=ref<Commission|null>(null),wallet=ref<Wallet|null>(null),offset=ref(0),loading=ref(false),loadingWallet=ref(false)
const postingError=ref<unknown>(null),walletError=ref<unknown>(null)
const error=computed(()=>postingError.value??walletError.value)
let generation=0,postingTicket=0,walletTicket=0
async function loadPostings(requestedOffset=offset.value){
 const scope=generation,ticket=++postingTicket;loading.value=true;postingError.value=null
 try{const result=await request<Commission>(`/commission?limit=${pageSize.value}&offset=${requestedOffset}`);if(scope===generation&&ticket===postingTicket){data.value=result;offset.value=requestedOffset}}
 catch(cause){if(scope===generation&&ticket===postingTicket){if(cause instanceof AgentError&&cause.status===401){emit('session-invalid');return}postingError.value=cause}}
 finally{if(scope===generation&&ticket===postingTicket)loading.value=false}
}
async function loadWallet(){
 const scope=generation,ticket=++walletTicket;loadingWallet.value=true;walletError.value=null
 try{const result=await request<Wallet>('/wallet');if(scope===generation&&ticket===walletTicket)wallet.value=result}
 catch(cause){if(scope===generation&&ticket===walletTicket){wallet.value=null;if(cause instanceof AgentError&&cause.status===401){emit('session-invalid');return}walletError.value=cause}}
 finally{if(scope===generation&&ticket===walletTicket)loadingWallet.value=false}
}
async function refresh(){await Promise.all([loadPostings(),loadWallet()])}
function label(posting:Posting){if(posting.entry_type==='commission_adjustment'||posting.entry_type==='adjust')return t('commissionAdjustment');if(posting.entry_type==='commission_correction')return t('commissionCorrection');return t(pointsMinor(posting.points)>0?'commissionCredit':'commissionDebit')}
onMounted(refresh);onBeforeUnmount(()=>generation++)

useOperationToast(error, value => i18n.errorText(value), 'error');
</script>
<template><ManagementList :busy="loading||loadingWallet" class="commission-page"><template v-if="error" #actions><button v-refresh-button="t('retryRead')" type="button" class="button button-secondary" :disabled="loading||loadingWallet" @click="withGlobalLoading(refresh)">{{t('retryRead')}}</button></template><template #summary><SummaryFacts :fields="[{label:t('commission'),value:wallet ? wallet.commission_points : '—'},{label:t('credited'),value:data?.credited_points},{label:t('debited'),value:data?.debited_points}]" /></template><p v-global-loading v-if="loading" role="status">{{t('loadingCommission')}}</p><ManagementTable :entries="data?.items??[]" :columns="[{key:'date',label:t('postedAt')},{key:'type',label:t('details')},{key:'points',label:t('amount'),cellClass:'member-points'}]" :empty="!loading&&!error&&data?.items.length===0" :empty-label="t('emptyCommission')"><template #date="{entry}">{{formatDisplayDateTime(entry.created_at,locale)}}</template><template #type="{entry}">{{label(entry)}}</template><template #points="{entry}">{{pointsMinor(entry.points)>0?'+':''}}{{entry.points}}</template></ManagementTable><template #pagination><PaginationControls :total="data?.total_count ?? '0'" :page="Math.floor(offset / pageSize + 1)" :count="data?.items.length ?? 0" :locale="locale" :previous-label="t('previous')" :next-label="t('next')" :busy="loading" :has-previous="offset>0" :has-next="Boolean(data)&&BigInt(offset+pageSize)<BigInt(data!.total_count)"  :page-size="pageSize" :go-to="async target => { await loadPostings((target - 1) * pageSize) }" :change-page-size="async (size: number) => { pageSize = size; await loadPostings(0) }" /></template></ManagementList></template>
