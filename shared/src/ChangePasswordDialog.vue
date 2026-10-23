<script setup lang="ts">
import { vWriteGuard, vInlineValidation, withGlobalLoading } from './index'
import WriteFormActions from './WriteFormActions.vue'
import DetailModal from './DetailModal.vue'
defineProps<{ title: string; closeLabel: string; currentLabel: string; newLabel: string; confirmLabel: string; cancelLabel: string; submitLabel: string; busy: boolean; frozen?: boolean; submit: () => Promise<void> }>()
const current = defineModel<string>('current', { required: true })
const next = defineModel<string>('next', { required: true })
const confirmation = defineModel<string>('confirmation', { required: true })
defineEmits<{ close: [] }>()
</script>
<template>
  <DetailModal class="console-account-dialog" :title="title" :close-label="closeLabel" @close="!busy && !frozen && $emit('close')">
    <form v-write-guard="busy" novalidate v-inline-validation class="login-entry__form" @submit.prevent="withGlobalLoading(submit)">
      <fieldset class="am-form" :disabled="busy || frozen">
        <label class="login-entry__field">{{ currentLabel }}<input v-model="current" class="field" type="password" autocomplete="current-password" required /></label>
        <label class="login-entry__field">{{ newLabel }}<input v-model="next" class="field" type="password" autocomplete="new-password" required /></label>
        <label class="login-entry__field">{{ confirmLabel }}<input v-model="confirmation" class="field" type="password" autocomplete="new-password" required /></label>
      </fieldset>
      <slot />
      <WriteFormActions :submit-label="submitLabel" :cancel-label="cancelLabel" :busy="busy" :frozen="Boolean(frozen)" :invalid="false" @cancel="$emit('close')" />
    </form>
  </DetailModal>
</template>
