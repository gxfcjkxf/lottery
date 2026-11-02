<script setup lang="ts">
import { nextTick } from 'vue'
const filterReset = ref(0)

import ManagementList from "@lottery/shared/ManagementList.vue"
import { auditReasonLabel } from "@lottery/shared";
import { readPaginationTotal } from '@lottery/shared'
const totalCount = ref('0')

import SelectControl from '@lottery/shared/SelectControl.vue'
import { useOperationToast } from "@lottery/shared";
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { usePlatformI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from "@lottery/shared";
import { usePlatformI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vInlineValidation } from '@lottery/shared'
import { usePlatformI18n } from "./i18n"
const ui = usePlatformI18n().t
import TablePagination from "@lottery/shared/TablePagination.vue"
import DetailModal from "@lottery/shared/DetailModal.vue"
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import type { WithdrawalHistory, WithdrawalOrder, WithdrawalState } from '@lottery/shared'
import { PlatformApiError } from './platform-api'
import { createPlatformWithdrawalApi } from './withdrawal-api'
import FilterAssociationSearch from "@lottery/shared/FilterAssociationSearch.vue"
import { formatPlatformDateTime, type PlatformDisplaySettings } from './platform-format'

const props = defineProps<{ brandId: string; locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings; initialState?: WithdrawalState }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformWithdrawalApi()
const states: WithdrawalState[] = ['reviewing', 'processing', 'paid', 'rejected', 'failed', 'cancelled']
const memberDraft = ref('')
const memberSearchPending = ref(false)
const stateDraft = ref<WithdrawalState | ''>('')
const activeMember = ref('')
const activeState = ref<WithdrawalState | ''>('')
const rows = ref<WithdrawalOrder[]>([])
const detail = ref<WithdrawalOrder | null>(null)
const history = ref<WithdrawalHistory | null>(null)
const historyOpen=ref(false),historyLoading=ref(false),historyFailed=ref(false)
let historyGeneration=0
const offset = ref(0)
const more = ref(false)
const loading = ref(false)
const error = ref<string | import("@lottery/shared").LocalizedMessage>("")
let generation = 0
async function clearFilters() { filterReset.value++; memberDraft.value = ''; stateDraft.value = ''; await nextTick(); await load(0, '', ''); }
async function submitMemberQuery() {
  if (memberSearchPending.value) return
  try {
    await load(0, memberDraft.value, stateDraft.value)
  } catch (cause) { report(cause) }
}
const labels = computed(() => props.locale === 'en' ? {
  reviewing: ui('platform.copy.522d775b'), processing: ui('platform.copy.64d6efdd'), paid: ui('platform.copy.9980eb5d'), rejected: ui('platform.copy.8a2108c4'), failed: ui('platform.copy.f628af96'), cancelled: ui('platform.copy.d9b0562c'),
} : { reviewing: ui('platform.copy.522d775b'), processing: ui('platform.copy.64d6efdd'), paid: ui('platform.copy.9980eb5d'), rejected: ui('platform.copy.8a2108c4'), failed: ui('platform.copy.f628af96'), cancelled: ui('platform.copy.d9b0562c') })
const copy = computed(() => props.locale === 'en' ? {
  title: ui('platform.copy.fc97c660'), member: ui('platform.copy.3c207805'), memberPlaceholder: ui('platform.copy.2605fbd1'), state: ui('platform.copy.274ae047'), all: ui('platform.copy.978bc6d7'), query: ui('platform.copy.a1b2920e'), order: ui('platform.copy.68419640'), points: ui('platform.copy.a1f74153'), version: ui('platform.copy.59405491'), created: ui('platform.copy.00e7cdbe'), detail: ui('platform.copy.27f7a5a4'), close: ui('platform.copy.27b2fd29'), sources: ui('platform.copy.f04504c3'), source: ui('platform.copy.4fe341a5'), reason: ui('platform.copy.d43c4698'), reserve: ui('platform.copy.7af1e1a7'), release: ui('platform.copy.ea447322'), paid: ui('platform.copy.c65da62b'), cycle: ui('platform.copy.f8f5cc18'), cursor: ui('platform.copy.e2c6f576'), reservedVersion: ui('platform.copy.65b4bf16'), reviewed: ui('platform.copy.8ec1f15b'), completed: ui('platform.copy.df5884b0'), history: ui('platform.copy.1ae69897'), from: ui('platform.copy.150d10d3'), to: ui('platform.copy.d23bb391'), actor: ui('platform.copy.6ac2fcc9'), previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), empty: ui('platform.copy.dde1013b'), recharge: ui('platform.copy.3420a04d'), winning: ui('platform.copy.57e492c6'), gift: ui('platform.copy.c8d77732'), commission: ui('platform.copy.648ed708'),
} : {
  title: ui('platform.copy.fc97c660'), member: ui('platform.copy.3c207805'), memberPlaceholder: ui('platform.copy.2605fbd1'), state: ui('platform.copy.274ae047'), all: ui('platform.copy.978bc6d7'), query: ui('platform.copy.a1b2920e'), order: ui('platform.copy.68419640'), points: ui('platform.copy.a1f74153'), version: ui('platform.copy.59405491'), created: ui('platform.copy.00e7cdbe'), detail: ui('platform.copy.27f7a5a4'), close: ui('platform.copy.27b2fd29'), sources: ui('platform.copy.f04504c3'), source: ui('platform.copy.4fe341a5'), reason: ui('platform.copy.d43c4698'), reserve: ui('platform.copy.7af1e1a7'), release: ui('platform.copy.ea447322'), paid: ui('platform.copy.c65da62b'), cycle: ui('platform.copy.f8f5cc18'), cursor: ui('platform.copy.e2c6f576'), reservedVersion: ui('platform.copy.65b4bf16'), reviewed: ui('platform.copy.8ec1f15b'), completed: ui('platform.copy.df5884b0'), history: ui('platform.copy.1ae69897'), from: ui('platform.copy.150d10d3'), to: ui('platform.copy.d23bb391'), actor: ui('platform.copy.6ac2fcc9'), previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), empty: ui('platform.copy.dde1013b'), recharge: ui('platform.copy.3420a04d'), winning: ui('platform.copy.57e492c6'), gift: ui('platform.copy.c8d77732'), commission: ui('platform.copy.648ed708'),
})
function report(cause: unknown) {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  else error.value = cause instanceof Error ? publicErrorMessage(cause) : publicErrorMessage(cause)
}
async function load(nextOffset = 0, member = activeMember.value, state = activeState.value) {
  closeDetail()
  const request = ++generation
  const brandId = props.brandId
  error.value = ''
  if (!brandId) { loading.value = false; return }
  loading.value = true
  try {
    const page = await api.list(brandId, { limit: 50, offset: nextOffset, ...(member ? { memberId: member } : {}), ...(state ? { state } : {}) })
    if (request !== generation || props.brandId !== brandId) return
    [rows.value,totalCount.value] = [page.items,readPaginationTotal(page)]; offset.value = page.offset; more.value = page.has_more
    activeMember.value = member; activeState.value = state
  } catch (cause) { if (request === generation) report(cause) }
  finally { if (request === generation) loading.value = false }
}
async function open(order: WithdrawalOrder) {
  closeDetail()
  const request = ++generation
  const brandId = props.brandId
  error.value = ''; loading.value = true
  try {
    const current = await api.read(brandId, order.id)
    if (request === generation && props.brandId === brandId) detail.value = current
  } catch (cause) { if (request === generation) report(cause) }
  finally { if (request === generation) loading.value = false }
}
function closeDetail(){generation++;historyGeneration++;detail.value=null;history.value=null;historyOpen.value=historyLoading.value=historyFailed.value=false;loading.value=false}
async function loadHistory(){
 if(!detail.value||historyLoading.value)return
 const request=++historyGeneration,brandId=props.brandId,orderId=detail.value.id
 historyOpen.value=true;historyLoading.value=true;historyFailed.value=false;history.value=null;error.value=''
 try{const result=await api.history(brandId,orderId);if(request===historyGeneration&&props.brandId===brandId&&detail.value?.id===orderId)history.value=result}
 catch(cause){if(request===historyGeneration&&props.brandId===brandId&&detail.value?.id===orderId){historyFailed.value=true;report(cause)}}
 finally{if(request===historyGeneration)historyLoading.value=false}
}
onBeforeUnmount(()=>{generation++;historyGeneration++})
watch(() => props.brandId, () => {
  [rows.value,totalCount.value]=[[],'0']; more.value = false;
  memberDraft.value = ''; stateDraft.value = props.initialState ?? ''; activeMember.value = ''; activeState.value = props.initialState ?? ''; offset.value = 0
  void withGlobalLoading(load)
}, { immediate: true, flush: 'sync' })

useOperationToast(error, value => displayPublicError(value), 'error');
</script>

<template>
  <ManagementList data-testid="platform-withdrawals">
    <template #filters><form novalidate v-inline-validation class="wallet-query ui-filter-bar" @submit.prevent="withGlobalLoading(submitMemberQuery)">
      <label>
        <div class="ui-filter-input">
          <FilterAssociationSearch :reset-key="filterReset" v-model="memberDraft" v-model:pending="memberSearchPending" :brand-id="brandId" surface="platform" resource="withdrawal" :label="copy.member" :locale="locale" />
        </div>
      </label>
      <label>
        <SelectControl v-model="stateDraft" :label="copy.state" :disabled="loading || !brandId" :options="[{ value: '', label: (String(copy.all)).trim() }, ...((states).map((state) => ({ value: state, label: (String(labels[state])).trim() })))]" />
      </label>
      <button class="button button-primary" :disabled="loading || !brandId || memberSearchPending">{{ copy.query }}</button>
      <button type="button" class="button button-secondary" :disabled="loading || !brandId" @click="withGlobalLoading(clearFilters)">{{ ui('admin.copy.b089734d') }}</button>
    </form></template>

    <section class="reward-detail console-stack"><div class="table-wrap"><table class="console-table console-responsive-table"><thead><tr><th>{{ copy.order }}</th><th>{{ copy.points }}</th><th><BusinessDetails :value="copy.state" :locale="associationLocale" /></th><th>{{ copy.created }}</th></tr></thead><tbody>
      <tr v-for="order in rows" :key="order.id"><td :data-label="copy.order"><button class="text-button row-action" @click="withGlobalLoading(open, order)">{{ order.id }}</button><small><AssociationName :record="order" kind="member" :id="order.member_id" :locale="associationLocale" /></small></td><td :data-label="copy.points">{{ order.points }}</td><td :data-label="copy.state">{{ labels[order.state] }}</td><td :data-label="copy.created">{{ formatPlatformDateTime(order.created_at, props.displaySettings, props.locale) }}</td></tr>
      <tr v-if="!loading && !error && !rows.length"><td colspan="4" class="empty-state">{{ copy.empty }}</td></tr>
    </tbody></table></div></section>
    <DetailModal v-if="detail" :title="copy.detail" :close-label="copy.close" @close="closeDetail">
<section v-if="detail" class="detail-section" data-testid="platform-withdrawal-detail">
      <div class="detail-summary"><span class="status-pill" :class="detail.state">{{ labels[detail.state] }}</span></div>
      <dl class="confirm-list reward-fields">
        <div><dt>{{ copy.order }}</dt><dd>{{ detail.id }}</dd></div><div><dt>{{ copy.points }}</dt><dd>{{ detail.points }}</dd></div><div><dt>{{ copy.reason }}</dt><dd>{{ auditReasonLabel(detail.decision_reason, associationLocale) }}</dd></div>
        <div><dt>{{ copy.reserve }}</dt><dd>{{ detail.reserve_entry_id }}</dd></div><div><dt>{{ copy.release }}</dt><dd>{{ detail.release_entry_id || '—' }}</dd></div><div><dt>{{ copy.paid }}</dt><dd>{{ detail.paid_entry_id || '—' }}</dd></div>
        <div><dt>{{ copy.cycle }}</dt><dd>{{ formatPlatformDateTime(detail.cycle_from_at, props.displaySettings, props.locale) }}</dd></div><div><dt>{{ copy.reviewed }}</dt><dd>{{ formatPlatformDateTime(detail.reviewed_at, props.displaySettings, props.locale) }}</dd></div><div><dt>{{ copy.completed }}</dt><dd>{{ formatPlatformDateTime(detail.completed_at, props.displaySettings, props.locale) }}</dd></div>
      </dl>
      <div class="table-wrap"><table class="console-table console-responsive-table"><thead><tr><th>{{ copy.sources }}</th><th>{{ copy.points }}</th></tr></thead><tbody><tr v-for="allocation in detail.source_allocation" :key="allocation.source"><td :data-label="copy.sources">{{ copy[allocation.source] }}</td><td :data-label="copy.points">{{ allocation.points }}</td></tr></tbody></table></div>
      <div class="form-actions ui-form-actions"><button class="button button-secondary" type="button" :disabled="historyLoading" :aria-expanded="historyOpen" @click="withGlobalLoading(loadHistory)">{{ ui(historyFailed ? 'detailHistory.retry' : 'detailHistory.load') }}</button></div>
      <div v-if="history" class="table-wrap" data-testid="platform-withdrawal-history"><table class="console-table console-responsive-table"><thead><tr><th>{{ copy.from }}</th><th>{{ copy.to }}</th><th>{{ copy.actor }}</th><th>{{ copy.reason }}</th><th>{{ copy.created }}</th></tr></thead><tbody><tr v-for="item in history.items" :key="item.id"><td :data-label="copy.from">{{ item.from_state ? labels[item.from_state] : '—' }}</td><td :data-label="copy.to">{{ labels[item.to_state] }}</td><td :data-label="copy.actor"><BusinessDetails :value="item.actor_type" :locale="associationLocale" /></td><td :data-label="copy.reason">{{ auditReasonLabel(item.reason, associationLocale) }}</td><td :data-label="copy.created">{{ formatPlatformDateTime(item.created_at, props.displaySettings, props.locale) }}</td></tr></tbody></table></div>
    </section>
</DetailModal>
  <template #pagination><TablePagination :show-empty="!loading && !error" :total="totalCount" :page="Math.floor(offset / 50 + 1)" :count="rows.length" :locale="locale" data-testid="platform-withdrawal-pages" :page-size="50" :previous-disabled="loading || offset === 0 || !brandId" :next-disabled="loading || !more" :previous-label="copy.previous" :next-label="copy.next" :go-to="(target: number) => load((target - 1) * 50)" /></template></ManagementList>
</template>
