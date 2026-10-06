import { validJoinCodeDate, validJoinCodeDateRange, type CreateJoinCodeBody, type JoinCodeKind, type JoinCodeStatus, type UpdateJoinCodeBody } from "./join-codes-api";

export type JoinCodeWriteOperation = "create" | "update";
export type JoinCodeWriteBody = CreateJoinCodeBody | UpdateJoinCodeBody;
export interface JoinCodeIdentity {
  kind: JoinCodeKind;
  code: string | null;
  owner_member_id: string;
  agent_id: string | null;
}
export interface PendingJoinCodeWrite {
  readonly accountId: string;
  readonly brandId: string;
  readonly operation: JoinCodeWriteOperation;
  readonly resourceId: string | null;
  readonly identity: Readonly<JoinCodeIdentity>;
  readonly body: Readonly<JoinCodeWriteBody>;
  readonly key: string;
}
export interface JoinCodeWriteScope { accountId: string; brandId: string }
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const CODE_RE = /^[0-9A-F]{24}$/;
const pendingWrites = new Map<string, PendingJoinCodeWrite>();

export function scopeKey(accountId: string, brandId: string): string { return JSON.stringify([accountId, brandId]); }
function resolveScope(scope: string | JoinCodeWriteScope): (JoinCodeWriteScope & { key: string }) | null {
  if (typeof scope !== "string") return { ...scope, key: scopeKey(scope.accountId, scope.brandId) };
  try {
    const parsed: unknown = JSON.parse(scope);
    if (Array.isArray(parsed) && parsed.length === 2 && parsed.every((part) => typeof part === "string")) return { accountId: parsed[0], brandId: parsed[1], key: scope };
  } catch { /* Arbitrary strings cannot address a pending intent. */ }
  return null;
}
function validRange(start: unknown, end: unknown): boolean { return (start === null || validJoinCodeDate(start)) && (end === null || validJoinCodeDate(end)) && validJoinCodeDateRange(start as string | null, end as string | null); }
function validReason(value: unknown): boolean { return typeof value === "string" && value.trim().length > 0 && new TextEncoder().encode(value).length <= 500; }
function validBody(operation: JoinCodeWriteOperation, value: unknown): value is JoinCodeWriteBody {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const b = value as Record<string, unknown>;
  if (operation === "create") return (b.kind === "agent" || b.kind === "referral") && UUID_RE.test(String(b.owner_member_id)) &&
    (b.kind === "agent" ? UUID_RE.test(String(b.agent_id)) : b.agent_id === null) && validRange(b.starts_at, b.expires_at) && validReason(b.reason);
  return Number.isSafeInteger(b.version) && Number(b.version) >= 1 && (b.status === "active" || b.status === "disabled") &&
    validRange(b.starts_at, b.expires_at) && validReason(b.reason);
}
function validIdentity(operation: JoinCodeWriteOperation, value: unknown, body: JoinCodeWriteBody): value is JoinCodeIdentity {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const identity = value as Record<string, unknown>;
  if ((identity.kind !== "agent" && identity.kind !== "referral") || !UUID_RE.test(String(identity.owner_member_id)) ||
      !(identity.agent_id === null || UUID_RE.test(String(identity.agent_id))) ||
      (identity.kind === "agent" ? !UUID_RE.test(String(identity.agent_id)) : identity.agent_id !== null)) return false;
  if (operation === "create") return identity.code === null && identity.kind === (body as CreateJoinCodeBody).kind &&
    identity.owner_member_id === (body as CreateJoinCodeBody).owner_member_id && identity.agent_id === (body as CreateJoinCodeBody).agent_id;
  return typeof identity.code === "string" && CODE_RE.test(identity.code);
}
function validIntent(value: unknown, scope: JoinCodeWriteScope): value is PendingJoinCodeWrite {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const c = value as Record<string, unknown>, operation = c.operation;
  if (c.accountId !== scope.accountId || c.brandId !== scope.brandId || !UUID_RE.test(scope.accountId) || !UUID_RE.test(scope.brandId) ||
      (operation !== "create" && operation !== "update") || (operation === "create" ? c.resourceId !== null : !UUID_RE.test(String(c.resourceId))) ||
      typeof c.key !== "string" || c.key.length === 0 || !validBody(operation, c.body) || !validIdentity(operation, c.identity, c.body)) return false;
  return true;
}
/** JSON round-tripping unwraps Vue proxies and creates a detached immutable snapshot. */
export function freezeJoinCodeBody<T extends object>(body: T): Readonly<T> {
  let copy: T;
  try { copy = JSON.parse(JSON.stringify(body)) as T; }
  catch { throw new TypeError("Join code write body must be JSON serializable"); }
  const freeze = (value: unknown): void => {
    if (!value || typeof value !== "object" || Object.isFrozen(value)) return;
    Object.values(value).forEach(freeze); Object.freeze(value);
  };
  freeze(copy); return copy;
}
function copyIntent(intent: PendingJoinCodeWrite): PendingJoinCodeWrite {
  return Object.freeze({ ...intent, identity: Object.freeze({ ...intent.identity }), body: freezeJoinCodeBody(intent.body) });
}
export function getPendingJoinCodeWrite(scope: string | JoinCodeWriteScope): PendingJoinCodeWrite | null {
  const resolved = resolveScope(scope);
  if (!resolved || !UUID_RE.test(resolved.accountId) || !UUID_RE.test(resolved.brandId)) return null;
  const intent = pendingWrites.get(resolved.key);
  return intent && validIntent(intent, resolved) ? intent : null;
}
export function setPendingJoinCodeWrite(scope: string | JoinCodeWriteScope, intent: PendingJoinCodeWrite): void {
  const resolved = resolveScope(scope);
  if (resolved && validIntent(intent, resolved)) pendingWrites.set(resolved.key, copyIntent(intent));
}
export function clearPendingJoinCodeWrite(scope: string | JoinCodeWriteScope, expectedKey?: string): void {
  const resolved = resolveScope(scope);
  if (resolved && (expectedKey === undefined || pendingWrites.get(resolved.key)?.key === expectedKey)) pendingWrites.delete(resolved.key);
}
/** Pending writes are page-session memory only and are cleared on logout/auth scope change. */
export function clearAllPendingJoinCodeWrites(): void { pendingWrites.clear(); }
export type JoinCodeWriteFailure = "uncertain" | "definitive";
export function classifyJoinCodeWriteFailure(status: number | undefined): JoinCodeWriteFailure {
  return status === undefined || status === 0 || status >= 500 ? "uncertain" : "definitive";
}
export type { JoinCodeStatus };
