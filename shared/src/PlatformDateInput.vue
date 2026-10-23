<script setup lang="ts">
import SiteIcon from "./SiteIcon";
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import type { PlatformDisplaySettings } from './date-time'
import { createCatalogI18n } from './localization'
import { platformLotteryCatalog } from './platform-lottery'

const props = defineProps<{ modelValue: string; displaySettings: PlatformDisplaySettings; label: string; locale: 'en' | 'zh-CN'; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const i18n = createCatalogI18n(platformLotteryCatalog, props.locale)
watch(() => props.locale, value => i18n.setLocale(value))
const t = i18n.t
const expanded = ref(false)
const control = ref<HTMLElement | null>(null)
const calendar = ref<HTMLElement | null>(null)
const trigger = ref<HTMLButtonElement | null>(null)
const position = ref({ left: '0px', top: '0px' })
const month = ref(0), year = ref(2026)
const displayValue = computed(() => {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(props.modelValue)) return ''
  const [year, month, day] = props.modelValue.split('-')
  if (props.displaySettings.date_format === 'DD/MM/YYYY') return `${day}/${month}/${year}`
  if (props.displaySettings.date_format === 'MM/DD/YYYY') return `${month}/${day}/${year}`
  return `${year}-${month}-${day}`
})
function today() {
  const parts = Object.fromEntries(new Intl.DateTimeFormat('en', { timeZone: props.displaySettings.timezone, year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(new Date()).map(part => [part.type, part.value]))
  return `${parts.year}-${parts.month}-${parts.day}`
}
function toggle() {
  if (props.disabled) return
  if (expanded.value) { close(); return }
  const parts = (props.modelValue || today()).split('-').map(Number)
  year.value = parts[0]; month.value = parts[1] - 1
  calendar.value!.showPopover()
  expanded.value = true
  placeCalendar()
}
function placeCalendar() {
  if (!expanded.value) return
  const anchor = control.value!.getBoundingClientRect()
  const panel = calendar.value!.getBoundingClientRect()
  const below = anchor.bottom + 6
  const top = below + panel.height <= window.innerHeight - 12 ? below : anchor.top - panel.height - 6
  position.value = {
    left: `${Math.max(12, Math.min(anchor.left, window.innerWidth - panel.width - 12))}px`,
    top: `${Math.max(12, Math.min(top, window.innerHeight - panel.height - 12))}px`,
  }
}
function close() { calendar.value!.hidePopover(); expanded.value = false }
function syncPopover(event: Event) { expanded.value = (event as Event & { newState: string }).newState === 'open' }
watch(expanded, value => {
  if (value) { window.addEventListener('resize', placeCalendar); window.addEventListener('scroll', placeCalendar, true) }
  else { window.removeEventListener('resize', placeCalendar); window.removeEventListener('scroll', placeCalendar, true) }
})
watch([month, year, () => props.locale], placeCalendar, { flush: 'post' })
onBeforeUnmount(() => { window.removeEventListener('resize', placeCalendar); window.removeEventListener('scroll', placeCalendar, true) })
const monthTitle = computed(() => new Intl.DateTimeFormat(props.locale, { timeZone: 'UTC', year: 'numeric', month: 'long' }).format(new Date(Date.UTC(year.value, month.value, 1))))
const weekdays = computed(() => Array.from({ length: 7 }, (_, day) => new Intl.DateTimeFormat(props.locale, { timeZone: 'UTC', weekday: 'short' }).format(new Date(Date.UTC(2026, 0, 4 + day)))))
const days = computed(() => {
  const first = new Date(Date.UTC(year.value, month.value, 1)).getUTCDay()
  const count = new Date(Date.UTC(year.value, month.value + 1, 0)).getUTCDate()
  return [...Array.from({ length: first }, () => 0), ...Array.from({ length: count }, (_, index) => index + 1)]
})
function moveMonth(offset: number) {
  const next = new Date(Date.UTC(year.value, month.value + offset, 1))
  year.value = next.getUTCFullYear(); month.value = next.getUTCMonth()
}
function dateValue(day: number) { return `${year.value}-${String(month.value + 1).padStart(2, '0')}-${String(day).padStart(2, '0')}` }
function choose(value: string) { if (props.disabled) return; emit('update:modelValue', value); close(); trigger.value!.focus() }
watch(() => props.disabled, value => { if (value && expanded.value) close() })
</script>

<template>
  <span class="platform-date-input" :lang="locale" @keydown.esc.stop.prevent="close">
    <span ref="control" class="platform-date-input-control">
      <input class="platform-date-input-display" :value="displayValue" :placeholder="displaySettings.date_format" type="text" :aria-label="label" readonly tabindex="-1" @click="toggle" />
      <button ref="trigger" class="platform-date-input-button" type="button" :disabled="disabled" :aria-label="label" :aria-expanded="expanded" @click="toggle"><SiteIcon name="calendar" /></button>
    </span>
    <span ref="calendar" popover="auto" class="platform-calendar" :style="position" role="group" :aria-label="label" @toggle="syncPopover">
      <span class="platform-calendar-heading"><button type="button" :aria-label="t('calendarPreviousMonth')" @click="moveMonth(-1)"><SiteIcon name="back" /></button><strong aria-live="polite">{{ monthTitle }}</strong><button type="button" :aria-label="t('calendarNextMonth')" @click="moveMonth(1)"><SiteIcon name="next" /></button></span>
      <span class="platform-calendar-grid"><span v-for="day in weekdays" :key="day" class="platform-calendar-weekday">{{ day }}</span><template v-for="(day, index) in days" :key="index"><button v-if="day" type="button" :aria-label="dateValue(day)" :aria-pressed="modelValue === dateValue(day)" @click="choose(dateValue(day))">{{ day }}</button><span v-else /></template></span>
      <button class="platform-calendar-today" type="button" @click="choose(today())">{{ t('calendarToday') }}</button>
    </span>
  </span>
</template>

<style scoped>
.platform-date-input { display: block; width: 100%; min-width: 0; }
.platform-date-input-control { display: block; position: relative; }
.platform-date-input-display { width: 100%; min-width: 0; min-height: 40px; box-sizing: border-box; padding: 8px 44px 8px 8px; font: inherit; border: 1px solid var(--line); border-radius: var(--radius-sm); background: var(--surface); color: var(--ink); cursor: pointer; }
.platform-date-input-button { position: absolute; top: 0; right: 0; bottom: 0; width: 40px; display: grid; place-items: center; padding: 0; border: 0; border-radius: var(--radius-sm); background: transparent; color: var(--ink-secondary); cursor: pointer; }
.platform-calendar { position: fixed; inset: auto; box-sizing: border-box; width: min(340px, calc(100vw - 24px)); max-height: calc(100dvh - 24px); overflow: auto; margin: 0; padding: 8px; border: 1px solid var(--line); border-radius: var(--radius-sm); background: var(--surface); color: var(--ink); box-shadow: var(--shadow-lg); }
.platform-calendar:popover-open { display: block; }
.platform-calendar-heading { display: flex; align-items: center; justify-content: space-between; gap: 4px; font-size: 13px; }
.platform-calendar button { min-width: 0; min-height: 36px; padding: 4px; border: 0; border-radius: var(--radius-sm); background: transparent; color: var(--ink); font: inherit; cursor: pointer; }
.platform-calendar-heading button { width: 36px; }
.platform-calendar-grid { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); gap: 2px; font-size: 13px; }
.platform-calendar-weekday { padding: 8px 0; text-align: center; color: var(--muted); font-size: 11px; }
.platform-calendar button:hover { background: var(--primary-light); }
.platform-calendar button[aria-pressed="true"] { background: var(--primary); color: var(--surface); }
.platform-calendar .platform-calendar-today { width: 100%; margin-top: 4px; color: var(--primary); }
@media (max-width: 640px) { .platform-date-input-display { min-height: 44px; } .platform-date-input-button { width: 44px; } .platform-calendar button { min-height: 44px; } }
</style>
