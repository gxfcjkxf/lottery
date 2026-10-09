import { describe, expect, it, vi } from 'vitest'
import type { WithdrawalHistory, WithdrawalOrder, WithdrawalPage, WithdrawalState } from '@lottery/shared'
import { PlatformApiError } from './platform-api'
import { createPlatformWithdrawalApi } from './withdrawal-api'

const brand = '11111111-1111-4111-8111-111111111111'
const member = '33333333-3333-4333-8333-333333333333'
const orderId = '22222222-2222-4222-8222-222222222222'
const auditId = '44444444-4444-4444-8444-444444444444'
const at = '2026-10-07T12:00:00Z'
const order: WithdrawalOrder = {
  id: orderId, brand_id: brand, member_id: member, account_id: '55555555-5555-4555-8555-555555555555', points: '9007199254740993', state: 'reviewing', version: 1,
  source_allocation: [{ source: 'recharge', state: 'available', points: '9007199254740993' }], reserve_entry_id: '66666666-6666-4666-8666-666666666666',
  release_entry_id: null, paid_entry_id: null, cycle_from_at: null, cycle_from_version: '0', reserve_version: '1', created_at: at, updated_at: at,
  reviewed_at: null, completed_at: null, decision_reason: '', audit_log_id: auditId,
}
const page: WithdrawalPage = { brand_id: brand, items: [order], limit: 20, offset: 0, has_more: false }
const history: WithdrawalHistory = { brand_id: brand, order_id: orderId, items: [{ id: '77777777-7777-4777-8777-777777777777', version: 1, from_state: '', to_state: 'reviewing', reason: '', actor_type: 'user', created_at: at, audit_log_id: auditId }] }
const ok = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { status: 200 })

describe('platform withdrawal API', () => {
  it('uses the exact platform list route, provided filters, pagination, and DTO shape', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(page))
    await expect(createPlatformWithdrawalApi(fetcher).list(brand, { state: 'reviewing', memberId: member, limit: 20, offset: 0 })).resolves.toEqual(page)
    const [url, init] = fetcher.mock.calls[0]!
    expect(url).toBe(`${'/api/v1/platform/withdrawals'}?state=reviewing&member_id=${member}&limit=20&offset=0`)
    expect(init?.method).toBe('GET')
    expect(init?.credentials).toBe('same-origin')
    expect(new Headers(init?.headers).get('X-Brand-ID')).toBe(brand)
    expect(init?.body).toBeUndefined()
    const noFilters = vi.fn<typeof fetch>().mockResolvedValue(ok({ ...page, items: [], limit: 3, offset: 6 }))
    await createPlatformWithdrawalApi(noFilters).list(brand, { limit: 3, offset: 6 })
    expect(noFilters.mock.calls[0]![0]).toBe('/api/v1/platform/withdrawals?limit=3&offset=6')
  })

  it('reads current order and history DTOs at precise routes and checks requested IDs', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok(order)).mockResolvedValueOnce(ok(history))
    const api = createPlatformWithdrawalApi(fetcher)
    await expect(api.read(brand, orderId)).resolves.toEqual(order)
    await expect(api.history(brand, orderId)).resolves.toEqual(history)
    expect(fetcher.mock.calls.map(call => call[0])).toEqual([`/api/v1/platform/withdrawals/${orderId}`, `/api/v1/platform/withdrawals/${orderId}/history`])
    fetcher.mockResolvedValueOnce(ok({ ...order, id: '88888888-8888-4888-8888-888888888888' }))
    await expect(api.read(brand, orderId)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    fetcher.mockResolvedValueOnce(ok({ ...history, order_id: '88888888-8888-4888-8888-888888888888' }))
    await expect(api.history(brand, orderId)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it('rejects foreign brand, member, state, and mismatched pagination in list results', async () => {
    const variants = [
      { ...page, brand_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa' },
      { ...page, items: [{ ...order, brand_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa' }] },
      { ...page, items: [{ ...order, member_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa' }] },
      { ...page, items: [{ ...order, state: 'paid' as WithdrawalState }] },
      { ...page, limit: 10 },
      { ...page, items: Array.from({ length: 21 }, () => order) },
      { ...page, items: [{ ...order, obsolete: true }] },
    ]
    for (const value of variants) {
      const api = createPlatformWithdrawalApi(vi.fn<typeof fetch>().mockResolvedValue(ok(value)))
      await expect(api.list(brand, { state: 'reviewing', memberId: member, limit: 20, offset: 0 })).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    }
  })

  it.each([[401, 'AUTH_SESSION_REVOKED'], [403, 'PERMISSION_DENIED']])('preserves structured %i errors', async (status, code) => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: false, error: { code, message: 'denied' } }), { status }))
    await expect(createPlatformWithdrawalApi(fetcher).list(brand, { limit: 20, offset: 0 })).rejects.toMatchObject({ status, code, message: 'denied' })
  })

  it('requires a selected brand before fetching and exposes GET operations only', async () => {
    const fetcher = vi.fn<typeof fetch>()
    const api = createPlatformWithdrawalApi(fetcher)
    await expect(api.list('', { limit: 20, offset: 0 })).rejects.toBeInstanceOf(PlatformApiError)
    await expect(api.read('', orderId)).rejects.toBeInstanceOf(PlatformApiError)
    await expect(api.history('', orderId)).rejects.toBeInstanceOf(PlatformApiError)
    expect(fetcher).not.toHaveBeenCalled()
    expect(Object.keys(api)).toEqual(['list', 'read', 'history'])
  })
})
