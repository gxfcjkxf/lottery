<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import SiteIcon from "@lottery/shared/SiteIcon";
import { vRefreshButton } from '@lottery/shared';
const displayDate = (value: string | null | undefined) => formatDisplayDateTime(value, displayLocale.value);
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import { vInlineValidation } from '@lottery/shared'
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
import { lookupAdminMember } from "./member-lookup";

const props = withDefaults(defineProps<{
  account: AdminAccount & Partial<FinanceAccount>;
  brandId: string;
  initialMemberId?: string;
  initialMemberUsername?: string;
  view?: 'wallet' | 'recharges' | 'policy';
}>(), { view: 'wallet' });
const emit = defineEmits<{ (event: "session-invalid"): void }>();
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
function actorName(actorType: string) {
  return actorType === "system"
    ? t('admin.copy.532e2e04')
    : actorType === "user"
      ? t('admin.copy.689b5703')
      : t('admin.copy.37b47e95');
}
const memberId = ref("");
const loadedMemberId = ref("");
const loadedMemberUsername = ref("");
const fullSnapshotEntryIds = ref<string[]>([]);
const wallet = ref<Wallet | null>(null);
const entries = ref<LedgerEntry[]>([]);
const reconciliation = ref<Reconciliation | null>(null);
const recharges = ref<Recharge[]>([]);
const expandedRechargeId = ref('');
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
  fullSnapshotEntryIds.value = [];
  expandedRechargeId.value = '';
  loadedMemberUsername.value = '';
  loadedMemberId.value = "";
  wallet.value = null;
  entries.value = [];
  reconciliation.value = null;
  recharges.value = [];
  walletError.value = "";
  rechargeError.value = "";
}
async function inspectMember() {
  const username = memberId.value.trim();
  if (!username || loading.value) return;
  try {
    const id = (await lookupAdminMember(props.brandId, username)).id;
    if (loadedMemberId.value !== id) {
      wallet.value = null;
      entries.value = [];
      reconciliation.value = null;
      recharges.value = [];
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
  if (viewWallet.value) {
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
            entries.value = result.items;
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
            rechargeError.value = message('admin.copy.e7defc16',
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
  else if (viewRecharges.value && (props.view === 'recharges' || !viewWallet.value)) void withGlobalLoading(loadRechargeQueue);
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
    if (viewRecharges.value && (props.view === 'recharges' || !viewWallet.value)) void withGlobalLoading(loadRechargeQueue);
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
    !isPositiveAmount(rechargeForm.value.points)
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
    message('admin.copy.2c1b91f0'),
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
    !isPositiveAmount(freezeForm.value.points)
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
    message('admin.copy.6954dc9c'),
    () => freezeKeyFor.clear(),
  );
  if (!error.value) freezeForm.value = { points: "", reason: "" };
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
  if (
    !loadedMemberId.value ||
    !isSignedAmount(adjustForm.value.delta)
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
    message('admin.copy.21b7927d'),
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
        <h1>{{ view === 'recharges' ? t('admin.console.recharges') : view === 'policy' ? t('admin.console.financialPolicies') : t('admin.console.wallet') }}</h1>
      </div>
      <span class="brand-tag">{{ t('admin.copy.55bdd7a5') }}<AssociationName kind="brand" :id="brandId" :locale="associationLocale" /></span>
    </header>
    <div v-if="account.super_admin" class="server-note">
      {{ t('admin.copy.034cebc9') }}
    </div>
    <PointPolicySettings
      v-show="view === 'policy'"
      v-if="canViewPolicy"
      :account="account"
      :brand-id="brandId"
      @session-invalid="emit('session-invalid')"
    />
    <form novalidate v-inline-validation
      v-show="view !== 'policy'"
      v-if="canSearch || writeRecharge || writeFreeze || writeAdjust"
      class="lookup"
      @submit.prevent="withGlobalLoading(inspectMember)"
    >
      <label class="query-field">
        <span class="field-label">{{ t('admin.copy.92c59de3') }}</span>
        <div class="input-with-icon">
          <span class="input-icon"> <SiteIcon name="user" /> </span>
          <input
            v-model.trim="memberId"
            required
            autocomplete="off"
            :placeholder="t('admin.copy.b41d30df')"
          />
        </div>
      </label>
      <button class="button button-primary" :disabled="loading || !memberId.trim()">
        {{ loading ? t('admin.copy.27342bf3') : t('admin.copy.a3f96ba5') }}
      </button>
    </form>
    <p
      v-if="!canSearch && !(writeRecharge || writeFreeze || writeAdjust)"
      class="callout"
    >
      {{ t('admin.copy.1bc43da5') }}
    </p>
    <p v-if="error" class="notice error" role="alert">
      {{ display(error) }}
      <button v-refresh-button="t('admin.copy.894e2a7d')"
        type="button"
        @click="withGlobalLoading(async () => { loadedMemberId ? await reloadData() : await loadRechargeQueue() })"
        >
        {{ t('admin.copy.894e2a7d') }}
      </button>
    </p>
    <p v-if="notice" class="notice success" role="status">{{ display(notice) }}</p>
    <p v-if="walletError" class="notice error" role="alert">
      {{ display(walletError) }}
      <button v-refresh-button="t('admin.copy.bfd9564c')" type="button" @click="withGlobalLoading(reloadData)">{{ t('admin.copy.bfd9564c') }}</button>
    </p>
    <p v-if="rechargeError" class="notice error" role="alert">
      {{ display(rechargeError) }}
      <button v-refresh-button="t('admin.copy.547bccbf')"
        type="button"
        @click="withGlobalLoading(async () => { loadedMemberId ? await reloadData() : await loadRechargeQueue() })"
      >
        {{ t('admin.copy.547bccbf') }}
      </button>
    </p>

    <template v-if="loadedMemberId">
      <div v-show="view === 'wallet'">
      <section v-if="viewWallet && wallet" class="panel wallet-panel">
        <div class="panel-title">
          <div>
            <h2>{{ t('admin.copy.aadb5e3a') }}</h2>
            <p>
              {{ t('admin.copy.92c59de3') }} <AssociationName :record="wallet" kind="member" :id="wallet.member_id" :locale="associationLocale" />
            </p>
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

      <section v-if="viewWallet && reconciliation" class="panel">
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
        <div class="recon-table">
          <div class="table-head">
            <span>{{ t('admin.copy.4aa12ed7') }}</span><span>{{ t('admin.copy.4682e661') }}</span><span>{{ t('admin.copy.bcecdbf2') }}</span>
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
        </details>
      </section>

    <section
      v-show="view === 'wallet'"
      v-if="loadedMemberId && (writeFreeze || writeAdjust)"
      class="operations-grid"
    >
      <form v-write-guard="mutating" novalidate v-inline-validation
        v-if="writeFreeze"
        class="panel form-panel"
        @submit.prevent="withGlobalLoading(freezePoints)"
      >
        <div class="panel-title">
          <div>
            <h2>{{ t('admin.copy.2c6ca5bf') }}</h2>
            <p>{{ t('admin.copy.66997ca8') }}</p>
          </div>
        </div>
        <label
          >{{ t('admin.copy.b912f1a9') }}<input
            v-model.trim="freezeForm.points"
            inputmode="numeric"
            required /></label
        ><button
          class="button button-primary"
          :disabled="
            mutating ||
            !isPositiveAmount(freezeForm.points)
          "
        >
          {{ t('admin.copy.239b2c58') }}
        </button>
      </form>
      <form v-write-guard="mutating" novalidate v-inline-validation
        v-if="writeAdjust"
        class="panel form-panel"
        @submit.prevent="withGlobalLoading(adjustPoints)"
      >
        <div class="panel-title">
          <div>
            <h2>{{ t('admin.copy.f9f97fca') }}</h2>
            <p>{{ t('admin.copy.f4b46d95') }}</p>
          </div>
        </div>
        <label
          >{{ t('admin.copy.56bc02a1') }}<select v-model="adjustForm.source">
            <option value="recharge">{{ t('admin.copy.6ffe1179') }}</option>
            <option value="winning">{{ t('admin.copy.99c31497') }}</option>
            <option value="gift">{{ t('admin.copy.2b8ae17c') }}</option>
            <option value="commission">{{ t('admin.copy.101e2e66') }}</option>
          </select></label
        ><label
          >{{ t('admin.copy.10025d7b') }}<input
            v-model.trim="adjustForm.delta"
            inputmode="text"
            required
            :placeholder="t('admin.copy.9a348780')" /></label
        ><button
          class="button button-primary"
          :disabled="
            mutating ||
            !isSignedAmount(adjustForm.delta)
          "
        >
          {{ t('admin.copy.cb99f7c9') }}
        </button>
      </form>
    </section>

      <section v-if="viewWallet" class="panel">
        <div class="panel-title">
          <div>
            <h2>{{ t('admin.copy.de1c63a8') }}</h2>
          </div>
        </div>
        <p v-if="!entries.length" class="muted empty-state">{{ t('admin.copy.8249da4b') }}</p>
        <article v-for="entry in entries" :key="entry.id" class="ledger-entry">
          <div class="entry-head">
            <div>
              <b>{{ entryTypeName(entry.entry_type) }}</b
              ><small>{{ displayDate(entry.created_at) }}</small>
            </div>
            <code>{{ entry.id }}</code>
          </div>
          <p v-if="entry.reason" class="reason-text">{{ entry.reason }}</p>
          <p v-if="entry.source_allocation.length" class="allocation">
            {{ t('admin.copy.3169ecbf') }}{{
              entry.source_allocation
                .map(
                  (item) =>
                    `${sourceName(item.source)} / ${stateName(item.state)} ${formatIntegerAmount(item.points)}`,
                )
                .join(" · ")
            }}
          </p>
          <label class="snapshot-toggle"><input v-model="fullSnapshotEntryIds" type="checkbox" :value="entry.id" />{{ t('admin.finance.fullSnapshot') }}</label>
          <div class="snapshot-grid">
          <span class="snapshot-heading">{{ t('admin.copy.3aeb0ec1') }}</span
            ><span class="snapshot-heading">{{ t('admin.copy.f85d9f7f') }}</span
            ><span class="snapshot-heading">{{ t('admin.copy.574031a3') }}</span
            ><span class="snapshot-heading">{{ t('admin.copy.6db1d337') }}</span
            ><template v-for="source in sources" :key="`${entry.id}-${source}`"
              ><template
                v-for="state in states.filter(state => fullSnapshotEntryIds.includes(entry.id) || entry.delta_snapshot[source][state] !== '0')"
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
            <summary>{{ t('admin.copy.927756e7') }}</summary>

          </details>
          <div
            v-if="
              writeFreeze &&
              unfreezeCandidates.some((item) => item.id === entry.id)
            "
            class="unfreeze-row"
          >
            <button
              v-write-guard="mutating"
              type="button"
              class="button button-secondary"
              :disabled="mutating "
              @click="withGlobalLoading(unfreeze, entry)"
            >
              {{ t('admin.copy.f5c98e2d') }}
            </button>
          </div>
        </article>
        <footer class="pager">
          <span>{{ t('admin.copy.f3d61d27', { page: String(ledgerOffset / PAGE_SIZE + 1) }) }}</span>
          <div>
            <button
              type="button"
              :disabled="ledgerOffset === 0 || loading"
              @click="withGlobalLoading(moveLedgerPage, -1)"
            >
              {{ t('admin.copy.dbe7a5d7') }}</button
            ><button
              type="button"
              :disabled="!hasNextLedger || loading"
              @click="withGlobalLoading(moveLedgerPage, 1)"
            >
              {{ t('admin.copy.2e9c9e7a') }}
            </button>
          </div>
        </footer>
      </section>
      </div>
    </template>

    <section v-show="view === 'recharges'" v-if="viewRecharges" class="panel">
      <div class="panel-title">
        <div>
          <h2>{{ t('admin.copy.ae419781') }}</h2>
          <p>
            {{ loadedMemberId ? `${t('admin.copy.56b2d19b')} ${loadedMemberUsername}` : t('admin.copy.42010bdb') }} ·
            {{ t('admin.copy.90badab0', { count: "50" }) }}
          </p>
        </div>
        <button v-refresh-button="t('admin.copy.855b9442')"
          v-if="!loadedMemberId"
          type="button"
          class="button button-secondary"
          :disabled="loading"
          @click="withGlobalLoading(loadRechargeQueue)"
        >
          {{ t('admin.copy.855b9442') }}
        </button>
      </div>
      <p v-if="!recharges.length" class="muted empty-state">{{ t('admin.copy.dcbdb099') }}</p>
      <div v-if="recharges.length" class="table-wrap"><table class="console-recharges-table console-responsive-table"><thead><tr>
        <th>{{ t('admin.console.rechargeOrder') }}</th><th>{{ t('admin.copy.92c59de3') }}</th><th>{{ t('admin.console.points') }}</th><th>{{ t('admin.console.state') }}</th><th>{{ t('admin.console.created') }}</th><th>{{ t('admin.console.actions') }}</th>
      </tr></thead><tbody><template v-for="recharge in recharges" :key="recharge.id"><tr>
        <td :data-label="t('admin.console.rechargeOrder')">{{ recharge.id }}</td><td :data-label="t('admin.copy.92c59de3')"><AssociationName :record="recharge" kind="member" :id="recharge.member_id" :locale="associationLocale" /></td>
        <td :data-label="t('admin.console.points')">{{ formatIntegerAmount(recharge.points) }}</td><td :data-label="t('admin.console.state')"><span class="status" :class="recharge.state">{{ recharge.state === 'pending' ? t('admin.copy.7746bbe1') : recharge.state === 'confirmed' ? t('admin.copy.5f313d26') : t('admin.copy.a454aa98') }}</span></td>
        <td :data-label="t('admin.console.created')">{{ displayDate(recharge.created_at) }}</td><td :data-label="t('admin.console.actions')"><button type="button" class="text-button" :aria-expanded="expandedRechargeId === recharge.id" @click="expandedRechargeId = expandedRechargeId === recharge.id ? '' : recharge.id">{{ t('admin.copy.01db40da') }}</button></td>
      </tr>
      <tr v-show="expandedRechargeId === recharge.id" class="console-recharge-detail"><td colspan="6"><article class="recharge-row">
        <div class="recharge-head">
          <div>
            <b>{{ formatIntegerAmount(recharge.points) }} {{ t('admin.console.points') }}</b
            ><small><AssociationName :record="recharge" kind="member" :id="recharge.member_id" :locale="associationLocale" /> {{ displayDate(recharge.created_at) }}</small>
          </div>
          <span class="status" :class="recharge.state">{{
            recharge.state === "pending"
              ? t('admin.copy.7746bbe1')
              : recharge.state === "confirmed"
                ? t('admin.copy.5f313d26')
                : t('admin.copy.a454aa98')
          }}</span>
        </div>
        <p v-if="recharge.proof_reference">
          {{ t('admin.copy.bb54d12f') }}{{ recharge.proof_reference }}
        </p>
        <p v-if="recharge.remark">{{ t('admin.copy.d19e55e6') }}{{ recharge.remark }}</p>
        <p>
          {{ t('admin.copy.6a2d3973') }} <AssociationName :record="recharge" kind="admin" :id="recharge.created_by" :locale="associationLocale" /><span v-if="recharge.confirmed_by">
            {{ t('admin.copy.faa0379b') }} <AssociationName :record="recharge" kind="admin" :id="recharge.confirmed_by" :locale="associationLocale" /></span
          ><span v-if="recharge.ledger_entry_id">
            {{ t('admin.copy.0fc64455') }} {{ recharge.ledger_entry_id }}</span
          >
        </p>
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
      </article></td></tr>
      </template></tbody></table></div>
      <footer class="pager">
        <span>{{ t('admin.copy.f3d61d27', { page: String(rechargeOffset / PAGE_SIZE + 1) }) }}</span>
        <div>
          <button
            type="button"
            :disabled="rechargeOffset === 0 || loading"
            @click="withGlobalLoading(moveRechargePage, -1)"
          >
            {{ t('admin.copy.dbe7a5d7') }}</button
          ><button
            type="button"
            :disabled="!hasNextRecharge || loading"
            @click="withGlobalLoading(moveRechargePage, 1)"
          >
            {{ t('admin.copy.2e9c9e7a') }}
          </button>
        </div>
      </footer>
    </section>

    <section v-show="view === 'recharges'" v-if="writeRecharge && loadedMemberId" class="panel form-panel">
      <div class="panel-title">
        <div>
          <h2>{{ t('admin.copy.17bc097d') }}</h2>
          <p>{{ t('admin.copy.539fc0f2') }}</p>
        </div>
      </div>
      <form v-write-guard="mutating" novalidate v-inline-validation class="form-grid" @submit.prevent="withGlobalLoading(createRecharge)">
        <label
          >{{ t('admin.copy.8870d6d7') }}<input
            v-model.trim="rechargeForm.points"
            inputmode="numeric"
            required
          /><small>{{ t('admin.copy.fbd67a6f') }}</small></label
        ><label
          >{{ t('admin.copy.fd3b3477') }}<input
            v-model.trim="rechargeForm.proof_reference"
            maxlength="200" /></label
        ><label class="wide"
          >{{ t('admin.copy.412d293a') }}<input
            v-model.trim="rechargeForm.remark"
            maxlength="500" /></label
        ><button
          class="button button-primary"
          :disabled="
            mutating ||
            !isPositiveAmount(rechargeForm.points)
          "
        >
          {{ t('admin.copy.d6ee7013') }}
        </button>
      </form>
    </section>

    <p
      v-if="loadedMemberId && !(writeRecharge || writeFreeze || writeAdjust)"
      class="callout"
    >
      {{ t('admin.copy.196bbdcd') }}
    </p>
  </section>
</template>

<style scoped>
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
  font-size: 13px;
  line-height: 1.55;
}
.eyebrow {
  font-size: 13px !important;
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
  font-size: 13px;
  white-space: nowrap;
}
.server-note,
.callout {
  border-radius: 8px;
  background: var(--canvas);
  color: var(--ink-secondary);
  padding: 10px 12px;
  font-size: 13px;
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
  font-size: 13px;
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
  font-size: 13px;
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
  font-size: 13px;
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
  font-size: 13px;
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
  font-size: 13px;
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
  font-size: 13px;
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
  font-size: 13px;
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
  font-size: 13px;
}
.entry-head > div,
.recharge-head > div {
  display: grid;
  gap: 4px;
  min-width: 0;
}
.entry-head small,
.recharge-head small {
  font-size: 13px;
  color: var(--muted);
  overflow-wrap: anywhere;
}
.entry-head code {
  font-size: 13px;
  color: var(--muted);
  overflow-wrap: anywhere;
}
.reason-text,
.allocation,
.recharge-row > p {
  font-size: 13px;
  color: var(--ink-secondary);
  overflow-wrap: anywhere;
  line-height: 1.55;
}
.snapshot-toggle { display: flex; align-items: center; gap: 8px; min-height: 44px; color: var(--muted); font-size: 13px; }
.snapshot-grid {
  display: grid;
  grid-template-columns: minmax(120px, 1.2fr) repeat(3, minmax(0, 1fr));
  gap: 6px 8px;
  align-items: baseline;
  font-size: 13px;
  font-variant-numeric: tabular-nums;
}
.snapshot-grid > b {
  font-weight: 500;
  color: var(--ink-secondary);
  overflow-wrap: anywhere;
}
.snapshot-grid > span {
  text-align: right;
  overflow-wrap: anywhere;
}
.snapshot-heading {
  font-size: 13px;
  font-weight: 700;
  color: var(--muted);
}
.ledger-entry details {
  font-size: 13px;
  color: var(--muted);
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
  color: var(--muted);
  font-size: 13px;
}
.pager > div {
  display: flex;
  gap: 7px;
}
.pager button {
  border: 1px solid var(--line);
  border-radius: 6px;
  background: var(--surface);
  padding: 6px 9px;
  color: var(--ink-secondary);
  font-size: 13px;
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
  font-size: 13px;
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
.operations-grid .button-primary {
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
    font-size: 13px;
  }
  .snapshot-heading {
    font-size: 13px;
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
    font-size: 13px;
  }
}
.finance-breakdown { margin-top: 12px; }
.finance-breakdown > summary { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 44px; cursor: pointer; color: var(--ink-secondary); font-weight: 600; list-style: none; }
.finance-breakdown > summary::-webkit-details-marker { display: none; }
.finance-breakdown[open] > summary { margin-bottom: 8px; }
.finance-breakdown[open] > summary :deep(svg) { transform: rotate(180deg); }
</style>
