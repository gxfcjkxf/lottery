<script setup lang="ts">
import { globalLoadingActive, globalLoadingMessage } from './global-loading'
import { ref, watch } from 'vue'
import { activeModalDialog } from './modal-state'

const props = defineProps<{ label: string; hidden?: boolean; active?: boolean }>()
const overlay = ref<HTMLElement | null>(null)
// Native dialogs occupy the top layer. Keep foreground progress visible above
// the dialog that started the operation, using the same shared indicator.
watch([overlay, activeModalDialog], ([element]) => {
  if (element && !element.matches(':popover-open')) element.showPopover()
}, { flush: 'post' })

</script>

<template>
 <Teleport :to="activeModalDialog ?? 'body'">
  <div v-if="(globalLoadingActive || props.active || globalLoadingMessage) && !props.hidden" ref="overlay" popover="manual" class="global-loading-overlay" role="status" aria-live="polite" aria-busy="true" aria-atomic="true">
    <div class="global-loading-card">
      <span class="global-loading-spinner" aria-hidden="true"></span>
      <span>{{ globalLoadingMessage || props.label }}</span>
    </div>
  </div>
 </Teleport>
</template>

<style>
[data-global-loading="true"] { display: none !important; }
.global-loading-overlay {
  box-sizing: border-box;
  width: 100%;
  height: 100%;
  max-width: none;
  max-height: none;
  margin: 0;
  border: 0;
  position: fixed;
  inset: 0;
  z-index: 200;
  display: grid;
  place-items: center;
  padding: 24px;
  background: transparent;
  backdrop-filter: none;
}
.global-loading-overlay:not(:popover-open) { display: none; }

.global-loading-card {
  box-sizing: border-box;
  min-width: 0;
  max-width: 100%;
  overflow-wrap: anywhere;
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
  font-size: var(--body-font-size);
}

.global-loading-spinner {
  flex-shrink: 0;
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
