export type CommissionWriteFailure = "uncertain" | "conflict" | "definitive";

export function classifyCommissionWriteFailure(status: number): CommissionWriteFailure {
  if (status === 409) return "conflict";
  if (status === 0 || status >= 500) return "uncertain";
  return "definitive";
}

export function freezeCommissionPolicyBody<T extends object>(body: T): Readonly<T> {
  const freeze = (value: unknown): unknown => {
    if (value && typeof value === "object" && !Object.isFrozen(value)) {
      Object.values(value).forEach(freeze);
      Object.freeze(value);
    }
    return value;
  };
  return freeze(structuredClone(body)) as Readonly<T>;
}

export function commissionPolicyBodyFingerprint(body: unknown): string {
  const normalize = (value: unknown): unknown =>
    Array.isArray(value)
      ? value.map(normalize)
      : value && typeof value === "object"
        ? Object.fromEntries(Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, item]) => [key, normalize(item)]))
        : value;
  return JSON.stringify(normalize(body));
}

export function commissionPolicyContextKey(actorId: string, brandId: string): string {
  return JSON.stringify([actorId, brandId]);
}

export function commissionPolicyScopeMatches(input: {
  ticket: number;
  currentTicket: number;
  actorId: string;
  currentActorId: string;
  brandId: string;
  currentBrandId: string;
  canView: boolean;
  live: boolean;
}): boolean {
  return input.live && input.ticket === input.currentTicket && input.actorId === input.currentActorId &&
    input.brandId === input.currentBrandId && input.canView;
}

const pending = new Map<string, unknown>();

/** Page-session only; uncertain request intent is never written to browser storage. */
export function getPendingCommissionWrite<T>(key: string): T | null {
  return (pending.get(key) as T | undefined) ?? null;
}

export function setPendingCommissionWrite<T>(key: string, value: T): void {
  pending.set(key, value);
}

export function clearPendingCommissionWrite(key: string, expectedIntentKey?: string): boolean {
  if (expectedIntentKey !== undefined) {
    const current = pending.get(key) as { key?: unknown } | undefined;
    if (current?.key !== expectedIntentKey) return false;
  }
  pending.delete(key);
  return true;
}

export function findPendingCommissionWrite<T>(matches: (value: T) => boolean): T | null {
  for (const value of pending.values()) if (matches(value as T)) return value as T;
  return null;
}

export function clearAllPendingCommissionWrites(): void {
  pending.clear();
}
