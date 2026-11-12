<script setup lang="ts">
import { onUnmounted, ref, watch } from 'vue'
import PlatformDrawOverview from '../../shared/src/PlatformDrawOverview.vue'
import SelectControl from '@lottery/shared/SelectControl.vue'
import ManagementList from '@lottery/shared/ManagementList.vue'
import { createPlatformLotteryClient, PlatformLotteryError, type PlatformGame } from '../../shared/src/platform-lottery'
import { withGlobalLoading, vRefreshButton, vGlobalLoading } from '@lottery/shared'
import { createCatalogI18n } from '@lottery/shared'
import { drawOverviewCatalog } from '../../shared/src/draw-overview-catalog'
const props = defineProps<{ brandId: string; locale: 'en' | 'zh-CN' }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const i18n = createCatalogI18n(drawOverviewCatalog, props.locale)
watch(() => props.locale, value => i18n.setLocale(value))
const t = i18n.t
const games = ref<PlatformGame[]>([]), gameId = ref(''), failed = ref(false), busy = ref(false)
const api = createPlatformLotteryClient('admin', () => props.brandId)
let generation = 0, alive = true
async function load() {
 const ticket = ++generation
 games.value = []; gameId.value = ''; failed.value = false; busy.value = true
 try {
  const rows: PlatformGame[] = []
  for (let offset = 0; ; offset += 100) {
   const page = await api.list(offset, 100)
   rows.push(...page.games.filter(game => game.connected))
   if (offset + page.games.length >= Number(page.total_count)) break
  }
  if (!alive || ticket !== generation) return
  games.value = rows
  if (rows.length) gameId.value = rows[0]!.id
 } catch (cause) {
  if (!alive || ticket !== generation) return
  failed.value = true
  if (cause instanceof PlatformLotteryError && cause.status === 401) emit('failure', cause)
 } finally { if (alive && ticket === generation) busy.value = false }
}
watch(() => props.brandId, load, { immediate: true })
onUnmounted(() => { alive = false; generation++ })
</script>
<template>
 <PlatformDrawOverview v-if="gameId" :key="`${brandId}:${gameId}`" :game-id="gameId" :locale="locale" surface="admin" :brand-id="brandId" :can-manage="false" @failure="emit('failure', $event)">
  <template #filters><SelectControl v-model="gameId" :label="t('game')" :options="games.map(game => ({value: game.id, label: game.name}))" /></template>
 </PlatformDrawOverview>
 <ManagementList v-else :busy="busy">
  <p v-if="failed" class="message error" role="alert">{{ t('initialFailure') }} <button class="button button-secondary" v-refresh-button="t('refresh')" :disabled="busy" @click="withGlobalLoading(load)">{{ t('refresh') }}</button></p>
  <p v-else-if="busy" v-global-loading role="status">{{ t('loading') }}</p>
  <p v-else class="empty-state" role="status">{{ t('noGames') }}</p>
 </ManagementList>
</template>
