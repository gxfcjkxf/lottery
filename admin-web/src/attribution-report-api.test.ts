import { describe, expect, it, vi } from "vitest";
import type { AdminAccount } from "./admin-api";
import { attributionReportPermissions, createAttributionReportApi, type AttributionTotals } from "./attribution-report-api";

const brand = "11111111-1111-4111-8111-111111111111", actor = "22222222-2222-4222-8222-222222222222", agent = "33333333-3333-4333-8333-333333333333";
const from = "2026-01-01T00:00:00Z", to = "2026-01-02T00:00:00Z", huge = "922337203685477580812345678901234567890";
const totals: AttributionTotals = { order_count: "1", stake_points: huge, placed_count: "0", won_count: "0", lost_count: "1", abnormal_count: "0", cancelled_count: "0", refund_points: "0", settled_stake_points: huge, unfinalized_stake_points: "0", abnormal_stake_points: "0", current_prize_points: "0", correction_open_count: "0", final_lost_stake_points: huge };
const query = { from, to, group_by: "agent", limit: 20, offset: 0, game_id: null, member_id: null, agent_id: null, agent_scope: "direct", join_method: null };
const payload = (overrides: Record<string, unknown> = {}) => ({ success: true, request_id: "attribution-read-1", data: { brand_id: brand, snapshot_at: "2026-01-02T01:00:00Z", timezone: "Asia/Singapore", query, summary: totals, items: [{ key: agent, label: "Agent Alpha", totals }], total_groups: "1", ...overrides } });
const json = (value: unknown) => new Response(JSON.stringify(value), { status: 200, headers: { "content-type": "application/json" } });

describe("attribution report permissions", () => {
  const account: AdminAccount = { id: actor, super_admin: false, brand_ids: [brand], permissions: [], permissions_by_brand: { [brand]: ["report_attribution.view.brand"] } };
  it("requires separate view and export grants even for super admins", () => {
    expect(attributionReportPermissions(account, brand)).toEqual({ view: true, export: false });
    expect(attributionReportPermissions({ ...account, permissions_by_brand: { [brand]: ["report_attribution.export.brand"] } }, brand)).toEqual({ view: false, export: false });
    expect(attributionReportPermissions({ ...account, brand_ids: [], permissions_by_brand: {}, platform_permissions: ["report_attribution.view.platform", "report_attribution.export.platform"] }, brand)).toEqual({ view: false, export: false });
    expect(attributionReportPermissions({ ...account, brand_ids: [], permissions_by_brand: { [brand]: ["report_attribution.view.brand", "report_attribution.export.brand"] } }, brand)).toEqual({ view: false, export: false });
    expect(attributionReportPermissions({ ...account, super_admin: true, permissions_by_brand: { [brand]: ["report_attribution.view.brand", "report_attribution.export.brand"] } }, brand)).toEqual({ view: false, export: false });
  });
  it("requires a per-brand map and ignores flat grants", () => {
    const flat: AdminAccount = { id: actor, super_admin: false, brand_ids: [brand], permissions: ["report_attribution.view.brand", "report_attribution.export.brand"] };
    expect(attributionReportPermissions(flat, brand)).toEqual({ view: false, export: false });
    expect(attributionReportPermissions({ ...flat, permissions_by_brand: { "99999999-9999-4999-8999-999999999999": flat.permissions } }, brand)).toEqual({ view: false, export: false });
    expect(attributionReportPermissions({ ...flat, brand_ids: [], permissions: ["report_attribution.view.platform", "report_attribution.export.platform"] }, brand)).toEqual({ view: false, export: false });
  });
});

describe("attribution report API", () => {
  it("sends the closed query, omits nullable filters, and preserves arbitrary integer precision", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(json(payload()));
    const report = await createAttributionReportApi(fetcher).report(brand.toUpperCase(), { from, to, group_by: "agent" });
    expect(report.summary.stake_points).toBe(huge);
    const [url, init] = fetcher.mock.calls[0]!; const parsed = new URL(String(url), "http://local");
    expect(parsed.pathname).toBe("/api/v1/admin/reports/attribution");
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    expect(parsed.searchParams.get("limit")).toBe("20"); expect(parsed.searchParams.get("offset")).toBe("0");
    expect(parsed.searchParams.get("agent_scope")).toBe("direct"); expect(parsed.searchParams.has("agent_id")).toBe(false);
  });
  it("validates API inputs and rejects response scope/arithmetic/count mismatches", async () => {
    const api = createAttributionReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(payload())));
    await expect(api.report(brand, { from, to, group_by: "agent", agent_scope: "downline" })).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(api.report(brand, { from, to, group_by: "day", mystery: true } as never)).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(api.report(brand, { from: "2026-02-30T00:00:00Z", to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(api.report(brand, { from, to: "2026-04-05T00:00:00Z", group_by: "day" })).rejects.toMatchObject({ code: "INVALID_INPUT" });
    const changed = createAttributionReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(payload({ query: { ...query, agent_scope: "downline" } }))));
    await expect(changed.report(brand, { from, to, group_by: "agent" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const badTotals = createAttributionReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(payload({ summary: { ...totals, final_lost_stake_points: "-1" } }))));
    await expect(badTotals.report(brand, { from, to, group_by: "agent" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const badSum = createAttributionReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(payload({ summary: { ...totals, stake_points: "2" } }))));
    await expect(badSum.report(brand, { from, to, group_by: "agent" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });
  it("keeps a full summary with an empty out-of-range page", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(json(payload({ query: { ...query, offset: 40 }, items: [] })));
    await expect(createAttributionReportApi(fetcher).report(brand, { from, to, group_by: "agent", offset: 40 })).resolves.toMatchObject({ summary: totals, items: [], total_groups: "1" });
  });
  it("rejects grouping keys and removed legacy wire fields", async () => {
    const directFilter = { ...query, agent_id: agent };
    const wrongAgentGroup = payload({ query: directFilter, items: [{ key: actor, label: actor, totals }] });
    await expect(createAttributionReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(wrongAgentGroup))).report(brand, { from, to, group_by: "agent", agent_id: agent })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const removedField = { ...totals, legacy_attribution_count: "0" };
    await expect(createAttributionReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(payload({ summary: removedField })))).report(brand, { from, to, group_by: "agent" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(createAttributionReportApi(vi.fn<typeof fetch>()).report(brand, { from, to, group_by: "agent", join_method: "legacy" as never })).rejects.toMatchObject({ code: "INVALID_INPUT" });
  });
  it("rejects inconsistent status partitions and nonzero metrics with zero orders", async () => {
    const wrongPartition = { ...totals, placed_count: "1" };
    await expect(createAttributionReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(payload({ summary: wrongPartition })))).report(brand, { from, to, group_by: "agent" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const noOrders = { ...totals, order_count: "0", lost_count: "0" };
    await expect(createAttributionReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(payload({ summary: noOrders })))).report(brand, { from, to, group_by: "agent" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const badRealRow = { ...totals, cancelled_count: "1" };
    const badRowApi = createAttributionReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(payload({ items: [{ key: agent, label: "Agent", totals: badRealRow }] }))));
    await expect(badRowApi.report(brand, { from, to, group_by: "agent" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });
  it("exports a complete CSV only after metadata, digest, echoed filters, and all group sums validate", async () => {
    const snapshot = "2026-01-02T01:00:00Z";
    const columns = ["record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "game_id", "member_id", "agent_id", "agent_scope", "join_method", "key", "label", "order_count", "stake_points", "placed_count", "won_count", "lost_count", "abnormal_count", "cancelled_count", "refund_points", "settled_stake_points", "unfinalized_stake_points", "abnormal_stake_points", "current_prize_points", "correction_open_count", "final_lost_stake_points"];
    const values = Object.values(totals);
    const meta = [brand, snapshot, "Asia/Singapore", from, to, "agent", "", "", agent, "downline", "", "", "", ...values];
    const csv = `\uFEFF${columns.join(",")}\r\nsummary,${meta.join(",")}\r\ngroup,${meta.slice(0, 11).concat([agent, "Agent Alpha"], values).join(",")}\r\n`;
    const bytes = new TextEncoder().encode(csv), digest = [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))].map((n) => n.toString(16).padStart(2, "0")).join("");
    const filename = `lottery-attribution-${brand}-20260102T010000Z.csv`;
    const headers = new Headers({ "X-Report-Brand-ID": brand, "X-Report-Kind": "attribution", "X-Report-Snapshot-At": snapshot, "X-Report-Timezone": "Asia/Singapore", "X-Report-Group-Count": "1", "X-Report-Byte-Count": String(bytes.length), "X-Report-Audit-ID": actor, "X-Report-SHA256": digest, "X-Report-Format-Version": "1", "Content-Length": String(bytes.length), "Content-Type": "text/csv; charset=utf-8", "Content-Disposition": `attachment; filename="${filename}"`, "Cache-Control": "no-store" });
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(bytes, { status: 200, headers }));
    const exported = await createAttributionReportApi(fetcher).exportCsv(brand, { from, to, group_by: "agent", agent_id: agent, agent_scope: "downline" });
    expect(exported.filename).toBe(filename);
    const parsed = new URL(String(fetcher.mock.calls[0]![0]), "http://local");
    expect(parsed.pathname).toBe("/api/v1/admin/reports/attribution/export"); expect(parsed.searchParams.get("agent_id")).toBe(agent); expect(parsed.searchParams.get("agent_scope")).toBe("downline"); expect(parsed.searchParams.has("limit")).toBe(false);
    await expect(createAttributionReportApi(vi.fn<typeof fetch>().mockResolvedValue(new Response(bytes, { status: 200, headers: new Headers([...headers, ["X-Report-SHA256", "0".repeat(64)]]) }))).exportCsv(brand, { from, to, group_by: "agent", agent_id: agent, agent_scope: "downline" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });
  it("preserves formula-escaped game names with spaces and quoted line breaks", async () => {
    const game = "44444444-4444-4444-8444-444444444444", snapshot = "2026-01-02T01:00:00Z", name = "'  =Lottery,\r\nName  ";
    const columns = ["record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "game_id", "member_id", "agent_id", "agent_scope", "join_method", "key", "label", "order_count", "stake_points", "placed_count", "won_count", "lost_count", "abnormal_count", "cancelled_count", "refund_points", "settled_stake_points", "unfinalized_stake_points", "abnormal_stake_points", "current_prize_points", "correction_open_count", "final_lost_stake_points"];
    const values = Object.values(totals), base = [brand, snapshot, "Asia/Singapore", from, to, "game", game, "", "", "direct", "", "", ""];
    const row = (fields: string[]) => fields.map((v) => /[",\r\n]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v).join(",");
    const csvFor = (label: string) => `\uFEFF${row(columns)}\r\n${row(["summary", ...base, ...values])}\r\n${row(["group", ...base.slice(0, 11), game, label, ...values])}\r\n`;
    const makeResponse = async (label: string) => {
      const bytes = new TextEncoder().encode(csvFor(label)), digest = [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))].map((n) => n.toString(16).padStart(2, "0")).join("");
      const filename = `lottery-attribution-${brand}-20260102T010000Z.csv`;
      const headers = new Headers({ "X-Report-Brand-ID": brand, "X-Report-Kind": "attribution", "X-Report-Snapshot-At": snapshot, "X-Report-Timezone": "Asia/Singapore", "X-Report-Group-Count": "1", "X-Report-Byte-Count": String(bytes.length), "X-Report-Audit-ID": actor, "X-Report-SHA256": digest, "X-Report-Format-Version": "1", "Content-Length": String(bytes.length), "Content-Type": "text/csv; charset=utf-8", "Content-Disposition": `attachment; filename="${filename}"`, "Cache-Control": "no-store" });
      return new Response(bytes, { status: 200, headers });
    };
    const file = await createAttributionReportApi(vi.fn<typeof fetch>().mockResolvedValue(await makeResponse(name))).exportCsv(brand, { from, to, group_by: "game", game_id: game });
    const decoded = new TextDecoder().decode(file.bytes.subarray(3)); expect(decoded).toContain(name);
    await expect(createAttributionReportApi(vi.fn<typeof fetch>().mockResolvedValue(await makeResponse("  =Unsafe\r\nName"))).exportCsv(brand, { from, to, group_by: "game", game_id: game })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });
});
