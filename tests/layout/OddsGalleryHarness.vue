<script setup lang="ts">
import { computed, onMounted, onUnmounted, provide, shallowRef } from 'vue'
import ConfiguredSelection from '../../shared/src/ConfiguredSelection.vue'
import { type ConfiguredSnapshot } from '../../shared/src/configured-rules'
import { createUserI18n, userI18nKey } from '../../user-web/src/i18n'
const props = defineProps<{ initialSnapshot: ConfiguredSnapshot; locale: 'en' | 'zh-CN' }>()
const i18n = createUserI18n(); i18n.setLocale(props.locale); provide(userI18nKey, i18n)
const current = shallowRef({ name: 'initial', snapshot: props.initialSnapshot })
const cases = computed(() => current.value.snapshot.catalog.plates.flatMap(plate => current.value.snapshot.catalog.plays.map(play => ({ plate, play }))))
const update = (event: Event) => { current.value = (event as CustomEvent).detail }
onMounted(() => window.addEventListener('odds-gallery-catalog', update))
onUnmounted(() => window.removeEventListener('odds-gallery-catalog', update))
</script>
<template>
  <main class="odds-gallery" :data-catalog="current.name">
    <section v-for="entry in cases" :key="`${current.name}:${entry.plate}:${entry.play.id}`" class="odds-gallery-case" :data-case="`${entry.plate}:${entry.play.id}`">
      <ConfiguredSelection sale :snapshot="current.snapshot" :locale="locale" :model-value="{ play_id:entry.play.id, plate:entry.plate, options:[], points:'10', multiplier:'1' }" />
    </section>
  </main>
</template>
<style scoped>
.odds-gallery{box-sizing:border-box;padding:12px;min-width:0;width:100%}
/* Each case uses the sales cards; the surrounding page supplies the tab-column width. */
.odds-gallery-case :deep(.configured-selection){display:block;padding-bottom:0}
.odds-gallery-case :deep(.configured-play-tabs),.odds-gallery-case :deep(.configured-stakes){display:none}
.odds-gallery-case{margin-left:calc(clamp(88px,20vw,120px) + 8px);min-width:0;margin-bottom:12px}
@media(min-width:1000px){.odds-gallery{margin-left:220px;width:calc(100% - 220px);padding-inline:24px}}
</style>
