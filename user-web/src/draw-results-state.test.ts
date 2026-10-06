import { describe, expect, it } from "vitest";
import {
  formatDrawDateTime,
  isCancelledPeriodStatus,
  isUuid,
  isValidPeriodFilter,
  periodStatusLabel,
  resultNumberGroups,
  shiftedPageOffset,
} from "./draw-results-state";
import type { PublicDrawResult } from "./draws-api";

describe("draw results display helpers", () => {
  it("accepts canonical UUIDs only", () => {
    expect(isUuid("3e6d149e-d6ff-4261-8b5e-4f8b465feb9b")).toBe(true);
    expect(isUuid("not-a-uuid")).toBe(false);
    expect(isUuid("3e6d149ed6ff42618b5e4f8b465feb9b")).toBe(false);
  });

  it("validates period filters by UTF-8 bytes without normalizing", () => {
    expect(isValidPeriodFilter("")).toBe(true);
    expect(isValidPeriodFilter("期次-01")).toBe(true);
    expect(isValidPeriodFilter(" 期次-01 ")).toBe(true);
    expect(isValidPeriodFilter(" \t　")).toBe(false);
    expect(isValidPeriodFilter("中".repeat(26) + "ab")).toBe(true);
    expect(isValidPeriodFilter("中".repeat(27))).toBe(false);
  });

  it("preserves exact regular, special, and positional digit sequences", () => {
    const result: PublicDrawResult["result"] = {
      regular: [8, 1, 8],
      special: [3, 3, 0],
      digits: [0, 2, 0, 9, 0],
    };
    expect(resultNumberGroups(result, "en")).toEqual([
      { key: "regular", label: "Regular", positions: [8, 1, 8] },
      { key: "special", label: "Special", positions: [3, 3, 0] },
      {
        key: "digits",
        label: "Position digits",
        positions: [0, 2, 0, 9, 0],
      },
    ]);
  });

  it("maps every public period status in both locales and flags cancellations", () => {
    const statuses = [
      "pending",
      "betting",
      "closed",
      "waiting_draw",
      "drawn",
      "settling",
      "settled",
    ];
    for (const status of statuses) {
      expect(periodStatusLabel(status, "zh")).not.toBe(status);
      expect(periodStatusLabel(status, "en")).not.toBe(status);
      expect(isCancelledPeriodStatus(status)).toBe(false);
    }
    for (const status of ["bet_cancelled", "judged_cancelled"]) {
      expect(isCancelledPeriodStatus(status)).toBe(true);
      expect(periodStatusLabel(status, "zh")).toContain("不用于有效判定或派奖");
      expect(periodStatusLabel(status, "en")).toContain(
        "not a valid outcome or payout",
      );
    }
  });

  it("moves pagination offsets without resetting the current page", () => {
    expect(shiftedPageOffset(50, 1, 50)).toBe(100);
    expect(shiftedPageOffset(50, -1, 50)).toBe(0);
    expect(shiftedPageOffset(0, -1, 50)).toBe(0);
  });

  it("formats times in the game's timezone", () => {
    expect(
      formatDrawDateTime("2026-10-06T00:00:00Z", "Asia/Singapore", "en"),
    ).toMatch(/8:00:00/);
    expect(formatDrawDateTime("bad date", "UTC", "zh")).toBe("bad date");
  });
});
