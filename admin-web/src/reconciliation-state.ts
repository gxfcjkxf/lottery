import type { ReconciliationCreateBody, ReconciliationRetryBody } from "./reconciliation-api";

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const KEY_RE = /^[A-Za-z0-9_:.-]{8,128}$/;
export type ReconciliationWriteOperation = "create" | "retry";
export interface ReconciliationWriteScope { accountId: string; brandId: string; operation: ReconciliationWriteOperation; jobId?: string }
export interface PendingReconciliationWrite extends ReconciliationWriteScope {
  readonly body: Readonly<ReconciliationCreateBody | ReconciliationRetryBody>;
  readonly key: string;
}
const pending = new Map<string, PendingReconciliationWrite>();
let generation = 0;

export function reconciliationWriteScopeKey(scope: ReconciliationWriteScope): string {
  return JSON.stringify([scope.accountId.toLowerCase(), scope.brandId.toLowerCase(), scope.operation, scope.jobId?.toLowerCase() ?? null]);
}
function validIntent(value: unknown, scope: ReconciliationWriteScope): value is PendingReconciliationWrite {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const item = value as Record<string, unknown>;
  if (!item.body || typeof item.body !== "object" || Array.isArray(item.body)) return false;
  const body = item.body as Record<string, unknown>;
  const common = typeof item.accountId === "string" && item.accountId.toLowerCase() === scope.accountId.toLowerCase() &&
    typeof item.brandId === "string" && item.brandId.toLowerCase() === scope.brandId.toLowerCase() && item.operation === scope.operation &&
    (item.jobId === undefined && scope.jobId === undefined || typeof item.jobId === "string" && scope.jobId !== undefined && item.jobId.toLowerCase() === scope.jobId.toLowerCase()) &&
    UUID_RE.test(scope.accountId) && UUID_RE.test(scope.brandId) &&
    typeof body.reason === "string" && body.reason.trim() === body.reason && body.reason.length > 0 &&
    new TextEncoder().encode(body.reason).length <= 500 && !/[\u0000-\u001f\u007f-\u009f]/u.test(body.reason) && !hasLoneSurrogate(body.reason) &&
    typeof item.key === "string" && KEY_RE.test(item.key);
  if (!common) return false;
  if (scope.operation === "create") return scope.jobId === undefined &&
    (Object.keys(body).length === 1 || Object.keys(body).length === 2 && Object.hasOwn(body, "check_scope") &&
      (body.check_scope === "wallet" || body.check_scope === "wallet_and_business"));
  return scope.jobId !== undefined && UUID_RE.test(scope.jobId) && Object.keys(body).length === 2 &&
    Number.isSafeInteger(body.version) && Number(body.version) > 0 && Number(body.version) < Number.MAX_SAFE_INTEGER;
}
function hasLoneSurrogate(value: string): boolean {
  for (let i = 0; i < value.length; i++) {
    const code = value.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff) {
      const next = value.charCodeAt(i + 1);
      if (!(next >= 0xdc00 && next <= 0xdfff)) return true;
      i++;
    } else if (code >= 0xdc00 && code <= 0xdfff) return true;
  }
  return false;
}
function copyIntent(value: PendingReconciliationWrite): PendingReconciliationWrite {
  const body = value.operation === "create"
    ? Object.freeze({ reason: value.body.reason, ...("check_scope" in value.body && value.body.check_scope !== undefined ? { check_scope: value.body.check_scope } : {}) })
    : Object.freeze({ version: (value.body as ReconciliationRetryBody).version, reason: value.body.reason });
  return Object.freeze({ accountId: value.accountId, brandId: value.brandId, operation: value.operation,
    ...(value.jobId === undefined ? {} : { jobId: value.jobId }), body, key: value.key });
}
export function freezeReconciliationBody<T extends ReconciliationCreateBody | ReconciliationRetryBody>(body: T): Readonly<T> {
  return Object.freeze("version" in body ? { version: body.version, reason: body.reason } as T :
    { reason: body.reason, ...("check_scope" in body && body.check_scope !== undefined ? { check_scope: body.check_scope } : {}) } as T);
}
export function getPendingReconciliationWrite(scope: ReconciliationWriteScope): PendingReconciliationWrite | null {
  if (!UUID_RE.test(scope.accountId) || !UUID_RE.test(scope.brandId) || (scope.jobId !== undefined && !UUID_RE.test(scope.jobId))) return null;
  const intent = pending.get(reconciliationWriteScopeKey(scope));
  return intent && validIntent(intent, scope) ? intent : null;
}
export function listPendingReconciliationWrites(accountId: string, brandId: string): PendingReconciliationWrite[] {
  if (!UUID_RE.test(accountId) || !UUID_RE.test(brandId)) return [];
  return [...pending.values()].filter((intent) => intent.accountId.toLowerCase() === accountId.toLowerCase() && intent.brandId.toLowerCase() === brandId.toLowerCase() &&
    validIntent(intent, intent)).map(copyIntent);
}
export function setPendingReconciliationWrite(scope: ReconciliationWriteScope, intent: PendingReconciliationWrite): void {
  if (validIntent(intent, scope)) pending.set(reconciliationWriteScopeKey(scope), copyIntent(intent));
}
export function clearPendingReconciliationWrite(scope: ReconciliationWriteScope, expectedKey?: string): void {
  const key = reconciliationWriteScopeKey(scope);
  if (expectedKey === undefined || pending.get(key)?.key === expectedKey) pending.delete(key);
}
/** Called by the parent on logout/session clear. */
export function clearPendingReconciliationWrites(): void { pending.clear(); generation += 1; }
export function reconciliationSessionGeneration(): number { return generation; }
export type ReconciliationWriteFailure = "unknown" | "definitive";
export function classifyReconciliationWriteFailure(status: number | undefined): ReconciliationWriteFailure {
  return status === undefined || status === 0 || status >= 500 ? "unknown" : "definitive";
}
