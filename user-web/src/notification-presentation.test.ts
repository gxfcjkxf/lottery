import { describe, expect, it } from "vitest";
import {
  formatIntegerString,
  renderNotification,
  type NotificationPresentationItem,
} from "./notification-presentation";
import type { NotificationEventType } from "./notification-api";

const events = [
  "member.joined",
  "recharge.confirmed",
  "bet.order.placed",
  "bet.order.cancelled",
  "bet.order.judged_cancelled",
  "bet.order.abnormal",
  "bet.order.won",
  "bet.order.prize_reversed",
  "withdrawal.order.reviewing",
  "withdrawal.order.processing",
  "withdrawal.order.paid",
  "withdrawal.order.rejected",
  "withdrawal.order.failed",
  "withdrawal.order.cancelled",
] as const;

function item(event_type: NotificationEventType, template_version = 1): NotificationPresentationItem {
  const withdrawalState = event_type.startsWith("withdrawal.order.") ? event_type.slice("withdrawal.order.".length) : null;
  const withdrawalChinese: Record<string, string> = {
    reviewing: "审核中", processing: "提现中", paid: "已提现", rejected: "已驳回", failed: "失败", cancelled: "已取消",
  };
  return {
    event_type,
    template_key: event_type,
    template_version,
    ...(withdrawalState ? { content: {
      en: { title: "Withdrawal status recorded", body: `Historical withdrawal status: ${withdrawalState}. Points involved: {points}. This is an internal points record, not proof of an external transfer. Check the withdrawal order for its current state.` },
      "zh-CN": { title: "提现状态记录", body: `历史提现状态：${withdrawalChinese[withdrawalState]}，涉及 {points} 积分。此为内部积分记录，不证明外部转账；请查询提现订单的最新状态。` },
    } } : {}),
    payload: {
      resource_id: "business-reference-42",
      points: event_type === "member.joined" ? null : "900719925474099312345",
    },
    created_at: "2026-10-06T00:00:00.000Z",
  };
}

describe("notification presentation", () => {
  it.each(["zh", "en"] as const)("renders fixed copy for every %s event", (locale) => {
    for (const event of events) {
      const rendered = renderNotification(item(event), locale);
      expect(rendered.title.length).toBeGreaterThan(0);
      expect(rendered.body.length).toBeGreaterThan(0);
      expect(rendered.createdAt.length).toBeGreaterThan(0);
    }
  });

  it("does not claim a win or payout for an abnormal order", () => {
    const english = renderNotification(item("bet.order.abnormal"), "en");
    const chinese = renderNotification(item("bet.order.abnormal"), "zh");
    expect(english.body).not.toMatch(/win|payout|prize/i);
    expect(chinese.body).not.toMatch(/中奖|派奖/);
  });

  it("presents prize credits and reversals as retained historical ledger events", () => {
    const wonEn = renderNotification(item("bet.order.won"), "en");
    const reversedEn = renderNotification(item("bet.order.prize_reversed"), "en");
    expect(wonEn.body).toContain("Historical record");
    expect(wonEn.body).toContain("were credited");
    expect(wonEn.body).toContain("not your current wallet balance");
    expect(wonEn.body).toContain("or a guaranteed final outcome");
    expect(wonEn.body).toContain("separate prize event");
    expect(wonEn.body).toContain("this record is retained");
    expect(reversedEn.body).toContain("full original prize amount");
    expect(reversedEn.body).toContain("not your current wallet balance");
    expect(reversedEn.body).toContain("separate event");
    expect(reversedEn.body).toContain("this record is retained");

    const wonZh = renderNotification(item("bet.order.won"), "zh");
    const reversedZh = renderNotification(item("bet.order.prize_reversed"), "zh");
    expect(wonZh.body).toContain("历史记录");
    expect(wonZh.body).toContain("不代表当前钱包余额");
    expect(wonZh.body).toContain("不保证最终结果");
    expect(wonZh.body).toContain("单独的奖金事件");
    expect(wonZh.body).toContain("此记录会保留");
    expect(reversedZh.body).toContain("原奖金全额");
    expect(reversedZh.body).toContain("已冲回");
    expect(reversedZh.body).toContain("不代表当前钱包余额");
    expect(reversedZh.body).toContain("单独事件");
    expect(reversedZh.body).toContain("此记录会保留");
    expect(wonEn.protectedNote).toBeNull();
    expect(reversedEn.protectedNote).toBeNull();
  });

  it.each([
    ["withdrawal.order.reviewing", "reviewing", "审核中"],
    ["withdrawal.order.processing", "processing", "提现中"],
    ["withdrawal.order.paid", "paid", "已提现"],
    ["withdrawal.order.rejected", "rejected", "已驳回"],
    ["withdrawal.order.failed", "failed", "失败"],
    ["withdrawal.order.cancelled", "cancelled", "已取消"],
  ] as const)("keeps protected bilingual historical context for %s snapshots", (event, state, stateZh) => {
    const snapshot = {
      en: { title: "Operator title", body: "Operator copy: {points}." },
      "zh-CN": { title: "运营标题", body: "运营内容：{points}。" },
    };
    const custom = { ...item(event, 2), content: snapshot };
    const renderedEn = renderNotification(custom, "en");
    const renderedZh = renderNotification(custom, "zh");

    expect(renderedEn.body).toBe("Operator copy: 900,719,925,474,099,312,345.");
    expect(renderedEn.protectedNote).toContain(`Historical withdrawal status: ${state}.`);
    expect(renderedEn.protectedNote).toContain("not proof of an external transfer");
    expect(renderedEn.protectedNote).toContain("Check the withdrawal order for its current state.");
    expect(renderedZh.body).toBe("运营内容：900,719,925,474,099,312,345。");
    expect(renderedZh.protectedNote).toContain(`历史提现状态：${stateZh}`);
    expect(renderedZh.protectedNote).toContain("不证明外部转账");
    expect(renderedZh.protectedNote).toContain("请查询提现订单的最新状态");
    expect(renderedEn.protectedNote).not.toMatch(/bank|crypto|virtual currency|payment|transfer succeeded/i);
    expect(() => renderNotification({ ...item(event), content: null }, "en")).toThrow(/require an immutable content snapshot/);
  });

  it("renders snapshot copy by locale and replaces every supported placeholder exactly", () => {
    const snapshot = {
      en: { title: "Credit {points} ({points})", body: "Added {points}; again {points}; reference {resource_id}." },
      "zh-CN": { title: "到账 {points}（{points}）", body: "已到账 {points}；再次 {points}；编号 {resource_id}。" },
    };
    const custom = {
      ...item("recharge.confirmed", 2),
      content: snapshot,
      payload: { resource_id: "44444444-4444-4444-8444-444444444444", points: "9223372036854775807" },
    };

    expect(renderNotification(custom, "en")).toMatchObject({
      title: "Credit 9,223,372,036,854,775,807 (9,223,372,036,854,775,807)",
      body: "Added 9,223,372,036,854,775,807; again 9,223,372,036,854,775,807; reference 44444444-4444-4444-8444-444444444444.",
      protectedNote: null,
    });
    expect(renderNotification(custom, "zh")).toMatchObject({
      title: "到账 9,223,372,036,854,775,807（9,223,372,036,854,775,807）",
      body: "已到账 9,223,372,036,854,775,807；再次 9,223,372,036,854,775,807；编号 44444444-4444-4444-8444-444444444444。",
    });
  });

  it.each(["bet.order.won", "bet.order.prize_reversed"] as const)(
    "adds a fixed protected historical note beside snapshot copy for %s",
    (event) => {
      const snapshot = {
        en: { title: "Operator title", body: "Operator copy: {points}." },
        "zh-CN": { title: "自定义标题", body: "自定义内容：{points}。" },
      };
      const custom = { ...item(event, 2), content: snapshot };
      const renderedEn = renderNotification(custom, "en");
      const renderedZh = renderNotification(custom, "zh");
      const legacyEn = renderNotification(item(event), "en");
      const legacyZh = renderNotification(item(event), "zh");
      expect(renderedEn.body).toBe("Operator copy: 900,719,925,474,099,312,345.");
      expect(renderedEn.protectedNote).toBe(legacyEn.body);
      expect(renderedZh.body).toBe("自定义内容：900,719,925,474,099,312,345。");
      expect(renderedZh.protectedNote).toBe(legacyZh.body);
      expect(renderedEn.protectedNote).toContain("not your current wallet balance");
      expect(renderedZh.protectedNote).toContain("不代表当前钱包余额");
    },
  );

  it.each(["zh", "en"] as const)("renders exact points and business references in %s", (locale) => {
    const withPoints = events.filter((event) => event !== "member.joined");
    for (const event of withPoints) {
      const rendered = renderNotification(item(event), locale);
      expect(rendered.points).toBe("900,719,925,474,099,312,345");
      expect(rendered.body).toContain("900,719,925,474,099,312,345");
      expect(rendered.reference).toBe("business-reference-42");
    }
    const welcome = renderNotification(item("member.joined"), locale);
    expect(welcome.points).toBeNull();
    expect(welcome.reference).toBeNull();
  });

  it("rejects unknown versions, template keys, and unsupported events", () => {
    expect(() => renderNotification(item("member.joined", 2), "en")).toThrow(RangeError);
    expect(() => renderNotification(item("admin.internal" as NotificationEventType), "zh")).toThrow(RangeError);
    expect(() => renderNotification({ ...item("member.joined"), template_key: "bet.order.placed" }, "en")).toThrow(RangeError);
    expect(() => renderNotification({ ...item("member.joined", 2), content: null }, "en")).toThrow(RangeError);
  });

  it("formats large integer strings without numeric precision loss", () => {
    expect(formatIntegerString("900719925474099312345", "en")).toBe(
      "900,719,925,474,099,312,345",
    );
    expect(formatIntegerString("-1200000", "zh")).toBe("-1,200,000");
    expect(formatIntegerString(null, "en")).toBe("");
    expect(formatIntegerString("1.5", "en")).toBe("1.5");
  });
});
