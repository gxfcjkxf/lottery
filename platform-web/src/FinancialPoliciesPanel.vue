<script setup lang="ts">
import { usePlatformI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { usePlatformI18n } from "./i18n"
const ui = usePlatformI18n().t
import { onBeforeUnmount, ref, watch } from 'vue'
import { PlatformApiError } from './platform-api'
import { createPlatformFinancialPoliciesApi, type PointPolicy, type WithdrawalPolicy, type CommissionPolicy } from './financial-policies-api'
import { formatPlatformDateTime, type PlatformDisplaySettings } from './platform-format'

const props = defineProps<{ brandId: string; locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformFinancialPoliciesApi()
type PolicyType = 'points' | 'withdrawal' | 'commission'
const selected = ref<PolicyType>('points')
const policy = ref<PointPolicy | WithdrawalPolicy | CommissionPolicy | null>(null)
const loading = ref(false)
const error = ref<string | import("@lottery/shared").LocalizedMessage>("")
let generation = 0

const copy = () => props.locale === 'en' ? {
  title: ui('platform.copy.f731cce4'), policyType: ui('platform.copy.ac262406'), points: ui('platform.copy.a1f74153'), withdrawals: ui('platform.copy.c2a6a429'), commission: ui('platform.copy.648ed708'), rechargeSource: ui('platform.copy.3420a04d'), winningSource: ui('platform.copy.b8a2ff9b'), giftSource: ui('platform.copy.c8d77732'), commissionSource: ui('platform.copy.648ed708'), refresh: ui('platform.copy.256fe8b3'), empty: ui('platform.copy.aad579db'),
  brand: ui('platform.copy.d5b68c0f'), version: ui('platform.copy.59405491'), updated: ui('platform.copy.b8b9d3d6'), revision: ui('platform.copy.6a185a4f'), audit: ui('platform.copy.8262ff2c'), unlimited: ui('platform.copy.2b8630e5'), enabled: ui('platform.copy.997d5f45'), disabled: ui('platform.copy.1b47d863'),
  balance: ui('platform.copy.c530f4d5'), recharge: ui('platform.copy.7752d1b9'), adjustment: ui('platform.copy.b36178d5'), minimum: ui('platform.copy.619f1358'), maximum: ui('platform.copy.c47b156c'), sources: ui('platform.copy.12b9d6bb'), review: ui('platform.copy.97fb0fe9'), manual: ui('platform.copy.05b370d4'), automatic: ui('platform.copy.b080e80e'), turnover: ui('platform.copy.51f363cc'), defaultNote: ui('platform.copy.082fdfea'),
  payout: ui('platform.copy.a296bc29'), calendar: ui('platform.copy.0bb0776b'), notConfigured: ui('platform.copy.41ea503b'), cycle: ui('platform.copy.f6f6ceea'), weekly: ui('platform.copy.8916e895'), monthly: ui('platform.copy.dfecc78d'), timezone: ui('platform.copy.27570df6'), boundary: ui('platform.copy.1de328f6'), weekday: ui('platform.copy.54ce5a7d'), monthday: ui('platform.copy.b1050932'), shortMonth: ui('platform.copy.5e80124f'), lastDay: ui('platform.copy.b4999ff7'), skip: ui('platform.copy.ca61d3ea'),
  weekdays: ['platform.weekday.sunday', 'platform.weekday.monday', 'platform.weekday.tuesday', 'platform.weekday.wednesday', 'platform.weekday.thursday', 'platform.weekday.friday', 'platform.weekday.saturday'],
} : {
  title: ui('platform.copy.f731cce4'), policyType: ui('platform.copy.ac262406'), points: ui('platform.copy.a1f74153'), withdrawals: ui('platform.copy.c2a6a429'), commission: ui('platform.copy.648ed708'), rechargeSource: ui('platform.copy.3420a04d'), winningSource: ui('platform.copy.b8a2ff9b'), giftSource: ui('platform.copy.c8d77732'), commissionSource: ui('platform.copy.648ed708'), refresh: ui('platform.copy.256fe8b3'), empty: ui('platform.copy.aad579db'),
  brand: ui('platform.copy.d5b68c0f'), version: ui('platform.copy.59405491'), updated: ui('platform.copy.b8b9d3d6'), revision: ui('platform.copy.6a185a4f'), audit: ui('platform.copy.8262ff2c'), unlimited: ui('platform.copy.2b8630e5'), enabled: ui('platform.copy.997d5f45'), disabled: ui('platform.copy.1b47d863'),
  balance: ui('platform.copy.c530f4d5'), recharge: ui('platform.copy.7752d1b9'), adjustment: ui('platform.copy.b36178d5'), minimum: ui('platform.copy.619f1358'), maximum: ui('platform.copy.c47b156c'), sources: ui('platform.copy.12b9d6bb'), review: ui('platform.copy.97fb0fe9'), manual: ui('platform.copy.05b370d4'), automatic: ui('platform.copy.b080e80e'), turnover: ui('platform.copy.51f363cc'), defaultNote: ui('platform.copy.082fdfea'),
  payout: ui('platform.copy.a296bc29'), calendar: ui('platform.copy.0bb0776b'), notConfigured: ui('platform.copy.41ea503b'), cycle: ui('platform.copy.f6f6ceea'), weekly: ui('platform.copy.8916e895'), monthly: ui('platform.copy.dfecc78d'), timezone: ui('platform.copy.27570df6'), boundary: ui('platform.copy.1de328f6'), weekday: ui('platform.copy.54ce5a7d'), monthday: ui('platform.copy.b1050932'), shortMonth: ui('platform.copy.5e80124f'), lastDay: ui('platform.copy.b4999ff7'), skip: ui('platform.copy.ca61d3ea'),
  weekdays: ['platform.weekday.sunday', 'platform.weekday.monday', 'platform.weekday.tuesday', 'platform.weekday.wednesday', 'platform.weekday.thursday', 'platform.weekday.friday', 'platform.weekday.saturday'],
}
const enabledLabel = (value: boolean) => value ? copy().enabled : copy().disabled
const sourceLabel = (source: string) => source === 'recharge' ? copy().rechargeSource : source === 'winning' ? copy().winningSource : source === 'gift' ? copy().giftSource : copy().commissionSource

function report(cause: unknown) {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  else error.value = cause instanceof Error ? publicErrorMessage(cause) : publicErrorMessage(cause)
}

async function refresh() {
  const request = ++generation
  const brandId = props.brandId
  const kind = selected.value
  policy.value = null
  error.value = ''
  if (!brandId) { loading.value = false; return }
  loading.value = true
  try {
    const result = kind === 'points' ? await api.points(brandId)
      : kind === 'withdrawal' ? await api.withdrawal(brandId)
        : await api.commission(brandId)
    if (request === generation && props.brandId === brandId && selected.value === kind) policy.value = result
  } catch (cause) {
    if (request === generation && props.brandId === brandId && selected.value === kind) report(cause)
  } finally {
    if (request === generation) loading.value = false
  }
}

watch(() => [props.brandId, selected.value], () => { void withGlobalLoading(refresh) }, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { ++generation })
</script>

<template>
  <div data-testid="platform-financial-policies">
    <div class="brand-picker policy-picker"><label>{{ copy().policyType }}
      <select v-model="selected" :aria-label="copy().policyType" data-testid="platform-financial-policy-select">
        <option value="points">{{ copy().points }}</option><option value="withdrawal">{{ copy().withdrawals }}</option><option value="commission">{{ copy().commission }}</option>
      </select>
    </label><button v-refresh-button="copy().refresh" class="button button-secondary" :disabled="loading || !props.brandId" @click="withGlobalLoading(refresh)">{{ copy().refresh }}</button></div>
    <p v-if="error" class="message error global-message" role="alert">{{ displayPublicError(error) }}</p>
    <section class="panel">
      <section v-if="policy" class="reward-detail" data-testid="platform-financial-policy-detail">
        <div class="table-wrap">
          <table v-if="selected === 'points'" class="policy-table">
            <tbody>
              <tr></tr>
              <tr><th scope="row">{{ copy().balance }}</th><td>{{ (policy as PointPolicy).max_balance_points == null ? copy().unlimited : (policy as PointPolicy).max_balance_points }}</td></tr>
              <tr><th scope="row">{{ copy().recharge }}</th><td>{{ (policy as PointPolicy).max_recharge_points == null ? copy().unlimited : (policy as PointPolicy).max_recharge_points }}</td></tr>
              <tr><th scope="row">{{ copy().adjustment }}</th><td>{{ (policy as PointPolicy).max_adjustment_points == null ? copy().unlimited : (policy as PointPolicy).max_adjustment_points }}</td></tr>
            </tbody>
          </table>
          <table v-else-if="selected === 'withdrawal'" class="policy-table">
            <tbody>
              <tr></tr>
              <tr><th scope="row">{{ copy().updated }}</th><td>{{ formatPlatformDateTime((policy as WithdrawalPolicy).updated_at, props.displaySettings, props.locale) }}</td></tr>
              <tr><th scope="row">{{ copy().enabled }}</th><td>{{ enabledLabel((policy as WithdrawalPolicy).config.enabled) }}</td></tr>
              <tr><th scope="row">{{ copy().minimum }}</th><td>{{ (policy as WithdrawalPolicy).config.min_points }}</td></tr>
              <tr><th scope="row">{{ copy().maximum }}</th><td>{{ (policy as WithdrawalPolicy).config.max_points == null ? copy().unlimited : (policy as WithdrawalPolicy).config.max_points }}</td></tr>
              <tr><th scope="row">{{ copy().sources }}</th><td>{{ (policy as WithdrawalPolicy).config.allowed_sources.map(sourceLabel).join(', ') }}</td></tr>
              <tr><th scope="row">{{ copy().review }}</th><td>{{ (policy as WithdrawalPolicy).config.review_mode === 'manual' ? copy().manual : copy().automatic }}</td></tr>
              <tr><th scope="row">{{ copy().turnover }}</th><td>{{ (policy as WithdrawalPolicy).config.turnover_multiple }}</td></tr>
            </tbody>
          </table>
          <table v-else class="policy-table">
            <tbody>
              <tr></tr>
              <tr><th scope="row">{{ copy().updated }}</th><td>{{ formatPlatformDateTime((policy as CommissionPolicy).updated_at, props.displaySettings, props.locale) }}</td></tr>
              <tr><th scope="row">{{ copy().enabled }}</th><td>{{ enabledLabel((policy as CommissionPolicy).config.enabled) }}</td></tr>
              <tr><th scope="row">{{ copy().payout }}</th><td>{{ (policy as CommissionPolicy).config.payout_mode === 'manual' ? copy().manual : copy().automatic }}</td></tr>
              <tr><th scope="row">{{ copy().calendar }}</th><td>{{ (policy as CommissionPolicy).config.calendar == null ? copy().notConfigured : ((policy as CommissionPolicy).config.calendar!.cycle === 'weekly' ? copy().weekly : copy().monthly) }}</td></tr>
            </tbody>
          </table>
        </div>
        <template v-if="selected === 'commission' && (policy as CommissionPolicy).config.calendar">
          <div class="table-wrap">
            <table class="policy-table">
              <tbody>
                <tr><th scope="row">{{ copy().cycle }}</th><td>{{ (policy as CommissionPolicy).config.calendar!.cycle === 'weekly' ? copy().weekly : copy().monthly }}</td></tr>
                <tr><th scope="row">{{ copy().timezone }}</th><td>{{ (policy as CommissionPolicy).config.calendar!.timezone }}</td></tr>
                <tr><th scope="row">{{ copy().boundary }}</th><td>{{ (policy as CommissionPolicy).config.calendar!.boundary_time }}</td></tr>
                <tr v-if="(policy as CommissionPolicy).config.calendar!.cycle === 'weekly'"><th scope="row">{{ copy().weekday }}</th><td>{{ ui(copy().weekdays[(policy as CommissionPolicy).config.calendar!.weekday!]) }}</td></tr>
                <template v-else>
                  <tr><th scope="row">{{ copy().monthday }}</th><td>{{ (policy as CommissionPolicy).config.calendar!.month_day }}</td></tr>
                  <tr><th scope="row">{{ copy().shortMonth }}</th><td>{{ (policy as CommissionPolicy).config.calendar!.short_month === 'last_day' ? copy().lastDay : copy().skip }}</td></tr>
                </template>
              </tbody>
            </table>
          </div>
        </template>
        <p v-if="selected === 'withdrawal'" class="wallet-pagination">{{ copy().defaultNote }}</p>
      </section>
      <p v-else-if="!loading && !error" class="empty-state">{{ copy().empty }}</p>
    </section>
  </div>
</template>
