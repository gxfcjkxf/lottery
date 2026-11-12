<script setup lang="ts">
import { pointsMinor } from '@lottery/shared'
import SiteIcon from '@lottery/shared/SiteIcon'

import { onMounted,onUnmounted,ref,watch } from 'vue'
import { formatPoints,withGlobalLoading } from '@lottery/shared'
import { readMemberSummary,type MemberBettingSummary } from './member-summary-api'
import { WALLET_SNAPSHOT_EVENT, type Wallet } from './wallet-api'
import { useUserI18n } from './i18n'
const props=defineProps<{brandCode?:string;brandId:string;memberId:string}>()
const {t,locale}=useUserI18n()
const summary=ref<MemberBettingSummary|null>(null),failed=ref(false)
let generation=0,disposed=false
let controller:AbortController|undefined
async function load(){if(disposed)return;const g=++generation;controller?.abort();controller=new AbortController();try{const result=await readMemberSummary(props.brandCode,controller.signal);if(disposed||g!==generation)return;if(result.brand_id!==props.brandId||result.member_id!==props.memberId)throw new Error('Summary identity mismatch');summary.value=result;failed.value=false}catch{if(!disposed&&g===generation)failed.value=true}}
watch(()=>[props.brandId,props.memberId],()=>{summary.value=null;failed.value=false;void withGlobalLoading(load)},{immediate:true})
function background(event:Event){const value=(event as CustomEvent<Wallet>).detail;if(value.brand_id===props.brandId&&value.member_id===props.memberId)void load()}
onMounted(()=>window.addEventListener(WALLET_SNAPSHOT_EVENT,background))
onUnmounted(()=>{disposed=true;generation++;controller?.abort();window.removeEventListener(WALLET_SNAPSHOT_EVENT,background)})
</script>
<template><div class="user-overview-grid"><div><span><SiteIcon name="wallet" />{{ t('workspace.points') }}</span><slot /></div><div><span><SiteIcon name="ticket" />{{ t('workspace.unsettled') }}</span><strong>{{ summary?formatPoints(summary.unsettled_points,locale):'—' }}</strong></div><div><span><SiteIcon name="chart" />{{ t('workspace.today') }}</span><strong :class="{'net-negative':summary&&pointsMinor(summary.today_net_points)<0n}">{{ summary?formatPoints(summary.today_net_points,locale):'—' }}</strong></div></div><p v-if="failed" class="overview-error" role="alert">{{ t('workspace.failed') }}</p></template>
<style scoped>
.user-overview-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));padding:10px 8px;background:var(--surface);border-bottom:1px solid var(--line);backdrop-filter:blur(12px);-webkit-backdrop-filter:blur(12px);gap:8px}
.user-overview-grid span{display:inline-flex;align-items:center;justify-content:center;gap:5px;--site-icon-size:16px}
.user-overview-grid :deep(.header-points){height:auto;max-width:100%}.user-overview-grid :deep(.header-points>strong){overflow:visible;white-space:normal;overflow-wrap:anywhere}
.user-overview-grid>div{display:flex;align-items:center;justify-content:center;flex-flow:row wrap;gap:6px;min-width:0;border-right:1px solid var(--line);overflow-wrap:anywhere}.user-overview-grid>div:last-child{border:0}.user-overview-grid span{font-size:var(--body-font-size);color:var(--muted)}.user-overview-grid strong{font-size:16px;color:var(--primary-dark);font-variant-numeric:tabular-nums}.user-overview-grid .net-negative{color:var(--danger-text)}.overview-error{font-size:var(--body-font-size);text-align:center;color:var(--danger-text);margin:0;padding:8px}
</style>
