<script setup lang="ts">
import { computed } from 'vue'
import { pointsMinor, createCatalogI18n, configuredRuleCatalog, configuredTicketLabel, configuredTicketPrices, formatDisplayDateTime } from '@lottery/shared'
import type { OrderHistoryRecord } from './order-history'
import { bettingHistoryCatalog } from './betting-history-catalog'
import OrderOutcome from './OrderOutcome.vue'
type HistoryItem = OrderHistoryRecord & { member_name?: string }
const props = defineProps<{ items: HistoryItem[]; showGame?: boolean; showMember?: boolean; empty?: boolean; locale: 'en' | 'zh-CN' }>()
const ui = (key: keyof typeof bettingHistoryCatalog.en) => bettingHistoryCatalog[props.locale][key]
const labels = computed(() => createCatalogI18n(configuredRuleCatalog, props.locale).t)
const amountColumns = [
  { key: 'reserved', label: () => labels.value('reserved'), value: (item: HistoryItem) => item.order.quote.reserved_points },
  { key: 'consumed', label: () => labels.value('consumed'), value: (item: HistoryItem) => item.order.result?.consumed_points },
  { key: 'refund', label: () => ui('user.orders.column.refund'), value: (item: HistoryItem) => item.order.snapshot.catalog.funding !== 'reserve' ? item.order.result?.refund_points : undefined },
  { key: 'release', label: () => labels.value('release'), value: (item: HistoryItem) => item.order.snapshot.catalog.funding === 'reserve' ? item.order.result?.refund_points : undefined },
]
function positive(value: string | undefined) { return value !== undefined && pointsMinor(value) > 0n }
const visibleAmounts = computed(() => amountColumns.filter(column => props.items.some(item => positive(column.value(item)))))
const columns = computed(() => [...(props.showGame ? ['game' as const] : []), ...(props.showMember ? ['member' as const] : []), 'period', 'content', 'outcome', 'odds', 'stake', 'prize'] as const)
</script>
<template>
 <div class="table-wrap history-table-wrap">
  <table class="history-table" :aria-label="ui('user.orders.title')">
   <thead><tr><th v-for="column in columns" :key="column" :class="{ 'history-amount': column==='stake' || column==='prize' }" scope="col">{{ ui(`user.orders.column.${column}`) }}</th><th v-for="column in visibleAmounts" :key="column.key" class="history-amount" scope="col">{{ column.label() }}</th><th scope="col">{{ ui('user.orders.column.status') }}</th><th scope="col">{{ ui('user.orders.column.time') }}</th></tr></thead>
   <tbody>
    <tr v-for="item in items" :key="`${item.kind}:${item.id}`">
     <td v-if="showGame" :data-label="ui('user.orders.column.game')"><span class="history-value">{{ item.game_name }}</span></td>
     <td v-if="showMember" :data-label="ui('user.orders.column.member')"><span class="history-value">{{ item.member_name }}</span></td>
     <td class="history-period" :data-label="ui('user.orders.column.period')"><span class="history-value">{{ item.period_no }}</span></td>
     <td class="history-content" :data-label="ui('user.orders.column.content')"><div class="history-value"><p v-for="(line,index) in item.order.quote.lines" :key="index">{{ configuredTicketLabel(item.order.snapshot,line,locale) }}</p></div></td>
     <td class="history-outcomes" :data-label="ui('user.orders.column.outcome')"><div class="history-value"><p v-for="(_,index) in item.order.quote.lines" :key="index"><OrderOutcome :outcome="item.order.result?.lines[index]?.outcome" :locale="locale" /></p></div></td>
     <td class="history-odds" :data-label="ui('user.orders.column.odds')"><div class="history-value"><p v-for="(line,index) in item.order.quote.lines" :key="index">{{ configuredTicketPrices(item.order.snapshot,line,locale).join(' / ') }}</p></div></td>
     <td class="history-amount" :data-label="ui('user.orders.column.stake')"><span class="history-value">{{ item.order.quote.bet_points }}</span></td>
     <td class="history-amount" :data-label="ui('user.orders.column.prize')"><span class="history-value">{{ item.order.result ? item.order.result.prize_points : '—' }}</span></td>
     <td v-for="column in visibleAmounts" :key="column.key" class="history-amount" :class="{ 'history-amount-empty': !positive(column.value(item)) }" :data-label="column.label()"><span class="history-value">{{ positive(column.value(item)) ? column.value(item) : '—' }}</span></td>
     <td :data-label="ui('user.orders.column.status')"><span class="history-value history-status">{{ labels(item.order.status==='settled'?'state.settled':'state.placed') }}</span></td>
     <td class="history-time" :data-label="ui('user.orders.column.time')"><time class="history-value" :datetime="item.placed_at">{{ formatDisplayDateTime(item.placed_at,locale) }}</time></td>
    </tr>
    <tr v-if="empty"><td :colspan="columns.length+visibleAmounts.length+2" class="empty-state">{{ labels('ordersEmpty') }}</td></tr>
   </tbody>
  </table>
 </div>
</template>
<style scoped>
.history-table-wrap{width:100%;min-width:0;max-width:100%;overflow-x:auto}
.history-table{width:100%;min-width:0;white-space:normal;table-layout:auto;border-collapse:collapse;font-size:var(--body-font-size);line-height:1.5;background:var(--surface)}
.history-table th,.history-table td{padding:8px;height:auto;min-width:0;white-space:normal;text-align:start;vertical-align:middle;border-bottom:1px solid var(--line);overflow-wrap:anywhere;position:static}
.history-table th{color:var(--muted);font-weight:650;background:var(--table-header-bg)}
.history-table td{font-variant-numeric:tabular-nums}
.history-table .history-amount{text-align:end}
.history-amount .history-value{font-variant-numeric:tabular-nums}
.history-value{min-width:0;max-width:100%}.history-value p{margin:0}.history-value p+p{margin-top:4px}
.history-status{color:var(--muted)}
@media(min-width:701px){.history-table time{white-space:nowrap}}
@media(max-width:700px), (max-width:900px) and (pointer:coarse){
 .history-table-wrap{overflow:visible}
 .history-table,.history-table tbody{display:block}.history-table thead{display:none}
 .history-table tr{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:2px 12px;padding:8px;border-bottom:1px solid var(--line)}
 .history-table td{display:flex;align-items:baseline;gap:6px;min-width:0;border:0;padding:3px 0;text-align:start}
 .history-table td:not(.empty-state)::before{content:attr(data-label);flex:0 1 auto;max-width:52%;text-align:start;color:var(--muted);font-size:12px;line-height:1.5}
 .history-value{flex:1 1 0;overflow-wrap:anywhere}
 .history-amount .history-value{text-align:end}
 .history-table td.history-amount-empty{display:none}
 .history-table :is(.history-period,.history-content,.history-time){grid-column:1/-1}
}
</style>
