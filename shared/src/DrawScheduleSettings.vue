<script setup lang="ts">
import {ref,watch,onUnmounted} from 'vue'
import SelectControl from './SelectControl.vue'
import PlatformTimeInput from './PlatformTimeInput.vue'
import {platformDisplaySettings} from './display-settings'
import {createPlatformLotteryClient,PlatformLotteryError} from './platform-lottery'
import {createCatalogI18n} from './localization'
import {drawScheduleCatalog,type DrawSchedule} from './draw-schedule'
import {withGlobalLoading} from './global-loading'
import {vWriteGuard} from './write-guard'
import {vInlineValidation} from './form-validation'
import {showOperationToast} from './operation-toast'
const props=defineProps<{gameId:string;timezone:string;locale:'en'|'zh-CN';canWrite:boolean}>()
const emit=defineEmits<{failure:[cause:unknown];saved:[]}>()
const i18n=createCatalogI18n(drawScheduleCatalog,props.locale),t=i18n.t
watch(()=>props.locale,value=>i18n.setLocale(value))
const api=createPlatformLotteryClient('platform',()=> '')
const form=ref<DrawSchedule>({version:0,enabled:true,mode:'daily',draw_time:'',open_before_hours:24,close_before_seconds:15,collect_after_seconds:0})
const busy=ref(false),loaded=ref(false),failed=ref(false),pending=ref<{key:string;body:DrawSchedule}|null>(null)
let generation=0,alive=true
async function load(){const g=++generation;busy.value=true;failed.value=false;try{const result=await api.automation(props.gameId);if(!alive||g!==generation)return;if(result.schedule)form.value={...result.schedule};else form.value={version:0,enabled:true,mode:props.timezone==='Asia/Hong_Kong'?'hkjc':'daily',draw_time:'',open_before_hours:24,close_before_seconds:15,collect_after_seconds:0};loaded.value=true}catch(cause){if(alive&&g===generation){failed.value=true;showOperationToast(()=>t('loadFailure'),'error');if(cause instanceof PlatformLotteryError&&cause.status===401)emit('failure',cause)}}finally{if(alive&&g===generation)busy.value=false}}
async function save(){
 if(busy.value)return
 if(!pending.value)pending.value={key:crypto.randomUUID(),body:{...form.value}}
 const snapshot=pending.value,g=generation;busy.value=true
 try{const result=await api.write('/'+props.gameId+'/draw-schedule',snapshot.body,snapshot.key) as DrawSchedule;if(!alive||g!==generation)return;form.value=result;pending.value=null;showOperationToast(()=>t('saved'),'success');emit('saved')}
 catch(cause){if(!alive||g!==generation)return;if(cause instanceof PlatformLotteryError&&cause.status>=400&&cause.status<500)pending.value=null;showOperationToast(()=>t(cause instanceof PlatformLotteryError&&cause.code==='DRAW_CALENDAR_UNAVAILABLE'?'calendarMissing':'saveFailure'),'error');if(cause instanceof PlatformLotteryError&&cause.status===401)emit('failure',cause)}finally{if(alive&&g===generation)busy.value=false}
}
watch(()=>props.gameId,()=>{pending.value=null;loaded.value=false;void withGlobalLoading(load)},{immediate:true})
onUnmounted(()=>{alive=false;generation++})
</script>
<template>
<section class="draw-schedule-settings" data-testid="draw-schedule-settings" :aria-label="t('title')">
 <form class="ui-form" v-if="loaded" v-inline-validation v-write-guard="busy" novalidate @submit.prevent="withGlobalLoading(save)">
 <fieldset :disabled="busy||!!pending||!canWrite"><label class="check"><input type="checkbox" v-model="form.enabled" />{{t('enabled')}}</label>
 <div class="schedule-fields"><label>{{t('mode')}}<SelectControl v-model="form.mode" :label="t('mode')" :options="[{value:'daily',label:t('daily')},...(timezone==='Asia/Hong_Kong'?[{value:'hkjc',label:t('hkjc')}]:[])]" /></label>
 <label>{{t('drawTime')}}<PlatformTimeInput v-model="form.draw_time" :display-settings="platformDisplaySettings!" :label="t('drawTime')" :locale="locale" /></label>
 <label>{{t('openBefore')}}<input v-model.number="form.open_before_hours" type="number" min="0.001" max="8760" step="any" required /></label>
 <label>{{t('closeBefore')}}<input v-model.number="form.close_before_seconds" type="number" min="0" :max="form.open_before_hours*3600-1" step="1" required /></label>
 <label>{{t('collectAfter')}}<input v-model.number="form.collect_after_seconds" type="number" min="0" max="86400" step="1" required /></label></div></fieldset>
 <p v-if="pending" role="status">{{t('pending')}}</p><button v-if="canWrite" class="button button-primary" type="submit" :disabled="busy">{{t(pending?'retry':'save')}}</button>
 </form><button v-if="failed&&!pending" class="button button-secondary" @click="withGlobalLoading(load)">{{t('reload')}}</button>
</section>
</template>
<style scoped>
.draw-schedule-settings form>.button{margin-top:10px}
.draw-schedule-settings{min-width:0;font-size:var(--body-font-size)}.draw-schedule-settings h3{font-size:16px;margin:0}.draw-schedule-settings p{line-height:1.5;overflow-wrap:anywhere}.draw-schedule-settings fieldset{border:0;margin:0;padding:0;min-width:0}.schedule-fields{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px}.draw-schedule-settings label{display:grid;gap:6px;min-width:0}.draw-schedule-settings input[type=number]{width:100%;min-width:0;box-sizing:border-box;font:inherit;color:var(--ink);background:var(--surface);border:1px solid var(--line);border-radius:var(--radius-sm);min-height:var(--control-height);padding:4px 8px}.draw-schedule-settings .check{display:flex;align-items:center;gap:8px;min-height:44px;margin-bottom:10px}.check input{width:18px;height:18px}@media(max-width:620px){.schedule-fields{grid-template-columns:minmax(0,1fr)}}
</style>
