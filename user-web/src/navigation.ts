import type { Router } from 'vue-router';

const primaryPages = new Set(['/', '/orders', '/wallet', '/account']);
const authenticationPages = new Set(['/login', '/register']);

export function showsPageBack(path: string): boolean {
  return !primaryPages.has(path) && !authenticationPages.has(path);
}

/** Explicit parent destinations for direct entries without an in-app predecessor. */
export function parentUserPage(path: string): string {
  if (['/invites', '/help', '/notifications'].includes(path)) return '/account';
  if (['/wallet/buckets', '/wallet/ledger', '/recharge', '/withdraw'].includes(path)) return '/wallet';
  if (/^\/orders\/[^/]+$/.test(path)) return '/orders';
  if (['/privacy', '/terms'].includes(path)) return '/register';
  if (/^\/games\/[^/]+\/bet$/.test(path)) return path.slice(0, -4);
  if (['/games', '/results', '/bet/confirm'].includes(path)
    || /^\/(games|mark-six|canada|configured)\/[^/]+$/.test(path)) return '/';
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
