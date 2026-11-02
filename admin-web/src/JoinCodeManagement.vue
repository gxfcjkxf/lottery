<script setup lang="ts">
import { useOperationToast } from "@lottery/shared";
import { computed, ref, watch } from 'vue'
import { vGlobalLoading } from '@lottery/shared'
import { AdminApiError, type AdminAccount } from './admin-api'
import { createJoinCodesApi, joinCodePermissions, type JoinCode, type JoinCodeKind } from './join-codes-api'
import { useAdminI18n } from './i18n'

const props = defineProps<{ account: AdminAccount; brandId: string; memberId: string; kind: JoinCodeKind }>()
const emit = defineEmits<{ 'session-invalid': [] }>()
const { t, errorMessage, displayError } = useAdminI18n()
const api = createJoinCodesApi()
const canView = computed(() => joinCodePermissions(props.account, props.brandId).view)
const code = ref<JoinCode | null>(null)
const loading = ref(false)
const error = ref<ReturnType<typeof errorMessage> | ''>('')
watch(() => [props.account.id, props.brandId, props.memberId, props.kind, canView.value] as const, async (_, __, onCleanup) => {
 let active = true
 onCleanup(() => { active = false })
 code.value = null; error.value = ''; loading.value = false
 if (!canView.value) return
 const { brandId, memberId, kind } = props
 loading.value = true
 try {
  const page = await api.list(brandId, { kind, owner_member_id: memberId }, 20, 0)
  if (!active) return
  if (page.brand_id !== brandId || page.kind !== kind || page.owner_member_id !== memberId || page.offset !== 0 || page.limit !== 20 ||
      !['0', '1'].includes(page.total_count) || page.items.length !== Number(page.total_count)) {
   throw new AdminApiError('Invalid bound join code response', 502, 'INVALID_RESPONSE')
  }
  code.value = page.items[0] ?? null
 } catch (cause) {
  if (!active) return
  error.value = errorMessage(cause, 'read')
  if (cause instanceof AdminApiError && cause.status === 401) emit('session-invalid')
 } finally { if (active) loading.value = false }
}, { immediate: true })

useOperationToast(() => error.value, value => displayError(value), 'error');
</script>

<template>
 <dl v-if="canView" class="confirm-list owner-join-code">
  <div>
   <dt>{{ t(kind === 'agent' ? 'admin.copy.ae9fa0bf' : 'admin.copy.d245e2d6') }}</dt>
   <dd>
    <span v-if="loading" v-global-loading role="status" class="muted">{{ t('admin.copy.439fda03') }}</span>
    <template v-else-if="error" />
    <span v-else>{{ code ? code.code : '—' }}</span>
   </dd>
  </div>
 </dl>
</template>
