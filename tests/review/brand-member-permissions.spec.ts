import { test, expect } from './platform-fixture';

const origin = 'http://127.0.0.1:5184';
const brand = '0199a000-0000-7000-8000-000000000001';
const permissions = ['user.write.brand', 'user.kick.brand', 'user.password_reset.brand'];

test('brand member actions reject synthetic missing and foreign mappings despite flat grants', async ({ page, context, operatorSession }, info) => {
  await context.addCookies(operatorSession.cookies);
  let scope: 'actual' | 'missing' | 'empty' | 'foreign' = 'actual';
  await page.route('**/api/v1/admin/me', async route => {
    const response = await route.fetch();
    expect(response.status()).toBe(200);
    const body = await response.json();
    expect(body.data.account.super_admin).toBe(false);
    if (scope !== 'actual') {
      body.data.account.permissions = permissions;
      if (scope === 'missing') delete body.data.account.permissions_by_brand;
      if (scope === 'empty') body.data.account.permissions_by_brand = {};
      if (scope === 'foreign') body.data.account.permissions_by_brand = { '0199a000-0000-7000-8000-000000000002': permissions };
    }
    await route.fulfill({ response, json: body });
  });
  for (const mode of ['actual', 'missing', 'empty', 'foreign'] as const) {
    scope = mode;
    await page.goto(origin);
    await expect(page.locator('.app-shell')).toBeVisible();
    await page.getByLabel(/^(选择真实后台品牌|Select live admin brand)$/).selectOption(brand);
    await page.getByRole('combobox', { name: /^(Language|语言)$/ }).selectOption('zh-CN');
    if (info.project.name === 'mobile360') await page.locator('.mobile-nav').getByRole('button', { name: /用户/ }).click();
    else await page.getByRole('button', { name: /用户和成员/ }).click();
    await expect(page.locator('.member-actions').first()).toContainText('编辑');
    for (const label of ['编辑', '踢出', '重置密码']) {
      const action = page.locator('.member-actions').first().getByRole('button', { name: label, exact: true });
      if (mode === 'actual') await expect(action).toBeEnabled();
      else await expect(action).toBeDisabled();
    }
  }
});
