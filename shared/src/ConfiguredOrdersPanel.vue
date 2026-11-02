<script setup lang="ts">
import { readPaginationTotal } from './pagination'
const totalCount = ref('0')

import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { createCatalogI18n } from './localization'
import { configuredRuleCatalog } from './configured-rule-catalog'
import { createApiClient } from './api'
import type { ConfiguredLiveOrder } from './configured-business'
import ManagementList from './ManagementList.vue'
import TablePagination from './TablePagination.vue'
import ManagementTable from './ManagementTable.vue'
import {configuredTicketLabel,configuredTicketPrices} from './configured-business'
import {formatDisplayDateTime} from './display-settings'
import {withGlobalLoading} from './global-loading'
import {vRefreshButton} from './refresh-button'
const props=defineProps<{surface:'admin'|'platform';brandId:string;gameId?:string;locale:'en'|'zh-CN'}>()
const emit=defineEmits<{failure:[cause:unknown]}>()
const i18n=createCatalogI18n(configuredRuleCatalog,props.locale)
watch(()=>props.locale,value=>i18n.setLocale(value))
const t=i18n.t
const orders=shallowRef<ConfiguredLiveOrder[]>([]),offset=ref(0),busy=ref(false),failed=ref(false),loaded=ref(false)
let request=0
async function load(next=0){
 const id=++request,brand=props.brandId;busy.value=true;failed.value=false
 try{
  const query=new URLSearchParams({limit:'50',offset:String(next)});if(props.gameId)query.set('game_id',props.gameId)
  const out=await createApiClient().request<{items:ConfiguredLiveOrder[];total_count:string}>(`/api/v1/${props.surface}/configured-orders?${query}`,{headers:{'X-Brand-ID':brand}})
  if(id===request){[orders.value,totalCount.value]=[out.items,readPaginationTotal(out)];offset.value=next;loaded.value=true}
 }catch(cause){if(id===request){failed.value=true;emit('failure',cause)}}finally{if(id===request)busy.value=false}
}
const columns=computed(()=>[...(!props.gameId?[{key:"game",label:t("orderGame")}]:[]),{key:"member",label:t("orderMember")},{key:"period",label:t("periodLabel")},{key:"content",label:t("play")},{key:"odds",label:t("prices")},{key:"stake",label:t("bet")},{key:"prize",label:t("prize")},{key:"status",label:t("orderStatus")},{key:"time",label:t("orderTime")}])
function label(order:ConfiguredLiveOrder,index:number){return configuredTicketLabel(order.snapshot,order.quote.lines[index]!,props.locale)}
function prices(order:ConfiguredLiveOrder,index:number){return configuredTicketPrices(order.snapshot,order.quote.lines[index]!,props.locale).join(' / ')}
const more=computed(()=>BigInt(offset.value+orders.value.length)<BigInt(totalCount.value))
watch(()=>[props.brandId,props.gameId,props.surface],()=>{orders.value=[];totalCount.value='0';offset.value=0;loaded.value=false;void withGlobalLoading(load)},{immediate:true})
onBeforeUnmount(()=>{request++})
</script>
<template>
 <ManagementList class="configured-orders" :busy="busy">
  <template #actions><button v-refresh-button="t('reloadOrders')" class="button button-secondary" type="button" :disabled="busy" @click="withGlobalLoading(load,offset)">{{t('reloadOrders')}}</button></template>
  <p v-if="failed" role="alert">{{t('ordersFailed')}}</p>
  <ManagementTable :entries="orders" :columns="columns" :empty="loaded && !busy && !failed && !orders.length" :empty-label="t('ordersEmpty')">
   <template #game="{entry}">{{entry.game_name}}</template>
   <template #member="{entry}">{{entry.member_name}}</template>
   <template #period="{entry}">{{entry.period_no}}</template>
   <template #content="{entry}"><p v-for="(_,index) in entry.quote.lines" :key="index">{{label(entry,index)}}<template v-if="entry.result?.lines[index]"> · {{t(entry.result.lines[index]!.outcome)}}</template></p><p v-if="entry.quote.reserved_points">{{t('reserved')}} {{entry.quote.reserved_points}}</p><p v-if="entry.result?.consumed_points !== undefined">{{t('consumed')}} {{entry.result!.consumed_points}}</p><p v-if="entry.result">{{t(entry.snapshot.catalog.funding==='reserve'?'release':'refund')}} {{entry.result.refund_points}}</p></template>
   <template #odds="{entry}"><p v-for="(_,index) in entry.quote.lines" :key="index">{{prices(entry,index)}}</p></template>
   <template #stake="{entry}">{{entry.quote.bet_points}}</template>
   <template #prize="{entry}">{{entry.result?entry.result.prize_points:'—'}}</template>
   <template #status="{entry}">{{t(entry.status==='settled'?'state.settled':'state.placed')}}</template>
   <template #time="{entry}"><time :datetime="entry.placed_at">{{formatDisplayDateTime(entry.placed_at,locale)}}</time></template>
  </ManagementTable>
  <template #pagination><TablePagination :show-empty="loaded" :total="totalCount" :page="Math.floor(offset / 50 + 1)" :count="orders.length" :locale="locale" :aria-label="t('ordersTitle')" :page-size="50" :previous-disabled="busy || offset===0" :next-disabled="busy || !more" :previous-label="t('previousOrders')" :next-label="t('nextOrders')" :go-to="(target: number) => load((target - 1) * 50)" /></template>
 </ManagementList>
</template>
<style scoped>
.configured-orders{min-width:0}.configured-orders :deep(td){font-variant-numeric:tabular-nums;overflow-wrap:anywhere}.configured-orders p{margin:0}.configured-orders p+p{margin-top:4px}
@media(min-width:901px){.configured-orders time{white-space:nowrap}}
</style>
