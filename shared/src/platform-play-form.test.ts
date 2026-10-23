import { describe, expect, it } from 'vitest'
import { buildPlayDefinition, playCondition, playRuleDraft, playTier } from './platform-play-form'
import type { RuleModel } from './rules'
const digits: RuleModel = { model: 'DIGITS_0_9', regular_pool: { allow_repeat: false }, special_pool: { allow_repeat: false }, regular_count: 0, special_count: 0, pool_size: 0, total_count: 0, length: 3, ordered: true, allow_repeat: true }
const xy: RuleModel = { ...digits, model: 'X_PLUS_Y', length: 0, regular_pool: { min: 1, max: 49, allow_repeat: false }, special_pool: { min: 1, max: 10, allow_repeat: false }, regular_count: 6, special_count: 1, allow_repeat: false, ordered: false }
describe('platform play form conversion', () => {
  it('preserves each game model and builds all-match rules for all three models', () => {
    for (const model of [digits, xy, { ...xy, model: 'M_SELECT_N', special_pool: { ...xy.regular_pool }, pool_size: 49, total_count: 7 }]) {
      const definition = buildPlayDefinition(model, playRuleDraft(model))
      expect(definition.model).toEqual(model)
      expect(definition.model).not.toBe(model)
      expect(definition.selection.mode).toBe('numbers')
      expect(definition.prize_tiers![0].condition).toEqual(model.model === 'DIGITS_0_9' ? { op: 'equals', field: 'position_match', value: 3 } : { op: 'all', children: [{ op: 'equals', field: 'regular_match', value: 6 }, { op: 'equals', field: 'special_match', value: 1 }] })
    }
  })
  it('keeps decimal odds, caps, ranges and mixed-tier policy exact', () => {
    const draft = playRuleDraft(digits)
    draft.tiers[0].odds = '1.234567'; draft.tiers[0].cap = '9007199254740993'
    draft.tiers[0].conditions = [{ ...playCondition('draw_sum'), operator: 'between', min: 10, max: 20, target: 'digits' }]
    draft.tiers.push({ ...playTier(digits), exclusive: false }); draft.mixedPolicy = 'max_exclusive_plus_additive'
    const definition = buildPlayDefinition(digits, draft)
    expect(definition.prize_tiers![0]).toMatchObject({ odds: '1.234567', cap_points: '9007199254740993', condition: { op: 'between', field: 'draw_sum', target: 'digits', min: 10, max: 20 } })
    expect(definition.mixed_tier_policy).toBe('max_exclusive_plus_additive')
    expect(definition.cap_points).toBeNull()
  })
  it('rejects invalid input rather than substituting defaults', () => {
    for (const change of [
      (d: ReturnType<typeof playRuleDraft>) => { d.unitPoints = '0' },
      (d: ReturnType<typeof playRuleDraft>) => { d.maxMultiplier = '9223372036854775808' },
      (d: ReturnType<typeof playRuleDraft>) => { d.tiers[0].odds = '1e3' },
      (d: ReturnType<typeof playRuleDraft>) => { d.tiers.push({ ...playTier(digits), exclusive: false }) },
      (d: ReturnType<typeof playRuleDraft>) => { d.tiers[0].conditions[0].value = Number.NaN },
    ]) { const draft = playRuleDraft(digits); change(draft); expect(() => buildPlayDefinition(digits, draft)).toThrow() }
  })
})
