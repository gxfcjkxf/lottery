<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { PlatformApiError } from './platform-api'
import { createPlatformReportsApi, REPORT_FIELDS, REPORT_GROUPS, type ReportKind, type ReportQuery, type ReportResult } from './reports-api'
import { createPlatformReportExportApi } from './report-export-api'
import { REPORT_METRIC_LABELS as metricLabels } from './report-labels'

const props = defineProps<{ brandId: string; memberId: string; locale: 'en' | 'zh-CN' }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformReportsApi()
const exportApi = createPlatformReportExportApi()
type ExportRequest = Readonly<{ brandId: string; kind: ReportKind; query: Readonly<Omit<ReportQuery, 'limit' | 'offset'>> }>
const exportRequest = ref<ExportRequest | null>(null)
const exporting = ref(false)
const exportNotice = ref('')
const kind = ref<ReportKind>('betting')
const from = ref('')
const to = ref('')
const groupBy = ref('day')
const memberFilter = ref(props.memberId)
const gameFilter = ref('')
const agentFilter = ref('')
const cycleFilter = ref('')
const orderFilter = ref('')
const report = ref<ReportResult | null>(null)
const submitted = ref<{ kind: ReportKind; query: ReportQuery } | null>(null)
const offset = ref(0)
const loading = ref(false)
const error = ref('')
let generation = 0

const kinds: ReportKind[] = ['betting', 'ledger', 'withdrawal', 'commission', 'rewards', 'reward_orders']
const balanceFields = ['account_count', 'available_points', 'frozen_points', 'withdrawal_points', 'total_points'] as const
const copy = computed(() => props.locale === 'en' ? {
  title: 'Operational reports', intro: 'Read-only snapshots for the selected brand.', type: 'Report type', from: 'From (UTC)', to: 'To (UTC)', group: 'Group by', member: 'Member ID (optional)', game: 'Game ID (optional)', agent: 'Agent ID (optional)', cycle: 'Cycle ID (optional)', order: 'Order ID (optional)', run: 'Run report', loading: 'Loading report…', snapshot: 'Snapshot', timezone: 'Report timezone', summary: 'Summary for the full filter window', groups: 'Groups', empty: 'No groups in this report.', previous: 'Previous page', next: 'Next page', page: 'Page', total: 'Total groups', balances: 'Current wallet balances', balanceNote: 'Current balances at the snapshot; these are not a change within the selected time window.', bettingNote: 'Bet orders are shown by their current final state at the snapshot.', withdrawalNote: 'Withdrawal requests are shown by their current state; this is not a record of actual payment.', rewardOrdersNote: 'Reward orders are shown by their current state; this is not a record of actual payment.', commissionNote: 'Commission totals use the actual posting time.', rewardsNote: 'Reward totals use the actual wallet posting time.', ledgerNote: 'Ledger totals use the actual posting time.', noBrand: 'Select a brand and run the report to view data.',
  rewardPendingNote: 'Pending revocation amounts are original order points, not held wallet funds.',
  export: 'Export complete CSV', exportTitle: 'Confirm CSV export', exportNote: 'Export all groups in these filters, not just this page. The server creates a fresh snapshot and records an audit entry. Maximum 10,000 groups and 4 MiB; oversized exports fail without a partial file.',
  brand: 'Brand ID', cancel: 'Cancel', confirm: 'Confirm and download', exporting: 'Verifying export…', exported: 'Verified CSV downloaded', audit: 'Audit ID',
} : {
  title: '运营报表', intro: '所选品牌的只读快照。', type: '报表类型', from: '开始时间（UTC）', to: '结束时间（UTC）', group: '分组方式', member: '会员编号（可选）', game: '游戏编号（可选）', agent: '代理编号（可选）', cycle: '周期编号（可选）', order: '订单编号（可选）', run: '查询报表', loading: '正在加载报表…', snapshot: '快照时间', timezone: '报表时区', summary: '整个筛选时间范围汇总', groups: '分组数据', empty: '此报表没有分组数据。', previous: '上一页', next: '下一页', page: '页码', total: '分组总数', balances: '当前钱包余额', balanceNote: '快照时点的当前余额；不是所选时间范围内的变动额。', bettingNote: '投注订单按快照时点的当前最终状态统计。', withdrawalNote: '提现申请按当前状态统计；不代表实际付款记录。', rewardOrdersNote: '奖励订单按当前状态统计；不代表实际付款记录。', commissionNote: '佣金按实际入账时间统计。', rewardsNote: '奖励按实际钱包入账时间统计。', ledgerNote: '账本按实际入账时间统计。', noBrand: '请选择品牌并查询报表以查看数据。',
  rewardPendingNote: '待撤销金额是原订单积分，不代表已冻结的余额。',
  export: '导出完整CSV', exportTitle: '确认CSV导出', exportNote: '导出这些筛选条件下的全部分组，不是仅导出当前页。服务器生成新快照并记录审计。最多10000组、4MiB；超限直接报错，不提供截断文件。',
  brand: '品牌编号', cancel: '取消', confirm: '确认并下载', exporting: '正在验证导出…', exported: '已下载验证后的CSV', audit: '审计编号',
})

const kindLabels: Record<ReportKind, [string, string]> = {
  betting: ['Betting', '投注'], ledger: ['Points ledger', '积分账本'], withdrawal: ['Withdrawals', '提现'],
  commission: ['Commission', '佣金'], rewards: ['Reward postings', '奖励入账'], reward_orders: ['Reward orders', '奖励订单'],
}
const groupLabels: Record<string, [string, string]> = {
  day: ['Day', '日期'], game: ['Game', '游戏'], member: ['Member', '会员'], entry_type: ['Entry type', '分录类型'],
  state: ['Current state', '当前状态'], agent: ['Agent', '代理'], cycle: ['Cycle', '周期'], order: ['Order', '订单'],
  reviewing: ['Under review', '审核中'], processing: ['Processing', '处理中'], paid: ['Paid', '已付款'], rejected: ['Rejected', '已拒绝'],
  failed: ['Failed', '失败'], cancelled: ['Cancelled', '已取消'], granted: ['Granted', '已发放'], revocation_pending: ['Revocation pending', '待撤销'], revoked: ['Revoked', '已撤销'],
}
const groupOptions = computed(() => REPORT_GROUPS[kind.value].map(value => ({ value, label: label(groupLabels, value) })))
const note = computed(() => ({ betting: copy.value.bettingNote, withdrawal: copy.value.withdrawalNote, reward_orders: copy.value.rewardOrdersNote, commission: copy.value.commissionNote, rewards: copy.value.rewardsNote, ledger: copy.value.ledgerNote })[kind.value])
function label(labels: Record<string, [string, string]>, key: string) {
  const pair = labels[key]
  if (!pair) throw new Error(`Unsupported report label: ${key}`)
  return pair[props.locale === 'en' ? 0 : 1]
}
function clearReport() { generation++; report.value = null; submitted.value = null; offset.value = 0; loading.value = false; error.value = ''; exportRequest.value = null; exporting.value = false; exportNotice.value = '' }
function edited() { if (report.value || submitted.value || loading.value) clearReport() }
watch(kind, () => { groupBy.value = 'day'; gameFilter.value = ''; agentFilter.value = ''; cycleFilter.value = ''; orderFilter.value = ''; edited() })
watch(() => [props.brandId, props.memberId], () => {
  clearReport(); from.value = ''; to.value = ''; groupBy.value = 'day'
  memberFilter.value = props.memberId; gameFilter.value = ''; agentFilter.value = ''; cycleFilter.value = ''; orderFilter.value = ''
}, { flush: 'sync' })
onBeforeUnmount(() => { generation++; exportRequest.value = null })

function makeQuery(pageOffset: number): ReportQuery {
  const utc = (value: string) => `${value.length === 16 ? `${value}:00` : value}Z`
  const query: ReportQuery = { from: utc(from.value), to: utc(to.value), group_by: groupBy.value, limit: 50, offset: pageOffset }
  const optional: Partial<Record<'member_id' | 'game_id' | 'agent_id' | 'cycle_id' | 'order_id', string>> = {
    member_id: memberFilter.value.trim(), game_id: gameFilter.value.trim(), agent_id: agentFilter.value.trim(), cycle_id: cycleFilter.value.trim(), order_id: orderFilter.value.trim(),
  }
  for (const [key, value] of Object.entries(optional)) if (value) query[key as keyof typeof optional] = value
  return query
}
async function load(nextOffset = 0) {
  if (!props.brandId || !from.value || !to.value || !groupBy.value) return
  const request = ++generation
  exportRequest.value = null; exportNotice.value = ''
  const brand = props.brandId
  const requestKind = kind.value
  const query = submitted.value && nextOffset !== 0 ? { ...submitted.value.query, offset: nextOffset } : makeQuery(nextOffset)
  const frozen = { kind: requestKind, query: { ...query } }
  report.value = null; submitted.value = null; offset.value = nextOffset; error.value = ''; loading.value = true
  try {
    const result = await api.read(brand, requestKind, frozen.query)
    if (request !== generation || brand !== props.brandId) return
    report.value = result; submitted.value = frozen; offset.value = nextOffset
  } catch (cause) {
    if (request !== generation) return
    if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
    else error.value = cause instanceof Error ? cause.message : String(cause)
  } finally { if (request === generation) loading.value = false }
}
function page(nextOffset: number) { if (submitted.value) void load(nextOffset) }
function reviewExport() {
  if (!submitted.value || !report.value || exporting.value) return
  const { limit: _limit, offset: _offset, ...query } = submitted.value.query
  exportRequest.value = Object.freeze({ brandId: props.brandId, kind: submitted.value.kind, query: Object.freeze({ ...query }) })
  error.value = ''; exportNotice.value = ''
}
async function confirmExport() {
  const frozen = exportRequest.value
  if (!frozen || exporting.value) return
  const request = ++generation
  exporting.value = true; error.value = ''; exportNotice.value = ''
  try {
    const result = await exportApi.export(frozen.brandId, frozen.kind, frozen.query)
    if (request !== generation || props.brandId !== frozen.brandId || exportRequest.value !== frozen) return
    const blob = new Blob([new Uint8Array(result.bytes).buffer], { type: 'text/csv;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url; link.download = result.filename
    document.body.append(link); link.click(); link.remove()
    setTimeout(() => URL.revokeObjectURL(url), 0)
    exportNotice.value = `${copy.value.exported}: ${result.filename} · ${copy.value.total}: ${result.groupCount} · ${copy.value.snapshot}: ${result.snapshotAt} · ${copy.value.audit}: ${result.auditLogId}`
    exportRequest.value = null
  } catch (cause) {
    if (request !== generation) return
    if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
    else error.value = cause instanceof Error ? cause.message : String(cause)
    exportRequest.value = null
  } finally { if (request === generation) exporting.value = false }
}
const fields = computed(() => REPORT_FIELDS[report.value ? submitted.value!.kind : kind.value])
const hasNextPage = computed(() => report.value !== null && BigInt(offset.value + report.value.items.length) < BigInt(report.value.total_groups))
</script>

<template>
  <div data-testid="platform-reports">
    <div class="page-heading"><div><div class="eyebrow">{{ copy.intro }}</div><h1>{{ copy.title }}</h1></div></div>
    <form class="panel wallet-query" @submit.prevent="load(0)">
      <label>{{ copy.type }}<select v-model="kind" :aria-label="copy.type" :disabled="loading"><option v-for="option in kinds" :key="option" :value="option">{{ label(kindLabels, option) }}</option></select></label>
      <label>{{ copy.from }}<input v-model="from" :aria-label="copy.from" type="datetime-local" step="1" required :disabled="loading" @input="edited" /></label>
      <label>{{ copy.to }}<input v-model="to" :aria-label="copy.to" type="datetime-local" step="1" required :disabled="loading" @input="edited" /></label>
      <label>{{ copy.group }}<select v-model="groupBy" :aria-label="copy.group" required :disabled="loading" @change="edited"><option v-for="option in groupOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select></label>
      <label>{{ copy.member }}<input v-model="memberFilter" :aria-label="copy.member" autocomplete="off" :disabled="loading" @input="edited" /></label>
      <label v-if="kind === 'betting'">{{ copy.game }}<input v-model="gameFilter" :aria-label="copy.game" autocomplete="off" :disabled="loading" @input="edited" /></label>
      <label v-if="kind === 'commission'">{{ copy.agent }}<input v-model="agentFilter" :aria-label="copy.agent" autocomplete="off" :disabled="loading" @input="edited" /></label>
      <label v-if="kind === 'commission'">{{ copy.cycle }}<input v-model="cycleFilter" :aria-label="copy.cycle" autocomplete="off" :disabled="loading" @input="edited" /></label>
      <label v-if="kind === 'rewards' || kind === 'reward_orders'">{{ copy.order }}<input v-model="orderFilter" :aria-label="copy.order" autocomplete="off" :disabled="loading" @input="edited" /></label>
      <button class="secondary" :disabled="loading || !brandId">{{ copy.run }}</button>
    </form>
    <p v-if="error" class="message error" role="alert">{{ error }}</p>
    <p v-if="exportNotice" class="message success" role="status" data-testid="platform-report-export-receipt">{{ exportNotice }}</p>
    <p v-if="loading" class="loading-line">{{ copy.loading }}</p>
    <p v-if="!brandId && !loading" class="loading-line">{{ copy.noBrand }}</p>
    <template v-if="report && submitted">
      <p class="message" role="note">{{ note }}</p>
      <div class="wallet-pagination"><button class="secondary" :disabled="loading || exporting" @click="reviewExport">{{ copy.export }}</button></div>
      <p v-if="submitted.kind === 'reward_orders'" class="message" role="note">{{ copy.rewardPendingNote }}</p>
      <section class="panel reward-detail" data-testid="platform-reports-summary">
        <div class="panel-heading"><div><h2>{{ copy.summary }}</h2><p>{{ copy.snapshot }}: {{ report.snapshot_at }} · {{ copy.timezone }}: {{ report.timezone }}</p></div></div>
        <dl class="confirm-list reward-fields">
          <div v-for="field in fields" :key="field"><dt>{{ label(metricLabels, field) }}</dt><dd>{{ report.summary[field] }}</dd></div>
        </dl>
      </section>
      <section v-if="report.balances" class="panel reward-detail" data-testid="platform-reports-balances">
        <div class="panel-heading"><div><h2>{{ copy.balances }}</h2><p>{{ copy.balanceNote }}</p></div></div>
        <dl class="confirm-list reward-fields"><div v-for="field in balanceFields" :key="field"><dt>{{ label(metricLabels, field) }}</dt><dd>{{ report.balances[field] }}</dd></div></dl>
      </section>
      <section class="panel reward-detail" data-testid="platform-reports-list">
        <div class="panel-heading"><h2>{{ copy.groups }}</h2><span class="count-chip">{{ copy.total }}: {{ report.total_groups }}</span></div>
        <div class="table-wrap"><table><thead><tr><th>{{ label(groupLabels, submitted.query.group_by) }}</th><th v-for="field in fields" :key="field">{{ label(metricLabels, field) }}</th></tr></thead><tbody>
          <tr v-for="item in report.items" :key="item.key"><td>{{ submitted.query.group_by === 'state' ? label(groupLabels, item.key) : item.label }}</td><td v-for="field in fields" :key="field">{{ item.totals[field] }}</td></tr>
          <tr v-if="!report.items.length"><td :colspan="fields.length + 1" class="empty-state">{{ copy.empty }}</td></tr>
        </tbody></table></div>
        <div class="wallet-pagination" data-testid="platform-reports-pages"><button class="secondary" :disabled="loading || offset === 0" @click="page(Math.max(0, offset - 50))">{{ copy.previous }}</button><span>{{ copy.page }} {{ Math.floor(offset / 50) + 1 }}</span><button class="secondary" :disabled="loading || !hasNextPage" @click="page(offset + 50)">{{ copy.next }}</button></div>
      </section>
    </template>
    <div v-if="exportRequest" class="modal-scrim"><section class="modal-card" role="dialog" aria-modal="true" :aria-label="copy.exportTitle">
      <h2>{{ copy.exportTitle }}</h2><p class="modal-intro">{{ copy.exportNote }}</p>
      <dl class="confirm-list">
        <div><dt>{{ copy.brand }}</dt><dd>{{ exportRequest.brandId }}</dd></div>
        <div><dt>{{ copy.type }}</dt><dd>{{ label(kindLabels, exportRequest.kind) }}</dd></div>
        <div><dt>{{ copy.from }}</dt><dd>{{ exportRequest.query.from }}</dd></div><div><dt>{{ copy.to }}</dt><dd>{{ exportRequest.query.to }}</dd></div>
        <div><dt>{{ copy.group }}</dt><dd>{{ label(groupLabels, exportRequest.query.group_by) }}</dd></div>
        <div v-if="exportRequest.query.member_id"><dt>{{ copy.member }}</dt><dd>{{ exportRequest.query.member_id }}</dd></div>
        <div v-if="exportRequest.query.game_id"><dt>{{ copy.game }}</dt><dd>{{ exportRequest.query.game_id }}</dd></div>
        <div v-if="exportRequest.query.agent_id"><dt>{{ copy.agent }}</dt><dd>{{ exportRequest.query.agent_id }}</dd></div>
        <div v-if="exportRequest.query.cycle_id"><dt>{{ copy.cycle }}</dt><dd>{{ exportRequest.query.cycle_id }}</dd></div>
        <div v-if="exportRequest.query.order_id"><dt>{{ copy.order }}</dt><dd>{{ exportRequest.query.order_id }}</dd></div>
      </dl><div class="form-actions"><button class="secondary" :disabled="exporting" @click="exportRequest = null">{{ copy.cancel }}</button><button class="primary" :disabled="exporting" @click="confirmExport">{{ exporting ? copy.exporting : copy.confirm }}</button></div>
    </section></div>
  </div>
</template>
