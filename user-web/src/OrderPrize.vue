<script setup lang="ts">
import type { BetOrder } from './betting-api';

defineProps<{
  order: Pick<BetOrder, 'prize_points' | 'settled_at' | 'payout_entry_id'>;
  locale: 'en' | 'zh';
}>();
</script>

<template>
  <span v-if="order.settled_at" data-testid="order-prize">
    {{ locale === 'zh' ? '中奖积分' : 'Prize points' }} {{ order.prize_points }} ·
    {{ order.payout_entry_id
      ? (locale === 'zh' ? '已入账' : 'Applied')
      : (locale === 'zh' ? '无中奖积分入账记录' : 'No prize credit recorded') }}
  </span>
</template>
