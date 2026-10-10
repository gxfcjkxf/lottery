import { test, expect } from './platform-fixture';

test('brand role creation waits for a successfully loaded permission catalog', async ({ page, context, operatorSession }, info) => {
  await context.addCookies(operatorSession.cookies);
  await page.addInitScript(() => localStorage.setItem('lottery.admin.locale', 'zh-CN'));
  let unavailable = true;
  let writes = 0;
  await page.route('**/api/v1/admin/permissions', async route => {
    if (unavailable) await route.fulfill({ status: 503, json: { success: false, error: { code: 'CATALOG_UNAVAILABLE', message: 'Permission catalog unavailable' } } });
    else await route.continue();
  });
  await page.route('**/api/v1/admin/roles', async route => {
    if (route.request().method() === 'POST') writes++;
    await route.continue();
  });
  await page.goto('http://127.0.0.1:5184');
  await page.getByLabel('选择真实后台品牌', { exact: true }).selectOption('0199a000-0000-7000-8000-000000000001');
  if (info.project.name === 'mobile360') {
    await page.locator('.mobile-nav button').last().click();
    await page.getByRole('navigation', { name: '全部管理页面' }).getByRole('button', { name: /账号与权限/ }).click();
  } else await page.locator('.side-nav').getByRole('button', { name: /账号与权限/ }).click();
  const panel = page.locator('.access-page');
  await expect(panel.getByRole('alert')).toContainText('Permission catalog unavailable');
  await panel.getByLabel(/角色代码/).fill('catalog_check');
  await panel.getByLabel(/显示名称/).fill('Catalog check');
  await panel.getByLabel('变更原因', { exact: true }).fill('Verify catalog is required before saving');
  const save = panel.getByRole('button', { name: '创建角色', exact: true });
  await expect(save).toBeDisabled();
  expect(writes).toBe(0);
  unavailable = false;
  await panel.getByRole('button', { name: '刷新', exact: true }).click();
  await expect(panel.getByLabel('user.view.brand', { exact: true })).toBeVisible();
  await expect(save).toBeEnabled();
  expect(writes).toBe(0);
  // A later failed read must also invalidate the previously loaded catalog.
  await panel.getByRole('tab', { name: '管理员账号', exact: true }).click();
  unavailable = true;
  await panel.getByRole('tab', { name: '角色权限', exact: true }).click();
  await expect(panel.getByRole('alert')).toContainText('Permission catalog unavailable');
  await expect(save).toBeDisabled();
  expect(writes).toBe(0);
  unavailable = false;
  await panel.getByRole('button', { name: '刷新', exact: true }).click();
  await expect(save).toBeEnabled();
  const code = `receipt_${crypto.randomUUID().replaceAll('-', '').slice(0, 10)}`;
  await panel.getByLabel(/角色代码/).fill(code);
  await panel.getByLabel('user.view.brand', { exact: true }).check();
  const attempts: Array<{ body: string | null; key: string }> = [];
  const receiptIds: string[] = [];
  await page.route('**/api/v1/admin/roles', async route => {
    if (route.request().method() !== 'POST') { await route.continue(); return; }
    writes++;
    attempts.push({ body: route.request().postData(), key: route.request().headers()['idempotency-key'] });
    const response = await route.fetch();
    expect(response.status()).toBe(201);
    receiptIds.push((await response.json()).data.id);
    if (attempts.length === 1) await route.fulfill({ status: 200, json: { success: true, data: null } });
    else await route.fulfill({ response });
  });
  await save.click();
  await expect(panel.getByRole('alert')).toContainText('operation result is unconfirmed');
  await expect(panel.getByLabel(/角色代码/)).toHaveValue(code);
  await save.click();
  await expect(panel.getByRole('status')).toContainText('角色已创建');
  expect(attempts).toHaveLength(2);
  expect(attempts[1]).toEqual(attempts[0]);
  expect(receiptIds[1]).toBe(receiptIds[0]);
  expect(writes).toBe(2);
});
