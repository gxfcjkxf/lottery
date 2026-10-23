<script setup lang="ts">
import SummaryFacts from "@lottery/shared/SummaryFacts.vue"
import ManagementTable from "@lottery/shared/ManagementTable.vue"
import {ref,onMounted,onBeforeUnmount,computed} from 'vue'
import PaginationControls from '@lottery/shared/PaginationControls.vue'
import {vGlobalLoading,formatDisplayDateTime} from '@lottery/shared'
import {request,AgentError} from '../api'
import {i18n} from '../i18n'
import type {Page,Wallet} from '../types'
interface Posting{id:string;created_at:string;entry_type:string;points:string}
interface Commission extends Page<Posting>{credited_points:string;debited_points:string}
const {t,locale}=i18n,emit=defineEmits<{'session-invalid':[]}>()
const data=ref<Commission|null>(null),wallet=ref<Wallet|null>(null),offset=ref(0),loading=ref(false),error=ref<unknown>(null),errorText=computed(()=>i18n.errorText(error.value));let generation=0
async function load(){const ticket=++generation;loading.value=true;error.value=null;try{const [postings,balance]=await Promise.all([request<Commission>(`/commission?limit=20&offset=${offset.value}`),request<Wallet>('/wallet')]);if(ticket===generation){data.value=postings;wallet.value=balance}}catch(e){if(ticket!==generation)return;error.value=e;if(e instanceof AgentError&&e.status===401)emit('session-invalid')}finally{if(ticket===generation)loading.value=false}}
function label(posting:Posting){if(posting.entry_type==='commission_adjustment'||posting.entry_type==='adjust')return t('commissionAdjustment');if(posting.entry_type==='commission_correction')return t('commissionCorrection');return t(BigInt(posting.points)>0?'commissionCredit':'commissionDebit')}
onMounted(load);onBeforeUnmount(()=>generation++)
</script>
<template><section class="panel console-panel console-stack commission-page"><SummaryFacts class="console-facts--cards" :fields="[{label:t('commission'),value:wallet?.commission_points},{label:t('credited'),value:data?.credited_points},{label:t('debited'),value:data?.debited_points}]" /><p v-global-loading v-if="loading" role="status">{{t('loading')}}</p><p v-if="error" class="message error" role="alert">{{errorText}}</p><ManagementTable :entries="data?.items??[]" :columns="[{key:'date',label:t('postedAt')},{key:'type',label:t('details')},{key:'points',label:t('amount')}]" :empty="!loading&&!error&&data?.items.length===0" :empty-label="t('empty')"><template #date="{entry}">{{formatDisplayDateTime(entry.created_at,locale)}}</template><template #type="{entry}">{{label(entry)}}</template><template #points="{entry}">{{BigInt(entry.points)>0?'+':''}}{{entry.points}}</template></ManagementTable><PaginationControls :summary="t('totalRecords',{count:data?.total_count??'0'})" :previous-label="t('previous')" :next-label="t('next')" :busy="loading" :has-previous="offset>0" :has-next="Boolean(data)&&BigInt(offset+20)<BigInt(data!.total_count)" :previous="async()=>{offset-=20;await load()}" :next="async()=>{offset+=20;await load()}" /></section></template>
