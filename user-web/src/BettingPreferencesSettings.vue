<script setup lang="ts">
import {ref,watch,onUnmounted} from 'vue'
import {createCatalogI18n,configuredRuleCatalog,vWriteGuard,withGlobalLoading} from '@lottery/shared'
import Toast from '@lottery/shared/Toast.vue'
import SiteIcon from '@lottery/shared/SiteIcon'
import {createQuickAmountsApi,validQuickAmount} from './betting-preferences'
const props=defineProps<{brandId:string;memberId:string;brandCode?:string;locale:'en'|'zh-CN'}>()
const i18n=createCatalogI18n(configuredRuleCatalog,props.locale),t=i18n.t
watch(()=>props.locale,value=>i18n.setLocale(value))
const amounts=ref<string[]>(Array(5).fill('')),confirmed=ref<string[]>([])
const error=ref<'preferencesReadFailed'|'quickAmountInvalid'|'preferencesSaveFailed'|null>(null),readFailed=ref(false),saved=ref(false),saving=ref(false),loading=ref(false)
let generation=0,revision=0,timer:ReturnType<typeof setTimeout>|undefined
onUnmounted(()=>{generation++;clearTimeout(timer)})
async function load(){
 const token=++generation;clearTimeout(timer);loading.value=true;saving.value=false;amounts.value=Array(5).fill('');confirmed.value=[]
 saved.value=false;error.value=null;readFailed.value=false
 try{const values=await createQuickAmountsApi({brandCode:props.brandCode}).read(props.brandId,props.memberId);if(token===generation){confirmed.value=values;amounts.value=[...values,...Array(5-values.length).fill('')]}}
 catch{if(token===generation){readFailed.value=true;error.value='preferencesReadFailed'}}finally{if(token===generation)loading.value=false}
}
watch(()=>[props.brandId,props.memberId,props.brandCode],load,{immediate:true})
function scheduleSave(){clearTimeout(timer);timer=setTimeout(()=>{timer=undefined;void save()},400)}
function editAmount(index:number,event:Event){
 const input=event.target as HTMLInputElement,value=input.value
 saved.value=false
 if(value.length>6||/[^0-9]/.test(value)){error.value='quickAmountInvalid';input.value=amounts.value[index]!;return}
 amounts.value[index]=value;revision++;saved.value=false;scheduleSave()
}
async function save(){
 clearTimeout(timer);timer=undefined
 if(saving.value||loading.value||readFailed.value)return
 const values=amounts.value.filter(value=>value!==''),token=generation,version=revision
 saved.value=false;error.value=null
 if(!values.every(validQuickAmount)){error.value='quickAmountInvalid';return}
 if(JSON.stringify(values)===JSON.stringify(confirmed.value))return
 saving.value=true
 try{
  const result=await createQuickAmountsApi({brandCode:props.brandCode}).save(props.brandId,props.memberId,values)
  if(token===generation){confirmed.value=result;saved.value=revision===version}
 }catch{if(token===generation)error.value='preferencesSaveFailed'}finally{if(token===generation){saving.value=false;if(revision!==version)scheduleSave()}}
}
async function removeAmount(index:number){amounts.value.splice(index,1);amounts.value.push('');revision++;await save()}
</script>
<template>
 <section class="detail-panel betting-preferences" data-testid="betting-preferences">
  <h2>{{t('personalSettings')}}</h2><h3>{{t('quickAmounts')}}</h3>
  <p v-if="error&&error!=='quickAmountInvalid'" class="message error" role="alert">{{t(error)}} <button v-if="readFailed" type="button" class="button button-secondary" :disabled="loading" @click="withGlobalLoading(load)">{{t('retryRead')}}</button><button v-else-if="error==='preferencesSaveFailed'" v-write-guard="saving" type="button" class="button button-secondary" :disabled="saving" @click="withGlobalLoading(save)">{{t('retryRead')}}</button></p>
  <div class="stake-chips quick-amount-chips" :aria-label="t('quickAmounts')">
   <div v-for="(_,index) in amounts" :key="index" class="stake-chip quick-amount-chip" :style="{'--chip-digits':amounts[index]!.length}" :class="{'quick-amount-chip-empty':amounts[index]===''}">
    <input class="stake-chip-core" :aria-label="t('quickAmountNumber',{number:index+1})" :value="amounts[index]" @input="editAmount(index,$event)" @change="withGlobalLoading(save)" @keydown.enter.prevent="($event.target as HTMLInputElement).blur()" type="text" inputmode="numeric" pattern="[1-9][0-9]{0,5}" :disabled="readFailed||loading" />
    <button v-if="amounts[index]!==''" v-write-guard="saving" type="button" class="chip-delete" :disabled="readFailed||saving||loading" :aria-label="t('removeQuickAmount',{number:index+1})" @pointerdown.prevent @click="withGlobalLoading(removeAmount,index)"><SiteIcon name="close" /></button>
   </div>
  </div>
  <Toast v-if="error==='quickAmountInvalid'" tone="error" :title="t('quickAmounts')" :message="t('quickAmountInvalid')" :close-label="t('dismissNotice')" @dismiss="error=null" />
  <Toast v-if="saved&&error!=='quickAmountInvalid'" :title="t('preferencesSaved')" :message="t('quickAmountsSaved')" :close-label="t('dismissNotice')" @dismiss="saved=false" />
 </section>
</template>
<style src="@lottery/shared/quick-stake-chips.css"></style>
<style scoped>
.betting-preferences{display:grid;gap:10px;min-width:0}.betting-preferences h2,.betting-preferences h3,.betting-preferences p{margin:0}.betting-preferences h2{font-size:18px}.betting-preferences h3{font-size:14px}
:global(#app .betting-preferences .quick-amount-chips){display:flex;align-items:center;flex-wrap:wrap;gap:16px;padding:12px;min-width:0}
:global(#app .quick-amount-chips .quick-amount-chip){--chip-base-size:72px;font-size:16px}
:global(#app .quick-amount-chips .quick-amount-chip input.stake-chip-core){font-size:16px;min-height:44px}
.quick-amount-chip-empty{opacity:.65}.quick-amount-chip-empty:focus-within{opacity:1}
.chip-delete{position:absolute;top:-12px;right:-12px;z-index:1;display:grid;place-items:center;width:44px;min-width:44px;height:44px;min-height:44px;padding:0;border:0;background:transparent;color:var(--danger);cursor:pointer}
.chip-delete .site-icon{width:22px;height:22px;padding:2px;box-sizing:border-box;border:1px solid var(--surface);border-radius:50%;background:var(--danger);color:var(--lottery-ball-ink)}
.chip-delete:disabled{cursor:wait;opacity:.5}
</style>
