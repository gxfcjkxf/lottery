import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import {
  brandOperationPermissions,
  createBrandOperationApi,
  type BrandOperation,
  type BrandOperationHistoryItem,
} from "./brand-operation-api";

const brand = "00000000-0000-4000-8000-000000000001";
const otherBrand = "00000000-0000-4000-8000-000000000002";
const actor = "00000000-0000-4000-8000-000000000003";
const audit = "00000000-0000-4000-8000-000000000004";
const at = "2026-10-06T04:00:00Z";
const intent = { version: 4, status: "paused" as const, reason: "planned maintenance" };

function record(overrides: Partial<BrandOperation> = {}): BrandOperation {
  return { brand_id: brand, version: 5, name: "Aurora", status: "paused", updated_at: at, ...overrides };
}

function revision(overrides: Partial<BrandOperationHistoryItem> = {}): BrandOperationHistoryItem {
  return { id: actor, brand_id: brand, version: 5, previous_status: "active", status: "paused", changed_by: actor, reason: intent.reason, audit_log_id: audit, created_at: at, ...overrides };
}

function response(data: unknown, status = 200): Response {
  return new Response(JSON.stringify({ success: true, data }), { status, headers: { "Content-Type": "application/json" } });
}

function account(overrides: Partial<AdminAccount> = {}): AdminAccount {
  return { id: actor, super_admin: false, brand_ids: [brand], permissions: [], ...overrides };
}

describe("brand operation permissions", () => {
  it("requires mapped brand grants and denies flat or platform grants", () => {
    expect(brandOperationPermissions(account({ permissions_by_brand: { [brand]: ["brand_operation.view.brand", "brand_operation.write.brand"] } }), brand))
      .toEqual({ view: true, write: true });
    expect(brandOperationPermissions(account({ brand_ids: [], permissions: ["brand_operation.view.brand", "brand_operation.write.brand"] }), brand))
      .toEqual({ view: false, write: false });
    expect(brandOperationPermissions(account({ super_admin: true }), brand)).toEqual({ view: false, write: false });
    expect(brandOperationPermissions(account({ super_admin: true, platform_permissions: ["brand_operation.view.platform", "brand_operation.write.platform"], permissions_by_brand: { [brand]: ["brand_operation.view.brand", "brand_operation.write.brand"] } }), brand))
      .toEqual({ view: false, write: false });
    expect(brandOperationPermissions(account({ permissions: ["brand_operation.view.platform"] }), brand).view).toBe(false);
    expect(brandOperationPermissions(account({ permissions: ["brand_operation.view.brand"], permissions_by_brand: {} }), brand).view).toBe(false);
    expect(brandOperationPermissions(account({ permissions: ["brand_operation.view.brand", "brand_operation.write.brand"] }), brand)).toEqual({ view: false, write: false });
  });
});

describe("brand operation API", () => {
  it("reads the exact current record with same-origin credentials and X-Brand-ID", async () => {
    const fetchImpl = vi.fn<typeof fetch>().mockResolvedValueOnce(response(record())).mockResolvedValueOnce(response(record({ audit_log_id: audit })));
    const api = createBrandOperationApi(fetchImpl);
    await expect(api.get(brand)).resolves.toEqual(record());
    await expect(api.get(brand)).resolves.toEqual(record({ audit_log_id: audit }));
    const [url, init] = fetchImpl.mock.calls[0];
    expect(url).toBe("/api/v1/admin/brand-operation");
    expect(init?.method).toBe("GET");
    expect(init?.credentials).toBe("same-origin");
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
  });

  it("sends a frozen-intent patch with JSON, credentials, and idempotency, and verifies its receipt", async () => {
    const fetchImpl = vi.fn<typeof fetch>().mockResolvedValue(response({ ...record(), audit_log_id: audit }));
    await expect(createBrandOperationApi(fetchImpl).patch(brand, intent, "stable-key")).resolves.toEqual({ ...record(), audit_log_id: audit });
    const [url, init] = fetchImpl.mock.calls[0];
    expect(url).toBe("/api/v1/admin/brand-operation");
    expect(init?.method).toBe("PATCH");
    expect(init?.credentials).toBe("same-origin");
    const headers = new Headers(init?.headers);
    expect(headers.get("X-Brand-ID")).toBe(brand);
    expect(headers.get("Idempotency-Key")).toBe("stable-key");
    expect(headers.get("Content-Type")).toBe("application/json");
    expect(JSON.parse(String(init?.body))).toEqual(intent);
  });

  it("rejects malformed or mismatched 200 receipts as uncertain, and keeps exact response shapes", async () => {
    for (const bad of [
      { ...record(), audit_log_id: audit, brand_id: otherBrand },
      { ...record(), audit_log_id: audit, version: 6 },
      { ...record(), audit_log_id: audit, status: "active" },
      { ...record(), audit_log_id: "not-a-uuid" },
      { ...record(), audit_log_id: audit, ignored: true },
    ]) {
      await expect(createBrandOperationApi(vi.fn<typeof fetch>().mockResolvedValue(response(bad)))
        .patch(brand, intent, "stable-key"))
        .rejects.toMatchObject({ status: 0, code: "INVALID_RESPONSE" });
    }
    await expect(createBrandOperationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...record(), extra: null }))).get(brand))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(createBrandOperationApi(vi.fn<typeof fetch>().mockRejectedValue(new Error("offline"))).patch(brand, intent, "stable-key"))
      .rejects.toMatchObject({ status: 0, code: "NETWORK_ERROR" });
  });

  it("validates history pages and sends the requested limit and offset", async () => {
    const fetchImpl = vi.fn<typeof fetch>().mockResolvedValue(response({ items: [revision()], limit: 20, offset: 40 }));
    await expect(createBrandOperationApi(fetchImpl).history(brand, 20, 40))
      .resolves.toEqual({ items: [revision()], limit: 20, offset: 40 });
    const [url, init] = fetchImpl.mock.calls[0];
    expect(url).toBe("/api/v1/admin/brand-operation/history?limit=20&offset=40");
    expect(init?.credentials).toBe("same-origin");
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    for (const invalid of [
      { items: [{ ...revision(), surprise: 1 }], limit: 20, offset: 0 },
      { items: [revision({ brand_id: otherBrand })], limit: 20, offset: 0 },
      { items: [revision({ created_at: "2026-10-06 04:00:00" })], limit: 20, offset: 0 },
      { items: [revision({ status: "disabled" })], limit: 20, offset: 0 },
      { items: [], limit: 19, offset: 0 },
    ]) {
      await expect(createBrandOperationApi(vi.fn<typeof fetch>().mockResolvedValue(response(invalid))).history(brand))
        .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
    await expect(createBrandOperationApi(vi.fn<typeof fetch>()).history(brand, 101, 0)).rejects.toBeInstanceOf(AdminApiError);
    await expect(createBrandOperationApi(vi.fn<typeof fetch>()).patch(brand, intent, "bad-key"))
      .rejects.toMatchObject({ status: 0, code: "INVALID_INPUT" });
  });

  it("preserves known version conflicts as definitive server errors", async () => {
    await expect(createBrandOperationApi(vi.fn<typeof fetch>().mockResolvedValue(
      new Response(JSON.stringify({ success: false, error: { code: "CONFLICT", message: "version changed" } }), { status: 409 }),
    )).patch(brand, intent, "stable-key")).rejects.toMatchObject({ status: 409, code: "CONFLICT" });
  });
});
