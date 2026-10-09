import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { auditPermissions, createAuditApi, type AuditQuery } from "./audit-api";

const brand = "11111111-1111-4111-8111-111111111111";
const otherBrand = "22222222-2222-4222-8222-222222222222";
const actor = "33333333-3333-4333-8333-333333333333";
const recordId = "44444444-4444-4444-8444-444444444444";
const exportId = "55555555-5555-4555-8555-555555555555";
const from = "2026-10-01T00:00:00.000Z";
const to = "2026-10-02T00:00:00.000Z";
const columns = ["id", "brand_id", "action", "actor_type", "actor_id", "resource_type", "resource_id", "reason", "request_id", "created_at", "ip_address", "before_json", "after_json"];
const csv = `${columns.join(",")}\r\n${[recordId, brand, "user.update", "admin", actor, "user", "user-7", "reviewed", "req-1", "2026-10-01T12:00:00.000000001Z", "127.0.0.1", "{}", "{}"].join(",")}\r\n`;
const bytes = new TextEncoder().encode(`\uFEFF${csv}`);
async function digest(input: Uint8Array) { return [...new Uint8Array(await crypto.subtle.digest("SHA-256", input))].map(value => value.toString(16).padStart(2, "0")).join(""); }
function exportResponse(overrides: Record<string, string> = {}, body = bytes, length = body.length) {
  const headers = new Headers({
    "Content-Type": "text/csv; charset=utf-8",
    "Content-Length": String(length),
    "Cache-Control": "no-store",
    "X-Content-Type-Options": "nosniff",
    "Content-Disposition": `attachment; filename="audit-${brand}-v1.csv"`,
    "X-Audit-Brand-ID": brand,
    "X-Audit-Snapshot-At": "2026-10-02T00:00:00.123456789Z",
    "X-Audit-Row-Count": "1",
    "X-Audit-SHA256": "",
    "X-Audit-Format-Version": "1",
    "X-Audit-Export-ID": exportId,
    ...overrides,
  });
  return digest(body).then(hash => { if (!overrides["X-Audit-SHA256"]) headers.set("X-Audit-SHA256", hash); return new Response(body, { status: 200, headers }); });
}

describe("audit API", () => {
  it("sends an explicit brand, defaults paging, and serializes accepted filters", async () => {
    const fetcher = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ success: true, data: { items: [{ id: recordId, brand_id: brand, action: "view", actor_type: "admin", actor_id: actor, resource_type: "user", resource_id: "user-7", reason: "", request_id: "r1", created_at: "2026-10-01T01:00:00Z", ip_address: "", before_json: null, after_json: null }] } }), { status: 200 }));
    const api = createAuditApi(fetcher);
    const result = await api.list(brand, { from, to, action: "view", actor_id: actor, resource_type: "user", resource_id: actor, request_id: "r1" });
    expect(result.items).toHaveLength(1);
    const [url, init] = fetcher.mock.calls[0]!;
    const parsed = new URL(String(url), "http://localhost");
    expect(parsed.pathname).toBe("/api/v1/admin/audit");
    expect(parsed.searchParams.get("limit")).toBe("100");
    expect(parsed.searchParams.get("offset")).toBe("0");
    expect(parsed.searchParams.get("from")).toBe(from);
    expect(parsed.searchParams.get("action")).toBe("view");
    expect(parsed.searchParams.getAll("from")).toHaveLength(1);
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    expect(init?.credentials).toBe("same-origin");
  });

  it("permits an unbounded-time read only when both dates are omitted", async () => {
    const fetcher = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ success: true, data: { items: [] } }), { status: 200 }));
    await createAuditApi(fetcher).list(brand, {});
    const parsed = new URL(String(fetcher.mock.calls[0]![0]), "http://localhost");
    expect(parsed.searchParams.has("from")).toBe(false);
    expect(parsed.searchParams.has("to")).toBe(false);
  });

  it.each([
    ["one date", { from }],
    ["empty filter", { action: "" }],
    ["bad actor UUID", { actor_id: "not-a-uuid" }],
    ["unknown query", { unexpected: "x" }],
    ["text filter exceeds 128 UTF-8 bytes", { action: "é".repeat(65) }],
    ["unicode control in text filter", { request_id: "request\u0085id" }],
    ["empty pagination", { from, to, limit: 0 }],
    ["overlarge offset", { from, to, offset: 100001 }],
    ["overlong range", { from: "2026-10-01T00:00:00Z", to: "2026-11-02T00:00:00Z" }],
  ])("rejects invalid read query: %s", async (_label, query) => {
    const fetcher = vi.fn();
    await expect(createAuditApi(fetcher as typeof fetch).list(brand, query as AuditQuery)).rejects.toMatchObject({ code: "INVALID_INPUT" });
    expect(fetcher).not.toHaveBeenCalled();
  });

  it("rejects records returned for a different brand", async () => {
    const fetcher = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ success: true, data: { items: [{ id: recordId, brand_id: otherBrand, action: "view", actor_type: "admin", actor_id: actor, resource_type: "user", resource_id: "x", reason: "", request_id: "r", created_at: "2026-10-01T01:00:00Z", ip_address: "", before_json: null, after_json: null }] } }), { status: 200 }));
    await expect(createAuditApi(fetcher).list(brand, {})).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("requires brand_id while accepting null for platform audit records", async () => {
    const item = { id: recordId, action: "view", actor_type: "admin", actor_id: actor, resource_type: "user", resource_id: "x", reason: "", request_id: "r", created_at: "2026-10-01T01:00:00Z", ip_address: "", before_json: null, after_json: null };
    const missingBrand = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ success: true, data: { items: [item] } }), { status: 200 }));
    await expect(createAuditApi(missingBrand).list(brand, {})).rejects.toMatchObject({ code: "INVALID_RESPONSE" });

    const platformRecord = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ success: true, data: { items: [{ ...item, brand_id: null }] } }), { status: 200 }));
    await expect(createAuditApi(platformRecord).list(brand, {})).resolves.toEqual({ items: [{ ...item, brand_id: null }] });
  });

  it("validates full CSV, digest, metadata, row count, and brand before resolving", async () => {
    const fetcher = vi.fn<typeof fetch>(async () => await exportResponse());
    const result = await createAuditApi(fetcher).exportCsv(brand, { from, to });
    expect(result).toMatchObject({ brandId: brand, exportId, rowCount: 1, filename: `audit-${brand}-v1.csv` });
    expect(result.bytes).toEqual(bytes);
    const [url, init] = fetcher.mock.calls[0]!;
    const parsed = new URL(String(url), "http://localhost");
    expect(parsed.pathname).toBe("/api/v1/admin/audit/export");
    expect(parsed.searchParams.has("limit")).toBe(false);
    expect(parsed.searchParams.has("offset")).toBe(false);
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
  });

  it("rejects mismatched export brand, digest, and byte length", async () => {
    const wrongBrandFetch = vi.fn<typeof fetch>(async () => await exportResponse({ "X-Audit-Brand-ID": otherBrand }));
    await expect(createAuditApi(wrongBrandFetch).exportCsv(brand, { from, to })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const wrongDigestFetch = vi.fn<typeof fetch>(async () => await exportResponse({ "X-Audit-SHA256": "0".repeat(64) }));
    await expect(createAuditApi(wrongDigestFetch).exportCsv(brand, { from, to })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const wrongLengthFetch = vi.fn<typeof fetch>(async () => await exportResponse({}, bytes, bytes.length + 1));
    await expect(createAuditApi(wrongLengthFetch).exportCsv(brand, { from, to })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const wrongTypeFetch = vi.fn<typeof fetch>(async () => await exportResponse({ "Content-Type": "application/octet-stream" }));
    await expect(createAuditApi(wrongTypeFetch).exportCsv(brand, { from, to })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("surfaces the backend's bounded-export 422 without retrying", async () => {
    const fetcher = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ success: false, error: { code: "AUDIT_EXPORT_TOO_LARGE", message: "Export exceeds 4 MiB." } }), { status: 422 }));
    await expect(createAuditApi(fetcher).exportCsv(brand, { from, to })).rejects.toMatchObject({ status: 422, code: "AUDIT_EXPORT_TOO_LARGE" });
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it("rejects invalid row metadata and formula-risk cells even when the digest matches", async () => {
    const formulaCsv = `${columns.join(",")}\r\n${[recordId, brand, "=SUM(1,2)", "admin", actor, "user", "x", "reason", "req", "2026-10-01T12:00:00Z", "ip", "{}", "{}"].map(value => value.includes(",") ? `"${value}"` : value).join(",")}\r\n`;
    const formulaBytes = new TextEncoder().encode(`\uFEFF${formulaCsv}`);
    const fetcher = vi.fn<typeof fetch>(async () => await exportResponse({}, formulaBytes));
    await expect(createAuditApi(fetcher).exportCsv(brand, { from, to })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const tooManyFetch = vi.fn<typeof fetch>(async () => await exportResponse({ "X-Audit-Row-Count": "10001" }));
    await expect(createAuditApi(tooManyFetch).exportCsv(brand, { from, to })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("requires export dates and excludes mixed or wrong-brand grants", async () => {
    const brandViewer: AdminAccount = { id: actor, super_admin: false, brand_ids: [brand], permissions: [], permissions_by_brand: { [brand]: ["audit.view.brand"] } };
    const platformExportOnly: AdminAccount = { ...brandViewer, platform_permissions: ["audit.export.platform"] };
    expect(auditPermissions(brandViewer, brand)).toMatchObject({ view: true, export: false });
    expect(auditPermissions(platformExportOnly, brand)).toMatchObject({ view: true, export: false });
    expect(auditPermissions({ ...brandViewer, super_admin: true, permissions_by_brand: { [brand]: ["audit.view.brand", "audit.export.brand"] } }, brand)).toMatchObject({ view: false, export: false });
    expect(auditPermissions({ ...brandViewer, permissions_by_brand: { [otherBrand]: ["audit.view.brand", "audit.export.brand"] } }, brand)).toMatchObject({ view: false, export: false });
    const fetcher = vi.fn();
    await expect(createAuditApi(fetcher as typeof fetch).exportCsv(brand, {} as never)).rejects.toBeInstanceOf(AdminApiError);
    expect(fetcher).not.toHaveBeenCalled();
  });
});
