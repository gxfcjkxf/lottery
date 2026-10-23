import { PlatformApiError, readPlatformEnvelope } from './platform-api'

import type { OverviewSummary, OverviewBrand } from '@lottery/shared'
export type { OverviewDay, OverviewBrand } from '@lottery/shared'
export { overviewChange } from '@lottery/shared'
export interface PlatformOverview extends OverviewSummary { brands: OverviewBrand[] }
const unsigned = /^(0|[1-9]\d*)$/
const signed = /^(0|-?[1-9]\d*)$/
const date = /^\d{4}-\d{2}-\d{2}$/
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
const counts = ['total_members', 'online_members', 'unsettled_points', 'available_points', 'frozen_points', 'withdrawal_points', 'withdrawal_count', 'abnormal_count', 'failed_settlement_count', 'cancellation_count', 'yesterday_same_time_stake'] as const
function object(value: unknown): value is Record<string, unknown> { return !!value && typeof value === 'object' && !Array.isArray(value) }
function number(value: unknown, negative = false): boolean { return typeof value === 'string' && (negative ? signed : unsigned).test(value) }
function invalid(): never { throw new PlatformApiError('Invalid overview response', 502, 'INVALID_RESPONSE') }
export function validateOverview(value: unknown): PlatformOverview {
  if (!object(value) || typeof value.snapshot_at !== 'string' || !Number.isFinite(Date.parse(value.snapshot_at)) ||
    typeof value.timezone !== 'string' || typeof value.today !== 'string' || !date.test(value.today) ||
    counts.some(key => !number(value[key])) || !Array.isArray(value.days) || value.days.length !== 7 || !Array.isArray(value.brands) || value.brands.length > 10) invalid()
  try { new Intl.DateTimeFormat('en', { timeZone: value.timezone }).format(0) } catch { invalid() }
  for (const [index, day] of value.days.entries()) {
    if (!object(day) || typeof day.day !== 'string' || !date.test(day.day) ||
      ['stake_points', 'order_count', 'bettor_count', 'refund_points', 'new_members'].some(key => !number(day[key])) || !number(day.net_points, true)) invalid()
    const expected = new Date(`${value.today}T00:00:00Z`)
    expected.setUTCDate(expected.getUTCDate() - 6 + index)
    if (day.day !== expected.toISOString().slice(0, 10)) invalid()
  }
  const seen = new Set<string>()
  for (const brand of value.brands) {
    if (!object(brand) || typeof brand.brand_id !== 'string' || !uuid.test(brand.brand_id) || seen.has(brand.brand_id) ||
      typeof brand.brand_name !== 'string' || !brand.brand_name.trim() || !number(brand.stake_points) || !number(brand.net_points, true)) invalid()
    seen.add(brand.brand_id)
  }
  return value as unknown as PlatformOverview
}
export function createOverviewApi(fetcher: typeof fetch = fetch) {
  return {
    async read(signal?: AbortSignal): Promise<PlatformOverview> {
      let response: Response
      try { response = await fetcher('/api/v1/platform/overview', { method: 'GET', credentials: 'same-origin', headers: { Accept: 'application/json' }, signal }) }
      catch (cause) {
        if (signal?.aborted) throw cause
        throw new PlatformApiError('Overview request failed', 0, 'NETWORK_ERROR', true)
      }
      return validateOverview((await readPlatformEnvelope<unknown>(response)).data)
    },
  }
}

