<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { formatDisplayDateTime, readApiEnvelope, vGlobalLoading, vRefreshButton, withGlobalLoading } from '@lottery/shared'
import { bettingBasePath } from './betting-api'
import { isUuid } from './draw-results-state'
import { useUserI18n } from './i18n'
import { createCatalogI18n } from '@lottery/shared'
import { markSixCatalog } from '../../shared/src/mark-six'
import '../../shared/src/mark-six-balls.css'
import { archiveNumbers, specialFacts, totalFacts, type ArchiveAttribute } from './draw-archive-presentation'
interface Archive { mark_six: boolean; game_id: string; game_name: string; timezone: string; daily: boolean; choices: string[]; selected: string; items: Array<{ period_no: string; result: { regular: number[]; special: number[]; digits: number[] }; drawn_at: string; attributes: Record<string,ArchiveAttribute> }>; has_more: boolean }
const props = defineProps<{ gameId: string; kind: 'standard' | 'configured'; brandCode?: string; locale: 'en' | 'zh'; compact?: boolean }>()
const i18n = useUserI18n(), t = i18n.t
const archive = ref<Archive | null>(null), selected = ref(''), offset = ref(0), loading = ref(false)
const mode = ref<'numbers'|'special'|'tail'|'total'>('numbers')
const mt = (key:string) => createCatalogI18n(markSixCatalog, props.locale==='zh'?'zh-CN':'en').t(['big','small','odd','even'].includes(key)?`attribute_${key}`:key)
const modes = computed(()=>archive.value?.mark_six ? ['numbers','special','tail','total'] as const : ['numbers'] as const)
const dateText=(value:string)=>{const formatted=formatDisplayDateTime(value,props.locale);return archive.value?.daily?formatted.slice(formatted.indexOf(' ')+1):formatted.split(' ')[0]}
const error = ref<ReturnType<typeof i18n.errorMessage> | null>(null)
let generation = 0
async function load(page = 0) {
  const token = ++generation
  offset.value = page; loading.value = true; error.value = null
  try {
    if (!isUuid(props.gameId)) throw { code: 'REQUEST_INVALID', status: 400 }
    const query = new URLSearchParams({ game_id: props.gameId, kind: props.kind, offset: String(page) })
    if (selected.value) query.set('selected', selected.value)
    const response = await fetch(`${bettingBasePath(props.brandCode)}/draw-archive?${query}`, { credentials: 'same-origin', headers: { Accept: 'application/json' } })
    const envelope = await readApiEnvelope<Archive>(response, (message, status, code) => Object.assign(new Error(message), { status, code }))
    if (token !== generation) return
    if (envelope.data.game_id !== props.gameId) throw { code: 'INVALID_RESPONSE', status: 502 }
    archive.value = envelope.data; selected.value = envelope.data.selected
  } catch (cause) { if (token === generation) { archive.value = null; error.value = i18n.errorMessage(cause, 'read') } }
  finally { if (token === generation) loading.value = false }
}
watch(() => [props.gameId, props.kind, props.brandCode], () => { selected.value = ''; mode.value = 'numbers'; archive.value = null; void load() }, { immediate: true })
onUnmounted(() => { generation++ })
</script>
<template>
  <section class="draw-results draw-archive" data-testid="draw-archive">
    <h2>{{ t('draw.results') }}<template v-if="archive"> · {{ archive.game_name }}</template></h2>
    <Teleport defer to="#user-page-refresh" :disabled="compact"><button v-refresh-button="t('draw.refresh')" type="button" class="button button-secondary" :disabled="loading" @click="withGlobalLoading(load, offset)">{{ t('draw.refresh') }}</button></Teleport>
    <label v-if="archive" class="archive-filter">{{ t(archive.daily ? 'draw.historyDate' : 'draw.historyYear') }}<select v-model="selected" :disabled="loading" @change="withGlobalLoading(load)"><option v-for="choice in archive.choices" :key="choice" :value="choice">{{ choice }}</option></select></label>
    <p v-if="error" class="message error" role="alert">{{ i18n.displayError(error) }}</p>
    <div v-global-loading="loading" class="archive-list" role="table" :aria-label="t('draw.results')">
      <div class="archive-columns" role="row"><span role="columnheader">{{ t('draw.periodNumber') }}</span><span role="columnheader">{{ t('draw.archiveTime') }}</span><div role="columnheader" class="archive-views"><button v-for="view in modes" :key="view" type="button" :aria-pressed="mode===view" @click="mode=view">{{ t(`draw.view.${view}`) }}</button></div></div>
      <div v-for="period in archive?.items" :key="period.period_no" class="archive-row" role="row">
        <strong class="archive-period" role="cell">{{ period.period_no }}</strong>
        <time class="archive-time" role="cell" :datetime="period.drawn_at" :title="formatDisplayDateTime(period.drawn_at,locale)">{{ dateText(period.drawn_at) }}</time>
        <div class="archive-result" role="cell">
          <div v-if="mode==='numbers'" class="archive-numbers" :class="{'is-mark-six':archive!.mark_six}">
            <template v-for="(number,index) in archiveNumbers(period.result)" :key="index">
              <span v-if="number.special" class="archive-plus" :aria-label="t('draw.specialSeparator')">+</span>
              <span class="archive-number"><span class="archive-ball" :class="[period.attributes[String(number.value).padStart(2,'0')]?.wave ? ['mark-six-ball', `lottery-wave-${period.attributes[String(number.value).padStart(2,'0')]!.wave}`] : '',{special:number.special}]">{{ archive!.mark_six ? String(number.value).padStart(2,'0') : number.value }}</span><span v-if="archive!.mark_six" class="archive-zodiac" :title="period.attributes[String(number.value).padStart(2,'0')]?.zodiac ? undefined : t('draw.attributesMissing')">{{ period.attributes[String(number.value).padStart(2,'0')]?.zodiac ? mt(period.attributes[String(number.value).padStart(2,'0')]!.zodiac) : '—' }}</span></span>
            </template>
          </div>
          <div v-else-if="mode==='special'||mode==='tail'" class="archive-facts"><span v-for="(fact,index) in specialFacts(period.result,mode)" :key="index" class="archive-fact">{{ mt(fact) }}</span></div>
          <div v-else class="archive-facts"><strong>{{ totalFacts(period.result,period.attributes).total }}</strong><span class="archive-fact">{{ mt(totalFacts(period.result,period.attributes).parity) }}</span><span class="archive-fact">{{ mt(totalFacts(period.result,period.attributes).size) }}</span><span v-if="totalFacts(period.result,period.attributes).wave" class="archive-fact">{{ mt(totalFacts(period.result,period.attributes).wave!) }}</span></div>
          <small v-if="archive!.mark_six && archiveNumbers(period.result).some(number => !period.attributes[String(number.value).padStart(2,'0')]?.zodiac || !period.attributes[String(number.value).padStart(2,'0')]?.wave)" class="archive-missing">{{ t('draw.attributesMissing') }}</small>
        </div>
      </div>
      <p v-if="!loading && !error && !archive?.items.length" class="empty-state">{{ t('draw.noResults') }}</p>
    </div>
    <div v-if="offset > 0 || archive?.has_more" class="page-actions"><button type="button" class="button button-secondary" :disabled="loading || offset === 0" @click="withGlobalLoading(load, offset - 50)">{{ t('draw.previous') }}</button><span>{{ offset / 50 + 1 }}</span><button type="button" class="button button-secondary" :disabled="loading || !archive?.has_more" @click="withGlobalLoading(load, offset + 50)">{{ t('draw.next') }}</button></div>
  </section>
</template>
<style scoped>
.draw-archive{min-width:0}.draw-archive h2{font-size:18px;overflow-wrap:anywhere;margin:0 0 12px}.archive-filter{display:flex;flex-wrap:wrap;align-items:center;gap:10px;font-size:13px;margin:12px 0}.archive-filter select{width:auto;min-height:44px;max-width:100%;font:inherit;padding:8px 32px 8px 12px;border:1px solid var(--line);border-radius:var(--radius-sm);background:var(--surface);color:var(--ink)}.archive-list{min-width:0;background:var(--surface);border:1px solid var(--line);border-radius:var(--radius-sm)}.archive-columns,.archive-row{display:grid;grid-template-columns:100px 100px minmax(0,1fr);align-items:center;gap:12px;padding:10px 16px}.archive-columns{background:var(--canvas);border-bottom:1px solid var(--line);font-size:12px;color:var(--muted)}.archive-row+.archive-row{border-top:1px solid var(--line)}.archive-row{min-height:72px}.archive-period{font-size:13px;font-weight:600;overflow-wrap:anywhere;font-variant-numeric:tabular-nums}.archive-time{font-size:12px;color:var(--muted);font-variant-numeric:tabular-nums}.archive-result{min-width:0}.archive-missing{display:block;margin-top:6px;font-size:11px;color:var(--muted)}.archive-views{display:flex;flex-wrap:wrap;gap:4px}.archive-views button{appearance:none;border:1px solid var(--line);border-radius:var(--radius-sm);background:var(--surface);color:var(--ink-secondary);min-height:44px;padding:6px 10px;font:inherit;font-size:12px;cursor:pointer}.archive-views button[aria-pressed=true]{background:var(--primary);color:var(--lottery-ball-ink);border-color:var(--primary)}.archive-numbers{display:flex;align-items:center;flex-wrap:wrap;gap:8px}.archive-number{display:grid;justify-items:center;gap:5px;min-width:0;width:32px}.archive-ball{display:inline-grid;place-items:center;width:32px;height:32px;border-radius:50%;background:var(--primary-light);color:var(--primary-dark);font-size:15px;font-weight:750;font-variant-numeric:tabular-nums}.archive-ball.mark-six-ball{color:var(--lottery-ball-ink);background:radial-gradient(circle at 32% 26%,var(--wave-light) 0%,var(--wave-color) 48%,var(--wave-color) 100%)}.archive-zodiac{max-width:100%;overflow-wrap:anywhere;text-align:center;font-size:12px;line-height:1.35;color:var(--ink-secondary)}.archive-plus{font-size:16px;color:var(--muted)}.archive-facts{display:flex;align-items:center;flex-wrap:wrap;gap:8px;font-size:14px}.archive-fact{display:inline-flex;justify-content:center;align-items:center;min-height:30px;min-width:42px;padding:4px 10px;background:var(--primary-light);color:var(--primary-dark);border-radius:var(--radius-sm);font-size:13px}.page-actions{display:flex;align-items:center;justify-content:center;flex-wrap:wrap;gap:12px;margin-top:16px}.page-actions button{min-height:44px}
@media(max-width:620px){.archive-columns,.archive-row{grid-template-columns:minmax(0,1fr) auto;gap:8px;padding:10px 12px}.archive-columns>.archive-views,.archive-result{grid-column:1/-1}.archive-columns>.archive-views{justify-content:flex-start}.archive-columns>span:nth-child(2){text-align:right}.archive-time{text-align:right}.archive-numbers{gap:6px}.archive-ball{width:28px;height:28px;font-size:13px}.archive-number{width:28px}.archive-zodiac{font-size:11px}.archive-row{min-height:84px}.archive-views button{padding-inline:9px}.archive-facts{gap:6px}.archive-fact{min-width:34px;font-size:12px;padding-inline:8px}}
@media(max-width:359px){.archive-number{width:26px}.archive-numbers{gap:4px}.archive-ball{width:26px;height:26px;font-size:12px}}
</style>
