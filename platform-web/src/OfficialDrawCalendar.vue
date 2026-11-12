<script setup lang="ts">
import { useOperationToast } from "@lottery/shared";
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { withGlobalLoading, vWriteGuard, vRefreshButton, vGlobalLoading } from '@lottery/shared'
import { createPlatformLotteryClient, PlatformLotteryError, type OfficialDrawCalendar } from '../../shared/src/platform-lottery'
import { formatPlatformDateTime, type PlatformDisplaySettings } from './platform-format'
import { usePlatformI18n } from './i18n'
import {calendarWeeks,shiftCalendarMonth} from './official-calendar'

const props = defineProps<{ locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings; canManage: boolean }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const { t } = usePlatformI18n()
const data = ref<OfficialDrawCalendar | null>(null), busy = ref(false), failed = ref(false)
const syncing = ref(false), pendingKey = ref<string | null>(null)
const syncNotice = ref<'' | 'syncSuccess' | 'syncFailed' | 'syncBusy' | 'syncUnknown' | 'syncDenied' | 'syncRejected'>('')
let alive = true
const api = createPlatformLotteryClient('platform', () => '')
const selectedMonth=ref('')
const monthData=computed(()=>data.value?.months.find(month=>month.month===selectedMonth.value))
const months=computed(()=>[...new Set([data.value!.current_month,...data.value!.months.map(month=>month.month)])].sort())
const weeks=computed(()=>selectedMonth.value?calendarWeeks(selectedMonth.value):[])
const weekdays=computed(()=>Array.from({length:7},(_,i)=>new Intl.DateTimeFormat(props.locale,{weekday:props.locale==='zh-CN'?'narrow':'short',timeZone:'UTC'}).format(new Date(Date.UTC(2026,9,25+i)))))
const monthLabel = (month: string) => new Intl.DateTimeFormat(props.locale, { year: 'numeric', month: 'long', timeZone: 'UTC' }).format(new Date(`${month}-01T00:00:00Z`))
function marker(date:string,kind:'draw'|'snowball'|'presale'){
 const month=monthData.value
 return !!month && date.startsWith(selectedMonth.value+'-') && (kind==='draw'?month.draw_dates:kind==='snowball'?month.snowball_dates:month.presale_dates).includes(date)
}
function dayLabel(date:string){return [dateLabel(date),...(['draw','snowball','presale'] as const).filter(kind=>marker(date,kind)).map(kind=>t('calendar.'+kind))].join(' · ')}
function changeMonth(offset:number){selectedMonth.value=shiftCalendarMonth(selectedMonth.value,offset)}
function dateLabel(date: string) {
 const [year, month, day] = date.split('-')
 return props.displaySettings.date_format.replace('YYYY', year!).replace('MM', month!).replace('DD', day!)
}
const timeLabel = (value: string | null, empty: 'calendar.never' | 'calendar.unscheduled') => value ? formatPlatformDateTime(value, props.displaySettings, props.locale) : t(empty)
async function load() {
 if (busy.value) return
 busy.value = true; failed.value = false
 try {
  const result = await api.officialCalendar()
  if (alive) {data.value = result;if(!selectedMonth.value)selectedMonth.value=result.current_month}
 } catch (cause) {
  if (!alive) return
  data.value = null; failed.value = true
  if (cause instanceof PlatformLotteryError && cause.status === 401) emit('failure', cause)
 } finally { if (alive) busy.value = false }
}
async function syncNow() {
 if (!props.canManage || busy.value || syncing.value) return
 pendingKey.value ??= crypto.randomUUID()
 syncing.value = true; syncNotice.value = ''
 try {
  const result = await api.syncOfficialCalendar(pendingKey.value)
  if (!alive) return
  pendingKey.value = null
  syncNotice.value = result.status === 'synced' ? 'syncSuccess' : 'syncFailed'
  await load()
 } catch (cause) {
  if (!alive) return
  if (cause instanceof PlatformLotteryError && cause.status >= 400 && cause.status < 500) {
   pendingKey.value = null
   if (cause.status === 401) { emit('failure', cause); return }
   syncNotice.value = cause.code === 'HKJC_CALENDAR_BUSY' ? 'syncBusy' : cause.status === 403 ? 'syncDenied' : 'syncRejected'
  } else syncNotice.value = 'syncUnknown'
 } finally { if (alive) syncing.value = false }
}
onMounted(() => { void withGlobalLoading(load) })
onUnmounted(() => { alive = false })
useOperationToast(() => syncNotice.value === 'syncSuccess' ? syncNotice.value : '', value => t(`calendar.${value}`), 'success');
useOperationToast(() => syncNotice.value === 'syncFailed' ? syncNotice.value : '', value => t(`calendar.${value}`), 'error');
</script>

<template>
 <section class="panel official-calendar" data-testid="official-calendar" :aria-busy="busy || syncing" aria-labelledby="official-calendar-title">
  <div class="official-calendar-heading console-page-heading">
   <h2 id="official-calendar-title">{{ t('calendar.title') }}</h2>
   <span v-if="data" class="status-pill calendar-status" :class="`calendar-${data.status}`" role="status" :aria-label="t('calendar.status')">{{ t(`calendar.${data.status}`) }}</span>
   <div class="calendar-actions console-page-actions"><button v-if="canManage" v-write-guard="syncing" type="button" class="button button-primary" :disabled="busy || syncing" @click="withGlobalLoading(syncNow)">{{ t(syncing ? 'calendar.syncing' : pendingKey ? 'calendar.syncRetry' : 'calendar.syncNow') }}</button></div>
  </div>
  <p v-if="busy && !data" v-global-loading role="status" class="calendar-note">{{ t('calendar.loading') }}</p>
  <div v-if="failed" role="alert" class="message error">{{ t('calendar.readFailed') }} <button type="button" class="button button-secondary" :disabled="busy || syncing" v-refresh-button="t('calendar.retry')" @click="withGlobalLoading(load)">{{ t('calendar.retry') }}</button></div>
  <template v-if="data">
   <dl class="calendar-meta">
    <div><dt>{{ t('calendar.lastSuccess') }}</dt><dd>{{ timeLabel(data.last_success_at, 'calendar.never') }}</dd></div>
    <div><dt>{{ t('calendar.nextAttempt') }}</dt><dd>{{ timeLabel(data.next_attempt_at, 'calendar.unscheduled') }}</dd></div>
   </dl>
   <p v-if="(data.status === 'failed' && syncNotice !== 'syncFailed') || data.status === 'overdue'" class="message error" role="status">{{ t(data.status === 'failed' ? 'calendar.failedHelp' : 'calendar.overdueHelp') }}</p>
   <section class="calendar-month-view" :aria-label="t('calendar.monthView')">
    <div class="calendar-month-heading"><button type="button" class="calendar-month-arrow" :aria-label="t('calendar.previousMonth')" :disabled="selectedMonth<=months[0]!" @click="changeMonth(-1)"><span aria-hidden="true">‹</span></button><h3 aria-live="polite">{{monthLabel(selectedMonth)}}</h3><button type="button" class="calendar-month-arrow" :aria-label="t('calendar.nextMonth')" :disabled="selectedMonth>=months[months.length-1]!" @click="changeMonth(1)"><span aria-hidden="true">›</span></button></div>
    <p v-if="!monthData" class="calendar-note" role="status">{{t(data.status==='pending'?'calendar.never':'calendar.unpublishedMonth')}}</p>
    <table class="calendar-grid" :aria-label="monthLabel(selectedMonth)"><thead><tr><th v-for="(day,index) in weekdays" :key="index" scope="col">{{day}}</th></tr></thead><tbody><tr v-for="(week,index) in weeks" :key="index"><td v-for="day in week" :key="day.date" :class="{'calendar-outside':!day.inMonth}" :aria-label="dayLabel(day.date)"><time :datetime="day.date" class="calendar-day" :class="{'calendar-draw':marker(day.date,'draw'),'calendar-snowball':marker(day.date,'snowball'),'calendar-presale':marker(day.date,'presale')}">{{day.day}}</time></td></tr></tbody></table>
    <ul class="calendar-legend"><li><span class="calendar-symbol calendar-draw" aria-hidden="true"></span>{{t('calendar.draw')}}</li><li><span class="calendar-symbol calendar-snowball" aria-hidden="true"></span>{{t('calendar.snowball')}}</li><li><span class="calendar-symbol calendar-presale" aria-hidden="true"></span>{{t('calendar.presale')}}</li></ul>
   </section>
  </template>
  
 </section>
</template>

<style scoped>
.official-calendar{display:grid;gap:12px;min-width:0;margin-bottom:12px;padding:12px}
.official-calendar-heading{display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:8px}
.official-calendar h2{margin:0;font-size:16px}
.calendar-meta{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(230px,100%),1fr));gap:8px;margin:0}
.official-calendar .calendar-meta>div{min-width:0;padding:0;border:0}
.calendar-meta dt,.calendar-note,.calendar-weekday{font-size:var(--body-font-size);color:var(--muted)}
.calendar-meta dd{margin:4px 0 0;text-align:left;overflow-wrap:anywhere;font-size:var(--body-font-size);font-variant-numeric:tabular-nums}
.calendar-month-view{width:100%;max-width:420px;min-width:0}
.calendar-month-heading{display:grid;grid-template-columns:44px minmax(0,1fr) 44px;align-items:center;gap:8px;margin-bottom:6px}
.calendar-month-heading h3{font-size:var(--body-font-size);font-weight:650;text-align:center;margin:0}
.calendar-month-arrow{width:44px;height:44px;border:0;border-radius:var(--radius-sm);background:transparent;color:var(--ink);font:inherit;font-size:24px;cursor:pointer}
.calendar-month-arrow:hover:not(:disabled){background:var(--line-subtle)}.calendar-month-arrow:disabled{color:var(--muted);cursor:default}
.calendar-month-view .calendar-grid{--table-cell-padding-inline:0px;table-layout:fixed;width:100%;border-collapse:collapse;min-width:0;margin:0}
.calendar-month-view .calendar-grid th,.calendar-month-view .calendar-grid td{width:calc(100% / 7);height:42px;padding:0;text-align:center;white-space:normal;font-size:var(--body-font-size);border:0;vertical-align:middle}
.calendar-month-view .calendar-grid th{background:var(--table-header-bg);color:var(--ink-secondary);font-weight:650;border-bottom:1px solid var(--line)}
.calendar-month-view .calendar-grid td{color:var(--muted)}
.calendar-day{display:inline-grid;place-items:center;width:32px;height:32px;box-sizing:border-box;border:1px solid transparent;border-radius:50%;font-variant-numeric:tabular-nums}
.calendar-draw{background:var(--info-bg);color:var(--info-text);border-color:var(--info);font-weight:600}
.calendar-snowball{background:var(--warning);color:var(--ink);border-color:var(--warning);font-weight:650}
.calendar-presale{border-color:var(--warning-text);box-shadow:0 0 0 1px var(--warning-text);color:var(--ink)}
.calendar-legend{display:flex;align-items:center;flex-wrap:wrap;gap:8px 14px;margin:8px 0 0;padding:10px 0 0;list-style:none;border-top:1px solid var(--line);font-size:var(--body-font-size)}
.calendar-legend li{display:flex;align-items:center;gap:6px}.calendar-symbol{display:inline-block;width:16px;height:16px;box-sizing:border-box;border-radius:50%;border:1px solid transparent}.calendar-symbol.calendar-draw{border-color:var(--info)}.calendar-symbol.calendar-presale{border-color:var(--warning-text);background:transparent}
.calendar-snowball-label,.calendar-failed,.calendar-overdue{background:var(--warning-bg);color:var(--warning-text)}
.calendar-synced{background:var(--success-bg);color:var(--success-text)}
.calendar-note{margin:0;line-height:1.6}.official-calendar .message{margin:0}
.calendar-actions{display:flex;flex-wrap:wrap;gap:8px}.calendar-sync-success{background:var(--success-bg);color:var(--success-text)}

</style>
