import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { createJoinCodesApi, joinCodePermissions, joinCodeInstantMicros, validJoinCodeDateRange, type JoinCode } from "./join-codes-api";

const brand = "00000000-0000-4000-8000-000000000001";
const accountId = "00000000-0000-4000-8000-000000000002";
const owner = "00000000-0000-4000-8000-000000000003";
const agent = "00000000-0000-4000-8000-000000000004";
const id = "00000000-0000-4000-8000-000000000005";
const actor = "00000000-0000-4000-8000-000000000006";
const at = "2026-10-06T04:00:00.123456Z";
const code: JoinCode = { id, brand_id: brand, kind: "agent", code: "0123456789ABCDEF01234567", owner_member_id: owner,
  agent_id: agent, status: "active", starts_at: at, expires_at: null, version: 2, usable: true, created_at: at, updated_at: at, audit_log_id: actor };
const ok = (data: unknown, status = 200) => new Response(JSON.stringify({ success: true, data }), { status });
const fail = (status: number, errorCode = "JOIN_CODE_VERSION_CONFLICT") => new Response(JSON.stringify({ success: false, error: { code: errorCode, message: "rejected" } }), { status });

describe("join code permissions", () => {
  const account = (overrides: Partial<AdminAccount> = {}): AdminAccount => ({ id: accountId, super_admin: false, brand_ids: [brand], permissions: [], ...overrides });
  it("requires explicit in-scope view grants and never grants super-admin writes", () => {
    expect(joinCodePermissions(account({ permissions: ["join_code.view.brand", "join_code.write.brand"] }), brand)).toEqual({ view: true, write: true });
    expect(joinCodePermissions(account({ brand_ids: [] }), brand)).toEqual({ view: false, write: false });
    expect(joinCodePermissions(account({ permissions_by_brand: {} }), brand).view).toBe(false);
    expect(joinCodePermissions(account({ super_admin: true, platform_permissions: ["join_code.view.platform", "join_code.write.brand"] }), brand)).toEqual({ view: true, write: false });
    expect(joinCodePermissions(account({ super_admin: true, brand_ids: [], platform_permissions: ["join_code.view.platform"] }), "00000000-0000-4000-8000-000000000009").view).toBe(true);
    expect(joinCodePermissions(account({ super_admin: true, brand_ids: [] }), "00000000-0000-4000-8000-000000000009").view).toBe(false);
  });
});

describe("join codes admin API", () => {
  it("lists with only contract filters and validates returned scope and paging", async () => {
    const page = { brand_id: brand, kind: "agent", owner_member_id: owner, items: [code], limit: 10, offset: 20, total_count: "21" };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(page));
    await expect(createJoinCodesApi(fetcher).list(brand, { kind: "agent", owner_member_id: owner }, 10, 20)).resolves.toEqual(page);
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe(`/api/v1/admin/join-codes?kind=agent&owner_member_id=${owner}&limit=10&offset=20`);
    expect(init?.method).toBe("GET"); expect(init?.credentials).toBe("same-origin");
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    await expect(createJoinCodesApi(vi.fn<typeof fetch>().mockResolvedValue(ok({ ...page, items: [{ ...code, owner_member_id: actor }] })))
      .list(brand, { kind: "agent", owner_member_id: owner }, 10, 20)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("gets strict DTOs and admin audit history without exposing altered scopes", async () => {
    const history = { brand_id: brand, code_id: id, items: [{ id: actor, brand_id: brand, code_id: id, version: 2, status: "active",
      starts_at: at, expires_at: null, actor_id: accountId, reason: "reviewed owner", audit_log_id: owner, created_at: at }], limit: 20, offset: 0, total_count: "1" };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok(code)).mockResolvedValueOnce(ok(history));
    const api = createJoinCodesApi(fetcher);
    await expect(api.get(brand, id)).resolves.toEqual(code);
    await expect(api.history(brand, id)).resolves.toEqual(history);
    expect(fetcher.mock.calls[0][0]).toBe(`/api/v1/admin/join-codes/${id}`);
    expect(fetcher.mock.calls[1][0]).toBe(`/api/v1/admin/join-codes/${id}/history?limit=20&offset=0`);
    await expect(createJoinCodesApi(vi.fn<typeof fetch>().mockResolvedValue(ok({ ...code, code: "invented" }))).get(brand, id))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("creates only matching active version-one receipts and preserves exact request fields", async () => {
    const body = { kind: "agent" as const, owner_member_id: owner, agent_id: agent, starts_at: "2026-10-06T04:00:00.123400Z", expires_at: null, reason: "new campaign" };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok({ ...code, starts_at: "2026-10-06T12:00:00.1234+08:00", version: 1 }));
    await expect(createJoinCodesApi(fetcher).create(brand, body, "create-key")).resolves.toMatchObject({ version: 1 });
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe("/api/v1/admin/join-codes"); expect(init?.method).toBe("POST");
    expect(new Headers(init?.headers).get("Idempotency-Key")).toBe("create-key");
    expect(JSON.parse(String(init?.body))).toEqual(body);
    for (const badDate of ["2026-02-30T10:00:00Z", "2026-10-06T04:00:00.1234567Z", "2026-10-06 04:00:00Z"]) {
      await expect(createJoinCodesApi(vi.fn<typeof fetch>()).create(brand, { ...body, starts_at: badDate }, "key"))
        .rejects.toMatchObject({ status: 0, code: "JOIN_CODE_INPUT_INVALID" });
    }
    await expect(createJoinCodesApi(vi.fn<typeof fetch>()).create(brand, { ...body, extra: "not contracted" } as typeof body, "key"))
      .rejects.toMatchObject({ status: 0, code: "JOIN_CODE_INPUT_INVALID" });
    expect(validJoinCodeDateRange("2026-10-06T04:00:00.123456Z", "2026-10-06T04:00:00.123457Z")).toBe(true);
    expect(validJoinCodeDateRange("2026-10-06T04:00:00.123457Z", "2026-10-06T04:00:00.123456Z")).toBe(false);
    expect(joinCodeInstantMicros("2026-10-06T04:00:00.123400Z")).toBe(joinCodeInstantMicros("2026-10-06T12:00:00.1234+08:00"));
    await expect(createJoinCodesApi(vi.fn<typeof fetch>().mockResolvedValue(ok({ ...code, starts_at: "2026-10-06T12:00:00.123401+08:00", version: 1 })))
      .create(brand, body, "key")).rejects.toMatchObject({ status: 0, code: "INVALID_RESPONSE" });
    await expect(createJoinCodesApi(vi.fn<typeof fetch>().mockResolvedValue(ok({ ...code, audit_log_id: null, version: 1 })))
      .create(brand, body, "key")).rejects.toMatchObject({ status: 0, code: "INVALID_RESPONSE" });
    await expect(createJoinCodesApi(vi.fn<typeof fetch>().mockResolvedValue(ok({ ...code, owner_member_id: actor, version: 1 })))
      .create(brand, body, "key")).rejects.toMatchObject({ status: 0, code: "INVALID_RESPONSE" });
  });

  it("updates with an exact next-version receipt and never performs an implicit GET", async () => {
    const identity = { kind: code.kind, code: code.code, owner_member_id: owner, agent_id: agent };
    const body = { version: 2, status: "disabled" as const, starts_at: null, expires_at: "2026-10-06T04:00:00.123400Z", reason: "retire campaign" };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok({ ...code, status: body.status, starts_at: body.starts_at, expires_at: "2026-10-06T12:00:00.1234+08:00", version: 3 }));
    await expect(createJoinCodesApi(fetcher).update(brand, id, identity, body, "update-key")).resolves.toMatchObject({ version: 3 });
    expect(fetcher).toHaveBeenCalledTimes(1);
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe(`/api/v1/admin/join-codes/${id}`); expect(init?.method).toBe("PUT");
    expect(new Headers(init?.headers).get("Idempotency-Key")).toBe("update-key");
    expect(JSON.parse(String(init?.body))).toEqual(body);
    await expect(createJoinCodesApi(vi.fn<typeof fetch>().mockResolvedValue(ok({ ...code, status: body.status, starts_at: null,
      expires_at: "2026-10-06T12:00:00.123401+08:00", version: 3 }))).update(brand, id, identity, body, "key"))
      .rejects.toMatchObject({ status: 0, code: "INVALID_RESPONSE" });
    await expect(createJoinCodesApi(vi.fn<typeof fetch>()).update(brand, id, identity, { ...body, extra: true } as typeof body, "key"))
      .rejects.toMatchObject({ status: 0, code: "JOIN_CODE_INPUT_INVALID" });
    await expect(createJoinCodesApi(vi.fn<typeof fetch>().mockResolvedValue(ok({ ...code, version: 3, owner_member_id: actor,
      status: body.status, starts_at: body.starts_at, expires_at: body.expires_at }))).update(brand, id, identity, body, "key"))
      .rejects.toMatchObject({ status: 0, code: "INVALID_RESPONSE" });
  });

  it("rejects invalid query/input before transport and preserves backend error codes", async () => {
    const fetcher = vi.fn<typeof fetch>(); const api = createJoinCodesApi(fetcher);
    await expect(api.list(brand, {}, 101)).rejects.toMatchObject({ code: "JOIN_CODE_INPUT_INVALID" });
    await expect(api.get(brand, "bad-id")).rejects.toMatchObject({ code: "JOIN_CODE_INPUT_INVALID" });
    await expect(api.history(brand, id, 20, -1)).rejects.toMatchObject({ code: "JOIN_CODE_INPUT_INVALID" });
    expect(fetcher).not.toHaveBeenCalled();
    await expect(createJoinCodesApi(vi.fn<typeof fetch>().mockResolvedValue(fail(409))).get(brand, id))
      .rejects.toMatchObject({ status: 409, code: "JOIN_CODE_VERSION_CONFLICT" });
    await expect(createJoinCodesApi(vi.fn<typeof fetch>().mockRejectedValue(new Error("offline"))).get(brand, id))
      .rejects.toBeInstanceOf(AdminApiError);
  });
});
