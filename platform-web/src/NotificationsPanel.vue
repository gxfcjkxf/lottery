<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { createPlatformNotificationsApi, type Delivery, type Template, type TemplateRevision } from './notifications-api'

const props = defineProps<{ brandId: string; locale: 'en' | 'zh-CN' }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformNotificationsApi()
const tab = ref<'deliveries' | 'templates'>('deliveries')
const deliveries = ref<Delivery[]>([])
const templates = ref<Template[]>([])
const selected = ref<Template | null>(null)
const history = ref<TemplateRevision[]>([])
const deliveryOffset = ref(0)
const historyOffset = ref(0)
const deliveryMore = ref(false)
const historyMore = ref(false)
const loading = ref(false)
const error = ref('')
let generation = 0

const copy = computed(() => props.locale === 'en' ? {
  deliveries: 'Deliveries', templates: 'Templates', refresh: 'Refresh', previous: 'Previous', next: 'Next', page: 'Page',
  emptyDeliveries: 'No notification deliveries.', emptyTemplates: 'No notification templates.', emptyHistory: 'No template history.',
  status: 'Status', attempts: 'Attempts', lastError: 'Last error', nextAttempt: 'Next attempt', sent: 'Sent at', event: 'Event UUID',
  version: 'Version', updated: 'Updated at', audit: 'Audit UUID', current: 'Current content', historyTitle: 'Revision history',
  actor: 'Changed by', reason: 'Reason', created: 'Created at', english: 'English', chinese: 'Chinese (Simplified)', title: 'Title', body: 'Body',
  loading: 'Loading…',
} : {
  deliveries: '发送记录', templates: '模板', refresh: '刷新', previous: '上一页', next: '下一页', page: '页',
  emptyDeliveries: '暂无通知发送记录。', emptyTemplates: '暂无通知模板。', emptyHistory: '暂无模板历史版本。',
  status: '状态', attempts: '尝试次数', lastError: '最近错误', nextAttempt: '下次尝试', sent: '发送时间', event: '事件 UUID',
  version: '版本', updated: '更新时间', audit: '审计 UUID', current: '当前内容', historyTitle: '修订历史',
  actor: '修改人', reason: '原因', created: '创建时间', english: '英文', chinese: '简体中文', title: '标题', body: '正文',
  loading: '加载中…',
})

const messageOf = (cause: unknown) => cause instanceof Error ? cause.message : String(cause)
function statusLabel(status: Delivery['status']) {
  const labels = props.locale === 'en'
    ? { pending: 'Pending', retry: 'Retry scheduled', sent: 'Sent', failed: 'Failed' }
    : { pending: '待发送', retry: '待重试', sent: '已发送', failed: '发送失败' }
  return labels[status]
}
function clearData() {
  deliveries.value = []; templates.value = []; selected.value = null; history.value = []
  deliveryOffset.value = 0; historyOffset.value = 0; deliveryMore.value = false; historyMore.value = false
}

async function loadDeliveries(offset = 0) {
  const request = ++generation
  const brandId = props.brandId
  deliveries.value = []; deliveryMore.value = false; error.value = ''
  if (!brandId) return
  loading.value = true
  try {
    const rows = await api.deliveries(brandId, 51, offset)
    if (request !== generation || brandId !== props.brandId) return
    deliveries.value = rows.slice(0, 50); deliveryMore.value = rows.length > 50; deliveryOffset.value = offset
  } catch (cause) {
    if (request === generation) { deliveries.value = []; error.value = messageOf(cause); emit('failure', cause) }
  } finally { if (request === generation) loading.value = false }
}

async function loadTemplates() {
  const request = ++generation
  const brandId = props.brandId
  templates.value = []; selected.value = null; history.value = []; error.value = ''
  if (!brandId) return
  loading.value = true
  try {
    const rows = await api.templates(brandId)
    if (request !== generation || brandId !== props.brandId) return
    templates.value = rows
  } catch (cause) {
    if (request === generation) { templates.value = []; error.value = messageOf(cause); emit('failure', cause) }
  } finally { if (request === generation) loading.value = false }
}

async function selectTemplate(template: Template, offset = 0) {
  const request = ++generation
  const brandId = props.brandId
  selected.value = template; history.value = []; historyMore.value = false; historyOffset.value = 0; error.value = ''
  if (!brandId) return
  loading.value = true
  try {
    const rows = await api.history(brandId, template.key, 51, offset)
    if (request !== generation || brandId !== props.brandId || selected.value?.key !== template.key) return
    history.value = rows.slice(0, 50); historyMore.value = rows.length > 50; historyOffset.value = offset
  } catch (cause) {
    if (request === generation) { history.value = []; error.value = messageOf(cause); emit('failure', cause) }
  } finally { if (request === generation) loading.value = false }
}

function switchTab(next: 'deliveries' | 'templates') {
  if (tab.value === next) return
  tab.value = next; ++generation; clearData(); error.value = ''; loading.value = false
  if (next === 'deliveries') void loadDeliveries()
  else void loadTemplates()
}

watch(() => props.brandId, () => {
  ++generation; clearData(); error.value = ''; loading.value = false
  if (tab.value === 'deliveries') void loadDeliveries()
  else void loadTemplates()
}, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { ++generation })
</script>

<template>
  <div data-testid="platform-notifications">
    <div class="wallet-pagination" data-testid="platform-notification-tabs">
      <button :class="tab === 'deliveries' ? 'primary' : 'secondary'" @click="switchTab('deliveries')">{{ locale === 'en' ? 'Deliveries' : '发送记录' }}</button>
      <button :class="tab === 'templates' ? 'primary' : 'secondary'" @click="switchTab('templates')">{{ locale === 'en' ? 'Templates' : '模板' }}</button>
    </div>
    <div class="page-heading"><div><h1>{{ tab === 'deliveries' ? copy.deliveries : copy.templates }}</h1></div><button class="secondary" :disabled="loading || !brandId" @click="tab === 'deliveries' ? loadDeliveries(deliveryOffset) : loadTemplates()">{{ copy.refresh }}</button></div>
    <p v-if="error" class="message error" role="alert">{{ error }}</p>
    <p v-if="loading" class="loading-line">{{ copy.loading }}</p>

    <template v-if="tab === 'deliveries'">
      <section class="panel reward-detail" data-testid="platform-delivery-list">
        <div class="table-wrap"><table><thead><tr><th>{{ copy.event }}</th><th>{{ copy.status }}</th><th>{{ copy.attempts }}</th><th>{{ copy.lastError }}</th><th>{{ copy.nextAttempt }}</th><th>{{ copy.sent }}</th></tr></thead><tbody>
          <tr v-for="delivery in deliveries" :key="delivery.event_id"><td>{{ delivery.event_id }}</td><td>{{ statusLabel(delivery.status) }}</td><td>{{ delivery.attempt_count }}</td><td>{{ delivery.last_error ?? '—' }}</td><td>{{ delivery.next_attempt_at }}</td><td>{{ delivery.sent_at ?? '—' }}</td></tr>
          <tr v-if="!deliveries.length"><td colspan="6" class="empty-state">{{ copy.emptyDeliveries }}</td></tr>
        </tbody></table></div>
        <div class="wallet-pagination" data-testid="platform-delivery-pages"><button class="secondary" :disabled="loading || deliveryOffset === 0 || !brandId" @click="loadDeliveries(deliveryOffset - 50)">{{ copy.previous }}</button><span>{{ copy.page }} {{ Math.floor(deliveryOffset / 50) + 1 }}</span><button class="secondary" :disabled="loading || !deliveryMore" @click="loadDeliveries(deliveryOffset + 50)">{{ copy.next }}</button></div>
      </section>
    </template>

    <template v-else>
      <section class="panel reward-detail" data-testid="platform-template-list">
        <div class="table-wrap"><table><thead><tr><th>{{ copy.templates }}</th><th>{{ copy.version }}</th><th>{{ copy.updated }}</th><th>{{ copy.audit }}</th></tr></thead><tbody>
          <tr v-for="template in templates" :key="template.key"><td><button class="row-action" @click="selectTemplate(template)">{{ template.key }}</button></td><td>{{ template.version }}</td><td>{{ template.updated_at }}</td><td>{{ template.audit_log_id ?? '—' }}</td></tr>
          <tr v-if="!templates.length"><td colspan="4" class="empty-state">{{ copy.emptyTemplates }}</td></tr>
        </tbody></table></div>
      </section>
      <section v-if="selected" class="panel reward-detail" data-testid="platform-template-detail">
        <div class="panel-heading"><h2>{{ selected.key }}</h2><span>{{ copy.version }} {{ selected.version }}</span></div>
        <dl class="confirm-list reward-fields"><div><dt>{{ copy.updated }}</dt><dd>{{ selected.updated_at }}</dd></div><div><dt>{{ copy.audit }}</dt><dd>{{ selected.audit_log_id ?? '—' }}</dd></div></dl>
        <h3>{{ copy.current }}</h3>
        <div class="confirm-list reward-fields">
          <div><dt>{{ copy.english }}</dt><dd><strong>{{ copy.title }}:</strong> {{ selected.content.en.title }}<br><strong>{{ copy.body }}:</strong> {{ selected.content.en.body }}</dd></div>
          <div><dt>{{ copy.chinese }}</dt><dd><strong>{{ copy.title }}:</strong> {{ selected.content['zh-CN'].title }}<br><strong>{{ copy.body }}:</strong> {{ selected.content['zh-CN'].body }}</dd></div>
        </div>
        <h3>{{ copy.historyTitle }}</h3>
        <div data-testid="platform-template-history" class="table-wrap"><table><thead><tr><th>{{ copy.version }}</th><th>{{ copy.created }}</th><th>{{ copy.actor }}</th><th>{{ copy.reason }}</th><th>{{ copy.audit }}</th><th>{{ copy.english }}</th><th>{{ copy.chinese }}</th></tr></thead><tbody>
          <tr v-for="revision in history" :key="revision.id"><td>{{ revision.version }}</td><td>{{ revision.created_at }}</td><td>{{ revision.changed_by ?? '—' }}</td><td>{{ revision.reason }}</td><td>{{ revision.audit_log_id ?? '—' }}</td><td><strong>{{ revision.content.en.title }}</strong><br>{{ revision.content.en.body }}</td><td><strong>{{ revision.content['zh-CN'].title }}</strong><br>{{ revision.content['zh-CN'].body }}</td></tr>
          <tr v-if="!history.length"><td colspan="7" class="empty-state">{{ copy.emptyHistory }}</td></tr>
        </tbody></table></div>
        <div class="wallet-pagination" data-testid="platform-template-history-pages"><button class="secondary" :disabled="loading || historyOffset === 0 || !brandId" @click="selectTemplate(selected, historyOffset - 50)">{{ copy.previous }}</button><span>{{ copy.page }} {{ Math.floor(historyOffset / 50) + 1 }}</span><button class="secondary" :disabled="loading || !historyMore" @click="selectTemplate(selected, historyOffset + 50)">{{ copy.next }}</button></div>
      </section>
    </template>
  </div>
</template>
