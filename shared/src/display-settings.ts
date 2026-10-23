import { readApiEnvelope } from './api-response'

import { shallowRef } from 'vue'
import { formatPlatformDateTime, type PlatformDisplaySettings } from './date-time'

export const platformDisplaySettings = shallowRef<PlatformDisplaySettings | null>(null)
export const displaySettingsCatalog = {
  en: { loading: 'Loading…', unavailable: 'Unable to load date and time settings.', retry: 'Retry', date: 'Date', time: 'Time', 'datetime-local': 'Date and time' },
  'zh-CN': { loading: '加载中…', unavailable: '无法加载日期和时间设置。', retry: '重试', date: '日期', time: '时间', 'datetime-local': '日期和时间' },
}

export function setPlatformDisplaySettings(value: PlatformDisplaySettings): void {
  if (!value || !['YYYY-MM-DD', 'DD/MM/YYYY', 'MM/DD/YYYY'].includes(value.date_format) || !['HH:mm:ss', 'hh:mm:ss A'].includes(value.time_format) || typeof value.timezone !== 'string' || !value.timezone) throw new TypeError('Invalid platform display settings')
  new Intl.DateTimeFormat('en', { timeZone: value.timezone }).format(0)
  platformDisplaySettings.value = { timezone: value.timezone, date_format: value.date_format, time_format: value.time_format }
}

export async function loadPlatformDisplaySettings(fetcher: typeof fetch = fetch): Promise<void> {
  const response = await fetcher('/api/v1/display-settings', { headers: { Accept: 'application/json' }, credentials: 'same-origin', cache: 'no-store' })
  const result = await readApiEnvelope<PlatformDisplaySettings>(response,(message,status,code)=>Object.assign(new Error(message),{status,code}))
  if (!response.ok || result.success !== true || !result.data || Object.keys(result.data).sort().join(',') !== 'date_format,time_format,timezone') throw new Error('Invalid platform display settings response')
  setPlatformDisplaySettings(result.data)
}

/** All human-readable timestamps use the platform policy, never the device locale/zone. */
export function formatDisplayDateTime(value: string | null | undefined, locale: string): string {
  const language = locale === 'zh' || locale === 'zh-CN' ? 'zh-CN' : 'en'
  if (!platformDisplaySettings.value) return displaySettingsCatalog[language].unavailable
  return formatPlatformDateTime(value, platformDisplaySettings.value, language)
}
