import { afterEach, describe, expect, it } from "vitest";
import { brandDomainsSessionGeneration, classifyBrandDomainsFailure, clearAllPendingBrandDomainsWrites, clearPendingBrandDomainsWrite, createBrandDomainsRequestGuard, getBrandDomainsDraft, getPendingBrandDomainsWrite, retainPendingBrandDomainsWrite, setBrandDomainsDraft, setPendingBrandDomainsWrite, updatePendingBrandDomainsPhase, type PendingBrandDomainsWrite } from "./brand-domains-state";

const scope = { accountId: "00000000-0000-4000-8000-000000000001", brandId: "00000000-0000-4000-8000-000000000002" };
const intent: PendingBrandDomainsWrite = { ...scope, operation: "add", body: { version: 3, domain: "shop.example.com", enabled: true, is_primary: true, reason: "Launch" }, key: "domain-key-0001", phase: "unknown" };
afterEach(() => clearAllPendingBrandDomainsWrites());

describe("brand domains scoped write state", () => {
  it("retains detached frozen writes and editable drafts across remounts within the same account and brand", () => {
    const mutable = { ...intent.body } as { version: number; domain: string; enabled: boolean; is_primary: boolean; reason: string };
    setPendingBrandDomainsWrite(scope, { ...intent, body: mutable });
    mutable.domain = "changed.example.com";
    const stored = getPendingBrandDomainsWrite(scope);
    expect(stored).toEqual(intent); expect(Object.isFrozen(stored)).toBe(true); expect(Object.isFrozen(stored?.body)).toBe(true);
    expect(getPendingBrandDomainsWrite({ ...scope, brandId: "00000000-0000-4000-8000-000000000003" })).toBeNull();
    const draft = { ...scope, operation: "edit" as const, domainId: "00000000-0000-4000-8000-000000000004", domain: "legacy.localhost", enabled: false, is_primary: false, reason: "Retire" };
    setBrandDomainsDraft(draft);
    expect(getBrandDomainsDraft(scope)).toEqual(draft);
    expect(getBrandDomainsDraft({ ...scope, accountId: "00000000-0000-4000-8000-000000000005" })).toBeNull();
  });
  it("updates phase and clears only the matching idempotency key", () => {
    setPendingBrandDomainsWrite(scope, intent); updatePendingBrandDomainsPhase(scope, intent.key, "conflict");
    expect(getPendingBrandDomainsWrite(scope)?.phase).toBe("conflict");
    const replacement = { ...intent, key: "domain-key-0002" };
    setPendingBrandDomainsWrite(scope, replacement); clearPendingBrandDomainsWrite(scope, intent.key);
    expect(getPendingBrandDomainsWrite(scope)).toEqual(replacement);
    clearPendingBrandDomainsWrite(scope, replacement.key); expect(getPendingBrandDomainsWrite(scope)).toBeNull();
  });
  it("does not retain hostile keys and restores an edit with its frozen target", () => {
    const edit: PendingBrandDomainsWrite = { ...scope, operation: "edit", domainId: "00000000-0000-4000-8000-000000000005", body: { version: 4, enabled: false, is_primary: false, reason: "Retire" }, key: "stable-key_02:.-", phase: "unknown" };
    setPendingBrandDomainsWrite(scope, edit);
    expect(getPendingBrandDomainsWrite(scope)).toEqual(edit);
    setPendingBrandDomainsWrite(scope, { ...edit, key: "hostile/key!!" });
    expect(getPendingBrandDomainsWrite(scope)).toEqual(edit);
  });
  it("restores an uncertain retry only into an empty or same-intent slot", () => {
    expect(retainPendingBrandDomainsWrite(scope, intent, "unknown")).toBe(true);
    expect(getPendingBrandDomainsWrite(scope)).toEqual(intent);
    clearPendingBrandDomainsWrite(scope, intent.key);
    expect(retainPendingBrandDomainsWrite(scope, intent, "unknown")).toBe(true);
    const newer = { ...intent, key: "newer-domain-key-01", body: { ...intent.body, reason: "New operator intent" } };
    expect(retainPendingBrandDomainsWrite(scope, newer, "unknown")).toBe(false);
    expect(getPendingBrandDomainsWrite(scope)).toEqual(intent);
    expect(retainPendingBrandDomainsWrite(scope, { ...intent, body: { ...intent.body, reason: "Changed frozen body" } }, "conflict")).toBe(false);
    expect(getPendingBrandDomainsWrite(scope)).toEqual(intent);
  });
  it("classifies transport and server failures without generating replacement keys", () => {
    expect(classifyBrandDomainsFailure(undefined)).toBe("unknown"); expect(classifyBrandDomainsFailure(0)).toBe("unknown");
    expect(classifyBrandDomainsFailure(500)).toBe("unknown"); expect(classifyBrandDomainsFailure(409)).toBe("conflict");
    expect(classifyBrandDomainsFailure(403)).toBe("definitive");
  });
  it("invalidates late callbacks after request and session changes", () => {
    const guard = createBrandDomainsRequestGuard(), old = guard.begin(), current = guard.begin();
    expect(guard.isCurrent(old)).toBe(false); expect(guard.isCurrent(current)).toBe(true);
    const session = brandDomainsSessionGeneration(); clearAllPendingBrandDomainsWrites();
    expect(brandDomainsSessionGeneration()).toBe(session + 1); expect(guard.isCurrent(current)).toBe(false);
  });
});
