<script setup lang="ts" generic="T extends string | number | boolean | undefined">
import { computed, nextTick, onBeforeUnmount, ref, useAttrs, useId, watch } from 'vue'
import type { StyleValue } from 'vue'
import SiteIcon from './SiteIcon'
import ClearableInput from './ClearableInput.vue'

defineOptions({ inheritAttrs: false })
const attrs = useAttrs()
const props = defineProps<{
  search?: { value: string; locale: 'en' | 'zh-CN' }
  modelValue: T
  options: readonly { value: string | number | boolean; label: string; disabled?: boolean; group?: string }[]
  placeholder?: string
  label: string
  disabled?: boolean
  required?: boolean
  name?: string
}>()
const emit = defineEmits<{ 'update:modelValue': [value: T]; change: [value: T]; search: [value: string, event: Event] }>()
const id = useId()
const trigger = ref<HTMLElement>()
const searchText = ref(props.search?.value ?? '')
watch(() => props.search?.value, value => { if (value !== undefined) searchText.value = value })
function editSearch(event: Event) {
  emit('search', searchText.value, event)
  void show()
}
function blur(event: FocusEvent) {
  if (!trigger.value?.contains(event.relatedTarget as Node) && !list.value?.contains(event.relatedTarget as Node)) close()
}
const list = ref<HTMLDivElement>()
const validation = ref<HTMLInputElement>()
const open = ref(false)
const active = ref(-1)
const selected = computed(() => props.options.find(option => option.value === props.modelValue))
const groups = computed(() => {
  const result: { label?: string; items: { option: (typeof props.options)[number]; index: number }[] }[] = []
  props.options.forEach((option, index) => {
    if (!result.length || result[result.length - 1]!.label !== option.group) result.push({ label: option.group, items: [] })
    result[result.length - 1]!.items.push({ option, index })
  })
  return result
})
let typed = '', typedAt = 0

function close() {
  if (list.value?.matches(':popover-open')) list.value.hidePopover()
  open.value = false
  typed = ''
  cleanup()
}
function position() {
  if (!open.value || !trigger.value || !list.value) return
  const viewport = window.visualViewport
  const left = (viewport?.offsetLeft ?? 0) + 8, top = (viewport?.offsetTop ?? 0) + 8
  const right = left + (viewport?.width ?? innerWidth) - 16
  const bottom = top + (viewport?.height ?? innerHeight) - 16
  const anchor = trigger.value.getBoundingClientRect()
  const below = bottom - anchor.bottom - 4, above = anchor.top - top - 4
  const popup = list.value
  popup.style.minWidth = `${Math.min(anchor.width, right - left)}px`
  popup.style.maxWidth = `${right - left}px`
  const availableHeight = Math.max(0, Math.max(below, above))
  popup.style.maxHeight = `${props.search ? Math.min(240, availableHeight) : availableHeight}px`
  const box = popup.getBoundingClientRect()
  popup.style.left = `${Math.max(left, Math.min(anchor.left, right - box.width))}px`
  popup.style.top = `${below >= box.height ? anchor.bottom + 4 : Math.max(top, anchor.top - box.height - 4)}px`
}
function reveal() {
  list.value?.querySelector<HTMLElement>(`[data-index="${active.value}"]`)?.scrollIntoView({ block: 'nearest' })
}
async function show() {
  if (props.disabled || trigger.value?.matches(':disabled') || (!props.search && !props.options.some(option => !option.disabled))) return
  if (open.value) return
  active.value = props.options.findIndex(option => option.value === props.modelValue && !option.disabled)
  if (props.search) active.value = -1
  else if (active.value < 0) active.value = props.options.findIndex(option => !option.disabled)
  list.value!.showPopover()
  open.value = true
  if (!props.search) trigger.value!.focus({ preventScroll: true })
  window.addEventListener('resize', position)
  window.addEventListener('scroll', position, true)
  window.visualViewport?.addEventListener('resize', position)
  window.visualViewport?.addEventListener('scroll', position)
  await nextTick()
  position()
  reveal()
}
function choose(index: number) {
  const option = props.options[index]
  if (!option || option.disabled || trigger.value?.matches(':disabled')) return
  const changed = option.value !== props.modelValue
  emit('update:modelValue', option.value as T)
  if (changed) emit('change', option.value as T)
  nextTick(() => { validation.value?.dispatchEvent(new Event('change', { bubbles: true })) })
  const focusTarget = props.search ? trigger.value?.querySelector('input') : trigger.value
  focusTarget?.focus({ preventScroll: true })
  close()
}
async function navigate(event: KeyboardEvent) {
  if (props.disabled) return
  if (event.key === 'Escape' && open.value) { event.preventDefault(); event.stopPropagation(); close(); return }
  if (event.key === 'Tab') { if (open.value) { if (active.value >= 0) choose(active.value); else close() } return }
  if (event.key === 'Enter' || (!props.search && event.key === ' ')) {
    event.preventDefault()
    if (open.value) choose(active.value)
    else await show()
    return
  }
  if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
    event.preventDefault()
    const wasOpen = open.value
    if (!wasOpen) await show()
    const enabled = props.options.flatMap((option, index) => option.disabled ? [] : [index])
    if (!enabled.length) return
    const current = enabled.indexOf(active.value)
    if (event.key === 'Home') active.value = enabled[0]!
    else if (event.key === 'End') active.value = enabled[enabled.length - 1]!
    else if (wasOpen || props.search) active.value = enabled[Math.max(0, Math.min(enabled.length - 1, current + (event.key === 'ArrowDown' ? 1 : -1)))]!
    reveal()
    return
  }
  if (!props.search && event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
    event.preventDefault()
    if (!open.value) await show()
    const now = Date.now()
    typed = now - typedAt > 700 ? event.key : typed + event.key
    typedAt = now
    const index = props.options.findIndex(option => !option.disabled && option.label.toLocaleLowerCase().startsWith(typed.toLocaleLowerCase()))
    if (index >= 0) { active.value = index; reveal() }
  }
}
function cleanup() {
  window.removeEventListener('resize', position)
  window.removeEventListener('scroll', position, true)
  window.visualViewport?.removeEventListener('resize', position)
  window.visualViewport?.removeEventListener('scroll', position)
}
watch(() => props.disabled, value => { if (value) close() })
watch(() => JSON.stringify(props.options), async () => {
  if (!props.search) { close(); return }
  active.value = -1
  await nextTick(); position()
})
watch(() => props.modelValue, () => { if (!props.search) close() })
onBeforeUnmount(close)
</script>

<template>
  <span class="select-control" :class="attrs.class" :style="attrs.style as StyleValue">
    <span v-if="search" ref="trigger" class="select-control-trigger select-control-search" role="combobox" :aria-label="label" aria-haspopup="listbox" :aria-expanded="open" :aria-controls="`${id}-list`" :aria-activedescendant="open && active >= 0 ? `${id}-option-${active}` : undefined" @keydown="navigate" @focusout="blur">
      <ClearableInput v-bind="{ ...attrs, class: undefined, style: undefined }" v-model="searchText" :locale="search.locale" :aria-label="label" :placeholder="placeholder || label" :disabled="disabled" @input="editSearch" @focus="show" @click="show" />
      <button type="button" class="select-control-search-toggle" :disabled="disabled" :aria-label="label" :aria-expanded="open" @mousedown.prevent @click="open ? close() : show()"><SiteIcon name="down" /></button>
    </span>
    <button v-else v-bind="{ ...attrs, class: undefined, style: undefined }" ref="trigger" type="button" class="select-control-trigger" tabindex="0" role="combobox" :disabled="disabled"
      :data-value="modelValue === undefined ? '' : String(modelValue)" :aria-label="label" :aria-required="required || undefined" aria-haspopup="listbox" :aria-expanded="open" :aria-controls="`${id}-list`"
      :aria-activedescendant="open && active >= 0 ? `${id}-option-${active}` : undefined"
      @click="open ? close() : show()" @keydown="navigate" @blur="close">
      <span class="select-control-value"><span v-for="option in options" :key="`${typeof option.value}:${option.value}`" class="select-control-measure" aria-hidden="true">{{ option.label }}</span><span v-if="placeholder" class="select-control-measure" aria-hidden="true">{{ placeholder }}</span><span>{{ modelValue === '' && placeholder ? placeholder : selected?.label }}</span></span>
      <SiteIcon name="down" />
    </button>
    <input ref="validation" class="select-control-validation" data-select-validation type="text" tabindex="-1" aria-hidden="true"
      aria-label="" :name="name" :value="selected ? String(selected.value) : ''" :required="required" :disabled="disabled"
      @focus="trigger?.focus()" @invalid.prevent="trigger?.focus()" />
    <div :id="`${id}-list`" ref="list" popover="auto" role="listbox" :aria-label="label" class="select-control-list"
      @toggle="!($event.target as HTMLElement).matches(':popover-open') && close()" @mousedown.prevent>
      <div v-for="(group, groupIndex) in groups" :key="groupIndex" :role="group.label ? 'group' : 'presentation'" :aria-label="group.label">
      <div v-if="group.label" class="select-control-group" aria-hidden="true">{{ group.label }}</div>
      <div v-for="{ option, index } in group.items" :id="`${id}-option-${index}`" :key="`${typeof option.value}:${option.value}`" role="option"
        :data-index="index" :data-value="String(option.value)" :aria-selected="option.value === modelValue" :aria-disabled="option.disabled || undefined"
        class="select-control-option" :class="{ 'is-active': index === active }"
        @pointermove="!option.disabled && (active = index)" @click.prevent="choose(index)">
        <span>{{ option.label }}</span><SiteIcon v-if="option.value === modelValue" name="check" />
      </div>
      </div>
      <slot name="footer" />
    </div>
  </span>
</template>

<style scoped>
.select-control{position:relative;display:inline-flex;min-width:0;max-width:100%;flex:0 1 auto;justify-self:start;font-size:var(--body-font-size)}
.select-control-trigger{display:flex;align-items:center;justify-content:space-between;gap:8px;box-sizing:border-box;width:max-content;min-width:0;max-width:100%;min-height:var(--control-height);padding:4px 10px;border:1px solid var(--line);border-radius:var(--radius-sm);background:var(--surface);color:var(--ink);font:inherit;line-height:22px;text-align:start;cursor:pointer;transition:color .15s,border-color .15s,background-color .15s,box-shadow .15s}
.select-control-trigger:hover:not(:disabled),.select-control-trigger[aria-expanded="true"]{border-color:var(--primary);background:var(--primary-light)}
.select-control-trigger:focus-visible{outline:2px solid var(--primary);outline-offset:2px}
.select-control-trigger:disabled{opacity:.5;cursor:not-allowed}
.select-control-validation{position:absolute!important;inset:0 auto auto 0!important;width:1px!important;height:1px!important;min-width:0!important;min-height:0!important;padding:0!important;border:0!important;opacity:0;pointer-events:none}
.select-control-group{padding:6px 8px;color:var(--muted);font-weight:600;font-size:var(--body-font-size);overflow-wrap:anywhere}
.select-control:has(.inline-validation-invalid) .select-control-trigger{border-color:var(--danger)}
.select-control-value{display:grid;min-width:0;overflow-wrap:anywhere}
.select-control-value>span{grid-area:1/1}
.select-control-measure{visibility:hidden}
.select-control-search{padding:0;gap:0;width:var(--input-width,168px)}
.select-control-search :deep(.clearable-input){flex:1;min-width:0}
.select-control-search :deep(input){border:0!important;box-shadow:none!important;background:transparent!important;outline-offset:-2px}
.select-control-search-toggle{display:flex;align-items:center;justify-content:center;flex:0 0 var(--control-height);width:var(--control-height);height:var(--control-height);padding:0;border:0;background:transparent;color:var(--muted);cursor:pointer}
.select-control-search:focus-within{border-color:var(--primary);box-shadow:0 0 0 2px var(--primary-light)}
.select-control-trigger>.site-icon{color:var(--muted)}
.select-control-list{position:fixed;inset:auto;margin:0;padding:4px;box-sizing:border-box;width:max-content;overflow:auto;overscroll-behavior:contain;border:1px solid var(--line);border-radius:var(--radius-sm);background:var(--surface);color:var(--ink);box-shadow:var(--shadow-lg);font:inherit;font-size:var(--body-font-size)}
.select-control-list:empty{visibility:hidden}
.select-control-option{display:flex;align-items:center;justify-content:space-between;gap:12px;box-sizing:border-box;min-height:var(--control-height);padding:4px 8px;border-radius:var(--radius-sm);line-height:22px;cursor:pointer;overflow-wrap:anywhere}
.select-control-option>span{min-width:0;flex:1}
.select-control-option[aria-selected="true"]{background:var(--primary-light);color:var(--primary-dark);font-weight:600}
.select-control-option.is-active{background:var(--canvas);box-shadow:inset 0 0 0 1px var(--line)}
.select-control-option.is-active[aria-selected="true"]{background:var(--primary-light)}
.select-control-option[aria-disabled="true"]{opacity:.5;cursor:not-allowed}
@media(max-width:900px),(pointer:coarse){.select-control-trigger,.select-control-list{font-size:16px}}
</style>
