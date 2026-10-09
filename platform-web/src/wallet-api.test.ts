import { describe, expect, it, vi } from 'vitest'
import { createPlatformWalletApi } from './wallet-api'

const brandId = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'
const otherBrand = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb'
const memberId = 'cccccccc-cccc-4ccc-8ccc-cccccccccccc'
const otherMember = 'dddddddd-dddd-4ddd-8ddd-dddddddddddd'
const accountId = 'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee'
const entryId = 'ffffffff-ffff-4fff-8fff-ffffffffffff'
const timestamp = '2026-10-09T10:00:00Z'

function buckets(value = '0') {
  return Object.fromEntries(['recharge', 'winning', 'gift', 'commission'].map(source => [source, {
    available: value, manual_frozen: '0', system_frozen: '0', withdrawal: '0',
  }]))
}
const amount = '9007199254740993'
const wallet = {
  account_id: accountId, brand_id: brandId, member_id: memberId, version: 1,
  display_points: amount, available_points: amount, frozen_points: '0', withdrawal_points: '0',
  recharge_points: amount, winning_points: '0', gift_points: '0', commission_points: '0',
  manual_frozen_points: '0', system_frozen_points: '0',
  by_source: { ...buckets(), recharge: { available: amount, manual_frozen: '0', system_frozen: '0', withdrawal: '0' } },
}
const entry = {
  id: entryId, brand_id: brandId, account_id: accountId, member_id: memberId, entry_type: 'recharge',
  reference_type: 'recharge', reference_id: entryId, operation_key: 'recharge:fixture',
  before_snapshot: buckets(), delta_snapshot: { ...buckets(), recharge: { available: amount, manual_frozen: '0', system_frozen: '0', withdrawal: '0' } },
  after_snapshot: wallet.by_source, source_allocation: [{ source: 'recharge', state: 'available', points: amount }],
  reason: 'test', actor_type: 'admin', actor_id: accountId, request_id: 'request-1', version: 1, created_at: timestamp,
}
function response(data: unknown, status = 200) {
  return new Response(JSON.stringify({ success: true, data }), { status, headers: { 'Content-Type': 'application/json' } })
}

describe('platform wallet read API', () => {
  it('uses the exact platform GET routes, selected brand header, and same-origin credentials', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(response(wallet)).mockResolvedValueOnce(response({ items: [entry] }))
    const api = createPlatformWalletApi(fetcher)
    expect((await api.wallet(brandId, memberId)).display_points).toBe(amount)
    expect((await api.ledger(brandId, memberId, 25, 10))[0]?.id).toBe(entryId)
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
      `/api/v1/platform/wallets/${memberId}`,
      `/api/v1/platform/wallets/${memberId}/ledger?limit=25&offset=10`,
    ])
    for (const [, init] of fetcher.mock.calls) {
      expect(init?.method).toBe('GET')
      expect(init?.credentials).toBe('same-origin')
      expect(new Headers(init?.headers).get('X-Brand-ID')).toBe(brandId)
    }
  })

  it('preserves precise balances and snapshots as decimal strings', async () => {
    const api = createPlatformWalletApi(vi.fn<typeof fetch>().mockResolvedValueOnce(response(wallet)).mockResolvedValueOnce(response({ items: [entry] })))
    expect((await api.wallet(brandId, memberId)).display_points).toBe('9007199254740993')
    expect((await api.ledger(brandId, memberId))[0]?.after_snapshot.recharge.available).toBe('9007199254740993')
  })

  it('rejects malformed and non-current wallet and ledger DTOs', async () => {
    const badWallets = [{ ...wallet, points_display: amount }, { ...wallet, display_points: 7 }, { ...wallet, by_source: { ...wallet.by_source, legacy: {} } }]
    for (const bad of badWallets) {
      const api = createPlatformWalletApi(vi.fn<typeof fetch>().mockResolvedValue(response(bad)))
      await expect(api.wallet(brandId, memberId)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    }
    const api = createPlatformWalletApi(vi.fn<typeof fetch>().mockResolvedValue(response({ items: [{ ...entry, after_snapshot: { ...entry.after_snapshot, extra: {} } }] })))
    await expect(api.ledger(brandId, memberId)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it('requires selected brand and member before making a request', async () => {
    const fetcher = vi.fn<typeof fetch>()
    const api = createPlatformWalletApi(fetcher)
    await expect(api.wallet('', memberId)).rejects.toMatchObject({ code: 'BRAND_REQUIRED' })
    await expect(api.ledger(brandId, '')).rejects.toMatchObject({ code: 'MEMBER_REQUIRED' })
    expect(fetcher).not.toHaveBeenCalled()
  })

  it('rejects wallet and ledger records from another brand or member', async () => {
    const wrongWallet = createPlatformWalletApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...wallet, brand_id: otherBrand })))
    await expect(wrongWallet.wallet(brandId, memberId)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const wrongMemberWallet = createPlatformWalletApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...wallet, member_id: otherMember })))
    await expect(wrongMemberWallet.wallet(brandId, memberId)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const wrongEntry = createPlatformWalletApi(vi.fn<typeof fetch>().mockResolvedValue(response({ items: [{ ...entry, member_id: otherMember }] })))
    await expect(wrongEntry.ledger(brandId, memberId)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it('preserves structured authentication errors', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: false, error: { code: 'AUTH_SESSION_REVOKED', message: 'Sign in again' } }), { status: 401 }))
    await expect(createPlatformWalletApi(fetcher).wallet(brandId, memberId)).rejects.toMatchObject({ status: 401, code: 'AUTH_SESSION_REVOKED', message: 'Sign in again' })
  })
})
