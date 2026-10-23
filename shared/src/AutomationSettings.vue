<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import FeedMappingEditor from './FeedMappingEditor.vue'
import {newFeedMapping,type FeedReading} from './feed-mapping'
import { withGlobalLoading } from './global-loading';
import { vRefreshButton } from '@lottery/shared';
import PlatformTemporalInput from "./PlatformTemporalInput.vue";
const displayDate = (value: string | null | undefined) => formatDisplayDateTime(value, props.locale);
import { formatDisplayDateTime } from "./display-settings";
import { vInlineValidation } from './form-validation'
import {computed,onUnmounted,ref,watch} from 'vue'
import {createCatalogI18n} from './localization'
import {createPlatformLotteryClient,PlatformLotteryError} from './platform-lottery'
import {automationCatalog,type AutomationConfig,type AutomationPlan,type BrandAutomation,type DrawSource,type DrawSourceList} from './automation'
const props=defineProps<{surface:'platform'|'admin';gameId:string;brandId:string;locale:'en'|'zh-CN';canWrite:boolean}>()
const emit=defineEmits<{failure:[cause:unknown]}>()
const i18n=createCatalogI18n(automationCatalog,props.locale);watch(()=>props.locale,v=>i18n.setLocale(v));const t=i18n.t
const api=createPlatformLotteryClient(props.surface,()=>props.brandId),config=ref<AutomationConfig|null>(null),busy=ref(false),error=ref(''),saved=ref(false),reason=ref(''),anchor=ref('')
const notice=ref('saved')
const plan=ref<AutomationPlan>({source_type:'mapped',mapping:newFeedMapping(),version:0,enabled:false,anchor_draw_at:null,anchor_period_no:null,interval_seconds:null,close_before_seconds:0,source_url:'',credential_ref:''})
const brand=ref<BrandAutomation>({version:0,rule_version:0,enabled:false,close_before_seconds:0})
const registry=ref<DrawSourceList>({items:[],active_source_id:'',active_source_revision:0,version:0}),selected=ref(''),sourceName=ref('')
const pending=ref<{path:string;body:unknown;key:string}|null>(null);let generation=0,alive=true
const canEdit=computed(()=>props.canWrite&&(props.surface==='platform'?plan.value.source_type==='mapped':!!config.value?.plan))
const upstream=computed(()=>(props.surface==='platform'?plan.value:config.value?.plan)?.mapping?.schedule==='source'||(props.surface==='platform'?plan.value:config.value?.plan)?.source_type==='hk_marksix_6hch')
const closeMax=computed(()=>{const p=props.surface==='platform'?plan.value:config.value?.plan;return upstream.value?86400:(p?.interval_seconds==null?0:p.interval_seconds-1)})
const previewPeriod=ref(''),sample=ref(''),preview=ref<FeedReading|null>(null)
function definition(){return {...plan.value,anchor_draw_at:upstream.value?null:new Date(anchor.value+'Z').toISOString(),anchor_period_no:upstream.value?null:plan.value.anchor_period_no,interval_seconds:upstream.value?null:plan.value.interval_seconds}}
async function previewSource(fetchSource:boolean){const g=generation;error.value='';preview.value=null;try{const value=await api.write('/'+props.gameId+'/draw-source-preview',{plan:definition(),sample:fetchSource?'':sample.value,period_no:previewPeriod.value},crypto.randomUUID()) as FeedReading;if(alive&&g===generation)preview.value=value}catch(e){if(alive&&g===generation)error.value=t(e instanceof PlatformLotteryError&&e.code==='AUTOMATION_BROWSER_UNAVAILABLE'?'browserUnavailable':e instanceof PlatformLotteryError&&e.code==='AUTOMATION_BROWSER_RENDER_FAILED'?'browserFailure':'previewFailure')}}
watch(()=>plan.value,()=>{preview.value=null},{deep:true})
function time(value:string|null){return formatDisplayDateTime(value,props.locale)}
function report(e:unknown,key:'loadFailure'|'saveFailure'){error.value=t(e instanceof PlatformLotteryError&&e.code==='AUTOMATION_SOURCE_NOT_ALLOWED'?'sourceDenied':key);if(e instanceof PlatformLotteryError&&e.status===401)emit('failure',e)}
function chooseSource(item?:DrawSource){sample.value='';preview.value=null;selected.value=item?.id??'';sourceName.value=item?.name??'';plan.value=item?JSON.parse(JSON.stringify(item.plan)):{source_type:'mapped',mapping:newFeedMapping(),version:0,enabled:false,anchor_draw_at:null,anchor_period_no:null,interval_seconds:null,close_before_seconds:0,source_url:'',credential_ref:''};anchor.value=plan.value.anchor_draw_at===null?'':new Date(plan.value.anchor_draw_at).toISOString().slice(0,19)}
async function load(){
 const g=generation;busy.value=true;error.value=''
 try{
  const [value,sources]=await Promise.all([api.automation(props.gameId),props.surface==='platform'?api.drawSources(props.gameId):Promise.resolve(null)])
  if(!alive||g!==generation)return;config.value=value
  if(sources){registry.value=sources;const chosen=sources.items.find(x=>x.id===selected.value)||sources.items.find(x=>x.id===sources.active_source_id);chooseSource(chosen)}
  if(value.plan){brand.value.rule_version=value.plan.version;brand.value.close_before_seconds=value.plan.close_before_seconds}
  if(value.brand)brand.value={...value.brand,rule_version:value.plan!.version}
  reason.value=''
 }catch(e){if(alive&&g===generation)report(e,'loadFailure')}finally{if(alive&&g===generation)busy.value=false}
}
async function send(path?:string,body?:unknown){
 if(!pending.value){if(!path)return;pending.value={path,body:JSON.parse(JSON.stringify(body)),key:crypto.randomUUID()}}
 const snapshot=pending.value,g=generation;busy.value=true;error.value='';saved.value=false
 try{const out=await api.write(snapshot.path,snapshot.body,snapshot.key) as {id?:string};if(!alive||g!==generation)return;if(out.id)selected.value=out.id;pending.value=null;notice.value=snapshot.path.endsWith('/activate')?'sourceActivated':snapshot.path.endsWith('/draw-sources')?'sourceSaved':'saved';saved.value=true;await load()}
 catch(e){if(alive&&g===generation){if(e instanceof PlatformLotteryError&&e.status>=400&&e.status<500)pending.value=null;report(e,'saveFailure')}}finally{if(alive&&g===generation)busy.value=false}
}
async function save(){
 if(pending.value){await send();return}
 try{
  if(props.surface==='platform'){
   const definitionValue=definition()
   await send('/'+props.gameId+'/draw-sources',{id:selected.value,revision:registry.value.items.find(x=>x.id===selected.value)?.revision??0,name:sourceName.value,plan:definitionValue})
  }else await send('/'+props.gameId+'/automation',{...brand.value,reason:reason.value})
 }catch{error.value=t('saveFailure')}
}
async function activate(item:DrawSource){await send('/'+props.gameId+'/draw-sources/'+item.id+'/activate',{revision:item.revision,version:registry.value.version})}
watch(()=>[props.gameId,props.brandId],()=>{generation++;pending.value=null;config.value=null;selected.value='';registry.value={items:[],active_source_id:'',active_source_revision:0,version:0};void withGlobalLoading(load)},{immediate:true})
const poll=setInterval(async()=>{if(!busy.value&&!pending.value&&config.value){const g=generation;try{const value=await api.automation(props.gameId);if(alive&&g===generation){config.value={...config.value!,attempts:value.attempts,source_check:value.source_check};if(error.value===t('loadFailure'))error.value=''}}catch(e){if(alive&&g===generation)report(e,'loadFailure')}}},5000)
onUnmounted(()=>{alive=false;generation++;clearInterval(poll)})
</script>
<template><section class="automation-settings" data-testid="automation-settings">
<template v-if="surface==='platform'"><div class="source-heading"><h3>{{t('savedSources')}}</h3><button type="button" class="button button-secondary" :disabled="busy||!!pending||!canWrite" @click="chooseSource()">{{t('newSource')}}</button></div><p>{{t('currentSource')}}: {{registry.active_source_id ? registry.items.find(item=>item.id===registry.active_source_id)?.name : t('noSource')}}</p><div class="draw-source-list"><article v-for="item in registry.items" :key="item.id"><strong>{{item.name}}</strong><button type="button" class="button button-secondary" :disabled="busy||!!pending" @click="chooseSource(item)">{{t(item.plan.source_type==='mapped'?'selectSource':'viewSource')}}</button><button v-write-guard="busy" type="button" class="button button-primary" :disabled="busy||!!pending||!canWrite||item.plan.source_type!=='mapped'||(item.id===registry.active_source_id&&item.revision===registry.active_source_revision)" @click="withGlobalLoading(activate,item)">{{t('activateSource')}}</button></article></div></template><p v-else>{{t('brandHelp')}}</p>
<p v-if="surface==='platform'&&plan.source_type!=='mapped'" class="message" role="status">{{t('retiredSource')}}</p><p v-if="error" role="alert">{{ error }}</p><p v-if="saved" role="status">{{ t(notice) }}</p><p v-if="surface==='admin'&&!config?.plan">{{ t('notConfigured') }}</p>
<form v-write-guard="busy" novalidate v-inline-validation v-if="config" @submit.prevent="withGlobalLoading(save)"><fieldset :disabled="busy||!canEdit||!!pending">
<div v-if="surface==='platform'" class="automation-fields"><label>{{t('sourceName')}}<input v-model.trim="sourceName" maxlength="120" required /></label>

<label class="check"><input v-model="plan.enabled" type="checkbox" />{{ t('enabled') }}</label>
<label>{{ t('sourceURL') }}<input v-model.trim="plan.source_url" type="url" required /></label>
<label>{{ t('credential') }}<input v-model.trim="plan.credential_ref" pattern="DRAW_FEED_[A-Z0-9_]{1,109}" autocomplete="off" /></label><template v-if="!upstream"><label>{{ t('anchor') }}<PlatformTemporalInput v-model="anchor" type="datetime-local" required :locale="locale" /><small>{{ t('utcHint') }}</small></label><label>{{ t('periodNumber') }}<input v-model="plan.anchor_period_no" inputmode="numeric" pattern="[0-9]{1,40}" required /></label><label>{{ t('interval') }}<input v-model.number="plan.interval_seconds" type="number" min="10" max="31536000" required /></label></template>
<label>{{ t('closeBefore') }}<input v-model.number="plan.close_before_seconds" type="number" min="0" :max="closeMax" required /></label></div><template v-if="surface==='platform'&&plan.mapping"><FeedMappingEditor v-model="plan.mapping" :locale="locale" /><label v-if="plan.mapping.period_query">{{t('previewPeriod')}}<input v-model="previewPeriod" /></label><label v-if="plan.mapping.format!=='browser'">{{t('sample')}}<textarea v-model="sample" rows="6" /></label><button v-if="plan.mapping.format!=='browser'" type="button" class="button button-secondary" :disabled="!sample.trim()||busy" @click="withGlobalLoading(previewSource,false)">{{t('previewSample')}}</button><button type="button" class="button button-secondary" :disabled="busy" @click="withGlobalLoading(previewSource,true)">{{t('previewFetch')}}</button><section v-if="preview" data-testid="feed-preview"><h3>{{t('previewResult')}}</h3><dl v-if="preview.completed"><dt>{{t('periodField')}}</dt><dd>{{preview.completed.period_no}}</dd><dt>{{t('drawTimeField')}}</dt><dd>{{time(preview.completed.drawn_at)}}</dd><dt>{{t('regularField')}}</dt><dd>{{preview.completed.result.regular.join(', ')}}</dd><dt>{{t('specialField')}}</dt><dd>{{preview.completed.result.special.join(', ')}}</dd><dt>{{t('digitsField')}}</dt><dd>{{preview.completed.result.digits.join(', ')}}</dd></dl><p v-else>{{t('pendingResult')}}</p><p v-if="preview.next_period_no">{{t('nextPeriodField')}}: {{preview.next_period_no}} · {{time(preview.next_draw_at)}}</p></section></template>
<template v-else-if="config.plan"><dl><dt>{{ t('sourceType') }}</dt><dd>{{ t(config.plan.source_type==='mapped'?'mappedSource':config.plan.source_type==='interval_json'?'intervalSource':'hongKongSource') }}</dd><dt>{{ t('source') }}</dt><dd>{{ config.plan.source_url }}</dd><template v-if="!upstream"><dt>{{ t('interval') }}</dt><dd>{{ config.plan.interval_seconds }}</dd><dt>{{ t('anchor') }}</dt><dd>{{ displayDate(config.plan.anchor_draw_at) }}</dd></template></dl><label class="check"><input v-model="brand.enabled" type="checkbox" />{{ t('brandEnabled') }}</label><label>{{ t('closeBefore') }}<input v-model.number="brand.close_before_seconds" type="number" :min="config.plan.close_before_seconds" :max="closeMax" required /></label></template>
</fieldset><p v-if="pending" role="status">{{ t('unknown') }}</p><button v-if="canEdit" class="button button-primary" :disabled="busy">{{ pending?t('retry'):t(surface==='platform'?'saveSource':'save') }}</button></form>
<button v-refresh-button="t('refresh')" class="button button-secondary" :disabled="busy||!!pending" @click="withGlobalLoading(load)">{{ t('refresh') }}</button>
<section v-if="config?.plan && config.plan.enabled && upstream"><h3>{{ t('sourceSchedule') }}</h3><p v-if="!config.source_check">{{ t('awaitingCheck') }}</p><template v-else><p class="status-chip">{{ t(config.source_check.code) }}</p><dl><dt>{{ t('nextPeriod') }}</dt><dd>{{ config.source_check.next_period_no??'—' }}</dd><dt>{{ t('nextDraw') }}</dt><dd>{{ time(config.source_check.next_draw_at) }}</dd><dt>{{ t('checkedAt') }}</dt><dd>{{ time(config.source_check.at) }}</dd><template v-if="config.source_check.last_result"><dt>{{ t('lastConfirmed') }}</dt><dd>{{ config.source_check.last_result.period_no }} · {{ [...config.source_check.last_result.result.regular,...config.source_check.last_result.result.special,...config.source_check.last_result.result.digits].map(n=>String(n).padStart(2,'0')).join(', ') }}</dd></template></dl></template></section>
<details><summary>{{ t('help') }}</summary><p>{{ t('mappingHelp') }}</p><p>{{ t('origins') }} · {{ config?.allowed_origins.join(', ')||t('originMissing') }}</p></details>
<h3>{{ t('attempts') }}</h3><p v-if="!config?.attempts.length" class="empty-state">{{ t('empty') }}</p><article v-for="a in config?.attempts" :key="a.period_no"><strong>{{ a.period_no }}</strong><span>{{ t(a.code) }}</span><small>{{ time(a.at) }} · {{ t('attemptCount') }} {{ a.attempts }}</small></article></section></template>
<style scoped>.source-heading{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap}.source-heading h3{margin:0}.draw-source-list{display:grid;gap:8px;margin-bottom:16px}.automation-settings{min-width:0;font-size:13px;line-height:1.5}.automation-fields{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px}fieldset{border:0;margin:0;padding:0;min-width:0}.automation-settings label{display:grid;grid-template-columns:minmax(0,1fr);gap:6px;margin-bottom:10px;min-width:0}.automation-settings input,.automation-settings select,.automation-settings textarea{width:100%;min-width:0;box-sizing:border-box;min-height:44px;background:var(--surface);color:var(--ink);border:1px solid var(--line);border-radius:var(--radius-sm);padding:8px;font:inherit}.automation-settings .check{display:flex;align-items:center;gap:8px}.check input{width:18px}.automation-settings button{min-height:44px;margin:6px 8px 8px 0}.automation-settings small{font-size:12px;color:var(--muted)}.automation-settings dd{margin:4px 0 12px;overflow-wrap:anywhere}.automation-settings summary{min-height:44px;align-content:center;cursor:pointer}.automation-settings article{display:flex;flex-wrap:wrap;gap:8px;padding:10px;border:1px solid var(--line);border-radius:var(--radius-sm);margin-bottom:8px}.automation-settings article small{flex-basis:100%;overflow-wrap:anywhere}@media(max-width:620px){.automation-fields{grid-template-columns:minmax(0,1fr)}}</style>
