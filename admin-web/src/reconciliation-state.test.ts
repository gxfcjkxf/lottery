import { afterEach, describe, expect, it } from "vitest";
import {
  classifyReconciliationWriteFailure, clearPendingReconciliationWrite, clearPendingReconciliationWrites,
  freezeReconciliationBody, getPendingReconciliationWrite, listPendingReconciliationWrites, reconciliationSessionGeneration,
  setPendingReconciliationWrite, type PendingReconciliationWrite, type ReconciliationWriteScope,
} from "./reconciliation-state";

const accountId = "22222222-2222-4222-8222-222222222222";
const brandId = "11111111-1111-4111-8111-111111111111";
const jobId = "33333333-3333-4333-8333-333333333333";
const createScope: ReconciliationWriteScope = { accountId, brandId, operation: "create" };
const retryScope: ReconciliationWriteScope = { accountId, brandId, operation: "retry", jobId };
afterEach(() => clearPendingReconciliationWrites());

describe("reconciliation pending write state", () => {
  it("detaches and freezes exact create/retry bodies and scopes intents by account, brand, operation, and job", () => {
    const mutable = { reason: "creation reason" };
    const createIntent: PendingReconciliationWrite = { ...createScope, body: freezeReconciliationBody(mutable), key: "recon-create-key-001" };
    setPendingReconciliationWrite(createScope, createIntent);
    mutable.reason = "edited after submission";
    expect(getPendingReconciliationWrite(createScope)?.body).toEqual({ reason: "creation reason" });
    expect(getPendingReconciliationWrite({ ...createScope, brandId: brandId.toUpperCase() })?.key).toBe(createIntent.key);
    expect(Object.isFrozen(getPendingReconciliationWrite(createScope)?.body)).toBe(true);

    const retryIntent: PendingReconciliationWrite = { ...retryScope, body: freezeReconciliationBody({ version: 7, reason: "retry reason" }), key: "recon-retry-key-001" };
    setPendingReconciliationWrite(retryScope, retryIntent);
    expect(getPendingReconciliationWrite(retryScope)?.body).toEqual({ version: 7, reason: "retry reason" });
    expect(listPendingReconciliationWrites(accountId, brandId).map((item) => item.operation)).toEqual(["create", "retry"]);
    expect(getPendingReconciliationWrite({ ...retryScope, jobId: accountId })).toBeNull();
    expect(getPendingReconciliationWrite({ ...createScope, accountId: jobId })).toBeNull();
  });

  it("clears only the matching request key and preserves unknown writes across lookups", () => {
    const intent: PendingReconciliationWrite = { ...createScope, body: freezeReconciliationBody({ reason: "still uncertain" }), key: "recon-create-key-002" };
    setPendingReconciliationWrite(createScope, intent);
    clearPendingReconciliationWrite(createScope, "a-different-key");
    expect(getPendingReconciliationWrite(createScope)?.key).toBe(intent.key);
    clearPendingReconciliationWrite(createScope, intent.key);
    expect(getPendingReconciliationWrite(createScope)).toBeNull();
  });

  it("invalidates async work generation and clears all unresolved operations at logout", () => {
    const before = reconciliationSessionGeneration();
    setPendingReconciliationWrite(createScope, { ...createScope, body: freezeReconciliationBody({ reason: "pending" }), key: "recon-create-key-003" });
    clearPendingReconciliationWrites();
    expect(reconciliationSessionGeneration()).toBe(before + 1);
    expect(getPendingReconciliationWrite(createScope)).toBeNull();
  });

  it("classifies transport/server failures as unknown while 4xx is definitive", () => {
    expect(classifyReconciliationWriteFailure(undefined)).toBe("unknown");
    expect(classifyReconciliationWriteFailure(0)).toBe("unknown");
    expect(classifyReconciliationWriteFailure(503)).toBe("unknown");
    expect(classifyReconciliationWriteFailure(409)).toBe("definitive");
  });

  it("allows valid Unicode reasons and rejects an unpaired surrogate", () => {
    const valid: PendingReconciliationWrite = { ...createScope, body: freezeReconciliationBody({ reason: "检查余额 🔎" }), key: "recon-create-key-004" };
    setPendingReconciliationWrite(createScope, valid);
    expect(getPendingReconciliationWrite(createScope)?.body.reason).toBe("检查余额 🔎");
    const otherScope = { ...createScope, accountId: "66666666-6666-4666-8666-666666666666" };
    setPendingReconciliationWrite(otherScope, { ...otherScope, body: freezeReconciliationBody({ reason: "bad\ud800" }), key: "recon-create-key-005" });
    expect(getPendingReconciliationWrite(otherScope)).toBeNull();
    expect(getPendingReconciliationWrite(createScope)?.key).toBe("recon-create-key-004");
  });
});
