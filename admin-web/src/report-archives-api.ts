import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";
import type { BettingTotals, LedgerTotals } from "./reports-api";

const BASE = "/api/v1/admin/report-archives";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const UINT = /^(0|[1-9][0-9]*)$/;
const SINT = /^(0|-?[1-9][0-9]*)$/;
const HEX256 = /^[a-f0-9]{64}$/;
const MAX_REVISION = Number.MAX_SAFE_INTEGER;

export interface ReportArchiveWindow {
  kind: "daily" | "monthly";
  period_key: string;
  timezone: string;
  from: string;
  to: string;
}
export interface ReportArchiveBalances {
  account_count: string; available_points: string; frozen_points: string;
  withdrawal_points: string; total_points: string;
}
export interface ReportArchiveWallet { at_snapshot: string; balances: ReportArchiveBalances }
export interface ReportArchiveWithdrawalTotals {
  order_count: string; requested_points: string; reviewing_count: string; reviewing_points: string;
  processing_count: string; processing_points: string; paid_count: string; paid_points: string;
  rejected_count: string; rejected_points: string; failed_count: string; failed_points: string;
  cancelled_count: string; cancelled_points: string;
}
export interface ReportArchiveCommissionTotals {
  entry_count: string; paid_entry_count: string; paid_points: string; adjustment_entry_count: string;
  adjustment_credit_points: string; adjustment_debit_points: string; correction_entry_count: string;
  correction_credit_points: string; correction_debit_points: string; net_points: string;
}
export interface ReportArchiveRewardTotals {
  entry_count: string; grant_entry_count: string; grant_points: string; reversal_entry_count: string;
  reversal_points: string; net_points: string;
}
export interface ReportArchiveRewardOrderTotals {
  order_count: string; original_points: string; granted_count: string; granted_points: string;
  pending_count: string; pending_points: string; revoked_count: string; revoked_points: string;
}
export interface ReportArchiveSnapshot {
  brand_id: string; format_version: 1; snapshot_at: string; timezone: string; from: string; to: string;
  betting: BettingTotals; ledger: LedgerTotals; wallet_snapshot: ReportArchiveWallet;
  withdrawals: ReportArchiveWithdrawalTotals; commissions: ReportArchiveCommissionTotals;
  rewards: ReportArchiveRewardTotals; reward_orders: ReportArchiveRewardOrderTotals;
}
export interface ReportArchiveAutomationEvidence { task_id: string; policy_version: number }
interface ReportArchiveRecordFields {
  id: string; brand_id: string; window: ReportArchiveWindow; revision: number; previous_id: string | null;
  snapshot_at: string; created_by: string | null; reason: string; payload_sha256: string; audit_log_id: string;
  created_at: string; snapshot: ReportArchiveSnapshot;
}
export type ReportArchiveRecord = ReportArchiveRecordFields & (
  | { created_by: string; automation?: never }
  | { created_by: null; automation: ReportArchiveAutomationEvidence }
);
export interface ReportArchivePage {
  brand_id: string; items: ReportArchiveRecord[]; total_count: string; limit: number; offset: number;
}
export interface ReportArchiveCreateInput {
  kind: "daily" | "monthly"; period_key: string; expected_revision: number; reason: string;
}
export interface ReportArchiveDownload {
  bytes: Uint8Array; metadata: { sha256: string; revision: number; format_version: 1; filename: string };
}
export interface VerifiedReportArchiveDownload extends ReportArchiveDownload {
  metadata: ReportArchiveDownload["metadata"] & { record: ReportArchiveRecord };
}
export interface ReportArchivesApi {
  list(brandId: string, limit?: number, offset?: number): Promise<ReportArchivePage>;
  read(brandId: string, id: string): Promise<ReportArchiveRecord>;
  create(brandId: string, input: ReportArchiveCreateInput, key: string | undefined, expectedActor: string): Promise<ReportArchiveRecord>;
  download(brandId: string, id: string, previousRecord: ReportArchiveRecord): Promise<VerifiedReportArchiveDownload>;
}

const bettingFields = ["order_count", "stake_points", "placed_count", "won_count", "lost_count", "abnormal_count", "cancelled_count", "refund_points", "settled_stake_points", "unfinalized_stake_points", "abnormal_stake_points", "current_prize_points", "correction_open_count"] as const;
const ledgerFields = ["entry_count", "net_points", "recharge_points", "prize_credit_points", "prize_reversal_points", "refund_points"] as const;
const balanceFields = ["account_count", "available_points", "frozen_points", "withdrawal_points", "total_points"] as const;
const withdrawalFields = ["order_count", "requested_points", "reviewing_count", "reviewing_points", "processing_count", "processing_points", "paid_count", "paid_points", "rejected_count", "rejected_points", "failed_count", "failed_points", "cancelled_count", "cancelled_points"] as const;
const commissionFields = ["entry_count", "paid_entry_count", "paid_points", "adjustment_entry_count", "adjustment_credit_points", "adjustment_debit_points", "correction_entry_count", "correction_credit_points", "correction_debit_points", "net_points"] as const;
const rewardFields = ["entry_count", "grant_entry_count", "grant_points", "reversal_entry_count", "reversal_points", "net_points"] as const;
const rewardOrderFields = ["order_count", "original_points", "granted_count", "granted_points", "pending_count", "pending_points", "revoked_count", "revoked_points"] as const;
const recordFields = ["id", "brand_id", "window", "revision", "previous_id", "snapshot_at", "created_by", "reason", "payload_sha256", "audit_log_id", "created_at", "snapshot"] as const;
const automationRecordFields = [...recordFields, "automation"] as const;
const windowFields = ["kind", "period_key", "timezone", "from", "to"] as const;
const snapshotFields = ["brand_id", "format_version", "snapshot_at", "timezone", "from", "to", "betting", "ledger", "wallet_snapshot", "withdrawals", "commissions", "rewards", "reward_orders"] as const;

function isRecord(v: unknown): v is Record<string, unknown> { return v !== null && typeof v === "object" && !Array.isArray(v); }
function exact(v: Record<string, unknown>, fields: readonly string[]): boolean {
  const keys = Object.keys(v).sort(), want = [...fields].sort();
  return keys.length === want.length && keys.every((key, i) => key === want[i]);
}
function uuid(v: unknown): v is string { return typeof v === "string" && UUID.test(v); }
function sameUuid(a: unknown, b: string): boolean { return typeof a === "string" && a.toLowerCase() === b.toLowerCase(); }
function invalidInput(): never { throw new AdminApiError("Report archive request parameters are invalid.", 0, "INVALID_INPUT"); }
function invalidResponse(write = false): never {
  throw new AdminApiError("Report archive response is invalid; write result may be unknown.", write ? 0 : 502, write ? "UNKNOWN_WRITE_STATUS" : "INVALID_RESPONSE");
}
function unknownWrite(message = "Report archive write result is unknown; retry with the same idempotency key."): never {
  throw new AdminApiError(message, 0, "UNKNOWN_WRITE_STATUS");
}
function sameInstant(a: string, b: string): boolean { return instant(a) === instant(b); }
function deepEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (Array.isArray(a) || Array.isArray(b)) return Array.isArray(a) && Array.isArray(b) && a.length === b.length && a.every((v, i) => deepEqual(v, b[i]));
  if (!isRecord(a) || !isRecord(b)) return false;
  const ak = Object.keys(a).sort(), bk = Object.keys(b).sort();
  return ak.length === bk.length && ak.every((k, i) => k === bk[i] && deepEqual(a[k], b[k]));
}
function snapshotsMatch(a: ReportArchiveSnapshot, b: ReportArchiveSnapshot): boolean {
  return sameInstant(a.snapshot_at, b.snapshot_at) && sameInstant(a.from, b.from) && sameInstant(a.to, b.to) &&
    sameInstant(a.wallet_snapshot.at_snapshot, b.wallet_snapshot.at_snapshot) &&
    deepEqual({ ...a, snapshot_at: "", from: "", to: "", wallet_snapshot: { balances: a.wallet_snapshot.balances } },
      { ...b, snapshot_at: "", from: "", to: "", wallet_snapshot: { balances: b.wallet_snapshot.balances } });
}
function instant(value: unknown): bigint | null {
  if (typeof value !== "string") return null;
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|([+-])(\d{2}):(\d{2}))$/.exec(value);
  if (!m) return null;
  const [, ys, mos, ds, hs, mis, ss, fraction = "", zone, sign, zhs, zms] = m;
  const y = Number(ys), mo = Number(mos), d = Number(ds), h = Number(hs), mi = Number(mis), s = Number(ss);
  const leap = y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0);
  const mdays = mo === 2 ? leap ? 29 : 28 : [4, 6, 9, 11].includes(mo) ? 30 : 31;
  if (y < 1 || mo < 1 || mo > 12 || d < 1 || d > mdays || h > 23 || mi > 59 || s > 59) return null;
  let offset = 0;
  if (zone !== "Z") {
    const oh = Number(zhs), om = Number(zms);
    if (oh > 14 || om > 59 || oh === 14 && om !== 0) return null;
    offset = (oh * 60 + om) * (sign === "+" ? 1 : -1);
  }
  // Gregorian civil date to days since 1970-01-01, avoiding Date's year 0-99 behavior.
  const yy = BigInt(y - (mo <= 2 ? 1 : 0));
  const era = yy / 400n;
  const yoe = yy - era * 400n;
  const m2 = BigInt(mo + (mo > 2 ? -3 : 9));
  const doy = (153n * m2 + 2n) / 5n + BigInt(d - 1);
  const doe = yoe * 365n + yoe / 4n - yoe / 100n + doy;
  const days = era * 146097n + doe - 719468n;
  const sec = ((days * 24n + BigInt(h)) * 60n + BigInt(mi - offset)) * 60n + BigInt(s);
  const nanos = sec * 1_000_000_000n + BigInt(fraction.padEnd(9, "0") || "0");
  const utcYearOne = -62_135_596_800_000_000_000n;
  const utcYearTenThousand = 253_402_300_800_000_000_000n;
  return nanos < utcYearOne || nanos >= utcYearTenThousand ? null : nanos;
}
function validDate(value: unknown): value is string { return instant(value) !== null; }
function validTimezone(value: unknown): value is string {
  if (typeof value !== "string" || !value || value.length > 100 || value === "Local") return false;
  try { new Intl.DateTimeFormat("en", { timeZone: value }); return true; } catch { return false; }
}
function uintFields(v: unknown, fields: readonly string[], signed: readonly string[] = []): v is Record<string, string> {
  return isRecord(v) && exact(v, fields) && fields.every((f) => typeof v[f] === "string" && (signed.includes(f) ? SINT : UINT).test(v[f] as string));
}
function sum(v: Record<string, string>, keys: readonly string[]): bigint { return keys.reduce((n, k) => n + BigInt(v[k]!), 0n); }
function validSnapshot(value: unknown, brand: string): value is ReportArchiveSnapshot {
  if (!isRecord(value) || !exact(value, snapshotFields) || !sameUuid(value.brand_id, brand) || value.format_version !== 1 ||
    !validDate(value.snapshot_at) || !validTimezone(value.timezone) || !validDate(value.from) || !validDate(value.to) || instant(value.to)! <= instant(value.from)!) return false;
  if (!uintFields(value.betting, bettingFields) || !uintFields(value.ledger, ledgerFields, ["net_points"]) ||
    !uintFields(value.withdrawals, withdrawalFields) || !uintFields(value.commissions, commissionFields, ["net_points"]) ||
    !uintFields(value.rewards, rewardFields, ["net_points"]) || !uintFields(value.reward_orders, rewardOrderFields)) return false;
  const b = value.betting as unknown as Record<string, string>;
  if (sum(b, ["placed_count", "won_count", "lost_count", "abnormal_count", "cancelled_count"]) !== BigInt(b.order_count) ||
    sum(b, ["settled_stake_points", "unfinalized_stake_points", "abnormal_stake_points", "refund_points"]) !== BigInt(b.stake_points) ||
    BigInt(b.stake_points) < BigInt(b.order_count) || BigInt(b.correction_open_count) > BigInt(b.order_count) ||
    (b.cancelled_count === "0") !== (b.refund_points === "0") || (b.abnormal_count === "0") !== (b.abnormal_stake_points === "0") ||
    BigInt(b.refund_points) < BigInt(b.cancelled_count) || BigInt(b.abnormal_stake_points) < BigInt(b.abnormal_count) ||
    b.current_prize_points !== "0" && b.won_count === "0" ||
    b.settled_stake_points !== "0" && BigInt(b.won_count) + BigInt(b.lost_count) === 0n) return false;
  const wallet = value.wallet_snapshot;
  if (!isRecord(wallet) || !exact(wallet, ["at_snapshot", "balances"]) || !validDate(wallet.at_snapshot) || !sameInstant(wallet.at_snapshot as string, value.snapshot_at as string) || !uintFields(wallet.balances, balanceFields)) return false;
  const balances = wallet.balances as unknown as Record<string, string>;
  if (sum(balances, ["available_points", "frozen_points", "withdrawal_points"]) !== BigInt(balances.total_points)) return false;
  const w = value.withdrawals as unknown as Record<string, string>;
  if (sum(w, ["reviewing_count", "processing_count", "paid_count", "rejected_count", "failed_count", "cancelled_count"]) !== BigInt(w.order_count) ||
    sum(w, ["reviewing_points", "processing_points", "paid_points", "rejected_points", "failed_points", "cancelled_points"]) !== BigInt(w.requested_points)) return false;
  const c = value.commissions as unknown as Record<string, string>;
  if (sum(c, ["paid_entry_count", "adjustment_entry_count", "correction_entry_count"]) !== BigInt(c.entry_count) ||
    BigInt(c.net_points) !== BigInt(c.paid_points) + BigInt(c.adjustment_credit_points) - BigInt(c.adjustment_debit_points) + BigInt(c.correction_credit_points) - BigInt(c.correction_debit_points)) return false;
  const r = value.rewards as unknown as Record<string, string>;
  if (BigInt(r.grant_entry_count) + BigInt(r.reversal_entry_count) !== BigInt(r.entry_count) ||
    BigInt(r.net_points) !== BigInt(r.grant_points) - BigInt(r.reversal_points)) return false;
  const ro = value.reward_orders as unknown as Record<string, string>;
  return sum(ro, ["granted_count", "pending_count", "revoked_count"]) === BigInt(ro.order_count) &&
    sum(ro, ["granted_points", "pending_points", "revoked_points"]) === BigInt(ro.original_points);
}
function validRecord(v: unknown, brand: string, id?: string, actor?: string): v is ReportArchiveRecord {
  if (!isRecord(v) || !(exact(v, recordFields) || exact(v, automationRecordFields)) || !uuid(v.id) || id !== undefined && !sameUuid(v.id, id) ||
    !uuid(v.brand_id) || !sameUuid(v.brand_id, brand) || !Number.isSafeInteger(v.revision) || (v.revision as number) < 1 || (v.revision as number) > MAX_REVISION ||
    !(v.previous_id === null || uuid(v.previous_id)) || (v.revision === 1) !== (v.previous_id === null) || v.previous_id !== null && sameUuid(v.previous_id, v.id as string) ||
    !validDate(v.snapshot_at) ||
    (Object.hasOwn(v, "automation")
      ? v.created_by !== null || !isRecord(v.automation) || !exact(v.automation, ["task_id", "policy_version"]) || !uuid(v.automation.task_id) ||
        !Number.isSafeInteger(v.automation.policy_version) || (v.automation.policy_version as number) < 1 || (v.automation.policy_version as number) > MAX_REVISION || actor !== undefined
      : !uuid(v.created_by) || actor !== undefined && !sameUuid(v.created_by, actor)) ||
    !validStoredReason(v.reason) || typeof v.payload_sha256 !== "string" || !HEX256.test(v.payload_sha256) || !uuid(v.audit_log_id) || !validDate(v.created_at) ||
    !isRecord(v.window) || !exact(v.window, windowFields) || !(v.window.kind === "daily" || v.window.kind === "monthly") ||
    typeof v.window.period_key !== "string" || !validTimezone(v.window.timezone) || !validDate(v.window.from) || !validDate(v.window.to) || instant(v.window.to)! <= instant(v.window.from)!) return false;
  const w = v.window;
  const keyPattern = w.kind === "daily" ? /^(\d{4})-(\d{2})-(\d{2})$/ : /^(\d{4})-(\d{2})$/;
  const key = keyPattern.exec(w.period_key as string);
  if (!key) return false;
  if (w.kind === "daily" && !validCalendarDay(w.period_key as string) || w.kind === "monthly" && (Number(key[1]) < 1 || Number(key[1]) > 9998 || Number(key[2]) < 1 || Number(key[2]) > 12)) return false;
  const s = v.snapshot;
  return validSnapshot(s, brand) && sameInstant(v.snapshot_at as string, s.snapshot_at) && sameUuid(s.brand_id, v.brand_id as string) &&
    instant(s.from)! === instant(w.from)! && instant(s.to)! === instant(w.to)! && s.timezone === w.timezone &&
    instant(v.snapshot_at as string)! >= instant(w.to as string)! &&
    sameInstant(v.created_at as string, v.snapshot_at as string) && validDate(v.window.from) && validDate(v.window.to);
}
function validCalendarDay(value: string): boolean {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value)!;
  const y = Number(m[1]), mo = Number(m[2]), d = Number(m[3]);
  const days = mo === 2 ? y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0) ? 29 : 28 : [4, 6, 9, 11].includes(mo) ? 30 : 31;
  return y >= 1 && y <= 9998 && mo >= 1 && mo <= 12 && d >= 1 && d <= days;
}
function hasLoneSurrogate(value: string): boolean {
  for (let i = 0; i < value.length; i++) {
    const c = value.charCodeAt(i);
    if (c >= 0xd800 && c <= 0xdbff) { const n = value.charCodeAt(i + 1); if (!(n >= 0xdc00 && n <= 0xdfff)) return true; i++; }
    else if (c >= 0xdc00 && c <= 0xdfff) return true;
  }
  return false;
}
function validStoredReason(v: unknown): v is string {
  return typeof v === "string" && v.length > 0 && v.trim() === v && !hasLoneSurrogate(v) &&
    new TextEncoder().encode(v).length <= 500 && !/[\u0000\r\n]/u.test(v);
}
function validCreateReason(v: unknown): v is string {
  return validStoredReason(v) && !/[\u0000-\u001f\u007f-\u009f]/u.test(v);
}
function validateCreateInput(input: ReportArchiveCreateInput): void {
  if (!isRecord(input) || !exact(input, ["kind", "period_key", "expected_revision", "reason"]) ||
    !(input.kind === "daily" || input.kind === "monthly") || typeof input.period_key !== "string" ||
    !Number.isSafeInteger(input.expected_revision) || input.expected_revision < 0 || input.expected_revision >= MAX_REVISION || !validCreateReason(input.reason)) invalidInput();
  const re = input.kind === "daily" ? /^(\d{4})-(\d{2})-(\d{2})$/ : /^(\d{4})-(\d{2})$/;
  const m = re.exec(input.period_key);
  if (!m || (input.kind === "daily" ? !validCalendarDay(input.period_key) : Number(m[1]) < 1 || Number(m[1]) > 9998 || Number(m[2]) < 1 || Number(m[2]) > 12)) invalidInput();
}
function validatePage(value: unknown, brand: string, limit: number, offset: number): value is ReportArchivePage {
  if (!isRecord(value) || !exact(value, ["brand_id", "items", "total_count", "limit", "offset"]) || !sameUuid(value.brand_id, brand) ||
    !Array.isArray(value.items) || value.items.length > limit || !value.items.every((item) => validRecord(item, brand)) ||
    typeof value.total_count !== "string" || !UINT.test(value.total_count) || value.limit !== limit || value.offset !== offset) return false;
  const count = BigInt(value.total_count), start = BigInt(offset);
  const expectedItems = count <= start ? 0n : count - start < BigInt(limit) ? count - start : BigInt(limit);
  const ids = new Set<string>();
  for (const item of value.items) {
    const key = item.id.toLowerCase();
    if (ids.has(key)) return false;
    ids.add(key);
  }
  return BigInt(value.items.length) === expectedItems;
}
function contentLength(response: Response, actual: number): boolean {
  const header = response.headers.get("Content-Length");
  return header !== null && /^(0|[1-9][0-9]*)$/.test(header) && BigInt(header) === BigInt(actual);
}
async function envelope(response: Response, write: boolean): Promise<unknown> {
  let body: unknown;
  if (!response.ok) {
    try { body = await response.json(); } catch { body = undefined; }
    const e = isRecord(body) && isRecord(body.error) ? body.error : undefined;
    if (isRecord(body) && typeof body.error === "string") throw new AdminApiError("Invalid server response", 502, "INVALID_RESPONSE");
    if (write && (response.status < 400 || response.status >= 500)) unknownWrite();
    throw new AdminApiError(typeof e?.message === "string" ? e.message : `Request failed (${response.status})`, response.status, typeof e?.code === "string" ? e.code : undefined);
  }
  try { body = await response.json(); } catch { if (write) unknownWrite(); invalidResponse(); }
  if (!isRecord(body) || body.success !== true || body.data === undefined) { if (write) unknownWrite(); invalidResponse(); }
  return body.data;
}
function ensureBrand(brand: string): void { if (!uuid(brand)) invalidInput(); }

export function reportArchivesPermissions(account: AdminAccount, brandId: string): { view: boolean; create: boolean; download: boolean } {
  const scoped = uuid(account.id) && uuid(brandId);
  const brand = brandPermissionSet(account, brandId.toLowerCase());
  const view = scoped && brand.has("report_archive.view.brand");
  return {
    view: Boolean(view),
    create: Boolean(view && brand.has("report_archive.create.brand")),
    download: Boolean(view && brand.has("report_archive.download.brand")),
  };
}

export function createReportArchivesApi(fetcher: typeof fetch = fetch): ReportArchivesApi {
  async function request(path: string, brand: string, options: { method?: "GET" | "POST"; body?: unknown; key?: string; actor?: string; write?: boolean; successStatus?: number } = {}): Promise<unknown> {
    const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brand });
    if (options.body !== undefined) headers.set("Content-Type", "application/json");
    if (options.key !== undefined) headers.set("Idempotency-Key", options.key);
    if (options.actor !== undefined) headers.set("X-Report-Archive-Actor-ID", options.actor);
    let response: Response;
    try { response = await fetcher(path, { method: options.method ?? "GET", credentials: "same-origin", headers, ...(options.body === undefined ? {} : { body: JSON.stringify(options.body) }) }); }
    catch (e) { if (options.write) unknownWrite(e instanceof Error ? e.message : undefined); throw new AdminApiError(e instanceof Error ? e.message : "Network request failed", 0, "NETWORK_ERROR"); }
    if (options.successStatus !== undefined && response.ok && response.status !== options.successStatus) { if (options.write) unknownWrite(); invalidResponse(); }
    return envelope(response, Boolean(options.write));
  }
  return {
    async list(brandId, limit = 20, offset = 0) {
      ensureBrand(brandId);
      if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000) invalidInput();
      const data = await request(`${BASE}?${new URLSearchParams({ limit: String(limit), offset: String(offset) })}`, brandId, { successStatus: 200 });
      if (!validatePage(data, brandId, limit, offset)) invalidResponse();
      return data;
    },
    async read(brandId, id) {
      ensureBrand(brandId); if (!uuid(id)) invalidInput();
      const data = await request(`${BASE}/${encodeURIComponent(id)}`, brandId, { successStatus: 200 });
      if (!validRecord(data, brandId, id)) invalidResponse();
      return data;
    },
    async create(brandId, input, key = createIdempotencyKey(), expectedActor) {
      const intent = { ...input };
      ensureBrand(brandId); validateCreateInput(intent);
      if (!uuid(expectedActor) || !/^[A-Za-z0-9_:.-]{8,128}$/.test(key)) invalidInput();
      const data = await request(BASE, brandId, { method: "POST", body: intent, key, actor: expectedActor, write: true, successStatus: 201 });
      if (!validRecord(data, brandId, undefined, expectedActor) || data.revision !== intent.expected_revision + 1 ||
        (intent.expected_revision === 0 ? data.previous_id !== null : data.previous_id === null) ||
        data.window.kind !== intent.kind || data.window.period_key !== intent.period_key || data.reason !== intent.reason) invalidResponse(true);
      return data;
    },
    async download(brandId, id, previousRecord) {
      let expected: ReportArchiveRecord;
      try { expected = structuredClone(previousRecord); } catch { invalidInput(); }
      ensureBrand(brandId); if (!uuid(id) || !validRecord(expected, brandId, id)) invalidInput();
      let response: Response;
      try { response = await fetcher(`${BASE}/${encodeURIComponent(id)}/download`, { method: "GET", credentials: "same-origin", headers: { Accept: "application/json", "X-Brand-ID": brandId } }); }
      catch (e) { throw new AdminApiError(e instanceof Error ? e.message : "Network request failed", 0, "NETWORK_ERROR"); }
      if (!response.ok) {
        const data = await envelope(response, false); void data;
      }
      if (response.status !== 200) invalidResponse();
      let buffer: ArrayBuffer;
      try { buffer = await response.arrayBuffer(); } catch { invalidResponse(); }
      const bytes = new Uint8Array(buffer);
      const sha = response.headers.get("X-Content-SHA256");
      const revisionHeader = response.headers.get("X-Archive-Revision");
      const formatHeader = response.headers.get("X-Archive-Format-Version");
      const disposition = response.headers.get("Content-Disposition");
      const filename = `report-archive-${id}-v${expected.revision}.json`;
      if (!contentLength(response, bytes.byteLength) || !sha || !HEX256.test(sha) || !revisionHeader || !/^[1-9][0-9]*$/.test(revisionHeader) ||
        Number(revisionHeader) !== expected.revision || formatHeader !== "1" || disposition !== `attachment; filename="${filename}"` ||
        !/^application\/json(?:;\s*charset=utf-8)?$/i.test(response.headers.get("Content-Type") ?? "")) invalidResponse();
      let actualSha: string;
      try { actualSha = [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))].map((b) => b.toString(16).padStart(2, "0")).join(""); }
      catch { invalidResponse(); }
      if (actualSha !== sha || actualSha !== expected.payload_sha256) invalidResponse();
      let decoded: unknown;
      try { decoded = JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(bytes)); } catch { invalidResponse(); }
      if (!validSnapshot(decoded, brandId) || !sameInstant(decoded.snapshot_at, expected.snapshot_at) ||
        !sameInstant(decoded.snapshot_at, decoded.wallet_snapshot.at_snapshot) ||
        !sameInstant(decoded.from, expected.window.from) || !sameInstant(decoded.to, expected.window.to) ||
        decoded.timezone !== expected.window.timezone || !snapshotsMatch(decoded, expected.snapshot)) invalidResponse();
      return { bytes, metadata: { record: expected, sha256: sha, revision: Number(revisionHeader), format_version: 1, filename } };
    },
  };
}
