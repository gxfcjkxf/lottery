<script setup lang="ts">
import type { BrandPresentationConfig, BrandPresentationEffective, BrandPresentationLocale } from "./brand-presentation-api";
import { useAdminI18n } from "./i18n";

const { t } = useAdminI18n();

const props = defineProps<{
  config: BrandPresentationConfig;
  effective: BrandPresentationEffective;
  disabled: boolean;
}>();
const emit = defineEmits<{ "update:config": [value: BrandPresentationConfig] }>();

type NullableScalar = Exclude<keyof BrandPresentationConfig, "available_locales" | "content">;
type LocalizedField = "tagline" | "announcement";

function cloneCompleteConfig(source: BrandPresentationConfig): BrandPresentationConfig {
  return {
    ...source,
    available_locales: source.available_locales === null ? null : [...source.available_locales],
    content: source.content === null ? null : {
      en: { tagline: source.content.en.tagline, announcement: source.content.en.announcement },
      "zh-CN": { tagline: source.content["zh-CN"].tagline, announcement: source.content["zh-CN"].announcement },
    },
  };
}

function updateField(key: NullableScalar, value: BrandPresentationConfig[NullableScalar]): void {
  const next = cloneCompleteConfig(props.config);
  Object.assign(next, { [key]: value });
  emit("update:config", next);
}

function setInherited(key: NullableScalar, event: Event): void {
  const checked = (event.target as HTMLInputElement).checked;
  const value = checked ? null : key === "logo_url" ? props.effective.logo_url ?? "" : key === "favicon_url" ? props.effective.favicon_url ?? "" : props.effective[key];
  updateField(key, value);
}

function textValue(event: Event): string {
  return (event.target as HTMLInputElement).value;
}

function setAvailableLocalesInherited(event: Event): void {
  const checked = (event.target as HTMLInputElement).checked;
  const locales = checked ? null : [...props.effective.available_locales];
  const next = cloneCompleteConfig(props.config);
  next.available_locales = locales;
  emit("update:config", next);
}

function setLocaleEnabled(locale: BrandPresentationLocale, event: Event): void {
  const checked = (event.target as HTMLInputElement).checked;
  const current = props.config.available_locales ?? [];
  const locales = current.filter((item) => item !== locale);
  if (checked) locales.push(locale);
  const next = cloneCompleteConfig(props.config);
  next.available_locales = locales;
  emit("update:config", next);
}

function setContentInherited(event: Event): void {
  const checked = (event.target as HTMLInputElement).checked;
  const content = checked ? null : {
    en: { tagline: props.effective.content.en.tagline, announcement: props.effective.content.en.announcement },
    "zh-CN": { tagline: props.effective.content["zh-CN"].tagline, announcement: props.effective.content["zh-CN"].announcement },
  };
  const next = cloneCompleteConfig(props.config);
  next.content = content;
  emit("update:config", next);
}

function setLocalizedInherited(locale: BrandPresentationLocale, field: LocalizedField, event: Event): void {
  if (!props.config.content) return;
  const checked = (event.target as HTMLInputElement).checked;
  setLocalizedValue(locale, field, checked ? null : props.effective.content[locale][field]);
}

function setLocalizedValue(locale: BrandPresentationLocale, field: LocalizedField, value: string | null): void {
  if (!props.config.content) return;
  const next = cloneCompleteConfig(props.config);
  if (!next.content) return;
  next.content[locale] = { ...next.content[locale], [field]: value };
  emit("update:config", next);
}

function isLocalizedInherited(locale: BrandPresentationLocale, field: LocalizedField): boolean {
  return props.config.content === null || props.config.content[locale][field] === null;
}
</script>

<template>
  <div class="presentation-fields" :aria-disabled="disabled">
    <fieldset class="field-group">
      <legend>{{ t("身份", "Identity") }}</legend>
      <label class="field">
        <span>{{ t("展示名称", "Display name") }}</span>
        <input :value="config.display_name ?? ''" :placeholder="effective.display_name" :disabled="disabled || config.display_name === null" type="text" :aria-label="t('展示名称', 'Display name')" @input="updateField('display_name', textValue($event))">
      </label>
      <label class="inherit-toggle"><input type="checkbox" :checked="config.display_name === null" :disabled="disabled" :aria-label="t('{field}继承默认', '{field} inherit default', { field: t('展示名称', 'Display name') })" @change="setInherited('display_name', $event)"><span>{{ t("继承默认", "Inherit default") }}</span></label>

      <label class="field">
        <span>{{ t("标志文字", "Logo text") }}</span>
        <input :value="config.logo_text ?? ''" :placeholder="effective.logo_text" :disabled="disabled || config.logo_text === null" type="text" :aria-label="t('标志文字', 'Logo text')" @input="updateField('logo_text', textValue($event))">
      </label>
      <label class="inherit-toggle"><input type="checkbox" :checked="config.logo_text === null" :disabled="disabled" :aria-label="t('{field}继承默认', '{field} inherit default', { field: t('标志文字', 'Logo text') })" @change="setInherited('logo_text', $event)"><span>{{ t("继承默认", "Inherit default") }}</span></label>

      <label class="field">
        <span>{{ t("Logo地址", "Logo URL") }}</span>
        <input :value="config.logo_url ?? ''" :placeholder="effective.logo_url ?? t('未设置', 'Not set')" :disabled="disabled || config.logo_url === null" type="text" inputmode="url" autocomplete="off" :aria-label="t('Logo地址', 'Logo URL')" @input="updateField('logo_url', textValue($event))">
      </label>
      <label class="inherit-toggle"><input type="checkbox" :checked="config.logo_url === null" :disabled="disabled" :aria-label="t('{field}继承默认', '{field} inherit default', { field: t('Logo地址', 'Logo URL') })" @change="setInherited('logo_url', $event)"><span>{{ t("继承默认", "Inherit default") }}</span></label>

      <label class="field">
        <span>{{ t("favicon地址", "Favicon URL") }}</span>
        <input :value="config.favicon_url ?? ''" :placeholder="effective.favicon_url ?? t('未设置', 'Not set')" :disabled="disabled || config.favicon_url === null" type="text" inputmode="url" autocomplete="off" :aria-label="t('favicon地址', 'Favicon URL')" @input="updateField('favicon_url', textValue($event))">
      </label>
      <label class="inherit-toggle"><input type="checkbox" :checked="config.favicon_url === null" :disabled="disabled" :aria-label="t('{field}继承默认', '{field} inherit default', { field: t('favicon地址', 'Favicon URL') })" @change="setInherited('favicon_url', $event)"><span>{{ t("继承默认", "Inherit default") }}</span></label>
    </fieldset>

    <fieldset class="field-group">
      <legend>{{ t("颜色", "Colors") }}</legend>
      <div v-for="item in [
        { key: 'primary_color', label: t('主色', 'Primary color') },
        { key: 'accent_color', label: t('辅助色', 'Accent color') },
        { key: 'success_color', label: t('成功色', 'Success color') },
        { key: 'warning_color', label: t('警告色', 'Warning color') },
        { key: 'danger_color', label: t('错误色', 'Error color') },
      ]" :key="item.key" class="field color-field">
        <span>{{ item.label }}</span>
        <input :value="(config[item.key as keyof BrandPresentationConfig] as string | null) ?? effective[item.key as keyof BrandPresentationEffective] as string" :disabled="disabled || config[item.key as keyof BrandPresentationConfig] === null" type="color" :aria-label="item.label" @input="updateField(item.key as NullableScalar, textValue($event))">
        <span class="color-value">{{ config[item.key as keyof BrandPresentationConfig] ?? effective[item.key as keyof BrandPresentationEffective] }}</span>
        <label class="inherit-toggle"><input type="checkbox" :checked="config[item.key as keyof BrandPresentationConfig] === null" :disabled="disabled" :aria-label="t('{field}继承默认', '{field} inherit default', { field: item.label })" @change="setInherited(item.key as NullableScalar, $event)"><span>{{ t("继承默认", "Inherit default") }}</span></label>
      </div>
    </fieldset>

    <fieldset class="field-group">
      <legend>{{ t("样式", "Style") }}</legend>
      <label class="field">
        <span>{{ t("字体预设", "Font preset") }}</span>
        <select :value="config.font_family ?? ''" :disabled="disabled || config.font_family === null" :aria-label="t('字体预设', 'Font preset')" @change="updateField('font_family', ($event.target as HTMLSelectElement).value as BrandPresentationConfig['font_family'])">
          <option value="">{{ t("继承默认", "Inherit default") }}（{{ effective.font_family }}）</option><option value="system">{{ t("系统", "System") }}</option><option value="serif">{{ t("衬线", "Serif") }}</option><option value="mono">{{ t("等宽", "Monospace") }}</option>
        </select>
      </label>
      <label class="inherit-toggle"><input type="checkbox" :checked="config.font_family === null" :disabled="disabled" :aria-label="t('{field}继承默认', '{field} inherit default', { field: t('字体预设', 'Font preset') })" @change="setInherited('font_family', $event)"><span>{{ t("继承默认", "Inherit default") }}</span></label>

      <label class="field">
        <span>{{ t("字号预设", "Font size preset") }}</span>
        <select :value="config.font_scale ?? ''" :disabled="disabled || config.font_scale === null" :aria-label="t('字号预设', 'Font size preset')" @change="updateField('font_scale', ($event.target as HTMLSelectElement).value as BrandPresentationConfig['font_scale'])">
          <option value="">{{ t("继承默认", "Inherit default") }}（{{ effective.font_scale }}）</option><option value="compact">{{ t("紧凑", "Compact") }}</option><option value="standard">{{ t("标准", "Standard") }}</option><option value="large">{{ t("大号", "Large") }}</option>
        </select>
      </label>
      <label class="inherit-toggle"><input type="checkbox" :checked="config.font_scale === null" :disabled="disabled" :aria-label="t('{field}继承默认', '{field} inherit default', { field: t('字号预设', 'Font size preset') })" @change="setInherited('font_scale', $event)"><span>{{ t("继承默认", "Inherit default") }}</span></label>

      <label class="field">
        <span>{{ t("圆角预设", "Corner radius preset") }}</span>
        <select :value="config.radius ?? ''" :disabled="disabled || config.radius === null" :aria-label="t('圆角预设', 'Corner radius preset')" @change="updateField('radius', ($event.target as HTMLSelectElement).value as BrandPresentationConfig['radius'])">
          <option value="">{{ t("继承默认", "Inherit default") }}（{{ effective.radius }}）</option><option value="square">{{ t("直角", "Square") }}</option><option value="soft">{{ t("柔和", "Soft") }}</option><option value="round">{{ t("圆润", "Rounded") }}</option>
        </select>
      </label>
      <label class="inherit-toggle"><input type="checkbox" :checked="config.radius === null" :disabled="disabled" :aria-label="t('{field}继承默认', '{field} inherit default', { field: t('圆角预设', 'Corner radius preset') })" @change="setInherited('radius', $event)"><span>{{ t("继承默认", "Inherit default") }}</span></label>

      <label class="field">
        <span>{{ t("阴影预设", "Shadow preset") }}</span>
        <select :value="config.shadow ?? ''" :disabled="disabled || config.shadow === null" :aria-label="t('阴影预设', 'Shadow preset')" @change="updateField('shadow', ($event.target as HTMLSelectElement).value as BrandPresentationConfig['shadow'])">
          <option value="">{{ t("继承默认", "Inherit default") }}（{{ effective.shadow }}）</option><option value="none">{{ t("无", "None") }}</option><option value="subtle">{{ t("轻微", "Subtle") }}</option><option value="lifted">{{ t("明显", "Prominent") }}</option>
        </select>
      </label>
      <label class="inherit-toggle"><input type="checkbox" :checked="config.shadow === null" :disabled="disabled" :aria-label="t('{field}继承默认', '{field} inherit default', { field: t('阴影预设', 'Shadow preset') })" @change="setInherited('shadow', $event)"><span>{{ t("继承默认", "Inherit default") }}</span></label>
    </fieldset>

    <fieldset class="field-group">
      <legend>{{ t("语言", "Language") }}</legend>
      <label class="field">
        <span>{{ t("默认语言", "Default language") }}</span>
        <select :value="config.default_locale ?? ''" :disabled="disabled || config.default_locale === null" :aria-label="t('默认语言', 'Default language')" @change="updateField('default_locale', ($event.target as HTMLSelectElement).value as BrandPresentationConfig['default_locale'])">
          <option value="">{{ t("继承默认", "Inherit default") }}（{{ effective.default_locale }}）</option><option value="en">English</option><option value="zh-CN">简体中文</option>
        </select>
      </label>
      <label class="inherit-toggle"><input type="checkbox" :checked="config.default_locale === null" :disabled="disabled" :aria-label="t('{field}继承默认', '{field} inherit default', { field: t('默认语言', 'Default language') })" @change="setInherited('default_locale', $event)"><span>{{ t("继承默认", "Inherit default") }}</span></label>

      <div class="locale-field">
        <span>{{ t("可用语言", "Available languages") }}</span>
        <label class="inherit-toggle"><input type="checkbox" :checked="config.available_locales === null" :disabled="disabled" :aria-label="t('{field}继承默认', '{field} inherit default', { field: t('可用语言', 'Available languages') })" @change="setAvailableLocalesInherited($event)"><span>{{ t("继承默认", "Inherit default") }}</span></label>
        <div class="locale-options" :aria-disabled="disabled || config.available_locales === null">
          <label class="choice"><input type="checkbox" :checked="(config.available_locales ?? effective.available_locales).includes('en')" :disabled="disabled || config.available_locales === null" :aria-label="t('启用{locale}', 'Enable {locale}', { locale: 'English' })" @change="setLocaleEnabled('en', $event)"><span>English</span></label>
          <label class="choice"><input type="checkbox" :checked="(config.available_locales ?? effective.available_locales).includes('zh-CN')" :disabled="disabled || config.available_locales === null" :aria-label="t('启用{locale}', 'Enable {locale}', { locale: t('简体中文', 'Simplified Chinese') })" @change="setLocaleEnabled('zh-CN', $event)"><span>简体中文</span></label>
        </div>
      </div>
    </fieldset>

    <fieldset class="field-group copy-group">
      <legend>{{ t("文案", "Copy") }}</legend>
      <label class="inherit-toggle"><input type="checkbox" :checked="config.content === null" :disabled="disabled" :aria-label="t('{field}继承默认', '{field} inherit default', { field: t('文案', 'Copy') })" @change="setContentInherited($event)"><span>{{ t("文案继承默认", "Inherit default copy") }}</span></label>
      <template v-for="locale in (['en', 'zh-CN'] as const)" :key="locale">
        <template v-for="field in (['tagline', 'announcement'] as const)" :key="`${locale}-${field}`">
          <label class="field localized-field">
            <span>{{ t('{locale}{field}', '{locale} {field}', { locale: locale === 'en' ? t('英文', 'English') : t('中文', 'Chinese'), field: field === 'tagline' ? t('标语', 'tagline') : t('公告', 'announcement') }) }}</span>
            <textarea :value="config.content?.[locale][field] ?? ''" :placeholder="effective.content[locale][field]" :disabled="disabled || config.content === null || config.content[locale][field] === null" :aria-label="t('{locale}{field}', '{locale} {field}', { locale: locale === 'en' ? t('英文', 'English') : t('中文', 'Chinese'), field: field === 'tagline' ? t('标语', 'tagline') : t('公告', 'announcement') })" rows="3" @input="setLocalizedValue(locale, field, textValue($event))" />
            <label class="inherit-toggle"><input type="checkbox" :checked="isLocalizedInherited(locale, field)" :disabled="disabled || config.content === null" :aria-label="t('{label}继承默认', '{label} inherit default', { label: t('{locale}{field}', '{locale} {field}', { locale: locale === 'en' ? t('英文', 'English') : t('中文', 'Chinese'), field: field === 'tagline' ? t('标语', 'tagline') : t('公告', 'announcement') }) })" @change="setLocalizedInherited(locale, field, $event)"><span>{{ t("继承默认", "Inherit default") }}</span></label>
          </label>
        </template>
      </template>
    </fieldset>
  </div>
</template>

<style scoped>
.presentation-fields {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 300px), 1fr));
  gap: 14px;
  min-width: 0;
}
.field-group {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-content: start;
  gap: 8px 12px;
  min-width: 0;
  margin: 0;
  padding: 14px;
  border: 1px solid #d8dee8;
  border-radius: 10px;
  background: #fff;
}
.field-group legend {
  padding: 0 6px;
  color: #344054;
  font-weight: 650;
}
.field {
  display: grid;
  grid-column: 1 / -1;
  gap: 5px;
  min-width: 0;
  height: auto;
  padding: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
  color: #344054;
  font-size: 0.9rem;
}
.field input:not([type="checkbox"]), .field select, .field textarea {
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  min-height: 44px;
  padding: 9px 10px;
  border: 1px solid #cbd2dc;
  border-radius: 7px;
  background: #fff;
  color: #17202e;
  font: inherit;
}
.field textarea { resize: vertical; }
.field input::placeholder, .field textarea::placeholder { color: #737f90; opacity: 1; }
.field input:disabled, .field select:disabled, .field textarea:disabled { background: #f2f4f7; color: #667085; }
.inherit-toggle, .choice {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 44px;
  color: #475467;
  font-size: 0.85rem;
  cursor: pointer;
}
.inherit-toggle { grid-column: 1 / -1; justify-self: start; }
.inherit-toggle input, .choice input {
  width: 44px;
  height: 44px;
  margin: 0;
  accent-color: #315f9d;
  flex: 0 0 auto;
}
.color-field { grid-template-columns: minmax(0, 1fr) auto; align-items: center; }
.color-field > span:first-child { grid-column: 1 / -1; }
.color-field input[type="color"] { width: 48px; height: 44px; padding: 4px; border: 1px solid #cbd2dc; border-radius: 7px; }
.color-value { color: #667085; font: 0.82rem ui-monospace, monospace; }
.color-field .inherit-toggle { grid-column: 1 / -1; }
.locale-field { grid-column: 1 / -1; display: grid; gap: 4px; color: #344054; font-size: 0.9rem; }
.locale-options { display: flex; flex-wrap: wrap; gap: 4px 14px; }
.copy-group > .inherit-toggle { margin-bottom: 2px; }
.localized-field { padding-top: 4px; }
@media (max-width: 360px) {
  .field-group { padding: 11px; }
  .presentation-fields { grid-template-columns: minmax(0, 1fr); }
}
</style>
