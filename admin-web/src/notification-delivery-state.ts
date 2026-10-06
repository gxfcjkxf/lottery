import type { NotificationDeliveryRetryBody } from "./notification-delivery-api";

export interface PendingDeliveryRetry {
  readonly accountId: string;
  readonly brandId: string;
  readonly eventId: string;
  readonly body: Readonly<NotificationDeliveryRetryBody>;
  readonly key: string;
}

export interface DeliveryRetryScope {
  accountId: string;
  brandId: string;
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const pendingRetries = new Map<string, PendingDeliveryRetry>();

export function scopeKey(accountId: string, brandId: string): string {
  return JSON.stringify([accountId, brandId]);
}

function resolveScope(scope: string | DeliveryRetryScope): { key: string; accountId: string; brandId: string } | null {
  if (typeof scope !== "string") {
    return { key: scopeKey(scope.accountId, scope.brandId), ...scope };
  }
  try {
    const parsed: unknown = JSON.parse(scope);
    if (Array.isArray(parsed) && parsed.length === 2 && parsed.every((part) => typeof part === "string")) {
      return { key: scope, accountId: parsed[0], brandId: parsed[1] };
    }
  } catch {
    // Unknown scope strings cannot address a pending operation.
  }
  return null;
}

function validIntent(value: unknown, scope: { accountId: string; brandId: string }): value is PendingDeliveryRetry {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const candidate = value as Record<string, unknown>;
  const body = candidate.body;
  if (!body || typeof body !== "object" || Array.isArray(body)) return false;
  const request = body as Record<string, unknown>;
  return candidate.accountId === scope.accountId && candidate.brandId === scope.brandId &&
    UUID_RE.test(scope.accountId) && UUID_RE.test(scope.brandId) &&
    typeof candidate.eventId === "string" && UUID_RE.test(candidate.eventId) &&
    Number.isSafeInteger(request.attempt_count) && Number(request.attempt_count) >= 1 &&
    typeof request.reason === "string" && request.reason.trim().length > 0 &&
    new TextEncoder().encode(request.reason).length <= 500 &&
    typeof candidate.key === "string" && candidate.key.length > 0;
}

function copyBody(body: NotificationDeliveryRetryBody): Readonly<NotificationDeliveryRetryBody> {
  // Read known JSON fields from Vue proxies to create a detached immutable snapshot.
  return Object.freeze({ attempt_count: body.attempt_count, reason: body.reason });
}

function copyIntent(intent: PendingDeliveryRetry): PendingDeliveryRetry {
  return Object.freeze({
    accountId: intent.accountId,
    brandId: intent.brandId,
    eventId: intent.eventId,
    body: copyBody(intent.body),
    key: intent.key,
  });
}

export function freezeDeliveryRetryBody(body: NotificationDeliveryRetryBody): Readonly<NotificationDeliveryRetryBody> {
  return copyBody(body);
}

export function getPendingDeliveryRetry<T extends PendingDeliveryRetry = PendingDeliveryRetry>(
  scope: string | DeliveryRetryScope,
): T | null {
  const resolved = resolveScope(scope);
  if (!resolved || !UUID_RE.test(resolved.accountId) || !UUID_RE.test(resolved.brandId)) return null;
  const intent = pendingRetries.get(resolved.key);
  return intent && validIntent(intent, resolved) ? intent as T : null;
}

export function setPendingDeliveryRetry(scope: string | DeliveryRetryScope, intent: PendingDeliveryRetry): void {
  const resolved = resolveScope(scope);
  if (!resolved || !validIntent(intent, resolved)) return;
  pendingRetries.set(resolved.key, copyIntent(intent));
}

export function clearPendingDeliveryRetry(scope: string | DeliveryRetryScope, expectedKey?: string): void {
  const resolved = resolveScope(scope);
  if (resolved && (expectedKey === undefined || pendingRetries.get(resolved.key)?.key === expectedKey)) pendingRetries.delete(resolved.key);
}

/** Clear all page-session retry intents at logout or session invalidation. */
export function clearAllPendingDeliveryRetries(): void {
  pendingRetries.clear();
}

export type DeliveryRetryFailure = "uncertain" | "definitive";

export function classifyDeliveryRetryFailure(status: number | undefined): DeliveryRetryFailure {
  if (status === undefined || status === 0 || status >= 500) return "uncertain";
  if (status >= 400 && status < 500) return "definitive";
  return "uncertain";
}
