<script setup lang="ts">
import { computed, watch } from 'vue'
import { AssociationName } from './association-names'
import { createCatalogI18n } from './localization'
import { archiveReportCatalog, type ArchiveReportRecord } from './archive-report'
const props = defineProps<{ record: ArchiveReportRecord; locale: 'en' | 'zh-CN'; metricLabel: (field: string) => string; groupLabels: Record<string, string>; formatTime: (value: string) => string }>()
const i18n = createCatalogI18n(archiveReportCatalog, props.locale)
watch(() => props.locale, locale => i18n.setLocale(locale))
const t = i18n.t
const value = (group: object, field: string) => (group as Record<string, string>)[field]
const overview = computed(() => [
  { label: props.metricLabel('stake_points'), value: value(props.record.snapshot.betting, 'stake_points') },
  { label: props.metricLabel('current_prize_points'), value: value(props.record.snapshot.betting, 'current_prize_points') },
  { label: props.metricLabel('recharge_points'), value: value(props.record.snapshot.ledger, 'recharge_points') },
  { label: props.metricLabel('paid_points'), value: value(props.record.snapshot.withdrawals, 'paid_points') },
  { label: t('commissionNet'), value: value(props.record.snapshot.commissions, 'net_points') },
  { label: t('walletTotal'), value: value(props.record.snapshot.wallet_snapshot.balances, 'total_points') },
])
const groups = computed(() => [
  { key: 'betting', values: props.record.snapshot.betting, fields: ['order_count', 'stake_points', 'current_prize_points', 'settled_stake_points', 'unfinalized_stake_points', 'refund_points', 'abnormal_stake_points', 'placed_count', 'won_count', 'lost_count', 'cancelled_count', 'abnormal_count', 'correction_open_count'] },
  { key: 'withdrawals', values: props.record.snapshot.withdrawals, fields: [] },
  { key: 'ledger', values: props.record.snapshot.ledger, fields: ['recharge_points', 'prize_credit_points', 'prize_reversal_points', 'refund_points', 'net_points', 'entry_count'] },
  { key: 'commissions', values: props.record.snapshot.commissions, fields: ['net_points', 'paid_points', 'adjustment_credit_points', 'adjustment_debit_points', 'correction_credit_points', 'correction_debit_points', 'entry_count', 'paid_entry_count', 'adjustment_entry_count', 'correction_entry_count'] },
  { key: 'rewards', values: props.record.snapshot.rewards, fields: ['net_points', 'grant_points', 'reversal_points', 'entry_count', 'grant_entry_count', 'reversal_entry_count'] },
  { key: 'reward_orders', values: props.record.snapshot.reward_orders, fields: ['order_count', 'original_points', 'granted_points', 'granted_count', 'pending_points', 'pending_count', 'revoked_points', 'revoked_count'] },
  { key: 'wallet', values: props.record.snapshot.wallet_snapshot.balances, fields: ['total_points', 'available_points', 'frozen_points', 'withdrawal_points', 'account_count'] },
])
const withdrawalRows = ['reviewing', 'processing', 'paid', 'rejected', 'failed', 'cancelled'] as const
const formatPoints = (input: string) => BigInt(input).toLocaleString(props.locale)
</script>

<template>
  <article class="archive-report">
    <header class="archive-report-heading">
      <div><h2>{{ record.window.kind === 'daily' ? t('daily') : t('monthly') }} · {{ record.window.period_key }}</h2><p>{{ t('title') }}</p></div>
      <div class="archive-report-actions"><slot name="actions" /></div>
    </header>
    <div class="archive-report-context"><span>{{ t('from') }} {{ formatTime(record.window.from) }}</span><span>{{ t('to') }} {{ formatTime(record.window.to) }}</span><span>{{ t('timezone') }} {{ record.window.timezone }}</span><span>{{ t('snapshotAt') }} {{ formatTime(record.snapshot_at) }}</span></div>
    <p class="archive-report-note">{{ t('scopeNote') }}</p>
    <section class="archive-report-overview" :aria-label="t('overview')"><h3>{{ t('overview') }}</h3><dl><div v-for="item in overview" :key="item.label"><dt>{{ item.label }}</dt><dd>{{ formatPoints(item.value) }}</dd></div></dl></section>
    <div class="archive-report-groups">
      <section v-for="group in groups" :key="group.key" class="archive-report-group">
        <h3>{{ groupLabels[group.key] }}</h3>
        <div class="archive-report-table-wrap"><table class="archive-report-table">
          <template v-if="group.key === 'withdrawals'">
            <thead><tr><th scope="col">{{ t('withdrawalStatus') }}</th><th scope="col">{{ t('count') }}</th><th scope="col">{{ t('points') }}</th></tr></thead><tbody>
              <tr><th scope="row">{{ t('total') }}</th><td>{{ formatPoints(value(group.values, 'order_count')) }}</td><td>{{ formatPoints(value(group.values, 'requested_points')) }}</td></tr>
              <tr v-for="state in withdrawalRows" :key="state"><th scope="row">{{ t(state) }}</th><td>{{ formatPoints(value(group.values, `${state}_count`)) }}</td><td>{{ formatPoints(value(group.values, `${state}_points`)) }}</td></tr>
            </tbody>
          </template>
          <tbody v-else><tr v-for="field in group.fields" :key="field"><th scope="row">{{ metricLabel(field) }}</th><td>{{ formatPoints(value(group.values, field)) }}</td></tr></tbody>
        </table></div>
        <p v-if="group.key === 'wallet'" class="archive-report-note">{{ t('walletNote') }}</p>
      </section>
    </div>
    <section class="archive-report-record"><h3>{{ t('record') }}</h3><dl class="archive-report-metrics"><div><dt>{{ t('source') }}</dt><dd>{{ record.automation ? t('automatic') : t('manual') }}</dd></div><div><dt>{{ t('createdAt') }}</dt><dd>{{ formatTime(record.created_at) }}</dd></div><div v-if="record.reason"><dt>{{ t('reason') }}</dt><dd>{{ record.reason }}</dd></div></dl></section>
  </article>
</template>

<style scoped>
.archive-report { min-width: 0; padding: 20px; }
.archive-report .archive-report-heading { display: flex; justify-content: space-between; flex-wrap: wrap; align-items: start; gap: 12px; }
.archive-report h2 { margin: 0; font-size: 20px; }
.archive-report h3 { margin: 0 0 10px; font-size: 16px; }
.archive-report p { margin: 6px 0; font-size: 12px; color: var(--muted); }
.archive-report-actions, .archive-report-context { display: flex; flex-wrap: wrap; gap: 12px; }
.archive-report-context { padding: 12px 0; font-size: 12px; color: var(--muted); border-bottom: 1px solid var(--line); }
.archive-report-overview { padding: 18px 0; }
.archive-report .archive-report-overview dl { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 160px), 1fr)); gap: 12px; margin: 0; }
.archive-report-overview dl > div { min-width: 0; padding: 12px; background: var(--canvas); border: 1px solid var(--line); border-radius: var(--radius-sm); }
.archive-report .archive-report-overview dt { font-size: 12px; color: var(--muted); }
.archive-report .archive-report-overview dd { margin: 4px 0 0; font-size: 22px; font-weight: 700; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.archive-report-groups { columns: 300px; column-gap: 16px; }
.archive-report-group { display: inline-block; vertical-align: top; width: 100%; min-width: 0; box-sizing: border-box; break-inside: avoid; border: 1px solid var(--line); border-radius: var(--radius-sm); padding: 12px; margin-bottom: 16px; }
.archive-report .archive-report-metrics { display: block; margin: 0; }
.archive-report .archive-report-metrics > div { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); align-items: baseline; gap: 12px; padding: 7px 0; border-bottom: 1px solid var(--line-subtle); font-size: 13px; }
.archive-report .archive-report-metrics dt { color: var(--muted); font-size: 13px; }
.archive-report .archive-report-metrics dd { margin: 0; text-align: right; font-weight: 600; font-size: 13px; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.archive-report-table-wrap { min-width: 0; }
.archive-report .archive-report-table { width: 100%; min-width: 0; table-layout: fixed; border-collapse: collapse; }
.archive-report .archive-report-table th, .archive-report .archive-report-table td { padding: 7px 4px; font-size: 13px; text-align: right; overflow-wrap: anywhere; position: static; border: 0; border-bottom: 1px solid var(--line-subtle); background: transparent; }
.archive-report .archive-report-table th { color: var(--muted); font-weight: 400; }
.archive-report .archive-report-table th:first-child { text-align: left; width: 52%; }
.archive-report .archive-report-table td { color: var(--ink); font-weight: 600; font-variant-numeric: tabular-nums; }
.archive-report .archive-report-table thead th { font-weight: 600; border-bottom-color: var(--line); }
.archive-report-record, .archive-report-technical { margin-top: 20px; padding-top: 14px; border-top: 1px solid var(--line); }
.archive-report-technical summary { cursor: pointer; font-size: 13px; color: var(--muted); padding: 8px 0; }
@media (max-width: 700px) {
  .archive-report { padding: 14px; }
  .archive-report-actions button { min-height: 44px; }
}
</style>
