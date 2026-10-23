<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import { computed, ref } from 'vue';
import { withGlobalLoading, buildBrandCssTokens } from '@lottery/shared';
import { AdminApiError, readAdminEnvelope } from './admin-api';
import type { BrandPresentationConfig, BrandPresentationEffective, BrandPresentationLocale } from './brand-presentation-api';
import { useAdminI18n } from './i18n';

const props = defineProps<{ config: BrandPresentationConfig; defaults: BrandPresentationEffective; brandId: string; disabled: boolean }>();
const emit = defineEmits<{ 'update:config': [value: BrandPresentationConfig]; 'session-invalid': []; 'uploading': [value: boolean] }>();
const { t, locale, message, errorMessage, displayError } = useAdminI18n();
const uploading = ref(false), uploadError = ref<ReturnType<typeof errorMessage> | null>(null);
const advanced = ref(false), resetConfirm = ref(false);
const preview = computed<BrandPresentationEffective>(() => ({
  ...Object.fromEntries(Object.entries(props.defaults).map(([key, value]) => [key, props.config[key as keyof BrandPresentationConfig] ?? value])),
  logo_text: props.config.logo_text ?? Array.from(props.config.display_name ?? props.defaults.display_name)[0],
  content: Object.fromEntries((['en', 'zh-CN'] as const).map(language => [language, {
    tagline: props.config.content?.[language].tagline ?? props.defaults.content[language].tagline,
    announcement: props.config.content?.[language].announcement ?? props.defaults.content[language].announcement,
  }])),
}) as BrandPresentationEffective);
const previewTokens = computed(() => buildBrandCssTokens({
 primary:preview.value.primary_color, accent:preview.value.accent_color, success:preview.value.success_color,
 warning:preview.value.warning_color, danger:preview.value.danger_color, fontFamily:preview.value.font_family,
 fontScale:preview.value.font_scale, radius:preview.value.radius, shadow:preview.value.shadow,
}));
const locked = computed(() => props.disabled || uploading.value);
const text = (event: Event) => (event.target as HTMLInputElement).value;
function update(key: keyof BrandPresentationConfig, value: unknown) {
  emit('update:config', { ...props.config, [key]: value });
}
function localized(language: BrandPresentationLocale, field: 'tagline' | 'announcement', value: string) {
  const content = props.config.content ?? { en: { tagline: null, announcement: null }, 'zh-CN': { tagline: null, announcement: null } };
  update('content', { ...content, [language]: { ...content[language], [field]: value } });
}
function languageEnabled(language: BrandPresentationLocale, event: Event) {
  const values = preview.value.available_locales.filter(value => value !== language);
  if ((event.target as HTMLInputElement).checked) values.push(language);
  update('available_locales', values);
}
function defaultLanguage(event: Event) {
  const language = text(event) as BrandPresentationLocale;
  emit('update:config', { ...props.config, default_locale: language, available_locales: [...new Set([...preview.value.available_locales, language])] });
}
const themes = [
  { name: 'forest', primary_color: '#20594c', accent_color: '#d9ef9b' },
  { name: 'ocean', primary_color: '#274c75', accent_color: '#c9e4f5' },
  { name: 'plum', primary_color: '#694477', accent_color: '#e8d5ef' },
] as const;
function chooseTheme(theme: typeof themes[number]) { emit('update:config', { ...props.config, primary_color: theme.primary_color, accent_color: theme.accent_color }); }
function reset() {
  emit('update:config', Object.fromEntries(Object.keys(props.config).map(key => [key, null])) as unknown as BrandPresentationConfig);
  resetConfirm.value = false;
  uploadError.value = null;
}
async function upload(event: Event) {
  const input = event.target as HTMLInputElement, file = input.files?.[0];
  if (!file || locked.value) return;
  uploadError.value = null;
  if (!['image/png', 'image/jpeg'].includes(file.type) || file.size > 1024 * 1024) {
    uploadError.value = message('presentation.uploadInvalid');
    input.value = ''; return;
  }
  const brand = props.brandId;
  uploading.value = true; emit('uploading', true);
  try {
    const response = await fetch('/api/v1/admin/brand-presentation/logo', { method: 'POST', credentials: 'same-origin', headers: { 'X-Brand-ID': brand, 'Content-Type': file.type }, body: file });
    const { data } = await readAdminEnvelope(response);
    if (!data || typeof data !== 'object' || !('url' in data) || typeof data.url !== 'string' || !/^\/api\/v1\/brand-assets\/[a-f0-9]{64}\.(png|jpg)$/.test(data.url)) throw new AdminApiError('Invalid upload receipt', 502, 'INVALID_RESPONSE');
    if (brand === props.brandId) update('logo_url', data.url);
  } catch (cause) {
    if (brand === props.brandId) {
      uploadError.value = errorMessage(cause);
      if (cause instanceof AdminApiError && cause.status === 401) emit('session-invalid');
    }
  } finally { uploading.value = false; emit('uploading', false); input.value = ''; }
}
</script>

<template>
  <div class="presentation-fields" :aria-disabled="locked">
    <div class="presentation-basic">
      <label class="field brand-name-field"><span>{{ t('presentation.name') }}</span><input type="text" :value="preview.display_name" :disabled="locked" @input="update('display_name', text($event))"></label>
      <div class="logo-editor">
        <div class="logo-preview" :style="{ background: preview.primary_color }"><img v-if="preview.logo_url" :src="preview.logo_url" :alt="t('presentation.logoPreview')"><span v-else>{{ preview.logo_text }}</span></div>
        <div class="field"><span>{{ t('presentation.logo') }}</span><div class="logo-controls"><label class="button button-secondary logo-upload"><span>{{ t('presentation.uploadLogo') }}</span><input v-write-guard="uploading" type="file" accept="image/png,image/jpeg" :aria-label="t('presentation.uploadLogo')" :disabled="locked" @change="withGlobalLoading(upload, $event)"></label><button v-if="preview.logo_url" class="button button-secondary" type="button" :disabled="locked" @click="update('logo_url', null)">{{ t('presentation.removeLogo') }}</button></div><small>{{ t('presentation.uploadHint') }}</small></div>
      </div>
      <p v-if="uploadError" role="alert" class="message error">{{ displayError(uploadError) }}</p>
      <fieldset class="theme-options"><legend>{{ t('presentation.theme') }}</legend><button v-for="theme in themes" :key="theme.name" class="button button-secondary theme-choice" type="button" :disabled="locked" :aria-pressed="preview.primary_color === theme.primary_color && preview.accent_color === theme.accent_color" @click="chooseTheme(theme)"><i :style="{ background: theme.primary_color }" aria-hidden="true" />{{ t(`presentation.theme.${theme.name}`) }}</button></fieldset>
      <div class="language-settings">
        <label class="field"><span>{{ t('admin.copy.ba3233b5') }}</span><select :value="preview.default_locale" :disabled="locked" @change="defaultLanguage"><option value="en">{{ t('admin.copy.61f7e041') }}</option><option value="zh-CN">{{ t('admin.copy.07a5470d') }}</option></select></label>
        <fieldset><legend>{{ t('admin.copy.bdd2c4f3') }}</legend><label v-for="language in (['en', 'zh-CN'] as const)" :key="language" class="choice"><input type="checkbox" :checked="preview.available_locales.includes(language)" :disabled="locked || preview.default_locale === language" @change="languageEnabled(language, $event)"><span>{{ language === 'en' ? t('admin.copy.61f7e041') : t('admin.copy.07a5470d') }}</span></label></fieldset>
      </div>
      <label v-for="language in (['en', 'zh-CN'] as const)" :key="language" class="field"><span>{{ t('admin.copy.8c339889', { locale: language === 'en' ? t('admin.copy.61f7e041') : t('admin.copy.07a5470d'), field: t('admin.copy.e6c15e4e') }) }}</span><textarea :value="preview.content[language].announcement" rows="3" :disabled="locked" @input="localized(language, 'announcement', text($event))" /></label>
    </div>
    <aside class="presentation-preview" :aria-label="t('presentation.preview')">
      <span class="preview-caption">{{ t('presentation.preview') }}</span>
      <div class="preview-card" :style="previewTokens">
        <div class="preview-heading"><div class="logo-preview" :style="{ background: preview.primary_color }"><img v-if="preview.logo_url" :src="preview.logo_url" :alt="t('presentation.logoPreview')"><span v-else>{{ preview.logo_text }}</span></div><strong>{{ preview.display_name }}</strong></div>
        <p>{{ preview.content[locale].tagline }}</p><p class="preview-announcement">{{ preview.content[locale].announcement }}</p><span class="preview-action">{{ t('presentation.previewAction') }}</span>
      </div>
    </aside>
    <details class="advanced-settings" :open="advanced" @toggle="advanced = ($event.target as HTMLDetailsElement).open">
      <summary>{{ t('presentation.advanced') }}</summary>
      <div class="advanced-grid">
        <label class="field"><span>{{ t('admin.copy.9b05d29b') }}</span><input :value="preview.logo_text" :disabled="locked" @input="update('logo_text', text($event))"></label>
        <label class="field"><span>{{ t('admin.copy.ea7330c6') }}</span><input :value="preview.favicon_url ?? ''" :disabled="locked" @input="update('favicon_url', text($event) || null)"></label>
        <label v-for="field in (['primary_color', 'accent_color', 'success_color', 'warning_color', 'danger_color'] as const)" :key="field" class="field"><span>{{ t(`presentation.${field}`) }}</span><input type="color" :value="preview[field]" :disabled="locked" @input="update(field, text($event))"></label>
        <label class="field"><span>{{ t('admin.copy.5f4c4d51') }}</span><select :value="preview.font_family" :disabled="locked" @change="update('font_family', text($event))"><option value="system">{{ t('admin.copy.532e2e04') }}</option><option value="serif">{{ t('admin.copy.f6dcd251') }}</option><option value="mono">{{ t('admin.copy.7d2288e0') }}</option></select></label>
        <label class="field"><span>{{ t('admin.copy.1d853666') }}</span><select :value="preview.font_scale" :disabled="locked" @change="update('font_scale', text($event))"><option value="compact">{{ t('admin.copy.766ee6d8') }}</option><option value="standard">{{ t('admin.copy.0d292885') }}</option></select></label>
        <label class="field"><span>{{ t('admin.copy.db91572d') }}</span><select :value="preview.radius" :disabled="locked" @change="update('radius', text($event))"><option value="square">{{ t('admin.copy.b8447f2a') }}</option><option value="soft">{{ t('admin.copy.ff51352b') }}</option><option value="round">{{ t('admin.copy.3c75314c') }}</option></select></label>
        <label class="field"><span>{{ t('admin.copy.ee99116b') }}</span><select :value="preview.shadow" :disabled="locked" @change="update('shadow', text($event))"><option value="none">{{ t('admin.copy.c2b41b33') }}</option><option value="subtle">{{ t('admin.copy.8f131d41') }}</option><option value="lifted">{{ t('admin.copy.92aee827') }}</option></select></label>
        <label v-for="language in (['en', 'zh-CN'] as const)" :key="language" class="field"><span>{{ t('admin.copy.8c339889', { locale: language === 'en' ? t('admin.copy.61f7e041') : t('admin.copy.07a5470d'), field: t('admin.copy.5018d51b') }) }}</span><input :value="preview.content[language].tagline" :disabled="locked" @input="localized(language, 'tagline', text($event))"></label>
      </div>
    </details>
    <div class="reset-settings"><button v-if="!resetConfirm" class="button button-secondary" type="button" :disabled="locked" @click="resetConfirm = true">{{ t('presentation.reset') }}</button><template v-else><p>{{ t('presentation.resetHint') }}</p><button class="button button-secondary" type="button" @click="resetConfirm = false">{{ t('admin.copy.96f851ea') }}</button><button class="button button-secondary" type="button" :disabled="locked" @click="reset">{{ t('presentation.confirmReset') }}</button></template><slot name="actions" /></div>
  </div>
</template>

<style scoped>
.presentation-fields { display:grid; grid-template-columns:minmax(0, 1.5fr) minmax(0, 1fr); gap:16px; min-width:0; }
.presentation-basic, .advanced-grid { display:grid; gap:12px; min-width:0; }
.field { align-self:start; align-content:start; height:auto; padding:0; border:0; border-radius:0; background:transparent; display:grid; gap:6px; min-width:0; color:var(--ink); }
.field input, .field textarea, .field select { width:100%; min-width:0; box-sizing:border-box; font:inherit; }
.brand-name-field { width:min(100%, 28ch); }
.field textarea { resize:vertical; }
.field small, .preview-caption { color:var(--muted); }
.logo-editor { display:flex; align-items:center; gap:12px; min-width:0; }
.logo-editor .field { flex:1; }
.logo-controls { display:flex; flex-wrap:wrap; gap:8px; }
.logo-upload { position:relative; overflow:hidden; }
.logo-upload input[type=file] { position:absolute; inset:0; width:100%; height:100%; opacity:0; cursor:pointer; }
.logo-upload:focus-within { outline:2px solid var(--primary); outline-offset:2px; }
.logo-preview { display:grid; place-items:center; width:56px; height:56px; flex:0 0 56px; border-radius:var(--radius-sm); color:var(--surface); font-weight:700; overflow:hidden; }
.logo-preview img { width:100%; height:100%; object-fit:contain; }
fieldset { margin:0; padding:0; border:0; min-width:0; }
.theme-options { display:flex; flex-wrap:wrap; gap:8px; }
.theme-options legend { margin-bottom:6px; }
.theme-choice { display:flex; align-items:center; gap:8px; }
.theme-choice[aria-pressed="true"] { border-color:var(--primary); }
.theme-choice i { width:18px; height:18px; border-radius:50%; }
.language-settings { display:grid; grid-template-columns:1fr 1fr; gap:12px; }
.choice { display:inline-flex; align-items:center; gap:8px; min-height:44px; margin-right:12px; }
.choice input { width:18px; height:18px; min-height:18px; margin:0; accent-color:var(--primary); }
.presentation-preview { min-width:0; }
.preview-card { font-family:var(--font-family); font-size:var(--font-scale); box-shadow:var(--shadow); padding:16px; margin-top:8px; border:1px solid var(--line); border-radius:var(--radius); background:var(--surface); overflow-wrap:anywhere; }
.preview-heading { display:flex; align-items:center; gap:12px; }
.preview-card p { white-space:pre-wrap; }
.preview-action { display:inline-block; padding:8px 16px; background:var(--primary); color:var(--surface); border-radius:var(--radius-sm); }
.advanced-settings, .reset-settings { grid-column:1/-1; }
.advanced-settings summary { cursor:pointer; min-height:44px; box-sizing:border-box; padding:12px 0; color:var(--primary); }
.advanced-grid { grid-template-columns:repeat(auto-fit,minmax(min(100%,220px),1fr)); }
.reset-settings { display:flex; flex-wrap:wrap; gap:8px; align-items:center; }
.reset-settings p { flex-basis:100%; }
@media (max-width:700px) { .presentation-fields { grid-template-columns:minmax(0,1fr); } }
@media (max-width:360px) { .language-settings { grid-template-columns:minmax(0,1fr); } }
</style>
