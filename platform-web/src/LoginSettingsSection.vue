<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { formValidationText, useOperationToast, vGlobalLoading, vInlineValidation, vWriteGuard, withGlobalLoading } from '@lottery/shared';
import { usePlatformI18n } from './i18n';
import { PlatformApiError, type PlatformAccount } from './platform-api';
import { createSecuritySettingsApi, securityIntents, type AuthSettings, type SecurityKind, type SecurityIntent } from './security-settings-api';
const props = defineProps<{ account: PlatformAccount; kind: SecurityKind; blocked: boolean }>();
const emit = defineEmits<{ failure: [cause: unknown]; locked: [value: boolean] }>();
const { t, errorMessage, displayError } = usePlatformI18n();
const api = createSecuritySettingsApi();
const selected = computed(() => props.kind);
const auth = ref<AuthSettings | null>(null);
const loading = ref(false), busy = ref(false), readFailed = ref(false);
const pending = ref<SecurityIntent | null>(null), error = ref<ReturnType<typeof errorMessage> | null>(null), saved = ref(false);
let generation = 0;
const scope = () => `${props.account.id}::${selected.value}`;
const canView = computed(() => props.account.platform_permissions.includes('auth_config.view.platform'));
const canWrite = computed(() => props.account.platform_permissions.includes('auth_config.write.platform'));
const editable = computed(() => canWrite.value && !props.blocked && !loading.value && !busy.value && !pending.value && !readFailed.value);
function report(cause: unknown, read = false) {
 error.value = errorMessage(cause, read ? 'read' : 'write');
 if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause); }
async function load() {
  const token = ++generation, kind = selected.value;
  loading.value = true; error.value = null;
  try {
    const value = await api.read(kind, '');
    if (token !== generation) return;
    auth.value = value;
    readFailed.value = false;
    if (pending.value?.phase === 'conflict') {
      securityIntents.clear(scope());
      pending.value = null;
    }
  } catch (cause) { if (token === generation) { readFailed.value = true; report(cause, true); } }
  finally { if (token === generation) loading.value = false; }
}
async function save() {
  if (pending.value) { await submit(); return; }
  if (!editable.value) return;
  const body = selected.value === 'auth' && auth.value ? { version: auth.value.version, inherit_platform: false, captcha_enabled: auth.value.captcha_enabled, telegram_enabled: auth.value.telegram_enabled, telegram_client_id: auth.value.telegram_client_id.trim(), reason: '' } : null;
  if (!body) return;
  pending.value = { kind: selected.value, body: JSON.stringify(body), key: crypto.randomUUID(), phase: 'review' };
  securityIntents.set(scope(), pending.value); saved.value = false; await submit();
}
async function submit() {
  const intent = pending.value;
  if (!intent || intent.phase === 'conflict' || busy.value || loading.value || !canView.value || !canWrite.value || props.blocked) return;
  const own = scope(), token = generation, epoch = securityIntents.epoch();
  busy.value = true; error.value = null;
  pending.value = { ...intent, phase: 'unknown' }; securityIntents.set(own, pending.value);
  try {
    const receipt = await api.write('', intent); if(epoch !== securityIntents.epoch()) return; securityIntents.clear(own);
    if (token !== generation) return;
    pending.value = null; auth.value = receipt; saved.value = true; await load();
  } catch (cause) {
    if(epoch !== securityIntents.epoch()) return;
    const status = cause instanceof PlatformApiError ? cause.status : 0;
    const phase = status === 409 ? 'conflict' : status === 0 || status >= 500 ? 'unknown' : null;
    if (phase) securityIntents.set(own, { ...intent, phase }); else securityIntents.clear(own);
    if (token !== generation) return;
    pending.value = securityIntents.get(own);
    report(cause);
  } finally { if (scope() === own) busy.value = false; }
}
watch(() => [props.account.id, props.kind, canView.value, canWrite.value], () => {
  ++generation; loading.value = false; error.value = null; auth.value = null; busy.value = false; saved.value = false; readFailed.value = false;
  pending.value = securityIntents.get(scope());
  if (!canView.value) return;
  if (pending.value) auth.value = JSON.parse(pending.value.body) as AuthSettings;
  else void withGlobalLoading(load);
}, { immediate: true, flush: 'sync' });
watch(() => Boolean(pending.value) || busy.value, value => emit('locked', value), { immediate: true });
onBeforeUnmount(() => { ++generation; });
useOperationToast(saved, () => t('security.saved'), 'success');

useOperationToast(error, value => displayError(value), 'error');
</script>

<template>
  <section class="security-settings-section brand-settings-panel" :data-kind="kind">
    <p v-if="!canView" role="status">{{ t('security.noPermission') }}</p>
    <p v-global-loading v-else-if="loading && !auth">{{ t('platform.copy.fa02e7b1') }}</p>
    <form v-else-if="auth" v-write-guard="busy" class="brand-form ui-form" novalidate v-inline-validation @submit.prevent="withGlobalLoading(save)">
      <fieldset :disabled="!editable">
        <label class="check"><input v-model="auth.captcha_enabled" type="checkbox">{{ t('security.captcha') }}</label>
        <label class="check"><input v-model="auth.telegram_enabled" type="checkbox">{{ t('security.telegram') }}</label>
        <label class="field">{{ t('security.clientId') }}<input v-model="auth.telegram_client_id" inputmode="numeric" pattern="[1-9][0-9]*" :required="auth.telegram_enabled" :data-validation-format="formValidationText('positiveInteger')"></label>
      </fieldset>
      <p v-if="pending?.phase === 'unknown' && !busy" role="alert">{{ t('security.unknown') }}</p>
      <p v-if="pending?.phase === 'conflict'" role="alert">{{ t('security.conflict') }}</p>
      <div v-if="canWrite || readFailed" class="ui-form-actions">
        <button v-if="readFailed || pending?.phase === 'conflict'" class="button button-secondary" type="button" :disabled="busy || loading" @click="withGlobalLoading(load)">{{ t('security.reread') }}</button>
        <button v-else class="button button-primary" :disabled="busy || loading || blocked" type="submit">{{ t(busy ? 'platform.copy.fa02e7b1' : pending ? 'security.retry' : 'security.save') }}</button>
      </div>
    </form>
    <button v-else-if="readFailed" class="button button-secondary" type="button" @click="withGlobalLoading(load)">{{ t('security.reread') }}</button>
  </section>
</template>
<style scoped>
.security-settings-section .check { flex-direction:row; align-items:center; gap:8px; min-width:0; min-height:var(--control-height); overflow-wrap:anywhere; }
.check input { width:18px; height:18px; min-height:18px; margin:0; accent-color:var(--primary); }
.security-settings-section .field { min-width:0; max-width:480px; }
.field input { width:100%; min-width:0; box-sizing:border-box; }
</style>
