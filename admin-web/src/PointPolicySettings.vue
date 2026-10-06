<script setup lang="ts">
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
const { t, message } = useAdminI18n();
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
    Boolean(draft.value.reason.trim()) &&
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
      error.value = message("策略响应与当前品牌不匹配，请重新读取。", "The policy response does not match the current brand. Reload the policy.");
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
    error.value = cause instanceof AdminApiError && cause.status === 409
      ? cause.message
        ? message("{serverError}；策略版本已变化，请刷新后重试。", "{serverError} The policy version changed. Refresh and try again.", { serverError: cause.message })
        : message("策略版本已变化，请刷新后重试。", "The policy version changed. Refresh and try again.")
      : cause instanceof Error ? cause.message : message("读取积分策略失败。", "Failed to load the points policy.");
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
      error.value = message("保存响应与当前品牌不匹配；请重新读取确认结果。", "The save response does not match the current brand. Reload to confirm the result.");
      return;
    }
    policy.value = result;
    draft.value.max_balance_points = result.max_balance_points ?? "";
    draft.value.max_recharge_points = result.max_recharge_points ?? "";
    draft.value.max_adjustment_points = result.max_adjustment_points ?? "";
    draft.value.reason = "";
    notice.value = message("积分策略已保存并立即生效。", "Points policy saved and applied immediately.");
  } catch (cause) {
    if (!current(generation, brandId)) return;
    if (cause instanceof AdminApiError && cause.status === 401)
      emit("session-invalid");
    error.value = cause instanceof AdminApiError && cause.status === 409
      ? cause.message
        ? message("{serverError}；版本冲突，未覆盖当前表单。请刷新后再处理。", "{serverError} Version conflict. The current form was not overwritten. Refresh before continuing.", { serverError: cause.message })
        : message("版本冲突，未覆盖当前表单。请刷新后再处理。", "Version conflict. The current form was not overwritten. Refresh before continuing.")
      : cause instanceof Error ? cause.message : message("保存积分策略失败，当前策略未更新。", "Failed to save the points policy. The current policy was not changed.");
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
        <p class="eyebrow">{{ t("POINT POLICY", "POINT POLICY") }}</p>
        <h2>{{ t("积分限额策略", "Points policy settings") }}</h2>
        <p>{{ t("仅设置积分余额、充值和调整限额；不会直接编辑会员余额。", "Set limits for point balances, recharges, and adjustments; this does not directly edit member balances.") }}</p>
      </div>
      <span class="brand-tag">{{ t("品牌 · {brand}", "Brand · {brand}", { brand: brandId || t("未选择", "Not selected") }) }}</span>
    </header>

    <p class="server-note">
      {{ t("页面权限仅控制操作入口；服务端授权与版本校验始终有效。限额留空表示不限。", "Page permissions control access to the controls only; server authorization and version checks always apply. Leave a limit blank for no limit.") }}
    </p>
    <p v-if="!canView" class="callout">
      {{ t("当前账号缺少 point_policy.view 权限，无法读取此品牌策略。", "This account lacks the point_policy.view permission and cannot read this brand's policy.") }}
    </p>
    <template v-else>
      <div v-if="error" class="notice error" role="alert">
        {{ t(error) }}
        <button type="button" :disabled="loading || saving" @click="loadPolicy">
          {{ t("重新读取", "Reload") }}
        </button>
      </div>
      <p v-if="notice" class="notice success" role="status">{{ t(notice) }}</p>
      <div v-if="loading && !policy" class="callout" role="status">
        {{ t("正在读取当前品牌策略…", "Loading the current brand policy…") }}
      </div>
      <section v-if="policy" class="policy-card">
        <div class="card-head">
          <div>
            <h3>{{ t("当前策略", "Current policy") }}</h3>
            <p>{{ t("版本 {version} · {brand}", "Version {version} · {brand}", { version: policy.version, brand: policy.brand_id }) }}</p>
          </div>
          <button
            type="button"
            class="secondary"
            :disabled="loading || saving"
            @click="loadPolicy"
          >
            {{ t("刷新", "Refresh") }}
          </button>
        </div>
        <dl class="current-limits">
          <div>
            <dt>{{ t("余额上限", "Balance limit") }}</dt>
            <dd>{{ policy.max_balance_points ?? t("不限", "No limit") }}</dd>
          </div>
          <div>
            <dt>{{ t("单笔充值上限", "Per-recharge limit") }}</dt>
            <dd>{{ policy.max_recharge_points ?? t("不限", "No limit") }}</dd>
          </div>
          <div>
            <dt>{{ t("单次调整上限", "Per-adjustment limit") }}</dt>
            <dd>{{ policy.max_adjustment_points ?? t("不限", "No limit") }}</dd>
          </div>
        </dl>
        <p v-if="policy.audit_log_id" class="audit-id">
          {{ t("审计记录：{id}", "Audit record: {id}", { id: policy.audit_log_id }) }}
        </p>
      </section>

      <p v-if="!canWrite && policy" class="callout">
        {{ t("当前账号仅有读取权限；无法修改积分策略。", "This account has read-only access and cannot change the points policy.") }}
      </p>
      <form
        v-if="canWrite && policy"
        class="policy-card policy-form"
        @submit.prevent="savePolicy"
      >
        <div class="card-head">
          <div>
            <h3>{{ t("编辑限额", "Edit limits") }}</h3>
            <p>{{ t("提交时使用当前版本执行并发校验；保存后立即生效。", "The current version is checked when submitted; changes take effect immediately after saving.") }}</p>
          </div>
        </div>
        <label
          >{{ t("余额上限（正整数；留空表示不限）", "Balance limit (positive integer; blank means no limit)") }}
          <input
            v-model.trim="draft.max_balance_points"
            inputmode="numeric"
            autocomplete="off"
            :placeholder="t('不限', 'No limit')"
          />
          <small>{{ t("只能输入规范正整数，不接受 0、小数、正负号或前导零。", "Enter a canonical positive integer. Zero, decimals, signs, and leading zeros are not accepted.") }}</small>
        </label>
        <label
          >{{ t("单笔充值上限（正整数；留空表示不限）", "Per-recharge limit (positive integer; blank means no limit)") }}
          <input
            v-model.trim="draft.max_recharge_points"
            inputmode="numeric"
            autocomplete="off"
            :placeholder="t('不限', 'No limit')"
          />
        </label>
        <label
          >{{ t("单次调整上限（正整数；留空表示不限）", "Per-adjustment limit (positive integer; blank means no limit)") }}
          <input
            v-model.trim="draft.max_adjustment_points"
            inputmode="numeric"
            autocomplete="off"
            :placeholder="t('不限', 'No limit')"
          />
        </label>
        <label
          >{{ t("变更原因（必填）", "Reason for change (required)") }}
          <textarea
            v-model.trim="draft.reason"
            rows="3"
            maxlength="500"
            required
          />
        </label>
        <p v-if="!validLimits" class="field-error" role="alert">
          {{ t("限额必须是规范的正整数，或留空表示不限。", "Each limit must be a canonical positive integer, or blank for no limit.") }}
        </p>
        <button class="primary" type="submit" :disabled="!canSave">
          {{ saving ? t("保存中…", "Saving…") : t("保存策略", "Save policy") }}
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
  background: #fff;
  border: 1px solid #e9ebf0;
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
  font-size: 10px;
  color: #737b8c;
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
  border: 1px solid #eceef3;
  border-radius: 8px;
  padding: 11px;
  min-width: 0;
}
.current-limits dt {
  font-size: 10px;
  color: #737b8c;
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
  background: #fff;
  padding: 9px 10px;
  color: #252a36;
  font: inherit;
}
.policy-form textarea {
  resize: vertical;
}
.policy-form small {
  font-size: 9px;
  font-weight: 400;
  color: #8991a0;
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
  justify-self: start;
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
  font-size: 10px;
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
