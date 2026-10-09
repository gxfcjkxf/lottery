import { describe, expect, it, vi } from "vitest";
import type { AdminAccount } from "./admin-api";
import { commissionReportPermissions, createCommissionReportApi, type CommissionTotals } from "./commission-report-api";

const brand = "11111111-1111-4111-8111-111111111111", actor = "22222222-2222-4222-8222-222222222222", agent = "abcdefab-cdef-4abc-8def-abcdefabcdef", cycle = "44444444-4444-4444-8444-444444444444";
const from = "2026-01-01T00:00:00Z", to = "2026-01-02T00:00:00Z", huge = "922337203685477580812345678901234567890";
const totals: CommissionTotals = { entry_count: "3", paid_entry_count: "1", paid_points: huge, adjustment_entry_count: "1", adjustment_credit_points: "0", adjustment_debit_points: `${BigInt(huge) + 9n}`, correction_entry_count: "1", correction_credit_points: `${BigInt(huge) + 20n}`, correction_debit_points: `${BigInt(huge) + 20n}`, net_points: "-9" };
const payload = (overrides: Record<string, unknown> = {}) => ({ success: true, request_id: "report-request-1", data: { brand_id: brand, snapshot_at: "2026-01-02T01:00:00Z", timezone: "Asia/Singapore", query: { from, to, group_by: "day", limit: 20, offset: 0, agent_id: null, member_id: null, cycle_id: null }, summary: totals, items: [{ key: "2026-01-01", label: "2026-01-01", totals }], total_groups: "1", ...overrides } });
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
  it("rejects changed scope, invalid arithmetic, unsupported filters, and malformed pages", async () => {
    const api = createCommissionReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(payload())));
    await expect(api.report(brand, { from, to, group_by: "day", agent_id: agent })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const inconsistent = createCommissionReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(payload({ summary: { ...totals, net_points: "1" } }))));
    await expect(inconsistent.report(brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const badCounter = createCommissionReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(payload({ summary: { ...totals, correction_entry_count: "-1" } }))));
    await expect(badCounter.report(brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const badCountSum = createCommissionReportApi(vi.fn<typeof fetch>().mockResolvedValue(json(payload({ summary: { ...totals, entry_count: "2" } }))));
    await expect(badCountSum.report(brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
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
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ error: { message: "unavailable" } }), { status: 503 }));
    await expect(createCommissionReportApi(fetcher).exportCsv(brand.toUpperCase(), { from, to, group_by: "day", agent_id: agent.toUpperCase(), member_id: actor.toUpperCase(), cycle_id: cycle.toUpperCase() })).rejects.toMatchObject({ status: 503 });
    const [url, init] = fetcher.mock.calls[0]!;
    const parsed = new URL(String(url), "http://local");
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    expect(parsed.searchParams.get("agent_id")).toBe(agent);
    expect(parsed.searchParams.get("member_id")).toBe(actor);
    expect(parsed.searchParams.get("cycle_id")).toBe(cycle);
  });
  it("validates CSV bytes, metadata, complete group totals and export query", async () => {
    const snapshot = "2026-01-02T01:00:00Z";
    const columns = ["record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "agent_id", "member_id", "cycle_id", "key", "label", "entry_count", "paid_entry_count", "paid_points", "adjustment_entry_count", "adjustment_credit_points", "adjustment_debit_points", "correction_entry_count", "correction_credit_points", "correction_debit_points", "net_points"];
    const vals = [totals.entry_count, totals.paid_entry_count, totals.paid_points, totals.adjustment_entry_count, totals.adjustment_credit_points, totals.adjustment_debit_points, totals.correction_entry_count, totals.correction_credit_points, totals.correction_debit_points, "'-9"];
    const csv = `\uFEFF${columns.join(",")}\nsummary,${brand},${snapshot},Asia/Singapore,${from},${to},day,,,,,,${vals.join(",")}\ngroup,${brand},${snapshot},Asia/Singapore,${from},${to},day,,,,2026-01-01,2026-01-01,${vals.join(",")}\n`;
    const bytes = new TextEncoder().encode(csv), sha = [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))].map((v) => v.toString(16).padStart(2, "0")).join("");
    const headers = new Headers({ "X-Report-Brand-ID": brand, "X-Report-Kind": "commission", "X-Report-Snapshot-At": snapshot, "X-Report-Timezone": "Asia/Singapore", "X-Report-Group-Count": "1", "X-Report-Byte-Count": String(bytes.length), "X-Report-Audit-ID": actor, "X-Report-SHA256": sha, "X-Report-Format-Version": "2", "Content-Length": String(bytes.length), "Content-Type": "text/csv; charset=utf-8", "Content-Disposition": `attachment; filename="lottery-commission-${brand}-20260102T010000Z.csv"`, "Cache-Control": "no-store" });
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(bytes, { status: 200, headers }));
    const file = await createCommissionReportApi(fetcher).exportCsv(brand.toUpperCase(), { from, to, group_by: "day" });
    expect(file.filename).toBe(`lottery-commission-${brand}-20260102T010000Z.csv`);
    expect(new URL(String(fetcher.mock.calls[0]![0]), "http://local").pathname).toBe("/api/v1/admin/reports/commission.csv");
    await expect(createCommissionReportApi(fetcher).exportCsv(brand, { from, to, group_by: "day", offset: 20 })).rejects.toMatchObject({ code: "INVALID_INPUT" });
    const legacyHeaders = new Headers(headers); legacyHeaders.set("X-Report-Format-Version", "1");
    const legacyFetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(bytes, { status: 200, headers: legacyHeaders }));
    await expect(createCommissionReportApi(legacyFetcher).exportCsv(brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });

    const rawNegative = new TextEncoder().encode(csv.replaceAll("'-9", "-9"));
    const rawHash = [...new Uint8Array(await crypto.subtle.digest("SHA-256", rawNegative))].map((v) => v.toString(16).padStart(2, "0")).join("");
    const rawHeaders = new Headers(headers); rawHeaders.set("Content-Length", String(rawNegative.length)); rawHeaders.set("X-Report-Byte-Count", String(rawNegative.length)); rawHeaders.set("X-Report-SHA256", rawHash);
    const rawFetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(rawNegative, { status: 200, headers: rawHeaders }));
    await expect(createCommissionReportApi(rawFetcher).exportCsv(brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });

    for (const [column, value] of [[18, "2"], [21, "'-8"]] as const) {
      const malformed = new TextEncoder().encode(csv.split("\n").map((line, index) => {
        if (index === 0 || !line) return line;
        const cells = line.split(","); cells[column] = value; return cells.join(",");
      }).join("\n"));
      const malformedHash = [...new Uint8Array(await crypto.subtle.digest("SHA-256", malformed))].map((v) => v.toString(16).padStart(2, "0")).join("");
      const malformedHeaders = new Headers(headers); malformedHeaders.set("Content-Length", String(malformed.length)); malformedHeaders.set("X-Report-Byte-Count", String(malformed.length)); malformedHeaders.set("X-Report-SHA256", malformedHash);
      const malformedFetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(malformed, { status: 200, headers: malformedHeaders }));
      await expect(createCommissionReportApi(malformedFetcher).exportCsv(brand, { from, to, group_by: "day" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    }
  });

  it("accepts the complete 10,000-group CSV limit", async () => {
    const snapshot = "2026-01-02T01:00:00Z", groupTotal = 10_000;
    const columns = ["record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "agent_id", "member_id", "cycle_id", "key", "label", "entry_count", "paid_entry_count", "paid_points", "adjustment_entry_count", "adjustment_credit_points", "adjustment_debit_points", "correction_entry_count", "correction_credit_points", "correction_debit_points", "net_points"];
    const metadata = `group,${brand},${snapshot},Asia/Singapore,${from},${to},agent,,,,`;
    const totalsRow = ["10000", "10000", "0", "0", "0", "0", "0", "0", "0", "0"].join(",");
    const rows = [`\uFEFF${columns.join(",")}`, `summary,${brand},${snapshot},Asia/Singapore,${from},${to},agent,,,,,,${totalsRow}`];
    for (let i = 0; i < groupTotal; i++) {
      const key = `00000000-0000-4000-8000-${i.toString(16).padStart(12, "0")}`;
      rows.push(`${metadata}${key},${key},1,1,0,0,0,0,0,0,0,0`);
    }
    const bytes = new TextEncoder().encode(`${rows.join("\n")}\n`);
    const sha = [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))].map((v) => v.toString(16).padStart(2, "0")).join("");
    const headers = new Headers({ "X-Report-Brand-ID": brand, "X-Report-Kind": "commission", "X-Report-Snapshot-At": snapshot, "X-Report-Timezone": "Asia/Singapore", "X-Report-Group-Count": String(groupTotal), "X-Report-Byte-Count": String(bytes.length), "X-Report-Audit-ID": actor, "X-Report-SHA256": sha, "X-Report-Format-Version": "2", "Content-Length": String(bytes.length), "Content-Type": "text/csv; charset=utf-8", "Content-Disposition": `attachment; filename="lottery-commission-${brand}-20260102T010000Z.csv"`, "Cache-Control": "no-store" });
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(bytes, { status: 200, headers }));
    await expect(createCommissionReportApi(fetcher).exportCsv(brand, { from, to, group_by: "agent" })).resolves.toMatchObject({ groupCount: "10000" });
  });
});
