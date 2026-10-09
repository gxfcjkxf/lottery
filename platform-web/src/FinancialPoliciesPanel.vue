<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { PlatformApiError } from './platform-api'
import { createPlatformFinancialPoliciesApi, type PointPolicy, type WithdrawalPolicy, type CommissionPolicy } from './financial-policies-api'

const props = defineProps<{ brandId: string; locale: 'en' | 'zh-CN' }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformFinancialPoliciesApi()
type PolicyType = 'points' | 'withdrawal' | 'commission'
const selected = ref<PolicyType>('points')
const policy = ref<PointPolicy | WithdrawalPolicy | CommissionPolicy | null>(null)
const loading = ref(false)
const error = ref('')
let generation = 0

const copy = () => props.locale === 'en' ? {
  title: 'Financial policies', policyType: 'Policy type', points: 'Points', withdrawals: 'Withdrawals', commission: 'Commission', refresh: 'Refresh', loading: 'Loading…', empty: 'No policy loaded.',
  brand: 'Brand', version: 'Version', updated: 'Updated', revision: 'Revision', audit: 'Audit log', unlimited: 'Unlimited', enabled: 'Enabled', disabled: 'Disabled',
  balance: 'Maximum balance points', recharge: 'Maximum recharge points', adjustment: 'Maximum adjustment points', minimum: 'Minimum withdrawal points', maximum: 'Maximum withdrawal points', sources: 'Allowed sources', review: 'Review mode', manual: 'Manual', automatic: 'Automatic', turnover: 'Turnover multiple', defaultNote: 'The brand turnover multiple is the default; a game override takes precedence. This view does not calculate withdrawal eligibility.',
  payout: 'Payout mode', calendar: 'Calendar', notConfigured: 'Not configured', cycle: 'Cycle', weekly: 'Weekly', monthly: 'Monthly', timezone: 'Time zone', boundary: 'Boundary time', weekday: 'Weekday', monthday: 'Month day', shortMonth: 'Short month rule', lastDay: 'Use last day', skip: 'Skip short months',
  weekdays: ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'],
} : {
  title: '财务规则', policyType: '规则类型', points: '积分', withdrawals: '提现', commission: '佣金', refresh: '刷新', loading: '加载中…', empty: '尚未加载规则。',
  brand: '品牌', version: '版本', updated: '更新时间', revision: '修订记录', audit: '审计日志', unlimited: '不限', enabled: '启用', disabled: '停用',
  balance: '积分余额上限', recharge: '充值积分上限', adjustment: '积分调整上限', minimum: '最低提现积分', maximum: '最高提现积分', sources: '允许的来源', review: '审核模式', manual: '人工', automatic: '自动', turnover: '流水倍数', defaultNote: '品牌流水倍数为默认值；游戏单独设置时优先使用游戏规则。本页面不计算提现资格。',
  payout: '发放模式', calendar: '日历周期', notConfigured: '未配置', cycle: '周期', weekly: '每周', monthly: '每月', timezone: '时区', boundary: '周期边界时间', weekday: '星期', monthday: '每月日期', shortMonth: '短月规则', lastDay: '使用当月最后一天', skip: '跳过短月',
  weekdays: ['星期日', '星期一', '星期二', '星期三', '星期四', '星期五', '星期六'],
}
const enabledLabel = (value: boolean) => value ? copy().enabled : copy().disabled
const auditId = (value?: string | null) => value || '—'

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

watch(() => [props.brandId, selected.value], () => { void refresh() }, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { ++generation })
</script>

<template>
  <div data-testid="platform-financial-policies">
    <div class="page-heading"><div><h1>{{ copy().title }}</h1></div></div>
    <div class="brand-picker"><label>{{ copy().policyType }}
      <select v-model="selected" :aria-label="copy().policyType" data-testid="platform-financial-policy-select">
        <option value="points">{{ copy().points }}</option><option value="withdrawal">{{ copy().withdrawals }}</option><option value="commission">{{ copy().commission }}</option>
      </select>
    </label></div>
    <p v-if="error" class="message error global-message" role="alert">{{ error }}</p>
    <p v-if="loading" class="loading-line" aria-live="polite">{{ copy().loading }}</p>
    <section class="panel">
      <div class="panel-heading"><div><h2>{{ selected === 'points' ? copy().points : selected === 'withdrawal' ? copy().withdrawals : copy().commission }}</h2><p>{{ copy().brand }}: <span class="mono">{{ props.brandId || '—' }}</span></p></div><button class="secondary" :disabled="loading || !props.brandId" @click="refresh">{{ copy().refresh }}</button></div>
      <section v-if="policy" class="reward-detail" data-testid="platform-financial-policy-detail">
        <dl v-if="selected === 'points'" class="confirm-list reward-fields">
          <div><dt>{{ copy().brand }}</dt><dd class="mono">{{ (policy as PointPolicy).brand_id }}</dd></div>
          <div><dt>{{ copy().version }}</dt><dd>{{ (policy as PointPolicy).version }}</dd></div>
          <div><dt>{{ copy().balance }}</dt><dd>{{ (policy as PointPolicy).max_balance_points == null ? copy().unlimited : (policy as PointPolicy).max_balance_points }}</dd></div>
          <div><dt>{{ copy().recharge }}</dt><dd>{{ (policy as PointPolicy).max_recharge_points == null ? copy().unlimited : (policy as PointPolicy).max_recharge_points }}</dd></div>
          <div><dt>{{ copy().adjustment }}</dt><dd>{{ (policy as PointPolicy).max_adjustment_points == null ? copy().unlimited : (policy as PointPolicy).max_adjustment_points }}</dd></div>
          <div><dt>{{ copy().audit }}</dt><dd class="mono">{{ auditId((policy as PointPolicy).audit_log_id) }}</dd></div>
        </dl>
        <dl v-else-if="selected === 'withdrawal'" class="confirm-list reward-fields">
          <div><dt>{{ copy().brand }}</dt><dd class="mono">{{ (policy as WithdrawalPolicy).brand_id }}</dd></div>
          <div><dt>{{ copy().version }}</dt><dd>{{ (policy as WithdrawalPolicy).version }}</dd></div>
          <div><dt>{{ copy().updated }}</dt><dd>{{ (policy as WithdrawalPolicy).updated_at }}</dd></div>
          <div><dt>{{ copy().enabled }}</dt><dd>{{ enabledLabel((policy as WithdrawalPolicy).config.enabled) }}</dd></div>
          <div><dt>{{ copy().minimum }}</dt><dd>{{ (policy as WithdrawalPolicy).config.min_points }}</dd></div>
          <div><dt>{{ copy().maximum }}</dt><dd>{{ (policy as WithdrawalPolicy).config.max_points == null ? copy().unlimited : (policy as WithdrawalPolicy).config.max_points }}</dd></div>
          <div><dt>{{ copy().sources }}</dt><dd>{{ (policy as WithdrawalPolicy).config.allowed_sources.map((source) => source === 'recharge' ? (props.locale === 'en' ? 'Recharge' : '充值') : source === 'winning' ? (props.locale === 'en' ? 'Winnings' : '中奖') : source === 'gift' ? (props.locale === 'en' ? 'Gift' : '赠送') : (props.locale === 'en' ? 'Commission' : '佣金')).join(', ') }}</dd></div>
          <div><dt>{{ copy().review }}</dt><dd>{{ (policy as WithdrawalPolicy).config.review_mode === 'manual' ? copy().manual : copy().automatic }}</dd></div>
          <div><dt>{{ copy().turnover }}</dt><dd>{{ (policy as WithdrawalPolicy).config.turnover_multiple }}</dd></div>
          <div><dt>{{ copy().audit }}</dt><dd class="mono">{{ auditId((policy as WithdrawalPolicy).audit_log_id) }}</dd></div>
        </dl>
        <dl v-else class="confirm-list reward-fields">
          <div><dt>{{ copy().brand }}</dt><dd class="mono">{{ (policy as CommissionPolicy).brand_id }}</dd></div>
          <div><dt>{{ copy().version }}</dt><dd>{{ (policy as CommissionPolicy).version }}</dd></div>
          <div><dt>{{ copy().updated }}</dt><dd>{{ (policy as CommissionPolicy).updated_at }}</dd></div>
          <div><dt>{{ copy().enabled }}</dt><dd>{{ enabledLabel((policy as CommissionPolicy).config.enabled) }}</dd></div>
          <div><dt>{{ copy().payout }}</dt><dd>{{ (policy as CommissionPolicy).config.payout_mode === 'manual' ? copy().manual : copy().automatic }}</dd></div>
          <div><dt>{{ copy().calendar }}</dt><dd>{{ (policy as CommissionPolicy).config.calendar == null ? copy().notConfigured : ((policy as CommissionPolicy).config.calendar!.cycle === 'weekly' ? copy().weekly : copy().monthly) }}</dd></div>
          <template v-if="(policy as CommissionPolicy).config.calendar">
            <div><dt>{{ copy().cycle }}</dt><dd>{{ (policy as CommissionPolicy).config.calendar!.cycle === 'weekly' ? copy().weekly : copy().monthly }}</dd></div>
            <div><dt>{{ copy().timezone }}</dt><dd>{{ (policy as CommissionPolicy).config.calendar!.timezone }}</dd></div>
            <div><dt>{{ copy().boundary }}</dt><dd>{{ (policy as CommissionPolicy).config.calendar!.boundary_time }}</dd></div>
            <div v-if="(policy as CommissionPolicy).config.calendar!.cycle === 'weekly'"><dt>{{ copy().weekday }}</dt><dd>{{ copy().weekdays[(policy as CommissionPolicy).config.calendar!.weekday!] }}</dd></div>
            <template v-else>
              <div><dt>{{ copy().monthday }}</dt><dd>{{ (policy as CommissionPolicy).config.calendar!.month_day }}</dd></div>
              <div><dt>{{ copy().shortMonth }}</dt><dd>{{ (policy as CommissionPolicy).config.calendar!.short_month === 'last_day' ? copy().lastDay : copy().skip }}</dd></div>
            </template>
          </template>
          <div><dt>{{ copy().revision }}</dt><dd class="mono">{{ (policy as CommissionPolicy).revision_id }}</dd></div>
          <div><dt>{{ copy().audit }}</dt><dd class="mono">{{ auditId((policy as CommissionPolicy).audit_log_id) }}</dd></div>
        </dl>
        <p v-if="selected === 'withdrawal'" class="wallet-pagination">{{ copy().defaultNote }}</p>
      </section>
      <p v-else-if="!loading && !error" class="empty-state">{{ copy().empty }}</p>
    </section>
  </div>
</template>

<style scoped>
.panel-heading button { flex-shrink: 0; }
</style>
