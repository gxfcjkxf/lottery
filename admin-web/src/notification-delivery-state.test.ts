import { afterEach, describe, expect, it, vi } from "vitest";
import { reactive } from "vue";
import {
  classifyDeliveryRetryFailure,
  clearAllPendingDeliveryRetries,
  clearPendingDeliveryRetry,
  freezeDeliveryRetryBody,
  getPendingDeliveryRetry,
  scopeKey,
  setPendingDeliveryRetry,
  type DeliveryRetryScope,
  type PendingDeliveryRetry,
} from "./notification-delivery-state";

const accountA = "00000000-0000-4000-8000-000000000001";
const accountB = "00000000-0000-4000-8000-000000000002";
const brandA = "00000000-0000-4000-8000-000000000003";
const brandB = "00000000-0000-4000-8000-000000000004";
const eventID = "00000000-0000-4000-8000-000000000005";
const scope: DeliveryRetryScope = { accountId: accountA, brandId: brandA };
const intent: PendingDeliveryRetry = {
  ...scope,
  eventId: eventID,
  body: { attempt_count: 4, reason: "retry after inspection" },
  key: "stable-key",
};

afterEach(() => {
  clearAllPendingDeliveryRetries();
  vi.unstubAllGlobals();
});

describe("page-session notification delivery retry state", () => {
  it("freezes a detached JSON-safe snapshot of a Vue reactive retry body", () => {
    const form = reactive({ attempt_count: 4, reason: "retry after inspection" });
    const body = freezeDeliveryRetryBody(form);
    form.attempt_count = 9;
    form.reason = "changed";
    expect(body).toEqual({ attempt_count: 4, reason: "retry after inspection" });
    expect(Object.isFrozen(body)).toBe(true);
    expect(structuredClone(body)).toEqual({ attempt_count: 4, reason: "retry after inspection" });
  });

  it("retains an unknown intent through remount and brand switches only in page memory", () => {
    const getItem = vi.fn(() => { throw new Error("disk-backed storage must not be accessed"); });
    const setItem = vi.fn(() => { throw new Error("disk-backed storage must not be accessed"); });
    vi.stubGlobal("sessionStorage", { getItem, setItem });
    setPendingDeliveryRetry(scope, intent);

    // A remounted view reads the module's page-session map using the same account/brand scope.
    const remounted = getPendingDeliveryRetry<PendingDeliveryRetry>(scopeKey(accountA, brandA));
    expect(remounted).toEqual(intent);
    expect(Object.isFrozen(remounted)).toBe(true);
    expect(Object.isFrozen(remounted?.body)).toBe(true);
    expect(getPendingDeliveryRetry({ accountId: accountA, brandId: brandB })).toBeNull();
    expect(getPendingDeliveryRetry({ accountId: accountB, brandId: brandA })).toBeNull();
    expect(getItem).not.toHaveBeenCalled();
    expect(setItem).not.toHaveBeenCalled();
  });

  it("rejects cross-scope or malformed intents and clears by brand, account, or session", () => {
    setPendingDeliveryRetry(scope, { ...intent, accountId: accountB });
    expect(getPendingDeliveryRetry(scope)).toBeNull();
    setPendingDeliveryRetry(scope, { ...intent, eventId: "not-a-uuid" });
    expect(getPendingDeliveryRetry(scope)).toBeNull();

    setPendingDeliveryRetry(scope, intent);
    const otherScope = { accountId: accountA, brandId: brandB };
    setPendingDeliveryRetry(otherScope, { ...intent, brandId: brandB });
    clearPendingDeliveryRetry(scopeKey(accountA, brandA));
    expect(getPendingDeliveryRetry(scope)).toBeNull();
    expect(getPendingDeliveryRetry(otherScope)).not.toBeNull();
    clearAllPendingDeliveryRetries();
    expect(getPendingDeliveryRetry(otherScope)).toBeNull();
  });

  it("classifies unknown outcomes and keeps conflicts definitive", () => {
    expect(classifyDeliveryRetryFailure(undefined)).toBe("uncertain");
    expect(classifyDeliveryRetryFailure(0)).toBe("uncertain");
    expect(classifyDeliveryRetryFailure(502)).toBe("uncertain");
    expect(classifyDeliveryRetryFailure(503)).toBe("uncertain");
    expect(classifyDeliveryRetryFailure(409)).toBe("definitive");
    expect(classifyDeliveryRetryFailure(403)).toBe("definitive");
  });

  it("does not let an old acknowledgement clear a replacement intent after logout", () => {
    setPendingDeliveryRetry(scope,intent);
    clearAllPendingDeliveryRetries();
    const replacement={...intent,key:"new-session-key"};
    setPendingDeliveryRetry(scope,replacement);
    clearPendingDeliveryRetry(scope,intent.key);
    expect(getPendingDeliveryRetry(scope)).toEqual(replacement);
    clearPendingDeliveryRetry(scope,replacement.key);
    expect(getPendingDeliveryRetry(scope)).toBeNull();
  });
});
