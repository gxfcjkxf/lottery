<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { PlatformApiError } from './platform-api'
import { createPlatformCommissionApi, type AgentNode, type Cycle, type Earning, type Payment } from './commission-api'

const props = defineProps<{ brandId: string; locale: 'en' | 'zh-CN' }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformCommissionApi()
const pageSize = 50
type Tab = 'agents' | 'cycles' | 'payments'
const activeTab = ref<Tab>('agents')
const copy = computed(() => props.locale === 'en' ? {
  title: 'Commissions', readOnly: 'Read-only commission records for the selected brand.', agents: 'Agents', cycles: 'Cycles', payments: 'Payments',
  refresh: 'Refresh', previous: 'Previous page', next: 'Next page', total: 'Total', loading: 'Loading…', emptyAgents: 'No agents found.', emptyCycles: 'No commission cycles found.', emptyPayments: 'No commission payments found.',
  agent: 'Agent', member: 'Member', parent: 'Parent', path: 'Agent path', ratio: 'Ratio', mode: 'Mode', effectiveMode: 'Effective mode', status: 'Status', version: 'Version', depth: 'Depth',
  cycle: 'Cycle', window: 'Window (UTC)', state: 'State', currentRun: 'Current run', evidence: 'Current evidence', targets: 'Targets', calculated: 'Calculated', earnings: 'Earnings', totalPoints: 'Calculated points', scanComplete: 'Scan complete',
  payment: 'Payment', payoutMode: 'Payout mode', paidPoints: 'Paid points', paidCount: 'Paid / target count', created: 'Created', updated: 'Updated', errorCode: 'Last error code',
  detail: 'Detail', earningDetails: 'Current-run earnings', exact: 'Exact amount (numerator / denominator)', points: 'Integer points', calculatedNotice: 'These are calculated commission earnings for this run. They are not wallet balance or proof of payment.', noRun: 'This cycle has no current run.', noEarnings: 'No earnings for this run.', evidenceCurrent: 'Current', evidenceStale: 'Stale or unavailable', yes: 'Yes', no: 'No', inherited: 'Inherited',
  states: { active: 'Active', disabled: 'Disabled', enumerating: 'Enumerating', waiting: 'Waiting', calculating: 'Calculating', summarizing: 'Summarizing', ready: 'Ready', failed: 'Failed', awaiting_approval: 'Awaiting approval', paying: 'Paying', paid: 'Paid', stale: 'Stale', blocked: 'Blocked' } as Record<string, string>,
  modes: { loss: 'Loss', turnover: 'Turnover', manual: 'Manual', automatic: 'Automatic', mixed: 'Mixed', none: 'None' } as Record<string, string>,
} : {
  title: '佣金', readOnly: '只读查看所选品牌的佣金记录。', agents: '代理', cycles: '周期', payments: '派发',
  refresh: '刷新', previous: '上一页', next: '下一页', total: '总数', loading: '加载中…', emptyAgents: '暂无代理。', emptyCycles: '暂无佣金周期。', emptyPayments: '暂无佣金派发记录。',
  agent: '代理', member: '会员', parent: '上级代理', path: '代理路径', ratio: '比例', mode: '模式', effectiveMode: '生效模式', status: '状态', version: '版本', depth: '层级',
  cycle: '周期', window: '周期窗口（UTC）', state: '状态', currentRun: '当前运行代次', evidence: '当前证据', targets: '目标数', calculated: '已核算', earnings: '收益数', totalPoints: '核算积分', scanComplete: '扫描完成',
  payment: '派发', payoutMode: '派发模式', paidPoints: '已派发积分', paidCount: '已派发 / 目标数', created: '创建时间', updated: '更新时间', errorCode: '最近错误代码',
  detail: '详情', earningDetails: '当前运行代次收益', exact: '精确金额（分子 / 分母）', points: '整数积分', calculatedNotice: '此处为该运行代次核算出的佣金收益，不代表钱包余额或已派发。', noRun: '此周期没有当前运行代次。', noEarnings: '此运行代次暂无收益。', evidenceCurrent: '当前有效', evidenceStale: '已过期或不可用', yes: '是', no: '否', inherited: '继承',
  states: { active: '启用', disabled: '停用', enumerating: '扫描中', waiting: '等待中', calculating: '核算中', summarizing: '汇总中', ready: '就绪', failed: '失败', awaiting_approval: '等待批准', paying: '派发中', paid: '已派发', stale: '已过期', blocked: '受阻' } as Record<string, string>,
  modes: { loss: '亏损', turnover: '投注额', manual: '手动', automatic: '自动', mixed: '混合', none: '无' } as Record<string, string>,
})

const agents = ref<AgentNode[]>([])
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
const error = ref('')
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
  else error.value = cause instanceof Error ? cause.message : String(cause)
}

function clearDetails() {
  agentDetail.value = null
  cycleDetail.value = null
  paymentDetail.value = null
  earningItems.value = []; earningOffset.value = 0; earningTotal.value = '0'
}

function clearBrandState() {
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
      const page = await api.tree(brandId, pageSize, nextOffset)
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

function refresh() { void loadList(activeTab.value, 0) }
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
  if (props.brandId) void loadList(activeTab.value, 0)
  else loading.value = false
}, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { generation++ })
</script>

<template>
  <div v-if="brandId" data-testid="platform-commission">
    <div class="page-heading"><div><div class="eyebrow">{{ copy.readOnly }}</div><h1>{{ copy.title }}</h1></div></div>
    <p v-if="error" class="message error global-message" role="alert">{{ error }}</p>
    <p v-if="loading" class="loading-line">{{ copy.loading }}</p>
    <section class="panel">
      <div class="panel-heading">
        <div><h2>{{ copy[activeTab] }}</h2><p>{{ copy.readOnly }}</p></div>
        <div class="reward-list-actions"><span class="count-chip">{{ copy.total }} {{ total }}</span><button class="secondary" :disabled="loading || !brandId" @click="refresh">{{ copy.refresh }}</button></div>
      </div>
      <div class="wallet-pagination" role="tablist" :aria-label="copy.title" data-testid="platform-commission-tabs">
        <button v-for="tab in (['agents', 'cycles', 'payments'] as const)" :key="tab" class="secondary" role="tab" :aria-selected="activeTab === tab" :class="{ selected: activeTab === tab }" :disabled="loading || !brandId" @click="switchTab(tab)">{{ copy[tab] }}</button>
      </div>
      <div class="table-wrap" data-testid="platform-commission-list">
        <table v-if="activeTab === 'agents'"><thead><tr><th>{{ copy.agent }}</th><th>{{ copy.member }}</th><th>{{ copy.parent }}</th><th>{{ copy.ratio }}</th><th>{{ copy.mode }}</th><th>{{ copy.status }}</th></tr></thead><tbody>
          <tr v-for="agent in agents" :key="agent.id"><td><button class="row-action mono" @click="openAgent(agent)">{{ agent.id }}</button><small>{{ copy.depth }} {{ agent.depth }}</small></td><td class="mono">{{ agent.member_id }}</td><td class="mono">{{ agent.parent_id || '—' }}</td><td>{{ agent.config.ratio }}</td><td>{{ modeLabel(agent.config.mode) }}</td><td><span :class="['status-pill', agent.config.status]">{{ stateLabel(agent.config.status) }}</span></td></tr>
          <tr v-if="!agents.length"><td colspan="6" class="empty-state">{{ emptyLabel }}</td></tr>
        </tbody></table>
        <table v-else-if="activeTab === 'cycles'"><thead><tr><th>{{ copy.cycle }}</th><th>{{ copy.window }}</th><th>{{ copy.state }}</th><th>{{ copy.evidence }}</th><th>{{ copy.calculated }} / {{ copy.targets }}</th><th>{{ copy.totalPoints }}</th></tr></thead><tbody>
          <tr v-for="cycle in cycles" :key="cycle.id"><td><button class="row-action mono" @click="openCycle(cycle)">{{ cycle.id }}</button><small>{{ copy.currentRun }} · {{ cycle.current_run_id || '—' }}</small></td><td>{{ cycle.window_from }}<small>{{ cycle.window_to }}</small></td><td><span :class="['status-pill', cycle.state]">{{ stateLabel(cycle.state) }}</span></td><td>{{ cycle.evidence_current ? copy.evidenceCurrent : copy.evidenceStale }}</td><td>{{ cycle.calculated_count }} / {{ cycle.target_count }}</td><td>{{ cycle.total_points }}</td></tr>
          <tr v-if="!cycles.length"><td colspan="6" class="empty-state">{{ emptyLabel }}</td></tr>
        </tbody></table>
        <table v-else><thead><tr><th>{{ copy.payment }}</th><th>{{ copy.cycle }}</th><th>{{ copy.state }}</th><th>{{ copy.payoutMode }}</th><th>{{ copy.paidPoints }} / {{ copy.totalPoints }}</th><th>{{ copy.paidCount }}</th></tr></thead><tbody>
          <tr v-for="payment in payments" :key="payment.id"><td><button class="row-action mono" @click="openPayment(payment)">{{ payment.id }}</button><small>{{ payment.created_at }}</small></td><td class="mono">{{ payment.cycle_id }}</td><td><span :class="['status-pill', payment.state]">{{ stateLabel(payment.state) }}</span></td><td>{{ modeLabel(payment.payout_mode) }}</td><td>{{ payment.paid_points }} / {{ payment.total_points }}</td><td>{{ payment.paid_count }} / {{ payment.target_count }}</td></tr>
          <tr v-if="!payments.length"><td colspan="6" class="empty-state">{{ emptyLabel }}</td></tr>
        </tbody></table>
      </div>
      <div class="wallet-pagination" data-testid="platform-commission-list-pages">
        <button class="secondary" :disabled="loading || offset === 0 || !brandId" @click="loadList(activeTab, offset - pageSize)">{{ copy.previous }}</button>
        <span>{{ Math.floor(offset / pageSize) + 1 }}</span>
        <button class="secondary" :disabled="loading || !hasNext" @click="loadList(activeTab, offset + pageSize)">{{ copy.next }}</button>
      </div>
    </section>

    <section v-if="agentDetail" class="panel reward-detail" data-testid="platform-commission-details">
      <div class="panel-heading"><div><h2>{{ copy.detail }} · {{ copy.agent }}</h2><p class="mono">{{ agentDetail.id }}</p></div><span class="count-chip">{{ stateLabel(agentDetail.config.status) }} · v{{ agentDetail.version }}</span></div>
      <dl v-if="agentDetail" class="confirm-list reward-fields wallet-details">
        <div><dt>{{ copy.agent }}</dt><dd class="mono">{{ agentDetail.id }}</dd></div><div><dt>{{ copy.member }}</dt><dd class="mono">{{ agentDetail.member_id }}</dd></div><div><dt>{{ copy.parent }}</dt><dd class="mono">{{ agentDetail.parent_id || '—' }}</dd></div>
        <div><dt>{{ copy.path }}</dt><dd class="mono">{{ agentDetail.path.join(' → ') }}</dd></div><div><dt>{{ copy.depth }}</dt><dd>{{ agentDetail.depth }}</dd></div><div><dt>{{ copy.ratio }}</dt><dd>{{ agentDetail.config.ratio }}</dd></div>
        <div><dt>{{ copy.mode }}</dt><dd>{{ modeLabel(agentDetail.config.mode) }}</dd></div><div><dt>{{ copy.effectiveMode }}</dt><dd>{{ modeLabel(agentDetail.effective_mode) }}</dd></div><div><dt>{{ copy.status }}</dt><dd>{{ stateLabel(agentDetail.config.status) }}</dd></div><div><dt>{{ copy.version }}</dt><dd>{{ agentDetail.version }}</dd></div>
      </dl>
    </section>

    <section v-if="cycleDetail" class="panel reward-detail" data-testid="platform-commission-details">
      <div class="panel-heading"><div><h2>{{ copy.detail }} · {{ copy.cycle }}</h2><p class="mono">{{ cycleDetail.id }}</p></div><span :class="['status-pill', cycleDetail.state]">{{ stateLabel(cycleDetail.state) }}</span></div>
      <dl v-if="cycleDetail" class="confirm-list reward-fields wallet-details">
        <div><dt>{{ copy.cycle }}</dt><dd class="mono">{{ cycleDetail.id }}</dd></div><div><dt>{{ copy.window }}</dt><dd>{{ cycleDetail.window_from }} → {{ cycleDetail.window_to }}</dd></div><div><dt>{{ copy.state }}</dt><dd>{{ stateLabel(cycleDetail.state) }} · v{{ cycleDetail.version }}</dd></div>
        <div><dt>{{ copy.currentRun }}</dt><dd class="mono">{{ cycleDetail.current_run_id || '—' }}</dd></div><div><dt>{{ copy.evidence }}</dt><dd>{{ cycleDetail.evidence_current ? copy.evidenceCurrent : copy.evidenceStale }}</dd></div><div><dt>{{ copy.evidence }} epoch</dt><dd>{{ cycleDetail.evidence_epoch ?? '—' }}</dd></div>
        <div><dt>{{ copy.targets }}</dt><dd>{{ cycleDetail.target_count }}</dd></div><div><dt>{{ copy.scanComplete }}</dt><dd>{{ cycleDetail.scan_complete ? copy.yes : copy.no }}</dd></div><div><dt>{{ copy.calculated }}</dt><dd>{{ cycleDetail.calculated_count }}</dd></div>
        <div><dt>{{ copy.earnings }}</dt><dd>{{ cycleDetail.earning_count }}</dd></div><div><dt>{{ copy.totalPoints }}</dt><dd>{{ cycleDetail.total_points }}</dd></div><div><dt>{{ copy.created }}</dt><dd>{{ cycleDetail.created_at }}</dd></div><div><dt>{{ copy.updated }}</dt><dd>{{ cycleDetail.updated_at }}</dd></div>
      </dl>
      <div v-if="cycleDetail" class="panel-heading"><div><h2>{{ copy.earningDetails }}</h2><p>{{ cycleDetail.current_run_id || copy.noRun }}</p></div><span v-if="cycleDetail.current_run_id" class="count-chip">{{ copy.total }} {{ earningTotal }}</span></div>
      <template v-if="cycleDetail?.current_run_id">
        <p class="message">{{ copy.calculatedNotice }}</p>
        <div class="table-wrap"><table><thead><tr><th>{{ copy.agent }}</th><th>{{ copy.member }}</th><th>{{ copy.exact }}</th><th>{{ copy.points }}</th><th>{{ copy.created }}</th></tr></thead><tbody>
          <tr v-for="earning in earningItems" :key="earning.id"><td class="mono">{{ earning.agent_id }}</td><td class="mono">{{ earning.member_id }}</td><td class="mono">{{ earning.exact_amount.numerator }} / {{ earning.exact_amount.denominator }}</td><td>{{ earning.points }}</td><td>{{ earning.created_at }}</td></tr>
          <tr v-if="!earningItems.length"><td colspan="5" class="empty-state">{{ copy.noEarnings }}</td></tr>
        </tbody></table></div>
        <div class="wallet-pagination" data-testid="platform-commission-earnings-pages"><span>{{ copy.total }} {{ earningTotal }}</span><button class="secondary" :disabled="loading || earningOffset === 0" @click="loadEarnings(earningOffset - pageSize)">{{ copy.previous }}</button><span>{{ Math.floor(earningOffset / pageSize) + 1 }}</span><button class="secondary" :disabled="loading || !hasNextEarnings" @click="loadEarnings(earningOffset + pageSize)">{{ copy.next }}</button></div>
      </template>
    </section>

    <section v-if="paymentDetail" class="panel reward-detail" data-testid="platform-commission-details">
      <div class="panel-heading"><div><h2>{{ copy.detail }} · {{ copy.payment }}</h2><p class="mono">{{ paymentDetail.id }}</p></div><span :class="['status-pill', paymentDetail.state]">{{ stateLabel(paymentDetail.state) }}</span></div>
      <dl v-if="paymentDetail" class="confirm-list reward-fields wallet-details">
        <div><dt>{{ copy.payment }}</dt><dd class="mono">{{ paymentDetail.id }}</dd></div><div><dt>{{ copy.cycle }}</dt><dd class="mono">{{ paymentDetail.cycle_id }}</dd></div><div><dt>{{ copy.currentRun }}</dt><dd class="mono">{{ paymentDetail.run_id }}</dd></div>
        <div><dt>{{ copy.state }}</dt><dd>{{ stateLabel(paymentDetail.state) }} · v{{ paymentDetail.version }}</dd></div><div><dt>{{ copy.payoutMode }}</dt><dd>{{ modeLabel(paymentDetail.payout_mode) }}</dd></div><div><dt>{{ copy.totalPoints }}</dt><dd>{{ paymentDetail.total_points }}</dd></div>
        <div><dt>{{ copy.paidPoints }}</dt><dd>{{ paymentDetail.paid_points }}</dd></div><div><dt>{{ copy.paidCount }}</dt><dd>{{ paymentDetail.paid_count }} / {{ paymentDetail.target_count }}</dd></div><div><dt>{{ copy.evidence }} epoch</dt><dd>{{ paymentDetail.evidence_epoch }}</dd></div>
        <div><dt>{{ copy.errorCode }}</dt><dd>{{ paymentDetail.last_error_code || '—' }}</dd></div><div><dt>{{ copy.created }}</dt><dd>{{ paymentDetail.created_at }}</dd></div><div><dt>{{ copy.updated }}</dt><dd>{{ paymentDetail.updated_at }}</dd></div>
      </dl>
    </section>
  </div>
</template>
