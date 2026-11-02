<script setup lang="ts">
import TablePagination from "@lottery/shared/TablePagination.vue"
import { readPaginationTotal } from "@lottery/shared"
import SelectControl from '@lottery/shared/SelectControl.vue'
import { useOperationToast } from "@lottery/shared";
import { computed, onUnmounted, ref, watch } from 'vue'
import { formatDisplayDateTime, readApiEnvelope, vGlobalLoading, vRefreshButton, withGlobalLoading } from '@lottery/shared'
const bettingBasePath=(code?:string)=>`/api/v1${code?`/b/${encodeURIComponent(code)}`:''}`
const isUuid=(value:string)=>/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value)
import { useUserI18n } from './i18n'
import { createCatalogI18n } from '@lottery/shared'
import { markSixCatalog } from '../../shared/src/mark-six'
import '../../shared/src/mark-six-balls.css'
import { archiveNumbers, displayedArchiveNumbers, numberFacts, totalFacts, zodiacFamily, type ArchiveAttribute, type ArchiveSort } from './draw-archive-presentation'
interface Archive { total_count: string; mark_six: boolean; game_id: string; game_name: string; timezone: string; daily: boolean; choices: string[]; selected: string; items: Array<{ period_no: string; result: { regular: number[]; special: number[]; digits: number[] }; drawn_at: string | null; draw_date: string | null; regular_order: number[] | null; attributes: Record<string,ArchiveAttribute> }>; has_more: boolean }
const props = defineProps<{ gameId: string; kind: 'configured'; brandCode?: string; locale: 'en' | 'zh'; compact?: boolean }>()
const i18n = useUserI18n(), t = i18n.t
const archive = ref<Archive | null>(null), selected = ref(''), offset = ref(0), loading = ref(false)
const listElement = ref<HTMLElement | null>(null)
const options = ref({numbers:true,zodiac:true,wave:false,parity:false,animal:false})
const sort = ref<ArchiveSort>('drawn')
const mt = (key:string) => createCatalogI18n(markSixCatalog, props.locale==='zh'?'zh-CN':'en').t(['big','small','odd','even'].includes(key)?`attribute_${key}`:key)
const optionKeys=['numbers','zodiac','wave','parity','animal'] as const
const detailRows=computed(()=>optionKeys.filter((key):key is 'zodiac'|'wave'|'parity'|'animal'=>key!=='numbers' && options.value[key]))
const numbersFor=(period:Archive['items'][number])=>archive.value?.mark_six ? displayedArchiveNumbers(period.result,period.regular_order,sort.value) : archiveNumbers(period.result)
const attributeFor=(period:Archive['items'][number],number:number)=>period.attributes[String(number).padStart(2,'0')]
const factLabels=(kind:'zodiac'|'wave'|'parity'|'animal',period:Archive['items'][number],number:number)=>{
 const a=attributeFor(period,number), facts=numberFacts(number)
 if(kind==='zodiac')return [a?.zodiac?mt(a.zodiac):'—',a?.element?mt(a.element):'—']
 if(kind==='wave')return [a?.wave?t(`draw.wave.${a.wave}`):'—',mt(facts.size)]
 if(kind==='parity')return [mt(facts.parity),mt(facts.sumParity)]
 const family=zodiacFamily(a?.zodiac??'')
 return [family?t(`draw.animal.${family}`):'—',mt(totalFacts(period.result,period.attributes).parity)]
}
const factColor=(kind:'zodiac'|'wave'|'parity'|'animal',index:0|1,period:Archive['items'][number],number:number)=>{
 const attribute=attributeFor(period,number)
 if(kind==='zodiac')return index===0 ? (attribute?.zodiac?'zodiac':'missing') : (attribute?.element?`element-${attribute.element}`:'missing')
 if(kind==='wave')return index===0 ? (attribute?.wave?`wave-${attribute.wave}`:'missing') : `size-${numberFacts(number).size}`
 if(kind==='parity')return index===0?'parity':'sum-parity'
 return index===0 ? (attribute?.zodiac?'animal':'missing') : 'total-parity'
}
const missingAttributes=(period:Archive['items'][number])=>archiveNumbers(period.result).some(n=>{
 const a=attributeFor(period,n.value)
 return ((options.value.numbers||options.value.wave)&&!a?.wave)||(options.value.zodiac&&(!a?.zodiac||!a?.element))||(options.value.animal&&!a?.zodiac)
})
const dateTitle=(period:Archive['items'][number])=>period.draw_date !== null ? period.draw_date : period.drawn_at !== null ? formatDisplayDateTime(period.drawn_at,props.locale) : t('draw.dateUnknown')
const dateText=(period:Archive['items'][number])=>{
  if (period.draw_date !== null) return period.draw_date.slice(5)
  if (period.drawn_at === null) return '—'
  const value=period.drawn_at
  if(archive.value?.daily){const formatted=formatDisplayDateTime(value,props.locale);return formatted.slice(formatted.indexOf(' ')+1)}
  const parts=new Intl.DateTimeFormat('en',{timeZone:archive.value!.timezone,month:'2-digit',day:'2-digit'}).formatToParts(new Date(value))
  return `${parts.find(part=>part.type==='month')!.value}-${parts.find(part=>part.type==='day')!.value}`
}
const error = ref<ReturnType<typeof i18n.errorMessage> | null>(null)
let generation = 0
async function load(page = 0, requestedSelection = selected.value) {
  const token = ++generation
  loading.value = true; error.value = null
  try {
    if (!isUuid(props.gameId)) throw { code: 'REQUEST_INVALID', status: 400 }
    const query = new URLSearchParams({ game_id: props.gameId, kind: props.kind, offset: String(page) })
    if (requestedSelection) query.set('selected', requestedSelection)
    const response = await fetch(`${bettingBasePath(props.brandCode)}/draw-archive?${query}`, { credentials: 'same-origin', headers: { Accept: 'application/json' } })
    const envelope = await readApiEnvelope<Archive>(response, (message, status, code) => Object.assign(new Error(message), { status, code }))
    if (token !== generation) return
    if (envelope.data.game_id !== props.gameId) throw { code: 'INVALID_RESPONSE', status: 502 }
    readPaginationTotal(envelope.data); offset.value = page; archive.value = envelope.data; selected.value = envelope.data.selected
  } catch (cause) { if (token === generation) { if (archive.value) selected.value = archive.value.selected; error.value = i18n.errorMessage(cause, 'read') } }
  finally { if (token === generation) loading.value = false }
}
watch(() => [props.gameId, props.kind, props.brandCode], () => { selected.value = ''; archive.value = null; offset.value = 0; void load() }, { immediate: true })
onUnmounted(() => { generation++ })

useOperationToast(error, value => i18n.displayError(value), 'error');
</script>
<template>
  <section class="draw-results draw-archive user-record-list" :class="{'is-daily':archive?.daily}" data-testid="draw-archive">
    <Teleport defer to="#user-page-refresh" :disabled="compact"><button v-refresh-button="t('draw.refresh')" type="button" class="button button-secondary" :disabled="loading" @click="withGlobalLoading(load, offset)">{{ t('draw.refresh') }}</button></Teleport>
    <div v-if="archive" class="archive-toolbar ui-filter-bar" tabindex="0" :aria-label="t('draw.optionsLabel')">
    <label class="archive-filter ui-filter-field-inline">{{ t(archive.daily ? 'draw.historyDate' : 'draw.historyYear') }}<SelectControl v-model="selected" :disabled="loading" @change="withGlobalLoading(load)" :label="String(t(archive.daily ? 'draw.historyDate' : 'draw.historyYear'))" :options="(archive.choices).map((choice) => ({ value: choice, label: (String(choice)).trim() }))" /></label>
    <div v-if="archive?.mark_six" class="archive-controls">
      <div class="archive-options" role="group" :aria-label="t('draw.optionsLabel')"><span class="archive-control-label">{{ t('draw.optionsLabel') }}</span><label v-for="option in optionKeys" :key="option"><input v-model="options[option]" type="checkbox" :name="`archive-option-${option}`" /><span>{{ t(`draw.option.${option}`) }}</span></label></div>
      <div class="archive-sorting" role="group" :aria-label="t('draw.sortLabel')"><span class="archive-control-label">{{ t('draw.sortLabel') }}</span><label><input v-model="sort" type="radio" name="archive-sort" value="drawn" /><span>{{ t('draw.sort.drawn') }}</span></label><label><input v-model="sort" type="radio" name="archive-sort" value="size" /><span>{{ t('draw.sort.size') }}</span></label></div>
    </div>
    </div>

    <p v-if="loading" v-global-loading role="status">{{ t('draw.loading') }}</p>
    <div ref="listElement" :aria-busy="loading" class="archive-list" role="list" :aria-label="t('draw.results')">
      <article v-for="period in archive?.items" :key="period.period_no" class="archive-row" role="listitem">
        <div class="archive-meta"><strong class="archive-period">{{ period.period_no }}</strong><time class="archive-time" :datetime="period.draw_date ?? period.drawn_at ?? undefined" :title="dateTitle(period)" :aria-label="dateTitle(period)">{{ dateText(period) }}</time></div>
        <div class="archive-result">
          <p v-if="numbersFor(period)===null" class="archive-missing archive-order-missing">{{ t('draw.orderMissing') }}</p>
          <template v-else>
            <div v-if="!archive!.mark_six||options.numbers" class="archive-numbers" :class="{'archive-grid':archive!.mark_six}">
              <template v-for="(number,index) in numbersFor(period)" :key="index">
                <span v-if="number.special" class="archive-plus" :aria-label="t('draw.specialSeparator')">+</span>
                <span class="archive-number"><span class="archive-ball" :class="[archive!.mark_six && attributeFor(period,number.value)?.wave ? ['mark-six-ball', `lottery-wave-${attributeFor(period,number.value)!.wave}`] : '',{special:number.special}]">{{ archive!.mark_six ? String(number.value).padStart(2,'0') : number.value }}</span></span>
              </template>
            </div>
            <template v-if="archive!.mark_six">
              <div v-for="detail in detailRows" :key="detail" class="archive-detail-row archive-grid" :class="`archive-detail-${detail}`" :aria-label="t(`draw.option.${detail}`)">
                <template v-for="(number,index) in numbersFor(period)" :key="index">
                  <span v-if="number.special" class="archive-detail-gap" aria-hidden="true"></span>
                  <span class="archive-fact-cell" :title="factLabels(detail,period,number.value).join(' / ')"><span :class="[{'archive-zodiac':detail==='zodiac'}, `archive-fact-${factColor(detail,0,period,number.value)}`]">{{ factLabels(detail,period,number.value)[0] }}</span><span class="archive-fact-divider" aria-hidden="true">/</span><span :class="`archive-fact-${factColor(detail,1,period,number.value)}`">{{ factLabels(detail,period,number.value)[1] }}</span></span>
                </template>
              </div>
            </template>
            <small v-if="archive!.mark_six&&missingAttributes(period)" class="archive-missing">{{ t('draw.attributesMissing') }}</small>
          </template>
        </div>
      </article>
      <p v-if="archive && !loading && !archive.items.length" class="empty-state">{{ t('draw.noResults') }}</p>
    </div>
    <TablePagination show-empty v-if="archive" class="page-actions" :total="archive?.total_count ?? '0'" :page="Math.floor(offset / 50 + 1)" :count="archive?.items.length ?? 0" :locale="locale === 'en' ? 'en' : 'zh-CN'" :page-size="50" :previous-disabled="loading || offset === 0" :next-disabled="loading || !archive?.has_more" :previous-label="t('draw.previous')" :next-label="t('draw.next')" :go-to="(target: number) => load((target - 1) * 50)" />
  </section>
</template>
<style scoped>
.draw-archive{min-width:0}.archive-filter{display:flex;flex-wrap:wrap;align-items:center;gap:6px;font-size:var(--body-font-size);margin:0 0 4px}
.archive-toolbar{display:flex;align-items:center;flex-wrap:wrap;gap:0 12px;margin:0 0 4px}.archive-toolbar .archive-filter{flex:0 0 auto;margin:0}.archive-controls{display:flex;align-items:center;flex-wrap:wrap;gap:0 12px;min-width:0;flex:1 1 auto;margin:0}.archive-options,.archive-sorting{display:flex;align-items:center;flex-wrap:wrap;gap:0 12px;min-width:0;border:0;padding:0;margin:0;font-size:var(--body-font-size)}.archive-options,.archive-sorting{flex:0 1 auto}.archive-control-label{padding:0;min-height:var(--control-height);display:flex;align-items:center;color:var(--muted)}.archive-controls label{display:inline-flex;align-items:center;gap:5px;min-height:var(--control-height);cursor:pointer;min-width:0}.archive-controls input{margin:0;flex-shrink:0;accent-color:var(--primary)}.archive-controls input[type="radio"]{box-sizing:border-box;width:14px;height:14px;min-width:14px;min-height:14px;flex:0 0 14px;padding:0;border:0;border-radius:50%;background:none;appearance:auto;box-shadow:none}.archive-controls input:focus-visible{outline:2px solid var(--primary);outline-offset:2px}.archive-controls label>span{white-space:nowrap}.archive-row+.archive-row{border-top:1px solid var(--line)}.archive-row{display:grid;grid-template-columns:max-content max-content minmax(0,1fr);align-items:start;column-gap:12px;padding:4px 10px;font-variant-numeric:tabular-nums}.archive-period{max-width:9ch;padding-block:7px;font-size:var(--body-font-size);font-weight:650;overflow-wrap:anywhere}.archive-time{padding-block:7px;font-size:var(--body-font-size);color:var(--muted);white-space:nowrap}.archive-result{min-width:0}.archive-grid{display:grid;grid-template-columns:repeat(6,minmax(0,1fr)) 16px minmax(0,1fr);align-items:center;column-gap:4px}.archive-numbers{display:flex;align-items:center;gap:6px}.archive-numbers.archive-grid{display:grid}.archive-number{display:grid;justify-items:center;min-width:0;padding:3px 0}.archive-ball{display:inline-grid;place-items:center;width:30px;height:30px;border-radius:50%;background:var(--canvas);color:var(--ink);font-size:var(--body-font-size);font-weight:750;font-variant-numeric:tabular-nums}.archive-ball.mark-six-ball{color:var(--lottery-ball-ink);background:radial-gradient(circle at 32% 26%,var(--wave-light) 0%,var(--wave-color) 48%,var(--wave-color) 100%)}.archive-ball.special{outline:1px solid var(--accent);outline-offset:1px}.archive-plus{display:inline-grid;place-items:center;height:30px;font-size:16px;color:var(--muted)}.archive-detail-row{border-top:1px solid var(--line-subtle);min-height:26px;font-size:var(--body-font-size);line-height:1.4}.archive-fact-cell{display:flex;flex-wrap:wrap;align-items:center;justify-content:center;gap:2px;min-width:0;text-align:center;padding:3px 0;color:var(--ink-secondary)}.archive-fact-cell>span{max-width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.archive-fact-divider{color:var(--muted)}.archive-missing{display:block;margin:3px 0 0;font-size:var(--body-font-size);line-height:1.5;color:var(--muted)}.archive-order-missing{padding:6px 0}
.archive-fact-zodiac{color:var(--lottery-attribute-zodiac)}.archive-fact-element-metal{color:var(--lottery-element-metal)}.archive-fact-element-wood{color:var(--lottery-element-wood)}.archive-fact-element-water{color:var(--lottery-element-water)}.archive-fact-element-fire{color:var(--lottery-element-fire)}.archive-fact-element-earth{color:var(--lottery-element-earth)}.archive-fact-wave-red{color:var(--lottery-wave-red)}.archive-fact-wave-blue{color:var(--lottery-wave-blue)}.archive-fact-wave-green{color:var(--lottery-wave-green-text)}.archive-fact-size-big{color:var(--lottery-wave-blue)}.archive-fact-size-small{color:var(--lottery-wave-green-text)}.archive-fact-parity{color:var(--lottery-attribute-parity)}.archive-fact-sum-parity{color:var(--lottery-attribute-sum-parity)}.archive-fact-animal{color:var(--lottery-attribute-animal)}.archive-fact-total-parity{color:var(--lottery-attribute-total-parity)}.archive-fact-missing{color:var(--muted)}
.archive-meta{display:contents}
@media(max-width:999px){.archive-row{grid-template-columns:max-content minmax(0,1fr)}.archive-meta{display:flex;flex-direction:column;gap:2px;padding-block:5px}.archive-meta .archive-period,.archive-meta .archive-time{padding-block:0}}
@media(max-width:620px){.archive-options,.archive-sorting{column-gap:10px;font-size:var(--body-font-size)}.archive-row{padding:3px 5px;column-gap:6px}.archive-period,.archive-time{padding-block:5px}.archive-grid{grid-template-columns:repeat(6,minmax(0,1fr)) 12px minmax(0,1fr);column-gap:2px}.archive-ball{width:24px;height:24px;font-size:var(--body-font-size)}.archive-period,.archive-time,.archive-detail-row{font-size:var(--body-font-size)}.archive-plus{height:24px;font-size:14px}.archive-detail-row{min-height:25px}.archive-fact-cell{gap:0;flex-direction:column}.archive-fact-cell>span{flex-basis:auto}.archive-fact-divider{display:none}}
@media(max-width:359px){.archive-row{padding-inline:3px;column-gap:4px}.archive-grid{grid-template-columns:repeat(6,minmax(0,1fr)) 8px minmax(0,1fr);column-gap:0}.archive-ball{width:22px;height:22px;font-size:var(--body-font-size)}.archive-plus{height:22px}.archive-fact-cell{row-gap:0}.archive-fact-divider{display:none}.archive-fact-cell>span{flex-basis:100%}}
</style>
