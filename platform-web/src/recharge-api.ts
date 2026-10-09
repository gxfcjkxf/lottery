import { PlatformApiError } from './platform-api'

const BASE = '/api/v1/platform/recharges'
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i
const INT64_MAX = 9223372036854775807n

export interface Recharge {
  id: string
  brand_id: string
  member_id: string
  account_id: string
  points: string
  state: 'pending' | 'confirmed' | 'cancelled'
  proof_reference: string
  remark: string
  created_by: string
  confirmed_by?: string
  version: number
  created_at: string
  confirmed_at?: string
  ledger_entry_id?: string
  audit_log_id?: string
}

type Obj = Record<string, unknown>
const isObj = (v: unknown): v is Obj => Boolean(v && typeof v === 'object' && !Array.isArray(v))
const isDateTime = (v: unknown): v is string => typeof v === 'string' && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.test(v) && Number.isFinite(Date.parse(v))
const isUUID = (v: unknown): v is string => typeof v === 'string' && UUID.test(v)
const exactKeys = (v: Obj, required: string[], optional: string[]) => {
  const keys = Object.keys(v)
  return required.every(key => keys.includes(key)) && keys.every(key => required.includes(key) || optional.includes(key))
}

function isRecharge(v: unknown): v is Recharge {
  if (!isObj(v)) return false
  const required = ['id', 'brand_id', 'member_id', 'account_id', 'points', 'state', 'proof_reference', 'remark', 'created_by', 'version', 'created_at']
  const optional = ['confirmed_by', 'confirmed_at', 'ledger_entry_id', 'audit_log_id']
  if (!exactKeys(v, required, optional) || !isUUID(v.id) || !isUUID(v.brand_id) || !isUUID(v.member_id) || !isUUID(v.account_id) ||
    typeof v.points !== 'string' || !/^[1-9]\d*$/.test(v.points) || BigInt(v.points) > INT64_MAX ||
    !['pending', 'confirmed', 'cancelled'].includes(String(v.state)) || typeof v.proof_reference !== 'string' || typeof v.remark !== 'string' || !isUUID(v.created_by) ||
    typeof v.version !== 'number' || !Number.isSafeInteger(v.version) || v.version < 1 || !isDateTime(v.created_at)) return false
  for (const key of ['confirmed_by', 'ledger_entry_id', 'audit_log_id']) if (key in v && !isUUID(v[key])) return false
  if ('confirmed_at' in v && !isDateTime(v.confirmed_at)) return false
  const confirmedFields = ['confirmed_by', 'confirmed_at', 'ledger_entry_id']
  return v.state === 'confirmed' ? confirmedFields.every(key => key in v) : confirmedFields.every(key => !(key in v))
}

export function createPlatformRechargeApi(fetcher: typeof fetch = fetch) {
  return {
    async list(brandId: string, memberId = '', limit = 51, offset = 0): Promise<Recharge[]> {
      if (!brandId) throw new PlatformApiError('A brand must be selected before loading recharges', 400, 'BRAND_REQUIRED')
      if (!Number.isInteger(limit) || limit < 1 || limit > 100 || !Number.isInteger(offset) || offset < 0 || offset > 1_000_000) {
        throw new PlatformApiError('Invalid recharge pagination', 400, 'REQUEST_INVALID')
      }
      const query = new URLSearchParams({ limit: String(limit), offset: String(offset) })
      if (memberId) query.set('member_id', memberId)
      let response: Response
      try {
        response = await fetcher(`${BASE}?${query}`, { method: 'GET', credentials: 'same-origin', headers: { Accept: 'application/json', 'X-Brand-ID': brandId } })
      } catch (cause) {
        throw new PlatformApiError(cause instanceof Error ? cause.message : 'Network request failed', 0, 'NETWORK_ERROR', true)
      }
      let envelope: unknown
      try { envelope = await response.json() } catch {
        throw new PlatformApiError(response.ok ? 'Invalid server response' : `Request failed (${response.status})`, response.status, 'INVALID_RESPONSE')
      }
      if (!isObj(envelope) || envelope.success !== true || !('data' in envelope) || !response.ok) {
        const error = isObj(envelope) && isObj(envelope.error) ? envelope.error : undefined
        throw new PlatformApiError(typeof error?.message === 'string' ? error.message : `Request failed (${response.status})`, response.status, typeof error?.code === 'string' ? error.code : undefined)
      }
      const data = envelope.data
      if (!isObj(data) || !exactKeys(data, ['items'], []) || !Array.isArray(data.items) || data.items.length > limit || !data.items.every(isRecharge) ||
        data.items.some(item => item.brand_id !== brandId || (memberId !== '' && item.member_id !== memberId))) {
        throw new PlatformApiError('Invalid recharge response', 0, 'INVALID_RESPONSE')
      }
      return data.items
    },
  }
}
