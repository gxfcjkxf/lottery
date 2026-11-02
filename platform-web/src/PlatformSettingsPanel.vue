<script setup lang="ts">
import SelectControl from '@lottery/shared/SelectControl.vue'
import { useOperationToast } from "@lottery/shared";
import { vWriteGuard } from "@lottery/shared";
import { usePlatformI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import TimeZoneSelect from '@lottery/shared/TimeZoneSelect.vue'
import CommissionSettings from './CommissionSettings.vue'
import SecuritySettingsPanel from './SecuritySettingsPanel.vue'
import DetailModal from '@lottery/shared/DetailModal.vue'
import { vInlineValidation } from '@lottery/shared'
import { computed, ref, watch } from 'vue'
import { usePlatformI18n } from './i18n'
import { createPlatformApi, type PlatformSettings, type PlatformAccount } from './platform-api'

const props = defineProps<{ initial: PlatformSettings; locale: 'en' | 'zh-CN'; canManage: boolean; account: PlatformAccount; brandId: string; brandStatus: string }>()
const emit = defineEmits<{ saved: [settings: PlatformSettings]; failure: [cause: unknown] }>()
const tab = defineModel<'general' | 'auth' | 'commission'>('tab', { default: 'general' })
const securityVisited = ref(false)
const commissionVisited = ref(false)
const securityLocked = ref(false)
watch(tab, value => {
  if (value === 'auth') securityVisited.value = true
  if (value === 'commission') commissionVisited.value = true
}, { immediate: true })
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

useOperationToast(error, value => displayPublicError(value), 'error');
</script>

<template>
  <div class="platform-settings-page">
    <div class="console-page-heading page-heading compact-heading"><div><h1 class="console-page-title">{{ copy.title }}</h1></div></div>
    <div class="console-tabs platform-settings-tabs">
      <button v-for="item in ['general', 'auth', 'commission'] as const" :key="item" type="button" :class="{ selected: tab === item }" :aria-pressed="tab === item" :disabled="securityLocked || busy" @click="tab = item">{{ t(`settings.tab.${item}`) }}</button>
    </div>
    <section v-show="tab === 'general'" class="panel">
      <div class="panel-heading"><button type="button" class="button button-secondary" :disabled="!canManage" @click="startEdit">{{ t('common.editSettings') }}</button></div>
      <dl class="confirm-list">
        <div><dt>{{ copy.displayName }}</dt><dd>{{ current.display_name }}</dd></div>
        <div><dt>{{ copy.logoText }}</dt><dd>{{ current.logo_text }}</dd></div>
        <div><dt>{{ copy.logoUrl }}</dt><dd>{{ current.logo_url || '—' }}</dd></div>
        <div><dt>{{ copy.timezone }}</dt><dd>{{ current.timezone }}</dd></div>
        <div><dt>{{ copy.dateFormat }}</dt><dd>{{ current.date_format === 'YYYY-MM-DD' ? copy.dateYMD : current.date_format === 'DD/MM/YYYY' ? copy.dateDMY : copy.dateMDY }}</dd></div>
        <div><dt>{{ copy.timeFormat }}</dt><dd>{{ current.time_format === 'HH:mm:ss' ? copy.time24 : copy.time12 }}</dd></div>
      </dl>
    </section>
    <SecuritySettingsPanel v-if="securityVisited" v-show="tab === 'auth'" :account="account" @locked="securityLocked = $event" @failure="emit('failure', $event)" />
    <CommissionSettings v-if="commissionVisited" v-show="tab === 'commission'" :timezone="current.timezone" :can-manage="canManage" @failure="emit('failure', $event)" />
    <DetailModal v-if="editorOpen" :title="copy.title" :close-label="t('platform.copy.27b2fd29')" :close-disabled="busy" @close="!busy && (editorOpen = false)">
    <form v-write-guard="busy" novalidate v-inline-validation class="brand-form platform-settings-form ui-form" @submit.prevent="withGlobalLoading(save)">
      <div class="form-row"><label>{{ copy.displayName }}<input v-model.trim="form.display_name" maxlength="120" required /></label><label>{{ copy.logoText }}<input v-model.trim="form.logo_text" maxlength="8" required /></label></div>
      <label>{{ copy.logoUrl }}<input v-model.trim="form.logo_url" maxlength="512" /></label>
      <div class="form-row"><label>{{ copy.timezone }}<TimeZoneSelect v-model="form.timezone" :locale="locale" required /></label><label>{{ copy.dateFormat }}<SelectControl v-model="form.date_format" :label="String(copy.dateFormat)" :options="[{ value: 'YYYY-MM-DD', label: (String(copy.dateYMD)).trim() }, { value: 'DD/MM/YYYY', label: (String(copy.dateDMY)).trim() }, { value: 'MM/DD/YYYY', label: (String(copy.dateMDY)).trim() }]" /></label></div>
      <div class="form-row"><label>{{ copy.timeFormat }}<SelectControl v-model="form.time_format" :label="String(copy.timeFormat)" :options="[{ value: 'HH:mm:ss', label: (String(copy.time24)).trim() }, { value: 'hh:mm:ss A', label: (String(copy.time12)).trim() }]" /></label></div>

      <div class="form-actions ui-form-actions"><button type="submit" class="button button-primary" :disabled="busy">{{ busy ? copy.saving : copy.save }}</button></div>
    </form>
    </DetailModal>
  </div>
</template>

<style scoped>
.platform-settings-page { display: grid; gap: 12px; min-width: 0; }
.platform-settings-page > .page-heading, .platform-settings-page > .console-tabs { margin-bottom: 0; }
</style>
