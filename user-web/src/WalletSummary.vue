<script setup lang="ts">
import { pointsMinor, pointsFromMinor } from '@lottery/shared'

import { readPaginationTotal } from '@lottery/shared'
const ledgerTotal = ref('0')

import TablePagination from "@lottery/shared/TablePagination.vue"
import { useUserI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
import { formatDisplayDateTime, formatDisplayMonthDayTime } from "@lottery/shared";
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useUserI18n } from './i18n';
import {
  createWalletApi,
  WALLET_SNAPSHOT_EVENT,
  formatIntegerAmount,
  WalletApiError,
  type LedgerEntry,
  type Wallet,
  type WalletSource,
  type WalletState,
} from "./wallet-api";

const props = withDefaults(
  defineProps<{ brandCode?: string; locale?: "en" | "zh" | "zh-CN"; view?: 'overview' | 'buckets' | 'ledger' }>(),
  { locale: "en", view: 'overview' },
);
const ui = useUserI18n().t;
const copy = computed(() => ({
  title: ui('user.wallet.title'), refresh: ui('user.wallet.refresh'), loading: ui('user.wallet.loading'), reading: ui('user.wallet.reading'), unauthenticated: ui('user.wallet.unauthenticated'), recheck: ui('user.wallet.recheck'), retry: ui('user.wallet.retry'), balance: ui('user.wallet.balance'), points: ui('user.wallet.points'), frozen: ui('user.wallet.frozen'), withdrawal: ui('user.wallet.withdrawal'), buckets: ui('user.wallet.buckets'), ledger: ui('user.wallet.ledger'), empty: ui('user.wallet.empty'), page: (number: number) => ui('user.wallet.page', { number }), previous: ui('user.wallet.previous'), next: ui('user.wallet.next'), sources: { recharge: ui('user.wallet.source.recharge'), winning: ui('user.wallet.source.winning'), gift: ui('user.wallet.source.gift'), commission: ui('user.wallet.source.commission') }, states: { available: ui('user.wallet.state.available'), manual_frozen: ui('user.wallet.state.manualFrozen'), system_frozen: ui('user.wallet.state.systemFrozen'), withdrawal: ui('user.wallet.state.withdrawal') }, networkError: ui('user.wallet.networkError'), brandMismatch: ui('user.wallet.brandMismatch'),
}));
function ledgerLabel(entry:LedgerEntry):string{
 if(entry.entry_type==="currency_exchange_credit")return ui("cash.to_points");
 if(entry.entry_type==="currency_exchange_debit")return ui("cash.from_points");
 if(entry.reference_type==="canada_order"){
 const frozenDelta=Object.values(entry.delta_snapshot).reduce((sum,b)=>sum+pointsMinor(b.system_frozen),0n);
 if(entry.entry_type==='bet'&&frozenDelta>0n)return ui('user.wallet.canada.reserve');
 if(entry.entry_type==='bet'&&frozenDelta<0n)return ui('user.wallet.canada.loss');
 if(entry.entry_type==='refund'&&frozenDelta<0n)return ui('user.wallet.canada.release');
 switch(entry.entry_type){case "bet":return ui('user.wallet.canada.bet');case "prize":return ui('user.wallet.canada.prize');case "refund":return ui('user.wallet.canada.refund');default:throw new Error('Unsupported Canada ledger entry')}
 }
 if(entry.reference_type!=="mark_six_order") {
   return ui(`user.wallet.entry.${entry.entry_type}`);
 }
 switch(entry.entry_type){
 case "bet":return ui('user.wallet.markSix.bet');
 case "prize":return ui('user.wallet.markSix.prize');
 case "refund":return ui('user.wallet.markSix.refund');
 default:throw new Error('Unsupported Mark Six ledger entry');
 }
}
function availableChange(entry: LedgerEntry): string {
  const change = sources.reduce((sum, source) => sum + pointsMinor(entry.delta_snapshot[source].available), 0n);
  return `${change > 0n ? '+' : ''}${formatIntegerAmount(pointsFromMinor(change))}`;
}
function availableAfter(entry: LedgerEntry): string {
  return formatIntegerAmount(pointsFromMinor(sources.reduce((total, source) => total + pointsMinor(entry.after_snapshot[source].available), 0n)));
}
const PAGE_SIZE = ref(50);
const sources: WalletSource[] = ["recharge", "winning", "gift", "commission"];
const states: WalletState[] = [
  "available",
  "manual_frozen",
  "system_frozen",
  "withdrawal",
];
const wallet = ref<Wallet | null>(null);
const entries = ref<LedgerEntry[]>([]);
const state = ref<"loading" | "ready" | "unauthenticated" | "error">("loading");
const error = ref<string | import("@lottery/shared").LocalizedMessage>("");
const offset = ref(0);
const loading = ref(false);
const api = computed(() => createWalletApi({ brandCode: props.brandCode }));
let loadGeneration = 0;
const bucketRows = computed(() =>
  wallet.value
    ? sources.map((source) => ({
        source,
        values: wallet.value!.by_source[source],
      }))
    : [],
);

async function load(requestedOffset = offset.value) {
  const requestedBrandCode = props.brandCode;
  const generation = ++loadGeneration;
  const isCurrent = () =>
    generation === loadGeneration && props.brandCode === requestedBrandCode;
  loading.value = true;
  if (!wallet.value) state.value = "loading";
  error.value = "";
  try {
    const [currentWallet, ledger] = await Promise.all([
      api.value.wallet(),
      props.view === 'ledger' ? api.value.ledger(PAGE_SIZE.value, requestedOffset) : Promise.resolve(null),
    ]);
    if (!isCurrent()) return;
    if (
      ledger?.items.some(
        (entry) =>
          entry.brand_id !== currentWallet.brand_id ||
          entry.member_id !== currentWallet.member_id,
      )
    ) {
      throw new WalletApiError(
        copy.value.brandMismatch,
        0,
        "brand_mismatch",
      );
    }
    if (!wallet.value || wallet.value.version <= currentWallet.version) wallet.value = currentWallet;
    if (ledger) { [entries.value, ledgerTotal.value] = [ledger.items, readPaginationTotal(ledger)]; }
    offset.value = requestedOffset; state.value = "ready";
  } catch (cause) {
    if (!isCurrent()) return;
    if (cause instanceof WalletApiError && cause.status === 401) {
      wallet.value = null; [entries.value, ledgerTotal.value] = [[], '0'];
      state.value = "unauthenticated";
      error.value = copy.value.unauthenticated;
    } else {
      if (cause instanceof WalletApiError && cause.code === "brand_mismatch") {wallet.value=null;entries.value=[];ledgerTotal.value='0'}
      state.value = wallet.value ? "ready" : "error";
      error.value =
        cause instanceof WalletApiError &&
        cause.status === 0 &&
        cause.code !== "brand_mismatch"
          ? copy.value.networkError
          : cause instanceof Error
            ? publicErrorMessage(cause, 'read')
            : copy.value.networkError;
    }
  } finally {
    if (isCurrent()) loading.value = false;
  }
}

function walletUpdated(event: Event) {
  const next = (event as CustomEvent<Wallet>).detail;
  const current = wallet.value;
  if (current && next.brand_id === current.brand_id && next.member_id === current.member_id && next.version >= current.version) wallet.value = next;
}
onMounted(() => { window.addEventListener(WALLET_SNAPSHOT_EVENT, walletUpdated); void load(); });
onUnmounted(() => { window.removeEventListener(WALLET_SNAPSHOT_EVENT, walletUpdated); loadGeneration++; });
watch(
  () => [props.brandCode, props.view],
  () => {
    loadGeneration++;
    wallet.value = null;
    [entries.value,ledgerTotal.value]=[[],'0'];
    offset.value = 0;
    void load();
  },
);
async function changePage(direction: number) {
  await load(Math.max(0, offset.value + direction * PAGE_SIZE.value));
}
</script>

<template>
  <section class="wallet-summary">

      <Teleport defer to="#user-page-refresh"><button v-if="error && wallet" v-refresh-button="loading ? copy.loading : copy.refresh" class="refresh" type="button" :disabled="loading" @click="withGlobalLoading(load)">
        {{ loading ? copy.loading : copy.refresh }}
      </button></Teleport>

    <div v-global-loading v-if="state === 'loading' && !wallet" class="state-card" role="status">
      {{ copy.reading }}
    </div>
    <div
      v-else-if="state === 'unauthenticated'"
      class="state-card"
      role="status"
    >
      {{ displayPublicError(error) }}
      <button type="button" class="text-button" @click="withGlobalLoading(load)">
        {{ copy.recheck }}
      </button>
    </div>
    <div
      v-else-if="state === 'error' && !wallet"
      class="state-card state-error"
      role="alert"
    >
      <span>{{ displayPublicError(error) }}</span>
      <button type="button" class="text-button" @click="withGlobalLoading(load)">
        {{ copy.retry }}
      </button>
    </div>
    <template v-else-if="wallet">
      <p v-if="error" class="state-error" role="alert">{{ displayPublicError(error) }}</p>
      <div v-if="props.view === 'overview'" class="totals">
        <article class="total-card total-primary">
          <div class="balance-card-head">
            <span>{{ copy.balance }}</span>
            <div class="balance-amount"><strong>{{ formatIntegerAmount(wallet.display_points) }}</strong><span>{{ copy.points }}</span></div>

          </div>
          <div class="wallet-available">
            <span>{{ ui('user.wallet.availableBalance') }}</span>
            <b>{{ formatIntegerAmount(wallet.available_points) }} <small>{{ copy.points }}</small></b>
          </div>
        </article>
        <article class="total-card key-value-row">
          <span class="total-label key-value-label">{{ copy.frozen }}</span>
          <div class="metric-amount key-value-value"><strong>{{ formatIntegerAmount(wallet.frozen_points) }}</strong><span>{{ copy.points }}</span></div>
        </article>
        <article class="total-card key-value-row">
          <span class="total-label key-value-label">{{ copy.withdrawal }}</span>
          <div class="metric-amount key-value-value"><strong>{{ formatIntegerAmount(wallet.withdrawal_points) }}</strong><span>{{ copy.points }}</span></div>
        </article>
      </div>



      <section v-if="props.view === 'buckets'" class="panel">
        <div class="bucket-grid">
          <article
            v-for="row in bucketRows"
            :key="row.source"
            class="bucket-source"
          >
            <header class="source-head">
              <h3>{{ copy.sources[row.source] }}</h3>
            </header>
            <dl>
              <div v-for="bucketState in states" :key="bucketState" :class="{ 'bucket-available': bucketState === 'available' }">
                <dt>{{ copy.states[bucketState] }}</dt>
                <dd>{{ formatIntegerAmount(row.values[bucketState]) }}</dd>
              </div>
            </dl>
          </article>
        </div>
      </section>

      <section v-if="props.view === 'ledger'" class="panel ledger-panel user-record-list" :aria-busy="loading">
        <p v-if="!entries.length" class="empty empty-state">{{ copy.empty }}</p>
        <table v-if="entries.length" class="ledger-table">
          <thead><tr>
            <th scope="col">{{ ui('user.wallet.entryType') }}</th>
            <th scope="col" class="ledger-amount">{{ ui('user.wallet.availableChange') }}</th>
            <th scope="col" class="ledger-amount">{{ ui('user.wallet.availableAfter') }}</th>
            <th scope="col">{{ ui('user.wallet.entryTime') }}</th>
          </tr></thead>
          <tbody>
            <tr v-for="entry in entries" :key="entry.id" class="ledger-entry">
              <td class="ledger-type">
                <div class="ledger-heading"><b>{{ ledgerLabel(entry) }}</b><time class="ledger-mobile-time" :datetime="entry.created_at">{{ formatDisplayMonthDayTime(entry.created_at, props.locale) }}</time></div>
                <dl v-if="entry.agent_transfer" class="ledger-result agent-transfer-receipt"><div><dt>{{ ui('user.wallet.agentTransferFrom') }}</dt><dd>{{ entry.agent_transfer.agent_account ?? '—' }}</dd></div></dl>
              </td>
              <td class="ledger-amount" :data-label="ui('user.wallet.availableChange')"><strong :class="{ 'change-negative': availableChange(entry).startsWith('−'), 'change-positive': availableChange(entry).startsWith('+') }">{{ availableChange(entry) }}</strong></td>
              <td class="ledger-amount ledger-balance-after" :data-label="ui('user.wallet.availableAfter')"><span>{{ availableAfter(entry) }}</span></td>
              <td class="ledger-time">{{ formatDisplayDateTime(entry.created_at, props.locale) }}</td>
            </tr>
          </tbody>
        </table>
        <TablePagination class="pager" :total="ledgerTotal" :page="Math.floor(offset / PAGE_SIZE + 1)" :count="entries.length" :locale="publicErrorI18n.locale.value" :page-size="PAGE_SIZE" :previous-disabled="offset === 0 || loading" :next-disabled="BigInt(offset + entries.length) >= BigInt(ledgerTotal) || loading" :previous-label="copy.previous" :next-label="copy.next" :go-to="(target: number) => changePage(target - (Math.floor(offset / PAGE_SIZE + 1)))" :change-page-size="async (size: number) => { PAGE_SIZE = size; await load(0) }" :busy="loading" />
      </section>
    </template>
  </section>
</template>

<style scoped>
.wallet-summary { width: 100%; margin: 0 auto; min-width: 0; color: var(--ink); }
.wallet-head { display: flex; flex-direction: row; justify-content: space-between; align-items: center; gap: 12px; margin: 0 0 12px; }
.wallet-head h1 { font-size: 24px; margin: 0; }
.totals { display: grid; grid-template-columns: minmax(0, 1.6fr) repeat(2, minmax(0, 1fr)); gap: 12px; }
.total-card, .panel, .state-card { min-width: 0; background: var(--surface-card); backdrop-filter: blur(14px); -webkit-backdrop-filter: blur(14px); border: 1px solid var(--line); border-radius: var(--radius); padding: 16px; box-shadow: var(--shadow-sm); }
.total-card { display: flex; flex-direction: column; align-items: flex-start; gap: 12px; }
.total-label { font-size: 14px; font-weight: 600; color: var(--ink-secondary); }
.total-card.key-value-row { grid-template-columns: fit-content(60%) minmax(0, 1fr) !important; }
.balance-amount, .metric-amount { display: flex; flex-wrap: wrap; align-items: baseline; gap: 8px; min-width: 0; font-variant-numeric: tabular-nums; }
.balance-amount strong { font-size: 36px; line-height: 1.2; overflow-wrap: anywhere; min-width: 0; }
.metric-amount strong { font-size: 28px; line-height: 1.2; overflow-wrap: anywhere; min-width: 0; }
.balance-amount > span, .metric-amount > span { font-size: var(--body-font-size); }
.metric-amount > span { color: var(--muted); }
.total-primary { background: var(--primary-light); border-color: var(--primary-ring); color: var(--ink); gap: 10px; box-shadow: var(--shadow); }
.balance-card-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; width: 100%; font-size: 14px; font-weight: 600; }
.balance-card-head { flex-wrap: wrap; }
.balance-card-head .balance-amount { flex: 1 1 8ch; justify-content: flex-end; }
.key-value-value.metric-amount { justify-content: flex-end; }
.wallet-available { display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: 8px; width: 100%; margin-top: 4px; padding-top: 12px; border-top: 1px solid var(--line); font-size: var(--body-font-size); }
.wallet-available b { min-width: 0; font-size: 20px; overflow-wrap: anywhere; font-variant-numeric: tabular-nums; }
.wallet-available small { font-size: var(--body-font-size); font-weight: 400; }
.panel { margin-top: 20px; }
.bucket-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.bucket-source { min-width: 0; padding: 16px; background: var(--canvas); border: 1px solid var(--line); border-radius: var(--radius-sm); }
.source-head { margin-bottom: 12px; }
.source-head h3 { margin: 0; font-size: 15px; }
.bucket-source dl { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px 16px; margin: 0; }
.bucket-source dl > div { min-width: 0; }
.bucket-source dt { font-size: var(--body-font-size); color: var(--muted); }
.bucket-source dd { margin: 0; font-size: 16px; font-weight: 650; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.bucket-available dd { color: var(--success-text); }
.ledger-table { width: 100%; border-collapse: collapse; font-size: var(--body-font-size); }
.ledger-table th, .ledger-table td { position: static; padding: 8px var(--table-cell-padding-inline); text-align: left; vertical-align: middle; border-bottom: 1px solid var(--line); overflow-wrap: anywhere; }
.ledger-table .ledger-amount { text-align: right; }
.ledger-amount { font-variant-numeric: tabular-nums; }
.ledger-mobile-time { display: none; }
.ledger-time { white-space: nowrap; color: var(--muted); }
.ledger-type b, .ledger-amount strong { font-weight: 650; }
.ledger-amount::before { display: none; }
@media (max-width: 700px) {
  .ledger-table, .ledger-table tbody { display: block; }
  .ledger-table thead { display: none; }
  .ledger-entry { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 4px 12px; padding-block: 8px; border-bottom: 1px solid var(--line); }
  .ledger-table td { min-width: 0; padding: 0; border: 0; }
  .ledger-type { grid-column: 1 / -1; }
  .ledger-table .ledger-time { display: none; }
  .ledger-heading { display: flex; align-items: baseline; gap: 8px; }
  .ledger-heading b { flex: 1; min-width: 0; }
  .ledger-heading .ledger-mobile-time { display: block; flex: none; white-space: nowrap; color: var(--muted); font-weight: 400; }
  .ledger-table .ledger-amount { display: flex; flex-direction: column; align-items: flex-start; gap: 0; white-space: nowrap; text-align: left; }
  .ledger-table .ledger-balance-after { align-items: flex-end; text-align: right; }
  .ledger-amount::before { display: block; content: attr(data-label); color: var(--muted); }
}
.change-negative { color: var(--danger-text); }
.change-positive { color: var(--success-text); }
.state-card { display: flex; justify-content: space-between; align-items: center; gap: 12px; color: var(--muted); font-size: 14px; line-height: 1.6; }
.state-error { color: var(--danger); }
@media (max-width: 700px) {
  .totals { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
  .total-primary { grid-column: 1 / -1; }
  .total-card, .panel, .state-card { padding: 16px; }
  .metric-amount strong { font-size: 24px; }
  .bucket-grid { grid-template-columns: minmax(0, 1fr); }
  .panel { margin-top: 16px; }
}
@media (max-width: 620px) {
  .wallet-heading-copy { display: none; }
  .wallet-head { justify-content: flex-end; margin: 0; }
  .wallet-head:has(button) { margin-bottom: 12px; }
  .balance-amount strong { font-size: 32px; }
}

.ledger-result{margin:8px 0 0}.ledger-result>div{display:flex;flex-wrap:wrap;justify-content:space-between;gap:8px}.ledger-result dt{color:var(--muted)}.ledger-result dd{margin:0;font-weight:650;font-variant-numeric:tabular-nums;overflow-wrap:anywhere}
</style>
