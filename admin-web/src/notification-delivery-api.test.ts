import { describe, expect, it, vi } from "vitest";
import { reactive } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { createNotificationDeliveryApi, deliveryPermissions, type Delivery } from "./notification-delivery-api";

const brand = "00000000-0000-4000-8000-000000000001";
const otherBrand = "00000000-0000-4000-8000-000000000002";
const eventID = "00000000-0000-4000-8000-000000000003";
const accountID = "00000000-0000-4000-8000-000000000004";
const at = "2026-10-06T04:00:00Z";

function delivery(overrides: Partial<Delivery> = {}): Delivery {
  return {
    event_id: eventID,
    brand_id: brand,
    status: "pending",
    attempt_count: 2,
    last_error: null,
    next_attempt_at: at,
    sent_at: null,
    ...overrides,
  };
}

function response(data: unknown, status = 200) {
  return new Response(JSON.stringify({ success: true, data }), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function account(overrides: Partial<AdminAccount> = {}): AdminAccount {
  return { id: accountID, super_admin: false, brand_ids: [brand], permissions: [], ...overrides };
}

describe("notification delivery permissions", () => {
  it("requires mapped brand grants and denies flat or platform grants", () => {
    expect(deliveryPermissions(account({ permissions_by_brand: { [brand]: ["notification.view.brand", "notification.retry.brand"] } }), brand))
      .toEqual({ view: true, retry: true });
    expect(deliveryPermissions(account({ super_admin: true, permissions_by_brand: { [brand]: ["notification.view.brand", "notification.retry.brand"] }, platform_permissions: ["notification.view.platform"] }), brand))
      .toEqual({ view: false, retry: false });
    expect(deliveryPermissions(account({ super_admin: true, platform_permissions: ["notification.view.platform"] }), brand))
      .toEqual({ view: false, retry: false });
    expect(deliveryPermissions(account({ permissions: ["notification.view.platform"] }), brand).view).toBe(false);
    expect(deliveryPermissions(account({ brand_ids: [], permissions: ["notification.view.brand", "notification.retry.brand"] }), brand))
      .toEqual({ view: false, retry: false });
    expect(deliveryPermissions(account({ permissions: ["notification.view.brand"], permissions_by_brand: {} }), brand).view).toBe(false);
    expect(deliveryPermissions(account({ permissions: ["notification.view.brand", "notification.retry.brand"] }), brand)).toEqual({ view: false, retry: false });
  });
});

describe("notification delivery API", () => {
  it("lists a validated page with cookie credentials, brand header, and default pagination", async () => {
    const fetchImpl = vi.fn<typeof fetch>().mockResolvedValue(response({ items: [delivery()] }));
    await expect(createNotificationDeliveryApi(fetchImpl).list(brand)).resolves.toEqual([delivery()]);
    const [url, init] = fetchImpl.mock.calls[0];
    expect(url).toBe("/api/v1/admin/notification-deliveries?limit=20&offset=0");
    expect(init?.method).toBe("GET");
    expect(init?.credentials).toBe("include");
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    expect(new Headers(init?.headers).get("Accept")).toBe("application/json");
    await expect(createNotificationDeliveryApi(vi.fn<typeof fetch>()).list(brand, 101, 0))
      .rejects.toMatchObject({ status: 0, code: "INVALID_INPUT" });
    await expect(createNotificationDeliveryApi(vi.fn<typeof fetch>()).list(brand, 20, -1))
      .rejects.toBeInstanceOf(AdminApiError);
    await expect(createNotificationDeliveryApi(vi.fn<typeof fetch>().mockResolvedValue(response({ items: [
      delivery({ attempt_count: 0, last_error: null }),
    ] }))).list(brand)).resolves.toMatchObject([{ attempt_count: 0, last_error: null }]);
  });

  it("posts one retry with the supplied body and idempotency key and accepts only its first pending receipt", async () => {
    const body = reactive({ attempt_count: 2, reason: "operator requested retry" });
    const accepted = delivery({ last_error: "DELIVERY_TIMEOUT" });
    const fetchImpl = vi.fn<typeof fetch>().mockResolvedValue(response(accepted, 202));
    await expect(createNotificationDeliveryApi(fetchImpl).retry(brand, eventID, body, "stable-key"))
      .resolves.toEqual(accepted);
    expect(fetchImpl).toHaveBeenCalledTimes(1);
    const [url, init] = fetchImpl.mock.calls[0];
    expect(url).toBe(`/api/v1/admin/notification-deliveries/${eventID}/retry`);
    expect(init?.method).toBe("POST");
    expect(init?.credentials).toBe("include");
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    expect(new Headers(init?.headers).get("Idempotency-Key")).toBe("stable-key");
    expect(new Headers(init?.headers).get("Content-Type")).toBe("application/json");
    expect(JSON.parse(String(init?.body))).toEqual({ attempt_count: 2, reason: "operator requested retry" });
  });

  it("rejects malformed inputs before transport and treats network or malformed write receipts as uncertain status zero", async () => {
    const fetchImpl = vi.fn<typeof fetch>();
    const api = createNotificationDeliveryApi(fetchImpl);
    for (const body of [
      { attempt_count: 0, reason: "retry" },
      { attempt_count: 1.5, reason: "retry" },
      { attempt_count: 1, reason: "  " },
      { attempt_count: 1, reason: "x".repeat(501) },
    ]) {
      await expect(api.retry(brand, eventID, body, "key")).rejects.toMatchObject({ status: 0, code: "INVALID_INPUT" });
    }
    await expect(api.retry(brand.replace("00000000", "abcdef00").toUpperCase(), eventID, { attempt_count: 2, reason: "retry" }, "key"))
      .rejects.toMatchObject({ status: 0, code: "INVALID_INPUT" });
    await expect(api.retry(brand, eventID.replace("00000000", "abcdef00").toUpperCase(), { attempt_count: 2, reason: "retry" }, "key"))
      .rejects.toMatchObject({ status: 0, code: "INVALID_INPUT" });
    expect(fetchImpl).not.toHaveBeenCalled();

    await expect(createNotificationDeliveryApi(vi.fn<typeof fetch>().mockRejectedValue(new Error("offline")))
      .retry(brand, eventID, { attempt_count: 2, reason: "retry" }, "key"))
      .rejects.toMatchObject({ status: 0, code: "NETWORK_ERROR" });
    for (const invalid of [
      delivery({ event_id: otherBrand }),
      delivery({ brand_id: otherBrand }),
      delivery({ attempt_count: 3 }),
      delivery({ status: "failed", last_error: "unsafe message" }),
      delivery({ status: "unknown" as Delivery["status"] }),
      delivery({ status: "sent", sent_at: null }),
      delivery({ status: "pending", sent_at: at }),
      delivery({ status: "failed", attempt_count: 0, last_error: "DELIVERY_TIMEOUT" }),
      delivery({ status: "failed", last_error: null }),
      delivery({ status: "failed", last_error: "DELIVERY_TIMEOUT" }),
    ]) {
      await expect(createNotificationDeliveryApi(vi.fn<typeof fetch>().mockResolvedValue(response(invalid, 202)))
        .retry(brand, eventID, { attempt_count: 2, reason: "retry" }, "key"))
        .rejects.toMatchObject({ status: 0, code: "INVALID_RESPONSE" });
    }
  });

  it("validates every listed delivery and preserves definitive server errors", async () => {
    await expect(createNotificationDeliveryApi(vi.fn<typeof fetch>().mockResolvedValue(response({ items: [
      delivery({ last_error: "DELIVERY_TIMEOUT", status: "failed", sent_at: null }),
    ] }))).list(brand)).resolves.toHaveLength(1);
    await expect(createNotificationDeliveryApi(vi.fn<typeof fetch>().mockResolvedValue(response({ items: [
      delivery({ brand_id: otherBrand }),
    ] }))).list(brand)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(createNotificationDeliveryApi(vi.fn<typeof fetch>().mockResolvedValue(response({ items: [
      delivery({ brand_id: brand.replace("00000000", "abcdef00").toUpperCase() }),
    ] }))).list(brand)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(createNotificationDeliveryApi(vi.fn<typeof fetch>().mockResolvedValue(
      new Response(JSON.stringify({ success: false, error: { code: "CONFLICT", message: "changed" } }), { status: 409 }),
    )).retry(brand, eventID, { attempt_count: 2, reason: "retry" }, "key"))
      .rejects.toMatchObject({ status: 409, code: "CONFLICT" });
  });
});
