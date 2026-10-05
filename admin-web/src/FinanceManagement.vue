<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import type { AdminAccount } from "./admin-api";
import {
  canViewRecharges,
  canViewWallet,
  canWriteFinance,
  createBodyKeyTracker,
  createFinanceApi,
  FinanceApiError,
  formatIntegerAmount,
  isPositiveAmount,
  isSignedAmount,
  type FinanceAccount,
  type LedgerEntry,
  type Recharge,
  type Reconciliation,
  type Wallet,
  type WalletSource,
  type WalletState,
} from "./finance-api";
import PointPolicySettings from "./PointPolicySettings.vue";
import { canViewPointPolicy } from "./point-policy-api";

const props = defineProps<{
  account: AdminAccount & Partial<FinanceAccount>;
  brandId: string;
}>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const PAGE_SIZE = 50;
const api = createFinanceApi();
const sources: WalletSource[] = ["recharge", "winning", "gift"];
const states: WalletState[] = [
  "available",
  "manual_frozen",
  "system_frozen",
  "withdrawal",
];
const sourceNames: Record<WalletSource, string> = {
  recharge: "充值",
  winning: "中奖",
  gift: "赠送",
};
const stateNames: Record<WalletState, string> = {
  available: "可用",
  manual_frozen: "人工冻结",
  system_frozen: "系统冻结",
  withdrawal: "提现中",
};
const memberId = ref("");
const loadedMemberId = ref("");
const wallet = ref<Wallet | null>(null);
const entries = ref<LedgerEntry[]>([]);
const reconciliation = ref<Reconciliation | null>(null);
const recharges = ref<Recharge[]>([]);
const ledgerOffset = ref(0);
const rechargeOffset = ref(0);
const loading = ref(false);
const mutating = ref(false);
const error = ref("");
const walletError = ref("");
const rechargeError = ref("");
const notice = ref("");
const rechargeForm = ref({
  points: "",
  proof_reference: "",
  remark: "",
  reason: "",
});
const rechargeReasonById = ref<Record<string, string>>({});
const cancelReasonById = ref<Record<string, string>>({});
const freezeForm = ref({ points: "", reason: "" });
const unfreezeReason = ref("");
const adjustForm = ref({
  source: "recharge" as WalletSource,
  delta: "",
  reason: "",
});
const rechargeKeyFor = createBodyKeyTracker();
const confirmKeyFor = createBodyKeyTracker();
const cancelKeyFor = createBodyKeyTracker();
const freezeKeyFor = createBodyKeyTracker();
const unfreezeKeyFor = createBodyKeyTracker();
const adjustKeyFor = createBodyKeyTracker();
let dataRequestGeneration = 0;
let queueRequestGeneration = 0;
const viewWallet = computed(() => canViewWallet(props.account, props.brandId));
const viewRecharges = computed(() =>
  canViewRecharges(props.account, props.brandId),
);
const writeRecharge = computed(() =>
  canWriteFinance(props.account, props.brandId, "recharge.write.brand"),
);
const writeFreeze = computed(() =>
  canWriteFinance(props.account, props.brandId, "wallet.freeze.brand"),
);
const writeAdjust = computed(() =>
  canWriteFinance(props.account, props.brandId, "wallet.adjust.brand"),
);
const canSearch = computed(() => viewWallet.value || viewRecharges.value);
const canViewPolicy = computed(() =>
  canViewPointPolicy(props.account, props.brandId),
);
const walletBucketRows = computed(() =>
  wallet.value
    ? sources.flatMap((source) =>
        states.map((state) => ({
          source,
          state,
          value: wallet.value!.by_source[source][state],
        })),
      )
    : [],
);
const reconciliationRows = computed(() =>
  reconciliation.value
    ? sources.flatMap((source) =>
        states.map((state) => ({
          source,
          state,
          expected: reconciliation.value!.expected[source][state],
          actual: reconciliation.value!.actual[source][state],
        })),
      )
    : [],
);
const reversedEntryIds = computed(
  () =>
    new Set(
      entries.value.flatMap((entry) =>
        entry.reversal_of ? [entry.reversal_of] : [],
      ),
    ),
);
const unfreezeCandidates = computed(() =>
  entries.value.filter((entry) => {
    if (entry.entry_type !== "freeze") return false;
    if (entry.reversal_of || reversedEntryIds.value.has(entry.id)) return false;
    return sources.some((source) =>
      isPositiveAmount(entry.delta_snapshot[source].manual_frozen),
    );
  }),
);
const hasNextLedger = computed(() => entries.value.length === PAGE_SIZE);
const hasNextRecharge = computed(() => recharges.value.length === PAGE_SIZE);

function messageFor(cause: unknown): string {
  if (cause instanceof FinanceApiError) {
    const message =
      cause.status === 403
        ? `${cause.message}（服务端权限校验仍为最终依据。）`
        : cause.message;
    return cause.status === 409
      ? `${message}；数据版本已变化，请刷新后重试。`
      : message;
  }
  return cause instanceof Error ? cause.message : "请求失败，请重试。";
}
function handleFailure(
  cause: unknown,
  target: "global" | "wallet" | "recharge" = "global",
) {
  if (cause instanceof FinanceApiError && cause.status === 401)
    emit("session-invalid");
  const message = messageFor(cause);
  if (target === "wallet") walletError.value = message;
  else if (target === "recharge") rechargeError.value = message;
  else error.value = message;
}
function clearData() {
  loadedMemberId.value = "";
  wallet.value = null;
  entries.value = [];
  reconciliation.value = null;
  recharges.value = [];
  walletError.value = "";
  rechargeError.value = "";
}
async function inspectMember() {
  const id = memberId.value.trim();
  if (!id || loading.value) return;
  if (loadedMemberId.value !== id) {
    wallet.value = null;
    entries.value = [];
    reconciliation.value = null;
    recharges.value = [];
  }
  loadedMemberId.value = id;
  ledgerOffset.value = 0;
  rechargeOffset.value = 0;
  await reloadData();
}
async function reloadData() {
  const targetId = loadedMemberId.value;
  if (!targetId) return;
  const requestBrandId = props.brandId;
  const generation = ++dataRequestGeneration;
  const isCurrent = () =>
    generation === dataRequestGeneration &&
    props.brandId === requestBrandId &&
    loadedMemberId.value === targetId;
  loading.value = true;
  error.value = "";
  walletError.value = "";
  rechargeError.value = "";
  const jobs: Promise<void>[] = [];
  if (viewWallet.value) {
    jobs.push(
      api
        .wallet(requestBrandId, targetId)
        .then((value) => {
          if (!isCurrent()) return;
          if (value.brand_id === requestBrandId && value.member_id === targetId)
            wallet.value = value;
          else walletError.value = "钱包响应与当前品牌或会员不匹配。";
        })
        .catch((cause: unknown) => {
          if (isCurrent()) handleFailure(cause, "wallet");
        }),
    );
    jobs.push(
      api
        .ledger(requestBrandId, targetId, PAGE_SIZE, ledgerOffset.value)
        .then((result) => {
          if (!isCurrent()) return;
          if (
            result.items.every(
              (entry) =>
                entry.brand_id === requestBrandId &&
                entry.member_id === targetId,
            )
          )
            entries.value = result.items;
          else walletError.value = "流水响应与当前品牌或会员不匹配。";
        })
        .catch((cause: unknown) => {
          if (isCurrent()) handleFailure(cause, "wallet");
        }),
    );
    jobs.push(
      api
        .reconciliation(requestBrandId, targetId)
        .then((value) => {
          if (isCurrent() && value.member_id === targetId)
            reconciliation.value = value;
        })
        .catch((cause: unknown) => {
          if (isCurrent()) handleFailure(cause, "wallet");
        }),
    );
  } else {
    wallet.value = null;
    entries.value = [];
    reconciliation.value = null;
  }
  if (viewRecharges.value) {
    jobs.push(
      api
        .recharges(requestBrandId, targetId, PAGE_SIZE, rechargeOffset.value)
        .then((result) => {
          if (!isCurrent()) return;
          if (
            result.items.every(
              (item) =>
                item.brand_id === requestBrandId && item.member_id === targetId,
            )
          )
            recharges.value = result.items;
          else rechargeError.value = "充值单响应与当前品牌或会员不匹配。";
        })
        .catch((cause: unknown) => {
          if (isCurrent()) handleFailure(cause, "recharge");
        }),
    );
  } else {
    recharges.value = [];
  }
  await Promise.all(jobs);
  if (isCurrent()) loading.value = false;
}
onMounted(() => {
  if (viewRecharges.value && !viewWallet.value) void loadRechargeQueue();
});
watch(
  () => props.brandId,
  () => {
    dataRequestGeneration++;
    queueRequestGeneration++;
    memberId.value = "";
    clearData();
    ledgerOffset.value = 0;
    rechargeOffset.value = 0;
    if (viewRecharges.value && !viewWallet.value) void loadRechargeQueue();
  },
);

async function loadRechargeQueue() {
  if (!viewRecharges.value) return;
  const requestBrandId = props.brandId;
  const generation = ++queueRequestGeneration;
  const isCurrent = () =>
    generation === queueRequestGeneration &&
    props.brandId === requestBrandId &&
    !loadedMemberId.value;
  loading.value = true;
  rechargeError.value = "";
  try {
    const result = await api.recharges(
      requestBrandId,
      undefined,
      PAGE_SIZE,
      rechargeOffset.value,
    );
    if (
      isCurrent() &&
      result.items.every((item) => item.brand_id === requestBrandId)
    )
      recharges.value = result.items;
  } catch (cause) {
    if (isCurrent()) handleFailure(cause, "recharge");
  } finally {
    if (isCurrent()) loading.value = false;
  }
}
async function moveLedgerPage(direction: -1 | 1) {
  if (!loadedMemberId.value || !viewWallet.value) return;
  const requestBrandId = props.brandId;
  const targetId = loadedMemberId.value;
  const generation = ++dataRequestGeneration;
  const isCurrent = () =>
    generation === dataRequestGeneration &&
    props.brandId === requestBrandId &&
    loadedMemberId.value === targetId;
  ledgerOffset.value = Math.max(0, ledgerOffset.value + direction * PAGE_SIZE);
  loading.value = true;
  try {
    const result = await api.ledger(
      requestBrandId,
      targetId,
      PAGE_SIZE,
      ledgerOffset.value,
    );
    if (
      isCurrent() &&
      result.items.every(
        (entry) =>
          entry.brand_id === requestBrandId && entry.member_id === targetId,
      )
    )
      entries.value = result.items;
  } catch (cause) {
    if (isCurrent()) handleFailure(cause, "wallet");
  } finally {
    if (isCurrent()) loading.value = false;
  }
}
async function moveRechargePage(direction: -1 | 1) {
  if (!viewRecharges.value) return;
  rechargeOffset.value = Math.max(
    0,
    rechargeOffset.value + direction * PAGE_SIZE,
  );
  if (loadedMemberId.value) await reloadData();
  else await loadRechargeQueue();
}
async function mutate(
  action: () => Promise<unknown>,
  successText: string,
  clearKey: () => void,
) {
  if (mutating.value) return;
  mutating.value = true;
  error.value = "";
  notice.value = "";
  try {
    await action();
    clearKey();
    notice.value = successText;
    if (loadedMemberId.value) await reloadData();
    else if (viewRecharges.value) await loadRechargeQueue();
  } catch (cause) {
    handleFailure(cause);
  } finally {
    mutating.value = false;
  }
}
async function createRecharge() {
  if (
    !loadedMemberId.value ||
    !isPositiveAmount(rechargeForm.value.points) ||
    !rechargeForm.value.reason.trim()
  )
    return;
  const body: {
    member_id: string;
    points: string;
    proof_reference?: string;
    remark?: string;
    reason: string;
  } = {
    member_id: loadedMemberId.value,
    points: rechargeForm.value.points,
    reason: rechargeForm.value.reason.trim(),
  };
  if (rechargeForm.value.proof_reference.trim())
    body.proof_reference = rechargeForm.value.proof_reference.trim();
  if (rechargeForm.value.remark.trim())
    body.remark = rechargeForm.value.remark.trim();
  await mutate(
    () =>
      api.createRecharge(
        props.brandId,
        body,
        rechargeKeyFor({ brand_id: props.brandId, ...body }),
      ),
    "充值单已创建；实际入账须确认后完成。",
    () => rechargeKeyFor.clear(),
  );
  if (!error.value)
    rechargeForm.value = {
      points: "",
      proof_reference: "",
      remark: "",
      reason: "",
    };
}
async function confirmRecharge(recharge: Recharge) {
  const reason = rechargeReasonById.value[recharge.id]?.trim() ?? "";
  if (!reason) return;
  const body = { version: recharge.version, reason };
  await mutate(
    () =>
      api.confirmRecharge(
        props.brandId,
        recharge.id,
        body,
        confirmKeyFor({ brand_id: props.brandId, id: recharge.id, ...body }),
      ),
    "充值单已确认，正在读取最新钱包状态。",
    () => confirmKeyFor.clear(),
  );
}
async function cancelRecharge(recharge: Recharge) {
  const reason = cancelReasonById.value[recharge.id]?.trim() ?? "";
  if (recharge.state !== "pending" || !reason) return;
  const body = { version: recharge.version, reason };
  await mutate(
    () =>
      api.cancelRecharge(
        props.brandId,
        recharge.id,
        body,
        cancelKeyFor({ brand_id: props.brandId, id: recharge.id, ...body }),
      ),
    "充值单已取消。",
    () => cancelKeyFor.clear(),
  );
  if (!error.value) cancelReasonById.value[recharge.id] = "";
}
async function freezePoints() {
  if (
    !loadedMemberId.value ||
    !isPositiveAmount(freezeForm.value.points) ||
    !freezeForm.value.reason.trim()
  )
    return;
  const body = {
    points: freezeForm.value.points,
    reason: freezeForm.value.reason.trim(),
  };
  await mutate(
    () =>
      api.freezeWallet(
        props.brandId,
        loadedMemberId.value,
        body,
        freezeKeyFor({
          brand_id: props.brandId,
          member_id: loadedMemberId.value,
          ...body,
        }),
      ),
    "人工冻结请求已成功，正在读取最新账本。",
    () => freezeKeyFor.clear(),
  );
  if (!error.value) freezeForm.value = { points: "", reason: "" };
}
async function unfreeze(entry: LedgerEntry) {
  if (!loadedMemberId.value || !unfreezeReason.value.trim()) return;
  const body = { entry_id: entry.id, reason: unfreezeReason.value.trim() };
  await mutate(
    () =>
      api.unfreezeWallet(
        props.brandId,
        loadedMemberId.value,
        body,
        unfreezeKeyFor({
          brand_id: props.brandId,
          member_id: loadedMemberId.value,
          ...body,
        }),
      ),
    "整笔原路解冻请求已成功，正在读取最新账本。",
    () => unfreezeKeyFor.clear(),
  );
  if (!error.value) unfreezeReason.value = "";
}
async function adjustPoints() {
  if (
    !loadedMemberId.value ||
    !isSignedAmount(adjustForm.value.delta) ||
    !adjustForm.value.reason.trim()
  )
    return;
  const body = {
    source: adjustForm.value.source,
    delta: adjustForm.value.delta,
    reason: adjustForm.value.reason.trim(),
  };
  await mutate(
    () =>
      api.adjustWallet(
        props.brandId,
        loadedMemberId.value,
        body,
        adjustKeyFor({
          brand_id: props.brandId,
          member_id: loadedMemberId.value,
          ...body,
        }),
      ),
    "积分调整请求已成功，正在读取最新账本。",
    () => adjustKeyFor.clear(),
  );
  if (!error.value)
    adjustForm.value = { source: "recharge", delta: "", reason: "" };
}
function sourceSnapshotValue(
  entry: LedgerEntry,
  part: "before_snapshot" | "delta_snapshot" | "after_snapshot",
  source: WalletSource,
  state: WalletState,
) {
  return formatIntegerAmount(entry[part][source][state]);
}
</script>

<template>
  <section class="finance-management">
    <header class="page-head">
      <div>
        <p class="eyebrow">FINANCE OPERATIONS</p>
        <h1>钱包与财务</h1>
        <p>账本金额以整数点数读取；所有变更由服务端授权、校验并记录审计。</p>
      </div>
      <span class="brand-tag">品牌 · {{ brandId || "未选择" }}</span>
    </header>
    <div class="server-note">
      页面权限仅用于显示操作入口，服务端权限校验始终为最终依据。超级管理员在此只读。
    </div>
    <PointPolicySettings
      v-if="canViewPolicy"
      :account="account"
      :brand-id="brandId"
      @session-invalid="emit('session-invalid')"
    />
    <form
      v-if="canSearch || writeRecharge || writeFreeze || writeAdjust"
      class="lookup"
      @submit.prevent="inspectMember"
    >
      <label
        >会员 UUID<input
          v-model.trim="memberId"
          required
          autocomplete="off"
          placeholder="输入当前品牌的 member_id" /></label
      ><button class="primary" :disabled="loading || !memberId.trim()">
        {{ loading ? "读取中…" : "查询会员" }}
      </button>
    </form>
    <p
      v-if="!canSearch && !(writeRecharge || writeFreeze || writeAdjust)"
      class="callout"
    >
      当前账号没有此品牌的钱包或充值查看权限。
    </p>
    <p v-if="error" class="notice error" role="alert">
      {{ error }}
      <button
        type="button"
        @click="loadedMemberId ? reloadData() : loadRechargeQueue()"
      >
        刷新
      </button>
    </p>
    <p v-if="notice" class="notice success" role="status">{{ notice }}</p>
    <p v-if="walletError" class="notice error" role="alert">
      {{ walletError }}
      <button type="button" @click="reloadData">刷新钱包</button>
    </p>
    <p v-if="rechargeError" class="notice error" role="alert">
      {{ rechargeError }}
      <button
        type="button"
        @click="loadedMemberId ? reloadData() : loadRechargeQueue()"
      >
        刷新充值单
      </button>
    </p>

    <template v-if="loadedMemberId">
      <section v-if="viewWallet && wallet" class="panel wallet-panel">
        <div class="panel-title">
          <div>
            <h2>会员钱包</h2>
            <p>
              {{ wallet.member_id }} · 账户 {{ wallet.account_id }} · 版本
              {{ wallet.version }}
            </p>
          </div>
          <button
            type="button"
            class="secondary"
            :disabled="loading"
            @click="reloadData"
          >
            刷新
          </button>
        </div>
        <div class="totals">
          <article class="total prominent">
            <span>显示积分</span
            ><strong>{{ formatIntegerAmount(wallet.display_points) }}</strong>
          </article>
          <article class="total">
            <span>可用</span
            ><strong>{{ formatIntegerAmount(wallet.available_points) }}</strong>
          </article>
          <article class="total">
            <span>冻结</span
            ><strong>{{ formatIntegerAmount(wallet.frozen_points) }}</strong>
          </article>
          <article class="total">
            <span>提现中</span
            ><strong>{{
              formatIntegerAmount(wallet.withdrawal_points)
            }}</strong>
          </article>
        </div>
        <div class="bucket-table">
          <div class="table-head">
            <span>来源</span><span>状态</span><span>积分</span>
          </div>
          <div
            v-for="bucket in walletBucketRows"
            :key="`${bucket.source}-${bucket.state}`"
            class="bucket-row"
          >
            <b>{{ sourceNames[bucket.source] }}</b
            ><span>{{ stateNames[bucket.state] }}</span
            ><strong>{{ formatIntegerAmount(bucket.value) }}</strong>
          </div>
        </div>
      </section>
      <p v-else-if="viewWallet && !wallet && !walletError" class="callout">
        正在读取会员钱包…
      </p>
      <p v-else-if="!viewWallet" class="callout">
        缺少 wallet.view 权限；不会请求或展示该会员钱包、流水与对账。
      </p>

      <section v-if="viewWallet && reconciliation" class="panel">
        <div class="panel-title">
          <div>
            <h2>账本对账</h2>
            <p>
              版本 {{ reconciliation.version }} ·
              {{ reconciliation.entry_count }} 条流水
            </p>
          </div>
          <span
            class="status"
            :class="reconciliation.consistent ? 'good' : 'bad'"
            >{{ reconciliation.consistent ? "一致" : "存在差异" }}</span
          >
        </div>
        <ul v-if="reconciliation.issues.length" class="issues">
          <li v-for="(issue, index) in reconciliation.issues" :key="index">
            {{ issue }}
          </li>
        </ul>
        <div class="recon-table">
          <div class="table-head">
            <span>来源 / 状态</span><span>期望</span><span>实际</span>
          </div>
          <div
            v-for="row in reconciliationRows"
            :key="`${row.source}-${row.state}`"
            class="recon-row"
          >
            <b>{{ sourceNames[row.source] }} · {{ stateNames[row.state] }}</b
            ><span>{{ formatIntegerAmount(row.expected) }}</span
            ><span>{{ formatIntegerAmount(row.actual) }}</span>
          </div>
        </div>
      </section>

      <section v-if="viewWallet" class="panel">
        <div class="panel-title">
          <div>
            <h2>钱包流水</h2>
            <p>每笔展示全部 12 桶变更前、变动额与变更后金额。</p>
          </div>
        </div>
        <p v-if="!entries.length" class="muted">当前页没有流水。</p>
        <article v-for="entry in entries" :key="entry.id" class="ledger-entry">
          <div class="entry-head">
            <div>
              <b>{{ entry.entry_type }}</b
              ><small>{{ entry.created_at }} · 版本 {{ entry.version }}</small>
            </div>
            <code>{{ entry.id }}</code>
          </div>
          <p class="reason-text">{{ entry.reason }}</p>
          <p v-if="entry.source_allocation.length" class="allocation">
            分摊：{{
              entry.source_allocation
                .map(
                  (item) =>
                    `${sourceNames[item.source]} / ${stateNames[item.state]} ${formatIntegerAmount(item.points)}`,
                )
                .join(" · ")
            }}
          </p>
          <div class="snapshot-grid">
            <span class="snapshot-heading">积分桶</span
            ><span class="snapshot-heading">变更前</span
            ><span class="snapshot-heading">变动</span
            ><span class="snapshot-heading">变更后</span
            ><template v-for="source in sources" :key="`${entry.id}-${source}`"
              ><template
                v-for="state in states"
                :key="`${entry.id}-${source}-${state}`"
                ><b>{{ sourceNames[source] }} · {{ stateNames[state] }}</b
                ><span>{{
                  sourceSnapshotValue(entry, "before_snapshot", source, state)
                }}</span
                ><span>{{
                  sourceSnapshotValue(entry, "delta_snapshot", source, state)
                }}</span
                ><span>{{
                  sourceSnapshotValue(entry, "after_snapshot", source, state)
                }}</span></template
              ></template
            >
          </div>
          <details>
            <summary>审计与引用信息</summary>
            <p>
              引用：{{ entry.reference_type }} / {{ entry.reference_id
              }}<br />操作键：{{ entry.operation_key }}<br />操作者：{{
                entry.actor_type
              }}
              / {{ entry.actor_id }}<br />请求：{{ entry.request_id
              }}<span v-if="entry.reversal_of"
                ><br />冲正原流水：{{ entry.reversal_of }}</span
              >
            </p>
          </details>
          <div
            v-if="
              writeFreeze &&
              unfreezeCandidates.some((item) => item.id === entry.id)
            "
            class="unfreeze-row"
          >
            <label
              >整笔原路解冻原因<input
                v-model.trim="unfreezeReason"
                required
                maxlength="500" /></label
            ><button
              type="button"
              class="secondary"
              :disabled="mutating || !unfreezeReason.trim()"
              @click="unfreeze(entry)"
            >
              整笔原路解冻
            </button>
          </div>
        </article>
        <footer class="pager">
          <span>第 {{ ledgerOffset / PAGE_SIZE + 1 }} 页</span>
          <div>
            <button
              type="button"
              :disabled="ledgerOffset === 0 || loading"
              @click="moveLedgerPage(-1)"
            >
              上一页</button
            ><button
              type="button"
              :disabled="!hasNextLedger || loading"
              @click="moveLedgerPage(1)"
            >
              下一页
            </button>
          </div>
        </footer>
      </section>
    </template>

    <section v-if="viewRecharges" class="panel">
      <div class="panel-title">
        <div>
          <h2>充值单</h2>
          <p>
            {{ loadedMemberId ? `会员 ${loadedMemberId}` : "当前品牌充值单" }} ·
            每页 50 条
          </p>
        </div>
        <button
          v-if="!loadedMemberId"
          type="button"
          class="secondary"
          :disabled="loading"
          @click="loadRechargeQueue"
        >
          刷新列表
        </button>
      </div>
      <p v-if="!recharges.length" class="muted">当前页没有充值单。</p>
      <article
        v-for="recharge in recharges"
        :key="recharge.id"
        class="recharge-row"
      >
        <div class="recharge-head">
          <div>
            <b>{{ formatIntegerAmount(recharge.points) }} 分</b
            ><small>{{ recharge.member_id }} · {{ recharge.created_at }}</small>
          </div>
          <span class="status" :class="recharge.state">{{
            recharge.state === "pending"
              ? "待确认"
              : recharge.state === "confirmed"
                ? "已确认"
                : "已取消"
          }}</span>
        </div>
        <p v-if="recharge.proof_reference">
          凭证：{{ recharge.proof_reference }}
        </p>
        <p v-if="recharge.remark">备注：{{ recharge.remark }}</p>
        <p>
          创建人 {{ recharge.created_by
          }}<span v-if="recharge.confirmed_by">
            · 确认人 {{ recharge.confirmed_by }}</span
          ><span v-if="recharge.ledger_entry_id">
            · 流水 {{ recharge.ledger_entry_id }}</span
          >
        </p>
        <label v-if="writeRecharge && recharge.state === 'pending'"
          >确认原因<input
            v-model.trim="rechargeReasonById[recharge.id]"
            required
            maxlength="500" /></label
        ><button
          v-if="writeRecharge && recharge.state === 'pending'"
          type="button"
          class="primary small-button"
          :disabled="mutating || !rechargeReasonById[recharge.id]?.trim()"
          @click="confirmRecharge(recharge)"
        >
          确认并入账
        </button>
        <label v-if="writeRecharge && recharge.state === 'pending'"
          >取消原因<input
            v-model.trim="cancelReasonById[recharge.id]"
            required
            maxlength="500" /></label
        ><button
          v-if="writeRecharge && recharge.state === 'pending'"
          type="button"
          class="secondary small-button"
          :disabled="mutating || !cancelReasonById[recharge.id]?.trim()"
          @click="cancelRecharge(recharge)"
        >
          取消充值单
        </button>
      </article>
      <footer class="pager">
        <span>第 {{ rechargeOffset / PAGE_SIZE + 1 }} 页</span>
        <div>
          <button
            type="button"
            :disabled="rechargeOffset === 0 || loading"
            @click="moveRechargePage(-1)"
          >
            上一页</button
          ><button
            type="button"
            :disabled="!hasNextRecharge || loading"
            @click="moveRechargePage(1)"
          >
            下一页
          </button>
        </div>
      </footer>
    </section>

    <section v-if="writeRecharge && loadedMemberId" class="panel form-panel">
      <div class="panel-title">
        <div>
          <h2>创建充值单</h2>
          <p>这里只创建待确认充值单；确认后服务端才执行真实入账。</p>
        </div>
      </div>
      <form class="form-grid" @submit.prevent="createRecharge">
        <label
          >充值积分（正整数）<input
            v-model.trim="rechargeForm.points"
            inputmode="numeric"
            required
          /><small>大额金额按十进制字符串提交，不经浮点数转换。</small></label
        ><label
          >凭证编号（可选）<input
            v-model.trim="rechargeForm.proof_reference"
            maxlength="200" /></label
        ><label class="wide"
          >备注（可选）<input
            v-model.trim="rechargeForm.remark"
            maxlength="500" /></label
        ><label class="wide"
          >原因<input
            v-model.trim="rechargeForm.reason"
            required
            maxlength="500" /></label
        ><button
          class="primary"
          :disabled="
            mutating ||
            !isPositiveAmount(rechargeForm.points) ||
            !rechargeForm.reason.trim()
          "
        >
          创建待确认充值单
        </button>
      </form>
    </section>

    <section
      v-if="loadedMemberId && (writeFreeze || writeAdjust)"
      class="operations-grid"
    >
      <form
        v-if="writeFreeze"
        class="panel form-panel"
        @submit.prevent="freezePoints"
      >
        <div class="panel-title">
          <div>
            <h2>人工冻结</h2>
            <p>只冻结指定积分；系统冻结和投注冻结不提供操作入口。</p>
          </div>
        </div>
        <label
          >冻结积分（正整数）<input
            v-model.trim="freezeForm.points"
            inputmode="numeric"
            required /></label
        ><label
          >原因<input
            v-model.trim="freezeForm.reason"
            required
            maxlength="500" /></label
        ><button
          class="primary"
          :disabled="
            mutating ||
            !isPositiveAmount(freezeForm.points) ||
            !freezeForm.reason.trim()
          "
        >
          冻结积分
        </button>
      </form>
      <form
        v-if="writeAdjust"
        class="panel form-panel"
        @submit.prevent="adjustPoints"
      >
        <div class="panel-title">
          <div>
            <h2>来源积分调整</h2>
            <p>只允许充值、中奖或赠送来源；没有通用冲正或系统冻结入口。</p>
          </div>
        </div>
        <label
          >积分来源<select v-model="adjustForm.source">
            <option value="recharge">充值</option>
            <option value="winning">中奖</option>
            <option value="gift">赠送</option>
          </select></label
        ><label
          >调整额（非零有符号整数）<input
            v-model.trim="adjustForm.delta"
            inputmode="text"
            required
            placeholder="例如 250 或 -30" /></label
        ><label
          >原因<input
            v-model.trim="adjustForm.reason"
            required
            maxlength="500" /></label
        ><button
          class="primary"
          :disabled="
            mutating ||
            !isSignedAmount(adjustForm.delta) ||
            !adjustForm.reason.trim()
          "
        >
          提交来源调整
        </button>
      </form>
    </section>
    <p
      v-if="loadedMemberId && !(writeRecharge || writeFreeze || writeAdjust)"
      class="callout"
    >
      当前账号在此品牌只有财务查看权限；不会显示写操作。
    </p>
  </section>
</template>

<style scoped>
.finance-management {
  width: 100%;
  min-width: 0;
  color: #252a36;
}
.page-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 14px;
  margin: 4px 0 16px;
}
.page-head h1 {
  font-size: 24px;
  margin: 3px 0 5px;
  letter-spacing: -0.5px;
}
.page-head p {
  margin: 0;
  color: #737b8c;
  font-size: 11px;
  line-height: 1.55;
}
.eyebrow {
  font-size: 10px !important;
  letter-spacing: 1.2px;
  color: #8991a2 !important;
  font-weight: 700;
}
.brand-tag,
.status {
  border-radius: 20px;
  background: #f1f3f8;
  padding: 7px 11px;
  color: #626b7c;
  font-size: 11px;
  white-space: nowrap;
}
.server-note,
.callout {
  border-radius: 8px;
  background: #f4f6fc;
  color: #626b7c;
  padding: 10px 12px;
  font-size: 11px;
  line-height: 1.6;
  overflow-wrap: anywhere;
  margin: 12px 0;
}
.lookup {
  display: flex;
  align-items: flex-end;
  gap: 10px;
  margin: 14px 0;
}
.lookup label,
.form-grid label,
.form-panel > label,
.recharge-row label,
.unfreeze-row label {
  display: grid;
  gap: 6px;
  font-size: 11px;
  font-weight: 600;
  min-width: 0;
}
.lookup label {
  flex: 1;
}
.lookup input,
.form-grid input,
.form-panel input,
.recharge-row input,
.unfreeze-row input,
.form-grid select,
.form-panel select {
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  border: 1px solid #dfe3eb;
  border-radius: 7px;
  background: white;
  padding: 9px 10px;
  color: #252a36;
  font: inherit;
}
.primary,
.secondary {
  border-radius: 7px;
  padding: 9px 12px;
  font-size: 11px;
  font-weight: 700;
  white-space: nowrap;
}
.primary {
  border: 0;
  background: #5969df;
  color: #fff;
}
.secondary {
  border: 1px solid #e0e3ea;
  background: white;
  color: #586174;
}
.primary:disabled,
.secondary:disabled {
  opacity: 0.48;
  cursor: not-allowed;
}
.notice {
  border-radius: 8px;
  padding: 10px 12px;
  font-size: 11px;
  line-height: 1.5;
  overflow-wrap: anywhere;
  margin: 10px 0;
}
.notice button {
  border: 0;
  background: none;
  color: inherit;
  text-decoration: underline;
  margin-left: 6px;
}
.error {
  background: #fff0ef;
  color: #a83d36;
}
.success {
  background: #edf8f2;
  color: #287553;
}
.panel {
  background: white;
  border: 1px solid #e9ebf0;
  border-radius: 11px;
  padding: 16px;
  min-width: 0;
  box-shadow: 0 2px 8px #1e2a5008;
  margin: 13px 0;
}
.panel-title {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 13px;
}
.panel-title h2 {
  font-size: 14px;
  margin: 0 0 4px;
}
.panel-title p,
.muted {
  font-size: 10px;
  line-height: 1.55;
  color: #737b8c;
  margin: 0;
}
.totals {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 9px;
  margin-bottom: 14px;
}
.total {
  display: grid;
  gap: 6px;
  border: 1px solid #eceef3;
  border-radius: 8px;
  padding: 11px;
  min-width: 0;
}
.total span {
  font-size: 10px;
  color: #737b8c;
}
.total strong {
  font-size: 18px;
  overflow-wrap: anywhere;
  font-variant-numeric: tabular-nums;
}
.total.prominent {
  background: #f1f3ff;
}
.bucket-table,
.recon-table {
  display: grid;
  grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr) minmax(0, 1fr);
  font-size: 10px;
}
.table-head,
.bucket-row,
.recon-row {
  display: contents;
}
.table-head span {
  background: #f7f8fa;
  padding: 8px;
  font-weight: 700;
  color: #687183;
}
.bucket-row > *,
.recon-row > * {
  padding: 8px;
  border-bottom: 1px solid #f0f1f4;
  min-width: 0;
  overflow-wrap: anywhere;
}
.bucket-row strong,
.recon-row span {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.recon-row b {
  font-weight: 500;
}
.status.good,
.status.confirmed {
  background: #e9f6f0;
  color: #258763;
}
.status.bad,
.status.cancelled {
  background: #fff0ef;
  color: #a83d36;
}
.status.pending {
  background: #fff6e7;
  color: #a66a13;
}
.issues {
  background: #fff7e9;
  color: #875b1b;
  font-size: 11px;
  line-height: 1.6;
  border-radius: 7px;
  padding: 10px 10px 10px 28px;
}
.ledger-entry,
.recharge-row {
  border-top: 1px solid #eff0f4;
  padding: 13px 0;
  min-width: 0;
}
.entry-head,
.recharge-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  font-size: 11px;
}
.entry-head > div,
.recharge-head > div {
  display: grid;
  gap: 4px;
  min-width: 0;
}
.entry-head small,
.recharge-head small {
  font-size: 10px;
  color: #8991a0;
  overflow-wrap: anywhere;
}
.entry-head code {
  font-size: 9px;
  color: #8b92a0;
  overflow-wrap: anywhere;
}
.reason-text,
.allocation,
.recharge-row > p {
  font-size: 10px;
  color: #626b7c;
  overflow-wrap: anywhere;
  line-height: 1.55;
}
.snapshot-grid {
  display: grid;
  grid-template-columns: minmax(120px, 1.2fr) repeat(3, minmax(0, 1fr));
  gap: 6px 8px;
  align-items: baseline;
  font-size: 9px;
  font-variant-numeric: tabular-nums;
}
.snapshot-grid > b {
  font-weight: 500;
  color: #626b7c;
  overflow-wrap: anywhere;
}
.snapshot-grid > span {
  text-align: right;
  overflow-wrap: anywhere;
}
.snapshot-heading {
  font-size: 9px;
  font-weight: 700;
  color: #858c99;
}
.ledger-entry details {
  font-size: 10px;
  color: #737b8c;
  margin-top: 9px;
  overflow-wrap: anywhere;
}
.ledger-entry details p {
  line-height: 1.7;
}
.unfreeze-row {
  display: flex;
  align-items: flex-end;
  gap: 8px;
  margin-top: 10px;
}
.unfreeze-row label {
  flex: 1;
}
.recharge-row label {
  margin: 8px 0;
}
.small-button {
  padding: 7px 10px;
}
.pager {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-top: 12px;
  color: #737b8c;
  font-size: 10px;
}
.pager > div {
  display: flex;
  gap: 7px;
}
.pager button {
  border: 1px solid #e1e4eb;
  border-radius: 6px;
  background: white;
  padding: 6px 9px;
  color: #50596a;
  font-size: 10px;
}
.pager button:disabled {
  opacity: 0.45;
}
.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}
.form-grid small {
  font-size: 9px;
  color: #8991a0;
  font-weight: 400;
}
.form-grid .wide {
  grid-column: 1/-1;
}
.form-grid .primary {
  justify-self: start;
}
.form-panel > label {
  margin: 10px 0;
}
.operations-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 13px;
}
.operations-grid .panel {
  margin: 0;
}
.operations-grid .primary {
  margin-top: 12px;
}
.callout {
  margin: 12px 0;
}
@media (max-width: 700px) {
  .page-head {
    flex-direction: column;
  }
  .brand-tag {
    white-space: normal;
  }
  .lookup {
    align-items: stretch;
    flex-direction: column;
  }
  .lookup label {
    width: 100%;
  }
  .totals {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .panel {
    padding: 13px;
  }
  .operations-grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .snapshot-grid {
    grid-template-columns: minmax(94px, 1fr) repeat(3, minmax(0, 0.8fr));
    gap: 6px 4px;
    font-size: 8px;
  }
  .snapshot-heading {
    font-size: 8px;
  }
  .unfreeze-row {
    align-items: stretch;
    flex-direction: column;
  }
  .unfreeze-row button {
    align-self: flex-start;
  }
  .bucket-table,
  .recon-table {
    font-size: 9px;
  }
}
</style>
