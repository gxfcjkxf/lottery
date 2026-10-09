import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { createReportArchivesApi, reportArchivesPermissions, type ReportArchiveRecord, type ReportArchiveSnapshot } from "./report-archives-api";

const brand = "11111111-1111-4111-8111-111111111111";
const archiveId = "22222222-2222-4222-8222-222222222222";
const actor = "33333333-3333-4333-8333-333333333333";
const auditId = "44444444-4444-4444-8444-444444444444";
const from = "0001-01-01T00:00:00.000000001Z";
const to = "0001-01-02T00:00:00Z";
const at = "2026-04-03T00:00:00.123456Z";
function response(data: unknown, status = 200, headers?: HeadersInit): Response {
  return new Response(JSON.stringify({ success: true, data }), { status, headers: { "Content-Type": "application/json", ...Object.fromEntries(new Headers(headers)) } });
}
function zeroes(fields: readonly string[]) { return Object.fromEntries(fields.map((f) => [f, "0"])); }
function snapshot(overrides: Partial<ReportArchiveSnapshot> = {}): ReportArchiveSnapshot {
  return {
    brand_id: brand, format_version: 1, snapshot_at: at, timezone: "UTC", from, to,
    betting: { ...zeroes(["order_count", "stake_points", "placed_count", "won_count", "lost_count", "abnormal_count", "cancelled_count", "refund_points", "settled_stake_points", "unfinalized_stake_points", "abnormal_stake_points", "current_prize_points", "correction_open_count"]) } as unknown as ReportArchiveSnapshot["betting"],
    ledger: { ...zeroes(["entry_count", "recharge_points", "prize_credit_points", "prize_reversal_points", "refund_points"]), net_points: "0" } as ReportArchiveSnapshot["ledger"],
    wallet_snapshot: { at_snapshot: at, balances: { ...zeroes(["account_count", "available_points", "frozen_points", "withdrawal_points", "total_points"]) } as unknown as ReportArchiveSnapshot["wallet_snapshot"]["balances"] },
    withdrawals: { ...zeroes(["order_count", "requested_points", "reviewing_count", "reviewing_points", "processing_count", "processing_points", "paid_count", "paid_points", "rejected_count", "rejected_points", "failed_count", "failed_points", "cancelled_count", "cancelled_points"]) } as unknown as ReportArchiveSnapshot["withdrawals"],
    commissions: { ...zeroes(["entry_count", "paid_entry_count", "paid_points", "adjustment_entry_count", "adjustment_credit_points", "adjustment_debit_points", "correction_entry_count", "correction_credit_points", "correction_debit_points"]), net_points: "0" } as ReportArchiveSnapshot["commissions"],
    rewards: { ...zeroes(["entry_count", "grant_entry_count", "grant_points", "reversal_entry_count", "reversal_points"]), net_points: "0" } as ReportArchiveSnapshot["rewards"],
    reward_orders: { ...zeroes(["order_count", "original_points", "granted_count", "granted_points", "pending_count", "pending_points", "revoked_count", "revoked_points"]) } as unknown as ReportArchiveSnapshot["reward_orders"],
    ...overrides,
  };
}
function record(overrides: Partial<ReportArchiveRecord> = {}): ReportArchiveRecord {
  const s = snapshot();
  return {
    id: archiveId, brand_id: brand,
    window: { kind: "daily", period_key: "0001-01-01", timezone: "UTC", from, to },
    revision: 1, previous_id: null, snapshot_at: at, created_by: actor, reason: "scheduled capture",
    payload_sha256: "a".repeat(64), audit_log_id: auditId, created_at: at, snapshot: s, ...overrides,
  } as ReportArchiveRecord;
}
function page(items: ReportArchiveRecord[] = [record()]) { return { brand_id: brand, items, total_count: String(items.length), limit: 20, offset: 0 }; }
function fetcher(data: unknown, status = 200) { return vi.fn<typeof fetch>().mockResolvedValue(response(data, status)); }

describe("report archives API", () => {
  it("models all four routes, sends the exact brand and actor headers, and validates records/pages", async () => {
    const f = fetcher(page());
    const api = createReportArchivesApi(f);
    await expect(api.list(brand)).resolves.toMatchObject({ brand_id: brand, items: [{ id: archiveId }] });
    const [listUrl, listInit] = f.mock.calls[0]!;
    expect(listUrl).toBe("/api/v1/admin/report-archives?limit=20&offset=0");
    expect(new Headers(listInit?.headers).get("X-Brand-ID")).toBe(brand);
    await expect(createReportArchivesApi(fetcher(record())).read(brand, archiveId)).resolves.toMatchObject({ id: archiveId });
    await expect(createReportArchivesApi(fetcher(record(), 201)).read(brand, archiveId)).rejects.toMatchObject({ code: "INVALID_RESPONSE", status: 502 });

    const body = { kind: "daily" as const, period_key: "0001-01-01", expected_revision: 0, reason: "scheduled capture" };
    const post = fetcher(record(), 201);
    await expect(createReportArchivesApi(post).create(brand, body, "archive-key-0001", actor)).resolves.toMatchObject({ created_by: actor });
    const [postUrl, postInit] = post.mock.calls[0]!;
    const headers = new Headers(postInit?.headers);
    expect(postUrl).toBe("/api/v1/admin/report-archives");
    expect(headers.get("X-Brand-ID")).toBe(brand);
    expect(headers.get("X-Report-Archive-Actor-ID")).toBe(actor);
    expect(headers.get("Idempotency-Key")).toBe("archive-key-0001");
    expect(JSON.parse(String(postInit?.body))).toEqual(body);
    expect(postInit?.method).toBe("POST");

    const corrupt = { ...record(), snapshot: { ...snapshot(), surprise: "closed" } };
    await expect(createReportArchivesApi(fetcher(page([corrupt as ReportArchiveRecord]))).list(brand)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(createReportArchivesApi(fetcher({ ...page(), brand_id: actor })).list(brand)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(createReportArchivesApi(fetcher({ ...page(), total_count: "2" })).list(brand)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(createReportArchivesApi(fetcher(page([record(), record()]))).list(brand)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(createReportArchivesApi(fetcher({ ...record(), created_by: actor })).read(brand, auditId)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const automatic = record({ created_by: null, automation: { task_id: actor, policy_version: 1 } });
    await expect(createReportArchivesApi(fetcher(automatic)).read(brand, archiveId)).resolves.toMatchObject({ created_by: null, automation: { task_id: actor, policy_version: 1 } });
    for (const invalid of [
      { ...automatic, created_by: actor },
      { ...record(), created_by: null },
      { ...automatic, automation: { task_id: actor, policy_version: 0 } },
      { ...automatic, automation: { task_id: actor, policy_version: 1.5 } },
      { ...automatic, automation: { task_id: "not-a-uuid", policy_version: 1 } },
      { ...automatic, automation: { task_id: actor, policy_version: 1, extra: true } },
      { ...automatic, extra: true },
    ]) await expect(createReportArchivesApi(fetcher(invalid)).read(brand, archiveId)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(createReportArchivesApi(fetcher({ ...automatic, automation: { task_id: actor, policy_version: 1 } }, 201)).create(
      brand, { kind: "daily", period_key: "0001-01-01", expected_revision: 0, reason: "scheduled capture" }, "archive-key-0001", actor,
    )).rejects.toMatchObject({ code: "UNKNOWN_WRITE_STATUS" });
  });

  it("validates independent brand view/download grants and restricts create", () => {
    const account = (permissions_by_brand: Record<string, string[]>, platform_permissions: string[] = [], super_admin = false): AdminAccount => ({
      id: actor, super_admin, brand_ids: [brand], permissions: [], permissions_by_brand, platform_permissions,
    });
    expect(reportArchivesPermissions(account({ [brand]: ["report_archive.view.brand"] }), brand)).toEqual({ view: true, create: false, download: false });
    expect(reportArchivesPermissions(account({ [brand]: ["report_archive.view.brand", "report_archive.create.brand", "report_archive.download.brand"] }), brand)).toEqual({ view: true, create: true, download: true });
    expect(reportArchivesPermissions(account({ [brand]: [] }, ["report_archive.view.platform", "report_archive.download.platform"]), brand)).toEqual({ view: false, create: false, download: false });
    expect(reportArchivesPermissions(account({ [brand]: ["report_archive.create.brand"] }, ["report_archive.view.platform"]), brand).create).toBe(false);
    expect(reportArchivesPermissions(account({ [brand]: ["report_archive.view.brand", "report_archive.create.brand", "report_archive.download.brand"] }, [], true), brand)).toEqual({ view: false, create: false, download: false });
    expect(reportArchivesPermissions(account({ [brand]: ["report_archive.view.brand"] }), "55555555-5555-4555-8555-555555555555").view).toBe(false);
  });

  it("keeps amounts arbitrary precision, accepts a negative net, and enforces aggregate equations", async () => {
    const huge = "922337203685477580812345678901234567890";
    const good = snapshot({
      ledger: { entry_count: "1", net_points: "-" + huge, recharge_points: "0", prize_credit_points: "0", prize_reversal_points: huge, refund_points: "0" },
      wallet_snapshot: { at_snapshot: at, balances: { account_count: "1", available_points: huge, frozen_points: "2", withdrawal_points: "3", total_points: (BigInt(huge) + 5n).toString() } },
      commissions: { entry_count: "1", paid_entry_count: "1", paid_points: "0", adjustment_entry_count: "0", adjustment_credit_points: "0", adjustment_debit_points: "0", correction_entry_count: "0", correction_credit_points: "0", correction_debit_points: "0", net_points: "0" },
      rewards: { entry_count: "2", grant_entry_count: "1", grant_points: huge, reversal_entry_count: "1", reversal_points: huge, net_points: "0" },
    });
    const value = record({ snapshot: good });
    await expect(createReportArchivesApi(fetcher(value)).read(brand, archiveId)).resolves.toMatchObject({ snapshot: { ledger: { net_points: `-${huge}` } } });
    const malformed = record({ snapshot: { ...good, wallet_snapshot: { ...good.wallet_snapshot, balances: { ...good.wallet_snapshot.balances, total_points: "1" } } } });
    await expect(createReportArchivesApi(fetcher(malformed)).read(brand, archiveId)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const badCommission = record({ snapshot: { ...good, commissions: { ...good.commissions, net_points: "1" } } });
    await expect(createReportArchivesApi(fetcher(badCommission)).read(brand, archiveId)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const foreign = record({ snapshot: { ...good, brand_id: actor } });
    await expect(createReportArchivesApi(fetcher(foreign)).read(brand, archiveId)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("rejects bad revisions, actor relations, malformed dates, and lone-surrogate reasons before posting", async () => {
    const f = fetcher(record());
    const api = createReportArchivesApi(f);
    const base = { kind: "daily" as const, period_key: "2026-04-03", expected_revision: 0, reason: "capture" };
    for (const input of [
      { ...base, expected_revision: Number.MAX_SAFE_INTEGER },
      { ...base, period_key: "2026-02-29" },
      { ...base, reason: "bad\uD800" },
      { ...base, reason: "bad\u0085" },
      { ...base, reason: "stored\treason" },
      { ...base, reason: "é".repeat(251) },
      { ...base, extra: true },
    ]) await expect(api.create(brand, input as never, "archive-key-0001", actor)).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(createReportArchivesApi(fetcher(record({ reason: "stored\treason" }))).read(brand, archiveId)).resolves.toMatchObject({ reason: "stored\treason" });
    await expect(api.create(brand, base, "archive-key-0001", undefined as never)).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(api.create(brand, base, "archive-key-0001", auditId)).rejects.toMatchObject({ code: "UNKNOWN_WRITE_STATUS" });
    expect(f).toHaveBeenCalledOnce();
    const rev1WithPrevious = record({ previous_id: auditId });
    await expect(createReportArchivesApi(fetcher(rev1WithPrevious)).read(brand, archiveId)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const badYearTimestamp = record({ snapshot_at: "0000-01-01T00:00:00Z" });
    await expect(createReportArchivesApi(fetcher(badYearTimestamp)).read(brand, archiveId)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const storedTab = record({ reason: "prior\tarchive" });
    await expect(createReportArchivesApi(fetcher(storedTab)).read(brand, archiveId)).resolves.toMatchObject({ reason: "prior\tarchive" });
    const utcYearZeroBoundary = record({ snapshot: snapshot({ from: "0001-01-01T00:00:00+14:00" }) });
    await expect(createReportArchivesApi(fetcher(utcYearZeroBoundary)).read(brand, archiveId)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const beforeWindowEnd = "0001-01-01T23:59:59.999999999Z";
    const premature = record({ snapshot_at: beforeWindowEnd, created_at: beforeWindowEnd,
      snapshot: snapshot({ snapshot_at: beforeWindowEnd, wallet_snapshot: { at_snapshot: beforeWindowEnd, balances: snapshot().wallet_snapshot.balances } }) });
    await expect(createReportArchivesApi(fetcher(premature)).read(brand, archiveId)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const utcYearTenThousandBoundary = "9999-12-31T23:59:59-14:00";
    const overflowSnapshot = snapshot({ snapshot_at: utcYearTenThousandBoundary, wallet_snapshot: { at_snapshot: utcYearTenThousandBoundary, balances: snapshot().wallet_snapshot.balances } });
    await expect(createReportArchivesApi(fetcher(record({ snapshot_at: utcYearTenThousandBoundary, created_at: utcYearTenThousandBoundary, snapshot: overflowSnapshot }))).read(brand, archiveId))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("returns canonical download bytes only after validating all headers, byte length, SHA, and prior snapshot", async () => {
    const s = snapshot();
    // Deliberately exercise the supported year 0001 and nanosecond parser in the archive window.
    const value = record({ snapshot: s });
    const wireSnapshot = {
      ...s,
      snapshot_at: "2026-04-03T00:00:00.123456+00:00",
      from: "0001-01-01T00:00:00.000000001+00:00",
      to: "0001-01-02T00:00:00+00:00",
      wallet_snapshot: { ...s.wallet_snapshot, at_snapshot: "2026-04-03T00:00:00.123456+00:00" },
    };
    const bytes = new TextEncoder().encode(JSON.stringify(wireSnapshot));
    const sha = [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))].map((b) => b.toString(16).padStart(2, "0")).join("");
    const headers = {
      "Content-Type": "application/json; charset=utf-8", "Content-Disposition": `attachment; filename="report-archive-${archiveId}-v1.json"`,
      "Content-Length": String(bytes.length), "X-Content-SHA256": sha, "X-Archive-Revision": "1", "X-Archive-Format-Version": "1",
    };
    value.payload_sha256 = sha;
    const f = vi.fn<typeof fetch>().mockResolvedValue(new Response(bytes, { status: 200, headers }));
    const result = await createReportArchivesApi(f).download(brand, archiveId, value);
    expect(result.bytes).toEqual(bytes);
    expect(new TextDecoder().decode(result.bytes)).toContain("+00:00");
    expect(result.metadata).toEqual({ record: value, sha256: sha, revision: 1, format_version: 1, filename: `report-archive-${archiveId}-v1.json` });
    const [url, init] = f.mock.calls[0]!;
    expect(url).toBe(`${"/api/v1/admin/report-archives"}/${archiveId}/download`);
    expect(init?.method).toBe("GET");
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    const createdDownload = vi.fn<typeof fetch>().mockResolvedValue(new Response(bytes, { status: 201, headers }));
    await expect(createReportArchivesApi(createdDownload).download(brand, archiveId, value)).rejects.toMatchObject({ code: "INVALID_RESPONSE", status: 502 });

    const plusEight = {
      ...s, snapshot_at: "2026-04-03T08:00:00.123456+08:00",
      from: "0001-01-01T08:00:00.000000001+08:00", to: "0001-01-02T08:00:00+08:00",
      wallet_snapshot: { ...s.wallet_snapshot, at_snapshot: "2026-04-03T08:00:00.123456+08:00" },
    };
    const plusEightBytes = new TextEncoder().encode(JSON.stringify(plusEight));
    const plusEightSha = [...new Uint8Array(await crypto.subtle.digest("SHA-256", plusEightBytes))].map((b) => b.toString(16).padStart(2, "0")).join("");
    const plusEightHeaders = { ...headers, "Content-Length": String(plusEightBytes.length), "X-Content-SHA256": plusEightSha };
    const plusEightResponse = vi.fn<typeof fetch>().mockResolvedValue(new Response(plusEightBytes, { status: 200, headers: plusEightHeaders }));
    await expect(createReportArchivesApi(plusEightResponse).download(brand, archiveId, { ...value, payload_sha256: plusEightSha })).resolves.toMatchObject({ metadata: { sha256: plusEightSha } });

    const truncated = vi.fn<typeof fetch>().mockResolvedValue(new Response(bytes.slice(0, -1), { status: 200, headers }));
    await expect(createReportArchivesApi(truncated).download(brand, archiveId, value)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const forgedHeader = vi.fn<typeof fetch>().mockResolvedValue(new Response(bytes, { status: 200, headers: { ...headers, "X-Content-SHA256": "b".repeat(64) } }));
    await expect(createReportArchivesApi(forgedHeader).download(brand, archiveId, value)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const mismatch = vi.fn<typeof fetch>().mockResolvedValue(new Response(bytes, { status: 200, headers }));
    await expect(createReportArchivesApi(mismatch).download(brand, archiveId, { ...value, snapshot: { ...s, timezone: "Asia/Tokyo" } })).rejects.toMatchObject({ code: "INVALID_INPUT" });

    const oneNanosecondLater = { ...wireSnapshot, snapshot_at: "2026-04-03T00:00:00.123456001+00:00", wallet_snapshot: { ...wireSnapshot.wallet_snapshot, at_snapshot: "2026-04-03T00:00:00.123456001+00:00" } };
    const differentBytes = new TextEncoder().encode(JSON.stringify(oneNanosecondLater));
    const differentSha = [...new Uint8Array(await crypto.subtle.digest("SHA-256", differentBytes))].map((b) => b.toString(16).padStart(2, "0")).join("");
    const differentHeaders = { ...headers, "Content-Length": String(differentBytes.length), "X-Content-SHA256": differentSha };
    const nanosecondMismatch = vi.fn<typeof fetch>().mockResolvedValue(new Response(differentBytes, { status: 200, headers: differentHeaders }));
    await expect(createReportArchivesApi(nanosecondMismatch).download(brand, archiveId, { ...value, payload_sha256: differentSha })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("marks network, malformed success, and server failures on create as unknown", async () => {
    const body = { kind: "daily" as const, period_key: "2026-04-03", expected_revision: 0, reason: "capture" };
    const network = vi.fn<typeof fetch>().mockRejectedValue(new Error("offline"));
    await expect(createReportArchivesApi(network).create(brand, body, "archive-key-0001", actor)).rejects.toMatchObject({ status: 0, code: "UNKNOWN_WRITE_STATUS" });
    const server = vi.fn<typeof fetch>().mockResolvedValue(new Response("{}", { status: 503 }));
    await expect(createReportArchivesApi(server).create(brand, body, "archive-key-0001", actor)).rejects.toMatchObject({ status: 0, code: "UNKNOWN_WRITE_STATUS" });
    const invalid = vi.fn<typeof fetch>().mockResolvedValue(response(record({ revision: 2 }), 201));
    await expect(createReportArchivesApi(invalid).create(brand, body, "archive-key-0001", actor)).rejects.toBeInstanceOf(AdminApiError);
  });

  it("freezes create and download validation baselines before fetch resolves", async () => {
    const input = { kind: "daily" as const, period_key: "2026-04-03", expected_revision: 0, reason: "original reason" };
    const createReceipt = record({ reason: input.reason, window: { ...record().window, period_key: input.period_key } });
    let releaseCreate!: (r: Response) => void;
    const createFetch = vi.fn<typeof fetch>(() => new Promise((resolve) => { releaseCreate = resolve; }));
    const pendingCreate = createReportArchivesApi(createFetch).create(brand, input, "archive-key-0001", actor);
    input.reason = "mutated reason";
    input.period_key = "2026-04-04";
    input.expected_revision = 1;
    releaseCreate(response(createReceipt, 201));
    await expect(pendingCreate).resolves.toMatchObject({ reason: "original reason", window: { period_key: "2026-04-03" } });

    const body = snapshot();
    const bytes = new TextEncoder().encode(JSON.stringify(body));
    const sha = [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))].map((b) => b.toString(16).padStart(2, "0")).join("");
    const prior = record({ payload_sha256: sha });
    const downloadHeaders = {
      "Content-Type": "application/json; charset=utf-8", "Content-Disposition": `attachment; filename="report-archive-${archiveId}-v1.json"`,
      "Content-Length": String(bytes.length), "X-Content-SHA256": sha, "X-Archive-Revision": "1", "X-Archive-Format-Version": "1",
    };
    let releaseDownload!: (r: Response) => void;
    const downloadFetch = vi.fn<typeof fetch>(() => new Promise((resolve) => { releaseDownload = resolve; }));
    const pendingDownload = createReportArchivesApi(downloadFetch).download(brand, archiveId, prior);
    prior.revision = 2;
    prior.payload_sha256 = "b".repeat(64);
    prior.snapshot.timezone = "Asia/Tokyo";
    releaseDownload(new Response(bytes, { status: 200, headers: downloadHeaders }));
    await expect(pendingDownload).resolves.toMatchObject({ metadata: { sha256: sha, revision: 1 } });
  });
});
