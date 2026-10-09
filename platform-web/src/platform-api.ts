export interface PlatformAccount { id: string; super_admin: true }
export interface PlatformBrand { id: string; code: string; name: string; status: string }
export interface PlatformMember { id: string; global_user_id: string; username: string; phone: string; display_name: string; notes: string; status: string; joined_at: string; brand_id: string; tags: string[] }
export interface PlatformAudit { id: string; brand_id: string | null; action: string; actor_type: string; actor_id: string; resource_type: string; resource_id: string; reason: string; request_id: string; created_at: string; ip_address: string; before_json: unknown; after_json: unknown }
export interface BrandCreateInput { code: string; name: string; default_locale: 'en' | 'zh-CN'; timezone: string; reason: string }
export interface BrandCreateReceipt extends PlatformBrand { status: 'paused'; default_locale: 'en' | 'zh-CN'; timezone: string; version: 1; created_at: string; audit_log_id: string }
export interface FrozenBrandCreateRequest { readonly body: Readonly<BrandCreateInput>; readonly key: string }

export class PlatformApiError extends Error {
  constructor(message: string, readonly status: number, readonly code?: string, readonly network = false) {
    super(message)
    this.name = 'PlatformApiError'
  }
}

type Obj = Record<string, unknown>
const BASE = '/api/v1/platform'
const isObj = (v: unknown): v is Obj => Boolean(v && typeof v === 'object' && !Array.isArray(v))
const text = (v: unknown): v is string => typeof v === 'string'
const stringArray = (v: unknown): v is string[] => Array.isArray(v) && v.every(text)
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
function apiError(message: string, status = 0, code?: string): never { throw new PlatformApiError(message, status, code) }
function validate<T>(value: unknown, test: (value: unknown) => value is T, label: string): T {
  if (!test(value)) apiError(`Invalid ${label} response`, 0, 'INVALID_RESPONSE')
  return value
}
function isBrand(v: unknown): v is PlatformBrand {
  return isObj(v) && exactKeys(v, ['id', 'code', 'name', 'status']) && text(v.id) && text(v.code) && text(v.name) && text(v.status)
}
function isMember(v: unknown): v is PlatformMember {
  return isObj(v) && exactKeys(v, ['id', 'global_user_id', 'username', 'phone', 'display_name', 'notes', 'status', 'joined_at', 'brand_id', 'tags']) && text(v.id) && text(v.global_user_id) && text(v.username) && text(v.phone) && text(v.display_name) && text(v.notes) && text(v.status) && dateTime(v.joined_at) && text(v.brand_id) && stringArray(v.tags)
}
function isAudit(v: unknown): v is PlatformAudit {
  return isObj(v) && exactKeys(v, ['id', 'brand_id', 'action', 'actor_type', 'actor_id', 'resource_type', 'resource_id', 'reason', 'request_id', 'created_at', 'ip_address', 'before_json', 'after_json']) && text(v.id) && (v.brand_id === null || text(v.brand_id)) && text(v.action) && text(v.actor_type) && text(v.actor_id) && text(v.resource_type) && text(v.resource_id) && text(v.reason) && text(v.request_id) && dateTime(v.created_at) && text(v.ip_address) && 'before_json' in v && 'after_json' in v
}
function dateTime(v: unknown): v is string {
  return text(v) && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.test(v) && Number.isFinite(Date.parse(v))
}
function exactKeys(value: Obj, keys: string[]) {
  const actual = Object.keys(value).sort()
  const expected = [...keys].sort()
  return actual.length === expected.length && actual.every((key, index) => key === expected[index])
}
function isItems<T>(test: (v: unknown) => v is T) {
  return (v: unknown): v is { items: T[] } => isObj(v) && Array.isArray(v.items) && v.items.every(test)
}

export function createPlatformApi(fetcher: typeof fetch = fetch) {
  async function request<T>(path: string, options: { method?: 'GET' | 'POST'; brandId?: string; body?: unknown; key?: string; expectedStatus?: number } = {}): Promise<T> {
    const headers = new Headers({ Accept: 'application/json' })
    if (options.body !== undefined) headers.set('Content-Type', 'application/json')
    if (options.brandId) headers.set('X-Brand-ID', options.brandId)
    if (options.key) headers.set('Idempotency-Key', options.key)
    let response: Response
    try {
      response = await fetcher(`${BASE}${path}`, { method: options.method ?? 'GET', credentials: 'same-origin', headers, ...(options.body === undefined ? {} : { body: JSON.stringify(options.body) }) })
    } catch (cause) {
      throw new PlatformApiError(cause instanceof Error ? cause.message : 'Network request failed', 0, 'NETWORK_ERROR', true)
    }
    let envelope: unknown
    try { envelope = await response.json() } catch {
      if (!response.ok) apiError(`Request failed (${response.status})`, response.status)
      apiError('Invalid server response', response.status, 'INVALID_RESPONSE')
    }
    if (!isObj(envelope) || envelope.success !== true || envelope.data === undefined || !response.ok) {
      const err = isObj(envelope) && isObj(envelope.error) ? envelope.error : undefined
      const message = text(err?.message) ? err.message : `Request failed (${response.status})`
      apiError(message, response.status, text(err?.code) ? err.code : undefined)
    }
    if (options.expectedStatus !== undefined && response.status !== options.expectedStatus) apiError('Invalid server response status', response.status, 'INVALID_RESPONSE')
    return envelope.data as T
  }
  return {
    login(identifier: string, password: string, key = crypto.randomUUID()) {
      return request<unknown>('/auth/login', { method: 'POST', body: { identifier, password }, key })
    },
    logout(key = crypto.randomUUID()) { return request<unknown>('/auth/logout', { method: 'POST', body: {}, key }) },
    async me() {
      const data = await request<unknown>('/me')
      const account = isObj(data) ? data.account : undefined
      if (!isObj(account) || !text(account.id) || account.super_admin !== true) apiError('This account is not a platform super administrator', 403, 'PLATFORM_ADMIN_REQUIRED')
      return account as unknown as PlatformAccount
    },
    async brands() {
      const data = await request<unknown>('/brands')
      return validate(data, isItems(isBrand), 'brands').items
    },
    async users(brandId: string) {
      if (!brandId) apiError('A brand must be selected before loading members', 400, 'BRAND_REQUIRED')
      const data = await request<unknown>('/users?limit=100&offset=0', { brandId })
      return validate(data, isItems(isMember), 'brand members').items
    },
    async audit(brandId: string) {
      if (!brandId) apiError('A brand must be selected before loading audit records', 400, 'BRAND_REQUIRED')
      const data = await request<unknown>('/audit?limit=100&offset=0', { brandId })
      return validate(data, isItems(isAudit), 'audit').items
    },
    async createBrand(input: BrandCreateInput, key: string) {
      const body = { code: input.code, name: input.name, default_locale: input.default_locale, timezone: input.timezone, reason: input.reason }
      const data = await request<unknown>('/brands', { method: 'POST', body, key, expectedStatus: 201 })
      const ok = isObj(data) && exactKeys(data, ['id', 'code', 'name', 'status', 'default_locale', 'timezone', 'version', 'created_at', 'audit_log_id']) && UUID.test(String(data.id)) && data.code === body.code && data.name === body.name && data.status === 'paused' && data.default_locale === body.default_locale && data.timezone === body.timezone && data.version === 1 && dateTime(data.created_at) && UUID.test(String(data.audit_log_id))
      if (!ok) apiError('Invalid brand creation receipt', 0, 'INVALID_RESPONSE')
      return data as unknown as BrandCreateReceipt
    },
  }
}

export function freezeBrandCreateRequest(input: BrandCreateInput, key: string = crypto.randomUUID()): FrozenBrandCreateRequest {
  const body = Object.freeze({ code: input.code, name: input.name, default_locale: input.default_locale, timezone: input.timezone, reason: input.reason })
  return Object.freeze({ body, key })
}

export type PlatformApi = ReturnType<typeof createPlatformApi>
