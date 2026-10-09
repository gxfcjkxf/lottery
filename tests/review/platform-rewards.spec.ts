import { test, expect } from '@playwright/test';

const origin = 'http://127.0.0.1:5185';
const brand = '0199a000-0000-7000-8000-000000000001';
const otherBrand = '0199a000-0000-7000-8000-000000000002';
const uuid = (n: number) => `aaaaaaaa-aaaa-4aaa-8aaa-${n.toString(16).padStart(12, '0')}`;
const timestamp = '2026-10-09T10:00:00Z';
const orders = Array.from({ length: 101 }, (_, index) => ({
  id: uuid(index + 1), brand_id: brand, member_id: uuid(1000), points: '250', state: 'granted', version: 1,
  grant_ledger_entry_id: uuid(1001), revoke_ledger_entry_id: null, creation_audit_log_id: uuid(1002),
  last_audit_log_id: uuid(1002), last_error_code: null, created_by: uuid(1003), reason: 'Pagination UI fixture',
  point_policy_version: '1', created_at: timestamp, updated_at: timestamp, revoked_at: null,
}));

// Synthetic read responses exercise pagination UI; real reward finances have a separate workflow test.
test('platform rewards paginate orders and history, refresh and reset on brand change', async ({ page }, info) => {
  let failNext = false;
  const reads: string[] = [];
  await page.route('**/api/v1/platform/reward-orders**', async route => {
    const request = route.request();
    expect(request.method()).toBe('GET');
    const selectedBrand = request.headers()['x-brand-id'];
    const url = new URL(request.url());
    reads.push(url.pathname + url.search);
    const offset = Number(url.searchParams.get('offset') || 0);
    if (failNext) {
      failNext = false;
      await route.fulfill({ status: 503, json: { success: false, error: { code: 'TEST_READ_FAILED', message: 'Read failed for pagination test' } } });
      return;
    }
    let data: unknown;
    if (url.pathname.endsWith('/actions')) {
      const orderId = url.pathname.split('/').at(-2)!;
      const actions = Array.from({ length: 101 }, (_, i) => ({
        id: uuid(2000 + i), brand_id: brand, order_id: orderId, version: i + 1,
        operation: 'retry', state_before: 'revocation_pending', state_after: 'revocation_pending',
        actor_id: uuid(1003), reason: `History fixture ${i + 1}`, audit_log_id: uuid(3000 + i),
        ledger_entry_id: null, created_at: timestamp,
      }));
      data = { brand_id: brand, order_id: orderId, items: actions.slice(offset, offset + 100), total_count: '101', limit: 100, offset };
    } else if (url.pathname === '/api/v1/platform/reward-orders') {
      data = { brand_id: selectedBrand, items: selectedBrand === brand ? orders.slice(offset, offset + 100) : [], total_count: selectedBrand === brand ? '101' : '0', limit: 100, offset };
    } else {
      data = orders.find(order => order.id === url.pathname.split('/').at(-1));
    }
    await route.fulfill({ json: { success: true, data } });
  });
  await page.goto(origin);
  await page.getByLabel('Username', { exact: true }).fill('review_platform');
  await page.getByLabel('Password', { exact: true }).fill(process.env.TEST_REVIEW_ADMIN_PASSWORD!);
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await expect(page.locator('.app-frame')).toBeVisible();
  await page.locator('.nav-item').filter({ hasText: 'Manual rewards' }).click();
  await page.locator('.brand-picker select').selectOption(brand);
  const pages = page.getByTestId('platform-reward-pages');
  await expect(page.locator('.reward-order-link')).toHaveCount(100);
  await expect(pages.getByRole('button', { name: 'Previous page' })).toBeDisabled();
  await pages.getByRole('button', { name: 'Next page' }).click();
  await expect(page.locator('.reward-order-link')).toHaveCount(1);
  await expect(page.locator('.reward-order-link')).toHaveText(uuid(101));
  await expect(pages.getByRole('button', { name: 'Next page' })).toBeDisabled();
  await page.locator('.reward-order-link').click();
  const historyPages = page.getByTestId('platform-reward-history-pages');
  await expect(page.getByTestId('platform-reward-detail')).toContainText('History fixture 1');
  await historyPages.getByRole('button', { name: 'Next page' }).click();
  await expect(page.getByTestId('platform-reward-detail')).toContainText('History fixture 101');
  await expect(historyPages.getByRole('button', { name: 'Next page' })).toBeDisabled();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: info.outputPath('platform-reward-pagination.png'), fullPage: true });
  await historyPages.getByRole('button', { name: 'Previous page' }).click();
  await expect(page.getByTestId('platform-reward-detail')).toContainText('History fixture 1');
  await pages.getByRole('button', { name: 'Previous page' }).click();
  await expect(page.locator('.reward-order-link')).toHaveCount(100);
  await expect(page.getByTestId('platform-reward-detail')).toHaveCount(0);
  failNext = true;
  await pages.getByRole('button', { name: 'Next page' }).click();
  await expect(page.getByTestId('platform-rewards')).toContainText('Read failed for pagination test');
  await expect(page.locator('.reward-order-link')).toHaveCount(0);
  await page.getByTestId('platform-reward-refresh').click();
  await expect(page.locator('.reward-order-link')).toHaveCount(100);
  await page.locator('.brand-picker select').selectOption(otherBrand);
  await expect(page.locator('.reward-order-link')).toHaveCount(0);
  await expect(pages.getByRole('button', { name: 'Next page' })).toBeDisabled();
  expect(reads).toContain('/api/v1/platform/reward-orders?limit=100&offset=100');
  expect(reads).toContain(`/api/v1/platform/reward-orders/${uuid(101)}/actions?limit=100&offset=100`);
});
