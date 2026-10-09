import { PlatformApiError } from './platform-api'

const BASE = '/api/v1/platform'
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
const INTEGER = /^[1-9][0-9]*$/
const MULTIPLE = /^(0|[1-9][0-9]{0,6})(?:\.([0-9]{0,5}[1-9]))?$/
type Obj = Record<string, unknown>

export interface PointPolicy {
  brand_id: string
  version: number
  max_balance_points: string | null
  max_recharge_points: string | null
  max_adjustment_points: string | null
  audit_log_id?: string
}

export interface WithdrawalPolicy {
  brand_id: string
  version: number
  config: {
    enabled: boolean
    min_points: string
    max_points: string | null
    allowed_sources: Array<'recharge' | 'winning' | 'commission' | 'gift'>
    review_mode: 'manual' | 'automatic'
    turnover_multiple: string
  }
  updated_at: string
  audit_log_id?: string
}

export interface CommissionPolicy {
  brand_id: string
  version: number
  config: {
    enabled: boolean
    calendar: CommissionCalendar | null
    payout_mode: 'manual' | 'automatic'
  }
  created_at: string
  updated_at: string
  revision_id: string
  audit_log_id?: string
}

export interface CommissionCalendar {
  timezone: string
  cycle: 'weekly' | 'monthly'
  boundary_time: string
  weekday: number | null
  month_day: number | null
  short_month: '' | 'last_day' | 'skip'
}

function object(value: unknown): value is Obj { return Boolean(value && typeof value === 'object' && !Array.isArray(value)) }
function exactKeys(value: Obj, keys: string[]): boolean {
  const actual = Object.keys(value).sort()
  const expected = [...keys].sort()
  return actual.length === expected.length && actual.every((key, i) => key === expected[i])
}
function positiveVersion(value: unknown): value is number { return Number.isSafeInteger(value) && Number(value) > 0 }
function positiveIntegerString(value: unknown): value is string { return typeof value === 'string' && INTEGER.test(value) }
function nullableCap(value: unknown): value is string | null { return value === null || positiveIntegerString(value) }
function dateTime(value: unknown): value is string {
  return typeof value === 'string' && /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?(?:Z|[+-]\d\d:\d\d)$/.test(value) && Number.isFinite(Date.parse(value))
}
function validMultiple(value: unknown): value is string {
  if (typeof value !== 'string') return false
  const match = MULTIPLE.exec(value)
  if (!match) return false
  const [whole, fraction = ''] = value.split('.')
  const scaled = BigInt(whole) * 1_000_000n + BigInt(fraction.padEnd(6, '0') || '0')
  return scaled > 0n && scaled <= 1_000_000_000_000n
}
function optionalAudit(value: Obj): boolean {
  if (!('audit_log_id' in value)) return true
  return UUID.test(String(value.audit_log_id))
}
function isPointPolicy(value: unknown, brand: string): value is PointPolicy {
  if (!object(value) || !(exactKeys(value, ['brand_id', 'version', 'max_balance_points', 'max_recharge_points', 'max_adjustment_points']) || exactKeys(value, ['brand_id', 'version', 'max_balance_points', 'max_recharge_points', 'max_adjustment_points', 'audit_log_id']))) return false
  return value.brand_id === brand && positiveVersion(value.version) && nullableCap(value.max_balance_points) && nullableCap(value.max_recharge_points) && nullableCap(value.max_adjustment_points) && optionalAudit(value)
}
function isWithdrawalPolicy(value: unknown, brand: string): value is WithdrawalPolicy {
  if (!object(value) || !(exactKeys(value, ['brand_id', 'version', 'config', 'updated_at']) || exactKeys(value, ['brand_id', 'version', 'config', 'updated_at', 'audit_log_id'])) || !object(value.config)) return false
  const c = value.config
  if (!exactKeys(c, ['enabled', 'min_points', 'max_points', 'allowed_sources', 'review_mode', 'turnover_multiple'])) return false
  if (value.brand_id !== brand || !positiveVersion(value.version) || !dateTime(value.updated_at) || !optionalAudit(value) || typeof c.enabled !== 'boolean' || !positiveIntegerString(c.min_points) || !(c.max_points === null || positiveIntegerString(c.max_points)) ||
    c.max_points !== null && BigInt(c.max_points as string) < BigInt(c.min_points) || !Array.isArray(c.allowed_sources) || c.allowed_sources.length < 1 || c.allowed_sources.length > 4 ||
    !c.allowed_sources.every(source => source === 'recharge' || source === 'winning' || source === 'commission' || source === 'gift') || new Set(c.allowed_sources).size !== c.allowed_sources.length ||
    (c.review_mode !== 'manual' && c.review_mode !== 'automatic') || !validMultiple(c.turnover_multiple)) return false
  return true
}
function validTimezone(value: unknown): value is string {
  if (typeof value !== 'string' || value.length === 0 || value === 'Local') return false
  try { new Intl.DateTimeFormat('en', { timeZone: value }); return true } catch { return false }
}
function isCalendar(value: unknown): value is CommissionCalendar {
  if (!object(value) || !exactKeys(value, ['timezone', 'cycle', 'boundary_time', 'weekday', 'month_day', 'short_month']) || !validTimezone(value.timezone) ||
    typeof value.boundary_time !== 'string' || !/^(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d$/.test(value.boundary_time)) return false
  if (value.cycle === 'weekly') return Number.isInteger(value.weekday) && Number(value.weekday) >= 0 && Number(value.weekday) <= 6 && value.month_day === null && value.short_month === ''
  if (value.cycle === 'monthly') return value.weekday === null && Number.isInteger(value.month_day) && Number(value.month_day) >= 1 && Number(value.month_day) <= 31 && (value.short_month === 'last_day' || value.short_month === 'skip')
  return false
}
function isCommissionPolicy(value: unknown, brand: string): value is CommissionPolicy {
  if (!object(value) || !(exactKeys(value, ['brand_id', 'version', 'config', 'created_at', 'updated_at', 'revision_id']) || exactKeys(value, ['brand_id', 'version', 'config', 'created_at', 'updated_at', 'revision_id', 'audit_log_id'])) || !object(value.config)) return false
  const c = value.config
  if (!exactKeys(c, ['enabled', 'calendar', 'payout_mode']) || value.brand_id !== brand || !positiveVersion(value.version) || !dateTime(value.created_at) || !dateTime(value.updated_at) ||
    !UUID.test(String(value.revision_id)) || !optionalAudit(value) || value.version > 1 && !('audit_log_id' in value) || typeof c.enabled !== 'boolean' || (c.payout_mode !== 'manual' && c.payout_mode !== 'automatic') ||
    !(c.calendar === null || isCalendar(c.calendar)) || c.enabled && c.calendar === null) return false
  return true
}

export function createPlatformFinancialPoliciesApi(fetcher: typeof fetch = fetch) {
  async function request(path: string, brand: string): Promise<unknown> {
    if (!UUID.test(brand)) throw new PlatformApiError('A valid brand must be selected before loading financial policies', 400, 'BRAND_REQUIRED')
    let response: Response
    try {
      response = await fetcher(`${BASE}${path}`, { method: 'GET', credentials: 'same-origin', headers: new Headers({ Accept: 'application/json', 'X-Brand-ID': brand }) })
    } catch (cause) {
      throw new PlatformApiError(cause instanceof Error ? cause.message : 'Network request failed', 0, 'NETWORK_ERROR', true)
    }
    let envelope: unknown
    try { envelope = await response.json() } catch {
      if (!response.ok) throw new PlatformApiError(`Request failed (${response.status})`, response.status)
      throw new PlatformApiError('Invalid server response', response.status, 'INVALID_RESPONSE')
    }
    if (!object(envelope) || envelope.success !== true || envelope.data === undefined || !response.ok) {
      const error = object(envelope) && object(envelope.error) ? envelope.error : undefined
      throw new PlatformApiError(typeof error?.message === 'string' ? error.message : `Request failed (${response.status})`, response.status, typeof error?.code === 'string' ? error.code : undefined)
    }
    if (response.status !== 200) throw new PlatformApiError('Invalid server response status', response.status, 'INVALID_RESPONSE')
    return envelope.data
  }

  return {
    async points(brand: string): Promise<PointPolicy> {
      const data = await request('/point-policy', brand)
      if (!isPointPolicy(data, brand)) throw new PlatformApiError('Invalid point policy response', 0, 'INVALID_RESPONSE')
      return data
    },
    async withdrawal(brand: string): Promise<WithdrawalPolicy> {
      const data = await request('/withdrawal-policy', brand)
      if (!isWithdrawalPolicy(data, brand)) throw new PlatformApiError('Invalid withdrawal policy response', 0, 'INVALID_RESPONSE')
      return data
    },
    async commission(brand: string): Promise<CommissionPolicy> {
      const data = await request('/commission-policy', brand)
      if (!isCommissionPolicy(data, brand)) throw new PlatformApiError('Invalid commission policy response', 0, 'INVALID_RESPONSE')
      return data
    },
  }
}
