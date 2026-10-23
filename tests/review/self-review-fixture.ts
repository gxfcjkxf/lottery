import { test as base, expect } from './platform-fixture';
import type { APIRequestContext } from '@playwright/test';

type SessionState = Awaited<ReturnType<APIRequestContext['storageState']>>;
type OwnedRuleBrand = { id: string; code: string; storageState: SessionState };

export const test = base.extend<{ selfReviewOrigins: { platform: string; admin: string }; selfReviewBrand: OwnedRuleBrand }>({
  selfReviewOrigins: [{ platform: 'http://127.0.0.1:5185', admin: 'http://127.0.0.1:5184' }, { option: true }],
  selfReviewBrand: async ({ playwright, platformSession, selfReviewOrigins }, use) => {
    const platformOrigin = selfReviewOrigins.platform;
    const adminOrigin = selfReviewOrigins.admin;
    const platform = await playwright.request.newContext({ storageState: platformSession });
    const staff = await playwright.request.newContext();
    const suffix = crypto.randomUUID().replaceAll('-', '').slice(0, 10);
    const code = `self_review_${suffix}`;
    let brandId: string | undefined;
    async function post(path: string, body: unknown, brand?: string) {
      const response = await platform.post(`${platformOrigin}/api/v1/platform${path}`, {
        headers: { Origin: platformOrigin, 'Idempotency-Key': crypto.randomUUID(), ...(brand ? { 'X-Brand-ID': brand } : {}) }, data: body,
      });
      expect(response.status(), await response.text()).toBe(201);
      return (await response.json()).data;
    }
    try {
      const brand = await post('/brands', { code, name: `Self-review test ${suffix}`, default_locale: 'en', timezone: 'UTC', reason: '' });
      brandId = brand.id;
      const role = await post('/roles', {
        code: `self_review_${suffix}`, name: 'Self-review test operator', status: 'active',
        permissions: ['game.view.brand', 'game.write.brand', 'rule.view.brand', 'rule.write.brand', 'rule.validate.brand', 'rule.submit.brand', 'rule.review.brand'], reason: '',
      }, brandId);
      const username = `self_review_${suffix}`, password = `review-${suffix}-2026`;
      await post('/accounts', { username, password, role_ids: [role.id], reason: '' }, brandId);
      const login = await staff.post(`${adminOrigin}/api/v1/admin/auth/login`, {
        headers: { Origin: adminOrigin, 'Idempotency-Key': crypto.randomUUID() }, data: { identifier: username, password },
      });
      expect(login.status()).toBe(200);
      const me = await staff.get(`${adminOrigin}/api/v1/admin/me`);
      expect(me.status()).toBe(200);
      expect((await me.json()).data.account.brand_ids).toEqual([brandId]);
      await use({ id: brand.id, code, storageState: await staff.storageState() });
    } finally {
      try {
        if (brandId) {
          const response = await platform.delete(`${platformOrigin}/api/v1/platform/brands/${brandId}`, {
            headers: { Origin: platformOrigin, 'Idempotency-Key': crypto.randomUUID() },
          });
          expect(response.status(), await response.text()).toBe(200);
        }
      } finally { await staff.dispose(); await platform.dispose(); }
    }
  },
  storageState: async ({ selfReviewBrand }, use) => use(selfReviewBrand.storageState),
});
export { expect };
