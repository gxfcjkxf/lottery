<script setup lang="ts">
import { computed } from 'vue'
import { formatDisplayDateTime } from '@lottery/shared'
import { useUserI18n } from './i18n'
const props=defineProps<{previousNo?:string; periodNo?:string; closeAt?:string; drawAt?:string; now:number; clockOffset:number}>()
const {t,locale}=useUserI18n()
const sealed=computed(()=>!!props.closeAt && props.now+props.clockOffset>=Date.parse(props.closeAt))
const countdown=computed(()=>{
 const end=sealed.value?props.drawAt:props.closeAt
 if(!end)return '—'
 const n=Math.max(0,Math.ceil((Date.parse(end)-props.now-props.clockOffset)/1000))
 return [Math.floor(n/3600),Math.floor(n/60)%60,n%60].map(v=>String(v).padStart(2,'0')).join(':')
})
// The date is exposed in the accessible label; the compact row uses the same
// platform-configured clock, including its 12/24-hour and language settings.
function time(value?:string){return value?formatDisplayDateTime(value,locale.value).split(' ').slice(1).join(' '):'—'}
</script>
<template>
 <section class="user-period-strip">
  <div class="result-row"><strong>{{ previousNo?t('workspace.period',{number:previousNo}):'—' }}</strong><div class="result-numbers"><slot /></div></div>
  <div class="current-row"><strong class="period-number">{{ periodNo?t('workspace.period',{number:periodNo}):'—' }}</strong><span class="period-times">{{ t('workspace.closeAt') }} <time :datetime="closeAt" :aria-label="formatDisplayDateTime(closeAt,locale)">{{ time(closeAt) }}</time> · {{ t('workspace.drawAt') }} <time :datetime="drawAt" :aria-label="formatDisplayDateTime(drawAt,locale)">{{ time(drawAt) }}</time></span><strong class="period-countdown" :aria-label="t(sealed?'workspace.drawCountdown':'workspace.closeCountdown')">{{ countdown }}</strong></div>
 </section>
</template>
<style scoped>
.user-period-strip{background:var(--surface);border:1px solid var(--line);border-radius:var(--radius-sm);margin-bottom:8px;min-width:0;padding:0 10px;font-size:13px}
.result-row,.current-row{display:flex;align-items:center;justify-content:space-between;gap:8px;min-width:0;padding:8px 0}
.result-row{display:grid;grid-template-columns:auto minmax(0,1fr);border-bottom:1px solid var(--line)}.result-row>strong{overflow-wrap:anywhere;color:var(--primary-dark)}
.result-numbers{display:flex;justify-content:flex-end;gap:4px;align-items:center;flex-wrap:wrap;min-width:0}.result-numbers :deep(.mark-six-ball){width:clamp(24px,6.8vw,28px);height:clamp(24px,6.8vw,28px);font-size:12px}
.period-number{flex-shrink:0}.period-times{min-width:0;font-size:11px;color:var(--muted);text-align:center;overflow-wrap:anywhere}.period-times time{display:inline-block}.period-countdown{flex-shrink:0;color:var(--danger);font-size:18px;font-variant-numeric:tabular-nums}
</style>
