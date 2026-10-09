import { test, expect, type Page, type APIResponse } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { rememberAdminSession } from './support/admin-session';

const origin = process.env.TEST_ADMIN_ORIGIN ?? 'http://localhost:5174';
const userOrigin = process.env.TEST_USER_ORIGIN ?? 'http://localhost:5173';
const admin = `${origin}/api/v1/admin`, brand = '0199a000-0000-7000-8000-000000000001';
const fixture = process.env.COMMISSION_FIXTURE_BIN;
type Execution = { id: string; version: number; state: string; applied_debit_points: string; applied_count: string; cycle_hold_active: boolean };
function command(name: 'prepare-corrections' | 'execute-corrections' | 'freeze-corrections' | 'unfreeze-corrections' | 'verify-corrections' | 'notify') {
  return JSON.parse(execFileSync(fixture!, [name], { env: process.env, encoding: 'utf8', timeout: 120_000 })) as Record<string, unknown>;
}
async function data<T>(response: Pick<APIResponse, 'text' | 'status'>): Promise<T> {
  const raw = await response.text(); expect(response.status(), raw).toBe(200);
  const envelope = JSON.parse(raw); expect(envelope.success).toBe(true); return envelope.data as T;
}
async function navigate(page: Page, project: string, label: RegExp) {
  if (project === 'mobile') { await page.locator('.mobile-nav button').nth(4).click(); await page.locator('.mobile-more-menu').getByRole('button', { name: label }).click(); }
  else await page.locator('.side-nav').getByRole('button', { name: label }).click();
}
async function confirm(page: Page) {
  const review = page.getByTestId('cc-review'); await expect(review).toBeVisible();
  await review.getByTestId('cc-confirm-check').check(); await review.getByTestId('cc-submit').click();
}

test('actual manual correction preserves lost approval receipts, holds until explicit continuation and reclaims only C', async ({ page, context }, info) => {
  test.skip(!fixture || process.env.COMMISSION_FIXTURE_CONFIRM !== 'owned_synthetic_database' || !process.env.TEST_COMMISSION_ADMIN_PASSWORD, 'Requires explicitly owned synthetic correction workflow');
  test.setTimeout(150_000);
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  const prepared = command('prepare-corrections');
  expect(prepared.brand_id).toBe(brand); expect(prepared.plan_id).toMatch(/^[0-9a-f-]{36}$/);
  const before = command('verify-corrections'); expect(before.correction_ledger_entries).toBe(0);
  expect(command('execute-corrections').processed).toBe(0);
  await data(await page.request.post(`${admin}/auth/login`, { headers: { Origin: origin, 'Idempotency-Key': crypto.randomUUID() }, data: { identifier: 'commission_admin', password: process.env.TEST_COMMISSION_ADMIN_PASSWORD } }));
  rememberAdminSession('commission_admin', await context.cookies(`${admin}/me`), origin);
  const get = async <T>(path: string) => data<T>(await page.request.get(`${admin}${path}`, { headers: { 'X-Brand-ID': brand } }));
  const policy = await get<{ enabled: boolean; version: number }>('/commission-correction-policy'); expect(policy.enabled).toBe(false);
  await page.goto(origin); await page.getByLabel(/选择真实后台品牌|Select an administrative brand/, { exact: true }).selectOption(brand);
  await page.getByTestId('admin-language').selectOption('zh-CN'); await navigate(page, info.project.name, /佣金更正|Commission corrections/);
  let panel = page.locator('.correction-admin'); await expect(panel.getByRole('heading', { name: '佣金更正差额', exact: true })).toBeVisible();
  await expect(panel.getByTestId('cc-policy-toggle')).not.toBeChecked();
  await panel.getByTestId('cc-policy-toggle').check(); await panel.getByTestId('cc-reason').fill('Enable only the explicitly owned synthetic financial correction gate');
  await panel.getByTestId('cc-policy-review').click(); const enabled = page.waitForResponse(r => r.request().method() === 'PUT' && r.url() === `${admin}/commission-correction-policy`);
  await confirm(page); await data(await enabled); await expect(panel.getByTestId('cc-receipt')).toContainText('policy');
  command('execute-corrections');
  const jobs = await get<{ items: Execution[] }>('/commission-correction-executions?limit=100&offset=0');
  const execution = jobs.items.find(item => item.state === 'awaiting_approval'); expect(execution).toBeTruthy();
  const id = execution!.id, path = `/commission-correction-executions/${id}`;
  await panel.getByTestId('cc-refresh').click();
  await panel.getByRole('button', { name: String(prepared.plan_id), exact: true }).click();
  await expect(panel.getByTestId('cc-plan-detail')).toBeVisible();
  // Compare against the configured viewport, not innerWidth: mobile overflow
  // can enlarge innerWidth and conceal both scaling and physical hit-target bugs.
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(page.viewportSize()!.width + 1);
  await panel.getByRole('button', { name: id, exact: true }).click();
  await expect(panel.getByTestId('cc-execution-detail')).toContainText('待新批准');
  await panel.getByTestId('cc-execution-reason').fill('Freshly approve the exact one-point correction difference');
  const writes: Array<{ body: string | null; key: string | undefined; actor: string | undefined }> = [];
  await page.route(`**${path}/approve`, async route => {
    writes.push({ body: route.request().postData(), key: route.request().headers()['idempotency-key'], actor: route.request().headers()['x-commission-correction-actor-id'] });
    expect(writes.at(-1)?.actor).toMatch(/^[0-9a-f-]{36}$/);
    const response = await route.fetch(); const receipt = await data<Execution>(response); expect(receipt.state).toBe('applying');
    if (writes.length === 1) { command('freeze-corrections'); command('execute-corrections'); await route.abort('failed'); }
    else { expect(writes[1]).toEqual(writes[0]); await route.fulfill({ response }); }
  });
  await panel.getByTestId('cc-approve').click(); await confirm(page);
  await expect(panel.getByTestId('cc-unknown')).toBeVisible(); expect((await get<Execution>(path)).state).toBe('paused');
  await panel.getByTestId('cc-refresh').click(); await expect(panel.getByTestId('cc-unknown')).toBeVisible();
  await expect(panel.getByTestId('cc-policy-review')).toBeDisabled();
  await navigate(page, info.project.name, /佣金和奖励|Commissions.*rewards/);
  await navigate(page, info.project.name, /佣金更正|Commission corrections/); panel = page.locator('.correction-admin');
  await panel.getByTestId('cc-unknown').getByRole('button', { name: '查看原请求并重试', exact: true }).click(); await confirm(page);
  await expect(panel.getByTestId('cc-receipt')).toContainText('"state":"applying"');
  await panel.getByRole('button', { name: id, exact: true }).click();
  await expect(panel.getByTestId('cc-execution-detail')).toContainText('暂停');
  expect(writes).toHaveLength(2); await page.unroute(`**${path}/approve`);
  const held = command('verify-corrections'); expect(held.cycle_hold_active).toBe(true); expect(held.correction_ledger_entries).toBe(0);
  command('unfreeze-corrections'); expect(command('execute-corrections').processed).toBe(0);
  const restored = command('verify-corrections'); expect(restored.state).toBe('paused'); expect(restored.cycle_hold_active).toBe(true);
  await panel.getByTestId('cc-execution-reason').fill('Explicitly continue the current cycle after the original C freeze is reversed');
  await panel.getByTestId('cc-continue').click(); const continued = page.waitForResponse(r => r.request().method() === 'POST' && r.url() === `${admin}${path}/continue`);
  await confirm(page); await data(await continued); command('execute-corrections');
  await panel.getByTestId('cc-refresh').click(); await panel.getByRole('button', { name: id, exact: true }).click();
  const current = await get<Execution>(path); expect(current.state).toBe('completed'); expect(current.applied_debit_points).toBe('1'); expect(current.applied_count).toBe('1'); expect(current.cycle_hold_active).toBe(false);
  await expect(panel.getByTestId('cc-execution-detail')).toContainText('已完成'); await expect(panel.getByTestId('cc-continue')).toBeDisabled();
  const complete = command('verify-corrections'); expect(complete.correction_ledger_entries).toBe(1);
  command('execute-corrections'); expect(command('verify-corrections').economic_fingerprint).toBe(complete.economic_fingerprint);
  await panel.getByRole('button', { name: String(prepared.plan_id), exact: true }).click();
  await expect(panel.getByTestId('cc-plan-detail')).toContainText('计划就绪');
  await page.getByTestId('admin-language').selectOption('en'); await expect(panel.getByRole('heading', { name: 'Commission correction', exact: true })).toBeVisible();
  await expect(panel.getByRole('status')).toContainText('The server confirmed the operation receipt.');
  await expect(panel.getByTestId('cc-plan-detail')).toContainText('Frozen before / calculated');
  await panel.getByRole('button', { name: id, exact: true }).click(); await expect(panel.getByTestId('cc-execution-detail')).toContainText('Completed');
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(page.viewportSize()!.width + 1);
  await panel.screenshot({ path: info.outputPath('commission-corrections-current.png') }); expect(errors).toEqual([]);
  expect(command('verify-corrections').economic_fingerprint).toBe(complete.economic_fingerprint);

  // The actual report counts the original credit plus this correction debit,
  // not a ready plan, approved amount, freeze/unfreeze or current wallet.
  await navigate(page, info.project.name, /报表和对账|Reports/);
  const reportPanel = page.locator('.commission-report');
  await expect(reportPanel.getByRole('heading', { name: 'Commission ledger report', exact: true })).toBeVisible();
  await reportPanel.getByLabel('Member UUID', { exact: true }).fill(String(prepared.beneficiary_id));
  await reportPanel.getByLabel('Cycle UUID', { exact: true }).fill(String(prepared.cycle_id));
  await reportPanel.getByTestId('commission-report-group').selectOption('cycle');
  const reportResponse = page.waitForResponse(r => r.request().method() === 'GET' && r.url().startsWith(`${admin}/reports/commission?`));
  await reportPanel.getByTestId('commission-report-query').click();
  const expected = { entry_count: '2', paid_entry_count: '1', paid_points: '1', adjustment_entry_count: '0', adjustment_credit_points: '0', adjustment_debit_points: '0', correction_entry_count: '1', correction_credit_points: '0', correction_debit_points: '1', net_points: '0' };
  const report = await data<{ summary: typeof expected; items: Array<{ key: string }>; total_groups: string }>(await reportResponse);
  expect(report.summary).toEqual(expected); expect(report.total_groups).toBe('1'); expect(report.items[0]?.key).toBe(prepared.cycle_id);
  await expect(reportPanel.getByTestId('commission-report-summary').locator('strong')).toHaveText(Object.values(expected));
  const csvResponse = page.waitForResponse(r => r.request().method() === 'GET' && r.url().startsWith(`${admin}/reports/commission.csv?`));
  const download = page.waitForEvent('download'); await reportPanel.getByTestId('commission-report-export').click();
  const csv = await csvResponse; expect(csv.status()).toBe(200); expect(csv.headers()['x-report-format-version']).toBe('2');
  expect(csv.headers()['x-report-group-count']).toBe('1'); expect(new URL(csv.url()).searchParams.has('limit')).toBe(false);
  const file = await download, pathOnDisk = await file.path(); expect(pathOnDisk).toBeTruthy(); const bytes = readFileSync(pathOnDisk!);
  expect(createHash('sha256').update(bytes).digest('hex')).toBe(csv.headers()['x-report-sha256']);
  expect(String(bytes.length)).toBe(csv.headers()['x-report-byte-count']); expect(bytes.subarray(0, 3)).toEqual(Buffer.from([0xef, 0xbb, 0xbf]));
  const rows = bytes.toString('utf8').replace(/^\uFEFF/, '').trim().split(/\r?\n/);
  expect(rows).toHaveLength(3); expect(rows[0]!.split(',')).toHaveLength(22);
  expect(rows[0]).toContain('correction_entry_count,correction_credit_points,correction_debit_points,net_points');
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(page.viewportSize()!.width + 1);
  await reportPanel.screenshot({ path: info.outputPath('commission-correction-report.png') });
  expect(command('verify-corrections').economic_fingerprint).toBe(complete.economic_fingerprint);

  // Materialize only committed nonzero postings through the real inbox
  // consumer. Later template edits cannot rewrite the original v1 snapshot.
  command('notify');
  const login = await data<{ member: { id: string } }>(await page.request.post(`${userOrigin}/api/v1/auth/login`, { headers: { Origin: userOrigin, 'Idempotency-Key': crypto.randomUUID() }, data: { identifier: 'commission_agent_owner', password: process.env.COMMISSION_FIXTURE_USER_PASSWORD } }));
  expect(login.member.id).toBe(prepared.beneficiary_id);
  const inbox = async () => data<{ items: Array<{ id: string; event_type: string; template_version: number; content: unknown; payload: Record<string, string> }> }>(await page.request.get(`${userOrigin}/api/v1/notifications?limit=100&offset=0`));
  const messages = (await inbox()).items.filter(item => item.event_type === 'commission.corrected'); expect(messages).toHaveLength(1);
  const message = messages[0]!; expect(message.template_version).toBe(1); expect(message.content).toBeTruthy();
  expect(message.payload).toEqual({ resource_id: complete.execution_target_id, points: '-1' });
  command('notify'); expect((await inbox()).items.filter(item => item.event_type === 'commission.corrected')).toEqual(messages);
  const templates = await get<{ items: Array<{ key: string; version: number; content: unknown }> }>('/notification-templates'); expect(templates.items).toHaveLength(22);
  expect(templates.items.filter(item => item.key.startsWith('draw.result.')).map(item => item.key).sort()).toEqual(['draw.result.corrected','draw.result.published']);
  const template = templates.items.find(item => item.key === 'commission.corrected')!; expect(template.version).toBe(1);
  await data(await page.request.put(`${admin}/notification-templates/commission.corrected`, { headers: { Origin: origin, 'X-Brand-ID': brand, 'Idempotency-Key': crypto.randomUUID() }, data: { version: template.version, content: { en: { title: 'New correction copy', body: 'Later template copy: {points} points.' }, 'zh-CN': { title: '新更正文案', body: '后续模板文案：{points} 积分。' } }, reason: 'Owned synthetic edit proves earlier correction snapshot remains immutable' } }));
  expect((await inbox()).items.find(item => item.id === message.id)).toEqual(message);
  const userPage = await context.newPage(); userPage.on('pageerror', error => errors.push(error.message));
  await userPage.goto(`${userOrigin}/notifications`);
  if (await userPage.getByRole('button', { name: 'Switch to English', exact: true }).count()) await userPage.getByRole('button', { name: 'Switch to English', exact: true }).click();
  const inboxPanel = userPage.locator('.notifications-panel');
  await expect(inboxPanel.getByRole('heading', { name: 'Commission correction recorded', exact: true })).toBeVisible();
  await expect(inboxPanel).not.toContainText('New correction copy');
  await expect(inboxPanel.locator('.notification-protected-note').filter({ hasText: 'negative points record a past recovery' })).toHaveCount(1);
  await expect.poll(() => userPage.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(userPage.viewportSize()!.width + 1);
  await inboxPanel.screenshot({ path: info.outputPath('commission-correction-inbox-en.png') });
  await userPage.getByRole('button', { name: 'Switch to Chinese', exact: true }).click();
  await expect(inboxPanel.getByRole('heading', { name: '佣金更正记录', exact: true })).toBeVisible();
  await expect(inboxPanel.locator('.notification-protected-note').filter({ hasText: '负数表示过去的追回' })).toHaveCount(1);
  await inboxPanel.screenshot({ path: info.outputPath('commission-correction-inbox-zh.png') });
  expect(command('verify-corrections').economic_fingerprint).toBe(complete.economic_fingerprint); expect(errors).toEqual([]);
  await userPage.close();
});
