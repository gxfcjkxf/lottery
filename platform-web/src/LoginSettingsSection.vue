<script setup lang="ts">
import { useOperationToast } from "@lottery/shared";
import { formValidationText } from '@lottery/shared'
import { vWriteGuard } from "@lottery/shared";
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { withGlobalLoading, vGlobalLoading, vInlineValidation } from '@lottery/shared';
import DetailModal from '@lottery/shared/DetailModal.vue';
import { usePlatformI18n } from './i18n';
import { PlatformApiError, type PlatformAccount } from './platform-api';
import { createSecuritySettingsApi, securityIntents, type AuthSettings, type SecurityKind, type SecurityIntent } from './security-settings-api';
const props = defineProps<{ account: PlatformAccount; kind: SecurityKind; blocked: boolean }>();
const emit = defineEmits<{ failure: [cause: unknown]; locked: [value: boolean] }>();
const { t, errorMessage, displayError } = usePlatformI18n();
const api = createSecuritySettingsApi();
const selected = computed(() => props.kind);
const auth = ref<AuthSettings | null>(null);
const editorOpen = ref(false), draftAuth = ref<AuthSettings | null>(null);
const loading = ref(false), busy = ref(false), readFailed = ref(false), confirmOpen = ref(false), freshConflict = ref(false);
const pending = ref<SecurityIntent | null>(null), error = ref<ReturnType<typeof errorMessage> | null>(null), saved = ref(false);
let generation = 0;
const scope = () => `${props.account.id}::${selected.value}`;
const resource = computed(() => 'auth_config');
const canView = computed(() => props.account.platform_permissions.includes(`${resource.value}.view.platform`));
const canWrite = computed(() => props.account.platform_permissions.includes(`${resource.value}.write.platform`));
const editable = computed(() => canWrite.value && !props.blocked && !loading.value && !busy.value && !pending.value && !readFailed.value);
function startEdit() {
  if (!canView.value || !editable.value) return;
  draftAuth.value = auth.value ? { ...auth.value } : null;
  error.value = null; saved.value = false; editorOpen.value = true;
}
function report(cause: unknown, read = false) {
 error.value = errorMessage(cause, read ? 'read' : 'write');
 if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause); }
async function load() {
  const token = ++generation, kind = selected.value;
  loading.value = true; error.value = null;
  try {
    const value = await api.read(kind, '');
    if (token !== generation) return;
    auth.value = value as AuthSettings;
    readFailed.value = false; freshConflict.value = pending.value?.phase === 'conflict';
  } catch (cause) { if (token === generation) { readFailed.value = true; report(cause, true); } }
  finally { if (token === generation) loading.value = false; }
}
function prepare() {
  if (!editable.value) return;
  const body = selected.value === 'auth' && draftAuth.value ? { version: draftAuth.value.version, inherit_platform: false, captcha_enabled: draftAuth.value.captcha_enabled, telegram_enabled: draftAuth.value.telegram_enabled, telegram_client_id: draftAuth.value.telegram_client_id.trim(), reason: '' } : null;
  if (!body) return;
  pending.value = { kind: selected.value, body: JSON.stringify(body), key: crypto.randomUUID(), phase: 'review' };
  securityIntents.set(scope(), pending.value); saved.value = false; confirmOpen.value = true;
}
function close() { if (busy.value) return; if (pending.value?.phase === 'review') { securityIntents.clear(scope()); pending.value = null; } confirmOpen.value = false; }
async function discardConflict() { if (!freshConflict.value || busy.value) return; securityIntents.clear(scope()); pending.value = null; confirmOpen.value = false; editorOpen.value = false; await load(); }
async function submit() {
  const intent = pending.value;
  if (!intent || intent.phase === 'conflict' || busy.value || !canWrite.value) return;
  const own = scope(), token = generation, epoch = securityIntents.epoch();
  busy.value = true; error.value = null;
  pending.value = { ...intent, phase: 'unknown' }; securityIntents.set(own, pending.value);
  try {
    await api.write('', intent); if(epoch !== securityIntents.epoch()) return; securityIntents.clear(own);
    if (token !== generation) return;
    pending.value = null; confirmOpen.value = false; editorOpen.value = false; saved.value = true; await load();
  } catch (cause) {
    if(epoch !== securityIntents.epoch()) return;
    const status = cause instanceof PlatformApiError ? cause.status : 0;
    const phase = status === 409 ? 'conflict' : status === 0 || status >= 500 ? 'unknown' : null;
    if (phase) securityIntents.set(own, { ...intent, phase }); else securityIntents.clear(own);
    if (token !== generation) return;
    pending.value = securityIntents.get(own); if (!pending.value) confirmOpen.value = false;
    report(cause); if (phase === 'conflict') { freshConflict.value = false; await load(); }
  } finally { if (scope() === own) busy.value = false; }
}
watch(() => [props.account.id], () => {
  ++generation; editorOpen.value = false; draftAuth.value = null; auth.value = null; busy.value = false; saved.value = false; readFailed.value = false;
  pending.value = securityIntents.get(scope());
  confirmOpen.value = Boolean(pending.value);
  if (canView.value) void withGlobalLoading(load);
}, { immediate: true });
watch(() => Boolean(pending.value) || busy.value || editorOpen.value, value => emit('locked', value), { immediate: true });
onBeforeUnmount(() => { ++generation; });
useOperationToast(saved, () => t('security.saved'), 'success');

useOperationToast(error, value => displayError(value), 'error');
</script>

<template>
  <section class="security-settings-section brand-settings-panel" :data-kind="kind">
    <p v-if="!canView" role="status">{{ t('security.noPermission') }}</p>
    <p v-global-loading v-else-if="loading && !(auth)">{{ t('platform.copy.fa02e7b1') }}</p>
    <template v-else-if="canView && (auth)">
      <dl v-if="selected === 'auth' && auth" class="confirm-list">
        <div><dt>{{ t('security.captcha') }}</dt><dd>{{ t(auth.captcha_enabled ? 'platform.copy.997d5f45' : 'platform.copy.6762439b') }}</dd></div>
        <div><dt>{{ t('security.telegram') }}</dt><dd>{{ t(auth.telegram_enabled ? 'platform.copy.997d5f45' : 'platform.copy.6762439b') }}</dd></div>
        <div v-if="auth.telegram_enabled"><dt>{{ t('security.clientId') }}</dt><dd>{{ auth.telegram_client_id || '—' }}</dd></div>
      </dl>
      <div class="ui-form-actions"><button class="button button-secondary" type="button" :disabled="!editable" @click="startEdit">{{ t('common.editSettings') }}</button></div>
    </template>
    <DetailModal class="brand-settings-dialog" v-if="editorOpen && canView" :open="!confirmOpen && !pending" :title="t('settings.tab.auth')" :close-label="t('platform.copy.27b2fd29')" :close-disabled="busy" @close="!busy && (editorOpen = false)">

    <form class="ui-form" v-if="selected === 'auth' && draftAuth" novalidate v-inline-validation @submit.prevent="withGlobalLoading(prepare)">
      <fieldset :disabled="!editable">
      <label class="check"><input v-model="draftAuth.captcha_enabled" type="checkbox" :disabled="!editable">{{ t('security.captcha') }}</label>
      <label class="check"><input v-model="draftAuth.telegram_enabled" type="checkbox" :disabled="!editable">{{ t('security.telegram') }}</label>
      <label class="field">{{ t('security.clientId') }}<input v-model="draftAuth.telegram_client_id" inputmode="numeric" pattern="[1-9][0-9]*" :required="draftAuth.telegram_enabled" :data-validation-format="formValidationText('positiveInteger')" :disabled="!editable"></label>
      </fieldset>
      <div class="form-actions ui-form-actions"><button class="button button-primary" :disabled="!editable" type="submit">{{ t('security.save') }}</button></div>
    </form>
    </DetailModal>
    <button v-if="pending" class="button button-secondary" type="button" @click="confirmOpen = true">{{ t('security.pending') }}</button>
    <button v-if="readFailed" class="button button-secondary" type="button" :disabled="busy || loading" @click="withGlobalLoading(load)">{{ t('security.reread') }}</button>

    <DetailModal class="brand-settings-dialog" v-if="pending && confirmOpen" :title="t('security.confirm')" :close-label="t('platform.copy.27b2fd29')" :close-disabled="busy" @close="close">
      <p>{{ t('security.confirmAuth') }}</p>
      <p v-if="pending.phase === 'unknown'" role="alert">{{ t('security.unknown') }}</p><p v-if="pending.phase === 'conflict'" role="alert">{{ t('security.conflict') }}</p>
      <template #footer><button v-write-guard="busy" v-if="pending.phase !== 'conflict'" class="button button-primary" type="button" :disabled="busy || !canWrite" @click="withGlobalLoading(submit)">{{ t(pending.phase === 'unknown' ? 'security.retry' : 'security.confirm') }}</button><button v-if="pending.phase === 'conflict' && !freshConflict" class="button button-secondary" type="button" :disabled="busy || loading" @click="withGlobalLoading(load)">{{ t('security.reread') }}</button><button v-if="pending.phase === 'conflict' && freshConflict" class="button button-secondary" type="button" :disabled="busy" @click="withGlobalLoading(discardConflict)">{{ t('security.startAgain') }}</button></template>
    </DetailModal>
  </section>
</template>
<style scoped>
.confirm-list { margin-bottom: 0; }
.security-settings-section form { display:grid; gap:12px; min-width:0; }
.security-settings-section fieldset { display:grid; gap:12px; margin:0; padding:0; border:0; min-width:0; }
.check { min-width:0; overflow-wrap:anywhere; }
.check { display:flex; align-items:center; gap:8px; min-height:var(--control-height); }
.check input { width:18px; height:18px; min-height:18px; margin:0; accent-color:var(--primary); }
.field { align-self:start; align-content:start; height:auto; padding:0; border:0; border-radius:0; background:transparent; display:grid; gap:6px; min-width:0; max-width:480px; }
.field input { width:100%; min-width:0; box-sizing:border-box; }
.muted, small { color:var(--muted); }
</style>
