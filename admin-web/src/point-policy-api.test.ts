import { describe, expect, it, vi } from "vitest";
import {
  canViewPointPolicy,
  canWritePointPolicy,
  createPointPolicyApi,
  createPointPolicyKeyTracker,
  effectiveBrandPermissions,
  isCanonicalPositiveLimit,
  type PointPolicy,
} from "./point-policy-api";

const policy: PointPolicy = {
  brand_id: "brand-1",
  version: 7,
  max_balance_points: null,
  max_recharge_points: "10000",
  max_adjustment_points: "500",
  audit_log_id: "audit-1",
};
const ok = (data: unknown) =>
  new Response(JSON.stringify({ success: true, data }), { status: 200 });
const fail = (status: number) =>
  new Response(
    JSON.stringify({
      success: false,
      error: { code: `error_${status}`, message: `status ${status}` },
    }),
    { status },
  );

describe("point policy API", () => {
  it("reads with same-origin cookies and required brand scope", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(policy));
    const api = createPointPolicyApi(fetcher);
    await api.getPointPolicy("brand-1");
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe("/api/v1/admin/point-policy");
    expect(init?.method).toBe("GET");
    expect(init?.credentials).toBe("same-origin");
    const headers = new Headers(init?.headers);
    expect(headers.get("X-Brand-ID")).toBe("brand-1");
    expect(headers.get("Authorization")).toBeNull();
  });

  it("puts canonical limits/null with CAS reason and idempotency key", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(policy));
    const api = createPointPolicyApi(fetcher);
    const body = {
      version: 7,
      max_balance_points: null,
      max_recharge_points: "10000",
      max_adjustment_points: "500",
      reason: "Quarterly limits",
    };
    await api.updatePointPolicy("brand-1", body, "policy-key");
    const [, init] = fetcher.mock.calls[0];
    expect(init?.method).toBe("PUT");
    expect(init?.credentials).toBe("same-origin");
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe("brand-1");
    expect(new Headers(init?.headers).get("Idempotency-Key")).toBe(
      "policy-key",
    );
    expect(JSON.parse(String(init?.body))).toEqual(body);
    expect(JSON.parse(String(init?.body))).not.toHaveProperty("balance");
  });

  it("preserves 401, 403 and 409 failures without returning a policy", async () => {
    for (const status of [401, 403, 409]) {
      const api = createPointPolicyApi(
        vi.fn<typeof fetch>().mockResolvedValue(fail(status)),
      );
      await expect(api.getPointPolicy("brand-1")).rejects.toMatchObject({
        status,
        code: `error_${status}`,
      });
    }
  });

  it("applies only mapped grants for an authorized brand", () => {
    const scoped = {
      super_admin: false,
      brand_ids: ["a"],
      permissions_by_brand: {
        a: ["point_policy.view.brand", "point_policy.write.brand"],
      },
    };
    expect(canViewPointPolicy(scoped, "a")).toBe(true);
    expect(canViewPointPolicy(scoped, "b")).toBe(false);
    expect(effectiveBrandPermissions(scoped, "b").size).toBe(0);
    expect(canWritePointPolicy(scoped, "a")).toBe(true);
    expect(canWritePointPolicy({ ...scoped, super_admin: true }, "a")).toBe(
      false,
    );
    const platformView = {
      ...scoped,
      brand_ids: ["a"],
      permissions: [],
      permissions_by_brand: {},
      platform_permissions: ["point_policy.view.platform"],
    };
    expect(canViewPointPolicy(platformView, "b")).toBe(false);
    expect(canWritePointPolicy(platformView, "b")).toBe(false);
  });

  it("accepts only canonical positive integer limit strings", () => {
    for (const value of ["1", "10", "9007199254740993123456789"])
      expect(isCanonicalPositiveLimit(value)).toBe(true);
    for (const value of ["", "0", "00", "01", "-1", "+1", "1.0", " 1"])
      expect(isCanonicalPositiveLimit(value)).toBe(false);
  });

  it("reuses a same-brand/same-body retry key and rotates it for edits or another brand", () => {
    const keyFor = createPointPolicyKeyTracker();
    const input = {
      brand_id: "brand-1",
      version: 7,
      max_balance_points: null,
      max_recharge_points: "10000",
      max_adjustment_points: "500",
      reason: "Quarterly limits",
    };
    const first = keyFor(input);
    expect(keyFor({ ...input })).toBe(first);
    expect(keyFor({ ...input, reason: "Updated limits" })).not.toBe(first);
    expect(keyFor({ ...input, brand_id: "brand-2" })).not.toBe(first);
  });
});
