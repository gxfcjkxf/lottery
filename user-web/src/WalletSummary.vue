<script setup lang="ts">
import { useUserI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import SiteIcon from "@lottery/shared/SiteIcon";
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
import { formatDisplayDateTime } from "@lottery/shared";
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useUserI18n } from './i18n';
import { RouterLink } from 'vue-router';
import {
  createWalletApi,
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
const title = computed(() => ui(props.view === 'overview' ? 'user.wallet.title' : props.view === 'buckets' ? 'user.wallet.buckets' : 'user.wallet.ledger'));
const copy = computed(() => ({
  title: ui('user.wallet.title'), refresh: ui('user.wallet.refresh'), loading: ui('user.wallet.loading'), reading: ui('user.wallet.reading'), unauthenticated: ui('user.wallet.unauthenticated'), recheck: ui('user.wallet.recheck'), retry: ui('user.wallet.retry'), balance: ui('user.wallet.balance'), points: ui('user.wallet.points'), frozen: ui('user.wallet.frozen'), withdrawal: ui('user.wallet.withdrawal'), buckets: ui('user.wallet.buckets'), ledger: ui('user.wallet.ledger'), empty: ui('user.wallet.empty'), bucket: ui('user.wallet.bucket'), before: ui('user.wallet.before'), delta: ui('user.wallet.delta'), after: ui('user.wallet.after'), page: (number: number) => ui('user.wallet.page', { number }), previous: ui('user.wallet.previous'), next: ui('user.wallet.next'), sources: { recharge: ui('user.wallet.source.recharge'), winning: ui('user.wallet.source.winning'), gift: ui('user.wallet.source.gift'), commission: ui('user.wallet.source.commission') }, states: { available: ui('user.wallet.state.available'), manual_frozen: ui('user.wallet.state.manualFrozen'), system_frozen: ui('user.wallet.state.systemFrozen'), withdrawal: ui('user.wallet.state.withdrawal') }, networkError: ui('user.wallet.networkError'), brandMismatch: ui('user.wallet.brandMismatch'),
}));
function ledgerLabel(entry:LedgerEntry,reason=false):string{
 if(entry.reference_type==="canada_order"){
 const frozenDelta=Object.values(entry.delta_snapshot).reduce((sum,b)=>sum+BigInt(b.system_frozen),0n);
 if(entry.entry_type==='bet'&&frozenDelta>0n)return ui(reason?'user.wallet.canada.reserveReason':'user.wallet.canada.reserve');
 if(entry.entry_type==='bet'&&frozenDelta<0n)return ui(reason?'user.wallet.canada.lossReason':'user.wallet.canada.loss');
 if(entry.entry_type==='refund'&&frozenDelta<0n)return ui(reason?'user.wallet.canada.releaseReason':'user.wallet.canada.release');
 switch(entry.entry_type){case "bet":return ui(reason?'user.wallet.canada.betReason':'user.wallet.canada.bet');case "prize":return ui(reason?'user.wallet.canada.prizeReason':'user.wallet.canada.prize');case "refund":return ui(reason?'user.wallet.canada.refundReason':'user.wallet.canada.refund');default:throw new Error('Unsupported Canada ledger entry')}
 }
 if(entry.reference_type!=="mark_six_order") {
   if (reason) return entry.reason;
   return ui(`user.wallet.entry.${entry.entry_type}`);
 }
 switch(entry.entry_type){
 case "bet":return ui(reason?'user.wallet.markSix.betReason':'user.wallet.markSix.bet');
 case "prize":return ui(reason?'user.wallet.markSix.prizeReason':'user.wallet.markSix.prize');
 case "refund":return ui(reason?'user.wallet.markSix.refundReason':'user.wallet.markSix.refund');
 default:throw new Error('Unsupported Mark Six ledger entry');
 }
}
function availableChange(entry: LedgerEntry): string {
  const change = sources.reduce((sum, source) => sum + BigInt(entry.delta_snapshot[source].available), 0n);
  return `${change > 0n ? '+' : ''}${formatIntegerAmount(change.toString())}`;
}
const PAGE_SIZE = 50;
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
let polling = false;
const bucketRows = computed(() =>
  wallet.value
    ? sources.map((source) => ({
        source,
        values: wallet.value!.by_source[source],
      }))
    : [],
);

async function load(silent = false) {
  const requestedBrandCode = props.brandCode;
  const generation = ++loadGeneration;
  const isCurrent = () =>
    generation === loadGeneration && props.brandCode === requestedBrandCode;
  if (silent) polling = true;
  else {
    loading.value = true;
    state.value = "loading";
    error.value = "";
  }
  try {
    const [currentWallet, ledger] = await Promise.all([
      api.value.wallet(),
      props.view === 'ledger' ? api.value.ledger(PAGE_SIZE, offset.value) : Promise.resolve({ items: [] }),
    ]);
    if (!isCurrent()) return;
    if (
      ledger.items.some(
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
    wallet.value = currentWallet;
    entries.value = ledger.items;
    state.value = "ready";
  } catch (cause) {
    if (!isCurrent()) return;
    wallet.value = null;
    entries.value = [];
    if (cause instanceof WalletApiError && cause.status === 401) {
      state.value = "unauthenticated";
      error.value = copy.value.unauthenticated;
    } else {
      state.value = "error";
      error.value =
        cause instanceof WalletApiError &&
        cause.status === 0 &&
        cause.code !== "brand_mismatch"
          ? copy.value.networkError
          : cause instanceof Error
            ? publicErrorMessage(cause)
            : copy.value.networkError;
    }
  } finally {
    if (silent) polling = false;
    else if (isCurrent()) loading.value = false;
  }
}

const poll=setInterval(()=>{if(!loading.value&&!polling&&state.value==="ready")void load(true)},5000);
onUnmounted(()=>{clearInterval(poll);loadGeneration++});
onMounted(() => void load());
watch(
  () => props.brandCode,
  () => {
    loadGeneration++;
    wallet.value = null;
    entries.value = [];
    offset.value = 0;
    void load();
  },
);
async function changePage(direction: -1 | 1) {
  offset.value = Math.max(0, offset.value + direction * PAGE_SIZE);
  await load();
}
</script>

<template>
  <section class="wallet-summary">
    <header class="wallet-head page-heading">
      <div class="wallet-heading-copy">
        <h1>{{ title }}</h1>
      </div>
      <Teleport defer to="#user-page-refresh"><button v-if="!wallet || props.view !== 'overview'" v-refresh-button="loading ? copy.loading : copy.refresh" class="refresh" type="button" :disabled="loading" @click="withGlobalLoading(load)">
        {{ loading ? copy.loading : copy.refresh }}
      </button></Teleport>
    </header>

    <div v-global-loading v-if="state === 'loading' && !wallet" class="state-card" role="status">
      {{ copy.reading }}
    </div>
    <div
      v-else-if="state === 'unauthenticated'"
      class="state-card"
      role="status"
    >
      {{ displayPublicError(error) }}
      <button type="button" class="text-action" @click="withGlobalLoading(load)">
        {{ copy.recheck }}
      </button>
    </div>
    <div
      v-else-if="state === 'error'"
      class="state-card state-error"
      role="alert"
    >
      <span>{{ displayPublicError(error) }}</span>
      <button type="button" class="text-action" @click="withGlobalLoading(load)">
        {{ copy.retry }}
      </button>
    </div>
    <template v-else-if="wallet">
      <div v-if="props.view === 'overview'" class="totals">
        <article class="total-card total-primary">
          <div class="balance-card-head">
            <span>{{ copy.balance }}</span>
            <div class="balance-amount"><strong>{{ formatIntegerAmount(wallet.display_points) }}</strong><span>{{ copy.points }}</span></div>
            <Teleport defer to="#user-page-refresh"><button v-refresh-button="copy.refresh" class="refresh" type="button" :disabled="loading" @click="withGlobalLoading(load)">{{ copy.refresh }}</button></Teleport>
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

      <nav v-if="props.view === 'overview'" class="wallet-links">
        <RouterLink to="/wallet/buckets" class="button button-secondary">{{ copy.buckets }}</RouterLink>
        <RouterLink to="/wallet/ledger" class="button button-secondary">{{ copy.ledger }}</RouterLink>
      </nav>

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

      <section v-if="props.view === 'ledger'" class="panel ledger-panel">
        <p v-if="!entries.length" class="empty empty-state">{{ copy.empty }}</p>
        <article v-for="entry in entries" :key="entry.id" class="ledger-entry">
          <div class="entry-head">
            <div>
              <b>{{ ledgerLabel(entry) }}</b
              ><small>{{
                formatDisplayDateTime(entry.created_at, props.locale)
              }}</small>
            </div>
            <div class="entry-change">
              <span>{{ ui('user.wallet.availableChange') }}</span>
              <strong :class="{ 'change-negative': availableChange(entry).startsWith('−'), 'change-positive': availableChange(entry).startsWith('+') }">{{ availableChange(entry) }}</strong>
            </div>
          </div>
          <details class="ledger-details">
            <summary>{{ ui('user.wallet.viewDetails') }}<SiteIcon name="down" /></summary>
          <p class="entry-reason">{{ ledgerLabel(entry,true) }}</p>
          <p v-if="entry.reference_type==='mark_six_order'||entry.reference_type==='canada_order'" class="entry-reference">{{ ui('user.wallet.orderReference') }} · {{ entry.reference_id }}</p>
            <div class="snapshot-grid">
            <div class="snapshot-label">{{ copy.bucket }}</div>
            <div class="snapshot-label">{{ copy.before }}</div>
            <div class="snapshot-label">{{ copy.delta }}</div>
            <div class="snapshot-label">{{ copy.after }}</div>
            <template v-for="source in sources" :key="`${entry.id}-${source}`"
              ><template
                v-for="bucketState in states"
                :key="`${entry.id}-${source}-${bucketState}`"
                ><b class="snapshot-bucket"
                  >{{ copy.sources[source] }} ·
                  {{ copy.states[bucketState] }}</b
                ><span>{{
                  formatIntegerAmount(
                    entry.before_snapshot[source][bucketState],
                  )
                }}</span
                ><span>{{
                  formatIntegerAmount(entry.delta_snapshot[source][bucketState])
                }}</span
                ><span>{{
                  formatIntegerAmount(entry.after_snapshot[source][bucketState])
                }}</span></template
              ></template
            >
          </div>
          </details>
        </article>
        <footer class="pager">
          <span>{{ copy.page(offset / PAGE_SIZE + 1) }}</span>
          <div>
            <button
              type="button"
              :disabled="offset === 0 || loading"
              @click="withGlobalLoading(changePage, -1)"
            >
              {{ copy.previous }}</button
            ><button
              type="button"
              :disabled="entries.length < PAGE_SIZE || loading"
              @click="withGlobalLoading(changePage, 1)"
            >
              {{ copy.next }}
            </button>
          </div>
        </footer>
      </section>
    </template>
  </section>
</template>

<style scoped>
.wallet-summary { width: 100%; max-width: 1100px; margin: 0 auto; min-width: 0; color: var(--ink); }
.wallet-head { display: flex; flex-direction: row; justify-content: space-between; align-items: center; gap: 12px; margin: 0 0 12px; }
.wallet-head h1 { font-size: 24px; margin: 0; }
.totals { display: grid; grid-template-columns: minmax(0, 1.6fr) repeat(2, minmax(0, 1fr)); gap: 12px; }
.wallet-links { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; margin-top: 16px; }
.wallet-links a { min-width: 0; min-height: 44px; text-align: center; overflow-wrap: anywhere; }
.total-card, .panel, .state-card { min-width: 0; background: var(--surface); border: 1px solid var(--line); border-radius: var(--radius); padding: 16px; box-shadow: var(--shadow-sm); }
.total-card { display: flex; flex-direction: column; align-items: flex-start; gap: 12px; }
.total-label { font-size: 14px; font-weight: 600; color: var(--ink-secondary); }
.total-card.key-value-row { grid-template-columns: fit-content(60%) minmax(0, 1fr) !important; }
.balance-amount, .metric-amount { display: flex; flex-wrap: wrap; align-items: baseline; gap: 8px; min-width: 0; font-variant-numeric: tabular-nums; }
.balance-amount strong { font-size: 36px; line-height: 1.2; overflow-wrap: anywhere; min-width: 0; }
.metric-amount strong { font-size: 28px; line-height: 1.2; overflow-wrap: anywhere; min-width: 0; }
.balance-amount > span, .metric-amount > span { font-size: 12px; }
.metric-amount > span { color: var(--muted); }
.total-primary { background: linear-gradient(135deg, var(--primary), var(--primary-dark)); border-color: transparent; color: var(--surface); gap: 10px; }
.balance-card-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; width: 100%; font-size: 14px; font-weight: 600; }
.balance-card-head { flex-wrap: wrap; }
.balance-card-head .balance-amount { flex: 1 1 8ch; justify-content: flex-end; }
.key-value-value.metric-amount { justify-content: flex-end; }
.wallet-available { display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: 8px; width: 100%; margin-top: 4px; padding-top: 12px; border-top: 1px solid color-mix(in srgb, var(--surface) 25%, transparent); font-size: 13px; }
.wallet-available b { min-width: 0; font-size: 20px; overflow-wrap: anywhere; font-variant-numeric: tabular-nums; }
.wallet-available small { font-size: 12px; font-weight: 400; }
.panel { margin-top: 20px; }
.bucket-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.bucket-source { min-width: 0; padding: 16px; border: 1px solid var(--line); border-radius: var(--radius-sm); }
.source-head { margin-bottom: 12px; }
.source-head h3 { margin: 0; font-size: 15px; }
.bucket-source dl { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px 16px; margin: 0; }
.bucket-source dl > div { min-width: 0; }
.bucket-source dt { font-size: 12px; color: var(--muted); }
.bucket-source dd { margin: 0; font-size: 16px; font-weight: 650; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.bucket-available dd { color: var(--primary-dark); }
.ledger-entry { padding: 18px 0; border-top: 1px solid var(--line); min-width: 0; }
.entry-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; }
.entry-head > div { min-width: 0; display: grid; gap: 5px; overflow-wrap: anywhere; }
.entry-head b { font-size: 14px; }
.entry-head small { color: var(--muted); font-size: 12px; }
.entry-change { text-align: right; font-variant-numeric: tabular-nums; }
.entry-change > span { color: var(--muted); font-size: 11px; }
.entry-change strong { font-size: 18px; }
.change-negative { color: var(--danger); }
.change-positive { color: var(--primary-dark); }
.entry-reason { margin: 12px 0; font-size: 13px; line-height: 1.6; color: var(--ink-secondary); overflow-wrap: anywhere; }
.entry-reference { font-size: 12px; color: var(--muted); overflow-wrap: anywhere; }
.ledger-details summary { display: flex; align-items: center; gap: 8px; min-height: 44px; width: fit-content; font-size: 13px; color: var(--primary-dark); cursor: pointer; list-style: none; }
.ledger-details summary::-webkit-details-marker { display: none; }
.ledger-details[open] summary .site-icon { transform: rotate(180deg); }
.ledger-details summary:focus-visible { outline: 2px solid var(--primary); outline-offset: 3px; border-radius: var(--radius-sm); }
.snapshot-grid { display: grid; grid-template-columns: minmax(0, 1.4fr) repeat(3, minmax(0, 1fr)); gap: 0 10px; align-items: stretch; font-size: 12px; font-variant-numeric: tabular-nums; }
.snapshot-grid > * { display: flex; align-items: center; min-width: 0; overflow-wrap: anywhere; padding: 8px 0; border-bottom: 1px solid var(--line-subtle); }
.snapshot-label { color: var(--muted); font-weight: 600; }
.snapshot-bucket { font-weight: 400; color: var(--ink-secondary); }
.snapshot-grid > span, .snapshot-label:not(:first-child) { justify-content: flex-end; text-align: right; }
.pager { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 12px; padding-top: 16px; color: var(--muted); font-size: 12px; }
.pager > div { display: flex; gap: 8px; }
.pager button { min-height: 44px; border: 1px solid var(--line); border-radius: var(--radius-sm); background: var(--surface); padding: 8px 12px; color: var(--ink-secondary); font-size: 13px; }
.pager button:disabled { opacity: 0.48; cursor: not-allowed; }
.state-card { display: flex; justify-content: space-between; align-items: center; gap: 12px; color: var(--muted); font-size: 14px; line-height: 1.6; }
.state-error { color: var(--danger); }
.text-action { min-height: 44px; border: 0; background: transparent; color: var(--primary-dark); font-size: 14px; text-decoration: underline; }
@media (max-width: 700px) {
  .totals { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
  .total-primary { grid-column: 1 / -1; }
  .total-card, .panel, .state-card { padding: 16px; }
  .metric-amount strong { font-size: 24px; }
  .bucket-grid { grid-template-columns: minmax(0, 1fr); }
  .panel { margin-top: 16px; }
  .snapshot-grid { gap: 0 6px; }
}
@media (max-width: 620px) {
  .wallet-heading-copy { display: none; }
  .wallet-head { justify-content: flex-end; margin: 0; }
  .wallet-head:has(button) { margin-bottom: 12px; }
  .balance-amount strong { font-size: 32px; }
}
</style>
