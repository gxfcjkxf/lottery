import { test, expect } from './platform-fixture';

test.use({ platformAccountPrefix: 'review_reports' });
const origin = 'http://127.0.0.1:5185';
const brand = '0199a000-0000-7000-8000-000000000001';
const kinds = ['betting', 'ledger', 'withdrawal', 'commission', 'rewards', 'reward_orders'] as const;

test('platform ledger reports preserve signed totals, paginate and clear failed snapshots', async ({ page }, info) => {
  const amount = '9007199254740993';
  const fields = ['entry_count', 'net_points', 'recharge_points', 'prize_credit_points', 'prize_reversal_points', 'refund_points'];
  const totals = Object.fromEntries(fields.map(field => [field, field === 'net_points' ? `-${amount}` : field === 'entry_count' ? '1' : '0']));
  const rows = Array.from({ length: 51 }, (_, n) => {
    const key = new Date(Date.UTC(2026, 7, n + 1)).toISOString().slice(0, 10);
    return { key, label: key, totals };
  });
  let fail = false;
  await page.route('**/api/v1/platform/reports/ledger?**', async route => {
    expect(route.request().method()).toBe('GET');
    expect(route.request().headers()['x-brand-id']).toBe(brand);
    if (fail) {
      await route.fulfill({ status: 503, json: { success: false, error: { code: 'REPORT_UNAVAILABLE', message: 'Report unavailable' } } });
      return;
    }
    const params = new URL(route.request().url()).searchParams;
    const offset = Number(params.get('offset'));
    const limit = Number(params.get('limit'));
    await route.fulfill({ json: { success: true, data: {
      brand_id: brand, snapshot_at: '2026-10-10T00:00:00Z', timezone: 'UTC',
      query: { from: params.get('from'), to: params.get('to'), group_by: params.get('group_by'), limit, offset, game_id: null, member_id: params.get('member_id') },
      summary: { ...totals, entry_count: '51', net_points: (-BigInt(amount) * 51n).toString() },
      items: rows.slice(offset, offset + limit), total_groups: '51',
      balances: { account_count: '1', available_points: amount, frozen_points: '0', withdrawal_points: '0', total_points: amount },
    } } });
  });
  await page.goto(origin);
  await expect(page.locator('.app-frame')).toBeVisible();
  await page.locator('.nav-item').filter({ hasText: 'Member points' }).click();
  await page.locator('.brand-picker select').selectOption(brand);
  await page.getByTestId('platform-finance-tabs').getByRole('button', { name: 'Financial reports', exact: true }).click();
  const panel = page.getByTestId('platform-reports');
  await panel.getByLabel('Report type', { exact: true }).selectOption('ledger');
  await panel.getByLabel('From (UTC)', { exact: true }).fill('2026-08-01T00:00');
  await panel.getByLabel('To (UTC)', { exact: true }).fill('2026-10-11T00:00');
  await panel.getByRole('button', { name: 'Run report', exact: true }).click();
  await expect(page.getByTestId('platform-reports-list').locator('tbody tr')).toHaveCount(50);
  await expect(page.getByTestId('platform-reports-summary')).toContainText((-BigInt(amount) * 51n).toString());
  await expect(page.getByTestId('platform-reports-balances')).toContainText(amount);
  await page.getByTestId('platform-reports-pages').getByRole('button', { name: 'Next page' }).click();
  await expect(page.getByTestId('platform-reports-list').locator('tbody tr')).toHaveCount(1);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: info.outputPath('platform-ledger-report.png'), fullPage: true });
  fail = true;
  await panel.getByRole('button', { name: 'Run report', exact: true }).click();
  await expect(panel.getByRole('alert')).toContainText('Report unavailable');
  await expect(page.getByTestId('platform-reports-summary')).toHaveCount(0);
  await expect(page.getByTestId('platform-reports-balances')).toHaveCount(0);
  await page.locator('.language').click();
  await expect(panel.getByRole('heading', { name: '运营报表', exact: true })).toBeVisible();
  await page.locator('.brand-picker select').selectOption('');
  await expect(panel).toHaveCount(0);
});

test('platform reads all six genuine report endpoints and actual recharge postings without changing funds', async ({ page, playwright, operatorSession }) => {
  await page.goto('http://127.0.0.1:5183/register');
  await page.getByLabel('Choose a username or phone', { exact: true }).fill(`report_review_${crypto.randomUUID().replaceAll('-', '').slice(0, 10)}`);
  await page.getByLabel('Password', { exact: true }).fill('owned-report-review-password-2026');
  await page.locator('.auth-form input[type="checkbox"]').nth(0).check();
  await page.locator('.auth-form input[type="checkbox"]').nth(1).check();
  const registered = page.waitForResponse(r => r.url() === 'http://127.0.0.1:5183/api/v1/auth/register');
  await page.getByRole('button', { name: /Continue/ }).click();
  expect((await registered).status()).toBe(201);
  await expect(page).toHaveURL(/\/account$/);
  const me = await page.request.get('http://127.0.0.1:5183/api/v1/me');
  expect(me.status()).toBe(200);
  const member = (await me.json()).data.member.id;
  const operator = await playwright.request.newContext({ storageState: operatorSession });
  const brandOrigin = 'http://127.0.0.1:5184';
  const headers = { Origin: brandOrigin, 'X-Brand-ID': brand };
  try {
    const created = await operator.post(`${brandOrigin}/api/v1/admin/recharges`, {
      headers: { ...headers, 'Idempotency-Key': crypto.randomUUID() },
      data: { member_id: member, points: '7', proof_reference: 'owned-report-review', remark: 'synthetic funding', reason: 'Prepare actual posting report' },
    });
    expect(created.status()).toBe(201);
    const order = (await created.json()).data;
    const confirmed = await operator.post(`${brandOrigin}/api/v1/admin/recharges/${order.id}/confirm`, {
      headers: { ...headers, 'Idempotency-Key': crypto.randomUUID() }, data: { version: order.version, reason: 'Confirm owned posting report funding' },
    });
    expect(confirmed.status()).toBe(200);
  } finally { await operator.dispose(); }
  const readWallet = async () => {
    const response = await page.request.get(`${origin}/api/v1/platform/wallets/${member}`, { headers: { 'X-Brand-ID': brand } });
    expect(response.status()).toBe(200);
    return (await response.json()).data;
  };
  const before = await readWallet();
  expect(before.available_points).toBe('7');
  await page.goto(origin);
  await expect(page.locator('.app-frame')).toBeVisible();
  await page.locator('.nav-item').filter({ hasText: 'Member points' }).click();
  await page.locator('.brand-picker select').selectOption(brand);
  await page.getByTestId('platform-finance-tabs').getByRole('button', { name: 'Financial reports', exact: true }).click();
  const panel = page.getByTestId('platform-reports');
  const today = new Date();
  const tomorrow = new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate() + 1));
  await panel.getByLabel('From (UTC)', { exact: true }).fill(`${today.toISOString().slice(0, 10)}T00:00`);
  await panel.getByLabel('To (UTC)', { exact: true }).fill(`${tomorrow.toISOString().slice(0, 10)}T00:00`);
  await panel.getByLabel('Member ID (optional)', { exact: true }).fill(member);
  for (const kind of kinds) {
    await panel.getByLabel('Report type', { exact: true }).selectOption(kind);
    await expect(page.getByTestId('platform-reports-summary')).toHaveCount(0);
    const segment = kind === 'reward_orders' ? 'reward-orders' : kind;
    const loaded = page.waitForResponse(r => new URL(r.url()).pathname === `/api/v1/platform/reports/${segment}`);
    await panel.getByRole('button', { name: 'Run report', exact: true }).click();
    const response = await loaded;
    expect(response.status(), await response.text()).toBe(200);
    const data = (await response.json()).data;
    expect(data.brand_id).toBe(brand);
    expect(data.query.member_id).toBe(member);
    await expect(page.getByTestId('platform-reports-summary')).toBeVisible();
    await expect(panel.getByRole('alert')).toHaveCount(0);
    if (kind === 'ledger') {
      expect(data.summary.entry_count).toBe('1');
      expect(data.summary.recharge_points).toBe('7');
      expect(data.summary.net_points).toBe('7');
      expect(data.balances.available_points).toBe('7');
    }
  }
  await panel.getByLabel('Member ID (optional)', { exact: true }).fill('0199a000-0000-7000-8000-000000000099');
  await expect(page.getByTestId('platform-reports-summary')).toHaveCount(0);
  await expect(panel.getByRole('button', { name: /Approve|Pay out|Create|Retry/i })).toHaveCount(0);
  const denied = await page.request.post(`${origin}/api/v1/platform/reports/ledger`, { headers: { Origin: origin, 'X-Brand-ID': brand }, data: {} });
  expect([404, 405]).toContain(denied.status());
  expect(await readWallet()).toEqual(before);
});
