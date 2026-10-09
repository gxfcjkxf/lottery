import { PlatformApiError } from './platform-api'

export type RewardState = 'granted' | 'revocation_pending' | 'revoked'
export type RewardOperation = 'grant' | 'revoke' | 'retry'

export interface RewardOrder {
  id: string
  brand_id: string
  member_id: string
  points: string
  state: RewardState
  version: number
  grant_ledger_entry_id: string
  revoke_ledger_entry_id: string | null
  creation_audit_log_id: string
  last_audit_log_id: string
  last_error_code: string | null
  created_by: string
  reason: string
  point_policy_version: string
  created_at: string
  updated_at: string
  revoked_at: string | null
}

export interface RewardOrderPage {
  brand_id: string
  items: RewardOrder[]
  total_count: string
  limit: number
  offset: number
}

export interface RewardAction {
  id: string
  brand_id: string
  order_id: string
  version: number
  operation: RewardOperation
  state_before: RewardState | null
  state_after: RewardState
  actor_id: string
  reason: string
  audit_log_id: string
  ledger_entry_id: string | null
  created_at: string
}

export interface RewardActionPage {
  brand_id: string
  order_id: string
  items: RewardAction[]
  total_count: string
  limit: number
  offset: number
}

const BASE = '/api/v1/platform/reward-orders'
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
const DATE_TIME = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/
type Obj = Record<string, unknown>
const isObj = (value: unknown): value is Obj => Boolean(value && typeof value === 'object' && !Array.isArray(value))
const keysAre = (value: Obj, keys: readonly string[]) => Object.keys(value).length === keys.length && keys.every(key => key in value)
const isUuid = (value: unknown): value is string => typeof value === 'string' && UUID.test(value)
const isDateTime = (value: unknown): value is string => typeof value === 'string' && DATE_TIME.test(value) && Number.isFinite(Date.parse(value))
const isReason = (value: unknown): value is string => typeof value === 'string' && value.length > 0
const states: readonly string[] = ['granted', 'revocation_pending', 'revoked']
const orderKeys = ['id', 'brand_id', 'member_id', 'points', 'state', 'version', 'grant_ledger_entry_id', 'revoke_ledger_entry_id', 'creation_audit_log_id', 'last_audit_log_id', 'last_error_code', 'created_by', 'reason', 'point_policy_version', 'created_at', 'updated_at', 'revoked_at']
const actionKeys = ['id', 'brand_id', 'order_id', 'version', 'operation', 'state_before', 'state_after', 'actor_id', 'reason', 'audit_log_id', 'ledger_entry_id', 'created_at']

function invalid(label: string): never { throw new PlatformApiError(`Invalid ${label} response`, 0, 'INVALID_RESPONSE') }
function validOrder(value: unknown, brandId: string): value is RewardOrder {
  if (!isObj(value) || !keysAre(value, orderKeys) || value.brand_id !== brandId || !isUuid(value.id) || !isUuid(value.brand_id) ||
    !isUuid(value.member_id) || typeof value.points !== 'string' || !/^[1-9][0-9]*$/.test(value.points) || !states.includes(String(value.state)) ||
    typeof value.version !== 'number' || !Number.isSafeInteger(value.version) || value.version < 1 || !isUuid(value.grant_ledger_entry_id) ||
    !(value.revoke_ledger_entry_id === null || isUuid(value.revoke_ledger_entry_id)) || !isUuid(value.creation_audit_log_id) ||
    !isUuid(value.last_audit_log_id) || !(value.last_error_code === null || typeof value.last_error_code === 'string') ||
    !isUuid(value.created_by) || !isReason(value.reason) || typeof value.point_policy_version !== 'string' ||
    !isDateTime(value.created_at) || !isDateTime(value.updated_at) || !(value.revoked_at === null || isDateTime(value.revoked_at))) return false
  if (value.state === 'granted') return value.version === 1 && value.revoke_ledger_entry_id === null && value.revoked_at === null && value.last_error_code === null
  if (value.state === 'revocation_pending') return value.version >= 2 && value.revoke_ledger_entry_id === null && value.revoked_at === null && value.last_error_code === 'REWARD_AVAILABLE_INSUFFICIENT'
  return value.version >= 2 && isUuid(value.revoke_ledger_entry_id) && value.revoked_at !== null && value.last_error_code === null
}
function validAction(value: unknown, brandId: string, orderId: string): value is RewardAction {
  return isObj(value) && keysAre(value, actionKeys) && value.brand_id === brandId && value.order_id === orderId && isUuid(value.id) &&
    isUuid(value.brand_id) && isUuid(value.order_id) && Number.isSafeInteger(value.version) && Number(value.version) > 0 &&
    ['grant', 'revoke', 'retry'].includes(String(value.operation)) && (value.state_before === null || states.includes(String(value.state_before))) &&
    states.includes(String(value.state_after)) && isUuid(value.actor_id) && isReason(value.reason) && isUuid(value.audit_log_id) &&
    (value.ledger_entry_id === null || isUuid(value.ledger_entry_id)) && isDateTime(value.created_at)
}
function validPage(value: unknown, brandId: string, limit: number, offset: number): value is RewardOrderPage {
  if (!isObj(value) || !keysAre(value, ['brand_id', 'items', 'total_count', 'limit', 'offset']) || value.brand_id !== brandId ||
    !Array.isArray(value.items) || value.items.length > limit || typeof value.total_count !== 'string' || !/^(0|[1-9][0-9]*)$/.test(value.total_count) ||
    value.limit !== limit || value.offset !== offset || !value.items.every(item => validOrder(item, brandId))) return false
  return new Set(value.items.map(item => (item as RewardOrder).id)).size === value.items.length
}
function validActionPage(value: unknown, brandId: string, orderId: string, limit: number, offset: number): value is RewardActionPage {
  return isObj(value) && keysAre(value, ['brand_id', 'order_id', 'items', 'total_count', 'limit', 'offset']) && value.brand_id === brandId &&
    value.order_id === orderId && Array.isArray(value.items) && value.items.length <= limit && typeof value.total_count === 'string' &&
    /^(0|[1-9][0-9]*)$/.test(value.total_count) && value.limit === limit && value.offset === offset &&
    value.items.every(item => validAction(item, brandId, orderId))
}

export function createPlatformRewardsApi(fetcher: typeof fetch = fetch) {
  async function get(path: string, brandId: string): Promise<unknown> {
    if (!brandId) throw new PlatformApiError('A brand must be selected before loading rewards', 400, 'BRAND_REQUIRED')
    let response: Response
    try {
      response = await fetcher(path, { credentials: 'same-origin', headers: { Accept: 'application/json', 'X-Brand-ID': brandId } })
    } catch (cause) {
      throw new PlatformApiError(cause instanceof Error ? cause.message : 'Network request failed', 0, 'NETWORK_ERROR', true)
    }
    let envelope: unknown
    try { envelope = await response.json() } catch {
      throw new PlatformApiError(response.ok ? 'Invalid server response' : `Request failed (${response.status})`, response.status, 'INVALID_RESPONSE')
    }
    if (!isObj(envelope) || envelope.success !== true || envelope.data === undefined || !response.ok) {
      const error = isObj(envelope) && isObj(envelope.error) ? envelope.error : undefined
      throw new PlatformApiError(typeof error?.message === 'string' ? error.message : `Request failed (${response.status})`, response.status, typeof error?.code === 'string' ? error.code : undefined)
    }
    return envelope.data
  }
  return {
    async list(brandId: string, limit = 100, offset = 0) {
      const data = await get(`${BASE}?limit=${limit}&offset=${offset}`, brandId)
      if (!validPage(data, brandId, limit, offset)) invalid('reward orders')
      return data
    },
    async read(brandId: string, orderId: string) {
      const data = await get(`${BASE}/${encodeURIComponent(orderId)}`, brandId)
      if (!validOrder(data, brandId) || data.id !== orderId) invalid('reward order')
      return data
    },
    async actions(brandId: string, orderId: string, limit = 100, offset = 0) {
      const data = await get(`${BASE}/${encodeURIComponent(orderId)}/actions?limit=${limit}&offset=${offset}`, brandId)
      if (!validActionPage(data, brandId, orderId, limit, offset)) invalid('reward actions')
      return data
    },
  }
}
