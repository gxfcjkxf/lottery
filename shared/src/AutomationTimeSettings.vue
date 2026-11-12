<script setup lang="ts">
import {computed,onUnmounted,ref,watch} from 'vue'
import {createPlatformLotteryClient,PlatformLotteryError} from './platform-lottery'
import {automationCatalog,type AutomationPlan} from './automation'
import {drawScheduleCatalog} from './draw-schedule'
import {createCatalogI18n} from './localization'
import {withGlobalLoading} from './global-loading'
import {vWriteGuard} from './write-guard'
import {vInlineValidation,formValidationText} from './form-validation'
import {showOperationToast} from './operation-toast'
import PlatformTemporalInput from './PlatformTemporalInput.vue'
const props=defineProps<{gameId:string;locale:'en'|'zh-CN';canWrite:boolean}>()
const emit=defineEmits<{failure:[cause:unknown]}>()
const catalog={en:{...automationCatalog.en,...drawScheduleCatalog.en,openBefore:automationCatalog.en.openBefore,notConfigured:automationCatalog.en.notConfigured},'zh-CN':{...automationCatalog['zh-CN'],...drawScheduleCatalog['zh-CN'],openBefore:automationCatalog['zh-CN'].openBefore,notConfigured:automationCatalog['zh-CN'].notConfigured}}
const i18n=createCatalogI18n(catalog,props.locale),t=i18n.t
watch(()=>props.locale,locale=>i18n.setLocale(locale))
const api=createPlatformLotteryClient('platform',()=> '')
const plan=ref<AutomationPlan|null>(null),busy=ref(false),loaded=ref(false),failed=ref(false),anchor=ref('')
const upstream=computed(()=>plan.value?.mapping?.schedule==='source')
type Input={version:number;open_before_seconds:number|null;close_before_seconds:number;anchor_draw_at:string|null;anchor_period_no:string|null;interval_seconds:number|null}
const pending=ref<{body:Input;key:string}|null>(null)
let generation=0,alive=true
function assign(value:AutomationPlan){plan.value={...value};anchor.value=value.anchor_draw_at?new Date(value.anchor_draw_at).toISOString().slice(0,19):''}
async function load(){const g=++generation;busy.value=true;failed.value=false;loaded.value=false;plan.value=null;try{const out=await api.automation(props.gameId);if(!alive||g!==generation)return;if(out.plan&&['source','interval'].includes(out.plan.mapping?.schedule??''))assign(out.plan);loaded.value=true}catch(cause){if(alive&&g===generation){failed.value=true;showOperationToast(()=>t('loadFailure'),'error');if(cause instanceof PlatformLotteryError&&cause.status===401)emit('failure',cause)}}finally{if(alive&&g===generation)busy.value=false}}
async function save(){
 if(busy.value||!plan.value||!props.canWrite)return
 if(!pending.value){const p=plan.value;pending.value={key:crypto.randomUUID(),body:{version:p.version,open_before_seconds:p.open_before_seconds,close_before_seconds:p.close_before_seconds,anchor_draw_at:upstream.value?null:new Date(anchor.value+'Z').toISOString(),anchor_period_no:upstream.value?null:p.anchor_period_no,interval_seconds:upstream.value?null:p.interval_seconds}}}
 const intent=pending.value,g=generation;busy.value=true
 try{const out=await api.write('/'+props.gameId+'/automation-timing',intent.body,intent.key) as AutomationPlan;if(!alive||g!==generation)return;assign(out);pending.value=null;showOperationToast(()=>t('saved'),'success')}
 catch(cause){if(!alive||g!==generation)return;if(cause instanceof PlatformLotteryError&&cause.status>=400&&cause.status<500){pending.value=null;failed.value=true};showOperationToast(()=>t('saveFailure'),'error');if(cause instanceof PlatformLotteryError&&cause.status===401)emit('failure',cause)}finally{if(alive&&g===generation)busy.value=false}
}
watch(()=>props.gameId,()=>{pending.value=null;void withGlobalLoading(load)},{immediate:true})
onUnmounted(()=>{alive=false;generation++})
</script>
<template>
<section class="automation-time-settings" data-testid="automation-time-settings" :aria-label="t('title')">
<form v-if="plan" class="ui-form" v-inline-validation v-write-guard="busy" novalidate @submit.prevent="withGlobalLoading(save)">
<fieldset :disabled="busy||!!pending||!canWrite||failed">
<div class="timing-fields">
<template v-if="!upstream"><label>{{t('anchor')}}<PlatformTemporalInput v-model="anchor" type="datetime-local" required :locale="locale" /></label><label>{{t('periodNumber')}}<input v-model="plan.anchor_period_no" pattern="[0-9]{1,40}" inputmode="numeric" :data-validation-format="formValidationText('periodFormat')" required /></label><label>{{t('interval')}}<input v-model.number="plan.interval_seconds" type="number" min="10" max="31536000" required /></label></template>
<label>{{t('openBefore')}}<input v-model.number="plan.open_before_seconds" type="number" :min="plan.close_before_seconds+1" :max="upstream?31536000:plan.interval_seconds??undefined" step="1" required /></label>
<label>{{t('closeBefore')}}<input v-model.number="plan.close_before_seconds" type="number" min="0" :max="plan.open_before_seconds===null?0:Math.min(upstream?86400:plan.open_before_seconds-1,plan.open_before_seconds-1)" step="1" required /></label>
</div></fieldset>
<p v-if="pending" role="status">{{t('pending')}}</p>
<div class="ui-form-actions"><button v-if="canWrite" type="submit" class="button button-primary" :disabled="busy||failed">{{t(pending?'retry':'save')}}</button></div>
</form>
<p v-else-if="loaded">{{t('notConfigured')}}</p>
<button v-if="failed&&!pending" type="button" class="button button-secondary" @click="withGlobalLoading(load)">{{t('reload')}}</button>
</section>
</template>
<style scoped>
.automation-time-settings{min-width:0;font-size:var(--body-font-size)}fieldset{border:0;padding:0;margin:0;min-width:0}.timing-fields{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px}label{display:grid;gap:6px;min-width:0}input[type=number],input[inputmode=numeric]{box-sizing:border-box;width:100%;min-width:0;min-height:var(--control-height);padding:4px 8px;font:inherit;color:var(--ink);background:var(--surface);border:1px solid var(--line);border-radius:var(--radius-sm)}.check{display:flex;align-items:center;gap:8px;min-height:var(--control-height);margin-bottom:10px}.check input{width:18px;height:18px}@media(max-width:900px),(pointer:coarse){.timing-fields{gap:8px}label{gap:4px}.check{margin-bottom:8px}}@media(max-width:620px){.timing-fields{grid-template-columns:minmax(0,1fr)}}
</style>
