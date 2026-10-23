<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref } from 'vue'
import SiteIcon from './SiteIcon'
const props = defineProps<{ id: string; label: string }>()
const trigger = ref<HTMLButtonElement | null>(null), menu = ref<HTMLDivElement | null>(null), open = ref(false)
function close() { if (menu.value?.matches(':popover-open')) menu.value.hidePopover() }
async function toggle() {
  if (!menu.value || !trigger.value) return
  if (menu.value.matches(':popover-open')) { close(); return }
  menu.value.showPopover()
  await nextTick()
  position()
  menu.value.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus({ preventScroll: true })
}
function position() {
  if (!trigger.value || !menu.value?.matches(':popover-open')) return
  const anchor = trigger.value.getBoundingClientRect(), box = menu.value.getBoundingClientRect()
  menu.value.style.left = `${Math.max(12, Math.min(anchor.right - box.width, innerWidth - box.width - 12))}px`
  menu.value.style.top = `${anchor.bottom + box.height + 12 <= innerHeight ? anchor.bottom + 4 : Math.max(12, anchor.top - box.height - 4)}px`
}
function navigate(event: KeyboardEvent) {
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key) || !menu.value) return
  event.preventDefault()
  const items = [...menu.value.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')]
  const index = items.indexOf(document.activeElement as HTMLButtonElement)
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 : (index + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length
  items[next]?.focus()
}
function syncOpen(event: Event) {
  open.value = (event as ToggleEvent).newState === 'open'
  if (open.value) { window.addEventListener('scroll', position, true); window.addEventListener('resize', position) }
  else cleanup()
}
function cleanup() { window.removeEventListener('scroll', position, true); window.removeEventListener('resize', position) }
onBeforeUnmount(() => { close(); cleanup() })
</script>
<template>
  <button ref="trigger" class="icon-button member-menu-trigger" type="button" :aria-label="props.label" aria-haspopup="menu" :aria-expanded="open" :aria-controls="id" @click="toggle"><SiteIcon name="moreVertical" /></button>
  <div ref="menu" :id="id" popover="auto" class="member-action-menu" role="menu" :aria-label="props.label" @toggle="syncOpen" @keydown="navigate" @click="close"><slot /></div>
</template>
<style scoped>
.member-action-menu { position:fixed; inset:auto; margin:0; padding:6px; width:max-content; max-width:calc(100vw - 24px); border:1px solid var(--line); border-radius:var(--radius-sm); background:var(--surface); color:var(--ink); box-shadow:var(--shadow-lg); }
.member-action-menu:popover-open { display:grid; gap:4px; }
.member-action-menu :deep(button) { display:flex; align-items:center; width:100%; min-height:44px; padding:8px 12px; border:0; border-radius:var(--radius-sm); background:transparent; color:inherit; font:inherit; text-align:start; }
.member-action-menu :deep(button.danger-text) { color:var(--danger-text); }
.member-action-menu :deep(button:hover),.member-action-menu :deep(button:focus-visible) { background:var(--primary-light); color:var(--primary-dark); }
</style>
