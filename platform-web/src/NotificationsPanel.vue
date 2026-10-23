<script setup lang="ts">
import { notificationCatalog } from '@lottery/shared'
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
import { createPlatformNotificationsApi, type Delivery, type Template, type TemplateRevision } from './notifications-api'
import { formatPlatformDateTime, type PlatformDisplaySettings } from './platform-format'

const props = defineProps<{ brandId: string; locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings }>()
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
const error = ref<string | import("@lottery/shared").LocalizedMessage>("")
function templateLabel(key: string): string {
  if (!Object.hasOwn(notificationCatalog, key)) return displayPublicError(publicErrorMessage({ code: 'NOTIFICATION_NOT_FOUND' }))
  return notificationCatalog[key as keyof typeof notificationCatalog][props.locale].title
}
let generation = 0

const copy = computed(() => props.locale === 'en' ? {
  deliveries: ui('platform.copy.42e5c206'), templates: ui('platform.copy.c772d98c'), refresh: ui('platform.copy.256fe8b3'), previous: ui('platform.copy.d1c7b375'), next: ui('platform.copy.2d3bed9a'), page: ui('platform.copy.35a49df5'), detail: ui('platform.copy.543ae61a'), close: ui('platform.copy.27b2fd29'),
  emptyDeliveries: ui('platform.copy.207c3ccf'), emptyTemplates: ui('platform.copy.bf688a4b'), emptyHistory: ui('platform.copy.ecdfc27a'),
  status: ui('platform.copy.c7260510'), attempts: ui('platform.copy.9e780c26'), lastError: ui('platform.copy.8e0a51be'), nextAttempt: ui('platform.copy.e0b6b5b5'), sent: ui('platform.copy.8225f3fc'), event: ui('platform.copy.6dfc6406'),
  version: ui('platform.copy.59405491'), updated: ui('platform.copy.4eca7621'), audit: ui('platform.copy.8352f5be'), current: ui('platform.copy.d164b43b'), historyTitle: ui('platform.copy.9896df68'),
  actor: ui('platform.copy.4fae8ec9'), reason: ui('platform.copy.f3f300a4'), created: ui('platform.copy.f40187a7'), english: ui('platform.copy.d2332795'), chinese: ui('platform.copy.01f1aac2'), title: ui('platform.copy.1e908e34'), body: ui('platform.copy.29314db9'),
} : {
  deliveries: ui('platform.copy.42e5c206'), templates: ui('platform.copy.c772d98c'), refresh: ui('platform.copy.256fe8b3'), previous: ui('platform.copy.d1c7b375'), next: ui('platform.copy.2d3bed9a'), page: ui('platform.copy.35a49df5'), detail: ui('platform.copy.543ae61a'), close: ui('platform.copy.27b2fd29'),
  emptyDeliveries: ui('platform.copy.207c3ccf'), emptyTemplates: ui('platform.copy.bf688a4b'), emptyHistory: ui('platform.copy.ecdfc27a'),
  status: ui('platform.copy.c7260510'), attempts: ui('platform.copy.9e780c26'), lastError: ui('platform.copy.8e0a51be'), nextAttempt: ui('platform.copy.e0b6b5b5'), sent: ui('platform.copy.8225f3fc'), event: ui('platform.copy.6dfc6406'),
  version: ui('platform.copy.59405491'), updated: ui('platform.copy.4eca7621'), audit: ui('platform.copy.8352f5be'), current: ui('platform.copy.d164b43b'), historyTitle: ui('platform.copy.9896df68'),
  actor: ui('platform.copy.4fae8ec9'), reason: ui('platform.copy.f3f300a4'), created: ui('platform.copy.f40187a7'), english: ui('platform.copy.d2332795'), chinese: ui('platform.copy.01f1aac2'), title: ui('platform.copy.1e908e34'), body: ui('platform.copy.29314db9'),
})

const messageOf = (cause: unknown) => cause instanceof Error ? publicErrorMessage(cause) : publicErrorMessage(cause)
function statusLabel(status: Delivery['status']) {
  const labels = props.locale === 'en'
    ? { pending: ui('platform.copy.8a509db5'), retry: ui('platform.copy.fac2c41b'), sent: ui('platform.copy.a102f16f'), failed: ui('platform.copy.bbe25224') }
    : { pending: ui('platform.copy.8a509db5'), retry: ui('platform.copy.fac2c41b'), sent: ui('platform.copy.a102f16f'), failed: ui('platform.copy.bbe25224') }
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

async function switchTab(next: 'deliveries' | 'templates') {
  if (tab.value === next) return
  tab.value = next; ++generation; clearData(); error.value = ''; loading.value = false
  if (next === 'deliveries') await loadDeliveries()
  else await loadTemplates()
}

watch(() => props.brandId, () => {
  ++generation; clearData(); error.value = ''; loading.value = false
  if (tab.value === 'deliveries') void withGlobalLoading(loadDeliveries)
  else void withGlobalLoading(loadTemplates)
}, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { ++generation })
</script>

<template>
  <div data-testid="platform-notifications">
    <div class="wallet-pagination" data-testid="platform-notification-tabs">
      <button :class="tab === 'deliveries' ? 'button button-primary' : 'button button-secondary'" @click="withGlobalLoading(switchTab, 'deliveries')">{{ copy.deliveries }}</button>
      <button :class="tab === 'templates' ? 'button button-primary' : 'button button-secondary'" @click="withGlobalLoading(switchTab, 'templates')">{{ copy.templates }}</button>
    </div>
    <p v-if="error" class="message error" role="alert">{{ displayPublicError(error) }}</p>

    <template v-if="tab === 'deliveries'">
      <section class="panel reward-detail" data-testid="platform-delivery-list">
        <div class="table-wrap"><table><thead><tr><th>{{ copy.event }}</th><th><BusinessDetails :value="copy.status" :locale="props.locale" /></th><th>{{ copy.attempts }}</th><th>{{ copy.lastError }}</th><th>{{ copy.nextAttempt }}</th><th>{{ copy.sent }}</th></tr></thead><tbody>
          <tr v-for="delivery in deliveries" :key="delivery.event_id"><td>{{ delivery.event_id }}</td><td>{{ statusLabel(delivery.status) }}</td><td>{{ delivery.attempt_count }}</td><td>{{ delivery.last_error ? displayPublicError(publicErrorMessage({ code: delivery.last_error })) : "—" }}</td><td>{{ formatPlatformDateTime(delivery.next_attempt_at, props.displaySettings, props.locale) }}</td><td>{{ formatPlatformDateTime(delivery.sent_at, props.displaySettings, props.locale) }}</td></tr>
          <tr v-if="!deliveries.length"><td colspan="6" class="empty-state">{{ copy.emptyDeliveries }}</td></tr>
        </tbody></table></div>
        <div class="wallet-pagination" data-testid="platform-delivery-pages"><button class="button button-secondary" :disabled="loading || deliveryOffset === 0 || !brandId" @click="withGlobalLoading(loadDeliveries, deliveryOffset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(deliveryOffset / 50) + 1 }}</span><button class="button button-secondary" :disabled="loading || !deliveryMore" @click="withGlobalLoading(loadDeliveries, deliveryOffset + 50)">{{ copy.next }}</button><button v-refresh-button="copy.refresh" class="button button-secondary" :disabled="loading || !brandId" @click="withGlobalLoading(loadDeliveries, deliveryOffset)">{{ copy.refresh }}</button></div>
      </section>
    </template>

    <template v-else>
      <section class="panel reward-detail" data-testid="platform-template-list">
        <div class="table-wrap"><table><thead><tr><th>{{ copy.templates }}</th><th>{{ copy.updated }}</th></tr></thead><tbody>
          <tr v-for="template in templates" :key="template.key"><td><button class="row-action" @click="withGlobalLoading(selectTemplate, template)">{{ templateLabel(template.key) }}</button></td><td>{{ formatPlatformDateTime(template.updated_at, props.displaySettings, props.locale) }}</td></tr>
          <tr v-if="!templates.length"><td colspan="2" class="empty-state">{{ copy.emptyTemplates }}</td></tr>
        </tbody></table></div>
      </section>
      <DetailModal v-if="selected" :title="`${copy.detail} · ${templateLabel(selected.key)}`" :close-label="copy.close" @close="selected = null">
<section v-if="selected" class="detail-section" data-testid="platform-template-detail">

        <dl class="confirm-list reward-fields"><div><dt>{{ copy.updated }}</dt><dd>{{ formatPlatformDateTime(selected.updated_at, props.displaySettings, props.locale) }}</dd></div></dl>
        <h3>{{ copy.current }}</h3>
        <div class="confirm-list reward-fields">
          <div><dt>{{ copy.english }}</dt><dd><strong>{{ copy.title }}:</strong> {{ selected.content.en.title }}<br><strong>{{ copy.body }}:</strong> {{ selected.content.en.body }}</dd></div>
          <div><dt>{{ copy.chinese }}</dt><dd><strong>{{ copy.title }}:</strong> {{ selected.content['zh-CN'].title }}<br><strong>{{ copy.body }}:</strong> {{ selected.content['zh-CN'].body }}</dd></div>
        </div>
        <h3>{{ copy.historyTitle }}</h3>
        <div data-testid="platform-template-history" class="table-wrap"><table><thead><tr><th>{{ copy.created }}</th><th>{{ copy.reason }}</th><th>{{ copy.english }}</th><th>{{ copy.chinese }}</th></tr></thead><tbody>
          <tr v-for="revision in history" :key="revision.id"><td>{{ formatPlatformDateTime(revision.created_at, props.displaySettings, props.locale) }}</td><td>{{ revision.reason }}</td><td><strong>{{ revision.content.en.title }}</strong><br>{{ revision.content.en.body }}</td><td><strong>{{ revision.content['zh-CN'].title }}</strong><br>{{ revision.content['zh-CN'].body }}</td></tr>
          <tr v-if="!history.length"><td colspan="4" class="empty-state">{{ copy.emptyHistory }}</td></tr>
        </tbody></table></div>
        <div class="wallet-pagination" data-testid="platform-template-history-pages"><button class="button button-secondary" :disabled="loading || historyOffset === 0 || !brandId" @click="withGlobalLoading(selectTemplate, selected, historyOffset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(historyOffset / 50) + 1 }}</span><button class="button button-secondary" :disabled="loading || !historyMore" @click="withGlobalLoading(selectTemplate, selected, historyOffset + 50)">{{ copy.next }}</button><button v-refresh-button="copy.refresh" class="button button-secondary" :disabled="loading || !selected" @click="withGlobalLoading(async () => { selected && await selectTemplate(selected) })">{{ copy.refresh }}</button></div>
      </section>
</DetailModal>
    </template>
  </div>
</template>
