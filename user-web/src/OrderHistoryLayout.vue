<script setup lang="ts">
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { useUserI18n } from './i18n';

defineProps<{ title: string; loading: boolean; empty: boolean }>();
defineEmits<{ refresh: [] }>();
const ui = useUserI18n().t;
</script>

<template>
  <section class="order-history">
    <div class="order-history-toolbar">
      <h2>{{ title }}</h2>
      <Teleport defer to="#user-page-refresh"><button v-refresh-button="ui('betting.refresh')" class="button button-secondary" :disabled="loading" @click="withGlobalLoading(async () => { return $emit('refresh') })">
        {{ ui('betting.refresh') }}
      </button></Teleport>
    </div>
    <slot name="filters" />
    <p v-if="!loading && empty" class="order-history-empty empty-state">{{ ui('betting.noOrders') }}</p>
    <slot v-else />
    <slot name="pagination" />
  </section>
</template>

<style scoped>
.order-history { display: grid; gap: 12px; min-width: 0; width: 100%; max-width: 1100px; margin: 0 auto; }
.order-history-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-width: 0; }
.order-history-toolbar h2 { margin: 0; min-width: 0; font-size: 20px; overflow-wrap: anywhere; }
.order-history-toolbar button { flex-shrink: 0; min-height: 44px; }
.order-history :deep(.history-pages) { display: flex; gap: 8px; }
.order-history :deep(.history-pages button) { min-height: 44px; }

</style>
