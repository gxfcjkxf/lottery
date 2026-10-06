import type {
  CreateAgentBody,
  SaveAgentPolicyBody,
  UpdateAgentBody,
} from "./agents-api";

export type AgentWriteBody = SaveAgentPolicyBody | CreateAgentBody | UpdateAgentBody;
export type AgentWriteOperation = "policy" | "create" | "update";

export interface PendingAgentWrite {
  readonly accountId: string;
  readonly brandId: string;
  readonly operation: AgentWriteOperation;
  readonly resourceId: string | null;
  readonly body: Readonly<AgentWriteBody>;
  readonly key: string;
}

export interface AgentWriteScope {
  accountId: string;
  brandId: string;
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const RATIO_RE = /^(?:0|1|0\.(?:[0-9]{0,5}[1-9]))$/;
const pendingWrites = new Map<string, PendingAgentWrite>();

function validUuid(value: unknown): value is string {
  return typeof value === "string" && UUID_RE.test(value);
}

export function scopeKey(accountId: string, brandId: string): string {
  return JSON.stringify([accountId, brandId]);
}

function resolveScope(scope: string | AgentWriteScope): (AgentWriteScope & { key: string }) | null {
  if (typeof scope !== "string")
    return { ...scope, key: scopeKey(scope.accountId, scope.brandId) };
  try {
    const parsed: unknown = JSON.parse(scope);
    if (Array.isArray(parsed) && parsed.length === 2 && parsed.every((part) => typeof part === "string"))
      return { accountId: parsed[0], brandId: parsed[1], key: scope };
  } catch {
    // Arbitrary strings cannot address an account/brand pending-write slot.
  }
  return null;
}

function validBody(operation: AgentWriteOperation, value: unknown): value is AgentWriteBody {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const body = value as Record<string, unknown>;
  const config = body.config;
  if (!config || typeof config !== "object" || Array.isArray(config)) return false;
  const conf = config as Record<string, unknown>;
  const ratio = operation === "policy" ? conf.ratio_cap : conf.ratio;
  if (typeof ratio !== "string" || !RATIO_RE.test(ratio) ||
      (ratio !== "0" && ratio !== "1" && ratio.split(".")[1].length > 6) ||
      !(conf.mode === null || conf.mode === "loss" || conf.mode === "turnover") ||
      typeof body.reason !== "string" || body.reason.trim().length === 0 ||
      new TextEncoder().encode(body.reason).length > 500) return false;
  if (operation === "policy") {
    return Number.isSafeInteger(body.version) && Number(body.version) >= 1 &&
      typeof conf.enabled === "boolean" && Number.isSafeInteger(conf.max_depth) &&
      Number(conf.max_depth) >= 1 && Number(conf.max_depth) <= 32 &&
      (conf.mode === "loss" || conf.mode === "turnover") &&
      (conf.cycle === "weekly" || conf.cycle === "monthly");
  }
  if (typeof conf.status !== "string" || !["active", "disabled"].includes(conf.status) ||
      typeof conf.can_create_children !== "boolean") return false;
  if (operation === "create") {
    return Number.isSafeInteger(body.policy_version) && Number(body.policy_version) >= 1 &&
      validUuid(body.member_id) && (body.parent_id === null || validUuid(body.parent_id)) &&
      (body.parent_version === null || (Number.isSafeInteger(body.parent_version) && Number(body.parent_version) >= 1)) &&
      (body.parent_id === null ? body.parent_version === null : body.parent_version !== null);
  }
  return Number.isSafeInteger(body.version) && Number(body.version) >= 1 &&
    Number.isSafeInteger(body.policy_version) && Number(body.policy_version) >= 1 &&
    (body.parent_version === null || (Number.isSafeInteger(body.parent_version) && Number(body.parent_version) >= 1));
}

function validIntent(value: unknown, scope: AgentWriteScope): value is PendingAgentWrite {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const candidate = value as Record<string, unknown>;
  const operation = candidate.operation;
  if (candidate.accountId !== scope.accountId || candidate.brandId !== scope.brandId ||
      !validUuid(scope.accountId) || !validUuid(scope.brandId) ||
      (operation !== "policy" && operation !== "create" && operation !== "update") ||
      (operation === "update" ? !validUuid(candidate.resourceId) : candidate.resourceId !== null) ||
      typeof candidate.key !== "string" || candidate.key.length === 0 ||
      !validBody(operation, candidate.body)) return false;
  return true;
}

/** Makes a detached immutable JSON snapshot; JSON serialization safely unwraps Vue proxies. */
export function freezeAgentBody<T extends object>(body: T): Readonly<T> {
  let copy: T;
  try {
    copy = JSON.parse(JSON.stringify(body)) as T;
  } catch {
    throw new TypeError("Agent write body must be JSON serializable");
  }
  const freeze = (value: unknown): void => {
    if (!value || typeof value !== "object" || Object.isFrozen(value)) return;
    Object.values(value).forEach(freeze);
    Object.freeze(value);
  };
  freeze(copy);
  return copy;
}

function copyIntent(intent: PendingAgentWrite): PendingAgentWrite {
  return Object.freeze({
    accountId: intent.accountId,
    brandId: intent.brandId,
    operation: intent.operation,
    resourceId: intent.resourceId,
    body: freezeAgentBody(intent.body),
    key: intent.key,
  });
}

/** Pending writes live in this page module only; they are intentionally not persisted. */
export function getPendingAgentWrite(scope: string | AgentWriteScope): PendingAgentWrite | null {
  const resolved = resolveScope(scope);
  if (!resolved || !validUuid(resolved.accountId) || !validUuid(resolved.brandId)) return null;
  const intent = pendingWrites.get(resolved.key);
  return intent && validIntent(intent, resolved) ? intent : null;
}

export function setPendingAgentWrite(scope: string | AgentWriteScope, intent: PendingAgentWrite): void {
  const resolved = resolveScope(scope);
  if (!resolved || !validIntent(intent, resolved)) return;
  pendingWrites.set(resolved.key, copyIntent(intent));
}

export function clearPendingAgentWrite(scope: string | AgentWriteScope, expectedKey?: string): void {
  const resolved = resolveScope(scope);
  if (resolved && (expectedKey === undefined || pendingWrites.get(resolved.key)?.key === expectedKey))
    pendingWrites.delete(resolved.key);
}

export function clearAllPendingAgentWrites(): void {
  pendingWrites.clear();
}

export type AgentFailure = "uncertain" | "definitive";

export function classifyAgentFailure(status: number | undefined): AgentFailure {
  if (status === undefined || status === 0 || status >= 500) return "uncertain";
  return "definitive";
}
