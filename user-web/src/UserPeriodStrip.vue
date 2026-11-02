<script setup lang="ts">
import { computed } from 'vue'
import { formatDisplayDateTime, platformDisplaySettings } from '@lottery/shared'
import { useUserI18n } from './i18n'
const props=defineProps<{previousNo?:string; periodNo?:string; openAt?:string; closeAt?:string; drawAt?:string; now:number; clockOffset:number}>()
const {t,locale}=useUserI18n()
const waiting=computed(()=>!!props.openAt && props.now+props.clockOffset<Date.parse(props.openAt))
const sealed=computed(()=>!!props.closeAt && props.now+props.clockOffset>=Date.parse(props.closeAt))
const countdown=computed(()=>{
 const end=waiting.value?props.openAt:sealed.value?props.drawAt:props.closeAt
 if(!end)return '—'
 const n=Math.max(0,Math.ceil((Date.parse(end)-props.now-props.clockOffset)/1000))
 return [Math.floor(n/3600),Math.floor(n/60)%60,n%60].map(v=>String(v).padStart(2,'0')).join(':')
})
function mobileTime(value?:string){
 if(!value)return '—'
 if(!platformDisplaySettings.value)return formatDisplayDateTime(value,locale.value)
 const parts=new Intl.DateTimeFormat('en-US',{timeZone:platformDisplaySettings.value.timezone,month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',hourCycle:'h23'}).formatToParts(new Date(value))
 const fields=Object.fromEntries(parts.map(part=>[part.type,part.value]))
 return `${fields.month}-${fields.day} ${fields.hour}:${fields.minute}`
}
function time(value?:string){return value?formatDisplayDateTime(value,locale.value):'—'}
</script>
<template>
 <section class="user-period-strip">
  <div class="result-row"><strong>{{ previousNo?t('workspace.period',{number:previousNo}):'—' }}</strong><div class="result-numbers"><slot /></div></div>
  <div class="current-row"><strong class="period-number">{{ periodNo?t('workspace.period',{number:periodNo}):'—' }}</strong><span class="period-times"><span>{{ t('workspace.openAt') }} <time :datetime="openAt" :aria-label="formatDisplayDateTime(openAt,locale)"><span class="period-time-full">{{ time(openAt) }}</span><span class="period-time-mobile">{{ mobileTime(openAt) }}</span></time></span><span>{{ t('workspace.closeAt') }} <time :datetime="closeAt" :aria-label="formatDisplayDateTime(closeAt,locale)"><span class="period-time-full">{{ time(closeAt) }}</span><span class="period-time-mobile">{{ mobileTime(closeAt) }}</span></time></span><span>{{ t('workspace.drawAt') }} <time :datetime="drawAt" :aria-label="formatDisplayDateTime(drawAt,locale)"><span class="period-time-full">{{ time(drawAt) }}</span><span class="period-time-mobile">{{ mobileTime(drawAt) }}</span></time></span></span><strong class="period-countdown" :aria-label="t(waiting?'workspace.openCountdown':sealed?'workspace.drawCountdown':'workspace.closeCountdown')">{{ countdown }}</strong></div>
 </section>
</template>
<style scoped>
.user-period-strip{background:var(--surface);border:1px solid var(--line);border-radius:var(--radius-sm);margin-bottom:8px;min-width:0;padding:0 10px;font-size:var(--body-font-size)}
.result-row,.current-row{display:flex;align-items:center;justify-content:flex-start;gap:8px;min-width:0;padding:8px 0}
.result-row{display:grid;grid-template-columns:auto minmax(0,1fr);border-bottom:1px solid var(--line)}.result-row>strong{overflow-wrap:anywhere;color:var(--primary-dark)}
.result-numbers{display:flex;justify-content:flex-start;gap:4px;align-items:center;flex-wrap:wrap;min-width:0}.result-numbers :deep(.mark-six-ball){width:clamp(24px,6.8vw,28px);height:clamp(24px,6.8vw,28px);font-size:var(--body-font-size)}
.period-number{flex-shrink:0}.period-times{display:flex;flex-wrap:wrap;justify-content:flex-start;gap:2px 10px;min-width:0;font-size:var(--body-font-size);color:var(--muted);text-align:left;overflow-wrap:anywhere}.period-times>span{white-space:nowrap}.period-times time{display:inline-block}.period-countdown{flex-shrink:0;margin-left:auto;color:var(--danger);font-size:18px;font-variant-numeric:tabular-nums}
.has-result-attributes .result-numbers{display:grid;grid-template-columns:repeat(6,minmax(0,1fr)) 12px minmax(0,1fr);align-items:start;column-gap:4px;width:min(100%,420px);justify-self:start}
.period-time-mobile{display:none}
@media(max-width:999px){.period-time-full{display:none}.period-time-mobile{display:inline}}
@media(max-width:620px){.current-row{display:grid;grid-template-columns:minmax(0,1fr) auto}.period-times{grid-column:1/-1;grid-row:2}.period-countdown{grid-column:2;grid-row:1}.has-result-attributes .result-row{grid-template-columns:minmax(0,1fr);gap:6px}.has-result-attributes .result-numbers{width:100%;grid-template-columns:repeat(6,minmax(0,1fr)) 10px minmax(0,1fr);column-gap:3px;justify-self:stretch}}
</style>
