import { test, expect } from './platform-fixture';

const origin = 'http://127.0.0.1:5185';
const brandOrigin = 'http://127.0.0.1:5184';
const brand = '0199a000-0000-7000-8000-000000000001';
const foreignBrand = '0199a000-0000-7000-8000-000000000002';

test('platform commission pages preserve exact totals and clear failed details', async ({ page }, info) => {
  const id = (n: number) => `aaaaaaaa-aaaa-4aaa-8aaa-${n.toString(16).padStart(12, '0')}`;
  const at = '2026-10-10T00:00:00Z';
  const amount = '9007199254740993';
  const cycles = Array.from({ length: 51 }, (_, n) => ({ id: id(n + 1), brand_id: brand,
    window_from: '2026-10-03T00:00:00Z', window_to: at, anchor_order_id: id(100),
    calendar: { timezone: 'UTC', cycle: 'weekly', boundary_time: '00:00:00', weekday: 6, month_day: null, short_month: '' },
    state: 'ready', version: 4, target_count: '51', scan_complete: true, current_run_id: id(101), current_generation: '1', evidence_epoch: '0', evidence_current: true,
    calculated_count: '51', earning_count: '51', total_points: amount, created_by: null, creation_actor_type: 'system', reason: 'Synthetic read-only cycle',
    created_at: at, updated_at: at, last_error_code: null, creation_audit_log_id: id(102),
  }));
  const payments = Array.from({ length: 51 }, (_, n) => ({ id: id(n + 200), brand_id: brand, cycle_id: cycles[50].id, run_id: id(101),
    state: 'paid', payout_mode: 'automatic', version: 4, evidence_epoch: '0', total_points: amount, paid_points: amount, target_count: '51', paid_count: '51',
    creation_audit_log_id: id(102), last_error_code: null, created_at: at, updated_at: at,
  }));
  const earnings = Array.from({ length: 51 }, (_, n) => ({ id: id(n + 300), brand_id: brand, cycle_id: cycles[50].id, run_id: id(101),
    agent_id: id(400), member_id: id(401), exact_amount: { numerator: amount, denominator: '1' }, points: amount, created_at: at,
  }));
  let fail = false;
  let changedRun = false;
  await page.route('**/api/v1/platform/commission-**', async route => {
    expect(route.request().method()).toBe('GET');
    expect(route.request().headers()['x-brand-id']).toBe(brand);
    const url = new URL(route.request().url());
    const limit = Number(url.searchParams.get('limit'));
    const offset = Number(url.searchParams.get('offset'));
    const collection = url.pathname.endsWith('/commission-cycles') || url.pathname.endsWith('/commission-payments');
    let data: unknown;
    if (url.pathname.endsWith('/earnings')) {
      data = { brand_id: brand, cycle_id: cycles[50].id, items: earnings.slice(offset, offset + limit).map(row => changedRun ? { ...row, run_id: id(999) } : row), total_count: '51', limit, offset };
    } else if (collection) {
      const rows = url.pathname.endsWith('/commission-cycles') ? cycles : payments;
      data = { brand_id: brand, items: rows.slice(offset, offset + limit), total_count: '51', limit, offset };
    } else if (fail) {
      await route.fulfill({ status: 503, json: { success: false, error: { code: 'TEST_UNAVAILABLE', message: 'Commission read unavailable' } } });
      return;
    } else data = [...cycles, ...payments].find(row => url.pathname.endsWith(row.id));
    await route.fulfill({ json: { success: true, data } });
  });
  await page.goto(origin);
  await expect(page.locator('.app-frame')).toBeVisible();
  await page.locator('.nav-item').filter({ hasText: 'Agents and commission' }).click();
  await page.locator('.brand-picker select').selectOption(brand);
  const panel = page.getByTestId('platform-commission');
  const tabs = page.getByTestId('platform-commission-tabs');
  await tabs.getByRole('tab', { name: 'Cycles', exact: true }).click();
  const list = page.getByTestId('platform-commission-list');
  await expect(list.locator('tbody tr')).toHaveCount(50);
  await panel.getByTestId('platform-commission-list-pages').getByRole('button', { name: 'Next page' }).click();
  await expect(list.locator('tbody tr')).toHaveCount(1);
  await list.getByRole('button', { name: cycles[50].id, exact: true }).click();
  const detail = page.getByTestId('platform-commission-details');
  await expect(detail).toContainText(amount);
  await expect(detail.locator('tbody tr')).toHaveCount(50);
  await page.getByTestId('platform-commission-earnings-pages').getByRole('button', { name: 'Next page' }).click();
  await expect(detail.locator('tbody tr')).toHaveCount(1);
  changedRun = true;
  await page.getByTestId('platform-commission-earnings-pages').getByRole('button', { name: 'Previous page' }).click();
  await expect(panel.getByRole('alert')).toContainText('Commission run changed');
  await expect(detail).not.toContainText(id(999));
  changedRun = false;
  await tabs.getByRole('tab', { name: 'Payments', exact: true }).click();
  await expect(list.locator('tbody tr')).toHaveCount(50);
  await panel.getByTestId('platform-commission-list-pages').getByRole('button', { name: 'Next page' }).click();
  await expect(list.locator('tbody tr')).toHaveCount(1);
  await list.getByRole('button', { name: payments[50].id, exact: true }).click();
  await expect(detail).toContainText(amount);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: info.outputPath('platform-commission-payment.png'), fullPage: true });
  fail = true;
  await list.getByRole('button', { name: payments[50].id, exact: true }).click();
  await expect(panel.getByRole('alert')).toContainText('Commission read unavailable');
  await expect(detail).toHaveCount(0);
  await page.locator('.language').click();
  await expect(tabs.getByRole('tab', { name: '派发', exact: true })).toBeVisible();
  await page.locator('.brand-picker select').selectOption('');
  await expect(panel).toHaveCount(0);
});

test('platform reads a real agent and commission collections without financial or agent writes', async ({ page, playwright, operatorSession }, info) => {
  await page.goto('http://127.0.0.1:5183/register');
  await page.getByLabel('Choose a username or phone', { exact: true }).fill(`agent_review_${crypto.randomUUID().replaceAll('-', '').slice(0, 10)}`);
  await page.getByLabel('Password', { exact: true }).fill('owned-agent-review-password-2026');
  await page.locator('.auth-form input[type="checkbox"]').nth(0).check();
  await page.locator('.auth-form input[type="checkbox"]').nth(1).check();
  const registered = page.waitForResponse(r => r.url() === 'http://127.0.0.1:5183/api/v1/auth/register');
  await page.getByRole('button', { name: /Continue/ }).click();
  expect((await registered).status()).toBe(201);
  await expect(page).toHaveURL(/\/account$/);
  const me = await page.request.get('http://127.0.0.1:5183/api/v1/me');
  expect(me.status()).toBe(200);
  const member = (await me.json()).data.member.id;
  const operator = await playwright.request.newContext({ storageState: operatorSession });
  const headers = { Origin: brandOrigin, 'X-Brand-ID': brand };
  const base = `${brandOrigin}/api/v1/admin`;
  let agent: { id: string; member_id: string; version: number };
  try {
    const policyResponse = await operator.get(`${base}/agent-policy`, { headers });
    expect(policyResponse.status()).toBe(200);
    const policy = (await policyResponse.json()).data;
    let policyVersion = policy.version;
    if (!policy.config.enabled) {
      const enabled = await operator.put(`${base}/agent-policy`, {
        headers: { ...headers, 'Idempotency-Key': crypto.randomUUID() },
        data: { version: policy.version, config: { ...policy.config, enabled: true }, reason: 'Enable owned agent query fixture' },
      });
      expect(enabled.status(), await enabled.text()).toBe(200);
      policyVersion = (await enabled.json()).data.version;
    }
    const created = await operator.post(`${base}/agents`, {
      headers: { ...headers, 'Idempotency-Key': crypto.randomUUID() },
      data: { policy_version: policyVersion, member_id: member, parent_id: null, parent_version: null,
        config: { ratio: '0', mode: null, status: 'active', can_create_children: false }, reason: 'Create owned agent for platform read verification' },
    });
    expect(created.status(), await created.text()).toBe(201);
    agent = (await created.json()).data;
  } finally { await operator.dispose(); }
  const platformHeaders = { 'X-Brand-ID': brand };
  const read = async (path: string) => {
    const response = await page.request.get(`${origin}/api/v1/platform${path}`, { headers: platformHeaders });
    expect(response.status(), await response.text()).toBe(200);
    return (await response.json()).data;
  };
  const paths = [`/agents/${agent.id}`, `/wallets/${member}`, '/commission-cycles?limit=50&offset=0', '/commission-payments?limit=50&offset=0'];
  const before = [];
  for (const path of paths) before.push(await read(path));
  await page.goto(origin);
  await expect(page.locator('.app-frame')).toBeVisible();
  await page.locator('.nav-item').filter({ hasText: 'Agents and commission' }).click();
  await page.locator('.brand-picker select').selectOption(brand);
  const panel = page.getByTestId('platform-commission');
  await panel.getByRole('button', { name: agent.id, exact: true }).click();
  await expect(page.getByTestId('platform-commission-details')).toContainText(member);
  await expect(panel.getByRole('alert')).toHaveCount(0);
  await page.getByTestId('platform-commission-tabs').getByRole('tab', { name: 'Cycles', exact: true }).click();
  await expect(panel.getByRole('alert')).toHaveCount(0);
  await page.getByTestId('platform-commission-tabs').getByRole('tab', { name: 'Payments', exact: true }).click();
  await expect(panel.getByRole('alert')).toHaveCount(0);
  await expect(panel.getByRole('button', { name: /Approve|Create|Pay out|Retry|Cancel|Edit/i })).toHaveCount(0);
  const denied = await page.request.post(`${origin}/api/v1/platform/agents`, {
    headers: { Origin: origin, ...platformHeaders, 'Idempotency-Key': crypto.randomUUID() }, data: { member_id: member },
  });
  expect([404, 405]).toContain(denied.status());
  const foreign = await page.request.get(`${origin}/api/v1/platform/agents/${agent.id}`, { headers: { 'X-Brand-ID': foreignBrand } });
  expect(foreign.status()).toBe(404);
  const after = [];
  for (const path of paths) after.push(await read(path));
  expect(after).toEqual(before);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: info.outputPath('platform-commission-real.png'), fullPage: true });
  await page.locator('.brand-picker select').selectOption('');
  await expect(panel).toHaveCount(0);
});
