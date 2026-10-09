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

export function createPlatformAccessApi(fetcher: typeof fetch = fetch) {
  async function get(path: string, brand: string): Promise<unknown> {
    let response: Response
    try {
      response = await fetcher(`${BASE}${path}`, { method: 'GET', credentials: 'same-origin', headers: { Accept: 'application/json', 'X-Brand-ID': brand } })
    } catch (cause) {
      throw new PlatformAccessApiError(cause instanceof Error ? cause.message : 'Network request failed', 0, 'NETWORK_ERROR')
    }
    let envelope: unknown
    try { envelope = await response.json() } catch {
      if (!response.ok) throw new PlatformAccessApiError(`Request failed (${response.status})`, response.status)
      invalid()
    }
    if (!response.ok || !isObj(envelope) || envelope.success !== true || envelope.data === undefined) {
      const error = isObj(envelope) && isObj(envelope.error) ? envelope.error : undefined
      throw new PlatformAccessApiError(isText(error?.message) ? error.message : `Request failed (${response.status})`, response.status, isText(error?.code) ? error.code : undefined)
    }
    if (response.status !== 200) invalid()
    return envelope.data
  }
  return {
    async accounts(brand: string, limit = 51, offset = 0): Promise<AdminRecord[]> {
      validBrand(brand); validPage(limit, offset)
      const data = await get(`/accounts?limit=${limit}&offset=${offset}`, brand)
      if (!isObj(data) || !Array.isArray(data.items) || !data.items.every(item => isAdmin(item, brand)) || data.items.length > limit) invalid()
      return data.items
    },
    async roles(brand: string, limit = 51, offset = 0): Promise<RoleRecord[]> {
      validBrand(brand); validPage(limit, offset)
      const data = await get(`/roles?limit=${limit}&offset=${offset}`, brand)
      if (!isObj(data) || !Array.isArray(data.items) || !data.items.every(item => isRole(item, brand)) || data.items.length > limit) invalid()
      return data.items
    },
    async permissions(brand: string): Promise<string[]> {
      validBrand(brand)
      const data = await get('/permissions', brand)
      if (!isObj(data) || !Array.isArray(data.items) || !data.items.every(value => isText(value) && MACHINE_KEY.test(value) && value.endsWith('.brand'))) invalid()
      return data.items
    },
  }
}
