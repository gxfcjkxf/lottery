<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import type { LocalizedMessage } from "@lottery/shared";
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
import { useAdminI18n } from "./i18n";

const props = defineProps<{
  account: AdminAccount & Partial<FinanceAccount>;
  brandId: string;
}>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, message } = useAdminI18n();
const PAGE_SIZE = 50;
const api = createFinanceApi();
const sources: WalletSource[] = ["recharge", "winning", "gift"];
const states: WalletState[] = [
  "available",
  "manual_frozen",
  "system_frozen",
  "withdrawal",
];
function sourceName(source: WalletSource) {
  return source === "recharge"
    ? t("充值", "Recharge")
    : source === "winning"
      ? t("中奖", "Winnings")
      : source === "gift" ? t("赠送", "Gift") : source;
}
function stateName(state: WalletState) {
  return state === "available"
    ? t("可用", "Available")
    : state === "manual_frozen"
      ? t("人工冻结", "Manually frozen")
      : state === "system_frozen"
        ? t("系统冻结", "System frozen")
        : state === "withdrawal" ? t("提现中", "Withdrawal pending") : state;
}
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
const error = ref<string | LocalizedMessage>("");
const walletError = ref<string | LocalizedMessage>("");
const rechargeError = ref<string | LocalizedMessage>("");
const notice = ref<string | LocalizedMessage>("");
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

function messageFor(cause: unknown): string | LocalizedMessage {
  if (cause instanceof FinanceApiError) {
    if (cause.status === 409)
      return message(
        "{serverError}；数据版本已变化，请刷新后重试。",
        "{serverError} The data version changed; refresh and try again.",
        { serverError: cause.message },
      );
    if (cause.status === 403)
      return message(
        "{serverError}（服务端权限校验仍为最终依据。）",
        "{serverError} Server authorization remains authoritative.",
        { serverError: cause.message },
      );
    return cause.message;
  }
  return cause instanceof Error
    ? cause.message
    : message("请求失败，请重试。", "Request failed. Please try again.");
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
          else
            walletError.value = message(
              "钱包响应与当前品牌或会员不匹配。",
              "The wallet response does not match the current brand or member.",
            );
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
          else
            walletError.value = message(
              "流水响应与当前品牌或会员不匹配。",
              "The ledger response does not match the current brand or member.",
            );
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
          else
            rechargeError.value = message(
              "充值单响应与当前品牌或会员不匹配。",
              "The recharge response does not match the current brand or member.",
            );
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
  successText: string | LocalizedMessage,
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
    message("充值单已创建；实际入账须确认后完成。", "Recharge created. Confirm it to complete the credit."),
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
    message("充值单已确认，正在读取最新钱包状态。", "Recharge confirmed. Loading the latest wallet state."),
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
    message("充值单已取消。", "Recharge cancelled."),
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
    message("人工冻结请求已成功，正在读取最新账本。", "Manual freeze request succeeded. Loading the latest ledger."),
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
    message("整笔原路解冻请求已成功，正在读取最新账本。", "Full reversal unfreeze request succeeded. Loading the latest ledger."),
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
    message("积分调整请求已成功，正在读取最新账本。", "Points adjustment succeeded. Loading the latest ledger."),
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
        <p class="eyebrow">{{ t("财务操作", "FINANCE OPERATIONS") }}</p>
        <h1>{{ t("钱包与财务", "Funds and ledger") }}</h1>
        <p>{{ t("账本金额以整数点数读取；所有变更由服务端授权、校验并记录审计。", "Ledger amounts are read as whole points. The server authorizes, validates, and audits every change.") }}</p>
      </div>
      <span class="brand-tag">{{ t("品牌 · ", "Brand · ") }}{{ brandId || t("未选择", "Not selected") }}</span>
    </header>
    <div class="server-note">
      {{ t("页面权限仅用于显示操作入口，服务端权限校验始终为最终依据。超级管理员在此只读。", "Page permissions only control which actions are shown; server-side authorization is authoritative. Super administrators have read-only access here.") }}
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
        >{{ t("会员 UUID", "Member ID") }}<input
          v-model.trim="memberId"
          required
          autocomplete="off"
          :placeholder="t('输入当前品牌的 member_id', 'Enter the member_id for this brand')" /></label
      ><button class="primary" :disabled="loading || !memberId.trim()">
        {{ loading ? t("读取中…", "Loading…") : t("查询会员", "Load wallet") }}
      </button>
    </form>
    <p
      v-if="!canSearch && !(writeRecharge || writeFreeze || writeAdjust)"
      class="callout"
    >
      {{ t("当前账号没有此品牌的钱包或充值查看权限。", "This account cannot view wallets or recharges for this brand.") }}
    </p>
    <p v-if="error" class="notice error" role="alert">
      {{ t(error) }}
      <button
        type="button"
        @click="loadedMemberId ? reloadData() : loadRechargeQueue()"
        >
        {{ t("刷新", "Refresh") }}
      </button>
    </p>
    <p v-if="notice" class="notice success" role="status">{{ t(notice) }}</p>
    <p v-if="walletError" class="notice error" role="alert">
      {{ t(walletError) }}
      <button type="button" @click="reloadData">{{ t("刷新钱包", "Refresh wallet") }}</button>
    </p>
    <p v-if="rechargeError" class="notice error" role="alert">
      {{ t(rechargeError) }}
      <button
        type="button"
        @click="loadedMemberId ? reloadData() : loadRechargeQueue()"
      >
        {{ t("刷新充值单", "Refresh recharges") }}
      </button>
    </p>

    <template v-if="loadedMemberId">
      <section v-if="viewWallet && wallet" class="panel wallet-panel">
        <div class="panel-title">
          <div>
            <h2>{{ t("会员钱包", "Member wallet") }}</h2>
            <p>
              {{ t("会员 ID", "Member ID") }} {{ wallet.member_id }} · {{ t("账户", "Account") }} {{ wallet.account_id }} · {{ t("版本", "Version") }}
              {{ wallet.version }}
            </p>
          </div>
          <button
            type="button"
            class="secondary"
            :disabled="loading"
            @click="reloadData"
          >
            {{ t("刷新", "Refresh") }}
          </button>
        </div>
        <div class="totals">
          <article class="total prominent">
            <span>{{ t("显示积分", "Displayed points") }}</span
            ><strong>{{ formatIntegerAmount(wallet.display_points) }}</strong>
          </article>
          <article class="total">
            <span>{{ t("可用", "Available") }}</span
            ><strong>{{ formatIntegerAmount(wallet.available_points) }}</strong>
          </article>
          <article class="total">
            <span>{{ t("冻结", "Frozen") }}</span
            ><strong>{{ formatIntegerAmount(wallet.frozen_points) }}</strong>
          </article>
          <article class="total">
            <span>{{ t("提现中", "Withdrawal pending") }}</span
            ><strong>{{
              formatIntegerAmount(wallet.withdrawal_points)
            }}</strong>
          </article>
        </div>
        <div class="bucket-table">
          <div class="table-head">
            <span>{{ t("来源", "Source") }}</span><span>{{ t("状态", "State") }}</span><span>{{ t("积分", "Points") }}</span>
          </div>
          <div
            v-for="bucket in walletBucketRows"
            :key="`${bucket.source}-${bucket.state}`"
            class="bucket-row"
          >
            <b>{{ sourceName(bucket.source) }}</b
            ><span>{{ stateName(bucket.state) }}</span
            ><strong>{{ formatIntegerAmount(bucket.value) }}</strong>
          </div>
        </div>
      </section>
      <p v-else-if="viewWallet && !wallet && !walletError" class="callout">
        {{ t("正在读取会员钱包…", "Loading member wallet…") }}
      </p>
      <p v-else-if="!viewWallet" class="callout">
        {{ t("缺少 wallet.view 权限；不会请求或展示该会员钱包、流水与对账。", "Missing wallet.view permission. This member's wallet, ledger, and reconciliation will not be requested or shown.") }}
      </p>

      <section v-if="viewWallet && reconciliation" class="panel">
        <div class="panel-title">
          <div>
            <h2>{{ t("账本对账", "Ledger reconciliation") }}</h2>
            <p>
              {{ t("版本", "Version") }} {{ reconciliation.version }} ·
              {{ t("{count} 条流水", "{count} ledger entries", { count: String(reconciliation.entry_count) }) }}
            </p>
          </div>
          <span
            class="status"
            :class="reconciliation.consistent ? 'good' : 'bad'"
            >{{ reconciliation.consistent ? t("一致", "Consistent") : t("存在差异", "Discrepancies found") }}</span
          >
        </div>
        <ul v-if="reconciliation.issues.length" class="issues">
          <li v-for="(issue, index) in reconciliation.issues" :key="index">
            {{ issue }}
          </li>
        </ul>
        <div class="recon-table">
          <div class="table-head">
            <span>{{ t("来源 / 状态", "Source / state") }}</span><span>{{ t("期望", "Expected") }}</span><span>{{ t("实际", "Actual") }}</span>
          </div>
          <div
            v-for="row in reconciliationRows"
            :key="`${row.source}-${row.state}`"
            class="recon-row"
          >
            <b>{{ sourceName(row.source) }} · {{ stateName(row.state) }}</b
            ><span>{{ formatIntegerAmount(row.expected) }}</span
            ><span>{{ formatIntegerAmount(row.actual) }}</span>
          </div>
        </div>
      </section>

      <section v-if="viewWallet" class="panel">
        <div class="panel-title">
          <div>
            <h2>{{ t("钱包流水", "Wallet ledger") }}</h2>
            <p>{{ t("每笔展示全部 12 桶变更前、变动额与变更后金额。", "Each entry shows the before, change, and after amounts for all 12 buckets.") }}</p>
          </div>
        </div>
        <p v-if="!entries.length" class="muted">{{ t("当前页没有流水。", "No ledger entries on this page.") }}</p>
        <article v-for="entry in entries" :key="entry.id" class="ledger-entry">
          <div class="entry-head">
            <div>
              <b>{{ entry.entry_type }}</b
              ><small>{{ entry.created_at }} · {{ t("版本", "Version") }} {{ entry.version }}</small>
            </div>
            <code>{{ entry.id }}</code>
          </div>
          <p class="reason-text">{{ entry.reason }}</p>
          <p v-if="entry.source_allocation.length" class="allocation">
            {{ t("分摊：", "Allocation: ") }}{{
              entry.source_allocation
                .map(
                  (item) =>
                    `${sourceName(item.source)} / ${stateName(item.state)} ${formatIntegerAmount(item.points)}`,
                )
                .join(" · ")
            }}
          </p>
          <div class="snapshot-grid">
          <span class="snapshot-heading">{{ t("积分桶", "Points bucket") }}</span
            ><span class="snapshot-heading">{{ t("变更前", "Before") }}</span
            ><span class="snapshot-heading">{{ t("变动", "Change") }}</span
            ><span class="snapshot-heading">{{ t("变更后", "After") }}</span
            ><template v-for="source in sources" :key="`${entry.id}-${source}`"
              ><template
                v-for="state in states"
                :key="`${entry.id}-${source}-${state}`"
                ><b>{{ sourceName(source) }} · {{ stateName(state) }}</b
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
            <summary>{{ t("审计与引用信息", "Audit and reference details") }}</summary>
            <p>
              {{ t("引用：", "Reference: ") }}{{ entry.reference_type }} / {{ entry.reference_id
              }}<br />{{ t("操作键：", "Operation key: ") }}{{ entry.operation_key }}<br />{{ t("操作者：", "Actor: ") }}{{
                entry.actor_type
              }}
              / {{ entry.actor_id }}<br />{{ t("请求：", "Request: ") }}{{ entry.request_id
              }}<span v-if="entry.reversal_of"
                ><br />{{ t("冲正原流水：", "Reversed ledger entry: ") }}{{ entry.reversal_of }}</span
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
              >{{ t("整笔原路解冻原因", "Reason for full reversal unfreeze") }}<input
                v-model.trim="unfreezeReason"
                required
                maxlength="500" /></label
            ><button
              type="button"
              class="secondary"
              :disabled="mutating || !unfreezeReason.trim()"
              @click="unfreeze(entry)"
            >
              {{ t("整笔原路解冻", "Unfreeze entire original amount") }}
            </button>
          </div>
        </article>
        <footer class="pager">
          <span>{{ t("第 {page} 页", "Page {page}", { page: String(ledgerOffset / PAGE_SIZE + 1) }) }}</span>
          <div>
            <button
              type="button"
              :disabled="ledgerOffset === 0 || loading"
              @click="moveLedgerPage(-1)"
            >
              {{ t("上一页", "Previous") }}</button
            ><button
              type="button"
              :disabled="!hasNextLedger || loading"
              @click="moveLedgerPage(1)"
            >
              {{ t("下一页", "Next") }}
            </button>
          </div>
        </footer>
      </section>
    </template>

    <section v-if="viewRecharges" class="panel">
      <div class="panel-title">
        <div>
          <h2>{{ t("充值单", "Recharges") }}</h2>
          <p>
            {{ loadedMemberId ? `${t("会员", "Member")} ${loadedMemberId}` : t("当前品牌充值单", "Brand recharges") }} ·
            {{ t("每页 {count} 条", "{count} per page", { count: "50" }) }}
          </p>
        </div>
        <button
          v-if="!loadedMemberId"
          type="button"
          class="secondary"
          :disabled="loading"
          @click="loadRechargeQueue"
        >
          {{ t("刷新列表", "Refresh list") }}
        </button>
      </div>
      <p v-if="!recharges.length" class="muted">{{ t("当前页没有充值单。", "No recharges on this page.") }}</p>
      <article
        v-for="recharge in recharges"
        :key="recharge.id"
        class="recharge-row"
      >
        <div class="recharge-head">
          <div>
            <b>{{ formatIntegerAmount(recharge.points) }} {{ t("分", "points") }}</b
            ><small>{{ recharge.member_id }} · {{ recharge.created_at }}</small>
          </div>
          <span class="status" :class="recharge.state">{{
            recharge.state === "pending"
              ? t("待确认", "Pending")
              : recharge.state === "confirmed"
                ? t("已确认", "Confirmed")
                : t("已取消", "Cancelled")
          }}</span>
        </div>
        <p v-if="recharge.proof_reference">
          {{ t("凭证：", "Proof: ") }}{{ recharge.proof_reference }}
        </p>
        <p v-if="recharge.remark">{{ t("备注：", "Remark: ") }}{{ recharge.remark }}</p>
        <p>
          {{ t("创建人", "Created by") }} {{ recharge.created_by
          }}<span v-if="recharge.confirmed_by">
            · {{ t("确认人", "Confirmed by") }} {{ recharge.confirmed_by }}</span
          ><span v-if="recharge.ledger_entry_id">
            · {{ t("流水", "Ledger entry") }} {{ recharge.ledger_entry_id }}</span
          >
        </p>
        <label v-if="writeRecharge && recharge.state === 'pending'"
          >{{ t("确认原因", "Confirmation reason") }}<input
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
          {{ t("确认并入账", "Confirm and credit") }}
        </button>
        <label v-if="writeRecharge && recharge.state === 'pending'"
          >{{ t("取消原因", "Cancellation reason") }}<input
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
          {{ t("取消充值单", "Cancel recharge") }}
        </button>
      </article>
      <footer class="pager">
        <span>{{ t("第 {page} 页", "Page {page}", { page: String(rechargeOffset / PAGE_SIZE + 1) }) }}</span>
        <div>
          <button
            type="button"
            :disabled="rechargeOffset === 0 || loading"
            @click="moveRechargePage(-1)"
          >
            {{ t("上一页", "Previous") }}</button
          ><button
            type="button"
            :disabled="!hasNextRecharge || loading"
            @click="moveRechargePage(1)"
          >
            {{ t("下一页", "Next") }}
          </button>
        </div>
      </footer>
    </section>

    <section v-if="writeRecharge && loadedMemberId" class="panel form-panel">
      <div class="panel-title">
        <div>
          <h2>{{ t("创建充值单", "Create recharge") }}</h2>
          <p>{{ t("这里只创建待确认充值单；确认后服务端才执行真实入账。", "This creates a pending recharge only. The server credits it after confirmation.") }}</p>
        </div>
      </div>
      <form class="form-grid" @submit.prevent="createRecharge">
        <label
          >{{ t("充值积分（正整数）", "Recharge points (positive whole number)") }}<input
            v-model.trim="rechargeForm.points"
            inputmode="numeric"
            required
          /><small>{{ t("大额金额按十进制字符串提交，不经浮点数转换。", "Large amounts are submitted as decimal strings without floating-point conversion.") }}</small></label
        ><label
          >{{ t("凭证编号（可选）", "Proof reference (optional)") }}<input
            v-model.trim="rechargeForm.proof_reference"
            maxlength="200" /></label
        ><label class="wide"
          >{{ t("备注（可选）", "Remark (optional)") }}<input
            v-model.trim="rechargeForm.remark"
            maxlength="500" /></label
        ><label class="wide"
          >{{ t("原因", "Reason") }}<input
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
          {{ t("创建待确认充值单", "Create pending recharge") }}
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
            <h2>{{ t("人工冻结", "Manual freeze") }}</h2>
            <p>{{ t("只冻结指定积分；系统冻结和投注冻结不提供操作入口。", "Freeze only the specified points. System freezes and betting freezes cannot be changed here.") }}</p>
          </div>
        </div>
        <label
          >{{ t("冻结积分（正整数）", "Points to freeze (positive whole number)") }}<input
            v-model.trim="freezeForm.points"
            inputmode="numeric"
            required /></label
        ><label
          >{{ t("原因", "Reason") }}<input
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
          {{ t("冻结积分", "Freeze points") }}
        </button>
      </form>
      <form
        v-if="writeAdjust"
        class="panel form-panel"
        @submit.prevent="adjustPoints"
      >
        <div class="panel-title">
          <div>
            <h2>{{ t("来源积分调整", "Adjust points by source") }}</h2>
            <p>{{ t("只允许充值、中奖或赠送来源；没有通用冲正或系统冻结入口。", "Only Recharge, Winnings, or Gift sources can be adjusted. There is no general reversal or system-freeze action.") }}</p>
          </div>
        </div>
        <label
          >{{ t("积分来源", "Points source") }}<select v-model="adjustForm.source">
            <option value="recharge">{{ t("充值", "Recharge") }}</option>
            <option value="winning">{{ t("中奖", "Winnings") }}</option>
            <option value="gift">{{ t("赠送", "Gift") }}</option>
          </select></label
        ><label
          >{{ t("调整额（非零有符号整数）", "Adjustment (nonzero signed whole number)") }}<input
            v-model.trim="adjustForm.delta"
            inputmode="text"
            required
            :placeholder="t('例如 250 或 -30', 'For example, 250 or -30')" /></label
        ><label
          >{{ t("原因", "Reason") }}<input
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
          {{ t("提交来源调整", "Submit source adjustment") }}
        </button>
      </form>
    </section>
    <p
      v-if="loadedMemberId && !(writeRecharge || writeFreeze || writeAdjust)"
      class="callout"
    >
      {{ t("当前账号在此品牌只有财务查看权限；不会显示写操作。", "This account has read-only finance access for this brand; write actions are hidden.") }}
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
