<script setup lang="ts">
import ManagementList from "@lottery/shared/ManagementList.vue"
import { readPaginationTotal } from '@lottery/shared'
const totalCount = ref('0')

import TablePagination from "@lottery/shared/TablePagination.vue"
import { useOperationToast } from "@lottery/shared";
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { usePlatformI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import DirectorySearch from "@lottery/shared/DirectorySearch.vue";
import { usePlatformI18n } from "./i18n"
const ui = usePlatformI18n().t
import DetailModal from "@lottery/shared/DetailModal.vue"
import { computed, ref, watch } from 'vue'
import { PlatformApiError } from './platform-api'
import { createPlatformRechargeApi, type Recharge } from './recharge-api'
import FilterAssociationSearch from "@lottery/shared/FilterAssociationSearch.vue"
import { formatPlatformDateTime, type PlatformDisplaySettings } from './platform-format'
const pageSize = ref(50)


const props = defineProps<{ brandId: string; memberId: string; locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformRechargeApi()
const draftMember = ref('')
const memberSearchPending = ref(false)
const filterMember = ref('')
const rows = ref<Recharge[]>([])
const detail = ref<Recharge | null>(null)
const offset = ref(0)
const more = ref(false)
const loading = ref(false)
const error = ref<string | import("@lottery/shared").LocalizedMessage>("")
let generation = 0
function report(cause: unknown) {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  else error.value = cause instanceof Error ? publicErrorMessage(cause) : publicErrorMessage(cause)
}
const filterReset=ref(0)

async function submitMemberQuery() {
  if (memberSearchPending.value) return
  try {
    await load(0, draftMember.value)
  } catch (cause) { report(cause) }
}
const copy = computed(() => props.locale === 'en' ? {
  member: ui('platform.copy.3c207805'), memberPlaceholder: ui('platform.copy.2605fbd1'), query: ui('platform.copy.f5899769'), order: ui('platform.copy.68419640'), points: ui('platform.copy.a1f74153'), state: ui('platform.copy.274ae047'), created: ui('platform.copy.00e7cdbe'), pending: ui('platform.copy.e8534749'), confirmed: ui('platform.copy.563a3d42'), cancelled: ui('platform.copy.d9b0562c'), detail: ui('platform.copy.8539f8f9'), close: ui('platform.copy.27b2fd29'), version: ui('platform.copy.59405491'), proof: ui('platform.copy.cb0b05d7'), remark: ui('platform.copy.d1f3478c'), creator: ui('platform.copy.6d460b01'), confirmer: ui('platform.copy.66946cab'), confirmedAt: ui('platform.copy.0612adf5'), ledger: ui('platform.copy.bed1cc9b'), audit: ui('platform.copy.b4e123da'), previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), empty: ui('platform.copy.bd4db2d3'),
} : {
  member: ui('platform.copy.3c207805'), memberPlaceholder: ui('platform.copy.2605fbd1'), query: ui('platform.copy.f5899769'), order: ui('platform.copy.68419640'), points: ui('platform.copy.a1f74153'), state: ui('platform.copy.274ae047'), created: ui('platform.copy.00e7cdbe'), pending: ui('platform.copy.e8534749'), confirmed: ui('platform.copy.563a3d42'), cancelled: ui('platform.copy.d9b0562c'), detail: ui('platform.copy.8539f8f9'), close: ui('platform.copy.27b2fd29'), version: ui('platform.copy.59405491'), proof: ui('platform.copy.cb0b05d7'), remark: ui('platform.copy.d1f3478c'), creator: ui('platform.copy.6d460b01'), confirmer: ui('platform.copy.66946cab'), confirmedAt: ui('platform.copy.0612adf5'), ledger: ui('platform.copy.bed1cc9b'), audit: ui('platform.copy.b4e123da'), previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), empty: ui('platform.copy.bd4db2d3'),
})
async function load(nextOffset = 0, member = filterMember.value) {
  const request = ++generation
  const brandId = props.brandId
  detail.value = null; error.value = ''
  if (!brandId) { loading.value = false; return }
  loading.value = true
  try {
    const records = await api.list(brandId, member, pageSize.value, nextOffset)
    if (request !== generation || props.brandId !== brandId) return
    [rows.value, totalCount.value] = [records.items, readPaginationTotal(records)]; more.value = BigInt(nextOffset + records.items.length) < BigInt(totalCount.value)
    offset.value = nextOffset; filterMember.value = member
  } catch (cause) {
    if (request !== generation) return
    if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
    else error.value = cause instanceof Error ? publicErrorMessage(cause) : publicErrorMessage(cause)
  } finally { if (request === generation) loading.value = false }
}
watch(() => [props.brandId, props.memberId], () => {
  [rows.value,totalCount.value]=[[],'0']; more.value = false;
  draftMember.value = ''; filterMember.value = props.memberId; offset.value = 0
  void withGlobalLoading(load)
}, { immediate: true, flush: 'sync' })

useOperationToast(error, value => displayPublicError(value), 'error');
</script>

<template>
  <ManagementList data-testid="platform-recharges" class="recharge-records">
    <template #filters><DirectorySearch :locale="locale" class="wallet-query" v-model="draftMember" :label="copy.memberPlaceholder" :query-label="ui('members.query')" :busy="loading || !brandId" :query-pending="memberSearchPending" :search="submitMemberQuery">
      <template #input><FilterAssociationSearch :reset-key="filterReset" v-model="draftMember" v-model:pending="memberSearchPending" :brand-id="brandId" surface="platform" resource="recharge" :scope-key="memberId" :label="copy.member" :locale="locale" /></template>
      <template #actions>
      </template>
    </DirectorySearch></template>

    <section class="reward-detail recharge-list"><div class="table-wrap"><table class="console-table console-responsive-table"><thead><tr><th>{{ copy.detail }}</th><th>{{ui("platform.recharge.method")}}</th><th>{{ ui('platform.copy.098b8e97') }}</th><th>{{ copy.points }}</th><th><BusinessDetails :value="copy.state" :locale="props.locale" /></th><th>{{ copy.created }}</th></tr></thead><tbody>
      <tr v-for="order in rows" :key="order.id"><td :data-label="copy.detail"><button class="text-button row-action" @click="detail = order">{{ copy.detail }}</button></td><td :data-label="ui('platform.recharge.method')">{{ui(order.method === "manual" ? "platform.recharge.manual" : "platform.recharge.automatic")}}</td><td :data-label="ui('platform.copy.098b8e97')">{{ order.member_username }}</td><td :data-label="copy.points">{{ order.points }}</td><td :data-label="copy.state">{{ copy[order.state] }}</td><td :data-label="copy.created">{{ formatPlatformDateTime(order.created_at, props.displaySettings, props.locale) }}</td></tr>
      <tr v-if="!loading && !error && !rows.length"><td colspan="6" class="empty-state">{{ copy.empty }}</td></tr>
    </tbody></table></div></section>
    <DetailModal v-if="detail" :title="copy.detail" :close-label="copy.close" @close="detail = null">
<section v-if="detail" class="detail-section" data-testid="platform-recharge-detail">
      <div class="detail-summary"><span class="status-pill" :class="detail.state">{{ copy[detail.state] }}</span></div>
      <dl class="confirm-list reward-fields">
        <div><dt>{{ ui('platform.copy.098b8e97') }}</dt><dd>{{ detail.member_username }}</dd></div><div><dt>{{ ui("platform.recharge.method") }}</dt><dd>{{ ui(detail.method === "manual" ? "platform.recharge.manual" : "platform.recharge.automatic") }}</dd></div><div><dt>{{ copy.points }}</dt><dd>{{ detail.points }}</dd></div><div class="detail-field-wide"><dt>{{ copy.proof }}</dt><dd>{{ detail.proof_reference }}</dd></div><div class="detail-field-wide"><dt>{{ copy.remark }}</dt><dd>{{ detail.remark }}</dd></div><div><dt>{{ copy.creator }}</dt><dd>{{ detail.created_by_name }}</dd></div>
        <div><dt>{{ copy.confirmer }}</dt><dd>{{ detail.confirmed_by_name }}</dd></div><div><dt>{{ copy.confirmedAt }}</dt><dd>{{ formatPlatformDateTime(detail.confirmed_at, props.displaySettings, props.locale) }}</dd></div>
      </dl>

    </section>
</DetailModal>
  <template #pagination><TablePagination class="wallet-pagination" data-testid="platform-recharge-pages" :total="totalCount" :page="Math.floor(offset / pageSize + 1)" :count="rows.length" :locale="locale" :page-size="pageSize" :previous-disabled="loading || offset === 0 || !brandId" :next-disabled="loading || !more" :previous-label="copy.previous" :next-label="copy.next" :go-to="(target: number) => load((target - 1) * pageSize)" :change-page-size="async (size: number) => { pageSize = size; await load(0) }" :busy="loading" /></template></ManagementList>
</template>

<style scoped>
@media (max-width: 620px) {
  .recharge-list th:first-child, .recharge-list td:first-child,
  .recharge-list th:last-child, .recharge-list td:last-child { position: static; }
}
</style>
