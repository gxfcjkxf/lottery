<script setup lang="ts">
import { onUnmounted,ref,watch } from 'vue'
import { formatPoints,withGlobalLoading } from '@lottery/shared'
import { readMemberSummary,type MemberBettingSummary } from './member-summary-api'
import { WALLET_SNAPSHOT_EVENT } from './wallet-api'
import { useUserI18n } from './i18n'
const props=defineProps<{brandCode?:string;brandId:string;memberId:string}>()
const {t,locale}=useUserI18n()
const summary=ref<MemberBettingSummary|null>(null),failed=ref(false)
let generation=0,disposed=false
let controller:AbortController|undefined
async function load(){if(disposed)return;const g=++generation;controller?.abort();controller=new AbortController();try{const result=await readMemberSummary(props.brandCode,controller.signal);if(disposed||g!==generation)return;if(result.brand_id!==props.brandId||result.member_id!==props.memberId)throw new Error('Summary identity mismatch');summary.value=result;failed.value=false}catch{if(!disposed&&g===generation)failed.value=true}}
watch(()=>[props.brandId,props.memberId],()=>{summary.value=null;failed.value=false;void withGlobalLoading(load)},{immediate:true})
const background=()=>void load()
const poll=setInterval(background,30000)
window.addEventListener(WALLET_SNAPSHOT_EVENT,background)
onUnmounted(()=>{disposed=true;generation++;controller?.abort();clearInterval(poll);window.removeEventListener(WALLET_SNAPSHOT_EVENT,background)})
</script>
<template><div class="user-overview-grid"><div><span>{{ t('workspace.points') }}</span><slot /></div><div><span>{{ t('workspace.unsettled') }}</span><strong>{{ summary?formatPoints(summary.unsettled_points,locale):'—' }}</strong></div><div><span>{{ t('workspace.today') }}</span><strong :class="{'net-negative':summary&&BigInt(summary.today_net_points)<0n}">{{ summary?formatPoints(summary.today_net_points,locale):'—' }}</strong></div></div><p v-if="failed" class="overview-error" role="alert">{{ t('workspace.failed') }}</p></template>
<style scoped>
.user-overview-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));padding:10px 8px;background:var(--surface);gap:8px}
.user-overview-grid :deep(.header-points){height:auto;max-width:100%}.user-overview-grid :deep(.header-points>strong){overflow:visible;white-space:normal;overflow-wrap:anywhere}
.user-overview-grid>div{display:flex;align-items:center;justify-content:center;flex-flow:row wrap;gap:6px;min-width:0;border-right:1px solid var(--line);overflow-wrap:anywhere}.user-overview-grid>div:last-child{border:0}.user-overview-grid span{font-size:12px;color:var(--muted)}.user-overview-grid strong{font-size:16px;color:var(--primary);font-variant-numeric:tabular-nums}.user-overview-grid .net-negative{color:var(--danger)}.overview-error{font-size:12px;text-align:center;color:var(--danger);margin:0;padding:8px}
</style>
