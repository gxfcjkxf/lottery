import { platformDateBoundary } from '@lottery/shared'
import { PlatformApiError } from './platform-api'

/** Report API windows use UTC and an exclusive end; selected dates include both days. */
export function reportDateRange(from: string, to: string, timezone: string): { from: string; to: string } {
  const start = platformDateBoundary(from, false, timezone)
  const end = platformDateBoundary(to, true, timezone)
  if (end <= start) throw new PlatformApiError('Invalid report dates', 0, 'INVALID_INPUT')
  return { from: start.toISOString().replace('.000Z', 'Z'), to: end.toISOString().replace('.000Z', 'Z') }
}
