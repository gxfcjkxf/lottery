import { createHash } from 'node:crypto';
import { mkdirSync } from 'node:fs';
import { expect, test, request, type APIResponse } from '@playwright/test';

const brand = '0199a000-0000-7000-8000-000000000002';
const other = '0199a000-0000-7000-8000-000000000001';
const viewport = process.env.REPORT_ATTRIBUTION_VIEWPORT;
const origin = process.env.TEST_AUDIT_ADMIN_ORIGIN;
const username = process.env.TEST_ATTRIBUTION_ADMIN_USERNAME;
const password = process.env.TEST_ATTRIBUTION_ADMIN_PASSWORD;
const id = () => crypto.randomUUID();
const suffix = () => id().replaceAll('-', '').slice(0, 10);

async function data<T>(response: Pick<APIResponse, 'text' | 'status'>, status = 200): Promise<T> {
  const body = await response.text();
  expect(response.status(), body).toBe(status);
  const envelope = JSON.parse(body) as { success: boolean; data: T };
  expect(envelope.success, body).toBe(true);
  return envelope.data;
}

test('genuine brand audit filters and validated full CSV preserve finances on both layouts', async ({ page }, info) => {
  if (!['desktop', 'mobile'].includes(viewport ?? '') || info.project.name !== viewport || process.env.APP_ENV !== 'test' || process.env.REPORT_ATTRIBUTION_FIXTURE_CONFIRM !== 'owned_synthetic_database' || username !== `attribution_browser_${viewport}` || !password || !/^http:\/\/localhost:(?:5174|15292)$/.test(origin ?? '') || !new RegExp(`^postgres(?:ql)?:\\/\\/lottery_test@127\\.0\\.0\\.1:(?:5432|55432)\\/lottery_attribution_browser_${viewport}\\?sslmode=disable$`).test(process.env.DATABASE_URL ?? '') || process.env.DATABASE_READ_URL || process.env.DATABASE_READ_URLS) {
    throw new Error('Audit browser tests require the exact owned, loopback, isolated desktop/mobile database and administrator');
  }
  const base = `${origin}/api/v1/admin`;
  const login = await data<{ access_token: string }>(await page.request.post(`${base}/auth/login`, {
    headers: { Origin: origin!, 'X-Brand-ID': brand, 'Idempotency-Key': id() },
    data: { identifier: username, password },
  }));
  const headers = { Authorization: `Bearer ${login.access_token}`, Origin: origin!, 'X-Brand-ID': brand };
  const get = async <T>(path: string) => data<T>(await page.request.get(`${base}${path}`, { headers }));
  const post = async <T>(path: string, body: unknown, status = 200) => data<T>(await page.request.post(`${base}${path}`, { headers: { ...headers, 'Idempotency-Key': id() }, data: body }), status);
  const member = await post<{ member_id: string }>('/users', { username: `audit_member_${suffix()}`, password: `Audit-${suffix()}-Member9`, reason: 'create an owned real audit member' }, 201);
  const recharge = await post<{ id: string; version: number }>('/recharges', { member_id: member.member_id, points: '7', proof_reference: `audit-${suffix()}`, remark: 'owned synthetic audit export funding', reason: 'create synthetic manual deposit' }, 201);
  const reason = '=HYPERLINK("https://invalid.example","owned synthetic formula guard")';
  await post(`/recharges/${recharge.id}/confirm`, { version: recharge.version, reason });
  const economicState = async () => ({ wallet: await get(`/wallets/${member.member_id}`), ledger: await get(`/wallets/${member.member_id}/ledger?limit=100&offset=0`) });
  const before = await economicState();
  expect((before.wallet as { available_points: string }).available_points).toBe('7');
  const range = { from: new Date(Date.now() - 120_000).toISOString().slice(0, 19), to: new Date(Date.now() + 300_000).toISOString().slice(0, 19) };
  const query = new URLSearchParams({ from: `${range.from}Z`, to: `${range.to}Z`, resource_id: recharge.id });
  const auditFacts = await get<{ items: Array<{ action: string; resource_id: string; before_json: unknown; after_json: unknown }> }>(`/audit?${query}`);
  expect(auditFacts.items.length).toBeGreaterThanOrEqual(2);
  expect(auditFacts.items.every(item => item.resource_id === recharge.id)).toBe(true);
  // A separately provisioned viewer must not obtain CSV merely through viewing.
  const role = await post<{ id: string }>('/roles', { code: `audit_reader_${suffix()}`, name: 'Owned audit reader', status: 'active', permissions: ['audit.view.brand', 'brand.view.brand'], reason: 'verify independent export permission' }, 201);
  const readerName = `audit_reader_${suffix()}`, readerPassword = `Audit-${suffix()}-Reader9`;
  await post('/accounts', { username: readerName, password: readerPassword, role_ids: [role.id], reason: 'create owned audit view-only account' }, 201);
  const readerContext = await request.newContext();
  try {
    const reader = await data<{ access_token: string }>(await readerContext.post(`${base}/auth/login`, { headers: { Origin: origin!, 'X-Brand-ID': brand, 'Idempotency-Key': id() }, data: { identifier: readerName, password: readerPassword } }));
    const denied = await readerContext.get(`${base}/audit/export?${query}`, { headers: { Origin: origin!, 'X-Brand-ID': brand, Authorization: `Bearer ${reader.access_token}` } });
    expect(denied.status()).toBe(403);
    expect(denied.headers()['content-disposition']).toBeUndefined();
    const crossBrand = await page.request.get(`${base}/audit/export?${query}`, { headers: { ...headers, 'X-Brand-ID': other } });
    expect(crossBrand.status()).toBe(403);
    expect(crossBrand.headers()['content-disposition']).toBeUndefined();
  } finally { await readerContext.dispose(); }

  const pageErrors: string[] = [], mutations: string[] = [];
  page.on('pageerror', error => pageErrors.push(error.message));
  page.on('request', req => { if (new URL(req.url()).pathname.startsWith('/api/v1/admin/') && req.method() !== 'GET') mutations.push(`${req.method()} ${new URL(req.url()).pathname}`); });
  await page.addInitScript(() => localStorage.setItem('lottery.admin.locale', 'en'));
  await page.goto(origin!);
  await page.getByLabel('Select live admin brand', { exact: true }).selectOption(brand);
  if (info.project.name === 'mobile') {
    await page.locator('.mobile-nav').getByRole('button', { name: /More/ }).click();
    await page.locator('.mobile-more-menu').getByRole('button', { name: /Audit log/ }).click();
  } else { await page.locator('.side-nav').getByRole('button', { name: /Audit log/ }).click(); }
  const panel = page.locator('.audit-page');
  await expect(panel.getByRole('heading', { name: 'Audit log', exact: true })).toBeVisible();
  await panel.getByLabel('From (UTC)', { exact: true }).fill(range.from);
  await panel.getByLabel('To (UTC)', { exact: true }).fill(range.to);
  await panel.getByLabel('Resource UUID', { exact: true }).fill(recharge.id);
  const readPromise = page.waitForResponse(res => new URL(res.url()).pathname === '/api/v1/admin/audit');
  await panel.getByRole('button', { name: 'Query audit records', exact: true }).click();
  const read = await readPromise;
  expect(read.status()).toBe(200);
  expect(new URL(read.url()).searchParams.get('from')).toBe(new Date(`${range.from}Z`).toISOString());
  await expect(panel.locator('.audit-record')).toHaveCount(auditFacts.items.length);
  await panel.locator('.audit-record').first().getByText('View redacted before and after snapshots', { exact: true }).click();
  await expect(panel.locator('.audit-snapshots').first()).toBeVisible();
  const exportResponsePromise = page.waitForResponse(res => new URL(res.url()).pathname === '/api/v1/admin/audit/export');
  const downloadPromise = page.waitForEvent('download');
  await panel.getByRole('button', { name: 'Export full CSV', exact: true }).click();
  const [exportResponse, download] = await Promise.all([exportResponsePromise, downloadPromise]);
  expect(exportResponse.status()).toBe(200);
  expect(download.suggestedFilename()).toBe(`audit-${brand}-v1.csv`);
  const stream = await download.createReadStream();
  expect(stream).toBeTruthy();
  const chunks: Buffer[] = []; for await (const chunk of stream!) chunks.push(Buffer.from(chunk));
  const body = Buffer.concat(chunks), exportHeaders = exportResponse.headers();
  expect(createHash('sha256').update(body).digest('hex')).toBe(exportHeaders['x-audit-sha256']);
  expect(exportHeaders['x-audit-row-count']).toBe(String(auditFacts.items.length));
  expect(body.toString('utf8')).toContain(`"'=HYPERLINK(`);
  expect(body.toString('utf8')).toContain(recharge.id);
  expect(body.toString('utf8')).not.toContain(readerPassword);
  const proof = await get<{ items: Array<{ id: string; action: string; after_json: { sha256: string } }> }>(`/audit?action=audit.export`);
  expect(proof.items.find(item => item.id === exportHeaders['x-audit-export-id'])).toMatchObject({ action: 'audit.export', after_json: { sha256: exportHeaders['x-audit-sha256'] } });
  await expect(panel.locator('.audit-receipt')).toBeVisible();
  await page.getByTestId('admin-language').selectOption('zh-CN');
  await expect(panel.getByRole('heading', { name: '审计日志', exact: true })).toBeVisible();
  await expect(panel.locator('.audit-receipt')).toHaveCount(0);
  await expect(panel.getByRole('button', { name: '导出完整 CSV', exact: true })).toBeDisabled();
  await panel.getByRole('button', { name: '查询审计记录', exact: true }).click();
  await expect(panel.locator('.audit-record')).toHaveCount(auditFacts.items.length);
  expect(await economicState()).toEqual(before);
  expect(mutations).toEqual([]);
  expect(pageErrors).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  mkdirSync('.local', { recursive: true });
  await page.screenshot({ path: `.local/audit-exports-${info.project.name}.png`, fullPage: true });
});
