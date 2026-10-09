import type { Page } from '@playwright/test';
import { test, expect } from './platform-fixture';

const origin = 'http://127.0.0.1:5185';
const brand = '0199a000-0000-7000-8000-000000000001';
const uuid = (n: number) => `aaaaaaaa-aaaa-4aaa-8aaa-${n.toString(16).padStart(12, '0')}`;
const member = uuid(1);
const selection = { regular: [1], special: null, digits: null, exclude: null, attributes: null, features: null };
const definition = {
  schema_version: 1, model: { model: 'pick', regular_pool: { min: 1, max: 9, allow_repeat: false }, special_pool: { allow_repeat: false }, regular_count: 1, special_count: 0, pool_size: 9, total_count: 9, length: 1, allow_repeat: false, ordered: false },
  selection: { mode: 'numbers', regular_count: 1, special_count: 0, exclude_count: 0, attribute_groups: null, feature_choices: null }, number_attributes: null, unit_points: '1', prize_tiers: null, mixed_tier_policy: 'max_all', cap_points: null, rounding: 'half_up', rounding_scope: 'order', limits: { max_combinations: 100, max_multiplier: '100', max_bet_points: null },
};
const orders = Array.from({ length: 51 }, (_, i) => ({
  id: uuid(100 + i), brand_id: brand, global_user_id: uuid(2), brand_member_id: member, account_id: uuid(3), game_id: uuid(4), period_id: uuid(5), play_id: uuid(6), rule_version_id: uuid(7), definition_hash: 'a'.repeat(64), definition_snapshot: definition, status: 'placed', version: 1, selection_raw: selection, selection_normalized: selection, expanded_bets: [selection], unit_points: '1', combination_count: 1, multiplier: '1', total_points: '1', deduction_allocation: [{ source: 'recharge', state: 'available', points: '1' }], policy_snapshot: { min_bet_points: '1', max_bet_points: null, max_period_points: null, max_user_period_points: null, user_cancel_allowed: true }, policy_versions: { brand: 1, game: 1 }, debit_entry_id: uuid(8), client_key: `fixture-${i}`, placed_at: '2026-10-09T10:00:00Z', settlement_calculation_id: null, payout_entry_id: null, prize_points: '0', settled_at: null,
}));
async function login(page: Page) {
  await page.goto(origin);
  await expect(page.locator('.app-frame')).toBeVisible();
  await page.locator('.nav-item').filter({ hasText: 'Bet orders' }).click();
}

test('platform bet UI paginates original snapshots and applies member filters without mutations', async ({ page }, info) => {
  let fail = false;
  const filtered: string[] = [];
  await page.route('**/api/v1/platform/bet-orders**', async route => {
    expect(route.request().method()).toBe('GET');
    expect(route.request().headers()['x-brand-id']).toBe(brand);
    if (fail) { fail = false; await route.fulfill({ status: 503, json: { success: false, error: { code: 'TEST_READ_FAILED', message: 'Order read unavailable' } } }); return; }
    const url = new URL(route.request().url());
    const filter = url.searchParams.get('member_id');
    if (filter) filtered.push(filter);
    const offset = Number(url.searchParams.get('offset') || 0);
    const data = url.pathname.endsWith('/bet-orders') ? { items: orders.slice(offset, offset + 51) } : orders.find(order => url.pathname.endsWith(order.id));
    await route.fulfill({ json: { success: true, data } });
  });
  await login(page);
  await page.locator('.brand-picker select').selectOption(brand);
  const panel = page.getByTestId('platform-bets');
  const pages = page.getByTestId('platform-bet-pages');
  await expect(panel.locator('tbody tr')).toHaveCount(50);
  await pages.getByRole('button', { name: 'Next page' }).click();
  await expect(panel.locator('tbody tr')).toHaveCount(1);
  await expect(pages.getByRole('button', { name: 'Next page' })).toBeDisabled();
  await panel.getByRole('button', { name: uuid(150), exact: true }).click();
  const detail = page.getByTestId('platform-bet-detail');
  await expect(detail).toBeVisible();
  await detail.locator('summary').filter({ hasText: 'Rule definition' }).click();
  await expect(detail.locator('details[open] pre')).toContainText('"schema_version": 1');
  await detail.locator('summary').filter({ hasText: 'Deduction allocation' }).click();
  await expect(detail).toContainText('"recharge"');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: info.outputPath('platform-bet-detail.png'), fullPage: true });
  await panel.getByLabel('Member ID (optional)', { exact: true }).fill(member);
  await panel.getByRole('button', { name: 'Query orders', exact: true }).click();
  await expect(panel.locator('tbody tr')).toHaveCount(50);
  expect(filtered).toContain(member);
  fail = true;
  await pages.getByRole('button', { name: 'Next page' }).click();
  await expect(panel.getByRole('alert')).toContainText('Order read unavailable');
  await expect(detail).toHaveCount(0);
  await page.locator('.brand-picker select').selectOption('');
  await expect(panel.locator('tbody')).not.toContainText(uuid(150));
  await expect(panel.getByRole('button', { name: /Cancel order|Judge|Settle|Mark abnormal/i })).toHaveCount(0);
});

test('platform bet page reads the real brand endpoint and rejects financial write routes', async ({ page }) => {
  await login(page);
  const response = page.waitForResponse(r => r.url().endsWith('/api/v1/platform/bet-orders?limit=51&offset=0'));
  await page.locator('.brand-picker select').selectOption(brand);
  expect((await response).status()).toBe(200);
  await expect(page.getByTestId('platform-bets').getByRole('alert')).toHaveCount(0);
  const denied = await page.request.post(`${origin}/api/v1/platform/bet-orders/${uuid(100)}/cancel`, {
    headers: { Origin: origin, 'X-Brand-ID': brand, 'Idempotency-Key': crypto.randomUUID() }, data: { version: 1, reason: 'verify platform write route is not registered' },
  });
  expect([404, 405]).toContain(denied.status());
});
