<script setup lang="ts">
import {onMounted,ref} from 'vue'
import {vInlineValidation,vWriteGuard,vRefreshButton,withGlobalLoading,useOperationToast,formatPoints} from '@lottery/shared'
import SummaryFacts from '@lottery/shared/SummaryFacts.vue'
import DetailModal from '@lottery/shared/DetailModal.vue'
import WriteFormActions from '@lottery/shared/WriteFormActions.vue'
import {AgentError,request,pendingAgentTransfers,type PendingAgentTransfer} from './api'
import {i18n} from './i18n'
import type {Wallet,Member} from './types'
const props=defineProps<{ownerKey:string;member:Member}>()
const emit=defineEmits<{'session-invalid':[];close:[];transferred:[]}>()
const {t,locale}=i18n
const wallet=ref<Wallet|null>(null),amount=ref(''),busy=ref(false),error=ref<string | import('@lottery/shared').LocalizedMessage>(''),notice=ref('')
const pending=ref<PendingAgentTransfer|null>(pendingAgentTransfers.get(props.ownerKey)??null)
function fail(e:unknown){error.value=i18n.errorMessage(e);if(e instanceof AgentError&&e.status===401)emit('session-invalid')}
async function load(){try{wallet.value=await request<Wallet>('/wallet')}catch(e){fail(e)}}
async function transfer(){if(busy.value||!wallet.value)return;if(!pending.value)pending.value={body:{member_id:props.member.id,amount:amount.value},account:props.member.username,key:crypto.randomUUID(),sent:false};busy.value=true;error.value='';pending.value.sent=true;pendingAgentTransfers.set(props.ownerKey,pending.value)
 try{await request('/transfers',pending.value.body,pending.value.key);pending.value=null;pendingAgentTransfers.delete(props.ownerKey);amount.value='';notice.value=t('transferSuccess');emit('transferred');emit('close')}catch(e){if(e instanceof AgentError&&e.status>=400&&e.status<500){pending.value=null;pendingAgentTransfers.delete(props.ownerKey)};fail(e)}finally{busy.value=false}}
onMounted(()=>{if(pending.value)amount.value=pending.value.body.amount;void load()})
useOperationToast(error,v=>i18n.displayError(v),'error')
useOperationToast(notice,v=>v,'success')
</script>
<template>
 <DetailModal class="ui-form-dialog agent-transfer" :title="t('transfer')" :close-label="t('close')" :close-disabled="busy||!!pending?.sent" @close="emit('close')">
  <div class="console-stack">
   <SummaryFacts :fields="[{label:t('transferMember'),value:pending?.account??member.username},...(wallet?[{label:t('transferBalance'),value:formatPoints(wallet.available_points,locale)}]:[])]" />
   <button v-if="error&&!wallet" type="button" class="text-button" v-refresh-button :aria-label="t('retryRead')" @click="withGlobalLoading(load)">{{t('retryRead')}}</button>
   <form class="ui-form" novalidate v-inline-validation v-write-guard="busy" @submit.prevent="withGlobalLoading(transfer)">
    <fieldset :disabled="busy||!!pending||!wallet">
     <label><span>{{t('transferAmount')}}</span><input v-model="amount" required inputmode="decimal" pattern="[0-9]+([.][0-9]{1,2})?" :data-validation-format="t('cash.numberFormat',{digits:2})" /></label>
    </fieldset>
    <p v-if="pending" role="status">{{t('unknown')}}</p>
    <WriteFormActions :submit-label="t(pending?'retry':'transferConfirm')" :busy="busy" :invalid="!wallet" />
   </form>
  </div>
 </DetailModal>
</template>
