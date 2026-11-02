import { describe, expect, it, vi } from "vitest";
import type { AdminAccount } from "./admin-api";
import { commissionReportPermissions, createCommissionReportApi, type CommissionTotals } from "./commission-report-api";

const brand = "11111111-1111-4111-8111-111111111111", actor = "22222222-2222-4222-8222-222222222222", agent = "abcdefab-cdef-4abc-8def-abcdefabcdef", cycle = "44444444-4444-4444-8444-444444444444";
const from = "2026-01-01T00:00:00Z", to = "2026-01-02T00:00:00Z", huge = "922337203685477580812345678901234567890";
const totals: CommissionTotals = { entry_count: "3", paid_entry_count: "1", paid_points: huge, adjustment_entry_count: "1", adjustment_credit_points: "0", adjustment_debit_points: `${BigInt(huge) + 9n}`, correction_entry_count: "1", correction_credit_points: `${BigInt(huge) + 20n}`, correction_debit_points: `${BigInt(huge) + 20n}`, net_points: "-9" };
const payload = (overrides: Record<string, unknown> = {}) => ({ success: true, associations: { ["brand:" + brand]: { name: "Aurora" } }, request_id: "report-request-1", data: { brand_id: brand, snapshot_at: "2026-01-02T01:00:00Z", timezone: "Asia/Singapore", query: { from, to, group_by: "day", limit: 20, offset: 0, agent_id: null, member_id: null, cycle_id: null }, summary: totals, items: [{ key: "2026-01-01", label: "2026-01-01", totals }], total_groups: "1", ...overrides } });
const json = (value: unknown) => new Response(JSON.stringify(value), { status: 200, headers: { "content-type": "application/json" } });

describe("commission report permissions", () => {
  const account: AdminAccount = { id: actor, super_admin: false, brand_ids: [brand], permissions: [], permissions_by_brand: { [brand]: ["report_commission.view.brand"] } };
  it("requires mapped view and export grants and denies platform administrators", () => {
    expect(commissionReportPermissions(account, brand)).toEqual({ view: true, export: false });
    expect(commissionReportPermissions({ ...account, permissions_by_brand: { [brand]: ["report_commission.export.brand"] } }, brand)).toEqual({ view: false, export: false });
    expect(commissionReportPermissions({ ...account, permissions_by_brand: { [brand]: ["report_commission.view.brand", "report_commission.export.brand"] } }, brand)).toEqual({ view: true, export: true });
    expect(commissionReportPermissions({ ...account, brand_ids: [], permissions_by_brand: {}, platform_permissions: ["report_commission.view.platform", "report_commission.export.platform"] }, brand)).toEqual({ view: false, export: false });
    expect(commissionReportPermissions({ ...account, brand_ids: [], permissions_by_brand: { [brand]: ["report_commission.view.brand", "report_commission.export.brand"] } }, brand)).toEqual({ view: false, export: false });
    expect(commissionReportPermissions({ ...account, super_admin: true, permissions_by_brand: { [brand]: ["report_commission.view.brand", "report_commission.export.brand"] } }, brand)).toEqual({ view: false, export: false });
  });
});

describe("commission report API", () => {
  it("requests live data and preserves integers beyond int64 exactly", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(json(payload({ query: { from, to, group_by: "day", limit: 20, offset: 0, agent_id: agent, member_id: actor, cycle_id: cycle } })));
    const report = await createCommissionReportApi(fetcher).report(brand.toUpperCase(), { from, to, group_by: "day", agent_id: agent.toUpperCase(), member_id: actor.toUpperCase(), cycle_id: cycle.toUpperCase() });
    expect(report.summary.paid_points).toBe(huge);
    expect(report.summary.correction_credit_points).toBe(`${BigInt(huge) + 20n}`);
    expect(report.summary.net_points).toBe("-9");
    const [url, init] = fetcher.mock.calls[0]!;
    expect(String(url)).toContain("/api/v1/admin/reports/commission?");
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    expect(new URL(String(url), "http://local").searchParams.get("limit")).toBe("20");
    expect(new URL(String(url), "http://local").searchParams.get("agent_id")).toBe(agent);
  });
  it("rejects changed scope and malformed fields while accepting server-calculated totals", async () => {
    const api = createCommissionReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(payload())));
    await expect(api.report(brand, { from, to, group_by: "day", agent_id: agent })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const inconsistent = createCommissionReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(payload({ summary: { ...totals, net_points: "1" } }))));
    await expect(inconsistent.report(brand, { from, to, group_by: "day" })).resolves.toMatchObject({ summary: { net_points: "1" } });
    const badCounter = createCommissionReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(payload({ summary: { ...totals, correction_entry_count: "-1" } }))));
    await expect(badCounter.report(brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const badCountSum = createCommissionReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(payload({ summary: { ...totals, entry_count: "2" } }))));
    await expect(badCountSum.report(brand, { from, to, group_by: "day" })).resolves.toMatchObject({ summary: { entry_count: "2" } });
    await expect(api.report(brand, { from, to, group_by: "day", game_id: brand } as never)).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(api.report(brand, { from, to: "2026-04-05T00:00:00Z", group_by: "day" })).rejects.toMatchObject({ code: "INVALID_INPUT" });
  });
  it("keeps full-scope summary on an empty page", async () => {
    const data = payload({ query: { from, to, group_by: "day", limit: 20, offset: 40, agent_id: null, member_id: null, cycle_id: null }, items: [] });
    await expect(createCommissionReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(data))).report(brand, { from, to, group_by: "day", offset: 40 })).resolves.toMatchObject({ summary: totals, items: [], total_groups: "1" });
  });
  it("rejects nonempty pages extending beyond total_groups", async () => {
    const data = payload({ query: { from, to, group_by: "day", limit: 20, offset: 20, agent_id: null, member_id: null, cycle_id: null } });
    await expect(createCommissionReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(data))).report(brand, { from, to, group_by: "day", offset: 20 })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });
  it("accepts a canonical day key in years 0001 through 0099", async () => {
    const earlyFrom = "0001-01-01T00:00:00Z", earlyTo = "0001-01-02T00:00:00Z";
    const data = payload({ query: { from: earlyFrom, to: earlyTo, group_by: "day", limit: 20, offset: 0, agent_id: null, member_id: null, cycle_id: null }, items: [{ key: "0001-01-01", label: "0001-01-01", totals } ] });
    await expect(createCommissionReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(data))).report(brand, { from: earlyFrom, to: earlyTo, group_by: "day" })).resolves.toMatchObject({ items: [{ key: "0001-01-01" }] });
  });
  it("lowercases brand and filter UUIDs on CSV requests", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: false, error: { code: "SERVICE_UNAVAILABLE", message: "unavailable" } }), { status: 503 }));
    await expect(createCommissionReportApi(fetcher).exportCsv(brand.toUpperCase(), { from, to, group_by: "day", agent_id: agent.toUpperCase(), member_id: actor.toUpperCase(), cycle_id: cycle.toUpperCase() })).rejects.toMatchObject({ status: 503 });
    const [url, init] = fetcher.mock.calls[0]!;
    const parsed = new URL(String(url), "http://local");
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    expect(parsed.searchParams.get("agent_id")).toBe(agent);
    expect(parsed.searchParams.get("member_id")).toBe(actor);
    expect(parsed.searchParams.get("cycle_id")).toBe(cycle);
  });
});
