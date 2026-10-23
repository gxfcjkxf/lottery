<script setup lang="ts">
import { computed, ref, useId, watch, nextTick, onMounted, onUnmounted } from 'vue'
import Toast from './Toast.vue'
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
const quickPosition=ref<{left:string;top:string}>()
function positionQuick(){
 if(!quickOpen.value||!pointsInput.value||!quickPopup.value)return
 const input=pointsInput.value.getBoundingClientRect(),popup=quickPopup.value.getBoundingClientRect(),dock=pointsInput.value.closest('.configured-stakes')!.getBoundingClientRect()
 const left=Math.max(8,Math.min(input.left+input.width/2-popup.width/2,window.innerWidth-popup.width-8))
 quickPosition.value={left:`${left}px`,top:`${Math.max(8,dock.top-popup.height-8)}px`}
}
watch(quickPopup,(element,_,cleanup)=>{
 if(!element){quickPosition.value=undefined;return}
 const observer=new ResizeObserver(positionQuick)
 observer.observe(element);observer.observe(pointsInput.value!.closest('.configured-stakes')!)
 void nextTick(positionQuick)
 cleanup(()=>observer.disconnect())
},{flush:'post'})
onMounted(()=>{window.addEventListener('resize',positionQuick);window.addEventListener('scroll',positionQuick,true);window.visualViewport?.addEventListener('resize',positionQuick);window.visualViewport?.addEventListener('scroll',positionQuick)})
onUnmounted(()=>{window.removeEventListener('resize',positionQuick);window.removeEventListener('scroll',positionQuick,true);window.visualViewport?.removeEventListener('resize',positionQuick);window.visualViewport?.removeEventListener('scroll',positionQuick)})
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
</script>
<template>
  <section class="configured-selection" :class="{ 'configured-selection--tabs': sale }" data-testid="configured-selection">
    <label v-if="!sale">{{ t('play') }}<select :disabled="disabled" :value="modelValue.play_id" @change="update('play_id', ($event.target as HTMLSelectElement).value)"><option v-for="p in snapshot.catalog.plays" :key="p.id" :value="p.id">{{ p.label[locale] }}</option></select></label>
    <nav v-else class="configured-play-tabs" role="tablist" aria-orientation="vertical" :aria-label="t('play')">
      <button v-for="(p, index) in snapshot.catalog.plays" :id="`${tabId}-${p.id}`" :key="p.id" type="button" :disabled="disabled" role="tab" :aria-selected="modelValue.play_id === p.id" :aria-controls="`${tabId}-panel`" :tabindex="modelValue.play_id === p.id ? 0 : -1" :class="{ active: modelValue.play_id === p.id }" @click="update('play_id', p.id)" @keydown="moveTab($event, index)">{{ p.label[locale] }}</button>
    </nav>
    <div v-if="play" :id="`${tabId}-panel`" class="configured-selection-content" :role="sale ? 'tabpanel' : undefined" :aria-labelledby="sale ? `${tabId}-${play.id}` : undefined">
      <div class="configured-options"><button v-for="option in options" :key="option.id" type="button" class="button" :class="[modelValue.options.includes(option.id) ? 'button-primary' : 'button-secondary', option.wave ? ['configured-wave-option', `lottery-wave-${option.wave}`] : []]" :aria-pressed="modelValue.options.includes(option.id)" :disabled="disabled || (!modelValue.options.includes(option.id) && (modelValue.options.length >= play.selection.max || (play.selection.distinct_slot && options.some(o => o.slot === option.slot && modelValue.options.includes(o.id)))))" @click="toggle(option.id)"><strong>{{ option.label[locale] }}</strong><span v-if="option.prices.length" class="configured-option-price">{{ option.prices.join(' / ') }}</span></button></div>
      <div class="configured-stakes" :class="{ 'configured-stakes--dock': sale }">
        <div class="configured-selected"><span v-if="availablePoints !== undefined" class="configured-available">{{ t('availablePoints') }}: <strong>{{ availablePoints === null ? '—' : formatPoints(availablePoints, locale) }}</strong></span><span>{{ t('select') }}: {{ modelValue.options.length }}</span><span>{{ t('bet') }}: <output class="configured-total-points" aria-live="polite">{{ totalPoints === null ? '—' : formatPoints(totalPoints, locale) }}</output></span></div>
        <label><span>{{ t('plate') }}</span><select :disabled="disabled" :value="modelValue.plate" @change="update('plate', ($event.target as HTMLSelectElement).value)"><option v-for="plate in snapshot.catalog.plates" :key="plate">{{ plate }}</option></select></label>
        <div v-if="play.stake_mode === 'per_line'" class="configured-points-field" @focusout="!($event.currentTarget as HTMLElement).contains($event.relatedTarget as Node | null) && (quickOpen=false)" @keydown.esc.stop.prevent="quickOpen=false">
          <label><span>{{ t('points') }}</span><input ref="pointsInput" :disabled="disabled" :value="modelValue.points" type="number" inputmode="numeric" min="1" step="1" @focus="quickOpen=true" @click="quickOpen=true" @input="update('points', ($event.target as HTMLInputElement).value)" /></label>
          <div v-if="sale&&quickOpen&&quickAmounts?.length" ref="quickPopup" class="configured-quick-amounts stake-chips" :style="{...quickPosition,visibility:quickPosition?'visible':'hidden'}" role="group" :aria-label="t('quickAmounts')">
            <button v-for="(amount,index) in quickAmounts" :key="index" type="button" class="quick-stake-chip stake-chip" :style="{'--chip-digits':amount.length}" :aria-pressed="modelValue.points===amount" @pointerdown.prevent @click="pickAmount(amount)"><span class="stake-chip-core">{{formatPoints(amount,locale)}}</span></button>
          </div>
        </div>
        <label v-else><span>{{ t('multiplier') }} <small>{{ t('unit_points') }}: {{ play.unit_points }}</small></span><input :disabled="disabled" :value="modelValue.multiplier" type="number" min="1" :max="play.max_multiplier" step="1" @input="update('multiplier', ($event.target as HTMLInputElement).value)" /></label>
        <div v-if="sale||$slots.default" class="configured-dock-actions"><button v-if="sale" type="button" class="button button-secondary configured-clear" :disabled="disabled||modelValue.options.length===0" @click="update('options', [])">{{ t('clear') }}</button><slot :insufficient-points="insufficientPoints" /></div>
        <p v-if="walletFailed" class="configured-funds-warning" role="alert">{{ t('walletUnavailable') }} <button type="button" class="button button-secondary" @click="emit('refresh-wallet')">{{t('retryRead')}}</button></p>
      </div>
      <p v-if="!sale">{{ t('noPrice') }}</p>
    </div>
    <Toast v-if="sale&&fundsToast" class="configured-funds-toast" tone="error" :title="t('insufficientPointsTitle')" :message="t('insufficientPoints')" :close-label="t('dismissNotice')" @dismiss="fundsToast=false" />
  </section>
</template>
<style src="./quick-stake-chips.css"></style>
<style scoped>
.configured-selection{display:grid;gap:12px;min-width:0;font-size:14px}.configured-selection-content{display:grid;gap:12px;min-width:0}.configured-selection--tabs{padding-bottom:calc(196px + env(safe-area-inset-bottom));grid-template-columns:clamp(88px,20vw,120px) minmax(0,1fr);align-items:start;gap:8px}.configured-selection--tabs .configured-play-tabs{max-height:max(48px,calc(100dvh - 52px - var(--configured-dock-bottom,0px) - 196px))}.configured-play-tabs{display:flex;flex-direction:column;position:sticky;top:52px;max-height:calc(100dvh - 136px);overflow-y:auto;min-width:0;border-right:1px solid var(--line);background:var(--canvas)}.configured-play-tabs button{width:100%;min-width:0;min-height:48px;border:0;border-bottom:1px solid var(--line);border-radius:0;padding:10px 6px;font:inherit;font-size:13px;overflow-wrap:anywhere;background:transparent;color:var(--ink);cursor:pointer}.configured-play-tabs button:disabled{cursor:not-allowed}.configured-play-tabs button.active{border-left:3px solid var(--primary);background:var(--primary-light);color:var(--primary-dark)}.configured-selection label{display:grid;gap:6px;min-width:0}.configured-selection input,.configured-selection select{min-height:44px;min-width:0;width:100%;box-sizing:border-box}.configured-options{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(64px,100%),1fr));gap:8px}:global(#app .configured-selection .configured-options .button){width:100%!important;justify-self:stretch}.configured-options button{scroll-margin-top:64px;scroll-margin-bottom:calc(196px + var(--configured-dock-bottom,0px));flex-direction:column;gap:4px;min-width:0;min-height:56px;white-space:normal;overflow-wrap:anywhere}.configured-options .configured-wave-option{background:var(--wave-light);border-color:var(--wave-color)}.configured-options .configured-wave-option strong{color:var(--wave-color)}:global(#app .configured-options .configured-wave-option[aria-pressed="true"]){background:var(--wave-color)!important;border-color:var(--wave-color)!important;color:var(--lottery-ball-ink)!important}.configured-options .configured-wave-option[aria-pressed="true"] strong{color:inherit}.configured-options strong{font-size:14px;line-height:1.4}.configured-options .configured-option-price{color:var(--danger-text);font-size:12px;font-weight:600;line-height:1.4;font-variant-numeric:tabular-nums}.configured-options button[aria-pressed="true"] .configured-option-price{color:inherit}.configured-stakes{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(160px,100%),1fr));gap:12px}
.configured-selected{display:flex;flex-wrap:wrap;align-items:center;gap:4px 8px;margin:0;grid-column:1/-1;min-width:0;overflow-wrap:anywhere}.configured-funds-toast{--toast-bottom:calc(var(--configured-dock-bottom,0px) + 148px)}.configured-funds-warning{grid-column:1/-1;margin:0;color:var(--danger-text);font-size:12px;overflow-wrap:anywhere}.configured-available{display:inline-flex;flex-wrap:wrap;gap:4px}.configured-total-points{color:var(--danger-text);font-weight:750;font-variant-numeric:tabular-nums}.configured-stakes--dock{position:fixed;left:var(--configured-dock-left,0px);right:0;bottom:var(--configured-dock-bottom,0px);z-index:32;display:grid;grid-template-columns:auto minmax(0,240px) auto;align-items:end;justify-content:center;gap:4px 6px;box-sizing:border-box;padding:8px max(8px,env(safe-area-inset-right)) max(8px,env(safe-area-inset-bottom)) max(8px,env(safe-area-inset-left));border-top:1px solid var(--line);background:var(--surface);box-shadow:var(--shadow-md)}.configured-stakes--dock label{display:flex;align-items:center;gap:4px}:global(#app .configured-stakes--dock label){font-size:12px}.configured-stakes--dock label>span{flex:0 1 auto}.configured-stakes--dock label>input{--input-width:10ch;flex:1 1 64px;width:10ch;min-width:56px;padding-inline:6px}.configured-stakes--dock label>select{flex-shrink:0}.configured-stakes--dock small{font-size:11px}.configured-stakes--dock :deep(.button){min-height:44px;white-space:nowrap}.configured-stakes--dock input{font-size:16px}:global(#app .configured-stakes--dock .button-primary:disabled){background:var(--line)!important;border-color:var(--line)!important;color:var(--muted)!important;opacity:1;box-shadow:none}
.configured-dock-actions{display:flex;align-items:center;gap:4px;flex-wrap:nowrap}.configured-points-field{min-width:0}.configured-quick-amounts{position:fixed;width:max-content;max-width:min(340px,calc(100vw - 16px));box-sizing:border-box;z-index:40;display:flex;align-items:center;flex-wrap:wrap;justify-content:center;gap:12px;padding:10px;border:1px solid var(--line);border-radius:var(--radius-sm);background:var(--surface);box-shadow:var(--shadow-lg)}:global(#app .configured-dock-actions .button){padding:8px;min-width:44px;min-height:44px}
@media(min-width:620px){.configured-funds-toast{--toast-bottom:calc(var(--configured-dock-bottom,0px) + 80px)}.configured-stakes--dock{grid-template-columns:auto auto minmax(0,240px) auto}.configured-stakes--dock .configured-selected{grid-column:auto;align-self:center}}
</style>
