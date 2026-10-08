import { beforeEach, describe, expect, it } from "vitest";
import {
  clearAllReportArchivePolicyIntents,
  clearReportArchivePolicyIntent,
  createReportArchivePolicyIntent,
  getReportArchivePolicyIntent,
  getReportArchivePolicyIntents,
  markReportArchivePolicyAcknowledged,
  markReportArchivePolicyConflict,
  reportArchivePolicySessionGeneration,
  setReportArchivePolicyIntent,
} from "./report-archive-policy-state";
import type { ReportArchivePolicy } from "./report-archive-tasks-api";

const actor = "22222222-2222-4222-8222-222222222222";
const brand = "11111111-1111-4111-8111-111111111111";
const otherBrand = "44444444-4444-4444-8444-444444444444";
const audit = "33333333-3333-4333-8333-333333333333";
const intent = (version = 4, reason = "Enable daily archives", key = "policy-key-0001", daily = true, monthly = false) =>
  createReportArchivePolicyIntent(actor, brand, version, daily, monthly, reason, key);

const receipt: ReportArchivePolicy = {
  brand_id: brand, version: 5, daily_enabled: true, monthly_enabled: false,
  daily_start_period: "2026-10-01", monthly_start_period: null,
  timezone: "Asia/Singapore", audit_log_id: audit, updated_at: "2026-10-08T01:02:03.123456789Z",
};

describe("report archive policy intent state", () => {
  beforeEach(() => clearAllReportArchivePolicyIntents());

  it("freezes one actor-brand request snapshot and keeps unknown after reads", () => {
    const saved = intent();
    expect(saved).not.toBeNull();
    expect(setReportArchivePolicyIntent(saved!)).toBe(true);
    expect(getReportArchivePolicyIntent(actor, brand)).toMatchObject({
      actorId: actor, brandId: brand, version: 4, daily_enabled: true, monthly_enabled: false,
      reason: "Enable daily archives", key: "policy-key-0001", phase: "unknown", receipt: null,
    });
    // A caller's mutable policy GET does not transition the write intent.
    const currentPolicy = { ...receipt, version: 5 };
    expect(currentPolicy.daily_enabled).toBe(true);
    expect(getReportArchivePolicyIntent(actor, brand)?.phase).toBe("unknown");
    expect(setReportArchivePolicyIntent(intent(4, "different intent", "policy-key-0002")!)).toBe(false);
    expect(getReportArchivePolicyIntents(actor)).toHaveLength(1);
  });

  it("validates version bounds and strict UTF-8 reasons", () => {
    const badReasons = [" padded ", "control\u0085", "\ud800", "\udc00", "é".repeat(251)];
    for (const reason of badReasons) expect(intent(4, reason)).toBeNull();
    expect(intent(0)).toBeNull();
    expect(intent(Number.MAX_SAFE_INTEGER)).toBeNull();
    expect(intent(Number.MAX_SAFE_INTEGER - 1)).not.toBeNull();
    expect(intent(4, "界".repeat(166))).not.toBeNull();
    expect(intent(4, "界".repeat(167))).toBeNull();
  });

  it("requires a valid frozen generation and matching ACK, then preserves the full receipt", () => {
    const saved = intent()!;
    expect(setReportArchivePolicyIntent(saved)).toBe(true);
    expect(markReportArchivePolicyAcknowledged(saved, {
      brand_id: brand, version: 5, daily_enabled: true, monthly_enabled: false,
    } as ReportArchivePolicy)).toBe(false);
    expect(markReportArchivePolicyAcknowledged(saved, { ...receipt, version: 6 })).toBe(false);
    expect(markReportArchivePolicyAcknowledged(saved, { ...receipt, monthly_enabled: true })).toBe(false);
    expect(markReportArchivePolicyAcknowledged(saved, receipt)).toBe(true);
    const acknowledged = getReportArchivePolicyIntent(actor, brand)!;
    expect(acknowledged.phase).toBe("acknowledged");
    expect(acknowledged.receipt).toEqual(receipt);
    expect(acknowledged.receipt).not.toBe(receipt);
    expect(Object.isFrozen(acknowledged.receipt)).toBe(true);
    expect(acknowledged.receipt?.daily_start_period).toBe("2026-10-01");
    expect(acknowledged.receipt?.timezone).toBe("Asia/Singapore");
    expect(markReportArchivePolicyConflict(saved)).toBe(false);
    expect(getReportArchivePolicyIntent(actor, brand)?.phase).toBe("acknowledged");
    expect(getReportArchivePolicyIntent(actor, brand)?.receipt).toEqual(receipt);
  });

  it("separates brands, clears only the expected key, and rejects old generation intents", () => {
    const old = intent()!;
    const anotherBrand = createReportArchivePolicyIntent(actor, otherBrand, 2, false, false, "Keep disabled", "policy-key-0002")!;
    expect(setReportArchivePolicyIntent(old)).toBe(true);
    expect(setReportArchivePolicyIntent(anotherBrand)).toBe(true);
    expect(getReportArchivePolicyIntents(actor)).toHaveLength(2);
    expect(clearReportArchivePolicyIntent(actor, brand, "wrong-key")).toBe(false);
    expect(clearReportArchivePolicyIntent(actor, brand, old.key)).toBe(true);
    expect(getReportArchivePolicyIntent(actor, brand)).toBeNull();
    const generation = reportArchivePolicySessionGeneration();
    clearAllReportArchivePolicyIntents();
    expect(reportArchivePolicySessionGeneration()).toBe(generation + 1);
    expect(setReportArchivePolicyIntent(old)).toBe(false);
    expect(markReportArchivePolicyConflict(old)).toBe(false);
    expect(getReportArchivePolicyIntents(actor)).toEqual([]);
  });
});
