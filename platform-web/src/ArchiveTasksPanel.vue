<script setup lang="ts">
import {vInlineValidation} from "@lottery/shared";
import {computed,onBeforeUnmount,onMounted,ref,shallowRef} from 'vue';
import {archiveScheduleCopy,archiveScheduleOf,sameArchiveSchedule,validArchiveSchedule,withGlobalLoading,useOperationToast,vWriteGuard,type LocalizedMessage} from '@lottery/shared';
import ArchiveScheduleForm from '@lottery/shared/ArchiveScheduleForm.vue';
import {usePlatformI18n} from './i18n';
import {PlatformApiError} from './platform-api';
import {createPlatformArchivePolicyApi,getPlatformArchiveIntent,setPlatformArchiveIntent,discardPlatformArchiveIntent,type PlatformArchivePolicy,type PlatformArchiveInput,type PendingPlatformArchive} from './platform-archive-policy-api';
const props=defineProps<{accountId:string;canManage:boolean;locale:'en'|'zh-CN'}>();
const emit=defineEmits<{failure:[cause:unknown]}>();
const {errorMessage,displayError}=usePlatformI18n(),copy=computed(()=>archiveScheduleCopy(props.locale));
const api=createPlatformArchivePolicyApi(),policy=ref<PlatformArchivePolicy|null>(null),draft=ref<PlatformArchiveInput|null>(null),pending=shallowRef<PendingPlatformArchive|null>(getPlatformArchiveIntent(props.accountId));
const error=ref<string|LocalizedMessage>(''),notice=ref(''),busy=ref(false);let alive=true,ticket=0;
const unchanged=computed(()=>!!policy.value && !!draft.value && sameArchiveSchedule(policy.value,draft.value)&&policy.value.daily_enabled===draft.value.daily_enabled&&policy.value.monthly_enabled===draft.value.monthly_enabled);
async function load(){const id=++ticket;busy.value=true;error.value='';try{const result=await api.read();if(!alive||id!==ticket)return;policy.value=result;draft.value={...archiveScheduleOf(result),version:result.version,daily_enabled:result.daily_enabled,monthly_enabled:result.monthly_enabled};}catch(cause){if(alive&&id===ticket){error.value=errorMessage(cause);emit('failure',cause)}}finally{if(alive&&id===ticket)busy.value=false}}
async function save(){
 if(!props.canManage||busy.value||pending.value?.conflict||!draft.value||!validArchiveSchedule(draft.value))return;
 const intent=pending.value??{actor:props.accountId,key:crypto.randomUUID(),body:Object.freeze({...draft.value}),conflict:false};setPlatformArchiveIntent(intent);pending.value=intent;busy.value=true;error.value='';
 try{const result=await api.update(intent.body,intent.key,intent.actor);if(getPlatformArchiveIntent(intent.actor)!==intent)return;discardPlatformArchiveIntent(intent.actor,intent);if(!alive)return;pending.value=null;policy.value=result;draft.value={...archiveScheduleOf(result),version:result.version,daily_enabled:result.daily_enabled,monthly_enabled:result.monthly_enabled};notice.value=copy.value.saved;await load();}
 catch(cause){if(!alive||getPlatformArchiveIntent(intent.actor)!==intent)return;
  if(cause instanceof PlatformApiError && cause.status>=400 && cause.status<500){if(cause.status===409){intent.conflict=true;}else{discardPlatformArchiveIntent(intent.actor,intent);pending.value=null;}}
  error.value=errorMessage(cause);emit('failure',cause);
 }finally{if(alive)busy.value=false}
}
async function discard(){const intent=pending.value;if(!intent?.conflict)return;await load();if(!error.value){discardPlatformArchiveIntent(intent.actor,intent);pending.value=null;}}
onMounted(load);onBeforeUnmount(()=>{alive=false;ticket++});useOperationToast(()=>notice.value,value=>value,'success');

useOperationToast(error, value => displayError(value), 'error');
</script>
<template>
 <section class="panel" data-testid="platform-archive-policy">

  <button v-if="error && !pending" class="button button-secondary" :disabled="busy" @click="withGlobalLoading(load)">{{ copy.reload }}</button>
  <form v-inline-validation novalidate v-if="draft && policy" v-write-guard="busy" class="archive-platform-form ui-form" @submit.prevent="withGlobalLoading(save)">
   <ArchiveScheduleForm v-model="draft" :locale="locale" :disabled="busy||!canManage||Boolean(pending)||Boolean(error)" :next-runs="unchanged?policy.next_runs:undefined" />
   <p v-if="!validArchiveSchedule(draft)" class="message error">{{ copy.invalid }}</p>
   <p v-if="pending && !pending.conflict" class="message">{{ copy.unknown }}</p>
   <div class="ui-form-actions">
   <button v-if="canManage && !pending?.conflict" type="submit" class="button button-primary" :disabled="busy||!validArchiveSchedule(draft)||Boolean(error)&&!pending||unchanged&&!pending">{{ pending?copy.retry:copy.save }}</button>
   <button v-if="pending?.conflict" type="button" class="button button-secondary" :disabled="busy" @click="withGlobalLoading(discard)">{{ copy.discardConflict }}</button>
   </div>
  </form>
 </section>
</template>
<style scoped>.archive-platform-form{display:grid;gap:12px;min-width:0}</style>
