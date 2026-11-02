<script setup lang="ts">
import { provide, ref } from 'vue'
import { setPlatformDisplaySettings } from '../../shared/src/display-settings'
import OperationsWorkbench from '../../admin-web/src/OperationsWorkbench.vue'
import BrandPresentation from '../../admin-web/src/BrandPresentation.vue'
import CompliancePolicy from '../../admin-web/src/CompliancePolicy.vue'
import Templates from '../../platform-web/src/NotificationTemplatesPanel.vue'
import LoadingOverlay from '../../shared/src/LoadingOverlay.vue'
import { createAdminI18n, adminI18nKey } from '../../admin-web/src/i18n'
import { createPlatformI18n, platformI18nKey } from '../../platform-web/src/i18n'
const props = defineProps<{ locale: 'en' | 'zh-CN'; view: 'presentation' | 'compliance' | 'templates' | 'workbench' }>()
const admin = createAdminI18n(); admin.setLocale(props.locale); provide(adminI18nKey, admin)
const platform = createPlatformI18n(); platform.setLocale(props.locale); provide(platformI18nKey, platform)
const mounted = ref(true)
const brand = '11111111-1111-4111-8111-111111111111'
const grants = ['commission.view.brand', 'brand_presentation.view.brand', 'brand_presentation.write.brand', 'compliance_policy.view.brand', 'compliance_policy.write.brand', 'compliance_check.run.brand']
const account = { id: brand, super_admin: false, brand_ids: [brand], permissions: [], permissions_by_brand: { [brand]: grants } }
const displaySettings = { timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD' as const, time_format: 'HH:mm:ss' as const }
setPlatformDisplaySettings(displaySettings)
</script>
<template>
 <div class="management-console console-stack" style="padding:12px">
  <button data-testid="toggle-settings" class="button" @click="mounted = !mounted">Toggle fixture</button>
  <template v-if="mounted">
   <BrandPresentation v-if="view === 'presentation'" :account="account" :brand-id="brand" />
   <CompliancePolicy v-else-if="view === 'compliance'" :account="account" :brand-id="brand" />
   <OperationsWorkbench v-else-if="view === 'workbench'" :account="account" :brand-id="brand" />
   <Templates v-else :account-id="account.id" can-manage :locale="locale" :display-settings="displaySettings" />
  </template>
 </div>
 <LoadingOverlay label="Loading…" />
</template>
