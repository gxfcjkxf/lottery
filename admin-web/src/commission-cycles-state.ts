import type { CommissionCreateBody, CommissionRetryBody } from "./commission-cycles-api";

export type CommissionCycleWriteOperation = "create" | "retry_cycle" | "retry_discovery";
export interface CommissionWriteScope {
  accountId: string; brandId: string; operation: CommissionCycleWriteOperation; targetId?: string;
}
export interface PendingCommissionCycleWrite extends CommissionWriteScope {
  readonly body: Readonly<CommissionCreateBody | CommissionRetryBody>;
  readonly key: string;
}
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const keyPattern = /^[A-Za-z0-9_:.-]{8,128}$/;
const pending = new Map<string, PendingCommissionCycleWrite>();
let generation = 0;

export function commissionCycleWriteScopeKey(scope: CommissionWriteScope): string {
  return JSON.stringify([scope.accountId, scope.brandId, scope.operation, scope.targetId ?? null]);
}
function validScope(scope: CommissionWriteScope): boolean {
  return uuid.test(scope.accountId) && uuid.test(scope.brandId) && (scope.operation === "create"
    ? scope.targetId === undefined : ["retry_cycle", "retry_discovery"].includes(scope.operation) && typeof scope.targetId === "string" && uuid.test(scope.targetId));
}
function validReason(value: unknown): value is string {
  if (typeof value !== "string" || value.trim() !== value || value.length === 0 || new TextEncoder().encode(value).length > 500 || /[\u0000-\u001f\u007f-\u009f]/u.test(value)) return false;
  for (let i = 0; i < value.length; i++) {
    const code = value.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff) {
      const next = value.charCodeAt(++i);
      if (!(next >= 0xdc00 && next <= 0xdfff)) return false;
    } else if (code >= 0xdc00 && code <= 0xdfff) return false;
  }
  return true;
}
function validIntent(intent: PendingCommissionCycleWrite, scope: CommissionWriteScope): boolean {
  if (!validScope(scope) || commissionCycleWriteScopeKey(intent) !== commissionCycleWriteScopeKey(scope) || !keyPattern.test(intent.key) ||
    !intent.body || typeof intent.body !== "object" || Array.isArray(intent.body) || !validReason(intent.body.reason)) return false;
  const fields = Object.keys(intent.body).sort().join(",");
  if (scope.operation === "create") return fields === "anchor_order_id,reason" && "anchor_order_id" in intent.body && uuid.test(intent.body.anchor_order_id);
  return fields === "reason,version" && "version" in intent.body && Number.isSafeInteger(intent.body.version) && intent.body.version > 0 && intent.body.version < Number.MAX_SAFE_INTEGER;
}
// Canonical field order remains stable for the backend's raw-body fingerprint.
export function freezeCommissionCycleBody<T extends CommissionCreateBody | CommissionRetryBody>(body: T): Readonly<T> {
  return Object.freeze("anchor_order_id" in body ? { anchor_order_id: body.anchor_order_id, reason: body.reason } as T : { version: body.version, reason: body.reason } as T);
}
function copyIntent(intent: PendingCommissionCycleWrite): PendingCommissionCycleWrite {
  return Object.freeze({ accountId: intent.accountId, brandId: intent.brandId, operation: intent.operation,
    ...(intent.targetId === undefined ? {} : { targetId: intent.targetId }), key: intent.key, body: freezeCommissionCycleBody(intent.body) });
}
export function getPendingCommissionCycleWrite(scope: CommissionWriteScope): PendingCommissionCycleWrite | null {
  if (!validScope(scope)) return null;
  const intent = pending.get(commissionCycleWriteScopeKey(scope));
  return intent && validIntent(intent, scope) ? intent : null;
}
export function listPendingCommissionCycleWrites(accountId: string, brandId: string): PendingCommissionCycleWrite[] {
  if (!uuid.test(accountId) || !uuid.test(brandId)) return [];
  return [...pending.values()].filter(intent => intent.accountId === accountId && intent.brandId === brandId && validIntent(intent, intent)).map(copyIntent);
}
export function setPendingCommissionCycleWrite(scope: CommissionWriteScope, intent: PendingCommissionCycleWrite): void {
  if (validIntent(intent, scope)) pending.set(commissionCycleWriteScopeKey(scope), copyIntent(intent));
}
export function clearPendingCommissionCycleWrite(scope: CommissionWriteScope, expectedKey?: string): void {
  const scopeKey = commissionCycleWriteScopeKey(scope);
  if (expectedKey === undefined || pending.get(scopeKey)?.key === expectedKey) pending.delete(scopeKey);
}
/** Parent calls on session invalidation or account replacement, not navigation. */
export function clearAllPendingCommissionCycleWrites(): void { pending.clear(); generation++; }
export function commissionCycleSessionGeneration(): number { return generation; }
export function classifyCommissionCycleWriteFailure(status: number | undefined): "unknown" | "conflict" | "definitive" {
  if (status === 409) return "conflict";
  return status === undefined || status === 0 || status >= 500 ? "unknown" : "definitive";
}
