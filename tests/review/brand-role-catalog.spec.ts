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
});
