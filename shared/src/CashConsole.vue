<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { ApiError } from "./api";
import { createCatalogI18n } from "./localization";
import { publicApiErrorText } from "./public-api-errors";
import { cashCatalog, type CashKey } from "./cash-catalog";
import {
  createCashApi,
  pendingCashWrites,
  currencies,
  emptyDestination,
  minor,
  decimal,
  type Currency,
  type CashConfig,
  type CashDestination,
  type CashOrder,
} from "./cash";
import {
  useOperationToast,
  vInlineValidation,
  vWriteGuard,
  withGlobalLoading,
  formatDisplayDateTime,
} from "./index";
import ManagementList from "./ManagementList.vue";
import ManagementTable from "./ManagementTable.vue";
import TablePagination from "./TablePagination.vue";
import DetailModal from "./DetailModal.vue";
import SelectControl from "./SelectControl.vue";


import CashDestinationFields from "./CashDestinationFields.vue";
const props = defineProps<{
  basePath: string;
  brandId?: string;
  actorId: string;
  locale: "en" | "zh-CN";
  platform?: boolean;
  mode: "settings" | "deposit" | "withdrawal";
  permissions: string[];
}>();
const emit = defineEmits<{ failure: [cause: unknown] }>();
const i18n = createCatalogI18n(cashCatalog, props.locale),
  t = i18n.t;
const label = (key: CashKey) => t(key);
watch(
  () => props.locale,
  (v) => i18n.setLocale(v),
);
const recordKind = ref<string>(props.mode);
const api = computed(() =>
  createCashApi(
    props.basePath,
    props.platform && props.mode === "settings" ? undefined : props.brandId,
  ),
);
const config = ref<CashConfig | null>(null),
  destinations = ref<CashDestination[]>([]),
  orders = ref<CashOrder[]>([]),
  total = ref("0"),
  page = ref(1),
  size = ref(20),
  accountPage = ref(1),
  accountSize = ref(10);
const selected = ref<CashOrder | null>(null),
  draft = ref<CashDestination | null>(null),
  currency = ref<Currency>("CNY"),
  busy = ref(false),
  loading = ref(false),
  error = ref(""),
  notice = ref(""),
  actual = ref(""),
  reference = ref(""),
  proof = ref(""),
  operation = ref("");
const pending = ref<{
  kind: "config" | "destination" | "order";
  path: string;
  body: unknown;
  key: string;
} | null>(null);
const locked = computed(() => busy.value || !!pending.value);
const allowed = (resource: string, action: string) =>
  props.permissions.includes(
    `${resource}.${action}.${props.platform ? "platform" : "brand"}`,
  );
const canRead = computed(() =>
  props.mode === "settings"
    ? allowed(props.platform ? "platform_settings" : "point_policy", "view")
    : allowed(props.mode === "deposit" ? "recharge" : "withdrawal", "view"),
);
const canWrite = computed(() =>
  allowed(props.platform ? "platform_settings" : "point_policy", "write"),
);

const accountCurrencies = computed(() => currencies.filter(value =>
  config.value?.rules.some(rule => rule.currency === value && rule.enabled),
));
const showAccountList = computed(() => !props.platform && config.value?.rules.some(rule => rule.enabled));
const visibleAccounts = computed(() => destinations.value.filter(value => accountCurrencies.value.includes(value.currency)));
const pagedAccounts = computed(() =>
  visibleAccounts.value.slice(
    (accountPage.value - 1) * accountSize.value,
    accountPage.value * accountSize.value,
  ),
);
const accountColumns = computed(() => [
  { key: "currency", label: t("cash.currency") },
  { key: "number", label: t("cash.number") },
  { key: "status", label: t("cash.status") },
  ...(canWrite.value ? [{ key: "actions", label: t("cash.edit") }] : []),
]);
const columns = computed(() => [
  { key: "member", label: t("cash.member") },
  { key: "amount", label: t("cash.amount") },
  { key: "status", label: t("cash.status") },
  { key: "created", label: t("cash.created") },
  { key: "actions", label: t("cash.details") },
]);
const clone = <T,>(value: T): T => JSON.parse(JSON.stringify(value));
let generation = 0;
function message(cause: unknown) {
  const keys: Record<string, CashKey> = {
    CASH_INPUT_INVALID: "cash.invalid",
    CASH_ACCOUNT_INCOMPLETE: "cash.accountRequired",
    CASH_NOT_CONFIGURED: "cash.notConfigured",
    CASH_INSUFFICIENT: "cash.insufficient",
    CASH_LIMIT_EXCEEDED: "cash.limitExceeded",
    CASH_RATE_CHANGED: "cash.rateChanged",
  };
  return cause instanceof ApiError && cause.code && keys[cause.code]
    ? t(keys[cause.code])
    : publicApiErrorText(cause, props.locale);
}
async function load(target = page.value) {
  if (!canRead.value) return;
  const token = generation;
  loading.value = true;
  error.value = "";
  try {
    if (props.mode === "settings") {
      const value = await api.value.config();
      let ds: CashDestination[] = [];
      if (!props.platform) ds = await api.value.destinations();
      if (token !== generation) return;
      config.value =
        pending.value?.kind === "config"
          ? clone(pending.value.body as CashConfig)
          : value;
      destinations.value = ds;
    } else {
      const result = await api.value.orders(
        recordKind.value,
        target,
        size.value,
      );
      if (token !== generation) return;
      if (result.items.some((o) => o.brand_id !== props.brandId))
        throw new ApiError("Scope mismatch", 502, "INVALID_RESPONSE");
      orders.value = result.items;
      total.value = result.total_count;
      page.value = target;
    }
  } catch (cause) {
    if (token === generation) {
      error.value = message(cause);
      emit("failure", cause);
    }
  } finally {
    if (token === generation) loading.value = false;
  }
}
function addAccount() {
  currency.value = accountCurrencies.value[0];
  draft.value = emptyDestination(currency.value);
}
function editAccount(d: CashDestination) {
  currency.value = d.currency;
  draft.value = clone(d);
}
function changeCurrency(value: Currency) {
  currency.value = value;
  draft.value = emptyDestination(value);
}
function show(o: CashOrder) {
  selected.value = o;
  actual.value = o.automatic ? decimal(minor(o.requested_amount) + minor(o.fee)) : o.amount;
  reference.value = o.reference;
  proof.value = "";
  operation.value = "";
}
function actions(o: CashOrder): string[] {
  if (props.platform) return [];
  if (o.kind === "deposit")
    return ["pending", "submitted", "expired"].includes(o.state)
      ? ["confirm", "reject"]
          .filter((a) => a !== "reject" || o.state !== "expired")
          .filter((a) =>
            allowed("recharge", a === "reject" ? "cancel" : "confirm"),
          )
      : [];
  if (o.kind !== "withdrawal") return [];
  return (
    o.state === "reviewing"
      ? ["approve", "reject"]
      : o.state === "processing"
        ? ["paid", "fail"]
        : []
  ).filter((a) => allowed("withdrawal", a === "paid" ? "mark_paid" : a));
}
function actionLabel(action: string) {
  return t(
    action === "confirm"
      ? "cash.confirmDeposit"
      : action === "paid"
        ? "cash.markPaid"
        : `cash.${action}`,
  );
}
async function save(kind: "config" | "destination" | "order", action = "") {
  if (busy.value) return;
  const token = generation;
  if (!pending.value) {
    let body: unknown,
      path = "";
    if (kind === "config") body = config.value;
    else if (kind === "destination") body = draft.value;
    else {
      body = {
        version: selected.value!.version,
        amount: actual.value,
        reference: reference.value,
        proof: proof.value,
      };
      path = `/cash/orders/${selected.value!.id}/${action}`;
    }
    pending.value = { kind, path, body: clone(body), key: crypto.randomUUID() };
  }
  const frozen = pending.value;
  pendingCashWrites.set(
    `${props.basePath}:${props.brandId}:${props.actorId}:${props.mode}`,
    {
      ...frozen,
      destination: frozen.kind === "destination",
      config: frozen.kind === "config",
      dialog: frozen.kind,
      detail: selected.value ? clone(selected.value) : null,
    },
  );
  busy.value = true;
  error.value = "";
  try {
    if (frozen.kind === "config")
      await api.value.saveConfig(frozen.body as CashConfig, frozen.key);
    else if (frozen.kind === "destination")
      await api.value.saveDestination(
        frozen.body as CashDestination,
        frozen.key,
      );
    else await api.value.writeOrder(frozen.path, frozen.body, frozen.key);
    if (token !== generation) return;
    pending.value = null;
    pendingCashWrites.delete(
      `${props.basePath}:${props.brandId}:${props.actorId}:${props.mode}`,
    );
    draft.value = null;
    selected.value = null;
    operation.value = "";
    notice.value = t("cash.saved");
    await load();
  } catch (cause) {
    if (token !== generation) return;
    error.value = message(cause);
    emit("failure", cause);
    if (
      cause instanceof ApiError &&
      cause.status &&
      cause.status >= 400 &&
      cause.status < 500
    ) {
      pending.value = null;
      pendingCashWrites.delete(
        `${props.basePath}:${props.brandId}:${props.actorId}:${props.mode}`,
      );
    }
  } finally {
    if (token === generation) busy.value = false;
  }
}
const net = computed(() => {
  try {
    return selected.value
      ? decimal(minor(selected.value.amount) - minor(selected.value.fee))
      : "0";
  } catch {
    return "0";
  }
});
watch(
  () => [props.brandId, props.actorId, props.mode],
  () => {
    generation++;

    config.value = null;
    destinations.value = [];
    orders.value = [];
    selected.value = null;
    draft.value = null;
    pending.value = null;
    busy.value = false;
    accountPage.value = page.value = 1;
    recordKind.value = props.mode;
    const saved = pendingCashWrites.get(
      `${props.basePath}:${props.brandId}:${props.actorId}:${props.mode}`,
    );
    if (saved) {
      pending.value = {
        ...saved,
        kind: saved.config
          ? "config"
          : saved.destination
            ? "destination"
            : "order",
      };
      selected.value = saved.detail;
      if (saved.destination) {
        draft.value = clone(saved.body as CashDestination);
        currency.value = draft.value.currency;
      }
      if (saved.detail) {
        operation.value = saved.path.split("/").at(-1)!;
        const body = saved.body as Record<string, unknown>;
        if (typeof body.amount === "string") actual.value = body.amount;
        if (typeof body.reference === "string")
          reference.value = body.reference;
        if (typeof body.proof === "string") proof.value = body.proof;
      }
    }
    void load(1);
  },
  { immediate: true },
);
onUnmounted(() => generation++);
useOperationToast(error, (v) => v, "error");
useOperationToast(notice, (v) => v, "success");
</script>
<template>
  <div class="cash-console console-stack">
    <p v-if="!canRead" class="empty-state">
      {{
        publicApiErrorText(new ApiError("", 403, "PERMISSION_DENIED"), locale)
      }}
    </p>
    <template v-else-if="mode === 'settings'">
      <section class="brand-settings-panel panel">
        <form
          v-if="config && canWrite"
          novalidate
          v-inline-validation
          v-write-guard="locked"
          class="ui-form"
          @submit.prevent="withGlobalLoading(save, 'config')"
        >
          <fieldset
            v-for="rule in config.rules"
            :key="rule.currency"
            :disabled="locked || !canWrite"
            class="cash-rule"
          >
            <legend>{{ t(`cash.${rule.currency}`) }}</legend>
            <label class="checkbox-label"
              ><input v-model="rule.enabled" type="checkbox" />{{
                t("cash.enabled")
              }}</label
            >
            <div v-if="rule.enabled" class="cash-rule-fields">
              <label v-if="platform"
                >{{ t("cash.rate")
                }}<input
                  v-model="rule.rate"
                  inputmode="decimal"
                  :required="rule.enabled"
                  pattern="[0-9]+([.][0-9]{1,6})?" :data-validation-format="t('cash.numberFormat',{digits:6})"
              /></label>
              <label
                v-for="field in [
                  'deposit_min',
                  'deposit_max',
                  'withdrawal_min',
                  'withdrawal_max',
                  'fee',
                ] as const"
                :key="field"
                >{{
                  t(
                    platform && field === "fee"
                      ? "cash.feeMax"
                      : `cash.${field}`,
                  )
                }}<input
                  v-model="rule[field]"
                  inputmode="decimal"
                  required
                  :data-validation-format="t('cash.numberFormat',{digits:rule.currency==='USDT'?6:2})"
                  :pattern="
                    rule.currency === 'USDT'
                      ? '[0-9]+([.][0-9]{1,6})?'
                      : '[0-9]+([.][0-9]{1,2})?'
                  "
              /></label>
              <template v-if="!platform"
                ><label
                  >{{ t("cash.rotation")
                  }}<SelectControl
                    v-model="rule.mode"
                    :label="t('cash.rotation')"
                    :options="[
                      { value: 'count', label: t('cash.count') },
                      { value: 'amount', label: t('cash.volume') },
                    ]" /></label
                ><label
                  >{{ t("cash.threshold")
                  }}<input
                    v-model="rule.threshold"
                    :inputmode="rule.mode === 'count' ? 'numeric' : 'decimal'"
                    required
                    :data-validation-format="rule.mode==='count'?t('cash.countFormat'):t('cash.numberFormat',{digits:rule.currency==='USDT'?6:2})"
                    :pattern="
                      rule.mode === 'count'
                        ? '[1-9][0-9]*'
                        : rule.currency === 'USDT'
                          ? '[0-9]+([.][0-9]{1,6})?'
                          : '[0-9]+([.][0-9]{1,2})?'
                    " /></label
              ></template>
            </div>
          </fieldset>
          <div v-if="canWrite" class="ui-form-actions">
            <button
              type="submit"
              class="button button-primary"
              :disabled="locked || loading"
            >
              {{ t("cash.save") }}
            </button>
          </div>
        </form>
        <template v-if="config && !canWrite">
          <section
            v-for="rule in config.rules"
            :key="rule.currency"
            class="cash-rule"
          >
            <h3>{{ t(`cash.${rule.currency}`) }}</h3>
            <dl class="cash-facts">
              <div>
                <dt>{{ t("cash.status") }}</dt>
                <dd>
                  {{ t(rule.enabled ? "cash.enabled" : "cash.disabled") }}
                </dd>
              </div>
              <template v-if="rule.enabled"><div v-if="platform">
                <dt>{{ t("cash.rate") }}</dt>
                <dd>{{ rule.rate || "—" }}</dd>
              </div>
              <div
                v-for="field in [
                  'deposit_min',
                  'deposit_max',
                  'withdrawal_min',
                  'withdrawal_max',
                  'fee',
                ] as const"
                :key="field"
              >
                <dt>
                  {{
                    t(
                      platform && field === "fee"
                        ? "cash.feeMax"
                        : `cash.${field}`,
                    )
                  }}
                </dt>
                <dd>{{ rule[field] }}</dd>
              </div>
              <template v-if="!platform"
                ><div>
                  <dt>{{ t("cash.rotation") }}</dt>
                  <dd>
                    {{
                      t(rule.mode === "count" ? "cash.count" : "cash.volume")
                    }}
                  </dd>
                </div>
                <div>
                  <dt>{{ t("cash.threshold") }}</dt>
                  <dd>{{ rule.threshold }}</dd>
                </div></template
              >
              </template>
            </dl>
          </section>
        </template>
        <div v-if="pending?.kind === 'config' && !busy" class="ui-form-actions">
          <p role="status">{{ t("cash.unknown") }}</p>
          <button
            class="button button-primary"
            @click="withGlobalLoading(save, 'config')"
          >
            {{ t("cash.retry") }}
          </button>
        </div>
      </section>
      <ManagementList v-if="showAccountList" :busy="loading">
        <template #heading
          ><h2>{{ t("cash.accounts") }}</h2></template
        ><template v-if="canWrite" #actions
          ><button
            class="button button-primary"
            :disabled="locked"
            @click="addAccount"
          >
            {{ t("cash.addAccount") }}
          </button></template
        >
        <ManagementTable
          :entries="pagedAccounts"
          :columns="accountColumns"
          :empty="!visibleAccounts.length"
          :empty-label="t('cash.empty')"
          ><template #currency="{ entry }">{{
            t(`cash.${entry.currency}`)
          }}</template
          ><template #number="{ entry }"
            ><span class="cash-break"
              >{{ entry.institution || entry.channel }} ·
              {{ entry.number }}</span
            ></template
          ><template #status="{ entry }">{{
            entry.enabled ? t("cash.enabled") : t("cash.disabled")
          }}</template
          ><template #actions="{ entry }"
            ><button
              class="text-button"
              :disabled="!canWrite || locked"
              @click="editAccount(entry)"
            >
              {{ t("cash.edit") }}
            </button></template
          ></ManagementTable
        >
        <template #pagination
          ><TablePagination
            :page="accountPage"
            :count="pagedAccounts.length"
            :total="String(visibleAccounts.length)"
            :page-size="accountSize"
            :locale="locale"
            :busy="locked"
            :go-to="(target) => (accountPage = target)"
            :change-page-size="
              (value) => {
                accountSize = value;
                accountPage = 1;
              }
            "
        /></template>
      </ManagementList>
    </template>
    <ManagementList v-else :busy="loading">
      <template #heading
        ><h2>
          {{ t(mode === "deposit" ? "cash.deposit" : "cash.withdrawal") }}
        </h2></template
      ><template v-if="mode === 'deposit'" #filters
        ><SelectControl
          :model-value="recordKind"
          :label="t('cash.kind')"
          :disabled="loading || locked"
          :options="
            ['deposit', 'to_points', 'from_points'].map((v) => ({
              value: v,
              label: t(`cash.${v}`),
            }))
          "
          @change="
            recordKind = String($event);
            withGlobalLoading(load, 1);
          " /></template
      >
      <ManagementTable
        :entries="orders"
        :columns="columns"
        :empty="!orders.length && !loading && !error"
        :empty-label="t('cash.empty')"
        ><template #member="{ entry }">{{ entry.member_account }}</template
        ><template #amount="{ entry }"
          >{{ entry.amount }} {{ entry.currency }}</template
        ><template #status="{ entry }">{{ t(`cash.${entry.state}`) }}</template
        ><template #created="{ entry }">{{
          formatDisplayDateTime(entry.created_at, locale)
        }}</template
        ><template #actions="{ entry }"
          ><button class="text-button" :disabled="locked" @click="show(entry)">
            {{ t("cash.details") }}
          </button></template
        ></ManagementTable
      >
      <template #pagination
        ><TablePagination
          :page="page"
          :count="orders.length"
          :total="total"
          :page-size="size"
          :locale="locale"
          :busy="loading || locked"
          :go-to="(target) => load(target)"
          :change-page-size="
            async (value) => {
              size = value;
              await load(1);
            }
          "
      /></template>
    </ManagementList>
    <DetailModal
      v-if="draft"
      :title="t('cash.accounts')"
      :close-label="t('cash.close')"
      :close-disabled="locked"
      @close="draft = null"
      ><form
        novalidate
        v-inline-validation
        v-write-guard="locked"
        class="ui-form"
        @submit.prevent="withGlobalLoading(save, 'destination')"
      >
        <fieldset :disabled="locked">
          <label
            >{{ t("cash.currency")
            }}<SelectControl
              :model-value="currency"
              :disabled="!!draft.id"
              :label="t('cash.currency')"
              :options="
                accountCurrencies.map((v) => ({ value: v, label: t(`cash.${v}`) }))
              "
              @change="changeCurrency($event as Currency)" /></label
          ><CashDestinationFields v-model="draft" :t="label" />
        </fieldset>
        <div class="ui-form-actions">
          <button
            type="submit"
            class="button button-primary"
            :disabled="locked"
          >
            {{ t("cash.save") }}
          </button>
        </div>
      </form>
      <div v-if="pending && !busy" class="ui-form-actions">
        <p>{{ t("cash.unknown") }}</p>
        <button
          class="button button-primary"
          @click="withGlobalLoading(save, 'destination')"
        >
          {{ t("cash.retry") }}
        </button>
      </div></DetailModal
    >
    <DetailModal
      v-if="selected"
      :title="t('cash.details')"
      :close-label="t('cash.close')"
      :close-disabled="locked"
      @close="selected = null"
    >
      <dl class="cash-facts">
        <div>
          <dt>{{ t("cash.member") }}</dt>
          <dd>{{ selected.member_account }}</dd>
        </div>
        <div>
          <dt>{{ t(selected.automatic ? "cash.principal" : "cash.amount") }}</dt>
          <dd>{{ selected.amount }} {{ selected.currency }}</dd>
        </div>
        <template v-if="selected.automatic">
          <div><dt>{{ t(selected.state === 'confirmed' ? 'cash.actual' : 'cash.payable') }}</dt><dd>{{ decimal(minor(selected.state === 'confirmed' ? selected.amount : selected.requested_amount) + minor(selected.fee)) }} {{ selected.currency }}</dd></div>
          <div><dt>{{ t('cash.fee') }}</dt><dd>{{ selected.fee }} {{ selected.currency }}</dd></div>
        </template>
        <div v-if="selected.requested_amount !== selected.amount">
          <dt>{{ t("cash.requested") }}</dt>
          <dd>{{ selected.requested_amount }} {{ selected.currency }}</dd>
        </div>
        <div v-if="selected.points !== '0'">
          <dt>{{ t("cash.points") }}</dt>
          <dd>{{ selected.points }}</dd>
        </div>
        <div v-if="selected.rate">
          <dt>{{ t("cash.rateQuote") }}</dt>
          <dd>
            1 {{ selected.currency }} = {{ selected.rate }}
            {{ t("cash.points") }}
          </dd>
        </div>
        <div>
          <dt>{{ t("cash.status") }}</dt>
          <dd>{{ t(`cash.${selected.state}`) }}</dd>
        </div>
        <template v-if="selected.kind === 'withdrawal'"
          ><div>
            <dt>{{ t("cash.fee") }}</dt>
            <dd>{{ selected.fee }} {{ selected.currency }}</dd>
          </div>
          <div>
            <dt>{{ t("cash.receive") }}</dt>
            <dd>{{ net }} {{ selected.currency }}</dd>
          </div></template
        >
        <div v-if="selected.destination.id">
          <dt>{{ t("cash.accounts") }}</dt>
          <dd class="cash-break">
            {{ selected.destination.holder }}
            {{
              selected.destination.institution || selected.destination.channel
            }}
            · {{ selected.destination.number }}
          </dd>
        </div>
        <div v-if="selected.reference">
          <dt>
            {{
              t(selected.currency === "USDT" ? "cash.txid" : "cash.reference")
            }}
          </dt>
          <dd class="cash-break">{{ selected.reference }}</dd>
        </div>
        <div v-if="selected.proof">
          <dt>{{ t("cash.proof") }}</dt>
          <dd>{{ selected.proof }}</dd>
        </div>
      </dl>
      <div v-if="!operation" class="ui-form-actions">
        <button
          v-for="action in actions(selected)"
          :key="action"
          class="button button-secondary"
          :disabled="locked"
          @click="operation = action"
        >
          {{ actionLabel(action) }}
        </button>
      </div>
      <form
        v-else
        novalidate
        v-inline-validation
        v-write-guard="locked"
        class="ui-form"
        @submit.prevent="withGlobalLoading(save, 'order', operation)"
      >
        <fieldset :disabled="locked">
          <label v-if="operation === 'confirm'"
            >{{ t("cash.actual")
            }}<input
              v-model="actual"
              required
              inputmode="decimal"
              :data-validation-format="t('cash.numberFormat',{digits:selected.currency==='USDT'?6:2})"
              :pattern="
                selected.currency === 'USDT'
                  ? '[0-9]+([.][0-9]{1,6})?'
                  : '[0-9]+([.][0-9]{1,2})?'
              " /></label
          ><label v-if="['confirm', 'paid'].includes(operation)"
            >{{
              t(selected.currency === "USDT" ? "cash.txid" : "cash.reference")
            }}<input
              v-model.trim="reference"
              required
              :maxlength="selected.currency === 'USDT' ? 64 : 200" /></label
          ><label v-if="operation === 'fail'"
            >{{ t("cash.paymentFailure")
            }}<textarea v-model.trim="proof" required maxlength="500" />
          </label>
        </fieldset>
        <div class="ui-form-actions">
          <button class="button button-primary" :disabled="locked">
            {{ actionLabel(operation) }}</button
          ><button
            type="button"
            class="button button-secondary"
            :disabled="locked"
            @click="operation = ''"
          >
            {{ t("cash.back") }}
          </button>
        </div>
      </form>
      <div v-if="pending && !busy" class="ui-form-actions">
        <p>{{ t("cash.unknown") }}</p>
        <button
          class="button button-primary"
          @click="withGlobalLoading(save, 'order', operation)"
        >
          {{ t("cash.retry") }}
        </button>
      </div>
    </DetailModal>
  </div>
</template>
<style scoped>
.cash-tron-settings{display:grid;gap:12px;min-width:0}
@media(max-width:900px),(pointer:coarse){.cash-tron-settings{gap:8px}}
.cash-rule {
  min-width: 0;
  margin: 0;
  padding: 12px 0;
  border: 0;
  border-bottom: 1px solid var(--line);
  display: grid;
  gap: 8px;
}
.cash-rule legend {
  font-weight: 650;
  padding: 0;
}
.cash-rule-fields {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 190px), 1fr));
  gap: 8px 12px;
}
.cash-rule-fields label {
  min-width: 0;
}
.cash-break {
  overflow-wrap: anywhere;
  word-break: break-word;
}
.cash-facts {
  display: grid;
  gap: 8px;
}
.cash-facts > div {
  display: flex;
  justify-content: space-between;
  gap: 12px;
}
.cash-facts dt {
  color: var(--muted);
  flex-shrink: 0;
}
.cash-facts dd {
  margin: 0;
  min-width: 0;
  text-align: end;
  font-variant-numeric: tabular-nums;
}
.cash-console h2 {
  margin: 0;
  font-size: 20px;
}
.cash-console fieldset {
  min-width: 0;
}
@media (max-width: 400px) {
  .cash-facts > div {
    flex-wrap: wrap;
  }
}
</style>
