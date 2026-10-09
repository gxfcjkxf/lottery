<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { PlatformApiError } from './platform-api'
import { createPlatformLotteryApi, type Game, type Period, type DrawHistoryPage } from './lottery-api'
import RulesPanel from './RulesPanel.vue'

const props = defineProps<{ brandId: string; locale: 'en' | 'zh-CN' }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformLotteryApi()
const games = ref<Game[]>([])
const periods = ref<Period[]>([])
const game = ref<Game | null>(null)
const period = ref<Period | null>(null)
const draws = ref<DrawHistoryPage | null>(null)
const showRules = ref(false)
const gameOffset = ref(0), periodOffset = ref(0), drawOffset = ref(0)
const moreGames = ref(false), morePeriods = ref(false), moreDraws = ref(false)
const loading = ref(false), error = ref('')
let generation = 0
const copy = computed(() => props.locale === 'en' ? {
  title: 'Games and draws', note: 'Read-only catalogue, periods and published results', games: 'Games', code: 'Code', name: 'Name', state: 'State', model: 'Number model', timezone: 'Timezone', periods: 'Periods', period: 'Period', start: 'Bet start', end: 'Bet cutoff', drawAt: 'Draw time', current: 'Current result', history: 'Result history', attempts: 'Source attempts', source: 'Source', result: 'Numbers', previous: 'Previous page', next: 'Next page', empty: 'Nothing to show.', noResult: 'No published result.', loading: 'Loading…', refresh: 'Refresh games',
} : {
  title: '彩种与开奖', note: '只读彩种、期次及已发布开奖结果', games: '彩种', code: '代码', name: '名称', state: '状态', model: '号码模型', timezone: '时区', periods: '期次', period: '期号', start: '投注开始', end: '投注截止', drawAt: '开奖时间', current: '当前开奖结果', history: '结果历史', attempts: '来源尝试', source: '来源', result: '号码', previous: '上一页', next: '下一页', empty: '暂无记录。', noResult: '暂无已发布结果。', loading: '加载中…', refresh: '刷新彩种',
})
const json = (value: unknown) => JSON.stringify(value, null, 2)
function report(cause: unknown) {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  else error.value = cause instanceof Error ? cause.message : String(cause)
}
async function loadGames(offset = 0) {
  const request = ++generation, brandId = props.brandId
  games.value = []; periods.value = []; game.value = null; period.value = null; draws.value = null
  showRules.value = false
  moreGames.value = false; morePeriods.value = false; moreDraws.value = false; error.value = ''
  if (!brandId) { loading.value = false; return }
  loading.value = true
  try {
    const rows = await api.games(brandId, 51, offset)
    if (request === generation && brandId === props.brandId) { games.value = rows.slice(0, 50); moreGames.value = rows.length > 50; gameOffset.value = offset }
  } catch (cause) { if (request === generation) report(cause) }
  finally { if (request === generation) loading.value = false }
}
async function loadPeriods(selected: Game, offset = 0) {
  const request = ++generation, brandId = props.brandId
  game.value = selected; periods.value = []; period.value = null; draws.value = null; morePeriods.value = false; moreDraws.value = false; error.value = ''; loading.value = true
  showRules.value = false
  try {
    const rows = await api.periods(brandId, selected.id, 51, offset)
    if (request === generation && brandId === props.brandId) { periods.value = rows.slice(0, 50); morePeriods.value = rows.length > 50; periodOffset.value = offset }
  } catch (cause) { if (request === generation) report(cause) }
  finally { if (request === generation) loading.value = false }
}
async function loadDraw(selected: Period, offset = 0) {
  const request = ++generation, brandId = props.brandId
  period.value = selected; draws.value = null; moreDraws.value = false; error.value = ''; loading.value = true
  try {
    const page = await api.draw(brandId, selected.id, 51, offset)
    if (request === generation && brandId === props.brandId) { draws.value = page; drawOffset.value = offset; moreDraws.value = page.history.length > 50 || page.attempts.length > 50 }
  } catch (cause) { if (request === generation) report(cause) }
  finally { if (request === generation) loading.value = false }
}
watch(() => props.brandId, () => { gameOffset.value = 0; periodOffset.value = 0; drawOffset.value = 0; void loadGames() }, { immediate: true, flush: 'sync' })
</script>

<template>
  <div data-testid="platform-lottery">
    <div class="page-heading"><div><div class="eyebrow">{{ copy.note }}</div><h1>{{ copy.title }}</h1></div><button class="secondary" :disabled="loading || !brandId" @click="loadGames()">{{ copy.refresh }}</button></div>
    <p v-if="error" class="message error" role="alert">{{ error }}</p><p v-if="loading" class="loading-line">{{ copy.loading }}</p>
    <section class="panel"><div class="panel-heading"><h2>{{ copy.games }}</h2></div><div class="table-wrap"><table><thead><tr><th>{{ copy.name }}</th><th>{{ copy.code }}</th><th>{{ copy.state }}</th><th>{{ copy.timezone }}</th></tr></thead><tbody>
      <tr v-for="row in games" :key="row.id"><td><button class="row-action" @click="loadPeriods(row)">{{ row.name }}</button><small>{{ row.id }}</small></td><td>{{ row.code }}</td><td>{{ row.status }}</td><td>{{ row.timezone }}</td></tr><tr v-if="!games.length"><td colspan="4" class="empty-state">{{ copy.empty }}</td></tr>
    </tbody></table></div><div class="wallet-pagination" data-testid="platform-game-pages"><button class="secondary" :disabled="loading || gameOffset === 0" @click="loadGames(gameOffset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(gameOffset / 50) + 1 }}</span><button class="secondary" :disabled="loading || !moreGames" @click="loadGames(gameOffset + 50)">{{ copy.next }}</button></div></section>
    <section v-if="game" class="panel reward-detail" data-testid="platform-game-periods"><div class="panel-heading"><h2>{{ game.name }} / {{ showRules ? locale === 'en' ? 'Plays and rules' : '玩法与规则' : copy.periods }}</h2><button class="secondary" @click="showRules = !showRules">{{ showRules ? copy.periods : locale === 'en' ? 'Plays and rules' : '玩法与规则' }}</button></div>
      <RulesPanel v-if="showRules" :brand-id="brandId" :game-id="game.id" :locale="locale" @failure="emit('failure', $event)" />
      <template v-else>
      <details class="bet-snapshots"><summary>{{ copy.model }}</summary><pre>{{ json(game.model) }}</pre></details>
      <div class="table-wrap"><table><thead><tr><th>{{ copy.period }}</th><th>{{ copy.state }}</th><th>{{ copy.start }}</th><th>{{ copy.end }}</th><th>{{ copy.drawAt }}</th></tr></thead><tbody>
        <tr v-for="row in periods" :key="row.id"><td><button class="row-action" @click="loadDraw(row)">{{ row.period_no }}</button></td><td>{{ row.status }}</td><td>{{ row.bet_start_at }}</td><td>{{ row.bet_end_at }}</td><td>{{ row.draw_at }}</td></tr><tr v-if="!periods.length"><td colspan="5" class="empty-state">{{ copy.empty }}</td></tr>
      </tbody></table></div><div class="wallet-pagination" data-testid="platform-period-pages"><button class="secondary" :disabled="loading || periodOffset === 0" @click="loadPeriods(game, periodOffset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(periodOffset / 50) + 1 }}</span><button class="secondary" :disabled="loading || !morePeriods" @click="loadPeriods(game, periodOffset + 50)">{{ copy.next }}</button></div>
      </template>
    </section>
    <section v-if="!showRules && period && draws" class="panel reward-detail" data-testid="platform-period-draw"><div class="panel-heading"><h2>{{ period.period_no }} / {{ copy.current }}</h2></div>
      <div class="bet-snapshots"><pre v-if="draws.current">{{ json(draws.current.result) }}</pre><p v-else>{{ copy.noResult }}</p></div>
      <div class="table-wrap"><table><thead><tr><th>{{ copy.history }}</th><th>{{ copy.source }}</th><th>{{ copy.result }}</th><th>{{ copy.drawAt }}</th></tr></thead><tbody><tr v-for="row in draws.history.slice(0, 50)" :key="row.id"><td>{{ row.id }}</td><td>{{ row.kind }}<small>{{ row.source_id }}</small></td><td><pre>{{ json(row.result) }}</pre></td><td>{{ row.drawn_at }}</td></tr></tbody></table></div>
      <div class="bet-snapshots"><h3>{{ copy.attempts }}</h3><pre>{{ json(draws.attempts.slice(0, 50)) }}</pre></div>
      <div class="wallet-pagination" data-testid="platform-draw-pages"><button class="secondary" :disabled="loading || drawOffset === 0" @click="loadDraw(period, drawOffset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(drawOffset / 50) + 1 }}</span><button class="secondary" :disabled="loading || !moreDraws" @click="loadDraw(period, drawOffset + 50)">{{ copy.next }}</button></div>
    </section>
  </div>
</template>
