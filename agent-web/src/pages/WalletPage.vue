<script setup lang="ts">
import {onMounted,ref} from 'vue'
import {formatPoints,useOperationToast,withGlobalLoading} from '@lottery/shared'
import SummaryFacts from '@lottery/shared/SummaryFacts.vue'
import CashWallet from '../../../shared/src/CashWallet.vue'
import {AgentError,request} from '../api'
import {i18n} from '../i18n'
import type {Currency} from '../../../shared/src/cash'
import type {Wallet} from '../types'
defineProps<{member:{id:string;brand_id:string};mode:'overview'|'withdrawal'}>()
const emit=defineEmits<{'session-invalid':[];deposit:[];withdrawal:[];'manage-accounts':[currency:Currency]}>()
const {t,locale}=i18n
const wallet=ref<Wallet|null>(null),error=ref<unknown>(null)
async function load(){error.value=null;try{wallet.value=await request<Wallet>('/wallet')}catch(e){error.value=e;if(e instanceof AgentError&&e.status===401)emit('session-invalid')}}
async function readAvailablePoints(){return (await request<Wallet>('/wallet')).available_points}
onMounted(load)
useOperationToast(error,v=>i18n.errorText(v),'error')
</script>
<template>
 <section class="console-stack agent-wallet">
  <div v-if="wallet" class="management-list-summary"><SummaryFacts :fields="[{label:t('points'),value:formatPoints(wallet.available_points,locale)},{label:t('commission'),value:formatPoints(wallet.commission_points,locale)}]" /></div>
  <button v-if="error" type="button" class="text-button" v-refresh-button :aria-label="t('retryRead')" @click="withGlobalLoading(load)">{{t('retryRead')}}</button>
  <CashWallet api-base="/api/v1/agent" :read-available-points="readAvailablePoints" :member="member" :mode="mode" agent :t="t" :locale="locale" :error-text="i18n.errorText" @auth-expired="emit('session-invalid')" @deposit="emit('deposit')" @withdrawal="emit('withdrawal')" @exchanged="load" @manage-accounts="emit('manage-accounts', $event)" />
 </section>
</template>
