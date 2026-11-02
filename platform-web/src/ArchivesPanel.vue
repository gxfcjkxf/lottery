<script setup lang="ts">
import ManagementList from "@lottery/shared/ManagementList.vue"
import TablePagination from "@lottery/shared/TablePagination.vue"
import { useOperationToast } from "@lottery/shared";
import { usePlatformI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { usePlatformI18n } from "./i18n"
const ui = usePlatformI18n().t
import DetailModal from "@lottery/shared/DetailModal.vue"
import ArchiveReport from "@lottery/shared/ArchiveReport.vue"
import { archiveReportCatalog } from '@lottery/shared'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { PlatformApiError } from './platform-api'
import { createPlatformArchivesApi, type ArchivePage, type ArchiveRecord } from './archives-api'
import { REPORT_FIELDS } from './reports-api'
import { REPORT_METRIC_KEYS } from './report-labels'
import { formatPlatformDateTime, type PlatformDisplaySettings } from './platform-format'

const props = defineProps<{ brandId: string; locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformArchivesApi()
const page = ref<ArchivePage | null>(null)
const detail = ref<ArchiveRecord | null>(null)
const selectedId = ref('')
const offset = ref(0)
const kind = ref<'daily'|'monthly'>('daily')
const archiveCopy=computed(()=>archiveReportCatalog[props.locale])
function chooseKind(value:'daily'|'monthly'){if(kind.value===value)return;kind.value=value;backToList();void withGlobalLoading(load,0)}
const loading = ref(false)
const downloading = ref(false)
const error = ref<string | import("@lottery/shared").LocalizedMessage>("")
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
const reportLabels = computed(() => ({ ...Object.fromEntries(groups.map(group => [group.key, groupLabel(group.key)])), wallet: copy.value.balances }))
const reportTime = (value: string) => formatPlatformDateTime(value, props.displaySettings, props.locale)
function backToList() { generation++; detail.value = null; selectedId.value = ''; loading.value = false; downloading.value = false; error.value = ''; receipt.value = '' }
const copy = computed(() => props.locale === 'en' ? {
  title: ui('platform.copy.eaf765c0'), intro: ui('platform.copy.1309db8a'),
  refresh: ui('platform.copy.256fe8b3'), empty: ui('platform.copy.557531d9'), previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), page: ui('platform.copy.6695ff1c'), total: ui('platform.copy.8e24db68'),
  detail: ui('platform.copy.d33d7140'), close: ui('platform.copy.27b2fd29'), window: ui('platform.copy.859e1f82'), kind: ui('platform.copy.58ea1af7'), daily: ui('platform.copy.5238d49e'), monthly: ui('platform.copy.dfecc78d'), period: ui('platform.copy.f9128507'), timezone: ui('platform.copy.c67c04be'), from: ui('platform.copy.903f0ee2'), to: ui('platform.copy.19619e16'),
  version: ui('platform.copy.575743df'), revision: ui('platform.copy.0f6b5f8c'), previousId: ui('platform.copy.e6714df6'), creator: ui('platform.copy.f50751ac'), automation: ui('platform.copy.fc87077a'), task: ui('platform.copy.099dc9a8'), policyVersion: ui('platform.copy.9e14bb00'),
  reason: ui('platform.copy.f3f300a4'), hash: ui('platform.copy.2f54690f'), audit: ui('platform.copy.e15ea87c'), snapshotAt: ui('platform.copy.c8fc9d36'), createdAt: ui('platform.copy.f40187a7'), summaries: ui('platform.copy.c3d821c2'),
  balances: ui('platform.copy.d876624a'), balanceNote: ui('platform.copy.a2d25805'),
  walletAt: ui('platform.copy.74d7df61'), download: ui('platform.copy.d4df321a'), downloading: ui('platform.copy.84e842a4'), downloaded: ui('platform.copy.abed06fb'), emptyDetail: ui('platform.copy.8aa00f81'),
} : {
  title: ui('platform.copy.eaf765c0'), intro: ui('platform.copy.1309db8a'),
  refresh: ui('platform.copy.256fe8b3'), empty: ui('platform.copy.557531d9'), previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), page: ui('platform.copy.6695ff1c'), total: ui('platform.copy.8e24db68'),
  detail: ui('platform.copy.d33d7140'), close: ui('platform.copy.27b2fd29'), window: ui('platform.copy.859e1f82'), kind: ui('platform.copy.58ea1af7'), daily: ui('platform.copy.5238d49e'), monthly: ui('platform.copy.dfecc78d'), period: ui('platform.copy.f9128507'), timezone: ui('platform.copy.c67c04be'), from: ui('platform.copy.903f0ee2'), to: ui('platform.copy.19619e16'),
  version: ui('platform.copy.575743df'), revision: ui('platform.copy.0f6b5f8c'), previousId: ui('platform.copy.e6714df6'), creator: ui('platform.copy.f50751ac'), automation: ui('platform.copy.fc87077a'), task: ui('platform.copy.099dc9a8'), policyVersion: ui('platform.copy.9e14bb00'),
  reason: ui('platform.copy.f3f300a4'), hash: ui('platform.copy.2f54690f'), audit: ui('platform.copy.e15ea87c'), snapshotAt: ui('platform.copy.c8fc9d36'), createdAt: ui('platform.copy.f40187a7'), summaries: ui('platform.copy.c3d821c2'),
  balances: ui('platform.copy.d876624a'), balanceNote: ui('platform.copy.a2d25805'),
  walletAt: ui('platform.copy.74d7df61'), download: ui('platform.copy.d4df321a'), downloading: ui('platform.copy.84e842a4'), downloaded: ui('platform.copy.abed06fb'), emptyDetail: ui('platform.copy.8aa00f81'),
})

function metricLabel(field: string): string {
  const key = REPORT_METRIC_KEYS[field]
  if (!key) throw new Error(`Unsupported report metric label: ${field}`)
  return ui(key)
}
function groupLabel(key: typeof groups[number]['key']): string {
  const labels = props.locale === 'en'
    ? { betting: ui('platform.copy.4e6493d5'), ledger: ui('platform.copy.748ef186'), withdrawals: ui('platform.copy.c2a6a429'), commissions: ui('platform.copy.648ed708'), rewards: ui('platform.copy.4eed66c1'), reward_orders: ui('platform.copy.1a2d2e89') }
    : { betting: ui('platform.copy.4e6493d5'), ledger: ui('platform.copy.748ef186'), withdrawals: ui('platform.copy.c2a6a429'), commissions: ui('platform.copy.648ed708'), rewards: ui('platform.copy.4eed66c1'), reward_orders: ui('platform.copy.1a2d2e89') }
  return labels[key]
}
function reportFailure(cause: unknown) {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  else error.value = cause instanceof Error ? publicErrorMessage(cause) : publicErrorMessage(cause)
}
async function load(nextOffset = 0) {
  const request = ++generation
  const brand = props.brandId
  detail.value = null
  selectedId.value = ''
  error.value = ''
  receipt.value = ''
  downloading.value = false
  if (!brand) { loading.value = false; return }
  loading.value = true
  try {
    const result = await api.list(brand, pageSize, nextOffset, kind.value)
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
    receipt.value = `${copy.value.downloaded}: ${result.filename}`
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
  void withGlobalLoading(load, 0)
}, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => {
  generation++
  for (const url of [...objectUrls]) releaseObjectUrl(url)
})
useOperationToast(receipt, value => value, 'success');

useOperationToast(error, value => displayPublicError(value), 'error');
</script>

<template>
  <div data-testid="platform-archives">


    <ManagementList class="reward-detail" data-testid="platform-archive-list">
      <template #tabs><div class="archive-tabs" role="tablist" :aria-label="archiveCopy.records"><button v-for="tab in (['daily','monthly'] as const)" :key="tab" role="tab" class="button" :class="kind===tab?'button-primary':'button-secondary'" :aria-selected="kind===tab" @click="chooseKind(tab)">{{ tab==='daily'?archiveCopy.dailyArchives:archiveCopy.monthlyArchives }}</button></div></template><template #actions><button v-refresh-button="copy.refresh" class="button button-secondary" :disabled="loading || !brandId" @click="withGlobalLoading(load, 0)">{{ copy.refresh }}</button></template>
      <div class="table-wrap"><table class="console-table console-responsive-table"><thead><tr><th>{{ archiveCopy.period }}</th><th>{{ archiveCopy.archiveType }}</th><th>{{ archiveCopy.status }}</th><th>{{ archiveCopy.archivedAt }}</th></tr></thead><tbody>
        <tr v-for="record in page?.items ?? []" :key="record.id"><td :data-label="String(archiveCopy.period)"><button class="text-button row-action" :disabled="loading" :title="copy.title" @click="withGlobalLoading(read, record)">{{ record.window.period_key }}</button></td><td :data-label="String(archiveCopy.archiveType)">{{ record.window.kind==='daily'?archiveCopy.dailyArchives:archiveCopy.monthlyArchives }}</td><td :data-label="String(archiveCopy.status)"><span class="status-pill completed">{{ archiveCopy.archived }}</span></td><td :data-label="String(archiveCopy.archivedAt)">{{ formatPlatformDateTime(record.snapshot_at, props.displaySettings, props.locale) }}</td></tr>
        <tr v-if="page && !page.items.length"><td colspan="4" class="empty-state">{{ copy.empty }}</td></tr>
      </tbody></table></div>

    <template #pagination><TablePagination :show-empty="Boolean(page)" class="wallet-pagination" data-testid="platform-archive-pages" :total="page?.total_count ?? '0'" :page="Math.floor(offset / pageSize + 1)" :count="page?.items.length ?? 0" :locale="locale" :page-size="pageSize" :previous-disabled="loading || offset === 0 || !brandId" :next-disabled="loading || !hasNextPage" :previous-label="copy.previous" :next-label="copy.next" :go-to="(target: number) => load((target - 1) * pageSize)" /></template></ManagementList>
    <DetailModal v-if="selectedId" :title="detail ? detail.window.period_key : copy.detail" :close-label="copy.close" @close="backToList">

    <section class="panel" data-testid="platform-archive-detail">
      <ArchiveReport v-if="detail" :record="detail" :locale="locale" :metric-label="metricLabel" :group-labels="reportLabels" :format-time="reportTime">
        <template #actions><button class="button button-primary" :disabled="loading || downloading" @click="withGlobalLoading(download)">{{ downloading ? copy.downloading : copy.download }}</button></template>
      </ArchiveReport>
    </section>
    </DetailModal>
  </div>
</template>

<style scoped>
.archive-tabs{display:flex;gap:8px;flex-wrap:wrap;margin:0}
</style>
