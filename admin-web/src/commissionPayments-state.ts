import type { CommissionPaymentActionBody, CommissionPaymentPolicyBody } from "./commissionPayments-api";

export type CommissionPaymentOperation = "policy" | "approve" | "retry";
export interface CommissionPaymentIntentScope { accountId: string; brandId: string; operation: CommissionPaymentOperation; targetId?: string }
export interface PendingCommissionPaymentIntent extends CommissionPaymentIntentScope {
  readonly body: Readonly<CommissionPaymentPolicyBody | CommissionPaymentActionBody>;
  readonly key: string;
}
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const KEY = /^[A-Za-z0-9._:-]{8,128}$/;
const pending = new Map<string, PendingCommissionPaymentIntent>();
const conflicted = new Set<string>();
let sessionGeneration = 0;

export function commissionPaymentIntentKey(scope: CommissionPaymentIntentScope): string {
  return JSON.stringify([scope.accountId, scope.brandId, scope.operation, scope.targetId ?? null]);
}
function validScope(scope: CommissionPaymentIntentScope): boolean {
  return UUID.test(scope.accountId) && UUID.test(scope.brandId) && (scope.operation === "policy"
    ? scope.targetId === undefined
    : (scope.operation === "approve" || scope.operation === "retry") && typeof scope.targetId === "string" && UUID.test(scope.targetId));
}
function validReason(reason: unknown): reason is string {
  if (typeof reason !== "string" || !reason || reason.trim() !== reason || new TextEncoder().encode(reason).length > 500 || /[\u0000-\u001f\u007f-\u009f]/u.test(reason)) return false;
  for (let i = 0; i < reason.length; i++) {
    const code = reason.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff) { const next = reason.charCodeAt(++i); if (!(next >= 0xdc00 && next <= 0xdfff)) return false; }
    else if (code >= 0xdc00 && code <= 0xdfff) return false;
  }
  return true;
}
function canonicalBody(operation: CommissionPaymentOperation, body: CommissionPaymentPolicyBody | CommissionPaymentActionBody): Readonly<CommissionPaymentPolicyBody | CommissionPaymentActionBody> {
  return Object.freeze(operation === "policy"
    ? { version: body.version, enabled: (body as CommissionPaymentPolicyBody).enabled, reason: body.reason }
    : { version: body.version, reason: body.reason });
}
function validIntent(intent: PendingCommissionPaymentIntent, scope: CommissionPaymentIntentScope): boolean {
  if (!validScope(scope) || !validScope(intent) || commissionPaymentIntentKey(intent) !== commissionPaymentIntentKey(scope) || !KEY.test(intent.key) || !intent.body || typeof intent.body !== "object" || Array.isArray(intent.body) || !validReason(intent.body.reason)) return false;
  if (!Number.isSafeInteger(intent.body.version) || intent.body.version < 1 || intent.body.version >= Number.MAX_SAFE_INTEGER) return false;
  return scope.operation === "policy"
    ? Object.keys(intent.body).sort().join(",") === "enabled,reason,version" && typeof (intent.body as CommissionPaymentPolicyBody).enabled === "boolean"
    : Object.keys(intent.body).sort().join(",") === "reason,version";
}
function copy(intent: PendingCommissionPaymentIntent): PendingCommissionPaymentIntent {
  return Object.freeze({ accountId: intent.accountId, brandId: intent.brandId, operation: intent.operation,
    ...(intent.targetId === undefined ? {} : { targetId: intent.targetId }), body: canonicalBody(intent.operation, intent.body), key: intent.key });
}
export function getPendingCommissionPaymentIntent(scope: CommissionPaymentIntentScope): PendingCommissionPaymentIntent | null {
  if (!validScope(scope)) return null;
  const intent = pending.get(commissionPaymentIntentKey(scope));
  return intent && validIntent(intent, scope) ? copy(intent) : null;
}
export function listPendingCommissionPaymentIntents(accountId: string, brandId: string): PendingCommissionPaymentIntent[] {
  if (!UUID.test(accountId) || !UUID.test(brandId)) return [];
  return [...pending.values()].filter((intent) => intent.accountId === accountId && intent.brandId === brandId && validIntent(intent, intent)).map(copy);
}
export function setPendingCommissionPaymentIntent(scope: CommissionPaymentIntentScope, intent: PendingCommissionPaymentIntent): void {
  if (validIntent(intent, scope)) pending.set(commissionPaymentIntentKey(scope), copy(intent));
}
export function markCommissionPaymentIntentConflict(intent: PendingCommissionPaymentIntent): void { conflicted.add(intent.key); }
export function isCommissionPaymentIntentConflict(intent: PendingCommissionPaymentIntent): boolean { return conflicted.has(intent.key); }
export function clearPendingCommissionPaymentIntent(scope: CommissionPaymentIntentScope, expectedKey?: string): void {
  const key = commissionPaymentIntentKey(scope);
  if (expectedKey === undefined || pending.get(key)?.key === expectedKey) { pending.delete(key); if (expectedKey !== undefined) conflicted.delete(expectedKey); }
}
export function hideCommissionPaymentIntentsForScope(accountId: string, brandId: string): void {
  if (!UUID.test(accountId) || !UUID.test(brandId)) return;
  // Invalidate in-flight readers/writers, but retain the frozen request. The
  // component hides it while unauthorized; restoring authority requires a new
  // explicit review, never automatic replay or generating a replacement key.
  sessionGeneration++;
}
export function clearAllCommissionPaymentIntents(): void { pending.clear(); conflicted.clear(); sessionGeneration++; }
export function commissionPaymentSessionGeneration(): number { return sessionGeneration; }
export function freezeCommissionPaymentBody<T extends CommissionPaymentPolicyBody | CommissionPaymentActionBody>(operation: CommissionPaymentOperation, body: T): Readonly<T> {
  return canonicalBody(operation, body) as Readonly<T>;
}
export function classifyCommissionPaymentWriteFailure(status: number | undefined): "unknown" | "conflict" | "definitive" {
  if (status === 409) return "conflict";
  return status === undefined || status === 0 || status >= 500 ? "unknown" : "definitive";
}
export function createCommissionPaymentIntent(scope: CommissionPaymentIntentScope, body: CommissionPaymentPolicyBody | CommissionPaymentActionBody, key: string): PendingCommissionPaymentIntent | null {
  const intent = Object.freeze({ ...scope, body: canonicalBody(scope.operation, body), key });
  return validIntent(intent, scope) ? intent : null;
}
