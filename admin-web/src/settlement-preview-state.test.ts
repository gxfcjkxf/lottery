import { describe, expect, it } from "vitest";
import {
  createPreviewRequestLane,
  createFrozenPreviewStore,
  formatIntegerPoints,
  freezePreviewIntent,
  matchesPreviewReceipt,
  previewWriteFailure,
  samePreviewContext,
  validPreviewReason,
} from "./settlement-preview-state";

describe("frozen settlement preview intent", () => {
  it("deep copies and freezes the exact body and scopes retries to actor, brand, and order", () => {
    const source = {
      version: 4,
      reason: "核对开奖号码",
      nested: { values: [1, 2] },
    };
    const intent = freezePreviewIntent({
      accountId: "account-a",
      brandId: "brand-a",
      orderId: "order-a",
      body: source,
      key: "fixed-key",
    });
    source.nested.values.push(3);
    expect(intent.body.nested.values).toEqual([1, 2]);
    expect(Object.isFrozen(intent)).toBe(true);
    expect(Object.isFrozen(intent.body.nested.values)).toBe(true);

    const store = createFrozenPreviewStore<typeof source>();
    store.remember(intent);
    expect(store.find("account-a", "brand-a", "order-a")).toBe(intent);
    expect(store.find("account-b", "brand-a", "order-a")).toBeNull();
    expect(store.find("account-a", "brand-b", "order-a")).toBeNull();
    expect(store.find("account-a", "brand-a", "order-b")).toBeNull();
    expect(store.find("account-a", "brand-a", "order-a")?.key).toBe("fixed-key");
  });

  it("does not remove a newer frozen intent when an older result arrives late", () => {
    const store = createFrozenPreviewStore<{ version: number }>();
    const oldIntent = freezePreviewIntent({
      accountId: "a", brandId: "b", orderId: "o", body: { version: 1 }, key: "old",
    });
    const newIntent = freezePreviewIntent({
      accountId: "a", brandId: "b", orderId: "o", body: { version: 2 }, key: "new",
    });
    store.remember(oldIntent);
    store.remember(newIntent);
    store.forget(oldIntent);
    expect(store.find("a", "b", "o")).toBe(newIntent);
  });
});

describe("preview context and write result helpers", () => {
  it("requires an exact brand, order, and order version match", () => {
    const context = { brand_id: "b", order_id: "o", order_version: 3 };
    expect(samePreviewContext(context, { brandId: "b", orderId: "o", orderVersion: 3 })).toBe(true);
    expect(samePreviewContext(context, { brandId: "x", orderId: "o", orderVersion: 3 })).toBe(false);
    expect(samePreviewContext(context, { brandId: "b", orderId: "x", orderVersion: 3 })).toBe(false);
    expect(samePreviewContext(context, { brandId: "b", orderId: "o", orderVersion: 4 })).toBe(false);
  });

  it("keeps unknown outcomes distinct from known failures", () => {
    expect(previewWriteFailure(undefined)).toBe("unknown");
    expect(previewWriteFailure(0)).toBe("unknown");
    expect(previewWriteFailure(503)).toBe("unknown");
    expect(previewWriteFailure(409)).toBe("conflict");
    expect(previewWriteFailure(403)).toBe("rejected");
  });

  it("formats integer points without Number precision loss", () => {
    expect(formatIntegerPoints("9223372036854775807")).toBe("9,223,372,036,854,775,807");
    expect(formatIntegerPoints("001234567890123456789")).toBe("001,234,567,890,123,456,789");
    expect(formatIntegerPoints("12345678901234567890.50")).toBe("12,345,678,901,234,567,890.50");
    expect(formatIntegerPoints("12.50")).toBe("12.50");
  });

  it("counts UTF-8 reason bytes", () => {
    expect(validPreviewReason("  核对  ")).toBe(true);
    expect(validPreviewReason(" \n")).toBe(false);
  });
});

describe("qualified create acknowledgements and request lanes", () => {
  const expected = {
    brandId: "brand-1",
    orderId: "order-1",
    gameId: "game-1",
    periodId: "period-1",
    orderVersion: 5,
    periodVersion: 9,
    drawResultId: "draw-1",
    definitionHash: "a".repeat(64),
    accountId: "operator-1",
    reason: "核对开奖结果",
  };
  const receipt = {
    id: "preview-1",
    brand_id: expected.brandId,
    order_id: expected.orderId,
    game_id: expected.gameId,
    period_id: expected.periodId,
    order_version: expected.orderVersion,
    period_version: expected.periodVersion,
    draw_result_id: expected.drawResultId,
    definition_hash: expected.definitionHash,
    created_by: expected.accountId,
    reason: expected.reason,
    applied: false as const,
  };

  it("accepts only a receipt matching actor, order context, versions, draw, rule hash, and reason", () => {
    expect(matchesPreviewReceipt(receipt, expected)).toBe(true);
  });

  it.each([
    ["creator", { created_by: "another-operator" }],
    ["reason", { reason: "different reason" }],
    ["game", { game_id: "other-game" }],
    ["period", { period_id: "other-period" }],
    ["definition hash", { definition_hash: "b".repeat(64) }],
  ])("rejects a receipt with a mismatched %s", (_field, patch) => {
    expect(matchesPreviewReceipt({ ...receipt, ...patch }, expected)).toBe(false);
  });

  it("rejects a receipt with changed versions, draw identity, or an applied state", () => {
    for (const patch of [
      { order_version: 6 },
      { period_version: 10 },
      { draw_result_id: "other-draw" },
      { applied: true as const },
    ]) {
      expect(matchesPreviewReceipt({ ...receipt, ...patch }, expected)).toBe(false);
    }
  });

  it("clears the exact frozen request at acknowledgement before a later read failure", () => {
    const store = createFrozenPreviewStore<{ version: number; reason: string }>();
    const intent = freezePreviewIntent({
      accountId: expected.accountId,
      brandId: expected.brandId,
      orderId: expected.orderId,
      body: { version: expected.orderVersion, reason: expected.reason },
      key: "stable-key",
    });
    store.remember(intent);
    expect(matchesPreviewReceipt(receipt, expected)).toBe(true);
    store.forget(intent);
    expect(() => { throw new Error("follow-up GET failed"); }).toThrow("follow-up GET failed");
    expect(store.find(expected.accountId, expected.brandId, expected.orderId)).toBeNull();
  });

  it("keeps request lanes independent and makes superseded responses stale", () => {
    const context = createPreviewRequestLane();
    const lines = createPreviewRequestLane();
    const oldLinesTicket = lines.begin();
    const contextTicket = context.begin();
    const newLinesTicket = lines.begin();
    expect(context.isCurrent(contextTicket)).toBe(true);
    expect(lines.isCurrent(oldLinesTicket)).toBe(false);
    expect(lines.isCurrent(newLinesTicket)).toBe(true);
    lines.invalidate();
    expect(lines.isCurrent(newLinesTicket)).toBe(false);
  });

  it("retains exact intents only in the scoped in-memory store", () => {
    const store = createFrozenPreviewStore<{ version: number; reason: string }>();
    const intent = freezePreviewIntent({
      accountId: "operator-1", brandId: "brand-1", orderId: "order-1",
      body: { version: 5, reason: "核对开奖结果" }, key: "retry-same-key",
    });
    store.remember(intent);
    expect(store.find("operator-1", "brand-1", "order-1")).toBe(intent);
    expect(store.find("operator-2", "brand-1", "order-1")).toBeNull();
    expect(store.find("operator-1", "brand-2", "order-1")).toBeNull();
    expect(store.find("operator-1", "brand-1", "order-2")).toBeNull();
  });
});
