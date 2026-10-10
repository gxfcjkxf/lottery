import { test, expect } from './platform-fixture';

const origin = 'http://127.0.0.1:5185';

test('brand directory filters genuine brands without changing records', async ({ page }) => {
  const response = await page.request.get(`${origin}/api/v1/platform/brands`);
  expect(response.status()).toBe(200);
  const before = (await response.json()).data;
  const brands = before.items as Array<{ id: string; code: string; status: string }>;
  expect(brands.length).toBeGreaterThan(0);
  await page.goto(origin);
  const panel = page.getByTestId('platform-brand-list');
  const rows = panel.locator('tbody tr.clickable-row');
  await expect(rows).toHaveCount(brands.length);
  const writes: string[] = [];
  page.on('request', request => { if (request.url().includes('/api/v1/platform/') && request.method() !== 'GET') writes.push(request.url()); });
  const query = page.getByLabel('Find a brand', { exact: true });
  await query.fill(` ${brands[0].id.toUpperCase()} `);
  await expect(rows).toHaveCount(1);
  await expect(rows).toContainText(brands[0].code);
  await query.fill('not-an-existing-brand-for-trial');
  await expect(page.getByText('No brands match these filters.', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Clear filters', exact: true }).click();
  await expect(rows).toHaveCount(brands.length);
  await page.getByRole('combobox', { name: 'Brand status', exact: true }).selectOption('active');
  await expect(rows).toHaveCount(brands.filter(brand => brand.status === 'active').length);
  await page.locator('.language').click();
  await expect(page.getByRole('combobox', { name: '品牌状态', exact: true })).toHaveValue('active');
  await page.getByRole('button', { name: '清除筛选', exact: true }).click();
  await expect(rows).toHaveCount(brands.length);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  expect((await (await page.request.get(`${origin}/api/v1/platform/brands`)).json()).data).toEqual(before);
  expect(writes).toEqual([]);
});
