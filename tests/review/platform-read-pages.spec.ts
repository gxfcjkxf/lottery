import { test, expect } from '@playwright/test';

const origin = 'http://127.0.0.1:5185';
const brand = '0199a000-0000-7000-8000-000000000001';
const timestamp = '2026-10-09T10:00:00Z';

// Real platform login, synthetic read pages. This verifies navigation, not backend query correctness.
test('platform member and audit pages paginate and clear scope without write controls', async ({ page }) => {
  let fail = false;
  for (const resource of ['users', 'audit']) {
    await page.route(`**/api/v1/platform/${resource}?**`, async route => {
      const req = route.request();
      expect(req.method()).toBe('GET');
      expect(req.headers()['x-brand-id']).toBe(brand);
      const url = new URL(req.url());
      expect(url.searchParams.get('limit')).toBe('51');
      if (fail) {
        fail = false;
        await route.fulfill({ status: 503, json: { success: false, error: { code: 'TEST_READ_FAILED', message: 'Page read unavailable' } } });
        return;
      }
      const offset = Number(url.searchParams.get('offset'));
      const items = Array.from({ length: 51 }, (_, i) => resource === 'users' ? {
        id: `member-${i}`, global_user_id: `global-${i}`, username: `member_page_${i}`, phone: '', display_name: `Member ${i}`, notes: '', status: 'normal', joined_at: timestamp, brand_id: brand, tags: [],
      } : {
        id: `audit-${i}`, brand_id: brand, action: `audit_page_${i}`, actor_type: 'admin', actor_id: 'actor', resource_type: 'member', resource_id: `member-${i}`, reason: 'UI fixture', request_id: `request-${i}`, created_at: timestamp, ip_address: '', before_json: null, after_json: null,
      });
      await route.fulfill({ json: { success: true, data: { items: items.slice(offset, offset + 51) } } });
    });
  }
  await page.goto(origin);
  await page.getByLabel('Username', { exact: true }).fill('review_platform');
  await page.getByLabel('Password', { exact: true }).fill(process.env.TEST_REVIEW_ADMIN_PASSWORD!);
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await expect(page.locator('.app-frame')).toBeVisible();
  for (const [section, testId, marker] of [
    ['Brand members', 'platform-member-pages', 'member_page_50'],
    ['Audit log', 'platform-audit-pages', 'audit_page_50'],
  ]) {
    await page.locator('.nav-item').filter({ hasText: section }).click();
    await page.locator('.brand-picker select').selectOption(brand);
    const pages = page.getByTestId(testId);
    await expect(pages.getByRole('button', { name: 'Previous page' })).toBeDisabled();
    await expect(pages.getByRole('button', { name: 'Next page' })).toBeEnabled();
    await pages.getByRole('button', { name: 'Next page' }).click();
    await expect(page.locator('.content')).toContainText(marker);
    await expect(pages.getByRole('button', { name: 'Next page' })).toBeDisabled();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    await pages.getByRole('button', { name: 'Previous page' }).click();
    await expect(pages.getByRole('button', { name: 'Previous page' })).toBeDisabled();
    fail = true;
    await pages.getByRole('button', { name: 'Next page' }).click();
    await expect(page.locator('.global-message')).toContainText('Page read unavailable');
    await pages.getByRole('button', { name: 'Refresh' }).click();
    await expect(pages.getByRole('button', { name: 'Next page' })).toBeEnabled();
    await page.locator('.brand-picker select').selectOption('');
    await expect(pages).toHaveCount(0);
    await expect(page.locator('.content')).not.toContainText(marker);
  }
  await expect(page.getByRole('button', { name: /Edit|Reset password|Approve withdrawal/i })).toHaveCount(0);
});
