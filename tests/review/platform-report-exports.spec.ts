import { createHash } from 'node:crypto';
import { test, expect } from './platform-fixture';

test.use({ platformAccountPrefix: 'review_exports' });
const origin = 'http://127.0.0.1:5185';
const brand = '0199a000-0000-7000-8000-000000000001';
const kinds = ['betting', 'ledger', 'withdrawal', 'commission', 'rewards', 'reward_orders'] as const;

test('platform exports six genuine complete CSV reports with committed audit evidence and unchanged funds', async ({ page, playwright, operatorSession }) => {
  await page.goto('http://127.0.0.1:5183/register');
  await page.getByLabel('Choose a username or phone', { exact: true }).fill(`export_review_${crypto.randomUUID().replaceAll('-', '').slice(0, 10)}`);
  await page.getByLabel('Password', { exact: true }).fill('owned-export-review-password-2026');
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
  const brandOrigin = 'http://127.0.0.1:5184';
  const headers = { Origin: brandOrigin, 'X-Brand-ID': brand };
  try {
    const created = await operator.post(`${brandOrigin}/api/v1/admin/recharges`, {
      headers: { ...headers, 'Idempotency-Key': crypto.randomUUID() },
      data: { member_id: member, points: '7', proof_reference: 'owned-export-review', remark: 'synthetic funding', reason: 'Prepare actual CSV posting report' },
    });
    expect(created.status()).toBe(201);
    const order = (await created.json()).data;
    const confirmed = await operator.post(`${brandOrigin}/api/v1/admin/recharges/${order.id}/confirm`, {
      headers: { ...headers, 'Idempotency-Key': crypto.randomUUID() }, data: { version: order.version, reason: 'Confirm owned CSV funding' },
    });
    expect(confirmed.status()).toBe(200);
  } finally { await operator.dispose(); }
  const read = async (path: string) => {
    const response = await page.request.get(`${origin}/api/v1/platform${path}`, { headers: { 'X-Brand-ID': brand } });
    expect(response.status(), await response.text()).toBe(200);
    return (await response.json()).data;
  };
  const walletPath = `/wallets/${member}`;
  const ledgerPath = `${walletPath}/ledger?limit=51&offset=0`;
  const before = [await read(walletPath), await read(ledgerPath)];
  await page.goto(origin);
  await expect(page.locator('.app-frame')).toBeVisible();
  await page.locator('.nav-item').filter({ hasText: 'Member points' }).click();
  await page.locator('.brand-picker select').selectOption(brand);
  await page.getByTestId('platform-finance-tabs').getByRole('button', { name: 'Financial reports', exact: true }).click();
  const panel = page.getByTestId('platform-reports');
  const today = new Date();
  const tomorrow = new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate() + 1));
  await panel.getByLabel('From (UTC)', { exact: true }).fill(`${today.toISOString().slice(0, 10)}T00:00`);
  await panel.getByLabel('To (UTC)', { exact: true }).fill(`${tomorrow.toISOString().slice(0, 10)}T00:00`);
  await panel.getByLabel('Member ID (optional)', { exact: true }).fill(member);
  for (const kind of kinds) {
    await panel.getByLabel('Report type', { exact: true }).selectOption(kind);
    await panel.getByRole('button', { name: 'Run report', exact: true }).click();
    await expect(page.getByTestId('platform-reports-summary')).toBeVisible();
    await panel.getByRole('button', { name: 'Export complete CSV', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: 'Confirm CSV export', exact: true });
    await expect(dialog).toContainText(member);
    const segment = kind === 'reward_orders' ? 'reward-orders' : kind;
    const exportPath = `/api/v1/platform/reports/${segment}${kind === 'betting' || kind === 'ledger' || kind === 'withdrawal' ? '/export' : '.csv'}`;
    const responsePromise = page.waitForResponse(r => new URL(r.url()).pathname === exportPath);
    const downloadPromise = page.waitForEvent('download');
    await dialog.getByRole('button', { name: 'Confirm and download', exact: true }).click();
    const response = await responsePromise;
    expect(response.status(), kind).toBe(200);
    const params = new URL(response.url()).searchParams;
    expect(params.has('limit')).toBe(false);
    expect(params.has('offset')).toBe(false);
    expect(params.get('member_id')).toBe(member);
    const download = await downloadPromise;
    expect(await download.failure()).toBeNull();
    const stream = await download.createReadStream();
    if (!stream) throw new Error('Verified download stream is missing');
    const chunks: Buffer[] = [];
    for await (const chunk of stream) chunks.push(Buffer.from(chunk));
    const bytes = Buffer.concat(chunks);
    const meta = response.headers();
    expect(createHash('sha256').update(bytes).digest('hex')).toBe(meta['x-report-sha256']);
    expect(bytes.length).toBe(Number(meta['content-length']));
    expect(meta['x-report-kind']).toBe(kind);
    expect(meta['x-report-format-version']).toBe(kind === 'commission' ? '2' : '1');
    expect(download.suggestedFilename()).toMatch(new RegExp(`^lottery-${kind}-${brand}-[0-9]{8}T[0-9]{6}Z\\.csv$`));
    expect(bytes.subarray(0, 3)).toEqual(Buffer.from([0xef, 0xbb, 0xbf]));
    const csv = bytes.toString('utf8');
    expect(csv).toContain(member);
    const audit = await read(`/audit?limit=100&offset=0&action=report.${kind}.export`);
    const entry = audit.items.find((row: { id: string }) => row.id === meta['x-report-audit-id']);
    expect(entry, 'CSV audit must already be committed').toBeTruthy();
    expect(entry.after_json.sha256).toBe(meta['x-report-sha256']);
    if (kind === 'ledger') {
      expect(meta['x-report-group-count']).toBe('1');
      expect(csv).toContain('summary,');
      expect(csv).toContain('balances,');
      expect(csv).toContain(',1,7,7,0,0,0,');
    }
    await expect(page.getByTestId('platform-report-export-receipt')).toContainText(meta['x-report-audit-id']);
    await expect(dialog).toHaveCount(0);
  }
  expect([await read(walletPath), await read(ledgerPath)]).toEqual(before);
});

test('platform refuses an altered CSV and discards a late export after changing brands', async ({ page }, info) => {
  const at = '2026-10-10T00:00:00Z';
  const amount = '9007199254740993';
  const metrics = ['entry_count', 'net_points', 'recharge_points', 'prize_credit_points', 'prize_reversal_points', 'refund_points'];
  const balances = ['account_count', 'available_points', 'frozen_points', 'withdrawal_points', 'total_points'];
  const totals = Object.fromEntries(metrics.map(key => [key, key === 'entry_count' ? '1' : key === 'net_points' ? `-${amount}` : '0']));
  const summary: Record<string, string> = { ...totals, entry_count: '51', net_points: (-BigInt(amount) * 51n).toString() };
  const rows = Array.from({ length: 51 }, (_, index) => {
    const key = new Date(Date.UTC(2026, 7, index + 1)).toISOString().slice(0, 10);
    return { key, label: key, totals };
  });
  const currentBalances = Object.fromEntries(balances.map(key => [key, key === 'account_count' ? '1' : '0']));
  await page.route('**/api/v1/platform/reports/ledger?**', async route => {
    const params = new URL(route.request().url()).searchParams;
    await route.fulfill({ json: { success: true, data: { brand_id: brand, snapshot_at: at, timezone: 'UTC',
      query: { from: params.get('from'), to: params.get('to'), group_by: 'day', limit: Number(params.get('limit')), offset: Number(params.get('offset')), game_id: null, member_id: null },
      summary, items: rows.slice(Number(params.get('offset')), Number(params.get('offset')) + Number(params.get('limit'))), total_groups: '51', balances: currentBalances,
    } } });
  });
  let hold = false;
  let corrupt = true;
  let release: (() => void) | undefined;
  let entered: (() => void) | undefined;
  let enteredPromise = Promise.resolve();
  await page.route('**/api/v1/platform/reports/ledger/export?**', async route => {
    expect(route.request().method()).toBe('GET');
    const params = new URL(route.request().url()).searchParams;
    expect(params.has('limit') || params.has('offset')).toBe(false);
    const prefix = ['record_type', 'brand_id', 'snapshot_at', 'timezone', 'from', 'to', 'group_by', 'game_id', 'member_id', 'key', 'label'];
    const meta = ['', brand, at, 'UTC', params.get('from')!, params.get('to')!, 'day', '', '', '', ''];
    const body = Buffer.from('\ufeff' + [
      [...prefix, ...metrics, ...balances].join(','),
      ['summary', ...meta.slice(1), ...metrics.map(key => summary[key]), ...balances.map(() => '')].join(','),
      ['balances', ...meta.slice(1), ...metrics.map(() => ''), ...balances.map(key => currentBalances[key])].join(','),
      ...rows.map(row => ['group', ...meta.slice(1, 9), row.key, row.label, ...metrics.map(key => totals[key]), ...balances.map(() => '')].join(',')),
    ].join('\n') + '\n');
    if (hold) { entered?.(); await new Promise<void>(resolve => { release = resolve; }); }
    await route.fulfill({ body, headers: { 'Content-Type': 'text/csv; charset=utf-8', 'Content-Length': String(body.length), 'Cache-Control': 'no-store', 'X-Content-Type-Options': 'nosniff',
      'Content-Disposition': `attachment; filename="lottery-ledger-${brand}-20261010T000000Z.csv"`, 'X-Report-Brand-ID': brand, 'X-Report-Kind': 'ledger', 'X-Report-Snapshot-At': at,
      'X-Report-Group-Count': '51', 'X-Report-SHA256': corrupt ? '0'.repeat(64) : createHash('sha256').update(body).digest('hex'),
      'X-Report-Format-Version': '1', 'X-Report-Audit-ID': 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
    } });
  });
  let downloads = 0;
  page.on('download', () => downloads++);
  await page.goto(origin);
  await expect(page.locator('.app-frame')).toBeVisible();
  await page.locator('.nav-item').filter({ hasText: 'Member points' }).click();
  await page.locator('.brand-picker select').selectOption(brand);
  await page.getByTestId('platform-finance-tabs').getByRole('button', { name: 'Financial reports', exact: true }).click();
  const panel = page.getByTestId('platform-reports');
  await panel.getByLabel('Report type', { exact: true }).selectOption('ledger');
  await panel.getByLabel('From (UTC)', { exact: true }).fill('2026-08-01T00:00');
  await panel.getByLabel('To (UTC)', { exact: true }).fill('2026-10-11T00:00');
  await panel.getByRole('button', { name: 'Run report', exact: true }).click();
  await page.getByTestId('platform-reports-pages').getByRole('button', { name: 'Next page' }).click();
  await expect(page.getByTestId('platform-reports-list').locator('tbody tr')).toHaveCount(1);
  await panel.getByRole('button', { name: 'Export complete CSV', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Confirm CSV export', exact: true });
  await page.screenshot({ path: info.outputPath('platform-export-confirmation.png'), fullPage: true });
  await dialog.getByRole('button', { name: 'Confirm and download', exact: true }).click();
  await expect(panel.getByRole('alert')).toBeVisible();
  expect(downloads).toBe(0);
  corrupt = false;
  await panel.getByRole('button', { name: 'Export complete CSV', exact: true }).click();
  const completeDownload = page.waitForEvent('download');
  await dialog.getByRole('button', { name: 'Confirm and download', exact: true }).click();
  const downloaded = await completeDownload;
  const stream = await downloaded.createReadStream();
  if (!stream) throw new Error('Complete CSV stream is missing');
  const chunks: Buffer[] = [];
  for await (const chunk of stream) chunks.push(Buffer.from(chunk));
  expect(Buffer.concat(chunks).toString('utf8').split('\n').filter(line => line.startsWith('group,'))).toHaveLength(51);
  await expect(page.getByTestId('platform-report-export-receipt')).toContainText('Total groups: 51');
  expect(downloads).toBe(1);
  hold = true;
  enteredPromise = new Promise<void>(resolve => { entered = resolve; });
  await panel.getByRole('button', { name: 'Export complete CSV', exact: true }).click();
  await dialog.getByRole('button', { name: 'Confirm and download', exact: true }).click();
  await enteredPromise;
  const finished = page.waitForResponse(r => new URL(r.url()).pathname.endsWith('/ledger/export'));
  await page.locator('.brand-picker select').selectOption('0199a000-0000-7000-8000-000000000002');
  release?.();
  await finished;
  await expect(panel).toHaveCount(0);
  await expect(page.getByTestId('platform-report-export-receipt')).toHaveCount(0);
  expect(downloads).toBe(1);
});
