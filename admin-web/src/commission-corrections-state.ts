import type { CommissionCorrectionActionBody, CommissionCorrectionPolicyBody } from "./commission-corrections-api";

export type CommissionCorrectionOperation = "policy" | "plan_retry" | "approve" | "continue" | "execute_retry";
export interface CommissionCorrectionIntentScope {
  actorId: string;
  brandId: string;
  operation: CommissionCorrectionOperation;
  targetId?: string;
}
export interface PendingCommissionCorrectionIntent extends CommissionCorrectionIntentScope {
  readonly body: Readonly<CommissionCorrectionPolicyBody | CommissionCorrectionActionBody>;
  readonly key: string;
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const KEY = /^[A-Za-z0-9._:-]{8,128}$/;
const intents = new Map<string, PendingCommissionCorrectionIntent>();
const conflicts = new Set<string>();
let generation = 0;

export function commissionCorrectionIntentKey(scope: CommissionCorrectionIntentScope): string {
  return JSON.stringify([scope.actorId, scope.brandId, scope.operation, scope.targetId ?? null]);
}
export function commissionCorrectionContextKey(actorId: string, brandId: string): string { return JSON.stringify([actorId, brandId]); }
function validReason(reason: unknown): reason is string {
  if (typeof reason !== "string" || !reason || reason.trim() !== reason || new TextEncoder().encode(reason).length > 500 || /[\u0000-\u001f\u007f-\u009f]/u.test(reason)) return false;
  for (let i = 0; i < reason.length; i++) {
    const code = reason.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff) { const next = reason.charCodeAt(++i); if (!(next >= 0xdc00 && next <= 0xdfff)) return false; }
    else if (code >= 0xdc00 && code <= 0xdfff) return false;
  }
  return true;
}
function validScope(scope: CommissionCorrectionIntentScope): boolean {
  if (!UUID.test(scope.actorId) || !UUID.test(scope.brandId)) return false;
  return scope.operation === "policy" ? scope.targetId === undefined :
    ["plan_retry", "approve", "continue", "execute_retry"].includes(scope.operation) && typeof scope.targetId === "string" && UUID.test(scope.targetId);
}
function copyBody(scope: CommissionCorrectionIntentScope, body: CommissionCorrectionPolicyBody | CommissionCorrectionActionBody): Readonly<CommissionCorrectionPolicyBody | CommissionCorrectionActionBody> {
  return Object.freeze(scope.operation === "policy"
    ? { version: body.version, enabled: (body as CommissionCorrectionPolicyBody).enabled, reason: body.reason }
    : { version: body.version, reason: body.reason });
}
function validIntent(intent: PendingCommissionCorrectionIntent, expected: CommissionCorrectionIntentScope): boolean {
  if (!validScope(expected) || !validScope(intent) || commissionCorrectionIntentKey(intent) !== commissionCorrectionIntentKey(expected) || !KEY.test(intent.key) || !intent.body || typeof intent.body !== "object" || Array.isArray(intent.body) || !validReason(intent.body.reason)) return false;
  if (!Number.isSafeInteger(intent.body.version) || intent.body.version < 1 || intent.body.version >= Number.MAX_SAFE_INTEGER) return false;
  return expected.operation === "policy"
    ? Object.keys(intent.body).sort().join(",") === "enabled,reason,version" && typeof (intent.body as CommissionCorrectionPolicyBody).enabled === "boolean"
    : Object.keys(intent.body).sort().join(",") === "reason,version";
}
function frozenCopy(intent: PendingCommissionCorrectionIntent): PendingCommissionCorrectionIntent {
  return Object.freeze({ ...intent, ...(intent.targetId === undefined ? {} : { targetId: intent.targetId }), body: copyBody(intent, intent.body) });
}
export function createCommissionCorrectionIntent(scope: CommissionCorrectionIntentScope, body: CommissionCorrectionPolicyBody | CommissionCorrectionActionBody, key: string): PendingCommissionCorrectionIntent | null {
  const intent = Object.freeze({ ...scope, body: copyBody(scope, body), key });
  return validIntent(intent, scope) ? intent : null;
}
export function getPendingCommissionCorrectionIntent(scope: CommissionCorrectionIntentScope): PendingCommissionCorrectionIntent | null {
  if (!validScope(scope)) return null;
  const value = intents.get(commissionCorrectionIntentKey(scope));
  return value && validIntent(value, scope) ? frozenCopy(value) : null;
}
export function listPendingCommissionCorrectionIntents(actorId: string, brandId: string): PendingCommissionCorrectionIntent[] {
  if (!UUID.test(actorId) || !UUID.test(brandId)) return [];
  return [...intents.values()].filter((value) => value.actorId === actorId && value.brandId === brandId && validIntent(value, value)).map(frozenCopy);
}
export function setPendingCommissionCorrectionIntent(scope: CommissionCorrectionIntentScope, intent: PendingCommissionCorrectionIntent): void {
  if (validIntent(intent, scope)) intents.set(commissionCorrectionIntentKey(scope), frozenCopy(intent));
}
export function markCommissionCorrectionIntentConflict(intent: PendingCommissionCorrectionIntent): boolean {
  const slot = commissionCorrectionIntentKey(intent);
  if (intents.get(slot)?.key !== intent.key) return false;
  conflicts.add(slot);
  return true;
}
export function isCommissionCorrectionIntentConflict(intent: PendingCommissionCorrectionIntent): boolean {
  const slot = commissionCorrectionIntentKey(intent);
  return intents.get(slot)?.key === intent.key && conflicts.has(slot);
}
export function clearPendingCommissionCorrectionIntent(scope: CommissionCorrectionIntentScope, expectedKey?: string): boolean {
  const key = commissionCorrectionIntentKey(scope), current = intents.get(key);
  if (expectedKey !== undefined && current?.key !== expectedKey) return false;
  intents.delete(key);
  conflicts.delete(key);
  return true;
}
export function hideCommissionCorrectionIntentsForScope(_actorId: string, _brandId: string): void { /* Scope tickets invalidate component work; auth-session generation changes only on clearAll. */ }
export function clearAllCommissionCorrectionIntents(): void { intents.clear(); conflicts.clear(); generation++; }
export function commissionCorrectionSessionGeneration(): number { return generation; }
export function freezeCommissionCorrectionBody<T extends CommissionCorrectionPolicyBody | CommissionCorrectionActionBody>(scope: CommissionCorrectionIntentScope, body: T): Readonly<T> {
  return copyBody(scope, body) as Readonly<T>;
}
export function classifyCommissionCorrectionWriteFailure(status: number | undefined): "unknown" | "conflict" | "definitive" {
  if (status === 409) return "conflict";
  return status === undefined || status === 0 || status >= 500 ? "unknown" : "definitive";
}
