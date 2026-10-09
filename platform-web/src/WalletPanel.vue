<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { WALLET_SOURCES, WALLET_STATES } from '@lottery/shared'
import { PlatformApiError } from './platform-api'
import { createPlatformWalletApi, type Wallet, type LedgerEntry } from './wallet-api'
import RechargesPanel from './RechargesPanel.vue'
import ReportsPanel from './ReportsPanel.vue'

const props = defineProps<{ brandId: string; memberId: string; locale: 'en' | 'zh-CN' }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformWalletApi()
const inputMember = ref('')
const activeMember = ref('')
const wallet = ref<Wallet | null>(null)
const entries = ref<LedgerEntry[]>([])
const selected = ref<LedgerEntry | null>(null)
const offset = ref(0)
const more = ref(false)
const loading = ref(false)
const error = ref('')
const financeSection = ref<'wallet' | 'recharges' | 'reports'>('wallet')
const financeMember = ref('')
let generation = 0
const copy = computed(() => props.locale === 'en' ? {
  title: 'Member points', readOnly: 'Read-only wallet and immutable ledger', member: 'Member ID', load: 'Load wallet', refresh: 'Refresh', available: 'Available', frozen: 'Frozen', withdrawal: 'Withdrawal pending', display: 'Display points', version: 'Version', source: 'Source', recharge: 'Recharge', winning: 'Winning', gift: 'Gift', commission: 'Commission', manual_frozen: 'Manual freeze', system_frozen: 'System freeze', ledger: 'Point ledger', entry: 'Entry', type: 'Type', reason: 'Reason', date: 'Date', previous: 'Previous page', next: 'Next page', detail: 'Ledger entry detail', before: 'Before', delta: 'Change', after: 'After', state: 'State', empty: 'No ledger entries.', loading: 'Loading…',
} : {
  title: '会员积分', readOnly: '只读钱包及不可变积分流水', member: '会员编号', load: '查询钱包', refresh: '刷新', available: '可用积分', frozen: '冻结积分', withdrawal: '提现中积分', display: '显示积分', version: '版本', source: '来源', recharge: '充值', winning: '中奖', gift: '赠送', commission: '佣金', manual_frozen: '人工冻结', system_frozen: '系统冻结', ledger: '积分流水', entry: '分录', type: '类型', reason: '原因', date: '时间', previous: '上一页', next: '下一页', detail: '流水详情', before: '之前', delta: '变动', after: '之后', state: '状态', empty: '暂无积分流水。', loading: '加载中…',
})
function clear() {
  generation++
  wallet.value = null; entries.value = []; selected.value = null
  offset.value = 0; more.value = false; activeMember.value = ''; loading.value = false; error.value = ''
}
function showRecharges() {
  if (financeSection.value === 'recharges') return
  financeMember.value = activeMember.value || financeMember.value || props.memberId
  clear()
  financeSection.value = 'recharges'
}
function showReports() {
  if (financeSection.value === 'reports') return
  financeMember.value = activeMember.value || financeMember.value || props.memberId
  clear()
  financeSection.value = 'reports'
}
function showWallet() {
  financeSection.value = 'wallet'
  if (financeMember.value) {
    inputMember.value = financeMember.value
    void load()
  }
}
function report(cause: unknown) {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  else error.value = cause instanceof Error ? cause.message : String(cause)
}
async function load() {
  clear()
  const request = generation
  const brandId = props.brandId
  const memberId = inputMember.value.trim()
  loading.value = true
  try {
    const [current, rows] = await Promise.all([api.wallet(brandId, memberId), api.ledger(brandId, memberId, 51, 0)])
    if (request !== generation || brandId !== props.brandId) return
    wallet.value = current; entries.value = rows.slice(0, 50); more.value = rows.length > 50; activeMember.value = memberId
  } catch (cause) {
    if (request === generation) report(cause)
  } finally { if (request === generation) loading.value = false }
}
async function page(nextOffset: number) {
  const request = ++generation
  const brandId = props.brandId
  const memberId = activeMember.value
  entries.value = []; selected.value = null; more.value = false; error.value = ''; loading.value = true
  try {
    const rows = await api.ledger(brandId, memberId, 51, nextOffset)
    if (request !== generation || brandId !== props.brandId) return
    entries.value = rows.slice(0, 50); more.value = rows.length > 50; offset.value = nextOffset
  } catch (cause) { if (request === generation) report(cause) }
  finally { if (request === generation) loading.value = false }
}
watch(() => [props.brandId, props.memberId], () => {
  financeSection.value = 'wallet'; financeMember.value = ''
  clear(); inputMember.value = props.memberId
  if (props.brandId && props.memberId) void load()
}, { immediate: true, flush: 'sync' })
</script>

<template>
  <div class="wallet-pagination" data-testid="platform-finance-tabs">
    <button class="secondary" :aria-pressed="financeSection === 'wallet'" @click="showWallet">{{ locale === 'en' ? 'Wallet' : '钱包' }}</button>
    <button class="secondary" :aria-pressed="financeSection === 'recharges'" @click="showRecharges">{{ locale === 'en' ? 'Recharge records' : '充值记录' }}</button>
    <button class="secondary" :aria-pressed="financeSection === 'reports'" @click="showReports">{{ locale === 'en' ? 'Financial reports' : '财务报表' }}</button>
  </div>
  <RechargesPanel v-if="financeSection === 'recharges'" :brand-id="brandId" :member-id="financeMember" :locale="locale" @failure="emit('failure', $event)" />
  <ReportsPanel v-else-if="financeSection === 'reports'" :brand-id="brandId" :member-id="financeMember" :locale="locale" @failure="emit('failure', $event)" />
  <template v-else>
  <div data-testid="platform-wallet">
    <div class="page-heading"><div><div class="eyebrow">{{ copy.readOnly }}</div><h1>{{ copy.title }}</h1></div></div>
    <form class="panel wallet-query" @submit.prevent="load">
      <label>{{ copy.member }}<input v-model="inputMember" required :disabled="loading || !brandId" autocomplete="off" /></label>
      <button class="secondary" :disabled="loading || !brandId">{{ copy.load }}</button>
    </form>
    <p v-if="error" class="message error" role="alert">{{ error }}</p>
    <p v-if="loading" class="loading-line">{{ copy.loading }}</p>
    <template v-if="wallet">
      <section class="panel" data-testid="platform-wallet-balances">
        <div class="panel-heading"><h2>{{ activeMember }}</h2><span>{{ copy.version }} {{ wallet.version }}</span></div>
        <dl class="confirm-list reward-fields">
          <div><dt>{{ copy.display }}</dt><dd>{{ wallet.display_points }}</dd></div>
          <div><dt>{{ copy.available }}</dt><dd>{{ wallet.available_points }}</dd></div>
          <div><dt>{{ copy.frozen }}</dt><dd>{{ wallet.frozen_points }}</dd></div>
          <div><dt>{{ copy.withdrawal }}</dt><dd>{{ wallet.withdrawal_points }}</dd></div>
        </dl>
        <div class="table-wrap"><table><thead><tr><th>{{ copy.source }}</th><th v-for="state in WALLET_STATES" :key="state">{{ copy[state] }}</th></tr></thead><tbody>
          <tr v-for="source in WALLET_SOURCES" :key="source"><td>{{ copy[source] }}</td><td v-for="state in WALLET_STATES" :key="state">{{ wallet.by_source[source][state] }}</td></tr>
        </tbody></table></div>
      </section>
      <section class="panel reward-detail" data-testid="platform-wallet-ledger">
        <div class="panel-heading"><h2>{{ copy.ledger }}</h2><button class="secondary" :disabled="loading" @click="load">{{ copy.refresh }}</button></div>
        <div class="table-wrap"><table><thead><tr><th>{{ copy.entry }}</th><th>{{ copy.type }}</th><th>{{ copy.reason }}</th><th>{{ copy.date }}</th></tr></thead><tbody>
          <tr v-for="entry in entries" :key="entry.id"><td><button class="row-action" @click="selected = entry">{{ entry.id }}</button></td><td>{{ entry.entry_type }}</td><td>{{ entry.reason }}</td><td>{{ entry.created_at }}</td></tr>
          <tr v-if="!entries.length"><td colspan="4" class="empty-state">{{ copy.empty }}</td></tr>
        </tbody></table></div>
        <div class="wallet-pagination" data-testid="platform-wallet-pages"><button class="secondary" :disabled="loading || offset === 0" @click="page(offset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(offset / 50) + 1 }}</span><button class="secondary" :disabled="loading || !more" @click="page(offset + 50)">{{ copy.next }}</button></div>
      </section>
      <section v-if="selected" class="panel reward-detail" data-testid="platform-wallet-entry">
        <div class="panel-heading"><div><h2>{{ copy.detail }}</h2><p class="mono">{{ selected.id }}</p><p>{{ selected.reason }}</p></div></div>
        <div class="table-wrap"><table><thead><tr><th>{{ copy.source }}</th><th>{{ copy.state }}</th><th>{{ copy.before }}</th><th>{{ copy.delta }}</th><th>{{ copy.after }}</th></tr></thead><tbody>
          <template v-for="source in WALLET_SOURCES" :key="source"><tr v-for="state in WALLET_STATES" :key="state"><td>{{ copy[source] }}</td><td>{{ copy[state] }}</td><td>{{ selected.before_snapshot[source][state] }}</td><td>{{ selected.delta_snapshot[source][state] }}</td><td>{{ selected.after_snapshot[source][state] }}</td></tr></template>
        </tbody></table></div>
      </section>
    </template>
  </div>
  </template>
</template>
