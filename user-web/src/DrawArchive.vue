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
import { archiveNumbers, displayedArchiveNumbers, specialFacts, totalFacts, zodiacFamily, shortArchivePeriod, type ArchiveAttribute, type ArchiveSort } from './draw-archive-presentation'
const pageSize = ref(50)

interface Archive { total_count: string; mark_six: boolean; game_id: string; game_name: string; timezone: string; daily: boolean; choices: string[]; selected: string; items: Array<{ period_no: string; result: { regular: number[]; special: number[]; digits: number[] }; drawn_at: string | null; draw_date: string | null; regular_order: number[] | null; attributes: Record<string,ArchiveAttribute> }>; has_more: boolean }
const props = defineProps<{ gameId: string; kind: 'configured'; brandCode?: string; locale: 'en' | 'zh'; compact?: boolean }>()
const i18n = useUserI18n(), t = i18n.t
const archive = ref<Archive | null>(null), selected = ref(''), offset = ref(0), loading = ref(false)
const listElement = ref<HTMLElement | null>(null)
const attributeKeys = ['zodiac','element','animal'] as const
type AttributeKey = typeof attributeKeys[number]
const attribute = ref<AttributeKey>('zodiac')
const conditions=ref<string[]>([])
const conditionCatalog={
 zodiac:['rat','ox','tiger','rabbit','dragon','snake','horse','goat','monkey','rooster','dog','pig'],
 element:['metal','wood','water','fire','earth'],
 animal:['domestic','wild'],
} as const
const conditionOptions=computed(()=>conditionCatalog[attribute.value])
watch(attribute,()=>{conditions.value=[]})

const summaryKeys = ['total','totalParity','totalSize','totalWave','specialParity','specialSize'] as const
type SummaryKey = typeof summaryKeys[number]
const summaries = ref<SummaryKey[]>([])
const sort = ref<ArchiveSort>('drawn')
const numberI18n = createCatalogI18n(markSixCatalog, props.locale==='zh'?'zh-CN':'en')
watch(() => props.locale, value => numberI18n.setLocale(value==='zh'?'zh-CN':'en'))
const mt = (key:string) => numberI18n.t(['big','small','odd','even'].includes(key)?`attribute_${key}`:key)
const numbersFor=(period:Archive['items'][number])=>archive.value?.mark_six ? displayedArchiveNumbers(period.result,period.regular_order,sort.value) : archiveNumbers(period.result)
const attributeFor=(period:Archive['items'][number],number:number)=>period.attributes[String(number).padStart(2,'0')]
const matchesCondition=(period:Archive['items'][number],number:number)=>{
 if(!conditions.value.length)return true
 const saved=attributeFor(period,number)
 const value=attribute.value==='animal'?zodiacFamily(saved?.zodiac??''):saved?.[attribute.value]
 return value!==null&&value!==undefined&&conditions.value.includes(value)
}
const conditionLabel=(key:string)=>attribute.value==='animal'?t(`draw.animal.${key}`):mt(key)

const numberAttribute=(period:Archive['items'][number],number:number)=>{
 const saved=attributeFor(period,number)
 if(attribute.value==='animal'){const family=zodiacFamily(saved?.zodiac??'');return family?t(`draw.animal.${family}`):'—'}
 return saved?.[attribute.value]?mt(saved[attribute.value]):'—'
}
const attributeColor=(period:Archive['items'][number],number:number)=>attribute.value==='element' ? `element-${attributeFor(period,number)?.element??'missing'}` : attribute.value
const totalKeys=summaryKeys.slice(0,4)
const specialKeys=summaryKeys.slice(4)
const visibleTotal=computed(()=>totalKeys.filter(key=>summaries.value.includes(key)))
const visibleSpecial=computed(()=>specialKeys.filter(key=>summaries.value.includes(key)))
const visibleColumns=computed(()=>summaryKeys.filter(key=>summaries.value.includes(key)))
const columnLabel=(key:SummaryKey)=>t(`draw.column.${key==='total'?'total':key.endsWith('Parity')?'parity':key.endsWith('Size')?'size':'wave'}`)
const summaryValue=(key:SummaryKey,period:Archive['items'][number])=>{
 const total=totalFacts(period.result,period.attributes)
 if(key==='total')return String(total.total)
 if(key==='totalParity')return mt(total.parity)
 if(key==='totalSize')return mt(total.size)
 if(key==='totalWave')return total.wave ? total.wave==='push'?mt('push'):t(`draw.wave.${total.wave}`) : '—'
 return mt(specialFacts(period.result,'special')[key==='specialParity'?0:1]!)
}
const summaryColor=(key:SummaryKey,period:Archive['items'][number])=>key==='totalWave'?`wave-${totalFacts(period.result,period.attributes).wave??'missing'}`:key
const dateText=(period:Archive['items'][number])=>{
 if(period.draw_date!==null)return period.draw_date
 if(period.drawn_at===null)return '—'
 if(archive.value?.daily){const formatted=formatDisplayDateTime(period.drawn_at,props.locale);return formatted.slice(formatted.indexOf(' ')+1)}
 const parts=new Intl.DateTimeFormat('en',{timeZone:archive.value!.timezone,year:'numeric',month:'2-digit',day:'2-digit'}).formatToParts(new Date(period.drawn_at))
 const fields=Object.fromEntries(parts.map(part=>[part.type,part.value]))
 return `${fields.year}-${fields.month}-${fields.day}`
}
const periodText=(period:Archive['items'][number])=>t('workspace.period',{number:shortArchivePeriod(period.period_no,dateText(period),archive.value!.daily)})
const currentDate = computed(() => {
  if (!archive.value) return ''
  const parts = new Intl.DateTimeFormat('en', { timeZone: archive.value.timezone, year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(new Date())
  const fields = Object.fromEntries(parts.map(part => [part.type, part.value]))
  return `${fields.year}-${fields.month}-${fields.day}`
})
const error = ref<ReturnType<typeof i18n.errorMessage> | null>(null)
let generation = 0
async function load(page = 0, requestedSelection = selected.value) {
  const token = ++generation
  loading.value = true; error.value = null
  try {
    if (!isUuid(props.gameId)) throw { code: 'REQUEST_INVALID', status: 400 }
    const query = new URLSearchParams({ game_id: props.gameId, kind: props.kind, offset: String(page), limit: String(pageSize.value) })
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
  <section class="draw-results draw-archive user-record-list" :class="{'is-daily':archive?.daily,'is-english':locale==='en'}" data-testid="draw-archive">
    <Teleport defer to="#user-page-refresh" :disabled="compact"><button v-if="error || (archive?.daily && archive.selected === currentDate)" v-refresh-button="t('draw.refresh')" type="button" class="button button-secondary" :disabled="loading" @click="withGlobalLoading(load, offset)">{{ t('draw.refresh') }}</button></Teleport>
    <div v-if="archive" class="archive-toolbar ui-filter-bar" tabindex="0" :aria-label="t('draw.optionsLabel')">
    <div class="archive-filter ui-filter-field-inline"><span>{{ t(archive.daily ? 'draw.historyDate' : 'draw.historyYear') }}</span><SelectControl v-model="selected" :disabled="loading" @change="withGlobalLoading(load)" :label="String(t(archive.daily ? 'draw.historyDate' : 'draw.historyYear'))" :options="(archive.choices).map((choice) => ({ value: choice, label: (String(choice)).trim() }))" /></div>
    <div v-if="archive.mark_six" class="archive-controls">
      <div class="archive-attributes" role="radiogroup" :aria-label="t('draw.attributeLabel')"><span class="archive-control-label">{{ t('draw.attributeLabel') }}</span><div class="archive-option-items"><label v-for="key in attributeKeys" :key="key" class="archive-attribute-option"><input v-model="attribute" type="radio" name="archive-attribute" :value="key" /><span>{{ t(`draw.attribute.${key}`) }}</span></label></div></div>
      <div class="archive-sorting" role="group" :aria-label="t('draw.sortLabel')"><span class="archive-control-label">{{ t('draw.sortLabel') }}</span><div class="archive-option-items"><label><input v-model="sort" type="radio" name="archive-sort" value="drawn" /><span>{{ t('draw.sort.drawn') }}</span></label><label><input v-model="sort" type="radio" name="archive-sort" value="size" /><span>{{ t('draw.sort.size') }}</span></label></div></div>
      <div class="archive-condition-options" role="group" :aria-label="t('draw.conditionsLabel')"><span class="archive-control-label">{{ t('draw.conditionsLabel') }}</span><div class="archive-option-items"><label v-for="key in conditionOptions" :key="key"><input v-model="conditions" type="checkbox" :value="key" :name="`archive-condition-${key}`" /><span>{{ conditionLabel(key) }}</span></label></div></div>
      <div class="archive-summary-options" role="group" :aria-label="t('draw.summaryLabel')">
        <div class="archive-summary-group" role="group" :aria-label="t('draw.summary.total')"><span class="archive-control-label">{{ t('draw.summary.total') }}</span><div class="archive-option-items"><label v-for="key in totalKeys" :key="key"><input v-model="summaries" type="checkbox" :value="key" :name="`archive-summary-${key}`" /><span>{{ columnLabel(key) }}</span></label></div></div>
        <div class="archive-summary-group" role="group" :aria-label="t('draw.column.special')"><span class="archive-control-label">{{ t('draw.column.special') }}</span><div class="archive-option-items"><label v-for="key in specialKeys" :key="key"><input v-model="summaries" type="checkbox" :value="key" :name="`archive-summary-${key}`" /><span>{{ columnLabel(key) }}</span></label></div></div>
      </div>
    </div>
    </div>

    <p v-if="loading" v-global-loading role="status">{{ t('draw.loading') }}</p>
    <div ref="listElement" :aria-busy="loading" class="archive-list" tabindex="0" role="region" :aria-label="t('draw.results')">
      <table v-if="archive?.items.length" class="archive-table" :class="{'is-mark-six':archive.mark_six}" :aria-label="t('draw.results')">
        <thead>
          <tr class="archive-head">
            <th class="archive-meta-cell" :rowspan="archive.mark_six&&visibleColumns.length?2:1" scope="col">{{ t('draw.column.datePeriod') }}</th>
            <th class="archive-regular-cell" :rowspan="archive.mark_six&&visibleColumns.length?2:1" scope="col">{{ t(archive.mark_six?'draw.column.regular':'draw.column.numbers') }}</th>
            <th v-if="archive.mark_six" class="archive-separator-cell" :rowspan="visibleColumns.length?2:1" aria-hidden="true"></th>
            <th v-if="archive.mark_six" class="archive-special-cell" :rowspan="visibleColumns.length?2:1" scope="col">{{ t('draw.column.special') }}</th>
            <th v-if="archive.mark_six&&visibleTotal.length" class="archive-group-heading archive-group-start" :colspan="visibleTotal.length" scope="colgroup">{{ t('draw.summary.total') }}</th>
            <th v-if="archive.mark_six&&visibleSpecial.length" class="archive-group-heading archive-group-start" :colspan="visibleSpecial.length" scope="colgroup">{{ t('draw.column.special') }}</th>
          </tr>
          <tr v-if="archive.mark_six&&visibleColumns.length" class="archive-head">
            <th v-for="(key,index) in visibleColumns" :key="key" class="archive-stat-cell" :class="{'archive-group-start':index===0||index===visibleTotal.length}" :data-summary-header="key" scope="col"><span class="archive-header-text">{{ columnLabel(key).replaceAll('/','/\u200b') }}</span></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="period in archive.items" :key="period.period_no" class="archive-row">
            <td class="archive-meta-cell"><div class="archive-meta"><time class="archive-time" :datetime="period.draw_date??period.drawn_at??undefined">{{ dateText(period) }}</time><strong class="archive-period" :title="period.period_no">{{ periodText(period) }}</strong></div></td>
            <td class="archive-regular-cell">
              <p v-if="numbersFor(period)===null" class="archive-missing archive-order-missing">{{ t('draw.orderMissing') }}</p>
              <div v-else class="archive-numbers" :class="{'has-digit-sum':period.result.digits.length===3}">
                <span v-for="(number,index) in numbersFor(period)!.filter(number=>!archive!.mark_six||!number.special)" :key="index" class="archive-number" :class="{'is-dimmed':archive.mark_six&&!matchesCondition(period,number.value)}">
                  <span class="archive-ball" :class="archive.mark_six&&attributeFor(period,number.value)?.wave?`lottery-wave-${attributeFor(period,number.value)!.wave}`:''">{{ archive.mark_six?String(number.value).padStart(2,'0'):number.value }}</span>
                  <span v-if="archive.mark_six" class="archive-number-attribute" :class="`archive-fact-${attributeColor(period,number.value)}`">{{ numberAttribute(period,number.value) }}</span>
                </span>
                <span v-if="period.result.digits.length===3" class="archive-digit-sum" :aria-label="`${t('draw.summary.total')}: ${period.result.digits.reduce((sum,n)=>sum+n,0)}`"><span aria-hidden="true">=</span><strong>{{period.result.digits.reduce((sum,n)=>sum+n,0)}}</strong></span>
              </div>
            </td>
            <td v-if="archive.mark_six" class="archive-separator-cell"><span class="archive-plus" :aria-label="t('draw.specialSeparator')">+</span></td>
            <td v-if="archive.mark_six" class="archive-special-cell"><div class="archive-numbers"><span v-for="number in period.result.special" :key="number" class="archive-number" :class="{'is-dimmed':!matchesCondition(period,number)}"><span class="archive-ball" :class="attributeFor(period,number)?.wave?`lottery-wave-${attributeFor(period,number)!.wave}`:''">{{ String(number).padStart(2,'0') }}</span><span class="archive-number-attribute" :class="`archive-fact-${attributeColor(period,number)}`">{{ numberAttribute(period,number) }}</span></span></div></td>
            <template v-if="archive.mark_six"><td v-for="(key,index) in visibleColumns" :key="key" class="archive-stat-cell" :class="[{'archive-group-start':index===0||index===visibleTotal.length},`archive-fact-${summaryColor(key,period)}`]" :data-summary="key">{{ summaryValue(key,period) }}</td></template>
          </tr>
        </tbody>
      </table>
      <p v-if="archive&&!archive.items.length" class="empty-state">{{ t('draw.noResults') }}</p>
    </div>
    <TablePagination v-if="archive" class="page-actions" :total="archive?.total_count ?? '0'" :page="Math.floor(offset / pageSize + 1)" :count="archive?.items.length ?? 0" :locale="locale === 'en' ? 'en' : 'zh-CN'" :page-size="pageSize" :previous-disabled="loading || offset === 0" :next-disabled="loading || !archive?.has_more" :previous-label="t('draw.previous')" :next-label="t('draw.next')" :go-to="(target: number) => load((target - 1) * pageSize)" :change-page-size="async (size: number) => { pageSize = size; await load(0) }" :busy="loading" />
  </section>
</template>
<style scoped>
.draw-archive{min-width:0;--archive-label-width:42px}.draw-archive.is-english{--archive-label-width:72px}
.archive-toolbar{display:flex;align-items:center;align-content:flex-start;flex-wrap:wrap;gap:0 14px}
:global(#app .draw-archive.user-record-list>.archive-toolbar){padding-block:8px 4px;margin-block:0;row-gap:4px}
.archive-controls label,.archive-control-label,.archive-attributes,.archive-sorting,.archive-condition-options,.archive-summary-options,.archive-summary-group{margin-block:0;padding-block:0}
.archive-filter{display:flex;align-items:center;gap:6px;flex:0 0 auto;margin:0;font-size:var(--body-font-size)}
.archive-controls{display:contents}
.archive-option-items{display:flex;align-items:center;flex-wrap:wrap;gap:0 12px;min-width:0}.archive-attributes .archive-option-items{gap:0 6px}
.archive-attributes,.archive-sorting,.archive-summary-options,.archive-summary-group,.archive-condition-options{display:flex;align-items:center;flex-wrap:wrap;gap:0 12px;min-width:0;font-size:var(--body-font-size)}
.archive-attributes{gap:0 6px}.archive-attributes>.archive-control-label{margin-right:4px}.archive-sorting{margin-left:auto}
.archive-condition-options{flex-basis:100%}.archive-number.is-dimmed{opacity:.2}
.archive-summary-options{flex-basis:100%;gap:0 16px}.archive-summary-group+.archive-summary-group{border-left:1px solid var(--line);padding-left:16px}
.archive-control-label{display:flex;align-items:center;min-height:var(--control-height);color:var(--muted)}
.archive-controls label{display:inline-flex;align-items:center;gap:5px;min-height:var(--control-height);cursor:pointer}
.archive-controls input{margin:0;flex-shrink:0;accent-color:var(--primary)}
.archive-controls input[type="radio"]{box-sizing:border-box;width:14px;height:14px;min-width:14px;min-height:14px;flex:0 0 14px;padding:0;border:0;border-radius:50%;background:none;appearance:auto;box-shadow:none}
.archive-controls input:focus-visible{outline:2px solid var(--primary);outline-offset:2px}
.archive-controls label>span{white-space:nowrap}
.archive-attribute-option{position:relative}.archive-attribute-option>input[type="radio"]{position:absolute;width:1px;height:1px;min-width:0;min-height:0;opacity:0}
.archive-attribute-option>span{display:inline-flex;align-items:center;justify-content:center;min-height:var(--control-height);padding:0 12px;border:1px solid var(--line);border-radius:var(--radius-sm);background:var(--surface);color:var(--ink-secondary)}
.archive-attribute-option>input:checked+span{background:var(--primary);border-color:var(--primary);color:var(--on-primary);font-weight:650}
.archive-attribute-option:hover>span{border-color:var(--primary)}.archive-attribute-option>input:focus-visible+span{outline:2px solid var(--primary);outline-offset:2px}
.archive-list{max-width:100%;overflow:auto;overscroll-behavior:contain}
.archive-list:focus-visible{outline:2px solid var(--primary);outline-offset:-2px}
.archive-table{width:100%;border-collapse:separate;border-spacing:0;font-size:var(--body-font-size);line-height:1.4;font-variant-numeric:tabular-nums}
.archive-table thead{background:var(--table-header-bg)}
.archive-table th,.archive-table td{position:static;inset:auto;z-index:auto;padding:4px 8px;vertical-align:middle;min-width:0}
.archive-header-text{min-width:0;max-width:100%}.archive-table th{height:28px;text-align:center}.archive-table td{border-bottom:1px solid var(--line)}.archive-table tbody tr:last-child td{border-bottom:0}
.archive-group-start{border-left:1px solid var(--line)}
.archive-table .archive-meta-cell{text-align:left;width:1%}.archive-meta{display:flex;align-items:center;flex-wrap:nowrap;gap:2px 6px;min-width:0}.archive-time{color:var(--muted);white-space:nowrap}.archive-period{white-space:nowrap;flex-shrink:0;font-size:var(--body-font-size);font-weight:650;overflow-wrap:anywhere}
.archive-numbers{display:flex;align-items:center;flex-wrap:wrap;gap:4px 6px;min-width:0}.archive-regular-cell .archive-numbers{justify-content:space-between}.archive-number{display:inline-flex;align-items:center;gap:2px;min-width:0}
.archive-regular-cell .archive-numbers.has-digit-sum{justify-content:flex-start;gap:4px 6px}.archive-digit-sum{display:inline-flex;align-items:center;gap:6px;white-space:nowrap;color:var(--ink);font-variant-numeric:tabular-nums}.archive-digit-sum>span{color:var(--muted)}
.archive-ball{display:inline-grid;place-items:center;flex:0 0 24px;width:24px;height:24px;border-radius:50%;background:var(--ink-secondary);color:var(--lottery-ball-ink);font-size:var(--body-font-size);font-weight:650;font-variant-numeric:tabular-nums}
.archive-ball[class*="lottery-wave-"]{background:var(--wave-color)}
.archive-table .archive-separator-cell{--table-cell-padding-inline:0px;width:1px;padding-inline:0;text-align:center;white-space:nowrap}
.archive-table .archive-special-cell{text-align:center;white-space:nowrap}.archive-special-cell .archive-numbers{justify-content:center;flex-wrap:nowrap}
.archive-table.is-mark-six .archive-regular-cell{width:85.714286%}.archive-table.is-mark-six .archive-special-cell{width:14.285714%}
.archive-table.is-mark-six .archive-regular-cell .archive-numbers{display:grid;grid-template-columns:repeat(6,minmax(max-content,1fr));justify-items:center}
.archive-plus{display:flex;align-items:center;justify-content:center;width:8px;min-width:8px;height:24px;color:var(--muted)}.archive-number-attribute{white-space:nowrap}
.archive-table .archive-stat-cell{text-align:center;white-space:nowrap}.archive-table td[data-summary="total"]{font-weight:650}
.archive-missing{margin:0;font-size:var(--body-font-size);color:var(--muted)}
.archive-fact-zodiac{color:var(--lottery-attribute-zodiac)}.archive-fact-element-metal{color:var(--lottery-element-metal)}.archive-fact-element-wood{color:var(--lottery-element-wood)}.archive-fact-element-water{color:var(--lottery-element-water)}.archive-fact-element-fire{color:var(--lottery-element-fire)}.archive-fact-element-earth{color:var(--lottery-element-earth)}.archive-fact-animal{color:var(--lottery-attribute-animal)}
.archive-fact-wave-red{color:var(--lottery-wave-red)}.archive-fact-wave-blue{color:var(--lottery-wave-blue)}.archive-fact-wave-green{color:var(--lottery-wave-green-text)}.archive-fact-missing,.archive-fact-element-missing{color:var(--muted)}
@media(max-width:999px){
 .archive-table{width:max-content;min-width:100%}
 .archive-table th{white-space:nowrap}
 .archive-table .archive-numbers{flex-wrap:nowrap;gap:3px 2px}
 .archive-meta{flex-direction:column;align-items:flex-start;gap:0}

 :global(#app .draw-archive.user-record-list>.archive-toolbar){display:grid;grid-template-columns:max-content minmax(0,1fr);column-gap:10px;row-gap:4px}
 :global(#app .draw-archive.user-record-list .archive-filter){display:flex;gap:6px;align-items:center;align-self:start;justify-self:start}
 .archive-filter :deep(.select-control){justify-self:start}
 .archive-sorting,.archive-condition-options,.archive-summary-options{grid-column:1/-1}
 .archive-attributes,.archive-sorting,.archive-condition-options,.archive-summary-group{display:grid;grid-template-columns:var(--archive-label-width) minmax(0,1fr);align-items:start;gap:0 6px;margin:0}
 .archive-attributes{grid-column:2;grid-template-columns:max-content minmax(0,1fr);align-self:center}
 .archive-control-label{min-height:24px;line-height:24px}.archive-attributes>.archive-control-label{min-height:28px;line-height:28px;margin:0}
 .archive-controls label{min-height:24px;padding-block:0;line-height:20px}
 .archive-attribute-option>span{min-height:28px;padding:0 8px;line-height:20px}
 .archive-option-items,.archive-attributes .archive-option-items{gap:2px 6px;min-width:0}
 .archive-condition-options .archive-option-items{column-gap:4px}
 .archive-summary-options{display:grid;grid-template-columns:minmax(0,1fr);gap:4px}
 .archive-summary-group+.archive-summary-group{border:0;padding-left:0}

 .archive-number{flex-direction:column;align-items:center;gap:1px;flex-shrink:0}
 .archive-table td.archive-separator-cell{vertical-align:top}
 .archive-separator-cell .archive-plus{height:24px}
}
@media(max-width:699px){
 .archive-table th,.archive-table td{padding:3px 4px}
 .archive-ball{width:22px;height:22px;flex-basis:22px}
 .archive-separator-cell .archive-plus{height:22px}
}
</style>
