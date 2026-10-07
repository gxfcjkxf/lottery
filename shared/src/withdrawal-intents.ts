export interface ScopedWithdrawalIntent {
  scope: string;
}

/** Keep a frozen write only while the exact account, brand, and grant scope remains active. */
export function retainWithdrawalIntent<T extends ScopedWithdrawalIntent>(
  intent: T | null,
  scope: string,
): T | null {
  return intent?.scope === scope ? intent : null;
}

/** A pending write blocks every new financial intent until it is replayed or acknowledged. */
export function hasPendingWithdrawalIntent(intent: ScopedWithdrawalIntent | null): boolean {
  return intent !== null;
}
