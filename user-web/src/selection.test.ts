import { describe, expect, it } from 'vitest'
import { combinations, selectionCount, totalPoints, validateSelection, type SelectionDraft, type SelectionModel } from './selection'

const model: SelectionModel = { id: 'classic', kind: 'regular-special', regularPick: 6, regularPool: 49, specialPick: 1, specialPool: 10, unitPoints: 2n, minMultiplier: 1, maxMultiplier: 20 }
const emptyDraft: SelectionDraft = { regular: [], special: [], digits: [] }

describe('lottery selection maths', () => {
  it('expands a 7-number system pick into seven lines', () => {
    const draft = { regular: [1, 2, 3, 4, 5, 6, 7], special: [9], digits: [] }
    expect(combinations(7, 6)).toBe(7n)
    expect(selectionCount(model, draft)).toBe(7n)
    expect(totalPoints(model, draft, 3)).toBe(42n)
  })

  it('multiplies the regular system lines by distinct special-number combinations', () => {
    const draft = { regular: [1, 2, 3, 4, 5, 6, 7], special: [2, 8], digits: [] }
    expect(selectionCount(model, draft)).toBe(14n)
    expect(totalPoints(model, draft, 2)).toBe(56n)
  })

  it('accepts zero and repeated digits in positional 3-digit picks', () => {
    const digitsModel: SelectionModel = { id: 'daily-3', kind: 'digits', regularPick: 3, regularPool: 10, digitCount: 3, unitPoints: 1n, minMultiplier: 1, maxMultiplier: 20 }
    const repeated = { ...emptyDraft, digits: [1, 2, 1] }
    const triple = { ...emptyDraft, digits: [1, 1, 1] }
    const withZero = { ...emptyDraft, digits: [0, 2, 1] }
    expect(validateSelection(digitsModel, repeated, 1)).toBeNull()
    expect(validateSelection(digitsModel, triple, 1)).toBeNull()
    expect(validateSelection(digitsModel, withZero, 1)).toBeNull()
    expect(selectionCount(digitsModel, triple)).toBe(1n)
    expect(totalPoints(digitsModel, withZero, 4)).toBe(4n)
  })

  it('validates each regular and special pool and multiplier', () => {
    expect(validateSelection(model, { regular: [1, 2, 3, 4, 5], special: [2], digits: [] }, 1)).toMatch(/at least 6 regular/)
    expect(validateSelection(model, { regular: [1, 2, 3, 4, 5, 50], special: [2], digits: [] }, 1)).toMatch(/1 to 49/)
    expect(validateSelection(model, { regular: [1, 2, 3, 4, 5, 6], special: [], digits: [] }, 1)).toMatch(/special number/)
    expect(validateSelection(model, { regular: [1, 2, 3, 4, 5, 6], special: [11], digits: [] }, 1)).toMatch(/special numbers from 1 to 10/)
    expect(validateSelection(model, { regular: [1, 2, 3, 4, 5, 6], special: [2], digits: [] }, 21)).toMatch(/Multiplier/)
  })
})
