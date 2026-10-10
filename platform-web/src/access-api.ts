import { PlatformApiError } from './platform-api'

export interface AdminRecord {
  id: string
  username: string
  status: 'active' | 'disabled'
  version: number
  super_admin: boolean
  brand_ids: string[]
  role_ids: string[]
  role_codes: string[]
  audit_log_id?: string
}

export interface RoleRecord {
  id: string
  brand_id: string
  code: string
  name: string
  status: 'active' | 'disabled'
  version: number
  is_bootstrap: boolean
  permissions: string[]
  audit_log_id?: string
}

export interface AccountCreateBody { username: string; password: string; role_ids: string[]; reason: string }
export interface AccountUpdateBody { version: number; status: 'active' | 'disabled'; role_ids: string[]; reason: string }
export interface PasswordResetBody { version: number; password: string; reason: string }
export interface RoleCreateBody { code: string; name: string; status: 'active' | 'disabled'; permissions: string[]; reason: string }
export interface RoleUpdateBody { version: number; name: string; status: 'active' | 'disabled'; permissions: string[]; reason: string }

export class PlatformAccessApiError extends PlatformApiError {
  constructor(message: string, status: number, code?: string) {
    super(message, status, code)
    this.name = 'PlatformAccessApiError'
  }
}

type Obj = Record<string, unknown>
const BASE = '/api/v1/platform'
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
const MACHINE_KEY = /^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*\.(brand|platform)$/
const IDEMPOTENCY_KEY = /^[a-zA-Z0-9_.:-]{8,128}$/
const isObj = (v: unknown): v is Obj => Boolean(v && typeof v === 'object' && !Array.isArray(v))
const isText = (v: unknown): v is string => typeof v === 'string'
const isUUID = (v: unknown): v is string => isText(v) && UUID.test(v)
const isKeys = (v: unknown): v is string[] => Array.isArray(v) && v.every(isText)
const isMachineKeys = (v: unknown): v is string[] => Array.isArray(v) && v.every(v => isText(v) && MACHINE_KEY.test(v))
function invalid(): never { throw new PlatformAccessApiError('Invalid server response', 0, 'INVALID_RESPONSE') }
function validPage(limit: number, offset: number) {
  if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000) {
    throw new PlatformAccessApiError('Invalid pagination parameters', 400, 'REQUEST_INVALID')
  }
}
function validBrand(brand: string) {
  if (!UUID.test(brand)) throw new PlatformAccessApiError('A valid brand must be selected', 400, 'BRAND_REQUIRED')
}
function isAdmin(v: unknown, brand: string): v is AdminRecord {
  return isObj(v) && isUUID(v.id) && isText(v.username) && (v.status === 'active' || v.status === 'disabled') &&
    typeof v.version === 'number' && Number.isSafeInteger(v.version) && v.version > 0 && typeof v.super_admin === 'boolean' &&
    Array.isArray(v.brand_ids) && v.brand_ids.every(isUUID) && v.brand_ids.includes(brand) &&
    Array.isArray(v.role_ids) && v.role_ids.every(isUUID) && isKeys(v.role_codes) && v.role_ids.length === v.role_codes.length &&
    (v.audit_log_id === undefined || isUUID(v.audit_log_id))
}
function isRole(v: unknown, brand: string): v is RoleRecord {
  return isObj(v) && isUUID(v.id) && v.brand_id === brand && isText(v.code) && isText(v.name) &&
    (v.status === 'active' || v.status === 'disabled') && typeof v.version === 'number' && Number.isSafeInteger(v.version) && v.version > 0 &&
    typeof v.is_bootstrap === 'boolean' && isMachineKeys(v.permissions) && (v.audit_log_id === undefined || isUUID(v.audit_log_id))
}
function isPlatformAdmin(v: unknown): v is AdminRecord {
  return isObj(v) && isUUID(v.id) && isText(v.username) && (v.status === 'active' || v.status === 'disabled') &&
    typeof v.version === 'number' && Number.isSafeInteger(v.version) && v.version > 0 && v.super_admin === true &&
    Array.isArray(v.brand_ids) && v.brand_ids.length === 0 && Array.isArray(v.role_ids) && v.role_ids.every(isUUID) &&
    isKeys(v.role_codes) && v.role_ids.length === v.role_codes.length && (v.audit_log_id === undefined || isUUID(v.audit_log_id))
}
function isPlatformRole(v: unknown): v is RoleRecord {
  return isObj(v) && isUUID(v.id) && v.brand_id === '' && isText(v.code) && isText(v.name) &&
    (v.status === 'active' || v.status === 'disabled') && typeof v.version === 'number' && Number.isSafeInteger(v.version) && v.version > 0 &&
    typeof v.is_bootstrap === 'boolean' && isMachineKeys(v.permissions) && v.permissions.every(permission => permission.endsWith('.platform')) &&
    (v.audit_log_id === undefined || isUUID(v.audit_log_id))
}
function validKey(key: string) {
  if (!IDEMPOTENCY_KEY.test(key)) throw new PlatformAccessApiError('A valid idempotency key is required', 400, 'REQUEST_INVALID')
}
function validReason(reason: string) {
  if (typeof reason !== 'string' || !reason.trim() || new TextEncoder().encode(reason).length > 500) {
    throw new PlatformAccessApiError('A valid reason is required', 400, 'REQUEST_INVALID')
  }
}
function validVersion(version: number) {
  if (!Number.isSafeInteger(version) || version < 1 || version >= Number.MAX_SAFE_INTEGER) {
    throw new PlatformAccessApiError('A valid version is required', 400, 'REQUEST_INVALID')
  }
}
function validRoleIds(roleIds: string[]) {
  if (!Array.isArray(roleIds) || roleIds.length < 1 || roleIds.length > 100 || !roleIds.every(isUUID)) {
    throw new PlatformAccessApiError('At least one valid role is required', 400, 'REQUEST_INVALID')
  }
}
function validPassword(password: string) {
  if (typeof password !== 'string') {
    throw new PlatformAccessApiError('Password must be between 16 and 128 UTF-8 bytes', 400, 'REQUEST_INVALID')
  }
  const bytes = new TextEncoder().encode(password).length
  if (bytes < 16 || bytes > 128) {
    throw new PlatformAccessApiError('Password must be between 16 and 128 UTF-8 bytes', 400, 'REQUEST_INVALID')
  }
}

export function createPlatformAccessApi(fetcher: typeof fetch = fetch) {
  async function request(path: string, options: { method?: 'GET' | 'POST' | 'PATCH'; brand?: string; body?: unknown; key?: string; expectedStatus?: number } = {}): Promise<unknown> {
    const headers = new Headers({ Accept: 'application/json' })
    if (options.brand) headers.set('X-Brand-ID', options.brand)
    if (options.body !== undefined) headers.set('Content-Type', 'application/json')
    if (options.key !== undefined) headers.set('Idempotency-Key', options.key)
    let response: Response
    try {
      response = await fetcher(`${BASE}${path}`, {
        method: options.method ?? 'GET', credentials: 'same-origin', headers,
        ...(options.body === undefined ? {} : { body: JSON.stringify(options.body) }),
      })
    } catch (cause) {
      throw new PlatformAccessApiError(cause instanceof Error ? cause.message : 'Network request failed', 0, 'NETWORK_ERROR')
    }
    let envelope: unknown
    try { envelope = await response.json() } catch {
      if (!response.ok) throw new PlatformAccessApiError(`Request failed (${response.status})`, response.status)
      invalid()
    }
    if (!response.ok) {
      const error = isObj(envelope) && isObj(envelope.error) ? envelope.error : undefined
      throw new PlatformAccessApiError(isText(error?.message) ? error.message : `Request failed (${response.status})`, response.status, isText(error?.code) ? error.code : undefined)
    }
    if (!isObj(envelope) || envelope.success !== true || envelope.data === undefined) invalid()
    if (response.status !== (options.expectedStatus ?? 200)) invalid()
    return envelope.data
  }
  function writeOptions(brand: string | undefined, body: unknown, key: string, expectedStatus = 200) {
    if (brand !== undefined) validBrand(brand)
    validKey(key)
    return { method: 'POST' as const, ...(brand === undefined ? {} : { brand }), body, key, expectedStatus }
  }
  function auditReceipt(data: unknown): { audit_log_id: string } {
    if (!isObj(data) || !isUUID(data.audit_log_id)) invalid()
    return { audit_log_id: data.audit_log_id }
  }
  function adminReceipt(data: unknown, brand: string | undefined, expectedVersion: number): AdminRecord {
    const valid = brand === undefined ? isPlatformAdmin(data) : isAdmin(data, brand)
    if (!valid || !isObj(data) || !isUUID(data.audit_log_id) || data.version !== expectedVersion) invalid()
    if (brand !== undefined && data.super_admin !== false) invalid()
    return data as unknown as AdminRecord
  }
  function roleReceipt(data: unknown, brand: string, expectedVersion: number): RoleRecord {
    if (!isRole(data, brand) || !isObj(data) || !isUUID(data.audit_log_id) || data.version !== expectedVersion) invalid()
    return data as unknown as RoleRecord
  }
  return {
    async accounts(brand: string, limit = 51, offset = 0): Promise<AdminRecord[]> {
      validBrand(brand); validPage(limit, offset)
      const data = await request(`/accounts?limit=${limit}&offset=${offset}`, { brand })
      if (!isObj(data) || !Array.isArray(data.items) || !data.items.every(item => isAdmin(item, brand)) || data.items.length > limit) invalid()
      return data.items
    },
    async roles(brand: string, limit = 51, offset = 0): Promise<RoleRecord[]> {
      validBrand(brand); validPage(limit, offset)
      const data = await request(`/roles?limit=${limit}&offset=${offset}`, { brand })
      if (!isObj(data) || !Array.isArray(data.items) || !data.items.every(item => isRole(item, brand)) || data.items.length > limit) invalid()
      return data.items
    },
    async permissions(brand: string): Promise<string[]> {
      validBrand(brand)
      const data = await request('/permissions', { brand })
      if (!isObj(data) || !Array.isArray(data.items) || !data.items.every(value => isText(value) && MACHINE_KEY.test(value) && value.endsWith('.brand'))) invalid()
      return data.items
    },
    async platformAccounts(limit = 51, offset = 0): Promise<AdminRecord[]> {
      validPage(limit, offset)
      const data = await request(`/platform-accounts?limit=${limit}&offset=${offset}`)
      if (!isObj(data) || !Array.isArray(data.items) || !data.items.every(isPlatformAdmin) || data.items.length > limit) invalid()
      return data.items
    },
    async platformRoles(limit = 51, offset = 0): Promise<RoleRecord[]> {
      validPage(limit, offset)
      const data = await request(`/platform-roles?limit=${limit}&offset=${offset}`)
      if (!isObj(data) || !Array.isArray(data.items) || !data.items.every(isPlatformRole) || data.items.length > limit) invalid()
      return data.items
    },
    async createAccount(brand: string, body: AccountCreateBody, key: string): Promise<AdminRecord> {
      validBrand(brand); validRoleIds(body.role_ids); validPassword(body.password); validReason(body.reason)
      if (!/^[a-z][a-z0-9_]{2,31}$/.test(body.username)) throw new PlatformAccessApiError('A valid username is required', 400, 'REQUEST_INVALID')
      return adminReceipt(await request('/accounts', writeOptions(brand, body, key, 201)), brand, 1)
    },
    async updateAccount(brand: string, id: string, body: AccountUpdateBody, key: string): Promise<AdminRecord> {
      validBrand(brand); validVersion(body.version); validRoleIds(body.role_ids); validReason(body.reason)
      if (!isUUID(id) || (body.status !== 'active' && body.status !== 'disabled')) invalid()
      return adminReceipt(await request(`/accounts/${id}`, { ...writeOptions(brand, body, key), method: 'PATCH' }), brand, body.version + 1)
    },
    async resetPassword(brand: string, id: string, body: PasswordResetBody, key: string): Promise<{ audit_log_id: string }> {
      validBrand(brand); validVersion(body.version); validPassword(body.password); validReason(body.reason)
      if (!isUUID(id)) invalid()
      return auditReceipt(await request(`/accounts/${id}/reset-password`, writeOptions(brand, body, key)))
    },
    async createPlatformAccount(body: AccountCreateBody, key: string): Promise<AdminRecord> {
      validRoleIds(body.role_ids); validPassword(body.password); validReason(body.reason)
      if (!/^[a-z][a-z0-9_]{2,31}$/.test(body.username)) throw new PlatformAccessApiError('A valid username is required', 400, 'REQUEST_INVALID')
      return adminReceipt(await request('/platform-accounts', writeOptions(undefined, body, key, 201)), undefined, 1)
    },
    async updatePlatformAccount(id: string, body: AccountUpdateBody, key: string): Promise<AdminRecord> {
      validVersion(body.version); validRoleIds(body.role_ids); validReason(body.reason)
      if (!isUUID(id) || (body.status !== 'active' && body.status !== 'disabled')) invalid()
      return adminReceipt(await request(`/platform-accounts/${id}`, { ...writeOptions(undefined, body, key), method: 'PATCH' }), undefined, body.version + 1)
    },
    async resetPlatformPassword(id: string, body: PasswordResetBody, key: string): Promise<{ audit_log_id: string }> {
      validVersion(body.version); validPassword(body.password); validReason(body.reason)
      if (!isUUID(id)) invalid()
      return auditReceipt(await request(`/platform-accounts/${id}/reset-password`, writeOptions(undefined, body, key)))
    },
    async createRole(brand: string, body: RoleCreateBody, key: string): Promise<RoleRecord> {
      validBrand(brand); validReason(body.reason)
      if (!/^[a-z][a-z0-9_]{2,47}$/.test(body.code) || !body.name.trim() || body.name.length > 120 ||
        !Array.isArray(body.permissions) || !body.permissions.every(value => isText(value) && value.endsWith('.brand') && MACHINE_KEY.test(value))) invalid()
      return roleReceipt(await request('/roles', writeOptions(brand, body, key, 201)), brand, 1)
    },
    async updateRole(brand: string, id: string, body: RoleUpdateBody, key: string): Promise<RoleRecord> {
      validBrand(brand); validVersion(body.version); validReason(body.reason)
      if (!isUUID(id) || !body.name.trim() || body.name.length > 120 || (body.status !== 'active' && body.status !== 'disabled') ||
        !Array.isArray(body.permissions) || !body.permissions.every(value => isText(value) && value.endsWith('.brand') && MACHINE_KEY.test(value))) invalid()
      return roleReceipt(await request(`/roles/${id}`, { ...writeOptions(brand, body, key), method: 'PATCH' }), brand, body.version + 1)
    },
  }
}
