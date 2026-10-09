import { afterEach, describe, expect, it, vi } from "vitest";
import { createAuthClient, isBrandJoinRequired } from "../../shared/src/auth";
import {
  discardAccessToken,
  isTelegramClientIdConfigured,
  normalizeIdentifier,
  passwordByteLength,
  requestTelegramIdToken,
} from "./auth";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("auth client", () => {
  it("sends JSON with same-origin cookies and a fresh idempotency key, without persisting the access token", async () => {
    const localWrite = vi.fn();
    const sessionWrite = vi.fn();
    vi.stubGlobal("localStorage", { getItem: () => null, setItem: localWrite });
    vi.stubGlobal("sessionStorage", {
      getItem: () => null,
      setItem: sessionWrite,
    });
    const session = {
      access_token: "secret-token",
      token_type: "Bearer" as const,
      expires_at: "2026-10-07T00:00:00Z",
      user: { id: "u1", username: "lee", status: "active" },
      member: {
        id: "m1",
        brand_id: "b1",
        status: "active",
        display_name: "lee",
        joined_at: "2026-10-06T00:00:00Z",
      },
    };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(
        JSON.stringify({ success: true, data: session, request_id: "r1" }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );
    const client = createAuthClient({ brandCode: "luma", fetcher });

    const received = await client.register({
      username: "lee",
      password: "long-password",
      privacy_policy_version: "dev-1",
      service_terms_version: "dev-1",
    });

    const [url, init] = fetcher.mock.calls[0]!;
    expect(url).toBe("/api/v1/b/luma/auth/register");
    expect(init?.credentials).toBe("same-origin");
    expect(init?.method).toBe("POST");
    expect(new Headers(init?.headers).get("content-type")).toBe(
      "application/json",
    );
    expect(new Headers(init?.headers).get("idempotency-key")).toMatch(
      /^[\w-]{36}$/,
    );
    expect(JSON.parse(String(init?.body))).toMatchObject({
      username: "lee",
      privacy_policy_version: "dev-1",
    });
    expect(discardAccessToken(received)).not.toHaveProperty("access_token");
    expect(localWrite).not.toHaveBeenCalled();
    expect(sessionWrite).not.toHaveBeenCalled();
  });

  it("sends each write with a separate idempotency key and supports PATCH profile requests", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockImplementation(async () =>
        new Response(
          JSON.stringify({ success: true, data: { audit_log_id: "audit-1" } }),
          { status: 200 },
        ),
      );
    const client = createAuthClient({ fetcher });

    await expect(
      client.updateProfile({ username: "new-name" }),
    ).resolves.toEqual({ audit_log_id: "audit-1" });
    await client.logout();

    const firstHeaders = new Headers(fetcher.mock.calls[0]![1]?.headers);
    const secondHeaders = new Headers(fetcher.mock.calls[1]![1]?.headers);
    expect(firstHeaders.get("idempotency-key")).not.toBe(
      secondHeaders.get("idempotency-key"),
    );
    expect(fetcher.mock.calls[0]![1]?.method).toBe("PATCH");
    expect(fetcher.mock.calls[0]![0]).toBe("/api/v1/me/profile");
    expect(fetcher.mock.calls[1]![0]).toBe("/api/v1/auth/logout");
  });

  it("preserves API error status and code from the standard error envelope", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: false,
          error: {
            code: "BRAND_JOIN_REQUIRED",
            message: "Accept the current brand terms.",
          },
          request_id: "req-join",
        }),
        { status: 409 },
      ),
    );
    const client = createAuthClient({ fetcher });

    await expect(
      client.login({ identifier: "lee", password: "long-password" }),
    ).rejects.toSatisfy((error) => {
      expect(isBrandJoinRequired(error)).toBe(true);
      expect(error).toMatchObject({
        status: 409,
        code: "BRAND_JOIN_REQUIRED",
        message: "Accept the current brand terms.",
      });
      return true;
    });
  });

  it("preserves JOIN_ATTRIBUTION_FIXED as a distinct conflict without classifying it as brand-join-required", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: false,
          error: {
            code: "JOIN_ATTRIBUTION_FIXED",
            message: "This membership already has a join source.",
          },
        }),
        { status: 409 },
      ),
    );
    const client = createAuthClient({ fetcher });

    await expect(
      client.login({ identifier: "member", password: "long-password", referral_code: "A1B2C3D4E5F607182930ABCD" }),
    ).rejects.toSatisfy((error) => {
      expect(isBrandJoinRequired(error)).toBe(false);
      expect(error).toMatchObject({ status: 409, code: "JOIN_ATTRIBUTION_FIXED" });
      return true;
    });
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it("loads branded challenge SVG and reports captcha settings from context.auth", async () => {
    const payloads = [
      {
        success: true,
        data: {
          id: "challenge-1",
          svg: "<svg/>",
          expires_at: "2026-10-07T00:00:00Z",
        },
      },
      {
        success: true,
        data: {
          auth: {
            captcha_enabled: true,
            telegram_enabled: true,
            telegram_client_id: "123456",
          },
        },
      },
      {
        success: true,
        data: {
          id: "telegram-challenge",
          nonce: "nonce-1",
          expires_at: "2026-10-07T00:00:00Z",
        },
      },
    ];
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        new Response(JSON.stringify(payloads[0]), { status: 200 }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify(payloads[1]), { status: 200 }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify(payloads[2]), { status: 200 }),
      );
    const client = createAuthClient({ brandCode: "luma", fetcher });

    await expect(client.getChallenge()).resolves.toEqual(payloads[0]!.data);
    await expect(client.getAuthFeatures()).resolves.toEqual({
      captcha_enabled: true,
      telegram_enabled: true,
      telegram_client_id: "123456",
    });
    await expect(client.getTelegramChallenge()).resolves.toEqual(
      payloads[2]!.data,
    );
    expect(fetcher.mock.calls.map((call) => call[0])).toEqual([
      "/api/v1/b/luma/auth/challenge",
      "/api/v1/b/luma/context",
      "/api/v1/b/luma/auth/telegram/challenge",
    ]);
  });

  it("normalizes phone identifiers and counts password bytes", () => {
    expect(normalizeIdentifier("  +65 (8123) 4567 ")).toEqual({
      phone: "+6581234567",
      identifier: "+6581234567",
    });
    expect(normalizeIdentifier("  8123 4567  ")).toEqual({
      username: "8123 4567",
      identifier: "8123 4567",
    });
    expect(normalizeIdentifier("  Lucky_Lee  ")).toEqual({
      username: "lucky_lee",
      identifier: "lucky_lee",
    });
    expect(passwordByteLength("é")).toBe(2);
  });

  it("forwards one normalized explicit join code on register, confirmed login, and Telegram", async () => {
    const session = {
      access_token: "token",
      token_type: "Bearer",
      expires_at: "2026-10-07T00:00:00Z",
      user: { id: "u1", status: "active" },
      member: {
        id: "m1",
        brand_id: "b1",
        status: "active",
        display_name: "member",
        joined_at: "",
      },
    };
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () =>
      new Response(JSON.stringify({ success: true, data: session }), { status: 200 }),
    );
    const client = createAuthClient({ fetcher });

    await client.register({
      username: "member",
      password: "long-password",
      privacy_policy_version: "v1",
      service_terms_version: "v1",
      referral_code: "  a1b2c3d4e5f607182930abcd ",
    });
    await client.login({
      identifier: "member",
      password: "long-password",
      agent_code: " d1b2c3d4e5f607182930abcd\n",
    });
    await client.telegram({
      id_token: "signed.jwt",
      challenge_id: "challenge",
      nonce: "nonce",
      privacy_policy_version: "v1",
      service_terms_version: "v1",
      referral_code: " e1b2c3d4e5f607182930abcd ",
    });

    const bodies = fetcher.mock.calls.map((call) => JSON.parse(String(call[1]?.body)));
    expect(bodies[0]).toMatchObject({ referral_code: "A1B2C3D4E5F607182930ABCD" });
    expect(bodies[0]).not.toHaveProperty("agent_code");
    expect(bodies[1]).toMatchObject({ agent_code: "D1B2C3D4E5F607182930ABCD" });
    expect(bodies[1]).not.toHaveProperty("referral_code");
    expect(bodies[2]).toMatchObject({ referral_code: "E1B2C3D4E5F607182930ABCD" });
  });

  it("omits blank join codes and rejects invalid or mutually exclusive values before fetch", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(JSON.stringify({ success: true, data: {} }), { status: 200 }),
    );
    const client = createAuthClient({ fetcher });
    await client.login({ identifier: "member", password: "long-password", agent_code: "  " });
    expect(JSON.parse(String(fetcher.mock.calls[0]?.[1]?.body))).not.toHaveProperty("agent_code");

    const invalidFetcher = vi.fn<typeof fetch>();
    const invalidClient = createAuthClient({ fetcher: invalidFetcher });

    await expect(
      invalidClient.login({
        identifier: "member",
        password: "long-password",
        agent_code: "A1B2C3D4E5F607182930ABCD",
        referral_code: "B1B2C3D4E5F607182930ABCD",
      }),
    ).rejects.toThrow(/either an agent code or a referral code/i);
    await expect(
      invalidClient.login({ identifier: "member", password: "long-password", agent_code: "short" }),
    ).rejects.toThrow(/24 hexadecimal/i);
    expect(invalidFetcher).not.toHaveBeenCalled();
  });

  it("sends a Telegram OIDC token with the matching server challenge and explicit terms", async () => {
    const session = {
      access_token: "token",
      token_type: "Bearer",
      expires_at: "2026-10-07T00:00:00Z",
      user: { id: "u1", status: "active" },
      member: {
        id: "m1",
        brand_id: "b1",
        status: "active",
        display_name: "member",
        joined_at: "",
      },
    };
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValue(
        new Response(JSON.stringify({ success: true, data: session }), {
          status: 200,
        }),
      );
    const client = createAuthClient({ fetcher });

    await client.telegram({
      id_token: "oidc-signed-token",
      challenge_id: "tg-challenge",
      nonce: "server-nonce",
      privacy_policy_version: "dev-1",
      service_terms_version: "dev-1",
    });

    const body = JSON.parse(String(fetcher.mock.calls[0]![1]?.body));
    expect(fetcher.mock.calls[0]![0]).toBe("/api/v1/auth/telegram");
    expect(body).toEqual({
      id_token: "oidc-signed-token",
      challenge_id: "tg-challenge",
      nonce: "server-nonce",
      privacy_policy_version: "dev-1",
      service_terms_version: "dev-1",
    });
    expect(body).not.toHaveProperty("username");
    expect(
      new Headers(fetcher.mock.calls[0]![1]?.headers).get("idempotency-key"),
    ).toBeTruthy();
  });

  it("opens the official Telegram popup with the configured client, nonce, and profile scope", async () => {
    const authPopup = vi.fn(
      (_options: unknown, callback: (result: { id_token: string }) => void) =>
        callback({ id_token: "signed.jwt" }),
    );
    vi.stubGlobal("window", { Telegram: { Login: { auth: authPopup } } });

    await expect(
      requestTelegramIdToken("123456", "nonce-from-server"),
    ).resolves.toBe("signed.jwt");

    expect(isTelegramClientIdConfigured("123456")).toBe(true);
    expect(isTelegramClientIdConfigured("12e3")).toBe(false);
    expect(authPopup).toHaveBeenCalledWith(
      { client_id: 123456, nonce: "nonce-from-server", scope: ["profile"] },
      expect.any(Function),
    );
  });

  it("allows the UI to recover when the Telegram popup never returns", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("window", { Telegram: { Login: { auth: vi.fn() } } });
    const pending = requestTelegramIdToken("123456", "nonce-from-server");
    const assertion = expect(pending).rejects.toThrow("timed out");
    await vi.advanceTimersByTimeAsync(90_000);
    await assertion;
    expect(vi.getTimerCount()).toBe(0);
  });

  it("clears the timeout when the Telegram popup fails synchronously", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("window", {
      Telegram: {
        Login: {
          auth: () => {
            throw new Error("popup unavailable");
          },
        },
      },
    });
    await expect(
      requestTelegramIdToken("123456", "nonce-from-server"),
    ).rejects.toThrow("popup unavailable");
    expect(vi.getTimerCount()).toBe(0);
  });
});
