import { test as base, expect, type APIRequestContext } from '@playwright/test';

type SessionState = Awaited<ReturnType<APIRequestContext['storageState']>>;

export const test = base.extend<{}, { platformSession: SessionState; operatorSession: SessionState }>({
  operatorSession: [async ({ playwright }, use, info) => {
    const project = info.project.name;
    if (project !== 'desktop1440' && project !== 'mobile360') throw new Error(`Unsupported review viewport: ${project}`);
    const password = process.env.TEST_REVIEW_ADMIN_PASSWORD;
    if (!password) throw new Error('TEST_REVIEW_ADMIN_PASSWORD is required');
    const origin = 'http://127.0.0.1:5184';
    const api = await playwright.request.newContext();
    try {
      const login = await api.post(`${origin}/api/v1/admin/auth/login`, {
        headers: { Origin: origin, 'X-Brand-ID': '0199a000-0000-7000-8000-000000000001', 'Idempotency-Key': crypto.randomUUID() },
        data: { identifier: `review_operator_${project}`, password },
      });
      expect(login.status(), 'Owned operator session must authenticate without retries').toBe(200);
      const me = await api.get(`${origin}/api/v1/admin/me`);
      expect(me.status()).toBe(200);
      const account = (await me.json()).data.account;
      expect(account.super_admin).toBe(false);
      expect(account.brand_ids).toEqual(['0199a000-0000-7000-8000-000000000001']);
      await use(await api.storageState());
    } finally { await api.dispose(); }
  }, { scope: 'worker' }],
  platformSession: [async ({ playwright }, use, info) => {
    const project = info.project.name;
    if (project !== 'desktop1440' && project !== 'mobile360') throw new Error(`Unsupported review viewport: ${project}`);
    const password = process.env.TEST_REVIEW_ADMIN_PASSWORD;
    if (!password) throw new Error('TEST_REVIEW_ADMIN_PASSWORD is required');
    const origin = 'http://127.0.0.1:5185';
    const api = await playwright.request.newContext();
    try {
      const login = await api.post(`${origin}/api/v1/platform/auth/login`, {
        headers: { Origin: origin, 'Idempotency-Key': crypto.randomUUID() },
        data: { identifier: `review_platform_${project}`, password },
      });
      expect(login.status(), 'Owned platform session must authenticate without retries').toBe(200);
      const me = await api.get(`${origin}/api/v1/platform/me`);
      expect(me.status()).toBe(200);
      expect((await me.json()).data.account.super_admin).toBe(true);
      await use(await api.storageState());
    } finally { await api.dispose(); }
  }, { scope: 'worker' }],
  storageState: async ({ platformSession }, use) => { await use(platformSession); },
});
export { expect };
