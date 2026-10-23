<script setup lang="ts">
import { watch } from 'vue'
import { formatDisplayDateTime } from './index'
import { createCatalogI18n } from './localization'
import { orderTicketCatalog } from './order-ticket-catalog'
import type { OrderHistoryRecord } from './order-history'
import MarkSixOrders from './MarkSixOrders.vue'
import CanadaOrders from './CanadaOrders.vue'
import ConfiguredOrderTicket from './ConfiguredOrderTicket.vue'
import SelectionSummary from './SelectionSummary.vue'
import OrderPrize from './OrderPrize.vue'
const props=defineProps<{item:OrderHistoryRecord;locale:'en'|'zh-CN'}>()
const labels=createCatalogI18n(orderTicketCatalog,props.locale),ui=labels.t
watch(()=>props.locale,value=>labels.setLocale(value))
</script>
<template>
  <article class="history-record" data-testid="history-record">
    <header><strong>{{item.game_name}}</strong><time :datetime="item.placed_at">{{formatDisplayDateTime(item.placed_at,locale)}}</time></header>
    <MarkSixOrders v-if="item.kind==='mark-six'" :orders="[item.order]" :locale="locale" compact />
    <CanadaOrders v-else-if="item.kind==='canada'" :orders="[item.order]" :locale="locale" compact />
    <ConfiguredOrderTicket v-else-if="item.kind==='configured'" :order="item.order" :locale="locale" />
    <div v-else-if="item.kind==='standard'" class="standard-ticket">
      <strong>{{ui(item.order.status as 'placed'|'abnormal'|'won'|'lost'|'bet_cancelled'|'judged_cancelled')}}</strong>
      <p>{{ui('period')}} · {{item.period_no}}</p><p>{{ui('points')}} {{item.order.total_points}} · ×{{item.order.multiplier}}</p>
      <OrderPrize :order="item.order" :locale="locale" /><SelectionSummary :selection="item.order.selection_normalized" :locale="locale" /><slot name="details" />
    </div>
  </article>
</template>
<style scoped>
.history-record { display:grid;gap:8px;min-width:0; }
.history-record > header { display:flex;align-items:center;justify-content:space-between;gap:8px;flex-wrap:wrap;overflow-wrap:anywhere; }
.history-record time { color:var(--muted);font-size:12px; }
.standard-ticket { display:grid;gap:8px;min-width:0;padding:12px;border:1px solid var(--line);border-radius:var(--radius);overflow-wrap:anywhere; }
.standard-ticket p { margin:0; }
</style>
