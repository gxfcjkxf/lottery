import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import {
  BUSINESS_INVENTORY_SOURCES, businessInventoryPermissions, createBusinessInventoryApi,
  type BrandBusinessInventory, type BusinessInventoryIssue,
} from "./business-inventory-api";

const brand = "11111111-1111-4111-8111-111111111111";
const accountId = "22222222-2222-4222-8222-222222222222";
const stamp = "2026-10-09T08:30:00.123456Z";

function inventory(patch: Partial<BrandBusinessInventory> = {}): BrandBusinessInventory {
  return {
    brand_id: brand, snapshot_at: stamp, schema_version: 1, source_row_count: "2", reference_count: "3", issue_count: "0",
    issues_truncated: false, consistent: true, fingerprint: "a".repeat(64),
    coverage: BUSINESS_INVENTORY_SOURCES.map((source, index) => ({ source_table: source, source_row_count: index === 0 ? "2" : "0", reference_count: index === 0 ? "3" : "0", issue_count: "0" })),
    issues: [], ...patch,
  };
}

function inventoryWithIssues(issues: BusinessInventoryIssue[], totalIssues = String(issues.length), referenceCount = String(Math.max(3, Number(totalIssues)))): BrandBusinessInventory {
  const count = Number(totalIssues);
  return inventory({
    reference_count: referenceCount, issue_count: totalIssues, issues_truncated: BigInt(totalIssues) > 100n, consistent: count === 0, issues,
    coverage: BUSINESS_INVENTORY_SOURCES.map((source, index) => ({
      source_table: source, source_row_count: index === 0 ? "2" : "0",
      reference_count: index === 0 ? referenceCount : "0", issue_count: index === 0 ? totalIssues : "0",
    })),
  });
}

function response(data: unknown, status = 200) {
  return new Response(JSON.stringify({ success: true, data }), { status, headers: { "Content-Type": "application/json" } });
}

function account(patch: Partial<AdminAccount> = {}): AdminAccount {
  return { id: accountId, super_admin: false, brand_ids: [brand], permissions: [], permissions_by_brand: { [brand]: ["wallet.view.brand"] }, ...patch };
}

function issue(patch: Partial<BusinessInventoryIssue> = {}): BusinessInventoryIssue {
  return { source_table: "bet_order_exceptions", source_id: "brand-id/order-id", code: "MISSING_PARENT_REFERENCE", reference_key: "member_id", parent_table: "members", ...patch };
}

describe("brand business inventory SDK", () => {
  it("uses explicit wallet.view grants, without super-admin inference or reconciliation write permission", () => {
    expect(businessInventoryPermissions(account({ permissions_by_brand: { [brand]: ["wallet.view.brand"] } }), brand)).toEqual({ view: true });
    expect(businessInventoryPermissions(account({ brand_ids: [], permissions_by_brand: {}, platform_permissions: ["wallet.view.platform"], super_admin: true }), brand)).toEqual({ view: false });
    expect(businessInventoryPermissions(account({ permissions_by_brand: {}, super_admin: true }), brand)).toEqual({ view: false });
    expect(businessInventoryPermissions(account({ permissions_by_brand: { [brand]: ["wallet.reconcile.brand"] } }), brand)).toEqual({ view: false });
  });

  it("sends the exact scoped GET with no query or body and validates the closed DTO", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response(inventory()));
    const result = await createBusinessInventoryApi(fetcher).read(brand);
    expect(BUSINESS_INVENTORY_SOURCES).toHaveLength(41);
    expect(Object.keys(result)).toHaveLength(11); // The named DTO fields in docs/37; success/data are the outer envelope.
    expect(fetcher).toHaveBeenCalledWith("/api/v1/admin/reconciliations/business-inventory", expect.objectContaining({
      method: "GET", credentials: "same-origin", headers: expect.any(Headers),
    }));
    const [, init] = fetcher.mock.calls[0]!;
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    expect(init?.body).toBeUndefined();
    expect(new URL("/api/v1/admin/reconciliations/business-inventory", "http://localhost").search).toBe("");
  });

  it("accepts canonical large counts and composite source IDs while requiring sorted, complete issues", async () => {
    const first = issue();
    const next = issue({ source_id: "brand-id/order-id/part-2", reference_key: "account_id" });
    const value = inventoryWithIssues([first, next]);
    await expect(createBusinessInventoryApi(vi.fn<typeof fetch>().mockResolvedValue(response(value))).read(brand)).resolves.toMatchObject({ issues: [first, next] });

    const large = "90071992547409931234567890";
    const largeCoverage = inventory().coverage.map((row, index) => ({ ...row, source_row_count: index === 0 ? "2" : "0", reference_count: index === 0 ? large : "0" }));
    const largeFetcher = vi.fn<typeof fetch>().mockResolvedValue(response(inventory({ reference_count: large, coverage: largeCoverage })));
    await expect(createBusinessInventoryApi(largeFetcher).read(brand))
      .resolves.toMatchObject({ reference_count: large });
    const maxDigits = "9".repeat(200);
    const maxDigitCoverage = inventory().coverage.map((row, index) => ({ ...row, source_row_count: index === 0 ? "2" : "0", reference_count: index === 0 ? maxDigits : "0" }));
    await expect(createBusinessInventoryApi(vi.fn<typeof fetch>().mockResolvedValue(response(inventory({ reference_count: maxDigits, coverage: maxDigitCoverage })))).read(brand))
      .resolves.toMatchObject({ reference_count: maxDigits });
    const maxIdentifier = `a${"b".repeat(62)}`;
    const identifierFetcher = vi.fn<typeof fetch>().mockResolvedValue(response(inventoryWithIssues([issue({ reference_key: maxIdentifier, parent_table: maxIdentifier })])));
    await expect(createBusinessInventoryApi(identifierFetcher).read(brand))
      .resolves.toMatchObject({ issues: [{ reference_key: maxIdentifier, parent_table: maxIdentifier }] });

    const tupleOrder = [
      issue({ source_id: "same-key", reference_key: "a_reference", code: "MISSING_REQUIRED_LEDGER_REFERENCE" }),
      issue({ source_id: "same-key", reference_key: "z_reference", code: "MISSING_PARENT_REFERENCE" }),
    ];
    const tupleFetcher = vi.fn<typeof fetch>().mockResolvedValue(response(inventoryWithIssues(tupleOrder)));
    await expect(createBusinessInventoryApi(tupleFetcher).read(brand)).resolves.toMatchObject({ issues: tupleOrder });
  });

  it("rejects extra keys, incorrect totals, malformed timestamps, unsafe issue fields, and inconsistent truncation", async () => {
    const baseline = inventory();
    const badCoverage = baseline.coverage.map((row, index) => index === 0 ? { ...row, reference_count: "4" } : row);
    const malformed = [
      { ...baseline, extra: "forbidden" },
      { ...baseline, schema_version: 2 },
      { ...baseline, snapshot_at: "2026-10-09T08:30:00+00:00" },
      { ...baseline, snapshot_at: "2026-02-30T08:30:00Z" },
      { ...baseline, source_row_count: "100001" },
      { ...baseline, source_row_count: "02" },
      { ...baseline, coverage: baseline.coverage.slice(1) },
      { ...baseline, coverage: badCoverage },
      { ...baseline, fingerprint: "A".repeat(64) },
      { ...baseline, issues: [issue()] },
      { ...inventoryWithIssues([issue()], "1"), issues: [issue({ parent_id: "raw" } as never)] },
      { ...inventoryWithIssues([issue()], "1"), issues: [issue({ source_id: `a${"b".repeat(500)}` })] },
      { ...inventoryWithIssues([issue()], "1"), issues: [issue({ source_id: "order-é" })] },
      { ...inventoryWithIssues([issue()], "1"), issues: [issue({ reference_key: "A" })] },
      { ...inventoryWithIssues([issue()], "1"), issues: [issue({ reference_key: `a${"b".repeat(63)}` })] },
      { ...inventoryWithIssues([issue()], "1"), issues: [issue({ parent_table: "parent-table" })] },
      { ...inventoryWithIssues([issue()], "1"), issues: [issue({ parent_table: `p${"a".repeat(63)}` })] },
      { ...baseline, reference_count: `1${"0".repeat(200)}`, coverage: baseline.coverage.map((row, index) => index === 0 ? { ...row, reference_count: `1${"0".repeat(200)}` } : row) },
    ];
    for (const value of malformed) {
      await expect(createBusinessInventoryApi(vi.fn<typeof fetch>().mockResolvedValue(response(value))).read(brand))
        .rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    }
    const totalCapCoverage = baseline.coverage.map((row, index) => index === 0 ? { ...row, source_row_count: "100001" } : row);
    await expect(createBusinessInventoryApi(vi.fn<typeof fetch>().mockResolvedValue(response(inventory({ source_row_count: "100001", coverage: totalCapCoverage })))).read(brand))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const duplicateOrder = [issue({ source_id: "same-id" }), issue({ source_id: "same-id" })];
    const duplicateFetcher = vi.fn<typeof fetch>().mockResolvedValue(response(inventoryWithIssues(duplicateOrder)));
    await expect(createBusinessInventoryApi(duplicateFetcher).read(brand))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE" });

    const tooManyDigits = `1${"0".repeat(200)}`;
    await expect(createBusinessInventoryApi(vi.fn<typeof fetch>().mockResolvedValue(response({
      ...baseline, reference_count: tooManyDigits,
      coverage: baseline.coverage.map((row, index) => index === 0 ? { ...row, reference_count: tooManyDigits } : row),
    }))).read(brand)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });

    const issueBeyondReferences = inventoryWithIssues([issue(), issue({ source_id: "second-id" })]);
    const issueOverReference = { ...issueBeyondReferences, reference_count: "1", coverage: issueBeyondReferences.coverage.map((row, index) => index === 0 ? { ...row, reference_count: "1" } : row) };
    await expect(createBusinessInventoryApi(vi.fn<typeof fetch>().mockResolvedValue(response(issueOverReference))).read(brand))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE" });

    const zeroSourceReferences = { ...baseline, reference_count: "4", coverage: baseline.coverage.map((row, index) => index === 1 ? { ...row, reference_count: "1" } : row) };
    await expect(createBusinessInventoryApi(vi.fn<typeof fetch>().mockResolvedValue(response(zeroSourceReferences))).read(brand))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE" });

    const sampleOverCoverage = Array.from({ length: 100 }, (_, index) => issue({ source_id: `display-${String(index).padStart(3, "0")}` }));
    const sampledCoverage = BUSINESS_INVENTORY_SOURCES.map((source, index) => ({
      source_table: source, source_row_count: index === 0 ? "2" : index === 1 ? "1" : "0",
      reference_count: index === 0 ? "99" : index === 1 ? "2" : "0", issue_count: index === 0 ? "99" : index === 1 ? "2" : "0",
    }));
    const sampledOverCount = inventory({ source_row_count: "3", reference_count: "101", issue_count: "101", consistent: false, issues_truncated: true,
      coverage: sampledCoverage, issues: sampleOverCoverage });
    await expect(createBusinessInventoryApi(vi.fn<typeof fetch>().mockResolvedValue(response(sampledOverCount))).read(brand))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("requires an exact first-100 sample and full count/truncation agreement", async () => {
    const sampled = Array.from({ length: 100 }, (_, index) => issue({ source_id: `row-${String(index).padStart(3, "0")}` }));
    const valid = inventoryWithIssues(sampled, "101", "101");
    await expect(createBusinessInventoryApi(vi.fn<typeof fetch>().mockResolvedValue(response(valid))).read(brand)).resolves.toMatchObject({ issue_count: "101", issues: sampled });
    const invalidTruncation = { ...valid, issue_count: "100", issues_truncated: true,
      coverage: valid.coverage.map((row, index) => index === 0 ? { ...row, issue_count: "100", reference_count: "100" } : row) };
    const truncationFetcher = vi.fn<typeof fetch>().mockResolvedValue(response(invalidTruncation));
    await expect(createBusinessInventoryApi(truncationFetcher).read(brand))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("returns localized-friendly AdminApiErrors for network, HTTP, and invalid payload failures", async () => {
    await expect(createBusinessInventoryApi(vi.fn<typeof fetch>().mockRejectedValue(new Error("offline"))).read(brand))
      .rejects.toBeInstanceOf(AdminApiError);
    await expect(createBusinessInventoryApi(vi.fn<typeof fetch>().mockResolvedValue(response({ message: "denied" }, 403))).read(brand))
      .rejects.toMatchObject({ status: 403 });
    await expect(createBusinessInventoryApi(vi.fn<typeof fetch>().mockResolvedValue(response({}))).read(brand))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(createBusinessInventoryApi(vi.fn<typeof fetch>()).read("bad-brand"))
      .rejects.toMatchObject({ code: "INVALID_INPUT" });
  });
});
