import { test, expect } from './platform-fixture';

const origin = 'http://127.0.0.1:5185';
const brand = '0199a000-0000-7000-8000-000000000001';
const headers = { 'X-Brand-ID': brand };

test('expired access query removes private platform content and returns to login', async ({ page }) => {
  await page.route('**/api/v1/platform/accounts?**', route => route.fulfill({ status: 401, json: { success: false, error: { code: 'AUTH_SESSION_REVOKED', message: 'Session expired' } } }));
  await page.goto(origin);
  await page.getByRole('button', { name: 'Audit and operations', exact: true }).click();
  await page.locator('.content > .brand-picker select').selectOption(brand);
  await page.getByTestId('platform-operations-tabs').getByRole('button', { name: 'Access and permissions', exact: true }).click();
  await expect(page.locator('.login-card')).toBeVisible();
  await expect(page.locator('.app-frame')).toHaveCount(0);
  await expect(page.locator('.nav-item')).toHaveCount(0);
});

test('platform reads genuine scoped accounts roles and permission directory without writes', async ({ page }, info) => {
  const before = new Map<string, any>();
  for (const path of ['accounts?limit=51&offset=0', 'roles?limit=51&offset=0', 'permissions']) {
    const response = await page.request.get(`${origin}/api/v1/platform/${path}`, { headers });
    expect(response.status(), await response.text()).toBe(200);
    const data = (await response.json()).data;
    expect(data.items.length).toBeGreaterThan(0);
    before.set(path, data);
  }
  await page.goto(origin);
  await page.getByRole('button', { name: 'Audit and operations', exact: true }).click();
  await page.locator('.content > .brand-picker select').selectOption(brand);
  await page.getByTestId('platform-operations-tabs').getByRole('button', { name: 'Access and permissions', exact: true }).click();
  const panel = page.getByTestId('platform-access');
  const firstAccount = before.get('accounts?limit=51&offset=0').items[0];
  await expect(panel).toContainText(firstAccount.username);
  await page.getByTestId('platform-access-list').getByRole('button', { name: firstAccount.id, exact: true }).click();
  await expect(page.getByTestId('platform-access-detail')).toContainText(firstAccount.id);
  await page.getByTestId('platform-access-tabs').getByRole('tab', { name: 'Roles', exact: true }).click();
  const firstRole = before.get('roles?limit=51&offset=0').items[0];
  await page.getByTestId('platform-access-list').getByRole('button', { name: firstRole.id, exact: true }).click();
  await expect(page.getByTestId('platform-access-detail')).toContainText(firstRole.name);
  for (const permission of firstRole.permissions) await expect(page.getByTestId('platform-access-detail')).toContainText(permission);
  await page.getByTestId('platform-access-tabs').getByRole('tab', { name: 'Permission directory', exact: true }).click();
  await expect(page.getByTestId('platform-permission-directory')).toContainText(before.get('permissions').items[0]);
  await expect(panel.getByRole('alert')).toHaveCount(0);
  await page.locator('.language').click();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: info.outputPath('platform-access.png'), fullPage: true });
  await expect(panel.getByRole('button', { name: /Create|Save|Reset|创建|保存|重置/ })).toHaveCount(0);
  for (const path of ['/accounts', '/roles', `/accounts/${firstAccount.id}/reset-password`]) {
    const denied = await page.request.post(`${origin}/api/v1/platform${path}`, { headers: { ...headers, Origin: origin }, data: {} });
    expect([404, 405]).toContain(denied.status());
  }
  const patch = await page.request.patch(`${origin}/api/v1/platform/roles/${firstRole.id}`, { headers: { ...headers, Origin: origin }, data: {} });
  expect([404, 405]).toContain(patch.status());
  for (const [path, data] of before) {
    const response = await page.request.get(`${origin}/api/v1/platform/${path}`, { headers });
    expect(response.status()).toBe(200);
    expect((await response.json()).data).toEqual(data);
  }
  await page.locator('.content > .brand-picker select').selectOption('');
  await expect(page.getByTestId('platform-access-detail')).toHaveCount(0);
  await expect(page.getByTestId('platform-permission-directory')).not.toContainText(before.get('permissions').items[0]);
});

test('access lists paginate and clear list-snapshot details on errors and scope changes', async ({ page }) => {
  let fail = false;
  const roles = Array.from({ length: 51 }, (_, index) => ({ id: crypto.randomUUID(), brand_id: brand, code: `role_${index}`, name: `Role ${index}`, status: 'active', version: 1, is_bootstrap: false, permissions: ['user.view.brand'] }));
  await page.route('**/api/v1/platform/roles?**', async route => {
    const offset = Number(new URL(route.request().url()).searchParams.get('offset'));
    await route.fulfill(fail ? { status: 503, json: { success: false, error: { message: 'Roles unavailable' } } } : { json: { success: true, data: { items: roles.slice(offset, offset + 51) } } });
  });
  await page.goto(origin);
  await page.getByRole('button', { name: 'Audit and operations', exact: true }).click();
  await page.locator('.content > .brand-picker select').selectOption(brand);
  await page.getByTestId('platform-operations-tabs').getByRole('button', { name: 'Access and permissions', exact: true }).click();
  const panel = page.getByTestId('platform-access');
  await page.getByTestId('platform-access-tabs').getByRole('tab', { name: 'Roles', exact: true }).click();
  await expect(page.getByTestId('platform-access-list').locator('tbody tr')).toHaveCount(50);
  await page.getByTestId('platform-access-pages').getByRole('button', { name: /Next/ }).click();
  await expect(page.getByTestId('platform-access-list').locator('tbody tr')).toHaveCount(1);
  await page.getByTestId('platform-access-list').getByRole('button', { name: roles[50].id, exact: true }).click();
  await expect(page.getByTestId('platform-access-detail')).toContainText('Role 50');
  fail = true;
  await panel.getByRole('button', { name: 'Refresh', exact: true }).click();
  await expect(panel.getByRole('alert')).toContainText('Roles unavailable');
  await expect(page.getByTestId('platform-access-detail')).toHaveCount(0);
  await page.locator('.content > .brand-picker select').selectOption('');
  await expect(panel).not.toContainText('Role 50');
});
