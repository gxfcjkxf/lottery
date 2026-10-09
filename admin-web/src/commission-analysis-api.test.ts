import { describe, expect, it, vi } from "vitest";
import type { AdminAccount } from "./admin-api";
import { COMMISSION_ANALYSIS_COVERAGE_FIELDS, COMMISSION_ANALYSIS_FIELDS, commissionAnalysisPermissions, createCommissionAnalysisApi, type CommissionAnalysisCoverage, type CommissionAnalysisGroup, type CommissionAnalysisTotals } from "./commission-analysis-api";

const brand = "11111111-1111-4111-8111-111111111111", actor = "22222222-2222-4222-8222-222222222222", key = "abcdefab-cdef-4abc-8def-abcdefabcdef";
const from = "2026-01-01T00:00:00Z", to = "2026-01-02T00:00:00Z", huge = "922337203685477580812345678901234567890";
const coverage = { selected_cycle_count: "1", ready_cycle_count: "1", unready_cycle_count: "0" };
const totals: CommissionAnalysisTotals = {
  observed_calculated_points: huge, calculated_points: huge, paid_entry_count: "1", paid_points: huge,
  adjustment_entry_count: "1", adjustment_credit_points: "0", adjustment_debit_points: `${BigInt(huge) + 9n}`,
  correction_entry_count: "1", correction_credit_points: `${BigInt(huge) + 20n}`, correction_debit_points: `${BigInt(huge) + 20n}`,
  posting_entry_count: "3", actual_net_points: "-9", manual_adjustment_net_points: `${-(BigInt(huge) + 9n)}`,
  effective_target_points: huge, calculation_minus_actual_points: `${BigInt(huge) + 9n}`, effective_minus_actual_points: `${BigInt(huge) + 9n}`,
  calculation_complete: true, effective_target_complete: true,
};
const query = { from, to, group_by: "cycle", limit: 20, offset: 0, agent_id: null, member_id: null, cycle_id: null };
const payload = (overrides: Record<string, unknown> = {}) => ({ success: true, request_id: "analysis-1", data: { brand_id: brand, snapshot_at: "2026-01-02T01:00:00Z", timezone: "Asia/Singapore", query, coverage, summary: totals, items: [{ key, label: key, totals }], total_groups: "1", ...overrides } });
const json = (value: unknown) => new Response(JSON.stringify(value), { status: 200 });
const unfinishedCoverage = { ...coverage, ready_cycle_count: "0", unready_cycle_count: "1" };
const zeroTotals: CommissionAnalysisTotals = Object.fromEntries(COMMISSION_ANALYSIS_FIELDS.map((field) => [field, field.endsWith("_complete") ? true : "0"])) as CommissionAnalysisTotals;
const incomplete = (value: CommissionAnalysisTotals): CommissionAnalysisTotals => ({ ...value, calculated_points: null, effective_target_points: null, calculation_minus_actual_points: null, effective_minus_actual_points: null, calculation_complete: false, effective_target_complete: false });
async function csvResponse(summary: CommissionAnalysisTotals, items: CommissionAnalysisTotals[], cov: CommissionAnalysisCoverage = coverage, group: CommissionAnalysisGroup = "cycle", timezone = "Asia/Singapore") {
  const snapshot = "2026-01-02T01:00:00Z";
  const columns = ["record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "agent_id", "member_id", "cycle_id", "key", "label", ...COMMISSION_ANALYSIS_COVERAGE_FIELDS, ...COMMISSION_ANALYSIS_FIELDS];
  const row = (kind: string, id: string, value: CommissionAnalysisTotals) => [kind, brand, snapshot, timezone, from, to, group, "", "", "", id, id, ...COMMISSION_ANALYSIS_COVERAGE_FIELDS.map((f) => cov[f]), ...COMMISSION_ANALYSIS_FIELDS.map((f) => {
    const cell = value[f]; return cell === null ? "" : typeof cell === "boolean" ? String(cell) : cell.startsWith("-") ? `'${cell}` : cell;
  })].join(",");
  const bytes = new TextEncoder().encode(`\uFEFF${columns.join(",")}\n${row("summary", "", summary)}\n${items.map((value, i) => row("group", `00000000-0000-4000-8000-${i.toString(16).padStart(12, "0")}`, value)).map((r) => `${r}\n`).join("")}`);
  const digest = [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))].map((v) => v.toString(16).padStart(2, "0")).join("");
  return new Response(bytes, { status: 200, headers: { "X-Report-Brand-ID": brand, "X-Report-Kind": "commission_analysis", "X-Report-Snapshot-At": snapshot, "X-Report-Timezone": timezone, "X-Report-Group-Count": String(items.length), "X-Report-Byte-Count": String(bytes.length), "X-Report-Audit-ID": actor, "X-Report-SHA256": digest, "X-Report-Format-Version": "1", "Content-Length": String(bytes.length), "Content-Type": "text/csv; charset=utf-8", "Content-Disposition": `attachment; filename="lottery-commission-analysis-${brand}-20260102T010000Z.csv"`, "Cache-Control": "no-store" } });
}

describe("commission analysis permissions", () => {
  const base: AdminAccount = { id: actor, super_admin: true, brand_ids: [brand], permissions: [], permissions_by_brand: { [brand]: ["commission.view.brand", "report_commission.view.brand"] } };
  it("requires commission and report view, plus a separate report export grant", () => {
    expect(commissionAnalysisPermissions(base, brand)).toEqual({ view: true, export: false });
    expect(commissionAnalysisPermissions({ ...base, permissions_by_brand: { [brand]: ["commission.view.brand", "report_commission.export.brand"] } }, brand).view).toBe(false);
    expect(commissionAnalysisPermissions({ ...base, permissions_by_brand: { [brand]: ["commission.view.brand", "report_commission.view.brand", "report_commission.export.brand"] } }, brand)).toEqual({ view: true, export: true });
    expect(commissionAnalysisPermissions({ ...base, brand_ids: [], permissions_by_brand: {}, platform_permissions: ["commission.view.platform", "report_commission.view.platform", "report_commission.export.platform"] }, brand)).toEqual({ view: true, export: true });
    expect(commissionAnalysisPermissions({ ...base, permissions_by_brand: { [brand]: ["commission.view.brand", "report_commission.view.brand"] }, platform_permissions: ["report_commission.export.platform"] }, brand)).toEqual({ view: true, export: true });
    expect(commissionAnalysisPermissions({ ...base, permissions_by_brand: {}, platform_permissions: [] }, brand)).toEqual({ view: false, export: false });
  });
});

describe("commission analysis API", () => {
  it("keeps all point integers as strings and validates whole-page totals", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(json(payload()));
    const result = await createCommissionAnalysisApi(fetcher).report(brand.toUpperCase(), { from, to, group_by: "cycle" });
    expect(result.summary.paid_points).toBe(huge);
    expect(result.summary.manual_adjustment_net_points).toBe(`${-(BigInt(huge) + 9n)}`);
    expect(result.summary.calculation_minus_actual_points).toBe(`${BigInt(huge) + 9n}`);
    expect(result.items[0]?.label).toBe(key);
    const [url, init] = fetcher.mock.calls[0]!;
    expect(String(url)).toContain("/api/v1/admin/reports/commission-analysis?");
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    expect(new URL(String(url), "http://local").searchParams.get("limit")).toBe("20");
  });
  it("checks exact response keys, coverage, count and every arithmetic/null equation", async () => {
    const good = createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(json(payload())));
    await expect(good.report(brand, { from, to, group_by: "cycle" })).resolves.toBeDefined();
    const invalids = [
      { summary: { ...totals, actual_net_points: "0" } },
      { summary: { ...totals, effective_minus_actual_points: null } },
      { summary: { ...totals, posting_entry_count: "4" } },
      { summary: { ...totals, calculation_complete: false } },
      { summary: { ...totals, effective_target_complete: false, effective_target_points: huge } },
      { coverage: { ...coverage, unready_cycle_count: "1" } },
      { query: { ...query, agent_id: key } },
      { items: [{ key, label: "different", totals }] },
      { items: [{ key: key.toUpperCase(), label: key.toUpperCase(), totals }] },
      { data_extra: true },
    ];
    for (const override of invalids) {
      const data = { ...payload(), data: { ...(payload() as { data: Record<string, unknown> }).data, ...override } };
      await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(json(data))).report(brand, { from, to, group_by: "cycle" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    }
    const badFilter = createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(json(payload())));
    await expect(badFilter.report(brand, { from, to: "2026-04-05T00:00:00Z", group_by: "cycle" })).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(badFilter.exportCsv(brand, { from, to, group_by: "cycle", offset: 0 })).rejects.toMatchObject({ code: "INVALID_INPUT" });
  });
  it("preserves unknown calculated values as null and rejects nullability drift", async () => {
    const unknownTotals = incomplete({ ...totals, observed_calculated_points: "0" });
    const unknown = payload({ coverage: unfinishedCoverage, summary: unknownTotals, items: [{ key, label: key, totals: unknownTotals }] });
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(json(unknown))).report(brand, { from, to, group_by: "cycle" })).resolves.toMatchObject({ summary: { calculated_points: null, effective_target_points: null } });
    const mismatch = payload({ items: [{ key, label: key, totals: unknownTotals }] });
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(json(mismatch))).report(brand, { from, to, group_by: "cycle" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });
  it("accepts empty complete cycles and empty globally-unready agent cohorts", async () => {
    const zeros: CommissionAnalysisTotals = {
      observed_calculated_points: "0", calculated_points: "0", paid_entry_count: "0", paid_points: "0",
      adjustment_entry_count: "0", adjustment_credit_points: "0", adjustment_debit_points: "0", correction_entry_count: "0",
      correction_credit_points: "0", correction_debit_points: "0", posting_entry_count: "0", actual_net_points: "0",
      manual_adjustment_net_points: "0", effective_target_points: "0", calculation_minus_actual_points: "0", effective_minus_actual_points: "0",
      calculation_complete: true, effective_target_complete: true,
    };
    const readyCoverage = { selected_cycle_count: "1", ready_cycle_count: "1", unready_cycle_count: "0" };
    const readyCycle = payload({ coverage: readyCoverage, summary: zeros, items: [{ key, label: key, totals: zeros }] });
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(json(readyCycle))).report(brand, { from, to, group_by: "cycle" })).resolves.toMatchObject({ summary: zeros, coverage: readyCoverage });

    const unready: CommissionAnalysisTotals = { ...zeros, calculated_points: null, effective_target_points: null, calculation_minus_actual_points: null, effective_minus_actual_points: null, calculation_complete: false, effective_target_complete: false };
    const agentQuery = { ...query, group_by: "agent", limit: 20, offset: 0 };
    const noBeneficiaries = payload({ query: agentQuery, coverage: { selected_cycle_count: "1", ready_cycle_count: "0", unready_cycle_count: "1" }, summary: unready, items: [], total_groups: "0" });
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(json(noBeneficiaries))).report(brand, { from, to, group_by: "agent" })).resolves.toMatchObject({ summary: unready, items: [], coverage: noBeneficiaries.data.coverage });
  });
  it("accepts a valid manual offset on the same evidence and a new evidence target that supersedes it", async () => {
    const manualOffset: CommissionAnalysisTotals = { ...totals, observed_calculated_points: "10", calculated_points: "10", paid_entry_count: "1", paid_points: "10", adjustment_entry_count: "1", adjustment_credit_points: "2", adjustment_debit_points: "0", correction_entry_count: "0", correction_credit_points: "0", correction_debit_points: "0", posting_entry_count: "2", actual_net_points: "12", manual_adjustment_net_points: "2", effective_target_points: "12", calculation_minus_actual_points: "-2", effective_minus_actual_points: "0" };
    const sameEvidence = payload({ coverage: { selected_cycle_count: "1", ready_cycle_count: "1", unready_cycle_count: "0" }, summary: manualOffset, items: [{ key, label: key, totals: manualOffset }] });
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(json(sameEvidence))).report(brand, { from, to, group_by: "cycle" })).resolves.toMatchObject({ summary: { calculated_points: "10", effective_target_points: "12", manual_adjustment_net_points: "2" } });

    const newEvidence: CommissionAnalysisTotals = { ...manualOffset, observed_calculated_points: "8", calculated_points: "8", effective_target_points: "8", calculation_minus_actual_points: "-4", effective_minus_actual_points: "-4" };
    const superseded = payload({ coverage: { selected_cycle_count: "1", ready_cycle_count: "1", unready_cycle_count: "0" }, summary: newEvidence, items: [{ key, label: key, totals: newEvidence }] });
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(json(superseded))).report(brand, { from, to, group_by: "cycle" })).resolves.toMatchObject({ summary: { calculated_points: "8", effective_target_points: "8", manual_adjustment_net_points: "2", effective_target_complete: true } });
  });
  it("preserves whole-filter summary when the requested page is empty", async () => {
    const data = payload({ query: { ...query, offset: 20 }, items: [] });
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(json(data))).report(brand, { from, to, group_by: "cycle", offset: 20 })).resolves.toMatchObject({ summary: totals, items: [], total_groups: "1" });
  });
  it("rejects tampered observed points even when summary and groups agree with each other", async () => {
    const tampered = { ...totals, observed_calculated_points: `${BigInt(huge) + 1n}` };
    const data = payload({ summary: tampered, items: [{ key, label: key, totals: tampered }] });
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(json(data))).report(brand, { from, to, group_by: "cycle" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(await csvResponse(tampered, [tampered]))).exportCsv(brand, { from, to, group_by: "cycle" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });
  it("requires zero/complete for known empty results and permits null/incomplete only for an unfinished empty agent cohort", async () => {
    const none = { ...coverage, selected_cycle_count: "0", ready_cycle_count: "0" };
    const unknown = incomplete(zeroTotals);
    const unknownEffective = { ...zeroTotals, effective_target_points: null, effective_minus_actual_points: null, effective_target_complete: false };
    for (const group of ["cycle", "agent"] as const) {
      for (const summary of [unknown, unknownEffective]) {
        const data = payload({ query: { ...query, group_by: group, offset: 20 }, coverage: none, summary, items: [], total_groups: "0" });
        await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(json(data))).report(brand, { from, to, group_by: group, offset: 20 })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
        await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(await csvResponse(summary, [], none, group))).exportCsv(brand, { from, to, group_by: group })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
      }
      await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(await csvResponse(zeroTotals, [], none, group))).exportCsv(brand, { from, to, group_by: group })).resolves.toMatchObject({ groupCount: "0" });
    }
    const unfinished = payload({ query: { ...query, group_by: "agent" }, coverage: unfinishedCoverage, summary: unknown, items: [], total_groups: "0" });
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(json(unfinished))).report(brand, { from, to, group_by: "agent" })).resolves.toMatchObject({ summary: unknown });
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(await csvResponse(unknown, [], unfinishedCoverage, "agent"))).exportCsv(brand, { from, to, group_by: "agent" })).resolves.toMatchObject({ groupCount: "0" });
  });
  it("binds summary completeness and cycle group counts to coverage in both response formats", async () => {
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(json(payload({ coverage: unfinishedCoverage, items: [], query: { ...query, offset: 20 } })))).report(brand, { from, to, group_by: "cycle", offset: 20 })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(await csvResponse(totals, [totals], unfinishedCoverage))).exportCsv(brand, { from, to, group_by: "cycle" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const wrongCount = payload({ summary: zeroTotals, items: [{ key, label: key, totals: zeroTotals }], total_groups: "2", query: { ...query, limit: 1 } });
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(json(wrongCount))).report(brand, { from, to, group_by: "cycle", limit: 1 })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(await csvResponse(zeroTotals, [zeroTotals, zeroTotals]))).exportCsv(brand, { from, to, group_by: "cycle" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });
  it("uses global agent completeness while preserving observed ready evidence and whole-filter summary on partial pages", async () => {
    const mixed = { ...coverage, selected_cycle_count: "2", unready_cycle_count: "1" };
    const summary = incomplete(totals), pageTotals = incomplete(zeroTotals);
    const data = payload({ query: { ...query, group_by: "agent", limit: 1 }, coverage: mixed, summary, items: [{ key, label: key, totals: pageTotals }], total_groups: "2" });
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(json(data))).report(brand, { from, to, group_by: "agent", limit: 1 })).resolves.toMatchObject({ summary: { observed_calculated_points: huge, calculated_points: null }, items: [{ totals: { observed_calculated_points: "0", calculated_points: null } }] });
    const inconsistentAgent = { ...data, data: { ...data.data, items: [{ key, label: key, totals: zeroTotals }] } };
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(json(inconsistentAgent))).report(brand, { from, to, group_by: "agent", limit: 1 })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(await csvResponse(summary, [pageTotals, summary], mixed, "agent"))).exportCsv(brand, { from, to, group_by: "agent" })).resolves.toMatchObject({ groupCount: "2" });
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(await csvResponse(summary, [zeroTotals, summary], mixed, "agent"))).exportCsv(brand, { from, to, group_by: "agent" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const unknownEffective = { ...totals, effective_target_points: null, effective_minus_actual_points: null, effective_target_complete: false };
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(await csvResponse(unknownEffective, [unknownEffective]))).exportCsv(brand, { from, to, group_by: "cycle" })).resolves.toMatchObject({ groupCount: "1" });
  });
  it("rejects the producer's noncanonical Local timezone for JSON and CSV", async () => {
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(json(payload({ timezone: "Local" })))).report(brand, { from, to, group_by: "cycle" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(await csvResponse(totals, [totals], coverage, "cycle", "Local"))).exportCsv(brand, { from, to, group_by: "cycle" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });
  it("validates a real signed, nullable 33-column export, digest, metadata and whole-group sums", async () => {
    const snapshot = "2026-01-02T01:00:00Z";
    const columns = ["record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "agent_id", "member_id", "cycle_id", "key", "label", ...COMMISSION_ANALYSIS_COVERAGE_FIELDS, ...COMMISSION_ANALYSIS_FIELDS];
    const toCells = (type: string, rowKey: string, rowTotals: CommissionAnalysisTotals, cov = coverage) => [type, brand, snapshot, "Asia/Singapore", from, to, "cycle", "", "", "", rowKey, rowKey, ...COMMISSION_ANALYSIS_COVERAGE_FIELDS.map((f) => cov[f]), ...COMMISSION_ANALYSIS_FIELDS.map((f) => {
      const value = rowTotals[f]; if (typeof value === "boolean") return String(value); if (value === null) return "";
      return value.startsWith("-") ? `'${value}` : value;
    })].join(",");
    const csv = `\uFEFF${columns.join(",")}\n${toCells("summary", "", totals)}\n${toCells("group", key, totals)}\n`;
    const bytes = new TextEncoder().encode(csv), sha = [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))].map((v) => v.toString(16).padStart(2, "0")).join("");
    const filename = `lottery-commission-analysis-${brand}-20260102T010000Z.csv`;
    const headers = new Headers({ "X-Report-Brand-ID": brand, "X-Report-Kind": "commission_analysis", "X-Report-Snapshot-At": snapshot, "X-Report-Timezone": "Asia/Singapore", "X-Report-Group-Count": "1", "X-Report-Byte-Count": String(bytes.length), "X-Report-Audit-ID": actor, "X-Report-SHA256": sha, "X-Report-Format-Version": "1", "Content-Length": String(bytes.length), "Content-Type": "text/csv; charset=utf-8", "Content-Disposition": `attachment; filename="${filename}"`, "Cache-Control": "no-store" });
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(bytes, { status: 200, headers }));
    const file = await createCommissionAnalysisApi(fetcher).exportCsv(brand, { from, to, group_by: "cycle" });
    expect(file.filename).toBe(filename); expect(file.groupCount).toBe("1"); expect(file.bytes).toEqual(bytes);
    expect(new URL(String(fetcher.mock.calls[0]![0]), "http://local").pathname).toBe("/api/v1/admin/reports/commission-analysis.csv");
    const wrong = new TextEncoder().encode(csv.replace("'-9", "-9"));
    const wrongHeaders = new Headers(headers); wrongHeaders.set("Content-Length", String(wrong.length)); wrongHeaders.set("X-Report-Byte-Count", String(wrong.length)); wrongHeaders.set("X-Report-SHA256", [...new Uint8Array(await crypto.subtle.digest("SHA-256", wrong))].map((v) => v.toString(16).padStart(2, "0")).join(""));
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(new Response(wrong, { status: 200, headers: wrongHeaders }))).exportCsv(brand, { from, to, group_by: "cycle" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const wrongDigest = new Headers(headers); wrongDigest.set("X-Report-SHA256", "0".repeat(64));
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(new Response(bytes, { status: 200, headers: wrongDigest }))).exportCsv(brand, { from, to, group_by: "cycle" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const unknownTotals = incomplete({ ...totals, observed_calculated_points: "0" });
    const nullCsv = `\uFEFF${columns.join(",")}\n${toCells("summary", "", unknownTotals, unfinishedCoverage)}\n${toCells("group", key, unknownTotals, unfinishedCoverage)}\n`;
    const nullBytes = new TextEncoder().encode(nullCsv), nullHeaders = new Headers(headers);
    nullHeaders.set("Content-Length", String(nullBytes.length)); nullHeaders.set("X-Report-Byte-Count", String(nullBytes.length)); nullHeaders.set("X-Report-SHA256", [...new Uint8Array(await crypto.subtle.digest("SHA-256", nullBytes))].map((v) => v.toString(16).padStart(2, "0")).join(""));
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(new Response(nullBytes, { status: 200, headers: nullHeaders }))).exportCsv(brand, { from, to, group_by: "cycle" })).resolves.toMatchObject({ groupCount: "1" });
    const nullDrift = new TextEncoder().encode(`\uFEFF${columns.join(",")}\n${toCells("summary", "", unknownTotals, unfinishedCoverage)}\n${toCells("group", key, totals, unfinishedCoverage)}\n`), driftHeaders = new Headers(nullHeaders);
    driftHeaders.set("Content-Length", String(nullDrift.length)); driftHeaders.set("X-Report-Byte-Count", String(nullDrift.length)); driftHeaders.set("X-Report-SHA256", [...new Uint8Array(await crypto.subtle.digest("SHA-256", nullDrift))].map((v) => v.toString(16).padStart(2, "0")).join(""));
    await expect(createCommissionAnalysisApi(vi.fn<typeof fetch>().mockResolvedValue(new Response(nullDrift, { status: 200, headers: driftHeaders }))).exportCsv(brand, { from, to, group_by: "cycle" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });
});
