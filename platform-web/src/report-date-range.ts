import { PlatformApiError } from './platform-api'

/** Report API windows use UTC and an exclusive end; selected dates include both days. */
export function reportDateRange(from: string, to: string): { from: string; to: string } {
  const start = new Date(`${from}T00:00:00Z`)
  const end = new Date(`${to}T00:00:00Z`)
  if (!/^\d{4}-\d{2}-\d{2}$/.test(from) || !/^\d{4}-\d{2}-\d{2}$/.test(to) || !Number.isFinite(start.getTime()) || !Number.isFinite(end.getTime()) || start.toISOString().slice(0, 10) !== from || end.toISOString().slice(0, 10) !== to || end < start) throw new PlatformApiError('Invalid report dates', 0, 'INVALID_INPUT')
  end.setUTCDate(end.getUTCDate() + 1)
  return { from: start.toISOString().replace('.000Z', 'Z'), to: end.toISOString().replace('.000Z', 'Z') }
}
