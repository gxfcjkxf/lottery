import { test, expect } from './platform-fixture';

const origin = 'http://127.0.0.1:5185';
const brandOrigin = 'http://127.0.0.1:5184';
const brand = '0199a000-0000-7000-8000-000000000001';
const model = { model: 'DIGITS_0_9', regular_pool: { allow_repeat: false }, special_pool: { allow_repeat: false }, regular_count: 0, special_count: 0, pool_size: 0, total_count: 0, length: 3, allow_repeat: true, ordered: true };
const definition = {
  schema_version: 1, model, selection: { mode: 'numbers', regular_count: 0, special_count: 0, exclude_count: 0, attribute_groups: [], feature_choices: {} }, number_attributes: {}, unit_points: '1',
  prize_tiers: [{ code: 'EXACT', condition: { op: 'equals', field: 'position_match', value: 3 }, odds: '10', exclusive: true, cap_points: null }], mixed_tier_policy: 'max_all', cap_points: null, rounding: 'half_up', rounding_scope: 'order', limits: { max_combinations: 1000, max_multiplier: '1000', max_bet_points: null },
};

test('platform plays and versions paginate and clear failed details without writes', async ({ page }) => {
  const uuid = (n: number) => `aaaaaaaa-aaaa-4aaa-8aaa-${n.toString(16).padStart(12, '0')}`;
  const gameId = uuid(1);
  const plays = Array.from({ length: 51 }, (_, n) => ({ id: uuid(n + 10), brand_id: brand, game_id: gameId, code: `play_${n}`, name: `Play ${n}`, status: 'active', active_version_id: '', version: 1 }));
  const versions = Array.from({ length: 51 }, (_, n) => ({ id: uuid(n + 100), brand_id: brand, game_id: gameId, play_id: plays[50].id, version_no: n + 1, version: 1, definition, definition_hash: 'a'.repeat(64), status: 'draft', effect_mode: 'next_period', created_by: uuid(2), reviewed_by: '', review_comment: '', created_at: '2026-10-10T00:00:00Z', updated_at: '2026-10-10T00:00:00Z' }));
  let fail = false;
  for (const pattern of ['**/api/v1/platform/games?*', '**/api/v1/platform/games/*/periods?*', '**/api/v1/platform/games/*/plays?*', '**/api/v1/platform/plays/*/rule-versions?*', '**/api/v1/platform/rule-versions/*']) {
    await page.route(pattern, async route => {
      expect(route.request().method()).toBe('GET');
      expect(route.request().headers()['x-brand-id']).toBe(brand);
      const url = new URL(route.request().url());
      const limit = Number(url.searchParams.get('limit'));
      const offset = Number(url.searchParams.get('offset'));
      if (url.pathname.endsWith('/games')) await route.fulfill({ json: { success: true, data: { games: [{ id: gameId, brand_id: brand, code: 'test', name: 'Fixture game', model, status: 'active', timezone: 'UTC', version: 1, started_sequence: 0 }], limit, offset } } });
      else if (url.pathname.endsWith('/periods')) await route.fulfill({ json: { success: true, data: { periods: [], limit, offset } } });
      else if (url.pathname.endsWith('/plays')) await route.fulfill({ json: { success: true, data: { plays: plays.slice(offset, offset + limit), limit, offset } } });
      else if (url.pathname.endsWith('/rule-versions')) await route.fulfill({ json: { success: true, data: { versions: versions.slice(offset, offset + limit), limit, offset } } });
      else if (fail) await route.fulfill({ status: 503, json: { success: false, error: { code: 'TEST_FAILED', message: 'Rule read unavailable' } } });
      else await route.fulfill({ json: { success: true, data: versions.find(row => url.pathname.endsWith(row.id)) } });
    });
  }
  await page.goto(origin);
  await expect(page.locator('.app-frame')).toBeVisible();
  await page.locator('.nav-item').filter({ hasText: 'Bet orders' }).click();
  await page.locator('.brand-picker select').selectOption(brand);
  await page.getByTestId('platform-bet-tabs').getByRole('button', { name: 'Games and draws' }).click();
  await page.getByTestId('platform-lottery').getByRole('button', { name: 'Fixture game' }).click();
  await page.getByTestId('platform-game-periods').getByRole('button', { name: 'Plays and rules', exact: true }).click();
  const panel = page.getByTestId('platform-rules');
  await expect(panel.locator('tbody tr')).toHaveCount(50);
  await page.getByTestId('platform-play-pages').getByRole('button', { name: 'Next page' }).click();
  await expect(panel.locator('tbody tr')).toHaveCount(1);
  await panel.getByRole('button', { name: 'Play 50', exact: true }).click();
  const list = page.getByTestId('platform-rule-versions');
  await expect(list.locator('tbody tr')).toHaveCount(50);
  await page.getByTestId('platform-rule-pages').getByRole('button', { name: 'Next page' }).click();
  await expect(list.locator('tbody tr')).toHaveCount(1);
  await list.getByRole('button', { name: '51', exact: true }).click();
  await expect(page.getByTestId('platform-rule-detail')).toContainText('draft');
  fail = true;
  await list.getByRole('button', { name: '51', exact: true }).click();
  await expect(panel.getByRole('alert')).toContainText('Rule read unavailable');
  await expect(page.getByTestId('platform-rule-detail')).toHaveCount(0);
  await page.locator('.brand-picker select').selectOption('');
  await expect(panel).toHaveCount(0);
});

test('platform can read a real pending rule but cannot approve or alter it', async ({ page, playwright }, info) => {
  const operator = await playwright.request.newContext();
  const suffix = crypto.randomUUID().replaceAll('-', '').slice(0, 12);
  const name = `Platform rule ${suffix}`;
  const headers = { Origin: brandOrigin, 'X-Brand-ID': brand };
  const base = `${brandOrigin}/api/v1/admin`;
  async function post(path: string, body: unknown, status = 200) {
    const response = await operator.post(`${base}${path}`, { headers: { ...headers, 'Idempotency-Key': crypto.randomUUID() }, data: body });
    expect(response.status(), await response.text()).toBe(status);
    return (await response.json()).data;
  }
  try {
    await post('/auth/login', { identifier: 'review_operator', password: process.env.TEST_REVIEW_ADMIN_PASSWORD! });
    const game = await post('/games', { code: `platform_rule_${suffix}`, name, model, timezone: 'UTC', reason: 'Create owned rule query fixture' }, 201);
    const play = await post(`/games/${game.id}/plays`, { code: 'exact', name: 'Exact digits', reason: 'Create owned play' }, 201);
    const draft = await post('/rule-versions', { play_id: play.id, definition, effect_mode: 'immediate', reason: 'Create owned draft' }, 201);
    const validated = await post(`/rule-versions/${draft.id}/validate`, { version: draft.version, cases: [{
      name: '121 matches', selection: { regular: [], special: [], digits: [[1], [2], [1]], exclude: [], attributes: {}, features: {} }, draw: { regular: [], special: [], digits: [1, 2, 1] }, multiplier: '1', expected_bet_points: '1', expected_prize_points: '10', expected_won: true,
    }], reason: 'Validate owned draft' });
    expect(validated.validation.passed).toBe(true);
    await post(`/rule-versions/${draft.id}/submit-review`, { version: validated.version, reason: 'Submit for brand review only' });
    const read = await operator.get(`${base}/rule-versions/${draft.id}`, { headers });
    expect(read.status()).toBe(200);
    const before = (await read.json()).data;
    expect(before.status).toBe('pending_review');

    await page.goto(origin);
    await expect(page.locator('.app-frame')).toBeVisible();
    await page.locator('.nav-item').filter({ hasText: 'Bet orders' }).click();
    await page.locator('.brand-picker select').selectOption(brand);
    await page.getByTestId('platform-bet-tabs').getByRole('button', { name: 'Games and draws' }).click();
    await page.getByTestId('platform-lottery').getByRole('button', { name, exact: true }).click();
    await page.getByTestId('platform-game-periods').getByRole('button', { name: 'Plays and rules', exact: true }).click();
    const panel = page.getByTestId('platform-rules');
    await panel.getByRole('button', { name: 'Exact digits', exact: true }).click();
    await page.getByTestId('platform-rule-versions').getByRole('button', { name: '1', exact: true }).click();
    const detail = page.getByTestId('platform-rule-detail');
    await expect(detail).toContainText('pending_review');
    await expect(detail).toContainText(before.definition_hash);
    await detail.getByText('Rule definition', { exact: true }).click();
    expect(JSON.parse(await detail.locator('details[open] pre').innerText())).toEqual(before.definition);
    await detail.getByText('Validation report', { exact: true }).click();
    await expect(detail).toContainText('"passed": true');
    await expect(panel.getByRole('alert')).toHaveCount(0);
    await expect(panel.getByRole('button', { name: /Approve|Reject|Publish|Edit|Submit review/i })).toHaveCount(0);
    const denied = await page.request.post(`${origin}/api/v1/platform/rule-versions/${draft.id}/approve`, { headers: { Origin: origin, 'X-Brand-ID': brand, 'Idempotency-Key': crypto.randomUUID() }, data: { version: before.version, reason: 'Platform must not review', warnings_acknowledged: true } });
    expect([404, 405]).toContain(denied.status());
    const wrongBrand = await page.request.get(`${origin}/api/v1/platform/rule-versions/${draft.id}`, { headers: { 'X-Brand-ID': '0199a000-0000-7000-8000-000000000002' } });
    expect(wrongBrand.status()).toBe(404);
    const after = await operator.get(`${base}/rule-versions/${draft.id}`, { headers });
    expect(after.status()).toBe(200);
    expect((await after.json()).data).toEqual(before);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    await page.screenshot({ path: info.outputPath('platform-real-rule.png'), fullPage: true });
    await page.locator('.brand-picker select').selectOption('');
    await expect(detail).toHaveCount(0);
    await expect(panel).toHaveCount(0);
  } finally { await operator.dispose(); }
});
