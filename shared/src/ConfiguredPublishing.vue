<script setup lang="ts">
import { isPoints, pointsMinor } from './points'

import { useOperationToast } from "@lottery/shared";
import { computed, onUnmounted, ref, shallowRef, watch } from 'vue'
import { vWriteGuard } from './write-guard'
import { withGlobalLoading } from './global-loading'
import { createCatalogI18n, type LocalizedMessage } from './localization'
import { configuredRuleCatalog } from './configured-rule-catalog'
import { configuredAvailabilityDisabled, configuredGroupsForScope, configuredItemState, configuredOptionsForScope, createConfiguredBusinessClient, type ConfiguredAvailability, type ConfiguredOptionStates, type ConfiguredVersion, type ConfiguredSales } from './configured-business'
import { configuredOptionPriceKeys, type ConfiguredSnapshot } from './configured-rules'
import ConfiguredSalesPrices from './ConfiguredSalesPrices.vue'
const props=defineProps<{surface:'platform'|'admin';brandId:string;gameId:string;locale:'en'|'zh-CN';snapshot:ConfiguredSnapshot|null;canWrite?:boolean;oddsOnly?:boolean}>()
const emit=defineEmits<{load:[snapshot:ConfiguredSnapshot];locked:[value:boolean];failure:[cause:unknown];saved:[]}>()
const i18n=createCatalogI18n(configuredRuleCatalog,props.locale)
watch(()=>props.locale,value=>i18n.setLocale(value))
const t=i18n.t,api=computed(()=>createConfiguredBusinessClient(props.surface,props.brandId))
const oddsDraft=shallowRef<ConfiguredSnapshot|null>(null)
const platformSnapshot=computed(()=>props.oddsOnly?oddsDraft.value:props.snapshot)
const current=shallowRef<ConfiguredVersion|null>(null),sales=shallowRef<ConfiguredSales|null>(null)
const availability=shallowRef<ConfiguredAvailability|null>(null)
const availabilityDraft=shallowRef<ConfiguredOptionStates|null>(null)
const oddsAvailability=computed(()=>{
 const value=availability.value,items=availabilityDraft.value
 if(!value||!items)return value
 return value.brand
  ? {...value,brand:{...value.brand,items}}
  : {...value,platform:{...value.platform,items}}
})
const oddsGroup=ref('')
const oddsGroups=computed(()=>current.value&&oddsAvailability.value?configuredGroupsForScope(current.value.snapshot,oddsAvailability.value):[])
watch(oddsGroups,groups=>{
 if(!groups.some(group=>group.id===oddsGroup.value))oddsGroup.value=groups[0]?.id??''
})
const hasEditableOdds=computed(()=>{
 const group=oddsGroups.value.find(item=>item.id===oddsGroup.value)
 return !!group?.plays.some(play=>configuredOptionsForScope(current.value!.snapshot,play,oddsAvailability.value!).some(option=>
  !configuredAvailabilityDisabled(oddsAvailability.value!,play.id,option.id),
 ))
})
const originalPrices=shallowRef<ConfiguredSnapshot['prices']|null>(null)
const currentPrices=computed(()=>props.surface==='admin'?sales.value?.prices:oddsDraft.value?.prices)
const busy=ref(false),ready=ref(false),saved=ref<string|null>(null),error=ref<LocalizedMessage|null>(null)
const pending=shallowRef<{path:string;body:unknown;key:string}|null>(null)
let generation=0
onUnmounted(()=>generation++)
watch([busy,pending],()=>emit('locked',busy.value||!!pending.value),{flush:'sync'})
const limitFields=['min_points','max_points','max_period_points','max_user_period_points'] as const
const salesValid=computed(()=>{
 if(!sales.value)return false
 if(!limitFields.every(key=>isPoints(sales.value![key])&&pointsMinor(sales.value![key])>0n))return false
 const v=sales.value;return pointsMinor(v.max_points)>=pointsMinor(v.min_points)&&pointsMinor(v.max_period_points)>=pointsMinor(v.max_points)&&pointsMinor(v.max_user_period_points)>=pointsMinor(v.max_points)
})
function editLimit(key:typeof limitFields[number],value:string){if(sales.value)sales.value={...sales.value,[key]:value}}
function editPrices(prices:ConfiguredSnapshot['prices']){
 if(props.surface==='admin')sales.value={...sales.value!,prices}
 else oddsDraft.value={...oddsDraft.value!,prices}
}
function setItems(items:{play:string;option:string}[],enabled:boolean){
 const settings=oddsAvailability.value
 if(!availability.value||!settings||busy.value||pending.value||!props.canWrite)return
 const original=(availability.value.brand??availability.value.platform).items
 const next:ConfiguredOptionStates=Object.assign(Object.create(null),availabilityDraft.value??original)
 for(const {play,option} of items){
  if(configuredAvailabilityDisabled(settings,play,option)===!enabled)continue
  const states:Record<string,boolean>=Object.assign(Object.create(null),next[play])
  if(!enabled)states[option]=false
  else if(configuredItemState(original,play,option)===true)states[option]=true
  else delete states[option]
  if(Object.keys(states).length)next[play]=states
  else delete next[play]
 }
 const changed=Object.keys(next).length!==Object.keys(original).length||Object.entries(next).some(([play,options])=>
  Object.keys(options).length!==Object.keys(original[play]??{}).length||
  Object.entries(options).some(([option,state])=>configuredItemState(original,play,option)!==state),
 )
 availabilityDraft.value=changed?next:null
}
const changedPrices=computed(()=>{
 const snapshot=current.value?.snapshot, settings=oddsAvailability.value
 const original=originalPrices.value, prices=currentPrices.value
 if(!snapshot||!settings||!original||!prices||prices===original)return null
 const canEdit=new Set(snapshot.catalog.plays.flatMap(play=>configuredOptionsForScope(snapshot,play,settings)
  .filter(option=>!configuredAvailabilityDisabled(settings,play.id,option.id))
  .flatMap(option=>configuredOptionPriceKeys(snapshot,play,option)),
 ))
 let changed=false
 const plates=Object.fromEntries(Object.entries(prices.plates).map(([plate,values])=>[
  plate,{...original.plates[plate],...Object.fromEntries(Object.entries(values).filter(([key,value])=>{
   if(!canEdit.has(key)||value===original.plates[plate]?.[key])return false
   changed=true
   return true
  }))},
 ]))
 return changed?{...prices,plates}:null
})
const canSavePrices=computed(()=>!!changedPrices.value||(
 props.surface==='admin'&&!!sales.value&&!availabilityDraft.value&&hasEditableOdds.value&&
 (sales.value.version===0||sales.value.rule_id!==current.value?.id)
))
const canSaveConfiguration=computed(()=>
 (props.surface==='platform'?!!platformSnapshot.value:salesValid.value)&&(!props.oddsOnly||canSavePrices.value),
)
const canSave=computed(()=>{
 if(props.oddsOnly&&canSavePrices.value)return canSaveConfiguration.value
 return canSaveConfiguration.value||!!availabilityDraft.value
})
function report(cause:unknown){if((cause as {status?:number}).status===401){emit('failure',cause);return}error.value=i18n.errorMessage(cause)}
async function load(){
 const token=++generation;busy.value=true;ready.value=false;error.value=null;availabilityDraft.value=null
 try{
  const rules=await api.value.rules(props.gameId)
  if(token!==generation)return
  current.value=rules
  if(props.surface==='platform'&&props.oddsOnly)oddsDraft.value=rules?JSON.parse(JSON.stringify(rules.snapshot)):null
  if(rules){
   emit('load',JSON.parse(JSON.stringify(rules.snapshot)))
  }
  if(props.oddsOnly&&rules){
   const value=await api.value.availability(props.gameId)
   if(token!==generation)return
   if(value.rule_id!==rules.id)throw {code:'VERSION_CONFLICT',status:409}
   availability.value=value
  }
  if(props.surface==='admin'&&rules){
   try{const value=await api.value.sales(props.gameId);if(token!==generation)return;sales.value=value}
   catch(cause){if((cause as {code?:string}).code!=='RESOURCE_NOT_FOUND')throw cause;if(token!==generation)return;sales.value={version:0,rule_id:rules.id,enabled:false,prices:JSON.parse(JSON.stringify(rules.snapshot.prices)),min_points:'1',max_points:'100',max_period_points:'10000',max_user_period_points:'5000'}}
  }
  if(props.oddsOnly)originalPrices.value=currentPrices.value??null
  ready.value=true
 }catch(cause){if(token===generation)report(cause)}finally{if(token===generation)busy.value=false}
}
watch(()=>[props.gameId,props.brandId,props.surface],()=>{
 oddsGroup.value='';current.value=null;availability.value=null;sales.value=null;oddsDraft.value=null;originalPrices.value=null;pending.value=null;saved.value=null
 void withGlobalLoading(load)
},{immediate:true})
async function save(){
 if(!props.canWrite||busy.value||!ready.value)return
 if(!pending.value){
  if(!canSave.value)return
  const settings=availability.value, items=availabilityDraft.value
  const scope=settings&&items?{version:(settings.brand??settings.platform).version,items}:null
  let body:unknown
  let endpoint:string
  if(canSaveConfiguration.value){
   const prices=changedPrices.value
   const availabilityInput=scope?{availability:scope}:{}
   if(props.surface==='platform'){
    const snapshot=platformSnapshot.value!
    body={id:current.value?.id??'',revision:current.value?.revision??0,snapshot:prices?{...snapshot,prices}:snapshot,...availabilityInput}
    endpoint='rules'
   }else{
    const sale=sales.value!
    body={...sale,rule_id:current.value!.id,prices:prices??sale.prices,...availabilityInput}
    endpoint='sales'
   }
  }else{
   body={rule_id:settings!.rule_id,...scope}
   endpoint='availability'
  }
  pending.value={path:`/${props.gameId}/configured/${endpoint}`,body:JSON.parse(JSON.stringify(body)),key:crypto.randomUUID()}
 }
 const token=generation;busy.value=true;error.value=null;saved.value=null
 try{
  await api.value.request(pending.value.path,pending.value.body,pending.value.key)
  if(token!==generation)return
  pending.value=null
  await load()
  if(ready.value){
   saved.value=props.oddsOnly?'oddsSettingsSaved':props.surface==='admin'?'salesSaved':'rulesSaved'
   emit('saved')
  }
 }catch(cause){
  if(token!==generation)return
  const status=(cause as {status?:number}).status
  if(status&&status>=400&&status<500){
   pending.value=null
   if(status===409)ready.value=false
   report(cause)
  }
 }finally{if(token===generation)busy.value=false}
}
useOperationToast(saved, key => t(key), 'success');

useOperationToast(error, value => i18n.displayError(value), 'error');
</script>
<template>
 <section class="configured-publishing">
  <p v-if="pending&&!busy" role="status">{{t('unknownWrite')}}</p>

  <p v-if="ready&&!current&&(surface==='admin'||oddsOnly)" class="empty-state">{{t('rulesEmpty')}}</p>

  <template v-if="surface==='admin'&&ready&&current&&sales">
   <h3>{{t(oddsOnly?'salesOdds':'brandSales')}}</h3>
   <p v-if="sales.version===0" class="muted" role="status">{{t('salesUnsaved')}}</p>
   <p v-else-if="sales.rule_id!==current.id" role="status">{{t('salesOutdated')}}</p>
   <fieldset v-if="!oddsOnly&&(canWrite||sales.version>0)" :disabled="busy||!!pending||!canWrite" class="configured-sales-form">
    <label class="sales-toggle">
     <span :class="{'sales-toggle-active':!sales.enabled}">{{t('salesDisabled')}}</span>
     <span class="sales-switch">
      <input type="checkbox" role="switch" :aria-label="t('salesEnabled')" :checked="sales.enabled" @change="sales={...sales,enabled:($event.target as HTMLInputElement).checked}" />
      <span class="sales-switch-track" aria-hidden="true"></span>
     </span>
     <span :class="{'sales-toggle-active':sales.enabled}">{{t('salesEnabled')}}</span>
    </label>
    <div class="configured-sales-limits"><label v-for="field in limitFields" :key="field" class="ui-field-row"><span>{{t(field)}}</span><input inputmode="decimal" :value="sales[field]" @input="editLimit(field,($event.target as HTMLInputElement).value)" /></label></div>
    <p v-if="!salesValid" role="alert">{{t('salesLimitsInvalid')}}</p>
   </fieldset>
  </template>

  <ConfiguredSalesPrices v-if="oddsOnly&&ready&&current&&currentPrices&&oddsAvailability" v-model:group-id="oddsGroup"
   :snapshot="current.snapshot" :availability="oddsAvailability" :model-value="currentPrices" :locale="locale"
   :locked="busy||!!pending" :can-write="!!canWrite" @update:model-value="editPrices" @set-items="setItems" />

  <div class="configured-actions ui-form-actions">
   <button v-if="canWrite&&(!oddsOnly||oddsGroups.length)&&(surface==='platform'||sales)" v-write-guard="busy" type="button" class="button button-primary"
    :disabled="busy||!ready||(!pending&&!canSave)"
    @click="withGlobalLoading(save)">{{t(pending&&!busy?'retryWrite':oddsOnly?'saveOddsSettings':surface==='admin'?'saveSales':'saveRules')}}</button>
   <button v-if="error&&!pending" type="button" class="button button-secondary" :disabled="busy" @click="withGlobalLoading(load)">{{t('retryRead')}}</button>
  </div>
 </section>
</template>
<style scoped>
.configured-publishing{min-width:0;font-size:14px}.configured-publishing fieldset{border:0;padding:0;margin:0 0 12px;min-width:0}.configured-publishing p{margin:0 0 8px}
.configured-sales-limits{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.configured-sales-form label{display:grid;gap:4px}.configured-sales-form .sales-toggle{display:flex;align-items:center;gap:8px;min-height:44px}.sales-toggle{width:fit-content;color:var(--muted);cursor:pointer}.sales-toggle-active{color:var(--ink);font-weight:600}.sales-switch{position:relative;display:flex;align-items:center;flex:0 0 44px;width:44px;height:44px}.sales-switch input{position:absolute;inset:0;width:44px;height:44px;margin:0;opacity:0;cursor:pointer}.sales-switch-track{pointer-events:none;display:block;width:44px;height:24px;box-sizing:border-box;padding:3px;border-radius:999px;background:var(--muted-light)}.sales-switch-track::before{content:'';display:block;width:18px;height:18px;border-radius:50%;background:var(--surface)}.sales-switch input:checked+.sales-switch-track{background:var(--primary)}.sales-switch input:checked+.sales-switch-track::before{transform:translateX(20px)}.sales-switch input:focus-visible+.sales-switch-track{outline:2px solid var(--primary);outline-offset:3px}.sales-toggle:has(input:disabled){cursor:default;opacity:.6}.sales-switch input:disabled{cursor:default}.configured-sales-form input{min-width:0;box-sizing:border-box}.configured-sales-limits input{width:100%}.configured-publishing h3{margin:0 0 8px;font-size:var(--body-font-size)}
@media(max-width:480px){.configured-sales-limits{grid-template-columns:minmax(0,1fr)}}
</style>
