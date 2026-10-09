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
  content: {
    en: { title: "Welcome", body: "Your membership is ready. Welcome aboard." },
    "zh-CN": { title: "欢迎", body: "您的会员账户已准备就绪，欢迎加入。" },
  },
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

  it("parses published and corrected draws with exact snapshots and bounded historical results", async () => {
    const drawSnapshot = {
      en: { title: "Result {resource_id}", body: "Saved result for {resource_id}." },
      "zh-CN": { title: "结果 {resource_id}", body: "保存的结果：{resource_id}。" },
    };
    const published = {
      ...joined, event_type: "draw.result.published", template_key: "draw.result.published", template_version: 1,
      content: drawSnapshot,
      payload: { resource_id: resourceId, points: null, draw: {
        game_id: brandId, period_id: memberId, period_no: "20261009001",
        result: { regular: [3, 12, 28], special: [7], digits: [] },
        drawn_at: createdAt, previous_draw_id: null,
      } },
    };
    const corrected = {
      ...published, id: "cd333333-3333-4333-8333-333333333333", event_type: "draw.result.corrected", template_key: "draw.result.corrected",
      payload: { ...published.payload, draw: { ...published.payload.draw, previous_draw_id: "55555555-5555-4555-8555-555555555555", result: { regular: [], special: [], digits: [0, 1, 0] } } },
    };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok(page({ items: [published, corrected] })));
    await expect(createNotificationApi({ fetch: fetcher }).list()).resolves.toMatchObject({
      items: [{ payload: published.payload, content: drawSnapshot }, { payload: corrected.payload }],
    });
  });

  it("rejects malformed draw payload shapes, versions, snapshots, and results", async () => {
    const base = {
      ...joined, event_type: "draw.result.published", template_key: "draw.result.published", template_version: 1,
      content: {
        en: { title: "Result", body: "Result {resource_id}." },
        "zh-CN": { title: "结果", body: "结果 {resource_id}。" },
      },
      payload: { resource_id: resourceId, points: null, draw: {
        game_id: brandId, period_id: memberId, period_no: "period-01",
        result: { regular: [1, 2], special: [], digits: [] }, drawn_at: createdAt, previous_draw_id: null,
      } },
    };
    const invalid: unknown[] = [
      { ...base, template_version: 2, content: null },
      { ...base, content: { ...base.content, en: { title: "{points}", body: "Result {resource_id}." } } },
      { ...base, content: { ...base.content, "zh-CN": { title: "结果", body: "结果。" } } },
      { ...base, payload: { ...base.payload, private: true } },
      { ...base, payload: { ...base.payload, points: "0" } },
      { ...base, payload: { ...base.payload, draw: { ...base.payload.draw, model: "M_SELECT_N" } } },
      { ...base, payload: { ...base.payload, draw: { ...base.payload.draw, period_no: " ".repeat(2) } } },
      { ...base, payload: { ...base.payload, draw: { ...base.payload.draw, period_no: "界".repeat(27) } } },
      { ...base, payload: { ...base.payload, draw: { ...base.payload.draw, result: { regular: [], special: [], digits: [] } } } },
      { ...base, payload: { ...base.payload, draw: { ...base.payload.draw, result: { regular: [1.5], special: [], digits: [] } } } },
      { ...base, payload: { ...base.payload, draw: { ...base.payload.draw, result: { regular: [1_000_001], special: [], digits: [] } } } },
      { ...base, payload: { ...base.payload, draw: { ...base.payload.draw, result: { regular: [], special: [], digits: [10] } } } },
      { ...base, payload: { ...base.payload, draw: { ...base.payload.draw, result: { regular: Array(11).fill(1), special: [], digits: [] } } } },
      { ...base, payload: { ...base.payload, draw: { ...base.payload.draw, result: { regular: [1], special: [], digits: [2] } } } },
      { ...base, payload: { ...base.payload, draw: { ...base.payload.draw, result: { regular: [1], extra: [], special: [], digits: [] } } } },
    ];
    const corrected = { ...base, event_type: "draw.result.corrected", template_key: "draw.result.corrected" };
    invalid.push(
      { ...corrected, payload: { ...corrected.payload, draw: { ...corrected.payload.draw, previous_draw_id: null } } },
      { ...corrected, payload: { ...corrected.payload, draw: { ...corrected.payload.draw, previous_draw_id: resourceId } } },
      { ...base, payload: { ...base.payload, draw: { ...base.payload.draw, previous_draw_id: memberId } } },
    );
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () => ok(page({ items: [invalid.shift()] })));
    const api = createNotificationApi({ fetch: fetcher });
    const invalidCount = invalid.length;
    for (let i = 0; i < invalidCount; i++) await expect(api.list()).rejects.toMatchObject({ status: 502, code: "invalid_response" });
  });

  it.each([undefined, null])("rejects notifications missing content at template version 1 (%s)", async (content) => {
    const row = { ...joined, content };
    const api = createNotificationApi({
      fetch: vi.fn<typeof fetch>().mockResolvedValue(ok(page({ items: [row] }))),
    });
    await expect(api.list()).rejects.toMatchObject({ status: 502, code: "invalid_response" });
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
      content: {
        en: { title: "Recharge confirmed", body: "Recharge confirmed: {points} points." },
        "zh-CN": { title: "充值已确认", body: "充值已确认：{points} 积分。" },
      },
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
      content: {
        en: { title: "Prize credit recorded", body: "Historical prize: {points}." },
        "zh-CN": { title: "派奖入账记录", body: "历史奖金：{points}。" },
      },
      payload: { resource_id: resourceId, points: "9223372036854775807" },
    };
    const reversed = {
      ...joined,
      id: "cd333333-3333-4333-8333-333333333333",
      event_type: "bet.order.prize_reversed",
      template_key: "bet.order.prize_reversed",
      content: {
        en: { title: "Prize reversal recorded", body: "Historical reversal: {points}." },
        "zh-CN": { title: "奖金冲正记录", body: "历史冲正：{points}。" },
      },
      payload: { resource_id: resourceId, points: "9007199254740993" },
    };
    const invalidItems = [
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
    for (let index = 0; index < invalidItems.length; index++) {
      await expect(api.list()).rejects.toMatchObject({ status: 502, code: "invalid_response" });
    }
  });

  it("accepts all reward states with a template version 1 snapshot and rejects malformed points or private payload fields", async () => {
    const states = ["granted", "revocation_pending", "revoked"] as const;
    const rows = states.map((state, index) => ({
      ...joined,
      id: `00000000-0000-4000-8000-00000000001${index}`,
      event_type: `reward.order.${state}`,
      template_key: `reward.order.${state}`,
      template_version: 1,
      content: {
        en: { title: "Gift event recorded", body: `Historical gift event: {points}. ${state}.` },
        "zh-CN": { title: "赠送记录", body: `历史赠送事件：{points}。${state}。` },
      },
      payload: { resource_id: resourceId, points: "9223372036854775807" },
    }));
    const invalid: unknown[] = [
      { ...rows[0], event_type: "reward.order.revoked" },
      { ...rows[1], template_key: "reward.order.granted" },
      { ...rows[0], content: null },
      { ...rows[2], content: undefined },
      ...[0, -1, 1, "0", "-1", "01", "9223372036854775808", null, undefined].map((points) => ({
        ...rows[0], payload: { resource_id: resourceId, points },
      })),
      ...["actor_id", "reason", "ledger_entry_id", "member_id", "status"].map((key) => ({
        ...rows[0], payload: { ...rows[0].payload, [key]: "private" },
      })),
    ];
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(ok(page({ items: rows })))
      .mockImplementation(async () => ok(page({ items: [invalid.shift()] })));
    const api = createNotificationApi({ fetch: fetcher });
    await expect(api.list()).resolves.toMatchObject({
      items: states.map((state) => ({
        event_type: `reward.order.${state}`,
        template_key: `reward.order.${state}`,
        template_version: 1,
        payload: { resource_id: resourceId, points: "9223372036854775807" },
      })),
    });
    for (let index = 0; index < invalid.length; index++) {
      await expect(api.list()).rejects.toMatchObject({ status: 502, code: "invalid_response" });
    }
  });

  it.each(["commission.adjusted", "commission.corrected"] as const)("accepts signed nonzero %s points but rejects noncanonical values", async (event) => {
    const adjusted = {
      ...joined,
      event_type: event,
      template_key: event,
      template_version: 2,
      content: {
        en: { title: "Commission adjustment recorded", body: "Historical adjustment: {points}." },
        "zh-CN": { title: "佣金调整记录", body: "历史调整：{points}。" },
      },
      payload: { resource_id: resourceId, points: "-9223372036854775808" },
    };
    const invalid = ["0", "-0", "+1", "01", "-01", "9223372036854775808", "-9223372036854775809"];
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(ok(page({ items: [adjusted] })))
      .mockImplementation(async () => ok(page({ items: [{ ...adjusted, payload: { ...adjusted.payload, points: invalid.shift() } }] })));
    const api = createNotificationApi({ fetch: fetcher });
    await expect(api.list()).resolves.toMatchObject({ items: [{ payload: { points: "-9223372036854775808" } }] });
    for (let index = 0; index < 7; index++) {
      await expect(api.list()).rejects.toMatchObject({ status: 502, code: "invalid_response" });
    }
  });

  it("exposes only the correction target UUID and signed points for commission.corrected", async () => {
    const corrected = {
      ...joined,
      event_type: "commission.corrected",
      template_key: "commission.corrected",
      template_version: 1,
      content: {
        en: { title: "Commission correction recorded", body: "Historical correction: {points}." },
        "zh-CN": { title: "佣金更正记录", body: "历史更正：{points}。" },
      },
      payload: { resource_id: resourceId, points: "-1" },
    };
    const privateFields = ["member_id", "ledger_entry_id", "target_id", "audit_log_id", "version"];
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async (_input, _init) => {
      const field = privateFields.shift();
      return ok(page({ items: [{ ...corrected, payload: field ? { ...corrected.payload, [field]: resourceId } : corrected.payload }] }));
    });
    const api = createNotificationApi({ fetch: fetcher });
    for (let index = 0; index < 5; index++) {
      await expect(api.list()).rejects.toMatchObject({ status: 502, code: "invalid_response" });
    }
    await expect(api.list()).resolves.toMatchObject({ items: [{ payload: { resource_id: resourceId, points: "-1" } }] });
  });

  it("rejects commission notifications missing content regardless of template version", async () => {
    const variants = ["commission.paid", "commission.adjusted", "commission.corrected"].flatMap(event => [null, undefined].map(content => ({
      ...joined, event_type: event, template_key: event, template_version: 1, content,
      payload: { resource_id: resourceId, points: event === "commission.paid" ? "1" : "-1" },
    })));
    const fetcher = vi.fn<typeof fetch>(async () => ok(page({ items: [variants.shift()] })));
    const api = createNotificationApi({ fetch: fetcher });
    for (let i = 0; i < 6; i++) await expect(api.list()).rejects.toMatchObject({ status: 502, code: "invalid_response" });
  });

  it("accepts six withdrawal snapshot events with positive int64 points and rejects missing or private facts", async () => {
    const states = ["reviewing", "processing", "paid", "rejected", "failed", "cancelled"] as const;
    const events = states.map((state, index) => ({
      ...joined,
      id: `00000000-0000-4000-8000-00000000000${index + 1}`,
      event_type: `withdrawal.order.${state}`,
      template_key: `withdrawal.order.${state}`,
      template_version: 2,
      content: {
        en: { title: "Withdrawal status recorded", body: `Historical withdrawal status: ${state}. Points involved: {points}.` },
        "zh-CN": { title: "提现状态记录", body: `历史提现状态：${state}，涉及 {points} 积分。` },
      },
      payload: { resource_id: resourceId, points: "9223372036854775807" },
    }));
    const invalid = [
      { ...events[0], content: null },
      { ...events[1], content: undefined },
      { ...events[2], template_version: 1, content: null },
      { ...events[2], payload: { resource_id: resourceId, points: "0" } },
      { ...events[3], payload: { resource_id: resourceId, points: "9223372036854775808" } },
      { ...events[4], payload: { resource_id: resourceId, points: "1", reason: "private" } },
      { ...events[5], payload: { resource_id: resourceId, points: "1", bank_account: "private" } },
      { ...events[0], event_type: "withdrawal.order.unknown", template_key: "withdrawal.order.unknown" },
    ];
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(ok(page({ items: events })))
      .mockImplementation(async () => ok(page({ items: [invalid.shift()] })));
    const api = createNotificationApi({ fetch: fetcher });

    await expect(api.list()).resolves.toMatchObject({
      items: states.map((state) => ({
        event_type: `withdrawal.order.${state}`,
        template_key: `withdrawal.order.${state}`,
        payload: { resource_id: resourceId, points: "9223372036854775807" },
      })),
    });
    for (let index = 0; index < 8; index++) {
      await expect(api.list()).rejects.toMatchObject({ status: 502, code: "invalid_response" });
    }
  });

  it("accepts immutable bilingual snapshots and preserves template versions as safe integers", async () => {
    const custom = {
      ...joined,
      event_type: "recharge.confirmed",
      template_key: "recharge.confirmed",
      template_version: 9007199254740991,
      content: {
        en: { title: "  Earned {points}  ".trim(), body: "Added {points} points; total {points}. Ref {resource_id}." },
        "zh-CN": { title: "到账 {points}", body: "已到账 {points} 积分，再次核对 {points}。编号 {resource_id}。" },
      },
      payload: { resource_id: resourceId, points: "9223372036854775807" },
    };
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(ok(page({ items: [{ ...custom, template_version: 2 }] })))
      .mockResolvedValueOnce(ok(page({ items: [custom] })));
    const api = createNotificationApi({ fetch: fetcher });

    await expect(api.list()).resolves.toMatchObject({
      items: [{ template_version: 2, content: custom.content }],
    });
    await expect(api.list()).resolves.toMatchObject({
      items: [{ template_version: 9007199254740991, content: custom.content }],
    });
  });

  it("fails closed on missing snapshots for new versions and malformed snapshot content", async () => {
    const item = {
      ...joined,
      event_type: "recharge.confirmed",
      template_key: "recharge.confirmed",
      payload: { resource_id: resourceId, points: "1" },
      template_version: 2,
      content: {
        en: { title: "Credit", body: "Credit: {points}." },
        "zh-CN": { title: "到账", body: "到账：{points}。" },
      },
    };
    const invalidItems: unknown[] = [
      { ...item, content: undefined },
      { ...item, content: null },
      { ...item, template_version: 0 },
      { ...item, template_version: 1.5 },
      { ...item, template_version: 9007199254740992 },
      { ...item, content: { ...item.content, private: "secret" } },
      { ...item, content: { ...item.content, en: { ...item.content.en, html: "private" } } },
      { ...item, content: { ...item.content, en: { title: "Credit", body: "Credit {points} <script>alert(1)</script>" } } },
      { ...item, content: { ...item.content, en: { title: "Credit", body: "Credit {points} https://example.test" } } },
      { ...item, content: { ...item.content, en: { title: "Credit", body: "Credit {points} javascript:alert(1)" } } },
      { ...item, content: { ...item.content, en: { title: "Credit", body: "Credit {points} data:text/html" } } },
      { ...item, content: { ...item.content, en: { title: "Credit", body: "Credit {points} www.example.test" } } },
      { ...item, content: { ...item.content, en: { title: "Credit", body: "Credit {missing}. {points}" } } },
      { ...item, content: { ...item.content, en: { title: "Credit", body: "Credit {{points}}" } } },
      { ...item, content: { ...item.content, en: { title: " Credit", body: "Credit {points}" } } },
      { ...item, content: { ...item.content, en: { title: "Credit", body: "Credit only." } } },
      { ...item, content: { ...item.content, "zh-CN": { title: "到账" } } },
    ];
    const invalidCount = invalidItems.length;
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () => {
      const invalid = invalidItems.shift();
      return ok(page({ items: [invalid] }));
    });
    const api = createNotificationApi({ fetch: fetcher });
    for (let index = 0; index < invalidCount; index++) {
      await expect(api.list()).rejects.toMatchObject({ status: 502, code: "invalid_response" });
    }
  });

  it("accepts canonical snapshot length limits, while rejecting excess UTF-8 bytes and joined points", async () => {
    const base = {
      ...joined,
      event_type: "recharge.confirmed",
      template_key: "recharge.confirmed",
      payload: { resource_id: resourceId, points: "1" },
      template_version: 2,
    };
    const content = {
      en: { title: "T".repeat(120), body: `${"界".repeat(399)}{points}` },
      "zh-CN": { title: "标题", body: "积分 {points}" },
    };
    content.en.body = `${"x".repeat(1192)}{points}`;
    const joinedWithContent = {
      ...joined,
      template_version: 2,
      content: {
        en: { title: "Welcome", body: "Welcome aboard." },
        "zh-CN": { title: "欢迎", body: "欢迎加入。" },
      },
    };
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(ok(page({ items: [{ ...base, content }] })))
      .mockResolvedValueOnce(ok(page({ items: [{ ...base, content: { ...content, en: { ...content.en, body: `${"x".repeat(1193)}{points}` } } }] })))
      .mockResolvedValueOnce(ok(page({ items: [{ ...base, content: { ...content, en: { ...content.en, title: "界".repeat(41) } } }] })))
      .mockResolvedValueOnce(ok(page({ items: [{ ...joinedWithContent, content: { ...joinedWithContent.content, en: { title: "Welcome {points}", body: "Welcome aboard." } } }] })));
    const api = createNotificationApi({ fetch: fetcher });
    await expect(api.list()).resolves.toMatchObject({ items: [{ content }] });
    await expect(api.list()).rejects.toMatchObject({ status: 502, code: "invalid_response" });
    await expect(api.list()).rejects.toMatchObject({ status: 502, code: "invalid_response" });
    await expect(api.list()).rejects.toMatchObject({ status: 502, code: "invalid_response" });
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
