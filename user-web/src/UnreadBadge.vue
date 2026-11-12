<script setup lang="ts">
import { useUserI18n } from './i18n';
defineProps<{ count: string | null }>();
const { t } = useUserI18n();
</script>

<template>
  <span v-if="count !== null && count !== '0'" class="unread-badge" :aria-label="t('common.unreadNotifications', { count })">{{ BigInt(count) > 99n ? '99+' : count }}</span>
</template>

<style scoped>
.unread-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  min-width: 24px;
  min-height: 24px;
  padding-inline: 6px;
  border-radius: var(--radius);
  background: var(--danger-bg);
  color: var(--danger-text);
  font-size: var(--body-font-size);
  font-weight: 650;
  line-height: 1.5;
  font-variant-numeric: tabular-nums;
}
</style>
