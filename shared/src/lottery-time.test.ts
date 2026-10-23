import { describe, expect, it } from 'vitest'
import { lotteryLocalTimeToISO } from './lottery-time'
describe('lottery local time', () => {
  it('uses the game zone, not the computer zone, including half-hour offsets', () => {
    expect(lotteryLocalTimeToISO('2026-10-13T12:30', 'Asia/Singapore')).toBe('2026-10-13T04:30:00.000Z')
    expect(lotteryLocalTimeToISO('2026-10-13T12:30', 'Asia/Kolkata')).toBe('2026-10-13T07:00:00.000Z')
    expect(lotteryLocalTimeToISO('2026-07-13T12:30', 'America/New_York')).toBe('2026-07-13T16:30:00.000Z')
    expect(lotteryLocalTimeToISO('2026-01-13T12:30', 'America/New_York')).toBe('2026-01-13T17:30:00.000Z')
  })
  it('rejects invalid input, impossible dates and nonexistent local times', () => {
    expect(() => lotteryLocalTimeToISO('2026-02-30T12:30', 'Asia/Singapore')).toThrow()
    expect(() => lotteryLocalTimeToISO('2026-03-08T02:30', 'America/New_York')).toThrow()
    expect(() => lotteryLocalTimeToISO('bad', 'Asia/Singapore')).toThrow()
  })
})
