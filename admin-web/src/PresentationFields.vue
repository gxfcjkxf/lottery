<script setup lang="ts">
import type { BrandPresentationConfig, BrandPresentationEffective, BrandPresentationLocale } from "./brand-presentation-api";

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
      <legend>身份</legend>
      <label class="field">
        <span>展示名称</span>
        <input :value="config.display_name ?? ''" :placeholder="effective.display_name" :disabled="disabled || config.display_name === null" type="text" aria-label="展示名称" @input="updateField('display_name', textValue($event))">
      </label>
      <label class="inherit-toggle"><input type="checkbox" :checked="config.display_name === null" :disabled="disabled" aria-label="展示名称继承默认" @change="setInherited('display_name', $event)"><span>继承默认</span></label>

      <label class="field">
        <span>标志文字</span>
        <input :value="config.logo_text ?? ''" :placeholder="effective.logo_text" :disabled="disabled || config.logo_text === null" type="text" aria-label="标志文字" @input="updateField('logo_text', textValue($event))">
      </label>
      <label class="inherit-toggle"><input type="checkbox" :checked="config.logo_text === null" :disabled="disabled" aria-label="标志文字继承默认" @change="setInherited('logo_text', $event)"><span>继承默认</span></label>

      <label class="field">
        <span>Logo地址</span>
        <input :value="config.logo_url ?? ''" :placeholder="effective.logo_url ?? '未设置'" :disabled="disabled || config.logo_url === null" type="text" inputmode="url" autocomplete="off" aria-label="Logo地址" @input="updateField('logo_url', textValue($event))">
      </label>
      <label class="inherit-toggle"><input type="checkbox" :checked="config.logo_url === null" :disabled="disabled" aria-label="Logo地址继承默认" @change="setInherited('logo_url', $event)"><span>继承默认</span></label>

      <label class="field">
        <span>favicon地址</span>
        <input :value="config.favicon_url ?? ''" :placeholder="effective.favicon_url ?? '未设置'" :disabled="disabled || config.favicon_url === null" type="text" inputmode="url" autocomplete="off" aria-label="favicon地址" @input="updateField('favicon_url', textValue($event))">
      </label>
      <label class="inherit-toggle"><input type="checkbox" :checked="config.favicon_url === null" :disabled="disabled" aria-label="favicon地址继承默认" @change="setInherited('favicon_url', $event)"><span>继承默认</span></label>
    </fieldset>

    <fieldset class="field-group">
      <legend>颜色</legend>
      <div v-for="item in [
        { key: 'primary_color', label: '主色' },
        { key: 'accent_color', label: '辅助色' },
        { key: 'success_color', label: '成功色' },
        { key: 'warning_color', label: '警告色' },
        { key: 'danger_color', label: '错误色' },
      ]" :key="item.key" class="field color-field">
        <span>{{ item.label }}</span>
        <input :value="(config[item.key as keyof BrandPresentationConfig] as string | null) ?? effective[item.key as keyof BrandPresentationEffective] as string" :disabled="disabled || config[item.key as keyof BrandPresentationConfig] === null" type="color" :aria-label="item.label" @input="updateField(item.key as NullableScalar, textValue($event))">
        <span class="color-value">{{ config[item.key as keyof BrandPresentationConfig] ?? effective[item.key as keyof BrandPresentationEffective] }}</span>
        <label class="inherit-toggle"><input type="checkbox" :checked="config[item.key as keyof BrandPresentationConfig] === null" :disabled="disabled" :aria-label="`${item.label}继承默认`" @change="setInherited(item.key as NullableScalar, $event)"><span>继承默认</span></label>
      </div>
    </fieldset>

    <fieldset class="field-group">
      <legend>样式</legend>
      <label class="field">
        <span>字体预设</span>
        <select :value="config.font_family ?? ''" :disabled="disabled || config.font_family === null" aria-label="字体预设" @change="updateField('font_family', ($event.target as HTMLSelectElement).value as BrandPresentationConfig['font_family'])">
          <option value="">继承默认（{{ effective.font_family }}）</option><option value="system">系统</option><option value="serif">衬线</option><option value="mono">等宽</option>
        </select>
      </label>
      <label class="inherit-toggle"><input type="checkbox" :checked="config.font_family === null" :disabled="disabled" aria-label="字体预设继承默认" @change="setInherited('font_family', $event)"><span>继承默认</span></label>

      <label class="field">
        <span>字号预设</span>
        <select :value="config.font_scale ?? ''" :disabled="disabled || config.font_scale === null" aria-label="字号预设" @change="updateField('font_scale', ($event.target as HTMLSelectElement).value as BrandPresentationConfig['font_scale'])">
          <option value="">继承默认（{{ effective.font_scale }}）</option><option value="compact">紧凑</option><option value="standard">标准</option><option value="large">大号</option>
        </select>
      </label>
      <label class="inherit-toggle"><input type="checkbox" :checked="config.font_scale === null" :disabled="disabled" aria-label="字号预设继承默认" @change="setInherited('font_scale', $event)"><span>继承默认</span></label>

      <label class="field">
        <span>圆角预设</span>
        <select :value="config.radius ?? ''" :disabled="disabled || config.radius === null" aria-label="圆角预设" @change="updateField('radius', ($event.target as HTMLSelectElement).value as BrandPresentationConfig['radius'])">
          <option value="">继承默认（{{ effective.radius }}）</option><option value="square">直角</option><option value="soft">柔和</option><option value="round">圆润</option>
        </select>
      </label>
      <label class="inherit-toggle"><input type="checkbox" :checked="config.radius === null" :disabled="disabled" aria-label="圆角预设继承默认" @change="setInherited('radius', $event)"><span>继承默认</span></label>

      <label class="field">
        <span>阴影预设</span>
        <select :value="config.shadow ?? ''" :disabled="disabled || config.shadow === null" aria-label="阴影预设" @change="updateField('shadow', ($event.target as HTMLSelectElement).value as BrandPresentationConfig['shadow'])">
          <option value="">继承默认（{{ effective.shadow }}）</option><option value="none">无</option><option value="subtle">轻微</option><option value="lifted">明显</option>
        </select>
      </label>
      <label class="inherit-toggle"><input type="checkbox" :checked="config.shadow === null" :disabled="disabled" aria-label="阴影预设继承默认" @change="setInherited('shadow', $event)"><span>继承默认</span></label>
    </fieldset>

    <fieldset class="field-group">
      <legend>语言</legend>
      <label class="field">
        <span>默认语言</span>
        <select :value="config.default_locale ?? ''" :disabled="disabled || config.default_locale === null" aria-label="默认语言" @change="updateField('default_locale', ($event.target as HTMLSelectElement).value as BrandPresentationConfig['default_locale'])">
          <option value="">继承默认（{{ effective.default_locale }}）</option><option value="en">English</option><option value="zh-CN">简体中文</option>
        </select>
      </label>
      <label class="inherit-toggle"><input type="checkbox" :checked="config.default_locale === null" :disabled="disabled" aria-label="默认语言继承默认" @change="setInherited('default_locale', $event)"><span>继承默认</span></label>

      <div class="locale-field">
        <span>可用语言</span>
        <label class="inherit-toggle"><input type="checkbox" :checked="config.available_locales === null" :disabled="disabled" aria-label="可用语言继承默认" @change="setAvailableLocalesInherited($event)"><span>继承默认</span></label>
        <div class="locale-options" :aria-disabled="disabled || config.available_locales === null">
          <label class="choice"><input type="checkbox" :checked="(config.available_locales ?? effective.available_locales).includes('en')" :disabled="disabled || config.available_locales === null" aria-label="启用English" @change="setLocaleEnabled('en', $event)"><span>English</span></label>
          <label class="choice"><input type="checkbox" :checked="(config.available_locales ?? effective.available_locales).includes('zh-CN')" :disabled="disabled || config.available_locales === null" aria-label="启用简体中文" @change="setLocaleEnabled('zh-CN', $event)"><span>简体中文</span></label>
        </div>
      </div>
    </fieldset>

    <fieldset class="field-group copy-group">
      <legend>文案</legend>
      <label class="inherit-toggle"><input type="checkbox" :checked="config.content === null" :disabled="disabled" aria-label="文案继承默认" @change="setContentInherited($event)"><span>文案继承默认</span></label>
      <template v-for="locale in (['en', 'zh-CN'] as const)" :key="locale">
        <template v-for="field in (['tagline', 'announcement'] as const)" :key="`${locale}-${field}`">
          <label class="field localized-field">
            <span>{{ locale === 'en' ? '英文' : '中文' }}{{ field === 'tagline' ? '标语' : '公告' }}</span>
            <textarea :value="config.content?.[locale][field] ?? ''" :placeholder="effective.content[locale][field]" :disabled="disabled || config.content === null || config.content[locale][field] === null" :aria-label="`${locale === 'en' ? '英文' : '中文'}${field === 'tagline' ? '标语' : '公告'}`" rows="3" @input="setLocalizedValue(locale, field, textValue($event))" />
            <label class="inherit-toggle"><input type="checkbox" :checked="isLocalizedInherited(locale, field)" :disabled="disabled || config.content === null" :aria-label="`${locale === 'en' ? '英文' : '中文'}${field === 'tagline' ? '标语' : '公告'}继承默认`" @change="setLocalizedInherited(locale, field, $event)"><span>继承默认</span></label>
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
