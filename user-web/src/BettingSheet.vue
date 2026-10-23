<script setup lang="ts">
import './betting-sheet.css'
import DetailModal from '@lottery/shared/DetailModal.vue'
defineProps<{ title: string; closeLabel: string }>()
defineEmits<{ close: [] }>()
</script>
<template>
  <DetailModal class="betting-sheet" :title="title" :close-label="closeLabel" @close="$emit('close')">
    <slot /><template v-if="$slots.footer" #footer><slot name="footer" /></template>
  </DetailModal>
</template>
<style>
dialog.detail-modal.betting-sheet{width:min(680px,100%);max-width:100%;max-height:calc(100dvh - 16px);margin:auto auto 0;border-radius:var(--radius) var(--radius) 0 0}
.betting-sheet .detail-modal-content{display:grid;gap:8px;padding-bottom:max(12px,env(safe-area-inset-bottom))}
</style>
