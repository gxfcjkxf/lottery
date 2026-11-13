<script setup lang="ts">
import { pointsMinor } from './points'
import SiteIcon from './SiteIcon'

import { computed, onMounted, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { withGlobalLoading, formatPoints, type LocalizedMessage, type MessageParameters } from './index'
import type { OverviewSummary, OverviewDay } from './overview'
import type { PlatformDisplaySettings } from './date-time'

const props = defineProps<{
  canView: boolean; locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings
  scope: 'platform' | 'brand'
  read: (signal: AbortSignal) => Promise<OverviewSummary>
  t: (key: string, parameters?: MessageParameters) => string
  errorMessage: (cause: unknown) => LocalizedMessage
  displayError: (message: LocalizedMessage) => string
}>()
const emit = defineEmits<{ failure: [cause: unknown]; open: []; openWithdrawals: [] }>()
const t = (key: string, parameters?: MessageParameters) => props.t(key, parameters)
const data = shallowRef<OverviewSummary | null>(null)
const error = ref<LocalizedMessage | null>(null)
const reading = ref(false)
const metric = ref<'stake_points' | 'net_points' | 'new_members'>('stake_points')
let controller = new AbortController()
let readGeneration = 0
let timer: ReturnType<typeof setInterval> | undefined
const number = (value: string) => formatPoints(value, props.locale)
const netTone = (value: string) => pointsMinor(value) === 0n ? 'overview-neutral' : pointsMinor(value) < 0n ? 'overview-loss' : 'overview-win'
const net = (value: string) => pointsMinor(value) > 0n ? `+${number(value)}` : number(value)
const today = computed(() => data.value!.days[6])
const yesterday = computed(() => data.value!.days[5])
const cards = computed(() => data.value ? [
  { label: 'overview.totalMembers', value: data.value.total_members, icon: 'users' },
  { label: 'overview.online', value: data.value.online_members, icon: 'user' },
  { label: 'overview.newYesterday', value: yesterday.value.new_members, icon: 'network' },
  { label: 'overview.bettorsToday', value: today.value.bettor_count, icon: 'ticket' },
] as const : [])
const comparisons = computed(() => data.value ? [
  { title: 'overview.today', day: today.value },
  { title: 'overview.yesterday', day: yesterday.value },
] : [])
const tasks = computed(() => data.value ? [
  { label: 'overview.withdrawals', value: data.value.withdrawal_count },
] : [])
const fields: { key: keyof OverviewDay; label: string }[] = [
  { key: 'net_points', label: 'overview.net' }, { key: 'order_count', label: 'overview.orders' },
  { key: 'bettor_count', label: 'overview.bettors' }, { key: 'refund_points', label: 'overview.refunds' },
]
const metrics = [
  { key: 'stake_points', label: 'overview.trendStakes' },
  { key: 'net_points', label: 'overview.trendNet' },
  { key: 'new_members', label: 'overview.trendNew' },
] as const
const chartBounds = computed(() => {
  const values = data.value!.days.map(day => pointsMinor(day[metric.value]))
  return { max: values.reduce((a, b) => b > a ? b : a, 0n), min: values.reduce((a, b) => b < a ? b : a, 0n) }
})
function bar(value: string) {
  const { max, min } = chartBounds.value, range = max - min, amount = pointsMinor(value)
  if (range === 0n) return { height: '0%', bottom: '0%' }
  return { height: `${Number((amount < 0n ? -amount : amount) * 10000n / range) / 100}%`, bottom: `${Number((amount < 0n ? amount - min : -min) * 10000n / range) / 100}%` }
}
const baseline = computed(() => {
  const { max, min } = chartBounds.value
  return max === min ? '0%' : `${Number(-min * 10000n / (max - min)) / 100}%`
})
const compact = (value: string) => new Intl.NumberFormat(props.locale, { notation: 'compact', maximumFractionDigits: 1 }).format(Number(value))
async function load() {
  if (reading.value || !props.canView || controller.signal.aborted) return
  const token = ++readGeneration, signal = controller.signal
  reading.value = true
  try {
    const result = await props.read(signal)
    if (token === readGeneration && !signal.aborted) { data.value = result; error.value = null }
  } catch (cause) {
    if (token === readGeneration && !signal.aborted) {
      data.value = null
      error.value = props.errorMessage(cause)
      emit('failure', cause)
    }
  } finally { if (token === readGeneration) reading.value = false }
}
watch(() => props.canView, () => {
  readGeneration++; controller.abort(); controller = new AbortController()
  data.value = null; error.value = null; reading.value = false
  if (props.canView) void withGlobalLoading(load)
}, { immediate: true, flush: 'sync' })
onMounted(() => {
  timer = setInterval(() => { if (document.visibilityState === 'visible') void load() }, 60000)
})
onBeforeUnmount(() => { readGeneration++; controller.abort(); clearInterval(timer) })
</script>

<template>
  <div class="overview" :data-testid="scope + '-overview'">
    <div class="page-heading compact-heading">
      <h1>{{ t('overview.title') }}</h1>
    </div>
    <p v-if="!canView" class="message" role="status">{{ t('overview.denied') }}</p>
    <div v-else-if="error" class="message error" role="alert">
      {{ props.displayError(error) }}
      <button type="button" class="button button-secondary" :disabled="reading" @click="withGlobalLoading(load)">{{ t('overview.retry') }}</button>
    </div>
    <template v-else-if="data">
      <section v-if="scope === 'brand'" class="panel overview-tasks overview-tasks-priority"><h2>{{ t('overview.tasks') }}</h2><button v-for="task in tasks" :key="task.label" type="button" class="button button-secondary overview-task overview-task-action" @click="emit('openWithdrawals')"><span>{{ t(task.label) }}</span><strong :class="BigInt(task.value) === 0n ? 'overview-neutral' : 'overview-pending'">{{ number(task.value) }}</strong></button></section>
      <section class="overview-stats" :aria-label="t('overview.totalMembers')">
        <article v-for="card in cards" :key="card.label" class="panel overview-stat">
          <p class="overview-muted overview-stat-label"><span class="overview-stat-icon"><SiteIcon :name="card.icon" /></span>{{ t(card.label) }}</p><strong class="overview-value overview-member">{{ number(card.value) }}</strong>
        </article>
      </section>
      <section v-if="scope === 'platform'" class="panel overview-tasks overview-tasks-priority"><h2>{{ t('overview.tasks') }}</h2><button v-for="task in tasks" :key="task.label" type="button" class="button button-secondary overview-task overview-task-action" @click="emit('openWithdrawals')"><span>{{ t(task.label) }}</span><strong :class="BigInt(task.value) === 0n ? 'overview-neutral' : 'overview-pending'">{{ number(task.value) }}</strong></button></section>
      <div class="overview-section-heading"><h2>{{ t('overview.betting') }}</h2></div>
      <section class="overview-comparisons">
        <article v-for="item in comparisons" :key="item.title" class="panel overview-business">
          <div class="overview-section-heading"><h3>{{ t(item.title) }}</h3></div>
          <p class="overview-muted">{{ t('overview.stakes') }}</p><strong class="overview-value overview-stake">{{ number(item.day.stake_points) }}</strong>
          <div class="overview-mini-grid"><div v-for="field in fields" :key="field.key"><p class="overview-muted">{{ t(field.label) }}</p><strong :class="field.key === 'net_points' ? netTone(item.day.net_points) : field.key === 'refund_points' ? 'overview-pending' : 'overview-member'">{{ field.key === 'net_points' ? net(item.day[field.key]) : number(item.day[field.key]) }}</strong></div></div>
        </article>
      </section>
      <section class="overview-current" :aria-label="t('overview.unsettled')">
        <div><p class="overview-muted">{{ t('overview.unsettled') }}</p><strong class="overview-pending">{{ number(data.unsettled_points) }}</strong></div>
        <div><p class="overview-muted">{{ t('overview.available') }}</p><strong class="overview-win">{{ number(data.available_points) }}</strong></div>
        <div><p class="overview-muted">{{ t('overview.withdrawalPoints') }}</p><strong class="overview-pending">{{ number(data.withdrawal_points) }}</strong></div>
      </section>
      <section class="panel overview-trend">
        <div class="overview-section-heading"><h2>{{ t('overview.trend') }}</h2><div class="overview-actions" role="group" :aria-label="t('overview.trend')"><button v-for="option in metrics" :key="option.key" type="button" class="button" :class="metric === option.key ? 'button-primary' : 'button-secondary'" :aria-pressed="metric === option.key" @click="metric = option.key">{{ t(option.label) }}</button></div></div>
        <p class="overview-muted">{{ t(metric === 'new_members' ? 'overview.membersUnit' : 'overview.pointsUnit') }}</p>
        <div class="overview-chart">
          <div v-for="day in data.days" :key="day.day" class="overview-chart-column" :aria-label="`${day.day}: ${number(day[metric])}`">
            <span class="overview-chart-value" :class="metric === 'net_points' ? netTone(day[metric]) : metric === 'new_members' ? 'overview-member' : 'overview-stake'" :title="number(day[metric])">{{ compact(day[metric]) }}</span>
            <div class="overview-chart-track"><span class="overview-baseline" :style="{ bottom: baseline }" /><span class="overview-bar" :class="{ 'overview-bar-stake': metric === 'stake_points', 'overview-bar-loss': pointsMinor(day[metric]) < 0n }" :style="bar(day[metric])" /></div>
            <span class="overview-chart-date">{{ day.day.slice(5).replace('-', '/') }}</span>
          </div>
        </div>
      </section>
      <section v-if="scope === 'platform'" class="panel overview-ranking"><h2>{{ t('overview.brands') }}</h2>
        <div role="table" :aria-label="t('overview.brands')"><div role="row" class="overview-brand-row overview-muted"><span role="columnheader">{{ t('overview.brand') }}</span><span role="columnheader">{{ t('overview.trendStakes') }}</span><span role="columnheader">{{ t('overview.trendNet') }}</span></div>
          <div v-for="brand in data.brands" :key="brand.brand_id" class="overview-brand-row" role="row"><span role="cell">{{ brand.brand_name }}</span><span role="cell" class="overview-stake">{{ number(brand.stake_points) }}</span><span role="cell" :class="netTone(brand.net_points)">{{ net(brand.net_points) }}</span></div>
        </div><p v-if="!data.brands!.length" class="empty-state">{{ t('overview.emptyBrands') }}</p>
      </section>
    </template>
  </div>
</template>

<style scoped>
.overview { min-width: 0; }
.overview p, .overview h2, .overview h3 { margin: 0; }
.overview h2 { font-size: 16px; }
.overview h3 { font-size: 14px; }
.overview-muted { font-size: var(--body-font-size); color: var(--muted); }
.overview-section-heading, .overview-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.overview-section-heading { justify-content: space-between; margin-bottom: 12px; }
.overview-stats { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; margin-bottom: 20px; padding: 12px; border: 1px solid var(--line); border-radius: var(--radius); background: var(--canvas); background-size: cover; background-position: center; }
.overview-stat { padding: 14px; min-width: 0; margin: 0; background: var(--surface); border: 1px solid var(--line); border-radius: var(--radius-sm); }
.overview-stat-label { display: flex; align-items: center; gap: 8px; }
.overview-stat-icon { display: inline-grid; place-items: center; flex-shrink: 0; width: 28px; height: 28px; border-radius: var(--radius-sm); color: var(--primary-dark); background: var(--primary-light); }
.overview-value { display: block; font-family: var(--font-mono, ui-monospace, monospace); font-size: 28px; line-height: 1.3; font-weight: 750; letter-spacing: -.5px; margin: 6px 0; }
.overview strong { font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.overview-comparisons { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.overview-business { padding: 16px; min-width: 0; margin: 0; }
.overview-business .overview-value { font-size: 30px; }
.overview-mini-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; margin-top: 16px; padding-top: 12px; border-top: 1px solid var(--line); }
.overview-mini-grid strong, .overview-current strong { display: block; font-size: 18px; margin-top: 4px; }
.overview-current { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; margin: 16px 0 20px; }
.overview-current > div { min-width: 0; }
.overview-trend, .overview-ranking, .overview-tasks { padding: 16px; min-width: 0; }
.overview-chart { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); gap: 8px; margin-top: 16px; }
.overview-chart-column { display: flex; flex-direction: column; align-items: center; gap: 8px; min-width: 0; }
.overview-chart-value, .overview-chart-date { font-size: var(--body-font-size); font-variant-numeric: tabular-nums; color: var(--muted); overflow-wrap: anywhere; }
.overview-chart-track { height: 140px; width: 100%; position: relative; }
.overview-bar { position: absolute; background: var(--line); width: 64%; max-width: 44px; left: 50%; transform: translateX(-50%); border-radius: 4px 4px 0 0; transition: background-color 150ms ease; }
.overview-chart-column:nth-child(n+5) .overview-bar { background: color-mix(in srgb, var(--primary) 55%, var(--line)); }
.overview-chart-column:last-child .overview-bar { background: var(--primary); }
.overview-bar-stake { background: color-mix(in srgb, var(--warning) 60%, var(--line)); }
.overview-chart-column:last-child .overview-bar-stake { background: var(--warning-text); }
.overview-bar-loss { background: var(--danger); border-radius: 0 0 4px 4px; }
.overview-baseline { position: absolute; height: 1px; left: 0; right: 0; background: var(--line); }
.overview-ranking { margin-top: 16px; }
.overview-brand-row { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; align-items: center; padding: 10px 0; border-bottom: 1px solid var(--line); }
.overview-brand-row > span { min-width: 0; overflow-wrap: anywhere; }
.overview-brand-row > span:not(:first-child) { text-align: right; font-variant-numeric: tabular-nums; }
.overview-tasks-priority { margin: 0 0 20px; border-left: 3px solid var(--warning-text); }
.overview .overview-task-action { width: 100%; min-height: 44px; margin-top: 10px; padding: 10px 12px; text-align: left; }
.overview-task { display: flex; gap: 8px; align-items: center; justify-content: space-between; padding: 10px 0; border-bottom: 1px solid var(--line); }
.overview-task strong { font-size: 18px; }
.overview .overview-member { color: var(--primary-dark); }
.overview .overview-stake, .overview .overview-pending { color: var(--warning-text); }
.overview .overview-win { color: var(--success-text); }
.overview .overview-loss { color: var(--danger-text); }
.overview .overview-neutral { color: var(--muted); }
@media (max-width: 900px) { .overview-stats { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 620px) {
  .overview-comparisons, .overview-current { grid-template-columns: minmax(0, 1fr); }
  .overview-current > div { padding: 8px 0; border-bottom: 1px solid var(--line); }
  .overview-chart { gap: 4px; }
}
</style>
