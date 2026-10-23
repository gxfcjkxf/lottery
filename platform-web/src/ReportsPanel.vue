<script setup lang="ts">
import { associationGroupLabel } from "@lottery/shared";
import { usePlatformI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from "@lottery/shared";
import { usePlatformI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vInlineValidation } from '@lottery/shared'
import { usePlatformI18n } from "./i18n"
const ui = usePlatformI18n().t
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import PlatformTemporalInput from '@lottery/shared/PlatformTemporalInput.vue'
import FilterAssociationSelect from '@lottery/shared/FilterAssociationSelect.vue'
import { reportDateRange } from './report-date-range'
import { PlatformApiError } from './platform-api'
import { createPlatformReportsApi, REPORT_FIELDS, REPORT_GROUPS, type ReportKind, type ReportQuery, type ReportResult } from './reports-api'
import { createPlatformReportExportApi } from './report-export-api'
import { REPORT_METRIC_KEYS as metricLabels } from './report-labels'
import { formatPlatformDateTime, type PlatformDisplaySettings } from './platform-format'

const props = defineProps<{ brandId: string; memberId: string; locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings }>()
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
const memberFilter = ref('')
const gameFilter = ref('')
const agentFilter = ref('')
const cycleFilter = ref('')
const orderFilter = ref('')
const report = ref<ReportResult | null>(null)
const submitted = ref<{ kind: ReportKind; query: ReportQuery } | null>(null)
const offset = ref(0)
const loading = ref(false)
const error = ref<string | import("@lottery/shared").LocalizedMessage>("")
let generation = 0

const kinds: ReportKind[] = ['betting', 'ledger', 'withdrawal', 'commission', 'rewards', 'reward_orders']
const balanceFields = ['account_count', 'available_points', 'frozen_points', 'withdrawal_points', 'total_points'] as const
const copy = computed(() => props.locale === 'en' ? {
  type: ui('platform.copy.9accfb74'), from: ui('platform.copy.b29db694'), to: ui('platform.copy.52606ad4'), group: ui('platform.copy.936e4b65'), member: ui('filter.member'), game: ui('common.games'), agent: ui('filter.agent'), cycle: ui('filter.cycle'), order: ui('filter.reward_order'), run: ui('platform.copy.e479a50e'), snapshot: ui('platform.copy.54dce63b'), timezone: ui('platform.copy.323b7aa3'), summary: ui('platform.copy.d4e4c9cf'), groups: ui('platform.copy.355f0b33'), empty: ui('platform.copy.b797f882'), previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), page: ui('platform.copy.6695ff1c'), total: ui('platform.copy.2ba27d4c'), balances: ui('platform.copy.0d70e750'), balanceNote: ui('platform.copy.74c6280f'), bettingNote: ui('platform.copy.44d13134'), withdrawalNote: ui('platform.copy.8af3e706'), rewardOrdersNote: ui('platform.copy.7e5a2d6d'), commissionNote: ui('platform.copy.571b221b'), rewardsNote: ui('platform.copy.5c4eeedb'), ledgerNote: ui('platform.copy.81fd66fd'),
  rewardPendingNote: ui('platform.copy.2cc9bfe1'),
  export: ui('platform.copy.40108cb4'), exportTitle: ui('platform.copy.01e8cbab'), exportNote: ui('platform.copy.6a3cc60a'),
  brand: ui('platform.copy.8a866cb7'), cancel: ui('platform.copy.2230b1a7'), confirm: ui('platform.copy.9e4b7496'), exporting: ui('platform.copy.3d2ced1c'), exported: ui('platform.copy.c26d8333'), audit: ui('platform.copy.18bb6f26'),
} : {
  type: ui('platform.copy.9accfb74'), from: ui('platform.copy.b29db694'), to: ui('platform.copy.52606ad4'), group: ui('platform.copy.936e4b65'), member: ui('filter.member'), game: ui('common.games'), agent: ui('filter.agent'), cycle: ui('filter.cycle'), order: ui('filter.reward_order'), run: ui('platform.copy.e479a50e'), snapshot: ui('platform.copy.54dce63b'), timezone: ui('platform.copy.323b7aa3'), summary: ui('platform.copy.d4e4c9cf'), groups: ui('platform.copy.355f0b33'), empty: ui('platform.copy.b797f882'), previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), page: ui('platform.copy.6695ff1c'), total: ui('platform.copy.2ba27d4c'), balances: ui('platform.copy.0d70e750'), balanceNote: ui('platform.copy.74c6280f'), bettingNote: ui('platform.copy.44d13134'), withdrawalNote: ui('platform.copy.8af3e706'), rewardOrdersNote: ui('platform.copy.7e5a2d6d'), commissionNote: ui('platform.copy.571b221b'), rewardsNote: ui('platform.copy.5c4eeedb'), ledgerNote: ui('platform.copy.81fd66fd'),
  rewardPendingNote: ui('platform.copy.2cc9bfe1'),
  export: ui('platform.copy.40108cb4'), exportTitle: ui('platform.copy.01e8cbab'), exportNote: ui('platform.copy.6a3cc60a'),
  brand: ui('platform.copy.8a866cb7'), cancel: ui('platform.copy.2230b1a7'), confirm: ui('platform.copy.9e4b7496'), exporting: ui('platform.copy.3d2ced1c'), exported: ui('platform.copy.c26d8333'), audit: ui('platform.copy.18bb6f26'),
})

const kindLabels: Record<ReportKind, string> = {
  betting: 'platform.copy.4e6493d5', ledger: 'platform.copy.748ef186', withdrawal: 'platform.copy.c2a6a429',
  commission: 'platform.copy.648ed708', rewards: 'platform.copy.4eed66c1', reward_orders: 'platform.copy.1a2d2e89',
}
const groupLabels: Record<string, string> = {
  day: 'platform.copy.8a8d787b', game: 'platform.copy.20fdabb8', member: 'platform.copy.da8480f3', entry_type: 'platform.copy.53b1f7cc',
  state: 'platform.copy.78182fb4', agent: 'platform.copy.e592fcdf', cycle: 'platform.copy.f6f6ceea', order: 'platform.copy.68419640',
  reviewing: 'platform.copy.1be3df4b', processing: 'platform.copy.c0805393', paid: 'platform.copy.35188d0b', rejected: 'platform.copy.152a0128',
  failed: 'platform.copy.f628af96', cancelled: 'platform.copy.d9b0562c', granted: 'platform.copy.21e83e2d', revocation_pending: 'platform.copy.02e5e19d', revoked: 'platform.copy.97fc3c9b',
}
const groupOptions = computed(() => REPORT_GROUPS[kind.value].map(value => ({ value, label: label(groupLabels, value) })))
const note = computed(() => ({ betting: copy.value.bettingNote, withdrawal: copy.value.withdrawalNote, reward_orders: copy.value.rewardOrdersNote, commission: copy.value.commissionNote, rewards: copy.value.rewardsNote, ledger: copy.value.ledgerNote })[kind.value])
function label(labels: Record<string, string>, key: string) {
  const translationKey = labels[key]
  if (!translationKey) throw new Error(`Unsupported report label: ${key}`)
  return ui(translationKey)
}
function clearReport() { generation++; report.value = null; submitted.value = null; offset.value = 0; loading.value = false; error.value = ''; exportRequest.value = null; exporting.value = false; exportNotice.value = '' }
function edited() { if (report.value || submitted.value || loading.value) clearReport() }
watch(kind, () => { groupBy.value = 'day'; gameFilter.value = ''; agentFilter.value = ''; cycleFilter.value = ''; orderFilter.value = ''; edited() })
watch(() => props.brandId, () => {
  clearReport(); from.value = ''; to.value = ''; groupBy.value = 'day'
  memberFilter.value = ''; gameFilter.value = ''; agentFilter.value = ''; cycleFilter.value = ''; orderFilter.value = ''
}, { flush: 'sync' })
onBeforeUnmount(() => { generation++; exportRequest.value = null })

async function makeQuery(pageOffset: number): Promise<ReportQuery> {
  const query: ReportQuery = { ...reportDateRange(from.value, to.value), group_by: groupBy.value, limit: 50, offset: pageOffset }
  const username = memberFilter.value.trim()
  const optional: Partial<Record<'member_id' | 'game_id' | 'agent_id' | 'cycle_id' | 'order_id', string>> = {
    ...(username ? { member_id: username } : {}), game_id: gameFilter.value.trim(), agent_id: agentFilter.value.trim(), cycle_id: cycleFilter.value.trim(), order_id: orderFilter.value.trim(),
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
  const previousQuery = submitted.value?.query
  report.value = null; submitted.value = null; offset.value = nextOffset; error.value = ''; loading.value = true
  try {
    const query = previousQuery && nextOffset !== 0 ? { ...previousQuery, offset: nextOffset } : await makeQuery(nextOffset)
    if (request !== generation || brand !== props.brandId) return
    const frozen = { kind: requestKind, query: { ...query } }
    const result = await api.read(brand, requestKind, frozen.query)
    if (request !== generation || brand !== props.brandId) return
    report.value = result; submitted.value = frozen; offset.value = nextOffset
  } catch (cause) {
    if (request !== generation) return
    if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
    else error.value = cause instanceof Error ? publicErrorMessage(cause) : publicErrorMessage(cause)
  } finally { if (request === generation) loading.value = false }
}
async function page(nextOffset: number) { if (submitted.value) await load(nextOffset) }
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
    exportNotice.value = `${copy.value.exported}: ${result.filename} · ${copy.value.total}: ${result.groupCount} · ${copy.value.snapshot}: ${formatPlatformDateTime(result.snapshotAt, props.displaySettings, props.locale)} · ${copy.value.audit}: ${result.auditLogId}`
    exportRequest.value = null
  } catch (cause) {
    if (request !== generation) return
    if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
    else error.value = cause instanceof Error ? publicErrorMessage(cause) : publicErrorMessage(cause)
    exportRequest.value = null
  } finally { if (request === generation) exporting.value = false }
}
const fields = computed(() => REPORT_FIELDS[report.value ? submitted.value!.kind : kind.value])
const hasNextPage = computed(() => report.value !== null && BigInt(offset.value + report.value.items.length) < BigInt(report.value.total_groups))
</script>

<template>
  <div data-testid="platform-reports">
    <form novalidate v-inline-validation class="panel wallet-query report-filters" @submit.prevent="withGlobalLoading(load, 0)">
      <label class="query-field">
        <span class="field-label">{{ copy.type }}</span>
        <select v-model="kind" :aria-label="copy.type" :disabled="loading">
          <option v-for="option in kinds" :key="option" :value="option">{{ label(kindLabels, option) }}</option>
        </select>
      </label>
      <label class="query-field report-date-field">
        <span class="field-label">{{ ui('platform.reports.fromDate') }}</span>
        <PlatformTemporalInput v-model="from" type="date" :aria-label="ui('platform.reports.fromDate')" :locale="locale" required :disabled="loading" @update:model-value="edited" />
      </label>
      <label class="query-field report-date-field">
        <span class="field-label">{{ ui('platform.reports.toDate') }}</span>
        <PlatformTemporalInput v-model="to" type="date" :aria-label="ui('platform.reports.toDate')" :locale="locale" required :disabled="loading" @update:model-value="edited" />
      </label>
      <label class="query-field">
        <span class="field-label">{{ copy.group }}</span>
        <select v-model="groupBy" :aria-label="copy.group" required :disabled="loading" @change="edited">
          <option v-for="option in groupOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
        </select>
      </label>
      <label class="query-field report-member-field">
        <span class="field-label">{{ ui('filter.member') }}</span>
        <FilterAssociationSelect v-model="memberFilter" :brand-id="brandId" surface="platform" :resource="`report_${kind}`" kind="member" :label="ui('filter.member')" :locale="locale" :disabled="loading" @update:model-value="edited" />
      </label>
      <label v-if="kind === 'betting'" class="query-field">
        <span class="field-label">{{ ui('filter.game') }}</span>
        <FilterAssociationSelect v-model="gameFilter" :brand-id="brandId" surface="platform" :resource="`report_${kind}`" kind="game" :label="ui('filter.game')" :locale="locale" :disabled="loading" @update:model-value="edited" />
      </label>
      <label v-if="kind === 'commission'" class="query-field">
        <span class="field-label">{{ ui('filter.agent') }}</span>
        <FilterAssociationSelect v-model="agentFilter" :brand-id="brandId" surface="platform" :resource="`report_${kind}`" kind="agent" :label="ui('filter.agent')" :locale="locale" :disabled="loading" @update:model-value="edited" />
      </label>
      <label v-if="kind === 'commission'" class="query-field">
        <span class="field-label">{{ ui('filter.cycle') }}</span>
        <FilterAssociationSelect v-model="cycleFilter" :brand-id="brandId" surface="platform" :resource="`report_${kind}`" kind="cycle" :label="ui('filter.cycle')" :locale="locale" :disabled="loading" @update:model-value="edited" />
      </label>
      <label v-if="kind === 'rewards' || kind === 'reward_orders'" class="query-field">
        <span class="field-label">{{ ui('filter.reward_order') }}</span>
        <FilterAssociationSelect v-model="orderFilter" :brand-id="brandId" surface="platform" :resource="`report_${kind}`" kind="reward_order" :label="ui('filter.reward_order')" :locale="locale" :disabled="loading" @update:model-value="edited" />
      </label>
      <button class="button button-primary" :disabled="loading || !brandId">{{ copy.run }}</button>
    </form>
    <p v-if="error" class="message error" role="alert">{{ displayPublicError(error) }}</p>
    <p v-if="exportNotice" class="message success" role="status" data-testid="platform-report-export-receipt">{{ exportNotice }}</p>
    <template v-if="report && submitted">
      <p class="message" role="note">{{ note }}</p>
      <div class="wallet-pagination"><button class="button button-secondary" :disabled="loading || exporting" @click="reviewExport">{{ copy.export }}</button></div>
      <p v-if="submitted.kind === 'reward_orders'" class="message" role="note">{{ copy.rewardPendingNote }}</p>
      <section class="panel reward-detail" data-testid="platform-reports-summary">
        <div class="panel-heading"><div><h2>{{ copy.summary }}</h2><p>{{ copy.snapshot }}: {{ formatPlatformDateTime(report.snapshot_at, props.displaySettings, props.locale) }} · {{ copy.timezone }}: {{ report.timezone }}</p></div></div>
        <dl class="confirm-list reward-fields">
          <div v-for="field in fields" :key="field"><dt>{{ label(metricLabels, field) }}</dt><dd>{{ report.summary[field] }}</dd></div>
        </dl>
      </section>
      <section v-if="report.balances" class="panel reward-detail" data-testid="platform-reports-balances">
        <div class="panel-heading"><div><h2>{{ copy.balances }}</h2><p>{{ copy.balanceNote }}</p></div></div>
        <dl class="confirm-list reward-fields"><div v-for="field in balanceFields" :key="field"><dt>{{ label(metricLabels, field) }}</dt><dd>{{ report.balances[field] }}</dd></div></dl>
      </section>
      <section class="panel reward-detail" data-testid="platform-reports-list">
        <div class="panel-heading"><h2>{{ copy.groups }}</h2></div>
        <div class="table-wrap"><table><thead><tr><th>{{ label(groupLabels, submitted.query.group_by) }}</th><th v-for="field in fields" :key="field">{{ label(metricLabels, field) }}</th></tr></thead><tbody>
          <tr v-for="item in report.items" :key="item.key"><td>{{ submitted.query.group_by === 'state' ? label(groupLabels, item.key) : associationGroupLabel(item, submitted.query.group_by, item.key, item.label, associationLocale) }}</td><td v-for="field in fields" :key="field">{{ item.totals[field] }}</td></tr>
          <tr v-if="!report.items.length"><td :colspan="fields.length + 1" class="empty-state">{{ copy.empty }}</td></tr>
        </tbody></table></div>
        <div class="wallet-pagination" data-testid="platform-reports-pages"><span>{{ copy.total }}: {{ report.total_groups }}</span><button class="button button-secondary" :disabled="loading || offset === 0" @click="withGlobalLoading(page, Math.max(0, offset - 50))">{{ copy.previous }}</button><span>{{ Math.floor(offset / 50) + 1 }}</span><button class="button button-secondary" :disabled="loading || !hasNextPage" @click="withGlobalLoading(page, offset + 50)">{{ copy.next }}</button></div>
      </section>
    </template>
    <div v-if="exportRequest" class="modal-scrim"><section class="modal-card" role="dialog" aria-modal="true" :aria-label="copy.exportTitle">
      <h2>{{ copy.exportTitle }}</h2><p class="modal-intro">{{ copy.exportNote }}</p>
      <dl class="confirm-list">
        <div><dt>{{ copy.brand }}</dt><dd><AssociationName kind="brand" :id="exportRequest.brandId" :locale="associationLocale" /></dd></div>
        <div><dt>{{ copy.type }}</dt><dd>{{ label(kindLabels, exportRequest.kind) }}</dd></div>
        <div><dt>{{ copy.from }}</dt><dd>{{ formatPlatformDateTime(exportRequest.query.from, props.displaySettings, props.locale) }}</dd></div><div><dt>{{ copy.to }}</dt><dd>{{ formatPlatformDateTime(exportRequest.query.to, props.displaySettings, props.locale) }}</dd></div>
        <div><dt>{{ copy.group }}</dt><dd>{{ label(groupLabels, exportRequest.query.group_by) }}</dd></div>
        <div v-if="exportRequest.query.member_id"><dt>{{ copy.member }}</dt><dd><AssociationName :record="exportRequest.query" kind="member" :id="exportRequest.query.member_id" :locale="associationLocale" /></dd></div>
        <div v-if="exportRequest.query.game_id"><dt>{{ copy.game }}</dt><dd><AssociationName :record="exportRequest.query" kind="game" :id="exportRequest.query.game_id" :locale="associationLocale" /></dd></div>
        <div v-if="exportRequest.query.agent_id"><dt>{{ copy.agent }}</dt><dd><AssociationName :record="exportRequest.query" kind="agent" :id="exportRequest.query.agent_id" :locale="associationLocale" /></dd></div>
        <div v-if="exportRequest.query.cycle_id"><dt>{{ copy.cycle }}</dt><dd><AssociationName :record="report?.query" kind="cycle" :id="exportRequest.query.cycle_id" :locale="associationLocale" /></dd></div>
        <div v-if="exportRequest.query.order_id"><dt>{{ copy.order }}</dt><dd>{{ exportRequest.query.order_id }}</dd></div>
      </dl><div class="form-actions"><button class="button button-secondary" :disabled="exporting" @click="exportRequest = null">{{ copy.cancel }}</button><button class="button button-primary" :disabled="exporting" @click="withGlobalLoading(confirmExport)">{{ exporting ? copy.exporting : copy.confirm }}</button></div>
    </section></div>
  </div>
</template>

<style scoped>
.report-filters.wallet-query { gap: 10px; padding: 14px; }
.report-filters.wallet-query .query-field { flex: 0 0 auto !important; min-width: 0 !important; max-width: 100%; }
.report-filters.wallet-query .report-date-field { width: 148px; }
.report-filters.wallet-query .report-member-field { width: 168px; }
.report-filters.wallet-query .input-with-icon { flex: 1 1 auto !important; width: 100% !important; }
.report-filters.wallet-query .query-field > input { width: 168px; max-width: 100%; }
.report-filters :deep(.platform-date-input-display) { height: 36px; min-height: 36px; padding-right: 34px; font-size: 14px; }
.report-filters :deep(.platform-date-input-button) { width: 32px; }
.report-filters > .button { flex-shrink: 0; }
@media (max-width: 480px) {
  .report-filters.wallet-query .query-field { flex: 1 1 130px !important; }
}
</style>
