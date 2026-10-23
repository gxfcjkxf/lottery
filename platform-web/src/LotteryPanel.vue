<script setup lang="ts">
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { usePlatformI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { usePlatformI18n } from "./i18n"
const ui = usePlatformI18n().t
import DetailModal from "@lottery/shared/DetailModal.vue"
import NumberModelEditor from '@lottery/shared/NumberModelEditor.vue'
import { computed, ref, watch } from 'vue'
import { PlatformApiError } from './platform-api'
import { createPlatformLotteryApi, type Game, type Period, type DrawHistoryPage, type DrawResult } from './lottery-api'
import RulesPanel from './RulesPanel.vue'
import { formatPlatformDateTime, type PlatformDisplaySettings } from './platform-format'

const props = defineProps<{ brandId: string; selectedGame?: Game; locale: 'en' | 'zh-CN'; mode: 'games' | 'draws'; displaySettings: PlatformDisplaySettings; detailsOnly?: boolean }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformLotteryApi()
const games = ref<Game[]>([])
const periods = ref<Period[]>([])
const game = ref<Game | null>(null)
const period = ref<Period | null>(null)
const draws = ref<DrawHistoryPage | null>(null)
const drawRows = ref<Array<{ period: Period; draw: DrawResult }>>([])
const activeGameDetail = ref<'periods' | 'rules' | 'model' | null>(null)
const gameOffset = ref(0), periodOffset = ref(0), drawOffset = ref(0)
const moreGames = ref(false), morePeriods = ref(false), moreDraws = ref(false)
const loading = ref(false), error = ref<string | import("@lottery/shared").LocalizedMessage>("")
let generation = 0
const copy = computed(() => props.locale === 'en' ? {
  title: ui('platform.copy.a1bff730'), note: ui('platform.copy.971f16cb'), games: ui('platform.copy.f96d2cde'), code: ui('platform.copy.ba1081a4'), name: ui('platform.copy.00d0ada7'), state: ui('platform.copy.274ae047'), model: ui('platform.copy.9d34bb07'), close: ui('platform.copy.27b2fd29'), plays: ui('platform.copy.6934c281'), modelType: ui('platform.copy.8829ddac'), length: ui('platform.copy.54601822'), regularCount: ui('platform.copy.4458cea5'), specialCount: ui('platform.copy.080f6100'), poolSize: ui('platform.copy.a482dcaa'), totalCount: ui('platform.copy.e55d0ac2'), regularValues: ui('platform.copy.8fd809fb'), specialValues: ui('platform.copy.95a37a86'), allowRepeat: ui('platform.copy.4f0cea90'), ordered: ui('platform.copy.49dacc3a'), yes: ui('platform.copy.ab57ddad'), no: ui('platform.copy.502adcd8'), timezone: ui('platform.copy.c67c04be'), periods: ui('platform.copy.73a84523'), period: ui('platform.copy.fa6dd49a'), start: ui('platform.copy.cbc9ef64'), end: ui('platform.copy.497fa54a'), drawAt: ui('platform.copy.eb2cd818'), current: ui('platform.copy.8222b5c4'), history: ui('platform.copy.2913596f'), attempts: ui('platform.copy.8694a1a9'), source: ui('platform.copy.4fe341a5'), result: ui('platform.copy.189e038f'), previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), empty: ui('platform.copy.ef417ef3'), noResult: ui('platform.copy.c3c579cc'), refresh: ui('platform.copy.256fe8b3'),
} : {
  title: ui('platform.copy.a1bff730'), note: ui('platform.copy.971f16cb'), games: ui('platform.copy.f96d2cde'), code: ui('platform.copy.ba1081a4'), name: ui('platform.copy.00d0ada7'), state: ui('platform.copy.274ae047'), model: ui('platform.copy.9d34bb07'), close: ui('platform.copy.27b2fd29'), plays: ui('platform.copy.6934c281'), modelType: ui('platform.copy.8829ddac'), length: ui('platform.copy.54601822'), regularCount: ui('platform.copy.4458cea5'), specialCount: ui('platform.copy.080f6100'), poolSize: ui('platform.copy.a482dcaa'), totalCount: ui('platform.copy.e55d0ac2'), regularValues: ui('platform.copy.8fd809fb'), specialValues: ui('platform.copy.95a37a86'), allowRepeat: ui('platform.copy.4f0cea90'), ordered: ui('platform.copy.49dacc3a'), yes: ui('platform.copy.ab57ddad'), no: ui('platform.copy.502adcd8'), timezone: ui('platform.copy.c67c04be'), periods: ui('platform.copy.73a84523'), period: ui('platform.copy.fa6dd49a'), start: ui('platform.copy.cbc9ef64'), end: ui('platform.copy.497fa54a'), drawAt: ui('platform.copy.eb2cd818'), current: ui('platform.copy.8222b5c4'), history: ui('platform.copy.2913596f'), attempts: ui('platform.copy.8694a1a9'), source: ui('platform.copy.4fe341a5'), result: ui('platform.copy.189e038f'), previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), empty: ui('platform.copy.ef417ef3'), noResult: ui('platform.copy.c3c579cc'), refresh: ui('platform.copy.256fe8b3'),
})
const json = (value: unknown) => JSON.stringify(value, null, 2)
function statusLabel(status: string) {
  const labels: Record<string, [string, string]> = {
    active: [ui('platform.copy.15a92cfa'), ui('platform.copy.15a92cfa')], paused: [ui('platform.copy.3d294087'), ui('platform.copy.3d294087')], disabled: [ui('platform.copy.1b47d863'), ui('platform.copy.1b47d863')],
    pending: [ui('platform.copy.7174c9b9'), ui('platform.copy.7174c9b9')], betting: [ui('platform.copy.4cd5b8f0'), ui('platform.copy.4cd5b8f0')], closed: [ui('platform.copy.ef215d5f'), ui('platform.copy.ef215d5f')],
    waiting_draw: [ui('platform.copy.b1ade952'), ui('platform.copy.b1ade952')], drawn: [ui('platform.copy.d6cddd7b'), ui('platform.copy.d6cddd7b')], settled: [ui('platform.copy.f9680456'), ui('platform.copy.f9680456')],
  }
  const pair = labels[status]
  return pair ? pair[props.locale === 'en' ? 0 : 1] : status
}
function report(cause: unknown) {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  else error.value = cause instanceof Error ? publicErrorMessage(cause) : publicErrorMessage(cause)
}
async function loadGames(offset = 0) {
  const request = ++generation, brandId = props.brandId
  games.value = []; periods.value = []; drawRows.value = []; game.value = null; period.value = null; draws.value = null
  activeGameDetail.value = null
  moreGames.value = false; morePeriods.value = false; moreDraws.value = false; error.value = ''
  if (!brandId || props.mode === 'draws' || props.detailsOnly) { loading.value = false; return }
  loading.value = true
  try {
    const rows = await api.games(brandId, 51, offset)
    if (request === generation && brandId === props.brandId) {
      games.value = rows.slice(0, 50); moreGames.value = rows.length > 50; gameOffset.value = offset
    }
  } catch (cause) { if (request === generation) report(cause) }
  finally { if (request === generation) loading.value = false }
}
async function loadPeriods(selected: Game, offset = 0) {
  const request = ++generation, brandId = props.brandId
  game.value = selected; periods.value = []; period.value = null; draws.value = null; drawRows.value = []; morePeriods.value = false; moreDraws.value = false; error.value = ''; loading.value = true
  activeGameDetail.value = 'periods'
  try {
    const rows = await api.periods(brandId, selected.id, 51, offset)
    if (request === generation && brandId === props.brandId) {
      periods.value = rows.slice(0, 50); morePeriods.value = rows.length > 50; periodOffset.value = offset
      if (props.mode === 'draws') {
        const rowsWithDraws = await Promise.all(periods.value.map(async item => {
          const history = await api.draw(brandId, item.id, 1, 0)
          return history.current ? { period: item, draw: history.current } : null
        }))
        if (request === generation && brandId === props.brandId) drawRows.value = rowsWithDraws.filter((item): item is { period: Period; draw: DrawResult } => item !== null).sort((a, b) => b.period.sequence - a.period.sequence)
      }
    }
  } catch (cause) { if (request === generation) report(cause) }
  finally { if (request === generation) loading.value = false }
}
async function loadDraw(selected: Period, offset = 0) {
  const request = ++generation, brandId = props.brandId
  period.value = selected; draws.value = null; moreDraws.value = false; error.value = ''; loading.value = true
  if (props.mode === 'games') activeGameDetail.value = null
  try {
    const page = await api.draw(brandId, selected.id, 51, offset)
    if (request === generation && brandId === props.brandId) { draws.value = page; drawOffset.value = offset; moreDraws.value = page.history.length > 50 || page.attempts.length > 50 }
  } catch (cause) { if (request === generation) report(cause) }
  finally { if (request === generation) loading.value = false }
}
async function openGameDetail(selected: Game, detail: 'periods' | 'rules' | 'model') {
  if (detail === 'periods') { await loadPeriods(selected); return }
  game.value = selected; period.value = null; draws.value = null; activeGameDetail.value = detail
}
function closeGameDetail() {
  game.value = null; period.value = null; draws.value = null; activeGameDetail.value = null
}
watch(() => [props.brandId, props.mode, props.selectedGame] as const, () => {
  gameOffset.value = 0; periodOffset.value = 0; drawOffset.value = 0
  if (props.mode === 'draws' && props.selectedGame) void withGlobalLoading(loadPeriods, props.selectedGame)
  else void withGlobalLoading(loadGames)
}, { immediate: true, flush: 'sync' })
defineExpose({ openGameDetail })
</script>

<template>
  <div data-testid="platform-lottery">
    <p v-if="error" class="message error" role="alert">{{ displayPublicError(error) }}</p>
    <section v-if="props.mode === 'games' && !detailsOnly" class="panel"><div class="panel-heading"><h2>{{ copy.games }}</h2></div><div class="table-wrap"><table><thead><tr><th>{{ copy.name }}</th><th><BusinessDetails :value="copy.state" :locale="props.locale" /></th><th>{{ copy.timezone }}</th><th>{{ copy.periods }}</th><th>{{ copy.plays }}</th><th><BusinessDetails :value="copy.model" :locale="props.locale" /></th></tr></thead><tbody>
      <tr v-for="row in games" :key="row.id"><td>{{ row.name }}</td><td>{{ statusLabel(row.status) }}</td><td>{{ row.timezone }}</td><td><button class="row-action" type="button" @click="withGlobalLoading(openGameDetail, row, 'periods')">{{ copy.periods }}</button></td><td><button class="row-action" type="button" @click="withGlobalLoading(openGameDetail, row, 'rules')">{{ copy.plays }}</button></td><td><button class="row-action" type="button" @click="withGlobalLoading(openGameDetail, row, 'model')"><BusinessDetails :value="copy.model" :locale="props.locale" /></button></td></tr><tr v-if="!games.length"><td colspan="6" class="empty-state">{{ copy.empty }}</td></tr>
    </tbody></table></div><div class="wallet-pagination" data-testid="platform-game-pages"><button class="button button-secondary" :disabled="loading || gameOffset === 0" @click="withGlobalLoading(loadGames, gameOffset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(gameOffset / 50) + 1 }}</span><button class="button button-secondary" :disabled="loading || !moreGames" @click="withGlobalLoading(loadGames, gameOffset + 50)">{{ copy.next }}</button><button v-refresh-button="copy.refresh" class="button button-secondary" :disabled="loading || !brandId" @click="withGlobalLoading(loadGames)">{{ copy.refresh }}</button></div></section>
    <section v-else-if="props.mode === 'draws' && game" class="panel" data-testid="platform-draw-lookup"><div class="table-wrap"><table><thead><tr><th>{{ copy.period }}</th><th>{{ copy.result }}</th><th>{{ copy.drawAt }}</th><th>{{ copy.source }}</th></tr></thead><tbody>
      <tr v-for="row in drawRows" :key="row.draw.id"><td><button class="row-action" @click="withGlobalLoading(loadDraw, row.period)">{{ row.period.period_no }}</button></td><td><code><BusinessDetails :value="row.draw.result" :locale="props.locale" /></code></td><td>{{ formatPlatformDateTime(row.draw.drawn_at, props.displaySettings, props.locale) }}</td><td>{{ row.draw.kind }}</td></tr><tr v-if="!drawRows.length"><td colspan="3" class="empty-state">{{ copy.noResult }}</td></tr>
    </tbody></table></div><div class="wallet-pagination" v-if="game"><button class="button button-secondary" :disabled="loading || periodOffset === 0" @click="withGlobalLoading(loadPeriods, game, periodOffset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(periodOffset / 50) + 1 }}</span><button class="button button-secondary" :disabled="loading || !morePeriods" @click="withGlobalLoading(loadPeriods, game, periodOffset + 50)">{{ copy.next }}</button></div></section>
    <DetailModal v-if="props.mode === 'games' && game && activeGameDetail === 'periods'" :title="`${copy.periods} · ${game.name}`" :close-label="copy.close" @close="closeGameDetail">
      <section class="detail-section" data-testid="platform-game-periods">
        <div class="table-wrap"><table><thead><tr><th>{{ copy.period }}</th><th><BusinessDetails :value="copy.state" :locale="props.locale" /></th><th>{{ copy.start }}</th><th>{{ copy.end }}</th><th>{{ copy.drawAt }}</th></tr></thead><tbody>
          <tr v-for="row in periods" :key="row.id"><td><button class="row-action" @click="withGlobalLoading(loadDraw, row)">{{ row.period_no }}</button></td><td>{{ statusLabel(row.status) }}</td><td>{{ formatPlatformDateTime(row.bet_start_at, props.displaySettings, props.locale) }}</td><td>{{ formatPlatformDateTime(row.bet_end_at, props.displaySettings, props.locale) }}</td><td>{{ formatPlatformDateTime(row.draw_at, props.displaySettings, props.locale) }}</td></tr><tr v-if="!periods.length"><td colspan="5" class="empty-state">{{ copy.empty }}</td></tr>
        </tbody></table></div><div class="wallet-pagination" data-testid="platform-period-pages"><button class="button button-secondary" :disabled="loading || periodOffset === 0" @click="withGlobalLoading(loadPeriods, game, periodOffset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(periodOffset / 50) + 1 }}</span><button class="button button-secondary" :disabled="loading || !morePeriods" @click="withGlobalLoading(loadPeriods, game, periodOffset + 50)">{{ copy.next }}</button></div>
      </section>
    </DetailModal>
    <DetailModal v-if="props.mode === 'games' && game && activeGameDetail === 'rules'" :title="`${copy.plays} · ${game.name}`" :close-label="copy.close" @close="closeGameDetail">
      <section class="detail-section" data-testid="platform-game-periods"><RulesPanel :brand-id="brandId" :game-id="game.id" :locale="locale" :display-settings="displaySettings" @failure="emit('failure', $event)" /></section>
    </DetailModal>
    <DetailModal v-if="props.mode === 'games' && game && activeGameDetail === 'model'" :title="`${copy.model} · ${game.name}`" :close-label="copy.close" @close="closeGameDetail">
      <section class="detail-section" data-testid="platform-game-model"><NumberModelEditor :model-value="game.model" :locale="locale" disabled /></section>
    </DetailModal>
    <DetailModal v-if="period" :title="`${copy.period} · ${period.period_no}`" :close-label="copy.close" @close="period = null; draws = null; activeGameDetail = null">
      <section class="detail-section" data-testid="platform-period-draw"><div class="panel-heading"><h2>{{ copy.current }}</h2></div>
        <template v-if="draws"><div class="bet-snapshots"><BusinessDetails v-if="draws.current" :value="draws.current.result" :locale="props.locale" /><p v-else class="empty-state">{{ copy.noResult }}</p></div><div class="table-wrap"><table><thead><tr><th>{{ copy.drawAt }}</th><th>{{ copy.source }}</th><th>{{ copy.result }}</th></tr></thead><tbody><tr v-for="row in draws.history.slice(0, 50)" :key="row.id"><td>{{ formatPlatformDateTime(row.drawn_at, props.displaySettings, props.locale) }}</td><td>{{ row.kind }}</td><td><BusinessDetails :value="row.result" :locale="props.locale" /></td></tr></tbody></table></div><div class="bet-snapshots"><h3>{{ copy.attempts }}</h3><BusinessDetails :value="draws.attempts.slice(0, 50)" :locale="props.locale" /></div><div class="wallet-pagination" data-testid="platform-draw-pages"><button class="button button-secondary" :disabled="loading || drawOffset === 0" @click="withGlobalLoading(loadDraw, period, drawOffset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(drawOffset / 50) + 1 }}</span><button class="button button-secondary" :disabled="loading || !moreDraws" @click="withGlobalLoading(loadDraw, period, drawOffset + 50)">{{ copy.next }}</button></div></template>
      </section>
    </DetailModal>
  </div>
</template>
