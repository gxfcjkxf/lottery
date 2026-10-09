import { test, expect } from './platform-fixture';

const origin = 'http://127.0.0.1:5185';
const brand = '0199a000-0000-7000-8000-000000000001';
const uuid = (n: number) => `aaaaaaaa-aaaa-4aaa-8aaa-${n.toString(16).padStart(12, '0')}`;
const member = uuid(1);
const at = '2026-10-09T10:00:00Z';
const points = '9007199254740993';
const records = Array.from({ length: 51 }, (_, i) => ({
  id: uuid(100 + i), brand_id: brand, member_id: member, account_id: uuid(2), points, state: 'confirmed',
  proof_reference: 'owned-ui-proof', remark: 'Synthetic list snapshot', created_by: uuid(3), confirmed_by: uuid(4),
  version: 2, created_at: at, confirmed_at: at, ledger_entry_id: uuid(5), audit_log_id: uuid(6),
}));

test('wallet recharge tab paginates exact list snapshots and clears scope without writes', async ({ page }, info) => {
  let fail = false;
  let filtered = false;
  await page.route('**/api/v1/platform/recharges?**', async route => {
    expect(route.request().method()).toBe('GET');
    expect(route.request().headers()['x-brand-id']).toBe(brand);
    if (fail) { fail = false; await route.fulfill({ status: 503, json: { success: false, error: { code: 'TEST_READ_FAILED', message: 'Recharge read unavailable' } } }); return; }
    const url = new URL(route.request().url());
    const offset = Number(url.searchParams.get('offset'));
    if (url.searchParams.get('member_id') === member) filtered = true;
    await route.fulfill({ json: { success: true, data: { items: records.slice(offset, offset + 51) } } });
  });
  await page.goto(origin);
  await expect(page.locator('.app-frame')).toBeVisible();
  await page.locator('.nav-item').filter({ hasText: 'Member points' }).click();
  await page.locator('.brand-picker select').selectOption(brand);
  await page.getByTestId('platform-finance-tabs').getByRole('button', { name: 'Recharge records', exact: true }).click();
  const panel = page.getByTestId('platform-recharges');
  const pages = page.getByTestId('platform-recharge-pages');
  await expect(panel.locator('tbody tr')).toHaveCount(50);
  await pages.getByRole('button', { name: 'Next page' }).click();
  await expect(panel.locator('tbody tr')).toHaveCount(1);
  await expect(pages.getByRole('button', { name: 'Next page' })).toBeDisabled();
  await panel.getByRole('button', { name: uuid(150), exact: true }).click();
  const detail = page.getByTestId('platform-recharge-detail');
  await expect(detail).toContainText(points);
  await expect(detail).toContainText('owned-ui-proof');
  await expect(detail).toContainText(uuid(5));
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: info.outputPath('platform-recharges.png'), fullPage: true });
  await panel.getByLabel('Member ID (optional)', { exact: true }).fill(member);
  await panel.getByRole('button', { name: 'Query recharges', exact: true }).click();
  await expect(panel.locator('tbody tr')).toHaveCount(50);
  expect(filtered).toBe(true);
  fail = true;
  await pages.getByRole('button', { name: 'Next page' }).click();
  await expect(panel.getByRole('alert')).toContainText('Recharge read unavailable');
  await expect(detail).toHaveCount(0);
  await page.locator('.brand-picker select').selectOption('');
  await expect(panel).toHaveCount(0);
  await expect(page.getByTestId('platform-wallet')).toBeVisible();
  await expect(page.getByRole('button', { name: /Create recharge|Confirm recharge|Make payment/i })).toHaveCount(0);
});

test('platform recharge query reads real records without changing wallet or ledger', async ({ page, playwright, operatorSession }) => {
  await page.goto('http://127.0.0.1:5183/register');
  await page.getByLabel('Choose a username or phone', { exact: true }).fill(`recharge_review_${crypto.randomUUID().replaceAll('-', '').slice(0, 10)}`);
  await page.getByLabel('Password', { exact: true }).fill('owned-recharge-review-password-2026');
  await page.locator('.auth-form input[type="checkbox"]').nth(0).check();
  await page.locator('.auth-form input[type="checkbox"]').nth(1).check();
  const registered = page.waitForResponse(r => r.url() === 'http://127.0.0.1:5183/api/v1/auth/register');
  await page.getByRole('button', { name: /Continue/ }).click();
  expect((await registered).status()).toBe(201);
  await expect(page).toHaveURL(/\/account$/);
  const me = await page.request.get('http://127.0.0.1:5183/api/v1/me');
  expect(me.status()).toBe(200);
  const memberId = (await me.json()).data.member.id;
  const operator = await playwright.request.newContext({ storageState: operatorSession });
  const brandOrigin = 'http://127.0.0.1:5184';
  const operationHeaders = { Origin: brandOrigin, 'X-Brand-ID': brand };
  let rechargeId: string;
  let ledgerId: string;
  try {
    const created = await operator.post(`${brandOrigin}/api/v1/admin/recharges`, {
      headers: { ...operationHeaders, 'Idempotency-Key': crypto.randomUUID() },
      data: { member_id: memberId, points: '7', proof_reference: 'owned-recharge-review', remark: 'owned synthetic funding', reason: 'prepare real readonly verification' },
    });
    expect(created.status()).toBe(201);
    const record = (await created.json()).data;
    rechargeId = record.id;
    const confirmed = await operator.post(`${brandOrigin}/api/v1/admin/recharges/${rechargeId}/confirm`, {
      headers: { ...operationHeaders, 'Idempotency-Key': crypto.randomUUID() },
      data: { version: record.version, reason: 'confirm owned synthetic funding' },
    });
    expect(confirmed.status()).toBe(200);
    ledgerId = (await confirmed.json()).data.ledger_entry_id;
  } finally { await operator.dispose(); }
  await page.goto(origin);
  await expect(page.locator('.app-frame')).toBeVisible();
  const headers = { 'X-Brand-ID': brand };
  const walletUrl = `${origin}/api/v1/platform/wallets/${memberId}`;
  const ledgerUrl = `${walletUrl}/ledger?limit=51&offset=0`;
  const read = async (url: string) => { const r = await page.request.get(url, { headers }); expect(r.status()).toBe(200); return (await r.json()).data; };
  const before = [await read(walletUrl), await read(ledgerUrl)];
  expect(before[0].available_points).toBe('7');
  expect(before[1].items).toHaveLength(1);
  await page.locator('.nav-item').filter({ hasText: 'Member points' }).click();
  const response = page.waitForResponse(r => r.url().endsWith('/api/v1/platform/recharges?limit=51&offset=0'));
  await page.locator('.brand-picker select').selectOption(brand);
  await page.getByTestId('platform-finance-tabs').getByRole('button', { name: 'Recharge records', exact: true }).click();
  expect((await response).status()).toBe(200);
  await page.getByTestId('platform-recharges').getByLabel('Member ID (optional)', { exact: true }).fill(memberId);
  const filtered = page.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/platform/recharges' && new URL(r.url()).searchParams.get('member_id') === memberId);
  await page.getByRole('button', { name: 'Query recharges', exact: true }).click();
  expect((await filtered).status()).toBe(200);
  await expect(page.getByTestId('platform-recharges').getByRole('alert')).toHaveCount(0);
  await page.getByTestId('platform-recharges').getByRole('button', { name: rechargeId!, exact: true }).click();
  await expect(page.getByTestId('platform-recharge-detail')).toContainText(ledgerId!);
  await expect(page.getByTestId('platform-recharge-detail')).toContainText('Confirmed');
  const denied = await page.request.post(`${origin}/api/v1/platform/recharges`, {
    headers: { Origin: origin, 'X-Brand-ID': brand, 'Idempotency-Key': crypto.randomUUID() },
    data: { member_id: memberId, points: '1', proof_reference: 'must-not-create', remark: 'deny platform mutation', reason: 'verify unregistered write route' },
  });
  expect([404, 405]).toContain(denied.status());
  expect([await read(walletUrl), await read(ledgerUrl)]).toEqual(before);
});
