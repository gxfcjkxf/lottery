<script setup lang="ts">
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { vWriteGuard } from "@lottery/shared";
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
const displayDate = (value: string | null | undefined) => formatDisplayDateTime(value, displayLocale.value);
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import { vInlineValidation } from '@lottery/shared'
import { computed, ref, watch } from "vue";
import { useAdminI18n } from "./i18n";
import type { LocalizedMessage } from "@lottery/shared";
import type { AdminAccount } from "./admin-api";
import { AdminApiError } from "./admin-api";
import { lookupAdminMember } from "./member-lookup";
import {
  brandPermissions,
  canViewWallet,
  formatIntegerAmount,
  createBodyKeyTracker,
} from "./finance-api";
import {
  createRepairApi,
  type RepairPreview,
  type RepairHistory,
} from "./repair-api";
const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (e: "session-invalid"): void }>();
const api = createRepairApi();
const { t, message, display } = useAdminI18n();
const keyFor = createBodyKeyTracker();
const member = ref("");
const loaded = ref("");
const loadedUsername = ref("");
const preview = ref<RepairPreview | null>(null);
const history = ref<RepairHistory[]>([]);
const reason = ref("");
const acknowledge = ref(false);
const busy = ref(false);
const error = ref<string | LocalizedMessage>("");
const notice = ref<string | LocalizedMessage>("");
let generation = 0;
const view = computed(() => canViewWallet(props.account, props.brandId));
const write = computed(
  () =>
    !props.account.super_admin &&
    brandPermissions(props.account, props.brandId).has("wallet.repair.brand"),
);
const sources = ['recharge', 'winning', 'gift', 'commission'] as const;
const states = ['available', 'manual_frozen', 'system_frozen', 'withdrawal'] as const;
const sourceLabels = {recharge: 'admin.copy.6ffe1179', winning: 'admin.copy.99c31497', gift: 'admin.copy.2b8ae17c', commission: 'admin.copy.101e2e66'} as const;
const stateLabels = {available: 'admin.copy.e9231c7d', manual_frozen: 'admin.copy.3817714f', system_frozen: 'admin.copy.c4073e10', withdrawal: 'admin.copy.f56770cc'} as const;
const differences = computed(() => preview.value ? sources.flatMap(source => states.map(state => ({source, state, actual: preview.value!.actual[source]?.[state], expected: preview.value!.expected[source][state]}))).filter(row => row.actual !== row.expected) : []);
function failed(e: unknown) {
  error.value = e instanceof Error ? publicErrorMessage(e) : message('admin.copy.7e4e1c23');
  if (e instanceof AdminApiError && e.status === 401) emit("session-invalid");
}
watch(
  () => props.brandId,
  () => {
    generation++;
    preview.value = null;
    history.value = [];
    loaded.value = "";
    loadedUsername.value = "";
    reason.value = "";
    acknowledge.value = false;
    notice.value = "";
    error.value = "";
    busy.value = false;
    keyFor.clear();
  },
);
watch(member, () => {
  preview.value = null;
  acknowledge.value = false;
  notice.value = "";
});
async function inspect() {
  const username = member.value.trim();
  if (!username) return;
  const gen = ++generation,
    brand = props.brandId;
  busy.value = true;
  error.value = "";
  notice.value = "";
  preview.value = null;
  acknowledge.value = false;
  try {
    const id = (await lookupAdminMember(brand, username)).id;
    const [p, h] = await Promise.all([
      api.preview(brand, id),
      api.history(brand, id),
    ]);
    if (gen !== generation || member.value.trim() !== username) return;
    loaded.value = id;
    loadedUsername.value = username;
    preview.value = p;
    history.value = h.items;
  } catch (e) {
    if (gen === generation) failed(e);
  } finally {
    if (gen === generation) busy.value = false;
  }
}
async function repair() {
  const p = preview.value;
  if (
    !p?.repairable ||
    !write.value ||
    !acknowledge.value  ||
    member.value.trim() !== loadedUsername.value
  )
    return;
  const brand = props.brandId,
    id = loaded.value,
    gen = ++generation,
    body = { version: p.version, token: p.token, reason: reason.value.trim() };
  busy.value = true;
  error.value = "";
  try {
    const result = await api.repair(
      brand,
      id,
      body,
      keyFor({ brand, id, ...body }),
    );
    if (gen !== generation) return;
    const [next, h] = await Promise.all([
      api.preview(brand, id),
      api.history(brand, id),
    ]);
    if (gen !== generation) return;
    preview.value = next;
    history.value = h.items;
    notice.value = message('admin.copy.60b8df1f', { id: result.id });
    reason.value = "";
    acknowledge.value = false;
    keyFor.clear();
  } catch (e) {
    if (gen === generation) {
      failed(e);
      preview.value = null;
      acknowledge.value = false;
    }
  } finally {
    if (gen === generation) busy.value = false;
  }
}
</script>
<template>
  <article v-if="view" class="balance-repair">
    <h2>{{ t('admin.copy.e5e9afad') }}</h2>
    <p>
      {{ t('admin.copy.521c574a') }}
    </p>
    <form novalidate v-inline-validation @submit.prevent="withGlobalLoading(inspect)">
      <label
        >{{ t('admin.copy.e4b644ab') }}<input
          v-model="member"
          autocomplete="off"
          :disabled="busy" /></label
      ><button :disabled="busy">{{ t('admin.copy.52ad0d49') }}</button>
    </form>
    <p v-if="error" role="alert">{{ display(error) }}</p>
    <p v-if="notice" role="status">{{ display(notice) }}</p>
    <section v-if="preview">
      <p>
        {{
          preview.consistent
            ? t('admin.copy.a37ee945')
            : preview.repairable
              ? t('admin.copy.c6fb6b89')
              : t('admin.copy.61fba81e')
        }}
      </p>

      <table v-if="differences.length" class="repair-differences">
        <thead><tr><th>{{ t('admin.copy.3aeb0ec1') }}</th><th>{{ t('admin.copy.a501130b') }}</th><th>{{ t('admin.copy.bf702ff9') }}</th></tr></thead>
        <tbody><tr v-for="row in differences" :key="`${row.source}-${row.state}`"><td>{{ t(sourceLabels[row.source]) }} · {{ t(stateLabels[row.state]) }}</td><td>{{ row.actual === undefined ? t('admin.repair.missingBucket') : formatIntegerAmount(row.actual) }}</td><td>{{ formatIntegerAmount(row.expected) }}</td></tr></tbody>
      </table>
      <details>
        <summary>{{ t('admin.repair.rawDiagnostics') }}</summary>
        <ul v-if="preview.issues.length"><li v-for="issue in preview.issues" :key="issue">{{ issue }}</li></ul>
        <div class="snapshots"><section><h3>{{ t('admin.copy.a501130b') }}</h3><BusinessDetails :value="preview.actual" :locale="associationLocale" /></section><section><h3>{{ t('admin.copy.bf702ff9') }}</h3><BusinessDetails :value="preview.expected" :locale="associationLocale" /></section></div>
      </details>
      <form v-write-guard="busy" novalidate v-inline-validation v-if="write && preview.repairable" @submit.prevent="withGlobalLoading(repair)">

        <label class="ack"
          ><input
            v-model="acknowledge"
            type="checkbox"
            :disabled="busy"
          />{{ t('admin.copy.97584ee3') }}</label
        ><button :disabled="busy || !acknowledge ">
          {{ t('admin.copy.db995c77') }}
        </button>
      </form>
    </section>
    <h3>{{ t('admin.copy.4b60d7be') }}</h3>
    <p v-if="!history.length" class="empty-state">{{ t('admin.copy.0f272253') }}</p>
    <details v-for="item in history" :key="item.id">
      <summary>{{ displayDate(item.created_at) }}<span v-if="item.reason"> {{ item.reason }}</span></summary>

      <BusinessDetails :value="{ before: item.before_snapshot, after: item.after_snapshot }" :locale="associationLocale" />
    </details>
  </article>
</template>
<style scoped>
.balance-repair {
  background: white;
  padding: 20px;
  border: 1px solid var(--line);
  border-radius: 12px;
  display: grid;
  gap: 14px;
  color: var(--ink);
}
.balance-repair h2,
.balance-repair h3,
.balance-repair p {
  margin: 0;
}
.balance-repair p {
  font-size: 13px;
  line-height: 1.6;
  overflow-wrap: anywhere;
}
form,
label {
  display: grid;
  gap: 10px;
}
input,
textarea {
  width: 100%;
  padding: 10px;
  border: 1px solid var(--line);
  border-radius: 6px;
  box-sizing: border-box;
}
button {
  justify-self: start;
  padding: 10px 14px;
  border: 0;
  border-radius: 6px;
  background: var(--primary);
  color: white;
  cursor: pointer;
}
button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.repair-differences { width: 100%; table-layout: fixed; border-collapse: collapse; font-size: 13px; }
.repair-differences th, .repair-differences td { text-align: left; padding: 10px 6px; border-bottom: 1px solid var(--line); overflow-wrap: anywhere; }
summary { min-height: 44px; align-content: center; cursor: pointer; color: var(--muted); font-size: 13px; }
.snapshots {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  min-width: 0;
}
.snapshots section {
  min-width: 0;
}
pre {
  background: var(--canvas);
  padding: 12px;
  border-radius: 6px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-size: 12px;
  max-height: 400px;
  overflow: auto;
}
.ack {
  display: flex;
  align-items: center;
  font-size: 13px;
}
.ack input {
  width: auto;
}
details {
  min-width: 0;
}
summary {
  overflow-wrap: anywhere;
  font-size: 13px;
  cursor: pointer;
}
@media (max-width: 700px) {
  .snapshots {
    grid-template-columns: 1fr;
  }
  .balance-repair {
    padding: 14px;
  }
}
</style>
