import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { brandPresentationPermissions, createBrandPresentationApi, validBrandPresentationConfig, type BrandPresentationConfig, type BrandPresentationRecord, type EffectiveBrandPresentationConfig } from "./brand-presentation-api";

const brand = "00000000-0000-4000-8000-000000000001";
const other = "00000000-0000-4000-8000-000000000002";
const actor = "00000000-0000-4000-8000-000000000003";
const audit = "00000000-0000-4000-8000-000000000004";
const config: BrandPresentationConfig = {
  display_name: null, logo_text: "Aurora", logo_url: "/brand-assets/logo.png", favicon_url: null,
  primary_color: "#112233", accent_color: null, success_color: null, warning_color: null, danger_color: null,
  font_family: "system", font_scale: null, radius: "soft", shadow: null, default_locale: "en",
  available_locales: ["en", "zh-CN"], content: { en: { tagline: null, announcement: "Hello" }, "zh-CN": { tagline: "你好", announcement: null } },
};
const effective: EffectiveBrandPresentationConfig = {
  display_name: "Aurora", logo_text: "Aurora", logo_url: "/brand-assets/logo.png", favicon_url: null,
  primary_color: "#112233", accent_color: "#445566", success_color: "#008800", warning_color: "#ffaa00", danger_color: "#cc0000",
  font_family: "system", font_scale: "standard", radius: "soft", shadow: "subtle", default_locale: "en", available_locales: ["en", "zh-CN"],
  content: { en: { tagline: "Aurora", announcement: "Hello" }, "zh-CN": { tagline: "你好", announcement: "公告" } },
};
const at = "2026-10-06T04:00:00.123456Z";
function record(overrides: Partial<BrandPresentationRecord> = {}): BrandPresentationRecord {
  return { brand_id: brand, version: 4, status: "active", base_name: "Aurora", config, effective, updated_at: at, ...overrides };
}
function response(data: unknown, status = 200): Response { return new Response(JSON.stringify({ success: true, data }), { status, headers: { "Content-Type": "application/json" } }); }
function account(overrides: Partial<AdminAccount> = {}): AdminAccount { return { id: actor, super_admin: false, brand_ids: [brand], permissions: [], ...overrides }; }

describe("brand presentation permissions", () => {
  it("requires mapped grants for brand members and denies platform actors", () => {
    expect(brandPresentationPermissions(account({ permissions_by_brand: { [brand]: ["brand_presentation.view.brand", "brand_presentation.write.brand"] } }), brand)).toEqual({ view: true, write: true });
    expect(brandPresentationPermissions(account({ brand_ids: [], permissions_by_brand: { [brand]: ["brand_presentation.view.brand", "brand_presentation.write.brand"] } }), brand)).toEqual({ view: false, write: false });
    expect(brandPresentationPermissions(account({ super_admin: true }), brand)).toEqual({ view: false, write: false });
    expect(brandPresentationPermissions(account({ super_admin: true, platform_permissions: ["brand_presentation.view.platform", "brand_presentation.write.platform"], permissions_by_brand: { [brand]: ["brand_presentation.view.brand", "brand_presentation.write.brand"] } }), brand)).toEqual({ view: false, write: false });
    expect(brandPresentationPermissions(account({ permissions_by_brand: { [other]: ["brand_presentation.view.brand"] } }), brand).view).toBe(false);
    expect(brandPresentationPermissions(account({ permissions: ["brand_presentation.view.brand", "brand_presentation.write.brand"] }), brand)).toEqual({ view: false, write: false });
  });
});

describe("brand presentation API", () => {
  it("reads and validates config/effective records and optional audit IDs", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(response(record())).mockResolvedValueOnce(response(record({ audit_log_id: audit })));
    const api = createBrandPresentationApi(fetcher);
    await expect(api.get(brand)).resolves.toEqual(record());
    await expect(api.get(brand)).resolves.toEqual(record({ audit_log_id: audit }));
    expect(fetcher.mock.calls[0][0]).toBe("/api/v1/admin/brand-presentation");
    expect(new Headers(fetcher.mock.calls[0][1]?.headers).get("X-Brand-ID")).toBe(brand);
    for (const bad of [
      record({ brand_id: other }),
      record({ config: { ...config, font_family: "comic" } as unknown as BrandPresentationConfig }),
      record({ config: { ...config, primary_color: "red" } as unknown as BrandPresentationConfig }),
      record({ config: { ...config, available_locales: ["en", "en"] } as unknown as BrandPresentationConfig }),
      record({ config: { ...config, ignored: true } as unknown as BrandPresentationConfig }),
      record({ effective: { ...effective, primary_color: null } as unknown as typeof effective }),
      record({ updated_at: "2026-02-30T04:00:00Z" }),
      record({ audit_log_id: "bad" }),
    ]) await expect(createBrandPresentationApi(vi.fn<typeof fetch>().mockResolvedValue(response(bad))).get(brand)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("accepts only permitted asset URIs and validates the exact put receipt", async () => {
    const body = { version: 4, config, reason: "Brand refresh" };
    const key = "brand-presentation-key-01";
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(response({ ...record({ version: 5, audit_log_id: audit }) }))
      .mockResolvedValueOnce(response({ ...record({ version: 5, audit_log_id: audit }) }));
    const api = createBrandPresentationApi(fetcher);
    await expect(api.put(brand, body, key)).resolves.toEqual({ ...record({ version: 5, audit_log_id: audit }) });
    await expect(api.put(brand, body, key)).resolves.toEqual({ ...record({ version: 5, audit_log_id: audit }) });
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe("/api/v1/admin/brand-presentation");
    expect(init?.method).toBe("PUT");
    expect(init?.credentials).toBe("same-origin");
    expect(JSON.parse(String(init?.body))).toEqual(body);
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    expect(new Headers(init?.headers).get("Idempotency-Key")).toBe(key);
    expect(fetcher.mock.calls.map(([, call]) => call?.body)).toEqual([JSON.stringify(body), JSON.stringify(body)]);
    expect(fetcher.mock.calls.map(([, call]) => new Headers(call?.headers).get("Idempotency-Key"))).toEqual([key, key]);

    for (const logo_url of ["data:image/png,x", "javascript:alert(1)", "http://example.com/logo.png", "https://user:pass@example.com/logo.png", "https://localhost/logo.png", "https://127.0.0.1/logo.png", "https://127.1/logo.png", "https://0177.0.0.1/logo.png", "https://2130706433/logo.png", "https://0x7f000001/logo.png", "https://example.123/logo.png", "https://example.com:443/logo.png", "https://example.com/a.png?x=1", "https://example.com/a.png#frag", "https://example.com/a.svg", "HTTPS://example.com/a.png", "https://example.com/a b.png", "https://example.com/a\tb.png", "https://example.com/a%2epng", "/brand-assets/my logo.png", "/brand-assets/%2e%2e/private.png", "/icons/../secret.png", "//example.com/logo.png", "/uploads/logo.png", "/brand-assets\\evil.png"]) {
      const badConfig = { ...config, logo_url } as unknown as BrandPresentationConfig;
      await expect(createBrandPresentationApi(vi.fn<typeof fetch>()).put(brand, { ...body, config: badConfig }, key)).rejects.toMatchObject({ status: 400, code: "INVALID_INPUT" });
    }
    for (const receipt of [
      { ...record({ version: 5, audit_log_id: audit }), config: { ...config, logo_text: "different" } },
      { ...record({ version: 6, audit_log_id: audit }) },
      { ...record({ version: 5, audit_log_id: undefined }), audit_log_id: undefined },
      { ...record({ version: 5, audit_log_id: "bad" }) },
      { ...record({ version: 5, audit_log_id: audit }), effective: { ...effective, font_family: "sans" } },
    ]) await expect(createBrandPresentationApi(vi.fn<typeof fetch>().mockResolvedValue(response(receipt))).put(brand, body, key)).rejects.toMatchObject({ status: 0, code: "INVALID_RESPONSE" });
    await expect(createBrandPresentationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...record({ version: 5, audit_log_id: audit }) }, 201))).put(brand, body, key)).rejects.toMatchObject({ status: 0, code: "INVALID_RESPONSE" });
    const conflictResponse = new Response(JSON.stringify({ success: false, error: { code: "CONFLICT" } }), { status: 409 });
    await expect(createBrandPresentationApi(vi.fn<typeof fetch>().mockResolvedValue(conflictResponse)).put(brand, body, key)).rejects.toMatchObject({ status: 409, code: "CONFLICT" });
    await expect(createBrandPresentationApi(vi.fn<typeof fetch>().mockRejectedValue(new Error("offline"))).put(brand, body, key)).rejects.toMatchObject({ status: 0, code: "NETWORK_ERROR" });
    await expect(createBrandPresentationApi(vi.fn<typeof fetch>()).put(brand, { ...body, reason: "   " }, key)).rejects.toMatchObject({ status: 400, code: "INVALID_INPUT" });
    await expect(createBrandPresentationApi(vi.fn<typeof fetch>()).put(brand, body, "short")).rejects.toMatchObject({ status: 400, code: "INVALID_INPUT" });
    await expect(createBrandPresentationApi(vi.fn<typeof fetch>()).put(brand, body, "bad!key!!")).rejects.toMatchObject({ status: 400, code: "INVALID_INPUT" });
  });

  it("rejects effective values that contradict explicit config overrides", async () => {
    const inconsistent = [
      record({ effective: { ...effective, logo_text: "Changed" } }),
      record({ effective: { ...effective, available_locales: ["zh-CN", "en"] } }),
      record({ effective: { ...effective, content: { ...effective.content, en: { ...effective.content.en, announcement: "Changed" } } } }),
      record({ config: { ...config, default_locale: null, available_locales: null }, effective: { ...effective, default_locale: "zh-CN", available_locales: ["en"] } }),
    ];
    for (const value of inconsistent) {
      await expect(createBrandPresentationApi(vi.fn<typeof fetch>().mockResolvedValue(response(value))).get(brand))
        .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
    await expect(createBrandPresentationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...inconsistent[0], version: 5, audit_log_id: audit }))).put(brand, { version: 4, config, reason: "Brand refresh" }, "brand-presentation-key-01"))
      .rejects.toMatchObject({ status: 0, code: "INVALID_RESPONSE" });
  });

  it("exports server-matching UTF-8 and locale validation", () => {
    expect(validBrandPresentationConfig(config)).toBe(true);
    expect(validBrandPresentationConfig({ ...config, display_name: "   " })).toBe(false);
    expect(validBrandPresentationConfig({ ...config, display_name: "x".repeat(81) })).toBe(false);
    expect(validBrandPresentationConfig({ ...config, logo_text: "标志".repeat(11) })).toBe(false);
    expect(validBrandPresentationConfig({ ...config, available_locales: [] })).toBe(false);
    expect(validBrandPresentationConfig({ ...config, available_locales: ["zh-CN"], default_locale: null })).toBe(false);
    expect(validBrandPresentationConfig({ ...config, content: { ...config.content!, en: { tagline: "x".repeat(161), announcement: null } } })).toBe(false);
    expect(validBrandPresentationConfig({ ...config, content: { ...config.content!, en: { tagline: null, announcement: "x".repeat(2001) } } })).toBe(false);
    expect(validBrandPresentationConfig({ ...config, logo_url: `/icons/${"a".repeat(505)}.png` })).toBe(false);
    expect(validBrandPresentationConfig({ ...config, logo_url: "https://brand.example/logo.webp" })).toBe(true);
  });

  it("validates history page scope, shape, and timestamp", async () => {
    const revision = { id: actor, brand_id: brand, version: 4, config, effective, changed_by: actor, reason: "Brand refresh", audit_log_id: audit, created_at: at };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response({ items: [revision], limit: 20, offset: 40 }));
    await expect(createBrandPresentationApi(fetcher).history(brand, 20, 40)).resolves.toEqual({ items: [revision], limit: 20, offset: 40 });
    expect(fetcher.mock.calls[0][0]).toBe(`${"/api/v1/admin/brand-presentation"}/history?limit=20&offset=40`);
    await expect(createBrandPresentationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ items: [{ ...revision, brand_id: other }], limit: 20, offset: 0 }))).history(brand)).rejects.toMatchObject({ status: 502 });
    await expect(createBrandPresentationApi(vi.fn<typeof fetch>()).history(brand, 101, 0)).rejects.toBeInstanceOf(AdminApiError);
  });
});
