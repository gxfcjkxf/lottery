<script setup lang="ts">
import { useOperationToast } from "@lottery/shared";
import { computed, onUnmounted, ref, shallowRef, watch } from 'vue'
import { vWriteGuard } from './write-guard'
import { withGlobalLoading } from './global-loading'
import { createCatalogI18n, type LocalizedMessage } from './localization'
import { configuredRuleCatalog } from './configured-rule-catalog'
import { createConfiguredBusinessClient, type ConfiguredVersion, type ConfiguredSales } from './configured-business'
import type { ConfiguredSnapshot } from './configured-rules'
import ConfiguredSalesPrices from './ConfiguredSalesPrices.vue'
const props=defineProps<{surface:'platform'|'admin';brandId:string;gameId:string;locale:'en'|'zh-CN';snapshot:ConfiguredSnapshot|null;canWrite?:boolean}>()
const emit=defineEmits<{load:[snapshot:ConfiguredSnapshot];locked:[value:boolean];failure:[cause:unknown];saved:[]}>()
const i18n=createCatalogI18n(configuredRuleCatalog,props.locale)
watch(()=>props.locale,value=>i18n.setLocale(value))
const t=i18n.t,api=computed(()=>createConfiguredBusinessClient(props.surface,props.brandId))
const current=shallowRef<ConfiguredVersion|null>(null),sales=shallowRef<ConfiguredSales|null>(null)
const pricesOpen=ref(false)
const busy=ref(false),ready=ref(false),saved=ref(false),error=ref<LocalizedMessage|null>(null)
const pending=shallowRef<{path:string;body:unknown;key:string}|null>(null)
let generation=0
onUnmounted(()=>generation++)
watch([busy,pending],()=>emit('locked',busy.value||!!pending.value),{flush:'sync'})
const limitFields=['min_points','max_points','max_period_points','max_user_period_points'] as const
const salesValid=computed(()=>{
 if(!sales.value)return false
 if(!limitFields.every(key=>/^[1-9][0-9]*$/.test(sales.value![key])&&BigInt(sales.value![key])<=9223372036854775807n))return false
 const v=sales.value;return BigInt(v.max_points)>=BigInt(v.min_points)&&BigInt(v.max_period_points)>=BigInt(v.max_points)&&BigInt(v.max_user_period_points)>=BigInt(v.max_points)
})
function editLimit(key:typeof limitFields[number],value:string){if(sales.value)sales.value={...sales.value,[key]:value}}
function editPrices(value:ConfiguredSales['prices']){if(sales.value)sales.value={...sales.value,prices:value}}
function report(cause:unknown){error.value=i18n.errorMessage(cause);if((cause as {status?:number}).status===401)emit('failure',cause)}
async function load(){
 const token=++generation;busy.value=true;ready.value=false;error.value=null
 try{
  const rules=await api.value.rules(props.gameId)
  if(token!==generation)return
  current.value=rules
  if(rules)emit('load',JSON.parse(JSON.stringify(rules.snapshot)))
  if(props.surface==='admin'&&rules){
   try{const value=await api.value.sales(props.gameId);if(token!==generation)return;sales.value=value}
   catch(cause){if((cause as {code?:string}).code!=='RESOURCE_NOT_FOUND')throw cause;if(token!==generation)return;sales.value={version:0,rule_id:rules.id,enabled:false,prices:JSON.parse(JSON.stringify(rules.snapshot.prices)),min_points:'1',max_points:'100',max_period_points:'10000',max_user_period_points:'5000'}}
  }
  ready.value=true
 }catch(cause){if(token===generation)report(cause)}finally{if(token===generation)busy.value=false}
}
watch(()=>[props.gameId,props.brandId,props.surface],()=>{pricesOpen.value=false;current.value=null;sales.value=null;pending.value=null;saved.value=false;void withGlobalLoading(load)},{immediate:true})
async function save(){
 if(!props.canWrite||!ready.value||(!pending.value&&props.surface==='admin'&&!salesValid.value))return
 if(!pending.value){
  const body=props.surface==='platform'?{id:current.value?.id??'',revision:current.value?.revision??0,snapshot:props.snapshot}:sales.value&&current.value?{...sales.value,rule_id:current.value.id}:null
  if(!body||(props.surface==='platform'&&!props.snapshot))return
  pending.value={path:`/${props.gameId}/configured/${props.surface==='platform'?'rules':'sales'}`,body:JSON.parse(JSON.stringify(body)),key:crypto.randomUUID()}
 }
 const token=generation;busy.value=true;error.value=null;saved.value=false
 try{
  await api.value.request(pending.value.path,pending.value.body,pending.value.key)
  if(token!==generation)return
  pending.value=null;await load();saved.value=ready.value;if(ready.value)emit('saved')
 }catch(cause){if(token!==generation)return;const status=(cause as {status?:number}).status;if(status&&status>=400&&status<500){pending.value=null;if(status===409)ready.value=false}report(cause)}finally{if(token===generation)busy.value=false}
}
useOperationToast(saved, () => t(props.surface==='admin'?'salesSaved':'rulesSaved'), 'success');

useOperationToast(error, value => i18n.displayError(value), 'error');
</script>
<template>
 <section class="configured-publishing">

  <p v-if="pending" role="status">{{t('unknownWrite')}}</p>

  <p v-if="ready&&!current&&surface==='admin'" class="empty-state">{{t('rulesEmpty')}}</p>

  <template v-if="surface==='admin'&&ready&&current&&sales">
   <h3>{{t('brandSales')}}</h3>
   <p v-if="sales.version===0" class="muted" role="status">{{t('salesUnsaved')}}</p>
   <p v-else-if="sales.rule_id!==current.id" role="status">{{t('salesOutdated')}}</p>
   <fieldset v-if="canWrite||sales.version>0" :disabled="busy||!!pending||!canWrite" class="configured-sales-form">
    <label class="sales-toggle">
     <span :class="{'sales-toggle-active':!sales.enabled}">{{t('salesDisabled')}}</span>
     <span class="sales-switch">
      <input type="checkbox" role="switch" :aria-label="t('salesEnabled')" :checked="sales.enabled" @change="sales={...sales,enabled:($event.target as HTMLInputElement).checked}" />
      <span class="sales-switch-track" aria-hidden="true"></span>
     </span>
     <span :class="{'sales-toggle-active':sales.enabled}">{{t('salesEnabled')}}</span>
    </label>
    <div class="configured-sales-limits"><label v-for="field in limitFields" :key="field">{{t(field)}}<input inputmode="numeric" :value="sales[field]" @input="editLimit(field,($event.target as HTMLInputElement).value)" /></label></div>
    <p v-if="!salesValid" role="alert">{{t('salesLimitsInvalid')}}</p>
   </fieldset>
   <button v-if="canWrite||sales.version>0" type="button" class="button button-secondary configured-odds-entry" :disabled="busy||!!pending" @click="pricesOpen=true">{{t('salesOdds')}}</button>
  </template>

  <ConfiguredSalesPrices v-if="pricesOpen&&current&&sales" :snapshot="current.snapshot" :model-value="sales.prices" :locale="locale" :disabled="busy||!!pending||!canWrite" @update:model-value="editPrices" @close="pricesOpen=false" />
  <div class="configured-actions ui-form-actions">
   <button v-if="canWrite&&(surface==='platform'||sales)" v-write-guard="busy" type="button" class="button button-primary" :disabled="busy||!ready||(surface==='platform'&&!snapshot)||(surface==='admin'&&!pending&&!salesValid)" @click="withGlobalLoading(save)">{{t(pending?'retryWrite':surface==='admin'?'saveSales':'saveRules')}}</button>
   <button v-if="error&&!pending" type="button" class="button button-secondary" :disabled="busy" @click="withGlobalLoading(load)">{{t('retryRead')}}</button>
  </div>
 </section>
</template>
<style scoped>
.configured-odds-entry{margin-bottom:12px}.configured-publishing{min-width:0;font-size:14px}.configured-publishing fieldset{border:0;padding:0;margin:0 0 12px;min-width:0}.configured-publishing p{margin:0 0 8px}
.configured-sales-limits{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.configured-sales-form label{display:grid;gap:4px}.configured-sales-form .sales-toggle{display:flex;align-items:center;gap:8px;min-height:44px}.sales-toggle{width:fit-content;color:var(--muted);cursor:pointer}.sales-toggle-active{color:var(--ink);font-weight:600}.sales-switch{position:relative;display:flex;align-items:center;flex:0 0 44px;width:44px;height:44px}.sales-switch input{position:absolute;inset:0;width:44px;height:44px;margin:0;opacity:0;cursor:pointer}.sales-switch-track{pointer-events:none;display:block;width:44px;height:24px;box-sizing:border-box;padding:3px;border-radius:999px;background:var(--muted-light)}.sales-switch-track::before{content:'';display:block;width:18px;height:18px;border-radius:50%;background:var(--surface)}.sales-switch input:checked+.sales-switch-track{background:var(--primary)}.sales-switch input:checked+.sales-switch-track::before{transform:translateX(20px)}.sales-switch input:focus-visible+.sales-switch-track{outline:2px solid var(--primary);outline-offset:3px}.sales-toggle:has(input:disabled){cursor:default;opacity:.6}.sales-switch input:disabled{cursor:default}.configured-sales-form input{min-width:0;box-sizing:border-box}.configured-sales-limits input{width:100%}.configured-publishing h3{margin:0 0 8px;font-size:var(--body-font-size)}
@media(max-width:480px){.configured-sales-limits{grid-template-columns:minmax(0,1fr)}}
</style>
