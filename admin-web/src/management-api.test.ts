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
  KNOWN_BRAND_PERMISSION_KEYS,
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

  it("isolates mapped brand permissions and keeps flat permissions only as legacy fallback", () => {
    const account = {
      id: "a",
      super_admin: false,
      brand_ids: [],
      permissions: ["role.write.brand"],
      permissions_by_brand: { a: ["role.view.brand"] },
    };
    expect([...effectivePermissions(account, "a")]).toEqual([
      "role.view.brand",
    ]);
    expect([...effectivePermissions(account, "b")]).toEqual([]);
    expect(canView(account, "b", "role")).toBe(false);
    expect(canWrite(account, "b", "role")).toBe(false);
    expect(
      canManage({ ...account, permissions_by_brand: undefined }, "b", "role"),
    ).toBe(false);
    expect(
      canWrite(
        {
          id: "legacy",
          super_admin: false,
          brand_ids: [],
          permissions: ["role.write.brand"],
        },
        "b",
        "role",
      ),
    ).toBe(true);
  });

  it("does not treat write grants as read access and reads admin independently from role catalog access", () => {
    const writer: ManagementAccount = {
      id: "platform-admin",
      super_admin: false,
      brand_ids: [],
      permissions: [],
      platform_permissions: [
        "admin.view.platform",
        "admin.write.platform",
        "role.write.platform",
      ],
    };
    expect(canManage(writer, "brand-a", "admin")).toBe(true);
    expect(canManage(writer, "brand-a", "role")).toBe(false);
    expect(canWrite(writer, "brand-a", "role")).toBe(true);
    expect(canView(writer, "brand-a", "role")).toBe(false);
  });

  it("treats an explicit platform permission array as authoritative and falls back only for legacy accounts", () => {
    const account: ManagementAccount = {
      id: "platform-admin",
      super_admin: false,
      brand_ids: [],
      permissions: ["role.view.platform", "role.write.platform"],
      platform_permissions: [],
    };
    expect(canView(account, "brand-a", "role")).toBe(false);
    expect(canWrite(account, "brand-a", "role")).toBe(false);

    const legacy = { ...account, platform_permissions: undefined };
    expect(canView(legacy, "brand-a", "role")).toBe(true);
    expect(canWrite(legacy, "brand-a", "role")).toBe(true);
  });

  it("lets platform role writers grant registered brand keys and platform admin writers assign all active roles in the current brand", () => {
    const writer: ManagementAccount = {
      id: "platform-admin",
      super_admin: false,
      brand_ids: [],
      permissions: [],
      permissions_by_brand: {},
      platform_permissions: ["role.write.platform", "admin.write.platform"],
    };
    expect(rolePermissionChoices(writer, "brand-a", null)).toEqual(
      KNOWN_BRAND_PERMISSION_KEYS,
    );
    expect(
      rolePermissionChoices(writer, "brand-a", [
        "custom.read.brand",
        "admin.write.platform",
      ]),
    ).toEqual(["custom.read.brand"]);
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

  it("allows platform admin writers to edit multi-brand ordinary targets, but never self or super accounts", () => {
    const writer: ManagementAccount = {
      id: "platform-admin",
      super_admin: false,
      brand_ids: [],
      permissions: [],
      platform_permissions: ["admin.write.platform"],
    };
    const target = (id: string, super_admin = false): AdminRecord => ({
      id,
      username: id,
      status: "active",
      version: 1,
      super_admin,
      brand_ids: ["a", "b"],
      role_ids: [],
      role_codes: [],
    });
    expect(canEditAccountScope(writer, target("ordinary"))).toBe(true);
    expect(canEditAccountScope(writer, target("platform-admin"))).toBe(false);
    expect(canEditAccountScope(writer, target("super", true))).toBe(false);
    const brandWriter: ManagementAccount = {
      id: "brand-admin",
      super_admin: false,
      brand_ids: ["a"],
      permissions: [],
      permissions_by_brand: { a: ["admin.write.brand"] },
    };
    expect(canEditAccountScope(brandWriter, target("cross-brand"))).toBe(false);
  });
});
