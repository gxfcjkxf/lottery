<script lang="ts">
let openModals = 0
let savedOverflow = ""
</script>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = withDefaults(defineProps<{
  title: string
  closeLabel: string
  open?: boolean
}>(), { open: true })
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)
let locked = false

function unlock() {
  if (!locked) return
  openModals--
  if (openModals === 0) document.body.style.overflow = savedOverflow
  locked = false
}
async function syncOpen() {
  await nextTick()
  if (!dialog.value) return
  if (props.open && !dialog.value.open) {
    if (openModals === 0) {
      savedOverflow = document.body.style.overflow
      document.body.style.overflow = 'hidden'
    }
    openModals++
    locked = true
    dialog.value.showModal()
  } else if (!props.open && dialog.value.open) {
    dialog.value.close()
    unlock()
  }
}
function backdrop(event: MouseEvent) {
  if (event.target !== dialog.value || !dialog.value) return
  const rect = dialog.value.getBoundingClientRect()
  if (event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom) emit('close')
}
onMounted(syncOpen)
watch(() => props.open, syncOpen)
onBeforeUnmount(() => { dialog.value?.close(); unlock() })
</script>

<template>
    <dialog ref="dialog" class="detail-modal" :aria-label="title" aria-modal="true" @cancel.prevent="emit('close')" @click="backdrop">
      <header class="detail-modal-heading">
        <h2>{{ title }}</h2>
        <button type="button" class="detail-modal-close" autofocus @click="emit('close')">{{ closeLabel }} <span aria-hidden="true">×</span></button>
      </header>
      <div class="detail-modal-content"><slot /></div>
    </dialog>
</template>

<style scoped>
.detail-modal {
  box-sizing: border-box;
  width: min(1000px, calc(100vw - 48px));
  max-width: calc(100vw - 48px);
  max-height: calc(100dvh - 48px);
  margin: auto;
  padding: 0;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  background: var(--surface);
  color: var(--ink);
  box-shadow: var(--shadow-lg);
  overflow: auto;
  overscroll-behavior: contain;
}
.detail-modal::backdrop { background: rgb(15 23 42 / 45%); backdrop-filter: blur(4px); }
.detail-modal-heading {
  position: sticky;
  top: 0;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 16px 24px;
  border-bottom: 1px solid var(--line);
  background: var(--surface);
}
.detail-modal-heading h2 { margin: 0; font-size: 20px; font-weight: 750; overflow-wrap: anywhere; }
.detail-modal-close { display: inline-flex; align-items: center; gap: 12px; flex-shrink: 0; min-height: 44px; padding: 8px 14px; border: 1px solid var(--line); border-radius: var(--radius-sm); background: var(--surface); color: var(--ink); font: inherit; font-size: 14px; cursor: pointer; }
.detail-modal-close:focus-visible { outline: 2px solid var(--primary); outline-offset: 2px; }
.detail-modal-close span { font-size: 22px; }
.detail-modal-content { min-width: 0; padding: 20px 24px; overflow-wrap: anywhere; }
.detail-modal-content :deep(> *) { max-width: 100%; margin-top: 0; margin-bottom: 0; }
.detail-modal-content :deep(dd) { min-width: 0; overflow-wrap: anywhere; }
.detail-modal-content :deep(pre) { max-width: 100%; overflow: auto; }
.detail-modal-content :deep(.table-wrap) { max-width: 100%; overflow: auto; }
@media (max-width: 620px) {
  .detail-modal { width: calc(100vw - 24px); max-width: calc(100vw - 24px); max-height: calc(100dvh - 24px); }
  .detail-modal-heading { padding: 12px 14px; gap: 8px; }
  .detail-modal-heading h2 { font-size: 18px; }
  .detail-modal-content { padding: 14px; }
}
</style>
