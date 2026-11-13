<script setup lang="ts">
import { useOperationToast } from "@lottery/shared";
import { formValidationText } from '@lottery/shared'
import AccountCreateFields from "@lottery/shared/AccountCreateFields.vue"
import MemberDetailsFields from "@lottery/shared/MemberDetailsFields.vue"
import WriteFormActions from "@lottery/shared/WriteFormActions.vue"
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
import { withGlobalLoading } from '@lottery/shared';
import { vInlineValidation } from '@lottery/shared'
import { computed, ref } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { useAdminI18n } from "./i18n";
import type { LocalizedMessage } from "@lottery/shared";
import {
  createBodyKeyTracker,
  createManagementApi,
  effectivePermissions,
  type CreatedMember,
  type ManagementAccount,
} from "./management-api";

const props = defineProps<{
  account: AdminAccount & Partial<ManagementAccount>;
  brandId: string;
}>();
const emit = defineEmits<{
  (event: "session-invalid"): void;
  (event: "created", saved: CreatedMember): void;
}>();
const api = createManagementApi();
const { t, display } = useAdminI18n();
const keyForBody = createBodyKeyTracker();
const username = ref("");
const phone = ref("");
const password = ref("");
const notes = ref("");
const reason = ref("");
const busy = ref(false);
const error = ref<string | LocalizedMessage>("");
const saved = ref<CreatedMember | null>(null);
const passwordCharacters = computed(
  () => Array.from(password.value).length,
);
const allowed = computed(
  () =>
    !props.account.super_admin &&
    effectivePermissions(props.account, props.brandId).has("user.create.brand"),
);

async function submit() {
  if (
    busy.value ||
    !allowed.value ||
    passwordCharacters.value < 6 ||
    passwordCharacters.value > 30
  )
    return;
  const body: {
    username?: string;
    phone?: string;
    password: string;
    notes?: string;
    reason: string;
  } = { password: password.value, reason: reason.value.trim() };
  if (username.value.trim()) body.username = username.value.trim();
  if (phone.value.trim()) body.phone = phone.value.trim();
  if (notes.value.trim()) body.notes = notes.value.trim();
  busy.value = true;
  error.value = "";
  saved.value = null;
  try {
    const result = await api.createMember(
      props.brandId,
      body,
      keyForBody(body),
    );
    keyForBody.clear();
    saved.value = result;
    emit("created", result);
    // Do not retain a credential after the server accepted the one-time create operation.
    password.value = "";
    username.value = "";
    phone.value = "";
    notes.value = "";
    reason.value = "";
  } catch (cause) {
    if (cause instanceof AdminApiError && cause.status === 401)
      emit("session-invalid");
    error.value = publicErrorMessage(cause);
  } finally {
    busy.value = false;
  }
}

useOperationToast(() => error.value, value => display(value), 'error');
useOperationToast(() => saved.value, () => t('admin.copy.bdfd5d02'), 'success');
</script>

<template>
  <section class="provision">
    <p v-if="!allowed" class="notice">{{ t('admin.copy.57512fb0') }}</p>
    <template v-else>
      <div class="layout">
        <form v-write-guard="busy" novalidate v-inline-validation class="form am-form ui-form" @submit.prevent="withGlobalLoading(submit)">

          <AccountCreateFields v-model:account="username" v-model:password="password" :account-label="t('admin.copy.0db32fa9')" :password-label="t('admin.copy.87c404b9')" :account-required="false" :password-characters="true" />
          <p class="hint">{{formValidationText('identityRequired')}}</p>
          <MemberDetailsFields v-model:phone="phone" :phone-label="t('admin.copy.009f2b26')" :phone-maxlength="32" />
          <label>{{ t('admin.copy.0fafad85') }}<textarea v-model.trim="notes" rows="3" maxlength="1000" /></label>

          <WriteFormActions
            :submit-label="busy ? t('admin.copy.764449bc') : t('admin.copy.67b6db56')"
            :busy="busy"
            :invalid="passwordCharacters < 6 || passwordCharacters > 30"
          />
        </form>

      </div>

    </template>
  </section>
</template>

<style scoped>
.provision {
  width: 100%;
  min-width: 0;
  color: var(--ink);
}
.provision header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 14px;
  margin: 4px 0 18px;
}
.provision h1 {
  font-size: 24px;
  margin: 3px 0 5px;
  letter-spacing: -0.5px;
}
.provision header p {
  color: var(--muted);
  margin: 0;
}
.eyebrow {
  font-size: var(--body-font-size);
  letter-spacing: 1.2px;
  color: var(--muted);
  font-weight: 700;
}
.brand {
  border-radius: 20px;
  background: var(--canvas);
  padding: 7px 11px;
  color: var(--ink-secondary);
  font-size: var(--body-font-size);
  overflow-wrap: anywhere;
}
.layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}
.card {
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: 11px;
  padding: 18px;
  min-width: 0;
  box-shadow: 0 2px 8px color-mix(in srgb, var(--ink) 3%, transparent);
}
.form {
  display: grid;
  gap: 13px;
}
.card h2 {
  font-size: 15px;
  margin: 0;
}
.hint {
  font-size: var(--body-font-size);
  color: var(--muted);
  line-height: 1.6;
  margin: 0;
}
.required {
  color: var(--danger-text);
}
.notice,
.feedback {
  border-radius: 8px;
  padding: 11px 13px;
  font-size: var(--body-font-size);
  overflow-wrap: anywhere;
}
.notice {
  background: var(--canvas);
  color: var(--muted);
}
.feedback {
  margin-top: 13px;
  display: grid;
  gap: 5px;
}
.error {
  background: var(--danger-bg);
  color: var(--danger-text);
}
.success {
  background: var(--success-bg);
  color: var(--success-text);
}
@media (max-width: 720px) {
  .layout {
    grid-template-columns: minmax(0, 1fr);
  }
  .provision header {
    flex-direction: column;
  }
  .brand {
    white-space: normal;
  }
  .card {
    padding: 14px;
  }
}
</style>
