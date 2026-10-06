import { describe, expect, it } from "vitest";
import { reactive } from "vue";
import type { CorrectionMode } from "./correction-api";
import {
  canonicalCorrectionResult,
  correctionCountsMatch,
  correctionReasonByteLength,
  correctionWriteFailure,
  copyCorrectionJson,
  createCorrectionRequestLane,
  createFrozenCorrectionStore,
  formatCorrectionPoints,
  freezeCorrectionIntent,
  matchesCorrectionReceipt,
  matchesCorrectionRetryReceipt,
  parseCorrectionNumberCsv,
  sameCorrectionOperation,
  sameCorrectionResult,
  validCorrectionReason,
} from "./correction-state";

const result = { regular: [4, 2], special: [9], digits: [3, 1, 3] };
const receipt = {
  id: "correction-a", brand_id: "brand-a", period_id: "period-a", draw_result_id: "draw-new",
  previous_draw_result_id: "draw-old", period_version: 8, policy_version: 5, mode: "manual" as const,
  state: "reversing" as const, version: 1, result: { regular: [2, 4], special: [9], digits: [3, 1, 3] },
  created_by: "account-a", reason: "核对开奖结果",
};
const expected = {
  brandId: "brand-a", periodId: "period-a", drawResultId: "draw-new", previousDrawResultId: "draw-old",
  periodVersion: 7, policyVersion: 5, mode: "manual" as const,
  result: { regular: [2, 4], special: [9], digits: [3, 1, 3] }, accountId: "account-a",
  reason: "核对开奖结果", financial: true,
};

describe("correction write intents", () => {
  it("snapshots reactive JSON as immutable plain data without structuredClone", () => {
    const source = reactive({
      version: 8, policy_version: 5, result: { regular: [2, 4], special: [9], digits: [3, 1] }, reason: "核对开奖结果",
    });
    const snapshot = copyCorrectionJson(source);
    const intent = freezeCorrectionIntent({
      accountId: "account-a", brandId: "brand-a", periodId: "period-a", operation: "correct",
      expectedDrawResultId: "draw-old", body: source, key: "same-key",
    });
    source.result.regular.push(6);
    source.reason = "changed";
    expect(snapshot.result.regular).toEqual([2, 4]);
    expect(intent.body.result.regular).toEqual([2, 4]);
    expect(Object.isFrozen(intent.body.result.regular)).toBe(true);
    expect(JSON.stringify(intent.body)).toBe(JSON.stringify({
      version: 8, policy_version: 5, result: { regular: [2, 4], special: [9], digits: [3, 1] }, reason: "核对开奖结果",
    }));
  });

  it("keeps the original draw route, mode, and version when a newer read changes the context", () => {
    const confirmation = reactive({
      accountId: "account-a", brandId: "brand-a", periodId: "period-a", operation: "correct" as const,
      expectedDrawResultId: "draw-old", expectedMode: "manual" as CorrectionMode,
      body: { version: 7, policy_version: 5, result: expected.result, reason: expected.reason }, key: "original-key",
    });
    const intent = freezeCorrectionIntent(confirmation);
    const store = createFrozenCorrectionStore<typeof confirmation.body>();
    store.remember(intent);
    confirmation.expectedDrawResultId = "draw-new";
    confirmation.expectedMode = "automatic";
    confirmation.body.version = 8;
    confirmation.body.policy_version = 6;
    expect(intent.expectedDrawResultId).toBe("draw-old");
    expect(intent.expectedMode).toBe("manual");
    expect(intent.body.version).toBe(7);
    expect(intent.body.policy_version).toBe(5);
    expect(store.find("account-a", "brand-a", "period-a")).toBe(intent);
    expect(matchesCorrectionReceipt(receipt, {
      ...expected, previousDrawResultId: intent.expectedDrawResultId!, mode: intent.expectedMode!,
      periodVersion: intent.body.version, policyVersion: intent.body.policy_version,
    })).toBe(true);
    store.forget(intent); // Qualified write receipt acknowledges before any GET.
    expect(store.find("account-a", "brand-a", "period-a")).toBeNull();
  });

  it("scopes unknown intents to account, brand, and period and retains only the same entry", () => {
    const store = createFrozenCorrectionStore<{ version: number }>();
    const first = freezeCorrectionIntent({
      accountId: "a", brandId: "b", periodId: "p", operation: "retry", resourceId: "c",
      body: { version: 3 }, key: "k1",
    });
    const next = freezeCorrectionIntent({
      accountId: "a", brandId: "b", periodId: "p", operation: "retry", resourceId: "c",
      body: { version: 4 }, key: "k2",
    });
    store.remember(first); store.remember(next); store.forget(first);
    expect(store.find("a", "b", "p")).toBe(next);
    expect(store.find("a", "b", "other")).toBeNull();
    expect(store.find("other", "b", "p")).toBeNull();
    expect(store.find("a", "other", "p")).toBeNull();
    expect(sameCorrectionOperation(next, { accountId: "a", brandId: "b", periodId: "p", operation: "retry", resourceId: "c" })).toBe(true);
    expect(sameCorrectionOperation(next, { accountId: "a", brandId: "b", periodId: "p", operation: "correct" })).toBe(false);
  });

  it("keeps original request intent on unknown outcomes and distinguishes definite rejections", () => {
    expect(correctionWriteFailure(undefined)).toBe("unknown");
    expect(correctionWriteFailure(0)).toBe("unknown");
    expect(correctionWriteFailure(502)).toBe("unknown");
    expect(correctionWriteFailure(409)).toBe("conflict");
    expect(correctionWriteFailure(403)).toBe("rejected");
  });
});

describe("correction result and receipt validation", () => {
  it("parses zeros and preserves CSV order while rejecting signed, fractional, missing, or unsafe values", () => {
    expect(parseCorrectionNumberCsv(" 0, 00, 4, 2, 0 ")).toEqual([0, 0, 4, 2, 0]);
    expect(parseCorrectionNumberCsv(" ")).toEqual([]);
    for (const value of ["-1", "+1", "-0", "1.5", "1e2", "1,,2", "1,", "9007199254740992"]) {
      expect(parseCorrectionNumberCsv(value)).toBeNull();
    }
  });
  it("sorts unordered number groups while preserving ordered fields and digits", () => {
    expect(canonicalCorrectionResult(result, false)).toEqual({ regular: [2, 4], special: [9], digits: [3, 1, 3] });
    expect(canonicalCorrectionResult(result, true)).toEqual(result);
    expect(sameCorrectionResult(canonicalCorrectionResult(result, false), { ...result, regular: [2, 4] })).toBe(true);
  });

  it("equates a receipt with the exact original actor, IDs, version, mode, reason, and canonical result", () => {
    expect(matchesCorrectionReceipt(receipt, expected)).toBe(true);
    expect(matchesCorrectionReceipt(receipt, { ...expected, periodVersion: 8 })).toBe(false);
    expect(matchesCorrectionReceipt({ ...receipt, version: 2 }, expected)).toBe(false);
    expect(matchesCorrectionReceipt(receipt, { ...expected, mode: "automatic" })).toBe(false);
    expect(matchesCorrectionReceipt(receipt, { ...expected, accountId: "account-b" })).toBe(false);
    expect(matchesCorrectionReceipt(receipt, { ...expected, reason: "別の理由" })).toBe(false);
    expect(matchesCorrectionReceipt(receipt, { ...expected, result: { ...expected.result, digits: [3, 1] } })).toBe(false);
    expect(matchesCorrectionReceipt({ ...receipt, state: "completed" }, expected)).toBe(false);
    expect(matchesCorrectionReceipt(receipt, { ...expected, financial: false })).toBe(false);
  });

  it("requires the exact next reversing retry receipt while retaining the original creator and reason", () => {
    const retried = { ...receipt, version: 4, created_by: "original-creator", reason: "original-correction-reason" };
    expect(matchesCorrectionRetryReceipt(retried, { id: receipt.id, version: 3 })).toBe(true);
    expect(matchesCorrectionRetryReceipt(retried, { id: "other-correction", version: 3 })).toBe(false);
    expect(matchesCorrectionRetryReceipt({ ...retried, version: 5 }, { id: receipt.id, version: 3 })).toBe(false);
    expect(matchesCorrectionRetryReceipt({ ...retried, state: "resettling" }, { id: receipt.id, version: 3 })).toBe(false);
  });

  it("accepts the original zero-target no-job receipt with null financial policy and mode", () => {
    expect(matchesCorrectionReceipt({ ...receipt, state: "completed", mode: null, policy_version: null }, {
      ...expected, financial: false, mode: null, policyVersion: null,
    })).toBe(true);
  });

  it("checks target accounting and formats arbitrary precision points without Number", () => {
    expect(correctionCountsMatch({ target_count: 8, pending_count: 1, reversed_count: 2, unchanged_count: 3, excluded_count: 1, failed_count: 1 })).toBe(true);
    expect(correctionCountsMatch({ target_count: 8, pending_count: 1, reversed_count: 2, unchanged_count: 3, excluded_count: 1, failed_count: 0 })).toBe(false);
    expect(formatCorrectionPoints("9223372036854775807")).toBe("9,223,372,036,854,775,807");
    expect(formatCorrectionPoints("100000000000000000000000000001")).toBe("100,000,000,000,000,000,000,000,000,001");
    expect(formatCorrectionPoints("00012")).toBe("00012");
  });
});

describe("correction read generations", () => {
  it("makes old scoped responses stale after a newer request or scope invalidation", () => {
    const lane = createCorrectionRequestLane();
    const old = lane.begin();
    const current = lane.begin();
    expect(lane.isCurrent(old)).toBe(false);
    expect(lane.isCurrent(current)).toBe(true);
    lane.invalidate();
    expect(lane.isCurrent(current)).toBe(false);
  });

  it("counts reasons in UTF-8 bytes", () => {
    expect(validCorrectionReason(" 复核开奖结果 ")).toBe(true);
    expect(validCorrectionReason(" \n ")).toBe(false);
    expect(correctionReasonByteLength("界".repeat(167))).toBe(501);
    expect(validCorrectionReason("界".repeat(167))).toBe(false);
  });
});
