<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
import { vInlineValidation } from '@lottery/shared'
import { computed, onMounted, ref, watch } from "vue";
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
const { locale, t, message, display } = useAdminI18n();
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
    error.value = publicErrorMessage(cause);
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
  () => [props.brandId, canView.value] as const,
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
);
</script>

<template>
  <section class="point-policy-settings">
    <header class="policy-head">
      <div>
        <h2>{{ t('admin.copy.b127d28a') }}</h2>
      </div>
      <span class="brand-tag">{{ t('admin.copy.9c32661d') }} · <AssociationName kind="brand" :id="brandId" :locale="locale" /></span>
    </header>

    <p class="server-note">
      {{ t('admin.copy.68d54c01') }}
    </p>
    <p v-if="!canView" class="callout">
      {{ t('admin.copy.c47f4b0a') }}
    </p>
    <template v-else>
      <div v-if="error" class="notice error" role="alert">
        {{ display(error) }}
        <button v-refresh-button="t('admin.copy.deca021e')" type="button" :disabled="loading || saving" @click="withGlobalLoading(loadPolicy)">
          {{ t('admin.copy.deca021e') }}
        </button>
      </div>
        <p v-if="notice" class="notice success" role="status">{{ display(notice) }}</p>
      <div v-global-loading v-if="loading && !policy" class="callout" role="status">
        {{ t('admin.copy.dc40b640') }}
      </div>
      <section v-if="policy" class="policy-card">
        <div class="card-head">
          <div>
            <h3>{{ t('admin.copy.477adee5') }}</h3>
            <p> · <AssociationName :record="policy" kind="brand" :id="policy.brand_id" :locale="locale" /></p>
          </div>
          <button v-refresh-button="t('admin.copy.894e2a7d')"
            type="button"
            class="button button-secondary"
            :disabled="loading || saving"
            @click="withGlobalLoading(loadPolicy)"
          >
            {{ t('admin.copy.894e2a7d') }}
          </button>
        </div>
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

      <p v-if="!canWrite && policy" class="callout">
        {{ t('admin.copy.80bb8df5') }}
      </p>
      <form v-write-guard="saving" novalidate v-inline-validation
        v-if="canWrite && policy"
        class="policy-card policy-form"
        @submit.prevent="withGlobalLoading(savePolicy)"
      >
        <div class="card-head">
          <div>
            <h3>{{ t('admin.copy.112fc68f') }}</h3>
            <p>{{ t('admin.copy.2389d9f0') }}</p>
          </div>
        </div>
        <label
          >{{ t('admin.copy.48b813d8') }}
          <input
            v-model.trim="draft.max_balance_points"
            inputmode="numeric"
            autocomplete="off"
            :placeholder="t('admin.copy.bd97fa74')"
          />
          <small>{{ t('admin.copy.8e3b7fe0') }}</small>
        </label>
        <label
          >{{ t('admin.copy.17cf892f') }}
          <input
            v-model.trim="draft.max_recharge_points"
            inputmode="numeric"
            autocomplete="off"
            :placeholder="t('admin.copy.bd97fa74')"
          />
        </label>
        <label
          >{{ t('admin.copy.09ef0918') }}
          <input
            v-model.trim="draft.max_adjustment_points"
            inputmode="numeric"
            autocomplete="off"
            :placeholder="t('admin.copy.bd97fa74')"
          />
        </label>

        <p v-if="!validLimits" class="field-error" role="alert">
          {{ t('admin.copy.d51ced59') }}
        </p>
        <button class="button button-primary" type="submit" :disabled="!canSave">
          {{ saving ? t('admin.copy.2462134b') : t('admin.copy.780ef330') }}
        </button>
      </form>
    </template>
  </section>
</template>

<style scoped>
.point-policy-settings {
  width: 100%;
  min-width: 0;
  color: #252a36;
}
.policy-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 14px;
  margin: 4px 0 16px;
}
.policy-head h2 {
  font-size: 19px;
  margin: 3px 0 5px;
}
.policy-head p {
  margin: 0;
  color: var(--muted);
  font-size: 11px;
  line-height: 1.55;
}
.eyebrow {
  font-size: 11px !important;
  letter-spacing: 1.2px;
  color: #8991a2 !important;
  font-weight: 700;
}
.brand-tag {
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
.policy-card {
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: 11px;
  padding: 16px;
  min-width: 0;
  box-shadow: 0 2px 8px #1e2a5008;
  margin: 13px 0;
}
.card-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 13px;
}
.card-head h3 {
  font-size: 14px;
  margin: 0 0 4px;
}
.card-head p,
.audit-id {
  font-size: 11px;
  color: var(--muted);
  line-height: 1.55;
  margin: 0;
  overflow-wrap: anywhere;
}
.current-limits {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
  margin: 0;
}
.current-limits div {
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 11px;
  min-width: 0;
}
.current-limits dt {
  font-size: 11px;
  color: var(--muted);
}
.current-limits dd {
  font-size: 16px;
  font-weight: 700;
  margin: 6px 0 0;
  overflow-wrap: anywhere;
  font-variant-numeric: tabular-nums;
}
.audit-id {
  margin-top: 12px;
}
.policy-form {
  display: grid;
  gap: 12px;
}
.policy-form label {
  display: grid;
  gap: 6px;
  font-size: 11px;
  font-weight: 600;
  min-width: 0;
}
.policy-form input,
.policy-form textarea {
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  border: 1px solid #dfe3eb;
  border-radius: 7px;
  background: var(--surface);
  padding: 9px 10px;
  color: #252a36;
  font: inherit;
}
.policy-form textarea {
  resize: vertical;
}
.policy-form small {
  font-size: 11px;
  font-weight: 400;
  color: #8991a0;
}
.button-primary,
.button-secondary {
  border-radius: 7px;
  padding: 9px 12px;
  font-size: 11px;
  font-weight: 700;
  white-space: nowrap;
}
.button-primary {
  justify-self: start;
  border: 0;
  background: var(--primary);
  color: var(--surface);
}
.button-secondary {
  border: 1px solid #e0e3ea;
  background: white;
  color: #586174;
}
.button-primary:disabled,
.button-secondary:disabled {
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
.error,
.field-error {
  background: #fff0ef;
  color: #a83d36;
}
.success {
  background: #edf8f2;
  color: #287553;
}
.field-error {
  margin: 0;
  border-radius: 7px;
  padding: 8px;
  font-size: 11px;
}
@media (max-width: 600px) {
  .policy-head {
    flex-direction: column;
  }
  .brand-tag {
    white-space: normal;
  }
  .policy-card {
    padding: 13px;
  }
  .current-limits {
    grid-template-columns: minmax(0, 1fr);
  }
  .current-limits dd {
    font-size: 14px;
  }
}
</style>
