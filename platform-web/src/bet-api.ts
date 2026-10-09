import { PlatformApiError } from './platform-api'
import type { RuleDefinition, RuleTicketSelection } from '../../shared/src/rules'

export interface BetOrder {
  id: string
  brand_id: string
  global_user_id: string
  brand_member_id: string
  account_id: string
  game_id: string
  period_id: string
  play_id: string
  rule_version_id: string
  definition_hash: string
  definition_snapshot: RuleDefinition
  status: 'placed' | 'bet_cancelled' | 'judged_cancelled' | 'abnormal' | 'won' | 'lost'
  version: number
  selection_raw: RuleTicketSelection
  selection_normalized: RuleTicketSelection
  expanded_bets: RuleTicketSelection[]
  unit_points: string
  combination_count: number
  multiplier: string
  total_points: string
  deduction_allocation: { source: string; state: string; points: string }[]
  policy_snapshot: { min_bet_points: string; max_bet_points: string | null; max_period_points: string | null; max_user_period_points: string | null; user_cancel_allowed: boolean }
  policy_versions: { brand: number; game: number }
  debit_entry_id: string
  refund_entry_id?: string
  client_key: string
  placed_at: string
  cancelled_at?: string | null
  cancel_reason?: string
  settlement_calculation_id: string | null
  payout_entry_id: string | null
  prize_points: string
  settled_at: string | null
}

const BASE = '/api/v1/platform/bet-orders'
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
type Obj = Record<string, unknown>
const isObj = (v: unknown): v is Obj => Boolean(v && typeof v === 'object' && !Array.isArray(v))
const uuid = (v: unknown): v is string => typeof v === 'string' && UUID.test(v)
const text = (v: unknown): v is string => typeof v === 'string'
const integerString = (v: unknown): v is string => typeof v === 'string' && /^(0|[1-9][0-9]*)$/.test(v) && BigInt(v) <= 9223372036854775807n
const positiveIntegerString = (v: unknown): v is string => integerString(v) && v !== '0'
const dateTime = (v: unknown): v is string => text(v) && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.test(v) && Number.isFinite(Date.parse(v))
const exactKeys = (value: Obj, names: readonly string[]) => Object.keys(value).length === names.length && names.every(name => name in value)
const orderKeys = ['id','brand_id','global_user_id','brand_member_id','account_id','game_id','period_id','play_id','rule_version_id','definition_hash','definition_snapshot','status','version','selection_raw','selection_normalized','expanded_bets','unit_points','combination_count','multiplier','total_points','deduction_allocation','policy_snapshot','policy_versions','debit_entry_id','client_key','placed_at']
orderKeys.push('settlement_calculation_id', 'payout_entry_id', 'prize_points', 'settled_at')
const optionalKeys = ['refund_entry_id','cancelled_at','cancel_reason']

function validSelection(v: unknown): v is RuleTicketSelection {
  if (!isObj(v) || !exactKeys(v, ['regular','special','digits','exclude','attributes','features'])) return false
  const nums = (n: unknown): boolean => n === null || Array.isArray(n) && n.every(x => Number.isSafeInteger(x) && Number(x) >= 0)
  return nums(v.regular) && nums(v.special) && nums(v.exclude) && (v.digits === null || Array.isArray(v.digits) && v.digits.every(nums)) &&
    (v.attributes === null || isObj(v.attributes) && Object.values(v.attributes).every(x => Array.isArray(x) && x.every(text))) &&
    (v.features === null || isObj(v.features) && Object.values(v.features).every(nums))
}
function validDefinition(v: unknown): v is RuleDefinition {
  if (!isObj(v) || !exactKeys(v, ['schema_version','model','selection','number_attributes','unit_points','prize_tiers','mixed_tier_policy','cap_points','rounding','rounding_scope','limits'])) return false
  const model = v.model, selection = v.selection, limits = v.limits
  if (!isObj(model) || !isObj(model.regular_pool) || !isObj(model.special_pool) || !isObj(selection) || !isObj(limits)) return false
  const pool = (p: Obj) => exactKeys(p, ['allow_repeat']) && typeof p.allow_repeat === 'boolean' ||
    ['allow_repeat'].every(k => k in p) && ['min','max','values'].every(k => !(k in p) || (k === 'values' ? p[k] === null || Array.isArray(p[k]) && p[k].every(Number.isSafeInteger) : Number.isSafeInteger(p[k]))) && typeof p.allow_repeat === 'boolean'
  return v.schema_version === 1 && text(model.model) && pool(model.regular_pool) && pool(model.special_pool) &&
    ['regular_count','special_count','pool_size','total_count','length'].every(k => Number.isSafeInteger(model[k])) && typeof model.allow_repeat === 'boolean' && typeof model.ordered === 'boolean' &&
    text(selection.mode) && ['regular_count','special_count','exclude_count'].every(k => Number.isSafeInteger(selection[k])) &&
    (selection.attribute_groups === null || Array.isArray(selection.attribute_groups) && selection.attribute_groups.every(text)) &&
    (selection.feature_choices === null || isObj(selection.feature_choices) && Object.values(selection.feature_choices).every(x => Array.isArray(x) && x.every(Number.isSafeInteger))) &&
    positiveIntegerString(v.unit_points) && (v.number_attributes === null || isObj(v.number_attributes)) && (v.prize_tiers === null || Array.isArray(v.prize_tiers)) &&
    text(v.mixed_tier_policy) && (v.cap_points === null || positiveIntegerString(v.cap_points)) && text(v.rounding) && text(v.rounding_scope) &&
    exactKeys(limits, ['max_combinations','max_multiplier','max_bet_points']) && Number.isSafeInteger(limits.max_combinations) && positiveIntegerString(limits.max_multiplier) && (limits.max_bet_points === null || positiveIntegerString(limits.max_bet_points))
}
function validOrder(v: unknown, brand: string, member?: string): v is BetOrder {
  if (!isObj(v) || Object.keys(v).some(k => !orderKeys.includes(k) && !optionalKeys.includes(k)) || !orderKeys.every(k => k in v) ||
    v.brand_id !== brand || member !== undefined && v.brand_member_id !== member || !uuid(v.id) || !uuid(v.brand_id) || !uuid(v.global_user_id) || !uuid(v.brand_member_id) ||
    !uuid(v.account_id) || !uuid(v.game_id) || !uuid(v.period_id) || !uuid(v.play_id) || !uuid(v.rule_version_id) || !/^[0-9a-f]{64}$/i.test(String(v.definition_hash)) ||
    !validDefinition(v.definition_snapshot) || !['placed','bet_cancelled','judged_cancelled','abnormal','won','lost'].includes(String(v.status)) || !Number.isSafeInteger(v.version) || Number(v.version) < 1 ||
    !validSelection(v.selection_raw) || !validSelection(v.selection_normalized) || !Array.isArray(v.expanded_bets) || !v.expanded_bets.every(validSelection) ||
    !positiveIntegerString(v.unit_points) || !Number.isSafeInteger(v.combination_count) || Number(v.combination_count) < 1 || !positiveIntegerString(v.multiplier) || !positiveIntegerString(v.total_points) ||
    !Array.isArray(v.deduction_allocation) || !v.deduction_allocation.every(a => isObj(a) && exactKeys(a,['source','state','points']) && ['recharge','winning','gift','commission'].includes(String(a.source)) && ['available','manual_frozen','system_frozen','withdrawal'].includes(String(a.state)) && positiveIntegerString(a.points)) ||
    !isObj(v.policy_snapshot) || !exactKeys(v.policy_snapshot,['min_bet_points','max_bet_points','max_period_points','max_user_period_points','user_cancel_allowed']) || !positiveIntegerString(v.policy_snapshot.min_bet_points) ||
    ![v.policy_snapshot.max_bet_points,v.policy_snapshot.max_period_points,v.policy_snapshot.max_user_period_points].every(x => x === null || positiveIntegerString(x)) || typeof v.policy_snapshot.user_cancel_allowed !== 'boolean' ||
    !isObj(v.policy_versions) || !exactKeys(v.policy_versions,['brand','game']) || !Number.isSafeInteger(v.policy_versions.brand) || Number(v.policy_versions.brand) < 1 || !Number.isSafeInteger(v.policy_versions.game) || Number(v.policy_versions.game) < 1 ||
    !uuid(v.debit_entry_id) || !text(v.client_key) || !dateTime(v.placed_at)) return false
  if ('refund_entry_id' in v && !uuid(v.refund_entry_id) || 'cancelled_at' in v && !(v.cancelled_at === null || dateTime(v.cancelled_at)) || 'cancel_reason' in v && !text(v.cancel_reason) ||
    'settlement_calculation_id' in v && !(v.settlement_calculation_id === null || uuid(v.settlement_calculation_id)) || 'payout_entry_id' in v && !(v.payout_entry_id === null || uuid(v.payout_entry_id)) ||
    !integerString(v.prize_points) || 'settled_at' in v && !(v.settled_at === null || dateTime(v.settled_at))) return false
  return true
}

export function createPlatformBetApi(fetcher: typeof fetch = fetch) {
  async function get(brandId: string, path: string): Promise<unknown> {
    if (!brandId.trim()) throw new PlatformApiError('A brand must be selected before loading bet orders', 400, 'BRAND_REQUIRED')
    let response: Response
    try { response = await fetcher(path, { method: 'GET', credentials: 'same-origin', headers: { Accept: 'application/json', 'X-Brand-ID': brandId } }) }
    catch (cause) { throw new PlatformApiError(cause instanceof Error ? cause.message : 'Network request failed', 0, 'NETWORK_ERROR', true) }
    let envelope: unknown
    try { envelope = await response.json() } catch { throw new PlatformApiError(response.ok ? 'Invalid server response' : `Request failed (${response.status})`, response.status, 'INVALID_RESPONSE') }
    if (!isObj(envelope) || envelope.success !== true || envelope.data === undefined || !response.ok) {
      const error = isObj(envelope) && isObj(envelope.error) ? envelope.error : undefined
      throw new PlatformApiError(text(error?.message) ? error.message : `Request failed (${response.status})`, response.status, text(error?.code) ? error.code : undefined)
    }
    return envelope.data
  }
  return {
    async list(brandId: string, memberId = '', limit = 51, offset = 0): Promise<BetOrder[]> {
      const query = new URLSearchParams({ limit: String(limit), offset: String(offset) })
      if (memberId) query.set('member_id', memberId)
      const value = await get(brandId, `${BASE}?${query}`)
      if (!isObj(value) || !exactKeys(value, ['items']) || !Array.isArray(value.items) || value.items.length > limit || !value.items.every(item => validOrder(item, brandId, memberId || undefined))) throw new PlatformApiError('Invalid bet orders response', 0, 'INVALID_RESPONSE')
      if (new Set(value.items.map(item => (item as BetOrder).id)).size !== value.items.length) throw new PlatformApiError('Invalid bet orders response', 0, 'INVALID_RESPONSE')
      return value.items as BetOrder[]
    },
    async read(brandId: string, orderId: string): Promise<BetOrder> {
      if (!uuid(orderId)) throw new PlatformApiError('A valid order ID is required', 400, 'INVALID_INPUT')
      const value = await get(brandId, `${BASE}/${encodeURIComponent(orderId)}`)
      if (!validOrder(value, brandId) || value.id !== orderId) throw new PlatformApiError('Invalid bet order response', 0, 'INVALID_RESPONSE')
      return value
    },
  }
}
