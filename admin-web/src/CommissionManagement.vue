<script setup lang="ts">
import { ref } from 'vue'
import type { AdminAccount } from './admin-api'
import { useAdminI18n } from './i18n'
import CommissionCyclesManagement from './CommissionCyclesManagement.vue'
import CommissionPaymentsManagement from './CommissionPaymentsManagement.vue'
import CommissionCorrectionsManagement from './CommissionCorrectionsManagement.vue'

defineProps<{ account: AdminAccount; brandId: string; brandStatus?: string }>()
const emit = defineEmits<{ 'session-invalid': [] }>()
const { t } = useAdminI18n()
const showSettings = ref(false)
</script>

<template>
  <CommissionCyclesManagement :account="account" :brand-id="brandId" :brand-status="brandStatus"
    @session-invalid="emit('session-invalid')">
    <template #actions>
      <button type="button" class="button" aria-haspopup="dialog" @click="showSettings = true">{{ t('admin.commission.ui.settings') }}</button>
      <CommissionCorrectionsManagement v-if="showSettings" settings-only
        :account="account" :brand-id="brandId" :brand-status="brandStatus"
        @close-settings="showSettings = false" @session-invalid="emit('session-invalid')" />
    </template>
    <template #payments="{ cycleId, setLocked }">
      <CommissionPaymentsManagement :account="account" :brand-id="brandId" :brand-status="brandStatus" :cycle-id="cycleId"
        @writing-change="setLocked" @session-invalid="emit('session-invalid')" />
    </template>
    <template #corrections="{ cycleId, setLocked }">
      <CommissionCorrectionsManagement :account="account" :brand-id="brandId" :brand-status="brandStatus" :cycle-id="cycleId"
        @writing-change="setLocked" @session-invalid="emit('session-invalid')" />
    </template>
  </CommissionCyclesManagement>
</template>
