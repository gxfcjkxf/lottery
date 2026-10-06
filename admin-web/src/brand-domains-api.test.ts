import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { brandDomainsPermissions, createBrandDomainsApi, isCanonicalBrandDomain, type BrandDomainsRecord } from "./brand-domains-api";

const brand = "00000000-0000-4000-8000-000000000001", other = "00000000-0000-4000-8000-000000000002";
const actor = "00000000-0000-4000-8000-000000000003", audit = "00000000-0000-4000-8000-000000000004";
const at = "2026-10-06T04:00:00Z";
const domain1 = { id: actor, domain: "legacy.localhost", enabled: true, is_primary: true };
function record(overrides: Partial<BrandDomainsRecord> = {}): BrandDomainsRecord { return { brand_id: brand, version: 8, status: "active", domains: [domain1], updated_at: at, ...overrides }; }
function response(data: unknown, status = 200): Response { return new Response(JSON.stringify({ success: true, data }), { status }); }
function account(overrides: Partial<AdminAccount> = {}): AdminAccount { return { id: actor, super_admin: false, brand_ids: [brand], permissions: [], ...overrides }; }

describe("brand domain validation and permissions", () => {
  it("accepts canonical lowercase dotted DNS hostnames and rejects non-hosts", () => {
    for (const host of ["shop.example.com", "xn--bcher-kva.example", `${"a".repeat(63)}.example.com`]) expect(isCanonicalBrandDomain(host)).toBe(true);
    for (const host of ["localhost", "x.localhost", "x.local", "x.internal", "192.0.2.1", "127.1", "2001:db8::1", "*.example.com", "https://example.com", "UPPER.example", "example.com.", "a..example.com", "-a.example.com", "a-.example.com", "a.123", "a.0x7f", `${"a".repeat(64)}.example.com`, `${"a".repeat(250)}.com`]) expect(isCanonicalBrandDomain(host)).toBe(false);
  });
  it("requires explicit scoped grants and has no super-admin shortcut", () => {
    expect(brandDomainsPermissions(account({ permissions: ["brand_domains.view.brand", "brand_domains.write.brand"] }), brand)).toEqual({ view: true, write: true });
    expect(brandDomainsPermissions(account({ super_admin: true }), brand)).toEqual({ view: false, write: false });
    expect(brandDomainsPermissions(account({ brand_ids: [], permissions: ["brand_domains.write.brand"] }), brand).write).toBe(false);
    expect(brandDomainsPermissions(account({ platform_permissions: ["brand_domains.view.platform"] }), brand).view).toBe(true);
    expect(brandDomainsPermissions(account({ permissions: ["brand_domains.view.platform"] }), brand).view).toBe(false);
  });
});

describe("brand domains API", () => {
  it("reads exact record and supports legacy hostnames", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response(record()));
    await expect(createBrandDomainsApi(fetcher).get(brand)).resolves.toEqual(record());
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe("/api/v1/admin/brand-domains"); expect(init?.credentials).toBe("same-origin");
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    expect(new Headers(init?.headers).has("X-Admin-Brand-ID")).toBe(false);
  });
  it("posts a new binding and verifies the required 201 receipt", async () => {
    const body = { version: 8, domain: "shop.example.com", enabled: true, is_primary: true, reason: "Launch storefront" };
    const created = { id: other, domain: body.domain, enabled: true, is_primary: true };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response({ ...record({ version: 9, domains: [created], audit_log_id: audit }) }, 201));
    await expect(createBrandDomainsApi(fetcher).post(brand, body, "domain-key-0001")).resolves.toEqual({ ...record({ version: 9, domains: [created], audit_log_id: audit }) });
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe("/api/v1/admin/brand-domains"); expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual(body);
    expect(new Headers(init?.headers).get("Idempotency-Key")).toBe("domain-key-0001");
  });
  it("patches by binding id without sending a replacement domain and accepts legacy local/IP values", async () => {
    const body = { version: 8, enabled: false, is_primary: false, reason: "Retire legacy host" };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response({ ...record({ version: 9, domains: [{ ...domain1, enabled: false, is_primary: false }], audit_log_id: audit }) }));
    await createBrandDomainsApi(fetcher).patch(brand, actor, body, "domain-key-0002");
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe(`/api/v1/admin/brand-domains/${actor}`); expect(init?.method).toBe("PATCH");
    expect(JSON.parse(String(init?.body))).toEqual(body);
    expect(JSON.parse(String(init?.body))).not.toHaveProperty("domain");
  });
  it("requires exact receipt scope, version, target state, audit id, and method status", async () => {
    const body = { version: 8, enabled: false, is_primary: false, reason: "Retire legacy host" };
    const good = { ...record({ version: 9, domains: [{ ...domain1, enabled: false, is_primary: false }], audit_log_id: audit }) };
    for (const bad of [
      { ...good, brand_id: other }, { ...good, version: 10 }, { ...good, audit_log_id: undefined },
      { ...good, domains: [{ ...domain1, enabled: true, is_primary: true }] },
    ]) await expect(createBrandDomainsApi(vi.fn<typeof fetch>().mockResolvedValue(response(bad))).patch(brand, actor, body, "domain-key-0003")).rejects.toMatchObject({ status: 0, code: "INVALID_RESPONSE" });
    await expect(createBrandDomainsApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...good, audit_log_id: audit }, 201))).patch(brand, actor, body, "domain-key-0003")).rejects.toMatchObject({ status: 0, code: "INVALID_RESPONSE" });
  });
  it("validates history snapshots and passes pagination", async () => {
    const item = { id: other, brand_id: brand, version: 8, changed_by: actor, reason: "Launch", audit_log_id: audit, created_at: at, before_domains: [], domains: [domain1] };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response({ items: [item], limit: 20, offset: 40 }));
    await expect(createBrandDomainsApi(fetcher).history(brand, 20, 40)).resolves.toEqual({ items: [item], limit: 20, offset: 40 });
    expect(fetcher.mock.calls[0][0]).toBe("/api/v1/admin/brand-domains/history?limit=20&offset=40");
    await expect(createBrandDomainsApi(vi.fn<typeof fetch>().mockResolvedValue(response({ items: [{ ...item, brand_id: other }], limit: 20, offset: 0 }))).history(brand)).rejects.toMatchObject({ status: 502 });
  });
  it("preserves server conflicts and treats network failures as unknown", async () => {
    const body = { version: 8, enabled: false, is_primary: false, reason: "Retire legacy host" };
    const conflict = new Response(JSON.stringify({ success: false, error: { code: "VERSION_CONFLICT", message: "Reload" } }), { status: 409 });
    await expect(createBrandDomainsApi(vi.fn<typeof fetch>().mockResolvedValue(conflict)).patch(brand, actor, body, "domain-key-0004")).rejects.toMatchObject({ status: 409, code: "VERSION_CONFLICT" });
    await expect(createBrandDomainsApi(vi.fn<typeof fetch>().mockRejectedValue(new Error("offline"))).patch(brand, actor, body, "domain-key-0004")).rejects.toMatchObject({ status: 0, code: "NETWORK_ERROR" });
    await expect(createBrandDomainsApi(vi.fn<typeof fetch>()).post(brand, { ...body, domain: "bad" } as never, "domain-key-0004")).rejects.toBeInstanceOf(AdminApiError);
  });
  it("accepts only server-compatible idempotency key characters", async () => {
    const body = { version: 8, enabled: false, is_primary: false, reason: "Retire legacy host" };
    for (const key of ["domain/key-0001", "domain key-0001", "domain$key-0001", "domain\nkey-0001"]) {
      const fetcher = vi.fn<typeof fetch>();
      await expect(createBrandDomainsApi(fetcher).patch(brand, actor, body, key)).rejects.toMatchObject({ status: 400, code: "BRAND_DOMAIN_INPUT_INVALID" });
      expect(fetcher).not.toHaveBeenCalled();
    }
  });
  it("rejects extra keys on both write shapes instead of silently stripping them", async () => {
    const fetcher = vi.fn<typeof fetch>();
    const patchBody = { version: 8, enabled: false, is_primary: false, reason: "Retire legacy host", domain: "rename.example.com" };
    const postBody = { version: 8, domain: "shop.example.com", enabled: false, is_primary: false, reason: "Add disabled", target: actor };
    await expect(createBrandDomainsApi(fetcher).patch(brand, actor, patchBody as never, "domain-key-0005")).rejects.toMatchObject({ status: 400, code: "BRAND_DOMAIN_INPUT_INVALID" });
    await expect(createBrandDomainsApi(fetcher).post(brand, postBody as never, "domain-key-0006")).rejects.toMatchObject({ status: 400, code: "BRAND_DOMAIN_INPUT_INVALID" });
    expect(fetcher).not.toHaveBeenCalled();
  });
});
