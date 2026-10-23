<script setup lang="ts">
import { watch } from 'vue'
import { createCatalogI18n } from './localization'
import { orderTicketCatalog } from './order-ticket-catalog'
const props=defineProps<{order:{prize_points:string;settled_at?:string|null;payout_entry_id?:string|null};locale:'en'|'zh'|'zh-CN'}>()
const i18n=createCatalogI18n(orderTicketCatalog,props.locale==='zh'?'zh-CN':props.locale),ui=i18n.t
watch(()=>props.locale,value=>i18n.setLocale(value==='zh'?'zh-CN':value))
</script>
<template>
  <span v-if="order.settled_at" data-testid="order-prize">
    {{ ui('prize') }} {{ order.prize_points }} ·
    {{ order.payout_entry_id ? ui('applied') : ui('missing') }}
  </span>
</template>
