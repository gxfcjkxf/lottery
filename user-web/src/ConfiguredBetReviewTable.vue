<script setup lang="ts">
import { computed } from 'vue'
import { configuredTicketLabel, type ConfiguredSnapshot, type ConfiguredQuotedLine } from '@lottery/shared'
import { useUserI18n } from './i18n'
const props = defineProps<{ snapshot: ConfiguredSnapshot; lines: ConfiguredQuotedLine[]; locale: 'en' | 'zh-CN' }>()
const ui = useUserI18n().t
function priceEntries(line: ConfiguredQuotedLine, label = ''): Array<{ label: string; value: string }> {
  return [
    ...Object.values(line.prices).map(value => ({ label, value })),
    ...(line.legs ?? []).flatMap(leg => priceEntries(leg, [label, configuredTicketLabel(props.snapshot, leg, props.locale)].filter(Boolean).join(' / '))),
  ]
}
const rows = computed(() => props.lines.map(line => ({
  label: configuredTicketLabel(props.snapshot, line, props.locale), stake: line.stake,
  prices: priceEntries(line),
})))
</script>
<template>
  <table class="bet-review-table">
    <thead><tr><th scope="col">{{ ui('user.orders.column.content') }}</th><th scope="col">{{ ui('user.orders.column.odds') }}</th><th scope="col">{{ ui('user.orders.column.stake') }}</th></tr></thead>
    <tbody><tr v-for="(row, index) in rows" :key="index"><td>{{ row.label }}</td><td><span v-for="(price, priceIndex) in row.prices" :key="priceIndex" class="bet-review-price"><span v-if="price.label" class="bet-review-price-label">{{ price.label }}</span><span class="bet-review-price-value">{{ price.value }}</span></span></td><td>{{ row.stake }}</td></tr></tbody>
  </table>
</template>
<style scoped>
.bet-review-table{width:100%;max-width:100%;justify-self:start;border-collapse:collapse;font-size:var(--body-font-size);line-height:1.5;table-layout:auto}
.bet-review-table th,.bet-review-table td{padding:6px 10px;text-align:left;vertical-align:middle;border-bottom:1px solid var(--line);overflow-wrap:anywhere}
.bet-review-table th{background:var(--canvas);color:var(--muted);font-weight:600}
.bet-review-table td{font-variant-numeric:tabular-nums}.bet-review-table td:nth-child(3){white-space:nowrap}
.bet-review-table :is(th,td):nth-child(n+2){text-align:right}

.bet-review-price{display:block}.bet-review-price-label{display:block;overflow-wrap:anywhere}.bet-review-price-value{white-space:nowrap;overflow-wrap:normal;word-break:normal}.bet-review-price+.bet-review-price{margin-top:4px}
@media(max-width:620px){.bet-review-table th,.bet-review-table td{padding-inline:8px}}
</style>
