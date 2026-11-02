<script setup lang="ts">
import SelectControl from '@lottery/shared/SelectControl.vue'
import { useOperationToast } from "@lottery/shared";
import { onMounted, ref, watch } from 'vue'
import { vWriteGuard, vInlineValidation, withGlobalLoading } from '@lottery/shared'
import { createPlatformApi, PlatformApiError, type PlatformCommissionPolicy } from './platform-api'
import { usePlatformI18n } from './i18n'
const props = defineProps<{ timezone: string; canManage: boolean }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const { t, message, errorMessage, displayError } = usePlatformI18n()
const api = createPlatformApi()
const current = ref<PlatformCommissionPolicy | null>(null)
const enabled = ref(false)
const payoutsEnabled = ref(false)
const cycle = ref<'weekly' | 'monthly'>('weekly')
const weekday = ref(1)
const monthDay = ref(1)
const mode = ref<'manual' | 'automatic'>('manual')
const busy = ref(false)
const error = ref<string | import('@lottery/shared').LocalizedMessage>('')
const notice = ref<string | import('@lottery/shared').LocalizedMessage>('')
const conflict = ref(false)
const pending = ref<{ body: PlatformCommissionPolicy; key: string } | null>(null)
async function load() {
 busy.value = true; error.value = ''
 try {
  const item = await api.commissionSettings(); current.value = item; payoutsEnabled.value = item.payouts_enabled; enabled.value = item.config.enabled; mode.value = item.config.payout_mode
  if(item.config.calendar) { cycle.value = item.config.calendar.cycle; weekday.value = item.config.calendar.weekday ?? 1; monthDay.value = item.config.calendar.month_day ?? 1 }
  conflict.value = false
 } catch(cause) { error.value = errorMessage(cause); emit('failure', cause) }
 finally { busy.value = false }
}
async function save() {
 if(!props.canManage || busy.value || !current.value) return
 if(!pending.value) pending.value = {key: crypto.randomUUID(),body: {version: current.value.version,payouts_enabled: payoutsEnabled.value,config: {enabled: enabled.value,payout_mode: mode.value,calendar: {timezone: props.timezone,cycle: cycle.value,boundary_time:'00:00:00',weekday: cycle.value === 'weekly' ? weekday.value : null,month_day: cycle.value === 'monthly' ? monthDay.value : null,short_month: cycle.value === 'monthly' ? 'last_day' : ''}}}}
 busy.value = true; error.value = ''; notice.value = ''
 try {
  const item = await api.updateCommissionSettings(pending.value.body,pending.value.key); current.value = item; pending.value = null
  notice.value = message(item.payouts_enabled ? 'commission.settings.saved' : 'commission.settings.disabled')
 } catch(cause) {
  if(cause instanceof PlatformApiError && cause.status >= 400 && cause.status < 500) {pending.value = null;conflict.value = cause.status === 409}
  error.value = errorMessage(cause); emit('failure',cause)
 } finally {busy.value = false}
}
onMounted(load)
watch(() => props.timezone, () => { if(!pending.value) void load() })
useOperationToast(notice, value => displayError(value), 'success');

useOperationToast(error, value => displayError(value), 'error');
</script>
<template>
 <section class="panel console-panel console-stack">

  <form v-if="current" v-write-guard="busy" novalidate v-inline-validation class="brand-form ui-form" @submit.prevent="withGlobalLoading(save)">
   <fieldset :disabled="!canManage || busy || Boolean(pending) || conflict" class="commission-fields">
    <label>{{ t('commission.settings.cycle') }}<SelectControl v-model="cycle" :label="String(t('commission.settings.cycle'))" :options="[{ value: 'weekly', label: (String(t('commission.settings.weekly'))).trim() }, { value: 'monthly', label: (String(t('commission.settings.monthly'))).trim() }]" /></label>
    <label v-if="cycle === 'weekly'">{{ t('commission.settings.weekday') }}<SelectControl v-model="weekday" :label="String(t('commission.settings.weekday'))" :options="(t('commission.settings.days').split(',')).map((day, index) => ({ value: index, label: (String(day)).trim() }))" /></label>
    <label v-else>{{ t('commission.settings.monthDay') }}<SelectControl v-model="monthDay" :label="String(t('commission.settings.monthDay'))" :options="Array.from({ length: 31 }, (_, index) => index + 1).map((day) => ({ value: day, label: (String(day)).trim() }))" /></label>
    <label class="commission-enabled"><input v-model="enabled" type="checkbox" />{{ t('commission.settings.enabled') }}</label>
    <label class="commission-enabled"><input v-model="payoutsEnabled" type="checkbox" />{{ t('commission.settings.payoutsEnabled') }}</label>
    <label>{{ t('commission.settings.mode') }}<SelectControl v-model="mode" :label="String(t('commission.settings.mode'))" :options="[{ value: 'manual', label: (String(t('commission.settings.manual'))).trim() }, { value: 'automatic', label: (String(t('commission.settings.automatic'))).trim() }]" /></label>
   </fieldset>
   <div class="form-actions ui-form-actions"><button v-if="canManage && !conflict" type="submit" class="button button-primary" :disabled="busy">{{ t(pending ? 'commission.settings.retry' : 'commission.settings.save') }}</button><button v-if="conflict" type="button" class="button" :disabled="busy" @click="withGlobalLoading(load)">{{ t('commission.settings.reload') }}</button></div>
  </form>
  <button v-else type="button" class="button" :disabled="busy" @click="withGlobalLoading(load)">{{ t('commission.settings.reload') }}</button>
 </section>
</template>
<style scoped>
.commission-fields { margin: 0; padding: 0; border: 0; display: grid; gap: 12px; min-width: 0; grid-template-columns: repeat(2, minmax(0, 1fr)); }
.commission-fields label { min-width: 0; }
.commission-fields .commission-enabled { display: flex; flex-direction: row; align-items: center; gap: 8px; grid-column: 1 / -1; min-height: 44px; }
.commission-enabled input { flex: 0 0 18px; width: 18px; height: 18px; margin: 0; padding: 0; }
@media (max-width: 640px) { .commission-fields { grid-template-columns: minmax(0, 1fr); } }
</style>
