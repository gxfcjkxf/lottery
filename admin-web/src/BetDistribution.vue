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
  game_id: string; period_no: string; total_stake: string
  numbers: Array<{ number: number; wave: 'red' | 'blue' | 'green'; zodiac: string; stake: string; profit: string }>
}
const games = shallowRef<Array<{ id: string; name: string }>>([])
const game = ref(''), period = ref(''), busy = ref(false), failed = ref(false), initialized = ref(false)
const result = shallowRef<Distribution | null>(null)
const sort = ref<'lossToWin' | 'winToLoss' | 'number'>('number')
const sortOptions = computed(() => [
  { value: 'lossToWin', label: t('distribution.lossToWin') },
  { value: 'winToLoss', label: t('distribution.winToLoss') },
  { value: 'number', label: t('distribution.number') },
] as const)
const sortedNumbers = computed(() => [...(result.value?.numbers ?? [])].sort((a, b) => {
  if (sort.value === 'number') return a.number - b.number
  const left = pointsMinor(a.profit), right = pointsMinor(b.profit)
  if (left === right) return a.number - b.number
  return (left < right ? -1 : 1) * (sort.value === 'lossToWin' ? 1 : -1)
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
  const id = ++generation
  busy.value = true; failed.value = false; result.value = null
  try {
    const search = new URLSearchParams({ game_id: game.value, period_no: period.value.trim() })
    const data = await read<Distribution>(`bet-distribution?${search}`)
    if (id === generation) { result.value = data; period.value = data.period_no }
  } catch (cause) { if (id === generation) failure(cause) }
  finally { if (id === generation) busy.value = false }
}
async function initialize() {
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
  game.value = value; period.value = ''; void withGlobalLoading(query)
}
watch(() => props.brandId, () => { void withGlobalLoading(initialize) }, { immediate: true })
onBeforeUnmount(() => { generation++ })
function roundedPoints(value: string) {
  const minor = pointsMinor(value)
  const magnitude = ((minor < 0n ? -minor : minor) + 50n) / 100n
  return minor < 0n ? -magnitude : magnitude
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
        <SelectControl :model-value="game" :label="t('distribution.game')" :disabled="busy || !games.length" :options="games.map(item => ({ value: item.id, label: item.name }))" @update:model-value="changeGame(String($event))" />
        <ClearableInput v-model="period" :locale="locale" class="field" :aria-label="t('distribution.period')" :placeholder="t('distribution.period')" maxlength="100" :disabled="busy || !games.length" />
        <button class="button button-primary" type="submit" :disabled="busy || (initialized && !games.length)">{{ t('distribution.query') }}</button>
      </form>
    </template>
    <p v-if="failed" class="empty-state" role="alert">{{ t('distribution.failed') }}</p>
    <p v-else-if="initialized && !games.length" class="empty-state">{{ t('distribution.emptyGames') }}</p>
    <section v-else-if="result" class="distribution-results" :aria-label="t('distribution.special')">
      <div class="distribution-heading">
        <h2>{{ t('distribution.special') }}</h2>
        <div class="distribution-tools">
          <div class="distribution-sort" role="group" :aria-label="t('distribution.sort')">
            <button v-for="option in sortOptions" :key="option.value" type="button" class="button" :class="sort === option.value ? 'button-primary' : 'button-secondary'" :aria-pressed="sort === option.value" @click="sort = option.value">{{ option.label }}</button>
          </div>
          <div class="distribution-total"><span>{{ t('distribution.total') }}</span><strong>{{ wholePoints(result.total_stake) }}</strong></div>
        </div>
      </div>
      <div class="distribution-grid">
        <article v-for="item in sortedNumbers" :key="item.number" class="distribution-number" :class="highlights.get(item.number)">
          <div class="distribution-identity">
            <h3 :class="`lottery-wave-${item.wave}`" :aria-label="`${t('distribution.number')} ${String(item.number).padStart(2, '0')}`">{{ String(item.number).padStart(2, '0') }}</h3>
            <span class="distribution-zodiac">{{ attributesI18n.t(`binding.${item.zodiac}`) }}</span>
          </div>
          <dl>
            <div><dt>{{ t('distribution.stake') }}</dt><dd>{{ wholePoints(item.stake) }}</dd></div>
            <div><dt>{{ t('distribution.profit') }}</dt><dd :class="{ 'distribution-win': pointsMinor(item.profit) > 0n, 'distribution-loss': pointsMinor(item.profit) < 0n }">{{ wholePoints(item.profit, true) }}</dd></div>
          </dl>
        </article>
      </div>
    </section>
  </ManagementList>
</template>

<style src="../../shared/src/mark-six-balls.css"></style>
<style scoped>
.bet-distribution { min-width: 0; }
.distribution-results { min-width: 0; padding: 0 12px 12px; }
.distribution-heading { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 16px; }
.distribution-heading h2 { margin: 0; font-size: 16px; color: var(--ink); }
.distribution-tools { display: flex; flex-wrap: wrap; align-items: center; justify-content: flex-end; gap: 12px; min-width: 0; max-width: 100%; }
.distribution-sort { display: flex; gap: 6px; min-width: 0; max-width: 100%; }
.distribution-sort .button { flex: 1 1 auto; min-width: 0; padding-inline: 10px; white-space: normal; }
.distribution-total { display: flex; flex-wrap: wrap; align-items: baseline; gap: 12px; color: var(--ink-secondary); }
.distribution-total strong { color: var(--ink); font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.distribution-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 8px; }
.distribution-number { min-width: 0; padding: 12px; border: 1px solid var(--line); border-radius: var(--radius-sm); background: var(--surface); box-shadow: var(--shadow-sm); transition: border-color 150ms ease, box-shadow 150ms ease; }
.distribution-number:hover { border-color: #cbd5e1; }
.distribution-identity { display: flex; align-items: center; gap: 8px; min-width: 0; margin-bottom: 12px; }
.distribution-zodiac { color: var(--ink-secondary); font-weight: 650; overflow-wrap: anywhere; }
.distribution-number h3 { display: grid; place-items: center; width: 38px; height: 38px; margin: 0; border-radius: 999px; background: radial-gradient(circle at 32% 28%, rgba(255, 255, 255, 0.42) 0%, transparent 45%), var(--wave-color); color: var(--lottery-ball-ink); flex-shrink: 0; font-size: 16px; font-weight: 700; font-variant-numeric: tabular-nums; box-shadow: inset 0 -2px 4px rgba(15, 23, 20, 0.22), 0 1px 2px rgba(15, 23, 20, 0.12); }
.distribution-number dl { display: grid; gap: 8px; margin: 0; }
.distribution-number dt { color: var(--muted); }
.distribution-number dd { margin: 0; font-weight: 650; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; color: var(--ink); }
.distribution-top-win-1 { border-color: #a7f3d0; background: #ecfdf5; }
.distribution-top-win-2 { border-color: #bbf7d0; background: #f0fdf4; }
.distribution-top-win-3 { border-color: #d1fae5; background: #f7fef9; }
.distribution-top-loss-1 { border-color: #fda4af; background: #fff1f2; }
.distribution-top-loss-2 { border-color: #fecdd3; background: #fff5f6; }
.distribution-top-loss-3 { border-color: #ffe4e6; background: #fff8f9; }
.distribution-win { color: var(--success-text); }
.distribution-loss { color: var(--danger-text); }
@media (max-width: 640px) {
  .distribution-grid { grid-template-columns: minmax(0, 1fr); }
  .distribution-number { display: grid; grid-template-columns: auto minmax(0, 1fr); align-items: center; gap: 12px; }
  .distribution-identity { margin: 0; }
}
</style>
