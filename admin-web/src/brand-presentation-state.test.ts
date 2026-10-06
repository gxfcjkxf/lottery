import { afterEach, describe, expect, it } from "vitest";
import {
  brandPresentationScopeKey, brandPresentationSessionGeneration, clearAllPendingPresentationWrites,
  clearPendingBrandPresentationWrite, classifyBrandPresentationFailure, createBrandPresentationRequestGuard,
  getPendingBrandPresentationWrite, setPendingBrandPresentationWrite, updatePendingBrandPresentationPhase,
  type PendingBrandPresentationWrite,
} from "./brand-presentation-state";

const scope = { accountId: "00000000-0000-4000-8000-000000000001", brandId: "00000000-0000-4000-8000-000000000002" };
const source: PendingBrandPresentationWrite = {
  ...scope, key: "write-key", phase: "unknown",
  body: { version: 7, reason: "Refresh", config: {
    display_name: null, logo_text: "Aurora", logo_url: null, favicon_url: null,
    primary_color: "#112233", accent_color: null, success_color: null, warning_color: null, danger_color: null,
    font_family: "system", font_scale: null, radius: null, shadow: null, default_locale: "en",
    available_locales: ["en", "zh-CN"], content: { en: { tagline: null, announcement: "Hello" }, "zh-CN": { tagline: "你好", announcement: null } },
  } },
};
afterEach(() => clearAllPendingPresentationWrites());

describe("page-session brand presentation write state", () => {
  it("stores a detached, deeply frozen config by actor and brand across remounts", () => {
    const mutable = structuredClone(source);
    setPendingBrandPresentationWrite(scope, mutable);
    mutable.body.config.available_locales?.reverse();
    mutable.body.config.content!.en.announcement = "mutated";
    const saved = getPendingBrandPresentationWrite(brandPresentationScopeKey(scope.accountId, scope.brandId));
    expect(saved).toEqual(source);
    expect(Object.isFrozen(saved)).toBe(true);
    expect(Object.isFrozen(saved?.body)).toBe(true);
    expect(Object.isFrozen(saved?.body.config)).toBe(true);
    expect(Object.isFrozen(saved?.body.config.available_locales)).toBe(true);
    expect(Object.isFrozen(saved?.body.config.content?.en)).toBe(true);
    expect(getPendingBrandPresentationWrite({ ...scope, brandId: "00000000-0000-4000-8000-000000000003" })).toBeNull();
    expect(getPendingBrandPresentationWrite({ ...scope, accountId: "00000000-0000-4000-8000-000000000004" })).toBeNull();
  });

  it("updates phase and clears only the expected write key", () => {
    setPendingBrandPresentationWrite(scope, source);
    updatePendingBrandPresentationPhase(scope, source.key, "conflict");
    expect(getPendingBrandPresentationWrite(scope)?.phase).toBe("conflict");
    const next = { ...source, key: "next-key" };
    setPendingBrandPresentationWrite(scope, next);
    clearPendingBrandPresentationWrite(scope, source.key);
    expect(getPendingBrandPresentationWrite(scope)?.key).toBe("next-key");
    clearPendingBrandPresentationWrite(scope, "next-key");
    expect(getPendingBrandPresentationWrite(scope)).toBeNull();
  });

  it("classifies transport errors and known version conflicts", () => {
    expect(classifyBrandPresentationFailure(undefined)).toBe("unknown");
    expect(classifyBrandPresentationFailure(0)).toBe("unknown");
    expect(classifyBrandPresentationFailure(503)).toBe("unknown");
    expect(classifyBrandPresentationFailure(409)).toBe("conflict");
    expect(classifyBrandPresentationFailure(403)).toBe("definitive");
  });

  it("keeps current, history, and write guards independent and invalidates old session callbacks", () => {
    const currentRead = createBrandPresentationRequestGuard();
    const historyRead = createBrandPresentationRequestGuard();
    const write = createBrandPresentationRequestGuard();
    const currentToken = currentRead.begin(), historyToken = historyRead.begin(), writeToken = write.begin();
    historyRead.begin();
    expect(currentRead.isCurrent(currentToken)).toBe(true);
    expect(write.isCurrent(writeToken)).toBe(true);
    expect(historyRead.isCurrent(historyToken)).toBe(false);
    const generation = brandPresentationSessionGeneration();
    clearAllPendingPresentationWrites();
    expect(brandPresentationSessionGeneration()).toBe(generation + 1);
    expect(currentRead.isCurrent(currentToken)).toBe(false);
    expect(write.isCurrent(writeToken)).toBe(false);
  });
});
