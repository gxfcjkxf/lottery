<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
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
  defineProps<{ brandCode?: string; locale?: "en" | "zh" | "zh-CN" }>(),
  { locale: "en" },
);
const isZh = computed(() => props.locale !== "en");
const copy = computed(() =>
  isZh.value
    ? {
        title: "积分钱包",
        intro: "余额与流水来自当前品牌的真实钱包接口。",
        refresh: "刷新",
        loading: "读取中…",
        reading: "正在读取钱包…",
        unauthenticated: "请先登录后查看积分钱包。",
        recheck: "重新检查登录",
        retry: "重试",
        balance: "积分余额",
        available: "可用",
        points: "分",
        frozen: "冻结",
        frozenDetail: "人工及系统冻结",
        withdrawal: "提现中",
        withdrawalDetail: "暂不可用于投注",
        version: "账本版本",
        memberWallet: "会员钱包",
        buckets: "来源与状态",
        bucketsIntro: "逐项显示 12 个积分桶。",
        ledger: "钱包流水",
        ledgerIntro: "展示每笔变动前、变动额及变动后各积分桶。",
        empty: "当前页没有钱包流水。",
        bucket: "积分桶",
        before: "变动前",
        delta: "变动额",
        after: "变动后",
        page: (number: number) => `第 ${number} 页`,
        previous: "上一页",
        next: "下一页",
        sources: { recharge: "充值", winning: "中奖", gift: "赠送" },
        states: {
          available: "可用",
          manual_frozen: "人工冻结",
          system_frozen: "系统冻结",
          withdrawal: "提现中",
        },
        networkError: "钱包暂时无法读取。",
      }
    : {
        title: "Points wallet",
        intro:
          "Balances and ledger entries come from the current brand's wallet API.",
        refresh: "Refresh",
        loading: "Loading…",
        reading: "Loading wallet…",
        unauthenticated: "Sign in to view your points wallet.",
        recheck: "Check sign-in again",
        retry: "Retry",
        balance: "Points balance",
        available: "Available",
        points: "pts",
        frozen: "Frozen",
        frozenDetail: "Manual and system frozen",
        withdrawal: "Withdrawing",
        withdrawalDetail: "Not available for bets",
        version: "Ledger version",
        memberWallet: "Member wallet",
        buckets: "Sources and states",
        bucketsIntro: "All 12 points buckets.",
        ledger: "Wallet ledger",
        ledgerIntro:
          "Each entry shows all buckets before, delta, and after the change.",
        empty: "No wallet entries on this page.",
        bucket: "Bucket",
        before: "Before",
        delta: "Change",
        after: "After",
        page: (number: number) => `Page ${number}`,
        previous: "Previous",
        next: "Next",
        sources: { recharge: "Recharge", winning: "Winning", gift: "Gift" },
        states: {
          available: "Available",
          manual_frozen: "Manual freeze",
          system_frozen: "System freeze",
          withdrawal: "Withdrawing",
        },
        networkError: "Wallet is temporarily unavailable.",
      },
);
const PAGE_SIZE = 50;
const sources: WalletSource[] = ["recharge", "winning", "gift"];
const states: WalletState[] = [
  "available",
  "manual_frozen",
  "system_frozen",
  "withdrawal",
];
const wallet = ref<Wallet | null>(null);
const entries = ref<LedgerEntry[]>([]);
const state = ref<"loading" | "ready" | "unauthenticated" | "error">("loading");
const error = ref("");
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

async function load() {
  const requestedBrandCode = props.brandCode;
  const generation = ++loadGeneration;
  const isCurrent = () =>
    generation === loadGeneration && props.brandCode === requestedBrandCode;
  loading.value = true;
  state.value = "loading";
  error.value = "";
  try {
    const [currentWallet, ledger] = await Promise.all([
      api.value.wallet(),
      api.value.ledger(PAGE_SIZE, offset.value),
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
        isZh.value
          ? "钱包数据与当前品牌不匹配。"
          : "Wallet data does not match the current brand.",
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
            ? cause.message
            : copy.value.networkError;
    }
  } finally {
    if (isCurrent()) loading.value = false;
  }
}

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
function changePage(direction: -1 | 1) {
  offset.value = Math.max(0, offset.value + direction * PAGE_SIZE);
  void load();
}
</script>

<template>
  <section class="wallet-summary">
    <header class="wallet-head">
      <div>
        <p class="eyebrow">MY WALLET</p>
        <h1>{{ copy.title }}</h1>
        <p class="subhead">{{ copy.intro }}</p>
      </div>
      <button class="refresh" type="button" :disabled="loading" @click="load">
        {{ loading ? copy.loading : copy.refresh }}
      </button>
    </header>

    <div v-if="state === 'loading' && !wallet" class="state-card" role="status">
      {{ copy.reading }}
    </div>
    <div
      v-else-if="state === 'unauthenticated'"
      class="state-card"
      role="status"
    >
      {{ error }}
      <button type="button" class="text-action" @click="load">
        {{ copy.recheck }}
      </button>
    </div>
    <div
      v-else-if="state === 'error'"
      class="state-card state-error"
      role="alert"
    >
      <span>{{ error }}</span>
      <button type="button" class="text-action" @click="load">
        {{ copy.retry }}
      </button>
    </div>
    <template v-else-if="wallet">
      <div class="totals">
        <article class="total-card total-primary">
          <span>{{ copy.balance }}</span
          ><strong>{{ formatIntegerAmount(wallet.display_points) }}</strong>
          <small
            >{{ copy.available }}
            {{ formatIntegerAmount(wallet.available_points) }}
            {{ copy.points }}</small
          >
        </article>
        <article class="total-card">
          <span>{{ copy.frozen }}</span
          ><strong>{{ formatIntegerAmount(wallet.frozen_points) }}</strong
          ><small>{{ copy.frozenDetail }}</small>
        </article>
        <article class="total-card">
          <span>{{ copy.withdrawal }}</span
          ><strong>{{ formatIntegerAmount(wallet.withdrawal_points) }}</strong
          ><small>{{ copy.withdrawalDetail }}</small>
        </article>
        <article class="total-card">
          <span>{{ copy.version }}</span
          ><strong>{{ wallet.version }}</strong
          ><small>{{ copy.memberWallet }}</small>
        </article>
      </div>

      <section class="panel">
        <div class="panel-title">
          <div>
            <h2>{{ copy.buckets }}</h2>
            <p>{{ copy.bucketsIntro }}</p>
          </div>
        </div>
        <div class="bucket-grid">
          <article
            v-for="row in bucketRows"
            :key="row.source"
            class="bucket-source"
          >
            <h3>{{ copy.sources[row.source] }}</h3>
            <dl>
              <div v-for="bucketState in states" :key="bucketState">
                <dt>{{ copy.states[bucketState] }}</dt>
                <dd>{{ formatIntegerAmount(row.values[bucketState]) }}</dd>
              </div>
            </dl>
          </article>
        </div>
      </section>

      <section class="panel ledger-panel">
        <div class="panel-title">
          <div>
            <h2>{{ copy.ledger }}</h2>
            <p>{{ copy.ledgerIntro }}</p>
          </div>
        </div>
        <p v-if="!entries.length" class="empty">{{ copy.empty }}</p>
        <article v-for="entry in entries" :key="entry.id" class="ledger-entry">
          <div class="entry-head">
            <div>
              <b>{{ entry.entry_type }}</b
              ><small>{{
                new Date(entry.created_at).toLocaleString(isZh ? "zh-CN" : "en")
              }}</small>
            </div>
            <span>{{ copy.version }} {{ entry.version }}</span>
          </div>
          <p class="entry-reason">{{ entry.reason }}</p>
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
        </article>
        <footer class="pager">
          <span>{{ copy.page(offset / PAGE_SIZE + 1) }}</span>
          <div>
            <button
              type="button"
              :disabled="offset === 0 || loading"
              @click="changePage(-1)"
            >
              {{ copy.previous }}</button
            ><button
              type="button"
              :disabled="entries.length < PAGE_SIZE || loading"
              @click="changePage(1)"
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
.wallet-summary {
  width: 100%;
  min-width: 0;
  color: #252a36;
}
.wallet-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 14px;
  margin: 4px 0 18px;
}
.wallet-head h1 {
  font-size: 25px;
  letter-spacing: -0.5px;
  margin: 3px 0 5px;
}
.wallet-head p {
  margin: 0;
}
.eyebrow {
  font-size: 10px;
  letter-spacing: 1.2px;
  color: #8991a2;
  font-weight: 700;
}
.subhead,
.panel-title p {
  font-size: 11px;
  color: #737b8c;
  line-height: 1.55;
}
.refresh,
.pager button {
  border: 1px solid #e1e4eb;
  border-radius: 7px;
  background: white;
  padding: 8px 12px;
  color: #50596a;
  font-size: 11px;
}
.totals {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 14px;
}
.total-card,
.panel,
.state-card {
  background: white;
  border: 1px solid #e9ebf0;
  border-radius: 11px;
  padding: 15px;
  min-width: 0;
  box-shadow: 0 2px 8px #1e2a5008;
}
.total-card {
  display: grid;
  gap: 8px;
}
.total-card > span,
.total-card small {
  font-size: 10px;
  color: #737b8c;
}
.total-card strong {
  font-size: 22px;
  overflow-wrap: anywhere;
  font-variant-numeric: tabular-nums;
}
.total-primary {
  background: linear-gradient(135deg, #5969df, #7986ed);
  color: white;
  border: 0;
}
.total-primary > span,
.total-primary small {
  color: #eef0ff;
}
.panel {
  margin-top: 14px;
}
.panel-title {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 12px;
}
.panel-title h2 {
  font-size: 14px;
  margin: 0 0 4px;
}
.panel-title p {
  margin: 0;
}
.bucket-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}
.bucket-source {
  border: 1px solid #eceef3;
  border-radius: 8px;
  padding: 11px;
  min-width: 0;
}
.bucket-source h3 {
  font-size: 12px;
  margin: 0 0 8px;
}
.bucket-source dl {
  margin: 0;
  display: grid;
  gap: 7px;
}
.bucket-source dl div {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  align-items: baseline;
}
.bucket-source dt {
  font-size: 10px;
  color: #777f8f;
}
.bucket-source dd {
  font-size: 11px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  margin: 0;
  overflow-wrap: anywhere;
}
.ledger-entry {
  padding: 14px 0;
  border-top: 1px solid #eff0f4;
  min-width: 0;
}
.entry-head {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
  font-size: 11px;
}
.entry-head > div {
  display: grid;
  gap: 4px;
}
.entry-head small,
.entry-head > span {
  color: #8991a0;
  font-size: 10px;
}
.entry-reason {
  font-size: 11px;
  color: #626b7c;
  overflow-wrap: anywhere;
}
.snapshot-grid {
  display: grid;
  grid-template-columns: minmax(112px, 1.1fr) repeat(3, minmax(0, 1fr));
  gap: 6px 8px;
  align-items: baseline;
  font-size: 10px;
  font-variant-numeric: tabular-nums;
}
.snapshot-label {
  color: #8991a0;
  font-weight: 700;
  padding-bottom: 3px;
}
.snapshot-bucket {
  font-weight: 500;
  color: #626b7c;
  overflow-wrap: anywhere;
}
.snapshot-grid > span {
  overflow-wrap: anywhere;
  text-align: right;
}
.pager {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-top: 13px;
  color: #737b8c;
  font-size: 11px;
}
.pager div {
  display: flex;
  gap: 7px;
}
.pager button:disabled,
.refresh:disabled {
  opacity: 0.48;
  cursor: not-allowed;
}
.state-card {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  color: #667083;
  font-size: 12px;
  line-height: 1.6;
}
.state-error {
  background: #fff0ef;
  color: #a83d36;
}
.text-action {
  border: 0;
  background: transparent;
  color: #5969df;
  text-decoration: underline;
  white-space: nowrap;
}
.empty {
  font-size: 11px;
  color: #737b8c;
}
@media (max-width: 700px) {
  .totals {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
  }
  .total-card {
    padding: 12px;
  }
  .total-card strong {
    font-size: 19px;
  }
  .bucket-grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .panel {
    padding: 13px;
  }
  .snapshot-grid {
    grid-template-columns: minmax(94px, 1fr) repeat(3, minmax(0, 0.8fr));
    gap: 6px 4px;
    font-size: 9px;
  }
  .snapshot-bucket {
    font-size: 9px;
  }
  .wallet-head {
    align-items: center;
  }
}
</style>
