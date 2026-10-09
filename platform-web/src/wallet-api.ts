import { isWalletDTO, normalizeLedgerSnapshots, type SourceBuckets, type WalletDTO, type WalletSource, type WalletState } from '@lottery/shared'
import { PlatformApiError } from './platform-api'

export type Wallet = WalletDTO
export type { WalletSource, WalletState, SourceBuckets }

export interface SourceAllocation {
  source: WalletSource
  state: WalletState
  points: string
}

export interface LedgerEntry {
  id: string
  brand_id: string
  account_id: string
  member_id: string
  entry_type: string
  reference_type: string
  reference_id: string
  operation_key: string
  before_snapshot: SourceBuckets
  delta_snapshot: SourceBuckets
  after_snapshot: SourceBuckets
  source_allocation: SourceAllocation[]
  reason: string
  actor_type: string
  actor_id: string
  request_id: string
  reversal_of?: string
  version: number
  created_at: string
}

type Obj = Record<string, unknown>
const BASE = '/api/v1/platform/wallets'
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
const DATE_TIME = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/
const isObj = (value: unknown): value is Obj => Boolean(value && typeof value === 'object' && !Array.isArray(value))
const exactKeys = (value: Obj, required: readonly string[], optional: readonly string[] = []) => {
  const keys = Object.keys(value)
  return required.every(key => keys.includes(key)) && keys.every(key => required.includes(key) || optional.includes(key))
}
const isText = (value: unknown): value is string => typeof value === 'string'
const isUuid = (value: unknown): value is string => isText(value) && UUID.test(value)
const isDateTime = (value: unknown): value is string => isText(value) && DATE_TIME.test(value) && Number.isFinite(Date.parse(value))
const isAllocation = (value: unknown): value is SourceAllocation => isObj(value) && exactKeys(value, ['source', 'state', 'points']) &&
  ['recharge', 'winning', 'gift', 'commission'].includes(String(value.source)) &&
  ['available', 'manual_frozen', 'system_frozen', 'withdrawal'].includes(String(value.state)) && isText(value.points) && /^-?(0|[1-9]\d*)$/.test(value.points)

const ENTRY_KEYS = ['id', 'brand_id', 'account_id', 'member_id', 'entry_type', 'reference_type', 'reference_id', 'operation_key', 'before_snapshot', 'delta_snapshot', 'after_snapshot', 'source_allocation', 'reason', 'actor_type', 'actor_id', 'request_id', 'version', 'created_at'] as const
function validLedgerEntry(value: unknown, brandId: string, memberId: string): value is LedgerEntry {
  if (!isObj(value) || !exactKeys(value, ENTRY_KEYS, ['reversal_of']) || value.brand_id !== brandId || value.member_id !== memberId ||
    !isUuid(value.id) || !isUuid(value.brand_id) || !isUuid(value.account_id) || !isUuid(value.member_id) ||
    !isText(value.entry_type) || !isText(value.reference_type) || !isText(value.reference_id) || (value.reference_id !== '' && !isUuid(value.reference_id)) ||
    !isText(value.operation_key) || !Array.isArray(value.source_allocation) || !value.source_allocation.every(isAllocation) ||
    !isText(value.reason) || !isText(value.actor_type) || !isText(value.actor_id) || (value.actor_id !== '' && !isUuid(value.actor_id)) ||
    !isText(value.request_id) || !Number.isSafeInteger(value.version) || Number(value.version) < 1 || !isDateTime(value.created_at) ||
    ('reversal_of' in value && !isUuid(value.reversal_of))) return false
  return normalizeLedgerSnapshots(value) !== null
}

function invalid(label: string): never { throw new PlatformApiError(`Invalid ${label} response`, 0, 'INVALID_RESPONSE') }

export function createPlatformWalletApi(fetcher: typeof fetch = fetch) {
  async function get(brandId: string, path: string): Promise<unknown> {
    if (!brandId) throw new PlatformApiError('A brand must be selected before loading a wallet', 400, 'BRAND_REQUIRED')
    let response: Response
    try {
      response = await fetcher(`${BASE}${path}`, { method: 'GET', credentials: 'same-origin', headers: { Accept: 'application/json', 'X-Brand-ID': brandId } })
    } catch (cause) {
      throw new PlatformApiError(cause instanceof Error ? cause.message : 'Network request failed', 0, 'NETWORK_ERROR', true)
    }
    let envelope: unknown
    try { envelope = await response.json() } catch {
      throw new PlatformApiError(response.ok ? 'Invalid server response' : `Request failed (${response.status})`, response.status, 'INVALID_RESPONSE')
    }
    if (!isObj(envelope) || envelope.success !== true || envelope.data === undefined || !response.ok) {
      const error = isObj(envelope) && isObj(envelope.error) ? envelope.error : undefined
      throw new PlatformApiError(isText(error?.message) ? error.message : `Request failed (${response.status})`, response.status, isText(error?.code) ? error.code : undefined)
    }
    return envelope.data
  }

  return {
    async wallet(brandId: string, memberId: string): Promise<Wallet> {
      if (!memberId) throw new PlatformApiError('A member must be selected before loading a wallet', 400, 'MEMBER_REQUIRED')
      const value = await get(brandId, `/${encodeURIComponent(memberId)}`)
      if (!isWalletDTO(value) || value.brand_id !== brandId || value.member_id !== memberId) invalid('wallet')
      return value
    },
    async ledger(brandId: string, memberId: string, limit = 50, offset = 0): Promise<LedgerEntry[]> {
      if (!memberId) throw new PlatformApiError('A member must be selected before loading a wallet ledger', 400, 'MEMBER_REQUIRED')
      const value = await get(brandId, `/${encodeURIComponent(memberId)}/ledger?limit=${limit}&offset=${offset}`)
      if (!isObj(value) || !exactKeys(value, ['items']) || !Array.isArray(value.items) || value.items.length > limit || !value.items.every(item => validLedgerEntry(item, brandId, memberId))) invalid('wallet ledger')
      return value.items as LedgerEntry[]
    },
  }
}
