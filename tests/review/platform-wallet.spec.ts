import type { Page } from '@playwright/test';
import { test, expect } from './platform-fixture';

const origin = 'http://127.0.0.1:5185';
const brand = '0199a000-0000-7000-8000-000000000001';
const uuid = (n: number) => `aaaaaaaa-aaaa-4aaa-8aaa-${n.toString(16).padStart(12, '0')}`;
const member = uuid(1);
const timestamp = '2026-10-09T10:00:00Z';
function buckets(amount: string) {
  return Object.fromEntries(['recharge', 'winning', 'gift', 'commission'].map(source => [source, { available: source === 'recharge' ? amount : '0', manual_frozen: '0', system_frozen: '0', withdrawal: '0' }]));
}
async function signIn(page: Page) {
  await page.goto(origin);
  await expect(page.locator('.app-frame')).toBeVisible();
}

test('wallet UI preserves exact points, displays before/change/after and paginates reads', async ({ page }, info) => {
  const amount = '9007199254740993';
  const wallet = {
    account_id: uuid(2), brand_id: brand, member_id: member, version: 51,
    display_points: amount, available_points: amount, frozen_points: '0', withdrawal_points: '0',
    recharge_points: amount, winning_points: '0', gift_points: '0', commission_points: '0', manual_frozen_points: '0', system_frozen_points: '0', by_source: buckets(amount),
  };
  const entries = Array.from({ length: 51 }, (_, i) => ({
    id: uuid(100 + i), brand_id: brand, account_id: uuid(2), member_id: member, entry_type: 'recharge', reference_type: 'recharge', reference_id: uuid(3), operation_key: `fixture-${i}`, reason: 'Read-only UI fixture', actor_type: 'admin', actor_id: uuid(4), request_id: `request-${i}`, version: i + 1,
    before_snapshot: buckets('9007199254740992'), delta_snapshot: buckets('1'), after_snapshot: buckets(amount), source_allocation: [{ source: 'recharge', state: 'available', points: '1' }], created_at: timestamp,
  }));
  let fail = false;
  await page.route('**/api/v1/platform/wallets/**', async route => {
    expect(route.request().method()).toBe('GET');
    expect(route.request().headers()['x-brand-id']).toBe(brand);
    if (fail) {
      fail = false;
      await route.fulfill({ status: 503, json: { success: false, error: { code: 'TEST_FAILED', message: 'Wallet read unavailable' } } });
      return;
    }
    const url = new URL(route.request().url());
    const offset = Number(url.searchParams.get('offset') || 0);
    await route.fulfill({ json: { success: true, data: url.pathname.endsWith('/ledger') ? { items: entries.slice(offset, offset + 51) } : wallet } });
  });
  await signIn(page);
  await page.locator('.nav-item').filter({ hasText: 'Member points' }).click();
  await page.locator('.brand-picker select').selectOption(brand);
  await page.getByLabel('Member ID', { exact: true }).fill(member);
  await page.getByRole('button', { name: 'Load wallet', exact: true }).click();
  await expect(page.getByTestId('platform-wallet-balances')).toContainText(amount);
  await expect(page.getByTestId('platform-wallet-ledger').locator('tbody tr')).toHaveCount(50);
  const pages = page.getByTestId('platform-wallet-pages');
  await pages.getByRole('button', { name: 'Next page' }).click();
  await expect(page.getByTestId('platform-wallet-ledger').locator('tbody tr')).toHaveCount(1);
  await expect(pages.getByRole('button', { name: 'Next page' })).toBeDisabled();
  await page.getByRole('button', { name: uuid(150), exact: true }).click();
  const detail = page.getByTestId('platform-wallet-entry');
  await expect(detail).toContainText('9007199254740992');
  await expect(detail).toContainText(amount);
  await expect(detail.locator('tbody tr')).toHaveCount(16);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: info.outputPath('platform-wallet.png'), fullPage: true });
  fail = true;
  await pages.getByRole('button', { name: 'Previous page' }).click();
  await expect(page.getByRole('alert')).toContainText('Wallet read unavailable');
  await expect(page.getByTestId('platform-wallet-ledger').locator('tbody tr')).toHaveCount(1);
  await expect(detail).toHaveCount(0);
  await page.getByRole('button', { name: 'Refresh', exact: true }).click();
  await expect(page.getByTestId('platform-wallet-ledger').locator('tbody tr')).toHaveCount(50);
  await page.locator('.brand-picker select').selectOption('');
  await expect(page.getByTestId('platform-wallet-balances')).toHaveCount(0);
  await expect(page.getByTestId('platform-wallet-ledger')).toHaveCount(0);
});

test('real platform member wallet query does not change its wallet or ledger', async ({ page }) => {
  // Own this member so fresh CI databases do not depend on another test's registration.
  await page.goto('http://127.0.0.1:5183/register');
  await page.getByLabel('Choose a username or phone', { exact: true }).fill(`wallet_review_${crypto.randomUUID().replaceAll('-', '').slice(0, 10)}`);
  await page.getByLabel('Password', { exact: true }).fill('review-only-wallet-member-password-2026');
  await page.locator('.auth-form input[type="checkbox"]').nth(0).check();
  await page.locator('.auth-form input[type="checkbox"]').nth(1).check();
  const registered = page.waitForResponse(r => r.url() === 'http://127.0.0.1:5183/api/v1/auth/register');
  await page.getByRole('button', { name: /Continue/ }).click();
  expect((await registered).status()).toBe(201);
  await expect(page).toHaveURL(/\/account$/);
  const me = await page.request.get('http://127.0.0.1:5183/api/v1/me');
  expect(me.status()).toBe(200);
  const id = (await me.json()).data.member.id;
  await signIn(page);
  const headers = { 'X-Brand-ID': brand };
  const membersResponse = await page.request.get(`${origin}/api/v1/platform/users?limit=50&offset=0`, { headers });
  expect(membersResponse.status()).toBe(200);
  const members = (await membersResponse.json()).data.items;
  expect(members.some((item: { id: string }) => item.id === id)).toBe(true);
  const walletUrl = `${origin}/api/v1/platform/wallets/${id}`;
  const ledgerUrl = `${walletUrl}/ledger?limit=51&offset=0`;
  const read = async (url: string) => {
    const response = await page.request.get(url, { headers });
    expect(response.status()).toBe(200);
    return (await response.json()).data;
  };
  const before = await read(walletUrl);
  const beforeLedger = await read(ledgerUrl);
  await page.locator('.nav-item').filter({ hasText: 'Brand members' }).click();
  await page.locator('.brand-picker select').selectOption(brand);
  const visibleRows = page.locator('.desktop-table tbody tr, .mobile-cards .user-card').filter({ hasText: id });
  await visibleRows.locator('button').filter({ hasText: 'Member points' }).filter({ visible: true }).click();
  await expect(page.getByTestId('platform-wallet-balances')).toContainText(before.available_points);
  await expect(page.getByTestId('platform-wallet-ledger')).toBeVisible();
  await expect(page.getByRole('alert')).toHaveCount(0);
  await expect(page.getByRole('button', { name: /Freeze|Adjust|Approve|Recharge/i })).toHaveCount(0);
  expect(await read(walletUrl)).toEqual(before);
  expect(await read(ledgerUrl)).toEqual(beforeLedger);
});
