import { describe, expect, it, vi } from 'vitest'
import { PlatformApiError } from './platform-api'
import { createPlatformReportExportApi, type ReportExportQuery } from './report-export-api'
import { REPORT_FIELDS, type ReportKind } from './reports-api'

const brand = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'
const member = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb'
const from = '2026-10-09T00:00:00Z'
const to = '2026-10-10T00:00:00Z'
const snapshot = '2026-10-10T00:00:00.123456Z'
const query: ReportExportQuery = { from, to, group_by: 'day' }
const kindPath: Record<ReportKind, string> = {
  betting: 'betting/export', ledger: 'ledger/export', withdrawal: 'withdrawal/export',
  commission: 'commission.csv', rewards: 'rewards.csv', reward_orders: 'reward-orders.csv',
}
const headersByKind: Record<ReportKind, string[]> = {
  betting: ['record_type','brand_id','snapshot_at','timezone','from','to','group_by','game_id','member_id','key','label', ...REPORT_FIELDS.betting],
  ledger: ['record_type','brand_id','snapshot_at','timezone','from','to','group_by','game_id','member_id','key','label', ...REPORT_FIELDS.ledger, 'account_count','available_points','frozen_points','withdrawal_points','total_points'],
  withdrawal: ['record_type','brand_id','snapshot_at','timezone','from','to','group_by','member_id','key','label', ...REPORT_FIELDS.withdrawal],
  commission: ['record_type','brand_id','snapshot_at','timezone','from','to','group_by','agent_id','member_id','cycle_id','key','label', ...REPORT_FIELDS.commission],
  rewards: ['record_type','brand_id','snapshot_at','timezone','from','to','group_by','member_id','order_id','key','label', ...REPORT_FIELDS.rewards],
  reward_orders: ['record_type','brand_id','snapshot_at','timezone','from','to','group_by','member_id','order_id','key','label', ...REPORT_FIELDS.reward_orders],
}
const csvCell = (v: string) => /[",\n\r]/.test(v) ? `"${v.replaceAll('"', '""')}"` : v
const csv = (rows: string[][]) => `\ufeff${rows.map(row => row.map(csvCell).join(',')).join('\n')}\n`
function fixture(kind: ReportKind, options: { label?: string; wrongBrand?: boolean; queryFrom?: string; groups?: number; groupBy?: string; memberFilter?: string; wrongTotals?: boolean; splitTotals?: boolean; wrongColumns?: boolean } = {}) {
  const fields = [...headersByKind[kind]]
  const groupBy = options.groupBy ?? 'day'
  const values = Object.fromEntries(REPORT_FIELDS[kind].map(field => [field, '0']))
  const huge = '9007199254740993'
  let bigField: string
  if (kind === 'betting') {
    bigField = 'stake_points'; values.order_count = '1'; values.placed_count = '1'; values.stake_points = huge; values.unfinalized_stake_points = huge
  } else if (kind === 'ledger') {
    bigField = 'entry_count'; values.entry_count = huge; values.net_points = '-9'; values.prize_reversal_points = '9'
  } else if (kind === 'withdrawal') {
    bigField = 'requested_points'; values.order_count = '1'; values.requested_points = huge; values.paid_count = '1'; values.paid_points = huge
  } else if (kind === 'commission') {
    bigField = 'paid_points'; values.entry_count = '2'; values.paid_entry_count = '1'; values.paid_points = huge
    values.adjustment_entry_count = '1'; values.adjustment_debit_points = (BigInt(huge) + 9n).toString(); values.net_points = '-9'
  } else if (kind === 'rewards') {
    bigField = 'grant_points'; values.entry_count = '2'; values.grant_entry_count = '1'; values.grant_points = huge
    values.reversal_entry_count = '1'; values.reversal_points = (BigInt(huge) + 9n).toString(); values.net_points = '-9'
  } else {
    bigField = 'original_points'; values.order_count = '1'; values.original_points = huge; values.granted_count = '1'; values.granted_points = huge
  }
  const key = groupBy === 'member' || groupBy === 'game' ? member : '2026-10-09'
  const label = options.label ?? key
  const metadata: Record<string, string> = {
    brand_id: options.wrongBrand ? member : brand, snapshot_at: snapshot, timezone: 'Asia/Singapore',
    from: options.queryFrom ?? from, to, group_by: groupBy, game_id: '', member_id: options.memberFilter ?? '', agent_id: '', cycle_id: '', order_id: '',
  }
  const row = (record: string, k: string, l: string, totals: Record<string, string>) => fields.map((field, index) => {
    if (field === 'record_type') return record
    if (field === 'key') return k
    if (field === 'label') return l
    if (field in totals) {
      const value = totals[field]!
      return field === 'net_points' && (kind === 'commission' || kind === 'rewards') && value.startsWith('-') ? `'${value}` : value
    }
    return metadata[field] ?? ''
  })
  const summary = row('summary', '', '', options.wrongTotals ? { ...values, [bigField]: '1' } : values)
  const group = row('group', key, label.startsWith('=') || label.startsWith('+') || label.startsWith('-') || label.startsWith('@') ? `'${label}` : label, values)
  const rows = [fields as string[], summary, group]
  if (options.splitTotals) {
    const first = { ...values, [bigField]: (BigInt(values[bigField]!) - 1n).toString() }
    const second = Object.fromEntries(REPORT_FIELDS[kind].map(field => [field, '0']))
    second[bigField] = '1'
    const secondKey = 'dddddddd-dddd-4ddd-8ddd-dddddddddddd'
    rows.splice(2, 1, row('group', secondKey, secondKey, second))
    rows[2] = row('group', member, label.startsWith('=') || label.startsWith('+') || label.startsWith('-') || label.startsWith('@') ? `'${label}` : label, first)
    rows.splice(3, 0, row('group', secondKey, secondKey, second))
  }
  if (kind === 'ledger') {
    const balances: Record<string, string> = { account_count: '1', available_points: '2', frozen_points: '3', withdrawal_points: '4', total_points: '9' }
    rows.splice(2, 0, row('balances', '', '', balances))
  }
  if (options.wrongColumns) rows[0]![0] = 'unexpected_column'
  const text = csv(rows)
  const bytes = new TextEncoder().encode(text)
  return { bytes, text, groupCount: options.groups === undefined ? options.splitTotals ? '2' : '1' : String(options.groups) }
}

async function responseFor(kind: ReportKind, opts: Parameters<typeof fixture>[1] = {}) {
  const f = fixture(kind, opts)
  const bytes = f.bytes
  const hash = [...new Uint8Array(await crypto.subtle.digest('SHA-256', bytes))].map(v => v.toString(16).padStart(2, '0')).join('')
  const filename = `lottery-${kind}-${brand}-20261010T000000Z.csv`
  const headers: Record<string, string> = {
    'Content-Type': 'text/csv; charset=utf-8', 'Content-Disposition': `attachment; filename="${filename}"`,
    'Content-Length': String(bytes.length), 'Cache-Control': 'no-store', 'X-Content-Type-Options': 'nosniff',
    'X-Report-Brand-ID': brand, 'X-Report-Kind': kind, 'X-Report-Snapshot-At': snapshot,
    'X-Report-Group-Count': f.groupCount, 'X-Report-SHA256': hash,
    'X-Report-Format-Version': kind === 'commission' ? '2' : '1', 'X-Report-Audit-ID': 'cccccccc-cccc-4ccc-8ccc-cccccccccccc',
  }
  if (['commission', 'rewards', 'reward_orders'].includes(kind)) {
    headers['X-Report-Byte-Count'] = String(bytes.length)
    headers['X-Report-Timezone'] = 'Asia/Singapore'
  }
  if (kind === 'rewards' || kind === 'reward_orders') {
    headers['X-Report-From'] = from; headers['X-Report-To'] = to; headers['X-Report-Group-By'] = opts.groupBy ?? 'day'
    if (opts.memberFilter) headers['X-Report-Member-ID'] = opts.memberFilter
  }
  return new Response(bytes, { status: 200, headers })
}
const fetchResponse = (response: Response) => vi.fn<typeof fetch>().mockResolvedValue(response)

describe('platform report export API', () => {
  it('downloads all six current CSV families at their backend paths and keeps exact bytes', async () => {
    for (const kind of Object.keys(kindPath) as ReportKind[]) {
      const response = await responseFor(kind)
      const original = new Uint8Array(await response.clone().arrayBuffer())
      const fetcher = fetchResponse(response)
      const result = await createPlatformReportExportApi(fetcher).export(brand, kind, query)
      expect(result).toMatchObject({ filename: `lottery-${kind}-${brand}-20261010T000000Z.csv`, groupCount: '1', snapshotAt: snapshot, auditLogId: 'cccccccc-cccc-4ccc-8ccc-cccccccccccc' })
      expect(result.bytes).toEqual(original)
      const [url, init] = fetcher.mock.calls[0]!
      expect(new URL(String(url), 'http://localhost').pathname).toBe(`/api/v1/platform/reports/${kindPath[kind]}`)
      expect(init).toMatchObject({ method: 'GET', credentials: 'same-origin' })
      expect(new Headers(init?.headers).get('X-Brand-ID')).toBe(brand)
    }
  })

  it('accepts quoted formula escaped labels, negative net text, and integer totals beyond Number precision', async () => {
    const label = '=SUM("a",\n"b")'
    const result = await createPlatformReportExportApi(fetchResponse(await responseFor('betting', { label, groupBy: 'game', splitTotals: true }))).export(brand, 'betting', { ...query, group_by: 'game' })
    expect(result.groupCount).toBe('2')
    expect(new TextDecoder().decode(result.bytes)).toContain("'=SUM(")
    expect(new TextDecoder().decode(result.bytes)).toContain('""b"")')
  })

  it('rejects pagination input before fetching and sends only normalized filters', async () => {
    const fetcher = vi.fn<typeof fetch>()
    const api = createPlatformReportExportApi(fetcher)
    await expect(api.export(brand, 'betting', { ...query, limit: 10 } as ReportExportQuery)).rejects.toMatchObject({ code: 'INVALID_INPUT' })
    expect(fetcher).not.toHaveBeenCalled()
    const scoped = await responseFor('rewards', { memberFilter: member })
    const call = fetchResponse(scoped)
    await createPlatformReportExportApi(call).export(brand, 'rewards', { ...query, member_id: member })
    const url = new URL(String(call.mock.calls[0]![0]), 'http://localhost')
    expect(Object.fromEntries(url.searchParams)).toEqual({ from, to, group_by: 'day', member_id: member })
  })

  it('rejects brand/query mismatch, invalid schema/checksum/headers, missing audit IDs, and truncation', async () => {
    const wrongBrand = await responseFor('betting', { wrongBrand: true })
    await expect(createPlatformReportExportApi(fetchResponse(wrongBrand)).export(brand, 'betting', query)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const wrongQuery = await responseFor('betting', { queryFrom: '2026-10-08T00:00:00Z' })
    await expect(createPlatformReportExportApi(fetchResponse(wrongQuery)).export(brand, 'betting', query)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformReportExportApi(fetchResponse(await responseFor('betting'))).export(brand, 'betting', { ...query, member_id: member })).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const badHash = await responseFor('betting'); badHash.headers.set('X-Report-SHA256', '0'.repeat(64))
    await expect(createPlatformReportExportApi(fetchResponse(badHash)).export(brand, 'betting', query)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const noAudit = await responseFor('betting'); noAudit.headers.delete('X-Report-Audit-ID')
    await expect(createPlatformReportExportApi(fetchResponse(noAudit)).export(brand, 'betting', query)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const oldCommission = await responseFor('commission'); oldCommission.headers.set('X-Report-Format-Version', '1')
    await expect(createPlatformReportExportApi(fetchResponse(oldCommission)).export(brand, 'commission', query)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const complete = await responseFor('betting')
    const partialStatus = new Response(await complete.arrayBuffer(), { status: 206, headers: complete.headers })
    await expect(createPlatformReportExportApi(fetchResponse(partialStatus)).export(brand, 'betting', query)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const wrongKind = await responseFor('betting'); wrongKind.headers.set('X-Report-Kind', 'ledger')
    await expect(createPlatformReportExportApi(fetchResponse(wrongKind)).export(brand, 'betting', query)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const wrongColumns = await responseFor('betting', { wrongColumns: true })
    await expect(createPlatformReportExportApi(fetchResponse(wrongColumns)).export(brand, 'betting', query)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const noCacheGuard = await responseFor('betting'); noCacheGuard.headers.delete('Cache-Control')
    await expect(createPlatformReportExportApi(fetchResponse(noCacheGuard)).export(brand, 'betting', query)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const wrongTotals = await responseFor('betting', { wrongTotals: true })
    await expect(createPlatformReportExportApi(fetchResponse(wrongTotals)).export(brand, 'betting', query)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const truncated = await responseFor('betting'); truncated.headers.set('Content-Length', String((await truncated.clone().arrayBuffer()).byteLength + 1))
    await expect(createPlatformReportExportApi(fetchResponse(truncated)).export(brand, 'betting', query)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const oversized = await responseFor('betting'); oversized.headers.set('Content-Length', String(4 * 1024 * 1024 + 1))
    await expect(createPlatformReportExportApi(fetchResponse(oversized)).export(brand, 'betting', query)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const tooManyGroups = await responseFor('betting', { groups: 10001 })
    await expect(createPlatformReportExportApi(fetchResponse(tooManyGroups)).export(brand, 'betting', query)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it('preserves 403 and 413 errors and wraps network failures as PlatformApiError', async () => {
    for (const status of [403, 413]) {
      const denied = new Response(JSON.stringify({ success: false, error: { code: status === 403 ? 'PERMISSION_DENIED' : 'REPORT_EXPORT_TOO_LARGE', message: 'rejected' } }), { status })
      await expect(createPlatformReportExportApi(fetchResponse(denied)).export(brand, 'betting', query)).rejects.toMatchObject({ status, message: 'rejected' })
    }
    await expect(createPlatformReportExportApi(vi.fn<typeof fetch>().mockRejectedValue(new Error('offline'))).export(brand, 'betting', query)).rejects.toBeInstanceOf(PlatformApiError)
  })
})
