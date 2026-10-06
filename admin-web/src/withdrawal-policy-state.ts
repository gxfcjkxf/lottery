export type PolicyWriteFailure = "uncertain" | "conflict" | "definitive";

export function classifyPolicyWriteFailure(status: number): PolicyWriteFailure {
  if (status === 409) return "conflict";
  if (status === 0 || status >= 500) return "uncertain";
  return "definitive";
}

export function freezePolicyBody<T extends object>(body: T): Readonly<T> {
  const freeze = (value: unknown): unknown => {
    if (value && typeof value === "object" && !Object.isFrozen(value)) {
      Object.values(value).forEach(freeze);
      Object.freeze(value);
    }
    return value;
  };
  return freeze(structuredClone(body)) as Readonly<T>;
}

export function policyBodyFingerprint(body: unknown): string {
  const normalize = (value: unknown): unknown =>
    Array.isArray(value)
      ? value.map(normalize)
      : value && typeof value === "object"
        ? Object.fromEntries(
            Object.entries(value)
              .sort(([a], [b]) => a.localeCompare(b))
              .map(([key, item]) => [key, normalize(item)]),
          )
        : value;
  return JSON.stringify(normalize(body));
}

const pendingPolicyWrites = new Map<string, unknown>();

/** Page-session only: intentionally not persisted across reloads. */
export function getPendingPolicyWrite<T>(key: string): T | null {
  return (pendingPolicyWrites.get(key) as T | undefined) ?? null;
}

export function setPendingPolicyWrite<T>(key: string, value: T): void {
  pendingPolicyWrites.set(key, value);
}

export function clearPendingPolicyWrite(key: string): void {
  pendingPolicyWrites.delete(key);
}

export function findPendingPolicyWrite<T>(
  matches: (value: T) => boolean,
): T | null {
  for (const value of pendingPolicyWrites.values()) {
    if (matches(value as T)) return value as T;
  }
  return null;
}
