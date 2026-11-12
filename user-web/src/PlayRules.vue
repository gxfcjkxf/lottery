<script setup lang="ts">
import { computed, ref } from 'vue'
import { createCatalogI18n, type ConfiguredPlay, type ConfiguredSnapshot } from '@lottery/shared'
import SiteIcon from '@lottery/shared/SiteIcon'
import DetailModal from '@lottery/shared/DetailModal.vue'
import type { RuleDefinition } from '../../shared/src/rules'
import { describePlayRules,describePlayGroup } from './play-rules'
import { playRulesCatalog } from './play-rules-catalog'
import type {ConfiguredSelectionGroup} from '../../shared/src/configured-selection-groups'
const props = defineProps<{ locale: 'en' | 'zh'; name: string; snapshot?: ConfiguredSnapshot; play?: ConfiguredPlay; plate?: string; group?:ConfiguredSelectionGroup; definition?: RuleDefinition }>()
const open = ref(false)
const localeCode = computed(() => props.locale === 'en' ? 'en' : 'zh-CN')
const t = (key: string) => createCatalogI18n(playRulesCatalog, localeCode.value).t(key)
const grouped=computed(()=>props.group&&props.group.plays.length>1)
const groupDescription=computed(()=>{if(!grouped.value)return null;try{return describePlayGroup(localeCode.value,props.snapshot!,props.group!,props.plate!)}catch{return null}})
const description = computed(() => {
  try {
    if (props.snapshot && props.play && props.plate) return describePlayRules(localeCode.value, { snapshot: props.snapshot, play: props.play, plate: props.plate })
    if (props.definition) return describePlayRules(localeCode.value, { definition: props.definition })
    throw new Error('Missing play rules')
  } catch { return null }
})
</script>
<template>
  <div class="play-rules-entry"><button type="button" class="text-link text-button" tabindex="0" aria-haspopup="dialog" @click="open = true">{{ t('title') }}</button></div>
  <DetailModal v-if="open" class="player-rules-dialog" :title="`${t('title')} · ${name}`" :close-label="t('close')" @close="open = false">
    <template v-if="grouped&&groupDescription">
      <section><h3>{{t('how')}}</h3><p v-for="instruction in groupDescription.instructions" :key="instruction">{{instruction}}</p><p v-for="note in groupDescription.notes" :key="note">{{note}}</p></section>
      <section><h3>{{t('win')}} · {{t('odds')}}</h3>
        <details v-for="entry in groupDescription.entries" :key="entry.id" class="player-group-rule" :data-play-id="entry.id">
          <summary><span>{{entry.label}}</span><span class="player-group-price">{{entry.prices.map(price=>`× ${price}`).join(' / ')}}</span><SiteIcon name="down" aria-hidden="true" /></summary>
          <div v-for="(prize,index) in entry.rules.prizes" :key="index" class="player-rule-prize"><p>{{prize.text}}</p><p v-if="prize.detail" class="player-rule-detail">{{prize.detail}}</p></div>
        </details>
      </section>
    </template>
    <p v-else-if="grouped||!description" class="message error" role="alert">{{ t('unavailable') }}</p>
    <template v-else>
      <section><h3>{{ t('how') }}</h3><p v-for="instruction in description.instructions" :key="instruction">{{ instruction }}</p></section>
      <section><h3>{{ t('win') }}</h3><div v-for="(prize, index) in description.prizes" :key="index" class="player-rule-prize"><p>{{ prize.text }}</p><p v-if="prize.detail" class="player-rule-detail">{{ prize.detail }}</p></div></section>
      <details v-if="description.options.length"><summary>{{ t('options') }} · {{ t('odds') }}</summary><dl><div v-for="(option,index) in description.options" :key="index"><dt>{{ option.text }}</dt><dd>{{ option.detail }}</dd></div></dl></details>
    </template>
  </DetailModal>
</template>
<style scoped>
.play-rules-entry{display:flex;justify-content:flex-end;margin:4px 0}.player-rules-dialog h3{font-size:15px;margin:0 0 10px}.player-rules-dialog section+section{margin-top:20px}.player-rules-dialog p{font-size:14px;line-height:1.7;margin:6px 0;overflow-wrap:anywhere}.player-rule-prize+.player-rule-prize{border-top:1px solid var(--line);padding-top:10px;margin-top:10px}.player-rule-detail{white-space:pre-line;color:var(--ink-secondary)}.player-rules-dialog summary{min-height:44px;cursor:pointer;font-size:14px;line-height:1.6;padding:12px 0}.player-rules-dialog dl>div{display:grid;grid-template-columns:minmax(64px,1fr) minmax(0,3fr);gap:12px;padding:8px 0;border-top:1px solid var(--line);font-size:var(--body-font-size);line-height:1.6}.player-rules-dialog dd{margin:0;overflow-wrap:anywhere}
.player-group-rule{border-top:1px solid var(--line)}.player-group-rule summary{display:flex;justify-content:space-between;align-items:center;gap:8px;padding:8px 0;min-height:var(--control-height);box-sizing:border-box}.player-group-rule summary>span:first-child{flex:1;min-width:0;overflow-wrap:anywhere}.player-group-price{white-space:nowrap;color:var(--danger-text);font-variant-numeric:tabular-nums}.player-group-rule summary :deep(.site-icon){color:var(--muted)}.player-group-rule[open] summary{font-weight:600}.player-group-rule[open] summary :deep(.site-icon){transform:rotate(180deg)}.player-group-rule .player-rule-prize:last-child{padding-bottom:8px}@media(max-width:900px),(pointer:coarse){.player-rules-dialog section+section{margin-top:8px}.player-rules-dialog h3{margin-bottom:8px}.player-rules-dialog p{margin:4px 0}}
</style>
