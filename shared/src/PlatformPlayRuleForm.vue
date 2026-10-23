<script setup lang="ts">
import { computed, watch } from 'vue'
import type { RuleModel } from './rules'
import { createCatalogI18n } from './localization'
import { platformLotteryCatalog } from './platform-lottery'
import { playCondition, playTier, type PlayRuleDraft } from './platform-play-form'
const props = defineProps<{ model: RuleModel; draft: PlayRuleDraft; locale: 'en' | 'zh-CN' }>()
const i18n = createCatalogI18n(platformLotteryCatalog, props.locale)
watch(() => props.locale, value => i18n.setLocale(value))
const t = i18n.t
const fields = computed(() => [...(props.model.model === 'DIGITS_0_9' ? ['position_match'] : [...(props.model.regular_count ? ['regular_match'] : []), ...(props.model.special_count ? ['special_match'] : [])]), 'draw_sum', 'draw_odd_count', 'draw_even_count', 'draw_unique_count', 'draw_all_same', 'draw_span', 'draw_consecutive'])
const isMatch = (field: string) => ['position_match', 'regular_match', 'special_match'].includes(field)
const mixed = computed(() => props.draft.tiers.some(t => t.exclusive) && props.draft.tiers.some(t => !t.exclusive))
</script>

<template>
  <section class="play-rule-builder">
    <section class="play-rule-section"><h3>{{ t('playSelectionTitle') }}</h3><div class="play-rule-grid">
      <p v-if="model.model === 'DIGITS_0_9'">{{ t('playDigitsSelection', { count: model.length }) }}</p>
      <label v-if="model.regular_count">{{ t('playRegularCount') }}<input v-model.number="draft.regularCount" type="number" min="0" :max="model.regular_count" required /></label>
      <label v-if="model.special_count">{{ t('playSpecialCount') }}<input v-model.number="draft.specialCount" type="number" min="0" :max="model.special_count" required /></label>
      <label>{{ t('playUnit') }}<input v-model.trim="draft.unitPoints" inputmode="numeric" pattern="[1-9][0-9]*" required /></label>
    </div></section>
    <section class="play-rule-section"><h3>{{ t('playTiersTitle') }}</h3><p>{{ t('playTiersHelp') }}</p>
      <article v-for="(tier, index) in draft.tiers" :key="index" class="play-rule-tier"><div class="play-rule-heading"><h4>{{ t('playTierNumber', { count: index + 1 }) }}</h4><button type="button" class="button button-secondary" :disabled="draft.tiers.length === 1" @click="draft.tiers.splice(index, 1)">{{ t('playRemoveTier') }}</button></div>
        <div class="play-rule-grid"><label>{{ t('playOdds') }}<input v-model.trim="tier.odds" inputmode="decimal" pattern="(0|[1-9][0-9]{0,9})(\.[0-9]{1,6})?" required /></label><label>{{ t('playTierCap') }}<input v-model.trim="tier.cap" inputmode="numeric" pattern="[1-9][0-9]*" :placeholder="t('playNoCap')" /></label><label>{{ t('playTierPayout') }}<select v-model="tier.exclusive"><option :value="true">{{ t('playExclusive') }}</option><option :value="false">{{ t('playAdditive') }}</option></select></label><label>{{ t('playConditionLogic') }}<select v-model="tier.logic"><option value="all">{{ t('playAllConditions') }}</option><option value="any">{{ t('playAnyCondition') }}</option></select></label></div>
        <div v-for="(condition, cIndex) in tier.conditions" :key="cIndex" class="play-rule-condition"><div class="play-rule-grid">
          <label>{{ t('playConditionField') }}<select v-model="condition.field"><option v-for="field in fields" :key="field" :value="field">{{ t(`playField.${field}`) }}</option></select></label>
          <label v-if="!isMatch(condition.field)">{{ t('playDrawScope') }}<select v-model="condition.target"><option value="all">{{ t('playScopeAll') }}</option><option v-if="model.model === 'DIGITS_0_9'" value="digits">{{ t('playScopeDigits') }}</option><option v-if="model.regular_count" value="regular">{{ t('regular') }}</option><option v-if="model.special_count" value="special">{{ t('special') }}</option></select></label>
          <label>{{ t('playComparison') }}<select v-model="condition.operator"><option value="equals">{{ t('playEquals') }}</option><option value="between">{{ t('playBetween') }}</option></select></label>
          <label v-if="condition.operator === 'equals'">{{ t('playValue') }}<input v-model.number="condition.value" type="number" min="0" max="20000000" required /></label>
          <template v-else><label>{{ t('playMin') }}<input v-model.number="condition.min" type="number" min="0" max="20000000" required /></label><label>{{ t('playMax') }}<input v-model.number="condition.max" type="number" :min="condition.min" max="20000000" required /></label></template>
        </div><button type="button" class="button button-secondary" :disabled="tier.conditions.length === 1" @click="tier.conditions.splice(cIndex, 1)">{{ t('playRemoveCondition') }}</button></div>
        <button type="button" class="button button-secondary" :disabled="tier.conditions.length >= 32" @click="tier.conditions.push(playCondition(fields[0]))">{{ t('playAddCondition') }}</button>
      </article><button type="button" class="button button-secondary" :disabled="draft.tiers.length >= 32" @click="draft.tiers.push(playTier(model))">{{ t('playAddTier') }}</button>
      <label v-if="mixed">{{ t('playMixedPolicy') }}<select v-model="draft.mixedPolicy" required><option value="" disabled>{{ t('playChoosePolicy') }}</option><option value="max_all">{{ t('playMixedMax') }}</option><option value="max_exclusive_plus_additive">{{ t('playMixedAdd') }}</option></select></label>
    </section>
    <details class="play-rule-section"><summary>{{ t('playLimitsTitle') }}</summary><p>{{ t('playLimitsHelp') }}</p><div class="play-rule-grid">
      <label>{{ t('playOrderCap') }}<input v-model.trim="draft.cap" inputmode="numeric" pattern="[1-9][0-9]*" :placeholder="t('playNoCap')" /></label>
      <label>{{ t('playMaxCombinations') }}<input v-model.number="draft.maxCombinations" type="number" min="1" max="10000" required /></label>
      <label>{{ t('playMaxMultiplier') }}<input v-model.trim="draft.maxMultiplier" inputmode="numeric" pattern="[1-9][0-9]*" required /></label>
      <label>{{ t('playMaxBet') }}<input v-model.trim="draft.maxBetPoints" inputmode="numeric" pattern="[1-9][0-9]*" :placeholder="t('playNoCap')" /></label>
      <label>{{ t('playRounding') }}<select v-model="draft.roundingScope"><option value="order">{{ t('playRoundOrder') }}</option><option value="line">{{ t('playRoundLine') }}</option><option value="tier">{{ t('playRoundTier') }}</option></select></label>
    </div></details>
    <p class="play-rule-save-note">{{ t('playHelpSave') }}</p>
    <details class="play-rule-help"><summary>{{ t('playHelpTitle') }}</summary><p>{{ t('playHelpConditions') }}</p><p>{{ t('playHelpOdds') }}</p></details>
  </section>
</template>

<style scoped>
.play-rule-builder { display: grid; gap: 16px; min-width: 0; grid-column: 1 / -1; }
.play-rule-section, .play-rule-help { min-width: 0; padding: 16px; border: 1px solid var(--line); border-radius: var(--radius-sm); }
.play-rule-section h3 { margin: 0 0 8px; font-size: 16px; }
.play-rule-section p, .play-rule-help p { margin: 6px 0 12px; font-size: 13px; line-height: 1.6; color: var(--muted); }
.play-rule-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px 16px; }
.play-rule-grid label { display: grid; align-content: start; gap: 6px; min-width: 0; font-size: 13px; }
.play-rule-builder input, .play-rule-builder select { width: 100%; min-width: 0; min-height: 40px; box-sizing: border-box; padding: 8px; font: inherit; border: 1px solid var(--line); border-radius: var(--radius-sm); color: var(--ink); background: var(--surface); }
.play-rule-builder select { padding-right: 32px; }
.play-rule-tier { margin: 12px 0; padding: 14px; border: 1px solid var(--line); border-radius: var(--radius-sm); }
.play-rule-heading { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 8px; margin-bottom: 12px; }
.play-rule-heading h4 { margin: 0; font-size: 14px; }
.play-rule-condition { display: grid; gap: 10px; margin: 14px 0; padding-top: 14px; border-top: 1px solid var(--line); }
.play-rule-condition > button { justify-self: start; }
.play-rule-help { background: var(--canvas); }
.play-rule-help summary { min-height: 44px; line-height: 44px; cursor: pointer; font-size: 14px; font-weight: 600; }
.play-rule-save-note { margin: 0; font-size: 13px; line-height: 1.6; color: var(--muted); }
.play-rule-section summary { cursor: pointer; font-size: 14px; font-weight: 600; }
@media(max-width:640px) { .play-rule-grid { grid-template-columns: minmax(0, 1fr); } .play-rule-section, .play-rule-help, .play-rule-tier { padding: 12px; } .play-rule-builder input, .play-rule-builder select, .play-rule-builder button { min-height: 44px; } }
</style>
