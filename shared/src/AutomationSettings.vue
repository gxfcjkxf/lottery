<script setup lang="ts">
import DetailModal from './DetailModal.vue'
import ManagementList from './ManagementList.vue'
import ManagementTable from './ManagementTable.vue'
import TablePagination from './TablePagination.vue'
import MemberActionsMenu from './MemberActionsMenu.vue'
import { useOperationToast } from "@lottery/shared";
import { formValidationText } from './form-validation'
import { vWriteGuard } from "@lottery/shared";
import FeedMappingEditor from './FeedMappingEditor.vue'
import {newFeedMapping,type FeedReading} from './feed-mapping'
import { withGlobalLoading } from './global-loading';
import { vRefreshButton } from './refresh-button'
import PlatformTemporalInput from "./PlatformTemporalInput.vue";
import { formatDisplayDateTime } from "./display-settings";
import { vInlineValidation } from './form-validation'
import {computed,onUnmounted,ref,watch} from 'vue'
import {createCatalogI18n} from './localization'
import {createPlatformLotteryClient,PlatformLotteryError} from './platform-lottery'
import {automationCatalog,type AutomationConfig,type AutomationPlan,type DrawSource,type DrawSourceList} from './automation'
const editorOpen=ref(false)
function openSource(item?:DrawSource){chooseSource(item);editorOpen.value=true}
const pageSize = ref(50)

const props=defineProps<{surface:'platform'|'admin';gameId:string;brandId:string;locale:'en'|'zh-CN';canWrite:boolean}>()
const emit=defineEmits<{failure:[cause:unknown]}>()
const i18n=createCatalogI18n(automationCatalog,props.locale);watch(()=>props.locale,v=>i18n.setLocale(v));const t=i18n.t
const api=createPlatformLotteryClient(props.surface,()=>props.brandId),config=ref<AutomationConfig|null>(null),busy=ref(false),error=ref(''),saved=ref(false),reason=ref(''),anchor=ref('')
const notice=ref('saved')
const plan=ref<AutomationPlan>({source_type:'mapped',mapping:newFeedMapping(),version:0,enabled:false,anchor_draw_at:null,anchor_period_no:null,interval_seconds:null,open_before_seconds:null,close_before_seconds:0,source_url:'',credential_ref:''})
const registry=ref<DrawSourceList>({items:[],active_source_id:'',active_source_revision:0,version:0}),selected=ref(''),sourceName=ref('')
const sourcePage=ref(1)
const sourceRows=computed(()=>registry.value.items.slice((sourcePage.value-1)*pageSize.value,sourcePage.value*pageSize.value))
watch(()=>[props.gameId,props.brandId],()=>{sourcePage.value=1})
watch(()=>registry.value.items.length,count=>{sourcePage.value=Math.min(sourcePage.value,Math.max(1,Math.ceil(count/pageSize.value)))})
const pending=ref<{path:string;body:unknown;key:string}|null>(null);let generation=0,alive=true
const canEdit=computed(()=>props.canWrite&&props.surface==='platform'&&plan.value.source_type==='mapped')
const gameSchedule=computed(()=>plan.value.mapping?.schedule==='game')
const separateTiming=computed(()=>!!config.value?.plan&&config.value.plan.mapping?.schedule==='source'&&plan.value.mapping?.schedule==='source')
const upstream=computed(()=>plan.value.mapping?.schedule==='source')
const closeMax=computed(()=>upstream.value?86400:(plan.value.interval_seconds==null?0:plan.value.interval_seconds-1))
const previewPeriod=ref(''),sample=ref(''),preview=ref<FeedReading|null>(null)
function definition(){if(gameSchedule.value)return {...plan.value,anchor_draw_at:null,anchor_period_no:null,interval_seconds:null,open_before_seconds:null,close_before_seconds:0};return {...plan.value,anchor_draw_at:upstream.value?null:new Date(anchor.value+'Z').toISOString(),anchor_period_no:upstream.value?null:plan.value.anchor_period_no,interval_seconds:upstream.value?null:plan.value.interval_seconds}}
async function previewSource(fetchSource:boolean){const g=generation;error.value='';preview.value=null;try{const value=await api.write('/'+props.gameId+'/draw-source-preview',{plan:definition(),sample:fetchSource?'':sample.value,period_no:previewPeriod.value},crypto.randomUUID()) as FeedReading;if(alive&&g===generation)preview.value=value}catch(e){if(alive&&g===generation)error.value=t(e instanceof PlatformLotteryError&&e.code==='AUTOMATION_BROWSER_UNAVAILABLE'?'browserUnavailable':e instanceof PlatformLotteryError&&e.code==='AUTOMATION_BROWSER_RENDER_FAILED'?'browserFailure':'previewFailure')}}
watch(()=>plan.value,()=>{preview.value=null},{deep:true})
function time(value:string|null){return formatDisplayDateTime(value,props.locale)}
function report(e:unknown,key:'loadFailure'|'saveFailure'){if(e instanceof PlatformLotteryError&&e.status===401){emit('failure',e);return}error.value=t(e instanceof PlatformLotteryError&&e.code==='AUTOMATION_SOURCE_NOT_ALLOWED'?'sourceDenied':key)}
function chooseSource(item?:DrawSource){sample.value='';preview.value=null;selected.value=item?.id??'';sourceName.value=item?.name??'';plan.value=item?JSON.parse(JSON.stringify(item.plan)):{source_type:'mapped',mapping:newFeedMapping(),version:0,enabled:false,anchor_draw_at:null,anchor_period_no:null,interval_seconds:null,open_before_seconds:null,close_before_seconds:0,source_url:'',credential_ref:''};if(config.value?.schedule&&plan.value.mapping){plan.value.mapping.schedule='game';plan.value.mapping.next_period=null;plan.value.mapping.next_draw_time=null;plan.value.anchor_draw_at=null;plan.value.anchor_period_no=null;plan.value.interval_seconds=null;plan.value.open_before_seconds=null;plan.value.close_before_seconds=0};if(separateTiming.value&&config.value?.plan){plan.value.open_before_seconds=config.value.plan.open_before_seconds;plan.value.close_before_seconds=config.value.plan.close_before_seconds};anchor.value=plan.value.anchor_draw_at===null?'':new Date(plan.value.anchor_draw_at).toISOString().slice(0,19)}
async function load(){
 const g=generation;busy.value=true;error.value=''
 try{
  const [value,sources]=await Promise.all([api.automation(props.gameId),props.surface==='platform'?api.drawSources(props.gameId):Promise.resolve(null)])
  if(!alive||g!==generation)return;config.value=value
  if(sources){registry.value=sources;const chosen=sources.items.find(x=>x.id===selected.value)||sources.items.find(x=>x.id===sources.active_source_id);chooseSource(chosen)}
  reason.value=''
 }catch(e){if(alive&&g===generation)report(e,'loadFailure')}finally{if(alive&&g===generation)busy.value=false}
}
async function send(path?:string,body?:unknown){
 if(!pending.value){if(!path)return;pending.value={path,body:JSON.parse(JSON.stringify(body)),key:crypto.randomUUID()}}
 const snapshot=pending.value,g=generation;busy.value=true;error.value='';saved.value=false
 try{const out=await api.write(snapshot.path,snapshot.body,snapshot.key) as {id?:string};if(!alive||g!==generation)return;if(out.id)selected.value=out.id;pending.value=null;notice.value=snapshot.path.endsWith('/activate')?'sourceActivated':snapshot.path.endsWith('/draw-sources')?'sourceSaved':'saved';saved.value=true;if(snapshot.path.endsWith('/draw-sources'))editorOpen.value=false;await load()}
 catch(e){if(alive&&g===generation){if(e instanceof PlatformLotteryError&&e.status>=400&&e.status<500)pending.value=null;report(e,'saveFailure')}}finally{if(alive&&g===generation)busy.value=false}
}
async function save(){
 if(pending.value){await send();return}
 try{
  if(props.surface!=='platform')return
  {
   const definitionValue=definition()
   await send('/'+props.gameId+'/draw-sources',{id:selected.value,revision:registry.value.items.find(x=>x.id===selected.value)?.revision??0,name:sourceName.value,plan:definitionValue})
  }
 }catch{error.value=t('saveFailure')}
}
async function activate(item:DrawSource){await send('/'+props.gameId+'/draw-sources/'+item.id+'/activate',{revision:item.revision,version:registry.value.version})}
watch(()=>[props.gameId,props.brandId],()=>{generation++;editorOpen.value=false;pending.value=null;config.value=null;selected.value='';registry.value={items:[],active_source_id:'',active_source_revision:0,version:0};void withGlobalLoading(load)},{immediate:true})

onUnmounted(()=>{alive=false;generation++})
useOperationToast(saved, () => t(notice.value), 'success');

useOperationToast(error, value => value, 'error');
</script>
<template><section class="automation-settings" data-testid="automation-settings">
<ManagementList v-if="surface==='platform'" embedded :busy="busy" data-testid="draw-source-directory">
 <template #heading><h3>{{t('savedSources')}}</h3></template>
 <template #actions><button v-if="pending?.path.endsWith('/activate')" type="button" class="button button-primary" :disabled="busy" @click="withGlobalLoading(send)">{{t('retry')}}</button><button type="button" class="button button-primary" :disabled="busy||!!pending||!canWrite" @click="openSource()">{{t('newSource')}}</button></template>
 <template #summary><p>{{t('currentSource')}}: {{registry.active_source_id ? registry.items.find(item=>item.id===registry.active_source_id)?.name : t('noSource')}}</p></template>
 <ManagementTable :entries="sourceRows" :columns="[{key:'name',label:t('sourceName')},{key:'actions',label:t('actions')}]" :empty="Boolean(config) && !busy && !error && !registry.items.length" :empty-label="t('emptySources')">
  <template #name="{entry:item}"><button type="button" class="text-button" :disabled="busy||!!pending" @click="openSource(item)">{{item.name}}</button></template>
  <template #actions="{entry:item}"><MemberActionsMenu :id="'draw-source-'+item.id" :label="t('actions')" :disabled="busy||!!pending"><button type="button" role="menuitem" @click="openSource(item)">{{t(item.plan.source_type==='mapped'?'selectSource':'viewSource')}}</button><button v-write-guard="busy" type="button" role="menuitem" :disabled="!canWrite||item.plan.source_type!=='mapped'||(item.id===registry.active_source_id&&item.revision===registry.active_source_revision)" @click="withGlobalLoading(activate,item)">{{t('activateSource')}}</button></MemberActionsMenu></template>
 </ManagementTable>
 <template #pagination><TablePagination :total="String(registry.items.length)" :page="sourcePage" :count="sourceRows.length" :page-size="pageSize" :locale="locale" :previous-disabled="busy||!!pending||sourcePage===1" :next-disabled="busy||!!pending||sourcePage*pageSize>=registry.items.length" :go-to="async page=>{sourcePage=page}" :change-page-size="async (size: number) => { pageSize = size; sourcePage = 1; await load() }" :busy="busy" /></template>
</ManagementList>
<p v-if="surface==='admin'&&!config?.plan">{{ t('notConfigured') }}</p>
<DetailModal v-if="surface==='platform'&&editorOpen" :title="selected?t('selectSource'):t('newSource')" :close-label="t('close')" :close-disabled="busy||!!pending" @close="editorOpen=false">
<p v-if="surface==='platform'&&plan.source_type!=='mapped'" class="message" role="status">{{t('retiredSource')}}</p>
<p v-if="config?.plan&&config.plan.source_type!=='mapped'" role="alert">{{t('unsupportedSource')}}</p><form class="ui-form" v-write-guard="busy" novalidate v-inline-validation v-if="config" @submit.prevent="withGlobalLoading(save)"><fieldset :disabled="busy||!canEdit||!!pending">
<div v-if="surface==='platform'" class="automation-fields"><label>{{t('sourceName')}}<input v-model.trim="sourceName" maxlength="120" required /></label>

<label class="check"><input v-model="plan.enabled" type="checkbox" />{{ t('enabled') }}</label>
<label>{{ t('sourceURL') }}<input v-model.trim="plan.source_url" type="url" required /></label>
<label>{{ t('credential') }}<input v-model.trim="plan.credential_ref" pattern="DRAW_FEED_[A-Z0-9_]{1,109}" :data-validation-format="formValidationText('credentialFormat')" autocomplete="off" /></label><template v-if="!upstream&&!gameSchedule"><label>{{ t('anchor') }}<PlatformTemporalInput v-model="anchor" type="datetime-local" required :locale="locale" /></label><label>{{ t('periodNumber') }}<input v-model="plan.anchor_period_no" inputmode="numeric" pattern="[0-9]{1,40}" :data-validation-format="formValidationText('periodFormat')" required /></label><label>{{ t('interval') }}<input v-model.number="plan.interval_seconds" type="number" min="10" max="31536000" required /></label></template>
<template v-if="!gameSchedule&&!separateTiming"><label>{{ t('openBefore') }}<input v-model.number="plan.open_before_seconds" type="number" :min="plan.close_before_seconds+1" :max="upstream?31536000:plan.interval_seconds??undefined" required /></label><label>{{ t('closeBefore') }}<input v-model.number="plan.close_before_seconds" type="number" min="0" :max="plan.open_before_seconds===null?closeMax:Math.min(closeMax,plan.open_before_seconds-1)" required /></label></template></div><template v-if="surface==='platform'&&plan.mapping"><FeedMappingEditor v-model="plan.mapping" :locale="locale" :result-only="!!config.schedule" /><label v-if="plan.mapping.period_query">{{t('previewPeriod')}}<input v-model="previewPeriod" /></label><label v-if="plan.mapping.format!=='browser'">{{t('sample')}}<textarea v-model="sample" rows="6" /></label><button v-if="plan.mapping.format!=='browser'" type="button" class="button button-secondary" :disabled="!sample.trim()||busy" @click="withGlobalLoading(previewSource,false)">{{t('previewSample')}}</button><button type="button" class="button button-secondary" :disabled="busy" @click="withGlobalLoading(previewSource,true)">{{t('previewFetch')}}</button><section v-if="preview" data-testid="feed-preview"><h3>{{t('previewResult')}}</h3><dl v-if="preview.completed"><dt>{{t('periodField')}}</dt><dd>{{preview.completed.period_no}}</dd><dt>{{t('drawTimeField')}}</dt><dd>{{time(preview.completed.drawn_at)}}</dd><dt>{{t('regularField')}}</dt><dd>{{preview.completed.result.regular.join(', ')}}</dd><dt>{{t('specialField')}}</dt><dd>{{preview.completed.result.special.join(', ')}}</dd><dt>{{t('digitsField')}}</dt><dd>{{preview.completed.result.digits.join(', ')}}</dd></dl><p v-else>{{t('pendingResult')}}</p><p v-if="preview.next_period_no">{{t('nextPeriodField')}}: {{preview.next_period_no}} · {{time(preview.next_draw_at)}}</p></section></template>
</fieldset><p v-if="pending" role="status">{{ t('unknown') }}</p><div class="ui-form-actions"><button v-if="canEdit" class="button button-primary" :disabled="busy">{{ pending?t('retry'):t(surface==='platform'?'saveSource':'save') }}</button></div></form>
</DetailModal>
<button v-if="error && !pending" class="button button-secondary" :disabled="busy||!!pending" v-refresh-button="t('refresh')" @click="withGlobalLoading(load)">{{ t('refresh') }}</button>

</section></template>
<style scoped>.source-heading{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap}.source-heading h3{margin:0}.draw-source-list{display:grid;gap:8px;margin-bottom:16px}.automation-settings{min-width:0;font-size:var(--body-font-size);line-height:1.5}.automation-fields{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px}fieldset{border:0;margin:0;padding:0;min-width:0}.automation-settings label{display:grid;grid-template-columns:minmax(0,1fr);gap:6px;margin-bottom:10px;min-width:0}.automation-settings input, .automation-settings textarea{width:100%;min-width:0;box-sizing:border-box;min-height:var(--control-height);background:var(--surface);color:var(--ink);border:1px solid var(--line);border-radius:var(--radius-sm);padding:4px 8px;font:inherit}.automation-settings .check{display:flex;align-items:center;gap:8px}.check input{width:18px}.automation-settings button{min-height:var(--control-height);margin:6px 8px 8px 0;box-sizing:border-box;padding-block:4px;line-height:22px}.automation-settings small{font-size:var(--body-font-size);color:var(--muted)}.automation-settings dd{margin:4px 0 12px;overflow-wrap:anywhere}.automation-settings summary{min-height:var(--control-height);align-content:center;cursor:pointer}.automation-settings article{display:flex;align-items:center;flex-wrap:wrap;gap:8px;padding:10px;border:1px solid var(--line);border-radius:var(--radius-sm);margin-bottom:8px}:global(#app .automation-settings .source-actions){margin:0;margin-inline-start:auto}.automation-settings article strong{min-width:0;overflow-wrap:anywhere}.automation-settings article small{flex-basis:100%;overflow-wrap:anywhere}@media(max-width:620px){.automation-fields{grid-template-columns:minmax(0,1fr)}}</style>
