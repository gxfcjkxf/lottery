<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
import { vInlineValidation } from '@lottery/shared'
import { computed, onMounted, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { useAdminI18n } from "./i18n";
import type { LocalizedMessage } from "@lottery/shared";
import {
  canView,
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
const { t, message, display } = useAdminI18n();
const keyForBody = createBodyKeyTracker();
const settings = ref<SettingsRecord | null>(null);
const captcha = ref(false);
const telegram = ref(false);
const telegramClientId = ref("");
const reason = ref("");
const loading = ref(false);
const saving = ref(false);
const error = ref<string | LocalizedMessage>("");
const notice = ref<string | LocalizedMessage>("");
const allowed = computed(() =>
  canView(props.account, props.brandId, "auth_config"),
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

function showError(cause: unknown, intent: "read" | "write" = "write") {
  if (cause instanceof AdminApiError && cause.status === 401)
    emit("session-invalid");
  error.value = publicErrorMessage(cause, intent);
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
    showError(cause, "read");
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
    saving.value  ||
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
    notice.value = message('admin.settings.saved');
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
  <section class="auth-settings brand-settings-panel" :aria-label="t('admin.copy.09e25a7b')">
    <p v-if="!allowed" class="feedback">
      {{
        writable
          ? t('admin.copy.6de2922f')
          : t('admin.copy.b459040e')
      }}
    </p>
    <template v-else>
      <div v-global-loading v-if="loading && !settings" class="card hint">
        {{ t('admin.copy.c526b4f1') }}
      </div>
      <form v-write-guard="saving" novalidate v-inline-validation v-else-if="settings" class="form" @submit.prevent="withGlobalLoading(save)">
        <label class="toggle-row"
          ><span
            ><b>{{ t('admin.copy.e180d68d') }}</b></span
          ><input v-model="captcha" type="checkbox" :disabled="!writable"
        /></label>
        <label class="toggle-row"
          ><span
            ><b>{{ t('admin.copy.95b3bf4e') }}</b
            ></span
          ><input v-model="telegram" type="checkbox" :disabled="!writable"
        /></label>
        <label
          >{{ t('admin.copy.f37a3885') }}<input
            v-model.trim="telegramClientId"
            inputmode="numeric"
            autocomplete="off"
            :aria-invalid="!telegramIdValid"
            :readonly="!writable"
          /><small
            >{{ t('admin.copy.14f17663') }}</small
          ><span v-if="!telegramIdValid" class="field-error"
            >{{ t('admin.copy.e88c6c01') }}</span
          ></label
        >
        <div class="legal">
          <h3>{{ t('admin.copy.202fbc17') }}</h3>
          <dl>
            <div>
              <dt>{{ t('admin.copy.b7289037') }}</dt>
              <dd>{{ settings.privacy_policy_version || t('admin.copy.04aa7290') }}</dd>
            </div>
            <div>
              <dt>{{ t('admin.copy.3dfdbe0b') }}</dt>
              <dd>{{ settings.service_terms_version || t('admin.copy.04aa7290') }}</dd>
            </div>
          </dl>
        </div>

        <div class="actions">
          <button
            v-if="writable"
            class="button button-primary"
            :disabled="saving  || !telegramIdValid"
          >
            {{ saving ? t('admin.copy.2462134b') : t('admin.copy.1296b8e5') }}
          </button>
        </div>
      </form>
      <p v-if="error" class="feedback error" role="alert">
        {{ display(error) }} <button class="button button-secondary" type="button" :disabled="loading||saving" @click="withGlobalLoading(reload)">{{ t('admin.settings.readAgain') }}</button>
      </p>
      <p v-if="notice" class="feedback success" role="status">
        {{ display(notice)
        }}
      </p>
    </template>
  </section>
</template>

<style scoped>
.form { display:grid;gap:12px;max-width:720px;min-width:0; }
.toggle-row { display:flex;justify-content:space-between;align-items:center;gap:12px;min-height:44px;padding:8px 10px;border:1px solid var(--line);border-radius:var(--radius-sm);cursor:pointer; }
.toggle-row input { width:18px;height:18px;flex:0 0 18px;accent-color:var(--primary); }
.form > label:not(.toggle-row) { display:grid;gap:6px;min-width:0; }
.form input:not([type=checkbox]) { box-sizing:border-box;width:100%;min-width:0; }
.form small { color:var(--muted); }
.legal { padding:12px;border-radius:var(--radius-sm);background:var(--canvas); }
.legal h3 { font-size:14px;margin:0 0 8px; }
.legal dl { margin:0; }
.actions { display:flex;gap:12px;flex-wrap:wrap; }
.feedback { margin:0;overflow-wrap:anywhere; }
.feedback.error,.field-error { color:var(--danger-text); }
.feedback.success { color:var(--primary-dark); }
.feedback.error { display:flex;align-items:center;flex-wrap:wrap;gap:12px; }
</style>
