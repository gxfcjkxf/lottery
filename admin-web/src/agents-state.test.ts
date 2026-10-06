import { afterEach, describe, expect, it, vi } from "vitest";
import { reactive } from "vue";
import type { PendingAgentWrite } from "./agents-state";
import {
  classifyAgentFailure,
  clearAllPendingAgentWrites,
  clearPendingAgentWrite,
  freezeAgentBody,
  getPendingAgentWrite,
  scopeKey,
  setPendingAgentWrite,
} from "./agents-state";

const accountA = "00000000-0000-4000-8000-000000000001";
const accountB = "00000000-0000-4000-8000-000000000002";
const brandA = "00000000-0000-4000-8000-000000000003";
const brandB = "00000000-0000-4000-8000-000000000004";
const agentId = "00000000-0000-4000-8000-000000000005";

function policyIntent(overrides: Partial<PendingAgentWrite> = {}): PendingAgentWrite {
  return {
    accountId: accountA,
    brandId: brandA,
    operation: "policy",
    resourceId: null,
    body: {
      version: 4,
      config: { enabled: true, max_depth: 5, ratio_cap: "0.25", mode: "loss", cycle: "monthly" },
      reason: "reviewed policy",
    },
    key: "stable-policy-key",
    ...overrides,
  };
}

afterEach(() => {
  clearAllPendingAgentWrites();
  vi.unstubAllGlobals();
});

describe("page-session agent write state", () => {
  it("freezes a detached Vue reactive body snapshot without changing the source", () => {
    const form = reactive({
      version: 4,
      config: { enabled: true, max_depth: 5, ratio_cap: "0.25", mode: "loss", cycle: "monthly" },
      reason: "reviewed policy",
    });
    const snapshot = freezeAgentBody(form);
    form.version = 9;
    form.config.ratio_cap = "0.5";
    form.reason = "different request";
    expect(snapshot).toEqual({
      version: 4,
      config: { enabled: true, max_depth: 5, ratio_cap: "0.25", mode: "loss", cycle: "monthly" },
      reason: "reviewed policy",
    });
    expect(Object.isFrozen(snapshot)).toBe(true);
    expect(Object.isFrozen(snapshot.config)).toBe(true);
    expect(structuredClone(snapshot)).toEqual(snapshot);
  });

  it("retains only a valid account/brand intent across remounts in memory", () => {
    const getItem = vi.fn(() => { throw new Error("storage must not be read"); });
    const setItem = vi.fn(() => { throw new Error("storage must not be written"); });
    vi.stubGlobal("sessionStorage", { getItem, setItem });
    const intent = policyIntent();
    setPendingAgentWrite({ accountId: accountA, brandId: brandA }, intent);
    const remounted = getPendingAgentWrite(scopeKey(accountA, brandA));
    expect(remounted).toEqual(intent);
    expect(Object.isFrozen(remounted)).toBe(true);
    expect(Object.isFrozen(remounted?.body)).toBe(true);
    expect(getPendingAgentWrite({ accountId: accountA, brandId: brandB })).toBeNull();
    expect(getPendingAgentWrite({ accountId: accountB, brandId: brandA })).toBeNull();
    expect(getItem).not.toHaveBeenCalled();
    expect(setItem).not.toHaveBeenCalled();
  });

  it("rejects malformed and foreign-scope intents and validates resource identity per operation", () => {
    const scope = { accountId: accountA, brandId: brandA };
    setPendingAgentWrite(scope, policyIntent({ accountId: accountB }));
    expect(getPendingAgentWrite(scope)).toBeNull();
    setPendingAgentWrite(scope, policyIntent({ body: { version: 4 } as PendingAgentWrite["body"] }));
    expect(getPendingAgentWrite(scope)).toBeNull();
    setPendingAgentWrite(scope, policyIntent({ resourceId: agentId }));
    expect(getPendingAgentWrite(scope)).toBeNull();
    setPendingAgentWrite(scope, policyIntent({ body: {
      version: 4,
      config: { enabled: true, max_depth: 5, ratio_cap: "0.25", mode: "loss", cycle: "monthly" },
      reason: "x".repeat(501),
    } }));
    expect(getPendingAgentWrite(scope)).toBeNull();
    setPendingAgentWrite(scope, policyIntent({
      operation: "update",
      resourceId: agentId,
      body: { version: 4, policy_version: 2, parent_version: null,
        config: { ratio: "0.1", mode: null, status: "active", can_create_children: false }, reason: "edit child" },
    }));
    expect(getPendingAgentWrite(scope)?.resourceId).toBe(agentId);
  });

  it("classifies network/server failures as uncertain and all HTTP client outcomes as definitive", () => {
    expect(classifyAgentFailure(undefined)).toBe("uncertain");
    expect(classifyAgentFailure(0)).toBe("uncertain");
    expect(classifyAgentFailure(503)).toBe("uncertain");
    expect(classifyAgentFailure(409)).toBe("definitive");
    expect(classifyAgentFailure(403)).toBe("definitive");
    expect(classifyAgentFailure(400)).toBe("definitive");
  });

  it("does not let a late acknowledgement remove a newer retry intent", () => {
    const scope = { accountId: accountA, brandId: brandA };
    const first = policyIntent();
    const replacement = policyIntent({ key: "replacement-key", body: {
      version: 5,
      config: { enabled: true, max_depth: 5, ratio_cap: "0.3", mode: "loss", cycle: "monthly" },
      reason: "new policy edit",
    } });
    setPendingAgentWrite(scope, first);
    clearAllPendingAgentWrites();
    setPendingAgentWrite(scope, replacement);
    clearPendingAgentWrite(scope, first.key);
    expect(getPendingAgentWrite(scope)).toEqual(replacement);
    clearPendingAgentWrite(scope, replacement.key);
    expect(getPendingAgentWrite(scope)).toBeNull();
  });
});
