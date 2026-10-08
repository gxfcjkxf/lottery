import { expect, test, type APIResponse, type Page } from '@playwright/test';

const origin = process.env.TEST_ARCHIVE_POLICY_ORIGIN;
const brand = '0199a000-0000-7000-8000-000000000002';
const viewport = process.env.REPORT_ARCHIVE_POLICY_VIEWPORT;
const username = process.env.TEST_ARCHIVE_POLICY_ADMIN_USERNAME;
const password = process.env.TEST_ARCHIVE_POLICY_ADMIN_PASSWORD;
const ack = 'owned_synthetic_database';

type Policy = {
  brand_id: string;
  version: number;
  daily_enabled: boolean;
  monthly_enabled: boolean;
  daily_start_period: string | null;
  monthly_start_period: string | null;
  timezone: string;
  audit_log_id: string | null;
  updated_at: string;
};

async function data<T>(response: Pick<APIResponse, 'text' | 'status'>, status = 200): Promise<T> {
  const body = await response.text();
  expect(response.status(), body).toBe(status);
  const envelope = JSON.parse(body);
  expect(envelope.success).toBe(true);
  return envelope.data as T;
}

async function navigate(page: Page, project: string, name: string) {
  if (project === 'mobile' && name === 'Dashboard') {
    await page.locator('.mobile-nav').getByRole('button', { name: /Dashboard/ }).click();
  } else if (project === 'mobile') {
    await page.locator('.mobile-nav button').nth(4).click();
    await page.locator('.mobile-more-menu').getByRole('button', { name: new RegExp(name) }).click();
  } else {
    await page.locator('.side-nav').getByRole('button', { name: new RegExp(name) }).click();
  }
}

function periodAt(value: string, timezone: string, monthly: boolean): string {
  const parts = new Intl.DateTimeFormat('en', {
    timeZone: timezone,
    year: 'numeric',
    month: '2-digit',
    ...(monthly ? {} : { day: '2-digit' }),
  }).formatToParts(new Date(value));
  const part = (type: string) => parts.find(item => item.type === type)!.value;
  return `${part('year')}-${part('month')}${monthly ? '' : `-${part('day')}`}`;
}

test('real archive policy activation recovers the original receipt after a later configuration change', async ({ page, context }, info) => {
  test.setTimeout(60_000);
  page.setDefaultTimeout(10_000);
  expect(process.env.REPORT_ARCHIVE_POLICY_FIXTURE_CONFIRM).toBe(ack);
  expect(viewport).toBe(info.project.name);
  expect(/^http:\/\/localhost:(?:5174|15292)$/.test(origin ?? '')).toBe(true);
  expect(username).toBe(`archive_policy_browser_${info.project.name}`);
  expect(password).toBeTruthy();

  const browserAdmin = `${origin}/api/v1/admin`;
  const headers = { 'X-Brand-ID': brand };
  const get = <T>(path: string) => page.request.get(`${browserAdmin}${path}`, { headers }).then(response => data<T>(response));
  const post = <T>(path: string, body: unknown, key: string, status = 200) => page.request.post(`${browserAdmin}${path}`, {
    headers: { Origin: origin!, ...headers, 'Idempotency-Key': key },
    data: body,
  }).then(response => data<T>(response, status));
  const put = <T>(path: string, body: unknown, key: string, actor: string) => page.request.put(`${browserAdmin}${path}`, {
    headers: { Origin: origin!, ...headers, 'X-Report-Archive-Actor-ID': actor, 'Idempotency-Key': key },
    data: body,
  }).then(response => data<T>(response));
  const errors: string[] = [];
  page.on('pageerror', problem => errors.push(problem.message));

  await data(await page.request.post(`${browserAdmin}/auth/login`, {
    headers: { Origin: origin!, 'Idempotency-Key': crypto.randomUUID() },
    data: { identifier: username, password },
  }));
  expect((await context.cookies(`${browserAdmin}/me`)).some(cookie => cookie.name === 'lottery_admin')).toBe(true);
  const me = await get<{ account: { id: string; permissions_by_brand: Record<string, string[]> } }>('/me');
  expect(me.account.permissions_by_brand[brand]).toEqual(expect.arrayContaining(['report_archive.view.brand', 'report_archive_policy.write.brand']));

  const initial = await get<Policy>('/report-archive-policy');
  expect(initial).toMatchObject({ brand_id: brand, version: 1, daily_enabled: false, monthly_enabled: false, daily_start_period: null, monthly_start_period: null });

  const member = await post<{ member_id: string }>('/users', {
    username: `normalmember_${crypto.randomUUID().replaceAll('-', '').slice(0, 12)}`,
    password: 'owned-archive-policy-member-password-2026',
    display_name: 'Normal member for archive policy verification',
    reason: 'Create real member for archive policy economic fingerprint',
  }, crypto.randomUUID(), 201);
  const recharge = await post<{ id: string; version: number }>('/recharges', {
    member_id: member.member_id,
    points: '37',
    proof_reference: 'Owned offline archive policy funding',
    remark: 'Synthetic browser funding',
    reason: 'Create real 37 point funding for archive policy verification',
  }, crypto.randomUUID(), 201);
  await post(`/recharges/${recharge.id}/confirm`, { version: recharge.version, reason: 'Confirm actual offline funding' }, crypto.randomUUID());
  const economics = async () => ({
    wallet: await get<{ available_points: string }>(`/wallets/${member.member_id}`),
    ledger: await get<{ items: unknown[] }>(`/wallets/${member.member_id}/ledger?limit=100&offset=0`),
  });
  const before = await economics();
  expect(before.wallet.available_points).toBe('37');
  expect(before.ledger.items).toHaveLength(1);

  const writes: Array<{ body: string | null; key: string | undefined; actor: string | undefined; brand: string | undefined }> = [];
  let originalReceipt: Policy | undefined;
  await page.route(`**/api/v1/admin/report-archive-policy`, async route => {
    if (route.request().method() !== 'PUT') {
      await route.continue();
      return;
    }
    const requestHeaders = route.request().headers();
    writes.push({
      body: route.request().postData(),
      key: requestHeaders['idempotency-key'],
      actor: requestHeaders['x-report-archive-actor-id'],
      brand: requestHeaders['x-brand-id'],
    });
    const response = await route.fetch();
    const body = await response.text();
    expect(response.status(), body).toBe(200);
    const envelope = JSON.parse(body);
    expect(envelope.success).toBe(true);
    if (writes.length === 1) {
      originalReceipt = envelope.data as Policy;
      await route.abort('failed');
      return;
    }
    expect(envelope.data).toEqual(originalReceipt);
    await route.fulfill({ response });
  });

  await page.addInitScript(() => localStorage.setItem('lottery.admin.locale', 'en'));
  await page.goto(origin!);
  await page.getByLabel(/Select live admin brand/, { exact: true }).selectOption(brand);
  await navigate(page, info.project.name, 'Automatic archive tasks');
  let panel = page.locator('.archive-tasks');
  await expect(panel.getByTestId('archive-tasks-policy')).toBeVisible();
  await expect(panel.getByTestId('archive-policy-editor')).toBeVisible();
  const currentPolicy = panel.getByTestId('archive-tasks-policy');
  await expect(currentPolicy.locator('dd').nth(1)).toHaveText('1');
  await expect(panel.getByTestId('archive-policy-daily')).not.toBeChecked();
  await expect(panel.getByTestId('archive-policy-monthly')).not.toBeChecked();
  await panel.getByTestId('archive-policy-daily').check();
  await panel.getByTestId('archive-policy-monthly').check();
  const reason = 'Activate daily and monthly archives from this server date';
  await panel.getByTestId('archive-policy-reason').fill(reason);
  await panel.getByTestId('archive-policy-save').click();
  const review = page.getByTestId('archive-policy-review');
  await expect(review).toBeVisible();
  await review.getByRole('checkbox').check();
  const reviewDetails = await review.locator('dl').first().innerText();
  await review.getByTestId('archive-policy-submit').click();
  await expect(panel.getByRole('alert').filter({ hasText: /outcome is unknown/i })).toBeVisible();
  expect(originalReceipt).toMatchObject({ brand_id: brand, version: 2, daily_enabled: true, monthly_enabled: true });
  expect(originalReceipt!.daily_start_period).toBe(periodAt(originalReceipt!.updated_at, originalReceipt!.timezone, false));
  expect(originalReceipt!.monthly_start_period).toBe(periodAt(originalReceipt!.updated_at, originalReceipt!.timezone, true));
  expect(writes).toHaveLength(1);
  const frozenRequest = writes[0]!;
  expect(frozenRequest.brand).toBe(brand);
  expect(frozenRequest.actor).toBe(me.account.id);
  expect(frozenRequest.key).toMatch(/^[A-Za-z0-9_:.-]{8,128}$/);
  const frozenBody = JSON.parse(frozenRequest.body!);
  expect(frozenBody).toEqual({ version: 1, daily_enabled: true, monthly_enabled: true, reason });
  expect(reviewDetails).toContain(me.account.id);
  expect(reviewDetails).toContain(frozenRequest.key!);
  await expect(panel.getByTestId('archive-policy-replay')).toBeDisabled();

  // Reads and navigation preserve the unresolved intent and never send another PUT.
  await expect(await get<Policy>('/report-archive-policy')).toMatchObject({ version: 2, daily_enabled: true, monthly_enabled: true });
  await navigate(page, info.project.name, 'Dashboard');
  await navigate(page, info.project.name, 'Automatic archive tasks');
  panel = page.locator('.archive-tasks');
  await expect(panel.getByTestId('archive-policy-frozen')).toContainText(/Policy update outcome unknown/);
  await expect(panel.getByTestId('archive-policy-frozen')).toContainText(me.account.id);
  await expect(panel.getByTestId('archive-policy-frozen')).toContainText(brand);
  await expect(panel.getByTestId('archive-policy-frozen')).toContainText(frozenRequest.key!);
  await expect(panel.getByTestId('archive-policy-frozen').locator('dd').nth(2)).toHaveText('1');
  await expect(panel.getByTestId('archive-tasks-policy').locator('dd').nth(1)).toHaveText('2');
  expect(writes).toHaveLength(1);

  const disabled = await put<Policy>('/report-archive-policy', {
    version: 2,
    daily_enabled: false,
    monthly_enabled: false,
    reason: 'Disable both archives in a separate current policy update',
  }, `policy-current-${crypto.randomUUID()}`, me.account.id);
  expect(disabled).toMatchObject({ brand_id: brand, version: 3, daily_enabled: false, monthly_enabled: false });
  expect(disabled.daily_start_period).toBe(originalReceipt!.daily_start_period);
  expect(disabled.monthly_start_period).toBe(originalReceipt!.monthly_start_period);

  await navigate(page, info.project.name, 'Dashboard');
  await navigate(page, info.project.name, 'Automatic archive tasks');
  panel = page.locator('.archive-tasks');
  await expect(panel.getByTestId('archive-tasks-policy')).toBeVisible();
  const livePolicy = panel.getByTestId('archive-tasks-policy');
  await expect(livePolicy.locator('dd').nth(1)).toHaveText('3');
  await expect(livePolicy.locator('dd').nth(2)).toHaveText('false');
  await expect(livePolicy.locator('dd').nth(3)).toHaveText('false');
  await expect(livePolicy.locator('dd').nth(4)).toHaveText(originalReceipt!.daily_start_period!);
  await expect(livePolicy.locator('dd').nth(5)).toHaveText(originalReceipt!.monthly_start_period!);
  await expect(panel.getByTestId('archive-policy-daily')).not.toBeChecked();
  await expect(panel.getByTestId('archive-policy-monthly')).not.toBeChecked();
  await expect(panel.getByTestId('archive-policy-frozen')).toContainText(reason);
  expect(writes).toHaveLength(1);

  await panel.getByTestId('archive-policy-replay-check').check();
  await panel.getByTestId('archive-policy-replay').click();
  const receipt = panel.getByTestId('archive-policy-receipt');
  await expect(receipt).toBeVisible();
  await expect(receipt.locator('dd').nth(1)).toHaveText('2');
  await expect(receipt.locator('dd').nth(2)).toHaveText('true');
  await expect(receipt.locator('dd').nth(3)).toHaveText('true');
  await expect(panel.getByTestId('archive-tasks-policy').locator('dd').nth(1)).toHaveText('3');
  await expect(panel.getByTestId('archive-policy-daily')).not.toBeChecked();
  await expect(panel.getByTestId('archive-policy-monthly')).not.toBeChecked();
  expect(writes).toHaveLength(2);
  expect(writes[1]).toEqual(writes[0]);
  expect(JSON.parse(writes[1]!.body!)).toEqual(frozenBody);
  expect(await economics()).toEqual(before);

  const tasks = await get<{ items: unknown[]; total_count: string }>('/report-archive-tasks?limit=100&offset=0');
  expect(tasks).toMatchObject({ items: [], total_count: '0' });
  const archives = await get<{ items: unknown[]; total_count: string }>('/report-archives?limit=100&offset=0');
  expect(archives).toMatchObject({ items: [], total_count: '0' });
  expect(await economics()).toEqual(before);

  await page.getByTestId('admin-language').selectOption('en');
  await expect(panel.getByRole('heading', { name: 'Automatic archive tasks', exact: true })).toBeVisible();
  const width = await page.evaluate(() => ({ document: document.documentElement.scrollWidth, viewport: innerWidth }));
  expect(width.document).toBeLessThanOrEqual(width.viewport);
  expect(errors).toEqual([]);
  await page.screenshot({ path: `.local/report-archive-policy-${info.project.name}.png`, fullPage: true });
});
