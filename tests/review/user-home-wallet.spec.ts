import { test, expect } from './platform-fixture';

const origin = 'http://127.0.0.1:5183';
const brandOrigin = 'http://127.0.0.1:5184';
const brand = '0199a000-0000-7000-8000-000000000001';

test('unknown games and empty confirmation route cannot use a demo order implementation', async ({ page }) => {
  const requested = page.waitForResponse(r => r.url().endsWith('/api/v1/games/not-a-game'));
  await page.goto(`${origin}/games/not-a-game/bet`);
  expect((await requested).status()).toBe(400);
  await expect(page.getByTestId('bet-selection-page')).toBeVisible();
  await expect(page.getByRole('alert').first()).toBeVisible();
  await expect(page.getByTestId('preview-button')).toHaveCount(0);
  await page.goto(`${origin}/bet/confirm`);
  await expect(page.locator('body')).not.toContainText('Rules version 1.2');
  await page.goto(`${origin}/orders`);
  await expect(page.locator('body')).not.toContainText('LP-18426');
  await expect(page.locator('body')).not.toContainText('Demo order');
});

test('signed-out home has no fictional points or local demo order path', async ({ page }) => {
  await page.goto(origin);
  const card = page.getByTestId('home-wallet-card');
  await expect(card.getByRole('link', { name: /Sign in/ })).toBeVisible();
  await expect(page.getByTestId('home-wallet-balance')).toHaveCount(0);
  await expect(page.locator('.page-footer')).toContainText('Platform points · payments not connected');
  await expect(page.locator('body')).not.toContainText('12,840');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
});

test('home wallet shows real recharge and clears the balance on failure and logout', async ({ page, playwright, operatorSession }, info) => {
  await page.goto(`${origin}/register`);
  await page.getByLabel('Choose a username or phone', { exact: true }).fill(`home_${crypto.randomUUID().replaceAll('-', '').slice(0, 10)}`);
  await page.getByLabel('Password', { exact: true }).fill('owned-home-review-password-2026');
  await page.locator('.auth-form input[type="checkbox"]').nth(0).check();
  await page.locator('.auth-form input[type="checkbox"]').nth(1).check();
  const registered = page.waitForResponse(r => r.url() === `${origin}/api/v1/auth/register`);
  await page.getByRole('button', { name: /Continue/ }).click();
  expect((await registered).status()).toBe(201);
  await expect(page).toHaveURL(/\/account$/);
  const me = await page.request.get(`${origin}/api/v1/me`);
  expect(me.status()).toBe(200);
  const member = (await me.json()).data.member.id;
  const operator = await playwright.request.newContext({ storageState: operatorSession });
  try {
    const headers = { Origin: brandOrigin, 'X-Brand-ID': brand };
    const created = await operator.post(`${brandOrigin}/api/v1/admin/recharges`, {
      headers: { ...headers, 'Idempotency-Key': crypto.randomUUID() },
      data: { member_id: member, points: '7', proof_reference: 'owned-home-review', remark: 'synthetic funding', reason: 'Prepare real home balance' },
    });
    expect(created.status()).toBe(201);
    const order = (await created.json()).data;
    const confirmed = await operator.post(`${brandOrigin}/api/v1/admin/recharges/${order.id}/confirm`, {
      headers: { ...headers, 'Idempotency-Key': crypto.randomUUID() }, data: { version: order.version, reason: 'Confirm owned home funding' },
    });
    expect(confirmed.status()).toBe(200);
  } finally { await operator.dispose(); }
  const walletBefore = (await (await page.request.get(`${origin}/api/v1/wallet`)).json()).data;
  const ledgerBefore = (await (await page.request.get(`${origin}/api/v1/wallet/ledger?limit=50&offset=0`)).json()).data;
  expect(walletBefore.available_points).toBe('7');
  await page.goto(origin);
  const card = page.getByTestId('home-wallet-card');
  await expect(page.getByTestId('home-wallet-balance')).toHaveText('7 pts');
  let fail = true;
  await page.route('**/api/v1/wallet', async route => {
    if (fail) await route.fulfill({ status: 503, json: { success: false, error: { code: 'TEST_WALLET_UNAVAILABLE', message: 'Wallet unavailable' } } });
    else await route.continue();
  });
  await card.getByRole('button', { name: 'Refresh', exact: true }).click();
  await expect(card.getByRole('alert')).toContainText('Wallet unavailable');
  await expect(page.getByTestId('home-wallet-balance')).toHaveCount(0);
  fail = false;
  await card.getByRole('button', { name: 'Refresh', exact: true }).click();
  await expect(page.getByTestId('home-wallet-balance')).toHaveText('7 pts');
  await page.locator('.language-button').click();
  await expect(card).toContainText('我的积分');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: info.outputPath('home-wallet-real.png'), fullPage: true });
  expect((await (await page.request.get(`${origin}/api/v1/wallet`)).json()).data).toEqual(walletBefore);
  expect((await (await page.request.get(`${origin}/api/v1/wallet/ledger?limit=50&offset=0`)).json()).data).toEqual(ledgerBefore);
  const logout = await page.request.post(`${origin}/api/v1/auth/logout`, { headers: { Origin: origin, 'Idempotency-Key': crypto.randomUUID() }, data: {} });
  expect(logout.status()).toBe(200);
  await card.getByRole('button', { name: '刷新', exact: true }).click();
  await expect(card.getByRole('link', { name: /登录/ })).toBeVisible();
  await expect(page.getByTestId('home-wallet-balance')).toHaveCount(0);
});

test('home wallet preserves large integer points and rejects malformed wallet data', async ({ page }) => {
  const amount = '9007199254740993';
  const grouped = '9,007,199,254,740,993';
  const by_source = Object.fromEntries(['recharge', 'winning', 'commission', 'gift'].map(source => [source, {
    available: source === 'recharge' ? amount : '0', manual_frozen: '0', system_frozen: '0', withdrawal: '0',
  }]));
  let invalid = false;
  await page.route('**/api/v1/wallet', route => route.fulfill({ json: { success: true, data: invalid ? {} : {
    account_id: crypto.randomUUID(), brand_id: brand, member_id: crypto.randomUUID(), version: 1,
    display_points: amount, available_points: amount, frozen_points: '0', withdrawal_points: '0',
    recharge_points: amount, winning_points: '0', gift_points: '0', commission_points: '0',
    manual_frozen_points: '0', system_frozen_points: '0', by_source,
  } } }));
  await page.goto(origin);
  const card = page.getByTestId('home-wallet-card');
  await expect(page.getByTestId('home-wallet-balance')).toHaveText(`${grouped} pts`);
  invalid = true;
  await card.getByRole('button', { name: 'Refresh', exact: true }).click();
  await expect(card.getByRole('alert')).toContainText('invalid wallet response');
  await expect(page.getByTestId('home-wallet-balance')).toHaveCount(0);
  await expect(card).not.toContainText(grouped);
});
