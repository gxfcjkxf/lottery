<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { vInlineValidation, withGlobalLoading } from '@lottery/shared'
import ManagementList from '@lottery/shared/ManagementList.vue'
import ManagementTable from '@lottery/shared/ManagementTable.vue'
import PlatformTemporalInput from '@lottery/shared/PlatformTemporalInput.vue'
import SelectControl from '@lottery/shared/SelectControl.vue'
import TablePagination from '@lottery/shared/TablePagination.vue'
import { usePlatformI18n } from './i18n'
import { createPlatformApi, PlatformApiError } from './platform-api'
import type { PlatformDisplaySettings } from './platform-format'

const props = defineProps<{ locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const { t } = usePlatformI18n()
const api = createPlatformApi()

function defaultDates() {
  const parts = new Intl.DateTimeFormat('en', {
    timeZone: props.displaySettings.timezone, year: 'numeric', month: '2-digit', day: '2-digit',
  }).formatToParts()
  const part = (type: string) => parts.find(item => item.type === type)!.value
  const today = `${part('year')}-${part('month')}-${part('day')}`
  const start = new Date(`${today}T00:00:00Z`)
  start.setUTCDate(start.getUTCDate() - 6)
  return { from: start.toISOString().slice(0, 10), to: today }
}

const defaults = defaultDates()
const gameId = ref('')
const from = ref(defaults.from)
const to = ref(defaults.to)
const showSummary = ref(true)
const showDetails = ref(true)
const catalogBusy = ref(true)
const catalogFailed = ref(false)
const games = ref<Awaited<ReturnType<typeof api.betFrequencyGames>>['items']>([])
const result = ref<Awaited<ReturnType<typeof api.betFrequency>> | null>(null)
const queryError = ref('')
const busy = ref(false)
const pageSize = ref(20)
const page = ref(1)
let mounted = true

const columns = computed(() => [
  { key: 'play', label: t('betFrequency.play'), cellClass: 'frequency-play' },
  { key: 'count', label: t('betFrequency.items'), cellClass: 'frequency-number' },
  { key: 'share', label: t('betFrequency.share'), cellClass: 'frequency-number' },
])
const integerFormat = computed(() => new Intl.NumberFormat(props.locale))
const percentFormat = computed(() => new Intl.NumberFormat(props.locale, { style: 'percent', minimumFractionDigits: 2, maximumFractionDigits: 2 }))
const entries = computed(() => result.value?.items.map(item => ({ ...item, id: item.play_id })) ?? [])
const canSearch = computed(() => !!gameId.value && !!from.value && !!to.value && !catalogBusy.value && !busy.value)

function share(count: string, total: string) {
  const denominator = BigInt(total)
  const rounded = (BigInt(count) * 10000n + denominator / 2n) / denominator
  return percentFormat.value.format(Number(rounded) / 10000)
}

function sessionFailure(cause: unknown) {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
}

async function loadGames() {
  catalogBusy.value = true
  catalogFailed.value = false
  try {
    const data = await api.betFrequencyGames()
    if (mounted) games.value = data.items
  } catch (cause) {
    if (mounted) {
      catalogFailed.value = true
      sessionFailure(cause)
    }
  } finally {
    if (mounted) catalogBusy.value = false
  }
}

async function load(next = 1, size = pageSize.value) {
  if (!canSearch.value) return
  busy.value = true
  queryError.value = ''
  try {
    const data = await api.betFrequency({ game_id: gameId.value, from: from.value, to: to.value, limit: size, offset: (next - 1) * size })
    if (mounted) {
      result.value = data
      page.value = next
      pageSize.value = size
    }
  } catch (cause) {
    if (mounted) {
      const code = cause instanceof PlatformApiError ? cause.code : undefined
      queryError.value = code === 'REPORT_QUERY_INVALID' ? 'betFrequency.invalid'
        : code === 'REPORT_SCOPE_NOT_FOUND' ? 'betFrequency.gameMissing' : 'betFrequency.failed'
      sessionFailure(cause)
    }
  } finally {
    if (mounted) busy.value = false
  }
}

function clear() {
  const dates = defaultDates()
  gameId.value = ''
  from.value = dates.from
  to.value = dates.to
  result.value = null
  queryError.value = ''
}

watch([gameId, from, to], () => {
  result.value = null
  queryError.value = ''
})
onMounted(() => withGlobalLoading(loadGames))
onBeforeUnmount(() => { mounted = false })
</script>

<template>
  <ManagementList data-testid="platform-bet-frequency" :busy="busy || catalogBusy">
    <template v-if="$slots.tabs" #tabs><slot name="tabs" /></template>
    <template #filters>
      <form novalidate v-inline-validation class="ui-filter-bar" @submit.prevent="withGlobalLoading(load)">
        <SelectControl
          v-model="gameId" :label="t('filter.game')" :placeholder="t('filter.game')"
          :options="games.map(game => ({ value: game.id, label: game.name }))"
          :disabled="busy || catalogBusy || !games.length" required
        />
        <PlatformTemporalInput
          v-model="from" type="date" :locale="locale" :label="t('platform.reports.fromDate')"
          :placeholder="t('platform.reports.fromDate')" :disabled="busy" required
        />
        <PlatformTemporalInput
          v-model="to" type="date" :locale="locale" :label="t('platform.reports.toDate')"
          :placeholder="t('platform.reports.toDate')" :disabled="busy" required
        />
        <button type="submit" class="button button-primary" :disabled="!canSearch">{{ t('betFrequency.run') }}</button>
        <button type="button" class="button button-secondary" :disabled="busy" @click="clear">{{ t('betFrequency.clear') }}</button>
        <label class="report-display-option"><input v-model="showSummary" type="checkbox" />{{ t('platform.reports.showSummary') }}</label>
        <label class="report-display-option"><input v-model="showDetails" type="checkbox" />{{ t('platform.reports.showDetails') }}</label>
      </form>
    </template>
    <template v-if="result && showSummary" #summary>
      <dl class="report-summary-tags">
        <div><dt>{{ t('betFrequency.items') }}</dt><dd><span data-numeric-tone="info">{{ integerFormat.format(BigInt(result.effective_item_count)) }}</span></dd></div>
        <div><dt>{{ t('betFrequency.activePlays') }}</dt><dd><span data-numeric-tone="info">{{ integerFormat.format(BigInt(result.play_count)) }}</span></dd></div>
      </dl>
    </template>
    <div v-if="catalogFailed" class="empty-state" role="alert">
      {{ t('betFrequency.gameLoadFailed') }}
      <button type="button" class="button button-secondary" @click="withGlobalLoading(loadGames)">{{ t('betFrequency.retry') }}</button>
    </div>
    <p v-else-if="!catalogBusy && !games.length" class="empty-state" role="status">{{ t('betFrequency.noGames') }}</p>
    <p v-if="queryError" class="empty-state" role="alert">{{ t(queryError) }}</p>
    <ManagementTable v-if="result && showDetails" :entries="entries" :columns="columns" :empty="!entries.length" :empty-label="t('betFrequency.empty')" table-class="frequency-table">
      <template #play="{ entry }">{{ entry.label[locale] }}</template>
      <template #count="{ entry }"><span data-numeric-tone="info">{{ integerFormat.format(BigInt(entry.effective_item_count)) }}</span></template>
      <template #share="{ entry }"><span data-numeric-tone="muted">{{ share(entry.effective_item_count, result.effective_item_count) }}</span></template>
    </ManagementTable>
    <template v-if="result && showDetails && result.play_count !== '0'" #pagination>
      <TablePagination
        :total="result.play_count" :page="page" :count="entries.length" :locale="locale" :page-size="pageSize"
        :busy="busy" :previous-disabled="busy || page === 1" :next-disabled="busy || BigInt(page * pageSize) >= BigInt(result.play_count)"
        :previous-label="t('platform.copy.7f5aba9a')" :next-label="t('platform.copy.282be575')"
        :go-to="target => load(target)" :change-page-size="size => load(1, size)"
      />
    </template>
  </ManagementList>
</template>

<style scoped>
:deep(.frequency-number) { font-variant-numeric: tabular-nums; }
@media (max-width: 1080px) {
  .management-list :deep(.frequency-table td.frequency-number) { grid-column: auto; }
  .management-list :deep(.frequency-table td.frequency-play) { font-weight: 650; }
  .management-list :deep(.frequency-table td.frequency-play::before) { content: none; }
}
@media (min-width: 1081px) {
  .management-list :deep(.frequency-table th:nth-child(n+2)),
  .management-list :deep(.frequency-table td.frequency-number) { width: 20%; text-align: right; }
}
</style>
