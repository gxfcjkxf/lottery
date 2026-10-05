import { describe, expect, it } from 'vitest'
import { isValidWholeAmount } from './withdrawal'

describe('withdrawal amount preview', () => {
  it('accepts only positive whole point amounts within the demo limit', () => {
    expect(isValidWholeAmount('1', 8200n)).toBe(true)
    expect(isValidWholeAmount('8200', 8200n)).toBe(true)
    expect(isValidWholeAmount('0', 8200n)).toBe(false)
    expect(isValidWholeAmount('1.5', 8200n)).toBe(false)
    expect(isValidWholeAmount('NaN', 8200n)).toBe(false)
    expect(isValidWholeAmount('8201', 8200n)).toBe(false)
  })
})
