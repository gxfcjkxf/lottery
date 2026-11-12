import { type ThemeId, themeIds } from '@lottery/shared/theme'
import { isPoints } from '@lottery/shared'
import { readPaginationTotal } from '@lottery/shared'
import { readApiEnvelope } from '@lottery/shared'

import { notifySessionReplaced } from '@lottery/shared'
export interface PlatformAccount { id: string; username?: string; display_name?: string; super_admin: true; platform_permissions: string[] }
export interface PlatformBrand { id: string; code: string; name: string; status: string; member_count: string; agent_count: string; recharge_quota_available: string | null; created_at: string }
export interface PlatformMember { id: string; global_user_id: string; username: string; phone: string; display_name: string; notes: string; status: string; joined_at: string; brand_id: string; tags: string[]; online: boolean; display_points: string | null }
export interface PlatformAudit { id: string; brand_id: string | null; action: string; actor_type: string; actor_id: string; actor_username: string; resource_type: string; resource_id: string; reason: string; request_id: string; created_at: string; ip_address: string; before_json: unknown; after_json: unknown }
export interface PlatformCommissionPolicy {
 payouts_enabled: boolean
 version: number
 config: { enabled: boolean; payout_mode: 'manual' | 'automatic'; calendar: null | { timezone: string; cycle: 'weekly' | 'monthly'; boundary_time: string; weekday: number | null; month_day: number | null; short_month: '' | 'last_day' | 'skip' } }
}
export interface PlatformSettings { theme: ThemeId; version: number; display_name: string; logo_text: string; logo_url: string; timezone: string; date_format: 'YYYY-MM-DD' | 'DD/MM/YYYY' | 'MM/DD/YYYY'; time_format: 'HH:mm:ss' | 'hh:mm:ss A'; updated_at: string; audit_log_id?: string }
export type PlatformPresentation = Pick<PlatformSettings, 'display_name' | 'logo_text' | 'logo_url'>
export interface BrandCreateInput { code: string; name: string; default_locale: 'en' | 'zh-CN'; timezone: string; reason?: string }
interface BrandCreateReceipt extends Pick<PlatformBrand, 'id' | 'code' | 'name' | 'status'> { status: 'paused'; default_locale: 'en' | 'zh-CN'; timezone: string; version: 1; created_at: string; audit_log_id: string }
export interface FrozenBrandCreateRequest { readonly body: Readonly<BrandCreateInput>; readonly key: string }

export class PlatformApiError extends Error {
  constructor(message: string, readonly status: number, readonly code?: string, readonly network = false) {
    super(message)
    this.name = 'PlatformApiError'
    notifySessionReplaced(code)
  }
}

export function readPlatformEnvelope<T = unknown>(response: Response) {
  return readApiEnvelope<T>(response, (message, status, code) => new PlatformApiError(message, status, code))
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
function isPlatformCommissionPolicy(value: unknown): value is PlatformCommissionPolicy {
 if (!isObj(value) || !exactKeys(value, ['version','config','payouts_enabled']) || !Number.isSafeInteger(value.version) || Number(value.version)<1 || typeof value.payouts_enabled !== 'boolean') return false
 const c=value.config
 if (!isObj(c) || !exactKeys(c,['enabled','calendar','payout_mode']) || typeof c.enabled !== 'boolean' || !['manual','automatic'].includes(String(c.payout_mode))) return false
 if (c.calendar === null) return !c.enabled
 const cal=c.calendar
 if (!isObj(cal) || !exactKeys(cal,['timezone','cycle','boundary_time','weekday','month_day','short_month']) || !text(cal.timezone) || cal.boundary_time!=='00:00:00') return false
 try { new Intl.DateTimeFormat('en',{timeZone: cal.timezone}) } catch { return false }
 return cal.cycle==='weekly' ? Number.isInteger(cal.weekday) && Number(cal.weekday)>=0 && Number(cal.weekday)<=6 && cal.month_day===null && cal.short_month==='' : cal.cycle==='monthly' && cal.weekday===null && Number.isInteger(cal.month_day) && Number(cal.month_day)>=1 && Number(cal.month_day)<=31 && ['last_day','skip'].includes(String(cal.short_month))
}
function isBrand(v: unknown): v is PlatformBrand {
  return isObj(v) && exactKeys(v, ['id', 'code', 'name', 'status', 'member_count', 'agent_count', 'recharge_quota_available', 'created_at']) && text(v.id) && text(v.code) && text(v.name) && text(v.status) && text(v.member_count) && /^(0|[1-9]\d*)$/.test(v.member_count) && text(v.agent_count) && /^(0|[1-9]\d*)$/.test(v.agent_count) && (v.recharge_quota_available === null || isPoints(v.recharge_quota_available)) && dateTime(v.created_at)
}
function isMember(v: unknown): v is PlatformMember {
  return isObj(v) && exactKeys(v, ['id', 'global_user_id', 'username', 'phone', 'display_name', 'notes', 'status', 'joined_at', 'brand_id', 'tags', 'online', 'display_points']) && text(v.id) && text(v.global_user_id) && text(v.username) && text(v.phone) && text(v.display_name) && text(v.notes) && text(v.status) && dateTime(v.joined_at) && text(v.brand_id) && stringArray(v.tags) && typeof v.online === 'boolean' && (v.display_points === null || text(v.display_points) && isPoints(v.display_points))
}
function isAudit(v: unknown): v is PlatformAudit {
  return isObj(v) && exactKeys(v, ['id', 'brand_id', 'action', 'actor_type', 'actor_id', 'actor_username', 'resource_type', 'resource_id', 'reason', 'request_id', 'created_at', 'ip_address', 'before_json', 'after_json']) && text(v.id) && (v.brand_id === null || text(v.brand_id)) && text(v.action) && text(v.actor_type) && text(v.actor_id) && text(v.actor_username) && text(v.resource_type) && text(v.resource_id) && text(v.reason) && text(v.request_id) && dateTime(v.created_at) && text(v.ip_address) && 'before_json' in v && 'after_json' in v
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
  async function request<T>(path: string, options: { method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'; brandId?: string; body?: unknown; key?: string; expectedStatus?: number; headers?: Record<string, string> } = {}): Promise<T> {
    const headers = new Headers({ Accept: 'application/json' })
    if (options.body !== undefined) headers.set('Content-Type', 'application/json')
    if (options.brandId) headers.set('X-Brand-ID', options.brandId)
    if (options.key) headers.set('Idempotency-Key', options.key)
    if (options.headers) for (const [name, value] of Object.entries(options.headers)) headers.set(name, value)
    let response: Response
    try {
      response = await fetcher(`${BASE}${path}`, { method: options.method ?? 'GET', credentials: 'same-origin', headers, ...(options.body === undefined ? {} : { body: JSON.stringify(options.body) }) })
    } catch (cause) {
      throw new PlatformApiError(cause instanceof Error ? cause.message : 'Network request failed', 0, 'NETWORK_ERROR', true)
    }
    const envelope = await readPlatformEnvelope<T>(response)
    if (!isObj(envelope.data)) apiError('Invalid server response; operation result is unconfirmed', 502, 'INVALID_RESPONSE')
    if (options.expectedStatus !== undefined && response.status !== options.expectedStatus) apiError('Invalid server response status', response.status, 'INVALID_RESPONSE')
    return envelope.data as T
  }
  return {
    async presentation(): Promise<PlatformPresentation> {
      const data = await request<unknown>('/presentation')
      if (!isObj(data) || !exactKeys(data, ['display_name', 'logo_text', 'logo_url']) || !text(data.display_name) || !data.display_name.trim() || !text(data.logo_text) || !data.logo_text.trim() || !text(data.logo_url)) apiError('Invalid platform presentation response', 0, 'INVALID_RESPONSE')
      return data as PlatformPresentation
    },
    login(identifier: string, password: string, key = crypto.randomUUID()) {
      return request<unknown>('/auth/login', { method: 'POST', body: { identifier, password }, key })
    },
    logout(key = crypto.randomUUID()) { return request<unknown>('/auth/logout', { method: 'POST', body: {}, key }) },
    changePassword(currentPassword: string, newPassword: string, key = crypto.randomUUID()) {
      return request<unknown>('/me/password', { method: 'POST', body: { current_password: currentPassword, new_password: newPassword }, key })
    },
    async me() {
      const data = await request<unknown>('/me')
      const account = isObj(data) ? data.account : undefined
      if (!isObj(account) || !text(account.id) || account.super_admin !== true) apiError('This account is not a platform super administrator', 403, 'PLATFORM_ADMIN_REQUIRED')
      if (!stringArray(account.platform_permissions)) apiError('Invalid platform permissions response', 0, 'INVALID_RESPONSE')
      return account as unknown as PlatformAccount
    },
    async commissionSettings() { return validate(await request<unknown>('/commission-settings'), isPlatformCommissionPolicy, 'commission settings') },
    async updateCommissionSettings(input: PlatformCommissionPolicy, key: string) { return validate(await request<unknown>('/commission-settings', { method: 'PUT', body: { ...input, reason: '' }, key }), isPlatformCommissionPolicy, 'commission settings') },
    async platformSettings() {
      const data = await request<unknown>('/platform-settings')
      if (!isPlatformSettings(data)) apiError('Invalid platform settings response', 0, 'INVALID_RESPONSE')
      return data
    },
    async updatePlatformSettings(input: Omit<PlatformSettings, 'audit_log_id' | 'updated_at'>, key = crypto.randomUUID()) {
      const data = await request<unknown>('/platform-settings', { method: 'PATCH', body: input, key })
      if (!isPlatformSettings(data)) apiError('Invalid platform settings response', 0, 'INVALID_RESPONSE')
      return data
    },
    async brands() {
      const data = await request<unknown>('/brands')
      return validate(data, isItems(isBrand), 'brands').items
    },
    async users(brandId: string, limit = 100, offset = 0, username = '', filters: { query?: string; online?: '' | 'true' | 'false' } = {}) {
      if (!brandId) apiError('A brand must be selected before loading members', 400, 'BRAND_REQUIRED')
      const query = new URLSearchParams({ limit: String(limit), offset: String(offset) })
      if (username.trim()) query.set('username', username.trim())
      if (filters.query?.trim()) query.set('q', filters.query.trim())
      if (filters.online) query.set('online', filters.online)
      const data = await request<unknown>(`/users?${query}`, { brandId })
      const items = validate(data, isItems(isMember), 'brand members').items
      if (items.length > limit || items.some(item => item.brand_id !== brandId)) apiError('Invalid brand members response', 0, 'INVALID_RESPONSE')
      if (!isObj(data) || data.limit !== limit || data.offset !== offset || !text(data.total_count) || !/^(0|[1-9]\d*)$/.test(data.total_count)) apiError('Invalid member pagination response', 0, 'INVALID_RESPONSE')
      return { items, limit, offset, total_count: data.total_count }
    },
    async audit(brandId: string, limit = 100, offset = 0, filters: Record<string, string> = {}) {
      if (!brandId) apiError('A brand must be selected before loading audit records', 400, 'BRAND_REQUIRED')
      const data = await request<unknown>(`/audit?${new URLSearchParams({ ...filters, limit: String(limit), offset: String(offset) })}`, { brandId })
      const items = validate(data, isItems(isAudit), 'audit').items
      if (items.length > limit || items.some(item => item.brand_id !== brandId)) apiError('Invalid audit response', 0, 'INVALID_RESPONSE')
      return {items, total_count: readPaginationTotal(data)}
    },
    async createBrand(input: BrandCreateInput, key: string) {
      const body = { code: input.code, name: input.name, default_locale: input.default_locale, timezone: input.timezone, ...(input.reason === undefined ? {} : { reason: input.reason }) }
      const data = await request<unknown>('/brands', { method: 'POST', body, key, expectedStatus: 201 })
      const ok = isObj(data) && exactKeys(data, ['id', 'code', 'name', 'status', 'default_locale', 'timezone', 'version', 'created_at', 'audit_log_id']) && UUID.test(String(data.id)) && data.code === body.code && data.name === body.name && data.status === 'paused' && data.default_locale === body.default_locale && data.timezone === body.timezone && data.version === 1 && dateTime(data.created_at) && UUID.test(String(data.audit_log_id))
      if (!ok) apiError('Invalid brand creation receipt', 0, 'INVALID_RESPONSE')
      return data as unknown as BrandCreateReceipt
    },
    async deleteBrand(brandId: string, key: string) {
      if (!brandId) apiError('A brand is required', 400, 'REQUEST_INVALID')
      const data = await request<unknown>(`/brands/${brandId}`, { method: 'DELETE', key })
      if (!isObj(data) || data.id !== brandId || !UUID.test(String(data.id))) apiError('Invalid brand deletion receipt', 0, 'INVALID_RESPONSE')
      return data as { id: string }
    },
    async deleteMemberAccount(userId: string, brandId: string, key: string) {
      if (!UUID.test(userId) || !UUID.test(brandId)) apiError('A valid account and brand are required', 400, 'REQUEST_INVALID')
      const data = await request<unknown>(`/global-users/${userId}/delete`, { method: 'POST', body: { brand_id: brandId }, key })
      if (!isObj(data) || !exactKeys(data, ['id', 'account_name', 'audit_log_id']) || data.id !== userId || !text(data.account_name) || !data.account_name || !UUID.test(String(data.audit_log_id))) apiError('Invalid account deletion receipt', 0, 'INVALID_RESPONSE')
      return data as { id: string; account_name: string; audit_log_id: string }
    },
  }
}

function isPlatformSettings(v: unknown): v is PlatformSettings {
  return isObj(v) && (exactKeys(v, ['version', 'display_name', 'logo_text', 'logo_url', 'timezone', 'date_format', 'time_format', 'theme', 'updated_at']) || exactKeys(v, ['version', 'display_name', 'logo_text', 'logo_url', 'timezone', 'date_format', 'time_format', 'theme', 'updated_at', 'audit_log_id'])) &&
    themeIds.includes(v.theme as ThemeId) && typeof v.version === 'number' && Number.isSafeInteger(v.version) && v.version > 0 && text(v.display_name) && text(v.logo_text) && text(v.logo_url) && text(v.timezone) && ['YYYY-MM-DD', 'DD/MM/YYYY', 'MM/DD/YYYY'].includes(String(v.date_format)) && ['HH:mm:ss', 'hh:mm:ss A'].includes(String(v.time_format)) && dateTime(v.updated_at) && (!('audit_log_id' in v) || text(v.audit_log_id))
}

export function freezeBrandCreateRequest(input: BrandCreateInput, key: string = crypto.randomUUID()): FrozenBrandCreateRequest {
  const body = Object.freeze({ code: input.code, name: input.name, default_locale: input.default_locale, timezone: input.timezone, ...(input.reason === undefined ? {} : { reason: input.reason }) })
  return Object.freeze({ body, key })
}
