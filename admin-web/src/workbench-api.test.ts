import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { createWorkbenchApi, validWorkbenchSnapshot, workbenchPermissions, type WorkbenchCommissions, type WorkbenchSnapshot } from "./workbench-api";

const brand = "11111111-1111-4111-8111-111111111111";
const jobId = "33333333-3333-4333-8333-333333333333";
const stamp = "2026-10-07T10:00:00Z";
const commissionFields = ["discovery_pending_count", "discovery_failed_count", "cycle_processing_count", "cycle_waiting_count", "cycle_ready_count", "cycle_stale_count", "cycle_failed_count",
  "payment_awaiting_approval_count", "payment_processing_count", "payment_blocked_count", "payment_failed_count", "plan_processing_count", "plan_ready_count", "plan_failed_count",
  "execution_awaiting_approval_count", "execution_processing_count", "execution_paused_count", "execution_failed_count"] as const;
function commissions(value = "0"): WorkbenchCommissions { return Object.fromEntries(commissionFields.map((key, index) => [key, index === 0 ? value : "0"])) as unknown as WorkbenchCommissions; }

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
    withdrawals: ready({ reviewing_count: "2", reviewing_points: "500", processing_count: "1", processing_points: "900719925474099312345" }), commissions: ready(commissions()), rewards: ready({ granted_count: "1", pending_count: "2", revoked_count: "3" }),
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

  it("enforces current section statuses and data nullability", () => {
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
    expect(validWorkbenchSnapshot({ ...value, rewards: { status: "forbidden", data: null } }, brand)).toBe(true);
    expect(validWorkbenchSnapshot({ ...value, rewards: { status: "not_implemented", data: null } }, brand)).toBe(false);
    for (const invalid of ["01", "-1", 1, "1e3"]) {
      expect(validWorkbenchSnapshot({ ...value, rewards: { status: "ready", data: { ...value.rewards.data!, pending_count: invalid } } }, brand)).toBe(false);
    }
    expect(validWorkbenchSnapshot({ ...value, rewards: { status: "ready", data: { ...value.rewards.data!, gift_points: "10" } } }, brand)).toBe(false);
  });

  it("validates current commission counters and rejects old or unknown response shapes", async () => {
    const value = snapshot();
    const ready = { ...value, commissions: { status: "ready" as const, data: commissions("900719925474099312345") } };
    expect(validWorkbenchSnapshot(ready, brand)).toBe(true);
    await expect(createWorkbenchApi(vi.fn<typeof fetch>().mockResolvedValue(response(ready))).get(brand)).resolves.toEqual(ready);
    expect(validWorkbenchSnapshot(value, brand)).toBe(true);
    expect(validWorkbenchSnapshot({ ...value, commissions: { status: "forbidden", data: null } }, brand)).toBe(true);
    expect(validWorkbenchSnapshot({ ...value, commissions: { status: "not_implemented", data: null } }, brand)).toBe(false);
    expect(validWorkbenchSnapshot({ ...value, commissions: { status: "not_implemented", data: { ...commissions() } } }, brand)).toBe(false);
    expect(validWorkbenchSnapshot({ ...value, commissions: { status: "ready", data: {} } }, brand)).toBe(false);
    const obsoleteCount = { ...commissions() } as Record<string, string>;
    obsoleteCount.plan_blocked_count = "0";
    expect(validWorkbenchSnapshot({ ...value, commissions: { status: "ready", data: obsoleteCount } }, brand)).toBe(false);
    expect(validWorkbenchSnapshot({ ...value, commissions: { status: "ready", data: { ...commissions(), private_agent_id: jobId } } }, brand)).toBe(false);
    for (const invalid of ["01", "-1", "1e3", 1, null]) {
      expect(validWorkbenchSnapshot({ ...value, commissions: { status: "ready", data: { ...commissions(), cycle_stale_count: invalid } } }, brand)).toBe(false);
    }
    expect(validWorkbenchSnapshot({ ...value, commissions: { status: "ready", data: { ...commissions(), cycle_stale_count: "900719925474099312345" } } }, brand)).toBe(true);
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
    expect(workbenchPermissions(account({ permissions_by_brand: { [brand]: ["reward.view.brand"] } }), brand).rewards).toBe(true);
    expect(workbenchPermissions(account({ brand_ids: [], permissions_by_brand: {}, platform_permissions: ["reward.view.platform"] }), brand).rewards).toBe(true);
    expect(workbenchPermissions(account({ permissions_by_brand: { [brand]: ["wallet.view.brand", "report_reward.view.brand"] } }), brand).rewards).toBe(false);
  });

  it("gates commissions only on commission view in the matching brand or platform scope", () => {
    expect(workbenchPermissions(account({ permissions_by_brand: { [brand]: ["commission.view.brand"] } }), brand).commissions).toBe(true);
    for (const permission of ["wallet.view.brand", "report_commission.view.brand"]) {
      expect(workbenchPermissions(account({ permissions_by_brand: { [brand]: [permission] }, platform_permissions: [] }), brand).commissions).toBe(false);
    }
    expect(workbenchPermissions(account({ brand_ids: [], permissions_by_brand: { [brand]: ["commission.view.brand"] }, platform_permissions: [] }), brand).commissions).toBe(false);
    expect(workbenchPermissions(account({ brand_ids: [], permissions_by_brand: {}, platform_permissions: ["commission.view.platform"] }), brand).commissions).toBe(true);
    expect(workbenchPermissions(account({ brand_ids: [], permissions_by_brand: {}, platform_permissions: ["wallet.view.platform", "report_commission.view.platform"] }), brand).commissions).toBe(false);
    expect(workbenchPermissions(account({ brand_ids: [], permissions_by_brand: {}, permissions: ["commission.view.platform"], platform_permissions: [] }), brand).commissions).toBe(false);
    expect(workbenchPermissions(account({ super_admin: true, brand_ids: [], permissions_by_brand: {}, platform_permissions: [] }), brand).commissions).toBe(false);
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
