import { describe, expect, it, vi } from 'vitest'
import { createPlatformReportsApi, REPORT_FIELDS, type ReportKind, type ReportQuery } from './reports-api'

const brandId = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'
const memberId = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb'
const from = '2026-10-09T00:00:00Z'
const to = '2026-10-10T00:00:00Z'
const query: ReportQuery = { from, to, group_by: 'day' }

const totals = (kind: ReportKind) => Object.fromEntries(REPORT_FIELDS[kind].map(field => [field, '0']))
function data(kind: ReportKind, overrides: Record<string, unknown> = {}) {
  const filters = kind === 'betting' || kind === 'ledger' || kind === 'withdrawal'
    ? { game_id: null, member_id: null }
    : kind === 'commission'
      ? { agent_id: null, member_id: null, cycle_id: null }
      : { member_id: null, order_id: null }
  return {
    brand_id: brandId,
    snapshot_at: '2026-10-10T00:00:00.123456Z',
    timezone: 'Asia/Singapore',
    query: { from, to, group_by: 'day', limit: 20, offset: 0, ...filters },
    summary: totals(kind),
    items: [{ key: '2026-10-09', label: '2026-10-09', totals: totals(kind) }],
    total_groups: '1',
    ...(kind === 'ledger' ? { balances: { account_count: '1', available_points: '3', frozen_points: '2', withdrawal_points: '4', total_points: '9' } } : {}),
    ...overrides,
  }
}
const response = (payload: unknown, status = 200) => new Response(JSON.stringify({ success: true, data: payload }), { status })
const fetcherFor = (payload: unknown) => vi.fn<typeof fetch>().mockResolvedValue(response(payload))

describe('platform reports read API', () => {
  it('accepts the current backend RFC3339 snapshot offset without changing its instant or text', async () => {
    const snapshot = '2026-10-10T08:00:00.123456+08:00'
    const result = await createPlatformReportsApi(fetcherFor(data('betting', { snapshot_at: snapshot }))).read(brandId, 'betting', query)
    expect(result.snapshot_at).toBe(snapshot)
  })
  it('reads all six current report kinds with exact metrics and backend paths', async () => {
    const kinds: ReportKind[] = ['betting', 'ledger', 'withdrawal', 'commission', 'rewards', 'reward_orders']
    for (const kind of kinds) {
      const fetcher = fetcherFor(data(kind))
      const result = await createPlatformReportsApi(fetcher).read(brandId, kind, query)
      expect(Object.keys(result.summary)).toEqual(REPORT_FIELDS[kind])
      expect(result.items).toHaveLength(1)
      expect(String(fetcher.mock.calls[0]?.[0])).toContain(`/api/v1/platform/reports/${kind === 'reward_orders' ? 'reward-orders' : kind}?`)
      expect(fetcher.mock.calls[0]?.[1]).toMatchObject({ method: 'GET', credentials: 'same-origin' })
      expect(new Headers(fetcher.mock.calls[0]?.[1]?.headers).get('X-Brand-ID')).toBe(brandId)
    }
  })

  it('returns ledger balances and preserves arbitrary precision totals and negative net points', async () => {
    const huge = '922337203685477580812345678901234567890'
    const ledger = data('ledger', { summary: { ...totals('ledger'), entry_count: huge, net_points: '-900719925474099312345' } })
    const result = await createPlatformReportsApi(fetcherFor(ledger)).read(brandId, 'ledger', query)
    expect(result.summary.entry_count).toBe(huge)
    expect(result.summary.net_points).toBe('-900719925474099312345')
    expect(result.balances?.total_points).toBe('9')
  })

  it('sends only the filters supported by a report and applies page defaults', async () => {
    const payload = data('commission', { query: { from, to, group_by: 'day', limit: 20, offset: 0, agent_id: memberId, member_id: null, cycle_id: null } })
    const fetcher = fetcherFor(payload)
    await createPlatformReportsApi(fetcher).read(brandId, 'commission', { ...query, agent_id: memberId })
    const params = new URL(String(fetcher.mock.calls[0]?.[0]), 'http://localhost').searchParams
    expect(Object.fromEntries(params)).toEqual({ from, to, group_by: 'day', limit: '20', offset: '0', agent_id: memberId })
  })

  it('rejects invalid brand IDs, pagination, unsupported filters, invalid periods, and fractional request seconds before fetching', async () => {
    const fetcher = fetcherFor(data('betting'))
    const api = createPlatformReportsApi(fetcher)
    await expect(api.read('bad', 'betting', query)).rejects.toMatchObject({ code: 'INVALID_INPUT' })
    await expect(api.read(brandId, 'betting', { ...query, limit: 101 })).rejects.toMatchObject({ code: 'INVALID_INPUT' })
    await expect(api.read(brandId, 'ledger', { ...query, game_id: memberId })).rejects.toMatchObject({ code: 'INVALID_INPUT' })
    await expect(api.read(brandId, 'betting', { ...query, to: '2027-01-11T00:00:00Z' })).rejects.toMatchObject({ code: 'INVALID_INPUT' })
    await expect(api.read(brandId, 'betting', { ...query, from: '2026-10-09T00:00:00.001Z' })).rejects.toMatchObject({ code: 'INVALID_INPUT' })
    expect(fetcher).not.toHaveBeenCalled()
  })

  it('rejects negative unsigned values and wrong metric fields without substituting data', async () => {
    const negative = data('withdrawal', { summary: { ...totals('withdrawal'), order_count: '-1' } })
    await expect(createPlatformReportsApi(fetcherFor(negative)).read(brandId, 'withdrawal', query)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const wrong = data('rewards', { summary: { ...totals('rewards'), legacy_total: '0' } })
    await expect(createPlatformReportsApi(fetcherFor(wrong)).read(brandId, 'rewards', query)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const wrongItem = data('ledger', { items: [{ key: '2026-10-09', label: '2026-10-09', totals: { ...totals('ledger'), surprise: '0' } }] })
    await expect(createPlatformReportsApi(fetcherFor(wrongItem)).read(brandId, 'ledger', query)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it('preserves authorization errors and reports network failures', async () => {
    const forbidden = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: false, error: { code: 'PLATFORM_ADMIN_REQUIRED', message: 'Forbidden' } }), { status: 403 }))
    await expect(createPlatformReportsApi(forbidden).read(brandId, 'betting', query)).rejects.toMatchObject({ status: 403, code: 'PLATFORM_ADMIN_REQUIRED' })
    const network = vi.fn<typeof fetch>().mockRejectedValue(new Error('offline'))
    await expect(createPlatformReportsApi(network).read(brandId, 'betting', query)).rejects.toMatchObject({ status: 0, code: 'NETWORK_ERROR', network: true })
  })

  it('rejects a response scoped to another brand or echoed with another page', async () => {
    const wrongBrand = data('betting', { brand_id: memberId })
    await expect(createPlatformReportsApi(fetcherFor(wrongBrand)).read(brandId, 'betting', query)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const wrongPage = data('betting', { query: { from, to, group_by: 'day', limit: 20, offset: 1, game_id: null, member_id: null } })
    await expect(createPlatformReportsApi(fetcherFor(wrongPage)).read(brandId, 'betting', query)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const emptyPage = data('betting', { query: { from, to, group_by: 'day', limit: 20, offset: 2, game_id: null, member_id: null }, items: [] })
    await expect(createPlatformReportsApi(fetcherFor(emptyPage)).read(brandId, 'betting', { ...query, offset: 2 })).resolves.toMatchObject({ items: [], total_groups: '1' })
  })
})
