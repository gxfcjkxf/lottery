import { afterEach, describe, expect, it, vi } from 'vitest'
import { readBettingHistory } from './betting-history-api'
const brand = '11111111-1111-4111-8111-111111111111', member = '22222222-2222-4222-8222-222222222222'
const game = '33333333-3333-4333-8333-333333333333', at = '2026-10-20T00:00:00Z'
const scope = { brand_id: brand, member_id: member }
const item = (kind: string, index: number) => ({ kind, id: String(index), game_id: game, game_name: 'Retired custom name', period_no: 'period-001', placed_at: at,
  order: { id: String(index), game_id: game, total_points: '9007199254740993', prize_points: '0', snapshot: {}, quote: { lines: [], bet_points: '9007199254740993' } } })
const data = () => ({ ...scope, game_id: '', games: [{ id: game, name: 'Retired custom name' }], items: ['configured', 'standard', 'mark-six', 'canada'].map(item), limit: 20, next_cursor: null })
const ok = (value: unknown) => new Response(JSON.stringify({ success: true, data: value }))
afterEach(() => vi.unstubAllGlobals())
describe('one betting-history API', () => {
  it('preserves all historical payloads and exact amounts with one brand-scoped request', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () => ok(data())); vi.stubGlobal('fetch', fetcher)
    const page = await readBettingHistory('my brand', scope)
    expect(page.items.map(i => i.kind)).toEqual(['configured', 'standard', 'mark-six', 'canada'])
    expect(page.items[0]!.order).toMatchObject({ quote: { bet_points: '9007199254740993' } })
    expect(fetcher.mock.calls[0]![0]).toBe('/api/v1/b/my%20brand/betting-history?limit=20')
    expect(fetcher).toHaveBeenCalledOnce()
    expect(fetcher.mock.calls[0]![1]).toMatchObject({ credentials: 'same-origin' })
  })
  it('passes the same selected game, cursor and cancellation signal to the unified endpoint', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () => ok({ ...data(), game_id: game })); vi.stubGlobal('fetch', fetcher)
    const controller = new AbortController()
    await readBettingHistory(undefined, scope, game, 'opaque/page+2', controller.signal)
    const [url, init] = fetcher.mock.calls[0]!
    const query = new URL(String(url), 'http://local').searchParams
    expect(query.get('game_id')).toBe(game); expect(query.get('cursor')).toBe('opaque/page+2')
    expect([...query.keys()]).toEqual(['limit', 'game_id', 'cursor']); expect(init!.signal).toBe(controller.signal)
  })
  it('rejects foreign scope, unknown sources, incomplete pages and numeric money instead of silently dropping tickets', async () => {
    for (const change of [
      { member_id: 'other' }, { brand_id: 'other' }, { game_id: game }, { limit: 50 }, { next_cursor: '' },
      { items: [item('unsupported', 1)] }, { games: [] },
      { items: [{ ...item('standard', 1), order: { ...item('standard', 1).order, total_points: 123 } }] },
      { items: [item('standard', 1), item('standard', 1)] },
    ]) {
      vi.stubGlobal('fetch', vi.fn<typeof fetch>().mockResolvedValue(ok({ ...data(), ...change })))
      await expect(readBettingHistory(undefined, scope)).rejects.toThrow('Invalid')
    }
  })
})
