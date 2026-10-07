import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { createWorkbenchApi, validWorkbenchSnapshot, workbenchPermissions, type WorkbenchSnapshot } from "./workbench-api";

const brand = "11111111-1111-4111-8111-111111111111";
const jobId = "33333333-3333-4333-8333-333333333333";
const stamp = "2026-10-07T10:00:00Z";

function snapshot(patch: Partial<WorkbenchSnapshot> = {}): WorkbenchSnapshot {
  const ready = <T>(data: T) => ({ status: "ready" as const, data });
  return {
    brand_id: brand, snapshot_at: stamp, timezone: "UTC", day_from: "2026-10-07T00:00:00Z",
    brand: ready({ name: "Example", code: "EX", state: "active" }),
    periods: ready({ pending: "0", betting: "4", closed: "1", waiting_draw: "2", drawn: "3", settling: "0", refund_pending: "0", refund_failed: "0" }),
    orders: ready({ placed: "900719925474099312345", abnormal: "2" }),
    today_bets: ready({ order_count: "900719925474099312345", stake_points: "999999999999999999999", cancelled_count: "1", abnormal_count: "2" }),
    settlement: ready({ processing: "1", awaiting_approval: "0", paying: "0", failed: "0" }),
    recharges: ready({ pending_count: "3", pending_points: "999999999999999999999" }),
    ledger: ready({ entry_count: "8", net_points: "-999999999999999999999", recharge_points: "2", prize_credit_points: "3", prize_reversal_points: "4", refund_points: "5" }),
    balances: ready({ account_count: "10", available_points: "11", frozen_points: "12", withdrawal_points: "13", total_points: "36" }),
    reconciliation: ready({ latest_job: { id: jobId, state: "completed", created_at: "2026-10-07T09:00:00Z", completed_at: stamp, target_count: "2", checked_count: "2", repairable_count: "1", corrupt_count: "0", failed_count: "0" } }),
    sources: ready({ adapter_state: "stub", configured_games: "1", enabled_api_sources: "2", enabled_dom_sources: "0", attempts_today: "3", failed_today: "1", no_data_today: "1", last_attempt_at: "2026-10-07T09:30:00Z" }),
    withdrawals: ready({ reviewing_count: "2", reviewing_points: "500", processing_count: "1", processing_points: "900719925474099312345" }), commissions: { status: "not_implemented", data: null }, rewards: { status: "not_implemented", data: null },
    ...patch,
  };
}
function response(data: unknown, status = 200) {
  return new Response(JSON.stringify({ success: true, data, request_id: "test-workbench-request" }), { status, headers: { "Content-Type": "application/json" } });
}
function account(patch: Partial<AdminAccount> = {}): AdminAccount {
  return { id: "22222222-2222-4222-8222-222222222222", super_admin: false, brand_ids: [brand], permissions: [],
    permissions_by_brand: { [brand]: ["period.view.brand", "bet.view.brand", "wallet.view.brand"] }, ...patch };
}

describe("workbench SDK", () => {
  it("sends only a same-origin GET with the required brand header and validates the complete snapshot", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response(snapshot()));
    await expect(createWorkbenchApi(fetcher).get(brand)).resolves.toEqual(snapshot());
    expect(fetcher).toHaveBeenCalledOnce();
    expect(fetcher).toHaveBeenCalledWith("/api/v1/admin/workbench", expect.objectContaining({ method: "GET", credentials: "same-origin" }));
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe("/api/v1/admin/workbench");
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    expect(init?.body).toBeUndefined();
    expect(Object.keys(init ?? {}).sort()).toEqual(["credentials", "headers", "method", "signal"].sort());
  });

  it("keeps large financial values as decimal strings and permits signed ledger net only", async () => {
    const value = snapshot();
    expect(validWorkbenchSnapshot(value, brand)).toBe(true);
    expect(value.ledger.data?.net_points).toBe("-999999999999999999999");
    for (const damaged of [
      { ...value, ledger: { ...value.ledger, data: { ...value.ledger.data!, net_points: "-0" } } },
      { ...value, balances: { ...value.balances, data: { ...value.balances.data!, total_points: "1e4" } } },
      { ...value, orders: { ...value.orders, data: { ...value.orders.data!, placed: "01" } } },
      { ...value, today_bets: { ...value.today_bets, data: { ...value.today_bets.data!, stake_points: "-1" } } },
    ]) expect(validWorkbenchSnapshot(damaged, brand)).toBe(false);
  });

  it("rejects unknown keys, bad scope, invalid windows, and invalid timestamps or timezone", () => {
    const value = snapshot();
    expect(validWorkbenchSnapshot({ ...value, surprise: true }, brand)).toBe(false);
    expect(validWorkbenchSnapshot(value, "22222222-2222-4222-8222-222222222222")).toBe(false);
    expect(validWorkbenchSnapshot({ ...value, day_from: "2026-10-07T10:00:01Z" }, brand)).toBe(false);
    expect(validWorkbenchSnapshot({ ...value, day_from: "2026-10-07T00:30:00Z" }, brand)).toBe(true);
    expect(validWorkbenchSnapshot({ ...value, day_from: "2026-10-06T07:59:59Z" }, brand)).toBe(false);
    expect(validWorkbenchSnapshot({ ...value, timezone: "Mars/Olympus" }, brand)).toBe(false);
    expect(validWorkbenchSnapshot({ ...value, snapshot_at: "yesterday" }, brand)).toBe(false);
    expect(validWorkbenchSnapshot({ ...value, snapshot_at: "2026-02-30T10:00:00Z" }, brand)).toBe(false);
    expect(validWorkbenchSnapshot({ ...value, periods: { ...value.periods, extra: "0" } }, brand)).toBe(false);
  });

  it("accepts backend UUID syntax without imposing version or variant bits", async () => {
    const legacyBrand = "00000000-0000-0000-0000-000000000001";
    const data = snapshot({ brand_id: legacyBrand });
    expect(validWorkbenchSnapshot(data, legacyBrand)).toBe(true);
    await expect(createWorkbenchApi(vi.fn<typeof fetch>().mockResolvedValue(response(data))).get(legacyBrand)).resolves.toEqual(data);
  });

  it("enforces section status and data nullability, keeping only commissions and rewards unimplemented", () => {
    const value = snapshot();
    expect(validWorkbenchSnapshot({ ...value, balances: { status: "forbidden", data: null } }, brand)).toBe(true);
    expect(validWorkbenchSnapshot({ ...value, balances: { status: "forbidden", data: value.balances.data } }, brand)).toBe(false);
    expect(validWorkbenchSnapshot({ ...value, balances: { status: "not_implemented", data: null } }, brand)).toBe(false);
    expect(validWorkbenchSnapshot({ ...value, sources: { status: "ready", data: { ...value.sources.data!, adapter_state: "healthy" } } }, brand)).toBe(false);
    expect(validWorkbenchSnapshot({ ...value, withdrawals: { status: "forbidden", data: null } }, brand)).toBe(true);
    expect(validWorkbenchSnapshot({ ...value, withdrawals: { status: "not_implemented", data: null } }, brand)).toBe(false);
    for (const invalid of ["01", "-1", "1e3"]) {
      expect(validWorkbenchSnapshot({ ...value, withdrawals: { status: "ready", data: { ...value.withdrawals.data!, reviewing_points: invalid } } }, brand)).toBe(false);
    }
    expect(validWorkbenchSnapshot({ ...value, rewards: { status: "ready", data: {} } }, brand)).toBe(false);
  });

  it("validates latest job lifecycle, counters, and cross-field logical coherence", () => {
    const value = snapshot();
    const recon = value.reconciliation.data!;
    const job = recon.latest_job!;
    for (const invalidJob of [
      { ...job, state: "unknown" },
      { ...job, checked_count: "3" },
      { ...job, repairable_count: "3" },
      { ...job, completed_at: null },
      { ...job, created_at: "2026-10-07T11:00:00Z" },
      { ...job, id: "bad-id" },
      { ...job, state: "completed", checked_count: "1" },
    ]) expect(validWorkbenchSnapshot({ ...value, reconciliation: { status: "ready", data: { latest_job: invalidJob } } }, brand)).toBe(false);
    expect(validWorkbenchSnapshot({ ...value, reconciliation: { status: "ready", data: { latest_job: null } } }, brand)).toBe(true);
    const failedJob = { ...job, state: "failed", completed_at: null, target_count: "3", checked_count: "1", failed_count: "1" };
    expect(validWorkbenchSnapshot({ ...value, reconciliation: { status: "ready", data: { latest_job: failedJob } } }, brand)).toBe(true);
    const pendingJob = { ...job, state: "pending", completed_at: null, target_count: "3", checked_count: "1", failed_count: "0" };
    expect(validWorkbenchSnapshot({ ...value, reconciliation: { status: "ready", data: { latest_job: pendingJob } } }, brand)).toBe(true);
    expect(validWorkbenchSnapshot({ ...value, sources: { status: "ready", data: { ...value.sources.data!, attempts_today: "1", failed_today: "1", no_data_today: "1" } } }, brand)).toBe(false);
    expect(validWorkbenchSnapshot({ ...value, today_bets: { status: "ready", data: { ...value.today_bets.data!, order_count: "900719925474099312346" } } }, brand)).toBe(true);
    expect(validWorkbenchSnapshot({ ...value, balances: { status: "ready", data: { ...value.balances.data!, total_points: "35" } } }, brand)).toBe(false);
  });

  it("derives links only from effective brand or platform view grants", () => {
    expect(workbenchPermissions(account(), brand)).toMatchObject({ periods: true, orders: true, today_bets: false, settlement: false, balances: false, ledger: false, withdrawals: false });
    const scoped = workbenchPermissions(account({ permissions_by_brand: { [brand]: ["report_betting.view.brand", "settlement.view.brand", "report_ledger.view.brand"] } }), brand);
    expect(scoped).toMatchObject({ orders: false, today_bets: true, settlement: true, balances: true, ledger: true });
    expect(workbenchPermissions(account({ brand_ids: [], permissions_by_brand: {}, platform_permissions: ["report_ledger.view.platform"] }), brand).balances).toBe(true);
    expect(workbenchPermissions(account({ permissions_by_brand: { [brand]: ["withdrawal.view.brand"] } }), brand).withdrawals).toBe(true);
    expect(workbenchPermissions(account({ brand_ids: [], permissions_by_brand: {}, platform_permissions: ["withdrawal.view.platform"] }), brand).withdrawals).toBe(true);
    expect(workbenchPermissions(account({ permissions_by_brand: { [brand]: ["wallet.view.brand"] }, platform_permissions: [] }), brand).withdrawals).toBe(false);
    expect(workbenchPermissions(account({ permissions_by_brand: { [brand]: [] }, platform_permissions: [] }), brand).orders).toBe(false);
  });

  it("rejects invalid identifiers, malformed envelopes, preserves HTTP errors, and supports cancellation", async () => {
    await expect(createWorkbenchApi(vi.fn<typeof fetch>()).get("bad")).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(createWorkbenchApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...snapshot(), extra: true }))).get(brand))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE", status: 502 });
    await expect(createWorkbenchApi(vi.fn<typeof fetch>().mockResolvedValue(new Response("{}", { status: 503 }))).get(brand))
      .rejects.toMatchObject({ status: 503 });
    const abort = new DOMException("Aborted", "AbortError");
    await expect(createWorkbenchApi(vi.fn<typeof fetch>().mockRejectedValue(abort)).get(brand)).rejects.toBe(abort);
    await expect(createWorkbenchApi(vi.fn<typeof fetch>().mockRejectedValue(new Error("offline"))).get(brand))
      .rejects.toBeInstanceOf(AdminApiError);
  });
});
