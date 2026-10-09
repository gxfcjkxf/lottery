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

test('platform reads a genuine brand-created period and manual result without changing it', async ({ page, playwright }, info) => {
  test.setTimeout(60_000);
  const operator = await playwright.request.newContext();
  const brandOrigin = 'http://127.0.0.1:5184';
  const base = `${brandOrigin}/api/v1/admin`;
  const headers = { Origin: brandOrigin, 'X-Brand-ID': brand };
  const suffix = crypto.randomUUID().replaceAll('-', '').slice(0, 12);
  const name = `Platform real draw ${suffix}`;
  try {
    const login = await operator.post(`${base}/auth/login`, {
      headers: { ...headers, 'Idempotency-Key': crypto.randomUUID() },
      data: { identifier: 'review_operator', password: process.env.TEST_REVIEW_ADMIN_PASSWORD! },
    });
    expect(login.status()).toBe(200);
    const created = await operator.post(`${base}/games`, {
      headers: { ...headers, 'Idempotency-Key': crypto.randomUUID() },
      data: { code: `platform_draw_${suffix}`, name, model: games[0].model, timezone: 'UTC', reason: 'Owned synthetic game for real platform read verification' },
    });
    expect(created.status(), await created.text()).toBe(201);
    const game = (await created.json()).data;
    const drawAt = new Date(Date.now() + 20_000);
    drawAt.setUTCMilliseconds(0);
    const saved = await operator.put(`${base}/games/${game.id}/schedule`, {
      headers: { ...headers, 'Idempotency-Key': crypto.randomUUID() },
      data: { version: game.version, reason: 'Schedule one near-future owned draw', spec: {
        timezone: 'UTC', mode: 'daily', daily_draw_times: [drawAt.toISOString().slice(11, 19)], interval_seconds: 0,
        busy_windows: [], bet_open_before_seconds: 60, bet_close_before_seconds: 1, pause_dates: [], weekdays: [0, 1, 2, 3, 4, 5, 6], holiday_dates: [], holiday_policy: 'normal',
      } },
    });
    expect(saved.status(), await saved.text()).toBe(200);
    const generated = await operator.post(`${base}/games/${game.id}/periods/generate`, {
      headers: { ...headers, 'Idempotency-Key': crypto.randomUUID() },
      data: { from: new Date(Date.now() - 1000).toISOString(), to: new Date(drawAt.valueOf() + 1000).toISOString(), reason: 'Generate single real test period' },
    });
    expect(generated.status(), await generated.text()).toBe(200);
    const periods = (await generated.json()).data.periods;
    expect(periods).toHaveLength(1);
    const periodId = periods[0].id;
    async function readPeriod() {
      const response = await operator.get(`${base}/periods/${periodId}`, { headers });
      expect(response.status()).toBe(200);
      return (await response.json()).data;
    }
    await expect.poll(async () => (await readPeriod()).status, { timeout: 30_000, intervals: [200, 400, 800] }).toBe('waiting_draw');
    const ready = await readPeriod();
    const published = await operator.post(`${base}/periods/${periodId}/manual-draw`, {
      headers: { ...headers, 'Idempotency-Key': crypto.randomUUID() },
      data: { version: ready.version, period_no: ready.period_no, result: { regular: [], special: [], digits: [1, 2, 1] }, drawn_at: drawAt.toISOString(), reason: 'Publish owned synthetic 121 result' },
    });
    expect(published.status(), await published.text()).toBe(201);
    const draw = (await published.json()).data;
    const before = await readPeriod();
    const historyBefore = await operator.get(`${base}/periods/${periodId}/draw?limit=51&offset=0`, { headers });
    expect(historyBefore.status()).toBe(200);
    const original = (await historyBefore.json()).data;
    expect(original.current.id).toBe(draw.id);
    await page.goto(origin);
    await expect(page.locator('.app-frame')).toBeVisible();
    await page.locator('.nav-item').filter({ hasText: 'Bet orders' }).click();
    await page.locator('.brand-picker select').selectOption(brand);
    await page.getByTestId('platform-bet-tabs').getByRole('button', { name: 'Games and draws' }).click();
    await page.getByTestId('platform-lottery').getByRole('button', { name, exact: true }).click();
    await page.getByTestId('platform-game-periods').getByRole('button', { name: ready.period_no, exact: true }).click();
    const result = page.getByTestId('platform-period-draw');
    await expect(result).toContainText(draw.id);
    const numbers = JSON.parse(await result.locator('.bet-snapshots pre').first().innerText());
    expect(numbers).toEqual({ regular: [], special: [], digits: [1, 2, 1] });
    await expect(page.getByTestId('platform-lottery').getByRole('alert')).toHaveCount(0);
    const wrongBrand = await page.request.get(`${origin}/api/v1/platform/periods/${periodId}/draw?limit=51&offset=0`, { headers: { 'X-Brand-ID': '0199a000-0000-7000-8000-000000000002' } });
    expect(wrongBrand.status()).toBe(404);
    const denied = await page.request.post(`${origin}/api/v1/platform/periods/${periodId}/manual-draw`, {
      headers: { Origin: origin, 'X-Brand-ID': brand, 'Idempotency-Key': crypto.randomUUID() },
      data: { version: before.version, period_no: ready.period_no, result: { regular: [], special: [], digits: [9, 9, 9] }, drawn_at: drawAt.toISOString(), reason: 'Verify platform cannot replace result' },
    });
    expect([404, 405]).toContain(denied.status());
    expect(await readPeriod()).toEqual(before);
    const after = await operator.get(`${base}/periods/${periodId}/draw?limit=51&offset=0`, { headers });
    expect(after.status()).toBe(200);
    expect((await after.json()).data).toEqual(original);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    await page.screenshot({ path: info.outputPath('platform-real-draw.png'), fullPage: true });
  } finally { await operator.dispose(); }
});
