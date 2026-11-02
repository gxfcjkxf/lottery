<script setup lang="ts">
import ManagementList from "./ManagementList.vue"
import TablePagination from './TablePagination.vue'
import SelectControl from './SelectControl.vue'
import { computed, ref, watch, onUnmounted } from 'vue'
import { withGlobalLoading, vGlobalLoading } from './global-loading'
import { vRefreshButton } from './refresh-button'
import { createCatalogI18n } from './localization'
import { drawOverviewCatalog } from './draw-overview-catalog'
import { automationCatalog, type AutomationConfig } from './automation'
import { createPlatformLotteryClient, PlatformLotteryError, type PlatformDrawOverview, type PlatformPeriod } from './platform-lottery'
import { formatDisplayDateTime, platformDisplaySettings } from './display-settings'
const props = defineProps<{ gameId: string; locale: 'en' | 'zh-CN'; showSaleTimes?:boolean; canCreatePeriod?:boolean; canEditOpening?:boolean; canManage: boolean }>()
const emit = defineEmits<{ failure: [cause: unknown]; publish: [period: PlatformPeriod]; create: []; opening: [period: PlatformPeriod] }>()
const i18n = createCatalogI18n(drawOverviewCatalog, props.locale)
const statusI18n = createCatalogI18n(automationCatalog, props.locale)
watch(() => props.locale, value => { i18n.setLocale(value); statusI18n.setLocale(value) })
const t = i18n.t
const api = createPlatformLotteryClient('platform', () => '')
const data = ref<PlatformDrawOverview | null>(null), config = ref<AutomationConfig | null>(null)
const year = ref(''), offset = ref(0), busy = ref(false), failed = ref(false), updated = ref('')
let generation = 0, alive = true
const stale = computed(() => !!config.value?.source_check && new Date(updated.value).getTime() - new Date(config.value.source_check.at).getTime() > 120000)
const time = (value: string | null) => value ? formatDisplayDateTime(value, props.locale) : '—'
const numbers = (result: NonNullable<PlatformPeriod['result']>) => [...result.regular, ...result.special, ...result.digits].map(n => String(n).padStart(2, '0')).join(' · ')
function date(value: string | null) {
 if (!value) return t('unknownDate')
 const [year, month, day] = value.split('-')
 return platformDisplaySettings.value!.date_format.replace('YYYY', year!).replace('MM', month!).replace('DD', day!)
}
async function load(nextYear = year.value, nextOffset = offset.value) {
 const g = ++generation
 busy.value = true
 try {
  const results = await Promise.allSettled([api.drawOverview(props.gameId, nextYear, nextOffset), api.automation(props.gameId)])
  if (!alive || g !== generation) return
  const [overview, automation] = results
  if (overview.status === 'rejected' || automation.status === 'rejected') {
   for (const result of results) if (result.status === 'rejected' && result.reason instanceof PlatformLotteryError && result.reason.status === 401) emit('failure', result.reason)
   failed.value = true
   return
  }
  data.value = overview.value; config.value = automation.value; year.value = overview.value.year; offset.value = nextOffset
  failed.value = false; updated.value = new Date().toISOString()
 } finally { if (alive && g === generation) busy.value = false }
}
watch(() => props.gameId, () => { generation++; data.value = null; config.value = null; year.value = ''; offset.value = 0; failed.value = false; updated.value = ''; void load() }, { immediate: true })
const poll = setInterval(() => { if (!busy.value) void load() }, 5000)
onUnmounted(() => { alive = false; generation++; clearInterval(poll) })
</script>
<template>
<section class="draw-overview" data-testid="draw-overview" :aria-busy="busy">
 <div class="draw-overview-toolbar"><span>{{ t('updated') }}: {{ time(updated) }}</span><button class="button button-secondary" :disabled="busy" v-refresh-button="t('refresh')" @click="withGlobalLoading(load)">{{ t('refresh') }}</button></div>
 <p v-if="failed" class="message error" role="alert">{{ t(data ? 'failure' : 'initialFailure') }}</p>
 <p v-if="busy && !data" v-global-loading role="status">{{ t('loading') }}</p>
 <template v-if="data && config">
 <section class="panel draw-live"><h3>{{ t('sourceStatus') }}</h3><span class="status-chip">{{ t(!config.plan ? 'notConfigured' : config.plan.enabled ? 'enabled' : 'disabled') }}</span>
 <p v-if="stale" class="message error" role="status">{{ t('stale') }}</p>
 <dl class="draw-facts"><div v-if="config.plan"><dt>{{ t('sourceURL') }}</dt><dd>{{ config.plan.source_url }}</dd></div><div><dt>{{ t('checked') }}</dt><dd>{{ time(config.source_check?.at ?? null) }}</dd></div><div><dt>{{ t('sourceStatus') }}</dt><dd>{{ config.source_check ? statusI18n.t(config.source_check.code) : t('noCheck') }}</dd></div><div v-if="config.source_check?.next_period_no"><dt>{{ t('announced') }}</dt><dd>{{ config.source_check.next_period_no }} · {{ time(config.source_check.next_draw_at) }}</dd></div><div><dt>{{ t('lastResult') }}</dt><dd v-if="config.source_check?.last_result">{{ config.source_check.last_result.period_no }} · {{ numbers(config.source_check.last_result.result) }}<br>{{ time(config.source_check.last_result.drawn_at) }}</dd><dd v-else>{{ t('noResult') }}</dd></div></dl>
 <details v-if="config.attempts.length"><summary>{{ t('attempts') }}</summary><article v-for="attempt in config.attempts" :key="attempt.period_no" class="draw-attempt"><strong>{{ attempt.period_no }}</strong><span>{{ statusI18n.t(attempt.code) }}</span><span>{{ time(attempt.at) }} · {{ t('count') }} {{ attempt.attempts }}</span></article></details>
 </section>
 <section><div class="draw-overview-heading"><h3>{{ t('pending') }}</h3><button v-if="canManage&&canCreatePeriod!==false" class="button button-secondary" @click="emit('create')">{{ t('create') }}</button></div><p v-if="!data.pending.length" class="empty-state">{{ t('noPending') }}</p><div class="draw-pending-grid"><article v-for="period in data.pending" :key="period.id" class="panel"><div class="draw-overview-heading"><strong>{{ period.period_no }}</strong><span class="status-chip">{{ t(period.state) }}</span></div><dl class="draw-facts"><div v-if="showSaleTimes!==false"><dt>{{ t('opens') }}</dt><dd>{{ time(period.bet_start_at) }}</dd></div><div v-if="showSaleTimes!==false"><dt>{{ t('closes') }}</dt><dd>{{ time(period.bet_end_at) }}</dd></div><div><dt>{{ t('draws') }}</dt><dd>{{ time(period.draw_at) }}</dd></div></dl><button v-if="canManage && period.state === 'waiting'" class="button button-primary" @click="emit('publish', period)">{{ t('publish') }}</button><button v-if="canManage && canEditOpening!==false && (period.state==='scheduled'||period.state==='open')" class="button button-secondary" @click="emit('opening',period)">{{ t('editOpening') }}</button></article></div><p v-if="data.pending_has_more" class="draw-note">{{ t('morePending') }}</p></section>
 <ManagementList embedded><div class="draw-overview-heading"><h3>{{ t('history') }}</h3><label>{{ t('year') }} <SelectControl :model-value="year" :disabled="busy" @change="withGlobalLoading(load, $event, 0)" :label="String(t('year'))" :options="(data.years).map((choice) => ({ value: choice, label: (String(choice)).trim() }))" /></label></div><p v-if="!data.items.length" class="empty-state">{{ t('empty') }}</p><ol class="draw-history"><li v-for="item in data.items" :key="item.period_no"><div class="draw-history-heading"><strong>{{ item.period_no }}</strong><span>{{ date(item.draw_date) }}</span><span>{{ t(item.source) }}</span></div><div class="draw-numbers"><span v-for="(n, index) in item.result.regular" :key="`r${index}`">{{ String(n).padStart(2, '0') }}</span><b v-if="item.result.special.length">＋</b><span v-for="(n, index) in item.result.special" :key="`s${index}`" class="draw-special">{{ String(n).padStart(2, '0') }}</span><span v-for="(n,index) in item.result.digits" :key="`d${index}`">{{ n }}</span></div></li></ol><template #pagination><TablePagination :show-empty="Boolean(data)" :total="data.total_count" :page="Math.floor(offset / 50 + 1)" :count="data.items.length" :locale="locale" :page-size="50" :previous-disabled="busy || offset === 0" :next-disabled="busy || !data.has_more" :previous-label="t('previous')" :next-label="t('next')" :go-to="(target: number) => load(year, (target - 1) * 50)" /></template></ManagementList>
 </template>
</section>
</template>
<style scoped>
.draw-overview{display:grid;gap:16px;min-width:0;font-size:var(--body-font-size)}.draw-overview h3{margin:0;font-size:16px}.draw-overview-toolbar,.draw-overview-heading,.draw-history-heading{display:flex;align-items:center;flex-wrap:wrap;gap:8px}.draw-overview-heading{justify-content:space-between;margin-bottom:8px}.draw-overview-toolbar{color:var(--muted)}.draw-overview-toolbar button{flex:none}.draw-overview section{min-width:0}.draw-overview .panel{padding:12px}.draw-live{display:grid;gap:8px}.draw-live>.status-chip{justify-self:start}.draw-facts{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(240px,100%),1fr));gap:8px;margin:8px 0}.draw-overview .draw-facts>div{grid-template-columns:minmax(0,1fr)!important;gap:0!important;min-width:0;padding:0;border:0}.draw-facts dt{display:block;width:auto}.draw-facts dd{text-align:left;width:auto}.draw-facts dt,.draw-note,.draw-history-heading span{color:var(--muted);font-size:var(--body-font-size)}.draw-facts dd{margin:4px 0 0;overflow-wrap:anywhere}.draw-pending-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(320px,100%),1fr));gap:8px}.draw-note{margin:8px 0;line-height:1.6}.draw-history{list-style:none;padding:0;margin:0 0 12px;display:grid;gap:8px}.draw-history li{border:1px solid var(--line);border-radius:var(--radius-sm);padding:10px;min-width:0}.draw-history-heading{margin-bottom:8px}.draw-history-heading strong{color:var(--ink);font-variant-numeric:tabular-nums}.draw-numbers{display:flex;gap:6px;align-items:center;flex-wrap:wrap;font-variant-numeric:tabular-nums}.draw-numbers span{display:grid;place-items:center;min-width:28px;min-height:28px;border-radius:var(--radius-sm);background:var(--surface);border:1px solid var(--line);font-weight:600}.draw-numbers .draw-special{color:var(--primary);border-color:var(--primary)}.draw-attempt{display:flex;gap:8px;flex-wrap:wrap;align-items:center}.draw-attempt>div{min-width:0;overflow-wrap:anywhere}
</style>
