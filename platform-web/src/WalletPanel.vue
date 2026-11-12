<script setup lang="ts">
import CashMemberBalances from "../../shared/src/CashMemberBalances.vue"
import ManagementList from "@lottery/shared/ManagementList.vue"
import { auditReasonLabel, readPaginationTotal } from '@lottery/shared'
const ledgerTotal = ref('0')

import TablePagination from "@lottery/shared/TablePagination.vue"
import { useOperationToast } from "@lottery/shared";
import ManagementTable from "@lottery/shared/ManagementTable.vue"
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { usePlatformI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { usePlatformI18n } from "./i18n"
const ui = usePlatformI18n().t
import DetailModal from "@lottery/shared/DetailModal.vue"
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { WALLET_SOURCES, WALLET_STATES, pointsMinor, pointsFromMinor, formatPoints, type SourceBuckets } from '@lottery/shared'
import { PlatformApiError } from './platform-api'
import { createPlatformWalletApi, type Wallet, type LedgerEntry } from './wallet-api'
import type { PlatformDisplaySettings } from './platform-format'
import { formatPlatformDateTime } from './platform-format'
const pageSize = ref(50)


const props = defineProps<{ brandId: string; memberId: string; memberName: string; locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings; view: 'wallet' | 'ledger' }>()
const emit = defineEmits<{ failure: [cause: unknown]; close: [] }>()
const api = createPlatformWalletApi()
function closeMember() { clear(); emit('close') }
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
const ledgerColumns = computed(() => [
  { key: 'entry', label: copy.value.entry },
  { key: 'type', label: copy.value.type },
  { key: 'before', label: ui('platform.ledger.availableBefore'), cellClass: 'ledger-amount' },
  { key: 'delta', label: ui('platform.ledger.availableChange'), cellClass: 'ledger-amount' },
  { key: 'after', label: ui('platform.ledger.availableAfter'), cellClass: 'ledger-amount' },
  { key: 'reason', label: copy.value.reason },
  { key: 'date', label: copy.value.date },
])
function availableTotal(snapshot: SourceBuckets): string {
  return pointsFromMinor(WALLET_SOURCES.reduce((total, source) => total + pointsMinor(snapshot[source].available), 0n))
}
function signedPoints(value: string): string {
  return `${pointsMinor(value) > 0n ? '+' : ''}${formatPoints(value, props.locale)}`
}
const changedBalances = computed(() => selected.value ? WALLET_SOURCES.flatMap(source =>
  WALLET_STATES.filter(state => pointsMinor(selected.value!.delta_snapshot[source][state]) !== 0n)
    .map(state => ({ source, state })),
) : [])
function clear() {
  generation++
  wallet.value = null; [entries.value,ledgerTotal.value]=[[],'0']; selected.value = null
  offset.value = 0; more.value = false; activeMember.value = ''; loading.value = false; error.value = ''
}
function report(cause: unknown) {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  else error.value = cause instanceof Error ? publicErrorMessage(cause) : publicErrorMessage(cause)
}
async function load(memberId: string) {
  clear()
  const request = generation
  const brandId = props.brandId
  activeMember.value = memberId
  loading.value = true
  try {
    if (props.view === 'wallet') {
      const current = await api.wallet(brandId, memberId)
      if (request !== generation || brandId !== props.brandId) return
      wallet.value = current
    } else {
      const rows = await api.ledger(brandId, memberId, pageSize.value, 0)
      if (request !== generation || brandId !== props.brandId) return
      [entries.value, ledgerTotal.value] = [rows.items, readPaginationTotal(rows)]; more.value = BigInt(rows.items.length) < BigInt(ledgerTotal.value)
    }
  } catch (cause) {
    if (request === generation) report(cause)
  } finally { if (request === generation) loading.value = false }
}
async function page(nextOffset: number) {
  const request = ++generation
  const brandId = props.brandId
  const memberId = activeMember.value
  selected.value = null; error.value = ''; loading.value = true
  try {
    const rows = await api.ledger(brandId, memberId, pageSize.value, nextOffset)
    if (request !== generation || brandId !== props.brandId) return
    [entries.value, ledgerTotal.value] = [rows.items, readPaginationTotal(rows)]; more.value = BigInt(nextOffset + rows.items.length) < BigInt(ledgerTotal.value); offset.value = nextOffset
  } catch (cause) { if (request === generation) report(cause) }
  finally { if (request === generation) loading.value = false }
}
watch(() => [props.brandId, props.memberId, props.view], () => {
  clear()
  if (props.brandId && props.memberId) void withGlobalLoading(load, props.memberId)
}, { immediate: true })
onBeforeUnmount(() => { generation++ })

useOperationToast(error, value => displayPublicError(value), 'error');
</script>

<template>
  <div data-testid="platform-wallet">
    <DetailModal :title="`${view === 'wallet' ? ui('common.memberPoints') : ui('members.ledger')} · ${memberName}`" :close-label="copy.close" @close="closeMember">
      <button v-refresh-button="copy.refresh" v-if="error" type="button" class="button button-secondary" :disabled="loading" @click="withGlobalLoading(load, activeMember)">{{copy.refresh}}</button>
    <template v-if="view === 'wallet' && wallet">
<CashMemberBalances base-path="/api/v1/platform" :brand-id="brandId" :member-id="activeMember" :locale="locale" />
      <section class="detail-section" data-testid="platform-wallet-balances">
        <div class="panel-heading"><h2>{{ ui('common.points') }}</h2><button v-if="!error" v-refresh-button="copy.refresh" type="button" class="button button-secondary" :disabled="loading" @click="withGlobalLoading(load, activeMember)">{{ copy.refresh }}</button></div>
        <dl class="confirm-list reward-fields">
          <div><dt>{{ copy.display }}</dt><dd>{{ wallet.display_points }}</dd></div>
          <div><dt>{{ copy.available }}</dt><dd>{{ wallet.available_points }}</dd></div>
          <div><dt>{{ copy.frozen }}</dt><dd>{{ wallet.frozen_points }}</dd></div>
          <div><dt>{{ copy.withdrawal }}</dt><dd>{{ wallet.withdrawal_points }}</dd></div>
        </dl>
        <div class="table-wrap"><table class="console-table console-responsive-table wallet-source-table"><thead><tr><th>{{ copy.source }}</th><th v-for="state in WALLET_STATES" :key="state">{{ copy[state] }}</th></tr></thead><tbody>
          <tr v-for="source in WALLET_SOURCES" :key="source"><td :data-label="String(copy.source)">{{ copy[source] }}</td><td :data-label="String(copy[state])" v-for="state in WALLET_STATES" :key="state">{{ wallet.by_source[source][state] }}</td></tr>
        </tbody></table></div>
      </section>
    </template>
    <template v-if="view === 'ledger'">
      <ManagementList embedded class="detail-section" data-testid="platform-wallet-ledger">
        <template #heading><h2>{{ copy.ledger }}</h2></template>
        <ManagementTable :entries="entries" :columns="ledgerColumns" table-class="wallet-ledger-table" :empty="!entries.length" :empty-label="copy.empty">
          <template #entry="{entry}"><button class="text-button" @click="selected=entry">{{copy.detail}}</button></template><template #type="{entry}"><BusinessDetails :value="entry.entry_type" :locale="locale" /></template><template #before="{entry}">{{ formatPoints(availableTotal(entry.before_snapshot), locale) }}</template>
          <template #delta="{entry}"><strong :class="{ 'ledger-increase': pointsMinor(availableTotal(entry.delta_snapshot)) > 0n, 'ledger-decrease': pointsMinor(availableTotal(entry.delta_snapshot)) < 0n }">{{ signedPoints(availableTotal(entry.delta_snapshot)) }}</strong></template>
          <template #after="{entry}">{{ formatPoints(availableTotal(entry.after_snapshot), locale) }}</template>
          <template #reason="{entry}">{{auditReasonLabel(entry.reason, locale)}}</template><template #date="{entry}">{{formatPlatformDateTime(entry.created_at, props.displaySettings, props.locale)}}</template>
        </ManagementTable>

      <template #pagination><TablePagination class="wallet-pagination" data-testid="platform-wallet-pages" :total="ledgerTotal" :page="Math.floor(offset / pageSize + 1)" :count="entries.length" :locale="locale" :page-size="pageSize" :previous-disabled="loading || offset === 0" :next-disabled="loading || !more" :previous-label="copy.previous" :next-label="copy.next" :go-to="(target: number) => page((target - 1) * pageSize)" :change-page-size="async (size: number) => { pageSize = size; await page(0) }" :busy="loading" /></template></ManagementList>
      <DetailModal v-if="selected" :title="copy.detail" :close-label="copy.close" @close="selected = null">
<section v-if="selected" class="detail-section" data-testid="platform-wallet-entry">
        <div class="detail-summary"><p>{{ auditReasonLabel(selected.reason, locale) }}</p></div>
        <div class="table-wrap"><table class="console-table console-responsive-table"><thead><tr><th>{{ copy.source }}</th><th><BusinessDetails :value="copy.state" :locale="props.locale" /></th><th>{{ copy.before }}</th><th>{{ copy.delta }}</th><th>{{ copy.after }}</th></tr></thead><tbody>
          <tr v-for="{ source, state } in changedBalances" :key="`${source}-${state}`"><td :data-label="String(copy.source)">{{ copy[source] }}</td><td :data-label="copy.state">{{ copy[state] }}</td><td :data-label="String(copy.before)">{{ formatPoints(selected.before_snapshot[source][state], locale) }}</td><td :data-label="String(copy.delta)">{{ signedPoints(selected.delta_snapshot[source][state]) }}</td><td :data-label="String(copy.after)">{{ formatPoints(selected.after_snapshot[source][state], locale) }}</td></tr>
        </tbody></table></div>
      </section>
</DetailModal>
    </template>
    </DetailModal>
  </div>
</template>

<style scoped>
:deep(.wallet-ledger-table) { table-layout: fixed; width: 100%; }
:deep(.wallet-ledger-table :is(th, td)) { white-space: normal; overflow-wrap: anywhere; }
:deep(.wallet-ledger-table th:nth-child(n+3):nth-child(-n+5)),
:deep(.wallet-ledger-table .ledger-amount) { text-align: right; font-variant-numeric: tabular-nums; }
.ledger-increase { color: var(--success-text); }
.ledger-decrease { color: var(--danger-text); }
@media (max-width: 700px) {
  :deep(.wallet-ledger-table .ledger-amount) { text-align: left; }
}
.wallet-source-table { table-layout: fixed; white-space: normal; }
.wallet-source-table th, .wallet-source-table td { overflow-wrap: anywhere; white-space: normal; }
@media (max-width: 700px) {
  .wallet-source-table th, .wallet-source-table td { padding: 6px 4px; }
}
</style>
