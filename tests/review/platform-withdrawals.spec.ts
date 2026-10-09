import type { Page } from '@playwright/test';
import { test, expect } from './platform-fixture';

const origin = 'http://127.0.0.1:5185';
const brand = '0199a000-0000-7000-8000-000000000001';
const uuid = (n: number) => `aaaaaaaa-aaaa-4aaa-8aaa-${n.toString(16).padStart(12, '0')}`;
const member = uuid(1);
const at = '2026-10-09T10:00:00Z';
const points = '9007199254740993';
const orders = Array.from({ length: 51 }, (_, i) => ({
  id: uuid(100 + i), brand_id: brand, member_id: member, account_id: uuid(2), points, state: 'paid', version: 3,
  source_allocation: [{ source: 'recharge', state: 'available', points }], reserve_entry_id: uuid(3), release_entry_id: null, paid_entry_id: uuid(4),
  cycle_from_at: null, cycle_from_version: '0', reserve_version: '1', created_at: at, updated_at: at,
  reviewed_at: at, completed_at: at, decision_reason: 'Synthetic read-only UI receipt', audit_log_id: uuid(5),
}));
async function login(page: Page) {
  await page.goto(origin);
  await expect(page.locator('.app-frame')).toBeVisible();
  await page.locator('.nav-item').filter({ hasText: 'Withdrawals' }).click();
}

test('platform withdrawals retain exact sources, state history and filters without approval controls', async ({ page }, info) => {
  let fail = false;
  let matched = false;
  await page.route('**/api/v1/platform/withdrawals**', async route => {
    expect(route.request().method()).toBe('GET');
    expect(route.request().headers()['x-brand-id']).toBe(brand);
    if (fail) { fail = false; await route.fulfill({ status: 503, json: { success: false, error: { code: 'TEST_READ_FAILED', message: 'Withdrawal read unavailable' } } }); return; }
    const url = new URL(route.request().url());
    const offset = Number(url.searchParams.get('offset') || 0);
    if (url.searchParams.get('state') === 'paid' && url.searchParams.get('member_id') === member) matched = true;
    let data: unknown;
    if (url.pathname.endsWith('/history')) {
      const orderId = url.pathname.split('/').at(-2)!;
      data = { brand_id: brand, order_id: orderId, items: [
        { id: uuid(10), version: 1, from_state: '', to_state: 'reviewing', actor_type: 'user', reason: 'Submitted', created_at: at, audit_log_id: uuid(11) },
        { id: uuid(12), version: 2, from_state: 'reviewing', to_state: 'processing', actor_type: 'admin', reason: 'Approved', created_at: at, audit_log_id: uuid(13) },
        { id: uuid(14), version: 3, from_state: 'processing', to_state: 'paid', actor_type: 'system', reason: 'Paid', created_at: at, audit_log_id: uuid(15) },
      ] };
    } else if (url.pathname.endsWith('/withdrawals')) {
      data = { brand_id: brand, items: orders.slice(offset, offset + 50), limit: 50, offset, has_more: offset === 0 };
    } else data = orders.find(order => url.pathname.endsWith(order.id));
    await route.fulfill({ json: { success: true, data } });
  });
  await login(page);
  await page.locator('.brand-picker select').selectOption(brand);
  const panel = page.getByTestId('platform-withdrawals');
  const pages = page.getByTestId('platform-withdrawal-pages');
  await expect(panel.locator('tbody tr')).toHaveCount(50);
  await pages.getByRole('button', { name: 'Next page' }).click();
  await expect(panel.locator('tbody tr')).toHaveCount(1);
  await expect(pages.getByRole('button', { name: 'Next page' })).toBeDisabled();
  await panel.getByRole('button', { name: uuid(150), exact: true }).click();
  const detail = page.getByTestId('platform-withdrawal-detail');
  await expect(detail).toContainText(points);
  await expect(detail).toContainText('Recharge');
  await expect(page.getByTestId('platform-withdrawal-history').locator('tbody tr')).toHaveCount(3);
  await expect(detail).toContainText('Paid ledger entry');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  // Every mobile menu item must remain fully inside the viewport, not merely avoid body overflow.
  for (const nav of await page.locator('.nav-item').all()) {
    const box = await nav.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(page.viewportSize()!.width + 1);
  }
  await page.screenshot({ path: info.outputPath('platform-withdrawals.png'), fullPage: true });
  await panel.getByLabel('Member ID (optional)', { exact: true }).fill(member);
  await panel.getByLabel('State', { exact: true }).selectOption('paid');
  await panel.getByRole('button', { name: 'Query withdrawals', exact: true }).click();
  await expect(panel.locator('tbody tr')).toHaveCount(50);
  expect(matched).toBe(true);
  fail = true;
  await pages.getByRole('button', { name: 'Next page' }).click();
  await expect(panel.getByRole('alert')).toContainText('Withdrawal read unavailable');
  await expect(detail).toHaveCount(0);
  await page.locator('.brand-picker select').selectOption('');
  await expect(panel.getByLabel('State', { exact: true })).toHaveValue('');
  await expect(panel.getByLabel('Member ID (optional)', { exact: true })).toHaveValue('');
  await expect(panel.getByRole('button', { name: /Approve|Reject|Cancel|Mark paid/i })).toHaveCount(0);
});

test('platform withdrawal page uses the real read endpoint, not brand approval routes', async ({ page }) => {
  await login(page);
  const read = page.waitForResponse(r => r.url().endsWith('/api/v1/platform/withdrawals?limit=50&offset=0'));
  await page.locator('.brand-picker select').selectOption(brand);
  expect((await read).status()).toBe(200);
  await expect(page.getByTestId('platform-withdrawals').getByRole('alert')).toHaveCount(0);
  const response = await page.request.post(`${origin}/api/v1/platform/withdrawals/${uuid(100)}/approve`, {
    headers: { Origin: origin, 'X-Brand-ID': brand, 'Idempotency-Key': crypto.randomUUID() }, data: { version: 1, reason: 'verify platform approval route is not registered' },
  });
  expect([404, 405]).toContain(response.status());
});
