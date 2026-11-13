<script setup lang="ts">
import { vRefreshButton } from "@lottery/shared";
import { useOperationToast } from "@lottery/shared";
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
import { withGlobalLoading } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
import { vInlineValidation } from '@lottery/shared'
import { computed, onMounted, onBeforeUnmount, ref, watch } from "vue";
import type { AdminAccount } from "./admin-api";
import {
  canViewPointPolicy,
  canWritePointPolicy,
  createPointPolicyApi,
  createPointPolicyKeyTracker,
  isCanonicalPositiveLimit,
  type PointPolicy,
  type PointPolicyAccount,
  type UpdatePointPolicyBody,
} from "./point-policy-api";
import { AdminApiError } from "./admin-api";
import { useAdminI18n } from "./i18n";
import type { LocalizedMessage } from "@lottery/shared";

const props = defineProps<{
  account: AdminAccount & Partial<PointPolicyAccount>;
  brandId: string;
}>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createPointPolicyApi();
const { t, message, display } = useAdminI18n();
const policy = ref<PointPolicy | null>(null);
const draft = ref({
  max_balance_points: "",
  max_recharge_points: "",
  max_adjustment_points: "",
  reason: "",
});
const loading = ref(false);
const saving = ref(false);
const error = ref<string | LocalizedMessage>("");
const notice = ref<string | LocalizedMessage>("");
const keyForBody = createPointPolicyKeyTracker();
let requestGeneration = 0;
const canView = computed(() =>
  canViewPointPolicy(props.account, props.brandId),
);
const canWrite = computed(() =>
  canWritePointPolicy(props.account, props.brandId),
);
const validLimits = computed(() =>
  [
    draft.value.max_balance_points,
    draft.value.max_recharge_points,
    draft.value.max_adjustment_points,
  ].every((value) => value === "" || isCanonicalPositiveLimit(value)),
);
const canSave = computed(
  () =>
    canView.value &&
    canWrite.value &&
    policy.value !== null &&
    validLimits.value &&
    !loading.value &&
    !saving.value,
);

function current(generation: number, brandId: string) {
  return (
    generation === requestGeneration &&
    props.brandId === brandId &&
    canView.value
  );
}

async function loadPolicy() {
  const brandId = props.brandId;
  const generation = ++requestGeneration;
  error.value = "";
  notice.value = "";
  if (!canView.value) {
    policy.value = null;
    loading.value = false;
    return;
  }
  loading.value = true;
  try {
    const result = await api.getPointPolicy(brandId);
    if (!current(generation, brandId)) return;
    if (result.brand_id !== brandId) {
      error.value = message('admin.copy.701e9a9e');
      policy.value = null;
      return;
    }
    policy.value = result;
    draft.value.max_balance_points = result.max_balance_points ?? "";
    draft.value.max_recharge_points = result.max_recharge_points ?? "";
    draft.value.max_adjustment_points = result.max_adjustment_points ?? "";
    draft.value.reason = "";
  } catch (cause) {
    if (!current(generation, brandId)) return;
    policy.value = null;
    if (cause instanceof AdminApiError && cause.status === 401)
      emit("session-invalid");
    error.value = publicErrorMessage(cause, 'read');
  } finally {
    if (current(generation, brandId)) loading.value = false;
  }
}

async function savePolicy() {
  if (!canSave.value || !policy.value) return;
  const brandId = props.brandId;
  const generation = ++requestGeneration;
  const body: UpdatePointPolicyBody = {
    version: policy.value.version,
    max_balance_points: draft.value.max_balance_points || null,
    max_recharge_points: draft.value.max_recharge_points || null,
    max_adjustment_points: draft.value.max_adjustment_points || null,
    reason: draft.value.reason.trim(),
  };
  const keyInput = { brand_id: brandId, ...body };
  const idempotencyKey = keyForBody(keyInput);
  saving.value = true;
  error.value = "";
  notice.value = "";
  try {
    const result = await api.updatePointPolicy(brandId, body, idempotencyKey);
    if (!current(generation, brandId)) return;
    if (result.brand_id !== brandId) {
      error.value = message('admin.copy.6b3f515b');
      return;
    }
    policy.value = result;
    draft.value.max_balance_points = result.max_balance_points ?? "";
    draft.value.max_recharge_points = result.max_recharge_points ?? "";
    draft.value.max_adjustment_points = result.max_adjustment_points ?? "";
    draft.value.reason = "";
    notice.value = message('admin.copy.a41b44f9');
  } catch (cause) {
    if (!current(generation, brandId)) return;
    if (cause instanceof AdminApiError && cause.status === 401)
      emit("session-invalid");
    error.value = publicErrorMessage(cause);
  } finally {
    if (current(generation, brandId)) saving.value = false;
  }
}

onMounted(() => void loadPolicy());
watch(
  () => [props.account.id, props.brandId, canView.value] as const,
  () => {
    requestGeneration++;
    policy.value = null;
    draft.value = {
      max_balance_points: "",
      max_recharge_points: "",
      max_adjustment_points: "",
      reason: "",
    };
    error.value = "";
    notice.value = "";
    loading.value = false;
    saving.value = false;
    void loadPolicy();
  },
  { flush: "sync" },
);
onBeforeUnmount(() => { requestGeneration++; });

useOperationToast(() => error.value, value => display(value), 'error');
useOperationToast(() => notice.value, value => display(value), 'success');
</script>

<template>
  <section class="point-policy-settings brand-settings-panel">
    <header class="policy-head">
      <div>
        <h2>{{ t('admin.copy.b127d28a') }}</h2>
      </div>
    </header>

    <p v-if="!canView" class="callout" role="status">
      {{ t('admin.copy.c47f4b0a') }}
    </p>
    <template v-else>
      <div v-if="error" class="operation-recovery-actions">

        <button v-refresh-button="t('admin.copy.deca021e')" type="button" class="button button-secondary" :disabled="loading || saving" @click="withGlobalLoading(loadPolicy)">
          {{ t('admin.copy.deca021e') }}
        </button>
      </div>

      <div v-global-loading v-if="loading && !policy" class="callout" role="status">
        {{ t('admin.copy.dc40b640') }}
      </div>
      <section v-if="policy && !canWrite" class="policy-card">
        <dl class="current-limits">
          <div>
            <dt>{{ t('admin.copy.3b96f2f6') }}</dt>
            <dd>{{ policy.max_balance_points ?? t('admin.copy.bd97fa74') }}</dd>
          </div>
          <div>
            <dt>{{ t('admin.copy.81ed90dd') }}</dt>
            <dd>{{ policy.max_recharge_points ?? t('admin.copy.bd97fa74') }}</dd>
          </div>
          <div>
            <dt>{{ t('admin.copy.0237942c') }}</dt>
            <dd>{{ policy.max_adjustment_points ?? t('admin.copy.bd97fa74') }}</dd>
          </div>
        </dl>

      </section>


      <form v-write-guard="saving" novalidate v-inline-validation
        v-if="canWrite && policy"
        class="policy-card policy-form ui-form"
        @submit.prevent="withGlobalLoading(savePolicy)"
      >
        <label
          >{{ t('admin.copy.48b813d8') }}
          <input
            v-model.trim="draft.max_balance_points"
            inputmode="decimal"
            autocomplete="off"
            :placeholder="t('admin.copy.bd97fa74')"
          />
        </label>
        <label
          >{{ t('admin.copy.17cf892f') }}
          <input
            v-model.trim="draft.max_recharge_points"
            inputmode="decimal"
            autocomplete="off"
            :placeholder="t('admin.copy.bd97fa74')"
          />
        </label>
        <label
          >{{ t('admin.copy.09ef0918') }}
          <input
            v-model.trim="draft.max_adjustment_points"
            inputmode="decimal"
            autocomplete="off"
            :placeholder="t('admin.copy.bd97fa74')"
          />
        </label>

        <p v-if="!validLimits" class="field-error" role="alert">
          {{ t('admin.copy.d51ced59') }}
        </p>
        <div class="ui-form-actions"><button class="button button-primary" type="submit" :disabled="!canSave">
          {{ saving ? t('admin.copy.2462134b') : t('admin.copy.780ef330') }}
        </button></div>
      </form>
    </template>
  </section>
</template>

<style scoped>
.policy-head h2 { margin: 0; font-size: 16px; }
.policy-form, .current-limits { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; margin: 0; min-width: 0; }
.policy-form label { display: grid; gap: 6px; min-width: 0; }
.policy-form input { width: 100%; min-width: 0; box-sizing: border-box; }
.policy-form .field-error, .policy-form > .ui-form-actions { grid-column: 1 / -1; }
.current-limits div { min-width: 0; }
.current-limits dt { color: var(--muted); }
.current-limits dd { margin: 4px 0 0; font-weight: 600; overflow-wrap: anywhere; }
@media (max-width: 620px) { .policy-form, .current-limits { grid-template-columns: minmax(0, 1fr); } }
</style>
