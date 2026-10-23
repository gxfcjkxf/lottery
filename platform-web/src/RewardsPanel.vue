<script setup lang="ts">
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { usePlatformI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from "@lottery/shared";
import { usePlatformI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { usePlatformI18n } from "./i18n"
const ui = usePlatformI18n().t
import DetailModal from "@lottery/shared/DetailModal.vue"
import { computed, ref, watch } from 'vue'
import { PlatformApiError } from './platform-api'
import { createPlatformRewardsApi, type RewardAction, type RewardOrder } from './rewards-api'
import { formatPlatformDateTime, type PlatformDisplaySettings } from './platform-format'

const props = defineProps<{ brandId: string; brandName: string; locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformRewardsApi()
const copy = computed(() => props.locale === 'en' ? {
  title: ui('platform.copy.25982fc9'), brand: ui('platform.copy.d5b68c0f'), select: ui('platform.copy.1c0a1845'), detail: ui('platform.copy.3e9a2354'), close: ui('platform.copy.27b2fd29'), refresh: ui('platform.copy.256fe8b3'),
  order: ui('platform.copy.68419640'), member: ui('platform.copy.da8480f3'), points: ui('platform.copy.a1f74153'), state: ui('platform.copy.274ae047'), version: ui('platform.copy.59405491'), created: ui('platform.copy.00e7cdbe'), reason: ui('platform.copy.f3f300a4'), history: ui('platform.copy.c78082da'), operation: ui('platform.copy.412ae2c3'), actor: ui('platform.copy.3177f27f'), ledger: ui('platform.copy.bed1cc9b'), grantLedger: ui('platform.copy.ff2a40e6'), reversalLedger: ui('platform.copy.80166e37'), policy: ui('platform.copy.94038b59'), empty: ui('platform.copy.c5dc63d8'), noHistory: ui('platform.copy.a0610689'), revoked: ui('platform.copy.97fc3c9b'), granted: ui('platform.copy.21e83e2d'), pending: ui('platform.copy.a701ca64'), grant: ui('platform.copy.44defe04'), revoke: ui('platform.copy.3804e055'), retry: ui('platform.copy.5ef43f95'),
} : {
  title: ui('platform.copy.25982fc9'), brand: ui('platform.copy.d5b68c0f'), select: ui('platform.copy.1c0a1845'), detail: ui('platform.copy.3e9a2354'), close: ui('platform.copy.27b2fd29'), refresh: ui('platform.copy.256fe8b3'),
  order: ui('platform.copy.68419640'), member: ui('platform.copy.da8480f3'), points: ui('platform.copy.a1f74153'), state: ui('platform.copy.274ae047'), version: ui('platform.copy.59405491'), created: ui('platform.copy.00e7cdbe'), reason: ui('platform.copy.f3f300a4'), history: ui('platform.copy.c78082da'), operation: ui('platform.copy.412ae2c3'), actor: ui('platform.copy.3177f27f'), ledger: ui('platform.copy.bed1cc9b'), grantLedger: ui('platform.copy.ff2a40e6'), reversalLedger: ui('platform.copy.80166e37'), policy: ui('platform.copy.94038b59'), empty: ui('platform.copy.c5dc63d8'), noHistory: ui('platform.copy.a0610689'), revoked: ui('platform.copy.97fc3c9b'), granted: ui('platform.copy.21e83e2d'), pending: ui('platform.copy.a701ca64'), grant: ui('platform.copy.44defe04'), revoke: ui('platform.copy.3804e055'), retry: ui('platform.copy.5ef43f95'),
})
const orders = ref<RewardOrder[]>([])
const selected = ref<RewardOrder | null>(null)
const detail = ref<RewardOrder | null>(null)
const history = ref<RewardAction[]>([])
const pageSize = 100
const orderOffset = ref(0)
const orderTotal = ref('0')
const historyOffset = ref(0)
const historyTotal = ref('0')
const previousLabel = computed(() => ui('platform.pagination.previous'))
const nextLabel = computed(() => ui('platform.pagination.next'))
const totalLabel = computed(() => ui('platform.pagination.total'))
const hasNextOrders = computed(() => BigInt(orderOffset.value + pageSize) < BigInt(orderTotal.value))
const hasNextHistory = computed(() => BigInt(historyOffset.value + pageSize) < BigInt(historyTotal.value))
const loading = ref(false)
const error = ref<string | import("@lottery/shared").LocalizedMessage>("")
let generation = 0
const stateLabel = (state: RewardOrder['state']) => state === 'revoked' ? copy.value.revoked : state === 'granted' ? copy.value.granted : copy.value.pending
const operationLabel = (operation: RewardAction['operation']) => operation === 'grant' ? copy.value.grant : operation === 'revoke' ? copy.value.revoke : copy.value.retry

function report(cause: unknown) {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  else error.value = cause instanceof Error ? publicErrorMessage(cause) : ui('platform.error.unexpected')
}
async function refresh(offset = 0) {
  const request = ++generation
  const brandId = props.brandId
  error.value = ''
  orders.value = []
  selected.value = null
  detail.value = null
  history.value = []
  orderOffset.value = 0
  orderTotal.value = '0'
  historyTotal.value = '0'
  historyOffset.value = 0
  if (!brandId) { loading.value = false; return }
  loading.value = true
  try {
    const page = await api.list(brandId, pageSize, offset)
    if (request === generation && props.brandId === brandId) {
      orders.value = page.items
      orderOffset.value = page.offset
      orderTotal.value = page.total_count
    }
  } catch (cause) {
    if (request === generation && props.brandId === brandId) report(cause)
  } finally {
    if (request === generation) loading.value = false
  }
}
async function openOrder(order: RewardOrder) {
  const request = ++generation
  const brandId = props.brandId
  selected.value = order
  detail.value = null
  history.value = []
  historyOffset.value = 0
  historyTotal.value = '0'
  error.value = ''
  loading.value = true
  try {
    const [current, actions] = await Promise.all([api.read(brandId, order.id), api.actions(brandId, order.id)])
    if (request === generation && props.brandId === brandId) {
      detail.value = current
      history.value = actions.items
      historyTotal.value = actions.total_count
    }
  } catch (cause) {
    if (request === generation && props.brandId === brandId) report(cause)
  } finally {
    if (request === generation) loading.value = false
  }
}
async function loadHistory(offset: number) {
  if (!detail.value) return
  const request = ++generation
  const brandId = props.brandId
  const orderId = detail.value.id
  error.value = ''
  history.value = []
  historyTotal.value = '0'
  loading.value = true
  try {
    const page = await api.actions(brandId, orderId, pageSize, offset)
    if (request === generation && props.brandId === brandId) {
      history.value = page.items
      historyOffset.value = page.offset
      historyTotal.value = page.total_count
    }
  } catch (cause) {
    if (request === generation && props.brandId === brandId) report(cause)
  } finally {
    if (request === generation) loading.value = false
  }
}
watch(() => props.brandId, () => { void withGlobalLoading(refresh) }, { immediate: true, flush: 'sync' })
</script>

<template>
  <div data-testid="platform-rewards">
    <div v-if="error" class="message error global-message">{{ displayPublicError(error) }}</div>
    <section class="panel">
      <div class="table-wrap"><table><thead><tr><th>{{ copy.order }}</th><th>{{ copy.member }}</th><th>{{ copy.points }}</th><th><BusinessDetails :value="copy.state" :locale="associationLocale" /></th></tr></thead><tbody>
        <tr v-for="order in orders" :key="order.id"><td><button class="row-action mono reward-order-link" @click="withGlobalLoading(openOrder, order)">{{ order.id }}</button><small>{{ formatPlatformDateTime(order.created_at, props.displaySettings, props.locale) }}</small></td><td class="mono"><AssociationName :record="order" kind="member" :id="order.member_id" :locale="associationLocale" /></td><td>{{ order.points }}</td><td><span :class="['status-pill', order.state]">{{ stateLabel(order.state) }}</span></td></tr>
        <tr v-if="!orders.length"><td colspan="4" class="empty-state">{{ copy.empty }}</td></tr>
      </tbody></table></div>
      <div class="reward-list-actions" data-testid="platform-reward-pages">
        <span>{{ totalLabel }} {{ orderTotal }}</span>
        <button class="button button-secondary" :disabled="loading || orderOffset === 0" @click="withGlobalLoading(refresh, orderOffset - pageSize)">{{ previousLabel }}</button>
        <span>{{ Math.floor(orderOffset / pageSize) + 1 }}</span>
        <button class="button button-secondary" :disabled="loading || !hasNextOrders" @click="withGlobalLoading(refresh, orderOffset + pageSize)">{{ nextLabel }}</button>
        <button v-refresh-button="copy.refresh" class="button button-secondary" data-testid="platform-reward-refresh" :disabled="loading || !props.brandId" @click="withGlobalLoading(refresh)">{{ copy.refresh }}</button>
      </div>
    </section>
    <DetailModal v-if="selected" :title="copy.detail" :close-label="copy.close" @close="selected = null">
<section v-if="selected" class="detail-section" data-testid="platform-reward-detail">
      <div class="detail-summary"><p v-if="!detail" class="mono">{{ selected.id }}</p><span v-if="detail" class="count-chip">{{ stateLabel(detail.state) }}</span></div>
      <dl v-if="detail" class="confirm-list reward-fields">
        <div><dt>{{ copy.order }}</dt><dd class="mono">{{ detail.id }}</dd></div><div><dt>{{ copy.member }}</dt><dd class="mono"><AssociationName :record="detail" kind="member" :id="detail.member_id" :locale="associationLocale" /></dd></div>
        <div><dt>{{ copy.points }}</dt><dd>{{ detail.points }}</dd></div><div><dt><BusinessDetails :value="copy.state" :locale="associationLocale" /></dt><dd>{{ stateLabel(detail.state) }}</dd></div>
        <div><dt v-if="detail.reason">{{ copy.reason }}</dt><dd v-if="detail.reason">{{ detail.reason }}</dd></div><div><dt>{{ copy.created }}</dt><dd>{{ formatPlatformDateTime(detail.created_at, props.displaySettings, props.locale) }}</dd></div>
        <div><dt>{{ copy.grantLedger }}</dt><dd class="mono">{{ detail.grant_ledger_entry_id }}</dd></div>
        <div><dt>{{ copy.reversalLedger }}</dt><dd class="mono">{{ detail.revoke_ledger_entry_id || '—' }}</dd></div>

      </dl>
      <div v-if="detail" class="table-wrap"><table><thead><tr><th>{{ copy.history }}</th><th><BusinessDetails :value="copy.state" :locale="associationLocale" /></th><th>{{ copy.actor }}</th><th>{{ copy.reason }}</th><th>{{ copy.ledger }}</th><th>{{ copy.created }}</th></tr></thead><tbody>
        <tr v-for="action in history" :key="action.id"><td><strong>{{ operationLabel(action.operation) }}</strong><small>{{ action.id }}</small></td><td>{{ action.state_before ? `${stateLabel(action.state_before)} → ` : '' }}{{ stateLabel(action.state_after) }}</td><td class="mono"><AssociationName :record="action" kind="actor" :id="action.actor_id" :locale="associationLocale" /></td><td>{{ action.reason }}</td><td class="mono">{{ action.ledger_entry_id || '—' }}</td><td>{{ formatPlatformDateTime(action.created_at, props.displaySettings, props.locale) }}</td></tr>
        <tr v-if="!history.length"><td colspan="6" class="empty-state">{{ copy.noHistory }}</td></tr>
      </tbody></table></div>
      <div v-if="detail" class="reward-list-actions" data-testid="platform-reward-history-pages">
        <span>{{ totalLabel }} {{ historyTotal }}</span>
        <button class="button button-secondary" :disabled="loading || historyOffset === 0" @click="withGlobalLoading(loadHistory, historyOffset - pageSize)">{{ previousLabel }}</button>
        <span>{{ Math.floor(historyOffset / pageSize) + 1 }}</span>
        <button class="button button-secondary" :disabled="loading || !hasNextHistory" @click="withGlobalLoading(loadHistory, historyOffset + pageSize)">{{ nextLabel }}</button>
      </div>
    </section>
</DetailModal>
  </div>
</template>
