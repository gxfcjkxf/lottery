/** Mutating review suites must never target the shared development database. */
export function assertReviewIsolation(env) {
  if (env.APP_ENV !== 'test' || typeof env.DATABASE_URL !== 'string') {
    throw new Error('Review tests require APP_ENV=test and an explicitly owned DATABASE_URL.');
  }
  let target;
  try { target = new URL(env.DATABASE_URL); }
  catch { throw new Error('Invalid review test database configuration.'); }
  if (!['postgres:', 'postgresql:'].includes(target.protocol)
    || !['127.0.0.1', 'localhost'].includes(target.hostname)
    || !/^\/lottery_admin_entries_(desktop1440|mobilepixel7|local_[a-z0-9_]+)$/.test(target.pathname)) {
    throw new Error('Review tests require an isolated lottery_admin_entries_* database; shared brands and development databases are forbidden.');
  }
}
