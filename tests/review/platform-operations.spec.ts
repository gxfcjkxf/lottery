import { createHash } from 'node:crypto';
import { readFile } from 'node:fs/promises';
import { test, expect } from './platform-fixture';

const origin = 'http://127.0.0.1:5185';
const brandOrigin = 'http://127.0.0.1:5184';
const brand = '0199a000-0000-7000-8000-000000000001';
const scoped = { 'X-Brand-ID': brand };

test('platform reads genuine archive bytes and notification history without write routes', async ({ page, playwright, operatorSession }, info) => {
  await page.goto('http://127.0.0.1:5183/register');
  await page.getByLabel('Choose a username or phone', { exact: true }).fill(`operations_${crypto.randomUUID().replaceAll('-', '').slice(0, 10)}`);
  await page.getByLabel('Password', { exact: true }).fill('owned-operations-review-password-2026');
  await page.locator('.auth-form input[type="checkbox"]').nth(0).check();
  await page.locator('.auth-form input[type="checkbox"]').nth(1).check();
  const registered = page.waitForResponse(r => r.url() === 'http://127.0.0.1:5183/api/v1/auth/register');
  await page.getByRole('button', { name: /Continue/ }).click();
  expect((await registered).status()).toBe(201);
  await expect(page).toHaveURL(/\/account$/);
  const memberResponse = await page.request.get('http://127.0.0.1:5183/api/v1/me');
  expect(memberResponse.status()).toBe(200);
  const member = (await memberResponse.json()).data.member.id;
  const readWallet = async () => {
    const response = await page.request.get(`${origin}/api/v1/platform/wallets/${member}`, { headers: scoped });
    expect(response.status()).toBe(200);
    return (await response.json()).data;
  };
  const period = new Date();
  period.setUTCDate(period.getUTCDate() - (info.project.name === 'desktop1440' ? 2 : 3));
  const key = period.toISOString().slice(0, 10);
  let revision = 0;
  for (let offset = 0; ; offset += 100) {
    const response = await page.request.get(`${origin}/api/v1/platform/report-archives?limit=100&offset=${offset}`, { headers: scoped });
    expect(response.status(), await response.text()).toBe(200);
    const data = (await response.json()).data;
    for (const item of data.items) if (item.window.kind === 'daily' && item.window.period_key === key) revision = Math.max(revision, item.revision);
    if (BigInt(offset + data.items.length) >= BigInt(data.total_count)) break;
  }
  const operator = await playwright.request.newContext({ storageState: operatorSession });
  let archive: any;
  try {
    const recharge = await operator.post(`${brandOrigin}/api/v1/admin/recharges`, {
      headers: { Origin: brandOrigin, ...scoped, 'Idempotency-Key': crypto.randomUUID() },
      data: { member_id: member, points: '7', proof_reference: 'owned-operations-review', remark: 'synthetic funding', reason: 'Prepare real notification and balance check' },
    });
    expect(recharge.status(), await recharge.text()).toBe(201);
    const order = (await recharge.json()).data;
    const confirmed = await operator.post(`${brandOrigin}/api/v1/admin/recharges/${order.id}/confirm`, {
      headers: { Origin: brandOrigin, ...scoped, 'Idempotency-Key': crypto.randomUUID() }, data: { version: order.version, reason: 'Confirm owned operations review funding' },
    });
    expect(confirmed.status(), await confirmed.text()).toBe(200);
    const me = await operator.get(`${brandOrigin}/api/v1/admin/me`);
    expect(me.status()).toBe(200);
    const actor = (await me.json()).data.account.id;
    const created = await operator.post(`${brandOrigin}/api/v1/admin/report-archives`, {
      headers: { Origin: brandOrigin, ...scoped, 'Idempotency-Key': crypto.randomUUID(), 'X-Report-Archive-Actor-ID': actor },
      data: { kind: 'daily', period_key: key, expected_revision: revision, reason: 'Owned read-only platform browser verification' },
    });
    expect(created.status(), await created.text()).toBe(201);
    archive = (await created.json()).data;
  } finally { await operator.dispose(); }
  const walletBefore = await readWallet();
  expect(walletBefore.available_points).toBe('7');
  const original = await page.request.get(`${origin}/api/v1/platform/report-archives/${archive.id}/download`, { headers: scoped });
  expect(original.status(), await original.text()).toBe(200);
  const bytes = await original.body();
  expect(createHash('sha256').update(bytes).digest('hex')).toBe(archive.payload_sha256);
  const templatesResponse = await page.request.get(`${origin}/api/v1/platform/notification-templates`, { headers: scoped });
  expect(templatesResponse.status()).toBe(200);
  const templates = (await templatesResponse.json()).data;
  const selected = templates.items.find((item: any) => item.key === 'recharge.confirmed');
  expect(selected).toBeDefined();
  const historyURL = `${origin}/api/v1/platform/notification-templates/recharge.confirmed/history?limit=51&offset=0`;
  const historyResponse = await page.request.get(historyURL, { headers: scoped });
  expect(historyResponse.status()).toBe(200);
  const history = (await historyResponse.json()).data;
  expect(history.items.length).toBeGreaterThan(0);

  await page.goto(origin);
  await page.getByRole('button', { name: 'Audit and operations', exact: true }).click();
  await page.locator('.brand-picker select').selectOption(brand);
  const tabs = page.getByTestId('platform-operations-tabs');
  await tabs.getByRole('button', { name: 'Report archives', exact: true }).click();
  await page.getByTestId('platform-archive-list').getByRole('button', { name: archive.id, exact: true }).click();
  const detail = page.getByTestId('platform-archive-detail');
  await expect(detail).toContainText(archive.payload_sha256);
  await expect(detail).toContainText('not balances at the end');
  const downloaded = page.waitForEvent('download');
  await detail.getByRole('button', { name: 'Download original JSON', exact: true }).click();
  const file = await downloaded;
  expect(file.suggestedFilename()).toBe(`report-archive-${archive.id}-v${archive.revision}.json`);
  expect(await readFile((await file.path())!)).toEqual(bytes);
  await expect(page.getByTestId('platform-archive-download-receipt')).toContainText(archive.payload_sha256);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: info.outputPath('platform-archive.png'), fullPage: true });

  const deliveriesLoaded = page.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/platform/notification-deliveries');
  await tabs.getByRole('button', { name: 'Notifications', exact: true }).click();
  const deliveriesResponse = await deliveriesLoaded;
  expect(deliveriesResponse.status()).toBe(200);
  const deliveries = (await deliveriesResponse.json()).data.items;
  expect(deliveries.length).toBeGreaterThan(0);
  await expect(page.getByTestId('platform-delivery-list')).toContainText(deliveries[0].event_id);
  await page.getByTestId('platform-notification-tabs').getByRole('button', { name: 'Templates', exact: true }).click();
  await page.getByTestId('platform-template-list').getByRole('button', { name: 'recharge.confirmed', exact: true }).click();
  await expect(page.getByTestId('platform-template-detail')).toContainText(selected.content.en.title);
  await expect(page.getByTestId('platform-template-detail')).toContainText(selected.content['zh-CN'].title);
  await expect(page.getByTestId('platform-template-history')).toContainText(history.items[0].reason);
  await page.locator('.language').click();
  await expect(page.getByTestId('platform-template-detail')).toContainText('修订历史');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: info.outputPath('platform-notifications.png'), fullPage: true });
  await expect(page.getByTestId('platform-notifications').getByRole('button', { name: /Create|Save|Retry|重试|保存|创建/ })).toHaveCount(0);
  for (const [method, path] of [['POST', '/report-archives'], ['POST', `/notification-deliveries/${deliveries[0].event_id}/retry`], ['PUT', '/notification-templates/recharge.confirmed']]) {
    const denied = await page.request.fetch(`${origin}/api/v1/platform${path}`, { method, headers: { Origin: origin, ...scoped }, data: {} });
    expect([404, 405]).toContain(denied.status());
  }
  const after = await page.request.get(`${origin}/api/v1/platform/report-archives/${archive.id}/download`, { headers: scoped });
  expect(await after.body()).toEqual(bytes);
  expect((await (await page.request.get(`${origin}/api/v1/platform/notification-templates`, { headers: scoped })).json()).data).toEqual(templates);
  expect((await (await page.request.get(historyURL, { headers: scoped })).json()).data).toEqual(history);
  expect(await readWallet()).toEqual(walletBefore);
  await page.locator('.brand-picker select').selectOption('');
  await expect(page.getByTestId('platform-template-detail')).toHaveCount(0);
  await expect(page.getByTestId('platform-template-list').locator('tbody tr')).toHaveCount(1);
});

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
  await page.locator('.brand-picker select').selectOption(brand);
  await page.getByTestId('platform-operations-tabs').getByRole('button', { name: 'Notifications', exact: true }).click();
  const panel = page.getByTestId('platform-notifications');
  await expect(page.getByTestId('platform-delivery-list').locator('tbody tr')).toHaveCount(50);
  await page.getByTestId('platform-delivery-pages').getByRole('button', { name: 'Next', exact: true }).click();
  await expect(page.getByTestId('platform-delivery-list').locator('tbody tr')).toHaveCount(1);
  await expect(panel).toContainText(rows[50].event_id);
  fail = true;
  await panel.getByRole('button', { name: 'Refresh', exact: true }).click();
  await expect(panel.getByRole('alert')).toContainText('Delivery unavailable');
  await expect(panel).not.toContainText(rows[50].event_id);
  await page.locator('.brand-picker select').selectOption('');
  await expect(panel.getByRole('alert')).toHaveCount(0);
  await expect(panel).not.toContainText(rows[0].event_id);
});
