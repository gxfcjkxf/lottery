<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { AuthChallenge } from './auth'
import { captchaCatalog } from './captcha-catalog'
const props = defineProps<{ challenge: AuthChallenge; locale: 'en' | 'zh-CN'; disabled?: boolean }>()
const answer = defineModel<string>({ required: true })
const position = ref(0)
const text = computed(() => captchaCatalog[props.locale])
const puzzle = computed(() => props.challenge.puzzle)
const maximum = computed(() => puzzle.value.width - puzzle.value.piece_width)
watch(() => props.challenge.id, () => { position.value = 0; answer.value = '' })
function move(event: Event) {
 position.value = Number((event.target as HTMLInputElement).value)
 answer.value = ''
}
function place() { answer.value = position.value > 0 ? String(position.value) : '' }
</script>
<template>
 <div class="puzzle-captcha">
  <div class="puzzle-captcha__image" :style="{ aspectRatio: `${puzzle.width}/${puzzle.height}` }">
   <img :src="puzzle.image" :alt="text.title" draggable="false" />
   <img class="puzzle-captcha__piece" :src="puzzle.piece" alt="" draggable="false" :style="{ width: `${puzzle.piece_width/puzzle.width*100}%`, height: `${puzzle.piece_height/puzzle.height*100}%`, left: `${position/puzzle.width*100}%`, top: `${puzzle.y/puzzle.height*100}%` }" />
  </div>
  <div class="puzzle-captcha__track">
   <span aria-hidden="true" :class="{'puzzle-captcha__hint-hidden': position > 0}">{{ text.slide }}</span>
   <input type="range" min="0" :max="maximum" step="1" :value="position" :disabled="disabled" :aria-label="text.slide" :aria-valuetext="String(position)" @input="move" @change="place" @pointercancel="answer = ''" />
  </div>
 </div>
</template>
<style scoped>
.puzzle-captcha{display:grid;gap:10px;min-width:0;width:100%}
.puzzle-captcha__image{position:relative;overflow:hidden;border-radius:var(--radius-sm);background:var(--line-subtle)}
.puzzle-captcha__image>img{display:block;width:100%;height:100%;user-select:none}
.puzzle-captcha__image>.puzzle-captcha__piece{position:absolute;filter:drop-shadow(0 2px 3px var(--ink-secondary))}
.puzzle-captcha__track{position:relative;display:grid;align-items:center;min-height:46px;border:1px solid var(--line);border-radius:var(--radius-sm);background:var(--surface);color:var(--ink-secondary)}
.puzzle-captcha__track>span{text-align:center;font-size:var(--body-font-size);pointer-events:none;padding-inline:44px 8px}
.puzzle-captcha__hint-hidden{visibility:hidden}
.puzzle-captcha__track>input[type="range"]{position:absolute;inset:0;width:100%;height:44px;min-height:44px;margin:0;padding:0;border:0;box-shadow:none;appearance:none;background:transparent;transition:none;cursor:grab;touch-action:pan-y;border-radius:inherit}
.puzzle-captcha__track>input[type="range"]:active{cursor:grabbing}
.puzzle-captcha__track>input[type="range"]:focus{box-shadow:none !important}
.puzzle-captcha__track>input[type="range"]:focus-visible{outline:2px solid var(--brand-primary) !important;outline-offset:3px}
.puzzle-captcha__track>input[type="range"]::-webkit-slider-thumb{appearance:none;width:44px;height:44px;border:0;border-radius:var(--radius-sm);background:var(--brand-primary) var(--slider-chevron) center no-repeat}
.puzzle-captcha__track>input[type="range"]::-moz-range-thumb{width:44px;height:44px;border:0;border-radius:var(--radius-sm);background:var(--brand-primary)}
.puzzle-captcha__track>input[type="range"]:disabled{opacity:.5;cursor:wait}
</style>
