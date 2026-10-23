<script setup lang="ts">
import BusinessDetails from './BusinessDetails.vue'
import { vWriteGuard } from "@lottery/shared";
import { withGlobalLoading } from './global-loading';
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading } from './global-loading'
import { formatDisplayDateTime } from './display-settings'
import { vInlineValidation } from './form-validation'
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import DetailModal from './DetailModal.vue'
import NumberModelEditor from './NumberModelEditor.vue'
import PlatformDateInput from './PlatformDateInput.vue'
import PlatformTimeInput from './PlatformTimeInput.vue'
import PlatformPlayRuleForm from './PlatformPlayRuleForm.vue'
import ConfiguredRuleWorkbench from './ConfiguredRuleWorkbench.vue'
import AutomationSettings from './AutomationSettings.vue'
import { lotteryLocalTimeToISO } from './lottery-time'
import type { PlatformDisplaySettings } from './date-time'
import { createCatalogI18n } from './localization'
import { createPlatformLotteryClient, platformLotteryCatalog, PlatformLotteryError, type PlatformGame, type PlatformGameDetail, type PlatformPeriod } from './platform-lottery'
import type { RuleModel } from './rules'
import { buildPlayDefinition, playRuleDraft, type PlayRuleDraft } from './platform-play-form'

const props = defineProps<{ surface: 'platform' | 'admin' | 'user'; brandId: string; brandCode?: string; initialGameId?: string; locale: 'en' | 'zh-CN'; canManage: boolean; canConnect: boolean; canSell?: boolean; unifiedCatalog?: boolean; brandGames?: Array<{ id: string; name: string; code: string; timezone: string }>; brandName?: string; brandLoading?: boolean; catalogBrandId?: string; displaySettings?: PlatformDisplaySettings }>()
if (props.surface === 'platform' && !props.displaySettings) throw new Error('Platform lottery requires platform display settings')
const emit = defineEmits<{ failure: [cause: unknown]; brandDetail: [id: string, view: 'periods' | 'plays' | 'model']; refreshBrand: [] }>()
const i18n = createCatalogI18n(platformLotteryCatalog, props.locale)
watch(() => props.locale, value => i18n.setLocale(value))
const t = i18n.t
const timezones = ['UTC', 'Asia/Shanghai', 'Asia/Singapore', 'Asia/Tokyo', 'Asia/Seoul', 'Asia/Kolkata', 'Asia/Dubai', 'Asia/Jakarta', 'Asia/Bangkok', 'Asia/Hong_Kong', 'Asia/Taipei', 'Europe/London', 'Europe/Berlin', 'Europe/Paris', 'Europe/Moscow', 'America/New_York', 'America/Chicago', 'America/Denver', 'America/Los_Angeles', 'America/Toronto', 'America/Sao_Paulo', 'Australia/Sydney', 'Pacific/Auckland']
const api = createPlatformLotteryClient(props.surface, () => props.brandId, fetch, props.brandCode)
const lifecycleFilter = ref('active')
const games = ref<PlatformGame[]>([]), detail = ref<PlatformGameDetail | null>(null)
const selectedId = ref(''), offset = ref(0), periodOffset = ref(0), loading = ref(false), error = ref(''), notice = ref(false)
const drawTab = ref<'sources' | 'manual'>('sources')
const catalogColumnCount = computed(() => 4 + (props.unifiedCatalog ? 1 : 0) + (props.surface === 'admin' ? 1 : 0) + (props.surface !== 'user' ? 3 : 0))
const view = ref<'periods' | 'plays' | 'model' | 'automation'>('periods')
const detailPage = computed(() => props.surface !== 'user' && !!selectedId.value && (view.value === 'plays' || view.value === 'automation'))
const detailTitle = computed(() => `${t(view.value === 'automation' ? 'drawConfiguration' : view.value)} · ${detail.value?.game.name ?? ''}`)
let catalogScroll = 0
const catalogRows = computed(() => [
  ...games.value.map(game => ({ source: 'platform' as const, game })),
  ...(props.unifiedCatalog && props.catalogBrandId ? (props.brandGames ?? []).map(game => ({ source: 'brand' as const, game })) : []),
])
const filteredRows = computed(() => catalogRows.value.filter(row => lifecycleFilter.value === 'all' || (row.source === 'brand' ? lifecycleFilter.value === 'active' : row.game.status === lifecycleFilter.value)))
const visibleRows = computed(() => props.unifiedCatalog ? filteredRows.value.slice(offset.value, offset.value + 50) : filteredRows.value)
const hasNext = computed(() => props.unifiedCatalog ? offset.value + 50 < filteredRows.value.length : games.value.length === 50)
async function goPage(page: number) { if (props.unifiedCatalog) offset.value = page; else await load(page) }
async function refresh() { emit('refreshBrand'); await load() }
async function openRow(row: typeof catalogRows.value[number], next: typeof view.value) {
  if (row.source === 'brand') { if (next !== 'automation') emit('brandDetail', row.game.id, next) }
  else await open(row.game, next)
}
const formKind = ref<'name' | 'game' | 'connect' | 'period' | 'play' | 'result' | 'model' | 'lifecycle' | 'availability' | null>(null)
const target = ref<PlatformGame | PlatformPeriod | null>(null)
const form = ref({ name: '', code: '', timezone: 'Asia/Singapore', reason: '', periodNo: '', startDate: '', startTime: '', endDate: '', endTime: '', drawDate: '', drawTime: '', regular: '', special: '', digits: '' })
const pending = ref<{ path: string; body: unknown; key: string } | null>(null)
const defaultModel = (): RuleModel => ({ model: 'DIGITS_0_9', regular_pool: { allow_repeat: false }, special_pool: { allow_repeat: false }, regular_count: 0, special_count: 0, pool_size: 0, total_count: 0, length: 3, allow_repeat: true, ordered: true })
const modelDraft = ref<RuleModel>(defaultModel())
const playDraft = ref<PlayRuleDraft | null>(null)
const isMarkSix = (model: RuleModel) => model.model === 'X_PLUS_Y' && model.regular_count === 6 && model.special_count === 1 && model.regular_pool.min === 1 && model.regular_pool.max === 49 && model.special_pool.min === 1 && model.special_pool.max === 49
const resultModel = computed(() => detail.value?.game.model)
const resultHasRegular = computed(() => !!resultModel.value && ['X_PLUS_Y', 'M_SELECT_N'].includes(resultModel.value.model) && resultModel.value.regular_count > 0)
const resultHasSpecial = computed(() => !!resultModel.value && ['X_PLUS_Y', 'M_SELECT_N'].includes(resultModel.value.model) && resultModel.value.special_count > 0)
const resultHasDigits = computed(() => resultModel.value?.model === 'DIGITS_0_9' && resultModel.value.length > 0)
const modelLocked = computed(() => !!detail.value && (detail.value.mark_six_configured || detail.value.plays.length > 0 || detail.value.periods.length > 0))
const lifecycleAction = ref<'active' | 'offline' | 'deleted'>('offline')
const availabilityEnabled = ref(false)
const isConfirmation = computed(() => formKind.value === 'lifecycle' || formKind.value === 'availability')
function changeLifecycle(game: PlatformGame, status: 'active' | 'offline' | 'deleted') { lifecycleAction.value=status; begin('lifecycle',game) }
function changeAvailability(game: PlatformGame) { availabilityEnabled.value=!game.brand_enabled; begin('availability',game) }
const actionLabel = computed(() => formKind.value === 'availability' ? t(availabilityEnabled.value?'enableGame':'disableGame') : t(lifecycleAction.value==='deleted'?'deleteGame':lifecycleAction.value==='offline'?'takeOffline':'relist'))
const actionHint = computed(() => formKind.value==='availability' ? t(availabilityEnabled.value?'enableHint':'brandDisableHint') : lifecycleAction.value==='deleted' ? '' : t(lifecycleAction.value==='offline'?'offlineHint':'enableHint'))
let generation = 0
let alive = true
const dialogTitle = computed(() => isConfirmation.value ? actionLabel.value : formKind.value ? t(({ name: 'editName', game: 'create', connect: 'connect', period: 'createPeriod', play: 'createPlay', result: 'publish', model: 'editModel', lifecycle: 'lifecycleConfirm', availability: 'lifecycleConfirm' } as const)[formKind.value]) : '')
const shownError = computed(() => error.value ? (error.value in platformLotteryCatalog.en ? t(error.value) : i18n.errorText({ code: error.value.replace(/^error\./, '') })) : '')
function report(cause: unknown) {
  if (cause instanceof PlatformLotteryError) { error.value = `error.${cause.code}`; if (cause.status === 401) emit('failure', cause) }
  else error.value = 'invalid'
}
async function load(page = 0) {
  const request = ++generation
  loading.value = true; error.value = ''; games.value = []; detail.value = null; selectedId.value = ''; offset.value = page
  try {
    const rows: PlatformGame[] = []
    for (let next = props.unifiedCatalog ? 0 : page; ; next += 50) {
      const result = await api.list(next)
      if (!alive || request !== generation) return
      rows.push(...result.games)
      if (!props.unifiedCatalog || result.games.length < 50) break
    }
    games.value = rows
  }
  catch (cause) { if (alive && request === generation) report(cause) }
  finally { if (alive && request === generation) loading.value = false }
}
async function open(game: Pick<PlatformGame, 'id'>, next: typeof view.value, page = 0) {
  if (!selectedId.value && props.surface !== 'user' && (next === 'plays' || next === 'automation')) catalogScroll = window.scrollY
  const request = ++generation
  if (next === 'automation' && (selectedId.value !== game.id || view.value !== 'automation')) drawTab.value = 'sources'
  selectedId.value = game.id; view.value = next; periodOffset.value = page; detail.value = null; loading.value = true; error.value = ''
  if (detailPage.value) { await nextTick(); window.scrollTo(0, 0) }
  try { const record = await api.detail(game.id, page); if (alive && request === generation) detail.value = record }
  catch (cause) { if (alive && request === generation) report(cause) }
  finally { if (alive && request === generation) loading.value = false }
}
async function closeDetail() {
  const wasPage = detailPage.value
  generation++; detail.value = null; selectedId.value = ''; loading.value = false
  if (wasPage) { await nextTick(); window.scrollTo(0, catalogScroll) }
}
function begin(kind: NonNullable<typeof formKind.value>, row?: PlatformGame | PlatformPeriod) {
  if (kind === 'name') form.value.name = (row as PlatformGame).name
  if (kind === 'result') { form.value.regular = ''; form.value.special = ''; form.value.digits = '' }
  if (kind === 'play') { form.value.name = ''; form.value.code = ''; playDraft.value = playRuleDraft(detail.value!.game.model) }
  if (kind === 'game') { modelDraft.value = defaultModel(); form.value.name = ''; form.value.code = '' }
  if (kind === 'model') modelDraft.value = JSON.parse(JSON.stringify(detail.value!.game.model))
  if (kind === 'period') {
    const previous = detail.value?.periods[0]?.period_no ?? ''
    form.value.periodNo = /^\d+$/.test(previous) ? String(BigInt(previous) + 1n).padStart(previous.length, '0') : ''
    form.value.startDate = ''; form.value.startTime = ''
    form.value.endDate = ''; form.value.endTime = ''
    form.value.drawDate = ''; form.value.drawTime = ''
  }
  formKind.value = kind; target.value = row ?? null; form.value.reason = ''; error.value = ''; notice.value = false
}
function numbers(value: string) { if (!value.trim()) return []; const values = value.split(',').map(s => Number(s.trim())); if (values.some(n => !Number.isSafeInteger(n))) throw new Error(); return values }
function formatTime(value: string) {
  return formatDisplayDateTime(value, props.locale)
}
function inputTime(date: string, time: string) {
  if (props.surface !== 'platform' || !props.displaySettings) throw new Error('Only platform operators can create periods')
  if (!date || !time) throw new Error('Date and time are required')
  return lotteryLocalTimeToISO(`${date}T${time}`, props.displaySettings.timezone)
}
async function submit() {
  if (loading.value) return
  error.value = ''
  if (!pending.value) {
    const f = form.value
    try {
      let path = '', body: unknown
      switch (formKind.value) {
        case 'lifecycle': { const game=target.value as PlatformGame; path=`/${game.id}/lifecycle`; body={expected_version:game.version,status:lifecycleAction.value,reason:f.reason}; break }
        case 'availability': { const game=target.value as PlatformGame; path=`/${game.id}/availability`; body={expected_version:game.brand_version,enabled:availabilityEnabled.value,reason:f.reason}; break }
        case 'name': { const game = target.value as PlatformGame; path = `/${game.id}/name`; body = { name: f.name, expected_version: game.version }; break }
        case 'game': body = { code: f.code, name: f.name, model: modelDraft.value, timezone: f.timezone }; break
        case 'model': path = `/${selectedId.value}/model`; body = { model: modelDraft.value, expected_version: detail.value!.game.version }; break
        case 'connect': path = `/${target.value!.id}/connect`; body = {}; break
        case 'period': path = `/${selectedId.value}/periods`; body = { period_no: f.periodNo, bet_start_at: inputTime(f.startDate, f.startTime), bet_end_at: inputTime(f.endDate, f.endTime), draw_at: inputTime(f.drawDate, f.drawTime) }; break
        case 'play': path = `/${selectedId.value}/plays`; body = { code: f.code, name: f.name, definition: buildPlayDefinition(detail.value!.game.model, playDraft.value!) }; break
        case 'result': path = `/periods/${target.value!.id}/result`; body = { result: { regular: resultHasRegular.value ? numbers(f.regular) : [], special: resultHasSpecial.value ? numbers(f.special) : [], digits: resultHasDigits.value ? numbers(f.digits) : [] }, reason: f.reason }; break
        default: throw new Error()
      }
      pending.value = { path, body, key: crypto.randomUUID() }
    } catch { error.value = formKind.value === 'play' ? 'playFormInvalid' : 'invalid'; return }
  }
  loading.value = true
  const request = generation, snapshot = pending.value, current = detail.value?.game, nextView = view.value
  try {
    await api.write(snapshot.path, snapshot.body, snapshot.key)
    if (!alive || request !== generation) return
    pending.value = null; formKind.value = null; notice.value = true
    if (snapshot.path.endsWith('/name') || snapshot.path.endsWith('/lifecycle') || snapshot.path.endsWith('/availability')) await load(offset.value)
    else if (current) await open(current, nextView, periodOffset.value); else await load(offset.value)
  } catch (cause) {
    if (!alive || request !== generation) return
    if (cause instanceof PlatformLotteryError && cause.status >= 400 && cause.status < 500) pending.value = null
    report(cause)
  } finally { if (alive && request === generation) loading.value = false }
}
watch(() => [props.brandId, props.surface, props.initialGameId], () => {
  pending.value = null; formKind.value = null
  if (props.surface === 'user' && props.initialGameId) void open({ id: props.initialGameId }, 'periods')
  else void load()
}, { immediate: true })
watch(() => props.catalogBrandId, () => { offset.value = 0 })
const resultPoll=setInterval(async()=>{if(props.surface==='user'&&detail.value&&view.value==='periods'&&!loading.value){const current=generation,id=detail.value.game.id;try{const value=await api.detail(id,periodOffset.value);if(alive&&current===generation&&selectedId.value===id){detail.value=value;error.value=''}}catch(e){if(alive&&current===generation)report(e)}}},5000)
onUnmounted(() => { alive = false; generation++;clearInterval(resultPoll) })
</script>

<template>
  <section class="panel platform-lottery" data-testid="platform-shared-games">
    <div v-show="!detailPage" data-testid="lottery-catalog-page">
    <slot name="catalog-header" />
    <div class="platform-lottery-toolbar">
      <h2 v-if="!unifiedCatalog">{{ t('title') }}</h2>
      <label v-if="surface !== 'user'" class="platform-lottery-filter">{{ t('lifecycleFilter') }}<select :value="lifecycleFilter" @change="lifecycleFilter=($event.target as HTMLSelectElement).value; offset=0"><option value="active">{{ t('active') }}</option><option value="offline">{{ t('offline') }}</option><option value="all">{{ t('allStates') }}</option></select></label>
      <button v-if="canManage" class="button button-primary" :disabled="loading" @click="begin('game')">{{ t('create') }}</button>
    </div>
    <p v-if="!unifiedCatalog" class="platform-lottery-note">{{ t('readonly') }}</p>
    <p v-if="shownError" class="message error" role="alert">{{ shownError }}</p><p v-if="notice" class="message" role="status">{{ t('success') }}</p>
    <div class="table-wrap"><table class="platform-lottery-catalog-table"><thead><tr><th>{{ t('name') }}</th><th v-if="unifiedCatalog">{{ t('ownership') }}</th><th>{{ t('periods') }}</th><th>{{ t('plays') }}</th><th v-if="surface !== 'user'">{{ t('drawConfiguration') }}</th><th>{{ t('model') }}</th><th v-if="surface === 'admin'">{{ t('connect') }}</th><th v-if="surface !== 'user'">{{ t('status') }}</th><th v-if="surface !== 'user'">{{ t('actions') }}</th></tr></thead><tbody>
      <tr v-for="row in visibleRows" :key="`${row.source}:${row.game.id}`"><td>{{ row.game.name }}</td><td v-if="unifiedCatalog">{{ row.source === 'platform' ? t('platformOwner') : brandName }}</td><td><button class="row-action text-link" :disabled="surface === 'admin' && row.source === 'platform' && !row.game.connected" @click="withGlobalLoading(openRow, row, 'periods')">{{ t('periods') }}</button></td><td><button class="row-action text-link" :disabled="surface === 'admin' && row.source === 'platform' && !row.game.connected" @click="withGlobalLoading(openRow, row, 'plays')">{{ t('plays') }}</button></td><td v-if="surface !== 'user'"><button v-if="row.source === 'platform'" class="row-action text-link" :disabled="surface === 'admin' && !row.game.connected" @click="withGlobalLoading(openRow, row, 'automation')">{{ t('drawConfiguration') }}</button><span v-else>—</span></td><td><button class="row-action text-link" :disabled="surface === 'admin' && row.source === 'platform' && !row.game.connected" @click="withGlobalLoading(openRow, row, 'model')">{{ t('model') }}</button></td><td v-if="surface === 'admin' && row.source === 'platform'"><span v-if="row.game.connected">{{ t('connected') }}</span><button v-else-if="canConnect" class="button button-secondary" :disabled="loading" @click="begin('connect', row.game)">{{ t('connect') }}</button></td><td v-if="surface !== 'user'"><span v-if="row.source==='platform'" class="status-chip">{{ row.game.status==='offline'?t('offline'):surface==='admin'&&row.game.connected?t(row.game.brand_enabled?'enabled':'disabled'):t('active') }}</span><div v-if="row.source==='platform'" class="platform-lottery-readiness"><span>{{ t(row.game.rule_published ? 'rulePublished' : 'ruleUnpublished') }}</span><span>{{ t(surface === 'admin' && !row.game.connected ? 'brandNotConnected' : row.game.sales_status === 'ready' ? 'salesReady' : row.game.sales_status === 'disabled' ? 'salesDisabled' : row.game.sales_status === 'outdated' ? 'salesOutdated' : 'salesUnconfigured') }}</span></div></td><td v-if="surface !== 'user'"><div v-if="row.source==='platform'" class="platform-lottery-actions"><template v-if="canManage"><button class="row-action text-link" :disabled="loading" @click="begin('name', row.game)">{{ t('editName') }}</button><button class="row-action text-link" :disabled="loading" @click="changeLifecycle(row.game,row.game.status==='active'?'offline':'active')">{{ t(row.game.status==='active'?'takeOffline':'relist') }}</button><button class="row-action text-link" :disabled="loading || !row.game.deletable" :title="row.game.deletable ? undefined : t('cannotDelete')" @click="changeLifecycle(row.game,'deleted')">{{ t('deleteGame') }}</button></template><button v-else-if="surface==='admin' && canSell && row.game.connected" class="row-action text-link" :disabled="loading" @click="changeAvailability(row.game)">{{ t(row.game.brand_enabled?'disableGame':'enableGame') }}</button></div></td></tr>
      <tr v-global-loading="loading || brandLoading" v-if="!visibleRows.length"><td :colspan="catalogColumnCount" class="empty-state">{{ loading || brandLoading ? t('loading') : t('empty') }}</td></tr>
    </tbody></table></div>
    <div class="wallet-pagination page-actions"><button class="button button-secondary" :disabled="loading || brandLoading || offset === 0" @click="withGlobalLoading(goPage, offset - 50)">{{ t('previous') }}</button><span>{{ offset / 50 + 1 }}</span><button class="button button-secondary" :disabled="loading || brandLoading || !hasNext" @click="withGlobalLoading(goPage, offset + 50)">{{ t('next') }}</button><Teleport defer to="#user-page-refresh" :disabled="surface !== 'user'"><button v-refresh-button="t('refresh')" class="button button-secondary" :disabled="loading || brandLoading" @click="withGlobalLoading(refresh)">{{ t('refresh') }}</button></Teleport></div>
    </div>
    <component :is="detailPage ? 'section' : DetailModal" v-if="selectedId" :class="{ 'lottery-detail-page': detailPage }" v-bind="detailPage ? { 'aria-label': detailTitle } : { title: detailTitle, closeLabel: t('close'), open: !formKind }" @close="closeDetail">
      <header v-if="detailPage" class="lottery-detail-page-heading">
        <button type="button" class="button button-secondary" :disabled="loading || !!pending || !!formKind" @click="closeDetail">{{ t('backToGames') }}</button>
        <h2>{{ detailTitle }}</h2>
      </header>
      <p v-if="shownError" class="message error" role="alert">{{ shownError }}</p>
      <p v-if="detailPage && notice" class="message" role="status">{{ t('success') }}</p>
      <template v-if="detail">
        <nav v-if="view === 'automation'" class="platform-lottery-actions draw-config-tabs" :aria-label="t('drawConfiguration')"><button type="button" class="button" :class="drawTab === 'sources' ? 'button-primary' : 'button-secondary'" :aria-pressed="drawTab === 'sources'" @click="drawTab='sources'">{{ t('drawSources') }}</button><button type="button" class="button" :class="drawTab === 'manual' ? 'button-primary' : 'button-secondary'" :aria-pressed="drawTab === 'manual'" @click="drawTab='manual'">{{ t('manualResults') }}</button></nav>
        <AutomationSettings v-if="view === 'automation' && drawTab === 'sources' && surface !== 'user'" :surface="surface" :game-id="detail.game.id" :brand-id="brandId" :locale="locale" :can-write="surface === 'platform' ? canManage : !!canSell" @failure="emit('failure', $event)" />
        <template v-if="view === 'periods' || (view === 'automation' && drawTab === 'manual')">
          <button v-if="canManage" class="button button-primary" @click="begin('period')">{{ t('createPeriod') }}</button>
          <div class="table-wrap platform-lottery-period-table"><table><thead><tr><th>{{ t('periodNo') }}</th><th>{{ t('start') }}</th><th>{{ t('end') }}</th><th>{{ t('drawAt') }}</th><th>{{ t('result') }}</th></tr></thead><tbody><tr v-for="period in detail.periods" :key="period.id"><td>{{ period.period_no }}</td><td>{{ formatTime(period.bet_start_at) }}</td><td>{{ formatTime(period.bet_end_at) }}</td><td>{{ formatTime(period.draw_at) }}</td><td><span v-if="period.result">{{ [...period.result.regular, ...period.result.special, ...period.result.digits].join(', ') }}</span><button v-else-if="canManage && view === 'automation'" class="row-action text-link" @click="begin('result', period)">{{ t('publish') }}</button><span v-else>{{ t('pending') }}</span></td></tr><tr v-if="!detail.periods.length"><td colspan="5" class="empty-state">{{ t('empty') }}</td></tr></tbody></table></div>
          <div class="platform-lottery-period-cards"><article v-for="period in detail.periods" :key="period.id" class="panel"><h3>{{ period.period_no }}</h3><dl><div><dt>{{ t('start') }}</dt><dd>{{ formatTime(period.bet_start_at) }}</dd></div><div><dt>{{ t('end') }}</dt><dd>{{ formatTime(period.bet_end_at) }}</dd></div><div><dt>{{ t('drawAt') }}</dt><dd>{{ formatTime(period.draw_at) }}</dd></div><div><dt>{{ t('result') }}</dt><dd v-if="period.result">{{ [...period.result.regular, ...period.result.special, ...period.result.digits].join(', ') }}</dd><dd v-else><button v-if="canManage && view === 'automation'" class="row-action text-link" @click="begin('result', period)">{{ t('publish') }}</button><span v-else>{{ t('pending') }}</span></dd></div></dl></article><p v-if="!detail.periods.length" class="empty-state">{{ t('empty') }}</p></div>
          <div class="wallet-pagination page-actions"><button class="button button-secondary" :disabled="loading || periodOffset === 0" @click="withGlobalLoading(open, detail.game, view, periodOffset - 50)">{{ t('previous') }}</button><span>{{ periodOffset / 50 + 1 }}</span><button class="button button-secondary" :disabled="loading || detail.periods.length < 50" @click="withGlobalLoading(open, detail.game, view, periodOffset + 50)">{{ t('next') }}</button></div>
        </template>
        <template v-else-if="view === 'plays'">
          <ConfiguredRuleWorkbench v-if="surface !== 'user' && ['configured','generic'].includes(detail.rule_protocol)" :surface="surface" :brand-id="brandId" :game-id="detail.game.id" :model="detail.game.model" :locale="locale" @failure="emit('failure',$event)" />
          <template v-if="detail.rule_protocol === 'generic'"><button v-if="canManage" class="button button-primary" @click="begin('play')">{{ t('createPlay') }}</button><div v-for="play in detail.plays" :key="play.id"><h3>{{ play.name }}</h3><details><summary>{{ t('definition') }}</summary><BusinessDetails :value="play.definition" :locale="props.locale" /></details></div><p v-if="!detail.plays.length" class="empty-state">{{ t('empty') }}</p></template>
        </template>
        <template v-else-if="view === 'model'"><button v-if="canManage" class="button button-primary" :disabled="loading || modelLocked" @click="begin('model')">{{ t('editModel') }}</button><p v-if="canManage && modelLocked" class="platform-lottery-note">{{ t('modelLocked') }}</p><NumberModelEditor :model-value="detail.game.model" :locale="locale" disabled /></template>
      </template>
    </component>
    <DetailModal v-if="formKind" :class="{ 'platform-lottery-confirm-modal': isConfirmation }" :title="dialogTitle" :close-label="t('close')" @close="!loading && !pending && (formKind = null)">
      <form v-write-guard="loading" novalidate v-inline-validation class="brand-form platform-lottery-form" @submit.prevent="withGlobalLoading(submit)"><fieldset :class="{ 'confirmation-form-fields': isConfirmation, 'period-form-fields': formKind === 'period', 'play-form-fields': formKind === 'play' }" :disabled="loading || !!pending">
        <div v-if="isConfirmation" class="platform-lottery-confirm-copy"><p class="platform-lottery-confirm-prompt">{{ t('gameActionPrompt', { action: actionLabel, name: target && 'name' in target ? target.name : '' }) }}</p><p v-if="actionHint" class="platform-lottery-note">{{ actionHint }}</p></div>
        <label v-if="formKind === 'name'">{{ t('name') }}<input v-model.trim="form.name" required maxlength="120" /></label><template v-if="formKind === 'game' || formKind === 'play'"><label>{{ t('name') }}<input v-model.trim="form.name" required maxlength="120" /></label><label>{{ t('code') }}<input v-model.trim="form.code" pattern="[a-z][a-z0-9_]{0,47}" required /></label></template>
        <template v-if="formKind === 'game'"><label>{{ t('timezone') }}<select v-model="form.timezone" class="lottery-timezone" required><option v-for="zone in timezones" :key="zone" :value="zone">{{ zone }}</option></select></label><NumberModelEditor v-model="modelDraft" :locale="locale" :disabled="loading || !!pending" /></template><NumberModelEditor v-if="formKind === 'model'" v-model="modelDraft" :locale="locale" :disabled="loading || !!pending" />
        <template v-if="formKind === 'period'">
          <label class="period-field">{{ t('periodNo') }}<input v-model.trim="form.periodNo" required /></label>
          <div v-for="field in (['start', 'end', 'draw'] as const)" :key="field" class="period-field">
            <span>{{ t(field === 'start' ? 'start' : field === 'end' ? 'end' : 'drawAt') }}</span>
            <span class="period-time-controls">
              <PlatformDateInput v-model="form[`${field}Date`]" :locale="locale" :display-settings="displaySettings!" :label="`${t(field === 'start' ? 'start' : field === 'end' ? 'end' : 'drawAt')} · ${t('date')}`" />
              <PlatformTimeInput v-model="form[`${field}Time`]" :locale="locale" :display-settings="displaySettings!" :label="`${t(field === 'start' ? 'start' : field === 'end' ? 'end' : 'drawAt')} · ${t('time')}`" />
            </span>
          </div>
        </template>
        <PlatformPlayRuleForm v-if="formKind === 'play' && playDraft && detail" :model="detail.game.model" :draft="playDraft" :locale="locale" />
        <template v-if="formKind === 'result'"><label v-if="resultHasRegular">{{ t('regular') }}<input v-model="form.regular" /></label><label v-if="resultHasSpecial">{{ t('special') }}<input v-model="form.special" /></label><label v-if="resultHasDigits">{{ t('digits') }}<input v-model="form.digits" /></label><p>{{ t('numbers') }}</p></template>

      </fieldset><p v-if="pending">{{ t('awaiting') }}</p><p v-if="shownError" class="message error" role="alert">{{ shownError }}</p><div class="platform-lottery-form-actions" :class="{ 'confirmation-actions': isConfirmation }"><button v-if="isConfirmation && !pending" type="button" class="button button-secondary" :disabled="loading" @click="formKind=null">{{ t('cancelAction') }}</button><button class="button" :class="formKind === 'lifecycle' && lifecycleAction === 'deleted' ? 'button-danger' : 'button-primary'" :disabled="loading" type="submit">{{ pending ? t('retry') : isConfirmation ? actionLabel : t('save') }}</button></div></form>
    </DetailModal>
  </section>
</template>

<style scoped>
.lottery-detail-page{min-width:0;padding:16px}.lottery-detail-page-heading{display:flex;align-items:center;flex-wrap:wrap;gap:12px;margin-bottom:16px}.lottery-detail-page-heading h2{margin:0;overflow-wrap:anywhere;font-size:18px}.lottery-detail-page-heading .button{min-height:44px}
.platform-lottery{text-align:left}
.platform-lottery-readiness{display:grid;gap:4px;font-size:12px;color:var(--muted);white-space:normal}
.platform-lottery-toolbar{display:flex;align-items:center;flex-wrap:wrap;gap:12px;padding:12px 16px;border-bottom:1px solid var(--line)}
.platform-lottery-toolbar h2{margin:0 auto 0 0}
.platform-lottery-toolbar .platform-lottery-filter{margin:0;flex-wrap:wrap;min-width:0}
.platform-lottery-toolbar > .button{margin-left:auto;min-height:44px}
.platform-lottery-toolbar select{min-width:0}
.platform-lottery .row-action{appearance:none;border:0;background:transparent;color:var(--primary);font:inherit;font-size:12px;font-weight:650;line-height:1.35;padding:6px;border-radius:var(--radius-sm);cursor:pointer}
.platform-lottery .row-action:hover:not(:disabled){background:var(--primary-light)}
.platform-lottery .row-action:disabled{opacity:.45;cursor:not-allowed}
.platform-lottery { min-width: 0; margin-bottom: 16px; }
.platform-lottery-note { padding: 0 16px; font-size: 12px; color: var(--text-muted, var(--muted)); }
.draw-config-tabs { margin-bottom: 16px; }
.platform-lottery-actions { display: flex; flex-wrap: wrap; gap: 8px; }
.platform-lottery-form fieldset { border: 0; padding: 0; margin: 0; display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 300px), 1fr)); gap: 12px 20px; width: 100%; box-sizing: border-box; min-width: 0; }
.platform-lottery-form .period-form-fields { grid-template-columns: minmax(0, 1fr); }
.detail-modal.platform-lottery-confirm-modal { width: min(480px, calc(100vw - 24px)); max-width: calc(100vw - 24px); }
.platform-lottery-form .confirmation-form-fields { grid-template-columns: minmax(0, 1fr); }
.platform-lottery-confirm-copy { display: grid; gap: 8px; min-width: 0; }
.platform-lottery-confirm-prompt { margin: 0; font-size: 16px; font-weight: 650; line-height: 1.6; overflow-wrap: anywhere; }
.platform-lottery-confirm-copy .platform-lottery-note { margin: 0; padding: 0; font-size: 13px; line-height: 1.6; }
.platform-lottery-form-actions { display: flex; flex-wrap: wrap; gap: 10px; margin-top: 14px; grid-column: 1 / -1; }
.platform-lottery-form-actions.confirmation-actions { justify-content: flex-end; }
.platform-lottery-form .platform-lottery-form-actions > button { width: auto; margin: 0; min-height: 44px; }
.platform-lottery-form .play-form-fields { grid-template-columns: repeat(2, minmax(0, 1fr)); align-items: start; }
.platform-lottery-form .play-form-fields label { align-content: start; }
.platform-lottery-form .play-form-fields input { width: 100%; min-height: 40px; }
.platform-lottery-form .number-model-editor, .lottery-form-reason { grid-column: 1 / -1; }
.lottery-form-reason { width: 100%; max-width: 400px; }
.platform-lottery-form label, .platform-lottery-form .period-field { display: grid; grid-template-columns: minmax(0, 1fr); min-width: 0; gap: 4px; font-size: 13px; }
.platform-lottery-form input, .platform-lottery-form select, .platform-lottery-form textarea { box-sizing: border-box; min-width: 0; max-width: 100%; font: inherit; padding: 8px; border: 1px solid var(--line); border-radius: var(--radius-sm); background: var(--surface); color: var(--ink); }
.platform-lottery-form input { width: 300px; }
.platform-lottery-form .period-field input { width: min(100%, 420px); }
.platform-lottery-form .period-time-controls { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 12px; max-width: 852px; }
.platform-lottery-form .period-time-controls input { width: 100%; }
.platform-lottery-form select { width: 100%; min-height: 40px; padding-right: 32px; justify-self: stretch; }
.platform-lottery-form textarea { width: 100%; }
.platform-lottery-form > button { justify-self: start; width: auto; margin-top: 14px; }
.platform-lottery pre { white-space: pre-wrap; overflow-wrap: anywhere; font-size: 12px; }
.platform-lottery td { overflow-wrap: anywhere; }
.platform-lottery-period-cards { display: none; }
@media (max-width: 640px) {
  .platform-lottery-form .play-form-fields { grid-template-columns: minmax(0, 1fr); }
  .platform-lottery-form .period-time-controls { grid-template-columns: minmax(0, 1fr); }
  .platform-lottery-period-table { display: none; }
  .platform-lottery-period-cards { display: grid; gap: 12px; }
  .platform-lottery-period-cards article { padding: 12px; }
  .platform-lottery-period-cards dl div { display: grid; grid-template-columns: 80px minmax(0, 1fr); gap: 8px; font-size: 13px; margin: 8px 0; }
  .platform-lottery-period-cards dd { margin: 0; overflow-wrap: anywhere; }
  .platform-lottery button { min-height: 44px; }
  .platform-lottery th:first-child, .platform-lottery td:first-child,
  .platform-lottery th:last-child, .platform-lottery td:last-child { position: static; }
}
</style>

<style scoped>
.platform-lottery{text-align:left}

.platform-lottery-toolbar{display:flex;align-items:center;flex-wrap:wrap;gap:12px;padding:12px 16px;border-bottom:1px solid var(--line)}
.platform-lottery-toolbar h2{margin:0 auto 0 0}
.platform-lottery-toolbar .platform-lottery-filter{margin:0;flex-wrap:wrap;min-width:0}
.platform-lottery-toolbar > .button{margin-left:auto;min-height:44px}
.platform-lottery-toolbar select{min-width:0}
.platform-lottery .row-action{appearance:none;border:0;background:transparent;color:var(--primary);font:inherit;font-size:12px;font-weight:650;line-height:1.35;padding:6px;border-radius:var(--radius-sm);cursor:pointer}
.platform-lottery .row-action:hover:not(:disabled){background:var(--primary-light)}
.platform-lottery .row-action:disabled{opacity:.45;cursor:not-allowed}
.platform-lottery-filter{display:flex;align-items:center;gap:8px;margin:8px 0;font-size:12px}.platform-lottery-filter select{width:auto;min-width:130px;max-width:100%}.platform-lottery-actions{display:flex;flex-wrap:wrap;gap:6px}
@media(max-width:620px){
.platform-lottery-filter select{min-height:44px}.platform-lottery-actions button{min-height:44px}
.platform-lottery-catalog-table{display:block;min-width:0!important;width:100%;table-layout:auto}
.platform-lottery-catalog-table thead{display:none}
.platform-lottery-catalog-table tbody{display:block;width:100%}
.platform-lottery-catalog-table tr{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:4px 8px;margin:8px 0;padding:10px;border:1px solid var(--line);border-radius:var(--radius-sm);background:var(--surface)}
.platform-lottery-catalog-table td:not(.empty-state){display:flex;align-items:center;flex-wrap:wrap;gap:6px;min-width:0;height:auto!important;min-height:0;padding:2px;border:0;white-space:normal;overflow-wrap:anywhere}
.platform-lottery-catalog-table td:first-child,.platform-lottery-catalog-table td:nth-child(2),.platform-lottery-catalog-table td:last-child{grid-column:1/-1}
.platform-lottery-catalog-table td:first-child{font-weight:750;font-size:14px}
.platform-lottery-catalog-table td:nth-child(2){font-size:11px;color:var(--muted)}
.platform-lottery-catalog-table button{min-height:44px}
}
</style>
