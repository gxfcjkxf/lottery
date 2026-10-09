import { PlatformApiError } from './platform-api'

const BASE = '/api/v1/platform'
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i
const DECIMAL = /^(0|[1-9][0-9]*)$/
const RATIO = /^(0|1|0\.[0-9]{0,5}[1-9])$/
type Obj = Record<string, unknown>

export interface AgentNode {
  id: string
  brand_id: string
  member_id: string
  parent_id: string | null
  depth: number
  path: string[]
  version: number
  config: { ratio: string; mode: string | null; status: string; can_create_children: boolean }
  effective_mode: string
  mode_source_agent_id: string | null
  policy_version: number
  parent_version: number | null
  created_by: string
  created_at: string
  updated_at: string
  audit_log_id?: string
}

export interface AgentPage {
  brand_id: string
  parent_id: string | null
  items: AgentNode[]
  limit: number
  offset: number
  total_count: string
}

export interface Cycle {
  id: string
  brand_id: string
  window_from: string
  window_to: string
  anchor_order_id: string
  calendar: { timezone: string; cycle: string; boundary_time: string; weekday: number | null; month_day: number | null; short_month: string }
  state: string
  version: number
  target_count: string
  scan_complete: boolean
  current_run_id: string | null
  current_generation: string | null
  evidence_epoch: string | null
  evidence_current: boolean
  calculated_count: string
  earning_count: string
  total_points: string
  created_by: string | null
  creation_actor_type: string
  reason: string
  created_at: string
  updated_at: string
  last_error_code: string | null
  creation_audit_log_id: string
}

export interface CyclePage { brand_id: string; items: Cycle[]; total_count: string; limit: number; offset: number }

export interface Earning {
  id: string
  brand_id: string
  cycle_id: string
  run_id: string
  agent_id: string
  member_id: string
  exact_amount: { numerator: string; denominator: string }
  points: string
  created_at: string
}

export interface EarningPage { brand_id: string; cycle_id: string; items: Earning[]; total_count: string; limit: number; offset: number }

export interface Payment {
  id: string
  brand_id: string
  cycle_id: string
  run_id: string
  state: string
  payout_mode: string
  version: number
  evidence_epoch: string
  total_points: string
  paid_points: string
  target_count: string
  paid_count: string
  creation_audit_log_id: string
  last_error_code: string | null
  created_at: string
  updated_at: string
}

export interface PaymentPage { brand_id: string; items: Payment[]; total_count: string; limit: number; offset: number }

function object(value: unknown): value is Obj { return Boolean(value && typeof value === 'object' && !Array.isArray(value)) }
function string(value: unknown): value is string { return typeof value === 'string' }
function nonempty(value: unknown): value is string { return string(value) && value.length > 0 }
function uuid(value: unknown): value is string { return string(value) && UUID.test(value) }
function integer(value: unknown, min = 0): value is number { return Number.isSafeInteger(value) && Number(value) >= min }
function decimal(value: unknown): value is string { return string(value) && DECIMAL.test(value) }
function oneOf(value: unknown, values: string[]): value is string { return string(value) && values.includes(value) }
function dateTime(value: unknown): value is string {
  return string(value) && /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?(?:Z|[+-]\d\d:\d\d)$/.test(value) && Number.isFinite(Date.parse(value))
}
function sameBrand(value: unknown, brandId: string): boolean { return typeof value === 'string' && value.toLowerCase() === brandId.toLowerCase() }
function invalid(label: string): never { throw new PlatformApiError(`Invalid ${label} response`, 0, 'INVALID_RESPONSE') }
function input(message: string, code = 'REQUEST_INVALID'): never { throw new PlatformApiError(message, 400, code) }

function pageArgs(limit: number, offset: number) {
  if (!integer(limit, 1) || limit > 100 || !integer(offset) || offset > 1_000_000) input('Invalid commission pagination')
}
function brandArg(brandId: string) {
  if (!uuid(brandId)) input('A valid brand must be selected before loading commission data', 'BRAND_REQUIRED')
}
function idArg(id: string, label: string, canonical = false) {
  if (!uuid(id) || canonical && id !== id.toLowerCase()) input(`A valid ${label} ID is required`)
}
function isAgent(value: unknown, brandId: string): value is AgentNode {
  if (!object(value) || !uuid(value.id) || !sameBrand(value.brand_id, brandId) || !uuid(value.member_id) ||
    !(value.parent_id === null || uuid(value.parent_id)) || !integer(value.depth) || !Array.isArray(value.path) ||
    !value.path.every(uuid) || value.path.length !== value.depth || value.path.at(-1) !== value.id ||
    !integer(value.version, 1) || !object(value.config) || !string(value.config.ratio) || !RATIO.test(value.config.ratio) ||
    !(value.config.mode === null || oneOf(value.config.mode, ['loss', 'turnover'])) || !oneOf(value.config.status, ['active', 'disabled']) || typeof value.config.can_create_children !== 'boolean' ||
    !oneOf(value.effective_mode, ['loss', 'turnover']) || !(value.mode_source_agent_id === null || uuid(value.mode_source_agent_id)) ||
    !integer(value.policy_version, 1) || !(value.parent_version === null || integer(value.parent_version, 1)) ||
    !uuid(value.created_by) || !dateTime(value.created_at) || !dateTime(value.updated_at) ||
    (value.audit_log_id !== undefined && !uuid(value.audit_log_id))) return false
  if (value.parent_id === null ? value.depth !== 1 : value.path.at(-2) !== value.parent_id) return false
  return true
}
function isCycle(value: unknown, brandId: string): value is Cycle {
  return object(value) && uuid(value.id) && sameBrand(value.brand_id, brandId) && dateTime(value.window_from) && dateTime(value.window_to) &&
    uuid(value.anchor_order_id) && object(value.calendar) && nonempty(value.calendar.timezone) &&
    oneOf(value.calendar.cycle, ['weekly', 'monthly']) && typeof value.calendar.boundary_time === 'string' &&
    /^\d\d:\d\d:\d\d$/.test(value.calendar.boundary_time) &&
    (value.calendar.cycle === 'weekly'
      ? integer(value.calendar.weekday, 0) && value.calendar.weekday <= 6 && value.calendar.month_day === null && value.calendar.short_month === ''
      : value.calendar.weekday === null && integer(value.calendar.month_day, 1) && value.calendar.month_day <= 31 && oneOf(value.calendar.short_month, ['last_day', 'skip'])) &&
    oneOf(value.state, ['enumerating', 'waiting', 'calculating', 'summarizing', 'ready', 'failed']) && integer(value.version, 1) && decimal(value.target_count) && typeof value.scan_complete === 'boolean' &&
    (value.current_run_id === null || uuid(value.current_run_id)) && (value.current_generation === null || decimal(value.current_generation)) &&
    (value.evidence_epoch === null || decimal(value.evidence_epoch)) && typeof value.evidence_current === 'boolean' &&
    decimal(value.calculated_count) && decimal(value.earning_count) && decimal(value.total_points) &&
    (value.created_by === null || uuid(value.created_by)) && oneOf(value.creation_actor_type, ['admin', 'system']) && string(value.reason) &&
    dateTime(value.created_at) && dateTime(value.updated_at) && (value.last_error_code === null || string(value.last_error_code)) && uuid(value.creation_audit_log_id)
}
function isEarning(value: unknown, brandId: string, cycleId: string): value is Earning {
  return object(value) && uuid(value.id) && sameBrand(value.brand_id, brandId) && value.cycle_id === cycleId && uuid(value.run_id) &&
    uuid(value.agent_id) && uuid(value.member_id) && object(value.exact_amount) && decimal(value.exact_amount.numerator) &&
    decimal(value.exact_amount.denominator) && value.exact_amount.denominator !== '0' && decimal(value.points) && dateTime(value.created_at)
}
function isPayment(value: unknown, brandId: string): value is Payment {
  return object(value) && uuid(value.id) && sameBrand(value.brand_id, brandId) && uuid(value.cycle_id) && uuid(value.run_id) &&
    oneOf(value.state, ['awaiting_approval', 'blocked', 'paying', 'paid', 'failed', 'stale']) &&
    oneOf(value.payout_mode, ['manual', 'automatic', 'mixed', 'none']) && integer(value.version, 1) && decimal(value.evidence_epoch) &&
    decimal(value.total_points) && decimal(value.paid_points) && decimal(value.target_count) && decimal(value.paid_count) &&
    uuid(value.creation_audit_log_id) && (value.last_error_code === null || string(value.last_error_code)) &&
    dateTime(value.created_at) && dateTime(value.updated_at)
}

export function createPlatformCommissionApi(fetcher: typeof fetch = fetch) {
  async function get(path: string, brandId: string): Promise<unknown> {
    brandArg(brandId)
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
    if (!response.ok || !object(envelope) || envelope.success !== true || !('data' in envelope)) {
      const error = object(envelope) && object(envelope.error) ? envelope.error : undefined
      throw new PlatformApiError(typeof error?.message === 'string' ? error.message : `Request failed (${response.status})`, response.status, typeof error?.code === 'string' ? error.code : undefined)
    }
    return envelope.data
  }
  return {
    async tree(brandId: string, limit = 50, offset = 0): Promise<AgentPage> {
      pageArgs(limit, offset)
      const value = await get(`/agents/tree?limit=${limit}&offset=${offset}`, brandId)
      if (!object(value) || !sameBrand(value.brand_id, brandId) || value.parent_id !== null || !Array.isArray(value.items) ||
        value.items.length > limit || value.limit !== limit || value.offset !== offset || !decimal(value.total_count)) invalid('agent tree')
      return value.items.every(item => isAgent(item, brandId)) ? value as unknown as AgentPage : invalid('agent tree item')
    },
    async agent(brandId: string, id: string): Promise<AgentNode> {
      idArg(id, 'agent')
      const value = await get(`/agents/${encodeURIComponent(id)}`, brandId)
      return isAgent(value, brandId) && value.id.toLowerCase() === id.toLowerCase() ? value : invalid('agent')
    },
    async cycles(brandId: string, limit = 50, offset = 0): Promise<CyclePage> {
      pageArgs(limit, offset)
      const value = await get(`/commission-cycles?limit=${limit}&offset=${offset}`, brandId)
      if (!object(value) || !sameBrand(value.brand_id, brandId) || !Array.isArray(value.items) || value.items.length > limit ||
        value.limit !== limit || value.offset !== offset || !decimal(value.total_count) || !value.items.every(item => isCycle(item, brandId))) invalid('cycle page')
      return value as unknown as CyclePage
    },
    async cycle(brandId: string, id: string): Promise<Cycle> {
      idArg(id, 'cycle', true)
      const value = await get(`/commission-cycles/${encodeURIComponent(id)}`, brandId)
      return isCycle(value, brandId) && value.id === id ? value : invalid('cycle')
    },
    async earnings(brandId: string, cycleId: string, limit = 50, offset = 0): Promise<EarningPage> {
      idArg(cycleId, 'cycle', true)
      pageArgs(limit, offset)
      const value = await get(`/commission-cycles/${encodeURIComponent(cycleId)}/earnings?limit=${limit}&offset=${offset}`, brandId)
      if (!object(value) || !sameBrand(value.brand_id, brandId) || value.cycle_id !== cycleId || !Array.isArray(value.items) ||
        value.items.length > limit || value.limit !== limit || value.offset !== offset || !decimal(value.total_count) ||
        !value.items.every(item => isEarning(item, brandId, cycleId))) invalid('earning page')
      return value as unknown as EarningPage
    },
    async payments(brandId: string, limit = 50, offset = 0): Promise<PaymentPage> {
      pageArgs(limit, offset)
      const value = await get(`/commission-payments?limit=${limit}&offset=${offset}`, brandId)
      if (!object(value) || !sameBrand(value.brand_id, brandId) || !Array.isArray(value.items) || value.items.length > limit ||
        value.limit !== limit || value.offset !== offset || !decimal(value.total_count) || !value.items.every(item => isPayment(item, brandId))) invalid('payment page')
      return value as unknown as PaymentPage
    },
    async payment(brandId: string, id: string): Promise<Payment> {
      idArg(id, 'payment', true)
      const value = await get(`/commission-payments/${encodeURIComponent(id)}`, brandId)
      return isPayment(value, brandId) && value.id === id ? value : invalid('payment')
    },
  }
}
