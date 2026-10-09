import { test, expect } from './platform-fixture';

const origin = 'http://127.0.0.1:5185';
const brand = '0199a000-0000-7000-8000-000000000001';
const headers = { 'X-Brand-ID': brand };
const kinds = [['points', 'point-policy'], ['withdrawal', 'withdrawal-policy'], ['commission', 'commission-policy']] as const;

async function operations(page: any, tab: string) {
  await page.goto(origin);
  await page.getByRole('button', { name: 'Audit and operations', exact: true }).click();
  await page.locator('.content > .brand-picker select').selectOption(brand);
  await page.getByTestId('platform-operations-tabs').getByRole('button', { name: tab, exact: true }).click();
}

test('platform reads genuine archive and financial policies without enabling or changing them', async ({ page }, info) => {
  const before = new Map<string, unknown>();
  for (const path of ['report-archive-policy', 'report-archive-tasks?limit=50&offset=0', ...kinds.map(([, path]) => path)]) {
    const response = await page.request.get(`${origin}/api/v1/platform/${path}`, { headers });
    expect(response.status(), await response.text()).toBe(200);
    before.set(path, (await response.json()).data);
  }
  await operations(page, 'Archive tasks');
  await expect(page.getByTestId('platform-archive-policy')).toContainText('Policy version');
  await expect(page.getByTestId('platform-archive-tasks')).toContainText('Reading this page does not enable');
  await expect(page.getByTestId('platform-archive-tasks').getByRole('alert')).toHaveCount(0);
  const policyBefore = before.get('report-archive-policy') as any;
  await expect(page.getByTestId('platform-archive-policy')).toContainText(policyBefore.timezone);
  await page.getByTestId('platform-operations-tabs').getByRole('button', { name: 'Financial policies', exact: true }).click();
  const panel = page.getByTestId('platform-financial-policies');
  for (const [kind] of kinds) {
    await panel.getByTestId('platform-financial-policy-select').selectOption(kind);
    const detail = page.getByTestId('platform-financial-policy-detail');
    await expect(detail).toBeVisible();
    await expect(detail).toContainText(brand);
    await expect(panel.getByRole('alert')).toHaveCount(0);
  }
  await page.locator('.language').click();
  await expect(panel).toContainText('发放模式');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: info.outputPath('platform-financial-policies.png'), fullPage: true });
  await expect(panel.getByRole('button', { name: /Save|Approve|Retry|保存|批准|重试/ })).toHaveCount(0);
  for (const path of ['report-archive-policy', ...kinds.map(([, path]) => path)]) {
    const denied = await page.request.put(`${origin}/api/v1/platform/${path}`, { headers: { ...headers, Origin: origin }, data: {} });
    expect([404, 405]).toContain(denied.status());
  }
  const retry = await page.request.post(`${origin}/api/v1/platform/report-archive-tasks/${crypto.randomUUID()}/retry`, { headers: { ...headers, Origin: origin }, data: {} });
  expect([404, 405]).toContain(retry.status());
  for (const [path, data] of before) {
    const response = await page.request.get(`${origin}/api/v1/platform/${path}`, { headers });
    expect(response.status()).toBe(200);
    expect((await response.json()).data).toEqual(data);
  }
  await page.locator('.content > .brand-picker select').selectOption('');
  await expect(page.getByTestId('platform-financial-policy-detail')).toHaveCount(0);
});

test('synthetic archive task paging preserves saved policy versions and clears detail failures', async ({ page }, info) => {
  const audit = crypto.randomUUID();
  const stamp = '2026-10-10T00:00:00Z';
  const policy = { brand_id: brand, version: 3, daily_enabled: false, monthly_enabled: false, daily_start_period: '2026-10-01', monthly_start_period: '2026-10', timezone: 'Asia/Singapore', audit_log_id: audit, updated_at: stamp };
  const items = Array.from({ length: 51 }, (_, index) => ({
    id: crypto.randomUUID(), brand_id: brand, policy_version: 2,
    window: { kind: 'daily', period_key: '2026-10-08', timezone: 'Asia/Singapore', from: '2026-10-07T16:00:00Z', to: '2026-10-08T16:00:00Z' },
    state: index === 50 ? 'skipped' : 'failed', version: 2, attempt_count: 1, archive_id: index === 50 ? crypto.randomUUID() : null,
    last_error_code: index === 50 ? null : 'ARCHIVE_FAILED', creation_audit_log_id: audit, last_audit_log_id: audit, created_at: stamp, updated_at: stamp,
  }));
  let fail = false;
  await page.route('**/api/v1/platform/report-archive-policy', route => route.fulfill({ json: { success: true, data: policy } }));
  await page.route('**/api/v1/platform/report-archive-tasks?**', async route => {
    const url = new URL(route.request().url());
    const offset = Number(url.searchParams.get('offset'));
    await route.fulfill({ json: { success: true, data: { brand_id: brand, items: items.slice(offset, offset + 50), total_count: '51', limit: 50, offset } } });
  });
  await page.route('**/api/v1/platform/report-archive-tasks/*', async route => {
    if (fail) await route.fulfill({ status: 503, json: { success: false, error: { message: 'Task unavailable' } } });
    else await route.fulfill({ json: { success: true, data: items.find(item => route.request().url().endsWith(item.id)) } });
  });
  await operations(page, 'Archive tasks');
  await expect(page.getByTestId('platform-archive-policy')).toContainText('2026-10-01');
  await expect(page.getByTestId('platform-archive-task-list').locator('tbody tr')).toHaveCount(50);
  await page.getByTestId('platform-archive-task-pages').getByRole('button', { name: 'Next page', exact: true }).click();
  await expect(page.getByTestId('platform-archive-task-list').locator('tbody tr')).toHaveCount(1);
  await page.getByTestId('platform-archive-task-list').getByText(items[50].id, { exact: true }).click();
  const detail = page.getByTestId('platform-archive-task-detail');
  await expect(detail).toContainText(items[50].archive_id!);
  await expect(detail).toContainText('does not mean this task produced a new archive');
  await expect(detail).toContainText('2026-10-07T16:00:00Z');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: info.outputPath('platform-archive-task.png'), fullPage: true });
  fail = true;
  await page.getByTestId('platform-archive-task-list').getByText(items[50].id, { exact: true }).click();
  await expect(detail.getByRole('alert')).toContainText('Task unavailable');
  await expect(detail).not.toContainText(items[50].archive_id!);
  await page.locator('.content > .brand-picker select').selectOption('');
  await expect(page.getByTestId('platform-archive-policy')).toHaveCount(0);
  await expect(page.getByTestId('platform-archive-task-list')).toHaveCount(0);
});

test('financial policy views preserve integer limits and clear errors independently by policy', async ({ page }) => {
  let denied = false;
  const amount = '9007199254740993';
  await page.route('**/api/v1/platform/point-policy', route => route.fulfill({ json: { success: true, data: { brand_id: brand, version: 1, max_balance_points: amount, max_recharge_points: null, max_adjustment_points: null } } }));
  await page.route('**/api/v1/platform/withdrawal-policy', async route => {
    await route.fulfill(denied ? { status: 403, json: { success: false, error: { message: 'Policy permission denied' } } } : { json: { success: true, data: {
      brand_id: brand, version: 2, updated_at: '2026-10-10T00:00:00Z', audit_log_id: crypto.randomUUID(),
      config: { enabled: true, min_points: '1', max_points: amount, allowed_sources: ['recharge', 'winning', 'commission', 'gift'], review_mode: 'manual', turnover_multiple: '2.5' },
    } } });
  });
  await operations(page, 'Financial policies');
  const panel = page.getByTestId('platform-financial-policies');
  await expect(page.getByTestId('platform-financial-policy-detail')).toContainText(amount);
  await expect(panel).toContainText('Unlimited');
  await panel.getByTestId('platform-financial-policy-select').selectOption('withdrawal');
  await expect(panel).toContainText('2.5');
  await expect(panel).toContainText('Commission');
  denied = true;
  await panel.getByRole('button', { name: 'Refresh', exact: true }).click();
  await expect(panel.getByRole('alert')).toContainText('Policy permission denied');
  await expect(page.getByTestId('platform-financial-policy-detail')).toHaveCount(0);
  await panel.getByTestId('platform-financial-policy-select').selectOption('points');
  await expect(panel.getByRole('alert')).toHaveCount(0);
  await expect(panel).toContainText(amount);
  await page.locator('.language').click();
  await expect(panel).toContainText('积分余额上限');
});
