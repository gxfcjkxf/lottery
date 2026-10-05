import { describe, expect, it } from 'vitest'
import { previewCorrection, resolveWithdrawal, reviewRule, simulateRule, submitRuleForReview, type RuleDraft } from './workflows'

const rule: RuleDraft = { creator: '林岚', reviewer: '', status: 'draft', testSelection: '07 18 29', testResult: '07 12 29', unitPoints: '2', multiplier: '3' }

describe('demo admin workflows', () => {
  it('simulates rule matching and returns integer point strings', () => {
    expect(simulateRule(rule)).toMatchObject({ valid: true, matches: 2, points: '12', combinations: '3' })
  })
  it('uses exact integer arithmetic for values beyond Number safe precision', () => {
    expect(simulateRule({ ...rule, unitPoints: '9007199254740993', multiplier: '2' }).points).toBe('36028797018963972')
  })
  it('rejects malformed decimals and multipliers outside configured bounds', () => {
    expect(simulateRule({ ...rule, unitPoints: '1.234' })).toMatchObject({ valid: false, points: '0' })
    expect(simulateRule({ ...rule, unitPoints: '1.00' })).toMatchObject({ valid: false, points: '0' })
    expect(simulateRule({ ...rule, unitPoints: '1e3' })).toMatchObject({ valid: false, points: '0' })
    expect(simulateRule({ ...rule, unitPoints: '-1' })).toMatchObject({ valid: false, points: '0' })
    expect(simulateRule({ ...rule, multiplier: '101' })).toMatchObject({ valid: false, points: '0' })
    expect(simulateRule({ ...rule, multiplier: '1.5' })).toMatchObject({ valid: false, points: '0' })
  })
  it('does not allow a rule creator to self-review', () => {
    expect(() => submitRuleForReview({ ...rule, reviewer: '林岚' })).toThrow(/不能审核自己的规则/)
    const pending = submitRuleForReview(rule)
    expect(pending.status).toBe('pending_review')
    expect(reviewRule(pending, '周宁', 'approve').status).toBe('approved')
  })
  it('previews correction impact before a reasoned correction', () => {
    expect(previewCorrection('07 18', '07 19', 126)).toMatchObject({ affectedOrders: '126', estimatedSettlements: '126', reasonRequired: true })
  })
  it('requires a reason to resolve a withdrawal review', () => {
    expect(() => resolveWithdrawal('WD-2401', 'rejected', '  ')).toThrow(/原因必填/)
    expect(resolveWithdrawal('WD-2401', 'rejected', '资料不完整').status).toBe('rejected')
  })
})
