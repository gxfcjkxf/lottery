import { describe, expect, it } from "vitest";
import {
  formatIntegerString,
  renderNotification,
  type NotificationPresentationItem,
} from "./notification-presentation";

const events = [
  "member.joined",
  "recharge.confirmed",
  "bet.order.placed",
  "bet.order.cancelled",
  "bet.order.judged_cancelled",
  "bet.order.abnormal",
];

function item(event_type: string, template_version = 1): NotificationPresentationItem {
  return {
    event_type,
    template_key: event_type,
    template_version,
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
    expect(() => renderNotification(item("admin.internal"), "zh")).toThrow(RangeError);
    expect(() => renderNotification({ ...item("member.joined"), template_key: "bet.order.placed" }, "en")).toThrow(RangeError);
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
