import { test, expect } from './platform-fixture';

const origin = 'http://127.0.0.1:5185';
const brand = '0199a000-0000-7000-8000-000000000001';
// Fewer than 16 characters but at least 16 UTF-8 bytes, as required by the server.
const password = '本地账号测试密码';

test('platform manages both account types with explicit roles and original-request recovery', async ({ page, playwright }) => {
  page.setDefaultTimeout(10_000);
  const suffix = crypto.randomUUID().replaceAll('-', '').slice(0, 10);
  await page.goto(origin);
  await page.getByRole('button', { name: 'Accounts and permissions', exact: true }).click();
  const panel = page.getByTestId('platform-access');
  const scopes = page.getByTestId('platform-account-scopes');
  await expect(panel.getByRole('tab', { name: 'Accounts', exact: true })).toBeVisible();
  await panel.getByRole('button', { name: 'New account', exact: true }).click();
  let dialog = page.getByRole('dialog');
  await dialog.getByLabel('Username', { exact: true }).fill(`platform_${suffix}`);
  await dialog.getByLabel('Password (16–128 bytes)', { exact: true }).fill(password);
  await expect(dialog.locator('.access-options input[type=checkbox]').first()).toBeVisible();
  await dialog.locator('.access-options input[type=checkbox]').first().check();
  await dialog.getByLabel('Reason', { exact: true }).fill('Create owned phase-one platform account');
  const attempts: Array<{ body: string | null; key?: string }> = [];
  let created: { id: string; username: string; super_admin: boolean; brand_ids: string[] };
  await page.route('**/api/v1/platform/platform-accounts', async route => {
    if (route.request().method() !== 'POST') { await route.continue(); return; }
    expect(route.request().headers()['x-brand-id']).toBeUndefined();
    attempts.push({ body: route.request().postData(), key: route.request().headers()['idempotency-key'] });
    if (attempts.length === 1) {
      const committed = await route.fetch();
      expect(committed.status()).toBe(201);
      created = (await committed.json()).data;
      await route.abort('failed');
    } else await route.continue();
  });
  await dialog.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(dialog.getByRole('button', { name: 'Retry original request', exact: true })).toBeVisible();
  await expect(scopes.getByRole('button', { name: 'Brand staff', exact: true })).toBeDisabled();
  await dialog.getByRole('button', { name: 'Retry original request', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(attempts).toHaveLength(2);
  expect(attempts[1]).toEqual(attempts[0]);
  expect(created!.super_admin).toBe(true);
  expect(created!.brand_ids).toEqual([]);

  async function editStatus(username: string, status: 'active' | 'disabled') {
    await panel.getByRole('button', { name: `List snapshot details: ${username}`, exact: true }).click();
    await panel.getByRole('button', { name: 'Edit status and roles', exact: true }).click();
    dialog = page.getByRole('dialog');
    await expect(dialog.locator('.access-options input[type=checkbox]').first()).toBeVisible();
    await dialog.getByRole('combobox', { name: 'Status', exact: true }).selectOption(status);
    await dialog.getByLabel('Reason', { exact: true }).fill(`Set owned account ${status}`);
    await dialog.getByRole('button', { name: 'Save', exact: true }).click();
    await expect(dialog).toHaveCount(0);
    await expect(panel.locator('tbody tr').filter({ hasText: username })).toContainText(status === 'active' ? 'Active' : 'Disabled');
  }
  await editStatus(created!.username, 'disabled');
  await editStatus(created!.username, 'active');
  await panel.getByRole('button', { name: `List snapshot details: ${created!.username}`, exact: true }).click();
  await panel.getByRole('button', { name: 'Reset password', exact: true }).click();
  dialog = page.getByRole('dialog');
  const newPassword = '更换账号测试密码';
  await dialog.getByLabel('Password (16–128 bytes)', { exact: true }).fill(newPassword);
  await dialog.getByLabel('Reason', { exact: true }).fill('Reset owned platform account password');
  await dialog.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  const platformLogin = await playwright.request.newContext();
  try {
    const result = await platformLogin.post(`${origin}/api/v1/platform/auth/login`, { headers: { Origin: origin, 'Idempotency-Key': crypto.randomUUID() }, data: { identifier: created!.username, password: newPassword } });
    expect(result.status()).toBe(200);
    expect((await (await platformLogin.get(`${origin}/api/v1/platform/me`)).json()).data.account.super_admin).toBe(true);
  } finally { await platformLogin.dispose(); }

  await scopes.getByRole('button', { name: 'Brand staff', exact: true }).click();
  await page.locator('.content > .brand-picker select').selectOption(brand);
  await panel.getByRole('tab', { name: 'Roles', exact: true }).click();
  await panel.getByRole('button', { name: 'New role', exact: true }).click();
  dialog = page.getByRole('dialog');
  const roleCode = `support_${suffix}`;
  await dialog.getByLabel('Code', { exact: true }).fill(roleCode);
  await dialog.getByLabel('Name', { exact: true }).fill(`Support ${suffix}`);
  await dialog.getByRole('checkbox', { name: 'user.view.brand', exact: true }).check();
  await dialog.getByLabel('Reason', { exact: true }).fill('Create explicit owned support role');
  await dialog.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(panel).toContainText(roleCode);
  await panel.locator('tbody tr').filter({ hasText: roleCode }).getByRole('button').click();
  await panel.getByRole('button', { name: 'Edit role', exact: true }).click();
  dialog = page.getByRole('dialog');
  await dialog.getByLabel('Name', { exact: true }).fill(`Updated support ${suffix}`);
  await dialog.getByLabel('Reason', { exact: true }).fill('Update owned support role label');
  await dialog.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(panel).toContainText(`Updated support ${suffix}`);
  await panel.getByRole('tab', { name: 'Accounts', exact: true }).click();
  await panel.getByRole('button', { name: 'New account', exact: true }).click();
  dialog = page.getByRole('dialog');
  const staffUsername = `staff_${suffix}`;
  await dialog.getByLabel('Username', { exact: true }).fill(staffUsername);
  await dialog.getByLabel('Password (16–128 bytes)', { exact: true }).fill(password);
  await dialog.getByRole('checkbox', { name: new RegExp(roleCode) }).check();
  await dialog.getByLabel('Reason', { exact: true }).fill('Create owned brand staff');
  await dialog.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await editStatus(staffUsername, 'disabled');
  await editStatus(staffUsername, 'active');
  await panel.getByRole('button', { name: `List snapshot details: ${staffUsername}`, exact: true }).click();
  await panel.getByRole('button', { name: 'Reset password', exact: true }).click();
  dialog = page.getByRole('dialog');
  await dialog.getByLabel('Password (16–128 bytes)', { exact: true }).fill(newPassword);
  await dialog.getByLabel('Reason', { exact: true }).fill('Reset owned brand staff password');
  await dialog.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  const staffLogin = await playwright.request.newContext();
  try {
    const staffOrigin = 'http://127.0.0.1:5184';
    const result = await staffLogin.post(`${staffOrigin}/api/v1/admin/auth/login`, { headers: { Origin: staffOrigin, 'X-Brand-ID': brand, 'Idempotency-Key': crypto.randomUUID() }, data: { identifier: staffUsername, password: newPassword } });
    expect(result.status()).toBe(200);
    const account = (await (await staffLogin.get(`${staffOrigin}/api/v1/admin/me`)).json()).data.account;
    expect(account.super_admin).toBe(false);
    expect(account.brand_ids).toEqual([brand]);
    expect(account.permissions_by_brand[brand]).toEqual(['user.view.brand']);
  } finally { await staffLogin.dispose(); }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
});
