<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import { usePlatformI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import TimeZoneSelect from '@lottery/shared/TimeZoneSelect.vue'
import DetailModal from '@lottery/shared/DetailModal.vue'
import { vInlineValidation } from '@lottery/shared'
import { computed, ref, watch } from 'vue'
import { usePlatformI18n } from './i18n'
import { createPlatformApi, type PlatformSettings } from './platform-api'

const props = defineProps<{ initial: PlatformSettings; locale: 'en' | 'zh-CN' }>()
const emit = defineEmits<{ saved: [settings: PlatformSettings]; failure: [cause: unknown] }>()
const api = createPlatformApi()
const { t } = usePlatformI18n()
const form = ref({
  version: props.initial.version,
  display_name: props.initial.display_name,
  logo_text: props.initial.logo_text,
  logo_url: props.initial.logo_url,
  timezone: props.initial.timezone,
  date_format: props.initial.date_format,
  time_format: props.initial.time_format,
})
const current = ref(props.initial)
const editorOpen = ref(false)
watch(() => props.initial, value => { current.value = value })
function startEdit() {
  const { version, display_name, logo_text, logo_url, timezone, date_format, time_format } = current.value
  form.value = { version, display_name, logo_text, logo_url, timezone, date_format, time_format }
  error.value = ''; editorOpen.value = true
}
const busy = ref(false)
const error = ref<string | import("@lottery/shared").LocalizedMessage>("")
const copy = computed(() => ({
  title: t('common.platformSettings'), description: t('common.platformSettingsDescription'), displayName: t('common.platformDisplayName'), logoText: t('common.platformLogoText'), logoUrl: t('common.platformLogoUrl'), timezone: t('common.timeZone'), dateFormat: t('common.dateFormat'), timeFormat: t('common.timeFormat'), save: t('common.saveSettings'), saving: t('common.saving'), dateYMD: t('common.dateFormatYmd'), dateDMY: t('common.dateFormatDmy'), dateMDY: t('common.dateFormatMdy'), time24: t('common.timeFormat24'), time12: t('common.timeFormat12'),
}))

async function save() {
  busy.value = true; error.value = ''
  try {
    const next = await api.updatePlatformSettings(form.value)
    current.value = next
    form.value.version = next.version
    editorOpen.value = false
    emit('saved', next)
  } catch (cause) {
    error.value = cause instanceof Error ? publicErrorMessage(cause) : publicErrorMessage(cause)
    emit('failure', cause)
  } finally { busy.value = false }
}
</script>

<template>
  <div class="platform-settings-page">
    <div class="page-heading compact-heading"><div><h1>{{ copy.title }}</h1></div></div>
    <section class="panel">
      <div class="panel-heading"><button type="button" class="button button-secondary" @click="startEdit">{{ t('common.editSettings') }}</button></div>
      <dl class="confirm-list">
        <div><dt>{{ copy.displayName }}</dt><dd>{{ current.display_name }}</dd></div>
        <div><dt>{{ copy.logoText }}</dt><dd>{{ current.logo_text }}</dd></div>
        <div><dt>{{ copy.logoUrl }}</dt><dd>{{ current.logo_url || '—' }}</dd></div>
        <div><dt>{{ copy.timezone }}</dt><dd>{{ current.timezone }}</dd></div>
        <div><dt>{{ copy.dateFormat }}</dt><dd>{{ current.date_format === 'YYYY-MM-DD' ? copy.dateYMD : current.date_format === 'DD/MM/YYYY' ? copy.dateDMY : copy.dateMDY }}</dd></div>
        <div><dt>{{ copy.timeFormat }}</dt><dd>{{ current.time_format === 'HH:mm:ss' ? copy.time24 : copy.time12 }}</dd></div>
      </dl>
    </section>
    <DetailModal v-if="editorOpen" :title="copy.title" :close-label="t('platform.copy.27b2fd29')" @close="!busy && (editorOpen = false)">
    <form v-write-guard="busy" novalidate v-inline-validation class="brand-form platform-settings-form" @submit.prevent="withGlobalLoading(save)">
      <div class="form-row"><label>{{ copy.displayName }}<input v-model.trim="form.display_name" maxlength="120" required /></label><label>{{ copy.logoText }}<input v-model.trim="form.logo_text" maxlength="8" required /></label></div>
      <label>{{ copy.logoUrl }}<input v-model.trim="form.logo_url" maxlength="512" /></label>
      <div class="form-row"><label>{{ copy.timezone }}<TimeZoneSelect v-model="form.timezone" :locale="locale" required /></label><label>{{ copy.dateFormat }}<select v-model="form.date_format"><option value="YYYY-MM-DD">{{ copy.dateYMD }}</option><option value="DD/MM/YYYY">{{ copy.dateDMY }}</option><option value="MM/DD/YYYY">{{ copy.dateMDY }}</option></select></label></div>
      <div class="form-row"><label>{{ copy.timeFormat }}<select v-model="form.time_format"><option value="HH:mm:ss">{{ copy.time24 }}</option><option value="hh:mm:ss A">{{ copy.time12 }}</option></select></label></div>
      <p v-if="error" class="message error" role="alert">{{ displayPublicError(error) }}</p>
      <div class="form-actions"><button type="button" class="button button-secondary" :disabled="busy" @click="editorOpen = false">{{ t('platform.copy.2230b1a7') }}</button><button type="submit" class="button button-primary" :disabled="busy">{{ busy ? copy.saving : copy.save }}</button></div>
    </form>
    </DetailModal>
  </div>
</template>
