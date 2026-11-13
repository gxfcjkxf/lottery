<script setup lang="ts">
import ManagementList from "@lottery/shared/ManagementList.vue"
import { readPaginationTotal } from '@lottery/shared'
const deliveryTotal = ref('0')

import TablePagination from "@lottery/shared/TablePagination.vue"
import { useOperationToast } from "@lottery/shared";
import NotificationTemplatesPanel from './NotificationTemplatesPanel.vue'
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { usePlatformI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { usePlatformI18n } from "./i18n"
const ui = usePlatformI18n().t
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { createPlatformNotificationsApi, type Delivery } from './notifications-api'
import { formatPlatformDateTime, type PlatformDisplaySettings } from './platform-format'
const pageSize = ref(50)


const props = defineProps<{ view: 'deliveries' | 'templates'; brandId: string; accountId: string; canManage: boolean; canViewTemplates: boolean; locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformNotificationsApi()
const deliveries = ref<Delivery[]>([])
const deliveryOffset = ref(0)
const deliveryMore = ref(false)
const loading = ref(false)
const error = ref<string | import("@lottery/shared").LocalizedMessage>("")
let generation = 0

const copy = computed(() => ({
 deliveries: ui('platform.copy.42e5c206'), templates: ui('platform.copy.c772d98c'), refresh: ui('platform.copy.256fe8b3'), previous: ui('platform.copy.d1c7b375'), next: ui('platform.copy.2d3bed9a'),
 emptyDeliveries: ui('platform.copy.207c3ccf'), status: ui('platform.copy.c7260510'), attempts: ui('platform.copy.9e780c26'), lastError: ui('platform.copy.8e0a51be'), nextAttempt: ui('platform.copy.e0b6b5b5'), sent: ui('platform.copy.8225f3fc'), event: ui('platform.copy.6dfc6406'),
}))

const messageOf = (cause: unknown) => publicErrorMessage(cause)
function statusLabel(status: Delivery['status']) {
  const labels = props.locale === 'en'
    ? { pending: ui('platform.copy.8a509db5'), retry: ui('platform.copy.fac2c41b'), sent: ui('platform.copy.a102f16f'), failed: ui('platform.copy.bbe25224') }
    : { pending: ui('platform.copy.8a509db5'), retry: ui('platform.copy.fac2c41b'), sent: ui('platform.copy.a102f16f'), failed: ui('platform.copy.bbe25224') }
  return labels[status]
}
function clearData() {
  [deliveries.value,deliveryTotal.value]=[[],'0']
  deliveryOffset.value = 0; deliveryMore.value = false
}

async function loadDeliveries(offset = 0) {
  const request = ++generation
  const brandId = props.brandId
  error.value = ''
  if (!brandId) return
  loading.value = true
  try {
    const rows = await api.deliveries(brandId, pageSize.value, offset)
    if (request !== generation || brandId !== props.brandId) return
    [deliveries.value, deliveryTotal.value] = [rows.items, readPaginationTotal(rows)]; deliveryMore.value = BigInt(offset + rows.items.length) < BigInt(deliveryTotal.value); deliveryOffset.value = offset
  } catch (cause) {
    if (request === generation) { error.value = messageOf(cause); emit('failure', cause) }
  } finally { if (request === generation) loading.value = false }
}

watch(() => [props.brandId, props.view, props.accountId], () => {
  ++generation; clearData(); error.value = ''; loading.value = false
  if (props.view === 'deliveries') void withGlobalLoading(loadDeliveries)

}, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { ++generation })

useOperationToast(error, value => displayPublicError(value), 'error');
</script>

<template>
  <div data-testid="platform-notifications">
    <template v-if="view === 'deliveries'">
      <ManagementList class="reward-detail" data-testid="platform-delivery-list">
      <template v-if="error" #actions><button v-refresh-button="copy.refresh" class="button button-secondary" :disabled="loading || !brandId" @click="withGlobalLoading(loadDeliveries, deliveryOffset)">{{ copy.refresh }}</button></template>
        <div class="table-wrap"><table class="console-table console-responsive-table"><thead><tr><th>{{ copy.event }}</th><th><BusinessDetails :value="copy.status" :locale="props.locale" /></th><th>{{ copy.attempts }}</th><th>{{ copy.lastError }}</th><th>{{ copy.nextAttempt }}</th><th>{{ copy.sent }}</th></tr></thead><tbody>
          <tr v-for="delivery in deliveries" :key="delivery.event_id"><td :data-label="String(copy.event)">{{ delivery.event_id }}</td><td :data-label="copy.status">{{ statusLabel(delivery.status) }}</td><td :data-label="String(copy.attempts)">{{ delivery.attempt_count }}</td><td :data-label="String(copy.lastError)">{{ delivery.last_error ? displayPublicError(publicErrorMessage({ code: delivery.last_error })) : "—" }}</td><td :data-label="String(copy.nextAttempt)">{{ formatPlatformDateTime(delivery.next_attempt_at, props.displaySettings, props.locale) }}</td><td :data-label="String(copy.sent)">{{ formatPlatformDateTime(delivery.sent_at, props.displaySettings, props.locale) }}</td></tr>
          <tr v-if="!loading && !error && !deliveries.length"><td colspan="6" class="empty-state">{{ copy.emptyDeliveries }}</td></tr>
        </tbody></table></div>

      <template #pagination><TablePagination class="wallet-pagination" data-testid="platform-delivery-pages" :total="deliveryTotal" :page="Math.floor(deliveryOffset / pageSize + 1)" :count="deliveries.length" :locale="locale" :page-size="pageSize" :previous-disabled="loading || deliveryOffset === 0 || !brandId" :next-disabled="loading || !deliveryMore" :previous-label="copy.previous" :next-label="copy.next" :go-to="(target: number) => loadDeliveries((target - 1) * pageSize)" :change-page-size="async (size: number) => { pageSize = size; await loadDeliveries(0) }" :busy="loading" /></template></ManagementList>
    </template>

    <NotificationTemplatesPanel v-else :key="accountId" :account-id="accountId" :can-manage="canManage" :can-view="canViewTemplates" :locale="locale" :display-settings="displaySettings" @failure="emit('failure', $event)" />
  </div>
</template>
