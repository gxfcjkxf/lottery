import { afterEach, describe, expect, it } from "vitest";
import {
  brandOperationSessionGeneration,
  brandOperationCompletionMessage,
  classifyBrandOperationFailure,
  clearAllPendingBrandOperationWrites,
  clearPendingBrandOperationWrite,
  createBrandOperationRequestGuard,
  getPendingBrandOperationWrite,
  setPendingBrandOperationWrite,
  updatePendingBrandOperationPhase,
  type PendingBrandOperationWrite,
} from "./brand-operation-state";

const scope = { accountId: "00000000-0000-4000-8000-000000000001", brandId: "00000000-0000-4000-8000-000000000002" };
const intent: PendingBrandOperationWrite = { ...scope, body: { version: 4, status: "paused", reason: "maintenance" }, key: "stable-key", phase: "unknown" };

afterEach(() => clearAllPendingBrandOperationWrites());

describe("page-session brand operation write state", () => {
  it("retains a detached, frozen account-and-brand-scoped intent across remounts", () => {
    const mutable = { ...intent.body };
    setPendingBrandOperationWrite(scope, { ...intent, body: mutable });
    mutable.status = "active";
    const restored = getPendingBrandOperationWrite(scope);
    expect(restored).toEqual(intent);
    expect(Object.isFrozen(restored)).toBe(true);
    expect(Object.isFrozen(restored?.body)).toBe(true);
    expect(getPendingBrandOperationWrite({ ...scope, brandId: "00000000-0000-4000-8000-000000000003" })).toBeNull();
    expect(getPendingBrandOperationWrite({ ...scope, accountId: "00000000-0000-4000-8000-000000000004" })).toBeNull();
  });

  it("updates intent phase and clears only the expected idempotency key", () => {
    setPendingBrandOperationWrite(scope, intent);
    updatePendingBrandOperationPhase(scope, intent.key, "conflict");
    expect(getPendingBrandOperationWrite(scope)?.phase).toBe("conflict");
    const replacement = { ...intent, key: "new-key" };
    setPendingBrandOperationWrite(scope, replacement);
    clearPendingBrandOperationWrite(scope, intent.key);
    expect(getPendingBrandOperationWrite(scope)).toEqual(replacement);
    clearPendingBrandOperationWrite(scope, replacement.key);
    expect(getPendingBrandOperationWrite(scope)).toBeNull();
  });

  it("classifies unknown transport/server outcomes separately from stale conflicts", () => {
    expect(classifyBrandOperationFailure(undefined)).toBe("unknown");
    expect(classifyBrandOperationFailure(0)).toBe("unknown");
    expect(classifyBrandOperationFailure(503)).toBe("unknown");
    expect(classifyBrandOperationFailure(409)).toBe("conflict");
    expect(classifyBrandOperationFailure(403)).toBe("definitive");
  });

  it("invalidates late callbacks across actor/session generations", () => {
    const guard = createBrandOperationRequestGuard();
    const oldRequest = guard.begin();
    expect(guard.isCurrent(oldRequest)).toBe(true);
    const newRequest = guard.begin();
    expect(guard.isCurrent(oldRequest)).toBe(false);
    expect(guard.isCurrent(newRequest)).toBe(true);
    guard.invalidate();
    expect(guard.isCurrent(newRequest)).toBe(false);
    const session = brandOperationSessionGeneration();
    clearAllPendingBrandOperationWrites();
    expect(brandOperationSessionGeneration()).toBe(session + 1);
    expect(guard.isCurrent(newRequest)).toBe(false);
  });

  it("keeps current-read, history-read, and write acknowledgements on independent request guards", () => {
    const currentRead = createBrandOperationRequestGuard();
    const historyRead = createBrandOperationRequestGuard();
    const write = createBrandOperationRequestGuard();
    const currentToken = currentRead.begin();
    const historyToken = historyRead.begin();
    const writeToken = write.begin();
    expect(currentRead.isCurrent(currentToken)).toBe(true);
    expect(historyRead.isCurrent(historyToken)).toBe(true);
    expect(write.isCurrent(writeToken)).toBe(true);
    historyRead.begin();
    expect(currentRead.isCurrent(currentToken)).toBe(true);
    expect(write.isCurrent(writeToken)).toBe(true);
  });

  it("distinguishes a verified write receipt from a successful independent live-state read", () => {
    expect(brandOperationCompletionMessage(true)).toMatch(/已独立重新读取/);
    expect(brandOperationCompletionMessage(false)).toMatch(/回执已核验，但读取最新状态失败/);
    expect(brandOperationCompletionMessage(false)).not.toMatch(/当前状态已独立重新读取/);
  });
});
