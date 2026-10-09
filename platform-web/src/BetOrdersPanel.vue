<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { PlatformApiError } from './platform-api'
import { createPlatformBetApi, type BetOrder } from './bet-api'
import LotteryPanel from './LotteryPanel.vue'

const props = defineProps<{ brandId: string; locale: 'en' | 'zh-CN' }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformBetApi()
const catalogue = ref(false)
function chooseTab(value: boolean) {
  catalogue.value = value
  ++generation
  orders.value = []; detail.value = null; more.value = false; error.value = ''; loading.value = false
  if (!value) void load()
}
const draftMember = ref('')
const memberFilter = ref('')
const orders = ref<BetOrder[]>([])
const detail = ref<BetOrder | null>(null)
const offset = ref(0)
const more = ref(false)
const loading = ref(false)
const error = ref('')
let generation = 0
const copy = computed(() => props.locale === 'en' ? {
  title: 'Bet orders', note: 'Read-only orders and original betting snapshots', member: 'Member ID (optional)', query: 'Query orders', order: 'Order', points: 'Bet points', state: 'State', period: 'Period', date: 'Placed at', previous: 'Previous page', next: 'Next page', empty: 'No bet orders.', detail: 'Order detail', game: 'Game', play: 'Play', version: 'Order version', prize: 'Prize points', unit: 'Unit points', combinations: 'Combinations', multiplier: 'Multiplier', rule: 'Rule version', hash: 'Definition hash', snapshots: 'Original snapshots', selection: 'Selection', normalized: 'Normalized selection', expanded: 'Expanded bets', definition: 'Rule definition', policy: 'Bet policy', allocation: 'Deduction allocation', loading: 'Loading…',
} : {
  title: '投注订单', note: '只读订单及投注时的原始快照', member: '会员编号（可选）', query: '查询订单', order: '订单', points: '投注积分', state: '状态', period: '期次编号', date: '投注时间', previous: '上一页', next: '下一页', empty: '暂无投注订单。', detail: '订单详情', game: '彩种', play: '玩法', version: '订单版本', prize: '中奖积分', unit: '单注积分', combinations: '组合数', multiplier: '倍数', rule: '规则版本', hash: '规则摘要', snapshots: '原始快照', selection: '原始选号', normalized: '规范选号', expanded: '展开注项', definition: '规则定义', policy: '投注规则', allocation: '扣款来源', loading: '加载中…',
})
function report(cause: unknown) {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  else error.value = cause instanceof Error ? cause.message : String(cause)
}
async function load(nextOffset = 0, filter = memberFilter.value) {
  const request = ++generation
  const brandId = props.brandId
  orders.value = []; detail.value = null; more.value = false; error.value = ''
  if (!brandId) { loading.value = false; return }
  loading.value = true
  try {
    const rows = await api.list(brandId, filter, 51, nextOffset)
    if (request !== generation || brandId !== props.brandId) return
    orders.value = rows.slice(0, 50); more.value = rows.length > 50
    offset.value = nextOffset; memberFilter.value = filter
  } catch (cause) { if (request === generation) report(cause) }
  finally { if (request === generation) loading.value = false }
}
async function read(order: BetOrder) {
  const request = ++generation
  const brandId = props.brandId
  detail.value = null; error.value = ''; loading.value = true
  try {
    const current = await api.read(brandId, order.id)
    if (request === generation && brandId === props.brandId) detail.value = current
  } catch (cause) { if (request === generation) report(cause) }
  finally { if (request === generation) loading.value = false }
}
const json = (value: unknown) => JSON.stringify(value, null, 2)
const stateLabel = (state: BetOrder['status']) => (props.locale === 'en' ? {
  placed: 'Placed', bet_cancelled: 'Bet cancelled', judged_cancelled: 'Judged cancelled', abnormal: 'Abnormal order', won: 'Won', lost: 'Lost',
} : {
  placed: '已投注', bet_cancelled: '投注取消', judged_cancelled: '判定取消', abnormal: '异常注单', won: '中奖', lost: '未中奖',
})[state]
watch(() => props.brandId, () => {
  catalogue.value = false
  draftMember.value = ''; memberFilter.value = ''; offset.value = 0
  void load()
}, { immediate: true, flush: 'sync' })
</script>

<template>
  <div data-testid="platform-bets">
    <div class="wallet-pagination" data-testid="platform-bet-tabs">
      <button :class="catalogue ? 'secondary' : 'primary'" @click="chooseTab(false)">{{ copy.title }}</button>
      <button :class="catalogue ? 'primary' : 'secondary'" @click="chooseTab(true)">{{ locale === 'en' ? 'Games and draws' : '彩种与开奖' }}</button>
    </div>
    <LotteryPanel v-if="catalogue" :brand-id="brandId" :locale="locale" @failure="emit('failure', $event)" />
    <template v-else>
    <div class="page-heading"><div><div class="eyebrow">{{ copy.note }}</div><h1>{{ copy.title }}</h1></div></div>
    <form class="panel wallet-query" @submit.prevent="load(0, draftMember.trim())"><label>{{ copy.member }}<input v-model="draftMember" :disabled="loading || !brandId" autocomplete="off" /></label><button class="secondary" :disabled="loading || !brandId">{{ copy.query }}</button></form>
    <p v-if="error" class="message error" role="alert">{{ error }}</p><p v-if="loading" class="loading-line">{{ copy.loading }}</p>
    <section class="panel reward-detail">
      <div class="table-wrap"><table><thead><tr><th>{{ copy.order }}</th><th>{{ copy.period }}</th><th>{{ copy.points }}</th><th>{{ copy.state }}</th><th>{{ copy.date }}</th></tr></thead><tbody>
        <tr v-for="order in orders" :key="order.id"><td><button class="row-action" @click="read(order)">{{ order.id }}</button><small>{{ order.brand_member_id }}</small></td><td>{{ order.period_id }}</td><td>{{ order.total_points }}</td><td>{{ stateLabel(order.status) }}</td><td>{{ order.placed_at }}</td></tr>
        <tr v-if="!orders.length"><td colspan="5" class="empty-state">{{ copy.empty }}</td></tr>
      </tbody></table></div>
      <div class="wallet-pagination" data-testid="platform-bet-pages"><button class="secondary" :disabled="loading || offset === 0 || !brandId" @click="load(offset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(offset / 50) + 1 }}</span><button class="secondary" :disabled="loading || !more" @click="load(offset + 50)">{{ copy.next }}</button></div>
    </section>
    <section v-if="detail" class="panel reward-detail" data-testid="platform-bet-detail">
      <div class="panel-heading"><h2>{{ copy.detail }}</h2><span>{{ stateLabel(detail.status) }}</span></div>
      <dl class="confirm-list reward-fields">
        <div><dt>{{ copy.order }}</dt><dd>{{ detail.id }}</dd></div><div><dt>{{ copy.version }}</dt><dd>{{ detail.version }}</dd></div>
        <div><dt>{{ copy.game }}</dt><dd>{{ detail.game_id }}</dd></div><div><dt>{{ copy.period }}</dt><dd>{{ detail.period_id }}</dd></div><div><dt>{{ copy.play }}</dt><dd>{{ detail.play_id }}</dd></div>
        <div><dt>{{ copy.unit }}</dt><dd>{{ detail.unit_points }}</dd></div><div><dt>{{ copy.combinations }}</dt><dd>{{ detail.combination_count }}</dd></div><div><dt>{{ copy.multiplier }}</dt><dd>{{ detail.multiplier }}</dd></div>
        <div><dt>{{ copy.points }}</dt><dd>{{ detail.total_points }}</dd></div><div><dt>{{ copy.prize }}</dt><dd>{{ detail.prize_points }}</dd></div><div><dt>{{ copy.rule }}</dt><dd>{{ detail.rule_version_id }}</dd></div><div><dt>{{ copy.hash }}</dt><dd>{{ detail.definition_hash }}</dd></div>
      </dl>
      <div class="bet-snapshots"><h3>{{ copy.snapshots }}</h3>
        <details><summary>{{ copy.selection }}</summary><pre>{{ json(detail.selection_raw) }}</pre></details>
        <details><summary>{{ copy.normalized }}</summary><pre>{{ json(detail.selection_normalized) }}</pre></details>
        <details><summary>{{ copy.expanded }}</summary><pre>{{ json(detail.expanded_bets) }}</pre></details>
        <details><summary>{{ copy.definition }}</summary><pre>{{ json(detail.definition_snapshot) }}</pre></details>
        <details><summary>{{ copy.policy }}</summary><pre>{{ json({ config: detail.policy_snapshot, versions: detail.policy_versions }) }}</pre></details>
        <details><summary>{{ copy.allocation }}</summary><pre>{{ json(detail.deduction_allocation) }}</pre></details>
      </div>
    </section>
    </template>
  </div>
</template>
