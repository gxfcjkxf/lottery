import { readPaginationTotal } from '@lottery/shared'
import { resolveNotificationContent } from '@lottery/shared'
import { readPlatformEnvelope } from './platform-api'

import { PlatformApiError } from './platform-api'

const BASE = '/api/v1/platform'
const MAX_PAGE_SIZE = 100
const MAX_OFFSET = 1_000_000
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
const TEMPLATE_KEYS = [
  'bet.order.abnormal', 'bet.order.cancelled', 'bet.order.judged_cancelled', 'bet.order.placed',
  'bet.order.prize_reversed', 'bet.order.won', 'commission.adjusted', 'commission.corrected',
  'commission.paid', 'draw.result.corrected', 'draw.result.published', 'member.joined',
  'recharge.confirmed', 'reward.order.granted', 'reward.order.revoked',
  'withdrawal.order.cancelled', 'withdrawal.order.failed', 'withdrawal.order.paid',
  'withdrawal.order.processing', 'withdrawal.order.rejected', 'withdrawal.order.reviewing',
] as const

export type NotificationTemplateKey = typeof TEMPLATE_KEYS[number]
type DeliveryStatus = 'pending' | 'retry' | 'sent' | 'failed'
export interface Delivery {
  event_id: string
  brand_id: string
  status: DeliveryStatus
  attempt_count: number
  last_error: string | null
  next_attempt_at: string
  sent_at: string | null
}
interface TemplateCopy { title: string; body: string }
export interface TemplateContent { en: TemplateCopy; 'zh-CN': TemplateCopy }
export interface Template {
  key: NotificationTemplateKey
  version: number
  content: TemplateContent
  updated_at: string
  audit_log_id: string | null
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
function validTemplate(v: unknown): v is Template {
  return record(v) && exactKeys(v, ['key', 'version', 'content', 'updated_at', 'audit_log_id']) &&
    validKey(v.key) && validVersion(v.version) && validContent(v.content) && dateTime(v.updated_at) &&
    (v.audit_log_id === null || validUuid(v.audit_log_id))
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
  async function request(path: string, brand = '', body?: TemplateUpdate, key?: string): Promise<unknown> {
    const headers = new Headers({ Accept: 'application/json' })
    if (brand) headers.set('X-Brand-ID', brand)
    if (body) { headers.set('Content-Type', 'application/json'); headers.set('Idempotency-Key', key!) }
    let response: Response
    try {
      response = await fetcher(`${BASE}${path}`, { method: body ? 'PUT' : 'GET', credentials: 'same-origin', headers, ...(body ? { body: JSON.stringify(body) } : {}) })
    } catch (cause) {
      throw new PlatformApiError(cause instanceof Error ? cause.message : 'Network request failed', 0, 'NETWORK_ERROR', true)
    }
    const envelope = await readPlatformEnvelope<unknown>(response);
    const resolveRecord = (item: unknown): unknown => {
      if (!record(item) || !Object.hasOwn(item, 'content')) return item
      try { return { ...item, content: resolveNotificationContent(item.content, String(item.key)) } }
      catch { return invalid('notification translation content') }
    }
    const data = envelope.data
    return record(data) && Array.isArray(data.items)
      ? { ...data, items: data.items.map(resolveRecord) } : resolveRecord(data)
  }
  return {
    async deliveries(brandId: string, limit = 51, offset = 0): Promise<{items: Delivery[]; total_count: string}> {
      requiredBrand(brandId); pageArgs(limit, offset)
      const value = await request(`/notification-deliveries?limit=${limit}&offset=${offset}`, brandId)
      if (!record(value) || !exactKeys(value, ['items','total_count']) || !Array.isArray(value.items) || value.items.length > limit || !value.items.every(item => validDelivery(item, brandId))) invalid('notification deliveries')
      return {items: (value as { items: Delivery[] }).items, total_count: readPaginationTotal(value)}
    },
    async update(key: NotificationTemplateKey, body: TemplateUpdate, idempotencyKey: string): Promise<Template> {
      if (!validKey(key) || !validTemplateContent(body.content, key) || !validVersion(body.version) || body.version >= Number.MAX_SAFE_INTEGER || typeof body.reason !== 'string' || Array.from(body.reason).length > 500) throw new PlatformApiError('Invalid template input', 400, 'INVALID_INPUT')
      const value = await request(`/notification-templates/${encodeURIComponent(key)}`, '', body, idempotencyKey)
      if (!validTemplate(value) || value.key !== key || value.version !== body.version + 1 || JSON.stringify(value.content) !== JSON.stringify(body.content)) invalid('notification template update')
      return value as Template
    },
    async templates(): Promise<Template[]> {
      const value = await request('/notification-templates')
      if (!record(value) || !exactKeys(value, ['items']) || !Array.isArray(value.items) || value.items.length > TEMPLATE_KEYS.length ||
        !value.items.every(item => validTemplate(item)) || new Set(value.items.map(item => (item as Template).key)).size !== value.items.length) invalid('notification templates')
      return (value as { items: Template[] }).items
    },

  }
}


export interface TemplateUpdate { version: number; content: TemplateContent; reason: string }
function characters(value: string): number { return Array.from(value).length; }
function safeText(value: unknown, maxCharacters: number, title: boolean): value is string {
  return typeof value === "string" && value.trim() === value && value.trim().length > 0 && characters(value) <= maxCharacters &&
    !/[<>\uD800-\uDFFF]/u.test(value) && !/(?:https?:|javascript:|data:|www\.)/i.test(value) &&
    !(title ? /[\u0000-\u001f\u007f-\u009f]/ : /[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f-\u009f]/).test(value);
}
function placeholdersValid(text: string, key: NotificationTemplateKey, body: boolean): boolean {
  const tokens = text.match(/\{[^{}]*\}/g) ?? [];
  const residue = text.replace(/\{[^{}]*\}/g, "");
  if (/[{}]/.test(residue) || tokens.some((token) => !new Set(['points', 'resource_id']).has(token.slice(1, -1)))) return false;
  const hasPoints = tokens.some((token) => token === "{points}");
  const hasResourceId = tokens.some((token) => token === "{resource_id}");
  if (key.startsWith("draw.result.")) return !hasPoints && (!body || hasResourceId);
  return key === "member.joined" ? !hasPoints : !body || hasPoints;
}
export function validTemplateContent(value: unknown, key: NotificationTemplateKey): value is TemplateContent {
  if (!validKey(key) || !record(value) || !exactKeys(value, (['en', 'zh-CN'] as const))) return false;
  for (const locale of (['en', 'zh-CN'] as const)) {
    const entry = value[locale];
    if (!record(entry) || !exactKeys(entry, ["title", "body"]) || !safeText(entry.title, 120, true) ||
      !safeText(entry.body, 1200, false) || !placeholdersValid(entry.title, key, false) || !placeholdersValid(entry.body, key, true)) return false;
  }
  return true;
}
