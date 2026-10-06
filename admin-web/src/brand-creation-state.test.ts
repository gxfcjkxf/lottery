import { afterEach, describe, expect, it } from "vitest";
import { brandCreationSessionGeneration, classifyBrandCreationFailure, clearAllPendingBrandCreationWrites, clearPendingBrandCreationWrite, createBrandCreationRequestGuard, getPendingBrandCreationWrite, retainPendingBrandCreationWrite, updatePendingBrandCreationPhase, type PendingBrandCreationWrite } from "./brand-creation-state";

const accountId = "00000000-0000-4000-8000-000000000001";
const body = { code: "new_brand", name: "New Brand", default_locale: "en" as const, timezone: "Asia/Manila", reason: "Initial setup" };
const intent: PendingBrandCreationWrite = { accountId, body, key: "creation-key-0001", phase: "review" };
afterEach(() => clearAllPendingBrandCreationWrites());

describe("global brand creation pending writes", () => {
  it("freezes a detached account-specific intent across component remounts", () => {
    const mutable = { ...body }; retainPendingBrandCreationWrite({ ...intent, body: mutable }); mutable.name = "changed";
    const stored = getPendingBrandCreationWrite(accountId);
    expect(stored?.body).toEqual(body); expect(Object.isFrozen(stored)).toBe(true); expect(Object.isFrozen(stored?.body)).toBe(true);
    expect(getPendingBrandCreationWrite("00000000-0000-4000-8000-000000000009")).toBeNull();
  });
  it("refuses replacing a key or body, updating phase only, and clears by matching key", () => {
    retainPendingBrandCreationWrite(intent);
    expect(retainPendingBrandCreationWrite({ ...intent, key: "replacement-key" })).toBe(false);
    expect(retainPendingBrandCreationWrite({ ...intent, body: { ...body, name: "other" } })).toBe(false);
    updatePendingBrandCreationPhase(accountId, intent.key, "unknown");
    expect(getPendingBrandCreationWrite(accountId)?.phase).toBe("unknown");
    clearPendingBrandCreationWrite(accountId, "replacement-key"); expect(getPendingBrandCreationWrite(accountId)).not.toBeNull();
    clearPendingBrandCreationWrite(accountId, intent.key); expect(getPendingBrandCreationWrite(accountId)).toBeNull();
  });
  it("exposes only the frozen retry intent while the original POST is unresolved", async () => {
    retainPendingBrandCreationWrite(intent);
    let finishPost!: () => void;
    const post = new Promise<void>((resolve) => { finishPost = resolve; });
    updatePendingBrandCreationPhase(accountId, intent.key, "unknown");
    const remounted = getPendingBrandCreationWrite(accountId);
    expect(remounted?.phase).toBe("unknown");
    expect(remounted?.key).toBe(intent.key);
    expect(remounted?.body).toEqual(intent.body);
    expect(Object.isFrozen(remounted?.body)).toBe(true);
    finishPost();
    await post;
  });
  it("classifies 400, 401, 409, and 503 outcomes explicitly", () => {
    expect(classifyBrandCreationFailure(undefined)).toBe("unknown"); expect(classifyBrandCreationFailure(0)).toBe("unknown");
    expect(classifyBrandCreationFailure(503)).toBe("unknown"); expect(classifyBrandCreationFailure(409, "IDEMPOTENCY_CONFLICT")).toBe("conflict");
    expect(classifyBrandCreationFailure(409, "BRAND_CODE_CONFLICT")).toBe("conflict"); expect(classifyBrandCreationFailure(400, "BRAND_CREATE_INPUT_INVALID")).toBe("definitive");
    expect(classifyBrandCreationFailure(401, "AUTH")).toBe("definitive"); expect(classifyBrandCreationFailure(403, "PERMISSION_DENIED")).toBe("definitive");
  });
  it("retains pending work across remounts, clears it on logout, and rejects its late reply", () => {
    retainPendingBrandCreationWrite(intent);
    const guard = createBrandCreationRequestGuard(), first = guard.begin(); expect(guard.isCurrent(first)).toBe(true);
    const second = guard.begin(); expect(guard.isCurrent(first)).toBe(false); expect(guard.isCurrent(second)).toBe(true);
    const generation = brandCreationSessionGeneration(); clearAllPendingBrandCreationWrites();
    expect(brandCreationSessionGeneration()).toBe(generation + 1); expect(getPendingBrandCreationWrite(accountId)).toBeNull();
    expect(guard.isCurrent(second)).toBe(false);
  });
  it("rejects pending bodies outside the shared API contract", () => {
    for (const invalidBody of [
      { ...body, timezone: "Made/Up" }, { ...body, reason: "line\nbreak" }, { ...body, extra: true },
    ]) expect(retainPendingBrandCreationWrite({ ...intent, body: invalidBody } as PendingBrandCreationWrite)).toBe(false);
  });
});
