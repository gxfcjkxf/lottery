<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import {
  canManage,
  canWrite,
  createBodyKeyTracker,
  createManagementApi,
  type AuthSettings as SettingsRecord,
  type ManagementAccount,
} from "./management-api";

const props = defineProps<{
  account: AdminAccount & Partial<ManagementAccount>;
  brandId: string;
}>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createManagementApi();
const keyForBody = createBodyKeyTracker();
const settings = ref<SettingsRecord | null>(null);
const captcha = ref(false);
const telegram = ref(false);
const telegramClientId = ref("");
const reason = ref("");
const loading = ref(false);
const saving = ref(false);
const error = ref("");
const notice = ref("");
const allowed = computed(() =>
  canManage(props.account, props.brandId, "auth_config"),
);
const writable = computed(() =>
  canWrite(props.account, props.brandId, "auth_config"),
);
const telegramIdValid = computed(
  () =>
    (!telegram.value && telegramClientId.value === "") ||
    (/^[1-9]\d*$/.test(telegramClientId.value) &&
      Number.isSafeInteger(Number(telegramClientId.value))),
);

function showError(cause: unknown) {
  if (cause instanceof AdminApiError && cause.status === 401)
    emit("session-invalid");
  error.value = cause instanceof Error ? cause.message : "请求失败";
  if (cause instanceof AdminApiError && cause.status === 409)
    error.value += "；配置版本已变化，请刷新后重试。";
}
async function reload() {
  if (!allowed.value || !props.brandId) return;
  loading.value = true;
  error.value = "";
  try {
    const result = await api.authSettings(props.brandId);
    settings.value = result;
    captcha.value = result.captcha_enabled;
    telegram.value = result.telegram_enabled;
    telegramClientId.value = result.telegram_client_id;
  } catch (cause) {
    showError(cause);
  } finally {
    loading.value = false;
  }
}
onMounted(() => void reload());
watch(
  () => props.brandId,
  () => {
    settings.value = null;
    reason.value = "";
    void reload();
  },
);

async function save() {
  if (
    !settings.value ||
    saving.value ||
    !reason.value.trim() ||
    !telegramIdValid.value
  )
    return;
  const body = {
    version: settings.value.version,
    captcha_enabled: captcha.value,
    telegram_enabled: telegram.value,
    telegram_client_id: telegramClientId.value.trim(),
    reason: reason.value.trim(),
  };
  saving.value = true;
  error.value = "";
  notice.value = "";
  try {
    const updated = await api.updateAuthSettings(
      props.brandId,
      body,
      keyForBody(body),
    );
    keyForBody.clear();
    settings.value = updated;
    captcha.value = updated.captcha_enabled;
    telegram.value = updated.telegram_enabled;
    telegramClientId.value = updated.telegram_client_id;
    notice.value = "配置已保存并立即生效。";
    reason.value = "";
    await reload();
  } catch (cause) {
    showError(cause);
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <section class="auth-settings">
    <header>
      <div>
        <p class="eyebrow">AUTHENTICATION</p>
        <h1>认证设置</h1>
        <p>配置当前品牌的登录验证选项。</p>
      </div>
      <span class="brand">品牌 · {{ brandId || "未选择" }}</span>
    </header>
    <p v-if="!allowed" class="feedback">
      {{
        writable
          ? "当前账号有认证配置写权限，但缺少 view 权限，无法读取版本并安全保存。"
          : "当前账号没有此品牌的认证配置查看权限。"
      }}
    </p>
    <template v-else>
      <div v-if="loading && !settings" class="card hint">
        正在读取当前品牌认证配置…
      </div>
      <form v-else-if="settings" class="card form" @submit.prevent="save">
        <div class="section-title">
          <div>
            <h2>登录安全</h2>
            <p>保存后设置立即生效；变更会写入审计记录。</p>
          </div>
          <span class="version">配置版本 {{ settings.version }}</span>
        </div>
        <label class="toggle-row"
          ><span
            ><b>启用验证码</b><small>要求登录流程执行验证码校验。</small></span
          ><input v-model="captcha" type="checkbox" :disabled="!writable"
        /></label>
        <label class="toggle-row"
          ><span
            ><b>启用 Telegram 登录</b
            ><small>默认关闭；启用时必须填写有效的公开 Client ID。</small></span
          ><input v-model="telegram" type="checkbox" :disabled="!writable"
        /></label>
        <label
          >Telegram Client ID<input
            v-model.trim="telegramClientId"
            inputmode="numeric"
            autocomplete="off"
            :aria-invalid="!telegramIdValid"
            :readonly="!writable"
          /><small
            >仅接受正的安全整数。此设置只填写公开应用 ID；真实应用授权还需要在
            Telegram 外部完成配置。</small
          ><span v-if="!telegramIdValid" class="field-error"
            >Client ID
            非空时必须是无前导零的正整数安全值；开启登录时必填。</span
          ></label
        >
        <div class="legal">
          <h3>法律文本版本（只读）</h3>
          <dl>
            <div>
              <dt>隐私政策版本</dt>
              <dd>{{ settings.privacy_policy_version || "未设置" }}</dd>
            </div>
            <div>
              <dt>服务条款版本</dt>
              <dd>{{ settings.service_terms_version || "未设置" }}</dd>
            </div>
          </dl>
          <p>此页不会修改版本号或法律文本内容。</p>
        </div>
        <label v-if="writable"
          >变更原因<textarea
            v-model.trim="reason"
            required
            rows="2"
            maxlength="500"
          />
        </label>
        <div class="actions">
          <button
            class="secondary"
            type="button"
            :disabled="loading || saving"
            @click="reload"
          >
            重新读取</button
          ><button
            v-if="writable"
            class="primary"
            :disabled="saving || !reason.trim() || !telegramIdValid"
          >
            {{ saving ? "保存中…" : "保存并立即生效" }}
          </button>
        </div>
        <p v-if="settings.audit_log_id" class="audit">
          最近一次审计记录：{{ settings.audit_log_id }}
        </p>
      </form>
      <p v-if="error" class="feedback error" role="alert">
        {{ error }} <button type="button" @click="reload">刷新配置</button>
      </p>
      <p v-if="notice" class="feedback success" role="status">
        {{ notice
        }}<span v-if="settings?.audit_log_id"
          >审计记录：{{ settings.audit_log_id }}</span
        >
      </p>
    </template>
  </section>
</template>

<style scoped>
.auth-settings {
  width: 100%;
  min-width: 0;
  color: #252a36;
}
.auth-settings header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 14px;
  margin: 4px 0 18px;
}
.auth-settings h1 {
  font-size: 24px;
  margin: 3px 0 5px;
  letter-spacing: -0.5px;
}
.auth-settings header p {
  color: #737b8c;
  margin: 0;
}
.eyebrow {
  font-size: 10px;
  letter-spacing: 1.2px;
  color: #8991a2;
  font-weight: 700;
}
.brand,
.version {
  border-radius: 20px;
  background: #f1f3f8;
  padding: 7px 11px;
  color: #626b7c;
  font-size: 11px;
  overflow-wrap: anywhere;
}
.card {
  background: white;
  border: 1px solid #e9ebf0;
  border-radius: 11px;
  padding: 18px;
  min-width: 0;
  box-shadow: 0 2px 8px #1e2a5008;
}
.form {
  max-width: 720px;
  display: grid;
  gap: 15px;
}
.section-title {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
}
.section-title h2 {
  font-size: 15px;
  margin: 0 0 4px;
}
.section-title p,
.hint {
  color: #737b8c;
  font-size: 11px;
  line-height: 1.6;
  margin: 0;
}
.toggle-row {
  display: flex;
  justify-content: space-between;
  gap: 14px;
  align-items: center;
  border: 1px solid #eceef2;
  border-radius: 8px;
  padding: 12px;
}
.toggle-row span {
  display: grid;
  gap: 4px;
}
.toggle-row b {
  font-size: 12px;
}
.toggle-row small,
.form label small {
  font-size: 10px;
  font-weight: 400;
  color: #828a99;
  line-height: 1.55;
}
.toggle-row input {
  width: 18px;
  height: 18px;
  accent-color: #5969df;
  flex: none;
}
.form > label:not(.toggle-row) {
  display: grid;
  gap: 7px;
  font-size: 11px;
  font-weight: 600;
}
.form > label > input:not([type="checkbox"]),
.form textarea {
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
.form textarea {
  resize: vertical;
}
.legal {
  background: #f7f8fb;
  border-radius: 8px;
  padding: 12px;
}
.legal h3 {
  font-size: 12px;
  margin: 0 0 9px;
}
.legal dl {
  margin: 0;
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}
.legal dl div {
  min-width: 0;
}
.legal dt {
  font-size: 10px;
  color: #858c99;
}
.legal dd {
  font-size: 12px;
  font-weight: 600;
  margin: 4px 0 0;
  overflow-wrap: anywhere;
}
.legal p {
  font-size: 10px;
  color: #858c99;
  margin: 10px 0 0;
}
.field-error {
  color: #a83d36;
  font-size: 10px;
}
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 9px;
}
.primary,
.secondary {
  border-radius: 7px;
  padding: 9px 12px;
  font-size: 11px;
  font-weight: 700;
}
.primary {
  border: 0;
  background: #5969df;
  color: white;
}
.secondary {
  border: 1px solid #e0e3ea;
  background: white;
  color: #586174;
}
.primary:disabled,
.secondary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.feedback {
  border-radius: 8px;
  padding: 11px 13px;
  font-size: 12px;
  background: #f5f6fa;
  color: #697184;
  overflow-wrap: anywhere;
}
.feedback button {
  margin-left: 7px;
  background: none;
  border: 0;
  color: inherit;
  text-decoration: underline;
}
.error {
  background: #fff0ef;
  color: #a83d36;
}
.success {
  background: #edf8f2;
  color: #287553;
  display: grid;
  gap: 5px;
}
.audit {
  font-size: 10px;
  color: #858c99;
  margin: 0;
  overflow-wrap: anywhere;
}
@media (max-width: 600px) {
  .auth-settings header {
    flex-direction: column;
  }
  .brand {
    white-space: normal;
  }
  .card {
    padding: 14px;
  }
  .legal dl {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
