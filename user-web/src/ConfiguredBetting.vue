<script setup lang="ts">
import { vGlobalLoading, vWriteGuard } from "@lottery/shared";
import { computed,onMounted,onUnmounted,ref,shallowRef,watch } from 'vue'
import { RouterLink } from 'vue-router'
import {createPlatformLotteryClient,type PlatformPeriod} from '../../shared/src/platform-lottery'
import {readBettingPreferences,writeBettingPreferences,createQuickAmountsApi,validBetAmount,type BettingPreferences} from './betting-preferences'
import BettingSheet from './BettingSheet.vue'
import PlayRules from './PlayRules.vue'
import UserPeriodStrip from './UserPeriodStrip.vue'
import {WALLET_SNAPSHOT_EVENT,type Wallet,createWalletApi} from './wallet-api'
import ConfiguredSelection from '@lottery/shared/ConfiguredSelection.vue'
import {configuredNumberWave,createConfiguredBusinessClient,configuredTicketLabel,configuredTicketPrices,createCatalogI18n,configuredRuleCatalog,withGlobalLoading,type ConfiguredLiveCatalog,type ConfiguredLiveInput,type ConfiguredLiveQuote,type ConfiguredSnapshot,type ConfiguredRequest,type ConfiguredQuotedLine,type LocalizedMessage} from '@lottery/shared'
import type {AuthProfile} from '../../shared/src/auth'
const props=defineProps<{gameId:string;brandCode?:string;locale:'en'|'zh';auth?:AuthProfile|null}>()
const emit=defineEmits<{'auth-expired':[];'game-title':[title:string]}>()
const localeCode=computed(()=>props.locale==='en'?'en':'zh-CN');const i18n=createCatalogI18n(configuredRuleCatalog,localeCode.value);watch(localeCode,v=>i18n.setLocale(v));const t=i18n.t
const api=computed(()=>createConfiguredBusinessClient('user','',props.brandCode)),data=shallowRef<ConfiguredLiveCatalog|null>(null),quote=shallowRef<ConfiguredLiveQuote|null>(null),review=shallowRef<ConfiguredLiveInput|null>(null)
const wallet=shallowRef<Wallet|null>(null),walletFailed=ref(false)
let walletGeneration=0
const periods=shallowRef<PlatformPeriod[]>([])
const previousDraw=computed(()=>periods.value.find(period=>period.result!==null))
// Closed periods leave the sales catalog but remain visible until their draw.
const displayPeriod=computed(()=>data.value?.period??periods.value.find(period=>!period.result&&Date.parse(period.bet_start_at)<=now.value))
const previousNumbers=computed(()=>{const result=previousDraw.value?.result;return result&&data.value?[...result.regular.map(value=>({value,special:false})),...result.special.map(value=>({value,special:true})),...result.digits.map(value=>({value,special:false}))].map(number=>({...number,wave:configuredNumberWave(data.value!.snapshot,number.value)})):[]})
const request=ref<ConfiguredRequest>({play_id:'',options:[],plate:'',points:'',multiplier:'1'}),error=ref<LocalizedMessage|null>(null),notice=ref(false),busy=ref(false)
type Intent={owner:string;key:string;body:ConfiguredLiveInput;quote:ConfiguredLiveQuote;snapshot:ConfiguredSnapshot}
const pending=shallowRef<Intent|null>(null),intentError=ref(false),reviewOpen=ref(false),selectionInvalid=ref(false),pointsInvalid=ref(false)
const owner=computed(()=>`${props.brandCode??''}:${props.auth?.user.id??''}:${props.auth?.member.id??''}`)
const storageKey=computed(()=>`lottery.configured.intent:${owner.value}:${props.gameId}`)
const quickAmounts=ref<string[]>([])
const preferences=shallowRef<BettingPreferences>({lastPoints:''}),preferencesFailed=ref(false)
watch(owner,()=>{
 preferencesFailed.value=false;preferences.value={lastPoints:''};request.value={play_id:'',options:[],plate:'',points:'',multiplier:'1'}
 if(!props.auth)return
 try{preferences.value=readBettingPreferences(localStorage,props.auth.member.brand_id,props.auth.member.id)}catch{preferencesFailed.value=true}
},{immediate:true})
watch(()=>request.value.points,value=>{
 if(!props.auth||preferencesFailed.value||currentPlay.value?.stake_mode!=='per_line'||(value!==''&&!validBetAmount(value))||preferences.value.lastPoints===value)return
 try{const next={...preferences.value,lastPoints:value};writeBettingPreferences(localStorage,props.auth.member.brand_id,props.auth.member.id,next);preferences.value=next}catch{preferencesFailed.value=true}
})
function acceptWallet(value:Wallet){
 if(value.brand_id!==props.auth?.member.brand_id||value.member_id!==props.auth?.member.id)return false
 if(wallet.value&&value.version<wallet.value.version)return true
 walletGeneration++;wallet.value=value
 walletFailed.value=false
 return true
}
function walletSnapshot(event:Event){acceptWallet((event as CustomEvent<Wallet>).detail)}
async function refreshWallet(){
 if(!props.auth)return
 const token=++walletGeneration
 try{const value=await createWalletApi({brandCode:props.brandCode}).wallet();if(token!==walletGeneration)return;if(!acceptWallet(value))throw new Error('Wallet identity mismatch')}
 catch(cause){if(token!==walletGeneration)return;wallet.value=null;walletFailed.value=true;if((cause as {status?:number}).status===401)emit('auth-expired')}
}
watch(owner,()=>{walletGeneration++;wallet.value=null;walletFailed.value=false;void refreshWallet()},{immediate:true})
const quoteInsufficient=computed(()=>{if(!quote.value||!wallet.value||!data.value)return false;const needed=data.value.snapshot.catalog.funding==='reserve'?quote.value.reserved_points!:quote.value.bet_points;return BigInt(needed)>BigInt(wallet.value.available_points)})
watch(quoteInsufficient, value=>{if(value&&!pending.value)reviewOpen.value=false})
let generation=0
let syncTimer:ReturnType<typeof setTimeout>|undefined
let clockTimer:ReturnType<typeof setInterval>|undefined
const now=ref(Date.now())
function scheduleSync(){
 clearTimeout(syncTimer)
 if(busy.value||pending.value||error.value||document.visibilityState!=='visible')return
 const current=Date.now(),period=data.value?.period
 const upcoming=[...(period?[period.bet_start_at,period.bet_end_at]:[]),...periods.value.filter(p=>!p.result).map(p=>p.draw_at)].map(Date.parse).filter(time=>time>current)
 // During an open period, read again only at its next time boundary.
 // Without an upcoming boundary, check once a minute for the next period.
 const delay=upcoming.length?Math.min(...upcoming)-current+100:60000
 syncTimer=setTimeout(()=>{now.value=Date.now();void load(true)},delay)
}
function resumeSync(){if(document.visibilityState==='visible'&&!busy.value&&!pending.value){now.value=Date.now();void load(true);void refreshWallet()}else clearTimeout(syncTimer)}
onMounted(()=>{window.addEventListener(WALLET_SNAPSHOT_EVENT,walletSnapshot);document.addEventListener('visibilitychange',resumeSync);clockTimer=setInterval(()=>{now.value=Date.now()},1000)})
onUnmounted(()=>{generation++;walletGeneration++;window.removeEventListener(WALLET_SNAPSHOT_EVENT,walletSnapshot);clearTimeout(syncTimer);clearInterval(clockTimer);document.removeEventListener('visibilitychange',resumeSync)})
watch([busy,pending],scheduleSync)
const currentPlay=computed(()=>data.value?.snapshot.catalog.plays.find(play=>play.id===request.value.play_id))
const canBet=computed(()=>!!props.auth&&!!data.value?.period&&Date.parse(data.value.period.bet_start_at)<=now.value&&Date.parse(data.value.period.bet_end_at)>now.value&&data.value.sales.enabled&&data.value.game.status==='active'&&data.value.game.brand_enabled)
function report(cause:unknown,read=false){error.value=i18n.errorMessage(cause,read?'read':'write');if((cause as {status?:number}).status===401)emit('auth-expired')}
async function load(background=false){
 const token=++generation;clearTimeout(syncTimer);if(!background)busy.value=true;error.value=null;now.value=Date.now()
 try{
  try{const [value,detail,amounts]=await Promise.all([api.value.catalog(props.gameId),createPlatformLotteryClient('user',()=>'',fetch,props.brandCode).detail(props.gameId),createQuickAmountsApi({brandCode:props.brandCode}).read(props.auth!.member.brand_id,props.auth!.member.id)]);if(token!==generation)return;quickAmounts.value=amounts;periods.value=detail.periods;if(data.value&&(data.value.snapshot_hash!==value.snapshot_hash||data.value.period?.id!==value.period?.id)){quote.value=null;review.value=null}data.value=value;emit('game-title',value.game.name);if(!request.value.play_id||!value.snapshot.catalog.plays.some(p=>p.id===request.value.play_id)){const p=value.snapshot.catalog.plays[0]!;request.value={play_id:p.id,options:[],plate:value.snapshot.catalog.plates[0]!,points:p.stake_mode==='unit_multiplier'?'0':preferences.value.lastPoints,multiplier:'1'}}}catch(e){if(token===generation){data.value=null;periods.value=[];quote.value=null;review.value=null;report(e,true)}}
 }finally{if(token===generation){if(!background)busy.value=false;scheduleSync()}}
}
watch(()=>[props.gameId,owner.value],()=>{generation++;reviewOpen.value=false;selectionInvalid.value=false;pending.value=null;quote.value=null;review.value=null;data.value=null;periods.value=[];intentError.value=false;notice.value=false;try{const raw=sessionStorage.getItem(storageKey.value);if(raw!==null){const saved=JSON.parse(raw) as Intent;if(saved.owner!==owner.value||!saved.key||!saved.body.actor_context||!saved.body.requests.length||!saved.snapshot||!saved.quote)throw new Error('Invalid saved intent');pending.value=saved;reviewOpen.value=true}}catch(e){intentError.value=true;report(e)}void load()},{immediate:true})
watch(request,()=>{if(!pending.value){quote.value=null;review.value=null}},{deep:true})
async function preview(){if(busy.value)return;reviewOpen.value=true;selectionInvalid.value=false;pointsInvalid.value=false;error.value=null;if(pending.value)return;if(!data.value?.period||!canBet.value)return;if(request.value.options.length<currentPlay.value!.selection.min){selectionInvalid.value=true;return;}if(currentPlay.value!.stake_mode==='per_line'&&!validBetAmount(request.value.points)){pointsInvalid.value=true;return;}const token=generation;busy.value=true;error.value=null;notice.value=false;try{const body:ConfiguredLiveInput={period_id:data.value.period.id,rule_id:data.value.rule_id,sales_version:data.value.sales.version,snapshot_hash:data.value.snapshot_hash,actor_context:'',requests:[JSON.parse(JSON.stringify(request.value))]};const [result]=await Promise.all([api.value.preview(body),refreshWallet()]);if(token!==generation)return;if(result.snapshot_hash!==body.snapshot_hash||result.rule_id!==body.rule_id||result.sales_version!==body.sales_version)throw new Error('Quote version mismatch');review.value={...body,actor_context:result.actor_context};quote.value=result}catch(e){if(token===generation)report(e)}finally{if(token===generation)busy.value=false}}
async function place(){if(!props.auth||intentError.value)return;if(!pending.value){if(!review.value||!quote.value||!data.value||!wallet.value||quoteInsufficient.value)return;const intent:Intent={owner:owner.value,key:crypto.randomUUID(),body:JSON.parse(JSON.stringify(review.value)),quote:quote.value,snapshot:data.value.snapshot};try{sessionStorage.setItem(storageKey.value,JSON.stringify(intent));pending.value=intent}catch(e){intentError.value=true;report(e);return}}
 const intent=pending.value,token=++generation;busy.value=true;error.value=null
 try{await api.value.place(intent.body,intent.key);if(token!==generation)return;sessionStorage.removeItem(storageKey.value);pending.value=null;quote.value=null;review.value=null;notice.value=true;reviewOpen.value=false;request.value={...request.value,options:[]};await refreshWallet();await load()}
 catch(e){if(token!==generation)return;const status=(e as {status?:number}).status;if(status&&status>=400&&status<500){sessionStorage.removeItem(storageKey.value);pending.value=null;quote.value=null;review.value=null}report(e)}finally{if(token===generation)busy.value=false}
}
</script>
<template>
 <section class="panel configured-live"><p v-if="error&&!reviewOpen" class="message error" role="alert">{{i18n.displayError(error)}} <button v-if="!pending&&!intentError" type="button" class="button button-secondary" :disabled="busy" @click="withGlobalLoading(load)">{{t('retryRead')}}</button></p><p v-if="notice" class="configured-bet-notice" role="status">{{t('placed')}}</p>
 <p v-if="preferencesFailed" class="message error" role="alert">{{t('preferencesReadFailed')}}</p>
 <p v-if="!auth"><RouterLink to="/login">{{t('signIn')}}</RouterLink></p><UserPeriodStrip v-if="data" :previous-no="previousDraw?.period_no" :period-no="displayPeriod?.period_no" :close-at="displayPeriod?.bet_end_at" :draw-at="displayPeriod?.draw_at" :now="now" :clock-offset="0"><span v-for="(number,index) in previousNumbers" :key="index" class="configured-result-number"><span v-if="number.special&&!previousNumbers[index-1]?.special" class="configured-result-plus" aria-hidden="true">+</span><span class="configured-result-ball" :class="[number.wave ? ['mark-six-ball', `lottery-wave-${number.wave}`] : 'configured-plain-ball', { special: number.special }]">{{number.wave ? String(number.value).padStart(2,'0') : number.value}}</span></span></UserPeriodStrip><p v-if="data&&!canBet">{{t('closed')}}</p>
 <PlayRules v-if="data&&currentPlay" :locale="locale" :name="currentPlay.label[localeCode]" :snapshot="data.snapshot" :play="currentPlay" :plate="request.plate" />
 <ConfiguredSelection v-if="data" v-model="request" :snapshot="data.snapshot" :quick-amounts="quickAmounts" :remembered-points="preferences.lastPoints" :locale="localeCode" :disabled="busy||!!pending||intentError||!canBet" :available-points="wallet ? wallet.available_points : null" :wallet-failed="walletFailed" @refresh-wallet="withGlobalLoading(refreshWallet)" v-slot="{ insufficientPoints }" sale><button type="button" class="button button-primary" :disabled="busy||intentError||(!pending&&(!canBet||insufficientPoints))" @click="withGlobalLoading(preview)">{{t(pending?'retryWrite':'previewBet')}}</button></ConfiguredSelection>
 <BettingSheet v-if="reviewOpen" :title="t('previewBet')" :close-label="t('closeBet')" @close="reviewOpen=false">
  <p v-if="error" class="message error" role="alert">{{i18n.displayError(error)}}</p>
  <p v-if="selectionInvalid" class="message error" role="alert">{{t('selectionRequired',{min:currentPlay!.selection.min})}}</p>
  <p v-if="pointsInvalid" class="message error" role="alert">{{t('preferencesInvalid')}}</p>
  <p v-if="busy&&!quote&&!pending" v-global-loading role="status">{{t('loadingQuote')}}</p>
  <template v-if="pending">
   <p>{{t('unknownWrite')}}</p><p>{{t('bet')}}: {{pending.quote.bet_points}}</p><p v-if="pending.quote.reserved_points">{{t('reserved')}}: {{pending.quote.reserved_points}}</p>
   <p v-for="(line,index) in pending.quote.lines" :key="index">{{configuredTicketLabel(pending.snapshot,line,localeCode)}} · {{line.stake}}</p>
  </template>
  <template v-else-if="quote&&data">
<p v-if="!wallet" class="message error" role="alert">{{t('walletUnavailable')}}</p>
   <p>{{t('entries')}}: {{quote.lines.length}} · {{t('bet')}}: {{quote.bet_points}}</p><p v-if="quote.reserved_points">{{t('reserved')}}: {{quote.reserved_points}}</p>
   <p v-for="(line,index) in quote.lines" :key="index">{{configuredTicketLabel(data.snapshot,line,localeCode)}} · {{line.stake}} · {{configuredTicketPrices(data.snapshot,line,localeCode).join(' / ')}}</p>
  </template>
  <p v-else-if="!busy&&!selectionInvalid&&!error">{{t('closed')}}</p>
  <template #footer><button type="button" class="button button-secondary" @click="reviewOpen=false">{{t('closeBet')}}</button><button v-if="pending||(quote&&data)" v-write-guard="busy" type="button" class="button button-primary" :disabled="busy||!auth||intentError||(!pending&&(!canBet||!wallet||quoteInsufficient))" @click="withGlobalLoading(place)">{{t(pending?'retryWrite':'confirmBet')}}</button></template>
 </BettingSheet>

 </section>
</template>
<style scoped>
.configured-live{padding:16px;min-width:0;font-size:14px}.configured-bet-notice{position:fixed;top:64px;right:12px;z-index:40;padding:12px;background:var(--success-bg);color:var(--success-text);border:1px solid var(--line);border-radius:var(--radius-sm);box-shadow:var(--shadow-md)}.configured-live p{overflow-wrap:anywhere}.configured-live > .panel{padding:12px;margin:12px 0;min-width:0}.configured-plain-ball{display:inline-grid;place-items:center;flex-shrink:0;min-width:24px;height:24px;padding:0 4px;border-radius:50%;background:var(--primary-light);color:var(--primary-dark);font-size:12px;font-weight:700}.configured-result-number{display:inline-flex;align-items:center;gap:4px}.configured-result-plus{color:var(--ink-secondary);font-size:16px;font-weight:700}.configured-plain-ball.special{border:2px solid var(--primary)}
</style>
