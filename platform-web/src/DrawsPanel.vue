<script setup lang="ts">
import SelectControl from '@lottery/shared/SelectControl.vue'
import { onUnmounted, ref, computed, watch } from 'vue'
import PlatformLottery from '@lottery/shared/PlatformLottery.vue'
import { createPlatformLotteryClient, platformLotteryCatalog, type PlatformGame } from '../../shared/src/platform-lottery'
import { createCatalogI18n, withGlobalLoading, vRefreshButton } from '@lottery/shared'
import { drawOverviewCatalog } from '../../shared/src/draw-overview-catalog'
import type { PlatformDisplaySettings } from './platform-format'
const props = defineProps<{ locale: 'en' | 'zh-CN'; displaySettings: PlatformDisplaySettings; canView: boolean; canManage: boolean }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const i18n = createCatalogI18n(platformLotteryCatalog, props.locale), overviewI18n = createCatalogI18n(drawOverviewCatalog, props.locale)
watch(() => props.locale, value => { i18n.setLocale(value); overviewI18n.setLocale(value) })
const games = ref<PlatformGame[]>([]), selected = ref(''), busy = ref(false), failed = ref(false)
const platformId = computed(() => selected.value.startsWith('platform:') ? selected.value.slice(9) : '')
let alive = true, generation = 0
async function load() {
 const g = ++generation; busy.value = true; failed.value = false
 try {
  const api = createPlatformLotteryClient('platform', () => ''), items: PlatformGame[] = []
  for (let offset = 0; ; offset += 50) { const page = await api.list(offset); items.push(...page.games.filter(game => game.status !== 'deleted')); if (page.games.length < 50) break }
  if (alive && g === generation) games.value = items
 } catch (cause) { if (alive && g === generation) { failed.value = true; emit('failure', cause) } }
 finally { if (alive && g === generation) busy.value = false }
}
watch(() => props.canView, value => { if (value) void load(); else { generation++; games.value = []; selected.value = '' } }, { immediate: true })
onUnmounted(() => { alive = false; generation++ })
</script>
<template>
<section data-testid="platform-draws-panel">
 <div class="brand-picker draws-game-picker"><label>{{ overviewI18n.t('game') }}<SelectControl v-model="selected" :disabled="busy" :label="overviewI18n.t('game')" :options="[{ value: '', label: ('\u2014 ' + String(overviewI18n.t('game')) + ' \u2014').trim() }, ...((games.length) ? (games).map((game) => ({ value: `platform:${game.id}`, label: (String(game.name)).trim(), group: i18n.t('platformOwner') })) : [])]" /></label></div>
 <p v-if="failed" role="alert" class="message error">{{ overviewI18n.t('initialFailure') }} <button class="button button-secondary" :disabled="busy" v-refresh-button="i18n.t('refresh')" @click="withGlobalLoading(load)">{{ i18n.t('refresh') }}</button></p>
 <p v-if="!selected && !busy && !failed" class="empty-state">{{ overviewI18n.t('chooseGame') }}</p>
 <PlatformLottery v-if="platformId" :key="platformId" surface="platform" brand-id="" :initial-game-id="platformId" draws-only :locale="locale" :display-settings="displaySettings" :can-manage="canManage" :can-connect="false" @failure="emit('failure', $event)" />
</section>
</template>

<style scoped>
.draws-game-picker label{display:flex;align-items:center;gap:8px;min-width:0;max-width:360px;width:100%;white-space:nowrap}@media(max-width:620px){.draws-game-picker label{max-width:none}}
</style>
