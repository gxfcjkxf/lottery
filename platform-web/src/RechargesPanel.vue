<script setup lang="ts">
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { usePlatformI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import SiteIcon from "@lottery/shared/SiteIcon";
import { vInlineValidation } from '@lottery/shared'
import { usePlatformI18n } from "./i18n"
const ui = usePlatformI18n().t
import DetailModal from "@lottery/shared/DetailModal.vue"
import { computed, ref, watch } from 'vue'
import { PlatformApiError } from './platform-api'
import { createPlatformRechargeApi, type Recharge } from './recharge-api'
import { lookupPlatformMember } from './member-lookup'
import { formatPlatformDateTime, type PlatformDisplaySettings } from './platform-format'

const props = defineProps<{ brandId: string; memberId: string; locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformRechargeApi()
const draftMember = ref('')
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
async function submitMemberQuery() {
  const username = draftMember.value.trim()
  try {
    await load(0, username ? (await lookupPlatformMember(props.brandId, username)).id : '')
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
    else error.value = cause instanceof Error ? publicErrorMessage(cause) : publicErrorMessage(cause)
  } finally { if (request === generation) loading.value = false }
}
watch(() => [props.brandId, props.memberId], () => {
  draftMember.value = ''; filterMember.value = props.memberId; offset.value = 0
  void withGlobalLoading(load)
}, { immediate: true, flush: 'sync' })
</script>

<template>
  <div data-testid="platform-recharges">
    <form novalidate v-inline-validation class="panel wallet-query" @submit.prevent="withGlobalLoading(submitMemberQuery)">
      <label class="query-field">
        <div class="input-with-icon">
          <span class="input-icon"> <SiteIcon name="user" /> </span>
          <input v-model="draftMember" :disabled="loading || !brandId" autocomplete="off" :aria-label="copy.member" :placeholder="copy.memberPlaceholder" />
        </div>
      </label>
      <button class="button button-primary" :disabled="loading || !brandId">{{ copy.query }}</button>
    </form>
    <p v-if="error" class="message error" role="alert">{{ displayPublicError(error) }}</p>
    <section class="panel reward-detail recharge-list"><div class="table-wrap"><table><thead><tr><th>{{ copy.order }}</th><th>{{ ui('platform.copy.098b8e97') }}</th><th>{{ copy.points }}</th><th><BusinessDetails :value="copy.state" :locale="props.locale" /></th><th>{{ copy.created }}</th></tr></thead><tbody>
      <tr v-for="order in rows" :key="order.id"><td><button class="row-action" @click="detail = order">{{ order.id }}</button></td><td>{{ order.member_username }}</td><td>{{ order.points }}</td><td>{{ copy[order.state] }}</td><td>{{ formatPlatformDateTime(order.created_at, props.displaySettings, props.locale) }}</td></tr>
      <tr v-if="!rows.length"><td colspan="5" class="empty-state">{{ copy.empty }}</td></tr>
    </tbody></table></div><div class="wallet-pagination" data-testid="platform-recharge-pages"><button class="button button-secondary" :disabled="loading || offset === 0 || !brandId" @click="withGlobalLoading(load, offset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(offset / 50) + 1 }}</span><button class="button button-secondary" :disabled="loading || !more" @click="withGlobalLoading(load, offset + 50)">{{ copy.next }}</button></div></section>
    <DetailModal v-if="detail" :title="copy.detail" :close-label="copy.close" @close="detail = null">
<section v-if="detail" class="detail-section" data-testid="platform-recharge-detail">
      <div class="detail-summary"><span class="status-pill" :class="detail.state">{{ copy[detail.state] }}</span></div>
      <dl class="confirm-list reward-fields">
        <div class="detail-field-wide"><dt>{{ copy.order }}</dt><dd class="mono break-value">{{ detail.id }}</dd></div><div><dt>{{ ui('platform.copy.098b8e97') }}</dt><dd>{{ detail.member_username }}</dd></div><div><dt>{{ copy.points }}</dt><dd>{{ detail.points }}</dd></div><div class="detail-field-wide"><dt>{{ copy.proof }}</dt><dd>{{ detail.proof_reference }}</dd></div><div class="detail-field-wide"><dt>{{ copy.remark }}</dt><dd>{{ detail.remark }}</dd></div><div><dt>{{ copy.creator }}</dt><dd>{{ detail.created_by_name }}</dd></div>
        <div><dt>{{ copy.confirmer }}</dt><dd>{{ detail.confirmed_by_name }}</dd></div><div><dt>{{ copy.confirmedAt }}</dt><dd>{{ formatPlatformDateTime(detail.confirmed_at, props.displaySettings, props.locale) }}</dd></div>
      </dl>
      <details class="detail-technical"><summary>{{ ui('platform.detail.technical') }}</summary><dl class="confirm-list reward-fields"><div class="detail-field-wide"><dt>{{ copy.ledger }}</dt><dd class="mono break-value">{{ detail.ledger_entry_id || '—' }}</dd></div></dl></details>
    </section>
</DetailModal>
  </div>
</template>

<style scoped>
@media (max-width: 620px) {
  .recharge-list th:first-child, .recharge-list td:first-child,
  .recharge-list th:last-child, .recharge-list td:last-child { position: static; }
}
</style>
