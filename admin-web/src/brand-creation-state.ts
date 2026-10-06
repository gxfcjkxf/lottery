import { validateBrandCreationBody, type BrandCreationBody } from "./brand-creation-api";

export type BrandCreationPhase = "review" | "unknown" | "conflict";
export interface PendingBrandCreationWrite {
  readonly accountId: string;
  readonly body: Readonly<BrandCreationBody>;
  readonly key: string;
  readonly phase: BrandCreationPhase;
}
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const KEY_RE = /^[A-Za-z0-9_:.-]{8,128}$/;
const pending = new Map<string, PendingBrandCreationWrite>();
let sessionGeneration = 0;
let requestGeneration = 0;

function valid(value: unknown, accountId: string): value is PendingBrandCreationWrite {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const item = value as Record<string, unknown>;
  let bodyIsValid = false;
  try { validateBrandCreationBody(item.body as BrandCreationBody); bodyIsValid = true; } catch { /* invalid pending bodies are ignored */ }
  return item.accountId === accountId && UUID_RE.test(accountId) && bodyIsValid && typeof item.key === "string" && KEY_RE.test(item.key) &&
    ["review", "unknown", "conflict"].includes(String(item.phase));
}
function frozen(value: PendingBrandCreationWrite): PendingBrandCreationWrite {
  return Object.freeze({ accountId: value.accountId, body: Object.freeze({ ...value.body }), key: value.key, phase: value.phase });
}
export function getPendingBrandCreationWrite(accountId: string): PendingBrandCreationWrite | null {
  const value = pending.get(accountId);
  return value && valid(value, accountId) ? value : null;
}
export function retainPendingBrandCreationWrite(value: PendingBrandCreationWrite): boolean {
  if (!valid(value, value.accountId)) return false;
  const existing = getPendingBrandCreationWrite(value.accountId);
  if (existing) return existing.key === value.key && JSON.stringify(existing.body) === JSON.stringify(value.body);
  pending.set(value.accountId, frozen(value));
  return true;
}
export function updatePendingBrandCreationPhase(accountId: string, key: string, phase: BrandCreationPhase): void {
  const current = getPendingBrandCreationWrite(accountId);
  if (current?.key === key) pending.set(accountId, frozen({ ...current, phase }));
}
export function clearPendingBrandCreationWrite(accountId: string, expectedKey?: string): void {
  const current = getPendingBrandCreationWrite(accountId);
  if (current && (expectedKey === undefined || current.key === expectedKey)) pending.delete(accountId);
}
export function clearAllPendingBrandCreationWrites(): void { pending.clear(); sessionGeneration += 1; requestGeneration += 1; }
export function brandCreationSessionGeneration(): number { return sessionGeneration; }
export function classifyBrandCreationFailure(status: number | undefined, code?: string): "unknown" | "conflict" | "definitive" {
  if (status === undefined || status === 0 || status >= 500) return "unknown";
  if (status === 409 || code === "IDEMPOTENCY_CONFLICT" || code === "BRAND_CODE_CONFLICT") return "conflict";
  return "definitive";
}
export function createBrandCreationRequestGuard() {
  let current = 0; const bornInSession = sessionGeneration;
  return { begin(): number { current = ++requestGeneration; return current; }, invalidate(): void { current = ++requestGeneration; },
    isCurrent(token: number): boolean { return token === current && bornInSession === sessionGeneration; } };
}
