import { test, expect } from '@playwright/test';

const origin = 'http://127.0.0.1:5175';
const brand = '123e4567-e89b-12d3-a456-426614174001';
const roleId = '123e4567-e89b-12d3-a456-426614174002';
const display = { timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss' };

for (const locale of ['en', 'zh-CN'] as const) {
  const zh = locale === 'zh-CN';
  test(`brand account creation reuses another page's brand and reports blocked actions (${locale})`, async ({ page }, info) => {
    const errors: string[] = [];
    page.on('pageerror', error => errors.push(error.message));
    let rolesFail = false, emptyRoles = false;
    const roleBrands: string[] = [];
    await page.context().addCookies([{ name: 'lottery-locale', value: locale, url: origin }]);
    await page.route('**/api/**', async route => {
      const request = route.request();
      const path = new URL(request.url()).pathname;
      let data: unknown;
      if (path === '/api/v1/display-settings') data = display;
      else if (path.endsWith('/platform-settings')) data = { ...display, version: 1, display_name: 'Test platform', logo_text: 'T', logo_url: '', updated_at: '2026-10-18T00:00:00Z' };
      else if (path.endsWith('/me')) data = { account: { id: 'operator', username: 'operator', super_admin: true, platform_permissions: ['admin.write.platform', 'role.write.platform'] } };
      else if (path.endsWith('/brands')) data = { items: [{ id: brand, name: 'Aurora', code: 'aurora', status: 'active' }] };
      else if (path.endsWith('/roles')) {
        roleBrands.push(request.headers()['x-brand-id']);
        if (rolesFail) {
          await route.fulfill({ status: 403, json: { success: false, error: { code: 'PERMISSION_DENIED', message: 'Permission denied' } } });
          return;
        }
        data = { items: emptyRoles ? [] : [{ id: roleId, brand_id: brand, name: 'Operations', code: 'operations', status: 'active', version: 1, is_bootstrap: false, permissions: ['user.view.brand'] }] };
      } else if (path.endsWith('/accounts') || path.endsWith('/users')) data = { items: [] };
      else throw new Error(`Unexpected request: ${path}`);
      await route.fulfill({ json: { success: true, data } });
    });
    await page.goto(origin);
    // Select the brand through the members page, then reuse it in brand accounts.
    await page.getByRole('button', { name: zh ? '查看会员' : 'View members', exact: true }).click();
    if (await page.evaluate(() => matchMedia('(max-width: 640px)').matches)) {
      await page.getByRole('button', { name: zh ? '菜单' : 'Menu', exact: true }).click();
    }
    if (await page.locator('button[aria-controls="platform-brand-tabs"]').getAttribute('aria-expanded') === 'false') await page.locator('button[aria-controls="platform-brand-tabs"]').click()
    await page.locator('.nav-subitem').filter({ hasText: zh ? '后台账号' : 'Back-office accounts' }).click();
    await expect(page.locator('.brand-picker select')).toHaveValue(brand);
    const create = page.getByRole('button', { name: zh ? '新建账号' : 'New account', exact: true });
    await expect(page.getByTestId('platform-access-list')).toContainText(zh ? '暂无账号' : 'No accounts found');
    await expect(create).toBeEnabled();
    await create.click();
    const dialog = page.getByRole('dialog', { name: zh ? '新建账号' : 'New account', exact: true });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole('checkbox')).toHaveCount(1);
    expect(roleBrands).toEqual([brand]);
    await page.screenshot({ path: info.outputPath('new-brand-account.png'), fullPage: true });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    await dialog.getByRole('button', { name: zh ? '取消' : 'Cancel', exact: true }).click();

    rolesFail = true;
    await create.click();
    await expect(page.locator('.toast-error')).toContainText(zh ? '权限' : 'permission', { ignoreCase: true });
    await page.locator('.toast-error').getByRole('button').click();
    await dialog.getByRole('button', { name: zh ? '取消' : 'Cancel', exact: true }).click();
    rolesFail = false; emptyRoles = true;
    await create.click();
    await expect(page.locator('.toast-error')).toContainText(zh ? '请先到角色页新建或启用角色' : 'Create or enable a role');
    await page.locator('.toast-error').getByRole('button').click();
    await dialog.getByRole('button', { name: zh ? '取消' : 'Cancel', exact: true }).click();

    await page.locator('.brand-picker select').selectOption('');
    await expect(page.getByTestId('platform-access')).toHaveCount(0);
    await expect(create).toHaveCount(0);
    expect(errors).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  });
}
