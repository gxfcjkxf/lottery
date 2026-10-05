import { describe, expect, it, vi } from "vitest";
import { createAdminApi } from "./admin-api";

function response(data: unknown, status = 200) {
  return new Response(
    JSON.stringify({
      success: status < 400,
      ...(status < 400
        ? { data }
        : { error: { code: "denied", message: "Not allowed" } }),
    }),
    {
      status,
      headers: { "Content-Type": "application/json" },
    },
  );
}

describe("admin HTTP client", () => {
  it("posts login with same-origin cookies and never persists a returned token", async () => {
    const setItem = vi.fn();
    vi.stubGlobal("localStorage", { setItem });
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValue(response({ access_token: "must-not-be-stored" }));
    const api = createAdminApi(fetcher);
    await api.login("operator@example.test", "secret", "login-retry-key");
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe("/api/v1/admin/auth/login");
    expect(init?.credentials).toBe("same-origin");
    expect(init?.method).toBe("POST");
    expect(init?.headers).toBeInstanceOf(Headers);
    expect((init?.headers as Headers).get("Content-Type")).toBe(
      "application/json",
    );
    expect((init?.headers as Headers).get("Authorization")).toBeNull();
    expect((init?.headers as Headers).get("Idempotency-Key")).toBe(
      "login-retry-key",
    );
    expect(JSON.parse(String(init?.body))).toEqual({
      identifier: "operator@example.test",
      password: "secret",
    });
    expect(setItem).not.toHaveBeenCalled();
    vi.unstubAllGlobals();
  });

  it("fetches the restored account and permitted brands using cookie credentials", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        response({
          account: {
            id: "admin-1",
            super_admin: false,
            brand_ids: ["brand-1"],
            permissions: ["user.write.brand"],
          },
        }),
      )
      .mockResolvedValueOnce(
        response({
          items: [
            { id: "brand-1", code: "one", name: "One", status: "active" },
          ],
        }),
      );
    const api = createAdminApi(fetcher);
    expect(await api.me()).toMatchObject({ account: { id: "admin-1" } });
    expect(await api.brands()).toMatchObject({ items: [{ id: "brand-1" }] });
    expect(
      fetcher.mock.calls.every(
        ([, init]) => init?.credentials === "same-origin",
      ),
    ).toBe(true);
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
      "/api/v1/admin/me",
      "/api/v1/admin/brands",
    ]);
  });

  it("requires an explicit brand header on directory and audit requests", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockImplementation(async () => response({ items: [] }));
    const api = createAdminApi(fetcher);
    await api.users("brand-uuid", 100, 0);
    await api.audit("brand-uuid", 100, 0);
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
      "/api/v1/admin/users?limit=100&offset=0",
      "/api/v1/admin/audit?limit=100&offset=0",
    ]);
    for (const [, init] of fetcher.mock.calls) {
      expect((init?.headers as Headers).get("X-Brand-ID")).toBe("brand-uuid");
    }
  });

  it("sends mutation JSON, same-origin credentials, brand and a caller-reused idempotency key", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockImplementation(async () => response({}));
    const api = createAdminApi(fetcher);
    const key = "retry-stable-key";
    const body = {
      status: "frozen" as const,
      notes: "reviewed",
      reason: "support request",
    };
    await api.updateUser("brand-uuid", "member/1", body, key);
    await api.updateUser("brand-uuid", "member/1", body, key);
    expect(fetcher.mock.calls[0][0]).toBe("/api/v1/admin/users/member%2F1");
    for (const [, init] of fetcher.mock.calls) {
      const headers = init?.headers as Headers;
      expect(init?.method).toBe("PATCH");
      expect(init?.credentials).toBe("same-origin");
      expect(headers.get("X-Brand-ID")).toBe("brand-uuid");
      expect(headers.get("Idempotency-Key")).toBe(key);
      expect(headers.get("Content-Type")).toBe("application/json");
      expect(JSON.parse(String(init?.body))).toEqual(body);
    }
  });

  it("posts logout and scopes kick and global password reset requests", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockImplementation(async () => response({}));
    const api = createAdminApi(fetcher);
    await api.logout("logout-key");
    await api.kickUser("brand-uuid", "member-1", "reason", "kick-key");
    await api.resetPassword(
      "brand-uuid",
      "member-1",
      "new-password",
      "reason",
      "reset-key",
    );
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
      "/api/v1/admin/auth/logout",
      "/api/v1/admin/users/member-1/kick",
      "/api/v1/admin/users/member-1/reset-password",
    ]);
    expect(
      (fetcher.mock.calls[0][1]?.headers as Headers).get("Idempotency-Key"),
    ).toBe("logout-key");
    expect(JSON.parse(String(fetcher.mock.calls[1][1]?.body))).toEqual({
      reason: "reason",
    });
    expect(JSON.parse(String(fetcher.mock.calls[2][1]?.body))).toEqual({
      password: "new-password",
      reason: "reason",
    });
    expect(
      (fetcher.mock.calls[2][1]?.headers as Headers).get("Idempotency-Key"),
    ).toBe("reset-key");
  });

  it("surfaces envelope errors including an unauthenticated 401 status", async () => {
    const api = createAdminApi(
      vi.fn<typeof fetch>().mockResolvedValue(response(null, 401)),
    );
    await expect(api.me()).rejects.toMatchObject({
      status: 401,
      code: "denied",
      message: "Not allowed",
    });
  });
});
