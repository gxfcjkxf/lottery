import { describe, expect, it } from 'vitest'
import { formatPoints, points } from './points'

describe('decimal points', () => {
  it('keeps large balances exact when formatting', () => {
    const amount = points('9007199254740993')
    expect(amount).toBe('9007199254740993')
    expect(formatPoints(amount)).toBe('9,007,199,254,740,993')
  })

  it('accepts hundredths and rejects excess precision', () => {
    expect(formatPoints(points('1.5'))).toBe('1.50')
    expect(formatPoints(points('0.01'))).toBe('0.01')
    expect(formatPoints(points('9007199254740993.12'))).toBe('9,007,199,254,740,993.12')
    expect(() => points('1.234')).toThrow(TypeError)
    expect(() => points(Number.MAX_SAFE_INTEGER + 1)).toThrow(TypeError)
    expect(() => points(1.25)).toThrow(TypeError)
    expect(formatPoints(points('12500'))).toBe('12,500')
  })
})
