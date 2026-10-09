import { PlatformApiError } from './platform-api'

const BASE = '/api/v1/platform'
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
const INTEGER = /^[1-9][0-9]*$/
type Obj = Record<string, unknown>

export interface BettingPolicy {
  brand_id: string
  version: number
  config: {
    min_bet_points: string
    max_bet_points: string | null
    max_period_points: string | null
    max_user_period_points: string | null
    user_cancel_allowed: boolean
  }
  updated_at: string
}
export interface SettlementPolicy { brand_id: string; version: number; mode: 'automatic' | 'manual' | null; updated_at: string; audit_log_id?: string }
export interface AgentPolicy {
  brand_id: string
  version: number
  config: { enabled: boolean; max_depth: number; ratio_cap: string; mode: 'loss' | 'turnover'; cycle: 'weekly' | 'monthly' }
  updated_at: string
  audit_log_id?: string
}
export interface CompliancePolicy {
  brand_id: string
  version: number
  config: { age_enabled: boolean; minimum_age: number | null; region_enabled: boolean; allowed_countries: string[]; identity_enabled: boolean }
  updated_at: string
  audit_log_id?: string
}
export interface CommissionPaymentPolicy { brand_id: string; version: number; enabled: boolean; audit_log_id: string; updated_at: string }
export interface CommissionCorrectionPolicy { brand_id: string; version: number; enabled: boolean; audit_log_id: string; updated_at: string }

function object(value: unknown): value is Obj { return Boolean(value && typeof value === 'object' && !Array.isArray(value)) }
function exactKeys(value: Obj, keys: string[]): boolean {
  const actual = Object.keys(value).sort(), expected = [...keys].sort()
  return actual.length === expected.length && actual.every((key, i) => key === expected[i])
}
function version(value: unknown): value is number { return Number.isSafeInteger(value) && Number(value) > 0 }
function dateTime(value: unknown): value is string {
  return typeof value === 'string' && /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?(?:Z|[+-]\d\d:\d\d)$/.test(value) && Number.isFinite(Date.parse(value))
}
function auditShape(value: Obj): boolean {
  if (!('audit_log_id' in value)) return true
  return typeof value.audit_log_id === 'string' && UUID.test(value.audit_log_id)
}
function policyEnvelope(value: unknown, brand: string, keys: string[]): value is Obj {
  return object(value) && (exactKeys(value, keys) || exactKeys(value, [...keys, 'audit_log_id'])) && value.brand_id === brand && version(value.version) && dateTime(value.updated_at) && auditShape(value)
}
function pointCap(value: unknown): value is string | null { return value === null || typeof value === 'string' && INTEGER.test(value) }
function validRatio(value: unknown): value is string { return typeof value === 'string' && /^(0|1|0\.[0-9]{0,5}[1-9])$/.test(value) }
function validBetting(value: unknown, brand: string): value is BettingPolicy {
  if (!object(value) || !exactKeys(value, ['brand_id', 'version', 'config', 'updated_at'])) return false
  if (!policyEnvelope(value, brand, ['brand_id', 'version', 'config', 'updated_at']) || !object(value.config) || !exactKeys(value.config, ['min_bet_points', 'max_bet_points', 'max_period_points', 'max_user_period_points', 'user_cancel_allowed'])) return false
  const c = value.config
  if (typeof c.min_bet_points !== 'string' || !INTEGER.test(c.min_bet_points) || typeof c.user_cancel_allowed !== 'boolean') return false
  const caps = [c.max_bet_points, c.max_period_points, c.max_user_period_points]
  return caps.every(pointCap) && caps.every(cap => cap === null || BigInt(c.min_bet_points as string) <= BigInt(cap))
}
function validSettlement(value: unknown, brand: string): value is SettlementPolicy {
  return policyEnvelope(value, brand, ['brand_id', 'version', 'mode', 'updated_at']) && (value.mode === null || value.mode === 'automatic' || value.mode === 'manual')
}
function validAgents(value: unknown, brand: string): value is AgentPolicy {
  if (!policyEnvelope(value, brand, ['brand_id', 'version', 'config', 'updated_at']) || !object(value.config) || !exactKeys(value.config, ['enabled', 'max_depth', 'ratio_cap', 'mode', 'cycle'])) return false
  const c = value.config
  return typeof c.enabled === 'boolean' && Number.isInteger(c.max_depth) && Number(c.max_depth) >= 1 && Number(c.max_depth) <= 32 && validRatio(c.ratio_cap) &&
    (c.mode === 'loss' || c.mode === 'turnover') && (c.cycle === 'weekly' || c.cycle === 'monthly')
}
function validCompliance(value: unknown, brand: string): value is CompliancePolicy {
  if (!policyEnvelope(value, brand, ['brand_id', 'version', 'config', 'updated_at']) || !object(value.config) || !exactKeys(value.config, ['age_enabled', 'minimum_age', 'region_enabled', 'allowed_countries', 'identity_enabled'])) return false
  const c = value.config
  const countries = c.allowed_countries
  if (typeof c.age_enabled !== 'boolean' || typeof c.region_enabled !== 'boolean' || typeof c.identity_enabled !== 'boolean' ||
    !(c.minimum_age === null || Number.isInteger(c.minimum_age) && Number(c.minimum_age) >= 18 && Number(c.minimum_age) <= 120) || c.age_enabled && c.minimum_age === null ||
    !Array.isArray(countries) || countries.length > 250 || c.region_enabled && countries.length === 0) return false
  return countries.every((country, index) => typeof country === 'string' && /^[A-Z]{2}$/.test(country) && (index === 0 || country > countries[index - 1]))
}
function validAuditPolicy(value: unknown, brand: string): value is CommissionPaymentPolicy {
  if (!object(value) || !exactKeys(value, ['brand_id', 'version', 'enabled', 'audit_log_id', 'updated_at']) || value.brand_id !== brand || !version(value.version) || typeof value.enabled !== 'boolean' || typeof value.audit_log_id !== 'string' || !dateTime(value.updated_at)) return false
  if (value.version === 1) return !value.enabled && value.audit_log_id === ''
  return UUID.test(value.audit_log_id)
}

export function createPlatformOperationalPoliciesApi(fetcher: typeof fetch = fetch) {
  async function request(path: string, brand: string): Promise<unknown> {
    if (!UUID.test(brand)) throw new PlatformApiError('A valid brand must be selected before loading operational policies', 400, 'BRAND_REQUIRED')
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
  async function read<T>(path: string, brand: string, validate: (data: unknown, brand: string) => data is T, label: string): Promise<T> {
    const data = await request(path, brand)
    if (!validate(data, brand)) throw new PlatformApiError(`Invalid ${label} response`, 0, 'INVALID_RESPONSE')
    return data
  }
  return {
    betting: (brand: string) => read('/bet-policy', brand, validBetting, 'betting policy'),
    settlement: (brand: string) => read('/settlement-policy', brand, validSettlement, 'settlement policy'),
    agents: (brand: string) => read('/agent-policy', brand, validAgents, 'agent policy'),
    compliance: (brand: string) => read('/compliance-policy', brand, validCompliance, 'compliance policy'),
    payment: (brand: string) => read('/commission-payment-policy', brand, validAuditPolicy, 'commission payment policy'),
    correction: (brand: string) => read('/commission-correction-policy', brand, validAuditPolicy, 'commission correction policy'),
  }
}
