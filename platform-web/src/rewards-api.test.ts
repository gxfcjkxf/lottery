import { describe, expect, it, vi } from 'vitest'
import { createPlatformRewardsApi } from './rewards-api'

const brandId = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'
const orderId = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb'
const memberId = 'cccccccc-cccc-4ccc-8ccc-cccccccccccc'
const grantEntry = 'dddddddd-dddd-4ddd-8ddd-dddddddddddd'
const actionId = 'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee'
const timestamp = '2026-10-09T10:00:00Z'
const order = {
  id: orderId, brand_id: brandId, member_id: memberId, points: '250', state: 'revoked', version: 3,
  grant_ledger_entry_id: grantEntry, revoke_ledger_entry_id: actionId, creation_audit_log_id: actionId,
  last_audit_log_id: actionId, last_error_code: null, created_by: memberId, reason: 'Campaign award',
  point_policy_version: '4', created_at: timestamp, updated_at: timestamp, revoked_at: timestamp,
}
const action = {
  id: actionId, brand_id: brandId, order_id: orderId, version: 3, operation: 'revoke', state_before: 'granted',
  state_after: 'revoked', actor_id: memberId, reason: 'Duplicate award', audit_log_id: actionId,
  ledger_entry_id: actionId, created_at: timestamp,
}
function response(data: unknown, status = 200) {
  return new Response(JSON.stringify({ success: true, data }), { status, headers: { 'Content-Type': 'application/json' } })
}

describe('platform rewards read API', () => {
  it('reads list, current order, and action history with the selected brand header and GET only', async () => {
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(response({ brand_id: brandId, items: [order], total_count: '1', limit: 100, offset: 0 }))
      .mockResolvedValueOnce(response(order))
      .mockResolvedValueOnce(response({ brand_id: brandId, order_id: orderId, items: [action], total_count: '1', limit: 100, offset: 0 }))
    const api = createPlatformRewardsApi(fetcher)

    expect((await api.list(brandId)).items[0]?.id).toBe(orderId)
    expect((await api.read(brandId, orderId)).version).toBe(3)
    expect((await api.actions(brandId, orderId)).items[0]?.operation).toBe('revoke')
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
      `/api/v1/platform/reward-orders?limit=100&offset=0`,
      `/api/v1/platform/reward-orders/${orderId}`,
      `/api/v1/platform/reward-orders/${orderId}/actions?limit=100&offset=0`,
    ])
    for (const [, init] of fetcher.mock.calls) {
      expect(init?.method ?? 'GET').toBe('GET')
      expect(new Headers(init?.headers).get('X-Brand-ID')).toBe(brandId)
    }
  })

  it('rejects an order response with fields outside the current DTO', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response({ ...order, points_display: '250' }))
    await expect(createPlatformRewardsApi(fetcher).read(brandId, orderId)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it('requires an explicit brand before making a request', async () => {
    const fetcher = vi.fn<typeof fetch>()
    await expect(createPlatformRewardsApi(fetcher).list('')).rejects.toMatchObject({ code: 'BRAND_REQUIRED' })
    expect(fetcher).not.toHaveBeenCalled()
  })
})
