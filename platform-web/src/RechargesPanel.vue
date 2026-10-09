<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { PlatformApiError } from './platform-api'
import { createPlatformRechargeApi, type Recharge } from './recharge-api'

const props = defineProps<{ brandId: string; memberId: string; locale: 'en' | 'zh-CN' }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformRechargeApi()
const draftMember = ref('')
const filterMember = ref('')
const rows = ref<Recharge[]>([])
const detail = ref<Recharge | null>(null)
const offset = ref(0)
const more = ref(false)
const loading = ref(false)
const error = ref('')
let generation = 0
const copy = computed(() => props.locale === 'en' ? {
  title: 'Recharge records', note: 'Read-only list snapshots · No recharge or confirmation controls', member: 'Member ID (optional)', query: 'Query recharges', order: 'Order', points: 'Points', state: 'State', created: 'Created', pending: 'Pending', confirmed: 'Confirmed', cancelled: 'Cancelled', detail: 'Recharge record snapshot', version: 'Version', proof: 'Proof reference', remark: 'Remark', creator: 'Created by', confirmer: 'Confirmed by', confirmedAt: 'Confirmed at', ledger: 'Ledger entry', audit: 'Audit record', previous: 'Previous page', next: 'Next page', empty: 'No recharge records.', loading: 'Loading…',
} : {
  title: '充值记录', note: '只读列表快照，不提供创建或确认充值操作', member: '会员编号（可选）', query: '查询充值', order: '订单', points: '积分', state: '状态', created: '创建时间', pending: '待确认', confirmed: '已确认', cancelled: '已取消', detail: '充值记录快照', version: '版本', proof: '凭证引用', remark: '备注', creator: '创建人', confirmer: '确认人', confirmedAt: '确认时间', ledger: '账本分录', audit: '审计记录', previous: '上一页', next: '下一页', empty: '暂无充值记录。', loading: '加载中…',
})
async function load(nextOffset = 0, member = filterMember.value) {
  const request = ++generation
  const brandId = props.brandId
  rows.value = []; detail.value = null; more.value = false; error.value = ''
  if (!brandId) { loading.value = false; return }
  loading.value = true
  try {
    const records = await api.list(brandId, member, 51, nextOffset)
    if (request !== generation || props.brandId !== brandId) return
    rows.value = records.slice(0, 50); more.value = records.length > 50
    offset.value = nextOffset; filterMember.value = member
  } catch (cause) {
    if (request !== generation) return
    if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
    else error.value = cause instanceof Error ? cause.message : String(cause)
  } finally { if (request === generation) loading.value = false }
}
watch(() => [props.brandId, props.memberId], () => {
  draftMember.value = props.memberId; filterMember.value = props.memberId; offset.value = 0
  void load()
}, { immediate: true, flush: 'sync' })
</script>

<template>
  <div data-testid="platform-recharges">
    <div class="page-heading"><div><div class="eyebrow">{{ copy.note }}</div><h1>{{ copy.title }}</h1></div></div>
    <form class="panel wallet-query" @submit.prevent="load(0, draftMember.trim())"><label>{{ copy.member }}<input v-model="draftMember" :disabled="loading || !brandId" autocomplete="off" /></label><button class="secondary" :disabled="loading || !brandId">{{ copy.query }}</button></form>
    <p v-if="error" class="message error" role="alert">{{ error }}</p><p v-if="loading" class="loading-line">{{ copy.loading }}</p>
    <section class="panel reward-detail"><div class="table-wrap"><table><thead><tr><th>{{ copy.order }}</th><th>{{ copy.points }}</th><th>{{ copy.state }}</th><th>{{ copy.created }}</th></tr></thead><tbody>
      <tr v-for="order in rows" :key="order.id"><td><button class="row-action" @click="detail = order">{{ order.id }}</button><small>{{ order.member_id }}</small></td><td>{{ order.points }}</td><td>{{ copy[order.state] }}</td><td>{{ order.created_at }}</td></tr>
      <tr v-if="!rows.length"><td colspan="4" class="empty-state">{{ copy.empty }}</td></tr>
    </tbody></table></div><div class="wallet-pagination" data-testid="platform-recharge-pages"><button class="secondary" :disabled="loading || offset === 0 || !brandId" @click="load(offset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(offset / 50) + 1 }}</span><button class="secondary" :disabled="loading || !more" @click="load(offset + 50)">{{ copy.next }}</button></div></section>
    <section v-if="detail" class="panel reward-detail" data-testid="platform-recharge-detail">
      <div class="panel-heading"><h2>{{ copy.detail }}</h2><span>{{ copy[detail.state] }} · v{{ detail.version }}</span></div>
      <dl class="confirm-list reward-fields">
        <div><dt>{{ copy.order }}</dt><dd>{{ detail.id }}</dd></div><div><dt>{{ copy.points }}</dt><dd>{{ detail.points }}</dd></div><div><dt>{{ copy.proof }}</dt><dd>{{ detail.proof_reference }}</dd></div><div><dt>{{ copy.remark }}</dt><dd>{{ detail.remark }}</dd></div><div><dt>{{ copy.creator }}</dt><dd>{{ detail.created_by }}</dd></div>
        <div><dt>{{ copy.confirmer }}</dt><dd>{{ detail.confirmed_by || '—' }}</dd></div><div><dt>{{ copy.confirmedAt }}</dt><dd>{{ detail.confirmed_at || '—' }}</dd></div><div><dt>{{ copy.ledger }}</dt><dd>{{ detail.ledger_entry_id || '—' }}</dd></div><div><dt>{{ copy.audit }}</dt><dd>{{ detail.audit_log_id || '—' }}</dd></div>
      </dl>
    </section>
  </div>
</template>
