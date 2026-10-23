<script setup lang="ts">
import CanadaLine from './CanadaLine.vue'
import { watch } from 'vue'
import { createCatalogI18n } from './localization'
import { canadaCatalog, canadaLabel, type CanadaKey, type CanadaOrder } from './canada'
const props=defineProps<{orders:CanadaOrder[];locale:'en'|'zh-CN';compact?:boolean}>()
const i18n=createCatalogI18n(canadaCatalog,props.locale);watch(()=>props.locale,v=>i18n.setLocale(v));const t=i18n.t

</script>
<template>
 <div class="canada-orders"><p v-if="!orders.length" class="empty-state">{{ t('empty') }}</p>
  <article v-for="order in orders" :key="order.id" class="mini-panel" :class="{'compact-ticket':compact}">
   <p class="order-game">{{ t(order.profile as CanadaKey) }}</p><div class="order-heading"><strong>{{ t('period') }} {{ order.period_no }}</strong><span class="status-chip">{{ t(order.status) }}</span></div>
   <template v-if="!compact"><small>{{ order.id }}</small>
   <dl><div><dt>{{ t('total') }}</dt><dd>{{ order.total_points }}</dd></div><div><dt>{{ t('prize') }}</dt><dd>{{ order.prize_points }}</dd></div><div><dt>{{ t(order.profile==='canada_niuniu'?'riskRelease':'refund') }}</dt><dd>{{ order.refund_points }}</dd></div><div><dt>{{ t('effective') }}</dt><dd>{{ order.effective_points }}</dd></div></dl></template>
   <p v-if="!compact&&order.profile==='canada_niuniu'">{{ t('riskReserved') }} {{ order.reserved_points }} · {{ t('riskLoss') }} {{ order.lost_points }}</p><div v-if="compact" class="ticket-preview"><p v-for="(item,index) in order.items.slice(0,2)" :key="index">{{ canadaLabel(item.code,t) }} · {{ item.points }}</p><p v-if="order.items.length>2" class="ticket-more">{{ t('ticketMore',{count:order.items.length-2}) }}</p><p class="ticket-total">{{ t('total') }} <strong>{{ order.total_points }}</strong></p></div><details><summary v-if="compact">{{ t('ticketDetails') }}</summary><summary v-else>{{ t('selected') }} · {{ order.items.length }}</summary><template v-if="compact"><small>{{ t('ticketId') }} · {{ order.id }}</small><dl><div><dt>{{ t('prize') }}</dt><dd>{{ order.prize_points }}</dd></div><div><dt>{{ t(order.profile==='canada_niuniu'?'riskRelease':'refund') }}</dt><dd>{{ order.refund_points }}</dd></div><div><dt>{{ t('effective') }}</dt><dd>{{ order.effective_points }}</dd></div></dl><p v-if="order.profile==='canada_niuniu'">{{ t('riskReserved') }} {{ order.reserved_points }} · {{ t('riskLoss') }} {{ order.lost_points }}</p></template><CanadaLine v-for="(item,index) in order.items" :key="index" :item="item" :profile="order.profile" :locale="locale" /></details>
  </article>
 </div>
</template>
<style scoped>
.canada-orders{display:grid;gap:8px;min-width:0}.mini-panel{padding:12px;border:1px solid var(--line);border-radius:var(--radius);background:var(--surface)}.order-game{margin:0 0 6px;font-size:13px;font-weight:700;color:var(--ink-secondary)}.order-heading{display:flex;justify-content:space-between;gap:12px;flex-wrap:wrap}.canada-orders small{display:block;font-size:12px;color:var(--muted);overflow-wrap:anywhere;margin:8px 0}.canada-orders dl{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:8px;margin:10px 0}.canada-orders dt{font-size:12px;color:var(--muted)}.canada-orders dd{margin:4px 0;font-size:15px;font-weight:700;overflow-wrap:anywhere}.ticket-line{display:flex;flex-wrap:wrap;gap:12px;padding:10px 0;border-bottom:1px solid var(--line);font-size:13px}summary{cursor:pointer;font-size:13px;min-height:44px;align-content:center}@media(max-width:620px){.canada-orders dl{grid-template-columns:repeat(2,minmax(0,1fr))}}
</style>
