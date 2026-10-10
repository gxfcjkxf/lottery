import { test, expect } from './platform-fixture';

const origin = 'http://127.0.0.1:5185';
const api = `${origin}/api/v1/platform`;

test('platform resumes and pauses an owned brand with audited original-request replay', async ({ page }, info) => {
  page.setDefaultTimeout(10_000);
  const me = (await (await page.request.get(`${api}/me`)).json()).data.account;
  expect(me.platform_permissions).toContain('brand_operation.write.platform');
  const code = `state_${crypto.randomUUID().replaceAll('-', '').slice(0, 12)}`;
  const created = await page.request.post(`${api}/brands`, {
    headers: { Origin: origin, 'Idempotency-Key': crypto.randomUUID() },
    data: { code, name: `Status trial ${code}`, default_locale: 'en', timezone: 'Asia/Manila', reason: 'Owned platform status trial' },
  });
  expect(created.status()).toBe(201);
  const brand = (await created.json()).data;
  await page.goto(origin);
  const row = page.getByTestId('platform-brand-list').locator('tr').filter({ hasText: code });
  await row.getByRole('button', { name: 'Manage status', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Brand operating status', exact: true });
  await expect(dialog).toContainText('Paused · v1');
  await expect(dialog.getByRole('button', { name: 'Confirm status change', exact: true })).toBeDisabled();
  await dialog.getByLabel('Reason', { exact: true }).fill('Enable this owned brand');
  await dialog.getByRole('button', { name: 'Confirm status change', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(row.locator('.status-pill')).toHaveText('Active');

  await row.getByRole('button', { name: 'Manage status', exact: true }).click();
  await expect(dialog).toContainText('Active · v2');
  const reason = 'Pause owned brand while existing orders continue';
  await dialog.getByLabel('Reason', { exact: true }).fill(reason);
  const attempts: Array<{ body: string | null; key: string | undefined; brand: string | undefined }> = [];
  let receipt: { version: number; audit_log_id: string } | undefined;
  await page.route('**/api/v1/platform/brand-operation', async route => {
    if (route.request().method() !== 'PATCH') { await route.continue(); return; }
    attempts.push({ body: route.request().postData(), key: route.request().headers()['idempotency-key'], brand: route.request().headers()['x-brand-id'] });
    const response = await route.fetch();
    expect(response.status()).toBe(200);
    const data = (await response.json()).data;
    if (attempts.length === 1) { receipt = data; await route.abort('failed'); }
    else { expect(data).toEqual(receipt); await route.fulfill({ response }); }
  });
  await dialog.getByRole('button', { name: 'Confirm status change', exact: true }).click();
  await expect(dialog.getByRole('button', { name: 'Retry the same request', exact: true })).toBeVisible();
  await expect(dialog).toContainText(reason);
  await dialog.getByRole('button', { name: 'Close', exact: true }).last().click();
  await expect(dialog).toHaveCount(0);
  await row.getByRole('button', { name: 'Manage status', exact: true }).click();
  await dialog.getByRole('button', { name: 'Retry the same request', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(row.locator('.status-pill')).toHaveText('Paused');
  expect(attempts).toHaveLength(2);
  expect(attempts[1]).toEqual(attempts[0]);
  expect(attempts[0].brand).toBe(brand.id);
  expect(receipt!.version).toBe(3);
  expect(receipt!.audit_log_id).toMatch(/^[0-9a-f-]{36}$/);
  const history = await page.request.get(`${api}/brand-operation/history`, { headers: { 'X-Brand-ID': brand.id } });
  expect(history.status()).toBe(200);
  const items = (await history.json()).data.items;
  expect(items).toHaveLength(2);
  expect(items[0]).toMatchObject({ status: 'paused', reason, audit_log_id: receipt!.audit_log_id, changed_by: me.id });
  expect(items[1]).toMatchObject({ status: 'active', reason: 'Enable this owned brand' });
  const forbidden = await page.request.post(`${api}/withdrawals/${crypto.randomUUID()}/approve`, {
    headers: { Origin: origin, 'X-Brand-ID': brand.id, 'Idempotency-Key': crypto.randomUUID() }, data: {},
  });
  expect(forbidden.status()).toBe(404);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: info.outputPath('platform-brand-status.png'), fullPage: true });
});
