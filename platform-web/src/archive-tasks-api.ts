import { PlatformApiError } from './platform-api'

export interface ArchiveTasksPolicy {
  brand_id: string
  version: number
  daily_enabled: boolean
  monthly_enabled: boolean
  daily_start_period: string | null
  monthly_start_period: string | null
  timezone: string
  audit_log_id: string | null
  updated_at: string
}

export interface ArchiveTask {
  id: string
  brand_id: string
  policy_version: number
  window: { kind: 'daily' | 'monthly'; period_key: string; timezone: string; from: string; to: string }
  state: 'pending' | 'completed' | 'skipped' | 'failed'
  version: number
  attempt_count: number
  archive_id: string | null
  last_error_code: string | null
  creation_audit_log_id: string
  last_audit_log_id: string
  created_at: string
  updated_at: string
}

export interface ArchiveTaskPage {
  brand_id: string
  items: ArchiveTask[]
  total_count: string
  limit: number
  offset: number
}

type Obj = Record<string, unknown>
const BASE = '/api/v1/platform'
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
const text = (v: unknown): v is string => typeof v === 'string'
const isObj = (v: unknown): v is Obj => Boolean(v && typeof v === 'object' && !Array.isArray(v))
const positiveInteger = (v: unknown): v is number => Number.isSafeInteger(v) && Number(v) >= 1
const dateTime = (v: unknown): v is string => text(v) && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.test(v) && Number.isFinite(Date.parse(v))

function invalid(message = 'Invalid report archive response'): never {
  throw new PlatformApiError(message, 0, 'INVALID_RESPONSE')
}

function exactKeys(value: Obj, keys: string[]) {
  const actual = Object.keys(value).sort()
  const expected = [...keys].sort()
  return actual.length === expected.length && actual.every((key, index) => key === expected[index])
}

function period(value: unknown, kind: 'daily' | 'monthly'): value is string {
  if (!text(value)) return false
  if (kind === 'daily') {
    const date = Date.parse(`${value}T00:00:00Z`)
    return /^\d{4}-\d{2}-\d{2}$/.test(value) && Number.isFinite(date) && new Date(date).toISOString().slice(0, 10) === value
  }
  return /^\d{4}-(0[1-9]|1[0-2])$/.test(value)
}

function isPolicy(v: unknown): v is ArchiveTasksPolicy {
  if (!isObj(v) || !exactKeys(v, ['brand_id', 'version', 'daily_enabled', 'monthly_enabled', 'daily_start_period', 'monthly_start_period', 'timezone', 'audit_log_id', 'updated_at'])) return false
  return UUID.test(String(v.brand_id)) && positiveInteger(v.version) && typeof v.daily_enabled === 'boolean' && typeof v.monthly_enabled === 'boolean' &&
    (v.daily_start_period === null || period(v.daily_start_period, 'daily')) && (v.monthly_start_period === null || period(v.monthly_start_period, 'monthly')) &&
    (!v.daily_enabled || v.daily_start_period !== null) && (!v.monthly_enabled || v.monthly_start_period !== null) &&
    text(v.timezone) && v.timezone.length > 0 && (v.audit_log_id === null || UUID.test(String(v.audit_log_id))) && dateTime(v.updated_at)
}

function isTask(v: unknown): v is ArchiveTask {
  if (!isObj(v) || !exactKeys(v, ['id', 'brand_id', 'policy_version', 'window', 'state', 'version', 'attempt_count', 'archive_id', 'last_error_code', 'creation_audit_log_id', 'last_audit_log_id', 'created_at', 'updated_at'])) return false
  if (!isObj(v.window) || !exactKeys(v.window, ['kind', 'period_key', 'timezone', 'from', 'to'])) return false
  const w = v.window
  const state = v.state === 'pending' || v.state === 'completed' || v.state === 'skipped' || v.state === 'failed'
  const archived = v.state === 'completed' || v.state === 'skipped'
  return UUID.test(String(v.id)) && UUID.test(String(v.brand_id)) && positiveInteger(v.policy_version) &&
    (w.kind === 'daily' || w.kind === 'monthly') && period(w.period_key, w.kind) && text(w.timezone) && w.timezone.length > 0 && dateTime(w.from) && dateTime(w.to) && Date.parse(w.to) > Date.parse(w.from) &&
    state && positiveInteger(v.version) && Number.isSafeInteger(v.attempt_count) && Number(v.attempt_count) >= (v.state === 'pending' ? 0 : 1) &&
    (v.archive_id === null || UUID.test(String(v.archive_id))) && (v.last_error_code === null || v.last_error_code === 'ARCHIVE_FAILED') &&
    (archived === (v.archive_id !== null)) && ((v.state === 'failed') === (v.last_error_code !== null)) &&
    UUID.test(String(v.creation_audit_log_id)) && UUID.test(String(v.last_audit_log_id)) && dateTime(v.created_at) && dateTime(v.updated_at) && Date.parse(v.updated_at) >= Date.parse(v.created_at)
}

export function createPlatformArchiveTasksApi(fetcher: typeof fetch = fetch) {
  async function request(path: string, brand: string): Promise<unknown> {
    if (!UUID.test(brand)) throw new PlatformApiError('A valid brand must be selected before loading report archive data', 400, 'BRAND_REQUIRED')
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
    if (!isObj(envelope) || envelope.success !== true || envelope.data === undefined || !response.ok) {
      const err = isObj(envelope) && isObj(envelope.error) ? envelope.error : undefined
      throw new PlatformApiError(text(err?.message) ? err.message : `Request failed (${response.status})`, response.status, text(err?.code) ? err.code : undefined)
    }
    if (response.status !== 200) invalid()
    return envelope.data
  }

  return {
    async policy(brand: string): Promise<ArchiveTasksPolicy> {
      const data = await request('/report-archive-policy', brand)
      if (!isPolicy(data) || data.brand_id !== brand) invalid()
      return data
    },
    async list(brand: string, limit = 50, offset = 0): Promise<ArchiveTaskPage> {
      if (!Number.isInteger(limit) || limit < 1 || limit > 100 || !Number.isInteger(offset) || offset < 0 || offset > 1_000_000) {
        throw new PlatformApiError('Invalid report archive paging parameters', 400, 'REQUEST_INVALID')
      }
      const data = await request(`/report-archive-tasks?limit=${limit}&offset=${offset}`, brand)
      if (!isObj(data) || !exactKeys(data, ['brand_id', 'items', 'total_count', 'limit', 'offset']) || data.brand_id !== brand ||
        !Array.isArray(data.items) || data.items.length > limit || !data.items.every(isTask) || data.items.some(task => task.brand_id !== brand) ||
        !text(data.total_count) || !/^(0|[1-9]\d*)$/.test(data.total_count) || !Number.isInteger(data.limit) || data.limit !== limit || !Number.isInteger(data.offset) || data.offset !== offset) invalid('Invalid report archive task page')
      const remaining = BigInt(data.total_count) - BigInt(offset)
      const expected = remaining <= 0n ? 0n : remaining < BigInt(limit) ? remaining : BigInt(limit)
      if (BigInt(data.items.length) !== expected || new Set(data.items.map(task => task.id)).size !== data.items.length) invalid('Invalid report archive task page')
      return data as unknown as ArchiveTaskPage
    },
    async read(brand: string, id: string): Promise<ArchiveTask> {
      if (!UUID.test(id)) throw new PlatformApiError('Invalid report archive task ID', 400, 'REQUEST_INVALID')
      const data = await request(`/report-archive-tasks/${encodeURIComponent(id)}`, brand)
      if (!isTask(data) || data.brand_id !== brand || data.id !== id) invalid('Invalid report archive task')
      return data
    },
  }
}
