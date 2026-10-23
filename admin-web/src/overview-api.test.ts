import { describe, expect, it, vi } from 'vitest'
import { createBrandOverviewApi, validateOverview } from './overview-api'
import { brandOverviewCatalog, overviewChange } from '@lottery/shared'

const brand = '0199a000-0000-7000-8000-000000000001'
const other = '0199a000-0000-7000-8000-000000000002'
const fixture = () => ({
  brand_id: brand, snapshot_at: '2026-10-20T06:30:00Z', timezone: 'Asia/Singapore', today: '2026-10-20',
  total_members: '123', online_members: '3', unsettled_points: '9007199254740993', available_points: '100', frozen_points: '20',
  withdrawal_points: '5', withdrawal_count: '1', abnormal_count: '0', failed_settlement_count: '0', cancellation_count: '0', yesterday_same_time_stake: '60',
  days: Array.from({ length: 7 }, (_, i) => ({ day: `2026-10-${14 + i}`, stake_points: '100', net_points: '-30', order_count: '2', bettor_count: '1', refund_points: '0', new_members: '1' })),
})
const response = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { status: 200 })
describe('brand overview API', () => {
  it('requires the brand header and preserves exact amounts', async () => {
    const fetcher = vi.fn<typeof fetch>(async () => response(fixture()))
    const data = await createBrandOverviewApi(fetcher).read(brand)
    expect(data.unsettled_points).toBe('9007199254740993')
    expect(fetcher.mock.calls[0][0]).toBe('/api/v1/admin/overview')
    expect(new Headers(fetcher.mock.calls[0][1]?.headers).get('X-Brand-ID')).toBe(brand)
  })
  it('rejects another brand and global responses rather than displaying them', async () => {
    expect(() => validateOverview({ ...fixture(), brand_id: other }, brand)).toThrow()
    const { brand_id: _brand, ...global } = fixture()
    expect(() => validateOverview(global, brand)).toThrow()
    const fetcher = vi.fn<typeof fetch>()
    await expect(createBrandOverviewApi(fetcher).read('')).rejects.toMatchObject({ code: 'REQUEST_INVALID' })
    expect(fetcher).not.toHaveBeenCalled()
  })
  it('rejects malformed counts and incomplete days', () => {
    expect(() => validateOverview({ ...fixture(), online_members: 0 }, brand)).toThrow()
    expect(() => validateOverview({ ...fixture(), days: [] }, brand)).toThrow()
    const data = fixture(); data.days[0].day = '2026-10-15'
    expect(() => validateOverview(data, brand)).toThrow()
  })
  it('propagates permission and aborted reads', async () => {
    const fetcher = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ success: false, error: { code: 'PERMISSION_DENIED', message: 'internal' } }), { status: 403 }))
    await expect(createBrandOverviewApi(fetcher).read(brand)).rejects.toMatchObject({ status: 403 })
    const controller = new AbortController(); controller.abort()
    const cause = new DOMException('Aborted', 'AbortError')
    await expect(createBrandOverviewApi(vi.fn<typeof fetch>(async () => { throw cause })).read(brand, controller.signal)).rejects.toBe(cause)
  })
  it('provides both languages and shares precision-safe comparisons', () => {
    expect(Object.keys(brandOverviewCatalog.en).sort()).toEqual(Object.keys(brandOverviewCatalog['zh-CN']).sort())
    expect(brandOverviewCatalog['zh-CN']['overview.newNote']).toContain('加入当前品牌')
    expect(overviewChange('90071992547409930', '9007199254740993')).toBe('+900.0%')
  })
})
