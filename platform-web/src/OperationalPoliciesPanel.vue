<script setup lang="ts">
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

const props = defineProps<{ brandId: string; locale: 'en' | 'zh-CN' }>()
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
const error = ref('')
let generation = 0

const copy = () => props.locale === 'en' ? {
  title: 'Operational policies', type: 'Policy type', betting: 'Betting', settlement: 'Settlement', agents: 'Agents', compliance: 'Compliance', payment: 'Commission payment', correction: 'Commission correction',
  loading: 'Loading…', refresh: 'Refresh', empty: 'No policy loaded.', brand: 'Brand', version: 'Version', updated: 'Updated', audit: 'Audit log', enabled: 'Enabled', disabled: 'Disabled', yes: 'Yes', no: 'No', unlimited: 'Unlimited',
  minimum: 'Minimum bet points', maximum: 'Maximum bet points', periodMax: 'Maximum period points', userMax: 'Maximum user period points', cancel: 'User cancellation allowed', betNote: 'The brand policy is the default; a game override takes precedence.',
  mode: 'Settlement mode', notConfigured: 'Not configured', automatic: 'Automatic', manual: 'Manual',
  depth: 'Maximum agent depth', ratioCap: 'Ratio cap (original 0–1 value)', agencyMode: 'Commission basis', loss: 'Loss', turnover: 'Turnover', cycle: 'Agency policy cycle', weekly: 'Weekly', monthly: 'Monthly', lossNote: 'Loss means the user’s valid settled bet loss; cancelled, abnormal, invalid and tie bets are excluded.', cycleNote: 'This is the cycle saved in the agency policy. The financial commission calendar comes from a separate commission policy.',
  age: 'Age check enabled', minimumAge: 'Minimum age', region: 'Country/region check enabled', countries: 'Allowed country codes', identity: 'Identity check enabled',
  accountRisk: 'Account risk check (device/IP/account linkage)', bettingRisk: 'Betting risk check (abnormal bets/bonus abuse/arbitrage/multiple accounts)', exclusion: 'Blacklist/self-exclusion check', responsibleGambling: 'Responsible gambling/cooling check',
  adapterNote: 'Real verification and risk adapters are not connected. These settings are not proof of KYC, risk review, exclusion, or responsible-gambling checks, and this view does not run live checks.',
  gateNote: 'This read-only switch does not enable processing or pay funds.',
} : {
  title: '运营规则', type: '规则类型', betting: '投注', settlement: '结算', agents: '代理', compliance: '合规', payment: '佣金派发', correction: '佣金更正',
  loading: '加载中…', refresh: '刷新', empty: '尚未加载规则。', brand: '品牌', version: '版本', updated: '更新时间', audit: '审计日志', enabled: '启用', disabled: '停用', yes: '是', no: '否', unlimited: '不限',
  minimum: '单注最低积分', maximum: '单注最高积分', periodMax: '期次积分上限', userMax: '单用户期次积分上限', cancel: '允许用户取消', betNote: '品牌规则为默认值；单独设置的彩种规则优先。',
  mode: '结算模式', notConfigured: '未配置', automatic: '自动', manual: '人工',
  depth: '代理最大层级', ratioCap: '比例上限（原始0–1值）', agencyMode: '佣金计算依据', loss: '用户亏损', turnover: '有效投注额', cycle: '代理规则周期', weekly: '每周', monthly: '每月', lossNote: '用户输的有效注单；取消、异常、无效和平局注单不计入。', cycleNote: '这是代理规则中保存的周期。财务佣金日历来自独立的佣金规则。',
  age: '启用年龄检查', minimumAge: '最低年龄', region: '启用国家/地区检查', countries: '允许的国家代码', identity: '启用身份检查',
  accountRisk: '账户风险检查（设备/IP/账户关联）', bettingRisk: '投注风险检查（异常投注/奖金滥用/套利/多账户）', exclusion: '黑名单/自我排除检查', responsibleGambling: '负责任博彩/冷静期检查',
  adapterNote: '未连接真实验证或风险适配器。这些设置不证明已完成KYC、风险审查、排除或负责任博彩检查，本页面也不会执行实时检查。',
  gateNote: '此只读开关不会启用处理或派发资金。',
}
const yesNo = (value: boolean) => value ? copy().yes : copy().no
const state = (value: boolean) => value ? copy().enabled : copy().disabled
const label = (kind: Kind) => copy()[kind]

function report(cause: unknown) {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  else error.value = cause instanceof Error ? cause.message : String(cause)
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

watch(() => [props.brandId, selected.value], () => { void refresh() }, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { ++generation })
</script>

<template>
  <div data-testid="platform-operational-policies">
    <div class="page-heading"><div><h1>{{ copy().title }}</h1></div></div>
    <div class="brand-picker"><label>{{ copy().type }}
      <select v-model="selected" :aria-label="copy().type" data-testid="platform-operational-policy-select">
        <option value="betting">{{ copy().betting }}</option><option value="settlement">{{ copy().settlement }}</option><option value="agents">{{ copy().agents }}</option><option value="compliance">{{ copy().compliance }}</option><option value="payment">{{ copy().payment }}</option><option value="correction">{{ copy().correction }}</option>
      </select>
    </label></div>
    <p v-if="error" class="message error global-message" role="alert">{{ error }}</p>
    <p v-if="loading" class="loading-line" aria-live="polite">{{ copy().loading }}</p>
    <section class="panel">
      <div class="panel-heading"><div><h2>{{ label(selected) }}</h2><p>{{ copy().brand }}: <span class="mono">{{ props.brandId || '—' }}</span></p></div><button class="secondary" :disabled="loading || !props.brandId" @click="refresh">{{ copy().refresh }}</button></div>
      <section v-if="policy" class="reward-detail" data-testid="platform-operational-policy-detail">
        <dl v-if="policy.kind === 'betting'" class="confirm-list reward-fields">
          <div><dt>{{ copy().brand }}</dt><dd class="mono">{{ policy.value.brand_id }}</dd></div><div><dt>{{ copy().version }}</dt><dd>{{ policy.value.version }}</dd></div><div><dt>{{ copy().updated }}</dt><dd>{{ policy.value.updated_at }}</dd></div>
          <div><dt>{{ copy().minimum }}</dt><dd>{{ policy.value.config.min_bet_points }}</dd></div><div><dt>{{ copy().maximum }}</dt><dd>{{ policy.value.config.max_bet_points ?? copy().unlimited }}</dd></div><div><dt>{{ copy().periodMax }}</dt><dd>{{ policy.value.config.max_period_points ?? copy().unlimited }}</dd></div><div><dt>{{ copy().userMax }}</dt><dd>{{ policy.value.config.max_user_period_points ?? copy().unlimited }}</dd></div><div><dt>{{ copy().cancel }}</dt><dd>{{ yesNo(policy.value.config.user_cancel_allowed) }}</dd></div>
        </dl>
        <dl v-else-if="policy.kind === 'settlement'" class="confirm-list reward-fields">
          <div><dt>{{ copy().brand }}</dt><dd class="mono">{{ policy.value.brand_id }}</dd></div><div><dt>{{ copy().version }}</dt><dd>{{ policy.value.version }}</dd></div><div><dt>{{ copy().updated }}</dt><dd>{{ policy.value.updated_at }}</dd></div><div><dt>{{ copy().mode }}</dt><dd>{{ policy.value.mode === null ? copy().notConfigured : policy.value.mode === 'automatic' ? copy().automatic : copy().manual }}</dd></div><div v-if="policy.value.audit_log_id"><dt>{{ copy().audit }}</dt><dd class="mono">{{ policy.value.audit_log_id }}</dd></div>
        </dl>
        <dl v-else-if="policy.kind === 'agents'" class="confirm-list reward-fields">
          <div><dt>{{ copy().brand }}</dt><dd class="mono">{{ policy.value.brand_id }}</dd></div><div><dt>{{ copy().version }}</dt><dd>{{ policy.value.version }}</dd></div><div><dt>{{ copy().updated }}</dt><dd>{{ policy.value.updated_at }}</dd></div><div><dt>{{ copy().enabled }}</dt><dd>{{ state(policy.value.config.enabled) }}</dd></div><div><dt>{{ copy().depth }}</dt><dd>{{ policy.value.config.max_depth }}</dd></div><div><dt>{{ copy().ratioCap }}</dt><dd>{{ policy.value.config.ratio_cap }}</dd></div><div><dt>{{ copy().agencyMode }}</dt><dd>{{ policy.value.config.mode === 'loss' ? copy().loss : copy().turnover }}</dd></div><div><dt>{{ copy().cycle }}</dt><dd>{{ policy.value.config.cycle === 'weekly' ? copy().weekly : copy().monthly }}</dd></div>
        </dl>
        <template v-else-if="policy.kind === 'compliance'">
          <dl class="confirm-list reward-fields">
            <div><dt>{{ copy().brand }}</dt><dd class="mono">{{ policy.value.brand_id }}</dd></div><div><dt>{{ copy().version }}</dt><dd>{{ policy.value.version }}</dd></div><div><dt>{{ copy().updated }}</dt><dd>{{ policy.value.updated_at }}</dd></div>
            <div><dt>{{ copy().age }}</dt><dd>{{ state(policy.value.config.age_enabled) }}</dd></div><div><dt>{{ copy().minimumAge }}</dt><dd>{{ policy.value.config.minimum_age ?? copy().notConfigured }}</dd></div><div><dt>{{ copy().region }}</dt><dd>{{ state(policy.value.config.region_enabled) }}</dd></div><div><dt>{{ copy().countries }}</dt><dd>{{ policy.value.config.allowed_countries.join(', ') || copy().notConfigured }}</dd></div><div><dt>{{ copy().identity }}</dt><dd>{{ state(policy.value.config.identity_enabled) }}</dd></div>
            <div><dt>{{ copy().accountRisk }}</dt><dd>{{ state(policy.value.config.account_risk_enabled) }}</dd></div><div><dt>{{ copy().bettingRisk }}</dt><dd>{{ state(policy.value.config.betting_risk_enabled) }}</dd></div><div><dt>{{ copy().exclusion }}</dt><dd>{{ state(policy.value.config.exclusion_enabled) }}</dd></div><div><dt>{{ copy().responsibleGambling }}</dt><dd>{{ state(policy.value.config.responsible_gambling_enabled) }}</dd></div>
            <div v-if="policy.value.audit_log_id"><dt>{{ copy().audit }}</dt><dd class="mono">{{ policy.value.audit_log_id }}</dd></div>
          </dl><p class="wallet-pagination">{{ copy().adapterNote }}</p>
        </template>
        <template v-else-if="policy.kind === 'payment' || policy.kind === 'correction'">
          <dl class="confirm-list reward-fields">
            <div><dt>{{ copy().brand }}</dt><dd class="mono">{{ policy.value.brand_id }}</dd></div><div><dt>{{ copy().version }}</dt><dd>{{ policy.value.version }}</dd></div><div><dt>{{ copy().updated }}</dt><dd>{{ policy.value.updated_at }}</dd></div><div><dt>{{ copy().enabled }}</dt><dd>{{ state(policy.value.enabled) }}</dd></div><div><dt>{{ copy().audit }}</dt><dd class="mono">{{ policy.value.audit_log_id || '—' }}</dd></div>
          </dl><p class="wallet-pagination">{{ copy().gateNote }}</p>
        </template>
        <p v-if="policy.kind === 'betting'" class="wallet-pagination">{{ copy().betNote }}</p>
        <p v-if="policy.kind === 'agents'" class="wallet-pagination">{{ copy().lossNote }} {{ copy().cycleNote }}</p>
      </section>
      <p v-else-if="!loading && !error" class="empty-state">{{ copy().empty }}</p>
    </section>
  </div>
</template>

<style scoped>
.panel-heading button { flex-shrink: 0; }
</style>
