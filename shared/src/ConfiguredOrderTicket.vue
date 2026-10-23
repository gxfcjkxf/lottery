<script setup lang="ts">
import { computed, watch } from 'vue'
import { createCatalogI18n, configuredRuleCatalog, configuredTicketLabel, configuredTicketPrices, type ConfiguredLiveOrder, type ConfiguredQuotedLine } from './index'
const props = defineProps<{ order: ConfiguredLiveOrder; locale: 'en' | 'zh-CN' }>()
const localeCode = computed(() => props.locale)
const i18n = createCatalogI18n(configuredRuleCatalog, props.locale)
watch(localeCode, value => i18n.setLocale(value))
const t = i18n.t
import { orderTicketCatalog } from './order-ticket-catalog'
const labels=createCatalogI18n(orderTicketCatalog,props.locale)
watch(localeCode,value=>labels.setLocale(value))
const ui=labels.t
interface HistoryLine { id: string; text: string }
interface HistoryItem { id: string; title: string; summary: string; lines: HistoryLine[]; result: string }
function ticketLabel(order: ConfiguredLiveOrder, line: ConfiguredQuotedLine): string {
  return configuredTicketLabel(order.snapshot, line, localeCode.value)
}

function ticketPrices(order: ConfiguredLiveOrder, line: ConfiguredQuotedLine): string {
  return configuredTicketPrices(order.snapshot, line, localeCode.value).join(' / ')
}

const historyItems = computed<HistoryItem[]>(() => {
  const items: HistoryItem[] = []
  for (const order of [props.order]) {
    const lines: HistoryLine[] = []
    for (let index = 0; index < order.quote.lines.length; index++) {
      const line = order.quote.lines[index]!
      const lineResult = order.result?.lines[index]
      const outcome = lineResult ? ` · ${t(lineResult.outcome)} · ${lineResult.prize_points}` : ''
      const label = ticketLabel(order, line)
      const stake = String(line.stake)
      const prices = ticketPrices(order, line)
      lines.push({ id: `${order.id}:${index}`, text: [label, stake, prices].join(' · ') + outcome })
    }
    if(order.quote.reserved_points)lines.push({id:`${order.id}:reserved`,text:`${t('reserved')}: ${order.quote.reserved_points}`})
    if(order.result?.consumed_points)lines.push({id:`${order.id}:consumed`,text:`${t('consumed')}: ${order.result.consumed_points}`})
    items.push({
      id: order.id,
      title: order.game_name,
      summary: `${order.period_no} · ${order.quote.bet_points} · ${t(order.status === 'settled' ? 'state.settled' : 'state.placed')}`,
      lines,
      result: order.result ? `${t('prize')}: ${order.result.prize_points} · ${t(order.snapshot.catalog.funding==='reserve'?'release':'refund')}: ${order.result.refund_points}` : '',
    })
  }
  return items
})

</script>
<template>
  <article v-for="item in historyItems" :key="item.id" class="configured-order-card">
    <p>{{ item.summary }}</p>
    <p v-for="line in item.lines" :key="line.id">{{ line.text }}</p>
    <p v-if="item.result">{{ item.result }}</p>
    <details><summary>{{ ui('details') }}</summary><small>{{ item.id }}</small></details>
  </article>
</template>
<style scoped>
.configured-order-card { display: grid; gap: 4px; min-width: 0; padding: 12px; border: 1px solid var(--line); border-radius: var(--radius); overflow-wrap: anywhere; }
.configured-order-card p { margin: 0; }
.configured-order-card summary { cursor: pointer; min-height: 44px; align-content: center; }
</style>
