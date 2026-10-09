import { test, expect } from '@playwright/test';

const origin = process.env.TEST_PLATFORM_ADMIN_ORIGIN ?? 'http://127.0.0.1:5175';
const api = `${origin}/api/v1/platform`;

test('platform brand creation retains an uncertain committed request across navigation and replays once', async ({ page }, info) => {
  test.skip(!process.env.TEST_PLATFORM_ADMIN_USERNAME || !process.env.TEST_PLATFORM_ADMIN_PASSWORD, 'Provide isolated platform administrator credentials');
  test.setTimeout(60_000);
  page.setDefaultTimeout(10_000);
  const login = await page.request.post(`${api}/auth/login`, {
    headers: { Origin: origin, 'Idempotency-Key': crypto.randomUUID() },
    data: { identifier: process.env.TEST_PLATFORM_ADMIN_USERNAME!.replace('{project}', info.project.name), password: process.env.TEST_PLATFORM_ADMIN_PASSWORD },
  });
  expect(login.status()).toBe(200);
  const me = await page.request.get(`${api}/me`);
  expect(me.status()).toBe(200);
  const account = (await me.json()).data.account;
  expect(account.super_admin).toBe(true);
  expect(account.platform_permissions).toContain('brand.create.platform');
  await page.goto(origin);
  await expect(page.locator('.app-frame')).toBeVisible();
  await page.getByRole('button', { name: /New brand/, exact: false }).click();
  const dialog = page.getByRole('dialog');
  const code = `brand_${crypto.randomUUID().replaceAll('-', '').slice(0, 12)}`;
  await dialog.getByLabel('Brand code', { exact: true }).fill(code);
  await dialog.getByLabel('Brand name', { exact: true }).fill('Independent platform creation test');
  await dialog.getByRole('combobox').selectOption('zh-CN');
  await dialog.getByLabel('Timezone', { exact: true }).fill('Asia/Manila');
  await dialog.getByLabel('Reason for creation', { exact: true }).fill('Verify real audited platform creation');
  await dialog.getByRole('button', { name: /Review request/ }).click();
  const attempts: Array<{ body: string | null; key: string | undefined }> = [];
  let receipt: { id: string; code: string; status: string; version: number; audit_log_id: string } | undefined;
  await page.route('**/api/v1/platform/brands', async route => {
    if (route.request().method() !== 'POST') { await route.continue(); return; }
    attempts.push({ body: route.request().postData(), key: route.request().headers()['idempotency-key'] });
    expect(route.request().headers()['x-brand-id']).toBeUndefined();
    if (attempts.length === 1) {
      const committed = await route.fetch();
      expect(committed.status()).toBe(201);
      receipt = (await committed.json()).data;
      await route.abort('failed');
    } else await route.continue();
  });
  await dialog.getByRole('button', { name: 'Confirm and create', exact: true }).click();
  await expect.poll(() => Boolean(receipt)).toBe(true);
  await expect(dialog.getByRole('button', { name: 'Retry same request', exact: true })).toBeVisible();
  await dialog.getByRole('button', { name: 'Close', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await page.locator('.nav-item').filter({ hasText: 'Brand members' }).click();
  await page.locator('.nav-item').filter({ hasText: 'Brands' }).click();
  await page.getByRole('button', { name: /New brand/ }).click();
  await expect(dialog).toContainText(code);
  await dialog.getByRole('button', { name: 'Retry same request', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.locator('.message.success')).toContainText('Brand created in paused status.');
  expect(attempts).toHaveLength(2);
  expect(attempts[1]).toEqual(attempts[0]);
  expect(receipt).toMatchObject({ code, status: 'paused', version: 1 });
  expect(receipt!.audit_log_id).toMatch(/^[0-9a-f-]{36}$/);
  const brands = await page.request.get(`${api}/brands`);
  expect(brands.status()).toBe(200);
  expect((await brands.json()).data.items.filter((b: { code: string }) => b.code === code)).toHaveLength(1);
  const current = await page.request.get(`${api}/brand-operation`, { headers: { 'X-Brand-ID': receipt!.id } });
  expect(current.status()).toBe(200);
  expect((await current.json()).data).toMatchObject({ brand_id: receipt!.id, status: 'paused', version: 1 });
  expect((await page.request.get(`${api}/me`).then(r => r.json())).data.account.brand_ids).not.toContain(receipt!.id);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: info.outputPath('platform-brand-created.png'), fullPage: true });
});
