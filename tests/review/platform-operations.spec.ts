import { selectControl } from '../select-control'
import { selectLanguage } from '../language-switcher'
import { test, expect } from './platform-fixture';

const origin = 'http://127.0.0.1:5185';
const brandOrigin = 'http://127.0.0.1:5184';
const brand = '0199a000-0000-7000-8000-000000000001';
const scoped = { 'X-Brand-ID': brand };

test('notification paging clears failed and changed-brand data', async ({ page }) => {
  let fail = false;
  const rows = Array.from({ length: 51 }, () => ({ event_id: crypto.randomUUID(), brand_id: brand, status: 'sent', attempt_count: 1, last_error: null, next_attempt_at: '2026-10-10T00:00:00Z', sent_at: '2026-10-10T00:00:01Z' }));
  await page.route('**/api/v1/platform/notification-deliveries?**', async route => {
    expect(route.request().method()).toBe('GET');
    const offset = Number(new URL(route.request().url()).searchParams.get('offset'));
    await route.fulfill(fail ? { status: 503, json: { success: false, error: { code: 'DELIVERY_UNAVAILABLE', message: 'Delivery unavailable' } } } : { json: { success: true, data: { items: rows.slice(offset, offset + 51) } } });
  });
  await page.goto(origin);
  await page.getByRole('button', { name: 'Audit and operations', exact: true }).click();
  await page.getByTestId('platform-operations-tabs').getByRole('button', { name: 'Notifications', exact: true }).click();
  await selectControl(page.locator(".topbar-brand-picker .select-control-trigger"), brand);
  const panel = page.getByTestId('platform-notifications');
  await expect(page.getByTestId('platform-delivery-list').locator('tbody tr')).toHaveCount(50);
  await page.getByTestId('platform-delivery-pages').getByRole('button', { name: 'Next', exact: true }).click();
  await expect(page.getByTestId('platform-delivery-list').locator('tbody tr')).toHaveCount(1);
  await expect(panel).toContainText(rows[50].event_id);
  fail = true;
  await panel.getByRole('button', { name: 'Refresh', exact: true }).click();
  await expect(panel.getByRole('alert')).toContainText('Delivery unavailable');
  await expect(panel).not.toContainText(rows[50].event_id);
  await selectControl(page.locator(".topbar-brand-picker .select-control-trigger"), '');
  await expect(panel.getByRole('alert')).toHaveCount(0);
  await expect(panel).not.toContainText(rows[0].event_id);
});
