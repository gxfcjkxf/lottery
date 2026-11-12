import { describe, expect, it, vi } from 'vitest'
import { createOverviewApi, overviewChange, validateOverview } from './overview-api'
import { overviewCatalog } from '@lottery/shared'

function fixture() {
  return {
    snapshot_at: '2026-10-20T06:30:00Z', timezone: 'Asia/Singapore', today: '2026-10-20',
    total_members: '123', online_members: '3', unsettled_points: '9007199254740993',
    available_points: '99', frozen_points: '12', withdrawal_points: '0', withdrawal_count: '0',
    yesterday_same_time_stake: '60',
    days: Array.from({ length: 7 }, (_, i) => ({ day: `2026-10-${14 + i}`, stake_points: '100', net_points: '-30', order_count: '2', bettor_count: '1', refund_points: '0', new_members: '1' })),
    brands: [{ brand_id: '0199a000-0000-7000-8000-000000000001', brand_name: 'Aurora', stake_points: '100', net_points: '-30' }],
  }
}
describe('platform overview', () => {
  it('reads the global endpoint without a brand header and preserves exact integers', async () => {
    const fetcher = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ success: true, data: fixture(), request_id: 'overview-read' }), { status: 200 }))
    expect((await createOverviewApi(fetcher).read()).unsettled_points).toBe('9007199254740993')
    expect(fetcher.mock.calls[0][0]).toBe('/api/v1/platform/overview')
    expect(new Headers(fetcher.mock.calls[0][1]?.headers).has('X-Brand-ID')).toBe(false)
  })
  it('accepts exact decimal totals while keeping counts integral', () => {
    const data = fixture()
    Object.assign(data, { unsettled_points: '9007199254740993.25', available_points: '100.00', frozen_points: '0.50', withdrawal_points: '5.01', yesterday_same_time_stake: '60.25' })
    Object.assign(data.days[0]!, { stake_points: '9007199254740993.25', net_points: '-30.75', refund_points: '0.25' })
    Object.assign(data.brands[0]!, { stake_points: '100.25', net_points: '-30.75' })
    expect(validateOverview(data)).toBe(data)
    for (const key of ['total_members', 'online_members', 'withdrawal_count'] as const) {
      expect(() => validateOverview({ ...data, [key]: '1.5' })).toThrow()
    }
    for (const key of ['unsettled_points', 'available_points', 'frozen_points', 'withdrawal_points', 'yesterday_same_time_stake'] as const) {
      for (const invalid of ['-0.01', '1.001', '01.25', '1e2', 'NaN']) {
        expect(() => validateOverview({ ...data, [key]: invalid })).toThrow()
      }
    }
    for (const key of ['order_count', 'bettor_count', 'new_members'] as const) {
      const malformed = { ...data, days: data.days.map(day => ({ ...day, [key]: '1.5' })) }
      expect(() => validateOverview(malformed)).toThrow()
    }
    for (const [key, invalid] of [['stake_points', '-1.5'], ['refund_points', '1.001'], ['net_points', '-0.00']] as const) {
      const malformed = { ...data, days: data.days.map(day => ({ ...day, [key]: invalid })) }
      expect(() => validateOverview(malformed)).toThrow()
    }
  })
  it('rejects malformed counts, incomplete dates and anonymous brand labels', () => {
    expect(() => validateOverview({ ...fixture(), online_members: 0 })).toThrow()
    expect(() => validateOverview({ ...fixture(), days: fixture().days.slice(1) })).toThrow()
    const data = fixture(); data.days[0].day = '2026-10-15'
    expect(() => validateOverview(data)).toThrow()
    expect(() => validateOverview({ ...fixture(), brands: [{ ...fixture().brands[0], brand_name: '' }] })).toThrow()
    expect(() => validateOverview({ ...fixture(), timezone: 'not/a-zone' })).toThrow()
  })
  it('propagates permission, network and aborted reads instead of producing zero totals', async () => {
    const denied = createOverviewApi(vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ success: false, error: { code: 'PERMISSION_DENIED', message: 'internal' }, request_id: 'overview-read' }), { status: 403 })))
    await expect(denied.read()).rejects.toMatchObject({ status: 403, code: 'PERMISSION_DENIED' })
    await expect(createOverviewApi(vi.fn<typeof fetch>(async () => { throw new Error('offline') })).read()).rejects.toMatchObject({ code: 'NETWORK_ERROR' })
    const controller = new AbortController(); controller.abort()
    const cause = new DOMException('Aborted', 'AbortError')
    await expect(createOverviewApi(vi.fn<typeof fetch>(async () => { throw cause })).read(controller.signal)).rejects.toBe(cause)
  })
  it('compares exact amounts and identifies an absent comparison explicitly', () => {
    expect(overviewChange('90071992547409930', '9007199254740993')).toBe('+900.0%')
    expect(overviewChange('50', '100')).toBe('−50.0%')
    expect(overviewChange('0', '0')).toBeNull()
    expect(overviewChange('0', '100')).toBe('−100.0%')
  })
  it('provides matching catalogs for both platform languages', () => {
    expect(Object.keys(overviewCatalog.en).sort()).toEqual(Object.keys(overviewCatalog['zh-CN']).sort())
    expect(Object.values(overviewCatalog.en).every(value => !/[\u3400-\u9fff]/u.test(value))).toBe(true)
  })
})
