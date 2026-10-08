import { spawnSync } from 'node:child_process';
import { expect, test, type APIResponse, type Page } from '@playwright/test';

const origin = process.env.TEST_ARCHIVE_TASKS_ORIGIN;
const apiOrigin = process.env.TEST_ARCHIVE_TASKS_API_ORIGIN;
const brand = '0199a000-0000-7000-8000-000000000002';
const fixtureBin = process.env.ARCHIVE_TASKS_FIXTURE_BIN;
const viewport = process.env.REPORT_ARCHIVE_TASKS_VIEWPORT;
const username = process.env.TEST_ARCHIVE_TASKS_ADMIN_USERNAME;
const password = process.env.TEST_ARCHIVE_TASKS_ADMIN_PASSWORD;
const ack = 'owned_synthetic_database';
type Task = { id: string; brand_id: string; state: string; version: number; attempt_count: number; archive_id: string | null; policy_version: number };

async function data<T>(response: Pick<APIResponse, 'text' | 'status'>, status = 200): Promise<T> {
  const body = await response.text();
  expect(response.status(), body).toBe(status);
  const envelope = JSON.parse(body);
  expect(envelope.success).toBe(true);
  return envelope.data as T;
}
async function navigate(page: Page, project: string, name: string) {
  if (project === 'mobile' && name === '工作台|Dashboard') {
    await page.locator('.mobile-nav').getByRole('button', { name: new RegExp(name) }).click();
  } else if (project === 'mobile') {
    await page.locator('.mobile-nav button').nth(4).click();
    await page.locator('.mobile-more-menu').getByRole('button', { name: new RegExp(name) }).click();
  } else {
    await page.locator('.side-nav').getByRole('button', { name: new RegExp(name) }).click();
  }
}
function fixture(command: 'advance' | 'verify') {
  expect(fixtureBin).toBeTruthy();
  const env = { ...process.env };
  delete env.TEST_ARCHIVE_TASKS_ADMIN_PASSWORD;
  delete env.BOOTSTRAP_ADMIN_PASSWORD;
  const result = spawnSync(fixtureBin!, [command], { env, encoding: 'utf8', timeout: 30_000, maxBuffer: 64_000 });
  expect(result.error, 'Owned fixture must execute without exposing child diagnostics').toBeUndefined();
  expect(result.status, 'Owned fixture command must succeed').toBe(0);
  return JSON.parse(result.stdout) as { task_id: string; state: string; archive_id: string | null; archive_count: string; ledger_count: string; total_points: string };
}

test('real automatic archive tasks recover the original retry receipt after worker completion', async ({ page, context }, info) => {
  test.setTimeout(60_000);
  page.setDefaultTimeout(10_000);
  expect(process.env.REPORT_ARCHIVE_TASKS_FIXTURE_CONFIRM).toBe(ack);
  expect(viewport).toBe(info.project.name);
  expect(/^http:\/\/localhost:(?:5174|15292)$/.test(origin ?? '')).toBe(true);
  expect(/^http:\/\/127\.0\.0\.1:(?:8080|15291)$/.test(apiOrigin ?? '')).toBe(true);
  expect(username).toBe(`archive_tasks_browser_${info.project.name}`);
  expect(password).toBeTruthy();
  const browserAdmin = `${origin}/api/v1/admin`;
  const headers = { 'X-Brand-ID': brand };
  const get = <T>(path: string) => page.request.get(`${browserAdmin}${path}`, { headers }).then(r => data<T>(r));
  const post = <T>(path: string, body: unknown, status = 200) => page.request.post(`${browserAdmin}${path}`, {
    headers: { Origin: origin!, ...headers, 'Idempotency-Key': crypto.randomUUID() }, data: body,
  }).then(r => data<T>(r, status));
  const errors: string[] = [];
  page.on('pageerror', problem => errors.push(problem.message));
  await data(await page.request.post(`${browserAdmin}/auth/login`, {
    headers: { Origin: origin!, 'Idempotency-Key': crypto.randomUUID() }, data: { identifier: username, password },
  }));
  expect((await context.cookies(`${browserAdmin}/me`)).some(c => c.name === 'lottery_admin')).toBe(true);
  const me = await get<{ account: { id: string; permissions_by_brand: Record<string, string[]> } }>('/me');
  expect(me.account.permissions_by_brand[brand]).toEqual(expect.arrayContaining(['report_archive.view.brand', 'report_archive_task.retry.brand']));
  const initial = await get<{ items: Task[]; total_count: string }>('/report-archive-tasks?limit=100&offset=0');
  expect(initial.total_count).toBe('1');
  expect(initial.items).toHaveLength(1);
  const original = initial.items[0]!;
  expect(original).toMatchObject({ state: 'failed', version: 2, attempt_count: 1, archive_id: null });
  expect((await get<{ items: unknown[] }>('/report-archives?limit=100&offset=0')).items).toEqual([]);
  const member = await post<{ member_id: string }>('/users', {
    username: `archive_task_${crypto.randomUUID().replaceAll('-', '').slice(0, 10)}`, password: 'owned-archive-task-member-password-2026',
    display_name: 'Owned archive task member', reason: 'Create real member for automatic archive browser verification',
  }, 201);
  const recharge = await post<{ id: string; version: number }>('/recharges', {
    member_id: member.member_id, points: '37', proof_reference: 'Owned offline archive task funding',
    remark: 'Synthetic browser funding', reason: 'Create real 37 point recharge for archive task verification',
  }, 201);
  await post(`/recharges/${recharge.id}/confirm`, { version: recharge.version, reason: 'Confirm actual offline funding' });
  const economics = async () => JSON.stringify({
    wallet: await get(`/wallets/${member.member_id}`), ledger: await get(`/wallets/${member.member_id}/ledger?limit=100&offset=0`),
  });
  const before = await economics();
  const requests: Array<{ body: string | null; key: string | undefined; actor: string | undefined }> = [];
  let firstReceipt: Task | undefined;
  await page.route(`**/api/v1/admin/report-archive-tasks/${original.id}/retry`, async route => {
    requests.push({ body: route.request().postData(), key: route.request().headers()['idempotency-key'], actor: route.request().headers()['x-report-archive-actor-id'] });
    const response = await route.fetch();
    const receipt = await data<Task>(response);
    if (requests.length === 1) { firstReceipt = receipt; await route.abort('failed'); }
    else { expect(receipt).toEqual(firstReceipt); await route.fulfill({ response }); }
  });
  await page.addInitScript(() => localStorage.setItem('lottery.admin.locale', 'zh-CN'));
  await page.goto(origin!);
  await page.getByLabel(/选择真实后台品牌|Select live admin brand/, { exact: true }).selectOption(brand);
  await navigate(page, info.project.name, '自动归档任务|Automatic archive tasks');
  let panel = page.locator('.archive-tasks');
  await expect(panel.getByTestId('archive-tasks-policy')).toBeVisible();
  await panel.locator('.task-list').getByRole('button', { name: new RegExp(original.id) }).click();
  await panel.getByTestId('archive-tasks-reason').fill('Investigated synthetic capture failure; explicitly retry original task');
  await panel.getByTestId('archive-tasks-retry').click();
  await expect(panel.getByTestId('archive-tasks-review')).toBeVisible();
  await panel.getByTestId('archive-tasks-confirm').check();
  await panel.getByTestId('archive-tasks-submit').click();
  // The frozen intent is shown while the request is still in flight. Wait for
  // the real transport failure, not that provisional heading.
  await expect(panel.getByRole('alert').filter({ hasText: /重试结果未知。原账号|Retry outcome is unknown/ })).toBeVisible();
  expect(firstReceipt).toMatchObject({ id: original.id, state: 'pending', version: 3, attempt_count: 1, archive_id: null });
  expect(requests).toHaveLength(1);
  await expect(panel.getByTestId('archive-tasks-replay')).toBeDisabled();
  await panel.getByTestId('archive-tasks-refresh').click();
  expect(requests).toHaveLength(1);
  await navigate(page, info.project.name, '工作台|Dashboard');
  const advanced = fixture('advance');
  expect(advanced).toMatchObject({ task_id: original.id, state: 'completed', archive_count: '1', ledger_count: '1', total_points: '37' });
  expect(await economics()).toBe(before);
  await navigate(page, info.project.name, '自动归档任务|Automatic archive tasks');
  panel = page.locator('.archive-tasks');
  await expect(panel.getByTestId('archive-tasks-frozen')).toContainText(/重试结果未知|Retry outcome unknown/);
  expect(requests).toHaveLength(1);
  await panel.locator('.task-list').getByRole('button', { name: new RegExp(original.id) }).click();
  await expect(panel.getByTestId('archive-tasks-detail')).toContainText(/completed|已完成/);
  await panel.getByTestId('archive-tasks-replay-check').check();
  await panel.getByTestId('archive-tasks-replay').click();
  await expect(panel.getByTestId('archive-tasks-receipt')).toContainText('pending');
  expect(requests).toHaveLength(2);
  expect(requests[1]).toEqual(requests[0]);
  expect(requests[0]!.actor).toBe(me.account.id);
  await expect(panel.getByTestId('archive-tasks-detail')).toContainText(/completed|已完成/);
  expect(await economics()).toBe(before);
  const final = fixture('verify');
  expect(final).toMatchObject({ task_id: original.id, state: 'completed', archive_count: '1', ledger_count: '1', total_points: '37' });
  const archive = await get<{ created_by: null; automation: { task_id: string }; snapshot: { wallet_snapshot: { balances: { available_points: string } } } }>(`/report-archives/${final.archive_id}`);
  expect(archive.created_by).toBeNull();
  expect(archive.automation.task_id).toBe(original.id);
  expect(archive.snapshot.wallet_snapshot.balances.available_points).toBe('37');
  await page.getByTestId('admin-language').selectOption('en');
  await expect(panel.getByRole('heading', { name: 'Automatic archive tasks', exact: true })).toBeVisible();
  expect(requests).toHaveLength(2);
  const width = await page.evaluate(() => ({ body: document.documentElement.scrollWidth, viewport: innerWidth }));
  expect(width.body).toBeLessThanOrEqual(width.viewport);
  expect(errors).toEqual([]);
  await page.screenshot({ path: `.local/report-archive-tasks-${info.project.name}.png`, fullPage: true });
});
