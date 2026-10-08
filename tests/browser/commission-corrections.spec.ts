import { test, expect, type Page, type APIResponse } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { rememberAdminSession } from './support/admin-session';

const origin = process.env.TEST_ADMIN_ORIGIN ?? 'http://localhost:5174';
const admin = `${origin}/api/v1/admin`, brand = '0199a000-0000-7000-8000-000000000001';
const fixture = process.env.COMMISSION_FIXTURE_BIN;
type Execution = { id: string; version: number; state: string; applied_debit_points: string; applied_count: string; cycle_hold_active: boolean };
function command(name: 'prepare-corrections' | 'execute-corrections' | 'freeze-corrections' | 'unfreeze-corrections' | 'verify-corrections') {
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
});
