<script setup lang="ts">
import AccountCreateFields from "@lottery/shared/AccountCreateFields.vue"
import MemberDetailsFields from "@lottery/shared/MemberDetailsFields.vue"
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
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
const { t, message, display } = useAdminI18n();
const keyForBody = createBodyKeyTracker();
const username = ref("");
const phone = ref("");
const password = ref("");
const displayName = ref("");
const notes = ref("");
const reason = ref("");
const busy = ref(false);
const error = ref<string | LocalizedMessage>("");
const saved = ref<CreatedMember | null>(null);
const passwordBytes = computed(
  () => new TextEncoder().encode(password.value).length,
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
    passwordBytes.value < 6 ||
    passwordBytes.value > 30
  )
    return;
  const body: {
    username?: string;
    phone?: string;
    password: string;
    display_name?: string;
    notes?: string;
    reason: string;
  } = { password: password.value, reason: reason.value.trim() };
  if (username.value.trim()) body.username = username.value.trim();
  if (phone.value.trim()) body.phone = phone.value.trim();
  if (displayName.value.trim()) body.display_name = displayName.value.trim();
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
    displayName.value = "";
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
</script>

<template>
  <section class="provision">
    <p v-if="!allowed" class="notice">{{ t('admin.copy.57512fb0') }}</p>
    <template v-else>
      <div class="layout">
        <form v-write-guard="busy" novalidate v-inline-validation class="form am-form" @submit.prevent="withGlobalLoading(submit)">

          <AccountCreateFields v-model:account="username" v-model:password="password" :account-label="t('admin.copy.0db32fa9')" :password-label="t('admin.copy.87c404b9')" :account-required="false"><template #password-help>{{t('admin.copy.0b3f5d69')}} {{passwordBytes}} {{t('admin.copy.4c60451d')}}</template></AccountCreateFields>
          <MemberDetailsFields v-model:name="displayName" v-model:phone="phone" :name-label="t('admin.copy.b60e278d')" :phone-label="t('admin.copy.009f2b26')" :phone-maxlength="32" />
          <label
            >{{ t('admin.copy.0fafad85') }} <span>{{ t('admin.copy.fca11f17') }}</span
            ><textarea v-model.trim="notes" rows="3" maxlength="1000" />
          </label>

          <button
            class="button button-primary"
            :disabled="
              busy  ||
              passwordBytes < 6 ||
              passwordBytes > 30
            "
          >
            {{ busy ? t('admin.copy.764449bc') : t('admin.copy.67b6db56') }}
          </button>
        </form>
        <aside class="card facts">
          <h2>{{ t('admin.copy.12a87052') }}</h2>
          <ul>
            <li>{{ t('admin.copy.a43388ff') }}</li>
            <li>{{ t('admin.copy.ffc82c97') }}</li>
            <li>{{ t('admin.copy.0de103a1') }}</li>
            <li>{{ t('admin.copy.5f82b2d1') }}</li>
          </ul>
          <div class="consent">
            <b>{{ t('admin.copy.42dda18e') }}</b>
            <p>
              {{ t('admin.copy.73e5391d') }}
            </p>
          </div>
          <p class="hint">
            {{ t('admin.copy.13f56708') }}
          </p>
        </aside>
      </div>
      <p v-if="error" class="feedback error" role="alert">{{ display(error) }}</p>
      <div v-if="saved" class="feedback success" role="status">
        <b>{{ t('admin.copy.bdfd5d02') }}</b
        ><span
          >{{ t('admin.copy.92c59de3') }}: <AssociationName :record="saved" kind="member" :id="saved.member_id" :locale="associationLocale" /></span
        ><span>{{ t('admin.copy.9c32661d') }}: <AssociationName :record="saved" kind="brand" :id="saved.brand_id" :locale="associationLocale" /> · {{ t('admin.copy.48d8db38') }}: {{ t('admin.copy.b92bb6e0') }}</span
        ><span>{{ t('admin.copy.55378394') }}: </span>
      </div>
    </template>
  </section>
</template>

<style scoped>
.provision {
  width: 100%;
  min-width: 0;
  color: #252a36;
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
  font-size: 11px;
  letter-spacing: 1.2px;
  color: #8991a2;
  font-weight: 700;
}
.brand {
  border-radius: 20px;
  background: #f1f3f8;
  padding: 7px 11px;
  color: #626b7c;
  font-size: 11px;
  overflow-wrap: anywhere;
}
.layout {
  display: grid;
  grid-template-columns: minmax(0, 1.15fr) minmax(250px, 0.85fr);
  gap: 16px;
  align-items: start;
}
.card {
  background: white;
  border: 1px solid var(--line);
  border-radius: 11px;
  padding: 18px;
  min-width: 0;
  box-shadow: 0 2px 8px #1e2a5008;
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
  font-size: 11px;
  color: var(--muted);
  line-height: 1.6;
  margin: 0;
}
.required {
  color: #a45540;
}
.button-primary {
  border: 0;
  border-radius: 7px;
  background: var(--primary);
  color: white;
  font-weight: 700;
  padding: 10px 12px;
}
.button-primary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.facts ul {
  padding-left: 19px;
  color: #596172;
  font-size: 12px;
  line-height: 1.8;
}
.consent {
  border-left: 3px solid var(--primary);
  background: #f6f7fc;
  border-radius: 0 7px 7px 0;
  padding: 11px;
  margin: 15px 0;
}
.consent b {
  font-size: 12px;
}
.consent p {
  font-size: 11px;
  color: #626b7c;
  line-height: 1.6;
  margin: 5px 0 0;
}
.notice,
.feedback {
  border-radius: 8px;
  padding: 11px 13px;
  font-size: 12px;
  overflow-wrap: anywhere;
}
.notice {
  background: #f5f6fa;
  color: #697184;
}
.feedback {
  margin-top: 13px;
  display: grid;
  gap: 5px;
}
.error {
  background: #fff0ef;
  color: #a83d36;
}
.success {
  background: #edf8f2;
  color: #287553;
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
