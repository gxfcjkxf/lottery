<script lang="ts">
let openModals = 0
let savedOverflow = ""
</script>

<script setup lang="ts">
import SiteIcon from "./SiteIcon";
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { modalDialogs } from './modal-state'

const props = withDefaults(defineProps<{
  title: string
  titleId?: string
  closeLabel: string
  closeDisabled?: boolean
  open?: boolean
}>(), { open: true })
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)
let locked = false
let returnFocus: HTMLElement | null = null

function unlock() {
  if (!locked) return
  openModals--
  const index = modalDialogs.indexOf(dialog.value!)
  if (index !== -1) modalDialogs.splice(index, 1)
  if (openModals === 0) document.body.style.overflow = savedOverflow
  locked = false
}
async function syncOpen() {
  await nextTick()
  if (!dialog.value) return
  if (props.open && !dialog.value.open) {
    returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
    if (openModals === 0) {
      savedOverflow = document.body.style.overflow
      document.body.style.overflow = 'hidden'
    }
    openModals++
    locked = true
    dialog.value.showModal()
    modalDialogs.push(dialog.value)
  } else if (!props.open && dialog.value.open) {
    dialog.value.close()
    unlock()
  }
}
onMounted(syncOpen)
watch(() => props.open, syncOpen)
onBeforeUnmount(() => {
  dialog.value?.close()
  unlock()
  if (returnFocus?.isConnected) returnFocus.focus({ preventScroll: true })
})
</script>

<template>
    <dialog ref="dialog" class="detail-modal" :aria-label="title" aria-modal="true" @cancel.prevent="!closeDisabled && emit('close')">
      <header class="modal-heading detail-modal-heading">
        <h2 :id="titleId">{{ title }}</h2>
        <button type="button" class="icon-button detail-modal-close" :aria-label="closeLabel" :disabled="closeDisabled" autofocus @click="emit('close')"><SiteIcon name="close" /></button>
      </header>
      <div class="detail-modal-content"><slot /></div>
      <div v-if="$slots.footer" class="detail-modal-footer ui-form-actions"><slot name="footer" /></div>
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
.detail-modal.confirmation-dialog { width: min(680px, calc(100vw - 24px)); }
.detail-modal.ui-form-dialog { width:min(520px,calc(100vw - 24px)); max-width:calc(100vw - 24px); }
.detail-modal::backdrop { background: rgb(15 23 42 / 45%); backdrop-filter: blur(4px); }
.detail-modal-heading {
  flex: 0 0 auto;
  position: sticky;
  top: 0;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin: 0;
  padding: 8px 16px;
  border-bottom: 1px solid var(--line);
  background: var(--surface);
}
.detail-modal-heading h2 { margin: 0; font-size: 20px; font-weight: 750; overflow-wrap: anywhere; }
.detail-modal .detail-modal-close { --site-icon-size:16px; display:inline-flex; align-items:center; justify-content:center; flex-shrink:0; width:var(--control-height); height:var(--control-height); min-width:var(--control-height); min-height:var(--control-height); padding:0; border:0; border-radius:var(--radius-sm); background:transparent; color:var(--muted); box-shadow:none; cursor:pointer; transition:background-color 150ms,color 150ms; }
.detail-modal .detail-modal-close:hover { background:var(--canvas); color:var(--ink); }
.detail-modal-content { flex: 1 1 auto; min-width: 0; min-height: 0; margin: 0; padding: 12px 16px; overflow: auto; overflow-wrap: anywhere; }
.detail-modal-footer {
  flex: 0 0 auto;
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 10px;
  margin: 0;
  padding: 12px 16px;
  border-top: 1px solid var(--line);
  background: var(--surface);
}
.detail-modal-footer :deep(button) { min-width: 0; max-width: 100%; white-space: normal; overflow-wrap: anywhere; }
.detail-modal-content :deep(> *) { max-width: 100%; margin-top: 0; margin-bottom: 0; }
.detail-modal-content :deep(dd) { min-width: 0; overflow-wrap: anywhere; }
.detail-modal-content :deep(pre) { max-width: 100%; overflow: auto; }
.detail-modal-content :deep(.table-wrap) { max-width: 100%; overflow: auto; }
@media (max-width: 620px) {
  .detail-modal { width: calc(100vw - 24px); max-width: calc(100vw - 24px); max-height: calc(100dvh - 24px); }
  .detail-modal-heading { padding: 8px 14px; gap: 8px; }
  .detail-modal-heading h2 { font-size: 18px; }
  .detail-modal-content { padding: 12px 14px; }
  .detail-modal-footer { padding: 12px 14px; }
}
</style>
