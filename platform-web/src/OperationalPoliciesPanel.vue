<script setup lang="ts">
import { agentRatioToPercentage } from '../../admin-web/src/agent-percentage'
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
import {
  createPlatformOperationalPoliciesApi,
  type AgentPolicy,
  type BettingPolicy,
  type CommissionCorrectionPolicy,
  type CommissionPaymentPolicy,
  type CompliancePolicy,
  type SettlementPolicy,
} from './operational-policies-api'
import { formatPlatformDateTime, type PlatformDisplaySettings } from './platform-format'

const props = defineProps<{ brandId: string; locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformOperationalPoliciesApi()
type Kind = 'betting' | 'settlement' | 'agents' | 'compliance' | 'payment' | 'correction'
const selected = ref<Kind>('betting')
type LoadedPolicy =
  | { kind: 'betting'; value: BettingPolicy }
  | { kind: 'settlement'; value: SettlementPolicy }
  | { kind: 'agents'; value: AgentPolicy }
  | { kind: 'compliance'; value: CompliancePolicy }
  | { kind: 'payment'; value: CommissionPaymentPolicy }
  | { kind: 'correction'; value: CommissionCorrectionPolicy }
const policy = ref<LoadedPolicy | null>(null)
const loading = ref(false)
const error = ref<string | import("@lottery/shared").LocalizedMessage>("")
let generation = 0

const copy = () => props.locale === 'en' ? {
  title: ui('platform.copy.c51df1b1'), type: ui('platform.copy.ac262406'), betting: ui('platform.copy.4e6493d5'), settlement: ui('platform.copy.72dadaa0'), agents: ui('platform.copy.3705c4b4'), compliance: ui('platform.copy.ce20e672'), payment: ui('platform.copy.adf1e523'), correction: ui('platform.copy.0f4f5125'),
  refresh: ui('platform.copy.256fe8b3'), empty: ui('platform.copy.aad579db'), brand: ui('platform.copy.d5b68c0f'), version: ui('platform.copy.59405491'), updated: ui('platform.copy.b8b9d3d6'), audit: ui('platform.copy.8262ff2c'), enabled: ui('platform.copy.997d5f45'), disabled: ui('platform.copy.1b47d863'), yes: ui('platform.copy.ab57ddad'), no: ui('platform.copy.502adcd8'), unlimited: ui('platform.copy.2b8630e5'),
  minimum: ui('platform.copy.6cdb9aff'), maximum: ui('platform.copy.dceb4467'), periodMax: ui('platform.copy.f4b44d50'), userMax: ui('platform.copy.648d5e15'), cancel: ui('platform.copy.bf7cc8b5'), betNote: ui('platform.copy.4bcfebc0'),
  mode: ui('platform.copy.255cdc95'), notConfigured: ui('platform.copy.41ea503b'), automatic: ui('platform.copy.b080e80e'), manual: ui('platform.copy.05b370d4'),
  depth: ui('platform.copy.050267c0'), lossRatioCap: ui('platform.agents.lossRatioCap'), turnoverRatioCap: ui('platform.agents.turnoverRatioCap'), agencyMode: ui('platform.copy.65bbf62d'), loss: ui('platform.copy.8dced0dd'), turnover: ui('platform.copy.3e5cc553'), cycle: ui('platform.copy.ebedb50d'), weekly: ui('platform.copy.8916e895'), monthly: ui('platform.copy.dfecc78d'), lossNote: ui('platform.copy.0d2a8b65'), cycleNote: ui('platform.copy.9a0ee069'),
  age: ui('platform.copy.c232a5df'), minimumAge: ui('platform.copy.b04f79f6'), region: ui('platform.copy.db9ec297'), countries: ui('platform.copy.3b532413'), identity: ui('platform.copy.e4f571e2'),
  accountRisk: ui('platform.copy.43d4a949'), bettingRisk: ui('platform.copy.7cb9deac'), exclusion: ui('platform.copy.6395b4d0'), responsibleGambling: ui('platform.copy.59d41851'),
  adapterNote: ui('platform.copy.c8877535'),
  gateNote: ui('platform.copy.5b9252ab'),
} : {
  title: ui('platform.copy.c51df1b1'), type: ui('platform.copy.ac262406'), betting: ui('platform.copy.4e6493d5'), settlement: ui('platform.copy.72dadaa0'), agents: ui('platform.copy.3705c4b4'), compliance: ui('platform.copy.ce20e672'), payment: ui('platform.copy.adf1e523'), correction: ui('platform.copy.0f4f5125'),
  refresh: ui('platform.copy.256fe8b3'), empty: ui('platform.copy.aad579db'), brand: ui('platform.copy.d5b68c0f'), version: ui('platform.copy.59405491'), updated: ui('platform.copy.b8b9d3d6'), audit: ui('platform.copy.8262ff2c'), enabled: ui('platform.copy.997d5f45'), disabled: ui('platform.copy.1b47d863'), yes: ui('platform.copy.ab57ddad'), no: ui('platform.copy.502adcd8'), unlimited: ui('platform.copy.2b8630e5'),
  minimum: ui('platform.copy.6cdb9aff'), maximum: ui('platform.copy.dceb4467'), periodMax: ui('platform.copy.f4b44d50'), userMax: ui('platform.copy.648d5e15'), cancel: ui('platform.copy.bf7cc8b5'), betNote: ui('platform.copy.4bcfebc0'),
  mode: ui('platform.copy.255cdc95'), notConfigured: ui('platform.copy.41ea503b'), automatic: ui('platform.copy.b080e80e'), manual: ui('platform.copy.05b370d4'),
  depth: ui('platform.copy.050267c0'), lossRatioCap: ui('platform.agents.lossRatioCap'), turnoverRatioCap: ui('platform.agents.turnoverRatioCap'), agencyMode: ui('platform.copy.65bbf62d'), loss: ui('platform.copy.8dced0dd'), turnover: ui('platform.copy.3e5cc553'), cycle: ui('platform.copy.ebedb50d'), weekly: ui('platform.copy.8916e895'), monthly: ui('platform.copy.dfecc78d'), lossNote: ui('platform.copy.0d2a8b65'), cycleNote: ui('platform.copy.9a0ee069'),
  age: ui('platform.copy.c232a5df'), minimumAge: ui('platform.copy.b04f79f6'), region: ui('platform.copy.db9ec297'), countries: ui('platform.copy.3b532413'), identity: ui('platform.copy.e4f571e2'),
  accountRisk: ui('platform.copy.43d4a949'), bettingRisk: ui('platform.copy.7cb9deac'), exclusion: ui('platform.copy.6395b4d0'), responsibleGambling: ui('platform.copy.59d41851'),
  adapterNote: ui('platform.copy.c8877535'),
  gateNote: ui('platform.copy.5b9252ab'),
}
const yesNo = (value: boolean) => value ? copy().yes : copy().no
const state = (value: boolean) => value ? copy().enabled : copy().disabled
const label = (kind: Kind) => copy()[kind]

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
    const result: LoadedPolicy = kind === 'betting' ? { kind, value: await api.betting(brandId) }
      : kind === 'settlement' ? { kind, value: await api.settlement(brandId) }
        : kind === 'agents' ? { kind, value: await api.agents(brandId) }
          : kind === 'compliance' ? { kind, value: await api.compliance(brandId) }
            : kind === 'payment' ? { kind, value: await api.payment(brandId) }
              : { kind, value: await api.correction(brandId) }
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
  <div data-testid="platform-operational-policies">
    <div class="brand-picker policy-picker"><label>{{ copy().type }}
      <select v-model="selected" :aria-label="copy().type" data-testid="platform-operational-policy-select">
        <option value="betting">{{ copy().betting }}</option><option value="settlement">{{ copy().settlement }}</option><option value="agents">{{ copy().agents }}</option><option value="compliance">{{ copy().compliance }}</option><option value="payment">{{ copy().payment }}</option><option value="correction">{{ copy().correction }}</option>
      </select>
    </label><button v-refresh-button="copy().refresh" class="button button-secondary" :disabled="loading || !props.brandId" @click="withGlobalLoading(refresh)">{{ copy().refresh }}</button></div>
    <p v-if="error" class="message error global-message" role="alert">{{ displayPublicError(error) }}</p>
    <section class="panel">
      <section v-if="policy" class="reward-detail" data-testid="platform-operational-policy-detail">
        <dl v-if="policy.kind === 'betting'" class="policy-fields">
          <div><dt>{{ copy().updated }}</dt><dd>{{ formatPlatformDateTime(policy.value.updated_at, props.displaySettings, props.locale) }}</dd></div>
          <div><dt>{{ copy().minimum }}</dt><dd>{{ policy.value.config.min_bet_points }}</dd></div><div><dt>{{ copy().maximum }}</dt><dd>{{ policy.value.config.max_bet_points ?? copy().unlimited }}</dd></div><div><dt>{{ copy().periodMax }}</dt><dd>{{ policy.value.config.max_period_points ?? copy().unlimited }}</dd></div><div><dt>{{ copy().userMax }}</dt><dd>{{ policy.value.config.max_user_period_points ?? copy().unlimited }}</dd></div><div><dt>{{ copy().cancel }}</dt><dd>{{ yesNo(policy.value.config.user_cancel_allowed) }}</dd></div>
        </dl>
        <dl v-else-if="policy.kind === 'settlement'" class="policy-fields">
          <div><dt>{{ copy().updated }}</dt><dd>{{ formatPlatformDateTime(policy.value.updated_at, props.displaySettings, props.locale) }}</dd></div><div><dt>{{ copy().mode }}</dt><dd>{{ policy.value.mode === null ? copy().notConfigured : policy.value.mode === 'automatic' ? copy().automatic : copy().manual }}</dd></div>
        </dl>
        <dl v-else-if="policy.kind === 'agents'" class="policy-fields">
            <div><dt>{{ copy().updated }}</dt><dd>{{ formatPlatformDateTime(policy.value.updated_at, props.displaySettings, props.locale) }}</dd></div><div><dt>{{ copy().enabled }}</dt><dd>{{ state(policy.value.config.enabled) }}</dd></div><div><dt>{{ copy().depth }}</dt><dd>{{ policy.value.config.max_depth }}</dd></div><div><dt>{{ copy().lossRatioCap }}</dt><dd>{{ agentRatioToPercentage(policy.value.config.loss_ratio_cap) }}%</dd></div><div><dt>{{ copy().turnoverRatioCap }}</dt><dd>{{ agentRatioToPercentage(policy.value.config.turnover_ratio_cap) }}%</dd></div><div><dt>{{ copy().agencyMode }}</dt><dd>{{ policy.value.config.mode === 'loss' ? copy().loss : copy().turnover }}</dd></div><div><dt>{{ copy().cycle }}</dt><dd>{{ policy.value.config.cycle === 'weekly' ? copy().weekly : copy().monthly }}</dd></div>
        </dl>
        <template v-else-if="policy.kind === 'compliance'">
          <dl class="policy-fields">
            <div><dt>{{ copy().updated }}</dt><dd>{{ formatPlatformDateTime(policy.value.updated_at, props.displaySettings, props.locale) }}</dd></div>
            <div><dt>{{ copy().age }}</dt><dd>{{ state(policy.value.config.age_enabled) }}</dd></div><div><dt>{{ copy().minimumAge }}</dt><dd>{{ policy.value.config.minimum_age ?? copy().notConfigured }}</dd></div><div><dt>{{ copy().region }}</dt><dd>{{ state(policy.value.config.region_enabled) }}</dd></div><div><dt>{{ copy().countries }}</dt><dd>{{ policy.value.config.allowed_countries.join(', ') || copy().notConfigured }}</dd></div><div><dt>{{ copy().identity }}</dt><dd>{{ state(policy.value.config.identity_enabled) }}</dd></div>
            <div><dt>{{ copy().accountRisk }}</dt><dd>{{ state(policy.value.config.account_risk_enabled) }}</dd></div><div><dt>{{ copy().bettingRisk }}</dt><dd>{{ state(policy.value.config.betting_risk_enabled) }}</dd></div><div><dt>{{ copy().exclusion }}</dt><dd>{{ state(policy.value.config.exclusion_enabled) }}</dd></div><div><dt>{{ copy().responsibleGambling }}</dt><dd>{{ state(policy.value.config.responsible_gambling_enabled) }}</dd></div>
          </dl><p class="wallet-pagination">{{ copy().adapterNote }}</p>
        </template>
        <template v-else-if="policy.kind === 'payment' || policy.kind === 'correction'">
          <dl class="policy-fields">
            <div><dt>{{ copy().updated }}</dt><dd>{{ formatPlatformDateTime(policy.value.updated_at, props.displaySettings, props.locale) }}</dd></div><div><dt>{{ copy().enabled }}</dt><dd>{{ state(policy.value.enabled) }}</dd></div>
          </dl><p class="wallet-pagination">{{ copy().gateNote }}</p>
        </template>
        <p v-if="policy.kind === 'betting'" class="wallet-pagination">{{ copy().betNote }}</p>
        <p v-if="policy.kind === 'agents'" class="wallet-pagination">{{ copy().lossNote }} {{ copy().cycleNote }}</p>
      </section>
      <p v-else-if="!loading && !error" class="empty-state">{{ copy().empty }}</p>
    </section>
  </div>
</template>
