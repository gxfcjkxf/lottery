<script setup lang="ts">
import { themeIds, themeCatalog, applyTheme } from "@lottery/shared/theme"
import CashConsole from "@lottery/shared/CashConsole.vue"
import SelectControl from '@lottery/shared/SelectControl.vue'
import { useOperationToast, vInlineValidation, vWriteGuard, withGlobalLoading } from '@lottery/shared'
import TimeZoneSelect from '@lottery/shared/TimeZoneSelect.vue'
import CommissionSettings from './CommissionSettings.vue'
import SecuritySettingsPanel from './SecuritySettingsPanel.vue'
import { computed, ref, watch } from 'vue'
import { usePlatformI18n } from './i18n'
import { createPlatformApi, PlatformApiError, type PlatformSettings, type PlatformSettingsRequest, type PlatformAccount } from './platform-api'

const props = defineProps<{ initial: PlatformSettings; locale: 'en' | 'zh-CN'; canManage: boolean; account: PlatformAccount; brandId: string; brandStatus: string }>()
const emit = defineEmits<{ saved: [settings: PlatformSettings]; reloaded: [settings: PlatformSettings]; failure: [cause: unknown]; completed: [] }>()
const tab = defineModel<'general' | 'auth' | 'commission' | 'currency'>('tab', { default: 'general' })
const pending = defineModel<PlatformSettingsRequest | null>('pendingGeneral', { required: true })
const canViewGeneral = computed(() => props.account.platform_permissions.includes('platform_settings.view.platform'))
const securityVisited = ref(false)
const commissionVisited = ref(false)
const securityLocked = ref(false)
watch(tab, value => {
  if (value === 'auth') securityVisited.value = true
  if (value === 'commission') commissionVisited.value = true
}, { immediate: true })
const api = createPlatformApi()
const { t, errorMessage, displayError } = usePlatformI18n()
function toForm({ theme, version, display_name, logo_text, logo_url, timezone, date_format, time_format }: PlatformSettings) {
  return { theme, version, display_name, logo_text, logo_url, timezone, date_format, time_format }
}
const form = ref({ ...(pending.value?.body ?? toForm(props.initial)) })
watch(() => props.initial, value => {
  if (!pending.value) form.value = toForm(value)
})
const busy = ref(false)
const conflict = ref(false)
const canSave = computed(() => canViewGeneral.value && props.canManage)
const error = ref<ReturnType<typeof errorMessage> | null>(null)
const copy = computed(() => ({
  title: t('common.platformSettings'), displayName: t('common.platformDisplayName'), logoText: t('common.platformLogoText'), logoUrl: t('common.platformLogoUrl'), timezone: t('common.timeZone'), dateFormat: t('common.dateFormat'), timeFormat: t('common.timeFormat'), save: t('common.saveSettings'), saving: t('common.saving'), dateYMD: t('common.dateFormatYmd'), dateDMY: t('common.dateFormatDmy'), dateMDY: t('common.dateFormatMdy'), time24: t('common.timeFormat24'), time12: t('common.timeFormat12'),
}))

async function save() {
  if (!canSave.value || busy.value || conflict.value) return
  const request = pending.value ?? { body: { ...form.value }, key: crypto.randomUUID() }
  pending.value = request
  busy.value = true
  error.value = null
  try {
    const next = await api.updatePlatformSettings(request.body, request.key)
    pending.value = null
    applyTheme(next.theme)
    form.value = toForm(next)
    emit('saved', next)
  } catch (cause) {
    if (cause instanceof PlatformApiError && cause.status >= 400 && cause.status < 500) {
      pending.value = null
      conflict.value = cause.status === 409
    }
    error.value = errorMessage(cause)
    if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  } finally { busy.value = false }
}

async function reload() {
  if (busy.value || !canViewGeneral.value) return
  busy.value = true
  error.value = null
  try {
    const next = await api.platformSettings()
    applyTheme(next.theme)
    form.value = toForm(next)
    conflict.value = false
    emit('reloaded', next)
  } catch (cause) {
    error.value = errorMessage(cause, 'read')
    if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  } finally { busy.value = false }
}

useOperationToast(error, displayError, 'error')
</script>

<template>
  <div class="platform-settings-page">
    <div class="console-page-heading page-heading compact-heading"><div><h1 class="console-page-title">{{ copy.title }}</h1></div></div>
    <div class="console-tabs platform-settings-tabs">
      <button v-for="item in ['general', 'auth', 'commission', 'currency'] as const" :key="item" type="button" :class="{ selected: tab === item }" :aria-pressed="tab === item" :disabled="securityLocked || busy" @click="tab = item">{{ item === 'currency' ? t('cash.settings') : t(`settings.tab.${item}`) }}</button>
    </div>
    <p v-if="tab === 'general' && !canViewGeneral" role="status">{{ t('platform.error.PERMISSION_DENIED') }}</p>
    <section v-show="tab === 'general' && canViewGeneral" class="panel brand-settings-panel">
      <form v-write-guard="busy" novalidate v-inline-validation class="brand-form platform-settings-form ui-form" @submit.prevent="withGlobalLoading(save)">
        <fieldset class="platform-settings-fields" :disabled="!canSave || busy || Boolean(pending) || conflict">
          <label>{{ copy.displayName }}<input v-model.trim="form.display_name" maxlength="120" required /></label>
          <label>{{ copy.logoText }}<input v-model.trim="form.logo_text" maxlength="8" required /></label>
          <label>{{ themeCatalog[locale].theme }}<SelectControl v-model="form.theme" :label="themeCatalog[locale].theme" :options="themeIds.map(value => ({value, label: themeCatalog[locale][value]}))" /></label>
          <label>{{ copy.logoUrl }}<input v-model.trim="form.logo_url" maxlength="512" /></label>
          <label>{{ copy.timezone }}<TimeZoneSelect v-model="form.timezone" :locale="locale" required /></label>
          <label>{{ copy.dateFormat }}<SelectControl v-model="form.date_format" :label="String(copy.dateFormat)" :options="[{ value: 'YYYY-MM-DD', label: (String(copy.dateYMD)).trim() }, { value: 'DD/MM/YYYY', label: (String(copy.dateDMY)).trim() }, { value: 'MM/DD/YYYY', label: (String(copy.dateMDY)).trim() }]" /></label>
          <label>{{ copy.timeFormat }}<SelectControl v-model="form.time_format" :label="String(copy.timeFormat)" :options="[{ value: 'HH:mm:ss', label: (String(copy.time24)).trim() }, { value: 'hh:mm:ss A', label: (String(copy.time12)).trim() }]" /></label>
        </fieldset>
        <p v-if="pending && !busy" role="alert">{{ t('security.unknown') }}</p>
        <p v-if="conflict" role="alert">{{ t('security.conflict') }}</p>
        <div v-if="canSave" class="ui-form-actions">
          <button v-if="conflict" class="button button-secondary" type="button" :disabled="busy" @click="withGlobalLoading(reload)">{{ t('security.reread') }}</button>
          <button v-else type="submit" class="button button-primary" :disabled="busy">{{ busy ? copy.saving : pending ? t('security.retry') : copy.save }}</button>
        </div>
      </form>
    </section>
    <CashConsole v-if="tab === 'currency'" base-path="/api/v1/platform" platform mode="settings" :actor-id="account.id" :locale="locale" :permissions="account.platform_permissions" @saved="emit('completed')" @failure="emit('failure',$event)" />
    <SecuritySettingsPanel v-if="securityVisited" v-show="tab === 'auth'" :account="account" @locked="securityLocked = $event" @failure="emit('failure', $event)" />
    <CommissionSettings :can-view="account.platform_permissions.includes('platform_settings.view.platform')" v-if="commissionVisited" v-show="tab === 'commission'" :timezone="initial.timezone" :can-manage="canManage" @saved="emit('completed')" @failure="emit('failure', $event)" />
  </div>
</template>

<style scoped>
.platform-settings-fields { grid-template-columns: repeat(2, minmax(0, 1fr)); }
@media (max-width: 640px) { .platform-settings-fields { grid-template-columns: minmax(0, 1fr); } }
.platform-settings-page { display: grid; gap: 12px; min-width: 0; }
.platform-settings-page > .page-heading, .platform-settings-page > .console-tabs { margin-bottom: 0; }
</style>
