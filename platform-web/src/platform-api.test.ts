import { describe, expect, it, vi } from 'vitest'
import { createPlatformApi, freezeBrandCreateRequest, PlatformApiError, type BrandCreateInput } from './platform-api'

function reply(data: unknown, status = 200) {
  return new Response(JSON.stringify({ success: true, data }), { status, headers: { 'Content-Type': 'application/json' } })
}
const account = { id: 'admin-1', super_admin: true }
const brand = { id: 'b-1', code: 'luma', name: 'Luma', status: 'active' }
const validInput: BrandCreateInput = { code: 'northstar', name: 'Northstar Shop', default_locale: 'en', timezone: 'Asia/Singapore', reason: 'Launch a new market' }
const member = { id: 'm1', global_user_id: 'g1', username: 'member_one', phone: '+6500000000', display_name: 'Member One', notes: '', status: 'normal', joined_at: '2026-10-09T01:02:03Z', brand_id: 'b-1', tags: ['new'] }

describe('platform API boundary', () => {
  it('uses only the platform prefix and same-origin cookie credentials', async () => {
    const fetcher = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => reply({ account }))
    await createPlatformApi(fetcher).me()
    expect(fetcher.mock.calls[0][0]).toBe('/api/v1/platform/me')
    expect(fetcher.mock.calls[0][1]).toMatchObject({ credentials: 'same-origin', method: 'GET' })
    expect(JSON.stringify(fetcher.mock.calls.map(call => call[0]))).not.toContain('/api/v1/admin')
  })

  it('rejects accounts unless the current DTO explicitly says super_admin true', async () => {
    for (const wrong of [{ id: 'ordinary', super_admin: false }, { id: 'legacy' }, { id: 4, super_admin: true }]) {
      const api = createPlatformApi(async () => reply({ account: wrong }))
      await expect(api.me()).rejects.toMatchObject({ code: 'PLATFORM_ADMIN_REQUIRED' })
    }
  })

  it('preserves structured server errors such as HTTP 400', async () => {
    const fetcher = async () => new Response(JSON.stringify({ success: false, error: { code: 'BRAND_CODE_TAKEN', message: 'Code is already in use' } }), { status: 400 })
    await expect(createPlatformApi(fetcher).brands()).rejects.toMatchObject({ status: 400, code: 'BRAND_CODE_TAKEN', message: 'Code is already in use' })
  })

  it('does not parse legacy string-form errors', async () => {
    const fetcher = async () => new Response(JSON.stringify({ success: false, error: 'legacy error text' }), { status: 400 })
    await expect(createPlatformApi(fetcher).brands()).rejects.toMatchObject({ status: 400, message: 'Request failed (400)' })
  })

  it('validates list DTOs and rejects malformed current-format data', async () => {
    const api = createPlatformApi(async () => reply({ items: [brand] }))
    await expect(api.brands()).resolves.toEqual([brand])
    await expect(createPlatformApi(async () => reply({ items: [{ id: 'b-1', name: 'Missing fields' }] })).brands()).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformApi(async () => reply({ items: [{ ...brand, timezone: 'UTC' }] })).brands()).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformApi(async () => reply({ items: [{ id: 'u1', username: 'staff', status: 'active', role_codes: ['operator'] }] })).users('b-1')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformApi(async () => reply({ items: [member] })).users('b-1')).resolves.toEqual([member])
    await expect(createPlatformApi(async () => reply({ items: [{ ...member, role_codes: ['fictional'] }] })).users('b-1')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it('requires a selected brand and sends X-Brand-ID for member reads', async () => {
    const fetcher = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => reply({ items: [member] }))
    await expect(createPlatformApi(fetcher).users('b-1')).resolves.toEqual([member])
    expect(fetcher.mock.calls[0][0]).toBe('/api/v1/platform/users?limit=100&offset=0')
    expect(new Headers(fetcher.mock.calls[0][1]?.headers).get('X-Brand-ID')).toBe('b-1')
    await expect(createPlatformApi(fetcher).users('')).rejects.toMatchObject({ code: 'BRAND_REQUIRED' })
  })

  it('requires a selected brand and sends X-Brand-ID for audit reads', async () => {
    const audit = { id: 'a1', brand_id: 'b-1', action: 'member.view', actor_type: 'admin', actor_id: 'admin-1', resource_type: 'member', resource_id: 'm1', reason: '', request_id: 'request-1', created_at: '2026-10-09T01:02:03Z', ip_address: '', before_json: null, after_json: null }
    const fetcher = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => reply({ items: [audit] }))
    await expect(createPlatformApi(fetcher).audit('b-1')).resolves.toEqual([audit])
    expect(fetcher.mock.calls[0][0]).toBe('/api/v1/platform/audit?limit=100&offset=0')
    expect(new Headers(fetcher.mock.calls[0][1]?.headers).get('X-Brand-ID')).toBe('b-1')
  })

  it('sends the exact frozen creation body and idempotency key', async () => {
    const fetcher = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => reply({ id: 'f3cf04e4-9c31-4b6f-9a1d-8a741a6c6012', code: validInput.code, name: validInput.name, status: 'paused', default_locale: validInput.default_locale, timezone: validInput.timezone, version: 1, created_at: '2026-10-09T01:02:03Z', audit_log_id: '8b48d72e-a737-4c11-a54c-c4d96cbcf227' }, 201))
    const key = crypto.randomUUID()
    await createPlatformApi(fetcher).createBrand(validInput, key)
    const [path, options] = fetcher.mock.calls[0]
    expect(path).toBe('/api/v1/platform/brands')
    expect(options).toMatchObject({ method: 'POST', credentials: 'same-origin' })
    expect(new Headers(options?.headers).get('Idempotency-Key')).toBe(key)
    expect(JSON.parse(String(options?.body))).toEqual({ code: 'northstar', name: 'Northstar Shop', default_locale: 'en', timezone: 'Asia/Singapore', reason: 'Launch a new market' })
  })

  it('rejects receipts that do not match the submitted request', async () => {
    const fetcher = async () => reply({ id: 'f3cf04e4-9c31-4b6f-9a1d-8a741a6c6012', code: 'different', name: validInput.name, status: 'paused', default_locale: validInput.default_locale, timezone: validInput.timezone, version: 1, created_at: '2026-10-09T01:02:03Z', audit_log_id: '8b48d72e-a737-4c11-a54c-c4d96cbcf227' }, 201)
    await expect(createPlatformApi(fetcher).createBrand(validInput, crypto.randomUUID())).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it('exposes unknown network failures without retrying automatically', async () => {
    const fetcher = vi.fn(async () => { throw new TypeError('offline') })
    await expect(createPlatformApi(fetcher).createBrand(validInput, crypto.randomUUID())).rejects.toMatchObject({ network: true, code: 'NETWORK_ERROR' })
    expect(fetcher).toHaveBeenCalledTimes(1)
  })

  it('freezes unknown-outcome retry content and key as immutable state', () => {
    const request = freezeBrandCreateRequest(validInput, 'same-request-key')
    expect(Object.isFrozen(request)).toBe(true)
    expect(Object.isFrozen(request.body)).toBe(true)
    expect(request).toEqual({ body: validInput, key: 'same-request-key' })
  })

  it('has no user mutation methods or business-operation route names', async () => {
    const api = createPlatformApi()
    expect(Object.keys(api).sort()).toEqual(['audit', 'brands', 'createBrand', 'login', 'logout', 'me', 'users'])
    expect(PlatformApiError).toBeDefined()
  })
})
