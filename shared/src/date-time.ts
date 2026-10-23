export interface PlatformDisplaySettings {
  timezone: string
  date_format: 'YYYY-MM-DD' | 'DD/MM/YYYY' | 'MM/DD/YYYY'
  time_format: 'HH:mm:ss' | 'hh:mm:ss A'
}

export function platformDateTimeInputPlaceholder(settings: PlatformDisplaySettings): string {
  const date = settings.date_format
  const time = settings.time_format === 'hh:mm:ss A' ? 'hh:mm:ss AM' : 'HH:mm:ss'
  return `${date} ${time}`
}

export function parsePlatformDateTimeInput(value: string, settings: PlatformDisplaySettings): string {
  const raw = value.trim()
  const twelveHour = settings.time_format === 'hh:mm:ss A'
  const timePattern = twelveHour ? '(\\d{1,2}):(\\d{2})(?::(\\d{2}))?\\s*(AM|PM|上午|下午)' : '(\\d{2}):(\\d{2})(?::(\\d{2}))?'
  const datePattern = settings.date_format === 'YYYY-MM-DD' ? '(\\d{4})-(\\d{2})-(\\d{2})' : '(\\d{2})/(\\d{2})/(\\d{4})'
  const match = new RegExp(`^${datePattern}[ T]+${timePattern}$`, 'i').exec(raw)
  if (!match) throw new Error(`Invalid date and time. Use ${platformDateTimeInputPlaceholder(settings)}.`)
  const dateParts = settings.date_format === 'YYYY-MM-DD' ? { year: match[1], month: match[2], day: match[3] } : settings.date_format === 'DD/MM/YYYY' ? { year: match[3], month: match[2], day: match[1] } : { year: match[3], month: match[1], day: match[2] }
  const timeIndex = 4
  let hour = Number(match[timeIndex])
  const minute = match[timeIndex + 1]
  const second = match[timeIndex + 2] ?? '00'
  if (twelveHour) {
    const meridiem = String(match[timeIndex + 3]).toUpperCase()
    if (hour < 1 || hour > 12) throw new Error(`Invalid date and time. Use ${platformDateTimeInputPlaceholder(settings)}.`)
    hour = hour % 12 + (meridiem === 'PM' || meridiem === '下午' ? 12 : 0)
  }
  if (hour > 23 || Number(minute) > 59 || Number(second) > 59) throw new Error(`Invalid date and time. Use ${platformDateTimeInputPlaceholder(settings)}.`)
  return `${dateParts.year}-${dateParts.month}-${dateParts.day}T${String(hour).padStart(2, '0')}:${minute}:${second}`
}

function parts(value: string, timezone: string) {
  const values = new Intl.DateTimeFormat('en-US', { timeZone: timezone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23' }).formatToParts(new Date(value))
  return Object.fromEntries(values.map(part => [part.type, part.value]))
}

export function formatPlatformDateTime(value: string | null | undefined, settings: PlatformDisplaySettings, locale: 'en' | 'zh-CN'): string {
  if (!value) return '—'
  const valueParts = parts(value, settings.timezone)
  const date = settings.date_format === 'DD/MM/YYYY' ? `${valueParts.day}/${valueParts.month}/${valueParts.year}` : settings.date_format === 'MM/DD/YYYY' ? `${valueParts.month}/${valueParts.day}/${valueParts.year}` : `${valueParts.year}-${valueParts.month}-${valueParts.day}`
  const hour24 = Number(valueParts.hour)
  const hour12 = hour24 % 12 || 12
  const time = settings.time_format === 'hh:mm:ss A' ? `${String(hour12).padStart(2, '0')}:${valueParts.minute}:${valueParts.second} ${hour24 >= 12 ? (locale === 'zh-CN' ? '下午' : 'PM') : (locale === 'zh-CN' ? '上午' : 'AM')}` : `${valueParts.hour}:${valueParts.minute}:${valueParts.second}`
  return `${date} ${time}`
}

export type DateRangePreset = 'today' | 'yesterday' | 'last-week' | 'this-month' | 'last-month'

/** Returns a local-device half-open range for common report date shortcuts. */
export function dateRangePreset(preset: DateRangePreset, now = new Date()): { from: Date; to: Date } {
  const endNow = new Date(now)
  endNow.setMilliseconds(0)
  const startOfDay = (value: Date) => {
    const result = new Date(value)
    result.setHours(0, 0, 0, 0)
    return result
  }
  const today = startOfDay(now)
  if (preset === 'today') return { from: today, to: endNow }
  if (preset === 'yesterday') {
    const from = new Date(today)
    from.setDate(from.getDate() - 1)
    return { from, to: today }
  }
  if (preset === 'last-week') {
    const thisMonday = new Date(today)
    const day = thisMonday.getDay() || 7
    thisMonday.setDate(thisMonday.getDate() - day + 1)
    const from = new Date(thisMonday)
    from.setDate(from.getDate() - 7)
    return { from, to: thisMonday }
  }
  if (preset === 'this-month') return { from: new Date(today.getFullYear(), today.getMonth(), 1), to: endNow }
  return { from: new Date(today.getFullYear(), today.getMonth() - 1, 1), to: new Date(today.getFullYear(), today.getMonth(), 1) }
}
