<script setup lang="ts">
import { ref, watch } from 'vue'
import DetailModal from './DetailModal.vue'
import PlatformTemporalInput from './PlatformTemporalInput.vue'
import { createCatalogI18n } from './localization'
import { createPlatformLotteryClient, PlatformLotteryError, platformLotteryCatalog, type PlatformPeriod } from './platform-lottery'
import { lotteryLocalTimeToISO, lotteryISOToLocalTime } from './lottery-time'
import { platformDisplaySettings } from './display-settings'
import { vInlineValidation } from './form-validation'
import { withGlobalLoading } from './global-loading'
import { vWriteGuard } from './write-guard'
import { showOperationToast } from './operation-toast'
const props=defineProps<{period:PlatformPeriod;locale:'en'|'zh-CN'}>()
const emit=defineEmits<{close:[];saved:[];failure:[cause:unknown]}>()
const i18n=createCatalogI18n(platformLotteryCatalog,props.locale),t=i18n.t
watch(()=>props.locale,value=>i18n.setLocale(value))
const opening=ref(lotteryISOToLocalTime(props.period.bet_start_at,platformDisplaySettings.value!.timezone))
const busy=ref(false),pending=ref<{key:string;body:{bet_start_at:string;expected_bet_start_at:string}}|null>(null)
const api=createPlatformLotteryClient('platform',()=> '')
async function save(){
 if(busy.value)return
 if(!pending.value){
  try{
   const start=lotteryLocalTimeToISO(opening.value,platformDisplaySettings.value!.timezone)
   if(Date.parse(start)>=Date.parse(props.period.bet_end_at))throw new Error('Opening must precede closing')
   pending.value={key:crypto.randomUUID(),body:{bet_start_at:start,expected_bet_start_at:props.period.bet_start_at}}
  }catch{showOperationToast(()=>t('openingInvalid'),'error');return}
 }
 busy.value=true
 try{
  await api.write(`/periods/${props.period.id}/opening`,pending.value.body,pending.value.key)
  showOperationToast(()=>t('success'),'success');emit('saved')
 }catch(cause){
  if(cause instanceof PlatformLotteryError&&cause.status>=400&&cause.status<500)pending.value=null
  const message=i18n.errorMessage(cause)
  showOperationToast(()=>i18n.displayError(message),'error');if(cause instanceof PlatformLotteryError&&cause.status===401)emit('failure',cause)
 }finally{busy.value=false}
}
</script>
<template>
 <DetailModal :title="t('editOpening')" :close-label="t('close')" :close-disabled="busy||!!pending" @close="emit('close')">
  <form v-inline-validation v-write-guard="busy" novalidate class="period-opening-form ui-form" @submit.prevent="withGlobalLoading(save)">
   <p>{{t('periodNo')}}: {{period.period_no}}</p><p>{{t('openingHelp')}}</p>
   <label>{{t('start')}}<PlatformTemporalInput v-model="opening" type="datetime-local" :locale="locale" required :disabled="busy||!!pending" /></label>
   <p v-if="pending&&!busy">{{t('awaiting')}}</p>
   <div class="form-actions ui-form-actions"><button type="submit" class="button button-primary" :disabled="busy">{{t(pending?'retry':'save')}}</button></div>
  </form>
 </DetailModal>
</template>
<style scoped>
.period-opening-form{display:grid;gap:12px;min-width:0}.period-opening-form p{margin:0;overflow-wrap:anywhere}.period-opening-form label{display:grid;gap:6px;min-width:0}
</style>
