import { describe, it, expect, vi } from 'vitest'
import { readFilterOptions, readAllFilterOptions } from './filter-options'
const scope = { surface: 'admin' as const, brandId: 'brand-a', resource: 'bet_policy', kind: 'game' }
const ok = (items: unknown[], offset = 0, brand_id = 'brand-a') => new Response(JSON.stringify({ success: true, data: { brand_id, items, limit: 100, offset } }))
describe('shared scoped directory helper', () => {
  it('reads every page using the same resource and brand, without using a legacy endpoint', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok(Array.from({ length: 100 }, (_, i) => ({ id: String(i), name: `Game ${i}` })))).mockResolvedValueOnce(ok([{ id: 'last', name: 'Last' }], 100))
    expect(await readAllFilterOptions(scope, fetcher)).toHaveLength(101)
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual(['/api/v1/admin/filter-options?resource=bet_policy&kind=game&limit=100&offset=0', '/api/v1/admin/filter-options?resource=bet_policy&kind=game&limit=100&offset=100'])
    expect(new Headers(fetcher.mock.calls[1]![1]!.headers).get('X-Brand-ID')).toBe('brand-a')
  })
  it('passes account searches to the authorized server directory on each page', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok([], 100))
    await readFilterOptions({ ...scope, resource: 'report_betting', kind: 'member', search: 'member+trial' }, 100, fetcher)
    const url = new URL(String(fetcher.mock.calls[0]![0]), 'http://localhost')
    expect(url.searchParams.get('search')).toBe('member+trial')
    expect(url.searchParams.get('offset')).toBe('100')
    expect(new Headers(fetcher.mock.calls[0]![1]!.headers).get('X-Brand-ID')).toBe('brand-a')
  })
  it('rejects another brand, malformed records and wrong pagination', async () => {
    for (const response of [ok([], 0, 'brand-b'), ok([{ id: '', name: 'Game' }]), ok([], 100)]) await expect(readFilterOptions(scope, 0, vi.fn().mockResolvedValue(response))).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })
  it('preserves permission errors and never silently changes the requested directory', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: false, error: { code: 'PERMISSION_DENIED', message: 'Denied' } }), { status: 403 }))
    await expect(readAllFilterOptions(scope, fetcher)).rejects.toMatchObject({ status: 403, code: 'PERMISSION_DENIED' })
    expect(fetcher).toHaveBeenCalledTimes(1)
  })
  it('stops on a later-page failure instead of returning a partial directory', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok(Array.from({ length: 100 }, (_, i) => ({ id: String(i), name: 'Game' })))).mockRejectedValueOnce(Error('Offline'))
    await expect(readAllFilterOptions(scope, fetcher)).rejects.toMatchObject({ code: 'NETWORK_ERROR' })
  })
})
