<script setup lang="ts">
import BettingHistoryTable from "./BettingHistoryTable.vue"
import { readPaginationTotal } from './pagination'
const totalCount = ref('0')

import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { createCatalogI18n } from './localization'
import { configuredRuleCatalog } from './configured-rule-catalog'
import { createApiClient } from './api'
import type { ConfiguredOrderSummary } from './configured-business'
import ManagementList from './ManagementList.vue'
import TablePagination from './TablePagination.vue'
import {withGlobalLoading} from './global-loading'
import {vRefreshButton} from './refresh-button'
const pageSize = ref(50)

const props=defineProps<{surface:'admin'|'platform';brandId:string;gameId?:string;locale:'en'|'zh-CN'}>()
const emit=defineEmits<{failure:[cause:unknown]}>()
const i18n=createCatalogI18n(configuredRuleCatalog,props.locale)
watch(()=>props.locale,value=>i18n.setLocale(value))
const t=i18n.t
const orders=shallowRef<ConfiguredOrderSummary[]>([]),offset=ref(0),busy=ref(false),failed=ref(false),loaded=ref(false)
let request=0
async function load(next=0){
 const id=++request,brand=props.brandId;busy.value=true;failed.value=false
 try{
  const query=new URLSearchParams({limit:String(pageSize.value),offset:String(next)});if(props.gameId)query.set('game_id',props.gameId)
  const out=await createApiClient().request<{items:ConfiguredOrderSummary[];total_count:string}>(`/api/v1/${props.surface}/configured-orders?${query}`,{headers:{'X-Brand-ID':brand}})
  if(id===request){[orders.value,totalCount.value]=[out.items,readPaginationTotal(out)];offset.value=next;loaded.value=true}
 }catch(cause){if(id===request){failed.value=true;emit('failure',cause)}}finally{if(id===request)busy.value=false}
}
const historyItems=computed(()=>orders.value.map(order=>({id:order.id,kind:'configured' as const,game_name:order.game_name,member_name:order.member_name,period_no:order.period_no,placed_at:order.placed_at,order})))
const more=computed(()=>BigInt(offset.value+orders.value.length)<BigInt(totalCount.value))
watch(()=>[props.brandId,props.gameId,props.surface],()=>{orders.value=[];totalCount.value='0';offset.value=0;loaded.value=false;void withGlobalLoading(load)},{immediate:true,flush:'sync'})
onBeforeUnmount(()=>{request++})
</script>
<template>
 <ManagementList class="configured-orders" :busy="busy">
  <template v-if="failed || orders.some(order => order.status === 'placed')" #actions><button v-refresh-button="t('reloadOrders')" class="button button-secondary" type="button" :disabled="busy" @click="withGlobalLoading(load,offset)">{{t('reloadOrders')}}</button></template>
  <p v-if="failed" role="alert">{{t('ordersFailed')}}</p>
  <BettingHistoryTable :items="historyItems" :show-game="!gameId" show-member :locale="locale" :empty="loaded && !busy && !failed && !orders.length" />
  <template #pagination><TablePagination :total="totalCount" :page="Math.floor(offset / pageSize + 1)" :count="orders.length" :locale="locale" :aria-label="t('ordersTitle')" :page-size="pageSize" :previous-disabled="busy || offset===0" :next-disabled="busy || !more" :previous-label="t('previousOrders')" :next-label="t('nextOrders')" :go-to="(target: number) => load((target - 1) * pageSize)" :change-page-size="async (size: number) => { pageSize = size; await load(0) }" :busy="busy" /></template>
 </ManagementList>
</template>
<style scoped>
.configured-orders{min-width:0}
</style>
