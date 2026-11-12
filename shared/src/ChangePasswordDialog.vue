<script setup lang="ts">
import { vWriteGuard, vInlineValidation, withGlobalLoading } from './index'
import WriteFormActions from './WriteFormActions.vue'
import DetailModal from './DetailModal.vue'
import { ref, watch, useId } from 'vue'
import { formValidationText } from './form-validation'
const props = defineProps<{ title: string; closeLabel: string; currentLabel: string; newLabel: string; confirmLabel: string; submitLabel: string; busy: boolean; frozen?: boolean; submit: () => Promise<void> }>()
const current = defineModel<string>('current', { required: true })
const next = defineModel<string>('next', { required: true })
const confirmation = defineModel<string>('confirmation', { required: true })
defineEmits<{ close: [] }>()
const mismatch = ref(false)
const helpId = useId()
watch([next, confirmation], () => { mismatch.value = false })
async function save() {
  mismatch.value = next.value !== confirmation.value
  if (!mismatch.value) await props.submit()
}
</script>
<template>
  <DetailModal class="console-account-dialog ui-form-dialog" :title="title" :close-label="closeLabel" :close-disabled="busy || frozen" @close="!busy && !frozen && $emit('close')">
    <form v-write-guard="busy" novalidate v-inline-validation class="login-entry__form ui-form" @submit.prevent="withGlobalLoading(save)">
      <fieldset class="am-form" :disabled="busy || frozen">
        <label class="login-entry__field ui-field-row"><span>{{ currentLabel }}</span><input v-model="current" class="field" type="password" autocomplete="current-password" required /></label>
        <label class="login-entry__field ui-field-row"><span>{{ newLabel }}</span><input v-model="next" :aria-label="newLabel" :aria-describedby="helpId" class="field" type="password" autocomplete="new-password" required data-min-characters="6" data-max-characters="30" /><small :id="helpId">{{formValidationText('passwordHelp')}}</small></label>
        <label class="login-entry__field ui-field-row"><span>{{ confirmLabel }}</span><input v-model="confirmation" class="field" type="password" autocomplete="new-password" required :aria-invalid="mismatch || undefined" /></label>
      </fieldset>
      <slot />
      <p v-if="mismatch" class="form-error" role="alert">{{formValidationText('passwordMismatch')}}</p>
      <WriteFormActions :submit-label="submitLabel" :busy="busy" :invalid="false" />
    </form>
  </DetailModal>
</template>
