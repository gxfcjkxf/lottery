import { describe, expect, it, vi } from "vitest";
import {
  createNotificationApi,
  NotificationApiError,
  type NotificationItem,
} from "./notification-api";

const brandId = "11111111-1111-4111-8111-111111111111";
const memberId = "22222222-2222-4222-8222-222222222222";
const notificationId = "ab333333-3333-4333-8333-333333333333";
const resourceId = "44444444-4444-4444-8444-444444444444";
const createdAt = "2026-10-06T12:00:00.123+08:00";

const joined: NotificationItem = {
  id: notificationId,
  brand_id: brandId,
  member_id: memberId,
  event_type: "member.joined",
  template_key: "member.joined",
  template_version: 1,
  payload: { resource_id: memberId, points: null },
  created_at: createdAt,
  read_at: null,
};

const page = (overrides: Record<string, unknown> = {}) => ({
  brand_id: brandId,
  member_id: memberId,
  items: [joined],
  unread_count: "9223372036854775807",
  limit: 20,
  offset: 0,
  ...overrides,
});
const receipt = (overrides: Record<string, unknown> = {}) => ({
  brand_id: brandId,
  member_id: memberId,
  ids: [notificationId],
  changed: 1,
  unread_count: "0",
  ...overrides,
});
const ok = (data: unknown) =>
  new Response(JSON.stringify({ success: true, data }), { status: 200 });

describe("user notification API", () => {
  it("lists a strict page using the brand route and included cookie credentials", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(page()));
    const api = createNotificationApi({ brandCode: "north star", fetch: fetcher });
    const result = await api.list();

    expect(result.items[0]).toEqual(joined);
    expect(result.unread_count).toBe("9223372036854775807");
    expect(fetcher.mock.calls[0][0]).toBe(
      "/api/v1/b/north%20star/notifications?limit=20&offset=0",
    );
    const [, init] = fetcher.mock.calls[0];
    expect(init?.method).toBe("GET");
    expect(init?.credentials).toBe("include");
    const headers = new Headers(init?.headers);
    expect(headers.get("Accept")).toBe("application/json");
    expect(headers.get("Authorization")).toBeNull();
    expect(headers.get("Cookie")).toBeNull();
  });

  it("uses unprefixed paths, requested pagination and no payload-provided links or HTML", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValue(ok(page({ items: [], unread_count: "0", limit: 5, offset: 30 })));
    const api = createNotificationApi({ brandCode: "", fetch: fetcher });
    await expect(api.list(5, 30)).resolves.toMatchObject({ limit: 5, offset: 30 });
    expect(fetcher.mock.calls[0][0]).toBe("/api/v1/notifications?limit=5&offset=30");
  });

  it("validates event mapping, member join relationship, IDs, dates, scopes and int64 strings", async () => {
    const validOther = {
      ...joined,
      event_type: "recharge.confirmed",
      template_key: "recharge.confirmed",
      payload: { resource_id: resourceId, points: "9007199254740993123" },
      read_at: "2026-10-06T04:00:00Z",
    };
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(ok(page({ items: [validOther] })))
      .mockResolvedValueOnce(ok(page({ items: [{ ...joined, template_key: "other" }] })))
      .mockResolvedValueOnce(ok(page({ items: [{ ...joined, payload: { resource_id: resourceId, points: null } }] })))
      .mockResolvedValueOnce(ok(page({ items: [{ ...joined, created_at: "2026-02-30T12:00:00Z" }] })))
      .mockResolvedValueOnce(ok(page({ items: [{ ...joined, member_id: resourceId }] })))
      .mockResolvedValueOnce(ok(page({ unread_count: "01" })))
      .mockResolvedValueOnce(ok(page({ items: [joined, joined] })));
    const api = createNotificationApi({ fetch: fetcher });

    const parsed = await api.list();
    expect(parsed.items[0].payload.points).toBe("9007199254740993123");
    expect(parsed.items[0].read_at).toBe("2026-10-06T04:00:00Z");
    for (let index = 0; index < 6; index++) {
      await expect(api.list()).rejects.toMatchObject({
        status: 502,
        code: "invalid_response",
      });
    }
  });

  it("parses historical prize events with UUID order references and positive canonical int64 strings", async () => {
    const won = {
      ...joined,
      event_type: "bet.order.won",
      template_key: "bet.order.won",
      payload: { resource_id: resourceId, points: "9223372036854775807" },
    };
    const reversed = {
      ...joined,
      id: "cd333333-3333-4333-8333-333333333333",
      event_type: "bet.order.prize_reversed",
      template_key: "bet.order.prize_reversed",
      payload: { resource_id: resourceId, points: "9007199254740993" },
    };
    const invalidItems = [
      { ...won, template_version: 2 },
      { ...won, payload: { ...won.payload, resource_id: "order-42" } },
      { ...won, payload: { ...won.payload, points: 0 } },
      { ...won, payload: { ...won.payload, points: "0" } },
      { ...won, payload: { ...won.payload, points: "01" } },
      { ...won, payload: { ...won.payload, points: "9223372036854775808" } },
    ];
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(ok(page({ items: [won, reversed] })))
      .mockImplementation(async () => ok(page({ items: [invalidItems.shift()] })));
    const api = createNotificationApi({ fetch: fetcher });

    const parsed = await api.list();
    expect(parsed.items.map(({ event_type, payload }) => [event_type, payload])).toEqual([
      ["bet.order.won", { resource_id: resourceId, points: "9223372036854775807" }],
      ["bet.order.prize_reversed", { resource_id: resourceId, points: "9007199254740993" }],
    ]);
    for (let index = 0; index < 6; index++) {
      await expect(api.list()).rejects.toMatchObject({ status: 502, code: "invalid_response" });
    }
  });

  it("enforces backend page bounds, page size and minimum unread count", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(ok(page({ items: [joined, { ...joined, id: resourceId }], limit: 1 })))
      .mockResolvedValueOnce(ok(page({ unread_count: "0" })));
    const api = createNotificationApi({ fetch: fetcher });
    await expect(api.list(1)).rejects.toMatchObject({ status: 502, code: "invalid_response" });
    await expect(api.list()).rejects.toMatchObject({ status: 502, code: "invalid_response" });

    await expect(api.list(101)).rejects.toMatchObject({ status: 0, code: "invalid_parameter" });
    await expect(api.list(20, 1_000_001)).rejects.toMatchObject({ status: 0, code: "invalid_parameter" });
    expect(fetcher).toHaveBeenCalledTimes(2);
  });

  it("posts exact distinct IDs with the supplied idempotency key and validates replay receipts", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(ok(page()))
      .mockResolvedValueOnce(ok(receipt()))
      .mockResolvedValueOnce(ok(receipt({ changed: 0, unread_count: "0" })))
      .mockResolvedValueOnce(ok(receipt({ ids: [resourceId] })))
      .mockResolvedValueOnce(ok(receipt({ ids: [notificationId.toUpperCase()] })));
    const api = createNotificationApi({ fetch: fetcher });
    await api.list();

    const first = await api.markRead(
      [notificationId],
      { brand_id: brandId, member_id: memberId },
      "stable-read-operation-1",
    );
    expect(first.changed).toBe(1);
    expect(fetcher.mock.calls[1][0]).toBe("/api/v1/notifications/read");
    const [, init] = fetcher.mock.calls[1];
    expect(init?.method).toBe("POST");
    expect(init?.credentials).toBe("include");
    expect(JSON.parse(String(init?.body))).toEqual({ ids: [notificationId] });
    const headers = new Headers(init?.headers);
    expect(headers.get("Idempotency-Key")).toBe("stable-read-operation-1");
    expect(headers.get("Content-Type")).toBe("application/json");

    await expect(
      api.markRead(
        [notificationId],
        { brand_id: brandId, member_id: memberId },
        "stable-read-operation-1",
      ),
    ).resolves.toMatchObject({ ids: [notificationId], changed: 0 });
    expect(
      new Headers(fetcher.mock.calls[2][1]?.headers).get("Idempotency-Key"),
    ).toBe("stable-read-operation-1");

    await expect(
      api.markRead(
        [notificationId],
        { brand_id: brandId, member_id: memberId },
        "stable-read-operation-1",
      ),
    ).rejects.toMatchObject({ code: "invalid_response", status: 502 });
    await expect(
      api.markRead(
        [notificationId],
        { brand_id: brandId, member_id: memberId },
        "stable-read-operation-1",
      ),
    ).rejects.toMatchObject({ code: "invalid_response", status: 502 });
  });

  it("rejects context switching and mismatched or out-of-range receipts", async () => {
    const otherBrand = "55555555-5555-4555-8555-555555555555";
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(ok(page()))
      .mockResolvedValueOnce(ok(receipt({ brand_id: otherBrand })))
      .mockResolvedValueOnce(ok(receipt({ changed: 2 })));
    const api = createNotificationApi({ fetch: fetcher });
    await api.list();
    await expect(
      api.markRead(
        [notificationId],
        { brand_id: otherBrand, member_id: memberId },
        "key-aaaaa",
      ),
    ).rejects.toMatchObject({ code: "context_mismatch", status: 0 });
    expect(fetcher).toHaveBeenCalledTimes(1);

    await expect(
      api.markRead(
        [notificationId],
        { brand_id: brandId, member_id: memberId },
        "key-bbbbb",
      ),
    ).rejects.toMatchObject({ code: "invalid_response", status: 502 });
    await expect(
      api.markRead(
        [notificationId],
        { brand_id: brandId, member_id: memberId },
        "key-ccccc",
      ),
    ).rejects.toMatchObject({ code: "invalid_response", status: 502 });
  });

  it("rejects invalid input before network access and surfaces HTTP and network errors", async () => {
    const fetcher = vi.fn<typeof fetch>();
    const api = createNotificationApi({ fetch: fetcher });
    await expect(api.list(0)).rejects.toMatchObject({ code: "invalid_parameter" });
    await expect(
      api.markRead([], { brand_id: brandId, member_id: memberId }, "key"),
    ).rejects.toBeInstanceOf(NotificationApiError);
    await expect(
      api.markRead([notificationId, notificationId], { brand_id: brandId, member_id: memberId }, "key"),
    ).rejects.toMatchObject({ code: "invalid_parameter" });
    await expect(
      api.markRead([notificationId.toUpperCase()], { brand_id: brandId, member_id: memberId }, "valid-key"),
    ).rejects.toMatchObject({ code: "invalid_parameter" });
    for (const key of ["short", "bad/key-1", `a${"b".repeat(128)}`]) {
      await expect(
        api.markRead([notificationId], { brand_id: brandId, member_id: memberId }, key),
      ).rejects.toMatchObject({ code: "invalid_parameter" });
    }
    expect(fetcher).not.toHaveBeenCalled();

    const failed = createNotificationApi({
      fetch: vi.fn<typeof fetch>().mockRejectedValue(new Error("offline")),
    });
    await expect(failed.list()).rejects.toMatchObject({ status: 0, message: "offline" });

    const interruptedResponse = createNotificationApi({
      fetch: vi.fn<typeof fetch>().mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.reject(new TypeError("stream interrupted")),
      } as Response),
    });
    await expect(interruptedResponse.list()).rejects.toMatchObject({
      status: 0,
      code: "network_error",
    });

    const httpError = createNotificationApi({
      fetch: vi.fn<typeof fetch>().mockResolvedValue(
        new Response(JSON.stringify({ success: false, error: { code: "unauthorized", message: "Sign in" } }), { status: 401 }),
      ),
    });
    await expect(httpError.list()).rejects.toMatchObject({ status: 401, code: "unauthorized" });
  });

  it("classifies malformed HTTP success bodies and envelopes as invalid responses", async () => {
    for (const status of [200, 201]) {
      const malformedJson = createNotificationApi({
        fetch: vi.fn<typeof fetch>().mockResolvedValue(
          new Response("not json", { status }),
        ),
      });
      await expect(malformedJson.list()).rejects.toMatchObject({
        status: 502,
        code: "invalid_response",
      });

      const invalidEnvelope = createNotificationApi({
        fetch: vi.fn<typeof fetch>().mockResolvedValue(
          new Response(JSON.stringify({ success: true }), { status }),
        ),
      });
      await expect(invalidEnvelope.list()).rejects.toMatchObject({
        status: 502,
        code: "invalid_response",
      });

      const unsuccessfulEnvelope = createNotificationApi({
        fetch: vi.fn<typeof fetch>().mockResolvedValue(
          new Response(
            JSON.stringify({ success: false, error: { code: "unexpected" } }),
            { status },
          ),
        ),
      });
      await expect(unsuccessfulEnvelope.list()).rejects.toMatchObject({
        status: 502,
        code: "invalid_response",
      });
    }
  });
});
