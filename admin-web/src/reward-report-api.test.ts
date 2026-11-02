import { describe, expect, it, vi } from "vitest";
import type { AdminAccount } from "./admin-api";
import { createRewardReportApi, rewardReportPermissions, type RewardReportKind } from "./reward-report-api";

const brand = "11111111-1111-4111-8111-111111111111", actor = "22222222-2222-4222-8222-222222222222", member = "33333333-3333-4333-8333-333333333333", order = "44444444-4444-4444-8444-444444444444";
const from = "2026-10-01T00:00:00Z", to = "2026-10-08T00:00:00Z", snapshot = "2026-10-08T01:02:03.123456789+08:00";
const huge = "9223372036854775808123456789012345678901234567890";
const posting = { entry_count: "2", grant_entry_count: "1", grant_points: huge, reversal_entry_count: "1", reversal_points: `${BigInt(huge) + 9n}`, net_points: "-9" };
const orders = { order_count: "2", original_points: huge, granted_count: "1", granted_points: `${BigInt(huge) - 25n}`, revoked_count: "1", revoked_points: "25" };
const query = (group_by: string = "day", extras: Record<string, unknown> = {}) => ({ from, to, group_by, limit: 20, offset: 0, member_id: null, order_id: null, ...extras });
function payload(kind: RewardReportKind, overrides: Record<string, unknown> = {}) {
  const total = kind === "rewards" ? posting : orders;
  return { success: true, associations: { ["brand:" + brand]: { name: "Aurora" } }, request_id: "reward-report-request_1", data: { brand_id: brand, snapshot_at: "2026-10-08T01:02:03.123456780Z", timezone: "Asia/Singapore", query: query("day"), summary: total, items: [{ key: "2026-10-01", label: "2026-10-01", totals: total }], total_groups: "1", ...overrides } };
}
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status, headers: { "content-type": "application/json" } });

describe("reward report permissions", () => {
  const base: AdminAccount = { id: actor, super_admin: false, brand_ids: [brand], permissions: ["report_reward.view.brand", "report_reward.export.brand", "reward.view.brand"], permissions_by_brand: { [brand]: ["reward.view.brand"] } };
  it("requires independent explicit report rights and never inherits reward or unrelated permissions", () => {
    expect(rewardReportPermissions(base, brand)).toEqual({ view: false, export: false });
    expect(rewardReportPermissions({ ...base, platform_permissions: ["report_reward.export.platform"] }, brand)).toEqual({ view: false, export: false });
    expect(rewardReportPermissions({ ...base, permissions_by_brand: { [brand]: ["report_reward.view.brand"] } }, brand)).toEqual({ view: true, export: false });
    expect(rewardReportPermissions({ ...base, permissions_by_brand: { [brand]: ["report_reward.view.brand", "report_reward.export.brand"] } }, brand)).toEqual({ view: true, export: true });
    expect(rewardReportPermissions({ ...base, brand_ids: [], permissions_by_brand: {}, platform_permissions: ["report_reward.view.platform", "report_reward.export.platform"] }, brand)).toEqual({ view: false, export: false });
    expect(rewardReportPermissions({ ...base, brand_ids: [], permissions_by_brand: { [brand]: ["report_reward.view.brand", "report_reward.export.brand"] } }, brand)).toEqual({ view: false, export: false });
    expect(rewardReportPermissions({ ...base, super_admin: true, permissions_by_brand: { [brand]: ["report_reward.view.brand", "report_reward.export.brand"] } }, brand)).toEqual({ view: false, export: false });
  });
});

describe("reward report API", () => {
  it("calls distinct live endpoints, preserves exact integers/nanoseconds, and echoes omitted filters as null", async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async (input) => {
      const url = new URL(String(input), "http://local"), kind = url.pathname.endsWith("reward-orders") ? "reward_orders" : "rewards";
      const group = url.searchParams.get("group_by") ?? "day", key = group === "state" ? "granted" : group === "day" ? "2026-10-01" : member;
      const summary = kind === "rewards" ? posting : orders;
      const items = group === "state" ? [
        { key: "granted", label: "granted", totals: { ...orders, order_count: "1", original_points: orders.granted_points, granted_count: "1", revoked_count: "0", revoked_points: "0" } },
        { key: "revoked", label: "revoked", totals: { ...orders, order_count: "1", original_points: "25", granted_count: "0", granted_points: "0", revoked_count: "1", revoked_points: "25" } },
      ] : [{ key, label: key, totals: summary }];
      return json(payload(kind, { query: { from: url.searchParams.get("from"), to: url.searchParams.get("to"), group_by: group, limit: Number(url.searchParams.get("limit")), offset: Number(url.searchParams.get("offset")), member_id: url.searchParams.get("member_id"), order_id: url.searchParams.get("order_id") }, summary, items, total_groups: String(items.length) }));
    });
    const api = createRewardReportApi(fetcher);
    const postingReport = await api.read("rewards", brand.toUpperCase(), { from: "2026-10-01T08:00:00.120000000+08:00", to, group_by: "day" });
    expect(postingReport.summary).toMatchObject({ grant_points: huge });
    expect(postingReport.query.from).toBe("2026-10-01T00:00:00.12Z");
    expect(postingReport.snapshot_at).toBe("2026-10-08T01:02:03.123456780Z");
    const postedUrl = new URL(String(fetcher.mock.calls[0]![0]), "http://local");
    expect(postedUrl.pathname).toBe("/api/v1/admin/reports/rewards");
    expect(postedUrl.searchParams.get("from")).toBe("2026-10-01T00:00:00.12Z");
    expect(new Headers(fetcher.mock.calls[0]![1]?.headers).get("X-Brand-ID")).toBe(brand);
    const filteredFetch = vi.fn<typeof fetch>().mockImplementation(async () => {
      return json(payload("rewards", { query: { from, to, group_by: "order", limit: 20, offset: 0, member_id: member, order_id: order }, items: [{ key: order, label: order, totals: posting }] }));
    });
    await createRewardReportApi(filteredFetch).read("rewards", brand, { from, to, group_by: "order", member_id: member.toUpperCase(), order_id: order.toUpperCase() });
    const filteredURL = new URL(String(filteredFetch.mock.calls[0]![0]), "http://local");
    expect(filteredURL.searchParams.get("member_id")).toBe(member);
    expect(filteredURL.searchParams.get("order_id")).toBe(order);
    await api.read("reward_orders", brand, { from, to, group_by: "state" });
    expect(new URL(String(fetcher.mock.calls[1]![0]), "http://local").pathname).toBe("/api/v1/admin/reports/reward-orders");
    expect(fetcher.mock.calls[1]![1]?.signal).toBeUndefined();
    expect(posting.net_points).toBe("-9");
    expect(snapshot).toContain(".123456789+");
  });
  it("validates strict envelope, exact query, numeric fields, ordering and page boundaries", async () => {
    const apiFor = (body: unknown) => createRewardReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(body)));
    await expect(apiFor(payload("rewards")).read("rewards", brand, { from, to, group_by: "day" })).resolves.toMatchObject({ total_groups: "1" });
    await expect(apiFor({ ...payload("rewards"), request_id: undefined }).read("rewards", brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(apiFor(payload("rewards", { summary: { ...posting, grant_points: "01" } })).read("rewards", brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(apiFor(payload("rewards", { summary: { ...posting, entry_count: "3" } })).read("rewards", brand, { from, to, group_by: "day" })).resolves.toBeDefined();
    await expect(apiFor(payload("reward_orders", { summary: { ...orders, revoked_count: "0" } })).read("reward_orders", brand, { from, to, group_by: "day" })).resolves.toBeDefined();
    await expect(apiFor(payload("rewards", { query: query("day", { member_id: member }) })).read("rewards", brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(apiFor(payload("rewards", { items: [{ key: order, label: order, totals: posting }] })).read("rewards", brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(apiFor(payload("rewards", { items: [{ key: "2026-10-01", label: "2026-10-01", totals: posting }, { key: "2026-10-01", label: "2026-10-01", totals: posting }], total_groups: "2" })).read("rewards", brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(apiFor(payload("rewards", { items: [{ key: "2026-10-02", label: "2026-10-02", totals: posting }, { key: "2026-10-01", label: "2026-10-01", totals: posting }], total_groups: "2" })).read("rewards", brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(apiFor(payload("rewards", { items: [{ key: "2026-10-01", label: "2026-10-01", totals: { ...posting, grant_points: `${BigInt(huge) + 1n}`, net_points: "-8" } }] })).read("rewards", brand, { from, to, group_by: "day" })).resolves.toBeDefined();
    await expect(apiFor(payload("rewards", { extra: true })).read("rewards", brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(apiFor(payload("rewards", { query: { ...query(), extra: null } })).read("rewards", brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });
  it("requires exact remaining page length, validates kind at runtime, and normalizes fractional trailing zeroes", async () => {
    const apiFor = (body: unknown) => createRewardReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(body)));
    const pageQuery = query("day", { limit: 1, offset: 1 });
    const oneRemaining = payload("rewards", { query: pageQuery, total_groups: "2", items: [{ key: "2026-10-02", label: "2026-10-02", totals: posting }] });
    await expect(apiFor(oneRemaining).read("rewards", brand, { from, to, group_by: "day", limit: 1, offset: 1 })).resolves.toMatchObject({ items: [{ key: "2026-10-02" }] });
    await expect(apiFor(payload("rewards", { query: pageQuery, total_groups: "2", items: [] })).read("rewards", brand, { from, to, group_by: "day", limit: 1, offset: 1 })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const emptyPastEnd = payload("rewards", { query: query("day", { limit: 20, offset: 1 }), total_groups: "1", items: [] });
    await expect(apiFor(emptyPastEnd).read("rewards", brand, { from, to, group_by: "day", offset: 1 })).resolves.toMatchObject({ summary: posting, total_groups: "1", items: [] });
    const fetcher = vi.fn<typeof fetch>();
    await expect(createRewardReportApi(fetcher).read("other" as never, brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(createRewardReportApi(fetcher).export("other" as never, brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_INPUT" });
    expect(fetcher).not.toHaveBeenCalled();
    const normalized = await createRewardReportApi(vi.fn<typeof fetch>().mockImplementation(async (input) => {
      const url = new URL(String(input), "http://local");
      return json(payload("rewards", { query: { from: url.searchParams.get("from"), to: url.searchParams.get("to"), group_by: "day", limit: 20, offset: 0, member_id: null, order_id: null } }));
    })).read("rewards", brand, { from: "2026-10-01T00:00:00.120000000Z", to: "2026-10-08T00:00:00.000000000Z", group_by: "day" });
    expect(normalized.query.from).toBe("2026-10-01T00:00:00.12Z");
    expect(normalized.query.to).toBe(to);
  });
  it("preserves server-calculated totals and validates state keys and numeric formats", async () => {
    const apiFor = (body: unknown) => createRewardReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(body)));
    const queryInput = { from, to, group_by: "day" as const };
    const serverTotals = { ...posting, net_points: "-123456789012345678901234567890", entry_count: "0" };
    await expect(apiFor(payload("rewards", { summary: serverTotals })).read("rewards", brand, queryInput)).resolves.toMatchObject({ summary: serverTotals });
    const stateData = payload("reward_orders", { query: query("state"), items: [{ key: "revoked", label: "revoked", totals: orders }] });
    await expect(apiFor(stateData).read("reward_orders", brand, { from, to, group_by: "state" })).resolves.toMatchObject({ items: [{ key: "revoked" }] });
    await expect(apiFor(payload("reward_orders", { query: query("state"), items: [{ key: "unknown", label: "unknown", totals: orders }] })).read("reward_orders", brand, { from, to, group_by: "state" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(apiFor(payload("rewards", { summary: { ...posting, entry_count: "-1" } })).read("rewards", brand, queryInput)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const mismatchedQuery = payload("rewards", { query: query("member", { member_id: member }), items: [{ key: actor, label: actor, totals: posting }] });
    await expect(apiFor(mismatchedQuery).read("rewards", brand, { from, to, group_by: "member", member_id: member })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    for (const badQuery of [{ from: null }, { to: {} }, { group_by: null }]) await expect(createRewardReportApi(vi.fn()).read("rewards", brand, { ...queryInput, ...badQuery } as never)).rejects.toMatchObject({ code: "INVALID_INPUT" });
  });
  it("forwards AbortSignal and reports generic 401 without exposing server details", async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () => json({ success: false, error: { code: "AUTH_UNAUTHENTICATED", message: "expired" } }, 401));
    const controller = new AbortController();
    await expect(createRewardReportApi(fetcher).read("rewards", brand, { from, to, group_by: "day" }, controller.signal)).rejects.toMatchObject({ status: 401, code: "AUTH_UNAUTHENTICATED", message: "expired" });
    await expect(createRewardReportApi(fetcher).export("rewards", brand, { from, to, group_by: "day" }, controller.signal)).rejects.toMatchObject({ status: 401, code: "AUTH_UNAUTHENTICATED", message: "expired" });
    expect(fetcher.mock.calls[0]![1]?.signal).toBe(controller.signal);
    expect(fetcher.mock.calls[1]![1]?.signal).toBe(controller.signal);
  });
});
