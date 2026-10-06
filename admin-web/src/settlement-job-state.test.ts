import { describe, expect, it } from "vitest";
import {
  createFrozenSettlementJobStore,
  createSettlementJobRequestLane,
  formatSettlementPoints,
  freezeSettlementJobIntent,
  isSettlementJobTerminal,
  matchesSettlementJobReceipt,
  sameSettlementJobOperation,
  settlementJobWriteFailure,
  validSettlementReason,
} from "./settlement-job-state";

describe("frozen settlement job writes", () => {
  it("clones Vue reactive form bodies into immutable plain JSON", () => {
    const source = reactive({ version:1,mode:null as string|null,reason:"explicit configuration",nested:{choices:[1,2]} });
    const intent = freezeSettlementJobIntent({accountId:"a",brandId:"b",periodId:"",operation:"policy",body:source,key:"stable"});
    source.mode="automatic";source.nested.choices.push(3);
    expect(intent.body.mode).toBeNull();expect(intent.body.nested.choices).toEqual([1,2]);
    expect(structuredClone(intent.body)).toEqual({version:1,mode:null,reason:"explicit configuration",nested:{choices:[1,2]}});
    expect(Object.isFrozen(intent.body.nested)).toBe(true);
  });
  it("freezes the submitted body and scopes an intent to account, brand, and period", () => {
    const source = { version: 4, mode: "manual", nested: { choices: [1, 2] } };
    const intent = freezeSettlementJobIntent({
      accountId: "account-a", brandId: "brand-a", periodId: "period-a",
      operation: "policy", body: source, key: "fixed-key",
    });
    source.nested.choices.push(3);
    expect(intent.body.nested.choices).toEqual([1, 2]);
    expect(Object.isFrozen(intent.body.nested.choices)).toBe(true);

    const store = createFrozenSettlementJobStore<typeof source>();
    store.remember(intent);
    expect(store.find("account-a", "brand-a", "period-a")).toBe(intent);
    expect(store.find("account-b", "brand-a", "period-a")).toBeNull();
    expect(store.find("account-a", "brand-b", "period-a")).toBeNull();
    expect(store.find("account-a", "brand-a", "period-b")).toBeNull();
    expect(intent.key).toBe("fixed-key");
  });

  it("forgets only the same intent and distinguishes unknown from definite failures", () => {
    const store = createFrozenSettlementJobStore<{ version: number }>();
    const oldIntent = freezeSettlementJobIntent({
      accountId: "a", brandId: "b", periodId: "p", operation: "retry", resourceId: "j",
      body: { version: 1 }, key: "old",
    });
    const newIntent = freezeSettlementJobIntent({
      accountId: "a", brandId: "b", periodId: "p", operation: "retry", resourceId: "j",
      body: { version: 2 }, key: "new",
    });
    store.remember(oldIntent);
    store.remember(newIntent);
    store.forget(oldIntent);
    expect(store.find("a", "b", "p")).toBe(newIntent);
    expect(settlementJobWriteFailure(undefined)).toBe("unknown");
    expect(settlementJobWriteFailure(0)).toBe("unknown");
    expect(settlementJobWriteFailure(503)).toBe("unknown");
    expect(settlementJobWriteFailure(409)).toBe("conflict");
    expect(settlementJobWriteFailure(403)).toBe("rejected");
  });

  it("matches a start receipt against the original period, policy, draw, mode, reason, and actor", () => {
    const receipt = {
      id: "job-a", brand_id: "brand-a", period_id: "period-a", period_version: 8,
      policy_version: 3, draw_result_id: "draw-a", mode: "manual" as const,
      reason: "复核后结算", created_by: "account-a",
    };
    const expected = {
      brandId: "brand-a", periodId: "period-a", periodVersion: 8,
      policyVersion: 3, drawResultId: "draw-a", mode: "manual" as const,
      reason: "复核后结算", accountId: "account-a",
    };
    expect(matchesSettlementJobReceipt(receipt, expected)).toBe(true);
    expect(matchesSettlementJobReceipt(receipt, { ...expected, policyVersion: 4 })).toBe(false);
    expect(matchesSettlementJobReceipt(receipt, { ...expected, accountId: "account-b" })).toBe(false);
    expect(matchesSettlementJobReceipt(receipt, { ...expected, id: "job-b" })).toBe(false);
  });

  it("requires the intended operation and resource to match before retry", () => {
    const intent = freezeSettlementJobIntent({
      accountId: "a", brandId: "b", periodId: "p", operation: "approve", resourceId: "job-a",
      body: { version: 7, reason: "已复核" }, key: "k",
    });
    expect(sameSettlementJobOperation(intent, {
      accountId: "a", brandId: "b", periodId: "p", operation: "approve", resourceId: "job-a",
    })).toBe(true);
    expect(sameSettlementJobOperation(intent, {
      accountId: "a", brandId: "b", periodId: "p", operation: "retry", resourceId: "job-a",
    })).toBe(false);
    expect(sameSettlementJobOperation(intent, {
      accountId: "a", brandId: "b", periodId: "p", operation: "approve", resourceId: "job-b",
    })).toBe(false);
  });
});

describe("settlement job display and request generations", () => {
  it("formats huge point totals without precision loss and checks terminal accounting", () => {
    expect(formatSettlementPoints("9223372036854775807")).toBe("9,223,372,036,854,775,807");
    expect(formatSettlementPoints("100000000000000000000000000001")).toBe("100,000,000,000,000,000,000,000,000,001");
    expect(formatSettlementPoints("00012")).toBe("00012");
    expect(isSettlementJobTerminal({ state: "completed", paid_count: 8, excluded_count: 2, target_count: 10 })).toBe(true);
    expect(isSettlementJobTerminal({ state: "completed", paid_count: 8, excluded_count: 1, target_count: 10 })).toBe(false);
    expect(isSettlementJobTerminal({ state: "failed", paid_count: 8, excluded_count: 2, target_count: 10 })).toBe(false);
  });

  it("invalidates stale read generations when scope or lane changes", () => {
    const context = createSettlementJobRequestLane();
    const old = context.begin();
    expect(context.isCurrent(old)).toBe(true);
    const newer = context.begin();
    expect(context.isCurrent(old)).toBe(false);
    expect(context.isCurrent(newer)).toBe(true);
    context.invalidate();
    expect(context.isCurrent(newer)).toBe(false);
  });

  it("counts the reason in UTF-8 bytes", () => {
    expect(validSettlementReason(" 人工复核 ")).toBe(true);
    expect(validSettlementReason(" \n ")).toBe(false);
    expect(validSettlementReason("界".repeat(167))).toBe(false);
  });
});
import { reactive } from "vue";
