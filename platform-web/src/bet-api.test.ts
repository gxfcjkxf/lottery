import { describe, expect, it, vi } from 'vitest'
import { PlatformApiError } from './platform-api'
import { createPlatformBetApi, type BetOrder } from './bet-api'

const brand = '11111111-1111-4111-8111-111111111111'
const member = '22222222-2222-4222-8222-222222222222'
const orderId = '33333333-3333-4333-8333-333333333333'
const selection = { regular: [1], special: null, digits: null, exclude: null, attributes: null, features: null }
const definition = {
  schema_version: 1, model: { model: 'pick', regular_pool: { min: 1, max: 9, allow_repeat: false }, special_pool: { allow_repeat: false }, regular_count: 1, special_count: 0, pool_size: 9, total_count: 9, length: 1, allow_repeat: false, ordered: false },
  selection: { mode: 'numbers', regular_count: 1, special_count: 0, exclude_count: 0, attribute_groups: null, feature_choices: null }, number_attributes: null, unit_points: '1', prize_tiers: null, mixed_tier_policy: 'max_all', cap_points: null, rounding: 'half_up', rounding_scope: 'order', limits: { max_combinations: 100, max_multiplier: '100', max_bet_points: null },
}
const order: BetOrder = {
  id: orderId, brand_id: brand, global_user_id: '44444444-4444-4444-8444-444444444444', brand_member_id: member, account_id: '55555555-5555-4555-8555-555555555555', game_id: '66666666-6666-4666-8666-666666666666', period_id: '77777777-7777-4777-8777-777777777777', play_id: '88888888-8888-4888-8888-888888888888', rule_version_id: '99999999-9999-4999-8999-999999999999', definition_hash: 'a'.repeat(64), definition_snapshot: definition as BetOrder['definition_snapshot'], status: 'placed', version: 1, selection_raw: selection, selection_normalized: selection, expanded_bets: [selection], unit_points: '1', combination_count: 1, multiplier: '1', total_points: '1', deduction_allocation: [{ source: 'recharge', state: 'available', points: '1' }], policy_snapshot: { min_bet_points: '1', max_bet_points: null, max_period_points: null, max_user_period_points: null, user_cancel_allowed: true }, policy_versions: { brand: 1, game: 1 }, debit_entry_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', client_key: 'key-1', placed_at: '2026-01-02T03:04:05Z',
  settlement_calculation_id: null, payout_entry_id: null, prize_points: '0', settled_at: null,
}
const ok = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { status: 200 })

describe('platform bet API', () => {
  it('requires current settlement fields and rejects non-wallet allocation sources', async () => {
    const { prize_points: _prize, ...missing } = order
    await expect(createPlatformBetApi(async () => ok(missing)).read(brand, orderId)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformBetApi(async () => ok({ ...order, deduction_allocation: [{ source: 'cash', state: 'posted', points: '1' }] })).read(brand, orderId)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformBetApi(async () => ok({ ...order, prize_points: '0' })).read(brand, orderId)).resolves.toMatchObject({ prize_points: '0' })
  })
  it('lists with default/selected filtering and pagination, and returns an array', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok({ items: [order] })).mockResolvedValueOnce(ok({ items: [order] }))
    const api = createPlatformBetApi(fetcher)
    await expect(api.list(brand)).resolves.toEqual([order])
    await expect(api.list(brand, member, 8, 16)).resolves.toEqual([order])
    expect(String(fetcher.mock.calls[0][0])).toBe(`/api/v1/platform/bet-orders?limit=51&offset=0`)
    expect(String(fetcher.mock.calls[1][0])).toBe(`/api/v1/platform/bet-orders?limit=8&offset=16&member_id=${member}`)
    for (const [, init] of fetcher.mock.calls) {
      expect(init?.method).toBe('GET')
      expect(init?.credentials).toBe('same-origin')
      expect(new Headers(init?.headers).get('X-Brand-ID')).toBe(brand)
    }
  })

  it('reads one matching order and rejects foreign scope or mismatched IDs', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(order))
    await expect(createPlatformBetApi(fetcher).read(brand, orderId)).resolves.toEqual(order)
    fetcher.mockResolvedValueOnce(ok({ ...order, brand_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa' }))
    await expect(createPlatformBetApi(fetcher).read(brand, orderId)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    fetcher.mockResolvedValueOnce(ok({ ...order, id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa' }))
    await expect(createPlatformBetApi(fetcher).read(brand, orderId)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it('rejects foreign member filters and malformed or non-integer monetary strings', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok({ items: [{ ...order, brand_member_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa' }] }))
    await expect(createPlatformBetApi(fetcher).list(brand, member)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    fetcher.mockResolvedValueOnce(ok({ items: [{ ...order, total_points: '1.0' }] }))
    await expect(createPlatformBetApi(fetcher).list(brand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it.each([[401, 'AUTH_SESSION_REVOKED'], [403, 'PERMISSION_DENIED']])('preserves structured %i errors', async (status, code) => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: false, error: { code, message: 'denied' } }), { status }))
    await expect(createPlatformBetApi(fetcher).list(brand)).rejects.toMatchObject({ status, code, message: 'denied' })
  })

  it('requires brand before fetching and exposes no write methods', async () => {
    const fetcher = vi.fn<typeof fetch>()
    const api = createPlatformBetApi(fetcher)
    await expect(api.list('')).rejects.toBeInstanceOf(PlatformApiError)
    expect(fetcher).not.toHaveBeenCalled()
    expect(Object.keys(api)).toEqual(['list', 'read'])
  })
})
