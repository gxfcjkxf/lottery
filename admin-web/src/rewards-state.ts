import type { AdminAccount } from "./admin-api";
import type { RewardActionBody, RewardGrantBody } from "./rewards-api";

export type RewardOperation = "grant" | "revoke" | "retry";
export interface RewardIntentScope { accountId: string; brandId: string; operation: RewardOperation; targetId?: string }
export interface PendingRewardIntent extends RewardIntentScope {
  readonly body: Readonly<RewardGrantBody | RewardActionBody>;
  readonly key: string;
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const KEY = /^[A-Za-z0-9._:-]{8,128}$/;
const MAX_INT64 = 9_223_372_036_854_775_807n;
const pending = new Map<string, PendingRewardIntent>();
const conflicted = new Set<string>();
let sessionGeneration = 0;

function intentKey(scope: RewardIntentScope): string {
  return JSON.stringify([scope.accountId, scope.brandId, scope.operation, scope.targetId ?? null]);
}
function validScope(scope: RewardIntentScope): boolean {
  return UUID.test(scope.accountId) && UUID.test(scope.brandId) && (scope.operation === "grant"
    ? scope.targetId === undefined
    : (scope.operation === "revoke" || scope.operation === "retry") && typeof scope.targetId === "string" && UUID.test(scope.targetId));
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
function canonicalBody(operation: RewardOperation, body: RewardGrantBody | RewardActionBody): Readonly<RewardGrantBody | RewardActionBody> {
  return Object.freeze(operation === "grant"
    ? { member_id: (body as RewardGrantBody).member_id, points: (body as RewardGrantBody).points, reason: body.reason }
    : { version: (body as RewardActionBody).version, reason: body.reason });
}
function validIntent(intent: PendingRewardIntent, scope: RewardIntentScope): boolean {
  if (!validScope(scope) || !validScope(intent) || intentKey(intent) !== intentKey(scope) || !KEY.test(intent.key) ||
    !intent.body || typeof intent.body !== "object" || Array.isArray(intent.body) || !validReason(intent.body.reason)) return false;
  if (scope.operation === "grant") {
    const body = intent.body as RewardGrantBody;
    if (Object.keys(body).sort().join(",") !== "member_id,points,reason" || !UUID.test(body.member_id) || typeof body.points !== "string" || !/^[1-9][0-9]*$/.test(body.points)) return false;
    try { return BigInt(body.points) <= MAX_INT64; } catch { return false; }
  }
  const body = intent.body as RewardActionBody;
  return Object.keys(body).sort().join(",") === "reason,version" && Number.isSafeInteger(body.version) && body.version >= 1 && body.version < Number.MAX_SAFE_INTEGER;
}
function copy(intent: PendingRewardIntent): PendingRewardIntent {
  return Object.freeze({ accountId: intent.accountId, brandId: intent.brandId, operation: intent.operation,
    ...(intent.targetId === undefined ? {} : { targetId: intent.targetId }), body: canonicalBody(intent.operation, intent.body), key: intent.key });
}

export function rewardPermissions(account: AdminAccount, brand: string) {
  const member = account.brand_ids.includes(brand);
  const brandPermissions = new Set(account.permissions_by_brand === undefined ? (member ? account.permissions : []) : account.permissions_by_brand[brand] ?? []);
  const platformPermissions = new Set(account.platform_permissions ?? account.permissions ?? []);
  const view = UUID.test(account.id) && UUID.test(brand) && (account.super_admin
    ? platformPermissions.has("reward.view.platform")
    : (member && brandPermissions.has("reward.view.brand")) || platformPermissions.has("reward.view.platform"));
  return {
    view,
    grant: Boolean(view && member && !account.super_admin && brandPermissions.has("reward.grant.brand")),
    revoke: Boolean(view && member && !account.super_admin && brandPermissions.has("reward.revoke.brand")),
    retry: Boolean(view && member && !account.super_admin && brandPermissions.has("reward.retry.brand")),
  };
}

export function freezeRewardBody<T extends RewardGrantBody | RewardActionBody>(operation: RewardOperation, body: T): Readonly<T> {
  return canonicalBody(operation, body) as Readonly<T>;
}
export function createRewardIntent(scope: RewardIntentScope, body: RewardGrantBody | RewardActionBody, key: string): PendingRewardIntent | null {
  const intent = Object.freeze({ ...scope, body: canonicalBody(scope.operation, body), key });
  return validIntent(intent, scope) ? intent : null;
}
export function getPendingRewardIntent(scope: RewardIntentScope): PendingRewardIntent | null {
  if (!validScope(scope)) return null;
  const intent = pending.get(intentKey(scope));
  return intent && validIntent(intent, scope) ? copy(intent) : null;
}
export function listPendingRewardIntents(accountId: string, brandId: string): PendingRewardIntent[] {
  if (!UUID.test(accountId) || !UUID.test(brandId)) return [];
  return [...pending.values()].filter((intent) => intent.accountId === accountId && intent.brandId === brandId && validIntent(intent, intent)).map(copy);
}
export function setPendingRewardIntent(scope: RewardIntentScope, intent: PendingRewardIntent): void {
  if (validIntent(intent, scope)) pending.set(intentKey(scope), copy(intent));
}
export function markRewardIntentConflict(intent: PendingRewardIntent): void { conflicted.add(intent.key); }
export function isRewardIntentConflict(intent: PendingRewardIntent): boolean { return conflicted.has(intent.key); }
export function clearPendingRewardIntent(scope: RewardIntentScope, expectedKey?: string): void {
  const key = intentKey(scope);
  if (expectedKey === undefined || pending.get(key)?.key === expectedKey) {
    const oldKey = pending.get(key)?.key;
    pending.delete(key);
    if (expectedKey !== undefined) conflicted.delete(expectedKey);
    else if (oldKey) conflicted.delete(oldKey);
  }
}
export function hideRewardIntentsForScope(_accountId: string, _brandId: string): void { sessionGeneration++; }
export function clearAllRewardIntents(): void { pending.clear(); conflicted.clear(); sessionGeneration++; }
export function rewardSessionGeneration(): number { return sessionGeneration; }
export function classifyRewardWriteFailure(status: number | undefined): "unknown" | "conflict" | "definitive" {
  if (status === 409) return "conflict";
  return status === undefined || status === 0 || status >= 500 ? "unknown" : "definitive";
}
