import { test, expect } from './platform-fixture';

test.use({ storageState: async ({ operatorSession }, use) => { await use(operatorSession); } });

test('page finder navigates without filtering members and mobile keeps its existing menu', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('lottery.admin.locale', 'zh-CN'));
  // Prepare one real synthetic member; a fresh CI database can be empty.
  await page.goto('http://127.0.0.1:5183/register');
  await page.getByLabel('Choose a username or phone', { exact: true }).fill(`finder_${crypto.randomUUID().replaceAll('-', '').slice(0, 12)}`);
  await page.getByLabel('Password', { exact: true }).fill('owned-page-finder-password-2026');
  await page.locator('.auth-form input[type="checkbox"]').nth(0).check();
  await page.locator('.auth-form input[type="checkbox"]').nth(1).check();
  const registered = page.waitForResponse(response => response.url() === 'http://127.0.0.1:5183/api/v1/auth/register');
  await page.getByRole('button', { name: /Continue/ }).click();
  expect((await registered).status()).toBe(201);
  await expect(page).toHaveURL(/\/account$/);
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
  await expect(page.getByText('仅筛选当前已加载页面的成员；可翻页查看其他成员。', { exact: true })).toBeVisible();
  const rows = page.locator('.member-directory-table tbody tr');
  await expect(rows.first()).toBeVisible();
  const count = await rows.count();
  await memberSearch.fill('trial-member-filter');
  await expect(page.getByText('本页没有匹配的成员，请修改或清除筛选。', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '清除筛选', exact: true }).click();
  await expect(memberSearch).toHaveValue('');
  await expect(rows).toHaveCount(count);
  await memberSearch.fill('   ');
  await expect(rows).toHaveCount(count);
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
