<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { withGlobalLoading, vGlobalLoading, vInlineValidation } from '@lottery/shared';
import DetailModal from '@lottery/shared/DetailModal.vue';
import { usePlatformI18n } from './i18n';
import { PlatformApiError, type PlatformAccount } from './platform-api';
import { createSecuritySettingsApi, securityIntents, type AuthSettings, type CompliancePolicy, type SecurityKind, type SecurityIntent } from './security-settings-api';
const props = defineProps<{ brandId: string; account: PlatformAccount; status: string }>();
const emit = defineEmits<{ failure: [cause: unknown] }>();
const { t, errorMessage, displayError } = usePlatformI18n();
const api = createSecuritySettingsApi();
const selected = ref<SecurityKind>('auth'), auth = ref<AuthSettings | null>(null), policy = ref<CompliancePolicy | null>(null);
const editorOpen = ref(false), draftAuth = ref<AuthSettings | null>(null), draftPolicy = ref<CompliancePolicy | null>(null);
const countries = ref(''), loading = ref(false), busy = ref(false), readFailed = ref(false), confirmOpen = ref(false), freshConflict = ref(false);
const pending = ref<SecurityIntent | null>(null), error = ref<ReturnType<typeof errorMessage> | null>(null), saved = ref(false);
let generation = 0;
const scope = () => `${props.account.id}:${props.brandId}`;
const resource = computed(() => selected.value === 'auth' ? 'auth_config' : 'compliance_policy');
const canView = computed(() => props.account.platform_permissions.includes(`${resource.value}.view.platform`));
const canWrite = computed(() => props.account.platform_permissions.includes(`${resource.value}.write.platform`));
const editable = computed(() => canWrite.value && props.status !== 'disabled' && !loading.value && !busy.value && !pending.value && !readFailed.value);
function startEdit() {
  if (!canView.value || !editable.value) return;
  draftAuth.value = auth.value ? { ...auth.value } : null;
  draftPolicy.value = policy.value ? { ...policy.value, config: { ...policy.value.config, allowed_countries: [...policy.value.config.allowed_countries] } } : null;
  countries.value = policy.value?.config.allowed_countries.join(', ') ?? '';
  error.value = null; saved.value = false; editorOpen.value = true;
}
function report(cause: unknown, read = false) {
 error.value = errorMessage(cause, read ? 'read' : 'write');
 if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause); }
async function load() {
  const token = ++generation, brand = props.brandId, kind = selected.value;
  loading.value = true; error.value = null;
  try {
    const value = await api.read(kind, brand);
    if (token !== generation) return;
    if (kind === 'auth') auth.value = value as AuthSettings;
    else { policy.value = value as CompliancePolicy; countries.value = policy.value.config.allowed_countries.join(', '); }
    readFailed.value = false; freshConflict.value = pending.value?.phase === 'conflict';
  } catch (cause) { if (token === generation) { readFailed.value = true; report(cause, true); } }
  finally { if (token === generation) loading.value = false; }
}
function prepare() {
  if (!editable.value) return;
  const body = selected.value === 'auth' && draftAuth.value ? { version: draftAuth.value.version, captcha_enabled: draftAuth.value.captcha_enabled, telegram_enabled: draftAuth.value.telegram_enabled, telegram_client_id: draftAuth.value.telegram_client_id.trim(), reason: '' }
    : selected.value === 'compliance' && draftPolicy.value ? { version: draftPolicy.value.version, config: { ...draftPolicy.value.config, allowed_countries: [...new Set(countries.value.split(/[\s,]+/).filter(Boolean).map(value => value.toUpperCase()))].sort() }, reason: '' } : null;
  if (!body) return;
  pending.value = { kind: selected.value, body: JSON.stringify(body), key: crypto.randomUUID(), phase: 'review' };
  securityIntents.set(scope(), pending.value); saved.value = false; confirmOpen.value = true;
}
function close() { if (busy.value) return; if (pending.value?.phase === 'review') { securityIntents.clear(scope()); pending.value = null; } confirmOpen.value = false; }
async function discardConflict() { if (!freshConflict.value || busy.value) return; securityIntents.clear(scope()); pending.value = null; confirmOpen.value = false; editorOpen.value = false; await load(); }
async function submit() {
  const intent = pending.value;
  if (!intent || intent.phase === 'conflict' || busy.value || !canWrite.value || props.status === 'disabled') return;
  const own = scope(), brand = props.brandId, token = generation, epoch = securityIntents.epoch();
  busy.value = true; error.value = null;
  pending.value = { ...intent, phase: 'unknown' }; securityIntents.set(own, pending.value);
  try {
    await api.write(brand, intent); if(epoch !== securityIntents.epoch()) return; securityIntents.clear(own);
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
watch(() => [props.account.id, props.brandId], () => {
  ++generation; editorOpen.value = false; draftAuth.value = null; draftPolicy.value = null; auth.value = null; policy.value = null; busy.value = false; saved.value = false; readFailed.value = false;
  pending.value = securityIntents.get(scope()); if (pending.value) selected.value = pending.value.kind;
  confirmOpen.value = Boolean(pending.value);
  if (canView.value && props.brandId) void withGlobalLoading(load);
}, { immediate: true });
watch(selected, () => { editorOpen.value = false; saved.value = false; if (canView.value && props.brandId) void withGlobalLoading(load); });
onBeforeUnmount(() => { ++generation; });
const checks = ['age_enabled', 'region_enabled', 'identity_enabled', 'account_risk_enabled', 'betting_risk_enabled', 'exclusion_enabled', 'responsible_gambling_enabled'] as const;
</script>

<template>
  <section class="security-settings brand-settings-panel">
    <div class="console-tabs"><button type="button" :class="{ selected: selected === 'auth' }" :disabled="!!pending || busy" @click="selected = 'auth'">{{ t('security.auth') }}</button><button type="button" :class="{ selected: selected === 'compliance' }" :disabled="!!pending || busy" @click="selected = 'compliance'">{{ t('security.compliance') }}</button></div>
    <p v-if="!canView" role="status">{{ t('security.noPermission') }}</p>
    <p v-global-loading v-else-if="loading && !(selected === 'auth' ? auth : policy)">{{ t('platform.copy.fa02e7b1') }}</p>
    <template v-else-if="canView && (selected === 'auth' ? auth : policy)">
      <button class="button button-secondary" type="button" :disabled="!editable" @click="startEdit">{{ t('common.editSettings') }}</button>
      <dl v-if="selected === 'auth' && auth" class="confirm-list">
        <div><dt>{{ t('security.captcha') }}</dt><dd>{{ t(auth.captcha_enabled ? 'platform.copy.997d5f45' : 'platform.copy.6762439b') }}</dd></div>
        <div><dt>{{ t('security.telegram') }}</dt><dd>{{ t(auth.telegram_enabled ? 'platform.copy.997d5f45' : 'platform.copy.6762439b') }}</dd></div>
        <div v-if="auth.telegram_enabled"><dt>{{ t('security.clientId') }}</dt><dd>{{ auth.telegram_client_id || '—' }}</dd></div>
      </dl>
      <dl v-else-if="selected === 'compliance' && policy" class="confirm-list">
        <div v-for="check in checks" :key="check"><dt>{{ t(`security.${check}`) }}</dt><dd>{{ t(policy.config[check] ? 'platform.copy.997d5f45' : 'platform.copy.6762439b') }}</dd></div>
        <div v-if="policy.config.age_enabled"><dt>{{ t('security.minimumAge') }}</dt><dd>{{ policy.config.minimum_age }}</dd></div>
        <div v-if="policy.config.region_enabled"><dt>{{ t('security.countries') }}</dt><dd>{{ policy.config.allowed_countries.join(', ') || '—' }}</dd></div>
      </dl>
    </template>
    <DetailModal class="brand-settings-dialog" v-if="editorOpen && canView" :open="!confirmOpen && !pending" :title="t(selected === 'auth' ? 'security.auth' : 'security.compliance')" :close-label="t('platform.copy.27b2fd29')" @close="!busy && (editorOpen = false)">
      <p v-if="error" role="alert" class="message error">{{ displayError(error) }}</p>
    <form v-if="selected === 'auth' && draftAuth" novalidate v-inline-validation @submit.prevent="withGlobalLoading(prepare)">
      <label class="check"><input v-model="draftAuth.captcha_enabled" type="checkbox" :disabled="!editable">{{ t('security.captcha') }}</label>
      <label class="check"><input v-model="draftAuth.telegram_enabled" type="checkbox" :disabled="!editable">{{ t('security.telegram') }}</label>
      <label class="field">{{ t('security.clientId') }}<input v-model="draftAuth.telegram_client_id" inputmode="numeric" :disabled="!editable"></label>
      <div class="form-actions"><button class="button button-secondary" type="button" :disabled="busy" @click="editorOpen = false">{{ t('platform.copy.2230b1a7') }}</button><button class="button button-primary" :disabled="!editable" type="submit">{{ t('security.save') }}</button></div>
    </form>
    <form v-else-if="selected === 'compliance' && draftPolicy" novalidate v-inline-validation @submit.prevent="withGlobalLoading(prepare)">
      <div class="security-grid"><label v-for="check in checks" :key="check" class="check"><input v-model="draftPolicy.config[check]" type="checkbox" @change="check === 'age_enabled' && draftPolicy.config.age_enabled && (draftPolicy.config.minimum_age ??= 18)" :disabled="!editable">{{ t(`security.${check}`) }}</label></div>
      <div class="security-grid"><label v-if="draftPolicy.config.age_enabled" class="field">{{ t('security.minimumAge') }}<input v-model.number="draftPolicy.config.minimum_age" type="number" min="18" max="120" :disabled="!editable"></label>
      <label v-if="draftPolicy.config.region_enabled" class="field">{{ t('security.countries') }}<input v-model="countries" :disabled="!editable"><small>{{ t('security.countryHint') }}</small></label></div>
      <p class="muted">{{ t('security.adapterHint') }}</p>
      <div class="form-actions"><button class="button button-secondary" type="button" :disabled="busy" @click="editorOpen = false">{{ t('platform.copy.2230b1a7') }}</button><button class="button button-primary" :disabled="!editable" type="submit">{{ t('security.save') }}</button></div>
    </form>
    </DetailModal>
    <button v-if="pending" class="button button-secondary" type="button" @click="confirmOpen = true">{{ t('security.pending') }}</button>
    <button v-if="readFailed" class="button button-secondary" type="button" :disabled="busy || loading" @click="withGlobalLoading(load)">{{ t('security.reread') }}</button>
    <p v-if="error && !confirmOpen && !editorOpen" role="alert" class="message error">{{ displayError(error) }}</p><p v-if="saved" role="status">{{ t('security.saved') }}</p>
    <DetailModal class="brand-settings-dialog" v-if="pending && confirmOpen" :title="t('security.confirm')" :close-label="t('platform.copy.27b2fd29')" @close="close">
      <p>{{ t(pending.kind === 'auth' ? 'security.confirmAuth' : 'security.confirmPolicy') }}</p>
      <p v-if="pending.phase === 'unknown'" role="alert">{{ t('security.unknown') }}</p><p v-if="pending.phase === 'conflict'" role="alert">{{ t('security.conflict') }}</p><p v-if="error" role="alert">{{ displayError(error) }}</p>
      <template #footer><button v-write-guard="busy" v-if="pending.phase !== 'conflict'" class="button button-primary" type="button" :disabled="busy || !canWrite" @click="withGlobalLoading(submit)">{{ t(pending.phase === 'unknown' ? 'security.retry' : 'security.confirm') }}</button><button v-if="pending.phase === 'conflict' && !freshConflict" class="button button-secondary" type="button" :disabled="busy || loading" @click="withGlobalLoading(load)">{{ t('security.reread') }}</button><button v-if="pending.phase === 'conflict' && freshConflict" class="button button-secondary" type="button" :disabled="busy" @click="withGlobalLoading(discardConflict)">{{ t('security.startAgain') }}</button></template>
    </DetailModal>
  </section>
</template>
<style scoped>
.security-settings form { display:grid; gap:12px; min-width:0; }
.security-grid { display:grid; grid-template-columns:repeat(auto-fit,minmax(min(100%,240px),1fr)); gap:8px 16px; }
.check { display:flex; align-items:center; gap:8px; min-height:44px; }
.check input { width:18px; height:18px; min-height:18px; margin:0; accent-color:var(--primary); }
.field { align-self:start; align-content:start; height:auto; padding:0; border:0; border-radius:0; background:transparent; display:grid; gap:6px; min-width:0; max-width:480px; }
.field input { width:100%; min-width:0; box-sizing:border-box; }
.muted, small { color:var(--muted); }
</style>
