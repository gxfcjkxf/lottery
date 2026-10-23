import { describe, expect, it, vi } from 'vitest';
import { createMemoryHistory, createRouter, type Router } from 'vue-router';
import { backUserPage, parentUserPage, showsPageBack } from './navigation';

describe('mobile back navigation', () => {
  it('keeps primary pages and authentication entry branding without back buttons', () => {
    for (const path of ['/', '/orders', '/wallet', '/account', '/login', '/register']) expect(showsPageBack(path)).toBe(false);
    for (const path of ['/invites', '/help', '/results', '/orders/123', '/games/123/bet']) expect(showsPageBack(path)).toBe(true);
  });
  const parents = {
    '/invites': '/account', '/help': '/account', '/notifications': '/account',
    '/wallet/buckets': '/wallet', '/wallet/ledger': '/wallet', '/recharge': '/wallet', '/withdraw': '/wallet',
    '/orders/123': '/orders', '/privacy': '/register', '/terms': '/register',
    '/games': '/', '/games/123': '/', '/games/123/bet': '/games/123',
    '/mark-six/123': '/', '/canada/123': '/', '/results': '/', '/bet/confirm': '/',
  };
  for (const [path, parent] of Object.entries(parents)) it(`direct ${path} entry returns to ${parent}`, () => {
    expect(parentUserPage(path)).toBe(parent);
  });
  it('rejects undefined parent relationships', () => expect(() => parentUserPage('/unknown')).toThrow());
  function router(previous: unknown) {
    return {
      options: { history: { state: { back: previous } } },
      resolve: vi.fn(() => ({ matched: [{ path: '/account' }] })),
      back: vi.fn(), replace: vi.fn(),
    };
  }
  it('uses real history for known internal pages and preserves their query parameters', () => {
    const r = router('/results?period_no=20261017');
    backUserPage(r as unknown as Router, '/help');
    expect(r.resolve).toHaveBeenCalledWith('/results?period_no=20261017');
    expect(r.back).toHaveBeenCalledOnce(); expect(r.replace).not.toHaveBeenCalled();
  });
  for (const previous of [null, undefined, 'https://outside.example/', '//outside.example/', '']) {
    it(`does not leave the app for predecessor ${String(previous)}`, () => {
      const r = router(previous); backUserPage(r as unknown as Router, '/invites');
      expect(r.back).not.toHaveBeenCalled(); expect(r.replace).toHaveBeenCalledWith('/account');
    });
  }
  it('rejects unknown history routes handled only by the catch-all redirect', async () => {
    const r = createRouter({ history: createMemoryHistory(), routes: [
      { path: '/account', component: {} }, { path: '/invites', component: {} },
      { path: '/:pathMatch(.*)*', redirect: '/account' },
    ] });
    await r.push('/invites'); r.options.history.state.back = '/unknown';
    const back = vi.spyOn(r, 'back'), replace = vi.spyOn(r, 'replace');
    backUserPage(r, '/invites');
    expect(back).not.toHaveBeenCalled(); expect(replace).toHaveBeenCalledWith('/account');
  });
});
