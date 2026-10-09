import { describe, expect, it, vi } from 'vitest'
import { createPlatformArchivesApi, type ArchiveRecord, type ArchiveSnapshot } from './archives-api'
import { REPORT_FIELDS } from './reports-api'

const brand = '11111111-1111-4111-8111-111111111111'
const id = '22222222-2222-4222-8222-222222222222'
const actor = '33333333-3333-4333-8333-333333333333'
const task = '55555555-5555-4555-8555-555555555555'
const audit = '44444444-4444-4444-8444-444444444444'
const at = '2026-04-03T16:00:00.123456Z'
const from = '2026-04-02T16:00:00Z'
const to = '2026-04-03T16:00:00Z'
const zeroes = (names: readonly string[]) => Object.fromEntries(names.map(name => [name, '0']))

function snapshot(overrides: Partial<ArchiveSnapshot> = {}): ArchiveSnapshot {
  return {
    brand_id: brand, format_version: 1, snapshot_at: at, timezone: 'Asia/Singapore', from, to,
    betting: zeroes(REPORT_FIELDS.betting) as ArchiveSnapshot['betting'],
    ledger: zeroes(REPORT_FIELDS.ledger) as ArchiveSnapshot['ledger'],
    wallet_snapshot: { at_snapshot: at, balances: { account_count: '0', available_points: '0', frozen_points: '0', withdrawal_points: '0', total_points: '0' } },
    withdrawals: zeroes(REPORT_FIELDS.withdrawal) as ArchiveSnapshot['withdrawals'],
    commissions: zeroes(REPORT_FIELDS.commission) as ArchiveSnapshot['commissions'],
    rewards: zeroes(REPORT_FIELDS.rewards) as ArchiveSnapshot['rewards'],
    reward_orders: zeroes(REPORT_FIELDS.reward_orders) as ArchiveSnapshot['reward_orders'],
    ...overrides,
  }
}
function record(overrides: Partial<ArchiveRecord> = {}): ArchiveRecord {
  return {
    id, brand_id: brand, window: { kind: 'daily', period_key: '2026-04-03', timezone: 'Asia/Singapore', from, to },
    revision: 1, previous_id: null, snapshot_at: at, created_by: actor, reason: 'scheduled capture',
    payload_sha256: '0'.repeat(64), audit_log_id: audit, created_at: at, snapshot: snapshot(), ...overrides,
  } as ArchiveRecord
}
function envelope(data: unknown, status = 200): Response { return new Response(JSON.stringify({ success: true, data }), { status, headers: { 'Content-Type': 'application/json' } }) }
function page(items: ArchiveRecord[] = [record()], limit = 50, offset = 0) { return { brand_id: brand, items, total_count: String(items.length + offset), limit, offset } }
async function digest(bytes: Uint8Array): Promise<string> { return [...new Uint8Array(await crypto.subtle.digest('SHA-256', bytes))].map(b => b.toString(16).padStart(2, '0')).join('') }
async function fileResponse(body: string, rev = 1, headers: Record<string, string> = {}): Promise<Response> {
  const bytes = new TextEncoder().encode(body)
  return new Response(bytes, { status: 200, headers: {
    'Content-Type': 'application/json; charset=utf-8', 'Content-Length': String(bytes.byteLength), 'Content-Disposition': `attachment; filename="report-archive-${id}-v${rev}.json"`,
    'X-Content-SHA256': await digest(bytes), 'X-Archive-Revision': String(rev), 'X-Archive-Format-Version': '1', ...headers,
  } })
}

describe('platform archives API', () => {
  it('lists and reads current manual and automatic records with brand and paging checks', async () => {
    const manual = record()
    const automatic = record({ id: '66666666-6666-4666-8666-666666666666', created_by: null, automation: { task_id: task, policy_version: 2 } })
    const f = vi.fn<typeof fetch>().mockResolvedValueOnce(envelope(page([manual, automatic]))).mockResolvedValueOnce(envelope(automatic))
    const api = createPlatformArchivesApi(f)
    await expect(api.list(brand)).resolves.toMatchObject({ items: [manual, automatic], limit: 50 })
    await expect(api.read(brand, automatic.id)).resolves.toMatchObject({ automation: { task_id: task, policy_version: 2 } })
    expect(f.mock.calls[0]?.[0]).toBe('/api/v1/platform/report-archives?limit=50&offset=0')
    expect(new Headers(f.mock.calls[0]?.[1]?.headers).get('X-Brand-ID')).toBe(brand)
    expect(f.mock.calls.every(([, init]) => init?.method === 'GET')).toBe(true)
    const malformed = [
      { ...manual, created_by: null },
      { ...automatic, created_by: actor },
      { ...automatic, automation: undefined },
      { ...manual, revision: Number.MAX_SAFE_INTEGER + 1 },
      { ...manual, previous_id: id },
      { ...manual, audit_log_id: '' },
    ]
    for (const value of malformed) await expect(createPlatformArchivesApi(async () => envelope(value)).read(brand, id)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformArchivesApi(async () => envelope({ ...page(), brand_id: actor })).list(brand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformArchivesApi(async () => envelope({ ...page(), total_count: '99' })).list(brand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformArchivesApi(async () => envelope(page([manual, manual]))).list(brand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformArchivesApi(async () => envelope({ ...manual, brand_id: actor })).read(brand, id)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformArchivesApi(async () => envelope(manual)).read(brand, audit)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformArchivesApi(async () => envelope(manual)).list('bad')).rejects.toMatchObject({ code: 'INVALID_INPUT' })
    expect(f).toHaveBeenCalledTimes(2)
  })

  it('preserves sealed payload bytes and verifies SHA, headers, and semantic snapshot equality', async () => {
    const snapshotObject = snapshot({
      ledger: { entry_count: '1', net_points: '-900719925474099312345', recharge_points: '0', prize_credit_points: '0', prize_reversal_points: '900719925474099312345', refund_points: '0' },
      wallet_snapshot: { at_snapshot: at, balances: { account_count: '1', available_points: '900719925474099312345', frozen_points: '2', withdrawal_points: '3', total_points: '900719925474099312350' } },
    })
    const selected = record({ snapshot: snapshotObject })
    const raw = ` { "wallet_snapshot" : ${JSON.stringify(snapshotObject.wallet_snapshot)}, "rewards":${JSON.stringify(snapshotObject.rewards)},"reward_orders":${JSON.stringify(snapshotObject.reward_orders)},"commissions":${JSON.stringify(snapshotObject.commissions)},"withdrawals":${JSON.stringify(snapshotObject.withdrawals)},"ledger":${JSON.stringify(snapshotObject.ledger)},"betting":${JSON.stringify(snapshotObject.betting)},"to":"${to}","from":"${from}","timezone":"Asia/Singapore","snapshot_at":"${at}","format_version":1,"brand_id":"${brand}" } `
    const bytes = new TextEncoder().encode(raw)
    selected.payload_sha256 = await digest(bytes)
    const f = vi.fn<typeof fetch>().mockResolvedValue(await fileResponse(raw))
    const result = await createPlatformArchivesApi(f).download(brand, id, selected)
    expect(result).toEqual({ bytes, filename: `report-archive-${id}-v1.json`, sha256: selected.payload_sha256, record: selected })
    expect(new TextDecoder().decode(result.bytes)).toBe(raw)
    expect(f.mock.calls[0]?.[0]).toBe(`/api/v1/platform/report-archives/${id}/download`)
    expect(f.mock.calls[0]?.[1]?.method).toBe('GET')
    expect(new Headers(f.mock.calls[0]?.[1]?.headers).get('X-Brand-ID')).toBe(brand)
    await expect(createPlatformArchivesApi(async () => fileResponse(raw, 2)).download(brand, id, selected)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const mismatched = { ...selected, snapshot: snapshot() }
    await expect(createPlatformArchivesApi(async () => fileResponse(raw)).download(brand, id, mismatched)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    expect(f).toHaveBeenCalledOnce()
  })

  it('rejects corrupt download metadata, payload, and foreign record bindings', async () => {
    const selected = record()
    const body = JSON.stringify(selected.snapshot)
    selected.payload_sha256 = await digest(new TextEncoder().encode(body))
    const invalidHeaders: Record<string, string>[] = [
      { 'X-Content-SHA256': 'f'.repeat(64) }, { 'X-Archive-Revision': '2' }, { 'X-Archive-Format-Version': '2' },
      { 'Content-Length': '0' }, { 'Content-Type': 'application/octet-stream' }, { 'Content-Disposition': 'attachment; filename="wrong.json"' },
    ]
    for (const headers of invalidHeaders) await expect(createPlatformArchivesApi(async () => fileResponse(body, 1, headers)).download(brand, id, selected)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const wrongBrandPayload = JSON.stringify(snapshot({ brand_id: actor }))
    const wrongBrand = record({ payload_sha256: await digest(new TextEncoder().encode(wrongBrandPayload)) })
    await expect(createPlatformArchivesApi(async () => fileResponse(wrongBrandPayload)).download(brand, id, wrongBrand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformArchivesApi(async () => fileResponse('{bad json')).download(brand, id, selected)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformArchivesApi(async () => fileResponse(body)).download(brand, audit, selected)).rejects.toMatchObject({ code: 'INVALID_INPUT' })
    await expect(createPlatformArchivesApi(async () => fileResponse(body)).download(actor, id, selected)).rejects.toMatchObject({ code: 'INVALID_INPUT' })
  })
})
