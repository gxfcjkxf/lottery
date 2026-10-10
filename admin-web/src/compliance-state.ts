import { validComplianceConfig, validComplianceReason, type ComplianceCheckBody, type ComplianceConfig, type CompliancePolicyPutBody } from "./compliance-api";

export interface ComplianceScope { accountId: string; brandId: string }
export type CompliancePhase = "review" | "unknown" | "conflict";
export interface PendingPolicyWrite extends ComplianceScope { readonly kind: "policy"; readonly body: Readonly<CompliancePolicyPutBody>; readonly key: string; readonly phase: CompliancePhase }
export interface PendingComplianceCheck extends ComplianceScope { readonly kind: "check"; readonly body: Readonly<ComplianceCheckBody>; readonly key: string; readonly phase: CompliancePhase }
export type PendingComplianceIntent = PendingPolicyWrite | PendingComplianceCheck;

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const pending = new Map<string, PendingComplianceIntent>();
let sessionGeneration = 0;
let requestGeneration = 0;
export function complianceScopeKey(accountId: string, brandId: string): string { return JSON.stringify([accountId, brandId]); }
function key(scope: ComplianceScope, kind: PendingComplianceIntent["kind"]): string { return `${complianceScopeKey(scope.accountId, scope.brandId)}:${kind}`; }
function validIntent(value: PendingComplianceIntent, scope: ComplianceScope, kind: PendingComplianceIntent["kind"]): boolean {
  if (value.accountId !== scope.accountId || value.brandId !== scope.brandId || value.kind !== kind || !UUID_RE.test(scope.accountId) || !UUID_RE.test(scope.brandId) || !/^[A-Za-z0-9_:.-]{8,128}$/.test(value.key) || !["review", "unknown", "conflict"].includes(value.phase)) return false;
  if (value.kind === "policy") return Number.isSafeInteger(value.body.version) && value.body.version >= 1 && validComplianceConfig(value.body.config) && validComplianceReason(value.body.reason);
  return Number.isSafeInteger(value.body.version) && value.body.version >= 1 && ["registration", "betting", "withdrawal"].includes(value.body.operation) && validComplianceReason(value.body.reason);
}
function cloneConfig(config: ComplianceConfig): ComplianceConfig { return { ...config, allowed_countries: [...config.allowed_countries] }; }
function copy(value: PendingComplianceIntent): PendingComplianceIntent {
  if (value.kind === "policy") {
    const config = cloneConfig(value.body.config);
    Object.freeze(config.allowed_countries); Object.freeze(config);
    return Object.freeze({ ...value, body: Object.freeze({ ...value.body, config }) });
  }
  return Object.freeze({ ...value, body: Object.freeze({ ...value.body }) });
}
export function getPendingComplianceIntent(scope: ComplianceScope, kind: "policy"): PendingPolicyWrite | null;
export function getPendingComplianceIntent(scope: ComplianceScope, kind: "check"): PendingComplianceCheck | null;
export function getPendingComplianceIntent(scope: ComplianceScope, kind: PendingComplianceIntent["kind"]): PendingComplianceIntent | null;
export function getPendingComplianceIntent(scope: ComplianceScope, kind: PendingComplianceIntent["kind"]): PendingComplianceIntent | null {
  const value = pending.get(key(scope, kind));
  return value?.kind === kind && validIntent(value, scope, kind) ? value : null;
}
function sameBody(a: PendingComplianceIntent, b: PendingComplianceIntent): boolean {
  if (a.kind !== b.kind) return false;
  if (a.kind === "check" && b.kind === "check") return a.body.version === b.body.version && a.body.operation === b.body.operation && a.body.reason === b.body.reason;
  if (a.kind !== "policy" || b.kind !== "policy") return false;
  const ac = a.body.config, bc = b.body.config;
  return a.body.version === b.body.version && a.body.reason === b.body.reason && ac.age_enabled === bc.age_enabled && ac.minimum_age === bc.minimum_age && ac.region_enabled === bc.region_enabled && ac.identity_enabled === bc.identity_enabled && ac.account_risk_enabled === bc.account_risk_enabled && ac.betting_risk_enabled === bc.betting_risk_enabled && ac.exclusion_enabled === bc.exclusion_enabled && ac.responsible_gambling_enabled === bc.responsible_gambling_enabled && ac.allowed_countries.length === bc.allowed_countries.length && ac.allowed_countries.every((country, i) => country === bc.allowed_countries[i]);
}
export function setPendingComplianceIntent(value: PendingComplianceIntent): boolean {
  if (!validIntent(value, value, value.kind)) return false;
  const id = key(value, value.kind), existing = pending.get(id);
  if (existing) return existing.key === value.key && sameBody(existing, value);
  pending.set(id, copy(value));
  return true;
}
export function updatePendingCompliancePhase(scope: ComplianceScope, kind: PendingComplianceIntent["kind"], idempotencyKey: string, phase: CompliancePhase): void {
  const current = pending.get(key(scope, kind));
  if (current?.key === idempotencyKey && ["review", "unknown", "conflict"].includes(phase)) pending.set(key(scope, kind), copy({ ...current, phase }));
}
export function clearPendingComplianceIntent(scope: ComplianceScope, kind: PendingComplianceIntent["kind"], expectedKey?: string): void { const id = key(scope, kind), current = pending.get(id); if (current && (expectedKey === undefined || current.key === expectedKey)) pending.delete(id); }
export function clearAllPendingComplianceIntents(): void { pending.clear(); sessionGeneration++; requestGeneration++; }
export function complianceSessionGeneration(): number { return sessionGeneration; }
export function classifyComplianceFailure(status: number | undefined, code?: string): "unknown" | "conflict" | "definitive" {
  if (status === undefined || status === 0 || status >= 500) return "unknown";
  if (status === 409 || code === "COMPLIANCE_VERSION_CONFLICT" || code === "COMPLIANCE_STATE_CONFLICT") return "conflict";
  return "definitive";
}
export function createComplianceRequestGuard() { let current = 0; const bornInSession = sessionGeneration; return { begin(): number { current = ++requestGeneration; return current; }, invalidate(): void { current = ++requestGeneration; }, isCurrent(token: number): boolean { return token === current && bornInSession === sessionGeneration; } }; }
