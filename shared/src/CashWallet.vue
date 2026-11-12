<script setup lang="ts">
import { computed, onBeforeUnmount, onUnmounted, ref, watch, useId } from "vue";
import {
  ApiError,
  useOperationToast,
  vInlineValidation,
  vWriteGuard,
  withGlobalLoading,
  formatDisplayDateTime,
  formatPoints,
} from "@lottery/shared";
import DetailModal from "@lottery/shared/DetailModal.vue";
import SelectControl from "@lottery/shared/SelectControl.vue";
import ManagementList from "./ManagementList.vue";
import ManagementTable from "./ManagementTable.vue";
import TablePagination from "@lottery/shared/TablePagination.vue";
import CashDestinationFields from "./CashDestinationFields.vue";
import CashAddress from "./CashAddress.vue";
import {
  createCashApi,
  pendingCashWrites,
  currencies,
  emptyDestination,
  exchangeResult,
  minor,
  decimal,
  type Currency,
  type CashState,
  type CashOrder,
  type AgentPointTransfer,
  type AgentWalletRecord,
  type CashDestination,
} from "./cash";
import type { MessageParameters } from "./localization";
import type { CashKey } from "./cash-catalog";
const props = defineProps<{
  apiBase: string;
  locale: "en" | "zh-CN";
  t: (key: CashKey, parameters?:MessageParameters) => string;
  errorText: (cause: unknown) => string;
  agent?: boolean;
  readAvailablePoints: () => Promise<string>;
  member: { id: string; brand_id: string } | null;
  mode: "overview" | "deposit" | "withdrawal";
}>();
const emit = defineEmits<{ "auth-expired": []; "deposit": []; "withdrawal": []; "exchanged": [] }>();
const t = (key: string, parameters?:MessageParameters) => props.t(key as CashKey,parameters);
const errorText = (cause: unknown) => props.errorText(cause);
const label = (k: CashKey) => t(k);
const depositAmountHintId = useId();
const exchangeBalanceHintId = useId();
const availablePoints = ref<string | null>(null);
const api = computed(() =>
  createCashApi(
    props.apiBase,
    undefined,
    data.value?.actor_context,
  ),
);
const data = ref<CashState | null>(null),
  records = ref<CashOrder[]>([]),
  transfers = ref<AgentPointTransfer[]>([]),
  combined = ref<AgentWalletRecord[]>([]),
  total = ref("0"),
  page = ref(1),
  size = ref(20),
  busy = ref(false),
  loading = ref(false),
  error = ref(""),
  notice = ref("");
const currency = ref<Currency>("CNY"),
  amount = ref(""),
  direction = ref<"to_points" | "from_points">("to_points"),
  selectedAccount = ref("");
type WalletDialog =
  | "deposit"
  | "withdrawal"
  | "exchange"
  | "account"
  | "detail"
  | null;
const dialog = ref<WalletDialog>(null),
  detail = ref<CashOrder | null>(null),
  transferDetail = ref<AgentPointTransfer|null>(null),
  destination = ref<CashDestination>(emptyDestination("CNY")),
  proof = ref(""),
  reference = ref("");
const tutorialOpen = ref(false), tutorialVideo = ref<HTMLVideoElement | null>(null);
function closeTutorial() {
  tutorialVideo.value?.pause();
  tutorialOpen.value = false;
}
onBeforeUnmount(closeTutorial);
const returnToWithdrawal = ref(false),
  copied = ref(false);
const pending = ref<import("./cash").PendingCashWrite | null>(
  null,
);
const pendingScope = computed(
  () => `${props.apiBase}:${props.member?.brand_id}:${props.member?.id}`,
);
const clone = <T,>(value: T): T => JSON.parse(JSON.stringify(value));
let generation = 0;
const locked = computed(() => busy.value || !!pending.value);
const enabledCurrencies = computed(() =>
  currencies.filter((value) =>
    data.value?.brand.rules.some((r) => r.currency === value && r.enabled) &&
    data.value?.platform.rules.some((r) => r.currency === value && r.enabled),
  ),
);
const visibleBalances = computed(() =>
  data.value?.balances.filter((b) => enabledCurrencies.value.includes(b.currency)) ?? [],
);
const exchangeDirections = computed(() => props.mode === 'withdrawal'
  ? ['from_points', 'to_points'] as const
  : ['to_points', 'from_points'] as const);
const balance = computed(() =>
  data.value?.balances.find((b) => b.currency === currency.value),
);
const rule = computed(() =>
  data.value?.platform.rules.find(
    (r) => r.currency === currency.value && r.enabled,
  ),
);
const enabled = computed(
  () =>
    !!rule.value &&
    data.value?.brand.rules.some(
      (r) => r.currency === currency.value && r.enabled,
    ),
);
const managedAccounts = computed(
  () =>
    data.value?.destinations.filter(
      (d) => d.currency === currency.value && enabledCurrencies.value.includes(d.currency),
    ) ?? [],
);
const accounts = computed(
  () =>
    data.value?.destinations.filter(
      (d) => d.currency === currency.value && d.enabled,
    ) ?? [],
);
const quote = computed(() => {
  try {
    return rule.value
      ? exchangeResult(
          amount.value,
          rule.value.rate,
          direction.value === "to_points",
          currency.value,
        )
      : "0";
  } catch {
    return "0";
  }
});
const net = computed(() => {
  try {
    return decimal(
      minor(amount.value) -
        minor(
          data.value?.brand.rules.find((r) => r.currency === currency.value)
            ?.fee ?? "0",
        ),
    );
  } catch {
    return "0";
  }
});
const clock = ref(Date.now());
function canContinueDeposit(order: CashOrder) {
  return order.kind === 'deposit' && order.state === 'pending' && Date.parse(order.expires_at) > clock.value;
}
const recordKind = ref("to_points");
const cashRecords=computed(()=>recordKind.value==='all'?combined.value.flatMap(item=>item.order?[item.order]:[]):records.value);
function message(cause: unknown) {
  const code = cause instanceof ApiError ? cause.code : "";
  if (dialog.value === "exchange") {
    if (code === "CASH_INSUFFICIENT" && direction.value === "to_points")
      return t("cash.exchangeFundsInsufficient");
    if (["POINTS_INSUFFICIENT", "INSUFFICIENT_POINTS"].includes(code ?? "") && direction.value === "from_points")
      return t("cash.exchangePointsInsufficient");
  }
  const keys: Record<string, CashKey> = {
    CASH_DEPOSIT_PENDING: "cash.depositPending",
    CASH_TRON_WALLET_MISSING:"cash.tronWalletMissing",
    CASH_TRON_QUOTA_EXHAUSTED:"cash.tronQuotaExhausted",
    CASH_TRON_NOT_CONFIGURED:"cash.tronNotConfigured",
    CASH_TRON_POOL_EMPTY:"cash.tronPoolEmpty",
    CASH_INPUT_INVALID: "cash.invalid",
    CASH_ACCOUNT_INCOMPLETE: "cash.accountRequired",
    CASH_NOT_CONFIGURED: "cash.notConfigured",
    CASH_INSUFFICIENT: "cash.insufficient",
    CASH_LIMIT_EXCEEDED: "cash.limitExceeded",
    CASH_RATE_CHANGED: "cash.rateChanged",
    CASH_EXCHANGE_REQUIRED: "cash.exchangeRequired",
  };
  return code && keys[code] ? t(keys[code]) : errorText(cause);
}
async function readState() {
  const token = generation;
  const [state, points] = await Promise.all([api.value.state(), props.readAvailablePoints()]);
  if (token === generation) availablePoints.value = points;
  return state;
}
async function load(target = page.value) {
  const token = generation;
  loading.value = true;
  error.value = "";
  try {
    if(props.agent && recordKind.value==='all'){
      const [state,list]=await Promise.all([readState(),api.value.agentRecords(target,size.value)]);
      if(token!==generation)return;
      if(state.brand_id!==props.member?.brand_id||state.member_id!==props.member?.id||list.items.some(item=>item.order&&(item.order.brand_id!==state.brand_id||item.order.member_id!==state.member_id)))throw new ApiError("Scope mismatch",403,"AUTH_ACTOR_CONTEXT_CHANGED");
      data.value=state;combined.value=list.items;records.value=[];transfers.value=[];total.value=list.total_count;page.value=target;
      if(detail.value&&!locked.value){const updated=cashRecords.value.find(order=>order.id===detail.value?.id);if(updated)detail.value=updated;}
      if(!pending.value&&!enabledCurrencies.value.includes(currency.value)&&enabledCurrencies.value.length)currency.value=enabledCurrencies.value[0]!;
      return;
    }
    if(props.agent && recordKind.value==='transfer'){
      const [state,list]=await Promise.all([readState(),api.value.agentTransfers(target,size.value)]);
      if(token!==generation)return;
      if(state.brand_id!==props.member?.brand_id||state.member_id!==props.member?.id)throw new ApiError("Scope mismatch",403,"AUTH_ACTOR_CONTEXT_CHANGED");
      data.value=state;transfers.value=list.items;records.value=[];total.value=list.total_count;page.value=target;
      return;
    }
    const [state, list] = await Promise.all([
      readState(),
      api.value.orders(recordKind.value, target, size.value),
    ]);
    if (token !== generation) return;
    if (
      state.brand_id !== props.member?.brand_id ||
      state.member_id !== props.member?.id ||
      list.items.some(
        (o) => o.brand_id !== state.brand_id || o.member_id !== state.member_id,
      )
    )
      throw new ApiError("Scope mismatch", 403, "AUTH_ACTOR_CONTEXT_CHANGED");
    data.value = state;
    if (!pending.value && !enabledCurrencies.value.includes(currency.value) && enabledCurrencies.value.length)
      currency.value = enabledCurrencies.value[0]!;
    records.value = list.items;
    if (detail.value && !locked.value) {
      const updated = list.items.find(o => o.id === detail.value?.id);
      if (updated) detail.value = updated;
    }
    total.value = list.total_count;
    page.value = target;
  } catch (cause) {
    if (token === generation) {
      error.value = message(cause);
      if (cause instanceof ApiError && cause.status === 401)
        emit("auth-expired");
    }
  } finally {
    if (token === generation) loading.value = false;
  }
}
async function loadKind() {
  await load(1);
}
function defaults() {
  if (direction.value === "to_points" && balance.value)
    amount.value = decimal(
      minor(balance.value.deposited) + minor(balance.value.withdrawable),
    );
  else amount.value = "";
  selectedAccount.value = accounts.value[0]?.id ?? "";
}
watch(
  currency,
  () => {
    defaults();
    destination.value = emptyDestination(currency.value);
  },
  { flush: "sync" },
);
watch(direction, defaults, { flush: "sync" });
function open(which: typeof dialog.value) {
  if (locked.value) return;
  transferDetail.value=null;
  error.value = "";
  notice.value = "";
  returnToWithdrawal.value =
    which === "account" && dialog.value === "withdrawal";
  if (which === "exchange") direction.value = exchangeDirections.value[0];
  dialog.value = which;
  if (which !== "account") defaults();
  if (which === "deposit") amount.value = "";
  if (which === "account") destination.value = emptyDestination(currency.value);
}
function editAccount(d: CashDestination) {
  if (locked.value) return;
  currency.value = d.currency;
  destination.value = clone(d);
  returnToWithdrawal.value = false;
  dialog.value = "account";
}
function showTransfer(entry:AgentPointTransfer){
  closeTutorial();detail.value=null;transferDetail.value=entry;dialog.value="detail";
}
function show(o: CashOrder) {
  transferDetail.value=null;
  copied.value = false;
  detail.value = o;
  proof.value = o.proof;
  reference.value = o.reference;
  dialog.value = "detail";
}
async function write(path: string, body: unknown, isDestination = false) {
  if (busy.value) return;
  const token = generation;
  pending.value ??= {
    path,
    body: clone(body),
    key: crypto.randomUUID(),
    destination: isDestination,
    config: false,
    dialog: dialog.value ?? "exchange",
    detail: detail.value ? clone(detail.value) : null,
  };
  pendingCashWrites.set(pendingScope.value, pending.value);
  const frozen = pending.value;
  busy.value = true;
  error.value = "";
  notice.value = "";
  try {
    if (frozen.destination) {
      selectedAccount.value = (
        await api.value.saveDestination(
          frozen.body as CashDestination,
          frozen.key,
        )
      ).id;
    } else {
      const order = await api.value.writeOrder(
        frozen.path,
        frozen.body,
        frozen.key,
      );
      if (
        order.brand_id !== props.member?.brand_id ||
        order.member_id !== props.member?.id
      )
        throw new ApiError("Scope mismatch", 502, "INVALID_RESPONSE");
      detail.value = order;
    }
    if (token !== generation) return;
    pending.value = null;
    pendingCashWrites.delete(pendingScope.value);
    dialog.value =
      frozen.destination && returnToWithdrawal.value
        ? "withdrawal"
        : detail.value?.kind === "deposit" && frozen.path === "/cash/deposits"
          ? "detail"
          : null;
    if (!dialog.value) notice.value = t("cash.success");
    proof.value = "";
    reference.value = "";
    await load(frozen.path === "/cash/deposits" ? 1 : page.value);
    if (frozen.path === "/cash/exchanges")
      emit("exchanged");
  } catch (cause) {
    if (token !== generation) return;
    error.value = message(cause);
    if (
      cause instanceof ApiError &&
      cause.status &&
      cause.status >= 400 &&
      cause.status < 500
    ) {
      pending.value = null;
      pendingCashWrites.delete(pendingScope.value);
      if (
        cause.code === "CASH_RATE_CHANGED" ||
        cause.code === "CASH_STATE_CHANGED" ||
        cause.code === "CASH_DEPOSIT_PENDING"
      ) {
        await load(cause.code === "CASH_DEPOSIT_PENDING" ? 1 : page.value);
        error.value = message(cause);
      }
    }
    if (
      cause instanceof ApiError &&
      (cause.status === 401 || cause.code === "AUTH_ACTOR_CONTEXT_CHANGED")
    )
      emit("auth-expired");
  } finally {
    if (token === generation) busy.value = false;
  }
}
async function submit() {
  if (dialog.value === "account")
    return write("/cash/destinations", destination.value, true);
  if (dialog.value === "exchange")
    return write("/cash/exchanges", {
      currency: currency.value,
      amount: amount.value,
      direction: direction.value,
      rate: rule.value?.rate ?? "",
    });
  if (dialog.value === "deposit")
    return write("/cash/deposits", {
      currency: currency.value,
      amount: amount.value,
    });
  if (dialog.value === "withdrawal") {
    if (!selectedAccount.value) {
      destination.value = emptyDestination(currency.value);
      returnToWithdrawal.value = true;
      dialog.value = "account";
      error.value = t("cash.accountRequired");
      return;
    }
    return write("/cash/withdrawals", {
      currency: currency.value,
      amount: amount.value,
      destination_id: selectedAccount.value,
      destination_version: accounts.value.find(
        (a) => a.id === selectedAccount.value,
      )?.version,
      fee: data.value?.brand.rules.find((r) => r.currency === currency.value)
        ?.fee,
    });
  }
}
async function act(action: string) {
  if (!detail.value) return;
  await write(`/cash/orders/${detail.value.id}/${action}`, {
    version: detail.value.version,
    proof: proof.value,
    reference: reference.value,
  });
}
async function copyPayable() {
  if (!detail.value) return;
  try { await navigator.clipboard.writeText(decimal(minor(detail.value.requested_amount) + minor(detail.value.fee))); notice.value = t('cash.copied'); }
  catch (cause) { error.value = message(cause); }
}
async function copyAddress() {
  try {
    await navigator.clipboard.writeText(detail.value!.destination.number);
    copied.value = true;
  } catch (cause) {
    error.value = message(cause);
  }
}
watch(
  () => [props.apiBase, props.member?.id, props.member?.brand_id, props.mode],
  () => {
    generation++;
    data.value = null;
    availablePoints.value = null;
    records.value = [];
    transferDetail.value=null;
    transfers.value = [];
    combined.value = [];
    closeTutorial();
    dialog.value = null;
    pending.value = null;
    busy.value = false;
    page.value = 1;
    recordKind.value = props.mode === "overview" ? props.agent ? "all" : "to_points" : props.mode;
    if (props.member) {
      const saved = pendingCashWrites.get(pendingScope.value);
      if (saved) {
        pending.value = saved;
        detail.value = saved.detail;
        dialog.value = saved.dialog as WalletDialog;
        const body = saved.body as Record<string, unknown>;
        if (currencies.includes(body.currency as Currency))
          currency.value = body.currency as Currency;
        if (body.direction === "to_points" || body.direction === "from_points")
          direction.value = body.direction;
        if (typeof body.amount === "string") amount.value = body.amount;
        if (typeof body.destination_id === "string")
          selectedAccount.value = body.destination_id;
        if (typeof body.proof === "string") proof.value = body.proof;
        if (typeof body.reference === "string")
          reference.value = body.reference;
        if (saved.destination)
          destination.value = clone(saved.body as CashDestination);
      }
      void load(1);
    }
  },
  { immediate: true },
);
const refreshTimer = setInterval(() => {
  clock.value = Date.now();
  if (props.member && !loading.value && !locked.value && data.value?.auto_usdt &&
      cashRecords.value.some(o => o.automatic && ['pending', 'submitted', 'expired'].includes(o.state))) void load();
}, 10000);
onUnmounted(() => { generation++; clearInterval(refreshTimer); });
useOperationToast(error, (v) => v, "error");
useOperationToast(notice, (v) => v, "success");
</script>
<template>
  <section class="currency-wallet" :aria-busy="loading">
    <template v-if="data">
      <div class="cash-toolbar cash-actions">
        <slot name="actions-start" />
        <button
          v-if="mode === 'overview'"
          type="button"
          @click="emit('deposit')"
          class="button button-primary"
        >
          {{ t("cash.deposit") }}
        </button>
        <button v-if="mode === 'overview'" type="button" class="button button-secondary" @click="emit('withdrawal')">
          {{ t('cash.withdrawal') }}
        </button>
        <button
          v-if="mode === 'deposit'"
          class="button button-primary"
          :disabled="!enabled || locked"
          @click="open('deposit')"
        >
          {{ t("cash.deposit") }}
        </button>
        <button
          v-if="mode === 'withdrawal'"
          class="button button-primary"
          :disabled="!enabled || locked"
          @click="open('withdrawal')"
        >
          {{ t("cash.withdrawal") }}
        </button>
        <button
          class="button button-secondary"
          :disabled="!enabled || locked"
          @click="open('exchange')"
        >
          {{ t("cash.exchange") }}
        </button>
        <button
          v-if="mode === 'withdrawal'"
          class="button button-secondary"
          :disabled="!enabled || locked"
          @click="open('account')"
        >
          {{ t("cash.addAccount") }}
        </button>
      </div>
      <p v-if="!agent && mode !== 'overview'" class="cash-flow-help muted">{{ t(mode === 'deposit' ? 'cash.depositExchangeHelp' : 'cash.withdrawalExchangeHelp') }}</p>
      <div v-if="mode !== 'deposit'" class="cash-balances">
        <button
          v-for="b in visibleBalances"
          :key="b.currency"
          type="button"
          class="cash-balance"
          :class="{ 'is-selected': currency === b.currency }"
          :aria-pressed="currency === b.currency"
          :disabled="locked"
          @click="currency = b.currency"
        >
          <strong>{{ t(`cash.${b.currency}`) }}</strong>
          <dl>
            <div>
              <dt>{{ t("cash.deposited") }}</dt>
              <dd>{{ b.deposited }}</dd>
            </div>
            <div>
              <dt>{{ t("cash.withdrawable") }}</dt>
              <dd>{{ b.withdrawable }}</dd>
            </div>
            <div>
              <dt>{{ t("cash.reserved") }}</dt>
              <dd>{{ b.reserved }}</dd>
            </div>
          </dl>
        </button>
      </div>
      <p v-if="!enabled" class="muted" role="status">
        {{ t("cash.unconfigured") }}
      </p>
      <div v-if="mode === 'withdrawal'" class="cash-account-list">
        <button
          v-for="d in managedAccounts"
          :key="d.id"
          class="text-button cash-account-link"
          :disabled="locked"
          @click="editAccount(d)"
        >
          {{ d.institution || d.channel }} · {{ d.number
          }}<span v-if="!d.enabled"> · {{ t("cash.disabled") }}</span>
        </button>
      </div>
      <component :is="agent ? ManagementList : 'div'" :class="{'user-record-list': !agent}" :busy="loading || locked">
        <template v-if="agent" #heading><h3>{{ t(mode === 'deposit' ? "cash.depositHistory" : "cash.records") }}</h3></template>
        <template v-if="agent && mode==='overview'" #filters><SelectControl :model-value="recordKind" :label="t('cash.kind')" :disabled="loading||locked" :options="['all','deposit','withdrawal','to_points','from_points','transfer'].map(value=>({value,label:t(`cash.${value}`)}))" @change="recordKind=String($event);withGlobalLoading(loadKind)" /></template>
        <div v-if="!agent" class="cash-toolbar">
          <h3>{{ t(mode === 'deposit' ? "cash.depositHistory" : "cash.records") }}</h3>
          <SelectControl
            v-if="mode === 'overview'"
            :model-value="recordKind"
            :label="t('cash.kind')"
            :disabled="loading || locked"
            :options="
              ['to_points', 'from_points'].map((v) => ({
                value: v,
                label: t(`cash.${v}`),
              }))
            "
            @change="
              recordKind = String($event);
              withGlobalLoading(loadKind);
            "
          />
        </div>
        <ManagementTable v-if="agent && recordKind==='all'" :entries="combined" :columns="[{key:'kind',label:t('cash.kind')},{key:'member',label:t('cash.transferMember')},{key:'amount',label:t('cash.amount')},{key:'status',label:t('cash.status')},{key:'created',label:t('cash.created')},{key:'details',label:t('cash.details')}]" :empty="!loading&&!combined.length" :empty-label="t('cash.empty')">
          <template #kind="{entry}">{{t(`cash.${entry.kind}`)}}</template>
          <template #member="{entry}">{{entry.transfer?.member_account??'—'}}</template>
          <template #amount="{entry}"><template v-if="entry.transfer">{{formatPoints(entry.transfer.amount,locale)}} {{t('cash.points')}}</template><template v-else-if="entry.order">{{entry.order.amount}} {{entry.order.currency}}</template></template>
          <template #status="{entry}">{{entry.order?t(`cash.${entry.order.state}`):t('cash.transferCompleted')}}</template>
          <template #created="{entry}">{{formatDisplayDateTime(entry.created_at,locale)}}</template>
          <template #details="{entry}"><button v-if="entry.transfer" type="button" class="text-button" :disabled="locked" @click="showTransfer(entry.transfer)">{{t('cash.details')}}</button><button v-else-if="entry.order&&(entry.kind!=='deposit'||entry.order.state!=='cancelled')" type="button" class="text-button" :disabled="locked" @click="show(entry.order)">{{t('cash.details')}}</button><span v-else>—</span></template>
        </ManagementTable>
        <ManagementTable v-else-if="agent && recordKind==='transfer'" :entries="transfers" :columns="[{key:'member',label:t('cash.transferMember')},{key:'points',label:t('cash.points')},{key:'created',label:t('cash.transferTime')},{key:'details',label:t('cash.details')}]" :empty="!loading&&!transfers.length" :empty-label="t('cash.empty')">
          <template #member="{entry}">{{entry.member_account??'—'}}</template>
          <template #points="{entry}">{{formatPoints(entry.amount,locale)}}</template>
          <template #created="{entry}">{{formatDisplayDateTime(entry.created_at,locale)}}</template>
          <template #details="{entry}"><button type="button" class="text-button" :disabled="locked" @click="showTransfer(entry)">{{t('cash.details')}}</button></template>
        </ManagementTable>
        <ManagementTable v-else-if="agent" :entries="records" :columns="[{key:'amount',label:t('cash.amount')},{key:'channel',label:t('cash.channel')},{key:'status',label:t('cash.status')},{key:'created',label:t('cash.created')},{key:'details',label:t('cash.details')},...(mode==='deposit'?[{key:'operation',label:t('cash.operation')}]:[])]" :empty="!loading && !records.length" :empty-label="t('cash.empty')">
          <template #amount="{entry}">{{ entry.amount }} {{ entry.currency }}</template>
          <template #channel="{entry}">{{ entry.destination.id ? (entry.currency === 'USDT' ? entry.destination.channel : t('cash.bank')) : '—' }}</template>
          <template #status="{entry}">{{ t(`cash.${entry.state}`) }}</template>
          <template #created="{entry}">{{ formatDisplayDateTime(entry.created_at, locale) }}</template>
          <template #details="{entry}"><button v-if="entry.kind !== 'deposit' || entry.state !== 'cancelled'" type="button" class="text-button" :disabled="locked" @click="show(entry)">{{ t('cash.details') }}</button></template>
          <template #operation="{entry}"><button v-if="canContinueDeposit(entry)" type="button" class="text-button" :disabled="locked" @click="show(entry)">{{ t('cash.continueDeposit') }}</button><span v-else>—</span></template>
        </ManagementTable>
        <div v-else class="cash-records">
          <table class="cash-record-table" :class="{'cash-record-table--actions': mode === 'deposit'}">
            <thead><tr>
              <th>{{ t('cash.amount') }}</th>
              <th>{{ t('cash.kind') }}</th>
              <th>{{ t('cash.channel') }}</th>
              <th>{{ t('cash.status') }}</th>
              <th>{{ t('cash.created') }}</th>
              <th>{{ t('cash.details') }}</th>
              <th v-if="mode === 'deposit'">{{ t('cash.operation') }}</th>
            </tr></thead>
            <tbody>
              <tr v-for="o in records" :key="o.id">
                <td :data-label="t('cash.amount')"><strong>{{ o.amount }} {{ o.currency }}</strong></td>
                <td :data-label="t('cash.kind')">{{ t(`cash.${o.kind}`) }}</td>
                <td :data-label="t('cash.channel')">{{ o.destination.id ? (o.currency === 'USDT' ? o.destination.channel : t('cash.bank')) : '—' }}</td>
                <td :data-label="t('cash.status')">{{ t(`cash.${o.state}`) }}</td>
                <td :data-label="t('cash.created')"><time>{{ formatDisplayDateTime(o.created_at, locale) }}</time></td>
                <td :data-label="t('cash.details')"><button v-if="o.kind !== 'deposit' || o.state !== 'cancelled'" type="button" class="text-button" :disabled="locked" @click="show(o)">{{ t('cash.details') }}</button></td>
                <td v-if="mode === 'deposit'" :data-label="t('cash.operation')"><button v-if="canContinueDeposit(o)" type="button" class="text-button" :disabled="locked" @click="show(o)">{{ t('cash.continueDeposit') }}</button><span v-else>—</span></td>
              </tr>
              <tr v-if="!loading && !records.length"><td :colspan="mode === 'deposit' ? 7 : 6" class="empty-state">{{ t('cash.empty') }}</td></tr>
            </tbody>
          </table>
        </div>
        <template v-if="agent" #pagination>
        <TablePagination
          :total="total"
          :page="page"
          :count="recordKind==='all'?combined.length:recordKind==='transfer'?transfers.length:records.length"
          :locale="locale"
          :page-size="size"
          :busy="loading || locked"
          :go-to="(target) => load(target)"
          :change-page-size="
            async (value) => {
              size = value;
              await load(1);
            }
          "
        />
        </template>
      </component>
        <TablePagination v-if="!agent"
          :total="total"
          :page="page"
          :count="records.length"
          :locale="locale"
          :page-size="size"
          :busy="loading || locked"
          :go-to="(target) => load(target)"
          :change-page-size="
            async (value) => {
              size = value;
              await load(1);
            }
          "
        />

    </template>
    <DetailModal
      v-if="dialog"
      :class="dialog === 'detail' ? 'confirmation-dialog' : 'ui-form-dialog'"
      :title="
        t(
          dialog === 'detail'
            ? transferDetail ? 'cash.details' : 'cash.receipt'
            : dialog === 'account'
              ? 'cash.accounts'
              : `cash.${dialog}`,
        )
      "
      :close-label="t('cash.close')"
      :close-disabled="locked"
      @close="closeTutorial(); dialog = null; transferDetail=null"
    >
      <form
        v-if="dialog !== 'detail'"
        novalidate
        v-inline-validation
        v-write-guard="locked"
        class="ui-form"
        @submit.prevent="withGlobalLoading(submit)"
      >
        <fieldset :disabled="locked">
          <label class="ui-field-row"
            ><span>{{ t("cash.currency") }}</span><SelectControl
              v-model="currency"
              :label="t('cash.currency')"
              :disabled="dialog === 'account' && !!destination.id"
              :options="
                enabledCurrencies.map((v) => ({ value: v, label: t(`cash.${v}`) }))
              "
          /></label>
          <CashDestinationFields
            v-if="dialog === 'account'"
            v-model="destination"
            :t="label"
          />
          <template v-else>
            <label v-if="dialog === 'exchange'" class="ui-field-row"
              ><span>{{ t("cash.kind") }}</span><SelectControl
                v-model="direction"
                :label="t('cash.kind')"
                :disabled="props.mode !== 'overview'"
                :options="
                  exchangeDirections.map((v) => ({
                    value: v,
                    label: t(`cash.${v}`),
                  }))
                "
            /></label>
            <label class="ui-field-row"
              ><span>{{
                t(
                  dialog === "exchange" && direction === "from_points"
                    ? "cash.points"
                    : "cash.amount",
                )
              }}</span><input
                v-model="amount"
                :aria-describedby="dialog === 'exchange' ? exchangeBalanceHintId : dialog === 'deposit' && !(currency === 'USDT' && data?.auto_usdt) ? depositAmountHintId : undefined"
                required
                inputmode="decimal"
                :data-validation-format="t('cash.numberFormat',{digits:dialog==='exchange'&&direction==='from_points'?2:currency==='USDT'?6:2})"
                :pattern="
                  dialog === 'exchange' && direction === 'from_points'
                    ? '[0-9]+([.][0-9]{1,2})?'
                    : currency === 'USDT'
                      ? '[0-9]+([.][0-9]{1,6})?'
                      : '[0-9]+([.][0-9]{1,2})?'
                "
            /></label>
            <p v-if="dialog === 'deposit' && !(currency === 'USDT' && data?.auto_usdt)" :id="depositAmountHintId" class="muted">{{ t("cash.depositAmountHint") }}</p>
            <p v-if="dialog === 'exchange'" :id="exchangeBalanceHintId" class="cash-exchange-balance muted" role="status">{{ direction === 'to_points'
              ? t('cash.currentFunds', { amount: balance ? decimal(minor(balance.deposited) + minor(balance.withdrawable)) : '—', currency })
              : t('cash.currentPoints', { amount: availablePoints === null ? '—' : formatPoints(availablePoints, locale) }) }}</p>
            <dl v-if="dialog === 'exchange' && rule" class="cash-detail">
              <div>
                <dt>{{ t("cash.rateQuote") }}</dt>
                <dd>
                  1 {{ currency }} = {{ rule.rate }} {{ t("cash.points") }}
                </dd>
              </div>
              <div>
                <dt>{{ t("cash.receive") }}</dt>
                <dd>
                  {{ quote }}
                  {{ direction === "to_points" ? t("cash.points") : currency }}
                </dd>
              </div>
            </dl>
            <template v-if="dialog === 'withdrawal'"
              ><label class="ui-field-row"
                ><span>{{ t("cash.accounts") }}</span><SelectControl
                  v-model="selectedAccount"
                  :label="t('cash.accounts')"
                  :options="[
                    { value: '', label: t('cash.chooseAccount') },
                    ...accounts.map((a) => ({
                      value: a.id,
                      label: `${a.institution || a.channel} · ${a.number}`,
                    })),
                  ]"
              /></label>
              <dl class="cash-detail">
                <div>
                  <dt>{{ t("cash.fee") }}</dt>
                  <dd>
                    {{
                      data?.brand.rules.find((r) => r.currency === currency)
                        ?.fee
                    }}
                    {{ currency }}
                  </dd>
                </div>
                <div>
                  <dt>{{ t("cash.receive") }}</dt>
                  <dd>{{ net }} {{ currency }}</dd>
                </div>
              </dl>
              <button
                v-if="!accounts.length"
                type="button"
                class="text-button"
                @click="open('account')"
              >
                {{ t("cash.addAccount") }}
              </button></template
            >
          </template>
        </fieldset>
        <div class="ui-form-actions">
          <button
            type="submit"
            class="button button-primary"
            :disabled="locked"
          >
            {{ t("cash.confirm") }}
          </button>
        </div>
      </form>
      <template v-else-if="transferDetail">
        <dl class="cash-detail">
          <div><dt>{{t('cash.transferMember')}}</dt><dd>{{transferDetail.member_account??'—'}}</dd></div>
          <div><dt>{{t('cash.points')}}</dt><dd>{{formatPoints(transferDetail.amount,locale)}}</dd></div>
          <div><dt>{{t('cash.transferTime')}}</dt><dd>{{formatDisplayDateTime(transferDetail.created_at,locale)}}</dd></div>
        </dl>
      </template>
      <template v-else-if="detail">
        <dl class="cash-detail">
          <div>
            <dt>{{ t(detail.kind === 'deposit' ? detail.automatic ? "cash.principal" : detail.state === 'confirmed' ? "cash.actual" : "cash.requested" : "cash.amount") }}</dt>
            <dd>{{ detail.amount }} {{ detail.currency }}</dd>
          </div>
          <div v-if="detail.automatic">
            <dt>{{ t(detail.state === 'confirmed' ? 'cash.actual' : 'cash.payable') }}</dt>
            <dd>{{ decimal(minor(detail.state === 'confirmed' ? detail.amount : detail.requested_amount) + minor(detail.fee)) }} {{ detail.currency }}</dd>
          </div>
          <div v-if="detail.requested_amount !== detail.amount">
            <dt>{{ t("cash.requested") }}</dt>
            <dd>{{ detail.requested_amount }} {{ detail.currency }}</dd>
          </div>
          <div v-if="detail.points !== '0'">
            <dt>{{ t("cash.points") }}</dt>
            <dd>{{ detail.points }}</dd>
          </div>
          <div>
            <dt>{{ t("cash.status") }}</dt>
            <dd>{{ t(`cash.${detail.state}`) }}</dd>
          </div>
          <div v-if="detail.kind === 'deposit'"><dt>{{ t("cash.creditMode") }}</dt><dd>{{ t(detail.automatic && (detail.state !== 'confirmed' || detail.sender) ? "cash.autoCredit" : "cash.manualCredit") }}</dd></div>
          <div><dt>{{ t("cash.created") }}</dt><dd>{{ formatDisplayDateTime(detail.created_at, locale) }}</dd></div>
          <div v-if="detail.currency === 'USDT' && detail.destination.id"><dt>{{ t("cash.network") }}</dt><dd>{{ detail.destination.channel }}</dd></div>
          <div v-if="detail.credited_at"><dt>{{ t("cash.creditedAt") }}</dt><dd>{{ formatDisplayDateTime(detail.credited_at,locale) }}</dd></div>
          <div v-if="detail.sender"><dt>{{ t("cash.sender") }}</dt><dd class="cash-address key-value-wide">{{ detail.sender }}</dd></div>
          <div v-if="detail.fee !== '0'">
            <dt>{{ t("cash.fee") }}</dt>
            <dd>{{ detail.fee }} {{ detail.currency }}</dd>
          </div>
        </dl>
        <div
          v-if="detail.destination.id"
          class="cash-transfer"
          :class="{ 'cash-transfer--crypto': detail.currency === 'USDT' }"
        >
          <template v-if="detail.currency === 'USDT'"
            ><p class="cash-network-notice">{{ t("cash.networkNotice") }}</p>
            <CashAddress
              class="cash-transfer-qr"
              :address="detail.destination.number"
              :label="t('cash.address')"
              @failure="error = message($event)"
          /></template>
          <dl class="cash-detail">
            <div v-if="detail.destination.holder">
              <dt>{{ t("cash.holder") }}</dt>
              <dd>{{ detail.destination.holder }}</dd>
            </div>
            <div v-if="detail.destination.institution">
              <dt>{{ t("cash.institution") }}</dt>
              <dd>{{ detail.destination.institution }}</dd>
            </div>
            <div>
              <dt>
                {{
                  t(detail.currency === "USDT" ? "cash.address" : "cash.number")
                }}
              </dt>
              <dd class="cash-address key-value-wide">
                {{ detail.destination.number }}
              </dd>
            </div>
            <div v-if="detail.expires_at">
              <dt>{{ t("cash.expires") }}</dt>
              <dd>{{ formatDisplayDateTime(detail.expires_at, locale) }}</dd>
            </div>
          </dl>
          <div v-if="detail.currency === 'USDT'" class="cash-toolbar">
            <button v-if="detail.automatic && canContinueDeposit(detail)" type="button" class="button button-secondary" :disabled="busy" @click="copyPayable">{{ t('cash.copyPayable') }}</button>
            <button
              class="button button-secondary"
              :disabled="busy"
              @click="copyAddress"
            >
              {{ t(copied ? "cash.copied" : "cash.copy") }}</button
            ><button
              type="button"
              class="button button-secondary"
              :disabled="busy"
              @click="tutorialOpen = true"
              >{{ t("cash.tutorial") }}</button>

          </div>
        </div>
        <section v-if="detail.kind === 'deposit' && detail.currency === 'USDT' && canContinueDeposit(detail) && (!detail.automatic || data?.auto_usdt)" class="cash-transfer-guide" :aria-label="t('cash.transferSteps')">
          <h3>{{ t('cash.transferSteps') }}</h3>
          <ol><li>{{ t('cash.transferSource') }}</li><li>{{ t('cash.transferNetwork') }}</li><li>{{ t(detail.automatic ? 'cash.exactPayment' : 'cash.transferAmount') }}</li><li>{{ t(detail.automatic ? data?.auto_usdt ? 'cash.tronAutoTransfer' : 'cash.tronPaused' : 'cash.transferReview') }}</li></ol>
          <p class="cash-transfer-support">{{ t('cash.transferSupport') }}</p>
        </section>
        <p v-if="detail.automatic && detail.state === 'pending' && !data?.auto_usdt" class="muted" role="status">{{ t('cash.tronPaused') }}</p>
        <p v-if="detail.kind === 'deposit' && (detail.state === 'expired' || detail.state === 'pending' && !canContinueDeposit(detail))" class="muted" role="status">{{ t('cash.expiredDeposit') }}</p>
        <p v-if="detail.kind === 'deposit' && detail.state === 'submitted'" class="muted" role="status">{{ t(detail.automatic ? data?.auto_usdt ? 'cash.tronAutoSubmitted' : 'cash.tronPaused' : 'cash.awaitingReview') }}</p>
        <form
          v-if="
            detail.kind === 'deposit' &&
            ['pending', 'expired'].includes(detail.state)
          "
          novalidate
          v-inline-validation
          v-write-guard="locked"
          class="ui-form"
          @submit.prevent="withGlobalLoading(act, 'submit')"
        >
          <fieldset v-if="detail.currency !== 'USDT'" :disabled="locked">
            <label>{{ t("cash.reference") }}<input v-model.trim="reference" maxlength="200" /></label>
            <label>{{ t("cash.proof") }}<textarea v-model.trim="proof" maxlength="500" :required="!reference" /></label>
          </fieldset>
          <div class="ui-form-actions">
            <button class="button button-primary" :disabled="locked">
              {{ t("cash.submit") }}</button
            ><button
              v-if="detail.state === 'pending'"
              class="button button-secondary"
              type="button"
              :disabled="locked"
              @click="withGlobalLoading(act, 'cancel')"
            >
              {{ t("cash.cancelDeposit") }}
            </button>
          </div>
        </form>
        <button
          v-if="detail.kind === 'withdrawal' && detail.state === 'reviewing'"
          class="button button-secondary"
          :disabled="locked"
          @click="withGlobalLoading(act, 'cancel')"
        >
          {{ t("cash.cancelWithdrawal") }}
        </button>
        <dl v-if="detail.reference || detail.proof" class="cash-detail">
          <div v-if="detail.reference">
            <dt>
              {{
                t(detail.currency === "USDT" ? "cash.txid" : "cash.reference")
              }}
            </dt>
            <dd class="cash-address key-value-wide">{{ detail.reference }}</dd>
          </div>
          <div v-if="detail.proof">
            <dt>{{ t("cash.proof") }}</dt>
            <dd>{{ detail.proof }}</dd>
          </div>
        </dl>
      </template>
      <div v-if="pending && !busy" class="ui-form-actions">
        <p role="status">{{ t("cash.unknown") }}</p>
        <button
          class="button button-primary"
          @click="
            withGlobalLoading(
              write,
              pending.path,
              pending.body,
              pending.destination,
            )
          "
        >
          {{ t("cash.retry") }}
        </button>
      </div>
    </DetailModal>
    <DetailModal v-if="tutorialOpen && dialog === 'detail'" class="confirmation-dialog" :title="t('cash.tutorial')" :close-label="t('cash.close')" @close="closeTutorial">
      <video ref="tutorialVideo" class="cash-tutorial-video" src="/videos/buyusdt.mp4" :aria-label="t('cash.tutorial')" controls playsinline preload="metadata" />
    </DetailModal>
  </section>
</template>
<style scoped>
.currency-wallet {
  display: grid;
  gap: 12px;
  min-width: 0;
}
.cash-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 8px;
}
.cash-toolbar h3 {
  margin: 0;
}
.cash-toolbar h3 {
  font-size: 16px;
}
.cash-exchange-balance { margin: 0; overflow-wrap: anywhere; font-variant-numeric: tabular-nums; }
.cash-flow-help { margin: 0; line-height: 1.6; }
.cash-actions {
  justify-content: flex-start;
}
.cash-balances {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 180px), 1fr));
  gap: 8px;
}
.cash-balance:only-child { grid-column: 1 / -1; }
@media (min-width: 701px) {
  .cash-balance:only-child dl { grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px; }
  .cash-balance:only-child dl > div { display: grid; gap: 8px; }
}
.cash-balance {
  display: grid;
  gap: 8px;
  min-width: 0;
  text-align: start;
  padding: 12px;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  background: var(--surface);
  color: var(--ink);
  font: inherit;
}
.cash-balance.is-selected {
  border-color: var(--primary);
  box-shadow: inset 0 0 0 1px var(--primary);
}
.cash-balance dl {
  display: grid;
  gap: 4px;
  margin: 0;
}
.cash-balance dl > div {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 4px;
}
.cash-balance dt {
  color: var(--muted);
}
.cash-balance dd {
  margin: 0;
  white-space: nowrap;
  flex-shrink: 0;
  font-variant-numeric: tabular-nums;
}
.cash-detail {
  display: grid;
  gap: 8px;
  margin: 8px 0;
}
.cash-detail > div {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  min-width: 0;
}
.cash-detail dt {
  color: var(--muted);
  flex-shrink: 0;
}
.cash-detail dd {
  margin: 0;
  text-align: end;
  overflow-wrap: anywhere;
  min-width: 0;
  font-variant-numeric: tabular-nums;
}
.cash-address {
  font-family: var(--font-mono, monospace);
  overflow-wrap: anywhere;
}
.cash-transfer {
  display: grid;
  gap: 8px;
  margin-block: 12px;
}
.cash-network-notice {
  padding: 8px;
  border: 1px solid var(--warning);
  background: var(--warning-bg);
  color: var(--warning-text);
  border-radius: var(--radius-sm);
  margin: 0;
}
.user-record-list > .cash-toolbar { padding: 12px; margin: 0; }
.cash-records { min-width: 0; }
.cash-record-table { width: 100%; border-collapse: collapse; font-size: var(--body-font-size); }
.cash-record-table th, .cash-record-table td { padding: 10px 12px; text-align: start; vertical-align: middle; border-bottom: 1px solid var(--line); }
.cash-record-table tbody tr:last-child td { border-bottom: 0; }
.cash-record-table time { white-space: nowrap; font-variant-numeric: tabular-nums; }
.cash-record-table .text-button { min-height: 44px; }
@media (max-width: 700px) {
  .cash-record-table thead { display: none; }
  .cash-record-table tbody { display: block; }
  .cash-record-table tr { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; padding: 12px; border-bottom: 1px solid var(--line); }
  .cash-record-table tr:last-child { border-bottom: 0; }
  .cash-record-table td { display: grid; align-content: start; gap: 4px; min-width: 0; padding: 0; border: 0; overflow-wrap: anywhere; }
  .cash-record-table td::before { content: attr(data-label); color: var(--muted); }
  .cash-record-table td:nth-child(5) { grid-column: 1 / -1; }
  .cash-record-table td:last-child:not(.empty-state) { grid-column: 1 / -1; justify-items: start; }
  .cash-record-table td:last-child::before { display: none; }
  .cash-record-table--actions td:nth-last-child(-n+2):not(.empty-state) { grid-column: auto; justify-items: start; min-height: 44px; align-content: center; }
  .cash-record-table--actions td:nth-last-child(-n+2)::before { display: none; }
}
.cash-account-list {
  display: grid;
  gap: 4px;
}
.cash-account-link {
  overflow-wrap: anywhere;
  text-align: start;
}
fieldset {
  border: 0;
  min-width: 0;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 8px;
}
@media (max-width: 760px) {
  .currency-wallet { gap: 8px; }
  .cash-record-table tr { gap: 4px 8px; padding-block: 8px; }
  .user-record-list > .cash-toolbar { padding-block: 8px; }
  .cash-balances {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (max-width: 360px) {
  .cash-balances {
    grid-template-columns: 1fr;
  }
  .cash-detail > div {
    flex-wrap: wrap;
  }
}
.cash-tutorial-video { display: block; width: 100%; max-height: calc(100dvh - 140px); object-fit: contain; }
.cash-transfer-support { color: var(--danger-text); font-weight: 600; }
.cash-transfer-guide { display: grid; gap: 8px; margin-block: 12px; }
.cash-transfer-guide h3 { margin: 0; font-size: var(--body-font-size); }
.cash-transfer-guide ol { display: grid; gap: 6px; margin: 0; padding-inline-start: 22px; }
.cash-transfer-guide p { margin: 0; }
.cash-transfer-qr {
  width: 192px;
  justify-self: center;
}
.cash-transfer > .cash-toolbar {
  justify-content: flex-start;
}
@media (min-width: 620px) {
  .cash-transfer--crypto {
    grid-template-columns: 192px minmax(0, 1fr);
    column-gap: 16px;
    align-items: start;
  }
  .cash-transfer--crypto .cash-network-notice {
    grid-column: 1/-1;
  }
  .cash-transfer--crypto .cash-transfer-qr {
    grid-column: 1;
    grid-row: 2/4;
  }
  .cash-transfer--crypto .cash-detail,
  .cash-transfer--crypto .cash-toolbar {
    grid-column: 2;
    margin: 0;
  }
}
</style>
