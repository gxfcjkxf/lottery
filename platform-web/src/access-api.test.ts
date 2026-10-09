import { describe, expect, it, vi } from 'vitest'
import { createPlatformAccessApi, PlatformAccessApiError } from './access-api'

const brand = '123e4567-e89b-12d3-a456-426614174000'
const brand2 = '123e4567-e89b-12d3-a456-426614174001'
const accountId = '123e4567-e89b-12d3-a456-426614174002'
const roleId = '123e4567-e89b-12d3-a456-426614174003'
const account = { id: accountId, username: 'ops_user', status: 'active', version: 2, super_admin: false, brand_ids: [brand, brand2], role_ids: [roleId], role_codes: ['ops'], audit_log_id: accountId }
const role = { id: roleId, brand_id: brand, code: 'ops', name: 'Operations', status: 'disabled', version: 3, is_bootstrap: false, permissions: ['user.view.brand', 'audit.view.platform'] }
function reply(data: unknown, status = 200) { return new Response(JSON.stringify({ success: true, data }), { status, headers: { 'Content-Type': 'application/json' } }) }

describe('platform access API', () => {
  it('reads accounts and roles with the selected brand, paging, and same-origin credentials', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(reply({ items: [account] })).mockResolvedValueOnce(reply({ items: [role] }))
    const api = createPlatformAccessApi(fetcher)
    await expect(api.accounts(brand)).resolves.toEqual([account])
    await expect(api.roles(brand, 51, 100)).resolves.toEqual([role])
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
      `/api/v1/platform/accounts?limit=51&offset=0`,
      `/api/v1/platform/roles?limit=51&offset=100`,
    ])
    for (const [, init] of fetcher.mock.calls) {
      expect(init).toMatchObject({ method: 'GET', credentials: 'same-origin' })
      expect(new Headers(init?.headers).get('X-Brand-ID')).toBe(brand)
    }
  })

  it('preserves multi-role ID/code pairing and platform keys in provided role grants', async () => {
    const multi = { ...account, role_ids: [roleId, accountId], role_codes: ['ops', 'platform_admin'] }
    const api = createPlatformAccessApi(async () => reply({ items: [multi] }))
    await expect(api.accounts(brand)).resolves.toEqual([multi])
    await expect(createPlatformAccessApi(async () => reply({ items: [role] })).roles(brand)).resolves.toEqual([role])
  })

  it('reads permissions without a query and treats the catalog as server data', async () => {
    const fetcher = vi.fn<typeof fetch>(async () => reply({ items: ['user.view.brand', 'report.view.brand'] }))
    await expect(createPlatformAccessApi(fetcher).permissions(brand)).resolves.toEqual(['user.view.brand', 'report.view.brand'])
    expect(fetcher.mock.calls[0]?.[0]).toBe('/api/v1/platform/permissions')
  })

  it('rejects invalid brands, page bounds, cross-brand records, and mismatched role pairs before or after fetch', async () => {
    const fetcher = vi.fn(async () => reply({ items: [role] }))
    const api = createPlatformAccessApi(fetcher)
    await expect(api.roles('not-a-uuid')).rejects.toMatchObject({ code: 'BRAND_REQUIRED' })
    await expect(api.accounts(brand, 0)).rejects.toMatchObject({ code: 'REQUEST_INVALID' })
    await expect(api.accounts(brand, 51, 1_000_001)).rejects.toMatchObject({ code: 'REQUEST_INVALID' })
    await expect(api.roles(brand2)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformAccessApi(async () => reply({ items: [{ ...account, brand_ids: [brand2] }] })).accounts(brand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformAccessApi(async () => reply({ items: [{ ...account, role_codes: [] }] })).accounts(brand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    expect(fetcher).toHaveBeenCalledTimes(1)
  })

  it('rejects malformed records and propagates HTTP 403 details', async () => {
    await expect(createPlatformAccessApi(async () => reply({ items: [{ ...role, version: Number.MAX_SAFE_INTEGER + 1 }] })).roles(brand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformAccessApi(async () => reply({ items: ['user.view.brand', ''] })).permissions(brand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const forbidden = async () => new Response(JSON.stringify({ success: false, error: { code: 'PERMISSION_DENIED', message: 'Forbidden' } }), { status: 403 })
    await expect(createPlatformAccessApi(forbidden).accounts(brand)).rejects.toMatchObject({ status: 403, code: 'PERMISSION_DENIED', message: 'Forbidden' } satisfies Partial<PlatformAccessApiError>)
    await expect(createPlatformAccessApi(async () => new Response('not json', { status: 200 })).roles(brand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformAccessApi(async () => reply({ items: [role] }, 201)).roles(brand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformAccessApi(async () => reply({ items: ['audit.view.platform'] })).permissions(brand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })
})
