<script setup lang="ts">
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { useUserI18n } from './i18n';

defineProps<{ title: string; loading: boolean; empty: boolean; error: boolean }>();
defineEmits<{ refresh: [] }>();
const ui = useUserI18n().t;
</script>

<template>
  <section class="order-history user-record-list" :aria-busy="loading" :aria-label="title">
      <Teleport v-if="error" defer to="#user-page-refresh"><button type="button" v-refresh-button="ui('betting.refresh')" class="button button-secondary" :disabled="loading" @click="withGlobalLoading(async () => { return $emit('refresh') })">
        {{ ui('betting.refresh') }}
      </button></Teleport>
    <slot name="filters" />
    <p v-if="empty" class="order-history-empty empty-state">{{ ui('betting.noOrders') }}</p>
    <slot v-else />
    <slot name="pagination" />
  </section>
</template>

<style scoped>
.order-history { display: grid; gap: 12px; min-width: 0; width: 100%; margin: 0 auto; }

</style>
