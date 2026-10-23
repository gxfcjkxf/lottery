import { describe, expect, it } from 'vitest'
import { formatPlatformDateTime, parsePlatformDateTimeInput, platformDateTimeInputPlaceholder, type PlatformDisplaySettings } from './date-time'
import { lotteryLocalTimeToISO } from './lottery-time'
describe('configured date-time display and input', () => {
  it('uses the same date, time and timezone settings in both directions', () => {
    const settings: PlatformDisplaySettings = { timezone: 'Asia/Singapore', date_format: 'DD/MM/YYYY', time_format: 'hh:mm:ss A' }
    expect(platformDateTimeInputPlaceholder(settings)).toBe('DD/MM/YYYY hh:mm:ss AM')
    const canonical = parsePlatformDateTimeInput('13/10/2026 01:25:30 PM', settings)
    expect(canonical).toBe('2026-10-13T13:25:30')
    const iso = lotteryLocalTimeToISO(canonical, settings.timezone)
    expect(iso).toBe('2026-10-13T05:25:30.000Z')
    expect(formatPlatformDateTime(iso, settings, 'en')).toBe('13/10/2026 01:25:30 PM')
    expect(parsePlatformDateTimeInput(formatPlatformDateTime(iso, settings, 'zh-CN'), settings)).toBe(canonical)
    expect(() => parsePlatformDateTimeInput('2026-10-13T13:25', settings)).toThrow()
  })
  it('supports the platform month-first and 24-hour formats', () => {
    const settings: PlatformDisplaySettings = { timezone: 'UTC', date_format: 'MM/DD/YYYY', time_format: 'HH:mm:ss' }
    expect(parsePlatformDateTimeInput('10/13/2026 13:25:30', settings)).toBe('2026-10-13T13:25:30')
    expect(formatPlatformDateTime('2026-10-13T13:25:30Z', settings, 'en')).toBe('10/13/2026 13:25:30')
  })
})
