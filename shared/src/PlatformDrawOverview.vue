<script setup lang="ts">
import './mark-six-balls.css'
import { markSixCatalog } from './mark-six'
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
const pageSize = ref(50)

const props = defineProps<{ gameId: string; locale: 'en' | 'zh-CN'; canManage: boolean; surface?: 'platform' | 'admin'; brandId?: string }>()
const emit = defineEmits<{ failure: [cause: unknown]; manual: [] }>()
const i18n = createCatalogI18n(drawOverviewCatalog, props.locale)
watch(() => props.locale, value => i18n.setLocale(value))
const t = i18n.t
const numberI18n = createCatalogI18n(markSixCatalog, props.locale)
watch(() => props.locale, value => numberI18n.setLocale(value))
const attributeFor = (entry: PlatformDrawOverview['items'][number], number: number) => entry.attributes[String(number).padStart(2, '0')]
const numbersFor = (entry: PlatformDrawOverview['items'][number]) => [...entry.result.regular.map(value => ({value, special: false})), ...entry.result.special.map((value, index) => ({value, special: index === 0}))]
const api = createPlatformLotteryClient(props.surface ?? 'platform', () => props.brandId ?? '')
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
  const overview = await api.drawOverview(props.gameId, nextYear, nextOffset, pageSize.value)
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
 <p v-if="failed" class="message error" role="alert">{{ t(data ? 'failure' : 'initialFailure') }} <button class="button button-secondary" :disabled="busy" v-refresh-button="t('refresh')" @click="withGlobalLoading(load)">{{ t('refresh') }}</button></p>
 <p v-if="busy && !data" v-global-loading role="status">{{ t('loading') }}</p>
 <ManagementList v-if="data" :busy="busy">
 <template #filters><div class="ui-filter-bar"><slot name="filters" /><SelectControl :model-value="year" :disabled="busy" @change="withGlobalLoading(load, $event, 0)" :label="String(t('year'))" :options="data.years.map(choice => ({ value: choice, label: String(choice) }))" /></div></template>
 <template #actions><button v-if="canManage" class="button button-primary" @click="emit('manual')">{{ t('manualAdd') }}</button></template>
 <ManagementTable :entries="data.items.map(item => ({...item,id:item.period_no}))" :columns="[{key:'period',label:t('period')},{key:'result',label:t('result'),cellClass:'draw-result-cell'},{key:'date',label:t('date')},{key:'source',label:t('source')}]" :empty="!data.items.length" :empty-label="t('empty')">
 <template #period="{entry}"><strong>{{ entry.period_no }}</strong></template>
 <template #result="{entry}"><div class="draw-numbers">
  <span v-for="(number, index) in numbersFor(entry)" :key="index" class="draw-item">
   <b v-if="number.special" class="draw-plus">＋</b>
   <span class="draw-number"><span class="draw-ball" :class="data.mark_six ? ['is-mark-six', attributeFor(entry, number.value)?.wave ? `lottery-wave-${attributeFor(entry, number.value)!.wave}` : ''] : ''">{{ String(number.value).padStart(2, '0') }}</span><span v-if="data.mark_six" class="draw-zodiac">{{ attributeFor(entry, number.value)?.zodiac ? numberI18n.t(attributeFor(entry, number.value)!.zodiac) : '—' }}</span></span>
  </span>
  <span v-for="(n,index) in entry.result.digits" :key="`d${index}`" class="draw-ball">{{ n }}</span><strong v-if="entry.result.digits.length===3" class="draw-digit-sum" :aria-label="`${t('sum')}: ${entry.result.digits.reduce((sum:number,n:number)=>sum+n,0)}`">= {{entry.result.digits.reduce((sum:number,n:number)=>sum+n,0)}}</strong>
 </div></template>
 <template #date="{entry}">{{ date(entry.draw_date) }}</template><template #source="{entry}">{{ t(entry.source) }}</template>
 </ManagementTable><template #pagination><TablePagination :total="data.total_count" :page="Math.floor(offset / pageSize + 1)" :count="data.items.length" :locale="locale" :page-size="pageSize" :previous-disabled="busy || offset === 0" :next-disabled="busy || !data.has_more" :previous-label="t('previous')" :next-label="t('next')" :go-to="(target: number) => load(year, (target - 1) * pageSize)" :change-page-size="async (size: number) => { pageSize = size; await load(year, 0) }" :busy="busy" /></template></ManagementList>
</section>
</template>
<style scoped>
.draw-overview{display:grid;gap:12px;min-width:0;font-size:var(--body-font-size)}
.draw-numbers{display:flex;gap:6px;align-items:center;flex-wrap:wrap;font-variant-numeric:tabular-nums}
.draw-item{display:inline-flex;align-items:flex-start;gap:6px}
.draw-number{display:inline-flex;align-items:center;gap:4px}
.draw-ball{display:inline-grid;place-items:center;flex-shrink:0;min-width:28px;height:28px;border-radius:var(--radius-sm);background:var(--surface);border:1px solid var(--line);font-weight:650}
.draw-ball.is-mark-six{min-width:24px;width:24px;height:24px;border:0;border-radius:50%;background:var(--ink-secondary);color:var(--lottery-ball-ink)}
.draw-ball.is-mark-six[class*="lottery-wave-"]{background:var(--wave-color)}
.draw-zodiac{color:var(--lottery-attribute-zodiac);white-space:nowrap}
.draw-plus{display:grid;place-items:center;height:24px;color:var(--muted);font-weight:400}
.draw-numbers .draw-digit-sum{white-space:nowrap;font-variant-numeric:tabular-nums}
@media(max-width:1080px){:deep(td.draw-result-cell){grid-column:1 / -1}}
@media(max-width:1000px){.draw-number{flex-direction:column;gap:2px}.draw-numbers{align-items:flex-start}}
</style>
