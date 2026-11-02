<script setup lang="ts">
import { computed, provide, watch } from 'vue'
import BrandDomains from '../../admin-web/src/BrandDomains.vue'
import { adminI18nKey, createAdminI18n } from '../../admin-web/src/i18n'
import type { AdminAccount } from '../../admin-web/src/admin-api'
import type { PlatformAccount, PlatformBrand } from './platform-api'
const props = defineProps<{ account: PlatformAccount; brand: PlatformBrand; locale: 'en' | 'zh-CN' }>()
const emit = defineEmits<{ 'session-invalid': [] }>()
const i18n = createAdminI18n()
provide(adminI18nKey, i18n)
watch(() => props.locale, locale => i18n.setLocale(locale), { immediate: true })
const domainAccount = computed<AdminAccount>(() => ({ ...props.account, brand_ids: [], permissions: [] }))
</script>
<template>
  <BrandDomains :key="`${account.id}:${brand.id}`" :account="domainAccount" :brand-id="brand.id" :brand-status="brand.status" @session-invalid="emit('session-invalid')" />
</template>
