<script setup lang="ts">
import {formatDisplayDateTime} from './display-settings'
import type {OrderHistoryRecord} from './order-history'
import ConfiguredOrderTicket from './ConfiguredOrderTicket.vue'
defineProps<{item:OrderHistoryRecord;locale:'en'|'zh-CN'}>()
</script>
<template>
  <article class="history-record" data-testid="history-record">
    <header><strong>{{item.game_name}}</strong><time :datetime="item.placed_at">{{formatDisplayDateTime(item.placed_at,locale)}}</time></header>
    <ConfiguredOrderTicket :order="item.order" :locale="locale" />
  </article>
</template>
<style scoped>
.history-record { display:grid;gap:8px;min-width:0; }
.history-record > header { display:flex;align-items:center;justify-content:space-between;gap:8px;flex-wrap:wrap;overflow-wrap:anywhere; }
.history-record time { color:var(--muted);font-size:var(--body-font-size); }
</style>
