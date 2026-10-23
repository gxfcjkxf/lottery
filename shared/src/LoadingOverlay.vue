<script setup lang="ts">
import { globalLoadingActive, globalLoadingMessage } from './global-loading'

const props = defineProps<{ label: string; hidden?: boolean; active?: boolean }>()

</script>

<template>
  <div v-if="(globalLoadingActive || props.active || globalLoadingMessage) && !props.hidden" class="global-loading-overlay" role="status" aria-live="polite" aria-busy="true">
    <div class="global-loading-card">
      <span class="global-loading-spinner" aria-hidden="true"></span>
      <span>{{ globalLoadingMessage || props.label }}</span>
    </div>
  </div>
</template>

<style>
[data-global-loading="true"] { display: none !important; }
.global-loading-overlay {
  position: fixed;
  inset: 0;
  z-index: 200;
  display: grid;
  place-items: center;
  padding: 24px;
  background: transparent;
  backdrop-filter: none;
}

.global-loading-card {
  display: inline-flex;
  align-items: center;
  gap: 12px;
  min-height: 46px;
  padding: 12px 18px;
  color: var(--muted);
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: 12px;
  box-shadow: var(--shadow-md);
  font-size: 13px;
}

.global-loading-spinner {
  width: 18px;
  height: 18px;
  border: 2px solid var(--line);
  border-top-color: var(--primary);
  border-radius: 50%;
  animation: global-loading-spin .7s linear infinite;
}

@keyframes global-loading-spin {
  to { transform: rotate(360deg); }
}

@media (prefers-reduced-motion: reduce) {
  .global-loading-spinner { animation: none; }
}
</style>
