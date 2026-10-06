import { afterEach, describe, expect, it, vi } from "vitest";
import { reactive } from "vue";
import {
  classifyNotificationTemplateFailure, clearAllPendingTemplateWrites, clearPendingNotificationTemplateWrite,
  freezeNotificationTemplateBody, getPendingNotificationTemplateWrite,
  notificationTemplateSessionGeneration, setPendingNotificationTemplateWrite,
  type NotificationTemplateWriteScope, type PendingNotificationTemplateWrite,
} from "./notification-templates-state";

const accountA = "00000000-0000-4000-8000-000000000001";
const accountB = "00000000-0000-4000-8000-000000000002";
const brandA = "00000000-0000-4000-8000-000000000003";
const brandB = "00000000-0000-4000-8000-000000000004";
const scope: NotificationTemplateWriteScope = { accountId: accountA, brandId: brandA, templateKey: "recharge.confirmed" };
const content = {
  en: { title: "Points", body: "Received {points}" },
  "zh-CN": { title: "积分", body: "获得 {points}" },
};
const intent: PendingNotificationTemplateWrite = {
  ...scope, body: { version: 2, content, reason: "retain this exact reason" }, key: "stable-template-key",
};

afterEach(() => {
  clearAllPendingTemplateWrites();
  vi.unstubAllGlobals();
});

describe("page-session notification template write state", () => {
  it("freezes a detached copy of the complete put body", () => {
    const form = reactive({ version: 2, content: { en: { ...content.en }, "zh-CN": { ...content["zh-CN"] } }, reason: "retain this exact reason" });
    const body = freezeNotificationTemplateBody(form);
    form.content.en.body = "mutated";
    form.reason = "mutated";
    expect(body).toEqual(intent.body);
    expect(Object.isFrozen(body)).toBe(true);
    expect(Object.isFrozen(body.content)).toBe(true);
    expect(Object.isFrozen(body.content.en)).toBe(true);
    expect(structuredClone(body)).toEqual(intent.body);
  });

  it("retains unknown intents across remounts without reading or writing browser storage", () => {
    const getItem = vi.fn(() => { throw new Error("disk storage must not be used"); });
    const setItem = vi.fn(() => { throw new Error("disk storage must not be used"); });
    vi.stubGlobal("sessionStorage", { getItem, setItem });
    setPendingNotificationTemplateWrite(scope, intent);
    const remounted = getPendingNotificationTemplateWrite({ ...scope });
    expect(remounted).toEqual(intent);
    expect(Object.isFrozen(remounted)).toBe(true);
    expect(Object.isFrozen(remounted?.body.content.en)).toBe(true);
    expect(getPendingNotificationTemplateWrite({ ...scope, templateKey: "bet.order.won" })).toBeNull();
    expect(getPendingNotificationTemplateWrite({ ...scope, brandId: brandB })).toBeNull();
    expect(getPendingNotificationTemplateWrite({ ...scope, accountId: accountB })).toBeNull();
    expect(getItem).not.toHaveBeenCalled();
    expect(setItem).not.toHaveBeenCalled();
  });

  it("clears only the matching key, supports global logout clearing, and advances session generation", () => {
    setPendingNotificationTemplateWrite(scope, intent);
    const other: NotificationTemplateWriteScope = { ...scope, brandId: brandB };
    setPendingNotificationTemplateWrite(other, { ...intent, brandId: brandB });
    clearPendingNotificationTemplateWrite(scope, "older-key");
    expect(getPendingNotificationTemplateWrite(scope)).toEqual(intent);
    clearPendingNotificationTemplateWrite(scope, intent.key);
    expect(getPendingNotificationTemplateWrite(scope)).toBeNull();
    expect(getPendingNotificationTemplateWrite(other)).not.toBeNull();
    const generation = notificationTemplateSessionGeneration();
    clearAllPendingTemplateWrites();
    expect(notificationTemplateSessionGeneration()).toBe(generation + 1);
    expect(getPendingNotificationTemplateWrite(other)).toBeNull();
  });

  it("rejects malformed or cross-scope intents and classifies only uncertain transport and server failures", () => {
    setPendingNotificationTemplateWrite(scope, { ...intent, accountId: accountB });
    expect(getPendingNotificationTemplateWrite(scope)).toBeNull();
    setPendingNotificationTemplateWrite(scope, { ...intent, body: { ...intent.body, content: { ...content, en: { title: "x", body: "{money} {points}" } } } });
    expect(getPendingNotificationTemplateWrite(scope)).toBeNull();
    expect(classifyNotificationTemplateFailure(undefined)).toBe("unknown");
    expect(classifyNotificationTemplateFailure(0)).toBe("unknown");
    expect(classifyNotificationTemplateFailure(503)).toBe("unknown");
    expect(classifyNotificationTemplateFailure(409)).toBe("definitive");
    expect(classifyNotificationTemplateFailure(403)).toBe("definitive");
  });

  it("does not let a late acknowledgement clear a replacement intent", () => {
    setPendingNotificationTemplateWrite(scope, intent);
    clearAllPendingTemplateWrites();
    const replacement = { ...intent, key: "replacement-template-key" };
    setPendingNotificationTemplateWrite(scope, replacement);
    clearPendingNotificationTemplateWrite(scope, intent.key);
    expect(getPendingNotificationTemplateWrite(scope)).toEqual(replacement);
  });
});
