<script setup lang="ts">
import MarkSixLine from './MarkSixLine.vue'
import { watch } from 'vue'
import { createCatalogI18n } from './localization'
import { markSixCatalog, markSixLabel, markSixSelectionLabel, type MarkSixOrder } from './mark-six'
const props=defineProps<{orders:MarkSixOrder[];locale:'en'|'zh-CN';compact?:boolean}>()
const i18n=createCatalogI18n(markSixCatalog,props.locale);watch(()=>props.locale,v=>i18n.setLocale(v));const t=i18n.t

</script>
<template>
 <div class="mark-six-orders"><p v-if="!orders.length" class="empty-state">{{ t('empty') }}</p>
  <article v-for="order in orders" :key="order.id" class="mini-panel" :class="{'compact-ticket':compact}">
   <div class="order-heading"><span class="model-pill">{{ t('plateLabel',{plate:order.plate}) }}</span><strong>{{ t('period') }} {{ order.period_no }}</strong><span class="status-chip">{{ t(order.status) }}</span></div>
   <template v-if="!compact"><small>{{ order.id }}</small>
   <dl><div><dt>{{ t('total') }}</dt><dd>{{ order.total_points }}</dd></div><div><dt>{{ t('prize') }}</dt><dd>{{ order.prize_points }}</dd></div><div><dt>{{ t('refund') }}</dt><dd>{{ order.refund_points }}</dd></div><div><dt>{{ t('effective') }}</dt><dd>{{ order.effective_points }}</dd></div></dl></template>
   <div v-if="compact" class="ticket-preview"><p v-for="(item,index) in order.items.slice(0,2)" :key="index">{{ markSixLabel(item.code,t) }}<span v-if="item.selection?.length"> · {{ markSixSelectionLabel(item.code,item.selection,t) }}</span><span> · {{ item.points }}</span></p><p v-if="order.items.length>2" class="ticket-more">{{ t('ticketMore',{count:order.items.length-2}) }}</p><p class="ticket-total">{{ t('total') }} <strong>{{ order.total_points }}</strong></p></div><details><summary v-if="compact">{{ t('ticketDetails') }}</summary><summary v-else>{{ t('selected') }} · {{ order.items.length }}</summary><template v-if="compact"><small>{{ t('ticketId') }} · {{ order.id }}</small><dl><div><dt>{{ t('prize') }}</dt><dd>{{ order.prize_points }}</dd></div><div><dt>{{ t('refund') }}</dt><dd>{{ order.refund_points }}</dd></div><div><dt>{{ t('effective') }}</dt><dd>{{ order.effective_points }}</dd></div></dl></template><MarkSixLine v-for="(item,index) in order.items" :key="index" :item="item" :attributes="order.attributes" :locale="locale" /></details>
  </article>
 </div>
</template>
<style scoped>
.mark-six-orders{display:grid;gap:8px;min-width:0}.mini-panel{padding:12px;border:1px solid var(--line);border-radius:var(--radius);background:var(--surface)}.order-heading{display:flex;justify-content:space-between;gap:12px;flex-wrap:wrap}.mark-six-orders small{display:block;font-size:12px;color:var(--muted);overflow-wrap:anywhere;margin:8px 0}.mark-six-orders dl{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:8px;margin:10px 0}.mark-six-orders dt{font-size:12px;color:var(--muted)}.mark-six-orders dd{margin:4px 0;font-size:15px;font-weight:700;overflow-wrap:anywhere}.ticket-line{display:flex;flex-wrap:wrap;gap:12px;padding:10px 0;border-bottom:1px solid var(--line);font-size:13px}summary{cursor:pointer;font-size:13px;min-height:44px;align-content:center}@media(max-width:620px){.mark-six-orders dl{grid-template-columns:repeat(2,minmax(0,1fr))}}
</style>
