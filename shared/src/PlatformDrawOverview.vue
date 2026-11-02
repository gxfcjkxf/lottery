<script setup lang="ts">
import ManagementTable from './ManagementTable.vue'
import ManagementList from "./ManagementList.vue"
import TablePagination from './TablePagination.vue'
import SelectControl from './SelectControl.vue'
import { ref, watch, onUnmounted } from 'vue'
import { withGlobalLoading, vGlobalLoading } from './global-loading'
import { vRefreshButton } from './refresh-button'
import { createCatalogI18n } from './localization'
import { drawOverviewCatalog } from './draw-overview-catalog'
import { createPlatformLotteryClient, PlatformLotteryError, type PlatformDrawOverview } from './platform-lottery'
import { platformDisplaySettings } from './display-settings'
const props = defineProps<{ gameId: string; locale: 'en' | 'zh-CN'; canManage: boolean }>()
const emit = defineEmits<{ failure: [cause: unknown]; manual: [] }>()
const i18n = createCatalogI18n(drawOverviewCatalog, props.locale)
watch(() => props.locale, value => i18n.setLocale(value))
const t = i18n.t
const api = createPlatformLotteryClient('platform', () => '')
const data = ref<PlatformDrawOverview | null>(null)
const year = ref(''), offset = ref(0), busy = ref(false), failed = ref(false)
let generation = 0, alive = true
function date(value: string | null) {
 if (!value) return t('unknownDate')
 const [year, month, day] = value.split('-')
 return platformDisplaySettings.value!.date_format.replace('YYYY', year!).replace('MM', month!).replace('DD', day!)
}
async function load(nextYear = year.value, nextOffset = offset.value) {
 const g = ++generation
 busy.value = true
 try {
  const overview = await api.drawOverview(props.gameId, nextYear, nextOffset)
  if (!alive || g !== generation) return
  data.value = overview; year.value = overview.year; offset.value = nextOffset
  failed.value = false
 } catch (cause) { if (alive && g === generation) { failed.value = true; if (cause instanceof PlatformLotteryError && cause.status === 401) emit('failure', cause) } } finally { if (alive && g === generation) busy.value = false }
}
watch(() => props.gameId, () => { generation++; data.value = null; year.value = ''; offset.value = 0; failed.value = false; void load() }, { immediate: true })
defineExpose({ load })
const poll = setInterval(() => { if (!busy.value) void load() }, 5000)
onUnmounted(() => { alive = false; generation++; clearInterval(poll) })
</script>
<template>
<section class="draw-overview" data-testid="draw-overview" :aria-busy="busy">
 <p v-if="failed" class="message error" role="alert">{{ t(data ? 'failure' : 'initialFailure') }} <button v-if="!data" class="button button-secondary" :disabled="busy" v-refresh-button="t('refresh')" @click="withGlobalLoading(load)">{{ t('refresh') }}</button></p>
 <p v-if="busy && !data" v-global-loading role="status">{{ t('loading') }}</p>
 <ManagementList v-if="data" :busy="busy">
 <template #filters><SelectControl :model-value="year" :disabled="busy" @change="withGlobalLoading(load, $event, 0)" :label="String(t('year'))" :options="data.years.map(choice => ({ value: choice, label: String(choice) }))" /></template>
 <template #actions><button class="button button-secondary" :disabled="busy" v-refresh-button="t('refresh')" @click="withGlobalLoading(load)">{{ t('refresh') }}</button><button v-if="canManage" class="button button-primary" @click="emit('manual')">{{ t('manualAdd') }}</button></template>
 <ManagementTable :entries="data.items.map(item => ({...item,id:item.period_no}))" :columns="[{key:'period',label:t('period')},{key:'result',label:t('result')},{key:'date',label:t('date')},{key:'source',label:t('source')}]" :empty="!data.items.length" :empty-label="t('empty')">
 <template #period="{entry}"><strong>{{ entry.period_no }}</strong></template>
 <template #result="{entry}"><div class="draw-numbers"><span v-for="(n, index) in entry.result.regular" :key="`r${index}`">{{ String(n).padStart(2, '0') }}</span><b v-if="entry.result.special.length">＋</b><span v-for="(n, index) in entry.result.special" :key="`s${index}`" class="draw-special">{{ String(n).padStart(2, '0') }}</span><span v-for="(n,index) in entry.result.digits" :key="`d${index}`">{{ n }}</span></div></template>
 <template #date="{entry}">{{ date(entry.draw_date) }}</template><template #source="{entry}">{{ t(entry.source) }}</template>
 </ManagementTable><template #pagination><TablePagination :show-empty="Boolean(data)" :total="data.total_count" :page="Math.floor(offset / 50 + 1)" :count="data.items.length" :locale="locale" :page-size="50" :previous-disabled="busy || offset === 0" :next-disabled="busy || !data.has_more" :previous-label="t('previous')" :next-label="t('next')" :go-to="(target: number) => load(year, (target - 1) * 50)" /></template></ManagementList>
</section>
</template>
<style scoped>
.draw-overview{display:grid;gap:12px;min-width:0;font-size:var(--body-font-size)}
.draw-numbers{display:flex;gap:6px;align-items:center;flex-wrap:wrap;font-variant-numeric:tabular-nums}
.draw-numbers span{display:grid;place-items:center;min-width:28px;min-height:28px;border-radius:var(--radius-sm);background:var(--surface);border:1px solid var(--line);font-weight:600}
.draw-numbers .draw-special{color:var(--primary);border-color:var(--primary)}
</style>
