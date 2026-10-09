import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import {
  commissionCyclePermissions,
  createCommissionCyclesApi,
  type CommissionCalculation,
  type CommissionCycleRecord,
  type CommissionDiscovery,
  type CommissionRun,
} from "./commission-cycles-api";

const brand = "11111111-1111-4111-8111-111111111111";
const otherBrand = "22222222-2222-4222-8222-222222222222";
const cycleId = "33333333-3333-4333-8333-333333333333";
const runId = "44444444-4444-4444-8444-444444444444";
const discoveryId = "55555555-5555-4555-8555-555555555555";
const orderId = "66666666-6666-4666-8666-666666666666";
const memberId = "77777777-7777-4777-8777-777777777777";
const accountId = "88888888-8888-4888-8888-888888888888";
const auditId = "99999999-9999-4999-8999-999999999999";
const key = "idem_key_123";
const date = "2026-10-06T00:00:00Z";

const calendar = { timezone: "Asia/Singapore", cycle: "weekly", boundary_time: "06:30:00", weekday: 1, month_day: null, short_month: "" } as const;
const cycle: CommissionCycleRecord = {
  id: cycleId, brand_id: brand, window_from: "2026-09-28T06:30:00Z", window_to: "2026-10-05T06:30:00Z", anchor_order_id: orderId,
  calendar, state: "enumerating", version: 1, target_count: "9007199254740993", scan_complete: false,
  current_run_id: null, current_generation: null, evidence_epoch: null, evidence_current: false,
  calculated_count: "0", earning_count: "0", total_points: "0", created_by: accountId, creation_actor_type: "admin",
  reason: "Reviewed", created_at: date, updated_at: date, last_error_code: null, creation_audit_log_id: auditId,
};
const run: CommissionRun = {
  id: runId, brand_id: brand, cycle_id: cycleId, generation: "9223372036854775807", evidence_epoch: "0", state: "ready",
  calculated_count: "9007199254740993", earning_count: "1", total_points: "9223372036854775808", created_at: date,
};
const calculation: CommissionCalculation = {
  id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", brand_id: brand, cycle_id: cycleId, run_id: runId, order_id: orderId,
  member_id: memberId, reason: "eligible", status: "won", stake_points: "1", prize_points: "0", base_points: "1",
  job_id: null, calculation_id: null, generation: null, audit_log_id: auditId, created_at: date,
};
const discovery: CommissionDiscovery = {
  id: discoveryId, brand_id: brand, state: "pending", version: 1, cycle_id: null, window_from: null, window_to: null,
  next_check_at: date, last_error_code: null, last_audit_log_id: null, created_at: date, updated_at: date,
};
const allocation = {
  brand_id: brand, cycle_id: cycleId, run_id: runId, calculation_id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
  order_id: orderId, agent_id: accountId, member_id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaab", bettor_member_id: memberId,
  base_points: "100", mode: "loss", agent_ratio: "0.300000", downstream_ratio: "0.1", difference_ratio: "0.2",
  exact_amount: { numerator: "20", denominator: "1" }, created_at: date,
};
const ok = (data: unknown, status = 200) => new Response(JSON.stringify({ success: true, data }), { status });
const page = (items: unknown[], extra: Record<string, unknown> = {}) => ({ brand_id: brand, ...extra, items, total_count: "9007199254740993", limit: 20, offset: 0 });

describe("commission cycles API", () => {
  it("uses brand and platform view grants while keeping super-admin read-only", () => {
    const base = { id: accountId, super_admin: false, brand_ids: [brand], permissions: [] } satisfies AdminAccount;
    expect(commissionCyclePermissions({ ...base, permissions_by_brand: { [brand]: ["commission.view.brand", "commission.run.brand", "commission.retry.brand"] } }, brand)).toEqual({ view: true, run: true, retry: true });
    expect(commissionCyclePermissions({ ...base, permissions_by_brand: { [otherBrand]: ["commission.view.brand", "commission.run.brand"] } }, brand)).toEqual({ view: false, run: false, retry: false });
    expect(commissionCyclePermissions({ ...base, super_admin: true, platform_permissions: ["commission.view.platform"], permissions_by_brand: { [brand]: ["commission.run.brand", "commission.retry.brand"] } }, brand)).toEqual({ view: true, run: false, retry: false });
    expect(commissionCyclePermissions({ ...base, platform_permissions: ["commission.view.platform"] }, "")).toEqual({ view: false, run: false, retry: false });
    expect(commissionCyclePermissions({ ...base, brand_ids: [], permissions_by_brand: { [brand]: ["commission.view.brand", "commission.run.brand"] } }, brand)).toEqual({ view: false, run: false, retry: false });
    expect(commissionCyclePermissions({ ...base, brand_ids: [], permissions_by_brand: { [brand]: ["commission.run.brand"] }, platform_permissions: ["commission.view.platform"] }, brand)).toEqual({ view: true, run: false, retry: false });
    expect(commissionCyclePermissions({ ...base, id: "bad", platform_permissions: ["commission.view.platform"] }, brand)).toEqual({ view: false, run: false, retry: false });
  });

  it("calls all read routes with same-origin credentials, explicit brand, and echoed pagination", async () => {
    const earning = { id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaab", brand_id: brand, cycle_id: cycleId, run_id: runId,
      agent_id: accountId, member_id: memberId, exact_amount: { numerator: "900719925474099312347", denominator: "1000000" }, points: "900719925474099", created_at: date };
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(ok(page([cycle])))
      .mockResolvedValueOnce(ok(cycle))
      .mockResolvedValueOnce(ok(page([earning], { cycle_id: cycleId })))
      .mockResolvedValueOnce(ok(page([run], { cycle_id: cycleId })))
      .mockResolvedValueOnce(ok(page([calculation], { cycle_id: cycleId, run_id: runId })))
      .mockResolvedValueOnce(ok(page([discovery])));
    const api = createCommissionCyclesApi(fetcher);
    await expect(api.list(brand)).resolves.toMatchObject({ total_count: "9007199254740993", items: [{ target_count: "9007199254740993" }] });
    await expect(api.read(brand, cycleId)).resolves.toEqual(cycle);
    await expect(api.earnings(brand, cycleId)).resolves.toMatchObject({ items: [{ exact_amount: earning.exact_amount, points: earning.points }] });
    await expect(api.runs(brand, cycleId)).resolves.toEqual(page([run], { cycle_id: cycleId }));
    await expect(api.calculations(brand, cycleId, runId)).resolves.toEqual(page([calculation], { cycle_id: cycleId, run_id: runId }));
    await expect(api.discoveries(brand)).resolves.toMatchObject({ items: [discovery] });
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
      "/api/v1/admin/commission-cycles?limit=20&offset=0",
      `/api/v1/admin/commission-cycles/${cycleId}`,
      `/api/v1/admin/commission-cycles/${cycleId}/earnings?limit=20&offset=0`,
      `/api/v1/admin/commission-cycles/${cycleId}/runs?limit=20&offset=0`,
      `/api/v1/admin/commission-cycles/${cycleId}/runs/${runId}/calculations?limit=20&offset=0`,
      "/api/v1/admin/commission-discovery?limit=20&offset=0",
    ]);
    for (const [, init] of fetcher.mock.calls) {
      expect(init?.credentials).toBe("same-origin");
      expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    }
  });

  it("reads earnings and exact allocations for an explicit run with validated filters and ordering", async () => {
    const earning = { id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaac", brand_id: brand, cycle_id: cycleId, run_id: runId,
      agent_id: accountId, member_id: memberId, exact_amount: { numerator: "1", denominator: "1" }, points: "1", created_at: date };
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(ok(page([earning], { cycle_id: cycleId, run_id: runId })))
      .mockResolvedValueOnce(ok(page([allocation], { cycle_id: cycleId, run_id: runId, agent_id: accountId, order_id: orderId })));
    const api = createCommissionCyclesApi(fetcher);
    await expect(api.runEarnings(brand, cycleId, runId)).resolves.toMatchObject({ run_id: runId, items: [earning] });
    await expect(api.allocations(brand, cycleId, runId, { limit: 20, offset: 0, agent_id: accountId, order_id: orderId }))
      .resolves.toMatchObject({ run_id: runId, agent_id: accountId, order_id: orderId, items: [allocation] });
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
      `/api/v1/admin/commission-cycles/${cycleId}/runs/${runId}/earnings?limit=20&offset=0`,
      `/api/v1/admin/commission-cycles/${cycleId}/runs/${runId}/allocations?limit=20&offset=0&agent_id=${accountId}&order_id=${orderId}`,
    ]);
    for (const [, init] of fetcher.mock.calls) {
      expect(init?.method ?? "GET").toBe("GET");
      expect(init?.credentials).toBe("same-origin");
      expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    }
  });

  it("accepts equal beneficiary and bettor member IDs without relaxing allocation validation", async () => {
    const sameMember = { ...allocation, member_id: memberId, bettor_member_id: memberId };
    await expect(createCommissionCyclesApi(vi.fn<typeof fetch>().mockResolvedValue(ok(page([sameMember], {
      cycle_id: cycleId, run_id: runId, agent_id: null, order_id: null,
    })))).allocations(brand, cycleId, runId)).resolves.toMatchObject({ items: [sameMember] });
  });

  it("orders run earnings by precise RFC3339 fractions before using contrary UUID order as a tie-breaker", async () => {
    const newer = { id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", brand_id: brand, cycle_id: cycleId, run_id: runId,
      agent_id: accountId, member_id: memberId, exact_amount: { numerator: "1", denominator: "1" }, points: "1", created_at: "2026-10-06T00:00:00.000002+00:00" };
    const older = { ...newer, id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", created_at: "2026-10-06T00:00:00.000001Z" };
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(ok(page([newer, older], { cycle_id: cycleId, run_id: runId })))
      .mockResolvedValueOnce(ok(page([{ ...newer, created_at: older.created_at }, { ...older, created_at: newer.created_at }], { cycle_id: cycleId, run_id: runId })));
    const api = createCommissionCyclesApi(fetcher);
    await expect(api.runEarnings(brand, cycleId, runId)).resolves.toMatchObject({ items: [newer, older] });
    await expect(api.runEarnings(brand, cycleId, runId)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("rejects malformed, unsorted, or mis-scoped run history and non-strict allocation queries", async () => {
    const first = { id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaab", brand_id: brand, cycle_id: cycleId, run_id: runId,
      agent_id: accountId, member_id: memberId, exact_amount: { numerator: "1", denominator: "1" }, points: "1", created_at: date };
    const second = { ...first, id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" };
    const api = createCommissionCyclesApi(vi.fn<typeof fetch>()
      .mockResolvedValueOnce(ok(page([first, second], { cycle_id: cycleId, run_id: runId })))
      .mockResolvedValueOnce(ok(page([{ ...allocation, difference_ratio: "0.3" }], { cycle_id: cycleId, run_id: runId, agent_id: null, order_id: null })))
      .mockResolvedValueOnce(ok(page([{ ...allocation, saved_snapshot: {} }], { cycle_id: cycleId, run_id: runId, agent_id: null, order_id: null }))));
    await expect(api.runEarnings(brand, cycleId, runId)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(api.allocations(brand, cycleId, runId)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(api.allocations(brand, cycleId, runId)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(api.allocations(brand, cycleId, runId, { agent_id: accountId, extra: true } as never))
      .rejects.toMatchObject({ status: 0, code: "INVALID_INPUT" });
  });

  it("normalizes the legacy missing actor only on manual create receipts", async () => {
    const legacy = { ...cycle } as Record<string, unknown>;
    delete legacy.creation_actor_type;
    const receipt = { ...legacy, state: "enumerating", version: 1, anchor_order_id: orderId, reason: "Reviewed" };
    await expect(createCommissionCyclesApi(vi.fn<typeof fetch>().mockResolvedValue(ok(receipt, 201)))
      .create(brand, { anchor_order_id: orderId, reason: "Reviewed" }, key, accountId))
      .resolves.toMatchObject({ created_by: accountId, creation_actor_type: "admin" });
    await expect(createCommissionCyclesApi(vi.fn<typeof fetch>().mockResolvedValue(ok(legacy))).read(brand, cycleId))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    const invalidLegacy = { ...legacy, created_by: null };
    await expect(createCommissionCyclesApi(vi.fn<typeof fetch>().mockResolvedValue(ok(invalidLegacy))).read(brand, cycleId))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it.each([
    ["cross-brand cycle", { ...cycle, brand_id: otherBrand }],
    ["unsafe version", { ...cycle, version: Number.MAX_SAFE_INTEGER + 1 }],
    ["noncanonical calendar date", { ...cycle, window_from: "2026-02-30T00:00:00Z" }],
    ["open projection", { ...cycle, saved_snapshot: { private: true } }],
    ["incorrect null run relations", { ...cycle, current_run_id: runId }],
    ["invalid calendar combination", { ...cycle, calendar: { ...calendar, weekday: null } }],
    ["invalid bigint count", { ...cycle, target_count: "9223372036854775808" }],
  ])("rejects malformed cycle reads (%s)", async (_label, value) => {
    await expect(createCommissionCyclesApi(vi.fn<typeof fetch>().mockResolvedValue(ok(value))).read(brand, cycleId))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("rejects wrong pagination echoes, duplicate ids, and cross-scope history rows", async () => {
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(ok({ ...page([cycle]), offset: 1 }))
      .mockResolvedValueOnce(ok(page([run, run], { cycle_id: cycleId })))
      .mockResolvedValueOnce(ok(page([{ ...calculation, run_id: cycleId }], { cycle_id: cycleId, run_id: runId })))
      .mockResolvedValueOnce(ok({ ...page([cycle]), total_count: "20" }));
    const api = createCommissionCyclesApi(fetcher);
    await expect(api.list(brand)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(api.runs(brand, cycleId)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(api.calculations(brand, cycleId, runId)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(api.list(brand, 20, 20)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("binds reads to requested ids and earnings to one run with reduced rounded fractions", async () => {
    const earning = { id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaab", brand_id: brand, cycle_id: cycleId, run_id: runId,
      agent_id: accountId, member_id: memberId, exact_amount: { numerator: "3", denominator: "2" }, points: "2", created_at: date };
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(ok({ ...cycle, id: runId }))
      .mockResolvedValueOnce(ok(page([{ ...earning, exact_amount: { numerator: "2", denominator: "2" }, points: "1" }], { cycle_id: cycleId })))
      .mockResolvedValueOnce(ok(page([earning, { ...earning, id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", run_id: cycleId }], { cycle_id: cycleId })))
      .mockResolvedValueOnce(ok(page([{ ...calculation, job_id: orderId }], { cycle_id: cycleId, run_id: runId })));
    const api = createCommissionCyclesApi(fetcher);
    await expect(api.read(brand, cycleId)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(api.earnings(brand, cycleId)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(api.earnings(brand, cycleId)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(api.calculations(brand, cycleId, runId)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("validates closed discovery relations and registered state", async () => {
    const registered = { ...discovery, state: "registered", version: 2, cycle_id: cycleId, window_from: cycle.window_from, window_to: cycle.window_to, last_audit_log_id: auditId };
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(ok(page([registered])))
      .mockResolvedValueOnce(ok(page([{ ...registered, cycle_id: null }])));
    const api = createCommissionCyclesApi(fetcher);
    await expect(api.discoveries(brand)).resolves.toMatchObject({ items: [registered] });
    await expect(api.discoveries(brand)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("accepts the migration 0049 initial pending row with a null audit id", async () => {
    await expect(createCommissionCyclesApi(vi.fn<typeof fetch>().mockResolvedValue(ok(page([discovery])))).discoveries(brand))
      .resolves.toMatchObject({ items: [{ state: "pending", version: 1, last_audit_log_id: null }] });
  });

  it("keeps failure discovery relation validation aligned with its SQL state contract", async () => {
    const failed = { ...discovery, state: "failed", version: 2, cycle_id: cycleId, last_error_code: "DISCOVERY_FAILED", last_audit_log_id: auditId };
    await expect(createCommissionCyclesApi(vi.fn<typeof fetch>().mockResolvedValue(ok(page([failed])))).discoveries(brand))
      .resolves.toMatchObject({ items: [{ state: "failed", cycle_id: cycleId }] });
  });

  it("enforces cycle SQL stage invariants while allowing stale ready evidence", async () => {
    const staleReady = { ...cycle, state: "ready", version: 2, scan_complete: true, current_run_id: runId, current_generation: "1", evidence_epoch: "0", evidence_current: false };
    const invalid = [
      { ...staleReady, last_error_code: "unexpected" },
      { ...staleReady, current_run_id: null, current_generation: null, evidence_epoch: null, calculated_count: "1" },
      { ...cycle, state: "waiting", scan_complete: false },
      { ...cycle, state: "calculating", scan_complete: true, current_run_id: null },
    ];
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok(staleReady));
    for (const bad of invalid) fetcher.mockResolvedValueOnce(ok(bad));
    const api = createCommissionCyclesApi(fetcher);
    await expect(api.read(brand, cycleId)).resolves.toMatchObject({ state: "ready", evidence_current: false });
    for (const _bad of invalid) await expect(api.read(brand, cycleId)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("creates and retries only when receipts match the requested transition", async () => {
    const created = { ...cycle, state: "enumerating", version: 1, current_run_id: null, current_generation: null, evidence_epoch: null };
    const retried = { ...cycle, version: 3, state: "waiting", scan_complete: true, current_run_id: null, current_generation: null, evidence_epoch: null, last_error_code: null };
    const retryDiscovery = { ...discovery, state: "pending", version: 4, cycle_id: null, last_error_code: null, last_audit_log_id: auditId };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok(created, 201)).mockResolvedValueOnce(ok(retried)).mockResolvedValueOnce(ok(retryDiscovery));
    const api = createCommissionCyclesApi(fetcher);
    await expect(api.create(brand, { anchor_order_id: orderId, reason: "Reviewed" }, key, accountId)).resolves.toMatchObject({ state: "enumerating" });
    await expect(api.retryCycle(brand, cycleId, { version: 2, reason: "Retry" }, key)).resolves.toMatchObject({ version: 3, state: "waiting" });
    await expect(api.retryDiscovery(brand, discoveryId, { version: 3, reason: "Retry" }, key)).resolves.toMatchObject({ version: 4, state: "pending" });
    for (const [, init] of fetcher.mock.calls) {
      expect(init?.method).toBe("POST");
      expect(new Headers(init?.headers).get("Idempotency-Key")).toBe(key);
      expect(new Headers(init?.headers).get("Content-Type")).toBe("application/json");
      expect(init?.credentials).toBe("same-origin");
    }
    expect(fetcher.mock.calls.map(([, init]) => JSON.parse(String(init?.body)))).toEqual([
      { anchor_order_id: orderId, reason: "Reviewed" }, { version: 2, reason: "Retry" }, { version: 3, reason: "Retry" },
    ]);
  });

  it.each([
    ["mismatched anchor", { ...cycle, anchor_order_id: memberId }],
    ["wrong create state", { ...cycle, state: "waiting" }],
    ["wrong actor", { ...cycle, created_by: otherBrand }],
  ])("treats malformed successful creates as unknown (%s)", async (_name, value) => {
    await expect(createCommissionCyclesApi(vi.fn<typeof fetch>().mockResolvedValue(ok(value, 201)))
      .create(brand, { anchor_order_id: orderId, reason: "Reviewed" }, key, accountId))
      .rejects.toMatchObject({ status: 0, code: "UNKNOWN_WRITE_STATUS" });
  });

  it("keeps uncertain writes at status zero but preserves definitive 4xx errors", async () => {
    const network = createCommissionCyclesApi(vi.fn<typeof fetch>().mockRejectedValue(new Error("offline")));
    await expect(network.create(brand, { anchor_order_id: orderId, reason: "Reviewed" }, key))
      .rejects.toMatchObject({ status: 0, code: "UNKNOWN_WRITE_STATUS" });
    for (const status of [400, 409]) {
      const api = createCommissionCyclesApi(vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: false, error: { code: "CONFLICT", message: "definitive" } }), { status })));
      await expect(api.retryCycle(brand, cycleId, { version: 2, reason: "Retry" }, key))
        .rejects.toMatchObject({ status, code: "CONFLICT" });
    }
    const outage = createCommissionCyclesApi(vi.fn<typeof fetch>().mockResolvedValue(new Response("unavailable", { status: 503 })));
    await expect(outage.retryDiscovery(brand, discoveryId, { version: 2, reason: "Retry" }, key))
      .rejects.toMatchObject({ status: 0, code: "UNKNOWN_WRITE_STATUS" });
  });

  it("rejects invalid local inputs without a request", async () => {
    const fetcher = vi.fn<typeof fetch>();
    const api = createCommissionCyclesApi(fetcher);
    await expect(api.list(brand, 101)).rejects.toBeInstanceOf(AdminApiError);
    await expect(api.read(brand, `${cycleId.slice(0, -1)}G`)).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(api.create(brand, { anchor_order_id: orderId, reason: "bad\ud800" }, key)).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(api.retryCycle(brand, cycleId, { version: Number.MAX_SAFE_INTEGER, reason: "Retry" }, key)).rejects.toMatchObject({ code: "INVALID_INPUT" });
    expect(fetcher).not.toHaveBeenCalled();
  });
});
