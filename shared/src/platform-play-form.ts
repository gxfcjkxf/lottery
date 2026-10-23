import type { RuleCondition, RuleDefinition, RuleModel } from './rules'

export interface PlayConditionDraft { field: string; target: string; operator: 'equals' | 'between'; value: number; min: number; max: number }
export interface PlayTierDraft { odds: string; cap: string; exclusive: boolean; logic: 'all' | 'any'; conditions: PlayConditionDraft[] }
export interface PlayRuleDraft {
  regularCount: number; specialCount: number; unitPoints: string; cap: string; roundingScope: string; mixedPolicy: string;
  maxCombinations: number; maxMultiplier: string; maxBetPoints: string; tiers: PlayTierDraft[]
}
export function playCondition(field: string, value = 0): PlayConditionDraft { return { field, target: 'all', operator: 'equals', value, min: 0, max: value } }
export function playTier(model: RuleModel): PlayTierDraft {
  return { odds: '1', cap: '', exclusive: true, logic: 'all', conditions: model.model === 'DIGITS_0_9' ? [playCondition('position_match', model.length)] : [
    ...(model.regular_count ? [playCondition('regular_match', model.regular_count)] : []),
    ...(model.special_count ? [playCondition('special_match', model.special_count)] : []),
  ] }
}
export function playRuleDraft(model: RuleModel): PlayRuleDraft {
  return { regularCount: model.regular_count, specialCount: model.special_count, unitPoints: '1', cap: '', roundingScope: 'order', mixedPolicy: '', maxCombinations: 10000, maxMultiplier: '10', maxBetPoints: '', tiers: [playTier(model)] }
}
export function buildPlayDefinition(model: RuleModel, draft: PlayRuleDraft): RuleDefinition {
  const amount = (value: string) => /^[1-9]\d*$/.test(value) && BigInt(value) <= 9223372036854775807n
  const optionalAmount = (value: string) => value === '' || amount(value)
  const integer = (value: number, max: number) => Number.isInteger(value) && value >= 0 && value <= max
  if (!amount(draft.unitPoints) || !amount(draft.maxMultiplier) || !optionalAmount(draft.cap) || !optionalAmount(draft.maxBetPoints) ||
    !integer(draft.regularCount, model.regular_count) || !integer(draft.specialCount, model.special_count) ||
    model.model !== 'DIGITS_0_9' && draft.regularCount + draft.specialCount === 0 ||
    !integer(draft.maxCombinations, 10000) || draft.maxCombinations < 1 || draft.tiers.length < 1 || draft.tiers.length > 32 ||
    !['order', 'line', 'tier'].includes(draft.roundingScope) ||
    draft.tiers.some(t => t.exclusive) && draft.tiers.some(t => !t.exclusive) && !['max_all', 'max_exclusive_plus_additive'].includes(draft.mixedPolicy)) throw new Error('Invalid play rule form')
  const matchFields = model.model === 'DIGITS_0_9' ? ['position_match'] : [...(model.regular_count ? ['regular_match'] : []), ...(model.special_count ? ['special_match'] : [])]
  const drawFields = ['draw_sum', 'draw_odd_count', 'draw_even_count', 'draw_unique_count', 'draw_all_same', 'draw_span', 'draw_consecutive']
  return {
    schema_version: 1, model: JSON.parse(JSON.stringify(model)),
    selection: { mode: 'numbers', regular_count: draft.regularCount, special_count: draft.specialCount, exclude_count: 0, attribute_groups: null, feature_choices: null },
    number_attributes: null, unit_points: draft.unitPoints,
    prize_tiers: draft.tiers.map((tier, index) => {
      if (!/^(0|[1-9]\d{0,9})(\.\d{1,6})?$/.test(tier.odds) || Number(tier.odds) <= 0 || !optionalAmount(tier.cap) || tier.conditions.length < 1 || tier.conditions.length > 32 || !['all', 'any'].includes(tier.logic)) throw new Error('Invalid prize tier')
      const conditions: RuleCondition[] = tier.conditions.map(c => {
        if (!matchFields.includes(c.field) && !drawFields.includes(c.field) || !['equals', 'between'].includes(c.operator)) throw new Error('Invalid condition')
        if (c.operator === 'equals' ? !integer(c.value, 20000000) : !integer(c.min, 20000000) || !integer(c.max, 20000000) || c.min > c.max) throw new Error('Invalid comparison')
        if (!matchFields.includes(c.field) && !(model.model === 'DIGITS_0_9' ? ['all', 'digits'] : ['all', ...(model.regular_count ? ['regular'] : []), ...(model.special_count ? ['special'] : [])]).includes(c.target)) throw new Error('Invalid draw scope')
        return { op: c.operator, field: c.field, ...(!matchFields.includes(c.field) ? { target: c.target } : {}), ...(c.operator === 'equals' ? { value: c.value } : { min: c.min, max: c.max }) }
      })
      return { code: `TIER_${index + 1}`, odds: tier.odds, exclusive: tier.exclusive, cap_points: tier.cap || null, condition: conditions.length === 1 ? conditions[0] : { op: tier.logic, children: conditions } }
    }),
    mixed_tier_policy: draft.mixedPolicy, cap_points: draft.cap || null, rounding: 'half_up', rounding_scope: draft.roundingScope,
    limits: { max_combinations: draft.maxCombinations, max_multiplier: draft.maxMultiplier, max_bet_points: draft.maxBetPoints || null },
  }
}
