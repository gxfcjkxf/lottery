import { test, expect } from './platform-fixture';

const origin = 'http://127.0.0.1:5185';
const brand = '0199a000-0000-7000-8000-000000000001';
const id = (n: number) => `aaaaaaaa-aaaa-4aaa-8aaa-${n.toString(16).padStart(12, '0')}`;
const at = '2026-10-09T10:00:00Z';
const games = Array.from({ length: 51 }, (_, n) => ({ id: id(n + 1), brand_id: brand, code: `digits_${n}`, name: `Digits ${n}`, timezone: 'Asia/Singapore', status: 'active', version: 1, started_sequence: 1,
  model: { model: 'DIGITS_0_9', regular_pool: { allow_repeat: false }, special_pool: { allow_repeat: false }, regular_count: 0, special_count: 0, pool_size: 0, total_count: 0, length: 3, allow_repeat: true, ordered: true } }));
const period = { id: id(100), brand_id: brand, game_id: id(51), period_no: '20261009-001', sequence: 1, bet_start_at: at, bet_end_at: at, draw_at: at, status: 'drawn', version: 1, state_reason: '' };
const draw = { id: id(101), brand_id: brand, game_id: period.game_id, period_id: period.id, source_id: id(102), kind: 'manual', result: { regular: [], special: [], digits: [1, 2, 1] }, result_hash: 'hash', drawn_at: at, created_at: at };

test('platform catalogue pages games, reads periods and results without write controls', async ({ page }, info) => {
  await page.route('**/api/v1/platform/games**', async route => {
    expect(route.request().method()).toBe('GET');
    expect(route.request().headers()['x-brand-id']).toBe(brand);
    const url = new URL(route.request().url());
    const offset = Number(url.searchParams.get('offset'));
    const limit = Number(url.searchParams.get('limit'));
    const data = url.pathname.endsWith('/periods') ? { periods: [period], limit, offset } : { games: games.slice(offset, offset + limit), limit, offset };
    await route.fulfill({ json: { success: true, data } });
  });
  await page.route('**/api/v1/platform/periods/*/draw?*', async route => {
    expect(route.request().method()).toBe('GET');
    expect(route.request().headers()['x-brand-id']).toBe(brand);
    await route.fulfill({ json: { success: true, data: { current: draw, history: [draw], attempts: [], limit: 51, offset: 0 } } });
  });
  await page.goto(origin);
  await expect(page.locator('.app-frame')).toBeVisible();
  await page.locator('.nav-item').filter({ hasText: 'Bet orders' }).click();
  await page.locator('.brand-picker select').selectOption(brand);
  await page.getByTestId('platform-bet-tabs').getByRole('button', { name: 'Games and draws' }).click();
  const panel = page.getByTestId('platform-lottery');
  await expect(panel.locator('tbody tr')).toHaveCount(50);
  await page.getByTestId('platform-game-pages').getByRole('button', { name: 'Next page' }).click();
  await expect(panel.locator('tbody tr')).toHaveCount(1);
  await panel.getByRole('button', { name: 'Digits 50', exact: true }).click();
  await page.getByTestId('platform-game-periods').getByRole('button', { name: period.period_no }).click();
  await expect(page.getByTestId('platform-period-draw')).toContainText('"digits"');
  await expect(panel.getByRole('alert')).toHaveCount(0);
  await expect(panel.getByRole('button', { name: /Approve|Manual draw|Settle|Edit/i })).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: info.outputPath('platform-lottery.png'), fullPage: true });
  await page.locator('.brand-picker select').selectOption('');
  await expect(panel).toHaveCount(0);
  await expect(page.getByTestId('platform-bet-detail')).toHaveCount(0);
});

test('platform catalogue uses the real games endpoint and has no manual draw route', async ({ page }) => {
  await page.goto(origin);
  await expect(page.locator('.app-frame')).toBeVisible();
  await page.locator('.nav-item').filter({ hasText: 'Bet orders' }).click();
  await page.locator('.brand-picker select').selectOption(brand);
  const response = page.waitForResponse(r => r.url().endsWith('/api/v1/platform/games?limit=51&offset=0'));
  await page.getByTestId('platform-bet-tabs').getByRole('button', { name: 'Games and draws' }).click();
  expect((await response).status()).toBe(200);
  await expect(page.getByTestId('platform-lottery').getByRole('alert')).toHaveCount(0);
  const denied = await page.request.post(`${origin}/api/v1/platform/periods/${period.id}/manual-draw`, { headers: { Origin: origin, 'X-Brand-ID': brand }, data: {} });
  expect([404, 405]).toContain(denied.status());
});
