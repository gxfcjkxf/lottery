import { describe, expect, it, vi } from "vitest";
import {
  assignableRoles,
  canEditAccountScope,
  canManage,
  canView,
  canWrite,
  createBodyKeyTracker,
  createManagementApi,
  effectivePermissions,
  rolePermissionChoices,
  rolePermissionUnion,
  type AdminRecord,
  type ManagementAccount,
  type RoleRecord,
} from "./management-api";

const ok = (data: unknown, status = 200) =>
  new Response(JSON.stringify({ success: true, data }), { status });
const fail = (status: number) =>
  new Response(
    JSON.stringify({
      success: false,
      error: { code: `e${status}`, message: `error ${status}` },
    }),
    { status },
  );

describe("management API", () => {
  it("rejects malformed envelopes instead of confirming management writes", async () => {
    const body = { code: "owned_role", name: "Owned role", permissions: ["user.view.brand"], reason: "Verify request receipt" };
    for (const envelope of [null, [], { success: true, data: null }, { success: true, data: [] }, { success: false, error: { message: "Missing code" } }]) {
      const api = createManagementApi(async () => new Response(JSON.stringify(envelope), { status: 200 }));
      await expect(api.createRole("brand/a", body, "management-test-key")).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
    const api = createManagementApi(async () => new Response("upstream error", { status: 400 }));
    await expect(api.createRole("brand/a", body, "management-test-key")).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });
  it("uses base paths, same-origin cookies, JSON bodies and brand headers for list requests", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok({ items: [] }));
    const api = createManagementApi(fetcher);
    await api.roles("brand/a", 50, 100);
    expect(fetcher.mock.calls[0][0]).toBe(
      "/api/v1/admin/roles?limit=50&offset=100",
    );
    expect(fetcher.mock.calls[0][1]?.credentials).toBe("same-origin");
    expect(
      (fetcher.mock.calls[0][1]?.headers as Headers).get("X-Brand-ID"),
    ).toBe("brand/a");
  });

  it("reuses keys for unchanged bodies and rotates them after a body edit", () => {
    const keyFor = createBodyKeyTracker();
    const a = { name: "Editor", reason: "initial" };
    expect(keyFor(a)).toBe(keyFor({ name: "Editor", reason: "initial" }));
    const old = keyFor(a);
    expect(keyFor({ ...a, reason: "changed" })).not.toBe(old);
    const completed = keyFor({ code: "done" });
    keyFor.clear();
    expect(keyFor({ code: "done" })).not.toBe(completed);
  });

  it("sends the tracker key and brand header on same-body retries, then changes the key with the body", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockImplementation(async () => ok({}));
    const api = createManagementApi(fetcher);
    const keyFor = createBodyKeyTracker();
    const body = {
      code: "support",
      name: "Support",
      permissions: ["user.view.brand"],
      reason: "requested",
    };
    await api.createRole("brand-1", body, keyFor(body));
    await api.createRole("brand-1", { ...body }, keyFor({ ...body }));
    const changed = { ...body, reason: "updated" };
    await api.createRole("brand-1", changed, keyFor(changed));
    const keys = fetcher.mock.calls.map(([, init]) =>
      (init?.headers as Headers).get("Idempotency-Key"),
    );
    expect(keys[0]).toBe(keys[1]);
    expect(keys[2]).not.toBe(keys[1]);
    for (const [, init] of fetcher.mock.calls)
      expect((init?.headers as Headers).get("X-Brand-ID")).toBe("brand-1");
  });

  it("preserves 401, 403 and 409 statuses and codes", async () => {
    for (const status of [401, 403, 409]) {
      const api = createManagementApi(
        vi.fn<typeof fetch>().mockResolvedValue(fail(status)),
      );
      await expect(api.permissions("brand-1")).rejects.toMatchObject({
        status,
        code: `e${status}`,
      });
    }
  });

  it("never persists request secrets and exposes role permission union without duplicates", async () => {
    const setItem = vi.fn();
    vi.stubGlobal("localStorage", { setItem });
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
      ok({
        id: "a",
        username: "new",
        status: "active",
        version: 1,
        super_admin: false,
        brand_ids: ["brand-1"],
        role_ids: [],
        role_codes: [],
      }),
    );
    const api = createManagementApi(fetcher);
    await api.createAccount(
      "brand-1",
      {
        username: "new",
        password: "secret-password",
        role_ids: [],
        reason: "provision",
      },
      "stable-key",
    );
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe("/api/v1/admin/accounts");
    expect((init?.headers as Headers).get("Idempotency-Key")).toBe(
      "stable-key",
    );
    expect(JSON.parse(String(init?.body)).password).toBe("secret-password");
    expect(setItem).not.toHaveBeenCalled();
    expect(
      rolePermissionUnion([
        { permissions: ["x", "y"] },
        { permissions: ["y", "z"] },
      ]),
    ).toEqual(["x", "y", "z"]);
    vi.unstubAllGlobals();
  });

  it("uses only mapped permissions for authorized brands and denies missing mappings", () => {
    const account = {
      id: "a",
      super_admin: false,
      brand_ids: ["a", "b"],
      permissions: ["role.write.brand"],
      permissions_by_brand: { a: ["role.view.brand"] },
    };
    expect([...effectivePermissions(account, "a")]).toEqual([
      "role.view.brand",
    ]);
    expect([...effectivePermissions(account, "b")]).toEqual([]);
    expect(canView(account, "b", "role")).toBe(false);
    expect(canWrite(account, "b", "role")).toBe(false);
    expect(canManage({ ...account, permissions_by_brand: undefined }, "a", "role")).toBe(false);
    expect(canWrite({ ...account, brand_ids: ["b"], permissions_by_brand: undefined }, "b", "role")).toBe(false);
  });

  it("requires explicit mapped view and write grants and denies super-admin actors", () => {
    const writer: ManagementAccount = {
      id: "brand-admin",
      super_admin: false,
      brand_ids: ["brand-a"],
      permissions: ["role.view.brand", "admin.write.brand"],
      permissions_by_brand: { "brand-a": ["admin.write.brand", "role.write.brand"] },
    };
    expect(canManage(writer, "brand-a", "admin")).toBe(false);
    expect(canWrite(writer, "brand-a", "admin")).toBe(true);
    expect(canManage(writer, "brand-a", "role")).toBe(false);
    expect(canWrite(writer, "brand-a", "role")).toBe(true);
    expect(canView(writer, "brand-a", "role")).toBe(false);
    const superAdmin = { ...writer, super_admin: true, permissions_by_brand: { "brand-a": ["role.view.brand", "role.write.brand", "admin.write.brand"] } };
    expect(canManage(superAdmin, "brand-a", "role")).toBe(false);
    expect(canWrite(superAdmin, "brand-a", "admin")).toBe(false);
  });

  it("requires a loaded permission catalog and limits choices and role assignment to the actor's grants", () => {
    const writer: ManagementAccount = {
      id: "brand-admin",
      super_admin: false,
      brand_ids: ["brand-a"],
      permissions: ["role.view.brand", "role.write.brand"],
      permissions_by_brand: { "brand-a": ["role.write.brand", "user.view.brand"] },
    };
    expect(rolePermissionChoices(writer, "brand-a", null)).toEqual([]);
    expect(
      rolePermissionChoices(writer, "brand-a", [
        "custom.read.brand",
        "custom.read.brand",
        "user.view.brand",
        "role.write.brand",
      ]),
    ).toEqual(["role.write.brand", "user.view.brand"]);
    const roles: RoleRecord[] = [
      {
        id: "a",
        brand_id: "brand-a",
        code: "active",
        name: "Active",
        status: "active",
        version: 1,
        is_bootstrap: false,
        permissions: ["user.view.brand"],
      },
      {
        id: "b",
        brand_id: "brand-a",
        code: "disabled",
        name: "Disabled",
        status: "disabled",
        version: 1,
        is_bootstrap: false,
        permissions: [],
      },
      {
        id: "c",
        brand_id: "brand-b",
        code: "other",
        name: "Other brand",
        status: "active",
        version: 1,
        is_bootstrap: false,
        permissions: [],
      },
    ];
    expect(
      assignableRoles(writer, "brand-a", roles).map((role) => role.id),
    ).toEqual(["a"]);
  });

  it("allows only same-brand ordinary targets, never self, super-admin, or multi-brand targets", () => {
    const writer: ManagementAccount = {
      id: "brand-admin",
      super_admin: false,
      brand_ids: ["a"],
      permissions: [],
      permissions_by_brand: { a: ["admin.write.brand"] },
    };
    const target = (id: string, brand_ids = ["a"], super_admin = false): AdminRecord => ({
      id,
      username: id,
      status: "active",
      version: 1,
      super_admin,
      brand_ids,
      role_ids: [],
      role_codes: [],
    });
    expect(canEditAccountScope(writer, "a", target("ordinary"))).toBe(true);
    expect(canEditAccountScope(writer, "a", target("brand-admin"))).toBe(false);
    expect(canEditAccountScope(writer, "a", target("super", ["a"], true))).toBe(false);
    expect(canEditAccountScope(writer, "a", target("multi", ["a", "b"]))).toBe(false);
    const brandWriter: ManagementAccount = {
      id: "brand-admin",
      super_admin: false,
      brand_ids: ["a"],
      permissions: [],
      permissions_by_brand: { a: ["admin.write.brand"] },
    };
    expect(canEditAccountScope(brandWriter, "b", target("cross-brand"))).toBe(false);
  });
});
