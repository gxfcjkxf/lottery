<script setup lang="ts">
import { useOperationToast } from "@lottery/shared";
import { computed, ref, useId, watch, nextTick, onMounted, onUnmounted } from 'vue'
import SelectControl from './SelectControl.vue'
import { createCatalogI18n } from './localization'
import { configuredRuleCatalog } from './configured-rule-catalog'
import { formatPoints } from './points'
import { configuredBetPoints, configuredOptionWave, configuredOptionPrices, type ConfiguredSnapshot, type ConfiguredRequest } from './configured-rules'
const props = defineProps<{ snapshot: ConfiguredSnapshot; modelValue: ConfiguredRequest; locale: 'en' | 'zh-CN';sale?:boolean;disabled?:boolean;availablePoints?:string|null;walletFailed?:boolean;quickAmounts?:readonly string[];rememberedPoints?:string }>()
const emit = defineEmits<{ 'update:modelValue': [request: ConfiguredRequest]; 'refresh-wallet': [] }>()
const i18n = createCatalogI18n(configuredRuleCatalog, props.locale)
watch(() => props.locale, value => i18n.setLocale(value))
const t = i18n.t
const tabId = useId()
const pointsInput=ref<HTMLInputElement>()
const quickOpen=ref(false)
const quickPopup=ref<HTMLDivElement>()
const keyboardInset=ref(0)
const quickPosition=ref<{left:string;top:string;maxWidth:string;maxHeight:string}>()
function positionQuick(){
 if(!quickOpen.value||!pointsInput.value||!quickPopup.value)return
 const viewport=window.visualViewport!
 const input=pointsInput.value.getBoundingClientRect(),popup=quickPopup.value.getBoundingClientRect()
 const dock=pointsInput.value.closest('.configured-stakes')!
 const summary=dock.querySelector<HTMLElement>('.configured-selected')!
 const anchor=summary.offsetHeight ? Math.min(dock.getBoundingClientRect().top,summary.getBoundingClientRect().top) : dock.getBoundingClientRect().top
 const left=Math.max(viewport.offsetLeft+8,Math.min(input.left+input.width/2-popup.width/2,viewport.offsetLeft+viewport.width-popup.width-8))
 quickPosition.value={left:`${left}px`,top:`${Math.max(viewport.offsetTop+8,anchor-popup.height-8)}px`,maxWidth:`${Math.min(340,viewport.width-16)}px`,maxHeight:`${Math.max(0,anchor-viewport.offsetTop-16)}px`}
}
function updateViewport(){
 const viewport=window.visualViewport!
 keyboardInset.value=Math.max(0,window.innerHeight-viewport.height-viewport.offsetTop)
 void nextTick(positionQuick)
}
watch(quickPopup,(element,_,cleanup)=>{
 if(!element){quickPosition.value=undefined;return}
 const observer=new ResizeObserver(positionQuick)
 observer.observe(element);observer.observe(pointsInput.value!.closest('.configured-stakes')!)
 void nextTick(positionQuick)
 cleanup(()=>observer.disconnect())
},{flush:'post'})
onMounted(()=>{updateViewport();window.addEventListener('resize',updateViewport);window.addEventListener('scroll',updateViewport,true);window.visualViewport!.addEventListener('resize',updateViewport);window.visualViewport!.addEventListener('scroll',updateViewport)})
onUnmounted(()=>{window.removeEventListener('resize',updateViewport);window.removeEventListener('scroll',updateViewport,true);window.visualViewport!.removeEventListener('resize',updateViewport);window.visualViewport!.removeEventListener('scroll',updateViewport)})
watch([()=>props.modelValue.play_id,()=>props.disabled],()=>{quickOpen.value=false})
function pickAmount(value:string){update('points',value);quickOpen.value=false;pointsInput.value?.blur()}
const play = computed(() => props.snapshot.catalog.plays.find(p => p.id === props.modelValue.play_id))
const totalPoints = computed(() => play.value ? configuredBetPoints(play.value, props.modelValue) : null)
const insufficientPoints = computed(() => {
  if (props.availablePoints == null || totalPoints.value === null || !play.value) return false
  const factor = props.snapshot.catalog.funding === 'reserve' ? BigInt(play.value.risk_multiplier!) : 1n
  return BigInt(totalPoints.value) * factor > BigInt(props.availablePoints)
})
const fundsToast = ref(false)
watch(insufficientPoints, value => { fundsToast.value = value }, { immediate: true })
const options = computed(() => play.value ? props.snapshot.catalog.domains[play.value.selection.domain]!.options.map(option => ({ ...option, wave: configuredOptionWave(props.snapshot, option), prices: configuredOptionPrices(props.snapshot, play.value!, option, props.modelValue.plate) })) : [])
const plateOptions = computed(() => props.snapshot.catalog.plates.map(value => ({ value, label: value })))
const priceCharacters = computed(() => Math.max(0, ...options.value.map(option => option.prices.join(' / ').length)))
function update(key: keyof ConfiguredRequest, value: string | string[]) {
  const next = { ...props.modelValue, [key]: value }
  if (key === 'play_id') { next.options = []; next.multiplier = '1'; next.points = props.snapshot.catalog.plays.find(p => p.id === value)!.stake_mode === 'unit_multiplier' ? '0' : props.sale ? props.rememberedPoints! : '10' }
  emit('update:modelValue', next)
}
function moveTab(event: KeyboardEvent, index: number) {
  const plays = props.snapshot.catalog.plays
  let next: number
  if (event.key === 'ArrowDown') next = (index + 1) % plays.length
  else if (event.key === 'ArrowUp') next = (index - 1 + plays.length) % plays.length
  else if (event.key === 'Home') next = 0
  else if (event.key === 'End') next = plays.length - 1
  else return
  event.preventDefault()
  update('play_id', plays[next]!.id)
  ;(event.currentTarget as HTMLElement).parentElement!.querySelectorAll('button')[next]!.focus()
}
function toggle(id: string) { const current = props.modelValue.options; update('options', current.includes(id) ? current.filter(value => value !== id) : [...current, id]) }
useOperationToast(fundsToast, () => t('insufficientPoints'), 'error');
</script>
<template>
  <section class="configured-selection" :class="{ 'configured-selection--tabs': sale }" data-testid="configured-selection">
    <label v-if="!sale">{{ t('play') }}<SelectControl :disabled="disabled" :model-value="modelValue.play_id" @change="update('play_id', $event)" :label="String(t('play'))" :options="(snapshot.catalog.plays).map((p) => ({ value: p.id, label: (String(p.label[locale])).trim() }))" /></label>
    <nav v-else class="configured-play-tabs" role="tablist" aria-orientation="vertical" :aria-label="t('play')">
      <button v-for="(p, index) in snapshot.catalog.plays" :id="`${tabId}-${p.id}`" :key="p.id" type="button" :disabled="disabled" role="tab" :aria-selected="modelValue.play_id === p.id" :aria-controls="`${tabId}-panel`" :tabindex="modelValue.play_id === p.id ? 0 : -1" :class="{ active: modelValue.play_id === p.id }" @click="update('play_id', p.id)" @keydown="moveTab($event, index)">{{ p.label[locale] }}</button>
    </nav>
    <div v-if="play" :id="`${tabId}-panel`" class="configured-selection-content" :role="sale ? 'tabpanel' : undefined" :aria-labelledby="sale ? `${tabId}-${play.id}` : undefined">
      <div class="configured-options" :style="{ '--configured-price-characters': priceCharacters }"><button v-for="option in options" :key="option.id" type="button" class="button" :class="[modelValue.options.includes(option.id) ? 'button-primary' : 'button-secondary', option.wave ? ['configured-wave-option', `lottery-wave-${option.wave}`] : []]" :aria-pressed="modelValue.options.includes(option.id)" :disabled="disabled || (!modelValue.options.includes(option.id) && (modelValue.options.length >= play.selection.max || (play.selection.distinct_slot && options.some(o => o.slot === option.slot && modelValue.options.includes(o.id)))))" @click="toggle(option.id)"><span class="configured-option-body"><strong>{{ option.label[locale] }}</strong><span v-if="option.prices.length" class="configured-option-price">{{ option.prices.join(' / ') }}</span></span></button></div>
      <div class="configured-stakes" :class="{ 'configured-stakes--dock': sale }" :style="{'--configured-keyboard-inset':`${keyboardInset}px`}">
        <div class="configured-selected" :class="{'has-selection':modelValue.options.length>0}"><span v-if="availablePoints !== undefined" class="configured-available">{{ t('availablePoints') }}: <strong>{{ availablePoints === null ? '—' : formatPoints(availablePoints, locale) }}</strong></span><span>{{ t('bet') }}: <output class="configured-total-points" aria-live="polite">{{ totalPoints === null ? '—' : formatPoints(totalPoints, locale) }}</output></span></div>
        <label><span>{{ t('plate') }}</span><SelectControl :model-value="modelValue.plate" :options="plateOptions" :label="t('plate')" :disabled="disabled" @update:model-value="update('plate', $event)" /></label>
        <div class="configured-stake-entry">
          <div v-if="play.stake_mode === 'per_line'" class="configured-points-field" @focusout="!($event.currentTarget as HTMLElement).contains($event.relatedTarget as Node | null) && (quickOpen=false)" @keydown.esc.stop.prevent="quickOpen=false">
            <div class="configured-points-control"><span :id="`${tabId}-points-label`">{{ t('points') }}</span><input :aria-labelledby="`${tabId}-points-label`" ref="pointsInput" :disabled="disabled" :value="modelValue.points" type="number" inputmode="numeric" min="1" step="1" @click="quickOpen=true" @input="update('points', ($event.target as HTMLInputElement).value)" /></div>
            <div v-if="sale&&quickOpen&&quickAmounts?.length" ref="quickPopup" class="configured-quick-amounts stake-chips" :style="{...quickPosition,visibility:quickPosition?'visible':'hidden'}" role="group" :aria-label="t('quickAmounts')">
              <button v-for="(amount,index) in quickAmounts" :key="index" type="button" class="quick-stake-chip stake-chip" :style="{'--chip-digits':amount.length}" :aria-pressed="modelValue.points===amount" @pointerdown.prevent @click="pickAmount(amount)"><span class="stake-chip-core">{{formatPoints(amount,locale)}}</span></button>
            </div>
          </div>
          <label v-else><span>{{ t('multiplier') }} <small>{{ t('unit_points') }}: {{ play.unit_points }}</small></span><input :disabled="disabled" :value="modelValue.multiplier" type="number" min="1" :max="play.max_multiplier" step="1" @input="update('multiplier', ($event.target as HTMLInputElement).value)" /></label>
          <span class="configured-selected-count">{{ t('select') }}: {{ modelValue.options.length }}</span>
        </div>
        <div v-if="sale||$slots.default" class="configured-dock-actions"><button v-if="sale" type="button" class="button button-secondary configured-clear" :disabled="disabled||modelValue.options.length===0" @click="update('options', [])">{{ t('clear') }}</button><slot :insufficient-points="insufficientPoints" /></div>
        <p v-if="walletFailed" class="configured-funds-warning" role="alert">{{ t('walletUnavailable') }} <button type="button" class="button button-secondary" @click="emit('refresh-wallet')">{{t('retryRead')}}</button></p>
      </div>

    </div>
  </section>
</template>
<style src="./quick-stake-chips.css"></style>
<style scoped>
.configured-selection{display:grid;gap:12px;min-width:0;font-size:var(--body-font-size)}.configured-selection-content{display:grid;gap:12px;min-width:0}.configured-selection--tabs{padding-bottom:calc(196px + env(safe-area-inset-bottom));grid-template-columns:clamp(88px,20vw,120px) minmax(0,1fr);align-items:start;gap:8px}.configured-selection--tabs .configured-play-tabs{max-height:max(48px,calc(100dvh - 52px - var(--configured-dock-bottom,0px) - 196px))}.configured-play-tabs{display:flex;flex-direction:column;position:sticky;top:52px;max-height:calc(100dvh - 136px);overflow-y:auto;min-width:0;box-sizing:border-box;border:1px solid var(--line);background:var(--canvas)}.configured-play-tabs button{flex-shrink:0;width:100%;min-width:0;min-height:48px;border:0;border-bottom:1px solid var(--line);border-radius:0;padding:10px 6px;font:inherit;font-size:var(--body-font-size);overflow-wrap:anywhere;background:transparent;color:var(--ink);cursor:pointer}.configured-play-tabs button:last-child{border-bottom:0}.configured-play-tabs button:disabled{cursor:not-allowed}.configured-play-tabs button.active{border-left:3px solid var(--primary);background:var(--primary-light);color:var(--primary-dark)}.configured-selection label{display:grid;gap:6px;min-width:0}.configured-selection input{min-height:44px;min-width:0;width:100%;box-sizing:border-box}.configured-options{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(max(64px,calc(var(--configured-price-characters)*1ch + 18px)),100%),1fr));gap:8px;font-variant-numeric:tabular-nums;font-weight:600}:global(#app .configured-selection .configured-options .button){display:block;height:auto;align-self:start;appearance:none;padding-inline:6px;width:100%!important;justify-self:stretch}.configured-options button{scroll-margin-top:64px;scroll-margin-bottom:calc(196px + var(--configured-dock-bottom,0px));min-width:0;min-height:56px;white-space:normal;overflow-wrap:anywhere}.configured-options .configured-wave-option{background:var(--wave-light);border-color:var(--wave-color)}.configured-options .configured-wave-option strong{color:var(--wave-color)}:global(#app .configured-options .configured-wave-option[aria-pressed="true"]){background:var(--wave-color)!important;border-color:var(--wave-color)!important;color:var(--lottery-ball-ink)!important}.configured-options .configured-wave-option[aria-pressed="true"] strong{color:inherit}.configured-option-body{display:block;min-width:0}.configured-options strong{display:block;min-width:0;max-width:100%;font-size:14px;line-height:1.4}.configured-options .configured-option-price{display:block;margin-top:4px;color:var(--danger-text);font-size:var(--body-font-size);font-weight:600;line-height:1.4;font-variant-numeric:tabular-nums;white-space:nowrap;overflow-wrap:normal;word-break:normal}.configured-options button[aria-pressed="true"] .configured-option-price{color:inherit}.configured-stakes{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(160px,100%),1fr));gap:12px}
.configured-selected{display:flex;flex-wrap:wrap;align-items:center;gap:4px 8px;margin:0;grid-column:1/-1;min-width:0;overflow-wrap:anywhere}.configured-funds-warning{grid-column:1/-1;margin:0;color:var(--danger-text);font-size:var(--body-font-size);overflow-wrap:anywhere}.configured-available{display:inline-flex;flex-wrap:wrap;gap:4px}.configured-total-points{color:var(--danger-text);font-weight:750;font-variant-numeric:tabular-nums}.configured-stakes--dock{position:fixed;left:var(--configured-dock-left,0px);right:0;bottom:max(var(--configured-dock-bottom,0px),var(--configured-keyboard-inset,0px));z-index:32;display:flex;flex-wrap:wrap;align-items:center;justify-content:center;gap:4px 6px;box-sizing:border-box;padding:8px max(8px,env(safe-area-inset-right)) max(8px,env(safe-area-inset-bottom)) max(8px,env(safe-area-inset-left));border-top:1px solid var(--line);background:var(--surface);box-shadow:var(--shadow-md)}.configured-stakes--dock label,.configured-stakes--dock .configured-points-control{display:flex;align-items:center;gap:4px}:global(#app .configured-stakes--dock label),.configured-stakes--dock .configured-points-control,.configured-stakes--dock .configured-selected-count{font-size:var(--body-font-size)}.configured-stakes--dock label>span{flex:0 0 auto;white-space:nowrap}.configured-stakes--dock label>input{--input-width:10ch;flex:1 1 64px;width:10ch;min-width:56px;padding-inline:6px}.configured-stakes--dock small{font-size:var(--body-font-size)}.configured-stakes--dock :deep(.button){min-height:var(--control-height);white-space:nowrap}.configured-stakes--dock input{font-size:16px}:global(#app .configured-stakes--dock .button-primary:disabled){background:var(--line)!important;border-color:var(--line)!important;color:var(--muted)!important;opacity:1;box-shadow:none}
.configured-dock-actions{display:flex;align-items:center;gap:4px;flex-wrap:nowrap}.configured-stake-entry{display:flex;flex-wrap:wrap;align-items:center;gap:4px 8px;min-width:0}.configured-selected-count{overflow-wrap:anywhere}.configured-points-field{min-width:0}.configured-points-control{display:grid;gap:4px;min-width:0}:global(#app .configured-stakes--dock .configured-points-control>input){--input-width:calc(6ch + 14px)}.configured-stakes--dock .configured-points-control>input{flex:0 0 auto;width:calc(6ch + 14px);min-width:0;padding-inline:6px}.configured-stakes--dock .configured-points-control>span{white-space:nowrap}.configured-stakes--dock .configured-selected{flex-basis:100%}.configured-stakes--dock .configured-funds-warning{flex-basis:100%}.configured-quick-amounts{overflow:auto;overscroll-behavior:contain;position:fixed;width:max-content;max-width:min(340px,calc(100vw - 16px));box-sizing:border-box;z-index:40;display:flex;align-items:center;flex-wrap:wrap;justify-content:center;gap:12px;padding:10px;border:1px solid var(--line);border-radius:var(--radius-sm);background:var(--surface);box-shadow:var(--shadow-lg)}:global(#app .configured-dock-actions .button){padding:8px;min-width:44px;min-height:var(--control-height)}
@media(min-width:620px){.configured-stakes--dock .configured-selected{flex-basis:auto}.configured-stakes--dock .configured-selected{grid-column:auto;align-self:center}}
@media (min-width:901px) and (pointer:fine) {
 .configured-play-tabs button { min-height:var(--control-height); padding-block:4px; line-height:22px; }
}
</style>
