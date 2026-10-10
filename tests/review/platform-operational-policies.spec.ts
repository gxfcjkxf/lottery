import { test, expect } from './platform-fixture';

const origin = 'http://127.0.0.1:5185';
const brand = '0199a000-0000-7000-8000-000000000001';
const headers = { 'X-Brand-ID': brand };
const kinds = [
  ['betting', 'bet-policy'], ['settlement', 'settlement-policy'], ['agents', 'agent-policy'],
  ['compliance', 'compliance-policy'], ['payment', 'commission-payment-policy'], ['correction', 'commission-correction-policy'],
] as const;

test('platform reads six genuine operational policies without writes or verification claims', async ({ page }, info) => {
  const before = new Map<string, unknown>();
  for (const [, path] of kinds) {
    const response = await page.request.get(`${origin}/api/v1/platform/${path}`, { headers });
    expect(response.status(), await response.text()).toBe(200);
    before.set(path, (await response.json()).data);
  }
  await page.goto(origin);
  await page.getByRole('button', { name: 'Audit and operations', exact: true }).click();
  await page.locator('.content > .brand-picker select').selectOption(brand);
  await page.getByTestId('platform-operations-tabs').getByRole('button', { name: 'Operational policies', exact: true }).click();
  const panel = page.getByTestId('platform-operational-policies');
  for (const [kind] of kinds) {
    await panel.getByTestId('platform-operational-policy-select').selectOption(kind);
    await expect(page.getByTestId('platform-operational-policy-detail')).toBeVisible();
    await expect(page.getByTestId('platform-operational-policy-detail')).toContainText(brand);
    await expect(panel.getByRole('alert')).toHaveCount(0);
    if (kind === 'compliance') {
      await expect(panel).toContainText('Real verification and risk adapters are not connected');
      for (const label of ['Account risk check', 'Betting risk check', 'Blacklist/self-exclusion check', 'Responsible gambling/cooling check']) await expect(panel).toContainText(label);
    }
  }
  await page.locator('.language').click();
  await expect(panel).toContainText('此只读开关不会启用处理或派发资金');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: info.outputPath('platform-operational-policies.png'), fullPage: true });
  await expect(panel.getByRole('button', { name: /Save|Approve|Retry|Check|保存|批准|重试|执行检查/ })).toHaveCount(0);
  for (const [path, data] of before) {
    const denied = await page.request.put(`${origin}/api/v1/platform/${path}`, { headers: { ...headers, Origin: origin }, data: {} });
    expect([404, 405]).toContain(denied.status());
    const response = await page.request.get(`${origin}/api/v1/platform/${path}`, { headers });
    expect(response.status()).toBe(200);
    expect((await response.json()).data).toEqual(data);
  }
  await page.locator('.content > .brand-picker select').selectOption('');
  await expect(page.getByTestId('platform-operational-policy-detail')).toHaveCount(0);
});

test('operational policy views preserve exact caps, null settlement and fail closed per query', async ({ page }) => {
  const amount = '9007199254740993';
  const stamp = '2026-10-10T00:00:00Z';
  let fail = false;
  await page.route('**/api/v1/platform/bet-policy', route => route.fulfill({ json: { success: true, data: {
    brand_id: brand, version: 1, updated_at: stamp,
    config: { min_bet_points: '1', max_bet_points: amount, max_period_points: null, max_user_period_points: null, user_cancel_allowed: false },
  } } }));
  await page.route('**/api/v1/platform/settlement-policy', async route => {
    await route.fulfill(fail ? { status: 403, json: { success: false, error: { message: 'Settlement policy denied' } } } : { json: { success: true, data: { brand_id: brand, version: 1, mode: null, updated_at: stamp } } });
  });
  await page.goto(origin);
  await page.getByRole('button', { name: 'Audit and operations', exact: true }).click();
  await page.locator('.content > .brand-picker select').selectOption(brand);
  await page.getByTestId('platform-operations-tabs').getByRole('button', { name: 'Operational policies', exact: true }).click();
  const panel = page.getByTestId('platform-operational-policies');
  await expect(panel).toContainText(amount);
  await expect(panel).toContainText('Unlimited');
  await panel.getByTestId('platform-operational-policy-select').selectOption('settlement');
  await expect(page.getByTestId('platform-operational-policy-detail')).toContainText('Not configured');
  fail = true;
  await panel.getByRole('button', { name: 'Refresh', exact: true }).click();
  await expect(panel.getByRole('alert')).toContainText('Settlement policy denied');
  await expect(page.getByTestId('platform-operational-policy-detail')).toHaveCount(0);
  await panel.getByTestId('platform-operational-policy-select').selectOption('betting');
  await expect(panel).toContainText(amount);
  await expect(panel.getByRole('alert')).toHaveCount(0);
});
