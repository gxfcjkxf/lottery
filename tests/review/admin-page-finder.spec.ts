import { test, expect } from './platform-fixture';

test.use({ storageState: async ({ operatorSession }, use) => { await use(operatorSession); } });

test('page finder navigates without filtering members and mobile keeps its existing menu', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('lottery.admin.locale', 'zh-CN'));
  await page.goto('http://127.0.0.1:5184');
  await expect(page.locator('.app-shell')).toBeVisible();
  await page.getByLabel('选择真实后台品牌', { exact: true }).selectOption('0199a000-0000-7000-8000-000000000001');
  const finder = page.getByLabel('查找管理页面', { exact: true });
  if (page.viewportSize()!.width < 700) {
    await expect(finder).toBeHidden();
    await page.getByRole('navigation', { name: '移动端主导航', exact: true }).getByRole('button', { name: /用户$/ }).click();
  } else {
    await finder.fill('用户和成员');
    await finder.press('Enter');
    await expect(finder).toHaveValue('');
  }
  await expect(page.locator('.breadcrumbs b')).toHaveText('用户和成员');
  const memberSearch = page.getByLabel('搜索成员', { exact: true });
  await expect(memberSearch).toHaveValue('');
  await memberSearch.fill('trial-member-filter');
  if (page.viewportSize()!.width >= 700) {
    await finder.fill('not-an-admin-page');
    await finder.press('Enter');
    await expect(page.locator('.breadcrumbs b')).toHaveText('用户和成员');
    await expect(memberSearch).toHaveValue('trial-member-filter');
    await finder.press('Escape');
    await expect(finder).toHaveValue('');
    await page.getByTestId('admin-language').selectOption('en');
    await page.getByLabel('Find an admin page', { exact: true }).fill('Dashboard');
    await page.getByLabel('Find an admin page', { exact: true }).press('Enter');
    await expect(page.locator('.breadcrumbs b')).toHaveText('Dashboard');
  }
});
