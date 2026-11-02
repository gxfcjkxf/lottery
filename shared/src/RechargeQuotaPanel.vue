<script lang="ts">
import SelectControl from './SelectControl.vue'
const pendingQuotaChanges = new Map<string,{brand:string;points:string;key:string;kind:'grant'|'reclaim'}>()
</script>
<script setup lang="ts">
import ManagementList from "./ManagementList.vue"
import { readPaginationTotal } from './pagination'

import TablePagination from './TablePagination.vue'
import { useOperationToast } from "@lottery/shared";
import DetailModal from './DetailModal.vue'
import { formValidationText } from './form-validation'
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { vWriteGuard, vInlineValidation, withGlobalLoading, vRefreshButton } from './index'
import { createCatalogI18n } from './localization'
import { rechargeQuotaCatalog } from './recharge-quota-catalog'
import { readApiEnvelope } from './api-response'
import { publicApiErrorText } from './public-api-errors'
import { formatDisplayDateTime } from './display-settings'
const totalCount = ref('0')
const props=defineProps<{brandId:string;locale:'en'|'zh-CN';basePath:string;actorId?:string;summaryOnly?:boolean;compact?:boolean;canGrant?:boolean;externalFeedback?:boolean}>()
const emit=defineEmits<{failure:[cause:unknown];updated:[quota:{brand_id:string;available:string}]}>()
const i18n=createCatalogI18n(rechargeQuotaCatalog,props.locale),t=i18n.t
watch(()=>props.locale,v=>i18n.setLocale(v))
interface Entry {id:string;brand_id:string;kind:'grant'|'reclaim'|'manual_recharge'|'manual_adjustment';points:string;before_available:string;after_available:string;actor_name:string;created_at:string}
const readFailed=ref(false),historyOpen=ref(false),historyTrigger=ref<HTMLButtonElement|null>(null)
const kind=ref<'grant'|'reclaim'>('grant'),savedKind=ref<'grant'|'reclaim'>('grant')
const available=ref<string|null>(null),entries=ref<Entry[]>([]),amount=ref(''),offset=ref(0),more=ref(false),busy=ref(false),error=ref<unknown>(null),saved=ref(false)
const pending=ref<{brand:string;points:string;key:string;kind:'grant'|'reclaim'}|null>(null)
const validAmount=computed(()=>/^[1-9]\d*$/.test(amount.value)&&BigInt(amount.value)<=9223372036854775807n)
const exceedsAvailable=computed(()=>kind.value==='reclaim' && validAmount.value && available.value!==null && BigInt(amount.value)>BigInt(available.value))
let generation=0
class QuotaError extends Error { constructor(message:string,readonly status:number,readonly code:string){super(message)} }
async function request<T>(brand:string,path:string,body?:{points:string},key?:string):Promise<T>{
 let response:Response
 try {response=await fetch(`${props.basePath}/recharge-quota${path}`,{method:body?'POST':'GET',credentials:'same-origin',headers:{'X-Brand-ID':brand,Accept:'application/json',...(body?{'Content-Type':'application/json','Idempotency-Key':key!}:{})},...(body?{body:JSON.stringify(body)}:{})})} catch {throw new QuotaError('Network request failed',0,'NETWORK_ERROR')}
 return (await readApiEnvelope<T>(response,(m,s,c)=>new QuotaError(m,s,c))).data
}
async function load(page=0){
 const ticket=++generation,brand=props.brandId;error.value=null;readFailed.value=false
 if(!brand)return
 busy.value=true
 try{
  const [quota,history]=await Promise.all([request<{brand_id:string;available:string}>(brand,''),(props.summaryOnly || (props.compact && !historyOpen.value)) ? Promise.resolve(null) : request<{items:Entry[];total_count:string}>(brand,`/entries?limit=11&offset=${page}`)])
  if(ticket!==generation)return
  if(quota.brand_id!==brand||!/^\d+$/.test(quota.available)||(history!==null&&(!Array.isArray(history.items)||history.items.some(e=>e.brand_id!==brand||!['grant','reclaim','manual_recharge','manual_adjustment'].includes(e.kind)))))throw new QuotaError('Invalid quota response',502,'INVALID_RESPONSE')
  available.value=quota.available;emit('updated',quota);if(history){[entries.value,totalCount.value]=[history.items.slice(0,10),readPaginationTotal(history)];more.value=history.items.length>10};offset.value=page
 }catch(cause){if(ticket===generation){error.value=cause;readFailed.value=true;emit('failure',cause)}}finally{if(ticket===generation)busy.value=false}
}
async function changeQuota(){
 if(busy.value||!props.canGrant||!props.brandId||!props.actorId)return
 if(!pending.value){if(!validAmount.value||exceedsAvailable.value)return;pending.value={brand:props.brandId,points:amount.value,key:crypto.randomUUID(),kind:kind.value}}
 readFailed.value=false
 const frozen=pending.value;pendingQuotaChanges.set(`${props.actorId}:${frozen.brand}`,frozen);busy.value=true;error.value=null;saved.value=false
 try{
  const quota=await request<{brand_id:string;available:string}>(frozen.brand,frozen.kind==='grant'?'/grants':'/reclaims',{points:frozen.points},frozen.key)
  if(quota.brand_id!==frozen.brand||! /^(0|[1-9]\d*)$/.test(quota.available)||BigInt(quota.available)>9223372036854775807n)throw new QuotaError('Invalid quota receipt',502,'INVALID_RESPONSE')
  if(props.brandId!==frozen.brand)return
  pending.value=null;pendingQuotaChanges.delete(`${props.actorId}:${frozen.brand}`);amount.value='';saved.value=true;savedKind.value=frozen.kind;emit('updated',quota);await load()
 }catch(cause){
  if(props.brandId!==frozen.brand)return
  if(cause instanceof QuotaError&&cause.status>=400&&cause.status<500){pending.value=null;pendingQuotaChanges.delete(`${props.actorId}:${frozen.brand}`)}
  error.value=cause;readFailed.value=false;emit('failure',cause)
 }finally{if(props.brandId===frozen.brand)busy.value=false}
}
async function closeHistory(){historyOpen.value=false;await nextTick();historyTrigger.value?.focus()}
async function openHistory(){historyOpen.value=true;await withGlobalLoading(load)}
watch(()=>props.brandId,()=>{available.value=null;[entries.value,totalCount.value]=[[],'0'];more.value=false;offset.value=0;historyOpen.value=false;pending.value=pendingQuotaChanges.get(`${props.actorId}:${props.brandId}`)??null;amount.value=pending.value?.points??'';kind.value=pending.value?.kind??'grant';saved.value=false;void withGlobalLoading(load)},{immediate:true})
onBeforeUnmount(()=>{generation++})
defineExpose({reload:()=>load(offset.value)})
useOperationToast(saved, () => t(savedKind.value==='grant'?'saved':'reclaimed'), 'success');
</script>
<template>
 <div :class="summaryOnly || compact ? 'recharge-quota-summary' : ''" data-testid="recharge-quota">
  <div v-if="compact" class="quota-inline">
   <button ref="historyTrigger" type="button" class="button button-secondary" :disabled="busy || available===null" @click="withGlobalLoading(openHistory)">{{t('viewHistory')}}</button>
   <button v-refresh-button="t('reload')" v-if="error && !historyOpen" type="button" class="text-button" :disabled="busy" @click="withGlobalLoading(load)">{{t('reload')}}</button>
  </div>
  <component :is="compact ? DetailModal : 'section'" v-if="!compact || historyOpen" v-bind="compact ? {title:t('history'),closeLabel:t('close')} : {}" :class="compact ? '' : summaryOnly ? 'recharge-quota-summary' : 'panel console-panel console-stack recharge-quota'" @close="closeHistory">
<div class="console-stack" :class="{'recharge-quota':compact}">
  <h2 v-if="!summaryOnly && !compact">{{t('title')}}</h2>
  <span v-if="summaryOnly && available!==null" class="muted">{{t('availablePoints')}}{{new Intl.NumberFormat(locale).format(BigInt(available))}}</span>
  <div v-if="!summaryOnly && available!==null" class="console-facts"><span>{{t('available')}}</span><strong>{{new Intl.NumberFormat(locale).format(BigInt(available))}}</strong></div>
  
  <p v-if="error&&!externalFeedback" class="message error" role="alert">{{readFailed ? i18n.displayError(i18n.errorMessage(error,'read')) : publicApiErrorText(error,locale)}}</p>
  <button v-refresh-button="t('reload')" v-if="error" type="button" class="button button-secondary" :disabled="busy" @click="withGlobalLoading(load,offset)">{{t('reload')}}</button>
  <form v-if="canGrant&&available!==null" v-write-guard="busy" novalidate v-inline-validation class="quota-form ui-form" @submit.prevent="withGlobalLoading(changeQuota)">
   <label>{{t('operation')}}<SelectControl v-model="kind" :label="t('operation')" :disabled="busy||!!pending" :options="[{ value: 'grant', label: (String(t('grant'))).trim() }, { value: 'reclaim', label: (String(t('reclaim'))).trim() }]" /></label>
   <label>{{t(kind==='grant'?'amount':'reclaimAmount')}}<input v-model.trim="amount" inputmode="numeric" required :disabled="busy||!!pending" pattern="[1-9][0-9]*" :data-validation-format="formValidationText('positiveInteger')" /></label>
   <p v-if="amount&&!validAmount&&!pending" class="form-error" role="alert">{{t('invalidAmount')}}</p>
   <button type="submit" class="button button-primary" :disabled="busy||(!pending&&(!validAmount||exceedsAvailable))">{{t(pending?'retry':kind==='grant'?'grant':'reclaim')}}</button>
  </form>
  <p v-if="canGrant && kind==='reclaim'" class="muted">{{t('reclaimHelp')}}</p>
  <p v-if="exceedsAvailable && !pending" class="form-error" role="alert">{{t('reclaimExceeded')}}</p>

  <component :is="compact ? 'div' : 'details'" v-if="available!==null && !summaryOnly"><summary v-if="!compact">{{t('history')}}</summary>
   <ManagementList embedded><div class="table-wrap"><table class="console-table console-responsive-table"><thead><tr><th>{{t('method')}}</th><th>{{t('points')}}</th><th>{{t('before')}}</th><th>{{t('after')}}</th><th>{{t('actor')}}</th><th>{{t('time')}}</th></tr></thead><tbody>
    <tr v-for="entry in entries" :key="entry.id"><td :data-label="t('method')">{{t(entry.kind==='grant'?'grantKind':entry.kind==='reclaim'?'reclaimKind':entry.kind)}}</td><td :data-label="t('points')">{{entry.points}}</td><td :data-label="t('before')">{{entry.before_available}}</td><td :data-label="t('after')">{{entry.after_available}}</td><td :data-label="t('actor')">{{entry.actor_name}}</td><td :data-label="t('time')">{{formatDisplayDateTime(entry.created_at,locale)}}</td></tr>
    <tr v-if="!entries.length"><td colspan="6" class="empty-state">{{t('empty')}}</td></tr>
   </tbody></table></div>
   <template #pagination><TablePagination :show-empty="available!==null && !busy && !readFailed" :total="totalCount" :page="Math.floor(offset / 10 + 1)" :count="entries.length" :locale="locale" :page-size="10" :previous-disabled="busy||offset===0" :next-disabled="busy||!more" :previous-label="t('previous')" :next-label="t('next')" :go-to="(target: number) => load((target - 1) * 10)" /></template></ManagementList>
  </component>
 </div>
 </component>
 </div>
</template>
<style scoped>
.quota-inline {display:flex;flex-wrap:wrap;align-items:center;gap:4px 12px;}
.recharge-quota-summary {min-width:0;max-width:100%;font-size:var(--body-font-size);line-height:var(--control-height);overflow-wrap:anywhere;}
.recharge-quota h2 {margin:0;font-size:16px;}
.recharge-quota .console-facts {display:flex;flex-wrap:wrap;gap:8px 16px;align-items:baseline;}
.recharge-quota strong {font-size:24px;font-variant-numeric:tabular-nums;overflow-wrap:anywhere;}
.recharge-quota p {margin:0;font-size:var(--body-font-size);color:var(--muted);}
.quota-form {display:flex;flex-wrap:wrap;gap:12px;align-items:end;}
.quota-form label {display:grid;gap:6px;min-width:0;max-width:100%;font-size:var(--body-font-size);}
.quota-form input {width:220px;max-width:100%;box-sizing:border-box;}
.recharge-quota details {min-width:0;}
.recharge-quota summary {cursor:pointer;min-height:var(--control-height);display:flex;align-items:center;font-size:var(--body-font-size);font-weight:600;}
</style>
