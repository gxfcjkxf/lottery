import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import {
  commissionPolicyPermissions,
  createCommissionPolicyApi,
  isValidCommissionPolicyConfig,
  type CommissionPolicy,
  type CommissionPolicyConfig,
} from "./commission-policy-api";

const brand = "11111111-1111-4111-8111-111111111111";
const otherBrand = "22222222-2222-4222-8222-222222222222";
const revisionId = "33333333-3333-4333-8333-333333333333";
const auditId = "44444444-4444-4444-8444-444444444444";
const adminId = "55555555-5555-4555-8555-555555555555";
const idempotencyKey = "66666666-6666-4666-8666-666666666666";
const timestamp = "2026-10-06T00:00:00Z";
const config: CommissionPolicyConfig = { enabled: false, calendar: null, payout_mode: "manual" };
const policy: CommissionPolicy = {
  brand_id: brand,
  version: 1,
  config,
  created_at: timestamp,
  updated_at: timestamp,
  revision_id: revisionId,
  audit_log_id: auditId,
};
const ok = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { status: 200 });

describe("commission policy API", () => {
  it("uses only mapped brand grants and denies platform actors", () => {
    const base = { id: adminId, super_admin: false, brand_ids: [brand], permissions: [] } satisfies AdminAccount;
    expect(commissionPolicyPermissions({ ...base, permissions_by_brand: { [brand]: ["commission_policy.view.brand", "commission_policy.write.brand"] } }, brand)).toEqual({ view: true, write: true });
    expect(commissionPolicyPermissions({ ...base, permissions_by_brand: { [otherBrand]: ["commission_policy.view.brand", "commission_policy.write.brand"] } }, brand)).toEqual({ view: false, write: false });
    expect(commissionPolicyPermissions({ ...base, super_admin: true, platform_permissions: ["commission_policy.view.platform", "commission_policy.write.brand"], permissions_by_brand: { [brand]: ["commission_policy.view.brand", "commission_policy.write.brand"] } }, brand)).toEqual({ view: false, write: false });
    expect(commissionPolicyPermissions({ ...base, platform_permissions: ["commission_policy.view.platform"] }, brand)).toEqual({ view: false, write: false });
    expect(commissionPolicyPermissions({ ...base, platform_permissions: ["commission_policy.view.platform"] }, "")).toEqual({ view: false, write: false });
    expect(commissionPolicyPermissions({ ...base, permissions: ["commission_policy.view.brand", "commission_policy.write.brand"] }, brand)).toEqual({ view: false, write: false });
  });

  it("accepts only closed, semantically valid calendar shapes", () => {
    expect(isValidCommissionPolicyConfig(config)).toBe(true);
    expect(isValidCommissionPolicyConfig({ enabled: true, calendar: null, payout_mode: "manual" })).toBe(false);
    expect(isValidCommissionPolicyConfig({ ...config, unexpected: true })).toBe(false);
    expect(isValidCommissionPolicyConfig({ ...config, calendar: { timezone: "Asia/Singapore", cycle: "weekly", boundary_time: "00:00:00", weekday: 7, month_day: null, short_month: "" } })).toBe(false);
    expect(isValidCommissionPolicyConfig({ ...config, calendar: { timezone: "Local", cycle: "monthly", boundary_time: "24:00:00", weekday: null, month_day: 31, short_month: "last_day" } })).toBe(false);
  });

  it("validates chronology, fractional precision, and the fourteen-hour offset bound", async () => {
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(ok({ ...policy, updated_at: "2026-10-05T23:00:00-14:00" }))
      .mockResolvedValueOnce(ok({ ...policy, updated_at: "2026-10-06T00:00:00+14:01" }))
      .mockResolvedValueOnce(ok({ ...policy, updated_at: "2026-10-06T00:00:00.1234567890Z" }))
      .mockResolvedValueOnce(ok({ ...policy, updated_at: "2026-10-05T23:59:59Z" }));
    const api = createCommissionPolicyApi(fetcher);
    await expect(api.getPolicy(brand)).resolves.toMatchObject({ updated_at: "2026-10-05T23:00:00-14:00" });
    for (let index = 0; index < 3; index++) await expect(api.getPolicy(brand)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("reads brand policy and history with same-origin credentials and explicit brand header", async () => {
    const revision = { id: revisionId, brand_id: brand, version: 1, config, changed_by: null, reason: "Initial policy", audit_log_id: null, created_at: timestamp };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok(policy)).mockResolvedValueOnce(ok({ items: [revision], limit: 20, offset: 0 }));
    const api = createCommissionPolicyApi(fetcher);
    await expect(api.getPolicy(brand)).resolves.toEqual(policy);
    await expect(api.getHistory(brand)).resolves.toEqual({ items: [revision], limit: 20, offset: 0 });
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
      "/api/v1/admin/commission-policy",
      "/api/v1/admin/commission-policy/history?limit=20&offset=0",
    ]);
    for (const [, init] of fetcher.mock.calls) {
      expect(init?.credentials).toBe("same-origin");
      expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    }
  });

  it.each([
    ["cross-brand response", { ...policy, brand_id: otherBrand }],
    ["noncanonical date", { ...policy, updated_at: "2026-02-30T00:00:00Z" }],
    ["missing post-initial audit id", { ...policy, version: 2, audit_log_id: undefined }],
    ["updated before created", { ...policy, created_at: "2026-10-07T00:00:00Z" }],
    ["noncanonical UUID", { ...policy, revision_id: "33333333333343338333333333333333" }],
    ["unsafe version", { ...policy, version: Number.MAX_SAFE_INTEGER + 1 }],
    ["open config", { ...policy, config: { ...config, side_effect: true } }],
  ])("rejects invalid GET policy (%s)", async (_name, value) => {
    const api = createCommissionPolicyApi(vi.fn<typeof fetch>().mockResolvedValue(ok(value)));
    await expect(api.getPolicy(brand)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("rejects cross-brand and malformed history rows", async () => {
    const crossBrand = { id: revisionId, brand_id: otherBrand, version: 1, config, changed_by: null, reason: "reason", audit_log_id: null, created_at: timestamp };
    const api = createCommissionPolicyApi(vi.fn<typeof fetch>().mockResolvedValue(ok({ items: [crossBrand], limit: 20, offset: 0 })));
    await expect(api.getHistory(brand)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("requires initial revisions to have null actor and audit ids, and later revisions to have both UUIDs", async () => {
    const initial = { id: revisionId, brand_id: brand, version: 1, config, changed_by: null, reason: "Initial policy", audit_log_id: null, created_at: timestamp };
    const later = { ...initial, version: 2, changed_by: adminId, audit_log_id: auditId };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok({ items: [initial], limit: 20, offset: 0 })).mockResolvedValueOnce(ok({ items: [later], limit: 20, offset: 0 })).mockResolvedValueOnce(ok({ items: [{ ...initial, changed_by: adminId }], limit: 20, offset: 0 })).mockResolvedValueOnce(ok({ items: [{ ...later, audit_log_id: null }], limit: 20, offset: 0 }));
    const api = createCommissionPolicyApi(fetcher);
    await expect(api.getHistory(brand)).resolves.toMatchObject({ items: [initial] });
    await expect(api.getHistory(brand)).resolves.toMatchObject({ items: [later] });
    await expect(api.getHistory(brand)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(api.getHistory(brand)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("binds the update response to the submitted brand, next version, and exact config", async () => {
    const submitted: CommissionPolicyConfig = {
      enabled: true,
      calendar: { timezone: "Asia/Singapore", cycle: "weekly", boundary_time: "06:30:00", weekday: 1, month_day: null, short_month: "" },
      payout_mode: "automatic",
    };
    const body = { version: 1, config: submitted, reason: "Reviewed" };
    const saved = { ...policy, version: 2, config: submitted };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(saved));
    await expect(createCommissionPolicyApi(fetcher).updatePolicy(brand, body, idempotencyKey)).resolves.toEqual(saved);
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(fetcher.mock.calls[0][1]?.method).toBe("PUT");
    expect(JSON.parse(String(fetcher.mock.calls[0][1]?.body))).toEqual(body);
    expect(new Headers(fetcher.mock.calls[0][1]?.headers).get("Idempotency-Key")).toBe(idempotencyKey);
  });

  it.each([
    ["wrong version", { ...policy, version: 3, config }],
    ["wrong config", { ...policy, version: 2, config: { ...config, payout_mode: "automatic" } }],
    ["wrong brand", { ...policy, brand_id: otherBrand, version: 2, audit_log_id: auditId }],
  ])("treats malformed successful update response as uncertain data (%s)", async (_name, value) => {
    const api = createCommissionPolicyApi(vi.fn<typeof fetch>().mockResolvedValue(ok(value)));
    await expect(api.updatePolicy(brand, { version: 1, config, reason: "Reviewed" }, idempotencyKey))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it.each([400, 403, 409, 503])("does not autonomously replay failure status %i", async (status) => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: false, error: { message: "failed" } }), { status }));
    const api = createCommissionPolicyApi(fetcher);
    await expect(api.updatePolicy(brand, { version: 1, config, reason: "Reviewed" }, idempotencyKey)).rejects.toBeInstanceOf(AdminApiError);
    expect(fetcher).toHaveBeenCalledTimes(1);
  });
});
