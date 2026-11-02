<script setup lang="ts">
import { computed, watch } from 'vue'
import Toast from './Toast.vue'
import { operationToasts, dismissOperationToast } from './operation-toast'
import { activeModalDialog } from './modal-state'
import { createCatalogI18n } from './localization'
const groups = computed(() => (['corner', 'center'] as const).map(position => ({ position, notices: operationToasts.value.filter(notice => notice.position === position) })))
const props = defineProps<{ locale: 'en' | 'zh-CN' }>()
const i18n = createCatalogI18n({ en: { title: 'Operation result', close: 'Dismiss message' }, 'zh-CN': { title: '操作结果', close: '关闭提示' } }, props.locale)
watch(() => props.locale, value => i18n.setLocale(value))
</script>
<template>
 <Teleport :to="activeModalDialog ?? 'body'">
  <template v-for="group in groups" :key="group.position">
   <div v-if="group.notices.length" class="operation-toast-stack" :class="{'operation-toast-center': group.position === 'center'}">
    <Toast v-for="notice in group.notices" :key="notice.id" :tone="notice.tone" :duration="notice.duration" :text-only="group.position === 'center'" :title="i18n.t('title')" :message="notice.text()" :close-label="i18n.t('close')" @dismiss="dismissOperationToast(notice.id)" />
   </div>
  </template>
 </Teleport>
</template>
<style scoped>
.operation-toast-stack { pointer-events:none; position:fixed; right:20px; bottom:max(20px, env(safe-area-inset-bottom)); z-index:100; display:flex; flex-direction:column; gap:8px; width:min(340px,calc(100vw - 32px)); max-height:calc(100dvh - 40px - env(safe-area-inset-bottom)); overflow-y:auto; }
.operation-toast-stack :deep(.toast) { position:relative; inset:auto; flex-shrink:0; width:100%; max-width:none; }
@media(max-width:480px) { .operation-toast-stack { right:16px; bottom:max(16px, env(safe-area-inset-bottom)); max-height:calc(100dvh - 32px - env(safe-area-inset-bottom)); } }
.operation-toast-stack.operation-toast-center { left:50%; top:50%; right:auto; bottom:auto; transform:translate(-50%,-50%); max-height:calc(100dvh - 32px); }
</style>
