import { test, expect, type APIResponse, type Page } from '@playwright/test';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { rememberAdminSession } from './support/admin-session';

const origin = process.env.TEST_ADMIN_ORIGIN ?? 'http://localhost:5174';
const admin = `${origin}/api/v1/admin`, brand = '0199a000-0000-7000-8000-000000000001';
type Order = { id: string; state: string; version: number; revoke_ledger_entry_id: string | null };
type Entry = { id: string; created_at: string; entry_type: string; reference_id: string };
type Report = { summary: Record<string, string>; total_groups: string; query: Record<string, string | number | null>; items: Array<{ key: string; totals: Record<string, string> }> };
async function data<T>(response: Pick<APIResponse, 'text' | 'status'>, status = 200): Promise<T> {
  const body = await response.text(); expect(response.status(), body).toBe(status);
  const envelope = JSON.parse(body); expect(envelope.success).toBe(true); return envelope.data as T;
}
async function navigate(page: Page, project: string, destination: string) {
  if (project === 'mobile' && destination === '工作台|Dashboard') await page.locator('.mobile-nav').getByRole('button', { name: new RegExp(destination) }).click();
  else if (project === 'mobile') { await page.locator('.mobile-nav button').nth(4).click(); await page.locator('.mobile-more-menu').getByRole('button', { name: new RegExp(destination) }).click(); }
  else await page.locator('.side-nav').getByRole('button', { name: new RegExp(destination) }).click();
}

test('real reward reports separate immutable postings from current order cohorts and export complete audited CSV without financial writes', async ({ page, context }, info) => {
  test.skip(process.env.REWARD_UI_FIXTURE_CONFIRM !== 'owned_synthetic_database' || !process.env.TEST_REWARD_ADMIN_PASSWORD, 'Requires an explicitly owned reward test database');
  test.setTimeout(90_000); expect(process.env.REWARD_UI_VIEWPORT).toBe(info.project.name);
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  const get = async<T>(path: string) => data<T>(await page.request.get(admin + path, { headers: { 'X-Brand-ID': brand } }));
  const post = async<T>(path: string, body: unknown, extra: Record<string, string> = {}, status = 200) => data<T>(await page.request.post(admin + path, { headers: { Origin: origin, 'X-Brand-ID': brand, 'Idempotency-Key': crypto.randomUUID(), ...extra }, data: body }), status);
  await post('/auth/login', { identifier: 'reward_s22_operator', password: process.env.TEST_REWARD_ADMIN_PASSWORD });
  const { account } = await get<{ account: { id: string; permissions_by_brand: Record<string, string[]> } }>('/me');
  expect(account.permissions_by_brand[brand]).toEqual(expect.arrayContaining(['report_reward.view.brand', 'report_reward.export.brand', 'reward.view.brand']));
  rememberAdminSession('reward_s22_operator', await context.cookies(`${admin}/me`), origin, account.id);
  const member = await post<{ member_id: string }>('/users', { username: `rr_${info.project.name}_${crypto.randomUUID().replaceAll('-', '').slice(0, 8)}`, password: 'owned-report-member-password-2026', display_name: 'Owned reward report member', reason: 'Create explicitly owned reward report browser member' }, {}, 201);
  const memberID = member.member_id, actor = { 'X-Reward-Actor-ID': account.id };
  const grant = (points: string) => post<Order>('/reward-orders', { member_id: memberID, points, reason: 'Owned synthetic report grant' }, actor, 201);
  await grant('10'); const reversed = await grant('20');
  const revoked = await post<Order>(`/reward-orders/${reversed.id}/revoke`, { version: 1, reason: 'Owned report full reversal' }, actor); expect(revoked.state).toBe('revoked');
  const pending = await grant('30');
  await post(`/wallets/${memberID}/freeze`, { points: '40', reason: 'Owned report gift hold for pending reversal' });
  expect((await post<Order>(`/reward-orders/${pending.id}/revoke`, { version: 1, reason: 'Owned report insufficient available gift' }, actor)).state).toBe('revocation_pending');
  const ledger = () => get<{ items: Entry[] }>(`/wallets/${memberID}/ledger?limit=100&offset=0`);
  const economic = async () => JSON.stringify({ wallet: await get(`/wallets/${memberID}`), ledger: await ledger(), orders: await get('/reward-orders?limit=100&offset=0') });
  const before = await economic(), writes: string[] = [];
  page.on('request', r => { if (r.url().startsWith(admin) && r.method() !== 'GET') writes.push(r.url()); });
  await page.goto(origin); await page.getByLabel(/选择真实后台品牌|Select live admin brand/, { exact: true }).selectOption(brand);
  await navigate(page, info.project.name, '报表和对账|Reports'); const panel = page.locator('.reward-reports');
  await expect(panel.getByRole('heading', { name: '奖励报表', exact: true })).toBeVisible();
  await panel.getByLabel('会员 UUID（可选）', { exact: true }).fill(memberID);
  await panel.getByTestId('reward-report-group').selectOption('order');
  const read = async (path: string) => { const response = page.waitForResponse(r => r.request().method() === 'GET' && r.url().startsWith(`${admin}/reports/${path}?`)); await panel.getByTestId('reward-report-query').click(); return data<Report>(await response); };
  const posting = await read('rewards');
  expect(posting.summary).toEqual({ entry_count: '4', grant_entry_count: '3', grant_points: '60', reversal_entry_count: '1', reversal_points: '20', net_points: '40' }); expect(posting.total_groups).toBe('3');
  await expect(panel.getByTestId('reward-report-summary').locator('strong')).toHaveText(['4', '3', '60', '1', '20', '40']);
  // Export is bound to the submitted filter, not the deliberately wrong draft.
  await panel.getByLabel('会员 UUID（可选）', { exact: true }).fill('ffffffff-ffff-4fff-8fff-ffffffffffff');
  async function exportFile(path: string, kind: string, groupCount: string, fields: string) {
    const response = page.waitForResponse(r => r.request().method() === 'GET' && r.url().startsWith(`${admin}/reports/${path}.csv?`)); const download = page.waitForEvent('download');
    await panel.getByTestId('reward-report-export').click(); const csv = await response; expect(csv.status(), await csv.text()).toBe(200);
    expect(new URL(csv.url()).searchParams.get('member_id')).toBe(memberID); expect(new URL(csv.url()).searchParams.has('limit')).toBe(false);
    const file = await download, filePath = await file.path(); expect(filePath).toBeTruthy(); const bytes = readFileSync(filePath!);
    expect(bytes.subarray(0, 3)).toEqual(Buffer.from([0xef, 0xbb, 0xbf]));
    expect(createHash('sha256').update(bytes).digest('hex')).toBe(csv.headers()['x-report-sha256']); expect(String(bytes.length)).toBe(csv.headers()['x-report-byte-count']);
    expect(csv.headers()['x-report-kind']).toBe(kind); expect(csv.headers()['x-report-member-id']).toBe(memberID); expect(csv.headers()['x-report-group-count']).toBe(groupCount);
    expect(csv.headers()['x-report-audit-id']).toMatch(/^[0-9a-f-]{36}$/); expect(file.suggestedFilename()).toMatch(new RegExp(`^lottery-${kind}-${brand}-\\d{8}T\\d{6}Z\\.csv$`));
    const text = bytes.toString('utf8'); expect(text).toContain(`record_type,brand_id,snapshot_at,timezone,from,to,group_by,member_id,order_id,key,label,${fields}`);
    expect(text.split('\n').filter(line => line.startsWith('summary,'))).toHaveLength(1); expect(text.split('\n').filter(line => line.startsWith('group,'))).toHaveLength(Number(groupCount));
  }
  await exportFile('rewards', 'rewards', '3', 'entry_count,grant_entry_count,grant_points,reversal_entry_count,reversal_points,net_points');
  await panel.getByTestId('reward-report-mode-orders').click(); await expect(panel.getByTestId('reward-report-summary')).toHaveCount(0);
  await panel.getByLabel('会员 UUID（可选）', { exact: true }).fill(memberID); await panel.getByTestId('reward-report-group').selectOption('state');
  const orders = await read('reward-orders');
  expect(orders.summary).toEqual({ order_count: '3', original_points: '60', granted_count: '1', granted_points: '10', pending_count: '1', pending_points: '30', revoked_count: '1', revoked_points: '20' });
  expect(orders.items.map(item => item.key)).toEqual(['granted', 'revocation_pending', 'revoked']);
  await expect(panel.getByTestId('reward-report-summary').locator('strong')).toHaveText(['3', '60', '1', '10', '1', '30', '1', '20']);
  await exportFile('reward-orders', 'reward_orders', '3', 'order_count,original_points,granted_count,granted_points,pending_count,pending_points,revoked_count,revoked_points');
  const actualReversal = (await ledger()).items.find(entry => entry.id === revoked.revoke_ledger_entry_id)!; expect(actualReversal).toBeTruthy();
  const narrow = new URLSearchParams({ from: actualReversal.created_at, to: new Date(Date.now() + 60_000).toISOString(), group_by: 'order', order_id: reversed.id });
  const negative = await get<Report>(`/reports/rewards?${narrow}`); expect(negative.summary.net_points).toBe('-20'); expect(negative.summary.grant_points).toBe('0');
  const negativeCSV = await page.request.get(`${admin}/reports/rewards.csv?${narrow}`, { headers: { 'X-Brand-ID': brand } }); expect(negativeCSV.status()).toBe(200); expect(await negativeCSV.text()).toContain(",'\u002d20");
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true); await panel.screenshot({ path: info.outputPath('reward-reports.png') });
  await page.getByTestId('admin-language').selectOption('en'); await expect(panel.getByRole('heading', { name: 'Reward reports', exact: true })).toBeVisible();
  const snapshot = await get<{ rewards: { status: string; data: { granted_count: string; pending_count: string; revoked_count: string } } }>('/workbench');
  await navigate(page, info.project.name, '工作台|Dashboard'); const card = page.locator('.workbench .card').filter({ has: page.getByRole('heading', { name: 'Reward orders (current state)', exact: true }) });
  expect(snapshot.rewards.status).toBe('ready'); await expect(card.locator('dd')).toHaveText(Object.values(snapshot.rewards.data));
  await card.getByRole('button', { name: 'Open management page', exact: true }).click(); await expect(page.locator('.rewards-management')).toBeVisible();
  expect(await economic()).toBe(before); expect(writes).toEqual([]); expect(errors).toEqual([]);
});
