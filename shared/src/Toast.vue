<script setup lang="ts">
import SiteIcon from "./SiteIcon";
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'

const props = withDefaults(defineProps<{ title: string; message: string; closeLabel: string; tone?: 'success' | 'error' | 'info'; duration?: number; textOnly?: boolean }>(), {
  tone: 'success',
  duration: 3600,
})
const emit = defineEmits<{ dismiss: [] }>()
let timer: ReturnType<typeof setTimeout> | undefined
const toast = ref<HTMLElement | null>(null)

function pauseDismiss() { if (timer) clearTimeout(timer) }

function scheduleDismiss() {
  pauseDismiss()
  if (props.duration <= 0 || toast.value?.matches(':hover, :focus-within')) return
  timer = setTimeout(() => emit('dismiss'), props.duration)
}

watch(() => props.message, value => { if (value) scheduleDismiss() }, { immediate: true })
onBeforeUnmount(() => { if (timer) clearTimeout(timer) })
</script>

<template>
  <aside v-if="message" ref="toast" class="toast" :class="[`toast-${tone}`, { 'toast-text-only': textOnly }]" :role="tone === 'error' ? 'alert' : 'status'" :aria-live="tone === 'error' ? 'assertive' : 'polite'" aria-atomic="true" @mouseenter="pauseDismiss" @mouseleave="scheduleDismiss" @focusin="pauseDismiss" @focusout="nextTick(scheduleDismiss)">
    <div v-if="!textOnly" class="toast-heading"><strong class="toast-title">{{ title }}</strong>
      <button type="button" class="toast-close" :aria-label="closeLabel" @click="emit('dismiss')"><SiteIcon name="close" /></button>
    </div>
    <p class="toast-message">{{ message }}</p>
  </aside>
</template>

<style scoped>
.toast {
  pointer-events: none;
  box-sizing: border-box;
  overflow-wrap: anywhere;
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
  padding: 12px 54px 12px 14px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--surface);
  box-shadow: var(--shadow-lg);
  color: var(--ink);
  line-height: 1.45;
}
.toast-title { display: block; margin-bottom: 3px; font-size: var(--body-font-size); line-height: 1.35; }
.toast-heading { display: block; width: 100%; }
.toast-message { margin: 0; font-size: var(--body-font-size); line-height: 1.45; }
.toast-success { border-color: var(--success); background: var(--success-bg); color: var(--success-text); }
.toast-error { border-color: var(--danger); background: var(--danger-bg); color: var(--danger-text); }
.toast-info { border-color: var(--primary); background: var(--primary-light); color: var(--primary-dark); }
.toast-close { pointer-events: auto; position: absolute; top: 2px; right: 4px; display: grid; place-items: center; width: 44px; height: 44px; border: 0; padding: 10px; background: transparent; color: currentColor; opacity: .7; font-size: 18px; line-height: 1; cursor: pointer; }
.toast.toast-text-only { align-items:center; padding:12px 14px; border:0; background:var(--surface); box-shadow:none; text-align:center; }
.toast-close:hover { color: var(--ink); }
@media (max-width: 480px) { .toast { right: 16px; bottom: var(--toast-bottom, 16px); width: calc(100vw - 32px); } }
</style>
