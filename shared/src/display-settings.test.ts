import { afterEach, expect, it, vi } from 'vitest'
import { formatDisplayDateTime, loadPlatformDisplaySettings, platformDisplaySettings, setPlatformDisplaySettings } from './display-settings'
afterEach(() => { platformDisplaySettings.value = null })
it('uses platform date order, clock and timezone regardless of device locale', () => {
  setPlatformDisplaySettings({ timezone: 'Asia/Singapore', date_format: 'DD/MM/YYYY', time_format: 'hh:mm:ss A' })
  expect(formatDisplayDateTime('2026-10-17T08:21:53Z', 'en')).toBe('17/10/2026 04:21:53 PM')
  expect(formatDisplayDateTime('2026-10-17T08:21:53Z', 'zh')).toBe('17/10/2026 04:21:53 下午')
  setPlatformDisplaySettings({ timezone: 'UTC', date_format: 'MM/DD/YYYY', time_format: 'HH:mm:ss' })
  expect(formatDisplayDateTime('2026-10-17T08:21:53Z', 'zh-CN')).toBe('10/17/2026 08:21:53')
})
it('loads only the public settings contract without caching', async () => {
  const data = { timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss' }
  const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: true, data })))
  await loadPlatformDisplaySettings(fetcher)
  expect(fetcher).toHaveBeenCalledWith('/api/v1/display-settings', expect.objectContaining({ cache: 'no-store' }))
  expect(platformDisplaySettings.value).toEqual(data)
})
it('rejects unavailable or invalid policies rather than inventing a default format', async () => {
  expect(formatDisplayDateTime('2026-10-17T08:21:53Z', 'zh-CN')).toBe('无法加载日期和时间设置。')
  await expect(loadPlatformDisplaySettings(vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: false }), { status: 503 })))).rejects.toThrow()
  expect(() => setPlatformDisplaySettings({ timezone: 'not-a-zone', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss' })).toThrow()
  expect(platformDisplaySettings.value).toBeNull()
})
