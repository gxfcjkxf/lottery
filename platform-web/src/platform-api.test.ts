import { describe, expect, it, vi } from 'vitest'
import { createPlatformApi, freezeBrandCreateRequest, PlatformApiError, type BrandCreateInput } from './platform-api'

function reply(data: unknown, status = 200) {
  return new Response(JSON.stringify({ success: true, data }), { status, headers: { 'Content-Type': 'application/json' } })
}
const account = { id: 'admin-1', super_admin: true, platform_permissions: ['admin.view.platform', 'admin.write.platform'] }
const brand = { id: 'b-1', code: 'luma', name: 'Luma', status: 'active' }
const validInput: BrandCreateInput = { code: 'northstar', name: 'Northstar Shop', default_locale: 'en', timezone: 'Asia/Singapore', reason: 'Launch a new market' }
const member = { id: 'm1', global_user_id: 'g1', username: 'member_one', phone: '+6500000000', display_name: 'Member One', notes: '', status: 'normal', joined_at: '2026-10-09T01:02:03Z', brand_id: 'b-1', tags: ['new'], online: true, display_points: '9007199254740993' }

describe('platform API boundary', () => {
  it('deletes the global account without a brand header and requires a matching named receipt', async () => {
    const id = '00000000-0000-4000-8000-000000000001', audit = '00000000-0000-4000-8000-000000000002'
    const receipt = { id, account_name: 'test001', audit_log_id: audit }
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(reply(receipt))
    await expect(createPlatformApi(fetcher).deleteMemberAccount(id, 'stable-delete-key')).resolves.toEqual(receipt)
    const [url, init] = fetcher.mock.calls[0]!
    expect(url).toBe(`/api/v1/platform/global-users/${id}/delete`)
    expect(init?.method).toBe('POST')
    expect(new Headers(init?.headers).has('X-Brand-ID')).toBe(false)
    expect(new Headers(init?.headers).get('Idempotency-Key')).toBe('stable-delete-key')
    expect(init?.body).toBe('{}')
    for (const invalid of [{ ...receipt, id: audit }, { ...receipt, account_name: '' }, { audit_log_id: audit }]) {
      await expect(createPlatformApi(async () => reply(invalid)).deleteMemberAccount(id, 'stable-delete-key')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    }
  })
  it('uses explicit member and audit pagination and rejects cross-brand rows', async () => {
    const audit = { id: 'a1', brand_id: 'b-1', action: 'member.view', actor_type: 'admin', actor_id: 'admin-1', actor_username: 'admin', resource_type: 'member', resource_id: 'm1', reason: '', request_id: 'request-1', created_at: '2026-10-09T01:02:03Z', ip_address: '', before_json: null, after_json: null }
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(reply({ items: [member], limit: 51, offset: 50, total_count: '101' }))
      .mockResolvedValueOnce(reply({ items: [audit] }))
    const api = createPlatformApi(fetcher)
    await api.users('b-1', 51, 50)
    await api.audit('b-1', 51, 50)
    expect(fetcher.mock.calls.map(call => call[0])).toEqual(['/api/v1/platform/users?limit=51&offset=50', '/api/v1/platform/audit?limit=51&offset=50'])
    await expect(createPlatformApi(async () => reply({ items: [{ ...member, brand_id: 'b-2' }] })).users('b-1')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformApi(async () => reply({ items: [{ ...audit, brand_id: 'b-2' }] })).audit('b-1')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformApi(async () => reply({ items: [member, member] })).users('b-1', 1)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it('queries all members before pagination and retains exact points and presence', async () => {
    const data = { items: [member], limit: 10, offset: 20, total_count: '101' };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(reply(data));
    const api = createPlatformApi(fetcher);
    await expect(api.users('b-1', 10, 20, '', { query: ' member 100% ', online: 'false' })).resolves.toEqual(data);
    const url = new URL(String(fetcher.mock.calls[0][0]), 'https://example.test');
    expect(Object.fromEntries(url.searchParams)).toEqual({ limit: '10', offset: '20', q: 'member 100%', online: 'false' });
    expect(new Headers(fetcher.mock.calls[0][1]?.headers).get('X-Brand-ID')).toBe('b-1');
    for (const invalid of [{ ...data, total_count: 101 }, { ...data, offset: 0 }, { ...data, items: [{ ...member, display_points: 9007199254740992 }] }, { ...data, items: [{ ...member, online: 'true' }] }, { ...data, items: [{ ...member, brand_id: 'b-2' }] }]) {
      fetcher.mockResolvedValueOnce(reply(invalid));
      await expect(api.users('b-1', 10, 20)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
    }
    fetcher.mockResolvedValueOnce(reply({ ...data, items: [{ ...member, display_points: null }] }));
    expect((await api.users('b-1', 10, 20)).items[0].display_points).toBeNull();
  });

  it('uses only the platform prefix and same-origin cookie credentials', async () => {
    const fetcher = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => reply({ account }))
    await createPlatformApi(fetcher).me()
    expect(fetcher.mock.calls[0][0]).toBe('/api/v1/platform/me')
    expect(fetcher.mock.calls[0][1]).toMatchObject({ credentials: 'same-origin', method: 'GET' })
    expect(JSON.stringify(fetcher.mock.calls.map(call => call[0]))).not.toContain('/api/v1/admin')
  })
  it('rejects a missing platform grant directory rather than inventing write permissions', async () => {
    await expect(createPlatformApi(async () => reply({ account: { id: account.id, super_admin: true } })).me()).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
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

  it('rejects legacy string-form errors as unconfirmed protocol responses', async () => {
    const fetcher = async () => new Response(JSON.stringify({ success: false, error: 'legacy error text' }), { status: 400 })
    await expect(createPlatformApi(fetcher).brands()).rejects.toMatchObject({ status: 502, code: 'INVALID_RESPONSE' })
  })

  it('validates list DTOs and rejects malformed current-format data', async () => {
    const api = createPlatformApi(async () => reply({ items: [brand] }))
    await expect(api.brands()).resolves.toEqual([brand])
    await expect(createPlatformApi(async () => reply({ items: [{ id: 'b-1', name: 'Missing fields' }] })).brands()).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformApi(async () => reply({ items: [{ ...brand, timezone: 'UTC' }] })).brands()).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformApi(async () => reply({ items: [{ id: 'u1', username: 'staff', status: 'active', role_codes: ['operator'] }] })).users('b-1')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformApi(async () => reply({ items: [member], limit: 100, offset: 0, total_count: '1' })).users('b-1')).resolves.toEqual({ items: [member], limit: 100, offset: 0, total_count: '1' })
    await expect(createPlatformApi(async () => reply({ items: [{ ...member, role_codes: ['fictional'] }] })).users('b-1')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it('requires a selected brand and sends X-Brand-ID for member reads', async () => {
    const fetcher = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => reply({ items: [member], limit: 100, offset: 0, total_count: '1' }))
    await expect(createPlatformApi(fetcher).users('b-1')).resolves.toEqual({ items: [member], limit: 100, offset: 0, total_count: '1' })
    expect(fetcher.mock.calls[0][0]).toBe('/api/v1/platform/users?limit=100&offset=0')
    expect(new Headers(fetcher.mock.calls[0][1]?.headers).get('X-Brand-ID')).toBe('b-1')
    await expect(createPlatformApi(fetcher).users('')).rejects.toMatchObject({ code: 'BRAND_REQUIRED' })
  })

  it('requires a selected brand and sends X-Brand-ID for audit reads', async () => {
    const audit = { id: 'a1', brand_id: 'b-1', action: 'member.view', actor_type: 'admin', actor_id: 'admin-1', actor_username: 'admin', resource_type: 'member', resource_id: 'm1', reason: '', request_id: 'request-1', created_at: '2026-10-09T01:02:03Z', ip_address: '', before_json: null, after_json: null }
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

  it('exposes only the intended platform API methods', async () => {
    const api = createPlatformApi()
    expect(Object.keys(api).sort()).toEqual(['audit', 'brands', 'changePassword', 'createBrand', 'deleteBrand', 'deleteMemberAccount', 'login', 'logout', 'me', 'platformSettings', 'presentation', 'updatePlatformSettings', 'users'])
    expect(PlatformApiError).toBeDefined()
  })
  it('reads public branding before authentication and rejects incomplete or private fields', async () => {
    const presentation = { display_name: '智彩网', logo_text: 'Z', logo_url: '' }
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(reply(presentation))
    await expect(createPlatformApi(fetcher).presentation()).resolves.toEqual(presentation)
    expect(fetcher.mock.calls[0]![0]).toBe('/api/v1/platform/presentation')
    for (const invalid of [{ ...presentation, audit_log_id: 'private' }, { logo_text: 'N' }, { ...presentation, display_name: '' }]) {
      await expect(createPlatformApi(async () => reply(invalid)).presentation()).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    }
  })
})
