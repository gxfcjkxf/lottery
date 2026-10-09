import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import {
  createNotificationTemplatesApi, notificationTemplatePermissions, notificationTemplateKeys,
  type NotificationTemplate, type NotificationTemplateContent, type NotificationTemplateRevision,
} from "./notification-templates-api";

const brand = "00000000-0000-4000-8000-000000000001";
const accountId = "00000000-0000-4000-8000-000000000002";
const auditId = "00000000-0000-4000-8000-000000000003";
const at = "2026-10-07T02:00:00Z";
const content: NotificationTemplateContent = {
  en: { title: "Points update", body: "You received {points} points for {resource_id}." },
  "zh-CN": { title: "积分更新", body: "您获得 {points} 积分，编号 {resource_id}。" },
};
function template(overrides: Partial<NotificationTemplate> = {}): NotificationTemplate {
  return { brand_id: brand, key: "recharge.confirmed", version: 1, content, updated_at: at, audit_log_id: null, ...overrides };
}
function response(data: unknown, status = 200): Response {
  return new Response(JSON.stringify({ success: true, data }), { status, headers: { "Content-Type": "application/json" } });
}
function account(overrides: Partial<AdminAccount> = {}): AdminAccount {
  return { id: accountId, super_admin: false, brand_ids: [brand], permissions: [], ...overrides };
}

describe("notification template permissions", () => {
  it("checks view and write grants independently and requires brand membership", () => {
    expect(notificationTemplatePermissions(account({ permissions_by_brand: { [brand]: ["notification_template.view.brand", "notification_template.write.brand"] } }), brand))
      .toEqual({ view: true, write: true });
    expect(notificationTemplatePermissions(account({ permissions_by_brand: {}, permissions: ["notification_template.view.brand"] }), brand).view).toBe(false);
    expect(notificationTemplatePermissions(account({ platform_permissions: ["notification_template.view.platform"] }), brand).view).toBe(false);
    expect(notificationTemplatePermissions(account({ brand_ids: [], platform_permissions: ["notification_template.view.platform"] }), brand).view).toBe(false);
    expect(notificationTemplatePermissions(account({ permissions: ["notification_template.view.brand", "notification_template.write.brand"] }), brand))
      .toEqual({ view: false, write: false });
    expect(notificationTemplatePermissions(account({ super_admin: true, permissions: ["notification_template.write.brand"] }), brand).write).toBe(false);
    expect(notificationTemplatePermissions(account({ permissions: ["notification_template.write.brand"], brand_ids: [] }), brand))
      .toEqual({ view: false, write: false });
    expect(notificationTemplatePermissions(account({ permissions: ["notification.view.brand", "notification.write.brand"] }), brand))
      .toEqual({ view: false, write: false });
    expect(notificationTemplatePermissions(account({ permissions: ["notification_template.view.brand", "notification_template.write.brand"] }), brand))
      .toEqual({ view: false, write: false });
  });
});

describe("notification templates API", () => {
  it("lists the full endpoint with cookie auth and validates the brand scoped snapshot", async () => {
    const fetchImpl = vi.fn<typeof fetch>().mockResolvedValue(response({ items: [template()] }));
    await expect(createNotificationTemplatesApi(fetchImpl).list(brand)).resolves.toEqual([template()]);
    const [url, init] = fetchImpl.mock.calls[0];
    expect(url).toBe("/api/v1/admin/notification-templates");
    expect(init?.method).toBe("GET");
    expect(init?.credentials).toBe("same-origin");
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    expect(new Headers(init?.headers).get("Accept")).toBe("application/json");
    expect(notificationTemplateKeys).toHaveLength(22);
    expect(notificationTemplateKeys).toEqual([
      "member.joined", "recharge.confirmed", "bet.order.placed", "bet.order.cancelled",
      "bet.order.judged_cancelled", "bet.order.abnormal", "bet.order.won", "bet.order.prize_reversed",
      "reward.order.granted", "reward.order.revocation_pending", "reward.order.revoked",
      "commission.adjusted", "commission.corrected", "commission.paid",
      "withdrawal.order.reviewing", "withdrawal.order.processing", "withdrawal.order.paid",
      "withdrawal.order.rejected", "withdrawal.order.failed", "withdrawal.order.cancelled",
      "draw.result.published", "draw.result.corrected",
    ]);
    await expect(createNotificationTemplatesApi(vi.fn<typeof fetch>().mockResolvedValue(response({ items: [template({ brand_id: accountId })] }))).list(brand))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(createNotificationTemplatesApi(vi.fn<typeof fetch>().mockResolvedValue(response({ items: [template(), template()] }))).list(brand))
      .rejects.toBeInstanceOf(AdminApiError);
  });

  it("allows editable draw copy with only the existing placeholders and requires the period reference", async () => {
    const drawContent: NotificationTemplateContent = {
      en: { title: "Historical result", body: "The saved result for {resource_id} is available." },
      "zh-CN": { title: "历史开奖结果", body: "期号 {resource_id} 的保存结果已提供。" },
    };
    const keys = ["draw.result.published", "draw.result.corrected"] as const;
    const fetchImpl = vi.fn<typeof fetch>().mockImplementation(async (input) => {
      const key = String(input).split("/").at(-1) as typeof keys[number];
      return response(template({ key, version: 2, content: drawContent, audit_log_id: auditId }));
    });
    const api = createNotificationTemplatesApi(fetchImpl);
    for (const [index, key] of keys.entries()) {
      await expect(api.put(key, brand, { version: 1, content: drawContent, reason: "draw copy review" }, `draw-template-00${index + 1}`))
        .resolves.toMatchObject({ key, content: drawContent });
    }
    const invalid: NotificationTemplateContent[] = [
      { ...drawContent, en: { title: "Result {points}", body: drawContent.en.body } },
      { ...drawContent, "zh-CN": { title: "结果", body: "保存结果已提供。" } },
      { ...drawContent, en: { title: "Result", body: "Result {drawn_at}." } },
      { ...drawContent, en: { title: "Result", body: "Result {resource_id} {points}." } },
    ];
    for (const content of invalid) {
      await expect(api.put("draw.result.published", brand, { version: 1, content, reason: "draw copy review" }, "draw-template-invalid"))
        .rejects.toMatchObject({ status: 400, code: "INVALID_INPUT" });
    }
    expect(fetchImpl).toHaveBeenCalledTimes(2);
  });

  it("reads history using its template route and validates first and later revision audit rules", async () => {
    const initial: NotificationTemplateRevision = {
      id: accountId, brand_id: brand, key: "recharge.confirmed", version: 1, content,
      changed_by: null, reason: "initial", audit_log_id: null, created_at: at,
    };
    const fetchImpl = vi.fn<typeof fetch>().mockResolvedValue(response({ items: [initial] }));
    await expect(createNotificationTemplatesApi(fetchImpl).history("recharge.confirmed", brand)).resolves.toEqual([initial]);
    expect(fetchImpl.mock.calls[0][0]).toBe("/api/v1/admin/notification-templates/recharge.confirmed/history?limit=20&offset=0");
    const later = { ...initial, id: auditId, version: 2, changed_by: accountId, audit_log_id: auditId };
    await expect(createNotificationTemplatesApi(vi.fn<typeof fetch>().mockResolvedValue(response({ items: [later] })))
      .history("recharge.confirmed", brand)).resolves.toEqual([later]);
    await expect(createNotificationTemplatesApi(vi.fn<typeof fetch>().mockResolvedValue(response({ items: [{ ...later, audit_log_id: null }] })))
      .history("recharge.confirmed", brand)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(createNotificationTemplatesApi(vi.fn<typeof fetch>()).history("bogus" as never, brand))
      .rejects.toMatchObject({ status: 400, code: "INVALID_INPUT" });
  });

  it("puts a canonical frozen request with the supplied idempotency key and validates the receipt", async () => {
    const body = { version: 3, content, reason: "approved for updated wording" };
    const receipt = template({ version: 4, content, audit_log_id: auditId });
    const fetchImpl = vi.fn<typeof fetch>().mockResolvedValue(response(receipt));
    await expect(createNotificationTemplatesApi(fetchImpl).put("recharge.confirmed", brand, body, "template-key-001"))
      .resolves.toEqual(receipt);
    const [url, init] = fetchImpl.mock.calls[0];
    expect(url).toBe("/api/v1/admin/notification-templates/recharge.confirmed");
    expect(init?.method).toBe("PUT");
    expect(init?.credentials).toBe("same-origin");
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    expect(new Headers(init?.headers).get("Idempotency-Key")).toBe("template-key-001");
    expect(JSON.parse(String(init?.body))).toEqual(body);
    await expect(createNotificationTemplatesApi(vi.fn<typeof fetch>().mockResolvedValue(response(template({ version: 3, audit_log_id: auditId }))))
      .put("recharge.confirmed", brand, body, "template-key-001"))
      .rejects.toMatchObject({ status: 0, code: "INVALID_RESPONSE" });
  });

  it("rejects noncanonical, unsafe, malformed, or incomplete content without a request", async () => {
    const fetchImpl = vi.fn<typeof fetch>();
    const api = createNotificationTemplatesApi(fetchImpl);
    const badContents: unknown[] = [
      { ...content, en: { title: "https:bad", body: content.en.body } },
      { ...content, en: { title: "bad\ud800", body: content.en.body } },
      { ...content, en: { title: " title", body: content.en.body } },
      { ...content, en: { title: "x".repeat(121), body: content.en.body } },
      { ...content, en: { title: "é".repeat(61), body: content.en.body } },
      { ...content, en: { title: "<b>bad</b>", body: content.en.body } },
      { ...content, en: { title: "Visit www.example.com", body: content.en.body } },
      { ...content, en: { title: "title", body: "{unknown} {points}" } },
      { ...content, en: { title: "title", body: "{reason} {points}" } },
      { ...content, en: { title: "title", body: "{points" } },
      { ...content, en: { title: "title", body: "missing points" } },
      { ...content, en: { title: "title", body: `x${"x".repeat(1198)}\u0001` } },
      { ...content, "zh-CN": { title: "x", body: "{points}", extra: true } },
    ];
    for (const invalidContent of badContents) {
      await expect(api.put("recharge.confirmed", brand, { version: 1, content: invalidContent as NotificationTemplateContent, reason: "reason" }, "template-key-001"))
        .rejects.toMatchObject({ status: 400, code: "INVALID_INPUT" });
    }
    const joined: NotificationTemplateContent = {
      en: { title: "Welcome {points}", body: "Welcome" },
      "zh-CN": { title: "欢迎", body: "欢迎" },
    };
    await expect(api.put("member.joined", brand, { version: 1, content: joined, reason: "reason" }, "template-key-001"))
      .rejects.toMatchObject({ status: 400, code: "INVALID_INPUT" });
    expect(fetchImpl).not.toHaveBeenCalled();
  });

  it("accepts all six withdrawal templates but rejects unsupported event keys and reason placeholders", async () => {
    const withdrawalContent: NotificationTemplateContent = {
      en: { title: "Withdrawal status recorded", body: "Historical status: {points}." },
      "zh-CN": { title: "提现状态记录", body: "历史状态：{points} 积分。" },
    };
    const withdrawalKeys = [
      "withdrawal.order.reviewing", "withdrawal.order.processing", "withdrawal.order.paid",
      "withdrawal.order.rejected", "withdrawal.order.failed", "withdrawal.order.cancelled",
    ] as const;
    const fetchImpl = vi.fn<typeof fetch>().mockImplementation(async (input) => {
      const key = String(input).split("/").at(-1) as typeof withdrawalKeys[number];
      return response(template({ key, version: 2, content: withdrawalContent, audit_log_id: auditId }));
    });
    const api = createNotificationTemplatesApi(fetchImpl);
    for (const [index, key] of withdrawalKeys.entries()) {
      await expect(api.put(key, brand, {
        version: 1, content: withdrawalContent, reason: "wording review",
      }, `template-key-00${index + 1}`)).resolves.toMatchObject({ key, version: 2 });
      expect(fetchImpl.mock.calls[index][0]).toBe(`/api/v1/admin/notification-templates/${key}`);
    }
    await expect(api.put("withdrawal.order.legacy" as never, brand, {
      version: 1, content: withdrawalContent, reason: "wording review",
    }, "template-key-007")).rejects.toMatchObject({ status: 400, code: "INVALID_INPUT" });
    await expect(api.put("withdrawal.order.reviewing", brand, {
      version: 1,
      content: { ...withdrawalContent, en: { title: "Status", body: "Historical {reason} {points}." } },
      reason: "wording review",
    }, "template-key-008")).rejects.toMatchObject({ status: 400, code: "INVALID_INPUT" });
    expect(fetchImpl).toHaveBeenCalledTimes(6);
  });

  it("accepts all commission templates while keeping their fixed facts outside editable copy", async () => {
    const commissionContent: NotificationTemplateContent = {
      en: { title: "Custom commission wording", body: "Recorded {points} points for {resource_id}." },
      "zh-CN": { title: "自定义佣金文案", body: "记录 {points} 积分，编号 {resource_id}。" },
    };
    const keys = ["commission.adjusted", "commission.corrected", "commission.paid"] as const;
    const fetchImpl = vi.fn<typeof fetch>().mockImplementation(async (input) => {
      const key = String(input).split("/").at(-1) as typeof keys[number];
      return response(template({ key, version: 2, content: commissionContent, audit_log_id: auditId }));
    });
    const api = createNotificationTemplatesApi(fetchImpl);
    for (const [index, key] of keys.entries()) {
      await expect(api.put(key, brand, { version: 1, content: commissionContent, reason: "copy review" }, `commission-template-00${index + 1}`))
        .resolves.toMatchObject({ key, content: commissionContent });
    }
    expect(fetchImpl).toHaveBeenCalledTimes(3);
  });

  it("accepts exactly the three reward template keys and requires points in both localized bodies", async () => {
    const rewardContent: NotificationTemplateContent = {
      en: { title: "Edited reward wording", body: "Recorded {points} gift points for {resource_id}." },
      "zh-CN": { title: "已编辑赠送文案", body: "记录 {points} 赠送积分，编号 {resource_id}。" },
    };
    const keys = ["reward.order.granted", "reward.order.revocation_pending", "reward.order.revoked"] as const;
    const fetchImpl = vi.fn<typeof fetch>().mockImplementation(async (input) => {
      const key = String(input).split("/").at(-1) as typeof keys[number];
      return response(template({ key, version: 2, content: rewardContent, audit_log_id: auditId }));
    });
    const api = createNotificationTemplatesApi(fetchImpl);
    for (const [index, key] of keys.entries()) {
      await expect(api.put(key, brand, { version: 1, content: rewardContent, reason: "copy review" }, `reward-template-00${index + 1}`))
        .resolves.toMatchObject({ key, content: rewardContent });
    }
    await expect(api.put("reward.order.unknown" as never, brand,
      { version: 1, content: rewardContent, reason: "copy review" }, "reward-template-004"))
      .rejects.toMatchObject({ status: 400, code: "INVALID_INPUT" });
    for (const key of keys) {
      const invalidContent: NotificationTemplateContent = {
        ...rewardContent,
        en: { ...rewardContent.en, body: "No points placeholder." },
      };
      await expect(api.put(key, brand, { version: 1, content: invalidContent, reason: "copy review" }, "reward-template-005"))
        .rejects.toMatchObject({ status: 400, code: "INVALID_INPUT" });
    }
    expect(fetchImpl).toHaveBeenCalledTimes(3);
  });

  it("preserves backend conflict and permission error codes", async () => {
    const errorResponse = new Response(JSON.stringify({ success: false, error: { code: "VERSION_CONFLICT", message: "changed" } }), { status: 409 });
    await expect(createNotificationTemplatesApi(vi.fn<typeof fetch>().mockResolvedValue(errorResponse))
      .put("recharge.confirmed", brand, { version: 1, content, reason: "reason" }, "template-key-001"))
      .rejects.toMatchObject({ status: 409, code: "VERSION_CONFLICT" });
  });
});
