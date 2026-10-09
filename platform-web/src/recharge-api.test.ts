import { describe, expect, it, vi } from 'vitest'
import { PlatformApiError } from './platform-api'
import { createPlatformRechargeApi, type Recharge } from './recharge-api'

const brand = '11111111-1111-4111-8111-111111111111'
const member = '33333333-3333-4333-8333-333333333333'
const at = '2026-10-07T12:00:00Z'
const pending: Recharge = { id: '22222222-2222-4222-8222-222222222222', brand_id: brand, member_id: member, account_id: '55555555-5555-4555-8555-555555555555', points: '9007199254740993', state: 'pending', proof_reference: 'receipt', remark: '', created_by: '66666666-6666-4666-8666-666666666666', version: 1, created_at: at }
const confirmed: Recharge = { ...pending, id: '77777777-7777-4777-8777-777777777777', points: '25', state: 'confirmed', confirmed_by: '88888888-8888-4888-8888-888888888888', confirmed_at: at, ledger_entry_id: '99999999-9999-4999-8999-999999999999', audit_log_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', version: 2 }
const ok = (items: unknown[]) => new Response(JSON.stringify({ success: true, data: { items } }), { status: 200 })

describe('platform recharge API', () => {
  it('uses the exact GET list route and optional member filter', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok([pending]))
    await expect(createPlatformRechargeApi(fetcher).list(brand, member)).resolves.toEqual([pending])
    const [url, init] = fetcher.mock.calls[0]!
    expect(url).toBe(`/api/v1/platform/recharges?limit=51&offset=0&member_id=${member}`)
    expect(init?.method).toBe('GET')
    expect(init?.credentials).toBe('same-origin')
    expect(new Headers(init?.headers).get('X-Brand-ID')).toBe(brand)
    expect(init?.body).toBeUndefined()
    fetcher.mockResolvedValueOnce(ok([]))
    await createPlatformRechargeApi(fetcher).list(brand, '', 12, 5)
    expect(fetcher.mock.calls[1]![0]).toBe('/api/v1/platform/recharges?limit=12&offset=5')
  })

  it('accepts actual pending and confirmed DTO shapes and preserves large decimal points', async () => {
    const api = createPlatformRechargeApi(vi.fn<typeof fetch>().mockResolvedValue(ok([pending, confirmed])))
    const result = await api.list(brand)
    expect(result).toEqual([pending, confirmed])
    expect(result[0]?.points).toBe('9007199254740993')
  })

  it('rejects malformed, cross-brand, over-limit, and mismatched-member results', async () => {
    const invalid = [
      [{ ...pending, extra: true }],
      [{ ...pending, brand_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa' }],
      [{ ...pending, state: 'unknown' }],
      [{ ...pending, points: '9223372036854775808' }],
      [{ ...pending, points: 10 }],
      [{ ...pending, state: 'pending', confirmed_at: at }],
      [pending, pending],
    ]
    for (const records of invalid) {
      const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(records))
      await expect(createPlatformRechargeApi(fetcher).list(brand, member, 1)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    }
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok([{ ...pending, member_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa' }]))
    await expect(createPlatformRechargeApi(fetcher).list(brand, member)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it('preserves backend errors and validates selected brand and page bounds before fetching', async () => {
    const denied = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: false, error: { code: 'PERMISSION_DENIED', message: 'denied' } }), { status: 403 }))
    await expect(createPlatformRechargeApi(denied).list(brand)).rejects.toMatchObject({ status: 403, code: 'PERMISSION_DENIED', message: 'denied' })
    const fetcher = vi.fn<typeof fetch>()
    const api = createPlatformRechargeApi(fetcher)
    await expect(api.list('')).rejects.toBeInstanceOf(PlatformApiError)
    await expect(api.list(brand, '', 101)).rejects.toMatchObject({ code: 'REQUEST_INVALID' })
    expect(fetcher).not.toHaveBeenCalled()
    expect(Object.keys(api)).toEqual(['list'])
  })
})
