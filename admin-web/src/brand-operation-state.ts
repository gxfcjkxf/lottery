import type { BrandOperationPatchBody } from "./brand-operation-api";

export interface BrandOperationScope { accountId: string; brandId: string }
export type PendingBrandOperationPhase = "review" | "unknown" | "conflict";
export interface PendingBrandOperationWrite extends BrandOperationScope {
  readonly body: Readonly<BrandOperationPatchBody>;
  readonly key: string;
  readonly phase: PendingBrandOperationPhase;
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const pending = new Map<string, PendingBrandOperationWrite>();
let sessionGeneration = 0;
let requestGeneration = 0;

export function brandOperationScopeKey(accountId: string, brandId: string): string {
  return JSON.stringify([accountId, brandId]);
}

function scopeFrom(scope: string | BrandOperationScope): (BrandOperationScope & { key: string }) | null {
  if (typeof scope !== "string") return { ...scope, key: brandOperationScopeKey(scope.accountId, scope.brandId) };
  try {
    const value: unknown = JSON.parse(scope);
    return Array.isArray(value) && value.length === 2 && value.every((item) => typeof item === "string")
      ? { accountId: value[0], brandId: value[1], key: scope }
      : null;
  } catch { return null; }
}

function valid(value: unknown, scope: BrandOperationScope): value is PendingBrandOperationWrite {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const item = value as Record<string, unknown>;
  if (!item.body || typeof item.body !== "object" || Array.isArray(item.body)) return false;
  const body = item.body as Record<string, unknown>;
  return item.accountId === scope.accountId && item.brandId === scope.brandId && UUID_RE.test(scope.accountId) && UUID_RE.test(scope.brandId) &&
    typeof item.key === "string" && item.key.length > 0 && ["review", "unknown", "conflict"].includes(String(item.phase)) &&
    Number.isSafeInteger(body.version) && Number(body.version) >= 1 && (body.status === "active" || body.status === "paused") &&
    typeof body.reason === "string" && body.reason.trim().length > 0 && new TextEncoder().encode(body.reason).length <= 500;
}

function copy(value: PendingBrandOperationWrite): PendingBrandOperationWrite {
  return Object.freeze({ accountId: value.accountId, brandId: value.brandId, key: value.key, phase: value.phase,
    body: Object.freeze({ version: value.body.version, status: value.body.status, reason: value.body.reason }) });
}

export function getPendingBrandOperationWrite(scope: string | BrandOperationScope): PendingBrandOperationWrite | null {
  const resolved = scopeFrom(scope);
  if (!resolved || !UUID_RE.test(resolved.accountId) || !UUID_RE.test(resolved.brandId)) return null;
  const value = pending.get(resolved.key);
  return value && valid(value, resolved) ? value : null;
}

export function setPendingBrandOperationWrite(scope: string | BrandOperationScope, value: PendingBrandOperationWrite): void {
  const resolved = scopeFrom(scope);
  if (resolved && valid(value, resolved)) pending.set(resolved.key, copy(value));
}

export function updatePendingBrandOperationPhase(scope: string | BrandOperationScope, key: string, phase: PendingBrandOperationPhase): void {
  const resolved = scopeFrom(scope);
  const current = resolved ? pending.get(resolved.key) : undefined;
  if (resolved && current?.key === key) pending.set(resolved.key, copy({ ...current, phase }));
}

export function clearPendingBrandOperationWrite(scope: string | BrandOperationScope, expectedKey?: string): void {
  const resolved = scopeFrom(scope);
  const current = resolved ? pending.get(resolved.key) : undefined;
  if (resolved && (expectedKey === undefined || current?.key === expectedKey)) pending.delete(resolved.key);
}

export function clearAllPendingBrandOperationWrites(): void {
  pending.clear();
  sessionGeneration += 1;
  requestGeneration += 1;
}

export function brandOperationSessionGeneration(): number { return sessionGeneration; }

export function classifyBrandOperationFailure(status: number | undefined): "unknown" | "conflict" | "definitive" {
  if (status === undefined || status === 0 || status >= 500) return "unknown";
  if (status === 409) return "conflict";
  return "definitive";
}

export function brandOperationCompletionMessage(liveStateRead: boolean): string {
  return liveStateRead
    ? "变更回执已核验；当前状态已独立重新读取。"
    : "变更回执已核验，但读取最新状态失败。当前显示可能不是最新状态，请重新读取。";
}

export function createBrandOperationRequestGuard() {
  let current = 0;
  const bornInSession = sessionGeneration;
  return {
    begin(): number { current = ++requestGeneration; return current; },
    invalidate(): void { current = ++requestGeneration; },
    isCurrent(token: number): boolean { return token === current && bornInSession === sessionGeneration; },
  };
}
