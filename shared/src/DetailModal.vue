<script lang="ts">
let openModals = 0
let savedOverflow = ""
</script>

<script setup lang="ts">
import SiteIcon from "./SiteIcon";
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
      <header class="modal-heading detail-modal-heading">
        <h2>{{ title }}</h2>
        <button type="button" class="icon-button detail-modal-close" :aria-label="closeLabel" autofocus @click="emit('close')"><SiteIcon name="close" /></button>
      </header>
      <div class="detail-modal-content"><slot /></div>
      <div v-if="$slots.footer" class="detail-modal-footer"><slot name="footer" /></div>
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
  display: flex;
  flex-direction: column;
  overflow: hidden;
  overscroll-behavior: contain;
}
.detail-modal:not([open]) { display: none; }
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
.detail-modal-close { flex-shrink: 0; }
.detail-modal-content { flex: 1 1 auto; min-width: 0; min-height: 0; padding: 20px 24px; overflow: auto; overflow-wrap: anywhere; }
.detail-modal-footer {
  flex: 0 0 auto;
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 14px 24px;
  border-top: 1px solid var(--line);
  background: var(--surface);
}
.detail-modal-content :deep(> *) { max-width: 100%; margin-top: 0; margin-bottom: 0; }
.detail-modal-content :deep(dd) { min-width: 0; overflow-wrap: anywhere; }
.detail-modal-content :deep(pre) { max-width: 100%; overflow: auto; }
.detail-modal-content :deep(.table-wrap) { max-width: 100%; overflow: auto; }
@media (max-width: 620px) {
  .detail-modal { width: calc(100vw - 24px); max-width: calc(100vw - 24px); max-height: calc(100dvh - 24px); }
  .detail-modal-heading { padding: 12px 14px; gap: 8px; }
  .detail-modal-heading h2 { font-size: 18px; }
  .detail-modal-content { padding: 14px; }
  .detail-modal-footer { padding: 12px 14px; }
}
</style>
