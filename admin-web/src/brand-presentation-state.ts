import { validBrandPresentationConfig, type BrandPresentationConfig, type BrandPresentationPutBody } from "./brand-presentation-api";

export interface BrandPresentationScope { accountId: string; brandId: string }
export type PendingBrandPresentationPhase = "review" | "unknown" | "conflict";
export interface PendingBrandPresentationWrite extends BrandPresentationScope {
  readonly body: Readonly<BrandPresentationPutBody>;
  readonly key: string;
  readonly phase: PendingBrandPresentationPhase;
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const pending = new Map<string, PendingBrandPresentationWrite>();
let sessionGeneration = 0;
let requestGeneration = 0;

export function brandPresentationScopeKey(accountId: string, brandId: string): string { return JSON.stringify([accountId, brandId]); }
function scopeFrom(scope: string | BrandPresentationScope): (BrandPresentationScope & { key: string }) | null {
  if (typeof scope !== "string") return { ...scope, key: brandPresentationScopeKey(scope.accountId, scope.brandId) };
  try {
    const value: unknown = JSON.parse(scope);
    return Array.isArray(value) && value.length === 2 && value.every((item) => typeof item === "string") ? { accountId: value[0], brandId: value[1], key: scope } : null;
  } catch { return null; }
}
function isRecord(value: unknown): value is Record<string, unknown> { return Boolean(value && typeof value === "object" && !Array.isArray(value)); }
function valid(value: unknown, scope: BrandPresentationScope): value is PendingBrandPresentationWrite {
  if (!isRecord(value) || !isRecord(value.body)) return false;
  const body = value.body;
  return value.accountId === scope.accountId && value.brandId === scope.brandId && UUID_RE.test(scope.accountId) && UUID_RE.test(scope.brandId) &&
    typeof value.key === "string" && /^[A-Za-z0-9_:.-]{8,128}$/.test(value.key) && ["review", "unknown", "conflict"].includes(String(value.phase)) &&
    Number.isSafeInteger(body.version) && Number(body.version) >= 1 && validBrandPresentationConfig(body.config) && typeof body.reason === "string" && body.reason.trim().length > 0 && new TextEncoder().encode(body.reason).length <= 500;
}
function cloneConfig(config: BrandPresentationConfig): BrandPresentationConfig {
  return {
    ...config,
    available_locales: config.available_locales === null ? null : [...config.available_locales],
    content: config.content === null ? null : {
      en: { ...config.content.en }, "zh-CN": { ...config.content["zh-CN"] },
    },
  };
}
function copy(value: PendingBrandPresentationWrite): PendingBrandPresentationWrite {
  const config = cloneConfig(value.body.config);
  if (config.available_locales) Object.freeze(config.available_locales);
  if (config.content) {
    Object.freeze(config.content.en); Object.freeze(config.content["zh-CN"]); Object.freeze(config.content);
  }
  Object.freeze(config);
  return Object.freeze({ accountId: value.accountId, brandId: value.brandId, key: value.key, phase: value.phase,
    body: Object.freeze({ version: value.body.version, config, reason: value.body.reason }) });
}

export function getPendingBrandPresentationWrite(scope: string | BrandPresentationScope): PendingBrandPresentationWrite | null {
  const resolved = scopeFrom(scope);
  if (!resolved || !UUID_RE.test(resolved.accountId) || !UUID_RE.test(resolved.brandId)) return null;
  const value = pending.get(resolved.key);
  return value && valid(value, resolved) ? value : null;
}
export const getPendingPresentationWrite = getPendingBrandPresentationWrite;
export function setPendingBrandPresentationWrite(scope: string | BrandPresentationScope, value: PendingBrandPresentationWrite): void {
  const resolved = scopeFrom(scope);
  if (resolved && valid(value, resolved)) pending.set(resolved.key, copy(value));
}
export const setPendingPresentationWrite = setPendingBrandPresentationWrite;
export function updatePendingBrandPresentationPhase(scope: string | BrandPresentationScope, key: string, phase: PendingBrandPresentationPhase): void {
  const resolved = scopeFrom(scope), current = resolved ? pending.get(resolved.key) : undefined;
  if (resolved && current?.key === key && ["review", "unknown", "conflict"].includes(phase)) pending.set(resolved.key, copy({ ...current, phase }));
}
export const updatePendingPresentationPhase = updatePendingBrandPresentationPhase;
export function clearPendingBrandPresentationWrite(scope: string | BrandPresentationScope, expectedKey?: string): void {
  const resolved = scopeFrom(scope), current = resolved ? pending.get(resolved.key) : undefined;
  if (resolved && (expectedKey === undefined || current?.key === expectedKey)) pending.delete(resolved.key);
}
export const clearPendingPresentationWrite = clearPendingBrandPresentationWrite;
export function clearAllPendingPresentationWrites(): void { pending.clear(); sessionGeneration += 1; requestGeneration += 1; }
export function brandPresentationSessionGeneration(): number { return sessionGeneration; }
export function classifyBrandPresentationFailure(status: number | undefined): "unknown" | "conflict" | "definitive" {
  if (status === undefined || status === 0 || status >= 500) return "unknown";
  if (status === 409) return "conflict";
  return "definitive";
}
export const classifyPresentationFailure = classifyBrandPresentationFailure;
export function createBrandPresentationRequestGuard() {
  let current = 0;
  const bornInSession = sessionGeneration;
  return {
    begin(): number { current = ++requestGeneration; return current; },
    invalidate(): void { current = ++requestGeneration; },
    isCurrent(token: number): boolean { return token === current && bornInSession === sessionGeneration; },
  };
}
