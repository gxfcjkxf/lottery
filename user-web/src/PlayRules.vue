<script setup lang="ts">
import { computed, ref } from 'vue'
import { createCatalogI18n, type ConfiguredPlay, type ConfiguredSnapshot } from '@lottery/shared'
import DetailModal from '@lottery/shared/DetailModal.vue'
import type { RuleDefinition } from '../../shared/src/rules'
import { describePlayRules } from './play-rules'
import { playRulesCatalog } from './play-rules-catalog'
const props = defineProps<{ locale: 'en' | 'zh'; name: string; snapshot?: ConfiguredSnapshot; play?: ConfiguredPlay; plate?: string; definition?: RuleDefinition }>()
const open = ref(false)
const localeCode = computed(() => props.locale === 'en' ? 'en' : 'zh-CN')
const t = (key: string) => createCatalogI18n(playRulesCatalog, localeCode.value).t(key)
const description = computed(() => {
  try {
    if (props.snapshot && props.play && props.plate) return describePlayRules(localeCode.value, { snapshot: props.snapshot, play: props.play, plate: props.plate })
    if (props.definition) return describePlayRules(localeCode.value, { definition: props.definition })
    throw new Error('Missing play rules')
  } catch { return null }
})
</script>
<template>
  <div class="play-rules-entry"><button type="button" class="text-link" @click="open = true">{{ t('title') }}</button></div>
  <DetailModal v-if="open" class="player-rules-dialog" :title="`${t('title')} · ${name}`" :close-label="t('close')" @close="open = false">
    <p v-if="!description" class="message error" role="alert">{{ t('unavailable') }}</p>
    <template v-else>
      <section><h3>{{ t('how') }}</h3><p v-for="instruction in description.instructions" :key="instruction">{{ instruction }}</p></section>
      <section><h3>{{ t('win') }}</h3><div v-for="(prize, index) in description.prizes" :key="index" class="player-rule-prize"><p>{{ prize.text }}</p><p v-if="prize.detail" class="player-rule-detail">{{ prize.detail }}</p></div></section>
      <details v-if="description.options.length"><summary>{{ t('options') }} · {{ t('odds') }}</summary><dl><div v-for="(option,index) in description.options" :key="index"><dt>{{ option.text }}</dt><dd>{{ option.detail }}</dd></div></dl></details>
    </template>
  </DetailModal>
</template>
<style scoped>
.play-rules-entry{display:flex;justify-content:flex-end;margin:4px 0}.play-rules-entry button{appearance:none;border:0;background:transparent;color:var(--primary);font:inherit;font-size:13px;font-weight:600;min-height:44px;padding:8px 0;cursor:pointer}.player-rules-dialog h3{font-size:15px;margin:0 0 10px}.player-rules-dialog section+section{margin-top:20px}.player-rules-dialog p{font-size:14px;line-height:1.7;margin:6px 0;overflow-wrap:anywhere}.player-rule-prize+.player-rule-prize{border-top:1px solid var(--line);padding-top:10px;margin-top:10px}.player-rule-detail{white-space:pre-line;color:var(--ink-secondary)}.player-rules-dialog summary{min-height:44px;cursor:pointer;font-size:14px;line-height:1.6;padding:12px 0}.player-rules-dialog dl>div{display:grid;grid-template-columns:minmax(64px,1fr) minmax(0,3fr);gap:12px;padding:8px 0;border-top:1px solid var(--line);font-size:13px;line-height:1.6}.player-rules-dialog dd{margin:0;overflow-wrap:anywhere}
</style>
