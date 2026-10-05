import { describe, expect, it } from 'vitest'
import { addPoints, formatPoints, multiplyPoints, points } from './points'

describe('integer points', () => {
  it('keeps large balances exact across arithmetic', () => {
    const amount = points('9007199254740993')
    expect(addPoints(amount, '7')).toBe('9007199254741000')
    expect(multiplyPoints(amount, 3)).toBe('27021597764222979')
  })

  it('rejects decimals and formats whole points', () => {
    expect(() => points('1.5')).toThrow(TypeError)
    expect(() => points(Number.MAX_SAFE_INTEGER + 1)).toThrow(TypeError)
    expect(() => points(1.25)).toThrow(TypeError)
    expect(formatPoints(points('12500'))).toBe('12,500')
  })
})
