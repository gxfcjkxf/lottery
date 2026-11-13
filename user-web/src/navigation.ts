import type { Router } from 'vue-router';

const primaryPages = new Set(['/', '/orders', '/wallet', '/account']);
const authenticationPages = new Set(['/login', '/register']);

export function showsPageBack(path: string): boolean {
  return !primaryPages.has(path) && !authenticationPages.has(path);
}

/** Explicit parent destinations for direct entries without an in-app predecessor. */
export function parentUserPage(path: string): string {
  if (['/account/payment-accounts', '/invites', '/contacts', '/help', '/notifications', '/announcements'].includes(path)) return '/account';
  if (['/wallet/buckets', '/wallet/ledger', '/recharge', '/withdraw'].includes(path)) return '/wallet';
  if (['/privacy', '/terms'].includes(path)) return '/register';
  if (['/results'].includes(path)
    || /^\/(configured)\/[^/]+$/.test(path)) return '/';
  throw new Error(`No parent navigation defined for ${path}`);
}

export function backUserPage(router: Router, path: string): void {
  const previous = router.options.history.state.back;
  if (typeof previous === 'string' && previous.startsWith('/') && !previous.startsWith('//')) {
    const matched = router.resolve(previous).matched;
    if (matched.length > 0 && matched.every(record => !record.redirect)) {
      router.back();
      return;
    }
  }
  void router.replace(parentUserPage(path));
}
