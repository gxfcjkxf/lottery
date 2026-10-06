import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { brandCreationPermission, createBrandCreationApi, type BrandCreationBody } from "./brand-creation-api";

const actor = "00000000-0000-4000-8000-000000000001", id = "00000000-0000-4000-8000-000000000002", audit = "00000000-0000-4000-8000-000000000003";
const body: BrandCreationBody = { code: "new_brand", name: "新品牌", default_locale: "en", timezone: "Asia/Manila", reason: "Initial platform setup" };
const receipt = { id, code: body.code, name: body.name, status: "paused", default_locale: body.default_locale, timezone: body.timezone, version: 1, created_at: "2026-10-07T04:00:00Z", audit_log_id: audit };
const response = (data: unknown, status = 201) => new Response(JSON.stringify({ success: true, data }), { status });
function account(overrides: Partial<AdminAccount> = {}): AdminAccount { return { id: actor, super_admin: false, brand_ids: [], permissions: [], ...overrides }; }

describe("brand creation permission", () => {
  it("requires only the exact platform permission and rejects every fallback", () => {
    expect(brandCreationPermission(account({ platform_permissions: ["brand.create.platform"] }))).toBe(true);
    for (const value of [
      account({ super_admin: true }),
      account({ super_admin: true, permissions: ["brand.create.platform"], platform_permissions: [] }),
      account({ permissions: ["brand.create.platform"] }),
      account({ brand_ids: [id], permissions_by_brand: { [id]: ["brand.create.brand", "brand.create.platform"] } }),
      account({ platform_permissions: ["brand.create.brand"] }),
      account({ platform_permissions: ["brand.create.platform.extra"] }),
    ]) expect(brandCreationPermission(value)).toBe(false);
  });
});

describe("brand creation API", () => {
  it("posts the exact global JSON contract with idempotency and no brand header", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response(receipt));
    await expect(createBrandCreationApi(fetcher).create(body, "creation-key-0001")).resolves.toEqual(receipt);
    const [url, init] = fetcher.mock.calls[0]; const headers = new Headers(init?.headers);
    expect(url).toBe("/api/v1/admin/brands"); expect(init?.method).toBe("POST"); expect(init?.credentials).toBe("same-origin");
    expect(headers.get("Idempotency-Key")).toBe("creation-key-0001"); expect(headers.has("X-Brand-ID")).toBe(false);
    expect(JSON.parse(String(init?.body))).toEqual(body);
  });
  it("validates names and reasons by UTF-8 bytes, code case, locale, and IANA zone", async () => {
    const api = createBrandCreationApi(vi.fn<typeof fetch>().mockRejectedValue(new Error("offline")));
    for (const bad of [ { ...body, code: "Mixed" }, { ...body, name: "字".repeat(41) }, { ...body, name: " 新品牌" }, { ...body, name: "  " }, { ...body, reason: "界".repeat(167) }, { ...body, reason: " Initial platform setup" }, { ...body, reason: " " }, { ...body, default_locale: "fr" }, { ...body, timezone: "Not/AZone" }, { ...body, timezone: "Local" }, { ...body, timezone: "" }, { ...body, timezone: "x".repeat(81) }, { ...body, extra: true } ])
      await expect(api.create(bad as BrandCreationBody, "creation-key-0001")).rejects.toMatchObject({ status: 400, code: "BRAND_CREATE_INPUT_INVALID" });
    for (const zone of ["UTC", "Etc/GMT+5"]) await expect(api.create({ ...body, timezone: zone }, "creation-key-0001")).rejects.toMatchObject({ code: "NETWORK_ERROR" });
  });
  it("rejects every Unicode Cc control character and Unicode line separator in both text fields", async () => {
    const api = createBrandCreationApi(vi.fn<typeof fetch>());
    const controls = [...Array.from({ length: 0x20 }, (_, n) => String.fromCodePoint(n)), ...Array.from({ length: 0x21 }, (_, n) => String.fromCodePoint(0x7f + n)), "\u2028", "\u2029"];
    for (const control of controls) {
      await expect(api.create({ ...body, name: `Name${control}value` }, "creation-key-0001")).rejects.toMatchObject({ status: 400, code: "BRAND_CREATE_INPUT_INVALID" });
      await expect(api.create({ ...body, reason: `Reason${control}value` }, "creation-key-0001")).rejects.toMatchObject({ status: 400, code: "BRAND_CREATE_INPUT_INVALID" });
    }
  });
  it("treats malformed, non-201, or mismatched receipts as uncertain failures", async () => {
    for (const [data, status] of [
      [{ ...receipt, code: "other" }, 201], [{ ...receipt, name: "other" }, 201], [{ ...receipt, default_locale: "zh-CN" }, 201],
      [{ ...receipt, timezone: "UTC" }, 201], [{ ...receipt, status: "active" }, 201], [{ ...receipt, version: 2 }, 201],
      [{ ...receipt, id: "bad" }, 201], [{ ...receipt, audit_log_id: "bad" }, 201], [{ ...receipt, created_at: "2026-02-30T04:00:00Z" }, 201],
      [{ ...receipt, unexpected: true }, 201], [Object.fromEntries(Object.entries(receipt).filter(([key]) => key !== "timezone")), 201], [receipt, 200], [null, 201],
    ] as const)
      await expect(createBrandCreationApi(vi.fn<typeof fetch>().mockResolvedValue(response(data, status))).create(body, "creation-key-0001"))
        .rejects.toMatchObject({ status: 0, code: "INVALID_RESPONSE" });
  });
  it("preserves auth and business errors and marks transport/server failures", async () => {
    const fail = (status: number, code: string) => new Response(JSON.stringify({ success: false, error: { code, message: code } }), { status });
    for (const [status, code] of [[401, "AUTH"], [400, "BRAND_CREATE_INPUT_INVALID"], [403, "PERMISSION_DENIED"], [409, "BRAND_CODE_CONFLICT"], [409, "IDEMPOTENCY_CONFLICT"], [503, "TEMPORARY_FAILURE"]] as const)
      await expect(createBrandCreationApi(vi.fn<typeof fetch>().mockResolvedValue(fail(status, code))).create(body, "creation-key-0001"))
        .rejects.toMatchObject({ status, code });
    await expect(createBrandCreationApi(vi.fn<typeof fetch>().mockRejectedValue(new Error("offline"))).create(body, "creation-key-0001"))
      .rejects.toBeInstanceOf(AdminApiError);
  });
});
