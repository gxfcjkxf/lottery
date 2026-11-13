<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { ApiError, useOperationToast, vInlineValidation, vWriteGuard, withGlobalLoading } from '@lottery/shared';
import DetailModal from './DetailModal.vue';
import ManagementList from './ManagementList.vue';
import ManagementTable from './ManagementTable.vue';
import CashDestinationFields from './CashDestinationFields.vue';
import { createCashApi, currencies, emptyDestination, pendingCashWrites, type CashDestination, type CashState, type Currency } from './cash';
import type { CashKey } from './cash-catalog';
const props = defineProps<{
  agent?: boolean;
  apiBase: string;
  member: { id: string; brand_id: string };
  t: (key: CashKey) => string;
  errorText: (cause: unknown) => string;
  initialCurrency?: string;
  returnToWithdrawal?: boolean;
}>();
const emit = defineEmits<{ 'auth-expired': []; done: [] }>();
const state = ref<CashState | null>(null), loading = ref(false), busy = ref(false);
const draft = ref<CashDestination | null>(null), deleting = ref(false);
const error = ref(''), notice = ref('');
const scope = computed(() => `${props.apiBase}:${props.member.brand_id}:${props.member.id}:accounts`);
const pending = ref<{ body: CashDestination; key: string; deleting: boolean } | null>(null);
const locked = computed(() => busy.value || !!pending.value);
const api = computed(() => createCashApi(props.apiBase, undefined, state.value?.actor_context));
const columns = computed(() => [
  { key: 'currency', label: props.t('cash.currency') },
  { key: 'account', label: props.t('cash.accounts') },
  { key: 'actions', label: props.t('cash.operation') },
]);
const rows = computed(() => currencies.map(currency => ({ id: currency, currency, account: state.value?.destinations.find(d => d.currency === currency) })));
let generation = 0, readTicket = 0;
function fail(cause: unknown) {
  error.value = cause instanceof ApiError && cause.code === 'CASH_ACCOUNT_EXISTS' ? props.t('cash.accountExists') : props.errorText(cause);
  if (cause instanceof ApiError && (cause.status === 401 || cause.code === 'AUTH_ACTOR_CONTEXT_CHANGED')) emit('auth-expired');
}
async function load() {
  const token = generation, ticket = ++readTicket;
  loading.value = true;
  try {
    const next = await api.value.state();
    if (token !== generation || ticket !== readTicket) return;
    if (next.brand_id !== props.member.brand_id || next.member_id !== props.member.id) throw new ApiError('Scope mismatch', 403, 'AUTH_ACTOR_CONTEXT_CHANGED');
    state.value = next;
    return true;
  } catch (cause) { if (token === generation && ticket === readTicket) fail(cause); }
  finally { if (token === generation && ticket === readTicket) loading.value = false; }
}
function edit(currency: Currency, remove = false) {
  if (locked.value || loading.value) return;
  const account = state.value?.destinations.find(d => d.currency === currency);
  draft.value = account ? { ...account, enabled: true } : emptyDestination(currency);
  deleting.value = remove;
  error.value = '';
}
function close() { if (!locked.value) { draft.value = null; deleting.value = false; } }
async function save() {
  if (busy.value || !draft.value || !state.value) return;
  const token = generation, owner = scope.value;
  pending.value ??= { body: { ...draft.value }, key: crypto.randomUUID(), deleting: deleting.value };
  const frozen = pending.value;
  pendingCashWrites.set(owner, { path: frozen.deleting ? '/cash/destinations/delete' : '/cash/destinations', body: frozen.body, key: frozen.key, destination: true, config: false, dialog: frozen.deleting ? 'delete' : 'account', detail: null });
  busy.value = true; error.value = '';
  try {
    if (frozen.deleting) await api.value.deleteDestination({ id: frozen.body.id, version: frozen.body.version }, frozen.key);
    else await api.value.saveDestination(frozen.body, frozen.key);
    pendingCashWrites.delete(owner);
    if (token !== generation) return;
    pending.value = null; draft.value = null; deleting.value = false;
    notice.value = props.t('cash.success');
    const loaded = await load();
    if (token !== generation) return;
    if (loaded && props.returnToWithdrawal && !frozen.deleting) emit('done');
  } catch (cause) {
    if (token !== generation) return;
    if (cause instanceof ApiError && cause.status && cause.status >= 400 && cause.status < 500) {
      pending.value = null; pendingCashWrites.delete(owner);
      if (cause.status === 409) { draft.value = null; deleting.value = false; await load(); }
    }
    if (token !== generation) return;
    fail(cause);
  } finally { if (token === generation) busy.value = false; }
}
watch(() => [props.apiBase, props.member.brand_id, props.member.id], async () => {
  const token = ++generation;
  state.value = null; draft.value = null; pending.value = null; busy.value = false; error.value = ''; notice.value = '';
  const saved = pendingCashWrites.get(scope.value);
  if (saved) {
    const body = saved.body as CashDestination;
    pending.value = { body: { ...body }, key: saved.key, deleting: saved.dialog === 'delete' };
    draft.value = { ...body }; deleting.value = saved.dialog === 'delete';
  }
  const loaded = await load();
  if (loaded && token === generation && !pending.value && props.initialCurrency && currencies.includes(props.initialCurrency as Currency)) edit(props.initialCurrency as Currency);
}, { immediate: true, flush: 'sync' });
onBeforeUnmount(() => { generation++; });
useOperationToast(error, v => v, 'error');
useOperationToast(notice, v => v, 'success');
</script>
<template>
  <section class="cash-accounts" :class="{ 'cash-accounts--user': !agent }" :aria-busy="loading">
    <ManagementList :busy="loading || locked">
      <template v-if="$slots.actions || returnToWithdrawal" #actions><slot name="actions" /><button v-if="returnToWithdrawal" type="button" class="button button-secondary" :disabled="locked" @click="emit('done')">{{ t('cash.returnWithdrawal') }}</button></template>
      <button v-if="!state && !loading" type="button" class="button button-secondary" @click="withGlobalLoading(load)">{{ t('cash.retryLoad') }}</button>
      <ManagementTable v-if="state" :columns="columns" :entries="rows" :empty="false" :empty-label="t('cash.empty')">
        <template #currency="{ entry: row }">{{ t(`cash.${row.currency}`) }}</template>
        <template #account="{ entry: row }"><div v-if="row.account" class="cash-account-summary"><span>{{ row.account.institution || row.account.channel }}<template v-if="row.account.holder"> · {{ row.account.holder }}</template></span><span>{{ '•••• ' + row.account.number.slice(-6) }}</span></div><span v-else>{{ t('cash.unbound') }}</span></template>
        <template #actions="{ entry: row }"><div class="cash-account-actions"><button type="button" class="text-button" :disabled="locked || loading" @click="edit(row.currency)">{{ t(row.account ? 'cash.edit' : 'cash.addAccount') }}</button><button v-if="row.account" type="button" class="text-button" :disabled="locked || loading" @click="edit(row.currency, true)">{{ t('cash.deleteAccount') }}</button></div></template>
      </ManagementTable>
    </ManagementList>
    <DetailModal v-if="draft" class="ui-form-dialog" :title="t(deleting ? 'cash.deleteAccount' : 'cash.accounts')" :close-label="t('cash.close')" :close-disabled="locked" @close="close">
      <form class="ui-form" novalidate v-inline-validation v-write-guard="busy" @submit.prevent="withGlobalLoading(save)">
        <p v-if="deleting">{{ t('cash.deleteAccountConfirm') }}</p>
        <fieldset :disabled="locked">
          <div class="cash-account-field"><div class="cash-account-label">{{ t('cash.currency') }}</div><div>{{ t(`cash.${draft.currency}`) }}</div></div>
          <div v-if="draft.currency === 'USDT'" class="cash-account-field"><div class="cash-account-label">{{ t('cash.network') }}</div><div>TRON/TRC20</div></div>
          <CashDestinationFields v-if="!deleting" v-model="draft" :t="t" :manage-status="false" />
          <dl v-else><div><dt>{{ t(draft.currency === 'USDT' ? 'cash.address' : 'cash.number') }}</dt><dd>{{ draft.number }}</dd></div></dl>
        </fieldset>
        <p v-if="pending" class="muted" role="status">{{ t('cash.unknown') }}</p>
        <div class="ui-form-actions"><button type="submit" class="button button-primary" :disabled="busy || loading || !state">{{ t(pending ? 'cash.retry' : deleting ? 'cash.confirm' : 'cash.save') }}</button></div>
      </form>
    </DetailModal>
  </section>
</template>
<style scoped>
.cash-accounts :deep(.management-list > .management-list-toolbar) { position: static; width: auto; height: auto; min-height: 0; margin: 0; overflow: visible; clip-path: none; }
.cash-account-summary { display: grid; gap: 4px; overflow-wrap: anywhere; }
.cash-account-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 4px 8px; }
/* User pages use the same list structure with the existing user card tokens. */
.cash-accounts--user :deep(.management-list) { background: var(--surface); border: 1px solid var(--line); border-radius: var(--radius); box-shadow: var(--shadow); min-width: 0; }
.cash-accounts--user :deep(.management-list-toolbar) { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 8px; padding: 12px; border-radius: var(--radius) var(--radius) 0 0; }
.cash-accounts--user :deep(h3) { margin: 0; font-size: var(--body-font-size); }
.cash-accounts--user :deep(.console-table) { width: 100%; border-collapse: collapse; text-align: left; }
.cash-accounts--user :deep(th), .cash-accounts--user :deep(td) { padding: 8px var(--table-cell-padding-inline); border-bottom: 1px solid var(--line); overflow-wrap: anywhere; }
.cash-accounts--user :deep(th) { color: var(--muted); background: var(--table-header-bg); }
@media (max-width: 900px), (pointer: coarse) {
  .cash-accounts--user :deep(.management-list-toolbar) { padding-block: 8px; }
}
@media (max-width: 700px) {
  .cash-account-actions { justify-content: flex-start; }
  .cash-accounts:not(.cash-accounts--user) :deep(td:nth-child(2)) { grid-column: 1 / -1; }
  .cash-accounts--user :deep(thead) { display: none; }
  .cash-accounts--user :deep(tbody) { display: grid; }
  .cash-accounts--user :deep(tr) { display: grid; grid-template-columns: minmax(0, 1fr); padding: 8px; border-top: 1px solid var(--line); }
  .cash-accounts--user :deep(td) { display: grid; grid-template-columns: minmax(0, 30%) minmax(0, 1fr); gap: 4px 8px; min-width: 0; border: 0; padding-block: 4px; position: static; text-align: left; }
  .cash-accounts--user :deep(td)::before { content: attr(data-label); color: var(--muted); }
}
</style>
