<script setup lang="ts">
import ManagementList from "@lottery/shared/ManagementList.vue"
import { auditReasonLabel, readPaginationTotal } from '@lottery/shared'
const ledgerTotal = ref('0')
const rechargeTotal = ref('0')

import PaginationControls from '@lottery/shared/PaginationControls.vue'
import SelectControl from '@lottery/shared/SelectControl.vue'
import { useOperationToast } from "@lottery/shared";
import RechargeQuotaPanel from "@lottery/shared/RechargeQuotaPanel.vue";
const quotaPanel = ref<InstanceType<typeof RechargeQuotaPanel> | null>(null);
const adjustmentQuotaPanel = ref<InstanceType<typeof RechargeQuotaPanel> | null>(null);
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
import { withGlobalLoading } from '@lottery/shared';
import SiteIcon from "@lottery/shared/SiteIcon";
import { vRefreshButton } from '@lottery/shared';
const displayDate = (value: string | null | undefined) => formatDisplayDateTime(value, displayLocale.value);
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import { vInlineValidation } from '@lottery/shared'
import { computed, nextTick, onMounted, ref, watch } from "vue";
import type { LocalizedMessage } from "@lottery/shared";
import { type AdminAccount } from "./admin-api";
import DetailModal from "@lottery/shared/DetailModal.vue";
import ManagementTable from "@lottery/shared/ManagementTable.vue";
import DirectorySearch from "@lottery/shared/DirectorySearch.vue";
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
import { useAdminI18n } from "./i18n";
import FilterAssociationSearch from "@lottery/shared/FilterAssociationSearch.vue";
import { lookupAdminMember } from "./member-lookup";

const props = withDefaults(defineProps<{
  account: AdminAccount & Partial<FinanceAccount>;
  brandId: string;
  initialMemberId?: string;
  initialMemberUsername?: string;
  view?: 'wallet' | 'ledger' | 'recharges';
}>(), { view: 'wallet' });
const emit = defineEmits<{ (event: "session-invalid"): void; (event: "close"): void; (event: "changed"): void }>();
const { t, message, display } = useAdminI18n();
const PAGE_SIZE = 50;
const api = createFinanceApi();
const sources: WalletSource[] = ["recharge", "winning", "gift", "commission"];
const states: WalletState[] = [
  "available",
  "manual_frozen",
  "system_frozen",
  "withdrawal",
];
function sourceName(source: WalletSource) {
  return source === "recharge"
    ? t('admin.copy.6ffe1179')
    : source === "winning"
      ? t('admin.copy.99c31497')
      : source === "gift" ? t('admin.copy.2b8ae17c') : source === "commission" ? t('admin.copy.101e2e66') : source;
}
function stateName(state: WalletState) {
  return state === "available"
    ? t('admin.copy.e9231c7d')
    : state === "manual_frozen"
      ? t('admin.copy.3817714f')
      : state === "system_frozen"
        ? t('admin.copy.c4073e10')
        : state === "withdrawal" ? t('admin.copy.f56770cc') : state;
}
const entryTypeKeys = {
  recharge: 'admin.finance.entry.recharge',
  freeze: 'admin.finance.entry.freeze',
  unfreeze: 'admin.finance.entry.unfreeze',
  adjust: 'admin.finance.entry.adjust',
  adjustment: 'admin.finance.entry.adjustment',
  bet: 'admin.finance.entry.bet',
  refund: 'admin.finance.entry.refund',
  prize: 'admin.finance.entry.prize',
  prize_reversal: 'admin.finance.entry.prize_reversal',
  reversal: 'admin.finance.entry.reversal',
  reward_grant: 'admin.finance.entry.reward_grant',
  reward_reversal: 'admin.finance.entry.reward_reversal',
  commission: 'admin.finance.entry.commission',
  commission_adjustment: 'admin.finance.entry.commission_adjustment',
  commission_correction: 'admin.finance.entry.commission_correction',
  withdrawal_reserve: 'admin.finance.entry.withdrawal_reserve',
  withdrawal_release: 'admin.finance.entry.withdrawal_release',
  withdrawal_paid: 'admin.finance.entry.withdrawal_paid',
 } as const;
function entryTypeName(type: string) {
  if (!(type in entryTypeKeys)) return t('admin.finance.unknownEntry', { type });
  return t(entryTypeKeys[type as keyof typeof entryTypeKeys]);
}
function closeMember() {
  if (mutating.value || pendingMutation.value) return;
  emit('close');
}
const memberId = ref("");
const loadedMemberId = ref("");
const loadedMemberUsername = ref("");
const showAllBalances = ref(false);
const showAllReconciliationBalances = ref(false);
const wallet = ref<Wallet | null>(null);
const entries = ref<LedgerEntry[]>([]);
const reconciliation = ref<Reconciliation | null>(null);
const reconciliationQueriedAt = ref("");
const recharges = ref<Recharge[]>([]);
const expandedRechargeId = ref('');
const ledgerOffset = ref(0);
const rechargeOffset = ref(0);
const loading = ref(false);
const mutating = ref(false);
const pendingMutation = ref<(() => Promise<void>) | null>(null);
const error = ref<string | LocalizedMessage>("");
const walletError = ref<string | LocalizedMessage>("");
const rechargeError = ref<string | LocalizedMessage>("");
const notice = ref<string | LocalizedMessage>("");
const rechargeReasonById = ref<Record<string, string>>({});
const cancelReasonById = ref<Record<string, string>>({});
const unfreezeReason = ref("");
const adjustForm = ref({
  source: (canWriteFinance(props.account, props.brandId, "wallet.adjust.brand") ? "recharge" : "freeze") as WalletSource | "freeze",
  delta: "",
  remark: "",
});
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
const visibleReconciliationRows = computed(() => reconciliationRows.value.filter(row =>
  showAllReconciliationBalances.value || row.expected !== '0' || row.actual !== '0',
));
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
const ledgerColumns = computed(() => [
  { key: 'time', label: t('admin.finance.time') },
  { key: 'type', label: t('admin.finance.businessType') },
  { key: 'balance', label: t('admin.finance.balanceCategory') },
  { key: 'before', label: t('admin.copy.f85d9f7f'), cellClass: 'ledger-amount' },
  { key: 'change', label: t('admin.copy.574031a3'), cellClass: 'ledger-amount' },
  { key: 'after', label: t('admin.copy.6db1d337'), cellClass: 'ledger-amount' },
  { key: 'remark', label: t('admin.finance.adjustmentRemark') },
]);
const ledgerRows = computed(() => entries.value.flatMap(entry => {
  const balances = sources.flatMap(source => states.map(state => ({ source, state })));
  const changed = balances.filter(({ source, state }) => entry.delta_snapshot[source][state] !== '0');
  return (showAllBalances.value ? balances : changed)
    .map(({ source, state }, index) => ({ id: `${entry.id}-${source}-${state}`, entry, source, state, first: index === 0 }));
}));
const hasNextLedger = computed(() => entries.value.length === PAGE_SIZE);
const hasNextRecharge = computed(() => recharges.value.length === PAGE_SIZE);

function messageFor(cause: unknown): string | LocalizedMessage {
  return publicErrorMessage(cause);
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
  showAllBalances.value = false;
  showAllReconciliationBalances.value = false;
  expandedRechargeId.value = '';
  loadedMemberUsername.value = '';
  loadedMemberId.value = "";
  wallet.value = null;
  [entries.value,ledgerTotal.value]=[[],'0'];
  reconciliation.value = null;
  reconciliationQueriedAt.value = "";
  [recharges.value,rechargeTotal.value]=[[],'0'];
  walletError.value = "";
  rechargeError.value = "";
}
const rechargeMemberDraft = ref("");
const rechargeMemberName = ref("");
const rechargeSearchPending = ref(false);
const rechargeFilterReset = ref(0);
async function inspectRecharge() {
  if (rechargeSearchPending.value || loading.value) return;
  clearData();
  loadedMemberId.value = rechargeMemberDraft.value;
  loadedMemberUsername.value = rechargeMemberName.value;
  rechargeOffset.value = 0;
  if (loadedMemberId.value) await reloadData();
  else await loadRechargeQueue();
}
async function clearRechargeFilter() {
  rechargeFilterReset.value++;
  rechargeMemberDraft.value = "";
  memberId.value = '';
  await nextTick(); await inspectRecharge();
}
async function inspectMember() {
  const username = memberId.value.trim();
  if (loading.value) return;
  if (!username && props.view === 'recharges') { clearData(); rechargeOffset.value = 0; await loadRechargeQueue(); return; }
  if (!username) return;
  try {
    const id = (await lookupAdminMember(props.brandId, username)).id;
    if (loadedMemberId.value !== id) {
      wallet.value = null;
      [entries.value,ledgerTotal.value]=[[],'0'];
      reconciliation.value = null;
      [recharges.value,rechargeTotal.value]=[[],'0'];
    }
    loadedMemberId.value = id;
    loadedMemberUsername.value = username;
    ledgerOffset.value = 0;
    rechargeOffset.value = 0;
    await reloadData();
  } catch (cause) {
    handleFailure(cause);
  }
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
  if (viewWallet.value && props.view !== 'recharges') {
    jobs.push(
      api
        .wallet(requestBrandId, targetId)
        .then((value) => {
          if (!isCurrent()) return;
          if (value.brand_id === requestBrandId && value.member_id === targetId)
            wallet.value = value;
          else
            walletError.value = message('admin.copy.51c898aa',
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
            [entries.value, ledgerTotal.value] = [result.items, readPaginationTotal(result)];
          else
            walletError.value = message('admin.copy.d9a774cd',
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
          if (isCurrent() && value.member_id === targetId) {
            reconciliation.value = value;
            reconciliationQueriedAt.value = new Date().toISOString();
          }
        })
        .catch((cause: unknown) => {
          if (isCurrent()) handleFailure(cause, "wallet");
        }),
    );
  } else {
    wallet.value = null;
    [entries.value,ledgerTotal.value]=[[],'0'];
    reconciliation.value = null;
  }
  if (props.view === 'recharges' && viewRecharges.value) {
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
            [recharges.value, rechargeTotal.value] = [result.items, readPaginationTotal(result)];
          else
            rechargeError.value = message('admin.copy.e7defc16',
            );
        })
        .catch((cause: unknown) => {
          if (isCurrent()) handleFailure(cause, "recharge");
        }),
    );
  } else {
    [recharges.value,rechargeTotal.value]=[[],'0'];
  }
  await Promise.all(jobs);
  if (isCurrent()) loading.value = false;
}
async function inspectInitialMember() {
  if (!props.initialMemberId) return;
  clearData();
  memberId.value = props.initialMemberUsername ?? '';
  loadedMemberUsername.value = memberId.value;
  loadedMemberId.value = props.initialMemberId;
  ledgerOffset.value = 0;
  rechargeOffset.value = 0;
  await reloadData();
}
watch(() => props.initialMemberId, id => { if (id) void withGlobalLoading(inspectInitialMember); });
onMounted(() => {
  if (props.initialMemberId) void withGlobalLoading(inspectInitialMember);
  else if (memberId.value && viewWallet.value) void withGlobalLoading(inspectMember);
  else if (viewRecharges.value && props.view === 'recharges') void withGlobalLoading(loadRechargeQueue);
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
    if (viewRecharges.value && props.view === 'recharges') void withGlobalLoading(loadRechargeQueue);
  },
);

watch(() => props.view, value => {
  if (value === 'recharges' && !loadedMemberId.value && viewRecharges.value) void withGlobalLoading(loadRechargeQueue);
});
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
      [recharges.value, rechargeTotal.value] = [result.items, readPaginationTotal(result)];
  } catch (cause) {
    if (isCurrent()) handleFailure(cause, "recharge");
  } finally {
    if (isCurrent()) loading.value = false;
  }
}
async function moveLedgerPage(direction: number) {
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
      [entries.value, ledgerTotal.value] = [result.items, readPaginationTotal(result)];
  } catch (cause) {
    if (isCurrent()) handleFailure(cause, "wallet");
  } finally {
    if (isCurrent()) loading.value = false;
  }
}
async function moveRechargePage(direction: number) {
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
    if (props.view !== 'recharges') pendingMutation.value = () => mutate(action, successText, clearKey);
    await action();
    pendingMutation.value = null;
    clearKey();
    if (props.view !== 'recharges') {
      adjustForm.value.delta = '';
      adjustForm.value.remark = '';
      unfreezeReason.value = '';
      emit('changed');
    }
    await quotaPanel.value?.reload();
    notice.value = successText;
    if (loadedMemberId.value) await reloadData();
    else if (viewRecharges.value) await loadRechargeQueue();
  } catch (cause) {
    if (cause instanceof FinanceApiError && cause.status >= 400 && cause.status < 500) pendingMutation.value = null;
    handleFailure(cause);
  } finally {
    mutating.value = false;
  }
}
async function confirmRecharge(recharge: Recharge) {
  const reason = rechargeReasonById.value[recharge.id]?.trim() ?? "";
  const body = { version: recharge.version, reason };
  await mutate(
    () =>
      api.confirmRecharge(
        props.brandId,
        recharge.id,
        body,
        confirmKeyFor({ brand_id: props.brandId, id: recharge.id, ...body }),
      ),
    message('admin.copy.5342718a'),
    () => confirmKeyFor.clear(),
  );
}
async function cancelRecharge(recharge: Recharge) {
  const reason = cancelReasonById.value[recharge.id]?.trim() ?? "";
  if (recharge.state !== "pending") return;
  const body = { version: recharge.version, reason };
  await mutate(
    () =>
      api.cancelRecharge(
        props.brandId,
        recharge.id,
        body,
        cancelKeyFor({ brand_id: props.brandId, id: recharge.id, ...body }),
      ),
    message('admin.copy.2d33cb11'),
    () => cancelKeyFor.clear(),
  );
  if (!error.value) cancelReasonById.value[recharge.id] = "";
}
async function freezePoints() {
  if (
    !loadedMemberId.value ||
    !writeFreeze.value ||
    !isPositiveAmount(adjustForm.value.delta)
  )
    return;
  const body = {
    points: adjustForm.value.delta,
    reason: adjustForm.value.remark.trim(),
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
    message('admin.copy.6954dc9c'),
    () => freezeKeyFor.clear(),
  );
  if (!error.value) {
    adjustForm.value.delta = "";
    adjustForm.value.remark = "";
    await adjustmentQuotaPanel.value?.reload();
  }
}
async function unfreeze(entry: LedgerEntry) {
  if (!loadedMemberId.value ) return;
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
    message('admin.copy.6d969245'),
    () => unfreezeKeyFor.clear(),
  );
  if (!error.value) unfreezeReason.value = "";
}
async function adjustPoints() {
  if (adjustForm.value.source === "freeze") {
    await freezePoints();
    return;
  }
  if (
    !loadedMemberId.value ||
    !writeAdjust.value ||
    !isSignedAmount(adjustForm.value.delta)
  )
    return;
  const body = {
    source: adjustForm.value.source,
    delta: adjustForm.value.delta,
    reason: adjustForm.value.remark.trim(),
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
    message('admin.copy.21b7927d'),
    () => adjustKeyFor.clear(),
  );
  if (!error.value) {
    adjustForm.value.delta = "";
    adjustForm.value.remark = "";
    await adjustmentQuotaPanel.value?.reload();
  }
}
function sourceSnapshotValue(
  entry: LedgerEntry,
  part: "before_snapshot" | "delta_snapshot" | "after_snapshot",
  source: WalletSource,
  state: WalletState,
) {
  return formatIntegerAmount(entry[part][source][state]);
}

useOperationToast(() => error.value, value => display(value), 'error');
useOperationToast(() => notice.value, value => display(value), 'success');
useOperationToast(() => walletError.value, value => display(value), 'error');
useOperationToast(() => rechargeError.value, value => display(value), 'error');
</script>

<template>
  <section class="finance-management">
    <header v-if="view === 'recharges'" class="console-page-heading page-head">
      <div>
        <h1 class="console-page-title">{{ t('admin.console.recharges') }}</h1>
      </div>
    </header>
    <div v-if="account.super_admin" class="server-note">
      {{ t('admin.copy.034cebc9') }}
    </div>
    <p
      v-if="!canSearch && !(writeRecharge || writeFreeze || writeAdjust)"
      class="callout"
    >
      {{ t('admin.copy.1bc43da5') }}
    </p>
    <p v-if="view === 'recharges' && error" class="operation-recovery-actions">

      <button class="button button-secondary" v-refresh-button="t('admin.copy.894e2a7d')"
        type="button"
        @click="withGlobalLoading(async () => { loadedMemberId ? await reloadData() : await loadRechargeQueue() })"
        >
        {{ t('admin.copy.894e2a7d') }}
      </button>
    </p>

    <p v-if="rechargeError" class="operation-recovery-actions">

      <button class="button button-secondary" v-refresh-button="t('admin.copy.547bccbf')"
        type="button"
        @click="withGlobalLoading(async () => { loadedMemberId ? await reloadData() : await loadRechargeQueue() })"
      >
        {{ t('admin.copy.547bccbf') }}
      </button>
    </p>

    <DetailModal v-if="loadedMemberId && (view === 'wallet' || view === 'ledger')" :title="`${view === 'ledger' ? t('admin.nav.ledger') : t('admin.nav.points')} · ${loadedMemberUsername}`" :close-label="t('association.close')" :close-disabled="mutating || !!pendingMutation" @close="closeMember">
      <p v-if="pendingMutation" class="operation-recovery-actions">
        <button v-write-guard="mutating" type="button" class="button button-primary" :disabled="mutating" @click="withGlobalLoading(pendingMutation)">{{ t('admin.copy.7434d6ed') }}</button>
      </p>
      <p v-else-if="error || walletError" class="operation-recovery-actions">
        <button v-refresh-button="t('admin.copy.bfd9564c')" type="button" class="button button-secondary" :disabled="loading" @click="withGlobalLoading(reloadData)">{{ t('admin.copy.bfd9564c') }}</button>
      </p>
      <div v-show="view === 'wallet' || view === 'ledger'">
      <section v-if="view === 'wallet' && viewWallet && wallet" class="panel wallet-panel">
        <div class="panel-title">
          <div>
            <h2>{{ t('admin.copy.aadb5e3a') }}</h2>
          </div>
          <button v-refresh-button="t('admin.copy.894e2a7d')"
            type="button"
            class="button button-secondary"
            :disabled="loading"
            @click="withGlobalLoading(reloadData)"
          >
            {{ t('admin.copy.894e2a7d') }}
          </button>
        </div>
        <div class="totals">
          <article class="total prominent">
            <span>{{ t('admin.copy.628e6b32') }}</span
            ><strong>{{ formatIntegerAmount(wallet.display_points) }}</strong>
          </article>
          <article class="total">
            <span>{{ t('admin.copy.e9231c7d') }}</span
            ><strong>{{ formatIntegerAmount(wallet.available_points) }}</strong>
          </article>
          <article class="total">
            <span>{{ t('admin.copy.4816eabf') }}</span
            ><strong>{{ formatIntegerAmount(wallet.frozen_points) }}</strong>
          </article>
          <article class="total">
            <span>{{ t('admin.copy.f56770cc') }}</span
            ><strong>{{
              formatIntegerAmount(wallet.withdrawal_points)
            }}</strong>
          </article>
        </div>
        <details class="finance-breakdown">
          <summary>{{ t('admin.console.sourceBreakdown') }}<SiteIcon name="down" /></summary>
        <div class="bucket-table">
          <div class="table-head">
            <span>{{ t('admin.copy.d2089b75') }}</span><span>{{ t('admin.copy.cf6d9c6d') }}</span><span>{{ t('admin.copy.ec082fa7') }}</span>
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
        </details>
      </section>
      <p v-else-if="viewWallet && !wallet && !walletError" class="callout">
        {{ t('admin.copy.3a487e0e') }}
      </p>
      <p v-else-if="!viewWallet" class="callout">
        {{ t('admin.copy.e09d18b4') }}
      </p>

      <section v-if="view === 'ledger' && viewWallet && reconciliation" class="panel">
        <div class="panel-title">
          <div>
            <h2>{{ t('admin.copy.0ff02f56') }}</h2>
            <p>
               {{ t('admin.copy.be598abe', { count: String(reconciliation.entry_count) }) }}
            </p>
          </div>
          <span
            class="status"
            :class="reconciliation.consistent ? 'good' : 'bad'"
            >{{ reconciliation.consistent ? t('admin.copy.f97b639f') : t('admin.copy.4d1960ba') }}</span
          >
        </div>
        <ul v-if="reconciliation.issues.length" class="issues">
          <li v-for="(issue, index) in reconciliation.issues" :key="index">
            {{ issue }}
          </li>
        </ul>
        <details class="finance-breakdown" :open="!reconciliation.consistent">
          <summary>{{ t('admin.console.reconciliationDetails') }}<SiteIcon name="down" /></summary>
          <p class="reconciliation-time">{{ t('admin.finance.queryTime') }} <time :datetime="reconciliationQueriedAt">{{ displayDate(reconciliationQueriedAt) }}</time></p>
          <label class="snapshot-toggle"><input v-model="showAllReconciliationBalances" type="checkbox" />{{ t('admin.finance.showAllCategories') }}</label>
          <p v-if="!visibleReconciliationRows.length" class="muted empty-state">{{ t('admin.finance.zeroBalances') }}</p>
        <div class="recon-table">
          <div v-if="visibleReconciliationRows.length" class="table-head">
            <span>{{ t('admin.copy.4aa12ed7') }}</span><span>{{ t('admin.finance.expectedBalance') }}</span><span>{{ t('admin.finance.currentBalance') }}</span>
          </div>
          <div
            v-for="row in visibleReconciliationRows"
            :key="`${row.source}-${row.state}`"
            class="recon-row"
          >
            <b>{{ sourceName(row.source) }} · {{ stateName(row.state) }}</b
            ><span>{{ formatIntegerAmount(row.expected) }}</span
            ><span>{{ formatIntegerAmount(row.actual) }}</span>
          </div>
        </div>
        </details>
      </section>

    <section
      v-show="view === 'wallet'"
      v-if="loadedMemberId && (writeFreeze || writeAdjust)"
      class="operations-grid"
    >
      <form v-write-guard="mutating || !!pendingMutation" novalidate v-inline-validation
        class="form-panel ui-form"
        @submit.prevent="withGlobalLoading(adjustPoints)"
      >
        <div class="panel-title">
          <h2>{{ t('admin.copy.f9f97fca') }}</h2>
        </div>
        <label>{{ t('admin.finance.adjustmentType') }}<SelectControl :disabled="!!pendingMutation" v-model="adjustForm.source" :label="String(t('admin.finance.adjustmentType'))" :options="[...((writeAdjust) ? (sources).map((source) => ({ value: source, label: (String(sourceName(source))).trim() })) : []), ...((writeFreeze) ? [{ value: 'freeze', label: (String(t('admin.finance.freezeOption'))).trim() }] : [])]" /></label>
        <div class="adjustment-amount-row">
        <label>{{ t('admin.copy.10025d7b') }}<input
          :disabled="!!pendingMutation" v-model.trim="adjustForm.delta"
          style="--input-width: 140px"
          :inputmode="adjustForm.source === 'freeze' ? 'numeric' : 'text'"
          required
          :placeholder="adjustForm.source === 'freeze' ? undefined : t('admin.copy.9a348780')" /></label>
        <RechargeQuotaPanel v-if="writeAdjust && viewRecharges" external-feedback summary-only ref="adjustmentQuotaPanel" :brand-id="brandId" :locale="associationLocale" base-path="/api/v1/admin" @failure="cause => { error = publicErrorMessage(cause, 'read') }" />
        </div>
        <label>{{ t('admin.finance.adjustmentRemark') }}<input :disabled="!!pendingMutation" v-model.trim="adjustForm.remark" :placeholder="t('admin.finance.adjustmentRemarkPlaceholder')" maxlength="500" /></label>
        <button
          class="button button-primary"
          :disabled="mutating || !!pendingMutation || (adjustForm.source === 'freeze' ? !isPositiveAmount(adjustForm.delta) : !isSignedAmount(adjustForm.delta))"
        >{{ t('admin.copy.cb99f7c9') }}</button>
      </form>
    </section>

      <ManagementList embedded v-if="view === 'ledger' && viewWallet" class="panel">
        <div class="panel-title">
          <div>
            <h2>{{ t('admin.copy.de1c63a8') }}</h2>
          </div>
        </div>
        <label class="snapshot-toggle"><input v-model="showAllBalances" type="checkbox" />{{ t('admin.finance.fullSnapshot') }}</label>
        <ManagementTable :entries="ledgerRows" :columns="ledgerColumns" :empty="!loading && !walletError && !entries.length" :empty-label="t('admin.copy.8249da4b')" table-class="ledger-table">
          <template #time="{ entry: row }"><time class="ledger-time" :datetime="row.entry.created_at">{{ displayDate(row.entry.created_at) }}</time></template>
          <template #type="{ entry: row }">
            <span>{{ entryTypeName(row.entry.entry_type) }}</span>
            <button v-if="row.first && writeFreeze && unfreezeCandidates.some(item => item.id === row.entry.id)"
              v-write-guard="mutating" type="button" class="button button-secondary" :disabled="mutating || !!pendingMutation"
              @click="withGlobalLoading(unfreeze, row.entry)">{{ t('admin.copy.f5c98e2d') }}</button>
          </template>
          <template #balance="{ entry: row }"><span>{{ sourceName(row.source) }} · {{ stateName(row.state) }}</span></template>
          <template #before="{ entry: row }">{{ sourceSnapshotValue(row.entry, 'before_snapshot', row.source, row.state) }}</template>
          <template #change="{ entry: row }"><strong :class="{ 'ledger-increase': BigInt(row.entry.delta_snapshot[row.source][row.state]) > 0n, 'ledger-decrease': BigInt(row.entry.delta_snapshot[row.source][row.state]) < 0n }">{{ BigInt(row.entry.delta_snapshot[row.source][row.state]) > 0n ? '+' : '' }}{{ sourceSnapshotValue(row.entry, 'delta_snapshot', row.source, row.state) }}</strong></template>
          <template #after="{ entry: row }">{{ sourceSnapshotValue(row.entry, 'after_snapshot', row.source, row.state) }}</template>
          <template #remark="{ entry: row }">{{ auditReasonLabel(row.entry.reason, displayLocale) }}</template>
        </ManagementTable>

      <template #pagination><PaginationControls :show-empty="!loading && !rechargeError"
          :total="ledgerTotal" :page="Math.floor(ledgerOffset / PAGE_SIZE + 1)" :count="entries.length" :locale="associationLocale"
          :previous-label="t('admin.copy.dbe7a5d7')"
          :next-label="t('admin.copy.2e9c9e7a')"
          :busy="loading"
          :has-previous="ledgerOffset > 0"
          :has-next="hasNextLedger"
         :page-size="PAGE_SIZE" :go-to="(target: number) => moveLedgerPage(target - (Math.floor(ledgerOffset / PAGE_SIZE + 1)))" /></template></ManagementList>
      </div>
    </DetailModal>

    <ManagementList v-show="view === 'recharges'" v-if="viewRecharges">
    <template #filters><DirectorySearch class="recharge-filters" v-model="memberId" :label="t('admin.copy.b41d30df')" :query-label="t('admin.members.query')" :clear-label="t('admin.copy.b089734d')" :busy="loading" :query-pending="rechargeSearchPending" :active="Boolean(rechargeSearchPending || rechargeMemberDraft || loadedMemberId)" :search="inspectRecharge" :clear="clearRechargeFilter">
      <template #input><FilterAssociationSearch :reset-key="rechargeFilterReset" v-model="rechargeMemberDraft" v-model:pending="rechargeSearchPending" :brand-id="brandId" surface="admin" resource="recharge" :label="t('filter.member')" :locale="associationLocale" :scope-key="JSON.stringify(account)" :enabled="viewRecharges" @select="rechargeMemberName = $event.name" /></template>
      <template #actions>
        <RechargeQuotaPanel v-if="view === 'recharges' && viewRecharges" compact external-feedback @failure="cause => { error = publicErrorMessage(cause, 'read') }" ref="quotaPanel" :brand-id="brandId" :locale="associationLocale" base-path="/api/v1/admin" />
      </template>
    </DirectorySearch></template>

      <div class="table-wrap"><table class="console-table console-recharges-table console-responsive-table"><thead><tr>
        <th>{{ t('admin.recharge.method') }}</th><th>{{ t('admin.copy.92c59de3') }}</th><th>{{ t('admin.console.points') }}</th><th>{{ t('admin.console.state') }}</th><th>{{ t('admin.console.created') }}</th><th>{{ t('admin.console.actions') }}</th>
      </tr></thead><tbody><template v-for="recharge in recharges" :key="recharge.id"><tr>
        <td :data-label="t('admin.recharge.method')">{{ t(recharge.method === "manual" ? "admin.recharge.manual" : "admin.recharge.automatic") }}</td><td :data-label="t('admin.copy.92c59de3')"><AssociationName :record="recharge" kind="member" :id="recharge.member_id" :locale="associationLocale" /></td>
        <td :data-label="t('admin.console.points')">{{ formatIntegerAmount(recharge.points) }}</td><td :data-label="t('admin.console.state')"><span class="status" :class="recharge.state">{{ recharge.state === 'pending' ? t('admin.copy.7746bbe1') : recharge.state === 'confirmed' ? t('admin.copy.5f313d26') : t('admin.copy.a454aa98') }}</span></td>
        <td :data-label="t('admin.console.created')">{{ displayDate(recharge.created_at) }}</td><td :data-label="t('admin.console.actions')"><button type="button" class="text-button" :aria-expanded="expandedRechargeId === recharge.id" @click="expandedRechargeId = expandedRechargeId === recharge.id ? '' : recharge.id">{{ t('admin.copy.01db40da') }}</button></td>
      </tr>

      </template><tr v-if="!loading && !rechargeError && !recharges.length"><td colspan="6" class="empty-state">{{ t('admin.copy.dcbdb099') }}</td></tr></tbody></table></div>
      <template v-for="recharge in recharges" :key="`detail-${recharge.id}`">
        <DetailModal v-if="expandedRechargeId === recharge.id" :title="t('admin.copy.01db40da')" :close-label="t('association.close')" :close-disabled="mutating" @close="expandedRechargeId = ''">
<div class="recharge-detail">
        <table class="recharge-detail-table">
          <tbody>
            <tr><th scope="row">{{ t('admin.recharge.method') }}</th><td>{{ t(recharge.method === 'manual' ? 'admin.recharge.manual' : 'admin.recharge.automatic') }}</td></tr>
            <tr><th scope="row">{{ t('admin.copy.92c59de3') }}</th><td><AssociationName :record="recharge" kind="member" :id="recharge.member_id" :locale="associationLocale" /></td></tr>
            <tr><th scope="row">{{ t('admin.console.points') }}</th><td>{{ formatIntegerAmount(recharge.points) }}</td></tr>
            <tr><th scope="row">{{ t('admin.console.state') }}</th><td><span class="status" :class="recharge.state">{{ recharge.state === 'pending' ? t('admin.copy.7746bbe1') : recharge.state === 'confirmed' ? t('admin.copy.5f313d26') : t('admin.copy.a454aa98') }}</span></td></tr>
            <tr><th scope="row">{{ t('admin.console.created') }}</th><td>{{ displayDate(recharge.created_at) }}</td></tr>
            <tr v-if="recharge.proof_reference"><th scope="row">{{ t('admin.copy.bb54d12f') }}</th><td>{{ recharge.proof_reference }}</td></tr>
            <tr v-if="recharge.remark"><th scope="row">{{ t('admin.copy.d19e55e6') }}</th><td>{{ recharge.remark }}</td></tr>
            <tr><th scope="row">{{ t('admin.copy.6a2d3973') }}</th><td><AssociationName :record="recharge" kind="admin" :id="recharge.created_by" :locale="associationLocale" /></td></tr>
            <tr v-if="recharge.confirmed_by"><th scope="row">{{ t('admin.copy.faa0379b') }}</th><td><AssociationName :record="recharge" kind="admin" :id="recharge.confirmed_by" :locale="associationLocale" /></td></tr>
          </tbody>
        </table>
        <div v-if="writeRecharge && recharge.state === 'pending'" class="recharge-detail-actions ui-form-actions">
        <button
          v-write-guard="mutating"
          v-if="writeRecharge && recharge.state === 'pending'"
          type="button"
          class="button button-primary small-button"
          :disabled="mutating "
          @click="withGlobalLoading(confirmRecharge, recharge)"
        >
          {{ t('admin.copy.6a4e9f46') }}
        </button>
        <button
          v-write-guard="mutating"
          v-if="writeRecharge && recharge.state === 'pending'"
          type="button"
          class="button button-secondary small-button"
          :disabled="mutating "
          @click="withGlobalLoading(cancelRecharge, recharge)"
        >
          {{ t('admin.copy.d1072032') }}
        </button>
        </div>
      </div>
        </DetailModal>
      </template>

    <template #pagination><PaginationControls :show-empty="!loading && !rechargeError"
          :total="rechargeTotal" :page="Math.floor(rechargeOffset / PAGE_SIZE + 1)" :count="recharges.length" :locale="associationLocale"
          :previous-label="t('admin.copy.dbe7a5d7')"
          :next-label="t('admin.copy.2e9c9e7a')"
          :busy="loading"
          :has-previous="rechargeOffset > 0"
          :has-next="hasNextRecharge"
         :page-size="PAGE_SIZE" :go-to="(target: number) => moveRechargePage(target - (Math.floor(rechargeOffset / PAGE_SIZE + 1)))" /></template></ManagementList>


  </section>
</template>

<style scoped>
.recharge-detail{min-width:0}
.recharge-detail-table{width:100%;min-width:0;table-layout:fixed;border-collapse:collapse;font-size:var(--body-font-size)}
.recharge-detail-table :is(th,td){position:static;padding:8px 12px;border:1px solid var(--line);text-align:left;vertical-align:middle;white-space:normal;overflow-wrap:anywhere}
.recharge-detail-table th{width:30%;color:var(--ink-secondary);font-weight:650;background:var(--canvas)}
.recharge-detail-actions{display:flex;flex-wrap:wrap;gap:8px;margin-top:12px}
@media(min-width:901px) and (pointer:fine){.recharge-detail-table :is(th,td){padding-block:6px}}
@media(max-width:400px){.recharge-detail-table :is(th,td){padding-inline:8px}.recharge-detail-table th{width:34%}}

.page-head {flex-wrap:wrap;}
.finance-management {
  width: 100%;
  min-width: 0;
  color: var(--ink);
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
  color: var(--muted);
  font-size: var(--body-font-size);
  line-height: 1.55;
}
.eyebrow {
  font-size: var(--body-font-size) !important;
  letter-spacing: 1.2px;
  color: var(--muted) !important;
  font-weight: 700;
}
.brand-tag,
.status {
  border-radius: 20px;
  background: var(--line-subtle);
  padding: 7px 11px;
  color: var(--ink-secondary);
  font-size: var(--body-font-size);
  white-space: nowrap;
}
.server-note,
.callout {
  border-radius: 8px;
  background: var(--canvas);
  color: var(--ink-secondary);
  padding: 10px 12px;
  font-size: var(--body-font-size);
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
  font-size: var(--body-font-size);
  font-weight: 600;
  min-width: 0;
}
.lookup label {
  flex: 1;
}
.lookup input, .form-grid input, .form-panel input, .recharge-row input, .unfreeze-row input {
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  border: 1px solid var(--line);
  border-radius: 7px;
  background: var(--surface);
  padding: 9px 10px;
  color: var(--ink);
  font: inherit;
}
.button-primary,
.button-secondary {
  border-radius: 7px;
  padding: 9px 12px;
  font-size: var(--body-font-size);
  font-weight: 700;
  white-space: nowrap;
}
.button-primary {
  border: 0;
  background: var(--primary);
  color: var(--surface);
}
.button-secondary {
  border: 1px solid var(--line);
  background: var(--surface);
  color: var(--ink-secondary);
}
.button-primary:disabled,
.button-secondary:disabled {
  opacity: 0.48;
  cursor: not-allowed;
}
.notice {
  border-radius: 8px;
  padding: 10px 12px;
  font-size: var(--body-font-size);
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
  background: var(--danger-bg);
  color: var(--danger-text);
}
.success {
  background: var(--success-bg);
  color: var(--success-text);
}
.panel {
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: var(--radius);
  padding: 16px;
  min-width: 0;
  box-shadow: var(--shadow-sm);
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
  font-size: var(--body-font-size);
  line-height: 1.55;
  color: var(--muted);
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
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 11px;
  min-width: 0;
}
.total span {
  font-size: var(--body-font-size);
  color: var(--muted);
}
.total strong {
  font-size: 18px;
  overflow-wrap: anywhere;
  font-variant-numeric: tabular-nums;
}
.total.prominent {
  background: var(--primary-light);
}
.bucket-table,
.recon-table {
  display: grid;
  grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr) minmax(0, 1fr);
  font-size: var(--body-font-size);
}
.table-head,
.bucket-row,
.recon-row {
  display: contents;
}
.table-head span {
  background: var(--line-subtle);
  padding: 8px;
  font-weight: 700;
  color: var(--muted);
}
.bucket-row > *,
.recon-row > * {
  padding: 8px;
  border-bottom: 1px solid var(--line-subtle);
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
  background: var(--success-bg);
  color: var(--success-text);
}
.status.bad,
.status.cancelled {
  background: var(--danger-bg);
  color: var(--danger-text);
}
.status.pending {
  background: var(--warning-bg);
  color: var(--warning-text);
}
.issues {
  background: var(--warning-bg);
  color: var(--warning-text);
  font-size: var(--body-font-size);
  line-height: 1.6;
  border-radius: 7px;
  padding: 10px 10px 10px 28px;
}
.ledger-entry,
.recharge-row {
  border-top: 1px solid var(--line-subtle);
  padding: 13px 0;
  min-width: 0;
}
.entry-head,
.recharge-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  font-size: var(--body-font-size);
}
.entry-head > div,
.recharge-head > div {
  display: grid;
  gap: 4px;
  min-width: 0;
}
.entry-head small,
.recharge-head small {
  font-size: var(--body-font-size);
  color: var(--muted);
  overflow-wrap: anywhere;
}
.entry-head code {
  font-size: var(--body-font-size);
  color: var(--muted);
  overflow-wrap: anywhere;
}
.reason-text,
.allocation,
.recharge-row > p {
  font-size: var(--body-font-size);
  color: var(--ink-secondary);
  overflow-wrap: anywhere;
  line-height: 1.55;
}
.snapshot-toggle { display: flex; align-items: center; gap: 8px; min-height: 44px; color: var(--muted); font-size: var(--body-font-size); }
:deep(.ledger-table) { table-layout: fixed; min-width: 0; width: 100%; }
:deep(.ledger-table :is(th, td)) { white-space: normal; overflow-wrap: anywhere; }
:deep(.ledger-table th:nth-child(n+4):nth-child(-n+6)),
:deep(.ledger-table .ledger-amount) { text-align: right; font-variant-numeric: tabular-nums; }
.ledger-time { white-space: nowrap; font-variant-numeric: tabular-nums; }
.reconciliation-time { color: var(--muted); font-size: var(--body-font-size); line-height: 1.5; }
.reconciliation-time time { display: inline-block; white-space: nowrap; font-variant-numeric: tabular-nums; }
@media (min-width: 1081px) {
  :deep(.ledger-table th:first-child) { width: 190px; }
}
.ledger-increase { color: var(--success-text); }
.ledger-decrease { color: var(--danger-text); }
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
.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}
.form-grid small {
  font-size: var(--body-font-size);
  color: var(--muted);
  font-weight: 400;
}
.form-grid .wide {
  grid-column: 1/-1;
}
.form-grid .button-primary {
  justify-self: start;
}
.form-panel > label {
  margin: 0;
}
.form-panel > .panel-title { margin-bottom: 0; }
.adjustment-amount-row { display: flex; flex-wrap: wrap; align-items: end; gap: 6px 12px; min-width: 0; margin: 0; }
.adjustment-amount-row > label { display: grid; gap: 6px; width: 140px; max-width: 100%; min-width: 0; font-weight: 600; }
.adjustment-amount-row input { width: 100%; box-sizing: border-box; }
.operations-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 13px;
}
.operations-grid .panel {
  margin: 0;
}
.operations-grid .button-primary {
  margin-top: 0;
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
    flex-wrap: wrap;
  }
  .lookup label {
    max-width: 100%;
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
  :deep(.ledger-table .ledger-amount) { text-align: left; }
  .unfreeze-row {
    align-items: stretch;
    flex-direction: column;
  }
  .unfreeze-row button {
    align-self: flex-start;
  }
  .bucket-table,
  .recon-table {
    font-size: var(--body-font-size);
  }
}
.finance-breakdown { margin-top: 12px; }
.finance-breakdown > summary { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 44px; cursor: pointer; color: var(--ink-secondary); font-weight: 600; list-style: none; }
.finance-breakdown > summary::-webkit-details-marker { display: none; }
.finance-breakdown[open] > summary { margin-bottom: 8px; }
.finance-breakdown[open] > summary :deep(svg) { transform: rotate(180deg); }
@media (min-width: 901px) and (pointer: fine) {
  .ledger-entry, .recharge-row { padding-block: 8px; }
}
</style>
