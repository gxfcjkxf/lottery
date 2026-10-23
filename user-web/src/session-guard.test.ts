import { describe, expect, it, vi } from 'vitest';
import { createMemoryHistory, createRouter } from 'vue-router';
import { createUserSessionGuard } from './session-guard';

function routerFor(check: () => Promise<unknown>) {
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/:pathMatch(.*)*', component: { render: () => null } },
  ] });
  router.beforeEach(createUserSessionGuard(check));
  return router;
}

describe('user business routes require the real session', () => {
  for (const path of ['/', '/games', '/configured/game-id', '/orders', '/wallet', '/account', '/notifications', '/help', '/rule-preview']) {
    it(`redirects anonymous ${path} to login`, async () => {
      const check = vi.fn().mockRejectedValue({ status: 401, code: 'AUTH_SESSION_REVOKED' });
      const router = routerFor(check);
      await router.push(path);
      expect(router.currentRoute.value.path).toBe('/login');
      expect(check).toHaveBeenCalledOnce();
    });
  }
  for (const path of ['/login', '/register', '/terms', '/privacy']) {
    it(`allows public ${path} without a session request`, async () => {
      const check = vi.fn();
      const router = routerFor(check);
      await router.push(path);
      expect(router.currentRoute.value.path).toBe(path);
      expect(check).not.toHaveBeenCalled();
    });
  }
  it('preserves joining attribution when redirecting an anonymous entry', async () => {
    const router = routerFor(vi.fn().mockRejectedValue({ status: 401 }));
    await router.push('/?referral=referral-code&agent=agent-code');
    expect(router.currentRoute.value.fullPath).toBe('/login?referral=referral-code&agent=agent-code');
  });
  it('allows an authenticated business page', async () => {
    const router = routerFor(vi.fn().mockResolvedValue({ user: { id: 'member' } }));
    await router.push('/wallet');
    expect(router.currentRoute.value.path).toBe('/wallet');
  });
  it('reports backend outages rather than classifying them as logged out', async () => {
    const outage = new Error('service unavailable');
    const router = routerFor(vi.fn().mockRejectedValue(outage));
    const onError = vi.fn(); router.onError(onError);
    await expect(router.push('/wallet')).rejects.toBe(outage);
    expect(onError).toHaveBeenCalledWith(outage, expect.anything(), expect.anything());
    expect(router.currentRoute.value.path).not.toBe('/login');
  });
});
