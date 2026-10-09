<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { WithdrawalHistory, WithdrawalOrder, WithdrawalState } from '@lottery/shared'
import { PlatformApiError } from './platform-api'
import { createPlatformWithdrawalApi } from './withdrawal-api'

const props = defineProps<{ brandId: string; locale: 'en' | 'zh-CN' }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformWithdrawalApi()
const states: WithdrawalState[] = ['reviewing', 'processing', 'paid', 'rejected', 'failed', 'cancelled']
const memberDraft = ref('')
const stateDraft = ref<WithdrawalState | ''>('')
const activeMember = ref('')
const activeState = ref<WithdrawalState | ''>('')
const rows = ref<WithdrawalOrder[]>([])
const detail = ref<WithdrawalOrder | null>(null)
const history = ref<WithdrawalHistory | null>(null)
const offset = ref(0)
const more = ref(false)
const loading = ref(false)
const error = ref('')
let generation = 0
const labels = computed(() => props.locale === 'en' ? {
  reviewing: 'Reviewing', processing: 'Processing', paid: 'Paid', rejected: 'Rejected', failed: 'Failed', cancelled: 'Cancelled',
} : { reviewing: '审核中', processing: '提现中', paid: '已提现', rejected: '已驳回', failed: '失败', cancelled: '已取消' })
const copy = computed(() => props.locale === 'en' ? {
  title: 'Withdrawal orders', note: 'Read-only withdrawals · No approval or payout controls', member: 'Member ID (optional)', state: 'State', all: 'All states', query: 'Query withdrawals', order: 'Order', points: 'Points', version: 'Version', created: 'Created', detail: 'Withdrawal detail', sources: 'Reserved point sources', source: 'Source', reason: 'Decision reason', reserve: 'Reserve ledger entry', release: 'Release ledger entry', paid: 'Paid ledger entry', cycle: 'Previous successful withdrawal cutoff', cursor: 'Previous cutoff version', reservedVersion: 'Reserved wallet version', reviewed: 'Reviewed', completed: 'Completed', history: 'State history', from: 'From', to: 'To', actor: 'Actor type', previous: 'Previous page', next: 'Next page', empty: 'No withdrawal orders.', loading: 'Loading…', recharge: 'Recharge', winning: 'Winning', gift: 'Gift', commission: 'Commission',
} : {
  title: '提现订单', note: '只读提现记录，不提供审核或出款操作', member: '会员编号（可选）', state: '状态', all: '全部状态', query: '查询提现', order: '订单', points: '积分', version: '版本', created: '创建时间', detail: '提现详情', sources: '占用积分来源', source: '来源', reason: '处理理由', reserve: '占用账本分录', release: '退还账本分录', paid: '出款账本分录', cycle: '上次成功提现截止时间', cursor: '上次截止版本', reservedVersion: '占用时钱包版本', reviewed: '审核时间', completed: '完成时间', history: '状态历史', from: '之前', to: '之后', actor: '操作人类型', previous: '上一页', next: '下一页', empty: '暂无提现订单。', loading: '加载中…', recharge: '充值', winning: '中奖', gift: '赠送', commission: '佣金',
})
function report(cause: unknown) {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  else error.value = cause instanceof Error ? cause.message : String(cause)
}
async function load(nextOffset = 0, member = activeMember.value, state = activeState.value) {
  const request = ++generation
  const brandId = props.brandId
  rows.value = []; detail.value = null; history.value = null; more.value = false; error.value = ''
  if (!brandId) { loading.value = false; return }
  loading.value = true
  try {
    const page = await api.list(brandId, { limit: 50, offset: nextOffset, ...(member ? { memberId: member } : {}), ...(state ? { state } : {}) })
    if (request !== generation || props.brandId !== brandId) return
    rows.value = page.items; offset.value = page.offset; more.value = page.has_more
    activeMember.value = member; activeState.value = state
  } catch (cause) { if (request === generation) report(cause) }
  finally { if (request === generation) loading.value = false }
}
async function open(order: WithdrawalOrder) {
  const request = ++generation
  const brandId = props.brandId
  detail.value = null; history.value = null; error.value = ''; loading.value = true
  try {
    const [current, actions] = await Promise.all([api.read(brandId, order.id), api.history(brandId, order.id)])
    if (request === generation && props.brandId === brandId) { detail.value = current; history.value = actions }
  } catch (cause) { if (request === generation) report(cause) }
  finally { if (request === generation) loading.value = false }
}
watch(() => props.brandId, () => {
  memberDraft.value = ''; stateDraft.value = ''; activeMember.value = ''; activeState.value = ''; offset.value = 0
  void load()
}, { immediate: true, flush: 'sync' })
</script>

<template>
  <div data-testid="platform-withdrawals">
    <div class="page-heading"><div><div class="eyebrow">{{ copy.note }}</div><h1>{{ copy.title }}</h1></div></div>
    <form class="panel wallet-query" @submit.prevent="load(0, memberDraft.trim(), stateDraft)">
      <label>{{ copy.member }}<input v-model="memberDraft" :disabled="loading || !brandId" autocomplete="off" /></label>
      <label><span>{{ copy.state }}</span><select v-model="stateDraft" :aria-label="copy.state" :disabled="loading || !brandId"><option value="">{{ copy.all }}</option><option v-for="state in states" :key="state" :value="state">{{ labels[state] }}</option></select></label>
      <button class="secondary" :disabled="loading || !brandId">{{ copy.query }}</button>
    </form>
    <p v-if="error" class="message error" role="alert">{{ error }}</p><p v-if="loading" class="loading-line">{{ copy.loading }}</p>
    <section class="panel reward-detail"><div class="table-wrap"><table><thead><tr><th>{{ copy.order }}</th><th>{{ copy.points }}</th><th>{{ copy.state }}</th><th>{{ copy.created }}</th></tr></thead><tbody>
      <tr v-for="order in rows" :key="order.id"><td><button class="row-action" @click="open(order)">{{ order.id }}</button><small>{{ order.member_id }}</small></td><td>{{ order.points }}</td><td>{{ labels[order.state] }}</td><td>{{ order.created_at }}</td></tr>
      <tr v-if="!rows.length"><td colspan="4" class="empty-state">{{ copy.empty }}</td></tr>
    </tbody></table></div><div class="wallet-pagination" data-testid="platform-withdrawal-pages"><button class="secondary" :disabled="loading || offset === 0 || !brandId" @click="load(offset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(offset / 50) + 1 }}</span><button class="secondary" :disabled="loading || !more" @click="load(offset + 50)">{{ copy.next }}</button></div></section>
    <section v-if="detail" class="panel reward-detail" data-testid="platform-withdrawal-detail">
      <div class="panel-heading"><h2>{{ copy.detail }}</h2><span>{{ labels[detail.state] }} · v{{ detail.version }}</span></div>
      <dl class="confirm-list reward-fields">
        <div><dt>{{ copy.order }}</dt><dd>{{ detail.id }}</dd></div><div><dt>{{ copy.points }}</dt><dd>{{ detail.points }}</dd></div><div><dt>{{ copy.reason }}</dt><dd>{{ detail.decision_reason }}</dd></div>
        <div><dt>{{ copy.reserve }}</dt><dd>{{ detail.reserve_entry_id }}</dd></div><div><dt>{{ copy.release }}</dt><dd>{{ detail.release_entry_id || '—' }}</dd></div><div><dt>{{ copy.paid }}</dt><dd>{{ detail.paid_entry_id || '—' }}</dd></div>
        <div><dt>{{ copy.cycle }}</dt><dd>{{ detail.cycle_from_at || '—' }}</dd></div><div><dt>{{ copy.cursor }}</dt><dd>{{ detail.cycle_from_version }}</dd></div><div><dt>{{ copy.reservedVersion }}</dt><dd>{{ detail.reserve_version }}</dd></div><div><dt>{{ copy.reviewed }}</dt><dd>{{ detail.reviewed_at || '—' }}</dd></div><div><dt>{{ copy.completed }}</dt><dd>{{ detail.completed_at || '—' }}</dd></div>
      </dl>
      <div class="table-wrap"><table><thead><tr><th>{{ copy.sources }}</th><th>{{ copy.points }}</th></tr></thead><tbody><tr v-for="allocation in detail.source_allocation" :key="allocation.source"><td>{{ copy[allocation.source] }}</td><td>{{ allocation.points }}</td></tr></tbody></table></div>
      <div v-if="history" class="table-wrap" data-testid="platform-withdrawal-history"><table><thead><tr><th>{{ copy.version }}</th><th>{{ copy.from }}</th><th>{{ copy.to }}</th><th>{{ copy.actor }}</th><th>{{ copy.reason }}</th><th>{{ copy.created }}</th></tr></thead><tbody><tr v-for="item in history.items" :key="item.id"><td>{{ item.version }}</td><td>{{ item.from_state ? labels[item.from_state] : '—' }}</td><td>{{ labels[item.to_state] }}</td><td>{{ item.actor_type }}</td><td>{{ item.reason }}</td><td>{{ item.created_at }}</td></tr></tbody></table></div>
    </section>
  </div>
</template>
