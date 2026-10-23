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
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { PlatformApiError } from './platform-api'
import { createPlatformArchiveTasksApi } from './archive-tasks-api'
import { formatPlatformDateTime, type PlatformDisplaySettings } from './platform-format'

const props = defineProps<{ brandId: string; locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformArchiveTasksApi()
type Policy = Awaited<ReturnType<typeof api.policy>>
type Task = Awaited<ReturnType<typeof api.read>>
type TaskPage = Awaited<ReturnType<typeof api.list>>
const policy = ref<Policy | null>(null)
const page = ref<TaskPage | null>(null)
const detail = ref<Task | null>(null)
const selectedId = ref('')
const offset = ref(0)
const policyLoading = ref(false), listLoading = ref(false), detailLoading = ref(false)
const policyError = ref<string | import("@lottery/shared").LocalizedMessage>(""), listError = ref<string | import("@lottery/shared").LocalizedMessage>(""), detailError = ref<string | import("@lottery/shared").LocalizedMessage>("")
let alive = true, policyGeneration = 0, listGeneration = 0, detailGeneration = 0
const pageSize = 50
const hasNextPage = computed(() => !!page.value && BigInt(page.value.total_count) > BigInt(page.value.offset + pageSize))
const copy = computed(() => props.locale === 'en' ? {
  title: ui('platform.copy.c2008cbd'), intro: ui('platform.copy.7549d44d'),
  policy: ui('platform.copy.09fc8de0'), refreshPolicy: ui('platform.copy.349ebfec'),
  daily: ui('platform.copy.613699cd'), monthly: ui('platform.copy.0f5dcca2'), dailyStart: ui('platform.copy.94e8f170'), monthlyStart: ui('platform.copy.894d9f27'), timezone: ui('platform.copy.c67c04be'), version: ui('platform.copy.9e14bb00'), audit: ui('platform.copy.e15ea87c'), updated: ui('platform.copy.4eca7621'),
  activation: ui('platform.copy.dd841135'),
  list: ui('platform.copy.d70bdcbf'), total: ui('platform.copy.a3e0fc7a'), page: ui('platform.copy.6695ff1c'), previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), empty: ui('platform.copy.acca2c7a'), id: ui('platform.copy.099dc9a8'), kind: ui('platform.copy.58ea1af7'), dailyKind: ui('platform.copy.5238d49e'), monthlyKind: ui('platform.copy.dfecc78d'), period: ui('platform.copy.f9128507'), state: ui('platform.copy.274ae047'), pending: ui('platform.copy.7174c9b9'), completed: ui('platform.copy.17c14130'), skipped: ui('platform.copy.c766815f'), failed: ui('platform.copy.f628af96'), attempts: ui('platform.copy.9e780c26'), taskVersion: ui('platform.copy.193bd714'), policyVersion: ui('platform.copy.9e14bb00'), archiveId: ui('platform.copy.0d7979b2'), lastError: ui('platform.copy.8a054f73'), created: ui('platform.copy.f40187a7'), latestAudit: ui('platform.copy.1409b00a'), updatedTask: ui('platform.copy.4eca7621'), detail: ui('platform.copy.f2812642'), close: ui('platform.copy.27b2fd29'), refresh: ui('platform.copy.256fe8b3'), window: ui('platform.copy.32024b02'), from: ui('platform.copy.903f0ee2'), to: ui('platform.copy.19619e16'), creationAudit: ui('platform.copy.a3d9d216'), select: ui('platform.copy.2ec6dd3a'), skippedNote: ui('platform.copy.21b6cf9c'), readFailure: ui('platform.copy.f36114e8'), policyFailure: ui('platform.copy.af43d9a4'), listFailure: ui('platform.copy.19c43649'), readDetailFailure: ui('platform.copy.9bdc4bd8'),
} : {
  title: ui('platform.copy.c2008cbd'), intro: ui('platform.copy.7549d44d'),
  policy: ui('platform.copy.09fc8de0'), refreshPolicy: ui('platform.copy.349ebfec'),
  daily: ui('platform.copy.613699cd'), monthly: ui('platform.copy.0f5dcca2'), dailyStart: ui('platform.copy.94e8f170'), monthlyStart: ui('platform.copy.894d9f27'), timezone: ui('platform.copy.c67c04be'), version: ui('platform.copy.9e14bb00'), audit: ui('platform.copy.e15ea87c'), updated: ui('platform.copy.4eca7621'),
  activation: ui('platform.copy.dd841135'),
  list: ui('platform.copy.d70bdcbf'), total: ui('platform.copy.a3e0fc7a'), page: ui('platform.copy.6695ff1c'), previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), empty: ui('platform.copy.acca2c7a'), id: ui('platform.copy.099dc9a8'), kind: ui('platform.copy.58ea1af7'), dailyKind: ui('platform.copy.5238d49e'), monthlyKind: ui('platform.copy.dfecc78d'), period: ui('platform.copy.f9128507'), state: ui('platform.copy.274ae047'), pending: ui('platform.copy.7174c9b9'), completed: ui('platform.copy.17c14130'), skipped: ui('platform.copy.c766815f'), failed: ui('platform.copy.f628af96'), attempts: ui('platform.copy.9e780c26'), taskVersion: ui('platform.copy.193bd714'), policyVersion: ui('platform.copy.9e14bb00'), archiveId: ui('platform.copy.0d7979b2'), lastError: ui('platform.copy.8a054f73'), created: ui('platform.copy.f40187a7'), latestAudit: ui('platform.copy.1409b00a'), updatedTask: ui('platform.copy.4eca7621'), detail: ui('platform.copy.f2812642'), close: ui('platform.copy.27b2fd29'), refresh: ui('platform.copy.256fe8b3'), window: ui('platform.copy.32024b02'), from: ui('platform.copy.903f0ee2'), to: ui('platform.copy.19619e16'), creationAudit: ui('platform.copy.a3d9d216'), select: ui('platform.copy.2ec6dd3a'), skippedNote: ui('platform.copy.21b6cf9c'), readFailure: ui('platform.copy.f36114e8'), policyFailure: ui('platform.copy.af43d9a4'), listFailure: ui('platform.copy.19c43649'), readDetailFailure: ui('platform.copy.9bdc4bd8'),
})

function handleFailure(cause: unknown): string | import("@lottery/shared").LocalizedMessage {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  return cause instanceof Error ? publicErrorMessage(cause) : publicErrorMessage(cause)
}
function current(ticket: number, lane: 'policy' | 'list' | 'detail', brand: string): boolean {
  const actual = lane === 'policy' ? policyGeneration : lane === 'list' ? listGeneration : detailGeneration
  return alive && ticket === actual && brand === props.brandId
}
async function loadPolicy() {
  const ticket = ++policyGeneration, brand = props.brandId
  policy.value = null; policyError.value = ''
  if (!brand) { policyLoading.value = false; return }
  policyLoading.value = true
  try {
    const result = await api.policy(brand)
    if (current(ticket, 'policy', brand)) policy.value = result
  } catch (cause) { if (current(ticket, 'policy', brand)) { policy.value = null; policyError.value = handleFailure(cause) } }
  finally { if (current(ticket, 'policy', brand)) policyLoading.value = false }
}
async function loadList(nextOffset = 0) {
  const ticket = ++listGeneration, brand = props.brandId
  page.value = null; detail.value = null; selectedId.value = ''; detailError.value = ''; offset.value = nextOffset; listError.value = ''
  detailGeneration++; detailLoading.value = false
  if (!brand) { listLoading.value = false; return }
  listLoading.value = true
  try {
    const result = await api.list(brand, pageSize, nextOffset)
    if (current(ticket, 'list', brand)) { page.value = result; offset.value = result.offset }
  } catch (cause) { if (current(ticket, 'list', brand)) { page.value = null; listError.value = handleFailure(cause) } }
  finally { if (current(ticket, 'list', brand)) listLoading.value = false }
}
async function readTask(task: Task) {
  const ticket = ++detailGeneration, brand = props.brandId, id = task.id
  selectedId.value = id; detail.value = null; detailError.value = ''
  if (!brand) { detailLoading.value = false; return }
  detailLoading.value = true
  try {
    const result = await api.read(brand, id)
    if (current(ticket, 'detail', brand) && selectedId.value === id) detail.value = result
  } catch (cause) {
    if (current(ticket, 'detail', brand) && selectedId.value === id) { detail.value = null; detailError.value = handleFailure(cause) }
  } finally { if (current(ticket, 'detail', brand)) detailLoading.value = false }
}
function stateLabel(state: string): string {
  const labels: Record<string, [string, string]> = { pending: [ui('platform.copy.7174c9b9'), ui('platform.copy.7174c9b9')], completed: [ui('platform.copy.17c14130'), ui('platform.copy.17c14130')], skipped: [ui('platform.copy.c766815f'), ui('platform.copy.c766815f')], failed: [ui('platform.copy.f628af96'), ui('platform.copy.f628af96')] }
  const label = labels[state]
  if (!label) throw new Error(`Unsupported archive task state: ${state}`)
  return label[props.locale === 'en' ? 0 : 1]
}
watch(() => props.brandId, () => { void withGlobalLoading(loadPolicy); void withGlobalLoading(loadList, 0) }, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { alive = false; policyGeneration++; listGeneration++; detailGeneration++ })
</script>

<template>
  <div data-testid="platform-archive-tasks">
    <template>
      <section class="panel reward-detail" data-testid="platform-archive-policy">
        <div class="panel-heading"><h2>{{ copy.policy }}</h2><button class="button button-secondary" :disabled="policyLoading" @click="withGlobalLoading(loadPolicy)">{{ copy.refreshPolicy }}</button></div>
        <p v-if="policyError" class="message error" role="alert">{{ copy.policyFailure }}{{ displayPublicError(policyError) }}</p>
        <template v-if="policy">
          <dl class="confirm-list reward-fields">

            <div><dt>{{ copy.daily }}</dt><dd>{{ policy.daily_enabled ? '✓' : '—' }}</dd></div>
            <div><dt>{{ copy.dailyStart }}</dt><dd>{{ policy.daily_start_period ?? '—' }}</dd></div>
            <div><dt>{{ copy.monthly }}</dt><dd>{{ policy.monthly_enabled ? '✓' : '—' }}</dd></div>
            <div><dt>{{ copy.monthlyStart }}</dt><dd>{{ policy.monthly_start_period ?? '—' }}</dd></div>
            <div><dt>{{ copy.timezone }}</dt><dd>{{ policy.timezone }}</dd></div>
            <div><dt>{{ copy.updated }}</dt><dd>{{ formatPlatformDateTime(policy.updated_at, props.displaySettings, props.locale) }}</dd></div>

          </dl>
        </template>
        <p class="message">{{ copy.activation }}</p>
      </section>

      <section class="panel reward-detail" data-testid="platform-archive-task-list">
        <div class="panel-heading"><h2>{{ copy.list }}</h2></div>
        <p v-if="listError" class="message error" role="alert">{{ copy.listFailure }}{{ displayPublicError(listError) }}</p>
        <div v-if="page" class="table-wrap"><table><thead><tr><th>{{ copy.kind }}</th><th>{{ copy.period }}</th><th><BusinessDetails :value="copy.state" :locale="props.locale" /></th><th>{{ copy.attempts }}</th><th>{{ copy.lastError }}</th><th>{{ copy.created }}</th><th>{{ copy.updatedTask }}</th></tr></thead><tbody>
          <tr v-for="task in page.items" :key="task.id" :aria-current="selectedId === task.id ? 'true' : undefined"><td>{{ task.window.kind === 'daily' ? copy.dailyKind : copy.monthlyKind }}</td><td><button class="row-action" @click="withGlobalLoading(readTask, task)">{{ task.window.period_key }}</button></td><td><span class="status-pill" :class="task.state">{{ stateLabel(task.state) }}</span></td><td>{{ task.attempt_count }}</td><td>{{ task.last_error_code ? displayPublicError(publicErrorMessage({ code: task.last_error_code })) : "—" }}</td><td>{{ formatPlatformDateTime(task.created_at, props.displaySettings, props.locale) }}</td><td>{{ formatPlatformDateTime(task.updated_at, props.displaySettings, props.locale) }}</td></tr>
          <tr v-if="!page.items.length"><td colspan="7" class="empty-state">{{ copy.empty }}</td></tr>
        </tbody></table></div>
        <div class="wallet-pagination" data-testid="platform-archive-task-pages"><span>{{ copy.total }}: {{ page?.total_count ?? '0' }}</span><button class="button button-secondary" :disabled="listLoading || offset === 0" @click="withGlobalLoading(loadList, Math.max(0, offset - pageSize))">{{ copy.previous }}</button><span>{{ Math.floor(offset / pageSize) + 1 }}</span><button class="button button-secondary" :disabled="listLoading || !hasNextPage" @click="withGlobalLoading(loadList, offset + pageSize)">{{ copy.next }}</button><button v-refresh-button="copy.refresh" class="button button-secondary" :disabled="listLoading" @click="withGlobalLoading(loadList, offset)">{{ copy.refresh }}</button></div>
      </section>

      <DetailModal v-if="!!selectedId" :title="copy.detail" :close-label="copy.close" @close="selectedId = ''; detail = null; detailError = ''">
<section class="detail-section" data-testid="platform-archive-task-detail">
        <p v-if="detailError" class="message error" role="alert">{{ copy.readDetailFailure }}{{ displayPublicError(detailError) }}</p>
        <template v-if="detail">
          <dl class="confirm-list reward-fields">
            <div><dt>{{ copy.id }}</dt><dd>{{ detail.id }}</dd></div>
            <div><dt><BusinessDetails :value="copy.state" :locale="props.locale" /></dt><dd>{{ stateLabel(detail.state) }}</dd></div><div><dt>{{ copy.attempts }}</dt><dd>{{ detail.attempt_count }}</dd></div><div><dt>{{ copy.archiveId }}</dt><dd>{{ detail.archive_id ?? '—' }}</dd></div><div><dt>{{ copy.lastError }}</dt><dd>{{ detail.last_error_code ? displayPublicError(publicErrorMessage({ code: detail.last_error_code })) : "—" }}</dd></div>
            <div><dt>{{ copy.window }}</dt><dd>{{ detail.window.kind === 'daily' ? copy.dailyKind : copy.monthlyKind }} · {{ detail.window.period_key }}</dd></div><div><dt>{{ copy.timezone }}</dt><dd>{{ detail.window.timezone }}</dd></div><div><dt>{{ copy.from }}</dt><dd>{{ formatPlatformDateTime(detail.window.from, props.displaySettings, props.locale) }}</dd></div><div><dt>{{ copy.to }}</dt><dd>{{ formatPlatformDateTime(detail.window.to, props.displaySettings, props.locale) }}</dd></div>
            <div><dt>{{ copy.created }}</dt><dd>{{ formatPlatformDateTime(detail.created_at, props.displaySettings, props.locale) }}</dd></div><div><dt>{{ copy.updatedTask }}</dt><dd>{{ formatPlatformDateTime(detail.updated_at, props.displaySettings, props.locale) }}</dd></div>
          </dl>
          <p v-if="detail.state === 'skipped'" class="message">{{ copy.skippedNote }}</p>
        </template>
        <p v-else-if="!detailLoading && !detailError" class="message">{{ copy.select }}</p>
      </section>
</DetailModal>
    </template>
  </div>
</template>
