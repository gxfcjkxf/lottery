<script setup lang="ts">
import { computed, ref, watch, onBeforeUnmount } from 'vue'
import { ApiError } from './api'
import { createCatalogI18n } from './localization'
import { announcementsCatalog, type AnnouncementKey } from './announcements-catalog'
import { createAnnouncementsApi, newAnnouncement, safeAnnouncementHTML, safeAnnouncementLink, type Announcement, type AnnouncementInput } from './announcements'
import { useOperationToast, withGlobalLoading, vInlineValidation, vWriteGuard } from './index'
import ManagementList from './ManagementList.vue'
import ManagementTable from './ManagementTable.vue'
import TablePagination from './TablePagination.vue'
import DetailModal from './DetailModal.vue'
import SelectControl from './SelectControl.vue'
import ClearableInput from './ClearableInput.vue'
import {vRefreshButton} from './refresh-button'
import {platformDisplaySettings} from './display-settings'
import PlatformTemporalInput from './PlatformTemporalInput.vue'
import {lotteryLocalTimeToISO,lotteryISOToLocalTime} from './lottery-time'
import AnnouncementEditor from './AnnouncementEditor.vue'
import AnnouncementContent from './AnnouncementContent.vue'
const props=defineProps<{basePath:string;brandId?:string;actorId:string;locale:'en'|'zh-CN';permissions:string[];platform?:boolean}>()
const emit=defineEmits<{failure:[cause:unknown]}>()
const i18n=createCatalogI18n(announcementsCatalog,props.locale),t=i18n.t
watch(()=>props.locale,v=>i18n.setLocale(v))
const api=computed(()=>createAnnouncementsApi(props.basePath,props.brandId))
const can=(action:string)=>props.permissions.includes(`announcement.${action}.${props.platform?'platform':'brand'}`)
const rows=ref<Announcement[]>([]),brands=ref<{id:string;name:string}[]>([]),loading=ref(false),writing=ref(false),error=ref<AnnouncementKey|null>(null),notice=ref(false)
const query=ref(''),status=ref(''),page=ref(1),size=ref(20)
const draft=ref<AnnouncementInput|null>(null),selectedId=ref(''),language=ref<'en'|'zh-CN'>('zh-CN'),preview=ref(false),targetMode=ref('all'),selectedBrands=ref<string[]>([]),conflict=ref(false)
const pending=ref<{id:string;body:AnnouncementInput;key:string}|null>(null)
let generation=0
const filtered=computed(()=>rows.value.filter(v=>(!status.value||v.state===status.value)&&v.config.content[props.locale].title.toLowerCase().includes(query.value.toLowerCase())))
const visible=computed(()=>filtered.value.slice((page.value-1)*size.value,page.value*size.value))
watch([query,status,size],()=>page.value=1)
function failure(cause:unknown){error.value=cause instanceof ApiError&&cause.code==='ANNOUNCEMENT_CONFLICT'?'conflict':cause instanceof ApiError&&cause.status===403?'denied':cause instanceof ApiError&&cause.code==='ANNOUNCEMENT_IMAGE_INVALID'?'imageInvalid':cause instanceof ApiError&&cause.status===400?'invalid':'failure';emit('failure',cause)}
async function load(){const ticket=++generation;loading.value=true;error.value=null;try{const items=await api.value.list();const targets=props.platform?await api.value.brands():[];if(ticket!==generation)return;rows.value=items;brands.value=targets}catch(e){if(ticket===generation)failure(e)}finally{if(ticket===generation)loading.value=false}}
function open(item?:Announcement,copy=false){draft.value=item?{version:copy?0:item.version,status:copy?'draft':item.status,remind:false,config:JSON.parse(JSON.stringify(item.config))}:newAnnouncement();selectedId.value=item&&!copy?item.id:'';targetMode.value=draft.value.config.targets.length?'selected':'all';selectedBrands.value=[...draft.value.config.targets];language.value=props.locale;preview.value=false;conflict.value=false;pending.value=null;error.value=null}
function close(){if(writing.value)return;draft.value=null;pending.value=null;preview.value=false;error.value=null}
const readonly=computed(()=>!can('write')||(draft.value?.status==='published'&&!can('publish'))||writing.value||!!pending.value||conflict.value)
const ready=computed(()=>{if(!draft.value)return false;return (['en','zh-CN'] as const).every(l=>{const c=draft.value!.config.content[l];return c.title.trim()&&c.html.trim()&&(!c.url&&!c.button||c.button&&safeAnnouncementLink(c.url))})&&(targetMode.value!=='selected'||selectedBrands.value.length>0)})
const previewContent=computed(()=>{if(!draft.value)return null;try{const c=draft.value.config.content[language.value];if(c.url&&!safeAnnouncementLink(c.url))return null;return {...c,html:safeAnnouncementHTML(c.html)}}catch{return null}})
async function save(next:Announcement['status']){
 if(!draft.value||(next==='published'&&!ready.value)||!can('write')||writing.value||conflict.value)return
 if(!pending.value){try{for(const l of ['en','zh-CN'] as const){if(draft.value.config.content[l].html)safeAnnouncementHTML(draft.value.config.content[l].html)}}catch(e){failure(e);return};const body=JSON.parse(JSON.stringify(draft.value));body.status=next;body.remind=next==='published'&&body.remind;body.config.targets=targetMode.value==='all'?[]:[...selectedBrands.value];pending.value={id:selectedId.value,body,key:crypto.randomUUID()}}
 await sendPending()
}
async function sendPending(){if(!pending.value||writing.value)return;const intent=pending.value,ticket=generation;writing.value=true;error.value=null;notice.value=false
 try{await api.value.save(intent.id,intent.body,intent.key);if(ticket!==generation)return;pending.value=null;draft.value=null;notice.value=true;await load()}catch(e){if(ticket!==generation)return;if(e instanceof ApiError&&e.status&&e.status<500&&e.code!=='INVALID_RESPONSE'){pending.value=null;conflict.value=e.status===409;failure(e)}else{error.value='unknown';emit('failure',e)}}finally{writing.value=false}}
async function upload(file:File){if(file.size>5*1024*1024||!['image/png','image/jpeg','image/webp'].includes(file.type))throw new ApiError('Invalid image',400,'ANNOUNCEMENT_IMAGE_INVALID');return api.value.upload(file)}
function dateValue(value:string|null){return value?lotteryISOToLocalTime(value,platformDisplaySettings.value!.timezone):''}
function setDate(field:'starts_at'|'ends_at',value:string){try{draft.value!.config[field]=value?lotteryLocalTimeToISO(value,platformDisplaySettings.value!.timezone):null}catch{error.value='invalid'}}
onBeforeUnmount(()=>generation++)
watch(()=>[props.actorId,props.brandId,props.permissions.join('|')],()=>{generation++;draft.value=null;pending.value=null;rows.value=[];brands.value=[];page.value=1;if(can('view'))void load()},{immediate:true})
useOperationToast(()=>error.value,key=>key?t(key):'','error')
useOperationToast(notice,()=>t('saved'),'success')
</script>
<template>
 <p v-if="!can('view')" role="status">{{t('denied')}}</p>
 <ManagementList v-else :busy="loading" data-testid="announcements-management">
  <template #filters><form class="ui-filter-bar" novalidate v-inline-validation @submit.prevent="withGlobalLoading(() => {page=1})"><ClearableInput v-model="query" :locale="locale" :aria-label="t('search')" :placeholder="t('search')"/><SelectControl v-model="status" :label="t('status')" :options="[{value:'',label:t('allStates')},...(['draft','scheduled','active','ended','withdrawn'] as const).map(v=>({value:v,label:t(v)}))]" /></form></template>
  <template #actions><button type="button" v-refresh-button="t('refresh')" class="button button-secondary" :disabled="loading" @click="withGlobalLoading(load)">{{t('refresh')}}</button><button v-if="can('write')" class="button button-primary" @click="open()">{{t('add')}}</button></template>
  <ManagementTable :entries="visible" :columns="(['heading','status','audience','pinned','sort','actions'] as const).map(key=>({key,label:t(key)}))" :empty="!loading&&!visible.length" :empty-label="error?t(error):t('empty')" table-class="announcement-table">
   <template #heading="{entry}"><button class="text-button" @click="open(entry)">{{entry.config.content[locale].title}}</button></template>
   <template #status="{entry}">{{t(entry.state)}}</template><template #audience="{entry}">{{t(entry.config.audience)}}</template><template #pinned="{entry}">{{entry.config.pinned?t('pinned'):'—'}}</template><template #sort="{entry}">{{entry.config.sort}}</template><template #actions="{entry}"><button v-if="can('write')" class="text-button" @click="open(entry,true)">{{t('copy')}}</button></template>
  </ManagementTable>
  <template #pagination><TablePagination v-if="!error" :page="page" :page-size="size" :count="visible.length" :go-to="(value:number)=>page=value" :change-page-size="(value:number)=>{size=value;page=1}" :total="String(filtered.length)" :locale="locale" /></template>
 </ManagementList>
 <DetailModal v-if="draft" class="announcement-management-dialog" :title="selectedId?t('edit'):t('add')" :close-label="t('close')" :close-disabled="writing" @close="close">
  <form v-inline-validation v-write-guard="writing" class="ui-form" novalidate @submit.prevent="withGlobalLoading(save,'draft')">
    <div class="console-tabs"><button v-for="l in (['zh-CN','en'] as const)" :key="l" type="button" :class="{selected:language===l}" @click="language=l">{{t(l==='en'?'english':'chinese')}}</button></div>
   <fieldset :disabled="readonly" class="announcement-fields">

    <label class="ui-field-row">{{t('heading')}}<input v-model="draft.config.content[language].title" maxlength="120"></label>
    <div class="announcement-body-field"><span>{{t('body')}}</span><AnnouncementEditor v-model="draft.config.content[language].html" :locale="locale" :disabled="readonly" :upload="upload" @failure="failure" /></div>
    <div class="announcement-grid"><label class="ui-field-row">{{t('button')}}<input v-model="draft.config.content[language].button" maxlength="40"></label><label class="ui-field-row">{{t('url')}}<input v-model="draft.config.content[language].url" maxlength="2000"></label></div>
    <div class="announcement-grid"><label class="ui-field-row">{{t('audience')}}<SelectControl v-model="draft.config.audience" :label="t('audience')" :options="(['all','guest','member'] as const).map(v=>({value:v,label:t(v)}))" /></label><label class="ui-field-row">{{t('sort')}}<input v-model.number="draft.config.sort" type="number" min="0" max="9999" required></label></div>
    <template v-if="platform"><label class="ui-field-row">{{t('targets')}}<SelectControl v-model="targetMode" :label="t('targets')" :options="[{value:'all',label:t('allBrands')},{value:'selected',label:t('selectedBrands')}]" /></label><div v-if="targetMode==='selected'" class="announcement-targets"><label v-for="brand in brands" :key="brand.id" class="announcement-check"><input v-model="selectedBrands" type="checkbox" :value="brand.id">{{brand.name}}</label></div></template>
    <div class="announcement-dates"><label class="ui-field-row"><span>{{t('starts')}}</span><PlatformTemporalInput type="datetime-local" :model-value="dateValue(draft.config.starts_at)" :locale="locale" :disabled="readonly" :label="t('starts')" :placeholder="t('immediate')" @update:model-value="setDate('starts_at',$event)" /></label><label class="ui-field-row"><span>{{t('ends')}}</span><PlatformTemporalInput type="datetime-local" :model-value="dateValue(draft.config.ends_at)" :locale="locale" :disabled="readonly" :label="t('ends')" :placeholder="t('noEnd')" @update:model-value="setDate('ends_at',$event)" /></label></div>
    <div class="announcement-checks"><label class="announcement-check"><input v-model="draft.config.popup" type="checkbox">{{t('popup')}}</label><label class="announcement-check"><input v-model="draft.config.pinned" type="checkbox">{{t('pinned')}}</label><label v-if="draft.config.popup" class="announcement-check"><input v-model="draft.config.allow_mute" type="checkbox">{{t('mute')}}</label><label v-if="selectedId&&draft.status==='published'&&can('publish')" class="announcement-check"><input v-model="draft.remind" type="checkbox">{{t('remind')}}</label></div>
    <template v-if="draft.config.popup"><div class="announcement-grid"><label class="ui-field-row">{{t('frequency')}}<SelectControl v-model="draft.config.frequency" :label="t('frequency')" :options="(['session','daily','interval','once'] as const).map(v=>({value:v,label:t(v)}))" /></label><label class="ui-field-row" v-if="draft.config.frequency==='interval'">{{t('days')}}<input v-model.number="draft.config.interval_days" type="number" min="1" max="365" required></label></div></template>
   </fieldset>
   <p v-if="error" role="alert">{{t(error)}}</p>
   <div v-if="conflict" class="ui-form-actions"><button type="button" class="button button-secondary" @click="close();withGlobalLoading(load)">{{t('reset')}}</button></div>
   <div v-else class="ui-form-actions"><button type="button" class="button button-secondary" :disabled="!previewContent" @click="preview=true">{{t('preview')}}</button><button v-if="pending" type="button" class="button button-primary" :disabled="writing" @click="withGlobalLoading(sendPending)">{{t('retrySave')}}</button><template v-else-if="can('write')"><button v-if="draft.status!=='published'" type="submit" class="button button-secondary" :disabled="writing">{{t('save')}}</button><button v-if="can('publish')&&draft.status==='published'" type="button" class="button button-secondary" :disabled="writing" @click="withGlobalLoading(save,'withdrawn')">{{t('withdraw')}}</button><button v-if="can('publish')" type="button" class="button button-primary" :disabled="!ready||writing" @click="withGlobalLoading(save,'published')">{{t(draft.status==='published'?'savePublished':'publish')}}</button></template></div>
  </form>
 </DetailModal>
 <DetailModal v-if="preview&&previewContent" :title="previewContent.title" :close-label="t('close')" @close="preview=false"><AnnouncementContent :content="previewContent" /></DetailModal>
</template>
<style scoped>
:deep(.announcement-table){table-layout:fixed;min-width:0;white-space:normal}:deep(.announcement-table td),:deep(.announcement-table th){overflow-wrap:anywhere}:deep(.announcement-table .text-button){white-space:normal;text-align:left}.announcement-fields{display:grid;gap:12px;min-width:0;border:0;padding:0;margin:0}.announcement-fields label{display:grid;gap:6px;min-width:0}.announcement-fields input{min-width:0;max-width:100%;box-sizing:border-box}.announcement-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.announcement-dates{display:grid;gap:12px;min-width:0}.announcement-checks{display:flex;flex-wrap:wrap;gap:12px}.announcement-fields .announcement-check{display:flex;align-items:center;gap:8px;min-height:var(--control-height)}.announcement-targets{display:flex;flex-wrap:wrap;gap:8px 16px;max-height:180px;overflow:auto}.announcement-targets input,.announcement-checks input,.announcement-check input{width:auto}.announcement-fields .console-tabs{margin:0}@media(max-width:700px){.announcement-grid{grid-template-columns:minmax(0,1fr)}.announcement-fields{gap:8px}}
</style>
