import { PlatformApiError } from './platform-api'
import { REPORT_FIELDS } from './reports-api'

const BASE = '/api/v1/platform/report-archives'
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
const UINT = /^(0|[1-9][0-9]*)$/
const SINT = /^(0|-?[1-9][0-9]*)$/
const HEX256 = /^[a-f0-9]{64}$/
const MAX_REVISION = Number.MAX_SAFE_INTEGER

export interface ArchiveWindow { kind: 'daily' | 'monthly'; period_key: string; timezone: string; from: string; to: string }
export interface ArchiveBalances { account_count: string; available_points: string; frozen_points: string; withdrawal_points: string; total_points: string }
export interface ArchiveWallet { at_snapshot: string; balances: ArchiveBalances }
export interface ArchiveSnapshot {
  brand_id: string; format_version: 1; snapshot_at: string; timezone: string; from: string; to: string
  betting: Record<(typeof REPORT_FIELDS)['betting'][number], string>
  ledger: Record<(typeof REPORT_FIELDS)['ledger'][number], string>
  wallet_snapshot: ArchiveWallet
  withdrawals: Record<(typeof REPORT_FIELDS)['withdrawal'][number], string>
  commissions: Record<(typeof REPORT_FIELDS)['commission'][number], string>
  rewards: Record<(typeof REPORT_FIELDS)['rewards'][number], string>
  reward_orders: Record<(typeof REPORT_FIELDS)['reward_orders'][number], string>
}
export interface ArchiveAutomationEvidence { task_id: string; policy_version: number }
interface ArchiveRecordFields {
  id: string; brand_id: string; window: ArchiveWindow; revision: number; previous_id: string | null
  snapshot_at: string; created_by: string | null; reason: string; payload_sha256: string; audit_log_id: string
  created_at: string; snapshot: ArchiveSnapshot
}
export type ArchiveRecord = ArchiveRecordFields &
  ({ created_by: string; automation?: never } | { created_by: null; automation: ArchiveAutomationEvidence })
export interface ArchivePage { brand_id: string; items: ArchiveRecord[]; total_count: string; limit: number; offset: number }
export interface ArchiveDownload { bytes: Uint8Array; filename: string; sha256: string; record: ArchiveRecord }

const fields = {
  betting: REPORT_FIELDS.betting,
  ledger: REPORT_FIELDS.ledger,
  wallet: ['account_count', 'available_points', 'frozen_points', 'withdrawal_points', 'total_points'] as const,
  withdrawals: REPORT_FIELDS.withdrawal,
  commissions: REPORT_FIELDS.commission,
  rewards: REPORT_FIELDS.rewards,
  reward_orders: REPORT_FIELDS.reward_orders,
} as const
const recordFields = ['id', 'brand_id', 'window', 'revision', 'previous_id', 'snapshot_at', 'created_by', 'reason', 'payload_sha256', 'audit_log_id', 'created_at', 'snapshot'] as const
const automationRecordFields = [...recordFields, 'automation'] as const
const snapshotKeys = ['brand_id', 'format_version', 'snapshot_at', 'timezone', 'from', 'to', 'betting', 'ledger', 'wallet_snapshot', 'withdrawals', 'commissions', 'rewards', 'reward_orders'] as const
const windowKeys = ['kind', 'period_key', 'timezone', 'from', 'to'] as const
type Obj = Record<string, unknown>
const isObj = (v: unknown): v is Obj => v !== null && typeof v === 'object' && !Array.isArray(v)
function exact(v: Obj, keys: readonly string[]): boolean { const a = Object.keys(v).sort(), b = [...keys].sort(); return a.length === b.length && a.every((k, i) => k === b[i]) }
function fail(code = 'INVALID_RESPONSE', status = 502): never { throw new PlatformApiError(code === 'INVALID_INPUT' ? 'Report archive request parameters are invalid' : 'Invalid report archive response', status, code) }
function uuid(v: unknown): v is string { return typeof v === 'string' && UUID.test(v) }
function sameUuid(a: unknown, b: string): boolean { return typeof a === 'string' && a.toLowerCase() === b.toLowerCase() }
function instant(v: unknown): bigint | null {
  if (typeof v !== 'string') return null
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|([+-])(\d{2}):(\d{2}))$/.exec(v)
  if (!m) return null
  const [, ys, mos, ds, hs, mis, ss, fraction = '', zone, sign, zhs, zms] = m
  const y = Number(ys), mo = Number(mos), d = Number(ds), h = Number(hs), mi = Number(mis), s = Number(ss)
  const leap = y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0)
  const daysInMonth = mo === 2 ? leap ? 29 : 28 : [4, 6, 9, 11].includes(mo) ? 30 : 31
  if (y < 1 || mo < 1 || mo > 12 || d < 1 || d > daysInMonth || h > 23 || mi > 59 || s > 59) return null
  if (zone !== 'Z') { const oh = Number(zhs), om = Number(zms); if (oh > 14 || om > 59 || oh === 14 && om !== 0) return null }
  // Native date parsing handles offsets; keep the backend's sub-millisecond precision separately.
  const milliseconds = Date.parse(`${ys}-${mos}-${ds}T${hs}:${mis}:${ss}${zone}`)
  return Number.isFinite(milliseconds) ? BigInt(milliseconds) * 1_000_000n + BigInt(fraction.padEnd(9, '0')) : null
}
function date(v: unknown): v is string { return instant(v) !== null }
function timezone(v: unknown): v is string {
  if (typeof v !== 'string' || !v || v.length > 100 || v === 'Local') return false
  try { new Intl.DateTimeFormat('en', { timeZone: v }); return true } catch { return false }
}
function sameInstant(a: unknown, b: unknown): boolean { return instant(a) !== null && instant(a) === instant(b) }
function decimalObject(v: unknown, names: readonly string[], signed: readonly string[] = []): v is Obj {
  return isObj(v) && exact(v, names) && names.every(k => typeof v[k] === 'string' && (signed.includes(k) ? SINT : UINT).test(v[k] as string))
}
function total(v: Obj, names: readonly string[]): bigint { return names.reduce((n, k) => n + BigInt(v[k] as string), 0n) }
function validCalendarDay(value: string): boolean {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value)
  if (!m) return false
  const y = Number(m[1]), mo = Number(m[2]), d = Number(m[3]), leap = y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0)
  return y >= 1 && y <= 9998 && mo >= 1 && mo <= 12 && d >= 1 && d <= (mo === 2 ? leap ? 29 : 28 : [4, 6, 9, 11].includes(mo) ? 30 : 31)
}
function validSnapshot(v: unknown, brand: string): v is ArchiveSnapshot {
  if (!isObj(v) || !exact(v, snapshotKeys) || !sameUuid(v.brand_id, brand) || v.format_version !== 1 || !date(v.snapshot_at) || !timezone(v.timezone) || !date(v.from) || !date(v.to) || instant(v.to)! <= instant(v.from)!) return false
  if (!decimalObject(v.betting, fields.betting) || !decimalObject(v.ledger, fields.ledger, ['net_points']) || !decimalObject(v.wallet_snapshot && isObj(v.wallet_snapshot) ? v.wallet_snapshot.balances : null, fields.wallet) ||
      !decimalObject(v.withdrawals, fields.withdrawals) || !decimalObject(v.commissions, fields.commissions, ['net_points']) || !decimalObject(v.rewards, fields.rewards, ['net_points']) || !decimalObject(v.reward_orders, fields.reward_orders)) return false
  const b = v.betting
  if (total(b, ['placed_count', 'won_count', 'lost_count', 'abnormal_count', 'cancelled_count']) !== BigInt(b.order_count as string) ||
      total(b, ['settled_stake_points', 'unfinalized_stake_points', 'abnormal_stake_points', 'refund_points']) !== BigInt(b.stake_points as string) ||
      BigInt(b.stake_points as string) < BigInt(b.order_count as string) || BigInt(b.correction_open_count as string) > BigInt(b.order_count as string) ||
      (b.cancelled_count === '0') !== (b.refund_points === '0') || (b.abnormal_count === '0') !== (b.abnormal_stake_points === '0') ||
      BigInt(b.refund_points as string) < BigInt(b.cancelled_count as string) || BigInt(b.abnormal_stake_points as string) < BigInt(b.abnormal_count as string) ||
      b.current_prize_points !== '0' && b.won_count === '0' || b.settled_stake_points !== '0' && BigInt(b.won_count as string) + BigInt(b.lost_count as string) === 0n) return false
  const wallet = v.wallet_snapshot
  if (!isObj(wallet) || !exact(wallet, ['at_snapshot', 'balances']) || !date(wallet.at_snapshot) || !sameInstant(wallet.at_snapshot, v.snapshot_at)) return false
  if (total(wallet.balances as Obj, ['available_points', 'frozen_points', 'withdrawal_points']) !== BigInt((wallet.balances as Obj).total_points as string)) return false
  const w = v.withdrawals
  if (total(w, ['reviewing_count', 'processing_count', 'paid_count', 'rejected_count', 'failed_count', 'cancelled_count']) !== BigInt(w.order_count as string) ||
      total(w, ['reviewing_points', 'processing_points', 'paid_points', 'rejected_points', 'failed_points', 'cancelled_points']) !== BigInt(w.requested_points as string)) return false
  const c = v.commissions
  if (total(c, ['paid_entry_count', 'adjustment_entry_count', 'correction_entry_count']) !== BigInt(c.entry_count as string) || BigInt(c.net_points as string) !== BigInt(c.paid_points as string) + BigInt(c.adjustment_credit_points as string) - BigInt(c.adjustment_debit_points as string) + BigInt(c.correction_credit_points as string) - BigInt(c.correction_debit_points as string)) return false
  const r = v.rewards
  if (BigInt(r.grant_entry_count as string) + BigInt(r.reversal_entry_count as string) !== BigInt(r.entry_count as string) || BigInt(r.net_points as string) !== BigInt(r.grant_points as string) - BigInt(r.reversal_points as string)) return false
  const ro = v.reward_orders
  return total(ro, ['granted_count', 'pending_count', 'revoked_count']) === BigInt(ro.order_count as string) && total(ro, ['granted_points', 'pending_points', 'revoked_points']) === BigInt(ro.original_points as string)
}
function validReason(v: unknown): v is string { return typeof v === 'string' && !!v && v.trim() === v && !/[\u0000\r\n]/u.test(v) && new TextEncoder().encode(v).length <= 500 }
function validRecord(v: unknown, brand: string, id?: string): v is ArchiveRecord {
  if (!isObj(v) || !(exact(v, recordFields) || exact(v, automationRecordFields)) || !uuid(v.id) || id !== undefined && !sameUuid(v.id, id) || !uuid(v.brand_id) || !sameUuid(v.brand_id, brand) ||
      !Number.isSafeInteger(v.revision) || (v.revision as number) < 1 || (v.revision as number) > MAX_REVISION || !(v.previous_id === null || uuid(v.previous_id)) ||
      (v.revision === 1) !== (v.previous_id === null) || v.previous_id !== null && sameUuid(v.previous_id, v.id as string) || !date(v.snapshot_at) ||
      (Object.hasOwn(v, 'automation') ? v.created_by !== null || !isObj(v.automation) || !exact(v.automation, ['task_id', 'policy_version']) || !uuid(v.automation.task_id) || !Number.isSafeInteger(v.automation.policy_version) || (v.automation.policy_version as number) < 1 || (v.automation.policy_version as number) > MAX_REVISION : !uuid(v.created_by)) ||
      !validReason(v.reason) || typeof v.payload_sha256 !== 'string' || !HEX256.test(v.payload_sha256) || !uuid(v.audit_log_id) || !date(v.created_at) ||
      !isObj(v.window) || !exact(v.window, windowKeys) || !(v.window.kind === 'daily' || v.window.kind === 'monthly') || typeof v.window.period_key !== 'string' || !timezone(v.window.timezone) || !date(v.window.from) || !date(v.window.to) || instant(v.window.to)! <= instant(v.window.from)!) return false
  const w = v.window, period = w.kind === 'daily' ? /^(\d{4})-(\d{2})-(\d{2})$/.exec(w.period_key as string) : /^(\d{4})-(\d{2})$/.exec(w.period_key as string)
  if (!period || w.kind === 'daily' && !validCalendarDay(w.period_key as string) || w.kind === 'monthly' && (Number(period[1]) < 1 || Number(period[1]) > 9998 || Number(period[2]) < 1 || Number(period[2]) > 12)) return false
  const s = v.snapshot
  return validSnapshot(s, brand) && sameInstant(v.snapshot_at, s.snapshot_at) && instant(s.from)! === instant(w.from)! && instant(s.to)! === instant(w.to)! && s.timezone === w.timezone && instant(v.snapshot_at)! >= instant(w.to)! && sameInstant(v.created_at, v.snapshot_at)
}
function deepEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true
  if (Array.isArray(a) || Array.isArray(b)) return Array.isArray(a) && Array.isArray(b) && a.length === b.length && a.every((v, i) => deepEqual(v, b[i]))
  if (!isObj(a) || !isObj(b)) return false
  const ak = Object.keys(a).sort(), bk = Object.keys(b).sort()
  return ak.length === bk.length && ak.every((k, i) => k === bk[i] && deepEqual(a[k], b[k]))
}
function snapshotsMatch(a: ArchiveSnapshot, b: ArchiveSnapshot): boolean {
  return sameInstant(a.snapshot_at, b.snapshot_at) && sameInstant(a.from, b.from) && sameInstant(a.to, b.to) && sameInstant(a.wallet_snapshot.at_snapshot, b.wallet_snapshot.at_snapshot) &&
    deepEqual({ ...a, snapshot_at: '', from: '', to: '', wallet_snapshot: { balances: a.wallet_snapshot.balances } }, { ...b, snapshot_at: '', from: '', to: '', wallet_snapshot: { balances: b.wallet_snapshot.balances } })
}
async function responseData(response: Response): Promise<unknown> {
  let body: unknown
  try { body = await response.json() } catch { if (!response.ok) throw new PlatformApiError(`Request failed (${response.status})`, response.status); fail() }
  if (!response.ok || !isObj(body) || body.success !== true || body.data === undefined) {
    const error = isObj(body) && isObj(body.error) ? body.error : undefined
    throw new PlatformApiError(typeof error?.message === 'string' ? error.message : `Request failed (${response.status})`, response.status, typeof error?.code === 'string' ? error.code : undefined)
  }
  if (response.status !== 200) fail()
  return body.data
}
function contentLength(response: Response, actual: number): boolean { const value = response.headers.get('Content-Length'); return value !== null && /^(0|[1-9][0-9]*)$/.test(value) && BigInt(value) === BigInt(actual) }
function validatePage(v: unknown, brand: string, limit: number, offset: number): v is ArchivePage {
  if (!isObj(v) || !exact(v, ['brand_id', 'items', 'total_count', 'limit', 'offset']) || !sameUuid(v.brand_id, brand) || !Array.isArray(v.items) || v.items.length > limit || !v.items.every(item => validRecord(item, brand)) ||
      typeof v.total_count !== 'string' || !UINT.test(v.total_count) || v.limit !== limit || v.offset !== offset) return false
  const count = BigInt(v.total_count), start = BigInt(offset), expected = count <= start ? 0n : count - start < BigInt(limit) ? count - start : BigInt(limit)
  const ids = new Set<string>()
  for (const item of v.items as ArchiveRecord[]) { const key = item.id.toLowerCase(); if (ids.has(key)) return false; ids.add(key) }
  return BigInt(v.items.length) === expected
}
function checkBrandId(brandId: string, id?: string): void { if (!uuid(brandId) || id !== undefined && !uuid(id)) fail('INVALID_INPUT', 0) }

export function createPlatformArchivesApi(fetcher: typeof fetch = fetch) {
  async function get(path: string, brandId: string): Promise<Response> {
    try { return await fetcher(`${BASE}${path}`, { method: 'GET', credentials: 'same-origin', headers: { Accept: 'application/json', 'X-Brand-ID': brandId } }) }
    catch (cause) { throw new PlatformApiError(cause instanceof Error ? cause.message : 'Network request failed', 0, 'NETWORK_ERROR', true) }
  }
  return {
    async list(brandId: string, limit = 50, offset = 0): Promise<ArchivePage> {
      checkBrandId(brandId)
      if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000) fail('INVALID_INPUT', 0)
      const response = await get(`?${new URLSearchParams({ limit: String(limit), offset: String(offset) })}`, brandId)
      const data = await responseData(response)
      if (!validatePage(data, brandId, limit, offset)) fail()
      return data
    },
    async read(brandId: string, id: string): Promise<ArchiveRecord> {
      checkBrandId(brandId, id)
      const response = await get(`/${encodeURIComponent(id)}`, brandId), data = await responseData(response)
      if (!validRecord(data, brandId, id)) fail()
      return data
    },
    async download(brandId: string, id: string, selectedRecord: ArchiveRecord): Promise<ArchiveDownload> {
      checkBrandId(brandId, id)
      if (!validRecord(selectedRecord, brandId, id)) fail('INVALID_INPUT', 0)
      const response = await get(`/${encodeURIComponent(id)}/download`, brandId)
      if (!response.ok) { await responseData(response); fail() }
      if (response.status !== 200) fail()
      let bytes: Uint8Array
      try { bytes = new Uint8Array(await response.arrayBuffer()) } catch { fail() }
      const sha = response.headers.get('X-Content-SHA256'), revision = response.headers.get('X-Archive-Revision'), format = response.headers.get('X-Archive-Format-Version')
      const filename = `report-archive-${id}-v${selectedRecord.revision}.json`
      if (!contentLength(response, bytes.byteLength) || !sha || !HEX256.test(sha) || revision !== String(selectedRecord.revision) || format !== '1' ||
          response.headers.get('Content-Disposition') !== `attachment; filename="${filename}"` || !/^application\/json(?:;\s*charset=utf-8)?$/i.test(response.headers.get('Content-Type') ?? '')) fail()
      let actualSha: string
      try { actualSha = [...new Uint8Array(await crypto.subtle.digest('SHA-256', bytes))].map(b => b.toString(16).padStart(2, '0')).join('') } catch { fail() }
      if (actualSha !== sha || actualSha !== selectedRecord.payload_sha256) fail()
      let payload: unknown
      try { payload = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes)) } catch { fail() }
      if (!validSnapshot(payload, brandId) || !sameInstant(payload.snapshot_at, selectedRecord.snapshot_at) || !sameInstant(payload.from, selectedRecord.window.from) || !sameInstant(payload.to, selectedRecord.window.to) || payload.timezone !== selectedRecord.window.timezone || !snapshotsMatch(payload, selectedRecord.snapshot)) fail()
      return { bytes, filename, sha256: sha, record: selectedRecord }
    },
  }
}
