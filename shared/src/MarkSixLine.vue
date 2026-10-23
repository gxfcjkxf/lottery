<script setup lang="ts">
import {watch} from 'vue'
import {createCatalogI18n} from './localization'
import {markSixCatalog,markSixLabel,markSixParlayCodes,markSixBallNumber,markSixSelectionKinds,markSixSelectionLabel,type MarkSixItem,type MarkSixAttributes} from './mark-six'
import MarkSixBall from './MarkSixBall.vue'
const props=defineProps<{item:MarkSixItem;attributes:MarkSixAttributes;locale:'en'|'zh-CN'}>()
const i18n=createCatalogI18n(markSixCatalog,props.locale);watch(()=>props.locale,v=>i18n.setLocale(v));const t=i18n.t
</script>
<template><div class="mark-six-line"><strong>{{ markSixLabel(item.code,t) }}</strong><MarkSixBall v-if="markSixBallNumber(item.code)!==undefined" :value="markSixBallNumber(item.code)!" :attributes="attributes" :locale="locale" small /><div v-if="item.selection?.length" class="line-selection"><template v-if="markSixSelectionKinds[item.code]==='number'"><MarkSixBall v-for="n in item.selection" :key="n" :value="n" :attributes="attributes" :locale="locale" small /></template><span v-else>{{ markSixSelectionLabel(item.code,item.selection,t) }}</span></div><span>{{ t('stake') }} {{ item.points }}</span><span>{{ t('odds') }} {{ item.odds }}</span><span v-for="(legOdds,index) in item.leg_odds" :key="index">{{ markSixLabel(markSixParlayCodes(item.selection!)[index]!,t) }} · {{ legOdds }}</span><span v-if="item.odds_full">{{ t(item.code==='linked_2_special'?'specialHitOdds':'fullOdds') }} {{ item.odds_full }}</span><span v-if="item.winning_odds">{{ t('winningOdds') }} {{ item.winning_odds }}</span><span v-if="item.hit_count">{{ t('hitCount',{count:item.hit_count}) }}</span><span v-if="item.outcome">{{ t(item.outcome) }} · {{ item.payout }}</span></div></template>
<style scoped>.mark-six-line{display:flex;flex-wrap:wrap;align-items:center;gap:8px;padding:10px 0;border-bottom:1px solid var(--line);font-size:13px;min-width:0;overflow-wrap:anywhere}.line-selection{display:flex;flex-wrap:wrap;gap:4px;align-items:center;max-width:100%}</style>
