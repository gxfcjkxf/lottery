import { afterEach, expect, it, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { configuredDemo } from '@lottery/shared'
import { readNotificationOrder } from './notification-order'

const scope = { brand_id: 'brand', member_id: 'member' }
const snapshot = configuredDemo(JSON.parse(readFileSync(new URL('../../backend/internal/rules/config/mark_six_v2.json', import.meta.url), 'utf8')))
const play = snapshot.catalog.plays[0]!, option = snapshot.catalog.domains[play.selection.domain]!.options[0]!
const order = { id: 'order-42', game_name: 'New Macau', period_no: '2026302', snapshot, quote: { bet_points: '25', lines: [{ play_id: play.id, options: [option.id], stake: '25', prices: { win: '48.99' } }] } }
function respond(data: unknown) { return new Response(JSON.stringify({ success: true, data }), { status: 200, headers: { 'Content-Type': 'application/json' } }) }
afterEach(() => vi.unstubAllGlobals())

it('reads a scoped historical order with its purchased rules and prices', async () => {
  const fetcher = vi.fn().mockResolvedValue(respond({ ...scope, order }))
  vi.stubGlobal('fetch', fetcher)
  await expect(readNotificationOrder('aurora', scope, order.id)).resolves.toMatchObject(order)
  expect(fetcher.mock.calls[0][0]).toBe('/api/v1/b/aurora/configured/orders/order-42')
})

it('rejects another account, brand, order or unusable purchased ticket', async () => {
  for (const data of [
    { ...scope, member_id: 'other', order }, { ...scope, brand_id: 'other', order },
    { ...scope, order: { ...order, id: 'other' } },
    { ...scope, order: { ...order, snapshot: {} } },
    { ...scope, order: { ...order, quote: { ...order.quote, lines: [{ ...order.quote.lines[0], play_id: 'missing-play' }] } } },
  ]) {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(respond(data)))
    await expect(readNotificationOrder(undefined, scope, order.id)).rejects.toThrow()
  }
})
