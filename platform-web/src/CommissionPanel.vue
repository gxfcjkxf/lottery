<script setup lang="ts">
import DirectorySearch from "@lottery/shared/DirectorySearch.vue"
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { vInlineValidation } from '@lottery/shared'
import { usePlatformI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from "@lottery/shared";
import { usePlatformI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
const displayDate = (value: string | null | undefined) => formatDisplayDateTime(value, props.locale);
import { formatDisplayDateTime } from "@lottery/shared";
import { usePlatformI18n } from "./i18n"
const ui = usePlatformI18n().t
import MemberActionsMenu from '@lottery/shared/MemberActionsMenu.vue'
import TablePagination from '@lottery/shared/TablePagination.vue'
import DetailModal from "@lottery/shared/DetailModal.vue"
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { PlatformApiError } from './platform-api'
import { createPlatformCommissionApi, type AgentDirectoryNode, type AgentNode, type Cycle, type Earning, type Payment } from './commission-api'
import { formatPlatformDateTime, type PlatformDisplaySettings } from './platform-format'

const props = defineProps<{ brandId: string; locale: 'en' | 'zh-CN'; initialTab?: 'agents' | 'cycles' | 'payments'; displaySettings: PlatformDisplaySettings }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformCommissionApi()
const pageSize = 50
type Tab = 'agents' | 'cycles' | 'payments'
const activeTab = ref<Tab>(props.initialTab ?? 'agents')
const copy = computed(() => props.locale === 'en' ? {
  title: ui('platform.copy.68a797db'), agents: ui('platform.copy.3705c4b4'), cycles: ui('platform.copy.39ac5c23'), payments: ui('platform.copy.48f3ae6f'),
  refresh: ui('platform.copy.256fe8b3'), previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), total: ui('platform.copy.4c2a666c'), emptyAgents: ui('platform.copy.dd9037fd'), emptyCycles: ui('platform.copy.5184105f'), emptyPayments: ui('platform.copy.bfeb2432'), close: ui('platform.copy.27b2fd29'),
  agent: ui('platform.copy.e592fcdf'), member: ui('platform.copy.da8480f3'), parent: ui('platform.copy.366cbe5d'), path: ui('platform.copy.930ba259'), ratio: ui('platform.copy.da3069d9'), mode: ui('platform.copy.40a029ce'), effectiveMode: ui('platform.copy.cfd857b4'), status: ui('platform.copy.c7260510'), version: ui('platform.copy.59405491'), depth: ui('platform.copy.04925e69'),
  cycle: ui('platform.copy.f6f6ceea'), window: ui('platform.copy.42d37cd0'), state: ui('platform.copy.274ae047'), currentRun: ui('platform.copy.abe92a85'), evidence: ui('platform.copy.6d8802d6'), targets: ui('platform.copy.748cd210'), calculated: ui('platform.copy.9dbac0c2'), earnings: ui('platform.copy.459c9006'), totalPoints: ui('platform.copy.7d4b9eb8'), scanComplete: ui('platform.copy.afc97bb3'),
  payment: ui('platform.copy.48d7d322'), payoutMode: ui('platform.copy.51fcc28f'), paidPoints: ui('platform.copy.8418f8d4'), paidCount: ui('platform.copy.9b470917'), created: ui('platform.copy.00e7cdbe'), updated: ui('platform.copy.b8b9d3d6'), errorCode: ui('platform.copy.8a054f73'),
  detail: ui('platform.copy.433de451'), earningDetails: ui('platform.copy.bab417ff'), exact: ui('platform.copy.368b2914'), points: ui('platform.copy.14efea21'), calculatedNotice: ui('platform.copy.9d4334fd'), noRun: ui('platform.copy.bc6d10ef'), noEarnings: ui('platform.copy.f3df1495'), evidenceCurrent: ui('platform.copy.54c18cc7'), evidenceStale: ui('platform.copy.2067b3c7'), yes: ui('platform.copy.ab57ddad'), no: ui('platform.copy.502adcd8'), inherited: ui('platform.copy.b3b6dad9'),
  states: { active: ui('platform.commission.state.active'), disabled: ui('platform.commission.state.disabled'), enumerating: ui('platform.commission.state.enumerating'), waiting: ui('platform.commission.state.waiting'), calculating: ui('platform.commission.state.calculating'), summarizing: ui('platform.commission.state.summarizing'), ready: ui('platform.commission.state.ready'), failed: ui('platform.commission.state.failed'), awaiting_approval: ui('platform.commission.state.awaitingApproval'), paying: ui('platform.commission.state.paying'), paid: ui('platform.commission.state.paid'), stale: ui('platform.commission.state.stale'), blocked: ui('platform.commission.state.blocked') } as Record<string, string>,
  modes: { loss: ui('platform.commission.mode.loss'), turnover: ui('platform.commission.mode.turnover'), manual: ui('platform.commission.mode.manual'), automatic: ui('platform.commission.mode.automatic'), mixed: ui('platform.commission.mode.mixed'), none: ui('platform.commission.mode.none') } as Record<string, string>,
} : {
  title: ui('platform.copy.68a797db'), agents: ui('platform.copy.3705c4b4'), cycles: ui('platform.copy.39ac5c23'), payments: ui('platform.copy.48f3ae6f'),
  refresh: ui('platform.copy.256fe8b3'), previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), total: ui('platform.copy.4c2a666c'), emptyAgents: ui('platform.copy.dd9037fd'), emptyCycles: ui('platform.copy.5184105f'), emptyPayments: ui('platform.copy.bfeb2432'), close: ui('platform.copy.27b2fd29'),
  agent: ui('platform.copy.e592fcdf'), member: ui('platform.copy.da8480f3'), parent: ui('platform.copy.366cbe5d'), path: ui('platform.copy.930ba259'), ratio: ui('platform.copy.da3069d9'), mode: ui('platform.copy.40a029ce'), effectiveMode: ui('platform.copy.cfd857b4'), status: ui('platform.copy.c7260510'), version: ui('platform.copy.59405491'), depth: ui('platform.copy.04925e69'),
  cycle: ui('platform.copy.f6f6ceea'), window: ui('platform.copy.42d37cd0'), state: ui('platform.copy.274ae047'), currentRun: ui('platform.copy.abe92a85'), evidence: ui('platform.copy.6d8802d6'), targets: ui('platform.copy.748cd210'), calculated: ui('platform.copy.9dbac0c2'), earnings: ui('platform.copy.459c9006'), totalPoints: ui('platform.copy.7d4b9eb8'), scanComplete: ui('platform.copy.afc97bb3'),
  payment: ui('platform.copy.48d7d322'), payoutMode: ui('platform.copy.51fcc28f'), paidPoints: ui('platform.copy.8418f8d4'), paidCount: ui('platform.copy.9b470917'), created: ui('platform.copy.00e7cdbe'), updated: ui('platform.copy.b8b9d3d6'), errorCode: ui('platform.copy.8a054f73'),
  detail: ui('platform.copy.433de451'), earningDetails: ui('platform.copy.bab417ff'), exact: ui('platform.copy.368b2914'), points: ui('platform.copy.14efea21'), calculatedNotice: ui('platform.copy.9d4334fd'), noRun: ui('platform.copy.bc6d10ef'), noEarnings: ui('platform.copy.f3df1495'), evidenceCurrent: ui('platform.copy.54c18cc7'), evidenceStale: ui('platform.copy.2067b3c7'), yes: ui('platform.copy.ab57ddad'), no: ui('platform.copy.502adcd8'), inherited: ui('platform.copy.b3b6dad9'),
  states: { active: ui('platform.commission.state.active'), disabled: ui('platform.commission.state.disabled'), enumerating: ui('platform.commission.state.enumerating'), waiting: ui('platform.commission.state.waiting'), calculating: ui('platform.commission.state.calculating'), summarizing: ui('platform.commission.state.summarizing'), ready: ui('platform.commission.state.ready'), failed: ui('platform.commission.state.failed'), awaiting_approval: ui('platform.commission.state.awaitingApproval'), paying: ui('platform.commission.state.paying'), paid: ui('platform.commission.state.paid'), stale: ui('platform.commission.state.stale'), blocked: ui('platform.commission.state.blocked') } as Record<string, string>,
  modes: { loss: ui('platform.commission.mode.loss'), turnover: ui('platform.commission.mode.turnover'), manual: ui('platform.commission.mode.manual'), automatic: ui('platform.commission.mode.automatic'), mixed: ui('platform.commission.mode.mixed'), none: ui('platform.commission.mode.none') } as Record<string, string>,
})

const agents = ref<AgentDirectoryNode[]>([])
const search = ref(''), query = ref('')
const onlineFilter = ref<'' | 'true' | 'false'>(''), onlineQuery = ref<'' | 'true' | 'false'>('')
async function searchAgents() { query.value = search.value.trim(); onlineQuery.value = onlineFilter.value; await loadList('agents', 0) }
async function clearSearch() { search.value = ''; onlineFilter.value = ''; await searchAgents() }
const cycles = ref<Cycle[]>([])
const payments = ref<Payment[]>([])
const agentTotal = ref('0')
const cycleTotal = ref('0')
const paymentTotal = ref('0')
const agentOffset = ref(0)
const cycleOffset = ref(0)
const paymentOffset = ref(0)
const agentDetail = ref<AgentNode | null>(null)
const cycleDetail = ref<Cycle | null>(null)
const paymentDetail = ref<Payment | null>(null)
const earningItems = ref<Earning[]>([])
const earningOffset = ref(0)
const earningTotal = ref('0')
const loading = ref(false)
const error = ref<string | import("@lottery/shared").LocalizedMessage>("")
let generation = 0

const offset = computed(() => activeTab.value === 'agents' ? agentOffset.value : activeTab.value === 'cycles' ? cycleOffset.value : paymentOffset.value)
const total = computed(() => activeTab.value === 'agents' ? agentTotal.value : activeTab.value === 'cycles' ? cycleTotal.value : paymentTotal.value)
const hasNext = computed(() => BigInt(offset.value + pageSize) < BigInt(total.value))
const hasNextEarnings = computed(() => BigInt(earningOffset.value + pageSize) < BigInt(earningTotal.value))
const emptyLabel = computed(() => activeTab.value === 'agents' ? copy.value.emptyAgents : activeTab.value === 'cycles' ? copy.value.emptyCycles : copy.value.emptyPayments)
const stateLabel = (state: string) => copy.value.states[state] || state
const modeLabel = (mode: string | null) => mode === null ? copy.value.inherited : copy.value.modes[mode] || mode

function report(cause: unknown) {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  else error.value = cause instanceof Error ? publicErrorMessage(cause) : publicErrorMessage(cause)
}

function clearDetails() {
  agentDetail.value = null
  cycleDetail.value = null
  paymentDetail.value = null
  earningItems.value = []; earningOffset.value = 0; earningTotal.value = '0'
}

function clearBrandState() {
  search.value = ''; query.value = ''; onlineFilter.value = ''; onlineQuery.value = '';
  agents.value = []; cycles.value = []; payments.value = []
  agentTotal.value = '0'; cycleTotal.value = '0'; paymentTotal.value = '0'
  agentOffset.value = 0; cycleOffset.value = 0; paymentOffset.value = 0
  clearDetails(); error.value = ''
}

async function loadList(tab: Tab, nextOffset: number) {
  const request = ++generation
  const brandId = props.brandId
  activeTab.value = tab
  clearDetails()
  error.value = ''
  loading.value = true
  try {
    if (tab === 'agents') {
      agents.value = []
      const page = await api.list(brandId, pageSize, nextOffset, { query: query.value, online: onlineQuery.value })
      if (request !== generation || props.brandId !== brandId) return
      agents.value = page.items; agentOffset.value = nextOffset; agentTotal.value = page.total_count
    } else if (tab === 'cycles') {
      cycles.value = []
      const page = await api.cycles(brandId, pageSize, nextOffset)
      if (request !== generation || props.brandId !== brandId) return
      cycles.value = page.items; cycleOffset.value = nextOffset; cycleTotal.value = page.total_count
    } else {
      payments.value = []
      const page = await api.payments(brandId, pageSize, nextOffset)
      if (request !== generation || props.brandId !== brandId) return
      payments.value = page.items; paymentOffset.value = nextOffset; paymentTotal.value = page.total_count
    }
  } catch (cause) {
    if (request === generation && props.brandId === brandId) report(cause)
  } finally {
    if (request === generation) loading.value = false
  }
}

async function refresh() { await loadList(activeTab.value, 0) }
function switchTab(tab: Tab) { if (tab !== activeTab.value) void loadList(tab, tab === 'agents' ? agentOffset.value : tab === 'cycles' ? cycleOffset.value : paymentOffset.value) }

async function openAgent(agent: AgentNode) {
  const request = ++generation
  const brandId = props.brandId
  clearDetails()
  error.value = ''; loading.value = true
  try {
    const detail = await api.agent(brandId, agent.id)
    if (request === generation && props.brandId === brandId) agentDetail.value = detail
  } catch (cause) { if (request === generation && props.brandId === brandId) report(cause) }
  finally { if (request === generation) loading.value = false }
}

async function openCycle(cycle: Cycle) {
  const request = ++generation
  const brandId = props.brandId
  clearDetails()
  error.value = ''; loading.value = true
  try {
    const detail = await api.cycle(brandId, cycle.id)
    if (request !== generation || props.brandId !== brandId) return
    cycleDetail.value = detail
    if (detail.current_run_id) {
      const page = await api.earnings(brandId, detail.id, pageSize, 0)
      if (request !== generation || props.brandId !== brandId) return
      if (page.items.some(item => item.run_id !== detail.current_run_id)) throw new PlatformApiError('Commission run changed; refresh the cycle', 0, 'RUN_CHANGED')
      earningItems.value = page.items; earningOffset.value = 0; earningTotal.value = page.total_count
    }
  } catch (cause) { if (request === generation && props.brandId === brandId) report(cause) }
  finally { if (request === generation) loading.value = false }
}

async function loadEarnings(nextOffset: number) {
  if (!cycleDetail.value?.current_run_id) return
  const request = ++generation
  const brandId = props.brandId
  const cycleId = cycleDetail.value.id
  earningItems.value = []; error.value = ''; loading.value = true
  try {
    const page = await api.earnings(brandId, cycleId, pageSize, nextOffset)
    if (request === generation && props.brandId === brandId && cycleDetail.value?.id === cycleId) {
      if (page.items.some(item => item.run_id !== cycleDetail.value?.current_run_id)) throw new PlatformApiError('Commission run changed; refresh the cycle', 0, 'RUN_CHANGED')
      earningItems.value = page.items; earningOffset.value = nextOffset; earningTotal.value = page.total_count
    }
  } catch (cause) { if (request === generation && props.brandId === brandId) report(cause) }
  finally { if (request === generation) loading.value = false }
}

async function openPayment(payment: Payment) {
  const request = ++generation
  const brandId = props.brandId
  clearDetails()
  error.value = ''; loading.value = true
  try {
    const detail = await api.payment(brandId, payment.id)
    if (request === generation && props.brandId === brandId) paymentDetail.value = detail
  } catch (cause) { if (request === generation && props.brandId === brandId) report(cause) }
  finally { if (request === generation) loading.value = false }
}

watch(() => props.brandId, () => {
  generation++
  clearBrandState()
  if (props.brandId) void withGlobalLoading(loadList, activeTab.value, 0)
  else loading.value = false
}, { immediate: true, flush: 'sync' })
watch(() => props.initialTab, value => { if (value && value !== activeTab.value) switchTab(value) }, { flush: 'sync' })
onBeforeUnmount(() => { generation++ })
</script>

<template>
  <div v-if="brandId" data-testid="platform-commission">
    <p v-if="error" class="message error global-message" role="alert">{{ displayPublicError(error) }}</p>
    <section class="panel console-panel console-stack">
      <DirectorySearch v-if="activeTab==='agents'" v-model="search" :label="ui('members.search')" :query-label="ui('members.query')" :clear-label="ui('members.clear')" :busy="loading" :active="Boolean(search||onlineFilter)" :search="searchAgents" :clear="clearSearch"><select v-model="onlineFilter" :aria-label="ui('members.onlineStatus')"><option value="">{{ui('members.allOnline')}}</option><option value="true">{{ui('members.online')}}</option><option value="false">{{ui('members.offline')}}</option></select></DirectorySearch>
      <div class="table-wrap" data-testid="platform-commission-list">
        <table v-if="activeTab === 'agents'" class="agent-directory-table console-table console-responsive-table"><thead><tr><th>{{ ui('platform.copy.eeaa7275') }}</th><th>{{ ui('members.onlineStatus') }}</th><th>{{ ui('common.points') }}</th><th>{{ copy.depth }}</th><th>{{ copy.parent }}</th><th>{{ copy.ratio }}</th><th>{{ copy.effectiveMode }}</th><th><BusinessDetails :value="copy.status" :locale="associationLocale" /></th><th>{{ ui('common.actions') }}</th></tr></thead><tbody>
          <tr v-for="agent in agents" :key="agent.id"><td :data-label="ui('platform.copy.eeaa7275')"><button type="button" class="text-button member-account" @click="withGlobalLoading(openAgent, agent)">{{ agent.username || ui('association.unnamed') }}</button></td><td :data-label="ui('members.onlineStatus')"><span class="member-presence" :class="{ 'is-online': agent.online }"><i aria-hidden="true" />{{ ui(agent.online ? 'members.online' : 'members.offline') }}</span></td><td class="member-points" :data-label="ui('common.points')">{{ agent.display_points === null ? ui('members.pointsRestricted') : agent.display_points }}</td><td :data-label="copy.depth">{{ agent.depth }}</td><td :data-label="copy.parent"><AssociationName :record="agent" kind="agent" :id="agent.parent_id ?? undefined" :locale="associationLocale" /></td><td :data-label="copy.ratio">{{ agent.config.ratio }}</td><td :data-label="copy.effectiveMode">{{ modeLabel(agent.effective_mode) }}</td><td :data-label="copy.status"><span :class="['status-pill', agent.config.status]">{{ stateLabel(agent.config.status) }}</span></td><td :data-label="ui('common.actions')"><MemberActionsMenu :id="`platform-agent-actions-${agent.id}`" :label="ui('agents.actions', { account: agent.username })"><button type="button" role="menuitem" @click="withGlobalLoading(openAgent, agent)">{{ copy.detail }}</button></MemberActionsMenu></td></tr>
          <tr v-if="!agents.length"><td colspan="9" class="empty-state">{{ emptyLabel }}</td></tr>
        </tbody></table>
        <table v-else-if="activeTab === 'cycles'"><thead><tr><th>{{ copy.cycle }}</th><th>{{ copy.window }}</th><th><BusinessDetails :value="copy.state" :locale="associationLocale" /></th><th>{{ copy.evidence }}</th><th>{{ copy.calculated }} / {{ copy.targets }}</th><th>{{ copy.totalPoints }}</th></tr></thead><tbody>
          <tr v-for="cycle in cycles" :key="cycle.id"><td><button class="row-action mono" @click="withGlobalLoading(openCycle, cycle)"><AssociationName :record="cycle" kind="cycle" :id="cycle.id" :locale="associationLocale" /></button></td><td>{{ formatPlatformDateTime(cycle.window_from, props.displaySettings, props.locale) }}<small>{{ formatPlatformDateTime(cycle.window_to, props.displaySettings, props.locale) }}</small></td><td><span :class="['status-pill', cycle.state]">{{ stateLabel(cycle.state) }}</span></td><td>{{ cycle.evidence_current ? copy.evidenceCurrent : copy.evidenceStale }}</td><td>{{ cycle.calculated_count }} / {{ cycle.target_count }}</td><td>{{ cycle.total_points }}</td></tr>
          <tr v-if="!cycles.length"><td colspan="6" class="empty-state">{{ emptyLabel }}</td></tr>
        </tbody></table>
        <table v-else><thead><tr><th>{{ copy.payment }}</th><th>{{ copy.cycle }}</th><th><BusinessDetails :value="copy.state" :locale="associationLocale" /></th><th>{{ copy.payoutMode }}</th><th>{{ copy.paidPoints }} / {{ copy.totalPoints }}</th><th>{{ copy.paidCount }}</th></tr></thead><tbody>
          <tr v-for="payment in payments" :key="payment.id"><td><button class="row-action mono" @click="withGlobalLoading(openPayment, payment)">{{ payment.id }}</button><small>{{ formatPlatformDateTime(payment.created_at, props.displaySettings, props.locale) }}</small></td><td class="mono"><AssociationName :record="payment" kind="cycle" :id="payment.cycle_id" :locale="associationLocale" /></td><td><span :class="['status-pill', payment.state]">{{ stateLabel(payment.state) }}</span></td><td>{{ modeLabel(payment.payout_mode) }}</td><td>{{ payment.paid_points }} / {{ payment.total_points }}</td><td>{{ payment.paid_count }} / {{ payment.target_count }}</td></tr>
          <tr v-if="!payments.length"><td colspan="6" class="empty-state">{{ emptyLabel }}</td></tr>
        </tbody></table>
      </div>
      <TablePagination v-if="activeTab === 'agents'" data-testid="platform-commission-list-pages" :summary="ui('agents.page', { page: String(Math.floor(agentOffset / pageSize) + 1), count: String(agents.length), total: agentTotal })"><template #actions><button type="button" class="button button-secondary" :disabled="loading || agentOffset === 0" @click="withGlobalLoading(loadList, 'agents', agentOffset - pageSize)">{{ copy.previous }}</button><button type="button" class="button button-secondary" :disabled="loading || !hasNext" @click="withGlobalLoading(loadList, 'agents', agentOffset + pageSize)">{{ copy.next }}</button></template></TablePagination>
      <div v-else class="wallet-pagination" data-testid="platform-commission-list-pages">
        <span>{{ copy.total }} {{ total }}</span>
        <button class="button button-secondary" :disabled="loading || offset === 0 || !brandId" @click="withGlobalLoading(loadList, activeTab, offset - pageSize)">{{ copy.previous }}</button>
        <span>{{ Math.floor(offset / pageSize) + 1 }}</span>
        <button class="button button-secondary" :disabled="loading || !hasNext" @click="withGlobalLoading(loadList, activeTab, offset + pageSize)">{{ copy.next }}</button>
        <button v-refresh-button="copy.refresh" class="button button-secondary" :disabled="loading || !brandId" @click="withGlobalLoading(refresh)">{{ copy.refresh }}</button>
      </div>
    </section>

    <DetailModal class="agent-dialog" v-if="agentDetail" :title="`${copy.detail} · ${copy.agent}`" :close-label="copy.close" @close="agentDetail = null">
<section v-if="agentDetail" class="detail-section" data-testid="platform-commission-details">
      <div class="detail-summary"><span class="count-chip">{{ stateLabel(agentDetail.config.status) }}</span></div>
      <dl v-if="agentDetail" class="confirm-list reward-fields wallet-details">
        <div><dt>{{ copy.agent }}</dt><dd class="mono"><AssociationName :record="agentDetail" kind="agent" :id="agentDetail.id" :locale="associationLocale" /></dd></div><div><dt>{{ copy.parent }}</dt><dd class="mono"><AssociationName :record="agentDetail" kind="agent" :id="agentDetail.parent_id ?? undefined" :locale="associationLocale" /></dd></div>
        <div><dt>{{ copy.path }}</dt><dd class="mono"><span v-for="(ancestor,index) in agentDetail.path" :key="ancestor"><span v-if="index"> → </span><AssociationName :record="agentDetail" kind="agent" :id="ancestor" :locale="associationLocale" /></span></dd></div><div><dt>{{ copy.depth }}</dt><dd>{{ agentDetail.depth }}</dd></div><div><dt>{{ copy.ratio }}</dt><dd>{{ agentDetail.config.ratio }}</dd></div>
        <div><dt>{{ copy.mode }}</dt><dd>{{ modeLabel(agentDetail.config.mode) }}</dd></div><div><dt>{{ copy.effectiveMode }}</dt><dd>{{ modeLabel(agentDetail.effective_mode) }}</dd></div><div><dt><BusinessDetails :value="copy.status" :locale="associationLocale" /></dt><dd>{{ stateLabel(agentDetail.config.status) }}</dd></div>
      </dl>
    </section>
</DetailModal>

    <DetailModal v-if="cycleDetail" :title="`${copy.detail} · ${copy.cycle}`" :close-label="copy.close" @close="cycleDetail = null">
<section v-if="cycleDetail" class="detail-section" data-testid="platform-commission-details">
      <div class="detail-summary"><span :class="['status-pill', cycleDetail.state]">{{ stateLabel(cycleDetail.state) }}</span></div>
      <dl v-if="cycleDetail" class="confirm-list reward-fields wallet-details">
        <div><dt>{{ copy.cycle }}</dt><dd class="mono"><AssociationName :record="cycleDetail" kind="cycle" :id="cycleDetail.id" :locale="associationLocale" /></dd></div><div><dt>{{ copy.window }}</dt><dd>{{ displayDate(cycleDetail.window_from) }} → {{ displayDate(cycleDetail.window_to) }}</dd></div><div><dt><BusinessDetails :value="copy.state" :locale="associationLocale" /></dt><dd>{{ stateLabel(cycleDetail.state) }}</dd></div>
        <div><dt>{{ copy.currentRun }}</dt><dd class="mono">{{ cycleDetail.current_run_id || '—' }}</dd></div><div><dt>{{ copy.evidence }}</dt><dd>{{ cycleDetail.evidence_current ? copy.evidenceCurrent : copy.evidenceStale }}</dd></div>
        <div><dt>{{ copy.targets }}</dt><dd>{{ cycleDetail.target_count }}</dd></div><div><dt>{{ copy.scanComplete }}</dt><dd>{{ cycleDetail.scan_complete ? copy.yes : copy.no }}</dd></div><div><dt>{{ copy.calculated }}</dt><dd>{{ cycleDetail.calculated_count }}</dd></div>
        <div><dt>{{ copy.earnings }}</dt><dd>{{ cycleDetail.earning_count }}</dd></div><div><dt>{{ copy.totalPoints }}</dt><dd>{{ cycleDetail.total_points }}</dd></div><div><dt>{{ copy.created }}</dt><dd>{{ formatPlatformDateTime(cycleDetail.created_at, props.displaySettings, props.locale) }}</dd></div><div><dt>{{ copy.updated }}</dt><dd>{{ formatPlatformDateTime(cycleDetail.updated_at, props.displaySettings, props.locale) }}</dd></div>
      </dl>
      <div v-if="cycleDetail" class="panel-heading"><div><h2>{{ copy.earningDetails }}</h2><p>{{ cycleDetail.current_run_id || copy.noRun }}</p></div><span v-if="cycleDetail.current_run_id" class="count-chip">{{ copy.total }} {{ earningTotal }}</span></div>
      <template v-if="cycleDetail?.current_run_id">
        <p class="message">{{ copy.calculatedNotice }}</p>
        <div class="table-wrap"><table><thead><tr><th>{{ copy.agent }}</th><th>{{ copy.member }}</th><th>{{ copy.exact }}</th><th>{{ copy.points }}</th><th>{{ copy.created }}</th></tr></thead><tbody>
          <tr v-for="earning in earningItems" :key="earning.id"><td class="mono"><AssociationName :record="earning" kind="agent" :id="earning.agent_id" :locale="associationLocale" /></td><td class="mono"><AssociationName :record="earning" kind="member" :id="earning.member_id" :locale="associationLocale" /></td><td class="mono">{{ earning.exact_amount.numerator }} / {{ earning.exact_amount.denominator }}</td><td>{{ earning.points }}</td><td>{{ formatPlatformDateTime(earning.created_at, props.displaySettings, props.locale) }}</td></tr>
          <tr v-if="!earningItems.length"><td colspan="5" class="empty-state">{{ copy.noEarnings }}</td></tr>
        </tbody></table></div>
        <div class="wallet-pagination" data-testid="platform-commission-earnings-pages"><span>{{ copy.total }} {{ earningTotal }}</span><button class="button button-secondary" :disabled="loading || earningOffset === 0" @click="withGlobalLoading(loadEarnings, earningOffset - pageSize)">{{ copy.previous }}</button><span>{{ Math.floor(earningOffset / pageSize) + 1 }}</span><button class="button button-secondary" :disabled="loading || !hasNextEarnings" @click="withGlobalLoading(loadEarnings, earningOffset + pageSize)">{{ copy.next }}</button></div>
      </template>
    </section>
</DetailModal>

    <DetailModal v-if="paymentDetail" :title="`${copy.detail} · ${copy.payment}`" :close-label="copy.close" @close="paymentDetail = null">
<section v-if="paymentDetail" class="detail-section" data-testid="platform-commission-details">
      <div class="detail-summary"><span :class="['status-pill', paymentDetail.state]">{{ stateLabel(paymentDetail.state) }}</span></div>
      <dl v-if="paymentDetail" class="confirm-list reward-fields wallet-details">
        <div><dt>{{ copy.payment }}</dt><dd class="mono">{{ paymentDetail.id }}</dd></div><div><dt>{{ copy.cycle }}</dt><dd class="mono"><AssociationName :record="paymentDetail" kind="cycle" :id="paymentDetail.cycle_id" :locale="locale" /></dd></div><div><dt>{{ copy.currentRun }}</dt><dd class="mono">{{ paymentDetail.run_id }}</dd></div>
        <div><dt><BusinessDetails :value="copy.state" :locale="associationLocale" /></dt><dd>{{ stateLabel(paymentDetail.state) }}</dd></div><div><dt>{{ copy.payoutMode }}</dt><dd>{{ modeLabel(paymentDetail.payout_mode) }}</dd></div><div><dt>{{ copy.totalPoints }}</dt><dd>{{ paymentDetail.total_points }}</dd></div>
        <div><dt>{{ copy.paidPoints }}</dt><dd>{{ paymentDetail.paid_points }}</dd></div><div><dt>{{ copy.paidCount }}</dt><dd>{{ paymentDetail.paid_count }} / {{ paymentDetail.target_count }}</dd></div>
        <div><dt>{{ copy.errorCode }}</dt><dd>{{ paymentDetail.last_error_code ? displayPublicError(publicErrorMessage({ code: paymentDetail.last_error_code })) : "—" }}</dd></div><div><dt>{{ copy.created }}</dt><dd>{{ formatPlatformDateTime(paymentDetail.created_at, props.displaySettings, props.locale) }}</dd></div><div><dt>{{ copy.updated }}</dt><dd>{{ formatPlatformDateTime(paymentDetail.updated_at, props.displaySettings, props.locale) }}</dd></div>
      </dl>
    </section>
</DetailModal>
  </div>
</template>
