import { describe, expect, it } from 'vitest'
import { formatPoints, points } from './points'

describe('integer points', () => {
  it('keeps large balances exact when formatting', () => {
    const amount = points('9007199254740993')
    expect(amount).toBe('9007199254740993')
    expect(formatPoints(amount)).toBe('9,007,199,254,740,993')
  })

  it('rejects decimals and formats whole points', () => {
    expect(() => points('1.5')).toThrow(TypeError)
    expect(() => points(Number.MAX_SAFE_INTEGER + 1)).toThrow(TypeError)
    expect(() => points(1.25)).toThrow(TypeError)
    expect(formatPoints(points('12500'))).toBe('12,500')
  })
})
