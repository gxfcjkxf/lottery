<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { createApiClient, createCatalogI18n, configuredRuleCatalog, pointsMinor, withGlobalLoading, vInlineValidation } from '@lottery/shared'
import ManagementList from '@lottery/shared/ManagementList.vue'
import SelectControl from '@lottery/shared/SelectControl.vue'
import ClearableInput from '@lottery/shared/ClearableInput.vue'
import { useAdminI18n } from './i18n'

const props = defineProps<{ brandId: string }>()
const emit = defineEmits<{ 'session-invalid': [] }>()
const { t, locale } = useAdminI18n()
const attributesI18n = createCatalogI18n(configuredRuleCatalog, locale.value)
watch(locale, value => attributesI18n.setLocale(value))
interface Distribution {
  number_digits: 2 | 3; game_id: string; period_no: string; total_stake: string
  numbers: Array<{ number: number; wave: '' | 'red' | 'blue' | 'green'; zodiac: string; stake: string; profit: string }>
}
const games = shallowRef<Array<{ id: string; name: string }>>([])
const game = ref(''), period = ref(''), busy = ref(false), failed = ref(false), initialized = ref(false)
const result = shallowRef<Distribution | null>(null)
const canadaSelected = ref(false)
const rateMin = ref(''), rateMax = ref('')
const appliedRate = ref({ min: '', max: '' })
const visibleNumbers = computed(() => {
  const data = result.value
  if (!data) return []
  if (data.number_digits !== 3 || (appliedRate.value.min === '' && appliedRate.value.max === '')) return data.numbers
  const total = pointsMinor(data.total_stake)
  if (total === 0n) return []
  return data.numbers.filter(item => {
    const returned = (total - pointsMinor(item.profit)) * 100n
    return (appliedRate.value.min === '' || returned >= BigInt(Number(appliedRate.value.min)) * total)
      && (appliedRate.value.max === '' || returned <= BigInt(Number(appliedRate.value.max)) * total)
  })
})
function payoutRate(profit: string) {
  const total = pointsMinor(result.value!.total_stake)
  if (total === 0n) return '—'
  const returned = total - pointsMinor(profit)
  return `${new Intl.NumberFormat(locale.value).format((returned * 100n + total / 2n) / total)}%`
}
const sort = ref<'lossToWin' | 'winToLoss' | 'number' | 'rateAscending' | 'rateDescending'>('number')
const sortOptions = computed(() => [
  { value: 'lossToWin', label: t('distribution.lossToWin') },
  { value: 'winToLoss', label: t('distribution.winToLoss') },
  { value: 'number', label: t('distribution.number') },
  ...(canadaSelected.value ? [
    { value: 'rateAscending', label: t('distribution.rateAscending') },
    { value: 'rateDescending', label: t('distribution.rateDescending') },
  ] as const : []),
] as const)
const sortedNumbers = computed(() => [...visibleNumbers.value].sort((a, b) => {
  if (sort.value === 'number' || ((sort.value === 'rateAscending' || sort.value === 'rateDescending') && pointsMinor(result.value!.total_stake) === 0n)) return a.number - b.number
  // All outcomes share the total stake, so payout rate orders opposite to profit.
  const left = pointsMinor(a.profit), right = pointsMinor(b.profit)
  if (left === right) return a.number - b.number
  return (left < right ? -1 : 1) * (sort.value === 'lossToWin' || sort.value === 'rateDescending' ? 1 : -1)
}))
const highlights = computed(() => {
  const ranks = new Map<number, string>()
  const numbers = (result.value?.numbers ?? []).map(item => ({ number: item.number, profit: roundedPoints(item.profit) }))
  for (const side of ['win', 'loss'] as const) {
    const ordered = numbers.filter(item => side === 'win' ? item.profit > 0n : item.profit < 0n)
      .sort((a, b) => a.profit === b.profit ? a.number - b.number : (a.profit > b.profit ? -1 : 1) * (side === 'win' ? 1 : -1))
    const amounts = [...new Set(ordered.map(item => item.profit))].slice(0, 3)
    for (const item of ordered) {
      const rank = amounts.indexOf(item.profit)
      if (rank >= 0) ranks.set(item.number, `distribution-top-${side}-${rank + 1}`)
    }
  }
  return ranks
})
let generation = 0
const api = createApiClient()
function read<T>(path: string) { return api.request<T>(`/api/v1/admin/${path}`, { headers: { 'X-Brand-ID': props.brandId } }) }
function failure(cause: unknown) {
  failed.value = true
  if ((cause as { status?: number }).status === 401) emit('session-invalid')
}
async function query() {
  const range = { min: rateMin.value, max: rateMax.value }
  const id = ++generation
  busy.value = true; failed.value = false; result.value = null
  try {
    const search = new URLSearchParams({ game_id: game.value, period_no: period.value.trim() })
    const data = await read<Distribution>(`bet-distribution?${search}`)
    if (id === generation) { result.value = data; period.value = data.period_no; canadaSelected.value = data.number_digits === 3; appliedRate.value = range }
  } catch (cause) { if (id === generation) failure(cause) }
  finally { if (id === generation) busy.value = false }
}
async function initialize() {
  if (sort.value === 'rateAscending' || sort.value === 'rateDescending') sort.value = 'number'
  canadaSelected.value = false; rateMin.value = ''; rateMax.value = ''; appliedRate.value = { min: '', max: '' }
  const id = ++generation
  busy.value = true; failed.value = false; initialized.value = false; result.value = null; games.value = []; game.value = ''; period.value = ''
  try {
    const data = await read<{ games: Array<{ id: string; name: string }> }>('bet-distribution/games')
    if (id !== generation) return
    games.value = data.games; initialized.value = true
    if (data.games.length) { game.value = data.games[0]!.id; await query() }
  } catch (cause) { if (id === generation) failure(cause) }
  finally { if (id === generation) busy.value = false }
}
function changeGame(value: string) {
  if (sort.value === 'rateAscending' || sort.value === 'rateDescending') sort.value = 'number'
  canadaSelected.value = false; rateMin.value = ''; rateMax.value = ''; appliedRate.value = { min: '', max: '' }
  game.value = value; period.value = ''; void withGlobalLoading(query)
}
watch(() => props.brandId, () => { void withGlobalLoading(initialize) }, { immediate: true })
onBeforeUnmount(() => { generation++ })
function roundedPoints(value: string) {
  const minor = pointsMinor(value)
  const magnitude = ((minor < 0n ? -minor : minor) + 50n) / 100n
  return minor < 0n ? -magnitude : magnitude
}
function wholePointParts(value: string, signed = false) {
  return new Intl.NumberFormat(locale.value, { signDisplay: signed ? 'exceptZero' : 'auto' }).formatToParts(roundedPoints(value))
}
function wholePoints(value: string, signed = false) {
  const whole = roundedPoints(value)
  return `${signed && whole > 0n ? '+' : ''}${new Intl.NumberFormat(locale.value).format(whole)}`
}
</script>

<template>
  <ManagementList class="bet-distribution" :busy="busy">
    <template #filters>
      <form novalidate v-inline-validation class="ui-filter-bar" @submit.prevent="withGlobalLoading(initialized ? query : initialize)">
        <div class="distribution-period-fields">
          <SelectControl class="distribution-game" :model-value="game" :label="t('distribution.game')" :disabled="busy || !games.length" :options="games.map(item => ({ value: item.id, label: item.name }))" @update:model-value="changeGame(String($event))" />
          <ClearableInput v-model="period" :locale="locale" class="field distribution-period" :aria-label="t('distribution.period')" :placeholder="t('distribution.period')" maxlength="100" :disabled="busy || !games.length" />
        </div>
        <div v-if="canadaSelected" class="distribution-rate-filter" role="group" :aria-label="t('distribution.rate')">
          <span>{{ t('distribution.rate') }}</span>
          <input :value="rateMin" type="number" min="0" :max="rateMax === '' ? undefined : rateMax" step="1" inputmode="numeric" :aria-label="t('distribution.rateMin')" :placeholder="t('distribution.rateMin')" :disabled="busy" @input="rateMin = ($event.target as HTMLInputElement).value" />
          <span aria-hidden="true">–</span>
          <input :value="rateMax" type="number" :min="rateMin === '' ? 0 : rateMin" step="1" inputmode="numeric" :aria-label="t('distribution.rateMax')" :placeholder="t('distribution.rateMax')" :disabled="busy" @input="rateMax = ($event.target as HTMLInputElement).value" />
          <span>%</span>
        </div>
        <div class="distribution-query-actions">
          <SelectControl v-model="sort" :label="t('distribution.sort')" :options="sortOptions" :disabled="busy || !games.length" />
          <button class="button button-primary" type="submit" :disabled="busy || (initialized && !games.length)">{{ t('distribution.query') }}</button>
        </div>
      </form>
    </template>
    <p v-if="failed" class="empty-state" role="alert">{{ t('distribution.failed') }}</p>
    <p v-else-if="initialized && !games.length" class="empty-state">{{ t('distribution.emptyGames') }}</p>
    <section v-else-if="result" class="distribution-results" :aria-label="t(result.number_digits === 3 ? 'distribution.draw' : 'distribution.special')">
      <div class="distribution-heading">
        <h2>{{ t(result.number_digits === 3 ? 'distribution.draw' : 'distribution.special') }}</h2>
        <div class="distribution-total"><span>{{ t('distribution.total') }}</span><strong>{{ wholePoints(result.total_stake) }}</strong></div>
      </div>
      <p v-if="!sortedNumbers.length" class="empty-state" role="status">{{ t('distribution.emptyRange') }}</p>
      <div v-else class="distribution-grid">
        <article v-for="item in sortedNumbers" :key="item.number" class="distribution-number" :class="[highlights.get(item.number), { 'distribution-three-digits': result.number_digits === 3 }]">
          <div class="distribution-identity">
            <h3 :class="result.number_digits === 3 ? 'distribution-digits' : `lottery-wave-${item.wave}`" :aria-label="`${t('distribution.number')} ${String(item.number).padStart(result.number_digits, '0')}`">{{ String(item.number).padStart(result.number_digits, '0') }}</h3>
            <span v-if="result.number_digits === 2" class="distribution-zodiac">{{ attributesI18n.t(`binding.${item.zodiac}`) }}</span>
          </div>
          <dl>
            <div v-if="result.number_digits === 3" class="distribution-card-rate"><dt>{{ t('distribution.rate') }}</dt><dd :title="t('distribution.rate')">{{ payoutRate(item.profit) }}</dd></div>
            <div class="distribution-card-stake"><dt>{{ t('distribution.stake') }}</dt><dd :title="t('distribution.stake')"><span v-for="(part, index) in wholePointParts(item.stake)" :key="index" :class="{ 'distribution-amount-group': part.type === 'group' }">{{ part.value }}</span></dd></div>
            <div class="distribution-card-profit"><dt>{{ t('distribution.profit') }}</dt><dd :title="t('distribution.profit')" :class="{ 'distribution-win': pointsMinor(item.profit) > 0n, 'distribution-loss': pointsMinor(item.profit) < 0n }"><span v-for="(part, index) in wholePointParts(item.profit, true)" :key="index" :class="{ 'distribution-amount-group': part.type === 'group' }">{{ part.value }}</span></dd></div>
          </dl>
        </article>
      </div>
    </section>
  </ManagementList>
</template>

<style src="../../shared/src/mark-six-balls.css"></style>
<style scoped>
.bet-distribution { min-width: 0; }
:global(#app .bet-distribution .distribution-period-fields .clearable-input > input) { width: 100% !important; }
.distribution-period-fields { display: flex; align-items: center; gap: 6px; min-width: 0; max-width: 100%; }
.distribution-query-actions { display: flex; align-items: center; gap: 6px; min-width: 0; max-width: 100%; }
.distribution-rate-filter { display: flex; align-items: center; gap: 6px; min-width: 0; max-width: 100%; }
.distribution-rate-filter > span { flex: none; white-space: nowrap; }
:global(#app .bet-distribution .ui-filter-bar .distribution-rate-filter input) { width: 88px !important; flex: 1 1 88px; min-width: 0; }
.distribution-results { min-width: 0; padding: 0 12px 12px; }
.distribution-heading { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 16px; }
.distribution-heading h2 { margin: 0; font-size: 16px; color: var(--ink); }
.distribution-total { display: flex; flex-wrap: wrap; align-items: baseline; gap: 12px; color: var(--ink-secondary); }
.distribution-total strong { color: var(--ink); font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.distribution-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 8px; }
.distribution-number { min-width: 0; padding: 12px; border: 1px solid var(--line); border-radius: var(--radius-sm); background: var(--surface); box-shadow: var(--shadow-sm); transition: border-color 150ms ease, box-shadow 150ms ease; }
.distribution-number:hover { border-color: var(--ink-secondary); }
.distribution-identity { display: flex; align-items: center; gap: 8px; min-width: 0; margin-bottom: 12px; }
.distribution-zodiac { color: var(--ink-secondary); font-weight: 650; overflow-wrap: anywhere; }
.distribution-number h3 { display: grid; place-items: center; width: 38px; height: 38px; margin: 0; border-radius: 999px; background: radial-gradient(circle at 32% 28%, rgba(255, 255, 255, 0.42) 0%, transparent 45%), var(--wave-color); color: var(--lottery-ball-ink); flex-shrink: 0; font-size: 16px; font-weight: 700; font-variant-numeric: tabular-nums; box-shadow: inset 0 -2px 4px rgba(15, 23, 20, 0.22), 0 1px 2px rgba(15, 23, 20, 0.12); }
.distribution-number h3.distribution-digits { width: 52px; border-radius: var(--radius-sm); background: var(--table-header-bg); color: var(--ink); box-shadow: none; }
.distribution-number dl { display: grid; gap: 8px; margin: 0; }
.distribution-number dt { color: var(--muted); }
.distribution-number dd { margin: 0; font-weight: 650; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; color: var(--ink); }
.distribution-win { color: var(--success-text); }
.distribution-loss { color: var(--danger-text); }
@media (max-width: 640px) {
  :global(#app .bet-distribution .ui-filter-bar) { gap: 4px 6px; }
  .distribution-period-fields { flex-basis: 100%; }
  .distribution-period-fields > .distribution-game { flex: 1 1 0; }
  .distribution-period-fields > :deep(.clearable-input) { flex: 1.15 1 0; width: 0; }
  .distribution-game :deep(.select-control-trigger) { width: 100%; }
  .distribution-game :deep(.select-control-measure) { display: none; }
  .distribution-game :deep(.select-control-value > span:last-child) { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

}
@media (max-width: 900px) {
  .distribution-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 6px; }
  .distribution-number { padding: 4px; }
  .distribution-identity { margin: 0; gap: 6px; }
  .distribution-number h3.distribution-digits { width: 44px; height: 24px; }
  .distribution-amount-group { display: none; }
  .distribution-card-stake :is(dt, dd), .distribution-card-profit :is(dt, dd) { white-space: nowrap; }
  .distribution-card-rate dd { white-space: nowrap; }
  .distribution-number dd { margin-inline-start: auto; max-width: 100%; }
  .distribution-heading { gap: 8px; margin-bottom: 8px; }
}
.distribution-number { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); align-content: start; gap: 8px 6px; }
.distribution-number .distribution-identity { margin: 0; }
.distribution-number:not(.distribution-three-digits) .distribution-identity { grid-column: 1 / -1; }
.distribution-number dl { display: contents; }
.distribution-number dl > div { grid-template-columns: minmax(0, 1fr) !important; column-gap: 0 !important; min-width: 0; }
.distribution-number dl > .distribution-card-rate { grid-column: 2; grid-row: 1; align-self: center; text-align: right; }
.distribution-number dl > .distribution-card-stake { grid-column: 1; grid-row: 2; }
.distribution-number dl > .distribution-card-profit { grid-column: 2; grid-row: 2; text-align: right; }
.distribution-number dt { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
.distribution-number .distribution-card-rate dd { text-align: right; color: var(--warning-text); }
.distribution-number .distribution-card-stake dd { text-align: left; color: var(--info-text); }
.distribution-number .distribution-card-profit dd { text-align: right; color: var(--success-text); }
.distribution-number .distribution-card-profit dd.distribution-loss { color: var(--danger-text); }
.distribution-number[class*="distribution-top-win-"] { border-color: color-mix(in srgb, var(--success-text) 32%, var(--line)); background: color-mix(in srgb, var(--success-text) var(--distribution-highlight), var(--surface)); }
.distribution-number[class*="distribution-top-loss-"] { border-color: color-mix(in srgb, var(--danger-text) 32%, var(--line)); background: color-mix(in srgb, var(--danger-text) var(--distribution-highlight), var(--surface)); }
.distribution-number:is(.distribution-top-win-1, .distribution-top-loss-1) { --distribution-highlight: 12%; }
.distribution-number:is(.distribution-top-win-2, .distribution-top-loss-2) { --distribution-highlight: 8%; }
.distribution-number:is(.distribution-top-win-3, .distribution-top-loss-3) { --distribution-highlight: 4%; }
@media (max-width: 900px) {
  .distribution-number { gap: 4px 6px; }
}
</style>
