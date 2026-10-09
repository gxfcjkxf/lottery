import { PlatformApiError } from './platform-api'

const BASE = '/api/v1/platform'
const MAX_PAGE_SIZE = 100
const MAX_OFFSET = 1_000_000
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
const TEMPLATE_KEYS = [
  'bet.order.abnormal', 'bet.order.cancelled', 'bet.order.judged_cancelled', 'bet.order.placed',
  'bet.order.prize_reversed', 'bet.order.won', 'commission.adjusted', 'commission.corrected',
  'commission.paid', 'draw.result.corrected', 'draw.result.published', 'member.joined',
  'recharge.confirmed', 'reward.order.granted', 'reward.order.revocation_pending', 'reward.order.revoked',
  'withdrawal.order.cancelled', 'withdrawal.order.failed', 'withdrawal.order.paid',
  'withdrawal.order.processing', 'withdrawal.order.rejected', 'withdrawal.order.reviewing',
] as const

export type NotificationTemplateKey = typeof TEMPLATE_KEYS[number]
export type DeliveryStatus = 'pending' | 'retry' | 'sent' | 'failed'
export interface Delivery {
  event_id: string
  brand_id: string
  status: DeliveryStatus
  attempt_count: number
  last_error: string | null
  next_attempt_at: string
  sent_at: string | null
}
export interface TemplateCopy { title: string; body: string }
export interface TemplateContent { en: TemplateCopy; 'zh-CN': TemplateCopy }
export interface Template {
  brand_id: string
  key: NotificationTemplateKey
  version: number
  content: TemplateContent
  updated_at: string
  audit_log_id: string | null
}
export interface TemplateRevision {
  id: string
  brand_id: string
  key: NotificationTemplateKey
  version: number
  content: TemplateContent
  changed_by: string | null
  reason: string
  audit_log_id: string | null
  created_at: string
}

const record = (v: unknown): v is Record<string, unknown> => Boolean(v && typeof v === 'object' && !Array.isArray(v))
const exactKeys = (v: Record<string, unknown>, keys: readonly string[]) => {
  const actual = Object.keys(v).sort(), expected = [...keys].sort()
  return actual.length === expected.length && actual.every((key, i) => key === expected[i])
}
const invalid = (what: string): never => { throw new PlatformApiError(`Invalid ${what} response`, 0, 'INVALID_RESPONSE') }
const validUuid = (v: unknown): v is string => typeof v === 'string' && UUID.test(v)
const validVersion = (v: unknown): v is number => Number.isSafeInteger(v) && Number(v) >= 1
function dateTime(v: unknown): v is string {
  if (typeof v !== 'string') return false
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|([+-])(\d{2}):(\d{2}))$/.exec(v)
  if (!m) return false
  const [, ys, mos, ds, hs, mis, ss, , , , oh, om] = m
  const year = Number(ys), month = Number(mos), day = Number(ds)
  const days = month === 2 ? (year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28) : [4, 6, 9, 11].includes(month) ? 30 : 31
  return year > 0 && month >= 1 && month <= 12 && day >= 1 && day <= days && Number(hs) <= 23 && Number(mis) <= 59 && Number(ss) <= 59 &&
    (oh === undefined || (Number(oh) <= 14 && Number(om) <= 59 && (Number(oh) !== 14 || Number(om) === 0))) && Number.isFinite(Date.parse(v))
}
function validContent(v: unknown): v is TemplateContent {
  if (!record(v) || !exactKeys(v, ['en', 'zh-CN'])) return false
  return (['en', 'zh-CN'] as const).every(locale => {
    const copy = v[locale]
    return record(copy) && exactKeys(copy, ['title', 'body']) && typeof copy.title === 'string' && typeof copy.body === 'string'
  })
}
function validKey(v: unknown): v is NotificationTemplateKey { return typeof v === 'string' && (TEMPLATE_KEYS as readonly string[]).includes(v) }
function validTemplate(v: unknown, brand: string): v is Template {
  return record(v) && exactKeys(v, ['brand_id', 'key', 'version', 'content', 'updated_at', 'audit_log_id']) &&
    v.brand_id === brand && validKey(v.key) && validVersion(v.version) && validContent(v.content) && dateTime(v.updated_at) &&
    (v.audit_log_id === null || validUuid(v.audit_log_id))
}
function validRevision(v: unknown, brand: string, key: NotificationTemplateKey): v is TemplateRevision {
  return record(v) && exactKeys(v, ['id', 'brand_id', 'key', 'version', 'content', 'changed_by', 'reason', 'audit_log_id', 'created_at']) &&
    validUuid(v.id) && v.brand_id === brand && v.key === key && validVersion(v.version) && validContent(v.content) &&
    (v.changed_by === null || validUuid(v.changed_by)) && typeof v.reason === 'string' &&
    (v.audit_log_id === null || validUuid(v.audit_log_id)) && dateTime(v.created_at)
}
function validDelivery(v: unknown, brand: string): v is Delivery {
  return record(v) && exactKeys(v, ['event_id', 'brand_id', 'status', 'attempt_count', 'last_error', 'next_attempt_at', 'sent_at']) &&
    validUuid(v.event_id) && v.brand_id === brand && ['pending', 'retry', 'sent', 'failed'].includes(String(v.status)) &&
    Number.isSafeInteger(v.attempt_count) && Number(v.attempt_count) >= 0 && (v.last_error === null || typeof v.last_error === 'string') &&
    dateTime(v.next_attempt_at) && (v.sent_at === null || dateTime(v.sent_at))
}
function pageArgs(limit: number, offset: number) {
  if (!Number.isSafeInteger(limit) || limit < 1 || limit > MAX_PAGE_SIZE || !Number.isSafeInteger(offset) || offset < 0 || offset > MAX_OFFSET) {
    throw new PlatformApiError('Invalid notification pagination', 400, 'INVALID_INPUT')
  }
}
function requiredBrand(brand: string) {
  if (!validUuid(brand)) throw new PlatformApiError('A valid brand must be selected', 400, 'BRAND_REQUIRED')
}

export function createPlatformNotificationsApi(fetcher: typeof fetch = fetch) {
  async function request(path: string, brand: string): Promise<unknown> {
    const headers = new Headers({ Accept: 'application/json', 'X-Brand-ID': brand })
    let response: Response
    try {
      response = await fetcher(`${BASE}${path}`, { method: 'GET', credentials: 'same-origin', headers })
    } catch (cause) {
      throw new PlatformApiError(cause instanceof Error ? cause.message : 'Network request failed', 0, 'NETWORK_ERROR', true)
    }
    let envelope: unknown
    try { envelope = await response.json() } catch {
      if (!response.ok) throw new PlatformApiError(`Request failed (${response.status})`, response.status)
      invalid('notification')
    }
    if (!record(envelope) || envelope.success !== true || envelope.data === undefined || !response.ok) {
      const error = record(envelope) && record(envelope.error) ? envelope.error : undefined
      const message = typeof error?.message === 'string' ? error.message : `Request failed (${response.status})`
      throw new PlatformApiError(message, response.status, typeof error?.code === 'string' ? error.code : undefined)
    }
    return envelope.data
  }
  return {
    async deliveries(brandId: string, limit = 51, offset = 0): Promise<Delivery[]> {
      requiredBrand(brandId); pageArgs(limit, offset)
      const value = await request(`/notification-deliveries?limit=${limit}&offset=${offset}`, brandId)
      if (!record(value) || !exactKeys(value, ['items']) || !Array.isArray(value.items) || value.items.length > limit || !value.items.every(item => validDelivery(item, brandId))) invalid('notification deliveries')
      return (value as { items: Delivery[] }).items
    },
    async templates(brandId: string): Promise<Template[]> {
      requiredBrand(brandId)
      const value = await request('/notification-templates', brandId)
      if (!record(value) || !exactKeys(value, ['items']) || !Array.isArray(value.items) || value.items.length > TEMPLATE_KEYS.length ||
        !value.items.every(item => validTemplate(item, brandId)) || new Set(value.items.map(item => (item as Template).key)).size !== value.items.length) invalid('notification templates')
      return (value as { items: Template[] }).items
    },
    async history(brandId: string, key: string, limit = 51, offset = 0): Promise<TemplateRevision[]> {
      requiredBrand(brandId); pageArgs(limit, offset)
      if (!validKey(key)) throw new PlatformApiError('Invalid notification template key', 400, 'INVALID_INPUT')
      const query = new URLSearchParams({ limit: String(limit), offset: String(offset) })
      const value = await request(`/notification-templates/${encodeURIComponent(key)}/history?${query}`, brandId)
      if (!record(value) || !exactKeys(value, ['items']) || !Array.isArray(value.items) || value.items.length > limit || !value.items.every(item => validRevision(item, brandId, key))) invalid('notification template history')
      return (value as { items: TemplateRevision[] }).items
    },
  }
}

export const notificationTemplateKeys = TEMPLATE_KEYS
export type PlatformNotificationsApi = ReturnType<typeof createPlatformNotificationsApi>
