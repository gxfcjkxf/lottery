<script setup lang="ts">
import { computed, provide, ref } from 'vue'
import TablePagination from '../../shared/src/TablePagination.vue'
import DrawArchive from '../../user-web/src/DrawArchive.vue'
import Recharges from '../../platform-web/src/RechargesPanel.vue'
import Children from '../../agent-web/src/pages/ChildrenPage.vue'
import Beneficiaries from '../../admin-web/src/CommissionPaymentTargets.vue'
import { createAdminI18n, adminI18nKey } from '../../admin-web/src/i18n'
import { i18n as agentI18n } from '../../agent-web/src/i18n'
import LoadingOverlay from '../../shared/src/LoadingOverlay.vue'
import { createUserI18n, userI18nKey } from '../../user-web/src/i18n'
import { createPlatformI18n, platformI18nKey } from '../../platform-web/src/i18n'
import { setPlatformDisplaySettings } from '../../shared/src/display-settings'
import '../../shared/src/styles.css'
import '../../shared/src/management.css'
import '../../shared/src/controls.css'
import '../../shared/src/key-value.css'
import '../../shared/src/refresh-button.css'
const query = new URLSearchParams(location.search)
const locale = query.get('locale') === 'zh' ? 'zh-CN' : 'en'
agentI18n.setLocale(locale)
const admin = createAdminI18n(); admin.setLocale(locale); provide(adminI18nKey, admin)
const account = { id: 'aaaaaaaa-aaaa-4aaa-8aaa-000000000001', super_admin: false, brand_ids: ['aaaaaaaa-aaaa-4aaa-8aaa-000000000001'], permissions: ['commission.view.brand'], permissions_by_brand: {'aaaaaaaa-aaaa-4aaa-8aaa-000000000001':['commission.view.brand']} }
const user = createUserI18n(); user.setLocale(locale); provide(userI18nKey, user)
const platform = createPlatformI18n(); platform.setLocale(locale); provide(platformI18nKey, platform)
const display = { timezone:'Asia/Singapore', date_format:'YYYY-MM-DD', time_format:'HH:mm:ss' } as const
setPlatformDisplaySettings(display)
const currentPage = ref(1), total = ref('731')
const size = ref(20)
const count = computed(() => Math.min(50, Math.max(0, Number(total.value) - (currentPage.value - 1) * 50)))
async function goTo(target: number) { currentPage.value = target }
const id = ref('aaaaaaaa-aaaa-4aaa-8aaa-000000000001')
</script>
<template>
  <main class="management-console">
    <div id="user-page-refresh"></div>
    <button id="change-scope" @click="id='bbbbbbbb-bbbb-4bbb-8bbb-000000000001'">Switch scope</button>
    <template v-if="query.get('kind')==='controls'">
      <button id="empty-list" @click="total = '0'">Empty</button>
      <TablePagination :page="currentPage" :count="count" :total="total" :locale="locale" :page-size="50" :previous-disabled="currentPage === 1" :next-disabled="currentPage === 15" :change-page-size="(value: number) => { size = value; currentPage = 1 }" :go-to="goTo" />
    </template>
    <TablePagination v-else-if="query.get('kind')==='compact'" :page="currentPage" :count="size" :total="total" :locale="locale" :page-size="size" :change-page-size="(value: number) => { size = value; currentPage = 1 }" :go-to="goTo" />
    <Children v-else-if="query.get('kind')==='agent'" :key="id" />
    <Beneficiaries v-else-if="query.get('kind')==='brand'" :account="account" :brand-id="id" :payment-id="id" />
    <DrawArchive v-else-if="query.get('kind')==='draw'" :game-id="id" kind="configured" :locale="locale==='en'?'en':'zh'" />
    <Recharges v-else :brand-id="id" member-id="" :locale="locale" :display-settings="display" />
    <LoadingOverlay :label="locale==='en'?'Loading…':'加载中…'" />
  </main>
</template>
