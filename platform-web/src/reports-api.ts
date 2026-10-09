import { PlatformApiError } from './platform-api'

export type ReportKind = 'betting' | 'ledger' | 'withdrawal' | 'commission' | 'rewards' | 'reward_orders'

export interface ReportQuery {
  from: string
  to: string
  group_by: string
  limit?: number
  offset?: number
  member_id?: string
  game_id?: string
  agent_id?: string
  cycle_id?: string
  order_id?: string
}

export const REPORT_FIELDS: Record<ReportKind, readonly string[]> = {
  betting: ['order_count', 'stake_points', 'placed_count', 'won_count', 'lost_count', 'abnormal_count', 'cancelled_count', 'refund_points', 'settled_stake_points', 'unfinalized_stake_points', 'abnormal_stake_points', 'current_prize_points', 'correction_open_count'],
  ledger: ['entry_count', 'net_points', 'recharge_points', 'prize_credit_points', 'prize_reversal_points', 'refund_points'],
  withdrawal: ['order_count', 'requested_points', 'reviewing_count', 'reviewing_points', 'processing_count', 'processing_points', 'paid_count', 'paid_points', 'rejected_count', 'rejected_points', 'failed_count', 'failed_points', 'cancelled_count', 'cancelled_points'],
  commission: ['entry_count', 'paid_entry_count', 'paid_points', 'adjustment_entry_count', 'adjustment_credit_points', 'adjustment_debit_points', 'correction_entry_count', 'correction_credit_points', 'correction_debit_points', 'net_points'],
  rewards: ['entry_count', 'grant_entry_count', 'grant_points', 'reversal_entry_count', 'reversal_points', 'net_points'],
  reward_orders: ['order_count', 'original_points', 'granted_count', 'granted_points', 'pending_count', 'pending_points', 'revoked_count', 'revoked_points'],
}

export const REPORT_GROUPS: Record<ReportKind, readonly string[]> = {
  betting: ['day', 'game', 'member'],
  ledger: ['day', 'entry_type'],
  withdrawal: ['day', 'member', 'state'],
  commission: ['day', 'agent', 'cycle'],
  rewards: ['day', 'member', 'order'],
  reward_orders: ['day', 'member', 'state'],
}

export interface ReportResult {
  brand_id: string
  snapshot_at: string
  timezone: string
  query: {
    from: string
    to: string
    group_by: string
    limit: number
    offset: number
    game_id?: string | null
    member_id?: string | null
    agent_id?: string | null
    cycle_id?: string | null
    order_id?: string | null
  }
  summary: Record<string, string>
  items: { key: string; label: string; totals: Record<string, string> }[]
  total_groups: string
  balances?: {
    account_count: string
    available_points: string
    frozen_points: string
    withdrawal_points: string
    total_points: string
  }
}

const BASE = '/api/v1/platform/reports'
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i
const UINT = /^(0|[1-9][0-9]*)$/
const SINT = /^(0|-?[1-9][0-9]*)$/
const UTC_SECOND = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})Z$/
const SNAPSHOT = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/
const FILTERS: Record<ReportKind, readonly string[]> = {
  betting: ['game_id', 'member_id'],
  ledger: ['member_id'],
  withdrawal: ['member_id'],
  commission: ['agent_id', 'member_id', 'cycle_id'],
  rewards: ['member_id', 'order_id'],
  reward_orders: ['member_id', 'order_id'],
}
type Obj = Record<string, unknown>
const isObj = (value: unknown): value is Obj => !!value && typeof value === 'object' && !Array.isArray(value)
const exact = (value: Obj, keys: readonly string[]) => Object.keys(value).length === keys.length && Object.keys(value).every(key => keys.includes(key))
function invalidInput(message: string): never { throw new PlatformApiError(message, 0, 'INVALID_INPUT') }
function invalidResponse(): never { throw new PlatformApiError('Invalid report response', 502, 'INVALID_RESPONSE') }

function utcSecond(value: unknown): number {
  if (typeof value !== 'string') return invalidInput('Report dates must be UTC timestamps with whole-second precision')
  const match = UTC_SECOND.exec(value)
  if (!match) return invalidInput('Report dates must be UTC timestamps with whole-second precision')
  const [, ys, mos, ds, hs, mis, ss] = match
  const year = Number(ys), month = Number(mos), day = Number(ds), hour = Number(hs), minute = Number(mis), second = Number(ss)
  const date = new Date(`${ys}-${mos}-${ds}T${hs}:${mis}:${ss}Z`)
  if (year < 1 || month < 1 || month > 12 || day < 1 || hour > 23 || minute > 59 || second > 59 ||
    date.getUTCFullYear() !== year || date.getUTCMonth() + 1 !== month || date.getUTCDate() !== day) {
    return invalidInput('Report date is invalid')
  }
  return date.getTime()
}

function normalizeQuery(kind: ReportKind, query: ReportQuery) {
  if (!REPORT_FIELDS[kind]) invalidInput('Unsupported report kind')
  if (!isObj(query) || Object.keys(query).some(key => !['from', 'to', 'group_by', 'limit', 'offset', ...FILTERS[kind]].includes(key))) invalidInput('Unsupported report query field')
  const fromMs = utcSecond(query.from), toMs = utcSecond(query.to)
  if (toMs <= fromMs || toMs - fromMs > 93 * 24 * 60 * 60 * 1000) invalidInput('Report period must be positive and at most 93 days')
  if (typeof query.group_by !== 'string' || !REPORT_GROUPS[kind].includes(query.group_by)) invalidInput('Unsupported report group')
  const limit = query.limit ?? 20, offset = query.offset ?? 0
  if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000) invalidInput('Invalid report pagination')
  const filters: Record<string, string | null> = {}
  for (const name of FILTERS[kind]) {
    const value = query[name as keyof ReportQuery]
    if (value !== undefined) {
      if (typeof value !== 'string' || !UUID.test(value)) invalidInput(`${name} must be a UUID`)
      filters[name] = value.toLowerCase()
    } else filters[name] = null
  }
  return { from: query.from, to: query.to, group_by: query.group_by, limit, offset, filters }
}

function reportQueryKeys(kind: ReportKind): string[] {
  if (kind === 'betting' || kind === 'ledger' || kind === 'withdrawal') return ['from', 'to', 'group_by', 'limit', 'offset', 'game_id', 'member_id']
  if (kind === 'commission') return ['from', 'to', 'group_by', 'limit', 'offset', 'agent_id', 'member_id', 'cycle_id']
  return ['from', 'to', 'group_by', 'limit', 'offset', 'member_id', 'order_id']
}

function expectedEcho(kind: ReportKind, q: ReturnType<typeof normalizeQuery>): Obj {
  const echo: Obj = { from: q.from, to: q.to, group_by: q.group_by, limit: q.limit, offset: q.offset }
  for (const key of reportQueryKeys(kind).slice(5)) echo[key] = q.filters[key] ?? null
  return echo
}

function validTimestamp(value: unknown): value is string {
  return typeof value === 'string' && SNAPSHOT.test(value) && Number.isFinite(Date.parse(value))
}

function validateTotals(value: unknown, kind: ReportKind): Record<string, string> {
  const fields = REPORT_FIELDS[kind]
  if (!isObj(value) || !exact(value, fields) || fields.some(field => typeof value[field] !== 'string' || !(field === 'net_points' ? SINT : UINT).test(value[field] as string))) invalidResponse()
  return value as Record<string, string>
}

function validateReport(value: unknown, brandId: string, kind: ReportKind, q: ReturnType<typeof normalizeQuery>): ReportResult {
  const topKeys = ['brand_id', 'snapshot_at', 'timezone', 'query', 'summary', 'items', 'total_groups', ...(kind === 'ledger' ? ['balances'] : [])]
  if (!isObj(value) || !exact(value, topKeys) || value.brand_id !== brandId || !validTimestamp(value.snapshot_at) ||
    typeof value.timezone !== 'string' || value.timezone.length === 0 || value.timezone.length > 100 ||
    !isObj(value.query) || !Array.isArray(value.items) || typeof value.total_groups !== 'string' || !UINT.test(value.total_groups)) invalidResponse()
  try { new Intl.DateTimeFormat('en', { timeZone: value.timezone }).format(0) } catch { invalidResponse() }
  const echo = value.query
  const wanted = expectedEcho(kind, q)
  if (!exact(echo, Object.keys(wanted)) || Object.keys(wanted).some(key => echo[key] !== wanted[key])) invalidResponse()
  const summary = validateTotals(value.summary, kind)
  if (value.items.length > q.limit) invalidResponse()
  const totalGroups = BigInt(value.total_groups)
  const remainingGroups = totalGroups > BigInt(q.offset) ? totalGroups - BigInt(q.offset) : 0n
  if (BigInt(value.items.length) > remainingGroups) invalidResponse()
  const seen = new Set<string>()
  let previous = ''
  const items = value.items.map((raw: unknown) => {
    if (!isObj(raw) || !exact(raw, ['key', 'label', 'totals']) || typeof raw.key !== 'string' || !raw.key ||
      typeof raw.label !== 'string' || raw.label.length > 200 || /[\u0000-\u001f\u007f]/.test(raw.label)) invalidResponse()
    const key = raw.key
    if (seen.has(key) || previous && key <= previous) invalidResponse()
    seen.add(key); previous = key
    if (q.group_by === 'day' && (!/^\d{4}-\d{2}-\d{2}$/.test(key) || raw.label !== key)) invalidResponse()
    if (['game', 'member', 'agent', 'cycle', 'order'].includes(q.group_by) && !UUID.test(key)) invalidResponse()
    if (q.group_by === 'entry_type' && (!/^[a-zA-Z0-9_.:-]{1,200}$/.test(key) || raw.label !== key)) invalidResponse()
    if (q.group_by === 'state' && !(kind === 'withdrawal' ? ['reviewing', 'processing', 'paid', 'rejected', 'failed', 'cancelled'] : ['granted', 'revocation_pending', 'revoked']).includes(key)) invalidResponse()
    if (['member', 'order', 'game', 'agent', 'cycle'].includes(q.group_by) && q.filters[`${q.group_by}_id`] && key.toLowerCase() !== q.filters[`${q.group_by}_id`]) invalidResponse()
    if (q.group_by === 'member' && raw.label !== key) invalidResponse()
    return { key, label: raw.label, totals: validateTotals(raw.totals, kind) }
  })
  let balances: ReportResult['balances']
  if (kind === 'ledger') {
    const raw = value.balances
    const balanceFields = ['account_count', 'available_points', 'frozen_points', 'withdrawal_points', 'total_points'] as const
    if (!isObj(raw) || !exact(raw, balanceFields) || balanceFields.some(field => typeof raw[field] !== 'string' || !UINT.test(raw[field] as string))) invalidResponse()
    balances = raw as ReportResult['balances']
  }
  return { brand_id: brandId, snapshot_at: value.snapshot_at, timezone: value.timezone, query: echo as ReportResult['query'], summary, items, total_groups: value.total_groups, ...(balances ? { balances } : {}) }
}

export function createPlatformReportsApi(fetcher: typeof fetch = fetch) {
  return {
    async read(brandId: string, kind: ReportKind, query: ReportQuery): Promise<ReportResult> {
      if (typeof brandId !== 'string' || !UUID.test(brandId)) invalidInput('A valid brand UUID is required')
      if (!Object.hasOwn(REPORT_FIELDS, kind)) invalidInput('Unsupported report kind')
      const brand = brandId.toLowerCase(), q = normalizeQuery(kind, query)
      const params = new URLSearchParams({ from: q.from, to: q.to, group_by: q.group_by, limit: String(q.limit), offset: String(q.offset) })
      for (const [key, value] of Object.entries(q.filters)) if (value !== null) params.set(key, value)
      const segment = kind === 'reward_orders' ? 'reward-orders' : kind
      let response: Response
      try {
        response = await fetcher(`${BASE}/${segment}?${params}`, { method: 'GET', credentials: 'same-origin', headers: { Accept: 'application/json', 'X-Brand-ID': brand } })
      } catch (cause) {
        throw new PlatformApiError(cause instanceof Error ? cause.message : 'Network request failed', 0, 'NETWORK_ERROR', true)
      }
      let envelope: unknown
      try { envelope = await response.json() } catch {
        throw new PlatformApiError(response.ok ? 'Invalid server response' : `Request failed (${response.status})`, response.status, 'INVALID_RESPONSE')
      }
      if (!response.ok || !isObj(envelope) || envelope.success !== true || envelope.data === undefined) {
        const error = isObj(envelope) && isObj(envelope.error) ? envelope.error : undefined
        throw new PlatformApiError(typeof error?.message === 'string' ? error.message : `Request failed (${response.status})`, response.status, typeof error?.code === 'string' ? error.code : undefined)
      }
      return validateReport(envelope.data, brand, kind, q)
    },
  }
}
