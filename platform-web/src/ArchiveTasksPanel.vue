<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { PlatformApiError } from './platform-api'
import { createPlatformArchiveTasksApi } from './archive-tasks-api'

const props = defineProps<{ brandId: string; locale: 'en' | 'zh-CN' }>()
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
const policyError = ref(''), listError = ref(''), detailError = ref('')
let alive = true, policyGeneration = 0, listGeneration = 0, detailGeneration = 0
const pageSize = 50
const hasNextPage = computed(() => !!page.value && BigInt(page.value.total_count) > BigInt(page.value.offset + pageSize))
const copy = computed(() => props.locale === 'en' ? {
  title: 'Automatic archive tasks', intro: 'Read-only policy and task history. Policy and task data are separate snapshots and may reflect different moments.',
  policy: 'Automatic archive policy', refreshPolicy: 'Refresh policy', refreshTasks: 'Refresh tasks', loading: 'Loading…', noBrand: 'Select a brand to view automatic archive tasks.',
  daily: 'Daily enabled', monthly: 'Monthly enabled', dailyStart: 'Daily start period', monthlyStart: 'Monthly start period', timezone: 'Timezone', version: 'Policy version', audit: 'Audit log ID', updated: 'Updated at',
  activation: 'First enablement starts from the current day or month in the brand timezone. Tasks are created after their period has elapsed. Reading this page does not enable the policy or retry a task.',
  list: 'Tasks', total: 'Total tasks', page: 'Page', previous: 'Previous page', next: 'Next page', empty: 'No automatic archive tasks.', id: 'Task ID', kind: 'Kind', dailyKind: 'Daily', monthlyKind: 'Monthly', period: 'Period', state: 'State', pending: 'Pending', completed: 'Completed', skipped: 'Skipped', failed: 'Failed', attempts: 'Attempts', taskVersion: 'Task version', policyVersion: 'Policy version', archiveId: 'Archive ID', lastError: 'Last error code', created: 'Created at', latestAudit: 'Latest audit log ID', updatedTask: 'Updated at', detail: 'Task detail', window: 'Frozen window', from: 'From', to: 'To', creationAudit: 'Creation audit log ID', select: 'Select a task to inspect its frozen window and audit fields.', skippedNote: 'A skipped task linked to an archive does not mean this task produced a new archive.', readFailure: 'Task read failed: ', policyFailure: 'Policy read failed: ', listFailure: 'Task list read failed: ', readDetailFailure: 'Selected task read failed: ',
} : {
  title: '自动归档任务', intro: '只读策略与任务历史。策略和任务是分别读取的快照，可能反映不同时间。',
  policy: '自动归档策略', refreshPolicy: '刷新策略', refreshTasks: '刷新任务', loading: '正在读取…', noBrand: '请选择品牌以查看自动归档任务。',
  daily: '启用每日归档', monthly: '启用每月归档', dailyStart: '每日起始期间', monthlyStart: '每月起始期间', timezone: '时区', version: '策略版本', audit: '审计日志编号', updated: '更新时间',
  activation: '首次启用从品牌时区的当前日或当前月开始；期间结束后才创建任务。读取此页面不会启用策略，也不会重试任务。',
  list: '任务', total: '任务总数', page: '页码', previous: '上一页', next: '下一页', empty: '暂无自动归档任务。', id: '任务编号', kind: '类型', dailyKind: '每日', monthlyKind: '每月', period: '期间', state: '状态', pending: '待处理', completed: '已完成', skipped: '已跳过', failed: '失败', attempts: '尝试次数', taskVersion: '任务版本', policyVersion: '策略版本', archiveId: '归档编号', lastError: '最近错误代码', created: '创建时间', latestAudit: '最近审计日志编号', updatedTask: '更新时间', detail: '任务详情', window: '冻结的时间范围', from: '开始', to: '结束', creationAudit: '创建审计日志编号', select: '请选择任务以查看冻结的时间范围和审计字段。', skippedNote: '已关联归档的跳过任务不代表该任务新生成了归档。', readFailure: '任务读取失败：', policyFailure: '策略读取失败：', listFailure: '任务列表读取失败：', readDetailFailure: '所选任务读取失败：',
})

function handleFailure(cause: unknown): string {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  return cause instanceof Error ? cause.message : String(cause)
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
  const labels: Record<string, [string, string]> = { pending: ['Pending', '待处理'], completed: ['Completed', '已完成'], skipped: ['Skipped', '已跳过'], failed: ['Failed', '失败'] }
  const label = labels[state]
  if (!label) throw new Error(`Unsupported archive task state: ${state}`)
  return label[props.locale === 'en' ? 0 : 1]
}
watch(() => props.brandId, () => { void loadPolicy(); void loadList(0) }, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { alive = false; policyGeneration++; listGeneration++; detailGeneration++ })
</script>

<template>
  <div data-testid="platform-archive-tasks">
    <div class="panel-heading"><div><h2>{{ copy.title }}</h2><p>{{ copy.intro }}</p></div></div>
    <p v-if="!brandId" class="message">{{ copy.noBrand }}</p>
    <template v-else>
      <section class="panel reward-detail" data-testid="platform-archive-policy">
        <div class="panel-heading"><h2>{{ copy.policy }}</h2><button class="secondary" :disabled="policyLoading" @click="loadPolicy">{{ copy.refreshPolicy }}</button></div>
        <p v-if="policyLoading" class="loading-line" role="status">{{ copy.loading }}</p>
        <p v-if="policyError" class="message error" role="alert">{{ copy.policyFailure }}{{ policyError }}</p>
        <template v-if="policy">
          <dl class="confirm-list reward-fields">
            <div><dt>{{ copy.version }}</dt><dd>{{ policy.version }}</dd></div>
            <div><dt>{{ copy.daily }}</dt><dd>{{ policy.daily_enabled ? '✓' : '—' }}</dd></div>
            <div><dt>{{ copy.dailyStart }}</dt><dd>{{ policy.daily_start_period ?? '—' }}</dd></div>
            <div><dt>{{ copy.monthly }}</dt><dd>{{ policy.monthly_enabled ? '✓' : '—' }}</dd></div>
            <div><dt>{{ copy.monthlyStart }}</dt><dd>{{ policy.monthly_start_period ?? '—' }}</dd></div>
            <div><dt>{{ copy.timezone }}</dt><dd>{{ policy.timezone }}</dd></div>
            <div><dt>{{ copy.updated }}</dt><dd>{{ policy.updated_at }}</dd></div>
            <div><dt>{{ copy.audit }}</dt><dd>{{ policy.audit_log_id ?? '—' }}</dd></div>
          </dl>
        </template>
        <p class="loading-line">{{ copy.activation }}</p>
      </section>

      <section class="panel reward-detail" data-testid="platform-archive-task-list">
        <div class="panel-heading"><h2>{{ copy.list }}</h2><button class="secondary" :disabled="listLoading" @click="loadList(offset)">{{ copy.refreshTasks }}</button></div>
        <p v-if="page" class="loading-line">{{ copy.total }}: {{ page.total_count }}</p>
        <p v-if="listLoading" class="loading-line" role="status">{{ copy.loading }}</p>
        <p v-if="listError" class="message error" role="alert">{{ copy.listFailure }}{{ listError }}</p>
        <div v-if="page" class="table-wrap"><table><thead><tr><th>{{ copy.id }}</th><th>{{ copy.kind }}</th><th>{{ copy.period }}</th><th>{{ copy.state }}</th><th>{{ copy.attempts }}</th><th>{{ copy.taskVersion }}</th><th>{{ copy.policyVersion }}</th><th>{{ copy.archiveId }}</th><th>{{ copy.lastError }}</th><th>{{ copy.created }}</th><th>{{ copy.latestAudit }}</th><th>{{ copy.updatedTask }}</th></tr></thead><tbody>
          <tr v-for="task in page.items" :key="task.id" :aria-current="selectedId === task.id ? 'true' : undefined"><td><button class="row-action mono" @click="readTask(task)">{{ task.id }}</button></td><td>{{ task.window.kind === 'daily' ? copy.dailyKind : copy.monthlyKind }}</td><td>{{ task.window.period_key }}</td><td><span class="status-pill" :class="task.state">{{ stateLabel(task.state) }}</span></td><td>{{ task.attempt_count }}</td><td>{{ task.version }}</td><td>{{ task.policy_version }}</td><td class="mono">{{ task.archive_id ?? '—' }}</td><td>{{ task.last_error_code ?? '—' }}</td><td>{{ task.created_at }}</td><td class="mono">{{ task.last_audit_log_id }}</td><td>{{ task.updated_at }}</td></tr>
          <tr v-if="!page.items.length"><td colspan="12" class="empty-state">{{ copy.empty }}</td></tr>
        </tbody></table></div>
        <div class="wallet-pagination" data-testid="platform-archive-task-pages"><button class="secondary" :disabled="listLoading || offset === 0" @click="loadList(Math.max(0, offset - pageSize))">{{ copy.previous }}</button><span>{{ copy.page }} {{ Math.floor(offset / pageSize) + 1 }}</span><button class="secondary" :disabled="listLoading || !hasNextPage" @click="loadList(offset + pageSize)">{{ copy.next }}</button></div>
      </section>

      <section class="panel reward-detail" data-testid="platform-archive-task-detail">
        <div class="panel-heading"><h2>{{ copy.detail }}</h2></div>
        <p v-if="detailLoading" class="loading-line" role="status">{{ copy.loading }}</p>
        <p v-if="detailError" class="message error" role="alert">{{ copy.readDetailFailure }}{{ detailError }}</p>
        <template v-if="detail">
          <dl class="confirm-list reward-fields">
            <div><dt>{{ copy.id }}</dt><dd>{{ detail.id }}</dd></div><div><dt>{{ copy.taskVersion }}</dt><dd>{{ detail.version }}</dd></div><div><dt>{{ copy.policyVersion }}</dt><dd>{{ detail.policy_version }}</dd></div>
            <div><dt>{{ copy.state }}</dt><dd>{{ stateLabel(detail.state) }}</dd></div><div><dt>{{ copy.attempts }}</dt><dd>{{ detail.attempt_count }}</dd></div><div><dt>{{ copy.archiveId }}</dt><dd>{{ detail.archive_id ?? '—' }}</dd></div><div><dt>{{ copy.lastError }}</dt><dd>{{ detail.last_error_code ?? '—' }}</dd></div>
            <div><dt>{{ copy.window }}</dt><dd>{{ detail.window.kind === 'daily' ? copy.dailyKind : copy.monthlyKind }} · {{ detail.window.period_key }}</dd></div><div><dt>{{ copy.timezone }}</dt><dd>{{ detail.window.timezone }}</dd></div><div><dt>{{ copy.from }}</dt><dd>{{ detail.window.from }}</dd></div><div><dt>{{ copy.to }}</dt><dd>{{ detail.window.to }}</dd></div>
            <div><dt>{{ copy.created }}</dt><dd>{{ detail.created_at }}</dd></div><div><dt>{{ copy.updatedTask }}</dt><dd>{{ detail.updated_at }}</dd></div><div><dt>{{ copy.creationAudit }}</dt><dd>{{ detail.creation_audit_log_id }}</dd></div><div><dt>{{ copy.latestAudit }}</dt><dd>{{ detail.last_audit_log_id }}</dd></div>
          </dl>
          <p v-if="detail.state === 'skipped'" class="loading-line">{{ copy.skippedNote }}</p>
        </template>
        <p v-else-if="!detailLoading && !detailError" class="loading-line">{{ copy.select }}</p>
      </section>
    </template>
  </div>
</template>
