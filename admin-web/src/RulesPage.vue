<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, reactive, ref } from 'vue'
import PlatformLottery from '@lottery/shared/PlatformLottery.vue'
import ConfiguredRuleWorkbench from '@lottery/shared/ConfiguredRuleWorkbench.vue'
import { useAdminI18n } from './i18n'
import type { AdminAccount } from './admin-api'
import { brandPermissionSet } from './brand-permissions'

const RuleVersions = defineAsyncComponent(() => import('./RuleVersions.vue'))
const RuleSimulator = defineAsyncComponent(() => import('./RuleSimulator.vue'))
const props = defineProps<{ account: AdminAccount; brandId: string }>()
const emit = defineEmits<{ 'session-invalid': [] }>()
const { t, locale } = useAdminI18n()
type View = 'catalog' | 'ownGames' | 'simulator' | 'workbench'
const view = ref<View>('catalog')
// Mount tools on first entry and retain drafts and unresolved request receipts on return.
const visited = reactive({ ownGames: false, simulator: false, workbench: false })
const permissions = computed(() => brandPermissionSet(props.account, props.brandId))
const allowed = (permission: string) => permissions.value.has(permission)
const canViewPlatformGames = computed(() => allowed('platform_game.view.brand'))
let catalogScroll = 0
async function navigate(next: View) {
  if (view.value === 'catalog') catalogScroll = window.scrollY
  if (next !== 'catalog') visited[next] = true
  view.value = next
  await nextTick()
  window.scrollTo(0, next === 'catalog' ? catalogScroll : 0)
}
</script>

<template>
  <div class="rules-page" data-testid="brand-rules-page">
    <div v-show="view === 'catalog'">
      <PlatformLottery v-if="canViewPlatformGames" surface="admin" :brand-id="brandId" :locale="locale" :can-manage="false" :can-connect="allowed('platform_game.connect.brand')" :can-sell="allowed('platform_game.sell.brand')" @failure="emit('session-invalid')">
        <template #catalog-header>
          <nav class="rules-page-actions">
            <button type="button" class="button button-secondary" @click="navigate('ownGames')">{{ t('admin.rules.ownGames') }}</button>
            <button type="button" class="button button-secondary" @click="navigate('simulator')">{{ t('admin.rules.simulator') }}</button>
          </nav>
        </template>
      </PlatformLottery>
      <nav v-else class="rules-page-actions">
        <button type="button" class="button button-secondary" @click="navigate('ownGames')">{{ t('admin.rules.ownGames') }}</button>
        <button type="button" class="button button-secondary" @click="navigate('simulator')">{{ t('admin.rules.simulator') }}</button>
      </nav>
    </div>
    <header v-if="view !== 'catalog'" class="rules-page-heading">
      <button type="button" class="button button-secondary" @click="navigate(view === 'workbench' ? 'ownGames' : 'catalog')">{{ t(view === 'workbench' ? 'admin.rules.backOwn' : 'admin.rules.back') }}</button>
      <button v-if="view === 'ownGames'" type="button" class="button button-secondary" @click="navigate('workbench')">{{ t('admin.rules.workbench') }}</button>
    </header>
    <RuleVersions v-if="visited.ownGames" v-show="view === 'ownGames'" :account="account" :brand-id="brandId" @session-invalid="emit('session-invalid')" />
    <RuleSimulator v-if="visited.simulator" v-show="view === 'simulator'" :account="account" :brand-id="brandId" @session-invalid="emit('session-invalid')" />
    <ConfiguredRuleWorkbench v-if="visited.workbench" v-show="view === 'workbench'" surface="admin" :brand-id="brandId" :locale="locale" expanded @failure="emit('session-invalid')" />
  </div>
</template>

<style scoped>
.rules-page{min-width:0}
.rules-page-actions,.rules-page-heading{display:flex;align-items:center;flex-wrap:wrap;gap:12px;padding:12px 16px}
.rules-page-heading{padding:0;margin-bottom:16px}
.rules-page .button{min-height:44px}
</style>
