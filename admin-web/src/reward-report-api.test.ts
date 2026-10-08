import { describe, expect, it, vi } from "vitest";
import type { AdminAccount } from "./admin-api";
import { createRewardReportApi, rewardReportPermissions, type RewardReportKind, type RewardReportQuery } from "./reward-report-api";

const brand = "11111111-1111-4111-8111-111111111111", actor = "22222222-2222-4222-8222-222222222222", member = "33333333-3333-4333-8333-333333333333", order = "44444444-4444-4444-8444-444444444444";
const from = "2026-10-01T00:00:00Z", to = "2026-10-08T00:00:00Z", snapshot = "2026-10-08T01:02:03.123456789+08:00";
const huge = "9223372036854775808123456789012345678901234567890";
const posting = { entry_count: "2", grant_entry_count: "1", grant_points: huge, reversal_entry_count: "1", reversal_points: `${BigInt(huge) + 9n}`, net_points: "-9" };
const orders = { order_count: "3", original_points: huge, granted_count: "1", granted_points: `${BigInt(huge) - 50n}`, pending_count: "1", pending_points: "25", revoked_count: "1", revoked_points: "25" };
const query = (group_by: string = "day", extras: Record<string, unknown> = {}) => ({ from, to, group_by, limit: 20, offset: 0, member_id: null, order_id: null, ...extras });
function payload(kind: RewardReportKind, overrides: Record<string, unknown> = {}) {
  const total = kind === "rewards" ? posting : orders;
  return { success: true, request_id: "reward-report-request_1", data: { brand_id: brand, snapshot_at: "2026-10-08T01:02:03.123456780Z", timezone: "Asia/Singapore", query: query("day"), summary: total, items: [{ key: "2026-10-01", label: "2026-10-01", totals: total }], total_groups: "1", ...overrides } };
}
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status, headers: { "content-type": "application/json" } });

describe("reward report permissions", () => {
  const base: AdminAccount = { id: actor, super_admin: true, brand_ids: [brand], permissions: ["report_reward.view.brand", "report_reward.export.brand", "reward.view.brand"], permissions_by_brand: { [brand]: ["reward.view.brand"] } };
  it("requires independent explicit report rights and never inherits reward or unrelated permissions", () => {
    expect(rewardReportPermissions(base, brand)).toEqual({ view: false, export: false });
    expect(rewardReportPermissions({ ...base, platform_permissions: ["report_reward.export.platform"] }, brand)).toEqual({ view: false, export: false });
    expect(rewardReportPermissions({ ...base, permissions_by_brand: { [brand]: ["report_reward.view.brand"] } }, brand)).toEqual({ view: true, export: false });
    expect(rewardReportPermissions({ ...base, permissions_by_brand: { [brand]: ["report_reward.view.brand", "report_reward.export.brand"] } }, brand)).toEqual({ view: true, export: true });
    expect(rewardReportPermissions({ ...base, brand_ids: [], permissions_by_brand: {}, platform_permissions: ["report_reward.view.platform", "report_reward.export.platform"] }, brand)).toEqual({ view: true, export: true });
    expect(rewardReportPermissions({ ...base, brand_ids: [], permissions_by_brand: { [brand]: ["report_reward.view.brand", "report_reward.export.brand"] } }, brand)).toEqual({ view: false, export: false });
  });
});

describe("reward report API", () => {
  it("calls distinct live endpoints, preserves exact integers/nanoseconds, and echoes omitted filters as null", async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async (input) => {
      const url = new URL(String(input), "http://local"), kind = url.pathname.endsWith("reward-orders") ? "reward_orders" : "rewards";
      const group = url.searchParams.get("group_by") ?? "day", key = group === "state" ? "granted" : group === "day" ? "2026-10-01" : member;
      const summary = kind === "rewards" ? posting : orders;
      const items = group === "state" ? [
        { key: "granted", label: "granted", totals: { ...orders, order_count: "1", original_points: orders.granted_points, granted_count: "1", pending_count: "0", pending_points: "0", revoked_count: "0", revoked_points: "0" } },
        { key: "revocation_pending", label: "revocation_pending", totals: { ...orders, order_count: "1", original_points: "25", granted_count: "0", granted_points: "0", pending_count: "1", pending_points: "25", revoked_count: "0", revoked_points: "0" } },
        { key: "revoked", label: "revoked", totals: { ...orders, order_count: "1", original_points: "25", granted_count: "0", granted_points: "0", pending_count: "0", pending_points: "0", revoked_count: "1", revoked_points: "25" } },
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
  it("validates strict envelope, exact query, totals, subsets, ordering and page boundaries", async () => {
    const apiFor = (body: unknown) => createRewardReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(body)));
    await expect(apiFor(payload("rewards")).read("rewards", brand, { from, to, group_by: "day" })).resolves.toMatchObject({ total_groups: "1" });
    await expect(apiFor({ ...payload("rewards"), request_id: undefined }).read("rewards", brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(apiFor(payload("rewards", { summary: { ...posting, grant_points: "01" } })).read("rewards", brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(apiFor(payload("rewards", { summary: { ...posting, entry_count: "3" } })).read("rewards", brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(apiFor(payload("reward_orders", { summary: { ...orders, pending_count: "0" } })).read("reward_orders", brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(apiFor(payload("rewards", { query: query("day", { member_id: member }) })).read("rewards", brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(apiFor(payload("rewards", { items: [{ key: order, label: order, totals: posting }] })).read("rewards", brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(apiFor(payload("rewards", { items: [{ key: "2026-10-01", label: "2026-10-01", totals: posting }, { key: "2026-10-01", label: "2026-10-01", totals: posting }], total_groups: "2" })).read("rewards", brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(apiFor(payload("rewards", { items: [{ key: "2026-10-02", label: "2026-10-02", totals: posting }, { key: "2026-10-01", label: "2026-10-01", totals: posting }], total_groups: "2" })).read("rewards", brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(apiFor(payload("rewards", { items: [{ key: "2026-10-01", label: "2026-10-01", totals: { ...posting, grant_points: `${BigInt(huge) + 1n}`, net_points: "-8" } }] })).read("rewards", brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
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
  it("validates arbitrary-precision signed postings, canonical states, key/window membership, and page limits", async () => {
    const apiFor = (data: unknown) => createRewardReportApi(vi.fn<typeof fetch>().mockResolvedValue(json({ success: true, request_id: "r1", data })));
    const pendingOnly = { order_count: "1", original_points: "25", granted_count: "0", granted_points: "0", pending_count: "1", pending_points: "25", revoked_count: "0", revoked_points: "0" };
    const stateData = payload("reward_orders", { query: query("state"), summary: pendingOnly, items: [{ key: "revocation_pending", label: "revocation_pending", totals: pendingOnly }] });
    await expect(apiFor(stateData.data).read("reward_orders", brand, { from, to, group_by: "state" })).resolves.toMatchObject({ items: [{ key: "revocation_pending" }] });
    const outside = payload("rewards", { items: [{ key: "2026-10-09", label: "2026-10-09", totals: posting }] });
    await expect(apiFor(outside.data).read("rewards", brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(createRewardReportApi(vi.fn()).read("rewards", brand, { from, to, group_by: "state" as never })).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(createRewardReportApi(vi.fn()).read("rewards", brand, { from, to, group_by: "day", unknown: true } as never)).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(createRewardReportApi(vi.fn()).read("rewards", brand, { from, to: "2026-10-01T00:00:00.000000000Z", group_by: "day" })).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(createRewardReportApi(vi.fn()).read("reward_orders", brand, { from, to, group_by: "day", order_id: "bad" })).rejects.toMatchObject({ code: "INVALID_INPUT" });
    const legacyUUID = "deadbeef-0000-0000-0000-000000000001";
    const legacyPayload = payload("rewards", { query: query("order", { order_id: legacyUUID }), items: [{ key: legacyUUID, label: legacyUUID, totals: posting }] });
    await expect(apiFor(legacyPayload.data).read("rewards", brand, { from, to, group_by: "order", order_id: legacyUUID })).resolves.toMatchObject({ items: [{ key: legacyUUID }] });
    const zeroPosting = { entry_count: "0", grant_entry_count: "0", grant_points: "0", reversal_entry_count: "0", reversal_points: "0", net_points: "0" };
    await expect(apiFor(payload("rewards", { summary: posting, items: [{ key: "2026-10-01", label: "2026-10-01", totals: zeroPosting }] })).read("rewards", brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(apiFor(payload("rewards", { summary: { ...posting, entry_count: "1" }, items: [{ key: "2026-10-01", label: "2026-10-01", totals: posting }] })).read("rewards", brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const twoGroups = [{ key: "2026-10-01", label: "2026-10-01", totals: posting }, { key: "2026-10-02", label: "2026-10-02", totals: posting }];
    await expect(apiFor(payload("rewards", { items: twoGroups, total_groups: "2" })).read("rewards", brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    for (const badQuery of [{ from: null }, { to: {} }, { group_by: null }]) {
      await expect(createRewardReportApi(vi.fn()).read("rewards", brand, { from, to, group_by: "day", ...badQuery } as never)).rejects.toMatchObject({ code: "INVALID_INPUT" });
    }
    await expect(createRewardReportApi(vi.fn()).read("rewards", brand, { from: "0001-01-01T00:00:00+01:00", to: "0001-01-02T00:00:00Z", group_by: "day" })).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(createRewardReportApi(vi.fn()).read("rewards", brand, { from: "9999-12-31T23:59:59.999999999-00:01", to: "9999-12-31T23:59:59Z", group_by: "day" })).rejects.toMatchObject({ code: "INVALID_INPUT" });
  });
  it("forwards AbortSignal and reports generic 401 without exposing server details", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(json({ error: { code: "AUTH_UNAUTHENTICATED", message: "expired" } }, 401));
    const controller = new AbortController();
    await expect(createRewardReportApi(fetcher).read("rewards", brand, { from, to, group_by: "day" }, controller.signal)).rejects.toMatchObject({ status: 401, message: "Request failed (401)" });
    await expect(createRewardReportApi(fetcher).export("rewards", brand, { from, to, group_by: "day" }, controller.signal)).rejects.toMatchObject({ status: 401, message: "Request failed (401)" });
    expect(fetcher.mock.calls[0]![1]?.signal).toBe(controller.signal);
    expect(fetcher.mock.calls[1]![1]?.signal).toBe(controller.signal);
  });
  it("validates a full CSV digest, metadata, complete group coverage and totals, and omits page parameters", async () => {
    const kind: RewardReportKind = "rewards", queryInput: RewardReportQuery = { from, to, group_by: "day" };
    const cols = ["record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "member_id", "order_id", "key", "label", "entry_count", "grant_entry_count", "grant_points", "reversal_entry_count", "reversal_points", "net_points"];
    const negative = "'-9";
    const csv = `\uFEFF${cols.join(",")}\nsummary,${brand},${snapshot},Asia/Singapore,${from},${to},day,,,,,${posting.entry_count},${posting.grant_entry_count},${posting.grant_points},${posting.reversal_entry_count},${posting.reversal_points},${negative}\ngroup,${brand},${snapshot},Asia/Singapore,${from},${to},day,,,2026-10-01,2026-10-01,${posting.entry_count},${posting.grant_entry_count},${posting.grant_points},${posting.reversal_entry_count},${posting.reversal_points},${negative}\n`;
    const bytes = new TextEncoder().encode(csv), sha = [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))].map((x) => x.toString(16).padStart(2, "0")).join("");
    const filename = `lottery-${kind}-${brand}-20261007T170203Z.csv`;
    const headers = new Headers({ "Content-Length": String(bytes.length), "Content-Type": "text/csv; charset=utf-8", "Content-Disposition": `attachment; filename="${filename}"`, "Cache-Control": "no-store", "X-Report-SHA256": sha, "X-Report-Byte-Count": String(bytes.length), "X-Report-Format-Version": "1", "X-Report-Audit-ID": actor, "X-Report-Brand-ID": brand, "X-Report-Kind": kind, "X-Report-Group-Count": "1", "X-Report-Snapshot-At": snapshot, "X-Report-Timezone": "Asia/Singapore", "X-Report-From": from, "X-Report-To": to, "X-Report-Group-By": "day", "X-Report-Member-ID": "", "X-Report-Order-ID": "" });
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(bytes, { status: 200, headers }));
    const file = await createRewardReportApi(fetcher).export(kind, brand, queryInput);
    expect(file).toMatchObject({ filename, groupCount: "1", auditLogId: actor, snapshotAt: snapshot });
    const parsed = new URL(String(fetcher.mock.calls[0]![0]), "http://local");
    expect(parsed.pathname).toBe("/api/v1/admin/reports/rewards.csv");
    expect(parsed.searchParams.has("limit")).toBe(false); expect(parsed.searchParams.has("offset")).toBe(false);
    expect(() => file.bytes).not.toThrow();
    for (const [name, value] of [["X-Report-From", "2026-10-02T00:00:00Z"], ["X-Report-Group-By", "member"], ["X-Report-Member-ID", member], ["X-Report-Brand-ID", actor], ["X-Report-Format-Version", "2"], ["X-Report-Byte-Count", "1"]]) {
      const changed = new Headers(headers); changed.set(name, value);
      const changedFetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(bytes, { status: 200, headers: changed }));
      await expect(createRewardReportApi(changedFetcher).export(kind, brand, queryInput)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    }
    const partialText = csv.replace("group,", ""); const partialBytes = new TextEncoder().encode(partialText);
    const partialHeaders = new Headers(headers); partialHeaders.set("Content-Length", String(partialBytes.length));
    partialHeaders.set("X-Report-SHA256", [...new Uint8Array(await crypto.subtle.digest("SHA-256", partialBytes))].map((x) => x.toString(16).padStart(2, "0")).join("")); partialHeaders.set("X-Report-Byte-Count", String(partialBytes.length)); partialHeaders.set("Content-Length", String(partialBytes.length));
    const badFetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(partialBytes, { status: 200, headers: partialHeaders }));
    await expect(createRewardReportApi(badFetcher).export(kind, brand, queryInput)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const malformedBytes = new TextEncoder().encode(csv.replace("record_type", '"record_type'));
    const malformedHeaders = new Headers(headers); malformedHeaders.set("Content-Length", String(malformedBytes.length)); malformedHeaders.set("X-Report-Byte-Count", String(malformedBytes.length));
    malformedHeaders.set("X-Report-SHA256", [...new Uint8Array(await crypto.subtle.digest("SHA-256", malformedBytes))].map((v) => v.toString(16).padStart(2, "0")).join(""));
    const malformed = vi.fn<typeof fetch>().mockResolvedValue(new Response(malformedBytes, { status: 200, headers: malformedHeaders }));
    await expect(createRewardReportApi(malformed).export(kind, brand, queryInput)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(createRewardReportApi(fetcher).export(kind, brand, { ...queryInput, offset: 20 })).rejects.toMatchObject({ code: "INVALID_INPUT" });
  });
  it("accepts exactly 10,000 real nonempty CSV groups and rejects empty groups and mismatched digest", async () => {
    const kind: RewardReportKind = "reward_orders", groupTotal = 10_000;
    const summaryTotals = { order_count: String(groupTotal), original_points: String(groupTotal), granted_count: String(groupTotal), granted_points: String(groupTotal), pending_count: "0", pending_points: "0", revoked_count: "0", revoked_points: "0" };
    const groupTotals = { order_count: "1", original_points: "1", granted_count: "1", granted_points: "1", pending_count: "0", pending_points: "0", revoked_count: "0", revoked_points: "0" };
    const cols = ["record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "member_id", "order_id", "key", "label", "order_count", "original_points", "granted_count", "granted_points", "pending_count", "pending_points", "revoked_count", "revoked_points"];
    const summaryFields = Object.values(summaryTotals).join(","), groupFields = Object.values(groupTotals).join(","), memberIDs = Array.from({ length: groupTotal }, (_, i) => `00000000-0000-0000-0000-${String(i + 1).padStart(12, "0")}`);
    const csv = `\uFEFF${cols.join(",")}\nsummary,${brand},${snapshot},Asia/Singapore,${from},${to},member,,,,,${summaryFields}\n${memberIDs.map((id) => `group,${brand},${snapshot},Asia/Singapore,${from},${to},member,,,${id},${id},${groupFields}`).join("\n")}\n`;
    const bytes = new TextEncoder().encode(csv), digest = [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))].map((v) => v.toString(16).padStart(2, "0")).join("");
    const filename = `lottery-${kind}-${brand}-20261007T170203Z.csv`;
    const headers = new Headers({ "Content-Length": String(bytes.length), "Content-Type": "text/csv; charset=utf-8", "Content-Disposition": `attachment; filename="${filename}"`, "Cache-Control": "no-store", "X-Report-SHA256": digest, "X-Report-Byte-Count": String(bytes.length), "X-Report-Format-Version": "1", "X-Report-Audit-ID": actor, "X-Report-Brand-ID": brand, "X-Report-Kind": kind, "X-Report-Group-Count": String(groupTotal), "X-Report-Snapshot-At": snapshot, "X-Report-Timezone": "Asia/Singapore", "X-Report-From": from, "X-Report-To": to, "X-Report-Group-By": "member", "X-Report-Member-ID": "", "X-Report-Order-ID": "" });
    const good = vi.fn<typeof fetch>().mockResolvedValue(new Response(bytes, { status: 200, headers }));
    await expect(createRewardReportApi(good).export(kind, brand, { from, to, group_by: "member" })).resolves.toMatchObject({ groupCount: "10000", filename });
    const zeroGroupCsv = csv.replace(/,1,1,1,1,0,0,0,0\n$/, ",0,0,0,0,0,0,0,0\n");
    const zeroGroupBytes = new TextEncoder().encode(zeroGroupCsv), zeroGroupHeaders = new Headers(headers);
    zeroGroupHeaders.set("Content-Length", String(zeroGroupBytes.length)); zeroGroupHeaders.set("X-Report-Byte-Count", String(zeroGroupBytes.length));
    zeroGroupHeaders.set("X-Report-SHA256", [...new Uint8Array(await crypto.subtle.digest("SHA-256", zeroGroupBytes))].map((v) => v.toString(16).padStart(2, "0")).join(""));
    const zeroGroupFetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(zeroGroupBytes, { status: 200, headers: zeroGroupHeaders }));
    await expect(createRewardReportApi(zeroGroupFetcher).export(kind, brand, { from, to, group_by: "member" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const wrongHashHeaders = new Headers(headers); wrongHashHeaders.set("X-Report-SHA256", "0".repeat(64));
    const wrongHash = vi.fn<typeof fetch>().mockResolvedValue(new Response(bytes, { status: 200, headers: wrongHashHeaders }));
    await expect(createRewardReportApi(wrongHash).export(kind, brand, { from, to, group_by: "member" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });
});
