<script setup lang="ts">
import { ref, useAttrs, watch } from 'vue'
import type { LocaleCode } from './brand'
import { createCatalogI18n } from './localization'
import SiteIcon from './SiteIcon'

defineOptions({ inheritAttrs: false })
const value = defineModel<string>({ required: true })
const props = defineProps<{ locale: LocaleCode; disabled?: boolean; readonly?: boolean }>()
const emit = defineEmits<{ input: [event: Event] }>()
const attrs = useAttrs()
const input = ref<HTMLInputElement>()
const i18n = createCatalogI18n({ en: { clear: 'Clear input' }, 'zh-CN': { clear: '清除输入' } }, props.locale)
watch(() => props.locale, locale => i18n.setLocale(locale))
function update(event: Event) {
  value.value = (event.target as HTMLInputElement).value
  emit('input', event)
}
function clear() {
  value.value = ''
  emit('input', new Event('input'))
  input.value?.focus()
}
</script>
<template>
  <span class="clearable-input">
    <input v-bind="attrs" ref="input" type="search" :value="value" :disabled="disabled" :readonly="readonly" @input="update" />
    <button v-if="value && !readonly" type="button" class="clearable-input-clear" :disabled="disabled" :aria-label="i18n.t('clear')" @pointerdown.prevent @click="clear"><SiteIcon name="close" /></button>
  </span>
</template>
<style scoped>
.clearable-input { position:relative; display:inline-flex; align-items:center; width:var(--input-width,168px); min-width:0; max-width:100%; }
#app .clearable-input > input { box-sizing:border-box; width:100%!important; max-width:100%; padding-inline-end:44px!important; }
.clearable-input > input::-webkit-search-cancel-button, .clearable-input > input::-webkit-search-decoration { display:none; appearance:none; }
#app .clearable-input > .clearable-input-clear { position:absolute; inset-inline-end:0; top:50%; transform:translateY(-50%); display:inline-flex; align-items:center; justify-content:center; width:44px; min-width:44px; height:100%; min-height:0; padding:0; border:0; border-radius:var(--radius-sm); background:transparent; color:var(--muted); cursor:pointer; }
#app .clearable-input > .clearable-input-clear:hover:not(:disabled) { color:var(--primary); }
#app .clearable-input > .clearable-input-clear:focus-visible { outline:2px solid var(--primary); outline-offset:-3px; }
#app .clearable-input > .clearable-input-clear:disabled { opacity:.5; cursor:not-allowed; }
</style>
