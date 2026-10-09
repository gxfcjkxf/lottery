import { describe, expect, it, vi } from 'vitest'
import { createPlatformFinancialPoliciesApi } from './financial-policies-api'

const brand = '11111111-1111-4111-8111-111111111111'
const otherBrand = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'
const id = '22222222-2222-4222-8222-222222222222'
const at = '2026-10-10T08:30:00Z'
const ok = (data: unknown, status = 200) => new Response(JSON.stringify({ success: true, data }), { status })
const points = { brand_id: brand, version: 1, max_balance_points: null, max_recharge_points: '9007199254740993', max_adjustment_points: null }
const withdrawal = {
  brand_id: brand, version: 1,
  config: { enabled: false, min_points: '1', max_points: null, allowed_sources: ['recharge', 'winning', 'gift'], review_mode: 'manual', turnover_multiple: '1' },
  updated_at: at,
}
const commission = {
  brand_id: brand, version: 1,
  config: { enabled: false, calendar: null, payout_mode: 'manual' },
  created_at: at, updated_at: at, revision_id: id,
}

describe('platform financial policy read API', () => {
  it('reads the three current policy DTOs with exact brand-scoped GETs and no query', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok(points)).mockResolvedValueOnce(ok(withdrawal)).mockResolvedValueOnce(ok(commission))
    const api = createPlatformFinancialPoliciesApi(fetcher)

    await expect(api.points(brand)).resolves.toEqual(points)
    await expect(api.withdrawal(brand)).resolves.toEqual(withdrawal)
    await expect(api.commission(brand)).resolves.toEqual(commission)

    expect(fetcher.mock.calls.map(call => call[0])).toEqual([
      '/api/v1/platform/point-policy', '/api/v1/platform/withdrawal-policy', '/api/v1/platform/commission-policy',
    ])
    for (const [, init] of fetcher.mock.calls) {
      expect(init?.method).toBe('GET')
      expect(init?.credentials).toBe('same-origin')
      expect(new Headers(init?.headers).get('X-Brand-ID')).toBe(brand)
      expect(init?.body).toBeUndefined()
    }
    expect(Object.keys(api)).toEqual(['points', 'withdrawal', 'commission'])
  })

  it('accepts updated policies, cleared point caps, large exact amounts, and nondefault settings', async () => {
    const updatedPoints = { ...points, version: 3, max_balance_points: '1', max_recharge_points: null, max_adjustment_points: '9223372036854775807' }
    const updatedWithdrawal = {
      ...withdrawal, version: 2, audit_log_id: id,
      config: { enabled: true, min_points: '25', max_points: '9007199254740993', allowed_sources: ['commission', 'gift'], review_mode: 'automatic', turnover_multiple: '1000000' },
    }
    const updatedCommission = {
      ...commission, version: 2, audit_log_id: id, revision_id: otherBrand,
      config: { enabled: true, payout_mode: 'automatic', calendar: { timezone: 'Asia/Singapore', cycle: 'monthly', boundary_time: '18:30:00', weekday: null, month_day: 31, short_month: 'last_day' } },
    }
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok(updatedPoints)).mockResolvedValueOnce(ok(updatedWithdrawal)).mockResolvedValueOnce(ok(updatedCommission))
    const api = createPlatformFinancialPoliciesApi(fetcher)
    await expect(api.points(brand)).resolves.toEqual(updatedPoints)
    await expect(api.withdrawal(brand)).resolves.toEqual(updatedWithdrawal)
    await expect(api.commission(brand)).resolves.toEqual(updatedCommission)
    await expect(createPlatformFinancialPoliciesApi(async () => ok({ ...updatedWithdrawal, config: { ...updatedWithdrawal.config, turnover_multiple: '999999.999999' } })).withdrawal(brand)).resolves.toMatchObject({ config: { turnover_multiple: '999999.999999' } })
  })

  it('rejects invalid brand inputs and malformed policy DTOs before trusting them', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(points))
    const api = createPlatformFinancialPoliciesApi(fetcher)
    await expect(api.points('bad-brand')).rejects.toMatchObject({ code: 'BRAND_REQUIRED' })
    expect(fetcher).not.toHaveBeenCalled()

    const invalidCases: Array<[unknown, (api: ReturnType<typeof createPlatformFinancialPoliciesApi>) => Promise<unknown>]> = [
      [{ ...points, brand_id: otherBrand }, client => client.points(brand)],
      [{ ...points, max_balance_points: '0' }, client => client.points(brand)],
      [{ ...points, max_adjustment_points: 12 }, client => client.points(brand)],
      [{ ...withdrawal, config: { ...withdrawal.config, min_points: 1 } }, client => client.withdrawal(brand)],
      [{ ...withdrawal, config: { ...withdrawal.config, allowed_sources: ['gift', 'gift'] } }, client => client.withdrawal(brand)],
      [{ ...withdrawal, config: { ...withdrawal.config, max_points: '0' } }, client => client.withdrawal(brand)],
      [{ ...withdrawal, config: { ...withdrawal.config, turnover_multiple: '0.0000001' } }, client => client.withdrawal(brand)],
      [{ ...withdrawal, config: { ...withdrawal.config, turnover_multiple: '1000000.000001' } }, client => client.withdrawal(brand)],
      [{ ...commission, revision_id: '' }, client => client.commission(brand)],
      [{ ...commission, config: { ...commission.config, enabled: true } }, client => client.commission(brand)],
      [{ ...commission, config: { ...commission.config, calendar: { timezone: 'Asia/Singapore', cycle: 'weekly', boundary_time: '18:30:00', weekday: 1, month_day: 1, short_month: '' } } }, client => client.commission(brand)],
    ]
    for (const [data, call] of invalidCases) {
      const bad = vi.fn<typeof fetch>().mockResolvedValue(ok(data))
      await expect(call(createPlatformFinancialPoliciesApi(bad))).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    }
  })

  it('preserves a 403 response and rejects success payloads with a non-200 status', async () => {
    const denied = new Response(JSON.stringify({ success: false, error: { code: 'PERMISSION_DENIED', message: 'Denied' } }), { status: 403 })
    await expect(createPlatformFinancialPoliciesApi(vi.fn<typeof fetch>().mockResolvedValue(denied)).points(brand))
      .rejects.toMatchObject({ status: 403, code: 'PERMISSION_DENIED', message: 'Denied' })
    await expect(createPlatformFinancialPoliciesApi(vi.fn<typeof fetch>().mockResolvedValue(ok(points, 201))).points(brand))
      .rejects.toMatchObject({ status: 201, code: 'INVALID_RESPONSE' })
  })
})
