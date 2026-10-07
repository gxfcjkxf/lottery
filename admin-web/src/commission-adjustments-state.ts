import { validCommissionAdjustmentDelta, validCommissionAdjustmentPoints, validCommissionAdjustmentReason, type CommissionAdjustmentBody } from "./commission-adjustments-api";

export interface CommissionAdjustmentIntentScope { accountId: string; brandId: string; paymentId: string; targetId: string }
export interface PendingCommissionAdjustmentIntent extends CommissionAdjustmentIntentScope {
  readonly actorId: string;
  readonly pointsBefore: string;
  readonly body: Readonly<CommissionAdjustmentBody>;
  readonly key: string;
}
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const KEY = /^[A-Za-z0-9._:-]{8,128}$/;
const pending = new Map<string, PendingCommissionAdjustmentIntent>();
const conflicted = new Set<string>();
let sessionGeneration = 0;
export function commissionAdjustmentIntentKey(scope: CommissionAdjustmentIntentScope): string { return JSON.stringify([scope.accountId, scope.brandId, scope.paymentId, scope.targetId]); }
function validScope(scope: CommissionAdjustmentIntentScope) { return UUID.test(scope.accountId) && UUID.test(scope.brandId) && UUID.test(scope.paymentId) && UUID.test(scope.targetId); }
function freeze(intent: PendingCommissionAdjustmentIntent): PendingCommissionAdjustmentIntent {
  return Object.freeze({accountId:intent.accountId,brandId:intent.brandId,paymentId:intent.paymentId,targetId:intent.targetId,actorId:intent.actorId,pointsBefore:intent.pointsBefore,
    body:Object.freeze({version:intent.body.version,points:intent.body.points,reason:intent.body.reason}),key:intent.key});
}
function valid(intent: PendingCommissionAdjustmentIntent, scope: CommissionAdjustmentIntentScope) {
  return validScope(intent) && validScope(scope) && commissionAdjustmentIntentKey(intent) === commissionAdjustmentIntentKey(scope) && UUID.test(intent.actorId) && KEY.test(intent.key) &&
    Number.isSafeInteger(intent.body?.version) && intent.body.version >= 1 && intent.body.version < Number.MAX_SAFE_INTEGER &&
    validCommissionAdjustmentPoints(intent.pointsBefore) && validCommissionAdjustmentPoints(intent.body.points) && validCommissionAdjustmentDelta(intent.pointsBefore,intent.body.points) && validCommissionAdjustmentReason(intent.body.reason);
}
export function createCommissionAdjustmentIntent(scope: CommissionAdjustmentIntentScope, actorId: string, pointsBefore: string, body: CommissionAdjustmentBody, key: string): PendingCommissionAdjustmentIntent | null {
  const intent = { ...scope, actorId, pointsBefore, body, key } as PendingCommissionAdjustmentIntent;
  return valid(intent, scope) ? freeze(intent) : null;
}
export function getPendingCommissionAdjustmentIntent(scope: CommissionAdjustmentIntentScope): PendingCommissionAdjustmentIntent | null {
  if (!validScope(scope)) return null; const intent = pending.get(commissionAdjustmentIntentKey(scope)); return intent && valid(intent, scope) ? freeze(intent) : null;
}
export function listPendingCommissionAdjustmentIntents(accountId: string, brandId: string): PendingCommissionAdjustmentIntent[] {
  if (!UUID.test(accountId) || !UUID.test(brandId)) return [];
  return [...pending.values()].filter((intent) => intent.accountId === accountId && intent.brandId === brandId && valid(intent,intent)).map(freeze);
}
export function setPendingCommissionAdjustmentIntent(scope: CommissionAdjustmentIntentScope, intent: PendingCommissionAdjustmentIntent): boolean {
  if (!valid(intent,scope)) return false;
  const scopeKey = commissionAdjustmentIntentKey(scope), existing = pending.get(scopeKey);
  if (existing && existing.key !== intent.key) return false;
  pending.set(scopeKey,freeze(intent)); return true;
}
export function markCommissionAdjustmentConflict(intent: PendingCommissionAdjustmentIntent) { conflicted.add(intent.key); }
export function isCommissionAdjustmentConflict(intent: PendingCommissionAdjustmentIntent) { return conflicted.has(intent.key); }
export function clearPendingCommissionAdjustmentIntent(scope: CommissionAdjustmentIntentScope, expectedKey?: string) {
  const key = commissionAdjustmentIntentKey(scope); if (expectedKey === undefined || pending.get(key)?.key === expectedKey) { pending.delete(key); if (expectedKey) conflicted.delete(expectedKey); }
}
export function clearAllPendingCommissionAdjustments() { pending.clear(); conflicted.clear(); sessionGeneration++; }
export function commissionAdjustmentSessionGeneration() { return sessionGeneration; }
export function classifyCommissionAdjustmentFailure(status: number | undefined): "unknown" | "conflict" | "definitive" {
  if (status === 409) return "conflict"; return status === undefined || status === 0 || status >= 500 ? "unknown" : "definitive";
}
