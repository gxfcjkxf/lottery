<script setup lang="ts">
import { readPaginationTotal } from './pagination'
const catalogTotal = ref('0')

import ManagementList from './ManagementList.vue'
import ManagementTable from './ManagementTable.vue'
import TablePagination from "./TablePagination.vue"
import SelectControl from './SelectControl.vue'
import { useOperationToast } from "@lottery/shared";
import { formValidationText } from './form-validation'
import { vWriteGuard } from "@lottery/shared";
import { withGlobalLoading } from './global-loading';
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading } from './global-loading'
import { formatDisplayDateTime } from './display-settings'
import { vInlineValidation } from './form-validation'
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import DetailModal from './DetailModal.vue'
import MemberActionsMenu from './MemberActionsMenu.vue'
import NumberModelEditor from './NumberModelEditor.vue'
import PlatformDateInput from './PlatformDateInput.vue'
import PlatformTimeInput from './PlatformTimeInput.vue'
import ConfiguredRuleWorkbench from './ConfiguredRuleWorkbench.vue'
import ConfiguredPublishing from './ConfiguredPublishing.vue'
import DrawScheduleSettings from './DrawScheduleSettings.vue'
import AutomationTimeSettings from './AutomationTimeSettings.vue'
import AutomationSettings from './AutomationSettings.vue'
import PlatformDrawOverview from './PlatformDrawOverview.vue'
import PeriodOpeningDialog from './PeriodOpeningDialog.vue'
import DrawResultInputs from './DrawResultInputs.vue'
import { lotteryLocalTimeToISO } from './lottery-time'
import type { PlatformDisplaySettings } from './date-time'
import { createCatalogI18n } from './localization'
import { createPlatformLotteryClient, platformLotteryCatalog, PlatformLotteryError, type PlatformGame, type PlatformGameDetail, type PlatformPeriod } from './platform-lottery'
import type { RuleModel } from './rules'
const catalogPageSize = ref(50)
const periodPageSize = ref(50)


const props = defineProps<{ surface: 'platform' | 'admin' | 'user'; brandId: string; brandCode?: string; initialGameId?: string; drawsOnly?: boolean; locale: 'en' | 'zh-CN'; canManage: boolean; canConnect: boolean; canSell?: boolean; oddsOnly?: boolean; unifiedCatalog?: boolean; brandGames?: Array<{ id: string; name: string; code: string; timezone: string }>; brandName?: string; brandLoading?: boolean; catalogBrandId?: string; displaySettings?: PlatformDisplaySettings }>()
if (props.surface === 'platform' && !props.displaySettings) throw new Error('Platform lottery requires platform display settings')
const emit = defineEmits<{ failure: [cause: unknown]; brandDetail: [id: string, view: 'periods' | 'plays' | 'model']; refreshBrand: [] }>()
const i18n = createCatalogI18n(platformLotteryCatalog, props.locale)
watch(() => props.locale, value => i18n.setLocale(value))
const t = i18n.t
const timezones = ['UTC', 'Asia/Shanghai', 'Asia/Singapore', 'Asia/Tokyo', 'Asia/Seoul', 'Asia/Kolkata', 'Asia/Dubai', 'Asia/Jakarta', 'Asia/Bangkok', 'Asia/Hong_Kong', 'Asia/Taipei', 'Europe/London', 'Europe/Berlin', 'Europe/Paris', 'Europe/Moscow', 'America/New_York', 'America/Chicago', 'America/Denver', 'America/Los_Angeles', 'America/Toronto', 'America/Sao_Paulo', 'Australia/Sydney', 'Pacific/Auckland']
const api = createPlatformLotteryClient(props.surface, () => props.brandId, fetch, props.brandCode)
const lifecycleFilter = ref('active'), lifecycleDraft = ref('active')
const games = ref<PlatformGame[]>([]), detail = ref<PlatformGameDetail | null>(null)
const selectedId = ref(''), offset = ref(0), periodOffset = ref(0), loading = ref(false), error = ref(''), notice = ref(false)
const markSix=computed(()=>detail.value?.game.model.model==='X_PLUS_Y'&&detail.value.game.model.regular_count===6&&detail.value.game.model.special_count===1)
const canManagePlatform = computed(() => props.surface === 'platform' && props.canManage)
const catalogColumnCount = computed(() => props.surface === 'admin' ? 3 : 4 + (props.unifiedCatalog ? 1 : 0) + (props.surface === 'platform' ? 3 + (canManagePlatform.value ? 1 : 0) : 0))
const view = ref<'periods' | 'plays' | 'model' | 'automation' | 'schedule' | 'draws'>('periods')
const detailPage = computed(() => props.surface !== 'user' && !!selectedId.value)
defineExpose({ detailPage })
const showPeriodResults = computed(() => props.surface === 'user')
const detailTitle = computed(() => `${t(view.value === 'automation' ? 'drawSources' : view.value === 'schedule' ? 'timeSettings' : view.value === 'draws' ? 'drawResults' : view.value)} · ${detail.value?.game.name ?? ''}`)
let catalogScroll = 0
const catalogRows = computed(() => [
  ...games.value.filter(game => !props.oddsOnly || game.connected).map(game => ({ source: 'platform' as const, game })),
  ...(props.unifiedCatalog && props.catalogBrandId ? (props.brandGames ?? []).map(game => ({ source: 'brand' as const, game })) : []),
])
const filteredRows = computed(() => catalogRows.value.filter(row => lifecycleFilter.value === 'all' || (row.source === 'brand' ? lifecycleFilter.value === 'active' : row.game.status === lifecycleFilter.value)))
const brandCatalogGames = computed(() => games.value.filter(game => (!props.oddsOnly || game.connected) && (lifecycleFilter.value === 'all' || game.status === lifecycleFilter.value)).slice(offset.value, offset.value + catalogPageSize.value))
const visibleRows = computed(() => props.surface !== 'user' ? filteredRows.value.slice(offset.value, offset.value + catalogPageSize.value) : filteredRows.value)
const hasNext = computed(() => props.surface !== 'user' ? offset.value + catalogPageSize.value < filteredRows.value.length : BigInt(offset.value + catalogPageSize.value) < BigInt(catalogTotal.value))
async function goPage(page: number) { if (props.surface !== 'user') offset.value = page; else await load(page) }
async function refresh() { emit('refreshBrand'); await load() }
async function searchCatalog() { await load(0); if (!error.value) lifecycleFilter.value = lifecycleDraft.value }

async function openRow(row: typeof catalogRows.value[number], next: typeof view.value) {
  if (row.source === 'brand') { if (next === 'periods'||next==='plays'||next==='model') emit('brandDetail', row.game.id, next) }
  else await open(row.game, next)
}
const formKind = ref<'name' | 'game' | 'connect' | 'period' | 'result' | 'model' | 'lifecycle' | 'availability' | null>(null)
const salesGame = ref<PlatformGame|null>(null), salesLocked=ref(false)
const openingPeriod = ref<PlatformPeriod|null>(null)
const target = ref<PlatformGame | PlatformPeriod | null>(null)
const form = ref({ name: '', code: '', timezone: 'Asia/Singapore', reason: '', periodNo: '', startDate: '', startTime: '', endDate: '', endTime: '', drawDate: '', drawTime: '' })
const pending = ref<{ path: string; body: unknown; key: string } | null>(null)
const defaultModel = (): RuleModel => ({ model: 'DIGITS_0_9', regular_pool: { allow_repeat: false }, special_pool: { allow_repeat: false }, regular_count: 0, special_count: 0, pool_size: 0, total_count: 0, length: 3, allow_repeat: true, ordered: true })
const modelDraft = ref<RuleModel>(defaultModel())
const resultModel = computed(() => detail.value?.game.model)
const drawInputs = ref<InstanceType<typeof DrawResultInputs> | null>(null)
const drawOverview = ref<InstanceType<typeof PlatformDrawOverview> | null>(null)
function manualResult() {
 form.value.periodNo = ''; form.value.drawDate = ''
 begin('result')
}
const modelLocked = computed(() => !!detail.value && (detail.value.game.rule_published || detail.value.periods.length > 0))
const lifecycleAction = ref<'active' | 'offline' | 'deleted'>('offline')
const availabilityEnabled = ref(false)
const isConfirmation = computed(() => formKind.value === 'lifecycle' || formKind.value === 'availability' || formKind.value === 'connect')
function changeLifecycle(game: PlatformGame, status: 'active' | 'offline' | 'deleted') { lifecycleAction.value=status; begin('lifecycle',game) }
function changeAvailability(game: PlatformGame) { availabilityEnabled.value=!game.brand_enabled; begin('availability',game) }
const actionLabel = computed(() => formKind.value === 'connect' ? t('enableGame') : formKind.value === 'availability' ? t(availabilityEnabled.value?'enableGame':'disableGame') : t(lifecycleAction.value==='deleted'?'deleteGame':lifecycleAction.value==='offline'?'takeOffline':'relist'))
const actionHint = computed(() => formKind.value==='connect' ? t('enableHint') : formKind.value==='availability' ? t(availabilityEnabled.value?'enableHint':'brandDisableHint') : lifecycleAction.value==='deleted' ? '' : t(lifecycleAction.value==='offline'?'offlineHint':'enableHint'))
let generation = 0
let alive = true
const dialogTitle = computed(() => isConfirmation.value ? props.surface === 'admin' ? t('gameActionTitle', { action: actionLabel.value, name: target.value && 'name' in target.value ? target.value.name : '' }) : actionLabel.value : formKind.value ? t(({ name: 'editName', game: 'create', connect: 'connect', period: 'createPeriod', result: 'manualAdd', model: 'editModel', lifecycle: 'lifecycleConfirm', availability: 'lifecycleConfirm' } as const)[formKind.value]) : '')
const shownError = computed(() => error.value ? (error.value in platformLotteryCatalog.en ? t(error.value) : i18n.errorText({ code: error.value.replace(/^error\./, '') })) : '')
function report(cause: unknown) {
  if (cause instanceof PlatformLotteryError) {
    if (cause.status === 401) { emit('failure', cause); return }
    error.value = `error.${cause.code}`
  }
  else error.value = 'invalid'
}
async function load(page = 0) {
  const request = ++generation
  loading.value = true; error.value = ''; detail.value = null; selectedId.value = ''
  try {
    const rows: PlatformGame[] = []
    let total = '0'
    for (let next = props.surface !== 'user' ? 0 : page; ; next += catalogPageSize.value) {
      const result = await api.list(next, catalogPageSize.value)
      if (!alive || request !== generation) return
      total = readPaginationTotal(result)
      rows.push(...result.games)
      if (props.surface === 'user' || BigInt(rows.length) >= BigInt(total)) break
    }
    games.value = rows; catalogTotal.value = total; offset.value = page
  }
  catch (cause) { if (alive && request === generation) report(cause) }
  finally { if (alive && request === generation) loading.value = false }
}
async function open(game: Pick<PlatformGame, 'id'>, next: typeof view.value, page = 0) {
  if (!selectedId.value && props.surface !== 'user') catalogScroll = window.scrollY
  const request = ++generation
  if (selectedId.value !== game.id || view.value !== next) notice.value = false
  const sameView = selectedId.value === game.id && view.value === next
  if (!sameView) detail.value = null
  selectedId.value = game.id; view.value = next; loading.value = true; error.value = ''
  if (detailPage.value && !sameView) { await nextTick(); window.scrollTo(0, 0) }
  try { const record = await api.detail(game.id, page, periodPageSize.value); if (alive && request === generation) { detail.value = record; periodOffset.value = page } }
  catch (cause) { if (alive && request === generation) report(cause) }
  finally { if (alive && request === generation) loading.value = false }
}
const rulesWorkbench = ref<InstanceType<typeof ConfiguredRuleWorkbench> | null>(null)
async function closeDetail() {
  if (view.value === 'plays' && rulesWorkbench.value && !rulesWorkbench.value.confirmLeave()) return
  const wasPage = detailPage.value
  generation++; detail.value = null; selectedId.value = ''; loading.value = false
  if (wasPage) { await nextTick(); window.scrollTo(0, catalogScroll) }
}
function begin(kind: NonNullable<typeof formKind.value>, row?: PlatformGame | PlatformPeriod) {
  if (kind === 'name') form.value.name = (row as PlatformGame).name
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
        case 'result': {
          const result = drawInputs.value?.result()
          if (!result) return
          path = `/${selectedId.value}/results`; body = { period_no: f.periodNo, draw_date: f.drawDate, result }; break
        }
        default: throw new Error()
      }
      pending.value = { path, body, key: crypto.randomUUID() }
    } catch { error.value = 'invalid'; return }
  }
  loading.value = true
  const request = generation, snapshot = pending.value, current = detail.value?.game, nextView = view.value
  try {
    await api.write(snapshot.path, snapshot.body, snapshot.key)
    if (!alive || request !== generation) return
    pending.value = null; formKind.value = null; notice.value = true
    if (snapshot.path.endsWith('/results')) await drawOverview.value?.load((snapshot.body as {draw_date: string}).draw_date.slice(0, 4), 0)
    if (snapshot.path.endsWith('/name') || snapshot.path.endsWith('/lifecycle') || snapshot.path.endsWith('/availability')) await load(offset.value)
    else if (current) await open(current, nextView, periodOffset.value); else await load(offset.value)
  } catch (cause) {
    if (!alive || request !== generation) return
    if (cause instanceof PlatformLotteryError && cause.status >= 400 && cause.status < 500) pending.value = null
    report(cause)
  } finally { if (alive && request === generation) loading.value = false }
}
watch(() => [props.brandId, props.surface, props.initialGameId], () => {
  salesGame.value=null;salesLocked.value=false
  games.value = []; detail.value = null; selectedId.value = ''; offset.value = 0
  pending.value = null; formKind.value = null
  if (props.surface === 'platform' && props.drawsOnly && props.initialGameId) void open({ id: props.initialGameId }, 'draws')
  else if (props.surface === 'user' && props.initialGameId) void open({ id: props.initialGameId }, 'periods')
  else void load()
}, { immediate: true })
watch(() => props.catalogBrandId, () => { offset.value = 0 })
const resultPoll=setInterval(async()=>{if(props.surface==='user'&&detail.value&&view.value==='periods'&&!loading.value){const current=generation,id=detail.value.game.id;try{const value=await api.detail(id,periodOffset.value,periodPageSize.value);if(alive&&current===generation&&selectedId.value===id){detail.value=value;error.value=''}}catch(e){if(alive&&current===generation)report(e)}}},5000)
onUnmounted(() => { alive = false; generation++;clearInterval(resultPoll) })
useOperationToast(notice, () => t('success'), 'success');

useOperationToast(shownError, value => value, 'error');
async function clearCatalog() { lifecycleDraft.value = 'active'; await searchCatalog() }
</script>

<template>
  <section class="platform-lottery" :class="{ panel: surface === 'user' }" data-testid="platform-shared-games">
    <slot name="catalog-header" />
    <component :is="surface === 'user' ? 'div' : ManagementList" v-show="!detailPage && !drawsOnly" :busy="loading || brandLoading" data-testid="lottery-catalog-page">
    <Teleport v-if="surface === 'user'" defer to="#user-page-refresh"><button v-if="error" v-refresh-button="t('refresh')" class="button button-secondary" :disabled="loading || brandLoading" @click="withGlobalLoading(refresh)">{{ t('refresh') }}</button></Teleport>
    <template v-if="canManagePlatform" #actions><button v-if="canManagePlatform" class="button button-primary" :disabled="loading" @click="begin('game')">{{ t('create') }}</button></template>
    <template v-if="surface !== 'user'" #filters><form v-inline-validation novalidate class="ui-filter-bar" @submit.prevent="withGlobalLoading(searchCatalog)">
      <label class="platform-lottery-filter"><SelectControl :label="t('lifecycleFilter')" v-model="lifecycleDraft" :options="[{ value: 'active', label: (String(t('active'))).trim() }, { value: 'offline', label: (String(t('offline'))).trim() }, { value: 'all', label: (String(t('allStates'))).trim() }]" /></label>
      <button type="submit" class="button button-primary" :disabled="loading || brandLoading">{{t('filterSearch')}}</button><button v-if="lifecycleDraft !== 'active' || lifecycleFilter !== 'active'" type="button" class="button button-secondary" :disabled="loading || brandLoading" @click="withGlobalLoading(clearCatalog)">{{t('filterClear')}}</button>

    </form></template>
    <div v-if="surface === 'user'" class="platform-lottery-toolbar"><h2 v-if="!unifiedCatalog">{{t('title')}}</h2></div>


    <ManagementTable v-if="surface === 'admin'" table-class="brand-lottery-table" :entries="brandCatalogGames" :columns="[{key:'name',label:t('name')},{key:'sales',label:t('salesConfiguration')},{key:'status',label:t('status')},...((oddsOnly || canConnect || canSell) ? [{key:'actions',label:t('actions')}] : [])]" :empty="!loading && !brandLoading && !error && !brandCatalogGames.length" :empty-label="t('empty')">
      <template #name="{entry:game}">{{game.name}}</template>
      <template #sales="{entry:game}">{{game.connected ? t(game.sales_status==='ready'?'salesReady':game.sales_status==='outdated'?'salesOutdated':game.sales_status==='disabled'?'salesDisabled':'salesUnconfigured') : '—'}}</template>
      <template #status="{entry:game}">{{t(game.status === 'offline' ? 'offline' : game.connected && game.brand_enabled ? 'shortEnabled' : 'shortDisabled')}}</template>
      <template #actions="{entry:game}"><MemberActionsMenu v-if="game.connected && (oddsOnly || canSell) || !oddsOnly && !game.connected && canConnect && game.status === 'active'" :id="`brand-lottery-actions-${game.id}`" :label="t('actions')">
        <button v-if="game.connected && (oddsOnly || canSell)" type="button" role="menuitem" :disabled="loading" @click="salesGame=game">{{t(oddsOnly?'salesOdds':'salesConfiguration')}}</button>
        <button v-if="!oddsOnly && !game.connected && canConnect && game.status === 'active'" type="button" role="menuitem" :disabled="loading" @click="begin('connect', game)">{{t('shortEnable')}}</button>
        <button v-else-if="!oddsOnly && canSell && game.connected" type="button" role="menuitem" :disabled="loading" @click="changeAvailability(game)">{{t(game.brand_enabled ? 'shortDisable' : 'shortEnable')}}</button>
      </MemberActionsMenu></template>
    </ManagementTable>
    <div v-else class="table-wrap"><table class="console-table platform-lottery-catalog-table console-responsive-table"><thead><tr><th>{{ t('name') }}</th><th v-if="unifiedCatalog">{{ t('ownership') }}</th><th>{{ t('periods') }}</th><th>{{ t('plays') }}</th><th v-if="surface === 'platform'">{{ t('timeSettings') }}</th><th v-if="surface === 'platform'">{{ t('drawSources') }}</th><th>{{ t('model') }}</th><th v-if="surface !== 'user'">{{ t('status') }}</th><th v-if="canManagePlatform">{{ t('actions') }}</th></tr></thead><tbody>
      <tr v-for="row in visibleRows" :key="`${row.source}:${row.game.id}`"><td :data-label="String(t('name'))">{{ row.game.name }}</td><td :data-label="String(t('ownership'))" v-if="unifiedCatalog">{{ row.source === 'platform' ? t('platformOwner') : brandName }}</td><td :data-label="String(t('periods'))"><button class="text-button row-action text-link" @click="withGlobalLoading(openRow, row, 'periods')">{{ t('periods') }}</button></td><td :data-label="String(t('plays'))"><button class="text-button row-action text-link" @click="withGlobalLoading(openRow, row, 'plays')">{{ t('plays') }}</button></td><td :data-label="String(t('timeSettings'))" v-if="surface === 'platform'"><button v-if="row.source === 'platform'" class="text-button row-action text-link" @click="withGlobalLoading(openRow, row, 'schedule')">{{t('timeSettings')}}</button><span v-else>—</span></td><td :data-label="String(t('drawSources'))" v-if="surface === 'platform'"><button v-if="row.source === 'platform'" class="text-button row-action text-link" @click="withGlobalLoading(openRow, row, 'automation')">{{ t('drawSources') }}</button><span v-else>—</span></td><td :data-label="String(t('model'))"><button class="text-button row-action text-link" @click="withGlobalLoading(openRow, row, 'model')">{{ t('model') }}</button></td><td :data-label="String(t('status'))" v-if="surface !== 'user'"><span v-if="row.source==='platform'" class="status-chip">{{ row.game.status==='offline'?t('offline'):t('active') }}</span></td><td :data-label="String(t('actions'))" v-if="canManagePlatform"><div v-if="row.source==='platform'" class="platform-lottery-actions"><MemberActionsMenu v-if="canManagePlatform" :id="`lottery-actions-${row.game.id}`" :label="t('actions')"><button type="button" role="menuitem" :disabled="loading" @click="begin('name', row.game)">{{ t('editName') }}</button><button type="button" role="menuitem" :disabled="loading" @click="changeLifecycle(row.game,row.game.status==='active'?'offline':'active')">{{ t(row.game.status==='active'?'takeOffline':'relist') }}</button><button type="button" role="menuitem" class="danger-text" :disabled="loading || !row.game.deletable" :title="row.game.deletable ? undefined : t('cannotDelete')" @click="changeLifecycle(row.game,'deleted')">{{ t('deleteGame') }}</button></MemberActionsMenu></div></td></tr>
      <tr v-global-loading="loading || brandLoading" v-if="!error && !visibleRows.length"><td :colspan="catalogColumnCount" class="empty-state">{{ loading || brandLoading ? t('loading') : t('empty') }}</td></tr>
    </tbody></table></div>
    <template v-if="surface !== 'user'" #pagination><TablePagination class="wallet-pagination page-actions" :total="String(filteredRows.length)" :page="Math.floor(offset / catalogPageSize + 1)" :count="visibleRows.length" :locale="locale" :page-size="catalogPageSize" :previous-disabled="loading || brandLoading || offset === 0" :next-disabled="loading || brandLoading || !hasNext" :previous-label="t('previous')" :next-label="t('next')" :go-to="(target: number) => goPage((target - 1) * catalogPageSize)" :change-page-size="async (size: number) => { catalogPageSize = size; await load(0) }" :busy="loading || brandLoading" /></template>
    </component>
    <TablePagination v-if="surface === 'user' && !detailPage && !drawsOnly" class="wallet-pagination page-actions" :total="catalogTotal" :page="Math.floor(offset / catalogPageSize + 1)" :count="visibleRows.length" :locale="locale" :page-size="catalogPageSize" :previous-disabled="loading || brandLoading || offset === 0" :next-disabled="loading || brandLoading || !hasNext" :previous-label="t('previous')" :next-label="t('next')" :go-to="(target: number) => goPage((target - 1) * catalogPageSize)" :change-page-size="async (size: number) => { catalogPageSize = size; await load(0) }" :busy="loading || brandLoading" />
    <component :is="detailPage ? 'section' : DetailModal" v-if="selectedId" :class="{ 'lottery-detail-page': detailPage }" v-bind="detailPage ? { 'aria-label': detailTitle } : { title: detailTitle, closeLabel: t('close'), open: !formKind }" @close="closeDetail">
      <header v-if="detailPage && !drawsOnly" class="lottery-detail-page-heading">
        <button v-if="!drawsOnly" type="button" class="button button-secondary" :disabled="loading || !!pending || !!formKind" @click="closeDetail">{{ t('backToGames') }}</button>
        <h2>{{ detailTitle }}</h2>
      </header>


      <template v-if="detail">
        <PlatformDrawOverview ref="drawOverview" v-if="view === 'draws' && surface === 'platform'" :game-id="detail.game.id" :locale="locale" :can-manage="canManagePlatform&&detail.game.rule_published" @failure="emit('failure', $event)" @manual="manualResult" />
        <DrawScheduleSettings v-if="view==='schedule'&&surface==='platform'&&markSix" :game-id="detail.game.id" :timezone="detail.game.timezone" :locale="locale" :can-write="canManagePlatform" @failure="emit('failure',$event)" />
        <AutomationTimeSettings v-if="view==='schedule'&&surface==='platform'&&!markSix" :game-id="detail.game.id" :locale="locale" :can-write="canManagePlatform" @failure="emit('failure',$event)" />
        <slot v-if="view==='schedule'&&detail.game.timezone==='Asia/Hong_Kong'" name="calendar" />
        <AutomationSettings v-if="view === 'automation' && surface !== 'user'" :surface="surface" :game-id="detail.game.id" :brand-id="brandId" :locale="locale" :can-write="canManagePlatform" @failure="emit('failure', $event)" />
        <template v-if="view === 'periods'"><ManagementList embedded>
          <template v-if="canManagePlatform&&!markSix" #actions><button v-if="canManagePlatform&&!markSix" class="button button-primary" :disabled="!detail.game.rule_published" :title="detail.game.rule_published?undefined:t('ruleUnpublished')" @click="begin('period')">{{ t('createPeriod') }}</button></template>
          <div class="table-wrap platform-lottery-period-table"><table class="console-table console-responsive-table"><thead><tr><th>{{ t('periodNo') }}</th><th>{{ t('start') }}</th><th>{{ t('end') }}</th><th>{{ t('drawAt') }}</th><th v-if="showPeriodResults">{{ t('result') }}</th></tr></thead><tbody><tr v-for="period in detail.periods" :key="period.id"><td :data-label="String(t('periodNo'))">{{ period.period_no }}</td><td :data-label="String(t('start'))">{{ formatTime(period.bet_start_at) }} <button v-if="canManagePlatform&&!markSix&&!period.result" class="text-button row-action text-link" @click="openingPeriod=period">{{t('editOpening')}}</button></td><td :data-label="String(t('end'))">{{ formatTime(period.bet_end_at) }}</td><td :data-label="String(t('drawAt'))">{{ formatTime(period.draw_at) }}</td><td :data-label="String(t('result'))" v-if="showPeriodResults"><span v-if="period.result">{{ [...period.result.regular, ...period.result.special, ...period.result.digits].join(', ') }}</span><span v-else>{{ t('pending') }}</span></td></tr><tr v-if="!detail.periods.length"><td :colspan="showPeriodResults ? 5 : 4" class="empty-state">{{ t('empty') }}</td></tr></tbody></table></div>
          <div class="platform-lottery-period-cards"><article v-for="period in detail.periods" :key="period.id" class="panel"><h3>{{ period.period_no }}</h3><dl><div><dt>{{ t('start') }}</dt><dd>{{ formatTime(period.bet_start_at) }} <button v-if="canManagePlatform&&!markSix&&!period.result" class="text-button row-action text-link" @click="openingPeriod=period">{{t('editOpening')}}</button></dd></div><div><dt>{{ t('end') }}</dt><dd>{{ formatTime(period.bet_end_at) }}</dd></div><div><dt>{{ t('drawAt') }}</dt><dd>{{ formatTime(period.draw_at) }}</dd></div><div v-if="showPeriodResults"><dt>{{ t('result') }}</dt><dd v-if="period.result">{{ [...period.result.regular, ...period.result.special, ...period.result.digits].join(', ') }}</dd><dd v-else><span>{{ t('pending') }}</span></dd></div></dl></article><p v-if="!detail.periods.length" class="empty-state">{{ t('empty') }}</p></div>
          <template #pagination><TablePagination class="wallet-pagination page-actions" :total="detail.total_count" :page="Math.floor(periodOffset / periodPageSize + 1)" :count="detail.periods.length" :locale="locale" :page-size="periodPageSize" :previous-disabled="loading || periodOffset === 0" :next-disabled="loading || detail.periods.length < periodPageSize" :previous-label="t('previous')" :next-label="t('next')" :go-to="(target: number) => open(detail!.game, view, (target - 1) * periodPageSize)" :change-page-size="async (size: number) => { periodPageSize = size; await open(detail!.game, view, 0) }" :busy="loading" /></template></ManagementList>
        </template>
        <template v-else-if="view === 'plays'">
          <ConfiguredRuleWorkbench ref="rulesWorkbench" v-if="surface !== 'user'" :surface="surface" :brand-id="brandId" :game-id="detail.game.id" :model="detail.game.model" :locale="locale" :can-write="canManagePlatform" @failure="emit('failure',$event)" />

        </template>
        <template v-else-if="view === 'model'"><button v-if="canManagePlatform" class="button button-primary" :disabled="loading || modelLocked" @click="begin('model')">{{ t('editModel') }}</button><p v-if="canManagePlatform && modelLocked" class="platform-lottery-note">{{ t('modelLocked') }}</p><NumberModelEditor :model-value="detail.game.model" :locale="locale" disabled read-only /></template>
      </template>
    </component>
    <DetailModal class="brand-sales-modal" v-if="surface==='admin'&&salesGame" :class="{ 'brand-odds-modal': oddsOnly }" :title="`${salesGame.name} · ${t(oddsOnly?'salesOdds':'salesConfiguration')}`" :close-label="t('close')" :close-disabled="salesLocked" @close="salesGame=null">
      <ConfiguredPublishing :odds-only="oddsOnly" surface="admin" :brand-id="brandId" :game-id="salesGame.id" :locale="locale" :snapshot="null" :can-write="!!canSell" @locked="salesLocked=$event" @saved="withGlobalLoading(load)" @failure="emit('failure',$event)" />
    </DetailModal>
    <PeriodOpeningDialog v-if="openingPeriod" :key="openingPeriod.id" :period="openingPeriod" :locale="locale" @close="openingPeriod=null" @saved="openingPeriod=null;withGlobalLoading(open,detail!.game,view,periodOffset)" @failure="emit('failure',$event)" />
    <DetailModal v-if="formKind" :class="{ 'platform-lottery-confirm-modal': isConfirmation, 'ui-form-dialog': formKind === 'result' }" :title="dialogTitle" :close-label="t('close')" :close-disabled="loading||!!pending" @close="!loading && !pending && (formKind = null)">
      <form v-write-guard="loading" novalidate v-inline-validation class="brand-form platform-lottery-form ui-form" @submit.prevent="withGlobalLoading(submit)"><fieldset :class="{ 'confirmation-form-fields': isConfirmation, 'period-form-fields': formKind === 'period', 'result-form-fields': formKind === 'result' }" :disabled="loading || !!pending">
        <div v-if="isConfirmation" class="platform-lottery-confirm-copy"><p class="platform-lottery-confirm-prompt">{{ t('gameActionPrompt', { action: actionLabel, name: target && 'name' in target ? target.name : '' }) }}</p><p v-if="actionHint" class="platform-lottery-note">{{ actionHint }}</p></div>
        <label v-if="formKind === 'name'">{{ t('name') }}<input v-model.trim="form.name" required maxlength="120" /></label><template v-if="formKind === 'game'"><label>{{ t('name') }}<input v-model.trim="form.name" required maxlength="120" /></label><label>{{ t('code') }}<input v-model.trim="form.code" pattern="[a-z][a-z0-9_]{0,47}" :data-validation-format="formValidationText('codeFormat', {min: 1, max: 48})" required /></label></template>
        <template v-if="formKind === 'game'"><label>{{ t('timezone') }}<SelectControl v-model="form.timezone" class="lottery-timezone" required :label="String(t('timezone'))" :options="(timezones).map((zone) => ({ value: zone, label: (String(zone)).trim() }))" /></label><NumberModelEditor v-model="modelDraft" :locale="locale" :disabled="loading || !!pending" /></template><NumberModelEditor v-if="formKind === 'model'" v-model="modelDraft" :locale="locale" :disabled="loading || !!pending" />
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
        <template v-if="formKind === 'result'">
          <label>{{ t('periodNo') }}<input v-model.trim="form.periodNo" required maxlength="100" /></label>
          <label>{{ t('drawDate') }}<PlatformDateInput v-model="form.drawDate" :display-settings="displaySettings!" :locale="locale" :label="t('drawDate')" editable required /></label>
          <DrawResultInputs v-if="resultModel" ref="drawInputs" :model="resultModel" :locale="locale" />
        </template>

      </fieldset><p v-if="pending">{{ t('awaiting') }}</p><div class="platform-lottery-form-actions ui-form-actions" :class="{ 'confirmation-actions': isConfirmation }"><button class="button" :class="formKind === 'lifecycle' && lifecycleAction === 'deleted' ? 'button-danger' : 'button-primary'" :disabled="loading" type="submit">{{ pending ? t('retry') : isConfirmation ? actionLabel : t('save') }}</button></div></form>
    </DetailModal>
  </section>
</template>

<style scoped>
.brand-sales-modal.detail-modal{width:min(640px,calc(100vw - 24px))}
.brand-sales-modal.brand-odds-modal{width:min(760px,calc(100vw - 24px))}
.brand-sales-modal :deep(.configured-publishing>h3){display:none}

.lottery-detail-page{min-width:0;padding:16px}.lottery-detail-page-heading{display:flex;align-items:center;flex-wrap:wrap;gap:12px;margin-bottom:16px}.lottery-detail-page-heading h2{margin:0;overflow-wrap:anywhere;font-size:18px}.lottery-detail-page-heading .button{min-height:var(--control-height)}
.platform-lottery{text-align:left}
.platform-lottery-toolbar{display:flex;align-items:center;flex-wrap:wrap;gap:12px;padding:12px 16px;border-bottom:1px solid var(--line)}
.platform-lottery-toolbar h2{margin:0 auto 0 0}
.platform-lottery-toolbar .platform-lottery-filter{margin:0;flex-wrap:wrap;min-width:0}
.platform-lottery-toolbar > .button{margin-left:auto;min-height:var(--control-height)}
.platform-lottery .row-action{appearance:none;border:0;background:transparent;color:var(--primary);font:inherit;font-size:var(--body-font-size);font-weight:650;line-height:1.35;padding:6px;border-radius:var(--radius-sm);cursor:pointer}
.platform-lottery .row-action:hover:not(:disabled){background:var(--primary-light)}
.platform-lottery .row-action:disabled{opacity:.45;cursor:not-allowed}
.platform-lottery { min-width: 0; margin-bottom: 16px; }
.platform-lottery-note { padding: 0 16px; font-size: var(--body-font-size); color: var(--muted); }
.platform-lottery-actions { display: flex; flex-wrap: wrap; gap: 8px; }
.platform-lottery-form fieldset { border: 0; padding: 0; margin: 0; display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 300px), 1fr)); gap: 12px 20px; width: 100%; box-sizing: border-box; min-width: 0; }
.platform-lottery-form .period-form-fields { grid-template-columns: minmax(0, 1fr); }
.platform-lottery-form .result-form-fields > label > input { width: 100%; }
.detail-modal.platform-lottery-confirm-modal { width: min(480px, calc(100vw - 24px)); max-width: calc(100vw - 24px); }
.platform-lottery-form .confirmation-form-fields { grid-template-columns: minmax(0, 1fr); }
.platform-lottery-confirm-copy { display: grid; gap: 8px; min-width: 0; }
.platform-lottery-confirm-prompt { margin: 0; font-size: 16px; font-weight: 650; line-height: 1.6; overflow-wrap: anywhere; }
.platform-lottery-confirm-copy .platform-lottery-note { margin: 0; padding: 0; font-size: var(--body-font-size); line-height: 1.6; }
.platform-lottery-form-actions { display: flex; flex-wrap: wrap; gap: 10px; margin-top: 14px; grid-column: 1 / -1; }
.platform-lottery-form-actions.confirmation-actions { justify-content: flex-end; }
.platform-lottery-form .platform-lottery-form-actions > button { width: auto; margin: 0; min-height: var(--control-height); box-sizing:border-box; padding-block:4px; line-height:22px; }
.platform-lottery-form .play-form-fields { grid-template-columns: repeat(2, minmax(0, 1fr)); align-items: start; }
.platform-lottery-form .play-form-fields label { align-content: start; }
.platform-lottery-form .play-form-fields input { width: 100%; min-height: 40px; }
.platform-lottery-form .number-model-editor, .lottery-form-reason { grid-column: 1 / -1; }
.lottery-form-reason { width: 100%; max-width: 400px; }
.platform-lottery-form label, .platform-lottery-form .period-field { display: grid; grid-template-columns: minmax(0, 1fr); min-width: 0; gap: 4px; font-size: var(--body-font-size); }
.platform-lottery-form input, .platform-lottery-form textarea { box-sizing: border-box; min-width: 0; max-width: 100%; font: inherit; padding: 8px; border: 1px solid var(--line); border-radius: var(--radius-sm); background: var(--surface); color: var(--ink); }
.platform-lottery-form input { width: 300px; }
.platform-lottery-form .period-field input { width: min(100%, 420px); }
.platform-lottery-form .period-time-controls { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 12px; max-width: 852px; }
.platform-lottery-form .period-time-controls input { width: 100%; }
.platform-lottery-form textarea { width: 100%; }
.platform-lottery-form > button { justify-self: start; width: auto; margin-top: 14px; }
.platform-lottery pre { white-space: pre-wrap; overflow-wrap: anywhere; font-size: var(--body-font-size); }
.platform-lottery td { overflow-wrap: anywhere; }
.platform-lottery-period-cards { display: none; }
@media (max-width: 640px) {
  .platform-lottery-form .play-form-fields { grid-template-columns: minmax(0, 1fr); }
  .platform-lottery-form .period-time-controls { grid-template-columns: minmax(0, 1fr); }
  .platform-lottery-period-table { display: none; }
  .platform-lottery-period-cards { display: grid; gap: 12px; }
  .platform-lottery-period-cards article { padding: 12px; }
  .platform-lottery-period-cards dl div { display: grid; grid-template-columns: 80px minmax(0, 1fr); gap: 8px; font-size: var(--body-font-size); margin: 8px 0; }
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
.platform-lottery-toolbar > .button{margin-left:auto;min-height:var(--control-height)}
.platform-lottery .row-action{appearance:none;border:0;background:transparent;color:var(--primary);font:inherit;font-size:var(--body-font-size);font-weight:650;line-height:1.35;padding:6px;border-radius:var(--radius-sm);cursor:pointer}
.platform-lottery .row-action:hover:not(:disabled){background:var(--primary-light)}
.platform-lottery .row-action:disabled{opacity:.45;cursor:not-allowed}
.platform-lottery-filter{display:flex;align-items:center;gap:8px;margin:8px 0;font-size:var(--body-font-size)}.platform-lottery-actions{display:flex;flex-wrap:wrap;gap:6px}
@media(max-width:900px){
.platform-lottery-catalog-table th:first-child,.platform-lottery-catalog-table td:first-child,
.platform-lottery-catalog-table th:last-child,.platform-lottery-catalog-table td:last-child{position:static}
}
@media(max-width:620px){.platform-lottery-actions button{min-height:44px}
.platform-lottery-catalog-table{display:block;min-width:0!important;width:100%;table-layout:auto}
.platform-lottery-catalog-table thead{display:none}
.platform-lottery-catalog-table tbody{display:block;width:100%}
.platform-lottery-catalog-table tr{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:4px 8px;margin:8px 0;padding:10px;border:1px solid var(--line);border-radius:var(--radius-sm);background:var(--surface)}
.platform-lottery-catalog-table td:not(.empty-state){display:flex;align-items:center;flex-wrap:wrap;gap:6px;min-width:0;height:auto!important;min-height:0;padding:2px;border:0;white-space:normal;overflow-wrap:anywhere}
.platform-lottery-catalog-table td:first-child,.platform-lottery-catalog-table td:nth-child(2),.platform-lottery-catalog-table td:last-child{grid-column:1/-1}
.platform-lottery-catalog-table td:first-child{font-weight:750;font-size:14px}
.platform-lottery-catalog-table td:nth-child(2){font-size:var(--body-font-size);color:var(--muted)}
.platform-lottery-catalog-table button{min-height:44px}
}
</style>
