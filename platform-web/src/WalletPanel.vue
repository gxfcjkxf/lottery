<script setup lang="ts">
import ManagementTable from "@lottery/shared/ManagementTable.vue"
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { AssociationName } from '@lottery/shared';
import { usePlatformI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import SiteIcon from "@lottery/shared/SiteIcon";
import { vRefreshButton } from '@lottery/shared';
import { vInlineValidation } from '@lottery/shared'
import { usePlatformI18n } from "./i18n"
const ui = usePlatformI18n().t
import DetailModal from "@lottery/shared/DetailModal.vue"
import { computed, ref, watch } from 'vue'
import { WALLET_SOURCES, WALLET_STATES } from '@lottery/shared'
import { PlatformApiError } from './platform-api'
import { createPlatformWalletApi, type Wallet, type LedgerEntry } from './wallet-api'
import RechargesPanel from './RechargesPanel.vue'
import type { PlatformDisplaySettings } from './platform-format'
import { formatPlatformDateTime } from './platform-format'
import ReportsPanel from './ReportsPanel.vue'
import { lookupPlatformMember } from './member-lookup'

const props = defineProps<{ brandId: string; memberId: string; locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings; view: 'wallet' | 'recharges' | 'reports' }>()
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
const error = ref<string | import("@lottery/shared").LocalizedMessage>("")
let generation = 0
const copy = computed(() => props.locale === 'en' ? {
  member: ui('platform.copy.098b8e97'), memberPlaceholder: ui('platform.copy.13a7c871'), load: ui('platform.copy.93a53cd3'), refresh: ui('platform.copy.256fe8b3'), available: ui('platform.copy.529adfaa'), frozen: ui('platform.copy.4958de92'), withdrawal: ui('platform.copy.5e6ce513'), display: ui('platform.copy.bea6e46d'), version: ui('platform.copy.59405491'), source: ui('platform.copy.4fe341a5'), recharge: ui('platform.copy.3420a04d'), winning: ui('platform.copy.57e492c6'), gift: ui('platform.copy.c8d77732'), commission: ui('platform.copy.648ed708'), manual_frozen: ui('platform.copy.2ce7e185'), system_frozen: ui('platform.copy.1659fc95'), ledger: ui('platform.copy.3c5a4352'), entry: ui('platform.copy.a7b66898'), type: ui('platform.copy.e34df49d'), reason: ui('platform.copy.f3f300a4'), date: ui('platform.copy.7069c6b9'), previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), detail: ui('platform.copy.57d101e7'), close: ui('platform.copy.27b2fd29'), before: ui('platform.copy.e8d73caa'), delta: ui('platform.copy.f5fae35f'), after: ui('platform.copy.2be93f0a'), state: ui('platform.copy.274ae047'), empty: ui('platform.copy.9eaaa95f'),
} : {
  member: ui('platform.copy.098b8e97'), memberPlaceholder: ui('platform.copy.13a7c871'), load: ui('platform.copy.93a53cd3'), refresh: ui('platform.copy.256fe8b3'), available: ui('platform.copy.529adfaa'), frozen: ui('platform.copy.4958de92'), withdrawal: ui('platform.copy.5e6ce513'), display: ui('platform.copy.bea6e46d'), version: ui('platform.copy.59405491'), source: ui('platform.copy.4fe341a5'), recharge: ui('platform.copy.3420a04d'), winning: ui('platform.copy.57e492c6'), gift: ui('platform.copy.c8d77732'), commission: ui('platform.copy.648ed708'), manual_frozen: ui('platform.copy.2ce7e185'), system_frozen: ui('platform.copy.1659fc95'), ledger: ui('platform.copy.3c5a4352'), entry: ui('platform.copy.a7b66898'), type: ui('platform.copy.e34df49d'), reason: ui('platform.copy.f3f300a4'), date: ui('platform.copy.7069c6b9'), previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), detail: ui('platform.copy.57d101e7'), close: ui('platform.copy.27b2fd29'), before: ui('platform.copy.e8d73caa'), delta: ui('platform.copy.f5fae35f'), after: ui('platform.copy.2be93f0a'), state: ui('platform.copy.274ae047'), empty: ui('platform.copy.9eaaa95f'),
})
function clear() {
  generation++
  wallet.value = null; entries.value = []; selected.value = null
  offset.value = 0; more.value = false; activeMember.value = ''; loading.value = false; error.value = ''
}
const financeMember = computed(() => activeMember.value || props.memberId)
function report(cause: unknown) {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  else error.value = cause instanceof Error ? publicErrorMessage(cause) : publicErrorMessage(cause)
}
async function load() {
  clear()
  const request = generation
  const brandId = props.brandId
  const username = inputMember.value.trim()
  let memberId = ''
  loading.value = true
  try {
    memberId = (await lookupPlatformMember(brandId, username)).id
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
  clear(); inputMember.value = ''
}, { immediate: true, flush: 'sync' })
</script>

<template>
    <RechargesPanel v-if="view === 'recharges'" :brand-id="brandId" :member-id="financeMember" :locale="locale" :display-settings="displaySettings" @failure="emit('failure', $event)" />
  <ReportsPanel v-else-if="view === 'reports'" :brand-id="brandId" :member-id="financeMember" :locale="locale" :display-settings="displaySettings" @failure="emit('failure', $event)" />
  <template v-else>
  <div data-testid="platform-wallet">
    <form novalidate v-inline-validation class="panel wallet-query" @submit.prevent="withGlobalLoading(load)">
      <label class="query-field">
        <div class="input-with-icon">
          <span class="input-icon"> <SiteIcon name="user" /> </span>
          <input v-model="inputMember" required :disabled="loading || !brandId" autocomplete="off" :aria-label="copy.member" :placeholder="copy.memberPlaceholder" />
        </div>
      </label>
      <button class="button button-primary" :disabled="loading || !brandId">{{ copy.load }}</button>
    </form>
    <p v-if="error" class="message error" role="alert">{{ displayPublicError(error) }}</p>
    <template v-if="wallet">
      <section class="panel" data-testid="platform-wallet-balances">
        <div class="panel-heading"><h2><AssociationName :record="wallet" kind="member" :id="activeMember" :locale="locale" /></h2></div>
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
        <div class="panel-heading"><h2>{{ copy.ledger }}</h2></div>
        <ManagementTable :entries="entries" :columns="[{key:'entry',label:copy.entry},{key:'type',label:copy.type},{key:'reason',label:copy.reason},{key:'date',label:copy.date}]" :empty="!entries.length" :empty-label="copy.empty">
          <template #entry="{entry}"><button class="text-button" @click="selected=entry">{{entry.id}}</button></template><template #type="{entry}">{{entry.entry_type}}</template><template #reason="{entry}">{{entry.reason}}</template><template #date="{entry}">{{formatPlatformDateTime(entry.created_at, props.displaySettings, props.locale)}}</template>
        </ManagementTable>
        <div class="wallet-pagination" data-testid="platform-wallet-pages"><button class="button button-secondary" :disabled="loading || offset === 0" @click="withGlobalLoading(page, offset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(offset / 50) + 1 }}</span><button class="button button-secondary" :disabled="loading || !more" @click="withGlobalLoading(page, offset + 50)">{{ copy.next }}</button><button v-refresh-button="copy.refresh" class="button button-secondary" :disabled="loading" @click="withGlobalLoading(load)">{{ copy.refresh }}</button></div>
      </section>
      <DetailModal v-if="selected" :title="copy.detail" :close-label="copy.close" @close="selected = null">
<section v-if="selected" class="detail-section" data-testid="platform-wallet-entry">
        <div class="detail-summary"><p class="mono">{{ selected.id }}</p><p>{{ selected.reason }}</p></div>
        <div class="table-wrap"><table><thead><tr><th>{{ copy.source }}</th><th><BusinessDetails :value="copy.state" :locale="props.locale" /></th><th>{{ copy.before }}</th><th>{{ copy.delta }}</th><th>{{ copy.after }}</th></tr></thead><tbody>
          <template v-for="source in WALLET_SOURCES" :key="source"><tr v-for="state in WALLET_STATES" :key="state"><td>{{ copy[source] }}</td><td>{{ copy[state] }}</td><td>{{ selected.before_snapshot[source][state] }}</td><td>{{ selected.delta_snapshot[source][state] }}</td><td>{{ selected.after_snapshot[source][state] }}</td></tr></template>
        </tbody></table></div>
      </section>
</DetailModal>
    </template>
  </div>
  </template>
</template>
