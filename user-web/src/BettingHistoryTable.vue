<script setup lang="ts">
import { computed } from 'vue'
import { createCatalogI18n, configuredRuleCatalog, configuredTicketLabel, configuredTicketPrices, formatDisplayDateTime } from '@lottery/shared'
import type { HistoryItem } from './betting-history-api'
import { useUserI18n } from './i18n'
const props = defineProps<{ items: HistoryItem[]; showGame?: boolean; locale: 'en' | 'zh-CN' }>()
const ui = useUserI18n().t
const labels = computed(() => ({ configured: createCatalogI18n(configuredRuleCatalog, props.locale).t }))
function status(item: HistoryItem) { return labels.value.configured(item.order.status === 'settled' ? 'state.settled' : 'state.placed') }
const amountColumns = [
  { key: 'reserved', label: () => labels.value.configured('reserved'), value: (item: HistoryItem) => item.order.quote.reserved_points },
  { key: 'consumed', label: () => labels.value.configured('consumed'), value: (item: HistoryItem) => item.order.result?.consumed_points },
  { key: 'refund', label: () => ui('user.orders.column.refund'), value: (item: HistoryItem) => item.order.snapshot.catalog.funding !== 'reserve' ? item.order.result?.refund_points : undefined },
  { key: 'release', label: () => labels.value.configured('release'), value: (item: HistoryItem) => item.order.snapshot.catalog.funding === 'reserve' ? item.order.result?.refund_points : undefined },
]
function positive(value: string | undefined) { return value !== undefined && BigInt(value) > 0n }
const visibleAmounts = computed(() => amountColumns.filter(column => props.items.some(item => positive(column.value(item)))))
const columns = ['period', 'content', 'odds', 'stake', 'prize'] as const
</script>
<template>
  <table class="history-table" :aria-label="ui('user.orders.title')">
    <thead><tr><th v-for="column in columns" :key="column" scope="col">{{ ui(`user.orders.column.${column}`) }}</th><th v-for="column in visibleAmounts" :key="column.key" scope="col">{{ column.label() }}</th><th scope="col">{{ ui('user.orders.column.status') }}</th><th scope="col">{{ ui('user.orders.column.time') }}</th></tr></thead>
    <tbody>
      <tr v-for="item in items" :key="`${item.kind}:${item.id}`">
        <td :data-label="ui('user.orders.column.period')"><strong v-if="showGame" class="history-game">{{ item.game_name }}</strong>{{ item.period_no }}</td>
        <td class="history-content" :data-label="ui('user.orders.column.content')">
          <p v-for="(line, index) in item.order.quote.lines" :key="index">{{ configuredTicketLabel(item.order.snapshot, line, locale) }}</p>
        </td>
        <td class="history-odds" :data-label="ui('user.orders.column.odds')">
          <template v-if="item.kind === 'configured'"><p v-for="(line, index) in item.order.quote.lines" :key="index">{{ configuredTicketPrices(item.order.snapshot, line, locale).join(' / ') }}</p></template>

        </td>
        <td :data-label="ui('user.orders.column.stake')">{{ item.order.quote.bet_points }}</td>
        <td :data-label="ui('user.orders.column.prize')">{{ positive(item.order.result?.prize_points) ? item.order.result!.prize_points : '—' }}</td>
        <td v-for="column in visibleAmounts" :key="column.key" :class="{ 'history-amount-empty': !positive(column.value(item)) }" :data-label="column.label()">{{ positive(column.value(item)) ? column.value(item) : '—' }}</td>
        <td :data-label="ui('user.orders.column.status')"><span class="history-status" :class="`history-status--${item.order.status}`">{{ status(item) }}</span></td>
        <td :data-label="ui('user.orders.column.time')"><time :datetime="item.placed_at">{{ formatDisplayDateTime(item.placed_at, locale) }}</time></td>
      </tr>
    </tbody>
  </table>
</template>
<style scoped>
.history-table{width:100%;table-layout:auto;border-collapse:collapse;font-size:var(--body-font-size);line-height:1.5;background:var(--surface)}
.history-table th,.history-table td{padding:10px 8px;text-align:left;vertical-align:middle;border-bottom:1px solid var(--line);overflow-wrap:anywhere}
.history-table th{color:var(--muted);font-weight:600;background:var(--canvas)}
@media(min-width:701px){.history-table th:last-child,.history-table time{white-space:nowrap}}
.history-game{display:block;margin-bottom:4px}.history-table td{font-variant-numeric:tabular-nums}.history-content p,.history-odds p{margin:0}.history-content p+p,.history-odds p+p,.history-odds div+div{margin-top:4px}.history-content :deep(.selection-summary){padding:0;background:transparent;gap:4px}
.history-status{color:var(--muted);font-weight:600}
.history-status--placed,.history-status--won{color:var(--success-text)}
.history-status--abnormal{color:var(--warning-text)}
.history-status--lost{color:var(--danger-text)}
@media(max-width:700px){
 .history-table,.history-table tbody{display:block}.history-table thead{display:none}
 .history-table tr{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));padding:8px 0;border-bottom:1px solid var(--line)}
 .history-table td{display:block;min-width:0;border:0;padding:6px 8px}.history-table td::before{content:attr(data-label);display:block;margin-bottom:4px;color:var(--muted);font-weight:600}
 .history-table td.history-amount-empty{display:none}
 .history-table td:nth-child(2),.history-table td:last-child{grid-column:1/-1}.history-table td:nth-child(2){grid-row:2}
}
</style>
