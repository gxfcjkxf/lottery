<script lang="ts">
import SelectControl from './SelectControl.vue'
const pendingQuotaChanges = new Map<string,{brand:string;points:string;key:string;kind:'grant'|'reclaim'}>()
</script>
<script setup lang="ts">
import { isPoints, pointsMinor, formatPoints } from "./points";
import DetailModal from './DetailModal.vue'
import ManagementList from "./ManagementList.vue"
import { readPaginationTotal } from './pagination'

import TablePagination from './TablePagination.vue'
import { useOperationToast } from "@lottery/shared";
import { formValidationText } from './form-validation'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { vWriteGuard, vInlineValidation, withGlobalLoading, vRefreshButton } from './index'
import { createCatalogI18n } from './localization'
import { rechargeQuotaCatalog } from './recharge-quota-catalog'
import { readApiEnvelope } from './api-response'
import { publicApiErrorText } from './public-api-errors'
import { formatDisplayDateTime } from './display-settings'
const pageSize = ref(10)

const totalCount = ref('0')
const props=defineProps<{brandId:string;locale:'en'|'zh-CN';basePath:string;actorId?:string;summaryOnly?:boolean;standalone?:boolean;canGrant?:boolean;externalFeedback?:boolean}>()
const emit=defineEmits<{failure:[cause:unknown];updated:[quota:{brand_id:string;available:string}]}>()
const i18n=createCatalogI18n(rechargeQuotaCatalog,props.locale),t=i18n.t
watch(()=>props.locale,v=>i18n.setLocale(v))
interface Entry {id:string;brand_id:string;kind:'grant'|'reclaim'|'manual_recharge'|'manual_adjustment';points:string;before_available:string;after_available:string;member_id:string|null;member_account:string|null;actor_name:string;created_at:string}
const operationOpen=ref(false)
const readFailed=ref(false)
const kind=ref<'grant'|'reclaim'>('grant'),savedKind=ref<'grant'|'reclaim'>('grant')
const available=ref<string|null>(null),entries=ref<Entry[]>([]),amount=ref(''),offset=ref(0),more=ref(false),busy=ref(false),error=ref<unknown>(null),saved=ref(false)
const pending=ref<{brand:string;points:string;key:string;kind:'grant'|'reclaim'}|null>(null)
const validAmount=computed(()=>isPoints(amount.value)&&pointsMinor(amount.value)>0n)
const exceedsAvailable=computed(()=>kind.value==='reclaim' && validAmount.value && available.value!==null && pointsMinor(amount.value)>pointsMinor(available.value))
let generation=0,alive=true
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
  const [quota,history]=await Promise.all([request<{brand_id:string;available:string}>(brand,''),props.summaryOnly ? Promise.resolve(null) : request<{items:Entry[];total_count:string}>(brand,`/entries?limit=${pageSize.value}&offset=${page}`)])
  if(ticket!==generation)return
  if(quota.brand_id!==brand||!isPoints(quota.available)||(history!==null&&(!Array.isArray(history.items)||history.items.some(e=>e.brand_id!==brand||!['grant','reclaim','manual_recharge','manual_adjustment'].includes(e.kind)||(['manual_recharge','manual_adjustment'].includes(e.kind)&&(!e.member_id||!e.member_account))))))throw new QuotaError('Invalid quota response',502,'INVALID_RESPONSE')
  available.value=quota.available;emit('updated',quota);if(history){[entries.value,totalCount.value]=[history.items,readPaginationTotal(history)];more.value=BigInt(page + history.items.length)<BigInt(totalCount.value)};offset.value=page
 }catch(cause){if(ticket===generation){error.value=cause;readFailed.value=true;emit('failure',cause)}}finally{if(ticket===generation)busy.value=false}
}
async function changeQuota(){
 if(busy.value||!props.canGrant||!props.brandId||!props.actorId)return
 if(!pending.value){if(!validAmount.value||exceedsAvailable.value)return;pending.value={brand:props.brandId,points:amount.value,key:crypto.randomUUID(),kind:kind.value}}
 readFailed.value=false
 const actor=props.actorId,base=props.basePath;const current=()=>alive&&props.actorId===actor&&props.basePath===base&&props.brandId===frozen.brand
 const frozen=pending.value;pendingQuotaChanges.set(`${props.actorId}:${frozen.brand}`,frozen);busy.value=true;error.value=null;saved.value=false
 try{
  const quota=await request<{brand_id:string;available:string}>(frozen.brand,frozen.kind==='grant'?'/grants':'/reclaims',{points:frozen.points},frozen.key)
  if(quota.brand_id!==frozen.brand||!isPoints(quota.available))throw new QuotaError('Invalid quota receipt',502,'INVALID_RESPONSE')
  pendingQuotaChanges.delete(`${actor}:${frozen.brand}`)
  if(!current())return
  pending.value=null;amount.value='';operationOpen.value=false;saved.value=true;savedKind.value=frozen.kind;emit('updated',quota);await load()
 }catch(cause){
  if(cause instanceof QuotaError&&cause.status>=400&&cause.status<500)pendingQuotaChanges.delete(`${actor}:${frozen.brand}`)
  if(!current())return
  if(cause instanceof QuotaError&&cause.status>=400&&cause.status<500)pending.value=null
  error.value=cause;readFailed.value=false;emit('failure',cause)
 }finally{if(current())busy.value=false}
}
watch(()=>[props.brandId,props.actorId,props.basePath],()=>{operationOpen.value=false;available.value=null;[entries.value,totalCount.value]=[[],'0'];more.value=false;offset.value=0;pending.value=pendingQuotaChanges.get(`${props.actorId}:${props.brandId}`)??null;amount.value=pending.value?.points??'';kind.value=pending.value?.kind??'grant';saved.value=false;void withGlobalLoading(load)},{immediate:true,flush:'sync'})
onBeforeUnmount(()=>{alive=false;generation++})
defineExpose({reload:()=>load(offset.value)})
useOperationToast(saved, () => t(savedKind.value==='grant'?'saved':'reclaimed'), 'success');
</script>
<template>
 <component :is="summaryOnly ? 'section' : ManagementList" :busy="busy" :class="summaryOnly ? 'recharge-quota-summary' : 'panel console-panel console-stack recharge-quota'" data-testid="recharge-quota">
  <span v-if="summaryOnly && available!==null" class="muted">{{t('availablePoints')}}{{formatPoints(available,locale)}}</span>
  <template v-if="!summaryOnly" #heading>
   <h2 v-if="!standalone">{{t('title')}}</h2>
   <div v-if="available!==null" class="console-facts"><span>{{t('available')}}</span><strong>{{formatPoints(available,locale)}}</strong></div>
  </template>

  <p v-if="error&&!externalFeedback" class="message error" role="alert">{{readFailed ? i18n.displayError(i18n.errorMessage(error,'read')) : publicApiErrorText(error,locale)}}</p>
  <button v-refresh-button="t('reload')" v-if="error" type="button" class="button button-secondary" :disabled="busy" @click="withGlobalLoading(load,offset)">{{t('reload')}}</button>
  <template v-if="canGrant&&available!==null&&!summaryOnly" #actions>
   <button type="button" class="button button-primary" :disabled="busy" @click="($event.currentTarget as HTMLButtonElement).focus();operationOpen=true">{{t('operation')}}</button>
  </template>
  <DetailModal v-if="operationOpen&&canGrant" class="ui-form-dialog" :title="t('operation')" :close-label="t('close')" :close-disabled="busy" @close="operationOpen=false">
   <div v-if="available!==null" class="console-facts"><span>{{t('available')}}</span><strong>{{formatPoints(available,locale)}}</strong></div>
  <form v-write-guard="busy" novalidate v-inline-validation class="quota-form ui-form" @submit.prevent="withGlobalLoading(changeQuota)">
   <label>{{t('operation')}}<SelectControl v-model="kind" :label="t('operation')" :disabled="busy||!!pending" :options="[{ value: 'grant', label: (String(t('grant'))).trim() }, { value: 'reclaim', label: (String(t('reclaim'))).trim() }]" /></label>
   <label>{{t(kind==='grant'?'amount':'reclaimAmount')}}<input v-model.trim="amount" inputmode="decimal" required :disabled="busy||!!pending" pattern="(0|[1-9][0-9]*)([.][0-9]{1,2})?" :data-validation-format="formValidationText('positivePoints')" /></label>
   <p v-if="amount&&!validAmount&&!pending" class="form-error" role="alert">{{t('invalidAmount')}}</p>
  <p v-if="exceedsAvailable && !pending" class="form-error" role="alert">{{t('reclaimExceeded')}}</p>
   <button type="submit" class="button button-primary" :disabled="busy||(!pending&&(!validAmount||exceedsAvailable))">{{t(pending?'retry':kind==='grant'?'grant':'reclaim')}}</button>
  </form>
  </DetailModal>

   <div v-if="available!==null && !summaryOnly" class="table-wrap"><table class="console-table console-responsive-table"><thead><tr><th>{{t('method')}}</th><th>{{t('source')}}</th><th>{{t('destination')}}</th><th>{{t('points')}}</th><th>{{t('before')}}</th><th>{{t('after')}}</th><th>{{t('actor')}}</th><th>{{t('time')}}</th></tr></thead><tbody>
    <tr v-for="entry in entries" :key="entry.id"><td :data-label="t('method')">{{t(entry.kind==='grant'?'grantKind':entry.kind==='reclaim'?'reclaimKind':entry.kind)}}</td><td :data-label="t('source')">{{t(entry.kind==='grant'?'platform':'brandQuota')}}</td><td :data-label="t('destination')">{{entry.kind==='grant'?t('brandQuota'):entry.kind==='reclaim'?t('platform'):t('memberAccount',{account:entry.member_account!})}}</td><td :data-label="t('points')">{{entry.points}}</td><td :data-label="t('before')">{{entry.before_available}}</td><td :data-label="t('after')">{{entry.after_available}}</td><td :data-label="t('actor')">{{entry.actor_name}}</td><td :data-label="t('time')">{{formatDisplayDateTime(entry.created_at,locale)}}</td></tr>
    <tr v-if="!entries.length"><td colspan="8" class="empty-state">{{t('empty')}}</td></tr>
   </tbody></table></div>
   <template v-if="!summaryOnly" #pagination><TablePagination :total="totalCount" :page="Math.floor(offset / pageSize + 1)" :count="entries.length" :locale="locale" :page-size="pageSize" :previous-disabled="busy||offset===0" :next-disabled="busy||!more" :previous-label="t('previous')" :next-label="t('next')" :go-to="(target: number) => load((target - 1) * pageSize)" :change-page-size="async (size: number) => { pageSize = size; await load(0) }" :busy="busy" /></template>
 </component>
</template>
<style scoped>
.recharge-quota-summary {min-width:0;max-width:100%;font-size:var(--body-font-size);line-height:var(--control-height);overflow-wrap:anywhere;}
.recharge-quota h2 {margin:0;font-size:16px;}
.recharge-quota .console-facts {display:flex;flex-wrap:wrap;gap:8px 16px;align-items:baseline;}
.recharge-quota strong {font-size:24px;font-variant-numeric:tabular-nums;overflow-wrap:anywhere;}
.recharge-quota p {margin:0;font-size:var(--body-font-size);color:var(--muted);}
.quota-form {display:grid;gap:12px;}
.recharge-quota p.form-error {color:var(--danger-text);}
.quota-form label {display:grid;gap:6px;min-width:0;max-width:100%;font-size:var(--body-font-size);}
.quota-form input {width:100%;max-width:100%;box-sizing:border-box;}
.recharge-quota details {min-width:0;}
.recharge-quota summary {cursor:pointer;min-height:var(--control-height);display:flex;align-items:center;font-size:var(--body-font-size);font-weight:600;}
</style>
