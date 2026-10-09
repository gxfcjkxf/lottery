<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { PlatformApiError } from './platform-api'
import { createPlatformRewardsApi, type RewardAction, type RewardOrder } from './rewards-api'

const props = defineProps<{ brandId: string; brandName: string; locale: 'en' | 'zh-CN' }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformRewardsApi()
const copy = computed(() => props.locale === 'en' ? {
  title: 'Manual rewards', description: 'Read-only history of manually issued rewards for the selected brand.', brand: 'Brand', select: 'Select a brand', listTitle: 'Reward orders', detail: 'Reward order detail', refresh: 'Refresh',
  order: 'Order', member: 'Member', points: 'Points', state: 'State', version: 'Version', created: 'Created', reason: 'Reason', history: 'History', operation: 'Operation', actor: 'Actor', ledger: 'Ledger entry', grantLedger: 'Grant ledger entry', reversalLedger: 'Reversal ledger entry', policy: 'Point policy version', empty: 'No reward orders found.', noHistory: 'No history found.', loading: 'Loading…', revoked: 'Revoked', granted: 'Granted', pending: 'Revocation pending', grant: 'Grant', revoke: 'Revoke', retry: 'Retry', readOnly: 'Read-only',
} : {
  title: '人工奖励', description: '查看所选品牌的人工奖励记录（只读）。', brand: '品牌', select: '请选择品牌', listTitle: '奖励订单', detail: '奖励订单详情', refresh: '刷新',
  order: '订单', member: '会员', points: '积分', state: '状态', version: '版本', created: '创建时间', reason: '原因', history: '历史记录', operation: '操作', actor: '操作人', ledger: '账本分录', grantLedger: '发放账本分录', reversalLedger: '撤销账本分录', policy: '积分规则版本', empty: '暂无奖励订单。', noHistory: '暂无历史记录。', loading: '加载中…', revoked: '已撤销', granted: '已发放', pending: '等待撤销', grant: '发放', revoke: '撤销', retry: '重试', readOnly: '只读',
})
const orders = ref<RewardOrder[]>([])
const selected = ref<RewardOrder | null>(null)
const detail = ref<RewardOrder | null>(null)
const history = ref<RewardAction[]>([])
const loading = ref(false)
const error = ref('')
let generation = 0
const stateLabel = (state: RewardOrder['state']) => state === 'revoked' ? copy.value.revoked : state === 'granted' ? copy.value.granted : copy.value.pending
const operationLabel = (operation: RewardAction['operation']) => operation === 'grant' ? copy.value.grant : operation === 'revoke' ? copy.value.revoke : copy.value.retry

function report(cause: unknown) {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  else error.value = cause instanceof Error ? cause.message : 'Something went wrong'
}
async function refresh() {
  const request = ++generation
  const brandId = props.brandId
  error.value = ''
  orders.value = []
  selected.value = null
  detail.value = null
  history.value = []
  if (!brandId) { loading.value = false; return }
  loading.value = true
  try {
    const page = await api.list(brandId)
    if (request === generation && props.brandId === brandId) orders.value = page.items
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
  error.value = ''
  loading.value = true
  try {
    const [current, actions] = await Promise.all([api.read(brandId, order.id), api.actions(brandId, order.id)])
    if (request === generation && props.brandId === brandId) { detail.value = current; history.value = actions.items }
  } catch (cause) {
    if (request === generation && props.brandId === brandId) report(cause)
  } finally {
    if (request === generation) loading.value = false
  }
}
watch(() => props.brandId, () => { void refresh() }, { immediate: true, flush: 'sync' })
</script>

<template>
  <div data-testid="platform-rewards">
    <div class="page-heading"><div><div class="eyebrow">{{ copy.readOnly }}</div><h1>{{ copy.title }}</h1><p>{{ copy.description }}</p></div></div>
    <div class="brand-picker"><label>{{ copy.brand }}<span class="reward-brand">{{ props.brandName || copy.select }}</span></label></div>
    <div v-if="error" class="message error global-message">{{ error }}</div>
    <div v-if="loading" class="loading-line">{{ copy.loading }}</div>
    <section class="panel">
      <div class="panel-heading"><div><h2>{{ copy.listTitle }} <span v-if="props.brandName" class="subtle">/ {{ props.brandName }}</span></h2><p>{{ copy.readOnly }}</p></div><div class="reward-list-actions"><span class="count-chip">{{ orders.length }}</span><button class="secondary" data-testid="platform-reward-refresh" :disabled="loading || !props.brandId" @click="refresh">{{ copy.refresh }}</button></div></div>
      <div class="table-wrap"><table><thead><tr><th>{{ copy.order }}</th><th>{{ copy.member }}</th><th>{{ copy.points }}</th><th>{{ copy.state }}</th><th>{{ copy.version }}</th></tr></thead><tbody>
        <tr v-for="order in orders" :key="order.id"><td><button class="row-action mono reward-order-link" @click="openOrder(order)">{{ order.id }}</button><small>{{ order.created_at }}</small></td><td class="mono">{{ order.member_id }}</td><td>{{ order.points }}</td><td><span :class="['status-pill', order.state]">{{ stateLabel(order.state) }}</span></td><td>v{{ order.version }}</td></tr>
        <tr v-if="!orders.length"><td colspan="5" class="empty-state">{{ copy.empty }}</td></tr>
      </tbody></table></div>
    </section>
    <section v-if="selected" class="panel reward-detail" data-testid="platform-reward-detail">
      <div class="panel-heading"><div><h2>{{ copy.detail }}</h2><p class="mono">{{ selected.id }}</p></div><span v-if="detail" class="count-chip">{{ stateLabel(detail.state) }} · v{{ detail.version }}</span></div>
      <dl v-if="detail" class="confirm-list reward-fields">
        <div><dt>{{ copy.order }}</dt><dd class="mono">{{ detail.id }}</dd></div><div><dt>{{ copy.member }}</dt><dd class="mono">{{ detail.member_id }}</dd></div>
        <div><dt>{{ copy.points }}</dt><dd>{{ detail.points }}</dd></div><div><dt>{{ copy.state }}</dt><dd>{{ stateLabel(detail.state) }} · v{{ detail.version }}</dd></div>
        <div><dt>{{ copy.reason }}</dt><dd>{{ detail.reason }}</dd></div><div><dt>{{ copy.created }}</dt><dd>{{ detail.created_at }}</dd></div>
        <div><dt>{{ copy.grantLedger }}</dt><dd class="mono">{{ detail.grant_ledger_entry_id }}</dd></div>
        <div><dt>{{ copy.reversalLedger }}</dt><dd class="mono">{{ detail.revoke_ledger_entry_id || '—' }}</dd></div>
        <div><dt>{{ copy.policy }}</dt><dd>{{ detail.point_policy_version }}</dd></div>
      </dl>
      <div v-if="detail" class="table-wrap"><table><thead><tr><th>{{ copy.history }}</th><th>{{ copy.state }}</th><th>{{ copy.actor }}</th><th>{{ copy.reason }}</th><th>{{ copy.ledger }}</th><th>{{ copy.created }}</th></tr></thead><tbody>
        <tr v-for="action in history" :key="action.id"><td><strong>{{ operationLabel(action.operation) }}</strong><small>{{ action.id }} · v{{ action.version }}</small></td><td>{{ action.state_before ? `${stateLabel(action.state_before)} → ` : '' }}{{ stateLabel(action.state_after) }}</td><td class="mono">{{ action.actor_id }}</td><td>{{ action.reason }}</td><td class="mono">{{ action.ledger_entry_id || '—' }}</td><td>{{ action.created_at }}</td></tr>
        <tr v-if="!history.length"><td colspan="6" class="empty-state">{{ copy.noHistory }}</td></tr>
      </tbody></table></div>
    </section>
  </div>
</template>
