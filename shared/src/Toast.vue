<script setup lang="ts">
import SiteIcon from "./SiteIcon";
import { onBeforeUnmount, watch } from 'vue'

const props = withDefaults(defineProps<{ title: string; message: string; closeLabel: string; tone?: 'success' | 'error' | 'info'; duration?: number }>(), {
  tone: 'success',
  duration: 3600,
})
const emit = defineEmits<{ dismiss: [] }>()
let timer: ReturnType<typeof setTimeout> | undefined

function scheduleDismiss() {
  if (timer) clearTimeout(timer)
  timer = setTimeout(() => emit('dismiss'), props.duration)
}

watch(() => props.message, value => { if (value) scheduleDismiss() }, { immediate: true })
onBeforeUnmount(() => { if (timer) clearTimeout(timer) })
</script>

<template>
  <aside v-if="message" class="toast" :class="`toast-${tone}`" role="status" aria-live="polite">
    <div class="toast-heading"><strong class="toast-title">{{ title }}</strong>
      <button type="button" class="toast-close" :aria-label="closeLabel" @click="emit('dismiss')"><SiteIcon name="close" /></button>
    </div>
    <p class="toast-message">{{ message }}</p>
  </aside>
</template>

<style scoped>
.toast {
  position: fixed;
  right: 20px;
  top: auto;
  bottom: var(--toast-bottom, 20px);
  z-index: 100;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 0;
  width: min(340px, calc(100vw - 32px));
  padding: 12px 38px 12px 14px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--surface);
  box-shadow: var(--shadow-lg);
  color: var(--ink);
  line-height: 1.45;
}
.toast-title { display: block; margin-bottom: 3px; font-size: 13px; line-height: 1.35; }
.toast-heading { display: block; width: 100%; }
.toast-message { margin: 0; font-size: 13px; line-height: 1.45; }
.toast-success { border-color: var(--success); background: var(--success-bg); color: var(--success-text); }
.toast-error { border-color: var(--danger); background: var(--danger-bg); color: var(--danger-text); }
.toast-info { border-color: var(--primary); background: var(--primary-light); color: var(--primary-dark); }
.toast-close { position: absolute; top: 9px; right: 10px; border: 0; padding: 2px; background: transparent; color: currentColor; opacity: .7; font-size: 18px; line-height: 1; cursor: pointer; }
.toast-close:hover { color: var(--ink); }
@media (max-width: 480px) { .toast { right: 16px; bottom: var(--toast-bottom, 16px); width: calc(100vw - 32px); } }
</style>
