import { describe, expect, it, vi } from 'vitest'
import { createPlatformAccessApi, PlatformAccessApiError } from './access-api'

const brand = '123e4567-e89b-12d3-a456-426614174000'
const brand2 = '123e4567-e89b-12d3-a456-426614174001'
const accountId = '123e4567-e89b-12d3-a456-426614174002'
const roleId = '123e4567-e89b-12d3-a456-426614174003'
const auditId = '123e4567-e89b-12d3-a456-426614174004'
const key = 'access-api.test:0001'
const account = { id: accountId, username: 'ops_user', status: 'active', version: 2, super_admin: false, brand_ids: [brand, brand2], role_ids: [roleId], role_codes: ['ops'], audit_log_id: accountId }
const role = { id: roleId, brand_id: brand, code: 'ops', name: 'Operations', status: 'disabled', version: 3, is_bootstrap: false, permissions: ['user.view.brand', 'audit.view.platform'] }
const platformAccount = { id: accountId, username: 'platform_ops', status: 'active', version: 2, super_admin: true, brand_ids: [], role_ids: [roleId], role_codes: ['platform_ops'] }
const platformRole = { id: roleId, brand_id: '', code: 'platform_ops', name: 'Platform Operations', status: 'active', version: 3, is_bootstrap: false, permissions: ['admin.view.platform'] }
function reply(data: unknown, status = 200) { return new Response(JSON.stringify({ success: true, data }), { status, headers: { 'Content-Type': 'application/json' } }) }

describe('platform access API', () => {
  it('accepts a genuine disabled platform account update receipt', async () => {
    const disabled = { ...platformAccount, status: 'disabled', version: 3, audit_log_id: auditId };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(reply(disabled));
    await expect(createPlatformAccessApi(fetcher).updatePlatformAccount(accountId, { version: 2, status: 'disabled', role_ids: [roleId], reason: 'Disable test operator' }, key)).resolves.toEqual(disabled);
  });
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

  it('reads global accounts and roles without a brand header and validates their global scope', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(reply({ items: [platformAccount] })).mockResolvedValueOnce(reply({ items: [platformRole] }))
    const api = createPlatformAccessApi(fetcher)
    await expect(api.platformAccounts()).resolves.toEqual([platformAccount])
    await expect(api.platformRoles(20, 40)).resolves.toEqual([platformRole])
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
      '/api/v1/platform/platform-accounts?limit=51&offset=0',
      '/api/v1/platform/platform-roles?limit=20&offset=40',
    ])
    for (const [, init] of fetcher.mock.calls) {
      expect(init).toMatchObject({ method: 'GET', credentials: 'same-origin' })
      expect(new Headers(init?.headers).has('X-Brand-ID')).toBe(false)
    }
    await expect(createPlatformAccessApi(async () => reply({ items: [{ ...platformAccount, brand_ids: [brand] }] })).platformAccounts()).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformAccessApi(async () => reply({ items: [{ ...platformRole, permissions: ['user.view.brand'] }] })).platformRoles()).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it('writes brand accounts and roles with JSON, explicit brand scope, and the supplied key', async () => {
    const createdAccount = { ...account, version: 1, brand_ids: [brand], audit_log_id: auditId }
    const updatedAccount = { ...account, version: 3, audit_log_id: auditId }
    const createdRole = { ...role, status: 'active', version: 1, permissions: ['user.view.brand'], audit_log_id: auditId }
    const updatedRole = { ...role, version: 4, permissions: ['user.view.brand'], audit_log_id: auditId }
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(reply(createdAccount, 201))
      .mockResolvedValueOnce(reply(updatedAccount))
      .mockResolvedValueOnce(reply({ audit_log_id: auditId }))
      .mockResolvedValueOnce(reply(createdRole, 201))
      .mockResolvedValueOnce(reply(updatedRole))
    const api = createPlatformAccessApi(fetcher)
    const createAccountBody = { username: 'new_ops', password: 'a sufficiently long password', role_ids: [roleId], reason: 'Provision operations account' }
    const updateAccountBody = { version: 2, status: 'disabled' as const, role_ids: [roleId], reason: 'Disable operations account' }
    const createRoleBody = { code: 'new_ops', name: 'New Operations', status: 'active' as const, permissions: ['user.view.brand'], reason: 'Add operations role' }
    const updateRoleBody = { version: 3, name: 'Operations Updated', status: 'active' as const, permissions: ['user.view.brand'], reason: 'Update operations role' }
    await expect(api.createAccount(brand, createAccountBody, key)).resolves.toEqual(createdAccount)
    await expect(api.updateAccount(brand, accountId, updateAccountBody, key)).resolves.toEqual(updatedAccount)
    await expect(api.resetPassword(brand, accountId, { version: 2, password: 'another sufficiently long password', reason: 'Reset operations password' }, key)).resolves.toEqual({ audit_log_id: auditId })
    await expect(api.createRole(brand, createRoleBody, key)).resolves.toEqual(createdRole)
    await expect(api.updateRole(brand, roleId, updateRoleBody, key)).resolves.toEqual(updatedRole)
    expect(fetcher.mock.calls.map(([url, init]) => [url, init?.method])).toEqual([
      ['/api/v1/platform/accounts', 'POST'],
      [`/api/v1/platform/accounts/${accountId}`, 'PATCH'],
      [`/api/v1/platform/accounts/${accountId}/reset-password`, 'POST'],
      ['/api/v1/platform/roles', 'POST'],
      [`/api/v1/platform/roles/${roleId}`, 'PATCH'],
    ])
    for (const [, init] of fetcher.mock.calls) {
      const headers = new Headers(init?.headers)
      expect(init?.credentials).toBe('same-origin')
      expect(headers.get('X-Brand-ID')).toBe(brand)
      expect(headers.get('Content-Type')).toBe('application/json')
      expect(headers.get('Idempotency-Key')).toBe(key)
    }
    expect(JSON.parse(String(fetcher.mock.calls[0]?.[1]?.body))).toEqual(createAccountBody)
    expect(JSON.parse(String(fetcher.mock.calls[1]?.[1]?.body))).toEqual(updateAccountBody)
    expect(JSON.parse(String(fetcher.mock.calls[4]?.[1]?.body))).toEqual(updateRoleBody)
  })

  it('writes platform accounts without a brand header and requires audit receipts', async () => {
    const created = { ...platformAccount, version: 1, audit_log_id: auditId }
    const updated = { ...platformAccount, version: 3, audit_log_id: auditId }
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(reply(created, 201)).mockResolvedValueOnce(reply(updated)).mockResolvedValueOnce(reply({ audit_log_id: auditId }))
    const api = createPlatformAccessApi(fetcher)
    const createBody = { username: 'platform_new', password: 'a sufficiently long password', role_ids: [roleId], reason: 'Create platform operator' }
    const updateBody = { version: 2, status: 'disabled' as const, role_ids: [roleId], reason: 'Disable platform operator' }
    await expect(api.createPlatformAccount(createBody, key)).resolves.toEqual(created)
    await expect(api.updatePlatformAccount(accountId, updateBody, key)).resolves.toEqual(updated)
    await expect(api.resetPlatformPassword(accountId, { version: 2, password: 'another sufficiently long password', reason: 'Reset platform password' }, key)).resolves.toEqual({ audit_log_id: auditId })
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
      '/api/v1/platform/platform-accounts',
      `/api/v1/platform/platform-accounts/${accountId}`,
      `/api/v1/platform/platform-accounts/${accountId}/reset-password`,
    ])
    for (const [, init] of fetcher.mock.calls) {
      const headers = new Headers(init?.headers)
      expect(headers.has('X-Brand-ID')).toBe(false)
      expect(headers.get('Idempotency-Key')).toBe(key)
      expect(headers.get('Content-Type')).toBe('application/json')
      expect(init?.credentials).toBe('same-origin')
    }
    await expect(createPlatformAccessApi(async () => reply({ ...created, audit_log_id: undefined }, 201)).createPlatformAccount(createBody, key)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it('rejects mismatched write statuses and malformed successful responses while preserving HTTP failures', async () => {
    const createBody = { username: 'new_ops', password: 'a sufficiently long password', role_ids: [roleId], reason: 'Create operations account' }
    const apiFor = (fetcher: typeof fetch) => createPlatformAccessApi(fetcher)
    await expect(apiFor(async () => reply({ ...account, version: 1, audit_log_id: auditId })).createAccount(brand, createBody, key)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(apiFor(async () => reply({ ...account, version: 2, audit_log_id: auditId }, 201)).updateAccount(brand, accountId, { version: 2, status: 'active', role_ids: [roleId], reason: 'Update account' }, key)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(apiFor(async () => reply({})).resetPassword(brand, accountId, { version: 2, password: 'another sufficiently long password', reason: 'Reset password' }, key)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(apiFor(async () => { throw new Error('offline') }).platformAccounts()).rejects.toMatchObject({ code: 'NETWORK_ERROR', status: 0 })
    await expect(apiFor(async () => new Response(JSON.stringify({ success: false, error: { code: 'TEMPORARY_FAILURE', message: 'Try again' } }), { status: 503 })).platformAccounts()).rejects.toMatchObject({ code: 'TEMPORARY_FAILURE', status: 503, message: 'Try again' })
    await expect(apiFor(async () => new Response('not json', { status: 200 })).platformAccounts()).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it('validates write keys, versions, role scope, and credentials before sending', async () => {
    const fetcher = vi.fn(async () => reply({}))
    const api = createPlatformAccessApi(fetcher)
    const validCreate = { username: 'new_ops', password: 'a sufficiently long password', role_ids: [roleId], reason: 'Create operations account' }
    await expect(api.createAccount(brand, validCreate, 'short')).rejects.toMatchObject({ code: 'REQUEST_INVALID' })
    await expect(api.updateAccount(brand, accountId, { version: 0, status: 'active', role_ids: [roleId], reason: 'Update account' }, key)).rejects.toMatchObject({ code: 'REQUEST_INVALID' })
    await expect(api.createRole(brand, { code: 'brand_ops', name: 'Brand Operations', status: 'active', permissions: ['admin.view.platform'], reason: 'Wrong permission scope' }, key)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(api.updateRole(brand, roleId, { version: 3, name: 'Operations', status: 'active', permissions: ['admin.view.platform'], reason: 'Wrong permission scope' }, key)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    expect(fetcher).not.toHaveBeenCalled()
  })
})
