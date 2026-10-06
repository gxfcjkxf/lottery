import type { BrandDomainPatchBody, BrandDomainPostBody } from "./brand-domains-api";

export interface BrandDomainsScope { accountId: string; brandId: string }
export type BrandDomainsPhase = "review" | "unknown" | "conflict";
export interface PendingBrandDomainsWrite extends BrandDomainsScope {
  readonly operation: "add" | "edit";
  readonly domainId?: string;
  readonly body: Readonly<BrandDomainPostBody | BrandDomainPatchBody>;
  readonly key: string;
  readonly phase: BrandDomainsPhase;
}
export interface BrandDomainsDraft extends BrandDomainsScope {
  readonly operation: "add" | "edit";
  readonly domainId?: string;
  readonly domain: string;
  readonly enabled: boolean;
  readonly is_primary: boolean;
  readonly reason: string;
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const pending = new Map<string, PendingBrandDomainsWrite>();
const drafts = new Map<string, BrandDomainsDraft>();
let sessionGeneration = 0;
let requestGeneration = 0;
export function brandDomainsScopeKey(accountId: string, brandId: string): string { return JSON.stringify([accountId, brandId]); }
function scopeKey(scope: BrandDomainsScope): string { return brandDomainsScopeKey(scope.accountId, scope.brandId); }
function validScope(value: BrandDomainsScope): boolean { return UUID_RE.test(value.accountId) && UUID_RE.test(value.brandId); }
function validIntent(value: unknown, scope: BrandDomainsScope): value is PendingBrandDomainsWrite {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const v = value as Record<string, unknown>, b = v.body as Record<string, unknown> | undefined;
  return Boolean(b && typeof b === "object" && !Array.isArray(b)) && v.accountId === scope.accountId && v.brandId === scope.brandId && validScope(scope) &&
    (v.operation === "add" || v.operation === "edit") && (v.operation === "add" ? v.domainId === undefined && typeof b?.domain === "string" : UUID_RE.test(String(v.domainId))) &&
    /^[A-Za-z0-9_:.-]{8,128}$/.test(String(v.key)) && ["review", "unknown", "conflict"].includes(String(v.phase)) && Number.isSafeInteger(b?.version) && Number(b?.version) >= 1 &&
    typeof b?.enabled === "boolean" && typeof b?.is_primary === "boolean" && (!b.is_primary || b.enabled) && typeof b?.reason === "string" && b.reason.trim().length > 0 && new TextEncoder().encode(b.reason).length <= 500;
}
function freezeIntent(value: PendingBrandDomainsWrite): PendingBrandDomainsWrite {
  const body = Object.freeze({ ...value.body });
  return Object.freeze({ accountId: value.accountId, brandId: value.brandId, operation: value.operation, ...(value.domainId ? { domainId: value.domainId } : {}), body, key: value.key, phase: value.phase });
}
export function getPendingBrandDomainsWrite(scope: BrandDomainsScope): PendingBrandDomainsWrite | null {
  if (!validScope(scope)) return null;
  const value = pending.get(scopeKey(scope));
  return value && validIntent(value, scope) ? value : null;
}
export function setPendingBrandDomainsWrite(scope: BrandDomainsScope, value: PendingBrandDomainsWrite): void { if (validScope(scope) && validIntent(value, scope)) pending.set(scopeKey(scope), freezeIntent(value)); }
export function retainPendingBrandDomainsWrite(scope: BrandDomainsScope, value: PendingBrandDomainsWrite, phase: Exclude<BrandDomainsPhase, "review">): boolean {
  const existing = getPendingBrandDomainsWrite(scope);
  if (existing && (existing.key !== value.key || existing.operation !== value.operation || existing.domainId !== value.domainId || JSON.stringify(existing.body) !== JSON.stringify(value.body))) return false;
  if (existing) updatePendingBrandDomainsPhase(scope, value.key, phase);
  else setPendingBrandDomainsWrite(scope, { ...value, phase });
  return getPendingBrandDomainsWrite(scope)?.key === value.key;
}
export function updatePendingBrandDomainsPhase(scope: BrandDomainsScope, key: string, phase: BrandDomainsPhase): void {
  const current = getPendingBrandDomainsWrite(scope);
  if (current?.key === key) pending.set(scopeKey(scope), freezeIntent({ ...current, phase }));
}
export function clearPendingBrandDomainsWrite(scope: BrandDomainsScope, expectedKey?: string): void {
  const current = pending.get(scopeKey(scope));
  if (current && (expectedKey === undefined || current.key === expectedKey)) pending.delete(scopeKey(scope));
}
export function getBrandDomainsDraft(scope: BrandDomainsScope): BrandDomainsDraft | null {
  if (!validScope(scope)) return null;
  const value = drafts.get(scopeKey(scope));
  return value && value.accountId === scope.accountId && value.brandId === scope.brandId ? value : null;
}
export function setBrandDomainsDraft(value: BrandDomainsDraft): void {
  if (validScope(value) && (value.operation === "add" || value.operation === "edit") && (value.operation === "add" ? !value.domainId : UUID_RE.test(String(value.domainId))) &&
    typeof value.domain === "string" && typeof value.enabled === "boolean" && typeof value.is_primary === "boolean" && typeof value.reason === "string") drafts.set(scopeKey(value), Object.freeze({ ...value }));
}
export function clearBrandDomainsDraft(scope: BrandDomainsScope): void { drafts.delete(scopeKey(scope)); }
export function clearAllPendingBrandDomainsWrites(): void { pending.clear(); drafts.clear(); sessionGeneration += 1; requestGeneration += 1; }
export function brandDomainsSessionGeneration(): number { return sessionGeneration; }
export function classifyBrandDomainsFailure(status: number | undefined): "unknown" | "conflict" | "definitive" { return status === undefined || status === 0 || status >= 500 ? "unknown" : status === 409 ? "conflict" : "definitive"; }
export function createBrandDomainsRequestGuard() {
  let current = 0;
  const bornInSession = sessionGeneration;
  return { begin(): number { current = ++requestGeneration; return current; }, invalidate(): void { current = ++requestGeneration; }, isCurrent(token: number): boolean { return token === current && bornInSession === sessionGeneration; } };
}
