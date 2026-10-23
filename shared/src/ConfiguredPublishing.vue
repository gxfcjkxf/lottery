<script setup lang="ts">
import BusinessDetails from './BusinessDetails.vue'
import { vWriteGuard } from "@lottery/shared";
import { computed, onUnmounted, ref, shallowRef, watch } from 'vue'
import { createCatalogI18n, type LocalizedMessage } from './localization'
import { withGlobalLoading } from './global-loading'
import { configuredRuleCatalog } from './configured-rule-catalog'
import { createConfiguredBusinessClient,configuredTicketLabel,configuredTicketPrices,type ConfiguredLiveOrder, type ConfiguredVersion, type ConfiguredSales, type ConfiguredCase } from './configured-business'
import type { ConfiguredSnapshot, RuleNode } from './configured-rules'
import MarkSixOrders from './MarkSixOrders.vue'
import CanadaOrders from './CanadaOrders.vue'
import {createMarkSixClient,type MarkSixOrder} from './mark-six'
import {createCanadaClient,type CanadaOrder} from './canada'
import ConfiguredNodeEditor from './ConfiguredNodeEditor.vue'
const props=defineProps<{surface:'platform'|'admin';brandId:string;gameId:string;locale:'en'|'zh-CN';snapshot:ConfiguredSnapshot|null;sample?:Pick<ConfiguredCase,'requests'|'draw'>}>()
const emit=defineEmits<{load:[snapshot:ConfiguredSnapshot]}>()
const i18n=createCatalogI18n(configuredRuleCatalog,props.locale);watch(()=>props.locale,v=>i18n.setLocale(v));const t=i18n.t
const api=computed(()=>createConfiguredBusinessClient(props.surface,props.brandId));const rows=shallowRef<ConfiguredVersion[]>([]),selected=ref(''),casesJson=ref('[]'),error=ref<LocalizedMessage|null>(null),busy=ref(false),sales=shallowRef<ConfiguredSales|null>(null),salesMissing=ref(false)
const orders=shallowRef<ConfiguredLiveOrder[]>([]),orderOffset=ref(0)
async function orderPage(delta:number){orderOffset.value+=delta;await load()}
const legacyMarkSix=shallowRef<MarkSixOrder[]>([]),legacyCanada=shallowRef<CanadaOrder[]>([]),legacyLoaded=ref(false),legacyOffset=ref(0)
async function legacyPage(delta=0){const token=generation;busy.value=true;error.value=null;const next=legacyOffset.value+delta;try{const mark=await createMarkSixClient('admin',()=>props.brandId).orders(props.gameId,next);const canada=await createCanadaClient('admin',()=>props.brandId).orders(props.gameId,next);if(token===generation){legacyMarkSix.value=mark.items;legacyCanada.value=canada.items;legacyOffset.value=next;legacyLoaded.value=true}}catch(e){if(token===generation)report(e)}finally{if(token===generation)busy.value=false}}
const expectedBet=ref(''),expectedPrize=ref(''),expectedRefund=ref('')
const pending=shallowRef<{path:string;body:unknown;key:string}|null>(null)
const version=computed(()=>rows.value.find(v=>v.id===selected.value));let generation=0;onUnmounted(()=>generation++)
const salesNode=computed(()=>{if(!sales.value)return null;const {version,rule_id,...fields}=sales.value;return fields as unknown as RuleNode})
function editSales(value:RuleNode){if(sales.value)sales.value={...sales.value,...value as unknown as Omit<ConfiguredSales,'version'|'rule_id'>}}
const editable=computed(()=>!version.value||['draft','validated','rejected'].includes(version.value.status))
function report(cause:unknown){error.value=i18n.errorMessage(cause)}
async function load(){
 const token=++generation;busy.value=true;error.value=null
 try{
  const response=await api.value.versions(props.gameId);if(token!==generation)return;rows.value=response.items
  if(!selected.value || !rows.value.some(v=>v.id===selected.value)){selected.value=(rows.value.find(v=>v.status==='published')??rows.value[0])?.id??'';choose()}
  if(props.surface==='admin'){
   try{const current=await api.value.sales(props.gameId);if(token===generation){sales.value=current;salesMissing.value=false}}
   catch(e){if((e as {code?:string}).code!=='RESOURCE_NOT_FOUND')throw e;if(token===generation){sales.value=null;salesMissing.value=true}}
   if(token!==generation)return
   const page=await api.value.adminOrders(props.gameId,orderOffset.value);if(token===generation)orders.value=page.items
  }
 }catch(e){if(token===generation)report(e)}finally{if(token===generation)busy.value=false}
}
watch(()=>[props.gameId,props.brandId,props.surface],()=>{legacyLoaded.value=false;legacyOffset.value=0;legacyMarkSix.value=[];legacyCanada.value=[];selected.value='';pending.value=null;rows.value=[];sales.value=null;orders.value=[];orderOffset.value=0;casesJson.value='[]';void load()},{immediate:true})
function choose(){if(!version.value)return;emit('load',JSON.parse(JSON.stringify(version.value.snapshot)));casesJson.value=JSON.stringify(version.value.cases,null,2)}
function addCase(){try{if(!props.sample||![expectedBet.value,expectedPrize.value,expectedRefund.value].every(s=>/^(0|[1-9][0-9]*)$/.test(s)))throw new Error('Expected amounts required');const cases=JSON.parse(casesJson.value) as ConfiguredCase[];cases.push({...props.sample,bet_points:expectedBet.value,prize_points:expectedPrize.value,refund_points:expectedRefund.value});casesJson.value=JSON.stringify(cases,null,2);error.value=null}catch{report({code:'RULE_INVALID'})}}
async function send<T=unknown>(path?:string,body?:unknown):Promise<T|undefined>{
 if(!pending.value){if(!path)return;pending.value={path,body:JSON.parse(JSON.stringify(body)),key:crypto.randomUUID()}}
 busy.value=true;error.value=null;const token=generation
 try{const receipt=await api.value.request<T>(pending.value.path,pending.value.body,pending.value.key);if(token!==generation)return;pending.value=null;await load();return receipt}
 catch(e){if(token!==generation)return;const status=(e as {status?:number}).status;if(status&&status>=400&&status<500)pending.value=null;report(e)}finally{if(token===generation)busy.value=false}
}
async function save(){try{if(!props.snapshot)throw new Error('Snapshot required');const cases=JSON.parse(casesJson.value);if(!Array.isArray(cases))throw new Error('Cases required');const receipt=await send<ConfiguredVersion>(`/${props.gameId}/configured/versions`,{id:version.value?.id??'',revision:version.value?.revision??0,snapshot:props.snapshot,cases});if(receipt){if(!rows.value.some(row=>row.id===receipt.id))rows.value=[receipt,...rows.value];selected.value=receipt.id;choose()}}catch{report({code:'RULE_INVALID'})}}
async function action(action:string){if(!version.value)return;await send(`/${props.gameId}/configured/versions/${version.value.id}`,{revision:version.value.revision,action})}
function newSales(){if(!version.value||version.value.status!=='published'||(!sales.value&&!salesMissing.value))return;sales.value={version:sales.value?.version??0,rule_id:version.value.id,enabled:false,prices:JSON.parse(JSON.stringify(version.value.snapshot.prices)),min_points:'1',max_points:'100',max_period_points:'10000',max_user_period_points:'5000'}}
</script>
<template>
 <section class="configured-publishing"><h3>{{t('publication')}}</h3><p v-if="error" class="message error" role="alert">{{i18n.displayError(error)}}</p><p v-if="pending">{{t('unknownWrite')}}</p>
 <button type="button" class="button button-secondary" :disabled="busy||!!pending" @click="withGlobalLoading(load)">{{t('refreshVersions')}}</button>
 <button v-write-guard="busy" v-if="pending" type="button" class="button button-primary" :disabled="busy" @click="withGlobalLoading(send)">{{t('retryWrite')}}</button>
 <fieldset :disabled="busy||!!pending"><label>{{t('savedVersions')}}<select v-model="selected" @change="choose"><option value="">{{t('newDraft')}}</option><option v-for="v in rows" :key="v.id" :value="v.id">{{v.snapshot.catalog.plays[0]?.label[locale]}} · {{t(`state.${v.status}`)}}</option></select></label>
 <template v-if="surface==='platform'"><p>{{t('caseHint')}}</p><template v-if="sample"><label>{{t('expectedBet')}}<input v-model="expectedBet" inputmode="numeric" /></label><label>{{t('expectedPrize')}}<input v-model="expectedPrize" inputmode="numeric" /></label><label>{{t('expectedRefund')}}<input v-model="expectedRefund" inputmode="numeric" /></label><button type="button" class="button button-secondary" :disabled="!editable" @click="addCase">{{t('addCase')}}</button></template><label>{{t('validationCases')}}<BusinessDetails :value="JSON.parse(casesJson)" :locale="props.locale" /></label>
 <button v-write-guard="busy" type="button" class="button button-primary" :disabled="!snapshot||!editable" @click="withGlobalLoading(save)">{{t('saveDraft')}}</button>
 <div v-if="version" class="configured-actions"><button v-write-guard="busy" v-if="['draft','validated','rejected'].includes(version.status)" type="button" class="button button-secondary" @click="withGlobalLoading(action,'validate')">{{t('validateSaved')}}</button><button v-write-guard="busy" v-if="version.status==='validated'" type="button" class="button button-secondary" @click="withGlobalLoading(action,'submit')">{{t('submitReview')}}</button><template v-if="version.status==='submitted'"><button v-write-guard="busy" type="button" class="button button-primary" @click="withGlobalLoading(action,'approve')">{{t('approve')}}</button><button v-write-guard="busy" type="button" class="button button-secondary" @click="withGlobalLoading(action,'reject')">{{t('reject')}}</button></template><button v-write-guard="busy" v-if="version.status==='approved'" type="button" class="button button-primary" @click="withGlobalLoading(action,'publish')">{{t('publishRules')}}</button></div>
 </template>
 <template v-else><p>{{t('brandSalesHint')}}</p><div class="configured-actions"><button type="button" class="button button-secondary" :disabled="(!salesMissing&&!sales)||version?.status!=='published'" @click="newSales">{{t('initializeSales')}}</button></div><template v-if="sales"><ConfiguredNodeEditor :model-value="salesNode" :locale="locale" @update:model-value="editSales" /><button v-write-guard="busy" type="button" class="button button-primary" @click="withGlobalLoading(send,`/${gameId}/configured/sales`,sales)">{{t('saveSales')}}</button></template></template>
 </fieldset>
 <template v-if="surface==='admin'"><h3>{{t('brandOrders')}}</h3><article v-for="order in orders" :key="order.id"><p>{{order.member_name}} · {{order.period_no}} · {{order.quote.bet_points}} · {{t(order.status==='settled'?'state.settled':'state.placed')}}</p><p v-for="(line,index) in order.quote.lines" :key="index">{{configuredTicketLabel(order.snapshot,line,locale)}} · {{line.stake}} · {{configuredTicketPrices(order.snapshot,line,locale).join(' / ')}}</p><p v-if="order.quote.reserved_points">{{t('reserved')}}: {{order.quote.reserved_points}}</p><p v-if="order.result?.consumed_points">{{t('consumed')}}: {{order.result.consumed_points}}</p><p v-if="order.result">{{t('prize')}}: {{order.result.prize_points}} · {{t(order.snapshot.catalog.funding==='reserve'?'release':'refund')}}: {{order.result.refund_points}}</p></article><p v-if="!orders.length" class="empty-state">{{t('emptyOrders')}}</p><div class="configured-actions"><button type="button" class="button button-secondary" :disabled="busy||!!pending||orderOffset===0" @click="withGlobalLoading(orderPage,-50)">{{t('previousOrders')}}</button><button type="button" class="button button-secondary" :disabled="busy||!!pending||orders.length<50" @click="withGlobalLoading(orderPage,50)">{{t('nextOrders')}}</button></div></template>
 <section v-if="surface==='admin'"><h3>{{t('legacyOrders')}}</h3><button type="button" class="button button-secondary" :disabled="busy" @click="withGlobalLoading(legacyPage,0)">{{t('loadLegacyOrders')}}</button><template v-if="legacyLoaded"><MarkSixOrders v-if="legacyMarkSix.length" :orders="legacyMarkSix" :locale="locale" /><CanadaOrders v-if="legacyCanada.length" :orders="legacyCanada" :locale="locale" /><p v-if="!legacyMarkSix.length&&!legacyCanada.length" class="empty-state">{{t('legacyEmpty')}}</p><div class="configured-actions"><button type="button" class="button button-secondary" :disabled="busy||legacyOffset===0" @click="withGlobalLoading(legacyPage,-50)">{{t('previousOrders')}}</button><button type="button" class="button button-secondary" :disabled="busy||(legacyMarkSix.length<50&&legacyCanada.length<50)" @click="withGlobalLoading(legacyPage,50)">{{t('nextOrders')}}</button></div></template></section>
 </section>
</template>
<style scoped>
.configured-publishing{border-top:1px solid var(--line);padding-top:16px;min-width:0;font-size:14px}.configured-publishing fieldset{border:0;padding:0;display:grid;grid-template-columns:minmax(0,1fr);gap:12px;min-width:0}.configured-publishing label{display:grid;gap:6px;min-width:0}.configured-publishing input,.configured-publishing select,.configured-publishing .button{justify-self:start;max-width:100%;white-space:normal;height:auto;min-height:44px;overflow-wrap:anywhere}.configured-publishing textarea{min-width:0;width:100%;min-height:44px;box-sizing:border-box}.configured-publishing .button{justify-self:start;max-width:100%;white-space:normal;height:auto;min-height:44px;overflow-wrap:anywhere}.configured-publishing textarea{font:13px monospace}.configured-actions{display:flex;flex-wrap:wrap;gap:8px}
</style>
