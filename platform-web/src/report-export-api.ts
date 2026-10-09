import { PlatformApiError } from './platform-api'
import { REPORT_FIELDS, normalizeReportQuery, type ReportKind, type ReportQuery } from './reports-api'

const BASE = '/api/v1/platform/reports'
const MAX_BYTES = 4 * 1024 * 1024
const MAX_GROUPS = 10_000
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i
const UINT = /^(0|[1-9][0-9]*)$/
const SINT = /^(0|-?[1-9][0-9]*)$/
const TIMESTAMP = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/

export type ReportExportQuery = Omit<ReportQuery, 'limit' | 'offset'>
export interface PlatformReportExport {
  filename: string
  bytes: Uint8Array
  groupCount: string
  snapshotAt: string
  auditLogId: string
}

const metadataColumns = ['record_type','brand_id','snapshot_at','timezone','from','to','group_by'] as const
const columns: Record<ReportKind, readonly string[]> = {
  betting: [...metadataColumns,'game_id','member_id','key','label',...REPORT_FIELDS.betting],
  ledger: [...metadataColumns,'game_id','member_id','key','label',...REPORT_FIELDS.ledger,'account_count','available_points','frozen_points','withdrawal_points','total_points'],
  withdrawal: [...metadataColumns,'member_id','key','label',...REPORT_FIELDS.withdrawal],
  commission: [...metadataColumns,'agent_id','member_id','cycle_id','key','label',...REPORT_FIELDS.commission],
  rewards: [...metadataColumns,'member_id','order_id','key','label',...REPORT_FIELDS.rewards],
  reward_orders: [...metadataColumns,'member_id','order_id','key','label',...REPORT_FIELDS.reward_orders],
}
const metricStart: Record<ReportKind, number> = { betting: 11, ledger: 11, withdrawal: 10, commission: 12, rewards: 11, reward_orders: 11 }
const signedMetric: Partial<Record<ReportKind, string>> = { ledger: 'net_points', commission: 'net_points', rewards: 'net_points' }

function fail(message = 'Invalid report export response'): never {
  throw new PlatformApiError(message, 502, 'INVALID_RESPONSE')
}
function badInput(message: string): never { throw new PlatformApiError(message, 0, 'INVALID_INPUT') }
function parseCsv(text: string): string[][] {
  const rows: string[][] = []
  let row: string[] = [], cell = '', quoted = false, closed = false
  for (let i = 0; i < text.length; i++) {
    const ch = text[i]!
    if (quoted) {
      if (ch === '"') {
        if (text[i + 1] === '"') { cell += '"'; i++ }
        else { quoted = false; closed = true }
      } else cell += ch
    } else if (closed && ch !== ',' && ch !== '\n') fail()
    else if (ch === '"') {
      if (cell) fail()
      quoted = true
    } else if (ch === ',') { row.push(cell); cell = ''; closed = false }
    else if (ch === '\n') { row.push(cell); rows.push(row); row = []; cell = ''; closed = false }
    else { if (ch === '\r') fail(); cell += ch }
  }
  if (quoted) fail()
  if (cell || row.length || closed) { row.push(cell); rows.push(row) }
  if (rows.length && rows.at(-1)!.length === 1 && rows.at(-1)![0] === '') rows.pop()
  return rows
}
async function readBytes(response: Response): Promise<Uint8Array> {
  if (!response.body) fail()
  const reader = response.body.getReader()
  const chunks: Uint8Array[] = []
  let length = 0
  try {
    for (;;) {
      const part = await reader.read()
      if (part.done) break
      length += part.value.length
      if (length > MAX_BYTES) {
        await reader.cancel()
        fail('Report export exceeds the complete byte limit')
      }
      chunks.push(part.value)
    }
  } catch (cause) {
    if (cause instanceof PlatformApiError) throw cause
    throw new PlatformApiError(cause instanceof Error ? cause.message : 'Network request failed', 0, 'NETWORK_ERROR', true)
  }
  const bytes = new Uint8Array(length)
  let offset = 0
  for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.length }
  return bytes
}
function header(headers: Headers, name: string): string {
  const value = headers.get(name)
  if (!value) fail(`Missing ${name} response header`)
  return value
}
function parseTimestamp(value: string): boolean {
  if (!TIMESTAMP.test(value)) return false
  const millis = Date.parse(value)
  return Number.isFinite(millis) && new Date(millis).toISOString().slice(0, 19) === value.slice(0, 19)
}
function formulaValue(value: string): string {
  const isFormula = (raw: string) => {
    const lead = raw.replace(/^[\p{White_Space}\p{Cc}\ufeff]+/u, '')
    return lead.length > 0 && '=+-@'.includes(lead[0]!)
  }
  if (value.startsWith("'") && isFormula(value.slice(1))) return value.slice(1)
  if (isFormula(value)) fail()
  return value
}
function groupKeyValid(kind: ReportKind, groupBy: string, key: string): boolean {
  if (!key) return false
  if (groupBy === 'day') return /^\d{4}-\d{2}-\d{2}$/.test(key) && Number.isFinite(Date.parse(`${key}T00:00:00Z`)) && new Date(`${key}T00:00:00Z`).toISOString().slice(0, 10) === key
  if (groupBy === 'entry_type') return kind === 'ledger' && /^[a-zA-Z0-9_.:-]{1,200}$/.test(key)
  if (groupBy === 'state') return kind === 'withdrawal'
    ? ['reviewing','processing','paid','rejected','failed','cancelled'].includes(key)
    : ['granted','revocation_pending','revoked'].includes(key)
  if (['game','member','agent','cycle','order'].includes(groupBy)) return UUID.test(key)
  return false
}

function validateCsv(kind: ReportKind, brand: string, q: ReturnType<typeof normalizeReportQuery>, headers: Headers, rows: string[][], groupCount: string, snapshot: string): void {
  const expected = columns[kind]
  const metricCount = expected.length - metricStart[kind] - (kind === 'ledger' ? 5 : 0)
  if (!rows.length || rows[0]!.length !== expected.length || rows[0]!.some((v, i) => v !== expected[i])) fail()
  const groupCountBig = BigInt(groupCount)
  if (groupCountBig > BigInt(MAX_GROUPS)) fail('Report export exceeds the complete group limit')
  const expectedRows = groupCountBig + (kind === 'ledger' ? 2n : 1n)
  if (expectedRows !== BigInt(rows.length - 1)) fail()
  const filters = q.filters
  let groups = 0
  let priorKey = ''
  let timezone = ''
  for (let n = 1; n < rows.length; n++) {
    const row = rows[n]!
    if (row.length !== expected.length) fail()
    const record = row[0]!
    const valueAt = (name: string) => row[expected.indexOf(name)]!
    if (valueAt('brand_id') !== brand || valueAt('snapshot_at') !== snapshot || !parseTimestamp(snapshot) ||
      !valueAt('timezone') || valueAt('timezone') === 'Local' || valueAt('from') !== q.from || valueAt('to') !== q.to || valueAt('group_by') !== q.group_by) fail('Report export metadata does not match the request')
    if (!timezone) timezone = valueAt('timezone')
    else if (timezone !== valueAt('timezone')) fail()
    if (headers.has('X-Report-Timezone') && valueAt('timezone') !== headers.get('X-Report-Timezone')) fail()
    try { new Intl.DateTimeFormat('en', { timeZone: valueAt('timezone') }) } catch { fail() }
    for (const name of ['game_id', 'member_id', 'agent_id', 'cycle_id', 'order_id']) {
      if (expected.includes(name)) {
        const wanted = filters[name] ?? ''
        const got = formulaValue(valueAt(name))
        if (got !== wanted) fail('Report export filters do not match the request')
      }
    }
    if (kind === 'withdrawal' && formulaValue(valueAt('member_id')) !== (filters.member_id ?? '')) fail()
    if (kind === 'ledger' && record === 'balances') {
      if (n !== 2 || groups !== 0 || valueAt('key') || valueAt('label') || expected.slice(metricStart[kind], metricStart[kind] + metricCount).some((_, i) => row[metricStart[kind] + i] !== '')) fail()
      const balances = row.slice(metricStart[kind] + metricCount)
      if (balances.length !== 5 || balances.some(v => !UINT.test(v))) fail()
      if (BigInt(balances[1]!) + BigInt(balances[2]!) + BigInt(balances[3]!) !== BigInt(balances[4]!)) fail()
      continue
    }
    if (record !== (n === 1 ? 'summary' : 'group')) fail()
    if (kind === 'ledger' && row.slice(metricStart[kind] + metricCount).some(v => v !== '')) fail()
    const key = formulaValue(valueAt('key')), label = formulaValue(valueAt('label'))
    if (n === 1) {
      if (key || label) fail()
    } else {
      groups++
      if (!groupKeyValid(kind, q.group_by, key) || (priorKey && key <= priorKey)) fail()
      if (['commission','rewards','reward_orders'].includes(kind) && label !== key) fail()
      if (['betting','ledger'].includes(kind) && q.group_by === 'day' && label !== key) fail()
      if (['member','game','agent','cycle','order'].includes(q.group_by) && filters[`${q.group_by}_id`] && key.toLowerCase() !== filters[`${q.group_by}_id`]) fail()
      if (q.group_by === 'member' && label !== key) fail()
      priorKey = key
    }
    const values = row.slice(metricStart[kind], metricStart[kind] + metricCount)
    values.forEach((raw, i) => {
      const field = expected[metricStart[kind] + i]!
      let numeric = raw
      if (field === 'net_points' && (kind === 'commission' || kind === 'rewards') && raw.startsWith("'-")) numeric = raw.slice(1)
      else if (field === 'net_points' && (kind === 'commission' || kind === 'rewards') && raw.startsWith('-')) fail()
      if (!(field === signedMetric[kind] ? SINT : UINT).test(numeric)) fail()
    })
  }
  if (groups !== Number(groupCountBig)) fail()
  // The summary row is authoritative; group rows must add up exactly using integers.
  const summary = rows[1]!
  const groupSums = expected.slice(metricStart[kind], metricStart[kind] + metricCount).map(() => 0n)
  for (const row of rows.slice(2)) {
    if (kind === 'ledger' && row[0] === 'balances') continue
    row.slice(metricStart[kind], metricStart[kind] + metricCount).forEach((raw, i) => {
      const field = expected[metricStart[kind] + i]!
      const numeric = field === 'net_points' && (kind === 'commission' || kind === 'rewards') && raw.startsWith("'-") ? raw.slice(1) : raw
      groupSums[i] += BigInt(numeric)
    })
  }
  summary.slice(metricStart[kind], metricStart[kind] + metricCount).forEach((raw, i) => {
    const field = expected[metricStart[kind] + i]!
    const numeric = field === 'net_points' && (kind === 'commission' || kind === 'rewards') && raw.startsWith("'-") ? raw.slice(1) : raw
    if (groupSums[i] !== BigInt(numeric)) fail('Report export totals do not match its groups')
  })
}

function route(kind: ReportKind): string {
  if (kind === 'betting' || kind === 'ledger') return `${BASE}/${kind}/export`
  if (kind === 'withdrawal') return `${BASE}/withdrawal/export`
  if (kind === 'commission') return `${BASE}/commission.csv`
  return `${BASE}/${kind === 'reward_orders' ? 'reward-orders' : 'rewards'}.csv`
}

export function createPlatformReportExportApi(fetcher: typeof fetch = fetch) {
  return {
    async export(brandId: string, kind: ReportKind, query: ReportExportQuery): Promise<PlatformReportExport> {
      if (!UUID.test(brandId)) badInput('A valid brand UUID is required')
      if (!Object.hasOwn(columns, kind)) badInput('Unsupported report kind')
      if (!query || Object.hasOwn(query, 'limit') || Object.hasOwn(query, 'offset')) badInput('Report exports do not accept pagination')
      const brand = brandId.toLowerCase()
      const q = normalizeReportQuery(kind, { ...query, limit: 100, offset: 0 })
      const params = new URLSearchParams({ from: q.from, to: q.to, group_by: q.group_by })
      for (const [key, value] of Object.entries(q.filters)) if (value !== null) params.set(key, value)
      let response: Response
      try {
        response = await fetcher(`${route(kind)}?${params}`, { method: 'GET', credentials: 'same-origin', headers: { Accept: 'text/csv', 'X-Brand-ID': brand } })
      } catch (cause) {
        throw new PlatformApiError(cause instanceof Error ? cause.message : 'Network request failed', 0, 'NETWORK_ERROR', true)
      }
      if (!response.ok) {
        let message = `Request failed (${response.status})`, code: string | undefined
        try {
          const body: unknown = await response.json()
          if (body && typeof body === 'object' && 'error' in body) {
            const error = (body as { error?: unknown }).error
            if (error && typeof error === 'object') {
              if (typeof (error as { message?: unknown }).message === 'string') message = (error as { message: string }).message
              if (typeof (error as { code?: unknown }).code === 'string') code = (error as { code: string }).code
            }
          }
        } catch { /* Non-JSON errors retain their HTTP status. */ }
        throw new PlatformApiError(message, response.status, code)
      }
      if (response.status !== 200) fail('Report export requires a complete HTTP 200 response')
      const contentType = header(response.headers, 'Content-Type').toLowerCase()
      if (!contentType.startsWith('text/csv;') || !contentType.includes('charset=utf-8') || header(response.headers, 'Cache-Control').toLowerCase().split(',').map(v => v.trim()).indexOf('no-store') < 0 || header(response.headers, 'X-Content-Type-Options').toLowerCase() !== 'nosniff') fail()
      const lengthText = header(response.headers, 'Content-Length')
      if (!UINT.test(lengthText) || BigInt(lengthText) > BigInt(MAX_BYTES)) fail('Report export exceeds the complete byte limit')
      const groupCount = header(response.headers, 'X-Report-Group-Count')
      if (!UINT.test(groupCount) || BigInt(groupCount) > BigInt(MAX_GROUPS)) fail('Report export exceeds the complete group limit')
      const snapshotAt = header(response.headers, 'X-Report-Snapshot-At')
      const version = kind === 'commission' ? '2' : '1'
      const required: Record<string, string> = {
        'X-Report-Brand-ID': brand, 'X-Report-Kind': kind, 'X-Report-Format-Version': version,
      }
      for (const [name, value] of Object.entries(required)) if (header(response.headers, name) !== value) fail()
      const auditLogId = header(response.headers, 'X-Report-Audit-ID')
      if (!UUID.test(auditLogId) || !parseTimestamp(snapshotAt)) fail()
      const disposition = header(response.headers, 'Content-Disposition')
      // Go's filename timestamp uses UTC seconds, regardless of snapshot fractional precision.
      const canonicalFilename = `lottery-${kind}-${brand}-${snapshotAt.slice(0, 19).replace(/[-:]/g, '')}Z.csv`
      if (disposition !== `attachment; filename="${canonicalFilename}"`) fail()
      if (kind === 'commission' || kind === 'rewards' || kind === 'reward_orders') {
        if (!UINT.test(header(response.headers, 'X-Report-Byte-Count')) || header(response.headers, 'X-Report-Byte-Count') !== lengthText) fail()
      }
      if (kind === 'commission' || kind === 'rewards' || kind === 'reward_orders') {
        if (!header(response.headers, 'X-Report-Timezone')) fail()
      }
      if (kind === 'rewards' || kind === 'reward_orders') {
        const expectedHeaders: Record<string, string> = { 'X-Report-From': q.from, 'X-Report-To': q.to, 'X-Report-Group-By': q.group_by }
        if (q.filters.member_id) expectedHeaders['X-Report-Member-ID'] = q.filters.member_id
        if (q.filters.order_id) expectedHeaders['X-Report-Order-ID'] = q.filters.order_id
        for (const [name, value] of Object.entries(expectedHeaders)) if (header(response.headers, name) !== value) fail()
        for (const [name, value] of [['X-Report-Member-ID', q.filters.member_id], ['X-Report-Order-ID', q.filters.order_id]] as const) {
          if (!value && response.headers.has(name)) fail()
        }
      }
      const bytes = await readBytes(response)
      if (bytes.length !== Number(lengthText) || bytes.length > MAX_BYTES) fail('Report export length does not match its response header')
      const digest = [...new Uint8Array(await crypto.subtle.digest('SHA-256', bytes))].map(v => v.toString(16).padStart(2, '0')).join('')
      if (!/^[a-f0-9]{64}$/.test(header(response.headers, 'X-Report-SHA256')) || digest !== header(response.headers, 'X-Report-SHA256')) fail('Report export checksum does not match')
      if (bytes[0] !== 0xef || bytes[1] !== 0xbb || bytes[2] !== 0xbf) fail('Report CSV is missing its UTF-8 BOM')
      let decoded: string
      try { decoded = new TextDecoder('utf-8', { fatal: true }).decode(bytes.subarray(3)) } catch { return fail('Report CSV is not valid UTF-8') }
      validateCsv(kind, brand, q, response.headers, parseCsv(decoded), groupCount, snapshotAt)
      return { filename: canonicalFilename, bytes, groupCount, snapshotAt, auditLogId }
    },
  }
}
