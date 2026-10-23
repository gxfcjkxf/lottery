import { AdminApiError, readAdminEnvelope } from './admin-api'

import type { OverviewSummary } from '@lottery/shared'
export interface BrandOverview extends OverviewSummary { brand_id: string }
const unsigned = /^(0|[1-9]\d*)$/
const signed = /^(0|-?[1-9]\d*)$/
const date = /^\d{4}-\d{2}-\d{2}$/
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
const counts = ['total_members', 'online_members', 'unsettled_points', 'available_points', 'frozen_points', 'withdrawal_points', 'withdrawal_count', 'abnormal_count', 'failed_settlement_count', 'cancellation_count', 'yesterday_same_time_stake'] as const
function object(value: unknown): value is Record<string, unknown> { return !!value && typeof value === 'object' && !Array.isArray(value) }
function number(value: unknown, negative = false): boolean { return typeof value === 'string' && (negative ? signed : unsigned).test(value) }
function invalid(): never { throw new AdminApiError('Invalid overview response', 502, 'INVALID_RESPONSE') }
export function validateOverview(value: unknown, brandId: string): BrandOverview {
  if (!object(value) || value.brand_id !== brandId || typeof value.snapshot_at !== 'string' || !Number.isFinite(Date.parse(value.snapshot_at)) ||
    typeof value.timezone !== 'string' || typeof value.today !== 'string' || !date.test(value.today) ||
    counts.some(key => !number(value[key])) || !Array.isArray(value.days) || value.days.length !== 7) invalid()
  try { new Intl.DateTimeFormat('en', { timeZone: value.timezone }).format(0) } catch { invalid() }
  for (const [index, day] of value.days.entries()) {
    if (!object(day) || typeof day.day !== 'string' || !date.test(day.day) ||
      ['stake_points', 'order_count', 'bettor_count', 'refund_points', 'new_members'].some(key => !number(day[key])) || !number(day.net_points, true)) invalid()
    const expected = new Date(`${value.today}T00:00:00Z`)
    expected.setUTCDate(expected.getUTCDate() - 6 + index)
    if (day.day !== expected.toISOString().slice(0, 10)) invalid()
  }
  return value as unknown as BrandOverview
}
export function createBrandOverviewApi(fetcher: typeof fetch = fetch) {
  return {
    async read(brandId: string, signal?: AbortSignal): Promise<BrandOverview> {
      if (!uuid.test(brandId)) throw new AdminApiError('A valid brand is required', 400, 'REQUEST_INVALID')
      const brand = brandId.toLowerCase()
      let response: Response
      try { response = await fetcher('/api/v1/admin/overview', { method: 'GET', credentials: 'same-origin', headers: { Accept: 'application/json', 'X-Brand-ID': brand }, signal }) }
      catch (cause) {
        if (signal?.aborted) throw cause
        throw new AdminApiError('Overview request failed', 0, 'NETWORK_ERROR')
      }
      return validateOverview((await readAdminEnvelope<unknown>(response)).data, brand)
    },
  }
}

