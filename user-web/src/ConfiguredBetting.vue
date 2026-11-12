<script setup lang="ts">
import { pointsMinor } from '@lottery/shared'

import { useOperationToast } from "@lottery/shared";
import { vGlobalLoading, vWriteGuard } from "@lottery/shared";
import { computed,onMounted,onUnmounted,ref,shallowRef,watch } from 'vue'
import { RouterLink } from 'vue-router'
import {createPlatformLotteryClient,type PlatformPeriod} from '../../shared/src/platform-lottery'
import {createQuickAmountsApi,validBetAmount} from './betting-preferences'
import BettingSheet from './BettingSheet.vue'
import ConfiguredBetReviewTable from './ConfiguredBetReviewTable.vue'
import PlayRules from './PlayRules.vue'
import {configuredSelectionGroups,configuredSelectionRequests} from '../../shared/src/configured-selection-groups'
import UserPeriodStrip from './UserPeriodStrip.vue'
import {markSixCatalog} from '../../shared/src/mark-six'
import {useUserI18n} from './i18n'
import {WALLET_SNAPSHOT_EVENT,type Wallet,createWalletApi} from './wallet-api'
import ConfiguredSelection from '@lottery/shared/ConfiguredSelection.vue'
import {createConfiguredBusinessClient,createCatalogI18n,configuredRuleCatalog,withGlobalLoading,type ConfiguredLiveCatalog,type ConfiguredLiveInput,type ConfiguredLiveQuote,type ConfiguredSnapshot,type ConfiguredRequest,type LocalizedMessage} from '@lottery/shared'
import type {AuthProfile} from '../../shared/src/auth'
const props=defineProps<{gameId:string;brandCode?:string;locale:'en'|'zh';auth?:AuthProfile|null}>()
const emit=defineEmits<{'auth-expired':[];'game-title':[title:string]}>()
const localeCode=computed(()=>props.locale==='en'?'en':'zh-CN');const i18n=createCatalogI18n(configuredRuleCatalog,localeCode.value);watch(localeCode,v=>i18n.setLocale(v));const t=i18n.t
const api=computed(()=>createConfiguredBusinessClient('user','',props.brandCode)),data=shallowRef<ConfiguredLiveCatalog|null>(null),quote=shallowRef<ConfiguredLiveQuote|null>(null),review=shallowRef<ConfiguredLiveInput|null>(null)
const wallet=shallowRef<Wallet|null>(null),walletFailed=ref(false)
let walletGeneration=0
const periods=shallowRef<PlatformPeriod[]>([])
const previousDraw=computed(()=>data.value?.previous_draw)
// Closed periods leave the sales catalog but remain visible until their draw.
const displayPeriod=computed(()=>data.value?.period??periods.value.find(period=>!period.result&&Date.parse(period.bet_start_at)<=now.value))
const numberI18n=createCatalogI18n(markSixCatalog,localeCode.value)
watch(localeCode,value=>numberI18n.setLocale(value))
const {t:userText}=useUserI18n()
const attributeText=(value:string|undefined)=>value?numberI18n.t(value):'—'
const previousIsMarkSix=computed(()=>{const draw=previousDraw.value;return !!draw&&draw.result.regular.length===6&&draw.result.special.length===1})
const previousNumbers=computed(()=>{
 const draw=previousDraw.value
 if(!draw)return []
 const regular=draw.regular_order===null?draw.result.regular:draw.regular_order
 return [...regular.map(value=>({value,special:false})),...draw.result.special.map(value=>({value,special:true})),...draw.result.digits.map(value=>({value,special:false}))].map(number=>({...number,...draw.attributes[String(number.value).padStart(2,'0')]}))
})
const selectedPlays=ref<string[]>([])
const betRequests=computed(()=>data.value?configuredSelectionRequests(data.value.snapshot,request.value,selectedPlays.value):[])
const request=ref<ConfiguredRequest>({play_id:'',options:[],plate:'',points:'',multiplier:'1'}),error=ref<LocalizedMessage|null>(null),notice=ref(false),busy=ref(false)
type Intent={owner:string;key:string;body:ConfiguredLiveInput;quote:ConfiguredLiveQuote;snapshot:ConfiguredSnapshot}
const pending=shallowRef<Intent|null>(null),intentError=ref(false),reviewOpen=ref(false),selectionInvalid=ref(false),pointsInvalid=ref(false)
const owner=computed(()=>`${props.brandCode??''}:${props.auth?.user.id??''}:${props.auth?.member.id??''}`)
const storageKey=computed(()=>`lottery.configured.intent:${owner.value}:${props.gameId}`)
const quickAmounts=ref<string[]>([])
watch(owner,()=>{selectedPlays.value=[];request.value={play_id:'',options:[],plate:'',points:'',multiplier:'1'}},{immediate:true})
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
const quoteInsufficient=computed(()=>{if(!quote.value||!wallet.value||!data.value)return false;const needed=data.value.snapshot.catalog.funding==='reserve'?quote.value.reserved_points!:quote.value.bet_points;return pointsMinor(needed)>pointsMinor(wallet.value.available_points)})
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
const currentGroup=computed(()=>data.value?configuredSelectionGroups(data.value.snapshot).find(group=>group.plays.some(play=>play.id===request.value.play_id)):undefined)
const reviewQuote=computed(()=>pending.value ? pending.value.quote : quote.value)
const unavailableState=computed(()=>{
 const catalog=data.value
 if(!catalog)return ''
 if(!props.auth)return 'signIn'
 if(catalog.game.status!=='active')return 'gameDisabled'
 if(!catalog.game.brand_enabled)return 'brandGameDisabled'
 if(!catalog.sales.enabled)return 'bettingPaused'
 const period=displayPeriod.value
 if(period&&Date.parse(period.bet_start_at)>now.value)return 'notOpen'
 if(period&&Date.parse(period.bet_end_at)<=now.value)return 'closed'
 if(!catalog.period)return 'noBettingPeriod'
 return ''
})
const canBet=computed(()=>!!data.value&&!unavailableState.value)
const unavailableText=computed(()=>unavailableState.value==='notOpen'?userText('workspace.notOpen'):unavailableState.value?t(unavailableState.value):'')
const clearUnavailableToast=useOperationToast(unavailableState, () => unavailableText.value, 'error', 'center', 0)
onUnmounted(clearUnavailableToast)
function report(cause:unknown,read=false){error.value=i18n.errorMessage(cause,read?'read':'write');if((cause as {status?:number}).status===401)emit('auth-expired')}
async function load(background=false){
 const token=++generation;clearTimeout(syncTimer);if(!background)busy.value=true;error.value=null;now.value=Date.now()
 try{
  try{const [value,detail,amounts]=await Promise.all([api.value.catalog(props.gameId),createPlatformLotteryClient('user',()=>'',fetch,props.brandCode).detail(props.gameId),createQuickAmountsApi({brandCode:props.brandCode}).read(props.auth!.member.brand_id,props.auth!.member.id)]);if(token!==generation)return;quickAmounts.value=amounts;periods.value=detail.periods;if(data.value&&(data.value.snapshot_hash!==value.snapshot_hash||data.value.period?.id!==value.period?.id)){quote.value=null;review.value=null}data.value=value;emit('game-title',value.game.name);if(!request.value.play_id||!value.snapshot.catalog.plays.some(p=>p.id===request.value.play_id)){const p=value.snapshot.catalog.plays[0]!;request.value={play_id:p.id,options:[],plate:value.snapshot.catalog.plates[0]!,points:p.stake_mode==='unit_multiplier'?'0':'',multiplier:'1'}}}catch(e){if(token===generation){data.value=null;periods.value=[];quote.value=null;review.value=null;report(e,true)}}
 }finally{if(token===generation){if(!background)busy.value=false;scheduleSync()}}
}
watch(()=>[props.gameId,owner.value],()=>{generation++;selectedPlays.value=[];reviewOpen.value=false;selectionInvalid.value=false;pending.value=null;quote.value=null;review.value=null;data.value=null;periods.value=[];intentError.value=false;notice.value=false;try{const raw=sessionStorage.getItem(storageKey.value);if(raw!==null){const saved=JSON.parse(raw) as Intent;if(saved.owner!==owner.value||!saved.key||!saved.body.actor_context||!saved.body.requests.length||!saved.snapshot||!saved.quote)throw new Error('Invalid saved intent');pending.value=saved;reviewOpen.value=true}}catch(e){intentError.value=true;report(e)}void load()},{immediate:true})
watch([request,selectedPlays],()=>{if(!pending.value){quote.value=null;review.value=null}},{deep:true})
async function preview(){if(busy.value)return;reviewOpen.value=true;selectionInvalid.value=false;pointsInvalid.value=false;error.value=null;if(pending.value)return;if(!data.value?.period||!canBet.value)return;if(!betRequests.value.length||betRequests.value.some(r=>r.options.length<data.value!.snapshot.catalog.plays.find(p=>p.id===r.play_id)!.selection.min)){selectionInvalid.value=true;return;}if(betRequests.value.some(r=>data.value!.snapshot.catalog.plays.find(p=>p.id===r.play_id)!.stake_mode==='per_line'&&!validBetAmount(r.points))){pointsInvalid.value=true;return;}const token=generation;busy.value=true;error.value=null;notice.value=false;try{const body:ConfiguredLiveInput={period_id:data.value.period.id,rule_id:data.value.rule_id,sales_version:data.value.sales.version,snapshot_hash:data.value.snapshot_hash,actor_context:'',requests:JSON.parse(JSON.stringify(betRequests.value))};const [result]=await Promise.all([api.value.preview(body),refreshWallet()]);if(token!==generation)return;if(result.snapshot_hash!==body.snapshot_hash||result.rule_id!==body.rule_id||result.sales_version!==body.sales_version)throw new Error('Quote version mismatch');review.value={...body,actor_context:result.actor_context};quote.value=result}catch(e){if(token===generation)report(e)}finally{if(token===generation)busy.value=false}}
async function place(){now.value=Date.now();if(busy.value||!props.auth||intentError.value)return;if(!pending.value&&!canBet.value)return;if(!pending.value){if(!review.value||!quote.value||!data.value||!wallet.value||quoteInsufficient.value)return;const intent:Intent={owner:owner.value,key:crypto.randomUUID(),body:JSON.parse(JSON.stringify(review.value)),quote:quote.value,snapshot:data.value.snapshot};try{sessionStorage.setItem(storageKey.value,JSON.stringify(intent));pending.value=intent}catch(e){intentError.value=true;report(e);return}}
 const intent=pending.value,token=++generation;busy.value=true;error.value=null
 try{await api.value.place(intent.body,intent.key);if(token!==generation)return;sessionStorage.removeItem(storageKey.value);pending.value=null;quote.value=null;review.value=null;notice.value=true;reviewOpen.value=false;selectedPlays.value=[];request.value={...request.value,options:[],points:currentPlay.value!.stake_mode==='unit_multiplier'?'0':'',multiplier:'1'};await refreshWallet();await load()}
 catch(e){if(token!==generation)return;const status=(e as {status?:number}).status;if(status&&status>=400&&status<500){sessionStorage.removeItem(storageKey.value);pending.value=null;quote.value=null;review.value=null}report(e)}finally{if(token===generation)busy.value=false}
}
useOperationToast(notice, () => t('placed'), 'success');
useOperationToast(error, value => i18n.displayError(value), 'error');
</script>
<template>
 <section class="panel configured-live"><p v-if="error&&!reviewOpen" class="message error" role="alert"><button v-if="!pending&&!intentError" type="button" class="button button-secondary" :disabled="busy" @click="withGlobalLoading(load)">{{t('retryRead')}}</button></p>
 <p v-if="!auth"><RouterLink to="/login">{{t('signIn')}}</RouterLink></p><UserPeriodStrip v-if="data" :game-id="gameId" :class="{'has-result-attributes':previousIsMarkSix}" :previous-no="previousDraw?.period_no" :period-no="displayPeriod?.period_no" :open-at="displayPeriod?.bet_start_at" :close-at="displayPeriod?.bet_end_at" :draw-at="displayPeriod?.draw_at" :now="now" :clock-offset="0">
  <template v-for="(number,index) in previousNumbers" :key="index">
   <span v-if="number.special&&!previousNumbers[index-1]?.special" class="configured-result-plus" aria-hidden="true">+</span>
   <span class="configured-result-number">
    <span class="configured-result-ball" :class="[number.wave ? ['mark-six-ball', `lottery-wave-${number.wave}`] : 'configured-plain-ball', { special: number.special }]">{{previousIsMarkSix ? String(number.value).padStart(2,'0') : number.value}}</span>
    <span v-if="previousIsMarkSix" class="configured-result-attributes" :title="number.zodiac&&number.element?`${attributeText(number.zodiac)} / ${attributeText(number.element)}`:userText('draw.attributesMissing')"><span class="configured-result-zodiac" :class="{'is-missing':!number.zodiac}">{{attributeText(number.zodiac)}}</span><span class="configured-attribute-divider" aria-hidden="true">/</span><span class="configured-result-element" :class="number.element?`configured-element-${number.element}`:'is-missing'">{{attributeText(number.element)}}</span></span>
   </span>
  </template>
  <span v-if="previousDraw?.result.digits.length===3" class="configured-result-sum" :aria-label="`${userText('draw.summary.total')}: ${previousDraw.result.digits.reduce((sum,n)=>sum+n,0)}`"><span aria-hidden="true">=</span><strong>{{previousDraw.result.digits.reduce((sum,n)=>sum+n,0)}}</strong></span>
 </UserPeriodStrip>
 <PlayRules v-if="data&&currentPlay" :locale="locale" :name="currentGroup!.label[localeCode]" :group="currentGroup" :snapshot="data.snapshot" :play="currentPlay" :plate="request.plate" />
 <ConfiguredSelection v-if="data" v-model="request" v-model:selected-plays="selectedPlays" :snapshot="data.snapshot" :quick-amounts="quickAmounts" :locale="localeCode" :disabled="busy||!!pending||intentError||!canBet" :available-points="wallet ? wallet.available_points : null" :wallet-failed="walletFailed" @refresh-wallet="withGlobalLoading(refreshWallet)" v-slot="{ insufficientPoints }" sale><button type="button" class="button button-primary" :disabled="busy||intentError||(!pending&&(!canBet||insufficientPoints))" @click="withGlobalLoading(preview)">{{t(pending?'retryWrite':'previewBet')}}</button></ConfiguredSelection>
 <BettingSheet v-if="reviewOpen" :title="t('previewBet')" :close-label="t('closeBet')" @close="reviewOpen=false">

  <p v-if="selectionInvalid" class="message error" role="alert">{{t('selectionRequired',{min:currentPlay!.selection.min})}}</p>
  <p v-if="pointsInvalid" class="message error" role="alert">{{t('preferencesInvalid')}}</p>
  <p v-if="busy&&!quote&&!pending" v-global-loading role="status">{{t('loadingQuote')}}</p>
  <template v-if="pending">
   <p>{{t('unknownWrite')}}</p>
   <ConfiguredBetReviewTable :snapshot="pending.snapshot" :lines="pending.quote.lines" :locale="localeCode" />
  </template>
  <template v-else-if="quote&&data">
<p v-if="!wallet" class="message error" role="alert">{{t('walletUnavailable')}}</p>
   <ConfiguredBetReviewTable :snapshot="data.snapshot" :lines="quote.lines" :locale="localeCode" />
  </template>
  <p v-else-if="!busy&&!selectionInvalid&&!error&&unavailableText">{{unavailableText}}</p>
  <template #footer><div class="configured-review-footer"><div v-if="reviewQuote" class="configured-review-totals"><span>{{t('entries')}}: <strong>{{reviewQuote.lines.length}}</strong></span><span>{{t('bet')}}: <strong>{{reviewQuote.bet_points}}</strong></span><span v-if="reviewQuote.reserved_points">{{t('reserved')}}: <strong>{{reviewQuote.reserved_points}}</strong></span></div><div class="configured-review-actions"><button v-if="pending||(quote&&data)" v-write-guard="busy" type="button" class="button button-primary" :disabled="busy||!auth||intentError||(!pending&&(!canBet||!wallet||quoteInsufficient))" @click="withGlobalLoading(place)">{{t(pending?'retryWrite':'confirmBet')}}</button></div></div></template>
 </BettingSheet>

 </section>
</template>
<style scoped>
.configured-result-sum{display:inline-flex;align-items:center;gap:6px;white-space:nowrap;color:var(--ink);font-variant-numeric:tabular-nums}.configured-result-sum>span{color:var(--muted)}
.configured-review-footer{display:flex;flex:1 1 100%;flex-wrap:wrap;align-items:center;gap:8px 16px;min-width:0;width:100%}
.configured-review-totals{display:flex;flex:1 1 200px;flex-wrap:wrap;align-items:baseline;gap:4px 12px;min-width:0;font-size:var(--body-font-size);font-variant-numeric:tabular-nums}
.configured-review-totals>span{white-space:nowrap}
.configured-review-actions{display:flex;align-items:center;gap:8px;max-width:100%;margin-left:auto}

.configured-live{padding:0;min-width:0;width:100%;box-sizing:border-box;font-size:14px}.configured-live p{overflow-wrap:anywhere}.configured-live > .panel{padding:12px;margin:12px 0;min-width:0}.configured-plain-ball{display:inline-grid;place-items:center;flex-shrink:0;min-width:24px;height:24px;padding:0 4px;border-radius:50%;background:var(--primary-light);border:1px solid var(--primary-ring);color:var(--primary-dark);font-size:var(--body-font-size);font-weight:700}.configured-result-number{display:inline-flex;flex-direction:column;align-items:center;gap:3px;min-width:0}.configured-result-plus{display:inline-grid;place-items:center;min-height:clamp(24px,6.8vw,28px);color:var(--ink-secondary);font-size:16px;font-weight:700}.configured-plain-ball.special{border:2px solid var(--accent);color:var(--accent)}
.configured-result-attributes{display:flex;flex-wrap:wrap;align-items:center;justify-content:center;gap:1px;max-width:100%;font-size:var(--body-font-size);line-height:1.4}.configured-result-attributes>span{max-width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.configured-result-zodiac{color:var(--lottery-attribute-zodiac)}.configured-element-metal{color:var(--lottery-element-metal)}.configured-element-wood{color:var(--lottery-element-wood)}.configured-element-water{color:var(--lottery-element-water)}.configured-element-fire{color:var(--lottery-element-fire)}.configured-element-earth{color:var(--lottery-element-earth)}.configured-attribute-divider,.configured-result-attributes .is-missing{color:var(--muted)}
@media(max-width:620px){.configured-result-attributes{flex-direction:column;gap:0}.configured-attribute-divider{display:none}}
</style>
