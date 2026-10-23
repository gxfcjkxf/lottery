<script setup lang="ts">
import SiteIcon from "./SiteIcon";
import { onBeforeUnmount, onMounted, ref } from 'vue'

const props = defineProps<{
  modelValue: string
  options: readonly { code: string; label: string }[]
  label: string
}>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const open = ref(false)
const root = ref<HTMLElement>()
const trigger = ref<HTMLButtonElement>()

function closeOutside(event: PointerEvent) {
  if (event.target instanceof Node && !root.value?.contains(event.target)) open.value = false
}
function select(code: string) {
  emit('update:modelValue', code)
  open.value = false
  trigger.value?.focus()
}
function escape() {
  open.value = false
  trigger.value?.focus()
}
onMounted(() => document.addEventListener('pointerdown', closeOutside))
onBeforeUnmount(() => document.removeEventListener('pointerdown', closeOutside))
</script>

<template>
  <div ref="root" class="locale-picker" :data-locale="modelValue" @keydown.esc.stop.prevent="escape"
    @focusout="!root?.contains($event.relatedTarget as Node | null) && (open = false)">
    <button ref="trigger" type="button" class="locale-picker__trigger" :aria-label="label"
      :title="label" :aria-expanded="open" @click="open = !open">
      <SiteIcon name="language" />
    </button>
    <div v-if="open" class="locale-picker__list" :aria-label="label">
      <button v-for="option in props.options" :key="option.code" type="button"
        class="locale-picker__option" :data-code="option.code" :aria-pressed="modelValue === option.code"
        :lang="option.code === 'zh' ? 'zh-CN' : option.code" @click="select(option.code)">
        <span>{{ option.label }}</span><SiteIcon v-if="modelValue === option.code" name="check" />
      </button>
    </div>
  </div>
</template>

<style scoped>
.locale-picker { position: relative; display: inline-flex; flex-shrink: 0; color: var(--ink, #0f172a); }
.locale-picker__trigger { display: grid; place-items: center; width: var(--icon-control-size, 44px); height: var(--icon-control-size, 44px); padding: 0; border: 1px solid var(--line); border-radius: 10px; background: var(--surface); color: inherit; cursor: pointer; }
.locale-picker__trigger:hover { background: var(--primary-light); color: var(--primary); }
.locale-picker__list { position: absolute; top: calc(100% + 6px); right: 0; z-index: 100; width: 154px; padding: 4px; border: 1px solid var(--line, #e2e8f0); border-radius: 10px; background: var(--surface, #ffffff); box-shadow: 0 10px 25px rgb(15 23 42 / 12%); }
.locale-picker__option { display: flex; align-items: center; justify-content: space-between; width: 100%; min-height: 44px; padding: 8px 10px; border: 0; border-radius: 6px; background: transparent; color: inherit; font: inherit; font-size: 14px; text-align: left; cursor: pointer; }
.locale-picker__option:hover, .locale-picker__option[aria-pressed="true"] { background: var(--canvas, #f8fafc); }
.locale-picker__option[aria-pressed="true"] { font-weight: 700; }
.locale-picker button:focus-visible { outline: 2px solid var(--brand-primary, #059669); outline-offset: 2px; }
</style>
