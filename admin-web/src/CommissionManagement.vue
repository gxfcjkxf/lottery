<script setup lang="ts">
import { ref, watch } from 'vue'
import { withGlobalLoading } from '@lottery/shared'
import DetailModal from '@lottery/shared/DetailModal.vue'
import type { AdminAccount } from './admin-api'
import { useAdminI18n } from './i18n'
import CommissionCyclesManagement from './CommissionCyclesManagement.vue'
import CommissionPaymentsManagement from './CommissionPaymentsManagement.vue'
import CommissionCorrectionsManagement from './CommissionCorrectionsManagement.vue'

const props = defineProps<{ account: AdminAccount; brandId: string; brandStatus?: string }>()
const emit = defineEmits<{
  'session-invalid': []
}>()
const { t } = useAdminI18n()
const action = ref<{ kind: 'payments' | 'corrections'; cycleId: string } | null>(null)
const writing = ref(false)
const cycles = ref<InstanceType<typeof CommissionCyclesManagement> | null>(null)
async function openCycle(cycleId: string) {
  if (writing.value) return
  action.value = null
  await cycles.value?.openCycle(cycleId)
}
function closeAction() { if (!writing.value) action.value = null }
watch(() => [props.account.id, props.brandId], () => { action.value = null; writing.value = false })
</script>

<template>
  <CommissionCyclesManagement ref="cycles" :account="account" :brand-id="brandId" :brand-status="brandStatus" :covered="!!action"
    @open-payments="cycleId => action = { kind: 'payments', cycleId }"
    @open-corrections="cycleId => action = { kind: 'corrections', cycleId }"
    @session-invalid="emit('session-invalid')" />
  <DetailModal v-if="action" class="commission-action-dialog" :title="t(action.kind === 'payments' ? 'admin.copy.61a05916' : 'admin.commission.ui.corrections')"
    :close-label="t('association.close')" :close-disabled="writing" @close="closeAction">
    <CommissionPaymentsManagement v-if="action.kind === 'payments'" :key="action.cycleId"
      :account="account" :brand-id="brandId" :brand-status="brandStatus" :cycle-id="action.cycleId"
      @writing-change="writing = $event" @open-cycle="withGlobalLoading(openCycle, $event)" @session-invalid="emit('session-invalid')" />
    <CommissionCorrectionsManagement v-else :key="action.cycleId"
      :account="account" :brand-id="brandId" :brand-status="brandStatus" :cycle-id="action.cycleId"
      @writing-change="writing = $event" @session-invalid="emit('session-invalid')" />
  </DetailModal>
</template>

<style scoped>
.commission-action-dialog:focus { outline: none; }
</style>
