<script setup lang="ts">
import { useOperationToast } from "@lottery/shared";
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { vGlobalLoading } from '@lottery/shared'
import { commissionPolicyPermissions, createCommissionPolicyApi, type CommissionPolicy } from './commission-policy-api'
import { AdminApiError, type AdminAccount } from './admin-api'
import { useAdminI18n } from './i18n'
const props = defineProps<{ account: AdminAccount; brandId: string }>()
const emit = defineEmits<{ (event: 'session-invalid'): void; (event: 'saved'): void }>()
const { t, errorMessage, displayError } = useAdminI18n()
const rights = computed(() => commissionPolicyPermissions(props.account, props.brandId))
const policy = ref<CommissionPolicy | null>(null)
const error = ref<string | import('@lottery/shared').LocalizedMessage>('')
const loading = ref(false)
let generation = 0
async function load() {
 const ticket = ++generation; policy.value = null; error.value = ''; loading.value = true
 if (!rights.value.view || !props.brandId) { loading.value = false; return }
 try { const result = await createCommissionPolicyApi().getPolicy(props.brandId); if(ticket === generation) policy.value = result }
 catch(cause) { if(ticket === generation) { if (cause instanceof AdminApiError && cause.status === 401) emit('session-invalid'); else error.value = errorMessage(cause, 'read') } }
 finally { if(ticket === generation) loading.value = false }
}
onMounted(load)
watch(() => [props.brandId, props.account, rights.value.view], load, { deep: true, flush: 'sync' })
onUnmounted(() => { generation++ })

useOperationToast(() => error.value, value => displayError(value), 'error');
</script>
<template>
 <article class="brand-settings-panel console-stack">
  <h2>{{ t('admin.agents.settlement') }}</h2>

  <p v-if="loading" v-global-loading role="status">{{ t('admin.copy.9b7adaed') }}</p>

  <dl v-if="policy" class="confirm-list">
   <div><dt>{{ t('admin.copy.15c88745') }}</dt><dd>{{ t(policy.config.enabled ? 'admin.copy.50ce1f23' : 'admin.copy.1b7158cb') }}</dd></div>
   <div><dt>{{ t('admin.copy.182e7ede') }}</dt><dd>{{ policy.config.calendar ? t(policy.config.calendar.cycle === 'weekly' ? 'admin.copy.d6951da1' : 'admin.copy.97f41f37') : t('admin.copy.c18d7481') }}</dd></div>
   <div><dt>{{ t('admin.copy.ac042e69') }}</dt><dd>{{ t(policy.config.payout_mode === 'manual' ? 'admin.copy.6644f5f6' : 'admin.copy.63999ab7') }}</dd></div>
  </dl>
 </article>
</template>
