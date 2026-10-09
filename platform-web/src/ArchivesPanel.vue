<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { PlatformApiError } from './platform-api'
import { createPlatformArchivesApi, type ArchivePage, type ArchiveRecord, type ArchiveSnapshot } from './archives-api'
import { REPORT_FIELDS } from './reports-api'
import { REPORT_METRIC_LABELS } from './report-labels'

const props = defineProps<{ brandId: string; locale: 'en' | 'zh-CN' }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformArchivesApi()
const page = ref<ArchivePage | null>(null)
const detail = ref<ArchiveRecord | null>(null)
const selectedId = ref('')
const offset = ref(0)
const loading = ref(false)
const downloading = ref(false)
const error = ref('')
const receipt = ref('')
let generation = 0
const objectUrls = new Set<string>()

const pageSize = 50
const groups = [
  { key: 'betting', fields: REPORT_FIELDS.betting },
  { key: 'ledger', fields: REPORT_FIELDS.ledger },
  { key: 'withdrawals', fields: REPORT_FIELDS.withdrawal },
  { key: 'commissions', fields: REPORT_FIELDS.commission },
  { key: 'rewards', fields: REPORT_FIELDS.rewards },
  { key: 'reward_orders', fields: REPORT_FIELDS.reward_orders },
] as const
const balanceFields = ['account_count', 'available_points', 'frozen_points', 'withdrawal_points', 'total_points'] as const
const copy = computed(() => props.locale === 'en' ? {
  title: 'Archives', intro: 'Read-only, immutable report snapshots for this brand. Revisions are appended as new records; archive views do not query live reports.',
  refresh: 'Refresh', loading: 'Loading archives…', empty: 'No report archives.', previous: 'Previous page', next: 'Next page', page: 'Page', total: 'Total archives',
  detail: 'Archive detail', window: 'Window', kind: 'Kind', daily: 'Daily', monthly: 'Monthly', period: 'Period', timezone: 'Timezone', from: 'From', to: 'To',
  version: 'Format version', revision: 'Revision', previousId: 'Previous archive ID', creator: 'Created by', automation: 'Automation', task: 'Task ID', policyVersion: 'Policy version',
  reason: 'Reason', hash: 'Payload SHA-256', audit: 'Audit log ID', snapshotAt: 'Snapshot time', createdAt: 'Created at', summaries: 'Report summaries',
  balances: 'Wallet balances at snapshot', balanceNote: 'These are balances observed at the snapshot time, not balances at the end of the report window or current live balances.',
  walletAt: 'Balance observation time', download: 'Download original JSON', downloading: 'Verifying original…', downloaded: 'Verified original downloaded', emptyDetail: 'Select an archive to inspect its saved snapshot.', noBrand: 'Select a brand to view report archives.',
} : {
  title: '报表归档', intro: '此品牌的只读、不可变报表快照。新版本会追加为新记录；归档页面不会查询实时报表。',
  refresh: '刷新', loading: '正在加载归档…', empty: '暂无报表归档。', previous: '上一页', next: '下一页', page: '页码', total: '归档总数',
  detail: '归档详情', window: '时间范围', kind: '类型', daily: '每日', monthly: '每月', period: '期间', timezone: '时区', from: '开始', to: '结束',
  version: '格式版本', revision: '修订版本', previousId: '上一条归档编号', creator: '创建者', automation: '自动化任务', task: '任务编号', policyVersion: '策略版本',
  reason: '原因', hash: '载荷 SHA-256', audit: '审计日志编号', snapshotAt: '快照时间', createdAt: '创建时间', summaries: '报表汇总',
  balances: '快照时的钱包余额', balanceNote: '这是快照时观察到的余额，不是报表期间结束时或当前实时余额。',
  walletAt: '余额观察时间', download: '下载原始JSON', downloading: '正在验证原始文件…', downloaded: '已下载并验证原始文件', emptyDetail: '请选择一条归档以查看已保存的快照。', noBrand: '请选择品牌以查看报表归档。',
})

function metricLabel(field: string): string {
  const pair = REPORT_METRIC_LABELS[field]
  if (!pair) throw new Error(`Unsupported report metric label: ${field}`)
  return pair[props.locale === 'en' ? 0 : 1]
}
function groupLabel(key: typeof groups[number]['key']): string {
  const labels = props.locale === 'en'
    ? { betting: 'Betting', ledger: 'Points ledger', withdrawals: 'Withdrawals', commissions: 'Commission', rewards: 'Reward postings', reward_orders: 'Reward orders' }
    : { betting: '投注', ledger: '积分账本', withdrawals: '提现', commissions: '佣金', rewards: '奖励入账', reward_orders: '奖励订单' }
  return labels[key]
}
function summaryValue(snapshot: ArchiveSnapshot, key: typeof groups[number]['key'], field: string): string {
  return (snapshot[key] as unknown as Record<string, string>)[field]
}
function reportFailure(cause: unknown) {
  page.value = null
  detail.value = null
  selectedId.value = ''
  offset.value = 0
  receipt.value = ''
  downloading.value = false
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  else error.value = cause instanceof Error ? cause.message : String(cause)
}
async function load(nextOffset = 0) {
  const request = ++generation
  const brand = props.brandId
  page.value = null
  detail.value = null
  selectedId.value = ''
  offset.value = nextOffset
  error.value = ''
  receipt.value = ''
  downloading.value = false
  if (!brand) { loading.value = false; return }
  loading.value = true
  try {
    const result = await api.list(brand, pageSize, nextOffset)
    if (request !== generation || brand !== props.brandId) return
    page.value = result
    offset.value = result.offset
  } catch (cause) {
    if (request === generation && brand === props.brandId) reportFailure(cause)
  } finally { if (request === generation) loading.value = false }
}
async function read(record: ArchiveRecord) {
  const request = ++generation
  const brand = props.brandId
  selectedId.value = record.id
  detail.value = null
  error.value = ''
  receipt.value = ''
  downloading.value = false
  if (!brand) return
  loading.value = true
  try {
    const result = await api.read(brand, record.id)
    if (request !== generation || brand !== props.brandId || selectedId.value !== record.id) return
    detail.value = result
  } catch (cause) {
    if (request === generation && brand === props.brandId) reportFailure(cause)
  } finally { if (request === generation) loading.value = false }
}
function releaseObjectUrl(url: string) {
  if (objectUrls.delete(url)) URL.revokeObjectURL(url)
}
async function download() {
  const current = detail.value
  const id = selectedId.value
  const brand = props.brandId
  if (!current || !id || current.id !== id || !brand || downloading.value) return
  const request = ++generation
  downloading.value = true
  loading.value = true
  error.value = ''
  receipt.value = ''
  try {
    const result = await api.download(brand, id, current)
    if (request !== generation || brand !== props.brandId || selectedId.value !== id || detail.value !== current) return
    const digest = result.sha256
    const blob = new Blob([new Uint8Array(result.bytes).buffer], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    objectUrls.add(url)
    window.setTimeout(() => releaseObjectUrl(url), 0)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = result.filename
    document.body.append(anchor)
    anchor.click()
    anchor.remove()
    receipt.value = `${copy.value.downloaded}: ${result.filename} · ${copy.value.revision} ${current.revision} · ${copy.value.hash} ${digest} · ${copy.value.audit} ${current.audit_log_id}`
  } catch (cause) {
    if (request === generation && brand === props.brandId) reportFailure(cause)
  } finally {
    if (request === generation) { downloading.value = false; loading.value = false }
  }
}
const hasNextPage = computed(() => page.value !== null && BigInt(offset.value) + BigInt(page.value.items.length) < BigInt(page.value.total_count))
watch(() => props.brandId, () => {
  generation++
  page.value = null
  detail.value = null
  selectedId.value = ''
  offset.value = 0
  loading.value = false
  downloading.value = false
  error.value = ''
  receipt.value = ''
  void load(0)
}, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => {
  generation++
  for (const url of [...objectUrls]) releaseObjectUrl(url)
})
</script>

<template>
  <div data-testid="platform-archives">
    <div class="page-heading"><div><div class="eyebrow">{{ copy.intro }}</div><h1>{{ copy.title }}</h1></div><button class="secondary" :disabled="loading || !brandId" @click="load(0)">{{ copy.refresh }}</button></div>
    <p v-if="error" class="message error" role="alert">{{ error }}</p>
    <p v-if="receipt" class="message success" role="status" data-testid="platform-archive-download-receipt">{{ receipt }}</p>
    <p v-if="loading" class="loading-line">{{ downloading ? copy.downloading : copy.loading }}</p>
    <p v-if="!brandId && !loading" class="loading-line">{{ copy.noBrand }}</p>
    <section class="panel reward-detail" data-testid="platform-archive-list">
      <div class="panel-heading"><h2>{{ copy.title }}</h2><span v-if="page" class="count-chip">{{ copy.total }}: {{ page.total_count }}</span></div>
      <div class="table-wrap"><table><thead><tr><th>ID</th><th>{{ copy.window }}</th><th>{{ copy.revision }}</th><th>{{ copy.snapshotAt }}</th></tr></thead><tbody>
        <tr v-for="record in page?.items ?? []" :key="record.id"><td><button class="row-action mono" :disabled="loading" :title="copy.title" @click="read(record)">{{ record.id }}</button></td><td>{{ record.window.kind === 'daily' ? copy.daily : copy.monthly }} · {{ record.window.period_key }}</td><td>{{ record.revision }}</td><td>{{ record.snapshot_at }}</td></tr>
        <tr v-if="page && !page.items.length"><td colspan="4" class="empty-state">{{ copy.empty }}</td></tr>
      </tbody></table></div>
      <div class="wallet-pagination" data-testid="platform-archive-pages"><button class="secondary" :disabled="loading || offset === 0 || !brandId" @click="load(Math.max(0, offset - pageSize))">{{ copy.previous }}</button><span>{{ copy.page }} {{ Math.floor(offset / pageSize) + 1 }}</span><button class="secondary" :disabled="loading || !hasNextPage" @click="load(offset + pageSize)">{{ copy.next }}</button></div>
    </section>
    <section v-if="detail" class="panel reward-detail" data-testid="platform-archive-detail">
      <div class="panel-heading"><div><h2>{{ copy.detail }}</h2><p class="mono">{{ detail.id }}</p></div><button class="secondary" :disabled="loading || downloading" @click="download">{{ downloading ? copy.downloading : copy.download }}</button></div>
      <dl class="confirm-list reward-fields">
        <div><dt>{{ copy.window }}</dt><dd>{{ copy.from }}: {{ detail.window.from }} · {{ copy.to }}: {{ detail.window.to }}</dd></div>
        <div><dt>{{ copy.kind }}</dt><dd>{{ detail.window.kind === 'daily' ? copy.daily : copy.monthly }}</dd></div>
        <div><dt>{{ copy.period }}</dt><dd>{{ detail.window.period_key }}</dd></div>
        <div><dt>{{ copy.timezone }}</dt><dd>{{ detail.window.timezone }}</dd></div>
        <div><dt>{{ copy.version }}</dt><dd>{{ detail.snapshot.format_version }}</dd></div>
        <div><dt>{{ copy.revision }}</dt><dd>{{ detail.revision }}</dd></div>
        <div><dt>{{ copy.previousId }}</dt><dd>{{ detail.previous_id || '—' }}</dd></div>
        <div><dt>{{ copy.creator }}</dt><dd v-if="detail.created_by">{{ detail.created_by }}</dd><dd v-else-if="detail.automation">{{ copy.automation }} · {{ copy.task }}: {{ detail.automation.task_id }} · {{ copy.policyVersion }}: {{ detail.automation.policy_version }}</dd></div>
        <div><dt>{{ copy.reason }}</dt><dd>{{ detail.reason }}</dd></div>
        <div><dt>{{ copy.hash }}</dt><dd>{{ detail.payload_sha256 }}</dd></div>
        <div><dt>{{ copy.audit }}</dt><dd>{{ detail.audit_log_id }}</dd></div>
        <div><dt>{{ copy.snapshotAt }}</dt><dd>{{ detail.snapshot_at }}</dd></div>
        <div><dt>{{ copy.createdAt }}</dt><dd>{{ detail.created_at }}</dd></div>
      </dl>
      <div class="panel-heading"><h3>{{ copy.summaries }}</h3></div>
      <section v-for="group in groups" :key="group.key" class="panel reward-detail">
        <div class="panel-heading"><h3>{{ groupLabel(group.key) }}</h3></div>
        <dl class="confirm-list reward-fields"><div v-for="field in group.fields" :key="field"><dt>{{ metricLabel(field) }}</dt><dd>{{ summaryValue(detail.snapshot, group.key, field) }}</dd></div></dl>
      </section>
      <section class="panel reward-detail">
        <div class="panel-heading"><div><h3>{{ copy.balances }}</h3><p>{{ copy.balanceNote }}</p></div></div>
        <dl class="confirm-list reward-fields"><div><dt>{{ copy.walletAt }}</dt><dd>{{ detail.snapshot.wallet_snapshot.at_snapshot }}</dd></div><div v-for="field in balanceFields" :key="field"><dt>{{ metricLabel(field) }}</dt><dd>{{ detail.snapshot.wallet_snapshot.balances[field] }}</dd></div></dl>
      </section>
    </section>
    <p v-else-if="page && !loading" class="loading-line">{{ copy.emptyDetail }}</p>
  </div>
</template>
